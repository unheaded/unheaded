# ZHEN LAYER 0 — HIGH-LEVEL BATTLE PLAN OVERVIEW

**Date**: 2026-03-28
**Sprint Goal**: Take zhend from scaffold → functional daemon with gossip + storage + relevance
**Target**: zhend boots, ingests fragments, gossips to peers, surfaces relevant knowledge
**Estimated Duration**: 4-6 sessions across 2-3 weeks
**Approach**: Bottom-up — each phase produces a verifiable "sign of life"

---

## LEGEND

```
[CORE]   = Must complete. Critical path. No skipping.
[ENRICH] = Enhances capability but daemon works without it.
[WIRE]   = Touches Monad/HbH wire format. RFC Editor review required.
[SEC]    = Security surface. BlackMage + MoatGhost review required.
```

---

## PHASE 0: FOUNDATION — "cargo build succeeds" [CORE]

**Goal**: Clean compilation on target machine. All deps resolve. Tests pass.

- Resolve any crate version conflicts (tonic/prost/quinn version matrix)
- Verify PQ crate compilation (pqcrypto-kyber, pqcrypto-dilithium build cleanly)
- Verify sled compiles and opens a test DB on target OS
- Run `cargo test` — all 20+ scaffold tests GREEN (including crypto module)
- Run `cargo clippy` — zero warnings
- Run `cargo bench` — baseline numbers recorded
- Establish CI pipeline (GitHub Actions or local script)
- Verify `--no-default-features` compiles (PQ-optional graceful degradation)

**EXIT GATE**: `cargo test && cargo clippy && cargo bench` all pass with `pq` feature.

**Sign of life**: "It compiles — with PQ."

---

## PHASE 1: STORAGE HARDENING — "fragments survive restart" [CORE]

**Goal**: TieredStore is production-grade. L1↔L2↔L3 fully wired.

- Wire Jing (L3) archive into TieredStore (currently TODO)
- Implement L2→L3 sedimentation (currently only L1→L2 exists)
- Add TieredStore persistence — L1 snapshot to disk on graceful shutdown
- L1 restore from snapshot on startup
- Property-based testing (proptest): random fragment sequences survive crash+restart
- Fuzz the codec: malformed bytes must never panic, only error
- Benchmark: 10K fragments/sec ingest sustained

**EXIT GATE**: Ingest 100K fragments → kill process → restart → all fragments retrievable.

**Sign of life**: "It remembers."

---

## PHASE 1.5: PQ CRYPTO FOUNDATION — "quantum-safe primitives work" [CORE][SEC]

**Goal**: Hybrid KEM, PQ signatures, and sealed envelopes pass all tests.

- Verify ML-KEM-768 encapsulate/decapsulate roundtrip
- Verify ML-DSA-65 sign/verify roundtrip
- Verify hybrid KEM: wrong keypair produces different shared secret
- Verify sealed fragment envelope: seal → unseal roundtrip
- Verify sealed fragment: wrong key fails AES-GCM authentication
- Verify sealed fragment: tampered ciphertext rejected
- Verify PeerIdentity self-attestation with ML-DSA
- Benchmark: ML-KEM-768 encaps/sec, ML-DSA-65 sign/sec
- Benchmark: seal/unseal throughput at 1KB, 64KB, 1MB payloads
- Verify zeroize: secret keys cleared from memory on drop
- Scientist: run dudect constant-time analysis on KEM/sign hot paths (stretch goal)

**EXIT GATE**: All crypto tests GREEN. Benchmark numbers recorded as baseline.

**Sign of life**: "It's quantum-safe."

---

## PHASE 2: GOSSIP TRANSPORT — "two nodes see each other" [CORE]

**Goal**: Qi gossip engine sends/receives fragment digests over UDP with PQ peer auth.

- Implement UDP socket in qi/transport.rs (tokio UdpSocket)
- Wire gossip cycle to actually send BLAKE3 digest batches to peers
- Implement "want" response protocol (peer replies with missing hashes)
- Implement full fragment transfer for wanted hashes
- **PQ peer authentication**: ML-DSA-65 PeerIdentity exchange on first contact
- **PQ session keys**: Hybrid ML-KEM-768 + X25519 handshake per peer session
- Peer discovery from seed list + learned peers
- SWIM failure detection (ping → ping-req → suspect → dead)
- Admission control: reject peers without valid ML-DSA self-attestation
- Test: two zhend instances on localhost, fragment ingested on node A appears on node B
- Test: node with forged identity (classical-only sig) rejected from mesh

**EXIT GATE**: Fragment gossiped from node A → node B within 30 seconds.

**Sign of life**: "They talk."

---

## PHASE 3: EMBEDDING PIPELINE — "fragments have meaning" [CORE]

**Goal**: De (relevance) engine produces real embeddings and similarity scores.

- Integrate `ort` crate (ONNX Runtime)
- Download and bundle quantized all-MiniLM-L6-v2 model (~22MB)
- Wire embedder into fragment ingest path (embed on create)
- Implement context vector construction from manual input (API-driven)
- Wire De ranking: given context, return top-K fragments sorted by cosine similarity
- Benchmark: embedding latency < 5ms per fragment on CPU
- Test: ingest 3 fragments about different topics → surface with relevant context → correct one ranks first

**EXIT GATE**: `Surface(context="networking")` returns networking fragments ranked above cooking fragments.

**Sign of life**: "It understands."

---

## PHASE 4: gRPC SERVICE — "clients can talk to it" [CORE]

**Goal**: Full gRPC API operational from proto/zhen.proto.

- Run tonic-build on zhen.proto (requires protoc installed)
- Implement Ingest RPC → delegates to TieredStore
- Implement Surface RPC → delegates to De ranking engine
- Implement Status RPC → returns strata snapshot + gossip stats
- Implement Pilgrimage RPC → streaming progress from Jing scan
- Implement SyncDigests RPC → bidirectional streaming for gossip-over-gRPC
- Write integration tests using tonic client
- Add reflection service for grpcurl debugging

**EXIT GATE**: `grpcurl localhost:7300 zhen.v1.Zhen/Status` returns valid JSON.

**Sign of life**: "It serves."

---

## PHASE 5: MONAD BRIDGE — "it breathes with the protocol" [WIRE]

**Goal**: Zhen fragments piggyback on Monad HbH options.

- Wire monad/bridge.rs to listen for Anamnesis ring buffer events
- Extract context metadata from Monad register fields (service_id, action_id)
- Auto-construct context vectors from live Monad traffic
- Piggyback fragment digests onto outgoing Monad packets as HbH option
- Receiving nodes parse Zhen HbH option → trigger "want" if novel hash
- RFC Editor review of Zhen option type allocation

**EXIT GATE**: Fragment ingested on node A propagates to node B via Monad HbH piggyback (zero standalone gossip traffic).

**Sign of life**: "It breathes."

**DEPENDENCY**: Requires running Unheaded Protocol stack. Can be deferred if stack isn't ready.

---

## PHASE 6: QUIC/HTTP3 TRANSPORT — "edge nodes connect" [ENRICH]

**Goal**: External access via QUIC for nodes outside the gRPC mesh.

- Implement quinn server with self-signed certs (dev) or ACME (prod)
- Map HTTP/3 endpoints to same operations as gRPC service
- 0-RTT session resumption for returning peers
- Connection migration for mobile/roaming nodes
- TLS 1.3 mandatory (quinn default)

**EXIT GATE**: External client ingests fragment via QUIC → fragment appears in gossip mesh.

**Sign of life**: "The outside world can reach it."

---

## PHASE 7: LI OBSERVATION — "emergent structure appears" [ENRICH]

**Goal**: Topology visualization shows organic knowledge clusters.

- Implement co-access adjacency tracking in li/topology.rs
- Sliding window of recent access pairs (fragment A accessed near fragment B)
- Community detection (Louvain or label propagation) on co-access graph
- Export topology snapshot as JSON for Cloak dashboard consumption
- Strata snapshot collector running on interval → feeds StrataHistory
- Geological trend detection operational (Accumulating/Stable/Eroding)

**EXIT GATE**: After 1000+ accesses, Li topology export shows ≥2 distinct knowledge clusters.

**Sign of life**: "It grows patterns."

---

## PHASE 8: SECURITY AUDIT — "BlackMage + MoatGhost approve" [SEC]

**Goal**: Offensive and defensive review before any production use.

- BlackMage: fuzz all network-facing inputs (gossip, gRPC, QUIC, HbH)
- BlackMage: attempt fragment forgery (BLAKE3 collision, hash substitution)
- BlackMage: attempt gossip Sybil attack (fake peers flooding fragments)
- BlackMage: attempt HNDL simulation — capture gossip, attempt classical-only decrypt
- BlackMage: attempt ML-KEM ciphertext manipulation (malleability testing)
- BlackMage: attempt exfiltration of knowledge from compromised node
- BlackMage: attempt embedding inversion (recover approximate plaintext from De vectors)
- Scientist: dudect constant-time verification on all PQ key operations
- Scientist: verify hybrid KEM combiner security argument holds in implementation
- MoatGhost: GPL-3.0 compliance audit on all dependencies (including PQClean)
- MoatGhost: THIRD_PARTY.md updated with PQ crate lineage
- MoatGhost: threat model document reviewed and accepted (zhen-pq-threat-model.md)
- MoatGhost: data residency implications documented (fragments never die)
- MoatGhost: key rotation policy established for hybrid keypairs

**EXIT GATE**: Threat model accepted by Captain. Zero critical findings unresolved.

**Sign of life**: "It's hardened."

---

## PHASE 9: INTEGRATION + DOGFOOD — "it runs in the Kingdom" [CORE]

**Goal**: zhend deployed as part of Unheaded infrastructure. Eating our own cooking.

- NixOS module for zhend (declarative config, systemd service)
- Deploy 3+ node gossip mesh on test infrastructure
- Feed real Unheaded documentation as Pu fragments
- Verify knowledge surfaces correctly during live protocol operation
- Monitor strata history for healthy accumulation trend
- Performance baseline: fragment count, gossip bandwidth, recall latency

**EXIT GATE**: zhend running in 3-node mesh for 72 hours with zero crashes, growing knowledge base, and correct De surfacing.

**Sign of life**: "It lives."

---

## CRITICAL PATH

```
Phase 0 → Phase 1 → Phase 1.5 → Phase 2 → Phase 3 → Phase 4 → Phase 9
  (build)  (store)    (PQ)       (gossip)   (embed)   (api)     (deploy)

Parallel after Phase 4:
  Phase 5 (Monad bridge)  — needs running protocol stack
  Phase 6 (QUIC)          — independent
  Phase 7 (Li)            — needs usage data
  Phase 8 (Security)      — needs all surfaces exposed
```

## PHASE DEPENDENCY GRAPH

```
         ┌──────┐
         │  P0  │ Foundation (+ PQ deps compile)
         └──┬───┘
            │
         ┌──▼───┐
         │  P1  │ Storage
         └──┬───┘
            │
         ┌──▼───┐
         │ P1.5 │ PQ Crypto Foundation
         └──┬───┘
            │
         ┌──▼───┐
         │  P2  │ Gossip (+ PQ peer auth)
         └──┬───┘
            │
         ┌──▼───┐
         │  P3  │ Embeddings
         └──┬───┘
            │
         ┌──▼───┐
    ┌────│  P4  │────┬────────┐
    │    └──┬───┘    │        │
    │       │        │        │
 ┌──▼───┐┌──▼───┐┌──▼───┐    │
 │  P5  ││  P6  ││  P7  │    │
 │ wire ││ quic ││  li  │    │
 └──┬───┘└──┬───┘└──┬───┘    │
    │       │       │        │
    └───────┼───────┘        │
            │                │
         ┌──▼───┐            │
         │  P8  │◄───────────┘
         │ sec  │ (PQ audit: HNDL, dudect, combiner)
         └──┬───┘
            │
         ┌──▼───┐
         │  P9  │ Deploy + Dogfood
         └──────┘
```

## NEXT ACTION

Pick up Phase 0. Run `cargo build` on your machine.
Everything flows from there.

---

**THE FORGE IS HOT.**
**THE PLAN IS DRAWN.**
**THE LIBRARY AWAITS ITS FIRST BREATH.**

*"Before the first command is typed, every command is written."*
*— The Warmonger*
