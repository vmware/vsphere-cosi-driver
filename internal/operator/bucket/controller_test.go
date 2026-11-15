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

package bucket_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmware/vsphere-cosi-driver/api/v1alpha1"
	"github.com/vmware/vsphere-cosi-driver/internal/operator/bucket"
	"github.com/vmware/vsphere-cosi-driver/internal/platform/errs"
	"github.com/vmware/vsphere-cosi-driver/internal/platform/kube"
	"github.com/vmware/vsphere-cosi-driver/internal/s3"
	"github.com/vmware/vsphere-cosi-driver/internal/s3/fake"
	vboss "github.com/vmware/vsphere-cosi-driver/internal/vboss/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

const (
	storeNamespace = "store-a"
	storeEndpoint  = "http://s3.example.test"
)

func newBucket(name string) *v1alpha1.ObjectStoreBucket {
	return &v1alpha1.ObjectStoreBucket{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: storeNamespace, Generation: 2},
		Spec: v1alpha1.BucketSpec{
			DeletionPolicy: v1alpha1.DeletionPolicyDelete,
			Protocols:      []v1alpha1.ObjectProtocol{v1alpha1.ProtocolS3},
		},
	}
}

// newDeletingBucket is a bucket that carries the finalizer and has been deleted.
func newDeletingBucket(name string) *v1alpha1.ObjectStoreBucket {
	b := newBucket(name)
	b.Finalizers = []string{v1alpha1.Finalizer}
	now := metav1.Now()
	b.DeletionTimestamp = &now
	return b
}

func newStore(healthState string) *vboss.ObjectStore {
	return &vboss.ObjectStore{
		ObjectMeta: metav1.ObjectMeta{Name: "store", Namespace: storeNamespace},
		Status: vboss.ObjectStoreStatus{
			HealthState:        healthState,
			ExternalS3Endpoint: vboss.ExternalS3Endpoint{FQDN: storeEndpoint},
		},
	}
}

// newCredentials writes the admin credential as files, the way a mounted Secret appears.
func newCredentials(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "akey"), []byte("access"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "secret"), []byte("secret"), 0o600))
	return dir
}

// failingS3 is the in-memory store with chosen calls failing.
type failingS3 struct {
	s3.Client
	headErr, createErr, deleteErr error
}

func (f *failingS3) BucketExists(ctx context.Context, name string) (bool, error) {
	if f.headErr != nil {
		return false, f.headErr
	}
	return f.Client.BucketExists(ctx, name)
}

func (f *failingS3) CreateBucket(ctx context.Context, name string) error {
	if f.createErr != nil {
		return f.createErr
	}
	return f.Client.CreateBucket(ctx, name)
}

func (f *failingS3) DeleteBucket(ctx context.Context, name string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	return f.Client.DeleteBucket(ctx, name)
}

// factory builds the S3 client and records what it was asked to connect with.
type factory struct {
	client                         s3.Client
	err                            error
	endpoint, accessKey, secretKey string
}

func (f *factory) new(endpoint, accessKey, secretKey string) (s3.Client, error) {
	f.endpoint, f.accessKey, f.secretKey = endpoint, accessKey, secretKey
	return f.client, f.err
}

func newReconciler(t *testing.T, credentials string, f *factory, funcs interceptor.Funcs, objs ...client.Object) (*bucket.Reconciler, client.Client) {
	t.Helper()
	scheme, err := kube.NewScheme()
	require.NoError(t, err)
	c := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&v1alpha1.ObjectStoreBucket{}).
		WithInterceptorFuncs(funcs).
		Build()
	return &bucket.Reconciler{Client: c, NewS3Client: f.new, CredentialsDir: credentials}, c
}

func reconcileBucket(r *bucket.Reconciler, name string) (ctrl.Result, error) {
	return r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: storeNamespace, Name: name}})
}

func getBucket(t *testing.T, c client.Client, name string) *v1alpha1.ObjectStoreBucket {
	t.Helper()
	b := &v1alpha1.ObjectStoreBucket{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: storeNamespace, Name: name}, b))
	return b
}

func bucketExists(t *testing.T, s s3.Client, name string) bool {
	t.Helper()
	exists, err := s.BucketExists(context.Background(), name)
	require.NoError(t, err)
	return exists
}

func assertStatus(t *testing.T, b *v1alpha1.ObjectStoreBucket, isReady bool, reason string) {
	t.Helper()
	if assert.NotNil(t, b.Status.ReadyToUse) {
		assert.Equal(t, isReady, *b.Status.ReadyToUse)
	}
	cond := meta.FindStatusCondition(b.Status.Conditions, v1alpha1.ConditionReady)
	if assert.NotNil(t, cond) {
		assert.Equal(t, reason, cond.Reason)
		assert.Equal(t, isReady, cond.Status == metav1.ConditionTrue)
		assert.Equal(t, b.Generation, cond.ObservedGeneration)
	}
}

func TestReconcile(t *testing.T) {
	t.Run("WhenBucketIsNew", func(tt *testing.T) {
		// Setup
		store := fake.New()
		f := &factory{client: store}
		r, c := newReconciler(tt, newCredentials(tt), f, interceptor.Funcs{}, newBucket("bucket-a"), newStore("Ready"))

		// Execute
		_, err := reconcileBucket(r, "bucket-a")

		// Assert
		require.NoError(tt, err)
		got := getBucket(tt, c, "bucket-a")
		assert.Contains(tt, got.Finalizers, v1alpha1.Finalizer)
		assertStatus(tt, got, true, v1alpha1.ReasonProvisioned)
		assert.Equal(tt, "bucket-a", got.Status.BucketID)
		assert.Equal(tt, []v1alpha1.ObjectProtocol{v1alpha1.ProtocolS3}, got.Status.Protocols)
		assert.True(tt, bucketExists(tt, store, "bucket-a"))
		assert.Equal(tt, storeEndpoint, f.endpoint)
		assert.Equal(tt, "access", f.accessKey)
		assert.Equal(tt, "secret", f.secretKey)
	})

	t.Run("WhenBucketAlreadyExistsInTheStore", func(tt *testing.T) {
		// Setup
		store := fake.New()
		require.NoError(tt, store.CreateBucket(context.Background(), "bucket-a"))
		r, c := newReconciler(tt, newCredentials(tt), &factory{client: store}, interceptor.Funcs{}, newBucket("bucket-a"), newStore("Ready"))

		// Execute
		_, err := reconcileBucket(r, "bucket-a")

		// Assert
		require.NoError(tt, err)
		assertStatus(tt, getBucket(tt, c, "bucket-a"), true, v1alpha1.ReasonProvisioned)
	})

	t.Run("WhenBucketWasReadyAndTheStoreFails", func(tt *testing.T) {
		// Setup
		store := &failingS3{Client: fake.New()}
		r, c := newReconciler(tt, newCredentials(tt), &factory{client: store}, interceptor.Funcs{}, newBucket("bucket-a"), newStore("Ready"))
		_, err := reconcileBucket(r, "bucket-a")
		require.NoError(tt, err)
		store.headErr = s3.ErrUnavailable

		// Execute
		_, err = reconcileBucket(r, "bucket-a")

		// Assert
		assert.Error(tt, err)
		assertStatus(tt, getBucket(tt, c, "bucket-a"), false, v1alpha1.ReasonStoreUnavailable)
	})

	t.Run("WhenStoreIsMissing", func(tt *testing.T) {
		// Setup
		r, c := newReconciler(tt, newCredentials(tt), &factory{client: fake.New()}, interceptor.Funcs{}, newBucket("bucket-a"))

		// Execute
		_, err := reconcileBucket(r, "bucket-a")

		// Assert
		assert.Error(tt, err)
		assert.False(tt, errs.IsTerminal(err))
		got := getBucket(tt, c, "bucket-a")
		assertStatus(tt, got, false, v1alpha1.ReasonStoreNotFound)
		assert.Empty(tt, got.Finalizers)
	})

	t.Run("WhenStoreIsNotReady", func(tt *testing.T) {
		// Setup
		r, c := newReconciler(tt, newCredentials(tt), &factory{client: fake.New()}, interceptor.Funcs{}, newBucket("bucket-a"), newStore("Degraded"))

		// Execute
		_, err := reconcileBucket(r, "bucket-a")

		// Assert
		assert.Error(tt, err)
		assertStatus(tt, getBucket(tt, c, "bucket-a"), false, v1alpha1.ReasonStoreNotReady)
	})

	t.Run("WhenCredentialsAreMissing", func(tt *testing.T) {
		// Setup
		r, c := newReconciler(tt, tt.TempDir(), &factory{client: fake.New()}, interceptor.Funcs{}, newBucket("bucket-a"), newStore("Ready"))

		// Execute
		_, err := reconcileBucket(r, "bucket-a")

		// Assert
		assert.Error(tt, err)
		assertStatus(tt, getBucket(tt, c, "bucket-a"), false, v1alpha1.ReasonCredentialsUnavailable)
	})

	t.Run("WhenCredentialsLackAKey", func(tt *testing.T) {
		// Setup
		dir := newCredentials(tt)
		require.NoError(tt, os.Remove(filepath.Join(dir, "secret")))
		r, c := newReconciler(tt, dir, &factory{client: fake.New()}, interceptor.Funcs{}, newBucket("bucket-a"), newStore("Ready"))

		// Execute
		_, err := reconcileBucket(r, "bucket-a")

		// Assert
		assert.Error(tt, err)
		assertStatus(tt, getBucket(tt, c, "bucket-a"), false, v1alpha1.ReasonCredentialsUnavailable)
	})

	t.Run("WhenReadingCredentialsFails", func(tt *testing.T) {
		// Setup
		dir := tt.TempDir()
		require.NoError(tt, os.Mkdir(filepath.Join(dir, "akey"), 0o700))
		r, _ := newReconciler(tt, dir, &factory{client: fake.New()}, interceptor.Funcs{}, newBucket("bucket-a"), newStore("Ready"))

		// Execute
		_, err := reconcileBucket(r, "bucket-a")

		// Assert
		assert.ErrorContains(tt, err, "admin credentials")
	})

	t.Run("WhenReadingTheSecretKeyFails", func(tt *testing.T) {
		// Setup
		dir := newCredentials(tt)
		require.NoError(tt, os.Remove(filepath.Join(dir, "secret")))
		require.NoError(tt, os.Mkdir(filepath.Join(dir, "secret"), 0o700))
		r, _ := newReconciler(tt, dir, &factory{client: fake.New()}, interceptor.Funcs{}, newBucket("bucket-a"), newStore("Ready"))

		// Execute
		_, err := reconcileBucket(r, "bucket-a")

		// Assert
		assert.ErrorContains(tt, err, "admin credentials")
	})

	t.Run("WhenCredentialsAreRotated", func(tt *testing.T) {
		// Setup
		dir := newCredentials(tt)
		f := &factory{client: &failingS3{Client: fake.New(), headErr: s3.ErrAccessDenied}}
		r, _ := newReconciler(tt, dir, f, interceptor.Funcs{}, newBucket("bucket-a"), newStore("Ready"))
		_, err := reconcileBucket(r, "bucket-a")
		require.Error(tt, err)
		require.NoError(tt, os.WriteFile(filepath.Join(dir, "akey"), []byte("rotated"), 0o600))

		// Execute
		_, err = reconcileBucket(r, "bucket-a")

		// Assert
		assert.Error(tt, err)
		assert.Equal(tt, "rotated", f.accessKey)
	})

	t.Run("WhenTwoStoresExist", func(tt *testing.T) {
		// Setup
		second := newStore("Ready")
		second.Name = "store-b"
		r, _ := newReconciler(tt, newCredentials(tt), &factory{client: fake.New()}, interceptor.Funcs{}, newBucket("bucket-a"), newStore("Ready"), second)

		// Execute
		_, err := reconcileBucket(r, "bucket-a")

		// Assert
		assert.ErrorIs(tt, err, bucket.ErrMultipleStores)
	})

	t.Run("WhenListingStoresFails", func(tt *testing.T) {
		// Setup
		funcs := interceptor.Funcs{List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
			return errors.New("api unavailable")
		}}
		r, _ := newReconciler(tt, newCredentials(tt), &factory{client: fake.New()}, funcs, newBucket("bucket-a"), newStore("Ready"))

		// Execute
		_, err := reconcileBucket(r, "bucket-a")

		// Assert
		assert.ErrorContains(tt, err, "api unavailable")
	})

	t.Run("WhenS3ClientCannotBeBuilt", func(tt *testing.T) {
		// Setup
		r, c := newReconciler(tt, newCredentials(tt), &factory{err: errors.New("bad endpoint")}, interceptor.Funcs{}, newBucket("bucket-a"), newStore("Ready"))

		// Execute
		_, err := reconcileBucket(r, "bucket-a")

		// Assert
		assert.Error(tt, err)
		assertStatus(tt, getBucket(tt, c, "bucket-a"), false, v1alpha1.ReasonFailed)
	})

	t.Run("WhenAccessIsDenied", func(tt *testing.T) {
		// Setup
		store := &failingS3{Client: fake.New(), headErr: s3.ErrAccessDenied}
		r, c := newReconciler(tt, newCredentials(tt), &factory{client: store}, interceptor.Funcs{}, newBucket("bucket-a"), newStore("Ready"))

		// Execute
		_, err := reconcileBucket(r, "bucket-a")

		// Assert
		assert.Error(tt, err)
		assert.False(tt, errs.IsTerminal(err))
		got := getBucket(tt, c, "bucket-a")
		assertStatus(tt, got, false, v1alpha1.ReasonAccessDenied)
		if assert.NotNil(tt, got.Status.Error) {
			assert.Equal(tt, "the object store rejected the credentials", *got.Status.Error.Message)
			assert.NotNil(tt, got.Status.Error.Time)
		}
	})

	t.Run("WhenTheStoreIsUnavailable", func(tt *testing.T) {
		// Setup
		store := &failingS3{Client: fake.New(), headErr: s3.ErrUnavailable}
		r, c := newReconciler(tt, newCredentials(tt), &factory{client: store}, interceptor.Funcs{}, newBucket("bucket-a"), newStore("Ready"))

		// Execute
		_, err := reconcileBucket(r, "bucket-a")

		// Assert
		assert.Error(tt, err)
		assertStatus(tt, getBucket(tt, c, "bucket-a"), false, v1alpha1.ReasonStoreUnavailable)
	})

	t.Run("WhenTheNameBelongsToAnotherOwner", func(tt *testing.T) {
		// Setup
		store := &failingS3{Client: fake.New(), createErr: s3.ErrAlreadyExists}
		r, c := newReconciler(tt, newCredentials(tt), &factory{client: store}, interceptor.Funcs{}, newBucket("bucket-a"), newStore("Ready"))

		// Execute
		_, err := reconcileBucket(r, "bucket-a")

		// Assert
		assert.NoError(tt, err)
		assertStatus(tt, getBucket(tt, c, "bucket-a"), false, v1alpha1.ReasonBucketNameTaken)
	})

	t.Run("WhenBucketIsStatic", func(tt *testing.T) {
		// Setup
		b := newBucket("bucket-a")
		b.Spec.ExistingBucketID = "other"
		r, c := newReconciler(tt, newCredentials(tt), &factory{client: fake.New()}, interceptor.Funcs{}, b, newStore("Ready"))

		// Execute
		_, err := reconcileBucket(r, "bucket-a")

		// Assert
		assert.NoError(tt, err)
		assertStatus(tt, getBucket(tt, c, "bucket-a"), false, v1alpha1.ReasonUnsupported)
	})

	t.Run("WhenBucketIsGone", func(tt *testing.T) {
		// Setup
		r, _ := newReconciler(tt, tt.TempDir(), &factory{client: fake.New()}, interceptor.Funcs{})

		// Execute
		_, err := reconcileBucket(r, "bucket-a")

		// Assert
		assert.NoError(tt, err)
	})

	t.Run("WhenReadingTheBucketFails", func(tt *testing.T) {
		// Setup
		funcs := interceptor.Funcs{Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
			return errors.New("api unavailable")
		}}
		r, _ := newReconciler(tt, tt.TempDir(), &factory{client: fake.New()}, funcs, newBucket("bucket-a"))

		// Execute
		_, err := reconcileBucket(r, "bucket-a")

		// Assert
		assert.ErrorContains(tt, err, "api unavailable")
	})

	t.Run("WhenTheSameFailureRepeats", func(tt *testing.T) {
		// Setup
		writes := 0
		funcs := interceptor.Funcs{SubResourcePatch: func(ctx context.Context, c client.Client, sub string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
			writes++
			return c.SubResource(sub).Patch(ctx, obj, patch, opts...)
		}}
		r, c := newReconciler(tt, newCredentials(tt), &factory{client: fake.New()}, funcs, newBucket("bucket-a"), newStore("Degraded"))
		_, err := reconcileBucket(r, "bucket-a")
		require.Error(tt, err)
		first := getBucket(tt, c, "bucket-a")

		// Execute
		_, err = reconcileBucket(r, "bucket-a")

		// Assert
		assert.Error(tt, err)
		assert.Equal(tt, 1, writes)
		assert.Equal(tt, first.ResourceVersion, getBucket(tt, c, "bucket-a").ResourceVersion)
	})

	t.Run("WhenWritingStatusFails", func(tt *testing.T) {
		// Setup
		funcs := interceptor.Funcs{SubResourcePatch: func(context.Context, client.Client, string, client.Object, client.Patch, ...client.SubResourcePatchOption) error {
			return errors.New("api unavailable")
		}}
		r, _ := newReconciler(tt, newCredentials(tt), &factory{client: fake.New()}, funcs, newBucket("bucket-a"), newStore("Ready"))

		// Execute
		_, err := reconcileBucket(r, "bucket-a")

		// Assert
		assert.ErrorContains(tt, err, "api unavailable")
	})

	t.Run("WhenAddingTheFinalizerFails", func(tt *testing.T) {
		// Setup
		store := fake.New()
		funcs := interceptor.Funcs{Patch: func(context.Context, client.WithWatch, client.Object, client.Patch, ...client.PatchOption) error {
			return errors.New("api unavailable")
		}}
		r, _ := newReconciler(tt, newCredentials(tt), &factory{client: store}, funcs, newBucket("bucket-a"), newStore("Ready"))

		// Execute
		_, err := reconcileBucket(r, "bucket-a")

		// Assert
		assert.ErrorContains(tt, err, "api unavailable")
		assert.False(tt, bucketExists(tt, store, "bucket-a"))
	})

	t.Run("WhenDeletingABucketThatExists", func(tt *testing.T) {
		// Setup
		store := fake.New()
		require.NoError(tt, store.CreateBucket(context.Background(), "bucket-a"))
		r, c := newReconciler(tt, newCredentials(tt), &factory{client: store}, interceptor.Funcs{}, newDeletingBucket("bucket-a"), newStore("Ready"))

		// Execute
		_, err := reconcileBucket(r, "bucket-a")

		// Assert
		require.NoError(tt, err)
		assert.False(tt, bucketExists(tt, store, "bucket-a"))
		err = c.Get(context.Background(), types.NamespacedName{Namespace: storeNamespace, Name: "bucket-a"}, &v1alpha1.ObjectStoreBucket{})
		assert.True(tt, apierrors.IsNotFound(err))
	})

	t.Run("WhenDeletingABucketTheStoreNoLongerHas", func(tt *testing.T) {
		// Setup
		r, c := newReconciler(tt, newCredentials(tt), &factory{client: fake.New()}, interceptor.Funcs{}, newDeletingBucket("bucket-a"), newStore("Ready"))

		// Execute
		_, err := reconcileBucket(r, "bucket-a")

		// Assert
		require.NoError(tt, err)
		err = c.Get(context.Background(), types.NamespacedName{Namespace: storeNamespace, Name: "bucket-a"}, &v1alpha1.ObjectStoreBucket{})
		assert.True(tt, apierrors.IsNotFound(err))
	})

	t.Run("WhenDeletingANonEmptyBucket", func(tt *testing.T) {
		// Setup
		store := fake.New()
		require.NoError(tt, store.CreateBucket(context.Background(), "bucket-a"))
		store.AddObject("bucket-a")
		r, c := newReconciler(tt, newCredentials(tt), &factory{client: store}, interceptor.Funcs{}, newDeletingBucket("bucket-a"), newStore("Ready"))

		// Execute
		_, err := reconcileBucket(r, "bucket-a")

		// Assert
		assert.Error(tt, err)
		assert.False(tt, errs.IsTerminal(err))
		got := getBucket(tt, c, "bucket-a")
		assert.Contains(tt, got.Finalizers, v1alpha1.Finalizer)
		assertStatus(tt, got, false, v1alpha1.ReasonBucketNotEmpty)
		assert.True(tt, bucketExists(tt, store, "bucket-a"))
	})

	t.Run("WhenDeletingAndTheStoreFails", func(tt *testing.T) {
		// Setup
		store := &failingS3{Client: fake.New(), deleteErr: s3.ErrUnavailable}
		r, c := newReconciler(tt, newCredentials(tt), &factory{client: store}, interceptor.Funcs{}, newDeletingBucket("bucket-a"), newStore("Ready"))

		// Execute
		_, err := reconcileBucket(r, "bucket-a")

		// Assert
		assert.Error(tt, err)
		got := getBucket(tt, c, "bucket-a")
		assert.Contains(tt, got.Finalizers, v1alpha1.Finalizer)
		assertStatus(tt, got, false, v1alpha1.ReasonStoreUnavailable)
	})

	t.Run("WhenDeletingAndCredentialsAreMissing", func(tt *testing.T) {
		// Setup
		r, c := newReconciler(tt, tt.TempDir(), &factory{client: fake.New()}, interceptor.Funcs{}, newDeletingBucket("bucket-a"), newStore("Ready"))

		// Execute
		_, err := reconcileBucket(r, "bucket-a")

		// Assert
		assert.Error(tt, err)
		got := getBucket(tt, c, "bucket-a")
		assert.Contains(tt, got.Finalizers, v1alpha1.Finalizer)
		assertStatus(tt, got, false, v1alpha1.ReasonCredentialsUnavailable)
	})

	t.Run("WhenDeletingARetainedBucket", func(tt *testing.T) {
		// Setup
		store := fake.New()
		require.NoError(tt, store.CreateBucket(context.Background(), "bucket-a"))
		b := newDeletingBucket("bucket-a")
		b.Spec.DeletionPolicy = v1alpha1.DeletionPolicyRetain
		r, c := newReconciler(tt, newCredentials(tt), &factory{client: store}, interceptor.Funcs{}, b, newStore("Ready"))

		// Execute
		_, err := reconcileBucket(r, "bucket-a")

		// Assert
		require.NoError(tt, err)
		assert.True(tt, bucketExists(tt, store, "bucket-a"))
		err = c.Get(context.Background(), types.NamespacedName{Namespace: storeNamespace, Name: "bucket-a"}, &v1alpha1.ObjectStoreBucket{})
		assert.True(tt, apierrors.IsNotFound(err))
	})

	t.Run("WhenDeletingAndTheStoreIsGone", func(tt *testing.T) {
		// Setup
		r, c := newReconciler(tt, tt.TempDir(), &factory{client: fake.New()}, interceptor.Funcs{}, newDeletingBucket("bucket-a"))

		// Execute
		_, err := reconcileBucket(r, "bucket-a")

		// Assert
		require.NoError(tt, err)
		err = c.Get(context.Background(), types.NamespacedName{Namespace: storeNamespace, Name: "bucket-a"}, &v1alpha1.ObjectStoreBucket{})
		assert.True(tt, apierrors.IsNotFound(err))
	})

	t.Run("WhenReleasingTheFinalizerFails", func(tt *testing.T) {
		// Setup
		funcs := interceptor.Funcs{Patch: func(context.Context, client.WithWatch, client.Object, client.Patch, ...client.PatchOption) error {
			return errors.New("api unavailable")
		}}
		r, _ := newReconciler(tt, newCredentials(tt), &factory{client: fake.New()}, funcs, newDeletingBucket("bucket-a"), newStore("Ready"))

		// Execute
		_, err := reconcileBucket(r, "bucket-a")

		// Assert
		assert.ErrorContains(tt, err, "api unavailable")
	})
}

func TestMapStoreToBuckets(t *testing.T) {
	t.Run("WhenBucketsExistInTheStoresNamespace", func(tt *testing.T) {
		// Setup
		other := newBucket("elsewhere")
		other.Namespace = "store-b"
		r, _ := newReconciler(tt, tt.TempDir(), &factory{client: fake.New()}, interceptor.Funcs{}, newBucket("bucket-a"), newBucket("bucket-b"), other)

		// Execute
		got := r.MapStoreToBuckets(context.Background(), newStore("Ready"))

		// Assert
		assert.ElementsMatch(tt, []ctrl.Request{
			{NamespacedName: types.NamespacedName{Namespace: storeNamespace, Name: "bucket-a"}},
			{NamespacedName: types.NamespacedName{Namespace: storeNamespace, Name: "bucket-b"}},
		}, got)
	})

	t.Run("WhenNoBucketsExist", func(tt *testing.T) {
		// Setup
		r, _ := newReconciler(tt, tt.TempDir(), &factory{client: fake.New()}, interceptor.Funcs{})

		// Execute
		got := r.MapStoreToBuckets(context.Background(), newStore("Ready"))

		// Assert
		assert.Empty(tt, got)
	})

	t.Run("WhenListingBucketsFails", func(tt *testing.T) {
		// Setup
		funcs := interceptor.Funcs{List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
			return errors.New("api unavailable")
		}}
		r, _ := newReconciler(tt, tt.TempDir(), &factory{client: fake.New()}, funcs, newBucket("bucket-a"))

		// Execute
		got := r.MapStoreToBuckets(context.Background(), newStore("Ready"))

		// Assert
		assert.Empty(tt, got)
	})
}

func TestSetupWithManager(t *testing.T) {
	t.Run("WhenTheManagerIsValid", func(tt *testing.T) {
		// Setup
		scheme, err := kube.NewScheme()
		require.NoError(tt, err)
		mgr, err := ctrl.NewManager(&rest.Config{Host: "http://127.0.0.1:1"}, ctrl.Options{
			Scheme:  scheme,
			Metrics: metricsserver.Options{BindAddress: "0"},
		})
		require.NoError(tt, err)
		r := &bucket.Reconciler{Client: mgr.GetClient(), NewS3Client: (&factory{client: fake.New()}).new}

		// Execute
		err = r.SetupWithManager(mgr)

		// Assert
		assert.NoError(tt, err)
	})
}
