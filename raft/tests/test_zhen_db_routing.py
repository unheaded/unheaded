# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.

"""REV-2 (independent review 2026-11-15): after migration 053 the zhen_*
tables live only in unheaded_zhen (ADR-149), and the board's kanban_tasks
stays in unheaded_app. The app holds two connections, both as app_zhen: Zhen's
own rows on one, the board's reads on the other. A fake psycopg2 records the
connect arguments and fake connections record each query; no Postgres runs.
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


class _Opened:
    autocommit = None


@pytest.fixture
def connects(monkeypatch):
    seen = []
    fake = types.ModuleType('psycopg2')

    def connect(**kwargs):
        seen.append(kwargs)
        return _Opened()

    fake.connect = connect
    monkeypatch.setitem(sys.modules, 'psycopg2', fake)
    for k in ('ZHEN_DB_NAME', 'ZHEN_BOARD_DB_NAME'):
        monkeypatch.delenv(k, raising=False)
    monkeypatch.setenv('ZHEN_DB_USER', 'app_zhen')
    monkeypatch.setenv('ZHEN_DB_HOST', 'well.example')
    return seen


def test_zhen_rows_default_to_unheaded_zhen(connects):
    zhen_app._pg_connect()
    assert connects[-1]['dbname'] == 'unheaded_zhen'


def test_action_manager_defaults_to_unheaded_zhen(connects):
    action_manager.ActionManager()
    assert connects[-1]['dbname'] == 'unheaded_zhen'


def test_board_defaults_to_unheaded_app_as_the_same_role(connects):
    zhen_app._board_connect()
    board = connects[-1]
    assert board['dbname'] == 'unheaded_app'
    zhen_app._pg_connect()
    zhen = connects[-1]
    for k in ('user', 'host', 'port', 'sslmode', 'sslrootcert'):
        assert board[k] == zhen[k], k


def test_names_come_from_the_environment(connects, monkeypatch):
    monkeypatch.setenv('ZHEN_DB_NAME', 'zhen_test')
    monkeypatch.setenv('ZHEN_BOARD_DB_NAME', 'board_test')
    zhen_app._pg_connect()
    assert connects[-1]['dbname'] == 'zhen_test'
    zhen_app._board_connect()
    assert connects[-1]['dbname'] == 'board_test'
    action_manager.ActionManager()
    assert connects[-1]['dbname'] == 'zhen_test'


class _Cursor:
    def __init__(self, conn):
        self.conn = conn
        self.last = ''

    def execute(self, sql, params=None):
        self.last = sql
        self.conn.queries.append(sql)

    def fetchall(self):
        if 'GROUP BY status' in self.last:
            return [('todo', 1)]
        if 'kanban_tasks' in self.last:
            return [('card-1', 'a card', 'zhen', 0)]
        return [(1, 'file.read', 'done', 'look', 'zhen', '2026-11-15 00:00:00')]

    def close(self):
        pass


class _Conn:
    def __init__(self):
        self.queries = []

    def cursor(self):
        return _Cursor(self)


def test_kanban_reads_go_to_the_board_and_zhen_rows_to_unheaded_zhen(monkeypatch):
    zhen, board = _Conn(), _Conn()
    monkeypatch.setattr(zhen_app, 'pg_conn', zhen)
    monkeypatch.setattr(zhen_app, 'board_conn', board)
    out = zhen_app._build_live_context('how many kanban tasks, and the recent audit?')
    assert board.queries and all('kanban_tasks' in q for q in board.queries)
    assert zhen.queries and all('kanban_tasks' not in q for q in zhen.queries)
    assert any('zhen_actions' in q for q in zhen.queries)
    assert 'Kanban tasks (live from PG' in out and 'Recent audit' in out


def test_no_board_connection_leaves_out_the_kanban_part(monkeypatch):
    zhen = _Conn()
    monkeypatch.setattr(zhen_app, 'pg_conn', zhen)
    monkeypatch.setattr(zhen_app, 'board_conn', None)
    out = zhen_app._build_live_context('how many kanban tasks?') or ''
    assert all('kanban_tasks' not in q for q in zhen.queries)
    assert 'Kanban tasks (live' not in out
