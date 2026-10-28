# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Cerydwyn's OLLAMA_URL is scheme-checked where it enters (S-TRUE-WIRE Steps 360-363, bandit B310).

static-analysis.yml skips B310 by rule ID on the condition that every configured URL reaching
urllib.request.urlopen is checked for http(s) at its boundary. raft/zhen_app.py had that guard
since 2026-08; the three tomb/cerydwyn scripts did not, so a file:// OLLAMA_URL turned a model
call into a local file read. These tests load each script with a hostile OLLAMA_URL."""
import importlib.util
import pathlib

import pytest

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPTS = ["cerydwyn-daemon.py", "cerydwyn-tui.py", "cerydwyn-test.py"]


def load(name):
    spec = importlib.util.spec_from_file_location(name.replace("-", "_")[:-3], ROOT / "tomb" / "cerydwyn" / name)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


@pytest.mark.parametrize("name", SCRIPTS)
@pytest.mark.parametrize("bad", ["file:///etc/passwd", "ftp://example.test/", "/etc/passwd"])
def test_non_http_ollama_url_refuses(monkeypatch, name, bad):
    monkeypatch.setenv("OLLAMA_URL", bad)
    with pytest.raises(SystemExit):
        load(name)


@pytest.mark.parametrize("name", SCRIPTS)
def test_default_and_http_load(monkeypatch, name):
    monkeypatch.delenv("OLLAMA_URL", raising=False)
    assert load(name).OLLAMA_URL == "http://127.0.0.1:11434"
    monkeypatch.setenv("OLLAMA_URL", "http://10.10.20.5:11434")
    assert load(name).OLLAMA_URL == "http://10.10.20.5:11434"
