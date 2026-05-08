# Cargo Audit Wave C — Disposition (2026-05-08)

**Predecessor:** `docs/security/cargo-audit-2026-05-07.md` Wave C (unmaintained-but-functional cleanups).
**Auditor:** Marshal-led drain shift, post-Wave-A + Wave-B.
**Status:** PARKED with structured audit trail. No code changes land in Wave C.

---

## Why Wave C is parking-lot, not patch-and-ship

The 2026-05-07 audit named four advisories as "Wave C drop-in / hygiene":

| Advisory | Crate | Wave C plan | Reality on 2026-05-08 |
|----------|-------|-------------|----------------------|
| RUSTSEC-2025-0141 | bincode 1.3.3 | bincode 2 (breaking) | **Wire-format breaking** — zhend stores fragments to sled + ships them over gossip. bincode 2 default config produces different bytes than bincode 1; would break existing sled DBs and gossip peer compatibility. Requires migration plan with forward/backward compat. |
| RUSTSEC-2025-0057 | fxhash 0.2.1 | fxhash → rustc-hash drop-in | **Transitive via sled 0.34.7** — zhend doesn't import fxhash directly. sled owns the dep; can't replace without bumping sled. |
| RUSTSEC-2024-0384 | instant 0.1.13 | bump parking_lot | **Transitive via sled → parking_lot 0.11.2** — same chain. parking_lot 0.11 is the last sled-compatible major; 0.12 would require sled bump. |
| RUSTSEC-2025-0134 | rustls-pemfile 2.2 | rustls-pki-types drop-in | **Transitive via tonic 0.12** (just landed in Wave A) — tonic 0.12 still uses rustls-pemfile 2.2 for legacy PEM parsing. Closing requires tonic 0.13+ minor bump (out of scope for Wave A's reach). |

**Plus one new advisory surfaced in Wave B:**

| Advisory | Crate | Path | Disposition |
|----------|-------|------|-------------|
| RUSTSEC-2024-0436 | paste 1.0.15 (proc-macro) | pqcrypto-mldsa → paste | Transitive via the pqcrypto-mldsa crate that landed in Wave B. paste is a proc-macro author abandonment; resolving requires upstream pqcrypto-mldsa to swap macros. Not actionable on our side. |

---

## The real bottleneck: sled 0.34.7 unmaintained

Three of the four Wave C advisories (fxhash, instant transitively-via-parking_lot, and arguably part of the bincode 1.x lifecycle decision) chain through **sled 0.34.7**, last published 2022-08-04. The sled author (spacejam) has stated publicly that sled 1.0 will be a from-scratch rewrite (project "komora"); the current `1.0.0-alpha.*` line is not yet API- or storage-compatible with 0.34, and is not stable.

**Practical consequence:** clearing the bulk of Wave C requires either:

1. **Migrate zhend off sled** to a maintained alternative (`redb`, `fjall`, `heed`, or a custom WAL on top of `rocksdb`). Architect-scope decision; ~2-3 days of work; touches `crates/zhend/src/jing/storage.rs` + the entire L1/L2/L3 sedimentation path. Driving consideration: GPL-3.0 compatibility, `no_std` not required.
2. **Wait for sled 1.0 stable.** No published roadmap; could be months or years.
3. **Accept the transitive advisories.** Document that sled-chain advisories are tracked but not actionable until item 1 happens.

**Marshal recommendation:** option 3 short-term + open a forward-looking ADR for option 1 (sled migration) when the next storage-touching sprint kicks off.

---

## What landed in this audit pass

- **No code or Cargo.toml edits.** This document is the deliverable.
- The 4 Wave C advisories are now annotated with their actual blockers. Future audit shifts won't re-attempt the same patches and re-discover the same blockers.
- The new paste advisory is captured.

---

## Daytime recipes (when each becomes actionable)

### bincode 1 → 2 (when storage-format break is acceptable)
1. Author ADR for the migration. Specify wire-format strategy: tagged version byte? dual-read with version probe?
2. Edit `crates/zhend/Cargo.toml`: `bincode = { version = "2", features = ["serde"] }`.
3. Replace 6 call sites in `src/pu/codec.rs`, `src/pu/store.rs`, `src/qi/message.rs`:
   - `bincode::serialize(&v)` → `bincode::serde::encode_to_vec(&v, bincode::config::standard())`
   - `bincode::deserialize(b)` → `bincode::serde::decode_from_slice(b, bincode::config::standard()).map(|(v, _)| v)`
4. **OR** for wire-compat: use `bincode::config::legacy()` (matches bincode 1 byte-for-byte).
5. Run all storage roundtrip tests + gossip integration tests.

### sled migration (when storage layer touches the dock)
1. Author ADR (this is non-trivial — touches durability semantics, txn API, cursor API).
2. Spike with `redb` first — it's the closest API match; ACID; `no_std` optional; MIT.
3. Migration utility: read sled 0.34 trees, emit redb DBs.
4. Backwards-compat fallback for reading old DBs at first startup.

### tonic 0.12 → 0.13 (closes rustls-pemfile chain)
1. Audit tonic 0.13 release notes for breaking changes.
2. Likely API-stable for our small surface (Channel, Endpoint, Status, Code).
3. Single-commit minor bump per ADR-066 precedent.

### paste advisory
1. Track upstream pqcrypto-mldsa for paste removal. No action on our side.
2. Or: file an issue with rustpq/pqcrypto.

---

## Audit-trail attestation

The 4 Wave C advisories are tracked as KNOWN-NOT-FIXED in this shift, with explicit blockers documented. They remain in `cargo audit` output until the daytime recipes above land. This is **structured deferral**, not silent acceptance.

Per ADR-052, this document IS the in-tree source-of-truth for Wave C status until superseded.
