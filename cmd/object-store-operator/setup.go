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

package main

import (
	"fmt"
	"time"

	"github.com/vmware/vsphere-cosi-driver/internal/iam"
	"github.com/vmware/vsphere-cosi-driver/internal/operator/bucket"
	"github.com/vmware/vsphere-cosi-driver/internal/operator/credentials"
	"github.com/vmware/vsphere-cosi-driver/internal/platform/config"
	"github.com/vmware/vsphere-cosi-driver/internal/s3"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// The wait between attempts to create the operator's key starts at the first value and doubles up
// to the second.
const (
	minBootstrapRetry = time.Second
	maxBootstrapRetry = time.Minute
)

// setup adds the key bootstrap and the bucket reconciler to the manager.
func setup(mgr ctrl.Manager, cfg config.Config) error {
	// The manager's cache holds no Secrets, so the bootstrap reads and writes them directly.
	direct, err := client.New(mgr.GetConfig(), client.Options{Scheme: mgr.GetScheme()})
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}
	err = mgr.Add(&credentials.Bootstrap{
		Client:             direct,
		Namespace:          cfg.Namespace,
		RootCredentialsDir: cfg.RootCredentialsDir,
		NewIAMClient:       newIAMClient,
		MinRetry:           minBootstrapRetry,
		MaxRetry:           maxBootstrapRetry,
	})
	if err != nil {
		return fmt.Errorf("add the credentials bootstrap: %w", err)
	}
	r := &bucket.Reconciler{Client: mgr.GetClient(), NewS3Client: newS3Client, CredentialsDir: cfg.CredentialsDir}
	return r.SetupWithManager(mgr)
}

func newS3Client(endpoint, accessKey, secretKey string) (s3.Client, error) {
	return s3.New(endpoint, accessKey, secretKey, nil)
}

func newIAMClient(endpoint, accessKey, secretKey string) (iam.Client, error) {
	return iam.New(endpoint, accessKey, secretKey, nil)
}
