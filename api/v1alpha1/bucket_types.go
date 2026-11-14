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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ObjectProtocol is an object store protocol a bucket can speak.
// +kubebuilder:validation:Enum:=S3;Azure;GCS
type ObjectProtocol string

const (
	ProtocolS3    ObjectProtocol = "S3"
	ProtocolAzure ObjectProtocol = "Azure"
	ProtocolGCS   ObjectProtocol = "GCS"
)

// DeletionPolicy says what happens to the backend bucket when the ObjectStoreBucket is deleted.
// +kubebuilder:validation:Enum:=Retain;Delete
type DeletionPolicy string

const (
	DeletionPolicyRetain DeletionPolicy = "Retain"
	DeletionPolicyDelete DeletionPolicy = "Delete"
)

// BucketClaimReference points at the BucketClaim this bucket was created for.
// +kubebuilder:validation:XValidation:message="namespace cannot be removed once set",rule="!has(oldSelf.namespace) || has(self.namespace)"
// +kubebuilder:validation:XValidation:message="uid cannot be removed once set",rule="!has(oldSelf.uid) || has(self.uid)"
type BucketClaimReference struct {
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:XValidation:message="name must be a valid resource name",rule="!format.dns1123Subdomain().validate(self).hasValue()"
	// +kubebuilder:validation:XValidation:message="name is immutable",rule="self == oldSelf"
	Name string `json:"name,omitempty"`

	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:XValidation:message="namespace must be a valid namespace name",rule="!format.dns1123Label().validate(self).hasValue()"
	// +kubebuilder:validation:XValidation:message="namespace is immutable",rule="self == oldSelf"
	Namespace string `json:"namespace,omitempty"`

	// +optional
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:MinLength=36
	// +kubebuilder:validation:MaxLength=36
	// +kubebuilder:validation:Pattern=`^[0-9a-f]{8}-([0-9a-f]{4}\-){3}[0-9a-f]{12}$`
	// +kubebuilder:validation:XValidation:message="uid is immutable once set",rule="oldSelf == '' || self == oldSelf"
	UID types.UID `json:"uid,omitempty"`
}

// BucketSpec is the desired state of an ObjectStoreBucket.
// +kubebuilder:validation:XValidation:message="parameters map cannot be added or removed after creation",rule="has(oldSelf.parameters) == has(self.parameters)"
// +kubebuilder:validation:XValidation:message="protocols list cannot be added or removed after creation",rule="has(oldSelf.protocols) == has(self.protocols)"
// +kubebuilder:validation:XValidation:message="existingBucketID cannot be added or removed after creation",rule="has(oldSelf.existingBucketID) == has(self.existingBucketID)"
type BucketSpec struct {
	// DriverName names the driver that serves this bucket.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9]([a-zA-Z0-9\-\.]{0,61}[a-zA-Z0-9])?$`
	// +kubebuilder:validation:XValidation:message="driverName is immutable",rule="self == oldSelf"
	DriverName string `json:"driverName,omitempty"`

	// DeletionPolicy is mutable so an admin can change it after creation.
	// +required
	DeletionPolicy DeletionPolicy `json:"deletionPolicy,omitempty"`

	// Parameters is an opaque, driver-specific map.
	// +optional
	// +kubebuilder:validation:MinProperties=1
	// +kubebuilder:validation:MaxProperties=512
	// +kubebuilder:validation:XValidation:message="parameters map is immutable",rule="self == oldSelf"
	Parameters map[string]string `json:"parameters,omitempty"`

	// Protocols the bucket must support.
	// +optional
	// +listType=set
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=3
	// +kubebuilder:validation:XValidation:message="protocols list is immutable",rule="self == oldSelf"
	Protocols []ObjectProtocol `json:"protocols,omitempty"`

	// BucketClaimRef is the claim this bucket serves.
	// +required
	BucketClaimRef BucketClaimReference `json:"bucketClaimRef,omitzero"`

	// ExistingBucketID selects a pre-existing backend bucket. The operator does not support it
	// yet and treats a set value as a terminal error.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9/._-]+$`
	// +kubebuilder:validation:XValidation:message="existingBucketID is immutable",rule="self == oldSelf"
	ExistingBucketID string `json:"existingBucketID,omitempty"`
}

// TimestampedError records a failure and when it happened.
type TimestampedError struct {
	// +optional
	Time *metav1.Time `json:"time,omitempty"`

	// +optional
	// +kubebuilder:validation:MaxLength=1024
	Message *string `json:"message,omitempty"`
}

// BucketStatus is the observed state of an ObjectStoreBucket.
type BucketStatus struct {
	// ReadyToUse is true once the backend bucket exists.
	// +optional
	ReadyToUse *bool `json:"readyToUse,omitempty"`

	// BucketID is the backend bucket name.
	// +optional
	BucketID string `json:"bucketID,omitempty"`

	// Protocols the backend bucket supports.
	// +optional
	// +listType=set
	Protocols []ObjectProtocol `json:"protocols,omitempty"`

	// BucketInfo carries protocol details such as the endpoint.
	// +optional
	BucketInfo map[string]string `json:"bucketInfo,omitempty"`

	// Error is the last failure, cleared on success.
	// +optional
	Error *TimestampedError `json:"error,omitempty"`

	// Conditions report why the bucket is or is not ready. The operator writes a single Ready
	// condition whose reason names the cause.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ObjectStoreBucket is a bucket requested on an object store. It lives in the store's namespace.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
type ObjectStoreBucket struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BucketSpec   `json:"spec"`
	Status BucketStatus `json:"status,omitempty"`
}

// ObjectStoreBucketList is a list of ObjectStoreBucket.
// +kubebuilder:object:root=true
type ObjectStoreBucketList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ObjectStoreBucket `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ObjectStoreBucket{}, &ObjectStoreBucketList{})
}
