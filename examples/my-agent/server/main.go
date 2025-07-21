// Tencent is pleased to support the open source community by making trpc-a2a-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-a2a-go is licensed under the Apache License Version 2.0.

// Package main implements my A2A server example.
package main

import (
	"context"
	"log"

	"trpc.group/trpc-go/trpc-a2a-go/protocol"
	"trpc.group/trpc-go/trpc-a2a-go/server"
	"trpc.group/trpc-go/trpc-a2a-go/taskmanager"
)

// Implement the MessageProcessor interface
type myMessageProcessor struct {
	// Add your custom fields here
}

func (p *myMessageProcessor) ProcessMessage(
	ctx context.Context,
	message protocol.Message,
	options taskmanager.ProcessOptions,
	handle taskmanager.TaskHandler,
) (*taskmanager.MessageProcessingResult, error) {
	// Extract text from the incoming message
	text := extractTextFromMessage(message)

	// Process the text (example: reverse it)
	result := reverseString(text)

	// Return a simple response message
	responseMessage := protocol.NewMessage(
		protocol.MessageRoleAgent,
		[]protocol.Part{protocol.NewTextPart("Processed: " + result)},
	)

	return &taskmanager.MessageProcessingResult{
		Result: &responseMessage,
	}, nil
}

func extractTextFromMessage(message protocol.Message) string {
	for _, part := range message.Parts {
		if textPart, ok := part.(*protocol.TextPart); ok {
			return textPart.Text
		}
	}
	return ""
}

func reverseString(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}

// Helper function to create bool pointers
func boolPtr(b bool) *bool {
	return &b
}

func main() {
	// Create the agent card
	agentCard := server.AgentCard{
		Name:        "My Agent",
		Description: "Agent description",
		URL:         "http://localhost:8080/",
		Version:     "1.0.0",
		Provider: &server.AgentProvider{
			Organization: "Provider name",
		},
		Capabilities: server.AgentCapabilities{
			Streaming: boolPtr(true),
		},
		DefaultInputModes:  []string{protocol.KindText},
		DefaultOutputModes: []string{protocol.KindText},
		Skills: []server.AgentSkill{
			{
				ID:          "text_processing",
				Name:        "Text Processing",
				Description: stringPtr("Process and transform text input"),
				InputModes:  []string{protocol.KindText},
				OutputModes: []string{protocol.KindText},
			},
		},
	}

	// Create the task processor
	processor := &myMessageProcessor{}

	// Create task manager, inject processor
	taskManager, err := taskmanager.NewMemoryTaskManager(processor)
	if err != nil {
		log.Fatalf("Failed to create task manager: %v", err)
	}

	// Create the server
	srv, err := server.NewA2AServer(agentCard, taskManager)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Start the server
	log.Printf("Agent server started on :8080")
	if err := srv.Start(":8080"); err != nil {
		log.Fatalf("Server start failed: %v", err)
	}
}
