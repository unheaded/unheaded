# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Dry-run test for runbooks/wotan/oom-state-recovery.yaml (R3), against a
throwaway Wotan: one subscriber, kill -9, restart, then the runbook's verify
steps pointed at the throwaway through WOTAN_URL. The subscriber re-subscribes
every 2 s while Wotan is gone, which is what pkg/wotan-client's rejoin does
(7998692b); the client library itself is not under test here."""
import threading
import time

import harness

RB = "runbooks/wotan/oom-state-recovery.yaml"


def test_dry_run_lists_detection_before_recovery():
    names = harness.dry_run(RB)
    detect = [i for i, n in enumerate(names) if n.startswith("detect-")]
    restart = names.index("restart-wotan")
    verify = [i for i, n in enumerate(names) if n.startswith("verify-")]
    assert detect and verify
    assert max(detect) < restart < min(verify)


def test_restart_is_manual_and_detection_is_read_only():
    rb = harness.load(RB)
    assert harness.step(rb, "restart-wotan")["action"] == "manual"
    for s in harness.steps(rb):
        if s["name"].startswith("detect-"):
            assert s["action"] == "shell"
            for c in harness.commands(s["command"]):
                assert c.split()[0] in ("journalctl", "grep", "docker", "systemctl", "tail", "curl", "echo"), c
                if c.startswith(("docker", "systemctl")):
                    assert c.split()[1] in ("inspect", "status"), c


def test_wotan_comes_back_and_members_rejoin(throwaway_wotan):
    w = throwaway_wotan
    rb = harness.load(RB)
    env = {"WOTAN_URL": w.url}
    stop = threading.Event()

    def client():  # a service that rejoins whenever Wotan has forgotten it
        while not stop.is_set():
            try:
                w.subscribe("s2b.oom", "service")
            except OSError:
                pass
            time.sleep(2)

    t = threading.Thread(target=client, daemon=True)
    t.start()
    try:
        harness.run_step(rb, "verify-health", env=env)
        w.kill9()
        assert harness.run_step(rb, "verify-health", env=env, check=False).returncode != 0
        # detection steps still answer (they report, they do not fail)
        assert harness.run_step(rb, "detect-metrics", env=env).returncode == 0
        t0 = time.time()
        w.start()  # what restart-wotan does to the real unit
        harness.run_step(rb, "verify-health", env=env)
        harness.run_step(rb, "verify-members-approved", env=env, timeout=150)
        assert time.time() - t0 < 60, "the subscriber was not back within 60 s"
        harness.run_step(rb, "verify-members-pending", env=env, timeout=90)
    finally:
        stop.set()
        t.join(timeout=5)
