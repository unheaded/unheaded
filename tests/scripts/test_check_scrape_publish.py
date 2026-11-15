# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""check-scrape-publish.sh: a scrape target at host.docker.internal needs a
listener on docker0's gateway. A compose publish is one; a host unit that binds
the address itself is the other, and it counts only when a tracked unit file
says so. These hold both halves, and that a near miss does not count."""
import os
import subprocess
from pathlib import Path

import pytest

REPO = Path(__file__).resolve().parents[2]
GATE = REPO / "scripts/check-scrape-publish.sh"

SCRAPE = """scrape_configs:
  - job_name: "wotan"
    static_configs:
      - targets: ["host.docker.internal:18000"]
  - job_name: "wiki-server"
    static_configs:
      - targets: ["host.docker.internal:20002"]
"""
COMPOSE = """services:
  wotan:
    ports:
      - "127.0.0.1:18000:18000"
      - "${UNHEADED_GATEWAY_BIND:-172.17.0.1}:18000:18000"
"""


def run(tmp_path, unit_lines, compose=COMPOSE):
    (tmp_path / "scrape.yml").write_text(SCRAPE, encoding="utf-8")
    (tmp_path / "docker-compose.yml").write_text(compose, encoding="utf-8")
    dropin = tmp_path / "systemd" / "unheaded-wiki-server.service.d"
    dropin.mkdir(parents=True)
    (dropin / "west.conf").write_text("[Unit]\nAfter=docker.service\n" + unit_lines, encoding="utf-8")
    env = dict(os.environ, SCRAPE_PUBLISH_COMPOSE=str(tmp_path / "docker-compose.yml"),
               SCRAPE_PUBLISH_UNITS=str(tmp_path / "systemd"))
    return subprocess.run([str(GATE), str(tmp_path / "scrape.yml")], env=env, capture_output=True, text=True, check=False)


def test_a_host_unit_with_no_listener_on_the_gateway_fails(tmp_path):
    got = run(tmp_path, "")
    assert got.returncode == 1, got.stdout
    assert "job wiki-server" in got.stdout
    assert "job wotan" not in got.stdout


@pytest.mark.parametrize("line", [
    "X-Unheaded-Listen=172.17.0.1:20002\n",
    "[Service]\nExecStart=/usr/local/bin/wiki-server --listen 127.0.0.1:20002 --listen 172.17.0.1:20002 --wiki-dir /w\n",
    "[Service]\nExecStart=/usr/local/bin/wiki-server --listen=172.17.0.1:20002\n",
])
def test_a_listener_a_unit_file_names_on_the_gateway_passes(tmp_path, line):
    got = run(tmp_path, line)
    assert got.returncode == 0, got.stdout
    assert "2 scrape targets" in got.stdout
    assert "1 of them a host unit" in got.stdout


@pytest.mark.parametrize("line", [
    "X-Unheaded-Listen=127.0.0.1:20002\n",            # loopback is refused at the gateway
    "X-Unheaded-Listen=172.17.0.1:20003\n",           # another port
    "X-Unheaded-Listen=172.17.0.1:200022\n",          # a longer port
    "# X-Unheaded-Listen=172.17.0.1:20002\n",         # a comment is not a statement
    "[Service]\nExecStart=/usr/local/bin/wiki-server --listen 127.0.0.1:20002\n",
    "[Service]\nEnvironment=NOTE=--listen 172.17.0.1:20002\n",
])
def test_a_near_miss_does_not_count(tmp_path, line):
    got = run(tmp_path, line)
    assert got.returncode == 1, got.stdout
    assert "job wiki-server" in got.stdout


def test_a_compose_publish_on_loopback_only_still_fails(tmp_path):
    compose = COMPOSE.replace('      - "${UNHEADED_GATEWAY_BIND:-172.17.0.1}:18000:18000"\n', "")
    got = run(tmp_path, "X-Unheaded-Listen=172.17.0.1:20002\n", compose)
    assert got.returncode == 1, got.stdout
    assert "job wotan" in got.stdout


def test_the_repo_as_written_passes():
    got = subprocess.run([str(GATE)], capture_output=True, text=True, check=False)
    assert got.returncode == 0, got.stdout + got.stderr
