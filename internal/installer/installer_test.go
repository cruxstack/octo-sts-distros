// SPDX-License-Identifier: MIT

package installer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cruxstack/octo-sts-distros/internal/configstore"
)

type fakeStore struct {
	status        *configstore.InstallerStatus
	disabled      bool
	disableErr    error
	lastSavedCred *configstore.AppCredentials
}

func (f *fakeStore) Save(ctx context.Context, creds *configstore.AppCredentials) error {
	f.lastSavedCred = creds
	return nil
}

func (f *fakeStore) Status(ctx context.Context) (*configstore.InstallerStatus, error) {
	if f.status == nil {
		return &configstore.InstallerStatus{}, nil
	}
	return f.status, nil
}

func (f *fakeStore) DisableInstaller(ctx context.Context) error {
	if f.disableErr != nil {
		return f.disableErr
	}
	f.disabled = true
	if f.status != nil {
		f.status.InstallerDisabled = true
	}
	return nil
}

func TestHandlerHandleIndexShowsSuccessWhenRegistered(t *testing.T) {
	store := &fakeStore{
		status: &configstore.InstallerStatus{
			Registered: true,
			AppID:      12345,
			AppSlug:    "test-app",
			HTMLURL:    "https://github.com/apps/test-app",
		},
	}

	handler, err := New(Config{Store: store, GitHubURL: "https://github.com"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/setup", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Status code = %d, want 200", rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "Disable Web Installer") {
		t.Errorf("response body missing disable panel: %s", body)
	}
	if !strings.Contains(body, "test-app") {
		t.Errorf("response body missing app slug: %s", body)
	}
	if strings.Contains(body, "Create GitHub App") {
		t.Errorf("installer form should be hidden when already registered")
	}
}

func TestHandleDisablePersistsAndRedirects(t *testing.T) {
	store := &fakeStore{status: &configstore.InstallerStatus{Registered: true}}

	handler, err := New(Config{Store: store})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/setup/disable", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("Status code = %d, want %d", rr.Code, http.StatusSeeOther)
	}

	if loc := rr.Header().Get("Location"); loc != "/healthz" {
		t.Fatalf("redirect location = %q, want /healthz", loc)
	}

	if !store.disabled {
		t.Fatal("DisableInstaller was not called on the store")
	}
}

func TestHandleIndexHidesAppIDWhenDisabled(t *testing.T) {
	store := &fakeStore{
		status: &configstore.InstallerStatus{
			Registered:        true,
			InstallerDisabled: true,
			AppID:             9999,
			AppSlug:           "disabled-app",
			HTMLURL:           "https://github.com/apps/disabled-app",
		},
	}

	handler, err := New(Config{Store: store, GitHubURL: "https://github.com"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/setup", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Status code = %d, want 200", rr.Code)
	}

	body := rr.Body.String()
	if strings.Contains(body, "App ID") {
		t.Fatalf("App ID should not be shown when installer disabled: %s", body)
	}
	if !strings.Contains(body, "App URL") {
		t.Fatalf("App URL should still be shown when available: %s", body)
	}
}

func TestHandleRootRedirectsWhenEnabled(t *testing.T) {
	store := &fakeStore{}

	handler, err := New(Config{Store: store})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("Status code = %d, want %d", rr.Code, http.StatusFound)
	}

	if loc := rr.Header().Get("Location"); loc != "/setup" {
		t.Fatalf("redirect location = %q, want /setup", loc)
	}
}

func TestHandleRootReturnsNotFoundWhenDisabled(t *testing.T) {
	store := &fakeStore{status: &configstore.InstallerStatus{InstallerDisabled: true}}

	handler, err := New(Config{Store: store})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("Status code = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestHandleDisableRequiresRegistration(t *testing.T) {
	// Store with no registration
	store := &fakeStore{}

	handler, err := New(Config{Store: store})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/setup/disable", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("Status code = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	if store.disabled {
		t.Fatal("DisableInstaller should not be called when app is not registered")
	}
}
