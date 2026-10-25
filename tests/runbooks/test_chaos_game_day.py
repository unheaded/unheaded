# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Dry-run test for runbooks/observe/chaos-game-day.yaml (R11). Nothing is
injected: the runbook's shape is checked (one injection at a time, each
manual, named with its hypothesis and immediately followed by its restore;
an abort section; announce, steady state, injections, write-up in order)."""
import re

import harness

RB = "runbooks/observe/chaos-game-day.yaml"
INJECT = re.compile(r"docker (stop|kill)\b|systemctl (stop|kill)\b|/api/v1/chaos/inject")
RESTORE = re.compile(r"docker start\b|systemctl start\b|DELETE[^\n]*/api/v1/chaos/rules")


def steps():
    return harness.steps(harness.load(RB))


def injections():
    return [i for i, s in enumerate(steps()) if INJECT.search(s.get("command") or "")]


def test_dry_run_lists_every_step():
    assert harness.dry_run(RB) == [s["name"] for s in steps()]


def test_injections_exist_and_are_manual_with_hypothesis():
    st = steps()
    inj = injections()
    assert len(inj) >= 3, "the brief names three injections"
    for i in inj:
        s = st[i]
        assert s["action"] == "manual", f"{s['name']} injects a fault but is {s['action']}"
        assert re.search(r"hypothes", f"{s.get('command', '')} {s.get('description', '')}", re.IGNORECASE), f"{s['name']} names no hypothesis"


def test_each_injection_is_immediately_followed_by_its_restore():
    st = steps()
    for i in injections():
        nxt = st[i + 1]
        assert RESTORE.search(nxt.get("command") or ""), f"{st[i]['name']} is not followed by a restore (next: {nxt['name']})"
        assert nxt["action"] == "manual", nxt["name"]
        assert not INJECT.search(nxt.get("command") or ""), f"{nxt['name']} restores and injects at once"


def test_one_injection_at_a_time():
    st = steps()
    for i in injections():
        assert len(INJECT.findall(st[i]["command"])) == 1, f"{st[i]['name']} injects more than one fault"


def test_abort_section_exists_before_the_first_injection():
    st = steps()
    abort = [i for i, s in enumerate(st) if "abort" in s["name"]]
    assert abort, "no abort step"
    assert min(abort) < min(injections())
    text = " ".join(f"{st[i].get('command', '')} {st[i].get('description', '')}" for i in abort).lower()
    assert "kill switch" in text or "/api/v1/chaos/rules" in text


def test_order_announce_steady_injections_writeup():
    names = [s["name"] for s in steps()]
    announce = min(i for i, n in enumerate(names) if "announce" in n)
    steady = min(i for i, n in enumerate(names) if "steady" in n)
    writeup = max(i for i, n in enumerate(names) if re.search(r"write-?up", n))
    inj = injections()
    assert announce < steady < min(inj) and max(inj) < writeup, names


def test_steady_state_steps_are_read_only():
    for s in steps():
        if ("steady" in s["name"] or s["name"].startswith("observe")) and s["action"] == "shell":
            for c in harness.commands(s["command"]):
                assert not INJECT.search(c), c
                assert c.split()[0] in ("curl", "jq", "mkdir", "tee", "date", "echo", "grep", "python3", "cat", "awk", "test", "true", "ls"), c


def test_east_is_out_of_scope():
    for s in steps():
        assert s.get("host") in (None, "west"), s["name"]
        assert "ssh" not in (s.get("command") or "").split(), s["name"]
