#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
#
# scripts/coding-gate-probe.sh — multi-condition probe runner.
#
# Differs from run-coding-gate.sh: configurable via env vars so a single
# script can drive seed sweeps, system-prompt ablations, and baselines
# into separate output files.
#
# Env:
#   PROBE_NAME       — short label, used in output filename and inside doc
#   PROBE_SEED       — seed for llama-server (default 42)
#   PROBE_SYSTEM     — '' (default), '-' (no system msg), or path to file
#   PROBE_OUT        — output md path (default eval/coding-gate/probe-2026-05-02/<name>.md)
#   VOR_URL, LLAMA_URL — same as run-coding-gate.sh

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

PROBE_NAME="${PROBE_NAME:-default}"
PROBE_SEED="${PROBE_SEED:-42}"
PROBE_SYSTEM="${PROBE_SYSTEM:-}"
PROBE_OUT="${PROBE_OUT:-eval/coding-gate/probe-2026-05-02/${PROBE_NAME}.md}"
VOR_URL="${VOR_URL:-http://127.0.0.1:9876}"
LLAMA_URL="${LLAMA_URL:-http://127.0.0.1:8081}"

PROMPTS="eval/coding-gate/prompts.jsonl"
ZHEN_RAG="bin/zhen-rag"

[[ -f "$PROMPTS" ]] || { echo "missing $PROMPTS" >&2; exit 1; }
[[ -x "$ZHEN_RAG" ]] || { echo "missing $ZHEN_RAG — run 'make build-zhen-rag' first" >&2; exit 1; }
mkdir -p "$(dirname "$PROBE_OUT")"

# --- audit metadata ---

GGUF_PATH="/var/zhen/models/qwen2.5-coder-7b-instruct-q4_k_m.gguf"
GGUF_HASH="$(sha256sum "$GGUF_PATH" 2>/dev/null | cut -d' ' -f1 || echo unknown)"
SOURCES_FP="$(ls -la ~/.config/cs/sources/ 2>/dev/null | tail -n +2 | sha256sum | cut -d' ' -f1)"
RUN_UID="$(date -u +%Y%m%dT%H%M%SZ)-${PROBE_NAME}-seed${PROBE_SEED}"

cat > "$PROBE_OUT" <<EOF
# Coding-Gate Probe — ${PROBE_NAME}

**Run UID:** \`${RUN_UID}\`
**Date:** $(date -Iseconds)
**Probe label:** \`${PROBE_NAME}\`
**Seed:** ${PROBE_SEED}
**System-prompt source:** ${PROBE_SYSTEM:-default} (empty=default, '-'=none, path=file)
**Binary:** ${ZHEN_RAG} ($(git rev-parse --short HEAD 2>/dev/null || echo unknown))
**GGUF SHA-256:** \`${GGUF_HASH}\`
**vor sources fingerprint:** \`${SOURCES_FP}\`
**VOR_URL:** ${VOR_URL}
**LLAMA_URL:** ${LLAMA_URL}

---

EOF

# --- run all 14 prompts ---

idx=0
while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    idx=$((idx + 1))

    id=$(echo "$line" | python3 -c 'import sys, json; print(json.loads(sys.stdin.read())["id"])')
    kind=$(echo "$line" | python3 -c 'import sys, json; print(json.loads(sys.stdin.read())["kind"])')
    lang=$(echo "$line" | python3 -c 'import sys, json; print(json.loads(sys.stdin.read())["language"])')
    prompt=$(echo "$line" | python3 -c 'import sys, json; print(json.loads(sys.stdin.read())["prompt"])')

    echo "[$idx/14] $id ($lang/$kind) [seed=$PROBE_SEED, sys=${PROBE_SYSTEM:-default}]..." >&2

    {
        echo "### $idx. \`$id\` ($lang/$kind)"
        echo ""
    } >> "$PROBE_OUT"

    start=$(date +%s)
    out_tmp="$(mktemp)"

    if [[ "$PROBE_SYSTEM" == "" ]]; then
        sysflag=""
    else
        sysflag="-system-prompt-file=$PROBE_SYSTEM"
    fi

    if "$ZHEN_RAG" \
        -temperature 0 \
        -seed "$PROBE_SEED" \
        -k 5 \
        -max-tokens 600 \
        $sysflag \
        -q "$prompt" \
        > "$out_tmp" 2>/dev/null; then
        status="ok"
    else
        status="FAILED (rc=$?)"
    fi
    end=$(date +%s)
    dur=$((end - start))

    {
        echo "**Latency:** ${dur}s · status: $status"
        echo ""
        echo '```'
        cat "$out_tmp"
        echo '```'
        echo ""
    } >> "$PROBE_OUT"

    rm -f "$out_tmp"
done < "$PROMPTS"

{
    echo "---"
    echo "- Completed: $(date -Iseconds)"
} >> "$PROBE_OUT"

echo "Done. Output: $PROBE_OUT"
