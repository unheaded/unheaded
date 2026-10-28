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

## Phase 8 follow-up: PR #2 review findings fixed (2026-10-28)

`/code-review ultra` on PR #2 (`review/true-wire-ph8-base`..`review/true-wire-ph8`, the Phase 8 merge `88202527`)
returned six findings, all rated nit, two of them latent integrity gaps. All six fixed in one commit on the sprint branch:
1. PublishTopic refuses a non-UTF-8 payload on a signed topic (InvalidArgument): the stored Content is a string the JSON
   encoding rewrites, and a standby verifies the stored copy, so the signature would never verify again
   (`TestPublishTopic_SignedNonUTF8PayloadRefused`). Base64 storage remains the long-term fix if a signed producer ever
   needs binary payloads; today's signed producers (Heimdall, Akira) send JSON.
2. A rejected WAL entry halts replication instead of being skipped: `handleEntry` returns the `RejectError`,
   `StartReplication` ends with it, `lastAppliedSeq` stays below the entry, `halted_at_seq` in Stats. A standby that
   skipped would diverge from the primary with only a counter to show for it. **Open before cluster mode is wired
   (main.go still refuses cluster mode):** records written before signatures were persisted, and records signed by a
   retired key, now halt the first standby that replays them; a cutover sequence or a retired-but-trusted key set is
   needed first (note in `cluster/signature.go`).
3. A verified delta signed more than MaxSkew ago is applied and counted (`unheaded_sophia_delta_stale_total`), not
   refused: the version check already stops replay, and refusing a late delta wedged a receiver that was offline for
   longer than 5 minutes on the version before it (`TestApplyDelta_StaleSignatureAppliedAndCounted`). `ErrStaleDelta`
   removed. **RFC desk (Phase 1 queue):** sophia-dictionary-03 Security Considerations 2 says "reject updates with
   timestamps > 5 minutes in future/past"; proposed wording: replay protection is version monotonicity, the 5-minute
   window is logged and counted, not a refusal.
4. WALEntry fields 7-11 (the second copy of the signature) removed from `replication.proto`; the standby verifies
   `store.Message.Signed` straight from the decoded data. No `RejectMismatch`, no primary-side JSON decode per entry,
   ~5.2 KB of ML-DSA-65 material no longer travels twice. Fields were new in Phase 8, nothing shipped carried them.
   `replication.pb.go` regenerated (protoc 3.21.12; chat/topic pb left as committed).
5. `signing.go` wraps the verifier-construction error.
6. The three delta-signature logs are structured (`sophia_delta_refused` with reason, `sophia_delta_unsigned_applied`,
   `sophia_delta_stale_applied` with signed_at/now/max_skew) instead of Msgf.
Verified: `go test -race` on `services/wotan/...`, `pkg/protocol/sophiasync`, `pkg/wotan-client/...` all ok; gofmt and
vet clean. PR #2 CI's "Gates can actually fail" red is pre-existing (14 unclassified gate scripts on develop, fixed
there in `4ec0f5c4`; the sprint branch inherits it at the E2 merge). GitHub refused the review's automatic PR comment
("your account cannot comment on this pull request"); the findings are in the Round Table plan on develop.

## Phase 2 on sprint/true-wire (wire values; S25 coordinator, 2026-10-28 night)

`0d7782ba`. Started by an S24 agent that was stopped mid-edit (its WIP verified the spec's real order: DROP 0x03,
MIRROR 0x04, not the plan's guess); finished, tested and gated by the coordinator.
- Step 79/87: only hop-ebpf reads the flow-action byte; no producer writes a non-default value. The only Go decoder of
  Anamnesis bytes is `pkg/ebpf` (AnamnesisReader); the Rust trace-collector's EventType is a different enum. Nothing
  persists either byte (the Well's `event_type` columns are audit-event strings), so no migration (Step 80/88 override
  conditions not met).
- Step 84/91: as the commit says. Go `pkg/ebpf` keeps the JSON names `birth`/`hop`/`death` (dashboard packet-flow.js
  and Wotan topics read them) and accepts `born`/`computed`/`died` on input; legacy decode is per ring source
  (`AnamnesisReader.WithLegacyEventTypes`, `LegacyDecodes` counter), never guessed from the byte; a zero-timestamp record
  is refused (`ErrEventZeroTimestamp`) because an all-zero record now reads as BORN.
- Step 86/93: `go test -race` on pkg/protocol, services/sophia, pkg/ebpf, cmd/trace-collector-go,
  cmd/dashboard-backend: ok (one run of the dashboard flow-correlation e2e failed while the eBPF release build saturated
  the CPU; 3x and -race 5x clean afterwards: load-sensitive, not this change). monad-common host tests run outside the
  workspace (the ebpf workspace config forces build-std=core for the BPF target, which breaks `cargo test` there):
  copy the crate with `docs/protocol/error-registry.md` at the same relative path, then `cargo +nightly test`: 121
  (Phase 2) / 123 (Phase 4) passed.
- Step 94/95: load gate `[PASS] 34`; `upc-regression.sh` ALL GREEN, ascend 926542, Doom 761580, cart_xdp 42743,
  cart_tick 146897: identical to Step 16. A fresh worktree needs, before the regression: `cargo build --release` in
  `crates/monad-mbc`, `cmd/upc-bootctl`, `crates/doom-runner`; in `crates/xv6-mbc/upstream`
  `make -f ../adapters/Makefile.mbc clean kernel`, `make -f ../adapters/Makefile.mbc-userland ramdisk uinit gate2`
  (uinit and gate2 are separate targets; without them gate2 and the two uinit checks FAIL).
- Step 96: RFC-ALIGNMENT-FINDINGS foundation rows 4 (event types) and Sophia row 5 (flow actions) resolved;
  foundation-06 §Anamnesis Event Type Registry implementation note (decoders SHOULD accept the old numbering per source
  for one release, SHOULD NOT guess, SHOULD reject a zero timestamp).
- Step 97 (E2): open. Step 98: the pre-sprint decoder (a7fe3acd) counts and drops spec BORN and DIED without panicking,
  and misreads spec COMPUTED (0x01) as its old BIRTH: an old reader must not consume objects built after this phase.
  That is why the legacy switch sits on the new reader, not the old.

## Phase 4 on sprint/true-wire (S25 coordinator + one agent)

- Steps 131-140 `bc3b7569` (S24 agent's WIP, rebased onto Phase 2 cleanly, verified): Go `pkg/protocol/errors` and Rust
  `monad_common::ErrorCode` follow the registry, both with a parity test that reads `docs/protocol/error-registry.md`;
  Shield counts VERSION_NOT_SUPPORTED / CRC_VALIDATION_FAILED, skips CHAOS-flagged packets, no CRC under CUSTOM; TLV types
  follow foundation-06 (KEEP-AS-CONFORMANCE). Step 138: load gate `[PASS] 34`, UPC ALL GREEN, numbers unchanged.
- Steps 142-163 (agent, merged `09670867`): `pkg/wotanerr` (wotan-memory-03 32-bit code; tests read both drafts'
  tables); HTTP `X-Wotan-Error`, gRPC `x-wotan-error` trailer (not errdetails: that would make an indirect module
  direct), typed in pkg/wotan-client; reviewed by the coordinator: codes only, no topic or payload. SIGHUP reload lives
  on the shared `TopicConfig` (the plan put it on the topic service, which would have left HTTP on the old list): swaps
  `auto_approve` / `legacy_display_names` under a lock, refuses missing/malformed files and any `signing_keys` /
  `signed_prefixes` change (a new verifier would forget accepted signatures); only startup code reads the slices
  directly. Step 158 ran Wotan inside `unshare -rn`: reloads_total 1 after HUP, failures_total 1 after a bad file.
- **Step 150 ruling (for Phase 5 Step 207): `pkg/protocol/lifecycle` → E3 DELETE, and Shield's `GOAWAY_STATE` map and
  dead lookup are removed in Phase 5, not implemented. `pkg/protocol/settings` → E3 DELETE, overriding the plan's KEEP:**
  its premise ("foundation-06 names SETTINGS_* in its registries") is false (no SETTINGS in foundation-06). Recorded in
  `docs/protocol/G8-REFERENCE-RULINGS.md`; reverse that row to keep settings.
- Step 163 skipped (RFC desk, not live): WOT-09 (b) wotan-03 has no topic-signing text; (c) cross-spec drift. RFC desk
  items from the agent: wotan-03's "Normative Error Code Cross-Reference" (UNHD_*, "Foundation 18.11") contradicts
  foundation-06's registry; its bit-layout figure draws byte-wide columns its text does not describe (code follows the
  text); stale section pointers (3.2→3.3, 3.5.1); a "foundation Section 10.3 Emergency Mode" that does not exist;
  GOAWAY/SETTINGS only via the never-submitted draft-02. Finding only: Wotan does not enforce the draft's topic-name
  grammar.
- Phase 11 rows: `X-Wotan-Error` header/trailer carry the code only; SIGHUP handler from the same uid, unit unchanged,
  signing changes refused; Shield error counter: no new map, same key shape.
- Ledger edits for develop at merge time: OUTSTANDING WOT-07 done, WOT-09 (a) done, M-145 done; E3 table rows for
  `pkg/protocol/lifecycle` and `pkg/protocol/settings`; CLAUDE.md one line on `pkg/wotanerr` and SIGHUP reload.

## Phase 5 started (S25)

- Steps 169-171: FND-025 registered on develop (`7955ef3d`; register and ADR-098 live there). Step 172: the archived
  handoff and the Shield comment already said STUBBED / no writer (M-180, 2026-10-27); FND-025 added to both.
- Premises found false before coding: Shield's maps are pinned by `cmd/ebpf-loader --pin-maps` at
  `/sys/fs/bpf/unheaded/shield-ebpf/<MAP>`, not `/sys/fs/bpf/unhd_*` (Step 184); and Shield builds its HMAC/hop keys
  as `flow_id << 48 | namespace << 32` (u64), which `bpfschema.HMACKeyKey` written as raw bytes does not produce, so the
  writer mirrors Shield's key functions instead of the struct (Step 185).
- Steps 174-182: `cmd/ebpf-load-check` prints `<prog> OK verified N insns` (the kernel's verified-instruction count via
  prog info, as upc-bootctl reads it; the load gate parses field 2, unchanged). `shield_xdp_insns_pre` **1502**,
  `shield_tc_insns_pre` **1113**. STATS keys 13-15 (TC entry/IPv6/HBH, three map ops per egress packet) behind
  `shield-ebpf` feature `debug-stats`: `shield_tc_insns_post_stats` **1005** (default), 1113 (debug-stats); XDP 1502
  both. Load gate VARIANTS += `shield-ebpf:debug-stats`: `[PASS] 36`.
- Steps 206-210: GOAWAY removed per the Step 150 ruling: shield-ebpf `GOAWAY_STATE` map, `make_goaway_key` and the
  no-op lookup; Go `bpfschema.MapGoawayState` / `GoawayStateKey` / `GoawayStateValue` (parity tests follow, 11 map
  names); `pkg/ebpf/maploader.LoadGoawayState` (package unreached, E3 candidate); PATTERN_MATRIX, the interface map and
  maploader docs marked. `shield_tc_insns_post_goaway` **976** (from 1005), XDP 1502; load gate `[PASS] 36`.

### E8 (new, for Stevie or an Architect + BlackMage design pass): Shield check semantics

Steps 183-205 and the AUTHORITY half of 207 are not started, on purpose. Step 193's design does not fit the program it
targets:
- `shield_xdp` is the boundary ingress. A packet that already carries a Monad is passed through untouched at its step 4;
  for everything else the Monad and the flow label are created at steps 6-7. At the deferred block (step 4b, where the
  plan inserts the `bpf_loop`) there is no flow id to key `HMAC_KEYS` (`flow_id << 48 | namespace << 32`) or
  `HOP_VALIDATORS` on, and no Monad whose ENCRYPT flag could be tested. Only `RETRY_TOKENS` (flow id plus source address)
  could be keyed from the Shadow packet, and only if "flow id" means the arriving IPv6 flow label, which nothing says.
- `shield_tc` (egress) has a Monad, but its AUTHORITY lookup keys on `monad.qos_class` as a stand-in dictionary id
  ("Simplified") to guard "Sophia field writes" that nothing performs at egress.
- The writer (Steps 183-192) only matters once the checks exist; Step 183 also showed the plan's premises false: maps are
  pinned at `/sys/fs/bpf/unheaded/shield-ebpf/<MAP>` by `cmd/ebpf-loader --pin-maps`, and `cmd/shield` is an HTTP WAF
  daemon (ORPHAN in LIVE-PATHS, no auth on its rule API) unrelated to the eBPF object, so Step 184's own override applies:
  the writer belongs in `cmd/ebpf-loader`, which also avoids giving CAP_BPF to an unauthenticated HTTP service. And
  `bpfschema.HMACKeyKey` written as bytes does not produce Shield's u64 key; a writer must mirror Shield's key functions.
- **Question:** (a) move the per-flow integrity checks to hop-ebpf, where Kingdom packets carry a Monad on every hop,
  and keep Shield's ingress to the source blocklist and Monad stamping; or (b) define ingress keying from the Shadow
  packet (IPv6 flow label + source address) and what "HMAC key present" means before a Monad exists; or (c) delete the
  three maps and AUTHORITY from Shield until an ADR defines them. Recommendation: (a), with an ADR first; until then the
  docs say STUBBED (true) and FND-025 stays open. Building Step 193 as written would add checks that cannot fail, the
  defect FND-025 records.
- E8 addendum (found at Step 234): the PQC draft's §4 has the opposite model of the reference Shield. There, packets
  reach Shield ingress *carrying* a Monad (SigRef/KeyRef in its value field), Shield verifies and strips it at ingress,
  and the kingdom runs without it; in the code, Shield ingress *creates* the Monad and egress strips it. An E8 answer
  should say which model Shield follows: it decides where every one of the deferred checks can live.
- Steps 211-214: the cannon is `scripts/stress-cannon.py` (Python, not Go). It packed its own 20-byte layout (flags at
  the hop-count offset, made-up flow-id/TTL/seq/trace fields, no checksum): every packet an anomaly. Now the v0x01
  layout with CRC-16/CCITT-FALSE over 0x00-0x11 big-endian at 0x12, seq in the scratch registers, CUSTOM never set,
  flow action FORWARD; `--bad-crc` for the negative test. `tests/scripts/test_stress_cannon.py` (5 tests, independent
  CRC reference, check value 0x29B1) runs in Static Analysis "script unit tests (GATING)".
- Steps 215-223: packet-marker trace-ID extraction. Calling the existing pointer-walk extractors (they already had the
  `black_box(options_end)` bound) is still rejected: `R2 offset is outside of the packet`. Step 216's bpf_loop design
  does not help (a callback cannot hold packet pointers). Instead `extract_trace_option` copies the at-most-40-byte option
  area onto the stack with `bpf_xdp_load_bytes` and parses it with bounded indices (TCP kind 253 first, then IPv4 type
  0x1F). `packet_marker_insns_post` **5838** (from 1195); load gate PASS. Lab netns (own namespace `twlab`, veth
  `twlab0/1`, cmd/ebpf-loader on the root side, synthetic packets only): 50 SYN with the TCP option, 50 without, 20 UDP
  with the IP option → `trace_extracted = 70`, tcp 100, udp 20; netns, veth and loader gone afterwards. The collector
  check (Step 221's curl) was not run: no collector in the lab, the counter alone per the step. packet_marker stays
  IPv4-only; nothing writes TRACE_INJECT. wiki/eBPF-Programs.md and next.md corrected.
- Steps 224-227 (Phase 5 close): load gate `[PASS] 36`; UPC ALL GREEN 926542/761580/42743/146897 (Step 16 numbers);
  `make test-ebpf-compat` passed; whole-tree go test 235 ok, no new FAIL. Steps 228/229 held open by E8 (FND-025 stays
  open; the docs already say STUBBED, which is true).
- Steps 231-237 (PQC_SIG_STATUS): removed from shield-ebpf with `pqc_fast_path_check` and the result enum; shield no
  longer depends on `ebpf/pqc-common`, which loses its status enum and map entry (8 host tests pass in an
  out-of-workspace copy; nothing consumes the crate now: listed for E3). Shield XDP 1502 / TC 976 unchanged; load gate
  `[PASS] 36`; no PQC_SIG_STATUS in any .rs or .go. Docs: wiki/PQC-Authentication.md row, the 2026-08-03 security
  write-up marked resolved by removal, and an implementation-status note in the PQC draft (both copies and the XML;
  xml2rfc renders). INNOVATIONS.md and SPEC-VS-CODE-AUDIT.md already said "not implemented". This executes decision
  43's default on the branch; Stevie can still choose "wire OPTIMISTIC" (revert this commit).
- Steps 259-277 (Phase 7, the verifier ratchet): `cmd/ebpf-load-check` already printed `OK verified N insns` (Step 174,
  prog info). `check-ebpf-loads.sh` writes `<elf>/<program>\t<variant>\t<insns>` to `EBPF_INSNS_OUT`; new
  `scripts/check-ebpf-insns.sh` (growth > 10 %, over 900,000 unless in `docs/policy/ebpf-insns-over-budget.txt`
  (shrink-only, ascend 926,542 the one entry), membership both ways, refuses a baseline from another kernel with exit 2);
  `docs/policy/ebpf-insns-baseline.tsv` seeded: 36 rows (ascend 926542, Doom 761580, cart_xdp 42743, cart_tick 146897,
  packet_marker 5838, shield XDP 1502 / TC 976 / TC debug-stats 1084). Provoked: shield rows -20 % → FAIL +25 %;
  allowlist emptied → FAIL ascend; a missing program → FAIL. `bpf-verifier-check.sh` prints the baseline instead of
  bytes/8 (`SIZE / 8` gone; GATE PASSED). CI (`ebpf.yml`): the load gate writes the counts and a step publishes them
  next to the baseline in the job summary; not gating, because GitHub's kernel and its unpinned bpf-linker make them
  incomparable (comment in the workflow). Gating on west: `scripts/ci-local.sh` step 11c and the meta-gate
  (`check-ebpf-insns`, slow). ADR-012 status and wiki/eBPF-Programs.md "Verifier gates" updated; MARSHAL-COOKBOOK has
  no gate count to change.
