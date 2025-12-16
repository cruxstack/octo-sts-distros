// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"

	"github.com/chainguard-dev/clog"

	"github.com/cruxstack/github-app-setup-go/ghappsetup"
	"github.com/cruxstack/octo-sts-distros/internal/shared"
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
	log := clog.FromContext(r.Context())

	stsInstance := h.sts.Load()
	if stsInstance == nil {
		http.Error(w, "service not configured", http.StatusServiceUnavailable)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	headers := make(map[string]string)
	for k := range r.Header {
		headers[strings.ToLower(k)] = r.Header.Get(k)
	}

	// Convert URL query parameters to map[string]string
	queryParams := make(map[string]string)
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			queryParams[k] = v[0]
		}
	}

	req := shared.Request{
		Type:        shared.RequestTypeHTTP,
		Method:      r.Method,
		Path:        r.URL.Path,
		Headers:     headers,
		QueryParams: queryParams,
		Body:        body,
	}

	resp := stsInstance.HandleRequest(r.Context(), req)

	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}

	w.WriteHeader(resp.StatusCode)
	if resp.Body != nil {
		if _, err := w.Write(resp.Body); err != nil {
			log.Errorf("failed to write response body: %v", err)
		}
	}
}

func (h *stsHandler) SetSTS(s *sts.STS) {
	h.sts.Store(s)
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

	// Create STS handler (will be configured after config loads)
	stsHandler := &stsHandler{}

	// Create runtime with unified lifecycle management
	runtime, err := ghappsetup.NewRuntime(ghappsetup.Config{
		LoadFunc: func(ctx context.Context) error {
			return loadConfig(ctx, stsHandler)
		},
		AllowedPaths: []string{"/healthz"},
	})
	if err != nil {
		log.Errorf("failed to create runtime: %v", err)
		os.Exit(1)
	}

	// Set up routes
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", runtime.HealthHandler())
	mux.Handle("/", stsHandler)

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

// loadConfig loads configuration and creates the STS instance (supports reload).
func loadConfig(ctx context.Context, stsHandler *stsHandler) error {
	// Re-run env mapping for hot-reload support
	shared.SetupEnvMapping()

	baseCfg, err := envConfig.BaseConfig()
	if err != nil {
		return fmt.Errorf("base config: %w", err)
	}

	appConfig, err := envConfig.AppConfig()
	if err != nil {
		return fmt.Errorf("app config: %w", err)
	}

	atr, err := ghtransport.New(ctx, baseCfg, nil)
	if err != nil {
		return fmt.Errorf("error creating GitHub App transport: %w", err)
	}

	stsInstance, err := sts.New(atr, sts.Config{
		Domain: appConfig.Domain,
	})
	if err != nil {
		return fmt.Errorf("failed to create sts: %w", err)
	}

	stsHandler.SetSTS(stsInstance)
	return nil
}
