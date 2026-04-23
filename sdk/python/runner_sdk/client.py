"""Runner System Client"""

import json
import time
from dataclasses import dataclass
from typing import Any, Dict, Iterator, List, Optional

import httpx
import sseclient


@dataclass
class RunnerConfig:
    slug: str
    name: str
    tags: Optional[List[str]] = None
    concurrency_limit: int = 4
    gpu_capable: bool = False


@dataclass
class CommandRequest:
    target_type: str
    target_value: str
    payload: Dict[str, Any]
    max_retries: int = 0
    timeout_secs: int = 300
    deadline: Optional[str] = None


@dataclass
class RunnerFilter:
    status: Optional[str] = None
    tags: Optional[List[str]] = None


@dataclass
class MetricOpts:
    resolution: str = "raw"
    page: int = 1
    page_size: int = 100


class RunnerClient:
    def __init__(
        self,
        base_url: str,
        token: Optional[str] = None,
        timeout: float = 30.0,
        **kwargs
    ):
        self.base_url = base_url.rstrip("/")
        self.token = token
        self.client = httpx.Client(timeout=timeout, **kwargs)
        
        if self.token:
            self.client.headers["Authorization"] = f"Bearer {self.token}"

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        self.close()

    def close(self):
        self.client.close()

    def register_runner(self, config: RunnerConfig) -> Dict[str, Any]:
        """Register a runner with the server"""
        body = {
            "slug": config.slug,
            "name": config.name,
            "tags": config.tags or [],
            "concurrency_limit": config.concurrency_limit,
            "gpu_capable": config.gpu_capable,
        }
        
        response = self.client.post(f"{self.base_url}/api/v1/runners/register", json=body)
        response.raise_for_status()
        return response.json()

    def deregister_runner(self, slug: str) -> None:
        """Deregister a runner"""
        response = self.client.delete(f"{self.base_url}/api/v1/runners/{slug}")
        response.raise_for_status()

    def list_runners(self, filter_opts: Optional[RunnerFilter] = None) -> List[Dict[str, Any]]:
        """List all runners with optional filtering"""
        params = {}
        
        if filter_opts:
            if filter_opts.status:
                params["status"] = filter_opts.status
            if filter_opts.tags:
                params["tags"] = filter_opts.tags

        response = self.client.get(f"{self.base_url}/api/v1/runners", params=params)
        response.raise_for_status()
        return response.json()

    def get_runner(self, slug: str) -> Dict[str, Any]:
        """Get runner details"""
        response = self.client.get(f"{self.base_url}/api/v1/runners/{slug}")
        response.raise_for_status()
        return response.json()

    def send_command(self, req: CommandRequest) -> Dict[str, Any]:
        """Send a command to be executed"""
        body = {
            "target_type": req.target_type,
            "target_value": req.target_value,
            "payload": req.payload,
            "max_retries": req.max_retries,
            "timeout_secs": req.timeout_secs,
        }
        
        if req.deadline:
            body["deadline"] = req.deadline

        response = self.client.post(f"{self.base_url}/api/v1/commands", json=body)
        response.raise_for_status()
        return response.json()

    def get_command(self, command_id: str) -> Dict[str, Any]:
        """Get command details"""
        response = self.client.get(f"{self.base_url}/api/v1/commands/{command_id}")
        response.raise_for_status()
        return response.json()

    def kill_command(self, command_id: str) -> None:
        """Request to kill a running command"""
        response = self.client.post(f"{self.base_url}/api/v1/commands/{command_id}/kill")
        response.raise_for_status()

    def get_logs(
        self,
        command_id: str,
        page: int = 1,
        page_size: int = 100
    ) -> List[Dict[str, Any]]:
        """Get logs for a command"""
        params = {"page": page, "page_size": page_size}
        response = self.client.get(
            f"{self.base_url}/api/v1/commands/{command_id}/logs",
            params=params
        )
        response.raise_for_status()
        return response.json()

    def get_metrics(
        self,
        command_id: str,
        opts: Optional[MetricOpts] = None
    ) -> List[Dict[str, Any]]:
        """Get metrics for a command"""
        if opts is None:
            opts = MetricOpts()

        params = {
            "resolution": opts.resolution,
            "page": opts.page,
            "page_size": opts.page_size,
        }

        response = self.client.get(
            f"{self.base_url}/api/v1/commands/{command_id}/metrics",
            params=params
        )
        response.raise_for_status()
        return response.json()

    def watch_commands(self, slug: str) -> Iterator[Dict[str, Any]]:
        """Watch for command dispatch events via SSE"""
        url = f"{self.base_url}/api/v1/runners/{slug}/sse"
        
        headers = {}
        if self.token:
            headers["Authorization"] = f"Bearer {self.token}"

        while True:
            try:
                with httpx.stream("GET", url, headers=headers, timeout=None) as response:
                    response.raise_for_status()
                    client = sseclient.SSEClient(response)
                    
                    for event in client.events():
                        if event.event == "ping":
                            continue
                        
                        try:
                            data = json.loads(event.data)
                            yield {"type": event.event, "data": data}
                        except json.JSONDecodeError:
                            continue
                            
            except (httpx.HTTPError, Exception) as e:
                time.sleep(2)
                continue

    def watch_logs(self, command_id: str) -> Iterator[Dict[str, Any]]:
        """Poll and yield new logs as they arrive"""
        last_seq = 0
        
        while True:
            try:
                logs = self.get_logs(command_id, page=1, page_size=100)
                
                for log in logs:
                    if log["seq"] > last_seq:
                        yield log
                        last_seq = log["seq"]
                        
            except httpx.HTTPError:
                pass
            
            time.sleep(1)
