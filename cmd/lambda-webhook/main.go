// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"
	"github.com/chainguard-dev/clog"

	"github.com/cruxstack/github-app-setup-go/ssmresolver"
	"github.com/cruxstack/octo-sts-distros/internal/app"
	"github.com/cruxstack/octo-sts-distros/internal/configstore"
	"github.com/cruxstack/octo-sts-distros/internal/installer"
	"github.com/cruxstack/octo-sts-distros/internal/shared"
	envConfig "github.com/octo-sts/app/pkg/envconfig"
	"github.com/octo-sts/app/pkg/ghtransport"
)

var (
	// appInstance handles webhook requests (nil if not configured)
	appInstance *app.App

	// installerAdapter wraps the installer handler for Lambda (nil if installer disabled)
	installerAdapter *httpadapter.HandlerAdapterV2

	// configStore is used to check installer status at request time
	configStore configstore.Store

	// isConfigured indicates whether GitHub App credentials are available
	isConfigured bool

	// installerEnabled indicates whether the installer is enabled (from env var)
	installerEnabled bool
)

func init() {
	shared.SetupEnvMapping()

	ctx := context.Background()
	ctx = clog.WithLogger(ctx, clog.New(shared.NewSlogHandler()))
	log := clog.FromContext(ctx)

	installerEnabled = configstore.InstallerEnabled()

	// Initialize installer handler if enabled (doesn't require GitHub App credentials)
	if installerEnabled {
		store, err := configstore.NewFromEnv()
		if err != nil {
			log.Errorf("failed to create config store: %v", err)
			os.Exit(1)
		}
		configStore = store

		installerCfg := installer.NewOctoSTSConfig(store)

		installerHandler, err := installer.New(installerCfg)
		if err != nil {
			log.Errorf("failed to create installer handler: %v", err)
			os.Exit(1)
		}

		// Use httpadapter for proper HTTP response handling (same as lambda-sts)
		installerAdapter = httpadapter.NewV2(installerHandler)

		log.Infof("[config] installer enabled: /setup endpoint available")
	}

	// Try to resolve SSM ARNs - don't fail if parameters don't exist yet
	// (they may be created by the installer)
	retryCfg := ssmresolver.NewRetryConfigFromEnv()
	if err := ssmresolver.ResolveEnvironmentWithRetry(ctx, retryCfg); err != nil {
		if installerEnabled {
			// If installer is enabled, missing SSM params is expected before setup
			log.Warnf("SSM parameter resolution failed (expected if GitHub App not yet created): %v", err)
		} else {
			// If installer is disabled, this is a fatal error
			log.Errorf("failed to resolve SSM parameters: %v", err)
			os.Exit(1)
		}
	}

	// Try to initialize the webhook handler
	if err := initWebhookHandler(ctx); err != nil {
		if installerEnabled {
			// If installer is enabled, missing config is expected before setup
			log.Warnf("webhook handler not initialized (expected if GitHub App not yet created): %v", err)
		} else {
			// If installer is disabled, this is a fatal error
			log.Errorf("failed to initialize webhook handler: %v", err)
			os.Exit(1)
		}
	} else {
		isConfigured = true
		log.Infof("[config] webhook handler initialized successfully")
	}
}

// tryInitWebhookHandler attempts to resolve SSM parameters and initialize the webhook handler.
// This is used for lazy initialization after the setup wizard saves credentials.
func tryInitWebhookHandler(ctx context.Context) error {
	log := clog.FromContext(ctx)

	// First, try to resolve SSM ARNs to actual values
	if err := ssmresolver.ResolveEnvironmentWithDefaults(ctx); err != nil {
		return err
	}

	// Now try to initialize the webhook handler
	if err := initWebhookHandler(ctx); err != nil {
		return err
	}

	isConfigured = true
	log.Infof("[config] webhook handler initialized via lazy initialization")
	return nil
}

// initWebhookHandler attempts to create the webhook handler with current configuration.
// Returns an error if required configuration is missing.
func initWebhookHandler(ctx context.Context) error {
	log := clog.FromContext(ctx)

	baseCfg, err := envConfig.BaseConfig()
	if err != nil {
		return err
	}

	webhookConfig, err := envConfig.WebhookConfig()
	if err != nil {
		return err
	}

	baseCfg.Metrics = false // GCP-specific

	atr, err := ghtransport.New(ctx, baseCfg, nil)
	if err != nil {
		return err
	}

	var orgs []string
	for _, s := range strings.Split(webhookConfig.OrganizationFilter, ",") {
		if o := strings.TrimSpace(s); o != "" {
			orgs = append(orgs, o)
		}
	}

	appInstance, err = app.New(atr, app.Config{
		WebhookSecrets: [][]byte{[]byte(webhookConfig.WebhookSecret)},
		Organizations:  orgs,
	})
	if err != nil {
		return err
	}

	log.Infof("[config] webhook handler configured for %d organizations", len(orgs))
	return nil
}

func handler(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	ctx = clog.WithLogger(ctx, clog.New(shared.NewSlogHandler()))
	log := clog.FromContext(ctx)

	path := req.RawPath
	method := req.RequestContext.HTTP.Method

	log.Infof("request: method=%s path=%s", method, path)

	// Route based on path
	switch {
	// Health check - always returns 200
	case path == "/healthz":
		return healthzResponse(), nil

	// Installer routes - use httpadapter for proper HTTP handling
	case path == "/setup" || strings.HasPrefix(path, "/setup/"):
		if installerAdapter == nil {
			return notFoundResponse(), nil
		}
		return installerAdapter.ProxyWithContext(ctx, req)

	case path == "/callback":
		if installerAdapter == nil {
			return notFoundResponse(), nil
		}
		return installerAdapter.ProxyWithContext(ctx, req)

	// Root path
	case path == "/" || path == "":
		// Only redirect to /setup if:
		// 1. Installer is enabled via env var
		// 2. App is not yet configured (no credentials)
		// 3. Installer hasn't been disabled via UI (check SSM status)
		if installerEnabled && !isConfigured && !isInstallerDisabled(ctx) {
			return installerAdapter.ProxyWithContext(ctx, req)
		}
		return notFoundResponse(), nil

	// Webhook endpoint
	case path == "/webhook" || strings.HasPrefix(path, "/webhook/"):
		// If webhook handler isn't initialized yet, try to initialize it now.
		// This handles the case where the setup wizard saved credentials after
		// the Lambda cold started without them.
		if appInstance == nil && installerEnabled {
			if err := tryInitWebhookHandler(ctx); err != nil {
				log.Warnf("lazy webhook handler initialization failed: %v", err)
			}
		}
		if appInstance == nil {
			return serviceUnavailableResponse("webhook handler not configured"), nil
		}
		return handleWebhook(ctx, req)

	default:
		return notFoundResponse(), nil
	}
}

// handleWebhook processes webhook requests through the app handler.
func handleWebhook(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	appReq := shared.Request{
		Type:    shared.RequestTypeHTTP,
		Method:  req.RequestContext.HTTP.Method,
		Path:    req.RawPath,
		Headers: shared.NormalizeHeaders(req.Headers),
		Body:    []byte(req.Body),
	}

	resp := appInstance.HandleRequest(ctx, appReq)

	return events.APIGatewayV2HTTPResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Headers,
		Body:       string(resp.Body),
	}, nil
}

// isInstallerDisabled checks if the installer has been disabled via the UI.
// This checks the SSM-stored status, not the environment variable.
func isInstallerDisabled(ctx context.Context) bool {
	if configStore == nil {
		return false
	}
	status, err := configStore.Status(ctx)
	if err != nil {
		clog.FromContext(ctx).Warnf("failed to check installer status: %v", err)
		return false
	}
	return status != nil && status.InstallerDisabled
}

// Response helpers

func healthzResponse() events.APIGatewayV2HTTPResponse {
	return events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers:    map[string]string{"Content-Type": "text/plain"},
		Body:       "ok",
	}
}

func notFoundResponse() events.APIGatewayV2HTTPResponse {
	return events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusNotFound,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       `{"error":"not_found","message":"not found"}`,
	}
}

func serviceUnavailableResponse(message string) events.APIGatewayV2HTTPResponse {
	return events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusServiceUnavailable,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Retry-After":  "5",
		},
		Body: `{"error":"service_unavailable","message":"` + message + `"}`,
	}
}

func main() {
	lambda.Start(handler)
}
