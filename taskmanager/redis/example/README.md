# A2A Redis Task Manager Example

This example demonstrates how to create a complete A2A (Application-to-Application) flow using the Redis task manager. It includes both a server and client implementation that use the official A2A Go packages.

## Prerequisites

- Go 1.21 or later
- Redis 6.0 or later running locally (or accessible through network)

## Running the Server

The server implements a simple task processor that can receive tasks, process them, and return results. It uses Redis for task storage and state management.

```bash
# Start Redis if it's not already running
# For example, using Docker:
docker run --name redis -p 6379:6379 -d redis

# Run the server with default settings (Redis on localhost:6379)
cd server
go run main.go

# Or with custom settings
go run main.go -port=8080 -redis=localhost:6379 -redis-pass="" -redis-db=0
```

The server supports the following command-line arguments:

- `-port`: The HTTP port to listen on (default: 8080)
- `-redis`: Redis server address (default: localhost:6379)
- `-redis-pass`: Redis password (default: "")
- `-redis-db`: Redis database number (default: 0)

## Running the Client

The client provides a command-line interface to interact with the server. It supports sending tasks, retrieving task status, and streaming task updates.

```bash
cd client
go run main.go -op=send -message="Hello, world!"

# Get task status
go run main.go -op=get -task=<task-id>

# Cancel a task
go run main.go -op=cancel -task=<task-id>

# Stream task updates
go run main.go -op=stream -message="Hello, streaming!"
```

The client supports the following operations:

- `send`: Create and send a new task, then poll until completion
- `get`: Retrieve the status of an existing task
- `cancel`: Cancel an in-progress task
- `stream`: Create a task and stream updates until completion

Command-line arguments:

- `-server`: The A2A server URL (default: http://localhost:8080)
- `-message`: The message to send (default: "Hello, world!")
- `-op`: The operation to perform (default: "send")
- `-task`: The task ID (required for get and cancel operations)
- `-idkey`: An idempotency key for task creation (optional)

## Understanding the Code

### Server

The server implementation:

1. Uses the `redismgr.TaskManager` for persistent task storage in Redis
2. Implements a custom `DemoTaskProcessor` for task processing logic
3. Creates an official A2A server with appropriate configuration
4. Handles tasks via the A2A protocol endpoints

### Client

The client implementation:

1. Uses the official `client.A2AClient` to communicate with the server
2. Supports various operations through the command-line interface
3. Formats and displays task state and artifacts
4. Demonstrates both synchronous and streaming interaction

## Example Flow

1. Start the server:
   ```bash
   cd server
   go run main.go
   ```

2. Send a task from the client:
   ```bash
   cd client
   go run main.go -op=send -message="Process this message"
   ```

3. The client will display task status updates, including the final result and any artifacts produced.

## Error Handling

The example demonstrates error handling in several ways:

- If you include the word "error" in your message, the server will return a simulated error
- Connection errors between client and server are properly reported
- Task cancellation is supported through the cancel operation

## Next Steps

- Modify the `DemoTaskProcessor` to implement your own task processing logic
- Configure the Redis task manager for production use (authentication, clustering, etc.)
- Integrate the server into your own application 