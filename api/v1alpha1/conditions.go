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

// Finalizer allows the reconciler to gracefully handle the bucket creation and deletion.
const Finalizer = "cosi.vsphere.vmware.com/bucket-protection"

// ConditionReady is the condition type the operator writes to BucketStatus.Conditions.
const ConditionReady = "Ready"

// Reasons of the Ready condition. They are stable identifiers; the message carries the detail
// and clients should not parse it.
const (
	ReasonProvisioned            = "Provisioned"
	ReasonDeleting               = "Deleting"
	ReasonStoreNotFound          = "StoreNotFound"
	ReasonStoreNotReady          = "StoreNotReady"
	ReasonCredentialsUnavailable = "CredentialsUnavailable"
	ReasonStoreUnavailable       = "StoreUnavailable"
	ReasonAccessDenied           = "AccessDenied"
	ReasonBucketNameTaken        = "BucketNameTaken"
	ReasonInvalidName            = "InvalidBucketName"
	ReasonUnsupported            = "Unsupported"
	ReasonBucketNotEmpty         = "BucketNotEmpty"
	ReasonFailed                 = "Failed"
)
