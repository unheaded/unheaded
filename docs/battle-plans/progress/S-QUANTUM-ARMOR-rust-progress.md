<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->

# S-QUANTUM-ARMOR: Phase 7 progress (Rust TLS remediation + handshake tests)

Plan: `docs/battle-plans/S-QUANTUM-ARMOR-battle-plan.md`, Phase 7, Steps 141-158.
Track: KEX-Rust, own worktree, branched from develop at `424edf47`.
Decisions implemented: ADR-105 (written in parallel by the Phase 2 agent from the
plan's pre-seeded [DECIDE]s): Step 36 profiles (the WAF and zhend QUIC are both
reached by peers we do not build, so both use the **Preferred** profile: hybrid
first, classical fallback), Step 38 (aws-lc-rs provider, ring dropped, unused TLS
features dropped).

Skip Protocol acknowledged: I will skip any step after 3x its estimate or 2 failed
debug attempts, mark it [STUCK] with what I tried, commit known-good state first,
continue with the next step that does not depend on it, and finish with a
STUCK/ESCALATE report ordered by how many steps each item unblocks. [ESCALATE]
steps are never attempted; they are reported.

## Preflight for this track (Phase 0 rows that gate Phase 7)

| # | Verdict | Evidence |
|---|---|---|
| H6 | TRUE | WAF `Cargo.lock`: rustls 0.23.46 with features `ring` (Cargo.toml) and `aws_lc_rs` (tokio-rustls defaults); no `install_default`/`builder_with_provider` in `src/`. The new `build_server_config_does_not_panic` test panicked before the fix: "Could not automatically determine the process-level CryptoProvider from Rustls crate features". |
| H7 | TRUE | `crates/zhend/src/api/quic.rs` built both rustls configs on `rustls::crypto::ring::default_provider()`; ring has no ML-KEM, so every QUIC session was classical. |
| H8 | TRUE | `cmd/trace-collector/src`: no `ClientTlsConfig`/`ServerTlsConfig`/`https://`. `cmd/upc-bootctl/src/tty_bridge.rs`: only `http://<host>:<port>/api/v1/tty/ingest`. |
| H18 | TRUE | rustls 0.23.46 `crypto/aws_lc_rs/mod.rs` `DEFAULT_KX_GROUPS`: `X25519MLKEM768` first under `prefer-post-quantum`, then X25519, P-256, P-384. Confirmed by the handshake tests. |

## Baseline (before any change, at `424edf47`)

Tests (`cargo test --quiet`):

| workspace | passed | failed | ignored |
|---|---|---|---|
| `cmd/waf` | 53 (52 lib + 1 bin) | 0 | 0 |
| `crates/zhend` | 161 (133 lib + 28 integration) | 0 | 0 |
| `cmd/trace-collector` | 160 | 0 | 1 |
| `cmd/upc-bootctl` | 25 | 0 | 0 |

zhend's doctest pass in the baseline run was compiled after the Step 148 source edit
had landed (the 6-minute `phase2_exit_gate` test was still running), so it failed to
compile against the old lib; it is not a baseline failure. The post-change run has 0
doctests, the same as the base (no doc example was touched).

`cargo audit --file <424edf47 Cargo.lock>` (advisory DB of 2026-10-23): 0 vulnerabilities in all four.
Warnings, all "unmaintained": waf `RUSTSEC-2025-0134` (rustls-pemfile); trace-collector
`RUSTSEC-2025-0134` (rustls-pemfile, via tonic `tls`); zhend `RUSTSEC-2024-0384` (instant),
`RUSTSEC-2024-0436` (paste), `RUSTSEC-2025-0057` (fxhash), `RUSTSEC-2025-0141` (bincode),
`RUSTSEC-2026-0161/0162/0163/0166` (pqcrypto-*); upc-bootctl none.

Clippy baseline (`docs/security/clippy-baseline.txt`): 0 lines for the four workspaces.

## Steps

### 7.1 WAF (`b6822071`)

- [x] **141** [W][TEST] `cmd/waf/src/tls.rs` tests: `build_server_config_does_not_panic`,
  `handshake_negotiates_x25519mlkem768`, `handshake_with_tls12_floor_still_negotiates_x25519mlkem768`,
  `classical_only_client_falls_back_to_x25519`. Test-time Ed25519 cert (zhend's DER
  builder, on aws-lc-rs); in-memory `ServerConnection`/`ClientConnection` pump;
  client trusts the cert through a `RootCertStore`. `aws-lc-rs` dev-dependency (no
  default features; already in the lock graph). Plus `test_tls_min_version_defaults_to_1_3` in `config.rs`.
- [x] **142** [TEST] Red: 4 failed, each with the H6 panic ("Could not automatically
  determine the process-level CryptoProvider from Rustls crate features").
- [x] **143** [CODE] rustls / tokio-rustls on `aws_lc_rs` (+ `prefer-post-quantum`,
  `tls12`, `logging`, `std`), no `ring`; `ServerConfig::builder_with_provider(aws_lc_rs::default_provider())
  .with_protocol_versions(..)` with the error mapped to `TlsError::ConfigError`.
  Default `min_version` was "1.2": now "1.3" (help text and example config too).
- [x] **144** [TEST][V] `cargo test`: 57 lib (+5) + 1 bin, 0 failed. `cargo clippy --all-targets -- -D warnings`: clean.
  `cargo fmt --check`: clean.
  - Falsification (not in the plan, the Phase 6 Step 126 pattern): provider pinned to
    `kx_groups = [X25519]` turned both hybrid tests red (`left: X25519, right: X25519MLKEM768`); reverted.
- [x] **145** [D] not needed: aws-lc-sys built with the system gcc.
- [x] **146** [V] `cargo tree -i ring --target all`: nothing to print. `ring` stays in
  `Cargo.lock` as an unbuilt entry: `rustls-webpki` declares `alloc = ["ring?/alloc"]`,
  and cargo keeps weak optional dependencies in the lockfile.
- [x] **147** [C] `b6822071`.

### 7.2 zhend QUIC (`723f5b0c`)

- [x] **148** [CODE] `zhen_tls_server_config()` / `zhen_tls_client_config_insecure()`
  factored out; both on `aws_lc_rs::default_provider()`. Self-signed Ed25519 cert on
  `aws_lc_rs::signature` / `aws_lc_rs::rand`. `ring = "0.17"` replaced by `aws-lc-rs`
  (no default features); rustls default features off (`aws_lc_rs`, `prefer-post-quantum`,
  `std`, `logging`); quinn default features off (`log`, `runtime-tokio`, `rustls-aws-lc-rs`,
  `bloom`). `platform_verifier` had no user in `src/`, so it went too.
  The pre-commit hook asked for an SPDX header on `quic.rs`; added in the crate's form
  (`// SPDX-License-Identifier: GPL-3.0-or-later`, as `lib.rs`).
- [x] **149** [W][TEST] `tls_negotiates_x25519mlkem768`, `tls_classical_only_peer_falls_back_to_x25519`
  (quinn does not expose the group, so the two rustls configs the endpoints are built
  from are handshaken in memory). Falsified as in 144: red with `[X25519]`, reverted.
- [x] **150** [TEST][V] 163 passed (baseline 161 + 2), 0 failed; the 7 QUIC round-trip
  tests in `tests/phase8_quic.rs` pass on the new provider. clippy `--all-targets -D warnings` clean.
  `cargo tree -i ring`: nothing to print for the host; with `--target all` it appears
  only as quinn-proto's `wasm32-unknown-unknown` target dependency.
  Cargo.lock: 16 packages removed, none added, no version changes.
- [x] **151** [C] `723f5b0c`.

### 7.3 Unused TLS features

- [x] **152** [CODE] `cmd/trace-collector`: tonic `["gzip"]` (no `tls`). `cmd/upc-bootctl`:
  reqwest `["json", "blocking"]` (no `rustls-tls`).
  - Found while doing it: with tonic's `tls` feature, an `https://` endpoint with no TLS
    config fails ("Connecting to HTTPS without TLS enabled"); without the feature, tonic
    dials it as cleartext h2 and says nothing. The `--wotan-endpoint` / `WOTAN_ENDPOINT`
    value is user-supplied, so dropping the feature would have turned a loud failure
    into silent plaintext. Added `require_plaintext_scheme()` in
    `src/publisher/grpc.rs`, red first (3 tests failed to compile), then green: anything
    but `http://` is refused before connecting.
  - reqwest without TLS refuses `https://` by itself, and upc-bootctl only builds `http://` URLs.
- [x] **153** [BUILD][TEST][V] trace-collector 163 passed (160 + 3), 0 failed, 1 ignored (as
  baseline); upc-bootctl 25 passed. `cargo tree -i rustls`: no match in either.
  clippy `--all-targets -D warnings` clean for both (trace-collector also prints cargo's
  pre-existing `unused build dependency tonic-build` notice: not clippy, not from this change).
  Lockfiles: removals only (trace-collector: rustls, ring, tokio-rustls, rustls-pemfile, ...;
  upc-bootctl: rustls, ring, quinn*, hyper-rustls, webpki-roots, ...).

### 7.4 Compliance of the dependency change

- [x] **154** [COMPLIANCE] `scripts/verify-gpl-boundary.sh`: PASS (rc=0).
  `scripts/check-dependency-baseline.sh`: rc=1 with 17 packages gone from the build
  (combine, core-foundation, hyper-rustls, jni, jni-macros, jni-sys, jni-sys-macros,
  openssl-probe, rustls-native-certs, rustls-platform-verifier,
  rustls-platform-verifier-android, schannel, security-framework, security-framework-sys,
  simd_cesu8, simdutf8, webpki-root-certs); `--update` (shrink-only) → 747 packages,
  rc=0; diff is 17 removals, 0 additions. `ring`, `rustls`, `quinn` etc. stay in the
  baseline because other workspaces still lock them.
  Licence check of aws-lc-sys: its SPDX expression and `aws-lc/LICENSE` contain no
  OpenSSL or SSLeay licence (the OpenSSL/SSLeay-derived code is under Apache-2.0), so no [ESCALATE].
  Admission record: ADR-004 register row + "Admission — `aws-lc-rs` 1.18.2 / `aws-lc-sys`
  0.46.0" (ADR-096 §2 table; owner sign-off pending).
- [x] **155** [SECURITY] `cargo audit` on the four changed lockfiles: 0 vulnerabilities,
  0 yanked (baseline also 0). Warnings unchanged except trace-collector loses
  RUSTSEC-2025-0134 (rustls-pemfile). Cooldown: no version changed; aws-lc-rs 1.18.2 /
  aws-lc-sys 0.46.0 were published 2026-10-12, 11 days before.
- [ ] **156** [DEFERRED: lead regenerates after Phase 4 merges] `scripts/check-pq-kex.sh`,
  `cmd/crypto-inventory` and `docs/policy/pq-kex-baseline.txt` do not exist on this base
  (`424edf47`). When they land, the lint lines this phase resolves are the Rust ones:
  `cmd/waf/... rust-ring-provider` and `crates/zhend/... rust-ring-provider`; the CBOM
  should then show the WAF and zhend QUIC as `hybrid` (Preferred profile) and
  trace-collector / upc-bootctl with no TLS asset at all (was `unused-feature`).
- [x] **157** [C] `b9d457d9`.
- [x] **158** [V] **PHASE 7 EXIT GATE**: passed.
  - `git grep -n 'crypto::ring::default_provider' -- '*.rs'`: no output (no `ring =`
    dependency or `"ring"` rustls feature left in any `Cargo.toml` either).
  - `./scripts/check-clippy.sh`: `PASS: clippy 0/0 warnings, 0 errors.`, rc=0, all 19
    workspaces (including the four touched here).
  - Both rustls handshake tests assert `X25519MLKEM768` and were seen red on a
    classical group; audit clean (Step 155).

## Negotiated groups proven

| service | default peer | classical-only peer | test |
|---|---|---|---|
| WAF (`cmd/waf`, min TLS 1.3 and 1.2) | `X25519MLKEM768` | `X25519` (accepted edge fallback, Preferred profile) | `tls::tests::handshake_negotiates_x25519mlkem768`, `..._tls12_floor_...`, `classical_only_client_falls_back_to_x25519` |
| zhend QUIC (`crates/zhend`) | `X25519MLKEM768` | `X25519` (peers outside the Kingdom, Preferred profile) | `api::quic::tests::tls_negotiates_x25519mlkem768`, `tls_classical_only_peer_falls_back_to_x25519` |
| trace-collector, upc-bootctl | no TLS compiled in | n/a | `publisher::grpc::tests::test_https_*` (https:// refused, never silently cleartext) |

These are in-memory rustls handshakes (the same provider and config the services
build). No independent-implementation probe (OpenSSL `s_client`) was run against a live
WAF: the WAF needs a cert on disk and a running process, which Phase 7 does not ask
for. Phase 6's `scripts/probe-tls-groups.sh` can do it once it lands.

## Definition of Done (Phase 7)

| item | status |
|---|---|
| DoD-STD 1: every step executed or logged | 141-155, 157-158 done; 156 DEFERRED (lead) |
| DoD-STD 2: every [V] and the exit gate passed | yes (above) |
| DoD-STD 3: zero new failures vs baseline | yes: waf 53→58, zhend 161→163, trace-collector 160→163, upc-bootctl 25→25; 0 failed everywhere |
| DoD-STD 4: clippy `-D warnings` on touched crates | clean (`--all-targets`), and `check-clippy.sh` 0/0 |
| DoD-STD 5: commit after the gate | the commit that records this section |
| DoD-STD 6: no new dependency without the check | no package added to any lockfile; aws-lc-rs/aws-lc-sys recorded in ADR-004; baseline 17 lines smaller |
| DoD-STD 7: attack surface | no new listener; test keys live in memory only; trace-collector can no longer send to an https:// URL in cleartext |
| DoD-STD 8: zero user data access | handshake metadata only |
| `cargo tree -i ring` empty for WAF/zhend | yes for the host target (lock-only entries explained in 146 and 150) |
| dependency baseline smaller | 764 → 747 |
| licence gate green | `verify-gpl-boundary.sh` PASS |

## STUCK / ESCALATE

None. No [STUCK] step; no [ESCALATE] condition hit (aws-lc-sys built; no OpenSSL or SSLeay licence).

## Parked (Marshal: noticed, not done here)

- `cmd/waf` still depends on `rustls-pemfile` (RUSTSEC-2025-0134, unmaintained); rustls-pki-types' PEM parser replaces it. Not a TLS key-exchange change.
- `cmd/trace-collector`: cargo reports `tonic-build` as an unused build dependency (pre-existing).
- WAF `TlsInfo` does not log the negotiated group; useful for the CBOM/observability later.
