// Copyright 2025 Octo-STS
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
)

// NewSlogHandler creates a new slog.Handler based on the LOG_FORMAT environment variable.
// Defaults to JSON format if not specified.
func NewSlogHandler() slog.Handler {
	format := strings.ToLower(GetEnvDefault(EnvLogFormat, LogFormatJSON))

	opts := &slog.HandlerOptions{}

	switch format {
	case LogFormatText:
		return slog.NewTextHandler(os.Stderr, opts)
	default:
		return slog.NewJSONHandler(os.Stderr, opts)
	}
}
