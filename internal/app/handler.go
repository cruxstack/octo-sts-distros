// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

package app

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/chainguard-dev/clog"

	"github.com/octo-sts/app/pkg/webhook"
)

// Header keys (lowercase for normalized header access).
const (
	HeaderDelivery     = "x-github-delivery"
	HeaderEvent        = "x-github-event"
	HeaderSignature256 = "x-hub-signature-256"
	HeaderSignature    = "x-hub-signature"
	HeaderContentType  = "content-type"
)

// HandleRequest is the single entry point for processing all requests.
// It routes requests based on method and path to the appropriate handler.
func (a *App) HandleRequest(ctx context.Context, req Request) Response {
	// Strip base path from the request path
	path := a.stripBasePath(req.Path)

	// Add delivery ID and event type to logger context for tracing
	log := clog.FromContext(ctx).With(
		"delivery", req.Headers[HeaderDelivery],
		"event", req.Headers[HeaderEvent],
	)
	ctx = clog.WithLogger(ctx, log)

	// Route based on method and path
	switch {
	case req.Method == http.MethodPost && (path == "/" || path == "" || path == "/webhook"):
		return a.handleWebhook(ctx, req)
	default:
		return ErrorResponse(http.StatusNotFound, "not found")
	}
}

// ServeHTTP implements http.Handler interface, allowing the App to be used
// directly as an HTTP handler without the Request/Response abstraction.
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Convert http.Request headers to map[string]string (lowercase keys)
	headers := make(map[string]string)
	for k := range r.Header {
		headers[strings.ToLower(k)] = r.Header.Get(k)
	}

	// Create app.Request
	req := Request{
		Type:    RequestTypeHTTP,
		Method:  r.Method,
		Path:    r.URL.Path,
		Headers: headers,
		Body:    body,
	}

	// Handle request
	resp := a.HandleRequest(r.Context(), req)

	// Write response headers
	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}

	// Write status code and body
	w.WriteHeader(resp.StatusCode)
	if resp.Body != nil {
		if _, err := w.Write(resp.Body); err != nil {
			clog.FromContext(r.Context()).Errorf("failed to write response body: %v", err)
		}
	}
}

// stripBasePath removes the configured base path prefix from the request path.
func (a *App) stripBasePath(path string) string {
	if a.basePath == "" {
		return path
	}
	stripped := strings.TrimPrefix(path, a.basePath)
	// Ensure the path starts with "/" after stripping
	if stripped == "" || stripped[0] != '/' {
		stripped = "/" + stripped
	}
	return stripped
}

// handleWebhook processes GitHub webhook events by delegating to the existing
// webhook.Validator from pkg/webhook. This approach avoids duplicating the
// webhook handling logic while providing a runtime-agnostic interface.
func (a *App) handleWebhook(ctx context.Context, req Request) Response {
	log := clog.FromContext(ctx)

	// Create a Validator with our configuration
	validator := &webhook.Validator{
		Transport:     a.transport,
		WebhookSecret: a.webhookSecret,
		Organizations: a.organizations,
	}

	// Convert app.Request to http.Request
	httpReq, err := a.toHTTPRequest(ctx, req)
	if err != nil {
		log.Errorf("error creating http request: %v", err)
		return ErrorResponse(http.StatusBadRequest, err.Error())
	}

	// Use a responseRecorder to capture the response from ServeHTTP
	recorder := newResponseRecorder()

	// Delegate to existing webhook handler
	validator.ServeHTTP(recorder, httpReq)

	return Response{
		StatusCode: recorder.statusCode,
		Headers:    recorder.headers,
		Body:       recorder.body.Bytes(),
	}
}

// toHTTPRequest converts an app.Request to a standard http.Request.
func (a *App) toHTTPRequest(ctx context.Context, req Request) (*http.Request, error) {
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.Path, bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}

	// Copy headers - http.Header.Set will canonicalize the keys
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	return httpReq, nil
}

// responseRecorder implements http.ResponseWriter to capture the response
// from the webhook handler.
type responseRecorder struct {
	headers    map[string]string
	statusCode int
	body       *bytes.Buffer
}

// newResponseRecorder creates a new responseRecorder with default values.
func newResponseRecorder() *responseRecorder {
	return &responseRecorder{
		headers:    make(map[string]string),
		statusCode: http.StatusOK,
		body:       new(bytes.Buffer),
	}
}

// Header returns the response headers as an http.Header.
// Note: This is a simplified implementation that only supports single values per key.
func (r *responseRecorder) Header() http.Header {
	h := make(http.Header)
	for k, v := range r.headers {
		h.Set(k, v)
	}
	return h
}

// Write writes the data to the response body buffer.
func (r *responseRecorder) Write(data []byte) (int, error) {
	return r.body.Write(data)
}

// WriteHeader records the status code.
func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
}
