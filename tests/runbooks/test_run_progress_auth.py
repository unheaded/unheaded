# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""run-runbook.py progress against a Wotan with its auth on (S-IRON-GATE
Phase 7, ADR-103 §6): the runner signs an EdDSA service token in Python
(scripts/svc_token.py) and the Go Wotan verifies it. Keys come from
unheaded-keygen at run time, in tmp_path; none is ever committed."""
import os
import shutil
import subprocess
import time

import harness
import pytest
from test_run_progress import TOPIC, TWO_STEPS, run, write_runbook

RUNNER = harness._RUNNER


def _python_has_cryptography():
    r = subprocess.run(["python3", "-c", "import cryptography.hazmat.primitives.asymmetric.ed25519"],
                       capture_output=True, check=False)
    return r.returncode == 0


needs_crypto = pytest.mark.skipif(not _python_has_cryptography(),
                                  reason="python3 has no cryptography package (the runner signs with it)")


@pytest.fixture(scope="module")
def keygen(tmp_path_factory):
    if shutil.which("go") is None:
        pytest.skip("no go on PATH to build unheaded-keygen")
    out = tmp_path_factory.mktemp("keygen") / "unheaded-keygen"
    r = subprocess.run(["go", "build", "-o", str(out), "./cmd/unheaded-keygen"], cwd=harness.REPO,
                       capture_output=True, text=True, timeout=600, check=False)
    if r.returncode != 0:
        pytest.skip(f"unheaded-keygen does not build here: {r.stderr.strip()[-300:]}")
    return out


def make_key(keygen, tmp_path, name):
    keys, trust = tmp_path / "keys", tmp_path / "trust.d"
    keys.mkdir(exist_ok=True, mode=0o700)
    trust.mkdir(exist_ok=True, mode=0o700)
    subprocess.run([str(keygen), "-dir", str(keys), "-trust-dir", str(trust), name], check=True,
                   capture_output=True, timeout=30)
    return keys / f"{name}.key", trust


def authed_wotan(wotan_binary, tmp_path, trust, callers):
    return harness.Wotan(wotan_binary, tmp_path, auto_approve=callers, env={
        "AUTH_ENABLED": "true", "AUTH_TRUST_DIR": str(trust), "AUTH_ALLOW_CALLERS": ",".join(callers),
        "AUTH_KEY_FILE": str(tmp_path / "keys" / "wotan.key")})


def token(key, sub):
    r = subprocess.run(["python3", "-I", str(harness.REPO / "scripts" / "svc_token.py"), "mint", str(key), sub,
                        "wotan", str(int(time.time()))], capture_output=True, text=True, check=True, timeout=30)
    return r.stdout.strip()


def events_as(wotan, key, sub):
    code, body = harness.http("GET", f"{wotan.url}/api/v1/topics/{TOPIC}/messages?after_seq=0",
                              headers={"Authorization": "Bearer " + token(key, sub)})
    assert code == 200, body
    return [m for m in body["messages"]]


@needs_crypto
def test_champion_key_publishes_progress_to_an_authed_wotan(wotan_binary, keygen, tmp_path):
    # zhen-agentd.service sets AUTH_KEY_FILE to its own key; the runner it
    # spawns inherits it and reports as zhen-agentd.
    key, trust = make_key(keygen, tmp_path, "zhen-agentd")
    make_key(keygen, tmp_path, "wotan")
    w = authed_wotan(wotan_binary, tmp_path, trust, ("zhen-agentd",)).start()
    try:
        # Without a token Wotan refuses: the auth is really on.
        code, _ = harness.http("POST", f"{w.url}/api/v1/topics/{TOPIC}/subscribe", {"display_name": "x"})
        assert code == 401, code
        p = write_runbook(tmp_path, TWO_STEPS)
        code, out = run(p, {"RUNBOOK_PROGRESS_WOTAN": w.addr, "RUNBOOK_REF": "test/progress-fixture",
                            "AUTH_KEY_FILE": str(key)})
        assert code == 0, out
        assert "[progress]" not in out, out
        msgs = events_as(w, key, "zhen-agentd")
    finally:
        w.stop()
    assert len(msgs) == 5, msgs
    assert {m.get("sender_id") for m in msgs} != {""}


@needs_crypto
def test_no_key_turns_progress_off_not_the_run(wotan_binary, keygen, tmp_path):
    _, trust = make_key(keygen, tmp_path, "zhen-agentd")
    make_key(keygen, tmp_path, "wotan")
    w = authed_wotan(wotan_binary, tmp_path, trust, ("zhen-agentd",)).start()
    try:
        p = write_runbook(tmp_path, TWO_STEPS)
        code, out = run(p, {"RUNBOOK_PROGRESS_WOTAN": w.addr, "RUNBOOK_REF": "test/progress-fixture",
                            "AUTH_KEY_FILE": str(tmp_path / "keys" / "absent.key")})
    finally:
        w.stop()
    assert code == 0, out
    assert "[progress] no service key" in out and "[progress] off" in out, out
    assert (tmp_path / "two").exists()


@needs_crypto
def test_key_of_an_unlisted_identity_is_refused(wotan_binary, keygen, tmp_path):
    # A valid token whose subject Wotan does not allow: 403, progress off.
    key, trust = make_key(keygen, tmp_path, "intruder")
    make_key(keygen, tmp_path, "zhen-agentd")
    make_key(keygen, tmp_path, "wotan")
    w = authed_wotan(wotan_binary, tmp_path, trust, ("zhen-agentd",)).start()
    try:
        p = write_runbook(tmp_path, TWO_STEPS)
        code, out = run(p, {"RUNBOOK_PROGRESS_WOTAN": w.addr, "RUNBOOK_REF": "test/progress-fixture",
                            "AUTH_KEY_FILE": str(key)})
    finally:
        w.stop()
    assert code == 0, out
    assert "[progress] off" in out and "403" in out, out


def test_open_key_file_is_unusable(tmp_path):
    key = tmp_path / "zhen-agentd.key"
    key.write_text("-----BEGIN UNHEADED ED25519 SEED-----\nAAAA\n-----END UNHEADED ED25519 SEED-----\n")
    os.chmod(key, 0o644)
    with pytest.raises(RUNNER.svc_token.KeyFileError, match="0644"):
        RUNNER.svc_token.read_key_file(str(key))


@pytest.mark.parametrize("env,want", [
    ({"AUTH_KEY_FILE": "/etc/unheaded/keys/zhen-agentd.key"}, ("zhen-agentd", "/etc/unheaded/keys/zhen-agentd.key")),
    ({"AUTH_KEY_FILE": "/run/secrets/svc_key_kanban-app"}, ("kanban-app", "/run/secrets/svc_key_kanban-app")),
    ({"AUTH_KEY_FILE": "/k/x.key", "RUNBOOK_PROGRESS_IDENTITY": "zhen-agentd"}, ("zhen-agentd", "/k/x.key")),
    ({"RUNBOOK_PROGRESS_IDENTITY": "sword"}, ("sword", "/etc/unheaded/keys/sword.key")),
])
def test_progress_identity(env, want):
    assert RUNNER.progress_identity(env) == want


def test_progress_identity_defaults_to_the_cli(tmp_path, monkeypatch):
    monkeypatch.setenv("HOME", str(tmp_path))
    name, _key = RUNNER.progress_identity({})
    assert name == "unheaded-cli"
    cli = tmp_path / ".config" / "unheaded" / "cli.key"
    cli.parent.mkdir(parents=True)
    cli.write_text("x")
    assert RUNNER.progress_identity({}) == ("unheaded-cli", str(cli))
