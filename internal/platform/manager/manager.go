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

// Package manager builds the controller-runtime manager: metrics, health probes and a cache
// limited to the store's namespace. The operator runs one replica per store, so leader election
// is off (follow-up: confirm with the team whether HA is needed).
package manager

import (
	"fmt"

	"github.com/vmware/vsphere-cosi-driver/internal/platform/config"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// NewOptions returns the manager options for cfg. It is separate from New so tests can inspect
// them without a cluster.
func NewOptions(cfg config.Config, scheme *runtime.Scheme) ctrl.Options {
	return ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: cfg.MetricsAddr},
		HealthProbeBindAddress: cfg.ProbeAddr,
		LeaderElection:         false,
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{cfg.Namespace: {}},
		},
	}
}

// New creates the manager for the in-cluster or kubeconfig-selected cluster and registers the
// health and readiness probes.
func New(cfg config.Config, scheme *runtime.Scheme) (ctrl.Manager, error) {
	restCfg, err := ctrl.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("load Kubernetes client configuration: %w", err)
	}
	mgr, err := ctrl.NewManager(restCfg, NewOptions(cfg, scheme))
	if err != nil {
		return nil, fmt.Errorf("create manager: %w", err)
	}
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return nil, fmt.Errorf("add health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return nil, fmt.Errorf("add readiness check: %w", err)
	}
	return mgr, nil
}
