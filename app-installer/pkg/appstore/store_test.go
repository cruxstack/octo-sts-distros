// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

package appstore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
