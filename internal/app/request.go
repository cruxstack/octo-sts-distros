// Copyright 2025 Octo-STS
// SPDX-License-Identifier: MIT

package app

import (
	"net/http"
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
	return Response{
		StatusCode: statusCode,
		Headers: map[string]string{
			"content-type": "text/plain; charset=utf-8",
		},
		Body: []byte(message),
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

// AcceptedResponse creates a 202 Accepted response with no body.
// This is typically used when a webhook event was received but no action was taken.
func AcceptedResponse() Response {
	return Response{
		StatusCode: http.StatusAccepted,
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
