# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.

"""The Zhen pages load nothing from another host and run no inline code
(decision pack 2026-11-18 row 41, M-206.1 and M-206.2).

The chat page shows model output and retrieved text, some of it from an
`external` source. Three things hold it: the page is a constant file with no
inline script, style or handler; everything it loads is this origin's own
file; and the route sends a Content-Security-Policy that says so to the
browser. These tests read the files as they are served and the headers the
app sends: nothing listens, no model runs.
"""

import re
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

ORIGIN = 'http://zhen.test'

PAGES = sorted((RAFT / 'pages').glob('*.html'))
STATIC = sorted(p for p in (RAFT / 'static').rglob('*') if p.suffix in ('.html', '.css', '.js'))

# A tag that makes the browser fetch something: its src or href must be this
# origin's own path. <a href> is a link a person follows, not a load.
_LOADING_TAG = re.compile(r'<(script|link|img|iframe|frame|embed|object|source|video|audio|track|base)\b([^>]*)>',
                          re.IGNORECASE)
_URL_ATTR = re.compile(r'\b(?:src|href|data|srcset|poster)\s*=\s*("[^"]*"|\'[^\']*\'|[^\s>]+)', re.IGNORECASE)
_FONT_HOSTS = ('fonts.googleapis.com', 'fonts.gstatic.com')


@pytest.fixture
def client(monkeypatch):
    monkeypatch.setenv('ZHEN_ALLOWED_HOSTS', 'zhen.test')
    zhen_app.app.config['TESTING'] = True
    with zhen_app.app.test_client() as c:
        yield c


def _log_in(client, token='person-token'):  # nosec B107 - fixture token, no credential
    sid = zhen_app.LOGIN.sessions.put(zhen_login.Session(token, 'stevie', ('operator',), time.time() + 600))
    client.set_cookie(zhen_login.COOKIE_NAME, sid, domain='zhen.test')


def _policy(resp):
    return dict(part.strip().split(' ', 1) for part in resp.headers['Content-Security-Policy'].split(';'))


def _loads(html):
    """Every URL a page makes the browser fetch, with the tag it sits in."""
    out = []
    for tag, attrs in _LOADING_TAG.findall(html):
        for value in _URL_ATTR.findall(attrs):
            out.append((tag.lower(), value.strip('"\'')))
    return out


def _external(url):
    """True for a URL that leaves this origin: a scheme, or a host with none."""
    return bool(re.match(r'\s*(?:[a-z][a-z0-9+.-]*:|//)', url, re.IGNORECASE))


# ── the helpers themselves: a test that cannot fail proves nothing ───────

@pytest.mark.parametrize('html', [
    '<link href="https://fonts.googleapis.com/css2?family=X" rel="stylesheet">',
    '<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>',
    '<script src="//cdn.example/x.js"></script>',
    "<script src='http://cdn.example/x.js'></script>",
    '<img src=https://example.org/a.png>',
    '<link rel="stylesheet" href="data:text/css,body{}">',
])
def test_the_scan_sees_an_external_load(html):
    assert any(_external(url) for _, url in _loads(html))


@pytest.mark.parametrize('html', [
    '<link rel="stylesheet" href="/static/chat.css">',
    '<script src="/auth/nav.js"></script>',
    '<a href="http://localhost:20001" target="_blank" rel="noopener">Kanban</a>',
])
def test_the_scan_passes_an_own_file_and_a_plain_link(html):
    assert not any(_external(url) for _, url in _loads(html))


# ── the files ───────────────────────────────────────────────────────────

def test_there_are_pages_and_static_files_to_check():
    assert [p.name for p in PAGES] == ['chat.html', 'source-view.html']
    names = {p.name for p in STATIC}
    assert {'chat.js', 'chat.css', 'source-view.js', 'source-view.css'} <= names
    # A page under static/ would be served by the static route, with no
    # policy of its own.
    assert not [p.name for p in STATIC if p.suffix == '.html']


@pytest.mark.parametrize('page', PAGES, ids=lambda p: p.name)
def test_a_page_loads_only_this_origins_files(page):
    loads = _loads(page.read_text())
    assert loads, 'the page loads no script or stylesheet at all: the scan is not reading it'
    assert [(tag, url) for tag, url in loads if _external(url)] == []
    assert all(url.startswith('/') for _, url in loads)


@pytest.mark.parametrize('page', PAGES, ids=lambda p: p.name)
def test_a_page_has_no_inline_script_style_or_handler(page):
    html = page.read_text()
    scripts = re.findall(r'<script\b([^>]*)>(.*?)</script>', html, re.IGNORECASE | re.DOTALL)
    assert scripts, 'the page has no script'
    for attrs, body in scripts:
        assert re.search(r'\bsrc\s*=', attrs) and body.strip() == ''
    assert not re.search(r'<style\b', html, re.IGNORECASE)
    assert not re.search(r'\s(?:style|on[a-z]+)\s*=', html)
    assert 'javascript:' not in html.lower()


@pytest.mark.parametrize('path', STATIC, ids=lambda p: p.name)
def test_a_static_file_names_no_other_host(path):
    text = path.read_text()
    for host in _FONT_HOSTS:
        assert host not in text
    if path.suffix == '.css':
        assert '@import' not in text
        assert [u for u in re.findall(r'url\(\s*["\']?([^"\')]+)', text) if _external(u)] == []
    if path.suffix == '.js':
        # A string that starts with a scheme or a bare host: something to fetch.
        assert re.findall(r'["\'`](?:[a-z][a-z0-9+.-]*:)?//[A-Za-z0-9\[]', text) == []
        assert 'importScripts' not in text and not re.search(r'\bimport\s*\(', text)


@pytest.mark.parametrize('path', [p for p in STATIC if p.suffix == '.js'], ids=lambda p: p.name)
def test_a_script_writes_no_inline_style_or_handler_into_the_page(path):
    """Markup a script builds is under the same policy: a style attribute or
    an on* attribute in it would be refused by the browser, and the handler
    with it. Assigning element.style or element.onclick from script is not
    markup and is allowed."""
    text = path.read_text()
    assert re.findall(r'\s(?:style|on[a-z]+)=["\'\\$`]', text) == []
    assert not re.search(r'\beval\s*\(|\bnew\s+Function\b|\bdocument\.write\b', text)
    assert not re.search(r'set(?:Timeout|Interval)\(\s*["\'`]', text)


def test_every_element_the_chat_script_wires_is_in_the_page():
    """The handlers moved out of the markup into static/chat.js. A listener
    attached to an id the page does not have throws at load and takes every
    later handler with it, and no browser runs in these tests: so the ids and
    data attributes the script names are checked against the page here."""
    html = (RAFT / 'pages' / 'chat.html').read_text()
    js = (RAFT / 'static' / 'chat.js').read_text()
    page_ids = set(re.findall(r'\bid="([^"]+)"', html))
    built_ids = set(re.findall(r'\bid="([^"$]+)"', js))  # ids in markup the script builds
    wired = set(re.findall(r"onClick\('([^']+)'", js))
    assert wired >= {'btn', 'history-go', 'history-all', 'dash-refresh', 'remove-file', 'upload-btn'}
    assert wired <= page_ids
    looked_up = set(re.findall(r"getElementById\('([^']+)'\)", js))
    assert looked_up - page_ids - built_ids == set()
    # switchTab builds the view ids from the tab names.
    tabs = re.findall(r'<button id="tab-([a-z]+)"[^>]*data-tab="([a-z]+)"', html)
    assert tabs and all(a == b for a, b in tabs)
    assert {f'{name}-view' for name, _ in tabs} <= page_ids
    # Every operator button has a function behind it, and the other way round.
    ops_in_page = set(re.findall(r'data-op="([^"]+)"', html))
    table = re.search(r'const OP_ACTIONS = \{(.*?)\};', js, re.DOTALL).group(1)
    ops_in_script = set(re.findall(r"'([a-z-]+)':", table))
    assert ops_in_page and ops_in_page == ops_in_script
    assert len(re.findall(r'data-ask="[^"]+"', html)) == 8
    for selector in ('.tab-bar button[data-tab]', '#suggestions button[data-ask]', '.op-actions button[data-op]'):
        assert selector in js


def test_a_hidden_view_stays_hidden():
    """The tabs are shown and hidden with the hidden attribute (a style
    attribute is refused by the policy). The stylesheet sets display on the
    views, which would show a hidden one without this rule."""
    css = (RAFT / 'static' / 'chat.css').read_text()
    assert re.search(r'\[hidden\]\s*\{\s*display:\s*none\s*!important;\s*\}', css)
    html = (RAFT / 'pages' / 'chat.html').read_text()
    for view in ('search-view', 'history-view', 'dashboard-view'):
        assert re.search(rf'<div id="{view}" hidden>', html)
    assert '<div id="chat-view">' in html


def test_no_font_file_and_no_font_face_is_in_the_tree():
    """System fonts only (M-206.1): a font file would be third-party material,
    and a @font-face with nothing behind it is a claim that is not true."""
    assert [p.name for p in (RAFT / 'static').rglob('*') if p.suffix in ('.woff', '.woff2', '.ttf', '.otf')] == []
    for path in STATIC:
        assert '@font-face' not in path.read_text()


def test_the_highlight_stylesheet_names_no_other_host(client):
    r = client.get('/api/v1/highlight.css', base_url=ORIGIN)
    assert r.status_code == 200
    css = r.get_data(as_text=True)
    assert '@import' not in css and 'url(' not in css


# ── the headers ─────────────────────────────────────────────────────────

def test_the_chat_page_is_the_constant_file(client):
    r = client.get('/', base_url=ORIGIN)
    assert r.status_code == 200
    assert r.data == (RAFT / 'pages' / 'chat.html').read_bytes()
    assert r.mimetype == 'text/html'


def test_the_chat_page_sends_its_policy_before_and_after_login(client):
    for logged_in in (False, True):
        if logged_in:
            _log_in(client)
        r = client.get('/', base_url=ORIGIN)
        assert r.status_code == 200
        csp = _policy(r)
        assert csp == {
            'default-src': "'none'",
            'script-src': "'self'",
            'style-src': "'self'",
            'connect-src': "'self'",
            'img-src': "'none'",
            'base-uri': "'none'",
            'form-action': "'none'",
            'frame-ancestors': "'none'",
        }
        assert r.headers['X-Content-Type-Options'] == 'nosniff'
        assert r.headers['Referrer-Policy'] == 'no-referrer'
        assert r.headers['X-Frame-Options'] == 'DENY'


@pytest.mark.parametrize('path', ['/', '/view/source?id=x'])
def test_a_pages_policy_names_no_host_and_nothing_unsafe(client, path):
    _log_in(client)
    r = client.get(path, base_url=ORIGIN)
    assert r.status_code == 200
    raw = r.headers['Content-Security-Policy']
    assert 'unsafe' not in raw and '*' not in raw
    assert 'http' not in raw and 'data:' not in raw and 'blob:' not in raw
    for host in _FONT_HOSTS:
        assert host not in raw
    csp = _policy(r)
    assert csp['default-src'] == "'none'"
    assert csp['script-src'] == "'self'" and csp['style-src'] == "'self'"
    # No font is loaded, so no font-src: default-src 'none' covers it.
    assert 'font-src' not in csp


def test_the_chat_page_is_not_served_without_its_headers(client):
    _log_in(client)
    for path in ('/static/index.html', '/static/chat.html', '/static/../pages/chat.html'):
        assert client.get(path, base_url=ORIGIN).status_code == 404


@pytest.mark.parametrize('name,mimetype', [
    ('chat.js', ('text/javascript', 'application/javascript')),
    ('chat.css', ('text/css',)),
    ('source-view.js', ('text/javascript', 'application/javascript')),
    ('source-view.css', ('text/css',)),
])
def test_a_static_file_is_served_with_its_type_and_nosniff(client, name, mimetype):
    """The pages' policy lets the browser run /static scripts: nosniff makes it
    refuse one that is not served as a script."""
    r = client.get('/static/' + name, base_url=ORIGIN)
    assert r.status_code == 200
    assert r.mimetype in mimetype
    assert r.headers['X-Content-Type-Options'] == 'nosniff'


@pytest.mark.parametrize('path', ['/health', '/api/v1/highlight.css', '/static/chat.js', '/api/v1/stats', '/no-such-route'])
def test_every_other_response_carries_the_closed_policy(client, path):
    """A response that is not a page (JSON, a stylesheet, an error) allows
    nothing if a browser is ever made to render it as a document."""
    r = client.get(path, base_url=ORIGIN)
    assert _policy(r) == {
        'default-src': "'none'",
        'base-uri': "'none'",
        'form-action': "'none'",
        'frame-ancestors': "'none'",
    }
    assert r.headers['X-Content-Type-Options'] == 'nosniff'
    assert r.headers['X-Frame-Options'] == 'DENY'
    assert r.headers['Referrer-Policy'] == 'no-referrer'
