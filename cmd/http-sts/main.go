// Copyright 2025 Octo-STS
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"time"

	"github.com/chainguard-dev/clog"

	"github.com/cruxstack/octo-sts-distros/internal/configwait"
	"github.com/cruxstack/octo-sts-distros/internal/sts"
	envConfig "github.com/octo-sts/app/pkg/envconfig"
	"github.com/octo-sts/app/pkg/ghtransport"
)

// stsHandler wraps an atomic pointer to the current STS instance.
// This allows hot-swapping the handler when configuration is reloaded.
type stsHandler struct {
	sts atomic.Pointer[sts.STS]
}

func (h *stsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	stsInstance := h.sts.Load()
	if stsInstance == nil {
		http.Error(w, "service not configured", http.StatusServiceUnavailable)
		return
	}

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Convert http.Request headers to map[string]string
	headers := make(map[string]string)
	for k := range r.Header {
		headers[strings.ToLower(k)] = r.Header.Get(k)
	}

	// Create sts.Request
	req := sts.Request{
		Type:    sts.RequestTypeHTTP,
		Method:  r.Method,
		Path:    r.URL.Path,
		Headers: headers,
		Body:    body,
	}

	// Handle request
	resp := stsInstance.HandleRequest(r.Context(), req)

	// Write response headers
	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}

	// Write status code and body
	w.WriteHeader(resp.StatusCode)
	if resp.Body != nil {
		w.Write(resp.Body)
	}
}

func (h *stsHandler) SetSTS(s *sts.STS) {
	h.sts.Store(s)
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	ctx = clog.WithLogger(ctx, clog.New(slog.Default().Handler()))

	// Get port early (doesn't depend on GitHub App config)
	port := 8080
	if p := os.Getenv("PORT"); p != "" {
		fmt.Sscanf(p, "%d", &port)
	}

	// Create ready gate with healthz always allowed
	gate := configwait.NewReadyGate(nil, []string{"/healthz"})

	// Create HTTP mux
	mux := http.NewServeMux()

	// Create STS handler wrapper for hot-swapping
	stsHandler := &stsHandler{}

	// Register health check endpoint (always available)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if gate.IsReady() {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("not ready"))
		}
	})

	// Register STS handler (delegates to the atomic handler)
	mux.Handle("/", stsHandler)

	// Set the mux as the gate's handler for allowed paths
	gate.SetHandler(mux)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		ReadHeaderTimeout: 10 * time.Second,
		Handler:           gate,
	}

	log.Printf("Starting HTTP server on port %d (waiting for configuration...)", port)

	// Start server immediately (will return 503 until ready)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// loadConfig loads configuration and creates the STS instance.
	// It can be called multiple times to reload configuration.
	loadConfig := func(ctx context.Context) error {
		baseCfg, err := envConfig.BaseConfig()
		if err != nil {
			return fmt.Errorf("base config: %w", err)
		}

		appConfig, err := envConfig.AppConfig()
		if err != nil {
			return fmt.Errorf("app config: %w", err)
		}

		// Create GitHub App transport
		atr, err := ghtransport.New(ctx, baseCfg, nil)
		if err != nil {
			return fmt.Errorf("error creating GitHub App transport: %w", err)
		}

		// Create STS instance
		stsInstance, err := sts.New(atr, sts.Config{
			Domain: appConfig.Domain,
		})
		if err != nil {
			return fmt.Errorf("failed to create sts: %w", err)
		}

		// Hot-swap the STS handler
		stsHandler.SetSTS(stsInstance)

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
			log.Fatalf("failed to load configuration after retries: %v", err)
		}

		log.Printf("Configuration loaded, service is ready")

		// Set up reloader for SIGHUP and programmatic triggers
		reloader := configwait.NewReloader(ctx, gate, loadConfig)
		configwait.SetGlobalReloader(reloader)
		reloader.Start()

		log.Printf("Configuration reloader started (send SIGHUP to reload)")
	}()

	// Wait for interrupt signal
	<-ctx.Done()
	log.Println("Shutting down server...")

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server shutdown error: %v", err)
	}
}
