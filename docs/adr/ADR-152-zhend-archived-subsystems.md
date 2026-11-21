<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-152: zhend's four unspawned subsystems are archived; how one comes back

**Status:** Proposed (parked under ADR-112). The move out is done and is not the proposal: Stevie ruled it on 2026-11-18. What is parked is the return of any of the four. Nothing is planned, scheduled or built.
**Date:** 2026-11-21
**Grounded on:** `develop` at `231c7f344492d649a52660007396fa0a1b147abf`.
**Related:** `docs/battle-plans/DECISION-PACK-2026-11-18.md` row 36; DP-209 (zhend is single-node and gRPC-only); ADR-021 (Zhen Layer 0); ADR-105 (hybrid key exchange, the Rust TLS provider); ADR-126 (Hop-by-Hop option registry); ADR-004 and ADR-096 (dependency admission); ADR-147 (every service earns its place); ADR-121 (the WAF, archived the same way).

## The ruling (decision pack 2026-11-18, row 36)

> zhend's four subsystems that nothing spawns (QUIC server, Monad bridge listener, topology observer, strata snapshot collector). **Move them out of the repo** to a sibling archive repo with their history; zhend keeps its single-node gRPC path; a parked ADR records how to bring one back.

## Context

`crates/zhend/src/main.rs` started the store, the gossip engine, two sedimentation loops and the gRPC server, and then held four lines: `// TODO: spawn QUIC server`, `// TODO: spawn Monad bridge listener`, `// TODO: spawn Li topology observer`, `// TODO: spawn strata snapshot collector`. The code behind each line compiled into the library and had unit tests. No binary, unit, compose service or other crate called any of it. `--quic-addr` was parsed and never used. The QUIC server alone brought quinn, rustls and aws-lc-rs into the build, and three rows of the crypto inventory existed to say "compiled, never started".

## What left, and where it is

Archive: `~/tmp/projects/unheaded-zhend-subsystems` on west, a git repository with no remote. Head `5d9663e2b715af79a37e269ce9296d91c5f6cc96` (the README commit, the only one made there). Made from the source commit above by `git filter-branch --index-filter` on a throwaway clone, the method of the role-services archive (S-WASTE-OUT Steps 22 to 32): 126 commits and 25 files of history, 127 and 26 with the README. Its history is unsigned (the rewrite strips signatures). GPL-3.0-or-later, as here.

| Subsystem | Files that left `crates/zhend/` | Tests that left | What was never written |
|---|---|---|---|
| QUIC server | `src/api/quic.rs`, `tests/phase8_quic.rs` | 9 unit, 7 integration | the call in `main.rs`; a real certificate (it generated a self-signed one at each start); any client authentication |
| Monad bridge | `src/monad/bridge.rs`, `src/monad/hbh.rs`, `src/monad/mod.rs` | 15, 4 | a listener: nothing reads a packet and calls `on_packet` |
| Topology observer | `src/li/topology.rs` | 9 | a caller of `record_access`; a bound on its maps |
| Strata snapshot history | `src/li/strata.rs`, `src/li/mod.rs` | 4 | a collector: nothing pushed a snapshot |

All 154 tests of the crate passed at the source commit, these 48 among them. The archive also holds what is needed to read the four as a unit: `lib.rs`, `main.rs` and `api/mod.rs` as they were (the wiring), `Cargo.toml` and `Cargo.lock` as they were, the crate's README, SECURITY, THIRD_PARTY, example config and NixOS module, ADR-021, ADR-105, ADR-126, the S75 plan and its overview, the PQ threat model, and `LICENSE`. It is not a buildable crate: the four import the store, the embedder and the config from the rest of zhend. Its README says how to build at the source commit.

The same code is in this repository's history up to the source commit.

## What changed here with the move

- zhend: `pub mod li`, `pub mod monad` and `pub mod quic` are gone, with `ZhenConfig.quic_addr`, the `--quic-addr` flag, `quic_addr` in the example config and `quicAddr` in the NixOS module. 106 tests remain and pass. Not touched: the gRPC service, the store, gossip, De, Jing, the ML-DSA-65 peer signatures, `proto/zhen.proto` (its `StrataSnapshot` message is the Status reply's tier counts, filled from the store, and stays).
- Dependencies: `quinn`, `rustls`, `aws-lc-rs` and `serde_json` left `Cargo.toml`. 24 packages left `Cargo.lock`, none added or changed. 16 crate names are in no lock file any more (`docs/policy/dependency-baseline.txt`, 518 to 502), `aws-lc-rs`, `aws-lc-sys` and the three quinn crates among them; the Rust component count fell from 334 to 317.
- Crypto inventory: four entries gone (109 to 105). `cmd/crypto-inventory` lost its `not-spawned` link state and the table that named `quic.rs`: nothing else used either.
- zhend now has no transport encryption. gRPC is plaintext and refuses a bind off loopback without `--grpc-allow-non-loopback`; gossip is UDP with ML-DSA-65 peer admission. No first-party Rust code serves TLS.

## Bringing one back

No subsystem returns because its code exists. It returns when a caller needs it, with Stevie's word, as its own plan. For any of the four:

1. Say who calls it and what they cannot do without it (ADR-147's first test). Write that in this ADR as a dated note and move its status.
2. Copy its files from the archive into `crates/zhend` at the same paths, and restore its `pub mod` line (`lib.rs` for `li` and `monad`, `api/mod.rs` for `quic`). Check them against the crate as it is that day: the store, embedder and config types they import may have moved.
3. Write the half that never existed (the table above) and the test for it, the test first. The call in `main.rs` goes where the TODO line stood, with a shutdown path like the gossip engine's.
4. Every value an adopter may change is a config key with a default (`ZhenConfig`, the CLI flag, `config.example.toml`, `nix/module.nix`), with a line in the wiki page that owns it.
5. `cargo build --locked` and `cargo test --locked` in `crates/zhend`; `scripts/check-cargo-locked.sh`, `scripts/check-dependency-baseline.sh`, `scripts/check-sbom-trend.sh`, `scripts/check-crypto-inventory.sh`, `scripts/check-pq-kex.sh`.
6. Bring the documents back in line: the crate's README, SECURITY and THIRD_PARTY, `CLAUDE.md`, `wiki/Rust-Components.md`, and the lines of 2026-11-21 in the root `SECURITY.md`, `THIRD_PARTY.md` and `CHANGELOG.md`.

Particular to each:

**QUIC server.** It needs three crates that are no longer admitted: `quinn`, `rustls` with the `aws_lc_rs` and `prefer-post-quantum` features, and `aws-lc-rs` (the archived `Cargo.toml` has the exact lines). Each is a new admission under ADR-096 with Stevie's recorded yes, a line in `docs/policy/dependency-baseline.txt` added by hand, and a raised `rust_components` only on his word, since that count is shrink-only. Decide first: whose certificate (the archived code generates a self-signed one and its only client config accepts any certificate), who may connect (it had no authentication, and the gRPC path it mirrors is held to loopback for the same reason), and its port (the old default 7301 is not in `pkg/ports`). It is a TLS site, so `wiki/Post-Quantum-Key-Exchange.md` "Adding a TLS site" applies: a handshake test that asserts `X25519MLKEM768` (the archive has two), the inventory regenerated, the export notice and its audit given a dated update (`docs/compliance/export-control-notice.md`). The threat model (`docs/security/zhen-pq-threat-model.md`) gets a dated update.

**Monad bridge.** Not usable as archived. `MonadContext::from_register` does not read the frozen v0x01 register: it takes the flags from byte 1, a 16-bit service id from bytes 2-3 and a flow label from bytes 6-8, where the format has the source service at 0x01, the destination service at 0x02 and the flags at 0x07 (`ebpf/monad-common/src/lib.rs`, the `OFF_*` constants). Rewrite it against the format, with the layout test first. Its option type `0x3E` is the Monad's too; ADR-126 reserves `0x1E` for Zhen and must be accepted before the option goes on any wire. Then the listener itself: a source of packets (Anamnesis, or a ring from eBPF) is a design that does not exist, and it makes zhend read live traffic, which needs the zero-user-data-access review. Its event list is an unbounded `Vec`; bound it.

**Topology observer.** Needs a caller of `record_access` on the read path (`TieredStore::get` or the gRPC handlers), a bound on its two maps, a place for the snapshot to go (the Status reply or a metric), and `serde_json` back as a direct dependency (it is still in the lock file through `tracing-subscriber`, so no new package).

**Strata snapshot history.** The smallest of the four and the only one with no new dependency: a timer in `main.rs` that reads the tier counts and pushes a snapshot, and somewhere to read the trend. The Status reply already returns the current counts, so decide first what a history adds.

## Open question

Does any of the four have a caller? None is known. No decision is made here.
