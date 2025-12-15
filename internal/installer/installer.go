// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

// Package installer re-exports the installer package from the ghappsetup library
// and provides octo-sts specific configuration.
package installer

import (
	"github.com/cruxstack/octo-sts-distros/pkg/ghappsetup/configstore"
	"github.com/cruxstack/octo-sts-distros/pkg/ghappsetup/installer"
)

// Re-export types from the library
type (
	Config               = installer.Config
	Handler              = installer.Handler
	Manifest             = installer.Manifest
	HookAttributes       = installer.HookAttributes
	CredentialsSavedFunc = installer.CredentialsSavedFunc
)

// Re-export constants from the library
const (
	EnvGitHubURL = installer.EnvGitHubURL
	EnvGitHubOrg = installer.EnvGitHubOrg
)

// Re-export functions from the library
var (
	NewConfigFromEnv = installer.NewConfigFromEnv
	New              = installer.New
)

// OctoSTSManifest returns the GitHub App manifest with all permissions required for Octo-STS.
func OctoSTSManifest() Manifest {
	return Manifest{
		Name: "octo-sts",
		URL:  "https://github.com/octo-sts/app",
		DefaultPerms: map[string]string{
			// Repository permissions
			"actions":             "write",
			"administration":      "read",
			"checks":              "write",
			"security_events":     "write", // code_scanning_alerts
			"statuses":            "write",
			"contents":            "write",
			"deployments":         "write",
			"discussions":         "write",
			"environments":        "write",
			"issues":              "write",
			"packages":            "write",
			"pages":               "write",
			"repository_projects": "write",
			"pull_requests":       "write",
			"workflows":           "write",
			// Organization permissions
			"organization_administration": "write",
			"organization_events":         "read",
			"members":                     "write",
			"organization_projects":       "write",
		},
		DefaultEvents: []string{
			"pull_request",
		},
		Public: false,
	}
}

// NewOctoSTSConfig creates an installer config pre-configured for Octo-STS.
// It sets the manifest and branding for Octo-STS.
func NewOctoSTSConfig(store configstore.Store) Config {
	cfg := NewConfigFromEnv()
	cfg.Store = store
	cfg.Manifest = OctoSTSManifest()
	cfg.AppDisplayName = "Octo-STS"
	return cfg
}
