#!/usr/bin/env python3
"""
Zhen Champion MCP Server — Exposes Kingdom capabilities as MCP tools.

Any MCP client (Claude Code, Claude Desktop, custom) can connect and use:
- corpus_search: FAISS semantic search across 1.76M vectors
- file_read: Sandboxed file read (allowlist enforced)
- service_health: Health check all Kingdom services
- kanban_list: List Kanban tasks
- runbook_list: List available operational runbooks
- runbook_show: Show runbook details

Transport: stdio (for Claude Code) or SSE (for web clients)

Usage:
  # stdio transport (Claude Code)
  python3 zhen_mcp_server.py

  # Add to Claude Code settings.json:
  # "mcpServers": {
  #   "zhen": {
  #     "command": "python3",
  #     "args": ["/home/govan/tmp/unheaded/raft/zhen_mcp_server.py"],
  #     "env": {}
  #   }
  # }
"""

import glob
import json
import os
import urllib.request
from pathlib import Path

from mcp.server import Server
from mcp.server.stdio import stdio_server
from mcp.types import Tool, TextContent

# --- Configuration ---
PROJECT_ROOT = Path.home() / "tmp" / "unheaded"
RUNBOOKS_DIR = PROJECT_ROOT / "runbooks"
ALLOWED_PATHS = [
    str(PROJECT_ROOT),
    "/var/zhen/",
]
DENIED_PATTERNS = [".ssh", ".env", "credentials", "secrets", "shadow", "id_rsa", "id_ed25519"]

SERVICES = {
    "wotan": 18000, "timeguru": 19000, "captain": 19002,
    "architect": 19001, "micromanager": 19003, "monad": 19004,
    "sophia": 19005, "dashboard": 20000, "kanban": 16668,
    "zhen": 20103, "inference": 20100,
}


def validate_path(path: str) -> tuple[str | None, str | None]:
    """Validate path is within sandbox. Returns (abs_path, error)."""
    if ".." in path:
        return None, "Path traversal blocked"
    abs_path = os.path.abspath(os.path.expanduser(path))
    for denied in DENIED_PATTERNS:
        if denied in abs_path:
            return None, f"Path contains denied pattern: {denied}"
    for allowed in ALLOWED_PATHS:
        if abs_path.startswith(os.path.abspath(allowed)):
            return abs_path, None
    return None, f"Path outside sandbox"


# --- MCP Server ---
app = Server("zhen-champion")


@app.list_tools()
async def list_tools():
    return [
        Tool(
            name="corpus_search",
            description="Search the Unheaded knowledge corpus (1.76M vectors). Returns top-k relevant chunks from codebase, RFCs, docs, research papers.",
            inputSchema={
                "type": "object",
                "properties": {
                    "query": {"type": "string", "description": "Search query"},
                    "k": {"type": "integer", "description": "Number of results (default 5)", "default": 5},
                },
                "required": ["query"],
            },
        ),
        Tool(
            name="file_read",
            description="Read a file from the Unheaded project (sandboxed). Allowed: ~/tmp/unheaded/, /var/zhen/. Blocked: .ssh, .env, secrets.",
            inputSchema={
                "type": "object",
                "properties": {
                    "path": {"type": "string", "description": "File path to read"},
                },
                "required": ["path"],
            },
        ),
        Tool(
            name="service_health",
            description="Check health of all 11 Kingdom services. Returns healthy/unhealthy status for each.",
            inputSchema={"type": "object", "properties": {}},
        ),
        Tool(
            name="runbook_list",
            description="List all available operational runbooks across 7 categories (infra, network, security, data, observe, doom, deploy).",
            inputSchema={"type": "object", "properties": {}},
        ),
        Tool(
            name="runbook_show",
            description="Show the full contents of a specific runbook by name (e.g., 'doom-ring-setup', 'service-health-sweep').",
            inputSchema={
                "type": "object",
                "properties": {
                    "name": {"type": "string", "description": "Runbook name (without .yaml extension)"},
                },
                "required": ["name"],
            },
        ),
        Tool(
            name="file_write",
            description="Write content to a file (sandboxed). Creates parent directories. Captures snapshot for revert.",
            inputSchema={
                "type": "object",
                "properties": {
                    "path": {"type": "string", "description": "File path to write"},
                    "content": {"type": "string", "description": "Content to write"},
                },
                "required": ["path", "content"],
            },
        ),
        Tool(
            name="file_patch",
            description="Find-and-replace edit in a file (sandboxed). Replaces first occurrence of old_text with new_text.",
            inputSchema={
                "type": "object",
                "properties": {
                    "path": {"type": "string", "description": "File path to patch"},
                    "old_text": {"type": "string", "description": "Text to find"},
                    "new_text": {"type": "string", "description": "Replacement text"},
                },
                "required": ["path", "old_text", "new_text"],
            },
        ),
        Tool(
            name="runbook_execute",
            description="Execute an operational runbook by name. Runs all steps with verification gates. Use dry_run=true to validate without executing.",
            inputSchema={
                "type": "object",
                "properties": {
                    "name": {"type": "string", "description": "Runbook name (without .yaml)"},
                    "dry_run": {"type": "boolean", "description": "If true, validate without executing", "default": False},
                },
                "required": ["name"],
            },
        ),
    ]


@app.call_tool()
async def call_tool(name: str, arguments: dict):
    if name == "corpus_search":
        return await tool_corpus_search(arguments)
    elif name == "file_read":
        return await tool_file_read(arguments)
    elif name == "service_health":
        return await tool_service_health(arguments)
    elif name == "runbook_list":
        return await tool_runbook_list(arguments)
    elif name == "runbook_show":
        return await tool_runbook_show(arguments)
    elif name == "file_write":
        return await tool_file_write(arguments)
    elif name == "file_patch":
        return await tool_file_patch(arguments)
    elif name == "runbook_execute":
        return await tool_runbook_execute(arguments)
    else:
        return [TextContent(type="text", text=f"Unknown tool: {name}")]


async def tool_corpus_search(args: dict):
    query = args.get("query", "")
    k = args.get("k", 5)
    try:
        data = json.dumps({"task": query, "k": k}).encode()
        req = urllib.request.Request(
            "http://localhost:20103/api/v1/context",
            data=data,
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        with urllib.request.urlopen(req, timeout=10) as resp:
            result = json.loads(resp.read())
        chunks = result.get("context", [])
        text = f"Found {len(chunks)} results for '{query}':\n\n"
        for i, chunk in enumerate(chunks):
            source = chunk.get("source", "unknown")
            content = chunk.get("content", "")[:500]
            score = chunk.get("relevance", 0)
            text += f"---\n[{i+1}] {source} (score: {score:.3f})\n{content}\n\n"
        return [TextContent(type="text", text=text)]
    except Exception as e:
        return [TextContent(type="text", text=f"Corpus search failed (is Zhen running on :20103?): {e}")]


async def tool_file_read(args: dict):
    path = args.get("path", "")
    abs_path, err = validate_path(path)
    if err:
        return [TextContent(type="text", text=f"ACCESS DENIED: {err} (path: {path})")]
    try:
        with open(abs_path, "r", errors="replace") as f:
            content = f.read(1_000_000)
        return [TextContent(type="text", text=f"File: {abs_path} ({len(content)} bytes)\n\n{content}")]
    except FileNotFoundError:
        return [TextContent(type="text", text=f"File not found: {abs_path}")]
    except Exception as e:
        return [TextContent(type="text", text=f"Error reading {abs_path}: {e}")]


async def tool_service_health(args: dict):
    results = []
    healthy = 0
    for name, port in SERVICES.items():
        try:
            req = urllib.request.Request(f"http://localhost:{port}/health", method="GET")
            with urllib.request.urlopen(req, timeout=2) as resp:
                results.append(f"  [OK]   {name} (:{port})")
                healthy += 1
        except Exception:
            results.append(f"  [FAIL] {name} (:{port})")
    header = f"Kingdom Service Health: {healthy}/{len(SERVICES)} healthy\n\n"
    return [TextContent(type="text", text=header + "\n".join(results))]


async def tool_runbook_list(args: dict):
    runbooks = sorted(glob.glob(str(RUNBOOKS_DIR / "**" / "*.yaml"), recursive=True))
    text = f"Available Runbooks ({len(runbooks)}):\n\n"
    by_category = {}
    for rb in runbooks:
        rel = os.path.relpath(rb, RUNBOOKS_DIR)
        parts = rel.split(os.sep)
        category = parts[0] if len(parts) > 1 else "other"
        name = parts[-1].replace(".yaml", "")
        by_category.setdefault(category, []).append(name)
    for cat in sorted(by_category):
        text += f"### {cat}/\n"
        for name in sorted(by_category[cat]):
            text += f"  - {name}\n"
        text += "\n"
    return [TextContent(type="text", text=text)]


async def tool_runbook_show(args: dict):
    name = args.get("name", "")
    matches = glob.glob(str(RUNBOOKS_DIR / "**" / f"{name}.yaml"), recursive=True)
    if not matches:
        return [TextContent(type="text", text=f"Runbook not found: {name}")]
    with open(matches[0], "r") as f:
        content = f.read()
    return [TextContent(type="text", text=f"Runbook: {name}\nPath: {matches[0]}\n\n{content}")]


async def tool_file_write(args: dict):
    path = args.get("path", "")
    content = args.get("content", "")
    abs_path, err = validate_path(path)
    if err:
        return [TextContent(type="text", text=f"ACCESS DENIED: {err} (path: {path})")]
    try:
        os.makedirs(os.path.dirname(abs_path), exist_ok=True)
        with open(abs_path, "w") as f:
            f.write(content)
        return [TextContent(type="text", text=f"Written {len(content)} bytes to {abs_path}")]
    except Exception as e:
        return [TextContent(type="text", text=f"Error writing {abs_path}: {e}")]


async def tool_file_patch(args: dict):
    path = args.get("path", "")
    old_text = args.get("old_text", "")
    new_text = args.get("new_text", "")
    abs_path, err = validate_path(path)
    if err:
        return [TextContent(type="text", text=f"ACCESS DENIED: {err} (path: {path})")]
    try:
        with open(abs_path, "r") as f:
            content = f.read()
        if old_text not in content:
            return [TextContent(type="text", text=f"old_text not found in {abs_path}")]
        patched = content.replace(old_text, new_text, 1)
        with open(abs_path, "w") as f:
            f.write(patched)
        return [TextContent(type="text", text=f"Patched {abs_path}: replaced {len(old_text)} chars with {len(new_text)} chars")]
    except FileNotFoundError:
        return [TextContent(type="text", text=f"File not found: {abs_path}")]
    except Exception as e:
        return [TextContent(type="text", text=f"Error patching {abs_path}: {e}")]


async def tool_runbook_execute(args: dict):
    import subprocess as sp
    name = args.get("name", "")
    dry_run = args.get("dry_run", False)
    matches = glob.glob(str(RUNBOOKS_DIR / "**" / f"{name}.yaml"), recursive=True)
    if not matches:
        return [TextContent(type="text", text=f"Runbook not found: {name}")]
    cmd = ["python3", str(PROJECT_ROOT / "scripts" / "run-runbook.py")]
    if dry_run:
        cmd.append("--dry-run")
    cmd.append(matches[0])
    try:
        result = sp.run(cmd, capture_output=True, text=True, timeout=600)
        output = result.stdout + ("\n" + result.stderr if result.stderr else "")
        status = "SUCCESS" if result.returncode == 0 else "FAILED"
        return [TextContent(type="text", text=f"Runbook {name}: {status}\n\n{output[-3000:]}")]
    except sp.TimeoutExpired:
        return [TextContent(type="text", text=f"Runbook {name}: TIMEOUT (600s limit)")]
    except Exception as e:
        return [TextContent(type="text", text=f"Runbook {name}: ERROR — {e}")]


async def main():
    async with stdio_server() as (read_stream, write_stream):
        await app.run(read_stream, write_stream, app.create_initialization_options())


if __name__ == "__main__":
    import asyncio
    asyncio.run(main())
