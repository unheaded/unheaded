# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Dry-run test for runbooks/data/well-restore-from-east-offsite.yaml (R4).

Nothing touches the Well, west's backup disk or east: the runbook is
dry-run and its text is checked. The live test runs the repo's own
backup/restore round trip on throwaway servers (WELL_LIVE_TESTS=1)."""
import re
import subprocess

import harness
import pytest

RB = "runbooks/data/well-restore-from-east-offsite.yaml"
LIVE_FLAG = "WELL_LIVE_TESTS"
EAST_SETS = "/var/backups/unheaded-well-west"


def all_steps():
    rb = harness.load(RB)
    return harness.steps(rb) + list(rb.get("rollback") or [])


def test_dry_run_lists_every_step():
    assert harness.dry_run(RB) == [s["name"] for s in harness.steps(harness.load(RB))]


def test_every_script_path_exists_in_the_repo():
    text = (harness.REPO / RB).read_text()
    for rel in set(re.findall(r"\bscripts/[\w./-]+\.(?:sh|py)\b", text)):
        assert (harness.REPO / rel).is_file(), rel
    for installed in set(re.findall(r"/usr/local/lib/unheaded/well/([\w.-]+)", text)):
        assert (harness.REPO / "scripts" / "well" / installed).is_file(), installed
    for runbook in set(re.findall(r"\brunbooks/[\w/-]+\.yaml\b", text)):
        assert (harness.REPO / runbook).is_file(), runbook


def test_host_guards():
    east = [s for s in all_steps() if s.get("action") == "shell" and EAST_SETS in (s.get("command") or "")]
    assert east, "no shell step verifies the set on east"
    for s in east:
        assert s.get("host") == "east", f"{s['name']} reads east's sets but is not host: east"
    west = [s for s in all_steps() if s.get("action") == "shell" and re.search(r"restore-drill|\$\{?STAGE", s.get("command") or "")]
    assert west, "no shell step works on the staged copy on west"
    for s in west:
        assert s.get("host") == "west", f"{s['name']} works on west's copy but is not host: west"


def test_checksums_on_east_come_first():
    names = [s["name"] for s in harness.steps(harness.load(RB))]
    steps = harness.steps(harness.load(RB))
    first_east = min(i for i, s in enumerate(steps) if "sha256sum -c" in (s.get("command") or "") and s.get("host") == "east")
    first_drill = min(i for i, s in enumerate(steps) if "restore-drill" in (s.get("command") or ""))
    first_restore = min(i for i, s in enumerate(steps) if "pg_restore" in (s.get("command") or ""))
    assert first_east < first_drill < first_restore, names


def test_destructive_steps_are_manual():
    destructive = re.compile(r"pg_restore|psql\b.*globals\.sql|set-role-passwords|dropdb|DROP |scp\b")
    for s in all_steps():
        if destructive.search(s.get("command") or ""):
            assert s["action"] == "manual", f"{s['name']} is destructive but {s['action']}"
    for s in all_steps():
        assert "--clean" not in (s.get("command") or "") or s["action"] == "manual", s["name"]


def test_never_restores_over_a_running_well():
    text = (harness.REPO / RB).read_text().lower()
    assert re.search(r"never [^.\n]{0,60}(running|live) well", text), "the runbook must say never to restore over a running Well"


@pytest.mark.live
def test_live_backup_restore_round_trip_on_throwaway_servers():
    r = subprocess.run(["bash", str(harness.REPO / "scripts/well/test-backup-restore.sh")],
                       capture_output=True, text=True, timeout=1800, check=False)
    assert r.returncode == 0, r.stdout[-2000:] + r.stderr[-2000:]
