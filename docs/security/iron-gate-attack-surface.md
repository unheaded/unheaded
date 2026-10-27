<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# S-IRON-GATE attack surface

S-IRON-GATE Phase 15 (Steps 384-391), 2026-10-27. Every listener, key file, header and workflow the
sprint added or changed, read from the code on develop at `5cd72c8c` (Phases 1-14 merged), not from the
plan. Plan: `docs/battle-plans/S-IRON-GATE-battle-plan.md`; what was built: `docs/battle-plans/progress/S-IRON-GATE-progress.md`.

**Live state is not this table.** Checked read-only on west 2026-10-27: nothing listens on 19020, Wotan
answers plain HTTP on 0.0.0.0:18000, and the old unsigned apt repo is on 0.0.0.0:18888. The rows below are
what the repo deploys once the LIVE steps and the E3 host window run (`make binaries && sudo
scripts/gen-dev-keys.sh && sudo scripts/gen-dev-certs.sh`, recreate the stack; east via the Phase 14 debs).

Columns: **uid** is the account the process or file belongs to (`pkg/uids`); **reachable from** is the
network that can open the socket, or who can read the file; **auth** is what a caller must present;
**TLS** is the transport profile (`pkg/tls`: Strict = TLS 1.3 with hybrid `X25519MLKEM768` only,
ADR-105; `UNHEADED_TLS_PROFILE=preferred` admits classical clients); **fuzzed by** names the Go fuzz
target, re-run for 90 s in this review (results at the end).

Compose note for every "LAN" entry: compose publishes service ports on `0.0.0.0` (`ports: "N:N"`). Docker
routes published ports through its own DNAT and FORWARD chains, so `deploy/nftables/west.nft` (an INPUT
policy) does not filter them and no `DOCKER-USER` rule exists. A compose port is reachable from the home
LAN on west whatever the host firewall says. That is why the service-token and TLS gates below are the
boundary, not the network.

## Listeners

| what | listener/file | uid | mode | reachable from | auth | TLS | fuzzed by |
|---|---|---|---|---|---|---|---|
| unheaded-auth, built-in OIDC issuer (`cmd/unheaded-auth`, Phase 4) | `:19020` (unit `-listen :19020`; compose: no host port) | 16735 `unheaded-auth` | unit: no capabilities, `ProtectSystem=strict`, `@system-service` minus `@privileged @resources`; compose: read-only, `cap_drop: ALL` | compose: `data` network only (the two BFFs); host: every interface (west INPUT drops it from the LAN; east admits all of 192.168.13.2) | `POST /login`: username + password, argon2id (64 MiB, at most 2 in flight), 4 KiB body cap; lockout 5 failures per (user, source) and 20 per source in 10 min (429, never reaches argon2id; source is the peer address, forwarded headers ignored); `GET /jwks.json`, `/.well-known/openid-configuration`: none (public keys); `/health /ready /metrics` open | Strict; refuses everything but the probes over plain HTTP (`tls_required` 403) unless `-dev-plaintext`; HSTS over TLS; CSP `default-src 'none'`, `Cache-Control: no-store` | `FuzzLoginBody`, `FuzzUsersFile` |
| Wotan HTTP + gRPC, now token-gated (Phase 7) | `:18000` HTTP, `:18001` gRPC (compose publishes both) | 16720 | compose read-only | LAN (compose) | EdDSA service token, `aud=wotan`, `kid` in trust.d and `sub` in `AUTH_ALLOW_CALLERS` (24 identities); topic reads need a token; publishing under another identity's subscriber/sender id refused; `topics.auto_approve` matches verified `sub`, not `display_name`; admin API off unless `-admin` + admin key env + role `admin` | Strict on both listeners; cluster replication no longer drops to plaintext | `FuzzEdDSAParse` (token parser), `FuzzLoadTopicConfig`, `FuzzVerifyEvent` (signed prefixes) |
| Every first-party service API (Phases 5-6: timeguru, captain, architect, micromanager, monad, sophia, cuirass/`unheaded-daemon`, sword, akira, kanban-app, dashboard-backend, zhen-agentd, protocol-api incl. gRPC 17101) | 19000-19006, 19011, 19100, 20000, 20001 (compose `0.0.0.0`); zhen-agentd loopback | each its own `pkg/uids` uid | compose read-only, `cap_drop: ALL` | LAN (compose); hosts per nftables | `auth.NewServiceHandler`: service token from the per-service allowlist (e.g. sword <- `unheaded-cli`, `dashboard-backend`), `uhk_` API key, or (BFFs) the session cookie; RBAC behind it: writes need admin/operator/service; a bad auth config refuses to start; `/health /ready /metrics` open; Akira `GET /api/v1/status` open (runbooks curl it), empty allowlist | Strict when `UNHEADED_TLS_ENABLED=true` (compose and units set it); a non-loopback plaintext listener logs FND-017 | `FuzzEdDSAParse`, `FuzzTrustDirFilenames` |
| BFF routes on dashboard-backend and kanban-app (`pkg/bff`, Phase 6) | `/auth/login`, `/auth/me`, `/auth/logout`, `/auth/auth.js` on 20000 / 20001 | 16728 (dashboard-backend), 16729 (kanban-app) | as the service | LAN (compose) | password relayed to the issuer over Strict TLS with the deployment CA; 10 logins/min per (client, user) and 60/min per client in front of the issuer's lockout (behind traefik every user shares one address); 4 KiB body | same listener | none (`pkg/bff` has no fuzz target) |
| healthprobe (`cmd/healthprobe`, Phase 8) | none (client; replaces busybox `wget` in healthchecks) | the container's uid | | | | https with the CA | no |
| Heimdall drift monitor `/metrics` (unit added Phase 9) | `127.0.0.1:19101` (default `-metrics-addr`) | 16736 `unheaded-heimdall` | see host privilege table | loopback | none (metrics only) | none (loopback) | no |
| Signed apt repo (Phase 14, `deploy/nginx/unheaded-apt.conf`) | `127.0.0.1:18888`, `192.168.13.2:18888` (P2P to east) | nginx worker (`nginx` sysuser) | `/srv/unheaded-apt` 0755, `incoming/` 0750 | loopback and east over the P2P link only | none; only `/dists/`, `/pool/` and `/unheaded-apt.gpg` served, everything else `deny all` (403), `autoindex off`; method limit is nginx's static handler (GET/HEAD; others 405), no `limit_except` | none: integrity is the GPG `Release` signature (E4) and `SHA256SUMS.gpg` | no (`tests/apt/apt_repo_test.sh`) |
| Legacy apt repo (pre-sprint, still live on west) | `0.0.0.0:18888`, autoindex, `/conf/distributions` and `/db/` answer 200 on the LAN | nginx | | LAN | none | none | open finding `host-probe:west:legacy-apt-repo-18888`; retired in the E3 window |

## Key and secret files

| what | listener/file | uid | mode | reachable from | auth | TLS | fuzzed by |
|---|---|---|---|---|---|---|---|
| Service identity keys (Ed25519, Phase 2) | `/etc/unheaded/keys/<identity>.key`: one per deb identity (20: the 22 deb services less huginn and well-retention) + `unheaded-cli` (operator) + `ebpf-collector` (by hand); compose `configs/keys/dev/<id>.key` as secret `svc_key_<id>` at `/run/secrets/` | owner = the service account; `unheaded-cli` = the sudo user | key 0400, dir 0711 (no listing; 0750 root would lock every service out); every loader refuses group/other bits (`pkg/auth`, `scripts/svc_token.py`, `crates/svc-token`) | the owning service only; compose mounts each key into its own container only | n/a | n/a | `FuzzTrustDirFilenames` |
| Trust directory (public halves) | `/etc/unheaded/trust.d/<identity>.pub`; compose `configs/trust.d/dev` mounted read-only | root | dir 0755, files 0444 | every local user (public keys) | the file name binds `sub`: a key can only speak as the name it is filed under; root can add a trusted identity | n/a | `FuzzTrustDirFilenames` |
| Issuer users file | `/etc/unheaded/auth/users.json` (compose: `configs/auth/dev-users.json`, bind, read-only) | 16735 | 0600 (refused if group/other bits), dir 0700; written atomically, owner kept | `unheaded-auth` only | argon2id PHC hashes, parameters bounds-checked on load | n/a | `FuzzUsersFile` |
| Issuer signing key | `/etc/unheaded/keys/unheaded-auth.key` / secret `svc_key_unheaded-auth` | 16735 | 0400 | issuer only | n/a | n/a | |
| Service TLS material (Phase 8) | `/etc/unheaded/tls/<identity>/{<identity>.pem,<identity>-key.pem,ca.pem}`; compose `configs/certs/dev/<svc>/` mounted read-only into that service only | key `root:unheaded-<unit>`; dev: the service uid | key 0640, certs 0644, dir 0755 (Akira's hourly cert-expiry check lists every dir and reads certificates, never keys) | key: the service's group | n/a | Strict | no |
| Dev CA key | `configs/certs/dev/ca-key.pem` | the developer (or root under sudo) | 0600, dir 0750; never mounted | the developer | n/a | n/a | lab CA only (residual risk) |
| API keys (`uhk_`, Phase 3) | Well table `api_keys` (migration 036) or `AUTH_APIKEY_STORE=file:<path>` | Well role `app_auth` (SELECT, INSERT, UPDATE of `revoked_at`, `last_used_at` only) | SHA-256 of the key only (`key_hash`, checked by a CHECK constraint) + 12-character prefix for listing | services that verify keys | the key itself; 90-day default lifetime; gitleaks rule `unheaded-api-key` (`uhk_` + 43) blocks one in the tree | Well: TLS 1.3 (existing) | no |
| Drift signing key (ML-DSA-65, Phase 9) | `/etc/unheaded/keys/heimdall-drift.key` (`unheaded-keygen -algo mldsa65`) | 16736 | 0400; `pkg/wotansig` refuses group/other bits | Heimdall only | public half enrolled per deployment in `/etc/unheaded/wotan.yaml` (`keys: []` in the repo; E2) | n/a | `FuzzVerifyEvent` |
| Akira health signing key (ML-DSA-65, Phase 9) | `/etc/unheaded/keys/akira-health.key` via `HEALTH_SIGNING_KEY_FILE` (commented in the unit; not in compose) | `unheaded-akira` | 0400 | Akira only | without it Akira skips signed consensus/remediate publishes | n/a | `FuzzVerifyEvent` |
| Evidence build key (ML-DSA-65 seed, Phase 13) | per E1 (drafted: custody host, `unheaded-buildkey`) | 16737 `unheaded-buildkey` (no service runs as it) | 0400 drafted + auditd watch | the custody account; `build-sealed-cask.sh` signs through `sudo -n -u $UNHEADED_BUILD_SIGN_AS` | `BINDING_RUNE.sig` checked against a trusted `build.pubkey` (`cask-verify`, `yggdrasil-evidence cask verify`) | n/a | `FuzzVerify` (`pkg/sealedcask`), `FuzzReadPack`, `FuzzValidateManifest` |
| Sealed-cask age key | `secrets/cask.age.key` **inside the repo checkout** (gitignored), created by `build-sealed-cask.sh` when encrypting | the builder | 0600 | anyone who can read the checkout's owner's files | n/a | n/a | open: should live outside the tree (see findings) |
| apt signing key (Phase 14) | `GNUPGHOME=/var/lib/unheaded/apt/gnupg` drafted (account `unheaded-apt`, uid 16738 not yet in `pkg/uids`: E4 open); `DEB_SIGN_KEY` signs `SHA256SUMS` | per E4 | 0700 drafted | the signing account | `Release.gpg` / `InRelease`; east trusts `/etc/apt/keyrings/unheaded.gpg` via `signed-by=` | n/a | no |

## Headers, cookies, tokens

| what | listener/file | uid | mode | reachable from | auth | TLS | fuzzed by |
|---|---|---|---|---|---|---|---|
| Service token (Phase 1) | `Authorization: Bearer` on every first-party call; `unheaded token --aud <svc>` prints one for curl | minted by the caller's key | header `{"alg":"EdDSA","typ":"JWT","kid":<16 hex>}`; `alg` exactly `EdDSA` (`none`, `HS256`, lowercase refused before key lookup), `crit` refused, 5-minute life; HS256 is test-only (a test fails if production code opts in) | wherever it is sent | verifier: `kid` in trust.d, `sub` = the file's name, `sub` in the allowlist (else 403), `aud` = the target | sent only over TLS or to loopback (`pkg/wotan-client`; `scripts/run-runbook.py` since this review) | `FuzzEdDSAParse` |
| User token (Phase 4) | issuer JWT, 15 min, `aud` = `unheaded` / `dashboard-backend` / `kanban-app` (issuer `-audiences`) | 16735 signs | verifiers cap lifetime at `AUTH_OIDC_MAX_LIFETIME` (default 1 h; unparseable refuses start) | the BFF holds it server-side; never reaches the browser | JWKS from the issuer: discovery `issuer` must equal the configured one, `jwks_uri` https (plain http only to loopback or with `WithPlainHTTPIssuer`), at most 3 redirects each re-checked, 64 KiB cap, duplicate `kid` refused, only `OKP`/`Ed25519` 32-byte keys kept | Strict | `FuzzJWKSDocument` |
| Session cookie (Phase 6 BFF) | `unheaded_session` on 20000 and 20001 | in-memory store in each BFF (10,000 sessions max) | 32 random bytes; `HttpOnly; Secure; SameSite=Strict; Path=/`, `Max-Age` = token life; a fresh id at every login (a presented one is deleted first: no fixation); the store re-checks the token on every request | the browser | login, logout and every cookie-authenticated write need `X-Requested-With: unheaded` (`/auth/auth.js` adds it) | `Secure` drops only with `AUTH_BFF_DEV_PLAINTEXT` (compose before TLS; logs a warning) | none |
| On-behalf-of | `act` claim in a BFF-minted service token (`Mint(..., WithActor(sub))`), `X-Unheaded-On-Behalf-Of` informational | | the signed `act` claim is what counts; no caller uses it yet | | | | `FuzzEdDSAParse` |
| Security headers (Phase 11) | `httputil.SecurityHeaders` on every first-party router: alert-mailer, chaos-controller, dashboard-backend, kanban-app, kev-poller, monad, sophia, steel-echo, sword, unheaded-auth, unheaded-daemon, wiki-server, architect, captain, micromanager, timeguru, wotan (+ its middleware), WAF response inspection | | `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`, `Permissions-Policy` (camera, mic, geolocation off), CSP, `Cache-Control: no-store` where set, HSTS (1 year) only when the request arrived over TLS | | | | no |

## Workflows and supply chain

| what | listener/file | uid | mode | reachable from | auth | TLS | fuzzed by |
|---|---|---|---|---|---|---|---|
| `release.yml` (Phase 12) | GitHub Actions, `on: push: tags: v*` only | GitHub runner | top-level `contents: read`; the `release` job alone gets `contents: write`, `id-token: write`, `attestations: write` | whoever can push a `v*` tag | keyless cosign: Fulcio certificate identity = this workflow at the tag; `scripts/verify-release.sh` pins `--certificate-identity https://github.com/<repo>/.github/workflows/release.yml@refs/tags/<tag>` and the OIDC issuer | n/a | `verify-release.sh` offline test (6/6, CI) |
| What a compromised `release.yml` run could sign | any blob, under the identity of that workflow at that tag | | | a tag pusher, or a malicious action in the job (all actions pinned by SHA, `check-actions-pinned`) | a signature verifies only for that tag's identity, so it cannot re-sign an older tag; it can publish a signed malicious release for a new tag. Mitigation: tag protection (GitHub setting, Stevie) | | |
| Scorecard (`scorecard.yml`) | push to develop + weekly | runner | `read-all`; job `security-events: write`, `id-token: write` (publish_results) | | OIDC token used only to publish to the OpenSSF API | | |
| CodeQL (`codeql.yml`) | push, `pull_request`, dispatch | runner | `contents: read`; job `security-events: write` | a fork PR gets a read-only token | | | |
| Debs (Phase 14) | 24 debs from `scripts/lib/deb-packages.sh`; `SHA256SUMS` + `SHA256SUMS.gpg` | | | | apt verifies `Release` against the E4 key | | |
| `unheaded-<svc>` postinst runs keygen as root | `/opt/unheaded/bin/unheaded-keygen -owner <account> <identity>` | root (dpkg) | argv fixed at build time from `deb-packages.sh`; no user input; an existing key is never touched; a unit whose TLS files are missing is never restarted | dpkg | | | no |
| Akira exec checks (Phase 10) | `type: exec` targets in `configs/akira-{west,east}.yaml`: `/usr/bin/python3 -B /opt/unheaded/scripts/run-runbook.py .../akira-cert-expiry-check.yaml` hourly, 30 s timeout | `unheaded-akira` | argv only, no shell (`exec.CommandContext`); the compose akira image now carries python3, py3-yaml, bash, coreutils, openssl | Akira's `--config` (`/etc/unheaded/akira.yaml` via `AKIRA_OPTS`, operator-installed); runner and runbooks root-owned under `/opt/unheaded` (`unheaded-configs` deb, files 0644) | | | no |
| Sealed cask (Phase 13) | `BINDING_RUNE.sig` (ML-DSA-65 JSON seal over `BINDING_RUNE.sha256`) | | tar checked from headers before writing: one `sealed-cask-*` root, clean relative names, no links or devices, 2 GiB per file, 8 GiB total, no trailing bytes | | trusted `build.pubkey` (repeatable for rotation) and an exact file set; `--insecure-sha-only` keeps the old sha-only check, bannered | | `FuzzVerify` |
| Rust and Python token minters (Phase 7) | `crates/svc-token` (ring, base64; `cmd/trace-collector`, `cmd/ebpf-collector`), `scripts/svc_token.py` | the calling process | tokens byte-identical to Go's (`tests/svctoken` cross-language test) | | | Rust clients have no TLS yet (ebpf-collector cannot reach a TLS Wotan) | no |

## Host privilege

| what | listener/file | uid | mode | reachable from | auth | TLS | fuzzed by |
|---|---|---|---|---|---|---|---|
| `unheaded-heimdall.service` (Phase 9) | reads the whole filesystem for drift; manifest `/etc/unheaded/heimdall/mjolnir.yaml` (root-owned) verified against `heimdall-manifest.pub` | 16736 | `AmbientCapabilities=CAP_DAC_READ_SEARCH` (bounding set: that one), `ReadOnlyPaths=/`, writes only `/var/lib/unheaded/heimdall`; no PrivateTmp/PrivateDevices; BPF hooks off | | publishes signed `drift.detected.<node>` to Wotan with its own service token | CA `/etc/unheaded/tls/ca.pem` | `FuzzVerifyEvent` |
| `unheaded-auth.service` | see Listeners | 16735 | `CapabilityBoundingSet=` (empty), `MemoryDenyWriteExecute`, `ProtectProc=invisible`, `UMask=0077`, state dir 0700 | | | | |

## Fuzz re-run (Step 385)

Every fuzz target this sprint added, plus the targets in packages it changed, 90 s each with
`-parallel=2` on west (Go 1.27.3), 2026-10-27:

| package | target | added by | result |
|---|---|---|---|
| `pkg/auth` | `FuzzEdDSAParse` | S-IRON-GATE Phase 1 | PASS, 90.170 s |
| `pkg/auth` | `FuzzJWKSDocument` | S-IRON-GATE Phase 1 | PASS, 91.048 s |
| `pkg/auth` | `FuzzTrustDirFilenames` | S-IRON-GATE Phase 1 | PASS, 90.065 s |
| `cmd/unheaded-auth` | `FuzzLoginBody` | S-IRON-GATE Phase 4 | PASS, 91.042 s |
| `cmd/unheaded-auth` | `FuzzUsersFile` | S-IRON-GATE Phase 4 | PASS, 91.058 s |
| `pkg/sealedcask` | `FuzzVerify` | S-IRON-GATE Phase 13 | PASS, 91.027 s |
| `pkg/wotansig` | `FuzzVerifyEvent` | S-QUANTUM-ARMOR (package changed in Phase 9) | PASS, 90.095 s |
| `services/wotan/internal/api` | `FuzzLoadTopicConfig` | S-QUANTUM-ARMOR (signed prefixes changed in Phase 9) | PASS, 91.147 s |
| `cmd/yggdrasil-evidence` | `FuzzReadPack` | S-QUANTUM-ARMOR (package changed in Phase 13) | PASS, 91.080 s |
| `cmd/yggdrasil-evidence` | `FuzzValidateManifest` | S-QUANTUM-ARMOR (package changed in Phase 13) | PASS, 91.025 s |
| `cmd/alert-mailer` | `FuzzHeartbeatRequest` | S-WELL-SPRING (router changed in Phase 11) | PASS, 90.063 s |
| `services/wotan/internal/middleware` | `FuzzClientIP` | S-CLEAN-SLATE (Wotan middleware changed in Phase 11) | PASS, 90.060 s |

New crashers written to `testdata/fuzz`: none.

No fuzz target covers `pkg/bff` (login body, cookie parsing, CSRF check), the API-key store, or the
nginx/apt path: those are Lich targets (`tomb/lich/IRON-GATE-HOOKS.md`).

## Gates (Steps 387, 389)

Run in a worktree at `5cd72c8c` plus this review's changes, exit codes read directly, 2026-10-27.

- Dependencies and licences: `go mod tidy -diff` empty; `check-dependency-baseline` PASS (743 packages, no
  addition); `go.mod` unchanged since the sprint base `1f44cc46`; SPDX header missing on 0 Go files
  repo-wide and on 0 of the 283 `.go/.rs/.sh/.sql/.py` files added since `1f44cc46`; `check-tool-freshness`
  PASS (cosign `[ok]`); govulncheck v1.9.0 (DB 2026-10-27): no vulnerability reachable from our code (3 in
  required modules, not called).
- PASS: check-gitleaks, check-secrets-baseline, check-gosec-ratchet, check-systemd-units,
  check-unit-binaries, check-compose-hardening, check-compose-bind-nesting, check-compose-log-caps,
  check-image-tags, check-actions-pinned, check-crypto-inventory (after regenerating for
  `tomb/lich/seeds/auth/gen`), check-pq-kex, check-well-migrations, check-well-lint, check-runbooks,
  check-js-lint, check-manifest-yaml, check-hadolint, check-ruff, check-k8s-well-initdb,
  check-prometheus-rules, check-no-client-golang, check-haproxy-cfg, check-helm-security,
  check-tmp-log-baseline, check-zhen-mcp, check-python-syntax, check-python-audit, check-kanban-genesis,
  check-evidence-pack, check-timeline-freshness `--check`, test-check-perf-regression, shellcheck
  `-S warning` on the sprint's scripts, `scripts/compliance/check-service-auth.sh` (all 13 first-party
  services), clippy on the three Rust workspaces the sprint touched (`crates/svc-token`,
  `cmd/trace-collector`, `cmd/ebpf-collector/collector`: 0 errors), `tests/runbooks` (226 passed).
- FAIL, known: `scripts/compliance/check-edge-tls.sh` (no TLS 1.3 policy on traefik; risk accepted by
  Stevie until 2026-12-29, register entry `gate-script:scripts/compliance/check-edge-tls.sh`).
- FAIL, not this sprint's code: `check-ebpf-size-pin.sh` measures 919,096 bytes against the 918,840 pin with
  the pinned rustc and no eBPF source change since the pin; the object embeds the checkout's absolute
  path (BTF file names), so a build anywhere but the pinned checkout differs. The main checkout's object
  is 918,840. The gate is a build-host promotion gate; it should either build with
  `--remap-path-prefix` (and re-pin) or say it must run from the canonical checkout.
- Not run: `check-gates-can-fail.sh` (the coordinator times it), `check-ebpf-loads.sh` (needs root),
  the full `check-clippy.sh` ratchet (every Rust workspace from cold; CI runs it).

## Residual risk

- The dev-certs CA (`configs/certs/dev`, `scripts/gen-dev-certs.sh`) is a lab CA whose key sits in the
  developer's checkout. Hosts issue from the Kingdom CA (`runbooks/pki/generate-kingdom-certs.yaml`).
- The issuer has no MFA and no WebAuthn (first cut, ADR-103). A stolen password is a login until the
  lockout or a users-file edit.
- API keys live 90 days by default; nothing rotates them for you.
- Sessions are in memory: a BFF restart logs everyone out (accepted), and a second BFF replica would not
  share them.
- Behind traefik every browser shares one source address, so the BFF's 60/min per-client limit and the
  issuer's per-source lockout become global: one attacker can lock out every user (availability, not
  confidentiality).
- `jwks_uri` may name any https host: whoever controls the issuer's discovery document can point
  verifiers at another internal URL (a blind GET; the issuer is trusted anyway).
- Compose ports are reachable from the home LAN (Docker bypasses the INPUT policy); auth and TLS are the
  boundary there.
- A compromised `release.yml` run or tag pusher can publish a correctly signed release for a new tag
  (tag protection is a GitHub setting, not in the repo).
- The Rust collectors speak plaintext to Wotan; once Wotan's TLS is on, `ebpf-collector` cannot publish
  (follow-up: rustls on aws-lc-rs for the hybrid group).
- `crates/svc-token` puts `ring` back into `cmd/trace-collector`'s lock (S-QUANTUM-ARMOR had removed it
  there, ADR-004). Not a new identity in the shrink-only baseline (`cmd/ebpf-collector` already pulls it),
  so `check-dependency-baseline` passes; the ADR-096 note for the new use is open.

## Findings of this review

| finding | where | disposition |
|---|---|---|
| `scripts/run-runbook.py` sent its service token over plain http to any host (wotan-client withholds it except to loopback; Phase 7 left the decision to Phase 8, which did not take it) | `Progress.__init__` | **fixed here**: `token_in_clear()`; no token over plain http to a non-loopback host, one line on stderr (`tests/runbooks/test_run_progress_tls.py::test_token_only_over_tls_or_loopback`, 8 cases) |
| `deploy/systemd/README.md` installed `/etc/unheaded/tls/<identity>/` as 0750 `root:unheaded-<unit>`, so Akira's cert-expiry check (Phase 10) could not list or read other services' certificates | README TLS section | **fixed here**: dirs 0755 (keys stay 0640 `root:unheaded-<unit>`), matching the Phase 10 note |
| `secrets/cask.age.key` is generated inside the repo checkout (gitignored), next to code a build copies | `scripts/build-sealed-cask.sh:172` | open (raised by Phase 13): move it under the builder's home or the custody account with E1 |
| `pkg/compliance/crosswalk` `TestRepoRegister` fails on develop: register entry `host-probe:west:legacy-apt-repo-18888` names a probe no catalog control declares (and `collect.Probes` has no such probe) | `compliance/findings/register.yaml` (coordinator ledger) | open, coordinator: add the probe + a catalog evidence row, or re-source the entry |
