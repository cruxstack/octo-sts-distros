// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

package sts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"path"
	"strings"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/chainguard-dev/clog"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/go-github/v75/github"
	lru "github.com/hashicorp/golang-lru/v2"
	expirablelru "github.com/hashicorp/golang-lru/v2/expirable"
	"sigs.k8s.io/yaml"

	"github.com/cruxstack/octo-sts-distros/internal/shared"
	"github.com/octo-sts/app/pkg/octosts"
	"github.com/octo-sts/app/pkg/oidcvalidate"
	"github.com/octo-sts/app/pkg/provider"
)

var (
	// installationIDs is an LRU cache of recently used GitHub App installation IDs.
	installationIDs, _ = lru.New2Q[string, int64](200)
	trustPolicies      = expirablelru.NewLRU[cacheTrustPolicyKey, string](200, nil, 5*60*1e9) // 5 minutes
)

type cacheTrustPolicyKey struct {
	owner    string
	repo     string
	identity string
}

// HandleRequest routes requests to the appropriate handler.
func (s *STS) HandleRequest(ctx context.Context, req shared.Request) shared.Response {
	reqPath := s.stripBasePath(req.Path)

	log := clog.FromContext(ctx)
	ctx = clog.WithLogger(ctx, log)

	switch {
	case req.Method == http.MethodPost && (reqPath == "/" || reqPath == "" || reqPath == "/sts/exchange"):
		return s.handleExchange(ctx, req)
	case req.Method == http.MethodGet && (reqPath == "/exchange" || reqPath == "/sts/exchange"):
		// Support GET requests with query parameters (used by octo-sts/action)
		return s.handleExchange(ctx, req)
	case req.Method == http.MethodGet && (reqPath == "/" || reqPath == ""):
		return s.handleRoot(ctx)
	default:
		return ErrorResponse(http.StatusNotFound, "not found")
	}
}

// stripBasePath removes the configured base path prefix from the request path.
func (s *STS) stripBasePath(reqPath string) string {
	if s.basePath == "" {
		return reqPath
	}
	stripped := strings.TrimPrefix(reqPath, s.basePath)
	// Ensure the path starts with "/" after stripping
	if stripped == "" || stripped[0] != '/' {
		stripped = "/" + stripped
	}
	return stripped
}

// handleRoot returns documentation information for GET requests to root.
func (s *STS) handleRoot(_ context.Context) shared.Response {
	return JSONResponse(http.StatusOK, map[string]string{
		"msg": "please check documentation for usage: https://github.com/octo-sts/app",
	})
}

// handleExchange processes token exchange requests.
// Supports both POST with JSON body and GET with query parameters.
func (s *STS) handleExchange(ctx context.Context, req shared.Request) shared.Response {
	log := clog.FromContext(ctx)

	var exchangeReq ExchangeRequest

	// Support both GET with query params and POST with JSON body
	if req.Method == http.MethodGet {
		// Parse from query parameters (used by octo-sts/action)
		exchangeReq.Scope = req.QueryParams["scope"]
		exchangeReq.Identity = req.QueryParams["identity"]
	} else {
		// Parse from JSON body
		if err := json.Unmarshal(req.Body, &exchangeReq); err != nil {
			log.Debugf("failed to parse request body: %v", err)
			return ErrorResponse(http.StatusBadRequest, "invalid request body")
		}
	}

	log.Infof("exchange request: identity=%s, scope=%s", exchangeReq.Identity, exchangeReq.Scope)

	auth := req.Headers[HeaderAuthorization]
	if auth == "" {
		return ErrorResponse(http.StatusUnauthorized, "authorization header required")
	}
	bearer := strings.TrimPrefix(auth, "Bearer ")
	if bearer == auth {
		return ErrorResponse(http.StatusUnauthorized, "invalid authorization header format")
	}

	issuer, err := extractIssuer(bearer)
	if err != nil {
		log.Debugf("invalid bearer token: %v", err)
		return ErrorResponse(http.StatusBadRequest, "invalid bearer token")
	}

	if !oidcvalidate.IsValidIssuer(issuer) {
		return ErrorResponse(http.StatusBadRequest, "invalid issuer format")
	}

	p, err := provider.Get(ctx, issuer)
	if err != nil {
		log.Debugf("unable to fetch or create the provider: %v", err)
		return ErrorResponse(http.StatusBadRequest, "unable to fetch or create the provider")
	}

	// Audience is verified later by the trust policy
	verifier := p.Verifier(&oidc.Config{SkipClientIDCheck: true})
	tok, err := verifier.Verify(ctx, bearer)
	if err != nil {
		log.Debugf("unable to validate token: %v", err)
		return ErrorResponse(http.StatusUnauthorized, "unable to verify bearer token")
	}

	if exchangeReq.Scope == "" {
		return ErrorResponse(http.StatusBadRequest, "scope must be provided")
	}
	if exchangeReq.Identity == "" {
		return ErrorResponse(http.StatusBadRequest, "identity must be provided")
	}

	installID, trustPolicy, err := s.lookupInstallAndTrustPolicy(ctx, exchangeReq.Scope, exchangeReq.Identity)
	if err != nil {
		log.Debugf("failed to lookup trust policy: %v", err)
		return ErrorResponse(http.StatusNotFound, "unable to find trust policy")
	}
	log.Infof("trust policy: %#v", trustPolicy)

	_, err = trustPolicy.CheckToken(tok, s.domain)
	if err != nil {
		log.Warnf("token does not match trust policy: %v", err)
		return ErrorResponse(http.StatusForbidden, "token does not match trust policy")
	}

	atr := ghinstallation.NewFromAppsTransport(s.transport, installID)
	atr.InstallationTokenOptions = &github.InstallationTokenOptions{
		Repositories: trustPolicy.Repositories,
		Permissions:  &trustPolicy.Permissions,
	}

	// Log the token request details at debug level
	if shared.IsDebugEnabled() {
		log.Debugf("GitHub token exchange request: installation_id=%d, repositories=%v, permissions=%s",
			installID,
			trustPolicy.Repositories,
			formatPermissions(&trustPolicy.Permissions))
	}

	token, err := atr.Token(ctx)
	if err != nil {
		var herr *ghinstallation.HTTPError
		if errors.As(err, &herr) && herr.Response != nil {
			// Log response details at debug level
			if shared.IsDebugEnabled() {
				log.Debugf("GitHub API error response: status=%d, status_text=%s",
					herr.Response.StatusCode,
					herr.Response.Status)
			}

			if herr.Response.StatusCode == http.StatusUnprocessableEntity {
				if body, err := io.ReadAll(herr.Response.Body); err == nil {
					log.Warnf("token exchange failure (status=%d): %s", herr.Response.StatusCode, body)
					return ErrorResponse(http.StatusForbidden, "token exchange failure")
				}
			} else if herr.Response.Body != nil {
				body, err := httputil.DumpResponse(herr.Response, true)
				if err == nil {
					log.Warnf("token exchange failure (status=%d): %s", herr.Response.StatusCode, redactTokenInBody(string(body)))
				}
			}
		} else {
			log.Warnf("token exchange failure: %v", redactTokenInError(err))
		}
		return ErrorResponse(http.StatusInternalServerError, "failed to get token")
	}

	log.Infof("token exchange successful: installation_id=%d, repositories_count=%d", installID, len(trustPolicy.Repositories))
	return JSONResponse(http.StatusOK, ExchangeResponse{Token: token})
}

// lookupInstallAndTrustPolicy looks up the GitHub App installation ID and trust policy
// for the given scope and identity.
func (s *STS) lookupInstallAndTrustPolicy(ctx context.Context, scope, identity string) (int64, *octosts.OrgTrustPolicy, error) {
	otp := &octosts.OrgTrustPolicy{}
	var tp trustPolicy = &otp.TrustPolicy

	owner, repo := path.Dir(scope), path.Base(scope)
	if owner == "." {
		owner, repo = repo, ".github"
	} else {
		otp.Repositories = []string{repo}
	}

	if repo == ".github" {
		tp = otp
	}

	id, err := s.lookupInstall(ctx, owner)
	if err != nil {
		return 0, nil, err
	}

	trustPolicyKey := cacheTrustPolicyKey{
		owner:    owner,
		repo:     repo,
		identity: identity,
	}

	if err := s.lookupTrustPolicy(ctx, id, trustPolicyKey, tp); err != nil {
		return id, nil, err
	}
	return id, otp, nil
}

// trustPolicy interface for polymorphic trust policy handling
type trustPolicy interface {
	Compile() error
}

// lookupInstall looks up the GitHub App installation ID for the given owner.
func (s *STS) lookupInstall(ctx context.Context, owner string) (int64, error) {
	if v, ok := installationIDs.Get(owner); ok {
		clog.InfoContextf(ctx, "found installation in cache for %s", owner)
		return v, nil
	}

	client := github.NewClient(&http.Client{
		Transport: s.transport,
	})

	page := 1
	for page != 0 {
		installs, resp, err := client.Apps.ListInstallations(ctx, &github.ListOptions{
			Page:    page,
			PerPage: 100,
		})
		if err != nil {
			return 0, err
		}

		for _, install := range installs {
			if install.Account.GetLogin() == owner {
				installID := install.GetID()
				installationIDs.Add(owner, installID)
				return installID, nil
			}
		}
		page = resp.NextPage
	}

	return 0, fmt.Errorf("no installation found for %q", owner)
}

// lookupTrustPolicy fetches and parses the trust policy for the given identity.
func (s *STS) lookupTrustPolicy(ctx context.Context, install int64, trustPolicyKey cacheTrustPolicyKey, tp trustPolicy) error {
	raw := ""
	if cachedRawPolicy, ok := trustPolicies.Get(trustPolicyKey); ok {
		clog.InfoContextf(ctx, "found trust policy in cache for %s", trustPolicyKey)
		raw = cachedRawPolicy
	}

	if raw == "" {
		atr := ghinstallation.NewFromAppsTransport(s.transport, install)
		atr.InstallationTokenOptions = &github.InstallationTokenOptions{
			Repositories: []string{trustPolicyKey.repo},
			Permissions: &github.InstallationPermissions{
				Contents: ptr("read"),
			},
		}
		defer func() {
			tok, err := atr.Token(ctx)
			if err != nil {
				clog.WarnContextf(ctx, "failed to get token for revocation: %v", err)
				return
			}
			if err := octosts.Revoke(ctx, tok); err != nil {
				clog.WarnContextf(ctx, "failed to revoke token: %v", err)
				return
			}
		}()

		client := github.NewClient(&http.Client{
			Transport: atr,
		})

		file, _, _, err := client.Repositories.GetContents(ctx,
			trustPolicyKey.owner, trustPolicyKey.repo,
			fmt.Sprintf(".github/chainguard/%s.sts.yaml", trustPolicyKey.identity),
			&github.RepositoryContentGetOptions{},
		)
		if err != nil {
			clog.InfoContextf(ctx, "failed to find trust policy: %v", err)
			return fmt.Errorf("unable to find trust policy for %q", trustPolicyKey.identity)
		}

		raw, err = file.GetContent()
		if err != nil {
			clog.ErrorContextf(ctx, "failed to read trust policy: %v", err)
			return fmt.Errorf("unable to read trust policy for %q", trustPolicyKey.identity)
		}

		if evicted := trustPolicies.Add(trustPolicyKey, raw); evicted {
			clog.InfoContextf(ctx, "evicted cachekey %s", trustPolicyKey)
		}
	}

	if err := yaml.UnmarshalStrict([]byte(raw), tp); err != nil {
		clog.InfoContextf(ctx, "failed to parse trust policy: %v", err)
		return fmt.Errorf("unable to parse trust policy for %q", trustPolicyKey.identity)
	}

	if err := tp.Compile(); err != nil {
		clog.InfoContextf(ctx, "failed to compile trust policy: %v", err)
		return fmt.Errorf("unable to compile trust policy for %q", trustPolicyKey.identity)
	}

	return nil
}

// extractIssuer extracts the issuer claim from a JWT without verification.
func extractIssuer(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", errors.New("invalid JWT format")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("failed to decode JWT payload: %w", err)
	}

	var claims struct {
		Issuer string `json:"iss"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("failed to parse JWT claims: %w", err)
	}

	if claims.Issuer == "" {
		return "", errors.New("JWT missing issuer claim")
	}

	return claims.Issuer, nil
}

func ptr[T any](in T) *T {
	return &in
}

// formatPermissions returns a string representation of the permissions being requested.
func formatPermissions(perms *github.InstallationPermissions) string {
	if perms == nil {
		return "{}"
	}

	parts := []string{}
	if perms.Contents != nil {
		parts = append(parts, fmt.Sprintf("contents:%s", *perms.Contents))
	}
	if perms.Actions != nil {
		parts = append(parts, fmt.Sprintf("actions:%s", *perms.Actions))
	}
	if perms.Issues != nil {
		parts = append(parts, fmt.Sprintf("issues:%s", *perms.Issues))
	}
	if perms.PullRequests != nil {
		parts = append(parts, fmt.Sprintf("pull_requests:%s", *perms.PullRequests))
	}
	if perms.Packages != nil {
		parts = append(parts, fmt.Sprintf("packages:%s", *perms.Packages))
	}
	if perms.Metadata != nil {
		parts = append(parts, fmt.Sprintf("metadata:%s", *perms.Metadata))
	}
	if perms.Statuses != nil {
		parts = append(parts, fmt.Sprintf("statuses:%s", *perms.Statuses))
	}
	if perms.Checks != nil {
		parts = append(parts, fmt.Sprintf("checks:%s", *perms.Checks))
	}
	if perms.Deployments != nil {
		parts = append(parts, fmt.Sprintf("deployments:%s", *perms.Deployments))
	}
	if perms.Administration != nil {
		parts = append(parts, fmt.Sprintf("administration:%s", *perms.Administration))
	}

	if len(parts) == 0 {
		return "{}"
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// redactTokenInBody redacts any token values in the response body for safe logging.
func redactTokenInBody(body string) string {
	// Redact common token patterns in JSON responses
	if strings.Contains(body, "token") {
		for _, prefix := range []string{"ghs_", "ghp_", "gho_", "ghu_", "github_pat_"} {
			for {
				idx := strings.Index(body, prefix)
				if idx == -1 {
					break
				}
				// Find the end of the token (typically ends at quote, space, or end of string)
				endIdx := idx + len(prefix)
				for endIdx < len(body) && body[endIdx] != '"' && body[endIdx] != ' ' && body[endIdx] != '\n' {
					endIdx++
				}
				body = body[:idx] + "[REDACTED]" + body[endIdx:]
			}
		}
	}
	return body
}

// redactTokenInError redacts any token values in error messages for safe logging.
func redactTokenInError(err error) string {
	if err == nil {
		return ""
	}
	return redactTokenInBody(err.Error())
}
