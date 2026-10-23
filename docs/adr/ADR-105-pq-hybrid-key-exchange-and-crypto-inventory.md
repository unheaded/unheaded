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

5. **Evidence signatures: a domain-separated, non-expiring evidence seal in `pkg/gungnir`.**
   `SignEvidence(payload, purpose string)` / `VerifyEvidence(payload, seal, pub, purpose)`
   with ML-DSA-65 context `unheaded/gungnir/evidence/v1` and no expiry
   (`ExpiresAtUnix = 0`). `Purpose` is covered by the signature; the purposes are
   `manifest`, `iso-sha256` and `custody-entry`. The seal is stored as a JSON
   envelope (the same encoding as Heimdall's `.seal`):
   `{"format":"gungnir-evidence/v1","purpose":…,"payload_sha256":…,"key_id":…,"issued_at_unix":…,"algorithm":"ml-dsa-65","mldsa65_sig":…}`.
   Different ML-DSA contexts mean a short-lived config seal (`unheaded/gungnir/seal/v2`)
   can never verify as evidence, and evidence can never verify as a config seal.
   *Rationale:* quarterly audits verify packs months later; `Verify` rejects
   anything older than 31 days by design and must keep doing so for config seals.
   *Override only if:* Stevie wants evidence seals to expire and be re-signed;
   then `SignFor` with 31 days is reused and the re-sign ceremony is documented.

6. **Manifest validation without new dependencies.** Typed Go structs
   (`yaml.v3`, `KnownFields(true)`) plus explicit rule checks mirroring
   `nix/yggdrasil/evidence-pack/schema/manifest-v1.yaml`. A schema-drift test parses
   the schema and asserts that every `required`, `const`, `minimum`/`maximum`,
   `pattern` and `enum` it declares is enforced, so it fails when the schema gains
   a rule the Go code lacks. A differential test in CI runs Python `jsonschema` and
   the Go validator over the same fixtures and fails on any disagreement. The
   `yamllint` call in the runbook stays optional.
   *Rationale:* go.mod has no Go JSON-Schema validator (`invopop/jsonschema`
   generates schemas, it does not validate); ADR-004.
   *Override only if:* Stevie approves a validator dependency.

7. **Evidence pack layout and trust anchor.** The tarball root is `manifest/`
   (as the build runbook and `yggdrasil-build-evidence-pack.sh` already lay it
   out). There is one trust anchor, `build.pubkey`: `/etc/yggdrasil/build.pubkey`
   is the default for every verify subcommand and `--pubkey` overrides it. The
   runbook's `gungnir-sign` (which does not exist) becomes `yggdrasil-evidence sign`.
   A root→build key certification chain is parked.
   *Rationale:* the evidence-pack README's invariants already name `build.pubkey`;
   one anchor is honest about what exists.
   *Override only if:* Stevie has already generated a root key; then both are kept
   and `--root` is added.

8. **Signed drift events.** `canonicalize`, `TopicSigner` and a subscriber-side
   verifier move into a new importable `pkg/wotansig` (byte-identical, proven by a
   golden vector); `services/wotan/internal/signing` becomes a thin wrapper. Wotan
   requires signatures on `config.*` and `drift.*`, with keys scoped per prefix
   (`topics.signed_prefixes: [{prefix, keys}]`; the existing `signing_keys` stays
   the `config.` list). `TopicEvent` gains `int64 signed_at_unix_ms = 12;`
   (additive, in Wotan's gRPC API; not the Monad v0x01 wire format, so no RFC
   Editor review is needed) so subscribers can re-verify end to end. Heimdall
   publishes `drift.detected.<node_id>` through `wotan-client.PublishSigned`.
   *Rationale:* ADR-043 hard conditions #1 (alerts only) and #2 (signed topics);
   the open S17 item "an approved ID can publish to any topic" means unsigned drift
   is forgeable; scoping stops a Heimdall key from signing `config.*` (ADR-043 #4,
   key separation).
   *Override only if:* another publisher already uses a `drift.` prefix. None
   does (checked 2026-10-23): `kenoma.drift.detected`, `state.drift.detected` and
   `cuirass.drift` start differently, and `pkg/state`'s `drift.detected` is an
   event *type* inside `cuirass.reconciliation` payloads, not a topic.

9. **WireGuard: scheduled PSK rotation.** `scripts/wireguard/rotate-psk.sh`
   (a dry run unless `--apply`) generates a fresh PSK on west, installs it on east
   over SSH (which negotiates `mlkem768x25519-sha256`, H10, so the PSK never crosses
   a classical-only channel), applies it with `wg set` (no restart), checks for a
   handshake within 200 s and rolls back on failure (amended from 60 s: see
   "Decisions made during implementation"). A weekly systemd timer ships but
   is not enabled. Rosenpass is parked (C/liboqs dependency, ADR-004).
   *Rationale:* a WireGuard PSK mixed into the Noise handshake gives post-quantum
   confidentiality while the PSK stays secret; rotation bounds the damage of a
   leaked PSK (today's has not changed since 2026-03-25, H11); no new dependency.
   *Override only if:* H10 were false (SSH not hybrid); rotation would then wait
   for E2. H10 held.

10. **sshd target.** Hosts: `mlkem768x25519-sha256` first and present
    (opportunistic), as both hosts already are (H10). Strict hybrid-only is
    [ESCALATE] E2 (lockout risk). Repo configs: the runbook and the Ubuntu paths list
    `mlkem768x25519-sha256,sntrup761x25519-sha512,sntrup761x25519-sha512@openssh.com,curve25519-sha256`;
    Yggdrasil (bookworm anchor, OpenSSH 9.2, no ML-KEM) lists
    `sntrup761x25519-sha512@openssh.com` first, with a comment to switch to mlkem at
    the trixie anchor bump. A new probe `sshd-kex-pq` (every KEX entry hybrid) maps to
    the new control as the strict target; `sshd-kex` stays.
    *Rationale:* SSH key exchange is chosen by the client's order; only a hybrid-only
    server list guarantees a hybrid. Making it hybrid-only is a lockout-relevant
    change on Stevie's hosts.
    *Override only if:* Stevie says strict now.

11. **CNSA 2.0 posture: not claimed.** We claim NIST PQC (FIPS 203/204) hybrid
    migration progress only. The CBOM records CNSA 2.0 as not met
    (`unheaded:cnsa2 = no`): ML-KEM-768 and ML-DSA-65 are category 3, CNSA 2.0
    requires ML-KEM-1024 and ML-DSA-87. `SecP384r1MLKEM1024` is already negotiable by
    Go peers (Go 1.26 default) and is noted, not claimed.
    *Rationale:* honesty rules (ADR-097 §5); NSM-10 and OMB M-23-02 ask for an
    inventory, which we can give truthfully.
    *Override only if:* Stevie wants a CNSA 2.0 track (a separate sprint).

## Decisions made during implementation (2026-10-23)

The tracks implemented decisions 1-11 as written, plus the choices below. Each
is recorded in the track's progress file under
`docs/battle-plans/progress/S-QUANTUM-ARMOR-*.md`; this section is what a reviewer
accepts with the ADR.

**Evidence seals and packs (decisions 5-7; `evidence` track).**
- *Seal.* The signed message is length-prefixed `format, purpose, algorithm,
  key_id, issued_at (int64 big-endian), payload_sha256` under the context
  `unheaded/gungnir/evidence/v1`. `ParseEvidenceSeal` is strict: 16 KiB cap,
  unknown or duplicate keys and trailing data refused. gungnir accepts purposes
  matching `^[a-z0-9][a-z0-9-]{0,63}$`; the CLI allows only `manifest`,
  `iso-sha256` and `custody-entry`. Signing refuses a signer whose key id is not
  `KeyID(pub)` (the `dev-spike` placeholder cannot sign evidence).
- *Detached signatures.* `SignEvidenceDetached` / `VerifyEvidenceDetached`: an
  evidence seal with `issued_at_unix = 0` and `key_id = KeyID(pub)` whose
  signature bytes alone are stored. Custody entries use them.
- *Custody chain.* Each `chain_of_custody` entry's `signature` is the hex detached
  evidence signature (purpose `custody-entry`) over
  `lp("yggdrasil/custody/v1") lp(timestamp_utc) lp(actor) lp(action) lp(prev)`,
  `prev` = SHA-256 of the previous entry's message, 32 zero bytes for the first.
  The `signature` field is outside the chain. Reordering, removing or rewriting an
  entry breaks it; `TestCustodyMessageGolden` pins the bytes (cross-checked by an
  independent Python computation).
- *Waivers.* Manifest `cve_scan.critical` / `high` are UNWAIVED counts (the schema
  pins them to 0). A CRITICAL or HIGH finding in `cve-scan.json` is waived only by
  a `cve_scan.waivers` entry unexpired on the verification date (`expires_date`
  inclusive) whose CVE ID also appears in a custody `action`, so a waiver is signed
  by whoever recorded it. MEDIUM and LOW counts must match exactly.
- *Validator strictness.* The Go validator rejects everything the JSON Schema
  rejects (a CI differential with Python `jsonschema` agrees on every fixture) and
  is stricter on: YAML anchors, aliases, tags, duplicate keys and multi-document
  input; non-decimal integers and `.inf`/`.nan`; `$` before a trailing newline;
  non-integer `license_summary` values. Scalars are typed with YAML 1.1 resolvers
  (as PyYAML does), so `yes` and unquoted dates mean what the schema check sees.
  The differential supplies its own RFC 3339 `date-time` checker because
  `jsonschema` without `rfc3339-validator` silently skips that format.
  `validate --schema PATH` refuses to run when that schema differs from the
  compiled-in rules.
- *verify-gates* also cross-checks `cis-benchmark.json` `score_percent` (and
  `lynis_hardening_score` if present), `upc-doctor.json` `exit_code`, and every
  overlay patch (listed equals present, sha256 matches, nothing unlisted).
- *Pack reader.* In memory, streaming; names must already be clean (no absolute,
  `..`, `//`, `./`, backslash or NUL), entries only under `manifest/`, regular
  files and directories only (no links or devices), no duplicates; caps of 64 MiB
  per file, 256 MiB total, 10 000 entries.
- *Exit codes.* A malformed or unsafe pack is a verification failure (1); an
  unreadable path or missing public key is usage (2); `diff` returns 2 when it
  cannot read or validate an input and 3 when custody was rewritten.
- *`pubkey`* is a subcommand (derives `build.pubkey` from a seed; prints nothing secret).

**Signed drift (decision 8; `drift` track).**
- *Signed sender.* The canonical form covers a sender string. The signed sender
  is `sender_id` when the publisher gives one, else the display name Wotan
  authorized (`"service"` by default), and a signed event's `sender_id` is that
  string, so a subscriber can rebuild exactly what was signed and the signature is
  bound to the authorized identity. (Before, a publisher authorized by display name
  was verified against `""` and stored under a server-made UUID; no production
  `config.*` signer existed, so nothing outside one test changed.)
- *Prefix rules.* `topics.signed_prefixes` entries are validated at load: a prefix
  ends in `.`, has no empty segment, wildcard, space or control character, is at
  most a fixed length and is listed once; `config.` is refused there (its keys stay
  `signing_keys`); keys must be base64 ML-DSA-65 public keys, listed once; unknown
  fields fail. `config.` and `drift.` are built in, so `drift.` with `keys: []`
  (what `configs/wotan.yaml` ships) refuses every drift publish until E5. The
  longest matching prefix decides; a key trusted for one prefix verifies nothing
  under another. The HTTP publish path refuses every signed prefix (it carries no
  signature). Wotan now stores the verified signature on the ring-buffer message
  and copies it, with `signed_at_unix_ms`, onto every `TopicEvent`, live and
  replayed; before this, no subscriber ever received a signature.
- *Heimdall identity.* Heimdall publishes under the constant display name
  `heimdall` and signs for it; the node is in the topic (`drift.detected.<node_id>`,
  node IDs normalized to one `[a-z0-9-]` segment) and in its per-node key. The
  drift key must differ from `--sign-key` and `--pubkey` by gungnir key id
  (ADR-043 #4); a group- or world-readable key file is refused. Publishing goes
  through a bounded drop-oldest queue that never blocks detection. A test walks the
  code and fails on any file write, rename, chmod or remove outside manifest
  signing (ADR-043 #1, alerts only).
- *`ErrInvalidSignature`.* A signature or public key of the wrong length now fails
  as `wotansig.ErrInvalidSignature`, not as pqc's size error: bytes from the network
  that do not verify are a verification failure, whatever the reason.

**Rust provider (decision 3; `rust` track).** `aws-lc-rs` 1.18.2 / `aws-lc-sys`
0.46.0 were already compiled (tokio-rustls defaults); they are now the only rustls
provider, recorded as an ADR-004 admission (ADR-096 §2) whose owner sign-off is
open. The licence check found no OpenSSL or SSLeay licence in aws-lc-sys (that code
is under Apache-2.0). The WAF and zhend QUIC are reached by peers we do not build,
so both use the Preferred shape (X25519MLKEM768 first, classical fallback); the
WAF's default `min_version` is now 1.3. Dropping tonic's `tls` feature from
trace-collector would have turned an `https://` endpoint into silent cleartext, so
it now refuses any scheme but `http://`. The dependency baseline shrank by 17 packages.

**The Well (E1; `well` track).** The upgrade is a dump and restore into a NEW
volume (`restore-drill.sh --into-volume`, which refuses an existing volume), not
`pg_upgrade`: the official images carry one major each, the Well is small, the
Sunday drill already proves this path, and the PG 16 volume is never written, so
rollback is lossless until it is thawed. Override only if the Well outgrows a
restore window (the drill's `unheaded_well_drill_restore_seconds` says when).
`postgres:18.6-alpine` is pinned by digest for the throwaway suite only; compose,
Kubernetes and CI stay on 16.15 until E1. Clients need no change: libpq 16 and 18
and the Go services' `lib/pq` already offer X25519MLKEM768 first.

**Load balancer (decision 2).** Unset `min_version` is 1.3 and frontends use
Preferred. `min_version: "1.2"` (user frontends with old clients) and
`max_version: "1.2"` stay operator opt-ins, re-applied after `ApplyPreferred`, and
are the two accepted lines in `docs/policy/pq-kex-baseline.txt`.

**WireGuard (decision 9; `hosts` track).** The rotation tool waits 200 s for a
handshake, not 60 s: a new PSK is used only at the next handshake, which WireGuard
starts once the session is 120 s old (REKEY_AFTER_TIME), so a 60 s window would roll
back healthy rotations. The deployed PSK is inline in `wg0.conf`, so the tool
persists by rewriting that `PresharedKey` line (the key read from a file, never an
argument) and keeps `<if>.psk`, whose mtime is the last rotation (FND-016's check).

## Consequences

**What the gates enforce.** `scripts/check-crypto-inventory.sh` fails when the
checked-in CBOM no longer matches a regeneration from the tree, so every new TLS,
SSH, WireGuard, Postgres or signature site shows up in review. `scripts/check-pq-kex.sh`
fails on any new classical-only key-exchange configuration: a `CurvePreferences`
list with no hybrid group, a first-party TLS floor below 1.3, a `tlsmlkem=0` /
`tlssecpmlkem=0` kill switch, a go.mod `go` directive below 1.24, a rustls `ring`
provider, a Postgres image below 18 serving TLS, or an sshd `KexAlgorithms` list with
no hybrid. Its baseline (`docs/policy/pq-kex-baseline.txt`) holds today's violations
and may only shrink (ADR-098). Both gates are registered with the meta-gate, which
proves each one fails when provoked. `cmd/lich-security` is exempt from the lint: it
probes weak TLS on purpose.

**What stays classical, and why.**
- **The Well** stays on PG 16 (P-256 only) until Stevie runs E1 (PG 18 upgrade with
  `ssl_groups`, after a backup and restore drill, ADR-102). It stays in the lint
  baseline until then.
- **Plaintext links**: compose Wotan and the edge Traefik have no TLS today (accepted
  finding, 2026-09-30). The fixes here make them hybrid the day TLS is turned on;
  turning it on is another sprint (ADR-034, ADR-103).
- **Third-party outbound clients** (SMTP relays, the CISA KEV feed, Splunk HEC, load-balancer health
  checks to user backends)
  negotiate whatever the far end offers: hybrid with a modern server, classical
  otherwise. They are inventoried as `hybrid-opportunistic` / `third-party-peer`.
- **Preferred listeners** guarantee a hybrid only against a client that offers one.
  A client that lists a classical group first may still get it. The observed Go
  behaviour is recorded here by the red-team step (Phase 14).
- **Certificates and signatures on live sessions** stay classical (ECDSA, RSA,
  Ed25519). They are not exposed to harvest-now-decrypt-later; PQ certificates are
  parked until the ecosystem and NIST IR 8547's timeline call for them.
- **SSH** stays opportunistic (hybrid first, classical accepted) until E2.
- **WireGuard** remains X25519 Noise plus a static PSK; the PSK is the
  post-quantum part, so it is only as strong as its secrecy and rotation (E3).
- **CNSA 2.0** is not met and not claimed (decision 11).

**Costs.** Strict links fail closed against any peer without a hybrid group: an
old Go (< 1.24) binary or a non-Go client on a Strict port cannot connect. That is
the point on links we own both ends of; Appendix A.2 of the sprint plan moves a
site to Preferred, with a test, if a real client breaks. Rust crates take a C
toolchain dependency through `aws-lc-sys` (already compiled today via
tokio-rustls's defaults).

## Open questions

The sprint prepares these and stops; each needs Stevie (plan Appendix B.2):

- **E1** (FND-015): upgrade the live Well from PG 16 to PG 18 with `ssl_groups` (hybrid TLS).
  Prepared: include file, throwaway-container proof suite, dry-run runbook with a
  restore drill (`runbooks/data/well-pg18-upgrade.yaml`).
- **E2** (`host-probe:{west,east}:sshd-kex-pq`, control UH-PQC-03): make sshd on west and east hybrid-only (lockout risk). Prepared: strict
  probe, repo configs, procedure (east first, a second session kept open).
- **E3** (FND-016): the first live WireGuard PSK rotation, and enabling the weekly timer.
  Prepared: a tested rotation tool (dry run by default), units not enabled.
- **E4** (FND-018): the production evidence build key: where the seed lives, who holds it,
  how `build.pubkey` is published, the rotation ceremony. Prepared: `sign`,
  `pubkey` and `verify*` working with ephemeral keys.
- **E5** (FND-019): a Heimdall drift key per node, enrolled in live Wotan `signed_prefixes`,
  and a unit to run Heimdall. Prepared: signed publish, scoped verification,
  fail-closed configuration.

Also for Stevie, not blocking: accept this ADR, and sign off the `aws-lc-rs` /
`aws-lc-sys` admission recorded in ADR-004 (owner sign-off pending).
