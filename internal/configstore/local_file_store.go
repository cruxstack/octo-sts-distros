// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

package configstore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// LocalFileStore saves credentials as individual files in a directory.
type LocalFileStore struct {
	Dir string
}

// NewLocalFileStore creates a new LocalFileStore that saves credentials
// as individual files in the specified directory.
func NewLocalFileStore(dir string) *LocalFileStore {
	return &LocalFileStore{Dir: dir}
}

// Save writes credentials to individual files (app-id, private-key.pem, webhook-secret, client-id, client-secret).
func (s *LocalFileStore) Save(ctx context.Context, creds *AppCredentials) error {
	if err := os.MkdirAll(s.Dir, 0700); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", s.Dir, err)
	}

	files := map[string]struct {
		content string
		mode    os.FileMode
	}{
		"app-id":          {content: fmt.Sprintf("%d", creds.AppID), mode: 0644},
		"private-key.pem": {content: creds.PrivateKey, mode: 0600},
		"webhook-secret":  {content: creds.WebhookSecret, mode: 0600},
		"client-id":       {content: creds.ClientID, mode: 0644},
		"client-secret":   {content: creds.ClientSecret, mode: 0600},
	}

	for name, file := range files {
		path := filepath.Join(s.Dir, name)
		if err := os.WriteFile(path, []byte(file.content), file.mode); err != nil {
			return fmt.Errorf("failed to write %s: %w", path, err)
		}
	}

	return nil
}
