// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/chainguard-dev/clog"

	envConfig "github.com/octo-sts/app/pkg/envconfig"
	"github.com/octo-sts/app/pkg/ghtransport"

	"github.com/cruxstack/octo-sts-distros/internal/configstore"
	"github.com/cruxstack/octo-sts-distros/internal/shared"
	"github.com/cruxstack/octo-sts-distros/internal/ssmresolver"
	"github.com/cruxstack/octo-sts-distros/internal/sts"
)

var (
	// stsInstance handles STS requests (nil if not configured)
	stsInstance *sts.STS

	// isConfigured indicates whether GitHub App credentials are available
	isConfigured bool

	// installerEnabled indicates whether the installer is enabled (from env var)
	installerEnabled bool
)

func init() {
	ctx := context.Background()
	ctx = clog.WithLogger(ctx, clog.New(shared.NewSlogHandler()))
	log := clog.FromContext(ctx)

	installerEnabled = configstore.InstallerEnabled()

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

	// Try to initialize the STS handler
	if err := initSTSHandler(ctx); err != nil {
		if installerEnabled {
			// If installer is enabled, missing config is expected before setup
			log.Warnf("STS handler not initialized (expected if GitHub App not yet created): %v", err)
		} else {
			// If installer is disabled, this is a fatal error
			log.Errorf("failed to initialize STS handler: %v", err)
			os.Exit(1)
		}
	} else {
		isConfigured = true
		log.Infof("[config] STS handler initialized successfully")
	}
}

// initSTSHandler attempts to create the STS handler with current configuration.
// Returns an error if required configuration is missing.
func initSTSHandler(ctx context.Context) error {
	log := clog.FromContext(ctx)

	baseCfg, err := envConfig.BaseConfig()
	if err != nil {
		return err
	}

	appConfig, err := envConfig.AppConfig()
	if err != nil {
		return err
	}

	baseCfg.Metrics = false // GCP-specific

	atr, err := ghtransport.New(ctx, baseCfg, nil)
	if err != nil {
		return err
	}

	stsInstance, err = sts.New(atr, sts.Config{
		Domain:   appConfig.Domain,
		BasePath: "/sts", // API Gateway routes /sts/* to this Lambda
	})
	if err != nil {
		return err
	}

	log.Infof("[config] STS handler configured for domain: %s", appConfig.Domain)
	return nil
}

func handler(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	ctx = clog.WithLogger(ctx, clog.New(shared.NewSlogHandler()))
	log := clog.FromContext(ctx)

	// If STS handler is not configured, return service unavailable
	if stsInstance == nil {
		return events.APIGatewayV2HTTPResponse{
			StatusCode: http.StatusServiceUnavailable,
			Headers: map[string]string{
				"Content-Type": "application/json",
				"Retry-After":  "5",
			},
			Body: `{"error":"service_unavailable","message":"STS service not configured - complete GitHub App setup first"}`,
		}, nil
	}

	path := req.RawPath
	method := req.RequestContext.HTTP.Method

	log.Infof("request: method=%s path=%s", method, path)

	// Convert API Gateway request to STS request
	stsReq := sts.Request{
		Type:        sts.RequestTypeHTTP,
		Method:      method,
		Path:        path,
		Headers:     shared.NormalizeHeaders(req.Headers),
		QueryParams: req.QueryStringParameters,
		Body:        []byte(req.Body),
	}

	// Handle the request
	resp := stsInstance.HandleRequest(ctx, stsReq)

	return events.APIGatewayV2HTTPResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Headers,
		Body:       string(resp.Body),
	}, nil
}

func main() {
	lambda.Start(handler)
}
