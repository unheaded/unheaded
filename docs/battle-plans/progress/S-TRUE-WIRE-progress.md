<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# S-TRUE-WIRE progress

Plan: `docs/battle-plans/S-TRUE-WIRE-battle-plan.md`. Step ticks live in the plan; this file holds the session log,
baselines and STUCK/ESCALATE markers. Branch `sprint/true-wire` (from develop); its merge into develop waits on E2
(wire-value renumber confirmation, Step 404).

Skip Protocol acknowledgment: "I will skip any step after 3x its estimate or 2 failed debug attempts, mark it [STUCK]
with what I tried, commit known-good state first, continue with the next step that does not depend on it, and finish
with a STUCK/ESCALATE report ordered by how many steps each item unblocks."

## Session S24, 2026-10-27 night (unattended): Phase 0

Base: develop `35baa272` (NEXT-7 Sprints 1-4: Sprint 1 shipped, 2-4 code complete; S-IRON-GATE identity and TLS are in
develop in code, so Steps 300-304 are unblocked in code).

- Tools: go, cargo, rustup, bpftool, jq, python3, xml2rfc (`~/.local/bin`), cargo-audit, docker, sudo all present;
  passwordless sudo ok. `go build ./...` 0, `go mod tidy -diff` clean.
- Go baseline (`S-TRUE-WIRE-go-baseline.txt`, run under the shared lock while two agents ran suites): 233 ok, 1 FAIL
  `pkg/secrets/store`. It passes 3/3 alone: its watch tests wait fixed 100-200 ms windows (`store_extended_test.go`,
  `file_test.go`), a load flake, not a defect of this sprint. Treat it as known; fix it in Phase 11 if time allows.
- Rust: `crates/upc-api` ok (0 tests), `crates/monad-mbc` 1 passed / 1 ignored, `ebpf` `cargo check` Finished;
  `ebpf/monad-common` has no tests of its own.
- eBPF load gate (Step 13): `[PASS] 34 eBPF programs pass the verifier; no known failures`.
- UPC regression (Step 15): ALL GREEN after building `cmd/upc-bootctl` and `crates/doom-runner` (their target dirs were
  absent, so the first run failed every xv6 check with "command not found"): ascend `verified 926542 insns` (92.7%),
  Doom `761580` (76.2%), cart_xdp 42,743, cart_tick 146,897. These seed Phase 7's ratchet.
- Step 18 (code values): `ActionForward 0x01`, `ActionDrop 0x05`, `EventBirth 0x01`, `EventAnomaly 0x08`,
  `FlagK0 0x01`, `KingdomReserved 0x03`; Rust `Birth 0x01, Hop 0x02, Death 0x03, Chaos 0x04, Anomaly 0x08`. As claimed.
- Step 20 (spec): `0x00 FORWARD`; EVENT_BORN 0x00 .. EVENT_ANOMALY 0x08; RFC-EDITOR-REQUESTS-S5 `open` count 6 (>= 5).
- Step 22 (shield): all eight patterns present; `TODO(E1)` count 2 in packet-marker; no userspace writer of the HMAC key
  map (as claimed).
- Step 24 (supply chain): `rs-no-spdx=69` (plan ~71), **`mit-spdx=4`** (plan ~290: S-CLEAN-SLATE's relicense already
  converted the MIT files, so Phase 9's relicense is mostly verification), `ledger-pending=2192`, ADR-004 sign-off
  pending 2, LICENSE line 2 `Copyright (c) 2024-2026 Stevie Bellis`, syft 1.55.0, grype 0.121.0.
- Step 25 (worktrees): lanes run as isolated agent worktrees branched from `sprint/true-wire`; the coordinator merges
  each lane back into `sprint/true-wire`.

## Phase 3 merged into sprint/true-wire (bpfschema hygiene)

Flags: bits 1:0 are `FlagCustom` (0x02) / `FlagReserved` (0x01) per foundation-06; `FlagK1`/`FlagK0` deprecated aliases
for one release; Kingdom constants deleted (no users). One CRC-16/CCITT (`encoding.CRC16CCITT`): copies in `pkg/ebpf`,
`pkg/cartridge`, `pkg/protocol/integrity`, `cmd/protocol-api/monad.go` (not in the plan) and a fuzz test helper now call
it; `crc16_single_test.go` fails on any second definition or stray `0x1021`. `rust_source_test.go` reads the Rust source
at test time (7 struct sizes/offsets, map names, key/value sizes; mutation-tested). Real drift fixed on the Go side:
`MbcCpuState` 128 -> 136 (144 with ascend-linux), packed `FlowMigrationTokenValue` 44 and `FlowCancelValue` 20 (Go and Rust
comments both claimed 48/24), `ErrorCounterKey` 4 -> 8, `HopValidatorValue` 28 -> 32. No Rust map was ever `unhd_*`:
Aya names maps after the static; the 12 constants with a Rust map now hold the real name, four map-name constants with no
Rust map deleted. No wire byte, map or eBPF object changed. Follow-ons: Phase 1 can close RFC-ALIGNMENT-FINDINGS row 2
(and note the duplicate `docs/RFC-ALIGNMENT-FINDINGS.md`); Phase 2 must update `TestActionValues`/`TestEventTypeConstants`;
`pkg/ebpf/maploader` (parked, E3) still has old sizes and `unhd_*` names; `ebpf/flow-tracker` comments still say 48/24.
M-143, M-185 resolved; sweep-internal D-11 resolved.

## Phase 1 merged into sprint/true-wire (RFC desk, b8b2b8c2); Steps 33-35 by the coordinator

Items 1-8 of `RFC-EDITOR-REQUESTS-S5.md` ruled with text in the drafts: cartridge root key `0x15` (item 1); the opcode
dictionary's `code` key moves `0x10` -> `0x16` (new item 6: `routing` has held 0x10 since 03-05); mbc-isa gains NOP, the
five ISA v2 opcodes and an "ISA v2 Instructions" section, MOV/MOVI/LOAD_IMM32 match the engines (item 7); one MBC opcode
registry, in mbc-isa (item 8). Two new drafts: anamnesis-00, protocol-overview-00 (8 drafts validate). `make validate`
had been a false green (xml2rfc 3.32 has no `--check`, so it validated nothing): it now renders each draft and fails on
errors. `ietf-submission/*.xml` are hand-maintained upload sources; the `%.xml: %.md` rule became `%.kramdown.xml`.
Step 69 overrides: `flowtype`, `sequence`, `registry` go to E3 delete rows. `docs/protocol/RFC-EDITOR-REQUESTS-TW.md`:
seven new conflicts with recommendations (TW-1 PQC algorithm IDs defined three ways; TW-2 flag bit 3 reused as Signed;
TW-6 CSR region overlaps the boot stub region; TW-7 Standards Action unusable on the Independent Stream).
Steps 33-35 (coordinator): `cmd/upc-cartridgectl` writes `cartridge/<name>` (`Namespace = "cartridge"`,
`CartridgeRootKey = 0x15`) and reads `x-cartridge/<name>` as an alias (`LegacyNamespace`); test
`TestSophiaRegistry_ReadsLegacyNamespace`. Items 2 and 3 needed no code change.
**E1 (Stevie):** accept ADR-108 now, or keep it Proposed until the cartridges run on east (S-ASCEND-2 M-228)?
**E6 (Stevie):** upload the eight `-00` drafts to the datatracker now, or after the first staging->main promotion
and/or the mbc-isa `-01` pass (TW-5 divergences remain)? **TW-7:** keep the MBC opcode registry at Standards Action or
loosen to Specification Required? **TW-1, TW-2** need a decision before the PQC values are spec'd.

## Phase 8 merged into sprint/true-wire (FND-020, Sophia signing)

Wotan: the gRPC publish path stored messages without their signature, so replication could not carry it; the stored
message keeps it now and `WALEntry` gains fields 7-11 (signature, public key, algorithm, signing time, sender). The
standby verifies with `cluster.VerifyEntry` against its own policy before applying
(`unheaded_wotan_replication_rejected_total{reason}`); `TopicVerifier.VerifyStored` checks key and signature only (no
clock window or replay cache for stored messages). Bug fixed: the standby asked for entries starting at its last applied
one and re-applied it every 500 ms. Cluster mode is not wired in `cmd/wotan` (ADR-064), so the witness is the in-process
bufconn test. Sophia: deltas get `Signer`, `Verify`, `Digest()` (the old SHA-256 "signature" is now only an identity),
`SetSignaturePolicy`; bad/untrusted/older-than-5-min signatures always refused, unsigned accepted and counted in observe
mode; `DecodeDelta` bounds-checked, `FuzzDeserialize` (60 s ~924k execs clean; coordinator 30 s clean).
`SOPHIA_DELTA_SIGNING_KEY` / `SOPHIA_REQUIRE_DELTA_SIGNATURE` exist in the package; no service reads them yet.
Follow-ups: `wotan_messages` in Postgres has no signature column (a migration); LICH-011's observe-mode pin must change
when require=true. Step 291 (register): FND-020 -> mitigated (cluster mode not wired; PGStore keeps no signature) at
the merge into develop (the register lives on develop). Draft text for sophia-dictionary-03 Security Considerations 1:
"Reference implementation pkg/protocol/sophiasync: ML-DSA-65 over SHA-256("sophia/delta-sig/v1" || 0x00 || algorithm ||
0x00 || signed_at_ms int64 BE || canonical delta); signing time within 5 minutes of the receiver's clock; unsigned
deltas run in observe mode until the severity-ladder rule flips them."

## Phase 9 merged into sprint/true-wire (SPDX, relicense)

`scripts/check-spdx.sh` (default `--langs go,rs`, `--forbid MIT` respecting MIT-KEEP), `scripts/spdx-backfill.sh`,
`scripts/lib/spdx.sh`; ci.yml spdx-check and the Jenkinsfile stage run the gate over 1870 tracked Go/Rust files. 69 .rs
headed: 61 GPL-3.0-or-later, 8 GPL-2.0-only (`ebpf/{common,flow-tracker,packet-marker,latency-probe}`,
`cmd/ebpf-loader`: their Cargo.toml declares it). py/sh: 112 missing of 463 before, 28 after (22 Doom, 6 fixtures/archive),
so CI gates Go and Rust only. Steps 331-337 ALREADY-DONE (S-CLEAN-SLATE decision 12, 279 files relicensed; dry run finds
0 MIT headers; MIT-KEEP empty; Phase 0's mit-spdx=4 were quotes in plans/scripts). THIRD_PARTY: 11 rows MIT -> GPL.
`pkg/iac/chef.go:133` writes `license 'MIT'` into generated Chef cookbooks (product question). Rust line numbers in
packet-marker shifted by 2. eBPF objects unchanged (coordinator: load gate 34 PASS, UPC 926542/761580).
**E4 (Stevie):** `cmd/trace-collector/Cargo.toml` and `crates/monad-mbc/Cargo.toml` still say `license = "MIT"` (files
are GPL-3.0-or-later): change both to GPL-3.0-or-later? The `ebpf/` workspace says GPL-2.0-only (followed by the newly
headed common/flow-tracker/packet-marker/latency-probe and ebpf-loader) while monad-common, shield, monad-cpu, cartridge,
yaldabaoth, syscall-tracer say GPL-3.0-or-later, and zhend's manifest says GPL-3.0-only against or-later headers: which
licence on each side (GPL-2.0-only for kernel-side programs, or GPL-3.0-or-later everywhere)? Keep the external
doomgeneric module MIT?
