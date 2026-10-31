// Copyright 2026 CruxStack
// SPDX-License-Identifier: MIT

package sts

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/chainguard-dev/clog/slogtest"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	josejwt "github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-github/v88/github"
	envConfig "github.com/octo-sts/app/pkg/envconfig"
	"github.com/octo-sts/app/pkg/octosts"
	"github.com/octo-sts/app/pkg/provider"

	"github.com/cruxstack/octo-sts-distros/internal/shared"
)

const testIssuer = "https://token.actions.githubusercontent.com"

func newTestToken(t *testing.T) string {
	t.Helper()

	pk, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("cannot generate RSA key %v", err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: pk}, nil)
	if err != nil {
		t.Fatalf("jose.NewSigner() = %v", err)
	}
	token, err := josejwt.Signed(signer).Claims(josejwt.Claims{
		Subject:  "foo",
		Issuer:   testIssuer,
		Audience: josejwt.Audience{"octosts"},
		Expiry:   josejwt.NewNumericDate(time.Now().Add(10 * time.Minute)),
	}).Serialize()
	if err != nil {
		t.Fatalf("CompactSerialize failed: %v", err)
	}
	provider.AddTestKeySetVerifier(t, testIssuer, &oidc.StaticKeySet{
		PublicKeys: []crypto.PublicKey{pk.Public()},
	})
	return token
}

func doExchange(t *testing.T, s *STS, token string, req ExchangeRequest) shared.Response {
	t.Helper()

	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	return s.HandleRequest(slogtest.Context(t), shared.Request{
		Type:   shared.RequestTypeHTTP,
		Method: http.MethodPost,
		Path:   "/",
		Headers: shared.NormalizeHeaders(map[string]string{
			"Authorization": "Bearer " + token,
			"Content-Type":  "application/json",
		}),
		Body: body,
	})
}

func tokenOptions(t *testing.T, resp shared.Response) *github.InstallationTokenOptions {
	t.Helper()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("HandleRequest failed: status=%d, body=%s", resp.StatusCode, string(resp.Body))
	}
	var exchangeResp ExchangeResponse
	if err := json.Unmarshal(resp.Body, &exchangeResp); err != nil {
		t.Fatalf("Unmarshal response failed: %v", err)
	}
	b, err := base64.StdEncoding.DecodeString(exchangeResp.Token)
	if err != nil {
		t.Fatalf("DecodeString failed: %v", err)
	}
	got := new(github.InstallationTokenOptions)
	if err := json.Unmarshal(b, got); err != nil {
		t.Fatalf("Unmarshal token options failed: %v", err)
	}
	return got
}

func TestExchangePolicyLocation(t *testing.T) {
	atr := newGitHubClient(t, newFakeGitHub())
	token := newTestToken(t)

	s, err := New(atr, Config{
		Domain:        "octosts",
		OrgPolicyRepo: "policy-repo",
		PolicyDir:     "policies/",
	})
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	for _, tc := range []struct {
		name       string
		req        ExchangeRequest
		wantStatus int
		want       *github.InstallationTokenOptions
	}{{
		name:       "org scope reads the org policy repo and scopes by repository ID",
		req:        ExchangeRequest{Identity: "ids", Scope: "org"},
		wantStatus: http.StatusOK,
		want: &github.InstallationTokenOptions{
			RepositoryIDs: []int64{42},
			Permissions:   &github.InstallationPermissions{Contents: github.Ptr("write")},
		},
	}, {
		name:       "repo scope reads the policy dir in that repo",
		req:        ExchangeRequest{Identity: "bar", Scope: "org/repo"},
		wantStatus: http.StatusOK,
		want: &github.InstallationTokenOptions{
			Repositories: []string{"repo"},
			Permissions:  &github.InstallationPermissions{Contents: github.Ptr("read")},
		},
	}, {
		name:       "default policy dir is not read",
		req:        ExchangeRequest{Identity: "foo", Scope: "org/repo"},
		wantStatus: http.StatusNotFound,
	}, {
		name:       "default org policy repo is not read",
		req:        ExchangeRequest{Identity: "foo", Scope: "org"},
		wantStatus: http.StatusNotFound,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			resp := doExchange(t, s, token, tc.req)
			if tc.want == nil {
				if resp.StatusCode != tc.wantStatus {
					t.Fatalf("status = %d, want %d, body = %s", resp.StatusCode, tc.wantStatus, string(resp.Body))
				}
				return
			}
			if diff := cmp.Diff(tc.want, tokenOptions(t, resp)); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestExchangeAppPin(t *testing.T) {
	atr := newGitHubClient(t, newFakeGitHub())
	token := newTestToken(t)

	for _, tc := range []struct {
		name       string
		require    bool
		identity   string
		wantStatus int
	}{
		{name: "pinned to this app", identity: "pinned", wantStatus: http.StatusOK},
		{name: "pinned to another app", identity: "pinned-other", wantStatus: http.StatusForbidden},
		{name: "unpinned", identity: "unpinned", wantStatus: http.StatusOK},
		{name: "pin required, pinned to this app", require: true, identity: "pinned", wantStatus: http.StatusOK},
		{name: "pin required, unpinned", require: true, identity: "unpinned", wantStatus: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := New(atr, Config{
				Domain:        "octosts",
				OrgPolicyRepo: "policy-repo",
				PolicyDir:     "policies",
				AppID:         1234,
				AppSlug:       "octo-sts-test",
				RequireAppPin: tc.require,
			})
			if err != nil {
				t.Fatalf("New() = %v", err)
			}
			resp := doExchange(t, s, token, ExchangeRequest{Identity: tc.identity, Scope: "org"})
			if resp.StatusCode != tc.wantStatus {
				t.Errorf("status = %d, want %d, body = %s", resp.StatusCode, tc.wantStatus, string(resp.Body))
			}
		})
	}
}

func TestExchangeRejectsBeforePolicyLookup(t *testing.T) {
	atr := newGitHubClient(t, newFakeGitHub())
	token := newTestToken(t)

	for _, tc := range []struct {
		name    string
		allowed []string
		req     ExchangeRequest
	}{
		{name: "nested identity", req: ExchangeRequest{Identity: "sub/foo", Scope: "org/repo"}},
		{name: "traversal identity", req: ExchangeRequest{Identity: "..", Scope: "org/repo"}},
		{name: "issuer not allowed", allowed: []string{"https://issuer.example.com"}, req: ExchangeRequest{Identity: "foo", Scope: "org/repo"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := New(atr, Config{Domain: "octosts", AllowedIssuers: tc.allowed})
			if err != nil {
				t.Fatalf("New() = %v", err)
			}
			resp := doExchange(t, s, token, tc.req)
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d, want %d, body = %s", resp.StatusCode, http.StatusBadRequest, string(resp.Body))
			}
		})
	}
}

func TestCheckAppPin(t *testing.T) {
	s := &STS{appID: 1234, appSlug: "release-bot-prod"}
	strict := &STS{appID: 1234, appSlug: "release-bot-prod", requireAppPin: true}

	for _, tc := range []struct {
		name    string
		sts     *STS
		tp      octosts.TrustPolicy
		wantErr bool
	}{
		{name: "app slug", sts: s, tp: octosts.TrustPolicy{App: "release-bot-prod"}},
		{name: "app id", sts: s, tp: octosts.TrustPolicy{App: "1234"}},
		{name: "other app", sts: s, tp: octosts.TrustPolicy{App: "release-bot"}, wantErr: true},
		{name: "pattern match", sts: s, tp: octosts.TrustPolicy{AppPattern: "release-bot-[a-z]+"}},
		{name: "pattern is anchored", sts: s, tp: octosts.TrustPolicy{AppPattern: "release-bot"}, wantErr: true},
		{name: "pattern alternation is anchored", sts: s, tp: octosts.TrustPolicy{AppPattern: "other|release-bot"}, wantErr: true},
		{name: "unpinned", sts: s, tp: octosts.TrustPolicy{}},
		{name: "unpinned, pin required", sts: strict, tp: octosts.TrustPolicy{}, wantErr: true},
		{name: "pinned, pin required", sts: strict, tp: octosts.TrustPolicy{App: "release-bot-prod"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.sts.checkAppPin(&tc.tp); (err != nil) != tc.wantErr {
				t.Errorf("checkAppPin() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestConfigFromEnv(t *testing.T) {
	atr := newGitHubClient(t, newFakeGitHub())
	base := &envConfig.EnvConfig{AllowedIssuers: []string{testIssuer}}
	app := &envConfig.EnvConfigApp{Domain: "octosts", OrgPolicyRepo: "policy-repo", PolicyDir: "policies"}

	t.Setenv(EnvRequireAppPin, "true")
	got, err := ConfigFromEnv(slogtest.Context(t), atr, 1234, base, app)
	if err != nil {
		t.Fatalf("ConfigFromEnv() = %v", err)
	}
	want := Config{
		Domain:         "octosts",
		OrgPolicyRepo:  "policy-repo",
		PolicyDir:      "policies",
		AllowedIssuers: []string{testIssuer},
		AppID:          1234,
		AppSlug:        "octo-sts-test",
		RequireAppPin:  true,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Error(diff)
	}

	t.Setenv(EnvRequireAppPin, "yes please")
	if _, err := ConfigFromEnv(slogtest.Context(t), atr, 1234, base, app); err == nil {
		t.Error("ConfigFromEnv() with an invalid bool succeeded, want error")
	}
}
