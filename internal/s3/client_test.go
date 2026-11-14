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

package s3_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmware/vsphere-cosi-driver/internal/s3"
	"github.com/vmware/vsphere-cosi-driver/internal/s3/contract"
	"github.com/vmware/vsphere-cosi-driver/internal/s3/fake"
)

const (
	testAccessKey = "AKIATESTKEY"
	testSecretKey = "super-secret-value"
	bucket        = "bucket-a"
)

// newClient returns a client that talks to a server running handler.
func newClient(t *testing.T, handler http.Handler) s3.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := s3.New(srv.URL, testAccessKey, testSecretKey, srv.Client())
	require.NoError(t, err)
	return c
}

// newBackend runs the real client against the fake's HTTP handler and its in-memory state.
func newBackend(t *testing.T) contract.Backend {
	t.Helper()
	store := fake.New()
	return contract.Backend{Client: newClient(t, fake.NewHandler(store)), MakeNotEmpty: store.AddObject}
}

// errorBody answers every request with an S3 error body.
func errorBody(status int, code string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(status)
		_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>%s</Code><Message>m</Message><RequestId>r</RequestId></Error>`, code)
	}
}

// statusOnly answers every request with a status and no body, as a HEAD does.
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
	for _, sentinel := range []error{
		s3.ErrNotFound, s3.ErrAlreadyExists, s3.ErrNotEmpty, s3.ErrInvalidName, s3.ErrAccessDenied, s3.ErrUnavailable,
	} {
		assert.NotErrorIs(t, err, sentinel)
	}
}

func TestNew(t *testing.T) {
	t.Run("WhenEndpointHasSchemeAndPort", func(tt *testing.T) {
		// Execute
		c, err := s3.New("https://s3.example.test:9021", testAccessKey, testSecretKey, nil)

		// Assert
		require.NoError(tt, err)
		assert.NotNil(tt, c)
	})

	t.Run("WhenEndpointIsEmpty", func(tt *testing.T) {
		// Execute
		c, err := s3.New("", testAccessKey, testSecretKey, nil)

		// Assert
		require.Error(tt, err)
		assert.Nil(tt, c)
	})

	t.Run("WhenEndpointHasNoScheme", func(tt *testing.T) {
		// Execute
		c, err := s3.New("s3.example.test:9021", testAccessKey, testSecretKey, nil)

		// Assert
		require.Error(tt, err)
		assert.Nil(tt, c)
	})

	t.Run("WhenEndpointHasOtherScheme", func(tt *testing.T) {
		// Execute
		c, err := s3.New("ftp://s3.example.test", testAccessKey, testSecretKey, nil)

		// Assert
		require.Error(tt, err)
		assert.Nil(tt, c)
	})

	t.Run("WhenEndpointHasNoHost", func(tt *testing.T) {
		// Execute
		c, err := s3.New("https://", testAccessKey, testSecretKey, nil)

		// Assert
		require.Error(tt, err)
		assert.Nil(tt, c)
	})
}

func TestBucketExists(t *testing.T) {
	contract.BucketExists(t, newBackend)

	t.Run("WhenBucketIsAddressed_UsePathStyle", func(tt *testing.T) {
		// Setup
		var method, path, host string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			method, path, host = r.Method, r.URL.Path, r.Host
		}))
		tt.Cleanup(srv.Close)
		c, err := s3.New(srv.URL, testAccessKey, testSecretKey, srv.Client())
		require.NoError(tt, err)

		// Execute
		exists, err := c.BucketExists(tt.Context(), bucket)

		// Assert
		require.NoError(tt, err)
		assert.True(tt, exists)
		assert.Equal(tt, http.MethodHead, method)
		assert.Equal(tt, "/"+bucket, path)
		assert.Equal(tt, strings.TrimPrefix(srv.URL, "http://"), host, "the bucket name must not become part of the host")
	})

	t.Run("WhenAccessIsDenied", func(tt *testing.T) {
		// Setup
		c := newClient(tt, statusOnly(http.StatusForbidden))

		// Execute
		_, err := c.BucketExists(tt.Context(), bucket)

		// Assert
		assert.ErrorIs(tt, err, s3.ErrAccessDenied)
		assertNoCredentials(tt, err)
	})

	t.Run("WhenStoreFails", func(tt *testing.T) {
		// Setup
		c := newClient(tt, statusOnly(http.StatusInternalServerError))

		// Execute
		_, err := c.BucketExists(tt.Context(), bucket)

		// Assert
		assert.ErrorIs(tt, err, s3.ErrUnavailable)
	})

	t.Run("WhenRequestIsBad", func(tt *testing.T) {
		// Setup
		c := newClient(tt, statusOnly(http.StatusBadRequest))

		// Execute
		_, err := c.BucketExists(tt.Context(), bucket)

		// Assert
		assert.ErrorIs(tt, err, s3.ErrInvalidName)
	})
}

func TestCreateBucket(t *testing.T) {
	contract.CreateBucket(t, newBackend)

	t.Run("WhenNameIsTakenByAnotherOwner", func(tt *testing.T) {
		// Setup
		c := newClient(tt, errorBody(http.StatusConflict, "BucketAlreadyExists"))

		// Execute
		err := c.CreateBucket(tt.Context(), bucket)

		// Assert
		assert.ErrorIs(tt, err, s3.ErrAlreadyExists)
	})

	t.Run("WhenNameIsOwnedByTheCaller", func(tt *testing.T) {
		// Setup
		c := newClient(tt, errorBody(http.StatusConflict, "BucketAlreadyOwnedByYou"))

		// Execute
		err := c.CreateBucket(tt.Context(), bucket)

		// Assert
		assert.ErrorIs(tt, err, s3.ErrAlreadyExists)
	})

	t.Run("WhenNameIsRejected", func(tt *testing.T) {
		// Setup
		c := newClient(tt, errorBody(http.StatusBadRequest, "InvalidBucketName"))

		// Execute
		err := c.CreateBucket(tt.Context(), bucket)

		// Assert
		assert.ErrorIs(tt, err, s3.ErrInvalidName)
	})

	t.Run("WhenAccessIsDenied", func(tt *testing.T) {
		// Setup
		c := newClient(tt, errorBody(http.StatusForbidden, "AccessDenied"))

		// Execute
		err := c.CreateBucket(tt.Context(), bucket)

		// Assert
		assert.ErrorIs(tt, err, s3.ErrAccessDenied)
		assertNoCredentials(tt, err)
	})

	t.Run("WhenSignatureDoesNotMatch", func(tt *testing.T) {
		// Setup
		c := newClient(tt, errorBody(http.StatusForbidden, "SignatureDoesNotMatch"))

		// Execute
		err := c.CreateBucket(tt.Context(), bucket)

		// Assert
		assert.ErrorIs(tt, err, s3.ErrAccessDenied)
		assertNoCredentials(tt, err)
	})

	t.Run("WhenStoreFails", func(tt *testing.T) {
		// Setup
		c := newClient(tt, errorBody(http.StatusInternalServerError, "InternalError"))

		// Execute
		err := c.CreateBucket(tt.Context(), bucket)

		// Assert
		assert.ErrorIs(tt, err, s3.ErrUnavailable)
	})

	t.Run("WhenStoreIsUnavailable", func(tt *testing.T) {
		// Setup
		c := newClient(tt, errorBody(http.StatusServiceUnavailable, "ServiceUnavailable"))

		// Execute
		err := c.CreateBucket(tt.Context(), bucket)

		// Assert
		assert.ErrorIs(tt, err, s3.ErrUnavailable)
	})

	t.Run("WhenStoreIsUnreachable", func(tt *testing.T) {
		// Setup
		c, err := s3.New(unreachableEndpoint(tt), testAccessKey, testSecretKey, nil)
		require.NoError(tt, err)

		// Execute
		err = c.CreateBucket(tt.Context(), bucket)

		// Assert
		assert.ErrorIs(tt, err, s3.ErrUnavailable)
	})

	t.Run("WhenStoreAnswersWithAnUnknownCode", func(tt *testing.T) {
		// Setup
		c := newClient(tt, errorBody(http.StatusConflict, "OperationAborted"))

		// Execute
		err := c.CreateBucket(tt.Context(), bucket)

		// Assert
		assertUnclassified(tt, err)
	})
}

func TestDeleteBucket(t *testing.T) {
	contract.DeleteBucket(t, newBackend)

	t.Run("WhenBucketIsMissing", func(tt *testing.T) {
		// Setup
		c := newClient(tt, errorBody(http.StatusNotFound, "NoSuchBucket"))

		// Execute
		err := c.DeleteBucket(tt.Context(), bucket)

		// Assert
		assert.ErrorIs(tt, err, s3.ErrNotFound)
	})

	t.Run("WhenBucketHoldsObjects", func(tt *testing.T) {
		// Setup
		c := newClient(tt, errorBody(http.StatusConflict, "BucketNotEmpty"))

		// Execute
		err := c.DeleteBucket(tt.Context(), bucket)

		// Assert
		assert.ErrorIs(tt, err, s3.ErrNotEmpty)
	})

	t.Run("WhenAccessIsDenied", func(tt *testing.T) {
		// Setup
		c := newClient(tt, errorBody(http.StatusForbidden, "AccessDenied"))

		// Execute
		err := c.DeleteBucket(tt.Context(), bucket)

		// Assert
		assert.ErrorIs(tt, err, s3.ErrAccessDenied)
		assertNoCredentials(tt, err)
	})

	t.Run("WhenStoreIsUnavailable", func(tt *testing.T) {
		// Setup
		c := newClient(tt, errorBody(http.StatusServiceUnavailable, "ServiceUnavailable"))

		// Execute
		err := c.DeleteBucket(tt.Context(), bucket)

		// Assert
		assert.ErrorIs(tt, err, s3.ErrUnavailable)
	})
}
