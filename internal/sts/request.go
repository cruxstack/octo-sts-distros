// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

package sts

import (
	"encoding/json"
	"net/http"

	"github.com/cruxstack/octo-sts-distros/internal/shared"
)

// Header keys (lowercase for normalized header access).
const (
	HeaderAuthorization = "authorization"
	HeaderContentType   = "content-type"
)

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

// ErrorResponse creates an error response with the given status code and message.
// For the STS package, errors are returned as JSON.
func ErrorResponse(statusCode int, message string) shared.Response {
	body, _ := json.Marshal(ErrorResponseBody{Error: message})
	return shared.Response{
		StatusCode: statusCode,
		Headers: map[string]string{
			HeaderContentType: "application/json",
		},
		Body: body,
	}
}

// JSONResponse creates a JSON response with the given status code and data.
func JSONResponse(statusCode int, data any) shared.Response {
	body, err := json.Marshal(data)
	if err != nil {
		return ErrorResponse(http.StatusInternalServerError, "failed to encode response")
	}
	return shared.Response{
		StatusCode: statusCode,
		Headers: map[string]string{
			HeaderContentType: "application/json",
		},
		Body: body,
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
