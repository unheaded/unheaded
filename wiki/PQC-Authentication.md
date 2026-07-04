# PQC Authentication

Post-Quantum Cryptography (PQC) authentication for the Unheaded Protocol, ensuring resistance to quantum computing attacks on the control plane and service mesh.

## Internet-Draft

**draft-bellis-unheaded-pqc-authentication-00** (IETF Experimental)

## Algorithms

| Algorithm | Standard | Purpose |
|-----------|----------|---------|
| **ML-DSA** (FIPS 204) | Module-Lattice Digital Signature | Monad signature validation, service identity |
| **ML-KEM** (FIPS 203) | Module-Lattice Key Encapsulation | Key exchange for encrypted control channels |
| **SLH-DSA** (FIPS 205) | Stateless Hash-Based Digital Signature | Long-term identity keys, firmware signing |

## Integration Points

### Protocol Layer
- Monad integrity verification using ML-DSA signatures
- Kingdom Mode authentication for extended register space
- Per-hop signature validation in eBPF (shield-ebpf)

### Service Mesh
- Mutual TLS with ML-KEM key exchange between services
- Service identity certificates using ML-DSA
- Certificate rotation with post-quantum key pairs

### Infrastructure
- SLH-DSA for bare metal firmware attestation (WEST + EAST clusters)
- Container image signing with post-quantum signatures
- SBOM attestation signatures

## Test Coverage

**60 tests** covering:

- Key generation and serialization for all three algorithms
- Signature creation and verification round-trips
- Cross-implementation compatibility checks
- Performance benchmarks (latency per operation)
- Integration tests with eBPF signature verification
- Negative tests (invalid signatures, expired keys, revoked certificates)

## Migration Path

PQC authentication runs in hybrid mode alongside classical cryptography:

1. **Phase 1** (current) — PQC signatures alongside Ed25519
2. **Phase 2** — PQC-primary with classical fallback
3. **Phase 3** — PQC-only for all authentication

---

*Last updated: March 17, 2026*
