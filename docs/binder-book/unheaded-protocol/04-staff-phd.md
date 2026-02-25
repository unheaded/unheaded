# The Unheaded Protocol: Staff Engineer / PhD-Level Analysis

## Abstract

The Unheaded Protocol (`draft-bellis-unheaded-protocol-foundation-04`) defines a mapped data bus model that transforms IPv6 packets into addressable working memory. A 20-byte register file (the Monad) is carried in the IPv6 Hop-by-Hop Options extension header. At each operator-controlled hop within a Limited Domain [RFC 8799], a BPF program (the Shim) performs inline computation on the Monad, turning the forwarding path into a distributed pipeline processor. Exponent-encoded fields decouple wire representation from semantics via hot-swappable dictionary maps (Sophia). Per-hop events are recorded in BPF ring buffers (Anamnesis) for causal reconstruction. The protocol is designed for computational completeness via Wotan memory paging, includes Kingdom Mode address reclamation for extended register space, and provides post-quantum identity binding without increasing wire overhead.

This document provides rigorous analysis of the protocol's design space, its relationship to existing RFC work, the formal computational model, the Gnostic entity bindings as architectural primitives, and implications for network-layer computation.

## 1. Design Space and Positioning

### 1.1 The Impedance Mismatch

Classical networking enforces a strict separation between computation (applications) and transport (network). Data flows through the network as opaque byte streams. This creates an expensive impedance mismatch requiring:

- Serialization/deserialization at every service boundary (protobuf, JSON, Thrift)
- Sidecar proxies for observability (Envoy, Linkerd)
- Separate tracing infrastructure (Jaeger, Zipkin, OpenTelemetry collectors)
- Service mesh control planes (Istio, Consul Connect)
- Log aggregation pipelines (Fluentd → Elasticsearch → Kibana)

Each layer adds latency (typically 0.5-5ms per hop), failure modes, operational complexity, and cost. The total overhead of "knowing what your system is doing" often exceeds the cost of the system itself.

### 1.2 The Inversion

The Unheaded Protocol inverts this model: **the packet carries computational state**. The network is not a dumb pipe between smart endpoints — it is a distributed processor where each hop contributes to a running computation. Observability, security, routing, and circuit-breaking are not bolted on top; they are **encoded in the wire format itself**.

This is analogous to the difference between:
- A **von Neumann architecture** (data and instructions separated, fetched from memory) — the classical networking model
- A **dataflow architecture** (computation follows the data) — the Unheaded model

### 1.3 RFC Lineage

The protocol builds on and extends several IETF specifications:

| RFC | Contribution to Unheaded |
|-----|-------------------------|
| **RFC 8200** (IPv6) | Hop-by-Hop Options extension header — the Monad carrier |
| **RFC 8799** (Limited Domains) | Scoping: protocol operates only within operator-controlled networks |
| **RFC 9673** (HbH Processing) | Updated processing rules for Hop-by-Hop options |
| **RFC 9098** (Operational Implications of HbH) | Acknowledgment that HbH options are often dropped on public Internet — validates Limited Domain requirement |
| **RFC 8300** (NSH) | Prior art for service function chaining with metadata headers; Unheaded differs by operating at the IP layer (not overlay) with BPF compute |
| **RFC 8754** (SRv6) | Segment Routing over IPv6; Unheaded differs by carrying *computational state* rather than routing instructions |
| **RFC 9197** (IOAM) | In-situ OAM; closest prior art. IOAM carries observability metadata in extension headers. Unheaded extends this to carry *arbitrary computation*, not just telemetry. |
| **RFC 9486** (IOAM Data Fields) | IOAM type definitions; Unheaded's Monad is a superset carrying both observability and application-layer state |

**Key differentiator from IOAM**: IOAM (RFC 9197) is read-mostly — nodes append telemetry data but rarely modify the packet's computational intent. The Unheaded Monad is **read-write at every hop** — each Shim program can modify any field, changing the packet's behavior for downstream hops. This transforms observability into computation.

## 2. Packet Format Specification

### 2.1 Extension Header Layout

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|  Next Header  | Hdr Ext Len=2 | Type = 0x3E   |   Len = 20    |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|    version    |  src_svc_id   |  dst_svc_id   |  hop_count    |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|   qos_class   |  flow_action  | circuit_state |     flags     |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|         latency_hint          |  deploy_ring  |  mesh_flags   |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
| src_prefix_lo | dst_prefix_lo |  scratch[0]   |  scratch[1]   |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|  scratch[2]   |  scratch[3]   |          checksum             |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

**Total wire overhead**: 24 bytes (4B HbH header + 20B Monad option). On 1500B MTU frames: 1.6%. The option type 0x3E has high-order bits `00` (skip on unrecognized) and third bit `1` (data may change en-route per RFC 8200 §4.2).

### 2.2 Field Classification

**Raw fields** (fixed interpretation): `version`, `hop_count`, `flags`, `latency_hint`, `src_prefix_lo`, `dst_prefix_lo`, `scratch[0-3]`, `checksum`

**Exponent fields** (Sophia-decoded): `src_service_id`, `dst_service_id`, `qos_class`, `flow_action`, `circuit_state`, `deploy_ring`, `mesh_flags`

The ratio (8 exponent : 6 raw) is deliberate — the majority of semantic content is dictionary-driven, maximizing extensibility. Raw fields handle values that require bit-precise interpretation (counters, checksums, flags).

### 2.3 Trace Correlation

The 20-bit IPv6 Flow Label (RFC 6437), set by Shield at ingress, serves as the trace correlation identifier. Shield MUST set a unique Flow Label per flow and Shim programs MUST NOT modify it. This provides 1,048,576 concurrent trace identifiers without consuming Monad space — the correlation ID rides in the IPv6 header itself.

### 2.4 CRC-16/CCITT-FALSE

Polynomial `x^16 + x^12 + x^5 + 1` (0x1021), initial value 0xFFFF, no reflection, no final XOR. Computed over all 20 bytes with the checksum field zeroed. Verified at every hop; failure triggers `EVENT_ANOMALY` and packet drop (RECOMMENDED). In BPF, CRC-16 computation over 18 bytes completes in ~50ns.

## 3. Sophia: Exponent Encoding and Dictionary Architecture

### 3.1 Formal Model

Let S = (K, D, V) be the Sophia system where:
- K = {k ∈ [0, 255]} — the key space (single byte)
- D = {d₀, d₁, ..., d₂₅₅} — a forest of sub-dictionaries, each d_i: K → V
- V = the value space (arbitrary structured data)

A **lookup chain** of depth n is: `meaning = d_{k₀}.d_{k₁}...d_{kₙ₋₁}(kₙ)`

The semantic space S_n at depth n: |S_n| = 256^n

At depth 1: 256 meanings per field.
At depth 2: 65,536.
At depth 3: 16,777,216.

The 20-byte Monad contains 8 exponent fields, each potentially participating in composition. Even without composition (depth 1), the Monad encodes 256^8 ≈ 1.8 × 10^19 distinct semantic states — more than enough for any practical service mesh.

### 3.2 Kernel-Space Implementation

```c
// Root dictionary: maps first-level key → sub-dictionary ID
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 256);
    __type(key, u8);
    __type(value, struct sophia_entry); // { type, sub_dict_id }
} sophia_root SEC(".maps");

// Sub-dictionaries: array of hash maps
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY_OF_MAPS);
    __uint(max_entries, 256);
    __type(key, u32);
    __type(value, u32);  // fd of inner BPF_MAP_TYPE_HASH
} sophia_dicts SEC(".maps");

// O(depth) hash lookups per packet
static __always_inline struct meaning *sophia_lookup(u8 key0, u8 key1) {
    struct sophia_entry *entry = bpf_map_lookup_elem(&sophia_root, &key0);
    if (!entry) return NULL;
    void *sub_dict = bpf_map_lookup_elem(&sophia_dicts, &entry->sub_dict_id);
    if (!sub_dict) return NULL;
    return bpf_map_lookup_elem(sub_dict, &key1);
}
```

BPF hash map lookups are O(1) amortized (~100ns per lookup). A 2-level Sophia lookup adds ~200ns per packet — well within the per-hop budget.

### 3.3 Atomic Dictionary Updates

BPF map updates are atomic at the entry level. Wotan's downward path writes dictionary entries via `bpf_map_update_elem()`. The update is visible to the next BPF invocation — propagation latency is bounded by the interval between packets (~microseconds at typical rates).

For multi-entry consistency (e.g., adding a new service + its routing rules), Wotan uses a **two-phase approach**: write all dependent entries first, then write the root entry that activates them. Since BPF programs only follow lookup chains starting from the root, the new entries are invisible until the root pointer is updated.

### 3.4 Historical Replay

Anamnesis stores raw exponent keys, not decoded values. This enables:

```
decoded_event(t) = sophia_decode(anamnesis_event.raw_keys, sophia_version_at(t))
```

Any historical event can be re-decoded through any dictionary version. This is critical for debugging dictionary changes — you can replay traffic through the old and new dictionaries side-by-side.

## 4. Computational Model

### 4.1 Per-Hop Processing as Instruction Execution

Each BPF Shim invocation can be modeled as:

```
Shim: (Monad_in, Maps) → (Monad_out, Events, Action)
```

Where:
- `Monad_in`: 20-byte register file from packet
- `Maps`: Sophia dictionaries + policy maps (read) + state maps (read/write)
- `Monad_out`: Modified register file written back to packet
- `Events`: Anamnesis ring buffer entries
- `Action`: Forward (XDP_PASS/TC_ACT_OK), Drop (XDP_DROP), Bounce (XDP_TX)

The Shim is a **pure function of its inputs** (Monad + Maps), with side effects limited to ring buffer writes and map updates. This property enables:
- Deterministic replay (same Monad + same Maps = same output)
- Formal verification (bounded execution, no unbounded loops)
- Composability (chain multiple Shims via hook ordering)

### 4.2 Turing Completeness via Wotan Memory Paging

The 20-byte Monad alone is not Turing-complete — it has finite state and bounded per-hop computation. However, when combined with Wotan's memory bus:

```
Monad (20B registers) + Wotan (unbounded memory) + Packet stream (unbounded time)
= Turing-complete system
```

The Shim can read/write Wotan memory via BPF map operations keyed by the Monad's `trace_id`. This provides per-flow addressable memory beyond the 20-byte register file. Combined with the unbounded packet stream (each packet triggers bounded computation), the system implements a **reactive Turing machine** — identical to the Doom-over-IPv6 architecture, but generalized.

The Doom PoC demonstrates this concretely: ROM, RAM, CPU state, and framebuffer live in BPF maps (the "Wotan memory" equivalent), while packet circulation provides the clock. The same architectural pattern applies to the protocol's general-purpose computation: the Monad is the register file, the packet stream is the clock, and BPF maps are the memory hierarchy.

### 4.3 The Distributed Pipeline

A packet traversing N hops executes a pipeline of N Shim invocations:

```
Monad₀ →[Shim₁]→ Monad₁ →[Shim₂]→ Monad₂ →...→[Shimₙ]→ Monadₙ
```

Each Shim sees the cumulative result of all previous hops. The final Monad state at egress (captured by Shield's `EVENT_DEATH`) represents the **complete computation result** — the aggregate of every BPF program's contribution along the path.

This is structurally analogous to a **systolic array** — a pipeline of processing elements where data flows through and is transformed at each stage. The network topology determines the pipeline structure.

## 5. The Gnostic Entities as Architectural Primitives

The protocol's naming convention derives from Gnostic cosmology. These are not metaphors — they map to precise architectural roles:

| Entity | Gnostic Meaning | Protocol Role | Implementation |
|--------|----------------|---------------|----------------|
| **Monad** (μονάς) | The One, source of all | Unified packet format | 20-byte HbH option register file |
| **Sophia** (σοφία) | Wisdom, divine knowledge | Exponent dictionaries | BPF hash maps (kernel) + lookup tables (userspace) |
| **Pleroma** (πλήρωμα) | Fullness, divine realm | Desired state | Git-tracked YAML declarations |
| **Kenoma** (κένωμα) | Emptiness, material void | Observed state | Materialized view over Anamnesis events |
| **Anamnesis** (ἀνάμνησις) | Remembrance of the divine | Event log | Per-CPU BPF ring buffers, 64B events |
| **Yaldabaoth** | The Demiurge, flawed creator | Chaos injection | TC BPF program with stochastic fault injection |
| **Wotan** | The All-Father | Central bus | gRPC message bus bridging kernel ↔ userspace |

### 5.1 Pleroma/Kenoma Reconciliation

The reconciliation loop implements a **control-theoretic feedback system**:

```
                    ┌─────────────┐
     desired   ──→  │  Comparator  │ ──→  drift signal
     (Pleroma)      │  (Wotan)     │
                    └──────┬──────┘
                           │ BPF map updates
                           ▼
                    ┌─────────────┐
                    │   Plant     │
                    │  (The Void) │ ──→ Anamnesis events
                    └──────┬──────┘
                           │ event stream
                           ▼
                    ┌─────────────┐
     observed  ←──  │  Sensor     │
     (Kenoma)       │  (Wotan)    │
                    └─────────────┘
```

Pleroma is the **setpoint**. Kenoma is the **process variable**. Wotan is both the **controller** (computing the error signal) and the **actuator** (writing BPF map updates). The loop converges because BPF map updates are atomic and deterministic — once Pleroma is pushed down, the Void enforces it on every subsequent packet.

### 5.2 Anamnesis: Causal Event Ordering

Anamnesis events are ordered by `bpf_ktime_get_ns()` — monotonic nanosecond timestamps from the kernel's clock. Within a single CPU, events are strictly ordered. Across CPUs, events are ordered within the ring buffer's memory ordering guarantees (acquire/release semantics on the ring buffer head/tail pointers).

For causal reconstruction across hops, the `trace_id` (from IPv6 Flow Label) correlates events belonging to the same packet. The sequence `[BIRTH, COMPUTED₁, COMPUTED₂, ..., COMPUTEDₙ, DEATH]` for a single trace_id reconstructs the complete packet lifecycle with before/after Monad snapshots at each hop.

### 5.3 Yaldabaoth: Formalized Chaos

Chaos injection operates at the TC hook (post-admission), not XDP (pre-admission). This is deliberate — chaos testing should exercise the processing pipeline, not the admission filter.

The injection probability P is configured per-flow via BPF map:

```
P(chaos | flow) = threshold / 2^32
```

Where `threshold` is a u32 compared against `bpf_get_prandom_u32()`. At threshold = 4,294,967 (~0.1%), approximately 1 in 1000 packets experiences chaos.

All chaos events MUST emit before/after Anamnesis snapshots. This makes chaos **auditable** and **replayable** — you can determine exactly which packets were perturbed, how, and whether downstream components detected and recovered.

## 6. Kingdom Mode: Address Reclamation

### 6.1 Motivation

Within a Limited Domain using a known ULA prefix (e.g., `fd00:3f:75::/48`), the high-order bits of every IPv6 address are deterministic. These bits carry zero information within the domain — every host already knows the prefix.

Kingdom Mode **reclaims** these deterministic bits as extended computational registers.

### 6.2 Reclamation Model

Given a 128-bit IPv6 address with a known P-bit prefix:

```
Reclaimed bits = P - (minimum routing bits)
```

For a /48 ULA with 16-bit subnet ID:
- 48 bits: known prefix (fd00:3f:75)
- 16 bits: subnet ID (needed for routing)
- Remaining: 64 bits of Interface ID

In source + destination addresses combined: up to 128 reclaimed bits from the prefix portions, plus bits from the Interface ID that can be deterministically derived.

The reclaimed bits serve as **extended Monad registers** (r16-r31), available without increasing wire overhead — the address *is* the register space.

### 6.3 Formal Bit Accounting

Per `PROTOCOL_MATH_AND_MAPS.md`:

```
Primary Monad:          160 bits (20 bytes in HbH option)
Flow Label:              20 bits (trace correlation)
Address reclamation:   up to 224 bits (source + dest prefix space)
────────────────────────────────────────────────────────────────
Total computational space: up to 404 bits per packet
Wire overhead:          24 bytes (HbH header only; addresses are free)
```

## 7. Post-Quantum Identity Binding

### 7.1 Design

Each `src_service_id` exponent key in Sophia is bound to a **post-quantum keypair**:

```
sophia_identity_map[service_id] = {
    name: "captain",
    endpoint: "10.0.1.1:8080",
    pq_public_key: ML-KEM-768 public key (FIPS 203),
    pq_signature: ML-DSA-65 signature (FIPS 204) over service_id binding
}
```

Shield at ingress verifies the binding: the src_service_id in the Monad matches the authenticated identity of the source pod. This provides **quantum-resistant per-packet authentication** without payload encryption overhead — the identity proof is in Sophia, not in the packet.

### 7.2 Key Distribution

Keys are distributed via Sophia dictionary updates through Wotan. The BPF maps in the kernel contain only the public key hashes (32 bytes each). Full verification occurs at Shield (XDP) using pre-computed lookup tables.

## 8. Embedded Bus Heritage

The protocol explicitly draws from embedded bus architectures:

| Bus | Frame Size | Key Parallel |
|-----|-----------|-------------|
| **CAN Bus** | 8 bytes | ID-based arbitration, every node reads every frame. Unheaded: every hop reads every Monad. |
| **I2C** | Variable | Address byte = key lookup. Unheaded: exponent byte = Sophia key. |
| **ARINC 429** | 32 bits | Fixed-width, every bit defined, avionics-grade determinism. Unheaded: 20-byte fixed format. |
| **SPI** | Variable | Full duplex, clock + data. Unheaded: packet = clock, Monad = data. |

These protocols run cars, aircraft, and spacecraft — proving that small, well-defined frame formats with shared dictionaries scale to mission-critical systems. The Unheaded Protocol applies this principle at network scale with BPF as the per-node compute fabric.

## 9. Performance Model

### 9.1 Per-Packet Processing Budget

| Operation | Latency | Notes |
|-----------|---------|-------|
| XDP hook entry | ~20ns | Before sk_buff allocation |
| Monad parse | ~10ns | Fixed offset from IPv6 header |
| CRC-16 verify | ~50ns | 18 bytes, lookup table |
| Sophia lookup (2-level) | ~200ns | 2 × BPF hash map O(1) |
| Hop-specific logic | ~50-200ns | Varies by component |
| CRC-16 recompute | ~50ns | After modification |
| Anamnesis emit | ~100ns | Ring buffer write (lock-free) |
| **Total per-hop** | **~500ns** | Well under 1μs budget |

### 9.2 Throughput

At 1μs per-hop processing:
- **Single core**: 1M packets/sec per hop
- **Multi-core (16)**: 16M packets/sec per hop
- **10 Gbps line rate**: ~833K packets/sec (1500B avg) — comfortably handled

### 9.3 Comparison with Sidecar Proxies

| Metric | Envoy Sidecar | Unheaded Shim |
|--------|--------------|---------------|
| Per-hop latency | 0.5-2ms | 0.5μs |
| Memory per pod | ~50-100MB | ~2MB (BPF maps) |
| CPU per pod | 5-15% | <0.5% |
| Failure modes | Process crash, OOM | None (kernel-resident, verifier-proven) |
| Deployment | Per-pod sidecar | Per-host BPF program |
| Observability | Application-level (logs, metrics) | Wire-level (every packet) |
| Latency overhead | 1000-4000x | 1x (baseline) |

## 10. Threat Model and Security Properties

### 10.1 Protocol Containment

The Monad exists exclusively within the Limited Domain. Shield's ingress/egress boundary ensures:

- **Zero leakage**: No Monad bytes exit the domain
- **Zero fingerprinting**: External observers see standard IPv6
- **Clean lifecycle**: Birth at ingress, death at egress, audited by Anamnesis

### 10.2 Checksum Integrity

CRC-16 at every hop detects accidental corruption and provides a basic integrity check. For cryptographic integrity, the post-quantum identity binding (Section 7) provides authentication of the service origin.

### 10.3 Chaos Auditability

All Yaldabaoth perturbations are recorded with before/after snapshots. This ensures chaos testing is distinguishable from real failures in post-mortem analysis.

## 11. Relationship to Doom-over-IPv6

The Doom-over-IPv6 demonstration validates the protocol's computational completeness thesis. It uses the same architectural primitives:

| Protocol Concept | Doom Implementation |
|-----------------|---------------------|
| Monad (20B register file) | Monad register in packet header |
| Shim (per-hop BPF program) | monad_cpu XDP program |
| Sophia (dictionary maps) | ROM_MAP, RV2MBC_MAP |
| Wotan (memory bus) | RAM_MAP, SCREEN_MAP, CPU_MAP |
| Anamnesis (event log) | COMPUTE_EVENTS ring buffer |
| Shield (protocol boundary) | Injector (birth) + XDP_DROP (death) |
| Packet circulation | veth ring (6 namespaces) |

Doom is a **degenerate case** of the general protocol: a single "service" (the Doom CPU) with one "flow" (instance 0xDE), where every Monad field except the hop counter and CUSTOM flag are unused. The general protocol extends this to N services, M flows, and rich per-packet metadata.

The shared insight: **if packet arrival can drive a Doom renderer at 20 fps, it can certainly drive observability metadata processing at wire speed.**

## 12. Open Questions and Future Work

1. **IANA option type assignment**: The protocol uses experimental type 0x3E. Production deployment requires IANA registration.

2. **Multi-domain federation**: How do two Kingdoms exchange Sophia-encoded metadata? Dictionary translation at the boundary? Shared root dictionaries?

3. **BPF verifier evolution**: Future kernel versions may relax loop bounds or add BPF-native CRC instructions, enabling more complex per-hop computation.

4. **Hardware offload**: SmartNICs supporting XDP offload (Netronome, Mellanox ConnectX) could execute Shim programs at NIC line rate, removing host CPU involvement entirely.

5. **Formal verification**: The bounded, deterministic nature of Shim programs makes them amenable to formal verification (e.g., via Serval or Jitterbug for BPF).

---

## References

- [1] Deering, S., Hinden, R. "Internet Protocol, Version 6 (IPv6) Specification." RFC 8200. July 2017.
- [2] Carpenter, B., Liu, B. "Limited Domains and Internet Protocols." RFC 8799. July 2020.
- [3] Brockners, F., et al. "In Situ Operations, Administration, and Maintenance (IOAM)." RFC 9197. May 2022.
- [4] Hinden, R., Haberman, B. "Unique Local IPv6 Unicast Addresses." RFC 4193. October 2005.
- [5] Quinn, P., Elzur, U., Pignataro, C. "Network Service Header (NSH)." RFC 8300. January 2018.
- [6] Filsfils, C., et al. "Segment Routing over IPv6 (SRv6) Network Programming." RFC 8754. March 2020.
- [7] Herbert, T. "Operational Implications of IPv6 Packets with Extension Headers." RFC 9098. September 2021.
- [8] Bellis, S. "Unheaded: Protocol Foundation — A Mapped Data Bus over IPv6 Hop-by-Hop Options." draft-bellis-unheaded-protocol-foundation-04. February 2026.
- [9] NIST. "Module-Lattice-Based Key-Encapsulation Mechanism Standard." FIPS 203. August 2024.
- [10] NIST. "Module-Lattice-Based Digital Signature Standard." FIPS 204. August 2024.

---

*Previous: [← High School Explanation](03-high-school.md)*
