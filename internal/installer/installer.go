// Copyright 2025 Octo-STS
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
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cruxstack/octo-sts-distros/internal/configstore"
	"github.com/cruxstack/octo-sts-distros/internal/configwait"
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
	case r.Method == http.MethodGet && (path == "/setup" || path == "/setup/"):
		h.handleIndex(w, r)
	case r.Method == http.MethodGet && path == "/callback":
		h.handleCallback(w, r)
	default:
		http.NotFound(w, r)
	}
}

// handleIndex serves the main page with the manifest form.
func (h *Handler) handleIndex(w http.ResponseWriter, r *http.Request) {
	// Determine redirect URL: use configured value, or derive from request
	// Note: This is the base URL (without path). The manifest will append /setup/callback
	redirectURL := h.config.RedirectURL
	if redirectURL == "" {
		redirectURL = getBaseURL(r)
		log.Printf("[installer] auto-detected redirect url: url=%s host=%s forwarded_host=%s",
			redirectURL, r.Host, r.Header.Get("X-Forwarded-Host"))
	}

	// Determine webhook URL: use configured value, form input, or derive from request
	webhookURL := h.config.WebhookURL
	if webhookURL == "" {
		webhookURL = r.FormValue("webhook_url")
		if webhookURL == "" {
			// Auto-derive webhook URL from request host
			webhookURL = getBaseURL(r) + "/webhook"
			log.Printf("[installer] auto-detected webhook url: url=%s", webhookURL)
		}
	}

	manifest := buildManifest(redirectURL, webhookURL)
	log.Printf("[installer] manifest redirect_url: %s", manifest.RedirectURL)
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
		log.Printf("[installer] failed to render index template: %v", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	buf.WriteTo(w)
}

// handleCallback handles the GitHub redirect after app creation.
func (h *Handler) handleCallback(w http.ResponseWriter, r *http.Request) {
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
	creds, err := h.exchangeCode(r.Context(), code)
	if err != nil {
		log.Printf("[installer] failed to exchange code: %v", err)
		http.Error(w, "Failed to exchange code", http.StatusInternalServerError)
		return
	}

	// Set the STS domain from the cookie
	creds.STSDomain = stsDomain

	// Save credentials using the store
	if err := h.config.Store.Save(r.Context(), creds); err != nil {
		log.Printf("[installer] failed to save credentials: %v", err)
		http.Error(w, "Failed to save credentials", http.StatusInternalServerError)
		return
	}

	log.Printf("[installer] successfully created github app: slug=%s app_id=%d", creds.AppSlug, creds.AppID)

	// Trigger configuration reload so services pick up the new credentials
	log.Printf("[installer] triggering configuration reload")
	configwait.TriggerReload()

	// Build installation URL: https://github.com/apps/{slug}/installations/new
	installURL := fmt.Sprintf("%s/apps/%s/installations/new", h.config.GitHubURL, creds.AppSlug)

	// Render success page
	data := struct {
		AppID      int64
		AppSlug    string
		HTMLURL    string
		InstallURL string
	}{
		AppID:      creds.AppID,
		AppSlug:    creds.AppSlug,
		HTMLURL:    creds.HTMLURL,
		InstallURL: installURL,
	}

	var buf bytes.Buffer
	if err := successTemplate.Execute(&buf, data); err != nil {
		log.Printf("[installer] failed to render success template: %v", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	buf.WriteTo(w)
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

// getBaseURL derives the base URL from the request headers.
// It uses X-Forwarded-Proto and X-Forwarded-Host if present (common with reverse proxies),
// otherwise falls back to the Host header with https for non-localhost hosts.
func getBaseURL(r *http.Request) string {
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
	log.Printf("[installer] getBaseURL: scheme=%s host=%s r.Host=%s X-Forwarded-Proto=%s X-Forwarded-Host=%s result=%s",
		scheme, host, r.Host, r.Header.Get("X-Forwarded-Proto"), r.Header.Get("X-Forwarded-Host"), baseURL)
	return baseURL
}

func getEnvDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
