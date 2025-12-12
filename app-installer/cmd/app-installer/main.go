// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

// Package main provides a web-based installer for creating GitHub Apps using
// the GitHub App Manifest flow. The installer pre-configures all required
// permissions for Octo-STS and automatically saves credentials to local
// storage.
//
// The installer is designed for local development and proof-of-concept usage.
// For production deployments, use proper secret management solutions.
//
// See https://docs.github.com/en/apps/sharing-github-apps/registering-a-github-app-from-a-manifest
// for details on the GitHub App Manifest flow.
package main

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
	"os/signal"
	"strings"
	"time"

	"github.com/cruxstack/octo-sts-distros/app-installer/pkg/appstore"
)

//go:embed templates/*
var templateFS embed.FS

var indexTemplate = template.Must(template.ParseFS(templateFS, "templates/index.html"))
var successTemplate = template.Must(template.ParseFS(templateFS, "templates/success.html"))

const (
	// HTTP response constants
	httpOKResponse = "ok"

	// Server timeout constants
	serverReadTimeout     = 10 * time.Second
	serverShutdownTimeout = 5 * time.Second

	// HTTP client timeout
	httpClientTimeout = 30 * time.Second
)

// Config holds the application configuration from environment variables.
type Config struct {
	Port                  string
	RedirectURL           string
	WebhookURL            string
	StorageDir            string
	StorageMode           string
	GitHubURL             string
	GitHubOrg             string
	AWSSSMParameterPrefix string
	AWSSSMKMSKeyID        string
	AWSSSMTags            string
}

func loadConfig() *Config {
	cfg := &Config{
		Port:                  getEnv("PORT", "8080"),
		RedirectURL:           getEnv("REDIRECT_URL", ""),
		WebhookURL:            getEnv("WEBHOOK_URL", ""),
		StorageDir:            getEnv("STORAGE_DIR", "./.env"),
		StorageMode:           getEnv("STORAGE_MODE", "envfile"),
		GitHubURL:             getEnv("GITHUB_URL", "https://github.com"),
		GitHubOrg:             os.Getenv("GITHUB_ORG"),
		AWSSSMParameterPrefix: os.Getenv("AWS_SSM_PARAMETER_PREFIX"),
		AWSSSMKMSKeyID:        os.Getenv("AWS_SSM_KMS_KEY_ID"),
		AWSSSMTags:            os.Getenv("AWS_SSM_TAGS"),
	}
	return cfg
}

// getBaseURL derives the base URL from the request headers.
// It uses X-Forwarded-Proto and X-Forwarded-Host if present (common with reverse proxies),
// otherwise falls back to the Host header with https for non-localhost hosts.
func getBaseURL(r *http.Request) string {
	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "https"
		// Use http for localhost
		if host := r.Host; host == "localhost" || strings.HasPrefix(host, "localhost:") ||
			host == "127.0.0.1" || strings.HasPrefix(host, "127.0.0.1:") {
			scheme = "http"
		}
	}

	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}

	return scheme + "://" + host
}

func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

// Manifest represents the GitHub App manifest structure.
type Manifest struct {
	Name           string            `json:"name,omitempty"`
	URL            string            `json:"url"`
	HookAttributes HookAttributes    `json:"hook_attributes"`
	RedirectURL    string            `json:"redirect_url"`
	Public         bool              `json:"public"`
	DefaultPerms   map[string]string `json:"default_permissions"`
	DefaultEvents  []string          `json:"default_events"`
}

// HookAttributes configures the webhook for the GitHub App.
type HookAttributes struct {
	URL    string `json:"url"`
	Active bool   `json:"active"`
}

// buildManifest creates the GitHub App manifest with octo-sts permissions.
func buildManifest(redirectURL, webhookURL string) *Manifest {
	return &Manifest{
		URL: "https://github.com/octo-sts/app",
		HookAttributes: HookAttributes{
			URL:    webhookURL,
			Active: webhookURL != "",
		},
		RedirectURL: redirectURL + "/callback",
		Public:      false,
		DefaultPerms: map[string]string{
			// Repository permissions
			"actions":             "write",
			"administration":      "read",
			"checks":              "write",
			"security_events":     "write", // code_scanning_alerts
			"statuses":            "write",
			"contents":            "write",
			"deployments":         "write",
			"discussions":         "write",
			"environments":        "write",
			"issues":              "write",
			"packages":            "write",
			"pages":               "write",
			"repository_projects": "write",
			"pull_requests":       "write",
			"workflows":           "write",
			// Organization permissions
			"organization_administration": "write",
			"organization_events":         "read",
			"members":                     "write",
			"organization_projects":       "write",
		},
		DefaultEvents: []string{
			"pull_request",
		},
	}
}

// Server handles the manifest installation flow.
type Server struct {
	config *Config
	store  appstore.Store
}

// NewServer creates a new Server with the given configuration.
func NewServer(cfg *Config, store appstore.Store) *Server {
	return &Server{
		config: cfg,
		store:  store,
	}
}

// indexHandler serves the main page with the manifest form.
func (s *Server) indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// Determine redirect URL: use configured value, or derive from request
	redirectURL := s.config.RedirectURL
	if redirectURL == "" {
		redirectURL = getBaseURL(r)
		log.Printf("[oauth] auto-detected redirect url: url=%s host=%s forwarded_host=%s", redirectURL, r.Host, r.Header.Get("X-Forwarded-Host"))
	}

	// Determine webhook URL: use configured value, form input, or derive from request
	webhookURL := s.config.WebhookURL
	if webhookURL == "" {
		webhookURL = r.FormValue("webhook_url")
		if webhookURL == "" {
			// Auto-derive webhook URL from request host
			webhookURL = getBaseURL(r) + "/webhook"
			log.Printf("[oauth] auto-detected webhook url: url=%s", webhookURL)
		}
	}

	manifest := buildManifest(redirectURL, webhookURL)
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		http.Error(w, "Failed to generate manifest", http.StatusInternalServerError)
		return
	}

	// Build form action URL: org-scoped if GITHUB_ORG is set, otherwise user-scoped
	var formActionURL string
	if s.config.GitHubOrg != "" {
		formActionURL = fmt.Sprintf("%s/organizations/%s/settings/apps/new", s.config.GitHubURL, s.config.GitHubOrg)
	} else {
		formActionURL = fmt.Sprintf("%s/settings/apps/new", s.config.GitHubURL)
	}

	data := struct {
		GitHubURL     string
		GitHubOrg     string
		FormActionURL string
		ManifestJSON  template.JS
		WebhookURL    string
		NeedsWebhook  bool
	}{
		GitHubURL:     s.config.GitHubURL,
		GitHubOrg:     s.config.GitHubOrg,
		FormActionURL: formActionURL,
		ManifestJSON:  template.JS(manifestJSON),
		WebhookURL:    webhookURL,
		NeedsWebhook:  s.config.WebhookURL == "",
	}

	var buf bytes.Buffer
	if err := indexTemplate.Execute(&buf, data); err != nil {
		log.Printf("[server] failed to render index template: %v", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	buf.WriteTo(w)
}

// callbackHandler handles the GitHub redirect after app creation.
func (s *Server) callbackHandler(w http.ResponseWriter, r *http.Request) {
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
	creds, err := s.exchangeCode(r.Context(), code)
	if err != nil {
		log.Printf("[oauth] failed to exchange code: %v", err)
		http.Error(w, "Failed to exchange code", http.StatusInternalServerError)
		return
	}

	// Set the STS domain from the cookie
	creds.STSDomain = stsDomain

	// Save credentials using the store
	if err := s.store.Save(r.Context(), creds); err != nil {
		log.Printf("[storage] failed to save credentials: %v", err)
		http.Error(w, "Failed to save credentials", http.StatusInternalServerError)
		return
	}

	log.Printf("[oauth] successfully created github app: slug=%s app_id=%d", creds.AppSlug, creds.AppID)

	// Build installation URL: https://github.com/apps/{slug}/installations/new
	installURL := fmt.Sprintf("%s/apps/%s/installations/new", s.config.GitHubURL, creds.AppSlug)

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
		log.Printf("[server] failed to render success template: %v", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	buf.WriteTo(w)
}

// exchangeCode calls GitHub API to exchange the temporary code for app credentials.
func (s *Server) exchangeCode(ctx context.Context, code string) (*appstore.AppCredentials, error) {
	url := fmt.Sprintf("%s/api/v3/app-manifests/%s/conversions", s.config.GitHubURL, code)

	// For github.com, the API is at api.github.com
	if s.config.GitHubURL == "https://github.com" {
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

	var creds appstore.AppCredentials
	if err := json.Unmarshal(body, &creds); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &creds, nil
}

// healthHandler returns a simple health check response.
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// webhookHandler handles incoming GitHub webhook events.
func (s *Server) webhookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("[webhook] failed to read request body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer func() {
		if err := r.Body.Close(); err != nil {
			log.Printf("[webhook] failed to close request body: %v", err)
		}
	}()

	// Validate webhook signature if we have a secret
	signature := r.Header.Get("X-Hub-Signature-256")
	if signature != "" {
		// Load webhook secret from store (we'll need to enhance the store interface for this)
		// For now, we'll skip validation if we don't have access to the secret
		// In production, you'd want to validate this properly
		log.Printf("[webhook] received webhook with signature: signature=%s", signature)
	}

	// Get event type from header
	eventType := r.Header.Get("X-GitHub-Event")
	deliveryID := r.Header.Get("X-GitHub-Delivery")

	log.Printf("[webhook] received github webhook: event=%s delivery=%s", eventType, deliveryID)

	// Handle different event types
	switch eventType {
	case "ping":
		// Parse ping event
		var pingEvent struct {
			Zen    string `json:"zen"`
			HookID int64  `json:"hook_id"`
			Hook   struct {
				Type   string `json:"type"`
				ID     int64  `json:"id"`
				Active bool   `json:"active"`
			} `json:"hook"`
		}

		if err := json.Unmarshal(body, &pingEvent); err != nil {
			log.Printf("[webhook] failed to parse ping event: %v", err)
			http.Error(w, "Failed to parse ping event", http.StatusBadRequest)
			return
		}

		log.Printf("[webhook] received ping event: hook_id=%d zen=%s", pingEvent.HookID, pingEvent.Zen)

		// Respond with success
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"message": "pong",
		})
		return

	default:
		log.Printf("[webhook] received unsupported event: event=%s", eventType)
		// Return 200 for unsupported events to avoid GitHub retrying
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(httpOKResponse))
		return
	}
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	cfg := loadConfig()

	// Select store based on storage mode
	var store appstore.Store
	switch cfg.StorageMode {
	case "files":
		store = appstore.NewLocalFileStore(cfg.StorageDir)
		log.Printf("[config] using file-based storage: dir=%s", cfg.StorageDir)
	case "envfile":
		store = appstore.NewLocalEnvFileStore(cfg.StorageDir)
		log.Printf("[config] using env file storage: path=%s", cfg.StorageDir)
	case "aws-ssm":
		if cfg.AWSSSMParameterPrefix == "" {
			log.Fatal("[config] AWS_SSM_PARAMETER_PREFIX is required when using aws-ssm storage mode")
		}

		// Build store options
		var opts []appstore.SSMStoreOption
		if cfg.AWSSSMKMSKeyID != "" {
			opts = append(opts, appstore.WithKMSKey(cfg.AWSSSMKMSKeyID))
			log.Printf("[config] using custom KMS key: key_id=%s", cfg.AWSSSMKMSKeyID)
		}

		// Parse tags from JSON if provided
		if cfg.AWSSSMTags != "" {
			var tags map[string]string
			if err := json.Unmarshal([]byte(cfg.AWSSSMTags), &tags); err != nil {
				log.Fatalf("[config] failed to parse AWS_SSM_TAGS as JSON: %v", err)
			}
			opts = append(opts, appstore.WithTags(tags))
			log.Printf("[config] using AWS tags: tags=%v", tags)
		}

		var err error
		store, err = appstore.NewAWSSSMStore(cfg.AWSSSMParameterPrefix, opts...)
		if err != nil {
			log.Fatalf("[config] failed to create AWS SSM store: %v", err)
		}
		log.Printf("[config] using AWS SSM Parameter Store: prefix=%s", cfg.AWSSSMParameterPrefix)
	default:
		log.Fatalf("Unknown STORAGE_MODE: %s (expected 'envfile', 'files', or 'aws-ssm')", cfg.StorageMode)
	}

	server := NewServer(cfg, store)

	mux := http.NewServeMux()
	mux.HandleFunc("/", server.indexHandler)
	mux.HandleFunc("/callback", server.callbackHandler)
	mux.HandleFunc("/webhook", server.webhookHandler)
	mux.HandleFunc("/health", server.healthHandler)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: serverReadTimeout,
	}

	go func() {
		log.Printf("[server] starting app installer: port=%s", cfg.Port)
		log.Printf("[server] visit %s to create your github app", cfg.RedirectURL)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("[server] shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), serverShutdownTimeout)
	defer shutdownCancel()
	srv.Shutdown(shutdownCtx)
}
