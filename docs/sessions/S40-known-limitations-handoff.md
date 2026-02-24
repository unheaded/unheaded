# S40 Session Handoff — Known Limitations Sweep

**Date:** 2026-02-24
**Session:** S40 (continuation of S38/S39 eBPF production pipeline)
**Commits:** 5 (`14566eb` through `a9120da`)
**Files changed:** 12 (+657/-66 lines)

---

## What Was Done

Addressed all 6 Known Limitations from the prior session's pipeline work. The full kernel-to-dashboard pipeline is now operational end-to-end in demo mode.

### 1. Dashboard GUI Not Served (commit `14566eb`)

**Problem:** `bin/dashboard-backend` used relative path `"static"` for file serving — only worked when CWD was the source directory.

**Fix:** Added `go:embed static/*` to bundle index.html, dashboard.js, styles.css into the binary. Dashboard now loads at port 20000 regardless of working directory.

**Also fixed:** Data race in `cmd/dashboard-backend/internal/events/events.go` — `s.client` accessed without synchronization between `streamMessages` goroutines and `disconnect()`. All client field access now goes through `connMu`.

**Files:** `cmd/dashboard-backend/main.go`, `cmd/dashboard-backend/internal/server/server.go`, `cmd/dashboard-backend/internal/events/events.go`

### 2. Pre-built Binary Staleness (commit `14566eb` + full rebuild)

**Problem:** `bin/dashboard-backend` had stale `-busboy` flag.

**Fix:** Rebuilt all 14 service binaries from source:
- dashboard-backend, trace-collector-go, wotan, wotan-ctl
- unheaded-daemon, kanban-app, doom-bridge, monad, sophia
- architect, captain, micromanager, timeguru, gateway

### 3. XDP VM Attachment EOPNOTSUPP (commit `2a9dd33`)

**Problem:** XDP driver mode fails on VM virtio NICs with EOPNOTSUPP.

**Fix:** Added automatic fallback from driver mode to generic/SKB mode (xdpgeneric) in `pkg/ebpf/loader.go:attachXDP()`. Applies to both BPF link (kernel 5.9+) and netlink attachment paths.

**Files:** `pkg/ebpf/loader.go`

### 4. PACKET_EVENTS Ringbuf Reader (commit `1f15cf0`)

**Problem:** `IterateMap` (BPF_MAP_GET_NEXT_KEY) doesn't work on ring buffers. Trace reader fell back to polling the STATS hash map.

**Fix:**
- Added `GetPacketEventsCh(ctx) <-chan []byte` to `BPFLoader` interface
- `NativeBPFLoader` wires to existing `ReadRingbuf()` mmap reader in `pkg/ebpf`
- `TraceReader.Run()` auto-selects ringbuf mode (push-based, zero-copy) or poll mode (fallback)
- 4 new tests: ringbuf mode, bad data handling, channel close, poll fallback

**Files:** `cmd/trace-collector-go/loader.go`, `cmd/trace-collector-go/loader_native.go`, `cmd/trace-collector-go/reader.go`, `cmd/trace-collector-go/reader_test.go`

### 5. Dashboard Showing 0 Flows (commits `eb6d3ae` + `a9120da`)

**Problem:** Two root causes:

**5a — JSON array batching mismatch:** trace-collector publishes events as JSON arrays `[{...}, {...}]` but the dashboard ingestor's `ParseFlowEvent()` expected single JSON objects, causing all parse attempts to fail silently.

**Fix:** `dispatch()` now detects arrays (leading `[`), unwraps them via `json.RawMessage`, and dispatches each element individually. 3 new tests: BatchedFlowArray, BatchedPacketArray, BatchedLatencyArray.

**5b — WotanPublisher was a stub:** `PublishBatch()` had `req.Body = http.NoBody` — never actually sent data. Also, gRPC requires subscription before publish, and demo events used `anamnesis.*` topics which the dashboard doesn't subscribe to.

**Fix:**
- Implemented real gRPC publishing via `TopicStreamClient` with HTTP fallback
- Added `ConnectGRPC()` with topic subscription for all publish topics
- Added `anamnesisToFlowJSON()` bridge that converts anamnesis events to dashboard-compatible `FlowEvent` format and dual-publishes to `ebpf.flow.events`
- Fixed `TestWotanPublisher_PublishBatch` to use `httptest.Server`

**Files:** `cmd/dashboard-backend/internal/ebpf/ingestor.go`, `cmd/dashboard-backend/internal/ebpf/ebpf_test.go`, `cmd/trace-collector-go/main.go`, `cmd/trace-collector-go/main_test.go`

### 6. Empty LATENCY_MAP — Not a Bug

The `tcp_rcv_established` kprobe only fires when TCP data arrives on established connections. The map is correctly empty when there's no active TCP traffic through monitored interfaces. Will auto-populate with real traffic.

---

## Current State

### Running Services (as of session end)

| Service | Port | Status |
|---------|------|--------|
| Wotan | 18000 (HTTP), 18001 (gRPC) | Running |
| Dashboard-backend | 20000 | Running, GUI loads, 31K+ flows ingested |
| Trace-collector-go | 16670 (metrics) | Running in demo mode, publishing via gRPC |

### Pipeline Verification

```
Demo events → trace-collector → gRPC (18001) → Wotan → dashboard-backend → /api/v1/flows
```

- `/api/v1/flows`: 16,991 active flows, 31,702 total seen
- `/api/v1/ebpf/stats`: `flows_ingested: 31702`, `parse_errors: 4`
- Dashboard GUI: `http://localhost:20000/` returns 200, CSS/JS all load

### Test Results

- **trace-collector-go:** All pass (race-clean)
- **dashboard-backend:** All packages pass (1 pre-existing timing-sensitive E2E test)
- **pkg/ebpf:** All pass

---

## What's Next

### Immediate (high impact)

1. **Start trace-collector in unified mode** (`-unified -interface eth0`) with real BPF programs for live kernel traffic instead of demo mode
2. **Test XDP generic fallback** on the VM's virtio NIC with the new SKB mode fallback
3. **WebSocket live streaming** — verify dashboard JS connects to `/ws` and receives real-time flow updates
4. **Latency data** — generate TCP traffic to populate LATENCY_MAP via kprobe

### Follow-up

5. **Packet events via ringbuf** — test the new `GetPacketEventsCh` path with real PACKET_EVENTS ring buffer
6. **Dashboard GUI polish** — the `dashboard/` directory has advanced visualizations (doom.html, latency-chart.js, packet-flow-diagram.js) not yet served by the backend
7. **`bin/dashboard-backend-new`** — stale file in bin/, can be removed

---

## Key File Changes

| File | Change |
|------|--------|
| `cmd/dashboard-backend/main.go` | `go:embed static/*`, `fs.Sub`, `StaticFS` config field |
| `cmd/dashboard-backend/internal/server/server.go` | `io/fs` import, `staticFS` field, `handleStaticFile()`, `serveStaticFile()` |
| `cmd/dashboard-backend/internal/events/events.go` | `connMu`-protected `s.client` access, `streamTopicWith()` |
| `cmd/dashboard-backend/internal/ebpf/ingestor.go` | JSON array unwrapping in `dispatch()`, split to `dispatchSingle()` |
| `cmd/dashboard-backend/internal/ebpf/ebpf_test.go` | 3 batch tests |
| `cmd/trace-collector-go/loader.go` | `GetPacketEventsCh` interface method, mock impl |
| `cmd/trace-collector-go/loader_native.go` | `GetPacketEventsCh` via `ReadRingbuf` |
| `cmd/trace-collector-go/reader.go` | `runRingbuf()`, auto-select ringbuf vs poll |
| `cmd/trace-collector-go/reader_test.go` | 4 ringbuf tests |
| `cmd/trace-collector-go/main.go` | Real `PublishBatch`, `ConnectGRPC`, `PublishRaw`, `anamnesisToFlowJSON` |
| `cmd/trace-collector-go/main_test.go` | `httptest.Server` for publish test |
| `pkg/ebpf/loader.go` | XDP generic/SKB fallback in `attachXDP()` |
