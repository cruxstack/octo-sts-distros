// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"net/http"
	"os"

	"chainguard.dev/go-grpc-kit/pkg/duplex"
	pboidc "chainguard.dev/sdk/proto/platform/oidc/v1"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"
	"github.com/chainguard-dev/clog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	envConfig "github.com/octo-sts/app/pkg/envconfig"
	"github.com/octo-sts/app/pkg/ghtransport"
	"github.com/octo-sts/app/pkg/octosts"

	"github.com/cruxstack/octo-sts-distros/internal/shared"
	"github.com/cruxstack/octo-sts-distros/internal/ssmresolver"
)

var handler *httpadapter.HandlerAdapterV2

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

	appConfig, err := envConfig.AppConfig()
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

	d := duplex.New(
		shared.DefaultPort,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)

	pboidc.RegisterSecurityTokenServiceServer(d.Server,
		octosts.NewSecurityTokenServiceServer(atr, nil, appConfig.Domain, false))

	if err := d.RegisterHandler(ctx, pboidc.RegisterSecurityTokenServiceHandlerFromEndpoint); err != nil {
		log.Errorf("failed to register gateway endpoint: %v", err)
		os.Exit(1)
	}

	if err := d.MUX.HandlePath(http.MethodGet, "/", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {
		w.Header().Set("Content-Type", "application/json")
		s := `{"msg": "please check documentation for usage: https://github.com/octo-sts/app"}`
		if _, err := w.Write([]byte(s)); err != nil {
			clog.FromContext(r.Context()).Errorf("failed to write bytes back to client: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}); err != nil {
		log.Errorf("failed to register root GET handler: %v", err)
		os.Exit(1)
	}

	handler = httpadapter.NewV2(d.MUX)
}

func main() {
	lambda.Start(handler.ProxyWithContext)
}
