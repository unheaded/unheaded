# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.

"""The web UI's login (F4, H5): the issuer logs a person in, zhen-agentd says
who they are, and what the browser is given it can redeem. Nothing here
listens: the issuer and the daemon are faked at urllib's edge."""

import base64
import http.client
import io
import json
import re
import shutil
import socket
import sys
import tempfile
import threading
import time
import types
import urllib.error
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
import zhen_socket

LOCAL = 'http://127.0.0.1:20103'
SECRET = 'correct horse battery staple'  # nosec B105 - fixture password for a fake issuer


def _token(life=900):
    now = int(time.time())
    claims = base64.urlsafe_b64encode(json.dumps({'exp': now + life, 'iat': now}).encode()).rstrip(b'=').decode()
    return f'hdr.{claims}.sig'


class _Reply:
    def __init__(self, body, status=200):
        self.status, self._b = status, json.dumps(body).encode()

    def read(self, *_):
        return self._b

    def __enter__(self):
        return self

    def __exit__(self, *_):
        return False


@pytest.fixture
def world(monkeypatch):
    """A fake issuer and daemon; `calls` records what reached them."""
    w = types.SimpleNamespace(calls=[], users={'stevie': (SECRET, ['operator']), 'watcher': (SECRET, ['viewer'])},
                              tokens={}, whoami_code=200, daemon=lambda req: _Reply({'status': 'ok'}))

    def post_issuer(url, username, password):
        w.calls.append(('issuer', username))
        user = w.users.get(username)
        if user is None or user[0] != password:
            return 401, {}, ''
        tok = _token()
        w.tokens[tok] = username
        return 200, {'access_token': tok, 'token_type': 'Bearer'}, ''  # nosec B105 - token type name, no credential

    def whoami(url, token):
        w.calls.append(('whoami', token))
        if w.whoami_code != 200:
            return w.whoami_code, {}
        name = w.tokens[token]
        return 200, {'subject': name, 'roles': w.users[name][1], 'principal': 'ui:' + name}

    def urlopen(req, timeout=None):
        w.calls.append(('daemon', req.full_url, dict(req.header_items()), req.data))
        return w.daemon(req)

    monkeypatch.setattr(zhen_login, '_post_issuer', post_issuer)
    monkeypatch.setattr(zhen_login, '_get_whoami', whoami)
    import urllib.request as ur
    monkeypatch.setattr(ur, 'urlopen', urlopen)
    fresh = zhen_login.Login('https://issuer.test', 'http://127.0.0.1:20111')
    monkeypatch.setattr(zhen_app, 'LOGIN', fresh)
    w.login = fresh
    return w


@pytest.fixture
def client(monkeypatch):
    zhen_app.app.config['TESTING'] = True
    with zhen_app.app.test_client() as c:
        yield c


def _page_headers(c):
    r = c.get('/', base_url=LOCAL)
    tok = [h for h in r.headers.getlist('Set-Cookie') if h.startswith('zhen_csrf=')]
    if tok:
        c._csrf = tok[0].split(';', 1)[0].split('=', 1)[1]
    return {'Origin': LOCAL, 'Sec-Fetch-Site': 'same-origin', 'X-Zhen-CSRF': c._csrf}


def _login(c, user='stevie', password=SECRET):
    return c.post('/auth/login', base_url=LOCAL, json={'username': user, 'password': password},
                  headers=_page_headers(c))


# ── what a login gives the browser ──────────────────────────────────────

def test_login_gives_an_httponly_strict_cookie_and_no_token(client, world):
    r = _login(client)
    assert r.status_code == 200
    assert r.get_json()['sub'] == 'stevie'
    assert 'access_token' not in r.get_data(as_text=True)
    cookie = [h for h in r.headers.getlist('Set-Cookie') if h.startswith(zhen_login.COOKIE_NAME + '=')]
    assert cookie, r.headers.getlist('Set-Cookie')
    assert 'HttpOnly' in cookie[0] and 'SameSite=Strict' in cookie[0] and 'Path=/' in cookie[0]
    assert next(iter(world.tokens)) not in cookie[0]


def test_login_over_https_uses_the_host_prefixed_secure_cookie(client, world, monkeypatch):
    monkeypatch.setenv('ZHEN_ALLOWED_HOSTS', 'zhen.test')
    origin = 'https://zhen.test'
    r = client.get('/', base_url=origin)
    tok = next(h for h in r.headers.getlist('Set-Cookie') if h.startswith('zhen_csrf=')).split(';', 1)[0].split('=', 1)[1]
    r = client.post('/auth/login', base_url=origin, json={'username': 'stevie', 'password': SECRET},
                    headers={'Origin': origin, 'Sec-Fetch-Site': 'same-origin', 'X-Zhen-CSRF': tok})
    assert r.status_code == 200
    cookie = [h for h in r.headers.getlist('Set-Cookie') if h.startswith(zhen_login.SECURE_COOKIE_NAME + '=')]
    assert cookie and 'Secure' in cookie[0]


def test_login_off_loopback_over_plain_http_is_refused(client, world, monkeypatch):
    monkeypatch.setenv('ZHEN_ALLOWED_HOSTS', 'zhen.test')
    origin = 'http://zhen.test'
    r = client.get('/', base_url=origin)
    tok = next(h for h in r.headers.getlist('Set-Cookie') if h.startswith('zhen_csrf=')).split(';', 1)[0].split('=', 1)[1]
    r = client.post('/auth/login', base_url=origin, json={'username': 'stevie', 'password': SECRET},
                    headers={'Origin': origin, 'Sec-Fetch-Site': 'same-origin', 'X-Zhen-CSRF': tok})
    assert r.status_code == 403
    assert not [c for c in world.calls if c[0] == 'issuer']


def test_a_short_password_logs_in_and_the_person_hears_about_it(client, world, monkeypatch):
    def short(url, username, password):
        tok = _token()
        world.tokens[tok] = username
        return 200, {'access_token': tok, 'token_type': 'Bearer', 'password_below_floor': True}, ''  # nosec B105 - token type name and a flag, no credential
    monkeypatch.setattr(zhen_login, '_post_issuer', short)
    r = _login(client)
    assert r.status_code == 200 and r.get_json()['password_below_floor'] is True


def test_a_login_body_is_bounded(client, world):
    r = client.post('/auth/login', base_url=LOCAL, headers=_page_headers(client),
                    json={'username': 'stevie', 'password': 'x' * (zhen_login.MAX_LOGIN_BODY + 1)})
    assert r.status_code == 413
    assert not world.calls


def test_tls_on_with_no_ca_is_an_unreachable_issuer_not_a_crash(monkeypatch):
    monkeypatch.setenv('UNHEADED_TLS_ENABLED', 'true')
    monkeypatch.delenv('UNHEADED_TLS_CA_CERT', raising=False)
    assert zhen_login._post_issuer('https://127.0.0.1:1', 'u', 'p')[0] == 502


def test_a_wrong_password_is_401_and_makes_no_session(client, world):
    r = _login(client, password='wrong')  # nosec B106 - the wrong password under test
    assert r.status_code == 401
    assert not [h for h in r.headers.getlist('Set-Cookie') if h.startswith(zhen_login.COOKIE_NAME)]
    assert client.get('/auth/me', base_url=LOCAL).status_code == 401


def test_a_failed_login_is_counted_before_the_next_reaches_the_issuer(client, world):
    for _ in range(zhen_login.CLIENT_USER_FAILURES):
        assert _login(client, password='wrong').status_code == 401  # nosec B106 - the wrong password under test
    r = _login(client)
    assert r.status_code == 429
    assert int(r.headers['Retry-After']) > 0
    assert len([c for c in world.calls if c[0] == 'issuer']) == zhen_login.CLIENT_USER_FAILURES


def test_a_token_the_daemon_will_not_name_makes_no_session(client, world):
    world.whoami_code = 401
    r = _login(client)
    assert r.status_code == 502
    assert client.get('/auth/me', base_url=LOCAL).status_code == 401


def test_no_issuer_means_no_login_and_nothing_opens(client, world, monkeypatch):
    monkeypatch.setattr(zhen_app, 'LOGIN', zhen_login.Login('', 'http://127.0.0.1:20111'))
    assert _login(client).status_code == 503
    assert client.get('/api/v1/conversations', base_url=LOCAL).status_code == 401


def test_a_token_is_never_sent_to_a_plain_http_daemon_off_loopback():
    login = zhen_login.Login('https://issuer.test', 'http://10.0.0.9:20111')
    assert 'ZHEN_AGENTD_URL' in login.config_error()
    assert zhen_login.Login('http://10.0.0.9:19020', 'http://127.0.0.1:20111').config_error()
    assert zhen_login.Login('http://10.0.0.9:19020', 'http://10.0.0.9:20111', dev_plaintext=True).config_error() is None


def test_login_fixation_a_planted_cookie_is_dropped(client, world):
    _login(client)
    old = client.get_cookie(zhen_login.COOKIE_NAME, domain='127.0.0.1').value
    _login(client)
    assert client.get_cookie(zhen_login.COOKIE_NAME, domain='127.0.0.1').value != old
    assert world.login.sessions.get(old) is None


def test_logout_ends_the_session(client, world):
    _login(client)
    assert client.get('/auth/me', base_url=LOCAL).status_code == 200
    assert client.post('/auth/logout', base_url=LOCAL, headers=_page_headers(client)).status_code == 204
    assert client.get('/auth/me', base_url=LOCAL).status_code == 401


def test_a_session_ends_with_its_token():
    clock = [1000.0]
    sessions = zhen_login.Sessions(now=lambda: clock[0])
    sid = sessions.put(zhen_login.Session('t', 's', (), 1900.0))
    assert sessions.get(sid)
    clock[0] = 1900.0
    assert sessions.get(sid) is None


def test_the_session_table_is_bounded():
    sessions = zhen_login.Sessions(now=lambda: 0.0, limit=1)
    assert sessions.put(zhen_login.Session('t', 's', (), 10.0))
    assert sessions.put(zhen_login.Session('t', 's', (), 10.0)) is None


def test_a_session_id_is_not_stored_in_the_clear():
    sessions = zhen_login.Sessions()
    sid = sessions.put(zhen_login.Session('t', 's', (), time.time() + 60))
    assert sid not in sessions._by_key


def test_a_token_with_no_claims_has_no_life():
    assert zhen_login.token_life('not-a-jwt', 0) is None
    assert zhen_login.token_life('a.b.c', 0) is None


def test_a_long_token_is_held_to_the_cap():
    assert zhen_login.token_life(_token(life=86400), 0) == zhen_login.MAX_TOKEN_LIFE


def _go_const(path, name):
    src = (Path(__file__).resolve().parents[2] / path).read_text(encoding='utf-8')
    m = re.search(rf'^\s*{name}\s*=\s*(\d+)', src, re.MULTILINE)
    assert m, f'{name} not found in {path}'
    return int(m.group(1))


def test_budget_sums_stay_under_the_issuers_limits():
    # The dashboard, the board and this UI reach the issuer from one address.
    # Summed, one client's budgets stay strictly under the issuer's locks, so
    # one client cannot lock a username on every UI (Stevie, 2026-11-09).
    backends = _go_const('pkg/bff/failures.go', 'IssuerAddressBackends')
    per_user = _go_const('pkg/bff/failures.go', 'ClientUserLoginFailures')
    per_client = _go_const('pkg/bff/failures.go', 'ClientLoginFailures')
    pair = _go_const('cmd/unheaded-auth/lockout.go', 'maxFailuresPerPair')
    source = _go_const('cmd/unheaded-auth/lockout.go', 'maxFailuresPerSource')
    assert backends * per_user + zhen_login.CLIENT_USER_FAILURES < pair
    assert backends * per_client + zhen_login.CLIENT_FAILURES < source
    assert zhen_login.CLIENT_USER_FAILURES == 5
    assert zhen_login.FAILURE_WINDOW >= 600


# ── what a caller without a login gets ──────────────────────────────────

@pytest.mark.parametrize('path', [
    '/api/v1/conversations', '/api/v1/conversations/search?q=x', '/api/v1/audit/recent', '/api/v1/kanban/summary',
    '/api/v1/runbooks', '/api/v1/runbooks/recent', '/api/v1/runbooks/x', '/api/v1/models', '/api/v1/stats',
    '/api/v1/system/state', '/api/v1/approvals', '/api/v1/source?topic=x', '/api/v1/skills', '/api/v1/trust',
    '/view/source', '/api/v1/no-such-route',
])
def test_every_read_needs_a_login(client, world, path):
    r = client.get(path, base_url=LOCAL)
    assert r.status_code == 401
    assert r.get_json()['error'] == 'login_required'


@pytest.mark.parametrize('path', [
    '/api/v1/query', '/api/v1/search', '/api/v1/context', '/api/v1/remember', '/api/v1/forget', '/api/v1/teach',
    '/api/v1/generate-file', '/api/v1/champion/read', '/api/v1/models/switch', '/api/v1/kanban/create',
    '/api/v1/runbooks/x/execute', '/api/v1/agent/confirm', '/api/v1/approvals', '/api/v1/approvals/1/approve',
    '/api/v1/approvals/1/deny',
])
def test_every_write_needs_a_login_and_reaches_nothing(client, world, path):
    r = client.post(path, base_url=LOCAL, json={'question': 'emergency stop', 'task': {'id': 'x'}, 'key': 'k', 'token': 't'},  # nosec B105 - fixture token, no credential
                    headers=_page_headers(client))
    assert r.status_code == 401
    assert not world.calls


def test_chat_from_a_script_needs_a_login_too(client, world):
    r = client.post('/api/v1/query', base_url=LOCAL, json={'question': 'emergency stop'})
    assert r.status_code == 401
    assert not world.calls


@pytest.mark.parametrize('path', ['/', '/health', '/api/v1/highlight.css'])
def test_the_page_and_the_probe_stay_open(client, world, path):
    assert client.get(path, base_url=LOCAL).status_code == 200


def test_login_still_passes_the_csrf_guard(client, world):
    r = client.post('/auth/login', base_url=LOCAL, json={'username': 'stevie', 'password': SECRET},
                    headers={'Origin': 'http://evil.test', 'Sec-Fetch-Site': 'cross-site'})
    assert r.status_code == 403
    assert not world.calls


def test_a_forged_cookie_is_no_session(client, world):
    client.set_cookie(zhen_login.COOKIE_NAME, 'A' * 43, domain='127.0.0.1')
    assert client.get('/api/v1/runbooks', base_url=LOCAL).status_code == 401


# ── what a login can do ─────────────────────────────────────────────────

def _bearer(call):
    return {k.lower(): v for k, v in call[2].items()}.get('authorization', '')


def test_a_viewer_reads_but_does_not_write(client, world):
    assert _login(client, 'watcher').status_code == 200
    assert client.get('/auth/me', base_url=LOCAL).get_json()['roles'] == ['viewer']
    r = client.post('/api/v1/kanban/create', base_url=LOCAL, json={'task': {'id': 'x'}}, headers=_page_headers(client))
    assert r.status_code == 403
    assert not [c for c in world.calls if c[0] == 'daemon']


def test_the_daemon_sees_the_person_not_the_ui(client, world):
    _login(client)
    world.daemon = lambda req: _Reply({'status': 'ok'})
    r = client.post('/api/v1/kanban/create', base_url=LOCAL, json={'task': {'id': 'x'}}, headers=_page_headers(client))
    assert r.status_code == 200
    call = next(c for c in world.calls if c[0] == 'daemon')
    assert _bearer(call) == 'Bearer ' + next(iter(world.tokens))
    assert 'x-api-key' not in {k.lower() for k in call[2]}


def test_the_ui_holds_no_daemon_key(monkeypatch):
    # A key left in the environment is not picked up, and -gated exports none.
    monkeypatch.setenv('ZHEN_AGENTD_API_KEY', 'k-ui')
    assert zhen_app._agentd_headers() == {'Content-Type': 'application/json'}
    assert not hasattr(zhen_app, 'AGENTD_API_KEY')
    assert 'ZHEN_AGENTD_API_KEY' not in (RAFT / 'start-zhen.sh').read_text()


class _UnixConnection(http.client.HTTPConnection):
    def __init__(self, path):
        super().__init__('localhost', timeout=10)
        self._path = path

    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.settimeout(self.timeout)
        self.sock.connect(self._path)


@pytest.fixture
def ui_socket(world, monkeypatch):
    """The UI on a real Unix socket, as zhen-ui-front reaches it (ADR-138)."""
    monkeypatch.setenv('ZHEN_ALLOWED_HOSTS', 'zhen.test')
    d = Path(tempfile.mkdtemp(prefix='zl'))
    path = str(d / 'app.sock')
    server = zhen_socket.make(zhen_app.app, path)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    yield path
    server.shutdown()
    server.server_close()
    shutil.rmtree(d, ignore_errors=True)


def _socket_login(path, client, password=SECRET, host='zhen.test'):
    conn = _UnixConnection(path)
    try:
        conn.request('POST', '/auth/login', body=json.dumps({'username': 'stevie', 'password': password}),
                     headers={'Host': host, 'Content-Type': 'application/json', 'X-Unheaded-Client-Addr': client})
        r = conn.getresponse()
        r.read()
        return r
    finally:
        conn.close()


# The app has no TLS of its own. A login from another host arrives on the
# socket, through the front, under a Host that is not loopback: the password
# is taken and the session cookie is Secure with the __Host- prefix.
def test_a_login_past_loopback_works_through_the_socket(ui_socket):
    r = _socket_login(ui_socket, '203.0.113.9')
    assert r.status == 200
    cookies = r.headers.get_all('Set-Cookie')
    session = [c for c in cookies if c.startswith(zhen_login.SECURE_COOKIE_NAME + '=')]
    assert session and 'Secure' in session[0] and 'HttpOnly' in session[0] and 'SameSite=Strict' in session[0]
    assert not [c for c in cookies if c.startswith(zhen_login.COOKIE_NAME + '=')]


# The budget is per client address, and behind the front that address is the
# front's header, never the socket's peer: five wrong passwords from one
# address lock that address and nobody else.
def test_one_address_cannot_spend_anothers_login_budget(ui_socket, world):
    for _ in range(zhen_login.CLIENT_FAILURES):
        assert _socket_login(ui_socket, '203.0.113.5', password='wrong').status == 401  # nosec B106 - the wrong password under test
    locked = _socket_login(ui_socket, '203.0.113.5')
    assert locked.status == 429 and int(locked.headers['Retry-After']) > 0
    assert _socket_login(ui_socket, '203.0.113.6').status == 200
    assert _socket_login(ui_socket, '127.0.0.1').status == 200


# Served any other way, with no front to make it https, the app still refuses
# a password that would cross the network in the clear.
def test_without_the_front_a_login_past_loopback_is_refused(client, world, monkeypatch):
    monkeypatch.setenv('ZHEN_ALLOWED_HOSTS', 'zhen.test')
    r = client.post('/auth/login', base_url='http://zhen.test', json={'username': 'stevie', 'password': SECRET})
    assert r.status_code == 403
    assert r.get_json() == {'error': 'login_needs_https'}
    assert not [c for c in world.calls if c[0] == 'issuer']


def test_tls_settings_in_the_environment_open_no_listener():
    # The pair is the front's. The app has nothing left that reads it.
    src = (RAFT / 'zhen_app.py').read_text() + (RAFT / 'zhen_socket.py').read_text()
    assert 'UNHEADED_TLS_CERT' not in src and 'UNHEADED_TLS_KEY_FILE' not in src
    assert not hasattr(zhen_app, 'tls_context')


def test_the_token_does_not_outlive_the_request():
    assert zhen_rag.agentd_headers('k').get('X-API-Key') == 'k'
    zhen_rag.set_agentd_bearer('t')
    try:
        assert zhen_rag.agentd_headers('k') == {'Content-Type': 'application/json', 'Authorization': 'Bearer t'}
    finally:
        zhen_rag.set_agentd_bearer(None)
    assert 'Authorization' not in zhen_rag.agentd_headers('k')


def test_an_approval_names_who_decided(client, world):
    _login(client)
    h = _page_headers(client)
    made = client.post('/api/v1/approvals', base_url=LOCAL, json={'operation': 'o'}, headers=h).get_json()
    r = client.post(f"/api/v1/approvals/{made['id']}/approve", base_url=LOCAL, headers=h)
    assert r.get_json()['decided_by'] == 'stevie'


# ── what the browser is given, it can redeem ────────────────────────────

class _Held(urllib.error.HTTPError):
    def __init__(self, body, code=403):
        super().__init__('http://daemon/', code, 'held', {}, io.BytesIO(json.dumps(body).encode()))


def _held(req):
    if req.full_url.endswith('/api/v1/tool/exec'):
        raise _Held({'status': 'pending_confirmation', 'pending_token': 'pend-1', 'reason': 'held'})  # nosec B105 - fixture pending token
    return _Reply({'status': 'ok', 'result': {'ran': True}})


@pytest.fixture
def runbook(tmp_path, monkeypatch):
    root = tmp_path / 'runbooks'
    (root / 'observe').mkdir(parents=True)
    (root / 'observe' / 'sweep.yaml').write_text('metadata:\n  name: sweep\n  risk: low\n')
    monkeypatch.setattr(zhen_app, 'RUNBOOKS_DIR', str(root))
    return 'sweep'


def _redeem(client, token='pend-1'):  # nosec B107 - fixture pending token
    return client.post('/api/v1/agent/confirm', base_url=LOCAL, json={'token': token}, headers=_page_headers(client))


def test_a_held_runbook_can_be_redeemed_from_the_page(client, world, runbook):
    _login(client)
    world.daemon = _held
    r = client.post(f'/api/v1/runbooks/{runbook}/execute', base_url=LOCAL, json={}, headers=_page_headers(client))
    assert r.status_code == 403
    body = r.get_json()
    assert body['status'] == 'pending_confirmation' and body['pending_token'] == 'pend-1'
    assert body['confirm_endpoint'] == '/api/v1/agent/confirm'  # this origin, not the daemon

    r = _redeem(client)
    assert r.status_code == 200 and r.get_json()['result'] == {'ran': True}
    asked, redeemed = [c for c in world.calls if c[0] == 'daemon']
    assert redeemed[1].endswith('/api/v1/agent/confirm')
    assert json.loads(redeemed[3]) == {'token': 'pend-1'}  # nosec B105 - fixture pending token
    assert _bearer(asked) == _bearer(redeemed) == 'Bearer ' + next(iter(world.tokens))


def test_a_held_model_switch_can_be_redeemed_from_the_page(client, world, monkeypatch):
    monkeypatch.setattr(zhen_app, '_load_model_keys', lambda: ['qwen-coder-7b'])
    _login(client)
    world.daemon = _held
    r = client.post('/api/v1/models/switch', base_url=LOCAL, json={'key': 'qwen-coder-7b'}, headers=_page_headers(client))
    assert r.status_code == 403
    assert r.get_json()['confirm_endpoint'] == '/api/v1/agent/confirm'
    assert _redeem(client).status_code == 200


def test_the_daemons_refusal_of_a_redeem_reaches_the_page(client, world):
    _login(client)

    def refuse(req):
        raise _Held({'status': 'denied', 'reason': 'confirmation refused'})
    world.daemon = refuse
    r = _redeem(client)
    assert r.status_code == 403 and r.get_json()['status'] == 'denied'


def test_a_redeem_needs_a_token(client, world):
    _login(client)
    assert client.post('/api/v1/agent/confirm', base_url=LOCAL, json={}, headers=_page_headers(client)).status_code == 400
    assert not [c for c in world.calls if c[0] == 'daemon']


def test_a_viewer_cannot_redeem(client, world):
    _login(client, 'watcher')
    assert _redeem(client).status_code == 403
    assert not [c for c in world.calls if c[0] == 'daemon']


def test_the_page_offers_login_and_redeems_through_this_origin():
    page = (RAFT / 'static' / 'index.html').read_text()
    assert "'/auth/login'" in page
    assert "'/auth/logout'" in page
    assert "'/api/v1/agent/confirm'" in page
    assert ':20111' not in page
