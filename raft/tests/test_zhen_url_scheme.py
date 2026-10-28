# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.

"""The urlopen scheme guard that bandit B310's rule-wide skip rests on (S-TRUE-WIRE Steps 360-363).

static-analysis.yml skips B310 by rule ID because every service URL zhen_app.py hands to
urllib.request.urlopen enters through _http_url_from_env, which refuses anything but http(s):
urlopen would otherwise open file:// and read local files. If this guard goes, the skip is
unjustified, and these tests fail."""

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


@pytest.mark.parametrize('bad', ['file:///etc/passwd', 'ftp://example.test/x', 'data:,x', '/etc/passwd', ''])
def test_guard_refuses_non_http(monkeypatch, bad):
    monkeypatch.setenv('ZHEN_TEST_URL', bad)
    with pytest.raises(ValueError):
        zhen_app._http_url_from_env('ZHEN_TEST_URL', 'http://localhost:1')


@pytest.mark.parametrize('good', ['http://localhost:9876', 'https://vor.test'])
def test_guard_accepts_http(monkeypatch, good):
    monkeypatch.setenv('ZHEN_TEST_URL', good)
    assert zhen_app._http_url_from_env('ZHEN_TEST_URL', 'http://localhost:1') == good


def test_service_urls_go_through_the_guard():
    src = (RAFT / 'zhen_app.py').read_text()
    for var in ('VOR_URL', 'ZHEN_INFERENCE_URL', 'ZHEN_AGENTD_URL'):
        assert f"{var} = _http_url_from_env('{var}'" in src, var
