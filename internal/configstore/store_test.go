// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

package configstore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

func TestLocalFileStore_Save(t *testing.T) {
	dir := t.TempDir()
	store := NewLocalFileStore(dir)

	creds := &AppCredentials{
		AppID:         12345,
		AppSlug:       "test-app",
		ClientID:      "Iv1.abc123",
		ClientSecret:  "secret123",
		WebhookSecret: "webhook-secret",
		PrivateKey:    "-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----",
		HTMLURL:       "https://github.com/apps/test-app",
	}

	if err := store.Save(context.Background(), creds); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify files were created
	tests := []struct {
		file     string
		expected string
	}{
		{"app-id", "12345"},
		{"client-id", "Iv1.abc123"},
		{"client-secret", "secret123"},
		{"webhook-secret", "webhook-secret"},
		{"private-key.pem", "-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----"},
	}

	for _, tt := range tests {
		content, err := os.ReadFile(filepath.Join(dir, tt.file))
		if err != nil {
			t.Errorf("Failed to read %s: %v", tt.file, err)
			continue
		}
		if string(content) != tt.expected {
			t.Errorf("%s = %q, want %q", tt.file, string(content), tt.expected)
		}
	}
}

func TestLocalEnvFileStore_Save_NewFile(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	store := NewLocalEnvFileStore(envPath)

	creds := &AppCredentials{
		AppID:         12345,
		AppSlug:       "test-app",
		ClientID:      "Iv1.abc123",
		ClientSecret:  "secret123",
		WebhookSecret: "webhook-secret",
		PrivateKey:    "-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----",
		HTMLURL:       "https://github.com/apps/test-app",
	}

	if err := store.Save(context.Background(), creds); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Read the .env file
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("Failed to read .env file: %v", err)
	}

	contentStr := string(content)

	// Verify all expected keys are present
	expectedPairs := map[string]string{
		"GITHUB_APP_ID":                  "12345",
		"GITHUB_WEBHOOK_SECRET":          "webhook-secret",
		"GITHUB_CLIENT_ID":               "Iv1.abc123",
		"GITHUB_CLIENT_SECRET":           "secret123",
		"APP_SECRET_CERTIFICATE_ENV_VAR": "-----BEGIN RSA PRIVATE KEY-----\\ntest\\n-----END RSA PRIVATE KEY-----",
	}

	for key, expectedValue := range expectedPairs {
		if !strings.Contains(contentStr, key+"=") {
			t.Errorf(".env file missing key %s", key)
			continue
		}
		if !strings.Contains(contentStr, expectedValue) {
			t.Errorf(".env file missing expected value for %s: %s", key, expectedValue)
		}
	}

	// Verify file permissions
	info, err := os.Stat(envPath)
	if err != nil {
		t.Fatalf("Failed to stat .env file: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf(".env file permissions = %o, want 0600", info.Mode().Perm())
	}
}

func TestLocalEnvFileStore_Save_PreservesExistingValues(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")

	// Create existing .env file with some values
	existingContent := `# Octo-STS Configuration
STS_DOMAIN=sts.example.com
TLS_MODE=on

# Old app ID that should be updated
GITHUB_APP_ID=99999

# Other settings
SOME_OTHER_VAR=keep-this
`
	if err := os.WriteFile(envPath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write existing .env: %v", err)
	}

	store := NewLocalEnvFileStore(envPath)

	creds := &AppCredentials{
		AppID:         12345,
		AppSlug:       "test-app",
		ClientID:      "Iv1.abc123",
		ClientSecret:  "secret123",
		WebhookSecret: "webhook-secret",
		PrivateKey:    "-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----",
	}

	if err := store.Save(context.Background(), creds); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Read the updated .env file
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("Failed to read .env file: %v", err)
	}

	contentStr := string(content)

	// Verify existing values are preserved
	if !strings.Contains(contentStr, "STS_DOMAIN=sts.example.com") {
		t.Error("Existing STS_DOMAIN value was not preserved")
	}
	if !strings.Contains(contentStr, "TLS_MODE=on") {
		t.Error("Existing TLS_MODE value was not preserved")
	}
	if !strings.Contains(contentStr, "SOME_OTHER_VAR=keep-this") {
		t.Error("Existing SOME_OTHER_VAR value was not preserved")
	}

	// Verify comments are preserved
	if !strings.Contains(contentStr, "# Octo-STS Configuration") {
		t.Error("Comment was not preserved")
	}

	// Verify GITHUB_APP_ID was updated (not the old value)
	if strings.Contains(contentStr, "GITHUB_APP_ID=99999") {
		t.Error("GITHUB_APP_ID was not updated from old value")
	}
	if !strings.Contains(contentStr, "GITHUB_APP_ID=12345") {
		t.Error("GITHUB_APP_ID was not set to new value")
	}

	// Verify new credentials were added
	if !strings.Contains(contentStr, "GITHUB_CLIENT_ID=Iv1.abc123") {
		t.Error("GITHUB_CLIENT_ID was not added")
	}
}

func TestLocalEnvFileStore_Save_CreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "nested", "dir", ".env")
	store := NewLocalEnvFileStore(envPath)

	creds := &AppCredentials{
		AppID:         12345,
		WebhookSecret: "secret",
		ClientID:      "client",
		ClientSecret:  "clientsecret",
		PrivateKey:    "-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----",
	}

	if err := store.Save(context.Background(), creds); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		t.Error(".env file was not created in nested directory")
	}
}

func TestLocalEnvFileStore_Save_STSDomainFromWebhookURL(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	store := NewLocalEnvFileStore(envPath)

	creds := &AppCredentials{
		AppID:         12345,
		WebhookSecret: "secret",
		ClientID:      "client",
		ClientSecret:  "clientsecret",
		PrivateKey:    "-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----",
		HookConfig:    HookConfig{URL: "https://sts.example.com/webhook"},
	}

	if err := store.Save(context.Background(), creds); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Read the .env file
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("Failed to read .env file: %v", err)
	}

	contentStr := string(content)

	// Verify STS_DOMAIN was extracted from webhook URL
	if !strings.Contains(contentStr, "STS_DOMAIN=sts.example.com") {
		t.Errorf("STS_DOMAIN not set correctly, got: %s", contentStr)
	}
}

func TestLocalEnvFileStore_Save_STSDomainPreservesExisting(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")

	// Create existing .env file with STS_DOMAIN already set (non-ngrok)
	existingContent := `STS_DOMAIN=existing.example.com
`
	if err := os.WriteFile(envPath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write existing .env: %v", err)
	}

	store := NewLocalEnvFileStore(envPath)

	creds := &AppCredentials{
		AppID:         12345,
		WebhookSecret: "secret",
		ClientID:      "client",
		ClientSecret:  "clientsecret",
		PrivateKey:    "-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----",
		HookConfig:    HookConfig{URL: "https://new.example.com/webhook"},
	}

	if err := store.Save(context.Background(), creds); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Read the .env file
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("Failed to read .env file: %v", err)
	}

	contentStr := string(content)

	// Verify existing STS_DOMAIN was preserved (not overwritten) for non-ngrok domains
	if !strings.Contains(contentStr, "STS_DOMAIN=existing.example.com") {
		t.Errorf("Existing STS_DOMAIN was overwritten, got: %s", contentStr)
	}
	if strings.Contains(contentStr, "STS_DOMAIN=new.example.com") {
		t.Error("STS_DOMAIN should not be overwritten when already set to non-ngrok domain")
	}
}

func TestLocalEnvFileStore_Save_STSDomainUpdatesNgrok(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")

	// Create existing .env file with old ngrok domain
	existingContent := `STS_DOMAIN=old123.ngrok-free.app
`
	if err := os.WriteFile(envPath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write existing .env: %v", err)
	}

	store := NewLocalEnvFileStore(envPath)

	creds := &AppCredentials{
		AppID:         12345,
		WebhookSecret: "secret",
		ClientID:      "client",
		ClientSecret:  "clientsecret",
		PrivateKey:    "-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----",
		HookConfig:    HookConfig{URL: "https://new456.ngrok-free.app/webhook"},
	}

	if err := store.Save(context.Background(), creds); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Read the .env file
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("Failed to read .env file: %v", err)
	}

	contentStr := string(content)

	// Verify ngrok domain was updated
	if strings.Contains(contentStr, "STS_DOMAIN=old123.ngrok-free.app") {
		t.Error("Old ngrok STS_DOMAIN should have been updated")
	}
	if !strings.Contains(contentStr, "STS_DOMAIN=new456.ngrok-free.app") {
		t.Errorf("STS_DOMAIN should be updated to new ngrok domain, got: %s", contentStr)
	}
}

func TestLocalEnvFileStore_Save_STSDomainUpdatesWhenNewIsNgrok(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")

	// Create existing .env file with non-ngrok domain
	existingContent := `STS_DOMAIN=prod.example.com
`
	if err := os.WriteFile(envPath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write existing .env: %v", err)
	}

	store := NewLocalEnvFileStore(envPath)

	creds := &AppCredentials{
		AppID:         12345,
		WebhookSecret: "secret",
		ClientID:      "client",
		ClientSecret:  "clientsecret",
		PrivateKey:    "-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----",
		HookConfig:    HookConfig{URL: "https://abc123.ngrok-free.app/webhook"},
	}

	if err := store.Save(context.Background(), creds); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Read the .env file
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("Failed to read .env file: %v", err)
	}

	contentStr := string(content)

	// Verify domain was updated when new webhook URL is ngrok
	if !strings.Contains(contentStr, "STS_DOMAIN=abc123.ngrok-free.app") {
		t.Errorf("STS_DOMAIN should be updated when new URL is ngrok, got: %s", contentStr)
	}
}

func TestParseEnvFile(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")

	content := `# Comment line
KEY1=value1
KEY2="quoted value"
KEY3='single quoted'

# Another comment
KEY4=value with spaces
`
	if err := os.WriteFile(envPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write .env: %v", err)
	}

	values, lines, err := parseEnvFile(envPath)
	if err != nil {
		t.Fatalf("parseEnvFile() error = %v", err)
	}

	// Verify values
	expectedValues := map[string]string{
		"KEY1": "value1",
		"KEY2": "quoted value",
		"KEY3": "single quoted",
		"KEY4": "value with spaces",
	}

	for key, expected := range expectedValues {
		if got := values[key]; got != expected {
			t.Errorf("values[%s] = %q, want %q", key, got, expected)
		}
	}

	// Verify lines were preserved (7 lines + trailing newline is not counted)
	if len(lines) != 7 {
		t.Errorf("len(lines) = %d, want 7", len(lines))
	}
}

func TestFormatEnvLine(t *testing.T) {
	tests := []struct {
		key      string
		value    string
		expected string
	}{
		{"KEY", "simple", "KEY=simple"},
		{"KEY", "with spaces", "KEY=\"with spaces\""},
		{"KEY", "with\\nnewline", "KEY=\"with\\nnewline\""},
		{"KEY", "with\"quote", "KEY=\"with\\\"quote\""},
	}

	for _, tt := range tests {
		got := formatEnvLine(tt.key, tt.value)
		if got != tt.expected {
			t.Errorf("formatEnvLine(%q, %q) = %q, want %q", tt.key, tt.value, got, tt.expected)
		}
	}
}

// Mock SSM Client for testing

type mockSSMClient struct {
	calls            []putParameterCall
	putParameterFunc func(ctx context.Context, params *ssm.PutParameterInput,
		optFns ...func(*ssm.Options)) (*ssm.PutParameterOutput, error)
}

type putParameterCall struct {
	Name      string
	Value     string
	Type      string
	Overwrite bool
	KeyID     *string
	Tags      []types.Tag
}

func (m *mockSSMClient) PutParameter(ctx context.Context, params *ssm.PutParameterInput,
	optFns ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
	// Record the call
	call := putParameterCall{
		Name:      aws.ToString(params.Name),
		Value:     aws.ToString(params.Value),
		Type:      string(params.Type),
		Overwrite: aws.ToBool(params.Overwrite),
		KeyID:     params.KeyId,
		Tags:      params.Tags,
	}
	m.calls = append(m.calls, call)

	// Use custom function if provided
	if m.putParameterFunc != nil {
		return m.putParameterFunc(ctx, params, optFns...)
	}

	// Default success response
	return &ssm.PutParameterOutput{
		Version: 1,
	}, nil
}

func (m *mockSSMClient) getCall(paramName string) *putParameterCall {
	for _, call := range m.calls {
		if strings.HasSuffix(call.Name, paramName) {
			return &call
		}
	}
	return nil
}

// AWSSSMStore Tests

func TestAWSSSMStore_Save_Success(t *testing.T) {
	mockClient := &mockSSMClient{}
	store, err := NewAWSSSMStore("/octo-sts/app/",
		WithSSMClient(mockClient),
	)
	if err != nil {
		t.Fatalf("NewAWSSSMStore() error = %v", err)
	}

	creds := &AppCredentials{
		AppID:         12345,
		AppSlug:       "test-app",
		ClientID:      "Iv1.abc123",
		ClientSecret:  "secret123",
		WebhookSecret: "webhook-secret",
		PrivateKey:    "-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----",
		STSDomain:     "sts.example.com",
		HTMLURL:       "https://github.com/apps/test-app",
	}

	if err := store.Save(context.Background(), creds); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify all 6 parameters were created (including STS_DOMAIN)
	if len(mockClient.calls) != 6 {
		t.Errorf("Expected 6 PutParameter calls, got %d", len(mockClient.calls))
	}

	// Verify each parameter
	tests := []struct {
		name     string
		expected string
	}{
		{EnvGitHubAppID, "12345"},
		{EnvGitHubWebhookSecret, "webhook-secret"},
		{EnvGitHubClientID, "Iv1.abc123"},
		{EnvGitHubClientSecret, "secret123"},
		{EnvAppSecretCert, "-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----"},
		{EnvSTSDomain, "sts.example.com"},
	}

	for _, tt := range tests {
		call := mockClient.getCall(tt.name)
		if call == nil {
			t.Errorf("Parameter %s was not created", tt.name)
			continue
		}

		// Verify parameter name includes prefix
		expectedName := "/octo-sts/app/" + tt.name
		if call.Name != expectedName {
			t.Errorf("Parameter name = %q, want %q", call.Name, expectedName)
		}

		// Verify value
		if call.Value != tt.expected {
			t.Errorf("Parameter %s value = %q, want %q", tt.name, call.Value, tt.expected)
		}

		// Verify type is SecureString
		if call.Type != string(types.ParameterTypeSecureString) {
			t.Errorf("Parameter %s type = %q, want %q", tt.name, call.Type, types.ParameterTypeSecureString)
		}

		// Verify overwrite is true
		if !call.Overwrite {
			t.Errorf("Parameter %s overwrite = false, want true", tt.name)
		}

		// Verify no KMS key (using default)
		if call.KeyID != nil {
			t.Errorf("Parameter %s has KMS key %q, expected nil (default key)", tt.name, *call.KeyID)
		}

		// Verify no tags
		if len(call.Tags) > 0 {
			t.Errorf("Parameter %s has tags, expected none", tt.name)
		}
	}
}

func TestAWSSSMStore_Save_WithoutSTSDomain(t *testing.T) {
	mockClient := &mockSSMClient{}
	store, err := NewAWSSSMStore("/octo-sts/app/",
		WithSSMClient(mockClient),
	)
	if err != nil {
		t.Fatalf("NewAWSSSMStore() error = %v", err)
	}

	creds := &AppCredentials{
		AppID:         12345,
		ClientID:      "Iv1.abc123",
		ClientSecret:  "secret123",
		WebhookSecret: "webhook-secret",
		PrivateKey:    "-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----",
		// STSDomain is empty
	}

	if err := store.Save(context.Background(), creds); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify only 5 parameters were created (excluding STS_DOMAIN)
	if len(mockClient.calls) != 5 {
		t.Errorf("Expected 5 PutParameter calls, got %d", len(mockClient.calls))
	}

	// Verify STS_DOMAIN was NOT created
	if call := mockClient.getCall(EnvSTSDomain); call != nil {
		t.Error("STS_DOMAIN parameter should not be created when empty")
	}
}

func TestAWSSSMStore_Save_WithCustomKMSKey(t *testing.T) {
	mockClient := &mockSSMClient{}
	kmsKeyID := "arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012"

	store, err := NewAWSSSMStore("/octo-sts/app/",
		WithSSMClient(mockClient),
		WithKMSKey(kmsKeyID),
	)
	if err != nil {
		t.Fatalf("NewAWSSSMStore() error = %v", err)
	}

	creds := &AppCredentials{
		AppID:         12345,
		ClientID:      "Iv1.abc123",
		ClientSecret:  "secret123",
		WebhookSecret: "webhook-secret",
		PrivateKey:    "-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----",
	}

	if err := store.Save(context.Background(), creds); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify all parameters use the custom KMS key
	for _, call := range mockClient.calls {
		if call.KeyID == nil {
			t.Errorf("Parameter %s missing KMS key ID", call.Name)
			continue
		}
		if *call.KeyID != kmsKeyID {
			t.Errorf("Parameter %s KMS key = %q, want %q", call.Name, *call.KeyID, kmsKeyID)
		}
	}
}

func TestAWSSSMStore_Save_WithTags(t *testing.T) {
	mockClient := &mockSSMClient{}
	tags := map[string]string{
		"Environment": "production",
		"Application": "octo-sts",
		"ManagedBy":   "terraform",
	}

	store, err := NewAWSSSMStore("/octo-sts/app/",
		WithSSMClient(mockClient),
		WithTags(tags),
	)
	if err != nil {
		t.Fatalf("NewAWSSSMStore() error = %v", err)
	}

	creds := &AppCredentials{
		AppID:         12345,
		ClientID:      "Iv1.abc123",
		ClientSecret:  "secret123",
		WebhookSecret: "webhook-secret",
		PrivateKey:    "-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----",
	}

	if err := store.Save(context.Background(), creds); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify all parameters have the tags
	for _, call := range mockClient.calls {
		if len(call.Tags) != len(tags) {
			t.Errorf("Parameter %s has %d tags, want %d", call.Name, len(call.Tags), len(tags))
			continue
		}

		// Convert tags to map for easier verification
		callTags := make(map[string]string)
		for _, tag := range call.Tags {
			callTags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
		}

		// Verify each tag
		for key, expectedValue := range tags {
			if actualValue, ok := callTags[key]; !ok {
				t.Errorf("Parameter %s missing tag %s", call.Name, key)
			} else if actualValue != expectedValue {
				t.Errorf("Parameter %s tag %s = %q, want %q", call.Name, key, actualValue, expectedValue)
			}
		}
	}
}

func TestAWSSSMStore_Save_PrefixNormalization(t *testing.T) {
	tests := []struct {
		name           string
		prefix         string
		expectedPrefix string
	}{
		{"with trailing slash", "/octo-sts/app/", "/octo-sts/app/"},
		{"without trailing slash", "/octo-sts/app", "/octo-sts/app/"},
		{"simple path with slash", "/app/", "/app/"},
		{"simple path without slash", "/app", "/app/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockSSMClient{}
			store, err := NewAWSSSMStore(tt.prefix,
				WithSSMClient(mockClient),
			)
			if err != nil {
				t.Fatalf("NewAWSSSMStore() error = %v", err)
			}

			creds := &AppCredentials{
				AppID:         12345,
				ClientID:      "Iv1.abc123",
				ClientSecret:  "secret123",
				WebhookSecret: "webhook-secret",
				PrivateKey:    "-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----",
			}

			if err := store.Save(context.Background(), creds); err != nil {
				t.Fatalf("Save() error = %v", err)
			}

			// Verify all parameters have the normalized prefix
			for _, call := range mockClient.calls {
				if !strings.HasPrefix(call.Name, tt.expectedPrefix) {
					t.Errorf("Parameter %s does not have expected prefix %s", call.Name, tt.expectedPrefix)
				}
			}
		})
	}
}

func TestAWSSSMStore_Save_MultilinePrivateKey(t *testing.T) {
	mockClient := &mockSSMClient{}
	store, err := NewAWSSSMStore("/octo-sts/app/",
		WithSSMClient(mockClient),
	)
	if err != nil {
		t.Fatalf("NewAWSSSMStore() error = %v", err)
	}

	multilinePEM := `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA1234567890abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMN
OPQRSTUVWXYZ1234567890abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQR
STUVWXYZ1234567890abcdefghijklmnopqrstuvwxyz
-----END RSA PRIVATE KEY-----`

	creds := &AppCredentials{
		AppID:         12345,
		ClientID:      "Iv1.abc123",
		ClientSecret:  "secret123",
		WebhookSecret: "webhook-secret",
		PrivateKey:    multilinePEM,
	}

	if err := store.Save(context.Background(), creds); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify private key is stored with original newlines (no escaping)
	call := mockClient.getCall(EnvAppSecretCert)
	if call == nil {
		t.Fatal("APP_SECRET_CERTIFICATE_ENV_VAR parameter not created")
	}

	if call.Value != multilinePEM {
		t.Errorf("Private key stored incorrectly.\nGot:\n%s\n\nWant:\n%s", call.Value, multilinePEM)
	}
}

func TestAWSSSMStore_Save_ErrorHandling(t *testing.T) {
	// Mock client that fails on the 3rd call
	callCount := 0
	mockClient := &mockSSMClient{
		putParameterFunc: func(ctx context.Context, params *ssm.PutParameterInput,
			optFns ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
			callCount++
			if callCount == 3 {
				return nil, fmt.Errorf("simulated AWS error")
			}
			return &ssm.PutParameterOutput{Version: 1}, nil
		},
	}

	store, err := NewAWSSSMStore("/octo-sts/app/",
		WithSSMClient(mockClient),
	)
	if err != nil {
		t.Fatalf("NewAWSSSMStore() error = %v", err)
	}

	creds := &AppCredentials{
		AppID:         12345,
		ClientID:      "Iv1.abc123",
		ClientSecret:  "secret123",
		WebhookSecret: "webhook-secret",
		PrivateKey:    "-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----",
	}

	err = store.Save(context.Background(), creds)
	if err == nil {
		t.Fatal("Save() should have returned an error")
	}

	if !strings.Contains(err.Error(), "failed to save parameter") {
		t.Errorf("Error message should contain 'failed to save parameter', got: %v", err)
	}

	// Verify operation stopped immediately after error (should be exactly 3 calls)
	if callCount != 3 {
		t.Errorf("Expected exactly 3 calls before stopping, got %d", callCount)
	}
}

func TestNewAWSSSMStore_EmptyPrefix(t *testing.T) {
	_, err := NewAWSSSMStore("")
	if err == nil {
		t.Fatal("NewAWSSSMStore() should return error for empty prefix")
	}

	if !strings.Contains(err.Error(), "prefix cannot be empty") {
		t.Errorf("Error message should mention empty prefix, got: %v", err)
	}
}

func TestNewAWSSSMStore_WithOptions(t *testing.T) {
	mockClient := &mockSSMClient{}
	kmsKey := "arn:aws:kms:us-east-1:123456789012:key/test"
	tags := map[string]string{
		"Environment": "test",
	}

	store, err := NewAWSSSMStore("/octo-sts/app/",
		WithSSMClient(mockClient),
		WithKMSKey(kmsKey),
		WithTags(tags),
	)
	if err != nil {
		t.Fatalf("NewAWSSSMStore() error = %v", err)
	}

	// Verify options were applied
	if store.KMSKeyID != kmsKey {
		t.Errorf("KMSKeyID = %q, want %q", store.KMSKeyID, kmsKey)
	}

	if len(store.Tags) != len(tags) {
		t.Errorf("Tags length = %d, want %d", len(store.Tags), len(tags))
	}

	if store.Tags["Environment"] != "test" {
		t.Errorf("Tags[Environment] = %q, want 'test'", store.Tags["Environment"])
	}

	if store.ssmClient != mockClient {
		t.Error("SSM client was not set correctly")
	}
}

func TestAWSSSMStore_Save_AllCredentialFields(t *testing.T) {
	mockClient := &mockSSMClient{}
	store, err := NewAWSSSMStore("/test/",
		WithSSMClient(mockClient),
	)
	if err != nil {
		t.Fatalf("NewAWSSSMStore() error = %v", err)
	}

	creds := &AppCredentials{
		AppID:         99999,
		AppSlug:       "my-app",
		ClientID:      "Iv1.xyz789",
		ClientSecret:  "super-secret",
		WebhookSecret: "hook-secret-123",
		PrivateKey:    "-----BEGIN PRIVATE KEY-----\nKEY_DATA\n-----END PRIVATE KEY-----",
		HTMLURL:       "https://github.com/apps/my-app",
		STSDomain:     "sts.production.com",
		HookConfig:    HookConfig{URL: "https://sts.production.com/webhook"},
	}

	if err := store.Save(context.Background(), creds); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify the actual values stored
	expectedValues := map[string]string{
		EnvGitHubAppID:         "99999",
		EnvGitHubWebhookSecret: "hook-secret-123",
		EnvGitHubClientID:      "Iv1.xyz789",
		EnvGitHubClientSecret:  "super-secret",
		EnvAppSecretCert:       "-----BEGIN PRIVATE KEY-----\nKEY_DATA\n-----END PRIVATE KEY-----",
		EnvSTSDomain:           "sts.production.com",
	}

	for name, expectedValue := range expectedValues {
		call := mockClient.getCall(name)
		if call == nil {
			t.Errorf("Parameter %s was not created", name)
			continue
		}
		if call.Value != expectedValue {
			t.Errorf("Parameter %s = %q, want %q", name, call.Value, expectedValue)
		}
	}
}

func TestNewFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "default envfile mode",
			envVars: map[string]string{},
			wantErr: false,
		},
		{
			name: "explicit envfile mode",
			envVars: map[string]string{
				EnvStorageMode: StorageModeEnvFile,
				EnvStorageDir:  "/tmp/test.env",
			},
			wantErr: false,
		},
		{
			name: "files mode",
			envVars: map[string]string{
				EnvStorageMode: StorageModeFiles,
				EnvStorageDir:  "/tmp/creds",
			},
			wantErr: false,
		},
		{
			name: "aws-ssm mode without prefix",
			envVars: map[string]string{
				EnvStorageMode: StorageModeAWSSSM,
			},
			wantErr: true,
			errMsg:  "AWS_SSM_PARAMETER_PREFIX is required",
		},
		{
			name: "unknown mode",
			envVars: map[string]string{
				EnvStorageMode: "invalid",
			},
			wantErr: true,
			errMsg:  "unknown STORAGE_MODE",
		},
		{
			name: "aws-ssm invalid tags JSON",
			envVars: map[string]string{
				EnvStorageMode:        StorageModeAWSSSM,
				EnvAWSSSMParameterPfx: "/test/",
				EnvAWSSSMTags:         "not-valid-json",
			},
			wantErr: true,
			errMsg:  "failed to parse AWS_SSM_TAGS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear relevant env vars
			for _, key := range []string{EnvStorageMode, EnvStorageDir, EnvAWSSSMParameterPfx, EnvAWSSSMKMSKeyID, EnvAWSSSMTags} {
				os.Unsetenv(key)
			}

			// Set test env vars
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			store, err := NewFromEnv()

			if tt.wantErr {
				if err == nil {
					t.Error("NewFromEnv() expected error, got nil")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("NewFromEnv() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("NewFromEnv() unexpected error = %v", err)
				}
				if store == nil {
					t.Error("NewFromEnv() returned nil store")
				}
			}
		})
	}
}

func TestInstallerEnabled(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     bool
	}{
		{"true lowercase", "true", true},
		{"TRUE uppercase", "TRUE", true},
		{"True mixed", "True", true},
		{"1", "1", true},
		{"yes", "yes", true},
		{"YES", "YES", true},
		{"false", "false", false},
		{"0", "0", false},
		{"no", "no", false},
		{"empty", "", false},
		{"random", "enabled", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv(EnvInstallerEnabled, tt.envValue)
			defer os.Unsetenv(EnvInstallerEnabled)

			if got := InstallerEnabled(); got != tt.want {
				t.Errorf("InstallerEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}
