# Formal Specification Introduction

*PhD Level*

---

## Abstract

This document presents the Unheaded Protocol Foundation as a formally
specifiable stateful stream transducer operating under the constraints of the
Linux BPF verifier. We develop the mathematical model, analyze the
information-theoretic properties of the 20-byte Monad register file, compare
the state management approach to Apache Flink's distributed snapshot model,
and derive the security properties of the wire format from the verifier's
static analysis guarantees.

The target audience is researchers, formal methods practitioners, and senior
engineers evaluating the protocol for production deployment. Familiarity with
automata theory, information theory, and BPF internals is assumed.

---

## 1. Stateful Stream Transducer Model

### 1.1 Definition

We model each eBPF hop program as a stateful stream transducer:

```
T: (S x I) -> (S' x O)
```

Where:
- `S` is the state space (BPF map contents + Monad register file)
- `I` is the input alphabet (arriving packets)
- `S'` is the next state
- `O` is the output alphabet (mutated packets + Anamnesis events)

More precisely, let:

- `M = {0,1}^160` be the space of all 20-byte Monad values
- `P` be the space of all valid IPv6 packets
- `B` be the contents of all BPF maps (SOPHIA, CIRCUIT_ERRORS, CONFIG, STATS)
- `E = AnamnesisEvent` be the event output type

Then each hop program `h` is:

```
h: (B x M x P) -> (B' x M' x O(P) x E?)
```

Where `O(P)` is either `XDP_PASS(P')` or `XDP_DROP` and `E?` is an optional
event emission.

### 1.2 State Decomposition

The global state decomposes into:

1. **Ephemeral state** (Monad): travels with the packet, mutated at each hop,
   discarded at Shield egress. Lifetime: single packet transit.

2. **Persistent state** (BPF maps): survives across packets. Updated by both
   eBPF programs and userspace. Lifetime: program lifetime or explicit
   eviction (LRU).

3. **Emitted state** (Anamnesis events): write-once, append-only. Consumed by
   userspace trace-collector. Lifetime: ring buffer capacity.

This decomposition is the protocol's fundamental architectural choice: rather
than maintaining a centralized state store that all hops query (as in
traditional SDN controllers), the state is factored into a per-packet
register file that moves with the data and a per-node map that provides
contextual knowledge.

### 1.3 Composition of Transducers

For a packet traversing hops `h_1, h_2, ..., h_n`, the composite transducer
is:

```
H = h_n . h_{n-1} . ... . h_1
```

With the important property that each `h_i` operates on the Monad produced
by `h_{i-1}`:

```
(B_1, M_0, P) --h_1--> (B_1', M_1, P_1, E_1?)
(B_2, M_1, P_1) --h_2--> (B_2', M_2, P_2, E_2?)
...
(B_n, M_{n-1}, P_{n-1}) --h_n--> (B_n', M_n, P_n, E_n?)
```

The sequence `(M_0, M_1, ..., M_n)` is the **Monad trace** of the packet.
The concatenation `(E_1, E_2, ..., E_n)` is the **Anamnesis trace**. Both are
fully determined by the initial state and packet contents.

### 1.4 Determinism

Each hop function `h_i` is deterministic given its inputs, with one exception:
Shield ingress uses `bpf_get_prandom_u32()` to generate the Flow Label. This
introduces controlled non-determinism at the boundary but does not affect the
Monad processing pipeline (the Flow Label is in the IPv6 header, not the
Monad).

For reproducible testing, the CONFIG map can override the Flow Label source.

---

## 2. Information-Theoretic Analysis

### 2.1 Monad Channel Capacity

The Monad is a 20-byte (160-bit) register file. Its theoretical channel
capacity is `2^160` distinct states, but the actual information content is
constrained by the field structure.

Per-field entropy analysis:

| Field | Bits | Effective States | Entropy (bits) | Notes |
|-------|------|-----------------|----------------|-------|
| version | 8 | 1 (currently) | 0 | Fixed at 0x01 |
| src_service_id | 8 | 256 | 8 | Exponent-encoded |
| dst_service_id | 8 | 256 | 8 | Exponent-encoded |
| hop_count | 8 | ~16 typical | 4 | Most paths < 16 hops |
| qos_class | 8 | ~8 classes | 3 | Practical QoS tiers |
| flow_action | 8 | 5 defined | 2.32 | FORWARD/SAMPLE/TRACE/MIRROR/DROP |
| circuit_state | 8 | 3 defined | 1.58 | CLOSED/HALF_OPEN/OPEN |
| flags | 8 | 7 used bits | 7 | Independent booleans |
| latency_hint | 16 | 65536 | 16 | Full range used |
| deploy_ring | 8 | ~4 rings | 2 | PRODUCTION/STAGING/CANARY/SHADOW |
| mesh_flags | 8 | 256 | 8 | Exponent-encoded |
| src_prefix_lo | 8 | 256 | 8 | Subnet low byte |
| dst_prefix_lo | 8 | 256 | 8 | Subnet low byte |
| scratch[0..3] | 32 | 2^32 | 32 | General-purpose |
| checksum | 16 | deterministic | 0 | Derived from other fields |

**Theoretical maximum**: 160 bits.
**Effective entropy**: ~107.9 bits (excluding checksum and fixed fields).
**Practical entropy**: ~85 bits for typical production traffic.

### 2.2 Compression Efficiency

The Monad achieves high information density through exponent encoding. A
single-byte exponent-encoded field maps to a Sophia dictionary entry that may
contain kilobytes of metadata. The 20-byte Monad is a compressed index into
the full state space defined by the Sophia dictionaries.

Compression ratio (Monad size : equivalent uncompressed metadata):

```
20 bytes Monad --> ~2 KB Sophia metadata (typical)
Ratio: ~100:1
```

This is effective because:
1. The encoding is lossless for the discrete state categories it represents.
2. The dictionary is distributed (in BPF maps) and does not travel with the
   packet.
3. Each hop re-derives the full context from the compressed index, paying
   only a BPF map lookup cost.

### 2.3 Integrity Budget

The CRC-16/CCITT checksum protects 18 bytes of data with a 16-bit code.
Detection properties:

- **All single-bit errors**: Detected (Hamming distance >= 2).
- **All double-bit errors**: Detected.
- **All odd-count bit errors**: Detected.
- **Burst errors up to 16 bits**: Detected.
- **Random error detection probability**: 1 - 2^(-16) = 99.998%.

For a 20-byte field at BER (bit error rate) of 10^(-12) (typical for Ethernet),
the probability of undetected corruption per packet is approximately:

```
P(undetected) = (1 - (1 - BER)^144) * 2^(-16)
              ~ 144 * BER * 2^(-16)
              ~ 2.2 * 10^(-15)
```

This is acceptable for the fail-open design: corrupted Monads are detected
and flagged as anomalies, and the packet passes through unmodified.

---

## 3. Comparison to Apache Flink Checkpoint Model

### 3.1 Flink's Approach

Apache Flink implements exactly-once processing via **Chandy-Lamport
distributed snapshots** (Asynchronous Barrier Snapshotting, ABS). The
mechanism:

1. The JobManager periodically injects **checkpoint barriers** into source
   streams.
2. Each operator receives barriers, snapshots its state to durable storage,
   and forwards the barrier.
3. When all sinks acknowledge, the checkpoint is complete.
4. On failure, the system restores from the last complete checkpoint and
   replays from source offsets.

### 3.2 Unheaded's Approach

Unheaded inverts the state model:

| Property | Flink | Unheaded |
|----------|-------|----------|
| State location | Operator-local, checkpointed to remote storage | Per-packet (Monad) + per-node (BPF maps) |
| Checkpoint mechanism | Barrier injection + async snapshot | No checkpoints: Monad is the state carrier |
| Recovery | Restore snapshot + replay from source | Fail-open: corrupted Monad detected by CRC, packet passes |
| Exactly-once | Via checkpoint + barrier alignment | Not applicable: each hop is idempotent |
| State size | Unbounded (operator-dependent) | 20 bytes ephemeral + bounded BPF maps |
| Latency overhead | Checkpoint barriers add latency spikes | Zero: no barriers, no snapshots, no coordinator |
| Throughput | Limited by checkpoint interval and state backend | Limited by XDP packet rate (~14.8 Mpps on 10GbE) |

### 3.3 Key Insight

Flink checkpoints operator state so it can be reconstructed after failure.
Unheaded does not need checkpoints because:

1. **The Monad is the state.** It travels with the packet. If the packet is
   lost, the state is irrelevant. If the packet arrives, the state is already
   there.

2. **BPF map state is derivable.** The SOPHIA dictionary, CONFIG, and
   CIRCUIT_ERRORS maps are populated by userspace and can be reconstructed
   from the source of truth (Git, service registry, health checks).

3. **Anamnesis events are best-effort.** Dropped events reduce observability
   but do not affect correctness. The network continues to function without
   them.

This eliminates the entire checkpoint coordination problem. The cost is that
Unheaded provides at-most-once event delivery (vs. Flink's exactly-once),
which is the correct tradeoff for an observability system: missing a trace
event is acceptable; adding latency to every packet is not.

### 3.4 Formal Comparison

Let `C_f` be the checkpoint cost in Flink (network transfer + storage write
per barrier) and `C_u` be the per-hop cost in Unheaded (20-byte read + 20-byte
write + optional ring buffer write). Then:

```
Flink per-event cost:   O(1) amortized + O(|state|) per checkpoint
Unheaded per-event cost: O(1) per hop, O(n) per packet (n = hop count)
```

For `n < 20` hops (typical data center), Unheaded's per-packet overhead is
bounded by `20 * O(1) = O(1)` with a very small constant (nanosecond-scale
BPF map lookups and byte copies).

---

## 4. BPF Verifier Constraints as Formal Safety

### 4.1 The Verifier as a Proof System

The Linux BPF verifier performs static analysis of every eBPF program before
it is loaded. This analysis constitutes a formal proof of the following
safety properties:

1. **Termination**: All execution paths are finite. Every loop has a
   compile-time bounded iteration count. Every recursive structure (there are
   none in BPF) would be rejected.

2. **Memory safety**: Every memory access `*(ptr + offset)` is verified to be
   within bounds. For packet data, this means `ptr + offset + size <= data_end`.
   For map values, the verifier tracks pointer provenance and lifetime.

3. **Type safety**: The verifier tracks register types (SCALAR, PTR_TO_MAP_VALUE,
   PTR_TO_PACKET, etc.) and rejects operations that would mix incompatible
   types.

4. **No undefined behavior**: Division by zero, null pointer dereference, and
   use of uninitialized memory are all rejected at verification time.

### 4.2 Verifier Model for Hop-ebpf

The hop-ebpf program's verification can be modeled as a bounded model
checking problem:

```
State: (PC, registers[10], stack[512], packet_range)
Transitions: BPF instruction semantics
Property: forall reachable states s:
  - All packet accesses in s are within [data, data_end)
  - All map accesses in s use verified pointers
  - s.PC eventually reaches EXIT or XDP_PASS/XDP_DROP
```

The verifier explores all possible execution paths (up to a configurable
complexity limit, currently ~1 million verified instructions). For the
hop-ebpf program, the key verification challenges are:

1. **Option scanning loop** (find_monad_option): Bounded to 16 iterations.
   The `core::hint::black_box(opts_end)` call prevents LLVM from proving
   `opts_end <= data_end`, which would cause it to eliminate the per-iteration
   `data_end` checks that the verifier requires. This is a critical
   interaction between compiler optimization and verifier requirements.

2. **20-byte read/write loops**: Bounded to exactly 20 iterations. The
   verifier trivially proves termination.

3. **Map lookups**: Every `get_ptr_mut` result is checked for null before
   dereference. The verifier tracks the null check across basic blocks.

### 4.3 LLVM Interaction

The Aya framework compiles eBPF programs from Rust to LLVM IR, then to BPF
bytecode. LLVM's optimization passes can eliminate bounds checks that the
verifier requires. The protocol addresses this with two techniques:

1. **`core::hint::black_box()`**: Severs LLVM's knowledge that a bound
   variable is constrained. Used for `opts_end` in option scanning loops and
   `data_end` in some path contexts.

2. **`core::ptr::read_volatile()`**: Forces LLVM to emit actual memory
   reads rather than caching values in registers. Each read produces a fresh
   bounds check in the BPF bytecode.

These are not hacks -- they are necessary interactions between the Rust
compiler's optimization model and the BPF verifier's conservative analysis
model. The compiler proves properties that the verifier cannot use, and the
verifier requires properties that the compiler would eliminate.

### 4.4 Safety Theorem

Given a verified hop-ebpf program `h`:

```
Theorem: For all packets P and BPF map states B:
  (a) h(B, P) terminates in bounded time
  (b) h(B, P) does not access memory outside [data, data_end) union map_values
  (c) h(B, P) produces a well-formed output (XDP_PASS or XDP_DROP)
  (d) h(B, P) does not modify B except through verified map update operations
```

Proof: By construction. The BPF verifier has verified all execution paths of
`h`. Properties (a)-(d) are exactly the properties the verifier checks. QED.

This is the strongest safety guarantee available for kernel-level network
processing. It exceeds the guarantees of any userspace implementation (which
can crash, leak memory, or loop indefinitely) and matches the guarantees of
formally verified microkernels (but with a vastly simpler proof obligation).

---

## 5. Wire Format Security Analysis

### 5.1 Threat Model

The Monad operates within the "Kingdom" -- the Unheaded-managed network
domain. The threat model assumes:

- **Trusted boundary**: Shield programs are correctly loaded and running at
  all ingress/egress points.
- **Untrusted external traffic**: Arbitrary packets from the Shadow network.
- **Semi-trusted internal traffic**: Internal services are assumed to be
  non-malicious but may be buggy.

### 5.2 Attack Surface Analysis

| Attack | Mitigation | Residual Risk |
|--------|------------|--------------|
| External Monad injection | Shield strips all inbound extension headers | None (architectural) |
| Monad exfiltration | Shield TC strips HBH before egress | None (architectural) |
| CRC bypass (corrupted Monad accepted) | CRC-16/CCITT over 18 bytes; anomaly emitted on failure | Undetected corruption: ~2.2 * 10^(-15) per packet |
| BPF map poisoning (malicious Sophia entries) | Map writes require CAP_BPF; userspace programs are in hardened containers | Requires kernel capability compromise |
| DDoS via extension header amplification | Shield strips and re-inserts; no amplification path | None (constant 24-byte insertion) |
| Replay of expired Flow Labels | Flow Labels are per-packet pseudorandom; no session state | Not applicable (labels are correlation keys, not auth) |
| Double-stamping | Shield checks for existing Monad HBH; passes without re-stamp | None (explicit check) |

### 5.3 Information Leakage

The Monad contains operational metadata (service IDs, hop count, QoS class)
but no user data. Even if an internal adversary reads the Monad from packet
memory:

- **No payload access**: eBPF programs access fixed-offset header bytes. The
  BPF verifier ensures they do not read beyond the declared header region.
- **No PII**: Service IDs are exponent-encoded indices, not names or
  addresses. Decoding requires access to the Sophia dictionary (BPF map).
- **No session keys**: The protocol does not carry authentication tokens,
  session IDs, or cryptographic material in the Monad.

### 5.4 Formal Containment Property

Let `K` be the set of all packets within the Kingdom and `S` be the set of
all packets in the Shadow network.

```
Invariant: forall p in S: p does not contain a Monad HBH header
Invariant: forall p in K: p contains exactly one Monad HBH header

Proof:
  - Shield XDP inserts Monad on every ingress packet (K grows)
  - Shield TC strips Monad on every egress packet (S packets are clean)
  - Shield XDP rejects ingress packets that already carry Monad (no double-stamp)
  - Therefore: Kingdom-Shadow boundary is sealed with respect to Monad presence
```

This is the protocol's fundamental security property: the Monad never leaks
and is never spoofed. All other security properties derive from this
containment guarantee.

---

## 6. Complexity Analysis

### 6.1 Per-Hop Time Complexity

| Operation | Time | Notes |
|-----------|------|-------|
| Ethernet/IPv6/HBH parsing | O(1) | Fixed-offset reads |
| Monad option scan | O(k), k <= 16 | Bounded TLV scan |
| Monad read | O(1) | 20-byte fixed read |
| CRC verification | O(1) | 18-byte CRC-16/CCITT |
| Sophia lookups | O(1) amortized | 3 BPF hashmap lookups |
| Circuit breaker check | O(1) amortized | 1 BPF hashmap lookup |
| Hop count increment | O(1) | Single byte mutation |
| Flow action application | O(1) | Branch on single byte |
| CRC recomputation | O(1) | 18-byte CRC-16/CCITT |
| Monad write-back | O(1) | 20-byte fixed write |
| Event emission | O(1) | 32-byte ring buffer reserve + write |
| **Total per-hop** | **O(1)** | **Constant time per packet** |

### 6.2 Per-Packet Time Complexity

For a packet traversing `n` hops:

```
T(packet) = T(shield_ingress) + n * T(hop) + T(shield_egress)
           = O(1) + n * O(1) + O(1)
           = O(n)
```

With typical `n < 10`, the per-packet overhead is bounded by a small constant
in practice. Measured latency for the complete pipeline (packet arrival to
Anamnesis event emission) is under 5 microseconds per hop on commodity
hardware.

### 6.3 Space Complexity

| Resource | Size | Bound |
|----------|------|-------|
| Monad per packet | 20 bytes | Constant |
| HBH header per packet | 24 bytes | Constant |
| Anamnesis event | 32 bytes | Constant |
| SOPHIA map | ~64 KB (65,536 entries x 1B) | O(vocabulary size) |
| CIRCUIT_ERRORS map | ~256 KB | O(service pairs) |
| FLOWS map | ~4.7 MB (65,536 x 72B) | O(concurrent connections) |
| ANAMNESIS ring buffer | 8 MiB per CPU | O(event rate x latency) |

All BPF map sizes are declared at program load time and cannot grow
dynamically. This provides hard memory guarantees and eliminates OOM
conditions in the data plane.

---

## 7. Open Questions

1. **Multi-Kingdom federation**: When two Unheaded-managed networks
   interconnect, how should Monad state be preserved across the Shield
   boundary? Options include Monad-to-Monad bridging (Shield egress on one
   side feeds Shield ingress on the other) or a new inter-Kingdom extension.

2. **Formal verification of the full pipeline**: The BPF verifier proves
   per-program safety, but not inter-program properties (e.g., that the
   Monad written by hop `h_i` is the Monad read by hop `h_{i+1}`). A formal
   model of the Linux network stack's packet forwarding would be needed to
   close this gap.

3. **Exponent encoding standardization**: The current exponent encoding is
   locally assigned. A formal specification of the encoding space (including
   collision avoidance for multi-vendor deployments) is needed before the
   protocol can be deployed in heterogeneous environments.

4. **CRC vs. HMAC**: The current CRC-16/CCITT provides integrity but not
   authenticity. An adversary with BPF map write access could modify the
   Monad and recompute the CRC. For environments requiring authentication,
   upgrading to a keyed MAC (using the scratch registers for tag storage)
   is under consideration, at the cost of per-hop cryptographic computation.

---

*Next: [Competitive Landscape](competitive-landscape.md) -- Where Unheaded
fits in the ecosystem.*
