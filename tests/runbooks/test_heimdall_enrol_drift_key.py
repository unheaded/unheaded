# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Dry-run test for runbooks/security/heimdall-enrol-drift-key.yaml
(S-IRON-GATE Step 282). Nothing touches west, east or Wotan: the runbook is
dry-run and its text is checked."""
import re

import harness

RB = "runbooks/security/heimdall-enrol-drift-key.yaml"
ORDER = ["generate-drift-key", "print-public-key", "enrol-in-wotan", "restart-wotan",
         "start-heimdall", "provoke-drift-in-a-scratch-copy"]


def test_dry_run_lists_every_step_in_order():
    names = harness.dry_run(RB)
    assert names == [s["name"] for s in harness.steps(harness.load(RB))]
    assert names == ORDER


def test_every_host_changing_step_is_manual():
    rb = harness.load(RB)
    for s in harness.steps(rb):
        text = s["command"]
        if re.search(r"\b(sudo|systemctl (start|restart|enable)|sudoedit|unheaded-keygen -algo mldsa65 -owner)\b", text):
            assert s["action"] == "manual", s["name"]
    assert harness.step(rb, "generate-drift-key").get("irreversible") is True


def test_shell_steps_only_read():
    rb = harness.load(RB)
    for s in harness.steps(rb) + list(rb.get("verification") or []):
        if s.get("action") != "shell":
            continue
        for c in harness.commands(s["command"]):
            assert not re.search(r"\b(sudo|rm|kill|restart|start|chmod|chown|tee)\b", c), f"{s['name']}: {c}"


def test_key_is_mldsa65_owned_by_heimdall_and_never_the_repo_config():
    rb = harness.load(RB)
    gen = harness.step(rb, "generate-drift-key")["command"]
    assert "unheaded-keygen -algo mldsa65 -owner unheaded-heimdall heimdall-drift" in gen
    enrol = harness.step(rb, "enrol-in-wotan")["command"]
    assert '"drift."' in enrol and "Never commit" in enrol
    assert "configs/wotan.yaml" not in enrol.replace("repo's configs/wotan.yaml", "")


def test_drill_uses_a_scratch_copy_never_a_real_file():
    drill = harness.step(harness.load(RB), "provoke-drift-in-a-scratch-copy")["command"]
    assert "mktemp -d" in drill
    assert "--manifest \"$D/manifest.yaml\"" in drill
    # The only file it changes is the scratch canary.
    for m in re.findall(r">>\s*(\S+)", drill):
        assert m == '"$D/canary"', m
    assert "/etc/unheaded/heimdall/mjolnir.yaml" not in drill
    assert "rm -rf \"$D\"" in drill


def test_every_path_named_exists():
    text = (harness.REPO / RB).read_text()
    for rel in set(re.findall(r"(?<![\w/])(?:scripts|deploy|runbooks|configs|cmd|tests)/[\w./-]+\.(?:sh|py|yaml|md|conf|service)\b", text)):
        assert (harness.REPO / rel).is_file(), rel
