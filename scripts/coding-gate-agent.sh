#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
#
# scripts/coding-gate-agent.sh — runs the 14-prompt coding-gate fixture
# through zhen-agent (Phase D-A) instead of zhen-rag (Phase B).
#
# Validates that the agent runtime preserves the H1 verdict zhen-rag
# achieved on D-pre. Differences:
#   - zhen-rag: free-text prompt → free-text answer
#   - zhen-agent: free-text prompt → JSON {"thought","answer"} → answer
#                 field extracted
#
# Env (same as coding-gate-probe.sh):
#   PROBE_NAME, PROBE_SEED, PROBE_OUT, VOR_URL, LLAMA_URL

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

PROBE_NAME="${PROBE_NAME:-agent-default}"
PROBE_SEED="${PROBE_SEED:-42}"
PROBE_OUT="${PROBE_OUT:-eval/coding-gate/probe-2026-05-02/${PROBE_NAME}.md}"
VOR_URL="${VOR_URL:-http://127.0.0.1:9876}"
LLAMA_URL="${LLAMA_URL:-http://127.0.0.1:8081}"

PROMPTS="eval/coding-gate/prompts.jsonl"
ZHEN_AGENT="bin/zhen-agent"

[[ -f "$PROMPTS" ]] || { echo "missing $PROMPTS" >&2; exit 1; }
[[ -x "$ZHEN_AGENT" ]] || { echo "missing $ZHEN_AGENT — run 'make build-zhen-agent'" >&2; exit 1; }
mkdir -p "$(dirname "$PROBE_OUT")"

GGUF_PATH="/var/zhen/models/qwen2.5-coder-7b-instruct-q4_k_m.gguf"
GGUF_HASH="$(sha256sum "$GGUF_PATH" 2>/dev/null | cut -d' ' -f1 || echo unknown)"
RUN_UID="$(date -u +%Y%m%dT%H%M%SZ)-${PROBE_NAME}-seed${PROBE_SEED}"

cat > "$PROBE_OUT" <<EOF
# Coding-Gate (Agent) — ${PROBE_NAME}

**Run UID:** \`${RUN_UID}\`
**Date:** $(date -Iseconds)
**Probe label:** \`${PROBE_NAME}\`
**Backend:** zhen-agent (pkg/agent ReAct loop + Champion gate)
**Seed:** ${PROBE_SEED}
**Binary:** ${ZHEN_AGENT} ($(git rev-parse --short HEAD 2>/dev/null || echo unknown))
**GGUF SHA-256:** \`${GGUF_HASH}\`
**Decoding:** temperature=0, k=5, max_tokens=400, max_turns=3
**Note:** answer field extracted from agent's terminal turn; raw trace
also captured.

---

EOF

idx=0
while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    idx=$((idx + 1))

    id=$(echo "$line" | python3 -c 'import sys, json; print(json.loads(sys.stdin.read())["id"])')
    kind=$(echo "$line" | python3 -c 'import sys, json; print(json.loads(sys.stdin.read())["kind"])')
    lang=$(echo "$line" | python3 -c 'import sys, json; print(json.loads(sys.stdin.read())["language"])')
    prompt=$(echo "$line" | python3 -c 'import sys, json; print(json.loads(sys.stdin.read())["prompt"])')

    echo "[$idx/14+14] $id ($lang/$kind) [seed=$PROBE_SEED]..." >&2

    {
        echo "### $idx. \`$id\` ($lang/$kind)"
        echo ""
    } >> "$PROBE_OUT"

    start=$(date +%s)
    out_tmp="$(mktemp)"
    err_tmp="$(mktemp)"

    if "$ZHEN_AGENT" \
        -temperature 0 \
        -seed "$PROBE_SEED" \
        -k 5 \
        -max-tokens 400 \
        -max-turns 3 \
        -project-root "$REPO_ROOT" \
        -show-trace \
        -q "$prompt" \
        > "$out_tmp" 2> "$err_tmp"; then
        status="ok"
    else
        status="FAILED (rc=$?)"
    fi
    end=$(date +%s)
    dur=$((end - start))

    {
        echo "**Latency:** ${dur}s · status: $status"
        echo ""
        echo "**Agent trace (stderr):**"
        echo ""
        echo '```'
        # Filter [champion] log lines + just keep agent trace
        grep -E "^\[turn|^─── |^         " "$err_tmp" | head -40
        echo '```'
        echo ""
        echo "**Agent answer (stdout):**"
        echo ""
        echo '```'
        cat "$out_tmp"
        echo '```'
        echo ""
    } >> "$PROBE_OUT"

    rm -f "$out_tmp" "$err_tmp"
done < "$PROMPTS"

{
    echo "---"
    echo "- Completed: $(date -Iseconds)"
} >> "$PROBE_OUT"

echo "Done. Output: $PROBE_OUT"
