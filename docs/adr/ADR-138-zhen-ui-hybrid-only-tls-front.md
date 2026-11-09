<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-138: Hybrid-only TLS for the Zhen web UI through a Go front

**Status:** Proposed (parked). No code until Stevie reads this.
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
