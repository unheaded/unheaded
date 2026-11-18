# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Dry-run test for runbooks/infra/host-settings-drift.yaml (S-WEST-GROUND
Step 74). Nothing touches a host: the runbook is dry-run and its text is
checked."""
import re

import harness

RB = "runbooks/infra/host-settings-drift.yaml"
ORDER = ["list-drift-unprivileged", "list-drift-as-root", "see-the-difference", "decide-and-change"]


def test_dry_run_lists_every_step_in_order():
    names = harness.dry_run(RB)
    assert names == [s["name"] for s in harness.steps(harness.load(RB))]
    assert names == ORDER


def test_shell_steps_only_read():
    rb = harness.load(RB)
    for s in harness.steps(rb):
        if s.get("action") != "shell":
            continue
        for c in harness.commands(s["command"]):
            assert not re.search(r"\b(sudo|rm|kill|restart|start|chmod|chown|tee|install)\b", c), f"{s['name']}: {c}"


def test_no_step_changes_a_file():
    rb = harness.load(RB)
    for s in harness.steps(rb):
        text = s["command"]
        assert not re.search(r"\bsed -i\b|\binstall\b|\bcp\b|\bmv\b|\brm\b|\btee\b", text), s["name"]
        assert not re.search(r"(?:^|\s)>{1,2}\s*[/\w$\"]", text), s["name"]  # a shell redirection to a file


def test_redact_masks_every_key_kind_and_passwords():
    see = harness.step(harness.load(RB), "see-the-difference")["command"]
    assert see.count("PrivateKey|PresharedKey|PublicKey") == 2
    assert see.count("s/(password)[[:space:]]+[^[:space:]]+/\\1 <redacted>/") == 2


def test_every_path_named_exists():
    text = (harness.REPO / RB).read_text()
    for rel in set(re.findall(r"(?<![\w/])(?:scripts|deploy|runbooks|configs|cmd|tests)/[\w./-]+\.(?:sh|py|yaml|md|conf|service|tsv)\b", text)):
        assert (harness.REPO / rel).is_file(), rel
