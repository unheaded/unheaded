# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Tests for author-runbook.sh (S-TWO-BRAINS Step 279): a toy brief through
the real zhen-claude routed loop and the real run_check sandbox, with a fake
llama-server and the fake Claude review. Nothing is installed into the repo.
Run: python -m pytest scripts/two-brains/test_author_runbook.py
"""
import http.server
import json
import os
import shutil
import subprocess
import threading
from pathlib import Path

import pytest

REPO = Path(__file__).resolve().parents[2]
SCRIPT = REPO / "scripts" / "two-brains" / "author-runbook.sh"
REL = "runbooks/observe/toy-disk-check.yaml"

GOOD = """apiVersion: runbook/v1
kind: Runbook
metadata:
  name: toy-disk-check
  description: Toy runbook for the pipeline test
  owner: micromanager
  estimated_duration: 2m
  risk: low
  requires_sudo: false
  status: draft
  authored_by: zhen-local
  reviewed_by: ""
  approved_by: ""
  dry_run_test: tests/runbooks/test_toy_disk_check.py
steps:
  - name: show-disk
    action: shell
    command: |
      set -euo pipefail
      df -h /
      df -i /
    verify: "test -d /"
"""
BROKEN = GOOD.replace("action: shell", "action: python")   # fails the checker
REVIEWED = GOOD.replace("Toy runbook for the pipeline test", "Toy runbook, show disk and inode use")


def answer(text):
    return f"```file:{REL}\n{text}```\n"


class FakeLlama(http.server.BaseHTTPRequestHandler):
    answers = []
    prompts = []

    def do_POST(self):  # noqa: N802 - http.server API
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        FakeLlama.prompts.append(body["messages"][-1]["content"])
        text = FakeLlama.answers.pop(0) if FakeLlama.answers else answer(GOOD)
        out = json.dumps({"model": "fake", "choices": [{"finish_reason": "stop", "message": {"role": "assistant", "content": text}}],
                          "usage": {"prompt_tokens": 10, "completion_tokens": 10}}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(out)))
        self.end_headers()
        self.wfile.write(out)

    def log_message(self, *a):
        pass


@pytest.fixture
def fake_llama():
    srv = http.server.ThreadingHTTPServer(("127.0.0.1", 0), FakeLlama)
    t = threading.Thread(target=srv.serve_forever, daemon=True)
    t.start()
    FakeLlama.answers, FakeLlama.prompts = [], []
    yield f"http://127.0.0.1:{srv.server_address[1]}"
    srv.shutdown()


@pytest.fixture(scope="module")
def zhen_claude(tmp_path_factory):
    if shutil.which("systemd-run") is None:
        pytest.skip("run_check needs systemd-run (user session)")
    out = tmp_path_factory.mktemp("bin") / "zhen-claude"
    r = subprocess.run(["go", "build", "-o", str(out), "./cmd/zhen-claude"], cwd=REPO,
                       capture_output=True, text=True, timeout=600, check=False)
    if r.returncode != 0:
        pytest.skip(f"zhen-claude does not build: {r.stderr[-300:]}")
    return out


def run(tmp_path, zhen_claude, url, findings):
    brief = tmp_path / "toy-disk-check.md"
    brief.write_text("# Toy\nShow disk use of / with `df -h /` and `df -i /`. Owner micromanager. Low risk.\n")
    ff = tmp_path / "findings.json"
    ff.write_text(json.dumps(findings))
    wtroot = tmp_path / "wt"
    env = {**os.environ, "ZHEN_CLAUDE": str(zhen_claude), "AGENT_WT_ROOT": str(wtroot),
           "XDG_STATE_HOME": str(tmp_path / "state")}
    r = subprocess.run([str(SCRIPT), "observe/toy-disk-check", "--brief", str(brief), "--claude", "fake",
                        "--fake-findings", str(ff), "--local-url", url, "--no-install",
                        "--capture", str(tmp_path / "cap")],
                       capture_output=True, text=True, timeout=600, env=env, cwd=REPO, check=False)
    lines = [ln for ln in r.stdout.splitlines() if ln.startswith("{")]
    assert lines, f"no summary line\nstdout:{r.stdout}\nstderr:{r.stderr}"
    return r, json.loads(lines[-1])


def test_valid_draft_passes_with_summary_and_stub(tmp_path, zhen_claude, fake_llama):
    FakeLlama.answers = [answer(GOOD)]
    r, s = run(tmp_path, zhen_claude, fake_llama, [])
    assert r.returncode == 0, r.stderr
    assert s["name"] == "toy-disk-check" and s["harness"] == "pass"
    assert s["local_revisions"] == 0 and s["claude_findings"] == 0 and s["claude_patched"] is False
    wt = Path(s["worktree"])
    assert (wt / "tests/runbooks/test_toy_disk_check.py").exists()  # the test stub path
    text = (wt / REL).read_text()
    assert "reviewed_by: claude-opus-5-5" in text and 'approved_by: ""' in text and "status: draft" in text
    assert not (REPO / REL).exists()  # --no-install


def test_broken_draft_triggers_a_revision(tmp_path, zhen_claude, fake_llama):
    FakeLlama.answers = [answer(BROKEN), answer(GOOD)]
    r, s = run(tmp_path, zhen_claude, fake_llama, [])
    assert r.returncode == 0, r.stderr
    assert s["local_revisions"] == 1 and s["harness"] == "pass"
    assert "did not pass the check" in FakeLlama.prompts[1]


def test_findings_go_back_to_the_local_model(tmp_path, zhen_claude, fake_llama):
    FakeLlama.answers = [answer(GOOD), answer(REVIEWED)]
    findings = [{"severity": "low", "step": "metadata", "finding": "description vague", "fix": "say disk and inode use"}]
    r, s = run(tmp_path, zhen_claude, fake_llama, findings)
    assert r.returncode == 0, r.stderr
    assert s["claude_findings"] == 1 and s["local_revisions"] == 1
    assert "description vague" in FakeLlama.prompts[1]
    assert "disk and inode use" in (Path(s["worktree"]) / REL).read_text()


def test_draft_that_never_passes_fails_without_claude_patch(tmp_path, zhen_claude, fake_llama):
    FakeLlama.answers = [answer(BROKEN)] * 3
    r, s = run(tmp_path, zhen_claude, fake_llama, [])
    assert r.returncode == 1
    assert s["harness"] == "fail" and s["claude_patched"] is False  # fake review never patches
