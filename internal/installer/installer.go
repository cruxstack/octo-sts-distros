// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

// Package installer provides a web-based installer for creating GitHub Apps using
// the GitHub App Manifest flow. The installer pre-configures all required
// permissions for Octo-STS and automatically saves credentials to configurable
// storage backends.
//
// See https://docs.github.com/en/apps/sharing-github-apps/registering-a-github-app-from-a-manifest
// for details on the GitHub App Manifest flow.
package installer

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/chainguard-dev/clog"

	"github.com/cruxstack/octo-sts-distros/internal/configstore"
	"github.com/cruxstack/octo-sts-distros/internal/configwait"
	"github.com/cruxstack/octo-sts-distros/internal/shared"
)

//go:embed templates/*
var templateFS embed.FS

var indexTemplate = template.Must(template.ParseFS(templateFS, "templates/index.html"))
var successTemplate = template.Must(template.ParseFS(templateFS, "templates/success.html"))

const (
	// HTTP client timeout for GitHub API calls
	httpClientTimeout = 30 * time.Second

	// Environment variable names for installer configuration
	EnvGitHubURL = "GITHUB_URL"
	EnvGitHubOrg = "GITHUB_ORG"

	disableSetupPath = "/setup/disable"
)

// Config holds the installer configuration.
type Config struct {
	// Store is the storage backend for saving app credentials.
	Store configstore.Store

	// GitHubURL is the base URL for GitHub (default: "https://github.com").
	// Use this for GitHub Enterprise Server deployments.
	GitHubURL string

	// GitHubOrg is the organization to create the app under.
	// If empty, the app will be created under the user's personal account.
	GitHubOrg string

	// RedirectURL is the base URL (scheme + host, e.g., "https://example.com") for OAuth redirects.
	// The path /callback will be appended automatically.
	// If empty, it will be auto-derived from the request.
	RedirectURL string

	// WebhookURL is the URL for GitHub webhook events.
	// If empty, it will be auto-derived from the request.
	WebhookURL string
}

// NewConfigFromEnv creates a Config from environment variables.
// It does NOT create the store - that must be passed separately.
func NewConfigFromEnv() Config {
	return Config{
		GitHubURL: getEnvDefault(EnvGitHubURL, "https://github.com"),
		GitHubOrg: os.Getenv(EnvGitHubOrg),
	}
}

// Handler handles the GitHub App manifest installation flow.
type Handler struct {
	config Config
}

type successTemplateData struct {
	AppID             int64
	AppSlug           string
	HTMLURL           string
	InstallURL        string
	DisableActionURL  string
	InstallerDisabled bool
}

// New creates a new installer Handler with the given configuration.
func New(cfg Config) (*Handler, error) {
	if cfg.Store == nil {
		return nil, fmt.Errorf("store is required")
	}
	if cfg.GitHubURL == "" {
		cfg.GitHubURL = "https://github.com"
	}
	return &Handler{config: cfg}, nil
}

// ServeHTTP implements http.Handler.
// Routes:
//   - GET /setup - main page with manifest form
//   - GET /callback - OAuth callback after app creation (GitHub redirects here)
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch {
	case (r.Method == http.MethodGet || r.Method == http.MethodHead) && (path == "/" || path == ""):
		h.handleRoot(w, r)
	case (r.Method == http.MethodGet || r.Method == http.MethodHead) && (path == "/setup" || path == "/setup/"):
		h.handleIndex(w, r)
	case (r.Method == http.MethodGet || r.Method == http.MethodHead) && path == "/callback":
		h.handleCallback(w, r)

	case r.Method == http.MethodPost && (path == disableSetupPath || path == disableSetupPath+"/"):
		h.handleDisable(w, r)
	default:
		http.NotFound(w, r)
	}
}

// handleIndex serves the main page with the manifest form.
func (h *Handler) handleRoot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := clog.FromContext(ctx)

	status, err := h.config.Store.Status(ctx)
	if err != nil {
		log.Errorf("[installer] failed to read installer status: %v", err)
		http.Error(w, "Failed to load installer status", http.StatusInternalServerError)
		return
	}

	if status != nil && status.InstallerDisabled {
		http.NotFound(w, r)
		return
	}

	http.Redirect(w, r, "/setup", http.StatusFound)
}

// handleIndex serves the main page with the manifest form.
func (h *Handler) handleIndex(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := clog.FromContext(ctx)

	status, err := h.config.Store.Status(ctx)

	if err != nil {
		log.Errorf("[installer] failed to read installer status: %v", err)
		http.Error(w, "Failed to load installer status", http.StatusInternalServerError)
		return
	}
	if status != nil && status.Registered {
		data := h.successDataFromStatus(status)
		h.renderSuccess(w, r, data)
		return
	}

	// Determine redirect URL: use configured value, or derive from request
	// Note: This is the base URL (without path). The manifest will append /setup/callback
	redirectURL := h.config.RedirectURL

	if redirectURL == "" {
		redirectURL = getBaseURL(ctx, r)
		log.Infof("[installer] auto-detected redirect url: url=%s host=%s forwarded_host=%s",
			redirectURL, r.Host, r.Header.Get("X-Forwarded-Host"))
	}

	// Determine webhook URL: use configured value, form input, or derive from request
	webhookURL := h.config.WebhookURL
	if webhookURL == "" {
		webhookURL = r.FormValue("webhook_url")
		if webhookURL == "" {
			// Auto-derive webhook URL from request host
			webhookURL = getBaseURL(ctx, r) + "/webhook"
			log.Infof("[installer] auto-detected webhook url: url=%s", webhookURL)
		}
	}

	manifest := buildManifest(redirectURL, webhookURL)
	log.Infof("[installer] manifest redirect_url: %s", manifest.RedirectURL)
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		http.Error(w, "Failed to generate manifest", http.StatusInternalServerError)
		return
	}

	// Build form action URL: org-scoped if GitHubOrg is set, otherwise user-scoped
	var formActionURL string
	if h.config.GitHubOrg != "" {
		formActionURL = fmt.Sprintf("%s/organizations/%s/settings/apps/new",
			h.config.GitHubURL, h.config.GitHubOrg)
	} else {
		formActionURL = fmt.Sprintf("%s/settings/apps/new", h.config.GitHubURL)
	}

	data := struct {
		GitHubURL     string
		GitHubOrg     string
		FormActionURL string
		ManifestJSON  template.JS
		WebhookURL    string
		NeedsWebhook  bool
	}{
		GitHubURL:     h.config.GitHubURL,
		GitHubOrg:     h.config.GitHubOrg,
		FormActionURL: formActionURL,
		ManifestJSON:  template.JS(manifestJSON),
		WebhookURL:    webhookURL,
		NeedsWebhook:  h.config.WebhookURL == "",
	}

	var buf bytes.Buffer
	if err := indexTemplate.Execute(&buf, data); err != nil {
		log.Errorf("[installer] failed to render index template: %v", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if _, err := buf.WriteTo(w); err != nil {
		log.Errorf("[installer] failed to write response: %v", err)
	}
}

// handleCallback handles the GitHub redirect after app creation.
func (h *Handler) handleCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := clog.FromContext(ctx)

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Missing code parameter", http.StatusBadRequest)
		return
	}

	// Extract STS domain from cookie (set before form submission on the index page)
	var stsDomain string
	if cookie, err := r.Cookie("sts_domain"); err == nil {
		stsDomain = cookie.Value
		// Clear the cookie after reading
		http.SetCookie(w, &http.Cookie{
			Name:   "sts_domain",
			Value:  "",
			Path:   "/",
			MaxAge: -1,
		})
	}

	// Exchange the code for credentials
	creds, err := h.exchangeCode(ctx, code)
	if err != nil {
		log.Errorf("[installer] failed to exchange code: %v", err)
		http.Error(w, "Failed to exchange code", http.StatusInternalServerError)
		return
	}

	// Set the STS domain from the cookie
	creds.STSDomain = stsDomain

	// Save credentials using the store
	if err := h.config.Store.Save(ctx, creds); err != nil {
		log.Errorf("[installer] failed to save credentials: %v", err)
		http.Error(w, "Failed to save credentials", http.StatusInternalServerError)
		return
	}

	log.Infof("[installer] successfully created github app: slug=%s app_id=%d", creds.AppSlug, creds.AppID)

	// Trigger configuration reload so services pick up the new credentials
	log.Infof("[installer] triggering configuration reload")
	configwait.TriggerReload()

	data := h.successDataFromCreds(creds)
	h.renderSuccess(w, r, data)
}

// exchangeCode calls GitHub API to exchange the temporary code for app credentials.
func (h *Handler) exchangeCode(ctx context.Context, code string) (*configstore.AppCredentials, error) {
	url := fmt.Sprintf("%s/api/v3/app-manifests/%s/conversions", h.config.GitHubURL, code)

	// For github.com, the API is at api.github.com
	if h.config.GitHubURL == "https://github.com" {
		url = fmt.Sprintf("https://api.github.com/app-manifests/%s/conversions", code)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{
		Timeout: httpClientTimeout,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call GitHub API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, string(body))
	}

	var creds configstore.AppCredentials
	if err := json.Unmarshal(body, &creds); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &creds, nil
}

func (h *Handler) handleDisable(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := clog.FromContext(ctx)

	// Require app to be registered before allowing disable
	status, err := h.config.Store.Status(ctx)
	if err != nil {
		log.Errorf("[installer] failed to check status: %v", err)
		http.Error(w, "Failed to check installer status", http.StatusInternalServerError)
		return
	}
	if status == nil || !status.Registered {
		http.Error(w, "Cannot disable installer before app is registered", http.StatusBadRequest)
		return
	}

	if err := h.config.Store.DisableInstaller(ctx); err != nil {
		log.Errorf("[installer] failed to disable installer: %v", err)
		http.Error(w, "Failed to disable installer", http.StatusInternalServerError)
		return
	}

	log.Infof("[installer] installer disabled via setup UI")
	http.Redirect(w, r, "/healthz", http.StatusSeeOther)
}

func (h *Handler) successDataFromCreds(creds *configstore.AppCredentials) successTemplateData {
	data := successTemplateData{
		AppID:            creds.AppID,
		AppSlug:          creds.AppSlug,
		HTMLURL:          creds.HTMLURL,
		DisableActionURL: disableSetupPath,
	}
	data.InstallURL = h.installURLFor(creds.AppSlug, creds.HTMLURL)
	return data
}

func (h *Handler) successDataFromStatus(status *configstore.InstallerStatus) successTemplateData {
	if status == nil {
		return successTemplateData{}
	}
	data := successTemplateData{
		AppID:             status.AppID,
		AppSlug:           status.AppSlug,
		HTMLURL:           status.HTMLURL,
		InstallerDisabled: status.InstallerDisabled,
		DisableActionURL:  disableSetupPath,
	}
	data.InstallURL = h.installURLFor(status.AppSlug, status.HTMLURL)
	return data
}

func (h *Handler) installURLFor(slug, htmlURL string) string {
	if slug != "" {
		githubURL := h.config.GitHubURL
		if githubURL == "" {
			githubURL = "https://github.com"
		}
		return fmt.Sprintf("%s/apps/%s/installations/new", githubURL, slug)
	}
	if htmlURL != "" {
		trimmed := strings.TrimRight(htmlURL, "/")
		return trimmed + "/installations/new"
	}
	return ""
}

func (h *Handler) renderSuccess(w http.ResponseWriter, r *http.Request, data successTemplateData) {
	ctx := r.Context()
	log := clog.FromContext(ctx)

	var buf bytes.Buffer
	if err := successTemplate.Execute(&buf, data); err != nil {
		log.Errorf("[installer] failed to render success template: %v", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if _, err := buf.WriteTo(w); err != nil {
		log.Errorf("[installer] failed to write response: %v", err)
	}
}

// getBaseURL derives the base URL from the request headers.
// It uses X-Forwarded-Proto and X-Forwarded-Host if present (common with reverse proxies),
// otherwise falls back to the Host header with https for non-localhost hosts.
func getBaseURL(ctx context.Context, r *http.Request) string {
	log := clog.FromContext(ctx)

	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}

	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		// Default to https, except for localhost
		scheme = "https"
		if host == "localhost" || strings.HasPrefix(host, "localhost:") ||
			host == "127.0.0.1" || strings.HasPrefix(host, "127.0.0.1:") {
			scheme = "http"
		}
	} else if scheme == "http" && !strings.HasPrefix(host, "localhost") && !strings.HasPrefix(host, "127.0.0.1") {
		// Override http to https for non-localhost hosts (e.g., behind ngrok or other HTTPS proxies)
		// This handles cases where an internal proxy reports http but external access is via https
		scheme = "https"
	}

	baseURL := scheme + "://" + host
	log.Debugf("[installer] getBaseURL: scheme=%s host=%s r.Host=%s X-Forwarded-Proto=%s X-Forwarded-Host=%s result=%s",
		scheme, host, r.Host, r.Header.Get("X-Forwarded-Proto"), r.Header.Get("X-Forwarded-Host"), baseURL)
	return baseURL
}

// getEnvDefault is an alias to the shared implementation.
var getEnvDefault = shared.GetEnvDefault
