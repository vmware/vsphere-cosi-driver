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

package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vmware/vsphere-cosi-driver/internal/platform/config"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestConfigLoad(t *testing.T) {
	t.Run("WhenNamespaceIsMissing", func(tt *testing.T) {
		// Execute
		_, err := config.Load(env(nil))

		// Assert
		require.ErrorIs(tt, err, config.ErrMissingNamespace)
	})

	t.Run("WhenOnlyNamespaceIsSet", func(tt *testing.T) {
		// Execute
		got, err := config.Load(env(map[string]string{config.EnvNamespace: "store-ns"}))

		// Assert
		require.NoError(tt, err)
		assert.Equal(tt, config.Config{
			Namespace:   "store-ns",
			LogLevel:    "info",
			MetricsAddr: ":8080",
			ProbeAddr:   ":8081",
		}, got)
	})

	t.Run("WhenEverythingIsSet", func(tt *testing.T) {
		// Setup
		e := map[string]string{
			config.EnvNamespace:   "store-ns",
			config.EnvLogLevel:    "debug",
			config.EnvMetricsAddr: ":9090",
			config.EnvProbeAddr:   ":9091",
		}

		// Execute
		got, err := config.Load(env(e))

		// Assert
		require.NoError(tt, err)
		assert.Equal(tt, config.Config{Namespace: "store-ns", LogLevel: "debug", MetricsAddr: ":9090", ProbeAddr: ":9091"}, got)
	})
}
