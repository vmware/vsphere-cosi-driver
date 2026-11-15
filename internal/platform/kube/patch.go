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

package kube

import (
	"context"

	"k8s.io/apimachinery/pkg/api/equality"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// AddFinalizer adds the finalizer to obj. A reconciler calls it before its first change outside
// the cluster, so a delete cannot skip the cleanup. The patch fails with a conflict when obj has
// changed since it was read.
func AddFinalizer(ctx context.Context, c client.Client, obj client.Object, finalizer string) error {
	return patchFinalizer(ctx, c, obj, finalizer, controllerutil.AddFinalizer)
}

// RemoveFinalizer removes the finalizer from obj. A reconciler calls it once the cleanup is
// done, with the same conflict check as AddFinalizer.
func RemoveFinalizer(ctx context.Context, c client.Client, obj client.Object, finalizer string) error {
	return patchFinalizer(ctx, c, obj, finalizer, controllerutil.RemoveFinalizer)
}

// patchFinalizer patches with an optimistic lock, so another finalizer added in between is never
// dropped.
func patchFinalizer(ctx context.Context, c client.Client, obj client.Object, finalizer string, change func(client.Object, string) bool) error {
	base := obj.DeepCopyObject().(client.Object)
	change(obj, finalizer)
	return c.Patch(ctx, obj, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{}))
}

// PatchStatus runs mutate, which must change only the status of obj, and patches the status. A
// reconciler calls it on every pass; it makes no request when mutate changed nothing.
func PatchStatus(ctx context.Context, c client.Client, obj client.Object, mutate func()) error {
	base := obj.DeepCopyObject().(client.Object)
	mutate()
	if equality.Semantic.DeepEqual(base, obj) {
		return nil
	}
	return c.Status().Patch(ctx, obj, client.MergeFrom(base))
}
