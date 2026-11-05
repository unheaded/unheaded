# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""H5: run-runbook.py runs no command of a draft. A draft is dry-run only until
its owner approves it (ADR-024). Every run here is in a temp dir."""
import subprocess

import harness
import yaml

RUNNER = harness.RUNNER


def write_runbook(tmp_path, status):
    meta = {"name": "draft-fixture", "description": "fixture", "owner": "test", "estimated_duration": "1m",
            "risk": "low", "requires_sudo": False}
    if status is not None:
        meta["status"] = status
    rb = {
        "apiVersion": "runbook/v1", "kind": "Runbook", "metadata": meta, "env": {"DIR": str(tmp_path)},
        "preconditions": [{"check": "touch ${DIR}/precondition", "description": "a check"}],
        "steps": [{"name": "make-mark", "action": "shell", "command": "touch ${DIR}/mark"}],
    }
    p = tmp_path / "draft-fixture.yaml"
    p.write_text(yaml.safe_dump(rb, sort_keys=False))
    return p


def run(path, *args):
    r = subprocess.run(["python3", "-B", str(RUNNER), *args, str(path)], capture_output=True, text=True,
                       timeout=60, check=False, cwd=harness.REPO)
    return r.returncode, r.stdout + r.stderr


def test_a_draft_runs_nothing_live(tmp_path):
    code, out = run(write_runbook(tmp_path, "draft"))
    assert code != 0, out
    assert "DRAFT" in out and "--dry-run" in out, out
    assert not (tmp_path / "mark").exists()
    assert not (tmp_path / "precondition").exists()


def test_a_draft_runs_nothing_from_a_later_step(tmp_path):
    code, _ = run(write_runbook(tmp_path, "draft"), "--step", "0")
    assert code != 0
    assert not (tmp_path / "mark").exists()


def test_a_draft_dry_runs(tmp_path):
    code, out = run(write_runbook(tmp_path, "draft"), "--dry-run")
    assert code == 0, out
    assert not (tmp_path / "mark").exists()


def test_status_is_read_the_way_people_write_it(tmp_path):
    code, _ = run(write_runbook(tmp_path, " Draft "))
    assert code != 0
    assert not (tmp_path / "mark").exists()


def test_approved_and_unmarked_runbooks_run(tmp_path):
    for status in ("approved", None):
        code, out = run(write_runbook(tmp_path, status))
        assert code == 0, out
        assert (tmp_path / "mark").exists()
        (tmp_path / "mark").unlink()
