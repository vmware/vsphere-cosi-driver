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

package fake

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/vmware/vsphere-cosi-driver/internal/iam"
)

type user struct {
	policies map[string]string // policy name -> document
	keys     map[string]string // key ID -> secret
}

// Client is an in-memory service. The zero value is not usable; call New.
type Client struct {
	mu      sync.Mutex
	users   map[string]*user
	counter int
}

var _ iam.Client = (*Client)(nil)

// New returns an empty service.
func New() *Client { return &Client{users: map[string]*user{}} }

// HasUser reports whether the user exists, so a test can check what the Client interface cannot
// show.
func (c *Client) HasUser(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.users[name]
	return ok
}

// Policy returns the user's inline policy of that name.
func (c *Client) Policy(userName, policyName string) (document string, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	u, found := c.users[userName]
	if !found {
		return "", false
	}
	document, ok = u.policies[policyName]
	return document, ok
}

// HasKey reports whether the user has a key with that ID.
func (c *Client) HasKey(userName, id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	u, found := c.users[userName]
	if !found {
		return false
	}
	_, ok := u.keys[id]
	return ok
}

// CreateUser implements iam.Client.
func (c *Client) CreateUser(_ context.Context, name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.users[name]; ok {
		return fmt.Errorf("create user %q: %w", name, iam.ErrAlreadyExists)
	}
	c.users[name] = &user{policies: map[string]string{}, keys: map[string]string{}}
	return nil
}

// PutUserPolicy implements iam.Client.
func (c *Client) PutUserPolicy(_ context.Context, userName, policyName, document string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	u, ok := c.users[userName]
	if !ok {
		return fmt.Errorf("put policy %q on user %q: %w", policyName, userName, iam.ErrNotFound)
	}
	u.policies[policyName] = document
	return nil
}

// CreateAccessKey implements iam.Client.
func (c *Client) CreateAccessKey(_ context.Context, userName string) (iam.AccessKey, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	u, ok := c.users[userName]
	if !ok {
		return iam.AccessKey{}, fmt.Errorf("create access key for user %q: %w", userName, iam.ErrNotFound)
	}
	c.counter++
	key := iam.AccessKey{ID: fmt.Sprintf("AKIAFAKE%012d", c.counter), Secret: fmt.Sprintf("fake-secret-%d", c.counter)}
	u.keys[key.ID] = key.Secret
	return key, nil
}

// DeleteAccessKey implements iam.Client.
func (c *Client) DeleteAccessKey(_ context.Context, userName, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	u, ok := c.users[userName]
	if !ok {
		return fmt.Errorf("delete access key of user %q: %w", userName, iam.ErrNotFound)
	}
	if _, ok := u.keys[id]; !ok {
		return fmt.Errorf("delete access key of user %q: %w", userName, iam.ErrNotFound)
	}
	delete(u.keys, id)
	return nil
}

const xmlns = "https://iam.amazonaws.com/doc/2010-05-08/"

// NewHandler serves the IAM calls from c over HTTP, so the real client can be tested against the
// same state as the fake.
func NewHandler(c *Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil {
			writeCode(w, http.StatusBadRequest, "ValidationError")
			return
		}
		var (
			action = r.PostForm.Get("Action")
			name   = r.PostForm.Get("UserName")
			result string
			err    error
		)
		switch action {
		case "CreateUser":
			err = c.CreateUser(r.Context(), name)
			result = fmt.Sprintf("<User><UserName>%s</UserName></User>", escape(name))
		case "PutUserPolicy":
			err = c.PutUserPolicy(r.Context(), name, r.PostForm.Get("PolicyName"), r.PostForm.Get("PolicyDocument"))
		case "CreateAccessKey":
			var key iam.AccessKey
			key, err = c.CreateAccessKey(r.Context(), name)
			result = fmt.Sprintf("<AccessKey><UserName>%s</UserName><AccessKeyId>%s</AccessKeyId><Status>Active</Status><SecretAccessKey>%s</SecretAccessKey></AccessKey>",
				escape(name), escape(key.ID), escape(key.Secret))
		case "DeleteAccessKey":
			err = c.DeleteAccessKey(r.Context(), name, r.PostForm.Get("AccessKeyId"))
		default:
			writeCode(w, http.StatusBadRequest, "InvalidAction")
			return
		}
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(http.StatusOK)
		if result != "" {
			result = fmt.Sprintf("<%sResult>%s</%sResult>", action, result, action)
		}
		_, _ = fmt.Fprintf(w, `<%sResponse xmlns="%s">%s<ResponseMetadata><RequestId>fake</RequestId></ResponseMetadata></%sResponse>`, action, xmlns, result, action)
	})
}

// writeError answers with the error the fake returned, which is always one of these two.
func writeError(w http.ResponseWriter, err error) {
	if errors.Is(err, iam.ErrAlreadyExists) {
		writeCode(w, http.StatusConflict, "EntityAlreadyExists")
		return
	}
	writeCode(w, http.StatusNotFound, "NoSuchEntity")
}

func writeCode(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `<ErrorResponse xmlns="%s"><Error><Type>Sender</Type><Code>%s</Code><Message>%s</Message></Error><RequestId>fake</RequestId></ErrorResponse>`, xmlns, code, code)
}

// escape makes a value safe to place in an XML element.
func escape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
