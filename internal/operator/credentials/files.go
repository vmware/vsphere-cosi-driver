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

// Package credentials holds the operator's access to its object store: the key pair as a Secret
// and as mounted files, and the bootstrap that creates it.
package credentials

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// The names of the keys of a credentials Secret. A mounted Secret shows each key as a file of the
// same name.
const (
	AccessKeyName = "akey"
	SecretKeyName = "secret"
)

// Read returns the key pair mounted in dir, one file per key. A reconciler calls it on every pass.
// A missing file gives an empty value, so a caller can tell a Secret that is not there yet from
// one that cannot be read.
func Read(dir string) (accessKey, secretKey string, err error) {
	accessKey, err = readFile(filepath.Join(dir, AccessKeyName))
	if err != nil {
		return "", "", err
	}
	secretKey, err = readFile(filepath.Join(dir, SecretKeyName))
	if err != nil {
		return "", "", err
	}
	return accessKey, secretKey, nil
}

func readFile(path string) (string, error) {
	value, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read credentials: %w", err)
	}
	return string(value), nil
}
