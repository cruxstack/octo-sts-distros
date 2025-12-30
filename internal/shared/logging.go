// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

package shared

import (
	"log/slog"
	"os"
	"strings"
)

// Log format constants.
const (
	// LogFormatJSON outputs logs in JSON format (default).
	LogFormatJSON = "json"

	// LogFormatText outputs logs in human-readable text format.
	LogFormatText = "text"
)

// Environment variable names for logging configuration.
const (
	EnvLogFormat = "LOG_FORMAT"
	EnvLogLevel  = "LOG_LEVEL"
)

// NewSlogHandler creates a new slog.Handler based on the LOG_FORMAT environment variable.
// Defaults to JSON format if not specified.
// Log level can be set via LOG_LEVEL env var (debug, info, warn, error). Defaults to info.
func NewSlogHandler() slog.Handler {
	format := strings.ToLower(GetEnvDefault(EnvLogFormat, LogFormatJSON))
	level := parseLogLevel(GetEnvDefault(EnvLogLevel, "info"))

	opts := &slog.HandlerOptions{
		Level: level,
	}

	switch format {
	case LogFormatText:
		return slog.NewTextHandler(os.Stderr, opts)
	default:
		return slog.NewJSONHandler(os.Stderr, opts)
	}
}

// parseLogLevel converts a string log level to slog.Level.
func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// IsDebugEnabled returns true if debug logging is enabled.
func IsDebugEnabled() bool {
	level := strings.ToLower(GetEnvDefault(EnvLogLevel, "info"))
	return level == "debug"
}
