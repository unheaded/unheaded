# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Self-tests of the runbook dry-run harness (S-TWO-BRAINS Step 269)."""
import harness


def test_dry_run_lists_service_health_sweep_steps():
    rb = harness.load("runbooks/observe/service-health-sweep.yaml")
    names = harness.dry_run("runbooks/observe/service-health-sweep.yaml")
    assert names, "no steps listed"
    assert names == [s["name"] for s in harness.steps(rb)]


def test_sandbox_repo_has_three_branches_and_a_local_origin(sandbox_git_repo):
    r = sandbox_git_repo
    heads = harness.git(r.origin, "for-each-ref", "--format=%(refname:short)", "refs/heads", env=r.env).split()
    assert sorted(heads) == ["develop", "main", "staging"]
    url = harness.git(r.work, "remote", "get-url", "origin", env=r.env)
    assert url == str(r.origin)  # a path in the temp dir: no network remote
    (r.work / "x").write_text("x\n")
    harness.git(r.work, "add", "x", env=r.env)
    harness.git(r.work, "commit", "-q", "-m", "x", env=r.env)
    harness.git(r.work, "push", "-q", "origin", "develop", env=r.env)
    assert harness.git(r.origin, "rev-parse", "develop", env=r.env) == harness.git(r.work, "rev-parse", "HEAD", env=r.env)


def test_sandbox_git_ignores_operator_signing(sandbox_git_repo):
    r = sandbox_git_repo
    assert harness.git(r.work, "log", "-1", "--format=%G?", env=r.env) == "N"


def test_run_step_expands_like_the_runner():
    rb = {"env": {"A": "from-env"}, "parameters": [{"name": "P", "default": "dflt"}],
          "steps": [{"name": "s", "action": "shell", "command": "echo ${A} ${P}"}]}
    assert harness.run_step(rb, "s").stdout.strip() == "from-env dflt"
    # the operator's environment overrides a parameter default, never env:
    assert harness.run_step(rb, "s", env={"P": "op", "A": "op"}).stdout.strip() == "from-env op"


def test_commands_splits_pipelines_and_strips_sudo():
    text = "set -euo pipefail\n# note\nsudo -n vtysh -c 'show bgp summary' | grep -q Est && echo ok\nX=1 ip -br link show wg0\n"
    assert harness.commands(text) == ["vtysh -c 'show bgp summary'", "grep -q Est", "echo ok", "ip -br link show wg0"]
    assert harness.commands("if systemctl is-active frr; then echo y; fi") == ["systemctl is-active frr", "echo y"]
    assert "date +%s" in harness.commands("now=$(date +%s)")


def test_throwaway_wotan_answers_and_restarts(throwaway_wotan):
    w = throwaway_wotan
    code, body = harness.http("GET", w.url + "/health")
    assert code == 200 and body["service"] == "wotan"
    code, body = w.subscribe("s2b.probe", "service")
    assert code == 201 and body["subscriber"]["status"] == "approved"
    w.kill9()
    w.start()
    code, body = harness.http("GET", w.url + "/health")
    assert code == 200 and body["total_members"] == 0  # memory store: state is gone
