// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

// Package shared provides common types and utilities shared across internal packages.
package shared

import (
	"strings"
)

// RequestType identifies the type of incoming request.
type RequestType string

const (
	// RequestTypeHTTP represents a standard HTTP request.
	RequestTypeHTTP RequestType = "http"
)

// Request represents a runtime-agnostic HTTP request.
// This abstraction allows the same request handling logic to work
// with both standard HTTP servers and AWS API Gateway v2 with Lambda.
type Request struct {
	// Type identifies the request type.
	Type RequestType

	// Method is the HTTP method (GET, POST, etc.).
	Method string

	// Path is the request path after any base path stripping.
	Path string

	// Headers contains request headers with lowercase keys for consistent access.
	Headers map[string]string

	// QueryParams contains URL query parameters.
	QueryParams map[string]string

	// Body contains the raw request body.
	Body []byte
}

// Response represents a runtime-agnostic HTTP response.
type Response struct {
	// StatusCode is the HTTP status code.
	StatusCode int

	// Headers contains response headers.
	Headers map[string]string

	// Body contains the raw response body.
	Body []byte
}

// NormalizeHeaders converts header keys to lowercase for consistent access
// across different runtime environments.
func NormalizeHeaders(headers map[string]string) map[string]string {
	normalized := make(map[string]string, len(headers))
	for k, v := range headers {
		normalized[strings.ToLower(k)] = v
	}
	return normalized
}
