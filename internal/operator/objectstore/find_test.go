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

package objectstore_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmware/vsphere-cosi-driver/internal/operator/objectstore"
	"github.com/vmware/vsphere-cosi-driver/internal/platform/kube"
	vboss "github.com/vmware/vsphere-cosi-driver/internal/vboss/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func newStore(name, namespace string) *vboss.ObjectStore {
	return &vboss.ObjectStore{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
}

func newClient(t *testing.T, funcs interceptor.Funcs, objs ...client.Object) client.Client {
	t.Helper()
	scheme, err := kube.NewScheme()
	require.NoError(t, err)
	return clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).WithInterceptorFuncs(funcs).Build()
}

func TestFind(t *testing.T) {
	t.Run("WhenNamespaceHasOneStore", func(tt *testing.T) {
		// Setup
		c := newClient(tt, interceptor.Funcs{}, newStore("store", "store-a"))

		// Execute
		got, err := objectstore.Find(tt.Context(), c, "store-a")

		// Assert
		require.NoError(tt, err)
		require.NotNil(tt, got)
		assert.Equal(tt, "store", got.Name)
	})

	t.Run("WhenNamespaceHasNoStore", func(tt *testing.T) {
		// Setup
		c := newClient(tt, interceptor.Funcs{}, newStore("store", "store-b"))

		// Execute
		got, err := objectstore.Find(tt.Context(), c, "store-a")

		// Assert
		require.NoError(tt, err)
		assert.Nil(tt, got)
	})

	t.Run("WhenNamespaceHasTwoStores", func(tt *testing.T) {
		// Setup
		c := newClient(tt, interceptor.Funcs{}, newStore("store", "store-a"), newStore("other", "store-a"))

		// Execute
		got, err := objectstore.Find(tt.Context(), c, "store-a")

		// Assert
		assert.ErrorIs(tt, err, objectstore.ErrMultiple)
		assert.Nil(tt, got)
	})

	t.Run("WhenListingFails", func(tt *testing.T) {
		// Setup
		errList := errors.New("api unavailable")
		c := newClient(tt, interceptor.Funcs{List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
			return errList
		}})

		// Execute
		got, err := objectstore.Find(tt.Context(), c, "store-a")

		// Assert
		assert.ErrorIs(tt, err, errList)
		assert.Nil(tt, got)
	})
}
