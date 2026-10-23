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

## In the code

| Piece | What it does |
|-------|--------------|
| `pkg/crypto/pqc/` | ML-DSA, ML-KEM, SLH-DSA, FN-DSA, HQC and a KEM tunnel on `github.com/cloudflare/circl` |
| `services/wotan/internal/signing/` | ML-DSA-65 required on `config.*` topics, trusted keys only, replay window ([[Wotan Topic Signing|Wotan-Topic-Signing]]) |
| `cmd/pqc-verifier` (`:19008`) | `POST /verify` (PQC-authenticated packet), `POST /verify/sovereign` (2-of-3 multi-sig) |
| `ebpf/shield-ebpf` | Fast path: reads cached verification status from the `PQC_SIG_STATUS` map; eBPF does not verify signatures itself |
| `services/sophia/kem_registry.go`, `pkg/pqc_policy/` | KEM algorithm registry and NIST-level policy |

## Specified, not built

- Service mTLS (`pkg/transport/mtls`, minimum TLS 1.2) sets no post-quantum policy. Go's TLS 1.3 default key exchange includes X25519MLKEM768 when `CurvePreferences` is unset, so Go-to-Go TLS 1.3 links get hybrid ML-KEM from the standard library, not from Unheaded code.
- No PQ service certificates, firmware attestation, container image signing or SBOM attestation. Release signing is a placeholder (`release.yml`).

## Tests

`go test ./pkg/crypto/pqc/` covers the algorithm registry, parameter sets, signature binding (pass, fail, ML-DSA), constant-time hash-prefix checks, KEM tunnel key derivation, and benchmarks (`pqc_bench_test.go`). Wotan's signing tests cover tamper, untrusted-key and replay rejection.

## Migration Path

PQC authentication runs in hybrid mode alongside classical cryptography:

1. **Phase 1** — PQC signatures alongside classical ones (today: ML-DSA-65 on `config.*` only)
2. **Phase 2** — PQC-primary with classical fallback
3. **Phase 3** — PQC-only for all authentication

---

