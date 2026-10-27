# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.

"""The web UI's direct chat path sends the same prompt bytes as before the
prompt moved into pkg/zhenrag (S-CHAMPION-HARDENING Phase 4, RT-07/RT-08).

The goldens under pkg/zhenrag/testdata/prompts were captured from
zhen_rag.py's literals before the move; the Go side asserts the same goldens,
so Go and Python cannot drift apart without one of the two suites failing.
A prompt change needs `make coding-gate-ci` (memory: project_zhenai_coding_gate).

No network: requests.post is replaced by a recorder.
"""

import json
import sys
import types
from pathlib import Path

import pytest

if 'sentence_transformers' not in sys.modules:
    _st = types.ModuleType('sentence_transformers')
    _st.SentenceTransformer = lambda *a, **kw: object()
    sys.modules['sentence_transformers'] = _st

REPO = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(REPO / 'raft' / 'scripts'))

import zhen_rag

GOLDEN = REPO / 'pkg' / 'zhenrag' / 'testdata' / 'prompts'


def _golden(name):
    return (GOLDEN / name).read_text(encoding='utf-8')


class _Resp:
    status_code = 200

    def raise_for_status(self):
        pass

    def json(self):
        return {'choices': [{'message': {'content': 'ok'}}], 'usage': {}}


@pytest.fixture
def sent(monkeypatch):
    """The JSON body the direct path POSTs to llama-server."""
    calls = []

    def post(url, **kw):
        calls.append(kw['json'])
        return _Resp()

    monkeypatch.setattr(zhen_rag.requests, 'post', post)
    pipe = zhen_rag.RAGPipeline(inference_url='http://llama.test:8081',
                                agentd_url='http://agentd.test:20105',
                                proxy_via_agentd=False)
    chunks = [{'trust': 'external', 'label': 'src', 'category': 'lang',
               'id': 'bash', 'content': 'body'}]
    pipe._generate_via_llama('How do I loop?', chunks)
    assert len(calls) == 1
    return calls[0]


def test_direct_path_system_prompt_is_byte_identical(sent):
    assert sent['messages'][0] == {
        'role': 'system', 'content': _golden('zhen_rag_py.txt')}


def test_direct_path_worked_example_is_byte_identical(sent):
    want = json.loads(_golden('code-comment-example.json'))
    assert sent['messages'][1:3] == want


def test_direct_path_user_turn_ends_with_reminder(sent):
    user = sent['messages'][-1]
    assert user['role'] == 'user'
    assert user['content'] == (
        "References:\n\n--- [external] lang/bash (source: src) ---\nbody"
        "\n\nQuestion: How do I loop?\n\n" + _golden('code-comment-reminder.txt'))


def test_module_constants_match_goldens():
    assert zhen_rag.DEFAULT_SYSTEM_PROMPT == _golden('zhen_rag_py.txt')
    assert zhen_rag.CODE_COMMENT_REMINDER == _golden('code-comment-reminder.txt')
    assert zhen_rag.CODE_COMMENT_EXAMPLE == json.loads(_golden('code-comment-example.json'))
