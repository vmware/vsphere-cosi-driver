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

package fake_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vmware/vsphere-cosi-driver/internal/s3/contract"
	"github.com/vmware/vsphere-cosi-driver/internal/s3/fake"
)

func newBackend(*testing.T) contract.Backend {
	c := fake.New()
	return contract.Backend{Client: c, MakeNotEmpty: c.AddObject}
}

func TestBucketExists(t *testing.T) { contract.BucketExists(t, newBackend) }

func TestCreateBucket(t *testing.T) { contract.CreateBucket(t, newBackend) }

func TestDeleteBucket(t *testing.T) { contract.DeleteBucket(t, newBackend) }

func TestHandler(t *testing.T) {
	serve := func(h http.Handler, method, path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		return rec
	}

	t.Run("WhenHeadFindsNoBucket", func(tt *testing.T) {
		// Execute
		rec := serve(fake.NewHandler(fake.New()), http.MethodHead, "/bucket-a")

		// Assert
		assert.Equal(tt, http.StatusNotFound, rec.Code)
	})

	t.Run("WhenPutGetsInvalidName", func(tt *testing.T) {
		// Execute
		rec := serve(fake.NewHandler(fake.New()), http.MethodPut, "/Bad_Name")

		// Assert
		assert.Equal(tt, http.StatusBadRequest, rec.Code)
		assert.Contains(tt, rec.Body.String(), "<Code>InvalidBucketName</Code>")
	})

	t.Run("WhenMethodIsNotABucketCall", func(tt *testing.T) {
		// Execute
		rec := serve(fake.NewHandler(fake.New()), http.MethodPost, "/bucket-a")

		// Assert
		assert.Equal(tt, http.StatusMethodNotAllowed, rec.Code)
	})
}
