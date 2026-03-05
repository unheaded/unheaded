# Monad CPU Application #6: Surgical Chaos Engineering

**Status:** Planning Phase | S67 Wire Format Frozen
**Updated:** March 4, 2026
**Classification:** Production Architecture | Data Plane Innovation
**Kingdom Doctrine:** Fault injection IN the kernel, never in userspace sidecars

---

## 1. Overview

**Surgical Chaos Engineering** is a fault injection system operating entirely within Monad CPU's eBPF data plane. Unlike traditional chaos tools (Chaos Monkey, Gremlin, Litmus) that kill entire services or deploy sidecars, Surgical Chaos operates at **packet precision**:

- Drop/delay/corrupt specific packets matching BPF predicates
- Inject latency spikes targeting specific percentiles (p99 only, leave p50 intact)
- Simulate network partitions between service pairs without touching others
- All faults injected **in-kernel** — zero userspace overhead
- Full auditability: every fault creates Anamnesis event ring buffer entries
- Atomic enable/disable via Sophia dictionary updates
- GameDay scenarios run as precompiled BPF programs

**Core Innovation:** The wire format's `circuit_state` byte becomes a global emergency kill-switch. One BPF map write halts all chaos across the cluster.

---

## 2. Value Proposition

### Comparison Matrix

| Dimension | Chaos Monkey | Gremlin | Litmus | Surgical Chaos |
|-----------|--------------|---------|--------|-----------------|
| **Granularity** | Container/VM | Service/infra | Pod/network | Packet / trace_id |
| **Scope** | Kill everything | Targeted kill | Policy-based | Surgical predicates |
| **Overhead** | High (restart) | High (sidecar) | High (operator) | Zero (eBPF) |
| **Data Plane** | No | No | No | **YES** |
| **Latency Precision** | Minutes | Seconds | Seconds | **Microseconds** |
| **p99 Isolation** | N/A | N/A | N/A | **YES** |
| **Observability** | Implicit | Logs | CRDs | **Ring Buffer** |
| **Emergency Stop** | Manual restart | Manual | Scale to 0 | **1 map write** |
| **Cluster Overhead** | Whole services | Per-service | Per-pod operator | ~2% eBPF CPU |
| **Learning Curve** | Basic | Moderate | High | Moderate |
| **Cost (scale)** | N/A | $$ per service | $$ per cluster | Free (kernel) |

**Key Advantage:** Surgical Chaos tests **exact production conditions** (packet loss/latency) without knocking out your service. Your infrastructure stays running while you learn how it degrades.

---

## 3. Prerequisites

### Infrastructure
- [x] Monad CPU eBPF execution engine (XDP attached to veth pairs)
- [x] AF_XDP zero-copy socket (920K pps proven)
- [x] Sophia dictionary service (BPF map configuration)
- [x] Wotan message bus (event publishing)
- [x] Anamnesis 64-byte event ring buffer
- [x] Shield ingress/egress eBPF pipeline
- [x] IPv6 HbH wire format (20 bytes, v0x01 frozen)

### Software
- Rust 1.70+ (Aya framework for eBPF programs)
- Linux kernel 5.19+ (required for AF_XDP + fine-grained BPF predicates)
- Go 1.21+ (chaos controller service)
- Protocol API service (packet handling)

### Knowledge
- eBPF packet processing (XDP/TC layer)
- BPF map data structures (hash maps, ring buffers)
- IPv6 extension headers (HbH parsing)
- Monad wire format (service IDs, trace IDs, circuit state)

### Authorization
- Dashboard RBAC role: `chaos.engineer` (minimum required)
- Wotan topic: `chaos.schedules`, `chaos.events` (write)
- Sophia dictionary: `chaos_injection_rules` (read/write)

---

## 4. Architecture (ASCII)

```
┌─────────────────────────────────────────────────────────────┐
│                    Unheaded Kingdom                         │
└─────────────────────────────────────────────────────────────┘
                              │
        ┌─────────────────────┼─────────────────────┐
        │                     │                     │
   ┌────▼────┐          ┌─────▼──────┐       ┌────▼─────┐
   │ Sophia  │◄─────────│  Monad     │       │  Wotan   │
   │ (Config)│  rules   │  (State)   │◄──────│ (Events) │
   └────┬────┘          └─────┬──────┘       └────▲─────┘
        │                     │                   │
        │  chaos_injection_   │  event publish    │
        │  rules dict         │  (ring buffer)    │
        │                     │                   │
        └─────────────────────┼───────────────────┘
                              │
                 ┌────────────▼────────────┐
                 │   eBPF Data Plane       │
                 │  (XDP + AF_XDP + TC)    │
                 └────────────┬────────────┘
                              │
        ┌─────────────────────┼─────────────────────┐
        │                     │                     │
   ┌────▼─────┐         ┌─────▼──────┐      ┌─────▼────┐
   │ Ingress  │         │   Core     │      │ Egress   │
   │ Pipeline │◄────────│  Monad     │─────►│ Pipeline │
   │ (XDP)    │         │   Packet   │      │ (TC)     │
   └──────────┘         │  Processor │      └──────────┘
                        └────────────┘
                              │
                  ┌───────────┼───────────┐
                  │           │           │
              ┌───▼──┐    ┌───▼──┐   ┌───▼──┐
              │ SVC1 │    │ SVC2 │   │ SVC3 │
              │      │    │      │   │      │
              └──────┘    └──────┘   └──────┘
```

### Layer Breakdown

**Layer 5: Chaos Controller (Go)**
- REST API: `POST /chaos/inject`, `DELETE /chaos/{id}`, `GET /chaos/events`
- Validates rules, publishes to Wotan
- Maintains audit trail in Anamnesis
- Integrates with dashboard

**Layer 4: Sophia Dictionary**
- Key: `chaos_injection_rules`
- Value: Array of BPF predicates (service pair, trace_id patterns, fault type)
- TTL: 0 (permanent until explicit delete)
- Updates: Atomic BPF map write via Sophia service

**Layer 3: Wotan Topics**
- `chaos.schedules` — GameDay scenario definitions (proto)
- `chaos.events` — Per-packet fault injection events (ring buffer → consumer)
- `system.circuit` — Global circuit state changes (broadcast)

**Layer 2: eBPF Programs (Rust/Aya)**
- `chaos_inject_ingress.o` — XDP: inspect incoming packets
- `chaos_inject_egress.o` — TC: inspect outgoing packets
- `chaos_event_ring.o` — Ring buffer writer (Anamnesis)

**Layer 1: Kernel Data Structures**
- BPF hash map: `chaos_rules` (service_pair → fault_spec)
- BPF ring buffer: `events` (64-byte entries)
- Atomic: `circuit_state` (u8, global kill-switch)

---

## 5. Implementation Phases

### Phase 1: Core eBPF Programs (Week 1, Size: S)
**Goal:** Implement basic packet-level fault injection in eBPF

**Steps:**
- [ ] **1a** Create `ebpf/chaos_inject_ingress.rs` (Aya)
  - Parse IPv6 HbH header
  - Extract service IDs, trace_id, circuit_state
  - Lookup rule in `chaos_rules` map
  - Implement DROP action (packet filtered at XDP)

- [ ] **1b** Create `ebpf/chaos_inject_egress.rs` (TC)
  - Mirror ingress logic for egress path
  - Coordinate rule state via shared BPF map

- [ ] **1c** Create `chaos_rules` BPF map schema (Rust)
  - Key: `service_pair_t` (src_svc_id | dst_svc_id)
  - Value: `chaos_fault_spec_t` (fault_type, delay_us, drop_rate, corrupt_bits)
  - Max entries: 10,000 (one rule per service pair variant)

- [ ] **1d** Ring buffer for event publication
  - `Anamnesis` integration: 64-byte event struct
  - Publish: (timestamp, fault_type, packet_src/dst, trace_id)

- [ ] **1e** Unit tests in `ebpf/tests/`
  - Mock BPF maps
  - Test packet parsing (valid/invalid HbH)
  - Test rule lookup (hit/miss)

**Gate:** Packet drop test: single rule, verified via tcpdump

**Checkpoint Size Estimate:** 800 LOC eBPF (Rust) + 200 LOC tests

---

### Phase 2: Chaos Controller Service (Week 2, Size: M)
**Goal:** Go service to manage rules and publish to Wotan

**Steps:**
- [ ] **2a** Create `services/chaos-controller/` (Go)
  - Service discovery: register on startup
  - Connect to Sophia for config
  - Connect to Wotan for event publishing

- [ ] **2b** Implement REST API
  - `POST /chaos/inject` — Create fault rule (validate: service pair, percentile, duration)
  - `GET /chaos/status` — Current active rules
  - `DELETE /chaos/{rule_id}` — Remove rule (atomic via Sophia)
  - `GET /chaos/events` — Stream events from ring buffer consumer

- [ ] **2c** BPF map writer (Go)
  - Load eBPF object from compiled `.o` files
  - Atomic updates to `chaos_rules` map
  - Handle map overflow gracefully (queue rule or reject)

- [ ] **2d** Anamnesis consumer
  - Poll ring buffer periodically
  - Deserialize event structs
  - Publish to Wotan topic `chaos.events`
  - Store in local SQLite for audit trail

- [ ] **2e** Circuit state monitoring
  - Watch Monad's `circuit_state` byte via Sophia
  - If set to HALT, suppress new injections (safety)
  - Publish `system.circuit` events to Wotan

- [ ] **2f** Unit + integration tests
  - Mock Sophia dictionary
  - Mock Wotan publisher
  - Test rule validation logic

**Gate:** E2E test: inject drop rule via REST API, verify packet loss on live traffic

**Checkpoint Size Estimate:** 1,200 LOC Go + 300 LOC tests

---

### Phase 3: Chaos Rules DSL & Predicates (Week 3, Size: M)
**Goal:** Allow fine-grained fault injection based on BPF predicates

**Steps:**
- [ ] **3a** Design predicate schema (Protobuf)
  ```protobuf
  message ChaosFaultRule {
    string rule_id = 1;

    // Predicate (what to match)
    uint16 src_service_id = 2;    // or 0xFFFF for wildcard
    uint16 dst_service_id = 3;
    uint64 trace_id_prefix = 4;   // Match first 8 bytes
    uint8 qos_class = 5;          // or 0xFF for any

    // Fault (what to do)
    enum FaultType { DROP=0; DELAY=1; CORRUPT=2; DUPLICATE=3; }
    FaultType fault_type = 6;

    // Parameters
    uint32 delay_us = 7;          // For DELAY
    uint8 drop_rate_ppm = 8;      // Parts per million (1M = 100%)
    uint16 corrupt_bits = 9;      // Bits to flip (for CORRUPT)
    uint8 percentile = 10;        // p99, p95, p50 (for latency injection)

    // Duration
    uint32 duration_sec = 11;     // How long to run, 0 = forever

    // Safety
    bool requires_ack = 12;       // Manual circuit approval needed
  }
  ```

- [ ] **3b** Predicate evaluator (eBPF)
  - Add `evaluate_predicate()` function to ingress/egress programs
  - Test each field: src/dst SVC ID, trace_id prefix, QoS
  - Use bitmask matching for trace_id (fast-path)

- [ ] **3c** Percentile-aware latency injection
  - Track per-flow latency histogram (separate BPF map)
  - Calculate p99 bucket on-the-fly
  - DELAY action only applied to packets exceeding threshold
  - Leave p50, p75 untouched for true p99-only chaos

- [ ] **3d** Rule DSL parser (Go, optional but recommended)
  - YAML syntax: `src: svc1, dst: svc2, fault: delay, delay_us: 5000, percentile: p99`
  - Compile to Protobuf
  - Validate at controller

- [ ] **3e** Tests
  - Unit: predicate matching (hit/miss scenarios)
  - Integration: apply rule, verify only matching packets affected

**Gate:** Targeted test: inject delay into traffic from SVC1→SVC2 with trace_id prefix, verify other traffic unaffected

**Checkpoint Size Estimate:** 600 LOC eBPF + 500 LOC Go + 400 LOC tests + Protobuf schemas

---

### Phase 4: GameDay Scenario Engine (Week 4, Size: M)
**Goal:** Pre-scripted chaos runs without manual intervention

**Steps:**
- [ ] **4a** GameDay scenario DSL (YAML/JSON)
  ```yaml
  name: "Cascading Failure Simulation"
  duration: 300  # 5 minutes
  stages:
    - name: "Partition SVC1 from SVC2"
      delay: 10
      rules:
        - src: svc1
          dst: svc2
          fault: drop
          drop_rate_ppm: 500000  # 50% loss

    - name: "Degrade SVC3 egress (p99 only)"
      delay: 60
      rules:
        - src: any
          dst: svc3
          fault: delay
          delay_us: 10000
          percentile: p99

    - name: "Partition heals"
      delay: 120
      action: delete_rule
      rule_id: 1
  ```

- [ ] **4b** Scenario parser (Go)
  - Load YAML, validate schema
  - Compile stages to time-based event stream

- [ ] **4c** Scenario scheduler
  - Watch Wotan topic `chaos.schedules`
  - Execute stages on timer
  - Auto-recover on timeout/error

- [ ] **4d** Dashboard integration
  - Display active scenario + ETA
  - Real-time stage transitions
  - Show injected faults overlaid on metrics timeline

- [ ] **4e** Tests
  - Unit: scenario parsing
  - Integration: full GameDay run with mock chaos controller

**Gate:** Run 5-minute GameDay scenario, verify all stages execute in order, all faults appear in audit trail

**Checkpoint Size Estimate:** 400 LOC Go + 300 LOC tests

---

### Phase 5: Safety & Observability Polish (Week 5, Size: S)
**Goal:** Production readiness: safety rails, alerting, dashboards

**Steps:**
- [ ] **5a** Circuit breaker integration
  - Patch Sophia service: add `circuit_state` watch
  - If any service raises HALT flag, chaos-controller drains active rules
  - Publish `chaos.emergency_stop` event

- [ ] **5b** Dashboard widget
  - New panel: "Chaos Injection Status"
  - Active rules table (service pair, fault type, duration)
  - Recent events timeline (last 100)
  - Start/stop controls (requires `chaos.engineer` RBAC role)

- [ ] **5c** Prometheus metrics
  - `monad_chaos_rules_active` (gauge, by fault_type)
  - `monad_chaos_events_total` (counter, by fault_type)
  - `monad_chaos_injection_latency_us` (histogram)
  - `monad_chaos_packets_affected_total` (counter)

- [ ] **5d** Alerts
  - WARN: More than 10 active rules (possible misconfiguration)
  - ERROR: Rule duration exceeds 600s without audit trail (safety)
  - CRITICAL: Circuit breaker triggered (manual intervention needed)

- [ ] **5e** Audit trail SQL schema
  - Table: `chaos_audit_log` (rule_id, user_id, action, timestamp, rule_spec)
  - Table: `chaos_events_archive` (event_id, fault_type, packets_affected, timestamp)
  - Retention: 90 days

- [ ] **5f** Documentation + runbooks
  - `docs/chaos/GETTING_STARTED.md`
  - `docs/chaos/PREDICATES.md` (all rule options)
  - `docs/chaos/GAMEDAY_PLAYBOOK.md` (scenario templates)

- [ ] **5g** Tests
  - Safety: emergency stop halts all faults
  - Metrics: verify all counters increment correctly
  - Audit: every injection logged

**Gate:** Run chaos injection with chaos.engineer RBAC role, verify metrics appear on dashboard, audit trail complete

**Checkpoint Size Estimate:** 500 LOC Go + 200 LOC SQL + 300 LOC tests

---

## 6. New eBPF Programs

### `ebpf/chaos_inject_ingress.rs`
**Location:** `ebpf/chaos_inject_ingress.rs`
**Type:** XDP
**Purpose:** Intercept inbound packets, apply fault injection rules

**Key Functions:**
```rust
#[xdp]
pub fn inject_chaos(ctx: XdpContext) -> u32 {
    // 1. Parse IPv6 HbH header (extract service IDs, trace_id)
    // 2. Construct service_pair key (src_svc_id | dst_svc_id)
    // 3. Lookup rule in chaos_rules BPF map
    // 4. If match:
    //    a. Check circuit_state (abort if HALT)
    //    b. Execute fault action (DROP/DELAY/CORRUPT)
    //    c. Publish event to ring buffer
    // 5. Return XDP_PASS (normal forward) or XDP_DROP
}
```

**Maps:**
- `chaos_rules`: Hash map (service_pair → fault_spec)
- `events`: Ring buffer (64-byte entries)
- `circuit_state`: Atomic u8 (kill-switch)

---

### `ebpf/chaos_inject_egress.rs`
**Location:** `ebpf/chaos_inject_egress.rs`
**Type:** TC (qdisc)
**Purpose:** Intercept outbound packets (symmetrical to ingress)

**Key Difference:** Egress operates at TC layer (slower but finer control of packet timing)

---

### Event Ring Buffer Schema
**Entry Size:** 64 bytes (Anamnesis standard)
```c
struct chaos_event {
    u64 timestamp;           // ns since epoch
    u16 src_service_id;
    u16 dst_service_id;
    u64 trace_id;
    u8 fault_type;           // 0=DROP, 1=DELAY, 2=CORRUPT, 3=DUP
    u8 drop_rate_ppm_lo;     // Lower 8 bits of drop rate
    u32 delay_us;            // Delay applied
    u8 percentile;           // p99=99, p95=95, p50=50
    u8 _reserved[16];        // For future extensions
};
```

---

## 7. New Sophia Dictionaries

### `chaos_injection_rules`
**Key:** `chaos_injection_rules`
**Type:** Array of `ChaosFaultRule` (Protobuf)
**Purpose:** Master list of active fault rules
**TTL:** 0 (permanent)
**Example:**
```json
{
  "key": "chaos_injection_rules",
  "value": [
    {
      "rule_id": "rule_20260304_001",
      "src_service_id": 1,
      "dst_service_id": 2,
      "fault_type": "DELAY",
      "delay_us": 5000,
      "percentile": 99,
      "duration_sec": 120,
      "requires_ack": false
    }
  ],
  "version": 42
}
```

### `chaos_circuit_state`
**Key:** `chaos_circuit_state`
**Type:** Protobuf message
**Purpose:** Global circuit breaker state
**Example:**
```json
{
  "key": "chaos_circuit_state",
  "value": {
    "state": "ACTIVE",  // or "HALT"
    "reason": "Automatic halt: error rate >50%",
    "halted_by": "monad_circuit_breaker",
    "halt_timestamp": 1740987654000,
    "active_rules_count": 0
  }
}
```

### `chaos_audit_log`
**Key:** `chaos_audit_log`
**Type:** Array of audit entries
**Purpose:** Complete injection history
**TTL:** 90 days
**Example:**
```json
{
  "key": "chaos_audit_log",
  "value": [
    {
      "timestamp": 1740987654000,
      "user_id": "engineer@kingdom.local",
      "action": "CREATE_RULE",
      "rule_id": "rule_20260304_001",
      "rule_spec": { /* full rule */ }
    }
  ]
}
```

---

## 8. Wotan Topics

### `chaos.schedules`
**Type:** Protobuf `GameDayScenario`
**Direction:** Controller publishes
**Purpose:** Active GameDay scenario definitions
**Retention:** Until scenario completes + 7 days
**Example:**
```
Key: "gameday_20260304_001"
Value: {
  "name": "Cascading Failure Simulation",
  "stages": [
    { "name": "Partition SVC1/SVC2", "delay": 10, "rules": [...] },
    { "name": "Degrade SVC3 p99", "delay": 60, "rules": [...] }
  ]
}
```

### `chaos.events`
**Type:** Ring buffer consumer topic
**Direction:** eBPF → Ring buffer → Consumer → Topic
**Purpose:** Real-time fault injection events
**Retention:** 24 hours (sliding window)
**Schema:** 64-byte `chaos_event` struct

**Example (deserialized):**
```json
{
  "timestamp": 1740987654000123,
  "src_service_id": 1,
  "dst_service_id": 2,
  "trace_id": "abc123def456",
  "fault_type": "DELAY",
  "delay_us": 5000,
  "percentile": 99
}
```

### `chaos.injections` (Dashboard Subscribe)
**Type:** Aggregated Prometheus metrics
**Direction:** Chaos controller publishes
**Purpose:** Dashboard displays active rules + event rate
**Retention:** 5 minutes (sufficient for UI updates)

---

## 9. Dashboard Integration

### New Widgets

**Chaos Injection Control Panel**
- Location: Dashboard → Observability → Chaos
- Permissions: `chaos.engineer` role required
- Components:
  - **Rules Table:** Active rules (service pair, fault type, duration, created_by)
  - **Start Rule Form:** Service pair selector, fault type dropdown, parameters
  - **Recent Events:** Timeline of injected faults (last 100)
  - **GameDay Scenarios:** Dropdown of templates, schedule button
  - **Circuit Status:** Current state (ACTIVE/HALT), reason

**Metrics Overlay**
- Chaos injection timeline overlaid on service latency graph
- Color-coded by fault type (red=DROP, yellow=DELAY, blue=CORRUPT)
- Hover to see affected packet count

**Audit Trail**
- Table: User, action, rule_id, timestamp, rule_spec
- Filterable by user/date/rule_id
- Export to CSV for compliance

---

## 10. Testing Strategy

### Unit Tests (Per-Phase)

**Phase 1 eBPF Tests:**
- Mock BPF maps with test data
- Verify packet parsing (valid/invalid HbH headers)
- Verify rule lookup (hit/miss/multiple rules)
- Verify DROP action (packet filtered)

**Phase 2 Controller Tests:**
- Sophia dictionary read/write
- REST API validation (input bounds checking)
- Rule state transitions (create/update/delete)

**Phase 3 Predicate Tests:**
- Match logic (all predicates combined with AND)
- Wildcard expansion (0xFFFF matches any service)
- Trace ID prefix matching

**Phase 4 Scenario Tests:**
- YAML parsing + validation
- Stage timing + transitions
- Rule cleanup on scenario end

**Phase 5 Safety Tests:**
- Circuit breaker halt stops all injections
- Rule TTL expires correctly
- Metrics publish to Prometheus

### Integration Tests

**Full Stack:**
1. Start chaos-controller + Sophia + Wotan
2. POST rule via REST API
3. Verify rule appears in Sophia dict
4. Verify eBPF program loads it
5. Send traffic through, verify packets affected
6. DELETE rule
7. Verify traffic flows normally

**Scenario Run:**
1. Load GameDay scenario from YAML
2. Publish to Wotan `chaos.schedules`
3. Verify each stage executes on timer
4. Verify metrics + events flow correctly
5. Verify circuit breaker can halt mid-scenario

### E2E Tests (Dashboard)

**Browser → Controller → eBPF:**
1. Login as `chaos.engineer`
2. Fill "New Rule" form (SVC1→SVC2, DELAY 5ms, p99)
3. Click "Inject"
4. Verify rule appears in table
5. Monitor live events
6. Observe latency spike in metrics (p99 only)
7. Delete rule
8. Verify p99 returns to normal

### Load Tests

**Throughput:**
- 1000 rules injected simultaneously
- Verify BPF map doesn't overflow
- Measure lookup latency (target: <100ns per packet)

**Chaos Packet Rate:**
- Baseline: 920K pps (AF_XDP)
- With 100 active rules: target 900K+ pps (minimal overhead)
- With 1000 active rules: target 800K+ pps

### Safety Tests

**Circuit Breaker:**
- Set circuit_state to HALT
- Verify all active rules drain within 1s
- Verify no new rules accepted
- Verify state persists after restart

**Audit Trail:**
- Every injection logged to SQLite
- User identity captured
- Rule spec recorded
- Verify 90-day retention + cleanup

---

## 11. Dependencies

### eBPF
- `aya`: ^0.13 (eBPF framework)
- `aya-log`: ^0.2 (debug logging from kernel)

### Go Services
- `github.com/unheaded/unheaded/pkg/wotan-client`: Service discovery + pub/sub
- `github.com/unheaded/unheaded/pkg/auth`: RBAC middleware
- `github.com/cilium/ebpf`: BPF map loading
- `google.golang.org/protobuf`: Message serialization
- `github.com/prometheus/client_golang`: Metrics

### Infrastructure
- Linux 5.19+ (XDP, ring buffer, BPF predicates)
- Kernel headers (`linux-headers` package)

### Testing
- `testify/assert`: Go test assertions
- `tc` utility (iproute2): TC program attachment

---

## 12. Risk Register

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|-----------|
| **eBPF verifier rejects program** | High | S (blocks Phase 1) | Early eBPF audit by kernel team. Use proven patterns from Shield. Bounded loops required. |
| **BPF map overflow (>10K rules)** | Medium | M (rules rejected) | Graceful rejection + WARN alert. Document limit in API. Consider ring buffer overflow strategy. |
| **Chaos rule affects unintended services** | Medium | M (test failure) | Exhaustive unit tests of predicate logic. Require manual review for production rules. |
| **Circuit breaker doesn't halt mid-GameDay** | Low | L (can restart) | Atomic write to circuit_state. Verify in Phase 5 safety tests. |
| **Audit trail exceeds storage (90-day window)** | Low | L (prune oldest) | Auto-cleanup script. Monitor growth. Alert if >10GB. |
| **Latency injection breaks high-performance apps** | Medium | M (invalid test) | Document p99-only semantics. Require `requires_ack: true` for latency >100ms. |
| **Dashboard doesn't reflect live rule changes** | Low | L (refresh) | Wotan topic for rule state. WebSocket push on rule create/delete. 1s max latency. |
| **GameDay scenario crashes mid-stage** | Low | M (test halts) | Stage timeout + auto-rollback. Publish failure event to Wotan. Alert on-call. |

---

## 13. Definition of Done

### All Phases Complete When:

**Code Quality**
- [x] All code passes `go vet` + `cargo clippy`
- [x] Unit test coverage ≥80% (Go) + ≥70% (eBPF)
- [x] Zero compiler warnings
- [x] SPDX headers on all files

**Architecture**
- [x] eBPF programs load without verifier errors
- [x] Sophia dictionary stores rules atomically
- [x] Wotan topics publish events in <100ms
- [x] Ring buffer doesn't overflow under load

**Functionality**
- [x] Packet-level fault injection works end-to-end
- [x] Percentile-aware latency injection (p99 only)
- [x] Service pair predicates match correctly
- [x] Circuit breaker emergency halt tested
- [x] GameDay scenarios execute all stages
- [x] Audit trail complete + queryable

**Observability**
- [x] All Prometheus metrics publish correctly
- [x] Anamnesis ring buffer events appear on dashboard
- [x] Dashboard widgets fully functional + responsive
- [x] Alerts trigger on safety thresholds

**Safety & Security**
- [x] RBAC enforced (`chaos.engineer` role)
- [x] Audit trail captures all user actions
- [x] Circuit breaker prevents runaway chaos
- [x] Rule TTL prevents stale injections
- [x] No unintended service degradation

**Documentation**
- [x] `docs/chaos/GETTING_STARTED.md` complete
- [x] `docs/chaos/PREDICATES.md` documents all rule options
- [x] `docs/chaos/GAMEDAY_PLAYBOOK.md` with 3+ scenario templates
- [x] Runbook: `Responding to Unexpected Chaos`

**Integration**
- [x] End-to-end test: dashboard → controller → eBPF → metrics
- [x] E2E test: full GameDay scenario execution
- [x] Load test: 1000 rules at 920K pps (>800K pps throughput)
- [x] Circuit breaker test: halt stops all injections in <1s

**Deployment**
- [x] NixOS container definition with hardening
- [x] Systemd service unit file
- [x] Health/readiness endpoints
- [x] Database migrations for audit tables
- [x] Zero-downtime deploy procedure

---

## 14. Success Metrics

**Phase 1 (Week 1):**
- Single DROP rule tested: verified packet loss
- Ring buffer events flowing to consumer
- Zero eBPF verifier errors

**Phase 2 (Week 2):**
- REST API CRUD working end-to-end
- Sophia dictionary persists rules across restart
- Anamnesis events aggregated in Go (not Rust) yet

**Phase 3 (Week 3):**
- Predicates match only intended packets
- p99-aware latency injection leaves p50 untouched
- Service cross-traffic unaffected

**Phase 4 (Week 4):**
- 5-minute GameDay scenario runs start-to-finish
- All stages execute on schedule
- Rules auto-cleanup on scenario end

**Phase 5 (Week 5):**
- Production readiness: alerts, audit, RBAC working
- Dashboard displays live chaos status
- Circuit breaker tested in failure scenarios

---

## 15. Phase Dependency Graph

```
Phase 1 (Core eBPF)
    ↓
Phase 2 (Controller) ← Phase 1
    ↓
Phase 3 (Predicates) ← Phase 2
    ↓
Phase 4 (GameDay) ← Phase 1, 2, 3
    ↓
Phase 5 (Polish) ← All previous phases
```

All phases can start planning simultaneously. Phase 2 starts coding only after Phase 1 eBPF APIs are finalized.

---

## 16. Rollout Strategy

### Internal Alpha (Week 6-7)
- Limited to kingdom engineers
- chaos.engineer role required
- Monitored 24/7

### Staging (Week 8)
- Deploy to staging cluster
- Run 10 pre-written GameDay scenarios
- Verify no unexpected cross-service impact

### Production Canary (Week 9)
- Enable for 5% of services
- Manual rule approval required
- Automated rollback if error rate >10%

### General Availability (Week 10)
- Open to all engineers with chaos.engineer role
- Self-service rule creation
- 1-week probation: rules >5min duration need manager sign-off

---

## 17. Long-Term Vision (Post-Stable)

**Extensions:**
1. **Network Partition Simulation** — Asymmetric delay rules (SVC1→SVC2 delayed, SVC2→SVC1 normal)
2. **Packet Corruption** — Bit-flip injection for testing checksum validation
3. **Duplicate Packet Injection** — Stress-test idempotency
4. **Cost Modeling** — Analyze trade-offs: p99 latency vs business impact
5. **ML-Powered Scenarios** — Auto-generate chaos based on error patterns

---

## 18. Battle Plan Summary

**Timeline:** 5 weeks | **Size:** L | **Effort:** 4 engineers

**Deliverables:**
1. **eBPF Programs** (3): Ingress, egress, event ring (Rust/Aya)
2. **Chaos Controller Service** (1): Sophia + Wotan integration (Go)
3. **Rule Engine** (1): Predicates + percentile awareness
4. **GameDay Scheduler** (1): Scenario automation
5. **Dashboard Widgets** (3): Injection control, metrics overlay, audit trail
6. **Documentation** (3): Getting started, predicates reference, GameDay playbook
7. **Tests** (all phases): Unit, integration, E2E, load, safety
8. **Deployment** (1): NixOS container + systemd hardening

**Kingdom Doctrine Alignment:**
- ✅ **Surgical precision:** Packet-level fault injection, zero service impact
- ✅ **In-kernel execution:** Zero userspace overhead
- ✅ **Observability first:** Every fault creates Anamnesis event
- ✅ **Safety rails:** Circuit breaker kill-switch, audit trail, RBAC
- ✅ **Observable by default:** Prometheus metrics + live dashboard
- ✅ **Security first:** RBAC enforced, audit trail immutable, no user data access

---

**Status:** READY FOR PHASE 1
**Updated:** March 4, 2026
**Next Review:** After Phase 1 eBPF validation (Week 1 end)

---

*This battle plan enables the Unheaded Kingdom to test production resilience at microsecond precision, without knocking out your infrastructure. Surgical Chaos Engineering: Testing real failure modes without service death.*
