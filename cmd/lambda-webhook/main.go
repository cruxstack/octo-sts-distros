// Copyright 2025 Octo-STS
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/chainguard-dev/clog"

	"github.com/cruxstack/octo-sts-distros/internal/app"
	"github.com/cruxstack/octo-sts-distros/internal/ssmresolver"
	envConfig "github.com/octo-sts/app/pkg/envconfig"
	"github.com/octo-sts/app/pkg/ghtransport"
)

var appInstance *app.App

func init() {
	ctx := context.Background()
	ctx = clog.WithLogger(ctx, clog.New(slog.Default().Handler()))
	log := clog.FromContext(ctx)

	// Resolve SSM ARNs in environment variables with retry support
	// This helps during deployments where SSM parameters might not be immediately available
	retryCfg := ssmresolver.NewRetryConfigFromEnv()
	if err := ssmresolver.ResolveEnvironmentWithRetry(ctx, retryCfg); err != nil {
		log.Errorf("failed to resolve SSM parameters: %v", err)
		os.Exit(1)
	}

	baseCfg, err := envConfig.BaseConfig()
	if err != nil {
		log.Errorf("failed to process env var: %v", err)
		os.Exit(1)
	}

	webhookConfig, err := envConfig.WebhookConfig()
	if err != nil {
		log.Errorf("failed to process env var: %v", err)
		os.Exit(1)
	}

	// Disable metrics for Lambda (GCP-specific)
	baseCfg.Metrics = false

	// Create GitHub App transport (nil KMS client - not using GCP KMS in Lambda)
	atr, err := ghtransport.New(ctx, baseCfg, nil)
	if err != nil {
		log.Errorf("error creating GitHub App transport: %v", err)
		os.Exit(1)
	}

	// Parse organization filter
	var orgs []string
	for _, s := range strings.Split(webhookConfig.OrganizationFilter, ",") {
		if o := strings.TrimSpace(s); o != "" {
			orgs = append(orgs, o)
		}
	}

	// Create App instance using the runtime-agnostic internal/app package
	appInstance, err = app.New(atr, app.Config{
		WebhookSecrets: [][]byte{[]byte(webhookConfig.WebhookSecret)},
		Organizations:  orgs,
	})
	if err != nil {
		log.Errorf("failed to create app: %v", err)
		os.Exit(1)
	}
}

func handler(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	// Add logger to context
	ctx = clog.WithLogger(ctx, clog.New(slog.Default().Handler()))

	// Convert API Gateway v2 request to app.Request
	appReq := app.Request{
		Type:    app.RequestTypeHTTP,
		Method:  req.RequestContext.HTTP.Method,
		Path:    req.RawPath,
		Headers: app.NormalizeHeaders(req.Headers),
		Body:    []byte(req.Body),
	}

	// Handle request using the runtime-agnostic app
	resp := appInstance.HandleRequest(ctx, appReq)

	// Convert app.Response to API Gateway v2 response
	return events.APIGatewayV2HTTPResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Headers,
		Body:       string(resp.Body),
	}, nil
}

func main() {
	lambda.Start(handler)
}
