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

	cosi "sigs.k8s.io/container-object-storage-interface/proto"
	cositypes "sigs.k8s.io/vsphere-cosi-driver/pkg/cosi/types"
)

type IdentityServer struct {
	cosi.UnimplementedIdentityServer
}

func (driver *IdentityServer) DriverGetInfo(
	ctx context.Context,
	req *cosi.DriverGetInfoRequest) (
	*cosi.DriverGetInfoResponse, error) {

	return &cosi.DriverGetInfoResponse{
		Name: cositypes.Name,
		SupportedProtocols: []*cosi.ObjectProtocol{
			{Type: cosi.ObjectProtocol_S3},
		},
	}, nil
}
