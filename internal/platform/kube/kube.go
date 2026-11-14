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

// Package kube builds the scheme the operator's client and cache use.
package kube

import (
	"fmt"

	"github.com/vmware/vsphere-cosi-driver/api/v1alpha1"
	vboss "github.com/vmware/vsphere-cosi-driver/internal/vboss/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

// NewScheme returns a scheme with the built-in Kubernetes types, ObjectStoreBucket and the vSAN
// ObjectStore.
func NewScheme() (*runtime.Scheme, error) {
	s := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{
		clientgoscheme.AddToScheme,
		v1alpha1.AddToScheme,
		vboss.AddToScheme,
	} {
		if err := add(s); err != nil {
			return nil, fmt.Errorf("build scheme: %w", err)
		}
	}
	return s, nil
}
