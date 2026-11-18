# Security Policy — zhend

**Crate**: zhend
**License**: GPL-3.0-or-later
**Last reviewed**: 2026-03-28

## Security Posture

### Unsafe Code

`#![deny(unsafe_code)]` is enforced at the crate root (`lib.rs`). There are zero
`unsafe` blocks in the zhend codebase. All memory safety is guaranteed by the Rust
compiler. If unsafe is ever required (e.g., FFI for ONNX runtime), it must be
documented in this file with a justification and confined to a minimal scope.

### Post-Quantum Cryptography

Peer identity uses post-quantum signatures. Classical-only asymmetric crypto is
**forbidden** for any operation protecting data with lifetime > 1 year.

| Operation | Algorithm | Standard | Module |
|---|---|---|---|
| Digital signatures | ML-DSA-65 (Dilithium) | FIPS 204 | `crypto::sign` |
| Transport key exchange | TLS 1.3, X25519MLKEM768 first (rustls, aws-lc-rs) | FIPS 203, RFC 8446 | `api::quic` |
| Content addressing | BLAKE3 (256-bit) | — | `pu::fragment` |

Fragment payloads are not encrypted by zhend. The hybrid ML-KEM-768 + X25519 key
encapsulation (`crypto::kem`) and the AES-256-GCM sealed fragment envelope
(`crypto::envelope`) had no callers and were deleted on 2026-11-18
(`docs/legal/EXPORT-CRYPTO-AUDIT-2026-11-12.md`, "Update 2026-11-18").

The `crypto` module conditionally compiles its submodule behind `#[cfg(feature = "pq")]`.
Without the `pq` feature, the gossip engine runs without peer authentication (acceptable
for testing only, not production).

Secret key material is zeroized on drop via the `zeroize` crate.

### Input Validation

All network-facing inputs are treated as hostile:

- **Gossip messages** (`qi::message`): Size-checked against `MAX_MSG_SIZE` (65,000 bytes)
  before deserialization. Empty messages rejected. Digest and want batch counts capped
  at `MAX_DIGESTS_PER_BATCH` (1,500). Fragment batch bytes capped at
  `MAX_FRAGMENT_BATCH_BYTES` (60,000).
- **QUIC transport** (`api::quic`): Payload size capped at 16 MB before allocation.
  Op codes validated. Empty ingest payloads rejected.
- **gRPC service** (`api::grpc`): Empty payloads rejected with `INVALID_ARGUMENT`.
  Fragment IDs validated for correct length (32 bytes) before use.
- **UDP transport** (`qi::transport`): Outbound messages checked against `MAX_MSG_SIZE`.
- **Fragment codec** (`pu::codec`): All decoded fragments verified via BLAKE3 integrity
  check. Tampered payloads rejected.
- **HbH option parsing** (`monad::hbh`): Buffer length validated before all field reads.
  Option type and data length cross-checked.
- **PQ crypto** (`crypto::sign`): Key and signature lengths validated before
  cryptographic operations.

### Gossip Admission Control

When PQ is enabled and a local signing keypair is configured:
- Unauthenticated peers cannot receive fragment data (digests or payloads).
- Data messages from unauthenticated peers are dropped (with an IdentityOffer sent back).
- Ping/PingAck (membership protocol) operates pre-authentication.
- Identity self-attestation is verified via ML-DSA-65 before marking a peer as authenticated.

### Fuzz Testing

Property-based testing via `proptest` is available for the codec (Phase 2 of development).
The `proptest` crate is included as a dev-dependency.

## Reporting Vulnerabilities

If you discover a security vulnerability in zhend, please report it privately:

- **Email**: muck@bellis.tech
- **Subject**: `[SECURITY] zhend: <brief description>`

Do not open a public GitHub issue for security vulnerabilities.

We will acknowledge receipt within 48 hours and provide a timeline for a fix.

## Threat Model

zhend is designed to operate in adversarial network environments:

1. **Network adversary**: All gossip peers are untrusted. Messages are authenticated
   via PQ signatures. Fragment integrity is verified via BLAKE3.
2. **Quantum adversary**: Peer signatures are ML-DSA-65, and the QUIC transport
   offers the hybrid X25519MLKEM768 group first.
3. **Storage adversary**: Fragment IDs are content-addressed (BLAKE3). Tampered storage
   is detected on read. Stored fragments are not encrypted.
4. **Sybil adversary**: Peer identity is bound to ML-DSA-65 keypairs with self-attestation.
   Unauthenticated peers are excluded from fragment exchange.

## Dependencies

See `THIRD_PARTY.md` for a complete dependency inventory with licenses.
`sbom/SBOM.md` at the repo root gives the license of every crate in `Cargo.lock` and
names what is left for a person to rule on. `cargo audit` should be run before each release.
