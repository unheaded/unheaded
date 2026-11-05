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
import glob, json, os, re, subprocess, sys, tempfile

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

# M5. The history scan runs the fetched, sha256-checked tool through its
# script, and nothing softens the step: the script already decides which
# outcomes only report. A step that names its own binary path is the shape of
# the bug this replaced, a path nothing created, under continue-on-error.
def history_scan():
    job = workflow("security.yml")["jobs"]["gitleaks"]
    if job.get("continue-on-error"):
        bad("history-scan", "the job has continue-on-error")
    if job.get("steps", [{}])[0].get("with", {}).get("fetch-depth") != 0:
        bad("history-scan", "checkout must set fetch-depth: 0")
    runs = [s for s in job.get("steps", []) if str(s.get("run", "")).strip() == "./scripts/check-gitleaks.sh --history"]
    if len(runs) != 1:
        bad("history-scan", "no step runs ./scripts/check-gitleaks.sh --history")
    for s in runs:
        if s.get("continue-on-error"):
            bad("history-scan", "the step has continue-on-error")
    for s in job.get("steps", []):
        if re.search(r"(^|\s)/tmp/gitleaks\b", str(s.get("run", ""))):
            bad("history-scan", "a step calls /tmp/gitleaks, which nothing creates")

# M22. bpf-linker builds the eBPF objects a release signs. One version in
# every workflow, --locked so its dependencies are the ones that version
# shipped, and default features: they link through rustc's own LLVM, so the
# runner needs no system LLVM.
BPF_LINKER_INSTALL = re.compile(r"cargo install bpf-linker@(\d+\.\d+\.\d+) --locked$")

def bpf_linker():
    versions = {}
    for path in sorted(glob.glob(".github/workflows/*.yml")):
        with open(path, encoding="utf-8") as f:
            for n, line in enumerate(f, 1):
                body = line.split("#", 1)[0].strip()
                body = re.sub(r"^run:\s*", "", body)
                if not re.search(r"cargo install\b.*\bbpf-linker\b", body):
                    continue
                m = BPF_LINKER_INSTALL.fullmatch(body)
                if not m:
                    bad("bpf-linker", f"{path}:{n}: not `cargo install bpf-linker@<version> --locked`")
                else:
                    versions.setdefault(m.group(1), []).append(f"{path}:{n}")
    if len(versions) > 1:
        bad("bpf-linker", f"workflows disagree on the version: {versions}")
    for must in ("release.yml", "ebpf.yml", "static-analysis.yml"):
        if not any(must in w for v in versions.values() for w in v):
            bad("bpf-linker", f"{must} no longer installs a pinned bpf-linker")

# L21. A scanner step says whether it can fail. trivy-action defaults to
# exit 0, so a step that sets no exit-code only reports. With format: sarif the
# action also drops `severity` unless limit-severities-for-sarif is set, so a
# gate on sarif would fail on every LOW.
def trivy_steps():
    steps = workflow("security.yml")["jobs"]["trivy-fs"]["steps"]
    scans = [s for s in steps if "trivy-action" in str(s.get("uses", ""))]
    if not scans:
        bad("trivy", "trivy-fs runs no scan")
    for s in scans:
        if "exit-code" not in s.get("with", {}):
            bad("trivy", f"step '{s.get('name')}' sets no exit-code")
    gates = [s for s in scans if s.get("with", {}).get("scanners") == "vuln" and str(s["with"].get("exit-code")) == "1"]
    if len(gates) != 1:
        bad("trivy", f"expected one vuln step with exit-code 1, found {len(gates)}")
    for s in gates:
        w = s["with"]
        if set(str(w.get("severity", "")).split(",")) != {"CRITICAL", "HIGH"}:
            bad("trivy", "the vuln gate must fail on CRITICAL and HIGH")
        if w.get("format") == "sarif" and str(w.get("limit-severities-for-sarif")) != "true":
            bad("trivy", "the vuln gate on sarif ignores its severity")

# L21. A step that only prints what it found, and tells a person to look,
# cannot fail. The listener inventory is scripts/check-http-auth.sh.
def auth_listeners():
    job = workflow("security.yml")["jobs"]["doom-security"]
    step = next((s for s in job["steps"] if s.get("name") == "HTTP listeners use pkg/auth or are recorded"), None)
    if step is None or str(step.get("run", "")).strip() != "./scripts/check-http-auth.sh":
        bad("auth-listeners", "no step runs ./scripts/check-http-auth.sh")
    for path in sorted(glob.glob(".github/workflows/*.yml")):
        for name, j in workflow(os.path.basename(path)).get("jobs", {}).items():
            for s in j.get("steps", []):
                if "manual review" in str(s.get("run", "")).lower():
                    bad("auth-listeners", f"{path} {name}: a step ends in a note for a person, not a verdict")

security_gate()
history_scan()
bpf_linker()
trivy_steps()
auth_listeners()

if problems:
    print("\n".join(f"[FAIL] {p}" for p in problems))
    sys.exit(1)
print("[PASS] CI verdicts: every checked step can fail")
PY
