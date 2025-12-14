// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

package app

import (
	"net/http"

	"github.com/cruxstack/octo-sts-distros/internal/shared"
)

// Re-export shared types for package users.
type (
	// RequestType identifies the type of incoming request.
	RequestType = shared.RequestType
	// Request represents a runtime-agnostic HTTP request.
	Request = shared.Request
	// Response represents a runtime-agnostic HTTP response.
	Response = shared.Response
)

// Re-export shared constants.
const (
	// RequestTypeHTTP represents a standard HTTP request.
	RequestTypeHTTP = shared.RequestTypeHTTP
)

// NormalizeHeaders converts header keys to lowercase for consistent access.
var NormalizeHeaders = shared.NormalizeHeaders

// NewResponse creates a new Response with the given status code and body.
func NewResponse(statusCode int, body []byte) Response {
	return Response{
		StatusCode: statusCode,
		Headers:    make(map[string]string),
		Body:       body,
	}
}

// ErrorResponse creates an error response with the given status code and message.
// For the app package, errors are returned as plain text.
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
