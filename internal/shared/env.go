// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

package shared

import (
	"bufio"
	"os"
	"strings"
)

// GetEnvDefault returns the value of an environment variable,
// or the default value if the variable is not set or empty.
func GetEnvDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

// LoadEnvFile loads env vars from a file, only setting values that aren't already set.
func LoadEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet, not an error
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		idx := strings.Index(line, "=")
		if idx == -1 {
			continue
		}

		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])

		// Remove surrounding quotes if present
		if len(value) >= 2 {
			if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
				(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
				value = value[1 : len(value)-1]
			}
		}

		// Set if current env is empty and file has a value
		// This allows hot-reload to pick up newly saved credentials
		if value != "" && os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}

	return scanner.Err()
}

// SetupEnvMapping maps GITHUB_APP_PRIVATE_KEY to APP_SECRET_CERTIFICATE_ENV_VAR and handles escaped newlines.
func SetupEnvMapping() {
	// First, try to load from .env file if STORAGE_DIR is set (for hot-reload support)
	if storageDir := os.Getenv("STORAGE_DIR"); storageDir != "" {
		_ = LoadEnvFile(storageDir) // Ignore errors, file may not exist yet
	}

	// If GITHUB_APP_PRIVATE_KEY is set, copy it to APP_SECRET_CERTIFICATE_ENV_VAR
	// (which is what the upstream library reads)
	if pk := os.Getenv("GITHUB_APP_PRIVATE_KEY"); pk != "" {
		// Convert escaped newlines (literal \n) to actual newlines.
		// This is needed because the configstore's envfile format escapes
		// newlines when saving PEM keys to .env files.
		pk = strings.ReplaceAll(pk, "\\n", "\n")
		os.Setenv("APP_SECRET_CERTIFICATE_ENV_VAR", pk)
	}
}
