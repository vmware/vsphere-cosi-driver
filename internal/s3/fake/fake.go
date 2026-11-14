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

// Package fake is an in-memory S3 object store for tests and the fake stand-in. It returns the
// same errors as the real client and is held to the same contract suite.
package fake

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/vmware/vsphere-cosi-driver/internal/s3"
)

// validName is the S3 rule for DNS-compatible names: 3 to 63 lower-case letters, digits, dots
// and hyphens, starting and ending with a letter or digit.
var validName = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

// Client is an in-memory store. The zero value is not usable; call New.
type Client struct {
	mu      sync.Mutex
	buckets map[string]bool // name -> holds objects
}

var _ s3.Client = (*Client)(nil)

// New returns an empty store.
func New() *Client { return &Client{buckets: map[string]bool{}} }

// AddObject makes an existing bucket non-empty, so that deleting it fails like the real store.
// It does nothing when the bucket does not exist.
func (c *Client) AddObject(bucket string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.buckets[bucket]; ok {
		c.buckets[bucket] = true
	}
}

// BucketExists implements s3.Client.
func (c *Client) BucketExists(_ context.Context, name string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.buckets[name]
	return ok, nil
}

// CreateBucket implements s3.Client.
func (c *Client) CreateBucket(_ context.Context, name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !validName.MatchString(name) {
		return fmt.Errorf("create bucket %q: %w", name, s3.ErrInvalidName)
	}
	if _, ok := c.buckets[name]; ok {
		return fmt.Errorf("create bucket %q: %w", name, s3.ErrAlreadyExists)
	}
	c.buckets[name] = false
	return nil
}

// DeleteBucket implements s3.Client.
func (c *Client) DeleteBucket(_ context.Context, name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	notEmpty, ok := c.buckets[name]
	switch {
	case !ok:
		return fmt.Errorf("delete bucket %q: %w", name, s3.ErrNotFound)
	case notEmpty:
		return fmt.Errorf("delete bucket %q: %w", name, s3.ErrNotEmpty)
	}
	delete(c.buckets, name)
	return nil
}

// NewHandler serves the bucket calls from c over HTTP, so the real client can be tested against
// the same state as the fake.
func NewHandler(c *Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.Trim(r.URL.Path, "/")
		var err error
		status := http.StatusOK
		switch r.Method {
		case http.MethodHead:
			var exists bool
			exists, err = c.BucketExists(r.Context(), name)
			if err == nil && !exists {
				w.WriteHeader(http.StatusNotFound)
				return
			}
		case http.MethodPut:
			err = c.CreateBucket(r.Context(), name)
		case http.MethodDelete:
			err = c.DeleteBucket(r.Context(), name)
			status = http.StatusNoContent
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if err == nil {
			w.WriteHeader(status)
			return
		}
		writeError(w, err)
	})
}

func writeError(w http.ResponseWriter, err error) {
	status, code := http.StatusInternalServerError, "InternalError"
	switch {
	case errors.Is(err, s3.ErrInvalidName):
		status, code = http.StatusBadRequest, "InvalidBucketName"
	case errors.Is(err, s3.ErrAlreadyExists):
		status, code = http.StatusConflict, "BucketAlreadyExists"
	case errors.Is(err, s3.ErrNotEmpty):
		status, code = http.StatusConflict, "BucketNotEmpty"
	case errors.Is(err, s3.ErrNotFound):
		status, code = http.StatusNotFound, "NoSuchBucket"
	}
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>%s</Code><Message>%s</Message><RequestId>fake</RequestId></Error>`, code, code)
}
