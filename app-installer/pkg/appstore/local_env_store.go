// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

package appstore

import (
	"bufio"
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// LocalEnvFileStore saves credentials to a .env file, preserving existing content.
type LocalEnvFileStore struct {
	FilePath string
}

func NewLocalEnvFileStore(filepath string) *LocalEnvFileStore {
	return &LocalEnvFileStore{FilePath: filepath}
}

// Save writes credentials to .env format with all GitHub App values and STS_DOMAIN.
func (s *LocalEnvFileStore) Save(ctx context.Context, creds *AppCredentials) error {
	dir := filepath.Dir(s.FilePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	existingValues, originalLines, err := parseEnvFile(s.FilePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read existing .env file: %w", err)
	}
	if existingValues == nil {
		existingValues = make(map[string]string)
	}

	if creds.STSDomain != "" {
		existingValues[EnvSTSDomain] = creds.STSDomain
	} else if creds.HookConfig.URL != "" {
		// Auto-update STS_DOMAIN from webhook URL if blank or contains ngrok domain
		if parsedURL, err := url.Parse(creds.HookConfig.URL); err == nil && parsedURL.Host != "" {
			newHost := parsedURL.Host
			existingHost := existingValues[EnvSTSDomain]
			isNewNgrok := strings.Contains(newHost, "ngrok-free.app") || strings.Contains(newHost, "ngrok.io")
			isExistingNgrok := strings.Contains(existingHost, "ngrok-free.app") || strings.Contains(existingHost, "ngrok.io")

			if existingHost == "" || isNewNgrok || isExistingNgrok {
				existingValues[EnvSTSDomain] = newHost
			}
		}
	}

	singleLinePEM := strings.ReplaceAll(creds.PrivateKey, "\n", "\\n")

	existingValues[EnvGitHubAppID] = fmt.Sprintf("%d", creds.AppID)
	existingValues[EnvGitHubWebhookSecret] = creds.WebhookSecret
	existingValues[EnvGitHubClientID] = creds.ClientID
	existingValues[EnvGitHubClientSecret] = creds.ClientSecret
	existingValues[EnvAppSecretCert] = singleLinePEM

	if err := writeEnvFile(s.FilePath, existingValues, originalLines); err != nil {
		return fmt.Errorf("failed to write .env file: %w", err)
	}

	return nil
}

func parseEnvFile(path string) (map[string]string, []string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	values := make(map[string]string)
	var lines []string

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)

		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		idx := strings.Index(line, "=")
		if idx == -1 {
			continue
		}

		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])

		if len(value) >= 2 {
			if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
				(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
				value = value[1 : len(value)-1]
			}
		}

		values[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}

	return values, lines, nil
}

func writeEnvFile(path string, values map[string]string, originalLines []string) error {
	var outputLines []string
	writtenKeys := make(map[string]bool)

	for _, line := range originalLines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			outputLines = append(outputLines, line)
			continue
		}

		idx := strings.Index(line, "=")
		if idx == -1 {
			outputLines = append(outputLines, line)
			continue
		}

		key := strings.TrimSpace(line[:idx])

		if newValue, ok := values[key]; ok {
			outputLines = append(outputLines, formatEnvLine(key, newValue))
			writtenKeys[key] = true
		} else {
			outputLines = append(outputLines, line)
		}
	}

	for key, value := range values {
		if !writtenKeys[key] {
			outputLines = append(outputLines, formatEnvLine(key, value))
		}
	}

	content := strings.Join(outputLines, "\n")
	if len(outputLines) > 0 {
		content += "\n"
	}

	return os.WriteFile(path, []byte(content), 0600)
}

func formatEnvLine(key, value string) string {
	needsQuotes := strings.ContainsAny(value, " \t\n\r\"'\\#") || strings.Contains(value, "\\n")

	if needsQuotes {
		escaped := strings.ReplaceAll(value, "\"", "\\\"")
		return fmt.Sprintf("%s=\"%s\"", key, escaped)
	}

	return fmt.Sprintf("%s=%s", key, value)
}
