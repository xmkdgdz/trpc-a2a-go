// Tencent is pleased to support the open source community by making tRPC available.
//
// Copyright (C) 2025 THL A29 Limited, a Tencent company.
// All rights reserved.
//
// If you have downloaded a copy of the tRPC source code from Tencent,
// please note that tRPC source code is licensed under the  Apache 2.0 License,
// A copy of the Apache 2.0 License is included in this file.

package client

import (
	"net/http"
	"time"
)

// Option is a functional option type for configuring the A2AClient.
type Option func(*A2AClient)

// WithHTTPClient sets a custom http.Client for the A2AClient.
func WithHTTPClient(client *http.Client) Option {
	return func(c *A2AClient) {
		if client != nil {
			c.httpClient = client
		}
	}
}

// WithTimeout sets the timeout for the underlying http.Client.
// If a custom client was provided via WithHTTPClient, this modifies its timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(c *A2AClient) {
		if timeout > 0 && c.httpClient != nil {
			c.httpClient.Timeout = timeout
		}
	}
}

// WithUserAgent sets a custom User-Agent header for requests.
func WithUserAgent(userAgent string) Option {
	return func(c *A2AClient) {
		c.userAgent = userAgent
	}
}
