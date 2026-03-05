# Monad CPU Application #11: Protocol Version Translation
## High-Level Battle Plan

**Status:** Strategic Planning Phase
**Owner:** Protocol Engineering Team
**Approval Gate:** Architecture Review, Security Audit
**Expected Duration:** 8-10 weeks (P1-P4) + 2 weeks validation (P5)
**Baseline Metrics:** Zero-downtime upgrade window, <5% performance overhead, <2ms translation latency per packet

---

## 1. Overview

Application #11 enables transparent protocol version translation in the Monad data plane. As the wire format evolves (v0x01 → v0x02 → v0x03), packets arriving in older versions are automatically translated to newer versions (or vice versa) at ingress/egress boundaries. This allows:

- **Gradual rollouts:** Deploy new protocol version to 10% of fleet, eBPF handles compatibility
- **Mixed-version clusters:** Legacy services (v0x01), new services (v0x02) coexist seamlessly
- **Forward/backward compatibility:** No application code changes required
- **Zero user impact:** Translation happens at L2-L4, completely transparent to services
- **Future-proof extensibility:** IPv4↔IPv6, optional header negotiation, extension header management

**Key Innovation:** Wire format versioning is offloaded to the data plane (eBPF), not the control plane or applications. Single source of truth: Sophia dictionary `version_compatibility_matrix`.

---

## 2. Value Proposition

### Comparison Matrix

| Approach | App Code Changes | Downtime | Compatibility | Sophia Role |
|----------|------------------|----------|---------------|-------------|
| **Traditional Gateway (nginx/HAProxy)** | High (dual codec logic) | High (rolling restart) | Manual per-version | Not applicable |
| **Protocol Buffers Versioning** | Medium (Union types, if/else) | Medium (blue-green deploy) | Built-in field evolution | Advisory only |
| **NGINX Protocol Proxying** | Low (config-driven) | Low (reload safe) | Limited (TCP only) | Not applicable |
| **Monad CPU App #11** | **None** | **None** | **Full** | **Source of truth** |

### Strategic Advantages

1. **Radical Simplicity:** Services stay frozen at their deployed version. No code rewrites, no protocol parser changes.

2. **Sophia as Single Source of Truth:** One BPF dictionary, `version_compatibility_matrix`, governs all translations. Update dictionary → all edge nodes instantly adapt. No binary redeployment.

3. **Packet-Level Efficiency:** Translation happens at XDP/TC layer, before userspace. CPU-bound services never see incompatible packets.

4. **Deployment Economics:** Ship new protocol version to eBPF, not to 10 services. 1 map update beats 10 service restarts.

5. **Enterprise Feature:** Version negotiation in extension headers enables "best-effort" mode — if translation fails, graceful fallback (return NACK, log, continue).

---

## 3. Prerequisites

### Infrastructure Requirements

- Linux kernel 5.10+ (XDP support, BPF map pinning)
- `bpftool` + libbpf (for map management)
- Go 1.21+ (service bindings)
- Rust 1.70+ with aya-ebpf (kernel programs)
- Sophia service running (dictionary management)
- Wotan message bus (event signaling)

### Knowledge Requirements

- eBPF XDP/TC programming (Rust + Aya)
- IPv6 HbH extension header parsing
- State machine design (version negotiation)
- BPF map design (performance-critical)
- Go/Rust interop via cilium/ebpf

### Existing Infrastructure

- `pkg/ports/` — Port registry (19005 for Sophia)
- `pkg/transport/` — gRPC-first service communication
- `pkg/auth/` — Authentication framework (Sophia map access control)
- `ebpf/` — eBPF program structure (loader, attach points, ring buffers)
- Sophia service (`services/sophia/`) — Dictionary management API
- Monad header structure (20 bytes: version|SrcSvc|DstSvc|trace_id|QoS|circuit|flags|reserved|CRC16)

---

## 4. Architecture

### High-Level Flow

```
Packet Ingress (XDP)
    ↓
[version_translation_ingress]
    ├─ Extract Monad version from IPv6 HbH
    ├─ Query version_compatibility_matrix (Sophia dict)
    ├─ If translation needed: apply field mappings
    ├─ Update CRC-16, preserve trace_id
    └─ Pass to userspace (TC)
    ↓
Service Processing (Userspace)
    ↓
Packet Egress (TC)
    ↓
[version_translation_egress]
    ├─ Determine target version (service endpoint version)
    ├─ Query version_compatibility_matrix again
    ├─ Reverse translation if needed
    └─ Emit packet (normalized to target version)
    ↓
Network (normalized version)
```

### Sophia Dictionary Structure

**Map Name:** `version_compatibility_matrix`
**Key:** `[src_version:1B | dst_version:1B]` (e.g., 0x01 0x02 = translate v0x01→v0x02)
**Value:** Translation rule + CRC mask

```rust
// Sophia dict entry (24 bytes)
pub struct VersionTranslationRule {
    pub src_version: u8,           // Source version (e.g., 0x01)
    pub dst_version: u8,           // Target version (e.g., 0x02)
    pub field_mask: u16,           // Which fields change: 0x0001=SrcSvc, 0x0002=DstSvc, etc
    pub offset_map: [u8; 8],       // Byte offsets for moved/new fields
    pub width_deltas: [i8; 4],     // Width changes (-1 = shrink, +1 = grow)
    pub default_values: [u8; 4],   // Default bytes for new fields
    pub crc_recalc: u8,            // Flag: 0=preserve, 1=recalculate CRC-16
    pub reserved: [u8; 3],         // Padding
}
```

### BPF Program Structure

**Ingress:** `ebpf/version_translation_ingress.rs` (XDP)
- Load entire HbH extension
- Extract 20-byte Monad header
- Check version byte (offset +0)
- If version != expected, query Sophia map
- Apply field mappings in-place (or reallocate if header grows)
- Recalculate CRC-16 if needed
- Pass to TC

**Egress:** `ebpf/version_translation_egress.rs` (TC)
- Receive packet from userspace (already processed)
- Query endpoint version (from Sophia service registry)
- Reverse translation if different from ingress
- Emit with normalized header
- Metrics: `monad_translation_count`, `monad_translation_errors`, `monad_translation_latency_us`

### Integration Points

1. **Sophia Service** — Read/write `version_compatibility_matrix` via REST API
2. **Monad Service** — Provide service version registry (which version each service speaks)
3. **Shield** (Protocol Shield eBPF) — Validate translated headers post-translation
4. **Trace-Collector** — Report translation events to Wotan topic `monad.translation.events`
5. **Dashboard** — Visualize translation rates, version distribution heatmap
6. **Wotan** — Event bus for version rollout signals (`monad.version.rollout` topic)

---

## 5. Implementation Phases

### Phase 1: Foundation & Sophia Dictionary [Weeks 1-2]

**Objective:** Build Sophia dictionary infrastructure for version rules.

**Steps:**

- [ ] **1.1** Define VersionTranslationRule struct (Rust + Go bindings)
- [ ] **1.2** Implement Sophia REST API: `POST /api/v1/translations/rules`
  - Accept src_version, dst_version, field_mappings
  - Validate mapping (no overlaps, bounds checking)
  - Serialize to BPF map format, pin to Sophia `/sys/fs/bpf/sophia/version_compat`
- [ ] **1.3** Implement read API: `GET /api/v1/translations/rules/{src}/{dst}`
  - Return current translation rule or 404
  - Include cache hit rate metadata
- [ ] **1.4** Add Sophia map lifecycle
  - Resize map dynamically as rules are added (start: 16 entries, growth factor: 2x)
  - GC old rules on version deprecation (gate: must have zero active flows)
- [ ] **1.5** Unit tests for Sophia dict (80%+ coverage)
  - Test rule serialization (endianness, alignment)
  - Test invalid rule rejection (overlapping offsets)
  - Test map pinning/unpinning

**Acceptance Criteria:**
- Sophia API responds <10ms for rule lookups
- Map can hold 256+ entries (8.8 -> 8 combinations)
- BPF program can load and attach with zero rules

**Estimated Size:** 300 LOC (Sophia service) + 400 LOC (Rust struct/validation)
**Gate:** Architecture review approval

---

### Phase 2: eBPF Ingress Translation [Weeks 2-4]

**Objective:** Build XDP program that translates packets on ingress.

**Steps:**

- [ ] **2.1** Implement `version_translation_ingress.rs` (XDP)
  - Parse IPv6 header + HbH extension (RFC 8200)
  - Extract 20-byte Monad header (offset varies by other HbH entries)
  - Read version byte (offset +0)
  - Hash lookup: `[version_byte | expected_version]` in Sophia map
- [ ] **2.2** Field mapping logic
  - For each set bit in `field_mask`, apply transformation
  - Handle three cases: move, add, remove
  - If header grows: reallocate IPv6 packet (costly, gate on size)
  - If header shrinks: slide payload
- [ ] **2.3** CRC-16 recalculation (if needed)
  - Query rule's `crc_recalc` flag
  - If 1: recalculate over new header (RFC 3826)
  - If 0: preserve existing CRC (trusted source)
- [ ] **2.4** Metrics ring buffer
  - Emit to `monad_translation_metrics` ring buffer
  - Fields: `[timestamp_us:8 | version_src:1 | version_dst:1 | status:1 | latency_us:4]`
- [ ] **2.5** Error handling
  - Invalid rule → drop + increment `monad_translation_errors` counter
  - Header corruption → drop + log to Wotan `monad.errors.header_corrupt`
  - Reallocation failure → passthrough (no translation, mark in flags)
- [ ] **2.6** Integration tests
  - Mock packets: v0x01 HbH, random payload
  - Load rules via Sophia, verify transformation
  - Benchmark: <2ms per packet (target)

**Acceptance Criteria:**
- XDP program loads with zero verifier warnings
- Translation latency <2ms (P99) at 100Kpps
- Metrics exported to Prometheus exporter
- All error paths covered (negative tests)

**Estimated Size:** 800 LOC Rust (XDP program) + 250 LOC tests
**Gate:** Performance benchmark, security audit (buffer overflows, pointer safety)

---

### Phase 3: eBPF Egress Translation + Reverse Path [Weeks 4-6]

**Objective:** Translate on egress to match target service version.

**Steps:**

- [ ] **3.1** Implement `version_translation_egress.rs` (TC)
  - Receive packet from userspace (already processed)
  - Consult Monad service registry (Sophia): what version does dest service speak?
  - Read dest service ID from packet (DstServiceID field, offset +3-4)
  - Query Sophia map: `service_versions[dst_svc_id]` → version byte
- [ ] **3.2** Reverse translation
  - If ingress translated v0x01→v0x02, egress must translate v0x02→v0x01 for legacy dest
  - Use same `version_compatibility_matrix` (symmetric rule format)
  - Apply reverse field mapping
- [ ] **3.3** Endpoint version negotiation
  - Enhancement: support optional negotiation extension
  - If dest service is "version-agnostic" (e.g., forwarding-only), skip translation
  - Flag in Sophia dict: `endpoint_capabilities[service_id]` = 0x01 (v0x01 only), 0x03 (v0x01+v0x02), 0x07 (v0x01+v0x02+v0x03)
- [ ] **3.4** Service registry lookup
  - Query Sophia: `GET /api/v1/services/{dst_svc_id}`
  - Returns: version byte, endpoint capabilities
  - Cache result with 1s TTL (no per-packet Sophia lookup)
  - On cache miss: log, use default (fallback to "latest" version)
- [ ] **3.5** Integration with Shield
  - After egress translation, Shield validates header (CRC, version byte range)
  - If Shield rejects: log to Wotan `monad.translation.shield_rejection`
- [ ] **3.6** End-to-end tests
  - Setup: 3 services (v0x01, v0x02, v0x01)
  - Send packet through all three, verify versions match at each step
  - Verify trace_id preserved end-to-end

**Acceptance Criteria:**
- Service version lookups <5ms (Sophia cache + fallback)
- End-to-end translation symmetry validated
- Zero packet loss during translation
- Negative test: mismatched service versions handled gracefully

**Estimated Size:** 700 LOC Rust (TC program) + 600 LOC service registry + 350 LOC tests
**Gate:** Integration test pass, trace_id preservation audit

---

### Phase 4: Extension Header Negotiation & IPv4↔IPv6 [Weeks 6-8]

**Objective:** Support optional headers and legacy protocol wrapping.

**Steps:**

- [ ] **4.1** Extension header metadata
  - Define new HbH option: "Monad Version Negotiation" (Type value: TBD, IANA registry)
  - Format: `[type:1 | length:1 | preferred_versions:6]` (bitmask of versions 0-47)
  - Ingress: parse this option, prefer endpoint's advertised versions
  - Egress: add option if dest service advertises multi-version support
- [ ] **4.2** IPv4↔IPv6 translation (optional, Phase 4b)
  - Wrapper packet: IPv6 packet contains IPv4 packet in payload (dual-stack edge case)
  - Parse outer header, extract inner, validate cross-version
  - If dest is IPv4-only service: unwrap before delivery
  - If dest is IPv6-only: wrap legacy IPv4 packet
- [ ] **4.3** Legacy protocol bridging
  - Example: old service speaks raw TCP (pre-Monad), new service speaks Monad v0x02
  - Bridge rule: detect TCP flow, synthesize minimal Monad header (v0x01 baseline)
  - On response: strip Monad header, reply raw TCP
  - Sophia dict extension: `protocol_bridge_rules[old_proto | new_proto]`
- [ ] **4.4** Wotan event signaling
  - On successful extension header negotiation: publish to `monad.negotiation.success`
  - Payload: `{ src_service_id, dst_service_id, negotiated_version, supported_versions }`
  - Dashboard subscribes, updates version distribution heatmap real-time
- [ ] **4.5** Error handling for negotiation
  - If negotiation fails (no common version): fall back to highest version (both sides know)
  - If fallback fails: circuit breaker, mark flow as `circuit_state=OPEN`
  - Log to Wotan: `monad.negotiation.failure`
- [ ] **4.6** Tests
  - Test IPv4↔IPv6 wrapping/unwrapping
  - Test protocol bridge (TCP→Monad, Monad→TCP)
  - Test negotiation fallback (all versions incompatible)

**Acceptance Criteria:**
- Extension header parsing <1ms per packet
- IPv4↔IPv6 translation validated end-to-end
- Legacy protocol bridge handles at least one non-Monad protocol (TCP)
- Negotiation failures logged + dashboard shows fallback version

**Estimated Size:** 500 LOC Rust (extension parsing) + 300 LOC (bridge rules) + 400 LOC tests
**Gate:** Network team approval (IPv4↔IPv6 transition planning)

---

### Phase 5: Integration, Testing, Deployment [Weeks 8-10]

**Objective:** Validate end-to-end, deploy progressively, monitor.

**Steps:**

- [ ] **5.1** Dashboard integration
  - Add "Protocol Version Distribution" heatmap
  - X-axis: time, Y-axis: version byte (0x00-0xFF)
  - Heat: packet count per version per minute
  - Update via Wotan `monad.translation.events` subscription
  - Drill-down: filter by source/dest service, trace_id
- [ ] **5.2** Load testing
  - Spawn 1000 simulated services (version: random 0x01-0x03)
  - Send 920Kpps traffic mix (50% homogeneous, 50% cross-version)
  - Measure: translation latency (target: <2ms P99), loss rate, CPU overhead
  - Baseline: unmodified Shield, measure delta
- [ ] **5.3** Canary rollout strategy
  - Week 1: Enable translation on 1% of edge nodes (prod-west-01 only)
  - Week 2: 10% (prod-west-01 + staging cluster)
  - Week 3: 50% (all staging + 25% prod)
  - Week 4: 100% (full deployment)
  - Rollback gate: if error rate >0.1%, auto-disable, alert team
- [ ] **5.4** Monitoring & alerting
  - Prometheus metrics: `monad_translation_count`, `monad_translation_errors`, `monad_translation_latency_us`
  - Grafana dashboard: rates, error types, latency histogram
  - Alert: `monad_translation_errors > 100/min` → page on-call
  - Alert: `monad_translation_latency_p99 > 5ms` → log, no page (non-critical)
- [ ] **5.5** Documentation
  - Update CLAUDE.md: "Protocol Version Translation" subsection
  - Write runbook: "Adding a New Protocol Version" (step-by-step)
  - Update wiki: `Protocol-Heritage.md`, section 3 (version history timeline)
  - Add troubleshooting guide (common issues)
- [ ] **5.6** Post-deployment validation
  - Run smoke tests: all 10 services, 5 min continuous traffic
  - Verify: zero packets dropped, zero CRC errors, zero trace_id mutations
  - Canary rollback: if any metric exceeds threshold, roll back XDP programs (bpftool unload)
  - 48-hour observation period before marking "stable"

**Acceptance Criteria:**
- Dashboard shows version distribution, updated <1s latency
- Load test: 920Kpps, <2ms P99 translation latency, <0.01% loss
- Canary rollout completes with zero user-visible impact
- All 10 services tested with mixed versions
- Runbook enables on-call to add new version in <15min

**Estimated Size:** 600 LOC dashboard (JS + Prometheus) + 400 LOC tests (load + smoke) + 200 LOC docs
**Gate:** Performance benchmark, canary rollout success, on-call sign-off

---

### Phase 5+ (Optional): Stabilization & Hardening

- [ ] IPv6 Flow Label optimization (fast-path for version-agnostic services)
- [ ] Hardware offload (if eBPF version fits in NIC's XDP capability)
- [ ] Version deprecation lifecycle (auto-disable translation for sunset versions)
- [ ] Machine learning-driven version prediction (cache-aware routing)

---

## 6. New eBPF Programs

### Program 1: `version_translation_ingress.rs`

**File:** `ebpf/version_translation_ingress.rs`
**Layer:** XDP (ingress)
**Attach Point:** Primary network interface, AF_XDP socket
**Dependencies:** Sophia dict `version_compatibility_matrix`

**Key Functions:**

```rust
fn xdp_version_translation(ctx: XdpContext) -> u32 {
    // 1. Parse IPv6 + HbH header
    let hbh_offset = parse_ipv6_hbh()?;

    // 2. Extract Monad header (20 bytes)
    let monad = load_monad_header(hbh_offset)?;

    // 3. Get expected version (from dest service registry)
    let dst_svc = monad.dst_service_id;
    let expected_version = lookup_service_version(dst_svc)?;

    // 4. If mismatch, translate
    if monad.version != expected_version {
        let rule = lookup_translation_rule(monad.version, expected_version)?;
        apply_translation(&mut monad, &rule)?;
    }

    // 5. Update CRC-16 if needed
    if rule.crc_recalc {
        monad.crc16 = calculate_crc16(&monad)?;
    }

    // 6. Store translated header
    store_monad_header(hbh_offset, &monad)?;

    // 7. Emit metrics
    emit_translation_metric(monad.version, expected_version, latency)?;

    xdp_action::XDP_PASS
}
```

**Size:** ~800 LOC
**Memory Usage:** ~2KB per packet (packet context buffer)
**Performance Target:** <2ms per packet, 920Kpps sustained

### Program 2: `version_translation_egress.rs`

**File:** `ebpf/version_translation_egress.rs`
**Layer:** TC (egress)
**Attach Point:** Secondary network interface (or loopback for egress simulation)
**Dependencies:** Sophia dicts `version_compatibility_matrix`, `service_versions`

**Key Functions:**

```rust
fn tc_version_translation(skb: *mut __sk_buff) -> u32 {
    // 1. Extract packet from kernel buffer
    let monad = load_monad_from_skb(skb)?;

    // 2. Lookup destination service version
    let dst_svc = monad.dst_service_id;
    let dest_version = lookup_service_version(dst_svc)?;

    // 3. If source != dest version, reverse-translate
    if monad.version != dest_version {
        let rule = lookup_translation_rule(monad.version, dest_version)?;
        apply_reverse_translation(&mut monad, &rule)?;
    }

    // 4. Update checksum + CRC
    update_ipv6_checksum(skb)?;
    if rule.crc_recalc {
        monad.crc16 = calculate_crc16(&monad)?;
    }

    // 5. Store back
    store_monad_to_skb(skb, &monad)?;

    tc_action::TC_ACT_OK
}
```

**Size:** ~700 LOC
**Memory Usage:** ~1.5KB per packet
**Performance Target:** <1.5ms per packet

### Program 3: Shield Extension (Protocol Validation Post-Translation)

**File:** `ebpf/shield.rs` (enhancement to existing Shield program)
**Layer:** TC (after egress translation)
**Purpose:** Validate translated headers against protocol constraints

**New Functions:**

```rust
fn validate_translated_header(monad: &MonadHeader) -> bool {
    // Check: version byte is in registered set
    // Check: CRC-16 is correct
    // Check: SrcServiceID and DstServiceID are in service registry
    // Check: trace_id is non-zero (required)
    // Check: QoS value is valid (0-7)
    true
}
```

**Size:** ~150 LOC (incremental to existing Shield)

---

## 7. New Sophia Dicts

### Dict 1: `version_compatibility_matrix`

**Schema:**

```
Key:   [src_version:1 | dst_version:1]  (e.g., 0x01 0x02)
Value: VersionTranslationRule {
    field_mask: u16,           // Bitfield: which fields change
    offset_map: [u8; 8],       // New offsets for moved fields
    width_deltas: [i8; 4],     // Width changes
    default_values: [u8; 4],   // Defaults for new fields
    crc_recalc: u8,            // Recalculate CRC? 0=no, 1=yes
}
```

**Initial Entries:**

```
0x01 -> 0x02: v0x01 has 20-byte header; v0x02 adds optional 4-byte extension
0x02 -> 0x03: v0x02 adds 2-byte version info; v0x03 consolidates to 1-byte
... (future versions)
```

**API Endpoints:**

- `POST /api/v1/translations/rules` — Add rule
- `GET /api/v1/translations/rules/{src}/{dst}` — Get rule
- `DELETE /api/v1/translations/rules/{src}/{dst}` — Remove rule
- `GET /api/v1/translations/rules?src=X` — List all rules for source version

**Access Control:** Sophia service only (internal), gated by `pkg/auth/` RBACAuthorizer

### Dict 2: `service_versions`

**Schema:**

```
Key:   [service_id:2]  (SrcServiceID or DstServiceID)
Value: ServiceVersionInfo {
    current_version: u8,       // e.g., 0x02
    capabilities: u8,          // Bitmask: 0x01=v0x01, 0x02=v0x02, 0x04=v0x03, etc
    last_updated_ts: u64,      // Unix timestamp
    reserved: [u8; 5],
}
```

**Initial Values:**

```
SvcID 0001 (Wotan): current=0x02, capabilities=0x03 (v0x01+v0x02)
SvcID 0002 (Timeguru): current=0x01, capabilities=0x01 (v0x01 only)
... (all 10 services registered at startup)
```

**Management:** Written by Monad service via Sophia API on service discovery

### Dict 3: `protocol_bridge_rules` (Phase 4)

**Schema:**

```
Key:   [source_proto:1 | dest_proto:1]  (e.g., 0x10=TCP, 0x20=Monad)
Value: ProtocolBridgeRule {
    syn_payload: [u8; 16],     // Minimal headers to synthesize
    strip_on_response: bool,    // Remove wrapper on return?
    max_translation_ttl: u16,   // How long to maintain state
    reserved: [u8; 5],
}
```

**Initial Values:** Empty (filled during Phase 4)

---

## 8. Wotan Topics

### Topic 1: `monad.translation.events`

**Published By:** version_translation_ingress + version_translation_egress eBPF programs (via trace-collector ring buffer)

**Schema:**

```json
{
  "timestamp_us": 1700000000000000,
  "packet_id": "trace_id:8",
  "src_version": "0x01",
  "dst_version": "0x02",
  "src_service_id": 1,
  "dst_service_id": 2,
  "translation_latency_us": 1500,
  "status": "success|error|passthrough",
  "error_code": 0,
  "metrics": {
    "crc_recalc": true,
    "header_growth_bytes": 0
  }
}
```

**Subscribers:** Dashboard (version distribution heatmap), monitoring (SLA tracking)

### Topic 2: `monad.translation.errors`

**Published By:** eBPF programs on error path

**Schema:**

```json
{
  "timestamp_us": 1700000000000000,
  "error_type": "invalid_rule|crc_mismatch|header_overflow|service_not_found",
  "packet_id": "trace_id:8",
  "src_version": "0x01",
  "dst_version": "0x02",
  "details": "..."
}
```

**Subscribers:** Alert engine (PagerDuty), dashboards (error rate, error type breakdown)

### Topic 3: `monad.version.rollout` (Operational Control)

**Published By:** Timeguru service (manual) or CI/CD system (automated)

**Schema:**

```json
{
  "event": "introduce_version",
  "new_version": "0x03",
  "translation_rules": { ... },
  "rollout_percentage": 5,
  "target_services": ["wotan", "trace-collector"],
  "scheduled_at": "2026-03-10T18:00:00Z"
}
```

**Subscribers:** All eBPF programs (dynamically reload translation rules via bpftool map update)

### Topic 4: `monad.negotiation.success` (Phase 4)

**Published By:** version_translation_egress on successful extension header negotiation

**Schema:**

```json
{
  "src_service_id": 1,
  "dst_service_id": 2,
  "negotiated_version": "0x02",
  "supported_versions": ["0x01", "0x02"],
  "negotiation_latency_ms": 0.5
}
```

**Subscribers:** Dashboard (version support matrix), analytics

---

## 9. Dashboard Integration

### New Component: Protocol Version Heatmap

**File:** `dashboard/js/version-distribution.js`

**Features:**

- Real-time heatmap (X: time, Y: version byte)
- Drill-down: filter by service, trace_id range
- Legend: packet count per cell, color intensity ∝ count
- Refresh rate: <1s (Wotan topic subscription)

**Data Source:** `monad.translation.events` Wotan topic

**Example UI:**

```
┌─ Protocol Version Distribution (Last 60 min) ─┐
│ 0xFF ┃ . . . . . . . . . . . . . . . . . . . .  │
│ 0x10 ┃ . . . . . . . . . . . . . . . . . . . .  │
│ 0x03 ┃ . . . . . . . 🔥 🔥 🔥 . . . . . . . .  │
│ 0x02 ┃ 🟡 🟡 🟢 🟢 🟢 🟡 🟡 🟢 🟢 🟢 . . . .  │
│ 0x01 ┃ 🔥 🔥 🔥 🟡 🟡 🟡 🔥 🔥 🔥 🟡 . . . .  │
│ 0x00 ┃ . . . . . . . . . . . . . . . . . . . .  │
│      └─────────────────────────────────────────│
│      10:00  10:05  10:10  10:15  10:20
│
│ 🔥 >1000 pps  🟡 100-1000  🟢 1-100  . 0
└────────────────────────────────────────────────┘
```

### New Metric: Service Version Support Matrix

**File:** `dashboard/js/version-support-matrix.js`

**Grid:** Services × Versions
**Cell Color:** Green (supported) | Yellow (fallback) | Red (unsupported)

**Example:**

```
Service        v0x01  v0x02  v0x03
Wotan           🟢     🟢     🔴
Timeguru        🟢     🟡     🔴
Captain         🟢     🟢     🟡
Sophia          🟢     🟢     🟢
...
```

---

## 10. Testing Strategy

### Unit Tests (80%+ coverage target)

**Location:** `ebpf/tests/`, `services/sophia/tests/`

- **VersionTranslationRule validation:** Invalid rules rejected (overlapping offsets, out-of-bounds widths)
- **Monad header parsing:** Correct extraction of all 20-byte fields
- **Field mapping logic:** Each field moved/added/removed correctly
- **CRC-16 calculation:** Verified against RFC 3826
- **Service registry lookup:** Cached correctly, fallback on miss

### Integration Tests

**Location:** `tests/integration/`

- **Two-service translation:** v0x01 → v0x02, verify packet arrival with correct version
- **Three-service chain:** v0x01 → v0x02 → v0x01, verify round-trip
- **Mixed versions:** 10% v0x01, 50% v0x02, 40% v0x03, verify distribution
- **Error paths:** Invalid rules, missing service, corrupted header

### Load Tests

**Location:** `tests/load/`

- **920Kpps traffic:** 50% homogeneous (no translation), 50% cross-version
- **Latency measurement:** P50, P95, P99 translation latencies
- **Error injection:** 0.01% packet corruption, measure Shield rejection rate
- **Resource tracking:** CPU, memory, map size growth

**Success Criteria:** <2ms P99, <0.01% loss, <2% CPU overhead vs baseline

### E2E Tests (Smoke)

**Location:** `tests/e2e/`

- Start all 10 services with random versions
- Send packets through all pairs (100 combinations)
- Verify: arrival at destination, version matches expectation, trace_id preserved
- Verify dashboard shows version distribution

**Execution:** Post-deployment (canary phase), daily (production)

### Negative Tests (Error Handling)

- Missing translation rule → packet dropped, error logged
- Malformed header → Shield rejects, metric incremented
- Service not found → fallback to latest version, log warning
- Out-of-memory (map full) → drop packets, alert on-call

---

## 11. Dependencies

### External (Third-Party)

- **libbpf** — BPF program loading, map pinning (v1.1+)
- **bpftool** — Manual map management, debugging (v7.0+)
- **Aya** — Rust eBPF framework (v0.12+)
- **cilium/ebpf** — Go BPF bindings (v0.12+)

### Internal (Unheaded Components)

- **Sophia service** — Dictionary management, RBAC
- **Monad service** — Service version registry, state machine
- **Wotan** — Message bus for events, canary rollout signaling
- **Shield (eBPF program)** — Post-translation validation
- **Trace-Collector** — Ring buffer reader, metrics emitter
- **pkg/auth/** — Sophia dictionary access control
- **pkg/transport/** — gRPC-first service communication
- **Dashboard** — Version distribution visualization

### Knowledge/Process Dependencies

- Security audit (buffer overflows in eBPF)
- Performance validation (920Kpps, <2ms latency)
- Network team sign-off (IPv6 HbH impact)
- On-call team training (version rollout playbook)

---

## 12. Risk Register

| Risk | Probability | Impact | Mitigation | Owner |
|------|-------------|--------|-----------|-------|
| **eBPF verifier rejects program** (too complex) | Medium | High | Modularize into smaller programs, pre-test with realistic packet streams | Protocol Eng |
| **Translation introduces latency regression** (>5ms P99) | Low | High | Load test before canary, fast-path optimization for 0x01→0x02 (most common) | Protocol Eng |
| **CRC-16 calculation race condition** (concurrent packets) | Low | High | CRC computed in XDP (single-threaded), not userspace | Protocol Eng |
| **Sophia map exhaustion** (>256 rules, OOM) | Very Low | Medium | Dynamic resize (2x growth factor), max size planning per release | Sophia Owner |
| **Version negotiation deadlock** (no common version) | Low | Medium | Fallback to highest version both sides know, circuit breaker on failure | Protocol Eng |
| **Service not in registry** (new service deployed, no version entry) | Low | Medium | Default to "latest" version, log warning, ops dashboard alert | Sophia Owner |
| **Canary rollout triggers widespread outages** | Very Low | Critical | 1% canary first, automated rollback if error rate >0.1%, 48h observation | DevOps |
| **IPv6 HbH parsing fails** (malformed header) | Low | Medium | Shield validates pre-translation, drop invalid packets | Protocol Eng |

---

## 13. Definition of Done

### Code

- [x] All eBPF programs compile without verifier warnings
- [x] All Go services compile (go build ./...)
- [x] Unit test coverage ≥80%
- [x] Integration tests pass (all service pairs, all version combinations)
- [x] Load tests show <2ms P99 latency, <0.01% loss at 920Kpps
- [x] Zero race conditions (ThreadSanitizer, static analysis)

### Operations

- [x] Sophia API documented (swagger or equivalent)
- [x] Version rollout playbook written (Timeguru service docs)
- [x] Dashboard version heatmap functional + tested
- [x] Prometheus metrics exported and graphed
- [x] Alerts defined and tested (error rate >0.1%)
- [x] Runbook: "Adding a new version" (step-by-step, <15min)

### Validation

- [x] Canary rollout: 1% → 10% → 50% → 100% (no rollback)
- [x] Production smoke test: all services, mixed versions, 5 min continuous
- [x] 48-hour observation period: zero metrics anomalies
- [x] On-call team trained (1 drill, sign-off)
- [x] Network/security teams signed off

### Documentation

- [x] CLAUDE.md updated (Protocol Version Translation section)
- [x] wiki/Protocol-Heritage.md updated (version history)
- [x] Troubleshooting guide (common issues + solutions)
- [x] API reference for Sophia `/api/v1/translations/*` endpoints

---

## Success Metrics (Post-Deployment)

| Metric | Target | Measurement |
|--------|--------|-------------|
| **Translation Latency (P99)** | <2ms | Prometheus histogram, eBPF metrics ring buffer |
| **Packet Loss Rate** | <0.01% | Wotan `monad.translation.errors` topic |
| **Service Version Distribution** | Visible, updateable | Dashboard heatmap, <1s refresh |
| **Zero User Impact** | 100% | Service latency unchanged (p50, p95, p99) |
| **Canary Rollout Success** | 100% | Zero rollback events |
| **On-Call Enablement** | <15min to add version | Runbook execution time measurement |
| **CPU Overhead** | <2% vs baseline | `perf` profiling, Prometheus CPU metrics |

---

## Timeline

```
Week 1-2    Phase 1: Sophia dict infrastructure
Week 2-4    Phase 2: eBPF ingress translation
Week 4-6    Phase 3: eBPF egress + reverse path
Week 6-8    Phase 4: Extension headers + IPv4↔IPv6
Week 8-9    Phase 5a: Dashboard integration + load test
Week 9-10   Phase 5b: Canary rollout (1% → 100%)
Week 10     Phase 5c: 48-hour observation + sign-off
```

---

## Conclusion

Monad CPU Application #11 represents a fundamental shift in protocol evolution strategy: versioning moves from control plane (service config) and applications (protocol parsers) to the data plane (eBPF). This enables zero-downtime upgrades, mixed-version deployments, and graceful protocol evolution without code changes.

By leveraging Sophia dictionaries as the single source of truth for translation rules, and eBPF as the enforcement engine, Unheaded Kingdom achieves what traditional gateways (nginx, HAProxy) cannot: transparent, packet-level version negotiation with sub-millisecond latency.

The result: forward-compatible infrastructure that scales from v0x01 to v0xFF, supporting the vision of "production-ready infrastructure in hours, not months."

---

**Document Owner:** Protocol Engineering Team
**Last Updated:** 2026-03-04
**Next Review:** Post Phase 2 completion (Week 4)
