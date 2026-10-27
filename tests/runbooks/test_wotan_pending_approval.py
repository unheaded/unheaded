# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Dry-run test for runbooks/wotan/pending-subscription-approval.yaml (R2),
against a throwaway Wotan on free ports with a temp allowlist.

Plan amendment (H12): the plan's test had the pending subscriber "appear in
/api/v1/admin/pending". Wotan's admin API is off by default (S-CLEAN-SLATE:
routes registered only with -admin and the admin key), so the call answers
404. The test pins that instead, detects the
pending member the way the runbook does (the subscribe answer, the log line,
the unheaded_chat_members_pending gauge), then allowlists and restarts."""
import time

import harness
import yaml

RB = "runbooks/wotan/pending-subscription-approval.yaml"
UNKNOWN = "s2b-unknown"
TOPIC = "s2b.pending"


def wait_pending_gauge(w, timeout=25):
    deadline = time.time() + timeout
    while time.time() < deadline:
        _, text = harness.http("GET", w.url + "/metrics")
        for line in str(text).splitlines():
            if line.startswith("unheaded_chat_members_pending") and float(line.split()[-1]) > 0:
                return True
        time.sleep(1)
    return False


def test_dry_run_lists_every_step():
    rb = harness.load(RB)
    assert harness.dry_run(RB) == [s["name"] for s in harness.steps(rb)]


def test_restarts_and_host_edits_are_manual():
    rb = harness.load(RB)
    for s in harness.steps(rb) + list(rb.get("rollback") or []):
        c = s.get("command") or ""
        if "restart" in c or "--force-recreate" in c:
            assert s["action"] == "manual" and s.get("irreversible") is True, s["name"]
    assert harness.step(rb, "add-to-auto-approve")["action"] == "manual"
    assert harness.step(rb, "decide-approval")["action"] == "manual"


def test_admin_api_is_not_the_path():
    text = (harness.REPO / RB).read_text()
    for s in harness.steps(harness.load(RB)):
        assert "/api/v1/admin/approve" not in (s.get("command") or ""), s["name"]
    assert "404" in text  # the runbook says why


def test_known_name_is_found_and_unknown_is_not():
    rb = harness.load(RB)
    assert harness.run_step(rb, "check-display-name", env={"NAME": "kanban-app"}, cwd=harness.REPO).returncode == 0
    assert harness.run_step(rb, "check-display-name", env={"NAME": UNKNOWN}, cwd=harness.REPO, check=False).returncode != 0


def test_pending_then_allowlisted_after_restart(throwaway_wotan, tmp_path):
    w = throwaway_wotan
    rb = harness.load(RB)
    env = {"WOTAN_URL": w.url, "NAME": UNKNOWN, "TOPIC": TOPIC}

    # pending, and the admin API (off by default) cannot list it (H12)
    code, body = w.subscribe(TOPIC, UNKNOWN)
    assert code == 201 and body["subscriber"]["status"] == "pending"
    code, _ = harness.http("GET", w.url + "/api/v1/admin/pending")
    assert code == 404
    # detection the runbook uses: the log line and the gauge (the precondition)
    deadline = time.time() + 10
    while "topic_subscriber_pending_approval" not in w.log.read_text() and time.time() < deadline:
        time.sleep(0.2)
    assert UNKNOWN in next(ln for ln in w.log.read_text().splitlines() if "topic_subscriber_pending_approval" in ln)
    assert wait_pending_gauge(w), "unheaded_chat_members_pending never rose"
    pre = rb["preconditions"][0]["check"]
    assert harness.run_step({"steps": [{"name": "pre", "command": pre}]}, "pre", env=env, check=False).returncode == 0

    # the runbook's verify: refused while the name is not allowlisted
    assert harness.run_step(rb, "verify-approval", env=env, check=False).returncode != 0

    # the operator's manual edit (add-to-auto-approve), then the restart
    cfg = yaml.safe_load(w.config.read_text())
    cfg["topics"]["auto_approve"].append(UNKNOWN)
    w.config.write_text(yaml.safe_dump(cfg))
    w.restart()
    assert harness.run_step(rb, "verify-approval", env=env).returncode == 0


def test_verify_needs_name_and_topic():
    rb = harness.load(RB)
    r = harness.run_step(rb, "verify-approval", env={"WOTAN_URL": "http://127.0.0.1:9"}, check=False)
    assert r.returncode != 0 and "set NAME" in r.stderr
