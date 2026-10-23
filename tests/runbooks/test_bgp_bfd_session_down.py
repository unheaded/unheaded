# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Dry-run test for runbooks/network/bgp-bfd-session-down.yaml (S-TWO-BRAINS R1).

Nothing here touches a router: the runbook is dry-run, and every shell step
is held to a read-only allowlist; changes must be manual. The live test runs
the read steps on a lab host with FRR (FRR_LAB=1, e.g. inside the
nixos/tests/frr.nix VM)."""
import re

import pytest

import harness

RB = "runbooks/network/bgp-bfd-session-down.yaml"
LIVE_FLAG = "FRR_LAB"


def read_only(cmd):
    w = cmd.split()
    head = w[0]
    if head == "vtysh":
        return re.fullmatch(r"vtysh -c ['\"]show [^'\"]+['\"]", cmd) is not None
    if head == "ip":
        return "show" in w and not ({"add", "del", "set", "change", "replace", "flush"} & set(w))
    if head == "wg":
        return w[1:2] == ["show"] and "showconf" not in w
    if head == "systemctl":
        return w[1] in ("is-active", "status", "show")
    if head == "git":
        return "log" in w and not ({"push", "commit", "reset", "checkout"} & set(w))
    if head == "journalctl":
        return True
    return head in ("ping", "command", "grep", "jq", "awk", "tail", "head", "date", ":", "test", "echo", "true")


def shell_steps():
    return [s for s in harness.steps(harness.load(RB)) if s["action"] == "shell"]


def test_dry_run_lists_every_step():
    rb = harness.load(RB)
    assert harness.dry_run(RB) == [s["name"] for s in harness.steps(rb)]


@pytest.mark.parametrize("field", ["command", "verify"])
def test_every_shell_step_is_read_only(field):
    bad = [(s["name"], c) for s in shell_steps() for c in harness.commands(s.get(field)) if not read_only(c)]
    assert not bad, f"not on the read-only allowlist: {bad}"


def test_every_change_is_manual():
    changes = re.compile(r"clear bgp|systemctl (restart|reload|stop)|frr\.nix.*(edit|rebuild)|configure terminal|nixos-rebuild", re.I)
    for s in harness.steps(harness.load(RB)) + list(harness.load(RB).get("rollback") or []):
        text = f"{s.get('command', '')}\n{s.get('description', '')}"
        if changes.search(text):
            assert s["action"] == "manual", f"{s['name']} changes the router but is {s['action']}"
            assert s.get("irreversible") is True, s["name"]


def test_peer_is_a_parameter_without_default_and_guarded():
    rb = harness.load(RB)
    peer = [p for p in rb["parameters"] if p["name"] == "PEER"]
    assert peer and not peer[0].get("default")
    for s in shell_steps():
        for f in ("command", "verify"):
            t = s.get(f) or ""
            if "PEER" in t:
                assert "${PEER:?" in t, f"{s['name']}.{f} uses PEER unguarded"
                assert "'$PEER'" not in t and "'${PEER}'" not in t, f"{s['name']}.{f} single-quotes PEER"


def test_no_step_prints_wireguard_keys():
    for s in shell_steps():
        assert "showconf" not in f"{s['command']}{s.get('verify', '')}"


def test_far_side_and_escalation_are_manual_notes():
    names = {s["name"]: s for s in harness.steps(harness.load(RB))}
    far = [s for s in names.values() if "birdc show bfd sessions" in s["command"]]
    assert far and all(s["action"] == "manual" for s in far)
    assert any("escalat" in s["name"] and s["action"] == "manual" for s in names.values())


@pytest.mark.live
def test_live_read_steps_on_an_frr_lab_host():
    rb = harness.load(RB)
    for s in shell_steps():
        if "PEER" in (s.get("command") or "") or s["name"].startswith("verify-after"):
            continue
        harness.run_step(rb, s["name"])
