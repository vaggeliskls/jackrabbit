# Runner Client

The Runner Client is a lightweight agent that registers with the Remote Execute server, listens for command dispatch events, executes commands, and ships logs and metrics back to the server.

## Features

- **Command Execution**: Runs shell commands with configurable timeouts
- **Real-time Streaming**: Streams command output (stdout/stderr) line-by-line
- **SSE Integration**: Listens for command dispatch events via Server-Sent Events
- **Batched Shipping**: Efficiently batches and ships logs and metrics to the server
- **System Metrics**: Collects CPU and memory usage during command execution
- **Graceful Shutdown**: Handles SIGTERM/SIGINT for clean deregistration
- **Concurrency Control**: Configurable max concurrent command execution

## Configuration

All configuration is done via environment variables. Copy `.env.example` to `.env` and customize:

```bash
cp .env.example .env
```

### Required Environment Variables

- `CLIENT_SERVER_URL`: URL of the Remote Execute server (e.g., `http://localhost:80`)
- `CLIENT_RUNNER_SLUG`: Unique identifier for this runner
- `CLIENT_API_TOKEN`: Authentication token for server API

### Optional Environment Variables

- `CLIENT_RUNNER_NAME`: Human-readable name (defaults to slug)
- `CLIENT_RUNNER_TAGS`: Comma-separated tags (e.g., `dev,linux,docker`)
- `CLIENT_MAX_CONCURRENCY`: Max concurrent commands (default: 4)
- `CLIENT_GPU_CAPABLE`: Whether this runner has GPU (default: false)
- `CLIENT_LOG_BATCH_INTERVAL_MS`: Log batch interval in ms (default: 500)
- `CLIENT_METRIC_SAMPLE_INTERVAL_MS`: Metric sampling interval in ms (default: 1000)
- `CLIENT_METRIC_BATCH_SIZE`: Number of metrics per batch (default: 50)
- `CLIENT_HEARTBEAT_INTERVAL_SECS`: Heartbeat interval in seconds (default: 30)
- `CLIENT_TLS_SKIP_VERIFY`: Skip TLS verification (default: false)
- `CLIENT_CA_CERT_PATH`: Path to CA certificate for TLS
- `ENV`: Environment mode (`development` or `production`)

## Building

```bash
go build -o bin/runner.exe ./cmd/runner
```

Or use the build script (if available):

```bash
go build -o runner ./cmd/runner
```

## Running

### Development Mode

```bash
./bin/runner.exe run
```

This will:
1. Load configuration from `.env` file
2. Register with the server
3. Start SSE listener for command dispatch
4. Begin shipping logs and metrics
5. Send periodic heartbeats

### Production Mode

Set `ENV=production` to use JSON logging instead of console output:

```bash
ENV=production ./bin/runner.exe run
```

## Architecture

### Components

1. **Executor** (`internal/runner/executor.go`)
   - Runs commands using `sh -c` (or `cmd /c` on Windows)
   - Streams stdout/stderr line-by-line to channels
   - Tracks exit codes and execution time
   - Supports command cancellation and timeouts

2. **SSE Listener** (`internal/runner/sse.go`)
   - Connects to server's SSE endpoint (`/api/v1/runners/{slug}/sse`)
   - Listens for `command_dispatch` events
   - Auto-reconnects with exponential backoff (1s → 30s)

3. **Log Shipper** (`internal/shipper/logs.go`)
   - Batches log entries in memory
   - Ships to server every 500ms or when buffer is full (100 entries)
   - Includes sequence numbers for ordering

4. **Metric Shipper** (`internal/shipper/metrics.go`)
   - Collects CPU/memory metrics using `gopsutil`
   - Samples every 1 second during command execution
   - Batches and ships 50 metrics at a time

5. **Lifecycle Manager** (`internal/runner/lifecycle.go`)
   - Orchestrates all components
   - Registers/deregisters runner with server
   - Handles graceful shutdown
   - Enforces concurrency limits

### Data Flow

```
Server SSE → SSE Listener → Lifecycle Manager → Executor
                                    ↓
                            Log Shipper → Server API
                            Metric Shipper → Server API
```

## Command Execution Flow

1. Server dispatches command via SSE `command_dispatch` event
2. SSE Listener receives event and passes to Lifecycle Manager
3. Lifecycle Manager checks concurrency limit
4. Executor starts command in subprocess with timeout
5. Stdout/stderr are streamed line-by-line
6. Log Shipper batches and sends logs to server
7. Metric Shipper samples system metrics during execution
8. On completion, result (exit code, timestamps) is reported to server
9. Runner becomes available for next command

## Error Handling

- **Connection Errors**: SSE listener auto-reconnects with exponential backoff
- **Command Timeout**: Commands exceeding timeout are killed (exit code 124)
- **Command Cancellation**: Cancelled commands get exit code 130
- **Execution Failure**: Non-zero exit codes are reported with error messages
- **Shipping Errors**: Failed log/metric shipments are logged and dropped

## Graceful Shutdown

On SIGTERM/SIGINT:
1. SSE listener stops accepting new commands
2. Running commands are allowed to complete (up to their timeout)
3. Final batch of logs/metrics are flushed
4. Runner deregisters from server
5. Process exits

## Development

### Dependencies

- Go 1.22+
- github.com/runner/sdk: API client for server
- github.com/rs/zerolog: Structured logging
- github.com/spf13/cobra: CLI framework
- github.com/joho/godotenv: .env file loading
- github.com/shirou/gopsutil/v3: System metrics

### Testing Locally

1. Start the server (see server README)
2. Configure `.env` with correct `CLIENT_SERVER_URL`
3. Run the client:
   ```bash
   ./bin/runner.exe run
   ```
4. Send a test command from server:
   ```bash
   curl -X POST http://localhost:80/api/v1/commands \
     -H "Authorization: Bearer secret" \
     -H "Content-Type: application/json" \
     -d '{
       "runner_slug": "my-runner-01",
       "payload": {"cmd": "echo Hello World"}
     }'
   ```
5. Check server logs for command result

## Troubleshooting

### Runner fails to register
- Check `CLIENT_SERVER_URL` is correct
- Verify `CLIENT_API_TOKEN` matches server configuration
- Ensure server is running and accessible

### Commands not executing
- Check SSE connection in logs (should see "SSE connection established")
- Verify `CLIENT_RUNNER_SLUG` matches the slug used when dispatching commands
- Check concurrency limit (`CLIENT_MAX_CONCURRENCY`)

### Logs not appearing in server
- Verify log shipper is running (check for "shipped logs" debug messages)
- Check server API endpoint is accessible
- Review server logs for ingestion errors

### High memory usage
- Reduce `CLIENT_METRIC_BATCH_SIZE`
- Increase `CLIENT_LOG_BATCH_INTERVAL_MS` to flush more frequently
- Lower `CLIENT_MAX_CONCURRENCY`

## License

See root LICENSE file.
