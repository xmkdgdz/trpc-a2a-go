// Tencent is pleased to support the open source community by making tRPC available.
//
// Copyright (C) 2025 THL A29 Limited, a Tencent company.
// All rights reserved.
//
// If you have downloaded a copy of the tRPC source code from Tencent,
// please note that tRPC source code is licensed under the  Apache 2.0 License,
// A copy of the Apache 2.0 License is included in this file.

package taskmanager

// memoryTaskHandle implements the TaskHandle interface, providing callbacks
// for a specific task being processed by a TaskProcessor.
// It holds a reference back to the MemoryTaskManager.
type memoryTaskHandle struct {
	taskID  string
	manager *MemoryTaskManager
}

// UpdateStatus implements TaskHandle.
func (h *memoryTaskHandle) UpdateStatus(state TaskState, msg *Message) error {
	return h.manager.UpdateTaskStatus(h.taskID, state, msg)
}

// AddArtifact implements TaskHandle.
func (h *memoryTaskHandle) AddArtifact(artifact Artifact) error {
	return h.manager.AddArtifact(h.taskID, artifact)
}
