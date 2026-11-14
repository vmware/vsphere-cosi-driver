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

// Package errs classifies failures as retryable or terminal. Lower layers return plain wrapped
// errors; the boundary that decides what to do (a reconciler, an RPC handler) reads the class.
package errs

import "errors"

type classified struct {
	err      error
	terminal bool
	reason   string
}

func (e *classified) Error() string { return e.err.Error() }
func (e *classified) Unwrap() error { return e.err }

// Retryable marks err as worth retrying with backoff. It returns nil for a nil err.
func Retryable(err error) error {
	if err == nil {
		return nil
	}
	return &classified{err: err}
}

// Terminal marks err as permanent: retrying cannot help until the input changes. reason is a
// short CamelCase word shown to users, such as the status condition reason. It returns nil for
// a nil err.
func Terminal(err error, reason string) error {
	if err == nil {
		return nil
	}
	return &classified{err: err, terminal: true, reason: reason}
}

// IsTerminal reports whether the outermost classification in err is Terminal. An unclassified
// error is not terminal, so unknown failures are retried.
func IsTerminal(err error) bool {
	var c *classified
	return errors.As(err, &c) && c.terminal
}

// Reason returns the reason of a terminal error, or "" for any other error.
func Reason(err error) string {
	var c *classified
	if errors.As(err, &c) && c.terminal {
		return c.reason
	}
	return ""
}
