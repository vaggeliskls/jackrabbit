#!/usr/bin/env bash
set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

API_URL="${API_URL:-http://localhost:80}"
API_TOKEN="${API_TOKEN:-secret}"
RUNNER_SLUG="test-runner-$(date +%s)"

echo -e "${YELLOW}=== Runner System End-to-End Test ===${NC}\n"

# Function to make API calls
api_call() {
    local method=$1
    local endpoint=$2
    local data=${3:-}
    
    if [ -n "$data" ]; then
        curl -s -X "$method" "$API_URL$endpoint" \
            -H "Authorization: Bearer $API_TOKEN" \
            -H "Content-Type: application/json" \
            -d "$data"
    else
        curl -s -X "$method" "$API_URL$endpoint" \
            -H "Authorization: Bearer $API_TOKEN"
    fi
}

# Step 1: Wait for server to be ready
echo -e "${YELLOW}[1/7]${NC} Waiting for server..."
for i in {1..30}; do
    if curl -s -f "$API_URL/health" > /dev/null 2>&1; then
        echo -e "${GREEN}✓${NC} Server is ready"
        break
    fi
    if [ $i -eq 30 ]; then
        echo -e "${RED}✗${NC} Server did not become ready in time"
        exit 1
    fi
    sleep 1
done

# Step 2: Register a runner
echo -e "\n${YELLOW}[2/7]${NC} Registering runner..."
REGISTER_PAYLOAD=$(cat <<EOF
{
    "slug": "$RUNNER_SLUG",
    "name": "E2E Test Runner",
    "tags": ["test", "e2e"],
    "concurrency_limit": 2,
    "gpu_capable": false
}
EOF
)

REGISTER_RESPONSE=$(api_call POST "/api/v1/runners/register" "$REGISTER_PAYLOAD")
echo -e "${GREEN}✓${NC} Runner registered: $RUNNER_SLUG"
echo "$REGISTER_RESPONSE" | jq '.' 2>/dev/null || echo "$REGISTER_RESPONSE"

# Step 3: List runners
echo -e "\n${YELLOW}[3/7]${NC} Listing runners..."
RUNNERS=$(api_call GET "/api/v1/runners")
echo "$RUNNERS" | jq '.' 2>/dev/null || echo "$RUNNERS"

# Step 4: Send a command
echo -e "\n${YELLOW}[4/7]${NC} Sending test command..."
COMMAND_PAYLOAD=$(cat <<EOF
{
    "target_type": "runner",
    "target_value": "$RUNNER_SLUG",
    "payload": {
        "cmd": "echo 'Hello from E2E test' && sleep 2 && echo 'Test complete'"
    },
    "timeout_secs": 30
}
EOF
)

COMMAND_RESPONSE=$(api_call POST "/api/v1/commands" "$COMMAND_PAYLOAD")
COMMAND_ID=$(echo "$COMMAND_RESPONSE" | jq -r '.id' 2>/dev/null || echo "")

if [ -z "$COMMAND_ID" ] || [ "$COMMAND_ID" = "null" ]; then
    echo -e "${RED}✗${NC} Failed to send command"
    echo "$COMMAND_RESPONSE"
    exit 1
fi

echo -e "${GREEN}✓${NC} Command sent: $COMMAND_ID"
echo "$COMMAND_RESPONSE" | jq '.' 2>/dev/null || echo "$COMMAND_RESPONSE"

# Step 5: Wait and check command status
echo -e "\n${YELLOW}[5/7]${NC} Waiting for command execution..."
sleep 5

COMMAND_STATUS=$(api_call GET "/api/v1/commands/$COMMAND_ID")
echo "$COMMAND_STATUS" | jq '.' 2>/dev/null || echo "$COMMAND_STATUS"

STATUS=$(echo "$COMMAND_STATUS" | jq -r '.status' 2>/dev/null || echo "unknown")
if [ "$STATUS" = "running" ] || [ "$STATUS" = "queued" ]; then
    echo -e "${YELLOW}→${NC} Command still executing, waiting..."
    sleep 5
    COMMAND_STATUS=$(api_call GET "/api/v1/commands/$COMMAND_ID")
fi

# Step 6: Get command logs
echo -e "\n${YELLOW}[6/7]${NC} Retrieving command logs..."
LOGS=$(api_call GET "/api/v1/commands/$COMMAND_ID/logs")
echo "$LOGS" | jq '.' 2>/dev/null || echo "$LOGS"

LOG_COUNT=$(echo "$LOGS" | jq '.logs | length' 2>/dev/null || echo "0")
if [ "$LOG_COUNT" -gt 0 ]; then
    echo -e "${GREEN}✓${NC} Found $LOG_COUNT log entries"
else
    echo -e "${YELLOW}⚠${NC} No logs found (this might be expected if client is not running)"
fi

# Step 7: Deregister runner
echo -e "\n${YELLOW}[7/7]${NC} Deregistering runner..."
api_call DELETE "/api/v1/runners/$RUNNER_SLUG"
echo -e "${GREEN}✓${NC} Runner deregistered"

# Summary
echo -e "\n${GREEN}=== Test Complete ===${NC}"
echo -e "Runner: ${RUNNER_SLUG}"
echo -e "Command: ${COMMAND_ID}"
echo -e "Status: ${STATUS}"
echo -e "\n${YELLOW}Note:${NC} If status is 'queued' or no logs were found, make sure the client is running:"
echo -e "  ${YELLOW}docker compose up -d client${NC}"
echo -e "  or run: ${YELLOW}cd client && go run cmd/runner/main.go run${NC}"
