# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.

"""The web UI listens on one Unix socket and nowhere else (ADR-138).

Until 2026-11-09 zhen_app.py bound a TCP address (ZHEN_BIND_ADDR) and served
its own TLS, which could prefer the hybrid groups and never refuse the
classical ones. cmd/zhen-ui-front holds the port and the TLS now. What is
tested here is the app's half: where the socket may be, who can open it, and
what the app believes of a request that arrives on it."""

import http.client
import json
import os
import re
import shutil
import signal
import socket
import stat
import subprocess
import sys
import tempfile
import threading
import time
import types
from pathlib import Path

import pytest
from flask import Flask, jsonify, request

if 'sentence_transformers' not in sys.modules:
    _st = types.ModuleType('sentence_transformers')
    _st.SentenceTransformer = lambda *a, **kw: object()
    sys.modules['sentence_transformers'] = _st

RAFT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(RAFT / 'scripts'))
sys.path.insert(0, str(RAFT))

import zhen_app
import zhen_socket


def _rundir():
    d = Path(tempfile.mkdtemp(prefix='zs'))
    d.chmod(0o700)
    return d


@pytest.fixture
def rundir():
    """A short private directory: a socket path has 107 bytes to fit in."""
    d = _rundir()
    yield d
    shutil.rmtree(d, ignore_errors=True)


def _mode(path):
    return stat.S_IMODE(os.lstat(path).st_mode)


class UnixConnection(http.client.HTTPConnection):
    def __init__(self, path):
        super().__init__('localhost', timeout=10)
        self._path = path

    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.settimeout(self.timeout)
        self.sock.connect(self._path)


def _echo_app():
    app = Flask('echo')

    @app.route('/who')
    def who():
        return jsonify({'addr': request.remote_addr, 'secure': request.is_secure, 'scheme': request.scheme,
                        'host': request.host})

    return app


@pytest.fixture(scope='module')
def served():
    """The echo app on a real socket, through the same wrapper the UI gets."""
    d = _rundir()
    path = str(d / 'app.sock')
    server = zhen_socket.make(_echo_app(), path)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    yield path
    server.shutdown()
    server.server_close()
    shutil.rmtree(d, ignore_errors=True)


def _who(path, headers=None):
    conn = UnixConnection(path)
    try:
        conn.request('GET', '/who', headers={'Host': 'localhost', **(headers or {})})
        r = conn.getresponse()
        assert r.status == 200
        return json.loads(r.read())
    finally:
        conn.close()


# ── where the socket is ─────────────────────────────────────────────────

def test_the_default_socket_is_the_one_the_front_dials():
    assert zhen_socket.socket_path({}) == '/run/unheaded-zhen-ui/app.sock'
    front = (RAFT.parent / 'cmd' / 'zhen-ui-front' / 'main.go').read_text()
    assert f'defaultSocket = "{zhen_socket.DEFAULT_SOCKET}"' in re.sub(r'[ \t]+', ' ', front)


def test_the_environment_names_the_socket():
    assert zhen_socket.socket_path({'ZHEN_UI_SOCKET': '/run/user/1000/unheaded-zhen-ui/app.sock'}) \
        == '/run/user/1000/unheaded-zhen-ui/app.sock'
    assert zhen_socket.socket_path({'ZHEN_UI_SOCKET': ''}) == zhen_socket.DEFAULT_SOCKET


@pytest.mark.parametrize('bad', ['127.0.0.1:20103', '0.0.0.0:20103', 'localhost', 'unix:///run/x/app.sock',  # nosec B104 - test data: an address is what must be refused
                                 'tcp://127.0.0.1:20103', 'app.sock', './app.sock', '/run/x/../app.sock',
                                 '/run/x//app.sock', '/run/x/', '/' + 'a' * 120])
def test_anything_but_a_clean_absolute_path_refuses(bad):
    with pytest.raises(SystemExit):
        zhen_socket.socket_path({'ZHEN_UI_SOCKET': bad})


@pytest.mark.parametrize('addr', ['127.0.0.1', '10.10.20.1', '0.0.0.0', '::'])  # nosec B104 - test data: every one must stop the UI
def test_the_retired_tcp_setting_stops_the_ui(addr):
    # A UI that ignored it would come up loopback-only behind the front while
    # its operator believed the old address was served.
    with pytest.raises(SystemExit) as stop:
        zhen_socket.socket_path({'ZHEN_BIND_ADDR': addr})
    assert 'ZHEN_BIND_ADDR' in str(stop.value)


def test_the_app_has_no_tcp_listener_and_no_tls_of_its_own():
    src = (RAFT / 'zhen_app.py').read_text()
    for gone in ('import ssl', 'ssl_context', 'app.run(', 'ZHEN_BIND_ADDR', 'ProxyFix', 'X-Forwarded-Proto\')'):
        assert gone not in src, gone
    assert not hasattr(zhen_app, 'bind_address') and not hasattr(zhen_app, 'tls_context')
    assert src.rstrip().endswith('zhen_socket.serve(app)')
    # One place makes a server, and only for a unix:// address.
    sock_src = (RAFT / 'zhen_socket.py').read_text()
    assert re.findall(r'make_server\(([^,]+),', sock_src) == ["'unix://' + path"]
    assert 'AF_INET' not in sock_src and 'ssl' not in sock_src


# ── who can open it ─────────────────────────────────────────────────────

def test_the_socket_is_owner_only(rundir):
    path = str(rundir / 'app.sock')
    sock = zhen_socket.bind(path)
    try:
        assert stat.S_ISSOCK(os.lstat(path).st_mode)
        assert _mode(path) == 0o600
    finally:
        sock.close()


def test_a_setgid_directory_lets_its_group_connect_and_nobody_else(rundir):
    # The host's layout: the directory carries the front's group and the
    # setgid bit, so the socket is born in that group.
    rundir.chmod(0o2750)
    if not os.lstat(rundir).st_mode & stat.S_ISGID:
        pytest.skip('this filesystem drops the setgid bit')
    path = str(rundir / 'app.sock')
    sock = zhen_socket.bind(path)
    try:
        assert _mode(path) == 0o660
        assert os.lstat(path).st_gid == os.lstat(rundir).st_gid
    finally:
        sock.close()


def test_the_umask_is_put_back(rundir):
    before = os.umask(0o022)
    try:
        zhen_socket.bind(str(rundir / 'app.sock')).close()
        after = os.umask(0o022)
        assert after == 0o022
    finally:
        os.umask(before)


@pytest.mark.parametrize('mode', [0o777, 0o770, 0o757, 0o1777, 0o2770])
def test_a_directory_others_can_write_refuses(rundir, mode):
    rundir.chmod(mode)
    path = str(rundir / 'app.sock')
    with pytest.raises(SystemExit) as stop:
        zhen_socket.bind(path)
    assert 'writable' in str(stop.value)
    assert not os.path.lexists(path)


def test_a_missing_directory_refuses_and_is_not_made(rundir):
    path = str(rundir / 'absent' / 'app.sock')
    with pytest.raises(SystemExit):
        zhen_socket.bind(path)
    assert not (rundir / 'absent').exists()


def test_a_symlinked_directory_refuses(rundir):
    real = rundir / 'real'
    real.mkdir(mode=0o700)
    (rundir / 'link').symlink_to(real)
    with pytest.raises(SystemExit):
        zhen_socket.bind(str(rundir / 'link' / 'app.sock'))
    assert not (real / 'app.sock').exists()


def test_a_file_at_the_path_is_left_alone(rundir):
    path = rundir / 'app.sock'
    path.write_text('not a socket')
    with pytest.raises(SystemExit):
        zhen_socket.bind(str(path))
    assert path.read_text() == 'not a socket'


def test_a_symlink_at_the_path_is_left_alone(rundir):
    target = rundir / 'elsewhere.sock'
    (rundir / 'app.sock').symlink_to(target)
    with pytest.raises(SystemExit):
        zhen_socket.bind(str(rundir / 'app.sock'))
    assert (rundir / 'app.sock').is_symlink() and not target.exists()


def test_a_dead_socket_is_replaced(rundir):
    path = str(rundir / 'app.sock')
    old = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    mask = os.umask(0)
    try:
        old.bind(path)
    finally:
        os.umask(mask)
    old.close()  # bound, never listening, file left behind: a crash
    assert _mode(path) == 0o777
    sock = zhen_socket.bind(path)
    try:
        assert _mode(path) == 0o600
    finally:
        sock.close()


def test_a_running_ui_is_not_displaced(rundir):
    path = str(rundir / 'app.sock')
    first = zhen_socket.bind(path)
    try:
        inode = os.lstat(path).st_ino
        with pytest.raises(SystemExit) as stop:
            zhen_socket.bind(path)
        assert 'already has a listener' in str(stop.value)
        assert os.lstat(path).st_ino == inode
    finally:
        first.close()


def test_a_stopped_ui_takes_its_socket_with_it(rundir):
    path = rundir / 'app.sock'
    code = ('import sys; sys.path.insert(0, sys.argv[1]); import zhen_socket; from flask import Flask; '
            "zhen_socket.serve(Flask('stop'), {'ZHEN_UI_SOCKET': sys.argv[2]})")
    proc = subprocess.Popen([sys.executable, '-c', code, str(RAFT), str(path)],
                            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    try:
        deadline = time.monotonic() + 20
        while not path.exists() and proc.poll() is None and time.monotonic() < deadline:
            time.sleep(0.05)
        assert path.exists(), 'the UI did not bind its socket'
        proc.send_signal(signal.SIGTERM)
        assert proc.wait(timeout=20) == 0
        assert not os.path.lexists(path)
    finally:
        if proc.poll() is None:
            proc.kill()
            proc.wait()


# ── what the app believes of a request on the socket ────────────────────

def test_a_request_on_the_socket_is_https(served):
    got = _who(served)
    assert got['secure'] is True and got['scheme'] == 'https'


def test_the_client_address_is_the_one_the_front_recorded(served):
    assert _who(served, {'X-Unheaded-Client-Addr': '203.0.113.7'})['addr'] == '203.0.113.7'
    assert _who(served, {'X-Unheaded-Client-Addr': '2001:db8::7'})['addr'] == '2001:db8::7'


def test_an_address_has_one_spelling():
    # The failure budget is keyed on this string.
    assert zhen_socket.client_addr('::ffff:203.0.113.7') == '203.0.113.7'
    assert zhen_socket.client_addr('2001:0db8:0000::0007') == '2001:db8::7'


@pytest.mark.parametrize('value', ['', 'localhost', '203.0.113.7, 127.0.0.1', '203.0.113.7:4444', '127.0.0.1 ',
                                   '203.0.113.7/32', 'fe80::1%eth0x', '999.1.1.1'])
def test_an_address_that_is_not_one_address_is_nobody(served, value):
    # Never loopback by default: a request that names no client gets no
    # loopback allowance.
    assert zhen_socket.client_addr(value) == zhen_socket.LOCAL_PEER
    assert _who(served, {'X-Unheaded-Client-Addr': value} if value else None)['addr'] == zhen_socket.LOCAL_PEER


def test_two_copies_of_the_header_are_nobody(served):
    conn = UnixConnection(served)
    try:
        conn.putrequest('GET', '/who')
        conn.putheader('X-Unheaded-Client-Addr', '127.0.0.1')
        conn.putheader('X-Unheaded-Client-Addr', '127.0.0.1')
        conn.endheaders()
        assert json.loads(conn.getresponse().read())['addr'] == zhen_socket.LOCAL_PEER
    finally:
        conn.close()


@pytest.mark.parametrize('header', ['X-Forwarded-For', 'X-Real-IP', 'Forwarded', 'X-Client-IP', 'True-Client-IP',
                                    'X_Unheaded_Client_Addr'])
def test_no_other_header_names_the_client(served, header):
    value = 'for=127.0.0.1' if header == 'Forwarded' else '127.0.0.1'
    assert _who(served, {header: value})['addr'] == zhen_socket.LOCAL_PEER


@pytest.mark.parametrize('headers', [{'X-Forwarded-Proto': 'http'}, {'X-Forwarded-Host': 'evil.example'},
                                     {'X-Forwarded-Port': '80'}, {'X-Scheme': 'http'}])
def test_no_header_changes_the_scheme_or_the_host(served, headers):
    got = _who(served, headers)
    assert got['secure'] is True and got['host'] == 'localhost'


def test_a_request_without_a_host_has_no_name_the_ui_serves(served):
    raw = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    try:
        raw.settimeout(10)
        raw.connect(served)
        raw.sendall(b'GET /who HTTP/1.0\r\n\r\n')
        body = b''
        while chunk := raw.recv(4096):
            body += chunk
    finally:
        raw.close()
    got = json.loads(body.split(b'\r\n\r\n', 1)[1])
    assert got['host'].split(':')[0] not in zhen_app._allowed_hosts()
    assert got['addr'] == zhen_socket.LOCAL_PEER
