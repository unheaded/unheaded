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

Regression for 2026-11-11: "tell me about getting the ccna" listed the
kerberos sheet as its source, because vor matches substrings and `ccna` sits
inside `KRB5CCNAME`; and a question about JNCIA-Junos had no source at all,
because no one topic carries every word of a long question. The selection
rule is pkg/vorquery's Select; the Go test TestMatchesPython compares the
helpers.
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

import zhen_rag

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
    'stale copy': [
        _hit('Wotan-Stale-Copy', WT + '/wiki'),
        _hit('pki-overview', 'docs/security'),
    ],
    'rotate ca': [
        _hit('ca-compromise-root-rotation', 'docs/runbook-briefs'),
        _hit('pki-overview', 'docs/security'),
    ],
    # vor matches substrings: `ccna` is inside KRB5CCNAME.
    'ccna': [_hit('kerberos', 'security', line='export KRB5CCNAME=/tmp/krb5cc')],
    'getting': [_hit('pki-overview', 'docs/security')],
    'junos': [
        _hit('junos-lonely', 'networking'),
        _hit('junos-interfaces', 'networking'),
        _hit('junos-architecture', 'networking'),
        _hit('Junos-Stale', WT + '/networking'),
    ],
    'steps': [
        _hit('pki-overview', 'docs/security'),
        _hit('junos-interfaces', 'networking'),
    ],
    'associate': [_hit('junos-architecture', 'networking')],
}

# What a topic's sheet says, beyond its heading.
CONTENT = {
    'pki-overview': 'A stale copy of a certificate is refused. Getting one: '
                    'the steps to rotate the CA are in the runbook.',
    'ca-compromise-root-rotation': 'How to rotate the CA after a compromise.',
    'kerberos': 'export KRB5CCNAME=/tmp/krb5cc_1000',
    'junos-interfaces': 'Steps to configure an interface, then commit.',
    'junos-architecture': 'Routing engine and forwarding engine. Associate '
                          'level steps for the exam are listed below.',
    'junos-lonely': 'show version',
    'junos-unsearched': 'The steps an associate takes first.',
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
    {'name': 'kerberos', 'category': 'security'},
    {'name': 'junos-lonely', 'category': 'networking'},
    {'name': 'junos-interfaces', 'category': 'networking'},
    {'name': 'junos-architecture', 'category': 'networking'},
    {'name': 'junos-unsearched', 'category': 'networking'},
    {'name': 'Junos-Stale', 'category': WT + '/networking'},
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
    def __init__(self, topics_status=200, failing_searches=()):
        self.search_queries = []
        self.listing_calls = 0
        self.fetched = []
        self.topics_status = topics_status
        self.failing_searches = set(failing_searches)

    def get(self, url, timeout=None):
        u = urlparse(url)
        if u.path == '/api/search':
            q = parse_qs(u.query)['q'][0]
            self.search_queries.append(q)
            if q in self.failing_searches:
                return _Resp({}, 500)
            # vor answers null, not [], when nothing matches.
            return _Resp(SEARCH.get(q))
        if u.path == '/api/topics':
            self.listing_calls += 1
            return _Resp(TOPICS, self.topics_status)
        if u.path.startswith('/api/topics/'):
            name = unquote(u.path[len('/api/topics/'):])
            self.fetched.append(name)
            cat = next((t['category'] for t in TOPICS if t['name'] == name), '')
            return _Resp({'name': name, 'category': cat,
                          'content': f'# {name}\n{CONTENT.get(name, "")}',
                          'source_trust': 'external',
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


def test_worktree_copies_are_dropped_from_search_hits(vor, rag):
    assert _ids(rag.retrieve('stale copy', k=5)) == ['pki-overview']


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
    assert vor.search_queries[0] == 'what is this'


def test_topic_listing_is_cached(vor, rag):
    rag.retrieve('what is wotan', k=5)
    rag.retrieve('wotan', k=5)
    assert vor.listing_calls == 1


def test_listing_failure_degrades_to_search_order(monkeypatch, rag):
    fake = FakeVor(topics_status=500)
    monkeypatch.setattr(zhen_rag.requests, 'get', fake.get)
    assert _ids(rag.retrieve('what is wotan', k=5)) == ['wotan-cloud-battle-plan']


# --- 2026-11-11: a source carries the words; long questions go word by word ---


def test_substring_hit_is_not_a_source(vor, rag):
    # vor returns kerberos for `ccna` (KRB5CCNAME). The sheet is read and
    # refused: no word in it starts with `ccna`.
    assert rag.retrieve('ccna', k=5) == []
    assert vor.fetched == ['kerberos']


def test_ccna_question_has_no_source(vor, rag):
    assert rag.retrieve('tell me about getting the ccna', k=5) == []
    assert vor.search_queries == ['getting ccna', 'getting', 'ccna']
    # Second pass: no topic is named by `getting` or `ccna`, so none is read.
    assert vor.fetched == []


def test_long_question_falls_back_word_by_word(vor, rag):
    ids = _ids(rag.retrieve(
        'explain the steps towards Junos, Associate (JNCIA-Junos)', k=5))
    assert vor.search_queries == [
        'steps towards junos associate jncia-junos',
        'steps', 'towards', 'junos', 'associate', 'jncia',
    ]
    # architecture carries junos, steps and associate; unsearched (named by
    # `junos`, returned by no search) the same three; interfaces two.
    assert ids == ['junos-architecture', 'junos-unsearched', 'junos-interfaces']
    # pki-overview carries `steps` but no query word names it; junos-lonely
    # is named but carries one word; the worktree copy is never read.
    assert 'pki-overview' not in vor.fetched
    assert 'junos-lonely' in vor.fetched
    assert 'Junos-Stale' not in vor.fetched


def test_fallback_respects_k(vor, rag):
    ids = _ids(rag.retrieve(
        'explain the steps towards Junos, Associate (JNCIA-Junos)', k=1))
    assert ids == ['junos-architecture']


def test_one_word_question_never_falls_back(vor, rag):
    assert rag.retrieve('ccna?', k=5) == []
    assert vor.search_queries == ['ccna']


def test_fallback_skips_a_word_whose_search_fails(monkeypatch, rag):
    fake = FakeVor(failing_searches={'steps'})
    monkeypatch.setattr(zhen_rag.requests, 'get', fake.get)
    ids = _ids(rag.retrieve(
        'explain the steps towards Junos, Associate (JNCIA-Junos)', k=5))
    assert ids == ['junos-architecture', 'junos-unsearched', 'junos-interfaces']


def test_fallback_without_a_listing_keeps_nothing(monkeypatch, rag):
    fake = FakeVor(topics_status=500)
    monkeypatch.setattr(zhen_rag.requests, 'get', fake.get)
    assert rag.retrieve(
        'explain the steps towards Junos, Associate (JNCIA-Junos)', k=5) == []
    assert fake.fetched == []


def test_first_search_failure_returns_nothing(monkeypatch, rag):
    fake = FakeVor(failing_searches={'wotan'})
    monkeypatch.setattr(zhen_rag.requests, 'get', fake.get)
    assert rag.retrieve('what is wotan', k=5) == []
    assert fake.fetched == []


def test_no_topic_is_fetched_twice(vor, rag):
    rag.retrieve('explain the steps towards Junos, Associate (JNCIA-Junos)', k=5)
    assert len(vor.fetched) == len(set(vor.fetched))


def test_content_is_clipped_after_the_cover_check(vor):
    # The words that make pki-overview a source sit past the clip.
    rag = zhen_rag.RAGPipeline(vor_url='http://vor.test', max_topic_chars=20)
    chunks = rag.retrieve('stale copy', k=5)
    assert _ids(chunks) == ['pki-overview']
    assert chunks[0]['content'].endswith('…[truncated]')
