<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-138: Hybrid-only TLS for the Zhen web UI through a Go front

**Status:** Accepted (Stevie's answer #32, 2026-11-09). Built 2026-11-10 on `sprint/adr-138-zhen-ui-front`; installed nowhere. The install is S-E1-LIVE Step 244a.
**Date:** 2026-11-09
**Related:** ADR-105 (post-quantum key exchange, Strict and Preferred profiles), ADR-103 (service auth), S-ANSWERED zhen lane.

## The answer (Stevie, 2026-11-09)

Asked whether the Zhen web UI may stay on ADR-105's Preferred profile (hybrid ML-KEM first, classical still accepted), Stevie chose **hybrid-only**.

## What is there

- `raft/zhen_app.py` serves TLS 1.3 itself when `UNHEADED_TLS_CERT` and `UNHEADED_TLS_KEY_FILE` are set (S-ANSWERED, zhen lane). It negotiates `X25519MLKEM768` with clients that offer it.
- Python 3.13's `ssl` cannot restrict key-exchange groups, so a classical-only client is still accepted. That is the Preferred profile, not Strict.
- `pkg/tls/pq.go` has `ApplyStrict`: TLS 1.3, hybrid groups only. Every first-party Go listener uses it when `UNHEADED_TLS_ENABLED=true`.
- The zhen lane refused a forwarded-header (ProxyFix) design so that no header could claim TLS.

## Options

1. **Go front (recommended, native first).** A small first-party listener (`cmd/zhen-ui-front`, or a mode of an existing Go binary) terminates TLS with `pkg/tls.ApplyStrict` and proxies to the Flask app over a Unix socket. Only the front's account can open that socket (mode 0600 or a shared group). The app then treats every connection on the socket as having arrived over Strict TLS. No forwarded header is trusted, and the app's own TLS code goes. Cost: one more unit and nologin account, about 150 lines of Go plus tests.
2. **nginx with OpenSSL 3.5+.** `ssl_ecdh_curve X25519MLKEM768;` in front of the app. nginx is already on west for the apt repository, but it is third party, its OpenSSL version decides what works, and the same trust question about the hop to the app applies.
3. **Wait for Python.** A later `ssl` release that can set groups would allow Strict in place. Nothing to build, and nothing to date it.

## Decision direction

Option 1. Open questions before code:

1. A new binary, or a `-front` mode on `zhen-agentd`, which already runs on that host?
2. Should the app still serve TLS alone when the front is absent (dev), or refuse anything but the socket?
3. The front, the socket path and the account go in S-E1-LIVE's install steps alongside the Zhen login items.

## Ruling (Stevie, 2026-11-09, answer #32)

Accepted as a new binary (`cmd/zhen-ui-front`, own nologin account); the app drops its own TLS and listens on the socket only, in dev too.

The three questions, as answered and built:

1. **A new binary.** `cmd/zhen-ui-front`, unit `unheaded-zhen-ui-front.service`, account `unheaded-zhen-ui-front` (uid 16755, `pkg/uids`, `/usr/sbin/nologin`), no capability. `zhen-agentd` gains nothing.
2. **Socket only, in dev too.** `raft/zhen_app.py` has no `ssl` import, no TCP listener and no plaintext mode. `ZHEN_BIND_ADDR` is retired: set, it stops the app with a message. `raft/start-zhen.sh` starts the app on its socket and then the front with the dev pair from `scripts/gen-dev-certs.sh zhen-ui-front`.
3. **Install.** S-E1-LIVE Step 244a (Evening 13), written and not run. `deploy/systemd/README.md#zhen-ui-front` is the reference.

## As built (2026-11-10)

**The front.** `tls.Config` from `pkg/tls.ApplyStrict`: TLS 1.3, `X25519MLKEM768` only. There is no flag or environment variable for the profile and no plaintext mode; `UNHEADED_TLS_ENABLED` is not read. `--listen` repeats and refuses a wildcard address. The shipped unit listens on `127.0.0.1:20103`; a LAN listener is a host's drop-in. No CA and no client certificate: the client is a browser and the app's login is the identity.

**The socket.** Default `/run/unheaded-zhen-ui/app.sock` (`ZHEN_UI_SOCKET`). Option 1 said "mode 0600 or a shared group". Built is the tighter form of the second, and it is the only one of the two that works with two accounts:

- 0600 lets only the app's account open the socket, so the front would have to run as the app's account. That is the dev layout only (`start-zhen.sh`, one account, directory 0700 under `$XDG_RUNTIME_DIR`).
- A group both accounts join would give each the other's group files. Instead the directory is `2750 <app account>:unheaded-zhen-ui-front`: the setgid bit puts the socket in the front's own group, whose only member is the front, and the app gives it 0660. Neither account joins the other's group. Nothing else reaches the directory.
- The mode comes from the umask at `bind()`, so the socket never exists with a wider mode. The app refuses a directory that its group or others can write, a path that is not a socket, and a resulting mode other than the one it asked for.
- The front looks the socket up with `os.OpenRoot` and `Lstat` (no symlink followed) and refuses to start on one with any permission bit for others.
- The directory is one tmpfiles.d line, as root, because it names the account the app runs as. The unit cannot make it.

**What the app believes.** Every request on the socket is https (`wsgi.url_scheme`), because nothing but the front can open it. The app reads no `X-Forwarded-*` header and has no ProxyFix. The one thing it takes from the front is the client address, `X-Unheaded-Client-Addr`.

**Headers.** The front deletes, before proxying, every client header that states where a request came from or how it arrived: `Forwarded`, `X-Forwarded-*`, `X-Real-IP`, `X-Client-IP`, `X-Cluster-Client-IP`, `True-Client-IP`, `X-HTTPS`, `X-Scheme`, `X-URL-Scheme`, `Front-End-Https`, `X-Original-URL`, `X-Rewrite-URL`, `X-Original-Host`, `X-Host`, any `X-Unheaded-*`, and `Upgrade`. Underscore spellings count as the hyphen ones, since WSGI folds them together. It then sets `X-Unheaded-Client-Addr` from the TCP peer. The app's `Server` header is dropped from responses.

**Why the client address.** The login failure budget (`raft/zhen_login.py`, five failures per address per 600 s) is keyed on the client address. With every request arriving from one socket peer, all browsers would share one budget and any one of them could lock the rest out. A client cannot choose the value: the front overwrites it, and the app takes exactly one plain IP address or falls back to the local peer.

**Ops listener.** `/health`, `/ready` and `/metrics` are plain HTTP on `127.0.0.1:20112` (`pkg/ports.ZhenUIFrontMetrics`); the front refuses a non-loopback address for it. `/ready` is 200 while the app answers on its socket. Metrics: `unheaded_zhen_ui_front_tls_client_hellos_total{kex="hybrid"|"classical"}`, `..._upstream_errors_total`, `..._connections`. No Prometheus job scrapes it: the scrapers come over the overlay and this is loopback only.

**Auth baseline.** `cmd/zhen-ui-front/main.go` has a row in `docs/policy/http-auth-baseline.txt` (listeners that do not import `pkg/auth`). Reason: the app behind the front does the login; the front has none of its own and its ops listener is loopback. That file says a new row is a decision for review, so this one is Stevie's to confirm.

**Callers.** Port 20103 no longer answers plain http. Akira's `zhenai` target is `https://localhost:20103/health` through the front, checked against the deployment CA. `zhen-mcp`'s `service_health`, the dashboard's `zhenai` row (`172.28.1.1:20112`) and five runbooks probe `/ready` on 20112. `scripts/run-coding-gate-via-webui.sh` and `eval/memory-ablation/run.sh` default to `https://localhost:20103` with the dev CA.

**Clients.** A client must offer the hybrid group: a current browser, a Go client, curl on OpenSSL 3.5 or later. Anything else fails the handshake and is counted under `kex="classical"`.

**Checked.** `cmd/zhen-ui-front` tests: a classical-only client and a TLS 1.2 client fail the handshake; client-sent forwarding and client-address headers never reach the app; a socket any local user can open is refused. One run of the built front against the Python socket server on two unused loopback ports gave the same results end to end.

**Not built.**

- The app has no unit and no account of its own; it runs as whoever starts it. When it gets one, a `.socket` unit with `SocketUser=` and `SocketMode=0600` handing the listener to the app can replace the tmpfiles line.
- `cert-gen` has no flag for extra names, so the dev pair names `zhen-ui-front`, `localhost`, `127.0.0.1` and `::1` only. A LAN listener needs a certificate that names the LAN address.
- The gateway proxy that would carry 20112 to the dashboard container is host configuration, not in the repository.
- `cmd/wiki-server` still links to `http://192.168.69.184:20103`.

**Superseded text in dated records (not edited).** `docs/security/bandit-disposition-2026-08-03.md` (the B104 row: `bind_address()`, `ZHEN_BIND_ADDR`, `raft/tests/test_zhen_bind_addr.py`) and the `ZHEN_BIND_ADDR` rows in S-TRUE-WIRE's plan and progress log describe the listener this ADR removed. The app binds no address now; the test file is `raft/tests/test_zhen_socket.py`.
