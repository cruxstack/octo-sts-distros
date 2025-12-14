// Copyright 2025 Octo-STS
// SPDX-License-Identifier: MIT

package configstore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/cruxstack/octo-sts-distros/internal/shared"
)

const (
	EnvSTSDomain           = "STS_DOMAIN"
	EnvGitHubAppID         = "GITHUB_APP_ID"
	EnvGitHubWebhookSecret = "GITHUB_WEBHOOK_SECRET"
	EnvGitHubClientID      = "GITHUB_CLIENT_ID"
	EnvGitHubClientSecret  = "GITHUB_CLIENT_SECRET"
	EnvAppSecretCert       = "APP_SECRET_CERTIFICATE_ENV_VAR"
)

// Environment variable names for store configuration.
const (
	EnvInstallerEnabled   = "INSTALLER_ENABLED"
	EnvStorageMode        = "STORAGE_MODE"
	EnvStorageDir         = "STORAGE_DIR"
	EnvAWSSSMParameterPfx = "AWS_SSM_PARAMETER_PREFIX"
	EnvAWSSSMKMSKeyID     = "AWS_SSM_KMS_KEY_ID"
	EnvAWSSSMTags         = "AWS_SSM_TAGS"
)

// Storage mode constants.
const (
	StorageModeEnvFile = "envfile"
	StorageModeFiles   = "files"
	StorageModeAWSSSM  = "aws-ssm"
)

// HookConfig contains webhook configuration returned from GitHub.
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

// NewFromEnv creates a Store based on environment variable configuration.
// It reads STORAGE_MODE to determine the backend type:
//   - "envfile" (default): saves to a .env file at STORAGE_DIR (default: ./.env)
//   - "files": saves to individual files in STORAGE_DIR directory
//   - "aws-ssm": saves to AWS SSM Parameter Store with AWS_SSM_PARAMETER_PREFIX
//
// Returns an error if configuration is invalid or store creation fails.
func NewFromEnv() (Store, error) {
	mode := getEnvDefault(EnvStorageMode, StorageModeEnvFile)

	switch mode {
	case StorageModeFiles:
		dir := getEnvDefault(EnvStorageDir, "./.env")
		return NewLocalFileStore(dir), nil

	case StorageModeEnvFile:
		path := getEnvDefault(EnvStorageDir, "./.env")
		return NewLocalEnvFileStore(path), nil

	case StorageModeAWSSSM:
		prefix := os.Getenv(EnvAWSSSMParameterPfx)
		if prefix == "" {
			return nil, fmt.Errorf("%s is required when using %s storage mode", EnvAWSSSMParameterPfx, StorageModeAWSSSM)
		}

		var opts []SSMStoreOption

		if kmsKeyID := os.Getenv(EnvAWSSSMKMSKeyID); kmsKeyID != "" {
			opts = append(opts, WithKMSKey(kmsKeyID))
		}

		if tagsJSON := os.Getenv(EnvAWSSSMTags); tagsJSON != "" {
			var tags map[string]string
			if err := json.Unmarshal([]byte(tagsJSON), &tags); err != nil {
				return nil, fmt.Errorf("failed to parse %s as JSON: %w", EnvAWSSSMTags, err)
			}
			opts = append(opts, WithTags(tags))
		}

		return NewAWSSSMStore(prefix, opts...)

	default:
		return nil, fmt.Errorf("unknown %s: %s (expected '%s', '%s', or '%s')",
			EnvStorageMode, mode, StorageModeEnvFile, StorageModeFiles, StorageModeAWSSSM)
	}
}

// InstallerEnabled returns true if the installer is enabled via environment variable.
func InstallerEnabled() bool {
	v := strings.ToLower(os.Getenv(EnvInstallerEnabled))
	return v == "true" || v == "1" || v == "yes"
}

// getEnvDefault is an alias to the shared implementation.
var getEnvDefault = shared.GetEnvDefault
