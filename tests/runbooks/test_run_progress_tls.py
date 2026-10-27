# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""run-runbook.py progress to an https:// Wotan (S-IRON-GATE Phase 8): TLS
1.3, the server checked against UNHEADED_TLS_CA_CERT. The Wotan here is a
stand-in speaking its two topic calls over TLS with a certificate made at
run time in tmp_path."""
import http.server
import json
import shutil
import ssl
import subprocess
import threading
import typing

import harness
import pytest
from test_run_progress import TWO_STEPS, run, write_runbook

RUNNER = harness._RUNNER

pytestmark = pytest.mark.skipif(shutil.which("openssl") is None, reason="no openssl to make a test certificate")


class _Wotan(http.server.BaseHTTPRequestHandler):
    seen: typing.ClassVar[list] = []

    def do_POST(self):
        body = self.rfile.read(int(self.headers.get("Content-Length", 0)))
        _Wotan.seen.append((self.path, json.loads(body or b"{}")))
        reply = {"subscriber": {"subscriber_id": "6f1c2b9e-0000-4000-8000-000000000002", "status": "approved"}} \
            if self.path.endswith("/subscribe") else {}
        data = json.dumps(reply).encode()
        self.send_response(201)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def log_message(self, *_a):
        pass


@pytest.fixture
def tls_wotan(tmp_path):
    cert, key = tmp_path / "wotan.crt", tmp_path / "wotan.key"
    subprocess.run(["openssl", "req", "-x509", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:prime256v1",
                    "-nodes", "-keyout", str(key), "-out", str(cert), "-days", "1", "-subj", "/CN=127.0.0.1",
                    "-addext", "subjectAltName=IP:127.0.0.1"], check=True, capture_output=True, timeout=30)
    srv = http.server.ThreadingHTTPServer(("127.0.0.1", 0), _Wotan)
    ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    ctx.minimum_version = ssl.TLSVersion.TLSv1_3
    ctx.load_cert_chain(str(cert), str(key))
    srv.socket = ctx.wrap_socket(srv.socket, server_side=True)
    _Wotan.seen = []
    t = threading.Thread(target=srv.serve_forever, daemon=True)
    t.start()
    yield f"https://127.0.0.1:{srv.server_address[1]}", cert
    srv.shutdown()
    srv.server_close()


def test_https_wotan_with_its_ca(tls_wotan, tmp_path):
    url, cert = tls_wotan
    p = write_runbook(tmp_path, TWO_STEPS)
    code, out = run(p, {"RUNBOOK_PROGRESS_WOTAN": url, "RUNBOOK_REF": "test/progress-fixture",
                        "UNHEADED_TLS_CA_CERT": str(cert), "AUTH_KEY_FILE": str(tmp_path / "absent.key")})
    assert code == 0, out
    assert "[progress] off" not in out, out
    published = [b for path, b in _Wotan.seen if path.endswith("/publish")]
    assert len(published) == 5, _Wotan.seen
    assert json.loads(published[-1]["payload"])["state"] == "done"


def test_https_wotan_without_its_ca_is_refused(tls_wotan, tmp_path):
    url, _ = tls_wotan
    p = write_runbook(tmp_path, TWO_STEPS)
    code, out = run(p, {"RUNBOOK_PROGRESS_WOTAN": url, "RUNBOOK_REF": "test/progress-fixture",
                        "UNHEADED_TLS_CA_CERT": "", "AUTH_KEY_FILE": str(tmp_path / "absent.key")})
    assert code == 0, out
    assert "[progress] off" in out and "CERTIFICATE_VERIFY_FAILED" in out, out
    assert _Wotan.seen == []


# A service token travels only over TLS, or in clear to loopback (S-IRON-GATE
# Phase 15, matching pkg/wotan-client's tokenAuth.authorize): over plain http
# to another host the runner sends no token at all.
@pytest.mark.parametrize("addr,carries", [
    ("http://192.0.2.10:18000", False),
    ("192.0.2.10:18000", False),
    ("http://wotan:18000", False),
    ("http://[2001:db8::1]:18000", False),
    ("http://127.0.0.1:18000", True),
    ("http://localhost:18000", True),
    ("http://[::1]:18000", True),
    ("https://wotan:18000", True),
])
def test_token_only_over_tls_or_loopback(monkeypatch, capsys, addr, carries):
    sentinel = object()
    monkeypatch.setattr(RUNNER, "load_progress_minter", lambda: sentinel)
    monkeypatch.setenv("RUNBOOK_PROGRESS_WOTAN", addr)
    p = RUNNER.Progress("pki/akira-cert-expiry-check", 2, False)
    assert p.base is not None
    assert (p.minter is sentinel) is carries
    if not carries:
        assert "no token over plain http" in capsys.readouterr().err
