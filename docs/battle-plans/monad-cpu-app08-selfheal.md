# MONAD CPU APP #8: SELF-HEALING NETWORK (SUB-MS FAILOVER) — HIGH-LEVEL BATTLE PLAN

**Date**: 2026-03-04
**Application**: Monad CPU App #8 — Self-Healing Network (Sub-ms Failover)
**Prerequisite**: Monad CPU core (eBPF + Sophia + Wotan) operational, Anamnesis event ring functional, Shield XDP pipeline active
**Target**: Deploy sub-millisecond failover detection + automatic traffic rerouting via circuit breaker pattern in BPF
**Estimated Duration**: 14-21 hours across 3-4 sessions
**Success Metric**: Detect endpoint failure in <500μs, execute failover at packet speed, zero packet loss during recovery

---

## EXECUTIVE OVERVIEW

**The Problem**: Traditional health checks poll every 5-30 seconds. During failure, users experience 5-30 seconds of degradation.

**The Solution**: Embed health signals in every packet. BPF programs maintain real-time endpoint health scores. When health drops below threshold, XDP redirects traffic to backup at packet speed — no DNS wait, no drain interval, no external health check polling.

**Key Innovation**: Circuit breaker state machine (CLOSED → HALF_OPEN → OPEN) lives in eBPF, updated per-packet, automatic recovery detection via Wotan state persistence.

---

## VALUE PROPOSITION

### Monad vs. Traditional Approaches

| Metric | Traditional (Envoy/HAProxy/AWS ALB) | Monad CPU App #8 |
|--------|--------------------------------------|------------------|
| **Detection Latency** | 5-30 seconds (health check interval) | 500 microseconds (per-packet) |
| **Failover Time** | 5-30+ seconds | Sub-millisecond (XDP redirect) |
| **DNS TTL Wait** | 60-300+ seconds | None (packet-level routing) |
| **LB Drain Time** | 10-30 seconds | None (atomic redirect) |
| **Observability** | External (separate health service) | Embedded in data plane (Monad header) |
| **Cascading Failure Detection** | Manual tuning per endpoint | Automatic via flow_tracker (BPF isolation) |
| **Automatic Recovery** | Manual intervention | Gradual traffic restoration (HALF_OPEN) |
| **Infrastructure Overhead** | Dedicated health check infrastructure | None (packet-embedded) |
| **Blast Radius Containment** | Manual circuit breaker setup | Automatic via Wotan flow analytics |

---

## PREREQUISITES

1. **Monad Core Operational**
   - XDP programs loaded (`ebpf/xdp_*.rs`)
   - Sophia dictionaries available (`endpoint_health`, `circuit_state`, `flow_tracker`)
   - Wotan message bus active (port 18001)
   - Anamnesis 64-byte ring buffer functional

2. **Wire Format Compatibility**
   - IPv6 HbH header with circuit_state byte (offset 13)
   - Monad flags bitfield (C|Y|T|E|S|M|K1|K0) parseable
   - CRC-16 validation functional

3. **Infrastructure**
   - eBPF programs compiled with Aya (Rust)
   - BPF maps writable from userspace (Monad control service)
   - Wotan topics: `network.failover`, `network.recovery`, `network.circuit_state`
   - HAProxy/Nginx load balancers capable of L3/L4 redirect

4. **Monitoring**
   - Prometheus metrics export from BPF programs
   - Grafana dashboard with real-time health visualization
   - Anamnesis event ring captured + published to Wotan

---

## ARCHITECTURE

### High-Level Failover Flow

```
┌─────────────────────────────────────────────────────────────┐
│ Packet arrives at ingress (Shield XDP)                       │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
            ┌──────────────────────────┐
            │ Extract circuit_state    │
            │ from IPv6 HbH (byte 13)  │
            └────────────┬─────────────┘
                         │
                         ▼
        ┌────────────────────────────────┐
        │ Lookup endpoint in Sophia map  │
        │ health_scores[dst_endpoint]    │
        └────────────┬───────────────────┘
                     │
          ┌──────────┴──────────┐
          │                     │
          ▼                     ▼
    CLOSED?                OPEN?
    (Health>75%)          (Health<25%)
    │                     │
    │ YES                 │ YES
    │                     │
    ▼                     ▼
Forward       ┌─────────────────────┐
to primary    │ Query backup list   │
              │ from Sophia         │
              └──────────┬──────────┘
                         │
                         ▼
              ┌──────────────────────┐
              │ XDP redirect to      │
              │ healthiest backup    │
              └──────────┬───────────┘
                         │
                         ▼
              ┌──────────────────────┐
              │ Update circuit state │
              │ OPEN → HALF_OPEN     │
              │ in Sophia map        │
              └──────────┬───────────┘
                         │
                         ▼
              ┌──────────────────────┐
              │ Publish failover     │
              │ event to Wotan       │
              │ network.failover     │
              └──────────────────────┘
```

### Per-Endpoint Health Scoring

```
Health Score = weighted_average(
  latency_bucket      [40% weight],
  error_rate          [30% weight],
  packet_loss         [20% weight],
  circuit_state       [10% weight]
)

Range: 0-100
  >= 75  : CLOSED   (healthy, take traffic)
  50-75  : HALF_OPEN (recovering, gradual traffic)
  < 50   : OPEN     (failed, redirect all traffic)
```

### Circuit Breaker State Machine

```
┌─────────┐
│ CLOSED  │  (default, healthy)
│ (75+%)  │
└────┬────┘
     │ Health < 50% & failure_count > 3
     ▼
┌─────────┐
│ OPEN    │  (failed endpoint, redirect all)
│ (<50%)  │
└────┬────┘
     │ Successful response detected
     ▼
┌──────────────┐
│ HALF_OPEN    │  (recovery probe, 10% traffic)
│ (recovery%)  │
└────┬─────────┘
     │ Health > 75% for 5 consecutive pkts
     ▼
┌─────────┐
│ CLOSED  │  (recovered)
└─────────┘
```

---

## IMPLEMENTATION PHASES (5 Phases, 3-4 Sessions)

### PHASE 1: HEALTH SCORE CALCULATION (Steps 1-25)

**Goal**: Implement per-endpoint health scoring in BPF maps
**Time**: 4-5 hours
**Agent**: eBPF Specialist

- [ ] **Step 1-3**: Design health score BPF map structure
  - `endpoint_health: {endpoint_ip → score(0-100), last_update, failure_count}`
  - `latency_buckets: {endpoint_ip → [<1ms, 1-5ms, 5-10ms, 10-50ms, >50ms]}`
  - `packet_counters: {endpoint_ip → [total, success, error, timeout]}`

- [ ] **Step 4-8**: Implement `ebpf/health_scorer.rs`
  - Parse packet latency from IPv6 HbH circuit_state byte
  - Increment latency buckets per endpoint
  - Track error flags (E bit in Monad header)
  - Calculate weighted average health score

- [ ] **Step 9-12**: Add BPF helpers for score persistence
  - `bpf_health_update(endpoint, score)` — atomic write to Sophia
  - `bpf_health_read(endpoint)` — lookup current health
  - `bpf_failure_increment(endpoint)` — track consecutive failures

- [ ] **Step 13-18**: Integrate with Shield XDP pipeline
  - Call health_scorer on ingress for every packet
  - Update Sophia maps atomically (BPF_MAP_TYPE_HASH)
  - Export metrics (counter: health_updates_total)

- [ ] **Step 19-22**: Unit test health calculation
  - Test weighted average logic
  - Test latency bucket increments
  - Test error rate computation
  - Verify atomic updates don't race

- [ ] **Step 23-25** [C]: COMMIT — health scoring foundation
  ```bash
  git add ebpf/health_scorer.rs pkg/health/scorer.go
  git commit -m "feat(app08): implement per-endpoint health scoring in BPF"
  ```

---

### PHASE 2: CIRCUIT BREAKER STATE MACHINE (Steps 26-50)

**Goal**: Implement CLOSED → OPEN → HALF_OPEN state transitions in BPF
**Time**: 4-5 hours
**Agent**: eBPF Specialist

- [ ] **Step 26-30**: Design circuit state map
  - `circuit_state: {endpoint_ip → state(CLOSED|OPEN|HALF_OPEN), timestamp, success_count, fail_count}`
  - Thresholds: CLOSED→OPEN when score<50% + failures>3; OPEN→HALF_OPEN when success_count>5

- [ ] **Step 31-38**: Implement `ebpf/circuit_breaker.rs`
  - State transition logic (CLOSED → OPEN → HALF_OPEN → CLOSED)
  - Automatic recovery detection (successful packet after OPEN)
  - Timestamp-based backoff (don't query OPEN endpoint too fast)

- [ ] **Step 39-43**: Integrate with Shield XDP
  - Check circuit state before forwarding packet
  - If OPEN, query backup endpoint list
  - If HALF_OPEN, probabilistic traffic split (90% backup, 10% primary)

- [ ] **Step 44-47**: Unit test state machine
  - Test all valid transitions
  - Test invalid transitions (blocked)
  - Test timestamps and backoff logic

- [ ] **Step 48-50** [C]: COMMIT — circuit breaker foundation
  ```bash
  git add ebpf/circuit_breaker.rs
  git commit -m "feat(app08): implement circuit breaker state machine in XDP"
  ```

---

### PHASE 3: FAILOVER + BACKUP SELECTION (Steps 51-85)

**Goal**: Automatic traffic redirection to healthy backup endpoints
**Time**: 5-6 hours
**Agent**: eBPF Specialist + Go Backend Developer (parallel)

- [ ] **Step 51-58**: Design backup endpoint list storage
  - `endpoint_backups: {primary_ip → [backup_ip1, backup_ip2, backup_ip3]}`
  - `backup_health: {backup_ip → health_score}`
  - Sorted by health (healthiest first)

- [ ] **Step 59-68**: Implement `ebpf/failover.rs`
  - Lookup backup list when primary is OPEN
  - Select healthiest backup via BPF loop
  - Update packet dest IP via XDP redirect
  - Recompute IPv6 checksum

- [ ] **Step 69-75**: Implement userspace backup manager (Go)
  - Service: `services/backup-manager/main.go`
  - Watch Sophia endpoint_backups map
  - Write backup lists via Monad control API
  - Expose `/api/v1/endpoints` for dashboard

- [ ] **Step 76-80**: Unit test failover logic
  - Test backup selection (healthiest first)
  - Test packet redirect (IP rewrite)
  - Test checksum recalculation
  - Test handling of empty backup list (drop packet)

- [ ] **Step 81-85** [C]: COMMIT — failover + backup selection
  ```bash
  git add ebpf/failover.rs services/backup-manager/
  git commit -m "feat(app08): implement automatic failover to healthy backups"
  ```

---

### PHASE 4: WOTAN EVENTS + OBSERVABILITY (Steps 86-120)

**Goal**: Publish failover events to Wotan, expose metrics + dashboard integration
**Time**: 3-4 hours
**Agent**: Go Backend Developer

- [ ] **Step 86-92**: Design Wotan topic schema
  - `network.failover`: `{timestamp, endpoint_ip, backup_ip, reason, circuit_state}`
  - `network.recovery`: `{timestamp, endpoint_ip, health_score, restored_at}`
  - `network.circuit_state`: `{endpoint_ip, state, last_transition_time}`

- [ ] **Step 93-102**: Implement userspace Wotan publisher
  - Watch BPF maps for state changes
  - Publish events on transition (CLOSED→OPEN, OPEN→HALF_OPEN, etc.)
  - Include trace_id correlation
  - Buffer + batch if high churn

- [ ] **Step 103-110**: Export Prometheus metrics
  - `monad_failover_events_total{endpoint, backup, reason}`
  - `monad_circuit_state{endpoint, state}`
  - `monad_health_score{endpoint}`
  - `monad_recovery_duration_seconds{endpoint}`

- [ ] **Step 111-115**: Dashboard integration
  - Display real-time endpoint health grid
  - Highlight OPEN circuits in red
  - Show backup traffic percentage
  - Timeline of failover events (Anamnesis)

- [ ] **Step 116-120** [C]: COMMIT — observability
  ```bash
  git add services/monad/wotan_events.go cmd/dashboard-backend/failover_handler.go
  git commit -m "feat(app08): add Wotan events + Prometheus metrics + dashboard"
  ```

---

### PHASE 5: INTEGRATION + E2E TESTING (Steps 121-150)

**Goal**: End-to-end failover test, cascading failure detection, production hardening
**Time**: 3-4 hours
**Agent**: QA + Specialist

- [ ] **Step 121-130**: Implement cascading failure detection
  - `flow_tracker` BPF map: `{flow_hash → affected_endpoint}`
  - When endpoint OPEN, check how many flows affected
  - Publish flow analytics to Wotan (blast radius estimation)

- [ ] **Step 131-140**: E2E failover test
  - Setup 3-endpoint cluster (primary + 2 backups)
  - Generate traffic via test harness
  - Simulate primary failure (iptables drop, latency spike)
  - Verify: failover <500μs, zero packet loss, automatic recovery

- [ ] **Step 141-145**: Stress test + chaos engineering
  - Random endpoint failures (inject via eBPF blacklist)
  - Verify no packet loss, no loops
  - Verify backup sorting maintains consistency
  - Test recovery under sustained load

- [ ] **Step 146-150** [C]: COMMIT — E2E validated
  ```bash
  git add tests/e2e/failover_test.go scripts/failover_chaos.sh
  git commit -m "feat(app08): add E2E tests + chaos validation"
  ```

---

## NEW BPF PROGRAMS (Sophia Maps Required)

### 1. `ebpf/health_scorer.rs`
- **Input**: Every ingress packet (XDP)
- **Logic**: Extract latency from circuit_state, update health buckets, compute score
- **Maps**: `endpoint_health`, `latency_buckets`, `packet_counters`
- **Output**: Updated Sophia health scores

### 2. `ebpf/circuit_breaker.rs`
- **Input**: Every ingress packet, current health score
- **Logic**: Check endpoint state, transition on thresholds
- **Maps**: `circuit_state`
- **Output**: CLOSED|OPEN|HALF_OPEN decision

### 3. `ebpf/failover.rs`
- **Input**: Packet with OPEN circuit endpoint
- **Logic**: Select healthiest backup, redirect XDP
- **Maps**: `endpoint_backups`, `backup_health`
- **Output**: Redirected packet or drop

### 4. `ebpf/flow_tracker.rs`
- **Input**: Every packet (connection tracking)
- **Logic**: Hash flow 5-tuple, tag with endpoint
- **Maps**: `active_flows`, `flow_to_endpoint`
- **Output**: Flow analytics (blast radius) to Wotan

---

## NEW SOPHIA DICTIONARIES (Config Maps)

| Map Name | Type | Key | Value | Purpose |
|----------|------|-----|-------|---------|
| `endpoint_health` | BPF_MAP_TYPE_HASH | endpoint_ip (u32) | health_score(u8), timestamp, fail_cnt | Real-time health |
| `circuit_state` | BPF_MAP_TYPE_HASH | endpoint_ip | state(u8), last_txn, success_cnt | State machine |
| `endpoint_backups` | BPF_MAP_TYPE_HASH | primary_ip | [backup_ip1..3] | Backup roster |
| `backup_health` | BPF_MAP_TYPE_HASH | backup_ip | health_score(u8) | Backup scores |
| `latency_buckets` | BPF_MAP_TYPE_HASH | endpoint_ip | [cnt<1ms, cnt1-5, cnt5-10, cnt10-50, cnt>50] | Latency histogram |
| `packet_counters` | BPF_MAP_TYPE_HASH | endpoint_ip | total, success, error, timeout | Traffic stats |
| `active_flows` | BPF_MAP_TYPE_HASH | flow_hash | endpoint_ip, pkt_count, timestamp | Flow tracking |
| `circuit_thresholds` | BPF_MAP_TYPE_HASH | "global" | closed_threshold(75), open_threshold(50), fail_cnt_limit(3) | Config tuning |

---

## WOTAN TOPICS (Message Schema)

### 1. `network.failover`
```json
{
  "timestamp": 1741100800000,
  "trace_id": "f47ac10b58ccXXXX",
  "endpoint_ip": "10.10.10.50",
  "backup_ip": "10.10.10.51",
  "reason": "HEALTH_SCORE_BELOW_THRESHOLD",
  "health_score": 42,
  "circuit_state": "OPEN",
  "affected_flows": 127,
  "blast_radius_pct": 8.5
}
```

### 2. `network.recovery`
```json
{
  "timestamp": 1741100810000,
  "trace_id": "YYYY",
  "endpoint_ip": "10.10.10.50",
  "health_score": 78,
  "circuit_state": "HALF_OPEN",
  "time_to_recovery_ms": 10000,
  "traffic_restored_pct": 10
}
```

### 3. `network.circuit_state`
```json
{
  "endpoint_ip": "10.10.10.50",
  "state": "OPEN",
  "last_transition": 1741100800000,
  "failed_at": 1741100785000,
  "retry_after_ms": 5000
}
```

---

## DASHBOARD INTEGRATION

### New Widgets

1. **Endpoint Health Grid**
   - Real-time health scores (color-coded: green/yellow/red)
   - Circuit state badges (CLOSED|OPEN|HALF_OPEN)
   - Active flow count per endpoint
   - Failover button (manual trigger for testing)

2. **Failover Timeline**
   - Event stream from Wotan `network.failover` + `network.recovery`
   - Latency histogram per endpoint
   - Blast radius visualization (affected flows)

3. **Backup Roster Manager**
   - Edit backup lists (Sophia map writer)
   - Health score trends (Anamnesis)
   - Failure prediction (ML-ready, deferred)

### API Endpoints

- `GET /api/v1/health` — All endpoints + scores
- `GET /api/v1/health/:endpoint_ip` — Single endpoint detail
- `GET /api/v1/failovers?limit=100` — Recent failover events
- `POST /api/v1/failover/manual` — Trigger failover (testing)
- `PUT /api/v1/endpoints/:ip/backups` — Update backup list

---

## TESTING STRATEGY

### Unit Tests (Sophia Maps, Health Scoring)
- Health score calculation (weighted average)
- Latency bucket updates
- Circuit state transitions
- Backup selection logic

### Integration Tests (BPF + Userspace)
- Health scorer pushes to Sophia
- Circuit breaker reads health, changes state
- Failover selects correct backup
- Wotan events published on state change

### E2E Tests (Full Stack)
- 3-endpoint cluster with load generator
- Simulate failure (iptables drop)
- Verify failover latency <500μs
- Verify zero packet loss
- Verify automatic recovery (HALF_OPEN → CLOSED)

### Chaos Engineering
- Random endpoint failures
- Simultaneous multi-endpoint failures
- Network latency injection (tc qdisc)
- Packet loss injection (tc netem)
- Backup list churn (dynamic updates)

### Performance Validation
- Failover detection latency (target: <500μs)
- Backup selection latency (target: <10μs)
- Sophia map lookup latency (target: <5μs)
- Event publishing latency (target: <100ms)

---

## DEPENDENCIES

### Internal
- **Monad Core**: XDP programs, Sophia maps, circuit_state byte in IPv6 HbH
- **Wotan**: Message bus for event publishing, topic infrastructure
- **Anamnesis**: Event ring buffer for failover timeline
- **Shield**: XDP redirect capability, checksum helpers
- **Monad Control Service**: API for writing backup lists to Sophia

### External
- **Linux Kernel**: 5.8+ (BPF_PROG_TYPE_XDP, BPF map atomic ops)
- **LLVM/Clang**: For eBPF compilation
- **iptables**: For network simulation (testing)
- **tc (traffic control)**: For latency/loss injection (testing)

---

## RISK REGISTER

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|-----------|
| **BPF map memory exhaustion** | Failover logic fails | Low | Pre-allocate maps, LRU eviction policy |
| **Backup list corruption** | Redirect to invalid endpoint | Low | Atomic writes, CRC validation in Sophia |
| **Cascading backups failure** | No endpoint available | Low | Require 2+ backups per endpoint, alert on <2 |
| **Latency spikes during failover** | Packet drop | Low | Test under 10k pps load, validate queue depth |
| **Sophia map desync** | Stale health scores | Medium | Periodic full recompute, timestamps + TTL |
| **Wotan queue overflow** | Events dropped | Low | Batch + buffer in userspace, monotonic counter |
| **XDP redirect loop** | Infinite loop | Low | Detect via packet counter, TTL decrement in BPF |
| **Circuit state flip-flop** | Thrashing between states | Medium | Debounce threshold, hysteresis (75→50 vs 50→75) |

---

## DEFINITION OF DONE

### Code
- [x] 3 new eBPF programs compiled + linked
- [x] 4 new Sophia dictionaries defined + initialized
- [x] Userspace backup manager (Go service) with `/api/v1/endpoints`
- [x] Wotan event publisher in Monad control loop
- [x] Prometheus metrics exporter (health, circuit state, failovers)
- [x] Dashboard widgets (health grid, failover timeline, backup roster)

### Testing
- [x] Unit tests for health scoring (8+ test cases)
- [x] Unit tests for circuit state machine (12+ test cases)
- [x] Integration tests (BPF + Sophia + Wotan)
- [x] E2E failover test with <500μs latency validation
- [x] Chaos test (random failures, multi-endpoint)
- [x] Load test (10k pps sustained failover)

### Documentation
- [x] Architecture ADR (Sub-ms Failover Design)
- [x] BPF program source comments + function docs
- [x] Dashboard user guide (health grid, manual trigger)
- [x] Operational runbook (thresholds, tuning, recovery procedures)
- [x] CLAUDE.md appendix (App #8 integration)

### Security + Hardening
- [x] BPF verifier compliance (no memory errors, no integer overflow)
- [x] Rate limiting on Wotan event publish
- [x] Input validation (backup IP list, health scores)
- [x] Audit logging (manual failover triggers)

### Production Readiness
- [x] Sub-50ms failover event end-to-end (Anamnesis → dashboard)
- [x] Zero packet loss during failover (validated via pcap)
- [x] Automatic recovery detection (HALF_OPEN logic)
- [x] Graceful degradation (no backups = drop packet, alert)
- [x] Rollback plan (disable via Sophia config flag)

---

## SUCCESS CRITERIA

1. **Latency**: Failover detection <500μs, redirection <1ms total
2. **Correctness**: Zero packet loss, no redirect loops, atomic state transitions
3. **Observability**: All failovers logged to Wotan, visible in dashboard real-time
4. **Reliability**: Automatic recovery detection, cascading failure isolation
5. **Performance**: Sub-10μs backup selection, <100ms Wotan event propagation
6. **Scale**: Support 1000+ endpoints, 100+ backups per endpoint

---

**Estimated Effort**: 14-21 hours | **Target Ship Date**: 2026-03-18 | **Status**: Battle Plan Ready

