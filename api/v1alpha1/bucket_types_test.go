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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/vmware/vsphere-cosi-driver/api/v1alpha1"
)

func TestBucketSchemeRoundTrip(t *testing.T) {
	t.Run("WhenBucketIsFullyPopulated", func(tt *testing.T) {
		// Setup
		ready := true
		msg := "boom"
		now := metav1.Now()
		in := v1alpha1.ObjectStoreBucket{
			Spec: v1alpha1.BucketSpec{
				DriverName:       "cosi.vsphere.vmware.com",
				DeletionPolicy:   v1alpha1.DeletionPolicyDelete,
				Parameters:       map[string]string{"k": "v"},
				Protocols:        []v1alpha1.ObjectProtocol{v1alpha1.ProtocolS3},
				BucketClaimRef:   v1alpha1.BucketClaimReference{Name: "c", Namespace: "n", UID: "u"},
				ExistingBucketID: "old",
			},
			Status: v1alpha1.BucketStatus{
				ReadyToUse: &ready,
				BucketID:   "id",
				Protocols:  []v1alpha1.ObjectProtocol{v1alpha1.ProtocolS3},
				BucketInfo: map[string]string{"a": "b"},
				Error:      &v1alpha1.TimestampedError{Time: &now, Message: &msg},
			},
		}

		// Execute
		raw, err := json.Marshal(in)
		require.NoError(tt, err)
		var got map[string]map[string]any
		require.NoError(tt, json.Unmarshal(raw, &got))

		// Assert
		for _, k := range []string{"driverName", "deletionPolicy", "parameters", "protocols", "bucketClaimRef", "existingBucketID"} {
			require.Contains(tt, got["spec"], k)
		}
		for _, k := range []string{"readyToUse", "bucketID", "protocols", "bucketInfo", "error"} {
			require.Contains(tt, got["status"], k)
		}
	})

	t.Run("WhenSchemeIsBuilt", func(tt *testing.T) {
		// Setup
		s := runtime.NewScheme()

		// Execute
		require.NoError(tt, v1alpha1.AddToScheme(s))

		// Assert
		require.True(tt, s.Recognizes(v1alpha1.GroupVersion.WithKind("ObjectStoreBucket")))
		require.True(tt, s.Recognizes(v1alpha1.GroupVersion.WithKind("ObjectStoreBucketList")))
		require.Equal(tt, "cosi.vsphere.vmware.com", v1alpha1.GroupVersion.Group)
	})
}
