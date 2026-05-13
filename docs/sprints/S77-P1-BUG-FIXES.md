# S77 — P1 Bug Fixes (sprint index)

**Sprint:** S77 — Age 2 Acceleration Campaign
**Window:** ~Feb 24 – Mar 24, 2026
**Phase:** 1 — Triage & Hardening
**Status:** Closed (all four P1 bugs landed, regression tests green)
**Canonical doc:** [`docs/P1-BUG-FIXES-S77.md`](../P1-BUG-FIXES-S77.md)
**Gate test:** [`tests/s77/s77_verification_test.go::TestPhase1_BugFixes`](../../tests/s77/s77_verification_test.go)

---

## Purpose

This file is the sprint-folder index for S77 Phase 1. It points to the
canonical per-bug write-up and tabulates the four P1 production blockers
that S77 closed out. For root cause and regression-test detail, read the
canonical doc; for sprint accounting (commit refs, severity, status),
read this one.

---

## P1 bug table

| ID | Subject | Severity | Fix commit (canonical) | Test |
|----|---------|----------|------------------------|------|
| Bug-17 | X-Forwarded-For header spoofing in WAF rule engine | P1 / Security | `92f9838c` (`test(s77): add verification tests for P1 bug fixes — XFF spoofing, transport state, RWMutex`) + earlier engine.go landings | `pkg/waf/rules/clientip_test.go::TestExtractSecureClientIP` |
| Bug-19 | Wotan-client silent failure on partial degradation (503/504 with no degraded-state signaling) | P1 / Reliability | `92f9838c` test commit; client.go `degraded` state machine landed in the S77 acceleration commits `2a6194e5` / `6391b059` | `pkg/wotan-client/client_test.go` (degraded-state transitions) |
| Bug-25 | Double-checked-locking race in Wotan-client cache; unconditional `Lock()` on hot keys | P1 / Performance + concurrency | Switched read fast-path to `RLock()` (same S77 acceleration commit family) | `pkg/wotan-client/client_test.go` (concurrent-access) |
| Bug-22 | Bot-detection false positives on legitimate Googlebot UA strings | P1 / WAF correctness | `pkg/waf/detection/bot.go` adds `verifyReverseDNS` gated on `--bot-strict` flag | `pkg/waf/detection/bot_test.go` |

The four bugs are the same four called out in CLAUDE.md's "All 4 P1 bugs
fixed" line, mapped to their S77 sprint identity. The CLAUDE.md mention
of #20 Nix TLS, #36 log forwarding, #29 Kanban logging, and the E2E smoke
fix is a separate accounting layer (issue tracker numbers rather than
S77 bug IDs); both groups were closed in the same window and both groups
are verified by the S77 gate test.

---

## Deferred items

- **Bug-strict mode for bot detection** — `--bot-strict` ships disabled
  by default in Alpha. Enabling it Kingdom-wide is gated on Age 3 beta
  hardening once the reverse-DNS latency budget is measured against the
  sub-50ms latency gate (see `docs/sprints/S77-PERFORMANCE.md`).
- **WAF rate-limit storage backend swap** — the in-memory rate limiter
  is fine for Alpha but Phase 1 deliberately did not couple it to a
  durable store. Deferred to a follow-on sprint.

---

## How to verify

```bash
go test ./pkg/waf/rules/... ./pkg/wotan-client/... ./pkg/waf/detection/... -v
go test ./tests/s77/... -run TestPhase1_BugFixes -v
```

Expected: all green. The deliverable-gate suite reports
`Phase 1: 4/4 sub-tests pass`.

---

Free to use. Free to share.
