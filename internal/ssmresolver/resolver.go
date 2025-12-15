// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

// Package ssmresolver re-exports the ssmresolver package from the ghappsetup library.
package ssmresolver

import (
	"github.com/cruxstack/octo-sts-distros/pkg/ghappsetup/ssmresolver"
)

// Re-export types from the library
type Resolver = ssmresolver.Resolver
type Client = ssmresolver.Client
type RetryConfig = ssmresolver.RetryConfig

// Re-export constants from the library
const (
	EnvMaxRetries        = ssmresolver.EnvMaxRetries
	EnvRetryInterval     = ssmresolver.EnvRetryInterval
	DefaultMaxRetries    = ssmresolver.DefaultMaxRetries
	DefaultRetryInterval = ssmresolver.DefaultRetryInterval
)

// Re-export functions from the library
var New = ssmresolver.New
var NewWithClient = ssmresolver.NewWithClient
var IsSSMARN = ssmresolver.IsSSMARN
var ExtractParameterName = ssmresolver.ExtractParameterName
var ResolveEnvironmentWithDefaults = ssmresolver.ResolveEnvironmentWithDefaults
var NewRetryConfigFromEnv = ssmresolver.NewRetryConfigFromEnv
var ResolveEnvironmentWithRetry = ssmresolver.ResolveEnvironmentWithRetry
