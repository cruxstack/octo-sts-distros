// Copyright 2025 Octo-STS
// SPDX-License-Identifier: MIT

package sts

import (
	"errors"
	"strings"

	"github.com/bradleyfalzon/ghinstallation/v2"
)

// Config provides configuration for the STS service.
type Config struct {
	// Domain is the expected audience for OIDC tokens when no audience
	// is specified in the trust policy (e.g., "sts.octo-sts.dev").
	Domain string

	// BasePath is stripped from incoming request paths before routing.
	// For example, if BasePath is "/sts", then a request to "/sts/exchange"
	// will be routed as if it were "/exchange".
	BasePath string
}

// STS handles GitHub STS token exchange requests in a runtime-agnostic way.
// It provides a unified interface that works with both standard HTTP servers
// and AWS API Gateway v2 with Lambda.
type STS struct {
	transport *ghinstallation.AppsTransport
	domain    string
	basePath  string
}

// New creates a new STS instance with the given GitHub App transport and configuration.
//
// The transport is used to authenticate as the GitHub App when making API calls.
// It should be created using ghinstallation.NewAppsTransport or similar.
//
// Returns an error if transport is nil or if domain is empty.
func New(transport *ghinstallation.AppsTransport, cfg Config) (*STS, error) {
	if transport == nil {
		return nil, errors.New("transport is required")
	}
	if cfg.Domain == "" {
		return nil, errors.New("domain is required")
	}

	// Normalize base path: ensure no trailing slash
	basePath := strings.TrimSuffix(cfg.BasePath, "/")

	return &STS{
		transport: transport,
		domain:    cfg.Domain,
		basePath:  basePath,
	}, nil
}
