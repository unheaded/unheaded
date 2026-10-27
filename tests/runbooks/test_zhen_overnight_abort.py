# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Dry-run test for runbooks/observe/zhen-overnight-abort.yaml and the
morning report script it points at (S-CHAMPION-HARDENING Phase 13)."""
import subprocess

import harness

RB = "runbooks/observe/zhen-overnight-abort.yaml"
REPORT = harness.REPO / "scripts" / "zhen-morning-report.sh"


def test_dry_run_lists_every_step():
    rb = harness.load(RB)
    assert harness.dry_run(RB) == [s["name"] for s in harness.steps(rb)]


def test_stops_and_host_changes_are_manual():
    rb = harness.load(RB)
    for name in ("soft-stop", "hard-stop", "no-more-nights"):
        assert harness.step(rb, name)["action"] == "manual", name
    assert harness.step(rb, "hard-stop").get("irreversible") is True
    for s in rb.get("rollback") or []:
        assert s["action"] == "manual"


def test_soft_stop_names_the_units_stop_file():
    unit = (harness.REPO / "deploy/systemd/user/zhen-agentd-overnight.service").read_text()
    rb = harness.load(RB)
    assert "-overnight" in unit
    # The user unit takes zhen-agentd's default -stop-file, under XDG_STATE_HOME/unheaded.
    assert "-stop-file" not in unit
    assert "zhen-agentd.stop" in harness.step(rb, "soft-stop")["command"]
    params = {p["name"]: p["default"] for p in rb["parameters"]}
    assert params["STATE_DIR"].endswith("/.local/state/unheaded")


def test_no_sudo_for_a_user_unit():
    text = (harness.REPO / RB).read_text()
    assert "sudo " not in text and "systemctl --user" in text


def _report(*args):
    return subprocess.run(["bash", str(REPORT), *args], capture_output=True, text=True, timeout=30, check=False)


def test_report_newest_dated_and_list(tmp_path):
    for d, body in (("2026-10-28", "night one"), ("2026-10-29", "night two")):
        (tmp_path / f"{d}.md").write_text(body + "\n")
    (tmp_path / "notes.md").write_text("not a report\n")
    r = _report("--dir", str(tmp_path))
    assert r.returncode == 0 and r.stdout == "night two\n"
    r = _report("--dir", str(tmp_path), "--date", "2026-10-28")
    assert r.returncode == 0 and r.stdout == "night one\n"
    r = _report("--dir", str(tmp_path), "--list")
    assert r.stdout.split() == ["2026-10-28", "2026-10-29"]


def test_report_missing_and_bad_input(tmp_path):
    assert _report("--dir", str(tmp_path / "none")).returncode == 1
    assert _report("--dir", str(tmp_path)).returncode == 1
    (tmp_path / "2026-10-28.md").write_text("x\n")
    assert _report("--dir", str(tmp_path), "--date", "2026-10-30").returncode == 1
    assert _report("--dir", str(tmp_path), "--date", "../etc/passwd").returncode == 2
    assert _report("--bogus").returncode == 2
