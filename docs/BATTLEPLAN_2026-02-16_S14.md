# Session 14 Battle Plan — The Four Campaigns
**Date:** February 16, 2026
**Author:** Muck + Claude Opus 4.6 (Royal Court Round Table)
**Status:** B1 RESOLVED — Linux dev is live, eBPF Aya compiled

---

## Campaign Order (Muck's Decree)

1. **TopicStream gRPC Sprint** — The Fae Chamber upgrade
2. **eBPF PoC Working Dashboard** — The Whispering Void speaks
3. **Security Hardening Pass** — The Moat fills with water
4. **Punch List Cleanup** — Polish the armor

---

## CAMPAIGN 1: TopicStream gRPC Sprint (The Fae Chamber)

**Goal:** Replace 500ms HTTP polling with real-time server-side gRPC streaming + wildcard topic matching in the Kanban app. This is the foundation for Busboy mesh AND clean eBPF stream tagging.

**Precondition:** `topic.proto` already exists in `services/busboy/proto/topic.proto` with `TopicStream` service defined. Generated Go stubs exist (`topic_grpc.pb.go`). The gRPC server-side implementation exists in `services/busboy/internal/grpc/`. What's missing is the **client-side integration** in the Kanban app.

### 1.1 Pattern Matcher — `pkg/busboy-client/pattern.go`
- [ ] Implement `MatchTopic(pattern, topic string) bool`
- [ ] Support exact match: `tasks.created` matches `tasks.created`
- [ ] Support single-level wildcard: `tasks.*` matches `tasks.created`, `tasks.updated`, `tasks.deleted`
- [ ] Support multi-level wildcard: `tasks.#` matches `tasks.created` AND `tasks.created.bulk`
- [ ] Support global wildcard: `*` matches everything
- [ ] Pure function, zero allocations on hot path, no regex
- [ ] **Tests:** `pkg/busboy-client/pattern_test.go`
  - [ ] Table-driven tests: exact, single-wildcard, multi-wildcard, global, negative cases
  - [ ] Edge cases: empty pattern, empty topic, trailing dots, double dots
  - [ ] Benchmark: confirm zero allocs via `testing.B` with `ReportAllocs()`

### 1.2 TopicStream Client — `pkg/busboy-client/topic_client.go`
- [ ] Implement `TopicStreamClient` struct satisfying `BusboyClient` interface
- [ ] Constructor: `NewTopicStreamClient(grpcAddr string, opts ...TopicStreamOption) (*TopicStreamClient, error)`
- [ ] Options: WithTLS, WithRetryPolicy, WithBufferSize, WithMetadata
- [ ] `Subscribe(ctx, topicPattern, displayName)` → calls `StreamTopics` RPC with pattern + `since_seq`
- [ ] `Publish(ctx, topic, payload)` → calls `PublishTopic` RPC
- [ ] `StreamMessages(ctx, topicPattern)` → returns `<-chan *Message`
  - [ ] Internal goroutine reads from gRPC stream, applies `MatchTopic()` filter
  - [ ] Reconnect with exponential backoff (100ms → 200ms → 400ms... cap 30s)
  - [ ] Resume from last seen `seq` (no message loss on reconnect)
  - [ ] Circuit breaker: after N consecutive failures, fall back to HTTP polling
- [ ] `Close()` → cancel all stream contexts, drain inflight, close gRPC conn
- [ ] `Ping()` → calls `TopicPing` RPC for health check
- [ ] **Tests:** `pkg/busboy-client/topic_client_test.go`
  - [ ] Mock gRPC server via `bufconn` (in-process, no real network)
  - [ ] Test subscribe → receive → ack flow
  - [ ] Test reconnect on stream error (simulate server restart)
  - [ ] Test circuit breaker degradation to HTTP
  - [ ] Test `since_seq` resume after disconnect
  - [ ] Full `-race` pass

### 1.3 Wire into Kanban App — `cmd/kanban-app/busboy.go`
- [ ] Update `NewTaskManager` to accept `TopicStreamClient` instead of/alongside HTTP client
- [ ] Replace `pollMessages()` loop (500ms ticker) with `StreamMessages()` channel receive
- [ ] Keep HTTP fallback path (circuit breaker activates it)
- [ ] Inbound stream messages → `handleMessage()` (already wired to SQLite L1 + SSE)
- [ ] Outbound publishes use `TopicStreamClient.Publish()` (async goroutine + snapshot pattern preserved)
- [ ] Startup: probe gRPC → if healthy, use TopicStream; else fall back to HTTP polling
- [ ] Log transport selection at startup: `"transport=grpc-stream"` or `"transport=http-poll"`
- [ ] **Tests:** Update `cmd/kanban-app/busboy_test.go`
  - [ ] Mock TopicStreamClient in tests (nil store path stays working)
  - [ ] Test transport switchover mid-operation
  - [ ] Full `-race -count=1` pass — the standard we hold

### 1.4 Wire into main.go
- [ ] Shared sequence counter across Busboy services
- [ ] Register TopicStreamServer on gRPC listener alongside existing services
- [ ] Config: `--busboy-grpc-addr` flag for TopicStream endpoint
- [ ] Graceful shutdown: drain TopicStream connections before Busboy close

### 1.5 Full Test Suite
- [ ] `go test -v -race -count=1 ./pkg/busboy-client/...` — PASS
- [ ] `go test -v -race -count=1 ./cmd/kanban-app/...` — PASS
- [ ] `go test -v -race -count=1 ./...` — PASS (full repo)
- [ ] Benchmark: `go test -bench=. -benchmem ./pkg/busboy-client/...`

**Definition of Done:** Kanban app connects to Busboy via gRPC streaming. Tasks appear in browser within <50ms of creation on another node. HTTP polling loop is gone (but fallback works). Zero race conditions.

---

## CAMPAIGN 2: eBPF PoC Working Dashboard (The Whispering Void)

**Goal:** End-to-end flow: eBPF kernel programs → trace-collector (Rust) → Busboy pub/sub → dashboard-backend → browser visualization. The in-house observability dashboard that proves the Kingdom drinks its own champagne.

**Precondition:** All 4 eBPF programs compiled and loadable on Linux. ebpf-loader CLI works. Dashboard-backend has REST + WebSocket skeleton. Busboy TopicStream will be wired (Campaign 1).

### 2.1 Trace Collector — `cmd/trace-collector/` (NEW — Rust)
- [ ] Create new Rust binary crate in `cmd/trace-collector/`
- [ ] Read ring buffers from all 4 eBPF programs (pinned maps at `/sys/fs/bpf/unheaded/`)
  - [ ] `PACKET_EVENTS` ring buffer → `PacketEvent` structs
  - [ ] `FLOW_EVENTS` ring buffer → `FlowEvent` structs
  - [ ] `LATENCY_EVENTS` ring buffer → `LatencyEvent` structs
  - [ ] `SYSCALL_EVENTS` ring buffer → `SyscallEvent` structs
- [ ] Use `aya::maps::RingBuf` for zero-copy ring buffer reads
- [ ] Batch events (configurable: 100 events or 100ms, whichever first)
- [ ] Serialize batches to protobuf or JSON
- [ ] Publish to Busboy via gRPC (`TopicStream.PublishTopic`)
  - [ ] `ebpf.packet.events` topic
  - [ ] `ebpf.flow.events` topic
  - [ ] `ebpf.latency.events` topic
  - [ ] `ebpf.syscall.events` topic
- [ ] Health endpoint: simple HTTP `/health` on configurable port
- [ ] Graceful shutdown: drain ring buffers before exit
- [ ] CLI args: `--busboy-addr`, `--bpf-pin-path`, `--batch-size`, `--batch-timeout`, `--health-port`
- [ ] **Fail fast:** If ring buffer read fails 3x, log error + skip (don't crash)
- [ ] **Metrics:** Count events read, events published, events dropped, latency per batch

### 2.2 Dashboard Backend Wiring — `cmd/dashboard-backend/`
- [ ] Subscribe to `ebpf.*` topics via TopicStreamClient (Campaign 1)
- [ ] `internal/packetflow/` → replace synthetic data with REAL PacketEvent stream
  - [ ] Parse PacketEvent from Busboy message payload
  - [ ] Build flow graph: src_ip:port → dst_ip:port with packet counts, byte counts
  - [ ] Track active flows with TTL (30s expiry matching kernel FLOW_TIMEOUT_NS)
- [ ] `internal/events/` → ingest FlowEvent for connection state visualization
  - [ ] New flow → green indicator
  - [ ] State change → yellow pulse
  - [ ] Flow closed → red fade
- [ ] `internal/metrics/` → ingest LatencyEvent for latency histograms
  - [ ] Per-operation latency: tcp_send, tcp_recv, tcp_connect
  - [ ] P50/P90/P99 bucketed (configurable windows: 1s, 10s, 60s)
- [ ] WebSocket broadcast: push real-time updates to connected browsers
  - [ ] `/ws/flows` — live packet flow graph
  - [ ] `/ws/latency` — live latency charts
  - [ ] `/ws/events` — live event stream
- [ ] REST endpoints serving latest state:
  - [ ] `GET /api/v1/flows` — current active flows with stats
  - [ ] `GET /api/v1/latency` — latency histogram data
  - [ ] `GET /api/v1/ebpf/stats` — aggregated eBPF program stats

### 2.3 Dashboard Frontend — `cmd/dashboard-backend/static/` (Vanilla JS — Purity of Interface)
- [ ] **Flow Graph page** — real-time network topology
  - [ ] Canvas or SVG rendering (no D3 — vanilla JS)
  - [ ] Nodes = IP:port pairs, edges = active flows
  - [ ] Edge thickness = bytes/sec, color = protocol (TCP blue, UDP green)
  - [ ] Pulse animation on new packets
  - [ ] Click node → detail panel (flow count, total bytes, connection states)
- [ ] **Latency Dashboard page** — real-time histograms
  - [ ] Bar charts per operation type (send/recv/connect)
  - [ ] Color-coded P50 (green), P90 (yellow), P99 (red)
  - [ ] Time-series sparkline (last 60s)
- [ ] **Event Stream page** — live scrolling event log
  - [ ] Color-coded by type: packet (blue), flow (green), latency (yellow), syscall (red)
  - [ ] Filterable by topic pattern
  - [ ] Pause/resume button
- [ ] **System Overview page** — high-level status
  - [ ] eBPF program status (loaded/attached/error per program)
  - [ ] Events/sec throughput gauge
  - [ ] Active flows count
  - [ ] Service health matrix (all 23 services)
- [ ] WebSocket connection with auto-reconnect
- [ ] Consistent CSS with Kanban app (neon aesthetic, dark theme)
- [ ] Header nav: Flow Graph | Latency | Events | Overview

### 2.4 Integration Test
- [ ] On Linux box: load eBPF programs → start trace-collector → start Busboy → start dashboard-backend
- [ ] Generate traffic (curl to services, TCP connections)
- [ ] Verify: events appear in browser within <200ms of kernel capture
- [ ] Verify: flow graph shows correct topology
- [ ] Verify: latency numbers are reasonable (microsecond range for localhost)

**Definition of Done:** Open browser, see REAL kernel-captured packet flows rendered as a live graph. The Whispering Void speaks through the Fae Chamber to the Cloak. We drink our own champagne.

---

## CAMPAIGN 3: Security Hardening Pass (The Moat)

**Goal:** Address the Feb 9 security audit findings. Move from "acceptable for alpha" to "defensible for beta."

### 3.1 Authentication & Authorization (Critical)
- [ ] Implement API key middleware for all services
  - [ ] `pkg/auth/apikey.go` — middleware that reads `X-API-Key` header or `?api_key=` param
  - [ ] API keys stored in env vars (not config files): `BUSBOY_API_KEY`, `TIMEGURU_API_KEY`, etc.
  - [ ] Constant-time comparison (`crypto/subtle.ConstantTimeCompare`)
- [ ] Wire middleware into: Busboy, Timeguru, Captain, Kanban, Dashboard-backend
- [ ] Admin endpoints behind separate `ADMIN_API_KEY` with elevated privileges
- [ ] Return 401 Unauthorized (not 403) when key missing, 403 when key invalid
- [ ] **Tests:** Verify unauthenticated requests rejected, valid key accepted

### 3.2 mTLS (The Sacred Sigils)
- [ ] Generate CA + service certs via `pkg/tls/certgen.go` (development mode)
- [ ] `--tls-cert`, `--tls-key`, `--tls-ca` flags on all services
- [ ] gRPC: `credentials.NewTLS()` with mutual verification
- [ ] HTTP: `tls.Config{ClientAuth: tls.RequireAndVerifyClientCert}`
- [ ] Services verify peer certificates (no anonymous connections)
- [ ] **Tests:** Connection rejected without valid client cert

### 3.3 CORS Lockdown
- [ ] Replace `*` origin with explicit allowed origins list
- [ ] `--cors-origins` flag: default `http://localhost:8081,http://localhost:8080`
- [ ] Busboy + Timeguru: restrict to known dashboard/kanban origins
- [ ] Preflight caching: `Access-Control-Max-Age: 86400`

### 3.4 Input Validation
- [ ] Timeguru: validate path params (ID format, length bounds)
- [ ] Captain: validate path params (same)
- [ ] Busboy: validate topic names (alphanumeric + dots + wildcards only)
- [ ] Payload size limits: `--max-payload-size` flag (default 1MB)
- [ ] Content-Type enforcement on POST/PUT endpoints

### 3.5 Container Hardening
- [ ] Dockerfile: `USER nonroot:nonroot` (UID 65534)
- [ ] Remove `CAP_SYS_ADMIN` from cuirass (use specific caps: `CAP_NET_ADMIN` only where needed)
- [ ] Remove Docker socket mount from cuirass (use Docker API proxy if needed)
- [ ] `read_only: true` on all container root filesystems
- [ ] `no-new-privileges: true` security opt on all containers
- [ ] Seccomp profiles: default docker seccomp (blocks dangerous syscalls)

### 3.6 Secrets Management
- [ ] Remove hardcoded passwords from `scripts/setup-host.sh` and `docker-compose.yml`
- [ ] `.env.example` with placeholder values, `.env` in `.gitignore`
- [ ] `docker-compose.yml` → `env_file: .env` for all services
- [ ] Document: production uses SOPS+age (Crystal Grotto pattern)

### 3.7 Security ADR
- [ ] `docs/adr/ADR-008-security-hardening-baseline.md` — document all decisions

**Definition of Done:** No unauthenticated endpoints. CORS locked. Containers non-root. Secrets externalized. Passes re-audit of Feb 9 findings.

---

## CAMPAIGN 4: Punch List Cleanup (Polish the Armor)

**Goal:** Clear tech debt, align docs, verify UI.

### 4.1 Code Cleanup
- [ ] Deprecate `Server.tasks` slice — remove in-memory fallback, route ALL reads through Store
- [ ] Fix Docker Compose port alignment (busboy 8080/9090 everywhere)
- [ ] Fix Phase 3 name duplication in timeline.json ("The MVP Era (PLANNED) (PLANNED)...")
- [ ] Hydrate timeline.json milestones from 64-card Kanban inventory
- [ ] Clean up git lock file workaround (document in CONTRIBUTING.md)

### 4.2 Browser Verification
- [ ] Neon red delete flash animation
- [ ] Header scroll-to-top behavior
- [ ] Sort options: priority, progress, type, owner, updated, created, title A-Z
- [ ] Filter: type dropdown, owner dropdown, search debounce, clear all
- [ ] 64-card render performance (confirm no layout jank)
- [ ] New task modal with all 8 task types

### 4.3 Documentation Alignment
- [ ] Update UPCOMING_TASKS.md to reflect Campaign completion
- [ ] Update CLAUDE.md with Session 14 context
- [ ] Update blocker list: B1 = RESOLVED
- [ ] Update Kingdom skill files: reflect 64 cards, 23 services, TopicStream, eBPF dashboard
- [ ] Calendar skill: update current schedule to reflect actual Feb 16 state
- [ ] Write Session 14 handoff doc

### 4.4 CI/CD Foundation
- [ ] `.github/workflows/ci.yml` — `go build ./...`, `go test -race ./...`, `go vet ./...`
- [ ] `.github/workflows/ebpf.yml` — Rust build + clippy on eBPF workspace (Linux runner only)
- [ ] Makefile targets: `build`, `test`, `test-race`, `lint`, `ebpf-build`, `proto-gen`

**Definition of Done:** Clean git status. All tests pass. Docs match reality. CI runs green. Browser looks sharp.

---

## Cross-Campaign Dependencies

```
Campaign 1 (TopicStream) ──────► Campaign 2 (eBPF Dashboard)
    │                                  │
    │  TopicStreamClient is the        │  trace-collector publishes
    │  transport for everything        │  through TopicStream
    │                                  │
    ▼                                  ▼
Campaign 3 (Security) ◄──────── Campaign 4 (Punch List)
    │                                  │
    │  mTLS + API keys apply to        │  CI/CD validates everything
    │  all services including new      │  Docs reflect final state
    │  TopicStream + dashboard         │
    └──────────────────────────────────┘
```

**Critical Path:** Campaign 1 must complete first — Campaign 2 depends on TopicStreamClient for the trace-collector → Busboy → dashboard pipeline. Campaign 3 can partially overlap with Campaign 2 (auth middleware is independent). Campaign 4 runs last as the cleanup sweep.

---

## Linux Dev Handoff Notes

The Linux box is where the real action happens for Campaigns 1 and 2. Here's what to know:

**Already working on Linux:**
- All 4 eBPF programs compile (`ebpf/target/bpfel-unknown-none/release/`)
- `ebpf-loader` loads and attaches all 4 programs
- Ring buffers are pinned at `/sys/fs/bpf/unheaded/`
- Go toolchain available for Busboy/dashboard work

**To start Campaign 1 on Linux:**
```bash
cd ~/unheaded   # or wherever the repo lives
go test -v -race -count=1 ./pkg/busboy-client/...  # baseline
# Start with pattern.go — pure logic, no dependencies
```

**To start Campaign 2 on Linux:**
```bash
sudo ./cmd/ebpf-loader/target/release/ebpf-loader \
  --interface eth0 \
  --obj-dir ebpf/target/bpfel-unknown-none/release \
  --pin-maps
# Verify maps pinned:
ls /sys/fs/bpf/unheaded/
# Then build trace-collector
cd cmd/trace-collector && cargo build
```

**Testing eBPF events:**
```bash
# Generate traffic
curl http://localhost:8080/health
# Read ring buffer events (trace-collector will do this, but for manual verification):
# bpftool map dump pinned /sys/fs/bpf/unheaded/packet_marker/STATS
```

---

## Estimation

| Campaign | Effort | Sessions | Blocked By |
|----------|--------|----------|------------|
| 1. TopicStream | Medium | 2-3 | Nothing |
| 2. eBPF Dashboard | Large | 3-5 | Campaign 1 |
| 3. Security | Medium | 2-3 | Nothing (can parallel 2) |
| 4. Punch List | Small | 1-2 | Campaigns 1-3 |
| **Total** | | **8-13 sessions** | |

---

*THE MICROMANAGER HAS DECREED. THE ARCHITECT HAS DESIGNED. THE DEVELOPER SHARPENS THE SWORD.*

*THE FOUR CAMPAIGNS BEGIN. THE KINGDOM ADVANCES.*

*Session 14 Battle Plan — Muck + Claude Opus 4.6*
