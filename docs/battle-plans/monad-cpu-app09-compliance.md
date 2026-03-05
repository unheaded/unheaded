# MONAD CPU APP #9: COMPLIANCE ENFORCEMENT AT WIRE LEVEL

**Battle Plan — Production Deployment**

**Document Version:** 1.0
**Status:** Ready for Implementation
**Authority:** Unheaded Kingdom, Monad Protocol v0x01
**Last Updated:** March 4, 2026

---

## 1. OVERVIEW

**Mission:** Enforce service compliance rules in the data plane, not the control plane.

Instead of auditing security violations AFTER the fact, Monad CPU Application #9 embeds compliance checks directly into packet processing pipelines. Every packet is evaluated against:

- **Service-to-service communication policies** (which services are authorized to communicate)
- **Data classification tags** (sovereignty, encryption levels, PII barriers)
- **Geographic routing constraints** (data residency enforcement at XDP)
- **Immutable audit trails** (every decision logged in Anamnesis ring buffer)

**Core Principle:** Zero-trust micro-segmentation at packet level — enforced by eBPF, not ACLs.

**Impact:**
- SOC2/HIPAA/GDPR compliance evidence generated automatically
- Real-time policy violations detected and blocked
- Atomic policy updates with zero downtime (Sophia dict swap)
- 920K pps throughput (AF_XDP proven baseline)
- Sub-microsecond compliance checks (BPF map lookup + CRC validation)

---

## 2. VALUE PROPOSITION

### Why Compliance Enforcement at Wire Level?

Traditional compliance enforcement (OPA, Calico, cloud IAM) happens at application or network layers AFTER packets are already in flight. APP #9 shifts enforcement into the data plane:

| Dimension | OPA/Gatekeeper | Calico Network Policy | Cloud IAM | **Monad APP #9** |
|-----------|---|---|---|---|
| **Enforcement Point** | Control plane (slow) | Data plane (network rules only) | Control plane + kernel | Data plane (all L2-L7) |
| **Latency** | 100-500ms per decision | Sub-ms (netfilter) | 50-200ms (cloud API) | **<1µs (BPF map)** |
| **Policy Granularity** | Service + namespace | CIDR blocks + ports | User/group + resource | **Service ID + data class + geography** |
| **Audit Trail** | Post-hoc logs | Firewall counters | Cloud audit logs | **Immutable ring buffer (Anamnesis)** |
| **Data Classification** | No native support | IP reputation only | Attribute-based | **Flags (K1\|K0) + payload analysis** |
| **Geographic Enforcement** | Not possible | Not possible | Regional (slow) | **Per-packet XDP routing** |
| **Policy Hot-Swap** | Full restart | Config reload | Full restart | **Atomic Sophia dict swap (zero restart)** |
| **Evidence Generation** | Manual + aggregation | Raw logs | Cloud console | **Auto-generated compliance reports** |
| **Throughput Cost** | 30-40% overhead | <5% overhead | Varies | **<1% overhead (BPF native)** |

**Unique Advantages:**
1. **Zero-Trust at Wire Level:** Every packet authenticated by its service ID + crypto context
2. **Sovereign Data Enforcement:** PII packets literally cannot route outside designated segments
3. **Audit Completeness:** No packet is unaudited — Anamnesis captures 100% of policy decisions
4. **Instant Rollback:** Policy changes are atomic dicts in Sophia; rollback is swap + flush
5. **No Vendor Lock-in:** Pure eBPF + standard IPv6 HbH — runs on any Linux kernel 5.8+

---

## 3. PREREQUISITES

### Kernel & Infrastructure
- Linux kernel 5.8+ (BPF maps, XDP, AF_XDP)
- LLVM 12+ (Rust Aya compilation)
- Intel/AMD/ARM64 processor (eBPF verified)

### Unheaded Kingdom Components (Required)
- **Monad:** State management service (port 19004, gRPC)
- **Sophia:** BPF map dictionaries (port 19005, gRPC)
- **Anamnesis:** 64-byte event ring buffer (kernel-side)
- **Shield:** eBPF ingress/egress pipeline (XDP hooks)
- **Wotan:** Message bus for policy propagation (port 18001, gRPC)
- **Trace Collector:** eBPF → userspace bridge (ports 16670/16671)

### Cryptography
- **CRC-16-CCITT:** Fast checksum validation (Monad header integrity)
- **AES-GCM:** Service identity authentication (optional, for encrypted service tokens)
- **HMAC-SHA256:** Policy digest signing (Sophia dicts)

### Observability
- **Prometheus:** Metrics collection (policy match rates, blocked flows)
- **Grafana:** Dashboard for compliance visualization
- **Loki:** Query Anamnesis audit trail

### Testing & Validation
- **perf_event_open():** BPF program performance profiling
- **BPF_PROG_TEST_RUN:** Unit testing BPF bytecode
- **tcpdump + bpftrace:** Interactive debugging

---

## 4. ARCHITECTURE

### High-Level Data Flow

```
┌─────────────────────────────────────────────────────────────────┐
│  INGRESS (Packet Arrives at NIC)                                │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  1. XDP Hook (Shield)                                            │
│     ├─ Parse Monad header (20B in IPv6 HbH)                     │
│     ├─ Extract: version, SrcServiceID, DstServiceID             │
│     ├─ Extract: trace_id, QoS, circuit_state, K1|K0 flags      │
│     └─ Validate CRC-16 (header integrity)                       │
│                                                                  │
│  2. Policy Evaluation (BPF Maps)                                │
│     ├─ Sophia.ServicePolicies[SrcServiceID][DstServiceID]       │
│     │  └─ Returns: {allow/block, audit_level, routing_rules}    │
│     ├─ Sophia.ClassificationTags[K1|K0]                         │
│     │  └─ Returns: {data_class, sovereignty, requires_encryption}
│     ├─ Sophia.GeoZones[packet.route_flag]                       │
│     │  └─ Returns: {zone_id, allowed_exit_ifaces, pii_boundary} │
│     └─ Sophia.CircuitBreakers[DstServiceID]                     │
│        └─ Returns: {state, open_until_ts}                       │
│                                                                  │
│  3. Compliance Decision                                          │
│     ├─ IF (policy == ALLOW) {                                   │
│     │    IF (classification == PII && route_outside_zone) {     │
│     │       → BLOCK + audit "PII_EGRESS_BLOCKED"                │
│     │    } ELSE IF (circuit_broken) {                           │
│     │       → BLOCK + audit "CIRCUIT_OPEN"                      │
│     │    } ELSE {                                               │
│     │       → ALLOW, record audit with latency                  │
│     │    }                                                       │
│     │ } ELSE {                                                   │
│     │    → BLOCK + audit "POLICY_DENIED"                        │
│     │ }                                                          │
│     └─ Update metrics (Prometheus counters)                     │
│                                                                  │
│  4. Audit Trail (Anamnesis Ring)                                │
│     └─ Write: {ts, policy_decision, src, dst, data_class,       │
│                violation_type, trace_id} [64B total]            │
│                                                                  │
│  5. Forward or Drop                                             │
│     ├─ ALLOW → Ring buffer + continue to network stack          │
│     └─ BLOCK → Drop + ICMP unreach (optional)                   │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│  EGRESS (Packet Leaves Host)                                    │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  1. Validate Destination Zone (XDP egress hook)                 │
│     └─ IF (packet.classification == PII) {                      │
│        IF (dest_route NOT in allowed_zones) {                   │
│           → DROP + audit "PII_EGRESS_BLOCKED"                   │
│        }                                                         │
│     }                                                            │
│                                                                  │
│  2. Append Compliance Metadata (optional)                       │
│     └─ Add audit_token to Monad flags if S flag set             │
│                                                                  │
│  3. Forward                                                      │
│     └─ AF_XDP zero-copy send (920K pps)                         │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│  CONTROL PLANE (Sophia + Wotan)                                 │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  Policy Updates (Non-Blocking)                                  │
│  1. Admin updates policy YAML                                   │
│  2. Captain service parses + validates                          │
│  3. Captain publishes to Wotan "policies.updates" topic          │
│  4. Sophia subscribers receive update                           │
│  5. Sophia atomically swaps old dict → new dict                 │
│  6. Old dict freed after grace period (drain time)              │
│  7. Trace Collector publishes "policy.hot_swap" event           │
│                                                                  │
│  No kernel restart. No packet loss. Zero downtime.              │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### Monad Wire Format Extension (APP #9)

The 20-byte Monad header already supports classification tagging. APP #9 adds semantic meaning:

```
IPv6 Hop-by-Hop Extension Header (Monad)
┌─────────────────────────────────────────────┐
│ 20 Bytes Total                              │
├─────────────────────────────────────────────┤
│ [0]     version (1B)        = 0x01          │
│ [1-2]   SrcServiceID (2B)   = 0x0001-0xFFFF│
│ [3-4]   DstServiceID (2B)   = 0x0001-0xFFFF│
│ [5-12]  trace_id (8B)       = UUID          │
│ [13]    QoS (1B)            = priority      │
│ [14]    circuit_state (1B)  = [OPEN|HALF|CLOSED]
│ [15]    flags (1B)                          │
│         ├─ C: Encrypted (AES-GCM)           │
│         ├─ Y: Cached (Wotan memo)           │
│         ├─ T: Traced (include in Anamnesis) │
│         ├─ E: Egress validated               │
│         ├─ S: Signed (HMAC digest)          │
│         ├─ M: Mirrored (debugging)          │
│         ├─ K1: Classification bit 1 (K1|K0) │
│         └─ K0: Classification bit 0         │
│ [16-17] reserved (2B)                       │
│ [18-19] CRC-16-CCITT (2B)   = checksum     │
└─────────────────────────────────────────────┘

Classification Tags (K1|K0):
00 = PUBLIC          (no constraints)
01 = INTERNAL        (intra-datacenter only)
10 = SENSITIVE       (encryption required)
11 = SOVEREIGN_PII   (cannot leave geographic zone)
```

---

## 5. IMPLEMENTATION PHASES

### Phase 1: Policy Map Infrastructure (3 weeks)
**Deliverable:** Sophia dicts + BPF map definitions

**Steps:**
1. Define Sophia schema for compliance dicts (YAML → JSON)
2. Implement BPF map layouts in Shield (ingress program)
   - `service_policies`: 2B × 2B key → policy struct (128B)
   - `classification_rules`: 1B key (K1|K0) → rules struct (64B)
   - `geo_zones`: 1B key → zone struct (96B)
   - `circuit_breakers`: 2B key → breaker state (32B)
3. Generate Prometheus metric definitions
4. Wire Wotan topic subscriptions (policies.updates)

**Size Estimate:** 1,200 LOC (Go + Rust)

**Checklist:**
- [ ] BPF map definitions compile without errors
- [ ] Sophia schema passes JSON validation
- [ ] Test data loads into maps (100 policies minimum)
- [ ] Metrics exported to Prometheus

**Gate:** Code review + unit test coverage >85%

---

### Phase 2: XDP Ingress Pipeline (4 weeks)
**Deliverable:** Shield ingress program with compliance checks

**Steps:**
1. Implement Monad header parser (20B parse, CRC validation)
2. Build policy lookup engine (BPF map → decision)
   - Service-to-service allow/block rules
   - Classification tag validation
   - QoS prioritization
3. Implement audit trail writer
   - Anamnesis ring buffer push (64B records)
   - Timestamp + trace_id correlation
4. Handle edge cases (invalid headers, map misses, congestion)
5. Performance profiling (perf_event_open, target <1µs decision)

**Size Estimate:** 1,800 LOC (Rust/Aya)

**Checklist:**
- [ ] Ingress program loads without verifier warnings
- [ ] Packets with valid policies allowed through
- [ ] Packets with denied policies dropped (silent)
- [ ] Audit records appear in ring buffer
- [ ] CRC validation catches corrupted headers
- [ ] Performance <1µs/packet (measured via perf)

**Gate:** BPF_PROG_TEST_RUN passes all test vectors + load testing (10K pps sustained)

---

### Phase 3: XDP Egress + Zone Enforcement (3 weeks)
**Deliverable:** Shield egress program with geographic routing validation

**Steps:**
1. Parse destination route (next hop, outgoing interface)
2. Check geo_zones map: is this packet allowed to exit zone?
3. Special handling for K1|K0 = 11 (SOVEREIGN_PII)
   - PII packets can ONLY route to designated safe zones
   - Block any unrecognized route immediately
4. Update circuit breaker state (half-open → open if destination unreachable)
5. Append compliance metadata to Monad flags (if S flag set)

**Size Estimate:** 900 LOC (Rust/Aya)

**Checklist:**
- [ ] Egress program loaded on all outgoing interfaces
- [ ] PII packets blocked when routing outside zone
- [ ] Normal traffic passes through without latency increase
- [ ] Circuit breaker state transitions work correctly

**Gate:** Integration test with multi-zone network topology (3 zones minimum)

---

### Phase 4: Sophia Hot-Swap + Policy Updates (2 weeks)
**Deliverable:** Control plane for atomic dict updates (zero downtime)

**Steps:**
1. Extend Sophia service with policy dictionary management
   - Load YAML policies from disk
   - Validate policy syntax (service IDs valid, rules complete)
   - Publish to Wotan "policies.updates" topic
2. Implement BPF dict swap mechanism
   - Keep 2 dict versions (active + pending)
   - Atomic swap on update signal (use Sophia timestamp as trigger)
   - Grace period for in-flight packets (typically 100-500ms)
3. Wire Captain service to parse policy changes
4. Test rollback scenario (bad policy detected, swap back)

**Size Estimate:** 600 LOC (Go)

**Checklist:**
- [ ] Policy update completes without kernel restart
- [ ] In-flight packets not dropped during swap
- [ ] Old policy fully cleaned up after grace period
- [ ] Rollback works (bad policy → revert to previous)
- [ ] Wotan event "policy.hot_swap" published

**Gate:** Chaos test (deploy bad policy, trigger rollback, verify recovery <5s)

---

### Phase 5: Compliance Reporting + Dashboard (3 weeks)
**Deliverable:** Real-time compliance evidence dashboard + audit export

**Steps:**
1. Build Anamnesis reader (userspace bridge)
   - Ring buffer → Prometheus metrics
   - Ring buffer → Loki logs (structured JSON)
   - Aggregate by policy_type, violation_count, trace_id
2. Implement compliance report generator
   - SOC2: Policy violation rate, audit completeness %
   - HIPAA: PII egress attempts (should be 0)
   - GDPR: Data residency violations by geography
   - Generate CSV exports (daily, weekly, monthly)
3. Dashboard components
   - Real-time compliance gauge (% of packets audited)
   - Policy violation heatmap (src × dst service matrix)
   - Geographic zone map (show blocked egress attempts)
   - Audit trail search (filter by trace_id, timestamp, violation_type)
4. Alert rules
   - Policy violation spike (>5% denied packets)
   - PII egress attempt (immediate critical alert)
   - Circuit breaker open >60s (WARN level)

**Size Estimate:** 1,400 LOC (Go + JS)

**Checklist:**
- [ ] Anamnesis ring buffer data flows to Prometheus
- [ ] Compliance reports generate without latency impact
- [ ] Dashboard loads policy violation data <100ms
- [ ] Search by trace_id returns full audit path (5 services example)
- [ ] SOC2/HIPAA/GDPR reports exportable as PDF

**Gate:** Manual compliance auditor review + dashboard demo with 1M+ audit records

---

**Total Timeline:** 13 weeks (end-to-end, fully integrated)

**Parallel Tracks:**
- Phases 1 & 2 can run in parallel (different teams)
- Phase 3 waits for Phase 2 completion
- Phase 4 waits for Phase 3 (needs integrated data plane)
- Phase 5 can start after Phase 2 (reads Anamnesis)

---

## 6. NEW BPF PROGRAMS

### Program 1: `shield_ingress_compliance.rs`

**Location:** `ebpf/shield/ingress_compliance.rs`

**Purpose:** Parse packets, validate service policies, enforce classification rules

**Key Functions:**
- `parse_monad_header()` - Extract 20B header from IPv6 HbH
- `lookup_service_policy()` - Check Sophia dict for allow/block decision
- `validate_classification()` - Verify K1|K0 tags match packet content
- `write_audit_record()` - Push 64B record to Anamnesis ring
- `update_metrics()` - Increment Prometheus counters

**Maps Created:**
```rust
#[map]
pub static SERVICE_POLICIES: BTreeMap<[u8; 4], ServicePolicy> = BTreeMap::with_max_entries(10000, 0);

#[map]
pub static CLASSIFICATION_RULES: BTreeMap<u8, ClassificationRule> = BTreeMap::with_max_entries(4, 0);

#[map]
pub static ANAMNESIS_AUDIT: RingBuf = RingBuf::with_max_entries(65536, 0);

#[map]
pub static COMPLIANCE_METRICS: HashMap<u32, u64> = HashMap::with_max_entries(256, 0);
```

**Estimated Size:** 450 LOC

---

### Program 2: `shield_egress_zones.rs`

**Location:** `ebpf/shield/egress_zones.rs`

**Purpose:** Validate outgoing packets respect geographic zones, enforce PII containment

**Key Functions:**
- `parse_destination_route()` - Extract next-hop from egress context
- `lookup_geo_zone()` - Check if destination zone is allowed
- `validate_pii_boundary()` - Block SOVEREIGN_PII exiting safe zone
- `update_circuit_breaker()` - Track destination reachability
- `append_compliance_flags()` - Add S flag to Monad header if needed

**Maps Created:**
```rust
#[map]
pub static GEO_ZONES: BTreeMap<u8, GeoZone> = BTreeMap::with_max_entries(16, 0);

#[map]
pub static CIRCUIT_BREAKERS: HashMap<u16, CircuitState> = HashMap::with_max_entries(1000, 0);

#[map]
pub static ZONE_STATS: HashMap<u8, ZoneStats> = HashMap::with_max_entries(16, 0);
```

**Estimated Size:** 380 LOC

---

### Program 3: `shield_audit_writer.rs`

**Location:** `ebpf/shield/audit_writer.rs`

**Purpose:** Efficient audit trail recording (kernel-side ring buffer)

**Key Functions:**
- `write_audit_record()` - Serialize {ts, decision, src, dst, class, violation} to 64B
- `compress_trace_id()` - Pack 8B trace_id into available audit space
- `get_timestamp_ms()` - Get kernel time in milliseconds

**Struct Definition:**
```rust
#[repr(C)]
pub struct AuditRecord {
    pub timestamp_ms: u32,        // [0-3]
    pub src_service_id: u16,      // [4-5]
    pub dst_service_id: u16,      // [6-7]
    pub decision: u8,              // [8]: 0=ALLOW, 1=BLOCK
    pub violation_type: u8,        // [9]: 0=None, 1=POLICY, 2=PII_EGRESS, 3=CIRCUIT
    pub classification: u8,        // [10]: K1|K0
    pub qos_level: u8,             // [11]
    pub trace_id_lo: u32,          // [12-15] (lower 32 of 64B trace_id)
    pub trace_id_hi: u32,          // [16-19] (upper 32 of 64B trace_id)
    pub zone_id: u8,               // [20]
    pub padding: [u8; 43],         // [21-63] (reserved)
}
```

**Estimated Size:** 180 LOC

---

## 7. NEW SOPHIA DICTS

### Dict 1: `service_policies`

**Schema:**
```yaml
service_policies:
  version: "1.0"
  description: "Service-to-service communication policies"
  entries:
    - src_service_id: 0x0001
      dst_service_id: 0x0002
      action: "ALLOW"
      audit_level: "INFO"
      min_qos: 1
      max_latency_ms: 100
      requires_encryption: false
      trace_sampling: 0.1  # 10% sample rate

    - src_service_id: 0x0003
      dst_service_id: 0x0004
      action: "BLOCK"
      audit_level: "ERROR"
      reason: "Unauthorized tier"

    # Default deny (catch-all)
    - src_service_id: 0xFFFF
      dst_service_id: 0xFFFF
      action: "BLOCK"
      audit_level: "WARN"
```

**BPF Map Layout:** 4B key (SrcID | DstID) → 128B struct

---

### Dict 2: `classification_tags`

**Schema:**
```yaml
classification_tags:
  version: "1.0"
  description: "Data classification rules (K1|K0 bits)"
  entries:
    - tag: "00"  # PUBLIC
      name: "Public Data"
      encryption_required: false
      geographic_constraints: []
      retention_days: 365

    - tag: "01"  # INTERNAL
      name: "Internal Data"
      encryption_required: false
      geographic_constraints: ["zone_internal"]
      retention_days: 90

    - tag: "10"  # SENSITIVE
      name: "Sensitive Data"
      encryption_required: true
      geographic_constraints: ["zone_internal"]
      retention_days: 30

    - tag: "11"  # SOVEREIGN_PII
      name: "PII (GDPR/CCPA)"
      encryption_required: true
      geographic_constraints: ["zone_eu", "zone_us_west"]
      retention_days: 7
      audit_on_access: true
```

**BPF Map Layout:** 1B key → 96B struct

---

### Dict 3: `geo_zones`

**Schema:**
```yaml
geo_zones:
  version: "1.0"
  description: "Geographic zone definitions + exit rules"
  entries:
    - zone_id: 1
      name: "US-West (PII Vault)"
      allowed_exit_zones: []  # PII cannot exit this zone
      exit_interfaces: ["eth0"]
      requires_vpn: true

    - zone_id: 2
      name: "EU (GDPR Region)"
      allowed_exit_zones: []  # GDPR data stays in EU
      exit_interfaces: []
      requires_vpn: true

    - zone_id: 3
      name: "Internal (Datacenter)"
      allowed_exit_zones: [1, 2, 4]
      exit_interfaces: ["eth1", "eth2"]
      requires_vpn: false

    - zone_id: 4
      name: "Public (DMZ)"
      allowed_exit_zones: []  # Public data, any exit OK
      exit_interfaces: ["eth3"]
      requires_vpn: false
```

**BPF Map Layout:** 1B key → 96B struct

---

### Dict 4: `circuit_breakers`

**Schema:**
```yaml
circuit_breakers:
  version: "1.0"
  description: "Circuit breaker state for destination services"
  entries:
    - service_id: 0x0002
      state: "CLOSED"  # Normal operation
      failure_threshold: 5
      success_threshold: 2
      timeout_ms: 30000

    - service_id: 0x0005
      state: "OPEN"  # Service degraded, block requests
      open_until_ts: 1741097400  # Unix seconds
      half_open_at_ts: 1741097430
      last_error: "Connection refused"
```

**BPF Map Layout:** 2B key → 64B struct

---

## 8. WOTAN TOPICS

### Topic 1: `policies.updates`

**Published by:** Captain service (when policy YAML changes)

**Message Format:**
```json
{
  "event_type": "policy.hot_swap",
  "timestamp_ms": 1741097400000,
  "trace_id": "550e8400-e29b-41d4-a716-446655440000",
  "old_policy_hash": "sha256:abc123...",
  "new_policy_hash": "sha256:def456...",
  "changed_dicts": ["service_policies", "classification_tags"],
  "change_reason": "admin_update",
  "grace_period_ms": 500,
  "rollback_available": true
}
```

**Subscribers:**
- Sophia (loads new dicts into BPF maps)
- Dashboard (shows policy change timeline)
- Trace Collector (publishes to Prometheus "policy_hot_swap" metric)

---

### Topic 2: `compliance.violations`

**Published by:** Shield ingress/egress programs (via Trace Collector)

**Message Format:**
```json
{
  "event_type": "policy.violation",
  "timestamp_ms": 1741097400000,
  "trace_id": "550e8400-e29b-41d4-a716-446655440000",
  "src_service_id": 1,
  "dst_service_id": 2,
  "violation_type": "POLICY_DENIED",
  "classification": "10",
  "action_taken": "DROP",
  "qos_level": 2,
  "zone_id": 3
}
```

**Subscribers:**
- Dashboard (real-time violation heatmap)
- Alert engine (triggers PagerDuty on critical violations)
- Compliance reporter (aggregates for SOC2/HIPAA/GDPR reports)

---

### Topic 3: `compliance.audit_complete`

**Published by:** Trace Collector (on audit ring buffer flush)

**Message Format:**
```json
{
  "event_type": "audit.batch_complete",
  "timestamp_ms": 1741097400000,
  "batch_id": "batch_20260304_001",
  "audit_records_count": 50000,
  "summary": {
    "total_packets": 50000,
    "allowed_packets": 49950,
    "blocked_packets": 50,
    "audit_completeness": 1.0,
    "policy_violation_rate": 0.001
  }
}
```

**Subscribers:**
- Dashboard (updates compliance score)
- Compliance reporter (includes in daily reports)

---

## 9. DASHBOARD INTEGRATION

### New Dashboard Sections

#### 1. Compliance Score (Top-Level)
```
┌────────────────────────────────────┐
│ Compliance Score: 99.8%            │
│                                    │
│ ████████████████████░ 99.8         │
│                                    │
│ Audit Coverage: 1,234,567 packets  │
│ Policy Violations: 2,456 total     │
│ Critical Violations: 0             │
└────────────────────────────────────┘
```

#### 2. Policy Violation Heatmap
```
Service Interaction Matrix (blocked packets):

        Service-2  Service-3  Service-4  Service-5
Service-1    0         12         0        156
Service-2    8          0         0          5
Service-3   34          0         0          0
Service-4    0          0         0          2
Service-5   12          4         1          0

Color: Red = high violation, Yellow = moderate, Green = 0
```

#### 3. Geographic Zone Map
```
Zone View: PII Containment

┌──────────────────────────────────────┐
│ US-West (PII Vault)                  │
│ Egress Attempts: 5                   │
│ Status: All Blocked ✓                │
│                                      │
│ EU (GDPR Region)                     │
│ Egress Attempts: 0                   │
│ Status: No violations ✓              │
│                                      │
│ Internal (Datacenter)                │
│ Egress Attempts: 1,234               │
│ Status: Normal ✓                     │
└──────────────────────────────────────┘
```

#### 4. Real-Time Policy Violations
```
Timestamp            Src Service   Dst Service   Reason              Action
2026-03-04 14:23:45  Service-1     Service-5     POLICY_DENIED       BLOCKED
2026-03-04 14:23:42  Service-3     Service-2     POLICY_DENIED       BLOCKED
2026-03-04 14:23:39  DemoApp       PrivateAPI    PII_EGRESS_BLOCKED  BLOCKED

[Show More] [Export CSV] [Filter...]
```

#### 5. Audit Trail Search
```
Search Audit Records:

Trace ID: _______________  [Search]
Time Range: [2026-03-01] to [2026-03-04]
Violation Type: [All] [Policy] [PII] [Circuit]

Results: 3 matches
- trace_id: 550e8400-e29b-41d4-a716-446655440000
  Path: Service-1 → Service-3 → Service-5
  Violations: 1 (POLICY_DENIED on Service-3→Service-5)
  Audit Records: 5 (full flow captured)
```

---

## 10. TESTING STRATEGY

### Unit Tests (BPF Programs)

**Test Suite:** `ebpf/tests/compliance_*.rs`

1. **Monad Header Parser**
   - Valid header (all fields set correctly)
   - Corrupted CRC (should fail validation)
   - Invalid version (should reject)
   - Edge cases (min/max service IDs)

2. **Policy Lookup**
   - Service allowed to communicate (return ALLOW)
   - Service denied (return BLOCK)
   - Policy not found (return DEFAULT_DENY)
   - Circuit breaker open (return BLOCK)

3. **Classification Validation**
   - PUBLIC (00): no constraints
   - INTERNAL (01): must stay in zone_internal
   - SENSITIVE (10): encryption required
   - SOVEREIGN_PII (11): cannot exit safe zone

4. **Audit Record Write**
   - Record fits in 64B (no truncation)
   - Timestamp included
   - Trace ID packed correctly

**Coverage Target:** >90% instruction coverage (BPF verifier metric)

---

### Integration Tests (Full Pipeline)

**Test Suite:** `tests/compliance_integration_test.go`

1. **Happy Path**
   - Packet arrives with valid header + valid policy → forwarded
   - Audit record appears in Anamnesis

2. **Policy Denial**
   - Packet arrives with valid header + denied policy → dropped
   - Audit record logged with "POLICY_DENIED" reason

3. **PII Containment**
   - PII packet (K1|K0=11) destined for safe zone → forwarded
   - PII packet destined outside safe zone → dropped + audit log

4. **Circuit Breaker**
   - Destination service marked OPEN → packets dropped
   - After timeout, service transitions to HALF_OPEN → test packet sent
   - If successful, transition to CLOSED → normal forwarding

5. **Hot-Swap**
   - Ingress policy deployed (old dict)
   - New policy pushed via Sophia dict swap
   - In-flight packets not dropped
   - New policy takes effect immediately
   - Rollback to old policy works

---

### Load Testing

**Test Suite:** `tests/compliance_load_test.go`

**Scenario:** 920K pps sustained, 100 unique service pairs, mixed policies

**Metrics:**
- Throughput: ≥920K pps (AF_XDP baseline)
- Latency P99: <1µs per policy decision
- Audit completeness: 100% (no ring buffer drops)
- Packet loss: 0%

**Command:**
```bash
go test -bench=BenchmarkCompliance920Kpps -benchtime=60s
```

---

### Chaos Testing

**Test Suite:** `tests/compliance_chaos_test.go`

1. **Bad Policy Deploy**
   - Deploy invalid policy (syntax error)
   - Verify rollback triggered automatically
   - Verify in-flight packets not affected

2. **Ring Buffer Exhaustion**
   - Generate audit records faster than reader drains
   - Verify no packet drops (ring buffer oldest record overwritten)
   - Verify Prometheus metric for "audit_ring_wraparound"

3. **Concurrent Policy Updates**
   - Update multiple dicts simultaneously
   - Verify no corruption in BPF maps
   - Verify race conditions caught by verifier

4. **Service Restart**
   - Sophia service crashes mid-policy-swap
   - Verify BPF maps remain consistent
   - Verify policy reverts to last-known-good

---

### Compliance Validation

**Manual Test Plan:**

1. **SOC2 Evidence**
   - Run for 24 hours
   - Export compliance report
   - Verify: all policy decisions audited, audit completeness = 100%
   - Verify: zero unauthorized policy_bypass_attempts

2. **HIPAA PII Containment**
   - Inject test packet with K1|K0=11 (SOVEREIGN_PII)
   - Attempt to route to non-safe zone
   - Verify: dropped + audit "PII_EGRESS_BLOCKED"
   - Verify: alert triggered in 100ms

3. **GDPR Geographic Enforcement**
   - Configure EU zone
   - Inject EU PII packet destined for non-EU egress
   - Verify: dropped at egress hook
   - Verify: audit record zone_id = 2 (EU)

---

## 11. DEPENDENCIES

### Build-Time Dependencies

| Component | Version | Purpose |
|-----------|---------|---------|
| Rust | 1.70+ | Aya BPF compilation |
| LLVM | 14+ | BPF IR generation |
| Linux headers | 5.8+ | BPF map definitions |
| Aya | 0.12+ | BPF framework |

### Runtime Dependencies

| Component | Version | Purpose |
|-----------|---------|---------|
| Linux kernel | 5.8+ | XDP, AF_XDP, BPF maps |
| BPF subsystem | 5.8+ | ring buffer, hash maps |
| Sophia service | v1.0+ | Policy dictionary management |
| Wotan | v1.0+ | Topic subscription + messaging |
| Trace Collector | v1.0+ | Ring buffer → userspace bridge |

### Shared Go Packages

| Package | Purpose |
|---------|---------|
| `pkg/wotan-client` | Wotan topic pub/sub |
| `pkg/telemetry` | Prometheus metrics + structured logging |
| `pkg/monad` | Wire format parsing |
| `pkg/auth` | Authentication (optional, for admin API) |

---

## 12. RISK REGISTER

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|-----------|
| **BPF Verifier Rejects Program** | Medium | High | Early testing with BPF_PROG_TEST_RUN; iterative simplification of complex logic |
| **Ring Buffer Capacity Exceeded** | Low | Medium | Monitor audit_ring_wraparound metric; add alerting; increase buffer if needed |
| **Policy Dict Corruption on Swap** | Low | Critical | Implement validation before swap; test rollback thoroughly; keep 3 dict versions (active, pending, last-good) |
| **Packet Drop During Hot-Swap** | Low | Medium | Implement grace period (100-500ms drain time); test with 1M pps traffic |
| **Incorrect Policy Logic** | Medium | High | Mandatory manual review of policy YAML; unit test every policy rule; integration test 50+ scenarios |
| **Performance Regression** | Medium | Medium | Run load tests before every release; track latency P99 over time; alert if >10% regression |
| **False Positive Violations** | Medium | Medium | White-list testing/debugging traffic; verify against real compliance auditor; test PII detection false positive rate |
| **Audit Completeness Gap** | Low | Critical | Compare packet count (XDP) vs audit count (ring buffer); alert if mismatch >0.1%; monthly manual audit |

---

## 13. DEFINITION OF DONE

### Code Quality
- [ ] All BPF programs pass LLVM verifier (zero warnings)
- [ ] BPF instruction coverage >90% (measured via llvm-cov)
- [ ] Go code formatted with `gofmt` + linted with `golangci-lint`
- [ ] No hardcoded secrets (checked by `git-secrets`)

### Testing
- [ ] Unit tests for all BPF functions (BPF_PROG_TEST_RUN)
- [ ] Integration tests cover happy path + 5+ edge cases
- [ ] Load test passes (920K pps, <1µs latency, 0% loss)
- [ ] Chaos tests pass (bad policy, ring buffer exhaustion, concurrent updates)
- [ ] Security review completed (zero data exfiltration risks)

### Documentation
- [ ] Architecture diagram (ASCII + Mermaid)
- [ ] BPF program comments (every function documented)
- [ ] Sophia dict schema documented in YAML
- [ ] Wotan topic schemas documented with examples
- [ ] Deployment guide with quick-start (5 steps)

### Observability
- [ ] Prometheus metrics exported (policy match rate, violation count, audit completeness)
- [ ] Structured JSON logs for all policy decisions
- [ ] Anamnesis ring buffer correctly populated (100 test packets verified)
- [ ] Dashboard renders compliance score + violation heatmap + zone map

### Deployment
- [ ] BPF programs load on kernel 5.8+ (tested on 3+ kernel versions)
- [ ] AF_XDP zero-copy verified on Intel + AMD hardware
- [ ] Policy dicts load from YAML without errors
- [ ] Hot-swap mechanism tested with 10+ policy changes
- [ ] Rollback tested (deploy bad policy, auto-revert, verify recovery <5s)

### Compliance Artifacts
- [ ] SOC2 compliance report generated (daily + weekly + monthly)
- [ ] HIPAA PII containment report (0 unauthorized egress attempts)
- [ ] GDPR geographic enforcement audit (data stayed in designated zones)
- [ ] Audit trail exportable as CSV (trace_id, timestamp, decision, reason)
- [ ] Evidence package for external auditor review

### Performance Baselines
- [ ] Ingress policy decision: <1µs P99 latency
- [ ] Egress zone validation: <500ns P99 latency
- [ ] Audit record write: <100ns per packet
- [ ] Policy hot-swap: <500ms total duration, 0 packets dropped
- [ ] Dashboard compliance score: <100ms query latency (1M+ audit records)

---

## APPENDIX A: QUICK-START (5 Steps)

1. **Clone + Build**
   ```bash
   git clone github.com/unheaded/unheaded
   cd unheaded/ebpf/shield
   cargo build --release -p compliance
   ```

2. **Load Policy Dict**
   ```bash
   curl -X POST http://localhost:19005/api/v1/dicts/service_policies \
     -d @policies.yaml
   ```

3. **Load BPF Programs**
   ```bash
   sudo ip link set dev eth0 xdp obj shield_ingress_compliance.o sec xdp
   sudo ip link set dev eth0 xdp obj shield_egress_zones.o sec xdp/egress
   ```

4. **Verify Anamnesis**
   ```bash
   sudo bpftrace -e 'tracepoint:ringbuf:* { printf("%s\n", str(args->buf)); }'
   ```

5. **Open Dashboard**
   ```
   http://localhost:20000/compliance
   ```

---

## APPENDIX B: POLICY EXAMPLE (YAML)

```yaml
# policies.yaml
metadata:
  version: "1.0"
  deployment_date: 2026-03-04
  author: admin@unheaded.io

service_policies:
  # Web → API allowed
  - src: 0x0001  # WebApp
    dst: 0x0002  # API
    action: ALLOW
    audit: INFO
    requires_encryption: true

  # API → Database allowed (high QoS)
  - src: 0x0002
    dst: 0x0003
    action: ALLOW
    audit: INFO
    min_qos: 3

  # API → Auth service allowed
  - src: 0x0002
    dst: 0x0004
    action: ALLOW
    audit: INFO

  # Dashboard → Private API BLOCKED
  - src: 0xDEAD  # DemoApp
    dst: 0x0003  # PrivateAPI
    action: BLOCK
    audit: ERROR
    reason: "Unauthorized tier"

classification_tags:
  "00":
    name: "PUBLIC"
    encryption_required: false
  "11":
    name: "SOVEREIGN_PII"
    encryption_required: true
    allowed_zones: ["zone_us_west", "zone_eu"]

geo_zones:
  zone_us_west:
    id: 1
    pii_vault: true
    exit_routes: []
  zone_eu:
    id: 2
    pii_vault: true
    exit_routes: []
  zone_internal:
    id: 3
    pii_vault: false
    exit_routes: [1, 2, 4]
  zone_dmz:
    id: 4
    pii_vault: false
    exit_routes: []
```

---

**END OF BATTLE PLAN**

---

**Authority:** Unheaded Kingdom Executive Council
**Classification:** INTERNAL
**Distribution:** Engineering + Compliance Teams
**Next Review:** March 18, 2026
