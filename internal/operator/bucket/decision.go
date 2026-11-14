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

// Package bucket holds the operator's rules for one ObjectStoreBucket. Decide is a pure function:
// the reconciler observes, asks what to do next, acts, and asks again with the result added.
package bucket

import (
	"errors"
	"fmt"
	"slices"

	"github.com/vmware/vsphere-cosi-driver/api/v1alpha1"
	"github.com/vmware/vsphere-cosi-driver/internal/platform/errs"
	"github.com/vmware/vsphere-cosi-driver/internal/s3"
	vboss "github.com/vmware/vsphere-cosi-driver/internal/vboss/v1alpha1"
)

// Reasons written to the Ready condition, and carried by terminal errors.
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

// healthReady is the ObjectStore health state in which buckets can be created.
const healthReady = "Ready"

// Op is an S3 call the reconciler makes.
type Op int

// The S3 calls. The zero value is no call.
const (
	OpNone Op = iota
	OpHead
	OpCreate
	OpDelete
)

// Call is an S3 call already made in this pass. Err is nil on success. A head that finds no
// bucket records s3.ErrNotFound.
type Call struct {
	Op  Op
	Err error
}

// Observation is everything Decide looks at.
type Observation struct {
	Bucket         *v1alpha1.ObjectStoreBucket
	Store          *vboss.ObjectStore // nil when the namespace has none
	HasCredentials bool
	Calls          []Call // made in this pass, oldest first
}

// Step is what the reconciler does next. The zero value is nothing.
type Step int

// The steps.
const (
	StepNone Step = iota
	StepHead
	StepCreate
	StepDelete
	StepReleaseFinalizer
)

// StatusPatch is the status to write. An empty Reason means leave the status as it is.
type StatusPatch struct {
	ReadyToUse bool
	Reason     string
	Message    string
}

// Outcome is the answer to Decide. Err is errs.Retryable or errs.Terminal, and nil when the
// bucket is settled or the next step can run.
type Outcome struct {
	Next   Step
	Status StatusPatch
	Err    error
}

// Decide returns what to do next for the observed bucket.
func Decide(obs Observation) Outcome {
	if !obs.Bucket.DeletionTimestamp.IsZero() {
		return decideDeletion(obs)
	}
	return decideProvisioning(obs)
}

func decideProvisioning(obs Observation) Outcome {
	b := obs.Bucket
	switch {
	case unsupported(b):
		return stop(ReasonUnsupported, "only S3 buckets created by the operator are supported", nil)
	case !s3.ValidName(b.Name):
		return stop(ReasonInvalidName, "the name is not a valid S3 bucket name", nil)
	case obs.Store == nil:
		return retry(ReasonStoreNotFound, "no object store in the namespace", nil)
	case obs.Store.Status.HealthState != healthReady:
		return retry(ReasonStoreNotReady, "the object store is not ready", nil)
	case !obs.HasCredentials:
		return retry(ReasonCredentialsUnavailable, "the object store credentials are not available", nil)
	}

	last, called := lastCall(obs.Calls)
	switch {
	case !called:
		return Outcome{Next: StepHead}
	case last.Op == OpCreate:
		return afterCreate(last)
	default:
		return afterHead(last, nameTaken(obs.Calls))
	}
}

func afterHead(head Call, nameTaken bool) Outcome {
	switch {
	case head.Err == nil:
		return ready()
	case nameTaken && (errors.Is(head.Err, s3.ErrNotFound) || errors.Is(head.Err, s3.ErrAccessDenied)):
		// Create said the name exists, yet the bucket is not ours to see: another account has it.
		return stop(ReasonBucketNameTaken, "the bucket name is used by another owner", head.Err)
	case errors.Is(head.Err, s3.ErrNotFound):
		// TODO(CNAS-12565): revisit the ownership check with the vSAN team
		return Outcome{Next: StepCreate}
	default:
		return failure(head.Err)
	}
}

func afterCreate(create Call) Outcome {
	switch {
	case create.Err == nil:
		return ready()
	case errors.Is(create.Err, s3.ErrAlreadyExists):
		return Outcome{Next: StepHead}
	default:
		return failure(create.Err)
	}
}

func decideDeletion(obs Observation) Outcome {
	b := obs.Bucket
	switch {
	case b.Spec.DeletionPolicy != v1alpha1.DeletionPolicyDelete,
		unsupported(b), !s3.ValidName(b.Name), obs.Store == nil:
		// Retained, never provisioned, or the store (and the bucket with it) is gone.
		return Outcome{Next: StepReleaseFinalizer}
	case !obs.HasCredentials:
		return retry(ReasonCredentialsUnavailable, "the object store credentials are not available", nil)
	}

	last, called := lastCall(obs.Calls)
	switch {
	case !called:
		return Outcome{Next: StepDelete, Status: StatusPatch{Reason: ReasonDeleting, Message: "deleting the bucket"}}
	case last.Err == nil, errors.Is(last.Err, s3.ErrNotFound):
		return Outcome{Next: StepReleaseFinalizer}
	case errors.Is(last.Err, s3.ErrNotEmpty):
		return retry(ReasonBucketNotEmpty, "the bucket still holds objects", last.Err)
	default:
		return failure(last.Err)
	}
}

// unsupported reports a spec this slice does not provision: static provisioning or non-S3.
func unsupported(b *v1alpha1.ObjectStoreBucket) bool {
	return b.Spec.ExistingBucketID != "" || !slices.Contains(b.Spec.Protocols, v1alpha1.ProtocolS3)
}

func lastCall(calls []Call) (Call, bool) {
	if len(calls) == 0 {
		return Call{}, false
	}
	return calls[len(calls)-1], true
}

// nameTaken reports whether a create in this pass found the name already in use.
func nameTaken(calls []Call) bool {
	return slices.ContainsFunc(calls, func(c Call) bool {
		return c.Op == OpCreate && errors.Is(c.Err, s3.ErrAlreadyExists)
	})
}

func ready() Outcome {
	return Outcome{Status: StatusPatch{ReadyToUse: true, Reason: ReasonProvisioned, Message: "the bucket is ready"}}
}

// failure classifies an S3 error. The message is fixed text for users; err stays for logs.
func failure(err error) Outcome {
	switch {
	case errors.Is(err, s3.ErrInvalidName):
		return stop(ReasonInvalidName, "the object store rejected the bucket name", err)
	case errors.Is(err, s3.ErrAccessDenied):
		return retry(ReasonAccessDenied, "the object store rejected the credentials", err)
	case errors.Is(err, s3.ErrUnavailable):
		return retry(ReasonStoreUnavailable, "the object store is unavailable", err)
	default:
		return retry(ReasonFailed, "the object store request failed", err)
	}
}

func retry(reason, message string, cause error) Outcome {
	return Outcome{Status: StatusPatch{Reason: reason, Message: message}, Err: errs.Retryable(describe(message, cause))}
}

func stop(reason, message string, cause error) Outcome {
	return Outcome{Status: StatusPatch{Reason: reason, Message: message}, Err: errs.Terminal(describe(message, cause), reason)}
}

func describe(message string, cause error) error {
	if cause == nil {
		return errors.New(message)
	}
	return fmt.Errorf("%s: %w", message, cause)
}
