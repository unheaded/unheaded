<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-121: A future WAF (idea, lowest priority)

**Status:** Proposed (parked under ADR-112; lowest priority of the parked ideas). Nothing is built, planned or scheduled.
**Date:** 2026-11-04
**Grounded on:** `develop` at `d6ad43e2`.
**Related:** DP-401.1, DP-709, round 4 of the decision pack, ADR-113 (third-party verdicts), ADR-115 (Shop).

## Context

`pkg/waf` (seven Go packages, no caller) and `cmd/waf` (a Rust binary named `shield`: rule engine, rate limiter, TLS on rustls with aws-lc-rs; no unit, compose service or importer ran it) left the repo in S-CLEAN-CUT. Stevie's word of 2026-11-04: a WAF stays a future idea, and any open-source WAF sits outside the repo, after the third-party sitting. The Go `cmd/shield` stays and is not a WAF. Both deleted trees are in git history at `d6ad43e2`.

> **Note (2026-11-10).** Both trees now sit, with their history, in a separate repository, `unheaded-waf`, as an archive: nothing runs it and nothing here depends on it. Its README records what builds and what was never tested. This ADR is unchanged: the question below is still open.

## The idea

Small shops get request filtering at the edge without writing security code: an outside open-source WAF run beside the edge, as a separate deployment, admitted at the third-party sitting.

## Open question

Which WAF, and does it serve ADR-115's Shop?

No decision is made here.
