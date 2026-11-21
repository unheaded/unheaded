# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.

"""zhen_app.py: the chat path's gate (M-036) and the CSRF interim (T8).

Imports the Flask app with sentence-transformers stubbed (no model load) and
drives it with Flask's test client: nothing listens, no daemon or model runs.

CSRF (threat model T8, interim until the Go port): a request that changes
state must come from this UI's own origin and echo the per-session zhen_csrf
cookie (SameSite=Strict) in X-Zhen-CSRF. A cross-site page cannot read that
cookie, so it cannot forge the header. Only the scripted read and chat routes
take a request with no browser headers at all (curl, eval scripts).
"""

import io
import json
import sys
import time
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
import zhen_login
import zhen_rag

ORIGIN = 'http://zhen.test'
# A password is taken over plain http only on loopback, so the login is asked there.
LOCAL = 'http://127.0.0.1:20103'


@pytest.fixture
def client(monkeypatch):
    monkeypatch.setenv('ZHEN_ALLOWED_HOSTS', 'zhen.test')
    zhen_app.app.config['TESTING'] = True
    with zhen_app.app.test_client() as c:
        yield c


def _log_in(client, token='person-token'):  # nosec B107 - fixture token, no credential
    """A live operator session, as zhen_login.Login.login leaves one: the
    login itself is test_zhen_login.py's."""
    sid = zhen_app.LOGIN.sessions.put(zhen_login.Session(token, 'stevie', ('operator',), time.time() + 600))
    client.set_cookie(zhen_login.COOKIE_NAME, sid, domain='zhen.test')


def _csrf_cookie(client):
    r = client.get('/', base_url=ORIGIN)
    assert r.status_code == 200
    cookies = [h for h in r.headers.getlist('Set-Cookie') if h.startswith('zhen_csrf=')]
    assert cookies, 'GET / must set the zhen_csrf cookie'
    return cookies[0].split(';', 1)[0].split('=', 1)[1], cookies[0]


def _browser(client):
    """The headers the page sends on a state-changing request."""
    tok, _ = _csrf_cookie(client)
    return {'Origin': ORIGIN, 'Sec-Fetch-Site': 'same-origin', 'X-Zhen-CSRF': tok}


# ── CSRF ────────────────────────────────────────────────────────────────

def test_csrf_cookie_is_samesite_strict_and_random(client):
    tok1, raw = _csrf_cookie(client)
    assert 'SameSite=Strict' in raw
    assert len(tok1) >= 32
    with zhen_app.app.test_client() as other:
        tok2, _ = _csrf_cookie(other)
    assert tok1 != tok2


def test_csrf_browser_post_without_header_is_403(client):
    _log_in(client)
    _csrf_cookie(client)
    r = client.post('/api/v1/highlight', base_url=ORIGIN, json={'blocks': []},
                    headers={'Origin': ORIGIN})
    assert r.status_code == 403
    assert 'CSRF' in r.get_json()['error']


def test_csrf_cross_site_post_without_cookie_is_403(client):
    _log_in(client)
    r = client.post('/api/v1/highlight', base_url=ORIGIN, json={'blocks': []},
                    headers={'Origin': 'http://evil.test', 'Sec-Fetch-Site': 'cross-site'})
    assert r.status_code == 403


def test_csrf_browser_post_with_wrong_header_is_403(client):
    _log_in(client)
    _csrf_cookie(client)
    r = client.post('/api/v1/highlight', base_url=ORIGIN, json={'blocks': []},
                    headers={'Origin': ORIGIN, 'X-Zhen-CSRF': 'not-the-token'})
    assert r.status_code == 403


def test_csrf_browser_post_with_header_reaches_handler(client):
    _log_in(client)
    tok, _ = _csrf_cookie(client)
    r = client.post('/api/v1/highlight', base_url=ORIGIN, json={'blocks': []},
                    headers={'Origin': ORIGIN, 'Sec-Fetch-Site': 'same-origin',
                             'X-Zhen-CSRF': tok})
    assert r.status_code == 200
    assert r.get_json() == {'html': []}


def test_csrf_non_browser_post_is_not_csrf(client):
    # No issuer in the test, so the login answers 400 or 503; the point is
    # that a request with no browser headers is not a CSRF refusal.
    r = client.post('/auth/login', base_url=LOCAL, json={})
    assert _reached_handler(r)
    assert r.status_code in (400, 503)


def test_csrf_get_is_never_blocked(client):
    r = client.get('/api/v1/highlight.css', base_url=ORIGIN,
                   headers={'Origin': 'http://evil.test', 'Sec-Fetch-Site': 'cross-site'})
    assert r.status_code == 200


def test_csrf_page_sends_header_on_same_origin_posts():
    html = (RAFT / 'static' / 'chat.js').read_text()
    assert 'X-Zhen-CSRF' in html
    assert 'zhen_csrf' in html


def _js_function(html, name):
    """The source of one top-level `function name(` in the page, to the next
    top-level function."""
    import re
    m = re.search(r'^(async )?function ' + re.escape(name) + r'\(', html, re.MULTILINE)
    assert m, f'the page has no function {name}'
    nxt = re.search(r'^(async )?function \w+\(', html[m.end():], re.MULTILINE)
    return html[m.start():m.end() + (nxt.start() if nxt else len(html))]


def test_page_refetches_stats_after_login():
    """D-03: stats need a login, and were fetched once at page load, so the
    sidebar showed `?` after logging in. One refreshStats(), called at load
    and from the login handler; before login it says so instead of `?`."""
    html = (RAFT / 'static' / 'chat.js').read_text()
    stats_fetch = "fetch(API + '/api/v1/stats')"
    assert stats_fetch in _js_function(html, 'refreshStats')
    # The health check and the welcome go through refreshStats; neither keeps
    # a fetch of its own that a login would not repeat. (The model switch
    # polls stats for its own reason and is left alone.)
    assert stats_fetch not in _js_function(html, 'checkHealth')
    welcome = html[html.index('// Welcome'):]
    assert stats_fetch not in welcome and 'refreshStats()' in welcome
    assert "|| '?'" not in welcome
    login = html[html.index("getElementById('login-form').addEventListener('submit'"):
                 html.index("getElementById('login-logout').addEventListener")]
    assert 'refreshStats()' in login
    assert 'refreshStats()' in _js_function(html, 'checkHealth')
    assert 'log in to see' in _js_function(html, 'refreshStats')
    assert "'?'" not in _js_function(html, 'refreshStats')


def test_page_keeps_the_question_when_the_session_has_ended():
    """D-03: a 401 on the chat send opens the login panel (the fetch wrapper),
    puts the question back in the box, and sends nothing again by itself."""
    html = (RAFT / 'static' / 'chat.js').read_text()
    ask = _js_function(html, 'ask')
    i = ask.index('r.status === 401')
    branch = ask[i:ask.index('}', ask.index('return;', i))]
    assert 'qInput.value = question' in branch
    assert "qInput.value = ''" not in branch
    assert 'ask(' not in branch and 'fetch(' not in branch
    # A login after the session ended keeps the page (and the box) as it is;
    # only the first login of a page load reloads it.
    login = html[html.index("getElementById('login-form').addEventListener('submit'"):
                 html.index("getElementById('login-logout').addEventListener")]
    assert 'loadedLoggedIn' in login
    assert login.count('location.reload()') == 1


def test_csrf_view_source_escapes_path(client):
    # A reflected script would read the CSRF cookie and defeat the token.
    _log_in(client)
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
    _log_in(client)
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


def test_agentd_kanban_create_goes_through_the_app_as_the_person(client, monkeypatch):
    seen = {}

    def fake_urlopen(req, timeout=None):
        seen['url'] = req.full_url
        seen['headers'] = {k.lower(): v for k, v in req.header_items()}
        seen['body'] = json.loads(req.data.decode())
        return _FakeUrlopenResp(json.dumps({'status': 'ok', 'result': {}}).encode())

    import urllib.request as ur
    monkeypatch.setattr(ur, 'urlopen', fake_urlopen)
    _log_in(client)
    r = client.post('/api/v1/kanban/create', base_url=ORIGIN, headers=_browser(client),
                    json={'task': {'id': 'task-zhen-1', 'title': 't'}})
    assert r.status_code == 200, r.get_json()
    assert seen['url'].endswith('/api/v1/tool/exec')
    assert seen['headers']['authorization'] == 'Bearer person-token'
    assert 'x-api-key' not in seen['headers']
    assert seen['body']['tool'] == 'kanban_create'
    assert seen['body']['justification'][0]['source_trust'] == 'direct-user'


def test_agentd_kanban_create_without_a_login_is_refused(client):
    r = client.post('/api/v1/kanban/create', base_url=ORIGIN, headers=_browser(client),
                    json={'task': {'id': 'task-zhen-1', 'title': 't'}})
    assert r.status_code == 401


def test_agentd_page_no_longer_posts_to_the_daemon_directly():
    html = (RAFT / 'pages' / 'chat.html').read_text() + (RAFT / 'static' / 'chat.js').read_text()
    assert ':20111/api/v1/tool/exec' not in html


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


# ── Scheduler commands → zhen-agentd -scheduler (S-CHAMPION-HARDENING Step 165) ──

def test_schedule_commands_call_agentd_not_a_local_scheduler(monkeypatch):
    import subprocess

    import zhen_scheduler_client
    calls = []

    def fake_call(url, headers, op, body=None, timeout=30):
        calls.append((op, body))
        if op == 'list':
            return 200, {'jobs': [], 'paused': False, 'config': 'x'}
        return 200, {'status': 'ok', 'result': {'name': 'service-health-sweep', 'runbook': 'observe/service-health-sweep', 'next_due': 't'}}

    def no_subprocess(*a, **kw):
        raise AssertionError(f'a schedule command ran a subprocess: {a}')

    monkeypatch.setattr(zhen_scheduler_client, 'call', fake_call)
    monkeypatch.setattr(subprocess, 'run', no_subprocess)
    for q, op in (('schedule service-health-sweep every 6h', 'add'), ('unschedule service-health-sweep', 'remove'),
                  ('list schedules', 'list'), ('emergency stop', 'pause'), ('resume schedules', 'pause')):
        _, handled = zhen_app._try_command(q)
        assert handled, q
        assert calls[-1][0] == op, (q, calls[-1])
    assert calls[0][1] == {'name': 'service-health-sweep', 'every': '6h'}
    assert calls[3][1] == {'paused': True} and calls[4][1] == {'paused': False}
    assert 'zhen_scheduler' not in sys.modules or not hasattr(sys.modules['zhen_scheduler'], 'add_schedule')


def test_held_schedule_change_points_at_the_client_that_can_confirm(monkeypatch):
    import zhen_scheduler_client
    held = {'status': 'pending_confirmation', 'reason': 'held', 'pending_token': 'tok-secret'}  # nosec B105 - fixture string, no credential
    monkeypatch.setattr(zhen_scheduler_client, 'call', lambda *a, **kw: (403, held))
    for q in ('schedule service-health-sweep every 6h', 'unschedule service-health-sweep', 'resume schedules'):
        answer, handled = zhen_app._try_command(q)
        assert handled, q
        assert '/schedule' in answer['answer'] and 'zhen-cli' in answer['answer'], q
        assert 'tok-secret' not in answer['answer'], q


def test_emergency_stop_says_how_to_come_back(monkeypatch):
    import zhen_scheduler_client
    monkeypatch.setattr(zhen_scheduler_client, 'call', lambda *a, **kw: (200, {'status': 'ok'}))
    answer, _ = zhen_app._try_command('emergency stop')
    assert '/schedule resume' in answer['answer']
    assert 'until `resume schedules`' not in answer['answer']


def test_help_offers_only_what_the_web_ui_does():
    answer, handled = zhen_app._try_command('help')
    assert handled
    for gone in ('`run <name>`', '`run --dry <name>`', '`schedule <runbook>', '`unschedule <runbook>`', '`resume schedules`'):
        assert gone not in answer['answer'], gone
    assert '/runbook exec' in answer['answer'] and '/schedule resume' in answer['answer']


# ── runbooks: chat runs nothing, and a name must be on the list (H5) ────

@pytest.fixture
def runbooks(tmp_path, monkeypatch):
    root = tmp_path / 'runbooks'
    (root / 'observe').mkdir(parents=True)
    (root / 'observe' / 'health-sweep.yaml').write_text('metadata:\n  name: health-sweep\n  risk: low\nsteps: []\n')
    (tmp_path / 'outside.yaml').write_text('metadata:\n  name: outside\nsteps: []\n')
    monkeypatch.setattr(zhen_app, 'RUNBOOKS_DIR', str(root))
    return root


def _no_subprocess(monkeypatch):
    import subprocess

    def refuse(*a, **kw):
        raise AssertionError(f'a chat command started a process: {a}')
    monkeypatch.setattr(subprocess, 'run', refuse)


@pytest.mark.parametrize('question', [
    'run health-sweep', 'execute health-sweep', 'run --dry health-sweep', 'run --dry-run health-sweep',
])
def test_chat_run_of_a_known_runbook_starts_nothing(runbooks, monkeypatch, question):
    _no_subprocess(monkeypatch)
    result, handled = zhen_app._try_command(question)
    assert handled
    assert 'zhen-cli' in result['answer']
    assert 'SUCCESS' not in result['answer']


@pytest.mark.parametrize('question', [
    'run ../outside', 'run ../../outside', 'run *', 'execute **/*', 'run the tests please',
])
def test_chat_run_of_anything_else_is_not_a_command(runbooks, monkeypatch, question):
    _no_subprocess(monkeypatch)
    _, handled = zhen_app._try_command(question)
    assert not handled


@pytest.mark.parametrize('name', ['..', '*', '[a-z]*', '?ealth-sweep', 'health*', 'outside'])
def test_runbook_routes_take_only_a_listed_name(client, runbooks, name):
    from urllib.parse import quote
    _log_in(client)
    assert client.get(f'/api/v1/runbooks/{quote(name, safe="")}', base_url=ORIGIN).status_code == 404


def test_runbook_route_serves_a_listed_name(client, runbooks):
    _log_in(client)
    r = client.get('/api/v1/runbooks/health-sweep', base_url=ORIGIN)
    assert r.status_code == 200
    assert r.get_json()['parsed']['metadata']['name'] == 'health-sweep'


def test_runbook_link_out_of_the_tree_is_not_listed(runbooks):
    (runbooks / 'observe' / 'linked.yaml').symlink_to(runbooks.parent / 'outside.yaml')
    assert zhen_app._runbook_path('linked') is None
    assert zhen_app._runbook_path('health-sweep') is not None


def test_app_never_starts_the_runner_itself():
    src = (RAFT / 'zhen_app.py').read_text()
    assert "scripts/run-runbook.py')" not in src


# ── strict Host and Origin (H5) ─────────────────────────────────────────

GATED_ROUTES = [
    '/api/v1/runbooks/health-sweep/execute', '/api/v1/models/switch', '/api/v1/kanban/create',
    '/api/v1/approvals', '/api/v1/approvals/1/approve', '/api/v1/approvals/1/deny',
    '/api/v1/champion/read', '/api/v1/remember', '/api/v1/forget', '/api/v1/generate-file',
]


def _reached_handler(r):
    return not (r.status_code == 403 and 'CSRF' in (r.get_json() or {}).get('error', ''))


@pytest.mark.parametrize('route', GATED_ROUTES)
def test_gated_route_needs_a_browser_session_with_no_headers_at_all(client, route):
    r = client.post(route, base_url=ORIGIN, json={})
    assert r.status_code == 403
    assert 'CSRF' in r.get_json()['error']


@pytest.mark.parametrize('route', ['/auth/login'])
def test_scripted_routes_stay_open_to_a_caller_with_no_browser_headers(client, route):
    assert _reached_handler(client.post(route, base_url=LOCAL, json={}))


def test_the_highlighter_is_not_a_scripted_route(client):
    """It was open before login until S-ZHEN-FRONT; a bare POST is now a CSRF
    refusal like any other route the page alone calls."""
    r = client.post('/api/v1/highlight', base_url=ORIGIN, json={'blocks': []})
    assert r.status_code == 403
    assert 'CSRF' in r.get_json()['error']


@pytest.mark.parametrize('origin', [
    'http://evil.test', 'https://zhen.test.evil.test', 'http://zhen.test:1', 'null', 'zhen.test', 'file://zhen.test',
])
def test_a_token_does_not_excuse_another_origin(client, origin):
    _log_in(client)
    tok, _ = _csrf_cookie(client)
    r = client.post('/api/v1/highlight', base_url=ORIGIN, json={'blocks': []},
                    headers={'Origin': origin, 'X-Zhen-CSRF': tok})
    assert r.status_code == 403


@pytest.mark.parametrize('site', ['cross-site', 'same-site', 'none'])
def test_a_token_does_not_excuse_another_fetch_site(client, site):
    _log_in(client)
    tok, _ = _csrf_cookie(client)
    r = client.post('/api/v1/highlight', base_url=ORIGIN, json={'blocks': []},
                    headers={'Origin': ORIGIN, 'Sec-Fetch-Site': site, 'X-Zhen-CSRF': tok})
    assert r.status_code == 403


def test_a_browser_marker_without_origin_is_refused(client):
    _log_in(client)
    tok, _ = _csrf_cookie(client)
    r = client.post('/api/v1/highlight', base_url=ORIGIN, json={'blocks': []},
                    headers={'Sec-Fetch-Site': 'same-origin', 'X-Zhen-CSRF': tok})
    assert r.status_code == 403


@pytest.mark.parametrize('method', ['get', 'post'])
def test_a_host_off_the_list_is_refused_on_every_method(client, method):
    r = getattr(client, method)('/health', base_url='http://rebound.test:20103')
    assert r.status_code == 403
    assert 'Host' in r.get_json()['error']


@pytest.mark.parametrize('host', ['localhost', '127.0.0.1', '[::1]', 'localhost:20103', '127.0.0.1:20103'])
def test_loopback_hosts_are_on_the_list(client, host):
    assert client.get('/health', headers={'Host': host}).status_code == 200


def test_an_address_the_front_listens_on_is_served_once_it_is_listed(client, monkeypatch):
    # The app binds nothing (ADR-138), so no address is on the list by itself.
    assert client.get('/health', headers={'Host': '10.10.20.1:20103'}).status_code == 403
    monkeypatch.setenv('ZHEN_ALLOWED_HOSTS', '10.10.20.1')
    assert client.get('/health', headers={'Host': '10.10.20.1:20103'}).status_code == 200


def test_the_retired_bind_setting_adds_no_host(client, monkeypatch):
    monkeypatch.setenv('ZHEN_BIND_ADDR', '10.10.20.1')
    assert client.get('/health', headers={'Host': '10.10.20.1:20103'}).status_code == 403


# ── /api/v1/champion/read judges the real path (M21) ────────────────────

@pytest.fixture
def sandbox_root(tmp_path, monkeypatch):
    root = tmp_path / 'unheaded'
    (root / 'pkg').mkdir(parents=True)
    (root / 'pkg' / 'ok.go').write_text('package ok\n')
    monkeypatch.setattr(zhen_app, 'CHAMPION_ALLOWED', [str(root) + '/'])
    return root


def test_champion_read_allows_a_file_in_the_sandbox(sandbox_root):
    path, err = zhen_app.champion_validate_path(str(sandbox_root / 'pkg' / 'ok.go'))
    assert err is None and path.endswith('ok.go')


def test_champion_read_refuses_a_sibling_of_the_root(sandbox_root):
    sibling = sandbox_root.parent / 'unheaded-x'
    sibling.mkdir()
    (sibling / 'f').write_text('x\n')
    path, err = zhen_app.champion_validate_path(str(sibling / 'f'))
    assert path is None and err


def test_champion_read_refuses_a_link_out_of_the_root(sandbox_root):
    (sandbox_root.parent / 'outside').write_text('x\n')
    (sandbox_root / 'pkg' / 'link').symlink_to(sandbox_root.parent / 'outside')
    path, err = zhen_app.champion_validate_path(str(sandbox_root / 'pkg' / 'link'))
    assert path is None and err


def test_champion_read_refuses_a_link_to_a_denied_name_in_the_root(sandbox_root):
    (sandbox_root / 'server.key').write_text('x\n')
    (sandbox_root / 'pkg' / 'notes').symlink_to(sandbox_root / 'server.key')
    path, err = zhen_app.champion_validate_path(str(sandbox_root / 'pkg' / 'notes'))
    assert path is None and err


@pytest.mark.parametrize('rel', ['tls/server.key', 'tls/ca.PEM', '.git/config', 'sub/.git/config', '.env'])
def test_champion_read_refuses_key_material_and_git_config(sandbox_root, rel):
    p = sandbox_root / rel
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text('x\n')
    path, err = zhen_app.champion_validate_path(str(p))
    assert path is None and err


@pytest.mark.parametrize('rel', ['pkg/keyboard.go', 'docs/pem-format.md', '.gitignore'])
def test_champion_read_allows_names_that_only_look_alike(sandbox_root, rel):
    p = sandbox_root / rel
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text('x\n')
    path, err = zhen_app.champion_validate_path(str(p))
    assert err is None and path


# ── source viewer ───────────────────────────────────────────────────────
# A sheet can come from an `external` source. Three layers keep it text:
# raft/zhen_markdown.py (test_zhen_markdown.py), the page's own check
# (static/source-view.js), and the policy the route sends.

class _VorReply(io.BytesIO):
    def __enter__(self):
        return self

    def __exit__(self, *exc):
        return False


def _vor(monkeypatch, topic):
    """Stand in for vor: /api/v1/source reads one topic from it."""
    import urllib.request
    monkeypatch.setattr(zhen_app, 'rag', types.SimpleNamespace(vor_url='http://vor.test'))
    asked = []

    def fake(url, timeout=None):
        asked.append(url)
        return _VorReply(json.dumps(topic).encode())

    monkeypatch.setattr(urllib.request, 'urlopen', fake)
    return asked


def test_source_view_page_is_constant_whatever_is_asked(client):
    _log_in(client)
    page = (RAFT / 'pages' / 'source-view.html').read_bytes()
    for q in ('', '?id=x', '?id="><script>alert(1)</script>', '?path=</title><svg onload=alert(1)>'):
        r = client.get('/view/source' + q, base_url=ORIGIN)
        assert r.status_code == 200
        assert r.data == page


def test_source_view_page_has_no_inline_script_or_style():
    import re
    page = (RAFT / 'pages' / 'source-view.html').read_text()
    assert re.findall(r'<script\b[^>]*>', page) == ['<script src="/static/source-view.js">']
    assert '<style' not in page
    assert not re.search(r'\s(?:style|on[a-z]+)\s*=', page)


def test_source_view_policy_allows_no_inline_code_and_no_images(client):
    _log_in(client)
    r = client.get('/view/source?id=x', base_url=ORIGIN)
    csp = dict(part.strip().split(' ', 1) for part in r.headers['Content-Security-Policy'].split(';'))
    assert csp['default-src'] == "'none'"
    assert csp['script-src'] == "'self'"
    assert csp['img-src'] == "'none'"
    assert csp['connect-src'] == "'self'"
    assert csp['frame-ancestors'] == "'none'" and csp['base-uri'] == "'none'" and csp['form-action'] == "'none'"
    assert 'unsafe' not in r.headers['Content-Security-Policy']
    assert '*' not in r.headers['Content-Security-Policy']
    assert r.headers['X-Content-Type-Options'] == 'nosniff'
    assert r.headers['Referrer-Policy'] == 'no-referrer'
    assert r.headers['X-Frame-Options'] == 'DENY'


def test_source_view_page_is_not_served_without_its_headers(client):
    _log_in(client)
    assert client.get('/static/source-view.html', base_url=ORIGIN).status_code == 404
    assert client.get('/static/../pages/source-view.html', base_url=ORIGIN).status_code == 404
    for name in ('source-view.js', 'source-view.css'):
        assert client.get('/static/' + name, base_url=ORIGIN).status_code == 200


_SHEET = '# Top\n\n<script>alert(1)</script> [x](javascript:alert(1))\n\n```sh\necho "hi"\n```\n'


def test_source_api_returns_the_sheet_rendered_and_raw(client, monkeypatch):
    asked = _vor(monkeypatch, {'name': 'a/b c', 'content': _SHEET, 'title': 'T', 'category': 'net',
                               'source_trust': 'canonical', 'source_kind': 'user-source',
                               'source_path': '/p/t.md'})
    _log_in(client)
    r = client.get('/api/v1/source?id=a/b c', base_url=ORIGIN)
    assert r.status_code == 200
    assert asked == ['http://vor.test/api/topics/a%2Fb%20c']
    body = r.get_json()
    assert body['content'] == _SHEET
    assert body['trust'] == 'canonical' and body['title'] == 'T' and body['category'] == 'net'
    assert body['outline'] == [{'level': 1, 'id': 'md-top', 'text': 'Top'}]
    assert '<script' not in body['html'] and '&lt;script&gt;' in body['html']
    assert 'href="javascript' not in body['html']
    assert 'class="hl-' in body['html']


def test_source_api_shows_an_external_sheets_code_plain(client, monkeypatch):
    """D-06: Pygments is not run on text from an `external` source. The block
    is escaped and shown, with no highlighter spans; a lexer is never built."""
    _vor(monkeypatch, {'name': 'ext', 'content': _SHEET, 'source_trust': 'external'})

    def no_lexer(*a, **kw):
        raise AssertionError('the highlighter was reached for an external sheet')

    monkeypatch.setattr(zhen_app.zhen_markdown.zhen_highlight, 'lexer_for', no_lexer)
    monkeypatch.setattr(zhen_app.zhen_markdown.zhen_highlight, 'highlight_one', no_lexer)
    _log_in(client)
    r = client.get('/api/v1/source?id=ext', base_url=ORIGIN)
    assert r.status_code == 200
    body = r.get_json()
    assert body['trust'] == 'external'
    assert 'class="hl-' not in body['html']
    assert 'echo &quot;hi&quot;' in body['html']
    assert '<script' not in body['html'] and '&lt;script&gt;' in body['html']


def test_the_same_text_is_not_served_highlighted_from_the_cache(client, monkeypatch):
    """The renderer caches by content: a canonical sheet rendered first must
    not hand its highlighted html to an external sheet with the same text."""
    _log_in(client)
    _vor(monkeypatch, {'name': 'c', 'content': _SHEET, 'source_trust': 'canonical'})
    assert 'class="hl-' in client.get('/api/v1/source?id=c', base_url=ORIGIN).get_json()['html']
    _vor(monkeypatch, {'name': 'e', 'content': _SHEET, 'source_trust': 'external'})
    assert 'class="hl-' not in client.get('/api/v1/source?id=e', base_url=ORIGIN).get_json()['html']


@pytest.mark.parametrize('topic', [
    {'name': 'wotan', 'content': 'the nearest sheet'},   # vor answered with another sheet
    {'content': 'a sheet with no name'},
    {'name': '', 'content': 'x'},
    {'name': ['x'], 'content': 'x'},
    ['not', 'a', 'topic'],
])
def test_source_api_404s_an_id_vor_did_not_answer_exactly(client, monkeypatch, topic):
    """D-06: vor answers an unknown id with the nearest sheet. The viewer shows
    a sheet only when it carries the name that was asked for."""
    _vor(monkeypatch, topic)
    _log_in(client)
    r = client.get('/api/v1/source?id=x', base_url=ORIGIN)
    assert r.status_code == 404
    assert r.get_json() == {'error': 'no such topic'}


def test_source_api_by_path_wants_the_name_the_search_hit_named(client, monkeypatch):
    import urllib.request
    monkeypatch.setattr(zhen_app, 'rag', types.SimpleNamespace(vor_url='http://vor.test'))
    topic = {'name': 'other', 'content': 'the nearest sheet', 'source_path': '/p/t.md'}

    def fake(url, timeout=None):
        if '/api/search' in url:
            return _VorReply(json.dumps([{'topic': 'asked', 'source_path': '/p/t.md'}]).encode())
        return _VorReply(json.dumps(topic).encode())

    monkeypatch.setattr(urllib.request, 'urlopen', fake)
    _log_in(client)
    r = client.get('/api/v1/source?path=/p/t.md', base_url=ORIGIN)
    assert r.status_code == 404
    assert r.get_json() == {'error': 'no such topic'}
    topic['name'] = 'asked'
    r = client.get('/api/v1/source?path=/p/t.md', base_url=ORIGIN)
    assert r.status_code == 200
    assert r.get_json()['id'] == 'asked'


def test_source_api_falls_back_to_raw_when_the_renderer_fails(client, monkeypatch):
    _vor(monkeypatch, {'name': 'x', 'content': 'plain words'})

    def boom(source, **kw):
        raise RuntimeError('renderer bug')

    monkeypatch.setattr(zhen_app.zhen_markdown, 'render', boom)
    _log_in(client)
    body = client.get('/api/v1/source?id=x', base_url=ORIGIN).get_json()
    assert body['html'] is None and body['content'] == 'plain words'
    assert body['notes'] == ['This sheet could not be rendered. The raw text is shown.']


def test_source_api_treats_non_text_content_as_empty(client, monkeypatch):
    _vor(monkeypatch, {'name': 'x', 'content': {'not': 'text'}})
    _log_in(client)
    body = client.get('/api/v1/source?id=x', base_url=ORIGIN).get_json()
    assert body['content'] == '' and body['html'] == ''


def test_source_routes_need_a_login(client, monkeypatch):
    _vor(monkeypatch, {'content': 'x'})
    assert client.get('/view/source?id=x', base_url=ORIGIN).status_code == 401
    assert client.get('/api/v1/source?id=x', base_url=ORIGIN).status_code == 401


def test_highlight_css_covers_answers_and_the_viewer(client):
    css = client.get('/api/v1/highlight.css', base_url=ORIGIN).get_data(as_text=True)
    assert '.msg.zhen pre code .hl-k' in css
    assert '.md-body pre code .hl-k' in css


# ── the route list (D-01): what is open before login cannot drift ────────

ROUTES_GOLDEN = Path(__file__).with_name('zhen_routes.golden')
_OPEN_BEFORE_LOGIN = {
    'GET /', 'GET /health', 'POST /auth/login', 'POST /auth/logout', 'GET /auth/me',
    'GET /api/v1/highlight.css',
}


def _route_lines():
    lines = set()
    for rule in zhen_app.app.url_map.iter_rules():
        if rule.endpoint == 'static':
            continue
        for method in sorted(rule.methods - {'HEAD', 'OPTIONS'}):
            lines.add(f'{method} {rule.rule}')
    return sorted(lines)


def _golden_lines():
    return [ln for ln in ROUTES_GOLDEN.read_text().splitlines() if ln.strip()]


def test_route_list_is_current():
    """zhen_routes.golden is what scripts/zhen-lan-proof.sh walks on the live
    front. A route added to the app and not to the file fails here, so the
    live proof cannot test a stale list. ZHEN_UPDATE_GOLDEN=1 rewrites it."""
    import os
    got = _route_lines()
    if os.environ.get('ZHEN_UPDATE_GOLDEN') == '1':
        ROUTES_GOLDEN.write_text('\n'.join(got) + '\n')
    assert ROUTES_GOLDEN.exists(), 'run once with ZHEN_UPDATE_GOLDEN=1'
    assert _golden_lines() == got


def test_the_open_list_is_the_apps_open_list():
    assert {ln.split(' ', 1)[1] for ln in _OPEN_BEFORE_LOGIN} == set(zhen_app._LOGIN_OPEN)
    assert _OPEN_BEFORE_LOGIN <= set(_golden_lines())


class _Untouchable:
    """Stands in for the RAG pipeline: any use of it is a handler that ran."""

    def __getattr__(self, name):
        raise AssertionError(f'a handler reached rag.{name} with no login')


def _concrete(path):
    import re
    return re.sub(r'<int:[^>]+>', '1', re.sub(r'<(?!int:)[^>]+>', 'x', path))


def test_every_route_but_the_open_ones_refuses_a_stranger(client, monkeypatch):
    """Three strangers: a script with no headers, a browser on this origin with
    the CSRF cookie and no session, and a page on another origin. Each gets
    401 or 403 on every route that is not open, and no handler runs."""
    ran = []
    monkeypatch.setattr(zhen_app, 'rag', _Untouchable())
    closed = [ln for ln in _golden_lines() if ln not in _OPEN_BEFORE_LOGIN]
    assert len(closed) >= 30
    for rule in zhen_app.app.url_map.iter_rules():
        if rule.endpoint == 'static' or rule.rule in zhen_app._LOGIN_OPEN:
            continue
        monkeypatch.setitem(zhen_app.app.view_functions, rule.endpoint,
                            lambda *a, _ep=rule.endpoint, **kw: ran.append(_ep) or ('ran', 200))
    same_origin = _browser(client)
    strangers = [
        ('no headers', {}),
        ('same origin, no session', same_origin),
        ('cross site', {'Origin': 'http://evil.test', 'Sec-Fetch-Site': 'cross-site'}),
    ]
    for line in closed:
        method, path = line.split(' ', 1)
        for who, headers in strangers:
            kwargs = {'base_url': ORIGIN, 'headers': headers}
            if method != 'GET':
                kwargs['json'] = {}
            r = client.open(_concrete(path), method=method, **kwargs)
            assert r.status_code in (401, 403), f'{line} ({who}) answered {r.status_code}'
    assert not ran
