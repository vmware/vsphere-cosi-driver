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

// Package contract is the behaviour every s3.Client must follow. This ensures that the real client
// and the fakes behave the same way.
package contract

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmware/vsphere-cosi-driver/internal/s3"
)

const bucket = "bucket-a"

// Backend is a client plus the one thing a test cannot do through the Client interface: put an
// object in a bucket so that it cannot be deleted.
type Backend struct {
	Client       s3.Client
	MakeNotEmpty func(bucket string)
}

// NewBackend returns an empty store each time it is called.
type NewBackend func(t *testing.T) Backend

// BucketExists checks s3.Client.BucketExists.
func BucketExists(t *testing.T, newBackend NewBackend) {
	t.Run("WhenBucketDoesNotExist", func(tt *testing.T) {
		// Setup
		b := newBackend(tt)

		// Execute
		exists, err := b.Client.BucketExists(tt.Context(), bucket)

		// Assert
		require.NoError(tt, err)
		assert.False(tt, exists)
	})

	t.Run("WhenBucketExists", func(tt *testing.T) {
		// Setup
		b := newBackend(tt)
		require.NoError(tt, b.Client.CreateBucket(tt.Context(), bucket))

		// Execute
		exists, err := b.Client.BucketExists(tt.Context(), bucket)

		// Assert
		require.NoError(tt, err)
		assert.True(tt, exists)
	})
}

// CreateBucket checks s3.Client.CreateBucket.
func CreateBucket(t *testing.T, newBackend NewBackend) {
	t.Run("WhenNameIsFree", func(tt *testing.T) {
		// Setup
		b := newBackend(tt)

		// Execute
		err := b.Client.CreateBucket(tt.Context(), bucket)

		// Assert
		require.NoError(tt, err)
		assertExists(tt, b, bucket, true)
	})

	t.Run("WhenBucketAlreadyExists", func(tt *testing.T) {
		// Setup
		b := newBackend(tt)
		require.NoError(tt, b.Client.CreateBucket(tt.Context(), bucket))

		// Execute
		err := b.Client.CreateBucket(tt.Context(), bucket)

		// Assert
		assert.ErrorIs(tt, err, s3.ErrAlreadyExists)
	})

	t.Run("WhenNameHasUppercase", func(tt *testing.T) {
		// Setup
		b := newBackend(tt)

		// Execute
		err := b.Client.CreateBucket(tt.Context(), "Bucket-A")

		// Assert
		assert.ErrorIs(tt, err, s3.ErrInvalidName)
	})

	t.Run("WhenNameIsTooShort", func(tt *testing.T) {
		// Setup
		b := newBackend(tt)

		// Execute
		err := b.Client.CreateBucket(tt.Context(), "ab")

		// Assert
		assert.ErrorIs(tt, err, s3.ErrInvalidName)
	})

	t.Run("WhenNameHasUnderscore", func(tt *testing.T) {
		// Setup
		b := newBackend(tt)

		// Execute
		err := b.Client.CreateBucket(tt.Context(), "bucket_a")

		// Assert
		assert.ErrorIs(tt, err, s3.ErrInvalidName)
	})
}

// DeleteBucket checks s3.Client.DeleteBucket.
func DeleteBucket(t *testing.T, newBackend NewBackend) {
	t.Run("WhenBucketIsEmpty", func(tt *testing.T) {
		// Setup
		b := newBackend(tt)
		require.NoError(tt, b.Client.CreateBucket(tt.Context(), bucket))

		// Execute
		err := b.Client.DeleteBucket(tt.Context(), bucket)

		// Assert
		require.NoError(tt, err)
		assertExists(tt, b, bucket, false)
	})

	t.Run("WhenBucketDoesNotExist", func(tt *testing.T) {
		// Setup
		b := newBackend(tt)

		// Execute
		err := b.Client.DeleteBucket(tt.Context(), bucket)

		// Assert
		assert.ErrorIs(tt, err, s3.ErrNotFound)
	})

	t.Run("WhenBucketHoldsObjects", func(tt *testing.T) {
		// Setup
		b := newBackend(tt)
		require.NoError(tt, b.Client.CreateBucket(tt.Context(), bucket))
		b.MakeNotEmpty(bucket)

		// Execute
		err := b.Client.DeleteBucket(tt.Context(), bucket)

		// Assert
		assert.ErrorIs(tt, err, s3.ErrNotEmpty)
		assertExists(tt, b, bucket, true) // a refused delete must leave the bucket in place
	})
}

func assertExists(t *testing.T, b Backend, name string, want bool) {
	t.Helper()
	exists, err := b.Client.BucketExists(t.Context(), name)
	require.NoError(t, err)
	assert.Equal(t, want, exists)
}
