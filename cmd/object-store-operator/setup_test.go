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

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmware/vsphere-cosi-driver/internal/platform/config"
	"github.com/vmware/vsphere-cosi-driver/internal/platform/kube"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// newManager returns a manager that is never started and never connects.
func newManager(t *testing.T) ctrl.Manager {
	t.Helper()
	scheme, err := kube.NewScheme()
	require.NoError(t, err)
	mgr, err := ctrl.NewManager(&rest.Config{Host: "http://127.0.0.1:1"}, ctrl.Options{
		Scheme:  scheme,
		Metrics: metricsserver.Options{BindAddress: "0"},
	})
	require.NoError(t, err)
	return mgr
}

func TestSetup(t *testing.T) {
	cfg := config.Config{Namespace: "store-a", CredentialsDir: "/keys", RootCredentialsDir: "/root-keys"}

	t.Run("WhenTheManagerIsValid", func(tt *testing.T) {
		// Setup
		mgr := newManager(tt)

		// Execute
		err := setup(mgr, cfg)

		// Assert
		assert.NoError(tt, err)
	})
}

func TestNewS3Client(t *testing.T) {
	t.Run("WhenTheEndpointIsValid", func(tt *testing.T) {
		// Execute
		c, err := newS3Client("http://s3.example.test", "access", "secret")

		// Assert
		require.NoError(tt, err)
		assert.NotNil(tt, c)
	})
}

func TestNewIAMClient(t *testing.T) {
	t.Run("WhenTheEndpointIsValid", func(tt *testing.T) {
		// Execute
		c, err := newIAMClient("http://iam.example.test", "access", "secret")

		// Assert
		require.NoError(tt, err)
		assert.NotNil(tt, c)
	})
}
