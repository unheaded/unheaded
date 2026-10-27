<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# S-CLEAN-SLATE BATTLE PLAN: 9 Phases, 288 Steps

**Date**: 2026-10-27
**Sprint**: Sprint 1 of 7 (index: [NEXT-7-SPRINTS-2026-10-27](NEXT-7-SPRINTS-2026-10-27.md)). CLEAN SLATE: develop's CI goes green and stays promotable, every live-service correctness bug found by the sweeps is fixed, the public-repo security items that need no host or credential action are done, the ones that do are prepared behind escalations, and the docs stop saying things the code does not do.
**Prerequisite**: `develop` at `a7fe3acd` or newer, `git status --short` empty, `go build ./...` clean (verified 2026-10-27). No earlier sprint required: this plan runs first.
**Target**: all six GitHub workflows green on the develop HEAD this sprint produces; Wotan's admin endpoints usable with a key; no live service stays deaf to Wotan because it started first; both haproxy configs pass `haproxy -c`; the Zhen web UI goes through the Champion gate; the monitoring stack comes up with the kingdom; the ADR-098 register has no untriaged row; the 21 doc-hygiene items in `docs/sweeps/OUTSTANDING-2026-10-27.md` §5 are fixed; and `develop → staging` promotion is one answered escalation away.
**Estimated Duration**: 22-30 h of execution across 4-6 sessions. Thirteen escalations wait on Stevie (E1-E13 below); none blocks more than its own chain.
**Agent Strategy**: Coordinator owns Phases 0, 1, 4 (CI, hosts, live windows) and 8. Phases 2 and 3 are code-only and can run in parallel in two worktrees off develop ([P:3] / [P:2]). Phase 6 (docs) can run in parallel with Phases 2-5 in its own worktree ([P:2-5]); its commits touch only docs/, wiki/, CLAUDE.md, references/. Hosts are read-only for every agent unless the step is covered by an answered [ESCALATE].
**Commit Cadence**: every 5 steps (max(3, min(5, 288/20)) = 5), at every exit gate, before every STUCK skip, after every red-to-green test. Never commit red. Never push (Stevie pushes). Signed if `git commit-tree -S` works (Step 6), otherwise `git commit --no-gpg-sign` (memory: feedback_unsigned_commits_when_afk); Stevie re-signs with `scripts/resign-unpushed.py`.
**Stuck Protocol**: skip after 3x the step's `~time` or 2 failed debug attempts; write the STUCK marker into this file, commit, continue with the next step that does not depend on it (Appendix B dependency map).

**Builds on (cited, not re-derived):**

| Source | What this plan takes from it |
|---|---|
| `docs/sweeps/OUTSTANDING-2026-10-27.md` (master list) | Items M-001..M-008 (E01), M-010..M-021 (E02), M-030..M-042 (E03), §5 doc-hygiene 1-21, decisions 1-3, 7-15 (E2/E3 only of 10). Appendix D maps every id to its steps. |
| `docs/sweeps/outstanding-2026-10-27/sweep-*.md` | Per-item evidence (paths, greps, commits) the sweeps recorded at a7fe3acd. |
| GitHub Actions public API, develop `a7fe3acd` (read 2026-10-27) | Red: Security Scan ("Pinned tools are current" exit 2; gitleaks job's "Third-party packages may only shrink" exit 1), Static Analysis ("runbook dry-run tests (GATING)" exit 1; "Gates can actually fail" cancelled at 25 min). Green: Lich, Timeline Drift Guard, SBOM, GPL Boundary. Nightly drift-detect on main: failure 5/5. |
| Local reproduction 2026-10-27 | `check-dependency-baseline.sh` FAIL (fsnotify, kr/pty, gobreaker, x/term gone from build); `check-tool-freshness.sh` STALE syft 1.55.0 / grype 0.121.0; runbook pytest 100 passed under local venv, under a venv from `.github/ci-python/requirements.txt` on py3.13, and inside a depth-1 clone; `go build ./...` and `go mod tidy -diff` clean; `haproxy -c` fails on both configs (3.0.12). |
| ADR-098 findings register (`compliance/findings/register.yaml`) | FND-001 (high, due 10-30), FND-011 (high, due 11-18), FND-014, FND-016, FND-023, FND-024 texts and verify clauses; one untriaged row (gitleaks job). |
| S-REAL-STEEL plan + `~/tmp/next.md` S23 | FND-023/024 origin (Phase 11 Step 310, Phase 9 run 4); the Phase 2 Wotan-client follow-on list (degraded-mode one-shots, no periodic re-probe); "found, not fixed": haproxy configs, `switch-model.sh` fit check, `LXD_TRUST_PASSWORD:-changeme`, lxdbr0 probe restriction. |
| `docs/security/pq-host-escalations.md` | E2 (sshd hybrid-only) and E3 (WireGuard PSK) procedures and lockout analysis. |
| ADR-052 / `scripts/check-timeline-freshness.sh` | Timeline body + mirrors current to 2026-10-26; header/footer stale (doc item 1). |
| Memory: feedback_catch_in_develop, feedback_stevie_owns_commits, feedback_no_creds_in_repo, feedback_service_nologin_users, feedback_doom_lowest_priority | Fix on develop first; Claude commits, Stevie pushes; env files under /etc/unheaded; no Doom step in this plan. |

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
[AFTER-E<n>]   Runs only once E<n> is answered; skipped (not STUCK) until then
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
6. Attack surface unchanged, or the change is listed in Phase 7's review table.
7. Every claim of "real" in a commit message has a [LIVE-RO] or [LIVE-RW] step that proved it, cited by step number.
8. Zero user data access: no step reads packet payloads, log contents of other services, or user data; probes read metadata only.

---

## SITUATION (verified at a7fe3acd, 2026-10-27)

**CI (E01)**
- `scripts/check-dependency-baseline.sh` exits 1: `github.com/fsnotify/fsnotify`, `github.com/kr/pty`, `github.com/sony/gobreaker`, `golang.org/x/term` are in `docs/policy/dependency-baseline.txt` but no longer in the build. `--update` only shrinks (`scripts/check-dependency-baseline.sh:98-109`). This is the gitleaks job's "Third-party packages may only shrink" failure (run 40426721028).
- `scripts/check-tool-freshness.sh` exits 1: `SYFT_VERSION="1.54.1"` and `GRYPE_VERSION="0.120.1"` (`scripts/lib/tool-pins.sh:21-25`) while 1.55.0 / 0.121.0 are past the 7-day cooldown. This is "Pinned tools are current" exit 2 (same run). Not an HTTP error path, as next.md S21 guessed.
- `python3 -m pytest tests/runbooks -q -m "not live"` (`.github/workflows/static-analysis.yml:221-222`) passes here three ways (local venv; venv from `.github/ci-python/requirements.txt` on Python 3.13.7; depth-1 clone) and fails on GitHub (run 40426721063, Python 3.12). The step prints nothing to the public API; the job log needs auth.
- `meta-gate` ("Gates can actually fail") `timeout-minutes: 25` (`static-analysis.yml:276`); cancelled every run. `scripts/check-gates-can-fail.sh` full run took ~50 min on west (next.md S21); `--quick` skips SLOW gates (`:70`, `:986`); lich fuzz is 5 s/target since 43bf9199.
- `.github/workflows/drift-detect.yml` (nightly, terragrunt) fails 5/5 on main; nothing it checks is deployed.
- `modernc.org/sqlite v1.60.1` (`go.mod:44`); 1.61.0 is inside the 7-day cooldown until 2026-10-28.
- `go build ./...` clean; `go mod tidy -diff` empty; gpg key not cached (`git commit-tree -S` fails).

**Live services (E03)**
- `SetAdminAPIKey` (`services/wotan/internal/api/handlers.go:50`) has zero callers; `-admin` defaults true (`services/wotan/cmd/wotan/main.go:418`); `handlers.go:489` refuses when the key is empty, so `/api/v1/admin/*` (`main.go:497-500`) always 401/403s. `wiki/Service-Wotan.md:50` says it works.
- One-shot "wotan unavailable ... (degraded mode)" with no retry in live services: `services/micromanager/service.go:50,107,147,187,224`, `services/monad/monad.go:333,641,685`, `services/sophia/sophia.go:274,895,913`, `services/gateway/gateway.go:370`, `services/architect/core.go:254,268,303`, `services/anomaly/anomaly.go:768,882`, `services/loadbalancer/xdpsync.go:185,688`. `SubscribeWithRetry` exists (`pkg/wotan-client/subscribe_retry.go:30`) and is used by timeguru and yaldabaoth only. The other hits (nfv, compliance, qos, firewall, selfheal, version) are parked G9 services: out of scope (decision 41).
- `pkg/wotan-client/client.go:290` promises "Call ProbeGRPC() periodically"; nothing does (no ticker in client.go; `Close()` at :398).
- FND-024: `scripts/kingdom-startup.sh` Phase 1 (`:130-152`) starts only `$COMPOSE_FILE`; `monitoring/docker-compose.yml` is started by nobody; `pkg/sloguard` fails closed without `slo:error_ratio:5m`.
- FND-023: `cmd/steel-echo/load.go:91-97` discover mode refreshes from Cuirass every 2 s and trusts the list; a killed replica stays a target until Cuirass's 30 s sync (availability 0.9343 vs 0.99).
- `docker/haproxy/internal/haproxy.cfg:36` puts a `log-format` with `%[req.hdr(X-Trace-Id)]` in `defaults`, which a non-HTTP section inherits (ALERT). `docker/haproxy/edge/haproxy.cfg:30` `module-path` (unknown keyword), `:94` `http-request` after `use_backend`, `:100` `crt /etc/haproxy/certs/` missing. `docker/docker-compose.loadbalancers.yml:12` says "KNOWN BROKEN"; image `haproxy:2.9.15-alpine`; west has haproxy 3.0.12.
- `raft/scripts/zhen_rag.py:231` `proxy_via_agentd=None`: the web UI talks to llama-server directly, bypassing zhen-agentd's Champion gate (`/api/v1/agent/ask`).
- `scripts/switch-model.sh:170-204` fit check sums model + context against MemAvailable; it ignores llama-server's `--cache-ram` (default 8 GiB).
- `scripts/setup-host.sh:222` `core.trust_password: ${LXD_TRUST_PASSWORD:-changeme}`.
- `services/yaldabaoth/allowlist.go:128` refuses patterns matching host interfaces; nothing restricts instance-reported probe addresses to the lxdbr0 subnet.
- `cmd/zhen-agentd/main.go:70-82`: `-rate-limit` default 0, no `-no-auth`; `toolexec.go:140-152` synthesises `direct-user` when a request omits justification.
- `cmd/zhen-claude/routed.go:67 runRouted`: local-route turns are not written as zhen-capture records.
- East runs older debs (huginn rename, chaos-controller unit, wotan.yaml `auto_approve` without `chaos-controller`: `configs/wotan.yaml:17-23`).

**Public-repo security, code-only (E02)**
- `SECURITY.md` exists (sections :3-129); no `/.well-known/security.txt` is served (`grep security.txt` empty); README has no security-posture section (sections: Building, Components, Protocol, UPC, Stack, Zhen, License, Author).
- `cmd/unheaded-baseline/main.go:90-97` has no Wotan flag; no `compliance.deviation.<host>` publish.
- `deploy/systemd/user/unheaded-git-mirror.{service,timer}` exist (`OnCalendar=*:0/30`, `git-mirror.sh push`); FND-005 is "install on west".
- `compliance/remediation/fixes.yaml:78-92` kmod-cis part has no ebtables lines (FND-011 stopgap).
- `cmd/alert-mailer/main.go:68,113`: `mailer.mode` dry-run|send; send needs relay settings (FND-001).
- `.github/BRANCH_PROTECTION.md` exists; secret scanning / push protection state unknown (GitHub admin).

**Docs (§5)**
- `references/timeline.md:5-7,108`: STATUS/LAST UPDATED 2026-10-13/HEAD (09-29)/Synced 2026-09-30 stale; body and json/yaml/toml mirrors current to 2026-10-26 (2d42f587). Freshness gate FRESH.
- `docs/adr/ADR-036-claude-distillation-training.md:3` "IN PROGRESS"; `docs/adr/ADR-INDEX.md:97` "In Progress"; `docs/TODO-claude-qa-generation.md` exists. Anthropic (2026-10-26): no training on Claude outputs.
- `docs/protocol/ALIGNMENT_NOTES_DRAFT05.md`, `PROTOCOL_STATUS_S72.md`: 17/17 sampled paths absent (sweep-protocol).
- `docs/internal/`: 147 files = `battle-plans/` 88 + `sessions/` 58 + `security/` 1; `docs/INTERNAL.md:30` names `battle-plan-S73-public-launch` as active.
- `docs/workflow/AGENT-OPERATING-PROCEDURE.md:55,119`: "134/134", `git push origin main`. `docs/launch/RELEASE_CHECKLIST.md:6,7,37`: MIT. `CHANGELOG.md:18,53,76`: mTLS mesh. `docs/PERFORMANCE-S77.md:48-64`: `e2ePacketTrace`, `--only`, `ci-bench`, "18 ms p99" (none exist). `docs/BENCHMARKS.md:96`, `CLAUDE.md:829`: "920K pps (AF_XDP)".

## ESCALATIONS (table; each [ESCALATE] step below cites one)

> **Answered by Stevie 2026-10-27** (index "Decisions taken 2026-10-27"): every GRANTED chain below may run unattended; OPEN items wait on his hands. Step 288 (E12) stays a stop.

| E | Question for Stevie | Blocks | Answer 2026-10-27 |
|---|---|---| --- |
| E1 | Pull the job log for Static Analysis run 40426721063, job "Runbooks (schema + dry-run tests)", step "runbook dry-run tests (GATING)" (needs GitHub auth; west has no `gh`). | Step 42 only | OPEN, Stevie pastes the failing lines from run 40426721063 |
| E2 | West live window: start the monitoring stack from kingdom-startup, bring the haproxy pair up, enable the git-mirror user timer, re-run FND-023's Step 310 experiment. | Steps 153-157, 155b, 164-166 | GRANTED 2026-10-27: unattended west window OK; FND-024 via kingdom-startup.sh; FND-023 client probe |
| E3 | Deb deploy window on east and west (install the rebuilt debs, restart units, update huginn baseline). | Steps 173-174 | GRANTED 2026-10-27: any time, unattended; report before/after unit list |
| E4 | Apply the FND-011 ebtables modprobe stopgap on both hosts (kmod-cis); keep west's unprivileged userns open for LXD until tested. | Steps 195-196 | GRANTED 2026-10-27: stopgap on both hosts; west userns stays open until an LXD test |
| E5 | FND-001 SMTP relay: a Gmail account + app password, written to `/etc/unheaded/alert-mailer/smtp.env` on west by Stevie. | Steps 199-201 | OPEN, Stevie writes the Gmail app-password env on west; then run |
| E6 | Rotate the baselined credentials (20 history fingerprints in `.gitleaksignore`; OPNsense API, Grafana, LXD trust, tokens). | Steps 204-205 | IN PROGRESS, Stevie rotates during Sprint 1; shrink the baseline when he says done |
| E7 | FND-014 live Well window: role password rotation, `pg_stat_statements_reset()`, old PG logs. | Steps 207-208 | GRANTED 2026-10-27: any time, unattended; backup first; new passwords to .env on west only |
| E8 | E3 of Quantum Armor: first live WireGuard PSK `--apply` rotation on west, remove east's stale key, enable the weekly timer. | Steps 209-210 | GRANTED 2026-10-27: rotate, then enable the weekly timer on both hosts |
| E9 | E2 of Quantum Armor: sshd hybrid-only KEX on east then west (lockout risk; `docs/security/pq-host-escalations.md`). | Steps 212-213 | GRANTED 2026-10-27: east first with a fallback session, then west |
| E10 | GitHub admin: secret scanning + push protection on; default branch `develop`; protect `main`. | Steps 214-215 | OPEN, Stevie flips all three; record and verify push protection on a scratch branch |
| E11 | Send the EAR 5D002 TSU notification email (BIS + NSA). | Steps 217-218 | OPEN, plan drafts (Step 216); Stevie sends; record the date |
| E12 | Promote `develop → staging` (1,062 commits), then `staging → main` + signed tag, on your word. | Step 288 (end) | OPEN, only after Sprint 1 CI is green and on Stevie's word |
| E13 | Relicense the ~290 non-Go files whose header still says MIT (and LICENSE line 1 "Steven Bellis") to GPL-3.0-or-later, except files deliberately MIT. | Steps 256-258 | GRANTED 2026-10-27: mechanical pass from the Step 255 list; skip files with a documented MIT reason |

---

## PHASE 0: PREFLIGHT (Steps 1-24)

**Goal**: Re-verify every SITUATION claim live before touching anything; record the KNOWN FAILURES BASELINE; decide signing; set up worktrees.
**Prerequisite**: develop at a7fe3acd or newer, tree clean.
**Time**: 40 min (the Go test baseline dominates).
**Agent**: Coordinator.

### Environment

- [ ] **Step 1** [ENV][B] ~1m: Branch, HEAD, tree.
  ```bash
  cd /home/govan/tmp/unheaded && git rev-parse --abbrev-ref HEAD && git rev-parse --short HEAD && git status --short | wc -l
  ```
- [ ] **Step 2** [V] ~0.5m: Branch is `develop`, HEAD is `a7fe3acd` or a descendant (`git merge-base --is-ancestor a7fe3acd HEAD && echo ok`), status count is 0 (the two untracked `cmd/host-agent/` and `host-agent` July binaries are the known exception; see next.md S21).
  - If fail → STOP; this plan starts from a clean develop.
- [ ] **Step 3** [ENV][B] ~1m: Tools present.
  ```bash
  for t in go cargo jq curl python3 haproxy docker shellcheck gpg; do printf "%-10s %s\n" "$t" "$(command -v $t || echo MISSING)"; done; go version; haproxy -v 2>&1 | head -1
  ```
- [ ] **Step 4** [V] ~0.5m: `go`, `jq`, `curl`, `python3`, `haproxy`, `docker`, `shellcheck`, `gpg` all present. `cargo` may be missing (not needed here).
  - If `shellcheck` missing → Step 5.
- [ ] **Step 5** [D][B] ~2m: Install shellcheck from the pinned CI tooling, not apt.
  ```bash
  bash scripts/ci-install-tools.sh shellcheck 2>&1 | tail -3 || echo "see scripts/ci-install-tools.sh for the pinned recipe"
  ```
- [ ] **Step 6** [ENV][B] ~0.5m: GPG probe (decides the commit flag for the whole plan).
  ```bash
  cd /home/govan/tmp/unheaded && tree=$(git rev-parse HEAD^{tree}); if timeout 20 git commit-tree -S -m probe "$tree" >/dev/null 2>&1; then echo SIGN=yes; else echo SIGN=no; fi
  ```
- [ ] **Step 7** [DECIDE] ~0.5m: **Commit flag.** RECOMMENDATION: if Step 6 printed `SIGN=no`, every [C] in this plan uses `git commit --no-gpg-sign`; record `COMMIT_FLAG` in the progress file. Rationale: memory feedback_unsigned_commits_when_afk; Stevie re-signs with `scripts/resign-unpushed.py`. Override ONLY if Stevie cached the key mid-plan (re-run Step 6).
- [ ] **Step 8** [ENV][BUILD][B] ~3m: Build + tidy.
  ```bash
  cd /home/govan/tmp/unheaded && go build ./... && go mod tidy -diff && echo BUILD_TIDY_OK
  ```
- [ ] **Step 9** [V] ~0.5m: `BUILD_TIDY_OK` printed, no diff lines.
  - If fail → STOP; develop is not what the sweeps measured.
- [ ] **Step 10** [ENV][TEST][B] ~20m: **KNOWN FAILURES BASELINE.**
  ```bash
  cd /home/govan/tmp/unheaded && mkdir -p docs/battle-plans/progress && go test ./... 2>&1 | tee /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/go-test-baseline.log | tail -5; grep -E '^(FAIL|---  FAIL|ok)' /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/go-test-baseline.log | grep -c '^ok'; grep -E '^FAIL' /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/go-test-baseline.log || echo "FAIL lines: none"
  ```
- [ ] **Step 11** [W] ~2m: Create `docs/battle-plans/progress/S-CLEAN-SLATE-progress.md` with: date, HEAD, `COMMIT_FLAG`, the ok-count and the FAIL lines from Step 10 under `## KNOWN FAILURES BASELINE`, and an empty `## Steps` ledger. Every later [C] appends one line here.

### Hypothesis verification (one pair per SITUATION claim)

- [ ] **Step 12** [PREFLIGHT][B] ~1m: CI reds reproduce locally.
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/check-dependency-baseline.sh 2>&1 | tail -6; echo "dep=$?"; bash scripts/check-tool-freshness.sh 2>&1 | grep -E 'STALE|exit' ; echo "tools=${PIPESTATUS[0]}"
  ```
- [ ] **Step 13** [V] ~0.5m: dependency check lists exactly fsnotify, kr/pty, gobreaker, x/term; tool freshness lists STALE syft and grype only.
  - If a different set → update SITUATION in this file (the sweeps are stale by a day), continue.
- [ ] **Step 14** [PREFLIGHT][B] ~2m: Live CI state (public API, no auth).
  ```bash
  curl -s --max-time 20 "https://api.github.com/repos/unheaded/unheaded/actions/runs?branch=develop&per_page=12" | jq -r '.workflow_runs[] | [.name, .head_sha[0:8], .conclusion // .status] | @tsv' | column -t -s$'\t'
  ```
- [ ] **Step 15** [V] ~0.5m: For the newest sha, Security Scan and Static Analysis are `failure`, the other four `success`. If develop has moved and a run is `in_progress`, wait and re-run Step 14 once.
- [ ] **Step 16** [PREFLIGHT][B] ~1m: Code-site claims.
  ```bash
  cd /home/govan/tmp/unheaded && echo "SetAdminAPIKey callers: $(grep -rn 'SetAdminAPIKey(' --include=*.go . | grep -v 'func (s \*Server) SetAdminAPIKey' | grep -v _test | wc -l)"; echo "degraded sites (live services): $(grep -c 'degraded mode' services/micromanager/service.go services/monad/monad.go services/sophia/sophia.go services/gateway/gateway.go services/architect/core.go services/anomaly/anomaly.go services/loadbalancer/xdpsync.go | awk -F: '{s+=$2} END{print s}')"; echo "client ticker: $(grep -c 'NewTicker' pkg/wotan-client/client.go)"; echo "startup monitoring: $(grep -c 'monitoring/docker-compose.yml' scripts/kingdom-startup.sh)"; echo "cache-ram in switch-model: $(grep -c 'cache-ram' scripts/switch-model.sh)"; echo "proxy_via_agentd default: $(grep -n 'proxy_via_agentd=None' raft/scripts/zhen_rag.py | head -1)"; echo "changeme: $(grep -c 'changeme' scripts/setup-host.sh)"
  ```
- [ ] **Step 17** [V] ~0.5m: callers 0; degraded sites 18; ticker 0; startup monitoring 0; cache-ram 0; `proxy_via_agentd=None` found; changeme 1.
  - Any mismatch → that item may be partly done: read the site, adjust the phase's steps, note it in the progress file. Do not skip the item.
- [ ] **Step 18** [PREFLIGHT][B] ~1m: haproxy configs still fail.
  ```bash
  cd /home/govan/tmp/unheaded && for f in docker/haproxy/internal/haproxy.cfg docker/haproxy/edge/haproxy.cfg; do printf "%s: " $f; haproxy -c -f $f >/dev/null 2>&1 && echo OK || echo FAIL; done
  ```
- [ ] **Step 19** [V] ~0.5m: both FAIL.
- [ ] **Step 20** [PREFLIGHT][B] ~1m: Doc claims.
  ```bash
  cd /home/govan/tmp/unheaded && sed -n '6p' references/timeline.md; grep -n '^\*Synced:' references/timeline.md | cut -c1-40; sed -n '3p' docs/adr/ADR-036-claude-distillation-training.md | cut -c1-60; find docs/internal -name '*.md' | wc -l; grep -c 'git push origin main' docs/workflow/AGENT-OPERATING-PROCEDURE.md; grep -c 'e2ePacketTrace' docs/PERFORMANCE-S77.md
  ```
- [ ] **Step 21** [V] ~0.5m: LAST UPDATED 2026-10-13; Synced 2026-09-30; ADR-036 IN PROGRESS; 147 files; 1; ≥3.

### Worktrees (optional parallelism)

- [ ] **Step 22** [DECIDE] ~0.5m: **Parallel agents.** RECOMMENDATION: run Phases 2+3 in worktree `../unheaded-wt-code` (branch `clean-slate/code`) and Phase 6 in `../unheaded-wt-docs` (branch `clean-slate/docs`), merging into develop at each phase exit gate with `git merge --no-ff`. Rationale: disjoint file sets (Go/scripts vs docs/wiki). Override ONLY if running solo in one session: then execute phases in order on develop and skip Step 23.
- [ ] **Step 23** [ENV][B] ~1m: Create the worktrees (only under Step 22's recommendation).
  ```bash
  cd /home/govan/tmp/unheaded && git worktree add -b clean-slate/code ../unheaded-wt-code develop && git worktree add -b clean-slate/docs ../unheaded-wt-docs develop && git worktree list
  ```
- [ ] **Step 24** [V] ~0.5m: **PHASE 0 EXIT GATE** — Steps 2, 4, 9, 13, 15, 17, 19, 21 passed; progress file exists with the baseline; `COMMIT_FLAG` recorded.
  - If pass → Phase 1.
  - If fail → DO NOT PROCEED. Fix the environment or correct SITUATION.

---

## PHASE 1: CI GREEN ON DEVELOP (Steps 25-52)

**Goal**: Every red GitHub job has a local fix or a self-reporting step; the two stale pins and the stale baseline are corrected; the nightly drift job stops failing; sqlite is bumped when its cooldown ends. Covers M-001, M-002, M-003, M-004, M-005, M-006, M-007 (M-008 is Step 266).
**Prerequisite**: Phase 0 gate.
**Time**: 2.5 h (one ~60 min measurement).
**Agent**: Coordinator (edits `.github/workflows`).

### M-001 dependency baseline

- [ ] **Step 25** [B] ~0.5m: Shrink the baseline.
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/check-dependency-baseline.sh --update 2>&1 | tail -8 && git diff --stat docs/policy/dependency-baseline.txt
  ```
- [ ] **Step 26** [V] ~0.5m: Diff shows only removed lines (`git diff docs/policy/dependency-baseline.txt | grep -c '^+[^+]'` is 0) and exactly the 4 modules removed; then `bash scripts/check-dependency-baseline.sh` exits 0.
  - If `--update` prints `[REFUSED]` → a new module crept in: STOP, that is an ADR-004/096 admission question; raise it as a new escalation line in this file.

### M-002 tool pins

- [ ] **Step 27** [B] ~2m: Fetch the upstream checksums for the two releases and extract the linux_amd64 tarball lines.
  ```bash
  cd /home/govan/tmp/unheaded && S=/tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad; curl -sSL --max-time 60 -o $S/syft_sums.txt https://github.com/anchore/syft/releases/download/v1.55.0/syft_1.55.0_checksums.txt && grep 'linux_amd64.tar.gz' $S/syft_sums.txt; curl -sSL --max-time 60 -o $S/grype_sums.txt https://github.com/anchore/grype/releases/download/v0.121.0/grype_0.121.0_checksums.txt && grep 'linux_amd64.tar.gz' $S/grype_sums.txt
  ```
- [ ] **Step 28** [V] ~0.5m: One 64-hex line each. Cross-check by downloading each tarball and hashing it (`curl -sSL -o /tmp/x.tgz <release url> && sha256sum /tmp/x.tgz`); the hash must match the checksums file. Two independent sources agree before a pin changes.
  - If 404 → the version string is wrong; re-read `check-tool-freshness.sh` output from Step 12 and retry once.
- [ ] **Step 29** [W] ~1m: Edit `scripts/lib/tool-pins.sh`: `SYFT_VERSION="1.55.0"`, `SYFT_SHA256="<hash>"`, `GRYPE_VERSION="0.121.0"`, `GRYPE_SHA256="<hash>"`. Comment suffixes unchanged.
- [ ] **Step 30** [B] ~1m: Freshness gate and the SBOM script that consumes the pins.
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/check-tool-freshness.sh 2>&1 | grep -E 'syft|grype|STALE'; echo "rc=${PIPESTATUS[0]}"; grep -ln 'SYFT_SHA256\|GRYPE_SHA256' scripts/*.sh scripts/lib/*.sh .github/workflows/*.yml | head
  ```
- [ ] **Step 31** [V] ~0.5m: `[ok] syft 1.55.0`, `[ok] grype 0.121.0`, rc=0. Every consumer of the pins reads `scripts/lib/tool-pins.sh` (no second copy of the old hash anywhere: `grep -rn 'c069905b391cc4\|0a9ee97ef5ae2e' . --exclude-dir=.git` is empty).
- [ ] **Step 32** [B] ~4m: Run the SBOM path that downloads the pinned tools, so the new hashes are proven, not just pasted.
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/ci-install-tools.sh syft grype 2>&1 | tail -4 && syft version 2>/dev/null | head -2 && grype version 2>/dev/null | head -2
  ```
- [ ] **Step 33** [V] ~0.5m: Both tools install with checksum verified and report 1.55.0 / 0.121.0.
  - If checksum mismatch → the checksums file and tarball disagree; STOP the pin (keep old), log STUCK on Step 29, continue (the job stays red for this one line; say so in the handoff).
- [ ] **Step 34** [C] ~1m: **COMMIT CHECKPOINT** `[PLAN S-CLEAN-SLATE] Steps 25-33: dependency baseline shrunk (4 modules), syft 1.55.0 + grype 0.121.0 pinned`.

### M-003 runbook pytest: self-reporting + Python 3.12 reproduction

- [ ] **Step 35** [W] ~3m: Edit `.github/workflows/static-analysis.yml` step "runbook dry-run tests (GATING)" (`:221-222`) to:
  ```yaml
      - name: runbook dry-run tests (GATING)
        shell: bash
        run: |
          set -o pipefail
          python3 -m pytest tests/runbooks -q -rA -m "not live" 2>&1 | tee /tmp/runbook-pytest.log
          rc=${PIPESTATUS[0]}
          { echo '## runbook dry-run tests'; echo '```'; tail -80 /tmp/runbook-pytest.log; echo '```'; } >> "$GITHUB_STEP_SUMMARY"
          exit $rc
  ```
  Also add `pytest -rA` short-test-summary to the local wrapper: nothing else changes.
- [ ] **Step 36** [B] ~1m: Workflow YAML still valid and the gate script that checks workflows is happy.
  ```bash
  cd /home/govan/tmp/unheaded && python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/static-analysis.yml'))" && echo YAML_OK && bash scripts/check-actions-pinned.sh 2>&1 | tail -2
  ```
- [ ] **Step 37** [V] ~0.5m: `YAML_OK`; actions-pinned check passes (no new `uses:`).
- [ ] **Step 38** [B] ~6m: Reproduce the CI environment as closely as west can: Python 3.12 in a container, pinned requirements, no Go, no gpg (conftest skips those tests; a *fail* that is not a *skip* is the clue).
  ```bash
  cd /home/govan/tmp/unheaded && sudo docker run --rm -v "$PWD":/w -w /w python:3.12-slim bash -c 'pip install -q --require-hashes --no-deps --only-binary=:all: -r .github/ci-python/requirements.txt && python3 -m pytest tests/runbooks -q -rA -m "not live" 2>&1 | tail -40'
  ```
- [ ] **Step 39** [V] ~0.5m: Either (a) a FAIL appears → Step 40; or (b) all pass/skip → Step 41.
- [ ] **Step 40** [D][CODE] ~15m: Fix the failing test or the runbook it exercises (red → green in the same container command). If the failure is "needs X on PATH" that the runner lacks, make the test `skipif` on `shutil.which`, the pattern `tests/runbooks/harness.py:148` already uses. Re-run Step 38 until clean. Max 2 attempts.
- [ ] **Step 41** [B] ~1m: Local gate still green under the pinned venv.
  ```bash
  cd /home/govan/tmp/unheaded && S=/tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad; ( export PATH=$S/civenv/bin:$PATH; python3 -m pytest tests/runbooks -q -m "not live" 2>&1 | tail -2 )
  ```
- [ ] **Step 42** [ESCALATE] **E1** ~0m: Context: Steps 38-41 may not reproduce the GitHub failure (passes 4 ways locally). Question: pull the step log for run 40426721063 job "Runbooks (schema + dry-run tests)" and paste the FAIL lines into the progress file. Why no recommendation: the log needs GitHub auth; west has no `gh`. This chain stops here; the self-reporting change in Step 35 makes the next failure readable without auth.

### M-004 meta-gate timeout

- [ ] **Step 43** [B] ~60m: Measure the real duration once, the way the job runs it (full, not `--quick`).
  ```bash
  cd /home/govan/tmp/unheaded && ( time bash scripts/check-gates-can-fail.sh ) 2>&1 | tail -6
  ```
- [ ] **Step 44** [DECIDE] ~1m: **Timeout.** RECOMMENDATION: set `timeout-minutes` on `meta-gate` to `ceil(measured_minutes * 1.2) + 5`, minimum 30. If measured > 50 min, instead split: a `meta-gate-fast` job running `--quick` on every push and a `meta-gate-slow` job on `schedule` (nightly) + `workflow_dispatch`. Rationale: a gate that always cancels proves nothing (SPRINT-AUDIT, S21). Override ONLY if the measured time is under 20 min (then the cancellations had another cause: inspect `check-gates-can-fail.sh` for a network wait and fix that instead).
- [ ] **Step 45** [W] ~3m: Apply Step 44 to `.github/workflows/static-analysis.yml:276` (or add the split job below `meta-gate`, same pinned `uses:` shas as the existing job).
- [ ] **Step 46** [V] ~1m: YAML valid (Step 36 command); `scripts/check-actions-pinned.sh` green; `scripts/check-gates-can-fail.sh --quick 2>&1 | tail -3` still exits 0.

### M-005 nightly drift-detect

- [ ] **Step 47** [DECIDE] ~1m: **drift-detect.yml.** RECOMMENDATION: delete the workflow. Rationale: it runs terragrunt against infrastructure nothing deploys (decision 1 default; 5/5 failures on main since 2026-10-22; terraform/opentofu is not in LIVE-PATHS). Override ONLY if `grep -rln 'terragrunt\|opentofu' deploy/ docs/LIVE-PATHS.md` shows a live consumer.
- [ ] **Step 48** [B] ~1m: Remove it and every doc that advertises it.
  ```bash
  cd /home/govan/tmp/unheaded && git rm -q .github/workflows/drift-detect.yml && grep -rln 'drift-detect\|Nightly Drift Detection' docs wiki README.md CLAUDE.md .github 2>/dev/null
  ```
- [ ] **Step 49** [W] ~3m: In each file Step 48 listed, replace the drift-detect mention with one line: "Nightly Drift Detection removed 2026-10-27 (S-CLEAN-SLATE Step 48): terragrunt checked infrastructure that is not deployed; config drift is Heimdall's job (ADR-098 baseline agent)." Keep wiki pages consistent (`wiki/Security-Tooling.md` if present).
- [ ] **Step 50** [V] ~0.5m: `grep -rn 'drift-detect.yml' . --exclude-dir=.git` returns only this plan and the master list.

### M-007 sqlite (date-gated) and M-006 register

- [ ] **Step 51** [B] ~5m: Bump `modernc.org/sqlite` only on/after 2026-10-28; otherwise print SKIP and leave a dated TODO line in the progress file.
  ```bash
  cd /home/govan/tmp/unheaded && if [ "$(date -u +%F)" \> "2026-10-27" ]; then go get modernc.org/sqlite@v1.61.0 && go mod tidy && go build ./... && bash scripts/check-dependency-baseline.sh | tail -1 && go test $(go list -deps ./... | grep -q modernc.org/sqlite && go list -f '{{if .Deps}}{{.ImportPath}}{{end}}' ./cmd/... ./pkg/... | head -400) -count=1 2>&1 | grep -E '^(FAIL|ok)' | sort | uniq -c | tail -3; else echo "SKIP: cooldown until 2026-10-28; re-run Step 51 next session"; fi
  ```
- [ ] **Step 52** [V] ~1m: **PHASE 1 EXIT GATE** — `bash scripts/check-dependency-baseline.sh && bash scripts/check-tool-freshness.sh && bash scripts/check-actions-pinned.sh && python3 -c "import yaml;[yaml.safe_load(open(f)) for f in __import__('glob').glob('.github/workflows/*.yml')]" && echo PHASE1_OK` prints `PHASE1_OK`; `go build ./...` clean; if Step 51 ran, no new FAIL versus the baseline. Then [C] `[PLAN S-CLEAN-SLATE] Steps 35-52: runbook step self-reporting, meta-gate timeout set, drift-detect removed, sqlite 1.61.0 (if past cooldown)`; append to the progress ledger.
  - If pass → Phase 2 (and Phase 6 may start in parallel).
  - If fail → DO NOT PROCEED. Debug within this phase.

---

## PHASE 2: WOTAN ADMIN KEY + CLIENT RESILIENCE (Steps 53-101)

**Goal**: Wotan's admin endpoints work with a key and refuse without one; a Wotan client that started degraded promotes itself back to gRPC; every live service subscribes with retry so a Wotan that comes up late is still heard; the three Wotan server leftovers are fixed. Covers M-030, M-031, M-032, M-038.
**Prerequisite**: Phase 1 gate. Worktree `../unheaded-wt-code` if Step 22's recommendation is in force.
**Time**: 5 h.
**Agent**: Agent [P:3] (code-only; no hosts).

### M-030 admin API key

- [ ] **Step 53** [R] ~2m: How the key is checked and which header carries it.
  ```bash
  cd /home/govan/tmp/unheaded && sed -n '480,500p' services/wotan/internal/api/handlers.go && grep -n 'AdminAPIKey\|X-Admin\|Authorization' services/wotan/internal/api/*_test.go | head -8 && sed -n '410,425p' services/wotan/cmd/wotan/main.go
  ```
- [ ] **Step 54** [DECIDE] ~1m: **Admin default.** RECOMMENDATION: `-admin` defaults to `false`; when `-admin` is set, `WOTAN_ADMIN_API_KEY` must be non-empty or Wotan exits 2 with "WOTAN_ADMIN_API_KEY unset: set it or drop -admin". The key is read from the environment only (never a flag: flags show in `ps`). Rationale: the endpoints have never worked (zero `SetAdminAPIKey` callers), so defaulting them off changes nothing live, and fail-loud beats a silent 401 forever. Override ONLY if a live caller of `/api/v1/admin/*` exists (`grep -rn 'api/v1/admin' --include=*.go --include=*.sh --include=*.py . | grep -v services/wotan` non-empty).
- [ ] **Step 55** [CODE][W] ~10m: Add `services/wotan/cmd/wotan/admin_key_test.go` (package main): (a) `TestAdminKey_RequiredWhenAdminEnabled` builds the config with `AdminEnabled=true` and empty env → the startup validation function returns an error containing `WOTAN_ADMIN_API_KEY`; (b) `TestAdminKey_SetOnServer` with env set → `api.Server.AdminAPIKey` equals the env value; (c) in `services/wotan/internal/api/`, extend the existing admin handler test: request without the header → 401/403, with the header → 200. Factor the validation into `func adminKeyFromEnv(enabled bool) (string, error)` so (a)/(b) test it without starting listeners.
- [ ] **Step 56** [TEST][B] ~1m: Red.
  ```bash
  cd /home/govan/tmp/unheaded && go test ./services/wotan/cmd/wotan/ -run 'TestAdminKey' -count=1 2>&1 | tail -5
  ```
- [ ] **Step 57** [CODE][W] ~10m: `services/wotan/cmd/wotan/main.go`: flag default `false` at `:418`; after `api.NewServer(...)`, `key, err := adminKeyFromEnv(config.AdminEnabled); if err != nil { log.Fatal().Err(err).Msg("admin endpoints") }; if key != "" { s.SetAdminAPIKey(key) }`. `handlers.go:489` keeps refusing on empty key (defence in depth).
- [ ] **Step 58** [TEST][B] ~2m: Green + race.
  ```bash
  cd /home/govan/tmp/unheaded && go test ./services/wotan/... -race -count=1 2>&1 | grep -E '^(ok|FAIL|---)' | head -20
  ```
- [ ] **Step 59** [W] ~5m: Wire the environment without putting a secret in the repo: `docker-compose.yml` wotan service gets `env_file: [/etc/unheaded/wotan/wotan.env]` marked `required: false` and `command:` gains `-admin` only in a commented example; `deploy/systemd/unheaded-wotan.service` gets `EnvironmentFile=-/etc/unheaded/wotan/wotan.env`; add `deploy/systemd/wotan.env.example` with `WOTAN_ADMIN_API_KEY=` (empty) and a comment; `wiki/Service-Wotan.md:50` row: "registered only with `--admin` and `WOTAN_ADMIN_API_KEY` set (default off since S-CLEAN-SLATE)".
- [ ] **Step 60** [V] ~1m: `bash scripts/check-systemd-units.sh 2>&1 | tail -2` green; `bash scripts/check-gitleaks.sh 2>&1 | tail -1` PASS; `grep -c 'WOTAN_ADMIN_API_KEY=' deploy/systemd/wotan.env.example` is 1 and the value is empty; `sudo docker compose -f docker-compose.yml config -q && echo COMPOSE_OK`.
- [ ] **Step 61** [C] ~1m: `[PLAN S-CLEAN-SLATE] Steps 53-60: Wotan admin key read from WOTAN_ADMIN_API_KEY, -admin default off, fail-loud when enabled without a key`.

### M-032 periodic gRPC re-probe

- [ ] **Step 62** [R] ~2m: Existing probe tests and the fake health server they use.
  ```bash
  cd /home/govan/tmp/unheaded && grep -n 'ProbeGRPC\|grpc_health\|health.NewServer\|func Test' pkg/wotan-client/*_test.go | head -20 && sed -n '320,345p' pkg/wotan-client/client.go
  ```
- [ ] **Step 63** [CODE][W] ~10m: `pkg/wotan-client/reprobe_test.go`: `TestReprobe_PromotesToGRPC` — construct the client with a gRPC address nobody listens on (transport HTTP after the failed startup probe), set `c.reprobeInterval = 20 * time.Millisecond` (unexported test hook), start a health server on that address, wait ≤2 s, assert `c.Transport() == TransportGRPC` and `/health`-style `StreamHealth().GRPCHealthy == true`; `TestReprobe_StopsOnClose` — `Close()` returns and the goroutine exits (use a `done` channel the goroutine closes).
- [ ] **Step 64** [TEST][B] ~1m: Red.
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/wotan-client/ -run 'TestReprobe' -count=1 2>&1 | tail -5
  ```
- [ ] **Step 65** [CODE][W] ~10m: `pkg/wotan-client/client.go`: fields `reprobeInterval time.Duration` (default 30 s), `reprobeStop chan struct{}`, `reprobeDone chan struct{}`; in `NewClientWithGRPC`, when the startup probe fails, `go c.reprobeLoop()`; the loop ticks, calls `ProbeGRPC()`, and on success sets `transport = TransportGRPC`, `grpcHealthy = true` under the mutex, logs once, and exits; `Close()` closes `reprobeStop` and waits on `reprobeDone` with a 1 s cap. Update the comment at `:290` to describe the loop instead of asking callers to do it.
- [ ] **Step 66** [TEST][B] ~2m: Green + race on the client and on the two services that already use it.
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/wotan-client/... ./services/timeguru/... ./services/yaldabaoth/... -race -count=1 2>&1 | grep -E '^(ok|FAIL|---)' | head
  ```
- [ ] **Step 67** [C] ~1m: `[PLAN S-CLEAN-SLATE] Steps 62-66: wotan-client re-probes gRPC every 30 s while degraded and promotes itself back`.

### M-031 SubscribeWithRetry in every live service

- [ ] **Step 68** [R] ~5m: Retry semantics and all seven sites.
  ```bash
  cd /home/govan/tmp/unheaded && sed -n '1,60p' pkg/wotan-client/subscribe_retry.go && for f in services/micromanager/service.go services/monad/monad.go services/sophia/sophia.go services/gateway/gateway.go services/architect/core.go services/anomaly/anomaly.go services/loadbalancer/xdpsync.go; do echo "== $f"; grep -n -B6 'degraded mode' $f | grep -E 'if .*(nil|err)|Subscribe\(|degraded' | head -12; done
  ```
  Pattern to apply everywhere: the service constructs its Wotan client unconditionally; every subscribe becomes `SubscribeWithRetry(ctx, topic, name, handler)`; publish paths that drop events when the client is unhealthy keep the warn but no longer need a nil guard; the one-shot "subscriptions disabled" log lines go away. Each service gets a `TestLateWotan_<Service>` that starts the service against a fake Wotan that returns connection refused for the first N calls and succeeds after (reuse `pkg/wotan-client`'s test fake if exported; otherwise an `httptest.Server` whose handler flips on a counter), then asserts the subscription exists.
- [ ] **Step 69** [CODE] ~20m: micromanager (`services/micromanager/service.go:50,107,147,187,224`): test red → implement → test green.
- [ ] **Step 70** [TEST][B] ~2m: `cd /home/govan/tmp/unheaded && go test ./services/micromanager/... -race -count=1 2>&1 | grep -E '^(ok|FAIL|---)'`
- [ ] **Step 71** [V] ~0.5m: ok; `grep -c 'subscriptions disabled\|listener disabled' services/micromanager/service.go` is 0.
- [ ] **Step 72** [CODE] ~15m: monad (`services/monad/monad.go:333,641,685`), same recipe.
- [ ] **Step 73** [TEST][B] ~2m: `cd /home/govan/tmp/unheaded && go test ./services/monad/... -race -count=1 2>&1 | grep -E '^(ok|FAIL|---)'`
- [ ] **Step 74** [V] ~0.5m: ok; no one-shot lines left in monad.go.
- [ ] **Step 75** [C] ~1m: `[PLAN S-CLEAN-SLATE] Steps 68-74: micromanager and monad subscribe with retry (late Wotan test)`.
- [ ] **Step 76** [CODE] ~15m: sophia (`services/sophia/sophia.go:274,895,913`).
- [ ] **Step 77** [TEST][B] ~2m: `cd /home/govan/tmp/unheaded && go test ./services/sophia/... -race -count=1 2>&1 | grep -E '^(ok|FAIL|---)'`
- [ ] **Step 78** [V] ~0.5m: ok; no one-shot lines left.
- [ ] **Step 79** [CODE] ~10m: gateway (`services/gateway/gateway.go:370`).
- [ ] **Step 80** [TEST][B] ~2m: `cd /home/govan/tmp/unheaded && go test ./services/gateway/... -race -count=1 2>&1 | grep -E '^(ok|FAIL|---)'`
- [ ] **Step 81** [V] ~0.5m: ok.
- [ ] **Step 82** [C] ~1m: `[PLAN S-CLEAN-SLATE] Steps 76-81: sophia and gateway subscribe with retry`.
- [ ] **Step 83** [CODE] ~15m: architect (`services/architect/core.go:254,268,303`).
- [ ] **Step 84** [TEST][B] ~2m: `cd /home/govan/tmp/unheaded && go test ./services/architect/... -race -count=1 2>&1 | grep -E '^(ok|FAIL|---)'`
- [ ] **Step 85** [V] ~0.5m: ok.
- [ ] **Step 86** [CODE] ~15m: anomaly (`services/anomaly/anomaly.go:768,882`).
- [ ] **Step 87** [TEST][B] ~2m: `cd /home/govan/tmp/unheaded && go test ./services/anomaly/... ./cmd/anomaly/... -race -count=1 2>&1 | grep -E '^(ok|FAIL|---)'`
- [ ] **Step 88** [V] ~0.5m: ok.
- [ ] **Step 89** [C] ~1m: `[PLAN S-CLEAN-SLATE] Steps 83-88: architect and anomaly subscribe with retry`.
- [ ] **Step 90** [CODE] ~15m: loadbalancer xdpsync (`services/loadbalancer/xdpsync.go:185,688`); keep the f9c59e4b join-before-publish order.
- [ ] **Step 91** [TEST][B] ~2m: `cd /home/govan/tmp/unheaded && go test ./services/loadbalancer/... -race -count=1 2>&1 | grep -E '^(ok|FAIL|---)'`
- [ ] **Step 92** [V] ~1m: ok; repo-wide: `grep -rn 'degraded mode' services/micromanager services/monad services/sophia services/gateway services/architect services/anomaly services/loadbalancer | grep -v _test | grep -c 'disabled'` is 0 (event-dropped warns on publish may remain).

### M-038 Wotan server leftovers

- [ ] **Step 93** [R] ~3m: Rate limiter key, CORS, streams_active.
  ```bash
  cd /home/govan/tmp/unheaded && grep -rn 'RemoteAddr\|Allow-Credentials\|Allow-Origin\|streams_active\|StreamsActive' services/wotan/internal --include=*.go | grep -v _test | head -12
  ```
- [ ] **Step 94** [CODE] ~10m: Rate limiter keys on the client IP: `host, _, err := net.SplitHostPort(r.RemoteAddr)`; on error fall back to the raw string. Test: two requests from `1.2.3.4:1000` and `1.2.3.4:1001` share one bucket.
- [ ] **Step 95** [TEST][B] ~2m: `cd /home/govan/tmp/unheaded && go test ./services/wotan/internal/middleware/... -race -count=1 2>&1 | grep -E '^(ok|FAIL|---)'`
- [ ] **Step 96** [CODE] ~10m: CORS: when the configured allow-list is `*`, respond `Access-Control-Allow-Origin: *` and never `Allow-Credentials: true`; when it is a list, echo only a matching origin and allow credentials. Test both.
- [ ] **Step 97** [TEST][B] ~2m: same command as Step 95.
- [ ] **Step 98** [CODE] ~10m: `streams_active` (metrics/`/health`) counts topic streams as well as room streams; test increments on a topic stream open and decrements on close.
- [ ] **Step 99** [TEST][B] ~3m: `cd /home/govan/tmp/unheaded && go test ./services/wotan/... -race -count=1 2>&1 | grep -E '^(ok|FAIL|---)' | head -20`
- [ ] **Step 100** [C] ~1m: `[PLAN S-CLEAN-SLATE] Steps 90-99: loadbalancer retry; Wotan rate limiter by IP, CORS credentials, streams_active topic streams`.
- [ ] **Step 101** [V] ~5m: **PHASE 2 EXIT GATE** — `go build ./... && go test ./pkg/wotan-client/... ./services/wotan/... ./services/micromanager/... ./services/monad/... ./services/sophia/... ./services/gateway/... ./services/architect/... ./services/anomaly/... ./services/loadbalancer/... -race -count=1 2>&1 | grep -c '^ok'` equals the number of packages listed with tests; `grep -c '^FAIL'` is 0; Step 17's counters now read: callers 1, ticker ≥1, degraded "disabled" lines 0. If in a worktree: `git merge --no-ff clean-slate/code` into develop, then re-run `go build ./...`.
  - If pass → Phase 3.
  - If fail → DO NOT PROCEED. Debug within this phase.

---

## PHASE 3: LIVE-SERVICE FIXES II: HAPROXY, ZHEN GATE, FIT CHECK, DEFAULTS (Steps 102-146)

**Goal**: Both haproxy configs parse under the pinned image and a check script guards them; the Zhen web UI only reaches the model through zhen-agentd's gate; `switch-model.sh` counts llama-server's KV cache RAM; no shipped default credential; probes stay on the lxdbr0 subnet; zhen-agentd refuses to start unauthenticated; routed local turns are captured. Covers M-035, M-036, M-037, M-039, M-040, M-019, M-042.
**Prerequisite**: Phase 1 gate (runs in parallel with Phase 2).
**Time**: 5 h.
**Agent**: Agent [P:2].

### M-035 haproxy configs

- [ ] **Step 102** [W] ~10m: `scripts/check-haproxy-cfg.sh`: for each of `docker/haproxy/internal/haproxy.cfg` and `docker/haproxy/edge/haproxy.cfg`, (1) if `haproxy` is on PATH run `haproxy -c -f <cfg>`; (2) always run the pinned image: `sudo docker run --rm -v "$PWD/<cfg>":/usr/local/etc/haproxy/haproxy.cfg:ro -v "$CERTDIR":/etc/haproxy/certs:ro haproxy:2.9.15-alpine haproxy -c -f /usr/local/etc/haproxy/haproxy.cfg`, where `$CERTDIR` is a mktemp dir holding `dev.pem` generated with `openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -days 1 -subj /CN=haproxy-dev -keyout - -out -` concatenated. Exit 1 on any failure; print PASS/FAIL per config. `set -euo pipefail`; `# SPDX-License-Identifier: GPL-3.0-or-later` header.
- [ ] **Step 103** [TEST][B] ~2m: Red (both FAIL today).
  ```bash
  cd /home/govan/tmp/unheaded && shellcheck scripts/check-haproxy-cfg.sh && bash scripts/check-haproxy-cfg.sh 2>&1 | tail -6; echo "rc=${PIPESTATUS[0]}"
  ```
- [ ] **Step 104** [R] ~2m: Section map of the internal config.
  ```bash
  cd /home/govan/tmp/unheaded && grep -nE '^(global|defaults|frontend|backend|listen)\b|^\s+mode\s' docker/haproxy/internal/haproxy.cfg
  ```
- [ ] **Step 105** [W] ~5m: Internal: delete the `log-format` line from `defaults` (`:36`) and place it, unchanged, inside every `frontend`/`listen` whose `mode` is `http` (after its `mode http` line). Sections without `mode http` keep the default TCP log format.
- [ ] **Step 106** [R] ~2m: Section map of the edge config (same command on `docker/haproxy/edge/haproxy.cfg`).
- [ ] **Step 107** [W] ~8m: Edge: remove `module-path /usr/lib/haproxy` (`:30`); if a `http-request use-service prometheus-exporter` already exists keep it, else leave metrics to the stats listener; move `http-request redirect scheme https code 301 unless is_acme` (`:94`) above the first `use_backend`; move the `log-format` from `defaults` into the HTTP frontends as in Step 105; keep `bind *:21443 ssl crt /etc/haproxy/certs/ alpn h2,http/1.1` and document that the directory is mounted (next step).
- [ ] **Step 108** [W] ~6m: `docker/docker-compose.loadbalancers.yml`: edge service mounts `./docker/haproxy/edge/certs:/etc/haproxy/certs:ro`; add `docker/haproxy/edge/certs/.gitignore` (`*.pem`) and `docker/haproxy/edge/certs/README.md` (one paragraph: dev cert via `scripts/gen-haproxy-dev-cert.sh`, production certs from the gateway's ACME path; never commit a key); write `scripts/gen-haproxy-dev-cert.sh` (the openssl line from Step 102, output `docker/haproxy/edge/certs/dev.pem`, 0600); replace the `KNOWN BROKEN` comment (`:12`) with "Validated by scripts/check-haproxy-cfg.sh (S-CLEAN-SLATE Step 109)".
- [ ] **Step 109** [TEST][B] ~2m: Green.
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/check-haproxy-cfg.sh 2>&1 | tail -6; echo "rc=${PIPESTATUS[0]}"
  ```
- [ ] **Step 110** [V] ~0.5m: rc=0, both PASS under the host binary and the 2.9.15 image. Warnings about `option httplog` overridden are acceptable only if zero `[ALERT]` lines remain (`bash scripts/check-haproxy-cfg.sh 2>&1 | grep -c ALERT` is 0).
  - If the edge still alerts on the cert → Step 111.
- [ ] **Step 111** [D] ~5m: The mounted dir must contain at least one `.pem` with cert+key; confirm the check script generated it into `$CERTDIR` and that the compose mount path matches the `crt` path exactly. Max 2 attempts, then STUCK.
- [ ] **Step 112** [B] ~1m: Compose file still renders.
  ```bash
  cd /home/govan/tmp/unheaded && sudo docker compose -f docker/docker-compose.loadbalancers.yml config -q && echo LB_COMPOSE_OK; bash scripts/check-compose-hardening.sh 2>&1 | tail -2
  ```
- [ ] **Step 113** [W] ~3m: Add `scripts/check-haproxy-cfg.sh` to the local gate list in `scripts/ci-local.sh` (same block as the other `check-*.sh`) and to the Makefile `lint` target's script list; add one row to `wiki/Security-Tooling.md` (or the page that lists gates) if such a table exists.
- [ ] **Step 114** [V] ~1m: `grep -c check-haproxy-cfg scripts/ci-local.sh Makefile` ≥ 2; `make -n lint 2>/dev/null | grep -c check-haproxy-cfg` is 1.
- [ ] **Step 115** [C] ~1m: `[PLAN S-CLEAN-SLATE] Steps 102-114: both haproxy configs parse under 2.9.15 and 3.0; check-haproxy-cfg.sh gate`.

### M-036 Zhen web UI through the Champion gate

- [ ] **Step 116** [R] ~5m: How `proxy_via_agentd` is consumed, how `zhen_app.py` constructs the pipeline, and where the raft tests live.
  ```bash
  cd /home/govan/tmp/unheaded && grep -n 'proxy_via_agentd\|agentd_url\|/api/v1/agent/ask' raft/scripts/zhen_rag.py raft/zhen_app.py | head -20 && ls raft/tests | head -20 && grep -rn 'def test' raft/tests/test_zhen_rag*.py 2>/dev/null | wc -l
  ```
- [ ] **Step 117** [CODE] ~20m: In `raft/scripts/zhen_rag.py`, `proxy_via_agentd` defaults to `os.environ.get("ZHEN_PROXY_VIA_AGENTD", "1") == "1"`; when true, the ask path POSTs to `{agentd_url}/api/v1/agent/ask` (and `/stream` for streaming) and never falls back to the direct llama-server URL: an unreachable agentd is a 503 to the UI with the message "zhen-agentd not reachable; the gate is required", logged once. `zhen_app.py` passes the env-derived value explicitly. Tests in `raft/tests/`: (a) default is proxy; (b) `ZHEN_PROXY_VIA_AGENTD=0` restores direct mode; (c) agentd down → 503, no direct call (mock `requests`).
- [ ] **Step 118** [TEST][B] ~3m: `cd /home/govan/tmp/unheaded/raft && source ~/.venv/zhen/bin/activate && python3 -m pytest tests -q -k 'rag or agentd or proxy' 2>&1 | tail -4`
- [ ] **Step 119** [V] ~0.5m: all pass; `grep -c 'proxy_via_agentd=None' raft/scripts/zhen_rag.py` is 0.
- [ ] **Step 120** [CODE] ~15m: CSRF interim (threat model T8): `zhen_app.py` issues a per-session random token (`secrets.token_urlsafe(32)`) in a `SameSite=Strict` cookie and in the page; every state-changing POST (`/api/v1/ask`, `/api/v1/highlight` excluded as idempotent? no: all POSTs) requires header `X-Zhen-CSRF` equal to the cookie; test: POST without header → 403, with header → passes to the handler.
- [ ] **Step 121** [TEST][B] ~2m: same as Step 118 with `-k csrf`.
- [ ] **Step 122** [V] ~0.5m: pass; `raft/README` or `wiki/Zhen-AI.md` gains two lines (gate required, CSRF header) in Phase 8.
- [ ] **Step 123** [DECIDE][LIVE-RO] ~15m: **Coding-gate re-run.** RECOMMENDATION: if `curl -s --max-time 2 http://127.0.0.1:8081/health` answers (a model is already serving), run `bash scripts/coding-gate-probe.sh 2>&1 | tail -8` through the proxied UI path and record the result in the progress file; otherwise defer the re-run to the E2 window (Step 155b) rather than starting a model for it (memory: feedback_resource_awareness). Override ONLY if Stevie asks for the number now.
- [ ] **Step 124** [C] ~1m: `[PLAN S-CLEAN-SLATE] Steps 116-123: Zhen web UI goes through zhen-agentd (gate required, no direct fallback), CSRF token on POSTs`.

### M-037 switch-model fit check

- [ ] **Step 125** [R] ~3m: The fit arithmetic and per-model launch args.
  ```bash
  cd /home/govan/tmp/unheaded && sed -n '170,240p' scripts/switch-model.sh | grep -nE 'need|avail|ctx|MemAvailable|llama-server|--' | head -30
  ```
- [ ] **Step 126** [CODE][W] ~15m: Add `--cache-ram` to every model's launch args explicitly (per-model MiB; default 8192 if unspecified, matching llama-server), parse it into the fit sum (`need_mib = model_mib + ctx_mib + cache_ram_mib + margin`), and print the breakdown in `--check`. Swap stays refused when the sum exceeds MemAvailable (never swap: memory feedback_no_oom_from_ui_actions). Add a `tests/scripts/switch-model-fit.bats`-style check if a bats harness exists (`ls tests/scripts 2>/dev/null`); otherwise a `scripts/switch-model.sh --check qwen-7b` run whose output includes `cache_ram_mib=`.
- [ ] **Step 127** [TEST][B] ~1m: `cd /home/govan/tmp/unheaded && bash -n scripts/switch-model.sh && shellcheck scripts/switch-model.sh | tail -3; bash scripts/switch-model.sh --check qwen-7b 2>&1 | tail -6`
- [ ] **Step 128** [V] ~0.5m: shellcheck clean (or only pre-existing baselined codes), `--check` prints `cache_ram_mib=` and a verdict; the verdict for qwen-7b matches reality on this box (fits).

### M-040 shipped default credential

- [ ] **Step 129** [DECIDE] ~1m: **`core.trust_password`.** RECOMMENDATION: delete the `core.trust_password` line from the preseed entirely; LXD 5.21 uses trust tokens (`scripts/cuirass-lxd-trust.sh` already does). Rationale: a password default is the bug; an unset-guard would keep a deprecated mechanism alive. Override ONLY if `lxd --version` on west is < 5.21.
- [ ] **Step 130** [W] ~2m: Edit `scripts/setup-host.sh:222` accordingly; add a comment pointing at `scripts/cuirass-lxd-trust.sh`.
- [ ] **Step 131** [B] ~1m: `cd /home/govan/tmp/unheaded && bash -n scripts/setup-host.sh && shellcheck scripts/setup-host.sh | tail -3; grep -c 'changeme' scripts/setup-host.sh; grep -rn 'changeme\|password123\|:-secret' scripts/*.sh docker/*.yml docker-compose*.yml | grep -v '#' | head`
- [ ] **Step 132** [V] ~0.5m: `changeme` count 0; the repo-wide grep prints nothing new (anything it prints becomes a new M-item line in the progress file).

### M-039 probe addresses on lxdbr0 only

- [ ] **Step 133** [R] ~3m: Where instance-reported addresses enter probe URLs.
  ```bash
  cd /home/govan/tmp/unheaded && grep -rn 'Addresses\|\.Address\b\|probeURL\|http://%s' pkg/deploy/cuirassdriver/*.go cmd/unheaded-daemon/internal/*/*.go services/yaldabaoth/*.go 2>/dev/null | grep -v _test | head -12 && grep -n 'lxdbr0\|10.10.10' configs/chaos/allowlist.yaml docker/compose.cuirass-lxd.yml | head
  ```
- [ ] **Step 134** [CODE] ~20m: Add `probe_subnets` (list of CIDRs, default `["10.10.10.0/24"]`, env `CUIRASS_PROBE_SUBNETS` / allowlist key) to Cuirass's instance-probe config and to `configs/chaos/allowlist.yaml`; any instance-reported address outside the list is rejected with a logged reason and the instance counted unhealthy, never probed. Tests: in-subnet address probed; `192.168.13.1` (the P2P peer) rejected; IPv6 link-local rejected.
- [ ] **Step 135** [TEST][B] ~2m: `cd /home/govan/tmp/unheaded && go test ./pkg/deploy/... ./cmd/unheaded-daemon/... ./services/yaldabaoth/... -race -count=1 2>&1 | grep -E '^(ok|FAIL|---)'`
- [ ] **Step 136** [V] ~0.5m: ok; `configs/chaos/allowlist.yaml` passes `bash scripts/check-manifest-yaml.sh 2>&1 | tail -1`.

### M-019 zhen-agentd refuses to start unauthenticated

- [ ] **Step 137** [R] ~3m: The auth mechanism in place.
  ```bash
  cd /home/govan/tmp/unheaded && grep -n 'AGENTD_TOKEN\|auth\|Bearer\|apiKey' cmd/zhen-agentd/main.go cmd/zhen-agentd/server.go 2>/dev/null | head -12 && sed -n '136,160p' cmd/zhen-agentd/toolexec.go
  ```
- [ ] **Step 138** [CODE] ~20m: (a) `-no-auth` bool flag (default false); if the configured token env is empty and `-no-auth` is not set, exit 2 with "zhen-agentd: no auth token set; set <ENV> or pass -no-auth for a loopback dev listener"; `-no-auth` with `-host` other than `127.0.0.1` is refused. (b) `-rate-limit` default `5`. (c) `toolexec.go:140-152`: a request without `justification` is denied with "justification required" instead of a synthesised `direct-user`; the web UI (Step 117) and `zhen-claude` send it explicitly. Update `ask_tokens_test.go` / `integration_test.go` fixtures to pass `-no-auth` and a justification. Tests for (a), (b), (c).
- [ ] **Step 139** [TEST][B] ~3m: `cd /home/govan/tmp/unheaded && go test ./cmd/zhen-agentd/... ./pkg/champion/... -race -count=1 2>&1 | grep -E '^(ok|FAIL|---)'`
- [ ] **Step 140** [V] ~1m: ok; `deploy/systemd/zhen-agentd.service` + `zhen-agentd.env.example` document the token env (no value); `raft/start-zhen.sh` passes it from the env it already loads.

### M-042 routed local turns captured

- [ ] **Step 141** [R] ~2m: The capture writer used for the Claude route.
  ```bash
  cd /home/govan/tmp/unheaded && grep -n 'capture\|Capture' cmd/zhen-claude/*.go | grep -v _test | head -10 && grep -n 'func runRouted' -A30 cmd/zhen-claude/routed.go | grep -n 'local\|Local' | head
  ```
- [ ] **Step 142** [CODE] ~15m: In `runRouted`, when the route resolves to `local`, write the same redacted capture record the Claude leg writes (same privacy tier, `route: "local"`, family, worktree), so the Phase 12 data trigger counts them. Test: `TestRouted_LocalTurnCaptured` with a fake local backend and a temp capture dir → exactly one record with `route=local`.
- [ ] **Step 143** [TEST][B] ~2m: `cd /home/govan/tmp/unheaded && go test ./cmd/zhen-claude/... -race -count=1 2>&1 | grep -E '^(ok|FAIL|---)'`
- [ ] **Step 144** [V] ~0.5m: ok; `-include-claude-text` default remains OFF (`grep -n 'include-claude-text' cmd/zhen-flywheel/*.go | grep -c 'false'` ≥ 1).
- [ ] **Step 145** [C] ~1m: `[PLAN S-CLEAN-SLATE] Steps 125-144: switch-model counts cache-ram; LXD preseed has no password; probes confined to lxdbr0; zhen-agentd requires auth; local turns captured`.
- [ ] **Step 146** [V] ~5m: **PHASE 3 EXIT GATE** — `bash scripts/check-haproxy-cfg.sh` rc 0; raft pytest green; `go test ./cmd/zhen-agentd/... ./cmd/zhen-claude/... ./pkg/deploy/... ./services/yaldabaoth/... -race -count=1` all ok; `shellcheck scripts/switch-model.sh scripts/setup-host.sh scripts/check-haproxy-cfg.sh scripts/gen-haproxy-dev-cert.sh` clean; `bash scripts/check-gitleaks.sh | tail -1` PASS. If in a worktree: merge `clean-slate/code` into develop.
  - If pass → Phase 4.
  - If fail → DO NOT PROCEED.

---

## PHASE 4: FND-024, FND-023, EAST BUNDLE (Steps 147-176)

**Goal**: The monitoring stack starts with the kingdom so the SLO guard has series; the discover meter drops a dead replica immediately; the rebuilt debs and the wotan.yaml auto-approve fix are ready for the east/west window. Covers M-033, M-034, M-041, M-020 (install half).
**Prerequisite**: Phases 2 and 3 merged; Stevie's E2 and E3 windows for the live halves.
**Time**: 3 h code + two live windows (~1 h each).
**Agent**: Coordinator (hosts).

### M-033 FND-024

- [ ] **Step 147** [DECIDE] ~1m: **Where monitoring starts.** RECOMMENDATION: `scripts/kingdom-startup.sh` gains Phase 1b that runs `docker compose -f monitoring/docker-compose.yml up -d` after Phase 1 and waits ≤ 180 s for `slo:error_ratio:5m` to exist. Rationale: decision 13 default; the stacks stay separable (`kingdom-shutdown.sh` already excludes monitoring on purpose, 168410a9). Override ONLY if Stevie answers "fold into main compose".
- [ ] **Step 148** [W] ~10m: Insert after Phase 1 in `scripts/kingdom-startup.sh` (respecting `$NATIVE_ONLY`, `$DRY_RUN` via `run`, `$REPO_ROOT`):
  ```bash
  # ── Phase 1b: Monitoring stack (FND-024: the SLO guard fails closed without it) ──
  MON_COMPOSE="${REPO_ROOT}/monitoring/docker-compose.yml"
  if [[ "$NATIVE_ONLY" != "true" && -f "$MON_COMPOSE" ]]; then
      log_info "Phase 1b: Starting monitoring stack..."
      run docker compose -f "$MON_COMPOSE" up -d 2>&1 | grep -E "Started|Running|Created" | while read -r line; do log_info "  $line"; done || true
      if [[ "$DRY_RUN" != "true" ]]; then
          for _ in $(seq 1 36); do
              n=$(curl -s --max-time 3 'http://127.0.0.1:9090/api/v1/query?query=slo:error_ratio:5m' | jq -r '.data.result | length' 2>/dev/null || echo 0)
              [[ "${n:-0}" -gt 0 ]] && break; sleep 5
          done
          [[ "${n:-0}" -gt 0 ]] && log_success "Phase 1b complete (slo:error_ratio:5m has $n series)" || log_warn "Phase 1b: slo:error_ratio:5m still empty after 180 s (rules evaluate after the first scrapes; the SLO guard will refuse until then)"
      fi
      echo ""
  fi
  ```
  Confirm the Prometheus port from `wiki/Port-Registry.md:36` (9090) and that `REPO_ROOT` is defined above Phase 1 (else derive it from `$0`).
- [ ] **Step 149** [B] ~1m: Lint and dry-run.
  ```bash
  cd /home/govan/tmp/unheaded && shellcheck scripts/kingdom-startup.sh | tail -3; bash scripts/kingdom-startup.sh --dry-run --docker-only 2>&1 | grep -E 'Phase 1b|monitoring/docker-compose' | head -3
  ```
- [ ] **Step 150** [V] ~0.5m: shellcheck clean; dry-run prints the Phase 1b compose command.
- [ ] **Step 151** [DOC-UPDATE][W] ~3m: `wiki/Containers.md` bring-up recipe and `wiki/Chaos-Engineering.md:81` FND-024 bullet: "started by `kingdom-startup.sh` Phase 1b since S-CLEAN-SLATE; the register row closes when Step 154 verifies it live".
- [ ] **Step 152** [ESCALATE] **E2** ~0m: Context: Steps 153-157, 164-166 and 155b change west (start stacks, run a chaos experiment, enable a user timer). Question: grant a west live window (date/time), same scope as S-REAL-STEEL E2. Why no recommendation: host-changing.
- [ ] **Step 153** [AFTER-E2][LIVE-RW][BARE-METAL][S][B] ~5m: `cd /home/govan/tmp/unheaded && sudo bash scripts/kingdom-startup.sh --force --docker-only 2>&1 | tail -15`
- [ ] **Step 154** [AFTER-E2][LIVE-RO][V] ~3m: `curl -s 'http://127.0.0.1:9090/api/v1/query?query=slo:error_ratio:5m' | jq '.data.result|length'` > 0 within 180 s of the stack starting; `sudo docker compose -f monitoring/docker-compose.yml ps --format '{{.Name}} {{.Status}}' | grep -c healthy` equals the service count.
- [ ] **Step 155** [AFTER-E2][LIVE-RW][S][B] ~5m: haproxy pair live once: `sudo docker compose -f docker/docker-compose.loadbalancers.yml up -d 2>&1 | tail -3 && sleep 5 && curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:21405/ ; curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:21404/` (stats ports from `pkg/ports`); then `sudo docker compose -f docker/docker-compose.loadbalancers.yml down`. Record both codes.
- [ ] **Step 155b** [AFTER-E2][LIVE-RO][B] ~15m: If Step 123 deferred it: `bash scripts/switch-model.sh --check qwen-7b && bash scripts/switch-model.sh qwen-7b && bash scripts/coding-gate-probe.sh 2>&1 | tail -8`; record the gate result; `bash scripts/switch-model.sh --stop` if the model was not running before.
- [ ] **Step 156** [AFTER-E2][LIVE-RW][B] ~3m: FND-005 mirror timer (user units, no sudo): `mkdir -p ~/.config/systemd/user && cp deploy/systemd/user/unheaded-git-mirror.{service,timer} ~/.config/systemd/user/ && systemctl --user daemon-reload && systemctl --user enable --now unheaded-git-mirror.timer && systemctl --user list-timers unheaded-git-mirror.timer --no-pager | head -3 && bash scripts/backup/test-git-mirror.sh 2>&1 | tail -3`
- [ ] **Step 157** [AFTER-E2][AUDIT-UPDATE][W] ~5m: `compliance/findings/register.yaml` FND-024: add `resolved_on: <date>` + `evidence: S-CLEAN-SLATE Steps 153-154 (<n> series after <s> s)` in the row's existing format (read the FND-021/022 closed rows 06280b2d for the shape); FND-005 likewise with Step 156's timer listing. Regenerate the ADR-098 section in Phase 8.
- [ ] **Step 158** [C] ~1m: `[PLAN S-CLEAN-SLATE] Steps 147-157: kingdom-startup Phase 1b starts monitoring and waits for slo:error_ratio:5m (FND-024); git-mirror timer; wiki`.

### M-034 FND-023

- [ ] **Step 159** [DECIDE] ~1m: **Dead-replica detection.** RECOMMENDATION: the client drops a target on its first connection error and re-admits it only when a later discover refresh lists it AND a direct probe succeeds. Rationale: decision 14 default; no new Wotan topic; any discover-mode client gets the same fix. Override ONLY if Stevie answers "Cuirass publishes health transitions".
- [ ] **Step 160** [R] ~3m: `cd /home/govan/tmp/unheaded && grep -n 'refresh\|targets\|discover' cmd/steel-echo/load.go | head -30`
- [ ] **Step 161** [CODE] ~20m: `cmd/steel-echo/load.go`: per-target state `{addr, dead bool, deadSince}`; on `net.OpError`/connection refused/timeout mark dead and skip it in the round-robin; on each refresh, for a dead target that the list still reports healthy, `GET /health` once; success re-admits. Test `TestDiscover_DropsDeadTargetOnFirstError`: three `httptest.Server`s, close one mid-run, assert no further request reaches it and that the meter's availability over the next 100 requests is 1.0; `TestDiscover_ReadmitsAfterProbe`.
- [ ] **Step 162** [TEST][B] ~2m: `cd /home/govan/tmp/unheaded && go test ./cmd/steel-echo/... -race -count=1 2>&1 | grep -E '^(ok|FAIL|---)'`
- [ ] **Step 163** [V] ~0.5m: ok.
- [ ] **Step 164** [AFTER-E2][LIVE-RW][BARE-METAL][S][B] ~20m: Re-run the Step 310 experiment exactly as `wiki/Chaos-Engineering.md` records it (three steel-echo replicas, 20 rps discover meter from the rebuilt `cmd/steel-echo`, `container_kill` on one replica, `CHAOS_DRY_RUN=false` only for the window), capture the meter's availability and MTTR lines.
- [ ] **Step 165** [AFTER-E2][V] ~1m: availability ≥ 0.99 over the experiment window; MTTR within the FND-023 row's expectation (unchanged). Then `CHAOS_DRY_RUN=true` restored and the controller restarted (window off).
  - If < 0.99 → one [D] attempt: lower `-refresh` to 1 s and repeat; if still < 0.99, leave FND-023 open with the new numbers (not STUCK: the register row stays).
- [ ] **Step 166** [AFTER-E2][AUDIT-UPDATE][W] ~3m: FND-023 row: `resolved_on` + evidence (Steps 164-165 numbers).
- [ ] **Step 167** [C] ~1m: `[PLAN S-CLEAN-SLATE] Steps 159-166: discover meter drops a dead replica on first error (FND-023)`.

### M-041 east bundle

- [ ] **Step 168** [W] ~2m: `configs/wotan.yaml` `topics.auto_approve`: add `chaos-controller` (east's controller publishes `chaos.refused`; next.md S23) and `unheaded-baseline` (Phase 5 Step 185). One comment each.
- [ ] **Step 169** [B] ~2m: `cd /home/govan/tmp/unheaded && bash scripts/check-manifest-yaml.sh 2>&1 | tail -1 && go test ./services/wotan/... -run 'Config|Topic' -count=1 2>&1 | grep -E '^(ok|FAIL)'`
- [ ] **Step 170** [BUILD][B] ~10m: `cd /home/govan/tmp/unheaded && bash scripts/build-all-debs.sh 2>&1 | tail -8 && ls -1 dist/*.deb 2>/dev/null | wc -l`
- [ ] **Step 171** [V] ~2m: deb count ≥ the count in `scripts/build-all-debs.sh`'s package list; `bash scripts/test-deb-maintscripts.sh 2>&1 | tail -3` passes; `bash scripts/check-unit-binaries.sh 2>&1 | tail -1` passes.
- [ ] **Step 172** [ESCALATE] **E3** ~0m: Context: east runs older debs (huginn rename, chaos-controller unit, wotan.yaml without `chaos-controller`); west's installed debs predate Phases 2-3. Question: grant a deploy window on east then west (install `dist/*.deb`, restart units, update the huginn baseline). Why no recommendation: host-changing, both hosts.
- [ ] **Step 173** [AFTER-E3][LIVE-RW][BARE-METAL][S][B] ~20m: Follow `wiki/East-Deploy.md` (or the recipe in `scripts/east-bootstrap.sh` header) : `scp dist/*.deb govan@east:/tmp/debs/ && ssh govan@east 'sudo dpkg -i /tmp/debs/*.deb && sudo systemctl daemon-reload && sudo systemctl restart unheaded-wotan unheaded-timeguru unheaded-chaos-controller huginn'`; then the same on west for the units west runs.
- [ ] **Step 174** [AFTER-E3][LIVE-RO][V] ~5m: `ssh govan@east 'dpkg -l | grep unheaded | awk "{print \$2, \$3}"; systemctl is-active unheaded-wotan unheaded-chaos-controller huginn'` shows the new versions and `active`; huginn baseline updated per `wiki/Huginn.md` (`huginn baseline --write` or the documented command) and the ADR-098 `huginn-baseline` row clears on the next regen.
- [ ] **Step 175** [C] ~1m: `[PLAN S-CLEAN-SLATE] Steps 168-174: wotan.yaml auto-approves chaos-controller and unheaded-baseline; debs rebuilt (deployed under E3)`.
- [ ] **Step 176** [V] ~3m: **PHASE 4 EXIT GATE** — code half: Steps 150, 163, 169, 171 passed and committed. Live half: either Steps 154, 165, 174 passed, or E2/E3 are logged as open escalations in the progress file with their [AFTER-E] step numbers. `go build ./...` clean.
  - If pass → Phase 5.
  - If fail → DO NOT PROCEED.

---

## PHASE 5: PUBLIC-REPO SECURITY: CODE NOW, HOSTS AND CREDENTIALS BEHIND ESCALATIONS (Steps 177-220)

**Goal**: Everything in E02 that a repo change can do is done (security.txt, README/SECURITY posture, baseline deviation publish, FND-011 stopgap entry, alert-mailer wiring, export-control notice); every host or credential action is prepared to one command and parked behind its escalation. Covers M-006, M-010, M-011, M-012, M-013, M-014, M-015, M-016, M-017, M-018, M-021.
**Prerequisite**: Phase 1 gate (independent of Phases 2-4; may run in the code worktree after Phase 3).
**Time**: 3 h code; the [AFTER-E] steps add ~2 h across windows.
**Agent**: Agent for Steps 177-191, 197-198, 203, 211, 216; Coordinator for every [LIVE-RW].

### M-018 security.txt + posture

- [ ] **Step 177** [R] ~2m: Contact lines and the embed directive.
  ```bash
  cd /home/govan/tmp/unheaded && sed -n '12,30p' SECURITY.md | grep -iE 'mail|contact|pgp|http' ; grep -rn 'go:embed' cmd/dashboard-backend/ | head -3; grep -n 'staticHandler\|HandleFunc("/' cmd/dashboard-backend/internal/server/server.go | head -12
  ```
- [ ] **Step 178** [CODE][W] ~15m: `dashboard/security.txt` (RFC 9116): `Contact: mailto:<address from SECURITY.md>`, `Expires: 2027-10-27T00:00:00.000Z`, `Preferred-Languages: en`, `Canonical: https://unheaded.org/.well-known/security.txt`, `Policy: https://github.com/unheaded/unheaded/blob/develop/SECURITY.md`. In `server.go` register `GET /.well-known/security.txt` → serve that embedded file with `text/plain; charset=utf-8` (the embed pattern must include it; `all:` prefix if it lives under a dot path, which it does not). Test `TestSecurityTxt_Served` (200, content type, contains `Contact:`), and a test that `Expires` is in the future (so the file cannot silently rot).
- [ ] **Step 179** [TEST][B] ~2m: `cd /home/govan/tmp/unheaded && go test ./cmd/dashboard-backend/... -run 'SecurityTxt' -race -count=1 2>&1 | grep -E '^(ok|FAIL|---)'`
- [ ] **Step 180** [V] ~0.5m: ok.
- [ ] **Step 181** [W] ~10m: `README.md`: a `## Security` section before `## License`: what runs on every push (gitleaks + shrink-only baseline, gosec ratchet, dependency baseline that may only shrink, SBOM + grype, GPL boundary, pinned-tool freshness, runbook schema, BPF verifier budgets), where findings live (ADR-098 register), how to report (SECURITY.md, `/.well-known/security.txt`). Plain sentences, no taglines, no counts that will rot.
- [ ] **Step 182** [W] ~5m: `SECURITY.md` "Supported Versions": `develop` (all fixes land here first), `staging` (promoted batches), `main` (promoted by Stevie); link the security.txt; keep the rest.
- [ ] **Step 183** [V] ~1m: `grep -c '^## Security' README.md` is 1; `grep -c 'security.txt' README.md SECURITY.md` ≥ 2; `grep -ciE 'in hours|moat|revolutionary' README.md` is 0.
- [ ] **Step 184** [C] ~1m: `[PLAN S-CLEAN-SLATE] Steps 177-183: /.well-known/security.txt served; README security section; SECURITY.md versions`.

### M-021 baseline publishes deviations

- [ ] **Step 185** [CODE][W] ~20m: `cmd/unheaded-baseline`: flag `-wotan-addr` (default empty = off; env `WOTAN_ADDR` honoured); after `report.json` is written, if `-wotan-addr` is set and deviations > 0, publish one message on `compliance.deviation.<hostname>` via `pkg/wotan-client` (HTTP control plane, display name `unheaded-baseline`): `{host, mode, deviations:[{id, severity}], report_sha256, ts}`; metadata only, no file contents. Test with an `httptest.Server` standing in for Wotan: one publish with N deviations, zero publishes when clean, no publish when the flag is empty. Add the topic to `docs/protocol/WOTAN_TOPICS.md` (or the file that lists topics; `grep -rl 'compliance\.' docs/protocol | head -1`).
- [ ] **Step 186** [TEST][B] ~2m: `cd /home/govan/tmp/unheaded && go test ./cmd/unheaded-baseline/... -race -count=1 2>&1 | grep -E '^(ok|FAIL|---)'`
- [ ] **Step 187** [V] ~1m: ok; `deploy/systemd/unheaded-baseline.service` gains `Environment=WOTAN_ADDR=127.0.0.1:18000` only as a commented example (hosts set it in `/etc/unheaded/baseline.conf`'s env file); `bash scripts/check-systemd-units.sh | tail -1` green.
- [ ] **Step 188** [DECIDE] ~1m: **Baseline packaging.** RECOMMENDATION: units-only for now (`grep -rn 'unheaded-baseline' debian/ scripts/build-all-debs.sh | wc -l` is 0 today); the deb lands with the release-pipeline sprint (S-IRON-GATE, M-062). Rationale: packaging belongs with signing and the apt repo. Override ONLY if the grep shows a package already exists.
- [ ] **Step 189** [C] ~1m: `[PLAN S-CLEAN-SLATE] Steps 185-188: unheaded-baseline publishes compliance.deviation.<host> when -wotan-addr is set`.

### M-012 FND-011 stopgap

- [ ] **Step 190** [R] ~2m: `cd /home/govan/tmp/unheaded && grep -n 'Usage\|dry-run\|apply\|kmod-cis' scripts/host/remediate.sh | head -12; sed -n '92,110p' compliance/remediation/fixes.yaml`
- [ ] **Step 191** [W] ~5m: `compliance/remediation/fixes.yaml` kmod-cis part: append `install ebtables /bin/false`, `install ebtable_nat /bin/false`, `install ebt_snat /bin/false` with a comment `# FND-011 stopgap (CVE-2026-53266): nothing here uses ebtables`; `compliance/findings/register.yaml` FND-011 `plan`: "stopgap in fixes.yaml kmod-cis since S-CLEAN-SLATE Step 191; applied under E4".
- [ ] **Step 192** [B] ~2m: `cd /home/govan/tmp/unheaded && bash scripts/check-manifest-yaml.sh 2>&1 | tail -1 && sudo bash scripts/host/remediate.sh --dry-run apply kmod-cis 2>&1 | tail -8`
- [ ] **Step 193** [V] ~0.5m: dry-run shows the three `install` lines would be written to `/etc/modprobe.d/unheaded-cis.conf` and nothing else changes.
- [ ] **Step 194** [ESCALATE] **E4** ~0m: Context: FND-011 high, due 2026-11-18; two KEV kernel CVEs; no Ubuntu kernel yet. Question: apply `kmod-cis` on east then west (writes `/etc/modprobe.d/unheaded-cis.conf`)? Keep west's unprivileged userns open for LXD until tested? Why the call is Stevie's: host change on both hosts (default per decision 9: yes / keep open).
- [ ] **Step 195** [AFTER-E4][LIVE-RW][BARE-METAL][S][B] ~5m: `ssh govan@east 'cd ~/tmp/unheaded && sudo bash scripts/host/remediate.sh apply kmod-cis' 2>&1 | tail -3; cd /home/govan/tmp/unheaded && sudo bash scripts/host/remediate.sh apply kmod-cis 2>&1 | tail -3`
- [ ] **Step 196** [AFTER-E4][LIVE-RO][V] ~2m: on both hosts `modprobe -n -v ebtables 2>&1 | grep -c '/bin/false'` is 1 and `lsmod | grep -c '^ebt'` is 0; register FND-011 `plan` notes the apply date (the CVE row itself stays open until the kernel ships).

### M-011 FND-001 alerts reach somebody

- [ ] **Step 197** [R] ~3m: `cd /home/govan/tmp/unheaded && sed -n '55,120p' cmd/alert-mailer/main.go | grep -nE 'Mode|SMTP|env|Getenv|yaml|file' | head -14; grep -n -A12 'alert-mailer:' monitoring/docker-compose.yml | grep -E 'environment|env_file|command|volumes' | head -6`
- [ ] **Step 198** [W] ~8m: `monitoring/docker-compose.yml` alert-mailer: `env_file: [{path: /etc/unheaded/alert-mailer/smtp.env, required: false}]`; mode from `ALERT_MAILER_MODE` (default `dry-run`) if the binary reads env, else the config file path it reads is mounted from `/etc/unheaded/alert-mailer/`; add `monitoring/alert-mailer.env.example` (keys only: `ALERT_MAILER_MODE=dry-run`, `SMTP_HOST=`, `SMTP_PORT=587`, `SMTP_USER=`, `SMTP_PASSWORD=`, `MAIL_FROM=`, `MAIL_TO=`). `sudo docker compose -f monitoring/docker-compose.yml config -q`.
- [ ] **Step 199** [ESCALATE] **E5** ~0m: Context: FND-001 high, due 2026-10-30: alerts reach nobody. Question: create `/etc/unheaded/alert-mailer/smtp.env` on west (0600, root) with the Gmail account + app password and `ALERT_MAILER_MODE=send`. Why no recommendation: a credential only Stevie can mint.
- [ ] **Step 200** [AFTER-E5][LIVE-RW][S][B] ~5m: `cd /home/govan/tmp/unheaded && sudo docker compose -f monitoring/docker-compose.yml up -d alert-mailer 2>&1 | tail -2 && sleep 3 && bash scripts/monitoring/test-alert-path.sh 2>&1 | tail -6`
- [ ] **Step 201** [AFTER-E5][LIVE-RO][V] ~3m: `curl -s http://127.0.0.1:<alert-mailer port from pkg/ports>/v1/outbox | jq '.[0].outcome'` is `"sent"`; the `alert-route` host probe passes on the next baseline run; FND-001 row gets `resolved_on` + evidence.
- [ ] **Step 202** [C] ~1m: `[PLAN S-CLEAN-SLATE] Steps 190-201: FND-011 ebtables stopgap in fixes.yaml; alert-mailer reads /etc/unheaded/alert-mailer/smtp.env (send under E5)`.

### M-010 baselined credentials (repo is public)

- [ ] **Step 203** [B] ~1m: `cd /home/govan/tmp/unheaded && wc -l .gitleaksignore && cut -d: -f1 .gitleaksignore | sort | uniq -c | sort -rn | head -12`
- [ ] **Step 204** [ESCALATE] **E6** ~0m: Context: the repo is public (ADR-095:24, `.github/dependabot.yml`); CLAUDE.md's own "lab-only, no rotation" condition no longer holds; 20 history fingerprints + 25 tree entries are baselined. Question: rotate each baselined credential (OPNsense API key/secret, Grafana admin, LXD trust, service tokens) and tell me which fingerprints are now dead. Why no recommendation: only Stevie holds the systems that mint them.
- [ ] **Step 205** [AFTER-E6][B] ~5m: Remove the dead fingerprints from `.gitleaksignore`; `bash scripts/check-secrets-baseline.sh 2>&1 | tail -2 && bash scripts/check-gitleaks.sh 2>&1 | tail -1`; edit `docs/security/PRE-PUBLIC-BLOCKERS.md` and the CLAUDE.md "On the existing baseline" paragraph: rotation done <date>, remaining entries are test fixtures (list them).
- [ ] **Step 206** [AFTER-E6][V] ~0.5m: both gates PASS; `.gitleaksignore` line count fell (shrink-only honoured).

### M-013, M-014, M-015, M-016, M-017

- [ ] **Step 207** [ESCALATE] **E7** ~0m: Context: FND-014 (Well role password rotation, `pg_stat_statements_reset()`, old PG logs) needs the Well up and a short write window. Question: grant the window; the runbook is the register row's `plan` (`runbooks/data/` has the rotation runbook if `ls runbooks/data | grep -i rotat` lists one). Why no recommendation: live database.
- [ ] **Step 208** [AFTER-E7][LIVE-RW][S][B] ~15m: Run the rotation per the runbook with `scripts/run-runbook.py` (dry-run first, then live); `sudo -u postgres psql -c 'select pg_stat_statements_reset();'` via the documented path; prune old PG logs per the row; FND-014 `resolved_on` + evidence.
- [ ] **Step 209** [ESCALATE] **E8** ~0m: Context: FND-016 / Quantum Armor E3: WireGuard PSK rotation has never run live; `deploy/systemd/unheaded-wg-psk-rotate.{service,timer}` exist. Question: run `rotate-psk.sh --apply` on west once, remove east's stale key, enable the weekly timer. Why the call is Stevie's: a mis-step drops the P2P link that is east's only uplink.
- [ ] **Step 210** [AFTER-E8][LIVE-RW][BARE-METAL][S][B] ~15m: `sudo /usr/local/lib/unheaded/wireguard/rotate-psk.sh --apply 2>&1 | tail -4 && sudo wg show | grep -E 'latest handshake|preshared' && ssh -o ConnectTimeout=5 govan@east true && echo EAST_REACHABLE && sudo systemctl enable --now unheaded-wg-psk-rotate.timer && systemctl list-timers unheaded-wg-psk-rotate.timer --no-pager | head -3`; FND-016 `resolved_on` + evidence.
- [ ] **Step 211** [R] ~3m: `cd /home/govan/tmp/unheaded && sed -n '24,60p' docs/security/pq-host-escalations.md`
- [ ] **Step 212** [ESCALATE] **E9** ~0m: Context: Quantum Armor E2, sshd hybrid-only KEX, east first then west; lockout analysis in the doc read in Step 211. Question: go/no-go, and keep a second session open during the change. Why the call is Stevie's: lockout risk.
- [ ] **Step 213** [AFTER-E9][LIVE-RW][BARE-METAL][S][B] ~20m: Apply the doc's E2 procedure on east, verify a fresh `ssh -o KexAlgorithms=mlkem768x25519-sha256 govan@east true` succeeds and a classical-only `ssh -o KexAlgorithms=curve25519-sha256 govan@east true` fails; then west; `sshd-kex-pq` probe passes on the next baseline run.
- [ ] **Step 214** [ESCALATE] **E10** ~0m: Context: GitHub repository settings. Question: turn on secret scanning + push protection, set default branch `develop`, protect `main` (no force-push, required status checks: Security Scan, Static Analysis, SBOM, GPL Boundary). Why no recommendation: GitHub admin is Stevie's account.
- [ ] **Step 215** [AFTER-E10][W] ~3m: `.github/BRANCH_PROTECTION.md`: record what is on, when, and the required checks list.
- [ ] **Step 216** [W] ~10m: `docs/compliance/export-control-notice.md`: the repo publishes encryption source (ML-KEM, ML-DSA, SLH-DSA, TLS) under EAR §742.15(b) / License Exception TSU (publicly available encryption source code, 15 CFR 740.13(e)); the one-time notification email goes to `crypt@bis.doc.gov` and `enc@nsa.gov` with the repository URL; a two-line pointer in `README.md` under `## License`. Barrister review of PQC licensing §E is a line in the notice ("reviewed by: pending").
- [ ] **Step 217** [ESCALATE] **E11** ~0m: Context: Step 216 drafted the notice. Question: send the TSU notification email from your address (template in the notice doc) and give me the sent date. Why no recommendation: a legal notice carries Stevie's name.
- [ ] **Step 218** [AFTER-E11][W] ~1m: Record the sent date in `docs/compliance/export-control-notice.md`; register CG-21 row updated.
- [ ] **Step 219** [C] ~1m: `[PLAN S-CLEAN-SLATE] Steps 203-218: export-control notice; escalations E6-E11 prepared (rotation, Well, PSK, sshd, GitHub, TSU email)`.
- [ ] **Step 220** [V] ~3m: **PHASE 5 EXIT GATE** — Steps 180, 183, 187, 193 passed; `bash scripts/check-gitleaks.sh | tail -1` PASS; `bash scripts/check-systemd-units.sh | tail -1` green; `sudo docker compose -f monitoring/docker-compose.yml config -q`; the progress file lists E4-E11 with their [AFTER-E] step numbers; `go build ./...` clean.
  - If pass → Phase 6 (if not already done in parallel) else Phase 7.
  - If fail → DO NOT PROCEED.

---

## PHASE 6: DOCS STOP LYING (Steps 221-260)

**Goal**: The 21 doc-hygiene items in the master list §5 are fixed: stale headers, fabricated test tables, wrong statuses, archive moves, dead claims. No code changes in this phase. Covers §5 items 1-21 and decision 12 (relicense, E13).
**Prerequisite**: Phase 1 gate. Runs in `../unheaded-wt-docs` in parallel with Phases 2-5.
**Time**: 5 h.
**Agent**: Agent [P:2-5] (Librarian lens: every edit cites the code path that contradicts the doc).

### Item 1: timeline header/footer

- [ ] **Step 221** [W] ~5m: `references/timeline.md`: `**LAST UPDATED:** 2026-10-27`; `**HEAD:**` one paragraph: "Sprints 1-5 of the 2026-10-23 plan closed 2026-10-26 (Quantum Armor, Two Brains, Real Steel, Lich Awakens, UPC wire-compute); S-CLEAN-SLATE opened 2026-10-27 from the doc sweep (`docs/sweeps/OUTSTANDING-2026-10-27.md`). Before that: ..." (keep one sentence of the old HEAD as history); `**STATUS:**` "Age 3 IN PROGRESS — platform hardening after the five-sprint run; ASCEND-LINUX L5 complete, L6 not started"; footer `*Synced: 2026-10-27 (S-CLEAN-SLATE header refresh; body current to 2026-10-26; prior 2026-09-30 ...)*`; "Remaining for Age 3": replace WAVE14 retrain with "LoRA retrain when the flywheel reaches ≥ 300 SFT examples across ≥ 3 families (Sprint 2 Phase 12 trigger)", delete "VC/public readiness" wording, keep the SBOM regen and sub-50 ms lines (still true).
- [ ] **Step 222** [V] ~1m: `cd /home/govan/tmp/unheaded && bash scripts/check-timeline-freshness.sh | grep Status; grep -c 'VC' references/timeline.md; grep -n 'LAST UPDATED' references/timeline.md`. FRESH; VC count 0 (or only inside dated history entries); LAST UPDATED 2026-10-27. Do NOT touch the mirrors by hand: run `go run ./services/timeguru/cmd/timeguru -sync-once 2>/dev/null || true` only if such a flag exists (`grep -n 'sync' services/timeguru/cmd/timeguru/main.go | head -3`); otherwise the mirrors regenerate on the next timeguru write and the header fields are not part of the mirrored body.

### Items 2-5

- [ ] **Step 223** [W] ~5m: Banner at the top of `docs/protocol/ALIGNMENT_NOTES_DRAFT05.md` and `docs/protocol/PROTOCOL_STATUS_S72.md`: `> **Historical, unverified (2026-10-27).** The implementation paths and test tables below were not found in the tree (17 of 17 sampled, docs/sweeps/outstanding-2026-10-27/sweep-protocol.md). Kept for the drafting history; the live status is docs/RFC-ALIGNMENT-FINDINGS.md.`
- [ ] **Step 224** [W] ~5m: ADR-043 status line: "11 of 13 phases shipped; Phases 9/11/13 wait on east as a node (S5 Step 177)"; the same correction in `wiki/Mimirs-Law.md` and CLAUDE.md's Mímir bullet.
- [ ] **Step 225** [V][R] ~1m: Doom docs are NOT touched here: `docs/doom/STATUS.md` "NOT A BUG" and the banding wording belong to S-DOOM-LAST Phase 1 (M-240; Stevie 2026-10-27: Doom is the lowest priority of any work). [V]: `cd /home/govan/tmp/unheaded && git status --short docs/doom | wc -l` is 0.
- [ ] **Step 226** [W][B] ~6m: ADR-036: status line → `## Status: WITHDRAWN 2026-10-27 — Anthropic (2026-10-26): training on Claude outputs needs prior written authorization; none granted. The 368 QA pairs are not training data.`; `docs/adr/ADR-INDEX.md:97` → **Withdrawn**; `git mv docs/TODO-claude-qa-generation.md docs/archive/TODO-claude-qa-generation-2026-04.md`; then `cd /home/govan/tmp/unheaded && grep -rn 'distilled_qa_claude' --include=*.go --include=*.rs --include=*.py --include=*.sh --include=*.yaml . | grep -v docs/ | head -5` and confirm the flywheel's `--strict-data` path excludes that file (if a reference remains in a loader, add the exclusion with a test: that is a code change, so log it as a new M-item for S-CHAMPION-HARDENING instead of fixing it here).
- [ ] **Step 227** [V] ~1m: `grep -n 'ADR-036' docs/adr/ADR-INDEX.md | grep -c Withdrawn` is 1; `ls docs/TODO-claude-qa-generation.md 2>&1 | grep -c 'No such'` is 1.
- [ ] **Step 228** [C] ~1m: `[PLAN S-CLEAN-SLATE] Steps 221-227: timeline header, protocol notes banner, ADR-043/036 statuses`.

### Items 6-9: archive, procedure, checklist, changelog

- [ ] **Step 229** [W][B] ~10m: `cd /home/govan/tmp/unheaded && mkdir -p docs/archive/internal-2026 && git mv docs/internal/battle-plans docs/archive/internal-2026/battle-plans && git mv docs/internal/sessions docs/archive/internal-2026/sessions && ls docs/internal/security/`; keep `docs/internal/security/` (one live file: check its date; if historical, move it too). Write `docs/archive/internal-2026/README.md` (what moved, when, why: historical S-era battle plans and session logs; the live plans are `docs/battle-plans/`). Rewrite `docs/INTERNAL.md` to point at `docs/battle-plans/NEXT-7-SPRINTS-2026-10-27.md` as the active plan. Byte-identical S73 copy: `cmp docs/archive/internal-2026/battle-plans/battle-plan-S73-public-launch.md docs/battle-plans/battle-plan-S73-public-launch.md 2>/dev/null && git rm -q docs/battle-plans/battle-plan-S73-public-launch.md` (only if `cmp` is silent).
- [ ] **Step 230** [B] ~2m: Dangling references.
  ```bash
  cd /home/govan/tmp/unheaded && grep -rn 'docs/internal/\(battle-plans\|sessions\)' --include=*.md . | grep -v docs/archive | grep -v OUTSTANDING-2026-10-27 | head -20
  ```
- [ ] **Step 231** [V][W] ~5m: Fix every hit from Step 230 to the new path; re-run until empty.
- [ ] **Step 232** [W] ~5m: `docs/workflow/AGENT-OPERATING-PROCEDURE.md`: `:119` `git push origin main` → "Claude never pushes; Stevie pushes `develop` (CLAUDE.md §Branches)"; `:55` "(currently 134/134)" → "(0 failures, see KNOWN FAILURES BASELINE in the active plan)"; "Go 1.24" → `go.mod`'s version; handoff file names → `~/tmp/next.md` + `docs/battle-plans/progress/`.
- [ ] **Step 233** [W] ~5m: `docs/launch/RELEASE_CHECKLIST.md:6,7,37`: LICENSE is GPL-3.0-or-later; protocols GPL-3.0 / Apache-2.0; `docs/launch/LAUNCH_READINESS*.md`: drop the Suricata GPL isolation citation (file deleted 2026-10-22), "Branch: main / READY TO LAUNCH" → "Branch: develop; promotion per ADR-089".
- [ ] **Step 234** [W] ~5m: `CHANGELOG.md:18,53,76`: remove the mTLS mesh claims from `[Unreleased]` (a note: "internal TLS is tracked as FND-017 / S-IRON-GATE"); add an `[Unreleased]` line for S-CLEAN-SLATE. The full rebuild from sprint summaries is M-064 (S-IRON-GATE).
- [ ] **Step 235** [C] ~1m: `[PLAN S-CLEAN-SLATE] Steps 229-234: docs/internal archived, INTERNAL.md repointed, procedure/checklist/changelog corrected`.

### Items 10-14

- [ ] **Step 236** [W] ~8m: `docs/PERFORMANCE-S77.md:40-64`: banner "Historical design doc (S77). `e2ePacketTrace`, `--only`, `make ci-bench` and the 18 ms figure do not exist in the tree (2026-10-27); the benchmark is M-089 in docs/sweeps/OUTSTANDING-2026-10-27.md (S-WELL-SPRING)". `docs/BENCHMARKS.md:96` and `CLAUDE.md:829`: "920K pps" → "920K pps was a Python `sendto` ceiling, not AF_XDP (docs/research/VERDICTS.md); AF_XDP throughput not measured".
- [ ] **Step 237** [W] ~4m: S24 handoff file (`grep -rl 'WIRED' docs/archive/internal-2026 docs/internal 2>/dev/null | head -1`): banner "Shield HMAC/retry/GOAWAY checks are stubbed at ebpf/shield-ebpf/src/main.rs:268-273,516-544 (2026-10-27); code fix is M-180, S-TRUE-WIRE". `ebpf/shield-ebpf/src/main.rs:270` comment "populated by userspace" → "no userspace writer yet (M-180)". Comment-only; `cargo check` not required but run `cd ebpf && cargo fmt --check -p shield-ebpf 2>&1 | tail -1` if the toolchain is present.
- [ ] **Step 238** [W][B] ~5m: Control matrices snapshot banner: `cd /home/govan/tmp/unheaded && for f in docs/compliance/control-matrix/*-2026-05-06.md; do sed -i '1{/^<!--/!s/^/> **Snapshot of 2026-05-06.** Rows citing Suricata, the legacy compliance engine, DataIsolationConfig or the Sentinel daemon refer to code deleted since (E1 sweep 2026-10-26). Re-mapping is M-108 (S-WELL-SPRING).\n\n/}' "$f"; done; head -3 docs/compliance/control-matrix/gdpr-2026-05-06.md` (adjust the sed if the files start with an SPDX comment: insert after the closing `-->`).
- [ ] **Step 239** [W] ~8m: `docs/VISION.md` (the docs/ one, not root): remove adapters/"25 microservices"/wrong draft names, or replace the body with a pointer to root `VISION.md` and ADR-INDEX; `docs/GEMINI.md`: dead make targets and `:8080` → current (`make help`, dashboard 20000) or archive; `docs/README.md`: regenerate the subdirectory list from `ls docs` with one line each (do not describe runbooks/ as K8s).
- [ ] **Step 240** [W][B] ~4m: `docs/skills/README.md` + `docs/skills/unheaded-sentinel.md`: `.skills/` → `~/.claude/skills/` (synced) and drop Suricata; `git mv docs/potential-skill-adds.md docs/archive/potential-skill-adds-2026.md`.
- [ ] **Step 241** [C] ~1m: `[PLAN S-CLEAN-SLATE] Steps 236-240: performance/benchmark claims corrected, matrices bannered, VISION/GEMINI/README/skills docs fixed`.

### Items 15-18

- [ ] **Step 242** [DECIDE][W] ~5m: **`docs/runbooks/*.md`.** RECOMMENDATION: keep the five prose runbooks with a banner "Reference design for a Kubernetes/Vault deployment that is not running (decision 43 pending); live runbooks are `runbooks/**/*.yaml`"; `wiki/Runbooks.md` gets the same one-line caveat. Rationale: deletion waits on the k8s decision. Override ONLY if decision 43 is answered "not a target" before this step runs (then `git rm` them).
- [ ] **Step 243** [W] ~10m: `CLAUDE.md`: `:1180` branch line → develop/staging/main per §Branches; "Age 2 Remaining Epics" → remove shipped items (WireGuard overlay, Foundation-06, Sophia-03, Wotan-03) and the RAFT/Mistral and "Zhen Engine" lines (superseded by Gemma-4 forge and ADR-060), "Demo video + README polish (VC readiness)" → "README polish"; "Alpha Success Criteria" unchecked rows → the two true remainders (public access, benchmarks) with their M-ids; §Metrics `service` label wording → "const label set by the shared middleware"; "Doom: ... Move out of repo before going public" → "Doom stays in the repo (decision 50, 2026-10-27); lowest-priority track"; `**Updated:**` line → 2026-10-27.
- [ ] **Step 244** [W] ~12m: ADR status batch (each a one-line status edit + ADR-INDEX row): ADR-060 → Accepted/Implemented (8623c864); ADR-031 → Superseded by ADR-104/106; ADR-045 → Implemented; ADR-015 → Superseded (Fiber never started; cite the replacement); ADR-040 → Deferred; ADR-022 → Implemented (Pi-hole in LXD); ADR-092 → note "west shipping deployed, east pending (M-082)"; ADR-107 → note "chaos unit installed west+east 2026-10-26; acceptance pending (decision 29)"; ADR-094 → `NewServiceMetrics` note refreshed; ADR-067 → Decision 3 PerCPU deviation note; ADR-023 → Superseded by ADR-074; ADR-004:25 → remove commercial framing (doctrine).
- [ ] **Step 245** [V][B] ~2m: Index agrees with files.
  ```bash
  cd /home/govan/tmp/unheaded && for n in 060 031 045 015 040 022 023 036; do f=$(ls docs/adr/ADR-$n-*.md | head -1); printf "%s file=%s index=%s\n" "$n" "$(grep -m1 -oiE 'Status:?\*{0,2}:? *[A-Za-z/ -]+' "$f" | head -1 | cut -c1-40)" "$(grep -E "^\| \[$n\]" docs/adr/ADR-INDEX.md | awk -F'|' '{print $4}' | cut -c1-40)"; done
  ```
  Every pair agrees on the status word.
- [ ] **Step 246** [C] ~1m: `[PLAN S-CLEAN-SLATE] Steps 242-245: CLAUDE.md truth pass; ADR statuses 015/022/023/031/036/040/045/060/067/092/094/107; runbooks caveat`.
- [ ] **Step 247** [W] ~8m: UPC master plan stale REMAINS: `docs/battle-plans/UPC-LINUX-MASTER-BATTLE-PLAN.md` 1.2.2 byte oracle → done (K-2), `scripts/upc-regression.sh:113` comment → done (K-15); `references/marshal-parked-2026-05-06.md` C4 → resolved by S5, C2 → moot; `docs/doom/ROAD_TO_LINUX*.md`, `UCLINUX*`, `FUZIX*` → banner "superseded by ADR-081 (own tiny kernel)"; `wiki/Linux-on-UPC.md` L6 rung text likewise (`docs/doom/ROAD_TO_LINUX*`/`UCLINUX*`/`FUZIX*` are ASCEND-LINUX rung docs that happen to live under docs/doom; Doom's own docs stay for S-DOOM-LAST).
- [ ] **Step 248** [W] ~6m: Dormant banners: `docs/decisions/2026-04-29-track-call.md` (filled: Track C, 2026-05-11, cite the commit), `docs/demo/*` and `DEMO_VIDEO_SCRIPT*` ("dormant under Track C; commands unverified"), `docs/talks/EBPF_SUMMIT_2026.md` + TALK-ABSTRACT ("the 52-byte HbH option was a research proposal, not shipped; numbers unverified"), `docs/SUBMISSION_SUMMARY.md` file list → `ietf-submission/` current names, `docs/dev/branch-hygiene-2026-05-06.md` → resolved note + correct paths (and the two CLAUDE.md lines it mirrors).
- [ ] **Step 249** [W] ~8m: `docs/network/INGRESS_EGRESS_PORTS.md` → regenerate the table from `pkg/ports/ports.go` (one line per port; gRPC 50051-50067 removed); `docs/monitoring/TELEMETRY_STACK.md` → rewrite against `monitoring/docker-compose.yml` + `docker/telemetry` (VictoriaMetrics, Prometheus rules, Alertmanager, alert-mailer, Vector/ClickHouse; FND-024 note); `docs/ARCHITECTURE.md` "Future Enhancements" → pointers to ADRs; `docs/architecture/{AI_STACK,ZHEN_ARCHITECTURE}.md` + `docs/EXTERNAL_DEPENDENCIES.md` Mistral/vLLM → Gemma-4 / llama.cpp (ADR-049/060); `KINGDOM_ARCHITECTURE.md` "(future)" items → Sleipnir / not planned / ADR-103; `SERVICE_BREAKOUT_STRATEGY.md` → "retired under ADR-047" banner; `FUTURE_INTEGRATIONS.md` Suricata checklist → removed 2026-10-22; `IPV6_METRIC_CAPACITY.md` → "superseded by Foundation-06"; `dashboard-gap-analysis*` → historical; frr-isis TODO marker → done; `sbom-verify` wording → the Makefile target's real behaviour; `docs/MARSHAL-COOKBOOK.md` "11-gate" → the current gate count from `scripts/ci-local.sh`; `docs/RFC-ALIGNMENT-FINDINGS.md` rows MON-04/06/21/22 → status per sweep-protocol.
- [ ] **Step 250** [W][B] ~8m: Wiki reachability: `cd /home/govan/tmp/unheaded && ls wiki/ADR-*.md | wc -l; grep -c 'ADR-' wiki/_Sidebar.md`; add to `wiki/_Sidebar.md` a collapsed `<details><summary>ADRs</summary>` block listing every `wiki/ADR-*.md` as `[[Title|ADR-NNN-...]]` (generate the list with a one-line shell loop, do not hand-type); `wiki/Service-Wotan.md` admin-key section per Step 59; `wiki/Zhen-AI.md` gate-required + CSRF lines per Step 122.
- [ ] **Step 251** [V][B] ~3m: Link check.
  ```bash
  cd /home/govan/tmp/unheaded && ls scripts | grep -iE 'wiki|link' ; bash scripts/check-timeline-freshness.sh | grep Status; for p in $(grep -oE '\[\[[^]|]+(\|[^]]+)?\]\]' wiki/_Sidebar.md | sed -E 's/\[\[([^]|]+\|)?([^]]+)\]\]/\2/'); do [ -f "wiki/$p.md" ] || echo "MISSING wiki/$p.md"; done | head
  ```
  No MISSING lines; if a wiki link checker exists in scripts/, run it and require 0 broken.
- [ ] **Step 252** [C] ~1m: `[PLAN S-CLEAN-SLATE] Steps 247-251: UPC/talks/network/monitoring docs reconciled; wiki sidebar lists every ADR page`.

### Decision 12: relicense (E13)

- [ ] **Step 253** [B] ~2m: Inventory.
  ```bash
  cd /home/govan/tmp/unheaded && grep -rl 'SPDX-License-Identifier: MIT' --exclude-dir=.git --exclude-dir=llama.cpp --exclude-dir=node_modules --exclude-dir=target --exclude-dir=doom . | cut -d/ -f2 | sort | uniq -c | sort -rn | head -15; grep -rl 'SPDX-License-Identifier: MIT' --exclude-dir=.git --exclude-dir=llama.cpp --exclude-dir=node_modules --exclude-dir=target --exclude-dir=doom . | wc -l; sed -n '1p' LICENSE; grep -rln 'licensed under the MIT License' --include=*.go --include=*.rs . | wc -l
  ```
- [ ] **Step 254** [W] ~10m: `scripts/relicense-mit-to-gpl.sh [--dry-run|--apply]`: reads an allowlist `docs/legal/MIT-KEEP.txt` (paths that stay MIT on purpose: anything under `crates/xv6-mbc/upstream/`, vendored third-party, and whatever `THIRD_PARTY.md` names), rewrites `SPDX-License-Identifier: MIT` → `GPL-3.0-or-later` and the 21 "licensed under the MIT License" header sentences, and `LICENSE` line 1 `Steven Bellis` → `Stevie Bellis`; prints the file list; `--apply` refuses if the tree is dirty. shellcheck clean.
- [ ] **Step 255** [B] ~1m: `cd /home/govan/tmp/unheaded && shellcheck scripts/relicense-mit-to-gpl.sh && bash scripts/relicense-mit-to-gpl.sh --dry-run 2>&1 | tail -12`
- [ ] **Step 256** [ESCALATE] **E13** ~0m: Context: Step 255's list (count by directory). Question: relicense all of them to GPL-3.0-or-later (default per decision 12), and is anything missing from `docs/legal/MIT-KEEP.txt`? Why the call is Stevie's: copyright holder.
- [ ] **Step 257** [AFTER-E13][B] ~3m: `cd /home/govan/tmp/unheaded && bash scripts/relicense-mit-to-gpl.sh --apply 2>&1 | tail -3 && make license-check 2>&1 | tail -3 && bash scripts/check-gitleaks.sh | tail -1 && git diff --stat | tail -1`
- [ ] **Step 258** [AFTER-E13][V] ~1m: license-check green; the diff touches only header lines (`git diff | grep '^[-+]' | grep -v '^[-+][-+]' | grep -vc 'SPDX\|MIT License\|Bellis'` is 0).
- [ ] **Step 259** [C] ~1m: `[PLAN S-CLEAN-SLATE] Steps 253-258: relicense script; MIT headers → GPL-3.0-or-later (under E13)`.
- [ ] **Step 260** [V] ~5m: **PHASE 6 EXIT GATE** — Steps 222, 227, 231, 245, 251 passed; `bash scripts/check-timeline-freshness.sh` FRESH; `git status --short` empty after the [C]; if in the docs worktree, `git merge --no-ff clean-slate/docs` into develop and `go build ./...` still clean (docs only, but the merge must be clean).
  - If pass → Phase 7.
  - If fail → DO NOT PROCEED.

---

## PHASE 7: SECURITY AND COMPLIANCE REVIEW (Steps 261-272)

**Goal**: Every attack-surface change this sprint made is listed and reviewed; every gate runs; nothing new leaked. Covers DoD-STD 5-8 for the whole plan.
**Prerequisite**: Phases 1-6 merged on develop.
**Time**: 1.5 h.
**Agent**: Coordinator.

- [ ] **Step 261** [SECURITY][W] ~15m: Append to the progress file an **Attack-surface table**: one row per change with exposure before/after and the test that proves it: Wotan admin endpoints (off by default; key from env; 401 without header); wotan-client re-probe goroutine (outbound only); SubscribeWithRetry in 7 services (no new listeners); Wotan rate limiter keyed by IP (tighter), CORS credentials rule (tighter), streams_active (observability); haproxy configs (edge TLS bind now parses; dev cert never committed); Zhen UI → agentd only + CSRF (tighter; direct llama path removed by default); zhen-agentd auth required + rate limit 5 (tighter); baseline `compliance.deviation.<host>` publish (new outbound message: metadata only); security.txt (new public GET, static); kingdom-startup Phase 1b (starts Prometheus/Alertmanager/alert-mailer at boot: ports already documented); alert-mailer send mode (outbound SMTP, credentials in /etc only); probe subnet restriction (tighter); setup-host preseed (no password); relicense (no runtime effect).
- [ ] **Step 262** [SECURITY][B] ~3m: `cd /home/govan/tmp/unheaded && bash scripts/check-gitleaks.sh 2>&1 | tail -1 && bash scripts/check-secrets-baseline.sh 2>&1 | tail -1 && bash scripts/check-gosec-ratchet.sh 2>&1 | tail -2`
- [ ] **Step 263** [V] ~0.5m: all three PASS; gosec ratchet shows no new rule-ID (exclusions by rule-ID only, never severity: memory project_security_findings_program).
- [ ] **Step 264** [COMPLIANCE][B] ~3m: `cd /home/govan/tmp/unheaded && bash scripts/check-dependency-baseline.sh | tail -1 && go mod tidy -diff && bash scripts/check-tool-freshness.sh | grep -c STALE; bash scripts/check-actions-pinned.sh | tail -1; bash scripts/check-image-tags.sh 2>&1 | tail -1`
- [ ] **Step 265** [V] ~0.5m: baseline PASS, tidy empty, STALE 0, actions pinned, image tags baseline unchanged (haproxy image was already pinned).
- [ ] **Step 266** [SECURITY][B] ~5m: `cd /home/govan/tmp/unheaded && bash scripts/check-systemd-units.sh 2>&1 | tail -2 && bash scripts/check-compose-hardening.sh 2>&1 | tail -2 && bash scripts/check-compose-log-caps.sh 2>&1 | tail -1 && bash scripts/check-haproxy-cfg.sh 2>&1 | tail -2 && bash scripts/check-runbooks.sh 2>&1 | tail -1`
- [ ] **Step 267** [V] ~0.5m: all green; the new `env_file` entries in compose carry `required: false` and point only under `/etc/unheaded/`.
- [ ] **Step 268** [SECURITY][B] ~4m: Fuzz targets touched by this sprint get a short run: `cd /home/govan/tmp/unheaded && go test ./services/wotan/internal/middleware/... -run=^$ -fuzz=Fuzz -fuzztime=30s 2>&1 | tail -3 || echo "no fuzz target in middleware: add FuzzRateLimitKey (RemoteAddr parser) as a [W] here"`; if no target exists, write `FuzzRateLimitKey` (any string → never panics, output has no `:`-port suffix for valid host:port) and run it 30 s.
- [ ] **Step 269** [V] ~0.5m: no crashers; corpus additions committed.
- [ ] **Step 270** [TEST][B] ~20m: Full regression against the baseline.
  ```bash
  cd /home/govan/tmp/unheaded && go test ./... 2>&1 | tee /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/go-test-final.log | tail -3; diff <(grep '^FAIL' /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/go-test-baseline.log | sort) <(grep '^FAIL' /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/go-test-final.log | sort) && echo NO_NEW_FAILURES
  ```
- [ ] **Step 271** [V] ~0.5m: `NO_NEW_FAILURES`; the ok-count is ≥ the baseline's.
- [ ] **Step 272** [V] ~1m: **PHASE 7 EXIT GATE** — Steps 263, 265, 267, 269, 271 passed; attack-surface table written. [C] `[PLAN S-CLEAN-SLATE] Steps 261-271: security and compliance review; fuzz corpus`.
  - If pass → Phase 8.
  - If fail → DO NOT PROCEED.

---

## PHASE 8: DOCS, REGISTERS, HANDOFF (Steps 273-288)

**Goal**: Downstream docs, the ADR-098 register section, the timeline and next.md reflect what shipped; the master list rows this sprint closed are marked; the promotion escalation is filed. Covers M-006, M-008 and the AUDIT-UPDATE duty for every M-item above.
**Prerequisite**: Phase 7 gate.
**Time**: 1.5 h.
**Agent**: Coordinator (Librarian lens).

- [ ] **Step 273** [DOC-UPDATE][W] ~10m: Wiki: `wiki/Service-Wotan.md` (admin key, re-probe, retry subscriptions), `wiki/Zhen-AI.md` and `wiki/Zhen-Agents.md` (gate required, CSRF, agentd auth + rate limit), `wiki/Containers.md` + `wiki/Chaos-Engineering.md` (Phase 1b, FND-023/024 outcome), `wiki/Alerting.md` (alert-mailer env file; send mode status), `wiki/Port-Registry.md` (no change unless a port moved), `wiki/Security-Tooling.md` (haproxy gate; drift-detect removed), `wiki/Huginn.md` (baseline publish topic). Ops facts (paths under `/etc/unheaded/`, timers) go here, not only in docs/ (memory feedback_ops_facts_in_wiki).
- [ ] **Step 274** [DOC-UPDATE][W] ~5m: `CLAUDE.md`: the "Still classical until Stevie decides E1-E5" sentence updated for whichever of E8/E9 ran; the FND-023/024 "Open:" clause in the S-REAL-STEEL paragraph updated; "On the existing baseline" paragraph per Step 205 if E6 ran.
- [ ] **Step 275** [AUDIT-UPDATE][W] ~5m: `compliance/findings/register.yaml`: the untriaged gitleaks-job row (M-006): add the `source:` entry with a one-line note "dependency baseline shrink, S-CLEAN-SLATE Step 25" so the next regen reports it resolved; confirm the rows touched in Steps 157, 166, 196, 201, 208, 210 are in the file's format.
- [ ] **Step 276** [REGEN][B] ~3m: `cd /home/govan/tmp/unheaded && go run ./cmd/compliance-evidence findings -adr docs/adr/ADR-098-findings-register-and-baseline-modes.md 2>&1 | tail -4 && git diff --stat docs/adr/ADR-098-findings-register-and-baseline-modes.md | tail -1`
- [ ] **Step 277** [V] ~1m: the regenerated section has 0 untriaged rows; open count dropped by the number of FND rows closed live (0 if every window is still pending; say which in the handoff).
- [ ] **Step 278** [DOC-UPDATE][W] ~5m: `references/timeline.md`: one dated entry under Age 3 for 2026-10-27+ (S-CLEAN-SLATE: CI green, admin key, retry subscriptions, haproxy, Zhen gate, FND-023/024 status, docs archive, relicense status); then regenerate mirrors the way f83c9cde did (`grep -n 'sync\|mirror' services/timeguru/cmd/timeguru/main.go | head -3` for the flag; if timeguru has to run to sync, `go run ./services/timeguru/cmd/timeguru -once 2>/dev/null || true` and otherwise hand-edit json/yaml/toml consistently).
- [ ] **Step 279** [V][B] ~1m: `cd /home/govan/tmp/unheaded && bash scripts/check-timeline-freshness.sh | grep Status && for f in references/timeline.json references/timeline.yaml references/timeline.toml; do grep -c '2026-10-2[7-9]\|2026-10-3' $f; done` — FRESH; each mirror ≥ 1.
- [ ] **Step 280** [AUDIT-UPDATE][W] ~8m: `docs/sweeps/OUTSTANDING-2026-10-27.md`: for every M-id in Appendix D, append ` → DONE (S-CLEAN-SLATE Step n)` or ` → AFTER-E<n> pending` to the row; §5 items 1-21 likewise.
- [ ] **Step 281** [DOC-UPDATE][W] ~5m: `docs/battle-plans/NEXT-7-SPRINTS-2026-10-27.md` row for Sprint 1: status, date, open escalations; `docs/battle-plans/S-CLEAN-SLATE-battle-plan.md` gets the SHIPPED banner (PHASE24 style) listing phases, STUCK (if any), deferred escalations.
- [ ] **Step 282** [DOC-UPDATE][W] ~8m: `~/tmp/next.md`: new START HERE section (what shipped, which E-numbers wait on Stevie with their [AFTER-E] step numbers, bring-up changes: kingdom-startup Phase 1b, Wotan admin env, zhen-agentd token, haproxy certs dir), overwrite the stale START HERE (memory feedback_next_md_handoff).
- [ ] **Step 283** [B] ~1m: `cd /home/govan/tmp/unheaded && git status --short | grep -v '^?? \(cmd/host-agent/\|host-agent\)' | wc -l`
- [ ] **Step 284** [V] ~0.5m: 0 (everything committed) after Step 285.
- [ ] **Step 285** [C] ~1m: `[PLAN S-CLEAN-SLATE] Steps 273-284: wiki/CLAUDE.md/timeline/next.md; ADR-098 regenerated; master list rows marked`.
- [ ] **Step 286** [ENV][B] ~1m: If the key was cached meanwhile, re-sign now so Stevie pushes signed commits: `cd /home/govan/tmp/unheaded && tree=$(git rev-parse HEAD^{tree}); timeout 20 git commit-tree -S -m probe "$tree" >/dev/null 2>&1 && python3 scripts/resign-unpushed.py 2>&1 | tail -3 || echo "key not cached: Stevie re-signs with scripts/resign-unpushed.py before pushing"`
- [ ] **Step 287** [V] ~2m: **PHASE 8 EXIT GATE** — `git log origin/develop..develop --pretty='%h %G? %s' | awk '{print $2}' | sort | uniq -c` shows only `G` (or only `N` with Step 286's message in the handoff); `bash scripts/check-timeline-freshness.sh` FRESH; `go build ./...` clean; progress file complete.
- [ ] **Step 288** [ESCALATE] **E12** ~0m: Context: develop is green locally on every gate CI runs (Phase 7) and carries this sprint's commits; `develop → staging` has not been promoted since 2026-09-24 (1,062+ commits; `docs/battle-plans/STAGING-PROMOTION-LOG.md`). Question: (1) push `develop` and confirm the six workflows are green on GitHub (`curl -s "https://api.github.com/repos/unheaded/unheaded/actions/runs?branch=develop&per_page=8" | jq -r '.workflow_runs[]|[.name,.conclusion]|@tsv'`); (2) say the word for `develop → staging`, then `staging → main` + signed tag. Why no recommendation: promotion is Stevie's alone (memory feedback_catch_in_develop, feedback_stevie_owns_commits). This plan never merges to staging or main.

---

## APPENDIX A: EMERGENCY PROCEDURES

**A1. `check-dependency-baseline.sh --update` prints `[REFUSED]` (new module).**
1. `go mod why <module>` to find the importer. 2. If it arrived with this sprint's code, remove the import (prefer stdlib). 3. If it is pre-existing on develop, STOP: ADR-004/096 admission; file a new escalation line, leave the gate red for that reason only, say so in the handoff.

**A2. New tool pin fails checksum at install (Step 33).**
1. Re-download the checksums file and the tarball from the GitHub release page (not a mirror). 2. Compare with `sha256sum`. 3. If they disagree, keep the OLD pin, mark Step 29 STUCK, and note that the freshness gate stays red for that tool; do not pin an unverified hash.

**A3. Wotan refuses to start after Step 57 on a host (`WOTAN_ADMIN_API_KEY unset`).**
1. The unit or compose passes `-admin` without the env file. 2. Either create `/etc/unheaded/wotan/wotan.env` (0600) with a generated key (`openssl rand -hex 32`), or drop `-admin` from the command (default off). 3. Never put the key in the unit file or compose YAML.

**A4. A service's late-Wotan test flakes (Step 69-91).**
1. The fake must refuse connections (closed listener), not return 500: use `net.Listen` then `Close()` to get a port that refuses, then start the real fake on that port. 2. Bound the retry backoff in tests via the client's exported test hook (add one if missing: `SetRetryBackoffForTest`). 3. Two flakes → the fix goes into `pkg/wotan-client/subscribe_retry.go`'s backoff, not the service.

**A5. haproxy edge still alerts on `crt` (Step 110).**
1. The container sees `/etc/haproxy/certs/` empty: check the `-v` mount path in the check script and compose. 2. `dev.pem` must contain cert then key, PEM, 0600. 3. If using the host binary, pass `-f` with the same path or skip the host run for edge.

**A6. Zhen UI returns 503 "gate required" after Step 117 in normal use.**
1. `systemctl --user status zhen-agentd` / `curl 127.0.0.1:<agentd port>/health`. 2. zhen-agentd now exits without a token (Step 138): check `zhen-agentd.env` for the token var and that `raft/start-zhen.sh` exports it. 3. Dev escape: `ZHEN_PROXY_VIA_AGENTD=0` for the UI, or `-no-auth` on a loopback agentd. Never ship either.

**A7. kingdom-startup Phase 1b waits 180 s and warns (Step 154).**
1. `sudo docker compose -f monitoring/docker-compose.yml ps`: Prometheus healthy? 2. `curl -s 127.0.0.1:9090/api/v1/rules | jq '.data.groups[].name'` lists the SLO group? 3. Rules need two scrape intervals before `slo:error_ratio:5m` has samples: wait 2 min and re-query. 4. If the group is missing, `scripts/check-prometheus-rules.sh` and the compose volume for `monitoring/prometheus/rules`.

**A8. FND-023 re-run still < 0.99 (Step 165).**
1. Confirm the running `steel-echo load` is the rebuilt binary (`--version` or build time). 2. `-refresh 1s`. 3. If the gap is in the LXD proxy device rather than the meter, that is FND-023's "proxy with its own health checks" branch: leave the row open with the new numbers; do not claim closure.

**A9. `git mv` of docs/internal breaks a wiki or doc link (Step 230).**
1. Re-run the grep across `wiki/`, `docs/`, `CLAUDE.md`, `README.md`, `references/`. 2. Fix the path; never leave a redirect stub file (the archive README is the single pointer).

**A10. Relicense script touches a non-header line (Step 258).**
1. `git checkout -- .` (the script refused a dirty tree, so this reverts only its own changes). 2. Tighten the sed anchors to `^.*SPDX-License-Identifier: MIT$` and the exact header sentence. 3. Re-run `--dry-run`, then `--apply`.

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Phase | Agent | Parallel with | Depends on | Time |
|---|---|---|---|---|
| 0 Preflight | Coordinator | — | — | 40 min |
| 1 CI green | Coordinator | — | 0 | 2.5 h |
| 2 Wotan admin + client | Agent (wt-code) | 3, 6 | 1 | 5 h |
| 3 Live fixes II | Agent (wt-code, after 2 or second worktree) | 2, 6 | 1 | 5 h |
| 4 FND-023/024, east | Coordinator | 6 | 2, 3 merged; E2, E3 for live halves | 3 h + 2 windows |
| 5 Public-repo security | Agent (code) / Coordinator (live) | 6 | 1; E4-E11 for live halves | 3 h + ~2 h windows |
| 6 Docs | Agent (wt-docs) | 2, 3, 4, 5 | 1 | 5 h |
| 7 Security review | Coordinator | — | 1-6 merged | 1.5 h |
| 8 Handoff | Coordinator | — | 7 | 1.5 h |

**Critical path**: 0 → 1 → 2 → 3 → 4 (code half) → 7 → 8 (≈ 19 h). Phases 5 and 6 run alongside 2-4 and finish before 7.

**Dependency map (escalations)**: E1 blocks nothing (Step 42 informational). E2 blocks Steps 153-157, 155b, 164-166. E3 blocks 173-174. E4 blocks 195-196. E5 blocks 200-201. E6 blocks 205-206. E7 blocks 208. E8 blocks 210. E9 blocks 213. E10 blocks 215. E11 blocks 218. E13 blocks 257-258. E12 is the plan's last step and blocks nothing inside it. No escalation blocks a phase exit gate: every gate accepts "escalation logged open" for its live half.

## APPENDIX C: QUICK REFERENCE

**Paths**
- Gates: `scripts/check-dependency-baseline.sh`, `scripts/check-tool-freshness.sh` (+ `scripts/lib/tool-pins.sh`), `scripts/check-gates-can-fail.sh [--quick]`, `scripts/check-actions-pinned.sh`, `scripts/check-gitleaks.sh`, `scripts/check-secrets-baseline.sh`, `scripts/check-gosec-ratchet.sh`, `scripts/check-systemd-units.sh`, `scripts/check-compose-hardening.sh`, `scripts/check-runbooks.sh`, `scripts/check-manifest-yaml.sh`, `scripts/check-timeline-freshness.sh`, new `scripts/check-haproxy-cfg.sh`, `scripts/ci-local.sh` (runs them all).
- CI: `.github/workflows/{security,static-analysis,sbom,gpl-boundary,timeline-drift-guard,lich}.yml`; runbook step `static-analysis.yml:215-222`; meta-gate `:272-280`.
- Wotan: `services/wotan/cmd/wotan/main.go` (`-admin` :418, routes :472-500), `services/wotan/internal/api/handlers.go` (`SetAdminAPIKey` :50, key check :489), middleware under `services/wotan/internal/middleware/`.
- Client: `pkg/wotan-client/client.go` (`NewClientWithGRPC` :291, `ProbeGRPC` :332, `Close` :398), `pkg/wotan-client/subscribe_retry.go:30`.
- Services with one-shot subscribes: `services/{micromanager/service.go,monad/monad.go,sophia/sophia.go,gateway/gateway.go,architect/core.go,anomaly/anomaly.go,loadbalancer/xdpsync.go}`.
- haproxy: `docker/haproxy/{internal,edge}/haproxy.cfg`, `docker/docker-compose.loadbalancers.yml` (image `haproxy:2.9.15-alpine`), edge certs dir `docker/haproxy/edge/certs/` (gitignored pems).
- Zhen: `raft/scripts/zhen_rag.py` (`RAGPipeline.__init__` :222), `raft/zhen_app.py`, `raft/start-zhen.sh`, `cmd/zhen-agentd/{main.go,toolexec.go}`, `cmd/zhen-claude/routed.go`, `scripts/switch-model.sh` (fit check :170-240), venv `~/.venv/zhen`.
- Hosts: `scripts/kingdom-startup.sh` (Phase 1 :130-152), `scripts/kingdom-shutdown.sh`, `scripts/setup-host.sh` (preseed :218-226), `scripts/host/remediate.sh`, `compliance/remediation/fixes.yaml` (kmod-cis :78-92), `compliance/findings/register.yaml` (FND-001 :23, FND-011 :172, FND-023 :1601, FND-024 :1624).
- Monitoring: `monitoring/docker-compose.yml` (alert-mailer, Prometheus 9090, Alertmanager), `scripts/monitoring/test-alert-path.sh`, `cmd/alert-mailer/main.go` (`mode` :68, send needs relay :113).
- Baseline: `cmd/unheaded-baseline/main.go` (flags :90-97), `deploy/systemd/unheaded-baseline.{service,timer,path}`.
- User units: `deploy/systemd/user/unheaded-git-mirror.{service,timer}` → `~/.config/systemd/user/`.
- Docs: `references/timeline.md` (header :5-7, footer :108), `docs/adr/ADR-INDEX.md`, `docs/sweeps/OUTSTANDING-2026-10-27.md`, `docs/archive/internal-2026/` (new).

**Ports** (`pkg/ports/ports.go`): Wotan 18000/18001; Prometheus 9090 (monitoring stack); haproxy edge 21080/21443/21404, internal 21081/21405; zhen-agentd and alert-mailer: read from `pkg/ports` (`grep -n 'Agentd\|AlertMailer' pkg/ports/ports.go`).

**Env / files never in the repo**: `/etc/unheaded/wotan/wotan.env` (`WOTAN_ADMIN_API_KEY`), `/etc/unheaded/alert-mailer/smtp.env`, `/etc/unheaded/baseline.conf`, `/etc/unheaded/{cuirass,sword,chaos}/*.env`, zhen-agentd token env (name from Step 137).

**Commit**: `git commit $COMMIT_FLAG -m "[PLAN S-CLEAN-SLATE] Steps a-b: ..." -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>" -m "Claude-Session: https://claude.ai/code/session_01TzL3TuPhKAVVHnD9jzuabE"`; never `git push`.

**Public CI read (no auth)**: runs `https://api.github.com/repos/unheaded/unheaded/actions/runs?branch=develop&per_page=12`; jobs `.../actions/runs/<id>/jobs`; annotations `.../check-runs/<job id>/annotations`; logs need auth (E1).

## APPENDIX D: COVERAGE (master list → steps)

| M-id | Steps | | M-id | Steps |
|---|---|---|---|---|
| M-001 | 25-26 | | M-033 | 147-154, 157 |
| M-002 | 27-33 | | M-034 | 159-166 |
| M-003 | 35-42 (E1) | | M-035 | 102-115 |
| M-004 | 43-46 | | M-036 | 116-124 |
| M-005 | 47-50 | | M-037 | 125-128 |
| M-006 | 275-277 | | M-038 | 93-100 |
| M-007 | 51 | | M-039 | 133-136 |
| M-008 | 288 (E12) | | M-040 | 129-132 |
| M-010 | 203-206 (E6) | | M-041 | 168-174 (E3) |
| M-011 | 197-201 (E5) | | M-042 | 141-144 |
| M-012 | 190-196 (E4) | | §5 item 1 | 221-222 |
| M-013 | 207-208 (E7) | | §5 items 2-5 | 223-227 |
| M-014 | 209-210 (E8) | | §5 items 6-9 | 229-234 |
| M-015 | 211-213 (E9) | | §5 items 10-14 | 236-240 |
| M-016 | 214-215 (E10) | | §5 items 15-18 | 242-249 |
| M-017 | 216-218 (E11) | | §5 items 19-21 | 248-251 |
| M-018 | 177-183 | | decision 12 (relicense) | 253-258 (E13) |
| M-019 | 137-140 | | decision 1 (drift) | 47 |
| M-020 | 156 (E2) | | decision 2 (meta-gate) | 44 |
| M-021 | 185-188 | | decision 13 (FND-024) | 147 |
| M-030 | 53-61 | | decision 14 (FND-023) | 159 |
| M-031 | 68-92 | | decisions 3, 7-11, 15 | 288, 204, 199, 194, 209/212, 217, 172 |
| M-032 | 62-67 | | | |

Deferred from this sprint, with the plan that owns them: M-064 CHANGELOG rebuild → S-IRON-GATE; M-089 e2e benchmark → S-WELL-SPRING; M-108 control-matrix remap → S-WELL-SPRING; M-180 Shield stubs (code) → S-TRUE-WIRE; baseline `.deb` → S-IRON-GATE (M-062); the `--strict-data` exclusion if Step 226 finds a loader reference → S-CHAMPION-HARDENING. No Doom step in this plan (memory feedback_doom_lowest_priority).

---

*S-CLEAN-SLATE Battle Plan — Forged 2026-10-27*
*9 Phases. 288 Steps. The CI goes green, the services stop going deaf, and the docs say what the code does.*
