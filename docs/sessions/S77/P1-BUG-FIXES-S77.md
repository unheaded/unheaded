# P1 Bug Fixes — S77 Phase 1: Triage & Hardening

**Date:** March 5, 2026
**Sprint:** S77 Age 2 Acceleration Campaign
**Result:** All 3 P1 bugs fixed, 100% test passing

---

## Bug #17: Rate Limiter X-Forwarded-For IP Spoofing

**Severity:** Medium (security vulnerability)
**Root Cause:** WAF rules engine, chain rate limiter, and bot detector all trusted `X-Forwarded-For` header from any client without validating the peer IP.

**Attack Vector:** Attacker sends `X-Forwarded-For: <spoofed-ip>` header, causing the WAF/rate limiter to attribute requests to the wrong IP. This enables:
- Rate limit bypass (each request appears from a different IP)
- IP-based allowlist bypass
- Incorrect bot detection attribution

**Fix:** Use `RemoteAddr` as primary client IP. Only trust `X-Forwarded-For` and `X-Real-IP` when the direct peer (`RemoteAddr`) is an internal/private/loopback IP — i.e., traffic arrived through our own proxy infrastructure.

**Files Changed:**
- `pkg/waf/rules/engine.go` — Added `extractSecureClientIP()`, replaced blind XFF trust in `extractRequestData()`
- `pkg/waf/rules/chain.go` — Updated `getClientKey()` to use `extractSecureClientIP()`
- `pkg/waf/detection/bot.go` — Updated `getClientIP()` with same pattern

**Tests Added:**
- `pkg/waf/rules/clientip_test.go` — 8 test cases covering: external IP rejection of XFF, loopback trust, private IP trust, multiple XFF headers, XRI handling
- `pkg/waf/detection/bot_test.go` — 3 new cases: XFF spoofing blocked from external, XRI spoofing blocked, private IP trust

**Pre-existing Secure Pattern:** `pkg/http/context.go:ClientIP()` and `pkg/loadbalancer/l7.go:getClientIP()` already implemented the correct pattern. This fix brings the WAF into alignment.

**Security Assessment:** APPROVED — RemoteAddr spoofing requires network-layer compromise (acceptable residual risk).

---

## Bug #19: Wotan Silent Failure (Missing Degradation Logging)

**Severity:** Medium (operational visibility)
**Root Cause:** When gRPC transport to Wotan fails, the client silently degrades to HTTP polling. No log messages are emitted, so operators cannot tell that the service is running in degraded mode.

**Impact:** Services appear healthy (HTTP endpoints respond, health checks pass) but Wotan integration is broken. No alerts fire, no dashboards show the problem.

**Fix:** Added explicit logging at all degradation/recovery transition points:

1. **Startup probe failure:** `[WARN] wotan_client: gRPC probe failed at startup, running in degraded HTTP mode`
2. **Runtime degradation:** `[WARN] wotan_client: gRPC unhealthy, degraded to HTTP polling` (logged once per outage, not on every failure)
3. **Recovery:** `[INFO] wotan_client: gRPC recovered, promoted to primary transport`

**Files Changed:**
- `pkg/wotan-client/client.go` — Added `log` import, logging in `degradeToHTTP()`, `ProbeGRPC()`, and `NewClientWithGRPC()`

**Design Decision:** Used standard `log` package (not zerolog) to match existing wotan-client conventions and avoid adding a dependency.

**Security Assessment:** APPROVED — No new attack surface (logging only).

---

## Bug #25: gRPC Client Double-Check Locking Race Condition

**Severity:** Medium (resource leak, potential crash under load)
**Root Cause:** `getOrCreateGRPCClient()` claimed double-check locking in its comment but immediately acquired a write lock (`sync.Mutex`) on every call, eliminating the fast-path benefit.

**Impact:** Under high concurrency, all goroutines contend on the write lock even when a client already exists. While thread-safe, this creates unnecessary lock contention. The comment was misleading about the implementation.

**Fix:**
1. Changed `grpcMu` from `sync.Mutex` to `sync.RWMutex`
2. Implemented true double-check locking:
   - **First check (RLock):** Fast path — if client exists, return immediately with minimal contention
   - **Second check (Lock):** After acquiring write lock, check again in case another goroutine created it
   - **Create:** Only if still nil after both checks

**Files Changed:**
- `pkg/wotan-client/client.go` — Changed `grpcMu` type, rewrote `getOrCreateGRPCClient()`

**Verified:** `go test -race ./pkg/wotan-client/...` passes with 0 data races. Existing `TestGetOrCreateGRPCClient_ConcurrentInit` validates concurrent behavior.

**Security Assessment:** APPROVED — Standard concurrency pattern, no new race conditions possible.

---

## Phase 1 Exit Gate

- [x] All 3 P1s fixed and tested
- [x] 100% test passing (0 failures across all affected packages)
- [x] Security audit of fixes complete (all APPROVED)
- [x] Race detector clean (`go test -race`)
- [x] No new dependencies added (except `net` import where needed)
- [x] Consistent with existing secure patterns in codebase

**Age 2 Progress:** 45% → ~50% (Phase 1 complete)
