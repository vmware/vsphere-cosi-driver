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

package bucket

import (
	"context"

	"github.com/vmware/vsphere-cosi-driver/api/v1alpha1"
	"github.com/vmware/vsphere-cosi-driver/internal/operator/credentials"
	"github.com/vmware/vsphere-cosi-driver/internal/operator/objectstore"
	"github.com/vmware/vsphere-cosi-driver/internal/platform/errs"
	"github.com/vmware/vsphere-cosi-driver/internal/platform/kube"
	"github.com/vmware/vsphere-cosi-driver/internal/s3"
	vboss "github.com/vmware/vsphere-cosi-driver/internal/vboss/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Reconciler creates and deletes the buckets of one store on behalf of ObjectStoreBuckets in the
// store's namespace.
type Reconciler struct {
	Client client.Client
	// NewS3Client connects to the store. It runs at most once per reconcile.
	NewS3Client func(endpoint, accessKey, secretKey string) (s3.Client, error)
	// CredentialsDir is where the operator's own credentials Secret is mounted, one file per key.
	// It is read on every reconcile, so a changed Secret is picked up.
	CredentialsDir string
}

// SetupWithManager registers the reconciler. A change to the ObjectStore wakes every bucket in
// its namespace, so buckets do not wait out a backoff once the store is ready.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("objectstorebucket").
		For(&v1alpha1.ObjectStoreBucket{}).
		Watches(&vboss.ObjectStore{}, handler.EnqueueRequestsFromMapFunc(r.MapStoreToBuckets)).
		Complete(r)
}

// MapStoreToBuckets returns a request for every bucket in the namespace of the changed store.
func (r *Reconciler) MapStoreToBuckets(ctx context.Context, store client.Object) []reconcile.Request {
	var buckets v1alpha1.ObjectStoreBucketList
	// The manager's client serves this list from its informer cache.
	if err := r.Client.List(ctx, &buckets, client.InNamespace(store.GetNamespace())); err != nil {
		log.FromContext(ctx).Error(err, "failed to list buckets after an ObjectStore change")
		return nil
	}
	requests := make([]reconcile.Request, 0, len(buckets.Items))
	for i := range buckets.Items {
		requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&buckets.Items[i])})
	}
	return requests
}

// +kubebuilder:rbac:groups=cosi.vsphere.vmware.com,resources=objectstorebuckets,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=cosi.vsphere.vmware.com,resources=objectstorebuckets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cosi.vsphere.vmware.com,resources=objectstorebuckets/finalizers,verbs=update
// +kubebuilder:rbac:groups=objectstore.vsan.broadcom.com,resources=objectstores,verbs=get;list;watch

// Reconcile creates or deletes the backend bucket for an ObjectStoreBucket and reports the result
// in its status.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	b := &v1alpha1.ObjectStoreBucket{}
	if err := r.Client.Get(ctx, req.NamespacedName, b); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	store, err := objectstore.Find(ctx, r.Client, req.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}
	accessKey, secretKey, err := credentials.Read(r.CredentialsDir)
	if err != nil {
		return ctrl.Result{}, err
	}

	obs := Observation{Bucket: b, Store: store, HasCredentials: accessKey != "" && secretKey != ""}
	var s3Client s3.Client
	for {
		out := Decide(obs)
		if err := r.writeStatus(ctx, b, out); err != nil {
			return ctrl.Result{}, err
		}
		var op Op
		switch out.Next {
		case StepAddFinalizer:
			if err := kube.AddFinalizer(ctx, r.Client, b, v1alpha1.Finalizer); err != nil {
				return ctrl.Result{}, err
			}
			continue
		case StepReleaseFinalizer:
			return ctrl.Result{}, client.IgnoreNotFound(kube.RemoveFinalizer(ctx, r.Client, b, v1alpha1.Finalizer))
		case StepHead:
			op = OpHead
		case StepCreate:
			op = OpCreate
		case StepDelete:
			op = OpDelete
		default:
			return finish(ctx, out)
		}

		call := Call{Op: op}
		if s3Client == nil {
			s3Client, call.Err = r.NewS3Client(store.Status.ExternalS3Endpoint.FQDN, accessKey, secretKey)
		}
		if call.Err == nil {
			call.Err = run(ctx, s3Client, op, backendName(b))
		}
		obs.Calls = append(obs.Calls, call)
	}
}

// finish turns the last outcome into a result. A terminal error is not returned: retrying cannot
// help until the bucket changes, and the status already says why.
func finish(ctx context.Context, out Outcome) (ctrl.Result, error) {
	if errs.IsTerminal(out.Err) {
		log.FromContext(ctx).Info("bucket cannot proceed until it changes", "reason", errs.Reason(out.Err), "cause", out.Err.Error())
		return ctrl.Result{}, nil
	}
	return ctrl.Result{}, out.Err
}

// run makes one call to the object store.
func run(ctx context.Context, c s3.Client, op Op, name string) error {
	switch op {
	case OpHead:
		exists, err := c.BucketExists(ctx, name)
		if err == nil && !exists {
			return s3.ErrNotFound
		}
		return err
	case OpCreate:
		return c.CreateBucket(ctx, name)
	default:
		return c.DeleteBucket(ctx, name)
	}
}

// writeStatus updates the bucket's status, and does nothing when it is already up to date.
func (r *Reconciler) writeStatus(ctx context.Context, b *v1alpha1.ObjectStoreBucket, out Outcome) error {
	if out.Status.Reason == "" {
		return nil
	}
	return kube.PatchStatus(ctx, r.Client, b, func() { applyStatus(b, out) })
}

// applyStatus fills in the status in memory. The error keeps its time while the message is the
// same, so a failing bucket does not change on every retry.
func applyStatus(b *v1alpha1.ObjectStoreBucket, out Outcome) {
	patch := out.Status
	status := metav1.ConditionFalse
	if patch.ReadyToUse {
		status = metav1.ConditionTrue
	}
	b.Status.ReadyToUse = &patch.ReadyToUse
	meta.SetStatusCondition(&b.Status.Conditions, metav1.Condition{
		Type:               v1alpha1.ConditionReady,
		Status:             status,
		Reason:             patch.Reason,
		Message:            patch.Message,
		ObservedGeneration: b.Generation,
	})

	switch {
	case patch.ReadyToUse:
		b.Status.BucketID = backendName(b)
		b.Status.Protocols = []v1alpha1.ObjectProtocol{v1alpha1.ProtocolS3}
		b.Status.Error = nil
	case out.Err != nil:
		if b.Status.Error == nil || b.Status.Error.Message == nil || *b.Status.Error.Message != patch.Message {
			now := metav1.Now()
			b.Status.Error = &v1alpha1.TimestampedError{Time: &now, Message: &patch.Message}
		}
	}
}
