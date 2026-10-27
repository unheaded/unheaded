# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""run-runbook.py runs a runbook's `verification:` section after its steps
(S-CHAMPION-HARDENING Step 152). Every run here is in a temp dir: the steps
touch files under tmp_path and nothing else."""
import subprocess

import harness
import yaml

RUNNER = harness.RUNNER


def write_runbook(tmp_path, verification=None, steps=None, name="verify-fixture"):
    rb = {
        "apiVersion": "runbook/v1",
        "kind": "Runbook",
        "metadata": {"name": name, "description": "fixture", "owner": "test", "estimated_duration": "1m",
                     "risk": "low", "requires_sudo": False},
        "env": {"DIR": str(tmp_path)},
        "steps": steps or [{"name": "make-mark", "action": "shell", "command": "touch ${DIR}/mark"}],
    }
    if verification is not None:
        rb["verification"] = verification
    p = tmp_path / f"{name}.yaml"
    p.write_text(yaml.safe_dump(rb, sort_keys=False))
    return p


def run(path, *args):
    r = subprocess.run(["python3", "-B", str(RUNNER), *args, str(path)], capture_output=True, text=True,
                       timeout=60, check=False, cwd=harness.REPO)
    return r.returncode, r.stdout + r.stderr


ONE_PASS_ONE_FAIL = [
    {"name": "mark-exists", "description": "the step left its mark", "command": "test -f ${DIR}/mark"},
    {"name": "never-there", "description": "a file nothing creates", "command": "test -f ${DIR}/absent"},
]


def test_dry_run_lists_every_check_and_runs_none(tmp_path):
    p = write_runbook(tmp_path, ONE_PASS_ONE_FAIL)
    code, out = run(p, "--dry-run")
    assert code == 0, out
    assert "=== Verification (2 checks) ===" in out, out
    assert "the step left its mark" in out and "a file nothing creates" in out, out
    assert "verification: 2/2 (dry run)" in out, out
    assert not (tmp_path / "mark").exists()


def test_real_run_fails_naming_the_failing_check(tmp_path):
    p = write_runbook(tmp_path, ONE_PASS_ONE_FAIL)
    code, out = run(p)
    assert code == 1, out
    assert (tmp_path / "mark").exists()  # the steps ran first
    assert "[PASS] mark-exists" in out, out
    assert "[FAIL] never-there" in out, out
    assert "verification: 1/2 passed" in out, out
    assert "[FAILED] verification: never-there" in out, out
    assert "[SUCCESS]" not in out, out


def test_real_run_passes_when_every_check_passes(tmp_path):
    p = write_runbook(tmp_path, ONE_PASS_ONE_FAIL[:1])
    code, out = run(p)
    assert code == 0, out
    assert "verification: 1/1 passed" in out and "[SUCCESS]" in out, out


def test_check_key_and_manual_entries(tmp_path):
    ver = [
        {"name": "by-check", "check": "test -f ${DIR}/mark"},
        {"name": "by-hand", "action": "manual", "command": "look at the dashboard"},
    ]
    p = write_runbook(tmp_path, ver)
    code, out = run(p)
    assert code == 0, out
    assert "[PASS] by-check" in out, out
    assert "[MANUAL] by-hand" in out and "look at the dashboard" in out, out
    assert "verification: 1/1 passed, 1 manual" in out, out


def test_verification_does_not_run_after_a_failed_step(tmp_path):
    steps = [{"name": "boom", "action": "shell", "command": "touch ${DIR}/ran; false"}]
    ver = [{"name": "would-pass", "command": "touch ${DIR}/verified"}]
    p = write_runbook(tmp_path, ver, steps=steps)
    code, out = run(p)
    assert code == 1, out
    assert (tmp_path / "ran").exists() and not (tmp_path / "verified").exists(), out
    assert "Verification" not in out, out


def test_runbook_without_the_section_is_unchanged(tmp_path):
    p = write_runbook(tmp_path)
    code, out = run(p)
    assert code == 0, out
    assert "verification" not in out.lower(), out
    code, out = run(p, "--dry-run")
    assert code == 0 and "verification" not in out.lower(), out


def test_harness_dry_run_lists_steps_only(tmp_path):
    p = write_runbook(tmp_path, ONE_PASS_ONE_FAIL)
    assert harness.dry_run(p) == ["make-mark"]
