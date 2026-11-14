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

package manager_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/vmware/vsphere-cosi-driver/internal/platform/config"
	"github.com/vmware/vsphere-cosi-driver/internal/platform/manager"
)

func TestManagerOptions(t *testing.T) {
	cfg := config.Config{Namespace: "store-ns", LogLevel: "info", MetricsAddr: ":8080", ProbeAddr: ":8081"}

	t.Run("WhenOptionsAreBuilt", func(tt *testing.T) {
		// Setup
		scheme := runtime.NewScheme()

		// Execute
		opts := manager.NewOptions(cfg, scheme)

		// Assert
		assert.True(tt, opts.LeaderElection)
		assert.Equal(tt, "object-store-operator", opts.LeaderElectionID)
		assert.Equal(tt, "store-ns", opts.LeaderElectionNamespace)
		assert.Equal(tt, ":8080", opts.Metrics.BindAddress)
		assert.Equal(tt, ":8081", opts.HealthProbeBindAddress)
		assert.Same(tt, scheme, opts.Scheme)
	})

	t.Run("WhenOptionsAreBuilt_WatchOnlyTheStoreNamespace", func(tt *testing.T) {
		// Execute
		opts := manager.NewOptions(cfg, runtime.NewScheme())

		// Assert
		require.Len(tt, opts.Cache.DefaultNamespaces, 1)
		assert.Contains(tt, opts.Cache.DefaultNamespaces, "store-ns")
	})
}

// writeKubeconfig writes a kubeconfig that points at a server nobody listens on. The manager
// builds its client lazily, so New does not need a reachable cluster.
func writeKubeconfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kubeconfig")
	body := `apiVersion: v1
kind: Config
clusters:
- name: c
  cluster: {server: "https://127.0.0.1:1"}
users:
- name: u
  user: {token: t}
contexts:
- name: ctx
  context: {cluster: c, user: u}
current-context: ctx
`
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func TestManagerNew(t *testing.T) {
	cfg := config.Config{Namespace: "store-ns", LogLevel: "info", MetricsAddr: "0", ProbeAddr: "0"}

	t.Run("WhenKubeconfigIsValid", func(tt *testing.T) {
		// Setup
		tt.Setenv("KUBECONFIG", writeKubeconfig(tt))

		// Execute
		mgr, err := manager.New(cfg, runtime.NewScheme())

		// Assert
		require.NoError(tt, err)
		assert.NotNil(tt, mgr)
	})

	t.Run("WhenNoClusterConfigExists", func(tt *testing.T) {
		// Setup: no kubeconfig, no home directory and no in-cluster environment.
		tt.Setenv("KUBECONFIG", filepath.Join(tt.TempDir(), "missing"))
		tt.Setenv("HOME", tt.TempDir())
		tt.Setenv("KUBERNETES_SERVICE_HOST", "")

		// Execute
		mgr, err := manager.New(cfg, runtime.NewScheme())

		// Assert
		require.Error(tt, err)
		assert.Nil(tt, mgr)
	})
}
