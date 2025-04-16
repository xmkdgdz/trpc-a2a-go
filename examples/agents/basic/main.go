// Tencent is pleased to support the open source community by making tRPC available.
//
// Copyright (C) 2025 THL A29 Limited, a Tencent company.
// All rights reserved.
//
// If you have downloaded a copy of the tRPC source code from Tencent,
// please note that tRPC source code is licensed under the  Apache 2.0 License,
// A copy of the Apache 2.0 License is included in this file.

// Package main implements a basic A2A agent example.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	// Import necessary packages from pkg/
	"trpc.group/trpc-go/a2a-go/server"
	"trpc.group/trpc-go/a2a-go/taskmanager"
)

// basicTaskProcessor implements the TaskProcessor interface for the text reversal agent.
type basicTaskProcessor struct {
	// No fields needed for this simple example
}

// Process implements the core logic for the text reversal agent.
// It extracts text from the message, reverses it, and uses the TaskHandle
// to update the task state and add the result artifact.
func (p *basicTaskProcessor) Process(
	ctx context.Context,
	taskID string,
	message taskmanager.Message,
	handle taskmanager.TaskHandle,
) error {
	log.Printf("Processing task %s...", taskID)

	// Extract text from the incoming message.
	text := extractText(message)
	if text == "" {
		errMsg := "input message must contain text"
		log.Printf("Task %s failed: %s", taskID, errMsg)
		// Update status to Failed via handle.
		failedMessage := taskmanager.NewMessage(
			taskmanager.MessageRoleAgent,
			[]taskmanager.Part{taskmanager.NewTextPart(errMsg)}, // Use Part slice
		)
		_ = handle.UpdateStatus(taskmanager.TaskStateFailed, &failedMessage) // Ignore error on final fail update
		return fmt.Errorf(errMsg)                                            // Return the processing error
	}

	// Check for cancellation via context
	if err := ctx.Err(); err != nil {
		log.Printf("Task %s cancelled during processing: %v", taskID, err)
		// Update status to Cancelled via handle.
		_ = handle.UpdateStatus(taskmanager.TaskStateCanceled, nil)
		return err // Return context error
	}

	// Process the text (example: reverse it).
	result := processText(text)
	log.Printf("Task %s processing complete. Result: %s", taskID, result)

	// Create the response message part.
	responseTextPart := taskmanager.NewTextPart(result)

	// Create the final completed task status including the response message.
	finalStatusMsg := taskmanager.NewMessage(
		taskmanager.MessageRoleAgent,
		[]taskmanager.Part{responseTextPart}, // Use Part slice
	)
	finalStatusState := taskmanager.TaskStateCompleted

	// Create the result artifact.
	artifact := taskmanager.Artifact{
		Name:        stringPtr("Processed Text"),
		Description: stringPtr("The result of processing the input text."),
		Index:       0,
		Parts:       []taskmanager.Part{responseTextPart}, // Use Part slice
		LastChunk:   boolPtr(true),                        // Indicate this is the only/last chunk for this artifact.
	}

	// Update the task state to completed via handle.
	if err := handle.UpdateStatus(finalStatusState, &finalStatusMsg); err != nil {
		log.Printf("Error updating final status for task %s: %v", taskID, err)
		// If final status update fails, return the error.
		// Task state might be inconsistent.
		return fmt.Errorf("failed to update final task status: %w", err)
	}

	// Add the artifact via handle.
	if err := handle.AddArtifact(artifact); err != nil {
		log.Printf("Error adding artifact for task %s: %v", taskID, err)
		// Even if adding artifact fails, the task is logically complete.
		// Log the error but don't return it as a processing failure.
	}

	log.Printf("Task %s completed successfully.", taskID)

	// Return nil error to indicate successful processing.
	return nil
}

// extractText extracts the first text part from a message.
func extractText(message taskmanager.Message) string {
	for _, part := range message.Parts {
		// Type assert to the concrete TextPart type.
		if p, ok := part.(taskmanager.TextPart); ok {
			return p.Text
		}
	}
	return ""
}

// processText is an example function to process text (reverses it).
func processText(text string) string {
	return fmt.Sprintf("Input: %s\nProcessed: %s", text, reverseString(text))
}

// reverseString reverses a UTF-8 encoded string.
func reverseString(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

func main() {
	// Command-line flags for server configuration.
	var (
		host        string
		port        int
		description string
		noCORS      bool
	)

	flag.StringVar(&host, "host", "localhost", "Server host address")
	flag.IntVar(&port, "port", 8080, "Server port")
	flag.StringVar(&description, "desc", "A simple A2A example agent that reverses text", "Agent description")
	flag.BoolVar(&noCORS, "no-cors", false, "Disable CORS headers")
	flag.Parse()

	address := fmt.Sprintf("%s:%d", host, port)
	// Assuming HTTP for simplicity, HTTPS is recommended for production.
	serverURL := fmt.Sprintf("http://%s/", address)

	// Create the agent card using types from the server package.
	agentCard := server.AgentCard{
		Name:        "Basic Text Reverser Agent",
		Description: &description,
		URL:         serverURL,
		Version:     "1.1.0", // Incremented version
		Provider: &server.AgentProvider{
			Name: "A2A-Go Examples",
		},
		Capabilities: server.AgentCapabilities{
			Streaming:              true, // This agent supports streaming.
			StateTransitionHistory: true, // MemoryTaskManager stores history.
		},
		// Use PartTypeText constant from taskmanager package.
		DefaultInputModes:  []string{string(taskmanager.PartTypeText)},
		DefaultOutputModes: []string{string(taskmanager.PartTypeText)},
		Skills: []server.AgentSkill{
			{
				ID:          "text_reverser",
				Name:        "Text Reverser",
				Description: stringPtr("Reverses the input text."),
				Tags:        []string{"text", "reverse", "example"},
				Examples:    []string{"reverse this text"},
				InputModes:  []string{string(taskmanager.PartTypeText)},
				OutputModes: []string{string(taskmanager.PartTypeText)},
			},
		},
	}

	// Create the TaskProcessor (agent logic).
	processor := &basicTaskProcessor{}

	// Create the TaskManager, injecting the processor.
	taskManager, err := taskmanager.NewMemoryTaskManager(processor)
	if err != nil {
		log.Fatalf("Failed to create task manager: %v", err)
	}

	// Create the A2A server instance using the factory from server package.
	srv, err := server.NewA2AServer(agentCard, taskManager, server.WithCORSEnabled(!noCORS))
	if err != nil {
		log.Fatalf("Failed to create A2A server: %v", err)
	}

	// Set up signal handling for graceful shutdown.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start the server in a separate goroutine.
	go func() {
		// Use log.Printf for informational message, not Fatal.
		log.Printf("Basic Agent server starting on %s (CORS enabled: %t)", address, !noCORS)
		if err := srv.Start(address); err != nil {
			// Fatalf will exit the program if the server fails to start.
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for an interrupt or termination signal.
	<-sigChan
	log.Println("Shutdown signal received, initiating graceful shutdown...")

	// Create a context with a timeout for graceful shutdown.
	// Allow 10 seconds for existing requests to finish.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Attempt to stop the server gracefully.
	if err := srv.Stop(ctx); err != nil {
		log.Fatalf("Server shutdown failed: %v", err)
	}

	log.Println("Server exited gracefully.")
}

// stringPtr is a helper function to get a pointer to a string.
func stringPtr(s string) *string {
	return &s
}

// boolPtr is a helper function to get a pointer to a boolean.
func boolPtr(b bool) *bool {
	return &b
}
