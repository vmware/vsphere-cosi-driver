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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmware/vsphere-cosi-driver/internal/operator/credentials"
)

func writeFile(t *testing.T, dir, name, value string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(value), 0o600))
}

func TestRead(t *testing.T) {
	t.Run("WhenBothFilesExist", func(tt *testing.T) {
		// Setup
		dir := tt.TempDir()
		writeFile(tt, dir, credentials.AccessKeyName, "access")
		writeFile(tt, dir, credentials.SecretKeyName, "secret")

		// Execute
		accessKey, secretKey, err := credentials.Read(dir)

		// Assert
		require.NoError(tt, err)
		assert.Equal(tt, "access", accessKey)
		assert.Equal(tt, "secret", secretKey)
	})

	t.Run("WhenFilesAreMissing", func(tt *testing.T) {
		// Execute
		accessKey, secretKey, err := credentials.Read(tt.TempDir())

		// Assert
		require.NoError(tt, err)
		assert.Empty(tt, accessKey)
		assert.Empty(tt, secretKey)
	})

	t.Run("WhenOnlyTheSecretKeyIsMissing", func(tt *testing.T) {
		// Setup
		dir := tt.TempDir()
		writeFile(tt, dir, credentials.AccessKeyName, "access")

		// Execute
		accessKey, secretKey, err := credentials.Read(dir)

		// Assert
		require.NoError(tt, err)
		assert.Equal(tt, "access", accessKey)
		assert.Empty(tt, secretKey)
	})

	t.Run("WhenTheAccessKeyCannotBeRead", func(tt *testing.T) {
		// Setup
		dir := tt.TempDir()
		require.NoError(tt, os.Mkdir(filepath.Join(dir, credentials.AccessKeyName), 0o700))

		// Execute
		accessKey, secretKey, err := credentials.Read(dir)

		// Assert
		require.Error(tt, err)
		assert.Empty(tt, accessKey)
		assert.Empty(tt, secretKey)
	})

	t.Run("WhenTheSecretKeyCannotBeRead", func(tt *testing.T) {
		// Setup
		dir := tt.TempDir()
		writeFile(tt, dir, credentials.AccessKeyName, "access")
		require.NoError(tt, os.Mkdir(filepath.Join(dir, credentials.SecretKeyName), 0o700))

		// Execute
		accessKey, secretKey, err := credentials.Read(dir)

		// Assert
		require.Error(tt, err)
		assert.Empty(tt, accessKey)
		assert.Empty(tt, secretKey)
	})
}
