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

package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmware/vsphere-cosi-driver/api/v1alpha1"
	"github.com/vmware/vsphere-cosi-driver/internal/platform/kube"
	vboss "github.com/vmware/vsphere-cosi-driver/internal/vboss/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

func TestNewScheme(t *testing.T) {
	t.Run("WhenSchemeIsBuilt", func(tt *testing.T) {
		// Execute
		s, err := kube.NewScheme()

		// Assert
		require.NoError(tt, err)
		assert.True(tt, s.Recognizes(corev1.SchemeGroupVersion.WithKind("Secret")))
		assert.True(tt, s.Recognizes(v1alpha1.GroupVersion.WithKind("ObjectStoreBucket")))
		assert.True(tt, s.Recognizes(vboss.GroupVersion.WithKind("ObjectStore")))
	})
}
