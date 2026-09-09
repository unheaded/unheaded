#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
#
# WAVE15 Phase 3 — H2 hypothesis: conversation memory measurably improves
# answer quality on context-dependent prompts.
#
# A/B protocol:
#   For each of 30 prompts in eval/memory-ablation/prompts.jsonl, prime
#   the conversation history with a "prior" exchange (where one is
#   given), then ask the "prompt" follow-up. Run twice:
#     A: memory-recall ENABLED  (default ZHEN_RECALL_DISPLAY=on)
#     B: memory-recall DISABLED (ZHEN_RECALL_DISPLAY=off)
#
# Outputs eval/memory-ablation/results-<date>.md with both runs side-by-
# side per prompt for hand-grading. Cohen's d on the resulting 1-5
# usefulness scores is the H2 metric.
#
# Pass: d >= 0.5 (medium effect) → ship memory recall enabled
# Fail: d <  0.5 → ship recall disabled (writes still happen for audit)
#
# Pre-requisites:
#   - vor on :9876
#   - llama-server on :8081
#   - raft/zhen_app.py running on :20103
#   - The Well reachable (PG) for memory persistence
#
# Usage:
#   ./eval/memory-ablation/run.sh
#
# Output:
#   eval/memory-ablation/results-$(date +%Y-%m-%d).md
#
# Skips gracefully when the webapp or PG is unreachable.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

WEBUI_URL="${ZHEN_WEBUI_URL:-http://127.0.0.1:20103}"
PROMPTS="eval/memory-ablation/prompts.jsonl"
DATE="$(date +%Y-%m-%d)"
OUT="eval/memory-ablation/results-${DATE}.md"

# Pre-flight
if ! curl -sf --max-time 3 "$WEBUI_URL/health" > /dev/null; then
    echo "webapp not reachable at $WEBUI_URL/health" >&2
    exit 2  # 2 = "skipped, not failed" per CI convention
fi

if [[ ! -f "$PROMPTS" ]]; then
    echo "fixture missing: $PROMPTS" >&2
    exit 1
fi

NUM_PROMPTS=$(grep -c '^{' "$PROMPTS" || echo 0)
echo "prompts: $NUM_PROMPTS"

# Header
cat > "$OUT" <<EOF
# Memory Ablation Results — ${DATE}

**Hypothesis (H2):** Conversation memory measurably improves answer quality on context-dependent prompts.

**Test:** Each prompt is run TWICE through raft/zhen_app.py at \`${WEBUI_URL}/api/v1/query\`.

- **Run A (memory ON):** the matched_memory side-channel is included in the response.
- **Run B (memory OFF):** identical request but with \`ZHEN_RECALL_DISPLAY=off\` simulated by a separate session and no priming.

**Pass threshold:** Cohen's d ≥ 0.5 (medium effect), p < 0.05. Computed by hand-grading each pair on a 1-5 usefulness scale.

**Underlying inference:** llama-server qwen-coder-7b at :8081 (chat path direct per WAVE15 Phase 2b).
**Underlying retrieval:** vor at :9876.

---

## Per-prompt A/B answers

| ID | Kind | Run A (memory ON) | Run B (memory OFF) | A grade | B grade | Δ |
|----|------|-------------------|--------------------|---------|---------|----|
EOF

# Run each prompt twice with different session_ids; the "memory ON"
# session retains prior history across the priming + follow-up turns,
# while the "memory OFF" session asks the follow-up cold.
n=0
while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    n=$((n + 1))
    id=$(printf '%s' "$line"     | python3 -c "import sys,json; print(json.loads(sys.stdin.read())['id'])")
    kind=$(printf '%s' "$line"   | python3 -c "import sys,json; print(json.loads(sys.stdin.read())['kind'])")
    prior=$(printf '%s' "$line"  | python3 -c "import sys,json; print(json.loads(sys.stdin.read())['prior'])")
    prompt=$(printf '%s' "$line" | python3 -c "import sys,json; print(json.loads(sys.stdin.read())['prompt'])")

    echo "[$n/$NUM_PROMPTS] $id ($kind)..." >&2

    # Run A: prime the session with the prior turn (if any), then ask the
    # follow-up. The webapp's _add_to_history makes prior turns part of
    # subsequent /api/v1/query context.
    SESSION_A="ablation-on-$id"
    if [[ -n "$prior" ]]; then
        # Split the prior into Q and A so we can prime both sides of the
        # conversation history. Using a single "Q: ... A: ..." prompt
        # would conflate them; we want the model to see real turns.
        prior_q=$(printf '%s' "$prior" | sed -n 's/^Q: \(.*\)/\1/p' | head -1)
        if [[ -n "$prior_q" ]]; then
            curl -s --max-time 60 -X POST "$WEBUI_URL/api/v1/query" \
                -H 'Content-Type: application/json' \
                -d "$(python3 -c "import json,sys; print(json.dumps({'question': sys.argv[1], 'session_id': sys.argv[2]}))" "$prior_q" "$SESSION_A")" \
                > /dev/null
        fi
    fi
    answer_A=$(curl -s --max-time 120 -X POST "$WEBUI_URL/api/v1/query" \
        -H 'Content-Type: application/json' \
        -d "$(python3 -c "import json,sys; print(json.dumps({'question': sys.argv[1], 'session_id': sys.argv[2]}))" "$prompt" "$SESSION_A")" \
        | python3 -c "import sys,json; d=json.loads(sys.stdin.read()); print(d.get('answer','(none)'))")

    # Run B: cold session, no prior. Same prompt, but the model has zero
    # context — it must answer from training knowledge alone.
    SESSION_B="ablation-off-$id"
    answer_B=$(curl -s --max-time 120 -X POST "$WEBUI_URL/api/v1/query" \
        -H 'Content-Type: application/json' \
        -d "$(python3 -c "import json,sys; print(json.dumps({'question': sys.argv[1], 'session_id': sys.argv[2]}))" "$prompt" "$SESSION_B")" \
        | python3 -c "import sys,json; d=json.loads(sys.stdin.read()); print(d.get('answer','(none)'))")

    # Append to the markdown table. Truncate each answer to 140 chars
    # for the table; full answers go in the appendix below.
    answer_A_short=$(printf '%s' "$answer_A" | tr '\n' ' ' | head -c 140)
    answer_B_short=$(printf '%s' "$answer_B" | tr '\n' ' ' | head -c 140)
    printf '| %s | %s | %s | %s | _ | _ | _ |\n' "$id" "$kind" "$answer_A_short" "$answer_B_short" >> "$OUT"

    # And append full bodies to the appendix.
    {
        echo ""
        echo "<!-- FULL: $id -->"
        echo ""
        echo "### $id ($kind)"
        echo ""
        if [[ -n "$prior" ]]; then
            echo "**Prior context:**"
            echo ""
            echo '```'
            printf '%s\n' "$prior"
            echo '```'
            echo ""
        fi
        echo "**Prompt:**"
        echo ""
        printf '> %s\n' "$prompt"
        echo ""
        echo "**Run A (memory ON):**"
        echo ""
        echo '```'
        printf '%s\n' "$answer_A"
        echo '```'
        echo ""
        echo "**Run B (memory OFF):**"
        echo ""
        echo '```'
        printf '%s\n' "$answer_B"
        echo '```'
    } >> "$OUT"
done < "$PROMPTS"

# Footer for hand-grading + d-computation instructions.
cat >> "$OUT" <<EOF

---

## Hand-grading

Score each Run A and Run B answer on a 1-5 usefulness scale:

  1: useless or wrong
  2: weakly relevant, missing the point
  3: relevant but generic — would be the same with no context
  4: solidly answered, used context where appropriate
  5: precisely answered the contextual follow-up

Fill in the "A grade" and "B grade" columns of the table above. Compute
the per-prompt delta (B-grade − A-grade), then aggregate Cohen's d:

    d = (mean_A − mean_B) / pooled_stdev

  pooled_stdev = sqrt(((n_A−1)·var_A + (n_B−1)·var_B) / (n_A + n_B − 2))

For 30 prompts, 1-5 ordinal scale: typical pooled stdev ≈ 1.0-1.2.

  d ≥ 0.5 (medium): keep memory recall ON in production
  d < 0.5: ship memory in store-only mode (writes happen, recall display disabled)

Independent two-sample t-test for p < 0.05 if you want statistical
rigor; for n=30 per arm and d=0.5, t ≈ 1.93 → p ≈ 0.06 (one-tailed).
n=30 is undersized for tight CIs; the effect-size threshold is the
load-bearing decision criterion.

---

## Verdict

**Cohen's d:** _ (computed)
**p-value:** _ (computed)
**Decision:** _ (one of: keep-recall-on / disable-recall-display / re-run-with-larger-n)

**Justification:** _ (1-2 sentences)

**Phase 3 acceptance:** H2 produces a number. H2-pass keeps recall on; H2-fail does not block ship — it flips a config and we move forward.
EOF

echo ""
echo "Done: $OUT"
echo "Now hand-grade per the instructions at the bottom of the file."
