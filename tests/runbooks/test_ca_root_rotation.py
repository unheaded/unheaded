# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Dry-run test for runbooks/pki/ca-compromise-root-rotation.yaml (R5), in a
sandbox dir: an old root and leaf, a new root and leaf (openssl), the overlap
bundle and the new-only bundle. The runbook's own check steps, pointed at the
sandbox, must give the right verdicts; one runs against a throwaway
`openssl s_server`. No real key or certificate is touched."""
import subprocess
import time

import harness
import pytest

RB = "runbooks/pki/ca-compromise-root-rotation.yaml"
pytestmark = harness.requires("openssl")


def ossl(*args, cwd):
    subprocess.run(["openssl", *args], cwd=cwd, check=True, capture_output=True, timeout=60)


def root_and_leaf(d, tag):
    ossl("req", "-x509", "-new", "-nodes", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:prime256v1",
         "-subj", f"/CN=s2b {tag} root", "-days", "2", "-keyout", f"{tag}-ca.key", "-out", f"{tag}-ca.crt", cwd=d)
    ossl("req", "-new", "-nodes", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:prime256v1",
         "-subj", "/CN=localhost", "-keyout", f"{tag}-leaf.key", "-out", f"{tag}-leaf.csr", cwd=d)
    ext = d / "san.ext"
    ext.write_text("subjectAltName=DNS:localhost,IP:127.0.0.1\n")
    ossl("x509", "-req", "-in", f"{tag}-leaf.csr", "-CA", f"{tag}-ca.crt", "-CAkey", f"{tag}-ca.key", "-CAcreateserial",
         "-days", "2", "-extfile", "san.ext", "-out", f"{tag}-leaf.crt", cwd=d)


@pytest.fixture
def pki(tmp_path):
    root_and_leaf(tmp_path, "old")
    root_and_leaf(tmp_path, "new")
    newpki = tmp_path / "pki-new"
    newpki.mkdir()
    (newpki / "ca.crt").write_text((tmp_path / "new-ca.crt").read_text())
    overlap = tmp_path / "overlap.pem"
    overlap.write_text((tmp_path / "new-ca.crt").read_text() + (tmp_path / "old-ca.crt").read_text())
    return tmp_path, newpki, overlap


def check(rb, name, **env):
    return harness.run_step(rb, name, env={k: str(v) for k, v in env.items()}, check=False).returncode


def test_dry_run_lists_every_step():
    assert harness.dry_run(RB) == [s["name"] for s in harness.steps(harness.load(RB))]


def test_key_handling_and_deploy_steps_are_manual():
    rb = harness.load(RB)
    for s in harness.steps(rb) + list(rb.get("rollback") or []):
        c = s.get("command") or ""
        if any(k in c for k in ("ca.key", "systemctl restart", "mv ", "cp ")):
            assert s["action"] == "manual", s["name"]
    for s in harness.steps(rb):
        if s["action"] == "shell":
            assert ".key" not in s["command"], f"{s['name']} touches a key"


def test_overlap_bundle_trusts_both_then_new_only_refuses_old(pki):
    d, newpki, overlap = pki
    rb = harness.load(RB)
    base = {"NEW_PKI": newpki, "OLD_CA": d / "old-ca.crt"}
    # overlap window: both roots in the bundle, both leaves verify
    assert check(rb, "check-bundle-has-both-roots", **base, BUNDLE=overlap) == 0
    assert check(rb, "check-leaf", **base, BUNDLE=overlap, LEAF=d / "old-leaf.crt") == 0
    assert check(rb, "check-leaf", **base, BUNDLE=overlap, LEAF=d / "new-leaf.crt") == 0
    assert check(rb, "check-old-leaf-refused", **base, BUNDLE=overlap, LEAF=d / "old-leaf.crt") != 0
    # after drop-old-root: the new-only bundle refuses the old leaf
    newonly = newpki / "ca.crt"
    assert check(rb, "check-bundle-has-both-roots", **base, BUNDLE=newonly) != 0
    assert check(rb, "check-leaf", **base, BUNDLE=newonly, LEAF=d / "new-leaf.crt") == 0
    assert check(rb, "check-leaf", **base, BUNDLE=newonly, LEAF=d / "old-leaf.crt") != 0
    assert check(rb, "check-old-leaf-refused", **base, BUNDLE=newonly, LEAF=d / "old-leaf.crt") == 0


@pytest.mark.parametrize("tag, want", [("new", 0), ("old", 1)])
def test_endpoint_check_against_a_throwaway_server(pki, tag, want):
    d, newpki, _ = pki
    rb = harness.load(RB)
    port = harness.free_port()
    srv = subprocess.Popen(["openssl", "s_server", "-accept", str(port), "-cert", f"{tag}-leaf.crt", "-key", f"{tag}-leaf.key",
                            "-www", "-quiet"], cwd=d, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    try:
        time.sleep(0.5)
        rc = check(rb, "check-endpoint", NEW_PKI=newpki, ENDPOINT=f"127.0.0.1:{port}")
        assert (rc == 0) == (want == 0), f"{tag} leaf: check-endpoint exited {rc}"
    finally:
        srv.terminate()
        srv.wait(timeout=10)


def test_checks_need_their_inputs():
    rb = harness.load(RB)
    assert check(rb, "check-leaf") != 0
    assert check(rb, "check-endpoint") != 0
