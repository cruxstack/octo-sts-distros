// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

package installer

// Manifest represents the GitHub App manifest structure.
// See https://docs.github.com/en/apps/sharing-github-apps/registering-a-github-app-from-a-manifest
type Manifest struct {
	Name           string            `json:"name,omitempty"`
	URL            string            `json:"url"`
	HookAttributes HookAttributes    `json:"hook_attributes"`
	RedirectURL    string            `json:"redirect_url"`
	Public         bool              `json:"public"`
	DefaultPerms   map[string]string `json:"default_permissions"`
	DefaultEvents  []string          `json:"default_events"`
}

// HookAttributes configures the webhook for the GitHub App.
type HookAttributes struct {
	URL    string `json:"url"`
	Active bool   `json:"active"`
}

// buildManifest creates the GitHub App manifest with octo-sts permissions.
func buildManifest(redirectURL, webhookURL string) *Manifest {
	return &Manifest{
		URL: "https://github.com/octo-sts/app",
		HookAttributes: HookAttributes{
			URL:    webhookURL,
			Active: webhookURL != "",
		},
		RedirectURL: redirectURL + "/callback",
		Public:      false,
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
	}
}
