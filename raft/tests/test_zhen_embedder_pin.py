# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.

"""The memory embedder's weights are pinned by sha256 (RT-15).

raft/start-zhen.sh exports ZHEN_EMBEDDER_SHA256 and ZHEN_EMBEDDER_REVISION
from scripts/lib/model-pins.sh; RAGPipeline refuses to start when the cached
weights differ. No network and no real model: the weights path is a temp
file and sentence_transformers is a stub that records its arguments.
"""

import hashlib
import sys
import types
from pathlib import Path

import pytest

_loads = []
if 'sentence_transformers' not in sys.modules:
    _st = types.ModuleType('sentence_transformers')
    _st.SentenceTransformer = lambda *a, **kw: object()
    sys.modules['sentence_transformers'] = _st

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'scripts'))

import zhen_rag


@pytest.fixture
def weights(tmp_path, monkeypatch):
    p = tmp_path / 'model.safetensors'
    p.write_bytes(b'weights v1')
    monkeypatch.setattr(zhen_rag, '_embedder_weights_path', lambda revision: str(p))
    monkeypatch.setattr(zhen_rag, 'SentenceTransformer',
                        lambda *a, **kw: _loads.append((a, kw)) or object())
    for k in ('ZHEN_EMBEDDER_SHA256', 'ZHEN_EMBEDDER_REVISION'):
        monkeypatch.delenv(k, raising=False)
    _loads.clear()
    return p


def _pipe():
    return zhen_rag.RAGPipeline(inference_url='http://llama.test:8081',
                                agentd_url='http://agentd.test:20105')


def test_verify_embedder_pin_accepts_match_and_case(weights):
    good = hashlib.sha256(b'weights v1').hexdigest()
    zhen_rag.verify_embedder_pin(str(weights), good)
    zhen_rag.verify_embedder_pin(str(weights), good.upper())


def test_verify_embedder_pin_refuses_mismatch(weights):
    with pytest.raises(zhen_rag.EmbedderPinMismatch):
        zhen_rag.verify_embedder_pin(str(weights), hashlib.sha256(b'other').hexdigest())


def test_pipeline_refuses_swapped_weights(weights, monkeypatch):
    monkeypatch.setenv('ZHEN_EMBEDDER_SHA256', hashlib.sha256(b'weights v1').hexdigest())
    weights.write_bytes(b'weights v2 (swapped)')
    with pytest.raises(zhen_rag.EmbedderPinMismatch):
        _pipe()
    assert _loads == []  # never loaded


def test_pipeline_loads_pinned_revision(weights, monkeypatch):
    monkeypatch.setenv('ZHEN_EMBEDDER_SHA256', hashlib.sha256(b'weights v1').hexdigest())
    monkeypatch.setenv('ZHEN_EMBEDDER_REVISION', 'abc123')
    _pipe()
    assert _loads == [(('all-MiniLM-L6-v2',), {'revision': 'abc123'})]


def test_pipeline_refuses_pin_without_cached_weights(weights, monkeypatch):
    monkeypatch.setenv('ZHEN_EMBEDDER_SHA256', hashlib.sha256(b'weights v1').hexdigest())
    monkeypatch.setattr(zhen_rag, '_embedder_weights_path', lambda revision: None)
    with pytest.raises(zhen_rag.EmbedderPinMismatch):
        _pipe()


def test_pipeline_unpinned_warns_and_loads(weights, caplog):
    with caplog.at_level('WARNING', logger='zhen_rag'):
        _pipe()
    assert _loads == [(('all-MiniLM-L6-v2',), {})]
    assert 'not pinned' in caplog.text
