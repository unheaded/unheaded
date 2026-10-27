# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""run-runbook.py publishes per-step progress on Wotan `runbooks.progress`
when RUNBOOK_PROGRESS_WOTAN names a Wotan (S-CHAMPION-HARDENING Step 224),
and the Kanban board shows `step k/n` from it. Progress is best effort: a
Wotan that is down or refuses never changes a run's outcome. Every run here
touches files under tmp_path only."""
import json
import os
import subprocess

import harness
import yaml

RUNNER = harness.RUNNER
TOPIC = "runbooks.progress"


def write_runbook(tmp_path, steps, name="progress-fixture"):
    rb = {
        "apiVersion": "runbook/v1",
        "kind": "Runbook",
        "metadata": {"name": name, "description": "fixture", "owner": "test", "estimated_duration": "1m",
                     "risk": "low", "requires_sudo": False},
        "env": {"DIR": str(tmp_path)},
        "steps": steps,
    }
    p = tmp_path / f"{name}.yaml"
    p.write_text(yaml.safe_dump(rb, sort_keys=False))
    return p


def run(path, env_extra, *args):
    env = {k: v for k, v in os.environ.items() if not k.startswith("RUNBOOK_")}
    env.update(env_extra)
    r = subprocess.run(["python3", "-B", str(RUNNER), *args, str(path)], capture_output=True, text=True,
                       timeout=60, check=False, cwd=harness.REPO, env=env)
    return r.returncode, r.stdout + r.stderr


def events(wotan):
    code, body = harness.http("GET", f"{wotan.url}/api/v1/topics/{TOPIC}/messages?after_seq=0")
    assert code == 200, body
    return [json.loads(m["payload"]) for m in body["messages"]]


TWO_STEPS = [
    {"name": "first", "action": "shell", "command": "touch ${DIR}/one"},
    {"name": "second", "action": "shell", "command": "touch ${DIR}/two"},
]


def test_steps_and_outcome_are_published(wotan_binary, tmp_path):
    w = harness.Wotan(wotan_binary, tmp_path, auto_approve=("runbook-runner",)).start()
    try:
        p = write_runbook(tmp_path, TWO_STEPS)
        code, out = run(p, {"RUNBOOK_PROGRESS_WOTAN": w.addr, "RUNBOOK_REF": "test/progress-fixture"})
        assert code == 0, out
        got = events(w)
    finally:
        w.stop()
    seen = [(e["step"], e["total"], e["state"]) for e in got]
    assert seen == [(1, 2, "running"), (1, 2, "ok"), (2, 2, "running"), (2, 2, "ok"), (2, 2, "done")], got
    assert {e["runbook"] for e in got} == {"test/progress-fixture"}
    assert got[0]["name"] == "first" and got[2]["name"] == "second"
    assert all(e["dry_run"] is False and isinstance(e["ts"], int) for e in got)


def test_failed_step_is_published_and_the_run_still_fails(wotan_binary, tmp_path):
    w = harness.Wotan(wotan_binary, tmp_path, auto_approve=("runbook-runner",)).start()
    try:
        p = write_runbook(tmp_path, [{"name": "boom", "action": "shell", "command": "exit 3"}] + TWO_STEPS)
        code, out = run(p, {"RUNBOOK_PROGRESS_WOTAN": w.addr, "RUNBOOK_REF": "test/progress-fixture"})
        assert code == 1, out
        got = events(w)
    finally:
        w.stop()
    assert [(e["step"], e["state"]) for e in got] == [(1, "running"), (1, "failed")], got
    assert not (tmp_path / "one").exists()


def test_ref_defaults_to_the_path_under_runbooks(wotan_binary, tmp_path):
    w = harness.Wotan(wotan_binary, tmp_path, auto_approve=("runbook-runner",)).start()
    try:
        d = tmp_path / "runbooks" / "test"
        d.mkdir(parents=True)
        p = write_runbook(d, TWO_STEPS[:1], name="from-path")
        code, out = run(p, {"RUNBOOK_PROGRESS_WOTAN": w.addr}, "--dry-run")
        assert code == 0, out
        got = events(w)
    finally:
        w.stop()
    assert {e["runbook"] for e in got} == {"test/from-path"}, got
    assert all(e["dry_run"] is True for e in got)


def test_unapproved_publisher_does_not_change_the_outcome(wotan_binary, tmp_path):
    # runbook-runner is not on this Wotan's auto_approve list: the join stays
    # pending, nothing is published, the run succeeds.
    w = harness.Wotan(wotan_binary, tmp_path).start()
    try:
        p = write_runbook(tmp_path, TWO_STEPS)
        code, out = run(p, {"RUNBOOK_PROGRESS_WOTAN": w.addr, "RUNBOOK_REF": "test/progress-fixture"})
        assert code == 0, out
        assert events(w) == []
    finally:
        w.stop()
    assert "[progress] off" in out, out
    assert (tmp_path / "two").exists()


def test_unreachable_wotan_does_not_change_the_outcome(tmp_path):
    p = write_runbook(tmp_path, TWO_STEPS)
    code, out = run(p, {"RUNBOOK_PROGRESS_WOTAN": f"127.0.0.1:{harness.free_port()}"})
    assert code == 0, out
    assert "[progress] off" in out, out
    assert (tmp_path / "two").exists()


def test_no_env_no_publish_attempt(tmp_path):
    p = write_runbook(tmp_path, TWO_STEPS)
    code, out = run(p, {})
    assert code == 0, out
    assert "[progress]" not in out, out


def test_bad_ref_is_refused_not_published(tmp_path):
    p = write_runbook(tmp_path, TWO_STEPS)
    code, out = run(p, {"RUNBOOK_PROGRESS_WOTAN": f"127.0.0.1:{harness.free_port()}", "RUNBOOK_REF": "../etc/x"})
    assert code == 0, out
    assert "[progress] off: runbook ref" in out, out
