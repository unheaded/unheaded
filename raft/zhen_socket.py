# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""The web UI's only listener: one Unix socket, behind zhen-ui-front (ADR-138).

The app binds no TCP port and holds no TLS code. cmd/zhen-ui-front terminates
TLS 1.3 with hybrid ML-KEM key exchange only and proxies to this socket, so a
request that reaches the app arrived over that profile or came from an account
that can open the socket: the app's own, and the front's through the group of
a setgid directory. Nobody else can connect, which is the whole reason the
app may believe two things it is told here:

- every request is https (wsgi.url_scheme), so the session cookie is Secure
  and carries the __Host- prefix;
- the client's address is the front's X-Unheaded-Client-Addr, which the front
  sets from the TCP peer after deleting any copy the client sent. The login
  failure budget is per client address, so without it every browser would
  share one budget and one of them could spend it for all.

No other forwarding header is read, here or in zhen_app.py.
"""
import ipaddress
import os
import signal
import socket
import stat
import sys

from werkzeug.serving import make_server

SOCKET_ENV = 'ZHEN_UI_SOCKET'
# The same default as zhen-ui-front's --socket. The directory is made by the
# host (deploy/systemd/README.md#zhen-ui-front), never by the app.
DEFAULT_SOCKET = '/run/unheaded-zhen-ui/app.sock'
CLIENT_ADDR_KEY = 'HTTP_X_UNHEADED_CLIENT_ADDR'
# What a request without the front's header is charged to: a local account
# that opened the socket itself.
LOCAL_PEER = '<local>'
# sun_path is 108 bytes with its NUL.
_MAX_PATH = 107
_BACKLOG = 128


def socket_path(environ=None):
    """The socket's path: ZHEN_UI_SOCKET, or the default under /run. Exits on a
    retired TCP setting or a value that is not an absolute path."""
    env = os.environ if environ is None else environ
    if env.get('ZHEN_BIND_ADDR', '').strip():
        sys.exit('ZHEN_BIND_ADDR is retired (ADR-138): the UI binds no TCP port. '
                 "Unset it; the listen address is zhen-ui-front's --listen")
    path = env.get(SOCKET_ENV, '') or DEFAULT_SOCKET
    if not os.path.isabs(path) or os.path.normpath(path) != path:
        sys.exit(f'{SOCKET_ENV}={path!r} is not a clean absolute path to a Unix socket')
    if len(os.fsencode(path)) > _MAX_PATH:
        sys.exit(f'{SOCKET_ENV}={path!r} is longer than a socket path may be ({_MAX_PATH} bytes)')
    return path


def _check_directory(path):
    """The socket's directory, as lstat sees it. It must be a real directory,
    this account's or root's, that no group and no other account can write:
    whoever can write it can put its own socket where the front looks."""
    parent = os.path.dirname(path)
    try:
        st = os.lstat(parent)
    except OSError as exc:
        sys.exit(f'socket directory {parent}: {exc.strerror} '
                 '(the host makes it: deploy/systemd/README.md#zhen-ui-front)')
    if not stat.S_ISDIR(st.st_mode):
        sys.exit(f'socket directory {parent} is not a directory')
    if st.st_uid not in (os.geteuid(), 0):
        sys.exit(f'socket directory {parent} belongs to uid {st.st_uid}, not to this account or root')
    if st.st_mode & 0o022:
        sys.exit(f'socket directory {parent} is writable by its group or by others '
                 f'(mode {stat.S_IMODE(st.st_mode):04o})')
    return st


def _clear_stale(path):
    """Remove a socket nobody listens on. Anything else at the path, or a
    socket that answers, exits: the app never deletes what it did not make."""
    try:
        st = os.lstat(path)
    except FileNotFoundError:
        return
    except OSError as exc:
        sys.exit(f'socket {path}: {exc.strerror}')
    if not stat.S_ISSOCK(st.st_mode):
        sys.exit(f'{path} exists and is not a socket; remove it by hand')
    probe = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    try:
        probe.settimeout(2)
        probe.connect(path)
    except ConnectionRefusedError:
        os.unlink(path)
        return
    except OSError as exc:
        sys.exit(f'socket {path} exists and cannot be probed: {exc.strerror}')
    finally:
        probe.close()
    sys.exit(f'socket {path} already has a listener; is the UI running?')


def bind(path):
    """Bind and listen on the Unix socket at path; return the socket.

    Mode 0600. In a setgid directory the socket takes the directory's group,
    and it is then 0660 so that group can connect: the host gives the
    directory the front's own group, of which the front is the only member,
    and neither account joins the other's group. Either mode comes from the
    umask at bind, so there is no moment the socket is wider. Never a bit for
    others."""
    directory = _check_directory(path)
    _clear_stale(path)
    want = 0o660 if directory.st_mode & stat.S_ISGID else 0o600
    sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    old = os.umask(0o777 & ~want)
    try:
        sock.bind(path)
    except OSError as exc:
        sock.close()
        sys.exit(f'bind {path}: {exc.strerror}')
    finally:
        os.umask(old)
    try:
        mode = stat.S_IMODE(os.lstat(path).st_mode)
        if mode != want:
            raise OSError(0, f'mode is {mode:04o}, expected {want:04o}')
        sock.listen(_BACKLOG)
    except OSError as exc:
        sock.close()
        os.unlink(path)
        sys.exit(f'socket {path}: {exc.strerror}')
    return sock


def client_addr(value):
    """The front's client address as a plain IP, or LOCAL_PEER when it is absent
    or is not exactly one address. One spelling per address, as the front
    writes it: no zone, and an IPv4-mapped address as IPv4."""
    try:
        addr = ipaddress.ip_address(value)
    except ValueError:
        return LOCAL_PEER
    if addr.version == 6:
        if addr.scope_id is not None:
            return LOCAL_PEER
        if addr.ipv4_mapped is not None:
            addr = addr.ipv4_mapped
    return str(addr)


class Fronted:
    """WSGI wrapper for requests off the socket: https, and the client address
    the front recorded. Wrap the app only where it is served on the socket."""

    def __init__(self, app):
        self.app = app

    def __call__(self, environ, start_response):
        environ['wsgi.url_scheme'] = 'https'
        environ['REMOTE_ADDR'] = client_addr(environ.get(CLIENT_ADDR_KEY, ''))
        environ['REMOTE_PORT'] = 0
        # Werkzeug indexes the socket path as if it were (host, port). A
        # request with no Host then has no name the allowlist knows.
        environ['SERVER_NAME'] = 'unix'
        environ['SERVER_PORT'] = '0'
        return self.app(environ, start_response)


def make(app, path):
    """A threaded server for app on the Unix socket at path."""
    sock = bind(path)
    try:
        # fd= keeps Werkzeug from unlinking and re-binding the path itself.
        return make_server('unix://' + path, 0, Fronted(app), threaded=True, fd=sock.fileno())
    finally:
        sock.close()  # the server holds its own duplicate


def serve(app, environ=None):
    """Serve app on the socket until interrupted or terminated, then remove
    the socket. Call it from the main thread."""
    path = socket_path(environ)
    server = make(app, path)
    # SIGTERM unwinds like Ctrl-C, so a stopped UI leaves no dead socket.
    signal.signal(signal.SIGTERM, lambda *_: sys.exit(0))
    print(f'Zhen Web UI listening on unix:{path} (TLS and the TCP port are zhen-ui-front)')
    try:
        server.serve_forever()
    finally:
        try:
            os.unlink(path)
        except OSError:
            pass
