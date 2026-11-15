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

package kube_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmware/vsphere-cosi-driver/internal/platform/kube"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

const (
	finalizer      = "example.test/protect"
	otherFinalizer = "example.test/other"
)

var errBoom = errors.New("boom")

// newClient serves one Pod, which stands in for any object with metadata and a status.
func newClient(t *testing.T, funcs interceptor.Funcs, finalizers ...string) client.Client {
	t.Helper()
	scheme, err := kube.NewScheme()
	require.NoError(t, err)
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "ns", Finalizers: finalizers}}
	return clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pod).
		WithStatusSubresource(&corev1.Pod{}).
		WithInterceptorFuncs(funcs).
		Build()
}

func getPod(t *testing.T, c client.Client) *corev1.Pod {
	t.Helper()
	pod := &corev1.Pod{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "pod"}, pod))
	return pod
}

// addFinalizerBehindTheCallersBack changes the stored Pod after the caller has read it.
func addFinalizerBehindTheCallersBack(t *testing.T, c client.Client) {
	t.Helper()
	pod := getPod(t, c)
	pod.Finalizers = append(pod.Finalizers, otherFinalizer)
	require.NoError(t, c.Update(context.Background(), pod))
}

func TestAddFinalizer(t *testing.T) {
	t.Run("WhenTheObjectHasNone", func(tt *testing.T) {
		// Setup
		c := newClient(tt, interceptor.Funcs{})
		pod := getPod(tt, c)

		// Execute
		err := kube.AddFinalizer(tt.Context(), c, pod, finalizer)

		// Assert
		require.NoError(tt, err)
		assert.Equal(tt, []string{finalizer}, getPod(tt, c).Finalizers)
	})

	t.Run("WhenAnotherFinalizerIsAddedMeanwhile", func(tt *testing.T) {
		// Setup
		c := newClient(tt, interceptor.Funcs{})
		pod := getPod(tt, c)
		addFinalizerBehindTheCallersBack(tt, c)

		// Execute
		err := kube.AddFinalizer(tt.Context(), c, pod, finalizer)

		// Assert
		assert.True(tt, apierrors.IsConflict(err))
		assert.Equal(tt, []string{otherFinalizer}, getPod(tt, c).Finalizers)
	})

	t.Run("WhenThePatchFails", func(tt *testing.T) {
		// Setup
		c := newClient(tt, interceptor.Funcs{Patch: func(context.Context, client.WithWatch, client.Object, client.Patch, ...client.PatchOption) error {
			return errBoom
		}})
		pod := getPod(tt, c)

		// Execute
		err := kube.AddFinalizer(tt.Context(), c, pod, finalizer)

		// Assert
		assert.ErrorIs(tt, err, errBoom)
	})
}

func TestRemoveFinalizer(t *testing.T) {
	t.Run("WhenTheObjectHasOthers", func(tt *testing.T) {
		// Setup
		c := newClient(tt, interceptor.Funcs{}, finalizer, otherFinalizer)
		pod := getPod(tt, c)

		// Execute
		err := kube.RemoveFinalizer(tt.Context(), c, pod, finalizer)

		// Assert
		require.NoError(tt, err)
		assert.Equal(tt, []string{otherFinalizer}, getPod(tt, c).Finalizers)
	})

	t.Run("WhenAnotherFinalizerIsAddedMeanwhile", func(tt *testing.T) {
		// Setup
		c := newClient(tt, interceptor.Funcs{}, finalizer)
		pod := getPod(tt, c)
		addFinalizerBehindTheCallersBack(tt, c)

		// Execute
		err := kube.RemoveFinalizer(tt.Context(), c, pod, finalizer)

		// Assert
		assert.True(tt, apierrors.IsConflict(err))
		assert.Equal(tt, []string{finalizer, otherFinalizer}, getPod(tt, c).Finalizers)
	})

	t.Run("WhenThePatchFails", func(tt *testing.T) {
		// Setup
		c := newClient(tt, interceptor.Funcs{Patch: func(context.Context, client.WithWatch, client.Object, client.Patch, ...client.PatchOption) error {
			return errBoom
		}}, finalizer)
		pod := getPod(tt, c)

		// Execute
		err := kube.RemoveFinalizer(tt.Context(), c, pod, finalizer)

		// Assert
		assert.ErrorIs(tt, err, errBoom)
	})
}

func TestPatchStatus(t *testing.T) {
	t.Run("WhenTheStatusChanges", func(tt *testing.T) {
		// Setup
		c := newClient(tt, interceptor.Funcs{})
		pod := getPod(tt, c)

		// Execute
		err := kube.PatchStatus(tt.Context(), c, pod, func() { pod.Status.Phase = corev1.PodRunning })

		// Assert
		require.NoError(tt, err)
		assert.Equal(tt, corev1.PodRunning, getPod(tt, c).Status.Phase)
	})

	t.Run("WhenTheStatusIsUnchanged", func(tt *testing.T) {
		// Setup
		calls := 0
		c := newClient(tt, interceptor.Funcs{SubResourcePatch: func(context.Context, client.Client, string, client.Object, client.Patch, ...client.SubResourcePatchOption) error {
			calls++
			return nil
		}})
		pod := getPod(tt, c)

		// Execute
		err := kube.PatchStatus(tt.Context(), c, pod, func() {})

		// Assert
		require.NoError(tt, err)
		assert.Zero(tt, calls)
	})

	t.Run("WhenThePatchFails", func(tt *testing.T) {
		// Setup
		c := newClient(tt, interceptor.Funcs{SubResourcePatch: func(context.Context, client.Client, string, client.Object, client.Patch, ...client.SubResourcePatchOption) error {
			return errBoom
		}})
		pod := getPod(tt, c)

		// Execute
		err := kube.PatchStatus(tt.Context(), c, pod, func() { pod.Status.Phase = corev1.PodRunning })

		// Assert
		assert.ErrorIs(tt, err, errBoom)
	})
}
