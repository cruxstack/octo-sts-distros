// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

package sts

import (
	"errors"
	"strings"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/octo-sts/app/pkg/octosts"
)

// DefaultOrgPolicyRepo is the repository holding org-scoped trust policies
// when Config.OrgPolicyRepo is empty.
const DefaultOrgPolicyRepo = ".github"

// Config provides configuration for the STS service.
type Config struct {
	// Domain is the expected audience for OIDC tokens when no audience
	// is specified in the trust policy (e.g., "sts.octo-sts.dev").
	Domain string

	// BasePath is stripped from incoming request paths before routing.
	// For example, if BasePath is "/sts", then a request to "/sts/exchange"
	// will be routed as if it were "/exchange".
	BasePath string

	// OrgPolicyRepo is the repository holding org-scoped trust policies.
	// Empty means DefaultOrgPolicyRepo.
	OrgPolicyRepo string

	// PolicyDir is the directory, relative to a repository root, holding
	// trust policies. Empty means octosts.DefaultPolicyDir.
	PolicyDir string

	// AllowedIssuers, when non-empty, confines OIDC discovery to these issuers.
	AllowedIssuers []string

	// AppID and AppSlug identify this instance's GitHub App, which a trust
	// policy's app or app_pattern must name.
	AppID   int64
	AppSlug string

	// RequireAppPin refuses trust policies that do not set app or app_pattern.
	RequireAppPin bool
}

// STS handles GitHub STS token exchange requests in a runtime-agnostic way.
// It provides a unified interface that works with both standard HTTP servers
// and AWS API Gateway v2 with Lambda.
type STS struct {
	transport      *ghinstallation.AppsTransport
	domain         string
	basePath       string
	orgPolicyRepo  string
	policyDir      string
	allowedIssuers []string
	appID          int64
	appSlug        string
	requireAppPin  bool
}

// New creates a new STS instance with the given GitHub App transport and configuration.
//
// The transport is used to authenticate as the GitHub App when making API calls.
// It should be created using ghinstallation.NewAppsTransport or similar.
//
// Returns an error if transport is nil, if domain is empty, or if
// RequireAppPin is set without an app ID or slug to match against.
func New(transport *ghinstallation.AppsTransport, cfg Config) (*STS, error) {
	if transport == nil {
		return nil, errors.New("transport is required")
	}
	if cfg.Domain == "" {
		return nil, errors.New("domain is required")
	}
	if cfg.RequireAppPin && cfg.AppID <= 0 && cfg.AppSlug == "" {
		return nil, errors.New("app ID or slug is required when app pins are required")
	}

	// Normalize base path: ensure no trailing slash
	basePath := strings.TrimSuffix(cfg.BasePath, "/")

	orgPolicyRepo := cfg.OrgPolicyRepo
	if orgPolicyRepo == "" {
		orgPolicyRepo = DefaultOrgPolicyRepo
	}
	policyDir := strings.Trim(cfg.PolicyDir, "/")
	if policyDir == "" {
		policyDir = octosts.DefaultPolicyDir
	}

	return &STS{
		transport:      transport,
		domain:         cfg.Domain,
		basePath:       basePath,
		orgPolicyRepo:  orgPolicyRepo,
		policyDir:      policyDir,
		allowedIssuers: cfg.AllowedIssuers,
		appID:          cfg.AppID,
		appSlug:        cfg.AppSlug,
		requireAppPin:  cfg.RequireAppPin,
	}, nil
}
