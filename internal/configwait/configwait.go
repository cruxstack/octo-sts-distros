// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

// Package configwait re-exports the configwait package from the ghappsetup library.
package configwait

import (
	"github.com/cruxstack/octo-sts-distros/pkg/ghappsetup/configwait"
)

// Re-export types from the library
type Config = configwait.Config
type LoadFunc = configwait.LoadFunc
type ReloadFunc = configwait.ReloadFunc
type ReadyGate = configwait.ReadyGate
type Reloader = configwait.Reloader

// Re-export constants from the library
const (
	EnvMaxRetries        = configwait.EnvMaxRetries
	EnvRetryInterval     = configwait.EnvRetryInterval
	DefaultMaxRetries    = configwait.DefaultMaxRetries
	DefaultRetryInterval = configwait.DefaultRetryInterval
)

// Re-export functions from the library
var NewConfigFromEnv = configwait.NewConfigFromEnv
var Wait = configwait.Wait
var NewReadyGate = configwait.NewReadyGate
var NewReloader = configwait.NewReloader
var SetGlobalReloader = configwait.SetGlobalReloader
var TriggerReload = configwait.TriggerReload
