# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Tests for runbooks/observe/service-health-sweep.yaml, the sweep the
platform's own timer runs (ADR-149). Its targets come from a file, one
"name port scheme" per line; a fake curl answers 200 only for the URLs listed
in FAKE_OK, so what is under test is the sweep's judgment and its exit code.
Nothing touches a host."""
import os

import harness
import pytest

RB = "runbooks/observe/service-health-sweep.yaml"

FAKE_CURL = """#!/bin/sh
# Last argument is the URL. 200 when it is in $FAKE_OK, else 400.
for a in "$@"; do url="$a"; done
for ok in $FAKE_OK; do
  case "$url" in "$ok"*) printf 200; exit 0 ;; esac
done
printf 400
"""


@pytest.fixture
def sweep(tmp_path):
    bindir = tmp_path / "bin"
    bindir.mkdir()
    curl = bindir / "curl"
    curl.write_text(FAKE_CURL)
    curl.chmod(0o755)
    rb = harness.load(RB)

    def run(targets_text, ok, targets_path=None):
        targets = tmp_path / "targets"
        if targets_text is not None:
            targets.write_text(targets_text)
        env = {
            "PATH": f"{bindir}{os.pathsep}{os.environ['PATH']}",
            "FAKE_OK": ok,
            "SWEEP_TARGETS_FILE": str(targets_path or targets),
            "UNHEADED_TLS_CA_CERT": str(tmp_path / "ca.pem"),
        }
        return harness.run_step(rb, "sweep-all-services", env=env, check=False)

    return run


TWO = "wotan 18000 https\ntrace-collector 16671 http\n"


def test_all_healthy(sweep):
    r = sweep(TWO, "https://localhost:18000/health http://localhost:16671/health")
    assert r.returncode == 0, r.stdout + r.stderr
    assert "[OK]   wotan" in r.stdout
    assert "[OK]   trace-collector" in r.stdout
    assert "RESULT: 2/2 healthy" in r.stdout


def test_one_unhealthy_fails_the_step(sweep):
    r = sweep(TWO, "https://localhost:18000/health")
    assert r.returncode == 1
    fails = [line for line in r.stdout.splitlines() if "[FAIL]" in line]
    assert len(fails) == 1 and "trace-collector" in fails[0]


def test_missing_targets_file(sweep, tmp_path):
    r = sweep(None, "", targets_path=tmp_path / "nosuch")
    assert r.returncode == 1
    assert "no targets file" in r.stdout


def test_bad_scheme(sweep):
    r = sweep("x 1 gopher\n", "")
    assert r.returncode == 1
    assert "[FAIL] x: scheme 'gopher' is not http or https" in r.stdout


def test_comments_and_blank_lines_are_skipped(sweep):
    r = sweep("# a comment\n\n" + TWO + "\n   \n# another\n",
              "https://localhost:18000/health http://localhost:16671/health")
    assert r.returncode == 0, r.stdout + r.stderr
    assert "RESULT: 2/2 healthy" in r.stdout


def test_names_no_module():
    text = (harness.REPO / RB).read_text(encoding="utf-8")
    assert "zhen" not in text.lower()
    assert "inference" not in text


def test_dry_run_lists_both_steps():
    assert harness.dry_run(RB) == ["sweep-all-services", "check-system-resources"]


def test_west_targets_file_shape():
    path = harness.REPO / "configs/health-sweep-west.targets"
    lines = [ln.split() for ln in path.read_text(encoding="utf-8").splitlines()
             if ln.strip() and not ln.lstrip().startswith("#")]
    assert lines, "no targets"
    for parts in lines:
        assert len(parts) == 3, parts
        name, port, scheme = parts
        assert scheme in ("http", "https"), parts
        assert 1 <= int(port) <= 65535, parts
        assert "zhen" not in name and "llama" not in name, parts
        assert int(port) not in (8081, 20103, 20111, 20112, 19001, 19002, 19003, 19004, 19005, 19006), parts
