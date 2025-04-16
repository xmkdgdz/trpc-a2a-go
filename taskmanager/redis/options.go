// Tencent is pleased to support the open source community by making tRPC available.
//
// Copyright (C) 2025 THL A29 Limited, a Tencent company.
// All rights reserved.
//
// If you have downloaded a copy of the tRPC source code from Tencent,
// please note that tRPC source code is licensed under the  Apache 2.0 License,
// A copy of the Apache 2.0 License is included in this file.

package redis

import (
	"time"
)

// Option is a function that configures the RedisTaskManager.
type Option func(*TaskManager)

// WithExpiration sets the expiration time for Redis keys.
func WithExpiration(expiration time.Duration) Option {
	return func(o *TaskManager) {
		o.expiration = expiration
	}
}
