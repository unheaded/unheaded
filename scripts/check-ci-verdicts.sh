#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# check-ci-verdicts.sh — a CI step reports only what it checked (GATING).
#
# check-gates-can-fail.sh proves the check-*.sh scripts can fail. It does not
# read the workflows that call them, so a verdict written in workflow YAML has
# no proof of its own. This reads that YAML, and where a verdict is a shell
# body it runs that body against fixture results instead of matching its text.
#
#   scripts/check-ci-verdicts.sh
#
# CI_VERDICTS_ROOT points it at another tree (scripts/test-check-ci-verdicts.sh
# runs each rule against a broken copy). Exit 1 on a violation, 2 if it cannot
# run: this gate does not pass by skipping.
set -euo pipefail

ROOT="${CI_VERDICTS_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
cd "$ROOT"
command -v jq >/dev/null || { echo "[FAIL] jq not found; this gate does not pass by skipping" >&2; exit 2; }

python3 -I - <<'PY'
import json, os, re, subprocess, sys, tempfile

try:
    import yaml
except ImportError:
    print("[FAIL] PyYAML not found; this gate does not pass by skipping", file=sys.stderr)
    sys.exit(2)

problems = []

def bad(rule, msg):
    problems.append(f"{rule}: {msg}")

def workflow(name):
    with open(f".github/workflows/{name}", encoding="utf-8") as f:
        return yaml.safe_load(f)

def run_step(body, results):
    """Run a step body as the runner does (bash -e), with `results` as its
    needs context. Returns the exit status."""
    def expand(m):
        return results.get(m.group(1), {}).get("result", "")
    text = re.sub(r"\$\{\{\s*needs\.([A-Za-z0-9_-]+)\.result\s*\}\}", expand, body)
    with tempfile.NamedTemporaryFile("w", suffix=".sh", delete=False) as f:
        f.write(text)
    try:
        env = dict(os.environ, NEEDS_JSON=json.dumps(results))
        return subprocess.run(["bash", "-e", f.name], env=env, stdout=subprocess.DEVNULL,
                              stderr=subprocess.DEVNULL, timeout=30).returncode
    finally:
        os.unlink(f.name)

# M4. The summary job is what a branch rule would require, so it must fail
# when any job it waits for did not succeed, and must run when one of them
# failed: a job skipped by a failed dependency reads as passed.
SCANNERS = {"govulncheck", "gosec", "gitleaks", "cargo-audit", "trivy-fs"}

def security_gate():
    job = workflow("security.yml")["jobs"]["security-gate"]
    needs = job.get("needs", [])
    needs = [needs] if isinstance(needs, str) else list(needs)
    for missing in sorted(SCANNERS - set(needs)):
        bad("security-gate", f"needs no longer lists {missing}")
    if job.get("if") != "always()":
        bad("security-gate", "must run with if: always()")
    steps = [s for s in job.get("steps", []) if "run" in s]
    if len(steps) != 1:
        bad("security-gate", f"expected one run step, found {len(steps)}")
        return
    step = steps[0]
    if str(step.get("env", {}).get("NEEDS_JSON", "")).replace(" ", "") != "${{toJSON(needs)}}":
        bad("security-gate", "step must take NEEDS_JSON from toJSON(needs), not a list kept by hand")
    green = {n: {"result": "success"} for n in needs}
    if run_step(step["run"], green) != 0:
        bad("security-gate", "fails with every need green")
    for n in needs:
        for result in ("failure", "cancelled", "skipped"):
            if run_step(step["run"], {**green, n: {"result": result}}) == 0:
                bad("security-gate", f"passes with {n}={result}")
    if run_step(step["run"], {}) == 0:
        bad("security-gate", "passes with no job results at all")

security_gate()

if problems:
    print("\n".join(f"[FAIL] {p}" for p in problems))
    sys.exit(1)
print("[PASS] CI verdicts: every checked step can fail")
PY
