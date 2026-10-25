# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.

"""Unit tests for RAGPipeline.retrieve() against a fake vor (no network).

Regression for 2026-10-25: "what is wotan" was answered as if Wotan were a
scheduler / only a message bus. vor's search is AND-of-substrings ranked by
how many terms equal a topic-name token, so "what" and "is" ranked the
`what-is-unheaded` ELI5 page first, and vor's 50-line cap let one long doc
with dozens of "Wotan ..." section titles fill every slot for "wotan". The
docs that define Wotan (wiki/Service-Wotan, wiki/Wotan-Memory-Model, the
wotan-memory draft) never reached the model.

The fake vor below reproduces those two behaviours with the real topic names.
"""

import sys
import types
from pathlib import Path
from urllib.parse import parse_qs, unquote, urlparse

import pytest

# zhen_rag imports sentence-transformers at module load for memory recall,
# which retrieve() never touches. Stub it so the unit tier stays offline.
if 'sentence_transformers' not in sys.modules:
    _st = types.ModuleType('sentence_transformers')
    _st.SentenceTransformer = lambda *a, **kw: object()
    sys.modules['sentence_transformers'] = _st

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'scripts'))

import zhen_rag  # noqa: E402

WT = '.claude/worktrees/agent-a586f4d4bea34e922'


def _hit(topic, category, section='', line=''):
    return {'topic': topic, 'category': category, 'section': section,
            'line': line, 'source_kind': 'user-source',
            'source_trust': 'external', 'source_label': 'unheaded'}


# What live vor returned on 2026-10-25 (topic order; lines trimmed).
SEARCH = {
    'what is wotan': [
        _hit('what-is-unheaded', 'docs/binder-book/eli5'),
        _hit('ADR-093-what-shape-is-this-thing', 'docs/adr'),
        _hit('wotan-cloud-battle-plan', 'docs/battle-plans/tools'),
        _hit('ADR-064-wotan-active-active-cluster-k8s-native', 'docs/adr'),
        _hit('what-unheaded-does', 'docs/binder-book/middle-school'),
    ],
    # The 50-hit cap: every hit is one long battle plan.
    'wotan': [_hit('wotan-cloud-battle-plan', 'docs/battle-plans/tools',
                   section=f'Wotan section {i}') for i in range(50)],
    'rotate ca': [
        _hit('ca-compromise-root-rotation', 'docs/runbook-briefs'),
        _hit('pki-overview', 'docs/security'),
    ],
}

TOPICS = [
    {'name': 'what-is-unheaded', 'category': 'docs/binder-book/eli5'},
    {'name': 'ADR-093-what-shape-is-this-thing', 'category': 'docs/adr'},
    {'name': 'what-unheaded-does', 'category': 'docs/binder-book/middle-school'},
    {'name': 'wotan-cloud-battle-plan', 'category': 'docs/battle-plans/tools'},
    {'name': 'ADR-005-wotan-message-backbone', 'category': 'docs/adr'},
    {'name': 'ADR-023-wotan-virtual-memory', 'category': 'docs/adr'},
    {'name': 'Service-Wotan', 'category': 'wiki'},
    {'name': 'Service-Wotan', 'category': WT + '/wiki'},
    {'name': 'Wotan-Memory-Model', 'category': 'wiki'},
    {'name': 'Draft-Wotan-Memory-03', 'category': 'wiki'},
    {'name': 'WOTAN_TOPICS', 'category': 'docs/protocol'},
    {'name': 'Wotan-Stale-Copy', 'category': WT + '/wiki'},
    {'name': 'ca-compromise-root-rotation', 'category': 'docs/runbook-briefs'},
    {'name': 'pki-overview', 'category': 'docs/security'},
]


class _Resp:
    def __init__(self, payload, status=200):
        self._payload = payload
        self.status_code = status

    def raise_for_status(self):
        if self.status_code >= 400:
            raise zhen_rag.requests.HTTPError(f'status {self.status_code}')

    def json(self):
        return self._payload


class FakeVor:
    def __init__(self, topics_status=200):
        self.search_queries = []
        self.listing_calls = 0
        self.topics_status = topics_status

    def get(self, url, timeout=None):
        u = urlparse(url)
        if u.path == '/api/search':
            q = parse_qs(u.query)['q'][0]
            self.search_queries.append(q)
            return _Resp(SEARCH.get(q, []))
        if u.path == '/api/topics':
            self.listing_calls += 1
            return _Resp(TOPICS, self.topics_status)
        if u.path.startswith('/api/topics/'):
            name = unquote(u.path[len('/api/topics/'):])
            cat = next((t['category'] for t in TOPICS if t['name'] == name), '')
            return _Resp({'name': name, 'category': cat,
                          'content': f'# {name}\n', 'source_trust': 'external',
                          'source_kind': 'user-source', 'source_label': 'unheaded'})
        return _Resp({}, 404)


@pytest.fixture
def vor(monkeypatch):
    fake = FakeVor()
    monkeypatch.setattr(zhen_rag.requests, 'get', fake.get)
    return fake


@pytest.fixture
def rag():
    return zhen_rag.RAGPipeline(vor_url='http://vor.test')


def _ids(chunks):
    return [c['id'] for c in chunks]


def test_question_words_are_not_sent_to_vor(vor, rag):
    rag.retrieve('What is Wotan?', k=5)
    assert vor.search_queries == ['wotan']


def test_what_is_wotan_retrieves_the_wotan_docs(vor, rag):
    ids = _ids(rag.retrieve('what is wotan', k=5))
    assert ids[0] == 'Service-Wotan'
    assert 'Wotan-Memory-Model' in ids
    assert 'wotan-cloud-battle-plan' in ids
    assert 'what-is-unheaded' not in ids
    assert len(ids) == 5 and len(set(ids)) == 5


def test_worktree_copies_are_not_name_matches(vor, rag):
    ids = _ids(rag.retrieve('wotan', k=20))
    assert 'Wotan-Stale-Copy' not in ids
    assert ids.count('Service-Wotan') == 1


def test_name_matches_ordered_most_specific_first(vor, rag):
    ids = _ids(rag.retrieve('wotan', k=20))
    named = [i for i in ids if i != 'wotan-cloud-battle-plan']
    assert named[:3] == ['Service-Wotan', 'WOTAN_TOPICS', 'Wotan-Memory-Model']


def test_without_name_match_vor_order_is_kept(vor, rag):
    ids = _ids(rag.retrieve('how do I rotate the CA?', k=5))
    assert vor.search_queries == ['rotate ca']
    assert ids == ['ca-compromise-root-rotation', 'pki-overview']


def test_all_stopwords_falls_back_to_the_question(vor, rag):
    rag.retrieve('what is this?', k=5)
    assert vor.search_queries == ['what is this']


def test_topic_listing_is_cached(vor, rag):
    rag.retrieve('what is wotan', k=5)
    rag.retrieve('wotan', k=5)
    assert vor.listing_calls == 1


def test_listing_failure_degrades_to_search_order(monkeypatch, rag):
    fake = FakeVor(topics_status=500)
    monkeypatch.setattr(zhen_rag.requests, 'get', fake.get)
    assert _ids(rag.retrieve('what is wotan', k=5)) == ['wotan-cloud-battle-plan']
