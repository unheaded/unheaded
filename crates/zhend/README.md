# 真 zhend

**Layer 0 — Anti-fragile knowledge substrate for the Unheaded Protocol.**

The Library that cannot burn.

---

## What is Zhen?

Zhen (真, "truth/genuine") is a gossip-propagated, index-free, associative
knowledge system that sits beneath the Unheaded Protocol stack. It stores
content-addressed fragments that:

- **Propagate via gossip** — riding existing Monad packet traffic
- **Surface through relevance** — not query, not index, but contextual attraction
- **Never die** — fragments migrate to deeper strata but are never deleted
- **Cannot be seized** — no root, no index, no single point of exfiltration

## Architecture

```
┌─────────────────────────────────────────┐
│              zhend daemon               │
├──────┬──────┬──────┬──────┬──────┬──────┤
│  pu  │  qi  │  de  │  li  │ jing │ api  │
│ frag │ goss │ rel  │ topo │ deep │ grpc │
│ store│ prop │ emb  │ obs  │ arch │ quic │
├──────┴──────┴──────┴──────┴──────┴──────┤
│         crypto (PQ-hardened)            │
│   ML-KEM-768 + X25519 | ML-DSA-65      │
│   AES-256-GCM | HKDF-SHA256 | zeroize  │
├─────────────────────────────────────────┤
│              monad bridge               │
├─────────────────────────────────────────┤
│        IPv6 HbH / Monad transport       │
└─────────────────────────────────────────┘
```

### Module Breakdown

```
zhend/src/
├── pu/          # Pu (朴) — Fragment format (the Uncarved Block)
│   ├── fragment.rs  — Core Fragment type, BLAKE3 content addressing
│   ├── codec.rs     — Binary encode/decode with integrity verification
│   └── store.rs     — L1/L2/L3 tiered storage with sedimentation
├── qi/          # Qi (氣) — Gossip propagation (Vital Breath)
│   ├── gossip.rs    — SWIM-variant gossip engine, PQ admission control
│   ├── message.rs   — Wire protocol messages (DigestBatch, WantBatch, etc.)
│   ├── transport.rs — UDP transport layer with size limits
│   └── peer.rs      — Peer list management, liveness tracking
├── de/          # De (德) — Relevance engine (Virtue/Gravity)
│   ├── embedder.rs  — Embedding generation (ONNX-ready, stub for now)
│   ├── similarity.rs — Cosine similarity ranking
│   └── context.rs   — Context window management
├── li/          # Li (理) — Emergent structure observation (Pattern)
│   ├── strata.rs    — Strata snapshot history, geological trends
│   └── topology.rs  — Network topology observation
├── jing/        # Jing (經) — Deep archive (the Classics)
│   ├── archive.rs   — Append-only L3 cold storage
│   └── pilgrimage.rs — Deep retrieval with progress streaming
├── crypto/      # Post-Quantum cryptography
│   ├── kem.rs       — Hybrid ML-KEM-768 + X25519 key encapsulation
│   ├── sign.rs      — ML-DSA-65 signatures, PeerIdentity
│   └── envelope.rs  — Sealed fragment encryption (AES-256-GCM)
├── api/         # External interfaces
│   ├── grpc.rs      — tonic gRPC service (Ingest, Surface, Status, Pilgrimage)
│   └── quic.rs      — quinn QUIC server with custom wire protocol
├── monad/       # Monad transport bridge
│   ├── bridge.rs    — Monad packet integration
│   └── hbh.rs       — IPv6 Hop-by-Hop option encode/decode
├── lib.rs       — Crate root, ZhenConfig, ZhenError
├── main.rs      — Daemon binary, CLI, subsystem orchestration
└── proto/       — Protobuf service definitions (zhen.v1)
```

### Data Flow

```
                Ingest
                  │
                  v
  ┌─────────────────────────────┐
  │     Fragment::new()         │  BLAKE3 hash = identity
  │     Embedder::embed()       │  Vector projection for De
  └──────────────┬──────────────┘
                 │
                 v
  ┌─────────────────────────────┐
  │     L1 (Hot) — mmap'd      │  Sub-ms access
  │         │                   │  Gossip source (Qi)
  │     sedimentation           │  Access-time driven
  │         │                   │
  │     L2 (Warm) — sled        │  Single-digit ms
  │         │                   │
  │     L3 (Cold) — archive     │  Append-only, deep retrieval
  └─────────────────────────────┘

  ┌─────────────────────────────┐
  │    Qi Gossip Engine         │
  │  1. Select random L1 frags  │
  │  2. Send BLAKE3 digests     │  Pull-on-demand (not push)
  │  3. Peers reply with "want" │
  │  4. Send full fragments     │
  │  5. PQ auth: admission ctrl │
  └─────────────────────────────┘
```

## Post-Quantum Cryptography

**Sacred Law: "That which remembers forever must be armored against forever."**

Zhen fragments are immortal. Classical asymmetric cryptography has a finite
lifetime against quantum adversaries (est. 5-15 years). This mismatch is
existential for a system designed to never forget.

All asymmetric crypto in zhend uses hybrid post-quantum constructions:

| Operation | Algorithm | Standard |
|---|---|---|
| Key encapsulation | ML-KEM-768 + X25519 hybrid | FIPS 203 |
| Digital signatures | ML-DSA-65 (Dilithium) | FIPS 204 |
| Symmetric encryption | AES-256-GCM | FIPS 197 |
| Key derivation | HKDF-SHA256 | RFC 5869 |
| Content addressing | BLAKE3 (256-bit, quantum-safe) | -- |

Classical-only asymmetric crypto is **forbidden** for any operation protecting
data with lifetime > 1 year. The `pq` feature is enabled by default.

### Security

- `#![deny(unsafe_code)]` enforced at crate root
- Zero `unsafe` blocks in the codebase
- All network inputs treated as hostile with size validation
- Secret keys zeroized on drop
- See `SECURITY.md` for full security policy and threat model

## Mythological Foundation: Pillar 4 -- The Dao

| Concept | Chinese | Role |
|---------|---------|------|
| Zhen    | 真      | Layer 0 -- original truth before naming |
| Pu      | 朴      | Fragment format -- uncarved blocks |
| Qi      | 氣      | Gossip propagation -- vital breath |
| Wu Wei  | 無為    | No-index design -- non-action |
| De      | 德      | Relevance gravity -- context attracts |
| Jing    | 經      | Deep strata -- ancient knowledge |
| Li      | 理      | Emergent structure -- pattern in wood grain |

## Quick Start

```bash
# Build (PQ enabled by default)
cargo build --release

# Build without PQ (graceful degradation, testing only)
cargo build --release --no-default-features

# Run with defaults
./target/release/zhend

# Run with custom config
./target/release/zhend \
  --data-dir /tmp/zhend \
  --grpc-addr [::1]:7300 \
  --quic-addr [::1]:7301 \
  --gossip-addr [::]:7302 \
  --seed-peer [::1]:7302 \
  --gossip-fanout 3 \
  --gossip-interval-ms 1000 \
  --embedding-dims 384 \
  --log-format json

# Run tests
cargo test --all-features

# Run clippy
cargo clippy --all-targets --all-features

# Run benchmarks
cargo bench

# Security audit
cargo audit

# NixOS (flake)
nix build .#default
nix develop  # dev shell with all tools
```

### Usage Examples

**Ingest a fragment via gRPC (using grpcurl):**

```bash
# Ingest raw text
grpcurl -plaintext -d '{
  "payload": "'$(echo -n "the dao that can be named" | base64)'",
  "content_type": "text/plain"
}' [::1]:7300 zhen.v1.Zhen/Ingest

# Surface relevant fragments given a context embedding
grpcurl -plaintext -d '{
  "context_embedding": "'$(echo -n "knowledge" | base64)'",
  "top_k": 5,
  "min_de": 0.3
}' [::1]:7300 zhen.v1.Zhen/Surface

# Check node status
grpcurl -plaintext [::1]:7300 zhen.v1.Zhen/Status
```

**QUIC wire protocol (programmatic):**

```
Request:  [op: u8][payload_len: u32 BE][payload: bytes]
Response: [status: u8][payload_len: u32 BE][payload: bytes]

Op codes: 1=Ingest, 2=Surface, 3=Status
Status:   0=OK, 1=Bad Request, 2=Internal Error
```

**Multi-node gossip cluster:**

```bash
# Node A
zhend --data-dir /tmp/zhend-a --gossip-addr [::]:7302 --grpc-addr [::1]:7300

# Node B (seeds to A)
zhend --data-dir /tmp/zhend-b --gossip-addr [::]:7312 --grpc-addr [::1]:7310 \
  --seed-peer [::1]:7302

# Node C (seeds to A and B)
zhend --data-dir /tmp/zhend-c --gossip-addr [::]:7322 --grpc-addr [::1]:7320 \
  --seed-peer [::1]:7302 --seed-peer [::1]:7312

# Ingest on A, gossip propagates to B and C automatically
```

### NixOS Deployment

```nix
# In configuration.nix
imports = [ ./path/to/zhend/nix/module.nix ];

services.zhend = {
  enable = true;
  dataDir = "/var/lib/zhend";
  grpcAddr = "[::1]:7300";
  quicAddr = "[::1]:7301";
  gossipAddr = "[::]:7302";
  seedPeers = [ "[fd00::2]:7302" "[fd00::3]:7302" ];
  embeddingModel = null;      # path to ONNX model, or null
  embeddingDims = 384;        # must match model
  gossipFanout = 3;
  gossipIntervalMs = 1000;
  l1ToL2Secs = 3600;          # 1 hour
  l2ToL3Secs = 604800;        # 1 week
  logFormat = "json";
};
```

Systemd hardening is applied automatically: `NoNewPrivileges`, `ProtectSystem=strict`,
`PrivateTmp`, `PrivateDevices`, `MemoryDenyWriteExecute`, `RestrictNamespaces`,
`RestrictAddressFamilies=AF_INET6 AF_UNIX`, `SystemCallFilter`, and more.

## Configuration

All configuration is available via CLI flags or the NixOS module. See `config.example.toml`
for a TOML configuration template.

| Field | CLI Flag | Default | Description |
|---|---|---|---|
| data_dir | `--data-dir` | `/var/lib/zhend` | Storage directory for all tiers |
| grpc_addr | `--grpc-addr` | `[::1]:7300` | gRPC listen address |
| quic_addr | `--quic-addr` | `[::1]:7301` | QUIC listen address |
| gossip_addr | `--gossip-addr` | `[::]:7302` | UDP gossip listen address |
| seed_peers | `--seed-peer` | `[]` | Seed peers for mesh join (repeatable) |
| l1_to_l2_secs | `--l1-to-l2-secs` | `3600` | L1 to L2 sedimentation threshold |
| l2_to_l3_secs | `--l2-to-l3-secs` | `604800` | L2 to L3 sedimentation threshold |
| embedding_model | `--embedding-model` | `null` | ONNX model path (optional) |
| embedding_dims | `--embedding-dims` | `384` | Embedding vector dimensions |
| gossip_fanout | `--gossip-fanout` | `3` | Peers per gossip cycle |
| gossip_interval_ms | `--gossip-interval-ms` | `1000` | Gossip cycle interval (ms) |

## Status

**OPERATIONAL** -- 139+ tests passing, 10 of 14 development phases complete:

- Phase 0: Scaffold and build system
- Phase 1: Core storage (Pu) -- L1/L2/L3 tiered store, BLAKE3 content addressing
- Phase 2: Codec and fuzz testing -- bincode serialization, integrity verification
- Phase 3: Gossip engine (Qi) -- SWIM-variant gossip, peer management, message protocol
- Phase 4: Relevance engine (De) -- embedding, cosine similarity ranking
- Phase 5: Deep archive (Jing) -- L3 cold storage, pilgrimage retrieval
- Phase 6: Structure observation (Li) -- strata snapshots, geological trends
- Phase 8: API layer -- gRPC (tonic) + QUIC (quinn) servers
- Phase 9: Post-quantum crypto -- hybrid ML-KEM + X25519, ML-DSA-65, sealed envelopes
- Phase 10: PQ gossip integration -- authenticated peer admission control
- Phase 11: Security review -- `deny(unsafe_code)`, input validation audit, SECURITY.md
- Phase 12: NixOS module verification -- hardening flags, config field coverage
- Phase 13: Documentation -- architecture docs, usage examples

Remaining: Phase 7 (Monad bridge integration), Phase 14 (performance benchmarking).

## Dependencies

See `THIRD_PARTY.md` for the complete dependency inventory with licenses.
All dependencies are permissive or public domain -- no copyleft conflicts.

## License

GPL-3.0-only -- The Library cannot be enclosed.

Part of the [Unheaded Protocol](https://github.com/unheaded/unheaded).
