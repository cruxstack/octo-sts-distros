// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

package appstore

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// SSMClient defines the interface for AWS SSM operations, enabling mocking in tests.
type SSMClient interface {
	PutParameter(ctx context.Context, params *ssm.PutParameterInput,
		optFns ...func(*ssm.Options)) (*ssm.PutParameterOutput, error)
}

// AWSSSMStore saves credentials to AWS Systems Manager Parameter Store with encryption.
type AWSSSMStore struct {
	ParameterPrefix string
	KMSKeyID        string            // Empty string = default AWS managed key
	Tags            map[string]string // Optional tags for all parameters
	ssmClient       SSMClient
}

// SSMStoreOption is a functional option for configuring AWSSSMStore.
type SSMStoreOption func(*AWSSSMStore)

// WithKMSKey sets a custom KMS key ID for parameter encryption.
// If not set, AWS SSM uses the default AWS managed key.
func WithKMSKey(keyID string) SSMStoreOption {
	return func(s *AWSSSMStore) {
		s.KMSKeyID = keyID
	}
}

// WithTags adds AWS tags to all created parameters.
func WithTags(tags map[string]string) SSMStoreOption {
	return func(s *AWSSSMStore) {
		s.Tags = tags
	}
}

// WithSSMClient sets a custom SSM client (primarily for testing).
func WithSSMClient(client SSMClient) SSMStoreOption {
	return func(s *AWSSSMStore) {
		s.ssmClient = client
	}
}

// NewAWSSSMStore creates a new AWS SSM Parameter Store backend.
// The prefix is normalized to always end with a slash.
// Uses the default AWS credential chain (env vars, IAM role, ~/.aws/credentials).
func NewAWSSSMStore(prefix string, opts ...SSMStoreOption) (*AWSSSMStore, error) {
	if prefix == "" {
		return nil, fmt.Errorf("parameter prefix cannot be empty")
	}

	// Normalize prefix to always end with /
	if !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}

	store := &AWSSSMStore{
		ParameterPrefix: prefix,
	}

	// Apply functional options
	for _, opt := range opts {
		opt(store)
	}

	// Initialize SSM client if not provided (e.g., in tests)
	if store.ssmClient == nil {
		cfg, err := config.LoadDefaultConfig(context.Background())
		if err != nil {
			return nil, fmt.Errorf("failed to load AWS config: %w", err)
		}
		store.ssmClient = ssm.NewFromConfig(cfg)
	}

	return store, nil
}

// Save writes credentials to AWS SSM Parameter Store as encrypted SecureString parameters.
// All parameters are created with overwrite=true and fail-fast on any error.
func (s *AWSSSMStore) Save(ctx context.Context, creds *AppCredentials) error {
	// Build parameter map
	parameters := map[string]string{
		EnvGitHubAppID:         fmt.Sprintf("%d", creds.AppID),
		EnvGitHubWebhookSecret: creds.WebhookSecret,
		EnvGitHubClientID:      creds.ClientID,
		EnvGitHubClientSecret:  creds.ClientSecret,
		EnvAppSecretCert:       creds.PrivateKey,
	}

	// Optionally add STS_DOMAIN if provided
	if creds.STSDomain != "" {
		parameters[EnvSTSDomain] = creds.STSDomain
	}

	// Save each parameter
	for name, value := range parameters {
		if err := s.putParameter(ctx, name, value); err != nil {
			return fmt.Errorf("failed to save parameter %s: %w", name, err)
		}
	}

	return nil
}

// putParameter creates or updates a single SSM parameter with encryption.
func (s *AWSSSMStore) putParameter(ctx context.Context, name, value string) error {
	input := &ssm.PutParameterInput{
		Name:      aws.String(s.ParameterPrefix + name),
		Value:     aws.String(value),
		Type:      types.ParameterTypeSecureString,
		Overwrite: aws.Bool(true),
		DataType:  aws.String("text"),
	}

	// Add custom KMS key if specified
	if s.KMSKeyID != "" {
		input.KeyId = aws.String(s.KMSKeyID)
	}

	// Add tags if specified
	if len(s.Tags) > 0 {
		var tags []types.Tag
		for key, value := range s.Tags {
			tags = append(tags, types.Tag{
				Key:   aws.String(key),
				Value: aws.String(value),
			})
		}
		input.Tags = tags
	}

	_, err := s.ssmClient.PutParameter(ctx, input)
	if err != nil {
		return err
	}

	return nil
}
