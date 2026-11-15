# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""scripts/monitoring/module-listener-guard.sh reports, from ss, a module
listener on a public or unlisted address (ADR-149). It changes nothing; the
alert ModuleListenerPublic acts. A fake ss stands in for the host."""
import os
import pathlib
import subprocess
import time

import pytest

REPO = pathlib.Path(__file__).resolve().parents[2]
GUARD = REPO / "scripts/monitoring/module-listener-guard.sh"

MANIFEST = """name: zhen
switch: unheaded-zhen.target
ports: [8081, 20103, 20111, 20112]
bind_allow:
  west: ["fd00:dead:beef::1/128"]
"""


@pytest.fixture
def fx(tmp_path):
    src = tmp_path / "src"
    (src / "zhen").mkdir(parents=True)
    (src / "zhen/module.yaml").write_text(MANIFEST)
    tf = tmp_path / "textfile"
    tf.mkdir()
    bindir = tmp_path / "bin"
    bindir.mkdir()
    ss = bindir / "ss"
    ss.write_text('#!/bin/sh\ncat "$STATE_SS"\n')
    ss.chmod(0o755)
    state = tmp_path / "ss.out"
    env = dict(os.environ, PATH=f"{bindir}{os.pathsep}{os.environ['PATH']}", GUARD_HOST="west", STATE_SS=str(state))
    return {"src": src, "tf": tf, "state": state, "env": env}


def run(fx, listens):
    fx["state"].write_text("".join(f"LISTEN 0 4096 {a} *:*\n" for a in listens))
    return subprocess.run(["bash", str(GUARD), str(fx["src"]), str(fx["tf"])], env=fx["env"],
                          capture_output=True, text=True, timeout=30, check=False)


def read(fx):
    out = {}
    for line in (fx["tf"] / "module_listeners.prom").read_text().splitlines():
        if line and not line.startswith("#"):
            k, v = line.rsplit(" ", 1)
            out[k] = float(v)
    return out


TOTAL = 'unheaded_module_listener_public_total{module="zhen"}'


@pytest.mark.parametrize("listens,total,named", [
    (["127.0.0.1:20111", "[fd00:dead:beef::1]:20111"], 0, None),
    (["0.0.0.0:20103"], 1, ("20103", "0.0.0.0")),
    (["*:20111"], 1, ("20111", "*")),
    (["[::]:8081"], 1, ("8081", "::")),
    (["192.168.69.130:20103"], 1, ("20103", "192.168.69.130")),
    (["0.0.0.0:22"], 0, None),
])
def test_guard(fx, listens, total, named):
    r = run(fx, listens)
    assert r.returncode == 0, r.stderr
    m = read(fx)
    assert m[TOTAL] == total
    per = [k for k in m if k.startswith("unheaded_module_listener_public{")]
    if named is None:
        assert per == []
    else:
        port, addr = named
        assert per == [f'unheaded_module_listener_public{{module="zhen",port="{port}",addr="{addr}"}}']
    assert abs(m["unheaded_module_listener_guard_last_run_timestamp_seconds"] - time.time()) <= 5
    assert (fx["tf"] / "module_listeners.prom").stat().st_mode & 0o777 == 0o644
    assert not [p.name for p in fx["tf"].iterdir() if p.name.startswith(".")]


def test_ss_failing_keeps_the_old_textfile(fx):
    """REV-4 (independent review 2026-11-15): when ss fails, the guard fails
    and writes nothing, so the old textfile stays and its last-run timestamp
    ages: ModuleListenerGuardNotRunning fires instead of a fresh public=0."""
    r = run(fx, ["0.0.0.0:20103"])
    assert r.returncode == 0, r.stderr
    prom = fx["tf"] / "module_listeners.prom"
    before = prom.read_text()
    os.utime(prom, (1, 1))
    (pathlib.Path(fx["env"]["PATH"].split(os.pathsep)[0]) / "ss").write_text(
        '#!/bin/sh\necho "ss: netlink: Operation not permitted" >&2\nexit 1\n')
    r = run(fx, [])
    assert r.returncode != 0
    assert "ss" in r.stderr
    assert prom.read_text() == before
    assert prom.stat().st_mtime == 1
    assert not [p.name for p in fx["tf"].iterdir() if p.name.startswith(".")]


def test_missing_argument(fx):
    r = subprocess.run(["bash", str(GUARD), str(fx["src"])], env=fx["env"], capture_output=True,
                       text=True, timeout=30, check=False)
    assert r.returncode == 2
    assert list(fx["tf"].iterdir()) == []
