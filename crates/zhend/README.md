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
zhend
├── pu/       # Pu (朴) — Fragment format (the Uncarved Block)
├── qi/       # Qi (氣) — Gossip propagation (Vital Breath)
├── de/       # De (德) — Relevance engine (Virtue/Gravity)
├── li/       # Li (理) — Emergent structure observation (Pattern)
├── jing/     # Jing (經) — Deep archive (the Classics)
├── crypto/   # Post-Quantum cryptography (hybrid KEM, ML-DSA, envelopes)
├── api/      # gRPC + QUIC external interfaces
├── monad/    # Monad bridge (IPv6 HbH integration)
└── proto/    # Protobuf service definitions
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
| Content addressing | BLAKE3 (256-bit, quantum-safe) | — |

Classical-only asymmetric crypto is **forbidden** for any operation protecting
data with lifetime > 1 year. The `pq` feature is enabled by default.

## Mythological Foundation: Pillar 4 — The Dao

| Concept | Chinese | Role |
|---------|---------|------|
| Zhen    | 真      | Layer 0 — original truth before naming |
| Pu      | 朴      | Fragment format — uncarved blocks |
| Qi      | 氣      | Gossip propagation — vital breath |
| Wu Wei  | 無為    | No-index design — non-action |
| De      | 德      | Relevance gravity — context attracts |
| Jing    | 經      | Deep strata — ancient knowledge |
| Li      | 理      | Emergent structure — pattern in wood grain |

## Quick Start

```bash
# Build (PQ enabled by default)
cargo build --release

# Build without PQ (graceful degradation)
cargo build --release --no-default-features

# Run with defaults
./target/release/zhend

# Run with custom config
./target/release/zhend \
  --data-dir /tmp/zhend \
  --grpc-addr [::1]:7300 \
  --gossip-addr [::]:7302 \
  --seed-peer [::1]:7302

# Run tests
cargo test --all-features

# Run benchmarks
cargo bench

# Security audit
cargo audit

# NixOS (flake)
nix build .#default
nix develop  # dev shell with all tools
```

### NixOS Deployment

```nix
# In configuration.nix
imports = [ ./path/to/zhend/nix/module.nix ];

services.zhend = {
  enable = true;
  seedPeers = [ "[fd00::2]:7302" "[fd00::3]:7302" ];
  logFormat = "json";
};
```

## Status

**PROPOSED** — Scaffold complete with:
- Core data structures (Fragment, TieredStore, Gossip, Embedder)
- L1/L2/L3 tiered storage with sedimentation
- Post-quantum crypto module (hybrid ML-KEM + X25519, ML-DSA-65, sealed envelopes)
- gRPC proto definitions
- IPv6 HbH option encode/decode
- Geological trend observation
- NixOS module + flake
- GitHub Actions CI
- 35+ unit tests across all modules

Next: Phase 0 — `cargo build` on target machine.

## License

GPL-3.0-only — The Library cannot be enclosed.

Part of the [Unheaded Protocol](https://github.com/unheaded/unheaded).
