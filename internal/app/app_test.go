// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

package app

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"path/filepath"
	"testing"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/chainguard-dev/clog"
	"github.com/chainguard-dev/clog/slogtest"
	"github.com/google/go-github/v75/github"

	"github.com/cruxstack/octo-sts-distros/internal/shared"
)

func TestNew(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tr := ghinstallation.NewAppsTransportFromPrivateKey(http.DefaultTransport, 1234, key)

	tests := []struct {
		name      string
		transport *ghinstallation.AppsTransport
		config    Config
		wantErr   bool
	}{
		{
			name:      "valid config",
			transport: tr,
			config: Config{
				WebhookSecrets: [][]byte{[]byte("secret")},
			},
			wantErr: false,
		},
		{
			name:      "nil transport",
			transport: nil,
			config: Config{
				WebhookSecrets: [][]byte{[]byte("secret")},
			},
			wantErr: true,
		},
		{
			name:      "no webhook secrets",
			transport: tr,
			config: Config{
				WebhookSecrets: [][]byte{},
			},
			wantErr: true,
		},
		{
			name:      "with organizations filter",
			transport: tr,
			config: Config{
				WebhookSecrets: [][]byte{[]byte("secret")},
				Organizations:  []string{"org1", "org2"},
			},
			wantErr: false,
		},
		{
			name:      "with base path",
			transport: tr,
			config: Config{
				WebhookSecrets: [][]byte{[]byte("secret")},
				BasePath:       "/webhook/",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, err := New(tt.transport, tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && app == nil {
				t.Error("New() returned nil app without error")
			}
		})
	}
}

func TestNormalizeHeaders(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		expected map[string]string
	}{
		{
			name:     "empty headers",
			input:    map[string]string{},
			expected: map[string]string{},
		},
		{
			name: "mixed case headers",
			input: map[string]string{
				"Content-Type":      "application/json",
				"X-GitHub-Event":    "push",
				"X-HUB-SIGNATURE":   "sha256=abc",
				"x-github-delivery": "123",
			},
			expected: map[string]string{
				"content-type":      "application/json",
				"x-github-event":    "push",
				"x-hub-signature":   "sha256=abc",
				"x-github-delivery": "123",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shared.NormalizeHeaders(tt.input)
			if len(got) != len(tt.expected) {
				t.Errorf("NormalizeHeaders() returned %d headers, expected %d", len(got), len(tt.expected))
			}
			for k, v := range tt.expected {
				if got[k] != v {
					t.Errorf("NormalizeHeaders()[%q] = %q, expected %q", k, got[k], v)
				}
			}
		})
	}
}

func TestStripBasePath(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tr := ghinstallation.NewAppsTransportFromPrivateKey(http.DefaultTransport, 1234, key)

	tests := []struct {
		name     string
		basePath string
		path     string
		expected string
	}{
		{
			name:     "no base path",
			basePath: "",
			path:     "/webhook",
			expected: "/webhook",
		},
		{
			name:     "with base path",
			basePath: "/webhook",
			path:     "/webhook/foo",
			expected: "/foo",
		},
		{
			name:     "base path with trailing slash",
			basePath: "/webhook/",
			path:     "/webhook/foo",
			expected: "/foo",
		},
		{
			name:     "path equals base path",
			basePath: "/webhook",
			path:     "/webhook",
			expected: "/",
		},
		{
			name:     "path without base path prefix",
			basePath: "/webhook",
			path:     "/other",
			expected: "/other",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, err := New(tr, Config{
				WebhookSecrets: [][]byte{[]byte("secret")},
				BasePath:       tt.basePath,
			})
			if err != nil {
				t.Fatal(err)
			}
			got := app.stripBasePath(tt.path)
			if got != tt.expected {
				t.Errorf("stripBasePath(%q) = %q, expected %q", tt.path, got, tt.expected)
			}
		})
	}
}

func TestHandleRequestRouting(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tr := ghinstallation.NewAppsTransportFromPrivateKey(http.DefaultTransport, 1234, key)

	app, err := New(tr, Config{
		WebhookSecrets: [][]byte{[]byte("secret")},
	})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name           string
		request        shared.Request
		expectedStatus int
	}{
		{
			name: "GET request returns 404",
			request: shared.Request{
				Type:    shared.RequestTypeHTTP,
				Method:  http.MethodGet,
				Path:    "/",
				Headers: map[string]string{},
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name: "POST to /other returns 404",
			request: shared.Request{
				Type:    shared.RequestTypeHTTP,
				Method:  http.MethodPost,
				Path:    "/other",
				Headers: map[string]string{},
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name: "POST to / without signature returns 400",
			request: shared.Request{
				Type:    shared.RequestTypeHTTP,
				Method:  http.MethodPost,
				Path:    "/",
				Headers: map[string]string{},
				Body:    []byte("{}"),
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	ctx := slogtest.Context(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := app.HandleRequest(ctx, tt.request)
			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("HandleRequest() status = %d, expected %d", resp.StatusCode, tt.expectedStatus)
			}
		})
	}
}

func TestResponseHelpers(t *testing.T) {
	t.Run("OKResponse", func(t *testing.T) {
		resp := OKResponse()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("OKResponse().StatusCode = %d, expected %d", resp.StatusCode, http.StatusOK)
		}
	})

	t.Run("AcceptedResponse", func(t *testing.T) {
		resp := AcceptedResponse()
		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("AcceptedResponse().StatusCode = %d, expected %d", resp.StatusCode, http.StatusAccepted)
		}
	})

	t.Run("ErrorResponse", func(t *testing.T) {
		resp := ErrorResponse(http.StatusBadRequest, "bad request")
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("ErrorResponse().StatusCode = %d, expected %d", resp.StatusCode, http.StatusBadRequest)
		}
		if string(resp.Body) != "bad request" {
			t.Errorf("ErrorResponse().Body = %q, expected %q", string(resp.Body), "bad request")
		}
	})

	t.Run("NewResponse", func(t *testing.T) {
		resp := NewResponse(http.StatusCreated, []byte("created"))
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("NewResponse().StatusCode = %d, expected %d", resp.StatusCode, http.StatusCreated)
		}
		if string(resp.Body) != "created" {
			t.Errorf("NewResponse().Body = %q, expected %q", string(resp.Body), "created")
		}
	})
}

func TestOrgFilter(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "should not be called", http.StatusUnauthorized)
	}))
	defer gh.Close()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tr := ghinstallation.NewAppsTransportFromPrivateKey(gh.Client().Transport, 1234, key)
	tr.BaseURL = gh.URL

	secret := []byte("hunter2")
	app, err := New(tr, Config{
		WebhookSecrets: [][]byte{secret},
		Organizations:  []string{"foo"},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		org  string
		code int
	}{
		// This fails because the organization is in the filter, so we try to resolve it but it's pointed at a no-op github backend.
		{"foo", http.StatusInternalServerError},
		// This passes because the organization is not in the filter, so the server will fast-return a 200.
		{"bar", http.StatusOK},
	} {
		t.Run(tc.org, func(t *testing.T) {
			body, err := json.Marshal(github.PushEvent{
				Organization: &github.Organization{
					Login: github.Ptr(tc.org),
				},
				Repo: &github.PushEventRepository{
					Owner: &github.User{
						Login: github.Ptr(tc.org),
					},
				},
			})
			if err != nil {
				t.Fatal(err)
			}

			req := shared.Request{
				Type:   shared.RequestTypeHTTP,
				Method: http.MethodPost,
				Path:   "/",
				Headers: shared.NormalizeHeaders(map[string]string{
					"X-Hub-Signature": signature(secret, body),
					"X-GitHub-Event":  "push",
					"Content-Type":    "application/json",
				}),
				Body: body,
			}

			resp := app.HandleRequest(slogtest.Context(t), req)
			if resp.StatusCode != tc.code {
				t.Fatalf("expected %d, got %d: %s", tc.code, resp.StatusCode, string(resp.Body))
			}
		})
	}
}

func signature(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	b := mac.Sum(nil)

	return fmt.Sprintf("sha256=%s", hex.EncodeToString(b))
}

func TestWebhookOK(t *testing.T) {
	// CheckRuns will be collected here.
	got := []*github.CreateCheckRunOptions{}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v3/repos/foo/bar/check-runs", func(w http.ResponseWriter, r *http.Request) {
		opt := new(github.CreateCheckRunOptions)
		if err := json.NewDecoder(r.Body).Decode(opt); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		got = append(got, opt)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Serve testdata from local testdata directory
		path := filepath.Join("testdata", r.URL.Path)
		f, err := os.Open(path)
		if err != nil {
			clog.FromContext(r.Context()).Errorf("%s not found", path)
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		defer f.Close()
		if _, err := io.Copy(w, f); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	gh := httptest.NewServer(mux)
	defer gh.Close()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tr := ghinstallation.NewAppsTransportFromPrivateKey(gh.Client().Transport, 1234, key)
	tr.BaseURL = gh.URL

	secret := []byte("hunter2")
	app, err := New(tr, Config{
		WebhookSecrets: [][]byte{secret},
	})
	if err != nil {
		t.Fatal(err)
	}

	body, err := json.Marshal(github.PushEvent{
		Installation: &github.Installation{
			ID: github.Ptr(int64(1111)),
		},
		Organization: &github.Organization{
			Login: github.Ptr("foo"),
		},
		Repo: &github.PushEventRepository{
			Owner: &github.User{
				Login: github.Ptr("foo"),
			},
			Name: github.Ptr("bar"),
		},
		Before: github.Ptr("1234"),
		After:  github.Ptr("5678"),
	})
	if err != nil {
		t.Fatal(err)
	}

	req := shared.Request{
		Type:   shared.RequestTypeHTTP,
		Method: http.MethodPost,
		Path:   "/",
		Headers: shared.NormalizeHeaders(map[string]string{
			"X-Hub-Signature": signature(secret, body),
			"X-GitHub-Event":  "push",
			"Content-Type":    "application/json",
		}),
		Body: body,
	}

	resp := app.HandleRequest(slogtest.Context(t), req)
	if resp.StatusCode != http.StatusOK {
		out, _ := httputil.DumpResponse(&http.Response{StatusCode: resp.StatusCode, Body: io.NopCloser(bytes.NewReader(resp.Body))}, true)
		t.Fatalf("expected %d, got\n%s", http.StatusOK, string(out))
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 check run, got %d", len(got))
	}

	if got[0].Name != "Trust Policy Validation" {
		t.Errorf("expected check run name 'Trust Policy Validation', got %q", got[0].Name)
	}
	if got[0].HeadSHA != "5678" {
		t.Errorf("expected head SHA '5678', got %q", got[0].HeadSHA)
	}
	if *got[0].Conclusion != "success" {
		t.Errorf("expected conclusion 'success', got %q", *got[0].Conclusion)
	}
}

func TestWebhookWithBasePath(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "should not be called", http.StatusUnauthorized)
	}))
	defer gh.Close()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tr := ghinstallation.NewAppsTransportFromPrivateKey(gh.Client().Transport, 1234, key)
	tr.BaseURL = gh.URL

	secret := []byte("hunter2")
	app, err := New(tr, Config{
		WebhookSecrets: [][]byte{secret},
		BasePath:       "/webhook",
		Organizations:  []string{"foo"}, // Use org filter to distinguish 200 vs error
	})
	if err != nil {
		t.Fatal(err)
	}

	body, err := json.Marshal(github.PushEvent{
		Organization: &github.Organization{
			Login: github.Ptr("bar"), // Not in org filter, so should return 200
		},
		Repo: &github.PushEventRepository{
			Owner: &github.User{
				Login: github.Ptr("bar"),
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Request with base path prefix should work
	req := shared.Request{
		Type:   shared.RequestTypeHTTP,
		Method: http.MethodPost,
		Path:   "/webhook",
		Headers: shared.NormalizeHeaders(map[string]string{
			"X-Hub-Signature": signature(secret, body),
			"X-GitHub-Event":  "push",
			"Content-Type":    "application/json",
		}),
		Body: body,
	}

	resp := app.HandleRequest(slogtest.Context(t), req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected %d for /webhook, got %d: %s", http.StatusOK, resp.StatusCode, string(resp.Body))
	}
}

func TestMultipleWebhookSecrets(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// No-op server
	}))
	defer gh.Close()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tr := ghinstallation.NewAppsTransportFromPrivateKey(gh.Client().Transport, 1234, key)
	tr.BaseURL = gh.URL

	secret1 := []byte("old-secret")
	secret2 := []byte("new-secret")

	app, err := New(tr, Config{
		WebhookSecrets: [][]byte{secret1, secret2},
		Organizations:  []string{}, // Empty = allow all, will return 200 for skipped orgs
	})
	if err != nil {
		t.Fatal(err)
	}

	body, err := json.Marshal(github.PushEvent{
		Organization: &github.Organization{
			Login: github.Ptr("test-org"),
		},
		Repo: &github.PushEventRepository{
			Owner: &github.User{
				Login: github.Ptr("test-org"),
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Test with first secret
	req1 := shared.Request{
		Type:   shared.RequestTypeHTTP,
		Method: http.MethodPost,
		Path:   "/",
		Headers: shared.NormalizeHeaders(map[string]string{
			"X-Hub-Signature": signature(secret1, body),
			"X-GitHub-Event":  "push",
			"Content-Type":    "application/json",
		}),
		Body: body,
	}

	resp1 := app.HandleRequest(slogtest.Context(t), req1)
	// Will fail at GitHub API call since we have a no-op server, but not at signature validation
	// The 500 error indicates it passed signature validation and tried to call GitHub
	if resp1.StatusCode == http.StatusBadRequest {
		t.Error("signature validation failed with first secret")
	}

	// Test with second secret
	req2 := shared.Request{
		Type:   shared.RequestTypeHTTP,
		Method: http.MethodPost,
		Path:   "/",
		Headers: shared.NormalizeHeaders(map[string]string{
			"X-Hub-Signature": signature(secret2, body),
			"X-GitHub-Event":  "push",
			"Content-Type":    "application/json",
		}),
		Body: body,
	}

	resp2 := app.HandleRequest(slogtest.Context(t), req2)
	if resp2.StatusCode == http.StatusBadRequest {
		t.Error("signature validation failed with second secret")
	}

	// Test with invalid secret
	req3 := shared.Request{
		Type:   shared.RequestTypeHTTP,
		Method: http.MethodPost,
		Path:   "/",
		Headers: shared.NormalizeHeaders(map[string]string{
			"X-Hub-Signature": signature([]byte("wrong-secret"), body),
			"X-GitHub-Event":  "push",
			"Content-Type":    "application/json",
		}),
		Body: body,
	}

	resp3 := app.HandleRequest(slogtest.Context(t), req3)
	if resp3.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid signature, got %d", resp3.StatusCode)
	}
}
