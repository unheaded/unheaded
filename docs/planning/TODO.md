# Unheaded Kingdom — Security & Code Review TODO

**Generated:** February 17, 2026 (Full codebase review — 10 parallel agents)
**Last Updated:** February 23, 2026 (Post security P0 hardening commit verification)
**Source:** `fix(security): P0 hardening` commit a6b0b73 + parallel agent review
**Workspace snapshot:** Jan 27 files + verified commit diffs from git log

---

## LEGEND

- ✅ = Verified FIXED (code reviewed, confirmed in workspace files)
- 🔴 = P0 — Production blocker
- 🟠 = P1 — Deploy blocker
- 🟡 = P2 — Fix before GA
- 🟢 = P3 — Polish
- 📦 = Missing implementation
- ⏸️ = Blocked (needs Linux/eBPF env)

---

## P0 — PRODUCTION BLOCKERS

### ✅ FIXED — Security P0 Hardening (commit a6b0b73)

| # | Finding | Status | Verification |
|---|---------|--------|-------------|
| 1 | CORS wildcard echo — attacker origin reflected back | ✅ FIXED | `middleware.go:17-24` — explicit origin whitelist (localhost:8080/3000 only), preflight returns 403 for unlisted origins |
| 2 | CSP `unsafe-inline` in script-src | ✅ FIXED | `middleware.go:257` — `script-src 'self'` only (unsafe-inline removed, kept in style-src which is acceptable) |
| 3 | HSTS disabled (commented out) | ✅ FIXED | `middleware.go:260` — `max-age=31536000; includeSubDomains` active |
| 4 | WebSocket `CheckOrigin: return true` — cross-origin hijacking | ✅ FIXED | `server.go:339-346` — origin validated against whitelist, 403 on mismatch. `isOriginAllowed()` deny-by-default when config empty (falls back to DefaultAllowedOrigins) |
| 5 | Timeguru path traversal via `filepath.Base/Dir` on URL paths | ✅ FIXED | `main.go:322-329` — uses `strings.TrimPrefix` + `strings.SplitN`, comment explains traversal risk |
| 6 | Micromanager concurrent map race on `subscriptions` | ✅ FIXED | `service.go:21` — `subMu sync.RWMutex` added, all map access guarded (write: L55-57, read: L257-259) |
| 7 | wotan-client send-on-closed-channel panic | ✅ FIXED | `client.go:58-94` — `safeChannel` with `done` signal, `sync.Once` close, `send()` selects on `done` before sending |
| 8 | HTTP servers missing `IdleTimeout` | ✅ FIXED | `main.go:144` — `IdleTimeout: 60 * time.Second` on timeguru server |

### REMAINING P0s

| # | Finding | File(s) | Impact |
|---|---------|---------|--------|
| 9 | 🔴 eBPF programs are userspace stubs — won't compile to BPF target | `ebpf/packet-marker/`, `ebpf/flow-tracker/`, `ebpf/latency-probe/` | Core product feature non-functional. Aya programs need real kernel-target compilation. ⏸️ Blocked on Linux env (B1) |
| 10 | ✅ Nix cross-container `requires=wotan.service` — circular dep risk | `nix/containers/*.nix` | FIXED: commit 0c2e1da replaced `requires` with `wants + after` across all 14 nix container files |
| 11 | ✅ `gosec@master` unpinned in CI | `Makefile` / CI config | FIXED: Makefile pins `gosec@v2.21.0` |
| 12 | ✅ gosec `-no-fail` flag in CI | `Makefile` / CI config | FIXED: Verified no `-no-fail` flag in Makefile |
| 13 | 🔴 No release signing or SBOM generation | CI/CD pipeline | No supply chain verification for distributed binaries |
| 14 | 🔴 Captain service stores data in `/tmp` | `services/captain/` | Sensitive decision data in world-readable temp dir. Commit says fixed with configurable data dir — **NEEDS VERIFICATION after mount refresh** |
| 15 | ✅ No `MaxHeaderBytes` on kanban-app HTTP server | `cmd/kanban-app/main.go` | FIXED: All 14+ HTTP servers have `MaxHeaderBytes: 1 << 20` |

---

## P1 — DEPLOYMENT BLOCKERS

| # | Finding | File(s) | Impact |
|---|---------|---------|--------|
| 16 | 🟠 No authentication on ANY endpoint | All services | Every API is open. Need at minimum mTLS between services + API key for external |
| 17 | 🟠 Rate limiter uses X-Forwarded-For (spoofable) | `middleware.go:211-219` | Attacker bypasses rate limiting by spoofing XFF header. Use `RemoteAddr` only when not behind trusted proxy |
| 18 | ✅ No reconnection backoff in wotan-client HTTP polling | `client.go:510-528` | FIXED: `client.go` has `backoff()` with exponential backoff (500ms to 30s), `pollMessages` uses `consecutiveFailures` counter |
| 19 | 🟠 Silent wotan failures across all services | All services | `wotan = nil` path logs warning but provides no fallback behavior — silent message loss |
| 20 | 🟠 Nix network layer missing TLS/VXLAN/gateway config | `nix/containers/`, `nix/modules/` | No encryption in transit between containers, no VXLAN overlay, no gateway routing |
| 21 | ✅ `style-src 'unsafe-inline'` still in CSP | `middleware.go:257` | FIXED: commit 31dbbe2 removed unsafe-inline from CSP, CSSOM used instead |
| 22 | ✅ No input validation on WebSocket message content | `server.go` | FIXED: commit 31dbbe2 added JSON validation for text frames + 1KB message limit |
| 23 | ✅ HTTP client timeout 30s may be too long for control plane | `client.go:169-171` | FIXED: Split timeouts: `controlPlaneTimeout = 5s`, `streamingTimeout = 30s` |
| 24 | ✅ `cleanupLoop` goroutine in rate limiter never stops | `middleware.go:148-155` | FIXED: commit 79a5215 added context cancellation with `ctx.Done()` select |
| 25 | 🟠 Double-check locking in `getOrCreateGRPCClient` has subtle race | `client.go:471-493` | RLock → RUnlock → Lock pattern has a window where another goroutine could initialize. Works with double-check but fragile |
| 26 | 🟠 No TLS on gRPC connections | `client.go`, `grpc.go` | gRPC data plane unencrypted — plaintext streaming |
| 27 | 🟠 `Connection` header check too strict | `server.go:355` | Only checks exact strings — browsers may send mixed case or additional values like `keep-alive, Upgrade` (partially handled but fragile) |

---

## P2 — FIX BEFORE GA

| # | Finding | File(s) | Impact |
|---|---------|---------|--------|
| 28 | 🟡 Go version 1.21 — update to 1.22+ | `go.mod` | Missing security patches, range-over-func, improved stdlib |
| 29 | 🟡 No structured logging in kanban-app middleware | `middleware.go` | Uses `log.Debug()`/`log.Warn()` (zerolog) but rate limiter cleanup is only structured log |
| 30 | ✅ Timeguru uses stdlib `log` instead of zerolog | `services/timeguru/cmd/timeguru/main.go` | FIXED: Already uses `github.com/rs/zerolog` |
| 31 | ✅ `publishTimelineUpdate` generates trace_id with `UnixNano()` | `main.go:393` | FIXED: Uses atomic counter |
| 32 | ✅ WebSocket client ID uses `UnixNano()` | `server.go:315` | FIXED: Uses `atomic.AddInt64` |
| 33 | ✅ Coverage not enforced in CI | CI config | FIXED: commit 92dbf78 added CI coverage gate at 50% threshold |
| 34 | ✅ 90% of Nix integration tests are stubs | `nix/tests/` | FIXED: commit 92dbf78 implemented 4 security tests that validate actual Nix config files |
| 35 | 🟡 `make deploy` is a no-op | `Makefile` | Deployment pipeline doesn't exist yet |
| 36 | 🟡 Log forwarding commented out | Multiple services | Logs stay local, no aggregation |
| 37 | 🟡 `BroadcastJSON` returns error instead of encoding | `server.go:611` | Dead method — either implement or remove |
| 38 | ✅ No request ID / correlation in HTTP middleware | `middleware.go` | FIXED: commit 79a5215 added X-Request-ID middleware to kanban-app |

---

## P3 — POLISH

| # | Finding | File(s) | Impact |
|---|---------|---------|--------|
| 39 | ✅ `churn_analysis.awk`, `full-race-results.txt`, `race-fix-results.txt` in repo root | Root | FIXED: deleted in commit 92dbf78 |
| 40 | 🟢 Multiple `test-results.txt` files tracked | Root | Same — gitignore these |
| 41 | 🟢 `PROJECT_TREE.txt` gets stale | Root | Auto-generate in CI or remove |
| 42 | 🟢 Some services have both `services/X/` and `X/` directories | Root vs `services/` | Confusing layout — consolidate |

---

## 📦 MISSING IMPLEMENTATIONS

| # | Component | Location | Notes |
|---|-----------|----------|-------|
| 43 | ✅ Auth middleware (JWT/mTLS) | `pkg/auth/` | EXISTS: `pkg/auth/auth.go` has skeleton with JWT/mTLS validators, Identity, Middleware (stub, beta-phase) |
| 44 | Service discovery | `pkg/discovery/` | Hardcoded IPs everywhere, partially addressed by `--services-file` flag (commit d37e324) |
| 45 | ✅ Fuzz testing | `*_fuzz_test.go` | FIXED: commit 13e43f1 added 18 fuzz targets across 6 packages |
| 46 | Frontend unit tests | `dashboard/`, `kanban/` | No JS test framework configured |
| 47 | E2E test suite (real) | `tests/e2e/` | Existing E2E tests are partial |
| 48 | SBOM generation | CI | No `syft`/`cyclonedx` integration |
| 49 | Container image scanning | CI | No `trivy`/`grype` scanning |
| 50 | Secrets management | `pkg/secrets/` | No SOPS/age integration yet |

---

## FUTURE: IPv6 Header-Space Transport ("Packet-as-Message")

**Priority:** Post-alpha research spike
**Concept:** Ultra-lean boolean/flag transport embedded directly in IPv6 header space

The mesh metadata hack in `cmd/ebpf-collector/common/src/lib.rs` proves the concept: 10 bytes of structured data riding inside IPv4-mapped `[::ffff:x.x.x.x]` prefix bytes. Take it further:

**Vision:** A service-to-service transport where the **packet IS the message**. No payload. The IPv6 extension headers (Hop-by-Hop Options, Destination Options) carry 10+ bytes of yes/no boolean checkboxes, small enums, and bitfield state. eBPF programs at each hop read/write these fields at line rate.

**Architecture:**
```
API/encoder  ──→  IPv6 packet on wire (headers = the data)  ──→  API/decoder
   ↑                     ↑                                          ↑
   │              Hop-by-Hop Options header:                        │
   │              [2 bytes] service mesh flags                      │
   │              [4 bytes] trace correlation                       │
   │              [2 bytes] QoS/priority bits                       │
   │              [2+ bytes] boolean checkbox array                 │
   │                                                                │
   encode()       eBPF reads/stamps per-hop                    decode()
```

**Use cases:**
- Distributed feature flags propagated at wire speed (no sidecar, no config poll)
- Per-hop health/load signals (each eBPF program stamps its view)
- Traffic shaping decisions embedded in the packet itself
- Service mesh control plane at zero extra bandwidth cost

**Why it works:** IPv6 extension headers are designed for this. Hop-by-Hop Options are processed at every router. eBPF XDP/TC programs can read/write them before the kernel even sees the packet. Encoding/decoding is just bitfield ops — protobuf-level semantics at memcpy-level cost.

**Beyond booleans — exponential encoding:**
The header bytes don't have to be flat flags. At the encoder/decoder/API boundary nodes, use exponential mappings: a single byte in the header is an exponent key that indexes into a proprietary lookup table of arbitrarily large structures. Example: byte value `0x17` → the encoder knows that maps to a full service mesh routing policy, a feature flag bundle, or a 4KB config blob. The wire cost is 1 byte. The semantic cost is whatever you want — the exponent tables live at the edge nodes and can be versioned, hot-swapped, or domain-specific. Think of it as a compression scheme where the packet header is the compressed form and the API nodes hold the dictionaries. You get `2^8 = 256` distinct "messages" per byte, `2^16 = 65K` per two bytes, all riding at line rate with zero payload overhead. Proprietary maps stay private to the encoder/decoder pair — the network just moves opaque exponent keys.

**Research: Header-space compute bus ("64K computer"):**
IPv6 Hop-by-Hop extension headers can carry up to ~64KB of option data per packet. That's not just a message bus — it's a tiny **computer** riding on the wire. Look into running structured compute primitives inside that space, inspired by embedded/avionics bus protocols that already do real work in 8-10 byte frames:

- **CSDB (Commercial Standard Digital Bus):** Avionics standard where data frames are up to 10 bytes. Entire flight systems coordinate over this. If avionics can fly a plane on 10-byte messages, we can run a service mesh control plane in them.
- **I2C 10-bit addressing:** Supports 1,024 unique addresses in a 10-bit space. Map this to service discovery — 1,024 services addressable in 2 bytes of header space, no DNS lookup needed.
- **SPI custom transfers:** Microcontrollers use `uint16_t` buffers for 10-bit/10-byte custom messages with careful register handling. Same principle — pack structured micro-instructions into header bytes, decode at each hop.
- **CAN Bus:** Standard frames limited to 8 bytes of data, yet entire automotive systems (engine, brakes, ABS) run on this. Multi-frame segmentation for larger messages. Apply same pattern: fragment a "program" across sequential packets, reassemble at destination.

**The vision:** Instead of just flags and exponent keys, embed a tiny instruction set in the header space. Each hop's eBPF program is a "CPU" that reads opcodes from the header, executes them (route decision, load-balance pick, feature gate check, metric increment), writes results back into the header, and forwards. The packet arrives at the destination not just as data, but as a **completed computation** that every hop contributed to. A distributed computer where the network fabric IS the processor and packets are the instruction stream. 64KB is enough for a real program.

**Prereqs:** Working eBPF pipeline (this sprint), IPv6 network, extension header parsing in XDP programs.

---

## CROSS-REFERENCE: TASK_LIST.md

The detailed task list (`TASK_LIST.md`) contains 92 tasks across 9 phases. This TODO.md focuses on **findings from the code review** that map to specific code-level issues. Key overlaps:

| TODO Item | TASK_LIST Entry |
|-----------|----------------|
| #9 eBPF stubs | TASK-070 through TASK-074 (Phase G) |
| #16 No auth | TASK-060, TASK-062 (Phase F) |
| #10 Nix deps | TASK-040, TASK-041 (Phase D) |
| #44 Service discovery | TASK-050 (Phase E) |
| #33 Coverage | All Wave 2 + Wave 3 tasks |

---

## QUICK WINS (< 1 hour each, highest leverage first)

1. ✅ **Pin gosec version** — Change `gosec@master` → `gosec@v2.21.0` + remove `-no-fail`
2. ✅ **Add `MaxHeaderBytes: 1 << 20`** to all `http.Server{}` instances
3. ✅ **Add context to rate limiter cleanup** — pass `context.Context`, select on `ctx.Done()`
4. ✅ **Add exponential backoff** to `pollMessages` error path in wotan-client
5. ✅ **Replace `UnixNano()` IDs** with atomic counter + timestamp combo (already done in micromanager, replicate pattern)
6. ✅ **Add X-Request-ID middleware** — generate UUID, propagate in context, log with every request
7. **Remove dead `BroadcastJSON` method** from websocket server
8. ✅ **Gitignore dev artifacts** — `churn_analysis.awk`, `*-results.txt`, `PROJECT_TREE.txt`
9. ✅ **Migrate timeguru to zerolog** — consistency with all other services
10. ✅ **Split HTTP client timeouts** — 5s for control plane ops, 30s for streaming only

---

## AGENT REVIEW INDEX

This TODO was synthesized from 10 parallel review agents covering:

| Agent | Domain | Key Files |
|-------|--------|-----------|
| 1 | Security middleware | `cmd/kanban-app/middleware.go` |
| 2 | WebSocket server | `cmd/dashboard-backend/internal/websocket/server.go` |
| 3 | Timeguru service | `services/timeguru/cmd/timeguru/main.go` |
| 4 | Micromanager service | `services/micromanager/service.go` |
| 5 | wotan-client pkg | `pkg/wotan-client/client.go` |
| 6 | Captain service | `services/captain/` |
| 7 | Architect service | `services/architect/` |
| 8 | Nix containers | `nix/containers/*.nix` |
| 9 | eBPF programs | `ebpf/*/` |
| 10 | CI/Build system | `Makefile`, `.github/` |

---

---

## DOOM-OVER-IPV6 — Session Summary (2026-02-23)

### Completed
- **LH/LB sign-extension fix** — Doom was stuck at frame 565 (blockmap infinite loop). Fixed with SHL+SAR in translator.
- **Execution driver fault detection** — run.py now distinguishes syscall halts from ROM faults via diagnostic sentinels.
- **Keyboard input pipeline** — Fixed key encoding (PC/AT → doomkeys.h), added clear-after-read in BPF SYS_GET_KEY.
- **L1 cache cleanup** — Removed dead cache code; RAM_MAP is BPF Array (O(1)), cache was misleading.
- **Doom is RUNNING** in eBPF MBC VM with live WebSocket frame streaming at 30 FPS.

### Status: Playable but Slow
- Internal game FPS: ~6 (choppy). WebSocket streams at 30 FPS (repeats frames).
- Single-hop only (BPF on veth50p / hop 0). Full 6-namespace ring not yet utilized.
- Auto-restart crash loop bug: resets CPU but doesn't reload .data sections.

### Next — Performance Tuning (Priority Order)
1. **Rewrite execution driver in Go or Rust** — Python `sock.send()` loop is the bottleneck.
   Use batched sendmmsg(), AF_XDP, or spin-loop for 10-100x packet injection rate.
2. **Increase burst rate** — Experiment with 1000-5000+ packets per burst (currently 500).
3. **Fix auto-restart** — Must reload .data sections on CPU reset, not just zero PC/SP.
4. **THEN add multi-hop ring** — 6 hops = 6x insns/packet (768 vs 128). Only after
   single-hop is smooth and playable.

### Unpushed Commits (6)
```
30379cd fix(doom): keyboard input pipeline
3c97fb1 refactor(ebpf): remove dead L1 cache code
99d223f fix(doom): execution driver fault detection
f969c87 fix(mbc): LH/LB sign-extension
8ac0bc2 feat(doom): execution driver
a4305b9 feat(doom): S33 hardening sprint
```

---

**THE KNIGHT IS NEVER WITHOUT ARMOR.**
**THE KINGDOM RISES.**

⚔️🛡️🏰
