// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

package app

import (
	"net/http"

	"github.com/cruxstack/octo-sts-distros/internal/shared"
)

// NewResponse creates a new Response with the given status code and body.
func NewResponse(statusCode int, body []byte) shared.Response {
	return shared.Response{
		StatusCode: statusCode,
		Headers:    make(map[string]string),
		Body:       body,
	}
}

// ErrorResponse creates an error response with the given status code and message.
// For the app package, errors are returned as plain text.
func ErrorResponse(statusCode int, message string) shared.Response {
	return shared.Response{
		StatusCode: statusCode,
		Headers: map[string]string{
			"content-type": "text/plain; charset=utf-8",
		},
		Body: []byte(message),
	}
}

// OKResponse creates a 200 OK response with no body.
func OKResponse() shared.Response {
	return shared.Response{
		StatusCode: http.StatusOK,
		Headers:    make(map[string]string),
		Body:       nil,
	}
}

// AcceptedResponse creates a 202 Accepted response with no body.
// This is typically used when a webhook event was received but no action was taken.
func AcceptedResponse() shared.Response {
	return shared.Response{
		StatusCode: http.StatusAccepted,
		Headers:    make(map[string]string),
		Body:       nil,
	}
}
