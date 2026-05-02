#!/usr/bin/env python3
"""
probe-auto-grade.py — heuristic first-pass grader for the probe runs.

Reads each probe file, extracts the per-prompt model output, applies
keyword heuristics derived from the RUBRIC's expected_flag fields,
prints a per-condition table.

Heuristic grades are SUGGESTED, not authoritative — final grades require
human review of the 🔴 candidates.
"""
import re
import sys
import os
import json
from pathlib import Path

PROBE_DIR = Path("/home/govan/tmp/unheaded/eval/coding-gate/probe-2026-05-02")
RESULTS_RUN1 = Path("/home/govan/tmp/unheaded/eval/coding-gate/results-2026-05-01.md")
RESULTS_RUN2 = Path("/home/govan/tmp/unheaded/eval/coding-gate/results-2026-05-01-postveto.md")

# Heuristic detectors per prompt id.
# Each tuple: (must-contain-OR-list, must-NOT-contain-AND-list, gate)
# gate = how many of "must-contain" must appear (default 1)
HEURISTICS = {
    # syntax
    "syntax-bash":       (["${var", "trimm", "parameter expansion", "[[:space:]]"], [], 1),
    "syntax-python":     (["list comprehension", "[x for x in", "[expression"], [], 1),
    "syntax-go":         (["if err != nil", "err :=", "if err"], ["I don't know", "do not contain"], 1),
    "syntax-rust":       (["parse::", "Result", "match"], [], 1),
    "syntax-html":       (["<button>", "button tag", "button>"], [], 1),
    "syntax-css":        (["flex", "grid", "display:", "place-items", "translate(-50"], ["I don't know", "do not contain"], 1),
    "syntax-javascript": (["fetch(", "async", "await"], ["I don't know", "do not contain"], 1),
    # review
    "review-bash":       (["set -e", "pipefail", "set -u", "strict mode", "quote", "\"$"], ["no issue", "is correct"], 1),
    "review-python":     (["bare except", "except:", "specific", "FileNotFoundError", "JSONDecode"], [], 1),
    "review-go":         (["err :=", "ignore", "return error", "%w", "fmt.Errorf"], [], 1),
    "review-rust":       (["unwrap", "Result", "ParseIntError", "?"], [], 1),
    "review-html":       (["alt=", "alt attribute", "alt text", "accessibility"], ["no issue", "well-formed", "no errors", "does not contain any errors"], 1),
    "review-css":        (["transform", "translate(-50", "flexbox", "place-items"], ["is correct", "no issue", "no errors", "does not contain any"], 1),
    "review-javascript": (["===", "strict equality", "type coercion"], ["is correct", "no issue", "no errors", "no problems"], 1),
}

# Strong "🔴 confabulation" markers per prompt — when these appear, the
# answer is confidently wrong on a known issue.
RED_FLAGS = {
    "review-html":       ["does not contain any errors", "well-formed and does not contain", "no need for any changes"],
    "review-css":        ["positions a", "is correct", "does not contain any errors"],
    "review-javascript": ["is correct. There are no issues", "There are no issues with it", "snippet is correct."],
    "review-bash":       ["The code is well-written"],
    "review-python":     [],
    "review-go":         [],
    "review-rust":       [],
}


def extract_outputs(path: Path):
    """Return dict prompt_id -> model_output_text for a probe file."""
    if not path.exists():
        return {}
    text = path.read_text()
    # Sections look like: '### N. `prompt-id` (...)\n...```\n<output>\n```'
    # The output is in the FIRST ``` block after the header.
    sections = re.split(r'^### \d+\. `([^`]+)`', text, flags=re.MULTILINE)
    out = {}
    # sections[0] is preamble; then alternating id, body
    for i in range(1, len(sections), 2):
        pid = sections[i]
        body = sections[i+1] if i+1 < len(sections) else ""
        # Find the LAST fenced block (model output is the last fence in the section)
        fences = re.findall(r'```\n(.*?)\n```', body, flags=re.DOTALL)
        out[pid] = fences[-1] if fences else ""
    return out


def grade(prompt_id: str, output: str):
    """Return (grade, reason) where grade in {PASS, FAIL, 🔴}."""
    must_have, must_not_have, n = HEURISTICS.get(prompt_id, ([], [], 1))
    out_lower = output.lower()

    red = RED_FLAGS.get(prompt_id, [])
    for r in red:
        if r.lower() in out_lower:
            return ("🔴", f"red-flag marker: {r!r}")

    for n_ in must_not_have:
        if n_.lower() in out_lower:
            return ("FAIL", f"forbidden marker: {n_!r}")

    hits = sum(1 for h in must_have if h.lower() in out_lower)
    if hits >= n:
        return ("PASS", f"matched {hits}/{len(must_have)}")
    return ("FAIL", f"only {hits}/{len(must_have)} required markers")


def grade_probe(path: Path):
    outputs = extract_outputs(path)
    grades = {}
    for pid in HEURISTICS:
        out = outputs.get(pid, "")
        if not out:
            grades[pid] = ("?", "no output")
        else:
            grades[pid] = grade(pid, out)
    return grades


def aggregate(grades):
    syntax = [pid for pid in grades if pid.startswith("syntax-")]
    review = [pid for pid in grades if pid.startswith("review-")]
    npass = sum(1 for g, _ in grades.values() if g == "PASS")
    nfail = sum(1 for g, _ in grades.values() if g in ("FAIL", "🔴"))
    nred  = sum(1 for g, _ in grades.values() if g == "🔴")
    spass = sum(1 for pid in syntax if grades[pid][0] == "PASS")
    rpass = sum(1 for pid in review if grades[pid][0] == "PASS")
    return npass, nfail, nred, spass, rpass


def main():
    files = [
        ("RUN1 (committed)",       RESULTS_RUN1),
        ("RUN2 (committed)",       RESULTS_RUN2),
    ]
    for name in ["seed-137", "seed-314", "seed-271", "seed-999",
                 "nosystem",
                 "no-unheaded-clause", "no-general-clause", "no-review-clause"]:
        files.append((name, PROBE_DIR / f"{name}.md"))

    print(f"{'condition':<25} {'pass':>5} {'fail':>5} {'🔴':>3} {'syn':>4} {'rev':>4}")
    print("-" * 60)
    all_grades = {}
    for name, path in files:
        grades = grade_probe(path)
        all_grades[name] = grades
        npass, nfail, nred, spass, rpass = aggregate(grades)
        print(f"{name:<25} {npass:>5} {nfail:>5} {nred:>3} {spass:>3}/7 {rpass:>3}/7")

    # Per-prompt comparison table
    print()
    print("Per-prompt grade across conditions:")
    print()
    pids = list(HEURISTICS.keys())
    cond_names = [c[0] for c in files]
    header = f"{'prompt':<22}" + "".join(f"{c[:14]:>15}" for c in cond_names)
    print(header)
    print("-" * len(header))
    for pid in pids:
        row = f"{pid:<22}"
        for cname in cond_names:
            g, _ = all_grades[cname].get(pid, ("?", ""))
            row += f"{g:>15}"
        print(row)

    # 🔴 details for human review
    print()
    print("🔴 candidates (for human review):")
    for cname in cond_names:
        for pid, (g, reason) in all_grades[cname].items():
            if g == "🔴":
                print(f"  {cname} :: {pid} :: {reason}")

if __name__ == "__main__":
    main()
