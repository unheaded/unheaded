# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""A recalled memory carries its origin (S-CHAMPION-HARDENING Step 183):
the side-channel `matched_memory` names where it came from and when, as
`[memory: <topic> @ <date> via well]`. Memories never enter the prompt."""
import datetime as dt
import sys
import types
from pathlib import Path

import numpy as np

if 'sentence_transformers' not in sys.modules:
    _st = types.ModuleType('sentence_transformers')
    _st.SentenceTransformer = lambda *a, **kw: object()
    sys.modules['sentence_transformers'] = _st

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'scripts'))

import zhen_app


class _Cur:
    def __init__(self, rows):
        self.rows = rows

    def execute(self, sql, *a):
        self.sql = sql

    def fetchall(self):
        return self.rows

    def close(self):
        pass


class _Conn:
    def __init__(self, rows):
        self.rows = rows

    def cursor(self):
        return _Cur(self.rows)


class _Emb:
    def encode(self, text, convert_to_numpy=True):
        return np.array([1.0, 0.0], dtype='float32')


def test_matched_memory_names_its_origin(monkeypatch):
    when = dt.datetime(2026, 10, 1, 12, 0, tzinfo=dt.UTC)
    vec = np.array([1.0, 0.0], dtype='float32').tobytes()
    rows = [(7, "how do I restart wotan safely?", "use the runbook", vec, "qwen-7b", when)]
    monkeypatch.setattr(zhen_app, 'pg_conn', _Conn(rows))
    monkeypatch.setattr(zhen_app, 'rag', types.SimpleNamespace(embedding_model=_Emb()))
    m = zhen_app._search_memories("how do I restart wotan safely?")
    assert m is not None and m['id'] == 7
    assert m['created_at'] == '2026-10-01'
    assert m['provenance'] == '[memory: how do I restart wotan safely? @ 2026-10-01 via well]'


def test_long_question_topic_is_trimmed(monkeypatch):
    vec = np.array([1.0, 0.0], dtype='float32').tobytes()
    q = "x" * 200
    rows = [(1, q, "a", vec, "m", None)]
    monkeypatch.setattr(zhen_app, 'pg_conn', _Conn(rows))
    monkeypatch.setattr(zhen_app, 'rag', types.SimpleNamespace(embedding_model=_Emb()))
    m = zhen_app._search_memories(q)
    assert m['provenance'] == '[memory: ' + 'x' * 60 + '… @ unknown date via well]'
