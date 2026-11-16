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
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vmware/vsphere-cosi-driver/internal/iam/contract"
	"github.com/vmware/vsphere-cosi-driver/internal/iam/fake"
)

func newBackend(*testing.T) contract.Backend {
	c := fake.New()
	return contract.Backend{Client: c, HasUser: c.HasUser, Policy: c.Policy, HasKey: c.HasKey}
}

func TestCreateUser(t *testing.T) { contract.CreateUser(t, newBackend) }

func TestPutUserPolicy(t *testing.T) { contract.PutUserPolicy(t, newBackend) }

func TestCreateAccessKey(t *testing.T) { contract.CreateAccessKey(t, newBackend) }

func TestDeleteAccessKey(t *testing.T) { contract.DeleteAccessKey(t, newBackend) }

func TestPolicy(t *testing.T) {
	t.Run("WhenUserDoesNotExist", func(tt *testing.T) {
		// Execute
		document, ok := fake.New().Policy("user-a", "policy-a")

		// Assert
		assert.False(tt, ok)
		assert.Empty(tt, document)
	})
}

func TestHasKey(t *testing.T) {
	t.Run("WhenUserDoesNotExist", func(tt *testing.T) {
		// Execute
		ok := fake.New().HasKey("user-a", "AKIAX")

		// Assert
		assert.False(tt, ok)
	})
}

func TestHandler(t *testing.T) {
	post := func(h http.Handler, form url.Values) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	t.Run("WhenUserIsCreated", func(tt *testing.T) {
		// Execute
		rec := post(fake.NewHandler(fake.New()), url.Values{"Action": {"CreateUser"}, "UserName": {"user-a"}})

		// Assert
		assert.Equal(tt, http.StatusOK, rec.Code)
		assert.Contains(tt, rec.Body.String(), "<CreateUserResponse")
	})

	t.Run("WhenUserDoesNotExist", func(tt *testing.T) {
		// Execute
		rec := post(fake.NewHandler(fake.New()), url.Values{"Action": {"CreateAccessKey"}, "UserName": {"user-a"}})

		// Assert
		assert.Equal(tt, http.StatusNotFound, rec.Code)
		assert.Contains(tt, rec.Body.String(), "<Code>NoSuchEntity</Code>")
	})

	t.Run("WhenActionIsUnknown", func(tt *testing.T) {
		// Execute
		rec := post(fake.NewHandler(fake.New()), url.Values{"Action": {"ListAccessKeys"}, "UserName": {"user-a"}})

		// Assert
		assert.Equal(tt, http.StatusBadRequest, rec.Code)
		assert.Contains(tt, rec.Body.String(), "<Code>InvalidAction</Code>")
	})

	t.Run("WhenBodyIsMalformed", func(tt *testing.T) {
		// Setup
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("Action=%zz"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()

		// Execute
		fake.NewHandler(fake.New()).ServeHTTP(rec, req)

		// Assert
		assert.Equal(tt, http.StatusBadRequest, rec.Code)
		assert.Contains(tt, rec.Body.String(), "<Code>ValidationError</Code>")
	})

	t.Run("WhenMethodIsNotPost", func(tt *testing.T) {
		// Setup
		rec := httptest.NewRecorder()

		// Execute
		fake.NewHandler(fake.New()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

		// Assert
		assert.Equal(tt, http.StatusMethodNotAllowed, rec.Code)
	})
}
