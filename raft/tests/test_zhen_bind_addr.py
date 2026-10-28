# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.

"""zhen_app.py binds ZHEN_BIND_ADDR, loopback when it is unset (S-TRUE-WIRE Steps 360-363, bandit B104).

Until 2026-10-28 the web UI bound 0.0.0.0, every interface on the host, with CSRF (T8) still
interim. Exposing it beyond the host is now an explicit setting; a value that is not an IP
address refuses to start rather than falling back to a bind nobody chose."""

import sys
import types
from pathlib import Path

import pytest

if 'sentence_transformers' not in sys.modules:
    _st = types.ModuleType('sentence_transformers')
    _st.SentenceTransformer = lambda *a, **kw: object()
    sys.modules['sentence_transformers'] = _st

RAFT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(RAFT / 'scripts'))
sys.path.insert(0, str(RAFT))

import zhen_app


def test_unset_binds_loopback(monkeypatch):
    monkeypatch.delenv('ZHEN_BIND_ADDR', raising=False)
    assert zhen_app.bind_address() == '127.0.0.1'


def test_empty_binds_loopback(monkeypatch):
    monkeypatch.setenv('ZHEN_BIND_ADDR', '')
    assert zhen_app.bind_address() == '127.0.0.1'


@pytest.mark.parametrize('addr', ['10.10.20.1', '0.0.0.0', '::1', '192.168.13.2'])  # nosec B104 - test data: an explicit all-interfaces bind is still allowed
def test_explicit_address_is_used(monkeypatch, addr):
    monkeypatch.setenv('ZHEN_BIND_ADDR', addr)
    assert zhen_app.bind_address() == addr


@pytest.mark.parametrize('bad', ['localhost', 'all', '10.10.10', '127.0.0.1:20103', ' 0.0.0.0'])
def test_non_address_refuses(monkeypatch, bad):
    monkeypatch.setenv('ZHEN_BIND_ADDR', bad)
    with pytest.raises(SystemExit):
        zhen_app.bind_address()
