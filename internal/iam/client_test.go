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

package iam_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmware/vsphere-cosi-driver/internal/iam"
	"github.com/vmware/vsphere-cosi-driver/internal/iam/contract"
	"github.com/vmware/vsphere-cosi-driver/internal/iam/fake"
)

const (
	testAccessKey = "AKIATESTKEY"
	testSecretKey = "super-secret-value"
	user          = "user-a"
)

// newClient returns a client that talks to a server running handler.
func newClient(t *testing.T, handler http.Handler) iam.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := iam.New(srv.URL, testAccessKey, testSecretKey, srv.Client())
	require.NoError(t, err)
	return c
}

// newBackend runs the real client against the fake's HTTP handler and its in-memory state.
func newBackend(t *testing.T) contract.Backend {
	t.Helper()
	store := fake.New()
	return contract.Backend{
		Client:  newClient(t, fake.NewHandler(store)),
		HasUser: store.HasUser,
		Policy:  store.Policy,
		HasKey:  store.HasKey,
	}
}

// errorBody answers every request with an IAM error body.
func errorBody(status int, code string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(status)
		_, _ = fmt.Fprintf(w, `<ErrorResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/"><Error><Type>Sender</Type><Code>%s</Code><Message>m</Message></Error><RequestId>r</RequestId></ErrorResponse>`, code)
	}
}

// keyResponse answers every request as a successful CreateAccessKey whose result holds key.
func keyResponse(key string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		_, _ = fmt.Fprintf(w, `<CreateAccessKeyResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/"><CreateAccessKeyResult>%s</CreateAccessKeyResult></CreateAccessKeyResponse>`, key)
	}
}

// statusOnly answers every request with a status and no body.
func statusOnly(status int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }
}

// unreachableEndpoint returns the address of a server that has been shut down.
func unreachableEndpoint(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	return srv.URL
}

func assertNoCredentials(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	assert.NotContains(t, err.Error(), testSecretKey)
	assert.NotContains(t, err.Error(), testAccessKey)
}

func assertUnclassified(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	for _, sentinel := range []error{iam.ErrNotFound, iam.ErrAlreadyExists, iam.ErrAccessDenied, iam.ErrUnavailable} {
		assert.NotErrorIs(t, err, sentinel)
	}
}

func TestNew(t *testing.T) {
	t.Run("WhenEndpointHasSchemeAndPort", func(tt *testing.T) {
		// Execute
		c, err := iam.New("https://iam.example.test:8443", testAccessKey, testSecretKey, nil)

		// Assert
		require.NoError(tt, err)
		assert.NotNil(tt, c)
	})

	t.Run("WhenEndpointIsNotAURL", func(tt *testing.T) {
		// Execute
		c, err := iam.New("http://[::1", testAccessKey, testSecretKey, nil)

		// Assert
		require.Error(tt, err)
		assert.Nil(tt, c)
	})

	t.Run("WhenEndpointIsEmpty", func(tt *testing.T) {
		// Execute
		c, err := iam.New("", testAccessKey, testSecretKey, nil)

		// Assert
		require.Error(tt, err)
		assert.Nil(tt, c)
	})

	t.Run("WhenEndpointHasNoScheme", func(tt *testing.T) {
		// Execute
		c, err := iam.New("iam.example.test:8443", testAccessKey, testSecretKey, nil)

		// Assert
		require.Error(tt, err)
		assert.Nil(tt, c)
	})

	t.Run("WhenEndpointHasOtherScheme", func(tt *testing.T) {
		// Execute
		c, err := iam.New("ftp://iam.example.test", testAccessKey, testSecretKey, nil)

		// Assert
		require.Error(tt, err)
		assert.Nil(tt, c)
	})

	t.Run("WhenEndpointHasNoHost", func(tt *testing.T) {
		// Execute
		c, err := iam.New("https://", testAccessKey, testSecretKey, nil)

		// Assert
		require.Error(tt, err)
		assert.Nil(tt, c)
	})
}

func TestCreateUser(t *testing.T) {
	contract.CreateUser(t, newBackend)

	t.Run("WhenUserExistsInTheStore", func(tt *testing.T) {
		// Setup
		c := newClient(tt, errorBody(http.StatusConflict, "EntityAlreadyExists"))

		// Execute
		err := c.CreateUser(tt.Context(), user)

		// Assert
		assert.ErrorIs(tt, err, iam.ErrAlreadyExists)
	})

	t.Run("WhenAccessIsDenied", func(tt *testing.T) {
		// Setup
		c := newClient(tt, errorBody(http.StatusForbidden, "AccessDenied"))

		// Execute
		err := c.CreateUser(tt.Context(), user)

		// Assert
		assert.ErrorIs(tt, err, iam.ErrAccessDenied)
		assertNoCredentials(tt, err)
	})

	t.Run("WhenTheCredentialIsUnknown", func(tt *testing.T) {
		// Setup
		c := newClient(tt, errorBody(http.StatusForbidden, "InvalidClientTokenId"))

		// Execute
		err := c.CreateUser(tt.Context(), user)

		// Assert
		assert.ErrorIs(tt, err, iam.ErrAccessDenied)
	})

	t.Run("WhenTheSignatureIsRejected", func(tt *testing.T) {
		// Setup
		c := newClient(tt, errorBody(http.StatusForbidden, "SignatureDoesNotMatch"))

		// Execute
		err := c.CreateUser(tt.Context(), user)

		// Assert
		assert.ErrorIs(tt, err, iam.ErrAccessDenied)
	})

	t.Run("WhenAccessIsDeniedWithoutABody", func(tt *testing.T) {
		// Setup
		c := newClient(tt, statusOnly(http.StatusForbidden))

		// Execute
		err := c.CreateUser(tt.Context(), user)

		// Assert
		assert.ErrorIs(tt, err, iam.ErrAccessDenied)
	})

	t.Run("WhenStoreFails", func(tt *testing.T) {
		// Setup
		c := newClient(tt, errorBody(http.StatusInternalServerError, "ServiceFailure"))

		// Execute
		err := c.CreateUser(tt.Context(), user)

		// Assert
		assert.ErrorIs(tt, err, iam.ErrUnavailable)
	})

	t.Run("WhenStoreIsUnavailable", func(tt *testing.T) {
		// Setup
		c := newClient(tt, errorBody(http.StatusServiceUnavailable, "ServiceUnavailable"))

		// Execute
		err := c.CreateUser(tt.Context(), user)

		// Assert
		assert.ErrorIs(tt, err, iam.ErrUnavailable)
	})

	t.Run("WhenTheStoreThrottles", func(tt *testing.T) {
		// Setup
		c := newClient(tt, errorBody(http.StatusBadRequest, "Throttling"))

		// Execute
		err := c.CreateUser(tt.Context(), user)

		// Assert
		assert.ErrorIs(tt, err, iam.ErrUnavailable)
	})

	t.Run("WhenStoreFailsWithoutABody", func(tt *testing.T) {
		// Setup
		c := newClient(tt, statusOnly(http.StatusBadGateway))

		// Execute
		err := c.CreateUser(tt.Context(), user)

		// Assert
		assert.ErrorIs(tt, err, iam.ErrUnavailable)
	})

	t.Run("WhenStoreIsUnreachable", func(tt *testing.T) {
		// Setup
		c, err := iam.New(unreachableEndpoint(tt), testAccessKey, testSecretKey, nil)
		require.NoError(tt, err)

		// Execute
		err = c.CreateUser(tt.Context(), user)

		// Assert
		assert.ErrorIs(tt, err, iam.ErrUnavailable)
		assertNoCredentials(tt, err)
	})

	t.Run("WhenTheErrorIsUnknown", func(tt *testing.T) {
		// Setup
		c := newClient(tt, errorBody(http.StatusBadRequest, "ValidationError"))

		// Execute
		err := c.CreateUser(tt.Context(), user)

		// Assert
		assertUnclassified(tt, err)
	})
}

func TestPutUserPolicy(t *testing.T) {
	contract.PutUserPolicy(t, newBackend)

	t.Run("WhenUserIsMissingInTheStore", func(tt *testing.T) {
		// Setup
		c := newClient(tt, errorBody(http.StatusNotFound, "NoSuchEntity"))

		// Execute
		err := c.PutUserPolicy(tt.Context(), user, "policy-a", "{}")

		// Assert
		assert.ErrorIs(tt, err, iam.ErrNotFound)
	})

	t.Run("WhenTheDocumentIsRejected", func(tt *testing.T) {
		// Setup
		c := newClient(tt, errorBody(http.StatusBadRequest, "MalformedPolicyDocument"))

		// Execute
		err := c.PutUserPolicy(tt.Context(), user, "policy-a", "{")

		// Assert
		assertUnclassified(tt, err)
	})

	t.Run("WhenAccessIsDenied", func(tt *testing.T) {
		// Setup
		c := newClient(tt, errorBody(http.StatusForbidden, "AccessDenied"))

		// Execute
		err := c.PutUserPolicy(tt.Context(), user, "policy-a", "{}")

		// Assert
		assert.ErrorIs(tt, err, iam.ErrAccessDenied)
	})
}

func TestCreateAccessKey(t *testing.T) {
	contract.CreateAccessKey(t, newBackend)

	t.Run("WhenAccessIsDenied", func(tt *testing.T) {
		// Setup
		c := newClient(tt, errorBody(http.StatusForbidden, "AccessDenied"))

		// Execute
		key, err := c.CreateAccessKey(tt.Context(), user)

		// Assert
		assert.ErrorIs(tt, err, iam.ErrAccessDenied)
		assert.Empty(tt, key)
	})

	t.Run("WhenTheStoreAnswersWithoutAKey", func(tt *testing.T) {
		// Setup
		c := newClient(tt, keyResponse(""))

		// Execute
		key, err := c.CreateAccessKey(tt.Context(), user)

		// Assert
		require.Error(tt, err)
		assert.Empty(tt, key)
	})

	t.Run("WhenTheKeyHasNoID", func(tt *testing.T) {
		// Setup
		c := newClient(tt, keyResponse("<AccessKey><SecretAccessKey>s</SecretAccessKey></AccessKey>"))

		// Execute
		key, err := c.CreateAccessKey(tt.Context(), user)

		// Assert
		require.Error(tt, err)
		assert.Empty(tt, key)
	})

	t.Run("WhenTheKeyHasNoSecret", func(tt *testing.T) {
		// Setup
		c := newClient(tt, keyResponse("<AccessKey><AccessKeyId>AKIAX</AccessKeyId></AccessKey>"))

		// Execute
		key, err := c.CreateAccessKey(tt.Context(), user)

		// Assert
		require.Error(tt, err)
		assert.Empty(tt, key)
	})
}

func TestDeleteAccessKey(t *testing.T) {
	contract.DeleteAccessKey(t, newBackend)

	t.Run("WhenKeyIsMissingInTheStore", func(tt *testing.T) {
		// Setup
		c := newClient(tt, errorBody(http.StatusNotFound, "NoSuchEntity"))

		// Execute
		err := c.DeleteAccessKey(tt.Context(), user, "AKIAMISSING")

		// Assert
		assert.ErrorIs(tt, err, iam.ErrNotFound)
	})

	t.Run("WhenAccessIsDenied", func(tt *testing.T) {
		// Setup
		c := newClient(tt, errorBody(http.StatusForbidden, "AccessDenied"))

		// Execute
		err := c.DeleteAccessKey(tt.Context(), user, "AKIAMISSING")

		// Assert
		assert.ErrorIs(tt, err, iam.ErrAccessDenied)
	})
}
