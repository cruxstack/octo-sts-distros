// Copyright 2025 Octo-STS
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"

	"github.com/chainguard-dev/clog"

	"github.com/cruxstack/octo-sts-distros/internal/app"
	"github.com/cruxstack/octo-sts-distros/internal/configstore"
	"github.com/cruxstack/octo-sts-distros/internal/configwait"
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
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	ctx = clog.WithLogger(ctx, clog.New(slog.Default().Handler()))
	log := clog.FromContext(ctx)

	// Get port early (doesn't depend on GitHub App config)
	port := shared.DefaultPort
	if p := os.Getenv("PORT"); p != "" {
		fmt.Sscanf(p, "%d", &port)
	}

	// Determine allowed paths based on installer status
	allowedPaths := []string{"/healthz"}
	installerEnabled := configstore.InstallerEnabled()
	if installerEnabled {
		allowedPaths = append(allowedPaths, "/setup", "/callback")
	}

	// Create ready gate
	gate := configwait.NewReadyGate(nil, allowedPaths)

	// Create HTTP mux
	mux := http.NewServeMux()

	// Create webhook handler wrapper for hot-swapping
	webhook := &webhookHandler{}

	// Register health check endpoint (always available)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if gate.IsReady() {
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte("ok")); err != nil {
				clog.FromContext(r.Context()).Errorf("failed to write health response: %v", err)
			}
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			if _, err := w.Write([]byte("not ready")); err != nil {
				clog.FromContext(r.Context()).Errorf("failed to write health response: %v", err)
			}
		}
	})

	// Register webhook handler (delegates to the atomic handler)
	mux.Handle("/webhook", webhook)

	// Conditionally enable installer if INSTALLER_ENABLED=true
	// The installer doesn't require GitHub App config, so it can start immediately
	if installerEnabled {
		store, err := configstore.NewFromEnv()
		if err != nil {
			log.Errorf("failed to create config store: %v", err)
			os.Exit(1)
		}

		installerCfg := installer.NewConfigFromEnv()
		installerCfg.Store = store

		installerHandler, err := installer.New(installerCfg)
		if err != nil {
			log.Errorf("failed to create installer handler: %v", err)
			os.Exit(1)
		}

		// Register installer routes
		mux.Handle("/setup", installerHandler)
		mux.Handle("/setup/", installerHandler)
		mux.Handle("/callback", installerHandler) // GitHub redirects here after app creation

		log.Infof("[config] installer enabled: visit /setup to create GitHub App")
	}

	// Set the mux as the gate's handler for allowed paths
	gate.SetHandler(mux)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		ReadHeaderTimeout: shared.DefaultReadHeaderTimeout,
		Handler:           gate,
	}

	log.Infof("Starting HTTP server on port %d (waiting for configuration...)", port)

	// Start server immediately (will return 503 for non-allowed paths until ready)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Errorf("server error: %v", err)
			os.Exit(1)
		}
	}()

	// loadConfig loads configuration and creates the app handler.
	// It can be called multiple times to reload configuration.
	loadConfig := func(ctx context.Context) error {
		baseCfg, err := envConfig.BaseConfig()
		if err != nil {
			return fmt.Errorf("base config: %w", err)
		}

		webhookConfig, err := envConfig.WebhookConfig()
		if err != nil {
			return fmt.Errorf("webhook config: %w", err)
		}

		// Create GitHub App transport
		atr, err := ghtransport.New(ctx, baseCfg, nil)
		if err != nil {
			return fmt.Errorf("error creating GitHub App transport: %w", err)
		}

		// Parse organization filter
		var orgs []string
		for _, s := range strings.Split(webhookConfig.OrganizationFilter, ",") {
			if o := strings.TrimSpace(s); o != "" {
				orgs = append(orgs, o)
			}
		}

		// Create App instance
		appInstance, err := app.New(atr, app.Config{
			WebhookSecrets: [][]byte{[]byte(webhookConfig.WebhookSecret)},
			Organizations:  orgs,
		})
		if err != nil {
			return fmt.Errorf("failed to create app: %w", err)
		}

		// Hot-swap the webhook handler
		webhook.SetHandler(appInstance)

		// Mark as ready (idempotent)
		gate.SetReady()

		return nil
	}

	// Load configuration with retries in background
	go func() {
		waitCfg := configwait.NewConfigFromEnv()

		err := configwait.Wait(ctx, waitCfg, func(ctx context.Context) error {
			return loadConfig(ctx)
		})

		if err != nil {
			log.Errorf("failed to load configuration after retries: %v", err)
			os.Exit(1)
		}

		log.Infof("Configuration loaded, service is ready")

		// Set up reloader for SIGHUP and programmatic triggers
		reloader := configwait.NewReloader(ctx, gate, loadConfig)
		configwait.SetGlobalReloader(reloader)
		reloader.Start()

		log.Infof("Configuration reloader started (send SIGHUP to reload)")
	}()

	// Wait for interrupt signal
	<-ctx.Done()
	log.Infof("Shutting down server...")

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shared.DefaultShutdownTimeout)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Errorf("server shutdown error: %v", err)
		os.Exit(1)
	}
}
