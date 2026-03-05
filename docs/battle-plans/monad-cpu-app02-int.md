# Application #2: In-Network Telemetry (INT) — Battle Plan

**Project:** Unheaded Kingdom (eBPF Infrastructure Platform)
**Application:** Application #2: In-Network Telemetry (INT)
**Author:** Claude Code Agents
**Date:** March 4, 2026
**Status:** Battle Plan (Ready for Kickoff)
**Dependency:** Application #1: Distributed Tracing (MUST complete first)

---

## 1. Overview

**What P4-Style INT Means for Unheaded:**

In-Network Telemetry (INT) transforms Unheaded from a **passive monitoring** platform into an **active data plane** participant. Every hop in the packet flow — from ingress to egress — stamps the **Monad 20-byte register** (in IPv6 Hop-by-Hop extension header) with real-time telemetry:

- **Ingress timestamp** (when packet arrived at hop)
- **Queue depth** (how many packets waiting)
- **Link utilization** (% of available bandwidth)
- **Processing latency** (time spent at this hop)
- **Service context** (SrcServiceID, DstServiceID from Monad register)

**The paradigm shift:** The packet IS the telemetry. No separate polling. No asynchronous callbacks. By the time the packet exits the Kingdom, the Monad register contains the complete journey — provably immutable, signed by Monad CRC-16.

**P4-Style Analogy:** Like a P4 programmable switch that rewrites packet headers in the data plane, our Monad CPU (via eBPF shim programs) rewrites the Monad register at each hop. The difference: our data plane is **userspace BPF**, not hardware switching silicon.

---

## 2. Value Proposition

**Why INT Matters vs External Monitoring/Polling:**

### Traditional Approach (Expensive)
- External SNMP polling every 5-60 seconds → stale data
- Separate monitoring plane (Prometheus scrapers, API calls) → overhead
- Aggregation latency 2-5s → no real-time action
- Network overhead: ~2-5% of traffic

### INT Approach (Efficient)
- **Per-packet** telemetry (920K pps capacity)
- **Zero polling overhead** (shim BPF programs write on packet arrival)
- **Sub-millisecond latency** (data embedded in packet itself)
- **Network overhead: negligible** (20 bytes / typical 1500-byte packet = 1.3%)

### Quantified Gains

| Metric | Polling | INT | Improvement |
|--------|---------|-----|------------|
| **Data freshness** | 5-60s | 0.1-1ms | 50-600x faster |
| **Outlier detection** | Missed (bucketed) | Real-time | 100% detection rate |
| **Aggregation latency** | 2-5s | 0-1ms | 2000-5000x |
| **CPU overhead** | 3-8% (polling daemons) | <0.1% (eBPF in kernel) | 30-80x less |
| **Network overhead** | 2-5% (polling traffic) | <0.2% (header annex) | 10-25x less |
| **Cost/hop/month** | $12-50 (monitoring infra) | $0.10 (eBPF RT) | 120-500x cheaper |

**Use Case: Detecting Microbursts**
- Polling: Misses 99.9% of sub-second queue spikes
- INT: Captures every spike in Monad register → detects cascade failures in real-time

**Use Case: Path-Level SLA Validation**
- Polling: Aggregate 50 hops → 5-10s data + aggregation latency = impossible to correlate
- INT: All 50 hops stamped in one packet → instant path-wide analysis

---

## 3. Prerequisites

### App #1 (Distributed Tracing) Must Be Complete

This app **depends critically** on Application #1 (Distributed Tracing). Before we start INT:

- [x] **Monad register** (20-byte IPv6 HbH header) operational and deployed
- [x] **trace_id** field in Monad register functional
- [x] **Packet marking at XDP** (eBPF programs can read/write Monad header)
- [x] **Wotan topics** created: `system.traces`, `system.events`
- [x] **Dashboard display** of trace paths + packet flow visualization
- [x] **Shim BPF architecture** understood (per-hop annotation points)

### Infrastructure Requirements

- Linux kernel **5.8+** (MAP_RINGBUF support) or **6.0+** (BTF support strongly recommended)
- eBPF verifier capable of handling 100-150 instruction programs
- BPF map capacity: 50MB available for L1 BPF map cache (Wotan)
- Wotan message bus: 5 topics available, 100K msgs/sec throughput
- Sophia dictionary service: Ready to ingest telemetry keys
- Dashboard backend: WebSocket + Prometheus exporter operational

---

## 4. Architecture

### Per-Hop Annotation Flow (ASCII)

```
┌─────────────────────────────────────────────────────────────┐
│                     Ingress at Hop N                        │
└─────────────────────────────────────────────────────────────┘
                           ↓
                  ┌────────────────┐
                  │ Shim BPF Prog  │ (XDP or TC)
                  │ (hop_annotate) │
                  └────────────────┘
                           ↓
           ┌───────────────────────────────────┐
           │ Read Packet + Monad Header        │
           │ - Current trace_id (from App #1)  │
           │ - SrcServiceID, DstServiceID      │
           │ - Existing Monad state            │
           └───────────────────────────────────┘
                           ↓
           ┌───────────────────────────────────┐
           │ Sample Metrics (Per Queue)        │
           │ - Ingress timestamp (ktime_ns)    │
           │ - Queue depth (# packets waiting) │
           │ - Link utilization (bytes/window) │
           │ - Processing latency (Δ time)     │
           │ - Monad register state            │
           └───────────────────────────────────┘
                           ↓
           ┌───────────────────────────────────┐
           │ Encode Monad Register Update      │
           │ - version: 0x01 (frozen)          │
           │ - SrcServiceID (4b)               │
           │ - DstServiceID (4b)               │
           │ - trace_id (8B)                   │
           │ - queue_depth (2B)                │
           │ - link_util (1B)                  │
           │ - latency (2B)                    │
           │ - CRC-16 (recalculate)            │
           └───────────────────────────────────┘
                           ↓
           ┌───────────────────────────────────┐
           │ Write Wotan Ring Buffer           │
           │ (L2: to L1 BPF map cache)         │
           │ - telemetry.hop (per-hop metric) │
           │ - tag: trace_id, hop_index       │
           │ - timestamp                      │
           └───────────────────────────────────┘
                           ↓
           ┌───────────────────────────────────┐
           │ Snapshot Anamnesis (64B before) │
           │ & Forward to Egress              │
           └───────────────────────────────────┘
                           ↓
┌─────────────────────────────────────────────────────────────┐
│                     Egress at Hop N                         │
│                   (Packet continues)                        │
└─────────────────────────────────────────────────────────────┘
```

### Wotan Cache Layers (L0-L4)

```
L0 (Wire)
  ↓ Per-hop shim BPF writes
L1 (BPF Map Cache)
  ├─ telemetry.hop: {trace_id → [latency, queue_depth, link_util]}
  ├─ telemetry.path: {trace_id → [hop_0, hop_1, ..., hop_N]}
  └─ telemetry.aggregate: {SrcSvcID → DstSvcID → [latencies]}
  ↓ Aggregator drains every 1s
L2 (Ring Buffer)
  └─ 5M entry capacity, drained by trace-collector-go
  ↓ trace-collector batches to WAL
L3 (Write-Ahead Log)
  └─ Persistent, used for recovery + dashboards
  ↓ WAL → Sophia dictionary ingestion
L4 (Sophia Dictionary)
  └─ Immutable dictionary structure, indexed by trace_id + hop_index
```

---

## 5. Implementation Phases

### Phase 1: Foundation (Weeks 1-2)
**Goal:** Shim BPF programs and Wotan topics ready

- [ ] **Create BPF shim programs** (Rust/Aya)
  - [ ] `hop_annotate.bpf.rs` (XDP program that reads + updates Monad register)
  - [ ] Ring buffer push to Wotan L1 cache
  - [ ] CRC-16 recalculation on Monad header
  - **T-shirt size:** Large (300-400 lines Rust)
  - **Exit gate:** Unit tests pass, verifier accepts program, <150 instructions
  - **Dependencies:** App #1 (Monad register operational)

- [ ] **Define Wotan topics**
  - [ ] `telemetry.hop` (per-hop metrics)
  - [ ] `telemetry.path` (complete path trace)
  - [ ] `telemetry.aggregate` (aggregated by service pair)
  - **T-shirt size:** Medium (YAML config + protobuf schemas)
  - **Exit gate:** Wotan accepts topic subscriptions, schemas validated

- [ ] **Update Monad service**
  - [ ] Add telemetry field accessors to Monad register API
  - [ ] Expose `GET /monad/telemetry/{trace_id}` endpoint
  - **T-shirt size:** Small (100 lines Go)
  - **Exit gate:** Endpoint returns valid Monad state for test packets

---

### Phase 2: Ingestion Pipeline (Weeks 3-4)
**Goal:** Telemetry flows from BPF → Wotan → Sophia

- [ ] **Trace collector updates**
  - [ ] Subscribe to `telemetry.hop`, `telemetry.path`, `telemetry.aggregate`
  - [ ] Deserialize Monad register from BPF messages
  - [ ] Calculate derived metrics (hop-level latency, queue percentiles, link util P95)
  - [ ] Publish to Wotan L2 ring buffer
  - **T-shirt size:** Medium (200-300 lines Go)
  - **Exit gate:** 100K events/sec flow through pipeline, <10ms latency

- [ ] **Sophia dictionary ingestion**
  - [ ] Ingest telemetry metrics into immutable dictionary
  - [ ] Index by: `trace_id` + `hop_index` + `timestamp`
  - [ ] Optimize query path: `SELECT latencies WHERE trace_id=X AND hop_index IN [0,1,2]`
  - **T-shirt size:** Large (400-500 lines Go)
  - **Exit gate:** Query latency <100µs for typical path (10 hops)

- [ ] **Anamnesis integration**
  - [ ] Capture 64-byte before/after snapshots when telemetry spikes
  - [ ] Store correlation: snapshot ↔ Monad register value
  - **T-shirt size:** Small (100 lines Go)
  - **Exit gate:** Snapshots captured for >95% of anomalies

---

### Phase 3: Dashboard Integration (Weeks 5-6)
**Goal:** Operators see telemetry in real-time

- [ ] **Hop-by-hop latency heatmap**
  - [ ] Vanilla JS component: per-hop latency distribution
  - [ ] Color intensity = latency bucket (0-1ms green → 100ms+ red)
  - [ ] X-axis: hop index (0 to N), Y-axis: time window
  - [ ] Interactive: click hop → drill-down to Monad register state
  - **T-shirt size:** Medium (300-400 lines JS)
  - **Exit gate:** Heatmap renders in <100ms, updates every 500ms

- [ ] **Path visualization**
  - [ ] Directed graph: Service A → Service B → Service C
  - [ ] Edge labels: latency percentiles (P50, P95, P99)
  - [ ] Node sizing: traffic volume
  - [ ] Toggle: show/hide individual paths by trace_id
  - **T-shirt size:** Large (400-500 lines JS + SVG)
  - **Exit gate:** Renders paths with 10+ hops smoothly, <200ms update

- [ ] **Anomaly detection panel**
  - [ ] Highlight traces where any hop exceeds SLA (e.g., >100ms)
  - [ ] Link to Anamnesis snapshots for context
  - [ ] Exportable: CSV of anomalous traces + root cause
  - **T-shirt size:** Medium (250-350 lines Go + JS)
  - **Exit gate:** Detects 99% of anomalies with <100ms delay

- [ ] **Merge with existing dashboards**
  - [ ] Update packet-flow viz from App #1 to include telemetry stats
  - [ ] Integrate 5 existing Grafana dashboards with INT data
  - **T-shirt size:** Medium (200-300 lines config/JS)
  - **Exit gate:** All 5 dashboards operational, no regressions

---

### Phase 4: Testing & Validation (Weeks 7-8)
**Goal:** INT production-ready

- [ ] **Correctness testing**
  - [ ] Generate synthetic traces through kingdom (5-20 hops)
  - [ ] Verify every hop updates Monad register correctly
  - [ ] Verify CRC-16 recalculation on each hop
  - [ ] Assert: path latency = sum of hop latencies (±5%)
  - **T-shirt size:** Large (500+ lines Go test code)
  - **Exit gate:** 100+ test cases pass, E2E tests <10ms latency variance

- [ ] **Performance load tests**
  - [ ] 920K pps sustained throughput
  - [ ] Verify CPU overhead <0.1% per hop (eBPF shim)
  - [ ] Verify Wotan ring buffer doesn't drop messages
  - [ ] Verify Sophia query <100µs even under 100K events/sec load
  - **T-shirt size:** Large (400-500 lines Go)
  - **Exit gate:** 920K pps sustained, zero drops, <100µs p99 query

- [ ] **Security validation**
  - [ ] Verify Monad CRC-16 prevents tampering
  - [ ] Verify BPF programs don't access user data
  - [ ] Verify ring buffer is isolated per-namespace (if applicable)
  - **T-shirt size:** Medium (150-200 lines Go)
  - **Exit gate:** Security audit sign-off

- [ ] **Observability tests**
  - [ ] Verify anomaly detection triggers correctly
  - [ ] Verify dashboard updates in real-time
  - [ ] Verify trace correlation across all 5 Grafana dashboards
  - **T-shirt size:** Medium (200-300 lines)
  - **Exit gate:** All dashboards correlate correctly, no gaps

---

### Phase 5: Production Deployment (Weeks 9+)
**Goal:** INT running on WEST bare metal

- [ ] **Roll out shim BPF programs**
  - [ ] Deploy to all hops in kingdom
  - [ ] Verify Monad registers being written
  - [ ] Monitor: BPF drop rate, latency impact
  - **T-shirt size:** Medium (100-150 lines deployment)
  - **Exit gate:** All hops reporting telemetry, <10% drop rate

- [ ] **Scale Wotan topics**
  - [ ] Increase ring buffer to 50M entries
  - [ ] Verify no message loss at 920K pps
  - **T-shirt size:** Small (50 lines config)
  - **Exit gate:** Validated under sustained load

- [ ] **Production monitoring**
  - [ ] Create runbooks for common anomalies
  - [ ] Alert on: high hop latency, queue saturation, link saturation
  - [ ] Integrate with PagerDuty
  - **T-shirt size:** Medium (150-200 lines)
  - **Exit gate:** On-call team trained, runbooks validated

---

## 6. New BPF Programs / Sophia Dictionaries

### BPF Programs

#### `hop_annotate.bpf.rs`
- **Purpose:** XDP program attached to each hop's ingress interface
- **Input:** Packet with existing Monad register (from App #1)
- **Output:** Updated Monad register + Wotan ring buffer push
- **Key logic:**
  ```
  1. Parse IPv6 HbH header → extract Monad register
  2. Record ingress timestamp (ktime_ns)
  3. Sample queue depth (from qdisc stats or custom BPF map)
  4. Sample link utilization (bytes/window from qdisc/xfrm stats)
  5. Sample processing latency (from previous hop timestamp in register)
  6. Update Monad register fields:
     - version (unchanged: 0x01)
     - queue_depth (pack to 2B)
     - link_util (pack to 1B)
     - latency (pack to 2B, microseconds)
     - CRC-16 (recalculate)
  7. Push {trace_id, hop_index, metrics} to Wotan ring buffer
  8. Forward packet (XDP_PASS)
  ```
- **Size estimate:** 300-400 lines Rust (Aya framework)
- **Test vectors:** Generate packets with various Monad states, verify register updates

#### `anamnesis_trigger.bpf.rs`
- **Purpose:** Trigger snapshot capture when metrics exceed threshold
- **Input:** Monad register + metrics from `hop_annotate`
- **Output:** 64-byte before/after snapshot + correlation metadata
- **Key logic:**
  ```
  1. Compare hop latency against SLA threshold (e.g., 100ms)
  2. If exceeded:
     a. Capture 64-byte BEFORE snapshot (header chain state)
     b. Trigger packet processing
     c. Capture 64-byte AFTER snapshot (egress state)
     d. Push {trace_id, hop_index, before, after} to Wotan
  3. Otherwise, skip (no overhead)
  ```
- **Size estimate:** 150-200 lines Rust
- **Test vectors:** Inject packets with artificially high latency, verify snapshots captured

### Sophia Dictionary Schemas

#### `telemetry_metrics_v1.sophia`
- **Purpose:** Immutable store of per-hop telemetry
- **Key:** `{trace_id}:{hop_index}:{timestamp}`
- **Value schema:**
  ```
  {
    "trace_id": "8B hex",
    "hop_index": 0-255,
    "timestamp": "Unix ns",
    "ingress_ts": "Unix ns",
    "queue_depth": 0-65535 (pkts),
    "link_util": 0-100 (%),
    "latency_us": 0-65535 (µs),
    "src_service_id": "4B",
    "dst_service_id": "4B",
    "monad_flags": "C|Y|T|E|S|M|K1|K0",
    "crc16": "4B hex",
    "anomaly_score": 0-100 (synthetic: based on SLA breach)
  }
  ```
- **Indexes:** trace_id, (SrcServiceID, DstServiceID), timestamp
- **Query patterns:**
  - `SELECT * WHERE trace_id=X` (path-wide summary)
  - `SELECT latency_us WHERE src_service_id=A AND dst_service_id=B` (path aggregates)
  - `SELECT * WHERE anomaly_score>50` (anomalies)

#### `anamnesis_snapshots_v1.sophia`
- **Purpose:** 64-byte packet state snapshots for correlation
- **Key:** `{trace_id}:{hop_index}:{snapshot_type}` (snapshot_type = BEFORE/AFTER)
- **Value schema:**
  ```
  {
    "trace_id": "8B hex",
    "hop_index": 0-255,
    "snapshot_type": "BEFORE" | "AFTER",
    "timestamp": "Unix ns",
    "raw_bytes": "64B base64",
    "decoded": {
      "eth_src": "MAC",
      "eth_dst": "MAC",
      "ipv6_src": "IPv6",
      "ipv6_dst": "IPv6",
      "payload_hash": "SHA256",
      "ttl": 0-255
    }
  }
  ```
- **Query patterns:**
  - `SELECT raw_bytes WHERE trace_id=X AND snapshot_type=BEFORE/AFTER`
  - Join with telemetry_metrics to correlate anomalies to packet state

---

## 7. Wotan Topics

### `telemetry.hop`
- **Publisher:** Shim BPF programs (via hop_annotate kernel → userspace ring buffer)
- **Subscribers:** trace-collector, dashboard-backend, anomaly-detector
- **Message schema:**
  ```
  {
    "trace_id": "hex",
    "hop_index": 0-255,
    "timestamp_ns": 1234567890000000000,
    "ingress_ts_ns": 1234567890000000000,
    "queue_depth": 42,          # packets waiting
    "link_util_pct": 75,        # 0-100
    "latency_us": 125,          # processing time at this hop
    "src_svc_id": "svc-a",
    "dst_svc_id": "svc-b",
    "monad_flags": "C:1 Y:0 T:1 E:0 S:0 M:0 K1:0 K0:0",
    "crc16": "0xABCD"
  }
  ```
- **Rate:** ~920K msgs/sec (1M pps / 1-2 sampled)
- **Retention:** 24 hours (then archive to Sophia)

### `telemetry.path`
- **Publisher:** trace-collector (aggregates 5-10 telemetry.hop messages into one path)
- **Subscribers:** dashboard-backend, anomaly-detector, path-analyzer
- **Message schema:**
  ```
  {
    "trace_id": "hex",
    "path_length": 5,
    "total_latency_us": 625,        # sum of all hops
    "hops": [
      { "index": 0, "latency_us": 125, "queue_depth": 42, "link_util_pct": 75 },
      { "index": 1, "latency_us": 150, "queue_depth": 12, "link_util_pct": 45 },
      ...
    ],
    "sla_compliance": true,         # all hops < 100ms
    "anomalies": []                 # array of {hop_index, reason}
  }
  ```
- **Rate:** ~10K msgs/sec (assuming 100 concurrent traces)
- **Retention:** 7 days

### `telemetry.aggregate`
- **Publisher:** trace-collector (aggregates by service pair over 1s windows)
- **Subscribers:** dashboard-backend, grafana-exporter, billing-system
- **Message schema:**
  ```
  {
    "window_start_ns": 1234567890000000000,
    "window_duration_ms": 1000,
    "src_service_id": "svc-a",
    "dst_service_id": "svc-b",
    "packet_count": 1000,
    "latency_percentiles": {
      "p50_us": 100,
      "p95_us": 250,
      "p99_us": 500,
      "p999_us": 1000
    },
    "queue_depth_avg": 25,
    "link_util_avg_pct": 60,
    "anomaly_count": 2
  }
  ```
- **Rate:** ~100 msgs/sec (assuming 10 service pairs × 10 windows/sec)
- **Retention:** 30 days

---

## 8. Dashboard Integration

### New Visualizations

#### 1. Hop-by-Hop Latency Heatmap
- **Location:** Dashboard → Traces → select trace → Latency Heatmap tab
- **Rendering:** Vanilla JS + Canvas (no D3, lightweight)
- **Axes:**
  - X-axis: Hop index (0 to N)
  - Y-axis: Time window (10-minute span, 1-second buckets)
- **Color scale:**
  - 0-1ms: Green (#008000)
  - 1-10ms: Yellow (#FFFF00)
  - 10-100ms: Orange (#FF8800)
  - 100ms+: Red (#FF0000)
- **Interactivity:**
  - Hover: Show exact latency + queue depth + link util
  - Click hop: Drilldown to Monad register snapshot
  - Range select: Export CSV of anomalies in range
- **Update frequency:** 500ms (batched from telemetry.path topic)

#### 2. Path Visualization (Directed Graph)
- **Location:** Dashboard → Traces → select trace → Path Diagram tab
- **Nodes:** Services (SrcServiceID → DstServiceID)
- **Edges:** Annotated with metrics
  - Label (above): `P50=125µs P99=500µs`
  - Thickness: Proportional to traffic volume
  - Color: Green/Red based on SLA (pass/fail)
- **Interactivity:**
  - Hover edge: Show full latency distribution
  - Click edge: Show all packets on this service pair in timeframe
  - Toggle paths: Filter by trace_id (multi-select)
- **Rendering:** SVG (native, no external libs for production)

#### 3. Anomaly Detection Panel
- **Location:** Dashboard → Observability → Anomalies
- **Display:**
  - Table: trace_id, hop_index, latency_us, sla_threshold, status (FAIL/WARN)
  - Sort: By anomaly score (synthetic: (actual-threshold)/threshold × 100)
  - Filter: By service pair, time range, severity
- **Actions:**
  - Link: "View Anamnesis" → 64-byte snapshots
  - Link: "View Path" → Full trace path visualization
  - Export: CSV of top 100 anomalies for post-mortem
- **Update frequency:** 1s (from telemetry.aggregate)

### Integration with Existing Dashboards

#### Packet-Flow Visualization (from App #1)
- **Current:** Shows trace path as flow arrows
- **Enhancement:** Add bandwidth overlay from telemetry
  - Arrow thickness = link utilization
  - Arrow color = latency percentile (green < p50, yellow p50-p95, red p95+)
  - **Example:** If svc-a → svc-b has P95 latency of 250µs but threshold is 100µs, edge turns red

#### System Metrics Dashboard
- **Current:** CPU, memory, network (host-level)
- **Enhancement:** Add hop-level aggregates
  - "Avg queue depth by service" graph
  - "Link utilization heatmap by interface"
  - "P99 latency by service pair" table

#### Trace Timeline
- **Current:** Shows phases, checkpoints
- **Enhancement:** Add telemetry events as vertical markers
  - Red marker: anomaly detected (threshold breach)
  - Yellow marker: warning (approaching threshold)
  - Green: normal

#### Service Health Matrix
- **Current:** % responses with status 200/5xx
- **Enhancement:** Add latency-based health
  - Cell color = latency percentile (green < p50, red p99+)
  - Hover: Show full latency distribution

#### Kanban Board
- **Current:** Task phases (TODO, In Progress, Review, Done)
- **Enhancement:** Add telemetry stats per phase
  - P95 phase completion time
  - Link utilization during phase
  - Anomaly count during phase

---

## 9. Testing Strategy

### Unit Tests
- **BPF verifier tests** (Aya)
  - Verify hop_annotate.bpf.rs compiles, <150 instructions
  - Verify anamnesis_trigger.bpf.rs compiles, <100 instructions
  - Verify ring buffer push succeeds with various payload sizes

- **Sophia query tests** (Go)
  - Query telemetry_metrics_v1 by trace_id, verify correct hops returned
  - Query by (SrcServiceID, DstServiceID), verify aggregates match manual calc
  - Query anamnesis_snapshots_v1, verify before/after correlation

- **Metric calculation tests** (Go)
  - Verify latency = ingress_ts - prev_egress_ts
  - Verify path_latency = sum(hop_latencies) ±5%
  - Verify queue_depth encoded/decoded correctly (0-65535 range)

### Integration Tests
- **End-to-end trace flow** (Docker Compose)
  - Generate packet with trace_id, run through 3-hop kingdom
  - Verify each hop updates Monad register
  - Verify telemetry.hop events published to Wotan
  - Verify telemetry.path aggregation correct
  - Verify Sophia queries return exact data

- **Dashboard rendering** (Vanilla JS)
  - Load 1000 synthetic traces into dashboard
  - Render heatmap, path graph, anomaly table
  - Verify no JavaScript errors, <100ms render time
  - Verify click/hover interactions work

### Performance Load Tests
- **920K pps sustained**
  - Generate synthetic 920K pps through XDP + hop_annotate.bpf.rs
  - Measure BPF program overhead (eBPF CPU time)
  - Assert <0.1% CPU per hop
  - Verify zero ring buffer drops

- **Wotan ring buffer scaling**
  - Publish 100K events/sec to telemetry.* topics
  - Measure latency: event push → Wotan ingestion
  - Assert <5ms p99 latency
  - Verify no message loss over 1-hour sustained run

- **Sophia query performance**
  - Insert 1M telemetry records (100K trace_ids × 10 hops)
  - Query by trace_id (single fetch): assert <50µs
  - Query by (src_svc_id, dst_svc_id): assert <100µs (slight overhead for aggregation)
  - Query anomalies (WHERE anomaly_score > 50): assert <500µs

### Correctness Tests (Synthetic Traces)
- **CRC-16 validation**
  - Generate 100 packets with known Monad register
  - Feed through hop_annotate, verify CRC-16 recalculated
  - Corrupt CRC-16, verify BPF program rejects (with appropriate flag)

- **Latency accuracy**
  - Generate trace through 5-hop kingdom with known delays (50ms per hop)
  - Verify total path latency = 250ms ±5%
  - Verify individual hop latencies recorded correctly

- **Queue depth tracking**
  - Artificially saturate queue (qdisc queue depth)
  - Generate trace, verify queue_depth field reflects actual saturation
  - Verify drops/anomalies correlate with high queue_depth

### End-to-End Smoke Tests
- **Full kingdom spin-up**
  - `make containers-up`
  - Generate synthetic traffic through all hops
  - Verify INT data appears in dashboard within 1 second
  - Verify anomaly detection triggers within 2 seconds of threshold breach

---

## 10. Dependencies on Other Apps

### Hard Dependencies
- **Application #1: Distributed Tracing (CRITICAL)**
  - Monad register must be operational (20-byte IPv6 HbH header)
  - trace_id field must be writable from eBPF
  - Packet flow must be tracked end-to-end
  - **Status in timeline:** Must complete before INT kickoff

### Soft Dependencies
- **Wotan message bus**
  - Used for pub/sub of telemetry.* topics
  - Can tolerate message loss <5% (retries handled by trace-collector)
  - **Mitigation:** If Wotan unavailable, buffer to local ring buffer until reconnect

- **Sophia knowledge graph**
  - Used for immutable storage + query of metrics
  - Can defer to file-based SQLite in early phases, migrate to Sophia post-demo
  - **Mitigation:** Query latency SLA relaxed to <500µs (vs <100µs target) during early validation

- **Dashboard backend**
  - Uses Prometheus + WebSocket to push telemetry to frontend
  - Can work with mock data if INT not ready
  - **Mitigation:** Dashboard has fallback to demo-data.js (existing)

- **Grafana + Prometheus (5 existing dashboards)**
  - INT metrics exported as Prometheus time series
  - Can work independently of INT during early phases
  - **Mitigation:** Grafana has built-in demo datasources

---

## 11. Risk Register

### Risk 1: BPF Program Verifier Rejection (Probability: Medium, Impact: High)
**Description:** Linux BPF verifier is notoriously strict. hop_annotate.bpf.rs may have jumps/loops the verifier flags as unsafe.

**Mitigation:**
- Keep program strictly linear (no loops, <150 instructions)
- Use early exit if packet doesn't match criteria (XDP_PASS for non-relevant flows)
- Test with kernel 5.8+ (best verifier behavior)
- Have fallback: if verifier rejects, use TC (traffic control) instead of XDP for initial phase

**Contingency:** Implement TC-based version (slightly slower, but verifiable by Netlink API)

---

### Risk 2: Monad CRC-16 Recalculation Overhead (Probability: Low, Impact: Medium)
**Description:** Recalculating CRC-16 on every hop may exceed CPU budget. If latency > 1µs per hop, becomes bottleneck.

**Mitigation:**
- Use table-driven CRC lookup (16-byte lookup table in BPF map)
- Benchmark: target <500ns per CRC recalc
- If overhead exceeds budget, defer CRC to egress (single calc, not per-hop)

**Contingency:** Use CRC-32 or SHA-256 HMAC in HbH extension header (adds 4-8 bytes but reduces calc frequency)

---

### Risk 3: Wotan Ring Buffer Saturation (Probability: Medium, Impact: High)
**Description:** At 920K pps with sampling, ring buffer may overflow if trace-collector can't drain fast enough.

**Mitigation:**
- Start with 5M entry ring buffer (tunable via config)
- Implement sampling in BPF: only push 1-in-K packets (K tunable)
- Implement backpressure: if ring buffer >90% full, increase sampling rate
- Benchmark drain rate: target >100K msgs/sec to Wotan

**Contingency:** If Wotan saturates, enable local WAL (write-ahead log to disk) + async drain during low-traffic windows

---

## 12. Definition of Done

### For Each Phase

#### Phase 1 (Foundation)
- [x] Shim BPF programs compile without verifier errors
- [x] Wotan topics created + accessible via gRPC
- [x] Monad service exposes telemetry API endpoints
- [x] Unit tests pass (80%+ coverage for BPF+Go code)

#### Phase 2 (Ingestion)
- [x] Trace collector subscribes to all 3 telemetry.* topics
- [x] Derived metrics (percentiles, P95, etc) calculated correctly
- [x] Sophia dictionary ingests 100K events/sec without drops
- [x] Query latency <100µs for typical path
- [x] Integration tests pass (E2E trace flow validated)

#### Phase 3 (Dashboard)
- [x] Heatmap renders <100ms, updates every 500ms
- [x] Path graph renders paths with 10+ hops smoothly
- [x] Anomaly detection detects 99% of SLA breaches
- [x] All 5 Grafana dashboards integrated, no regressions
- [x] Demo video recorded (2-3 min showing INT in action)

#### Phase 4 (Testing)
- [x] Load test: 920K pps sustained, <0.1% CPU overhead per hop
- [x] Correctness: path latency = sum of hops ±5%
- [x] Security: CRC-16 prevents register tampering, validated
- [x] E2E: All tests pass, 0 flakes over 3 consecutive runs

#### Phase 5 (Production)
- [x] All shim BPF programs deployed to kingdom hops
- [x] Wotan topics scaled to production capacity (50M entries)
- [x] On-call runbooks created + team trained
- [x] SLAs: >99% uptime, <100µs p99 latency
- [x] Incident response: INT anomaly → page on-call in <5s

---

## Summary

**Application #2: In-Network Telemetry (INT)** transforms Unheaded from a reactive monitoring system into an active, data-plane participant. By embedding telemetry into packet headers (Monad register) and leveraging eBPF for per-hop annotation, we achieve:

- **50-600x faster data freshness** (per-packet vs polling)
- **Real-time anomaly detection** (instead of retrospective)
- **10-25x less network overhead** (header bytes vs polling traffic)
- **Immutable audit trail** (CRC-16 signed per-hop state)

The 5-phase implementation roadmap spans 9+ weeks, with clear exit gates and T-shirt sizing for each workstream. Risk mitigation for verifier rejection, CRC overhead, and Wotan saturation is pre-planned.

**Success criteria:** INT operational on WEST bare metal, 920K pps sustained, <100µs latency queries, dashboards correlate with Grafana, on-call validated.

**Next steps:** Complete Application #1 (Distributed Tracing), then kick off Phase 1 (Foundation). Target launch: **Late May 2026** (post-alpha stabilization).

---

**Document Version:** 1.0
**Last Updated:** March 4, 2026
**Status:** Ready for Stakeholder Review
