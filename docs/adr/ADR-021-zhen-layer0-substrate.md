# ADR-XXXX: Zhen — Layer 0 Anti-Fragile Knowledge Substrate

**Status:** ACCEPTED — 133 tests passing (2026-04-03). PQ crypto Phase 1 complete, gossip core operational.  
**Date:** 2026-03-28  
**Author:** Muck (Dworkin)  
**Deciders:** Captain, Architect, Micromanager, Scientist  
**Skills Consulted:** Architect, Scientist, Lore, Busboy, Developer

---

## Context

The Unheaded Protocol stack (Monad → Sophia → Wotan → Shim/UPC) operates on
explicitly indexed, hierarchically structured data. Every component has a
registry, an address, a lookup path. This mirrors DNS, databases, libraries —
systems that are powerful but brittle. A single root compromise, policy change,
or physical destruction can erase entire knowledge domains.

The Library of Alexandria burned because it was one building. DNS gets seized
because it has a root. etcd loses quorum because it requires consensus.

The Kingdom needs a substrate that **cannot be burned, cannot be seized, and
cannot lose quorum** — because it has no building, no root, and no consensus
requirement.

## Decision

Introduce **Zhen (真)** as Layer 0 — the knowledge substrate beneath Monad.

Zhen is an anti-fragile, index-free, gossip-propagated associative memory
system. It stores content-addressed knowledge fragments that propagate via
existing Monad traffic, surface through contextual relevance rather than
explicit query, and never die — only migrate to deeper strata.

### Mythological Foundation: Pillar 4 — The Dao

Zhen opens a fourth mythological pillar grounding the entire Kingdom:

| Concept   | Chinese | Role in Zhen                                    |
|-----------|---------|--------------------------------------------------|
| Zhen (真) | Truth   | Layer 0 itself. Original nature before naming.   |
| Pu (朴)   | Uncarved Block | Fragment format. Self-describing, uncommitted. |
| Qi (氣)   | Vital Breath | Gossip propagation. Rides existing traffic.   |
| Wu Wei (無為) | Non-Action | No-index design philosophy.              |
| De (德)   | Virtue/Power | Relevance gravity. Context attracts.        |
| Jing (經) | Classics | Deep strata. Ancient knowledge.                 |
| Li (理)   | Pattern  | Emergent structure from traversal.               |

The Dao predates Gnostic cosmology. Layer 0 predates the protocol. The pillar
ordering reflects this: Dao (substrate) → Gnostic (state) → Amber (protocol) →
Medieval (infrastructure).

## Architecture

### What Zhen Is

- **Content-addressed fragment store** — BLAKE3 hash as identity, no external index
- **Gossip-propagated** — fragments ride Qi (piggybacking Monad transport), never generate own packets
- **Associative recall** — context-triggered surfacing via embedding similarity (De), not query
- **Tiered by recency** — L1 hot / L2 warm / L3 cold (mirrors Wotan hierarchy)
- **Immortal fragments** — no TTL, no deletion. Cold migration only. Deeper strata = older knowledge
- **Geological learning** — networks accumulate wisdom over time. Young = shallow. Old = wise.

### What Zhen Is NOT

- Not a database (no schema, no tables, no SQL)
- Not a DHT (no guaranteed lookup, no consistent hashing ring)
- Not a search engine (no inverted index, no ranking algorithm)
- Not a blockchain (no consensus, no chain, no mining)
- Not etcd (no Raft, no leader election, no strong consistency)
- Not Ollama (no full LLM inference, embeddings only)

### Implementation: `zhend` — The Zhen Daemon (Rust)

A Rust binary combining ideas from Ollama (local model inference) and etcd
(distributed state) while being neither:

```
zhend
├── pu/          # Fragment format — content-addressed, self-describing
│   ├── fragment.rs    # Pu struct: blake3_hash + embedding_vec + payload + metadata
│   ├── codec.rs       # Encode/decode (serde + bincode, zero-copy where possible)
│   └── store.rs       # Tiered storage: L1 (mmap hot) / L2 (sled warm) / L3 (cold files)
├── qi/          # Gossip propagation
│   ├── gossip.rs      # SWIM-variant protocol over existing Monad transport
│   ├── peer.rs        # Peer discovery + membership
│   └── transport.rs   # IPv6 HbH piggyback or fallback UDP
├── de/          # Relevance engine
│   ├── embedder.rs    # Local embedding model (ONNX Runtime, quantized)
│   ├── similarity.rs  # Cosine similarity / ANN (HNSW via usearch or instant-distance)
│   └── context.rs     # Current-flow context vector construction
├── li/          # Emergent structure observation
│   ├── topology.rs    # Traffic pattern analysis → knowledge geography
│   └── strata.rs      # Geological layer management (hot→warm→cold migration)
├── jing/        # Deep archive
│   ├── archive.rs     # Cold storage format (append-only log, mmap)
│   └── pilgrimage.rs  # Deep retrieval — slow, intentional, traversal-based
├── api/         # gRPC + HTTP/3 interface
│   ├── grpc.rs        # tonic gRPC service definitions
│   └── quic.rs        # QUIC/HTTP3 for external access (quinn)
├── monad/       # Protocol integration
│   ├── hbh.rs         # IPv6 Hop-by-Hop extension header parsing
│   └── bridge.rs      # Monad ↔ Zhen fragment correlation
└── main.rs      # Daemon entrypoint, signal handling, graceful shutdown
```

### Key Dependencies (Rust)

| Crate            | Purpose                              |
|------------------|--------------------------------------|
| `blake3`         | Content addressing (fragment identity) |
| `sled`           | Embedded KV for L2 warm storage      |
| `ort` (ONNX Runtime) | Local embedding inference         |
| `usearch` or `instant-distance` | ANN similarity search |
| `tonic`          | gRPC server/client                   |
| `quinn`          | QUIC/HTTP3 transport                 |
| `serde` + `bincode` | Zero-copy serialization           |
| `memmap2`        | mmap for L1 hot and L3 cold          |
| `tracing`        | Structured logging (integrates w/ Anamnesis) |
| `tokio`          | Async runtime                        |
| `pqcrypto-kyber` | ML-KEM key encapsulation (PQ)        |
| `pqcrypto-dilithium` | ML-DSA signatures (PQ)           |
| `x25519-dalek`   | Classical ECDH (hybrid KEM partner)  |
| `aes-gcm`        | AES-256-GCM symmetric encryption     |
| `hkdf` + `sha2`  | Key derivation for hybrid KEM        |

### Fragment Lifecycle (The Dao De Jing of Data)

```
1. BIRTH (Pu)
   Raw fragment enters via API or Monad piggyback.
   BLAKE3 hash computed. Embedding vector generated via local model.
   Stored in L1 hot tier. No index entry created.

2. BREATH (Qi)
   Gossip daemon selects random fragments weighted by recency.
   Fragments piggyback on outgoing Monad packets as HbH option.
   Peers receive, check BLAKE3 (dedup), store if novel.

3. ATTRACTION (De)
   When a Monad packet flow establishes context (service_id, action_id, etc.),
   Zhen constructs a context vector from the flow metadata.
   ANN search over local fragment embeddings surfaces relevant Pu.
   Relevance score = De. High De = fragment bubbles up. Low De = stays deep.

4. SEDIMENTATION (Jing)
   Fragments not accessed within configurable window migrate:
   L1 (hot, mmap) → L2 (warm, sled) → L3 (cold, append-only archive).
   Migration is one-way under normal operation.
   Access at any tier resets the fragment to L1.

5. EMERGENCE (Li)
   Over time, traffic patterns carve "riverbeds" in the knowledge topology.
   Fragments that co-occur in context develop implicit associations.
   Li is not computed — it is observed. Topology visualization shows
   emergent clusters that no one designed.
```

### Wire Integration

Zhen fragments can optionally travel as a new IPv6 HbH option type
(pending IANA allocation via RFC Editor). The option carries:

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
| Option Type   | Opt Data Len  |  Fragment Hash (BLAKE3)       |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+                               +
|                     (32 bytes total)                          |
+                                                               +
|                                                               |
+                                                               +
|                                                               |
+                                                               +
|                                                               |
+                                                               +
|                                                               |
+                                                               +
|                                                               |
+                               +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                               | Embedding Dim | Payload Len   |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
| Embedding Vector (variable, quantized uint8)                  |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
| Payload (variable)                                            |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

This is a **mutable** HbH option (nodes MAY modify embedding metadata as
fragments traverse). Aligns with existing Unheaded HbH architecture.

## Security Considerations

### Structural Security (Index-Free)

- **No index = no exfiltration target.** Compromising one node yields partial,
  contextual fragments — not a queryable database.
- **BLAKE3 content addressing prevents forgery.** A fragment's hash IS its
  identity. Tampering invalidates the hash. BLAKE3 at 256-bit provides
  128-bit post-quantum security via Grover's bound — sufficient.
- **Embedding vectors are lossy projections.** Original content cannot be
  reconstructed from embeddings alone. Privacy by mathematical design.
- **Cold strata are naturally air-gapped.** L3 Jing archives on offline
  media require physical access — geological security.

### Post-Quantum Cryptography (PQC) — MANDATORY

**Sacred Law: "That which remembers forever must be armored against forever."**

Zhen fragments are immortal. They sediment into Jing and persist for years,
decades, potentially the lifetime of the network. Any cryptographic protection
applied to fragments MUST withstand quantum adversaries, because the
harvest-now-decrypt-later (HNDL) threat window equals the fragment's
unbounded lifetime.

#### Threat Model: Quantum Adversary

| Attack Surface | Classical Crypto | Quantum Threat | PQ Mitigation |
|---|---|---|---|
| Fragment payload encryption | ECDH + AES-256-GCM | ECDH broken (Shor). AES-256 safe. | ML-KEM-1024 (Kyber) + AES-256-GCM hybrid |
| Gossip peer authentication | ECDSA / Ed25519 | Broken (Shor) | ML-DSA-65 (Dilithium) signatures |
| Gossip key exchange | ECDH (X25519) | Broken (Shor) | ML-KEM-768 (Kyber) + X25519 hybrid |
| TLS 1.3 (gRPC, QUIC) | ECDH + ECDSA | Both broken | Hybrid KEM + PQ signature when rustls ships |
| Content addressing (BLAKE3) | 256-bit hash | 128-bit (Grover) | Sufficient. No change needed. |
| Symmetric encryption (AES-256) | 256-bit | 128-bit (Grover) | Sufficient. No change needed. |
| HbH option integrity | Optional HMAC | HMAC-SHA256 safe | Sufficient. No change needed. |
| Monad transport auth | Kingdom-level | Inherits Monad PQ posture | Track Monad PQ migration |

#### Mandatory PQ Requirements

1. **Fragment payload encryption MUST use hybrid ML-KEM-1024 + X25519.**
   Classical ECDH alone is FORBIDDEN for Pu fragment encryption.
   Rationale: HNDL window is unbounded. Fragments in Jing today will be
   vulnerable to quantum decryption in 5-15 years.

2. **Gossip peer authentication MUST use ML-DSA-65 (Dilithium) signatures.**
   Classical ECDSA/Ed25519 alone is FORBIDDEN for peer identity.
   Rationale: Sybil resistance depends on unforgeable peer identity.
   Quantum adversary forging peer signatures can poison the entire mesh.

3. **Gossip key exchange MUST use hybrid ML-KEM-768 + X25519.**
   Rationale: Belt-and-suspenders. If ML-KEM is broken by cryptanalysis,
   X25519 still holds (and vice versa). Hybrid costs ~1KB extra per handshake.

4. **TLS 1.3 transport MUST enable PQ key exchange when available.**
   Track rustls/quinn PQ support. Enable hybrid KEM immediately upon availability.
   Until then, document the HNDL exposure window in the threat model.

5. **Classical-only asymmetric crypto is FORBIDDEN** for any operation
   protecting data with lifetime > 1 year. This is a hard architectural
   constraint, not a recommendation.

#### PQ Implementation Strategy

**Phase 1 (immediate)**: Feature-gated PQ crate dependencies. Compiles
without them but the API surface expects PQ types.

**Phase 2 (gossip)**: Hybrid key exchange for peer connections. ML-KEM-768
encapsulation concatenated with X25519 shared secret. Both feed into
HKDF-SHA256 for symmetric key derivation.

**Phase 3 (fragment encryption)**: Hybrid ML-KEM-1024 + X25519 for
per-fragment key encapsulation. AES-256-GCM for payload. KEM ciphertext
stored alongside fragment (adds ~1568 bytes per encrypted fragment).

**Phase 4 (TLS)**: Enable PQ in rustls/quinn when upstream ships.
Monitor: https://github.com/rustls/rustls/issues/PQ

#### PQ Key Sizes and Performance Budget

| Algorithm | Public Key | Ciphertext/Sig | Operations/sec (est.) |
|---|---|---|---|
| ML-KEM-768 | 1,184 B | 1,088 B | ~50K encaps/sec |
| ML-KEM-1024 | 1,568 B | 1,568 B | ~35K encaps/sec |
| ML-DSA-65 | 1,952 B | 3,293 B | ~15K sign/sec |
| X25519 (classical) | 32 B | 32 B | ~25K/sec |
| Ed25519 (classical) | 32 B | 64 B | ~30K sign/sec |

PQ key/ciphertext sizes are ~50x larger than classical. This impacts:
- Gossip bandwidth (mitigated: handshake is infrequent vs fragment transfer)
- Fragment storage overhead (+1.5KB per encrypted fragment)
- HbH option size (if carrying PQ signatures — may exceed MTU)

These costs are acceptable. The alternative is a Library whose encryption
has an expiration date. Unacceptable.

### Sybil Resistance

- **Gossip protocol is Sybil-resistant** via Monad transport authentication
  using PQ signatures (ML-DSA-65). Only Kingdom-authenticated peers
  participate in Qi propagation. Peer identity is bound to ML-DSA keypair.
- **Admission control**: New peers must present a valid ML-DSA signature
  chain rooted in a Kingdom trust anchor before joining the gossip mesh.

## Trade-offs

### What We Gain
- Knowledge that survives node failure, network partition, even physical destruction
- No single point of failure for knowledge storage
- Organic relevance without algorithmic bias
- Network that gets wiser with age
- Resistance to censorship and seizure

### What We Pay
- Non-deterministic retrieval latency (power law: ms for hot, hours for deep)
- No guaranteed recall — relevance is probabilistic, not exact
- Storage grows monotonically (fragments never die)
- Embedding model quality bounds recall quality
- Complexity of a fourth mythological pillar in an already rich system

### What We Explicitly Reject
- Primary indexes (they create roots that can be cut)
- Strong consistency (consensus requires quorum; quorum can be denied)
- Guaranteed retrieval (deterministic lookup requires an index)
- Fragment deletion (burning books is the failure mode we're preventing)

## Alternatives Considered

| Alternative              | Why Rejected                                           |
|--------------------------|--------------------------------------------------------|
| etcd / Raft consensus    | Leader dependency. Quorum loss = unavailability.       |
| CockroachDB / Spanner    | SQL requires schema = index. Heavy. Not Layer 0.       |
| IPFS / Filecoin          | Content-addressed but DHT-indexed. Has a lookup ring.  |
| Git-based knowledge      | DAG is an index. Requires clone. Not gossip-native.    |
| Plain Ollama             | Full LLM inference is overkill. Embeddings only.       |
| SQLite per-node          | Local index. Doesn't gossip. Not anti-fragile.         |
| Do nothing               | The Library stays in one building. Unacceptable.       |

## Compliance Notes (MoatGhost)

- GPL-3.0 applies to zhend and all Zhen components
- ONNX Runtime (MIT) — compatible
- blake3 (CC0/Apache-2.0) — compatible
- sled (MIT/Apache-2.0) — compatible
- tonic (MIT) — compatible
- quinn (MIT/Apache-2.0) — compatible
- pqcrypto-kyber (MIT/Apache-2.0, wraps public-domain PQClean) — compatible
- pqcrypto-dilithium (MIT/Apache-2.0, wraps public-domain PQClean) — compatible
- x25519-dalek (BSD-3-Clause) — compatible
- aes-gcm (MIT/Apache-2.0) — compatible
- hkdf (MIT/Apache-2.0) — compatible
- THIRD_PARTY.md update required at implementation time
- PQClean upstream is public domain (CC0) — no encumbrance

## Success Metrics

- [ ] `zhend` boots and joins gossip mesh within 5 seconds
- [ ] Fragment ingestion: >10K fragments/sec sustained
- [ ] L1 recall latency: <1ms P99 for hot fragments
- [ ] L2 recall latency: <10ms P99 for warm fragments
- [ ] Gossip convergence: novel fragment reaches 90% of peers within 30 seconds
- [ ] Embedding quality: >0.85 cosine similarity on known-relevant fragment pairs
- [ ] Zero fragments lost after killing 50% of nodes simultaneously
- [ ] Storage grows <1GB/day under normal Kingdom traffic patterns

## References

- Kademlia DHT (Maymounkov & Mazières, 2002) — what we learned from and departed
- SWIM gossip protocol (Das et al., 2002) — membership + failure detection basis
- Hopfield networks (Hopfield, 1982) — associative memory as energy minimization
- Stigmergy in ant colonies — distributed coordination without central control
- Holographic memory — every fragment contains the whole at lower resolution
- Dao De Jing (Laozi) — the philosophical substrate of the design
- The Library of Alexandria — the failure mode we exist to prevent
- FIPS 203: ML-KEM (Module-Lattice-Based Key-Encapsulation Mechanism) — NIST, 2024
- FIPS 204: ML-DSA (Module-Lattice-Based Digital Signature Algorithm) — NIST, 2024
- FIPS 205: SLH-DSA (Stateless Hash-Based Digital Signature Algorithm) — NIST, 2024
- Hybrid Key Exchange in TLS 1.3 (draft-ietf-tls-hybrid-design) — belt-and-suspenders
- Harvest Now, Decrypt Later (HNDL) — NSA/CISA/NIST quantum threat framing
- PQClean project — portable, clean C implementations of PQ algorithms
