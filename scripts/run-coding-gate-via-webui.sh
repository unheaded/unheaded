#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
#
# scripts/run-coding-gate-via-webui.sh — same gate, same fixture,
# same RUBRIC, but routes each prompt through the rewired Python
# /api/v1/query endpoint (port 20103) instead of bin/zhen-rag direct.
#
# Used to verify H0 non-regression at the end of WAVE15 Phase 1
# (and again at Phase 2 / Phase 4).
#
# Pre-requisites:
#   - vor at $VOR_URL    (default 127.0.0.1:9876)
#   - llama-server at $LLAMA_URL (default 127.0.0.1:8081)
#   - raft/zhen_app.py running on $WEBUI_URL (default 127.0.0.1:20103)
#
# Usage:
#   ./scripts/run-coding-gate-via-webui.sh
#
# Output:
#   eval/coding-gate/results-via-webui-$(date +%Y-%m-%d).md

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

PROMPTS="eval/coding-gate/prompts.jsonl"
TEMPLATE="eval/coding-gate/RESULTS-TEMPLATE.md"
WEBUI_URL="${WEBUI_URL:-http://127.0.0.1:20103}"
DATE="$(date +%Y-%m-%d)"
OUT="eval/coding-gate/results-via-webui-phase1-${DATE}.md"

[[ -f "$PROMPTS" ]] || { echo "missing $PROMPTS" >&2; exit 1; }
[[ -f "$TEMPLATE" ]] || { echo "missing $TEMPLATE" >&2; exit 1; }

# Probe the webapp first.
if ! curl -sf --max-time 3 "$WEBUI_URL/health" > /dev/null; then
    echo "webapp not reachable at $WEBUI_URL/health" >&2
    exit 1
fi

cp "$TEMPLATE" "$OUT"

# Replace template placeholders with actual run values.
SHORT_SHA="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
sed -i "s|YYYY-MM-DD|${DATE}|g; s|&lt;short-sha&gt;|${SHORT_SHA}|g; s|&lt;url&gt;|${WEBUI_URL}|g" "$OUT"

echo "" >> "$OUT"
echo "## Run log" >> "$OUT"
echo "" >> "$OUT"
echo "- Started: $(date -Iseconds)" >> "$OUT"
echo "- Target: rewired Python webui at ${WEBUI_URL}/api/v1/query" >> "$OUT"
echo "- Underlying inference: llama-server (qwen-coder-7b) via zhen_rag.py" >> "$OUT"
echo "- Underlying retrieval: vor at \$VOR_URL" >> "$OUT"
echo "- Decoding: temperature=0.0, seed=42, max_tokens=600 (zhen_rag.py defaults)" >> "$OUT"
echo "- WAVE15 Phase 1 H0 verification" >> "$OUT"
echo "" >> "$OUT"
echo "---" >> "$OUT"
echo "" >> "$OUT"
echo "## Raw outputs" >> "$OUT"
echo "" >> "$OUT"

n=0
total="$(wc -l < "$PROMPTS")"
while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    n=$((n + 1))
    id=$(printf '%s' "$line" | python3 -c "import sys,json; print(json.loads(sys.stdin.read())['id'])")
    lang=$(printf '%s' "$line" | python3 -c "import sys,json; print(json.loads(sys.stdin.read())['language'])")
    kind=$(printf '%s' "$line" | python3 -c "import sys,json; print(json.loads(sys.stdin.read())['kind'])")
    prompt=$(printf '%s' "$line" | python3 -c "import sys,json; print(json.loads(sys.stdin.read())['prompt'])")
    expected=$(printf '%s' "$line" | python3 -c "import sys,json; d=json.loads(sys.stdin.read()); print(d.get('expected_flag',''))")
    echo "[${n}/${total}] ${id} (${lang}/${kind})..."

    # Build request with deterministic decoding.
    payload=$(python3 -c "
import json, sys
print(json.dumps({
    'question': sys.argv[1],
    'session_id': f'gate-{sys.argv[2]}',
}))
" "$prompt" "$id")

    start=$(date +%s)
    response=$(curl -s --max-time 180 -X POST "$WEBUI_URL/api/v1/query" \
        -H 'Content-Type: application/json' \
        -d "$payload")
    elapsed=$(( $(date +%s) - start ))

    answer=$(printf '%s' "$response" | python3 -c "
import sys, json
try:
    d = json.loads(sys.stdin.read())
    print(d.get('answer', '(no answer field)'))
except Exception as e:
    print(f'(error decoding response: {e})')
")
    sources=$(printf '%s' "$response" | python3 -c "
import sys, json
try:
    d = json.loads(sys.stdin.read())
    srcs = d.get('sources', [])
    print(', '.join(s.get('id','?') for s in srcs[:5]))
except Exception:
    print('(none)')
")
    matched_mem=$(printf '%s' "$response" | python3 -c "
import sys, json
try:
    d = json.loads(sys.stdin.read())
    mm = d.get('matched_memory')
    print('none' if mm is None else f\"sim={mm.get('similarity','?'):.3f}\")
except Exception:
    print('?')
")

    {
        echo "### ${n}. \`${id}\` (${lang}/${kind})"
        echo ""
        echo "**Prompt:**"
        echo ""
        printf '%s\n' "$prompt"
        echo ""
        echo "**Expected (review prompts only):** ${expected:-(no flag — syntax)}"
        echo ""
        echo "**Latency:** ${elapsed}s · status: ok"
        echo ""
        echo "**Retrieved (top-5 vor topic IDs):** ${sources}"
        echo ""
        echo "**Matched memory:** ${matched_mem}"
        echo ""
        echo "**Model output:**"
        echo ""
        echo '```'
        printf '%s\n' "$answer"
        echo '```'
        echo ""
    } >> "$OUT"
done < "$PROMPTS"

echo ""
echo "Done. Output: $OUT"
echo "Now hand-grade per eval/coding-gate/RUBRIC.md and compare to"
echo "baseline at eval/coding-gate/baseline-direct-cmd-zhen-rag-2026-05-02.md"
