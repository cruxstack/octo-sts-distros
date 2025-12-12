// Copyright 2025 Octo-STS
// SPDX-License-Identifier: MIT

package ssmresolver

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

// ssmARNPattern matches SSM Parameter Store ARNs
// Format: arn:aws:ssm:<region>:<account>:parameter/<path>
var ssmARNPattern = regexp.MustCompile(`^arn:aws:ssm:[^:]+:[^:]+:parameter/(.+)$`)

// Client interface for SSM operations, enabling mocking in tests
type Client interface {
	GetParameter(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
}

// Resolver handles SSM parameter resolution
type Resolver struct {
	client Client
}

// New creates a new SSM Resolver with the default AWS configuration
func New(ctx context.Context) (*Resolver, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}
	return &Resolver{
		client: ssm.NewFromConfig(cfg),
	}, nil
}

// NewWithClient creates a Resolver with a custom SSM client (for testing)
func NewWithClient(client Client) *Resolver {
	return &Resolver{client: client}
}

// IsSSMARN checks if the given value is an SSM Parameter Store ARN
func IsSSMARN(value string) bool {
	return ssmARNPattern.MatchString(value)
}

// ExtractParameterName extracts the parameter name from an SSM ARN
// Returns the parameter name with leading slash, e.g., "/octo-sts/prod/GITHUB_APP_ID"
func ExtractParameterName(arn string) (string, bool) {
	matches := ssmARNPattern.FindStringSubmatch(arn)
	if len(matches) != 2 {
		return "", false
	}
	// Ensure parameter name starts with /
	paramName := matches[1]
	if !strings.HasPrefix(paramName, "/") {
		paramName = "/" + paramName
	}
	return paramName, true
}

// ResolveValue resolves a value that may be an SSM ARN to its actual value
// If the value is not an SSM ARN, it returns the value unchanged
func (r *Resolver) ResolveValue(ctx context.Context, value string) (string, error) {
	if !IsSSMARN(value) {
		return value, nil
	}

	paramName, ok := ExtractParameterName(value)
	if !ok {
		return "", fmt.Errorf("invalid SSM ARN format: %s", value)
	}

	resp, err := r.client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           &paramName,
		WithDecryption: ptr(true),
	})
	if err != nil {
		return "", fmt.Errorf("failed to get SSM parameter %s: %w", paramName, err)
	}

	if resp.Parameter == nil || resp.Parameter.Value == nil {
		return "", fmt.Errorf("SSM parameter %s has no value", paramName)
	}

	return *resp.Parameter.Value, nil
}

// ResolveEnvironment scans all environment variables and resolves any SSM ARN values
// This modifies the process environment in place
func (r *Resolver) ResolveEnvironment(ctx context.Context) error {
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, value := parts[0], parts[1]

		if IsSSMARN(value) {
			resolved, err := r.ResolveValue(ctx, value)
			if err != nil {
				return fmt.Errorf("failed to resolve %s: %w", key, err)
			}
			if err := os.Setenv(key, resolved); err != nil {
				return fmt.Errorf("failed to set %s: %w", key, err)
			}
		}
	}
	return nil
}

// ResolveEnvironmentWithDefaults is a convenience function that creates a resolver
// with default configuration and resolves all environment variables
func ResolveEnvironmentWithDefaults(ctx context.Context) error {
	resolver, err := New(ctx)
	if err != nil {
		return err
	}
	return resolver.ResolveEnvironment(ctx)
}

func ptr[T any](v T) *T {
	return &v
}
