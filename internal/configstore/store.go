// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

// Package configstore re-exports the configstore package from the ghappsetup library
// and adds octo-sts specific functionality.
package configstore

import (
	"net/url"
	"strings"

	"github.com/cruxstack/github-app-setup-go/configstore"
)

// Re-export types from the library
type (
	Store           = configstore.Store
	AppCredentials  = configstore.AppCredentials
	InstallerStatus = configstore.InstallerStatus
	HookConfig      = configstore.HookConfig
	AWSSSMStore     = configstore.AWSSSMStore
	SSMStoreOption  = configstore.SSMStoreOption
	SSMClient       = configstore.SSMClient
)

// Re-export constants from the library
const (
	EnvGitHubAppID               = configstore.EnvGitHubAppID
	EnvGitHubAppSlug             = configstore.EnvGitHubAppSlug
	EnvGitHubAppHTMLURL          = configstore.EnvGitHubAppHTMLURL
	EnvGitHubWebhookSecret       = configstore.EnvGitHubWebhookSecret
	EnvGitHubClientID            = configstore.EnvGitHubClientID
	EnvGitHubClientSecret        = configstore.EnvGitHubClientSecret
	EnvGitHubAppPrivateKey       = configstore.EnvGitHubAppPrivateKey
	EnvGitHubAppInstallerEnabled = configstore.EnvGitHubAppInstallerEnabled
	EnvStorageMode               = configstore.EnvStorageMode
	EnvStorageDir                = configstore.EnvStorageDir
	EnvAWSSSMParameterPfx        = configstore.EnvAWSSSMParameterPfx
	EnvAWSSSMKMSKeyID            = configstore.EnvAWSSSMKMSKeyID
	EnvAWSSSMTags                = configstore.EnvAWSSSMTags
	StorageModeEnvFile           = configstore.StorageModeEnvFile
	StorageModeFiles             = configstore.StorageModeFiles
	StorageModeAWSSSM            = configstore.StorageModeAWSSSM
)

// Octo-STS specific constant
const (
	EnvSTSDomain = "STS_DOMAIN"
)

// Re-export functions from the library
var (
	NewFromEnv           = configstore.NewFromEnv
	InstallerEnabled     = configstore.InstallerEnabled
	NewAWSSSMStore       = configstore.NewAWSSSMStore
	NewLocalFileStore    = configstore.NewLocalFileStore
	NewLocalEnvFileStore = configstore.NewLocalEnvFileStore
	WithKMSKey           = configstore.WithKMSKey
	WithTags             = configstore.WithTags
	WithSSMClient        = configstore.WithSSMClient
	GetEnvDefault        = configstore.GetEnvDefault
)

// ExtractSTSDomainFromWebhookURL extracts the STS domain from a webhook URL.
// This is an octo-sts specific helper function.
func ExtractSTSDomainFromWebhookURL(webhookURL string) string {
	if webhookURL == "" {
		return ""
	}
	if parsedURL, err := url.Parse(webhookURL); err == nil && parsedURL.Host != "" {
		return parsedURL.Host
	}
	return ""
}

// ShouldUpdateSTSDomain returns true if existing is empty or either host is an ngrok domain.
func ShouldUpdateSTSDomain(existingHost, newHost string) bool {
	if existingHost == "" {
		return true
	}
	isNewNgrok := strings.Contains(newHost, "ngrok-free.app") || strings.Contains(newHost, "ngrok.io")
	isExistingNgrok := strings.Contains(existingHost, "ngrok-free.app") || strings.Contains(existingHost, "ngrok.io")
	return isNewNgrok || isExistingNgrok
}
