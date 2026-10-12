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

package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"google.golang.org/grpc"

	cosi "sigs.k8s.io/container-object-storage-interface/proto"
	cositypes "sigs.k8s.io/vsphere-cosi-driver/pkg/cosi/types"
)

type Driver interface {
	Run(ctx context.Context, endpoint string) error
}

type vsphereCOSIDriver struct {
	log *zap.SugaredLogger
}

func NewDriver(log *zap.SugaredLogger) Driver {
	return &vsphereCOSIDriver{log: log}
}

func (driver *vsphereCOSIDriver) Run(ctx context.Context, endpoint string) error {
	listener, err := listen(endpoint)
	if err != nil {
		return err
	}

	server := grpc.NewServer()
	cosi.RegisterIdentityServer(server, &IdentityServer{})
	cosi.RegisterProvisionerServer(server, &ProvisionerServer{})

	// Serve returns before GracefulStop finishes draining in-flight RPCs.
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-ctx.Done()
		driver.log.Info("shutting down")
		server.GracefulStop()
	}()

	driver.log.Infof("serving COSI driver %q on %s", cositypes.Name, endpoint)
	if err := server.Serve(listener); err != nil {
		return fmt.Errorf("gRPC server failed: %w", err)
	}
	<-done
	return nil
}
