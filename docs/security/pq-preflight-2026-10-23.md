# S-QUANTUM-ARMOR preflight: hypothesis verdicts (2026-10-23)

Phase 0 of `docs/battle-plans/S-QUANTUM-ARMOR-battle-plan.md` (Steps 1-25), run on west
from a develop worktree based on `424edf47`. Read-only on both hosts. Throwaway Postgres
containers (`qa-pg-*`, loopback-only ports) were removed after the probe; the probe key was
shredded. No addresses or key material below.

Step 2 note: the plan's prerequisite commit `443de3f82` is not an ancestor of develop. It is
the pre-re-sign copy of `0a605108` ("fix(zhen-mcp): runbook_list description no longer
hard-codes 7 categories", same subject, same parent chain content), which IS in develop.
Prerequisite met by content; base used: `424edf47`.

| # | Verdict | Evidence |
|---|---|---|
| H1 | TRUE | `git grep CurvePreferences` (non-test Go): only `services/wotan/cmd/wotan/main.go:267` (`X25519, CurveP256`). |
| H2 | TRUE | go1.27.3, go.mod `go 1.26.0` + `toolchain go1.27.3`; `X25519MLKEM768=4588`, `SecP256r1MLKEM768=4587`, `SecP384r1MLKEM1024=4589`, `MLKEM1024=514` in `crypto/tls/common.go`; `ConnectionState.CurveID` present. |
| H3 | TRUE | No `tlsmlkem`/`tlssecpmlkem`/`tlskyber`/`x25519Kyber` anywhere; no `godebug` in go.mod; no `//go:debug`. |
| H4 | TRUE | 17 `VersionTLS12` hits: `pkg/transport/mtls/mtls.go` (96,116,362,371), `pkg/mesh/mesh.go` (210,223), `pkg/mesh/mtls/mtls.go` (54; 295/385 are comment/switch), `pkg/mesh/mtls/provider.go:75`, `pkg/loadbalancer/config.go` (733,740,745 MaxVersion,786), `pkg/loadbalancer/health.go:61`, `cmd/alert-mailer/smtp.go:99`, `pkg/threatfeed/kev/fetch.go:53`. Note `pkg/loadbalancer/config.go:745` sets **MaxVersion** 1.2 when the LB config says `max_version: "1.2"` (that caps the link classical); Phase 5 must handle it. |
| H5 | TRUE | `services/wotan/cmd/wotan/main.go:330` `credentials.NewServerTLSFromFile` → `grpc.Creds` (Go default floor 1.2, default groups). |
| H6 | TRUE | `cmd/waf`: rustls features `ring` (direct, Cargo.toml) and `aws_lc_rs`/`aws-lc-rs` (via tokio-rustls defaults) both compiled; no `install_default`/`builder_with_provider` in `src/`. Throwaway test calling `build_server_config` panicked: "Could not automatically determine the process-level CryptoProvider from Rustls crate features". WAF test baseline: 52 + 1 + 0 passed. |
| H7 | TRUE | `crates/zhend/src/api/quic.rs:412,430` `rustls::crypto::ring::default_provider()`; Cargo.toml `rustls = { features = ["ring"] }`. ring has no ML-KEM, so classical only (X25519 first). Verdict from code; no live zhend handshake run. |
| H8 | TRUE | trace-collector: tonic `tls` feature on, zero `ClientTlsConfig`/`ServerTlsConfig`/`https://` in `src/`. upc-bootctl: reqwest `rustls-tls` on, only `http://` URL (`tty_bridge.rs:48`). |
| H9 | TRUE | `postgres:16.15-alpine` (libssl3 3.5.8): `ssl_ecdh_curve=prime256v1`, negotiates `ECDH prime256v1`, hybrid-only client gets alert 40. `postgres:18-alpine` = PG 18.3 (libssl3 3.5.5, ≥3.5 OK): default `ssl_groups=X25519:prime256v1` negotiates X25519, hybrid-only client gets alert 40; with `-c ssl_groups=X25519MLKEM768:X25519:prime256v1` it negotiates `X25519MLKEM768` (mixed and hybrid-only client). Local tag used: `18-alpine` (18.3); the plan's `18.0-alpine` was not pulled; Phase 8 picks the exact tag (ADR-102). |
| H10 | TRUE | west + east: `OpenSSH_10.0p2 Ubuntu-5ubuntu5.4, OpenSSL 3.5.3`; server KEX list starts `mlkem768x25519-sha256,sntrup761x25519-sha512,sntrup761x25519-sha512@openssh.com` and ALSO lists classical `curve25519-sha256,curve25519-sha256@libssh.org,ecdh-sha2-nistp256/384/521` (opportunistic, not strict: E2). Our client → east negotiated `kex: algorithm: mlkem768x25519-sha256`. |
| H11 | TRUE | Interface is `wg0` on both hosts (not `unheaded`). One peer each, `psk:set`. east `/etc/wireguard/preshared.key` dated 2026-03-25 and its content is the PSK in `wg0.conf` (grep -c match = 1, key never printed); west keeps it inline in `wg0.conf` (no separate file). `wg0.conf` mtime 2026-10-20 on both, but the PSK equals the 2026-03-25 file: never rotated in 7 months. |
| H12 | TRUE | All 8 subcommands with valid args exit 1 printing one `TODO(task #65)` (with bad args: rc=2 usage first). |
| H13 | TRUE | `pkg/gungnir/gungnir.go:134` rejects `ExpiresAtUnix < now`; `keys.go:46` caps TTL at 31 days; context `unheaded/gungnir/seal/v2`; no archival mode. |
| H14 | TRUE | `pkg/wotan-client` has no Signature/SignedAt; `RequiresSignature` = `strings.HasPrefix(topic, "config.")`; `signed_at_unix_ms` appears once in `topic.proto` (request field 8); `TopicEvent` stops at field 11. |
| H15 | TRUE | No `enable-tls`/`ENABLE_TLS` in `docker-compose.yml` or `deploy/`; edge TLS finding open in the register (source `gate-script:scripts/compliance/check-edge-tls.sh`, opened 2026-09-30). |
| H16 | TRUE | REGISTRY rows are `name|provoke_fn|fast|desc` (e.g. line 662 `check-image-tags|provoke_image_tags|fast|...`); header documents that an unregistered `check-*.sh` fails the meta-gate. |
| H17 | TRUE | Local `jsonschema` 4.19.2 + `yaml` import; CI `.github/ci-python/requirements.txt` pins `jsonschema==4.26.0`, `pyyaml==6.0.3` (hashed). |
| H18 | TRUE (version only) | `cmd/waf/Cargo.lock` rustls 0.23.46 (≥ 0.23.27). Group order confirmed by the Phase 7 handshake test. |
| H19 | TRUE | west `OpenSSL 3.5.3`; `list -kem-algorithms` shows ML-KEM-512/768/1024; `s_client -groups X25519MLKEM768` used successfully in H9. |

## Downstream effect

No hypothesis is FALSE, so no phase changes scope. Notes carried forward:
- Phase 5: `pkg/loadbalancer/config.go:745` honours a config `max_version: "1.2"`; a 1.3 floor must reject or ignore that value, and the lint should flag it.
- Phase 8: use an exact PG 18 tag (local `18-alpine` resolved to 18.3, Alpine 3.23, libssl3 3.5.5).
- Phase 9: the WireGuard interface is `wg0`; west stores the PSK inline in `wg0.conf`, east in both `preshared.key` and `wg0.conf`. Rotation must update both places on east.
