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
# runs each rule against a broken copy), and CI_VERDICTS_TREE at the tree the
# workflows' paths are looked up in, default the same one. Exit 1 on a violation, 2 if it cannot
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

def softened(step):
    """A step that runs only sometimes, or whose failure is ignored, is not a
    verdict. It reads as passed, and the job above it does too."""
    return bool(step.get("continue-on-error")) or step.get("if", "always()") != "always()"

# M4. The summary job is what a branch rule would require, so it must fail
# when any job it waits for did not succeed, and must run when one of them
# failed: a job skipped by a failed dependency reads as passed.
SCANNERS = {"govulncheck", "gosec", "gitleaks", "cargo-audit", "trivy-fs"}

def security_gate():
    jobs = workflow("security.yml")["jobs"]
    job = jobs["security-gate"]
    needs = job.get("needs", [])
    needs = [needs] if isinstance(needs, str) else list(needs)
    for n in needs:
        if jobs.get(n, {}).get("continue-on-error"):
            bad("security-gate", f"{n} has continue-on-error, so its failure reads as success here")
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
        if softened(s):
            bad("history-scan", "the step has continue-on-error or an if:")
    if not any(str(s.get("run", "")).strip() == "./scripts/test-check-gitleaks-history.sh" for s in job.get("steps", [])):
        bad("history-scan", "no step runs ./scripts/test-check-gitleaks-history.sh")
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
        if softened(s):
            bad("trivy", "the vuln gate has continue-on-error or an if:")
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
    elif softened(step):
        bad("auth-listeners", "the listener check has continue-on-error or an if:")
    for path in sorted(glob.glob(".github/workflows/*.yml")):
        for name, j in workflow(os.path.basename(path)).get("jobs", {}).items():
            for s in j.get("steps", []):
                if "manual review" in str(s.get("run", "")).lower():
                    bad("auth-listeners", f"{path} {name}: a step ends in a note for a person, not a verdict")

# L22. What CI fetches and runs is decided in the repo, not by whatever was
# newest that day. @latest, a bare `cargo install`, an install whose failure is
# swallowed and an install script read from a branch all let it move unseen.
CI_FILES = sorted(glob.glob(".github/workflows/*.yml") + glob.glob("jenkins/Jenkinsfile.*")
                  + glob.glob("jenkins/vars/*.groovy") + ["Jenkinsfile", "nix/yggdrasil/Jenkinsfile", "Makefile"])
FETCH_MODULE = re.compile(r"\bgo (?:install|run)\b[^|;&]*?\s([a-z0-9.-]+\.[a-z]+/\S+?)@(\S+)")
CARGO_INSTALL = re.compile(r"\bcargo install (?:--\S+ )*([A-Za-z0-9_-]+)(@\S+)?(.*)")
CURL_TO_SHELL = re.compile(r"\bcurl\b.*\|\s*(?:sudo\s+)?(?:sh|bash)\b")
PIP_INSTALL = re.compile(r"\bpip3? install\b(.*)")
PIP_EXACT = re.compile(r"[A-Za-z0-9._-]+==\d+(?:\.\d+)+")

def tool_fetches():
    seen = {}
    with open("scripts/lib/tool-pins.sh", encoding="utf-8") as f:
        if not re.search(r'^TRIVY_SHA256="[0-9a-f]{64}"', f.read(), re.M):
            bad("tool-fetch", "no TRIVY_SHA256 in scripts/lib/tool-pins.sh for install-trivy.sh to check")
    for path in CI_FILES:
        with open(path, encoding="utf-8") as f:
            for n, line in enumerate(f, 1):
                text = line.strip()
                if text.startswith(("#", "//")):
                    continue
                text = re.split(r"\s#\s", text, maxsplit=1)[0]
                where = f"{path}:{n}"
                for mod, version in FETCH_MODULE.findall(text):
                    if not re.fullmatch(r"v\d+\.\d+\.\d+", version.rstrip(")\"'")):
                        bad("tool-fetch", f"{where}: {mod}@{version} is not an exact version")
                    seen.setdefault(mod, {}).setdefault(version.rstrip(")\"'"), []).append(where)
                m = CARGO_INSTALL.search(text)
                if m:
                    crate, version, rest = m.group(1), m.group(2), m.group(3)
                    if not version or not re.fullmatch(r"@\d+\.\d+\.\d+", version) or "--locked" not in rest:
                        bad("tool-fetch", f"{where}: `cargo install {crate}` needs @<version> and --locked")
                    else:
                        seen.setdefault(crate, {}).setdefault(version[1:], []).append(where)
                    if "|| true" in rest:
                        bad("tool-fetch", f"{where}: `cargo install {crate}` swallows its failure")
                if CURL_TO_SHELL.search(text):
                    bad("tool-fetch", f"{where}: a script piped to a shell runs before anything checks it")
                m = PIP_INSTALL.search(text)
                if m and "--require-hashes" not in m.group(1):
                    for arg in m.group(1).split():
                        if not arg.startswith("-") and not PIP_EXACT.fullmatch(arg):
                            bad("tool-fetch", f"{where}: pip install {arg} is not an exact version")
                if re.match(r"GO_VERSION\s*=", text):
                    bad("tool-fetch", f"{where}: GO_VERSION is read by nothing; the agent image sets Go")
    for tool, versions in sorted(seen.items()):
        if len(versions) > 1:
            bad("tool-fetch", f"{tool}: more than one version in CI: {sorted(versions)}")

# A Jenkins pipeline that does not compile, or whose agent cannot build the
# module, fails with nothing scanned. The declarative validator rejects an
# empty environment block, and the golang images set GOTOOLCHAIN=local, so the
# image's Go must be the one go.mod names.
def jenkins_agents():
    with open("go.mod", encoding="utf-8") as f:
        go = re.search(r"^toolchain go(\S+)$", f.read(), re.M)
    if not go:
        bad("jenkins", "go.mod names no toolchain")
        return
    for path in CI_FILES:
        if "Jenkinsfile" not in path:
            continue
        with open(path, encoding="utf-8") as f:
            text = re.sub(r"(?m)^\s*//.*$", "", f.read())
        if re.search(r"\benvironment\s*\{\s*\}", text):
            bad("jenkins", f"{path}: an empty environment block does not compile")
        for image in re.findall(r"\bimage\s+'golang:([^']+)'", text):
            if image != go.group(1) and not image.startswith(go.group(1) + "-"):
                bad("jenkins", f"{path}: golang:{image} is not go.mod's toolchain {go.group(1)}")

# M4. A required check that no job reports leaves every merge waiting, and a
# summary job nothing requires blocks nothing. This reads the documented list;
# what GitHub enforces is a setting no file here can show.
def branch_protection():
    with open(".github/BRANCH_PROTECTION.md", encoding="utf-8") as f:
        text = f.read()
    names = set()
    for path in glob.glob(".github/workflows/*.yml"):
        w = workflow(os.path.basename(path))
        if "pull_request" in (w.get("on") or w.get(True) or {}):
            for job_id, j in w.get("jobs", {}).items():
                names.add(j.get("name", job_id))
    lists = [re.findall(r'"([^"]+)"', c) for c in re.findall(r'"contexts": \[(.*?)\]', text, re.S)]
    lists += [re.findall(r"^- \[x\] `([^`]+)` —", s, re.M)
              for s in re.findall(r"#### Required Status Checks \(All must pass\)\n(.*?)\n####", text, re.S)]
    if len(lists) < 4:
        bad("branch-protection", f"found {len(lists)} required-check lists, expected main and develop, prose and command")
    for contexts in lists:
        if "Security Gate" not in contexts or "CI Gate" not in contexts:
            bad("branch-protection", f"a required-check list omits a summary job: {contexts}")
        for c in contexts:
            if c not in names:
                bad("branch-protection", f"no pull_request job reports `{c}`")

# A target rustup has no prebuilt std for stops the toolchain install, and with
# it the job. ebpf/.cargo/config.toml builds core for it instead (build-std),
# from rust-src.
def rust_targets():
    for path in sorted(glob.glob(".github/workflows/*.yml")):
        for name, j in workflow(os.path.basename(path)).get("jobs", {}).items():
            for s in j.get("steps", []):
                if "rust-toolchain" in str(s.get("uses", "")) and "bpf" in str(s.get("with", {}).get("targets", "")):
                    bad("rust-target", f"{path} {name}: rustup has no bpf target to install; build-std supplies it")

# A step that names a file the tree does not have fails with nothing run, and
# no fixture catches it: docker.yml named two Dockerfiles the tree did not have,
# and moving code to another repository makes more of these. CI_VERDICTS_TREE
# is where the paths are looked up, for a test that breaks only the workflows.
RUN_PATH = re.compile(r"(?:^|[\s;&|(])((?:\./)?(?:scripts|tests|nix/yggdrasil/(?:scripts|tests))/[\w./-]+\.(?:sh|py))(?=$|[\s;&|)])")

def workflow_paths():
    tree = os.environ.get("CI_VERDICTS_TREE", ".")
    def there(p):
        return os.path.exists(os.path.join(tree, p))
    def named(path, job, what, p):
        if p and "${{" not in p and not there(p):
            bad("workflow-paths", f"{path} {job}: {what} {p} is not in the tree")
    for path in sorted(glob.glob(".github/workflows/*.yml")):
        for name, j in workflow(os.path.basename(path)).get("jobs", {}).items():
            for s in j.get("steps", []):
                w = s.get("with") or {}
                named(path, name, "working-directory", s.get("working-directory"))
                named(path, name, "go-version-file", w.get("go-version-file"))
                named(path, name, "trivyignores", w.get("trivyignores"))
                for key in re.findall(r"hashFiles\('([^'*]+)'\)", str(w.get("key", ""))):
                    named(path, name, "hashFiles", key)
                if "build-push-action" in str(s.get("uses", "")):
                    named(path, name, "build context", w.get("context"))
                    named(path, name, "Dockerfile", w.get("file"))
                    target = w.get("target")
                    if target and w.get("file") and there(w["file"]):
                        with open(os.path.join(tree, w["file"]), encoding="utf-8") as f:
                            if not re.search(rf"(?im)^FROM\s.*\sAS\s+{re.escape(target)}\s*$", f.read()):
                                bad("workflow-paths", f"{path} {name}: {w['file']} has no stage {target}")
                for line in str(s.get("run", "")).splitlines():
                    if line.strip().startswith("#"):
                        continue
                    for script in RUN_PATH.findall(line):
                        named(path, name, "script", script)
                        if (line.strip().startswith(script) and there(script)
                                and not os.access(os.path.join(tree, script), os.X_OK)):
                            bad("workflow-paths", f"{path} {name}: {script} is run directly but is not executable")

security_gate()
history_scan()
bpf_linker()
trivy_steps()
auth_listeners()
tool_fetches()
jenkins_agents()
branch_protection()
rust_targets()
workflow_paths()

if problems:
    print("\n".join(f"[FAIL] {p}" for p in problems))
    sys.exit(1)
print("[PASS] CI verdicts: every checked step can fail")
PY
