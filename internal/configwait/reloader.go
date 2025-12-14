// Copyright 2025 Octo-STS
// SPDX-License-Identifier: MIT

package configwait

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// ReloadFunc is called when a reload is triggered.
// It should reload configuration and return the new http.Handler to use.
// If an error is returned, the reload is considered failed and the old handler remains.
type ReloadFunc func(ctx context.Context) error

// Reloader manages configuration reloading via SIGHUP signals or programmatic triggers.
// It coordinates with ReadyGate to atomically swap handlers when config changes.
type Reloader struct {
	gate       *ReadyGate
	reloadFunc ReloadFunc
	ctx        context.Context

	mu        sync.Mutex
	reloading bool
	reloadCh  chan struct{}
}

// NewReloader creates a new Reloader that will call reloadFunc when triggered.
// The reloadFunc should reload configuration and call gate.SetHandler() with the new handler.
func NewReloader(ctx context.Context, gate *ReadyGate, reloadFunc ReloadFunc) *Reloader {
	return &Reloader{
		gate:       gate,
		reloadFunc: reloadFunc,
		ctx:        ctx,
		reloadCh:   make(chan struct{}, 1),
	}
}

// Start begins listening for SIGHUP signals and programmatic reload triggers.
// It runs in the background and should be called after initial configuration is loaded.
// The returned channel is closed when the reloader stops (context cancelled).
func (r *Reloader) Start() <-chan struct{} {
	done := make(chan struct{})

	// Set up SIGHUP handler
	sighupCh := make(chan os.Signal, 1)
	signal.Notify(sighupCh, syscall.SIGHUP)

	go func() {
		defer close(done)
		defer signal.Stop(sighupCh)

		for {
			select {
			case <-r.ctx.Done():
				return
			case <-sighupCh:
				log.Printf("[reloader] received SIGHUP, triggering reload")
				r.doReload()
			case <-r.reloadCh:
				log.Printf("[reloader] programmatic reload triggered")
				r.doReload()
			}
		}
	}()

	return done
}

// Trigger requests a configuration reload.
// If a reload is already in progress, this call is a no-op.
// This is safe to call from any goroutine.
func (r *Reloader) Trigger() {
	select {
	case r.reloadCh <- struct{}{}:
		// Reload queued
	default:
		// Reload already pending
		log.Printf("[reloader] reload already pending, ignoring trigger")
	}
}

// doReload performs the actual reload operation.
func (r *Reloader) doReload() {
	r.mu.Lock()
	if r.reloading {
		r.mu.Unlock()
		log.Printf("[reloader] reload already in progress, skipping")
		return
	}
	r.reloading = true
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		r.reloading = false
		r.mu.Unlock()
	}()

	log.Printf("[reloader] starting configuration reload...")

	if err := r.reloadFunc(r.ctx); err != nil {
		log.Printf("[reloader] reload failed: %v", err)
		return
	}

	log.Printf("[reloader] configuration reloaded successfully")
}

// Global reloader instance for use by the installer
var (
	globalReloaderMu sync.RWMutex
	globalReloader   *Reloader
)

// SetGlobalReloader sets the global reloader instance.
// This is called by the main application after creating the reloader.
func SetGlobalReloader(r *Reloader) {
	globalReloaderMu.Lock()
	defer globalReloaderMu.Unlock()
	globalReloader = r
}

// TriggerReload triggers a reload using the global reloader.
// If no global reloader is set, this is a no-op.
// This is intended to be called by the installer after saving credentials.
func TriggerReload() {
	globalReloaderMu.RLock()
	r := globalReloader
	globalReloaderMu.RUnlock()

	if r != nil {
		r.Trigger()
	} else {
		log.Printf("[reloader] no global reloader set, cannot trigger reload")
	}
}
