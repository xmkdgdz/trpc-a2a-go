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
	"strings"
	"syscall"
	"time"

	"trpc.group/trpc-go/a2a-go/protocol"
	"trpc.group/trpc-go/a2a-go/server"
	"trpc.group/trpc-go/a2a-go/taskmanager"
)

// Interaction mode constants.
const (
	modeReverse      = "reverse"
	modeUppercase    = "uppercase"
	modeLowercase    = "lowercase"
	modeWordCount    = "count"
	modeMultiStep    = "multi" // New multi-step mode
	modeHelp         = "help"
	modeInputExample = "example" // For demonstrating input-required state
)

// basicTaskProcessor implements the TaskProcessor interface for the text processing agent.
type basicTaskProcessor struct {
	// Map to track multi-turn conversations
	multiTurnSessions map[string]multiTurnSession

	// Flag to determine if we should use streaming mode
	useStreaming bool
}

// multiTurnSession tracks state for a multi-turn interaction.
type multiTurnSession struct {
	stage    int
	text     string
	mode     string
	complete bool
}

// Process implements the core logic for the text processing agent.
// It extracts text from the message, processes it, and uses the TaskHandle
// to update the task state and add the result artifact.
func (p *basicTaskProcessor) Process(
	ctx context.Context,
	taskID string,
	message protocol.Message,
	handle taskmanager.TaskHandle,
) error {
	log.Printf("Processing task %s...", taskID)

	// Initialize multi-turn sessions map if not already initialized
	if p.multiTurnSessions == nil {
		p.multiTurnSessions = make(map[string]multiTurnSession)
	}

	// Extract text from the incoming message
	text := extractText(message)
	if text == "" {
		errMsg := "input message must contain text"
		log.Printf("Task %s failed: %s", taskID, errMsg)
		// Update status to Failed via handle
		failedMessage := protocol.NewMessage(
			protocol.MessageRoleAgent,
			[]protocol.Part{protocol.NewTextPart(errMsg)},
		)
		_ = handle.UpdateStatus(protocol.TaskStateFailed, &failedMessage)
		return fmt.Errorf(errMsg)
	}

	// Check for continuation of a multi-turn session
	session, exists := p.multiTurnSessions[taskID]

	if exists && !session.complete {
		return p.handleMultiTurnSession(ctx, taskID, text, handle, session)
	}

	// New interaction - determine mode and process accordingly
	return p.handleNewInteraction(ctx, taskID, text, handle)
}

// handleMultiTurnSession processes the next step in a multi-turn interaction.
func (p *basicTaskProcessor) handleMultiTurnSession(
	ctx context.Context,
	taskID string,
	text string,
	handle taskmanager.TaskHandle,
	session multiTurnSession,
) error {
	// Update session with new input
	switch session.stage {
	case 1:
		// First response received - this is the mode
		session.mode = strings.ToLower(strings.TrimSpace(text))
		session.stage = 2

		// Ask for the text to process
		msg := protocol.NewMessage(
			protocol.MessageRoleAgent,
			[]protocol.Part{protocol.NewTextPart("Please enter the text you want to process:")},
		)

		if err := handle.UpdateStatus(protocol.TaskStateInputRequired, &msg); err != nil {
			return fmt.Errorf("failed to update task status: %w", err)
		}

		// Store updated session
		p.multiTurnSessions[taskID] = session
		return nil

	case 2:
		// Second response received - this is the text to process
		session.text = text
		session.stage = 3
		session.complete = true

		// Process the text based on the selected mode
		result := p.processTextWithMode(session.text, session.mode)

		// Create the completed message and artifact
		finalMsg := protocol.NewMessage(
			protocol.MessageRoleAgent,
			[]protocol.Part{protocol.NewTextPart(result)},
		)

		// Check if this is a streaming request
		isStreaming := handle.IsStreamingRequest()
		if !isStreaming && p.useStreaming {
			// Fall back to the flag for backward compatibility
			isStreaming = true
		}

		// Use streaming if enabled
		if isStreaming {
			// Send intermediate status update
			inProgressMsg := protocol.NewMessage(
				protocol.MessageRoleAgent,
				[]protocol.Part{protocol.NewTextPart("Processing your request...")},
			)
			if err := handle.UpdateStatus(protocol.TaskStateWorking, &inProgressMsg); err != nil {
				log.Printf("Error sending intermediate status: %v", err)
			}

			// Simulate processing delay
			time.Sleep(500 * time.Millisecond)
		}

		// Update task status to completed
		if err := handle.UpdateStatus(protocol.TaskStateCompleted, &finalMsg); err != nil {
			return fmt.Errorf("failed to update final task status: %w", err)
		}

		// Add the artifact
		artifact := protocol.Artifact{
			Name:        stringPtr("Processed Text"),
			Description: stringPtr(fmt.Sprintf("Text processed with mode: %s", session.mode)),
			Index:       0,
			Parts:       []protocol.Part{protocol.NewTextPart(result)},
			LastChunk:   boolPtr(true),
		}

		if err := handle.AddArtifact(artifact); err != nil {
			log.Printf("Error adding artifact for task %s: %v", taskID, err)
		}

		// Update session in map
		p.multiTurnSessions[taskID] = session
		return nil
	}

	return fmt.Errorf("unexpected stage in multi-turn session: %d", session.stage)
}

// handleNewInteraction processes a new interaction.
func (p *basicTaskProcessor) handleNewInteraction(
	ctx context.Context,
	taskID string,
	text string,
	handle taskmanager.TaskHandle,
) error {
	// Check for cancellation via context
	if err := ctx.Err(); err != nil {
		log.Printf("Task %s cancelled during processing: %v", taskID, err)
		_ = handle.UpdateStatus(protocol.TaskStateCanceled, nil)
		return err
	}

	// Parse the first word as the command
	parts := strings.SplitN(text, " ", 2)
	command := strings.ToLower(parts[0])

	// Handle multi-step mode
	if command == modeMultiStep {
		session := multiTurnSession{
			stage:    1,
			complete: false,
		}

		// Store the session
		p.multiTurnSessions[taskID] = session

		// Ask for the processing mode
		msg := protocol.NewMessage(
			protocol.MessageRoleAgent,
			[]protocol.Part{protocol.NewTextPart(
				"This is a multi-step interaction. Please select a processing mode:\n" +
					"- reverse: Reverses the text\n" +
					"- uppercase: Converts text to uppercase\n" +
					"- lowercase: Converts text to lowercase\n" +
					"- count: Counts words and characters")},
		)

		if err := handle.UpdateStatus(protocol.TaskStateInputRequired, &msg); err != nil {
			return fmt.Errorf("failed to update task status: %w", err)
		}

		return nil
	}

	// Handle example input-required state
	if command == modeInputExample {
		msg := protocol.NewMessage(
			protocol.MessageRoleAgent,
			[]protocol.Part{protocol.NewTextPart("Please provide more information to continue:")},
		)

		if err := handle.UpdateStatus(protocol.TaskStateInputRequired, &msg); err != nil {
			return fmt.Errorf("failed to update task status: %w", err)
		}

		// Create a session for the follow-up
		session := multiTurnSession{
			stage:    2,           // Skip to stage 2 (text input)
			mode:     modeReverse, // Default to reverse mode
			complete: false,
		}
		p.multiTurnSessions[taskID] = session
		return nil
	}

	// For direct processing (non-multi-turn), extract the rest as content
	var content string
	if len(parts) > 1 {
		content = parts[1]
	} else {
		content = "" // No content provided, command only
	}

	// Check if request is streaming using the new interface method
	// Fall back to the flag if needed for backward compatibility
	isStreaming := handle.IsStreamingRequest()
	if !isStreaming && p.useStreaming {
		// If the request itself isn't streaming but the processor is configured to use streaming,
		// use streaming mode anyway (for backward compatibility)
		isStreaming = true
	}

	// Process in streaming or non-streaming mode based on the request type
	if isStreaming {
		return p.processWithStreaming(ctx, taskID, command, content, handle)
	} else {
		return p.processDirectly(ctx, taskID, command, content, handle)
	}
}

// processWithStreaming handles the processing with intermediate updates.
func (p *basicTaskProcessor) processWithStreaming(
	ctx context.Context,
	taskID string,
	command string,
	content string,
	handle taskmanager.TaskHandle,
) error {
	// Send initial "working" status
	workingMsg := protocol.NewMessage(
		protocol.MessageRoleAgent,
		[]protocol.Part{protocol.NewTextPart("Processing your request...")},
	)

	if err := handle.UpdateStatus(protocol.TaskStateWorking, &workingMsg); err != nil {
		return fmt.Errorf("failed to update working status: %w", err)
	}

	// Simulate processing delay
	time.Sleep(500 * time.Millisecond)

	// Process the content based on command
	result := p.processTextWithMode(content, command)

	// Add artifact in chunks to demonstrate streaming
	// Split at a complete line to avoid breaking words/sentences
	splitIndex := findSplitIndex(result)

	// First chunk
	artifact1 := protocol.Artifact{
		Name:        stringPtr("Processed Text (Part 1)"),
		Description: stringPtr("First part of the processed text"),
		Index:       0,
		Parts:       []protocol.Part{protocol.NewTextPart(result[:splitIndex])},
		LastChunk:   boolPtr(false),
	}

	if err := handle.AddArtifact(artifact1); err != nil {
		log.Printf("Error adding first artifact chunk for task %s: %v", taskID, err)
	}

	// Small delay to demonstrate streaming
	time.Sleep(300 * time.Millisecond)

	// Second chunk (appends to first)
	artifact2 := protocol.Artifact{
		Name:        stringPtr("Processed Text (Complete)"),
		Description: stringPtr("Complete processed text"),
		Index:       0, // Same index as first chunk
		Append:      boolPtr(true),
		Parts:       []protocol.Part{protocol.NewTextPart(result[splitIndex:])},
		LastChunk:   boolPtr(true),
	}

	if err := handle.AddArtifact(artifact2); err != nil {
		log.Printf("Error adding second artifact chunk for task %s: %v", taskID, err)
	}

	// Create final message
	finalMsg := protocol.NewMessage(
		protocol.MessageRoleAgent,
		[]protocol.Part{protocol.NewTextPart(result)},
	)
	// Update task to completed
	if err := handle.UpdateStatus(protocol.TaskStateCompleted, &finalMsg); err != nil {
		return fmt.Errorf("failed to update final status: %w", err)
	}
	return nil
}

// findSplitIndex finds a good place to split text without breaking words or lines
func findSplitIndex(text string) int {
	// If text is short, don't split
	if len(text) < 20 {
		return len(text)
	}

	// Start at halfway
	splitIndex := len(text) / 2

	// Try to find a newline near the middle
	for i := splitIndex; i < len(text); i++ {
		if text[i] == '\n' {
			return i + 1 // Include the newline in the first chunk
		}
	}

	// If no newline, try to find space near the middle
	for i := splitIndex; i < len(text); i++ {
		if text[i] == ' ' {
			return i + 1 // Include the space in the first chunk
		}
	}

	// If no good natural boundary found, just use halfway point
	return splitIndex
}

// processDirectly handles the processing without intermediate updates.
func (p *basicTaskProcessor) processDirectly(
	ctx context.Context,
	taskID string,
	command string,
	content string,
	handle taskmanager.TaskHandle,
) error {
	// Process the content based on command
	result := p.processTextWithMode(content, command)

	// Create the response message
	finalMsg := protocol.NewMessage(
		protocol.MessageRoleAgent,
		[]protocol.Part{protocol.NewTextPart(result)},
	)

	// Update task to completed
	if err := handle.UpdateStatus(protocol.TaskStateCompleted, &finalMsg); err != nil {
		return fmt.Errorf("failed to update final status: %w", err)
	}

	// Add artifact
	artifact := protocol.Artifact{
		Name:        stringPtr("Processed Text"),
		Description: stringPtr(fmt.Sprintf("Text processed with mode: %s", command)),
		Index:       0,
		Parts:       []protocol.Part{protocol.NewTextPart(result)},
		LastChunk:   boolPtr(true),
	}

	if err := handle.AddArtifact(artifact); err != nil {
		log.Printf("Error adding artifact for task %s: %v", taskID, err)
	}

	return nil
}

// processTextWithMode processes text with the specified mode.
func (p *basicTaskProcessor) processTextWithMode(text, mode string) string {
	switch strings.ToLower(mode) {
	case modeReverse:
		return fmt.Sprintf("Input: %s\nReversed: %s", text, reverseString(text))
	case modeUppercase:
		return fmt.Sprintf("Input: %s\nUppercase: %s", text, strings.ToUpper(text))
	case modeLowercase:
		return fmt.Sprintf("Input: %s\nLowercase: %s", text, strings.ToLower(text))
	case modeWordCount:
		words := len(strings.Fields(text))
		chars := len(text)
		return fmt.Sprintf("Input: %s\nWord count: %d\nCharacter count: %d", text, words, chars)
	case modeHelp:
		return "Available commands (type one of these):\n" +
			"- reverse <text>: Reverses the input text\n   Example: reverse hello world\n\n" +
			"- uppercase <text>: Converts text to uppercase\n   Example: uppercase hello world\n\n" +
			"- lowercase <text>: Converts text to lowercase\n   Example: lowercase HELLO WORLD\n\n" +
			"- count <text>: Counts words and characters\n   Example: count this is a test\n\n" +
			"- multi: Start a multi-step interaction\n\n" +
			"- example: Demonstrates input-required state\n\n" +
			"- help: Shows this help message"
	default:
		// Default to help if command not recognized
		if text == "" {
			return "Please provide a command. Type 'help' for available commands.\n\nExample commands:\n" +
				"- reverse hello world\n" +
				"- uppercase hello\n" +
				"- count these words"
		}
		return fmt.Sprintf("Unknown command '%s'. Type 'help' for available commands.\n\nAssuming 'reverse' mode:\nReversed: %s",
			mode, reverseString(text))
	}
}

// extractText extracts the first text part from a message.
func extractText(message protocol.Message) string {
	for _, part := range message.Parts {
		// Type assert to the concrete TextPart type.
		if p, ok := part.(protocol.TextPart); ok {
			return p.Text
		}
	}
	return ""
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
		host          string
		port          int
		description   string
		noCORS        bool
		forceNoStream bool // New flag to disable streaming
	)

	flag.StringVar(&host, "host", "localhost", "Server host address")
	flag.IntVar(&port, "port", 8080, "Server port")
	flag.StringVar(&description, "desc", "A versatile A2A example agent that processes text", "Agent description")
	flag.BoolVar(&noCORS, "no-cors", false, "Disable CORS headers")
	flag.BoolVar(&forceNoStream, "no-stream", false, "Disable streaming capability")
	flag.Parse()

	address := fmt.Sprintf("%s:%d", host, port)
	// Assuming HTTP for simplicity, HTTPS is recommended for production.
	serverURL := fmt.Sprintf("http://%s/", address)

	// Create the agent card using types from the server package.
	agentCard := server.AgentCard{
		Name:        "Text Processing Agent",
		Description: &description,
		URL:         serverURL,
		Version:     "2.0.0", // Updated version
		Provider: &server.AgentProvider{
			Name: "A2A-Go Examples",
		},
		Capabilities: server.AgentCapabilities{
			Streaming:              !forceNoStream, // Support streaming based on flag
			StateTransitionHistory: true,           // MemoryTaskManager stores history
		},
		// Support text input/output
		DefaultInputModes:  []string{string(protocol.PartTypeText)},
		DefaultOutputModes: []string{string(protocol.PartTypeText)},
		Skills: []server.AgentSkill{
			{
				ID:          "text_processor_reverse",
				Name:        "Text Reverser",
				Description: stringPtr("Input: reverse hello\nOutput: Reversed: olleh"),
				Tags:        []string{"text", "reverse"},
				Examples:    []string{"reverse hello world", "reverse The quick brown fox"},
				InputModes:  []string{string(protocol.PartTypeText)},
				OutputModes: []string{string(protocol.PartTypeText)},
			},
			{
				ID:          "text_processor_uppercase",
				Name:        "Uppercase Converter",
				Description: stringPtr("Input: uppercase hello world\nOutput: HELLO WORLD"),
				Tags:        []string{"text", "uppercase"},
				Examples:    []string{"uppercase hello world", "uppercase Example text"},
				InputModes:  []string{string(protocol.PartTypeText)},
				OutputModes: []string{string(protocol.PartTypeText)},
			},
			{
				ID:          "text_processor_lowercase",
				Name:        "Lowercase Converter",
				Description: stringPtr("Input: lowercase HELLO\nOutput: hello"),
				Tags:        []string{"text", "lowercase"},
				Examples:    []string{"lowercase HELLO WORLD", "lowercase TEXT"},
				InputModes:  []string{string(protocol.PartTypeText)},
				OutputModes: []string{string(protocol.PartTypeText)},
			},
			{
				ID:          "text_processor_count",
				Name:        "Word Counter",
				Description: stringPtr("Input: count hello world\nOutput: Word count: 2, Character count: 11"),
				Tags:        []string{"text", "count"},
				Examples:    []string{"count The quick brown fox", "count hello world"},
				InputModes:  []string{string(protocol.PartTypeText)},
				OutputModes: []string{string(protocol.PartTypeText)},
			},
			{
				ID:          "text_processor_multi",
				Name:        "Multi-step Process",
				Description: stringPtr("Input: multi\nOutput: Interactive conversation requesting processing mode then text"),
				Tags:        []string{"text", "interactive"},
				Examples:    []string{"multi"},
				InputModes:  []string{string(protocol.PartTypeText)},
				OutputModes: []string{string(protocol.PartTypeText)},
			},
			{
				ID:          "text_processor_help",
				Name:        "Help Guide",
				Description: stringPtr("Input: help\nOutput: List of available commands and usage"),
				Tags:        []string{"help"},
				Examples:    []string{"help"},
				InputModes:  []string{string(protocol.PartTypeText)},
				OutputModes: []string{string(protocol.PartTypeText)},
			},
		},
	}

	// Create the TaskProcessor (agent logic).
	processor := &basicTaskProcessor{
		multiTurnSessions: make(map[string]multiTurnSession),
		useStreaming:      !forceNoStream,
	}

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
		log.Printf("Text Processing Agent server starting on %s (CORS enabled: %t, Streaming: %t)",
			address, !noCORS, !forceNoStream)
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
