// Tencent is pleased to support the open source community by making tRPC available.
//
// Copyright (C) 2025 THL A29 Limited, a Tencent company.
// All rights reserved.
//
// If you have downloaded a copy of the tRPC source code from Tencent,
// please note that tRPC source code is licensed under the  Apache 2.0 License,
// A copy of the Apache 2.0 License is included in this file.

// Package main implements a basic example of streaming a task from a client to a server.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"trpc.group/trpc-go/a2a-go/client"
	"trpc.group/trpc-go/a2a-go/protocol"
)

func main() {
	// 1. Create a new client instance.
	// Assuming the server is running locally on port 8080.
	c, err := client.NewA2AClient("http://localhost:8080") // Corrected: Use NewA2AClient
	if err != nil {
		log.Fatalf("Error creating client: %v.", err)
	}

	// 2. Define the task specification (using TaskSendParams for streaming).
	// Note: StreamTask now expects TaskSendParams.
	// The specific TaskSpec details are often encapsulated within the initial Message.
	taskParams := protocol.SendTaskParams{
		ID: fmt.Sprintf("stream-task-%d-%s", time.Now().UnixNano(), uuid.New().String()), // Generate a unique Task ID
		Message: protocol.Message{
			Role: protocol.MessageRoleUser,
			Parts: []protocol.Part{
				protocol.NewTextPart("Process this streaming data chunk by chunk."), // Example user input
			},
		},
		AcceptedOutputModes: []string{"stream"}, // Indicate preference for streaming
	}

	// 3. Create a context with a timeout for the streaming operation.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 4. Call StreamTask to start the SSE stream.
	// This returns a channel that will receive TaskEvent updates.
	streamChan, err := c.StreamTask(ctx, taskParams)
	if err != nil {
		log.Fatalf("Error starting stream task: %v.", err)
	}

	log.Println("Waiting for streaming updates...")

	// 5. Process the stream.
	for {
		select {
		case <-ctx.Done():
			// Context timed out or was cancelled.
			log.Println("Streaming context done:", ctx.Err())
			return
		case event, ok := <-streamChan:
			if !ok {
				// Channel closed by the client/server (e.g., task finished or error).
				log.Println("Stream closed.")
				// Check context error again in case it was cancelled concurrently.
				if ctx.Err() != nil {
					log.Println("Context error after stream close:", ctx.Err())
				}
				return
			}
			// Process the received event.
			switch e := event.(type) {
			case protocol.TaskStatusUpdateEvent:
				fmt.Printf("Received Status Update - TaskID: %s, State: %s, Final: %t\n", e.ID, e.Status.State, e.Final)
				if e.Status.Message != nil {
					fmt.Printf("  Status Message: Role=%s, Parts=%+v\n", e.Status.Message.Role, e.Status.Message.Parts)
				}
				if e.IsFinal() {
					log.Println("Received final status update, exiting.")
					return
				}
			case protocol.TaskArtifactUpdateEvent:
				fmt.Printf("Received Artifact Update - TaskID: %s, Index: %d, Append: %v, LastChunk: %v\n",
					e.ID, e.Artifact.Index, e.Artifact.Append, e.Artifact.LastChunk)
				fmt.Printf("  Artifact Parts: %+v\n", e.Artifact.Parts)
				if e.IsFinal() {
					log.Println("Received final artifact update, exiting.")
					return
				}
			default:
				log.Printf("Received unknown event type: %T %v", event, event)
			}
		}
	}
}
