# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""scripts/monitoring/platform-schedule.sh is the clock for platform checks
(ADR-149): it runs one runbook and leaves three unheaded_schedule_* series in
the textfile directory. A stub runner stands in for run-runbook.py."""
import os
import pathlib
import re
import subprocess
import time

import pytest

REPO = pathlib.Path(__file__).resolve().parents[2]
SCRIPT = REPO / "scripts/monitoring/platform-schedule.sh"


@pytest.fixture
def env(tmp_path):
    stub = tmp_path / "stub-runner"
    stub.write_text("#!/bin/sh\nexit ${STUB_RC:-0}\n")
    stub.chmod(0o755)
    runbook = tmp_path / "runbook.yaml"
    runbook.write_text("")
    out = tmp_path / "textfile"
    out.mkdir()
    e = dict(os.environ, PLATFORM_SCHEDULE_RUNNER=str(stub))
    e.pop("STUB_RC", None)
    return {"env": e, "runbook": runbook, "dir": out}


def run(fx, name="ioc-audit", runbook=None, rc=None):
    e = dict(fx["env"])
    if rc is not None:
        e["STUB_RC"] = str(rc)
    return subprocess.run(
        ["bash", str(SCRIPT), name, str(runbook or fx["runbook"]), str(fx["dir"])],
        env=e, capture_output=True, text=True, timeout=30, check=False,
    )


def samples(path):
    out = {}
    for line in path.read_text().splitlines():
        if not line or line.startswith("#"):
            continue
        key, value = line.rsplit(" ", 1)
        out[key] = float(value)
    return out


RUN = 'unheaded_schedule_last_run_timestamp_seconds{schedule="ioc-audit"}'
CODE = 'unheaded_schedule_last_exit_code{schedule="ioc-audit"}'
OK = 'unheaded_schedule_last_success_timestamp_seconds{schedule="ioc-audit"}'


def no_temp_left(fx):
    assert not [p.name for p in fx["dir"].iterdir() if p.name.startswith(".schedule_")]


def test_success(env):
    r = run(env)
    assert r.returncode == 0, r.stderr
    f = env["dir"] / "schedule_ioc-audit.prom"
    s = samples(f)
    assert set(s) == {RUN, CODE, OK}
    assert s[CODE] == 0
    assert s[RUN] == s[OK]
    assert abs(s[RUN] - time.time()) <= 5
    assert (f.stat().st_mode & 0o777) == 0o644
    no_temp_left(env)


def test_later_failure_keeps_the_old_success(env):
    assert run(env).returncode == 0
    f = env["dir"] / "schedule_ioc-audit.prom"
    f.write_text(re.sub(r"(last_success_timestamp_seconds\{[^}]*\}) \S+", r"\1 1000", f.read_text()))
    r = run(env, rc=1)
    assert r.returncode == 1
    s = samples(f)
    assert s[CODE] == 1
    assert s[OK] == 1000
    assert abs(s[RUN] - time.time()) <= 5
    no_temp_left(env)


def test_first_run_that_fails_starts_the_clock(env):
    r = run(env, rc=1)
    assert r.returncode == 1
    s = samples(env["dir"] / "schedule_ioc-audit.prom")
    assert s[OK] == s[RUN]
    assert s[CODE] == 1
    no_temp_left(env)


@pytest.mark.parametrize("name", ["a b", "x" * 65, "", "A", "a/b"])
def test_bad_name(env, name):
    r = run(env, name=name)
    assert r.returncode == 2
    assert list(env["dir"].iterdir()) == []


def test_missing_runbook(env, tmp_path):
    r = run(env, runbook=tmp_path / "nosuch.yaml")
    assert r.returncode == 2
    assert list(env["dir"].iterdir()) == []


def test_usage(env):
    r = subprocess.run(["bash", str(SCRIPT), "ioc-audit"], env=env["env"],
                       capture_output=True, text=True, timeout=30, check=False)
    assert r.returncode == 2
    assert list(env["dir"].iterdir()) == []
