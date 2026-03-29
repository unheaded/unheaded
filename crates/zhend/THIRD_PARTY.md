# Third-Party Dependencies — zhend

**Last updated**: 2026-03-28  
**License**: GPL-3.0-only  
**Compliance reviewed by**: MoatGhost  

All dependencies are compatible with GPL-3.0-only.

## Runtime Dependencies

| Crate | Version | License | Source | Purpose |
|---|---|---|---|---|
| blake3 | 1.x | CC0-1.0 OR Apache-2.0 | https://github.com/BLAKE3-team/BLAKE3 | Content addressing (BLAKE3 hash) |
| hex | 0.4 | MIT OR Apache-2.0 | https://github.com/KokaKiwi/rust-hex | Hex encoding for fragment IDs |
| sled | 0.34 | MIT OR Apache-2.0 | https://github.com/spacejam/sled | L2 warm embedded KV store |
| memmap2 | 0.9 | MIT OR Apache-2.0 | https://github.com/RazrFalcon/memmap2-rs | Memory-mapped I/O for L1/L3 |
| serde | 1.x | MIT OR Apache-2.0 | https://github.com/serde-rs/serde | Serialization framework |
| bincode | 1.x | MIT | https://github.com/bincode-org/bincode | Compact binary serialization |
| tokio | 1.x | MIT | https://github.com/tokio-rs/tokio | Async runtime |
| tonic | 0.12 | MIT | https://github.com/hyperium/tonic | gRPC server/client |
| prost | 0.13 | Apache-2.0 | https://github.com/tokio-rs/prost | Protobuf codegen |
| quinn | 0.11 | MIT OR Apache-2.0 | https://github.com/quinn-rs/quinn | QUIC transport |
| rustls | 0.23 | MIT OR Apache-2.0 OR ISC | https://github.com/rustls/rustls | TLS 1.3 |
| tracing | 0.1 | MIT | https://github.com/tokio-rs/tracing | Structured logging |
| tracing-subscriber | 0.3 | MIT | https://github.com/tokio-rs/tracing | Log formatting |
| clap | 4.x | MIT OR Apache-2.0 | https://github.com/clap-rs/clap | CLI argument parsing |
| thiserror | 2.x | MIT OR Apache-2.0 | https://github.com/dtolnay/thiserror | Error derive macro |
| anyhow | 1.x | MIT OR Apache-2.0 | https://github.com/dtolnay/anyhow | Error handling |
| chrono | 0.4 | MIT OR Apache-2.0 | https://github.com/chronotope/chrono | Date/time |
| rand | 0.8 | MIT OR Apache-2.0 | https://github.com/rust-random/rand | RNG for gossip/nonces |
| zeroize | 1.x | MIT OR Apache-2.0 | https://github.com/RustCrypto/utils | Secret memory cleanup |

## Post-Quantum Cryptography Dependencies (feature: `pq`)

| Crate | Version | License | Source | Purpose |
|---|---|---|---|---|
| pqcrypto-kyber | 0.8 | MIT OR Apache-2.0 | https://github.com/rustpq/pqcrypto | ML-KEM key encapsulation (FIPS 203) |
| pqcrypto-dilithium | 0.5 | MIT OR Apache-2.0 | https://github.com/rustpq/pqcrypto | ML-DSA digital signatures (FIPS 204) |
| pqcrypto-traits | 0.3 | MIT OR Apache-2.0 | https://github.com/rustpq/pqcrypto | Common PQ type traits |
| x25519-dalek | 2.x | BSD-3-Clause | https://github.com/dalek-cryptography/curve25519-dalek | Classical ECDH (hybrid partner) |
| aes-gcm | 0.10 | MIT OR Apache-2.0 | https://github.com/RustCrypto/AEADs | AES-256-GCM symmetric encryption |
| hkdf | 0.12 | MIT OR Apache-2.0 | https://github.com/RustCrypto/KDFs | Key derivation (RFC 5869) |
| sha2 | 0.10 | MIT OR Apache-2.0 | https://github.com/RustCrypto/hashes | SHA-256 for HKDF |

### PQClean Upstream

The `pqcrypto-*` crates wrap the PQClean project's C implementations:
- **Source**: https://github.com/PQClean/PQClean
- **License**: Public domain (CC0-1.0) for reference implementations
- **Algorithms**: NIST-standardized (FIPS 203, 204, 205)
- **Constant-time**: Reference implementations are designed to be constant-time
- **Audit status**: NIST-reviewed as part of the PQ standardization process

## Dev Dependencies

| Crate | Version | License | Purpose |
|---|---|---|---|
| tempfile | 3.x | MIT OR Apache-2.0 | Temporary directories for tests |
| proptest | 1.x | MIT OR Apache-2.0 | Property-based testing |
| criterion | 0.5 | MIT OR Apache-2.0 | Benchmarking |

## License Compatibility Matrix

```
GPL-3.0-only (zhend)
  ├── CC0-1.0       ✓ (public domain, compatible with everything)
  ├── MIT            ✓ (permissive, compatible with GPL)
  ├── Apache-2.0     ✓ (compatible with GPL-3.0 per FSF guidance)
  ├── BSD-3-Clause   ✓ (permissive, compatible with GPL)
  └── ISC            ✓ (permissive, compatible with GPL)
```

No copyleft dependency conflicts detected. All dependencies are permissive
or public domain, allowing GPL-3.0-only distribution without issue.

## Supply Chain Notes

- All crates sourced from crates.io (Rust's official package registry)
- `cargo audit` should be run before each release to check for known vulnerabilities
- `cargo deny` recommended for license and advisory checking in CI
- PQClean C code is vendored into the pqcrypto crates (no runtime C dependency download)
