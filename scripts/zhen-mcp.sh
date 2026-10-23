#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# zhen-mcp.sh — start Zhen's MCP server (raft/zhen_mcp_server.py) on stdio.
#
# The launcher .mcp.json points Claude Code at. It picks a Python that can
# import the `mcp` package, in this order, and fails loudly if none can:
#
#   1. $ZHEN_MCP_PYTHON           (pin one explicitly)
#   2. ~/.venv/zhen/bin/python    (the Zhen venv on west)
#   3. python3 on PATH
#
# The server runs read-only unless the environment already carries
# ZHEN_MCP_ALLOW_WRITE=1 / ZHEN_MCP_ALLOW_EXEC=1; this script sets neither.
# stdout is the MCP channel: everything this script says goes to stderr.
#
# Smoke test (initialize, then list tools):
#   printf '%s\n' \
#     '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}' \
#     '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
#     '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' | scripts/zhen-mcp.sh

set -euo pipefail

src="${BASH_SOURCE[0]}"
[[ "$src" == */* ]] || src="./$src"
here="$(cd "${src%/*}" && pwd -P)"
server="$here/../raft/zhen_mcp_server.py"
if [[ ! -f "$server" ]]; then
    echo "zhen-mcp: server not found at $server" >&2
    exit 1
fi

candidates=()
if [[ -n "${ZHEN_MCP_PYTHON:-}" ]]; then
    candidates+=("$ZHEN_MCP_PYTHON")
fi
if [[ -n "${HOME:-}" ]]; then
    candidates+=("$HOME/.venv/zhen/bin/python")
fi
candidates+=("python3")

# -I (isolated): no PYTHONPATH, no user site, no script-dir imports from
# the caller's cwd. The venv's own site-packages still load.
for py in "${candidates[@]}"; do
    if command -v "$py" >/dev/null 2>&1 \
        && "$py" -I -c 'import mcp.server.stdio' >/dev/null 2>&1; then
        exec "$py" -I "$server"
    fi
done

{
    echo "zhen-mcp: no Python with the 'mcp' package. Tried: ${candidates[*]}"
    echo "zhen-mcp: install it into a venv (pin in raft/requirements.txt) and set ZHEN_MCP_PYTHON=/path/to/venv/bin/python"
} >&2
exit 1
