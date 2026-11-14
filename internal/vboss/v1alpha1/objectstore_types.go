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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ObjectStoreSpec holds the fields of the vSAN ObjectStore spec the operator reads.
type ObjectStoreSpec struct {
	EnableTLS     bool                        `json:"enableTLS,omitempty"`
	CertSecretRef corev1.LocalObjectReference `json:"certSecretRef,omitempty"`
	Domain        string                      `json:"domain,omitempty"`
	MaxBuckets    int32                       `json:"maxBuckets,omitempty"`
}

// ExternalS3Endpoint is where S3 clients reach the store. FQDN is a full URL.
type ExternalS3Endpoint struct {
	FQDN string `json:"fqdn,omitempty"`
}

// ObjectStoreStatus holds the fields of the vSAN ObjectStore status the operator reads.
type ObjectStoreStatus struct {
	HealthState        string             `json:"healthState,omitempty"`
	ExternalS3Endpoint ExternalS3Endpoint `json:"externalS3Endpoint,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

// ObjectStore is the operator's read-only view of a vSAN object store. It is owned by the
// vSAN team; the operator never writes it.
// +kubebuilder:object:root=true
type ObjectStore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ObjectStoreSpec   `json:"spec,omitempty"`
	Status ObjectStoreStatus `json:"status,omitempty"`
}

// ObjectStoreList is a list of ObjectStore.
// +kubebuilder:object:root=true
type ObjectStoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ObjectStore `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ObjectStore{}, &ObjectStoreList{})
}
