# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Dry-run test for runbooks/infra/host-rebuild-exercise.yaml (contingency
plan section 5). Nothing touches west or east: the runbook is dry-run, its
text is checked, and the one local step runs against a temp log."""
import re

import harness

RB = "runbooks/infra/host-rebuild-exercise.yaml"
READ_ONLY = ("grep", "uname", "systemctl", "awk", "ls", "du", "true", "curl", "python3", "ssh", "echo", "tee", "date", "set")


def test_dry_run_lists_every_step():
    assert harness.dry_run(RB, env={"LOG": "/dev/null"}) == [s["name"] for s in harness.steps(harness.load(RB))]


def test_copies_are_preserved_before_the_wipe():
    rb = harness.load(RB)
    assert harness.index(rb, "inventory-east") < harness.index(rb, "preserve-east-copies") < harness.index(rb, "reinstall-os")
    assert harness.index(rb, "reinstall-os") < harness.index(rb, "restore-copies") < harness.index(rb, "verify-from-west")


def test_every_host_changing_step_is_manual():
    for s in harness.steps(harness.load(RB)):
        if s["action"] != "shell":
            continue
        for c in harness.commands(s["command"]):
            word = c.split()[0]
            assert word in READ_ONLY, f"{s['name']}: {c}"
            if word == "systemctl":
                assert c.split()[1] in ("list-units", "status", "is-active"), c
        assert s.get("host") in ("west", "east"), f"{s['name']} has no host"


def test_wipe_is_irreversible_and_secrets_are_reissued():
    rb = harness.load(RB)
    assert harness.step(rb, "reinstall-os").get("irreversible") is True
    text = " ".join(harness.step(rb, "reissue-secrets")["command"].lower().split())
    assert "never copy old secrets back" in text


def test_every_path_named_exists():
    text = (harness.REPO / RB).read_text()
    for rel in set(re.findall(r"(?<![\w/])(?:scripts|deploy|runbooks|compliance|wiki)/[\w./-]+\.(?:sh|py|yaml|md|conf)\b", text)):
        assert (harness.REPO / rel).is_file(), rel


def test_record_start_appends_a_stamp(tmp_path):
    log = tmp_path / "log.md"
    harness.run_step(harness.load(RB), "record-start", env={"LOG": str(log)})
    assert re.fullmatch(r"\| \d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ \| start \|\n", log.read_text())
