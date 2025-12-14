// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

package app

import (
	"errors"
	"strings"

	"github.com/bradleyfalzon/ghinstallation/v2"
)

// Config provides configuration for the App.
type Config struct {
	// WebhookSecrets contains one or more webhook secrets for signature validation.
	// Multiple secrets support rolling updates where both old and new secrets
	// are valid during a transition period.
	WebhookSecrets [][]byte

	// Organizations filters webhook events to only process events from these
	// organizations. If empty, events from all organizations are processed.
	Organizations []string

	// BasePath is stripped from incoming request paths before routing.
	// For example, if BasePath is "/webhook", then a request to "/webhook/foo"
	// will be routed as if it were "/foo".
	BasePath string
}

// App handles GitHub App webhook requests in a runtime-agnostic way.
// It provides a unified interface that works with both standard HTTP servers
// and AWS API Gateway v2 with Lambda.
type App struct {
	transport     *ghinstallation.AppsTransport
	webhookSecret [][]byte
	organizations []string
	basePath      string
}

// New creates a new App instance with the given GitHub App transport and configuration.
//
// The transport is used to authenticate as the GitHub App when making API calls.
// It should be created using ghinstallation.NewAppsTransport or similar.
//
// Returns an error if transport is nil or if no webhook secrets are provided.
func New(transport *ghinstallation.AppsTransport, cfg Config) (*App, error) {
	if transport == nil {
		return nil, errors.New("transport is required")
	}
	if len(cfg.WebhookSecrets) == 0 {
		return nil, errors.New("at least one webhook secret is required")
	}

	// Normalize base path: ensure no trailing slash
	basePath := strings.TrimSuffix(cfg.BasePath, "/")

	return &App{
		transport:     transport,
		webhookSecret: cfg.WebhookSecrets,
		organizations: cfg.Organizations,
		basePath:      basePath,
	}, nil
}
