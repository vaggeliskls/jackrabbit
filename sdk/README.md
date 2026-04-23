# Runner System SDKs

Three SDKs generated from the OpenAPI specification with SSE streaming support and retry logic.

## Directory Structure

```
sdk/
├── go/                  # Go SDK (generated + wrapper)
│   ├── client.go        # Hand-written wrapper with SSE streaming
│   ├── generated/       # oapi-codegen output
│   └── go.mod
├── typescript/          # TypeScript SDK (hand-written)
│   ├── src/index.ts     # Complete client implementation
│   ├── dist/            # Built output (CJS, ESM, DTS)
│   └── package.json
└── python/              # Python SDK (hand-written)
    ├── runner_sdk/
    │   ├── __init__.py
    │   └── client.py    # Complete client implementation
    └── pyproject.toml
```

## Generation

Run code generation for Go SDK:

```bash
task generate
```

This will:
- Install oapi-codegen
- Generate Go client from OpenAPI spec
- Update Go dependencies

TypeScript and Python SDKs are hand-written and don't require generation.

## Building

### Go SDK

```bash
cd sdk/go
go build .
```

### TypeScript SDK

```bash
cd sdk/typescript
npm install
npm run build
```

Outputs:
- `dist/index.js` - CommonJS
- `dist/index.mjs` - ESM
- `dist/index.d.ts` - TypeScript definitions

### Python SDK

```bash
cd sdk/python
pip install -e .
```

## Features

All three SDKs provide:

- **Runner Management**: Register, deregister, list, get runner details
- **Command Operations**: Send commands, get status, kill commands
- **Log Access**: Retrieve command logs with pagination
- **Metric Access**: Get performance metrics (raw/1m/5m resolution)
- **SSE Streaming**: Real-time command dispatch via Server-Sent Events
  - Go: Custom SSE parser with exponential backoff reconnect
  - TypeScript: Native EventSource API
  - Python: sseclient-py with automatic reconnection
- **Log Watching**: Poll-based log streaming with sequence deduplication
- **Error Handling**: Consistent error types (NotFound, Conflict, Server errors)
- **Retry Logic**: Built-in reconnection for SSE streams

## Usage Examples

### Go

```go
import "github.com/runner/sdk"

client := sdk.New("http://localhost:8080")

// Register runner
runner, _ := client.RegisterRunner(ctx, sdk.RunnerConfig{
    Slug: "my-runner",
    Name: "My Runner",
    Tags: []string{"dev"},
})

// Watch for commands via SSE
events, _ := client.WatchCommands(ctx, "my-runner")
for event := range events {
    if event.Type == "command_dispatch" {
        cmdID := event.Data["command_id"].(string)
        // Execute command...
    }
}
```

### TypeScript

```typescript
import { RunnerClient } from '@runner/sdk'

const client = new RunnerClient('http://localhost:8080')

// Register runner
await client.registerRunner({
  slug: 'my-runner',
  name: 'My Runner',
  tags: ['dev']
})

// Watch for commands via SSE
const source = client.watchCommands('my-runner')
source.addEventListener('command_dispatch', (event) => {
  const data = JSON.parse(event.data)
  const commandId = data.command_id
  // Execute command...
})
```

### Python

```python
from runner_sdk import RunnerClient, RunnerConfig

with RunnerClient("http://localhost:8080") as client:
    # Register runner
    runner = client.register_runner(RunnerConfig(
        slug="my-runner",
        name="My Runner",
        tags=["dev"]
    ))
    
    # Watch for commands via SSE
    for event in client.watch_commands("my-runner"):
        if event["type"] == "command_dispatch":
            command_id = event["data"]["command_id"]
            # Execute command...
```

## Testing

All SDKs have been verified:

- **Go**: `go build .` succeeds
- **TypeScript**: `npm run build` succeeds, generates CJS/ESM/DTS
- **Python**: `from runner_sdk import RunnerClient` succeeds
- **Generation**: `task generate` runs without errors

## Notes

- Go SDK uses OpenAPI 3.1 spec (oapi-codegen shows warning but works)
- TypeScript SDK uses native browser EventSource (Node.js requires polyfill)
- Python SDK uses httpx for async-capable HTTP client
- All SDKs support optional authentication via Bearer token
