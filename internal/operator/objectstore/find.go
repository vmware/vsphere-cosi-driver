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

// Package objectstore finds the vSAN ObjectStore an operator serves.
package objectstore

import (
	"context"
	"errors"
	"fmt"

	vboss "github.com/vmware/vsphere-cosi-driver/internal/vboss/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ErrMultiple is returned when the namespace holds more than one ObjectStore.
var ErrMultiple = errors.New("more than one ObjectStore in the namespace")

// Find returns the one ObjectStore in the namespace, or nil when there is none. Each operator
// serves a single store, so more than one is an error.
func Find(ctx context.Context, c client.Reader, namespace string) (*vboss.ObjectStore, error) {
	var stores vboss.ObjectStoreList
	if err := c.List(ctx, &stores, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("list ObjectStores: %w", err)
	}
	switch len(stores.Items) {
	case 0:
		return nil, nil
	case 1:
		return &stores.Items[0], nil
	default:
		return nil, ErrMultiple
	}
}
