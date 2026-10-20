# LICH-012: Configuration Convergence Attacks

**Target**: Mímir's Law / Gleipnir Phase 0 PoC (ADR-043)
**Branch**: `spike/mimirs-law`
**Status**: OPEN: L12a, L12b, L12c, L12e worked (results below); L12d not run
**Lead**: BlackMage
**Opened**: 2026-04-11

## Mission

Red team the config convergence stack before it ships. Every attack must
produce either an exploit (finding) or a signed "no exploit" certification.

## Sub-Campaigns

### L12a — Baseline Signing Attack
**Target**: `pkg/gungnir/` + Wotan config.* topic signing
**Goals**:
- Forge ML-DSA-65 signature without private key (should be computationally infeasible)
- Downgrade algorithm to weaker crypto (blocked by explicit algorithm="ml-dsa-65" check)
- Replay old signed messages past expiry
- Key exfiltration from dev key storage
- Side-channel timing attacks on signature verification

### L12b — Wotan Message Injection
**Target**: `services/wotan/internal/grpc/topic_service.go`
**Goals**:
- Publish to `config.*` topics without valid signature (must be rejected)
- Spoof sender_id in canonical form
- Replay attacks on drift.detected.<node_id> topics
- Topic flooding / DoS of signing verifier

### L12c — Restore Race Conditions
**Scope**: LIMITED — alerts-only v1 (ADR-043 #1) means no restore path
**Goals**:
- Verify enkrateia NEVER calls file write syscalls (already in negative test)
- TOCTOU between drift detection and alert emission
- dm-verity bypass (Phase 8 preparation)

### L12d — eBPF Drift Detection Fuzzing
**Target**: `crates/heimdall-bpf/`
**Goals**:
- BPF verifier complexity bombs (make programs reject)
- Ringbuf overflow attacks (drop drift events)
- BPF map poisoning via unauthorized writes
- XDP program bypass via packet fragmentation

### L12e — Gjallarhorn Forgery (NEW)
**Target**: `pkg/gjallarhorn/`
**Goals**:
- Forge UPC trigger packets (magic GJLR + arbitrary manifest_ptr)
- Multicast flood on local segment
- Unauthenticated bootstrap injection (redirect fresh nodes to attacker manifest)
- Integer overflow in cluster_id / manifest_ptr parsing

## Results (2026-10-19/20, on develop)

| Sub | Attack | Verdict | Where |
|---|---|---|---|
| L12a | Replay a signed seal past its expiry | **FINDING, fixed**: gungnir signed only the payload hash, so anyone could extend a stale seal's expiry. Seals now bind alg, key id, issued, expiry and hash under ML-DSA context `unheaded/gungnir/seal/v2` | 2c554119 |
| L12a | Forge / downgrade | No exploit: verification takes only ML-DSA-65 and the v2 context; pre-v2 seals refuse | 2c554119, 3df487bb |
| L12a | Timing side channel on verify | No exploit by construction: verification handles only public data (key, message, signature) | — |
| L12a | Key exfiltration from dev key storage | Not attempted | — |
| L12b | Publish `config.*` without a valid signature | **FINDING, fixed**: the signature was checked against the key the publisher sent with it (decorative), and the HTTP path took `config.*` unsigned. Now trusted keys only (`topics.signing_keys`, empty = closed), HTTP refuses `config.*` | 472290bd |
| L12b | Replay a signed `config.*` message | **FINDING, fixed**: signed `signed_at_unix_ms`, ±5 min window, accepted-signature cache | a6a61d3c |
| L12b | Spoof sender / burn a signature with a refused publish | **FINDING, fixed**: signature recorded only after sender authz; rooms created only after authz | 169d93fc |
| L12b | Flood the verifier | Not attempted | — |
| L12c | enkrateia writes the filesystem | No exploit; the test that claimed to prove it could not fail ("no assertion needed"). Now an import allowlist read from the package source fails on any `os`/`syscall`/`exec`/net import | this commit |
| L12c | TOCTOU detection → alert | Not applicable while alerts-only (ADR-043 #1): nothing acts on the alert | — |
| L12d | heimdall-bpf verifier bombs, ringbuf overflow, map poisoning | Not run (needs sudo and a live attach) | — |
| L12e | Forge a trigger | **By design, no exploit today**: triggers are unauthenticated (ADR-043), and nothing acts on one: `cmd/gjallarhorn-listener` prints, `ManifestPtr` resolves nowhere yet. heimdall-daemon verifies the manifest's seal on every scan (c1a0f22d), so a forged pointer can only name a manifest that must still carry a valid seal. The consumer that resolves `ManifestPtr` must verify before acting and rate-limit triggers | — |
| L12e | Malformed / ambiguous triggers | **FINDING, fixed**: `Unmarshal` accepted any length >= 20 and ignored the reserved bytes, so different datagrams meant the same trigger. Now exactly 20 bytes, reserved zero; `FuzzUnmarshal` requires every accepted datagram to re-marshal byte-identically (10M execs clean) | this commit |
| L12e | Integer overflow in `cluster_id` / `manifest_ptr` | No exploit: fixed-width big-endian reads of a length-checked buffer | — |

Count against the criteria below: 5 exploitable findings, all fixed on
develop; L12a key storage, L12b flooding and L12d still to run before a
verdict.

## Success Criteria

| Findings | Verdict |
|---|---|
| 0 exploitable | HARDENED — ADR-043 may promote to "PoC Complete" |
| 1-2 exploitable | REMEDIATION REQUIRED — fix before promote |
| 3+ exploitable | HIGH RISK — ADR-043 must address architecturally before promote |

## References
- Parent ADR: `docs/adr/ADR-043-mimirs-law-upc-baseline-gleipnir-phase-0.md`
- LICH framework: ADR-042
- BlackMage hard conditions: ADR-043 §Decision #1-8
