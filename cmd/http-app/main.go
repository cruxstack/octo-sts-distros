// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"

	"github.com/chainguard-dev/clog"

	"github.com/cruxstack/github-app-setup-go/ghappsetup"
	"github.com/cruxstack/octo-sts-distros/internal/app"
	"github.com/cruxstack/octo-sts-distros/internal/configstore"
	"github.com/cruxstack/octo-sts-distros/internal/installer"
	"github.com/cruxstack/octo-sts-distros/internal/shared"
	envConfig "github.com/octo-sts/app/pkg/envconfig"
	"github.com/octo-sts/app/pkg/ghtransport"
)

// webhookHandler wraps an atomic pointer to the current app handler.
// This allows hot-swapping the handler when configuration is reloaded.
type webhookHandler struct {
	handler atomic.Pointer[http.Handler]
}

func (h *webhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handler := h.handler.Load()
	if handler == nil || *handler == nil {
		http.Error(w, "service not configured", http.StatusServiceUnavailable)
		return
	}
	(*handler).ServeHTTP(w, r)
}

func (h *webhookHandler) SetHandler(handler http.Handler) {
	h.handler.Store(&handler)
}

func main() {
	shared.SetupEnvMapping()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	ctx = clog.WithLogger(ctx, clog.New(shared.NewSlogHandler()))
	log := clog.FromContext(ctx)

	port := shared.DefaultPort
	if p := os.Getenv("PORT"); p != "" {
		fmt.Sscanf(p, "%d", &port)
	}

	// Build allowed paths for the ready gate
	allowedPaths := []string{"/healthz"}
	installerEnabled := configstore.InstallerEnabled()
	if installerEnabled {
		allowedPaths = append(allowedPaths, "/setup", "/setup/", "/callback", "/")
	}

	// Create webhook handler (will be configured after config loads)
	webhook := &webhookHandler{}

	// Create runtime with unified lifecycle management
	runtime, err := ghappsetup.NewRuntime(ghappsetup.Config{
		LoadFunc: func(ctx context.Context) error {
			return loadConfig(ctx, webhook)
		},
		AllowedPaths: allowedPaths,
	})
	if err != nil {
		log.Errorf("failed to create runtime: %v", err)
		os.Exit(1)
	}

	// Set up routes
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", runtime.HealthHandler())
	mux.Handle("/webhook", webhook)

	// Enable installer (doesn't require GitHub App config)
	if installerEnabled {
		store, err := configstore.NewFromEnv()
		if err != nil {
			log.Errorf("failed to create config store: %v", err)
			os.Exit(1)
		}

		installerCfg := installer.NewOctoSTSConfig(store)
		// Wire the runtime's reload callback into the installer
		installerCfg.OnCredentialsSaved = installer.WrapOnCredentialsSaved(installerCfg.OnCredentialsSaved, runtime.ReloadCallback())

		installerHandler, err := installer.New(installerCfg)
		if err != nil {
			log.Errorf("failed to create installer handler: %v", err)
			os.Exit(1)
		}

		mux.Handle("/setup", installerHandler)
		mux.Handle("/setup/", installerHandler)
		mux.Handle("/callback", installerHandler)
		mux.Handle("/", installerHandler)

		log.Infof("[config] installer enabled: visit /setup to create GitHub App")
	}

	// Start HTTP server with ReadyGate middleware
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		ReadHeaderTimeout: shared.DefaultReadHeaderTimeout,
		Handler:           runtime.Handler(mux),
	}

	log.Infof("Starting HTTP server on port %d (waiting for configuration...)", port)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Errorf("server error: %v", err)
			os.Exit(1)
		}
	}()

	// Block until config loads
	if err := runtime.Start(ctx); err != nil {
		log.Errorf("failed to load configuration: %v", err)
		os.Exit(1)
	}
	log.Infof("Configuration loaded, service is ready")

	// Listen for SIGHUP reloads in background
	go runtime.ListenForReloads(ctx)

	<-ctx.Done()
	log.Infof("Shutting down server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shared.DefaultShutdownTimeout)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Errorf("server shutdown error: %v", err)
		os.Exit(1)
	}
}

// loadConfig loads configuration and creates the app handler (supports reload).
func loadConfig(ctx context.Context, webhook *webhookHandler) error {
	// Re-run env mapping for hot-reload support
	shared.SetupEnvMapping()

	baseCfg, err := envConfig.BaseConfig()
	if err != nil {
		return fmt.Errorf("base config: %w", err)
	}

	webhookConfig, err := envConfig.WebhookConfig()
	if err != nil {
		return fmt.Errorf("webhook config: %w", err)
	}

	atr, err := ghtransport.New(ctx, baseCfg, nil)
	if err != nil {
		return fmt.Errorf("error creating GitHub App transport: %w", err)
	}

	var orgs []string
	for _, s := range strings.Split(webhookConfig.OrganizationFilter, ",") {
		if o := strings.TrimSpace(s); o != "" {
			orgs = append(orgs, o)
		}
	}

	appInstance, err := app.New(atr, app.Config{
		WebhookSecrets: [][]byte{[]byte(webhookConfig.WebhookSecret)},
		Organizations:  orgs,
	})
	if err != nil {
		return fmt.Errorf("failed to create app: %w", err)
	}

	webhook.SetHandler(appInstance)
	return nil
}
