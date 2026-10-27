<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# S-IRON-GATE progress log

Plan: `docs/battle-plans/S-IRON-GATE-battle-plan.md`. This file is the session log the plan refers to, and where
`[STUCK]` / `[ESCALATE]` markers live; step ticks live in the plan.

Skip Protocol acknowledgment: "I will skip any step after 3x its estimate or 2 failed debug attempts, mark
it [STUCK] with what I tried, commit known-good state first, continue with the next step that does not depend
on it, and finish with a STUCK/ESCALATE report ordered by how many steps each item unblocks."

## Session S24, 2026-10-27 evening (unattended, Stevie's "churn unattended multi agent")

Base: develop `1f44cc46` (S-CLEAN-SLATE shipped; Go known failures 0). Same pattern as Sprint 1: a coordinator
on develop and worktree agents under one brief (`EXEC-BRIEF-v2`: reset to develop first, never push, no host
changes, no shared-ledger edits, gate exit codes read directly). Phase 0 premises are checked per slice by each
agent, since SITUATION was written at `a7fe3acd` and Sprint 1 changed many of them.

First wave: Phase 1 (pkg/auth core, Steps 29-66) in a worktree agent; next: Phases 2+3 after it merges, then 4; Phases 11 and 12 can run beside them (not beside S-WELL-SPRING Phase 8: both touch every router).

### Phase 12 (Steps 304-328): done by the coordinator on develop, 2026-10-27 evening

Commits 5160faa6, 7a27fd19, 5993ec31, 97ade150 and the docs commit. release.yml builds all 19 binaries, the debs and the eBPF
objects, signs every file with keyless cosign, attests the SBOM, adds GitHub build provenance; `scripts/verify-release.sh` with an
offline test (6/6) run in CI. Scorecard and CodeQL workflows. cosign 3.1.3 pinned. [DECIDE] deviation Step 319: the eBPF size pin
is a build-host promotion gate (toolchain-bound), not a CI step; listed under NEEDS_HARDWARE. First release, Scorecard and
CodeQL runs only happen after Stevie pushes (and tags).

### Phase 1 (Steps 29-66): merged ed46d54f

Ed25519 service tokens; trust.d with `sub` bound to the `.pub` file name (a service cannot mint another's identity); caller
allowlist answers 403; JWKS-verified user tokens; `alg` exactly `EdDSA` (none/HS256/lowercase refused before key lookup), 15-minute
cap, `crit` refused; HS256 test-only with a test that fails if production code opts in. ADR-103 gained measurements (west: Ed25519
verify 34.1 µs, ML-DSA-65 11.7 µs; token 308 B vs ~4,640 B). Full suite 0 FAIL before and after.

Phase 0 corrections: Step 17 now 11 of 12 services open (akira, architect, captain, cuirass, dashboard-backend, kanban-app,
micromanager, monad, sophia, timeguru, wotan), not 8; Step 19's compose hit is a commented example (effectively still true).

Hand-offs to Phase 6: switch each service from `SetupMiddleware` (now fails closed when misconfigured) to `NewServiceMiddleware`
(returns errors so a service can refuse to start); dashboard-backend's own HS256 authenticators now refuse JWTs (API keys still
work; compose runs it with auth off, so no live impact) and need `NewServiceAuthenticator`; `raft/start-zhen.sh` and
`cmd/sword/main.go` still mention `AUTH_JWT_KEY`; `pkg/auth/README.md` says "Stub" (Phase 16). Phase 3 (API keys) touches
`pkg/auth/setup.go` next; golangci-lint panics under Go 1.27.3 (tool mismatch, not code).

### Phases 2, 3, 4 merged (b341e904 Phase 3; 362755e9 Phases 2+4)

Phase 3: `uhk_` keys (memory/file/Well stores), migrations 035 (`app_auth` role, in `postgres`) and 036
(`api_keys`, in `unheaded_app`), `unheaded-cli apikey`, gitleaks `uhk_` rule. Deviations: role `observer` not
`viewer` (pkg/auth has no viewer); `wellint` tag + `WELL_TEST_APP_DSN`; own `AUTH_WELL_*` env. pkg/auth now imports
pkg/database (lib/pq linked by auth users). Old `AUTH_API_KEYS` lists still work with a deprecation warning.
LIVE: apply 035/036 to the Well; `APP_AUTH_PASSWORD` in `.env` then `scripts/well/set-role-passwords.sh`.

Phases 2+4: `unheaded-keygen` (own deb, every service deb depends on it), key dir 0711 (0750 root-owned would
block each service reading its key), keys named by each binary's auth identity (cuirass = `unheaded-daemon`),
unit env on 13 units, compose `auth` on `data`, `-dev-plaintext` issuer at `http://auth:19020`. No WebAuthn in the
first cut. No argon2id CBOM row (symmetric/hash primitives are out of the inventory by design). Merge: `auth`
moved to the journald logging anchor; sysusers/uids/CBOM conflicts resolved. LIVE: Steps 144-148 (compose up
auth, login round trip) and one-time `sudo scripts/gen-dev-keys.sh` after `make binaries`.

Coordinator fix (fefac3e0): `JWKSVerifier` passed lifetime 0, so user tokens of any lifetime were accepted. Now
capped at 1h by default (`AUTH_OIDC_MAX_LIFETIME`; unparseable value refuses start). Open for Phase 6: the verifier
checks `aud` against the service name while the plan says user tokens carry `aud=unheaded`; the issuer works
around it with `-audiences` + a login `audience`, so the BFF requests e.g. `dashboard-backend`. Plain-http JWKS
fetch is loopback-only, so no other container can verify compose issuer tokens until TLS (Phase 8). Deb name
`unheaded-unheaded-auth` (double prefix, Phase 14). Jenkinsfile `deb_postinst` call is out of date (Phase 14).

### Phase 5 code half (coordinator, deb25a08)

RBAC behind auth in Cuirass (writes: admin/operator/service); CLI mints `unheaded-cli` tokens for aud
`unheaded-daemon` (Cuirass's identity, not `cuirass`); Sword mints its own (CUIRASS_API_KEY now fallback);
`cuirassdriver.Config.Authorizer`. compose + unit: `AUTH_ENABLED=true`, allowlist sword, unheaded-cli,
dashboard-backend. check-service-auth: issuer exempt with reason; 10 services remain (plan said 7, counted before
the issuer and Akira). Live Cuirass on west runs from compose without auth today (writes refused); Sword is not
running. LIVE 171-174: `sudo scripts/gen-dev-keys.sh` (after `make binaries`), then recreate cuirass + sword and run
Steps 171-174.

### Phase 6 per-service wave merged (0c547c46)

`auth.NewServiceHandler` / `NewServiceChain` (bad config refuses start; RBAC behind auth; per-route open reads).
Allowlists (identity = key name = aud): protocol-api <- unheaded-cli; kanban-app <- dashboard-backend, zhen-agentd,
micromanager; timeguru <- kanban-app, micromanager; captain/architect/micromanager <- each other + dashboard-backend;
monad, sophia, sword, zhen-agentd <- unheaded-cli, dashboard-backend; wiki-server <- none.
AUTH_ENABLED on in compose+units for timeguru, captain, architect, micromanager, monad, sophia (+ sword, already on,
now with an allowlist: before it every service token got 403); zhen-agentd unit; kanban-app left OFF (the browser UI
writes with no credential: waits on board login, Steps 211-216); protocol-api and wiki-server have no compose/unit.
Found: an observer key could ask/confirm/exec/remember on zhen-agentd (200) -> 403. Sword needs its own RBAC policy
(default reserves `/api/v1/deploy*`, which also matches `/api/v1/deployments`, for admin/operator). protocol-api's gRPC
(17101) now checks tokens too. `/etc/zhen-agentd.env` is read after the unit and can override. Legacy
`services/timeguru/main.go` (never built) left on SetupMiddleware. check-service-auth: 4 remain (akira,
dashboard-backend, kanban-app, wotan); Step 185's expected list should include akira and kanban-app until 216.
**LIVE:** `make binaries && sudo scripts/gen-dev-keys.sh` FIRST (containers must own their key files); rebuild
kanban-app together with timeguru (else the board loses its timeline); recreate timeguru, captain, architect,
micromanager, monad, sophia, sword; probe anonymous write 401 and /health 200 each; update + restart zhen-agentd unit.

### Phase 6 BFF merged (e4603171)

`pkg/bff`: login/me/logout, swappable session store, cookie authenticator (re-checks the stored token each request),
CSRF header, login rate limit (60/min per client; behind traefik all users share the BFF's address), `AuthorizeOutgoing`;
fake issuer in `pkg/bff/bfftest`. `pkg/auth`: `ServiceRoutes.Authenticators`, `WithPlainHTTPIssuer`, `RoleViewer` in the
default read policies, `Mint(..., WithActor(sub))` (user in the service token's `act` claim; no caller yet). One
`auth.js` at `pkg/bff/static/auth.js`, served at `/auth/auth.js` by both backends (wraps fetch for the CSRF header).
The dashboard requires a login for every `/api` read and WebSocket; the board stays publicly readable (Step 215).
Profile switch `POST /api/v1/compliance/profile` writes as `config_admin`: 503 until `WELL_CONFIG_ADMIN_PASSWORD` is
decided. The BFF uses unheaded-auth's password `/login`: another OIDC provider would need a code flow (ADR-103 note).
ADR-099 §5's signed change record: the Well history row records who and why but is not signed (follow-up).
check-service-auth: akira, wotan remain (Steps 209/222 should expect those two until Akira's step).
**LIVE 202-205, 208-210, 215-216:** `make binaries && sudo scripts/gen-dev-keys.sh`, then recreate `auth` (now
`-audiences unheaded,dashboard-backend,kanban-app`), `dashboard-backend`, `kanban-app` (kanban only after the E-A batch).

### Phase 7 code merged (701428ef): all 13 first-party services on auth

`wotanClient.InitServiceIdentity("<id>")` once per process covers healthreport/discovery/logagg clients; a missing key
warns and runs without a token. Tokens go over plaintext gRPC with a one-time warning until Phase 8 (plan said
TLS-only: Phase 8 must switch it). `auto_approve` = verified identities; old display names moved to
`legacy_display_names` (count only with Wotan's auth off: keeps Appendix A3 rollback). Topic reads need a token; a
publish under another identity's subscriber/sender id is refused; admin API needs role `admin` + the admin key, off by
default. Akira: `GET /api/v1/status` stays open (runbooks curl it), empty allowlist. Wotan clients on its allowlist:
akira, anomaly, architect, captain, chaos-controller, dashboard-backend, heimdall, kanban-app, kev-poller, micromanager,
monad, perf-benchmark, sophia, sword, timeguru, trace-collector (Go), unheaded-baseline, unheaded-daemon, zhen-agentd,
zhen-claude (gateway/shield authenticate but are not auto-approved; unheaded-cli listed, no caller).
**Not converted, refused once Wotan's auth is on:** `scripts/run-runbook.py` (runbook-runner: Champion 12's step
progress goes dark), the Rust `cmd/trace-collector` gRPC publisher and `cmd/ebpf-collector` HTTP publisher (eBPF flows!),
`cmd/demo-trace-injector`, anonymous curls in `scripts/demo-start.sh`, `scripts/bare-metal/validate-cross-host.sh`,
`runbooks/wotan/cluster-*`. The Rust collectors need a token before Wotan's auth goes live or the dashboard loses flows.
perf-benchmark e2e reads the dashboard's `/api/v1/ebpf/events`, which needs a BFF login since Phase 6.
LIVE 239-250: `make binaries && sudo scripts/gen-dev-keys.sh` first (Wotan refuses to start with auth on and nothing to
verify against; every binary without a readable key loses Wotan). East needs keys for every service before a deb upgrade.

### Non-Go Wotan publishers converted (028f2e72)

Correction to the Phase 7 note: the dashboard does not lose eBPF flows. `crates/svc-token` (Rust, ring) and
`scripts/svc_token.py` mint tokens byte-identical to `auth.SignEdDSA` (`tests/svctoken` cross-language test).
The Rust `cmd/trace-collector` publish is a stub and nothing deploys it (production traces are trace-collector-go,
converted in Phase 7). `cmd/ebpf-collector`'s gRPC and HTTP publishes were malformed and its default ports pointed at
9080/9090 (Prometheus): fixed; it publishes as new identity `ebpf-collector` (on Wotan's allowlist and auto_approve;
key by hand: `unheaded-keygen ebpf-collector`). run-runbook.py progress carries a token (identity follows the key:
`zhen-agentd` under the Champion, else `unheaded-cli`) and accepts an https Wotan (`UNHEADED_TLS_CA_CERT`).
`unheaded token --aud <svc>` prints a 5-minute token for curl; demo-trace-injector is `unheaded-cli`.
Still broken regardless of tokens: `scripts/bare-metal/validate-cross-host.sh` and `runbooks/wotan/cluster-*` curls
(wrong body, admin API). `docker/services/trace-collector/Dockerfile` go-builds a Rust directory and cannot build.
Rust clients have no TLS (would need rustls on aws-lc-rs for the hybrid group); Phase 8 must decide whether
run-runbook.py should refuse a token over plain http, matching wotan-client.

### Phase 8 code merged (19a968a9)

`scripts/gen-dev-certs.sh`: dev CA + per-service cert dirs (13 services + unheaded-cli + gateway; SANs = compose name,
auth identity, localhost, 127.0.0.1, ::1); under sudo each dir goes to the service's uid; re-run keeps the CA;
`configs/certs/` gitignored. Listeners refuse to start without cert/key/CA (no plaintext fallback); a non-loopback
plaintext listener logs FND-017. Every first-party link uses TLS under `UNHEADED_TLS_ENABLED=true` (healthreport,
Akira, dashboard, kanban->timeguru, sword/chaos->Cuirass, BFFs, JWKS, CLI). wotan-client: TLS with CA and server name,
token withheld on plaintext except loopback; Wotan cluster replication no longer drops to plaintext. One
`insecure.NewCredentials` site left. `cmd/healthprobe` replaces busybox wget in healthchecks. Compose: issuer at
`https://auth:19020` (no `-dev-plaintext`), BFFs without `AUTH_BFF_DEV_PLAINTEXT`, sword -> `https://cuirass:19006`,
VictoriaMetrics scrapes and two traefik routes over https. Units: 13 server units get TLS env
(`/etc/unheaded/tls/<identity>/`), plus CA-only env for chaos-controller, kev-poller, anomaly, baseline,
trace-collector, zhen-agentd. New `UNHEADED_TLS_PROFILE=strict|preferred` (strict default; ADR-105 wanted preferred on
outside-facing listeners: now explicit).
**LIVE 262-267, 269:** `scripts/gen-dev-certs.sh && sudo scripts/gen-dev-certs.sh` first. Expect: VictoriaMetrics
v1.108.1 (Go 1.23) and maybe traefik v3.3.7 fail the hybrid handshake (bump images or `preferred` on those links); the
Secure session cookie does not hold through traefik's plain-http `web` entrypoint (use https://host:20000 or
`websecure`); host Prometheus jobs (wotan, unheaded-daemon, dashboard, zhen-agentd) need `scheme: https` + CA in the E3
window; east debs must ship certs (Phase 14). The Rust ebpf-collector has no TLS, so it cannot reach a TLS Wotan
(follow-up: rustls on aws-lc-rs for X25519MLKEM768). FND-017: closed in compose once 264-265 pass; hosts after E3.

### Phase 10 merged (Steps 286-293)

Akira `type: exec` targets (`argv`, `interval`, `timeout`, `env`); cert-expiry runbook hourly via
`/usr/bin/python3 -B /opt/unheaded/scripts/run-runbook.py` (configs/akira-{west,east}.yaml); `unheaded-configs` deb
ships the runner and depends on python3, python3-yaml, openssl; the compose akira image gains python3, bash, openssl
(attack-surface note for Phase 15). Runbook reads `$UNHEADED_TLS_DIR` (default `/etc/unheaded/tls`), never opens keys,
reports instead of rotating (the old auto-rotate called a runner flag that does not exist). Host perms for E3:
`/etc/unheaded/tls/<id>/` listable by unheaded-akira (dirs 0755, certs 0644, keys 0640). West and east cert-expiry
reports share one verdict. `runbooks/pki/rotate-service-cert.yaml` still targets the old layout (follow-up).
LIVE Step 294: check `/api/v1/status` for "cert-expiry" (not `/api/v1/checks`); re-issue with
`sudo CERT_VALIDITY=2m scripts/gen-dev-certs.sh timeguru`; expect an immediate failing vote (certs backdated 5 min).

### History correction (coordinator, 2026-10-27 ~22:00Z)

The security-headers commit (Steps 298-300) was staged with `git add -A cmd services pkg`, which swept in the
operator's untracked 10 MB `cmd/host-agent/host-agent` binary. Nothing was pushed. The binary was removed from all 120
unpushed commits after that point with `git filter-branch --index-filter`, those commits were re-signed with
`scripts/resign-unpushed.py --merges` (all 254 unpushed commits G), the binary was restored on disk untracked (same
sha256), and every commit hash cited in the progress logs, OUTSTANDING and next.md was remapped to the new commits.
Commit messages in that range that cite earlier hashes from the same range still show the pre-rewrite values.
Backups: `refs/backup/develop-before-host-agent-purge` (contains the binary: delete once satisfied) and
`refs/backup/develop-before-resign`. Lesson: stage named files, never `git add -A <dir>` while untracked operator files exist.
