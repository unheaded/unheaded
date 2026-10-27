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
