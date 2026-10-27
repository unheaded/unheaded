# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.

"""zhen_app.py: the chat path's gate (M-036) and the CSRF interim (T8).

Imports the Flask app with sentence-transformers stubbed (no model load) and
drives it with Flask's test client: nothing listens, no daemon or model runs.

CSRF (threat model T8, interim until the Go port): a browser request
(it carries Origin or Sec-Fetch-Site) that changes state must echo the
per-session zhen_csrf cookie (SameSite=Strict) in X-Zhen-CSRF. A cross-site
page cannot read that cookie, so it cannot forge the header. Requests with
neither header are not from a browser (curl, zhen-mcp, eval scripts)
and carry no ambient browser state to ride, so they are not CSRF.
"""

import io
import json
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
import zhen_rag

ORIGIN = 'http://zhen.test'


@pytest.fixture
def client():
    zhen_app.app.config['TESTING'] = True
    with zhen_app.app.test_client() as c:
        yield c


def _csrf_cookie(client):
    r = client.get('/', base_url=ORIGIN)
    assert r.status_code == 200
    cookies = [h for h in r.headers.getlist('Set-Cookie') if h.startswith('zhen_csrf=')]
    assert cookies, 'GET / must set the zhen_csrf cookie'
    return cookies[0].split(';', 1)[0].split('=', 1)[1], cookies[0]


# ── CSRF ────────────────────────────────────────────────────────────────

def test_csrf_cookie_is_samesite_strict_and_random(client):
    tok1, raw = _csrf_cookie(client)
    assert 'SameSite=Strict' in raw
    assert len(tok1) >= 32
    with zhen_app.app.test_client() as other:
        tok2, _ = _csrf_cookie(other)
    assert tok1 != tok2


def test_csrf_browser_post_without_header_is_403(client):
    _csrf_cookie(client)
    r = client.post('/api/v1/highlight', base_url=ORIGIN, json={'blocks': []},
                    headers={'Origin': ORIGIN})
    assert r.status_code == 403
    assert 'CSRF' in r.get_json()['error']


def test_csrf_cross_site_post_without_cookie_is_403(client):
    r = client.post('/api/v1/highlight', base_url=ORIGIN, json={'blocks': []},
                    headers={'Origin': 'http://evil.test', 'Sec-Fetch-Site': 'cross-site'})
    assert r.status_code == 403


def test_csrf_browser_post_with_wrong_header_is_403(client):
    _csrf_cookie(client)
    r = client.post('/api/v1/highlight', base_url=ORIGIN, json={'blocks': []},
                    headers={'Origin': ORIGIN, 'X-Zhen-CSRF': 'not-the-token'})
    assert r.status_code == 403


def test_csrf_browser_post_with_header_reaches_handler(client):
    tok, _ = _csrf_cookie(client)
    r = client.post('/api/v1/highlight', base_url=ORIGIN, json={'blocks': []},
                    headers={'Origin': ORIGIN, 'Sec-Fetch-Site': 'same-origin',
                             'X-Zhen-CSRF': tok})
    assert r.status_code == 200
    assert r.get_json() == {'html': []}


def test_csrf_non_browser_post_is_not_csrf(client):
    r = client.post('/api/v1/highlight', base_url=ORIGIN, json={'blocks': []})
    assert r.status_code == 200


def test_csrf_get_is_never_blocked(client):
    r = client.get('/api/v1/highlight.css', base_url=ORIGIN,
                   headers={'Origin': 'http://evil.test', 'Sec-Fetch-Site': 'cross-site'})
    assert r.status_code == 200


def test_csrf_page_sends_header_on_same_origin_posts():
    html = (RAFT / 'static' / 'index.html').read_text()
    assert 'X-Zhen-CSRF' in html
    assert 'zhen_csrf' in html


def test_csrf_view_source_escapes_path(client):
    # A reflected script would read the CSRF cookie and defeat the token.
    r = client.get('/view/source?path=</title><script>alert(1)</script>', base_url=ORIGIN)
    assert r.status_code == 200
    assert b'<script>alert(1)</script>' not in r.data


# ── chat path through zhen-agentd ───────────────────────────────────────

class _GateDownRag:
    inference_url = 'http://127.0.0.1:1'

    def query(self, *a, **kw):
        raise zhen_rag.AgentdGateUnavailable(zhen_rag.GATE_REQUIRED_MSG)


def test_agentd_down_query_is_503(client, monkeypatch):
    monkeypatch.setattr(zhen_app, 'rag', _GateDownRag())
    monkeypatch.setattr(zhen_app, '_search_memories', lambda q: None)
    monkeypatch.setattr(zhen_app, '_build_live_context', lambda q: '')
    r = client.post('/api/v1/query', base_url=ORIGIN,
                    json={'question': 'explain wotan topics please'})
    assert r.status_code == 503
    assert r.get_json()['error'] == zhen_rag.GATE_REQUIRED_MSG


def test_agentd_proxy_is_on_in_the_app_by_default(monkeypatch):
    monkeypatch.delenv('ZHEN_PROXY_VIA_AGENTD', raising=False)
    assert zhen_rag.proxy_via_agentd_from_env() is True
    src = (RAFT / 'zhen_app.py').read_text()
    assert 'proxy_via_agentd=proxy_via_agentd_from_env()' in src


class _FakeUrlopenResp(io.BytesIO):
    status = 200

    def __enter__(self):
        return self

    def __exit__(self, *a):
        return False


def test_agentd_kanban_create_goes_through_the_app_with_key(client, monkeypatch):
    seen = {}

    def fake_urlopen(req, timeout=None):
        seen['url'] = req.full_url
        seen['headers'] = {k.lower(): v for k, v in req.header_items()}
        seen['body'] = json.loads(req.data.decode())
        return _FakeUrlopenResp(json.dumps({'status': 'ok', 'result': {}}).encode())

    import urllib.request as ur
    monkeypatch.setattr(ur, 'urlopen', fake_urlopen)
    monkeypatch.setattr(zhen_app, 'AGENTD_API_KEY', 'k-test')
    monkeypatch.setattr(zhen_app, 'AGENTD_AUTH_ERROR', None)
    r = client.post('/api/v1/kanban/create', base_url=ORIGIN,
                    json={'task': {'id': 'task-zhen-1', 'title': 't'}})
    assert r.status_code == 200, r.get_json()
    assert seen['url'].endswith('/api/v1/tool/exec')
    assert seen['headers']['x-api-key'] == 'k-test'
    assert seen['body']['tool'] == 'kanban_create'
    assert seen['body']['justification'][0]['source_trust'] == 'direct-user'


def test_agentd_kanban_create_without_auth_config_is_503(client, monkeypatch):
    monkeypatch.setattr(zhen_app, 'AGENTD_API_KEY', '')
    monkeypatch.setattr(zhen_app, 'AGENTD_AUTH_ERROR', 'ZHEN_AGENTD_API_KEY is not set')
    r = client.post('/api/v1/kanban/create', base_url=ORIGIN,
                    json={'task': {'id': 'task-zhen-1', 'title': 't'}})
    assert r.status_code == 503


def test_agentd_page_no_longer_posts_to_the_daemon_directly():
    html = (RAFT / 'static' / 'index.html').read_text()
    assert ':20105/api/v1/tool/exec' not in html


# ── CORS (found 2026-10-27, S-CLEAN-SLATE Phase 3) ──────────────────────
# CORS(app) answered every origin, so any page a LAN user visited could read
# Zhen's GET responses, conversation history included. The UI is served by
# this app (same origin) and every other surface only links to it, so no
# cross-origin read is needed: no Access-Control-Allow-Origin, for any origin.

@pytest.mark.parametrize('path', ['/health', '/api/v1/conversations'])
def test_no_cors_grant_to_another_origin(client, path):
    r = client.get(path, base_url=ORIGIN, headers={'Origin': 'http://evil.example'})
    assert 'Access-Control-Allow-Origin' not in r.headers, (path, r.headers.get('Access-Control-Allow-Origin'))
    assert 'Access-Control-Allow-Credentials' not in r.headers


def test_no_cors_preflight_grant(client):
    r = client.options('/api/v1/conversations', base_url=ORIGIN,
                       headers={'Origin': 'http://evil.example', 'Access-Control-Request-Method': 'GET'})
    assert 'Access-Control-Allow-Origin' not in r.headers
