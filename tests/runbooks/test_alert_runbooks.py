# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Dry-run tests for the runbooks alert rules point at (S-WELL-SPRING Phases
6-7) that were written for them. Nothing
runs against a host. Each runbook dry-runs through the runner; every shell
step only reads (the commands are held to an allowlist); anything that
restarts or changes state is a manual step; and the alert rules that name
the runbook exist."""
import json
import os
import re
import subprocess

import harness
import pytest

RUNBOOKS = {
    "runbooks/observe/heartbeat-missing.yaml": ("HeartbeatMissing", "Watchdog"),
    "runbooks/observe/trace-collector-down.yaml": ("TraceCollectorDown",),
    "runbooks/observe/high-latency.yaml": ("TraceCollectorHighLatency",),
    "runbooks/observe/log-pipeline.yaml": ("JournalNotShipping", "JournalShipErrors", "VectorDown", "LogSpam", "LogErrorSpam", "LogSilence"),
    "runbooks/security/kev-feed-stale.yaml": ("KEVFeedStale", "KEVFeedExpired", "KEVPollerDown"),
    # S-WELL-SPRING Phase 11 detections (monitoring/prometheus/rules/detection.yml).
    "runbooks/security/ssh-login-new-source.yaml": ("SSHLoginNewSource", "SSHAuthFailureBurst"),
    "runbooks/security/outbound-volume-deviation.yaml": ("OutboundVolumeDeviation",),
    "runbooks/observe/zhen-scheduler-stalled.yaml": ("ZhenSchedulerStalled",),
    # The ci-develop SLO's own runbook (policy.yaml runbook:), no hand rule.
    "runbooks/observe/ci-red.yaml": (),
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


# The ci-develop SLI's writer (scripts/monitoring/ci-status-textfile.sh),
# against a throwaway API on 127.0.0.1 serving a fixed answer.
CI_SCRIPT = harness.REPO / "scripts/monitoring/ci-status-textfile.sh"


def _run(name, conclusion, created, status="completed"):
    return {"name": name, "status": status, "conclusion": conclusion, "created_at": created}


CI_RUNS = {"workflow_runs": [
    _run("Green", "success", "2026-10-27T10:00:00Z"),
    _run("Green", "failure", "2026-10-26T10:00:00Z"),
    _run("Red", "failure", "2026-10-27T10:00:00Z"),
    _run("Red", "success", "2026-10-26T10:00:00Z"),
    # A newer push cancelled the latest run: judge the one before it.
    _run("Superseded", "cancelled", "2026-10-27T11:00:00Z"),
    _run("Superseded", "success", "2026-10-27T09:00:00Z"),
    # Still running: judge the last finished one.
    _run("Running", None, "2026-10-27T12:00:00Z", status="in_progress"),
    _run("Running", "failure", "2026-10-27T08:00:00Z"),
    _run('Odd "name" \\ here', "success", "2026-10-27T08:00:00Z"),
]}


class _API:
    def __init__(self, body, code=200):
        import http.server
        import threading

        payload = json.dumps(body).encode()

        class H(http.server.BaseHTTPRequestHandler):
            def do_GET(self):
                self.send_response(code)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(payload)

            def log_message(self, *a):
                pass

        self.srv = http.server.HTTPServer(("127.0.0.1", 0), H)
        self.url = f"http://127.0.0.1:{self.srv.server_address[1]}"
        threading.Thread(target=self.srv.serve_forever, daemon=True).start()

    def close(self):
        self.srv.shutdown()


def _ci_status(tmp_path, api):
    return subprocess.run(["bash", str(CI_SCRIPT), str(tmp_path)], capture_output=True, text=True, timeout=60,
                          env={**os.environ, "CI_STATUS_API": api}, check=False)


@harness.requires("jq", "curl")
def test_ci_status_latest_decided_run_per_workflow(tmp_path):
    api = _API(CI_RUNS)
    try:
        r = _ci_status(tmp_path, api.url)
    finally:
        api.close()
    assert r.returncode == 0, r.stderr
    text = (tmp_path / "ci_status.prom").read_text()
    got = dict(re.findall(r'^unheaded_ci_run_success\{workflow="((?:[^"\\]|\\.)*)"\} ([01])$', text, re.MULTILINE))
    assert got == {"Green": "1", "Red": "0", "Superseded": "1", "Running": "0", 'Odd \\"name\\" \\\\ here': "1"}, got
    assert re.search(r"^unheaded_ci_run_checked_timestamp_seconds \d+$", text, re.MULTILINE)


@harness.requires("jq", "curl")
def test_ci_status_keeps_the_old_file_when_the_api_fails(tmp_path):
    (tmp_path / "ci_status.prom").write_text("old\n")
    api = _API({"message": "API rate limit exceeded"}, code=403)
    try:
        r = _ci_status(tmp_path, api.url)
    finally:
        api.close()
    assert r.returncode != 0
    assert (tmp_path / "ci_status.prom").read_text() == "old\n"
    assert [p.name for p in tmp_path.iterdir()] == ["ci_status.prom"], "a temp file was left behind"
