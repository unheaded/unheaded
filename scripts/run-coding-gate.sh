#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
#
# scripts/run-coding-gate.sh — execute the 14-prompt coding-gate eval.
#
# Reads eval/coding-gate/prompts.jsonl, calls bin/zhen-rag for each prompt
# with greedy decoding (temperature=0.0, k=5, max_tokens=600), appends each
# raw output to eval/coding-gate/results-<date>.md for hand-grading per
# eval/coding-gate/RUBRIC.md.
#
# Pre-requisites (per plan §3.6):
#   - cs serve at $VOR_URL (default 127.0.0.1:9876) with ~/.config/cs/sources/unheaded symlink
#   - llama-server at $LLAMA_URL (default 127.0.0.1:8081) with ctx-size 16384
#   - bin/zhen-rag built from current HEAD (`make build-zhen-rag`)
#
# Usage:
#   ./scripts/run-coding-gate.sh
#
# Output:
#   eval/coding-gate/results-$(date +%Y-%m-%d).md
#   (template at eval/coding-gate/RESULTS-TEMPLATE.md is copied first.)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

PROMPTS="eval/coding-gate/prompts.jsonl"
TEMPLATE="eval/coding-gate/RESULTS-TEMPLATE.md"
DATE="$(date +%Y-%m-%d)"
RESULTS="eval/coding-gate/results-${DATE}.md"
ZHEN_RAG="bin/zhen-rag"

VOR_URL="${VOR_URL:-http://127.0.0.1:9876}"
LLAMA_URL="${LLAMA_URL:-http://127.0.0.1:8081}"

# --- pre-flight ---

[[ -f "$PROMPTS" ]] || { echo "missing $PROMPTS" >&2; exit 1; }
[[ -f "$TEMPLATE" ]] || { echo "missing $TEMPLATE" >&2; exit 1; }
[[ -x "$ZHEN_RAG" ]] || { echo "missing $ZHEN_RAG — run 'make build-zhen-rag' first" >&2; exit 1; }

if ! curl -sf "$VOR_URL/api/categories" > /dev/null; then
    echo "vor unreachable at $VOR_URL — start it with 'cs serve'" >&2
    exit 1
fi
if ! curl -sf "$LLAMA_URL/health" > /dev/null; then
    echo "llama-server unreachable at $LLAMA_URL" >&2
    exit 1
fi

# --- copy template ---

cp "$TEMPLATE" "$RESULTS"
{
    echo ""
    echo "---"
    echo ""
    echo "## Run log"
    echo ""
    echo "- Started: $(date -Iseconds)"
    echo "- Binary: $ZHEN_RAG ($(git rev-parse --short HEAD 2>/dev/null || echo unknown))"
    echo "- VOR_URL: $VOR_URL"
    echo "- LLAMA_URL: $LLAMA_URL"
    echo ""
} >> "$RESULTS"

# --- run all 14 prompts ---

idx=0
while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    idx=$((idx + 1))

    id=$(echo "$line" | python3 -c 'import sys, json; print(json.loads(sys.stdin.read())["id"])')
    kind=$(echo "$line" | python3 -c 'import sys, json; print(json.loads(sys.stdin.read())["kind"])')
    lang=$(echo "$line" | python3 -c 'import sys, json; print(json.loads(sys.stdin.read())["language"])')
    prompt=$(echo "$line" | python3 -c 'import sys, json; print(json.loads(sys.stdin.read())["prompt"])')
    expected=$(echo "$line" | python3 -c 'import sys, json; d=json.loads(sys.stdin.read()); print(d.get("expected_flag",""))')

    echo "[$idx/14] $id ($lang/$kind)..." >&2

    {
        echo ""
        echo "---"
        echo ""
        echo "### $idx. \`$id\` ($lang/$kind)"
        echo ""
        echo "**Prompt:**"
        echo ""
        echo "$prompt"
        echo ""
        if [[ -n "$expected" ]]; then
            echo "**Expected flag (graders only):** $expected"
            echo ""
        fi
    } >> "$RESULTS"

    start=$(date +%s)
    out_tmp="$(mktemp)"
    err_tmp="$(mktemp)"
    if "$ZHEN_RAG" \
        -temperature 0 \
        -seed 42 \
        -k 5 \
        -max-tokens 600 \
        -show-context \
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
        echo "**Retrieved references (stderr):**"
        echo ""
        echo '```'
        cat "$err_tmp"
        echo '```'
        echo ""
        echo "**Model output:**"
        echo ""
        echo '```'
        cat "$out_tmp"
        echo '```'
        echo ""
    } >> "$RESULTS"

    rm -f "$out_tmp" "$err_tmp"
done < "$PROMPTS"

{
    echo ""
    echo "---"
    echo ""
    echo "- Completed: $(date -Iseconds)"
    echo ""
    echo "## Next step"
    echo ""
    echo "Hand-grade each prompt section per \`eval/coding-gate/RUBRIC.md\`,"
    echo "fill in the table at the top, write the verdict, commit."
} >> "$RESULTS"

echo ""
echo "Done. Output: $RESULTS"
