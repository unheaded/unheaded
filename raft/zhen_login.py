# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Login for the Zhen web UI: pkg/bff's contract, in Python (F4, H5).

The browser never holds a bearer token. POST /auth/login sends the username
and password to the issuer (unheaded-auth, AUTH_ISSUER_URL) asking for a token
whose audience is zhen-agentd, keeps that token here, and gives the browser a
random session id in an HttpOnly, SameSite=Strict cookie.

This process verifies no signature. zhen-agentd does, with pkg/auth: login asks
its /api/v1/whoami who the token belongs to, and every later call into the
daemon carries the token, so the daemon judges each one again and a pending
confirmation binds to the person, not to this UI.

A session ends with its token. Sessions live in memory: a restart logs
everyone out.
"""
import base64
import hashlib
import ipaddress
import json
import logging
import os
import secrets
import ssl
import threading
import time
import urllib.error
import urllib.request
from collections import namedtuple
from urllib.parse import urlsplit

log = logging.getLogger('zhen.login')

# The audience a login asks for; unheaded-auth -audiences must list it.
AUDIENCE = 'zhen-agentd'
COOKIE_NAME = 'unheaded_session_zhen_web_ui'
SECURE_COOKIE_NAME = '__Host-' + COOKIE_NAME

MAX_SESSIONS = 1000
MAX_LOGIN_BODY = 4 << 10
MAX_REPLY = 16 << 10
MAX_TOKEN_LIFE = 3600  # pkg/auth's cap on a user token (AUTH_OIDC_MAX_LIFETIME)
ISSUER_TIMEOUT = 15  # argon2id is slow on purpose
SESSION_ID_BYTES = 32

# The issuer's lockout sees only this backend's address. The dashboard and the
# board each spend 2 and 9 of its 5 and 20 from that address (pkg/bff), so this
# third backend gets what is left of the 20: one failure per client, which brings
# the per-username sum to the issuer's limit with no margin.
FAILURE_WINDOW = 600
CLIENT_USER_FAILURES = 1
CLIENT_FAILURES = 1
MAX_TRACKED_KEYS = 10000

OPERATOR_ROLES = frozenset({'admin', 'operator'})

Session = namedtuple('Session', 'token subject roles expires')


def _loopback_host(host):
    if host == 'localhost':
        return True
    try:
        return ipaddress.ip_address(host).is_loopback
    except ValueError:
        return False


def tls_context():
    """The deployment CA when UNHEADED_TLS_ENABLED, TLS 1.3 at least."""
    ctx = ssl.create_default_context()
    ctx.minimum_version = ssl.TLSVersion.TLSv1_3
    if os.environ.get('UNHEADED_TLS_ENABLED', '') == 'true':
        cafile = os.environ.get('UNHEADED_TLS_CA_CERT', '').strip()
        if not cafile:
            raise OSError('UNHEADED_TLS_ENABLED=true but UNHEADED_TLS_CA_CERT is not set')
        ctx.load_verify_locations(cafile=cafile)
    return ctx


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    """The login carries a password: never follow a redirect with it."""

    def redirect_request(self, *args, **kwargs):
        return None


def _open(req, timeout):
    opener = urllib.request.build_opener(
        _NoRedirect, urllib.request.HTTPSHandler(context=tls_context()))
    return opener.open(req, timeout=timeout)


def _post_issuer(issuer_url, username, password):
    """(http status, parsed body, Retry-After)."""
    body = json.dumps({'username': username, 'password': password, 'audience': AUDIENCE}).encode('utf-8')
    req = urllib.request.Request(f'{issuer_url}/login', data=body, method='POST',
                                 headers={'Content-Type': 'application/json'})
    try:
        with _open(req, ISSUER_TIMEOUT) as resp:
            return resp.status, json.loads(resp.read(MAX_REPLY)), ''
    except urllib.error.HTTPError as e:
        return e.code, {}, e.headers.get('Retry-After', '') if e.headers else ''
    except (OSError, ValueError) as e:
        log.warning('issuer unreachable at %s: %s', issuer_url, type(e).__name__)
        return 502, {}, ''


def _get_whoami(agentd_url, token):
    """(http status, parsed body) of zhen-agentd's /api/v1/whoami."""
    req = urllib.request.Request(f'{agentd_url}/api/v1/whoami', method='GET',
                                 headers={'Authorization': f'Bearer {token}'})
    try:
        with _open(req, ISSUER_TIMEOUT) as resp:
            return resp.status, json.loads(resp.read(MAX_REPLY))
    except urllib.error.HTTPError as e:
        return e.code, {}
    except (OSError, ValueError) as e:
        log.warning('zhen-agentd unreachable at %s: %s', agentd_url, type(e).__name__)
        return 502, {}


def token_life(token, now):
    """Seconds this session may last: exp - iat of the token, so this host's
    clock and the issuer's need not agree. None for a token with no usable
    claims. Its signature is zhen-agentd's to judge, not this function's."""
    parts = token.split('.')
    if len(parts) != 3:
        return None
    try:
        raw = base64.urlsafe_b64decode(parts[1] + '=' * (-len(parts[1]) % 4))
        claims = json.loads(raw)
        exp, iat = int(claims['exp']), int(claims.get('iat', 0))
    except (ValueError, TypeError, KeyError):
        return None
    life = exp - iat if iat > 0 and exp > iat else exp - now
    return min(life, MAX_TOKEN_LIFE) if life > 0 else None


class FailureBudget:
    """pkg/bff's loginFailures: an attempt is charged before the issuer answers,
    so concurrent attempts cannot outrun the count, and given back when the
    answer was no credential failure. Safe across threads."""

    def __init__(self, now=time.time):
        self._now = now
        self._lock = threading.Lock()
        self._fail = {}

    def _recent(self, key, now):
        kept = [t for t in self._fail.get(key, ()) if now - t < FAILURE_WINDOW]
        if kept:
            self._fail[key] = kept
        else:
            self._fail.pop(key, None)
        return kept

    def charge(self, client, user):
        """(True, 0) with the attempt charged, or (False, seconds to wait)."""
        with self._lock:
            now = self._now()
            if len(self._fail) >= MAX_TRACKED_KEYS:
                for key in list(self._fail):
                    self._recent(key, now)
                if len(self._fail) >= MAX_TRACKED_KEYS:
                    return False, 60
            budgets = ((f'u|{client}|{user}', CLIENT_USER_FAILURES), (f'c|{client}', CLIENT_FAILURES))
            wait = 0
            for key, limit in budgets:
                ts = self._recent(key, now)
                if len(ts) >= limit:
                    wait = max(wait, ts[len(ts) - limit] + FAILURE_WINDOW - now)
            if wait > 0:
                return False, wait
            for key, _ in budgets:
                self._fail.setdefault(key, []).append(now)
            return True, 0

    def refund(self, client, user):
        with self._lock:
            for key in (f'u|{client}|{user}', f'c|{client}'):
                ts = self._fail.get(key)
                if ts:
                    ts.pop()
                    if not ts:
                        del self._fail[key]


class Sessions:
    """Session id -> Session. Ids are stored hashed, so a dump of this table
    is no cookie. Safe across threads."""

    def __init__(self, now=time.time, limit=MAX_SESSIONS):
        self._now = now
        self._limit = limit
        self._lock = threading.Lock()
        self._by_key = {}

    @staticmethod
    def _key(sid):
        return hashlib.sha256(sid.encode('utf-8')).hexdigest()

    def put(self, session):
        """A fresh session id, or None with the table full of live sessions."""
        sid = secrets.token_urlsafe(SESSION_ID_BYTES)
        with self._lock:
            now = self._now()
            if len(self._by_key) >= self._limit:
                self._by_key = {k: s for k, s in self._by_key.items() if s.expires > now}
                if len(self._by_key) >= self._limit:
                    return None
            self._by_key[self._key(sid)] = session
        return sid

    def get(self, sid):
        if not sid:
            return None
        key = self._key(sid)
        with self._lock:
            s = self._by_key.get(key)
            if s is None:
                return None
            if s.expires <= self._now():
                del self._by_key[key]
                return None
            return s

    def delete(self, sid):
        if sid:
            with self._lock:
                self._by_key.pop(self._key(sid), None)


class Login:
    """The login routes' logic; zhen_app.py holds the Flask side."""

    def __init__(self, issuer_url, agentd_url, dev_plaintext=False, now=time.time):
        self.issuer_url = issuer_url.rstrip('/')
        self.agentd_url = agentd_url.rstrip('/')
        self.dev_plaintext = dev_plaintext
        self._now = now
        self.sessions = Sessions(now)
        self.failures = FailureBudget(now)

    @classmethod
    def from_env(cls, agentd_url, environ=None):
        env = os.environ if environ is None else environ
        return cls(env.get('AUTH_ISSUER_URL', '').strip(), agentd_url,
                   dev_plaintext=env.get('AUTH_BFF_DEV_PLAINTEXT', '').strip() == 'true')

    def config_error(self):
        """Why nobody can log in, or None. A token and a password go over TLS,
        except to loopback or under AUTH_BFF_DEV_PLAINTEXT."""
        if not self.issuer_url:
            return 'no issuer: set AUTH_ISSUER_URL to unheaded-auth'
        for name, url in (('AUTH_ISSUER_URL', self.issuer_url), ('ZHEN_AGENTD_URL', self.agentd_url)):
            parts = urlsplit(url)
            if parts.scheme == 'https' or self.dev_plaintext:
                continue
            if parts.scheme == 'http' and _loopback_host(parts.hostname or ''):
                continue
            return f'{name} must be https off loopback (AUTH_BFF_DEV_PLAINTEXT is for a stack without TLS)'
        return None

    def login(self, client, username, password):
        """(status, body, retry_after, sid, session). The status and body are
        the answer; sid and session are set on success alone."""
        error = self.config_error()
        if error:
            log.error('login refused: %s', error)
            return 503, {'error': 'login_unavailable', 'detail': error}, '', None, None
        ok, wait = self.failures.charge(client, username)
        if not ok:
            log.warning('login failure budget spent for %s', client)
            return 429, {'error': 'temporarily_locked'}, str(int(wait + 0.999)), None, None

        status, body, retry = _post_issuer(self.issuer_url, username, password)
        if status != 401:
            self.failures.refund(client, username)
        if status != 200:
            log.warning('login refused for %s: %s', client, status)
            word = {401: 'invalid_credentials', 429: 'temporarily_locked', 400: 'invalid_request'}.get(status)
            return (status if word else 502), {'error': word or 'issuer_unavailable'}, retry, None, None

        token = body.get('access_token', '') if isinstance(body, dict) else ''
        kind = body.get('token_type', '') if isinstance(body, dict) else ''
        now = self._now()
        life = token_life(token, now) if token and kind.lower() == 'bearer' else None
        if life is None:
            return 502, {'error': 'issuer_token_invalid'}, '', None, None

        code, who = _get_whoami(self.agentd_url, token)
        subject = who.get('subject', '') if code == 200 and isinstance(who, dict) else ''
        if not subject:
            log.error('zhen-agentd did not accept the login token: %s', code)
            return 502, {'error': 'agentd_refused_login'}, '', None, None
        roles = who.get('roles') or []
        session = Session(token, subject, tuple(roles), now + life)
        sid = self.sessions.put(session)
        if sid is None:
            return 503, {'error': 'sessions_unavailable'}, '', None, None
        log.info('login: %s from %s', subject, client)
        answer = me_body(session)
        if body.get('password_below_floor') is True:
            answer['password_below_floor'] = True
        return 200, answer, '', sid, session


def me_body(session):
    return {
        'sub': session.subject,
        'roles': list(session.roles),
        'expires_at': time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime(session.expires)),
    }


def can_write(session):
    return bool(OPERATOR_ROLES.intersection(session.roles))
