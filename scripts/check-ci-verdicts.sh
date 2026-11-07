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
    summary_verdict("security-gate", job, needs)

def summary_verdict(rule, job, needs):
    """A summary job runs whatever its needs did, and its one step fails
    unless every one of them succeeded."""
    if job.get("if") != "always()":
        bad(rule, "must run with if: always()")
    steps = [s for s in job.get("steps", []) if "run" in s]
    if len(steps) != 1:
        bad(rule, f"expected one run step, found {len(steps)}")
        return
    step = steps[0]
    if str(step.get("env", {}).get("NEEDS_JSON", "")).replace(" ", "") != "${{toJSON(needs)}}":
        bad(rule, "step must take NEEDS_JSON from toJSON(needs), not a list kept by hand")
    green = {n: {"result": "success"} for n in needs}
    if run_step(step["run"], green) != 0:
        bad(rule, "fails with every need green")
    for n in needs:
        for result in ("failure", "cancelled", "skipped"):
            if run_step(step["run"], {**green, n: {"result": result}}) == 0:
                bad(rule, f"passes with {n}={result}")
    if run_step(step["run"], {}) == 0:
        bad(rule, "passes with no job results at all")

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
        if "ignore-unfixed" in w:
            bad("trivy", "the vuln gate must see findings with no fixed release")
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
FETCH_TO_SHELL = re.compile(r"\b(?:curl|wget)\b.*\|\s*(?:sudo\s+)?(?:sh|bash)\b")
FETCH_TO_TAR = re.compile(r"\b(?:curl|wget)\b.*\|\s*(?:sudo\s+)?tar\b")
PIP_INSTALL = re.compile(r"\bpip3? install\b(.*)")
PIP_EXACT = re.compile(r"[A-Za-z0-9._-]+==\d+(?:\.\d+)+")

# A shell line as the shell reads it: one ending in a backslash or a pipe goes
# on, so a pipe split across lines is still that pipe. Gives (first line, text).
def shell_lines(lines):
    start, parts = None, []
    for n, line in enumerate(lines, 1):
        text = line.strip()
        if text.startswith(("#", "//")):
            continue
        text = re.split(r"\s#\s", text, maxsplit=1)[0].rstrip()
        start = start or n
        if text.endswith("\\"):
            parts.append(text[:-1])
        elif text.endswith("|") and not text.endswith("||"):
            parts.append(text)
        else:
            parts.append(text)
            yield start, " ".join(parts)
            start, parts = None, []
    if parts:
        yield start, " ".join(parts)

def tool_fetches():
    seen = {}
    with open("scripts/lib/tool-pins.sh", encoding="utf-8") as f:
        if not re.search(r'^TRIVY_SHA256="[0-9a-f]{64}"', f.read(), re.M):
            bad("tool-fetch", "no TRIVY_SHA256 in scripts/lib/tool-pins.sh for install-trivy.sh to check")
    for path in CI_FILES:
        with open(path, encoding="utf-8") as f:
            lines = f.readlines()
        for n, text in shell_lines(lines):
            if FETCH_TO_SHELL.search(text):
                bad("tool-fetch", f"{path}:{n}: a script piped to a shell runs before anything checks it")
            if FETCH_TO_TAR.search(text):
                bad("tool-fetch", f"{path}:{n}: an archive piped to tar is unpacked before anything checks it")
        for n, line in enumerate(lines, 1):
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
        if "Jenkinsfile" not in path and not path.endswith(".groovy"):
            continue
        with open(path, encoding="utf-8") as f:
            text = re.sub(r"(?m)^\s*//.*$", "", f.read())
        if "docker.sock" in text:
            bad("jenkins", f"{path}: mounts the docker socket into an agent; that socket is root on the host")
        if "Jenkinsfile" not in path:
            continue
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
# A script is any relative path to a .sh or .py a run line names, from the
# step's working directory. A path holding a variable is not looked up.
RUN_PATH = re.compile(r"""(?:^|[\s;&|('"])((?:\.{1,2}/)*[\w.-]+(?:/[\w.-]+)*\.(?:sh|py))(?=$|[\s;&|)'"])""")
DOCKER_FILE = re.compile(r"\bdocker(?:\s+buildx)?\s+build\b.*?\s(?:-f|--file)[\s=]+([^\s;&|)]+)")

def workflow_paths():
    tree = os.environ.get("CI_VERDICTS_TREE", ".")
    def there(p):
        return os.path.exists(os.path.join(tree, p))
    def named(path, job, what, p):
        if p and "${{" not in p and not there(p):
            bad("workflow-paths", f"{path} {job}: {what} {p} is not in the tree")
    for path in sorted(glob.glob(".github/workflows/*.yml")):
        wf = workflow(os.path.basename(path))
        for name, j in wf.get("jobs", {}).items():
            default_dir = ((j.get("defaults") or wf.get("defaults") or {}).get("run") or {}).get("working-directory", "")
            for s in j.get("steps", []):
                w = s.get("with") or {}
                base = s.get("working-directory") or default_dir
                def from_step(p):
                    return os.path.normpath(os.path.join(base, p))
                named(path, name, "working-directory", s.get("working-directory"))
                named(path, name, "go-version-file", w.get("go-version-file"))
                named(path, name, "trivyignores", w.get("trivyignores"))
                for call in re.findall(r"hashFiles\(([^)]*)\)", f"{w.get('key', '')} {w.get('restore-keys', '')}"):
                    for key in re.findall(r"'([^'*]+)'", call):
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
                    for dockerfile in DOCKER_FILE.findall(line):
                        if "$" not in dockerfile:
                            named(path, name, "Dockerfile", from_step(dockerfile))
                    for script in RUN_PATH.findall(line):
                        named(path, name, "script", from_step(script))
                        if (line.strip().startswith(script) and there(from_step(script))
                                and not os.access(os.path.join(tree, from_step(script)), os.X_OK)):
                            bad("workflow-paths", f"{path} {name}: {script} is run directly but is not executable")

# The meta-gate outgrew one job's time limit, so its gates are split over a
# matrix. Each shard is a share of the registry: a matrix short of its own
# denominator drops gates with every job green, and a call with no --shard is
# back to one job the limit cancels. The step's body runs here against a stub
# of the meta-gate, once per shard and event, so what is checked is the call a
# push makes, and that a red meta-gate turns the step red.
def meta_gate_calls(body, event, shard, stub_exit):
    """Run a step body for one event and shard. Returns its exit status and
    the argument lines the meta-gate got."""
    text = body.replace("${{ github.event_name }}", event).replace("${{ matrix.shard }}", str(shard))
    with tempfile.TemporaryDirectory() as d:
        os.mkdir(f"{d}/scripts")
        stub = f"{d}/scripts/check-gates-can-fail.sh"
        with open(stub, "w", encoding="utf-8") as f:
            f.write(f'#!/bin/sh\nprintf "%s\\n" "$*" >>"$(dirname "$0")/calls"\nexit {stub_exit}\n')
        os.chmod(stub, 0o755)
        rc = subprocess.run(["bash", "-e", "-c", text], cwd=d, stdout=subprocess.DEVNULL,
                            stderr=subprocess.DEVNULL, timeout=30).returncode
        calls = f"{d}/scripts/calls"
        if not os.path.exists(calls):
            return rc, []
        with open(calls, encoding="utf-8") as f:
            return rc, f.read().splitlines()

def meta_gate_shards():
    jobs = workflow("static-analysis.yml").get("jobs", {})
    job = jobs.get("meta-gate", {})
    strategy = job.get("strategy") or {}
    shards = (strategy.get("matrix") or {}).get("shard") or []
    if strategy.get("fail-fast") is not False:
        bad("meta-gate", "needs fail-fast: false, or one shard's failure cancels the others' verdicts")
    if job.get("continue-on-error") or "if" in job:
        bad("meta-gate", "the job has a condition or continue-on-error, so a push need not prove anything")
    steps = [s for s in job.get("steps", []) if "check-gates-can-fail.sh" in str(s.get("run", ""))]
    if len(steps) != 1:
        bad("meta-gate", f"expected one step running check-gates-can-fail.sh, found {len(steps)}")
    if sorted(shards) != list(range(1, len(shards) + 1)):
        bad("meta-gate", f"matrix {shards} is not shares 1 to {len(shards)}")
    for step in steps:
        if softened(step):
            bad("meta-gate", "the step has a condition or continue-on-error, so a push need not prove anything")
        body = str(step["run"])
        for k in shards:
            share = f"--shard {k}/{len(shards)}"
            _, calls = meta_gate_calls(body, "push", k, 0)
            if calls != [share]:
                bad("meta-gate", f"a push on shard {k} ran {calls}, not the full share {share}")
            _, calls = meta_gate_calls(body, "pull_request", k, 0)
            if calls not in ([share], [f"--quick {share}"]):
                bad("meta-gate", f"a pull request on shard {k} ran {calls}, not share {share}")
            for event in ("push", "pull_request"):
                if meta_gate_calls(body, event, k, 1)[0] == 0:
                    bad("meta-gate", f"a {event} on shard {k} stays green with the meta-gate red")

    # The shards report as "name (1/3)", which nothing reads. This job carries
    # the verdict under the name without the share.
    needs = jobs.get("meta-gate-verdict", {}).get("needs", [])
    needs = [needs] if isinstance(needs, str) else list(needs)
    if needs != ["meta-gate"]:
        bad("meta-gate", f"the verdict job must need meta-gate alone, not {needs}")
    summary_verdict("meta-gate", jobs.get("meta-gate-verdict", {}), needs)

# Both hosts are amd64, so that is all docker.yml builds: an arm64 layer under
# QEMU multiplies the build time and ships an image nothing runs.
def docker_images():
    jobs = workflow("docker.yml")["jobs"]
    builds = [s for j in jobs.values() for s in j.get("steps", [])
              if "docker/build-push-action" in str(s.get("uses", ""))]
    if not builds:
        bad("docker-images", "docker.yml builds nothing; this rule has stopped reading it")
    for s in builds:
        if s.get("with", {}).get("platforms") != "linux/amd64":
            bad("docker-images", f"step '{s.get('name')}' must build linux/amd64 alone")
    # Each job builds into the runner's Docker, scans that tag, then pushes.
    # The scan used to name :<full sha> while the only tag was sha-<short>,
    # and nothing was loaded for it to read, so its first run would have failed.
    for job_id, j in jobs.items():
        steps = j.get("steps", [])
        loaded, scanned = set(), False
        for s in steps:
            w = s.get("with") or {}
            uses = str(s.get("uses", ""))
            if "docker/build-push-action" in uses:
                if w.get("push") is True:
                    if not scanned:
                        bad("docker-images", f"{job_id}: pushes before it scans")
                    if s.get("if") != "github.event_name != 'pull_request'":
                        bad("docker-images", f"{job_id}: a pull request can reach the push")
                elif w.get("load") is True:
                    loaded.add(w.get("tags"))
            if "trivy-action" in uses:
                scanned = True
                if w.get("image-ref") not in loaded:
                    bad("docker-images", f"{job_id}: the scan reads {w.get('image-ref')}, which no earlier step loaded")
                if w.get("version") != "${{ env.TRIVY_PIN }}":
                    bad("docker-images", f"{job_id}: the scan's trivy is not the pinned one")
        if not scanned:
            bad("docker-images", f"{job_id}: scans nothing")
        if not any((s.get("with") or {}).get("push") is True for s in steps):
            bad("docker-images", f"{job_id}: pushes nothing")

# Sixteen Dockerfiles under docker/services built commands that had left the
# tree, and nothing ran them to find out: each must name a command that exists,
# and docker/Makefile must list exactly the Dockerfiles there are.
def docker_services():
    tree = os.environ.get("CI_VERDICTS_TREE", ".")
    found = {os.path.basename(os.path.dirname(p)): p for p in glob.glob("docker/services/*/Dockerfile")}
    if not found:
        bad("docker-services", "no Dockerfile under docker/services; this rule has stopped reading them")
    for name, path in sorted(found.items()):
        with open(path, encoding="utf-8") as f:
            cmds = re.findall(r"\s(\./cmd/\S+)\s*$", f.read(), re.M)
        if not cmds:
            bad("docker-services", f"{path} builds no ./cmd/ package this rule can see")
        for cmd in cmds:
            if not os.path.isdir(os.path.join(tree, cmd)):
                bad("docker-services", f"{path} builds {cmd}, which the tree lacks")
    with open("docker/Makefile", encoding="utf-8") as f:
        m = re.search(r"^SERVICES := ((?:.*\\\n)*.*)$", f.read(), re.M)
    listed = set(m.group(1).replace("\\", " ").split()) if m else set()
    for name in sorted(listed - set(found)):
        bad("docker-services", f"docker/Makefile lists {name}, which has no Dockerfile")
    for name in sorted(set(found) - listed):
        bad("docker-services", f"docker/Makefile does not list {name}")

# plan.yml and apply.yml are off: apply runs -auto-approve, so off means no
# event starts them and a manual dispatch runs nothing either.
def iac_off():
    for name in ("plan.yml", "apply.yml"):
        w = workflow(name)
        events = w.get(True, w.get("on"))  # YAML reads a bare `on` as true
        events = [events] if isinstance(events, str) else list(events or [])
        if events != ["workflow_dispatch"]:
            bad("iac-off", f"{name} must start on workflow_dispatch alone, not {events}")
        for job_id, j in w.get("jobs", {}).items():
            if str(j.get("if")).replace(" ", "") != "${{false}}":
                bad("iac-off", f"{name} {job_id}: must carry if: ${{{{ false }}}}")

# compliance/catalog/controls.yaml and the findings register name CI jobs as
# <workflow name>/<job name>, and the evidence collector matches those exactly.
# A renamed job leaves its control with no new records, whatever the job did,
# and a matrix job reports under a different name per share. A ref to a job
# that is gone reads as a control nobody can pass.
def evidence_jobs():
    reports = set()
    for path in glob.glob(".github/workflows/*.yml"):
        w = workflow(os.path.basename(path))
        for job_id, j in w.get("jobs", {}).items():
            reports.add(f"{w.get('name', '')}/{j.get('name', job_id)}")
    with open("compliance/catalog/controls.yaml", encoding="utf-8") as f:
        catalog = yaml.safe_load(f)
    refs = [(c["id"], e["ref"]) for c in catalog["controls"] for e in c.get("evidence", [])
            if e.get("kind") == "github-job"]
    with open("compliance/findings/register.yaml", encoding="utf-8") as f:
        refs += [("the findings register", m) for m in re.findall(r'^\s*- source: "github-job:(.+)"\s*$', f.read(), re.M)]
    if not refs:
        bad("evidence-jobs", "found no github-job refs; this rule has stopped reading them")
    for who, ref in refs:
        if ref not in reports:
            bad("evidence-jobs", f"{who}: no workflow job reports as `{ref}`")

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
meta_gate_shards()
docker_images()
docker_services()
iac_off()
evidence_jobs()

if problems:
    print("\n".join(f"[FAIL] {p}" for p in problems))
    sys.exit(1)
print("[PASS] CI verdicts: every checked step can fail")
PY
