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

// Package boundary_test keeps the operator and the driver from importing each other's
// dependencies: the operator alone talks to S3, the driver alone speaks gRPC.
package boundary_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const module = "github.com/vmware/vsphere-cosi-driver"

func TestBoundary(t *testing.T) {
	root, err := filepath.Abs("../..")
	require.NoError(t, err)

	for _, tc := range []struct {
		name      string
		pkg       string
		forbidden []string
	}{
		{
			name: "WhenPackageIsTheOperator",
			pkg:  "cmd/object-store-operator",
			forbidden: []string{
				"google.golang.org/grpc",
				"sigs.k8s.io/container-object-storage-interface/proto",
				module + "/pkg/cosi",
			},
		},
		{
			name: "WhenPackageIsTheDriver",
			pkg:  "cmd/vsphere-cosi",
			forbidden: []string{
				"github.com/aws/aws-sdk-go-v2",
				module + "/internal/s3",
				module + "/internal/operator",
			},
		},
	} {
		t.Run(tc.name, func(tt *testing.T) {
			// Setup
			if _, err := os.Stat(filepath.Join(root, tc.pkg)); os.IsNotExist(err) {
				tt.Skipf("%s does not exist yet", tc.pkg)
			}

			// Execute
			cmd := exec.CommandContext(tt.Context(), "go", "list", "-deps", "-f", "{{.ImportPath}}", "./"+tc.pkg)
			cmd.Dir = root
			out, err := cmd.Output()
			require.NoError(tt, err)

			// Assert
			for _, dep := range strings.Fields(string(out)) {
				for _, bad := range tc.forbidden {
					require.Falsef(tt, dep == bad || strings.HasPrefix(dep, bad+"/"), "%s must not import %s (found %s)", tc.pkg, bad, dep)
				}
			}
		})
	}
}
