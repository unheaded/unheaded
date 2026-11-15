# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""scripts/test-zhen-switch.sh reads, from outside, whether the Zhen module is
on or off everywhere it should show (ADR-149). Fakes for systemctl, curl, ss
and docker stand in for the host; nothing here touches one."""
import hashlib
import json
import os
import pathlib
import subprocess
import time

import pytest

REPO = pathlib.Path(__file__).resolve().parents[2]
SCRIPT = REPO / "scripts/test-zhen-switch.sh"

PROM = "http://127.0.0.1:9090"
VM = "http://127.0.0.1:8428"
DASH = "https://127.0.0.1:20000"

FAKES = {
    "systemctl": """#!/bin/sh
# systemctl is-active|is-enabled UNIT: answers from $STATE/<verb>.<unit>
verb="$1"; unit="$2"
f="$STATE/$verb.$unit"
if [ -f "$f" ]; then v=$(cat "$f"); else v=inactive; [ "$verb" = is-enabled ] && v=disabled; fi
echo "$v"
case "$v" in active|enabled) exit 0 ;; *) exit 3 ;; esac
""",
    "curl": """#!/bin/sh
# Prints $URLS/<sha1 of the last argument>; nothing (exit 7) when absent.
for a in "$@"; do last="$a"; done
h=$(printf '%s' "$last" | sha1sum | cut -c1-40)
[ -f "$URLS/$h" ] || exit 7
cat "$URLS/$h"
""",
    "ss": """#!/bin/sh
cat "$STATE/ss" 2>/dev/null
""",
    "docker": """#!/bin/sh
cat "$STATE/sessions"
""",
}


def sha1(s):
    return hashlib.sha1(s.encode()).hexdigest()


def vm_query_url(q):
    from urllib.parse import quote
    return f"{VM}/api/v1/query?query={quote(q, safe='')}"


class Host:
    def __init__(self, tmp):
        self.tmp = tmp
        self.bin = tmp / "bin"
        self.state = tmp / "state"
        self.urls = tmp / "urls"
        self.run = tmp / "run"
        for d in (self.bin, self.state, self.urls, self.run):
            d.mkdir()
        for name, text in FAKES.items():
            p = self.bin / name
            p.write_text(text)
            p.chmod(0o755)

    def url(self, url, body):
        (self.urls / sha1(url)).write_text(body)

    def unit(self, unit, active=None, enabled=None):
        if active is not None:
            (self.state / f"is-active.{unit}").write_text(active)
        if enabled is not None:
            (self.state / f"is-enabled.{unit}").write_text(enabled)

    def set(self, on, rules=True):
        self.unit("unheaded-zhen.target", "active" if on else "inactive")
        for u in ("zhen-agentd.service", "unheaded-zhen-ui-front.service"):
            self.unit(u, "active" if on else "inactive")
        for u in ("unheaded-zhen-app.service", "unheaded-llama.service"):
            self.unit(u, "inactive", "disabled")
        z = self.run / "zhen"
        if on:
            z.mkdir(exist_ok=True)
        elif z.exists():
            z.rmdir()
        groups = [{"name": "kingdom", "rules": []}]
        if on and rules:
            groups.append({"name": "zhen", "rules": [{}, {}]})
        self.url(f"{PROM}/api/v1/rules", json.dumps({"status": "success", "data": {"groups": groups}}))
        tgt = [{"labels": {"job": "wotan"}, "health": "up"}]
        if on:
            tgt.append({"labels": {"job": "zhen-agentd"}, "health": "up"})
        self.url(f"{PROM}/api/v1/targets", json.dumps({"status": "success", "data": {"activeTargets": tgt}}))
        self.url(f"{VM}/api/v1/targets", json.dumps({"status": "success", "data": {"activeTargets": tgt}}))
        self.url(f"{DASH}/metrics", 'unheaded_dashboard_health_targets{module=""} 9\n'
                 + ('unheaded_dashboard_health_targets{module="zhen"} 2\n' if on else
                    'unheaded_dashboard_health_targets{module="zhen"} 0\n'))
        self.url(f"{DASH}/auth/nav.js", 'const apps = [{"id":"dashboard"}' + (',{"id":"zhenai"}' if on else "") + "];")
        listens = ["127.0.0.1:20000", "0.0.0.0:22"]
        if on:
            listens += ["127.0.0.1:20103", "127.0.0.1:20111", "127.0.0.1:20112", "[fd00:dead:beef::1]:20111"]
        (self.state / "ss").write_text("".join(f"LISTEN 0 4096 {a} 0.0.0.0:*\n" for a in listens))
        (self.state / "sessions").write_text("3\n" if on else "0\n")
        self.url(vm_query_url('unheaded_module_enabled{module="zhen"}'),
                 json.dumps({"status": "success", "data": {"result": [
                     {"metric": {"module": "zhen"}, "value": [0, "1" if on else "0"]}]}}))

    def check(self, *args, timeout=60):
        env = dict(os.environ, PATH=f"{self.bin}{os.pathsep}{os.environ['PATH']}",
                   STATE=str(self.state), URLS=str(self.urls), MODULES_RUN=str(self.run))
        return subprocess.run(["bash", str(SCRIPT), *args], env=env, capture_output=True,
                              text=True, timeout=timeout, check=False)


@pytest.fixture
def host(tmp_path):
    return Host(tmp_path)


def lines(out, word):
    return [ln for ln in out.splitlines() if ln.startswith(word + " ")]


def test_on(host):
    host.set(on=True)
    r = host.check("--expect", "on")
    assert r.returncode == 0, r.stdout + r.stderr
    assert len(lines(r.stdout, "PASS")) == 10, r.stdout
    assert len(lines(r.stdout, "SKIP")) == 1 and "C10" in lines(r.stdout, "SKIP")[0]
    assert r.stdout.strip().splitlines()[-1].startswith("RESULT on: pass, ELAPSED=")


def test_on_without_the_rule_group(host):
    host.set(on=True, rules=False)
    r = host.check("--expect", "on")
    assert r.returncode == 1
    fails = lines(r.stdout, "FAIL")
    assert len(fails) == 1 and "C4" in fails[0], r.stdout


def test_off(host):
    host.set(on=False)
    r = host.check("--expect", "off")
    assert r.returncode == 0, r.stdout + r.stderr
    assert len(lines(r.stdout, "PASS")) == 11, r.stdout
    assert r.stdout.strip().splitlines()[-1].startswith("RESULT off: pass")


def test_off_with_the_fragments_still_there_waits_then_fails(host):
    host.set(on=False)
    (host.run / "zhen").mkdir()
    t0 = time.monotonic()
    r = host.check("--expect", "off", "--wait", "2")
    took = time.monotonic() - t0
    assert r.returncode == 1
    assert 2 <= took <= 6, took
    assert any("C3" in ln for ln in lines(r.stdout, "FAIL")), r.stdout


def test_bad_expect(host):
    assert host.check("--expect", "maybe").returncode == 2
    assert host.check().returncode == 2
    assert host.check("--expect", "on", "--wait", "x").returncode == 2


def test_no_well(host):
    host.set(on=False)
    r = host.check("--expect", "off", "--no-well")
    assert r.returncode == 0, r.stdout
    assert any("C10" in ln for ln in lines(r.stdout, "SKIP")), r.stdout


def test_off_with_a_session_left_fails(host):
    host.set(on=False)
    (host.state / "sessions").write_text("2\n")
    r = host.check("--expect", "off")
    assert r.returncode == 1
    assert any("C10" in ln for ln in lines(r.stdout, "FAIL")), r.stdout


def test_optional_units_checked_only_when_enabled(host):
    host.set(on=True)
    host.unit("unheaded-llama.service", "inactive", "enabled")
    r = host.check("--expect", "on")
    assert r.returncode == 1
    assert any("C2" in ln and "unheaded-llama" in ln for ln in lines(r.stdout, "FAIL")), r.stdout
