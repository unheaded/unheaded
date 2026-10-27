# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.

"""zhen_app._pg_log: conversation rows are redacted before the Well sees
them, and nothing is persisted without the redaction rules (retention
policy v1 §11). A fake connection records the INSERT; no Postgres runs.
"""

import json
import sys
import types
from pathlib import Path

if 'sentence_transformers' not in sys.modules:
    _st = types.ModuleType('sentence_transformers')
    _st.SentenceTransformer = lambda *a, **kw: object()
    sys.modules['sentence_transformers'] = _st

RAFT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(RAFT / 'scripts'))
sys.path.insert(0, str(RAFT))

import zhen_app


class _Cursor:
    def __init__(self, sink):
        self.sink = sink

    def execute(self, sql, params):
        self.sink.append((sql, params))

    def close(self):
        pass


class _Conn:
    def __init__(self):
        self.rows = []

    def cursor(self):
        return _Cursor(self.rows)


def test_pg_log_redacts_content_and_sources(monkeypatch):
    conn = _Conn()
    monkeypatch.setattr(zhen_app, 'pg_conn', conn)
    key = 'sk-ant-' + 'z' * 24
    zhen_app._pg_log('user', f'ticket from jane@example.org, key {key}',
                     sources=json.dumps([{'preview': 'cc bob@example.com'}]))
    assert len(conn.rows) == 1
    params = conn.rows[0][1]
    content, sources = params[2], params[3]
    assert 'jane@example.org' not in content and key not in content
    assert '[REDACTED:email]' in content and '[REDACTED:anthropic_key]' in content
    assert json.loads(sources) == [{'preview': 'cc [REDACTED:email]'}]


def test_pg_log_persists_nothing_without_rules(monkeypatch):
    conn = _Conn()
    monkeypatch.setattr(zhen_app, 'pg_conn', conn)
    monkeypatch.setattr(zhen_app, '_REDACT', None)
    zhen_app._pg_log('user', 'mail jane@example.org')
    assert conn.rows == []
