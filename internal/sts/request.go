// Copyright 2025 Octo-STS
// SPDX-License-Identifier: MIT

package sts

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Header keys (lowercase for normalized header access).
const (
	HeaderAuthorization = "authorization"
	HeaderContentType   = "content-type"
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

// ExchangeRequest represents a token exchange request.
type ExchangeRequest struct {
	// Identity is the name of the trust policy to use (e.g., "my-workflow").
	Identity string `json:"identity"`

	// Scope is the target scope for the token (e.g., "org/repo" or "org").
	Scope string `json:"scope"`
}

// ExchangeResponse represents a successful token exchange response.
type ExchangeResponse struct {
	// Token is the GitHub installation access token.
	Token string `json:"token"`
}

// ErrorResponseBody represents an error response body.
type ErrorResponseBody struct {
	// Error is the error message.
	Error string `json:"error"`
}

// NewResponse creates a new Response with the given status code and body.
func NewResponse(statusCode int, body []byte) Response {
	return Response{
		StatusCode: statusCode,
		Headers:    make(map[string]string),
		Body:       body,
	}
}

// ErrorResponse creates an error response with the given status code and message.
func ErrorResponse(statusCode int, message string) Response {
	body, _ := json.Marshal(ErrorResponseBody{Error: message})
	return Response{
		StatusCode: statusCode,
		Headers: map[string]string{
			HeaderContentType: "application/json",
		},
		Body: body,
	}
}

// JSONResponse creates a JSON response with the given status code and data.
func JSONResponse(statusCode int, data any) Response {
	body, err := json.Marshal(data)
	if err != nil {
		return ErrorResponse(http.StatusInternalServerError, "failed to encode response")
	}
	return Response{
		StatusCode: statusCode,
		Headers: map[string]string{
			HeaderContentType: "application/json",
		},
		Body: body,
	}
}

// OKResponse creates a 200 OK response with no body.
func OKResponse() Response {
	return Response{
		StatusCode: http.StatusOK,
		Headers:    make(map[string]string),
		Body:       nil,
	}
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
