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

package credentials

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/vmware/vsphere-cosi-driver/internal/iam"
	"github.com/vmware/vsphere-cosi-driver/internal/operator/objectstore"
	vboss "github.com/vmware/vsphere-cosi-driver/internal/vboss/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// SecretName is the Secret that holds the operator's own key.
	SecretName = "cosi-operator-credentials"
	// UserName is the store user the key belongs to.
	UserName = "cosi-operator"
	// PolicyName is the user's inline policy.
	PolicyName = "cosi-operator"
)

// policyDocument lets the user manage buckets and, for the access resources that come later,
// users and keys. It is applied only when a key is created.
const policyDocument = `{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": [
      "s3:*",
      "iam:CreateUser",
      "iam:DeleteUser",
      "iam:ListUsers",
      "iam:CreateAccessKey",
      "iam:DeleteAccessKey",
      "iam:PutUserPolicy",
      "iam:DeleteUserPolicy"
    ],
    "Resource": "*"
  }]
}`

var (
	// ErrStoreNotReady is returned while the namespace has no ObjectStore that is ready.
	ErrStoreNotReady = errors.New("the object store is not ready")
	// ErrRootUnavailable is returned while the root credentials are not mounted.
	ErrRootUnavailable = errors.New("the root credentials are not available")
)

// Bootstrap gives the operator its own key. With the store's root credentials it creates a
// dedicated user, attaches a policy and mints a key, and keeps the key in a Secret in the
// operator's namespace. The reconciler uses that key and never sees the root credentials.
type Bootstrap struct {
	// Client reads and writes the Secret and reads the ObjectStore. It should talk to the API
	// server directly: the manager's cache holds no Secrets.
	Client client.Client
	// Namespace is the operator's own namespace, which is the store's.
	Namespace string
	// RootCredentialsDir is where the root credentials are mounted, one file per key.
	RootCredentialsDir string
	// NewIAMClient connects to the store's IAM endpoint.
	NewIAMClient func(endpoint, accessKey, secretKey string) (iam.Client, error)
	// MinRetry and MaxRetry bound the wait between attempts, which doubles from the first to the
	// second. Both must be positive.
	MinRetry, MaxRetry time.Duration
}

// Start runs Ensure until it succeeds or ctx ends. The manager calls it once at startup.
func (b *Bootstrap) Start(ctx context.Context) error {
	delay := b.MinRetry
	for {
		err := b.Ensure(ctx)
		if err == nil {
			return nil
		}
		if errors.Is(err, ErrStoreNotReady) || errors.Is(err, ErrRootUnavailable) {
			log.FromContext(ctx).Info("waiting to create the operator's credentials", "reason", err.Error())
		} else {
			log.FromContext(ctx).Error(err, "failed to create the operator's credentials, retrying")
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(delay):
		}
		delay = min(delay*2, b.MaxRetry)
	}
}

// +kubebuilder:rbac:groups="",resources=secrets,verbs=create
// +kubebuilder:rbac:groups="",resources=secrets,resourceNames=cosi-operator-credentials,verbs=get;update
// +kubebuilder:rbac:groups=objectstore.vsan.broadcom.com,resources=objectstores,verbs=list

// Ensure makes one attempt to leave the Secret holding a complete key. Start calls it until it
// succeeds. It does nothing when the Secret is already complete, so a restart never mints a
// second key.
func (b *Bootstrap) Ensure(ctx context.Context) error {
	secret := &corev1.Secret{}
	err := b.Client.Get(ctx, client.ObjectKey{Namespace: b.Namespace, Name: SecretName}, secret)
	isFound := err == nil
	switch {
	case apierrors.IsNotFound(err):
		secret = &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: b.Namespace, Name: SecretName}}
	case err != nil:
		return fmt.Errorf("get Secret %q: %w", SecretName, err)
	case len(secret.Data[AccessKeyName]) > 0 && len(secret.Data[SecretKeyName]) > 0:
		return nil
	}

	store, err := objectstore.Find(ctx, b.Client, b.Namespace)
	if err != nil {
		return err
	}
	if store == nil || store.Status.HealthState != vboss.HealthStateReady {
		return ErrStoreNotReady
	}
	rootAccessKey, rootSecretKey, err := Read(b.RootCredentialsDir)
	if err != nil {
		return err
	}
	if rootAccessKey == "" || rootSecretKey == "" {
		return ErrRootUnavailable
	}
	iamClient, err := b.NewIAMClient(store.Status.IAMEndpoint.FQDN, rootAccessKey, rootSecretKey)
	if err != nil {
		return fmt.Errorf("connect to the object store: %w", err)
	}

	key, err := mint(ctx, iamClient)
	if err != nil {
		return err
	}
	if err := b.save(ctx, secret, isFound, store, key); err != nil {
		// The store cannot list keys, so one that is not kept could never be found again.
		if deleteErr := iamClient.DeleteAccessKey(ctx, UserName, key.ID); deleteErr != nil {
			err = errors.Join(err, deleteErr)
		}
		return err
	}
	return nil
}

// mint creates the user if it is missing, attaches the policy and adds a key.
func mint(ctx context.Context, c iam.Client) (iam.AccessKey, error) {
	if err := c.CreateUser(ctx, UserName); err != nil && !errors.Is(err, iam.ErrAlreadyExists) {
		return iam.AccessKey{}, err
	}
	if err := c.PutUserPolicy(ctx, UserName, PolicyName, policyDocument); err != nil {
		return iam.AccessKey{}, err
	}
	return c.CreateAccessKey(ctx, UserName)
}

// save writes the key into the Secret, which goes away with the store that issued the key.
func (b *Bootstrap) save(ctx context.Context, secret *corev1.Secret, isFound bool, store *vboss.ObjectStore, key iam.AccessKey) error {
	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}
	secret.Data[AccessKeyName] = []byte(key.ID)
	secret.Data[SecretKeyName] = []byte(key.Secret)
	secret.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: vboss.GroupVersion.String(),
		Kind:       "ObjectStore",
		Name:       store.Name,
		UID:        store.UID,
	}}
	if isFound {
		return b.Client.Update(ctx, secret)
	}
	return b.Client.Create(ctx, secret)
}
