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
	"os"

	"github.com/go-logr/zapr"
	"github.com/vmware/vsphere-cosi-driver/internal/platform/config"
	"github.com/vmware/vsphere-cosi-driver/internal/platform/kube"
	"github.com/vmware/vsphere-cosi-driver/internal/platform/logging"
	"github.com/vmware/vsphere-cosi-driver/internal/platform/manager"
	ctrl "sigs.k8s.io/controller-runtime"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "exiting on error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	logger, err := logging.New(cfg.LogLevel)
	if err != nil {
		return err
	}
	defer func() { _ = logger.Sync() }()
	ctrl.SetLogger(zapr.NewLogger(logger))

	scheme, err := kube.NewScheme()
	if err != nil {
		return err
	}
	mgr, err := manager.New(cfg, scheme)
	if err != nil {
		return err
	}
	if err := setup(mgr, cfg); err != nil {
		return err
	}
	return mgr.Start(ctrl.SetupSignalHandler())
}
