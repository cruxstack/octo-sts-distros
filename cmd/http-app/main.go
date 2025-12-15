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

	"github.com/cruxstack/github-app-setup-go/configwait"
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

	allowedPaths := []string{"/healthz"}
	installerEnabled := configstore.InstallerEnabled()
	if installerEnabled {
		allowedPaths = append(allowedPaths, "/setup", "/callback", "/")
	}

	gate := configwait.NewReadyGate(nil, allowedPaths)
	mux := http.NewServeMux()
	webhook := &webhookHandler{}

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

	mux.Handle("/webhook", webhook)

	// Enable installer (doesn't require GitHub App config)
	if installerEnabled {
		store, err := configstore.NewFromEnv()
		if err != nil {
			log.Errorf("failed to create config store: %v", err)
			os.Exit(1)
		}

		installerCfg := installer.NewOctoSTSConfig(store)

		installerHandler, err := installer.New(installerCfg)
		if err != nil {
			log.Errorf("failed to create installer handler: %v", err)
			os.Exit(1)
		}

		mux.Handle("/setup", installerHandler)
		mux.Handle("/setup/", installerHandler)
		mux.Handle("/callback", installerHandler) // GitHub redirects here after app creation
		mux.Handle("/", installerHandler)

		log.Infof("[config] installer enabled: visit /setup to create GitHub App")
	}

	gate.SetHandler(mux)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		ReadHeaderTimeout: shared.DefaultReadHeaderTimeout,
		Handler:           gate,
	}

	log.Infof("Starting HTTP server on port %d (waiting for configuration...)", port)

	// Start server (returns 503 until ready)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Errorf("server error: %v", err)
			os.Exit(1)
		}
	}()

	// loadConfig loads configuration and creates the app handler (supports reload).
	loadConfig := func(ctx context.Context) error {
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
		gate.SetReady()

		return nil
	}

	// Set up reloader BEFORE starting wait loop so installer can trigger reload
	reloader := configwait.NewReloader(ctx, gate, loadConfig)
	configwait.SetGlobalReloader(reloader)
	reloader.Start()
	log.Infof("Configuration reloader started (send SIGHUP to reload)")

	// Load config in background with retries
	go func() {
		waitCfg := configwait.NewConfigFromEnv()

		// Track reload count to detect installer triggers
		initialReloadCount := configwait.GetReloadCount()

		err := configwait.Wait(ctx, waitCfg, func(ctx context.Context) error {
			// Check if a reload was triggered (e.g., by installer saving credentials)
			if configwait.GetReloadCount() > initialReloadCount {
				log.Infof("[configwait] reload triggered, retrying immediately")
				initialReloadCount = configwait.GetReloadCount()
			}
			return loadConfig(ctx)
		})

		if err != nil {
			log.Errorf("failed to load configuration after retries: %v", err)
			os.Exit(1)
		}

		log.Infof("Configuration loaded, service is ready")
	}()

	<-ctx.Done()
	log.Infof("Shutting down server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shared.DefaultShutdownTimeout)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Errorf("server shutdown error: %v", err)
		os.Exit(1)
	}
}
