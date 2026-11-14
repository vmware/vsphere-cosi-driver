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

// Package s3 is the operator's view of an S3 object store: the three bucket calls it needs, and
// errors it can branch on. The driver never imports it; only the operator talks S3.
package s3

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

// Errors returned by the Client methods. Callers are expected to match them with errors.Is.
var (
	// ErrNotFound means the bucket does not exist.
	ErrNotFound = errors.New("bucket not found")
	// ErrAlreadyExists means the name is taken. It does not say by whom.
	ErrAlreadyExists = errors.New("bucket already exists")
	// ErrNotEmpty means a delete was refused because the bucket still holds objects.
	ErrNotEmpty = errors.New("bucket is not empty")
	// ErrInvalidName means the store rejected the bucket name.
	ErrInvalidName = errors.New("invalid bucket name")
	// ErrAccessDenied means the store rejected the credentials or the permission.
	ErrAccessDenied = errors.New("access denied")
	// ErrUnavailable means the store could not be reached or answered with a server error.
	ErrUnavailable = errors.New("object store unavailable")
)

// Client is the S3 surface the operator uses. It is defined here, next to its two
// implementations, because the operator is the only consumer.
type Client interface {
	// BucketExists reports whether the bucket exists. A missing bucket is (false, nil), not an
	// error.
	BucketExists(ctx context.Context, name string) (exists bool, err error)
	// CreateBucket returns ErrAlreadyExists when the name is taken, including by the caller.
	// Callers that want to adopt an existing bucket call BucketExists first.
	CreateBucket(ctx context.Context, name string) error
	// DeleteBucket returns ErrNotFound when the bucket is gone and ErrNotEmpty when it holds
	// objects. It never deletes objects.
	DeleteBucket(ctx context.Context, name string) error
}

// region is a placeholder: the vSAN object store has no regions but the SDK needs one to sign.
const region = "us-east-1"

type client struct {
	api *awss3.Client
}

// New returns a Client for the store at endpoint, which must be an http or https URL such as
// "https://s3.example.test:9021". Requests are path-style and signed with the given keys.
// httpClient may be nil.
func New(endpoint, accessKey, secretKey string, httpClient *http.Client) (Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse S3 endpoint: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("S3 endpoint must be an http or https URL with a host")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &client{api: awss3.New(awss3.Options{
		Region:       region,
		BaseEndpoint: aws.String(endpoint),
		Credentials:  credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		UsePathStyle: true,
		HTTPClient:   httpClient,
		Retryer:      aws.NopRetryer{}, // the reconciler retries with backoff; two layers would multiply the delay
	})}, nil
}

func (c *client) BucketExists(ctx context.Context, name string) (bool, error) {
	_, err := c.api.HeadBucket(ctx, &awss3.HeadBucketInput{Bucket: aws.String(name)})
	if err == nil {
		return true, nil
	}
	err = toSentinel(err)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return false, fmt.Errorf("check bucket %q: %w", name, err)
}

func (c *client) CreateBucket(ctx context.Context, name string) error {
	if _, err := c.api.CreateBucket(ctx, &awss3.CreateBucketInput{Bucket: aws.String(name)}); err != nil {
		return fmt.Errorf("create bucket %q: %w", name, toSentinel(err))
	}
	return nil
}

func (c *client) DeleteBucket(ctx context.Context, name string) error {
	if _, err := c.api.DeleteBucket(ctx, &awss3.DeleteBucketInput{Bucket: aws.String(name)}); err != nil {
		return fmt.Errorf("delete bucket %q: %w", name, toSentinel(err))
	}
	return nil
}

// toSentinel maps an SDK error to one of the package's sentinels, retaining the cause.
func toSentinel(err error) error {
	join := func(sentinel error) error { return fmt.Errorf("%w: %w", sentinel, err) }

	if apiErr, ok := errors.AsType[smithy.APIError](err); ok {
		switch apiErr.ErrorCode() {
		case "NoSuchBucket", "NotFound":
			return join(ErrNotFound)
		case "BucketAlreadyExists", "BucketAlreadyOwnedByYou":
			return join(ErrAlreadyExists)
		case "BucketNotEmpty":
			return join(ErrNotEmpty)
		case "InvalidBucketName":
			return join(ErrInvalidName)
		case "AccessDenied", "InvalidAccessKeyId", "SignatureDoesNotMatch":
			return join(ErrAccessDenied)
		case "InternalError", "ServiceUnavailable", "SlowDown", "RequestTimeout":
			return join(ErrUnavailable)
		}
	}
	if respErr, ok := errors.AsType[*smithyhttp.ResponseError](err); ok {
		switch code := respErr.HTTPStatusCode(); {
		case code == 0 || code >= http.StatusInternalServerError:
			// No HTTP response at all (status 0) also lands here.
			return join(ErrUnavailable)
		case code == http.StatusNotFound:
			return join(ErrNotFound)
		case code == http.StatusForbidden:
			return join(ErrAccessDenied)
		case code == http.StatusBadRequest:
			// The bucket name is the only input we send.
			return join(ErrInvalidName)
		}
	}
	if _, ok := errors.AsType[net.Error](err); ok || errors.Is(err, context.DeadlineExceeded) {
		return join(ErrUnavailable)
	}
	return err
}
