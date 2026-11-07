# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Every URL in nix/east-flake.nix and nixos/modules/telemetry.nix can be read by
what it is handed to: an IPv6 host is in brackets (RFC 3986), or its last group
cannot be told from a port."""
import re
from pathlib import Path

import pytest

REPO = Path(__file__).resolve().parents[2]
FILES = ["nix/east-flake.nix", "nixos/modules/telemetry.nix"]
URL = re.compile(r"https?://([^/\"'\s]+)")


def unbracketed(text):
    """The URL authorities in text with an IPv6 host outside brackets."""
    return [a for a in URL.findall(text) if not a.startswith("[") and a.count(":") > 1]


def test_rule_finds_an_unbracketed_host_and_passes_a_bracketed_one():
    assert unbracketed('url = "http://fd00::1:3100/x";') == ["fd00::1:3100"]
    assert unbracketed('url = "http://[fd00::1]:3100/x";') == []
    assert unbracketed('url = "http://192.0.2.1:3100/x";') == []


@pytest.mark.parametrize("name", FILES)
def test_every_ipv6_url_is_bracketed(name):
    text = (REPO / name).read_text(encoding="utf-8")
    assert URL.search(text), "no URL found: the rule is not reading the file"
    assert unbracketed(text) == []
