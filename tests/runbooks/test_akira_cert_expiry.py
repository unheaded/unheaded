# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""runbooks/pki/akira-cert-expiry-check.yaml (S-IRON-GATE Steps 291-293).

Akira runs this runbook as an exec health check (exit 0 healthy, else
failing). Fixture certificates are made with openssl in a temp dir laid out
like /etc/unheaded/tls (S-IRON-GATE Phase 8); UNHEADED_TLS_DIR points the
runbook at it. A 1-day cert's remaining lifetime is set with explicit
validity windows (`openssl x509 -not_before/-not_after`, OpenSSL 3.4+; the
tests skip on an older openssl), so no faketime is needed. No real key or
cert is touched, nothing runs as root."""
import datetime as dt
import os
import stat
import subprocess

import harness
import pytest

RB = "runbooks/pki/akira-cert-expiry-check.yaml"
pytestmark = harness.requires("openssl")


def _supports_not_before():
    r = subprocess.run(["openssl", "x509", "-help"], capture_output=True, text=True, timeout=30, check=False)
    return "-not_before" in (r.stdout + r.stderr)


NOT_BEFORE = _supports_not_before()


def _stamp(t):
    return t.strftime("%Y%m%d%H%M%SZ")


def make_cert(path, *, pct_left, hours=24, cn="svc"):
    """A self-signed cert of `hours` lifetime with `pct_left` % of it left."""
    now = dt.datetime.now(dt.UTC).replace(microsecond=0)
    total = dt.timedelta(hours=hours)
    not_after = now + total * pct_left / 100
    not_before = not_after - total
    key = str(path) + ".tmpkey"
    subprocess.run(["openssl", "req", "-new", "-nodes", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:prime256v1",
                    "-subj", f"/CN={cn}", "-keyout", key, "-out", str(path) + ".csr"],
                   check=True, capture_output=True, timeout=60)
    if NOT_BEFORE:
        subprocess.run(["openssl", "x509", "-req", "-in", str(path) + ".csr", "-key", key,
                        "-not_before", _stamp(not_before), "-not_after", _stamp(not_after), "-out", str(path)],
                       check=True, capture_output=True, timeout=60)
    else:
        pytest.skip("openssl lacks -not_before/-not_after (needs 3.4+)")
    os.remove(key)
    os.remove(str(path) + ".csr")


def tls_layout(root, certs, ca_pct=90):
    """root/<id>/<id>.pem + root/<id>/<id>-key.pem (junk) + root/<id>/ca.pem."""
    ca = root / "ca.pem"
    make_cert(ca, pct_left=ca_pct, hours=72, cn="ca")
    for ident, pct in certs.items():
        d = root / ident
        d.mkdir()
        make_cert(d / f"{ident}.pem", pct_left=pct, cn=ident)
        # A key file the check must never open: unreadable, and not a cert.
        k = d / f"{ident}-key.pem"
        k.write_text("not a certificate\n")
        k.chmod(0)
        (d / "ca.pem").write_bytes(ca.read_bytes())
    return root


def check(rb, name, root, **extra):
    env = {"UNHEADED_TLS_DIR": str(root), "UNHEADED_PKI_DIR": str(root / "no-such-pki"), **extra}
    return harness.run_step(rb, name, env=env, check=False)


@pytest.fixture
def rb():
    return harness.load(RB)


def test_dry_run_lists_every_step(rb):
    assert harness.dry_run(RB) == [s["name"] for s in harness.steps(rb)]


def test_read_only_no_sudo_no_rotation(rb):
    """An Akira exec check runs unprivileged and must not change anything."""
    assert rb["metadata"]["requires_sudo"] is False
    for s in harness.steps(rb):
        for c in harness.commands(s.get("command")):
            assert not c.startswith(("sudo", "run-runbook", "/opt/unheaded/bin/run-runbook", "mv ", "cp ", "rm ")), c
        assert "sudo " not in (s.get("command") or "")


def test_points_at_phase8_layout_with_fallback(rb):
    text = harness.step(rb, "check-all-certs")["command"]
    assert "/etc/unheaded/tls" in text and "/var/lib/unheaded/pki" in text


def test_healthy_certs_pass(rb, tmp_path):
    tls_layout(tmp_path, {"timeguru": 90, "wotan": 80})
    r = check(rb, "check-all-certs", tmp_path)
    assert r.returncode == 0, r.stdout + r.stderr
    assert "OK: timeguru" in r.stdout and "OK: wotan" in r.stdout
    assert "-key" not in r.stdout and "ca (" not in r.stdout
    r = check(rb, "check-ca-cert", tmp_path)
    assert r.returncode == 0, r.stdout + r.stderr
    assert "OK: CA cert" in r.stdout


def test_cert_under_a_third_left_fails(rb, tmp_path):
    """Step 291: a 1-day cert with < 33 % left makes the check exit non-zero."""
    tls_layout(tmp_path, {"timeguru": 20, "wotan": 90})
    r = check(rb, "check-all-certs", tmp_path)
    assert r.returncode == 1, r.stdout + r.stderr
    assert "ERROR: timeguru (" in r.stdout and "OK: wotan" in r.stdout


@pytest.mark.parametrize("pct,code,word", [(60, 0, "ROTATE"), (40, 1, "WARN"), (20, 1, "ERROR"), (5, 2, "CRITICAL")])
def test_percentage_bands(rb, tmp_path, pct, code, word):
    tls_layout(tmp_path, {"timeguru": pct})
    r = check(rb, "check-all-certs", tmp_path)
    assert r.returncode == code, r.stdout + r.stderr
    assert f"{word}: timeguru" in r.stdout


def test_expired_cert_is_exit_2(rb, tmp_path):
    tls_layout(tmp_path, {"timeguru": -10})
    r = check(rb, "check-all-certs", tmp_path)
    assert r.returncode == 2 and "EXPIRED: timeguru" in r.stdout


def test_ca_bands(rb, tmp_path):
    tls_layout(tmp_path, {"timeguru": 90}, ca_pct=20)
    r = check(rb, "check-ca-cert", tmp_path)
    assert r.returncode == 1 and "ERROR: CA cert" in r.stdout


def test_flat_container_mount(rb, tmp_path):
    """One container's own mount: <id>.pem, <id>-key.pem and ca.pem side by side."""
    make_cert(tmp_path / "ca.pem", pct_left=90, hours=72, cn="ca")
    make_cert(tmp_path / "akira.pem", pct_left=15, cn="akira")
    (tmp_path / "akira-key.pem").write_text("junk\n")
    r = check(rb, "check-all-certs", tmp_path)
    assert r.returncode == 1 and "ERROR: akira" in r.stdout
    assert check(rb, "check-ca-cert", tmp_path).returncode == 0


def test_no_certs_fails(rb, tmp_path):
    r = check(rb, "check-all-certs", tmp_path)
    assert r.returncode == 1 and "NO CERTS FOUND" in r.stdout
    r = check(rb, "check-ca-cert", tmp_path)
    assert r.returncode == 2 and "NO CA CERT FOUND" in r.stdout


@pytest.mark.skipif(os.geteuid() == 0, reason="root reads every directory")
def test_unlistable_dir_votes_failing(rb, tmp_path):
    """A directory Akira's uid cannot list is not a silent pass."""
    tls_layout(tmp_path, {"timeguru": 90, "wotan": 90})
    (tmp_path / "wotan").chmod(0)
    try:
        r = check(rb, "check-all-certs", tmp_path)
    finally:
        (tmp_path / "wotan").chmod(stat.S_IRWXU)
    assert r.returncode == 1, r.stdout + r.stderr
    assert "UNREADABLE" in r.stdout and "OK: timeguru" in r.stdout


def test_falls_back_to_old_pki_layout(rb, tmp_path):
    pki = tmp_path / "pki"
    for sub in ("services", "hosts", "ca"):
        (pki / sub).mkdir(parents=True)
    make_cert(pki / "services" / "wotan.crt", pct_left=90, cn="wotan")
    make_cert(pki / "hosts" / "west.crt", pct_left=20, cn="west")
    make_cert(pki / "ca" / "ca.crt", pct_left=90, hours=72, cn="ca")
    env = {"UNHEADED_TLS_DIR": str(tmp_path / "absent"), "UNHEADED_PKI_DIR": str(pki)}
    r = harness.run_step(rb, "check-all-certs", env=env, check=False)
    assert r.returncode == 1 and "OK: wotan" in r.stdout and "ERROR: west" in r.stdout
    r = harness.run_step(rb, "check-ca-cert", env=env, check=False)
    assert r.returncode == 0 and "OK: CA cert" in r.stdout


def test_runner_exit_code_is_the_vote(tmp_path):
    """What Akira runs: the runner, live, against fixtures. Exit 0 healthy, 1 failing."""
    runner = harness.REPO / "scripts" / "run-runbook.py"

    def run(root):
        env = {**os.environ, "UNHEADED_TLS_DIR": str(root), "UNHEADED_PKI_DIR": str(root / "none")}
        env.pop("RUNBOOK_PROGRESS_WOTAN", None)
        return subprocess.run(["python3", "-B", str(runner), str(harness.REPO / RB)], capture_output=True,
                              text=True, timeout=60, env=env, cwd="/", check=False)

    good = tmp_path / "good"
    good.mkdir()
    tls_layout(good, {"timeguru": 90})
    r = run(good)
    assert r.returncode == 0, r.stdout + r.stderr
    bad = tmp_path / "bad"
    bad.mkdir()
    tls_layout(bad, {"timeguru": 20})
    r = run(bad)
    assert r.returncode == 1, r.stdout + r.stderr
    assert "ERROR: timeguru" in r.stdout
