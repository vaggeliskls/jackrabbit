# Runner System

Remote command execution system with PostgreSQL-backed queuing, real-time SSE dispatch, metrics collection, and Traefik authentication.

## Features

- **Distributed Command Execution**: Dispatch commands to remote runners via SSE
- **Real-time Streaming**: Live command output via Server-Sent Events
- **Metrics & Logs**: Collect system metrics (CPU/memory) and stream logs to server
- **Background Workers**: Automatic cleanup (reaper), retention policies, metric rollups
- **Authentication**: Traefik ForwardAuth middleware for API security
- **Multi-language SDKs**: Go, TypeScript, Python clients
- **Docker Compose**: Single command deployment

## Quick Start

```bash
# 1. Clone and configure
git clone <repo-url>
cd remote-execute
cp .env.example .env
cp .env.client.example .env.client

# 2. Start all services (postgres, server, traefik, client)
docker compose up -d

# 3. Wait for services to be ready
docker compose ps

# 4. Run end-to-end test
./test-e2e.ps1  # Windows PowerShell
# or
bash test-e2e.sh  # Linux/Mac

# 5. View logs
docker compose logs -f client
docker compose logs -f server
```

## Prerequisites

- Go 1.22+
- Docker with Compose v2
- Task CLI (`go install github.com/go-task/task/v3/cmd/task@latest`) - optional
- Node.js 18+ (for TypeScript SDK development)
- Python 3.10+ (for Python SDK development)

## Architecture

```
┌─────────────┐         ┌──────────────┐         ┌──────────────┐
│   Client    │◄────SSE─┤    Server    │◄───────►│  PostgreSQL  │
│  (Runner)   │         │  (HTTP/API)  │         │   (Queue)    │
└─────────────┘         └──────────────┘         └──────────────┘
      │                        │                         │
      │ Logs/Metrics           │ LISTEN/NOTIFY          │
      └────────────────────────┴─────────────────────────┘
                               │
                         ┌─────┴──────┐
                         │   Traefik  │
                         │ (Auth/LB)  │
                         └────────────┘
```

### Components

- **Server** ([server/](server/)): Go HTTP API, SSE streaming, background workers
- **Client** ([client/](client/)): Runner agent that executes commands and ships telemetry
- **SDKs** ([sdk/](sdk/)): Generated API clients (Go, TypeScript, Python)
- **PostgreSQL**: Command queue, logs, metrics, retention policies
- **Traefik**: Reverse proxy with authentication middleware

## Development

### Building

```bash
# Build all modules
task build

# Or build individually
cd server && go build ./...
cd client && go build ./...
cd sdk/go && go build ./...
```

### Testing

```bash
# Run end-to-end test
./test-e2e.ps1  # PowerShell
bash test-e2e.sh  # Bash

# Manual testing
# 1. Register a runner
curl -X POST http://localhost:80/api/v1/runners/register \
  -H "Authorization: Bearer secret" \
  -H "Content-Type: application/json" \
  -d '{
    "slug": "test-runner",
    "name": "Test Runner",
    "tags": ["test"],
    "concurrency_limit": 2,
    "gpu_capable": false
  }'

# 2. Send a command
curl -X POST http://localhost:80/api/v1/commands \
  -H "Authorization: Bearer secret" \
  -H "Content-Type: application/json" \
  -d '{
    "runner_slug": "test-runner",
    "payload": {"cmd": "echo Hello World"},
    "timeout_secs": 30
  }'

# 3. Get command status
curl http://localhost:80/api/v1/commands/{id} \
  -H "Authorization: Bearer secret"
```

### Hot Reload Development

```bash
# Start with file watching
docker compose watch

# Or use Task
task dev
```

### Useful Commands

```bash
# View all services
docker compose ps

# View logs
docker compose logs -f server
docker compose logs -f client

# Access database
task db  # Opens Adminer on localhost:8082

# Stop everything
docker compose down

# Reset database
docker compose down -v
docker compose up -d
```

## Configuration

### Server Environment Variables

See [.env.example](.env.example):

- `SERVER_PORT`: HTTP port (default: 8080)
- `SERVER_ENV`: Environment mode (development/production)
- `POSTGRES_*`: Database connection settings
- `TRAEFIK_API_TOKEN`: Authentication token for API requests
- Worker intervals (reaper, retention, rollup)

### Client Environment Variables

See [.env.client.example](.env.client.example):

- `CLIENT_SERVER_URL`: Server URL (default: http://localhost:80)
- `CLIENT_RUNNER_SLUG`: Unique runner identifier
- `CLIENT_API_TOKEN`: Must match server's TRAEFIK_API_TOKEN
- `CLIENT_MAX_CONCURRENCY`: Max concurrent commands
- `CLIENT_LOG_BATCH_INTERVAL_MS`: Log shipping interval
- `CLIENT_METRIC_SAMPLE_INTERVAL_MS`: Metric collection interval

## API Documentation

The API is defined in [openapi/openapi.yaml](openapi/openapi.yaml) and exposed via:

- **Swagger UI**: http://localhost:8080/docs (when server runs standalone)
- **Traefik Dashboard**: http://localhost:8081 (when using compose)

### Key Endpoints

- `POST /api/v1/runners/register` - Register a runner
- `DELETE /api/v1/runners/{slug}` - Deregister a runner
- `POST /api/v1/runners/{slug}/heartbeat` - Send heartbeat
- `GET /api/v1/runners/{slug}/sse` - SSE stream for command dispatch
- `POST /api/v1/commands` - Send a command to a runner
- `GET /api/v1/commands/{id}` - Get command status
- `POST /api/v1/commands/{id}/kill` - Kill a running command
- `GET /api/v1/commands/{id}/logs` - Get command logs
- `GET /api/v1/commands/{id}/metrics` - Get command metrics

## SDKs

### Go SDK

```go
import "github.com/runner/sdk"

client := sdk.New("http://localhost:80", sdk.WithToken("secret"))

// Register runner
runner, err := client.RegisterRunner(ctx, sdk.RunnerConfig{
    Slug: "my-runner",
    Name: "My Runner",
    Tags: []string{"prod"},
    ConcurrencyLimit: 4,
    GPUCapable: false,
})

// Send command
cmd, err := client.SendCommand(ctx, "my-runner", map[string]interface{}{
    "cmd": "echo hello",
}, 30)

// Watch for commands via SSE
for event := range client.WatchCommands(ctx, "my-runner") {
    fmt.Printf("Command: %s\n", event.CommandID)
}
```

### TypeScript SDK

```typescript
import { RunnerClient } from '@runner/sdk';

const client = new RunnerClient('http://localhost:80', { token: 'secret' });

// Register runner
const runner = await client.registerRunner({
  slug: 'my-runner',
  name: 'My Runner',
  tags: ['prod'],
  concurrency_limit: 4,
  gpu_capable: false
});

// Send command
const cmd = await client.sendCommand('my-runner', { cmd: 'echo hello' }, 30);

// Watch for commands via SSE
const events = client.watchCommands('my-runner');
events.addEventListener('command_dispatch', (e) => {
  console.log('Command:', JSON.parse(e.data));
});
```

### Python SDK

```python
from runner_sdk import RunnerClient

client = RunnerClient("http://localhost:80", token="secret")

# Register runner
runner = client.register_runner(
    slug="my-runner",
    name="My Runner",
    tags=["prod"],
    concurrency_limit=4,
    gpu_capable=False
)

# Send command
cmd = client.send_command("my-runner", {"cmd": "echo hello"}, timeout_secs=30)

# Watch for commands via SSE
for event in client.watch_commands("my-runner"):
    print(f"Command: {event['command_id']}")
```

## Background Workers

The server runs three background workers:

1. **Reaper** (30s interval)
   - Marks commands past deadline as failed
   - Detects orphaned runners (offline >5min)
   - Fails commands assigned to unavailable runners

2. **Retention** (1h interval)
   - Applies age-based retention (deletes logs older than N days)
   - Applies size-based retention (deletes oldest logs when over N MB)
   - Configurable per scope (global/runner/command)

3. **Rollup** (1min interval)
   - Aggregates raw metrics into 1-minute buckets
   - Aggregates 1-minute metrics into 5-minute buckets
   - Reduces storage and improves query performance

## Troubleshooting

### Server won't start

```bash
# Check postgres is healthy
docker compose ps postgres

# View server logs
docker compose logs server

# Verify migrations ran
docker compose exec postgres psql -U runner -d runner -c "\dt"
```

### Client can't connect to server

- Verify `CLIENT_SERVER_URL` is correct (use `http://server:8080` for Docker, `http://localhost:80` for local)
- Check `CLIENT_API_TOKEN` matches `TRAEFIK_API_TOKEN` in server .env
- Ensure server is running: `docker compose ps server`

### Commands stay in "queued" state

- Client must be running to execute commands
- Check client logs: `docker compose logs -f client`
- Verify SSE connection: Should see "SSE connection established"
- Check runner is registered: `curl http://localhost:80/api/v1/runners -H "Authorization: Bearer secret"`

### No logs/metrics appearing

- Logs/metrics are batched - wait 500ms for logs, 1s for metrics
- Check client is shipping: Look for "shipped logs" and "shipped metrics" in client logs
- Verify server receives data: Check server logs for batch insert operations

## Project Structure

```
remote-execute/
├── server/           # Go HTTP server
│   ├── cmd/server/   # Main entry point
│   ├── internal/     # Internal packages
│   │   ├── api/      # HTTP handlers, middleware
│   │   ├── config/   # Configuration
│   │   ├── models/   # Domain models
│   │   ├── store/    # Database interface & implementation
│   │   └── workers/  # Background workers
│   └── migrations/   # SQL migrations
├── client/           # Go runner client
│   ├── cmd/runner/   # CLI entry point
│   └── internal/     # Client components
│       ├── config/   # Config loading
│       ├── runner/   # Executor, SSE, lifecycle
│       └── shipper/  # Log & metric shippers
├── sdk/              # API client SDKs
│   ├── go/           # Go SDK (generated + wrapper)
│   ├── typescript/   # TypeScript SDK (hand-written)
│   └── python/       # Python SDK (hand-written)
├── openapi/          # OpenAPI 3.1 spec
├── traefik/          # Traefik dynamic config
├── compose.yml       # Docker Compose services
├── Taskfile.yml      # Task commands
└── test-e2e.*        # End-to-end test scripts
```

## License

MIT
