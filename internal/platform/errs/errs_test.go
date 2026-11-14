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

package errs_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmware/vsphere-cosi-driver/internal/platform/errs"
)

func TestErrsClassify(t *testing.T) {
	base := errors.New("boom")

	t.Run("WhenErrorIsTerminal", func(tt *testing.T) {
		// Setup
		err := errs.Terminal(base, "Unsupported")

		// Execute / Assert
		assert.True(tt, errs.IsTerminal(err))
		assert.Equal(tt, "Unsupported", errs.Reason(err))
		assert.ErrorIs(tt, err, base)
	})

	t.Run("WhenTerminalErrorIsWrapped", func(tt *testing.T) {
		// Setup
		err := fmt.Errorf("reconcile: %w", errs.Terminal(base, "BucketNameTaken"))

		// Execute / Assert
		assert.True(tt, errs.IsTerminal(err))
		assert.Equal(tt, "BucketNameTaken", errs.Reason(err))
		assert.ErrorIs(tt, err, base)
	})

	t.Run("WhenErrorIsRetryable", func(tt *testing.T) {
		// Setup
		err := errs.Retryable(base)

		// Execute / Assert
		assert.False(tt, errs.IsTerminal(err))
		assert.Empty(tt, errs.Reason(err))
		assert.ErrorIs(tt, err, base)
	})

	t.Run("WhenErrorIsUnclassified", func(tt *testing.T) {
		// Execute / Assert: unknown failures are retried, never silently dropped.
		assert.False(tt, errs.IsTerminal(base))
		assert.Empty(tt, errs.Reason(base))
	})

	t.Run("WhenRetryableWrapsTerminal", func(tt *testing.T) {
		// Setup
		err := errs.Retryable(errs.Terminal(base, "Unsupported"))

		// Execute / Assert: the outermost classification wins.
		assert.False(tt, errs.IsTerminal(err))
	})

	t.Run("WhenErrorIsNil", func(tt *testing.T) {
		// Execute / Assert
		require.NoError(tt, errs.Retryable(nil))
		require.NoError(tt, errs.Terminal(nil, "Unsupported"))
		assert.False(tt, errs.IsTerminal(nil))
	})
}
