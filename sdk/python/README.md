# Runner System Python SDK

Python client for the Runner System API.

## Installation

```bash
pip install -e .
```

## Usage

```python
from runner_sdk import RunnerClient, RunnerConfig, CommandRequest

# Create client
client = RunnerClient("http://localhost:8080", token="your-token")

# Register runner
config = RunnerConfig(
    slug="my-runner",
    name="My Runner",
    tags=["dev", "test"],
    concurrency_limit=4,
    gpu_capable=False
)
runner = client.register_runner(config)

# Send command
cmd_req = CommandRequest(
    target_type="slug",
    target_value="my-runner",
    payload={"cmd": "echo hello"},
    timeout_secs=300
)
command = client.send_command(cmd_req)

# Watch for commands (SSE)
for event in client.watch_commands("my-runner"):
    if event["type"] == "command_dispatch":
        command_id = event["data"]["command_id"]
        print(f"Received command: {command_id}")
```
