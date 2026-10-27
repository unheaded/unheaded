#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# zhen-mcp.sh — start Zhen's MCP server (bin/zhen-mcp, cmd/zhen-mcp) on stdio.
#
# The launcher .mcp.json points Claude Code at. It runs the Go binary built
# by `make build-zhen-mcp` and fails loudly, naming that target, when the
# binary is missing. The Python server it used to run was retired in
# S-CHAMPION-HARDENING Phase 10.
#
# The server runs read-only unless the environment already carries
# ZHEN_MCP_ALLOW_WRITE=1 / ZHEN_MCP_ALLOW_EXEC=1; this script sets neither.
# ZHEN_MCP_PROJECT_ROOT defaults to this checkout.
# stdout is the MCP channel: everything this script says goes to stderr.
#
# Smoke test (initialize, then list tools): scripts/check-zhen-mcp.sh

set -euo pipefail

src="${BASH_SOURCE[0]}"
[[ "$src" == */* ]] || src="./$src"
here="$(cd "${src%/*}" && pwd -P)"
root="$(cd "$here/.." && pwd -P)"
bin="$root/bin/zhen-mcp"

if [[ ! -x "$bin" ]]; then
    {
        echo "zhen-mcp: $bin is missing or not executable"
        echo "zhen-mcp: build it with: make -C $root build-zhen-mcp"
    } >&2
    exit 1
fi

export ZHEN_MCP_PROJECT_ROOT="${ZHEN_MCP_PROJECT_ROOT:-$root}"
exec "$bin"
