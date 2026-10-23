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
| L12b | Flood the verifier | **FINDING, fixed**: the signature was verified before the sender was authorized, and trusted public keys are public, so a caller the allowlist refuses could make every request cost an ML-DSA verify. Authorization now runs first. (Found alongside: one member per unnamed publish, never freed: 245 B/message) | bfdb194f |
| L12b | Forge, unsign, replay or re-topic a `drift.detected.<node_id>` event (signed drift.*, ADR-105, 2026-10-23) | **FINDING, fixed**: `drift.*` took any publisher's unsigned message, and Wotan dropped every signature after checking it, so no subscriber could tell a real drift alert from a forged one. Now `drift.*` is signed by its own prefix-scoped keys (`topics.signed_prefixes`, shipped with none = closed), a `config.*` key cannot sign `drift.*` and the drift key cannot sign `config.*`, HTTP refuses it, and events carry signature + `signed_at_unix_ms` for subscriber re-verification. Refused, in tests: attacker key; attacker signature with the trusted key attached; unsigned; config key on drift; replayed bytes; the same signature moved to `config.*`, to another node's topic or to another allowlisted name; re-dated. Tests: `TestLICH012_L12b_ForgedDriftRefused`, `TestLICH012_L12b_ReplayedDriftRefused`, `TestDriftE2E_SubscriberReverifies` (`services/wotan/internal/grpc/drift_e2e_test.go`) | S-QUANTUM-ARMOR Phase 12 |
| L12c | enkrateia writes the filesystem | No exploit; the test that claimed to prove it could not fail ("no assertion needed"). Now an import allowlist read from the package source fails on any `os`/`syscall`/`exec`/net import | this commit |
| L12c | TOCTOU detection → alert | Not applicable while alerts-only (ADR-043 #1): nothing acts on the alert | — |
| L12d | heimdall-bpf verifier bombs, ringbuf overflow, map poisoning | Not run (needs sudo and a live attach) | — |
| L12e | Forge a trigger | **By design, no exploit today**: triggers are unauthenticated (ADR-043), and nothing acts on one: `cmd/gjallarhorn-listener` prints, `ManifestPtr` resolves nowhere yet. heimdall-daemon verifies the manifest's seal on every scan (c1a0f22d), so a forged pointer can only name a manifest that must still carry a valid seal. The consumer that resolves `ManifestPtr` must verify before acting and rate-limit triggers | — |
| L12e | Malformed / ambiguous triggers | **FINDING, fixed**: `Unmarshal` accepted any length >= 20 and ignored the reserved bytes, so different datagrams meant the same trigger. Now exactly 20 bytes, reserved zero; `FuzzUnmarshal` requires every accepted datagram to re-marshal byte-identically (10M execs clean) | this commit |
| L12e | Integer overflow in `cluster_id` / `manifest_ptr` | No exploit: fixed-width big-endian reads of a length-checked buffer | — |

Count against the criteria below: 6 exploitable findings, all fixed on
develop; L12a key storage and L12d still to run before a verdict.

### S-QUANTUM-ARMOR Phase 14 re-run (2026-10-23, sprint tip)

| Sub | Attack | Verdict | Where |
|---|---|---|---|
| L12b | Re-run the signed-drift refusals on the merged tip: forged (attacker key, attacker signature with the trusted key attached, unsigned, config key on drift), replayed, re-topicked, re-dated | No exploit: all refused, subscriber re-verification holds (`go test -race -run 'LICH012\|DriftE2E' ./services/wotan/internal/grpc/`: 3 PASS) | `drift_e2e_test.go` |
| L12b | Strip a drift signature by having a peer node replicate the event | **FINDING (low, open)**: `replication.proto` carries no signature, key or `signed_at`, so a replicated signed event reaches the peer's subscribers unsigned. A verifying subscriber refuses it (fails closed); nothing replicates drift today. FND-020 | `compliance/findings/register.yaml` |
| L12b | Exhaust Wotan memory with signed messages the byte budget does not count | **FINDING (low, open)**: the ring budget counts `Content` only; each signed message also holds ~5.3 KB of signature and key. Needs an approved publisher of a signed prefix. FND-021 | `compliance/findings/register.yaml` |

Transport-level downgrade and evidence-pack tamper results for the same pass:
`docs/security/pq-preflight-2026-10-23.md` ("Post-sprint verification").

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
