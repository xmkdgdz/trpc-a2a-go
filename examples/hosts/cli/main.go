// Tencent is pleased to support the open source community by making tRPC available.
//
// Copyright (C) 2025 THL A29 Limited, a Tencent company.
// All rights reserved.
//
// If you have downloaded a copy of the tRPC source code from Tencent,
// please note that tRPC source code is licensed under the  Apache 2.0 License,
// A copy of the Apache 2.0 License is included in this file.

// Package main implements a CLI host for the A2A agent.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"trpc.group/trpc-go/a2a-go/client"
	"trpc.group/trpc-go/a2a-go/protocol"
)

// Config holds the application configuration.
type Config struct {
	AgentURL string
	Timeout  time.Duration
}

func main() {
	// Parse command-line flags.
	config := parseFlags()

	// Create A2A client.
	a2aClient, err := createClient(config)
	if err != nil {
		log.Fatalf("Failed to create A2A client: %v", err)
	}

	// Display welcome message.
	displayWelcomeMessage(config)

	// Start interactive session.
	runInteractiveSession(a2aClient, config)

	fmt.Println("Exiting CLI host.")
}

// parseFlags parses command-line flags and returns a Config.
func parseFlags() Config {
	var config Config
	flag.StringVar(&config.AgentURL, "agent", "http://localhost:8080/", "Target A2A agent URL")
	flag.DurationVar(&config.Timeout, "timeout", 60*time.Second, "Request timeout (e.g., 30s, 1m)")
	flag.Parse()
	return config
}

// createClient creates a new A2A client with the given configuration.
func createClient(config Config) (*client.A2AClient, error) {
	return client.NewA2AClient(config.AgentURL, client.WithTimeout(config.Timeout))
}

// displayWelcomeMessage prints the welcome message with connection details.
func displayWelcomeMessage(config Config) {
	log.Printf("Connecting to agent: %s (Timeout: %v)", config.AgentURL, config.Timeout)
	fmt.Println("Enter text to send to the agent (uses streaming). Type 'exit' or press Ctrl+D to quit.")
	fmt.Println(strings.Repeat("-", 60))
}

// runInteractiveSession runs the main interactive session loop.
func runInteractiveSession(a2aClient *client.A2AClient, config Config) {
	reader := bufio.NewReader(os.Stdin)
	var sessionID string

	for {
		fmt.Print("> ")
		input, readErr := reader.ReadString('\n')

		if readErr != nil {
			if readErr == io.EOF {
				fmt.Println("\nExiting.")
				break
			}
			log.Printf("ERROR: Failed to read input: %v", readErr)
			continue
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		if strings.ToLower(input) == "exit" {
			break
		}

		// Process the user input and handle the agent interaction.
		sessionID = processUserInput(a2aClient, input, sessionID, config)
	}
}

// processUserInput handles a single user input, sends it to the agent, and processes the response.
func processUserInput(a2aClient *client.A2AClient, input, sessionID string, config Config) string {
	// Generate or use existing session ID.
	if sessionID == "" {
		sessionID = generateSessionID()
		log.Printf("Started new session: %s", sessionID)
	}

	// Generate unique task ID.
	taskID := generateTaskID()

	// Create message and parameters.
	params := createTaskParams(taskID, sessionID, input)

	// Send the request and process the response.
	handleAgentInteraction(a2aClient, params, taskID, config)

	return sessionID
}

// generateSessionID creates a new unique session ID.
func generateSessionID() string {
	return fmt.Sprintf("cli-session-%d-%s", time.Now().Unix(), uuid.New().String())
}

// generateTaskID creates a new unique task ID.
func generateTaskID() string {
	return fmt.Sprintf("cli-task-%d-%s", time.Now().UnixNano(), uuid.New().String())
}

// createTaskParams creates the parameters for sending a task.
func createTaskParams(taskID, sessionID, input string) protocol.SendTaskParams {
	message := protocol.NewMessage(
		protocol.MessageRoleUser,
		[]protocol.Part{protocol.NewTextPart(input)},
	)

	return protocol.SendTaskParams{
		ID:        taskID,
		SessionID: &sessionID,
		Message:   message,
	}
}

// handleAgentInteraction sends a request to the agent and processes the response.
func handleAgentInteraction(
	a2aClient *client.A2AClient,
	params protocol.SendTaskParams,
	taskID string,
	config Config,
) {
	// Create context for the stream.
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout*2)
	defer cancel()

	log.Printf("Sending stream request for task %s (Session: %s)...", taskID, *params.SessionID)
	eventChan, streamErr := a2aClient.StreamTask(ctx, params)

	if streamErr != nil {
		log.Printf("ERROR: StreamTask request failed: %v", streamErr)
		fmt.Println(strings.Repeat("-", 60))
		return
	}

	// Process the stream response.
	finalTaskState, finalArtifacts := processStreamResponse(ctx, eventChan)

	// Get the final task state.
	getFinalTaskState(a2aClient, taskID, config.Timeout, finalTaskState, finalArtifacts)

	log.Printf("Stream processing finished for task %s.", taskID)
	fmt.Println(strings.Repeat("-", 60))
}

// processStreamResponse processes the stream of events from the agent.
func processStreamResponse(
	ctx context.Context, eventChan <-chan protocol.TaskEvent,
) (protocol.TaskState, []protocol.Artifact) {
	fmt.Println("\n<< Agent Response Stream:")
	fmt.Println(strings.Repeat("-", 10))

	var finalTaskState protocol.TaskState
	finalArtifacts := []protocol.Artifact{}
	streamEnded := false

streamLoop:
	for !streamEnded {
		select {
		case event, ok := <-eventChan:
			if !ok {
				streamEnded = true
				log.Println("Stream channel closed.")
				break streamLoop
			}

			// Process the received event.
			finalTaskState, finalArtifacts = processEvent(event, finalArtifacts)

		case <-ctx.Done():
			log.Printf("ERROR: Context timeout or cancellation while waiting for stream events: %v", ctx.Err())
			streamEnded = true
			break streamLoop
		}
	}

	return finalTaskState, finalArtifacts
}

// processEvent processes a single event from the stream.
func processEvent(
	event protocol.TaskEvent, finalArtifacts []protocol.Artifact,
) (protocol.TaskState, []protocol.Artifact) {
	var finalTaskState protocol.TaskState

	switch e := event.(type) {
	case protocol.TaskStatusUpdateEvent:
		fmt.Printf("  [Status Update: %s (%s)]\n", e.Status.State, formatTimestamp(e.Status.Timestamp))
		if e.Status.Message != nil {
			printMessage(*e.Status.Message)
		}
		if e.IsFinal() {
			finalTaskState = e.Status.State
		}
	case protocol.TaskArtifactUpdateEvent:
		name := fmt.Sprintf("Artifact #%d", len(finalArtifacts)+1)
		if e.Artifact.Name != nil {
			name = *e.Artifact.Name
		}
		fmt.Printf("  [Artifact Update: %s]\n", name)
		printParts(e.Artifact.Parts)
		finalArtifacts = append(finalArtifacts, e.Artifact)
	default:
		log.Printf("Warning: Received unknown event type: %T\n", event)
	}

	return finalTaskState, finalArtifacts
}

// getFinalTaskState fetches and displays the final task state.
func getFinalTaskState(
	a2aClient *client.A2AClient,
	taskID string,
	timeout time.Duration,
	streamState protocol.TaskState,
	streamArtifacts []protocol.Artifact,
) {
	finalCtx, finalCancel := context.WithTimeout(context.Background(), timeout)
	defer finalCancel()

	finalTask, getErr := a2aClient.GetTasks(finalCtx, protocol.TaskQueryParams{ID: taskID})

	fmt.Println(strings.Repeat("-", 10))
	fmt.Println("<< Final Result (from GetTask):")

	if getErr != nil {
		log.Printf("ERROR: Failed to get final task state for %s: %v", taskID, getErr)
		fmt.Printf("  State: %s (from stream)\n", streamState)
		return
	}

	if finalTask == nil {
		log.Printf("WARNING: TasksGet for %s returned nil task without error.", taskID)
		fmt.Printf("  State: %s (from stream)\n", streamState)
		return
	}

	displayFinalTaskState(finalTask)
}

// displayFinalTaskState displays the final state of a task.
func displayFinalTaskState(task *protocol.Task) {
	fmt.Printf("  State: %s (%s)\n", task.Status.State, formatTimestamp(task.Status.Timestamp))

	if task.Status.Message != nil {
		fmt.Println("  Message:")
		printMessage(*task.Status.Message)
	}

	if len(task.Artifacts) > 0 {
		fmt.Println("  Artifacts:")
		for i, artifact := range task.Artifacts {
			name := fmt.Sprintf("Artifact #%d", i+1)
			if artifact.Name != nil {
				name = *artifact.Name
			}
			fmt.Printf("    [%s]\n", name)
			printParts(artifact.Parts)
		}
	}
}

// printMessage prints the parts contained within a message.
func printMessage(message protocol.Message) {
	printParts(message.Parts)
}

// printParts iterates through and prints different message/artifact part types.
func printParts(parts []protocol.Part) {
	for _, part := range parts {
		printPart(part)
	}
}

// printPart handles the printing logic based on the concrete part type.
// It includes fallbacks for map[string]interface{} representations.
func printPart(part interface{}) {
	indent := "  " // Indentation for nested content.

	// Handle direct types from taskmanager first (preferred).
	switch p := part.(type) {
	case protocol.TextPart:
		fmt.Println(indent + p.Text)
	case protocol.FilePart:
		printFilePart(p, indent)
	case protocol.DataPart:
		printDataPart(p, indent)
	case map[string]interface{}:
		printMapPart(p, indent)
	default:
		fmt.Printf("%s[Unknown Part Type: %T]\n", indent, p)
	}
}

// printFilePart prints a file part.
func printFilePart(p protocol.FilePart, indent string) {
	name := "(unnamed file)"
	if p.File.Name != nil {
		name = *p.File.Name
	}
	mime := "(unknown type)"
	if p.File.MimeType != nil {
		mime = *p.File.MimeType
	}
	fmt.Printf("%s[File: %s (%s)]\n", indent, name, mime)
	if p.File.URI != nil {
		fmt.Printf("%s  URI: %s\n", indent, *p.File.URI)
	}
	if p.File.Bytes != nil {
		fmt.Printf("%s  Bytes: %d bytes\n", indent, len(*p.File.Bytes))
	}
}

// printDataPart prints a data part.
func printDataPart(p protocol.DataPart, indent string) {
	fmt.Printf("%s[Structured Data]\n", indent)
	dataContent, err := json.MarshalIndent(p.Data, indent, "  ")
	if err == nil {
		fmt.Printf("%s%s\n", indent, string(dataContent))
	} else {
		fmt.Printf("%s  Error marshaling data: %v\n", indent, err)
		fmt.Printf("%s  Raw: %+v\n", indent, p.Data)
	}
}

// printMapPart prints a part represented as a map.
func printMapPart(p map[string]interface{}, indent string) {
	if typeStr, ok := p["type"].(string); ok {
		switch typeStr {
		case string(protocol.PartTypeText):
			if text, ok := p["text"].(string); ok {
				fmt.Println(indent + text)
			}
		case string(protocol.PartTypeFile):
			fmt.Printf("%s[File (from map)]\n", indent)
			fileData, err := json.MarshalIndent(p["file"], indent, "  ")
			if err == nil {
				fmt.Printf("%s%s\n", indent, string(fileData))
			} else if p["file"] != nil {
				fmt.Printf("%s  %+v\n", indent, p["file"])
			}
		case string(protocol.PartTypeData):
			fmt.Printf("%s[Structured Data (from map)]\n", indent)
			dataContent, err := json.MarshalIndent(p["data"], indent, "  ")
			if err == nil {
				fmt.Printf("%s%s\n", indent, string(dataContent))
			} else if p["data"] != nil {
				fmt.Printf("%s  %+v\n", indent, p["data"])
			}
		default:
			fmt.Printf("%s[Unknown map part type: %s]\n", indent, typeStr)
		}
	} else {
		mapData, _ := json.MarshalIndent(p, indent, "  ")
		fmt.Printf("%s[Unknown map structure]:\n%s%s\n", indent, indent, string(mapData))
	}
}

// formatTimestamp attempts to parse and reformat an ISO8601 timestamp.
func formatTimestamp(ts string) string {
	if ts == "" {
		return "(no timestamp)"
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		log.Printf("Warning: could not parse timestamp '%s': %v", ts, err)
		return ts
	}
	return t.Local().Format(time.Stamp)
}
