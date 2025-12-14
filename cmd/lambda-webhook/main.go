// Copyright 2025 Octo-STS
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/chainguard-dev/clog"

	"github.com/cruxstack/octo-sts-distros/internal/app"
	"github.com/cruxstack/octo-sts-distros/internal/shared"
	"github.com/cruxstack/octo-sts-distros/internal/ssmresolver"
	envConfig "github.com/octo-sts/app/pkg/envconfig"
	"github.com/octo-sts/app/pkg/ghtransport"
)

var appInstance *app.App

func init() {
	ctx := context.Background()
	ctx = clog.WithLogger(ctx, clog.New(shared.NewSlogHandler()))
	log := clog.FromContext(ctx)

	// Resolve SSM ARNs with retry (helps during deployments)
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

	baseCfg.Metrics = false // GCP-specific

	atr, err := ghtransport.New(ctx, baseCfg, nil)
	if err != nil {
		log.Errorf("error creating GitHub App transport: %v", err)
		os.Exit(1)
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
		log.Errorf("failed to create app: %v", err)
		os.Exit(1)
	}
}

func handler(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	ctx = clog.WithLogger(ctx, clog.New(shared.NewSlogHandler()))

	appReq := app.Request{
		Type:    app.RequestTypeHTTP,
		Method:  req.RequestContext.HTTP.Method,
		Path:    req.RawPath,
		Headers: app.NormalizeHeaders(req.Headers),
		Body:    []byte(req.Body),
	}

	resp := appInstance.HandleRequest(ctx, appReq)

	return events.APIGatewayV2HTTPResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Headers,
		Body:       string(resp.Body),
	}, nil
}

func main() {
	lambda.Start(handler)
}
