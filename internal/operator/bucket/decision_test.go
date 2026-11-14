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

package bucket_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vmware/vsphere-cosi-driver/api/v1alpha1"
	"github.com/vmware/vsphere-cosi-driver/internal/operator/bucket"
	"github.com/vmware/vsphere-cosi-driver/internal/platform/errs"
	"github.com/vmware/vsphere-cosi-driver/internal/s3"
	vboss "github.com/vmware/vsphere-cosi-driver/internal/vboss/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// newObservation returns a healthy bucket on a ready store with credentials and no S3 calls made.
func newObservation() bucket.Observation {
	return bucket.Observation{
		Bucket: &v1alpha1.ObjectStoreBucket{
			ObjectMeta: metav1.ObjectMeta{Name: "bucket-a"},
			Spec: v1alpha1.BucketSpec{
				DeletionPolicy: v1alpha1.DeletionPolicyDelete,
				Protocols:      []v1alpha1.ObjectProtocol{v1alpha1.ProtocolS3},
			},
		},
		Store:          &vboss.ObjectStore{Status: vboss.ObjectStoreStatus{HealthState: "Ready"}},
		HasCredentials: true,
	}
}

// newDeletion is newObservation with the bucket marked for deletion.
func newDeletion() bucket.Observation {
	obs := newObservation()
	now := metav1.Now()
	obs.Bucket.DeletionTimestamp = &now
	return obs
}

func assertRetryable(t *testing.T, out bucket.Outcome, reason string) {
	t.Helper()
	assert.Equal(t, bucket.StepNone, out.Next)
	assert.Equal(t, reason, out.Status.Reason)
	assert.False(t, out.Status.ReadyToUse)
	assert.Error(t, out.Err)
	assert.False(t, errs.IsTerminal(out.Err))
}

func assertTerminal(t *testing.T, out bucket.Outcome, reason string) {
	t.Helper()
	assert.Equal(t, bucket.StepNone, out.Next)
	assert.Equal(t, reason, out.Status.Reason)
	assert.False(t, out.Status.ReadyToUse)
	assert.True(t, errs.IsTerminal(out.Err))
	assert.Equal(t, reason, errs.Reason(out.Err))
}

func assertReady(t *testing.T, out bucket.Outcome) {
	t.Helper()
	assert.Equal(t, bucket.StepNone, out.Next)
	assert.True(t, out.Status.ReadyToUse)
	assert.Equal(t, bucket.ReasonProvisioned, out.Status.Reason)
	assert.NoError(t, out.Err)
}

func assertReleased(t *testing.T, out bucket.Outcome) {
	t.Helper()
	assert.Equal(t, bucket.StepReleaseFinalizer, out.Next)
	assert.Empty(t, out.Status.Reason)
	assert.NoError(t, out.Err)
}

func TestDecideProvisioning(t *testing.T) {
	t.Run("WhenNothingHasBeenCalledYet", func(tt *testing.T) {
		// Setup
		obs := newObservation()

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assert.Equal(tt, bucket.StepHead, out.Next)
		assert.NoError(tt, out.Err)
	})

	t.Run("WhenStoreIsMissing", func(tt *testing.T) {
		// Setup
		obs := newObservation()
		obs.Store = nil

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assertRetryable(tt, out, bucket.ReasonStoreNotFound)
	})

	t.Run("WhenStoreIsNotReady", func(tt *testing.T) {
		// Setup
		obs := newObservation()
		obs.Store.Status.HealthState = "Degraded"

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assertRetryable(tt, out, bucket.ReasonStoreNotReady)
	})

	t.Run("WhenCredentialsAreUnavailable", func(tt *testing.T) {
		// Setup
		obs := newObservation()
		obs.HasCredentials = false

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assertRetryable(tt, out, bucket.ReasonCredentialsUnavailable)
	})

	t.Run("WhenBucketExistsAfterRestart", func(tt *testing.T) {
		// Setup
		obs := newObservation()
		obs.Calls = []bucket.Call{{Op: bucket.OpHead}}

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assertReady(tt, out)
	})

	t.Run("WhenBucketIsMissing", func(tt *testing.T) {
		// Setup
		obs := newObservation()
		obs.Calls = []bucket.Call{{Op: bucket.OpHead, Err: s3.ErrNotFound}}

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assert.Equal(tt, bucket.StepCreate, out.Next)
		assert.NoError(tt, out.Err)
	})

	t.Run("WhenCreateSucceeds", func(tt *testing.T) {
		// Setup
		obs := newObservation()
		obs.Calls = []bucket.Call{{Op: bucket.OpHead, Err: s3.ErrNotFound}, {Op: bucket.OpCreate}}

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assertReady(tt, out)
	})

	t.Run("WhenCreateFindsTheNameTaken", func(tt *testing.T) {
		// Setup
		obs := newObservation()
		obs.Calls = []bucket.Call{{Op: bucket.OpHead, Err: s3.ErrNotFound}, {Op: bucket.OpCreate, Err: s3.ErrAlreadyExists}}

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assert.Equal(tt, bucket.StepHead, out.Next)
		assert.NoError(tt, out.Err)
	})

	t.Run("WhenBucketAppearsAfterNameTaken", func(tt *testing.T) {
		// Setup
		obs := newObservation()
		obs.Calls = []bucket.Call{
			{Op: bucket.OpHead, Err: s3.ErrNotFound},
			{Op: bucket.OpCreate, Err: s3.ErrAlreadyExists},
			{Op: bucket.OpHead},
		}

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assertReady(tt, out)
	})

	t.Run("WhenBucketStaysMissingAfterNameTaken", func(tt *testing.T) {
		// Setup
		obs := newObservation()
		obs.Calls = []bucket.Call{
			{Op: bucket.OpHead, Err: s3.ErrNotFound},
			{Op: bucket.OpCreate, Err: s3.ErrAlreadyExists},
			{Op: bucket.OpHead, Err: s3.ErrNotFound},
		}

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assertTerminal(tt, out, bucket.ReasonBucketNameTaken)
	})

	t.Run("WhenHeadIsDeniedAfterNameTaken", func(tt *testing.T) {
		// Setup
		obs := newObservation()
		obs.Calls = []bucket.Call{
			{Op: bucket.OpHead, Err: s3.ErrNotFound},
			{Op: bucket.OpCreate, Err: s3.ErrAlreadyExists},
			{Op: bucket.OpHead, Err: s3.ErrAccessDenied},
		}

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assertTerminal(tt, out, bucket.ReasonBucketNameTaken)
	})

	t.Run("WhenHeadIsDenied", func(tt *testing.T) {
		// Setup
		obs := newObservation()
		obs.Calls = []bucket.Call{{Op: bucket.OpHead, Err: s3.ErrAccessDenied}}

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assertRetryable(tt, out, bucket.ReasonAccessDenied)
	})

	t.Run("WhenCreateIsDenied", func(tt *testing.T) {
		// Setup
		obs := newObservation()
		obs.Calls = []bucket.Call{{Op: bucket.OpHead, Err: s3.ErrNotFound}, {Op: bucket.OpCreate, Err: s3.ErrAccessDenied}}

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assertRetryable(tt, out, bucket.ReasonAccessDenied)
	})

	t.Run("WhenCreateRejectsTheName", func(tt *testing.T) {
		// Setup
		obs := newObservation()
		obs.Calls = []bucket.Call{{Op: bucket.OpHead, Err: s3.ErrNotFound}, {Op: bucket.OpCreate, Err: s3.ErrInvalidName}}

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assertTerminal(tt, out, bucket.ReasonInvalidName)
	})

	t.Run("WhenStoreIsUnavailable", func(tt *testing.T) {
		// Setup
		obs := newObservation()
		obs.Calls = []bucket.Call{{Op: bucket.OpHead, Err: s3.ErrUnavailable}}

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assertRetryable(tt, out, bucket.ReasonStoreUnavailable)
	})

	t.Run("WhenCallFailsUnexpectedly", func(tt *testing.T) {
		// Setup
		obs := newObservation()
		obs.Calls = []bucket.Call{{Op: bucket.OpHead}, {Op: bucket.OpCreate, Err: errors.New("boom")}}

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assertRetryable(tt, out, bucket.ReasonFailed)
	})

	t.Run("WhenNameIsTooLong", func(tt *testing.T) {
		// Setup
		obs := newObservation()
		obs.Bucket.Name = strings.Repeat("a", 64)

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assertTerminal(tt, out, bucket.ReasonInvalidName)
	})

	t.Run("WhenNameIsTooShort", func(tt *testing.T) {
		// Setup
		obs := newObservation()
		obs.Bucket.Name = "ab"

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assertTerminal(tt, out, bucket.ReasonInvalidName)
	})

	t.Run("WhenExistingBucketIDIsSet", func(tt *testing.T) {
		// Setup
		obs := newObservation()
		obs.Bucket.Spec.ExistingBucketID = "other"

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assertTerminal(tt, out, bucket.ReasonUnsupported)
	})

	t.Run("WhenProtocolIsNotS3", func(tt *testing.T) {
		// Setup
		obs := newObservation()
		obs.Bucket.Spec.Protocols = []v1alpha1.ObjectProtocol{v1alpha1.ProtocolAzure}

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assertTerminal(tt, out, bucket.ReasonUnsupported)
	})
}

func TestDecideDeletion(t *testing.T) {
	t.Run("WhenPolicyIsRetain", func(tt *testing.T) {
		// Setup
		obs := newDeletion()
		obs.Bucket.Spec.DeletionPolicy = v1alpha1.DeletionPolicyRetain

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assertReleased(tt, out)
	})

	t.Run("WhenPolicyIsDelete", func(tt *testing.T) {
		// Setup
		obs := newDeletion()

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assert.Equal(tt, bucket.StepDelete, out.Next)
		assert.Equal(tt, bucket.ReasonDeleting, out.Status.Reason)
		assert.NoError(tt, out.Err)
	})

	t.Run("WhenStoreIsDegraded", func(tt *testing.T) {
		// Setup
		obs := newDeletion()
		obs.Store.Status.HealthState = "Degraded"

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assert.Equal(tt, bucket.StepDelete, out.Next)
	})

	t.Run("WhenStoreIsGone", func(tt *testing.T) {
		// Setup
		obs := newDeletion()
		obs.Store = nil

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assertReleased(tt, out)
	})

	t.Run("WhenCredentialsAreUnavailable", func(tt *testing.T) {
		// Setup
		obs := newDeletion()
		obs.HasCredentials = false

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assertRetryable(tt, out, bucket.ReasonCredentialsUnavailable)
	})

	t.Run("WhenDeleteSucceeds", func(tt *testing.T) {
		// Setup
		obs := newDeletion()
		obs.Calls = []bucket.Call{{Op: bucket.OpDelete}}

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assertReleased(tt, out)
	})

	t.Run("WhenBucketIsAlreadyGone", func(tt *testing.T) {
		// Setup
		obs := newDeletion()
		obs.Calls = []bucket.Call{{Op: bucket.OpDelete, Err: s3.ErrNotFound}}

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assertReleased(tt, out)
	})

	t.Run("WhenBucketHoldsObjects", func(tt *testing.T) {
		// Setup
		obs := newDeletion()
		obs.Calls = []bucket.Call{{Op: bucket.OpDelete, Err: s3.ErrNotEmpty}}

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assertRetryable(tt, out, bucket.ReasonBucketNotEmpty)
	})

	t.Run("WhenDeleteIsDenied", func(tt *testing.T) {
		// Setup
		obs := newDeletion()
		obs.Calls = []bucket.Call{{Op: bucket.OpDelete, Err: s3.ErrAccessDenied}}

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assertRetryable(tt, out, bucket.ReasonAccessDenied)
	})

	t.Run("WhenStoreIsUnavailable", func(tt *testing.T) {
		// Setup
		obs := newDeletion()
		obs.Calls = []bucket.Call{{Op: bucket.OpDelete, Err: s3.ErrUnavailable}}

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assertRetryable(tt, out, bucket.ReasonStoreUnavailable)
	})

	t.Run("WhenExistingBucketIDIsSet", func(tt *testing.T) {
		// Setup
		obs := newDeletion()
		obs.Bucket.Spec.ExistingBucketID = "other"

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assertReleased(tt, out) // nothing was provisioned, and the named bucket is not ours to delete
	})

	t.Run("WhenProtocolIsNotS3", func(tt *testing.T) {
		// Setup
		obs := newDeletion()
		obs.Bucket.Spec.Protocols = []v1alpha1.ObjectProtocol{v1alpha1.ProtocolGCS}

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assertReleased(tt, out)
	})

	t.Run("WhenNameIsInvalid", func(tt *testing.T) {
		// Setup
		obs := newDeletion()
		obs.Bucket.Name = "ab"

		// Execute
		out := bucket.Decide(obs)

		// Assert
		assertReleased(tt, out)
	})
}
