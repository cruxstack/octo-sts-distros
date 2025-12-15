// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

package shared

import "time"

// Server configuration defaults.
const (
	// DefaultPort is the default HTTP server port.
	DefaultPort = 8080

	// DefaultReadHeaderTimeout is the default timeout for reading request headers.
	DefaultReadHeaderTimeout = 10 * time.Second

	// DefaultShutdownTimeout is the default timeout for graceful server shutdown.
	DefaultShutdownTimeout = 30 * time.Second
)

// Cache configuration defaults.
const (
	// DefaultCacheSize is the default size for LRU caches.
	DefaultCacheSize = 200

	// DefaultCacheTTL is the default TTL for cached items (5 minutes).
	DefaultCacheTTL = 5 * time.Minute
)
