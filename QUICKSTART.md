# Quick Start Guide

Get the Runner System up and running in 5 minutes.

## Prerequisites

- Docker Desktop (Windows/Mac) or Docker Engine + Docker Compose (Linux)
- Git

## Step 1: Clone & Configure

```bash
# Clone the repository
git clone <repository-url>
cd remote-execute

# Copy environment files
cp .env.example .env
cp .env.client.example .env.client
```

### Windows PowerShell
```powershell
Copy-Item .env.example .env
Copy-Item .env.client.example .env.client
```

## Step 2: Start Services

```bash
# Start all services (postgres, server, traefik, adminer, client)
docker compose up -d

# Wait 10-15 seconds for services to initialize
# Check status
docker compose ps
```

All services should show "running" status:
- `postgres` - Database (port 5432)
- `server` - API server (port 8080)
- `traefik` - Reverse proxy (port 80)
- `adminer` - Database UI (port 8082)
- `client` - Runner agent

## Step 3: Verify Installation

### Check Service Health

```bash
# Server health check
curl http://localhost:80/health

# Should return: {"status":"ok"}
```

### View Logs

```bash
# Server logs
docker compose logs -f server

# Client logs (should see "runner started successfully")
docker compose logs -f client

# Stop following logs with Ctrl+C
```

## Step 4: Run End-to-End Test

### Windows PowerShell
```powershell
.\test-e2e.ps1
```

### Linux/Mac Bash
```bash
chmod +x test-e2e.sh
./test-e2e.sh
```

The test will:
1. ✓ Wait for server to be ready
2. ✓ Register a test runner
3. ✓ List all runners
4. ✓ Send a test command
5. ✓ Wait for execution
6. ✓ Retrieve logs
7. ✓ Deregister the runner

If you see all green checkmarks, the system is working! 🎉

## Step 5: Explore the System

### Access Database UI

Open http://localhost:8082 in your browser:
- System: PostgreSQL
- Server: postgres
- Username: runner
- Password: runner
- Database: runner

Explore tables:
- `runners` - Registered runner agents
- `commands` - Command queue
- `logs` - Command output logs
- `metrics` - System metrics

### View Traefik Dashboard

Open http://localhost:8081 in your browser to see:
- Active routers
- Services
- Middleware (including auth)

### Check Logs in Real-Time

```bash
# Follow server logs
docker compose logs -f server

# Follow client logs
docker compose logs -f client

# Follow all logs
docker compose logs -f
```

## Manual Testing

### 1. Register a Runner

```bash
curl -X POST http://localhost:80/api/v1/runners/register \
  -H "Authorization: Bearer secret" \
  -H "Content-Type: application/json" \
  -d '{
    "slug": "manual-runner",
    "name": "My Manual Test Runner",
    "tags": ["test", "manual"],
    "concurrency_limit": 2,
    "gpu_capable": false
  }'
```

### 2. List Runners

```bash
curl http://localhost:80/api/v1/runners \
  -H "Authorization: Bearer secret"
```

### 3. Send a Command

```bash
curl -X POST http://localhost:80/api/v1/commands \
  -H "Authorization: Bearer secret" \
  -H "Content-Type: application/json" \
  -d '{
    "runner_slug": "manual-runner",
    "payload": {
      "cmd": "echo Hello from Runner System && date && sleep 2 && echo Complete"
    },
    "timeout_secs": 30
  }'
```

Copy the `id` from the response.

### 4. Check Command Status

```bash
curl http://localhost:80/api/v1/commands/{COMMAND_ID} \
  -H "Authorization: Bearer secret"
```

Replace `{COMMAND_ID}` with the ID from step 3.

### 5. Get Command Logs

```bash
curl http://localhost:80/api/v1/commands/{COMMAND_ID}/logs \
  -H "Authorization: Bearer secret"
```

### 6. Get Command Metrics

```bash
curl http://localhost:80/api/v1/commands/{COMMAND_ID}/metrics \
  -H "Authorization: Bearer secret"
```

### 7. Deregister Runner

```bash
curl -X DELETE http://localhost:80/api/v1/runners/manual-runner \
  -H "Authorization: Bearer secret"
```

## Common Issues

### Services won't start

```bash
# Check Docker is running
docker ps

# View detailed logs
docker compose logs

# Reset everything
docker compose down -v
docker compose up -d
```

### Client can't connect

1. Check `.env.client` has correct server URL:
   ```
   CLIENT_SERVER_URL=http://server:8080
   ```

2. Verify API token matches between `.env` and `.env.client`:
   ```
   TRAEFIK_API_TOKEN=secret  # in .env
   CLIENT_API_TOKEN=secret   # in .env.client
   ```

3. Restart client:
   ```bash
   docker compose restart client
   ```

### Database connection errors

```bash
# Check postgres is healthy
docker compose ps postgres

# View postgres logs
docker compose logs postgres

# Recreate database
docker compose down -v postgres
docker compose up -d postgres
```

### Commands stay "queued"

Make sure the client service is running:
```bash
docker compose ps client
docker compose logs client
```

Should see: `"runner started successfully"` and `"SSE connection established"`

## Next Steps

- Read the [main README](README.md) for architecture details
- Explore the [OpenAPI spec](openapi/openapi.yaml)
- Check out the [SDKs](sdk/) for Go, TypeScript, and Python
- Review [client documentation](client/README.md) for runner configuration
- Review [server documentation](server/README.md) for API details

## Development Workflow

### Make Changes to Server

```bash
# Edit server code
# Then rebuild
docker compose build server
docker compose restart server

# Or use hot reload
docker compose watch
```

### Make Changes to Client

```bash
# Edit client code
# Then rebuild
docker compose build client
docker compose restart client

# Or use hot reload
docker compose watch
```

### Run Without Docker

```bash
# Terminal 1: Start postgres
docker compose up -d postgres

# Terminal 2: Run server
cd server
go run cmd/server/main.go

# Terminal 3: Run client
cd client
go run cmd/runner/main.go run
```

## Stopping Services

```bash
# Stop all services
docker compose down

# Stop and remove volumes (clears database)
docker compose down -v
```

## Getting Help

- Check the [Troubleshooting section](README.md#troubleshooting) in main README
- Review logs: `docker compose logs -f`
- Inspect database in Adminer: http://localhost:8082
- Check Traefik dashboard: http://localhost:8081

---

**Congratulations!** 🎉 Your Runner System is up and running. You can now:
- Send commands to remote runners
- Stream logs and metrics in real-time
- Scale horizontally by adding more runner clients
- Monitor everything through the database UI
