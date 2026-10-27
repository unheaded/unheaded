<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# S-IRON-GATE BATTLE PLAN: 17 Phases, 402 Steps

**Date**: 2026-10-27
**Sprint**: Sprint 2 of 7 (NEXT-7-SPRINTS-2026-10-27), IRON GATE. Services authenticate each other, every internal link is TLS, and the release pipeline signs what it ships.
**Prerequisite**: branch based on `a7fe3acd` (develop) or newer; S-CLEAN-SLATE exit gates passed (every CI gate green on develop, M-001..M-008 closed); `go build ./...` clean; `go test ./...` baseline recorded in Phase 0. Nothing in this plan touches Doom.
**Target**: `scripts/compliance/check-service-auth.sh` passes (every first-party compose service sets `AUTH_ENABLED=true` and refuses an unauthenticated write with 401 while a correctly tokened one gets 200); every service holds its own Ed25519 key, verifiers hold only public keys, and `AUTH_JWT_KEY` exists in no unit or compose file; Wotan and the eleven services talk TLS 1.3 with hybrid ML-KEM groups and a classical-only offer is refused (FND-017 closed); Wotan auto-approve binds to the caller's token identity, not a self-reported name; `release.yml` produces cosign signatures and SLSA provenance instead of `echo TODO`; debs are signed and published from an apt repo on west, and east installs them with `apt install` (M-050..M-056, M-060..M-067, M-201).
**Estimated Duration**: 38-48 h of execution across 8-10 sessions. Five waits on Stevie (E1 build-key custody, E2 drift-key custody, E3 west/east host windows, E4 apt-repo signing-key custody, E5 ADR-004 admissions), none of which blocks Phases 1-8.
**Agent Strategy**: Coordinator owns Phases 0, 7, 8, 9, 14 (Wotan cutover, TLS cutover, host-touching, iterative). Phases 1+3, 2+4, 10+11+12 can run in parallel (Appendix B), each agent in its own git worktree off develop. Hosts are read-only for every agent unless the step is covered by an answered [ESCALATE].
**Commit Cadence**: every 5 steps (max(3, min(5, 402/20)) = 5), at every exit gate, before every STUCK skip, after every red-to-green test. Never commit red. Never push (Stevie pushes). Signed if `git commit-tree -S` succeeds in Step 6, otherwise `--no-gpg-sign` (memory: feedback_unsigned_commits_when_afk) and `scripts/resign-unpushed.py` later.
**Stuck Protocol**: skip after 3x the step's `~time` or 2 failed debug attempts; write the STUCK marker into this file, commit, continue with the next step that does not depend on it (Appendix B dependency map).

**Builds on (cited, not re-derived):**

| Source | What this plan takes from it |
|---|---|
| ADR-103 (Proposed, design only) | The whole auth design: Ed25519 service JWTs, per-service 0400 keys + `trust.d`, caller allowlists, JWKS for humans, BFF cookies, `uhk_` API keys, Wotan last. Its four open questions become [DECIDE] steps using the ADR's own recommendations (Steps 7-10). |
| `docs/sweeps/OUTSTANDING-2026-10-27.md` E04, E05, E13 | The item list this plan must cover: M-050..M-056, M-060..M-067, M-201. Appendix D maps each to steps. |
| FND-017, FND-018, FND-019 (`compliance/findings/register.yaml:308-360`) | Verify commands: TLS negotiates X25519MLKEM768 and refuses X25519-only (`scripts/probe-tls-groups.sh`); `yggdrasil-evidence verify --pubkey build.pubkey` on a real pack; a provoked drift arrives on `drift.detected.<node>` and re-verifies. |
| ADR-105 (Proposed), `pkg/tls/pq.go` | Strict profile (`ApplyStrict`, hybrid groups only, TLS 1.3 floor); Wotan already applies it when `-enable-tls` is set (`services/wotan/cmd/wotan/main.go:258,328`). |
| ADR-034 Phases 7/8, `runbooks/pki/akira-cert-expiry-check.yaml` | The cert-expiry runbook exists; what is missing is Akira registering it as an exec check and one end-to-end rotation test. |
| ADR-026, ADR-085 (Planned), `scripts/build-all-debs.sh`, `Jenkinsfile` | reprepro on west port 18888 (the `nginx` sysuser for it already exists), signed packages, east installs via apt. GHA is the live runner ([DECIDE], Step 11). |
| ADR-043 #4, ADR-010, `pkg/gungnir`, `cmd/yggdrasil-evidence` | ML-DSA-65 seal/verify code that already exists; the build key (FND-018) is the missing custody item. |
| ADR-089 entry gate item 4 | The 901,888-byte `monad-cpu-ebpf --features ascend-linux` pin becomes a script, not a sentence. |
| ADR-102, `docs/policy/image-tag-baseline.txt` | Shrink-only image-tag baseline; the 7 sophia-eye images are the remaining unpinned rows. |
| `pkg/uids/uids.go`, `pkg/ports/ports.go`, `deploy/sysusers.d/unheaded.conf` | Next free Kingdom-tier uid 16735 and port 19020 for `unheaded-auth`; the one-account-per-service rule and its gate (`scripts/check-systemd-units.sh`). |
| S-REAL-STEEL battle plan | Legend, DoD-STD, [DECIDE]/[ESCALATE] formats, the Phase 0 PREFLIGHT pattern, the worktree-per-agent rule. |
| Code read 2026-10-27 at `a7fe3acd` | Every path:line in SITUATION. |

---

## LEGEND

```
[B]            Bash command (run directly)
[V]            Verification step (MUST pass before proceeding)
[D]            Debug step (only if the prior step fails)
[W]            Write/create file
[R]            Read/inspect file
[S]            Sudo/elevated privileges required
[P]            Parallelizable with other [P] steps in the same phase
[P:N]          Parallelizable with Phase N
[SEQ]          MUST be sequential
[C]            Commit checkpoint
[ENV]          Environment setup/check
[BUILD]        Build/compilation step
[TEST]         Test execution
[CODE]         Code implementation (test first: red, then green)
[DESIGN]       Design decision recorded in-repo
[STUCK]        Step skipped via Skip Protocol (needs human)
[BLOCKED]      Blocked by an upstream [STUCK] (never used for decisions)
[DECIDE]       Decision with a pre-seeded RECOMMENDATION; proceed without asking
[ESCALATE]     Needs Stevie. STOP this chain; continue independent chains
[AFTER-E1]     Runs only once E1 is answered; skipped (not STUCK) until then
[PREFLIGHT]    Hypothesis verification (Phase 0 only)
[REGEN]        Regenerate derived artifacts
[AUDIT-UPDATE] Mark a finding resolved in its tracking doc
[DOC-UPDATE]   Update downstream docs (wiki, CLAUDE.md, next.md)
[SECURITY]     Security review step
[COMPLIANCE]   License/SBOM/dependency check
[BARE-METAL]   Requires the real hosts (west/east), real kernel, real LXD
[LIVE-RO]      Live read-only verification on west/east (GET, list, show)
[LIVE-RW]      Live host-changing step; only under an answered [ESCALATE]
```

**[DECIDE] format**: RECOMMENDATION, Rationale, "Override ONLY if". The agent proceeds with the recommendation.
**[ESCALATE] format**: Context, the exact question(s), "Why no recommendation" (or why the call is Stevie's even with one). The agent stops that chain only.
**Time tags**: `~Nm` is expected wall-clock. 3x over = Skip Protocol.

**DoD-STD (every phase's Definition of Done includes these; Micromanager gate):**
1. Every step executed or STUCK-logged in this file.
2. Every [V] in the phase passed.
3. Zero NEW test failures versus the KNOWN FAILURES BASELINE (`go test ./...`, plus `-race` on touched packages).
4. Commit checkpoint made; tree clean (`git status --short` empty).
5. No new dependency without a Barrister/ADR-004 check (`go mod tidy -diff` empty, `scripts/check-*` green).
6. Attack surface unchanged, or the change is listed in Phase 15's review table.
7. Every claim of "real" in a commit message has a [LIVE-RO] or [LIVE-RW] step that proved it, cited by step number.
8. Zero user data access: no step reads packet payloads, log contents of other services, or user data; probes read metadata only.

---

## SITUATION (verified at `a7fe3acd`, 2026-10-27)

**Auth**
- `pkg/auth` signs and verifies JWTs with HS256 only: `pkg/auth/jwt.go:186` refuses any `alg` but `HS256`; `SignToken` at `:227` emits `HS256`. `ServiceTokenManager` (`service_token.go:39-106`) mints service tokens with the same shared key. No Ed25519, no `kid`, no trust directory, no JWKS client anywhere under `pkg/auth`.
- `LoadServiceAuthConfig` (`pkg/auth/setup.go:17-47`) reads `AUTH_ENABLED`, `AUTH_JWT_KEY`, `AUTH_API_KEYS`, `AUTH_API_KEY_FILE`, `AUTH_JWT_ISSUER`, `AUTH_JWT_AUDIENCE` from the environment. With `AUTH_ENABLED` unset it installs a Noop authenticator.
- 18 binaries wrap their mux with `auth.WrapHandler`/`SetupMiddleware` (cmd/{chaos-controller,dashboard-backend,doom-bridge,kanban-app,monad,protocol-api,sophia,sword,trace-collector-go,unheaded-daemon,wiki-server,zhen-agentd}, services/{architect,captain,micromanager,timeguru,wotan}). `pkg/auth/auth.go:109` already has an `MTLSValidator`; `pkg/auth/middleware_grpc.go:24,55` already has unary and stream gRPC interceptors; `pkg/auth/rbac.go:51,134` has `RBACMiddleware` and `DefaultServicePolicies`; `pkg/auth/apikey.go:61` has an API-key authenticator with a rate counter, keys hashed by `hashKey` at `:189`.
- `scripts/compliance/check-service-auth.sh` FAILS today: 8 of 12 first-party compose services lack `AUTH_ENABLED=true` (cuirass, dashboard-backend, kanban-app, micromanager, monad, sophia, timeguru, wotan). Only `sword` sets it.
- No login, session or cookie code exists in `cmd/dashboard-backend` (`compliance_profiles.go:16` says "until the dashboard has logins and RBAC"). `cmd/unheaded-cli` exists and is the operator CLI ADR-103 names.
- `golang.org/x/crypto v0.58.0` is already in `go.mod:37` (argon2id available, no new module). Ed25519 is stdlib. No JWT library is in `go.mod`; `pkg/auth` hand-rolls JWS, so EdDSA is added the same way.

**TLS**
- `pkg/tls` is complete: `LoadServiceTLSConfig` (`setup.go:30`) reads `UNHEADED_TLS_ENABLED`, `UNHEADED_TLS_CA_CERT`, `UNHEADED_TLS_CERT`, `UNHEADED_TLS_KEY_FILE`, `UNHEADED_TLS_MTLS_ENABLED`; `MaybeWrapServer` (`setup.go:53`), `NewTLSHTTPClient` (`client.go:63`), gRPC credentials (`grpc.go:14,24`), `ApplyStrict` (`pq.go:41`). `scripts/gen-dev-certs.sh` issues a CA and per-service certs into `configs/certs/dev` by default.
- Wotan has `-enable-tls`, `-tls-cert`, `-tls-key` (`services/wotan/cmd/wotan/main.go:421-423`) and applies the hybrid profile on both listeners (`:258-262`, `:328-334`). Nothing sets them: `docker-compose.yml:106-117` is a commented example; `deploy/systemd/unheaded-wotan.service` ExecStart has no TLS flags.
- `pkg/wotan-client` dials with `insecure.NewCredentials()` at `grpc_client.go:81` and `topic_client.go:187` whenever no TLS config is given. No per-RPC credentials (no token in metadata) anywhere in `pkg/wotan-client`.
- All 12 first-party compose services run as their `pkg/uids` uid (`user: 167xx:167xx`) and carry `WOTAN_ADDR`/`WOTAN_GRPC_ADDR`; none carries `UNHEADED_TLS_*`.

**Wotan identity**
- `services/wotan/internal/grpc/topic_service.go:56-67`: `AutoApprover.IsAutoApproved(displayName string)`; the comment says "display_name is self-reported ... not authentication". `configs/wotan.yaml:17-20` lists `service`, `timeguru`, `loadbalancer`.
- Signing policy (`services/wotan/internal/signing/policy.go:17`) hard-codes `config.` and `drift.`; `configs/wotan.yaml:67-69` has `drift.` with `keys: []`, so every drift publish is refused (FND-019 fails closed, as designed).
- `cmd/heimdall-daemon` publishes signed `drift.detected.<node_id>` when given a key (`main.go:331-334` via `gungnir.LoadSeedFile`); no node has a key, no unit runs it (`deploy/systemd` has no heimdall unit).

**Release**
- `.github/workflows/release.yml:91-100`: SLSA and cosign steps are `echo TODO`. The build job compiles only `./wotan/cmd/wotan` and `./wotan/cmd/wotan-cli` (`:45,48`), paths that do not exist (`services/wotan/cmd/wotan` does). It never built a usable release.
- `scripts/build-all-debs.sh:33` lists 9 services and names packages `unheaded-${svc}`, so `unheaded-daemon` becomes `unheaded-unheaded-daemon` (`dist/debs/unheaded-unheaded-daemon_0.1.0~alpha+...`), while `debian/control:18` says `unheaded-daemon`. No signing step, no `reprepro`, no apt repo; ADR-026 planned reprepro on west port 18888 and `deploy/sysusers.d/unheaded.conf` already has `nginx "(apt repo on 18888)"`.
- `debian/unheaded-wotan.postinst` creates a shared `unheaded` user and writes a unit with `User=unheaded` (`:5-7,20-21`), contradicting the one-account-per-service rule the deploy/systemd units follow.
- `Jenkinsfile` exists; `docs/LIVE-PATHS.md` does not list Jenkins. No workflow runs `build-all-debs.sh`.
- No `codeql` or `scorecard` workflow under `.github/workflows`. `.github/dependabot.yml` covers `github-actions` only.
- `docs/policy/image-tag-baseline.txt` (shrink-only) still lists four `deploy/sophia-eye/docker-compose.yml` images unpinned (`vllm/vllm-openai:latest-rocm` x2 sites, `qdrant/qdrant:latest`, `ghcr.io/open-webui/open-webui:main`, `ghcr.io/huggingface/text-embeddings-inference:cpu-1.5`); `alpine:3.24` and `prom/prometheus:v3.15.0` there are pinned.
- ADR-089 entry gate item 4 names a 901,888-byte pin for `monad-cpu-ebpf --features ascend-linux`; no script checks it (`.github/workflows/ebpf.yml:108` runs `check-ebpf-loads.sh`, which does not compare sizes).
- `CHANGELOG.md` last entry is `v0.1.0-alpha` 2026-02-24 and still advertises an mTLS service mesh (`pkg/transport/mtls`) that the E1 sweep deleted.

**Keys and evidence**
- `pkg/gungnir` has `LoadSeedFile`, `LoadPublicKeyFile`, `PublicKeyFromSeed`, `SignEvidence`/`VerifyEvidence`, detached variants, `KeyID`. `cmd/yggdrasil-evidence` has `pubkey`, `sign`, `verify` with a fixture-only key. `scripts/build-sealed-cask.sh` writes `BINDING_RUNE.sha256` with no signature; `verify-binding-rune.sh` checks sha256 only.
- `compliance/crypto/cbom.cdx.json` is generated (`scripts/check-crypto-inventory.sh`); any new key material or algorithm use must regenerate it or the CI gate fails.

**Registries**
- `pkg/uids`: Kingdom tier last used 16734 (Anomaly); 16735 is free. `pkg/ports`: 19000 band last used 19019 (TelemetryAgg); 19020 is free. Both have tests that enforce uniqueness and range.
- `db/migrations`: latest is `034_app_kanban_genesis.sql`; MANIFEST + `well_schema_migrations` ledger (ADR-101); `scripts/check-well-migrations.sh` gates it.

**Escalations** (each stops only its own chain; see Appendix B for what each blocks)

| ID | Question | Blocks |
|---|---|---|
| E1 | Production evidence build key custody (FND-018 / Quantum Armor E4): where the ML-DSA-65 seed lives (sealed file on west under strict ACL + audit log, TPM, or offline), who holds it, how `build.pubkey` is published | Phase 13 Steps 343-345 (raised at Step 342) |
| E2 | Heimdall drift key custody per node (FND-019 / E5): generate on each host and enrol the public half, or generate centrally; run a root-reading daemon on west/east | Phase 9 live half of M-201 (raised at Step 283) |
| E3 | Host windows: install `unheaded-auth` + keys + TLS material on west (and east), install the apt repo on west, `apt install` on east, restart units | Phase 8 Step 268 (host install), Phase 14 Steps 369-381 |
| E4 | apt-repo signing key custody (GPG key reprepro signs `Release` with; ADR-004 exception): who holds it, where | Phase 14 Steps 367-376 (raised at Step 366) |
| E5 | ADR-004 admissions: `aws-lc-rs`/`aws-lc-sys` (WAF, zhend) and `anthropic-sdk-go` (ADR-104), already in use since S-QUANTUM-ARMOR / S-TWO-BRAINS | Phase 16 Step 396 (record only) |

---

## PHASE 0: PREFLIGHT (Steps 1-28)

**Goal**: Re-verify every SITUATION claim live, record the test baseline, settle ADR-103's four questions as [DECIDE], set up worktrees. (All M-ids: nothing closes here.)
**Prerequisite**: S-CLEAN-SLATE shipped; on `develop` at or after `a7fe3acd`.
**Time**: 45 min
**Agent**: Coordinator

### Environment and baseline

- [x] **Step 1** [ENV][B] ~1m: Confirm branch, base and clean tree
  ```bash
  cd /home/govan/tmp/unheaded && git rev-parse --abbrev-ref HEAD && git merge-base --is-ancestor a7fe3acd HEAD && echo "base ok" && git status --short | wc -l
  ```
- [x] **Step 2** [V] ~1m: Branch is `develop` (or a sprint branch off it), `base ok` printed, `0` untracked/modified files (the two untracked `host-agent` binaries from July may show; they are not ours, ignore exactly those two).
  - If fail → STOP; finish or stash whatever is in flight first.
- [x] **Step 3** [ENV][B] ~2m: Toolchain check
  ```bash
  go version && cargo --version && docker --version && python3 --version && which reprepro gpg cosign openssl haproxy 2>/dev/null; echo "(missing above = install in the phase that needs it)"
  ```
- [x] **Step 4** [BUILD][B] ~4m: Compile everything
  ```bash
  cd /home/govan/tmp/unheaded && go build ./... && echo BUILD_OK
  ```
- [x] **Step 5** [V] ~1m: `BUILD_OK`. If fail → S-CLEAN-SLATE is not actually done; STOP.
- [x] **Step 6** [ENV][B] ~1m: GPG probe (decides `--no-gpg-sign` for every [C] in this plan)
  ```bash
  cd /home/govan/tmp/unheaded && t=$(git rev-parse HEAD^{tree}); timeout 20 git commit-tree -S -m probe "$t" >/dev/null 2>&1 && echo "SIGN=yes" || echo "SIGN=no (use --no-gpg-sign)"
  ```
- [x] **Step 7** [DECIDE][DESIGN] ~2m: **ADR-103 Q1, issuer placement.** RECOMMENDATION: a separate service `unheaded-auth` (new binary `cmd/unheaded-auth`, uid 16735, port 19020). Rationale: the ADR's own recommendation; the dashboard then never holds user signing keys; smallest attack surface per uid. Override ONLY if Stevie has said the dashboard should hold the issuer.
- [x] **Step 8** [DECIDE][DESIGN] ~1m: **ADR-103 Q2, lab IdP.** RECOMMENDATION: the built-in issuer is the default; any OIDC provider plugs in via JWKS discovery URL (`AUTH_OIDC_ISSUER_URL`). Rationale: ADR recommendation; Authentik/Keycloak are new third-party services (Barrister + nologin user + images) and nothing today needs SSO/MFA. Override ONLY if an IdP is already running in the lab.
- [x] **Step 9** [DECIDE][DESIGN] ~1m: **ADR-103 Q3, token signature.** RECOMMENDATION: Ed25519 now; Phase 1 includes a measured ML-DSA-65 switch point (header bytes + verify ns) recorded in the ADR so the later flip is one `alg` value. Rationale: ADR recommendation; 3.3 KB signature per request header is the cost to measure before paying. Override ONLY if Stevie wants ML-DSA from day one.
- [x] **Step 10** [DECIDE][DESIGN] ~1m: **ADR-103 Q4, API-key store.** RECOMMENDATION: the Well (`unheaded_app.api_keys`, migration 035) where the Well runs; a 0600 file under `/etc/unheaded/auth/` elsewhere, same hashed format. Rationale: ADR recommendation; one code path, two stores behind one interface. Override ONLY if the Well is not to hold any auth data.
- [x] **Step 11** [DECIDE][DESIGN] ~1m: **Live release runner (master decision 4).** RECOMMENDATION: GitHub Actions. Rationale: `docs/LIVE-PATHS.md` lists no Jenkins; every gate already runs in GHA; the Jenkinsfile keeps working as the offline fallback. Override ONLY if Stevie wants Jenkins live.
- [x] **Step 12** [TEST][B] ~12m: KNOWN FAILURES BASELINE
  ```bash
  cd /home/govan/tmp/unheaded && mkdir -p docs/battle-plans/progress && go test ./... 2>&1 | grep -E '^(FAIL|ok|---)' | grep -v '^ok' | tee docs/battle-plans/progress/S-IRON-GATE-baseline.txt | tail -20; echo "FAIL lines: $(grep -c '^FAIL' docs/battle-plans/progress/S-IRON-GATE-baseline.txt)"
  ```
- [x] **Step 13** [V] ~1m: Baseline recorded. S-CLEAN-SLATE left `go test ./...` at 0 FAIL; if the count is not 0, list the failing packages in the progress file and treat them as pre-existing (DoD-STD 3 is "zero NEW").

### Hypothesis verification

- [x] **Step 14** [PREFLIGHT][B] ~1m: H1 — pkg/auth is HS256-only, no EdDSA, no trust.d
  ```bash
  cd /home/govan/tmp/unheaded && grep -c 'HS256' pkg/auth/jwt.go; grep -rlE 'ed25519|EdDSA|trust\.d|jwks' pkg/auth/*.go | grep -v _test | wc -l
  ```
- [x] **Step 15** [V] ~1m: First number ≥ 2, second number `0`. If the second is not 0 → someone started Phase 1 already; read those files before continuing.
- [x] **Step 16** [PREFLIGHT][B] ~1m: H2 — the service-auth gate fails with 8 open services
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/compliance/check-service-auth.sh; echo "exit=$?"
  ```
- [x] **Step 17** [V] ~1m: `exit=1` and the list is exactly cuirass, dashboard-backend, kanban-app, micromanager, monad, sophia, timeguru, wotan. A different list changes Phase 6's order; update the phase header, not the code.
- [x] **Step 18** [PREFLIGHT][B] ~1m: H3 — no TLS flags in compose or the Wotan unit; client dials insecure
  ```bash
  cd /home/govan/tmp/unheaded && grep -cE 'UNHEADED_TLS_ENABLED: *"true"|enable-tls' docker-compose.yml deploy/systemd/unheaded-wotan.service; grep -n 'insecure.NewCredentials' pkg/wotan-client/*.go | grep -v _test
  ```
- [x] **Step 19** [V] ~1m: Both counts `0`; two insecure sites (`grpc_client.go`, `topic_client.go`).
- [x] **Step 20** [PREFLIGHT][B] ~1m: H4 — release.yml placeholders and the deb name bug
  ```bash
  cd /home/govan/tmp/unheaded && grep -nE 'TODO: Integrate' .github/workflows/release.yml; grep -n 'PKG_NAME="unheaded-\${svc}"' scripts/build-all-debs.sh; grep -n '^Package:' debian/control
  ```
- [x] **Step 21** [V] ~1m: Two `TODO: Integrate` lines; `PKG_NAME` line present; `Package: unheaded-daemon`.
- [x] **Step 22** [PREFLIGHT][B] ~1m: H5 — registries have the slots this plan uses
  ```bash
  cd /home/govan/tmp/unheaded && grep -nE '= 1673[5-9]' pkg/uids/uids.go; grep -nE '= 1902[0-9]' pkg/ports/ports.go; echo "(empty above = 16735 and 19020 free)"
  ```
- [x] **Step 23** [V] ~1m: Both greps empty. If 16735 or 19020 is taken → use the next free value and replace it in every step of this plan (grep this file for `16735` / `19020`).
- [x] **Step 24** [PREFLIGHT][B] ~1m: H6 — image-tag baseline still has the sophia-eye rows; FND-017/018/019 are open
  ```bash
  cd /home/govan/tmp/unheaded && grep -c 'deploy/sophia-eye' docs/policy/image-tag-baseline.txt; grep -nE 'id: FND-01[789]' compliance/findings/register.yaml | wc -l
  ```
- [x] **Step 25** [V] ~1m: `4` and `3`.

### Worktrees for parallel agents

- [ ] **Step 26** [ENV][B] ~2m: Create worktrees for the parallel pairs in Appendix B
  ```bash
  cd /home/govan/tmp/unheaded && for w in auth-core apikeys issuer keygen release; do git worktree add -q ../unheaded-wt-$w -b wt/iron-gate-$w develop 2>/dev/null || echo "exists: $w"; done; git worktree list | head -8
  ```
- [ ] **Step 27** [V] ~1m: Five worktrees listed. Each agent works only in its own; the coordinator merges with `git merge --no-ff wt/iron-gate-<name>` at the phase exit gates.
- [ ] **Step 28** [V]: **PHASE 0 EXIT GATE** — Steps 5, 13, 15, 17, 19, 21, 23, 25, 27 all passed; the five [DECIDE]s are recorded in the progress file (`docs/battle-plans/progress/S-IRON-GATE-progress.md`, create it with the baseline and the decisions).
  - If pass → Phase 1 (and Phase 3 in parallel).
  - If fail → DO NOT PROCEED. Fix or re-scope within Phase 0.

---

## PHASE 1: pkg/auth, ASYMMETRIC CORE (Steps 29-66)

**Goal**: Ed25519 (EdDSA) JWT sign/verify, `kid`, `aud`, trust-directory loading, caller allowlist, JWKS client; HS256 kept for tests only. (M-050 part 1; ADR-103 §7 step 1)
**Prerequisite**: Phase 0 exit gate.
**Time**: 4 h
**Agent**: Agent (worktree `auth-core`) [P:3]

### Key material and trust directory

- [x] **Step 29** [R] ~3m: Read what exists so the new code extends it, not forks it
  ```bash
  cd /home/govan/tmp/unheaded && sed -n '1,60p' pkg/auth/jwt.go && sed -n '160,260p' pkg/auth/jwt.go && sed -n '1,110p' pkg/auth/service_token.go
  ```
- [x] **Step 30** [W][CODE] ~10m: Write the failing tests first: `pkg/auth/keys_test.go`
  - `TestGenerateKeyPair_WritesSeedAndPub`: `GenerateKeyPair(dir, "cuirass")` writes `cuirass.key` (0400, 32-byte seed, PEM `UNHEADED ED25519 SEED`) and `cuirass.pub` (0444, PEM `PUBLIC KEY`), returns `kid` = first 16 hex of SHA-256(pub).
  - `TestLoadPrivateKey_RefusesWorldReadable`: a 0644 key file returns `ErrKeyPermissions`.
  - `TestTrustDir_LoadsOnlyPub`: a dir with `a.pub`, `b.pub`, `junk.txt`, `c.key` yields 2 keys keyed by kid; `.key` in trust.d is an error (private key in a public dir).
  - `TestTrustDir_Reload_RemovedKeyRevokes`: delete `b.pub`, `Reload()`, `Lookup(kidB)` fails.
- [x] **Step 31** [TEST][B] ~1m: Red
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/auth/ -run 'TestGenerateKeyPair|TestLoadPrivateKey|TestTrustDir' -count=1 2>&1 | tail -5
  ```
- [x] **Step 32** [V] ~1m: Compile error or FAIL (red). If PASS → the tests are not testing anything; fix them.
- [x] **Step 33** [W][CODE] ~25m: `pkg/auth/keys.go`: `GenerateKeyPair(dir, name string) (kid string, err error)` using `crypto/ed25519` + `crypto/rand`; `LoadPrivateKey(path) (ed25519.PrivateKey, kid string, error)` refusing mode bits `0077` (`ErrKeyPermissions`); `type TrustDir struct` with `NewTrustDir(path)`, `Reload()`, `Lookup(kid) (ed25519.PublicKey, bool)`, `Kids() []string`; `kidOf(pub)` = hex(SHA-256(pub))[:16]. No new module.
- [x] **Step 34** [TEST][B] ~1m: Green
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/auth/ -run 'TestGenerateKeyPair|TestLoadPrivateKey|TestTrustDir' -race -count=1 2>&1 | tail -3
  ```
- [x] **Step 35** [V] ~1m: `ok`. If fail → Step 35a [D]: `go vet ./pkg/auth/` and read the first failure; max 2 attempts.
- [x] **Step 36** [C]: **COMMIT CHECKPOINT**
  ```bash
  cd /home/govan/tmp/unheaded && git add pkg/auth/keys.go pkg/auth/keys_test.go && git commit --no-gpg-sign -m "[PLAN S-IRON-GATE] Steps 29-35: Ed25519 key files and trust directory in pkg/auth

Phase 1: pkg/auth asymmetric core
Steps completed: 29, 30, 31, 32, 33, 34, 35
Verification gates passed: red (32), green -race (35)
Next: Step 37 (EdDSA JWS)

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01TzL3TuPhKAVVHnD9jzuabE"
  ```

### EdDSA JWS

- [x] **Step 37** [W][CODE] ~12m: Tests `pkg/auth/jwt_eddsa_test.go`
  - `TestSignEdDSA_HeaderHasKidAndAlg`: header `{"alg":"EdDSA","typ":"JWT","kid":"<kid>"}`.
  - `TestVerifyEdDSA_RoundTrip`: sign with key A, verify with trust dir containing A.pub → claims.
  - `TestVerifyEdDSA_UnknownKid`: kid not in trust dir → `ErrUnknownKey`.
  - `TestVerifyEdDSA_AlgConfusion`: a token with `alg: HS256` presented to the EdDSA verifier is refused even if an HS256 key is configured (`ErrAlgMismatch`); and `alg: none` refused.
  - `TestVerifyEdDSA_AudienceAndExpiry`: wrong `aud` refused; `exp` in the past refused; missing `exp` refused (pkg/auth README rule).
  - `TestVerify_HS256OnlyUnderTestBuildTag`: HS256 verification requires `AllowHS256ForTests()` to have been called; production default refuses (`ErrAlgMismatch`).
- [x] **Step 38** [TEST][B] ~1m: Red
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/auth/ -run 'EdDSA|HS256Only' -count=1 2>&1 | tail -5
  ```
- [x] **Step 39** [V] ~1m: Red.
- [x] **Step 40** [W][CODE] ~30m: `pkg/auth/jwt_eddsa.go`: `SignEdDSA(priv ed25519.PrivateKey, kid string, claims *JWTClaims) (string, error)`; `type EdDSAVerifier struct{ trust *TrustDir; issuer, audience string; skew time.Duration }` implementing `Authenticator` (`Authenticate(ctx, r)`), parsing header, requiring `alg == "EdDSA"` and a known `kid`, constant-time where it matters, reusing `JWTClaims.Validate` (`jwt.go:56`). In `jwt.go`, gate HS256 verification behind a package-level `allowHS256` set only by `AllowHS256ForTests()`; `parseAndVerify` returns `ErrAlgMismatch` otherwise. Keep `SignToken` (HS256) exported for tests, document it as test-only.
- [x] **Step 41** [TEST][B] ~2m: Green, whole package with race
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/auth/... -race -count=1 2>&1 | tail -3
  ```
- [x] **Step 42** [V] ~1m: `ok`. Existing HS256 tests must now call `AllowHS256ForTests()` in a `TestMain` or per test; if they fail for that reason, that is the intended change: fix the tests, not the gate.
- [x] **Step 43** [C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 37-42: EdDSA JWS sign/verify; HS256 verification test-only` (same trailer format as Step 36).

### Service tokens, caller allowlist, JWKS

- [x] **Step 44** [W][CODE] ~10m: Tests `pkg/auth/service_token_eddsa_test.go`
  - `TestServiceTokenMinter_PerTargetCache`: `Mint("wotan")` twice within 4 min returns the same token; after the clock passes `exp - 30s` a new one.
  - `TestServiceTokenMinter_Claims`: `sub`=caller name, `aud`=target, `iss`="unheaded", `exp-iat`=5m, `kid` set.
  - `TestCallerAllowlist`: verifier with `AllowCallers: ["cuirass","unheaded-cli"]` accepts `sub=cuirass`, refuses `sub=sword` with `ErrCallerNotAllowed` (403, not 401), refuses empty allowlist = deny-all.
- [x] **Step 45** [TEST][B] ~1m: Red
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/auth/ -run 'ServiceTokenMinter|CallerAllowlist' -count=1 2>&1 | tail -4
  ```
- [x] **Step 46** [V] ~1m: Red.
- [x] **Step 47** [W][CODE] ~25m: `pkg/auth/service_token_eddsa.go`: `type ServiceTokenMinter struct` (`NewServiceTokenMinter(name string, priv ed25519.PrivateKey, kid string, clock func() time.Time)`), `Mint(target string) (string, error)` with a per-target cache (mutex; refresh at `exp-30s`); `AuthorizationHeader(target)`. Extend `EdDSAVerifier` with `AllowCallers []string` and the 403 path: add `ErrCallerNotAllowed` and make `Middleware` (`auth.go:165`) map it to 403.
- [x] **Step 48** [TEST][B] ~2m: Green
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/auth/... -race -count=1 2>&1 | tail -3
  ```
- [x] **Step 49** [V] ~1m: `ok`.
- [x] **Step 50** [W][CODE] ~10m: Tests `pkg/auth/jwks_test.go`: an `httptest.Server` serving `/.well-known/openid-configuration` (with `jwks_uri`) and a JWKS with one `OKP`/`Ed25519` key; `JWKSVerifier` verifies a token signed by that key; cache TTL honoured (second verify makes no HTTP call); unknown `kid` triggers one refetch then `ErrUnknownKey`; discovery over plain `http://` is refused unless host is `127.0.0.1`/`localhost` (test uses loopback).
- [x] **Step 51** [TEST][B] ~1m: Red
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/auth/ -run 'JWKS' -count=1 2>&1 | tail -4
  ```
- [x] **Step 52** [V] ~1m: Red.
- [x] **Step 53** [W][CODE] ~30m: `pkg/auth/jwks.go`: `NewJWKSVerifier(issuerURL string, audience string, httpClient *http.Client, ttl time.Duration)`; fetch discovery → `jwks_uri` → keys (`kty=OKP, crv=Ed25519, x=base64url`), cache with TTL, single refetch on unknown kid, size cap 64 KiB on both documents, 5 s timeout; implements `Authenticator`; roles from the `roles` claim into `Identity.Roles`.
- [x] **Step 54** [TEST][B] ~2m: Green
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/auth/... -race -count=1 2>&1 | tail -3
  ```
- [x] **Step 55** [V] ~1m: `ok`.
- [x] **Step 56** [C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 44-55: service token minter, caller allowlist (403), JWKS verifier`.

### Config, fuzz, measurement

- [x] **Step 57** [W][CODE] ~15m: Extend `LoadServiceAuthConfig`/`SetupMiddleware` (`pkg/auth/setup.go`) with the file-based mode: `AUTH_MODE` (`eddsa` default when `AUTH_ENABLED=true`, `hs256-test` only when `AllowHS256ForTests` is set), `AUTH_KEY_FILE` (default `/etc/unheaded/keys/<service>.key`), `AUTH_TRUST_DIR` (default `/etc/unheaded/trust.d`), `AUTH_ALLOW_CALLERS` (comma list), `AUTH_OIDC_ISSUER_URL` (optional JWKS for humans), keep `AUTH_API_KEY_FILE`. Build a `MultiAuthenticator` of EdDSA(service) + JWKS(human, if configured) + APIKey(if configured). Refuse to start when `AUTH_ENABLED=true` and `AUTH_JWT_KEY` is set (`ErrSharedKeyForbidden`), with a log line naming ADR-103. Tests: `setup_eddsa_test.go` for each branch, including the refusal.
- [x] **Step 58** [TEST][B] ~2m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/auth/... -race -count=1 2>&1 | tail -3
  ```
- [x] **Step 59** [V] ~1m: `ok`.
- [x] **Step 60** [W][CODE] ~10m: Fuzz targets `pkg/auth/fuzz_test.go`: `FuzzEdDSAParse` (random token bytes never panic, never verify), `FuzzJWKSDocument` (random JSON never panics, oversize refused), `FuzzTrustDirFilenames`.
- [x] **Step 61** [TEST][B] ~4m
  ```bash
  cd /home/govan/tmp/unheaded && for f in FuzzEdDSAParse FuzzJWKSDocument FuzzTrustDirFilenames; do go test ./pkg/auth/ -run '^$' -fuzz="^$f\$" -fuzztime=60s 2>&1 | tail -1; done
  ```
- [x] **Step 62** [V] ~1m: Three lines ending `ok` or `PASS`; no `testdata/fuzz/.../` crasher written (`git status --short pkg/auth | grep -c testdata` is 0). If a crasher appears → fix the parser, keep the crasher as a regression test, commit.
- [x] **Step 63** [TEST][B] ~5m: Measure the ML-DSA-65 switch point (ADR-103 Q3 evidence)
  ```bash
  cd /home/govan/tmp/unheaded && cat > /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/sigbench_test.go <<'EOF'
  package auth_test
  import ("crypto/ed25519";"crypto/rand";"testing";"github.com/cloudflare/circl/sign/mldsa/mldsa65")
  func BenchmarkEd25519Verify(b *testing.B){pub,priv,_:=ed25519.GenerateKey(rand.Reader);m:=[]byte("hdr.payload");s:=ed25519.Sign(priv,m);b.ReportMetric(float64(len(s)),"sig_bytes");b.ResetTimer();for i:=0;i<b.N;i++{ed25519.Verify(pub,m,s)}}
  func BenchmarkMLDSA65Verify(b *testing.B){pub,priv,_:=mldsa65.GenerateKey(rand.Reader);m:=[]byte("hdr.payload");s,_:=priv.Sign(rand.Reader,m,nil);b.ReportMetric(float64(len(s)),"sig_bytes");b.ResetTimer();for i:=0;i<b.N;i++{mldsa65.Verify(pub,m,nil,s)}}
  EOF
  cp /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/sigbench_test.go pkg/auth/zz_sigbench_test.go && go test ./pkg/auth/ -run '^$' -bench 'Verify' -benchtime=2000x 2>&1 | grep -E 'Benchmark' ; rm pkg/auth/zz_sigbench_test.go
  ```
- [x] **Step 64** [W][DOC-UPDATE] ~5m: Append to `docs/adr/ADR-103-service-auth-token-distribution.md` under a new `## Measurements (S-IRON-GATE Step 63)` heading: both `ns/op` and `sig_bytes` numbers, the resulting per-request header size (base64url of sig + header + claims), and the switch rule: "flip `alg` to ML-DSA-65 when a verifier's p99 under load is dominated by something other than signature checks, or when policy requires PQ signatures; the code path is `SignEdDSA`/`EdDSAVerifier` with `alg` and the key type parametrised". Do not change the Status line yet.
- [x] **Step 65** [C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 57-64: file-based auth config, fuzz targets, ML-DSA switch-point measurement`.
- [x] **Step 66** [V]: **PHASE 1 EXIT GATE**
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/auth/... -race -count=1 2>&1 | tail -1 && go vet ./pkg/auth/... && grep -c 'AllowHS256ForTests' pkg/auth/*_test.go | awk -F: '{s+=$2} END {print "hs256 test opt-ins:", s}' && git status --short | wc -l
  ```
  - Pass: `ok`, vet silent, opt-ins ≥ 1, tree clean (`0`). → Phase 2.
  - Fail → DO NOT PROCEED.

---

## PHASE 2: unheaded-keygen AND KEY PACKAGING (Steps 67-96)

**Goal**: One command creates a service's key pair with the right owner and mode; compose, sysusers, systemd and deb postinst carry it; `unheaded-auth` gets its uid and port. (M-050 part 2; ADR-103 §7 step 2)
**Prerequisite**: Phase 1 exit gate.
**Time**: 3 h
**Agent**: Agent (worktree `keygen`) [P:4]

### Registries

- [x] **Step 67** [W][CODE] ~5m: `pkg/uids/uids.go`: add `Auth = 16735 // cmd/unheaded-auth, the built-in OIDC issuer (ADR-103)` to the Kingdom tier block after `Anomaly`. `pkg/ports/ports.go`: add `Auth = 19020 // unheaded-auth issuer (ADR-103): login, JWKS, discovery` after `TelemetryAgg`.
- [x] **Step 68** [TEST][B] ~1m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/uids/... ./pkg/ports/... -count=1 2>&1 | tail -3
  ```
- [x] **Step 69** [V] ~1m: both `ok` (uniqueness and range tests).
- [x] **Step 70** [W] ~3m: `deploy/sysusers.d/unheaded.conf`: add
  ```
  u     unheaded-auth          16735 "Unheaded auth issuer (ADR-103)"       /    /usr/sbin/nologin
  ```
- [x] **Step 71** [B] ~1m
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/check-systemd-units.sh 2>&1 | tail -2
  ```
- [x] **Step 72** [V] ~1m: `[PASS]` (the gate reads sysusers; no unit references `unheaded-auth` yet).

### The keygen tool

- [x] **Step 73** [W][CODE] ~10m: Tests `cmd/unheaded-keygen/main_test.go`: `--dir <tmp> --trust-dir <tmp2> cuirass` creates `cuirass.key` 0400 and `cuirass.pub` 0444 and copies the pub into trust-dir; running twice without `--rotate` exits 2 ("key exists"); `--rotate` writes `cuirass.key` with a new kid and keeps the old `.pub` in trust-dir until `--retire <oldkid>`; `--owner` is refused when not root (exit 2 with a clear message) so tests never chown.
- [x] **Step 74** [TEST][B] ~1m: Red
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/unheaded-keygen/ -count=1 2>&1 | tail -3
  ```
- [x] **Step 75** [V] ~1m: Red (package does not exist yet).
- [x] **Step 76** [W][CODE] ~25m: `cmd/unheaded-keygen/main.go` (flags: `-dir` default `/etc/unheaded/keys`, `-trust-dir` default `/etc/unheaded/trust.d`, `-owner` (user name; chown only when euid 0), `-rotate`, `-retire <kid>`, `-print-kid`); uses `auth.GenerateKeyPair`; creates directories 0750 (keys) and 0755 (trust.d); never prints the private key; exit codes 0/1/2 documented in `--help`.
- [x] **Step 77** [TEST][B] ~1m: Green
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/unheaded-keygen/ -race -count=1 2>&1 | tail -2
  ```
- [x] **Step 78** [V] ~1m: `ok`.
- [x] **Step 79** [W] ~5m: Add `unheaded-keygen` to `scripts/build-binaries.sh`'s known binaries list (read the file's list format first: `grep -n 'unheaded-daemon' scripts/build-binaries.sh`) and to `Makefile` `build-services` if it enumerates binaries.
- [x] **Step 80** [B] ~3m
  ```bash
  cd /home/govan/tmp/unheaded && ./scripts/build-binaries.sh --out /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/bin unheaded-keygen && ls -la /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/bin/unheaded-keygen
  ```
- [x] **Step 81** [V] ~1m: binary present.
- [x] **Step 82** [C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 67-81: uid 16735 / port 19020 for unheaded-auth; unheaded-keygen`.

### Compose, systemd and deb carry the keys

- [x] **Step 83** [W] ~15m: `scripts/gen-dev-keys.sh` (new): for every first-party service name in `docker-compose.yml` (`python3` + yaml, services with `build:`) plus `unheaded-cli` and `unheaded-auth`, run the built keygen into `configs/keys/dev/` (gitignored) and `configs/trust.d/dev/`; print the kid table. Add `configs/keys/` to `.gitignore` (whole dir; private material must never be tracked) and `configs/trust.d/dev/` too (per-deployment).
- [x] **Step 84** [B] ~2m
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/gen-dev-keys.sh && ls configs/keys/dev | wc -l && ls configs/trust.d/dev | wc -l && git status --short configs | wc -l
  ```
- [x] **Step 85** [V] ~1m: 14 keys, 14 pubs, `0` tracked changes under configs (gitignore works).
- [x] **Step 86** [W] ~20m: `docker-compose.yml`: add a top-level `secrets:` block with one `svc_key_<name>` per first-party service (`file: ./configs/keys/dev/<name>.key`); for each first-party service add `secrets: [svc_key_<name>]`, the read-only bind `./configs/trust.d/dev:/etc/unheaded/trust.d:ro`, and environment `AUTH_KEY_FILE: /run/secrets/svc_key_<name>`, `AUTH_TRUST_DIR: /etc/unheaded/trust.d`. Do NOT set `AUTH_ENABLED` yet (Phases 5-7 flip it per service). Replace the commented block at `:106-117` with a two-line pointer to this plan and ADR-103.
- [x] **Step 87** [B] ~1m
  ```bash
  cd /home/govan/tmp/unheaded && docker compose -f docker-compose.yml config --quiet && echo COMPOSE_OK && bash scripts/check-compose-hardening.sh 2>&1 | tail -2 && bash scripts/check-compose-bind-nesting.sh 2>&1 | tail -1
  ```
- [x] **Step 88** [V] ~1m: `COMPOSE_OK` and both gates `[PASS]`. If a gate fails on the new bind → it wants `:ro` and a non-nested path; both are already so; read its message.
- [x] **Step 89** [W] ~10m: `deploy/systemd/README.md` section "Keys (ADR-103)": the install recipe (`unheaded-keygen -owner unheaded-<svc> <svc>` as root once per service, trust.d is world-readable, rotation and retire). Add to each first-party unit in `deploy/systemd/` an `Environment=AUTH_KEY_FILE=/etc/unheaded/keys/<svc>.key` and `Environment=AUTH_TRUST_DIR=/etc/unheaded/trust.d` line, and `ReadOnlyPaths=/etc/unheaded/trust.d` (ProtectSystem=strict already blocks writes; this documents intent). Units: wotan, timeguru, architect, captain, micromanager, monad, sophia, dashboard, kanban, daemon, sword, zhen-agentd, anomaly.
- [x] **Step 90** [B] ~1m
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/check-systemd-units.sh 2>&1 | tail -2 && bash scripts/check-unit-binaries.sh 2>&1 | tail -1
  ```
- [x] **Step 91** [V] ~1m: both `[PASS]`.
- [x] **Step 92** [W][CODE] ~15m: `scripts/lib/deb-maintscripts.sh` `deb_postinst`: after `systemd-sysusers`, if `/etc/unheaded/keys/<svc>.key` is absent, run `/usr/local/bin/unheaded-keygen -owner unheaded-<svc> <svc>` (keygen ships in every deb; `scripts/build-all-debs.sh` copies it alongside the service binary). Rewrite `debian/unheaded-wotan.postinst` to call the shared helper instead of creating the shared `unheaded` user (it contradicts `deploy/sysusers.d`). Test: `tests/deb/postinst_test.sh` (new, bash) runs the postinst in a fakeroot-free dry mode (`DEB_DRY_RUN=1` prints the commands) and asserts the keygen line and the absence of `useradd ... unheaded$`.
- [x] **Step 93** [B] ~2m
  ```bash
  cd /home/govan/tmp/unheaded && bash tests/deb/postinst_test.sh 2>&1 | tail -3 && bash -n debian/unheaded-wotan.postinst && shellcheck -S warning scripts/lib/deb-maintscripts.sh debian/unheaded-wotan.postinst 2>&1 | tail -3
  ```
- [x] **Step 94** [V] ~1m: test PASS, `bash -n` silent, shellcheck no warnings.
- [x] **Step 95** [C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 83-94: dev keys script, compose secrets + trust.d, unit env, deb postinst keygen`.
- [x] **Step 96** [V]: **PHASE 2 EXIT GATE** — Steps 69, 72, 78, 81, 85, 88, 91, 94 passed; `go test ./cmd/unheaded-keygen/... ./pkg/uids/... ./pkg/ports/... -count=1` ok; tree clean.
  - Pass → Phase 5 may start (Phases 3 and 4 may already be running).

---

## PHASE 3: API KEYS FOR AUTOMATION (Steps 97-124)

**Goal**: `uhk_`-prefixed API keys, hashed at rest in the Well (or a file), with a gitleaks rule, created and listed from `unheaded-cli`. (M-050 part 3; ADR-103 §5)
**Prerequisite**: Phase 0 exit gate (parallel with Phase 1).
**Time**: 3 h
**Agent**: Agent (worktree `apikeys`) [P:1]

- [x] **Step 97** [R] ~3m
  ```bash
  cd /home/govan/tmp/unheaded && sed -n '1,120p' pkg/auth/apikey.go && grep -n 'func\|type' cmd/unheaded-cli/*.go | head -30 && sed -n '1,30p' db/migrations/034_app_kanban_genesis.sql
  ```
- [x] **Step 98** [W][CODE] ~10m: Tests `pkg/auth/apikey_store_test.go`: `NewAPIKey()` returns `uhk_` + 43 base64url chars (32 bytes), shown once; `Store.Create(owner, role, expiry)` stores only `sha256(key)`; `Store.Authenticate(key)` returns identity with role and refuses expired/revoked; `FileStore` (JSON lines, 0600) and an in-memory store share the test suite; a `fakeWellStore` implements the same interface so the SQL store can be tested against a throwaway Postgres later.
- [x] **Step 99** [TEST][B] ~1m: Red
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/auth/ -run 'APIKeyStore|NewAPIKey' -count=1 2>&1 | tail -3
  ```
- [x] **Step 100** [V] ~1m: Red.
- [x] **Step 101** [W][CODE] ~25m: `pkg/auth/apikey_store.go`: `NewAPIKey()`, `type APIKeyStore interface { Create; Authenticate; Revoke; List }`, `FileStore`, `MemStore`; wire `APIKeyAuthenticator` (`apikey.go:42`) to accept a store (keep the existing env/file list path for backward compatibility one release, logged as deprecated).
- [x] **Step 102** [TEST][B] ~2m: Green
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/auth/... -race -count=1 2>&1 | tail -2
  ```
- [x] **Step 103** [V] ~1m: `ok`.
- [x] **Step 104** [C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 97-103: uhk_ API keys, hashed stores`.
- [x] **Step 105** [W] ~15m: Migration `db/migrations/035_app_api_keys.sql` (header style of 033): `unheaded_app.api_keys (id uuid pk, key_hash char(64) unique, prefix char(12), owner text, role text check (role in ('admin','operator','viewer','service')), created_at, expires_at not null, revoked_at, last_used_at)`; append-only on `key_hash` (no UPDATE of hash), grants to `app_auth` role only (create the role in this migration if the ledger pattern creates roles per service; read `db/migrations/007_grants.sql` first). Add the file to `db/migrations/MANIFEST` with the pattern the other `app` migrations use.
- [x] **Step 106** [B] ~2m
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/check-well-migrations.sh 2>&1 | tail -3 && bash scripts/check-well-lint.sh 2>&1 | tail -2
  ```
- [x] **Step 107** [V] ~1m: both `[PASS]`.
- [x] **Step 108** [W][CODE] ~20m: `pkg/auth/apikey_well.go`: `WellStore` over `*sql.DB` (uses `pkg/database` config `AppKanbanConfig`-style helper for `unheaded_app`; read `pkg/database/config.go:80-110` and add `AppAuthConfig` with `WELL_DB` default `unheaded_app`). Test `apikey_well_test.go` behind `//go:build well` runs against the throwaway Postgres the CI job `well-schema` provides (see `.github/workflows/security.yml:512`); the shared suite from Step 98 runs against it.
- [x] **Step 109** [TEST][B] ~4m: Throwaway Postgres run (local docker)
  ```bash
  cd /home/govan/tmp/unheaded && grep -nE 'docker run|postgres:' .github/workflows/security.yml | sed -n '1,4p'; echo "--- replicate locally:"; cid=$(docker run -d --rm -e POSTGRES_PASSWORD=t -p 55432:5432 postgres:16-alpine) && sleep 6 && WELL_TEST_DSN='postgres://postgres:t@127.0.0.1:55432/postgres?sslmode=disable' go test -tags well ./pkg/auth/ -run 'WellStore' -count=1 2>&1 | tail -3; docker stop "$cid" >/dev/null
  ```
- [x] **Step 110** [V] ~1m: `ok`. If the CI job uses a different image/DSN env, use that one (the step's first grep shows it) and fix the test's env name to match.
- [x] **Step 111** [W][CODE] ~15m: `cmd/unheaded-cli`: subcommands `apikey create --owner <o> --role <r> --expires 90d`, `apikey list`, `apikey revoke <prefix>`; store chosen by `AUTH_APIKEY_STORE` (`well` default when `WELL_DB` is set, else `file:/etc/unheaded/auth/apikeys.jsonl`); prints the key exactly once with a leading line "store this now; it is not shown again". Tests with the `MemStore`.
- [x] **Step 112** [TEST][B] ~2m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/unheaded-cli/... -race -count=1 2>&1 | tail -2
  ```
- [x] **Step 113** [V] ~1m: `ok`.
- [x] **Step 114** [W] ~5m: gitleaks rule for the prefix: create `.gitleaks.toml` (if `scripts/check-gitleaks.sh` passes a config, extend that one; check `grep -n 'config\|toml' scripts/check-gitleaks.sh`) with
  ```toml
  [extend]
  useDefault = true
  [[rules]]
  id = "unheaded-api-key"
  description = "Unheaded uhk_ API key (ADR-103 §5)"
  regex = '''uhk_[A-Za-z0-9_-]{43}'''
  keywords = ["uhk_"]
  ```
- [x] **Step 115** [TEST][B] ~2m: The rule bites and the tree is clean
  ```bash
  cd /home/govan/tmp/unheaded && printf 'k = "uhk_%s"\n' "$(head -c 32 /dev/urandom | base64 | tr '+/' '-_' | tr -d '=' | head -c 43)" > /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/leak.txt && gitleaks detect --no-git --source /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad --config .gitleaks.toml 2>&1 | grep -c 'unheaded-api-key'; rm /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/leak.txt; bash scripts/check-gitleaks.sh 2>&1 | tail -1
  ```
- [x] **Step 116** [V] ~1m: first number ≥ 1 (rule fires on the planted key), then `[PASS]` on the tree.
- [x] **Step 117** [W][COMPLIANCE] ~3m: `scripts/check-gitleaks.sh`: pass `--config .gitleaks.toml` if it does not already; `docs/security/` finding note: none needed (no new finding), but `wiki/Security-Tooling.md` (or the page `grep -l gitleaks wiki/*.md` finds) gets one line for the rule.
- [x] **Step 118** [B] ~1m
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/check-gitleaks.sh 2>&1 | tail -1 && bash scripts/check-secrets-baseline.sh 2>&1 | tail -1
  ```
- [x] **Step 119** [V] ~1m: both `[PASS]`.
- [x] **Step 120** [C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 105-119: api_keys migration 035, WellStore, unheaded-cli apikey, gitleaks uhk_ rule`.
- [x] **Step 121** [W][DOC-UPDATE] ~5m: `wiki/The-Well.md` (or `wiki/Well-Schema.md`, whichever lists tables): add `api_keys` with its grant; `docs/adr/ADR-103`: note Q4 decided (Step 10) and the store interface.
- [x] **Step 122** [B] ~1m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/auth/... ./cmd/unheaded-cli/... -race -count=1 2>&1 | tail -2 && git status --short | wc -l
  ```
- [x] **Step 123** [V] ~1m: `ok` x2, `0`.
- [x] **Step 124** [V]: **PHASE 3 EXIT GATE** — Steps 103, 107, 110, 113, 116, 119, 123 passed. → merge `wt/iron-gate-apikeys` into the sprint branch (`git merge --no-ff`), re-run Step 122 on the merged tree.

---

## PHASE 4: THE BUILT-IN ISSUER, unheaded-auth (Steps 125-158)

**Goal**: A small OIDC-shaped issuer: password login (argon2id), EdDSA access tokens (15 min) with a `roles` claim, JWKS and discovery endpoints, its own uid/port/unit/compose entry. (M-050 part 4; ADR-103 §3; Q1/Q2 per Steps 7-8)
**Prerequisite**: Phase 1 exit gate (uses `SignEdDSA`, `TrustDir`).
**Time**: 5 h
**Agent**: Agent (worktree `issuer`) [P:2]

- [x] **Step 125** [DECIDE][DESIGN] ~2m: **WebAuthn in the first cut?** RECOMMENDATION: no. Rationale: it needs a new Go module (Barrister/ADR-004 admission) and a browser ceremony to test; password + argon2id + rate limit + lockout covers the lab; the ADR already says "optional". Override ONLY if Stevie wants WebAuthn now; then add an ADR-004 row first.
- [x] **Step 126** [W][CODE] ~15m: Tests `cmd/unheaded-auth/issuer_test.go` (httptest):
  - `GET /.well-known/openid-configuration` returns `issuer`, `jwks_uri`, `token_endpoint`, `response_types_supported: ["token"]`.
  - `GET /jwks.json` returns one `OKP/Ed25519` key with `kid` = `auth.TrustDir` kid of the issuer's key.
  - `POST /login` (JSON `{username,password}`) with the right password returns `{access_token, token_type:"Bearer", expires_in: 900}`; the token verifies with `auth.JWKSVerifier` pointed at the test server; claims `sub`, `roles`, `iss`, `aud:"unheaded"`, `exp-iat=900s`.
  - wrong password → 401 with constant-time compare; 6th failure within 10 min → 429 for that username and source IP (lockout 10 min).
  - users file `0600` JSON `{username, argon2id_hash, roles}`; a world-readable users file refuses to start.
  - `/health`, `/ready`, `/metrics` unauthenticated; everything else over plaintext refused unless `-dev-plaintext` (test uses it).
- [x] **Step 127** [TEST][B] ~1m: Red
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/unheaded-auth/ -count=1 2>&1 | tail -3
  ```
- [x] **Step 128** [V] ~1m: Red.
- [x] **Step 129** [W][CODE] ~45m: `cmd/unheaded-auth/{main.go,issuer.go,users.go,lockout.go}`: flags `-listen :19020` (`ports.Auth`), `-issuer-url` (external URL for `iss`), `-key-file /etc/unheaded/keys/unheaded-auth.key`, `-users-file /etc/unheaded/auth/users.json`, `-dev-plaintext`; argon2id via `golang.org/x/crypto/argon2` (time 3, memory 64 MiB, threads 2, salt 16); `httputil.NewServiceMetrics("unheaded-auth")` instrumented; structured zerolog like the other services; refuse start if key or users file is group/world readable; `Content-Security-Policy` and the Step 300 security headers on every response; request body cap 4 KiB; `ReadHeaderTimeout 5s`.
- [x] **Step 130** [TEST][B] ~2m: Green
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/unheaded-auth/... -race -count=1 2>&1 | tail -2
  ```
- [x] **Step 131** [V] ~1m: `ok`.
- [x] **Step 132** [C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 125-131: unheaded-auth issuer (login, JWKS, discovery, lockout)`.
- [x] **Step 133** [W][CODE] ~15m: `cmd/unheaded-auth` subcommand `user add <name> --roles viewer,operator` (prompts for the password on a tty; `--password-stdin` for scripts), `user list`, `user del`; writes the users file atomically (temp + rename, 0600). Tests for add/list/del and for refusing an empty password.
- [x] **Step 134** [TEST][B] ~1m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/unheaded-auth/... -race -count=1 2>&1 | tail -2
  ```
- [x] **Step 135** [V] ~1m: `ok`.
- [x] **Step 136** [W] ~10m: `deploy/systemd/unheaded-auth.service`: copy the sandbox block from `unheaded-wotan.service` verbatim; `User=unheaded-auth`, `Group=unheaded-auth`, `ExecStart=/usr/local/bin/unheaded-auth -listen :19020 -issuer-url https://%H:19020 -key-file /etc/unheaded/keys/unheaded-auth.key -users-file /etc/unheaded/auth/users.json`, `StateDirectory=unheaded/auth`, `ReadOnlyPaths=/etc/unheaded/trust.d`, `EnvironmentFile=-/etc/unheaded/auth.env`. `deploy/sysusers.d` already has the user (Step 70).
- [x] **Step 137** [B] ~1m
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/check-systemd-units.sh 2>&1 | tail -1 && bash scripts/check-unit-binaries.sh 2>&1 | tail -1 && systemd-analyze verify deploy/systemd/unheaded-auth.service 2>&1 | grep -vE 'Unit .* not found|^$' | head -3
  ```
- [x] **Step 138** [V] ~1m: two `[PASS]`; `systemd-analyze` prints nothing but "not found" noise for the binary path.
- [x] **Step 139** [W] ~10m: `docker-compose.yml`: service `auth` (image built from the shared Dockerfile stage like `kanban-app`; read how `kanban-app`'s `build:` is declared and copy it), `user: 16735:16735`, port `19020` on the `control` network only (no `ports:` to the host unless the dashboard needs it; dashboard-backend is the BFF and reaches it on the Docker network), secret `svc_key_unheaded-auth`, a bind for a users file `./configs/auth/dev-users.json:/etc/unheaded/auth/users.json:ro` (file is created by Step 140 and gitignored), `healthcheck` on `/health`, `kingdom.component: control-plane` label, log cap like the others.
- [x] **Step 140** [W] ~5m: `scripts/gen-dev-keys.sh`: also create `configs/auth/dev-users.json` with one `operator` user `dev` whose password is printed once to the terminal (random 20 chars), never written anywhere else; add `configs/auth/` to `.gitignore`. Add `scripts/build-binaries.sh` entry for `unheaded-auth` and `SERVICES` entry in `scripts/build-all-debs.sh` (`unheaded-auth` → `unheaded-auth.service`).
- [x] **Step 141** [B] ~2m
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/gen-dev-keys.sh | tail -3 && docker compose config --quiet && echo COMPOSE_OK && bash scripts/check-compose-hardening.sh 2>&1 | tail -1 && bash scripts/check-compose-log-caps.sh 2>&1 | tail -1 && bash scripts/check-image-tags.sh 2>&1 | tail -1
  ```
- [x] **Step 142** [V] ~1m: `COMPOSE_OK` and three `[PASS]`.
- [x] **Step 143** [C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 133-142: unheaded-auth user mgmt, unit, compose service`.
- [ ] **Step 144** [BUILD][B] ~4m: Bring up only the issuer and prove the round trip locally
  ```bash
  cd /home/govan/tmp/unheaded && docker compose up -d --build auth && sleep 5 && docker compose exec -T auth wget -qO- http://127.0.0.1:19020/.well-known/openid-configuration | head -c 300; echo; docker compose exec -T auth wget -qO- http://127.0.0.1:19020/jwks.json | grep -c '"kty":"OKP"'
  ```
- [ ] **Step 145** [V] ~1m: discovery JSON with `jwks_uri`; `1` OKP key. If `wget` is missing in the image, use `docker compose exec -T auth /usr/local/bin/unheaded-auth probe` (add a tiny `probe` subcommand in Step 129 if the image is distroless, which it is for the other services: `grep distroless docker/base/Dockerfile.base`).
- [ ] **Step 146** [TEST][B] ~2m: Login from the host side through the Docker network via a one-off container
  ```bash
  cd /home/govan/tmp/unheaded && PW=$(grep -oE 'dev password: .*' /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/gen-dev-keys.log 2>/dev/null | awk '{print $3}'); [ -n "$PW" ] || echo "re-run Step 141 teeing output to scratchpad/gen-dev-keys.log"; docker compose run --rm -T --no-deps --entrypoint /bin/sh kanban-app -c "wget -qO- --post-data='{\"username\":\"dev\",\"password\":\"$PW\"}' --header='Content-Type: application/json' http://auth:19020/login" 2>/dev/null | grep -c access_token
  ```
- [ ] **Step 147** [V] ~1m: `1`. (If the kanban image has no shell, run a `busybox:1.37.0` one-off on the compose `control` network instead: `docker run --rm --network unheaded_control busybox:1.37.0 wget ...`.)
- [ ] **Step 148** [B] ~1m: Tear down
  ```bash
  cd /home/govan/tmp/unheaded && docker compose stop auth && docker compose rm -f auth >/dev/null && echo down
  ```
- [x] **Step 149** [W][DOC-UPDATE] ~8m: `wiki/Service-Auth.md` (new, from the `wiki/Service-Wotan.md` template): purpose, port 19020, uid 16735, endpoints, users file, lockout, how a service verifies (JWKS URL), how an external OIDC provider replaces it (`AUTH_OIDC_ISSUER_URL`). Add to `wiki/_Sidebar.md` under Services and to `wiki/Port-Registry.md` + the uid table page (`grep -l 16734 wiki/*.md`).
- [x] **Step 150** [B] ~1m
  ```bash
  cd /home/govan/tmp/unheaded && grep -c 'Service-Auth' wiki/_Sidebar.md && grep -c '19020' wiki/Port-Registry.md && grep -lc '16735' wiki/*.md | head -2
  ```
- [x] **Step 151** [V] ~1m: all ≥ 1.
- [x] **Step 152** [SECURITY][W] ~10m: Fuzz `cmd/unheaded-auth`: `FuzzLoginBody` (random JSON bodies never panic, never 200), `FuzzUsersFile` (random users files never panic; malformed refused at start). Run 60 s each.
- [x] **Step 153** [TEST][B] ~3m
  ```bash
  cd /home/govan/tmp/unheaded && for f in FuzzLoginBody FuzzUsersFile; do go test ./cmd/unheaded-auth/ -run '^$' -fuzz="^$f\$" -fuzztime=60s 2>&1 | tail -1; done; git status --short cmd/unheaded-auth | grep -c testdata
  ```
- [x] **Step 154** [V] ~1m: two `ok`/`PASS`, `0` crashers.
- [x] **Step 155** [C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 144-154: issuer round trip in compose, wiki page, fuzz`.
- [x] **Step 156** [B] ~3m: Crypto inventory must learn the new key material (CBOM gate)
  ```bash
  cd /home/govan/tmp/unheaded && go run ./cmd/crypto-inventory scan --out compliance/crypto/cbom.cdx.json 2>&1 | tail -2 && bash scripts/check-crypto-inventory.sh 2>&1 | tail -1 && git diff --stat compliance/crypto/cbom.cdx.json | tail -1
  ```
- [x] **Step 157** [V] ~1m: `[PASS]`; the diff adds Ed25519 (`crypto/ed25519`) and argon2id entries. Commit as part of Step 158's gate commit.
- [x] **Step 158** [V]: **PHASE 4 EXIT GATE**
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/unheaded-auth/... ./pkg/auth/... -race -count=1 2>&1 | tail -2 && docker compose config --quiet && bash scripts/check-systemd-units.sh 2>&1 | tail -1 && git add -A compliance/crypto && git commit --no-gpg-sign -q -m "[PLAN S-IRON-GATE] Steps 156-158: CBOM regenerated for Ed25519 + argon2id; Phase 4 exit

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01TzL3TuPhKAVVHnD9jzuabE" && git status --short | wc -l
  ```
  - Pass: `ok` x2, gates pass, `0`. → merge `wt/iron-gate-issuer` and `wt/iron-gate-keygen`; Phase 5.
  - Fail → DO NOT PROCEED.

---

## PHASE 5: AUTH ON, CUIRASS FIRST (Steps 159-178)

**Goal**: The first service refuses unauthenticated writes and accepts the CLI's service token; the pattern every later service copies. (M-050 part 5; ADR-103 §7 step 3)
**Prerequisite**: Phases 1-4 merged into the sprint branch.
**Time**: 2.5 h
**Agent**: Coordinator

- [x] **Step 159** [R] ~3m: How Cuirass wires auth today and which routes write
  ```bash
  cd /home/govan/tmp/unheaded && grep -nE 'auth\.|AUTH_|SkipAuthPaths|RBAC' cmd/unheaded-daemon/*.go | grep -v _test | head -20 && grep -nE 'HandleFunc\("(POST|PUT|DELETE)|Methods\(|"/api/v1/' cmd/unheaded-daemon/*.go | grep -v _test | head -20
  ```
- [x] **Step 160** [W][CODE] ~15m: Test `cmd/unheaded-daemon/auth_e2e_test.go`: start the daemon handler with `AUTH_ENABLED=true`, `AUTH_MODE=eddsa`, a temp key, temp trust.d holding `unheaded-cli.pub`, `AUTH_ALLOW_CALLERS=unheaded-cli,sword`; `GET /api/v1/info` and `/api/v1/state` → 200 unauthenticated (read endpoints opened 2ff3be29 stay open); `POST /api/v1/reconcile` without a token → 401; with a `sword`-minted token → 200 (or the handler's normal non-auth error, asserted ≠ 401/403); with a token from a key NOT in trust.d → 401; with a trusted key whose `sub` is not allowlisted → 403; `/health`, `/ready`, `/metrics` → 200.
- [x] **Step 161** [TEST][B] ~1m: Red
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/unheaded-daemon/ -run 'AuthE2E' -count=1 2>&1 | tail -3
  ```
- [x] **Step 162** [V] ~1m: Red (today everything is 200 under Noop, or the env is ignored).
- [x] **Step 163** [W][CODE] ~20m: `cmd/unheaded-daemon`: use `auth.SetupMiddleware(auth.LoadServiceAuthConfig("cuirass"))` with the Phase 1 config (it already does, so the work is: ensure read-only GETs are in `SkipAuthPaths` exactly as 2ff3be29 opened them, and that `RBACMiddleware(DefaultServicePolicies("cuirass"))` requires role `operator` or `service` on writes). Where a `service` identity comes from an EdDSA token, give it role `service` in `Identity.Roles` (pkg/auth change if missing).
- [x] **Step 164** [TEST][B] ~2m: Green
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/unheaded-daemon/... ./pkg/auth/... -race -count=1 2>&1 | tail -3
  ```
- [x] **Step 165** [V] ~1m: `ok`.
- [x] **Step 166** [W][CODE] ~15m: `cmd/unheaded-cli`: mint a service token from its own key (`AUTH_KEY_FILE` default `~/.config/unheaded/cli.key` for a human operator, `/etc/unheaded/keys/unheaded-cli.key` on a host) for `aud=cuirass` on every Cuirass call; `--api-key` alternative sends `Authorization: Bearer uhk_...`. Test: the CLI's HTTP client sets the header (httptest server asserts).
- [x] **Step 167** [TEST][B] ~1m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/unheaded-cli/... -race -count=1 2>&1 | tail -2
  ```
- [x] **Step 168** [V] ~1m: `ok`.
- [x] **Step 169** [W] ~5m: `docker-compose.yml` `cuirass`: `AUTH_ENABLED: "true"`, `AUTH_ALLOW_CALLERS: "sword,unheaded-cli,dashboard-backend"`. `deploy/systemd/unheaded-daemon.service`: `Environment=AUTH_ENABLED=true` and the same allowlist. `docker/compose.cuirass-lxd.yml`: nothing (inherits).
- [x] **Step 170** [C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 159-169: Cuirass requires service tokens on writes; CLI mints them`.
- [ ] **Step 171** [BUILD][B] ~5m: Live in compose: Cuirass + Sword (Sword is Cuirass's only in-stack writer)
  ```bash
  cd /home/govan/tmp/unheaded && docker compose up -d --build cuirass sword && sleep 8 && docker compose logs --tail=20 cuirass sword 2>&1 | grep -iE 'auth|401|403|error' | head -10; echo "--- unauth write:"; docker run --rm --network unheaded_control busybox:1.37.0 wget -S -qO- --post-data='{}' http://cuirass:19006/api/v1/reconcile 2>&1 | grep -E 'HTTP/' | head -1
  ```
- [ ] **Step 172** [V] ~1m: Sword logs show no 401/403 against Cuirass (its token works); the busybox write shows `HTTP/1.1 401`. If Sword gets 401 → Step 172a [D]: Sword's key secret is missing or its `sub` is not in Cuirass's allowlist; `docker compose exec sword ls -la /run/secrets/` and the Cuirass env.
- [ ] **Step 173** [TEST][B] ~2m: CLI round trip through the host-published Cuirass port (if published) or via a one-off container running the CLI binary
  ```bash
  cd /home/govan/tmp/unheaded && go build -o /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/bin/unheaded-cli ./cmd/unheaded-cli && AUTH_KEY_FILE=configs/keys/dev/unheaded-cli.key /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/bin/unheaded-cli --cuirass http://127.0.0.1:19006 state 2>&1 | head -3
  ```
- [ ] **Step 174** [V] ~1m: JSON state, not 401. (Cuirass publishes 19006 to the host per `wiki/Containers.md`; if not published in this profile, run the binary inside the network as in Step 171.)
- [x] **Step 175** [B] ~1m
  ```bash
  cd /home/govan/tmp/unheaded && docker compose stop cuirass sword && bash scripts/compliance/check-service-auth.sh 2>&1 | tail -9
  ```
- [x] **Step 176** [V] ~1m: The FAIL list no longer contains `cuirass` (7 remain).
- [ ] **Step 177** [C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 171-176: Cuirass auth proven live in compose (Sword 200, anonymous 401)`.
- [ ] **Step 178** [V]: **PHASE 5 EXIT GATE** — Steps 165, 168, 172, 174, 176 passed; tree clean. → Phase 6.

---

## PHASE 6: AUTH ON, EVERY OTHER CALLER (Steps 179-222)

**Goal**: zhen-agentd, protocol-api, kanban-app, timeguru, captain, architect, micromanager, monad, sophia, sword, wiki-server, dashboard-backend (as BFF with login + session cookie), each its own commit with a 401/200 test. (M-050 part 6, M-055; ADR-103 §4, §7 step 4)
**Prerequisite**: Phase 5 exit gate.
**Time**: 7 h
**Agent**: Coordinator for dashboard-backend (BFF); Agent [P] for the per-service wave (one worktree, sequential commits)

### The per-service recipe (apply to each service in order; one commit each)

For each `<svc>` in: zhen-agentd, protocol-api, kanban-app, timeguru, captain, architect, micromanager, monad, sophia, sword, wiki-server:

- [ ] **Step 179** [W][CODE] ~10m each: `cmd/<svc>` or `services/<svc>`: an `auth_e2e_test.go` modelled on Step 160 (unauth write 401; trusted+allowlisted token 200/non-auth error; untrusted kid 401; trusted-not-allowlisted 403; health/ready/metrics open). Writers per service: zhen-agentd ← unheaded-cli, dashboard-backend; protocol-api ← unheaded-cli; kanban-app ← dashboard-backend, zhen-agentd, micromanager; timeguru ← micromanager, kanban-app; captain/architect/micromanager ← each other + dashboard-backend; monad/sophia ← unheaded-cli, dashboard-backend; sword ← unheaded-cli, dashboard-backend; wiki-server ← none (reads only; writes 403 for everyone until a user token). Human JWKS tokens (role `operator`) also pass writes where the service is browser-facing (kanban-app, sword, cuirass).
- [ ] **Step 180** [TEST][B] ~1m each: Red
  ```bash
  cd /home/govan/tmp/unheaded && go test ./<pkg-of-svc>/ -run 'AuthE2E' -count=1 2>&1 | tail -3
  ```
- [ ] **Step 181** [W][CODE] ~10m each: wire `LoadServiceAuthConfig("<svc>")` + RBAC default policies; where the service calls another service, inject `auth.ServiceTokenMinter` and set `Authorization` on outgoing requests (grep each service's HTTP client construction: `http.Client{` / `httputil.NewClient`).
- [ ] **Step 182** [TEST][B] ~2m each: Green with `-race`
  ```bash
  cd /home/govan/tmp/unheaded && go test ./<pkg-of-svc>/... -race -count=1 2>&1 | tail -2
  ```
- [ ] **Step 183** [W] ~3m each: compose `AUTH_ENABLED: "true"` + `AUTH_ALLOW_CALLERS`; systemd unit `Environment=AUTH_ENABLED=true` + allowlist.
- [ ] **Step 184** [C] each: `[PLAN S-IRON-GATE] Step 179-184 (<svc>): auth on, writers allowlisted`.
  - Steps 179-184 are repeated 11 times, once per service in the order listed; log each service's pass in the progress file as `179.<svc>` … `184.<svc>`. Time: ~30 min per service.
- [ ] **Step 185** [V] ~2m: After every service
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/compliance/check-service-auth.sh 2>&1 | tail -4
  ```
  Expected after the wave: only `dashboard-backend` and `wotan` remain in the FAIL list.

### Dashboard-backend as the browser's BFF

- [ ] **Step 186** [R] ~3m
  ```bash
  cd /home/govan/tmp/unheaded && grep -nE 'HandleFunc|Handle\(|mux\.' cmd/dashboard-backend/internal/server/server.go | head -40 && grep -nE 'SameOriginOrAllowed|Upgrade' cmd/dashboard-backend/internal/server/*.go | grep -v _test | head
  ```
- [ ] **Step 187** [W][CODE] ~15m: Tests `cmd/dashboard-backend/internal/server/session_test.go`: `POST /auth/login` (JSON username/password) proxies to the issuer (`AUTH_ISSUER_URL`), on 200 stores the access token server-side in a session store keyed by a random 32-byte id, sets cookie `unheaded_session` with `HttpOnly; Secure; SameSite=Strict; Path=/` (Secure dropped only with `-dev-plaintext`); `GET /auth/me` returns `{sub, roles}` from the session; `POST /auth/logout` deletes it; a write API without a session → 401; with an `operator` session → passes to the handler; with a `viewer` session on a write → 403; WebSocket upgrade requires the cookie after the existing Origin check; sessions expire with the token (`exp`) and the store evicts them; `/metrics`, `/health`, `/ready` open; static assets open.
- [ ] **Step 188** [TEST][B] ~1m: Red
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/dashboard-backend/... -run 'Session|Login|BFF' -count=1 2>&1 | tail -3
  ```
- [ ] **Step 189** [V] ~1m: Red.
- [ ] **Step 190** [W][CODE] ~45m: `cmd/dashboard-backend/internal/server/session.go` (in-memory session store with mutex + eviction ticker; interface so a Well-backed store can follow), `auth_bff.go` (login/me/logout handlers, cookie middleware producing an `auth.Identity` from the session's token claims, re-verified against the issuer's JWKS via `auth.JWKSVerifier`), `act` claim: outgoing service calls use the backend's own `ServiceTokenMinter` and add header `X-Unheaded-On-Behalf-Of: <sub>` signed into the service token as claim `act` (extend `Mint(target, opts...)` with `WithActor(sub)`). CSRF: state-changing BFF routes require header `X-Requested-With: unheaded` (SameSite=Strict + custom header; document why in a comment).
- [ ] **Step 191** [TEST][B] ~2m: Green
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/dashboard-backend/... ./pkg/auth/... -race -count=1 2>&1 | tail -3
  ```
- [ ] **Step 192** [V] ~1m: `ok`.
- [ ] **Step 193** [C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 186-192: dashboard-backend BFF login, session cookie, on-behalf-of claim`.
- [ ] **Step 194** [W][CODE] ~30m: `dashboard/` (vanilla JS): a login panel (username/password → `POST /auth/login` with `X-Requested-With`), the header shows `sub` and roles from `/auth/me`, logout button; every `fetch` to a write API adds `X-Requested-With: unheaded` and `credentials: 'same-origin'`; on 401 show the login panel. No token ever touches JS. Keep it in one new file `dashboard/js/auth.js` loaded by `dashboard/index.html`; `scripts/check-js-lint.sh` must pass.
- [ ] **Step 195** [B] ~1m
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/check-js-lint.sh 2>&1 | tail -2 && grep -c 'auth.js' dashboard/index.html
  ```
- [ ] **Step 196** [V] ~1m: lint `[PASS]`, `1`.
- [ ] **Step 197** [W][CODE] ~15m: RBAC `operator` on the compliance profile-switch POST (`compliance_profiles.go:16` comment says this is the gap): the handler requires role `operator` and records `sub` in the Well row it writes (`ADR-099 §5`); test asserts a `viewer` session gets 403 and the row has the actor.
- [ ] **Step 198** [TEST][B] ~2m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/dashboard-backend/... -race -count=1 2>&1 | tail -2
  ```
- [ ] **Step 199** [V] ~1m: `ok`.
- [ ] **Step 200** [W] ~5m: compose `dashboard-backend`: `AUTH_ENABLED: "true"`, `AUTH_ISSUER_URL: http://auth:19020` (TLS in Phase 8 changes this to https), `AUTH_ALLOW_CALLERS: "unheaded-cli"`, `depends_on: [auth]`; unit `unheaded-dashboard.service`: the same via `Environment=`.
- [ ] **Step 201** [C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 194-200: dashboard login UI, operator RBAC on profile switch, compose/unit`.

### Live proof of the BFF

- [ ] **Step 202** [BUILD][B] ~6m
  ```bash
  cd /home/govan/tmp/unheaded && docker compose up -d --build auth dashboard-backend kanban-app && sleep 10 && curl -s -o /dev/null -w 'health=%{http_code}\n' http://127.0.0.1:20000/health && curl -s -o /dev/null -w 'unauth_write=%{http_code}\n' -X POST http://127.0.0.1:20000/api/v1/compliance/profile -d '{}' -H 'Content-Type: application/json'
  ```
- [ ] **Step 203** [V] ~1m: `health=200`, `unauth_write=401`.
- [ ] **Step 204** [TEST][B] ~2m: Login → cookie → write
  ```bash
  cd /home/govan/tmp/unheaded && PW=$(grep -oE 'dev password: .*' /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/gen-dev-keys.log | awk '{print $3}'); J=/tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/cj; curl -s -c $J -o /dev/null -w 'login=%{http_code}\n' -X POST http://127.0.0.1:20000/auth/login -H 'Content-Type: application/json' -H 'X-Requested-With: unheaded' -d "{\"username\":\"dev\",\"password\":\"$PW\"}" && curl -s -b $J http://127.0.0.1:20000/auth/me && echo && grep -c 'unheaded_session' $J && curl -s -b $J -o /dev/null -w 'write_with_session=%{http_code}\n' -X POST http://127.0.0.1:20000/api/v1/compliance/profile -H 'Content-Type: application/json' -H 'X-Requested-With: unheaded' -d '{"profile":"baseline"}'; rm -f $J
  ```
- [ ] **Step 205** [V] ~1m: `login=200`, `/auth/me` shows `"sub":"dev"` and `operator`, cookie present, `write_with_session=` not 401/403 (200 or the handler's own validation code). Note the cookie lacks `Secure` here because compose is plaintext until Phase 8; the test in Step 187 pins that `Secure` is set whenever `-dev-plaintext` is off.
- [ ] **Step 206** [TEST][B] ~1m: Browser-side token leak check: the token never reaches the page
  ```bash
  cd /home/govan/tmp/unheaded && grep -rnE 'access_token|localStorage|sessionStorage' dashboard/js/auth.js | grep -vE '^\s*//' | wc -l
  ```
- [ ] **Step 207** [V] ~1m: `0`.
- [ ] **Step 208** [B] ~1m
  ```bash
  cd /home/govan/tmp/unheaded && docker compose stop auth dashboard-backend kanban-app && bash scripts/compliance/check-service-auth.sh 2>&1 | tail -3
  ```
- [ ] **Step 209** [V] ~1m: Only `wotan` remains in the FAIL list.
- [ ] **Step 210** [C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 202-209: BFF login proven live; only Wotan left open`.

### Kanban and wiki through the BFF

- [ ] **Step 211** [W][CODE] ~20m: `cmd/kanban-app`: it is browser-facing on 20001 and today serves its own UI. Two options; [DECIDE]: RECOMMENDATION: kanban-app accepts the dashboard's session cookie by validating it against the same issuer JWKS? No: cookies are per-origin and 20001 ≠ 20000. Recommended: kanban-app gets the same BFF code (`pkg/bff`, extracted from Step 190 into a shared package) with its own login route and `AUTH_ISSUER_URL`; same for `wiki-server` writes (none today). Rationale: one code path; ADR-103 §4 names kanban and wiki as BFFs. Override ONLY if the dashboard is to proxy kanban (bigger change).
- [ ] **Step 212** [W][CODE] ~25m: Extract `pkg/bff` from Step 190 (session store, cookie middleware, login/me/logout handlers parametrised by issuer URL and cookie name); dashboard-backend and kanban-app use it; `kanban/` JS gets the same `auth.js` (shared file under `dashboard/js/`, served by both; check how kanban serves static: `grep -n 'static' cmd/kanban-app/main.go | head -3`).
- [ ] **Step 213** [TEST][B] ~3m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/bff/... ./cmd/dashboard-backend/... ./cmd/kanban-app/... -race -count=1 2>&1 | tail -4 && bash scripts/check-js-lint.sh 2>&1 | tail -1
  ```
- [ ] **Step 214** [V] ~1m: `ok` x3, lint `[PASS]`.
- [ ] **Step 215** [TEST][B] ~5m: Kanban live: anonymous card write 401, session write ok, SSE stream still works unauthenticated for reads only if the kanban is meant to be publicly readable ([DECIDE] RECOMMENDATION: reads stay open, "Kanban board is public (optional auth)" per CLAUDE.md The Meta Moment; writes need `operator`).
  ```bash
  cd /home/govan/tmp/unheaded && docker compose up -d --build auth kanban-app && sleep 8 && curl -s -o /dev/null -w 'read=%{http_code}\n' http://127.0.0.1:20001/api/v1/tasks && curl -s -o /dev/null -w 'unauth_write=%{http_code}\n' -X POST http://127.0.0.1:20001/api/v1/tasks -H 'Content-Type: application/json' -d '{"title":"x"}'; docker compose stop auth kanban-app
  ```
- [ ] **Step 216** [V] ~1m: `read=200`, `unauth_write=401`.
- [ ] **Step 217** [C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 211-216: pkg/bff shared; kanban writes need a session`.
- [ ] **Step 218** [TEST][B] ~12m: Whole-tree regression against the baseline
  ```bash
  cd /home/govan/tmp/unheaded && go test ./... 2>&1 | grep -E '^(FAIL|---)' | tee /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/p6.txt | tail -10; echo "FAIL lines: $(grep -c '^FAIL' /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/p6.txt) vs baseline $(grep -c '^FAIL' docs/battle-plans/progress/S-IRON-GATE-baseline.txt)"
  ```
- [ ] **Step 219** [V] ~1m: FAIL count equals the baseline (zero NEW). Any new failure is a service whose tests assumed Noop auth: fix the test fixture to set `AUTH_ENABLED` unset or supply a token; never weaken the gate.
- [ ] **Step 220** [W][DOC-UPDATE] ~8m: `wiki/Service-Auth.md`: the BFF section (cookie, `X-Requested-With`, `act` claim); `wiki/Service-Dashboard.md` and `wiki/Service-Kanban.md` (or the pages `grep -l 20000 wiki/*.md` finds): login paragraph; `CLAUDE.md` "Authentication Framework" section: replace the HS256/Noop description with the ADR-103 shape and the env names (keep it to the facts).
- [ ] **Step 221** [C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 218-220: regression clean after the auth wave; docs`.
- [ ] **Step 222** [V]: **PHASE 6 EXIT GATE**
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/compliance/check-service-auth.sh 2>&1 | tail -3; grep -rn 'AUTH_JWT_KEY' docker-compose.yml deploy/systemd debian 2>/dev/null | wc -l; git status --short | wc -l
  ```
  - Pass: FAIL list = `wotan` only; `0` `AUTH_JWT_KEY` in deploy artefacts; tree clean. → Phase 7.
  - Fail → DO NOT PROCEED.

---

## PHASE 7: WOTAN LAST, AND IDENTITY-BOUND AUTO-APPROVE (Steps 223-250)

**Goal**: Every Wotan client presents a service token in gRPC metadata and HTTP headers; Wotan verifies it; auto-approve keys on the verified `sub`, not `display_name`. (M-050 part 7, M-052; ADR-103 §6, §7 step 5)
**Prerequisite**: Phase 6 exit gate (every caller can mint).
**Time**: 4 h
**Agent**: Coordinator

- [ ] **Step 223** [R] ~5m
  ```bash
  cd /home/govan/tmp/unheaded && grep -nE 'IsAutoApproved|displayName|display_name' services/wotan/internal/grpc/topic_service.go services/wotan/internal/api/topics.go | head -20 && grep -nE 'UnaryInterceptor|StreamInterceptor|grpc.NewServer' services/wotan/cmd/wotan/main.go | head && sed -n '70,120p' pkg/wotan-client/grpc_client.go
  ```
- [ ] **Step 224** [W][CODE] ~15m: Tests `pkg/wotan-client/auth_test.go`: `NewClientWithGRPC` given `WithServiceToken(minter)` attaches `authorization: Bearer <token aud=wotan>` to every unary and stream call (a `bufconn` gRPC server asserts metadata) and to every HTTP request (`httptest` asserts header); without a minter, nothing is attached (compatibility while Wotan is still open); the token refreshes when the minter rotates.
- [ ] **Step 225** [TEST][B] ~1m: Red
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/wotan-client/ -run 'ServiceToken|Auth' -count=1 2>&1 | tail -3
  ```
- [ ] **Step 226** [V] ~1m: Red.
- [ ] **Step 227** [W][CODE] ~30m: `pkg/wotan-client`: option `WithServiceToken(m *auth.ServiceTokenMinter)`; a `credentials.PerRPCCredentials` implementation (`RequireTransportSecurity()` returns `true` unless the client was built with `WithInsecureTransport()` for tests/localhost; this makes Phase 8 TLS the default path); HTTP control-plane requests set `Authorization`. Wire every service's Wotan client construction (grep `NewClientWithGRPC(` across cmd/ services/) to pass the minter from Phase 6.
- [ ] **Step 228** [TEST][B] ~3m: Green
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/wotan-client/... -race -count=1 2>&1 | tail -2 && go build ./... && echo BUILD_OK
  ```
- [ ] **Step 229** [V] ~1m: `ok`, `BUILD_OK`.
- [ ] **Step 230** [C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 223-229: wotan-client sends service tokens (gRPC metadata + HTTP)`.
- [ ] **Step 231** [W][CODE] ~15m: Tests `services/wotan/internal/grpc/auth_test.go` + `services/wotan/internal/api/auth_test.go`: with `AUTH_ENABLED=true`, a publish/subscribe without a token → `codes.Unauthenticated` / HTTP 401; with a token whose `aud≠wotan` → refused; with a valid token, the verified `sub` becomes the member identity; `AutoApprover.IsAutoApproved(identity)` is consulted with the verified `sub` and `display_name` is ignored for approval (it stays as a label); `/health`, `/ready`, `/metrics`, `grpc.health.v1` open.
- [ ] **Step 232** [TEST][B] ~1m: Red
  ```bash
  cd /home/govan/tmp/unheaded && go test ./services/wotan/... -run 'Auth' -count=1 2>&1 | tail -4
  ```
- [ ] **Step 233** [V] ~1m: Red.
- [ ] **Step 234** [W][CODE] ~45m: `services/wotan/cmd/wotan/main.go`: build the `auth` config with `LoadServiceAuthConfig("wotan")`; add `auth.GRPCUnaryInterceptor` / `GRPCStreamInterceptor` to `grpcOpts` (skip `grpc.health.v1`), and `auth.WrapHandler` already on the HTTP mux. `internal/grpc/topic_service.go`: read `auth.IdentityFromContext(ctx)`; change `AutoApprover` to `IsAutoApproved(identity *auth.Identity, displayName string) bool` where the api.TopicConfig implementation matches `identity.Subject` against `topics.auto_approve` and, when `identity == nil` (auth off), falls back to the old display-name behaviour with a one-time WARN log naming ADR-103. Same on the HTTP path in `internal/api/topics.go`. Member records store `sub`. `configs/wotan.yaml` `auto_approve`: rename entries to service names (`timeguru`, `loadbalancer`, … one per first-party service and `heimdall`, `chaos-controller`, `unheaded-cli`), and delete the catch-all `service` (every caller now has a name).
- [ ] **Step 235** [TEST][B] ~3m: Green
  ```bash
  cd /home/govan/tmp/unheaded && go test ./services/wotan/... -race -count=1 2>&1 | tail -3
  ```
- [ ] **Step 236** [V] ~1m: `ok`.
- [ ] **Step 237** [C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 231-236: Wotan verifies service tokens; auto-approve keys on verified sub`.
- [ ] **Step 238** [W] ~5m: compose `wotan`: `AUTH_ENABLED: "true"`, `AUTH_ALLOW_CALLERS` = every first-party service name + `heimdall,chaos-controller,unheaded-cli,kev-poller,huginn,anomaly,trace-collector`; unit `unheaded-wotan.service`: `Environment=AUTH_ENABLED=true` + the list; east's `wotan.yaml` note in Phase 14 (M-041 adds chaos-controller already).
- [ ] **Step 239** [BUILD][B] ~8m: Whole stack live (auth on everywhere)
  ```bash
  cd /home/govan/tmp/unheaded && docker compose up -d --build 2>&1 | tail -3 && sleep 25 && for s in wotan timeguru captain architect micromanager monad sophia dashboard-backend kanban-app cuirass akira; do printf '%-18s ' $s; docker compose logs --tail=200 $s 2>&1 | grep -ciE 'unauthenticated|401|403|permission denied'; done
  ```
- [ ] **Step 240** [V] ~2m: Every count `0`. A non-zero count names a caller missing from Wotan's allowlist or a service without a key secret → Step 240a [D]: `docker compose logs <svc> | grep -iE 'auth|token' | tail -5`; fix the allowlist or the secret, `docker compose up -d <svc>`, recount. Max 2 rounds.
- [ ] **Step 241** [TEST][B] ~2m: Anonymous publish refused; the dashboard still streams
  ```bash
  cd /home/govan/tmp/unheaded && curl -s -o /dev/null -w 'anon_publish=%{http_code}\n' -X POST http://127.0.0.1:18000/api/v1/topics/test.iron/publish -H 'Content-Type: application/json' -d '{"payload":"x"}' && curl -s http://127.0.0.1:18000/api/v1/topics 2>/dev/null | head -c 200; echo; curl -s -o /dev/null -w 'dashboard_flows=%{http_code}\n' http://127.0.0.1:20000/api/v1/flows
  ```
- [ ] **Step 242** [V] ~1m: `anon_publish=401`; topics list is either 401 (if reads are gated) or the JSON (reads open is acceptable, record which); `dashboard_flows=200`.
- [ ] **Step 243** [TEST][B] ~2m: Discovery and health still register (CLAUDE.md: 11 services join `system.discovery`)
  ```bash
  cd /home/govan/tmp/unheaded && sleep 20 && docker compose logs --tail=50 wotan 2>&1 | grep -c 'system.discovery' ; curl -s http://127.0.0.1:19100/api/v1/consensus 2>/dev/null | head -c 200; echo
  ```
- [ ] **Step 244** [V] ~1m: discovery count ≥ 1; Akira consensus answers (band `OK` or whatever the stack reports; not an auth error).
- [ ] **Step 245** [B] ~1m
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/compliance/check-service-auth.sh 2>&1 | tail -2
  ```
- [ ] **Step 246** [V] ~1m: `[PASS] all 13 first-party services set AUTH_ENABLED=true` (12 + auth).
- [ ] **Step 247** [C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 238-246: whole stack with auth on; check-service-auth PASS`.
- [ ] **Step 248** [AUDIT-UPDATE][W] ~5m: `compliance/findings/register.yaml`: the `gate-script:scripts/compliance/check-service-auth.sh` finding (step 7, high) → status closed with the Step 246 evidence line; `scripts/compliance/check-service-auth.sh` header comment: it is now a CI gate (add it to `.github/workflows/security.yml` `security-gate` job's list, mirroring how `check-systemd-units.sh` is invoked).
- [ ] **Step 249** [B] ~3m
  ```bash
  cd /home/govan/tmp/unheaded && go run ./cmd/compliance-evidence findings -adr docs/adr/ADR-098-findings-register-and-baseline-modes.md 2>&1 | tail -2 && git diff --stat docs/adr/ADR-098-findings-register-and-baseline-modes.md compliance/findings/register.yaml | tail -2 && docker compose down 2>&1 | tail -1
  ```
- [ ] **Step 250** [V]: **PHASE 7 EXIT GATE** — Steps 229, 236, 240, 242, 244, 246 passed; ADR-098 section regenerated; stack down; commit `[PLAN S-IRON-GATE] Steps 248-250: check-service-auth is a CI gate; finding closed`. → Phase 8.

---

## PHASE 8: TLS ON EVERY INTERNAL LINK (Steps 251-269)

**Goal**: Wotan and the twelve services listen and dial TLS 1.3 with the Strict hybrid profile; a classical-only offer is refused; plaintext REST from a non-loopback address is refused. FND-017 closes. (M-051)
**Prerequisite**: Phase 7 exit gate (tokens travel over TLS from now on; `RequireTransportSecurity` is true).
**Time**: 4 h (+ E3 host window)
**Agent**: Coordinator

- [ ] **Step 251** [B] ~2m: Dev certificates (CA + per-service, SANs = compose service name + `localhost` + `127.0.0.1`)
  ```bash
  cd /home/govan/tmp/unheaded && sed -n '25,60p' scripts/gen-dev-certs.sh | grep -nE 'SERVICES|san|SAN|dns' | head -6; CERT_DIR=configs/certs/dev bash scripts/gen-dev-certs.sh 2>&1 | tail -3 && ls configs/certs/dev | wc -l && git check-ignore configs/certs/dev && echo IGNORED
  ```
- [ ] **Step 252** [V] ~1m: certs generated for every first-party service plus `auth` (add `auth` and `unheaded-cli` to the script's default list if absent); `IGNORED` printed. If the SAN list lacks the compose service names → fix the script's SAN template first (TLS verification by name is the point).
- [ ] **Step 253** [W][CODE] ~20m: Tests `pkg/tls/strict_server_test.go`: `MaybeWrapServer` under `UNHEADED_TLS_ENABLED=true` serves TLS 1.3 only; a client offering only `X25519` fails the handshake (`tls: no supported group`-class error); a hybrid client succeeds; `UNHEADED_TLS_ENABLED=true` with a missing cert file is a startup error, not a silent plaintext fallback; `pkg/wotan-client` with `WithTLS(cfg)` dials gRPC with `credentials.NewTLS` (no `insecure`), verifies the server name.
- [ ] **Step 254** [TEST][B] ~1m: Red where behaviour is missing
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/tls/ ./pkg/wotan-client/ -run 'Strict|WithTLS|NoSilentFallback' -count=1 2>&1 | tail -4
  ```
- [ ] **Step 255** [W][CODE] ~40m: Make it green: `pkg/tls/setup.go` `MaybeWrapServer` applies `ApplyStrict` and refuses to start plaintext when TLS is requested but misconfigured; a shared helper `pkg/tls.ClientFromEnv()` for every service's outgoing HTTP; `pkg/wotan-client` `WithTLS`/`FromEnv()` replacing both `insecure.NewCredentials()` sites behind an explicit `WithInsecureTransport()` that logs WARN once and is refused when `UNHEADED_TLS_ENABLED=true`. Every service's main: wrap the listener with `tls.MaybeWrapServer` (grep `ListenAndServe(` across cmd/ services/ and replace; many already call `MaybeWrapServer`, check with `grep -rl MaybeWrapServer cmd services`). Non-loopback plaintext: when TLS is on, `MaybeWrapServer` is the only listener; when TLS is off and the bind address is not loopback, log WARN `plaintext listener on non-loopback address (FND-017)` so the compose-without-TLS case stays visible.
- [ ] **Step 256** [TEST][B] ~4m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/tls/... ./pkg/wotan-client/... -race -count=1 2>&1 | tail -3 && go build ./... && echo BUILD_OK && grep -rn 'insecure.NewCredentials' pkg cmd services --include=*.go | grep -v _test | wc -l
  ```
- [ ] **Step 257** [V] ~1m: `ok` x2, `BUILD_OK`, insecure sites `1` (the explicit opt-in helper only).
- [ ] **Step 258** [C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 251-257: Strict TLS listeners, no silent plaintext fallback, wotan-client TLS creds`.
- [ ] **Step 259** [W] ~20m: compose: every first-party service gets `./configs/certs/dev:/etc/unheaded/tls:ro` plus `UNHEADED_TLS_ENABLED: "true"`, `UNHEADED_TLS_CA_CERT: /etc/unheaded/tls/ca.pem`, `UNHEADED_TLS_CERT: /etc/unheaded/tls/<svc>.pem`, `UNHEADED_TLS_KEY_FILE: /etc/unheaded/tls/<svc>-key.pem` (match the file names `gen-dev-certs.sh` writes); `wotan` command gains `--enable-tls --tls-cert /etc/unheaded/tls/wotan.pem --tls-key /etc/unheaded/tls/wotan-key.pem`; every `WOTAN_ADDR`/`WOTAN_GRPC_ADDR` stays the same host:port (TLS is the same port); `AUTH_ISSUER_URL` → `https://auth:19020`; healthchecks that use `wget http://` → `https://` with the CA (`--ca-certificate=/etc/unheaded/tls/ca.pem`) or the binary's `probe` subcommand. Keys are per-service private material: mount only that service's key, so use one bind per service to a per-service sub-dir (`configs/certs/dev/<svc>/`), and have `gen-dev-certs.sh` lay files out that way (`CA` copied into each).
- [ ] **Step 260** [B] ~1m
  ```bash
  cd /home/govan/tmp/unheaded && docker compose config --quiet && echo COMPOSE_OK && bash scripts/check-compose-hardening.sh 2>&1 | tail -1 && bash scripts/check-compose-bind-nesting.sh 2>&1 | tail -1
  ```
- [ ] **Step 261** [V] ~1m: `COMPOSE_OK`, two `[PASS]`.
- [ ] **Step 262** [BUILD][B] ~8m: Full stack over TLS
  ```bash
  cd /home/govan/tmp/unheaded && docker compose up -d --build 2>&1 | tail -2 && sleep 30 && for s in wotan timeguru captain architect micromanager monad sophia dashboard-backend kanban-app cuirass akira auth; do printf '%-18s ' $s; docker compose logs --tail=200 $s 2>&1 | grep -ciE 'tls|x509|handshake|certificate' ; done | awk '{print}'; docker compose logs --tail=200 2>&1 | grep -iE 'x509|handshake failure|certificate' | head -5
  ```
- [ ] **Step 263** [V] ~2m: No `x509`/`handshake failure` lines. A failure names the SAN or CA mismatch → Step 263a [D]: `openssl x509 -in configs/certs/dev/<svc>/<svc>.pem -noout -ext subjectAltName`; regenerate with the right names; `docker compose up -d <svc>`. Max 2 rounds.
- [ ] **Step 264** [TEST][B] ~2m: FND-017's verify command: hybrid negotiates, classical-only is refused, on Wotan's gRPC port
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/probe-tls-groups.sh 127.0.0.1:18001 2>&1 | tail -4; echo "--- plaintext to a TLS port:"; curl -s -o /dev/null -w 'plain=%{http_code} err=%{errormsg}\n' http://127.0.0.1:18000/health
  ```
- [ ] **Step 265** [V] ~1m: hybrid line shows `X25519MLKEM768`; classical-only line shows the refusal; the plaintext curl fails (`err=` non-empty, no 200).
- [ ] **Step 266** [TEST][B] ~2m: Everything still works over TLS: auth round trip (Step 204 with `https://` and `--cacert configs/certs/dev/ca.pem`), dashboard flows, Akira consensus, check-service-auth
  ```bash
  cd /home/govan/tmp/unheaded && CA=configs/certs/dev/ca.pem; curl -s --cacert $CA -o /dev/null -w 'dash_health=%{http_code}\n' https://127.0.0.1:20000/health && curl -s --cacert $CA -o /dev/null -w 'akira=%{http_code}\n' https://127.0.0.1:19100/health && bash scripts/compliance/check-service-auth.sh 2>&1 | tail -1 && docker compose down 2>&1 | tail -1
  ```
- [ ] **Step 267** [V] ~1m: `200`, `200`, `[PASS]`. (If a published port is not mapped for a service, skip that curl and note it.)
- [ ] **Step 268** [W] ~15m: `deploy/systemd/*.service` for the thirteen first-party units: `Environment=UNHEADED_TLS_ENABLED=true` + the three path vars under `/etc/unheaded/tls/<svc>/`; `unheaded-wotan.service` ExecStart gains the three TLS flags; `deploy/systemd/README.md` "TLS (ADR-105/FND-017)": cert issuance on a host (`scripts/gen-dev-certs.sh` is dev-only; hosts use the Kingdom CA runbook `runbooks/pki/generate-kingdom-certs.yaml`, cite it), file ownership `root:unheaded-<svc> 0640` for the key, 0644 for certs. `scripts/check-systemd-units.sh` and `check-unit-binaries.sh` pass. [ESCALATE E3] **Installing certs and restarting units on west/east** is a host window: Context: units are ready; Question: "open a window on west (then east) to install `/etc/unheaded/tls/<svc>/` material and restart the units?"; why Stevie's: host change, brief outage. Chain stops here; Phase 14's host steps are [AFTER-E3] too.
- [ ] **Step 269** [V]: **PHASE 8 EXIT GATE**
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/tls/... ./pkg/wotan-client/... -race -count=1 2>&1 | tail -2 && bash scripts/check-systemd-units.sh 2>&1 | tail -1 && bash scripts/check-pq-kex.sh 2>&1 | tail -1 && git add -A && git commit --no-gpg-sign -q -m "[PLAN S-IRON-GATE] Steps 259-268: compose and units on Strict TLS; FND-017 verify passes in compose

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01TzL3TuPhKAVVHnD9jzuabE" && git status --short | wc -l
  ```
  - Pass: `ok` x2, `[PASS]` x2, `0`. FND-017 moves to "closed in compose; hosts [AFTER-E3]" in the register (Phase 16 regenerates). → Phase 9 (and Phases 10, 11, 12 in parallel).
  - Fail → DO NOT PROCEED.

---

## PHASE 9: SIGNED TOPIC PREFIXES AND HEIMDALL DRIFT KEYS (Steps 270-285)

**Goal**: `audit.`, `health.` and `alerts.critical` join the signed-prefix policy; Heimdall has a unit, a drift key per node (E2) and lands on `drift.detected.<node>`. (M-053, M-201; FND-019)
**Prerequisite**: Phase 8 exit gate.
**Time**: 3 h (+ E2)
**Agent**: Coordinator

- [ ] **Step 270** [W][CODE] ~12m: Tests `services/wotan/internal/signing/policy_prefix_test.go`: a policy built from `signed_prefixes` with `audit.`, `health.`, `alerts.critical.` refuses an unsigned publish under each and accepts one signed by the prefix's key; a key trusted for `audit.` does not verify `health.` (ADR-043 #4 key separation already enforced for `config.`/`drift.`; this extends the table); `ValidatePrefix` accepts `alerts.critical.` and refuses `alerts.` (too broad: `alerts.*` carries unsigned service alerts today; [DECIDE] RECOMMENDATION: sign only `alerts.critical.`; Rationale: every service MUST subscribe to it per CLAUDE.md, so a forged critical alert has the widest blast radius; override ONLY if Stevie wants all alerts signed).
- [ ] **Step 271** [TEST][B] ~1m: Red
  ```bash
  cd /home/govan/tmp/unheaded && go test ./services/wotan/internal/signing/ -run 'Prefix' -count=1 2>&1 | tail -3
  ```
- [ ] **Step 272** [W][CODE] ~25m: Green: the policy already supports arbitrary prefixes from config (`signed_prefixes`), so the code change is small (any validation that blocked `alerts.critical.`); the real work is producers: `pkg/wotansig` `TopicSigner` wired into the publishers of `audit.*` (grep `"audit\.` across cmd/ services/ pkg/), `health.*` (Akira `system.health.*`? check the exact topic names; only prefixes that exist get signed) and `alerts.critical`; each producer loads its ML-DSA-65 key from `AUDIT_SIGNING_KEY_FILE`/`HEALTH_SIGNING_KEY_FILE` (0400, same owner rules as Phase 2) or, if unset, refuses to publish under a signed prefix with a clear error (never silently unsigned). `configs/wotan.yaml` `signed_prefixes` gains the three prefixes with `keys: []` + comment "enrol per deployment (E2 pattern)".
- [ ] **Step 273** [TEST][B] ~3m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./services/wotan/... ./pkg/wotansig/... -race -count=1 2>&1 | tail -3 && go build ./... && echo BUILD_OK
  ```
- [ ] **Step 274** [V] ~1m: `ok`, `BUILD_OK`.
- [ ] **Step 275** [C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 270-274: audit./health./alerts.critical. signed prefixes; producers sign or refuse`.
- [ ] **Step 276** [W] ~15m: `deploy/systemd/unheaded-heimdall.service`: sandbox block from the Wotan unit, `User=unheaded-heimdall` (add to sysusers: `u unheaded-heimdall - "Unheaded Heimdall drift monitor" / /usr/sbin/nologin`; uid from pkg/uids: add `Heimdall = 16736` to the Kingdom tier), `ExecStart=/usr/local/bin/heimdall-daemon --manifest /var/lib/unheaded/heimdall/manifest.json --pubkey /etc/unheaded/keys/heimdall-manifest.pub --drift-key /etc/unheaded/keys/heimdall-drift.key --wotan-grpc 127.0.0.1:18001 --node-id %H` (match the daemon's real flag names: `grep -nE 'flag\.(String|Bool|Duration)' cmd/heimdall-daemon/main.go`), `ReadOnlyPaths=/` for the scan plus `ReadWritePaths=/var/lib/unheaded/heimdall`; `AmbientCapabilities=CAP_DAC_READ_SEARCH` only (it reads root-owned files; it never writes outside its state dir), `SystemCallFilter=@system-service`. Add `heimdall-daemon` to `scripts/build-binaries.sh` and `SERVICES` in `build-all-debs.sh`.
- [ ] **Step 277** [B] ~1m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/uids/ -count=1 2>&1 | tail -1 && bash scripts/check-systemd-units.sh 2>&1 | tail -1 && bash scripts/check-unit-binaries.sh 2>&1 | tail -1
  ```
- [ ] **Step 278** [V] ~1m: `ok`, `[PASS]` x2.
- [ ] **Step 279** [W][CODE] ~10m: `cmd/unheaded-keygen` gains `-algo mldsa65` (uses `pkg/gungnir` seed format, `gungnir.LoadSeedFile`-compatible) so drift keys and signing-prefix keys are made by the same tool with the same ownership rules; test: round-trip seed → `PublicKeyFromSeed` → base64 for `wotan.yaml`.
- [ ] **Step 280** [TEST][B] ~1m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/unheaded-keygen/... -race -count=1 2>&1 | tail -1
  ```
- [ ] **Step 281** [V] ~1m: `ok`.
- [ ] **Step 282** [W] ~10m: Runbook `runbooks/security/heimdall-enrol-drift-key.yaml` (runbook/v1 schema; copy the metadata shape of `runbooks/pki/rotate-service-cert.yaml`): on a node, `unheaded-keygen -algo mldsa65 -owner unheaded-heimdall heimdall-drift`; print the public key base64; append to `configs/wotan.yaml` `signed_prefixes[drift.].keys` and `topics.auto_approve: heimdall` on the Wotan host; `systemctl restart unheaded-wotan`; verify with a provoked drift (`touch` a manifest-listed file in a scratch copy, never a real one) that `drift.detected.<node>` arrives and `wotan-cli` re-verifies. `scripts/check-runbooks.sh` passes; mark `status: draft`.
- [ ] **Step 283** [ESCALATE][AFTER-E2] ~0m: **E2, drift-key custody and running Heimdall on the hosts.** Context: unit, keygen, runbook ready (Steps 276-282). Questions: (1) generate the drift key on each node (west, east) with `unheaded-keygen` under `unheaded-heimdall`, or generate centrally and copy? (2) permission to run the root-reading daemon on both hosts. Why Stevie's: key custody and a privileged daemon on the hosts. The chain (M-201 live, FND-019 verify) stops here; everything else continues.
- [ ] **Step 284** [C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 276-283: Heimdall unit + uid, keygen mldsa65, enrol runbook; E2 escalated`.
- [ ] **Step 285** [V]: **PHASE 9 EXIT GATE** — Steps 274, 278, 281 passed; `bash scripts/check-runbooks.sh` `0 new finding(s)`; tree clean. E2 open does not block the gate. → Phases 10-12.

---

## PHASE 10: AKIRA CERT-EXPIRY CHECK AND A ROTATION TEST (Steps 286-295)

**Goal**: Akira runs the existing cert-expiry runbook as an exec check and one end-to-end rotation is proven with a short-lived cert. (M-054; ADR-034 Phases 7/8)
**Prerequisite**: Phase 8 exit gate (certs exist).
**Time**: 2 h
**Agent**: Agent [P:11,12]

- [ ] **Step 286** [R] ~3m
  ```bash
  cd /home/govan/tmp/unheaded && grep -nE 'exec|runbook|check' configs/akira-west.yaml | head -12 && grep -rnE 'type.*exec|ExecCheck|runCommand' cmd/akira/*.go pkg/health/*.go 2>/dev/null | grep -v _test | head -8
  ```
- [ ] **Step 287** [DECIDE] ~2m: **Does Akira already support exec checks?** If Step 286 shows an exec check type → configure it. If not: RECOMMENDATION: add a minimal `exec` check type to Akira (command, timeout 30 s, non-zero = failing vote, output capped 4 KiB, runs as Akira's own uid, no shell: argv only). Rationale: ADR-034 Phase 7 names "Akira cert-expiry exec check"; one check type serves every runbook-shaped probe. Override ONLY if Akira is meant to stay HTTP-only (then the runbook runs from a systemd timer instead and publishes to `system.health.reports` itself).
- [ ] **Step 288** [W][CODE] ~25m: Tests + implementation per Step 287 (red → green with `-race`): `pkg/health/exec_check_test.go` (exit 0 = ok, exit 1 = failing, timeout = failing, output cap, argv-only), config entry in `configs/akira-west.yaml` and `akira-east.yaml`:
  ```yaml
  - name: cert-expiry
    type: exec
    argv: ["/usr/local/bin/run-runbook", "--dry-run=false", "/usr/share/unheaded/runbooks/pki/akira-cert-expiry-check.yaml"]
    interval: 6h
    timeout: 30s
  ```
  (match the real runner path: `grep -rn 'run-runbook\|run_runbook' scripts runbooks | head -3`; if the runner is Python, argv is `["python3","-B","<runner>", ...]`.)
- [ ] **Step 289** [TEST][B] ~2m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/health/... ./cmd/akira/... -race -count=1 2>&1 | tail -3
  ```
- [ ] **Step 290** [V] ~1m: `ok`.
- [ ] **Step 291** [W][CODE] ~15m: `runbooks/pki/akira-cert-expiry-check.yaml`: point `PKI` at `/etc/unheaded/tls` (the Phase 8 layout) with `/var/lib/unheaded/pki` as fallback; thresholds unchanged; `tests/runbooks/test_akira_cert_expiry.py`: dry-run passes schema; a fixture cert with 20 % lifetime left makes the runbook's check step exit non-zero (use `openssl req -x509 -days 1` into a temp dir, then `faketime`-free: generate with `-not_before`/`-not_after` via `openssl x509 -days` and assert on the runbook's percentage math with a 1-day cert where remaining < 33 %).
- [ ] **Step 292** [TEST][B] ~2m
  ```bash
  cd /home/govan/tmp/unheaded && export PATH=$HOME/.venv/zhen/bin:$PATH; python3 -m pytest tests/runbooks/test_akira_cert_expiry.py -q 2>&1 | tail -2 && bash scripts/check-runbooks.sh 2>&1 | tail -1
  ```
- [ ] **Step 293** [V] ~1m: tests pass, `0 new finding(s)`.
- [ ] **Step 294** [TEST][B] ~6m: End-to-end rotation in compose: issue a 2-minute cert for `timeguru`, start the stack subset, watch Akira's exec check flip to failing, run `rotate-service-cert.yaml` (dry-run false) for timeguru, confirm the check returns to ok and timeguru's listener serves the new cert
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/gen-dev-certs.sh timeguru >/dev/null && openssl x509 -in configs/certs/dev/timeguru/timeguru.pem -noout -enddate && echo "(set CERT_VALIDITY=2m and regenerate timeguru; then:)" && CERT_VALIDITY=2m bash scripts/gen-dev-certs.sh timeguru >/dev/null && docker compose up -d --build wotan auth akira timeguru && sleep 90 && curl -s --cacert configs/certs/dev/ca.pem https://127.0.0.1:19100/api/v1/checks 2>/dev/null | grep -o '"cert-expiry"[^}]*' | head -1; bash scripts/gen-dev-certs.sh timeguru >/dev/null && docker compose restart timeguru && sleep 20 && curl -s --cacert configs/certs/dev/ca.pem https://127.0.0.1:19100/api/v1/checks 2>/dev/null | grep -o '"cert-expiry"[^}]*' | head -1; docker compose down 2>&1 | tail -1
  ```
- [ ] **Step 295** [V]: **PHASE 10 EXIT GATE** — the first `cert-expiry` read shows failing/warn, the second shows ok; Steps 290, 293 passed; commit `[PLAN S-IRON-GATE] Steps 286-294: Akira exec check, cert-expiry runbook on the TLS layout, rotation proven`. ADR-034 Phases 7/8 ticked in the ADR. → Phase 13 (after 11, 12).

---

## PHASE 11: SECURITY HEADERS ON EVERY ROUTER (Steps 296-303)

**Goal**: One middleware in `pkg/httputil` sets the security headers; every service's mux uses it. (M-056)
**Prerequisite**: Phase 0.
**Time**: 1.5 h
**Agent**: Agent [P:10,12]

- [x] **Step 296** [W][CODE] ~10m: Tests `pkg/httputil/security_headers_test.go`: `SecurityHeaders(next)` sets `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`, `Cache-Control: no-store` on `/api/` and `/auth/` paths, `Permissions-Policy: camera=(), microphone=(), geolocation=()`, `Content-Security-Policy: default-src 'self'; frame-ancestors 'none'; base-uri 'none'` (opt-out per service via `WithCSP(string)` for the dashboard's inline needs), `Strict-Transport-Security: max-age=31536000` only when the request arrived over TLS (`r.TLS != nil`); `/metrics` keeps `Content-Type` from the handler; WebSocket upgrades untouched.
- [x] **Step 297** [TEST][B] ~1m: Red
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/httputil/ -run 'SecurityHeaders' -count=1 2>&1 | tail -3
  ```
- [x] **Step 298** [W][CODE] ~15m: `pkg/httputil/security_headers.go`; apply it in every first-party router next to the `NewServiceMetrics(...).Instrument` call (grep `Instrument(` across cmd/ services/; same 13 binaries + wiki-server + protocol-api + trace-collector-go + doom-bridge + chaos-controller). Dashboard and kanban: `WithCSP` allowing `'unsafe-inline'` for styles only if `scripts/check-js-lint.sh` / a quick page load proves inline scripts are not used; otherwise move inline scripts out (one small follow-on commit).
- [x] **Step 299** [TEST][B] ~3m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/httputil/... -race -count=1 2>&1 | tail -1 && go build ./... && echo BUILD_OK && grep -rl 'SecurityHeaders(' cmd services --include=*.go | grep -v _test | wc -l
  ```
- [x] **Step 300** [V] ~1m: `ok`, `BUILD_OK`, count ≥ 17.
- [ ] **Step 301** [TEST][B] ~4m: Live header check on three services
  ```bash
  cd /home/govan/tmp/unheaded && docker compose up -d --build wotan auth dashboard-backend kanban-app && sleep 15 && for p in 18000/health 20000/ 20001/; do echo "--- $p"; curl -sk -D - -o /dev/null https://127.0.0.1:$p | grep -iE 'x-content-type|x-frame|content-security|strict-transport|referrer' ; done; docker compose down 2>&1 | tail -1
  ```
- [ ] **Step 302** [V] ~1m: Each service prints the five headers (HSTS present because TLS). Dashboard page still renders: open `https://127.0.0.1:20000/` once in a browser or `curl -sk https://127.0.0.1:20000/ | grep -c '<script'` ≥ 1 and no CSP violation in the dashboard-backend log.
- [ ] **Step 303** [V]: **PHASE 11 EXIT GATE** — Steps 300, 302 passed; commit `[PLAN S-IRON-GATE] Steps 296-302: security headers middleware on every router`.

---

## PHASE 12: RELEASE WORKFLOW, SIGNED (Steps 304-328)

**Goal**: `release.yml` builds every binary, deb and eBPF object, signs them with keyless cosign, attaches SLSA provenance and the SBOM; Scorecard and CodeQL run; ADR-089's byte-size pin is a script. (M-060, M-063, M-065, M-066)
**Prerequisite**: Phase 0 (independent of auth).
**Time**: 4 h
**Agent**: Agent (worktree `release`) [P:10,11]

- [x] **Step 304** [R] ~3m
  ```bash
  cd /home/govan/tmp/unheaded && sed -n '1,40p' scripts/build-binaries.sh | grep -nE 'BINARIES|case|known' | head && grep -nE 'uses:' .github/workflows/sbom.yml .github/workflows/security.yml | sort -u -t: -k3 | head -20
  ```
- [x] **Step 305** [W] ~20m: Rewrite `.github/workflows/release.yml` `build` job: single `ubuntu-latest` runner (no darwin matrix: nothing here runs on darwin); `permissions: contents: write, id-token: write, attestations: write`; steps: checkout (pinned SHA already), setup-go (pinned), `./scripts/build-binaries.sh --out dist/bin` for every known binary, `./scripts/build-all-debs.sh "${GITHUB_REF_NAME#v}"` with `DEB_OUTPUT_DIR=dist/debs` (after Phase 14 fixes the name bug; until then this step is allowed to produce `unheaded-daemon_*.deb` only when Step 349 has landed, so order Phase 14 Steps 348-352 before the first tag), eBPF objects via `make ebpf` (Rust toolchain pinned via `dtolnay/rust-toolchain@<sha>` as `security.yml` does), `sha256sum` into `dist/checksums.txt`, upload-artifact. Remove the `wotan/cmd/wotan` paths.
- [x] **Step 306** [W] ~15m: `release` job: download artifacts; SBOM with pinned syft (keep); **cosign**: `sigstore/cosign-installer@<sha>` then `cosign sign-blob --yes --bundle dist/<file>.sigstore.json dist/<file>` for every file in `dist/` (keyless, OIDC from the workflow's `id-token`), plus `cosign attest-blob --yes --predicate dist/sbom.json --type cyclonedx dist/checksums.txt`; **SLSA**: `actions/attest-build-provenance@<sha>` with `subject-path: dist/*` (GitHub's native provenance generator, simpler than the slsa-github-generator reusable workflow and pinnable by SHA; [DECIDE] RECOMMENDATION: use it; override ONLY if SLSA level 3 via the reusable workflow is required); release upload includes `*.sigstore.json`. Resolve every new action's SHA with `git ls-remote https://github.com/<owner>/<repo> 'refs/tags/<tag>^{}'` and write `@<sha> # <tag>` as `scripts/check-actions-pinned.sh` demands.
- [x] **Step 307** [B] ~2m
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/check-actions-pinned.sh 2>&1 | tail -2 && python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/release.yml')); print('YAML_OK')" && grep -c 'echo "TODO' .github/workflows/release.yml
  ```
- [x] **Step 308** [V] ~1m: `[PASS]`, `YAML_OK`, `0`.
- [x] **Step 309** [W] ~10m: `scripts/verify-release.sh` (new): given a tag, downloads `checksums.txt` + its bundle from the GitHub release and runs `cosign verify-blob --bundle ... --certificate-identity-regexp 'https://github.com/unheaded/unheaded/.github/workflows/release.yml@refs/tags/v.*' --certificate-oidc-issuer https://token.actions.githubusercontent.com`, then `sha256sum -c`; documented in `wiki/Releases.md` (new). Test: `tests/release/verify_release_test.sh` runs the script against a locally produced bundle made with `cosign sign-blob --key` from a throwaway key (`cosign generate-key-pair` into a temp dir, `COSIGN_PASSWORD=""`), so the parsing path is exercised without network.
- [x] **Step 310** [B] ~3m
  ```bash
  cd /home/govan/tmp/unheaded && which cosign || (echo "install cosign for local tests: scripts/ci-install-tools.sh \$RUNNER_TEMP/bin cosign (add cosign to tool-pins.sh first)"); bash tests/release/verify_release_test.sh 2>&1 | tail -3
  ```
- [x] **Step 311** [V] ~1m: PASS. If `cosign` is not in `scripts/lib/tool-pins.sh`, add `COSIGN_VERSION` + sha256 there (tool freshness gate covers it) and teach `ci-install-tools.sh` to fetch it; re-run.
- [x] **Step 312** [C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 304-311: release.yml builds everything, keyless cosign + provenance; verify-release.sh`.
- [x] **Step 313** [W] ~10m: `.github/workflows/scorecard.yml` (ossf/scorecard-action pinned by SHA, `publish_results: true`, weekly + on push to develop, `permissions: security-events: write, id-token: write`) and `.github/workflows/codeql.yml` (github/codeql-action init/analyze pinned, languages `go, javascript, python`, on push/PR to develop/staging/main + weekly; `build-mode: autobuild` for Go). Pins via `git ls-remote` as in Step 306.
- [x] **Step 314** [B] ~1m
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/check-actions-pinned.sh 2>&1 | tail -1 && for f in scorecard codeql; do python3 -c "import yaml; yaml.safe_load(open('.github/workflows/$f.yml')); print('$f YAML_OK')"; done
  ```
- [x] **Step 315** [V] ~1m: `[PASS]`, two `YAML_OK`. (Results appear only after the push; Phase 16 records "first run pending push" in next.md.)
- [x] **Step 316** [W][CODE] ~15m: ADR-089 entry gate as a script: `scripts/check-ebpf-size-pin.sh`: builds `monad-cpu-ebpf --features ascend-linux` (`cargo build --release -p monad-cpu-ebpf --features ascend-linux` under `ebpf/`), finds the object (`ebpf/target/bpfel-unknown-none/release/monad-cpu-ebpf`), compares its byte size to `docs/policy/ebpf-size-pins.txt` (`monad-cpu-ebpf+ascend-linux <bytes>`); `--update` rewrites the pin and prints the delta; without `--update`, any difference fails with both numbers. Pin file seeded from the current build (the ADR's 901,888 is from July; S5 measured 926,542 verified insns, so the byte size has moved: record today's).
- [x] **Step 317** [B] ~6m
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/check-ebpf-size-pin.sh --update 2>&1 | tail -2 && bash scripts/check-ebpf-size-pin.sh 2>&1 | tail -1 && cat docs/policy/ebpf-size-pins.txt
  ```
- [x] **Step 318** [V] ~1m: second run `[PASS]`; pin file has one line with a byte count.
- [ ] **Step 319** [W] ~5m: `.github/workflows/ebpf.yml`: add a step after `check-ebpf-loads.sh` that runs `scripts/check-ebpf-size-pin.sh`; `docs/adr/ADR-089`: entry gate item 4 now cites the script and the pin file instead of the literal number.
- [x] **Step 320** [TEST][B] ~2m: The gate can fail (ADR-089 "gates that bite")
  ```bash
  cd /home/govan/tmp/unheaded && cp docs/policy/ebpf-size-pins.txt /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/pin.bak && sed -i 's/[0-9]\+$/1/' docs/policy/ebpf-size-pins.txt && bash scripts/check-ebpf-size-pin.sh >/dev/null 2>&1; echo "tampered exit=$?"; cp /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/pin.bak docs/policy/ebpf-size-pins.txt && git diff --quiet docs/policy/ebpf-size-pins.txt && echo restored
  ```
- [x] **Step 321** [V] ~1m: `tampered exit=1`, `restored`. Add the script to `scripts/check-gates-can-fail.sh`'s list.
- [x] **Step 322** [C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 313-321: Scorecard + CodeQL workflows; ADR-089 eBPF size pin is a gate`.
- [x] **Step 323** [W] ~10m: `.github/dependabot.yml`: add `package-ecosystem: docker` for `/`, `/deploy/sophia-eye`, `/docker` (weekly, grouped, target develop, limit 2) and `gomod` for `/` (weekly, grouped, limit 2; ADR-096's 7-day cooldown is enforced by `check-dependency-baseline.sh`/`check-tool-freshness.sh`, not by Dependabot).
- [x] **Step 324** [B] ~1m
  ```bash
  cd /home/govan/tmp/unheaded && python3 -c "import yaml; d=yaml.safe_load(open('.github/dependabot.yml')); print([u['package-ecosystem'] for u in d['updates']])"
  ```
- [x] **Step 325** [V] ~1m: `['github-actions', 'docker', 'docker', 'docker', 'gomod']` (order may differ).
- [x] **Step 326** [W][DOC-UPDATE] ~8m: `wiki/Releases.md`: how a release is cut (tag `vX.Y.Z-alpha`, what the workflow produces, how to verify with `scripts/verify-release.sh`, where provenance shows up); `wiki/Security-Tooling.md` rows for Scorecard, CodeQL, cosign, size pin; `SECURITY.md`: "verifying releases" paragraph linking the script.
- [x] **Step 327** [C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 323-326: dependabot docker+gomod; release and tooling docs`.
- [x] **Step 328** [V]: **PHASE 12 EXIT GATE**
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/check-actions-pinned.sh 2>&1 | tail -1 && bash scripts/check-ebpf-size-pin.sh 2>&1 | tail -1 && bash scripts/check-gates-can-fail.sh --quick 2>&1 | tail -2 && git status --short | wc -l
  ```
  - Pass: `[PASS]` x2, gates-can-fail includes the new script and passes, `0`. → merge `wt/iron-gate-release`; Phase 13.

---

## PHASE 13: THE BUILD KEY AND A SIGNED SEALED CASK (Steps 329-347)

**Goal**: The evidence build key has a custody procedure and a ceremony runbook; the Sealed Cask's Binding Rune is an ML-DSA-65 Gungnir seal with a Go verifier; tar extraction is hardened. (M-061; FND-018)
**Prerequisite**: Phase 12 exit gate.
**Time**: 3 h (+ E1)
**Agent**: Coordinator

- [ ] **Step 329** [R] ~3m
  ```bash
  cd /home/govan/tmp/unheaded && sed -n '40,120p' scripts/build-sealed-cask.sh | grep -nE 'sha256|MANIFEST|tar ' | head -10 && cat scripts/verify-binding-rune.sh | grep -nE 'sha256|tar|verify' | head -10 && grep -nE 'func main|case "' cmd/yggdrasil-evidence/main.go | head -12
  ```
- [ ] **Step 330** [W][CODE] ~15m: Tests `cmd/cask-verify/main_test.go` (new binary): given a cask tarball + `BINDING_RUNE.sha256` + `BINDING_RUNE.sig` (Gungnir detached evidence seal, purpose `sealed-cask/v1`) + `build.pubkey`, `verify` exits 0; a flipped byte in any listed file → 1; a missing `.sig` → 1 with `UNSIGNED CASK`; a tar member with `..` or an absolute path or a symlink escaping the root → 1 before extraction (hardening); the wrong public key → 1.
- [ ] **Step 331** [TEST][B] ~1m: Red
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/cask-verify/ -count=1 2>&1 | tail -2
  ```
- [ ] **Step 332** [W][CODE] ~35m: `cmd/cask-verify/main.go` using `pkg/gungnir.VerifyEvidenceDetached` and a safe tar reader (`archive/tar`, reject `..`, absolute, symlink/hardlink targets outside root, size cap per member 2 GiB, total cap 8 GiB); `cmd/cask-sign` (or a `sign` subcommand of `cmd/yggdrasil-evidence`, [DECIDE] RECOMMENDATION: a `cask` subcommand pair inside `cmd/yggdrasil-evidence` to keep one evidence binary; override ONLY if the cask must ship without yggdrasil tooling) that signs `BINDING_RUNE.sha256` with the build key seed (`gungnir.LoadSeedFile`, 0400 enforced).
- [ ] **Step 333** [TEST][B] ~2m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/yggdrasil-evidence/... ./cmd/cask-verify/... -race -count=1 2>&1 | tail -3
  ```
- [ ] **Step 334** [V] ~1m: `ok`.
- [ ] **Step 335** [W] ~10m: `scripts/build-sealed-cask.sh`: after the manifest, if `UNHEADED_BUILD_KEY` is set, `yggdrasil-evidence cask sign --key "$UNHEADED_BUILD_KEY" --manifest BINDING_RUNE.sha256 --out BINDING_RUNE.sig`; otherwise print `UNSIGNED CASK (set UNHEADED_BUILD_KEY)` and still build (dev). `scripts/verify-binding-rune.sh`: delegate to `cask-verify` (keep the sha256-only path behind `--insecure-sha-only` with a loud banner). `STAGING` dir: `mktemp -d`, not `/tmp/sealed-cask-<ts>` (another user could pre-create it).
- [ ] **Step 336** [TEST][B] ~4m: Round trip with a throwaway key
  ```bash
  cd /home/govan/tmp/unheaded && K=$(mktemp -d) && go run ./cmd/yggdrasil-evidence keygen --out $K/build.key --pubkey $K/build.pubkey 2>&1 | tail -1 && UNHEADED_BUILD_KEY=$K/build.key bash scripts/build-sealed-cask.sh --output $K/cask.tar.gz 2>&1 | tail -3 && go run ./cmd/cask-verify verify --cask $K/cask.tar.gz --pubkey $K/build.pubkey && echo VERIFIED && printf 'x' >> $K/cask.tar.gz && (go run ./cmd/cask-verify verify --cask $K/cask.tar.gz --pubkey $K/build.pubkey && echo BAD || echo "tamper refused"); rm -rf $K
  ```
- [ ] **Step 337** [V] ~1m: `VERIFIED` then `tamper refused`. (If `yggdrasil-evidence` has no `keygen`, use its `pubkey` derivation from a 32-byte seed: `head -c 32 /dev/urandom > $K/build.key; chmod 0400 $K/build.key; go run ./cmd/yggdrasil-evidence pubkey --key $K/build.key > $K/build.pubkey`.)
- [ ] **Step 338** [C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 329-337: signed Binding Rune, cask-verify with tar hardening`.
- [ ] **Step 339** [W] ~15m: Runbook `runbooks/security/build-key-ceremony.yaml`: generate the ML-DSA-65 seed on the custody host (E1), 0400 under a dedicated account `unheaded-buildkey` (sysusers entry + uid `BuildKey = 16737`), derive and publish `build.pubkey` (commit it to `compliance/keys/build.pubkey` in the repo: public, and the repo IS the trusted channel because commits are GPG-signed by Stevie; record the sha256 in `SECURITY.md`), quarterly rotation with overlap (both pubkeys valid one quarter), revocation = remove the pubkey + re-sign the current release; `status: draft`. `scripts/check-runbooks.sh` passes.
- [ ] **Step 340** [B] ~1m
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/check-runbooks.sh 2>&1 | tail -1 && go test ./pkg/uids/ -count=1 2>&1 | tail -1 && bash scripts/check-systemd-units.sh 2>&1 | tail -1
  ```
- [ ] **Step 341** [V] ~1m: `0 new finding(s)`, `ok`, `[PASS]`.
- [ ] **Step 342** [ESCALATE][AFTER-E1] ~0m: **E1, build-key custody.** Context: ceremony runbook, signer and verifier ready (Steps 329-341). Questions: (1) where the seed lives (west under `unheaded-buildkey` 0400 with auditd watch; a TPM-sealed file; an offline machine), (2) who may run `cask sign`, (3) publishing `build.pubkey` by signed commit as recommended. Why Stevie's: custody of the key that every release's trust reduces to. Steps 343-345 are [AFTER-E1].
- [ ] **Step 343** [AFTER-E1][LIVE-RW][S][BARE-METAL] ~10m: Run the ceremony on the custody host per the runbook; copy only `build.pubkey` back.
- [ ] **Step 344** [AFTER-E1][W] ~3m: `compliance/keys/build.pubkey` committed; `SECURITY.md` records its sha256 and the ceremony date; `release.yml` gains an optional step: if the repository secret `UNHEADED_BUILD_KEY_B64` exists, decode it to `$RUNNER_TEMP` 0400, sign the cask and the checksums with it in addition to keyless cosign ([DECIDE] RECOMMENDATION: do NOT put the build key in GitHub secrets; sign casks on the custody host and attach; keyless cosign covers GitHub-built artefacts. Override ONLY if Stevie accepts GitHub as a key holder).
- [ ] **Step 345** [AFTER-E1][TEST][B] ~5m: FND-018's verify: a real evidence pack signed with the real key verifies with the published pubkey (`nix/yggdrasil/scripts/yggdrasil-build-evidence-pack.sh` with `YGGDRASIL_BUILD_KEY` on the custody host; `yggdrasil-evidence verify --pubkey compliance/keys/build.pubkey <pack>`); record output in the progress file; register FND-018 closed.
- [ ] **Step 346** [C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 339-345: build-key ceremony runbook + uid; E1 escalated (or closed)`.
- [ ] **Step 347** [V]: **PHASE 13 EXIT GATE** — Steps 334, 337, 341 passed; tree clean. E1 open does not block. → Phase 14.

---

## PHASE 14: DEBS SIGNED, APT REPO ON WEST, EAST UPGRADED (Steps 348-383)

**Goal**: `build-all-debs.sh` names packages correctly and builds every unit's binary; reprepro on west serves a signed repo on 18888 behind the existing `nginx` sysuser; east installs with `apt`; the sophia-eye images are pinned; the CHANGELOG is true and `v0.2.0-alpha` is cut. (M-062, M-063 live half, M-064, M-067, M-041 overlap)
**Prerequisite**: Phases 2, 9 (units), 12 exit gates.
**Time**: 5 h (+ E3, E4)
**Agent**: Coordinator

### Packaging correctness

- [ ] **Step 348** [W][CODE] ~10m: `scripts/build-all-debs.sh`: `pkg_name_for()` mapping `unheaded-daemon → unheaded-daemon` (not `unheaded-unheaded-daemon`), everything else `unheaded-<svc>`; `SERVICES` += `unheaded-auth unheaded-keygen heimdall-daemon sword chaos-controller anomaly kev-poller trace-collector-go` (every binary a unit under `deploy/systemd` references, per `scripts/check-unit-binaries.sh`); each deb ships `/usr/lib/sysusers.d/unheaded-<svc>.conf` (one line extracted from `deploy/sysusers.d/unheaded.conf`) and the unit from `deploy/systemd/`; `debian/control` gets one stanza per package (generate it from the same list so the two cannot drift; a test asserts `grep -c '^Package:' debian/control` equals `${#SERVICES[@]}`).
- [ ] **Step 349** [TEST][B] ~6m
  ```bash
  cd /home/govan/tmp/unheaded && bash tests/deb/postinst_test.sh 2>&1 | tail -1 && DEB_OUTPUT_DIR=/tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/debs bash scripts/build-all-debs.sh 0.2.0~alpha+test 2>&1 | tail -4 && ls /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/debs/*.deb | sed 's#.*/##' | head -20
  ```
- [ ] **Step 350** [V] ~1m: `unheaded-daemon_0.2.0~alpha+test_amd64.deb` present, no `unheaded-unheaded-*`, one deb per `SERVICES` entry.
- [ ] **Step 351** [TEST][B] ~3m: Lintian-light: every deb has its unit, sysusers file and the keygen postinst hook
  ```bash
  cd /home/govan/tmp/unheaded && for d in /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/debs/*.deb; do n=$(basename $d); u=$(dpkg-deb -c $d | grep -c 'systemd/system/.*\.service$'); s=$(dpkg-deb -c $d | grep -c 'sysusers.d/'); k=$(dpkg-deb -I $d postinst 2>/dev/null | grep -c keygen); printf '%-50s unit=%s sysusers=%s keygen=%s\n' $n $u $s $k; done | awk '$2!="unit=1"||$3!="sysusers=1"||$4!="keygen=1"{bad++} END{print "bad:", bad+0}'
  ```
- [ ] **Step 352** [V] ~1m: `bad: 0` (`unheaded-keygen` itself has no unit: exempt it in the awk or ship it inside each deb and drop the standalone package; [DECIDE] RECOMMENDATION: ship keygen inside every service deb at `/usr/local/bin/unheaded-keygen` (dpkg handles the shared file via `Replaces`/identical content? no: identical files in multiple debs conflict). Simpler: one `unheaded-common` deb with keygen + sysusers + trust.d dir that every service deb `Depends:` on. Do that.)
- [ ] **Step 353** [C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 348-352: deb names fixed, unheaded-common, every unit's binary packaged`.

### Signing and the repo

- [ ] **Step 354** [W] ~15m: `scripts/apt-repo.sh` (new): `init` creates `/srv/unheaded-apt/{conf,incoming}` with `conf/distributions` (`Codename: unheaded`, `Suite: stable`, `Architectures: amd64`, `Components: main`, `SignWith: <keyid>`), `include <deb...>` runs `reprepro -b /srv/unheaded-apt includedeb unheaded <debs>`, `serve` prints the nginx server block for 18888 (root `/srv/unheaded-apt`, autoindex off, `location /conf { deny all; }`, `location /db { deny all; }`), runs as `nginx` (sysuser exists). `deploy/nginx/unheaded-apt.conf` holds that block. A `tests/apt/apt_repo_test.sh` runs `init` + `include` against a temp dir with a throwaway GPG key (`gpg --batch --quick-gen-key` under a temp `GNUPGHOME`, as `tests/runbooks/test_branch_promotion.py` does) and checks `dists/unheaded/Release.gpg` exists and `apt-ftparchive`/`reprepro check` passes.
- [ ] **Step 355** [B] ~3m
  ```bash
  cd /home/govan/tmp/unheaded && which reprepro >/dev/null || echo "reprepro missing: sudo apt-get install -y reprepro (dev box; E3 for hosts)"; bash tests/apt/apt_repo_test.sh 2>&1 | tail -3
  ```
- [ ] **Step 356** [V] ~1m: PASS (install reprepro locally if missing; it is a build-box tool, not a host change).
- [ ] **Step 357** [W] ~10m: `scripts/build-all-debs.sh`: optional `DEB_SIGN_KEY=<keyid>` runs `dpkg-sig --sign builder -k $DEB_SIGN_KEY` (or `debsigs`) on each deb; the repo's `Release` signature (reprepro `SignWith`) is the primary trust anchor, per-deb signatures are belt-and-braces. `wiki/Releases.md` "apt repo" section: trust key install on a client (`/etc/apt/keyrings/unheaded.gpg` + `signed-by=` in `sources.list.d/unheaded.list`), never `apt-key`.
- [ ] **Step 358** [W] ~8m: Runbook `runbooks/deploy/apt-repo-publish.yaml`: build debs on west from a tagged commit, `apt-repo.sh include`, verify `curl http://west:18888/dists/unheaded/Release` and signature, then on east `apt update && apt install unheaded-*` with the pinned version; rollback = `apt install unheaded-<svc>=<previous>`. `status: draft`.
- [ ] **Step 359** [B] ~1m
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/check-runbooks.sh 2>&1 | tail -1 && shellcheck -S warning scripts/apt-repo.sh scripts/build-all-debs.sh 2>&1 | tail -2
  ```
- [ ] **Step 360** [V] ~1m: `0 new finding(s)`, no shellcheck warnings.
- [ ] **Step 361** [C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 354-360: apt-repo.sh, reprepro test, deb signing option, publish runbook`.
- [ ] **Step 362** [W] ~10m: `release.yml`: after debs are built, upload them as release assets; a separate manual-dispatch workflow `.github/workflows/apt-publish.yml` is NOT added (the repo lives on west, not GitHub; publishing is the runbook's job). `docs/adr/ADR-026`: status note "implemented S-IRON-GATE; repo on west, GHA builds, runbook publishes"; `docs/adr/ADR-085`: Phase 3 decided GHA (Step 11).
- [ ] **Step 363** [W] ~10m: Pin the sophia-eye images (M-067): `deploy/sophia-eye/docker-compose.yml` `vllm/vllm-openai:latest-rocm` → the exact tag currently running on west (`docker image inspect` on west is [LIVE-RO]: `ssh west docker images --digests | grep vllm` or, if west is this box, `docker images --digests | grep -E 'vllm|qdrant|open-webui|text-embeddings'`), `qdrant/qdrant:latest` → exact version, `open-webui:main` → exact version, `text-embeddings-inference:cpu-1.5` → `cpu-1.5.x` exact; then `scripts/check-image-tags.sh --update` to shrink the baseline.
- [ ] **Step 364** [B] ~1m
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/check-image-tags.sh 2>&1 | tail -1 && grep -c 'deploy/sophia-eye' docs/policy/image-tag-baseline.txt
  ```
- [ ] **Step 365** [V] ~1m: `[PASS]`, `0`.
- [ ] **Step 366** [ESCALATE][AFTER-E4] ~0m: **E4, apt-repo signing key.** Context: reprepro signs `Release` with a GPG key; the key must live on west under the `nginx`? no, under a dedicated `unheaded-apt` account (sysusers + uid 16738 `AptRepo`) with `GNUPGHOME=/var/lib/unheaded/apt/gnupg` 0700. Questions: (1) a new dedicated key (recommended: `unheaded-apt <stevie@bellis.tech>`, ed25519, 2-year expiry) or Stevie's existing signing key? (2) who may run `apt-repo.sh include`. Why Stevie's: a signing key under ADR-004's "approved exceptions" wording in ADR-026. Steps 367-368 are [AFTER-E4]; Steps 369-383 also need E3.
- [ ] **Step 367** [AFTER-E4][W] ~3m: sysusers `unheaded-apt` + `pkg/uids` `AptRepo = 16738`; `apt-repo.sh` runs reprepro as that user via `runuser -u unheaded-apt --` when invoked as root.
- [ ] **Step 368** [AFTER-E4][C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 362-367: ADR-026/085 notes, sophia-eye pins, apt-repo account`.

### Live on the hosts

- [ ] **Step 369** [AFTER-E3][AFTER-E4][LIVE-RW][S][BARE-METAL] ~10m: On west: `sudo apt-get install -y reprepro`; `sudo install -m 0644 deploy/sysusers.d/unheaded.conf /etc/sysusers.d/ && sudo systemd-sysusers`; `sudo bash scripts/apt-repo.sh init --keyid <E4 key>`; `sudo install -m 0644 deploy/nginx/unheaded-apt.conf /etc/nginx/sites-enabled/`; `sudo nginx -t && sudo systemctl reload nginx`.
- [ ] **Step 370** [LIVE-RO][V] ~1m
  ```bash
  curl -s -o /dev/null -w 'release=%{http_code}\n' http://127.0.0.1:18888/dists/unheaded/Release; curl -s -o /dev/null -w 'conf_denied=%{http_code}\n' http://127.0.0.1:18888/conf/distributions
  ```
  Expected: `release=404` before the first include (or 200 after), `conf_denied=403`.
- [ ] **Step 371** [AFTER-E3][B] ~8m: Build the debs from the sprint's tagged commit on west
  ```bash
  cd /home/govan/tmp/unheaded && git status --porcelain | wc -l && DEB_SIGN_KEY=<E4 keyid> bash scripts/build-all-debs.sh 2>&1 | tail -3 && ls dist/debs/*.deb | wc -l
  ```
- [ ] **Step 372** [AFTER-E3][AFTER-E4][LIVE-RW][S] ~3m: Publish
  ```bash
  cd /home/govan/tmp/unheaded && sudo bash scripts/apt-repo.sh include dist/debs/*.deb 2>&1 | tail -3 && curl -s http://127.0.0.1:18888/dists/unheaded/Release | head -8 && curl -s http://127.0.0.1:18888/dists/unheaded/Release.gpg | head -c 60; echo
  ```
- [ ] **Step 373** [V] ~1m: `Release` lists `Codename: unheaded`, `Release.gpg` is a PGP block.
- [ ] **Step 374** [AFTER-E3][LIVE-RW][S][BARE-METAL] ~8m: On east: trust key + source + install (the P2P address of west is 192.168.13.2 per CLAUDE.md)
  ```bash
  ssh east 'set -e; curl -fsS http://192.168.13.2:18888/unheaded-apt.gpg | sudo tee /etc/apt/keyrings/unheaded.gpg >/dev/null; echo "deb [signed-by=/etc/apt/keyrings/unheaded.gpg] http://192.168.13.2:18888 unheaded main" | sudo tee /etc/apt/sources.list.d/unheaded.list; sudo apt-get update 2>&1 | tail -2; apt-cache policy unheaded-wotan | head -4'
  ```
  (`apt-repo.sh init` exports the public key to `/srv/unheaded-apt/unheaded-apt.gpg`.)
- [ ] **Step 375** [V] ~1m: `apt-cache policy` shows the candidate from `192.168.13.2:18888`.
- [ ] **Step 376** [AFTER-E3][LIVE-RW][S][BARE-METAL] ~15m: Upgrade east's 9 debs + install the new ones (chaos-controller, anomaly, heimdall units stay disabled until their escalations), restart units, install TLS material and keys per `deploy/systemd/README.md` (Phase 8 Step 268 and Phase 2 Step 89), wotan.yaml with `auto_approve` names + `chaos-controller` (M-041)
  ```bash
  ssh east 'sudo apt-get install -y unheaded-common unheaded-wotan unheaded-daemon unheaded-dashboard-backend unheaded-kanban-app unheaded-akira unheaded-timeguru unheaded-captain unheaded-architect unheaded-micromanager unheaded-auth 2>&1 | tail -3; systemctl list-units "unheaded-*" --no-pager | head -20'
  ```
- [ ] **Step 377** [LIVE-RO][V] ~3m: Every installed unit `active (running)`; `ssh east 'curl -sk https://127.0.0.1:18000/health; journalctl -u unheaded-wotan --since -5m | grep -ciE "unauthenticated|x509"'` → health 200, `0`.
- [ ] **Step 378** [LIVE-RO][TEST][B] ~2m: FND-017 verify on a host
  ```bash
  ssh east 'bash -s' < scripts/probe-tls-groups.sh 127.0.0.1:18001 2>&1 | tail -3
  ```
- [ ] **Step 379** [V] ~1m: hybrid negotiated, classical refused. Register FND-017 closed with this line.
- [ ] **Step 380** [AFTER-E3][LIVE-RW][S][BARE-METAL] ~15m: Same install on west (`sudo apt-get install` from its own repo), then huginn baseline update (M-041: `huginn` new binary name + baseline `12c6a543c995` per next.md; follow `wiki/Huginn.md` or the unit's README).
- [ ] **Step 381** [LIVE-RO][V] ~2m: `systemctl list-units "unheaded-*"` on west all running; `bash scripts/check-unit-binaries.sh` on the repo; `curl -sk https://127.0.0.1:19020/.well-known/openid-configuration | head -c 100` answers on west.
- [ ] **Step 382** [C]: **COMMIT CHECKPOINT** `[PLAN S-IRON-GATE] Steps 369-381: apt repo live on west, east installed from it over TLS; FND-017 closed on hosts`.
- [ ] **Step 383** [V]: **PHASE 14 EXIT GATE** — Steps 350, 352, 356, 360, 365 passed (code half); if E3/E4 answered: 370, 373, 375, 377, 379, 381 passed (live half) and `docs/LIVE-PATHS.md` gains the apt repo and the auth issuer. Otherwise the live half is recorded as [AFTER-E3]/[AFTER-E4] in the progress file and the gate passes on the code half. → Phase 15.

---

## PHASE 15: SECURITY AND COMPLIANCE REVIEW (Steps 384-392)

**Goal**: Every new listener, key file, header and workflow is in the attack-surface table; every gate that can see this sprint's changes is green; no new dependency slipped in. (DoD-STD 5, 6, 8 for the whole sprint)
**Prerequisite**: Phases 1-14 exit gates (live halves may be [AFTER-E*]).
**Time**: 2 h
**Agent**: Coordinator

- [ ] **Step 384** [SECURITY][W] ~25m: `docs/security/iron-gate-attack-surface.md`: table of every change with columns `what | listener/file | uid | mode | reachable from | auth | TLS | fuzzed by`. Rows (at least): `unheaded-auth :19020` (16735, control network only, login rate-limited + lockout, TLS Strict, FuzzLoginBody/FuzzUsersFile); `/etc/unheaded/keys/<svc>.key` x15 (0400, owner uid, never mounted elsewhere); `/etc/unheaded/trust.d` (0755/0444, public); `/etc/unheaded/auth/users.json` (0600 unheaded-auth, argon2id); `/etc/unheaded/tls/<svc>/` (key 0640 root:svc); Wotan gRPC/HTTP now token-gated; dashboard/kanban `/auth/*` routes + session cookie (HttpOnly/Secure/SameSite=Strict, `X-Requested-With`); `api_keys` table (hash only, `app_auth` role); gitleaks `uhk_` rule; `release.yml` `id-token: write` (OIDC for keyless cosign: what a compromised workflow could sign); Scorecard/CodeQL tokens; reprepro on `:18888` (nginx, GET only, `/conf` and `/db` denied; signing key under `unheaded-apt` 0700); `unheaded-common` deb postinst runs keygen as root (argv fixed, no user input); Heimdall unit `CAP_DAC_READ_SEARCH`; security headers on every router. Then a "Residual risk" list: the dev-certs CA (configs/certs/dev) is a lab CA; the issuer has no MFA (Step 125); API keys expire in 90 d by default; session store is in-memory (restart logs everyone out; acceptable).
- [ ] **Step 385** [SECURITY][TEST][B] ~8m: Re-run every fuzz target this sprint added, 90 s each
  ```bash
  cd /home/govan/tmp/unheaded && for t in 'pkg/auth FuzzEdDSAParse' 'pkg/auth FuzzJWKSDocument' 'pkg/auth FuzzTrustDirFilenames' 'cmd/unheaded-auth FuzzLoginBody' 'cmd/unheaded-auth FuzzUsersFile'; do set -- $t; go test ./$1/ -run '^$' -fuzz="^$2\$" -fuzztime=90s 2>&1 | tail -1; done; git status --short | grep -c testdata/fuzz
  ```
- [ ] **Step 386** [V] ~1m: five `ok`/`PASS`, `0` new crashers.
- [ ] **Step 387** [COMPLIANCE][B] ~6m: Dependency and license gates (no new Go module is expected: argon2 is x/crypto, Ed25519 is stdlib)
  ```bash
  cd /home/govan/tmp/unheaded && go mod tidy -diff && echo TIDY_CLEAN; bash scripts/check-dependency-baseline.sh 2>&1 | tail -1; echo "SPDX missing (Go, as ci.yml spdx-check runs it): $(find . -name '*.go' -not -path './.git/*' -not -path './vendor/*' -exec grep -L 'SPDX-License-Identifier' {} + 2>/dev/null | wc -l)"; echo "SPDX missing (new .rs/.sh/.sql this sprint): $(git diff --name-only a7fe3acd -- '*.rs' '*.sh' '*.sql' '*.py' | xargs -r grep -L 'SPDX-License-Identifier' 2>/dev/null | wc -l)"; bash scripts/check-tool-freshness.sh 2>&1 | grep -E 'STALE|cosign' | head -3; go run golang.org/x/vuln/cmd/govulncheck@latest ./... 2>&1 | tail -3
  ```
- [ ] **Step 388** [V] ~1m: `TIDY_CLEAN`, baseline `[PASS]` (shrink-only still holds), both `SPDX missing` counts `0` (every file this sprint adds carries a header; the repo-wide `.rs` backfill is S-TRUE-WIRE M-160), cosign pin `[ok]`, govulncheck `No vulnerabilities found`. A new module anywhere → STOP: ADR-004 row + Barrister note before the gate passes.
- [ ] **Step 389** [SECURITY][B] ~6m: Every repo gate that sees this sprint
  ```bash
  cd /home/govan/tmp/unheaded && for g in check-gitleaks check-secrets-baseline check-gosec-ratchet check-systemd-units check-unit-binaries check-compose-hardening check-compose-bind-nesting check-compose-log-caps check-image-tags check-actions-pinned check-crypto-inventory check-pq-kex check-well-migrations check-well-lint check-runbooks check-js-lint check-ebpf-size-pin check-manifest-yaml check-hadolint; do printf '%-28s ' $g; bash scripts/$g.sh >/tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/g.log 2>&1 && echo PASS || { echo FAIL; tail -3 /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/g.log; }; done 2>/dev/null; printf '%-28s ' shellcheck-new-scripts; shellcheck -S warning scripts/apt-repo.sh scripts/gen-dev-keys.sh scripts/verify-release.sh scripts/check-ebpf-size-pin.sh scripts/lib/deb-maintscripts.sh && echo PASS || echo FAIL; bash scripts/compliance/check-service-auth.sh 2>&1 | tail -1
  ```
- [ ] **Step 390** [V] ~2m: Every line `PASS` (a script that does not exist prints FAIL with "No such file": drop it from the list, it was a guess); `check-service-auth` `[PASS]`. gosec: new `#nosec` only with a justification that leads the comment (memory: reference_gosec_nosec_directive).
- [ ] **Step 391** [SECURITY][W] ~8m: Lich hand-off `tomb/lich/IRON-GATE-HOOKS.md`: targets for a future campaign (token confusion `alg`/`kid`, JWKS poisoning via discovery, session fixation on the BFF, API-key timing, reprepro path traversal, cosign bundle substitution), with the fuzz seeds under `tomb/lich/seeds/auth/` (10 malformed tokens, 5 JWKS docs). Register nothing new in ADR-098 unless Step 389 found something; if it did, one row per finding with the remediation step number.
- [ ] **Step 392** [V]: **PHASE 15 EXIT GATE** — Steps 386, 388, 390 passed; attack-surface doc and Lich hooks committed: `[PLAN S-IRON-GATE] Steps 384-391: attack surface, fuzz re-run, all gates green, Lich hooks`. → Phase 16.

---

## PHASE 16: DOCS, REGISTERS, HANDOFF (Steps 393-402)

**Goal**: ADR statuses, findings register, wiki, CLAUDE.md, timeline and next.md tell the truth about what shipped and what waits on E1-E5. (DoD-STD 1, 4, 7; master-list AUDIT-UPDATE)
**Prerequisite**: Phase 15 exit gate.
**Time**: 2 h
**Agent**: Coordinator (Librarian lens: ripple every layer)

- [ ] **Step 393** [DOC-UPDATE][W] ~15m: ADR statuses: ADR-103 `**Status:** Implemented (S-IRON-GATE 2026-10-xx: pkg/auth EdDSA, unheaded-keygen, unheaded-auth, BFF, api_keys, Wotan identity); Q1-Q4 decided per plan Steps 7-10 (recommendations); Stevie to accept`; ADR-034 Phases 7/8 ticked with Step 294's numbers; ADR-026 and ADR-085 notes (Step 362); ADR-105 "Still classical until E1-E5" paragraph: E5 Heimdall keys now have a unit + runbook, custody still Stevie's (E2 here); `docs/adr/ADR-INDEX.md` rows updated. No ADR is marked Accepted by this plan (Stevie accepts: master decision 29).
- [ ] **Step 394** [AUDIT-UPDATE][W] ~10m: `compliance/findings/register.yaml`: FND-017 closed (compose: Step 265; hosts: Step 379 or [AFTER-E3]); FND-018 remediation progress "signer/verifier/ceremony ready, custody E1 open" (closed only if Step 345 ran); FND-019 "unit + keygen + runbook ready, E2 open"; the `check-service-auth` finding closed (Step 248). Then regenerate:
  ```bash
  cd /home/govan/tmp/unheaded && go run ./cmd/compliance-evidence findings -adr docs/adr/ADR-098-findings-register-and-baseline-modes.md 2>&1 | tail -1 && git diff --stat docs/adr/ADR-098-findings-register-and-baseline-modes.md | tail -1
  ```
- [ ] **Step 395** [DOC-UPDATE][W] ~20m: Wiki ripple (every page this sprint touched or that lists what it changed): `Service-Auth.md` (final shape), `Service-Wotan.md` (token metadata, auto-approve by `sub`, `signed_prefixes` three new), `Releases.md`, `Security-Tooling.md`, `Port-Registry.md` (19020 auth, 18888 apt), the uid table page (16735-16738), `Containers.md` (compose now needs `scripts/gen-dev-keys.sh` + `gen-dev-certs.sh` before `up`: add to the bring-up recipe), `Huginn.md`/`Mimirs-Law.md` (Heimdall unit + E2), `Alerting.md` (`alerts.critical.` signed), `_Sidebar.md` entries for the new pages, `Home.md` Recent Progress one bullet. Ops facts (key paths, cert paths, repo path `/srv/unheaded-apt`, port 18888) go in the wiki, not only docs/ (memory: feedback_ops_facts_in_wiki).
- [ ] **Step 396** [ESCALATE] ~0m: **E5, ADR-004 admissions (record only; nothing in this plan depends on it).** Context: `aws-lc-rs`/`aws-lc-sys` (WAF, zhend; S-QUANTUM-ARMOR) and `anthropic-sdk-go` (ADR-104; S-TWO-BRAINS) are in use without an ADR-004 row. Question: sign off both, with dates, in `docs/adr/ADR-004`. Why Stevie's: dependency admissions are the owner's per ADR-004/096. Write the two draft rows into ADR-004 marked `pending Stevie's sign-off` so the paperwork is one word away.
- [ ] **Step 397** [DOC-UPDATE][W] ~10m: `CLAUDE.md`: "Authentication Framework" section rewritten to the ADR-103 shape (env names, keygen, issuer, BFF; delete the Noop/HS256 "production uses APIKey or JWT" text); Port Registry table rows for `unheaded-auth 19020` and `apt repo 18888`; the Secrets Management paragraph gains one line: service keys are files under `/etc/unheaded/keys`, never env. Keep every edit to facts; no doctrine lines.
- [ ] **Step 398** [DOC-UPDATE][W][REGEN] ~10m: `references/timeline.md`: one dated entry under the current Age ("S-IRON-GATE shipped <date>: service auth (ADR-103), TLS on every internal link (FND-017), signed releases, apt repo on west; E1-E5 open") and update the `LAST UPDATED:` line; regenerate the mirrors the way the 2026-10-26 sync did (`git show --stat f83c9cde` lists `references/timeline.{json,yaml,toml}`; the generator is timeguru's syncer: `grep -rn 'SYNC_DIR\|sync-once\|-sync' services/timeguru/cmd/timeguru/main.go | head -3` to get the exact flag, then run it with `--timeline references/timeline.md`), and `bash scripts/check-timeline-freshness.sh`.
- [ ] **Step 399** [AUDIT-UPDATE][W] ~8m: `docs/sweeps/OUTSTANDING-2026-10-27.md`: mark M-050, M-051, M-052, M-053, M-054, M-055, M-056, M-060, M-062, M-063, M-064, M-065, M-066, M-067 with `→ S-IRON-GATE Steps …` and status DONE / DONE-code-half-[AFTER-E3]; M-061 and M-201 "ready, E1/E2 open"; the index `docs/battle-plans/NEXT-7-SPRINTS-2026-10-27.md` row for sprint 2 gets its status cell.
- [ ] **Step 400** [TEST][B] ~12m: Final regression against the baseline
  ```bash
  cd /home/govan/tmp/unheaded && go build ./... && go test ./... 2>&1 | grep -E '^(FAIL|---)' > /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/final.txt; echo "FAIL now: $(grep -c '^FAIL' /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/final.txt) baseline: $(grep -c '^FAIL' docs/battle-plans/progress/S-IRON-GATE-baseline.txt)"; (cd ebpf && cargo test -p monad-common 2>&1 | tail -1); bash scripts/check-timeline-freshness.sh 2>&1 | grep Status
  ```
- [ ] **Step 401** [DOC-UPDATE][W] ~10m: `~/tmp/next.md` handoff (overwrite the START HERE block): what shipped, the five escalations with their exact questions and the step numbers that resume when answered, how to bring the compose stack up now (keys + certs scripts first), first CI runs to check after Stevie pushes (Scorecard, CodeQL, release.yml on the first `v0.2.0-alpha` tag), and the SHIPPED banner text for the top of this plan. Also write the SHIPPED banner into this file (the PHASE24/S-REAL-STEEL style: one blockquote with the headline numbers from Steps 246, 265, 294, 302, 350, 373/379 and the STUCK list).
- [ ] **Step 402** [C][V]: **PHASE 16 EXIT GATE AND FINAL COMMIT**
  ```bash
  cd /home/govan/tmp/unheaded && git add -A && git commit --no-gpg-sign -q -m "[PLAN S-IRON-GATE] Steps 393-402: ADR statuses, findings register, wiki, CLAUDE.md, timeline, handoff; sprint shipped

Phase 16: docs, registers, handoff
Open escalations: E1 build key, E2 drift keys, E3 host windows, E4 apt key, E5 ADR-004 rows

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01TzL3TuPhKAVVHnD9jzuabE" && git status --short | wc -l && git log --oneline -1
  ```
  - Pass: FAIL count equals baseline (Step 400), freshness `FRESH`, tree `0`, commit present. Never push: Stevie pushes, then re-signs with `scripts/resign-unpushed.py` if Step 6 said `SIGN=no`.
  - Fail → the sprint is not shipped; fix within Phase 16.

---

## APPENDIX A: EMERGENCY PROCEDURES

**A1. A service refuses every caller after `AUTH_ENABLED=true` (all 401/403)**
1. `docker compose logs --tail=50 <svc> | grep -iE 'auth|kid|caller'`: `unknown key` means the caller's `.pub` is not in the trust dir mounted into `<svc>`; `caller not allowed` means `AUTH_ALLOW_CALLERS` lacks the caller's `sub`.
2. `docker compose exec <svc> ls /etc/unheaded/trust.d` and compare kids with `unheaded-keygen -print-kid -dir configs/keys/dev <caller>`.
3. Fix the allowlist or re-run `scripts/gen-dev-keys.sh` (it never overwrites an existing key without `-rotate`); `docker compose up -d <svc>`.
4. Still failing after 2 rounds → set `AUTH_ENABLED` unset for that one service, commit the STUCK marker, continue; never set `AUTH_MODE=hs256-test` in compose.

**A2. TLS handshake failures (`x509: certificate is valid for X, not Y`)**
1. `openssl x509 -in configs/certs/dev/<svc>/<svc>.pem -noout -ext subjectAltName`.
2. The SAN must include the compose service name the caller dials (`wotan`, `auth`, …) and `localhost`/`127.0.0.1` for host curls. Fix the SAN list in `scripts/gen-dev-certs.sh`, regenerate, `docker compose up -d`.
3. `unknown authority` → the caller lacks `UNHEADED_TLS_CA_CERT` or mounts a different CA dir. One CA per deployment; `gen-dev-certs.sh` copies it into each service dir.

**A3. Wotan refuses everything after Phase 7 (`Unauthenticated` from every service)**
1. `docker compose logs wotan | grep -iE 'auth|token' | head`: `aud mismatch` means callers mint `aud=wotan-grpc` or similar; the audience is the service name `wotan`.
2. `configs/wotan.yaml` `auto_approve` must list every caller's `sub` (service names); the old catch-all `service` is gone on purpose.
3. If the stack must come back up fast: `AUTH_ENABLED` unset on `wotan` only, STUCK marker, continue Phase 8 with Wotan open; return after.

**A4. Lost the dev issuer password**
1. `scripts/gen-dev-keys.sh --reset-users` rewrites `configs/auth/dev-users.json` and prints a new password once. Restart `auth`. Sessions are invalidated (in-memory store).

**A5. Keyless cosign fails in `release.yml`**
1. The job needs `permissions: id-token: write` (OIDC) and `contents: write`; a fork PR cannot get `id-token`. Tags only (`on: push: tags`), so this is not a PR problem.
2. `cosign sign-blob --yes` must be given `--bundle`; without `--yes` it waits for a TTY and the job hangs until timeout.
3. Verify locally with `scripts/verify-release.sh <tag>`; if the identity regexp fails, print the certificate identity from the bundle (`jq -r '.verificationMaterial.certificate' | base64 -d | openssl x509 -noout -text | grep URI`) and fix the regexp.

**A6. reprepro refuses to sign (`gpg: signing failed`)**
1. Run as the `unheaded-apt` account with `GNUPGHOME=/var/lib/unheaded/apt/gnupg` (0700). `gpg --list-secret-keys` there must show the E4 key.
2. `conf/distributions` `SignWith:` must be the key id or fingerprint, not an email.
3. No agent/TTY on a service account: use a key with no passphrase (it is protected by the account and mode, E4 decision) or `--pinentry-mode loopback` with a passphrase file 0400.

**A7. east `apt update` says `NO_PUBKEY`**
1. `/etc/apt/keyrings/unheaded.gpg` must be the binary (dearmored) key: `gpg --dearmor < unheaded-apt.asc > unheaded.gpg` if `apt-repo.sh init` exported ASCII.
2. The `signed-by=` path in `sources.list.d/unheaded.list` must match. Never `apt-key add`.

**A8. CBOM gate red after new crypto**
1. `go run ./cmd/crypto-inventory scan --out compliance/crypto/cbom.cdx.json` and commit. The gate compares the tree to the checked-in inventory; every new algorithm use (Ed25519, argon2id) must be scanned in.

**A9. Worktree merge conflicts in `docker-compose.yml`**
1. Phases 2, 4, 5, 6 all edit compose. Merge order: keygen → issuer → apikeys → release; resolve by keeping every service's `secrets:`/`environment:` additions (they are disjoint), then `docker compose config --quiet`.

**A10. A unit fails `check-systemd-units.sh` with `User=… has no line in sysusers`**
1. Every new `User=` needs a line in `deploy/sysusers.d/unheaded.conf` and (for a fixed uid) a constant in `pkg/uids`. This sprint adds unheaded-auth 16735, unheaded-heimdall 16736, unheaded-buildkey 16737, unheaded-apt 16738.

---

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Phase | Steps | Agent | Parallel with | Depends on | Time |
|---|---|---|---|---|---|
| 0 Preflight | 1-28 | Coordinator | — | S-CLEAN-SLATE | 45 m |
| 1 pkg/auth core | 29-66 | Agent (auth-core) | 3 | 0 | 4 h |
| 2 keygen + packaging | 67-96 | Agent (keygen) | 4 | 1 | 3 h |
| 3 API keys | 97-124 | Agent (apikeys) | 1 | 0 | 3 h |
| 4 Issuer | 125-158 | Agent (issuer) | 2 | 1 | 5 h |
| 5 Cuirass first | 159-178 | Coordinator | — | 1, 2, 3, 4 merged | 2.5 h |
| 6 Every caller + BFF | 179-222 | Coordinator (BFF) + Agent (wave) | — | 5 | 7 h |
| 7 Wotan last | 223-250 | Coordinator | — | 6 | 4 h |
| 8 TLS everywhere | 251-269 | Coordinator | — | 7 | 4 h (+E3) |
| 9 Signed prefixes + Heimdall | 270-285 | Coordinator | 10, 11, 12 | 8 | 3 h (+E2) |
| 10 Akira cert check | 286-295 | Agent | 9, 11, 12 | 8 | 2 h |
| 11 Security headers | 296-303 | Agent | 9, 10, 12 | 0 | 1.5 h |
| 12 Release signed | 304-328 | Agent (release) | 9, 10, 11 | 0 | 4 h |
| 13 Build key + cask | 329-347 | Coordinator | — | 12 | 3 h (+E1) |
| 14 Debs + apt + hosts | 348-383 | Coordinator | — | 2, 9, 12 | 5 h (+E3, E4) |
| 15 Security review | 384-392 | Coordinator | — | 1-14 | 2 h |
| 16 Docs + handoff | 393-402 | Coordinator | — | 15 | 2 h |

**Critical path**: 0 → 1 → 2 → 4 → 5 → 6 → 7 → 8 → 9 → 14 → 15 → 16 (≈ 38 h of execution; Phases 3, 10, 11, 12, 13 hang off it in parallel).

**Dependency map for escalations**
- E1 (build-key custody) blocks Steps 343-345 only. FND-018 stays open until then.
- E2 (drift-key custody, Heimdall on hosts) blocks the live half of M-201 / FND-019 only (after Step 283).
- E3 (host windows) blocks Step 268's host install and Steps 369-381. Every code half ships without it.
- E4 (apt signing key) blocks Steps 367-376. The repo tooling and tests ship without it.
- E5 (ADR-004 rows) blocks nothing; recorded at Step 396.

**STUCK policy per chain**: Phases 5-7 are the only chain where a skip cascades (a service that cannot be gated blocks Wotan's allowlist being complete). Skip that one service with a STUCK marker and keep `AUTH_ENABLED` unset for it; Phase 7 still proceeds with the others.

---

## APPENDIX C: QUICK REFERENCE

**New identities**

| Thing | uid | port | account | key file |
|---|---|---|---|---|
| unheaded-auth (issuer) | 16735 | 19020 | unheaded-auth | /etc/unheaded/keys/unheaded-auth.key |
| heimdall-daemon | 16736 | — | unheaded-heimdall | /etc/unheaded/keys/heimdall-drift.key (ML-DSA-65) |
| build key custody | 16737 | — | unheaded-buildkey | per E1 |
| apt repo signer | 16738 | 18888 (nginx) | unheaded-apt | GNUPGHOME=/var/lib/unheaded/apt/gnupg |

**Env names (pkg/auth, pkg/tls)**: `AUTH_ENABLED`, `AUTH_MODE` (`eddsa` | `hs256-test`), `AUTH_KEY_FILE`, `AUTH_TRUST_DIR`, `AUTH_ALLOW_CALLERS`, `AUTH_OIDC_ISSUER_URL` / `AUTH_ISSUER_URL` (BFF), `AUTH_API_KEY_FILE`, `AUTH_APIKEY_STORE` (`well` | `file:<path>`), `UNHEADED_TLS_ENABLED`, `UNHEADED_TLS_CA_CERT`, `UNHEADED_TLS_CERT`, `UNHEADED_TLS_KEY_FILE`, `UNHEADED_TLS_MTLS_ENABLED`. Forbidden with `AUTH_ENABLED=true`: `AUTH_JWT_KEY`.

**Paths**: keys `/etc/unheaded/keys/<svc>.key` (0400 svc), trust `/etc/unheaded/trust.d/<svc>.pub` (0444), TLS `/etc/unheaded/tls/<svc>/{<svc>.pem,<svc>-key.pem,ca.pem}`, users `/etc/unheaded/auth/users.json` (0600), apt repo `/srv/unheaded-apt`, dev material `configs/{keys,trust.d,certs,auth}/dev` (gitignored).

**Commands**
```bash
scripts/gen-dev-keys.sh                 # dev service keys + trust.d + dev user
CERT_DIR=configs/certs/dev scripts/gen-dev-certs.sh
unheaded-keygen -owner unheaded-<svc> <svc>        # host install, as root
unheaded-keygen -algo mldsa65 -owner unheaded-heimdall heimdall-drift
unheaded-auth user add <name> --roles operator
unheaded-cli apikey create --owner <o> --role operator --expires 90d
scripts/compliance/check-service-auth.sh           # the sprint's headline gate
scripts/probe-tls-groups.sh 127.0.0.1:18001        # FND-017 verify
scripts/verify-release.sh v0.2.0-alpha             # cosign + checksums
scripts/apt-repo.sh init|include|serve
scripts/check-ebpf-size-pin.sh [--update]          # ADR-089 item 4
go run ./cmd/compliance-evidence findings -adr docs/adr/ADR-098-findings-register-and-baseline-modes.md
```

**Token shape**: header `{"alg":"EdDSA","typ":"JWT","kid":"<16 hex>"}`; claims `iss=unheaded`, `sub=<caller>`, `aud=<target service>`, `iat`, `exp=iat+300` (service) / `+900` (user), `roles` (user), `act` (on-behalf-of, BFF). Verifier: `kid` in trust.d AND `sub` in `AUTH_ALLOW_CALLERS` (403 otherwise).

**Wotan**: `configs/wotan.yaml` `topics.auto_approve` = verified `sub`s; `signed_prefixes`: `drift.`, `audit.`, `health.`, `alerts.critical.` (+ built-in `config.`).

---

## APPENDIX D: COVERAGE (master list → steps)

| M-id | Item | Steps | Status at plan end |
|---|---|---|---|
| M-050 | ADR-103 build | 7-10, 29-66, 67-96, 97-124, 125-158, 159-178, 179-222, 223-250 | DONE (Stevie to accept ADR-103) |
| M-051 | TLS on every internal link (FND-017) | 251-269, 376-379 | DONE in compose; hosts [AFTER-E3] |
| M-052 | Wotan auto-approve bound to identity | 231-237 | DONE |
| M-053 | Signed topic prefixes + E5 drift keys | 270-275, 276-283 | prefixes DONE; drift keys [AFTER-E2] |
| M-054 | Akira cert-expiry exec check + rotation test | 286-295 | DONE |
| M-055 | Dashboard admin login + RBAC | 186-210, 211-217 | DONE |
| M-056 | Security headers middleware | 296-303 | DONE |
| M-060 | cosign + SLSA in release.yml | 304-312 | DONE (first run on the first tag) |
| M-061 | Build key custody, signed Sealed Cask | 329-347 | signer/verifier/runbook DONE; ceremony [AFTER-E1] |
| M-062 | deb signing + apt repo + name fix | 348-361, 366-375 | tooling DONE; live [AFTER-E3, E4] |
| M-063 | release.yml builds all debs + eBPF; east deploy | 305, 362, 374-379 | workflow DONE; east [AFTER-E3] |
| M-064 | CHANGELOG + v0.2.0-alpha + SBOM | 326, 362, 401 (banner), tag by Stevie | CHANGELOG DONE; tag is Stevie's push |
| M-065 | Scorecard + CodeQL | 313-315 | DONE (results after push) |
| M-066 | ADR-089 byte-size pin script | 316-321 | DONE |
| M-067 | sophia-eye image pins + Dependabot docker | 323-325, 363-365 | DONE |
| M-201 | Heimdall drift keys + unit | 276-283 | unit/keygen/runbook DONE; live [AFTER-E2] |
| M-041 (overlap) | East deb bundle + wotan.yaml + huginn baseline | 376, 380 | live [AFTER-E3] |
| Decisions 4, 5, 6, 16-19 | runner, custody, admissions, ADR-103 Q1-4 | 11, 342/366, 396, 7-10 | 4 and 16-19 decided by recommendation; 5, 6 escalated |

Deferred to other sprints: M-040 `LXD_TRUST_PASSWORD` default (S-CLEAN-SLATE E03), M-016 GitHub secret scanning (S-CLEAN-SLATE E02, GitHub admin), M-188 FND-020 replication signatures (S-TRUE-WIRE), Well-backed session store (follow-on when ADR-099 lands in S-WELL-SPRING).

---

*S-IRON-GATE Battle Plan — Forged 2026-10-27*
*17 Phases. 402 Steps. Every service proves who it is, every internal byte is encrypted, and every release carries a signature someone can check.*
