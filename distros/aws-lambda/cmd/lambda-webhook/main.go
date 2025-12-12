// Copyright 2025 Octo-STS
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"strings"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"
	"github.com/chainguard-dev/clog"

	envConfig "github.com/octo-sts/app/pkg/envconfig"
	"github.com/octo-sts/app/pkg/ghtransport"
	"github.com/octo-sts/app/pkg/webhook"

	"github.com/cruxstack/octo-sts-distros/distros/aws-lambda/pkg/ssmresolver"
)

var handler *httpadapter.HandlerAdapterV2

func init() {
	ctx := context.Background()
	ctx = clog.WithLogger(ctx, clog.New(slog.Default().Handler()))

	// Resolve SSM ARNs in environment variables before loading config
	if err := ssmresolver.ResolveEnvironmentWithDefaults(ctx); err != nil {
		log.Fatalf("failed to resolve SSM parameters: %v", err)
	}

	baseCfg, err := envConfig.BaseConfig()
	if err != nil {
		log.Fatalf("failed to process env var: %v", err)
	}

	webhookConfig, err := envConfig.WebhookConfig()
	if err != nil {
		log.Fatalf("failed to process env var: %v", err)
	}

	// Disable metrics for Lambda (GCP-specific)
	baseCfg.Metrics = false

	// Create GitHub App transport (nil KMS client - not using GCP KMS in Lambda)
	atr, err := ghtransport.New(ctx, baseCfg, nil)
	if err != nil {
		log.Fatalf("error creating GitHub App transport: %v", err)
	}

	// Parse webhook secrets (support single secret for Lambda, not GCP Secret Manager)
	webhookSecrets := [][]byte{[]byte(webhookConfig.WebhookSecret)}

	// Parse organization filter
	var orgs []string
	for _, s := range strings.Split(webhookConfig.OrganizationFilter, ",") {
		if o := strings.TrimSpace(s); o != "" {
			orgs = append(orgs, o)
		}
	}

	// Create the webhook handler
	mux := http.NewServeMux()
	mux.Handle("/", &webhook.Validator{
		Transport:     atr,
		WebhookSecret: webhookSecrets,
		Organizations: orgs,
	})

	// Wrap with Lambda HTTP adapter (API Gateway v2 payload format)
	handler = httpadapter.NewV2(mux)
}

func main() {
	lambda.Start(handler.ProxyWithContext)
}
