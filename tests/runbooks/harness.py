# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Dry-run harness for runbook tests (S-TWO-BRAINS Step 268).

A runbook test never touches west, east, the Well or a running service.
It either dry-runs the runbook through scripts/run-runbook.py, or runs one
step's text, exactly as the runner would expand it, against a throwaway:
a sandbox git repository, a throwaway Wotan on free ports, a temp dir.

  dry_run(path)            step names the runner lists, in order
  load(path), step(rb, n)  the parsed runbook, one step by name
  run_step(rb, name, env)  run a step's command (or verify) like the runner
  requires(cmd)            skip marker when a tool is missing
  free_port()              an unused TCP port on 127.0.0.1
  Wotan                    a throwaway Wotan process (see conftest.py)
"""
import importlib.util
import json
import os
import re
import shutil
import signal
import socket
import subprocess
import time
import urllib.error
import urllib.request
from pathlib import Path

import pytest
import yaml

REPO = Path(__file__).resolve().parents[2]
RUNNER = REPO / "scripts" / "run-runbook.py"


def _runner_module():
    spec = importlib.util.spec_from_file_location("run_runbook", RUNNER)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


_RUNNER = _runner_module()


def load(path):
    with open(REPO / path if not os.path.isabs(str(path)) else path, encoding="utf-8") as f:
        return yaml.safe_load(f)


def steps(rb):
    return list(rb.get("steps") or [])


def step(rb, name):
    for s in steps(rb):
        if s.get("name") == name:
            return s
    raise KeyError(f"no step {name!r}; have {[s.get('name') for s in steps(rb)]}")


def index(rb, name):
    return [s.get("name") for s in steps(rb)].index(name)


def dry_run(path, env=None):
    """Run the runner in --dry-run mode; return the step names it lists."""
    p = REPO / path if not os.path.isabs(str(path)) else Path(path)
    r = subprocess.run(["python3", "-B", str(RUNNER), "--dry-run", str(p)],
                       capture_output=True, text=True, timeout=60, check=False,
                       env={**os.environ, **(env or {})}, cwd=REPO)
    assert r.returncode == 0, f"dry run failed ({r.returncode}):\n{r.stdout}\n{r.stderr}"
    return re.findall(r"^\s+\[\d+/\d+\] (\S+)", r.stdout, re.MULTILINE)


def runner_env(rb, environ):
    """The env dict run-runbook.py's main() builds (runbook env, then
    parameter defaults the operator's environment did not set)."""
    env = {k: os.path.expanduser(str(v)) for k, v in (rb.get("env") or {}).items()}
    for p in rb.get("parameters") or []:
        if isinstance(p, dict) and "default" in p:
            n = p.get("name")
            if n and n not in env and n not in environ:
                env[n] = str(p["default"])
    return env


def run_step(rb, name, env=None, field="command", cwd=None, timeout=120, check=True):
    """Run one step's text the way run-runbook.py runs it: ${VAR}s of the
    runbook's env and parameter defaults substituted, then /bin/bash with the
    caller's environment (env= stands for the operator's shell). Manual
    steps run too: a test exercises the operator's commands against a
    sandbox, which the runner itself never does."""
    environ = {**os.environ, **(env or {})}
    renv = runner_env(rb, environ)
    text = step(rb, name).get(field) or ""
    cmd = _RUNNER.expand_env(text, renv)
    r = subprocess.run(cmd, shell=True, executable="/bin/bash", capture_output=True, text=True,  # nosec B602 - runbook text under test, in a sandbox
                       timeout=timeout, env={**environ, **renv}, cwd=cwd, check=False)
    if check and r.returncode != 0:
        raise AssertionError(f"step {name}.{field} exited {r.returncode}\n--- cmd\n{cmd}\n--- stdout\n{r.stdout}\n--- stderr\n{r.stderr}")
    return r


_SPLIT = re.compile(r"\|\||&&|[|;]|\$\(|`|<\(|\)")


def commands(text):
    """The simple commands of a shell snippet, roughly: one argv-ish string
    per pipeline element, `sudo`/`sudo -n` and leading VAR= assignments
    stripped, comments, `set -...` and control keywords dropped. Good enough
    to hold read-only runbook steps to an allowlist, not a shell parser."""
    out = []
    for line in (text or "").splitlines():
        # Quoted text is one word: mask it so a | or ( inside a jq or awk
        # program is not taken for a pipeline.
        quoted = []

        def mask(m):
            quoted.append(m.group(0))
            return f"\x00{len(quoted) - 1}\x00"

        line = re.sub(r"'[^']*'|\"(?:[^\"\\]|\\.)*\"", mask, line)
        line = line.split(" #", 1)[0].strip()
        if not line or line.startswith("#"):
            continue
        for seg in _SPLIT.split(line):
            seg = re.sub(r"\x00(\d+)\x00", lambda m: quoted[int(m.group(1))], seg)
            seg = seg.strip().lstrip("({!").strip()
            seg = re.sub(r"^(?:[A-Za-z_]\w*=\S*\s+)+", "", seg)
            seg = re.sub(r"^sudo(?:\s+-n)?\s+", "", seg)
            words = seg.split()
            if words and words[0] == "for":
                continue
            while words and words[0] in ("if", "elif", "while", "until", "then", "else", "do", "!"):
                words = words[1:]
            seg = " ".join(words)
            if not words or words[0] in ("fi", "done", "}", "esac") or re.fullmatch(r"[A-Za-z_]\w*=.*", seg):
                continue
            if seg.startswith("set -"):
                continue
            out.append(seg)
    return out


def requires(*cmds):
    missing = [c for c in cmds if shutil.which(c) is None]
    return pytest.mark.skipif(bool(missing), reason=f"needs {', '.join(missing)} on PATH")


def free_port():
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def git(repo, *args, env=None, check=True):
    r = subprocess.run(["git", "-C", str(repo), *args], capture_output=True, text=True,
                       env={**os.environ, **(env or {})}, timeout=60, check=False)
    if check and r.returncode != 0:
        raise AssertionError(f"git {' '.join(args)}: {r.stderr}")
    return r.stdout.strip()


def http(method, url, body=None, timeout=5):
    """(status, parsed JSON or text) without raising on 4xx/5xx."""
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method,
                                 headers={"Content-Type": "application/json"} if data else {})
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:  # nosec B310 - 127.0.0.1 throwaway
            raw = resp.read().decode()
            code = resp.status
    except urllib.error.HTTPError as e:
        raw, code = e.read().decode(), e.code
    try:
        return code, json.loads(raw)
    except ValueError:
        return code, raw


class Wotan:
    """A throwaway Wotan: its own binary, free ports, a temp topic config,
    memory store. restart() keeps ports and config, like a unit restart."""

    def __init__(self, binary, workdir, auto_approve=("service",)):
        self.binary = binary
        self.dir = Path(workdir)
        self.http_port = free_port()
        self.grpc_port = free_port()
        self.config = self.dir / "wotan.yaml"
        self.log = self.dir / "wotan.log"
        self.write_config(list(auto_approve))
        self.proc = None

    @property
    def addr(self):
        return f"127.0.0.1:{self.http_port}"

    @property
    def url(self):
        return f"http://{self.addr}"

    def write_config(self, names):
        self.config.write_text(yaml.safe_dump({"topics": {"auto_approve": list(names)}, "store": {"type": "memory"}}))

    def start(self, timeout=20):
        logf = open(self.log, "ab")  # noqa: SIM115 - handed to the child
        self.proc = subprocess.Popen(
            [str(self.binary), "-http-port", str(self.http_port), "-grpc-port", str(self.grpc_port),
             "-topic-config", str(self.config), "-store-type", "memory", "-log-level", "info"],
            stdout=logf, stderr=subprocess.STDOUT, cwd=self.dir, start_new_session=True,
            env={k: v for k, v in os.environ.items() if k != "WOTAN_TOPIC_CONFIG"})
        logf.close()
        deadline = time.time() + timeout
        while time.time() < deadline:
            if self.proc.poll() is not None:
                raise RuntimeError(f"wotan exited {self.proc.returncode}: {self.log.read_text()[-2000:]}")
            try:
                code, _ = http("GET", self.url + "/health", timeout=1)
                if code == 200:
                    return self
            except OSError:
                pass
            time.sleep(0.1)
        raise RuntimeError("wotan did not answer /health")

    def kill9(self):
        if self.proc and self.proc.poll() is None:
            os.killpg(self.proc.pid, signal.SIGKILL)
            self.proc.wait(timeout=10)

    def stop(self):
        if self.proc and self.proc.poll() is None:
            os.killpg(self.proc.pid, signal.SIGTERM)
            try:
                self.proc.wait(timeout=20)
            except subprocess.TimeoutExpired:
                self.kill9()

    def restart(self):
        self.stop()
        return self.start()

    def subscribe(self, topic, name):
        return http("POST", f"{self.url}/api/v1/topics/{topic}/subscribe", {"display_name": name})
