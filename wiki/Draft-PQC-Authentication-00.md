# draft-bellis-unheaded-pqc-authentication-00

**Status:** IETF Experimental · Independent Submission · March 2026

This document defines a multi-algorithm, dual-layer, tiered post-quantum cryptographic (PQC) authentication architecture for the Unheaded Protocol. Layer 1 (wire-level, REQUIRED) stores full PQC signatures in Sophia BPF maps via a "signature-by-reference" scheme — the Monad register carries compact 12-byte references (SigRef, KeyRef, SeqNum, HashPfx), while Shield verifies at the network perimeter and strips Monad headers at ingress so internal kingdom traffic carries zero PQC wire overhead. Layer 2 (application-level, OPTIONAL) allows user applications to define their own verification requirements via Sophia policy dictionaries. Four compliance tiers (NONE, STANDARD, ENHANCED, SOVEREIGN) are signaled via Kingdom Mode bits in the Monad flags byte.

## Key Sections

- **Introduction** — Motivation (quantum threat to infrastructure); NIST PQC standards covered (FIPS 203-207); signature-by-reference scheme enabling 20-byte Monad constraint; benefits: zero wire overhead increase, amortized verification cost, algorithm agility, tiered compliance, clean perimeter isolation
- **Terminology** — SLH-DSA (FIPS 205, hash-based, eBPF-native), ML-DSA (FIPS 204, lattice, eBPF-native), FN-DSA (FIPS 206, lattice, userspace signing required), ML-KEM (FIPS 203, key establishment), HQC (FIPS 207, code-based KEM), Compliance Tier, Signature-by-Reference, SigRef/KeyRef/SeqNum/HashPfx, Pseudo-Header, Verification Policy (PESSIMISTIC/OPTIMISTIC), Layer 1, Layer 2, Application Policy Dictionary, Header Stripping
- **Protocol Overview** — Four phases: (1) Key/Signature Provisioning into Sophia BPF maps; (2) Packet Marking (populate Monad value field at Shield ingress); (3) Wire-Level Verification at XDP (fast path: cached lookup; slow path: async userspace verifier); (4) Application-Level Policy (optional Layer 2 after header stripping)
- **Monad Value Layout for PQC Authentication** — 12-byte layout: SigRef[23:0] (3 bytes) | KeyRef[23:0] (3 bytes) | HashPfx (2 bytes) | SeqNum (4 bytes); field definitions and reject conditions; dual-use of S flag with Kingdom Mode bits
- **Sophia PQC Map Structures** — `sophia_pqc_sigs` (BPF_MAP_TYPE_HASH, BPF_F_RDONLY_PROG, max 1M entries): `sophia_pqc_sig_entry` with algo_id, verified status, sig_len, flow_label, verify_timestamp, variable-length signature; `sophia_pqc_keys` (max 65K entries): `sophia_pqc_key_entry` with algo_id, key_epoch, key_len, SHA-256 fingerprint, variable-length pubkey; full algo_id registry table (SLH-DSA 0x01-0x0C, ML-DSA 0x10-0x12, FN-DSA 0x20-0x21, ML-KEM 0x80-0x82, HQC 0x90-0x91)
- **Signed Pseudo-Header** — 52-byte structure: Source IPv6 (16B) + Destination IPv6 (16B) + Flow Label (4B, 12 bits zeroed) + Source Port (2B) + Destination Port (2B) + SeqNum (4B); excludes mutable fields (hop_count, flags, CRC-16)
- **Verification Pipeline** — Fast path (cached): SigRef lookup → flow label check → HashPfx compare → SeqNum replay check → forward; Slow path (async): ring buffer write → policy-based forward/hold → userspace verifier daemon (retrieve sig+key, construct pseudo-header, execute algorithm verify, update map); verifier daemon health check (5s interval, 10s failure timeout, auto-restart)
- **Verification Policies** — PESSIMISTIC (hold until verified, recommended for untrusted ingress); OPTIMISTIC (forward immediately, tear down on failure, 1-5ms risk window); default PESSIMISTIC when unconfigured; tier determines default (SOVEREIGN → PESSIMISTIC)
- **Compliance Tiers** — NONE (K1=0,K0=0: no PQC processing, zero overhead); STANDARD (K1=0,K0=1: SLH-DSA only, Layer 2 off, OPTIMISTIC default); ENHANCED (K1=1,K0=0: all three sig algos, Layer 2 optional, OPTIMISTIC default); SOVEREIGN (K1=1,K0=1: 2-of-3 multi-algo cross-verification, Layer 2 MANDATORY, PESSIMISTIC default, audit events); tier transitions via control plane (zero-downtime hot-reload)
- **Key Lifecycle Management** — Key generation (CSPRNG per NIST SP 800-90A; HSM recommended for FIPS 140-3); key rotation (new KeyRef with incremented epoch, 60-second grace period); key revocation (key_revoke flow action 0x12, immediate invalidation of all associated SigRefs)
- **Wotan Integration** — Per-flow PQC state at reserved addresses 0x0000FF00-0x0000FF27: last_seen_seq, pqc_verified, pqc_algo_id, pqc_key_epoch, pqc_verify_count, pqc_fail_count, pqc_key_fp, pqc_verify_ts, pqc_key_created, pqc_app_policy_id; sequence number management with CAS retry (max 3 attempts)
- **Shield Processing Rules (Header Stripping)** — Ingress (untrusted→kingdom): validate CRC, verify signature, persist Wotan PQC state, strip HbH header, forward; egress (kingdom→external): re-stamp Monad with fresh SigRef/KeyRef/SeqNum+1/HashPfx; internal transit: no Monad headers, no repeated PQC verification, Wotan state is authoritative
- **Application-Level Policy Verification (Layer 2)** — `sophia_pqc_app_policy` map with `sophia_pqc_policy` struct (min_security_level, require_pinned_key, max_key_age_sec, allowed_algos[12], pinned_fp[32]); 8-step verification procedure; Layer 2 independence (no dependency on wire headers); Sophia dictionary definition format example
- **Multi-Algorithm Considerations** — eBPF compatibility matrix (SLH-DSA: verify YES; ML-DSA: verify YES; FN-DSA: verify PARTIAL, sign NO due to float); FN-DSA signing daemon requirements (process isolation, constant-time, randomized signing, entropy validation, Unix domain socket auth); algorithm negotiation; Sovereign multi-signature layout (`sophia_pqc_sovereign_entry` with consensus bitfield)
- **KEM Integration** — ML-KEM/HQC for Shield-to-Shield tunnel key establishment only (not per-packet); 3-step encapsulation/decapsulation/HKDF-SHA256 key derivation; algo IDs in 0x80-0x9F range
- **Security Considerations** — 15 threat analyses: signature-by-reference trust model (PQC map write protection critical); replay resistance (SeqNum in pseudo-header, 64-packet sliding window); HashPfx collision probability (1/65,536, optimization only); async verification window; memory exhaustion (LRU eviction, rate limiting); CRC-16 not cryptographic; side-channel considerations; quantum security level selection; header stripping perimeter isolation; dual-layer security properties; FN-DSA floating-point side channels (PQC-009, Eurocrypt 2025 attack); userspace-kernel boundary TOCTOU (PQC-010); algorithm confusion/downgrade (PQC-011); FN-DSA entropy requirements (PQC-013); compliance tier security boundaries
- **IANA Considerations** — PQC Algorithm Registry (Specification Required, 33 initial entries); Monad Flags Registry update (S flag semantics with Kingdom Mode); Sophia Map Type Registry update (5 new PQC map types 0x10-0x14)
- **Appendices** — Algorithm Parameter Set Selection Guide (7-row comparison table); Performance Analysis (fast path ~215-265ns, slow path ~1-5ms, memory ~8.1 KB per flow); Test Vectors (deferred to future revision)

## Related

- [[PQC Authentication|PQC-Authentication]]
- [[Protocol Foundation|Protocol-Foundation]]
- [[Draft Protocol Foundation 06|Draft-Protocol-Foundation-06]]
- [[Sophia Dictionaries|Sophia-Dictionaries]]
- [[Draft Sophia Dictionary 03|Draft-Sophia-Dictionary-03]]
- [[Wotan Memory Model|Wotan-Memory-Model]]
- [[Draft Wotan Memory 03|Draft-Wotan-Memory-03]]
- [[Drafts Index|Drafts-Index]]

---

> **Source:** [docs/protocol/draft-bellis-unheaded-pqc-authentication-00.md](../docs/protocol/draft-bellis-unheaded-pqc-authentication-00.md)
