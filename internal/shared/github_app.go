// Copyright 2026 CruxStack
// SPDX-License-Identifier: MIT

package shared

import (
	"fmt"
	"strings"

	envConfig "github.com/octo-sts/app/pkg/envconfig"
)

// PrimaryGitHubApp returns the first configured app ID and KMS key.
func PrimaryGitHubApp(env *envConfig.EnvConfig) (int64, string, error) {
	if env == nil {
		return 0, "", fmt.Errorf("missing env config")
	}

	if len(env.AppIDs) == 0 {
		return 0, "", fmt.Errorf("no GitHub app IDs configured")
	}

	appID := env.AppIDs[0]
	kmsKey := ""
	if len(env.KMSKeys) > 0 {
		kmsKey = strings.TrimSpace(env.KMSKeys[0])
	}

	return appID, kmsKey, nil
}
