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

package credentials_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmware/vsphere-cosi-driver/internal/iam"
	"github.com/vmware/vsphere-cosi-driver/internal/iam/fake"
	"github.com/vmware/vsphere-cosi-driver/internal/operator/credentials"
	"github.com/vmware/vsphere-cosi-driver/internal/operator/objectstore"
	"github.com/vmware/vsphere-cosi-driver/internal/platform/kube"
	vboss "github.com/vmware/vsphere-cosi-driver/internal/vboss/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

const (
	storeNamespace = "store-a"
	iamEndpoint    = "http://iam.example.test"
)

func newStore(healthState string) *vboss.ObjectStore {
	return &vboss.ObjectStore{
		ObjectMeta: metav1.ObjectMeta{Name: "store", Namespace: storeNamespace, UID: "store-uid"},
		Status: vboss.ObjectStoreStatus{
			HealthState: healthState,
			IAMEndpoint: vboss.IAMEndpoint{FQDN: iamEndpoint},
		},
	}
}

func newSecret(data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: credentials.SecretName, Namespace: storeNamespace},
		Data:       data,
	}
}

// newRootDir writes the root credentials as files, the way a mounted Secret appears.
func newRootDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, credentials.AccessKeyName, "root-access")
	writeFile(t, dir, credentials.SecretKeyName, "root-secret")
	return dir
}

// recordingIAM is the in-memory service with calls counted and chosen calls failing.
type recordingIAM struct {
	*fake.Client
	calls int
	// outage is the number of leading calls that fail as unavailable.
	outage                                                  int
	createUserErr, putPolicyErr, createKeyErr, deleteKeyErr error
	created                                                 iam.AccessKey
}

func (r *recordingIAM) enter(err error) error {
	r.calls++
	if r.outage > 0 {
		r.outage--
		return iam.ErrUnavailable
	}
	return err
}

func (r *recordingIAM) CreateUser(ctx context.Context, name string) error {
	if err := r.enter(r.createUserErr); err != nil {
		return err
	}
	return r.Client.CreateUser(ctx, name)
}

func (r *recordingIAM) PutUserPolicy(ctx context.Context, user, policyName, document string) error {
	if err := r.enter(r.putPolicyErr); err != nil {
		return err
	}
	return r.Client.PutUserPolicy(ctx, user, policyName, document)
}

func (r *recordingIAM) CreateAccessKey(ctx context.Context, user string) (iam.AccessKey, error) {
	if err := r.enter(r.createKeyErr); err != nil {
		return iam.AccessKey{}, err
	}
	key, err := r.Client.CreateAccessKey(ctx, user)
	r.created = key
	return key, err
}

func (r *recordingIAM) DeleteAccessKey(ctx context.Context, user, id string) error {
	if err := r.enter(r.deleteKeyErr); err != nil {
		return err
	}
	return r.Client.DeleteAccessKey(ctx, user, id)
}

func newIAM() *recordingIAM { return &recordingIAM{Client: fake.New()} }

// factory builds the IAM client and records what it was asked to connect with.
type factory struct {
	client                         iam.Client
	err                            error
	endpoint, accessKey, secretKey string
}

func (f *factory) new(endpoint, accessKey, secretKey string) (iam.Client, error) {
	f.endpoint, f.accessKey, f.secretKey = endpoint, accessKey, secretKey
	return f.client, f.err
}

func newBootstrap(t *testing.T, rootDir string, f *factory, funcs interceptor.Funcs, objs ...client.Object) (*credentials.Bootstrap, client.Client) {
	t.Helper()
	scheme, err := kube.NewScheme()
	require.NoError(t, err)
	c := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).WithInterceptorFuncs(funcs).Build()
	return &credentials.Bootstrap{
		Client:             c,
		Namespace:          storeNamespace,
		RootCredentialsDir: rootDir,
		NewIAMClient:       f.new,
		MinRetry:           time.Millisecond,
		MaxRetry:           time.Millisecond,
	}, c
}

func getSecret(t *testing.T, c client.Client) *corev1.Secret {
	t.Helper()
	s := &corev1.Secret{}
	require.NoError(t, c.Get(t.Context(), client.ObjectKey{Namespace: storeNamespace, Name: credentials.SecretName}, s))
	return s
}

func TestEnsure(t *testing.T) {
	t.Run("WhenFirstStart", func(tt *testing.T) {
		// Setup
		svc := newIAM()
		f := &factory{client: svc}
		b, c := newBootstrap(tt, newRootDir(tt), f, interceptor.Funcs{}, newStore("Ready"))

		// Execute
		err := b.Ensure(tt.Context())

		// Assert
		require.NoError(tt, err)
		assert.True(tt, svc.HasUser(credentials.UserName))
		document, ok := svc.Policy(credentials.UserName, credentials.PolicyName)
		assert.True(tt, ok)
		assert.True(tt, json.Valid([]byte(document)))
		require.NotEmpty(tt, svc.created.ID)
		assert.True(tt, svc.HasKey(credentials.UserName, svc.created.ID))
		secret := getSecret(tt, c)
		assert.Equal(tt, svc.created.ID, string(secret.Data[credentials.AccessKeyName]))
		assert.Equal(tt, svc.created.Secret, string(secret.Data[credentials.SecretKeyName]))
		require.Len(tt, secret.OwnerReferences, 1)
		assert.Equal(tt, "store", secret.OwnerReferences[0].Name)
		assert.Equal(tt, "store-uid", string(secret.OwnerReferences[0].UID))
	})

	t.Run("WhenConnectingToTheStore", func(tt *testing.T) {
		// Setup
		f := &factory{client: newIAM()}
		b, _ := newBootstrap(tt, newRootDir(tt), f, interceptor.Funcs{}, newStore("Ready"))

		// Execute
		err := b.Ensure(tt.Context())

		// Assert
		require.NoError(tt, err)
		assert.Equal(tt, iamEndpoint, f.endpoint)
		assert.Equal(tt, "root-access", f.accessKey)
		assert.Equal(tt, "root-secret", f.secretKey)
	})

	t.Run("WhenSecretIsComplete", func(tt *testing.T) {
		// Setup
		svc := newIAM()
		f := &factory{client: svc}
		secret := newSecret(map[string][]byte{credentials.AccessKeyName: []byte("a"), credentials.SecretKeyName: []byte("s")})
		b, c := newBootstrap(tt, tt.TempDir(), f, interceptor.Funcs{}, secret)

		// Execute
		err := b.Ensure(tt.Context())

		// Assert
		require.NoError(tt, err)
		assert.Zero(tt, svc.calls)
		assert.Empty(tt, f.endpoint)
		got := getSecret(tt, c)
		assert.Equal(tt, "a", string(got.Data[credentials.AccessKeyName]))
		assert.Equal(tt, "s", string(got.Data[credentials.SecretKeyName]))
	})

	t.Run("WhenAccessKeyIsEmptyInTheSecret", func(tt *testing.T) {
		// Setup
		svc := newIAM()
		secret := newSecret(map[string][]byte{credentials.AccessKeyName: nil, credentials.SecretKeyName: []byte("s")})
		b, c := newBootstrap(tt, newRootDir(tt), &factory{client: svc}, interceptor.Funcs{}, secret, newStore("Ready"))

		// Execute
		err := b.Ensure(tt.Context())

		// Assert
		require.NoError(tt, err)
		require.NotEmpty(tt, svc.created.ID)
		got := getSecret(tt, c)
		assert.Equal(tt, svc.created.ID, string(got.Data[credentials.AccessKeyName]))
		assert.Equal(tt, svc.created.Secret, string(got.Data[credentials.SecretKeyName]))
	})

	t.Run("WhenSecretKeyIsMissingFromTheSecret", func(tt *testing.T) {
		// Setup
		svc := newIAM()
		secret := newSecret(map[string][]byte{credentials.AccessKeyName: []byte("a")})
		b, c := newBootstrap(tt, newRootDir(tt), &factory{client: svc}, interceptor.Funcs{}, secret, newStore("Ready"))

		// Execute
		err := b.Ensure(tt.Context())

		// Assert
		require.NoError(tt, err)
		require.NotEmpty(tt, svc.created.ID)
		got := getSecret(tt, c)
		assert.Equal(tt, svc.created.ID, string(got.Data[credentials.AccessKeyName]))
		assert.Equal(tt, svc.created.Secret, string(got.Data[credentials.SecretKeyName]))
		assert.Len(tt, got.OwnerReferences, 1)
	})

	t.Run("WhenUserExists", func(tt *testing.T) {
		// Setup
		svc := newIAM()
		require.NoError(tt, svc.CreateUser(tt.Context(), credentials.UserName))
		svc.calls = 0
		b, c := newBootstrap(tt, newRootDir(tt), &factory{client: svc}, interceptor.Funcs{}, newStore("Ready"))

		// Execute
		err := b.Ensure(tt.Context())

		// Assert
		require.NoError(tt, err)
		require.NotEmpty(tt, svc.created.ID)
		assert.True(tt, svc.HasKey(credentials.UserName, svc.created.ID))
		assert.Equal(tt, svc.created.ID, string(getSecret(tt, c).Data[credentials.AccessKeyName]))
	})

	t.Run("WhenStoreIsNotReady", func(tt *testing.T) {
		// Setup
		svc := newIAM()
		b, c := newBootstrap(tt, newRootDir(tt), &factory{client: svc}, interceptor.Funcs{}, newStore("Degraded"))

		// Execute
		err := b.Ensure(tt.Context())

		// Assert
		assert.ErrorIs(tt, err, credentials.ErrStoreNotReady)
		assert.Zero(tt, svc.calls)
		assert.True(tt, apierrors.IsNotFound(c.Get(tt.Context(), client.ObjectKey{Namespace: storeNamespace, Name: credentials.SecretName}, &corev1.Secret{})))
	})

	t.Run("WhenThereIsNoStore", func(tt *testing.T) {
		// Setup
		svc := newIAM()
		b, _ := newBootstrap(tt, newRootDir(tt), &factory{client: svc}, interceptor.Funcs{})

		// Execute
		err := b.Ensure(tt.Context())

		// Assert
		assert.ErrorIs(tt, err, credentials.ErrStoreNotReady)
		assert.Zero(tt, svc.calls)
	})

	t.Run("WhenThereAreTwoStores", func(tt *testing.T) {
		// Setup
		second := newStore("Ready")
		second.Name = "other"
		svc := newIAM()
		b, _ := newBootstrap(tt, newRootDir(tt), &factory{client: svc}, interceptor.Funcs{}, newStore("Ready"), second)

		// Execute
		err := b.Ensure(tt.Context())

		// Assert
		assert.ErrorIs(tt, err, objectstore.ErrMultiple)
		assert.Zero(tt, svc.calls)
	})

	t.Run("WhenRootCredentialsAreMissing", func(tt *testing.T) {
		// Setup
		svc := newIAM()
		b, _ := newBootstrap(tt, tt.TempDir(), &factory{client: svc}, interceptor.Funcs{}, newStore("Ready"))

		// Execute
		err := b.Ensure(tt.Context())

		// Assert
		assert.ErrorIs(tt, err, credentials.ErrRootUnavailable)
		assert.Zero(tt, svc.calls)
	})

	t.Run("WhenRootCredentialsCannotBeRead", func(tt *testing.T) {
		// Setup
		dir := tt.TempDir()
		require.NoError(tt, os.Mkdir(filepath.Join(dir, credentials.AccessKeyName), 0o700))
		svc := newIAM()
		b, _ := newBootstrap(tt, dir, &factory{client: svc}, interceptor.Funcs{}, newStore("Ready"))

		// Execute
		err := b.Ensure(tt.Context())

		// Assert
		require.Error(tt, err)
		assert.NotErrorIs(tt, err, credentials.ErrRootUnavailable)
		assert.Zero(tt, svc.calls)
	})

	t.Run("WhenConnectingFails", func(tt *testing.T) {
		// Setup
		errConnect := errors.New("bad endpoint")
		b, _ := newBootstrap(tt, newRootDir(tt), &factory{err: errConnect}, interceptor.Funcs{}, newStore("Ready"))

		// Execute
		err := b.Ensure(tt.Context())

		// Assert
		assert.ErrorIs(tt, err, errConnect)
	})

	t.Run("WhenReadingTheSecretFails", func(tt *testing.T) {
		// Setup
		errGet := errors.New("api unavailable")
		funcs := interceptor.Funcs{Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
			return errGet
		}}
		svc := newIAM()
		b, _ := newBootstrap(tt, newRootDir(tt), &factory{client: svc}, funcs, newStore("Ready"))

		// Execute
		err := b.Ensure(tt.Context())

		// Assert
		assert.ErrorIs(tt, err, errGet)
		assert.Zero(tt, svc.calls)
	})

	t.Run("WhenListingStoresFails", func(tt *testing.T) {
		// Setup
		errList := errors.New("api unavailable")
		funcs := interceptor.Funcs{List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
			return errList
		}}
		svc := newIAM()
		b, _ := newBootstrap(tt, newRootDir(tt), &factory{client: svc}, funcs, newStore("Ready"))

		// Execute
		err := b.Ensure(tt.Context())

		// Assert
		assert.ErrorIs(tt, err, errList)
		assert.Zero(tt, svc.calls)
	})

	t.Run("WhenCreatingTheUserFails", func(tt *testing.T) {
		// Setup
		svc := newIAM()
		svc.createUserErr = iam.ErrAccessDenied
		b, c := newBootstrap(tt, newRootDir(tt), &factory{client: svc}, interceptor.Funcs{}, newStore("Ready"))

		// Execute
		err := b.Ensure(tt.Context())

		// Assert
		assert.ErrorIs(tt, err, iam.ErrAccessDenied)
		assert.False(tt, svc.HasUser(credentials.UserName))
		assert.True(tt, apierrors.IsNotFound(c.Get(tt.Context(), client.ObjectKey{Namespace: storeNamespace, Name: credentials.SecretName}, &corev1.Secret{})))
	})

	t.Run("WhenAttachingThePolicyFails", func(tt *testing.T) {
		// Setup
		svc := newIAM()
		svc.putPolicyErr = iam.ErrUnavailable
		b, c := newBootstrap(tt, newRootDir(tt), &factory{client: svc}, interceptor.Funcs{}, newStore("Ready"))

		// Execute
		err := b.Ensure(tt.Context())

		// Assert
		assert.ErrorIs(tt, err, iam.ErrUnavailable)
		assert.Empty(tt, svc.created.ID)
		assert.True(tt, apierrors.IsNotFound(c.Get(tt.Context(), client.ObjectKey{Namespace: storeNamespace, Name: credentials.SecretName}, &corev1.Secret{})))
	})

	t.Run("WhenCreatingTheKeyFails", func(tt *testing.T) {
		// Setup
		svc := newIAM()
		svc.createKeyErr = iam.ErrAccessDenied
		b, c := newBootstrap(tt, newRootDir(tt), &factory{client: svc}, interceptor.Funcs{}, newStore("Ready"))

		// Execute
		err := b.Ensure(tt.Context())

		// Assert
		assert.ErrorIs(tt, err, iam.ErrAccessDenied)
		assert.True(tt, apierrors.IsNotFound(c.Get(tt.Context(), client.ObjectKey{Namespace: storeNamespace, Name: credentials.SecretName}, &corev1.Secret{})))
	})

	t.Run("WhenCreatingTheSecretFails", func(tt *testing.T) {
		// Setup
		errCreate := errors.New("api unavailable")
		funcs := interceptor.Funcs{Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
			return errCreate
		}}
		svc := newIAM()
		b, _ := newBootstrap(tt, newRootDir(tt), &factory{client: svc}, funcs, newStore("Ready"))

		// Execute
		err := b.Ensure(tt.Context())

		// Assert
		assert.ErrorIs(tt, err, errCreate)
		require.NotEmpty(tt, svc.created.ID)
		assert.False(tt, svc.HasKey(credentials.UserName, svc.created.ID))
	})

	t.Run("WhenUpdatingTheSecretFails", func(tt *testing.T) {
		// Setup
		errUpdate := errors.New("api unavailable")
		funcs := interceptor.Funcs{Update: func(context.Context, client.WithWatch, client.Object, ...client.UpdateOption) error {
			return errUpdate
		}}
		svc := newIAM()
		secret := newSecret(map[string][]byte{credentials.AccessKeyName: []byte("a")})
		b, _ := newBootstrap(tt, newRootDir(tt), &factory{client: svc}, funcs, secret, newStore("Ready"))

		// Execute
		err := b.Ensure(tt.Context())

		// Assert
		assert.ErrorIs(tt, err, errUpdate)
		require.NotEmpty(tt, svc.created.ID)
		assert.False(tt, svc.HasKey(credentials.UserName, svc.created.ID))
	})

	t.Run("WhenDeletingTheUnusedKeyAlsoFails", func(tt *testing.T) {
		// Setup
		errCreate := errors.New("api unavailable")
		funcs := interceptor.Funcs{Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
			return errCreate
		}}
		svc := newIAM()
		svc.deleteKeyErr = iam.ErrUnavailable
		b, _ := newBootstrap(tt, newRootDir(tt), &factory{client: svc}, funcs, newStore("Ready"))

		// Execute
		err := b.Ensure(tt.Context())

		// Assert
		assert.ErrorIs(tt, err, errCreate)
		assert.ErrorIs(tt, err, iam.ErrUnavailable)
	})
}

func TestStart(t *testing.T) {
	t.Run("WhenSecretIsCreatedAtOnce", func(tt *testing.T) {
		// Setup
		svc := newIAM()
		b, c := newBootstrap(tt, newRootDir(tt), &factory{client: svc}, interceptor.Funcs{}, newStore("Ready"))

		// Execute
		err := b.Start(tt.Context())

		// Assert
		require.NoError(tt, err)
		assert.Equal(tt, svc.created.ID, string(getSecret(tt, c).Data[credentials.AccessKeyName]))
	})

	t.Run("WhenTheStoreIsUnavailableAtFirst", func(tt *testing.T) {
		// Setup
		svc := newIAM()
		svc.outage = 2
		b, c := newBootstrap(tt, newRootDir(tt), &factory{client: svc}, interceptor.Funcs{}, newStore("Ready"))

		// Execute
		err := b.Start(tt.Context())

		// Assert
		require.NoError(tt, err)
		require.NotEmpty(tt, svc.created.ID)
		assert.Equal(tt, svc.created.ID, string(getSecret(tt, c).Data[credentials.AccessKeyName]))
	})

	t.Run("WhenContextIsCancelledWhileWaiting", func(tt *testing.T) {
		// Setup
		svc := newIAM()
		b, c := newBootstrap(tt, newRootDir(tt), &factory{client: svc}, interceptor.Funcs{}, newStore("Degraded"))
		ctx, cancel := context.WithCancel(tt.Context())
		cancel()

		// Execute
		err := b.Start(ctx)

		// Assert
		require.NoError(tt, err)
		assert.Zero(tt, svc.calls)
		assert.True(tt, apierrors.IsNotFound(c.Get(tt.Context(), client.ObjectKey{Namespace: storeNamespace, Name: credentials.SecretName}, &corev1.Secret{})))
	})
}
