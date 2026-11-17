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

package contract

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmware/vsphere-cosi-driver/internal/iam"
)

const (
	user       = "user-a"
	policyName = "policy-a"
	document   = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]}`
)

// Backend is a client plus what a test cannot read through the Client interface: the store's state.
type Backend struct {
	Client  iam.Client
	HasUser func(name string) bool
	Policy  func(user, name string) (document string, ok bool)
	HasKey  func(user, id string) bool
}

// NewBackend returns an empty store each time it is called.
type NewBackend func(t *testing.T) Backend

// CreateUser checks iam.Client.CreateUser.
func CreateUser(t *testing.T, newBackend NewBackend) {
	t.Run("WhenNameIsFree", func(tt *testing.T) {
		// Setup
		b := newBackend(tt)

		// Execute
		err := b.Client.CreateUser(tt.Context(), user)

		// Assert
		require.NoError(tt, err)
		assert.True(tt, b.HasUser(user))
	})

	t.Run("WhenUserExists", func(tt *testing.T) {
		// Setup
		b := newBackend(tt)
		createUser(tt, b)

		// Execute
		err := b.Client.CreateUser(tt.Context(), user)

		// Assert
		assert.ErrorIs(tt, err, iam.ErrAlreadyExists)
	})
}

// PutUserPolicy checks iam.Client.PutUserPolicy.
func PutUserPolicy(t *testing.T, newBackend NewBackend) {
	t.Run("WhenUserExists", func(tt *testing.T) {
		// Setup
		b := newBackend(tt)
		createUser(tt, b)

		// Execute
		err := b.Client.PutUserPolicy(tt.Context(), user, policyName, document)

		// Assert
		require.NoError(tt, err)
		got, ok := b.Policy(user, policyName)
		assert.True(tt, ok)
		assert.JSONEq(tt, document, got)
	})

	t.Run("WhenPolicyWithTheNameExists", func(tt *testing.T) {
		// Setup
		b := newBackend(tt)
		createUser(tt, b)
		require.NoError(tt, b.Client.PutUserPolicy(tt.Context(), user, policyName, `{"Version":"2012-10-17","Statement":[]}`))

		// Execute
		err := b.Client.PutUserPolicy(tt.Context(), user, policyName, document)

		// Assert
		require.NoError(tt, err)
		got, ok := b.Policy(user, policyName)
		assert.True(tt, ok)
		assert.JSONEq(tt, document, got)
	})

	t.Run("WhenUserDoesNotExist", func(tt *testing.T) {
		// Setup
		b := newBackend(tt)

		// Execute
		err := b.Client.PutUserPolicy(tt.Context(), user, policyName, document)

		// Assert
		assert.ErrorIs(tt, err, iam.ErrNotFound)
	})
}

// CreateAccessKey checks iam.Client.CreateAccessKey.
func CreateAccessKey(t *testing.T, newBackend NewBackend) {
	t.Run("WhenUserExists", func(tt *testing.T) {
		// Setup
		b := newBackend(tt)
		createUser(tt, b)

		// Execute
		key, err := b.Client.CreateAccessKey(tt.Context(), user)

		// Assert
		require.NoError(tt, err)
		assert.NotEmpty(tt, key.ID)
		assert.NotEmpty(tt, key.Secret)
		assert.True(tt, b.HasKey(user, key.ID))
	})

	t.Run("WhenCalledTwice", func(tt *testing.T) {
		// Setup
		b := newBackend(tt)
		createUser(tt, b)
		first, err := b.Client.CreateAccessKey(tt.Context(), user)
		require.NoError(tt, err)

		// Execute
		second, err := b.Client.CreateAccessKey(tt.Context(), user)

		// Assert
		require.NoError(tt, err)
		assert.NotEqual(tt, first.ID, second.ID)
		assert.True(tt, b.HasKey(user, first.ID), "a second key must not replace the first")
		assert.True(tt, b.HasKey(user, second.ID))
	})

	t.Run("WhenUserDoesNotExist", func(tt *testing.T) {
		// Setup
		b := newBackend(tt)

		// Execute
		_, err := b.Client.CreateAccessKey(tt.Context(), user)

		// Assert
		assert.ErrorIs(tt, err, iam.ErrNotFound)
	})
}

// DeleteAccessKey checks iam.Client.DeleteAccessKey.
func DeleteAccessKey(t *testing.T, newBackend NewBackend) {
	t.Run("WhenKeyExists", func(tt *testing.T) {
		// Setup
		b := newBackend(tt)
		createUser(tt, b)
		key, err := b.Client.CreateAccessKey(tt.Context(), user)
		require.NoError(tt, err)

		// Execute
		err = b.Client.DeleteAccessKey(tt.Context(), user, key.ID)

		// Assert
		require.NoError(tt, err)
		assert.False(tt, b.HasKey(user, key.ID))
	})

	t.Run("WhenKeyDoesNotExist", func(tt *testing.T) {
		// Setup
		b := newBackend(tt)
		createUser(tt, b)

		// Execute
		err := b.Client.DeleteAccessKey(tt.Context(), user, "AKIAMISSING")

		// Assert
		assert.ErrorIs(tt, err, iam.ErrNotFound)
	})

	t.Run("WhenUserDoesNotExist", func(tt *testing.T) {
		// Setup
		b := newBackend(tt)

		// Execute
		err := b.Client.DeleteAccessKey(tt.Context(), user, "AKIAMISSING")

		// Assert
		assert.ErrorIs(tt, err, iam.ErrNotFound)
	})
}

func createUser(t *testing.T, b Backend) {
	t.Helper()
	require.NoError(t, b.Client.CreateUser(t.Context(), user))
}
