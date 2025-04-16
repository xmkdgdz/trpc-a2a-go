// Tencent is pleased to support the open source community by making tRPC available.
//
// Copyright (C) 2025 THL A29 Limited, a Tencent company.
// All rights reserved.
//
// If you have downloaded a copy of the tRPC source code from Tencent,
// please note that tRPC source code is licensed under the  Apache 2.0 License,
// A copy of the Apache 2.0 License is included in this file.

package jsonrpc

import "encoding/json"

// Request represents a JSON-RPC request object.
type Request struct {
	Message
	// Method is a String containing the name of the method to be invoked.
	Method string `json:"method"`
	// Params is a Structured value that holds the parameter values to be used
	// during the invocation of the method. This member MAY be omitted.
	// It's stored as raw JSON to be decoded by the method handler.
	Params json.RawMessage `json:"params,omitempty"`
}

// NewRequest creates a new JSON-RPC request with the given method and ID.
func NewRequest(method string, id interface{}) *Request {
	return &Request{
		Message: Message{
			JSONRPC: Version,
			ID:      id,
		},
		Method: method,
	}
}
