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

package iam

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awsiam "github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

// Errors returned by the Client methods. Callers are expected to match them with errors.Is.
var (
	// ErrNotFound means the user or the key does not exist.
	ErrNotFound = errors.New("user or key not found")
	// ErrAlreadyExists means the user exists.
	ErrAlreadyExists = errors.New("user already exists")
	// ErrAccessDenied means the store rejected the credentials or the permission.
	ErrAccessDenied = errors.New("access denied")
	// ErrUnavailable means the store could not be reached or answered with a server error.
	ErrUnavailable = errors.New("object store unavailable")
)

// AccessKey is a key pair. The store returns the secret only when it creates the key.
type AccessKey struct {
	ID     string
	Secret string
}

// Client is the IAM surface the operator uses. It is defined here, next to its two
// implementations, because the operator is the only consumer.
type Client interface {
	// CreateUser returns ErrAlreadyExists when the user exists.
	CreateUser(ctx context.Context, name string) error
	// PutUserPolicy sets the user's inline policy, replacing one of the same name. It returns
	// ErrNotFound when the user does not exist.
	PutUserPolicy(ctx context.Context, user, policyName, document string) error
	// CreateAccessKey adds a key to the user, next to any it already has. It returns ErrNotFound
	// when the user does not exist.
	CreateAccessKey(ctx context.Context, user string) (AccessKey, error)
	// DeleteAccessKey returns ErrNotFound when the key or the user does not exist.
	DeleteAccessKey(ctx context.Context, user, id string) error
}

// region is a placeholder: the vSAN object store has no regions but the SDK needs one to sign.
const region = "us-east-1"

type client struct {
	api *awsiam.Client
}

// New returns a Client for the store's IAM endpoint, which must be an http or https URL such as
// "https://iam.example.test:8443". Requests are signed with the given keys. httpClient may be nil.
func New(endpoint, accessKey, secretKey string, httpClient *http.Client) (Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse IAM endpoint: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("IAM endpoint must be an http or https URL with a host")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &client{api: awsiam.New(awsiam.Options{
		Region:       region,
		BaseEndpoint: aws.String(endpoint),
		Credentials:  credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		HTTPClient:   httpClient,
		Retryer:      aws.NopRetryer{}, // the caller retries with backoff; two layers would multiply the delay
	})}, nil
}

func (c *client) CreateUser(ctx context.Context, name string) error {
	if _, err := c.api.CreateUser(ctx, &awsiam.CreateUserInput{UserName: aws.String(name)}); err != nil {
		return fmt.Errorf("create user %q: %w", name, toSentinel(err))
	}
	return nil
}

func (c *client) PutUserPolicy(ctx context.Context, user, policyName, document string) error {
	_, err := c.api.PutUserPolicy(ctx, &awsiam.PutUserPolicyInput{
		UserName:       aws.String(user),
		PolicyName:     aws.String(policyName),
		PolicyDocument: aws.String(document),
	})
	if err != nil {
		return fmt.Errorf("put policy %q on user %q: %w", policyName, user, toSentinel(err))
	}
	return nil
}

func (c *client) CreateAccessKey(ctx context.Context, user string) (AccessKey, error) {
	out, err := c.api.CreateAccessKey(ctx, &awsiam.CreateAccessKeyInput{UserName: aws.String(user)})
	if err != nil {
		return AccessKey{}, fmt.Errorf("create access key for user %q: %w", user, toSentinel(err))
	}
	if out.AccessKey == nil || aws.ToString(out.AccessKey.AccessKeyId) == "" || aws.ToString(out.AccessKey.SecretAccessKey) == "" {
		return AccessKey{}, fmt.Errorf("create access key for user %q: the store returned no key", user)
	}
	return AccessKey{ID: aws.ToString(out.AccessKey.AccessKeyId), Secret: aws.ToString(out.AccessKey.SecretAccessKey)}, nil
}

func (c *client) DeleteAccessKey(ctx context.Context, user, id string) error {
	_, err := c.api.DeleteAccessKey(ctx, &awsiam.DeleteAccessKeyInput{UserName: aws.String(user), AccessKeyId: aws.String(id)})
	if err != nil {
		return fmt.Errorf("delete access key of user %q: %w", user, toSentinel(err))
	}
	return nil
}

// toSentinel maps an SDK error to one of the package's sentinels, retaining the cause. An error it
// does not recognise is returned as it is.
func toSentinel(err error) error {
	join := func(sentinel error) error { return fmt.Errorf("%w: %w", sentinel, err) }

	// These codes share a status with other errors, so the status alone cannot identify them.
	if apiErr, ok := errors.AsType[smithy.APIError](err); ok {
		switch apiErr.ErrorCode() {
		case "EntityAlreadyExists":
			return join(ErrAlreadyExists)
		case "Throttling":
			return join(ErrUnavailable)
		}
	}
	if respErr, ok := errors.AsType[*smithyhttp.ResponseError](err); ok {
		switch code := respErr.HTTPStatusCode(); {
		case code >= http.StatusInternalServerError:
			return join(ErrUnavailable)
		case code == http.StatusNotFound:
			return join(ErrNotFound)
		case code == http.StatusForbidden:
			return join(ErrAccessDenied)
		}
	}
	if _, ok := errors.AsType[net.Error](err); ok || errors.Is(err, context.DeadlineExceeded) {
		return join(ErrUnavailable)
	}
	return err
}
