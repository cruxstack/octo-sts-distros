// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

package installer

import (
	"context"
	"testing"

	"github.com/cruxstack/github-app-setup-go/configstore"
)

type recordingStore struct {
	configstore.Store
	saved *configstore.AppCredentials
}

func (s *recordingStore) Save(_ context.Context, creds *configstore.AppCredentials) error {
	s.saved = creds
	return nil
}

func TestNewOctoSTSConfigMapsCustomDomainBeforeSave(t *testing.T) {
	tests := []struct {
		name         string
		customFields map[string]string
		wantDomain   string
	}{
		{
			name:         "custom domain is copied to STS_DOMAIN",
			customFields: map[string]string{"CUSTOM_DOMAIN": "sts.example.com"},
			wantDomain:   "sts.example.com",
		},
		{
			name:         "empty custom domain leaves STS_DOMAIN unset",
			customFields: map[string]string{"CUSTOM_DOMAIN": ""},
		},
		{
			name: "nil custom fields are tolerated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inner := &recordingStore{}
			cfg := NewOctoSTSConfig(inner)

			creds := &configstore.AppCredentials{AppID: 1, CustomFields: tt.customFields}
			if err := cfg.Store.Save(context.Background(), creds); err != nil {
				t.Fatalf("Save() error = %v", err)
			}

			if inner.saved != creds {
				t.Fatalf("wrapped store did not receive the credentials")
			}
			if got := inner.saved.CustomFields["STS_DOMAIN"]; got != tt.wantDomain {
				t.Errorf("STS_DOMAIN = %q, want %q", got, tt.wantDomain)
			}
		})
	}
}
