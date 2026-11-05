# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.

"""The Python clients of the Well pass ZHEN_DB_SSLROOTCERT to libpq, so
ZHEN_DB_SSLMODE=verify-full can check the server. A fake psycopg2 records the
connect arguments; no Postgres runs.
"""

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

import action_manager
import zhen_app


class _Conn:
    autocommit = None


@pytest.fixture
def connect_args(monkeypatch):
    seen = {}
    fake = types.ModuleType('psycopg2')

    def connect(**kwargs):
        seen.update(kwargs)
        return _Conn()

    fake.connect = connect
    monkeypatch.setitem(sys.modules, 'psycopg2', fake)
    return seen


def _open_zhen_app():
    zhen_app._pg_connect()


def _open_action_manager():
    action_manager.ActionManager()


@pytest.mark.parametrize('open_client', [_open_zhen_app, _open_action_manager])
def test_root_cert_reaches_libpq(monkeypatch, connect_args, open_client):
    monkeypatch.setenv('ZHEN_DB_SSLMODE', 'verify-full')
    monkeypatch.setenv('ZHEN_DB_SSLROOTCERT', '/etc/unheaded/well-tls/ca.crt')
    open_client()
    assert connect_args['sslmode'] == 'verify-full'
    assert connect_args['sslrootcert'] == '/etc/unheaded/well-tls/ca.crt'


@pytest.mark.parametrize('open_client', [_open_zhen_app, _open_action_manager])
@pytest.mark.parametrize('value', [None, ''])
def test_no_root_cert_leaves_libpq_its_default(monkeypatch, connect_args, open_client, value):
    monkeypatch.delenv('ZHEN_DB_SSLMODE', raising=False)
    if value is None:
        monkeypatch.delenv('ZHEN_DB_SSLROOTCERT', raising=False)
    else:
        monkeypatch.setenv('ZHEN_DB_SSLROOTCERT', value)
    open_client()
    assert connect_args['sslmode'] == 'require'
    assert connect_args['sslrootcert'] is None
