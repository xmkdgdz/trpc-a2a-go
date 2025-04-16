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
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"trpc.group/trpc-go/a2a-go/client"
	"trpc.group/trpc-go/a2a-go/protocol"
	"trpc.group/trpc-go/a2a-go/server"
)

// Config holds the application configuration.
type Config struct {
	AgentURL         string
	Timeout          time.Duration
	ForceNoStreaming bool
	SessionID        string
	UseTasksGet      bool
	HistoryLength    int
}

// Command types for CLI
const (
	cmdExit    = "exit"
	cmdHelp    = "help"
	cmdSession = "session"
	cmdMode    = "mode"
	cmdCancel  = "cancel"
	cmdGet     = "get"
	cmdCard    = "card"
	cmdPush    = "push"
	cmdGetPush = "getpush"
)

func main() {
	// Parse command-line flags.
	config := parseFlags()

	// Create A2A client.
	a2aClient, err := createClient(config)
	if err != nil {
		log.Fatalf("Failed to create A2A client: %v", err)
	}

	// Fetch and display agent capabilities
	agentCard, err := fetchAgentCard(config.AgentURL)
	if err != nil {
		log.Printf("WARNING: Failed to fetch agent card: %v", err)
	} else {
		displayAgentCapabilities(agentCard)
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
	flag.BoolVar(&config.ForceNoStreaming, "no-stream", false, "Disable streaming mode")
	flag.StringVar(&config.SessionID, "session", "", "Use specific session ID (empty = generate new)")
	flag.BoolVar(&config.UseTasksGet, "use-tasks-get", true, "Use tasks/get to fetch final state")
	flag.IntVar(&config.HistoryLength, "history", 0, "Number of history messages to request (0 = none)")
	flag.Parse()

	// Generate a session ID if not provided
	if config.SessionID == "" {
		config.SessionID = generateSessionID()
	}

	return config
}

// createClient creates a new A2A client with the given configuration.
func createClient(config Config) (*client.A2AClient, error) {
	return client.NewA2AClient(config.AgentURL, client.WithTimeout(config.Timeout))
}

// fetchAgentCard retrieves the agent card from the .well-known endpoint.
func fetchAgentCard(baseURL string) (*server.AgentCard, error) {
	// Ensure base URL ends with "/"
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}

	// Construct agent card URL
	cardURL := baseURL + ".well-known/agent.json"

	// Make the request
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cardURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch agent card: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Decode the response
	var card server.AgentCard
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		return nil, fmt.Errorf("failed to decode agent card: %w", err)
	}

	return &card, nil
}

// displayAgentCapabilities displays the capabilities from the agent card.
func displayAgentCapabilities(card *server.AgentCard) {
	fmt.Println("Agent Capabilities:")
	fmt.Printf("  Name: %s\n", card.Name)
	if card.Description != nil {
		fmt.Printf("  Description: %s\n", *card.Description)
	}
	fmt.Printf("  Version: %s\n", card.Version)

	// Print provider if available
	if card.Provider != nil {
		fmt.Printf("  Provider: %s\n", card.Provider.Name)
	}

	// Print capabilities
	fmt.Printf("  Streaming: %t\n", card.Capabilities.Streaming)
	fmt.Printf("  Push Notifications: %t\n", card.Capabilities.PushNotifications)
	fmt.Printf("  State Transition History: %t\n", card.Capabilities.StateTransitionHistory)

	// Print input/output modes
	fmt.Printf("  Input Modes: %s\n", strings.Join(card.DefaultInputModes, ", "))
	fmt.Printf("  Output Modes: %s\n", strings.Join(card.DefaultOutputModes, ", "))

	// Print skills if available
	if len(card.Skills) > 0 {
		fmt.Println("  Skills:")
		for _, skill := range card.Skills {
			fmt.Printf("    - %s: ", skill.Name)
			if skill.Description != nil {
				fmt.Printf("%s\n", *skill.Description)
			} else {
				fmt.Println("(no description)")
			}

			if len(skill.Examples) > 0 {
				fmt.Printf("      Examples: %s\n", strings.Join(skill.Examples, ", "))
			}
		}
	}

	fmt.Println(strings.Repeat("-", 60))
}

// displayWelcomeMessage prints the welcome message with connection details.
func displayWelcomeMessage(config Config) {
	log.Printf("Connecting to agent: %s (Timeout: %v)", config.AgentURL, config.Timeout)
	fmt.Printf("Session ID: %s\n", config.SessionID)
	fmt.Printf("Streaming mode: %v\n", !config.ForceNoStreaming)
	fmt.Println("Enter text to send to the agent. Type 'help' for commands or 'exit' to quit.")
	fmt.Println(strings.Repeat("-", 60))
}

// runInteractiveSession runs the main interactive session loop.
func runInteractiveSession(a2aClient *client.A2AClient, config Config) {
	reader := bufio.NewReader(os.Stdin)
	sessionID := config.SessionID
	var lastTaskID string
	var useStreaming = !config.ForceNoStreaming

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

		// Process built-in commands
		if cmdResult := processCommand(
			a2aClient,
			input,
			&config,
			&sessionID,
			&useStreaming,
			lastTaskID,
		); cmdResult {
			continue
		}

		// Process the user input and handle the agent interaction
		lastTaskID = processUserInput(a2aClient, input, sessionID, config, useStreaming)
	}
}

// processCommand handles built-in client commands and returns true if a command was processed.
func processCommand(
	a2aClient *client.A2AClient,
	input string,
	config *Config,
	sessionID *string,
	useStreaming *bool,
	lastTaskID string,
) bool {
	parts := strings.Fields(input)
	cmd := strings.ToLower(parts[0])

	switch cmd {
	case cmdExit:
		fmt.Println("Exiting.")
		os.Exit(0)
		return true

	case cmdHelp:
		displayHelpMessage()
		return true

	case cmdSession:
		if len(parts) > 1 {
			// Set new session ID
			*sessionID = parts[1]
			fmt.Printf("Session ID set to: %s\n", *sessionID)
		} else {
			// Generate new session ID
			*sessionID = generateSessionID()
			fmt.Printf("Generated new session ID: %s\n", *sessionID)
		}
		return true

	case cmdMode:
		if len(parts) > 1 {
			modeStr := strings.ToLower(parts[1])
			if modeStr == "stream" || modeStr == "streaming" {
				*useStreaming = true
				fmt.Println("Switched to streaming mode.")
			} else if modeStr == "sync" || modeStr == "standard" {
				*useStreaming = false
				fmt.Println("Switched to standard (non-streaming) mode.")
			} else {
				fmt.Printf("Unknown mode: %s. Use 'stream' or 'sync'.\n", modeStr)
			}
		} else {
			fmt.Printf("Current mode: %s\n", getModeName(*useStreaming))
			fmt.Println("Usage: mode [stream|sync]")
		}
		return true

	case cmdCancel:
		taskID := lastTaskID
		if len(parts) > 1 {
			taskID = parts[1]
		}

		if taskID == "" {
			fmt.Println("No task ID provided or available from last request.")
			return true
		}

		cancelTask(a2aClient, taskID, config.Timeout)
		return true

	case cmdGet:
		taskID := lastTaskID
		if len(parts) > 1 {
			taskID = parts[1]
		}

		if taskID == "" {
			fmt.Println("No task ID provided or available from last request.")
			return true
		}

		historyLength := config.HistoryLength
		if len(parts) > 2 {
			_, err := fmt.Sscanf(parts[2], "%d", &historyLength)
			if err != nil {
				fmt.Printf("Invalid history length: %s. Using default: %d\n", parts[2], config.HistoryLength)
				historyLength = config.HistoryLength
			}
		}

		getTask(a2aClient, taskID, historyLength, config.Timeout)
		return true

	case cmdCard:
		// Fetch and display agent card
		agentCard, err := fetchAgentCard(config.AgentURL)
		if err != nil {
			fmt.Printf("Failed to fetch agent card: %v\n", err)
			return true
		}

		displayAgentCapabilities(agentCard)
		return true

	case cmdPush:
		if len(parts) < 3 {
			fmt.Println("Usage: push <task-id> <callback-url> [token]")
			return true
		}

		taskID := parts[1]
		callbackURL := parts[2]
		var token *string

		if len(parts) > 3 {
			tokenStr := parts[3]
			token = &tokenStr
		}

		fmt.Println("The push notification feature is not implemented in this example.")
		fmt.Printf("Would set push notification for task %s to URL %s\n", taskID, callbackURL)
		if token != nil {
			fmt.Printf("With token: %s\n", *token)
		}
		return true

	case cmdGetPush:
		if len(parts) < 2 {
			fmt.Println("Usage: getpush <task-id>")
			return true
		}

		fmt.Println("The push notification feature is not implemented in this example.")
		fmt.Printf("Would get push notification configuration for task %s\n", parts[1])
		return true
	}

	return false
}

// getModeName returns a user-friendly name for the current mode.
func getModeName(streaming bool) string {
	if streaming {
		return "streaming (real-time updates)"
	}
	return "standard (non-streaming)"
}

// displayHelpMessage shows available commands and their usage.
func displayHelpMessage() {
	fmt.Println("Available commands:")
	fmt.Println("  help                     - Show this help message")
	fmt.Println("  exit                     - Exit the program")
	fmt.Println("  session [id]             - Set or generate a new session ID")
	fmt.Println("  mode [stream|sync]       - Set interaction mode (streaming or standard)")
	fmt.Println("  cancel [task-id]         - Cancel a task (uses last task ID if not specified)")
	fmt.Println("  get [task-id] [history]  - Get task details (uses last task ID if not specified)")
	fmt.Println("  card                     - Fetch and display the agent's capabilities card")
	fmt.Println("  push <task-id> <url> [token] - Set push notification for a task")
	fmt.Println("  getpush <task-id>        - Get push notification configuration for a task")
	fmt.Println("")
	fmt.Println("For normal interaction, just type your message and press Enter.")
	fmt.Println(strings.Repeat("-", 60))
}

// processUserInput handles a single user input, sends it to the agent, and processes the response.
func processUserInput(
	a2aClient *client.A2AClient,
	input,
	sessionID string,
	config Config,
	useStreaming bool,
) string {
	// Generate unique task ID.
	taskID := generateTaskID()

	// Create message and parameters.
	params := createTaskParams(taskID, sessionID, input, config.HistoryLength)

	// Send the request and process the response based on mode
	if useStreaming && !config.ForceNoStreaming {
		handleStreamingInteraction(a2aClient, params, taskID, config)
	} else {
		handleStandardInteraction(a2aClient, params, taskID, config)
	}

	return taskID
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
func createTaskParams(taskID, sessionID, input string, historyLength int) protocol.SendTaskParams {
	message := protocol.NewMessage(
		protocol.MessageRoleUser,
		[]protocol.Part{protocol.NewTextPart(input)},
	)

	params := protocol.SendTaskParams{
		ID:        taskID,
		SessionID: &sessionID,
		Message:   message,
	}

	if historyLength > 0 {
		params.HistoryLength = &historyLength
	}

	return params
}

// handleStreamingInteraction sends a streaming request to the agent and processes the response.
func handleStreamingInteraction(
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

	// Get the final task state if configured to do so.
	if config.UseTasksGet {
		getFinalTaskState(a2aClient, taskID, config.Timeout, finalTaskState, finalArtifacts, config.HistoryLength)
	}

	log.Printf("Stream processing finished for task %s.", taskID)
	fmt.Println(strings.Repeat("-", 60))
}

// handleStandardInteraction sends a standard (non-streaming) request to the agent.
func handleStandardInteraction(
	a2aClient *client.A2AClient,
	params protocol.SendTaskParams,
	taskID string,
	config Config,
) {
	// Create context for the request.
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	log.Printf("Sending standard request for task %s (Session: %s)...", taskID, *params.SessionID)

	// Send the task
	task, err := a2aClient.SendTasks(ctx, params)

	if err != nil {
		log.Printf("ERROR: SendTasks request failed: %v", err)
		fmt.Println(strings.Repeat("-", 60))
		return
	}

	// Process the response
	fmt.Println("\n<< Agent Response:")
	fmt.Println(strings.Repeat("-", 10))

	// Display task state
	fmt.Printf("  State: %s (%s)\n", task.Status.State, formatTimestamp(task.Status.Timestamp))

	// Display message if present
	if task.Status.Message != nil {
		fmt.Println("  Message:")
		printMessage(*task.Status.Message)
	}

	// Display artifacts if present
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

	// Display history if present
	if task.History != nil && len(task.History) > 0 {
		fmt.Println("  History:")
		for i, msg := range task.History {
			role := "User"
			if msg.Role == protocol.MessageRoleAgent {
				role = "Agent"
			}
			fmt.Printf("    [%d] %s:\n", i+1, role)
			printParts(msg.Parts)
		}
	}

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

	for {
		select {
		case <-ctx.Done():
			// Context timed out or was cancelled
			log.Printf("ERROR: Context timeout or cancellation while waiting for stream events: %v", ctx.Err())
			return finalTaskState, finalArtifacts

		case event, ok := <-eventChan:
			if !ok {
				// Channel closed by the client/server
				log.Println("Stream channel closed.")
				if ctx.Err() != nil {
					log.Printf("Context error after stream close: %v", ctx.Err())
				}
				return finalTaskState, finalArtifacts
			}

			// Process the received event based on its type
			switch e := event.(type) {
			case protocol.TaskStatusUpdateEvent:
				fmt.Printf("  [Status Update: %s (%s)]\n", e.Status.State, formatTimestamp(e.Status.Timestamp))
				if e.Status.Message != nil {
					printMessage(*e.Status.Message)
				}

				// Store the final state if this is a terminal status
				if e.IsFinal() {
					finalTaskState = e.Status.State
					log.Printf("Final status received: %s", finalTaskState)

					// Print a message indicating task completion state
					if e.Status.State == protocol.TaskStateCompleted {
						fmt.Println("  [Task completed successfully]")
					} else if e.Status.State == protocol.TaskStateFailed {
						fmt.Println("  [Task failed]")
					} else if e.Status.State == protocol.TaskStateCanceled {
						fmt.Println("  [Task was canceled]")
					}
					return finalTaskState, finalArtifacts
				}

			case protocol.TaskArtifactUpdateEvent:
				// Get the artifact name or use a default
				name := getArtifactName(e.Artifact)

				// Show if this is an append operation
				if e.Artifact.Append != nil && *e.Artifact.Append {
					fmt.Printf("  [Artifact Update: %s (Appending)]\n", name)
				} else {
					fmt.Printf("  [Artifact Update: %s]\n", name)
				}

				// Print the artifact parts
				printParts(e.Artifact.Parts)

				// Handle artifact storage for return value
				if e.Artifact.Append != nil && *e.Artifact.Append && len(finalArtifacts) > 0 {
					// Find existing artifact with same index to append to
					for i, art := range finalArtifacts {
						if art.Index == e.Artifact.Index {
							// Append parts
							combinedParts := append(art.Parts, e.Artifact.Parts...)
							finalArtifacts[i].Parts = combinedParts

							// Update other fields if needed
							if e.Artifact.Name != nil {
								finalArtifacts[i].Name = e.Artifact.Name
							}
							if e.Artifact.Description != nil {
								finalArtifacts[i].Description = e.Artifact.Description
							}
							if e.Artifact.LastChunk != nil {
								finalArtifacts[i].LastChunk = e.Artifact.LastChunk
							}

							// Break after updating
							break
						}
					}
				} else {
					finalArtifacts = append(finalArtifacts, e.Artifact)
				}

				// For artifact updates, we note it's the final artifact,
				// but we don't exit yet - per A2A spec, we should wait for the final status update
				if e.IsFinal() {
					log.Printf("Final artifact received for index %d", e.Artifact.Index)
				}

			default:
				log.Printf("Warning: Received unknown event type: %T\n", event)
			}
		}
	}
}

// getArtifactName returns the name of an artifact or a default if name is nil
func getArtifactName(artifact protocol.Artifact) string {
	if artifact.Name != nil {
		return *artifact.Name
	}
	return fmt.Sprintf("Artifact #%d", artifact.Index+1)
}

// getFinalTaskState fetches and displays the final task state.
func getFinalTaskState(
	a2aClient *client.A2AClient,
	taskID string,
	timeout time.Duration,
	streamState protocol.TaskState,
	streamArtifacts []protocol.Artifact,
	historyLength int,
) {
	finalCtx, finalCancel := context.WithTimeout(context.Background(), timeout)
	defer finalCancel()

	params := protocol.TaskQueryParams{ID: taskID}
	if historyLength > 0 {
		params.HistoryLength = &historyLength
	}

	finalTask, getErr := a2aClient.GetTasks(finalCtx, params)

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

// cancelTask attempts to cancel a running task.
func cancelTask(a2aClient *client.A2AClient, taskID string, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	log.Printf("Attempting to cancel task %s...", taskID)

	task, err := a2aClient.CancelTasks(ctx, protocol.TaskIDParams{ID: taskID})

	if err != nil {
		log.Printf("ERROR: Failed to cancel task %s: %v", taskID, err)
		fmt.Printf("Failed to cancel task: %v\n", err)
		return
	}

	fmt.Println("Task cancellation result:")
	fmt.Printf("  State: %s (%s)\n", task.Status.State, formatTimestamp(task.Status.Timestamp))

	if task.Status.Message != nil {
		fmt.Println("  Message:")
		printMessage(*task.Status.Message)
	}
}

// getTask fetches and displays a task's current state.
func getTask(a2aClient *client.A2AClient, taskID string, historyLength int, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	log.Printf("Fetching task %s...", taskID)

	params := protocol.TaskQueryParams{ID: taskID}
	if historyLength > 0 {
		params.HistoryLength = &historyLength
	}

	task, err := a2aClient.GetTasks(ctx, params)

	if err != nil {
		log.Printf("ERROR: Failed to get task %s: %v", taskID, err)
		fmt.Printf("Failed to get task: %v\n", err)
		return
	}

	fmt.Println("Task details:")
	displayFinalTaskState(task)
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

	if task.History != nil && len(task.History) > 0 {
		fmt.Println("  History:")
		for i, msg := range task.History {
			role := "User"
			if msg.Role == protocol.MessageRoleAgent {
				role = "Agent"
			}
			fmt.Printf("    [%d] %s:\n", i+1, role)
			printParts(msg.Parts)
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
