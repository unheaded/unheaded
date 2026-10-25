# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Dry-run test for runbooks/security/claude-api-key-rotation-spend-breach.yaml (R9).

No key is used and nothing is sent: zhen-claude gets a dummy credential and
an API base URL nobody listens on, so a request that got past the spend cap
would fail with a connection error instead of the cap error the test wants.
The stop-file step runs against a temp state dir. The overnight worker that
honours the stop file is Phase 7 (another branch); here the file is checked."""
import json
import os
import re
import shutil
import subprocess
from datetime import UTC
from pathlib import Path

import harness
import pytest

RB = "runbooks/security/claude-api-key-rotation-spend-breach.yaml"


@pytest.fixture(scope="module")
def zhen_claude(tmp_path_factory):
    out = tmp_path_factory.mktemp("zc") / "zhen-claude"
    r = subprocess.run(["go", "build", "-o", str(out), "./cmd/zhen-claude"], cwd=harness.REPO,
                       capture_output=True, text=True, timeout=600, check=False)
    if r.returncode != 0:
        pytest.skip(f"zhen-claude does not build: {r.stderr[-300:]}")
    return out


def run_zc(binary, ledger, tmp_path):
    env = {k: v for k, v in os.environ.items() if not k.startswith("ANTHROPIC_")}
    env.update(ANTHROPIC_API_KEY="dummy-not-a-real-key", ANTHROPIC_BASE_URL=f"http://127.0.0.1:{harness.free_port()}",
               XDG_STATE_HOME=str(tmp_path / "state"), HOME=os.environ.get("HOME", str(tmp_path)))
    return subprocess.run([str(binary), "-tools", "none", "-ledger", str(ledger), "-daily-cap-usd", "0.50",
                           "-max-tokens", "1024", "-effort", "low", "-p", "x"],
                          capture_output=True, text=True, timeout=120, env=env, cwd=harness.REPO, check=False)


def test_dry_run_lists_every_step():
    assert harness.dry_run(RB) == [s["name"] for s in harness.steps(harness.load(RB))]


def test_no_step_prints_the_key():
    text = (harness.REPO / RB).read_text()
    assert not re.search(r"echo[^\n]*ANTHROPIC", text)
    for s in harness.steps(harness.load(RB)):
        c = s.get("command") or ""
        assert not re.search(r"\$\{?ANTHROPIC_(API_KEY|AUTH_TOKEN)", c), f"{s['name']} expands the key"
        assert not re.search(r"\b(cat|less|more|head|tail|grep)\b[^\n|]*(zhen-agentd\.env|anthropic\.env)", c), f"{s['name']} reads an env file's content"


def test_rotation_is_manual_and_stop_comes_first():
    st = harness.steps(harness.load(RB))
    names = [s["name"] for s in st]
    rot = [i for i, s in enumerate(st) if re.search(r"console|revoke|rotate", f"{s['name']} {s.get('description', '')}", re.IGNORECASE)]
    assert rot and all(st[i]["action"] == "manual" for i in rot)
    stop = min(i for i, s in enumerate(st) if "zhen-agentd.stop" in (s.get("command") or "") or "systemctl stop zhen-agentd" in (s.get("command") or "") or "--user stop" in (s.get("command") or ""))
    assert stop < min(rot), names


def test_stop_step_creates_the_stop_file(tmp_path):
    rb = harness.load(RB)
    st = [s for s in harness.steps(rb) if s["action"] == "shell" and "zhen-agentd.stop" in (s.get("command") or "")]
    assert st, "no shell step touches the stop file"
    state = tmp_path / "state"
    state.mkdir()
    fakebin = tmp_path / "bin"
    fakebin.mkdir()
    (fakebin / "systemctl").write_text("#!/bin/sh\necho \"fake systemctl $*\"\nexit 0\n")
    (fakebin / "systemctl").chmod(0o755)
    harness.run_step(rb, st[0]["name"], env={"STATE": str(state), "PATH": f"{fakebin}:{os.environ['PATH']}"})
    assert (state / "zhen-agentd.stop").exists()


@pytest.mark.skipif(shutil.which("gitleaks") is None and not Path.home().joinpath(".local/bin/gitleaks").exists(),
                    reason="zhen-claude's privacy tier needs gitleaks before it sends anything")
def test_cap_refuses_before_sending(zhen_claude, tmp_path):
    from datetime import datetime
    day = datetime.now(UTC).strftime("%Y-%m-%d")
    ledger = tmp_path / "spend.json"
    ledger.write_text(json.dumps({"day": day, "spent_nano_usd": 600_000_000, "requests": 9}))  # $0.60 > $0.50 cap
    r = run_zc(zhen_claude, ledger, tmp_path)
    assert r.returncode != 0
    assert "daily spend cap reached" in r.stderr, r.stderr[-1500:]
    assert "connection refused" not in r.stderr.lower()
    # an unparseable ledger refuses every request
    ledger.write_text("{not json")
    r = run_zc(zhen_claude, ledger, tmp_path)
    assert r.returncode != 0 and "corrupt" in r.stderr, r.stderr[-1500:]
