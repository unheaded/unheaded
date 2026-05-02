#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
#
# scripts/coding-gate-ci.sh — 5-seed CI gate for the coding-gate eval.
#
# Per eval/coding-gate/CONTRIBUTING.md "CI gate (5-seed sweep)" — before
# merging any change to cmd/zhen-rag/main.go, scripts/run-coding-gate.sh,
# eval/coding-gate/RUBRIC.md, or eval/coding-gate/prompts.jsonl, run this
# script. It executes the 14-prompt fixture across 5 fixed seeds and
# auto-grades each, printing a variance band.
#
# A PR that produces a >2-prompt swing in the verdict band requires a
# probe-results doc explaining why the change is intentional.
#
# Output:
#   eval/coding-gate/ci-runs/<utc-timestamp>/seed-<N>.md  (per seed)
#   eval/coding-gate/ci-runs/<utc-timestamp>/SUMMARY.md   (auto-grade)
#
# Pre-requisites: bin/zhen-rag built; cs serve up at $VOR_URL;
# llama-server up at $LLAMA_URL.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

SEEDS=(42 137 314 271 999)
RUN_DIR="eval/coding-gate/ci-runs/$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$RUN_DIR"

SUMMARY="$RUN_DIR/SUMMARY.md"

echo "Coding-gate CI — 5-seed sweep" > "$SUMMARY"
echo "" >> "$SUMMARY"
echo "**Run dir:** \`$RUN_DIR\`" >> "$SUMMARY"
echo "**HEAD:** $(git rev-parse --short HEAD 2>/dev/null || echo unknown)" >> "$SUMMARY"
echo "**Date:** $(date -Iseconds)" >> "$SUMMARY"
echo "" >> "$SUMMARY"

for seed in "${SEEDS[@]}"; do
    echo ""
    echo "=== seed=$seed ==="
    PROBE_NAME="ci-seed-${seed}" \
        PROBE_SEED="$seed" \
        PROBE_OUT="$RUN_DIR/seed-${seed}.md" \
        bash scripts/coding-gate-probe.sh 2>&1 | tail -2
done

echo ""
echo "=== auto-grade summary ==="

# Inline auto-grader call against just the CI run directory.
python3 - "$RUN_DIR" "${SEEDS[@]}" <<'PY' >> "$SUMMARY"
import sys, re, os
from pathlib import Path

run_dir = Path(sys.argv[1])
seeds = [int(s) for s in sys.argv[2:]]

# Reuse the same heuristic detectors as scripts/probe-auto-grade.py
HEURISTICS = {
    "syntax-bash":       (["${var", "trimm", "parameter expansion", "[[:space:]]"], [], 1),
    "syntax-python":     (["list comprehension", "[x for x in", "[expression"], [], 1),
    "syntax-go":         (["if err != nil", "err :=", "if err"], ["I don't know", "do not contain"], 1),
    "syntax-rust":       (["parse::", "Result", "match"], [], 1),
    "syntax-html":       (["<button>", "button tag", "button>"], [], 1),
    "syntax-css":        (["flex", "grid", "display:", "place-items", "translate(-50"], ["I don't know", "do not contain"], 1),
    "syntax-javascript": (["fetch(", "async", "await"], ["I don't know", "do not contain"], 1),
    "review-bash":       (["set -e", "pipefail", "set -u", "strict mode", "quote", "\"$"], ["no issue", "is correct"], 1),
    "review-python":     (["bare except", "except:", "specific", "FileNotFoundError", "JSONDecode"], [], 1),
    "review-go":         (["err :=", "ignore", "return error", "%w", "fmt.Errorf"], [], 1),
    "review-rust":       (["unwrap", "Result", "ParseIntError", "?"], [], 1),
    "review-html":       (["alt=", "alt attribute", "alt text", "accessibility"], ["no issue", "well-formed", "no errors", "does not contain any errors"], 1),
    "review-css":        (["transform", "translate(-50", "flexbox", "place-items"], ["is correct", "no issue", "no errors", "does not contain any"], 1),
    "review-javascript": (["===", "strict equality", "type coercion"], ["is correct", "no issue", "no errors", "no problems"], 1),
}

RED_FLAGS = {
    "review-html":       ["does not contain any errors", "well-formed and does not contain", "no need for any changes"],
    "review-css":        ["positions a", "is correct", "does not contain any errors"],
    "review-javascript": ["is correct. There are no issues", "There are no issues with it", "snippet is correct."],
}

def extract_outputs(p):
    if not p.exists(): return {}
    text = p.read_text()
    sections = re.split(r'^### \d+\. `([^`]+)`', text, flags=re.MULTILINE)
    out = {}
    for i in range(1, len(sections), 2):
        pid = sections[i]
        body = sections[i+1] if i+1 < len(sections) else ""
        fences = re.findall(r'```\n(.*?)\n```', body, flags=re.DOTALL)
        out[pid] = fences[-1] if fences else ""
    return out

def grade(pid, output):
    must, must_not, n = HEURISTICS.get(pid, ([], [], 1))
    out = output.lower()
    for r in RED_FLAGS.get(pid, []):
        if r.lower() in out:
            return "🔴"
    for nm in must_not:
        if nm.lower() in out:
            return "FAIL"
    hits = sum(1 for h in must if h.lower() in out)
    return "PASS" if hits >= n else "FAIL"

print()
print("## Per-seed aggregate")
print()
print("| seed | PASS | FAIL | 🔴 | syntax | review |")
print("|---|---|---|---|---|---|")

per_seed_pass = []
per_seed_red = []

for seed in seeds:
    p = run_dir / f"seed-{seed}.md"
    outs = extract_outputs(p)
    grades = {pid: grade(pid, outs.get(pid, "")) for pid in HEURISTICS}
    npass = sum(1 for g in grades.values() if g == "PASS")
    nfail = sum(1 for g in grades.values() if g in ("FAIL", "🔴"))
    nred  = sum(1 for g in grades.values() if g == "🔴")
    spass = sum(1 for pid in grades if pid.startswith("syntax-") and grades[pid] == "PASS")
    rpass = sum(1 for pid in grades if pid.startswith("review-") and grades[pid] == "PASS")
    print(f"| {seed} | {npass} | {nfail} | {nred} | {spass}/7 | {rpass}/7 |")
    per_seed_pass.append(npass)
    per_seed_red.append(nred)

# Verdict band analysis
pass_min = min(per_seed_pass)
pass_max = max(per_seed_pass)
red_max = max(per_seed_red)
print()
print(f"**PASS-count band:** [{pass_min}, {pass_max}] — width {pass_max - pass_min}")
print(f"**🔴 count (max across seeds):** {red_max}")
print()

# Verdict per RUBRIC.md §4 (heuristic — confirms with hand-grading)
print("## Heuristic verdict per seed")
print()
print("| seed | verdict (heuristic, RUBRIC v2) |")
print("|---|---|")
for i, seed in enumerate(seeds):
    npass = per_seed_pass[i]
    nred = per_seed_red[i]
    if npass >= 10 and nred == 0:
        verdict = "H1"
    elif 7 <= npass <= 9 and nred <= 1:
        verdict = "H2"
    elif nred >= 2:
        verdict = "H4 (veto)"
    else:
        verdict = "H3"
    print(f"| {seed} | {verdict} |")

# CI gate decision
print()
if pass_max - pass_min > 2:
    print(f"**CI GATE: WARN** — verdict band swing of {pass_max - pass_min} prompts (>2). PR requires probe-results doc justifying the change.")
else:
    print(f"**CI GATE: OK** — verdict band swing of {pass_max - pass_min} prompts (≤2 threshold).")
PY

echo ""
echo "Done. See $SUMMARY"
cat "$SUMMARY" | tail -30
