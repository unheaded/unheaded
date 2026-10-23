# Post-Quantum Key Exchange

Every TLS link we own negotiates a hybrid ML-KEM group (`X25519MLKEM768`), so traffic recorded today stays confidential against a future quantum computer ("harvest now, decrypt later"). This page says where that holds, how to keep it holding when you add a TLS site, and what is still classical. Decisions: ADR-105.

## Profiles (`pkg/tls/pq.go`)

| Profile | Call | Groups | Use when | Classical-only peer |
|---|---|---|---|---|
| Strict | `pqtls.ApplyStrict(&tls.Config{...})` | `X25519MLKEM768`, `SecP256r1MLKEM768`, `SecP384r1MLKEM1024`, TLS 1.3 | both ends are Unheaded Go binaries | refused |
| Preferred | `pqtls.ApplyPreferred(&tls.Config{...})` | Go default (hybrids first), TLS 1.3 | third-party clients connect (browsers, curl, VictoriaMetrics) | falls back to X25519 |

A Preferred listener guarantees a hybrid only to a client that offers one. Rust: rustls on `aws_lc_rs::default_provider()` (hybrid first), never the `ring` provider.

| Surface | Profile |
|---|---|
| Wotan gRPC, wotan-client | Strict |
| `pkg/transport/mtls`, `pkg/mesh`, `pkg/mesh/mtls` | Strict |
| Wotan HTTP and `/metrics`, dashboard, `pkg/http` fallback, load-balancer frontends | Preferred |
| WAF (`cmd/waf`), zhend QUIC (`crates/zhend`) | Preferred (rustls, aws-lc-rs) |
| SMTP relays, CISA KEV, Splunk HEC, LB health checks to user backends | TLS 1.2 floor: the far end chooses |

## Adding a TLS site

1. Build the config through `ApplyStrict` or `ApplyPreferred`. Never set `CurvePreferences` yourself, never a `MinVersion` below 1.3 for a first-party server.
2. Add a handshake test that asserts the negotiated group (`pkg/tls/pqtest`; Strict sites also assert an X25519-only peer is refused).
3. Regenerate the inventory: `go run ./cmd/crypto-inventory scan --out compliance/crypto/cbom.cdx.json`.
4. Run `./scripts/check-pq-kex.sh` and `./scripts/check-crypto-inventory.sh`.

## Gates

| Gate | Fails when | Baseline |
|---|---|---|
| `scripts/check-pq-kex.sh` | a classical-only key-exchange config appears: `CurvePreferences` without a hybrid, a first-party TLS floor below 1.3, `tlsmlkem=0`, a `go` directive below 1.24, the rustls `ring` provider, Postgres below 18 serving TLS, an sshd `KexAlgorithms` with no hybrid | `docs/policy/pq-kex-baseline.txt`, shrink-only; a fixed line must be removed (`--update`) |
| `scripts/check-crypto-inventory.sh` | `compliance/crypto/cbom.cdx.json` differs from a regeneration (`--schema` also validates CycloneDX 1.6) | none |

Both run in CI and the meta-gate proves each can fail. Explain one file: `go run ./cmd/crypto-inventory lint --explain <path>`. Paths given to the tool are repository-relative; it reads and writes nothing outside the repository.

## The inventory (CBOM)

`compliance/crypto/cbom.cdx.json`: CycloneDX 1.6, one component per TLS, SSH, WireGuard, Postgres or signature site, with `unheaded:pq-status` (`hybrid-strict`, `hybrid`, `hybrid-opportunistic`, `classical`, `pq-psk-static`, `pq-signature`, ...). Count by status: `go run ./cmd/crypto-inventory summary`. Host facts come from read-only snapshots in `compliance/crypto/hosts/<host>.json` (`collect-host --host east --ssh govan@east --out compliance/crypto/hosts/east.json`); `scan` never connects anywhere.

## Probes

```bash
scripts/probe-tls-groups.sh 127.0.0.1:18001              # hybrid offer, then classical offer
scripts/probe-tls-groups.sh 127.0.0.1:5432 --starttls postgres
ssh -vv east true 2>&1 | grep 'kex: algorithm'           # mlkem768x25519-sha256
```

## Still classical

| What | Why | Tracked |
|---|---|---|
| The Well (PostgreSQL 16, P-256 only) | PG 16 takes one curve; PG 18 `ssl_groups` is needed | E1, FND-015, [[The Well|The-Well]] |
| Host sshd accepts classical KEX | ML-KEM is first and negotiated; hybrid-only is a lockout-relevant change | E2, UH-PQC-03 |
| WireGuard PSK static since setup | the PSK is the post-quantum part; rotation tool ready | E3, FND-016, [[Overlay Network|Overlay-Network]] |
| Links without TLS (Wotan in compose, the dev edge) | plaintext today; hybrid the day TLS is on | FND-017 |
| Certificates, handshake signatures | not exposed to harvest-now-decrypt-later | ADR-105 |

CNSA 2.0 is not claimed (ML-KEM-768 / ML-DSA-65, not 1024 / 87).

## Compliance

Controls `UH-PQC-01` (inventory current), `UH-PQC-02` (first-party KEX hybrid), `UH-PQC-03` (host SSH hybrid-only; fails until E2) and `UH-EVID-01` (signed build evidence) in `compliance/catalog/controls.yaml`.

---

> **Source:** [ADR-105](../docs/adr/ADR-105-pq-hybrid-key-exchange-and-crypto-inventory.md) · [SECURITY.md](../SECURITY.md) · [S-QUANTUM-ARMOR plan](../docs/battle-plans/S-QUANTUM-ARMOR-battle-plan.md)
