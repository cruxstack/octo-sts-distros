// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

package appstore

import (
	"context"
)

const (
	EnvSTSDomain           = "STS_DOMAIN"
	EnvGitHubAppID         = "GITHUB_APP_ID"
	EnvGitHubWebhookSecret = "GITHUB_WEBHOOK_SECRET"
	EnvGitHubClientID      = "GITHUB_CLIENT_ID"
	EnvGitHubClientSecret  = "GITHUB_CLIENT_SECRET"
	EnvAppSecretCert       = "APP_SECRET_CERTIFICATE_ENV_VAR"
)

type HookConfig struct {
	URL string `json:"url"`
}

// AppCredentials are returned from GitHub App manifest creation.
type AppCredentials struct {
	AppID         int64      `json:"id"`
	AppSlug       string     `json:"slug"`
	ClientID      string     `json:"client_id"`
	ClientSecret  string     `json:"client_secret"`
	WebhookSecret string     `json:"webhook_secret"`
	PrivateKey    string     `json:"pem"`
	HTMLURL       string     `json:"html_url"`
	HookConfig    HookConfig `json:"hook_config"`

	STSDomain string `json:"-"` // Set by installer, not from GitHub API
}

// Store saves app credentials to various backends (local disk, AWS SSM, etc).
type Store interface {
	Save(ctx context.Context, creds *AppCredentials) error
}
