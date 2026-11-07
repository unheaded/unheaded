# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""The nix firewalls open no port for a service that binds loopback: such a rule
guards nothing, and it would expose the service the day someone widens its bind
without deciding to. shield and sophia-gateway bind loopback unless told, and
off loopback they want AUTH_ENABLED=true, which the nix units do not set."""
import re
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
SHIELD = "nixos/modules/services/shield.nix"
SOPHIA_EYE = "nix/containers/sophia-eye.nix"
HOSTS = ["nixos/hosts/host-a/configuration.nix", "nixos/hosts/host-b/configuration.nix"]


def read(path):
    return (REPO / path).read_text(encoding="utf-8")


def code(path):
    """The file without its comments, so a note about a port is not a rule."""
    return "\n".join(line.split("#", 1)[0] for line in read(path).splitlines())


def opened(text):
    """Every token inside an allowedTCPPorts list."""
    return [t for body in re.findall(r"allowedTCPPorts\s*=\s*(?:lib\.\w+\s*)?\[(.*?)\]", text, re.DOTALL)
            for t in body.split()]


def test_opened_reads_a_list_across_lines_and_ignores_what_a_comment_says():
    assert opened("allowedTCPPorts = [ cfg.port ];") == ["cfg.port"]
    assert opened("allowedTCPPorts = [\n 22\n 443\n];") == ["22", "443"]


def test_both_services_still_bind_loopback_unless_told():
    assert re.search(r'gatewayBind = "127\.0\.0\.1"', read("deploy/sophia-eye/sophia-gateway/main.go"))
    assert "Default bind: 127.0.0.1" in read("cmd/shield/main.go")


def test_shield_opens_no_port():
    assert opened(code(SHIELD)) == []


def test_the_hosts_open_no_port_for_shield():
    for host in HOSTS:
        assert "50055" not in opened(code(host)), host


def test_sophia_eye_opens_no_port_for_the_gateway():
    ports = opened(code(SOPHIA_EYE))
    assert ports, "no list found: the rule is not reading the file"
    assert "20105" not in ports
    assert "9090" not in ports


def test_nothing_widens_those_binds_in_nix():
    assert "SOPHIA_GATEWAY_BIND" not in code(SOPHIA_EYE)
    assert "--listen" not in code(SHIELD)
