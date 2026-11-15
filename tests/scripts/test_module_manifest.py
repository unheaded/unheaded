# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""modules/zhen/module.yaml restates what the unit files and the port registry
already say (ADR-149). This test fails when the two drift apart; the fix is
always to the manifest, never to a unit file."""
import pathlib
import re

import yaml

REPO = pathlib.Path(__file__).resolve().parents[2]
MANIFEST = REPO / "modules/zhen/module.yaml"
UNITS = REPO / "deploy/systemd"


def manifest():
    return yaml.safe_load(MANIFEST.read_text(encoding="utf-8"))


def directive(text, name):
    """The value of the last NAME= line of a unit file, or None."""
    found = None
    for line in text.splitlines():
        if line.startswith(name + "="):
            found = line.split("=", 1)[1].strip()
    return found


def test_units_exist():
    for u in manifest()["units"]:
        assert (UNITS / u["unit"]).is_file(), u["unit"]


def test_limits_match_the_units():
    for u in manifest()["units"]:
        text = (UNITS / u["unit"]).read_text(encoding="utf-8")
        assert directive(text, "MemoryMax") == u["memory_max"], u["unit"]
        if "tasks_max" in u:
            assert directive(text, "TasksMax") == str(u["tasks_max"]), u["unit"]


def test_ports_are_registered():
    ports_go = (REPO / "pkg/ports/ports.go").read_text(encoding="utf-8")
    for port in manifest()["ports"]:
        if port == 8081:
            assert "tcp:8081" in (UNITS / "unheaded-llama.service").read_text(encoding="utf-8")
            continue
        assert re.search(r"=\s*%d\b" % port, ports_go), port


def test_name_and_switch():
    m = manifest()
    assert m["name"] == "zhen"
    assert m["switch"] == "unheaded-zhen.target"
    limit = m["database"]["connection_limit"]
    assert isinstance(limit, int) and 1 <= limit <= 20


def test_units_belong_to_the_switch():
    names = [u["unit"] for u in manifest()["units"]] + ["unheaded-zhen-module.service"]
    for name in names:
        text = (UNITS / name).read_text(encoding="utf-8")
        assert "PartOf=unheaded-zhen.target" in text.splitlines(), name
        assert "WantedBy=unheaded-zhen.target" in text.splitlines(), name
        assert "WantedBy=multi-user.target" not in text, name


def test_bind_allow_is_private_and_matches_the_dropins():
    import ipaddress

    allow = manifest().get("bind_allow") or {}
    for host, prefixes in allow.items():
        for p in prefixes:
            net = ipaddress.ip_network(p, strict=False)
            assert net.is_private and not net.is_link_local, (host, p)
            assert net.prefixlen > 0, (host, p)
    hosts = REPO / "modules/zhen/hosts"
    for hostdir in sorted(d for d in hosts.iterdir() if d.is_dir()) if hosts.is_dir() else []:
        want = ",".join(allow.get(hostdir.name, []))
        for conf in sorted(hostdir.glob("*.service.d/bind-allow.conf")):
            vals = [ln.split("=", 2)[2] for ln in conf.read_text(encoding="utf-8").splitlines()
                    if ln.startswith("Environment=UNHEADED_MODULE_BIND_ALLOW=")]
            assert vals == [want], (conf, vals, want)


USER_UNITS = UNITS / "user"
MODULE_MARKER = "/run/unheaded/modules/zhen"


def test_user_units_running_zhen_skip_while_the_switch_is_off():
    """REV-1 (independent review 2026-11-15): a user unit sits outside
    unheaded-zhen.target, so the switch cannot stop it. Every user service
    that runs a Zhen binary carries an ExecCondition on the module's runtime
    directory, which exists only while the switch is on."""
    found = 0
    for svc in sorted(USER_UNITS.glob("*.service")):
        text = svc.read_text(encoding="utf-8")
        starts = [ln for ln in text.splitlines() if ln.startswith("ExecStart=")]
        if not any(re.search(r"\bzhen-[a-z-]+\b", ln) for ln in starts):
            continue
        found += 1
        conds = [ln for ln in text.splitlines() if ln.startswith("ExecCondition=")]
        assert any(MODULE_MARKER in ln and "test -d" in ln and "test ! -L" in ln for ln in conds), svc.name
    assert found >= 1  # zhen-agentd-overnight.service


def test_timers_belong_to_the_switch():
    for name in manifest().get("timers") or []:
        timer = UNITS / name
        assert timer.is_file(), name
        lines = timer.read_text(encoding="utf-8").splitlines()
        assert "PartOf=unheaded-zhen.target" in lines and "WantedBy=unheaded-zhen.target" in lines, name
        service = UNITS / name.replace(".timer", ".service")
        assert service.is_file(), service
        assert "PartOf=unheaded-zhen.target" in service.read_text(encoding="utf-8").splitlines(), service
