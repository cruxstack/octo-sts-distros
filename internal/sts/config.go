// Copyright 2026 CruxStack
// SPDX-License-Identifier: MIT

package sts

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/google/go-github/v88/github"
	envConfig "github.com/octo-sts/app/pkg/envconfig"

	"github.com/cruxstack/octo-sts-distros/internal/shared"
)

// EnvRequireAppPin names the variable that makes the STS refuse trust policies
// without an app or app_pattern pin.
const EnvRequireAppPin = "OCTOSTS_REQUIRE_APP_PIN"

// ConfigFromEnv builds the STS configuration from the upstream env config and
// the app that atr authenticates as.
func ConfigFromEnv(ctx context.Context, atr http.RoundTripper, appID int64, base *envConfig.EnvConfig, app *envConfig.EnvConfigApp) (Config, error) {
	requirePin, err := strconv.ParseBool(shared.GetEnvDefault(EnvRequireAppPin, "false"))
	if err != nil {
		return Config{}, fmt.Errorf("parsing %s: %w", EnvRequireAppPin, err)
	}

	slug, err := AppSlug(ctx, atr)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Domain:         app.Domain,
		OrgPolicyRepo:  app.OrgPolicyRepo,
		PolicyDir:      app.PolicyDir,
		AllowedIssuers: base.AllowedIssuers,
		AppID:          appID,
		AppSlug:        slug,
		RequireAppPin:  requirePin,
	}, nil
}

// AppSlug returns the slug of the GitHub App that atr authenticates as.
func AppSlug(ctx context.Context, atr http.RoundTripper) (string, error) {
	client, err := github.NewClient(github.WithTransport(atr))
	if err != nil {
		return "", err
	}
	app, _, err := client.Apps.Get(ctx, "")
	if err != nil {
		return "", fmt.Errorf("looking up GitHub App: %w", err)
	}
	return app.GetSlug(), nil
}
