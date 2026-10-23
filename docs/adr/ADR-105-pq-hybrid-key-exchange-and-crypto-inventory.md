<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# ADR-105: Hybrid ML-KEM key exchange on every link we own, a crypto inventory, and signed evidence

**Status:** Proposed (Stevie to accept).
**Date:** 2026-10-23
**Sprint:** S-QUANTUM-ARMOR (`docs/battle-plans/S-QUANTUM-ARMOR-battle-plan.md`).
**Related:** ADR-004 (no new external deps), ADR-034 (gRPC mTLS default), ADR-043 (Mímir's Law hard conditions), ADR-097 (compliance crosswalk; evidence must be able to fail; honesty rules), ADR-098 (findings register; shrink-only baselines), ADR-099 (hosts act only on Gungnir-sealed snapshots), ADR-102 (exact image tags; Well upgrades after a restore drill), ADR-103 (service auth).

## Context

Recorded traffic can be decrypted later by a quantum computer ("harvest now,
decrypt later"). The defence is a hybrid key exchange: classical ECDH and
ML-KEM (FIPS 203) combined, so the session key is safe if either holds.
Go has negotiated `X25519MLKEM768` by default since 1.24 and adds
`SecP256r1MLKEM768` and `SecP384r1MLKEM1024` in 1.26. rustls 0.23.27+ does the
same with the `aws-lc-rs` provider. OpenSSH 9.9+ offers
`mlkem768x25519-sha256`, PostgreSQL 18 takes a group list (`ssl_groups`).
So most of the work is removing the things that turn the defaults off, and
proving the result.

Authentication (certificates, signatures on live sessions) is not exposed to
harvest-now-decrypt-later and stays classical here. Wire-level PQC on the Monad
(the PQC-00 draft, Layer 1) is frozen and out of scope.

Preflight, 2026-10-23 (full table: `docs/security/pq-preflight-2026-10-23.md`).
All nineteen hypotheses held:

- H1: the only non-test `CurvePreferences` in Go is Wotan's HTTP listener, pinned to `X25519, CurveP256`: it turns the hybrid default off.
- H2: go1.27.3, `go 1.26.0`; all three hybrid constants, `MLKEM1024` and `ConnectionState.CurveID` exist.
- H3: no `tlsmlkem=0` / `tlssecpmlkem=0` kill switch anywhere; no `godebug` directive.
- H4: first-party mTLS (`pkg/transport/mtls`, `pkg/mesh`, `pkg/mesh/mtls`, `pkg/loadbalancer`) floors at TLS 1.2; a 1.2 session is always classical. The load balancer also honours `max_version: "1.2"`.
- H5: Wotan gRPC uses `credentials.NewServerTLSFromFile` (Go defaults: 1.2 floor, hybrid groups).
- H6: `cmd/waf` compiles both the `ring` and `aws-lc-rs` rustls providers and installs neither, so building its TLS config panics ("Could not automatically determine the process-level CryptoProvider"); `ring` has no ML-KEM.
- H7: zhend QUIC uses `rustls::crypto::ring::default_provider()`: classical only.
- H8: trace-collector's tonic `tls` and upc-bootctl's reqwest `rustls-tls` features are compiled but unused.
- H9: `postgres:16.15-alpine` negotiates P-256 only (one curve, `ssl_ecdh_curve`); PG 18.3 with `ssl_groups=X25519MLKEM768:X25519:prime256v1` negotiates `X25519MLKEM768`; its default list is classical.
- H10: west and east run OpenSSH 10.0p2; `mlkem768x25519-sha256` is first and our client negotiates it, but the server lists still accept classical KEX.
- H11: WireGuard `wg0` on both hosts uses one static PSK, set 2026-03-25, never rotated.
- H12: all eight `yggdrasil-evidence` subcommands exit 1 with `TODO(task #65)`.
- H13: `gungnir.Verify` refuses expired seals; `SignFor` caps TTL at 31 days.
- H14: `pkg/wotan-client` cannot sign; topic canonicalization is `internal/`; Wotan requires signatures on `config.*` only; `TopicEvent` has no `signed_at_unix_ms`.
- H15: compose Wotan and the edge Traefik run without TLS (accepted finding, 2026-09-30).
- H16: the meta-gate registry is `name|provoke_fn|fast|desc`; an unregistered `check-*.sh` fails it.
- H17: Python `jsonschema` and `pyyaml` are importable locally and pinned in CI.
- H18: `cmd/waf` locks rustls 0.23.46 (≥ 0.23.27).
- H19: west's OpenSSL 3.5.3 lists ML-KEM and accepts `s_client -groups X25519MLKEM768`.

## Decision

1. **Two TLS key-exchange profiles in `pkg/tls` (`pkg/tls/pq.go`).**
   **Strict**: TLS 1.3 only, `CurvePreferences = [X25519MLKEM768, SecP256r1MLKEM768, SecP384r1MLKEM1024]`
   (hybrid only; a classical-only peer fails the handshake). Used where both ends
   are Unheaded Go binaries: `pkg/transport/mtls`, `pkg/mesh`, `pkg/mesh/mtls`,
   wotan-client to Wotan gRPC. **Preferred**: TLS 1.3 floor, `CurvePreferences`
   left nil (Go default: hybrids first, classical fallback). Used on servers that
   third-party clients reach: Wotan HTTP including `/metrics`, the dashboard,
   load-balancer frontends.
   *Rationale:* Strict closes downgrade on links we own both ends of; Preferred
   keeps VictoriaMetrics, curl and browsers working while still negotiating a
   hybrid with any modern client.
   *Override only if:* a first-party mTLS peer is not a Go ≥ 1.24 binary; that
   link then uses Preferred and the CBOM marks it `hybrid-opportunistic`.
   (Preflight H4 found none.)

2. **TLS floors.** TLS 1.3 on every first-party server and first-party mTLS
   client. Third-party outbound clients (`cmd/alert-mailer` SMTP STARTTLS,
   `pkg/threatfeed/kev` CISA fetch, `pkg/audit/export/splunk`,
   `pkg/alerting/notify/email`) keep a 1.2 floor and are recorded
   `hybrid-opportunistic` in the CBOM: their servers decide.
   *Rationale:* CLAUDE.md "TLS 1.3 minimum for external traffic"; ML-KEM exists
   only in 1.3; we cannot force a mail relay's stack.
   *Override only if:* a provider documents TLS 1.3 for its endpoint; that
   client is raised to 1.3 too.

3. **Rust TLS provider: `aws-lc-rs`, explicitly.** Build configs with
   `builder_with_provider(Arc::new(rustls::crypto::aws_lc_rs::default_provider()))`;
   drop the `ring` feature from `cmd/waf` and `crates/zhend`; drop the unused TLS
   features from `cmd/trace-collector` (tonic `tls`) and `cmd/upc-bootctl`
   (reqwest `rustls-tls`).
   *Rationale:* aws-lc-rs is already in both lockfiles, offers `X25519MLKEM768`
   by default in rustls ≥ 0.23.27, and removing `ring` and unused TLS shrinks
   the dependency set (ADR-004).
   *Override only if:* aws-lc-sys fails to build in CI (it needs a C toolchain).
   That crate is then marked STUCK and an ADR-004 exception request goes to
   Stevie. Never fall back to `ring`.

4. **Crypto inventory (CBOM): CycloneDX 1.6 JSON at `compliance/crypto/cbom.cdx.json`.**
   Components of type `cryptographic-asset` with `cryptoProperties`, generated by
   a new Go command `cmd/crypto-inventory` (`go/ast` for Go; lockfile and source
   scan for Rust; config scanners for compose, Postgres conf, sshd and WireGuard).
   Output is deterministic (sorted, no timestamp, no serial number). Host facts
   are merged from `compliance/crypto/hosts/<host>.json`, written by a separate,
   read-only `collect-host` subcommand; `scan` never connects anywhere. CI checks
   the CBOM against the CycloneDX 1.6 schema with Python `jsonschema`, the schema
   fetched by sha256 (the `scripts/lib/fetch-tool.sh` pattern), never vendored.
   *Rationale:* CycloneDX 1.6 is the CBOM format that NIST and CISA PQC-migration
   inventories reference; Go keeps it in the main module with no new dependency;
   determinism makes "stale" checkable by a gate.
   *Override only if:* the schema cannot be fetched reproducibly; then required
   fields are checked by Go struct tests only, and this ADR says so.

## Consequences

(Completed with decisions 9-11.)

## Open questions

(Completed with decisions 9-11.)
