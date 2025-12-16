// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/chainguard-dev/clog"

	envConfig "github.com/octo-sts/app/pkg/envconfig"
	"github.com/octo-sts/app/pkg/ghtransport"

	"github.com/cruxstack/github-app-setup-go/ghappsetup"
	"github.com/cruxstack/github-app-setup-go/ssmresolver"
	"github.com/cruxstack/octo-sts-distros/internal/shared"
	"github.com/cruxstack/octo-sts-distros/internal/sts"
)

var (
	// runtime provides unified lifecycle management for the Lambda function
	runtime *ghappsetup.Runtime

	// stsInstance handles STS requests (initialized via runtime.EnsureLoaded)
	stsInstance *sts.STS
)

func init() {
	shared.SetupEnvMapping()

	ctx := context.Background()
	ctx = clog.WithLogger(ctx, clog.New(shared.NewSlogHandler()))
	log := clog.FromContext(ctx)

	var err error
	runtime, err = ghappsetup.NewRuntime(ghappsetup.Config{
		LoadFunc: func(ctx context.Context) error {
			// Resolve SSM parameters passed as ARNs
			if err := ssmresolver.ResolveEnvironmentWithDefaults(ctx); err != nil {
				return err
			}
			return initSTSHandler(ctx)
		},
	})
	if err != nil {
		log.Errorf("failed to create runtime: %v", err)
		// Don't exit - let EnsureLoaded handle the error
	}
}

// initSTSHandler creates the STS handler with current configuration.
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

	// Lazy-load config with retries (idempotent after first success)
	if err := runtime.EnsureLoaded(ctx); err != nil {
		log.Warnf("failed to load configuration: %v", err)
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
	stsReq := shared.Request{
		Type:        shared.RequestTypeHTTP,
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
