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

package v1alpha1_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	vboss "github.com/vmware/vsphere-cosi-driver/internal/vboss/v1alpha1"
)

func TestObjectStoreDecodesRealShape(t *testing.T) {
	t.Run("WhenPayloadHasUnknownFields", func(tt *testing.T) {
		// Setup
		payload := []byte(`{
		  "apiVersion": "objectstore.vsan.broadcom.com/v1alpha1",
		  "kind": "ObjectStore",
		  "metadata": {"name": "store-1"},
		  "spec": {"enableTLS": true, "domain": "s3.example", "maxBuckets": 50,
		           "certSecretRef": {"name": "cert"}, "somethingNew": {"x": 1}},
		  "status": {"healthState": "Ready", "other": 7,
		             "externalS3Endpoint": {"fqdn": "https://s3.example:443", "extra": "y"}}
		}`)

		// Execute
		var got vboss.ObjectStore
		err := json.Unmarshal(payload, &got)

		// Assert
		require.NoError(tt, err)
		require.Equal(tt, "Ready", got.Status.HealthState)
		require.Equal(tt, "https://s3.example:443", got.Status.ExternalS3Endpoint.FQDN)
		require.True(tt, got.Spec.EnableTLS)
		require.Equal(tt, "s3.example", got.Spec.Domain)
		require.EqualValues(tt, 50, got.Spec.MaxBuckets)
		require.Equal(tt, "cert", got.Spec.CertSecretRef.Name)
	})

	t.Run("WhenSchemeIsBuilt", func(tt *testing.T) {
		// Setup / Execute / Assert
		require.Equal(tt, "objectstore.vsan.broadcom.com", vboss.GroupVersion.Group)
	})
}
