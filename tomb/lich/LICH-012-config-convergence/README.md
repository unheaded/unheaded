# LICH-012: Configuration Convergence Attacks

**Target**: Mímir's Law / Gleipnir Phase 0 PoC (ADR-043)
**Branch**: `spike/mimirs-law`
**Status**: OPEN (parallel with spike implementation)
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
