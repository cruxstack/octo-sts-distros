// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

// Package configwait provides utilities for waiting on configuration availability
// during startup, while allowing HTTP servers to respond appropriately.
package configwait

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chainguard-dev/clog"
)

// Environment variable names for configwait configuration.
const (
	EnvMaxRetries    = "CONFIG_WAIT_MAX_RETRIES"
	EnvRetryInterval = "CONFIG_WAIT_RETRY_INTERVAL"
)

// Default configuration values.
const (
	DefaultMaxRetries    = 30
	DefaultRetryInterval = 2 * time.Second
)

// Config configures the wait behavior.
type Config struct {
	// MaxRetries is the maximum number of retry attempts.
	// Default: 30 (from CONFIG_WAIT_MAX_RETRIES env var)
	MaxRetries int

	// RetryInterval is the duration between retry attempts.
	// Default: 2s (from CONFIG_WAIT_RETRY_INTERVAL env var)
	RetryInterval time.Duration
}

// NewConfigFromEnv creates a Config from environment variables.
// Uses defaults if environment variables are not set.
func NewConfigFromEnv() Config {
	cfg := Config{
		MaxRetries:    DefaultMaxRetries,
		RetryInterval: DefaultRetryInterval,
	}

	if v := os.Getenv(EnvMaxRetries); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxRetries = n
		}
	}

	if v := os.Getenv(EnvRetryInterval); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.RetryInterval = d
		}
	}

	return cfg
}

// LoadFunc is called repeatedly until it succeeds or max retries is reached.
// It should attempt to load configuration and return nil on success.
type LoadFunc func(ctx context.Context) error

// Wait blocks until the load function succeeds or max retries is reached.
// It logs retry attempts and returns the last error on failure.
func Wait(ctx context.Context, cfg Config, load LoadFunc) error {
	log := clog.FromContext(ctx)
	var lastErr error

	for attempt := 1; attempt <= cfg.MaxRetries; attempt++ {
		if err := load(ctx); err != nil {
			lastErr = err
			log.Warnf("[configwait] attempt %d/%d failed: %v", attempt, cfg.MaxRetries, err)

			if attempt < cfg.MaxRetries {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(cfg.RetryInterval):
					// Continue to next attempt
				}
			}
		} else {
			if attempt > 1 {
				log.Infof("[configwait] configuration loaded successfully after %d attempts", attempt)
			}
			return nil
		}
	}

	return lastErr
}

// ReadyGate is an HTTP handler that gates requests based on readiness state.
// It returns 503 Service Unavailable for requests to non-allowed paths until
// the service is marked as ready.
type ReadyGate struct {
	inner        http.Handler
	allowedPaths []string
	ready        atomic.Bool
	handler      atomic.Value // stores http.Handler once ready

	mu           sync.Mutex
	handlerReady chan struct{}
}

// NewReadyGate creates a new ReadyGate that wraps the given handler.
// The allowedPaths parameter specifies path prefixes that are always allowed
// through, even before the service is ready (e.g., "/setup", "/healthz").
//
// The inner handler can be nil initially if it depends on configuration.
// In that case, call SetHandler() once the handler is ready.
func NewReadyGate(inner http.Handler, allowedPaths []string) *ReadyGate {
	rg := &ReadyGate{
		inner:        inner,
		allowedPaths: allowedPaths,
		handlerReady: make(chan struct{}),
	}
	if inner != nil {
		rg.handler.Store(inner)
	}
	return rg
}

// SetReady marks the service as ready to handle all requests.
func (rg *ReadyGate) SetReady() {
	rg.ready.Store(true)
}

// SetHandler sets the main handler to use once ready.
// This is useful when the handler depends on configuration that isn't
// available at construction time.
func (rg *ReadyGate) SetHandler(h http.Handler) {
	rg.handler.Store(h)
	rg.mu.Lock()
	defer rg.mu.Unlock()
	select {
	case <-rg.handlerReady:
		// Already closed
	default:
		close(rg.handlerReady)
	}
}

// IsReady returns true if the service is ready.
func (rg *ReadyGate) IsReady() bool {
	return rg.ready.Load()
}

// ServeHTTP implements http.Handler.
func (rg *ReadyGate) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check if this path is always allowed
	if rg.isAllowedPath(r.URL.Path) {
		h := rg.getHandler()
		if h != nil {
			h.ServeHTTP(w, r)
			return
		}
		// Handler not ready yet, return 503
		rg.serveUnavailable(w, r, "service starting up")
		return
	}

	// For non-allowed paths, check readiness
	if !rg.ready.Load() {
		rg.serveUnavailable(w, r, "service not ready, configuration loading")
		return
	}

	// Service is ready, pass through
	h := rg.getHandler()
	if h == nil {
		rg.serveUnavailable(w, r, "service starting up")
		return
	}
	h.ServeHTTP(w, r)
}

// isAllowedPath checks if the given path matches any allowed path prefix.
func (rg *ReadyGate) isAllowedPath(path string) bool {
	for _, allowed := range rg.allowedPaths {
		if allowed == "/" {
			if path == "/" {
				return true
			}
			continue
		}
		if strings.HasPrefix(path, allowed) {
			return true
		}
	}
	return false
}

// getHandler returns the current handler, or nil if not set.
func (rg *ReadyGate) getHandler() http.Handler {
	h := rg.handler.Load()
	if h == nil {
		return nil
	}
	return h.(http.Handler)
}

// serveUnavailable writes a 503 Service Unavailable response.
func (rg *ReadyGate) serveUnavailable(w http.ResponseWriter, r *http.Request, message string) {
	log := clog.FromContext(r.Context())

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", "5")
	w.WriteHeader(http.StatusServiceUnavailable)
	if err := json.NewEncoder(w).Encode(map[string]string{
		"error":   "service_unavailable",
		"message": message,
	}); err != nil {
		log.Errorf("[configwait] failed to write unavailable response: %v", err)
	}
}
