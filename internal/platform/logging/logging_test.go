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

package logging_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"

	"github.com/vmware/vsphere-cosi-driver/internal/platform/logging"
)

func TestNewLogger(t *testing.T) {
	for _, tc := range []struct {
		name    string
		level   string
		enabled zapcore.Level
		off     zapcore.Level
	}{
		{"WhenLevelIsEmpty", "", zapcore.InfoLevel, zapcore.DebugLevel},
		{"WhenLevelIsDebug", "debug", zapcore.DebugLevel, zapcore.Level(-2)},
		{"WhenLevelIsUpperCase", "WARN", zapcore.WarnLevel, zapcore.InfoLevel},
		{"WhenLevelIsError", "error", zapcore.ErrorLevel, zapcore.WarnLevel},
	} {
		t.Run(tc.name, func(tt *testing.T) {
			// Execute
			l, err := logging.New(tc.level)

			// Assert
			require.NoError(tt, err)
			assert.True(tt, l.Core().Enabled(tc.enabled))
			assert.False(tt, l.Core().Enabled(tc.off))
		})
	}

	t.Run("WhenLevelIsUnknown", func(tt *testing.T) {
		// Execute
		l, err := logging.New("loud")

		// Assert
		require.Error(tt, err)
		assert.Nil(tt, l)
	})
}
