// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

package shared

import "os"

// GetEnvDefault returns the value of an environment variable,
// or the default value if the variable is not set or empty.
func GetEnvDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
