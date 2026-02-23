# SKILL ASSESSMENT S33: WS1 + WS3 Readiness Review
**Date:** February 23, 2026
**Reviewed By:** Developer, BlackMage, Architect (Four-Mind Council)
**Subject:** Code readiness for WS1 (Doom-Bridge) and WS3 (Scaling to Playable)
**Decision Deadline:** Today, EOD

---

## DEVELOPER ASSESSMENT — Code Readiness for WS1+WS3

### Go Patterns & Implementation Baseline

**Status:** READY with minor hardening needed

#### Gorilla WebSocket (WS1)

- `github.com/gorilla/websocket v1.5.3` already in `go.mod` ✅
- `cmd/doom-bridge/main.go` skeleton exists (~489 lines) with:
  - `Client` struct wrapping `*websocket.Conn` ✅
  - `Bridge` server state with client map + mutex ✅
  - `tagScreen = 0x01`, `tagKbd = 0x02` protocol tags defined ✅
  - Binary frame support for efficiency ✅

**Concern:** Line 81–83 shows no explicit `ReadDeadline`/`WriteDeadline` on WebSocket connections. WS1 must add:
```go
conn.SetReadDeadline(time.Now().Add(60 * time.Second))
conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
// Refresh on each successful message
```

#### BPF Map Access (WS1 + WS3)

- `BPFMap` struct defined in `bpf.go` ✅
- `openMaps()`, `screenLoop()`, `statsLoop()` functions present ✅
- **Concern:** No verification that `cilium/ebpf` is in go.mod. WS1 reads BPF maps, but:
  - If using syscall-based `BPF_OBJ_GET` (pinned maps), no external dep needed ✅
  - If using `cilium/ebpf` library, must add: `go get github.com/cilium/ebpf@latest`

**Recommendation:** Verify in `bpf.go` which approach is used. Syscall approach is lighter, already present in WS3 plan (Step 2.6 Python snippet uses syscalls).

#### PNG Encoding (WS1)

- **CRITICAL:** No PNG encoding found in codebase.
- Battle plan Step 4–5 (lines 150–200, first 100 lines snippet) implies serving raw framebuffer OR PNG-encoded images.
- If WebSocket sends raw 64KB frames: `image/png` stdlib sufficient.
- If dashboard expects PNG: Need `github.com/disintegration/imaging` or `golang.org/x/image/png`

**Action Required:** Clarify protocol. WS1 battle plan doesn't specify frame format. Recommend **raw 8-bit indexed frames** (64KB per frame) with palette sent once on client connect. This avoids PNG overhead.

#### Race Conditions (WS1)

**Critical Risk:** `screenLoop()` and `statsLoop()` running concurrently, both writing to BPF maps via `bpf.go`:

```go
// In bridge.go
go b.screenLoop(ctx)      // Reads SCREEN_MAP
go b.statsLoop(ctx)       // Reads STATS_MAP
go b.broadcastLoop(ctx)   // Broadcasts to all clients
```

**Concurrent Clients:** Multiple WebSocket clients all reading the same client map:

```go
// Line 80: clients map[*client]struct{}
b.clientsMu RWLock // Good: RWLock for concurrent reads
```

**Risk:** If `clientsMu` is used correctly, clients are safe. But `screenMap`, `statsMap` operations must be atomic:

- cilium/ebpf provides atomic map ops ✅
- Syscall approach: Need `sync/atomic` wrapper around BPF_OBJ_GET calls

**Verification Needed:** Review `bpf.go` to confirm:
- [ ] SCREEN_MAP reads don't race with kernel writes
- [ ] STATS_MAP lookups use proper locking (BPF ring buffer is already thread-safe)
- [ ] Multiple clients don't cause write conflicts on KBD_MAP

### TDD (Test-First) Checklist for WS1

**Before writing implementation, these tests should be written FIRST:**

1. **Unit: BPF Map Opening**
   - Test: `TestOpenScreenMap()` — Verify `/sys/fs/bpf/unheaded/doom-ring/maps/SCREEN_MAP` is readable
   - Teardown: Mock or skip if no BPF environment
   - File: `cmd/doom-bridge/bpf_test.go`

2. **Unit: Frame Buffer Read**
   - Test: `TestReadScreenFramebuffer()` — Read 64KB from SCREEN_MAP, verify shape (320x200)
   - Test data: Create fake 64KB pattern, verify pattern after read
   - File: `cmd/doom-bridge/bpf_test.go`

3. **Unit: Binary Frame Encoding**
   - Test: `TestEncodeDoomFrame()` — Input: 64KB buffer, Output: `[]byte` with `tagScreen` prefix
   - Verify: First byte is 0x01, next 4 bytes are big-endian uint32 size, data follows
   - File: `cmd/doom-bridge/protocol_test.go`

4. **Unit: WebSocket Upgrade**
   - Test: `TestWebSocketUpgrade()` — Simulate HTTP upgrade request, verify 101 response
   - Use standard `httptest.NewServer()` with `test.DialWebsocket()`
   - File: `cmd/doom-bridge/websocket_test.go`

5. **Unit: Client Broadcast**
   - Test: `TestBroadcastScreenFrame()` — Write frame to 5 client connections (mock), verify all receive same data
   - Verify: No write errors, all clients still connected
   - File: `cmd/doom-bridge/broadcast_test.go`

6. **Unit: Keyboard Input Parsing**
   - Test: `TestParseKeyboardFrame()` — Input: `[]byte{0x02, 0x00, 0x00, 0x00, 0x01, ...}`, verify keycode extraction
   - Test: Invalid frames rejected (wrong tag, truncated)
   - File: `cmd/doom-bridge/protocol_test.go`

7. **Unit: Keyboard Map Write**
   - Test: `TestWriteKeyboardMap()` — Write keycode to KBD_MAP, verify BPF map updated
   - File: `cmd/doom-bridge/bpf_test.go`

8. **Integration: Full WebSocket Loop**
   - Test: `TestScreenStreamingEndToEnd()` — Real WebSocket client, mock BPF maps, verify 10 frames received
   - Duration: ~100ms
   - File: `cmd/doom-bridge/integration_test.go`

9. **Integration: Concurrent Clients**
   - Test: `TestConcurrentClients()` — 10 WebSocket clients simultaneously, each reads 100 frames
   - Verify: No race conditions, all clients see same data
   - File: `cmd/doom-bridge/integration_test.go`

10. **Load Test: Frame Throughput**
    - Test: `BenchmarkFrameEncoding()` — Encode 10,000 frames, measure time
    - Target: <100µs per frame
    - File: `cmd/doom-bridge/protocol_benchmark_test.go`

**Coverage Target:** 80%+ for all production code (main.go, bpf.go, protocol.go)

### Security Checklist for Doom-Bridge (WS1)

1. **WebSocket Origin Validation**
   - [ ] Add CORS origin whitelist to `upgrader.CheckOrigin`
   - Current: Line 83 shows `upgrader` with no explicit check
   - Fix: `upgrader.CheckOrigin = func(r *http.Request) bool { return isAllowedOrigin(r.Header.Get("Origin")) }`

2. **Frame Injection Prevention**
   - [ ] Validate incoming keyboard frames: Must start with `tagKbd = 0x02`
   - [ ] Reject frames >8 bytes (keycode is uint16, max data)
   - [ ] Reject frames with invalid keycodes (outside valid Doom range)

3. **BPF Map Boundary Checks**
   - [ ] SCREEN_MAP: Verify size is exactly 64000 bytes
   - [ ] KBD_MAP: Verify key/value sizes before write
   - [ ] STATS_MAP: Verify keys 0–15 exist before read

4. **Panic Prevention**
   - [ ] All BPF map reads wrapped in `if err != nil` check
   - [ ] Zero-length read attempts logged, not panicked
   - [ ] Client disconnects don't crash server (test 100 rapid connects/disconnects)

5. **Denial of Service (DoS) Prevention**
   - [ ] Rate limit: Max 1000 frames/sec per client (kill slow consumers)
   - [ ] Buffer management: Track broadcast queue size per client, drop client if >100 frames queued
   - [ ] Connection limits: Max 1000 concurrent clients per instance

6. **Information Disclosure**
   - [ ] Error messages never leak BPF map paths, kernel internals
   - [ ] All errors logged to stderr/zerolog, never returned to client
   - [ ] Metrics endpoint (`GET /metrics`) is Prometheus-native, no secrets

### Dependency Analysis (Go Modules)

**Required additions for WS1:**

```
ALREADY IN go.mod:
  github.com/gorilla/websocket v1.5.3  ✅ (for WebSocket)
  golang.org/x/sys v0.40.0             ✅ (for syscalls, BPF_OBJ_GET)
  github.com/rs/zerolog v1.31.0        ✅ (for logging)
  github.com/prometheus/client_golang  ✅ (for /metrics endpoint)

NEEDED FOR WS1:
  None, if using syscall approach for BPF maps ✅
  OR
  github.com/cilium/ebpf (if using lib approach) — NOT YET IN go.mod

VERIFICATION:
  $ grep -i "cilium" go.mod  # Should be empty if syscall approach
```

**Recommendation:** Confirm bpf.go uses syscalls, not cilium/ebpf, to avoid extra dependency.

### Estimated Test Coverage for WS1

| Component | Lines | Coverage Target | Priority |
|-----------|-------|-----------------|----------|
| `main.go` | ~489 | 75% | P0 |
| `bpf.go` | ~295 | 80% | P0 |
| `protocol.go` (new) | ~150 | 90% | P0 |
| `websocket.go` (new) | ~100 | 85% | P1 |
| **Total** | **~1034** | **80%** | — |

**Target:** 80% coverage before merge = ~830 lines tested

---

### Quick Wins from TODO.md (10 Items, All Applicable to WS1+WS3)

**File locations for each quick win:**

1. **Pin gosec version** (10 min)
   - **File:** `/sessions/funny-lucid-lamport/mnt/unheaded/Makefile`
   - **Change:** Line with `gosec@master` → `gosec@v2.21.0` + remove `-no-fail`
   - **Impact:** Enforce security linting; catch unsafe patterns in doom-bridge before merge

2. **Add `MaxHeaderBytes: 1 << 20`** (5 min)
   - **Files:**
     - `/sessions/funny-lucid-lamport/mnt/unheaded/cmd/doom-bridge/main.go` (new http.Server)
     - Any other service with `http.Server{}`
   - **Change:** Add `MaxHeaderBytes: 1 << 20` (1 MB limit) to all `http.Server{}` instances
   - **Impact:** Prevent header injection attacks; doom-bridge WebSocket upgrade validates headers

3. **Add context to rate limiter cleanup** (15 min)
   - **Files:** `/sessions/funny-lucid-lamport/mnt/unheaded/pkg/wotan-client/client.go` (or wherever rate limiter lives)
   - **Change:** Pass `context.Context`, select on `ctx.Done()` in cleanup loop
   - **Impact:** Graceful shutdown; doom-bridge can drain WebSocket clients on SIGINT

4. **Add exponential backoff** (20 min)
   - **File:** `/sessions/funny-lucid-lamport/mnt/unheaded/pkg/wotan-client/client.go`
   - **Change:** In `pollMessages` error path, add exponential backoff (e.g., retry with 1s, 2s, 4s delays, max 30s)
   - **Impact:** Resilience; if BPF maps become unavailable, doom-bridge reconnects gracefully

5. **Replace `UnixNano()` IDs** (10 min)
   - **Files:** Any service generating request IDs
   - **Change:** Replace `time.Now().UnixNano()` with atomic counter + timestamp combo (see micromanager pattern)
   - **Impact:** No clock skew collisions; doom-bridge trace IDs are monotonic

6. **Add X-Request-ID middleware** (20 min)
   - **Files:**
     - `/sessions/funny-lucid-lamport/mnt/unheaded/cmd/doom-bridge/main.go`
     - `/sessions/funny-lucid-lamport/mnt/unheaded/pkg/telemetry/middleware.go` (new or existing)
   - **Change:** Generate UUID for each HTTP request, propagate in context, log with every message
   - **Impact:** Request tracing; doom-bridge WebSocket streams are traceable end-to-end

7. **Remove dead `BroadcastJSON` method** (5 min)
   - **File:** `/sessions/funny-lucid-lamport/mnt/unheaded/cmd/doom-bridge/websocket.go` (or main.go)
   - **Change:** Delete unused method (search codebase first to confirm no callers)
   - **Impact:** Code cleanliness; reduces confusion in doom-bridge WebSocket handler

8. **Gitignore dev artifacts** (5 min)
   - **File:** `/sessions/funny-lucid-lamport/mnt/unheaded/.gitignore`
   - **Change:** Add lines:
     ```
     churn_analysis.awk
     *-results.txt
     PROJECT_TREE.txt
     ```
   - **Impact:** Keep repo clean; WS1/WS3 profiling won't spam git status

9. **Migrate timeguru to zerolog** (30 min)
   - **Files:** All `log.Printf()` or `fmt.Println()` calls in timeguru
   - **Change:** Replace with `zerolog.Logger` (already imported in other services)
   - **Impact:** Consistency; doom-bridge and all services log same format for trace correlation

10. **Split HTTP client timeouts** (15 min)
    - **Files:**
      - `/sessions/funny-lucid-lamport/mnt/unheaded/cmd/doom-bridge/main.go`
      - Any place creating `*http.Client`
    - **Change:** Use 5s timeout for control plane ops (health checks), 30s for streaming (WebSocket upgrade)
    - **Impact:** WS1 doesn't timeout on slow BPF reads; other services don't hang on I/O

**Total Time for All 10 Quick Wins:** ~2.5 hours (can be parallelized)

**Applicability to WS1/WS3:**
- Items 1, 2, 3, 4, 6, 7, 10 directly benefit doom-bridge
- Items 5, 8, 9 are global improvements that reduce WS1/WS3 integration friction

---

## BLACKMAGE ASSESSMENT — Attack Surface of WS1+WS3

### WebSocket Attack Vectors (WS1)

#### 1. Origin Bypass (CVE-like)
**Threat:** Attacker crafts WebSocket request from evil.com, targeting ws://doom-bridge.local:6660

**Current Defense:** None visible in skeleton code (no origin check in line 83)

**Exploit Path:**
```http
GET / HTTP/1.1
Host: doom-bridge.local:6660
Upgrade: websocket
Origin: evil.com  ← Forged, but upgrader doesn't validate
Sec-WebSocket-Key: ...
```

**Impact:** CRITICAL if dashboard is public, MODERATE if internal-only

**Mitigation:**
```go
upgrader.CheckOrigin = func(r *http.Request) bool {
    origin := r.Header.Get("Origin")
    // Whitelist: localhost, internal subnet only
    return strings.HasPrefix(origin, "http://localhost:") ||
           strings.HasPrefix(origin, "http://10.10.10.")
}
```

**Verdict:** MUST FIX before WS1 launch

---

#### 2. Frame Injection (Keyboard Spoofing)
**Threat:** Attacker sends crafted keyboard frames to simulate user input

**Attack:**
```javascript
// In browser console on any page with doom-bridge connected
ws.send(new Uint8Array([0x02, 0xFF, 0xFF, ...]))  // Inject keycode
```

**Impact:** HIGH (can crash Doom if invalid keycode, or spam inputs)

**Mitigation:**
- Validate keycode range: Doom accepts 0x00–0xFF (256 keycodes)
- Drop frames with invalid tag (not 0x02)
- Rate limit: Max 100 keycodes/sec per client

**Test Case:** Send 1000 invalid frames/sec, verify bridge doesn't crash

---

#### 3. Memory Exhaustion (Broadcast Queue Bomb)
**Threat:** Attacker opens 1000 WebSocket connections, doesn't read data (slow consumer)

**Attack:**
```go
// Attacker sends slowloris-style incomplete reads
for i := 0; i < 1000; i++ {
    ws := new WebSocket(...)
    ws.onopen = () => { /* never call ws.read() */ }
}
```

**Impact:** HIGH (bridge allocates 64KB frame per client per broadcast; 1000 clients = 64MB/sec, OOM in seconds)

**Mitigation:**
- Add per-client output buffer limit: Drop client if queue > 5 frames
- Add global client limit: Max 1000 concurrent clients
- Monitor heap; GC if > 80% usage

**Test Case:** `TestSlowConsumerDoS()` — Connect 100 clients, don't read, verify server stays responsive

---

#### 4. Ping/Pong Amplification
**Threat:** Attacker sends high-frequency WebSocket pings, forcing server to pong repeatedly

**Attack:**
```javascript
setInterval(() => ws.send(new Uint8Array([0xFF])), 10)  // Ping every 10ms
```

**Impact:** MEDIUM (CPU spike, but not memory bomb)

**Mitigation:**
- Automatic ping/pong with timeout (SetReadDeadline/WriteDeadline)
- If client doesn't respond to ping within 30s, close

**Test Case:** `TestPingTimeout()` — Send pings, don't pong, verify client closes after 30s

---

### BPF Map Access Threat Model (WS1+WS3)

#### 5. Privilege Escalation (Root-to-User)
**Threat:** Can doom-bridge (running as root or with CAP_SYS_ADMIN) leak kernel data?

**Current Risk:**
- doom-bridge reads SCREEN_MAP, STATS_MAP directly from `/sys/fs/bpf/...`
- If BPF maps aren't properly isolated, bridge might read other processes' memory

**WS3 Impact:** Injector script runs as root (Step 1.2 uses `sudo`), could see kernel state

**Mitigation:**
- Verify BPF map permissions: `ls -la /sys/fs/bpf/unheaded/doom-ring/maps/`
  - Should be `600` (owner only) or `640` (owner + group)
- Verify bridge doesn't run as root if possible (use capability drop)
- Run bridge in restricted namespace if possible

**Verdict:** Check deployment; if bridge is not root, risk is LOW

---

#### 6. Timing Side-Channel (WS3 Burst Injection)
**Threat:** Can an attacker measure timing differences in BPF bounce cycles to infer CPU state?

**Attack:** WS3 measures LAST_BOUNCE_NS (Step 2.6). If attacker controls packet timing, they might:
- Measure CPU cache hits vs misses via bounce timestamps
- Infer Doom CPU state (is it executing at tick, or sleeping?)

**Impact:** LOW (information is already visible in STATS map counters)

**Mitigation:**
- Don't expose LAST_BOUNCE_NS to unprivileged clients
- Only publish aggregate metrics (avg bounce time), not per-packet timestamps

**Verdict:** ACCEPTABLE for internal dev tool; MUST restrict if exposed externally

---

#### 7. RAM_MAP Read (WS3 Only)
**Threat:** WS3 profiling script reads CPU_MAP, RAM_MAP directly. Does it leak customer data?

**Architecture Check:** CLAUDE.md line ~92: "Zero customer data access (architectural isolation enforced)"

**WS3 Execution:** Script runs in monad0 namespace, isolated from customer workloads ✅

**But Risk:** If Doom ever runs customer code, RAM_MAP contains Doom's memory (320x200 framebuffer + heap). This is Doom's private data, not customer data.

**Verdict:** SAFE if Doom is always a demo; DANGEROUS if used for customer workloads (can't do that anyway)

---

#### 8. Syscall-Based BPF Access (WS1 bpf.go)
**Threat:** Does bpf.go's syscall approach (BPF_OBJ_GET) leak secrets?

**Check:** Step 2.6 of WS3 shows:
```python
struct.pack_into("=Q", attr, 0, ctypes.addressof(path_buf))
return self.libc.syscall(321, 7, ctypes.byref(attr), 120)  # 321 = bpf()
```

This is `BPF_OBJ_GET` syscall, directly reading pinned maps. Secure if:
- Maps pinned with correct permissions (owner only, or admin group)
- bpf.go runs with appropriate capability (CAP_BPF or CAP_SYS_ADMIN)

**Verdict:** SECURE if permissions are correct; verify in NixOS config (Step 4 of WS1)

---

### WS1 Doom-Bridge Threat Posture

| Vector | Severity | Mitigation | Status |
|--------|----------|-----------|--------|
| Origin bypass | CRITICAL | Whitelist origins | NOT IN SKELETON |
| Frame injection | HIGH | Validate keycodes, rate limit | NOT HARDENED |
| Memory bomb (slow consumer) | HIGH | Per-client queue limit | NOT IMPLEMENTED |
| Ping/pong amplification | MEDIUM | Ping timeout | Not visible |
| BPF privilege leak | LOW | Verify namespace isolation | Depends on deploy |
| RAM_MAP side-channel | LOW | Don't expose timestamps | Not exposed yet |
| Syscall BPF access | LOW | Verify permissions | To verify |

**Overall Threat Posture:** EXPOSED (needs hardening)

**Go/No-Go Impact:** WS1 can proceed, but **MUST implement origin whitelist + frame validation + buffer limits before public access**

---

### WS3 Scaling Threat Posture

#### Burst Injection Risks (WS3 Phases 4-5)

**Threat 1: Kernel Crash via Malformed Packets**

**Attack:** Inject packets with invalid IPv6/HBH headers, cause XDP verifier panic

**Mitigation:**
- WS3 packet builder (Step 5 Go code, line 900+) constructs valid IPv6 + HBH headers ✅
- Verify header checksums before send

**Verdict:** LOW RISK (packets are well-formed in battle plan)

---

**Threat 2: DoS via Burst Rate (H2)**

**Attack:** Burst mode sends 1000+ pps. Can this crash the kernel?

**WS3 Netflix Comparison (Appendix B, line 1380+):**
- Netflix steady-state: 1,500 pps
- Aggressive Doom: 2,000 pps
- XDP hardware max: 10,000,000 pps (estimated)

**Verdict:** SAFE; we're using 0.02% of estimated XDP capacity

---

**Threat 3: Memory Corruption via Underflow Injection**

**Attack:** Craft packets that cause SCREEN_MAP/RAM_MAP underflow

**Mitigation:** Battle plan Step 4.2 checks for ROM faults; WS3 halts injection if any occur

**Verdict:** ACCEPTABLE; battle plan has fault detection

---

#### D1-D6 Campaign Readiness (WS2, but affects WS3 security)

**Lich campaigns** (CLAUDE.md ~line 140 mentions WS2) are security-focused adversarial tests.

**WS3 Impact:**
- D1–D3: Basic BPF map access attacks
- D4–D6: Advanced kernel exploitation

**Blocking Issue for WS3?** WS3 can run independently, but findings from D1–D6 will inform WS3 hardening

**Verdict:** WS3 can execute in parallel with WS2 (noted in DEV-MACHINE-AGENT-MATRIX), but D1–D3 findings should be integrated into final WS3 report

---

### Rate Limiting the Threat: Fortress vs Hardening

**Current State:** EXPOSED (no active hardening in WS1 skeleton)

**After WS1 Hardening:** HARDENING (origin check + frame validation + buffer limits)

**After WS2 Lich Findings:** FORTIFIED (kernel-level exploits blocked)

**Verdict:** Launch WS1 in HARDENING mode; don't expose to internet until FORTIFIED (after WS2 D1–D3)

---

## ARCHITECT ASSESSMENT — Infrastructure Alignment

### Systems Mind: Standard Service Template (WS1)

**Question:** Does doom-bridge fit the Unheaded service template?

**Template (from CLAUDE.md ~line 100–150):**
```
Required endpoints:
  GET /health    — Health check (200 if healthy)
  GET /ready     — Readiness probe (200 if ready)
  GET /metrics   — Prometheus metrics
  GET /api/v1/*  — Service-specific API
```

**Doom-Bridge Status:**

| Endpoint | Required | Status | Impact |
|----------|----------|--------|--------|
| `/health` | YES | NOT VISIBLE IN SKELETON | P0 |
| `/ready` | YES | NOT VISIBLE IN SKELETON | P0 |
| `/metrics` | YES | PROMETHEUS IMPORTED IN go.mod | P0 |
| `/api/v1/frame` (GET) | YES | WebSocket instead | ACCEPTABLE |
| `/api/v1/keyboard` (POST) | YES | WebSocket instead | ACCEPTABLE |

**Architecture Concern:** Doom-bridge uses WebSocket, not REST. This is correct for streaming, but:
- Health check: MUST be REST (WebSocket can't be queried by k8s liveness probe)
- Metrics: MUST be REST (Prometheus scrapes HTTP)

**WS1 Requirement:** Add to doom-bridge:
```go
// Health endpoint — check BPF maps are readable
func (b *bridge) healthHandler(w http.ResponseWriter, r *http.Request) {
    if !b.mapsAccessible() {
        http.Error(w, "BPF maps unavailable", http.StatusServiceUnavailable)
        return
    }
    w.WriteHeader(http.StatusOK)
    fmt.Fprintf(w, `{"status":"healthy"}`)
}

// Ready endpoint — check at least one client connected OR maps ready
func (b *bridge) readyHandler(w http.ResponseWriter, r *http.Request) {
    if !b.mapsAccessible() {
        http.Error(w, "Not ready", http.StatusServiceUnavailable)
        return
    }
    w.WriteHeader(http.StatusOK)
}

// Metrics endpoint — Prometheus metrics
func (b *bridge) metricsHandler(w http.ResponseWriter, r *http.Request) {
    promhttp.Handler().ServeHTTP(w, r)
}
```

**Verdict:** ARCHITECTURALLY SOUND with above additions

---

### Network Mind: Port Allocation & Gateway Routing (WS1)

**Network Design (CLAUDE.md ~line 65–75):**
```
Bridge: lxdbr0 (10.10.10.0/24)
Wotan: 10.10.10.10 (message hub)
Services: 10.10.10.20–30 (agent services)
Apps: 10.10.10.200+ (kanban, demo)
```

**Doom-Bridge Allocation:**

**Current:** Line 88 of main.go shows `--port 6660` (absolute port, not relative to 10.10.10.0)

**Question:** Should doom-bridge be:
1. A service on 10.10.10.26 (internal only)?
2. Exposed via gateway on 10.10.10.100?
3. Both?

**Battle Plan Implication:** WS1 says browser accesses dashboard at `http://localhost:8080` (Step 6.5, line 1190). This implies:
- Dashboard runs locally (or on 10.10.10.100 gateway)
- Doom-bridge accessible to dashboard WebSocket client

**Recommended Architecture:**

```
┌─────────────────────────────────────────┐
│ Gateway (10.10.10.100)                  │
│  ├─ HTTP/3 TLS termination              │
│  ├─ Route /doom → doom-bridge           │
│  └─ Serves dashboard (port 8080→443)    │
└─────────────────────────────────────────┘
                    │
         ┌──────────┴──────────┐
         │                     │
┌─────────▼─────────┐  ┌──────▼──────────┐
│ Doom-Bridge       │  │ Wotan           │
│ (10.10.10.26:6660)│  │ (10.10.10.10)   │
│ ✓ REST+WebSocket  │  │ Message bus     │
└───────────────────┘  └─────────────────┘
```

**WS1 Port Assignment:**

- **Internal:** `10.10.10.26:6660` (doom-bridge direct, for debugging)
- **External:** Gateway routes `/ws/doom` → `10.10.10.26:6660`
- **Dashboard Client:** Connects via `wss://gateway.local/ws/doom` (over TLS)

**Go/No-Go:** Port allocation is CORRECT if gateway routing is configured

---

### Infrastructure Mind: Container Definition (WS1)

**Requirement (CLAUDE.md ~line 170–230):** Every service needs NixOS container definition

**Current Status:** doom-bridge skeleton exists, but no `nix/containers/doom-bridge.nix`

**WS1 Required Config:**

```nix
# nix/containers/doom-bridge.nix
{ config, pkgs, ... }:

{
  systemd.services.doom-bridge = {
    description = "Doom-over-IPv6 → WebSocket Bridge";
    wantedBy = [ "multi-user.target" ];
    after = [ "monad0-cpu.service" ];  # Depends on eBPF service

    serviceConfig = {
      ExecStart = "${pkgs.doom-bridge}/bin/doom-bridge --port 6660";
      Restart = "always";
      RestartSec = "5s";

      # Security hardening
      CapabilityBoundingSet = [ "CAP_BPF" "CAP_PERFMON" ];
      AmbientCapabilities = [ "CAP_BPF" ];
      NoNewPrivileges = true;
      PrivateTmp = true;
      ProtectSystem = "strict";
      ProtectHome = true;
      ReadOnlyPaths = [ "/etc" "/usr" ];
      ReadWritePaths = [ "/sys/fs/bpf/unheaded/doom-ring/maps" ];  # BPF maps

      PrivateDevices = true;
      ProtectKernelTunables = true;
      RestrictRealtime = true;
      RestrictNamespaces = true;
      SystemCallFilter = [
        "@system-service"
        "~@privileged"
        "~@io-uring"  # No async I/O needed
      ];
    };
  };

  # Network config
  networking.firewall.enable = true;
  networking.firewall.allowedTCPPorts = [ 6660 ];

  # Logging
  systemd.services.doom-bridge.serviceConfig.StandardOutput = "journal";
  systemd.services.doom-bridge.serviceConfig.StandardError = "journal";
}
```

**Capabilities Analysis:**
- `CAP_BPF`: Read BPF maps
- `CAP_PERFMON`: Not needed unless using perf (probably not)
- `CAP_SYS_ADMIN`: Needed for BPF_OBJ_GET syscall; already covered by CAP_BPF

**Verdict:** NixOS definition is **REQUIRED** before WS1 merge

---

### Security Mind: Zero Customer Data Access (WS1)

**CLAUDE.md Doctrine:** "Zero customer data access (architectural isolation enforced)" (line ~92)

**WS1 Question:** Does doom-bridge access any customer data?

**Analysis:**

| Data Source | Purpose | Customer Data? | Mitigation |
|-------------|---------|----------------|-----------|
| SCREEN_MAP | Framebuffer display | Demo only | Doom is internal demo, not customer workload ✅ |
| RAM_MAP | CPU memory state | Demo only | Doom internal state, not customer data ✅ |
| STATS_MAP | Metrics (packets, insns) | No PII | Aggregate counters only ✅ |
| KBD_MAP | Keyboard input | Demo only | Doom keycodes, not customer input ✅ |

**Architectural Guarantee:**
- doom-bridge only talks to `/sys/fs/bpf/unheaded/doom-ring/` (isolated BPF namespace)
- doom-bridge never talks to wotan (no message bus, no data leakage)
- doom-bridge never talks to customer workload containers
- Doom CPU is isolated in monad0 namespace (never runs customer code)

**Verdict:** SAFE; doom-bridge respects Zero Customer Data doctrine ✅

---

### Protocol Mind: Doom-Bridge ↔ Monad Wire Format (WS1)

**Question:** How does doom-bridge protocol relate to Monad wire format?

**Monad Context (CLAUDE.md ~line 60–70):**
```
Monad: Unified state management service
Wotan: Message bus (ring buffer + protocol RAM)
Trace-collector: Bridges eBPF → Wotan
```

**Doom-Bridge Purpose:**
- Bridge: BPF maps → WebSocket (human-readable frames)
- NOT: BPF → Monad message format

**Classification:** doom-bridge is a **demo utility**, not a protocol service

**Does it need to emit Monad format?**
- Current design: Binary WebSocket frames (0x01 for screen, 0x02 for keyboard)
- Not compatible with Monad wire format (which is protobuf/gRPC via Wotan)

**Recommendation:** Keep doom-bridge as standalone demo utility. Don't force Monad compatibility; it's not a microservice in the architecture, it's a reference implementation.

**Verdict:** ARCHITECTURAL CLARITY NEEDED — Document that doom-bridge is a demo tool, separate from core services

---

### WS5 Preparation: Reusable Patterns from Doom-Bridge

**WS5 (Return to Core) is packet tracing eBPF programs** (DEV-MACHINE-AGENT-MATRIX, line 104+)

**What patterns from WS1 doom-bridge should be reused?**

1. **BPF Map Reading (bpf.go)**
   - Doom-bridge reads pinned BPF maps efficiently
   - WS5 trace-collector will also read BPF ring buffers
   - Pattern: `openMaps()` + `readLoop()` with error handling

2. **Streaming Architecture**
   - Doom-bridge streams frames to WebSocket clients
   - WS5 trace-collector streams spans to Wotan topic
   - Pattern: Single goroutine reads, broadcasts to subscribers

3. **Binary Protocol**
   - Doom-bridge uses tagScreen = 0x01, tagKbd = 0x02
   - WS5 should define trace packet tags (0x10 for span, 0x11 for metrics, etc.)
   - Pattern: 1-byte tag + size prefix + binary payload

4. **Rate Limiting**
   - WS1 hardening adds per-client queue limits
   - WS5 should rate-limit trace ingestion (don't spam Wotan)
   - Pattern: Backpressure handling

**Verdict:** WS1 becomes a prototype for WS5 streaming architecture ✅

---

### Repository Location: cmd/ vs examples/doom/

**Question:** Should doom-bridge live in `cmd/` (permanent service) or `examples/doom/` (temporary demo)?

**Analysis:**

**Arguments for `cmd/`:**
- It's a permanent reference implementation
- Demonstrates BPF map reading for future services
- Will be packaged in official binary releases
- Reduces friction for users who want to browse eBPF data

**Arguments for `examples/doom/`:**
- It's tied to the Doom demo (not core infrastructure)
- Might not be maintained long-term
- Keeps cmd/ reserved for core Unheaded services (daemon, captain, architect, etc.)

**CLAUDE.md Architecture (line ~20–30):**
```
cmd/:
  ├─ unheaded-daemon        (control plane)
  ├─ trace-collector        (eBPF → Wotan bridge)
  ├─ dashboard-backend      (metrics + WebSocket)
  ├─ kanban-app             (meta moment)
  └─ doom-bridge            ??? (demo or core?)
```

**Recommendation:** Keep in `cmd/` because:
1. It's a canonical example of BPF map integration
2. Users need it to understand eBPF workflows
3. WS5 trace-collector will follow the same pattern
4. It's part of self-hosting proof ("Meta Moment")

**Verdict:** `cmd/doom-bridge/` is correct home ✅

---

## UNIFIED VERDICT: WS1 + WS3 Readiness Assessment

### GO/NO-GO Decision Matrix

#### WS1 GO/NO-GO: **GO** (with mandatory hardening)

**Conditions:**
- [ ] Origin whitelist implemented (10 min)
- [ ] Frame validation added (15 min)
- [ ] Per-client buffer limit (15 min)
- [ ] /health and /ready endpoints (20 min)
- [ ] NixOS container definition (20 min)
- [ ] 80%+ test coverage (4 hours)

**Estimated Hardening Time:** ~5 hours

**Blocker Status:** None (all blockers are implementation tasks, not architectural)

**Verdict:** **GO for WS1** — Can begin implementation today, merge criteria above

---

#### WS3 GO/NO-GO: **GO** (independent of WS1)

**Conditions:**
- [ ] eBPF infrastructure verified (Phase 1, 15 min)
- [ ] BPF maps accessible (Step 1.1–1.4, 30 min)
- [ ] Python injector ready (scripts/doom/inject.py exists, assumed working)
- [ ] Linux environment with XDP support (VERIFICATION NEEDED)

**Blocker Status:** WS3 execution depends on actual Linux environment (not present in current session)

**Verdict:** **GO for WS3 planning** — Can create detailed profiling scripts today, execution blocked on hardware availability

---

### Top 3 Cross-Functional Risks

#### Risk 1: BPF Map Permission Isolation (Security)
**Severity:** HIGH
**Scope:** WS1 + WS3 both depend on this

**Issue:** If `/sys/fs/bpf/unheaded/doom-ring/maps/` is world-readable, unprivileged users can read CPU/RAM state

**Detection:** Run `ls -la /sys/fs/bpf/unheaded/doom-ring/maps/` → If mode is 644, FAIL

**Mitigation:**
```bash
sudo chmod 640 /sys/fs/bpf/unheaded/doom-ring/maps/*
sudo chgrp unheaded-admins /sys/fs/bpf/unheaded/doom-ring/maps/*
```

**WS1 Impact:** doom-bridge must verify this at startup (health check)

**WS3 Impact:** Profiling scripts must run with correct group membership

**Timeline:** Verify in deployment (Phase 0 of WS1)

---

#### Risk 2: WebSocket Origin Bypass (Security)
**Severity:** CRITICAL
**Scope:** WS1 only, but high impact if dashboard is public

**Issue:** If origin whitelist not implemented, attacker can open ws:// connection from evil.com, inject keyboard frames

**Detection:** Manual test in browser console after WS1 deploy

**Mitigation:** Implement CheckOrigin before public access (noted in WS1 hardening above)

**Timeline:** Must be done before step 6 of WS1 ("Browser Verification")

---

#### Risk 3: XDP Bounce Cycle Timing (Performance)
**Severity:** MEDIUM
**Scope:** WS3 hypothesis validation

**Issue:** H1 assumes 3ms bounce cycle is Python socket.send() overhead, not XDP hard limit. If H1 is wrong, WS3 cannot achieve 15+ fps with steady-state injection

**Detection:** Phase 2 of WS3 (STATS timestamp instrumentation) should reveal if bounce time varies with packet rate

**Mitigation:** If H1 rejected, shift to burst mode (Phase 4) or Go injector (Phase 5) early

**Timeline:** Will be known by WS3 Phase 2 completion (15 minutes into execution)

---

### Top 3 Quick Wins (Security + Code Quality + Architecture Alignment)

#### Quick Win 1: Origin Whitelist for doom-bridge (WS1 Blocker)
**Time:** 10 minutes
**Impact:** Prevents WebSocket hijacking from external origins
**Files:** `cmd/doom-bridge/main.go` (add 5 lines to upgrader config)
**Success Criteria:** Manual test with `curl -i -N -H "Connection: Upgrade" ... -H "Origin: evil.com"` → HTTP 403

```go
upgrader.CheckOrigin = func(r *http.Request) bool {
    origin := r.Header.Get("Origin")
    if origin == "" {
        return true  // Browser doesn't send origin for same-origin requests
    }
    return strings.HasPrefix(origin, "http://localhost") ||
           strings.HasPrefix(origin, "http://127.0.0.1") ||
           strings.HasPrefix(origin, "http://10.10.10.")
}
```

**Priority:** P0 (blocking WS1 go-live)

---

#### Quick Win 2: Pin gosec + Add security scanning to Makefile (Both)
**Time:** 15 minutes
**Impact:** Catches unsafe patterns in both WS1 doom-bridge and WS3 profiling scripts
**Files:** `Makefile` (update gosec rule), `.golangci.yml`
**Success Criteria:** `make lint` runs gosec v2.21.0, finds any unsafe patterns

**Pattern to enforce:**
- No hardcoded secrets
- No `unsafe` pointer arithmetic without safety comments
- No unbounded loops in BPF map operations

**Priority:** P0 (apply before all merges)

---

#### Quick Win 3: Add /health and /ready endpoints to doom-bridge (WS1 Architecture)
**Time:** 20 minutes
**Impact:** Makes doom-bridge a proper Unheaded service; enables k8s probes
**Files:** `cmd/doom-bridge/main.go` (add 30 lines for handlers)
**Success Criteria:**
- `curl http://localhost:6660/health` → 200 `{"status":"healthy"}`
- `curl http://localhost:6660/ready` → 200 if BPF maps accessible, 503 if not

```go
func (b *bridge) setupHealthEndpoints() {
    http.HandleFunc("/health", b.healthHandler)
    http.HandleFunc("/ready", b.readyHandler)
    http.HandleFunc("/metrics", promhttp.Handler().ServeHTTP)
}

func (b *bridge) healthHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

func (b *bridge) readyHandler(w http.ResponseWriter, r *http.Request) {
    if !b.mapsAccessible() {
        w.WriteHeader(http.StatusServiceUnavailable)
        return
    }
    w.WriteHeader(http.StatusOK)
}

func (b *bridge) mapsAccessible() bool {
    // Try to read 1 byte from each map; if any fails, return false
    _, err := b.screenMap.LookupBytes([]byte{0, 0, 0, 0})
    return err == nil
}
```

**Priority:** P0 (architectural requirement)

---

## Final Recommendations for S33 Execution

### For Agent A (Developer/Architect, WS1 Doom-Bridge)

1. **Day 1 (Feb 24):** Implement WS1 foundation
   - Complete Phase 1 (environment verification)
   - Implement /health, /ready, /metrics endpoints
   - Add origin whitelist
   - Write 10 unit tests from TDD checklist above

2. **Day 2 (Feb 25):** WS1 hardening + protocol
   - Implement frame validation (keyboard input)
   - Add per-client buffer limits
   - Write integration tests
   - Create NixOS container definition
   - Merge to main with 80%+ coverage

3. **Day 3 (Feb 26):** WS1 documentation + browser verification
   - Test in browser (Step 6.5 of battle plan)
   - Document doom-bridge as demo utility (not core service)
   - Create `docs/services/doom-bridge.md`
   - Prepare WS4 output for Captain

### For Agent B (Developer/Scientist, WS3 Scaling)

1. **Day 1 (Feb 24):** Baseline measurement (Phase 1)
   - Verify infrastructure (veth50p, BPF maps, monad0)
   - Run baseline injection (3000µs, 500 packets)
   - Document baseline metrics

2. **Day 2 (Feb 25):** Profiling (Phases 2–3)
   - Add STATS timestamp instrumentation (if eBPF writable)
   - Run delay profile tests (3000–500µs)
   - Identify minimum safe delay or confirm all safe

3. **Day 2–3:** Burst validation (Phase 4–5)
   - Test burst injection (100–500 batch)
   - Build/test Go injector if Python bottleneck confirmed
   - Run sustained 60s test

### For Agent C (Engineer/Coordinator, Quick Wins)

1. **Parallel during WS1+WS3:** Execute quick wins 1–10
   - Priority: 1 (origin), 2 (gosec), 3 (health/ready)
   - Each ~15 min; do 3–4 per day alongside A/B

2. **End of WS1/WS3 (Feb 27–28):** Integration
   - Verify all quick wins merged
   - Run `make test` and `make lint` (all passing)
   - Document for WS4

### Sprint Schedule Summary

```
Feb 23 (TODAY):
  ✓ Hardening (all phases) → DONE
  ✓ This assessment → DONE
  → GO for WS1 + WS3

Feb 24:
  Agent A: WS1 Phase 1 (env verify) + Unit tests
  Agent B: WS3 Phase 1 (baseline)
  Agent C: Quick wins 1–3

Feb 25:
  Agent A: WS1 Phases 2–3 (hardening + protocol)
  Agent B: WS3 Phases 2–3 (profiling)
  Agent C: Quick wins 4–6

Feb 26:
  Agent A: WS1 Phase 4 (browser verify + docs)
  Agent B: WS3 Phases 4–5 (burst + Go injector)
  Agent C: Quick wins 7–10

Feb 27–28:
  Integration, final testing, merges
  Prepare input for WS4 (Captain)

Mar 1–7:
  WS2 (Lich D1–D6, BlackMage) + WS4 (Docs, Captain) in parallel
  WS5 kickoff planning

Mar 8:
  Round Table reconvenes
  WS5 packet tracing pipeline architecture review
```

---

## Conclusion: SYNTHESIS OF THREE MINDS

### Developer Says:
"Code is ready to build. We have Go patterns, WebSocket library, BPF utilities. Missing pieces are small (health endpoints, origin whitelist, test coverage). I can have WS1 skeleton to working service in 2–3 days with 80%+ coverage."

### BlackMage Says:
"Current surface is EXPOSED. WebSocket origin bypass, frame injection, memory bomb risks are real but fixable in <1 hour. WS3 is safer (internal profiling). Launch WS1 in HARDENING mode; don't expose publicly until Lich D1–D3 pass. After D1–D3, we move to FORTIFIED."

### Architect Says:
"Architecture is sound. Doom-bridge fits the service template once /health and /ready are added. Port allocation is correct (10.10.10.26:6660). NixOS definition required. Zero customer data access is maintained. This is a demo utility, not a core service—document accordingly. Patterns reusable for WS5 trace-collector."

### Combined Verdict:

**WS1 GO/NO-GO: GO**
- Hardening path is clear
- Risks are manageable
- Can launch in HARDENING mode; FORTIFY after WS2

**WS3 GO/NO-GO: GO**
- Independent of WS1
- Depends on hardware availability
- Execution blocked pending Linux environment; planning can proceed

**Confidence:** 90% (only risk: XDP bounce cycle hypothesis in WS3; will be known by Day 1)

---

**Signed:**
Developer, BlackMage, Architect (Four-Mind Consensus)
**Date:** February 23, 2026 18:00 UTC
**Status:** READY FOR EXECUTION

⚔️🛡️🏰
