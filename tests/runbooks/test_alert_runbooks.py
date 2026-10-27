# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Dry-run tests for the runbooks alert rules point at (S-WELL-SPRING Phases
6-7): heartbeat-missing, trace-collector-down, high-latency, ci-red. Nothing
runs against a host. Each runbook dry-runs through the runner; every shell
step only reads (the commands are held to an allowlist); anything that
restarts or changes state is a manual step; and the alert rules that name
the runbook exist."""
import re
from pathlib import Path

import pytest

import harness

RUNBOOKS = {
    "runbooks/observe/heartbeat-missing.yaml": ("HeartbeatMissing", "Watchdog"),
    "runbooks/observe/trace-collector-down.yaml": ("TraceCollectorDown",),
    "runbooks/observe/high-latency.yaml": ("TraceCollectorHighLatency",),
}

READ_ONLY = {
    "curl", "python3", "ping", "ssh", "docker", "systemctl", "journalctl",
    "grep", "tail", "head", "jq", "bpftool", "echo", "ls", "cat", "date", "wc",
}
CHANGES_STATE = re.compile(r"\b(restart|start|stop|kill|enable|disable|rm|chmod|chown|up -d|down)\b")


@pytest.mark.parametrize("path", sorted(RUNBOOKS))
def test_dry_run_lists_every_step(path):
    rb = harness.load(path)
    assert harness.dry_run(path) == [s["name"] for s in harness.steps(rb)]


@pytest.mark.parametrize("path", sorted(RUNBOOKS))
def test_shell_steps_only_read(path):
    for s in harness.steps(harness.load(path)):
        if s["action"] != "shell":
            continue
        for c in harness.commands(s["command"]):
            argv = c.split()
            if argv[0] == "timeout":
                argv = argv[2:]
            assert argv[0] in READ_ONLY, f"{s['name']}: {c}"
            if argv[0] in ("docker", "systemctl"):
                assert len(argv) > 1 and argv[1] in ("ps", "logs", "inspect", "status", "is-active", "list-timers"), f"{s['name']}: {c}"
            if argv[0] == "ssh":
                remote = " ".join(argv[1:])
                assert not CHANGES_STATE.search(remote.replace("status", "")), f"{s['name']}: {c}"


@pytest.mark.parametrize("path", sorted(RUNBOOKS))
def test_state_changes_are_manual(path):
    rb = harness.load(path)
    for s in harness.steps(rb) + list(rb.get("rollback") or []):
        text = s.get("command") or ""
        if re.search(r"systemctl (restart|start|stop)|docker (restart|start|stop)|compose up", text):
            assert s["action"] == "manual", s["name"]


@pytest.mark.parametrize("path", sorted(RUNBOOKS))
def test_named_alerts_exist_and_point_here(path):
    rules = "\n".join(p.read_text() for p in (harness.REPO / "monitoring/prometheus/rules").glob("*.yml"))
    for alert in RUNBOOKS[path]:
        m = re.search(r"- alert: " + alert + r"\n(?:(?!\n      - (?:alert|record):).)*", rules, re.DOTALL)
        assert m, f"no alert {alert}"
        assert f'runbook_url: "{path}"' in m.group(0), f"{alert} does not point at {path}"
