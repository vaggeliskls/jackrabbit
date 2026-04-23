# Runner System End-to-End Test
# PowerShell version

$ErrorActionPreference = "Stop"

$API_URL = if ($env:API_URL) { $env:API_URL } else { "http://localhost:80" }
$API_TOKEN = if ($env:API_TOKEN) { $env:API_TOKEN } else { "secret" }
$RUNNER_SLUG = "test-runner-$(Get-Date -Format 'yyyyMMddHHmmss')"

Write-Host "`n=== Runner System End-to-End Test ===`n" -ForegroundColor Yellow

function Invoke-ApiCall {
    param(
        [string]$Method,
        [string]$Endpoint,
        [string]$Body = ""
    )
    
    $headers = @{
        "Authorization" = "Bearer $API_TOKEN"
        "Content-Type" = "application/json"
    }
    
    $params = @{
        Uri = "$API_URL$Endpoint"
        Method = $Method
        Headers = $headers
    }
    
    if ($Body) {
        $params.Body = $Body
    }
    
    try {
        $response = Invoke-RestMethod @params
        return $response
    } catch {
        Write-Host "Error: $_" -ForegroundColor Red
        return $null
    }
}

# Step 1: Wait for server
Write-Host "[1/7] Waiting for server..." -ForegroundColor Yellow
$ready = $false
for ($i = 1; $i -le 30; $i++) {
    try {
        $health = Invoke-WebRequest -Uri "$API_URL/health" -UseBasicParsing -TimeoutSec 2
        if ($health.StatusCode -eq 200) {
            Write-Host "✓ Server is ready" -ForegroundColor Green
            $ready = $true
            break
        }
    } catch {
        Start-Sleep -Seconds 1
    }
}

if (-not $ready) {
    Write-Host "✗ Server did not become ready in time" -ForegroundColor Red
    exit 1
}

# Step 2: Register runner
Write-Host "`n[2/7] Registering runner..." -ForegroundColor Yellow
$registerPayload = @{
    slug = $RUNNER_SLUG
    name = "E2E Test Runner"
    tags = @("test", "e2e")
    concurrency_limit = 2
    gpu_capable = $false
} | ConvertTo-Json

$registerResponse = Invoke-ApiCall -Method POST -Endpoint "/api/v1/runners/register" -Body $registerPayload
if ($registerResponse) {
    Write-Host "✓ Runner registered: $RUNNER_SLUG" -ForegroundColor Green
    $registerResponse | ConvertTo-Json -Depth 10
}

# Step 3: List runners
Write-Host "`n[3/7] Listing runners..." -ForegroundColor Yellow
$runners = Invoke-ApiCall -Method GET -Endpoint "/api/v1/runners"
$runners | ConvertTo-Json -Depth 10

# Step 4: Send command
Write-Host "`n[4/7] Sending test command..." -ForegroundColor Yellow
$commandPayload = @{
    target_type = "runner"
    target_value = $RUNNER_SLUG
    payload = @{
        cmd = "echo 'Hello from E2E test' && sleep 2 && echo 'Test complete'"
    }
    timeout_secs = 30
} | ConvertTo-Json

$commandResponse = Invoke-ApiCall -Method POST -Endpoint "/api/v1/commands" -Body $commandPayload
$COMMAND_ID = $commandResponse.id

if (-not $COMMAND_ID) {
    Write-Host "✗ Failed to send command" -ForegroundColor Red
    exit 1
}

Write-Host "✓ Command sent: $COMMAND_ID" -ForegroundColor Green
$commandResponse | ConvertTo-Json -Depth 10

# Step 5: Wait and check status
Write-Host "`n[5/7] Waiting for command execution..." -ForegroundColor Yellow
Start-Sleep -Seconds 5

$commandStatus = Invoke-ApiCall -Method GET -Endpoint "/api/v1/commands/$COMMAND_ID"
$commandStatus | ConvertTo-Json -Depth 10

$status = $commandStatus.status
if ($status -eq "running" -or $status -eq "queued") {
    Write-Host "→ Command still executing, waiting..." -ForegroundColor Yellow
    Start-Sleep -Seconds 5
    $commandStatus = Invoke-ApiCall -Method GET -Endpoint "/api/v1/commands/$COMMAND_ID"
}

# Step 6: Get logs
Write-Host "`n[6/7] Retrieving command logs..." -ForegroundColor Yellow
$logs = Invoke-ApiCall -Method GET -Endpoint "/api/v1/commands/$COMMAND_ID/logs"
$logs | ConvertTo-Json -Depth 10

$logCount = if ($logs.logs) { $logs.logs.Count } else { 0 }
if ($logCount -gt 0) {
    Write-Host "✓ Found $logCount log entries" -ForegroundColor Green
} else {
    Write-Host "⚠ No logs found (this might be expected if client is not running)" -ForegroundColor Yellow
}

# Step 7: Deregister
Write-Host "`n[7/7] Deregistering runner..." -ForegroundColor Yellow
Invoke-ApiCall -Method DELETE -Endpoint "/api/v1/runners/$RUNNER_SLUG" | Out-Null
Write-Host "✓ Runner deregistered" -ForegroundColor Green

# Summary
Write-Host "`n=== Test Complete ===" -ForegroundColor Green
Write-Host "Runner: $RUNNER_SLUG"
Write-Host "Command: $COMMAND_ID"
Write-Host "Status: $status"
Write-Host "`nNote: If status is 'queued' or no logs were found, make sure the client is running:" -ForegroundColor Yellow
Write-Host "  docker compose up -d client" -ForegroundColor Yellow
Write-Host "  or run: cd client; go run cmd/runner/main.go run" -ForegroundColor Yellow
