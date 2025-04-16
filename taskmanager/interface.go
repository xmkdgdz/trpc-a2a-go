// Tencent is pleased to support the open source community by making tRPC available.
//
// Copyright (C) 2025 THL A29 Limited, a Tencent company.
// All rights reserved.
//
// If you have downloaded a copy of the tRPC source code from Tencent,
// please note that tRPC source code is licensed under the  Apache 2.0 License,
// A copy of the Apache 2.0 License is included in this file.

// Package taskmanager defines interfaces and implementations for managing A2A task lifecycles.
package taskmanager

import (
	"context"

	"trpc.group/trpc-go/a2a-go/protocol"
)

// TaskHandle provides methods for the agent logic (TaskProcessor) to interact
// with the task manager during processing. It encapsulates the necessary callbacks.
type TaskHandle interface {
	// UpdateStatus updates the task's state and optional message.
	// Returns an error if the task cannot be found or updated.
	UpdateStatus(state protocol.TaskState, msg *protocol.Message) error

	// AddArtifact adds a new artifact to the task.
	// Returns an error if the task cannot be found or updated.
	AddArtifact(artifact protocol.Artifact) error
}

// TaskProcessor defines the interface for the core agent logic that processes a task.
// Implementations of this interface are injected into a TaskManager.
type TaskProcessor interface {
	// Process executes the specific logic for a task.
	// It receives the task ID, the initial message, and a TaskHandle for callbacks.
	// It should use handle.Context() to check for cancellation.
	// It should report progress and results via handle.UpdateStatus and handle.AddArtifact.
	// Returning an error indicates the processing failed fundamentally.
	Process(ctx context.Context, taskID string, initialMsg protocol.Message, handle TaskHandle) error
}

// TaskManager defines the interface for managing A2A task lifecycles based on the protocol.
// Implementations handle task creation, updates, retrieval, cancellation, and events,
// delegating the actual processing logic to an injected TaskProcessor.
// This interface corresponds to the Task Service defined in the A2A Specification.
// Exported interface.
type TaskManager interface {
	// OnSendTask handles a request corresponding to the 'tasks/send' RPC method.
	// It creates and potentially starts processing a new task via the TaskProcessor.
	// It returns the initial state of the task, possibly reflecting immediate processing results.
	OnSendTask(ctx context.Context, request protocol.SendTaskParams) (*protocol.Task, error)

	// OnSendTaskSubscribe handles a request corresponding to the 'tasks/sendSubscribe' RPC method.
	// It creates a new task and returns a channel for receiving TaskEvent updates (streaming).
	// It initiates asynchronous processing via the TaskProcessor.
	// The channel will be closed when the task reaches a final state or an error occurs during setup/processing.
	OnSendTaskSubscribe(ctx context.Context, request protocol.SendTaskParams) (<-chan protocol.TaskEvent, error)

	// OnGetTask handles a request corresponding to the 'tasks/get' RPC method.
	// It retrieves the current state of an existing task.
	OnGetTask(ctx context.Context, params protocol.TaskQueryParams) (*protocol.Task, error)

	// OnCancelTask handles a request corresponding to the 'tasks/cancel' RPC method.
	// It requests the cancellation of an ongoing task.
	// This typically involves canceling the context passed to the TaskProcessor.
	// It returns the task state after the cancellation attempt.
	OnCancelTask(ctx context.Context, params protocol.TaskIDParams) (*protocol.Task, error)

	// OnPushNotificationSet handles a request corresponding to the 'tasks/pushNotification/set' RPC method.
	// It configures push notifications for a specific task.
	OnPushNotificationSet(ctx context.Context, params protocol.TaskPushNotificationConfig) (*protocol.TaskPushNotificationConfig, error)

	// OnPushNotificationGet handles a request corresponding to the 'tasks/pushNotification/get' RPC method.
	// It retrieves the current push notification configuration for a task.
	OnPushNotificationGet(ctx context.Context, params protocol.TaskIDParams) (*protocol.TaskPushNotificationConfig, error)

	// OnResubscribe handles a request corresponding to the 'tasks/resubscribe' RPC method.
	// It reestablishes an SSE stream for an existing task.
	OnResubscribe(ctx context.Context, params protocol.TaskIDParams) (<-chan protocol.TaskEvent, error)
}
