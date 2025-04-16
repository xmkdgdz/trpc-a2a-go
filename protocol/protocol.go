// Tencent is pleased to support the open source community by making tRPC available.
//
// Copyright (C) 2025 THL A29 Limited, a Tencent company.
// All rights reserved.
//
// If you have downloaded a copy of the tRPC source code from Tencent,
// please note that tRPC source code is licensed under the  Apache 2.0 License,
// A copy of the Apache 2.0 License is included in this file.

// Package protocol defines constants and potentially shared types for the A2A protocol itself.
package protocol

// A2A RPC Method Names define the standard method strings used in the A2A protocol's Task Service.
const (
	MethodTasksSend                = "tasks/send"
	MethodTasksSendSubscribe       = "tasks/sendSubscribe"
	MethodTasksGet                 = "tasks/get"
	MethodTasksCancel              = "tasks/cancel"
	MethodTasksPushNotificationSet = "tasks/pushNotification/set"
	MethodTasksPushNotificationGet = "tasks/pushNotification/get"
	MethodTasksResubscribe         = "tasks/resubscribe"
)

// A2A SSE Event Types define the standard event type strings used in A2A SSE streams.
const (
	EventTaskStatusUpdate   = "task_status_update"
	EventTaskArtifactUpdate = "task_artifact_update"
	// EventClose is used internally by this implementation's server to signal stream closure.
	// Note: This might not be part of the formal A2A spec but is used in server logic.
	EventClose = "close"
)

// A2A HTTP Endpoint Paths define the standard paths used in the A2A protocol.
const (
	// AgentCardPath is the path for the agent metadata JSON endpoint.
	AgentCardPath = "/.well-known/agent.json"
	// JWKSPath is the path for the JWKS endpoint.
	JWKSPath = "/.well-known/jwks.json"
	// DefaultJSONRPCPath is the default path for the JSON-RPC endpoint.
	DefaultJSONRPCPath = "/"
)
