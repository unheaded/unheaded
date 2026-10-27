# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.

"""The web UI's chat path goes through zhen-agentd (threat model T6a).

S-CLEAN-SLATE M-036: RAGPipeline proxied to cmd/zhen-agentd
/api/v1/agent/ask only when ZHEN_PROXY_VIA_AGENTD opted in, and zhen_app.py
passed False, so every chat answer came straight from llama-server with no
Champion gate. Now the proxy is the default, an unreachable or refusing
daemon is a 503 (never a silent fall-back to llama-server), and the UI
authenticates to the daemon with ZHEN_AGENTD_API_KEY (X-API-Key) unless
ZHEN_AGENTD_NO_AUTH=1 says the daemon runs -no-auth on loopback.

No network: requests.post is replaced by a recorder.
"""

import logging
import sys
import types
from pathlib import Path

import pytest
import requests

if 'sentence_transformers' not in sys.modules:
    _st = types.ModuleType('sentence_transformers')
    _st.SentenceTransformer = lambda *a, **kw: object()
    sys.modules['sentence_transformers'] = _st

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'scripts'))

import zhen_rag

AGENTD = 'http://agentd.test:20105'
LLAMA = 'http://llama.test:8081'


class _Resp:
    def __init__(self, status=200, body=None):
        self.status_code = status
        self._body = body if body is not None else {'answer': 'gated answer'}

    def raise_for_status(self):
        if self.status_code >= 400:
            raise requests.exceptions.HTTPError(f'{self.status_code}', response=self)

    def json(self):
        return self._body


class _Recorder:
    """Stands in for requests.post; raises or answers per the script."""

    def __init__(self, outcome):
        self.outcome = outcome
        self.calls = []

    def __call__(self, url, **kw):
        self.calls.append((url, kw))
        if isinstance(self.outcome, Exception):
            raise self.outcome
        return self.outcome


@pytest.fixture
def clean_env(monkeypatch):
    for k in ('ZHEN_PROXY_VIA_AGENTD', 'ZHEN_AGENTD_API_KEY', 'ZHEN_AGENTD_NO_AUTH'):
        monkeypatch.delenv(k, raising=False)
    zhen_rag._reset_gate_log_for_test()
    return monkeypatch


def _pipe(**kw):
    return zhen_rag.RAGPipeline(inference_url=LLAMA, agentd_url=AGENTD, **kw)


def test_proxy_via_agentd_is_the_default(clean_env):
    assert _pipe().proxy_via_agentd is True


@pytest.mark.parametrize('val', ['0', 'false', 'no', 'off', 'FALSE'])
def test_proxy_env_off_restores_direct_mode(clean_env, val):
    clean_env.setenv('ZHEN_PROXY_VIA_AGENTD', val)
    assert _pipe().proxy_via_agentd is False


@pytest.mark.parametrize('val', ['1', 'true', 'maybe', ''])
def test_proxy_env_anything_else_keeps_the_gate(clean_env, val):
    clean_env.setenv('ZHEN_PROXY_VIA_AGENTD', val)
    assert _pipe().proxy_via_agentd is True


def test_proxy_posts_ask_with_api_key(clean_env):
    clean_env.setenv('ZHEN_AGENTD_API_KEY', 'k-test')
    rec = _Recorder(_Resp(200, {'answer': 'gated answer', 'turns_used': 1}))
    clean_env.setattr(zhen_rag.requests, 'post', rec)
    out = _pipe().generate('what is wotan', [])
    assert out['answer'] == 'gated answer'
    assert out['model'].endswith('@agentd')
    assert len(rec.calls) == 1
    url, kw = rec.calls[0]
    assert url == AGENTD + '/api/v1/agent/ask'
    assert kw['headers']['X-API-Key'] == 'k-test'
    assert kw['json']['goal'] == 'what is wotan'


def test_proxy_no_auth_dev_mode_sends_no_key(clean_env):
    clean_env.setenv('ZHEN_AGENTD_NO_AUTH', '1')
    rec = _Recorder(_Resp())
    clean_env.setattr(zhen_rag.requests, 'post', rec)
    _pipe().generate('q', [])
    assert 'X-API-Key' not in rec.calls[0][1]['headers']


def test_proxy_without_key_fails_loud_and_calls_nothing(clean_env):
    rec = _Recorder(_Resp())
    clean_env.setattr(zhen_rag.requests, 'post', rec)
    with pytest.raises(zhen_rag.AgentdGateUnavailable) as ei:
        _pipe().generate('q', [])
    assert 'ZHEN_AGENTD_API_KEY' in str(ei.value)
    assert rec.calls == []


@pytest.mark.parametrize('outcome', [
    requests.exceptions.ConnectionError('refused'),
    requests.exceptions.Timeout('slow'),
    _Resp(502),
])
def test_proxy_agentd_down_is_unavailable_never_direct(clean_env, outcome):
    clean_env.setenv('ZHEN_AGENTD_API_KEY', 'k-test')
    rec = _Recorder(outcome)
    clean_env.setattr(zhen_rag.requests, 'post', rec)
    with pytest.raises(zhen_rag.AgentdGateUnavailable) as ei:
        _pipe().generate('q', [])
    assert 'zhen-agentd not reachable; the gate is required' in str(ei.value)
    # Exactly one call, to the daemon: no fall-back to llama-server.
    assert [u for u, _ in rec.calls] == [AGENTD + '/api/v1/agent/ask']


@pytest.mark.parametrize('status', [401, 403])
def test_proxy_agentd_refuses_credentials(clean_env, status):
    clean_env.setenv('ZHEN_AGENTD_API_KEY', 'wrong')
    clean_env.setattr(zhen_rag.requests, 'post', _Recorder(_Resp(status)))
    with pytest.raises(zhen_rag.AgentdGateUnavailable) as ei:
        _pipe().generate('q', [])
    assert 'ZHEN_AGENTD_API_KEY' in str(ei.value)


def test_proxy_gate_down_logged_once(clean_env, caplog):
    clean_env.setenv('ZHEN_AGENTD_API_KEY', 'k-test')
    clean_env.setattr(zhen_rag.requests, 'post',
                      _Recorder(requests.exceptions.ConnectionError('refused')))
    p = _pipe()
    with caplog.at_level(logging.WARNING, logger='zhen_rag'):
        for _ in range(3):
            with pytest.raises(zhen_rag.AgentdGateUnavailable):
                p.generate('q', [])
    hits = [r for r in caplog.records if 'gate is required' in r.getMessage()]
    assert len(hits) == 1


def test_direct_mode_still_talks_to_llama(clean_env):
    clean_env.setenv('ZHEN_PROXY_VIA_AGENTD', '0')
    rec = _Recorder(_Resp(200, {'choices': [{'message': {'content': 'direct'}}],
                                'usage': {'completion_tokens': 1}}))
    clean_env.setattr(zhen_rag.requests, 'post', rec)
    out = _pipe().generate('q', [])
    assert out['answer'] == 'direct'
    assert rec.calls[0][0].startswith(LLAMA)
