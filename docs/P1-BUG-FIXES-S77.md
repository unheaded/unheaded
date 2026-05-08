# S77 — Phase 1: P1 Bug-Fix Closeout

**Sprint:** S77 (Age 2 Acceleration)
**Phase:** 1 — Triage & Hardening
**Status:** Shipped
**Coverage doc:** `docs/compliance/go-test-status-2026-05-07.md` (s77 deliverable-gate test)

---

## Summary

S77 Phase 1 closed out four production-blocker bugs surfaced during the WAF + Wotan-client integration sweep. All four landed with regression tests and have shipped against the Alpha release line.

| Bug ID | Subject | Root cause | Fix | Test |
|--------|---------|-----------|-----|------|
| Bug-17 | X-Forwarded-For header spoofing | The WAF rule engine trusted the leftmost X-Forwarded-For value without validating against a TrustedProxies allowlist; an attacker could prepend a fake IP and bypass IP-based rate limiting / blocklists. | New `extractSecureClientIP` in `pkg/waf/rules/engine.go` walks the X-Forwarded-For chain right-to-left, returning the first IP NOT in the trusted-proxies CIDR set. Falls back to direct socket address. | `pkg/waf/rules/clientip_test.go::TestExtractSecureClientIP` (covers single-hop, multi-hop, all-trusted, none-trusted, malformed-IP). |
| Bug-19 | Wotan client silent failure on partial degradation | Wotan-client treated a server returning 503/504 as a fatal error; the connection retried with no backoff and no degraded-mode signaling. | `pkg/wotan-client/client.go` adds a `degraded` state machine with exponential-backoff retries and a `IsDegraded()` accessor for callers that want to drop non-critical traffic. | `pkg/wotan-client/client_test.go` (degraded-state transitions). |
| Bug-25 | Double-checked locking race in Wotan client cache | Cache lookup used Lock() unconditionally even when value was populated; high contention on hot keys. | Switched to `RLock()` for the read fast-path; only escalates to `Lock()` on cache miss. | `pkg/wotan-client/client_test.go` (concurrent-access). |
| Bug-22 | Bot-detection false positives on Googlebot UA | Heuristic flagged any UA containing "bot" without the reverse-DNS verify step. | `pkg/waf/detection/bot.go` adds `verifyReverseDNS` step gated behind a `--bot-strict` flag (default off in Alpha; on for beta). | `pkg/waf/detection/bot_test.go`. |

---

## Verification

```bash
go test ./pkg/waf/rules/... ./pkg/wotan-client/... ./pkg/waf/detection/... -v
go test ./tests/s77/... -run TestPhase1_BugFixes
```

Expected: all green.

---

## References

- `pkg/waf/rules/engine.go` — Bug-17 fix.
- `pkg/waf/rules/clientip_test.go` — regression test.
- `pkg/wotan-client/client.go` — Bugs 19 + 25.
- `pkg/waf/detection/bot.go` + `bot_test.go` — Bug-22.
- `tests/s77/s77_verification_test.go::TestPhase1_BugFixes` — deliverable gate.
