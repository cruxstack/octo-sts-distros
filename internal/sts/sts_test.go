// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

package sts

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/chainguard-dev/clog/slogtest"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	josejwt "github.com/go-jose/go-jose/v4/jwt"
	jwt "github.com/golang-jwt/jwt/v4"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-github/v75/github"

	"github.com/cruxstack/octo-sts-distros/internal/shared"
	"github.com/octo-sts/app/pkg/provider"
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
				Domain: "sts.example.com",
			},
			wantErr: false,
		},
		{
			name:      "nil transport",
			transport: nil,
			config: Config{
				Domain: "sts.example.com",
			},
			wantErr: true,
		},
		{
			name:      "empty domain",
			transport: tr,
			config: Config{
				Domain: "",
			},
			wantErr: true,
		},
		{
			name:      "with base path",
			transport: tr,
			config: Config{
				Domain:   "sts.example.com",
				BasePath: "/sts/",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sts, err := New(tt.transport, tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && sts == nil {
				t.Error("New() returned nil sts without error")
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
				"Content-Type":  "application/json",
				"Authorization": "Bearer token",
				"X-Custom":      "value",
			},
			expected: map[string]string{
				"content-type":  "application/json",
				"authorization": "Bearer token",
				"x-custom":      "value",
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
			path:     "/exchange",
			expected: "/exchange",
		},
		{
			name:     "with base path",
			basePath: "/sts",
			path:     "/sts/exchange",
			expected: "/exchange",
		},
		{
			name:     "base path with trailing slash",
			basePath: "/sts/",
			path:     "/sts/exchange",
			expected: "/exchange",
		},
		{
			name:     "path equals base path",
			basePath: "/sts",
			path:     "/sts",
			expected: "/",
		},
		{
			name:     "path without base path prefix",
			basePath: "/sts",
			path:     "/other",
			expected: "/other",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sts, err := New(tr, Config{
				Domain:   "sts.example.com",
				BasePath: tt.basePath,
			})
			if err != nil {
				t.Fatal(err)
			}
			got := sts.stripBasePath(tt.path)
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

	sts, err := New(tr, Config{
		Domain: "sts.example.com",
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
			name: "GET request to root returns 200",
			request: shared.Request{
				Type:    shared.RequestTypeHTTP,
				Method:  http.MethodGet,
				Path:    "/",
				Headers: map[string]string{},
			},
			expectedStatus: http.StatusOK,
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
			name: "POST to / without auth returns 401",
			request: shared.Request{
				Type:    shared.RequestTypeHTTP,
				Method:  http.MethodPost,
				Path:    "/",
				Headers: map[string]string{},
				Body:    []byte(`{"identity": "test", "scope": "org/repo"}`),
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "POST to / with invalid body returns 400",
			request: shared.Request{
				Type:   shared.RequestTypeHTTP,
				Method: http.MethodPost,
				Path:   "/",
				Headers: map[string]string{
					HeaderAuthorization: "Bearer invalid",
				},
				Body: []byte("not json"),
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	ctx := slogtest.Context(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := sts.HandleRequest(ctx, tt.request)
			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("HandleRequest() status = %d, expected %d, body = %s", resp.StatusCode, tt.expectedStatus, string(resp.Body))
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

	t.Run("ErrorResponse", func(t *testing.T) {
		resp := ErrorResponse(http.StatusBadRequest, "bad request")
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("ErrorResponse().StatusCode = %d, expected %d", resp.StatusCode, http.StatusBadRequest)
		}
		var errBody ErrorResponseBody
		if err := json.Unmarshal(resp.Body, &errBody); err != nil {
			t.Fatalf("failed to unmarshal error response: %v", err)
		}
		if errBody.Error != "bad request" {
			t.Errorf("ErrorResponse().Body.Error = %q, expected %q", errBody.Error, "bad request")
		}
	})

	t.Run("JSONResponse", func(t *testing.T) {
		resp := JSONResponse(http.StatusOK, ExchangeResponse{Token: "test-token"})
		if resp.StatusCode != http.StatusOK {
			t.Errorf("JSONResponse().StatusCode = %d, expected %d", resp.StatusCode, http.StatusOK)
		}
		var exchangeResp ExchangeResponse
		if err := json.Unmarshal(resp.Body, &exchangeResp); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		if exchangeResp.Token != "test-token" {
			t.Errorf("JSONResponse().Body.Token = %q, expected %q", exchangeResp.Token, "test-token")
		}
	})
}

func TestExtractIssuer(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		want    string
		wantErr bool
	}{
		{
			name:    "invalid format - no dots",
			token:   "notavalidtoken",
			wantErr: true,
		},
		{
			name:    "invalid format - wrong number of parts",
			token:   "part1.part2",
			wantErr: true,
		},
		{
			name:    "invalid base64",
			token:   "header.!!!invalid!!!.signature",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractIssuer(tt.token)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractIssuer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("extractIssuer() = %v, want %v", got, tt.want)
			}
		})
	}
}

// fakeGitHub provides a mock GitHub API server for testing
type fakeGitHub struct {
	mux *http.ServeMux
}

func newFakeGitHub() *fakeGitHub {
	mux := http.NewServeMux()
	mux.HandleFunc("/app/installations", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]github.Installation{{
			ID: github.Ptr(int64(1234)),
			Account: &github.User{
				Login: github.Ptr("org"),
			},
		}})
	})
	mux.HandleFunc("/app/installations/{appID}/access_tokens", func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(github.InstallationToken{
			Token:     github.Ptr(base64.StdEncoding.EncodeToString(b)),
			ExpiresAt: &github.Timestamp{Time: time.Now().Add(10 * time.Minute)},
		})
	})
	mux.HandleFunc("/repos/{org}/{repo}/contents/.github/chainguard/{identity}", func(w http.ResponseWriter, r *http.Request) {
		b, err := os.ReadFile(filepath.Join("testdata", r.PathValue("org"), r.PathValue("repo"), r.PathValue("identity")))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(io.MultiWriter(w, os.Stdout), "ReadFile failed: %v\n", err)
		}
		json.NewEncoder(w).Encode(github.RepositoryContent{
			Content:  github.Ptr(base64.StdEncoding.EncodeToString(b)),
			Type:     github.Ptr("file"),
			Encoding: github.Ptr("base64"),
		})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotImplemented)
		fmt.Fprintf(io.MultiWriter(w, os.Stdout), "%s %s not implemented\n", r.Method, r.URL.Path)
	})

	return &fakeGitHub{
		mux: mux,
	}
}

func (f *fakeGitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mux.ServeHTTP(w, r)
}

func TestExchange(t *testing.T) {
	ctx := slogtest.Context(t)
	atr := newGitHubClient(t, newFakeGitHub())

	pk, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("cannot generate RSA key %v", err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{
		Algorithm: jose.RS256,
		Key:       pk,
	}, nil)
	if err != nil {
		t.Fatalf("jose.NewSigner() = %v", err)
	}

	iss := "https://token.actions.githubusercontent.com"
	token, err := josejwt.Signed(signer).Claims(josejwt.Claims{
		Subject:  "foo",
		Issuer:   iss,
		Audience: josejwt.Audience{"octosts"},
		Expiry:   josejwt.NewNumericDate(time.Now().Add(10 * time.Minute)),
	}).Serialize()
	if err != nil {
		t.Fatalf("CompactSerialize failed: %v", err)
	}
	provider.AddTestKeySetVerifier(t, iss, &oidc.StaticKeySet{
		PublicKeys: []crypto.PublicKey{pk.Public()},
	})

	sts, err := New(atr, Config{
		Domain: "octosts",
	})
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	for _, tc := range []struct {
		name string
		req  ExchangeRequest
		want *github.InstallationTokenOptions
	}{
		{
			name: "repo",
			req: ExchangeRequest{
				Identity: "foo",
				Scope:    "org/repo",
			},
			want: &github.InstallationTokenOptions{
				Repositories: []string{"repo"},
				Permissions: &github.InstallationPermissions{
					PullRequests: github.Ptr("write"),
				},
			},
		},
		{
			name: "org",
			req: ExchangeRequest{
				Identity: "foo",
				Scope:    "org",
			},
			want: &github.InstallationTokenOptions{
				Permissions: &github.InstallationPermissions{
					PullRequests: github.Ptr("write"),
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(tc.req)
			if err != nil {
				t.Fatalf("json.Marshal failed: %v", err)
			}

			resp := sts.HandleRequest(ctx, shared.Request{
				Type:   shared.RequestTypeHTTP,
				Method: http.MethodPost,
				Path:   "/",
				Headers: shared.NormalizeHeaders(map[string]string{
					"Authorization": "Bearer " + token,
					"Content-Type":  "application/json",
				}),
				Body: body,
			})

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("HandleRequest failed: status=%d, body=%s", resp.StatusCode, string(resp.Body))
			}

			var exchangeResp ExchangeResponse
			if err := json.Unmarshal(resp.Body, &exchangeResp); err != nil {
				t.Fatalf("Unmarshal response failed: %v", err)
			}

			b, err := base64.StdEncoding.DecodeString(exchangeResp.Token)
			if err != nil {
				t.Fatalf("DecodeString failed: %v", err)
			}
			got := new(github.InstallationTokenOptions)
			if err := json.Unmarshal(b, got); err != nil {
				t.Fatalf("Unmarshal token options failed: %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestExchangeValidation(t *testing.T) {
	ctx := slogtest.Context(t)
	atr := newGitHubClient(t, newFakeGitHub())

	pk, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("cannot generate RSA key %v", err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{
		Algorithm: jose.RS256,
		Key:       pk,
	}, nil)
	if err != nil {
		t.Fatalf("jose.NewSigner() = %v", err)
	}

	iss := "https://token.actions.githubusercontent.com"
	token, err := josejwt.Signed(signer).Claims(josejwt.Claims{
		Subject:  "foo",
		Issuer:   iss,
		Audience: josejwt.Audience{"octosts"},
		Expiry:   josejwt.NewNumericDate(time.Now().Add(10 * time.Minute)),
	}).Serialize()
	if err != nil {
		t.Fatalf("CompactSerialize failed: %v", err)
	}
	provider.AddTestKeySetVerifier(t, iss, &oidc.StaticKeySet{
		PublicKeys: []crypto.PublicKey{pk.Public()},
	})

	sts, err := New(atr, Config{
		Domain: "octosts",
	})
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	tests := []struct {
		name           string
		req            ExchangeRequest
		expectedStatus int
	}{
		{
			name: "empty scope",
			req: ExchangeRequest{
				Identity: "foo",
				Scope:    "",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "empty identity",
			req: ExchangeRequest{
				Identity: "",
				Scope:    "org/repo",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "both empty",
			req: ExchangeRequest{
				Identity: "",
				Scope:    "",
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(tc.req)
			if err != nil {
				t.Fatalf("json.Marshal failed: %v", err)
			}

			resp := sts.HandleRequest(ctx, shared.Request{
				Type:   shared.RequestTypeHTTP,
				Method: http.MethodPost,
				Path:   "/",
				Headers: shared.NormalizeHeaders(map[string]string{
					"Authorization": "Bearer " + token,
					"Content-Type":  "application/json",
				}),
				Body: body,
			})

			if resp.StatusCode != tc.expectedStatus {
				t.Errorf("HandleRequest() status = %d, expected %d, body = %s", resp.StatusCode, tc.expectedStatus, string(resp.Body))
			}
		})
	}
}

func newGitHubClient(t *testing.T, h http.Handler) *ghinstallation.AppsTransport {
	t.Helper()

	tlsConfig, err := generateTLS(&x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotAfter:     time.Now().Add(10 * time.Hour),
		DNSNames:     []string{"localhost"},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(h)
	srv.TLS = tlsConfig
	srv.StartTLS()
	t.Cleanup(srv.Close)

	// Create a custom transport that overrides the Dial funcs
	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
		DialTLS: func(network, addr string) (net.Conn, error) {
			return tls.Dial(network, strings.TrimPrefix(srv.URL, "https://"), tlsConfig)
		},
		Dial: func(network, addr string) (net.Conn, error) {
			return tls.Dial(network, strings.TrimPrefix(srv.URL, "http://"), tlsConfig)
		},
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}
	ghsigner := ghinstallation.NewRSASigner(jwt.SigningMethodRS256, key)

	atr, err := ghinstallation.NewAppsTransportWithOptions(transport, 1234, ghinstallation.WithSigner(ghsigner))
	if err != nil {
		t.Fatalf("NewAppsTransportWithOptions failed: %v", err)
	}
	atr.BaseURL = srv.URL

	return atr
}

func generateTLS(tmpl *x509.Certificate) (*tls.Config, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("error generating private key: %w", err)
	}
	pub := &priv.PublicKey
	raw, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	if err != nil {
		return nil, fmt.Errorf("error generating certificate: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: raw,
	})
	keyBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("error marshaling key bytes: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: keyBytes,
	})
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("error loading tls certificate: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(certPEM) {
		return nil, fmt.Errorf("error adding cert to pool")
	}

	return &tls.Config{
		Certificates:       []tls.Certificate{tlsCert},
		RootCAs:            pool,
		InsecureSkipVerify: true,
	}, nil
}
