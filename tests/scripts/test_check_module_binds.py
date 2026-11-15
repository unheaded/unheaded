# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""scripts/check-module-binds.sh refuses, in the repository, a module unit that
would listen on a wildcard, a global address or an unlisted private one
(ADR-149, D5 (a)). Run against a fake repository in tmp_path."""
import pathlib
import subprocess

import pytest

REPO = pathlib.Path(__file__).resolve().parents[2]
GATE = REPO / "scripts/check-module-binds.sh"

MANIFEST = """name: x
switch: x.target
units:
  - {unit: x.service, memory_max: 1M}
ports: [1]
bind_allow:
  west: ["192.168.1.0/24"]
"""


def repo(tmp_path, unit_body, other=None):
    (tmp_path / "modules/x").mkdir(parents=True)
    (tmp_path / "modules/x/module.yaml").write_text(MANIFEST)
    sd = tmp_path / "deploy/systemd"
    sd.mkdir(parents=True)
    (sd / "x.service").write_text("[Service]\n" + unit_body + "\n")
    if other is not None:
        (sd / "y.service").write_text("[Service]\n" + other + "\n")
    return tmp_path


def gate(root):
    return subprocess.run(["bash", str(GATE), "--root", str(root)], capture_output=True,
                          text=True, timeout=60, check=False)


@pytest.mark.parametrize("body", [
    "ExecStart=/bin/x --listen 0.0.0.0:1",
    "ExecStart=/bin/x -host 0.0.0.0",
    "ExecStart=/bin/x --host=::",
    "ExecStart=/bin/x --listen 8.8.8.8:1",
    "ExecStart=/bin/x --listen 10.9.9.9:1",
    "Environment=X_ADDR=0.0.0.0:1",
    "ExecStart=/bin/x \\\n    -port 1 \\\n    -host 0.0.0.0",
])
def test_public_bind_fails(tmp_path, body):
    r = gate(repo(tmp_path, body))
    assert r.returncode == 1, r.stdout + r.stderr
    fails = [ln for ln in r.stdout.splitlines() if ln.startswith("[FAIL]")]
    assert fails and "x.service" in fails[0]


@pytest.mark.parametrize("body", [
    "ExecStart=/bin/x -host 127.0.0.1",
    "ExecStart=/bin/x --listen [::1]:1",
    "ExecStart=/bin/x --listen 192.168.1.5:1",
    "ExecStart=/bin/x",
])
def test_private_or_loopback_passes(tmp_path, body):
    r = gate(repo(tmp_path, body))
    assert r.returncode == 0, r.stdout + r.stderr
    assert "[PASS] module binds:" in r.stdout


def test_units_no_module_names_are_not_read(tmp_path):
    r = gate(repo(tmp_path, "ExecStart=/bin/x -host 127.0.0.1", other="ExecStart=/bin/y -host 0.0.0.0"))
    assert r.returncode == 0, r.stdout


def test_the_repository_passes():
    r = gate(REPO)
    assert r.returncode == 0, r.stdout + r.stderr
    assert "[PASS] module binds: 4 units" in r.stdout


def test_usage():
    r = subprocess.run(["bash", str(GATE), "--root"], capture_output=True, text=True, timeout=30, check=False)
    assert r.returncode == 2
