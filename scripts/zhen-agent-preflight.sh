#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
#
# scripts/zhen-agent-preflight.sh — checks that the prerequisites for
# running zhen-agent are satisfied. Designed to be invoked by
# `make zhen-agent-up` before launching anything; prints a friendly
# diagnostic and exits non-zero if any check fails.
#
# Checks:
#   1. bin/zhen-agent exists and is executable
#   2. vor (cs serve) is reachable at $VOR_URL
#   3. llama-server is reachable at $LLAMA_URL with /health = 200
#   4. ~/.config/cs/sources/ exists (Phase A source-discovery dir)
#   5. The local cs/vor binary supports source-provenance schema
#      (returns source_kind on /api/topics/<known> — fails gracefully
#      if vor is the older pre-B1 build)
#
# Env (defaults match cmd/zhen-agent):
#   VOR_URL    (http://127.0.0.1:9876)
#   LLAMA_URL  (http://127.0.0.1:8081)

set -euo pipefail

VOR_URL="${VOR_URL:-http://127.0.0.1:9876}"
LLAMA_URL="${LLAMA_URL:-http://127.0.0.1:8081}"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

fail=0
warn=0

check() {
    local label="$1" cmd="$2"
    if eval "$cmd" >/dev/null 2>&1; then
        printf "  ✅ %s\n" "$label"
    else
        printf "  ❌ %s\n" "$label"
        fail=$((fail + 1))
    fi
}

check_warn() {
    local label="$1" cmd="$2" remediation="$3"
    if eval "$cmd" >/dev/null 2>&1; then
        printf "  ✅ %s\n" "$label"
    else
        printf "  ⚠  %s\n" "$label"
        printf "       remediation: %s\n" "$remediation"
        warn=$((warn + 1))
    fi
}

echo "zhen-agent preflight — verifying prerequisites"
echo ""
echo "Binary:"
check "bin/zhen-agent built (run \`make build-zhen-agent\` if missing)" \
    "[[ -x bin/zhen-agent ]]"

echo ""
echo "Backends:"
check "vor reachable at $VOR_URL" \
    "curl -sf -o /dev/null --max-time 3 $VOR_URL/api/health"
check "llama-server reachable at $LLAMA_URL" \
    "curl -sf -o /dev/null --max-time 3 $LLAMA_URL/health"

echo ""
echo "Configuration:"
check_warn "~/.config/cs/sources/ exists (Phase A symlink discovery)" \
    "[[ -d $HOME/.config/cs/sources ]]" \
    "mkdir -p ~/.config/cs/sources && ln -s /home/govan/tmp/unheaded ~/.config/cs/sources/unheaded"

# Source-provenance schema check: query a known canonical topic and
# look for the source_kind field.
if [[ $fail -eq 0 ]]; then
    if curl -sf --max-time 3 "$VOR_URL/api/topics/bash" 2>/dev/null \
        | grep -q '"source_kind"'; then
        printf "  ✅ vor supports source-provenance schema (B1)\n"
    else
        printf "  ⚠  vor does NOT expose source_kind — running pre-B1 build\n"
        printf "       remediation: rebuild cs from harden/api-dos-and-traversal branch\n"
        warn=$((warn + 1))
    fi
fi

echo ""
if [[ $fail -gt 0 ]]; then
    echo "❌ $fail prerequisite(s) failed. Fix and re-run."
    exit 1
fi
if [[ $warn -gt 0 ]]; then
    echo "⚠  $warn warning(s). Agent will run but with reduced capability."
    exit 0
fi
echo "✅ All preflight checks passed."
