<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Iron Gate hooks for the Lich

Targets that S-IRON-GATE (`docs/battle-plans/S-IRON-GATE-battle-plan.md`, Phase 15 Step 391) leaves for a
future Lich campaign. The sprint ships the code, its unit tests and the fuzz targets below and runs them
once (`docs/security/iron-gate-attack-surface.md`, "Fuzz re-run"); campaigns, budgets and findings belong
to the Lich (ADR-062). Design: ADR-103 (auth), ADR-105 (TLS), ADR-043 / Phase 13 (sealed cask).

## Fuzz targets already in the tree

| Target | Package | What it holds |
|---|---|---|
| `FuzzEdDSAParse` | `pkg/auth` | no presented token verifies (the trusted key never signs a seed); the raw parser `verifyEdDSA` refuses too |
| `FuzzJWKSDocument` | `pkg/auth` | any JWKS body: no panic, more than 64 KiB refused, every kept key has a kid and 32 bytes |
| `FuzzTrustDirFilenames` | `pkg/auth` | any trust.d file name: no panic, a loaded key is bound to a valid service name |
| `FuzzLoginBody` | `cmd/unheaded-auth` | any `/login` body: no panic, never earns a token, no token in an error response |
| `FuzzUsersFile` | `cmd/unheaded-auth` | any users file: no panic, hash parameters bounded |
| `FuzzVerify` | `pkg/sealedcask` | any byte string: no panic, never accepted without a trusted signature, anything accepted names only clean paths under one `sealed-cask-*` root |
| `FuzzVerifyEvent` | `pkg/wotansig` | signed-topic events |
| `FuzzLoadTopicConfig` | `services/wotan/internal/api` | `configs/wotan.yaml` signed prefixes and keys |

```
go test ./pkg/auth/ -run '^$' -fuzz '^FuzzEdDSAParse$' -fuzztime 10m
```

Nothing fuzzes `pkg/bff` (login relay, cookie, CSRF header), the API-key stores, or the apt path.

## Seeds

`tomb/lich/seeds/auth/`:

- `jwks-01..05-*.json` (committed): wrong curve (`X25519`), one kid for two keys, a 31-byte key, a
  symmetric `oct` key with `alg: HS256`, `keys` as a nested object plus a metadata-service `jwks_uri`.
- `tokens/token-01..10-*.txt` (built at run time, gitignored): `go run ./tomb/lich/seeds/auth/gen`
  (`-jwks` also rewrites the JWKS documents). They have the shape of real JWTs, so gitleaks' `jwt` rule
  would flag them in the tree; every signature is made with an Ed25519 key drawn at run time and
  discarded. Shapes: `alg: none`; HS256 keyed with the Ed25519 public key (asymmetric to symmetric
  confusion); `alg: eddsa`; `alg` twice in one header; a path-traversal `kid`; no `kid`; `crit`; `aud` as an
  array with a 24-hour life; `sub` not the owner of the `kid` with `roles: [admin]`; four segments with
  padding.

Against `pkg/auth` at `5cd72c8c` every token is refused (checked 2026-10-27): 01-04 at the `alg` check (04:
Go's `encoding/json` keeps the last duplicate key, `none`), 05, 08, 09 at key lookup (signed by an
untrusted key), 06 no kid, 07 `crit`, 10 malformed. Every JWKS document yields no keys (02: duplicate kid
refused). A campaign that wants 08 and 09 to reach the claim checks must sign them with a key it put in
the verifier's trust.d (see "Token confusion" below).

To use them as Go corpus: `go test` reads `testdata/fuzz/<Target>/` files in its own format
(`go test fuzz v1` + `string("...")`); write them into a temporary copy of the package, never commit
the result.

## Targets

### Token confusion (`alg`, `kid`, `sub`)

`pkg/auth/jwt_eddsa.go` (`verifyEdDSA`, `NewEdDSAVerifier`), `pkg/auth/keys.go` (`NewTrustDir`, `GenerateKeyPair`).

- `alg`: exactly `EdDSA`; try Unicode look-alikes, `EdDSA\u0000`, a JSON number, `alg` inside `crit`.
- `kid`: 16 hex, looked up in trust.d. Try collisions with a rotated-out key (`unheaded-keygen -rotate`
  keeps the old public half until `-retire`), a kid that names a file outside trust.d, a kid shared by
  two `.pub` files.
- Identity binding: `sub` must equal the file name of the key that verified. Mint with a key you placed
  as `a.pub` and claim `sub: b`; drop a `.pub` under a second name; use a name the allowlist parser
  treats differently (`AUTH_ALLOW_CALLERS` split and trim).
- Claims: `aud` array vs string, `exp` / `iat` / `nbf` at the int64 edges, the 5-minute service cap and the
  1-hour user cap (`AUTH_OIDC_MAX_LIFETIME`), `act` (on-behalf-of) on a token from a service that is not a BFF.
- Cross-language: `crates/svc-token` (Rust, serde) and `scripts/svc_token.py` mint; only Go verifies.
  Duplicate keys and number formats parse differently across the three; a minter that emits something Go
  reads differently is a finding (`tests/svctoken` is the existing differential).

Oracle: a token verifies iff a key in trust.d signed exactly those bytes, its file name is `sub`, `sub` is
allowed and `aud` is the target. Anything else verifying is a finding.

### JWKS poisoning through discovery

`pkg/auth/jwks.go` (`JWKSVerifier.fetch`). Discovery must name the configured issuer; `jwks_uri` must be
https (http only to loopback or with `WithPlainHTTPIssuer`); at most three redirects, each re-checked;
64 KiB cap; kept keys are refreshed on a TTL and kept on fetch failure.

- A discovery document whose `jwks_uri` points at another internal https service (blind GET from every
  verifier: SSRF reach), at a slow endpoint (fetch timeout vs request latency), at a redirect chain that
  ends in http.
- Key rollover: a fetch that drops a key mid-session; a fetch that fails forever (stale keys kept: how
  long?); two kids for one key.
- Harness: `pkg/bff/bfftest` has a fake issuer; point a `JWKSVerifier` at it and mutate its answers.

### Session fixation and CSRF on the BFF

`pkg/bff/bff.go`, `/auth/login`, `/auth/me`, `/auth/logout` on dashboard-backend (20000) and kanban-app
(20001). A login always issues a fresh 32-byte id and deletes a presented one; the cookie is
`HttpOnly; Secure; SameSite=Strict`; cookie-authenticated writes need `X-Requested-With: unheaded`.

- Plant a cookie, log in, check the planted id is dead; log out on one BFF, replay on the other (separate
  in-memory stores: the cookie name is shared, the sessions are not).
- CSRF: a cross-site form cannot set the header; try a same-site sibling (another port on the same host
  is same-site for cookies), `text/plain` bodies, method override, WebSocket upgrade (`/ws/*` reads with
  the cookie).
- Lockout as a denial of service: behind traefik every user shares one source address, so 60 bad logins a
  minute lock everyone out of the BFF, and 20 in 10 minutes lock the BFF out of the issuer.

### API-key timing and state oracle

`pkg/auth/apikey.go`, `pkg/auth/apikey_store.go`, migration 036. Keys are looked up by SHA-256
(`key_hash`, unique index, or a map in the memory and file stores); legacy `AUTH_API_KEYS` lists compare
with `hmac.Equal` in a loop that stops at the first match.

- Measure unknown vs revoked vs expired vs valid-but-wrong-role: a timing or status difference tells an
  attacker a stolen key's state.
- The `uhk_` prefix (12 characters) is stored in clear for listing: confirm nothing answers a prefix query
  without auth (`unheaded-cli apikey list` needs the Well role).
- `last_used_at` updates under load (one UPDATE per use): write amplification as a denial of service.

### reprepro and the apt site

`scripts/apt-repo.sh`, `deploy/nginx/unheaded-apt.conf`, `tests/apt/apt_repo_test.sh`. nginx serves only
`/dists/`, `/pool/` and `/unheaded-apt.gpg` from `/srv/unheaded-apt`; everything else is 403.

- Path traversal and normalisation: `/dists/../conf/distributions`, `%2e%2e`, `//conf`, `/pool/..;/db/`,
  overlong UTF-8, a symlink placed in `pool/` by `include`.
- `include` input: a `.deb` whose control fields or file names carry `../`, newlines, or reprepro
  override syntax.
- Clients: an unsigned or re-signed `Release`, an old `Release` replayed (freeze attack: `Valid-Until` is
  not set), `SHA256SUMS.gpg` from another key.
- Note: the old repo on west (`0.0.0.0:18888`, autoindex, `/conf` and `/db` readable) is a live finding
  until the E3 window retires it; do not probe it beyond read-only GETs.

### cosign bundle substitution

`.github/workflows/release.yml`, `scripts/verify-release.sh` (offline test in CI). The verifier pins the
certificate identity to `release.yml` at the release's own tag and the GitHub OIDC issuer.

- Swap a file's `.sigstore.json` for one from another release of the same repo (identity at a different
  tag must fail), from a fork (different repo), from another workflow of this repo.
- Strip the Rekor entry or the inclusion proof; offline vs online verification differences.
- `checksums.txt` vs the files: a file listed twice, a file present but unlisted, path separators in names.

### Sealed cask

`pkg/sealedcask`, `cmd/cask-verify`, `yggdrasil-evidence cask`. `FuzzVerify` covers the tar reader.
Add: seal from a key that `--pubkey` lists only for rotation, `purpose` other than `sealed-cask-v1`, a seal
over a different `BINDING_RUNE.sha256` with the same file set.

## Confinement

Everything above runs against throwaway processes on loopback with keys made in a temp dir
(`unheaded-keygen -dir $TMP -trust-dir $TMP/trust.d`) and the fake issuer in `pkg/bff/bfftest`. No
campaign uses `configs/keys/dev`, `/etc/unheaded/keys` or a running service's trust.d, and none signs
anything with a key a deployment trusts. The apt and cosign targets run against a local reprepro tree
(`tests/apt/apt_repo_test.sh` builds one) and downloaded release assets, never against west's live sites.
