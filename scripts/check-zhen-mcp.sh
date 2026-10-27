#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# check-zhen-mcp.sh — the MCP stdio handshake Claude Code performs, against
# a fresh build of cmd/zhen-mcp (S-CHAMPION-HARDENING Step 201).
#
# Builds the server into a temp dir (bin/ is left alone), sends initialize,
# tools/list and two calls that must be refused, and fails unless:
#   - initialize answers with the requested protocol revision,
#   - tools/list shows exactly the six read-only tools (no ZHEN_MCP_ALLOW_*),
#   - file_read of /etc/shadow is an isError result (ACCESS DENIED),
#   - file_write is an isError result (disabled without the flag).
# No model, no network, no port; a few seconds. Needs go and jq.

set -euo pipefail
cd "$(dirname "$0")/.."
root="$(pwd -P)"

command -v go >/dev/null || { echo "check-zhen-mcp: go not found" >&2; exit 2; }
command -v jq >/dev/null || { echo "check-zhen-mcp: jq not found" >&2; exit 2; }

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
go build -o "$tmp/zhen-mcp" ./cmd/zhen-mcp

cat >"$tmp/in.jsonl" <<'JSON'
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"check-zhen-mcp","version":"0"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/list"}
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"file_read","arguments":{"path":"/etc/shadow"}}}
{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"file_write","arguments":{"path":"/tmp/check-zhen-mcp","content":"x"}}}
JSON

# The env is cleared of the opt-ins and of anything that points at a live
# service: local_llm and corpus_search are not called, but nothing here
# should depend on the caller's shell.
env -u ZHEN_MCP_ALLOW_WRITE -u ZHEN_MCP_ALLOW_EXEC -u ZHEN_CAPTURE_DIR \
    ZHEN_MCP_PROJECT_ROOT="$root" \
    timeout 10 "$tmp/zhen-mcp" <"$tmp/in.jsonl" >"$tmp/out.jsonl" 2>"$tmp/err.log" || {
    echo "FAIL: zhen-mcp exited non-zero" >&2
    tail -5 "$tmp/err.log" >&2
    exit 1
}

fail=0
check() { # check <description> <jq filter that must print true>
    if [[ "$(jq -s "$2" "$tmp/out.jsonl")" != "true" ]]; then
        echo "FAIL: $1" >&2
        fail=1
    else
        echo "ok   $1"
    fi
}
check "four replies (the notification gets none)" 'length == 4'
check "initialize echoes 2025-06-18" '.[] | select(.id == 1) | .result.protocolVersion == "2025-06-18"'
check "tools/list = the six read-only tools" \
    '[.[] | select(.id == 2) | .result.tools[].name] | sort == ["corpus_search","file_read","local_llm","runbook_list","runbook_show","service_health"]'
check "file_read /etc/shadow is isError ACCESS DENIED" \
    '.[] | select(.id == 3) | .result.isError == true and (.result.content[0].text | startswith("ACCESS DENIED"))'
check "file_write without the flag is isError" \
    '.[] | select(.id == 4) | .result.isError == true and (.result.content[0].text | contains("disabled"))'

if [[ -e /tmp/check-zhen-mcp ]]; then
    echo "FAIL: a disabled file_write wrote /tmp/check-zhen-mcp" >&2
    fail=1
fi
if (( fail )); then
    echo "--- replies ---" >&2
    cat "$tmp/out.jsonl" >&2
    exit 1
fi
echo "zhen-mcp handshake ok"
