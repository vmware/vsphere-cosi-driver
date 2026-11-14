/*
Copyright 2026 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package config reads process settings (namespace, log level, listen addresses) from the
// environment, so a binary needs one place to load its configuration.
package config

import "errors"

// Environment variables read at startup.
const (
	// EnvNamespace is the namespace the operator runs in and serves. Set it from the downward API.
	EnvNamespace = "POD_NAMESPACE"
	// EnvLogLevel is the log level (debug, info, warn, error).
	EnvLogLevel = "LOG_LEVEL"
	// EnvMetricsAddr is the metrics listen address.
	EnvMetricsAddr = "METRICS_ADDR"
	// EnvProbeAddr is the health probe listen address.
	EnvProbeAddr = "PROBE_ADDR"
)

// ErrMissingNamespace is returned when POD_NAMESPACE is unset.
var ErrMissingNamespace = errors.New(EnvNamespace + " is required")

// Config holds the process settings.
type Config struct {
	Namespace   string
	LogLevel    string
	MetricsAddr string
	ProbeAddr   string
}

// Load reads the settings through getenv, so tests do not touch the process environment.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		Namespace:   getenv(EnvNamespace),
		LogLevel:    valueOr(getenv(EnvLogLevel), "info"),
		MetricsAddr: valueOr(getenv(EnvMetricsAddr), ":8080"),
		ProbeAddr:   valueOr(getenv(EnvProbeAddr), ":8081"),
	}
	if cfg.Namespace == "" {
		return Config{}, ErrMissingNamespace
	}
	return cfg, nil
}

func valueOr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
