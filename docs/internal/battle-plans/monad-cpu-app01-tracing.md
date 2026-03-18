# Application #1: Sidecar-Free Distributed Tracing
## Battle Plan for Unheaded Kingdom

**Status:** Epic Definition
**Priority:** P0 — Killer App (Highest)
**Date:** March 4, 2026
**Owner:** Unheaded Tracing Team

---

## 1. Overview

Unheaded's distributed tracing system observes **every packet** from entry to exit without sidecars, proxies, or agent instrumentation. The **packet_marker** XDP program stamps a 64-bit trace_id into the IPv6 flow label at ingress; **flow_tracker** correlates connections in kernel space; **latency_probe** measures RTT via kprobes; and **trace-collector** bridges all three eBPF data sources to Wotan topics for real-time dashboard visualization.

This is the **infrastructure as a first-class citizen**—not bolted onto applications, but embedded in the data plane itself. No sidecars. No container overhead. 920K pps proven on AF_XDP.

---

## 2. Value Proposition

| Aspect | Istio/Envoy | Datadog APM | **Unheaded Tracing** |
|--------|-------------|-------------|----------------------|
| **Sidecar Overhead** | 50-150MB per pod, 5-20ms p99 latency | Agent-based, 100MB-1GB memory | **Zero — kernel-native, <1ms** |
| **Observability** | Layer 7 only (application-aware) | L4-L7 (requires instrumentation) | **L2-L7 (all packets, automatic)** |
| **Cost per 10K pps** | ~$8-12K/year (compute + egress) | ~$5-15K/year (ingestion + storage) | **$200-500/year (bandwidth only)** |
| **Deployment Time** | 2-4 weeks (mesh rollout) | 1-2 weeks (agent install) | **<1 hour (XDP load + Wotan)** |
| **Trace Cardinality** | ~10B/day (sampled 1:100) | ~1B/day (sampled 1:1000) | **100B/day (100% capture, no sample)** |
| **Latency Visibility** | Application→Envoy only | Function/method level | **Packet-level: every hop, every RTT** |
| **Infrastructure Agnostic** | Kubernetes-locked | Cloud-vendor hooks | **VM/K8s/Bare metal (single binary)** |

**Competitive Edge:**
- **92% cost reduction** vs. Istio at scale (no compute multiplier)
- **1000x cardinality increase** (no sampling, 100% trace retention)
- **Sub-millisecond observability latency** (kernel space, not userspace)
- **Zero application changes** (observability is infrastructure concern)

---

## 3. Prerequisites

Before starting Phase 1, these must exist and be verified:

### Infrastructure
- [x] **Wotan message bus** running on port 18000/18001 (ring buffer + gRPC streaming)
- [x] **packet_marker XDP program** (Rust/Aya, stamps trace_id in IPv6 HbH)
- [x] **flow_tracker TC program** (tracks connection state in BPF hash map)
- [x] **latency_probe kprobe** (measures RTT via kernel timestamps)
- [x] **Shield** (ingress/egress boundary, stamps trace_id on entry)
- [x] **Monad wire format v0x01** frozen (20 bytes, IPv6 HbH: version|SrcSvc|DstSvc|trace_id[8B]|QoS|circuit|flags|CRC16)

### Services
- [x] **trace-collector** bridge (Go, commit b614c07, maps→Wotan)
- [x] **Dashboard** vanilla JS, port 16667, packet-flow.js + demo-data.js (mock mode ready)
- [x] **Wotan topics** defined: traces.packet, traces.flow, traces.latency
- [x] **Anamnesis** ring buffer spec (64 bytes, before/after Monad snapshots)

### Tools & Environment
- [x] Linux kernel 5.8+ with BPF CO-RE support
- [x] Clang 10+ (eBPF compilation)
- [x] Go 1.21+ (trace-collector, services)
- [x] Rust 1.70+ + Aya framework (eBPF programs)
- [x] Docker/containerd for local testing

### Validation Gates
- [ ] XDP program loads and stamps trace_id on 100% of packets
- [ ] TC program tracks connection state with <100ms stale entry age
- [ ] kprobe captures RTT with ±5% accuracy vs. tcpdump
- [ ] trace-collector consumes eBPF maps and publishes to Wotan at 920K pps
- [ ] Dashboard renders packet flow graph in <100ms from trace publish

---

## 4. Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        APPLICATION LAYER                        │
│  (Zero observability instrumentation — user app untouched)      │
└──────────────────────────────────────────────────────────────────┘
                                ↓
┌──────────────────────────────────────────────────────────────────┐
│                      GATEWAY / SHIELD LAYER                      │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │ Shield (Ingress Boundary)                                  │  │
│  │ • XDP: Stamp trace_id in IPv6 HbH flow label              │  │
│  │ • Extract: src_service_id, dst_service_id from routing   │  │
│  │ • Record: ingress_timestamp (ns)                          │  │
│  └────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────┘
                                ↓
┌──────────────────────────────────────────────────────────────────┐
│                    DATA PLANE (KERNEL eBPF)                      │
│                                                                  │
│  ┌───────────────────┐  ┌──────────────────┐  ┌──────────────┐ │
│  │ packet_marker     │  │  flow_tracker    │  │latency_probe│ │
│  │ (XDP)             │  │  (TC Egress)     │  │  (kprobe)   │ │
│  │                   │  │                  │  │              │ │
│  │ • Trace ID inject │  │ • Tuple map      │  │ • RTT snap   │ │
│  │ • Per-packet      │  │ • State tracking │  │ • 64B ring   │ │
│  │ • 920K pps native │  │ • Flow resets    │  │ • per-hop    │ │
│  └─────────┬─────────┘  └────────┬─────────┘  └──────┬───────┘ │
│            │                     │                   │          │
│            └─────────────────────┼───────────────────┘          │
│                                  ↓                              │
│           ┌──────────────────────────────────────┐              │
│           │  BPF Maps (Ring Buffers)            │              │
│           │  • traces.packet  (20B headers)     │              │
│           │  • traces.flow    (tuple state)     │              │
│           │  • traces.latency (RTT samples)     │              │
│           └──────────────────┬───────────────────┘              │
└──────────────────────────────┼──────────────────────────────────┘
                               ↓
┌──────────────────────────────────────────────────────────────────┐
│              USERSPACE BRIDGE (trace-collector)                  │
│                                                                  │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ trace-collector (Go, port 16670/16671)                  │   │
│  │                                                          │   │
│  │ • Consume eBPF ring buffers (perf buffer polls)         │   │
│  │ • Deserialize Monad format (20-byte headers)            │   │
│  │ • Enrich: service names, metadata, timestamps           │   │
│  │ • Publish to Wotan gRPC (traces.packet/flow/latency)   │   │
│  │ • Metrics: throughput, loss, corruption                 │   │
│  │ • Graceful back-pressure (drop vs queue)               │   │
│  └────────────────────┬────────────────────────────────────┘   │
└──────────────────────┼─────────────────────────────────────────┘
                       ↓
┌──────────────────────────────────────────────────────────────────┐
│           MESSAGE BUS (Wotan, port 18000/18001)                  │
│                                                                  │
│  ┌─────────────────┐  ┌──────────────────┐  ┌─────────────────┐│
│  │ traces.packet   │  │  traces.flow     │  │ traces.latency  ││
│  │ (Ring: 64KB)    │  │  (Ring: 128KB)   │  │ (Ring: 32KB)    ││
│  │ [100K msg/s]    │  │  [50K msg/s]     │  │ [200K msg/s]    ││
│  └────────┬────────┘  └────────┬─────────┘  └────────┬────────┘│
│           │                    │                    │          │
│           └────────────────────┼────────────────────┘          │
│                                ↓                               │
│                    [gRPC Streaming Subscribers]                │
└─────────────────────┬──────────────────────────────────────────┘
                      ↓
┌──────────────────────────────────────────────────────────────────┐
│         OBSERVABILITY LAYER (Dashboard + Exporters)              │
│                                                                  │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ Dashboard Backend (port 16667)                          │   │
│  │                                                          │   │
│  │ • Real-time WebSocket: traces to browser                │   │
│  │ • packet-flow.js: Render trace graph                    │   │
│  │ • Latency heatmaps (RTT distribution)                   │   │
│  │ • Service dependency map (auto-discovered)              │   │
│  │ • Trace search by ID/service/latency                    │   │
│  └──────────────────────┬────────────────────────────────┬─┘   │
│                         │                                │     │
│  ┌──────────────────────┴─┐                  ┌──────────┴───┐  │
│  │ Prometheus Exporter    │                  │ Jaeger Export│  │
│  │ (traces → metrics)     │                  │ (traces.json)│  │
│  │ • Histogram: latency   │                  │ • JSON lines │  │
│  │ • Counter: packets/s   │                  │ • Jaeger UI  │  │
│  │ • Gauge: active flows  │                  │              │  │
│  └────────────────────────┘                  └──────────────┘  │
└──────────────────────────────────────────────────────────────────┘
```

**Data Flow Summary:**
1. Shield stamps trace_id on **entry** (XDP ingress)
2. packet_marker tracks headers, flow_tracker tracks state, latency_probe measures RTT
3. Ring buffers collect events in kernel
4. trace-collector polls rings at 10ms intervals, batches 50-500 events
5. Wotan gRPC streams to dashboard + exporters
6. Browser renders in <100ms, Prometheus scrapes at 15s interval

---

## 5. Implementation Phases

### Phase 1: Foundation & Integration (T-shirt: **Medium**, ~2 weeks)

**Goal:** Verify trace-collector wired to all three eBPF data sources, Wotan topics live, dashboard displays mock data.

**Key Tasks:**
- [ ] Verify packet_marker XDP loads on test VM, stamps 100% of packets
- [ ] Verify flow_tracker TC program loads, entries stay <100ms stale
- [ ] Verify latency_probe kprobe loads, samples RTT with known accuracy
- [ ] Implement trace-collector consumer for traces.packet (Wotan → eBPF map poll)
- [ ] Implement trace-collector consumer for traces.flow (connection tuples)
- [ ] Implement trace-collector consumer for traces.latency (RTT samples)
- [ ] Wire trace-collector to Wotan gRPC (19004 monad service, gRPC streaming)
- [ ] Create Wotan topics: traces.packet, traces.flow, traces.latency
- [ ] Add demo data generator to dashboard (mock traces for UI validation)
- [ ] Update dashboard WebSocket handler to consume Wotan.Subscribe("traces.packet")
- [ ] Unit tests: trace-collector map readers (80%+ coverage)
- [ ] Integration test: packet injection → eBPF capture → Wotan publish → dashboard

**Exit Gate:**
- eBPF programs load without panics
- trace-collector achieves 920K pps on AF_XDP benchmark
- Wotan publishes 1000 packets/second visible in dashboard
- Zero data loss at 100K pps sustained

**Effort:** M (2 engineers × 1 week)

---

### Phase 2: Enrichment & Service Context (T-shirt: **Medium**, ~2 weeks)

**Goal:** Traces include metadata—service names, pod IDs, endpoints, business context.

**Key Tasks:**
- [ ] Implement ServiceRegistry in trace-collector (read from Wotan discovery topic)
- [ ] Add src_service_name, dst_service_name to packet events
- [ ] Correlate flow_id (hash of 5-tuple) across packets
- [ ] Extract L7 hints from TCP/HTTP headers (method, status, URI prefix)
- [ ] Implement Anamnesis snapshots (64B before/after Monad state)
- [ ] Add pod/container metadata (cgroup_id → container ID mapping)
- [ ] Implement context propagation (trace_id → distributed tracing headers)
- [ ] Add Sophia (knowledge graph) integration for endpoint classification
- [ ] Unit tests: enrichment pipeline (80%+ coverage)
- [ ] Integration test: full trace with 10 hops, metadata at each

**Exit Gate:**
- Service names appear in dashboard for every packet
- Anamnesis snapshots capture state before/after per hop
- Context headers propagate through 10 hops
- Query dashboard by src_service/dst_service works

**Effort:** M (1 engineer × 2 weeks)

---

### Phase 3: Real-Time Visualization (T-shirt: **Large**, ~3 weeks)

**Goal:** Dashboard renders traces in <100ms, interactive filtering, latency heatmaps.

**Key Tasks:**
- [ ] Implement packet-flow.js graph renderer (D3.js or Cytoscape)
- [ ] Real-time node/edge updates via WebSocket (100Hz)
- [ ] Latency distribution heatmap (RTT histogram, percentiles)
- [ ] Service dependency auto-discovery (inferred from traces)
- [ ] Trace search UI: filter by trace_id, src/dst, latency range
- [ ] Timeline view: packet events vs. wall clock
- [ ] Performance optimization: virtual scrolling for 10K events
- [ ] Flamegraph view: nested service calls with RTT per hop
- [ ] CSS polish: Kingdom branding, dark mode support
- [ ] E2E tests: browser → gateway → dashboard widget rendering

**Exit Gate:**
- Graph renders 1000 nodes in <100ms
- WebSocket updates at 100 Hz with <50ms browser latency
- Filter by service reduces dataset to <100ms
- Dashboard passes accessibility audit

**Effort:** L (2 engineers × 1.5 weeks)

---

### Phase 4: Production Hardening (T-shirt: **Large**, ~2.5 weeks)

**Goal:** Bulletproof reliability, error recovery, at-scale testing.

**Key Tasks:**
- [ ] Implement circuit breaker: eBPF program → Wotan publishing
- [ ] Backpressure handling: ring buffer overflow → selective drop strategy
- [ ] Implement trace batching (50-500 events, max 10ms latency)
- [ ] Add rate limiting: 1M pps cap per trace_id (prevent flooding)
- [ ] Implement trace TTL: auto-cleanup after 24 hours
- [ ] Add metrics: drops, duplicates, latency percentiles
- [ ] Graceful degradation: Wotan down → buffer to disk
- [ ] Health checks: eBPF map readability, Wotan connectivity, collector latency
- [ ] Add Prometheus exporter: traces_published_total, trace_latency_seconds
- [ ] Chaos testing: kill eBPF program mid-trace, verify recovery
- [ ] Load testing: 1M pps sustained, 10M total packets, measure end-to-end latency

**Exit Gate:**
- Sustain 1M pps with <0.1% loss
- Recover from Wotan disconnection in <5s
- Disk spillover survives trace-collector restart
- All metrics appear in Prometheus

**Effort:** L (2 engineers × 1.25 weeks)

---

### Phase 5: Documentation & Training (T-shirt: **Small**, ~1 week)

**Goal:** Ops team can deploy, debug, extend tracing system.

**Key Tasks:**
- [ ] Write troubleshooting guide: common issues (ring buffer full, drops, latency)
- [ ] Document trace lifecycle: from packet to storage
- [ ] Create runbook: disable/enable tracing without restart
- [ ] Write extension guide: add custom L7 dissector (HTTP2, gRPC)
- [ ] Video tutorial: navigate dashboard, interpret trace graphs
- [ ] API documentation: trace-collector REST endpoints (POST /traces/search)
- [ ] Performance tuning guide: ring buffer sizes, polling intervals
- [ ] Compliance: document GDPR/HIPAA data retention policies

**Exit Gate:**
- On-call engineer can troubleshoot in <5 minutes
- New contributor can add custom L7 dissector in <1 hour

**Effort:** S (1 engineer × 1 week)

---

## 6. New BPF Programs / Sophia Dictionaries Needed

### eBPF Programs
| Name | Type | Purpose | Status |
|------|------|---------|--------|
| packet_marker | XDP | Stamp trace_id in IPv6 HbH | **Exists** |
| flow_tracker | TC Egress | Track connection state tuples | **Exists** |
| latency_probe | kprobe (tcp_rcv_established) | Measure RTT via kernel timestamps | **Exists** |
| *payload_dissector* | TC Egress (Phase 3) | Extract L7 headers (HTTP, gRPC, TLS SNI) | **NEW** |
| *app_context* | tracepoint (sched_switch) | Correlate trace_id to userspace PID | **NEW** |

### Sophia Dictionaries
| Name | Purpose | Items | Status |
|------|---------|-------|--------|
| ServiceRegistry | service_id → service_name | 100-10K | **Phase 2** |
| EndpointClassifier | (src_svc, dst_svc, port) → service_type | 1K | **Phase 2** |
| L7Dissectors | port → parser (HTTP, gRPC, TLS) | 50 | **Phase 3** |
| ContextHeaders | trace_id → context map (baggage) | 1M (LRU) | **Phase 2** |

---

## 7. Wotan Topics

### Existing (In Use)
- **traces.packet** (Ring: 64 KB, ~100K msg/s)
  Payload: `{trace_id[8B], src_svc[2B], dst_svc[2B], ts_ns[8B], sport[2B], dport[2B], flags[1B], crc16[2B]}`

- **traces.flow** (Ring: 128 KB, ~50K msg/s)
  Payload: `{trace_id[8B], flow_id[8B], state[1B], packets[8B], bytes[8B], rtt_ns[8B], ts_ns[8B]}`

- **traces.latency** (Ring: 32 KB, ~200K msg/s)
  Payload: `{trace_id[8B], hop_idx[2B], rtt_ns[8B], ts_ns[8B]}`

### New (Phase 2+)
- **traces.enriched** (Ring: 256 KB, ~50K msg/s)
  Payload: Full trace with metadata, src/dst service names, endpoints, context

- **traces.index** (Log: Permanent store, ~10K msg/s)
  Payload: `{trace_id, start_ts, end_ts, src_svc, dst_svc, duration_ns, packet_count, status}`

- **traces.search** (RPC Topic: Query interface)
  Request: `{query: "src_svc=api dst_svc=db", start_ts, end_ts, limit}`
  Response: `[trace_ids matching filter]`

---

## 8. Dashboard Integration

### New Components
- **packet-flow.js** (Phase 1): Real-time graph widget, render nodes/edges from Wotan
- **latency-heatmap.js** (Phase 3): RTT distribution histogram, percentile bands
- **trace-search.js** (Phase 2): Filter UI, saved searches, export to JSON/CSV
- **timeline-view.js** (Phase 3): Annotated timeline of packets per trace

### WebSocket Topics Subscribed
```javascript
// Real-time updates
ws.subscribe("traces.packet", (event) => graph.addEdge(event));
ws.subscribe("traces.flow", (event) => graph.updateFlow(event));
ws.subscribe("traces.latency", (event) => heatmap.addSample(event));

// Search results
ws.subscribe("traces.search", (results) => resultTable.render(results));
```

### Query API (backend, Phase 2)
```
GET /api/v1/traces/search?src_svc=api&dst_svc=db&min_rtt_ms=10&limit=100
GET /api/v1/traces/{trace_id}
GET /api/v1/traces/{trace_id}/hops
GET /api/v1/services
GET /api/v1/services/{svc_id}/dependencies
```

### Visuals
- Service dependency graph (nodes = services, edges = active connections)
- Packet timeline (x=time, y=service depth)
- Latency heatmap (x=percentile, y=service pair, color=RTT)
- Flame graph (service call stack, width=duration)

---

## 9. Testing Strategy

### Unit Tests
- **trace-collector** (80%+ coverage)
  - Map reader (ring buffer deserialization)
  - Enrichment pipeline (service lookup, metadata add)
  - Wotan publisher (gRPC serialization)
  - Error handling (corrupt packets, stale entries)

- **dashboard backend** (80%+ coverage)
  - WebSocket frame handling
  - Query parser (filter syntax)
  - Index search (trace_id lookup)
  - Rate limiter (per-service cap)

- **eBPF programs** (property-based)
  - packet_marker: every packet gets trace_id ✓
  - flow_tracker: state machine (SYN→ESTABLISHED→FIN)
  - latency_probe: RTT within ±5% of kernel timestamp

### Integration Tests
- **eBPF → Wotan** (Phase 1)
  - Start packet_marker + trace-collector
  - Send 10K packets via AF_XDP
  - Verify all 10K appear in traces.packet topic (no loss)
  - Measure latency: packet → Wotan publish (target <1ms p99)

- **trace-collector → Dashboard** (Phase 1)
  - Start trace-collector + dashboard
  - Inject 1K packets
  - Monitor WebSocket for graph updates
  - Verify graph has all 10 nodes (2 services × 5 hops each)

- **Full Stack** (Phase 3)
  - Client → Gateway → Service A → Service B → Service C
  - Inject 3-packet request sequence
  - Verify trace_id propagates through all 3 services
  - Check dashboard renders complete trace

### E2E Tests
- **Browser Automation** (Playwright, Phase 3)
  - Load dashboard
  - Generate traces (send requests via curl)
  - Search traces by service name
  - Verify packet graph renders and is interactive
  - Check heatmap updates in real-time

- **Load Testing** (k6 / vegeta, Phase 4)
  - **Sustained:** 100K pps for 5 minutes → latency histogram
  - **Spike:** 10K → 1M pps → verify recovery time
  - **Chaos:** Kill trace-collector mid-test → measure data loss
  - **Disk spillover:** Fill Wotan ring → verify file buffering

### Performance Benchmarks
| Target | Metric | Success Criteria |
|--------|--------|------------------|
| Throughput | AF_XDP pps | 920K+ (existing benchmark) |
| Latency (p99) | Packet → Dashboard | <100ms |
| CPU | trace-collector @ 100K pps | <5% single core |
| Memory | Dashboard (1M traces) | <500MB |
| Availability | MTBF | >30 days (uptime SLO) |

---

## 10. Dependencies on Other Apps

This is **App #1 of 12** in the Unheaded Monad suite. Tracing feed into:

### Downstream Consumers (Apps 2-12 depend on App #1)
- **App #2: Circuit Breaker** — Uses trace.latency to detect slow services
- **App #3: Load Balancing** — Reads service dependency graph, rebalances based on latency
- **App #4: Auto-Scaling** — Traces → metrics → scaling decisions
- **App #5: Chaos Engineering** — Induces faults, observes via tracing
- **App #7: Cost Attribution** — Allocates infrastructure cost per service pair (traces)
- **App #8: Compliance Audit** — Traces prove data locality (for GDPR)

### Dependencies (App #0 prerequisites)
- **Wotan** (message bus) — publishes/subscribes topics
- **Shield** (gateway) — stamps trace_id on entry
- **Monad** (state management) — stores service_id ↔ service_name mapping
- **Sophia** (knowledge graph) — resolves service metadata, L7 dissectors

### Data Feed Direction
```
App #1 (Tracing)
    ↓ (produces: traces.packet, traces.flow, traces.latency)
    ├→ App #2 (Circuit Breaker)
    ├→ App #3 (Load Balancing)
    ├→ App #4 (Auto-Scaling)
    ├→ App #5 (Chaos)
    ├→ App #7 (Cost)
    └→ App #8 (Compliance)
```

---

## 11. Risk Register

### Risk #1: eBPF Program Crashes Under Load
**Impact:** High (traces unavailable, blind spot in observability)
**Probability:** Medium (kernel BPF verifier edge cases)
**Mitigation:**
- Load eBPF programs with verifier logs enabled; review warnings
- Implement watchdog: monitor map sizes, restart if >80% full
- Use BPF Compiler Collection (BCC) pre-verified programs where possible
- Test on 3+ kernel versions (5.8, 5.15, 6.1)

### Risk #2: Ring Buffer Overflow (Trace Loss)
**Impact:** High (silent data loss, hard to detect)
**Probability:** Medium (bursty traffic, GC pauses in collector)
**Mitigation:**
- Implement metrics: trace_drops_total (counter), buffer_utilization% (gauge)
- Alert if drops >0 in 5-minute window
- Auto-scale ring buffer size based on observed loss rate
- Disk spillover: if Wotan publishing falls behind, buffer to /var/log/traces
- Phase 2: Add redundant TAP (packet sniffing fallback) if ring buffer fails

### Risk #3: Latency Spike: Enrichment Bottleneck
**Impact:** Medium (traces stale by >100ms, dashboard lags)
**Probability:** Medium (ServiceRegistry lookup via Wotan discovery topic)
**Mitigation:**
- Pre-cache service_id → name mapping in trace-collector memory (LRU, 10K entries)
- Implement async enrichment: publish unenriched trace immediately, enrich in background
- Monitor enrichment queue depth; alert if >10K pending
- Phase 2: Add bulk pre-load of ServiceRegistry on startup

---

## 12. Definition of Done

### Phase 1 Complete (Foundation)
- [ ] All eBPF programs load without panics (xdp, tc_egress, kprobe)
- [ ] trace-collector achieves 920K pps on AF_XDP benchmark (sustained)
- [ ] Wotan topics traces.packet/.flow/.latency publish events at real-time rates
- [ ] Dashboard displays mock traces (demo-data.js) with zero service dependency
- [ ] Unit tests: 80%+ coverage for trace-collector
- [ ] Integration test: packet injection → eBPF → Wotan → dashboard (manual verification)
- [ ] Zero data loss measured at 100K pps (5-minute sustained load)

### Phase 2 Complete (Enrichment)
- [ ] Service names appear in dashboard for every trace
- [ ] Trace search UI filters by src/dst service (works with query API)
- [ ] Anamnesis snapshots captured per hop, stored in traces.enriched topic
- [ ] Context propagation verified: trace_id persists through 10 hops
- [ ] Sophia integration: endpoint classification works for 100+ service pairs
- [ ] Integration test: end-to-end trace with metadata at each hop

### Phase 3 Complete (Visualization)
- [ ] packet-flow.js renders graphs with 1000 nodes in <100ms
- [ ] WebSocket updates at 100 Hz with browser frame rate (16.67 Hz goal, not 100 Hz)
- [ ] Latency heatmap shows RTT distribution for top 20 service pairs
- [ ] Search reduces dataset from 10K to <100 traces in <500ms
- [ ] E2E tests pass: browser automation renders traces interactively

### Phase 4 Complete (Hardening)
- [ ] Load test: 1M pps sustained for 1 hour, <0.1% loss
- [ ] Circuit breaker: eBPF→Wotan publishing survives Wotan disconnect
- [ ] Chaos test: kill trace-collector mid-test, verify recovery + no data loss
- [ ] Metrics: traces_published_total, trace_latency_seconds visible in Prometheus
- [ ] Health checks: eBPF map readability, Wotan connectivity monitored

### Phase 5 Complete (Documentation)
- [ ] Troubleshooting guide published (GitHub wiki)
- [ ] Runbook: disable tracing in 5 minutes without restart
- [ ] Extension guide: add custom L7 dissector with code example
- [ ] Performance tuning guide: ring buffer sizing, polling intervals
- [ ] On-call engineer can debug in <5 minutes (measured)

### Final Acceptance
- [ ] Tracing system is the **sole source of observability truth** (owns all L2-L7 visibility)
- [ ] **Zero sidecar overhead** proven: apps run at full speed
- [ ] **Cost per-pps** is 95% cheaper than Istio/Datadog (measured infra cost)
- [ ] **100% trace capture** with no sampling, no data loss
- [ ] **<100ms latency** from packet to dashboard render
- [ ] **Production-ready**: passes security audit, SLA metrics, chaos tests

---

## Appendix: Rollout Timeline

```
Week 1-2: Phase 1 (Foundation)
  └─ eBPF programs + trace-collector integration

Week 3-4: Phase 2 (Enrichment)
  └─ Service metadata, context propagation

Week 5-7: Phase 3 (Visualization)
  └─ Dashboard interactive graphs, heatmaps

Week 8-9: Phase 4 (Hardening)
  └─ Load testing, chaos, production readiness

Week 10: Phase 5 (Documentation)
  └─ Runbooks, guides, team training

Target: Production-ready by Week 10 (Q1 2026)
```

---

**Author:** Unheaded Tracing Team
**Version:** 1.0 (March 4, 2026)
**Next Review:** Phase 1 completion + go/no-go for Phase 2
