<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# S-WELL-SPRING BATTLE PLAN: 16 Phases, 294 Steps

**Date**: 2026-10-27
**Sprint**: Sprint 3 of 7 (NEXT-7-SPRINTS-2026-10-27), WELL SPRING. The Well becomes the source of truth for deploy and chaos state and for SLO definitions; the SRE remainder of ADR-100 is built; the compliance program's paperwork and enforcing-mode code paths exist so zero becomes claimable.
**Prerequisite**: S-CLEAN-SLATE done (CI green on develop, FND-024 monitoring start fixed, east debs rebuilt: M-033, M-041). Branch based on `a7fe3acd` or newer. `go build ./...` clean. S-IRON-GATE is NOT required; Steps 70, 108, 242 and 285 get stronger once it lands (noted inline).
**Target**: every deploy, chaos experiment and SLO definition is a row in The Well with a ledgered migration behind it; the watchdog, four postmortems, the `runbook_url` gate and the error-budget panel exist; no alert rule names a metric nobody emits; the five unmeasured performance gates have numbers; the three attestations, the policies, the contingency plan and the SSP are drafted for Stevie's signature; `unheaded-baseline` has a typed enforcing path behind a breaker and a soak; every claim of "live" was proven by a [LIVE-RO] or [LIVE-RW] step.
**Estimated Duration**: 30-38 h of execution across 6-8 sessions. Five waits on Stevie (E-A migration window, E-B PG 18 window, E-C ratify + sign, E-D host window, E-E enforcing/MFA/pentest/IDS calls).
**Agent Strategy**: Coordinator owns Phases 0, 1, 5, 9, 13 (host-touching or live-Well). Phases 2+3, 6+7+8, 10+11, 12 can run in parallel in their own git worktrees off develop (Appendix B). Hosts are read-only for every agent unless the step is covered by an answered [ESCALATE].
**Commit Cadence**: every 5 steps (max(3, min(5, 294/20)) = 5), at every exit gate, before every STUCK skip, after every red-to-green test. Never commit red. Never push (Stevie pushes). Signed if `git commit-tree -S` succeeds in Step 3, otherwise `--no-gpg-sign` (memory: feedback_unsigned_commits_when_afk).
**Stuck Protocol**: skip after 3x the step's `~time` or 2 failed debug attempts; write the STUCK marker into this file, commit, continue with the next step that does not depend on it (Appendix B dependency map).

**Builds on (cited, not re-derived):**

| Source | What this plan takes from it |
|---|---|
| `docs/sweeps/OUTSTANDING-2026-10-27.md` | Every M-### cited in a phase goal; this plan covers E06 (M-070..077), E07 (M-080..090), E08 (M-100..109), E13 (M-200, M-203, M-205). Appendix D maps each to steps. |
| ADR-099 (Proposed) | Fix model as data; Phase 2 native executor with parity to `remediate.sh`, then retire it; Phase 3 Ansible renderer + drift test; Phase 4 enforcing behind ADR-098's conditions. "Applied so far" = the eight `lab` fixes on east then west (2026-10-01). |
| ADR-100 (Accepted, implementation 2026-10-20 + Phase 3 2026-10-25) | SLIs are time-based; every hand rule is `urgency: none`; alert-mailer is the only sender; Phases 1-5 table; Stevie's decisions (99.5 % / 99 % targets, budgets not frozen, postmortem threshold undecided); "No Watchdog yet" because it needs a receiver outside the host. |
| ADR-098 (Proposed, rolling) | Phase 1 remainder (receiver + heartbeat, timer for `compliance-evidence`); Phase 2 `unheaded-baseline` audit only, "this build has no enforcing code path"; Q2 heartbeat default = west and east watching each other; Q5 enforcing strictness unanswered; answers 1-10 of 2026-09-30/10-01 (SSH/passwords stay, lab). |
| ADR-101 ledger (`db/migrations/MANIFEST`, `LEDGER.sql`) | Routing `<database> <file> [notx] [superuser]`; never edit an applied file; owner roles after 025; `well-migrate -dir db/migrations` with lock/statement timeouts. |
| ADR-106 Q1 | Migrations 030-034 are written and tested in docker but not applied live; `-confirm queue` and the worker wait on the batch. |
| `docs/battle-plans/WELL-ERDA-AUDIT-2026-10-13.md` §5.3, 5.4, 5.7 | Low-value index drops wait for stats after 2026-11-15; partitioning trigger (1 M rows / 1 GB / 90 days before archive age); NOT NULL per table with its writers. |
| `docs/policy/retention-policy-v1-2026-05-06.md` | Retention classes and ages this sprint turns into timers; audit rows are never deleted, partitions are detached and archived. |
| `observability/alerting/policy.yaml`, `cmd/alert-policy` | The one tunable file: 7 SLOs (wotan, dashboard, well, well-backups, overlay, log-shipping, kev-feed), 28-day window, mailer `dry-run`, `promote: []`. `alert-policy -check` fails on stale generated files. |
| `monitoring/prometheus/rules/*` + `scripts/check-prometheus-rules.sh` | 9 rule files, 75 alerts; `UNTESTED=(loadbalancers.yml monad.yml)`; `runbook_url` present on 32 alerts (slo 28, host 3, compliance 1). |
| `compliance/attestations/README.md`, `compliance/catalog/controls.yaml` (49 controls) | Attestations are signed (`%G? == G`), expiring, self-attested YAML; unknown fields rejected; only the accountable person signs. Catalog ids UH-*; `compliance-evidence` prints 13 FAIL rows today incl. UH-REMED-01, UH-BASE-01, UH-MAC-01. |
| `compliance/remediation/fixes.yaml` (13 fixes), `cmd/unheaded-remediate`, `cmd/unheaded-baseline` | Fix ids: sysctl-cis, apport-off, kmod-cis, cron-perms, banner-cis, apparmor-utils, auditd, sudo-cis, shell-tmout, sshd-safe, auditd-rules, journald-caps, time-p2p. Remediate takes `-model`, `-snapshot` (sealed), `-state /var/lib/unheaded/remediate`. Baseline takes `-config /etc/unheaded/baseline.conf` (`mode=audit|enforcing`), `-state-dir`, `-textfile-dir`. |
| `services/sword/sword.go`, `services/yaldabaoth/journal.go`, `cmd/chaos-controller/packetcmd.go` | The three `pkg/statefile` writers this sprint moves to The Well (`statefile.In(cfg.DataDir, stateFileName)`; `JournalEntry`; `packetJournalFile`). |
| `pkg/logagg/metrics.go` | `Publisher.Collectors()` already exists; `WriteMetrics` is the hand-rolled path still used by unheaded-daemon, kanban-app, sophia, monad, timeguru. |
| `cmd/perf-benchmark/main.go` | Flags `-iterations -output -east-p2p -east-wg -health -wotan`; no e2e packet→browser, container-start, req/s or HTTP/3 scenario exists (PERFORMANCE-S77 cites them). |
| `docs/research/VERDICTS.md` | Eight hypotheses H1-H8 (not nine) defined, none scored; `docs/BENCHMARKS.md:80,96` and `CLAUDE.md:829` label a Python `sendto` ceiling as "AF_XDP 920K pps". |
| `cmd/akira`, `pkg/ports` | Akira on 19100 publishes `system.health.reports` and `system.health.consensus`; no per-service reporter; no `system.health.remediate`. Sword 19011, Chaos 19012, AlertMailer 19130, TraceCollectorMetrics 16671. |
| ADR-055 | Phase 2 named NVD and GHSA sibling pollers; S13 judged them low value next to govulncheck/cargo-audit/pip-audit/grype. |
| ADR-062 | Campaign inventory and activation rules; new offensive campaigns are registered before host exposure. |
| `~/tmp/next.md` S12b, S7b, S14, S6e | Wotan OOM-killed 06:42 and down 10 h (postmortem 1); 13 dead alerts; journald 3.4 GB no SystemMaxUse; two world-writable ROCm logs; Vector→ClickHouse never shipped. |

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
6. Attack surface unchanged, or the change is listed in Phase 14's review table.
7. Every claim of "real" in a commit message has a [LIVE-RO] or [LIVE-RW] step that proved it, cited by step number.
8. Zero user data access: no step reads packet payloads, log contents of other services, or user data; probes read metadata only.

---

## SITUATION (verified at a7fe3acd, 2026-10-27)

- **Statefile is still the deploy/chaos store.** `services/sword/sword.go:253` `statefile.In(cfg.DataDir, stateFileName)`; `services/yaldabaoth/journal.go:51`; `cmd/chaos-controller/packetcmd.go:162`. Also `services/architect`, `services/micromanager`, `cmd/unheaded-daemon/desired_store.go` (out of scope; noted). M-072.
- **Migrations 030-034 exist, are tested in docker (`scripts/well/test-llm-requests.sh`, `test-zhen-confirm.sh`, `test-zhen-agent-runs.sh`, `test-zhen-captures.sh`), and are not applied to the live Well** (ADR-106 Q1). `db/migrations/MANIFEST` routes by database; `LEDGER.sql` has 29 lines. M-070.
- **Two services still create tables at startup**: `cmd/kanban-app/store.go:121` and `services/timeguru/internal/storage/storage.go:83,94` (`CREATE TABLE IF NOT EXISTS`). `pkg/database` has no ledger-presence helper (`config.go`, `connect.go`, `kanban_store.go`, `timeline.go`, `health.go`, `zhen.go`, `config_store.go`, `bridge.go`). M-073.
- **PG 18 upgrade runbook exists** (`runbooks/data/well-pg18-upgrade.yaml`), never run. Well is PG 16, TLS-only, owner roles, passwords in `.env` (never in git). M-071.
- **Retention policy v1 is a doc** (`docs/policy/retention-policy-v1-2026-05-06.md`), unratified; no retention timer among `deploy/systemd/unheaded-well-{backup,drill,offsite,offsite-check}`; `zhen_conversations` (003) has no DELETE grant for any app role. M-074.
- **ERDA 5.3 index drops wait for stats after 2026-11-15**; 5.4 partitioning has a volume trigger; 5.7 NOT NULL is per table (`WELL-ERDA-AUDIT-2026-10-13.md:236-258`). M-076.
- **SLOs are YAML only**: `observability/alerting/policy.yaml` → `cmd/alert-policy` → `monitoring/prometheus/rules/slo.yml` (28 alerts) + `alertmanager.yml`. No Well table, no `slo` evidence kind (ADR-100 Phase 2 "none started"). M-077.
- **No Watchdog** (ADR-100 Implementation: "needs a receiver outside this host"); ADR-098 Q2 default is west/east cross-watch. Alertmanager receivers: `none`, `page`, `ticket` (webhooks to alert-mailer on 19130). M-080.
- **No postmortems**: no `docs/postmortems/` or `docs/incidents/`; ADR-100 Phase 1 names three, next.md adds the Wotan OOM silent outage (2026-10-18, 10 h). M-081.
- **Dead alerts**: `monad.yml` 5 alerts (4 on `unheaded_monad_hbh_drop_total`, `unheaded_bgp_session_up`, `unheaded_bfd_failover_total`, `unheaded_routing_health_up`, none emitted by any binary; `NodeExporterDown` is real); `loadbalancers.yml` 7 alerts on `haproxy_*`/`nginx_*` with no scrape job. Both baselined `UNTESTED` in `scripts/check-prometheus-rules.sh:56`. M-083.
- **No rule or runbook for the live trace-collector**; `runbook_url` is on 32 of 75 alerts and nothing gates it. No CI SLO; no error-budget Grafana panel (dashboards 01-07 + firewall + infrastructure). M-084, M-087.
- **heimdall-daemon has counters (`cmd/heimdall-daemon/drift.go:155-157`) and no listener** (`main.go:359-363` flags: manifest, node-id, scan-interval, pubkey, allow-unsigned). M-085.
- **Hand-rolled `WriteMetrics` in 5 binaries** (`cmd/unheaded-daemon/main.go:1249`, `cmd/kanban-app/main.go:1293`, `cmd/sophia/main.go:457`, `cmd/monad/main.go:571`, `services/timeguru/cmd/timeguru/main.go:196`) while `Publisher.Collectors()` exists (`pkg/logagg/metrics.go:28`). `pkg/metrics/pqc_metrics.go` declares 9 `unheaded_pqc_*` metrics nothing sets; `pkg/secrets` constructs 4 metrics nothing serves. M-086.
- **Vector reads journald only** (`config/vector/vector.yaml:19`); container stdout and east's journal are not shipped; ADR-092 step 7 (journald caps) not applied. M-082.
- **ADR-076/086 (topology, Muninn) are unbuilt and pre-date Vector.** M-088.
- **`cmd/perf-benchmark` has no e2e, container-start, req/s or HTTP/3 scenario**; `docs/research/VERDICTS.md` defines H1-H8, none scored. M-089, M-090.
- **Akira**: `cmd/akira` publishes `system.health.reports` (7 refs) and `system.health.consensus` (2 refs); no per-service reporter; no remediate topic (ADR-029). M-200.
- **No detection rules** for SSH origin, lockout, outbound volume, DGA/beaconing, DNS tunnelling; Vector already emits `journal_auth_failures` metrics. ADR-055 Phase 2 pollers unbuilt. M-203.
- **Compliance**: `compliance/attestations/` holds only README; 13 catalog controls FAIL today (UH-SECRET-01, UH-GATE-01, UH-REMED-01, UH-HOST-02/03/04, UH-SVC-01/02, UH-ACCT-02, UH-BASE-01, UH-MAC-01, UH-BOOT-01, UH-SUDO-01, UH-PASS-01, UH-SESS-01; several accepted until 2026-12-30 per ADR-098 answer 10). No infosec policy, contingency plan, risk strategy or SSP file. M-100..M-104.
- **Hosts have no remediation journals** (UH-REMED-01 FAIL): the eight lab fixes were applied by `scripts/host/remediate.sh` on 2026-10-01; `cmd/unheaded-remediate` exists with `-state /var/lib/unheaded/remediate`. `fixes.yaml` has `journald-caps` and `time-p2p` unapplied. M-105.
- **`unheaded-baseline` has no enforcing code path** (ADR-098 Phase 2 spec). M-106.
- **Ansible**: `deploy/ansible/compliance/{inventory.yml, profile-*.yml, roles/*}` exist; nothing syntax-checks them in CI; no kingdom-base/huginn/victoria roles. M-107.
- **Control matrices** under `docs/compliance/control-matrix/*-2026-05-06.md` cite deleted code (DataIsolationConfig, Suricata). M-108.
- **Threat models**: 8 docs under `docs/security/`, no review-cadence line; ADR-062 §"Existing campaign inventory" is the registration point. M-205.
- **Monitoring stack** (`monitoring/docker-compose.yml`: prometheus, grafana, loki, victoriametrics, alertmanager, alert-mailer) is started by kingdom-startup after S-CLEAN-SLATE (M-033). This plan assumes it is up for every [LIVE-RO] step that reads series.

### Escalations

| ID | Question (exact) | Blocks | Decision # |
|---|---|---|---|
| E-A | Window to apply migrations 030-034 live as one batch (ADR-106 Q1), plus the new 035-038 from this sprint. Date/time; the Well backup runs first. Asked at Step 44. | Steps 45-49, 78-80, 105-106, 127-129 | 20 |
| E-B | Go/no-go for PG 16→18 on the live Well (downtime ~15 min, `runbooks/data/well-pg18-upgrade.yaml`). Asked at Step 138. | Steps 139-143 | 10 (E1) |
| E-C | Ratify retention policy v1 (archive/purge ages as drafted in Step 113) and sign the three attestations after reading the policy, IR plan and access list (drafts from Step 244, `scripts/compliance/sign-attestation.sh`). Signing must be Stevie's GPG commit. Asked at Step 130. | Step 131; the three signing commits (Stevie's own) | 21 |
| E-D | Host window: journald `SystemMaxUse=1G RuntimeMaxUse=200M` (east first), `chmod 0640` the two ROCm logs, re-apply the eight lab fixes through `unheaded-remediate` for journals, apply `journald-caps` + `time-p2p` + `auditd-rules`, install the east heartbeat timer, vector-east, the ci-status timer and the per-service health reporter via the deb bundle, restart Vector and re-up the compose stack with the journald driver, run the container-start benchmark. Asked at Step 18a. | Steps 208-209, 219-220, 235-236, 280-282 | 26 |
| E-E | Enforcing-mode Q5 (refuse starts vs installs only); MFA scope; external pentest; host AV/IDS stance; counsel/DFIR; risk appetite. Asked at Step 275. | Nothing in this plan: the enforcing flip happens after the soak, outside this plan; the answers are recorded in `docs/compliance/privacy/decisions-for-stevie.md` | 28 |

---

## PHASE 0: PREFLIGHT (Steps 1-18, 18a)

**Goal**: every SITUATION claim re-checked live; toolchain, baseline and worktrees ready. Cites M-070..M-205 by re-verifying their premises.
**Prerequisite**: S-CLEAN-SLATE exit gates passed.
**Time**: 45 min
**Agent**: Coordinator

- [ ] **Step 1** [PREFLIGHT][B] ~1m: Branch and base
  ```bash
  cd /home/govan/tmp/unheaded && git rev-parse --abbrev-ref HEAD && git merge-base --is-ancestor a7fe3acd HEAD && echo "base ok" && git status --short | wc -l
  ```
- [ ] **Step 2** [V] ~1m: On `develop` (or a sprint branch off it), `base ok`, 0 modified files. If not → stash or commit the stray work before continuing.
- [ ] **Step 3** [PREFLIGHT][B] ~1m: Signing probe and identity
  ```bash
  cd /home/govan/tmp/unheaded && git config user.email && (git commit-tree -S -m probe "$(git rev-parse HEAD^{tree})" >/dev/null 2>&1 && echo SIGN=yes || echo SIGN=no)
  ```
- [ ] **Step 4** [V] ~1m: email is `stevie@bellis.tech`. Record `SIGN=yes|no` in the progress file; `no` means every [C] uses `--no-gpg-sign`.
- [ ] **Step 5** [PREFLIGHT][ENV][B] ~2m: Tools
  ```bash
  for t in go cargo docker psql promtool jq python3 ansible-playbook; do printf "%-16s %s\n" "$t" "$(command -v $t || echo MISSING)"; done; go version; docker --version | head -1
  ```
- [ ] **Step 6** [V] ~1m: go, docker, psql, jq, python3 present. `promtool` MISSING is fine (check-prometheus-rules runs it in docker). `ansible-playbook` MISSING is expected (Phase 13 installs it into the CI venv).
- [ ] **Step 7** [PREFLIGHT][BUILD][B] ~4m: Build + KNOWN FAILURES BASELINE
  ```bash
  cd /home/govan/tmp/unheaded && go build ./... && mkdir -p docs/battle-plans/progress && (go test ./... 2>&1 | grep -E '^(FAIL|ok|---)' | grep -v '^ok' | tee docs/battle-plans/progress/S-WELL-SPRING-baseline.txt | tail -5; echo "baseline lines: $(wc -l < docs/battle-plans/progress/S-WELL-SPRING-baseline.txt)")
  ```
- [ ] **Step 8** [V] ~1m: `go build` exit 0. Baseline file written (expected 0 FAIL lines after S-CLEAN-SLATE). Any FAIL present now is pre-existing and must not grow.
- [ ] **Step 9** [PREFLIGHT][B] ~1m: Statefile writers still in place (M-072)
  ```bash
  cd /home/govan/tmp/unheaded && grep -rl 'unheaded/pkg/statefile' --include=*.go services/sword services/yaldabaoth cmd/chaos-controller | sort
  ```
- [ ] **Step 10** [V] ~1m: exactly `cmd/chaos-controller/packetcmd.go`, `services/sword/sword.go`, `services/yaldabaoth/journal.go`. If fewer, someone moved one already: read it and shrink Phase 2.
- [ ] **Step 11** [PREFLIGHT][B] ~1m: Startup DDL and ledger helper absence (M-073); unapplied migrations (M-070)
  ```bash
  cd /home/govan/tmp/unheaded && grep -rnE 'CREATE TABLE IF NOT EXISTS' --include=*.go cmd services | grep -v _test; ls pkg/database/ledger.go 2>/dev/null || echo "no ledger helper"; ls db/migrations | tail -6; grep -c . db/migrations/LEDGER.sql
  ```
- [ ] **Step 12** [V] ~1m: three DDL sites (kanban-app store.go, timeguru storage.go x2); `no ledger helper`; last migration `034_app_kanban_genesis.sql`; LEDGER 29 lines. New migrations in this plan therefore start at 035.
- [ ] **Step 13** [PREFLIGHT][B] ~1m: Alert-rule facts (M-083, M-087)
  ```bash
  cd /home/govan/tmp/unheaded/monitoring/prometheus/rules && for f in *.yml; do printf "%-20s alerts=%-3s runbook_url=%s\n" "$f" "$(grep -cE '^\s*- alert:' $f)" "$(grep -c runbook_url $f)"; done; grep -n 'UNTESTED=' ../../../scripts/check-prometheus-rules.sh
  ```
- [ ] **Step 14** [V] ~1m: 75 alerts total; `runbook_url` on 32; `UNTESTED=(loadbalancers.yml monad.yml)`.
- [ ] **Step 15** [PREFLIGHT][LIVE-RO][B] ~2m: Monitoring stack up and SLO series present (needs S-CLEAN-SLATE M-033)
  ```bash
  curl -s --max-time 5 'http://localhost:8428/api/v1/query?query=count(slo:error_ratio:5m)' | jq -r '.data.result[0].value[1] // "NONE"'; docker ps --format '{{.Names}}' | grep -E 'prometheus|alertmanager|alert-mailer|victoriametrics' | sort
  ```
- [ ] **Step 16** [V] ~1m: count ≥ 7 and the four containers listed. If NONE → `sudo docker compose -f monitoring/docker-compose.yml up -d` and wait 5 min (this is FND-024's recipe; S-CLEAN-SLATE should have made kingdom-startup do it).
- [ ] **Step 17** [PREFLIGHT][B] ~2m: Worktrees for parallel agents (Appendix B)
  ```bash
  cd /home/govan/tmp/unheaded && for w in well-store slo-data sre-rules metrics-clean measure akira paperwork; do git worktree add -q "../wt-wellspring-$w" -b "ws/$w" HEAD 2>/dev/null || echo "exists: $w"; done; git worktree list | grep wellspring | wc -l
  ```
- [ ] **Step 18** [V] ~1m: **PHASE 0 EXIT GATE**: 7 worktrees; Steps 2, 4, 6, 8, 10, 12, 14, 16 passed. Record the progress file header (date, base SHA, SIGN, baseline count). If any [V] failed → fix within Phase 0; do not start Phase 1.
- [ ] **Step 18a** [ESCALATE] **E-D** ~0m: Asked now so the window can be scheduled while Phases 1-12 run; nothing in it executes before its units exist (Phases 6, 9, 11, 13). Context: the Escalations table row E-D lists every host change; east first, west second; each is reversible (`unheaded-remediate undo`, `docker compose up -d` with the previous compose, timers disabled). Questions: (1) when is the window and how long; (2) journald `SystemMaxUse=1G RuntimeMaxUse=200M`: these numbers or others; (3) `chmod 0640` on the two world-writable ROCm logs: yes; (4) may the compose stack on west be re-created once (journald logging driver) inside the window. Why Stevie: host changes on both machines and a full stack restart.

---

## PHASE 1: WELL LEDGER DISCIPLINE (Steps 19-50)

**Goal**: no service creates tables at startup (M-073); the 030-034 batch is rehearsed, dry-run and rollback-scripted so E-A is a 10-minute window (M-070); soft-delete purge question closed (M-075); the ERDA index-drop step is date-gated (M-076).
**Prerequisite**: Phase 0.
**Time**: 3 h
**Agent**: Coordinator (Steps 44-50 are [LIVE-RW] under E-A); Steps 19-43 may run in worktree `well-store`.

### Ledger-presence helper

- [x] **Step 19** [CODE][W] ~5m: Test first: `pkg/database/ledger_test.go`
  ```go
  // RequireMigrations returns nil when every named file has a row in
  // well_schema_migrations, ErrMigrationMissing{File} otherwise. Test with
  // sqlmock-free fake: a *sql.DB from the docker Well is not needed; use the
  // Querier interface so the test passes an in-memory implementation.
  // Cases: all present -> nil; one missing -> error names the file; empty
  // list -> nil; query error -> wrapped.
  ```
  Write the four cases against a `fakeQuerier` returning canned rows.
- [x] **Step 20** [TEST][B] ~1m: Expect FAIL (undefined symbols)
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/database/ -run TestRequireMigrations -count=1 2>&1 | tail -3
  ```
- [x] **Step 21** [V] ~1m: FAIL with `undefined: RequireMigrations` (red).
- [x] **Step 22** [CODE][W] ~8m: `pkg/database/ledger.go`: `type Querier interface{ QueryContext(ctx, q string, args ...any) (*sql.Rows, error) }`; `type ErrMigrationMissing struct{ File string }`; `func RequireMigrations(ctx context.Context, q Querier, files ...string) error` doing `SELECT filename FROM well_schema_migrations WHERE filename = ANY($1)` and diffing. Column name: confirm with `grep -n 'well_schema_migrations' db/migrations/*.sql | head -3` and use the real column.
- [x] **Step 23** [TEST][B] ~1m: Expect PASS
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/database/ -run TestRequireMigrations -race -count=1 2>&1 | tail -3
  ```
- [x] **Step 24** [V] ~1m: `ok` (green). If FAIL → Step 25.
- [x] **Step 25** [D] ~5m: Print the ledger DDL and fix the column: `grep -n -A6 'CREATE TABLE.*well_schema_migrations' db/migrations/*.sql`. Max 2 attempts, then STUCK.
- [x] **Step 26** [CODE][W] ~10m: kanban-app: replace the DDL at `cmd/kanban-app/store.go:121` with `database.RequireMigrations(ctx, db, "034_app_kanban_genesis.sql")` (the file that owns `kanban_tasks` today; confirm with `grep -ln 'kanban_tasks' db/migrations/*.sql`) and return a fatal, logged `ErrMigrationMissing` so the unit fails loud (ADR-101: the ledger is the truth). Keep the `tasks` DDL only in a `_test.go` fixture if tests used it.
- [x] **Step 27** [CODE][W] ~10m: timeguru: same at `services/timeguru/internal/storage/storage.go:83,94` for `timeline` and `milestone_history` (confirm owning migration with `grep -lnE 'timeline_milestones|milestone_history|CREATE TABLE timeline' db/migrations/*.sql`). If the table has NO migration yet, write `db/migrations/035_app_timeguru_tables.sql` (owner role per 025 pattern) and add the MANIFEST line `unheaded_app 035_app_timeguru_tables.sql`.
- [x] **Step 28** [TEST][B] ~3m: Package tests + docker migrate suite
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/kanban-app/... ./services/timeguru/... -race -count=1 2>&1 | tail -4 && bash scripts/well/test-migrate.sh 2>&1 | tail -5
  ```
- [x] **Step 29** [V] ~1m: both `ok`; test-migrate PASS (ledger sha256 of 035 recorded). If test-migrate fails on 035 → Step 30.
- [x] **Step 30** [D] ~5m: `bash scripts/check-well-migrations.sh 2>&1 | tail -8` (psql meta-commands, ordering, owner). Fix the file; a migration never applied anywhere may still be edited.
- [x] **Step 31** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  cd /home/govan/tmp/unheaded && git add -A && git commit $GPGFLAG -m "[PLAN S-WELL-SPRING] Steps 19-30: ledger-presence helper; kanban-app and timeguru stop creating tables at startup

Phase 1: Well ledger discipline
Steps completed: 19-30
Verification gates passed: 21 (red), 24 (green), 29 (suite)
Next: Step 32

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01TzL3TuPhKAVVHnD9jzuabE"
  ```
  (`GPGFLAG=--no-gpg-sign` when SIGN=no, else empty. Same in every [C].)

### The 030-034 batch, rehearsed

- [x] **Step 32** [B] ~3m: Fresh docker Well from init, then apply 030-038 as `well-migrate` would (the suites do this per file; this runs the whole batch once)
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/well/test-migrate.sh 2>&1 | grep -E 'applied|PASS|FAIL' | tail -12
  ```
- [x] **Step 33** [V] ~1m: every file 030..035 `applied` once, ledger rows present, PASS.
- [x] **Step 34** [W] ~10m: `scripts/well/batch-apply.sh`: wraps `go run ./cmd/well-migrate -dir db/migrations -lock-timeout 5s -statement-timeout 5m` with: (1) refuse unless `WELL_MIGRATE_PASSWORD` set, (2) `pg_dump` of `unheaded_app` and `unheaded_ops` to `/srv/backups/unheaded-well/pre-batch-$(date -u +%Y%m%dT%H%M%SZ)/` first (same layout `scripts/well/test-backup-restore.sh` restores), (3) `--dry-run` flag that prints the pending files from the MANIFEST minus the ledger and exits, (4) prints the ledger diff after. Mode 0750, SPDX header.
- [x] **Step 35** [W] ~6m: `scripts/well/batch-rollback.sh`: given the pre-batch directory, restores both databases with `pg_restore --clean --if-exists` as the owner role (the drill script's path), then prints `well_schema_migrations` tail. Refuses if the directory lacks both dumps.
- [x] **Step 36** [TEST][W] ~8m: `scripts/well/test-batch-apply.sh`: docker Well at 029 → `batch-apply.sh --dry-run` lists 030-035 → apply → ledger has them → `batch-rollback.sh` → ledger back at 029 → apply again idempotent. Add it to the suite list that `scripts/ci-local.sh` or the Makefile runs (grep for `test-migrate.sh` and add beside it).
- [x] **Step 37** [TEST][B] ~4m
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/well/test-batch-apply.sh 2>&1 | tail -6; bash -n scripts/well/batch-apply.sh scripts/well/batch-rollback.sh && echo "syntax ok"
  ```
- [x] **Step 38** [V] ~1m: PASS; `syntax ok`. If rollback leaves 030 rows → Step 39.
- [x] **Step 39** [D] ~5m: The ledger lives in the migrated database; `--clean` must drop and recreate `well_schema_migrations` too. Check `pg_restore -l` lists it; add `--no-owner` only if role mismatch, never `--superuser`.
- [x] **Step 40** [DECIDE][DESIGN][W] ~3m: **M-075 kanban_tasks soft-delete purge.** RECOMMENDATION: amend ADR-011 Q4 to "soft-deleted cards are purged by the retention timer after 90 days" and let Phase 4's retention job own it (no separate purge). Rationale: one purge mechanism; the retention policy already names a 90-day class. Override ONLY if the policy ratification (E-C) picks a different age. Write the amendment paragraph into `docs/adr/ADR-011*.md` under "Amendments".
- [x] **Step 41** [W] ~4m: `scripts/well/index-drops-after-2026-11-15.sh`: refuses to run before 2026-11-15 (`date -u +%s` check), runs `docs/battle-plans/well-health-2026-10-15/well_health.sql` (confirm the path with `ls docs/battle-plans/well-health-2026-10-15/`) against the Well, prints indexes still at `idx_scan = 0`, and emits `DROP INDEX CONCURRENTLY` statements to stdout only (never executes). M-076.
- [x] **Step 42** [B] ~1m
  ```bash
  cd /home/govan/tmp/unheaded && bash -n scripts/well/index-drops-after-2026-11-15.sh && bash scripts/well/index-drops-after-2026-11-15.sh 2>&1 | head -2
  ```
- [x] **Step 43** [V] ~1m: syntax ok; output is the "not before 2026-11-15" refusal. Record the follow-on in the progress file: run it in the first live window after 2026-11-15 and commit the DROP list as a 03x migration.
- [ ] **Step 44** [ESCALATE] **E-A** ~0m: Context: 030-034 are tested, `batch-apply.sh --dry-run` lists them plus 035 (and 036-038 from Phases 2-4 once written; this escalation covers the whole sprint's batch, applied once at the end of Phase 4 or in any window Stevie opens). Question: when is the window, and does the Well backup timer's last run (`systemctl list-timers unheaded-well-backup.timer`) count or do we run `batch-apply.sh`'s own dump? Why Stevie: live data, downtime for kanban/timeguru while they restart on the ledger check.
- [ ] **Step 45** [AFTER-E-A][LIVE-RO][B] ~1m: Pre-batch dry run against the live Well
  ```bash
  cd /home/govan/tmp/unheaded && WELL_MIGRATE_PASSWORD="$(grep '^WELL_MIGRATE_PASSWORD=' .env | cut -d= -f2-)" bash scripts/well/batch-apply.sh --dry-run 2>&1 | tail -12
  ```
- [ ] **Step 46** [AFTER-E-A][V] ~1m: the pending list is exactly the files this sprint added plus 030-034; nothing older. If an older file appears → STOP, it means the live ledger disagrees with the repo; investigate before applying.
- [ ] **Step 47** [AFTER-E-A][LIVE-RW][S][BARE-METAL][B] ~6m: Apply
  ```bash
  cd /home/govan/tmp/unheaded && WELL_MIGRATE_PASSWORD="$(grep '^WELL_MIGRATE_PASSWORD=' .env | cut -d= -f2-)" bash scripts/well/batch-apply.sh 2>&1 | tail -20
  ```
- [ ] **Step 48** [AFTER-E-A][LIVE-RO][V] ~2m: ledger tail shows every file with a sha256; `sudo docker compose restart kanban-app timeguru` then `curl -s localhost:$(grep -oE 'Kanban[A-Za-z]* *= *[0-9]+' pkg/ports/ports.go | grep -oE '[0-9]+$')/health` returns 200 and `docker logs --tail 5 unheaded-kanban-app` shows no `ErrMigrationMissing`. If kanban-app loops → Step 49.
- [ ] **Step 49** [AFTER-E-A][D] ~5m: `docker logs unheaded-kanban-app 2>&1 | grep -i migration | tail -3`; if the file name differs from Step 26's constant, the ledger row names the real file: fix the constant, rebuild, restart. If the batch itself failed → `bash scripts/well/batch-rollback.sh <pre-batch dir>` and STUCK.
- [ ] **Step 50** [V]+[C] ~2m: **PHASE 1 EXIT GATE** — Steps 24, 29, 33, 38, 43 passed; if E-A answered, 46 and 48 passed. DoD-STD 1-8 checked. Commit: `[PLAN S-WELL-SPRING] Steps 32-50: 030-034 batch rehearsed, apply/rollback scripts, index-drop gate, ADR-011 amendment`.

---

## PHASE 2: DEPLOY HISTORY AND CHAOS JOURNAL INTO THE WELL (Steps 51-86)

**Goal**: Sword's deployment history, Yaldabaoth's revert journal and the chaos-controller's packet journal are rows in `unheaded_ops`, written by the service as `ops_writer`, with `pkg/statefile` kept only as the explicit no-Well fallback (M-072, ADR-099 §4 "a journal of what was applied", S-REAL-STEEL follow-on).
**Prerequisite**: Phase 1 Steps 19-43 (ledger helper, suites). Worktree `well-store`. [P:3]
**Time**: 5 h
**Agent**: Agent

### Schema

- [x] **Step 51** [R] ~5m: Read the three record shapes: `sed -n '115,210p' services/sword/sword.go` (StepSpec, AnalysisSpec, StrategySpec, DeployRequest, Deployment), `sed -n '25,60p' services/yaldabaoth/journal.go` (JournalEntry), `grep -n -A20 'type packetJournal' cmd/chaos-controller/packetcmd.go`. List every field and its type in the progress file.
- [x] **Step 52** [DECIDE][DESIGN] ~3m: **Store shape.** RECOMMENDATION: three append-only tables in `unheaded_ops` with a `jsonb spec` column for the strategy/recipe and typed columns for what is queried (id, service, strategy, status, started_at, finished_at, host, actor), plus `deploy_history_transitions` for status changes; `chaos_journal` rows are never deleted (reverted = status column). Rationale: ERDA §5.4 append-only + the retention policy's archive class; jsonb avoids a migration per strategy field; queries are by id/service/time. Override ONLY if Stevie wants a normalized step table (nothing reads steps individually today).
- [x] **Step 53** [W] ~12m: `db/migrations/036_ops_deploy_chaos.sql`: tables `deploy_history`, `deploy_history_transitions`, `chaos_journal` (owner per 025 pattern; `GRANT SELECT, INSERT ON ... TO ops_writer`; `GRANT SELECT TO ops_reader` if that role exists: `grep -n 'ops_reader\|ops_writer' db/migrations/025*.sql`); indexes on `(service, started_at DESC)` and `(status)`; `CHECK (status IN (...))` from the Go constants; an `append_only` trigger copied from the one 019/028 use (grep `change_reason`/`append` for the pattern). MANIFEST line `unheaded_ops 036_ops_deploy_chaos.sql`.
- [x] **Step 54** [TEST][W] ~8m: `scripts/well/test-deploy-journal.sh`: docker Well → migrate → as `ops_writer` INSERT a deployment + 2 transitions + 1 chaos row → UPDATE on `chaos_journal` is refused by the trigger → SELECT by service returns ordered rows → `well_lint.py` clean on 036.
- [x] **Step 55** [TEST][B] ~4m
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/well/test-deploy-journal.sh 2>&1 | tail -6 && bash scripts/check-well-lint.sh 2>&1 | tail -3
  ```
- [x] **Step 56** [V] ~1m: PASS and lint has no new rows (baseline shrink-only). If lint flags nullable defaults → fix 036 (`NOT NULL`, `timestamptz`, identity columns; ERDA 5.7).
- [x] **Step 57** [C] ~1m: `[PLAN S-WELL-SPRING] Steps 51-56: migration 036 deploy_history/chaos_journal + suite`

### Go stores (red → green)

- [x] **Step 58** [CODE][W] ~10m: `pkg/database/deploy_store_test.go`: table-driven against the docker Well when `WELL_TEST_DSN` is set, else `t.Skip` (the pattern `kanban_store_test.go` uses: `grep -n 'Skip' pkg/database/*_test.go | head -3`). Cases: Record → Get round-trips the jsonb spec byte-for-byte; Transition appends and `List(service, limit)` orders newest first; `Record` twice with the same id is a unique-violation error, not silent.
- [x] **Step 59** [CODE][W] ~10m: `pkg/database/chaos_store_test.go`: Append → `Open()` lists only non-reverted; `MarkReverted(id, why)` inserts a transition row (never UPDATEs the original); `OpenOlderThan(d)` for revert-all.
- [x] **Step 60** [TEST][B] ~2m: Expect FAIL
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/database/ -run 'TestDeployStore|TestChaosStore' -count=1 2>&1 | tail -3
  ```
- [x] **Step 61** [V] ~1m: red (`undefined: NewDeployStore`).
- [x] **Step 62** [CODE][W] ~20m: `pkg/database/deploy_store.go` (`NewDeployStore(db)`, `Record`, `Transition`, `Get`, `List`) and `chaos_store.go` (`NewChaosStore(db)`, `Append`, `MarkReverted`, `Open`, `OpenOlderThan`), both following `kanban_store.go`'s style (context first, `sql.ErrNoRows` → typed `ErrNotFound`, no string-built SQL).
- [x] **Step 63** [TEST][B] ~3m: Expect PASS, with the docker Well
  ```bash
  cd /home/govan/tmp/unheaded && WELL_TEST_DSN="$(bash scripts/well/test-config.sh --dsn ops_writer 2>/dev/null || echo '')" go test ./pkg/database/ -run 'TestDeployStore|TestChaosStore' -race -count=1 2>&1 | tail -4
  ```
  (If `test-config.sh` has no `--dsn` flag, read how the other `scripts/well/test-*.sh` build their connection string and export the same.)
- [x] **Step 64** [V] ~1m: green, not skipped (`-v` shows the cases ran). If skipped → the DSN export is wrong: Step 65.
- [x] **Step 65** [D] ~5m: `grep -n 'PGPASSWORD\|psql' scripts/well/test-zhen-captures.sh | head -5` and mirror it. Max 2 attempts.
- [x] **Step 66** [C] ~1m: `[PLAN S-WELL-SPRING] Steps 58-65: DeployStore and ChaosStore on the Well`

### Writers: Sword, Yaldabaoth, chaos-controller

- [x] **Step 67** [CODE][W] ~10m: `services/sword/history.go`: `type History interface { Record(ctx, *Deployment) error; Transition(ctx, id, from, to string, note string) error; Get(ctx, id) (*Deployment, error); List(ctx, service string, n int) ([]*Deployment, error) }`; two implementations: `wellHistory` (wraps `database.DeployStore`) and `fileHistory` (today's statefile code moved verbatim). Test: `history_test.go` runs the same contract against both (file impl always; well impl under `WELL_TEST_DSN`).
- [x] **Step 68** [CODE][W] ~8m: Wire `services/sword/sword.go`: `Config` gains `Well database.Config` (zero value = file mode); `New()` picks `wellHistory` when `cfg.Well.DBName != ""` and logs `history=well` or `history=file`; every `s.file.` call site goes through `s.history`. `ErrNoDir` stays for file mode.
- [x] **Step 69** [CODE][W] ~8m: Same for Yaldabaoth: `services/yaldabaoth/journal.go` → `Journal interface` with `wellJournal` (ChaosStore) and the existing file journal; `chaos-controller revert-all` reads `OpenOlderThan`. The packet journal in `cmd/chaos-controller/packetcmd.go` writes one `chaos_journal` row per injection with `kind='packet'`.
- [x] **Step 70** [CODE][W] ~5m: Compose/env: `docker-compose.yml` sword and chaos-controller services get `WELL_DB=unheaded_ops`, `WELL_USER=ops_writer`, `WELL_PASSWORD_FILE=/run/secrets/...` the same way dashboard-backend gets ops_writer today (`grep -n -B2 -A8 'ops_writer' docker-compose.yml | head -30`); `deploy/systemd/unheaded-sword.service` and `unheaded-chaos-controller.service` read `/etc/unheaded/sword/sword.env` / `/etc/unheaded/chaos/chaos.env` (already their EnvironmentFile). (Stronger after S-IRON-GATE: the password file becomes a service token; nothing here changes then.)
- [x] **Step 71** [TEST][B] ~4m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./services/sword/... ./services/yaldabaoth/... ./cmd/chaos-controller/... -race -count=1 2>&1 | tail -5 && WELL_TEST_DSN="$WELL_TEST_DSN" go test ./services/sword/ -run TestHistoryContract -race -count=1 -v 2>&1 | grep -E '^(=== RUN|--- |ok|FAIL)' | tail -8
  ```
- [x] **Step 72** [V] ~1m: all `ok`; contract ran for both `file` and `well` subtests. If the well subtest skipped → the DSN was not exported in this shell; re-run with Step 63's export.
- [x] **Step 73** [TEST][B] ~3m: Sword in docker against the docker Well, end to end (in-memory Cuirass as the S-REAL-STEEL Phase 7 tests do)
  ```bash
  cd /home/govan/tmp/unheaded && go test ./services/sword/ -run 'TestDeploy.*Well' -race -count=1 2>&1 | tail -3
  ```
  (Write `TestDeployRollingRecordsHistoryInWell` in Step 67 if no such test exists: a rolling deploy against the fake driver leaves 1 `deploy_history` row and ≥ 3 transitions.)
- [x] **Step 74** [V] ~1m: green.
- [x] **Step 75** [C] ~1m: `[PLAN S-WELL-SPRING] Steps 67-74: Sword, Yaldabaoth and chaos-controller write to the Well; file mode kept as fallback`
- [x] **Step 76** [W] ~5m: `docs/adr/ADR-099*.md` "Progress" entry: deploy and chaos journals are Well tables (036), file mode is the documented fallback when `WELL_DB` is unset; `pkg/statefile` remains for architect/micromanager/unheaded-daemon (follow-on, out of this sprint). `wiki/Service-Sword.md` and `wiki/Chaos-Engineering.md`: where history lives now, how to query it (`psql ... -c "SELECT id, service, status, started_at FROM deploy_history ORDER BY started_at DESC LIMIT 10"`).
- [x] **Step 77** [B] ~1m: `cd /home/govan/tmp/unheaded && bash scripts/check-systemd-units.sh 2>&1 | tail -3 && bash scripts/check-compose-hardening.sh 2>&1 | tail -3`
- [ ] **Step 78** [AFTER-E-A][LIVE-RW][S][BARE-METAL][B] ~8m: After the batch (Step 47 included 036): rebuild and restart sword + chaos-controller on west with the Well env (`sudo docker compose up -d --build sword`; `sudo systemctl restart unheaded-chaos-controller`), then one dry-run deploy
  ```bash
  curl -s -X POST localhost:19011/api/v1/deploy -H 'content-type: application/json' -d @services/sword/testdata/rolling-dryrun.json | jq -r '.id' ; docker logs --tail 3 unheaded-sword | grep -E 'history=(well|file)'
  ```
  (Create `testdata/rolling-dryrun.json` in Step 67 if Sword has no dry-run request fixture; a dry run must not touch LXD.)
- [ ] **Step 79** [AFTER-E-A][LIVE-RO][V] ~2m: `history=well`; `psql` as ops_reader (or the owner) `SELECT count(*) FROM deploy_history` increased by 1; `SELECT count(*) FROM chaos_journal` unchanged (dry run, no chaos).
- [ ] **Step 80** [AFTER-E-A][D] ~5m: If `history=file`: the env did not reach the container (`docker inspect unheaded-sword | jq '.[0].Config.Env' | grep WELL`); fix compose, re-up. If `history=well` but count unchanged: the dry-run path returns before `Record`; it must record with `status='dry-run'`.
- [x] **Step 81** [W] ~4m: `scripts/well/test-deploy-journal.sh` gains the revert-all case: 3 open chaos rows → `chaos-controller revert-all` (against the docker Well, fake injector) → 3 transition rows `reverted`, originals untouched.
- [x] **Step 82** [TEST][B] ~3m: `cd /home/govan/tmp/unheaded && bash scripts/well/test-deploy-journal.sh 2>&1 | tail -4`
- [x] **Step 83** [V] ~1m: PASS.
- [x] **Step 84** [SECURITY][R] ~3m: Confirm no row stores a packet payload or user data: `grep -nE 'payload|body|headers' pkg/database/deploy_store.go pkg/database/chaos_store.go db/migrations/036_ops_deploy_chaos.sql` returns nothing; the jsonb spec is the strategy/recipe only (DoD-STD 8).
- [x] **Step 85** [V] ~1m: nothing matched.
- [x] **Step 86** [V]+[C] ~2m: **PHASE 2 EXIT GATE** — Steps 56, 64, 72, 74, 83, 85 passed (79 if E-A answered). Commit: `[PLAN S-WELL-SPRING] Steps 76-86: ADR-099 progress, wiki, revert-all on the Well, security check`.

---

## PHASE 3: SLOs AS DATA (Steps 87-110)

**Goal**: SLO definitions are rows in The Well with `policy.yaml` as the reviewable source; `alert-policy -check` fails when the two drift; compliance has an `slo` evidence kind (M-077; ADR-100 Phase 2). [P:2]
**Prerequisite**: Phase 1 Steps 19-43. Worktree `slo-data`.
**Time**: 3 h
**Agent**: Agent

- [x] **Step 87** [R] ~4m: `sed -n '60,140p' observability/alerting/policy.yaml` and `grep -n 'type SLO\|type Policy' -A14 cmd/alert-policy/*.go | head -60`: list every SLO field (name, objective, window, sli expr, urgency labels, runbook).
- [x] **Step 88** [W] ~10m: `db/migrations/037_ops_slo_definitions.sql`: `slo_definitions(name text primary key, objective numeric(6,4) not null check (objective > 0 and objective < 1), window interval not null, sli_good_expr text not null, sli_total_expr text, urgency text not null check (urgency in ('page','ticket','none')), runbook_url text not null, source_sha256 text not null, updated_at timestamptz not null default now())` plus `slo_definitions_history` append-only via the same trigger pattern as 036. Grants: `ops_writer` INSERT/UPDATE; readers SELECT. MANIFEST `unheaded_ops 037_ops_slo_definitions.sql`.
- [x] **Step 89** [TEST][W] ~6m: `scripts/well/test-slo-definitions.sh`: migrate → insert 7 rows from a fixture → UPDATE objective writes a history row → objective 1.0 refused by CHECK.
- [x] **Step 90** [TEST][B] ~3m: `cd /home/govan/tmp/unheaded && bash scripts/well/test-slo-definitions.sh 2>&1 | tail -4 && bash scripts/check-well-lint.sh 2>&1 | tail -2`
- [x] **Step 91** [V] ~1m: PASS; lint clean.
- [x] **Step 92** [C] ~1m: `[PLAN S-WELL-SPRING] Steps 87-91: migration 037 slo_definitions`
- [x] **Step 93** [CODE][W] ~8m: `cmd/alert-policy/well_test.go`: with `WELL_TEST_DSN`, `-sync-well` upserts the YAML SLOs into `slo_definitions` (sha256 of the policy file recorded); `-check` with the Well reachable fails when a row differs from YAML (`drift: slo wotan objective well=0.99 yaml=0.995`) and passes when equal; without a DSN `-check` behaves as today.
- [x] **Step 94** [TEST][B] ~1m: expect FAIL (`unknown flag -sync-well`)
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/alert-policy/ -run TestWell -count=1 2>&1 | tail -3
  ```
- [x] **Step 95** [V] ~1m: red.
- [x] **Step 96** [CODE][W] ~15m: implement `-sync-well` and the `-check` drift leg in `cmd/alert-policy` using `database.OpsWriterConfig()` + a new `pkg/database/slo_store.go` (`Upsert`, `All`); connection only when `WELL_DB` env is set; errors are loud, a missing Well with `WELL_DB` set is a check failure (fail closed), no `WELL_DB` means YAML-only (today).
- [x] **Step 97** [TEST][B] ~3m
  ```bash
  cd /home/govan/tmp/unheaded && WELL_TEST_DSN="$WELL_TEST_DSN" go test ./cmd/alert-policy/ ./pkg/database/ -run 'TestWell|TestSLOStore' -race -count=1 2>&1 | tail -4 && go run ./cmd/alert-policy -check && echo "check ok (yaml-only)"
  ```
- [x] **Step 98** [V] ~1m: green; `check ok (yaml-only)`.
- [x] **Step 99** [C] ~1m: `[PLAN S-WELL-SPRING] Steps 93-98: alert-policy -sync-well and Well drift check`
- [x] **Step 100** [CODE][W] ~8m: `pkg/compliance/crosswalk/collect/slo_test.go`: evidence kind `slo` → for each SLO row, a record `slo:<name>` PASS when `slo:error_budget_remaining{slo=<name>} > 0` over the window from Victoria (URL from env `VICTORIA_URL`, default `http://localhost:8428`), FAIL when ≤ 0, NOT_ASSESSED when the series is absent. Fake HTTP server in the test.
- [x] **Step 101** [TEST][B] ~1m: expect FAIL (`undefined: SLO`)
- [x] **Step 102** [CODE][W] ~12m: `collect/slo.go` + register the kind where `agent.go`'s kind is registered (`grep -rn '"host-agent"' pkg/compliance/crosswalk | head -3`); add control `UH-SLO-01` ("Service-level objectives are declared as data and their error budgets are measured") to `compliance/catalog/controls.yaml` with mappings (800-53 CA-7 continuous monitoring, SI-4; CSF DE.CM-01; ISO A.8.16) and `rationale`; `evidence: [{kind: slo, ref: "*"}]` following the catalog's ref syntax (read two existing entries first).
- [x] **Step 103** [TEST][B] ~3m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/compliance/... -race -count=1 2>&1 | tail -4 && go run ./cmd/compliance-evidence 2>&1 | grep -E 'UH-SLO-01|catalog' | head -3
  ```
- [x] **Step 104** [V] ~1m: green; `UH-SLO-01` appears (NOT_ASSESSED or PASS depending on the stack). Catalog loader accepted the new control (no `catalog:` error).
- [ ] **Step 105** [AFTER-E-A][LIVE-RW][B] ~2m: After the batch (037 included): `cd /home/govan/tmp/unheaded && WELL_DB=unheaded_ops WELL_USER=ops_writer WELL_PASSWORD="$(grep '^OPS_WRITER_PASSWORD=' .env | cut -d= -f2-)" go run ./cmd/alert-policy -sync-well 2>&1 | tail -3` (confirm the env variable names against `pkg/database/config.go:80-100` first).
- [ ] **Step 106** [AFTER-E-A][LIVE-RO][V] ~1m: 7 rows in `slo_definitions`; `go run ./cmd/alert-policy -check` with the same env prints no drift; `go run ./cmd/compliance-evidence | grep UH-SLO-01` is PASS (stack up, budgets > 0).
- [x] **Step 107** [W] ~4m: `security.yml` CI: the existing `alert-policy -check` step stays YAML-only in CI (no Well in GitHub); add a comment line saying the Well leg runs on west via `scripts/ci-local.sh` and add it there under the Well suites.
- [x] **Step 108** [W] ~4m: ADR-100 "Phase 2" row → built: table 037, `-sync-well`, drift check, `slo` evidence kind; the YAML stays the reviewable source and the Well is what the dashboard and compliance read. (After S-IRON-GATE: dashboard admin can edit objectives with a signed change record, ADR-099 §5; not here.)
- [x] **Step 109** [B] ~1m: `cd /home/govan/tmp/unheaded && go run ./cmd/alert-policy -check && git status --short | head`
- [x] **Step 110** [V]+[C] ~2m: **PHASE 3 EXIT GATE** — Steps 91, 98, 104 passed (106 if E-A). Commit: `[PLAN S-WELL-SPRING] Steps 100-110: slo evidence kind, UH-SLO-01, ADR-100 Phase 2 recorded`.

---

## PHASE 4: RETENTION AS TIMERS (Steps 111-132)

**Goal**: the retention policy's classes become one `well-retention` job with archive-then-purge, DELETE grants scoped to the tables it owns, PII redaction before anything reaches `_pg_log`, and a weekly timer; the policy itself goes to Stevie for ratification (M-074, M-075 via Step 40, ERDA 5.4 trigger check).
**Prerequisite**: Phase 1 Steps 19-43.
**Time**: 3 h
**Agent**: Coordinator (E-C) / Agent for 111-126

- [ ] **Step 111** [R] ~5m: `sed -n '1,120p' docs/policy/retention-policy-v1-2026-05-06.md`: list each class, its online age, archive age, and which tables fall in it (zhen_conversations, service_health_*, compliance_findings_history, audit_events, kanban soft-deletes, llm_requests from 030, zhen_captures from 033, agent_runs from 032).
- [ ] **Step 112** [LIVE-RO][B] ~2m: Volumes today, for the ERDA 5.4 trigger
  ```bash
  cd /home/govan/tmp/unheaded && for db in unheaded_app unheaded_ops; do sudo docker exec unheaded-postgres psql -U postgres -d $db -At -c "SELECT relname, n_live_tup, pg_size_pretty(pg_total_relation_size(relid)) FROM pg_stat_user_tables ORDER BY n_live_tup DESC LIMIT 8"; done 2>&1 | head -20
  ```
- [ ] **Step 113** [W][DESIGN] ~10m: `docs/policy/retention-policy-v1-2026-05-06.md` gains a "Proposed for ratification 2026-10-27" table: class → tables → online age → archive age → action (`archive+purge` / `partition-detach` / `never`), with the defaults: conversations 180 d online then purge (no archive: user text), captures 365 d, llm_requests 90 d, service_health_transitions 365 d, kanban soft-deleted 90 d purge, compliance_findings_history and audit never deleted (partition when ERDA 5.4 trigger fires). Mark which values are the sweeps' defaults. This is E-C's reading material.
- [ ] **Step 114** [W] ~10m: `db/migrations/038_ops_retention.sql`: role `retention_runner` (nologin, `GRANT DELETE ON zhen_conversations, zhen_captures, llm_requests, service_health_transitions TO retention_runner` in their databases: this may need two files if tables span `unheaded_app` and `unheaded_ops`; route each in MANIFEST), table `retention_runs(id identity, started_at, finished_at, class text, deleted bigint, archived bigint, dry_run bool)` append-only, and `GRANT INSERT ON retention_runs TO retention_runner`. Kanban purge: `DELETE FROM kanban_tasks WHERE deleted_at < now() - interval '90 days'` requires DELETE on kanban_tasks for `retention_runner` only.
- [ ] **Step 115** [TEST][W] ~6m: `scripts/well/test-retention.sh`: migrate → seed 3 old + 2 new conversations → run `well-retention -dry-run` (counts, deletes nothing) → run live → old gone, new kept, `retention_runs` has 2 rows → `retention_runner` cannot DELETE from `compliance_findings_history` (permission denied).
- [ ] **Step 116** [CODE][W] ~15m: `cmd/well-retention/main.go` (+ `main_test.go` unit tests for the age math and the class table parsing): reads the classes from a small YAML `compliance/retention/classes.yaml` (generated from the policy table in Step 113; one source), connects as `retention_runner` via `database.Config` from env, runs each class in a transaction with `statement_timeout`, `-dry-run`, `-class` filter, exit 0 on success, logs counts, writes `retention_runs`. PII: before any class that writes to a `_pg_log` or archive table, pass rows through `redact.Text()` (`grep -rn 'func Redact\|package redact' pkg | head -3`; the Champion's redactor exists for prompts, reuse it, do not write a second).
- [ ] **Step 117** [TEST][B] ~4m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/well-retention/ -race -count=1 2>&1 | tail -3 && bash scripts/well/test-retention.sh 2>&1 | tail -6
  ```
- [ ] **Step 118** [V] ~1m: green; PASS; the permission-denied case is asserted (fail closed on tables the policy says never delete).
- [ ] **Step 119** [C] ~1m: `[PLAN S-WELL-SPRING] Steps 111-118: migration 038 retention role/runs, well-retention binary and suite`
- [ ] **Step 120** [W] ~6m: `deploy/systemd/unheaded-well-retention.service` + `.timer` (weekly, `OnCalendar=Sun 04:10`, after `unheaded-well-backup.service`, `User=unheaded-well-retention` with a sysusers line in the same file the other well units use: `grep -rn 'unheaded-well' deploy/systemd/*.conf nixos/ debian/ 2>/dev/null | head -3`; uid from `pkg/uids` next free, add it there), `EnvironmentFile=/etc/unheaded/well/retention.env`, `ExecStart=/usr/local/bin/well-retention`, the hardening block copied from `unheaded-well-drill.service`.
- [ ] **Step 121** [B] ~1m: `cd /home/govan/tmp/unheaded && bash scripts/check-systemd-units.sh 2>&1 | tail -3 && go test ./pkg/uids/ -count=1 2>&1 | tail -2`
- [ ] **Step 122** [V] ~1m: units pass the gate (no `User=root`, hardening present); uids registry test green (no collision).
- [ ] **Step 123** [W] ~4m: `scripts/build-all-debs.sh` / `debian/` include `well-retention` + unit (follow how `unheaded-well-drill` is packaged: `grep -rn 'well-drill' debian scripts/build-all-debs.sh | head -5`).
- [ ] **Step 124** [B] ~5m: `cd /home/govan/tmp/unheaded && bash scripts/build-all-debs.sh --only unheaded-well 2>&1 | tail -4 || bash scripts/build-all-debs.sh 2>&1 | tail -4; ls dist/*.deb 2>/dev/null | grep -i well`
- [ ] **Step 125** [V] ~1m: a deb containing `/usr/local/bin/well-retention` and the timer (`dpkg -c dist/<deb> | grep -E 'retention'`).
- [ ] **Step 126** [W] ~5m: `wiki/Well-Backups.md` (or `wiki/The-Well.md`): retention section: classes, timer, dry-run command, where `retention_runs` is, and that audit rows are never deleted (ops facts live in wiki, memory feedback_ops_facts_in_wiki).
- [ ] **Step 127** [AFTER-E-A][LIVE-RW][S][B] ~3m: 038 is in the batch (Step 47). Then on west: `sudo install -m 0640 -o root -g unheaded-well-retention /dev/null /etc/unheaded/well/retention.env` and write `WELL_DB`, `WELL_USER=retention_runner`, `WELL_PASSWORD=<set with scripts/well/set-role-passwords.sh retention_runner>` (password generated, stored in `.env` per the Well's convention, never in git).
- [ ] **Step 128** [AFTER-E-A][LIVE-RW][S][B] ~2m: `sudo dpkg -i dist/unheaded-well*.deb && sudo systemctl daemon-reload && sudo -u unheaded-well-retention /usr/local/bin/well-retention -dry-run 2>&1 | tail -6`
- [ ] **Step 129** [AFTER-E-A][LIVE-RO][V] ~1m: dry run prints per-class counts and exits 0; `retention_runs` gained a `dry_run=true` row. The timer is NOT enabled until E-C ratifies.
- [ ] **Step 130** [ESCALATE] **E-C** ~0m: Context: Step 113's table is the proposal; the job exists and dry-runs; nothing is deleted until the timer is enabled. Questions: (1) ratify the ages as drafted, or change which? (2) sign the three attestations (drafts: Phase 12 Step 244; `scripts/compliance/sign-attestation.sh <name>` makes the signed single-file commit) once you have read the policy, IR plan and access list. Why Stevie: data deletion and signed statements are the operator's.
- [ ] **Step 131** [AFTER-E-C][LIVE-RW][S][B] ~2m: `sudo systemctl enable --now unheaded-well-retention.timer && systemctl list-timers unheaded-well-retention.timer --no-pager | head -3`; if Stevie changed ages, edit `compliance/retention/classes.yaml` + the policy table first and commit.
- [ ] **Step 132** [V]+[C] ~2m: **PHASE 4 EXIT GATE** — Steps 118, 122, 125 passed (129, 131 if escalations answered). Commit: `[PLAN S-WELL-SPRING] Steps 120-132: retention timer, deb, wiki; policy table proposed for ratification`.

---

## PHASE 5: THE WELL TO POSTGRESQL 18 (Steps 133-144)

**Goal**: the live Well runs PostgreSQL 18 with `ssl_groups` carrying the hybrid group, rehearsed in docker first, applied only under E-B (M-071, FND-015, Quantum Armor E1).
**Prerequisite**: Phase 1 (batch scripts exist; the Well is on the ledger). Not parallel with any live-Well step.
**Time**: 1.5 h (plus the window)
**Agent**: Coordinator

- [ ] **Step 133** [R] ~5m: `cat runbooks/data/well-pg18-upgrade.yaml`: list preconditions, steps, verification, rollback. Note the image tag it pins and the `ssl_groups` value.
- [ ] **Step 134** [B] ~2m: Dry run through the runner
  ```bash
  cd /home/govan/tmp/unheaded && RUNNER=$(git ls-files | grep -E '(^|/)run-runbook\.py$' | head -1) && echo "runner=$RUNNER" && python3 -B "$RUNNER" --dry-run runbooks/data/well-pg18-upgrade.yaml 2>&1 | tail -15
  ```
- [ ] **Step 135** [V] ~1m: dry run exits 0, every step rendered, the first step is the backup. If the runner path is empty → `grep -rn 'dry-run' tests/runbooks/harness.py | head -2` names it.
- [ ] **Step 136** [TEST][B] ~6m: Docker rehearsal: current-schema dump restored into an 18 image, then the PQ groups check
  ```bash
  cd /home/govan/tmp/unheaded && grep -nE 'postgres:1[68]|PG_IMAGE' scripts/well/test-backup-restore.sh scripts/well/test-pq-groups.sh | head -4; PG_IMAGE=postgres:18 bash scripts/well/test-backup-restore.sh 2>&1 | tail -4; bash scripts/well/test-pq-groups.sh 2>&1 | tail -4
  ```
  (If `test-backup-restore.sh` has no `PG_IMAGE` variable, add one defaulting to the current pin; a one-line change.)
- [ ] **Step 137** [V] ~1m: restore into 18 PASS; pq-groups PASS (`X25519MLKEM768` listed). If the restore fails on a 16→18 incompatibility → record it in the runbook's preconditions and STUCK this phase (E-B must not be asked with a failing rehearsal).
- [ ] **Step 138** [ESCALATE] **E-B** ~0m: Context: rehearsal green (Step 137); runbook dry-run green (Step 135); the Well backup timer's last run and the pre-upgrade dump are the rollback. Question: go/no-go and when (≈15 min of Well downtime; kanban, timeguru, dashboard-backend, zhen degrade to read-only or SQLite fallback meanwhile). Why Stevie: downtime on the live ledger.
- [ ] **Step 139** [AFTER-E-B][LIVE-RW][S][BARE-METAL][B] ~4m: Backup first
  ```bash
  sudo systemctl start unheaded-well-backup.service && sleep 20 && journalctl -u unheaded-well-backup.service -n 4 --no-pager && ls -t /srv/backups/unheaded-well/ | head -2
  ```
- [ ] **Step 140** [AFTER-E-B][V] ~1m: a new dated directory with both database dumps; offsite copy logged OK (`journalctl -u unheaded-well-offsite -n 2`). No backup, no upgrade.
- [ ] **Step 141** [AFTER-E-B][LIVE-RW][S][BARE-METAL][B] ~15m: Run the runbook live (the runner's confirm flag is whatever `tests/runbooks/harness.py` shows the live path uses)
  ```bash
  cd /home/govan/tmp/unheaded && RUNNER=$(git ls-files | grep -E '(^|/)run-runbook\.py$' | head -1) && sudo -E python3 -B "$RUNNER" runbooks/data/well-pg18-upgrade.yaml 2>&1 | tail -30
  ```
- [ ] **Step 142** [AFTER-E-B][LIVE-RO][V] ~3m
  ```bash
  sudo docker exec unheaded-postgres psql -U postgres -At -c 'SHOW server_version' -c 'SHOW ssl_groups'; for p in 20001 19000 20000; do printf "%s " $p; curl -s -o /dev/null -w '%{http_code}\n' localhost:$p/health; done; cd /home/govan/tmp/unheaded && bash scripts/check-pq-kex.sh 2>&1 | tail -3
  ```
  `18.x`; `ssl_groups` includes `X25519MLKEM768`; three 200s; pq-kex gate PASS on the Well row.
- [ ] **Step 143** [AFTER-E-B][D] ~10m: If a service stays unhealthy: `docker logs --tail 20 <svc>`; a TLS handshake failure means the client pins a classical group: fix in `pkg/database/config.go` TLS config (Strict profile from `pkg/tls`), rebuild. If the server will not start: runbook `rollback:` section (restore the 16 volume from Step 139's dump), then STUCK with the server log.
- [ ] **Step 144** [V]+[C]+[AUDIT-UPDATE] ~3m: **PHASE 5 EXIT GATE** — Steps 135, 137 passed (140, 142 if E-B). On success: `compliance/findings/register.yaml` FND-015 → fixed with the date and Step 142 as evidence; `go run ./cmd/crypto-inventory 2>&1 | tail -2 && bash scripts/check-crypto-inventory.sh 2>&1 | tail -2` (CBOM shows PG 18 + hybrid group). Commit: `[PLAN S-WELL-SPRING] Steps 133-144: Well on PostgreSQL 18 with hybrid ssl_groups (FND-015)`.

---

## PHASE 6: WATCHDOG AND POSTMORTEMS (Steps 145-160)

**Goal**: a dead-man's switch that notifies when a host stops reporting, and the postmortem practice ADR-100 §5 promised: a template and four written incidents (M-080, M-081; decisions 22, 23). [P:7][P:8]
**Prerequisite**: Phase 0. Worktree `sre-rules`.
**Time**: 2.5 h
**Agent**: Agent (host install of the east timer is Phase 13 under E-D)

- [x] **Step 145** [DECIDE][DESIGN] ~3m: **Watchdog placement.** RECOMMENDATION: west and east watch each other now; an outside receiver later. Mechanism: Prometheus `Watchdog` rule (`vector(1)`, `urgency: none`, visible in Alertmanager so a silent Prometheus is noticed on the Grafana page) plus a heartbeat: east POSTs to west's alert-mailer every 2 min; west raises `HeartbeatMissing{host="east"}` (page) after 10 min without one. West dying is detected by Stevie's own observation until the outside receiver exists (recorded as the gap). Rationale: ADR-098 Q2's default; no third party; no Docker socket; the only new network path is east→west over the P2P link. Override ONLY if Stevie names an outside service now (decision 22).
- [x] **Step 146** [W] ~5m: `monitoring/prometheus/rules/watchdog.yml` (`Watchdog`, `vector(1)`, `urgency: none`, `runbook_url` → `runbooks/observe/heartbeat-missing.yaml`) and `monitoring/prometheus/rules/tests/watchdog_test.yml` asserting it fires at t=0 and keeps firing. Add the file to `prometheus.yml`'s `rule_files`.
- [x] **Step 147** [CODE][W] ~8m: `cmd/alert-mailer/heartbeat_test.go`: `POST /heartbeat/east` with header `X-Heartbeat-Token` equal to the file at `HEARTBEAT_TOKEN_FILE` → 204 and gauge `unheaded_heartbeat_last_seen_timestamp_seconds{host="east"}` updated; wrong token → 401 and no update; unknown host (not in `HEARTBEAT_HOSTS=east,west`) → 404; GET → 405.
- [x] **Step 148** [TEST][B] ~1m: red
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/alert-mailer/ -run TestHeartbeat -count=1 2>&1 | tail -3
  ```
- [x] **Step 149** [CODE][W] ~12m: implement in `cmd/alert-mailer` (same mux as the webhook; constant-time token compare; token file 0400 read at start, fail loud if `HEARTBEAT_HOSTS` set and the file missing; gauge on the existing registry).
- [x] **Step 150** [TEST][B] ~2m: green
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/alert-mailer/ -race -count=1 2>&1 | tail -3
  ```
- [x] **Step 151** [W] ~8m: `deploy/systemd/unheaded-heartbeat.service` + `.timer` (`OnCalendar=*:0/2`, `User=unheaded-heartbeat` nologin with a `pkg/uids` entry and the sysusers line beside the other units; `EnvironmentFile=/etc/unheaded/heartbeat.env` holding `HEARTBEAT_URL=http://192.168.13.2:19130/heartbeat/east` and `HEARTBEAT_TOKEN_FILE`; `ExecStart=/usr/bin/curl -fsS -m 10 -X POST -H "X-Heartbeat-Token: $(cat ...)"` via a 10-line `/usr/local/bin/unheaded-heartbeat` script so no secret lands in the unit; hardening block from `unheaded-well-drill.service`). Compose: alert-mailer gets `HEARTBEAT_HOSTS=east` and the token file as a secret mount (mirror how `alert-mailer` gets its SMTP password file today).
- [x] **Step 152** [W] ~8m: `monitoring/prometheus/rules/heartbeat.yml`: `HeartbeatMissing` = `time() - unheaded_heartbeat_last_seen_timestamp_seconds > 600` for 1m, `urgency: page` via `observability/alerting/policy.yaml` `promote:` with reason "dead-man's switch; the only alert allowed to page on absence"; `tests/heartbeat_test.yml`; `runbooks/observe/heartbeat-missing.yaml` (runbook/v1, `status: draft`: check P2P link, east power, `systemctl status unheaded-heartbeat.timer` on east, west alert-mailer logs).
- [x] **Step 153** [B] ~3m
  ```bash
  cd /home/govan/tmp/unheaded && go run ./cmd/alert-policy && go run ./cmd/alert-policy -check && bash scripts/check-prometheus-rules.sh 2>&1 | tail -3 && bash scripts/check-runbooks.sh 2>&1 | tail -2 && bash scripts/check-systemd-units.sh 2>&1 | tail -2 && go test ./pkg/uids/ -count=1 2>&1 | tail -1
  ```
- [x] **Step 154** [V] ~1m: all PASS; `slo.yml`/`alertmanager.yml` regenerated with the promotion; `check-runbooks` shows one new runbook, 0 new findings.
- [x] **Step 155** [C] ~1m: `[PLAN S-WELL-SPRING] Steps 145-154: Watchdog rule, heartbeat endpoint + east timer, HeartbeatMissing page`
- [x] **Step 156** [DECIDE][DESIGN] ~2m: **Postmortem threshold (ADR-100 Q3).** RECOMMENDATION: a postmortem for any outage of a Stevie-facing service (Wotan, dashboard, the Well, Zhen) longer than 1 h, any silent failure longer than a day, and any data-loss near-miss; never for a single paged alert that resolved inside the hour. Rationale: Stevie's "I might read 5"; the four incidents below all meet it. Override ONLY if Stevie wants every page written up (decision 23).
- [x] **Step 157** [R] ~4m: Names and evidence for the four incidents
  ```bash
  cd /home/govan/tmp/unheaded && sed -n '91,98p' docs/adr/ADR-100*.md; grep -nE 'OOM-killed|dockerd went down|exit 255|auth burst|clippy red' /home/govan/tmp/next.md | head -12
  ```
- [x] **Step 158** [W] ~6m: `docs/incidents/TEMPLATE.md`: blameless; sections Summary, Impact (who/what/how long), Timeline (UTC), Detection (what noticed it, what should have), Root cause, What went well, Action items (each a M-### or a new register row), Threshold check (Step 156). Link from ADR-100 §5 and `wiki/Alerting.md`.
- [x] **Step 159** [W] ~25m: four postmortems from the Step 157 evidence: `docs/incidents/2026-10-18-wotan-oom-silent-outage.md` (OOM-kill 06:42, down 10 h, `restart: "no"` in the host override, nothing at boot; action: FND-024/M-033 pattern, Watchdog Step 145), `docs/incidents/2026-10-13-well-silent-outage.md` (dockerd down, backup failed 04:25, found at the ERDA audit), and the two ADR-100 §5 names from Step 157 (one file each, same template). Every action item cites an M-id or says "done: <commit>".
- [x] **Step 160** [V]+[C] ~2m: **PHASE 6 EXIT GATE** — Steps 150, 154 passed; `ls docs/incidents | wc -l` = 5; each postmortem has every template section (`for f in docs/incidents/2026-*.md; do grep -c '^## ' $f; done` = same count as TEMPLATE). Commit: `[PLAN S-WELL-SPRING] Steps 156-160: postmortem template and four incidents`.

---

## PHASE 7: ALERT RULES TELL THE TRUTH (Steps 161-180)

**Goal**: no rule names a metric nobody emits; the live trace-collector has a rule and a runbook; every alert carries a `runbook_url` that resolves and a gate keeps it so; CI on develop has an SLO; the error budget is visible in Grafana (M-083, M-084, M-087; decision 25). [P:6][P:8]
**Prerequisite**: Phase 0 (monitoring stack up for Step 161). Worktree `sre-rules`.
**Time**: 3.5 h
**Agent**: Agent

- [x] **Step 161** [LIVE-RO][B] ~3m: Which metrics the suspect rules name actually exist
  ```bash
  cd /home/govan/tmp/unheaded && for m in $(grep -hoE '(unheaded|haproxy|nginx)_[a-z_]+' monitoring/prometheus/rules/monad.yml monitoring/prometheus/rules/loadbalancers.yml | sort -u); do n=$(curl -s --max-time 5 "http://localhost:8428/api/v1/series?match[]=$m" | jq '.data|length'); printf "%-45s %s\n" "$m" "$n"; done; curl -s 'http://localhost:8428/api/v1/series?match[]=up{job="node_exporter"}' | jq '.data|length'
  ```
- [x] **Step 162** [V] ~1m: every `unheaded_monad_*`, `unheaded_bgp_*`, `unheaded_bfd_*`, `unheaded_routing_*`, `haproxy_*`, `nginx_*` series count is 0; `up{job="node_exporter"}` ≥ 1. Record the table in the progress file (this is the evidence for deleting).
- [x] **Step 163** [DECIDE][DESIGN] ~2m: **Dead alerts.** RECOMMENDATION: delete the 11 rules with no producer (4 in monad.yml, 7 in loadbalancers.yml), move `NodeExporterDown` into `host.yml` with a unit test, remove both files and the `UNTESTED` baseline entries. Re-add any rule the day its producer ships (haproxy exporter is an S-IRON-GATE/S-CLEAN-SLATE M-035 follow-on). Rationale: a rule nobody can fire is noise in Alertmanager and a false sense of coverage; decision 25 default. Override ONLY if Stevie names a producer to wire instead (frr_exporter for BGP).
- [x] **Step 164** [W] ~8m: apply Step 163: `git rm monitoring/prometheus/rules/monad.yml monitoring/prometheus/rules/loadbalancers.yml`; add `NodeExporterDown` to `host.yml` + a case in `tests/host_test.yml`; `UNTESTED=()` in `scripts/check-prometheus-rules.sh`; drop the two files from `prometheus.yml` `rule_files`; leave `grafana/dashboards/06_loadbalancers.json` (no alert dependency) but add a one-line note in `wiki/Alerting.md` that LB rules return with their exporter.
- [x] **Step 165** [B] ~3m: `cd /home/govan/tmp/unheaded && bash scripts/check-prometheus-rules.sh 2>&1 | tail -4`
- [x] **Step 166** [V] ~1m: PASS; no `UNTESTED` complaints; `host_test.yml` covers `NodeExporterDown`.
- [x] **Step 167** [W] ~8m: Trace-collector coverage: `monitoring/prometheus/prometheus.yml` scrape job `trace-collector` on `16671` (target form copied from the job that scrapes another host process, e.g. node-exporter: `grep -n -B2 -A4 'node_exporter' monitoring/prometheus/prometheus.yml`); `monitoring/prometheus/rules/collector.yml` with `TraceCollectorDown` (`up{job="trace-collector"} == 0` for 5m, `urgency: none` hand rule, promoted to `ticket` in `policy.yaml` with reason) and `TraceCollectorHighLatency` (its own latency histogram if it exports one: `curl -s localhost:16671/metrics | grep -E '^trace_collector_.*(latency|duration)' | head -3`; if none exists, the rule is `urgency: none` on `rate(..._errors_total)` and the latency gap is a one-line finding); `tests/collector_test.yml`.
- [x] **Step 168** [W] ~8m: `runbooks/observe/trace-collector-down.yaml` and `runbooks/observe/high-latency.yaml` (runbook/v1; status draft; steps: `systemctl status unheaded-trace-collector`, `journalctl -u ... -n 50`, BPF program attached `bpftool prog list | grep -c xdp`, ring buffer backlog metric, restart; rollback section).
- [x] **Step 169** [B] ~3m: `cd /home/govan/tmp/unheaded && go run ./cmd/alert-policy && go run ./cmd/alert-policy -check && bash scripts/check-prometheus-rules.sh 2>&1 | tail -3 && bash scripts/check-runbooks.sh 2>&1 | tail -2`
- [x] **Step 170** [V] ~1m: PASS ×3; 81 runbooks (79 + 2), 0 new findings.
- [x] **Step 171** [C] ~1m: `[PLAN S-WELL-SPRING] Steps 161-170: dead alerts deleted, NodeExporterDown kept, trace-collector rule + runbooks`
- [x] **Step 172** [R] ~2m: `grep -h runbook_url monitoring/prometheus/rules/*.yml | sort -u | head -5`: learn the existing URL form (repo path vs wiki URL) and mirror it.
- [x] **Step 173** [CODE][W] ~10m: `runbook_url` gate in `scripts/check-prometheus-rules.sh`: for every `- alert:` block, `annotations.runbook_url` must exist and, when it is a repo path, the file must exist; when it is a URL, its last path segment must match a file under `runbooks/` or `wiki/`. No baseline: the gate starts strict. Add 3 shell test cases to the script's self-test if it has one (`grep -n 'self-test\|SELFTEST' scripts/check-prometheus-rules.sh`), else to `scripts/check-gates-can-fail.sh`'s provocation list so the meta-gate proves it bites.
- [x] **Step 174** [B] ~2m: expect FAIL listing the alerts without a runbook (compliance 9, host ≤ 5, logs 7, network 1, threat-feeds 3, well 6, watchdog/heartbeat/collector 0)
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/check-prometheus-rules.sh 2>&1 | grep -c 'runbook_url' ; bash scripts/check-prometheus-rules.sh >/dev/null 2>&1; echo "exit=$?"
  ```
- [x] **Step 175** [V] ~1m: exit 1 and the count equals the number of alerts lacking the annotation (red, as intended).
- [x] **Step 176** [W] ~20m: add `runbook_url` to every remaining alert, reusing existing runbooks where one fits (`well.yml` → `runbooks/data/postgresql-backup-restore.yaml` / `well-restore-from-east-offsite.yaml`; `logs.yml` → `runbooks/observe/log-rotation.yaml` (rewritten to verify, ADR-092 step 6) ; `threat-feeds.yml` → `runbooks/security/vuln-management-2026-05-06.md` or a new `kev-feed-stale.yaml`; `compliance.yml` → `runbooks/security/compliance-evidence.yaml`; `host.yml` → `runbooks/infra/*` by symptom; `network.yml` → `runbooks/network/*`). Write a new runbook only where nothing fits; list them in the progress file.
- [ ] **Step 177** [B] ~3m: `cd /home/govan/tmp/unheaded && bash scripts/check-prometheus-rules.sh 2>&1 | tail -3 && bash scripts/check-runbooks.sh 2>&1 | tail -2 && bash scripts/check-gates-can-fail.sh --only prometheus-rules 2>&1 | tail -3 || true`
- [x] **Step 178** [V]+[C] ~2m: rules PASS; runbooks 0 new findings; the meta-gate shows the runbook_url provocation bites (or the `--only` flag does not exist and the full meta-gate is left to Phase 14). Commit: `[PLAN S-WELL-SPRING] Steps 172-178: runbook_url gate, every alert has a runbook`.
- [x] **Step 179** [CODE][W] ~20m: CI-red SLO + error-budget panel: `scripts/monitoring/ci-status-textfile.sh` writes `unheaded_ci_run_success{workflow="..."} 0|1` and `unheaded_ci_run_checked_timestamp_seconds` for the latest develop run of each workflow from the public API (`https://api.github.com/repos/unheaded/unheaded/actions/runs?branch=develop&per_page=30`, no token, `jq`), into the node-exporter textfile dir; `deploy/systemd/unheaded-ci-status.{service,timer}` every 10 min as a nologin user (uids entry); `policy.yaml` SLO `ci-develop` (good = every workflow's latest run succeeded, objective 0.99) with runbook `runbooks/observe/ci-red.yaml` (open the run via the API recipe in next.md "Next (in order)" step 1); `go run ./cmd/alert-policy` regenerates. Grafana `monitoring/grafana/dashboards/08_slo_error_budget.json`: one stat tile per `slo:error_budget_remaining{slo}` (thresholds 0 red / 0.25 amber), a burn-rate time series, provisioned the way 01-07 are (`ls monitoring/grafana/provisioning/dashboards/`).
- [x] **Step 180** [V]+[C] ~3m: **PHASE 7 EXIT GATE** — `bash scripts/monitoring/ci-status-textfile.sh /tmp/ci-status-test && cat /tmp/ci-status-test/*.prom | head -8` shows 6 workflows; `go run ./cmd/alert-policy -check`, `check-prometheus-rules.sh`, `check-runbooks.sh`, `check-systemd-units.sh` PASS; `jq . monitoring/grafana/dashboards/08_slo_error_budget.json >/dev/null`. Steps 162, 166, 170, 175, 178 passed. Commit: `[PLAN S-WELL-SPRING] Steps 179-180: ci-develop SLO, textfile collector, error-budget dashboard`.

---

## PHASE 8: METRICS WITHOUT HAND-ROLLING (Steps 181-196)

**Goal**: heimdall-daemon exposes `/metrics`; the five binaries still writing exposition by hand use the registry collectors; metrics nothing sets or serves are gone (M-085, M-086). [P:6][P:7]
**Prerequisite**: Phase 0. Worktree `metrics-clean`.
**Time**: 2.5 h
**Agent**: Agent

- [x] **Step 181** [R] ~4m: `grep -n 'NewCollectorRegistry\|metrics.Handler\|/metrics' cmd/anomaly/*.go | head -6; grep -n '= 19[01][0-9][0-9]' pkg/ports/ports.go | sort -t= -k2 -n | tail -6`: the exposition pattern to copy and the next free port after Akira 19100 (expect 19101).
- [x] **Step 182** [CODE][W] ~6m: `cmd/heimdall-daemon/metrics_test.go`: with `-metrics-addr 127.0.0.1:0`, `GET /metrics` serves `heimdall_drift_published_total`, `..._failed_total`, `..._dropped_total` and `heimdall_scan_duration_seconds` (new histogram around one scan); default addr is `127.0.0.1:ports.HeimdallMetrics`; empty string disables the listener (the unit on a host without a scraper).
- [x] **Step 183** [TEST][B] ~1m: red (`cd /home/govan/tmp/unheaded && go test ./cmd/heimdall-daemon/ -run TestMetrics -count=1 2>&1 | tail -3`).
- [x] **Step 184** [CODE][W] ~10m: implement; `pkg/ports`: `HeimdallMetrics = 19101 // heimdall-daemon /metrics (loopback)` + the port-registry test fixture; `wiki/Port-Registry.md` row.
- [x] **Step 185** [TEST][B] ~2m: green (`go test ./cmd/heimdall-daemon/ ./pkg/ports/ -race -count=1 2>&1 | tail -3`).
- [x] **Step 186** [W] ~4m: `monitoring/prometheus/prometheus.yml` scrape job `heimdall` (west + east targets on 19101 the way node-exporter lists both hosts); a `HeimdallDriftPublishFailing` rule in `collector.yml` (`rate(heimdall_drift_publish_failed_total[15m]) > 0`, urgency none, runbook `runbooks/infra/<existing heimdall/mimir runbook>` — `ls runbooks/infra | grep -iE 'heimdall|mimir|drift'`) + test case.
- [x] **Step 187** [C] ~1m: `[PLAN S-WELL-SPRING] Steps 181-186: heimdall-daemon /metrics on 19101, scrape job, publish-failure rule`
- [x] **Step 188** [CODE][W] ~20m: replace `WriteMetrics` at the five sites: in each binary find its registry (`grep -n 'NewCollectorRegistry\|Register(' cmd/unheaded-daemon/main.go cmd/kanban-app/main.go cmd/sophia/main.go cmd/monad/main.go services/timeguru/cmd/timeguru/main.go`), register `lp.Collectors()` (or `logPublisher.Collectors()`), delete the hand-rolled write; then delete `Publisher.WriteMetrics` from `pkg/logagg/metrics.go` and its test, keeping `TestPublisherCollectors` (add a case asserting the exposition text is byte-identical to what `WriteMetrics` produced for a fixed publisher state, captured before deletion).
- [x] **Step 189** [TEST][B] ~4m
  ```bash
  cd /home/govan/tmp/unheaded && go build ./... && go test ./pkg/logagg/ ./cmd/unheaded-daemon/... ./cmd/kanban-app/... ./cmd/sophia/... ./cmd/monad/... ./services/timeguru/... -race -count=1 2>&1 | tail -8 && grep -rn 'WriteMetrics' --include=*.go . | grep -v _test | wc -l
  ```
- [x] **Step 190** [V] ~1m: build ok; all `ok`; `0` remaining `WriteMetrics` references.
- [x] **Step 191** [DECIDE][DESIGN] ~2m: **Unset and unserved metrics.** RECOMMENDATION: delete `pkg/metrics/pqc_metrics.go` (9 `unheaded_pqc_*` never set; the PQC that exists is TLS negotiation, measured by `check-pq-kex.sh`, not runtime counters) and the 4 unserved `pkg/secrets` metrics; re-add a metric the day a code path sets it. Rationale: a declared metric that is always 0 reads as "healthy" on a dashboard; master list M-086 default. Override ONLY if S-IRON-GATE wires `pkg/secrets` into a served registry first (then keep those 4).
- [x] **Step 192** [W] ~8m: apply Step 191; `grep -rn 'unheaded_pqc_\|pqc_metrics' --include=*.go --include=*.yml --include=*.json . | grep -v _test` must return nothing after (rules, dashboards, docs that referenced them get the reference removed, with a line in `wiki/Post-Quantum-Key-Exchange.md` saying what measures PQ now).
- [x] **Step 193** [B] ~4m: `cd /home/govan/tmp/unheaded && go build ./... && go test ./pkg/metrics/ ./pkg/secrets/... -race -count=1 2>&1 | tail -3 && bash scripts/check-prometheus-rules.sh 2>&1 | tail -2 && grep -rn 'unheaded_pqc_' monitoring docs wiki | wc -l`
- [x] **Step 194** [V] ~1m: green; `0`.
- [x] **Step 195** [TEST][B] ~2m: exposition sanity on one binary without the stack: `cd /home/govan/tmp/unheaded && go test ./pkg/logagg/ -run 'Collectors|Exposition' -v -count=1 2>&1 | grep -E '^(=== RUN|--- PASS|--- FAIL|ok)' | tail -6`
- [x] **Step 196** [V]+[C] ~2m: **PHASE 8 EXIT GATE** — Steps 185, 190, 194, 195 passed. Commit: `[PLAN S-WELL-SPRING] Steps 188-196: registry collectors replace WriteMetrics; unset pqc/secrets metrics deleted`.

---

## PHASE 9: ONE LOG PATH, AND WHAT TOPOLOGY MEANS NOW (Steps 197-210)

**Goal**: container stdout reaches Vector without a Docker socket; the journald caps are the one log cap; east's journal has a shipping unit ready for the host window; ADR-076/086 are re-scoped against Vector instead of built (M-082, M-088; decisions 24, 27).
**Prerequisite**: Phase 0. Coordinator (Steps 208-209 are [LIVE-RW]).
**Time**: 2.5 h
**Agent**: Coordinator

- [x] **Step 197** [R] ~4m: `grep -n -A3 'logging:' docker-compose.yml | head -24; sed -n '143,200p' config/vector/vector.yaml; grep -n 'vector\|clickhouse' docker-compose.yml | head -6`: the current driver and caps (ADR-092 step 1), the sinks, the pinned Vector image tag.
- [x] **Step 198** [DECIDE][DESIGN] ~3m: **Container stdout path.** RECOMMENDATION: compose `logging: driver: journald` (tag `{{.Name}}`) for every unheaded service; Vector's existing `journald` source picks them up (`CONTAINER_NAME` field → `service` label); journald's `SystemMaxUse`/`RuntimeMaxUse` (E-D, `journald-caps` fix) becomes the one cap; `scripts/check-compose-log-caps.sh` learns that `driver: journald` satisfies the cap when the host profile applies `journald-caps`. Rationale: no Docker socket exposure (the socket-proxy option hands Vector every container's env), one pipeline, ADR-092 step 7 already planned the caps; decision 24 default. Override ONLY if Stevie wants per-container json-file caps kept as defence in depth (then both: json-file with caps AND a journald sidecar is not possible; it would be a Vector `docker_logs` source behind a read-only socket proxy).
- [x] **Step 199** [W] ~10m: apply Step 198: a YAML anchor `x-logging-journald` in `docker-compose.yml` merged into every unheaded service's `logging:`; `scripts/check-compose-log-caps.sh` accepts `driver: journald` (and still requires caps for `json-file`); `config/vector/vector.yaml` `journal_rows` remap sets `.service = .CONTAINER_NAME ?? .SYSLOG_IDENTIFIER` and drops the Docker-only noise fields; a fixture line in `config/vector/tests/` for a container entry.
- [x] **Step 200** [B] ~3m: `cd /home/govan/tmp/unheaded && bash scripts/test-vector-config.sh 2>&1 | tail -4 && bash scripts/check-compose-log-caps.sh 2>&1 | tail -2 && bash scripts/check-compose-hardening.sh 2>&1 | tail -2 && sudo docker compose config >/dev/null && echo "compose ok"`
- [x] **Step 201** [V] ~1m: Vector unit tests PASS (the container fixture included), both compose gates PASS, `compose ok`.
- [x] **Step 202** [C] ~1m: `[PLAN S-WELL-SPRING] Steps 197-201: journald logging driver for every service; Vector labels containers`
- [x] **Step 203** [DECIDE][DESIGN] ~3m: **Topology map and Muninn (ADR-076/086).** RECOMMENDATION: do not build either as specified. Re-scope ADR-076 to "topology is a ClickHouse view over Vector's rows plus `pkg/discovery`'s live registry, served by dashboard-backend at `/api/v1/topology`" (one endpoint, no new daemon) and mark ADR-086 Muninn Superseded by Vector (`ops.login_events` become a Vector `journal_auth_*` metric, already emitted). Rationale: both pre-date Vector→ClickHouse and ADR-094; nothing consumes them; decision 27 default. Override ONLY if Stevie still wants the SVG map (then `/viz/topology.html` over the new endpoint is a Small follow-on).
- [x] **Step 204** [W] ~12m: amendments: ADR-092 (step 1 → journald driver + caps; step 7 → the `journald-caps` fix; container stdout path = journald), ADR-076 (re-scoped per Step 203; the `stepSim` demo noted as demo-only), ADR-086 (Status: Superseded; what replaces each promised artifact), ADR-INDEX rows. `wiki/Logging.md` (or the page `grep -rl 'Vector' wiki | head -1` names): one diagram line of the path host journal → Vector → ClickHouse/Victoria, and the compose driver.
- [x] **Step 205** [W] ~12m: East shipping, prepared: `config/vector/vector-east.yaml` (journald source → the same ClickHouse sink over the P2P link `192.168.13.2`, batch + disk buffer under `/var/lib/vector`, password from `VECTOR_SINK_PASSWORD_FILE`), `deploy/systemd/vector-east.service` (`User=vector` nologin + uids entry, `ExecStart=/usr/bin/vector --config /etc/unheaded/vector/vector-east.yaml`, hardening block), and the install recipe in `wiki/Logging.md` (Vector `.deb` pinned by version + sha256 in `scripts/lib/tool-pins.sh`, since it is a new host binary: [COMPLIANCE] record it in `docs/policy/dependency-baseline.txt` only if the baseline covers host tools; otherwise in THIRD_PARTY.md).
- [x] **Step 206** [B] ~3m: validate with the compose-pinned image
  ```bash
  cd /home/govan/tmp/unheaded && IMG=$(grep -oE 'timberio/vector:[A-Za-z0-9._-]+' docker-compose.yml | head -1) && echo "$IMG" && sudo docker run --rm -v "$PWD/config/vector:/cfg:ro" "$IMG" validate --no-environment /cfg/vector-east.yaml 2>&1 | tail -3 && bash scripts/check-systemd-units.sh 2>&1 | tail -2
  ```
- [x] **Step 207** [V] ~1m: `Validated`; units gate PASS.
- [ ] **Step 208** [AFTER-E-D][LIVE-RW][S][BARE-METAL][B] ~3m: restart Vector on west with the new config (ADR-100 Phase 3 metrics) and re-up the services with the journald driver
  ```bash
  cd /home/govan/tmp/unheaded && sudo docker compose up -d --force-recreate vector && sudo docker compose up -d 2>&1 | tail -3 && sleep 90
  ```
- [ ] **Step 209** [AFTER-E-D][LIVE-RO][V] ~2m: `curl -s 'http://localhost:8428/api/v1/query?query=count(vector_journal_volume_bytes_total)' | jq -r '.data.result[0].value[1]'` ≥ 1; `curl -s 'http://localhost:8428/api/v1/series?match[]={service="unheaded-wotan"}' | jq '.data|length'` ≥ 1 (a container's line reached Vector through journald). If 0 → `sudo journalctl -o json -n 1 CONTAINER_NAME=unheaded-wotan | jq -r .CONTAINER_NAME` (driver active?) then `docker logs --tail 10 unheaded-vector`.
- [ ] **Step 210** [V]+[C] ~2m: **PHASE 9 EXIT GATE** — Steps 201, 207 passed (209 if E-D). Commit: `[PLAN S-WELL-SPRING] Steps 203-210: ADR-076/086/092 re-scoped; east Vector unit prepared`.

---

## PHASE 10: MEASURE THE UNMEASURED (Steps 211-226)

**Goal**: the four performance claims the docs make without numbers get a harness and a number: packet→browser e2e, container start, 1000 req/s, the HTTP/3 leg; H1-H8 get verdicts; the AF_XDP label is corrected (M-089, M-090).
**Prerequisite**: Phase 0; the main stack up for Steps 217-220. Worktree `measure`.
**Time**: 4 h
**Agent**: Agent (Steps 217-220 Coordinator, [LIVE-RO] + one [AFTER-E-D])

- [x] **Step 211** [R] ~5m: `sed -n '25,80p' cmd/perf-benchmark/main.go; sed -n '40,63p' docs/PERFORMANCE-S77.md; grep -nE 'e2ePacketTrace|ci-bench|--only' -r docs Makefile scripts | head`: what exists vs what is claimed.
- [x] **Step 212** [CODE][W] ~10m: `cmd/perf-benchmark/scenarios_test.go`: `-scenario` accepts `e2e-trace,container-start,http-load,http3` (and `all`); each scenario returns a `Result{Name, N, P50, P95, P99 time.Duration, ErrorRatio float64, Skipped string}`; the JSON report has a `schema: "perf/v1"` field and a `host`, `commit`, `at` header; unknown scenario → error; `http3` sets `Skipped` with the reason when `curl --version` lacks `HTTP3` and no Go HTTP/3 client is available.
- [x] **Step 213** [TEST][B] ~1m: red (`cd /home/govan/tmp/unheaded && go test ./cmd/perf-benchmark/ -count=1 2>&1 | tail -3`).
- [x] **Step 214** [CODE][W] ~35m: implement: **e2e-trace** publishes a message with a nonce trace_id to a `bench.*` topic on Wotan (`POST /api/v1/topics/<t>/publish` the way `demo-trace-injector` does; the topic must be in `topics.auto_approve` → add `bench.perf` to `configs/wotan.yaml`) and polls dashboard-backend `GET /api/v1/flows?trace_id=<nonce>` (or the SSE `/ws/logs` stream) until it appears; N=`-iterations`; **container-start** POSTs one `steel-echo` instance through Cuirass `POST /api/v1/reconcile` and polls `GET /api/v1/state` until healthy, then deletes it ([BARE-METAL]; refuses unless `-allow-lxd`); **http-load** runs 1000 req/s for `-duration` (default 60 s) against `-health` with a token bucket, records error ratio + latency; **http3** uses `curl --http3 -w %{time_total}` against the gateway 21443 when available. All loops honour `ctx` cancellation.
- [x] **Step 215** [TEST][B] ~2m: green (`go test ./cmd/perf-benchmark/ -race -count=1 2>&1 | tail -3`).
- [x] **Step 216** [W] ~8m: `scripts/check-perf-regression.sh`: compares a `perf/v1` report with `docs/policy/perf-baseline.json`: p99 may not grow more than 20 % per scenario, error ratio may not grow; `--update` writes the baseline (shrink-only is not the right shape here: faster is allowed, slower fails). Wired into `scripts/ci-local.sh`'s west-only section (needs the stack), not GitHub.
- [ ] **Step 217** [LIVE-RO][B] ~8m: Measure on west (stack up; publishes only to `bench.perf`; GETs otherwise)
  ```bash
  cd /home/govan/tmp/unheaded && go run ./cmd/perf-benchmark -scenario e2e-trace,http-load,http3 -iterations 200 -duration 60s -output /tmp/perf-$(date -u +%Y%m%d).json 2>&1 | tail -12 && jq '.results[] | {name, p99, error_ratio, skipped}' /tmp/perf-$(date -u +%Y%m%d).json
  ```
- [ ] **Step 218** [V] ~2m: e2e-trace has a p99 (any value; the number is the deliverable), http-load error ratio < 0.01 at 1000 req/s or the real ceiling is recorded, http3 measured or `skipped` with the reason. No scenario crashed.
- [ ] **Step 219** [AFTER-E-D][LIVE-RW][BARE-METAL][B] ~6m: container-start inside the host window (creates and deletes one `steel-echo` instance in LXD project `unheaded` via Cuirass; `CUIRASS_ENFORCE=true` window per `wiki/Containers.md`)
  ```bash
  cd /home/govan/tmp/unheaded && go run ./cmd/perf-benchmark -scenario container-start -allow-lxd -iterations 5 -output /tmp/perf-cs.json 2>&1 | tail -6 && jq '.results[]|{name,p50,p99}' /tmp/perf-cs.json; lxc list --project unheaded --format csv | grep -c steel-echo-bench || true
  ```
- [ ] **Step 220** [AFTER-E-D][V] ~1m: five starts measured; `0` bench instances left behind.
- [x] **Step 221** [W] ~10m: `docs/BENCHMARKS.md`: a dated 2026-10 table with Steps 217/219 numbers (hostname, commit, stack version); lines 80 and 96: "AF_XDP 920K pps" → "Python `sendto` sender ceiling, 920K pps (not AF_XDP; the AF_XDP product path is M-187)"; `CLAUDE.md:829` same correction; `docs/PERFORMANCE-S77.md`: banner "S77 figures were never measured by the named harness (it did not exist); see BENCHMARKS.md 2026-10" and the `e2ePacketTrace`/`ci-bench`/`--only` references replaced by the real `perf-benchmark` flags.
- [x] **Step 222** [W] ~12m: `docs/research/VERDICTS.md`: a verdict line per H1-H8 (`SUPPORTED` / `REFUTED` / `UNTESTED: needs <x>`) citing BENCHMARKS.md rows and this run (H8 end-to-end trace = Step 217; H3 service recovery = game day 1 MTTR 24.4 s; H4/H7 WireGuard = `-east-p2p/-east-wg` scenarios if present; H5 LLM throughput = the 60 tok/s llama.cpp figure with its date); "Verdict History" gains the 2026-10-27 row.
- [x] **Step 223** [B] ~1m: `cd /home/govan/tmp/unheaded && grep -cE '^\*\*Verdict\*\*|^- \*\*Verdict' docs/research/VERDICTS.md; grep -c '920K pps (AF_XDP)' docs/BENCHMARKS.md CLAUDE.md`
- [x] **Step 224** [V] ~1m: 8 verdict lines; `0` remaining wrong labels.
- [ ] **Step 225** [W] ~2m: `docs/policy/perf-baseline.json` from Step 217 (and 219 when run): `bash scripts/check-perf-regression.sh --update /tmp/perf-*.json`; then `bash scripts/check-perf-regression.sh /tmp/perf-*.json` PASS.
- [ ] **Step 226** [V]+[C] ~2m: **PHASE 10 EXIT GATE** — Steps 215, 218, 224 passed (220 if E-D); baseline committed. Commit: `[PLAN S-WELL-SPRING] Steps 211-226: perf-benchmark scenarios, measured numbers, VERDICTS scored, AF_XDP label fixed`.

---

## PHASE 11: AKIRA REPORTERS AND DETECTION RULES (Steps 227-242)

**Goal**: every service reports its own health so Akira's ballot has voters; a remediate topic exists; the cross-host two-reporter ballot is recorded; detection rules exist for what has a producer and are honestly marked UNTESTED where none does; threat models carry a review cadence and the Tomb's campaigns are registered (M-200, M-203, M-205; ADR-029, ADR-055, ADR-062). [P:12]
**Prerequisite**: Phase 0. Worktree `akira`.
**Time**: 3.5 h
**Agent**: Agent (Steps 235-236 Coordinator, [AFTER-E-D])

- [x] **Step 227** [R] ~5m: `grep -n 'system.health' cmd/akira/main.go | head; grep -n 'type .*Report' -A12 cmd/akira/main.go | head -40; ls pkg/health 2>/dev/null || echo "pkg/health gone (E1)"; grep -rln 'discovery.Start(' --include=*.go cmd services | wc -l`: Akira's report shape, where the type lives, and the 11 service entry points that will host the reporter.
- [x] **Step 228** [CODE][W] ~10m: `pkg/healthreport/reporter_test.go`: `Start(ctx, pub Publisher, Config{Service, Host, Interval, Probe func(ctx) Status})` publishes one `system.health.reports` message per interval whose JSON decodes into Akira's report struct (contract test imports the struct from its home, moved to `pkg/healthreport` in Step 230 so Akira and the reporter share it); stops on ctx; a probe error reports `status: "error"` with the message truncated to 200 bytes; a nil Publisher is a programming error (panic in `Start`, not silent).
- [x] **Step 229** [TEST][B] ~1m: red (`cd /home/govan/tmp/unheaded && go test ./pkg/healthreport/ -count=1 2>&1 | tail -3`).
- [x] **Step 230** [CODE][W] ~25m: implement `pkg/healthreport`; move Akira's report/consensus types there and import them back in `cmd/akira`; wire `healthreport.Start` beside each `discovery.Start` call (11 sites) with the service's own `/health` handler as the probe (every service already has one); interval 30 s; `configs/wotan.yaml` `topics.auto_approve` gains each service for `system.health.reports` (the reporter publishes under the service's name, same as discovery).
- [x] **Step 231** [TEST][B] ~4m
  ```bash
  cd /home/govan/tmp/unheaded && go build ./... && go test ./pkg/healthreport/ ./cmd/akira/ -race -count=1 2>&1 | tail -3 && go test ./services/... ./cmd/... -run 'Health|Report|Start' -count=1 2>&1 | grep -E '^(FAIL|ok)' | grep -c FAIL
  ```
- [x] **Step 232** [V] ~1m: green; `0` FAIL.
- [x] **Step 233** [CODE][W] ~12m: Akira remediate path (test first in `cmd/akira/remediate_test.go`): when the ballot for `(service, host)` enters CRITICAL (≥ 62.5 %) Akira publishes `system.health.remediate` `{service, host, band, pct, suggested: "systemctl restart unheaded-<service>.service", at}` once per band entry (not per sweep) and increments `akira_remediate_suggested_total{service}`; `--remediate` keeps its existing restart behaviour and now also publishes; the topic is added to `configs/wotan.yaml` and `docs/protocol/WOTAN_TOPICS.md` (or the registry file `grep -rl 'system.health.consensus' docs wiki | head -2` names).
- [x] **Step 234** [TEST][B]+[C] ~3m: `go test ./cmd/akira/ -race -count=1 2>&1 | tail -2` green → commit `[PLAN S-WELL-SPRING] Steps 227-234: per-service health reporter, shared report types, system.health.remediate`.
- [ ] **Step 235** [AFTER-E-D][LIVE-RW][S][BARE-METAL][B] ~6m: the reporter ships inside the rebuilt debs/compose images of the host window (Phase 13 Step 281). Then, west + east up ≥ 2 min:
  ```bash
  curl -s 'http://localhost:18000/api/v1/topics/system.health.reports/messages?limit=60' | jq -r '.[] | .payload | fromjson? | "\(.host) \(.reporter // .service)"' | sort -u | head -20; curl -s 'http://localhost:18000/api/v1/topics/system.health.consensus/messages?limit=3' | jq -c '.[0]'
  ```
  (Adjust the read endpoint to what `wiki/Service-Wotan.md` documents for reading a topic; GET only.)
- [ ] **Step 236** [AFTER-E-D][V] ~2m: reporters from two hosts (`west <svc>` and `east <svc>` lines); one consensus message. Paste the consensus JSON into `docs/adr/ADR-029*.md` under "Cross-host ballot recorded 2026-10-xx" (ADR H-03 closed).
- [x] **Step 237** [DECIDE][DESIGN] ~2m: **ADR-055 Phase 2 pollers.** RECOMMENDATION: declare KEV-only; govulncheck, cargo-audit, pip-audit and grype already consume NVD/GHSA data on every push; a second NVD poller duplicates them without a consumer. Amend ADR-055 "Phase 2" to "closed, KEV-only; revisit when a consumer for raw NVD rows exists". Rationale: S13 judgment, master M-203 default. Override ONLY if Stevie wants NVD rows in the Well for the dashboard.
- [x] **Step 238** [W] ~25m: detection: `config/vector/vector.yaml` adds `journal_ssh` filter + `log_to_metric` → `unheaded_ssh_accepted_total{user, src}` from `sshd` "Accepted" lines (src is a LAN address; user is the operator login; metadata only, no payload) with a test fixture; `monitoring/prometheus/rules/detection.yml`: `SSHLoginNewSource` (`increase(unheaded_ssh_accepted_total[10m]) > 0 unless on (src) increase(unheaded_ssh_accepted_total[30d] offset 10m) > 0`), `SSHAuthFailureBurst` (on the existing `journal_auth_failures` metric, `rate > 0.2/s for 5m`), `OutboundVolumeDeviation` (`rate(node_network_transmit_bytes_total{device=~"enp.*"}[1h]) > 3 * quantile_over_time(0.5, rate(...[1h])[7d:1h])`); all `urgency: none`, `SSHLoginNewSource` promoted to `ticket` in `policy.yaml` with reason; `tests/detection_test.yml`; runbooks `runbooks/security/ssh-login-new-source.yaml`, `runbooks/security/outbound-volume-deviation.yaml` (burst → existing `ssh-hardening.yaml`). DGA / beaconing / DNS tunnelling: a comment block at the top of `detection.yml` and a row in `docs/security/threat-register.md`: "UNTESTED: no DNS query log source; needs pihole FTL log shipped by Vector (pihole runs in LXD `default` project; mount `/var/log/pihole/pihole.log` read-only into a Vector file source)" with the M-203 id.
- [x] **Step 239** [B] ~3m: `cd /home/govan/tmp/unheaded && bash scripts/test-vector-config.sh 2>&1 | tail -3 && go run ./cmd/alert-policy && go run ./cmd/alert-policy -check && bash scripts/check-prometheus-rules.sh 2>&1 | tail -2 && bash scripts/check-runbooks.sh 2>&1 | tail -2`
- [x] **Step 240** [V] ~1m: all PASS (runbook_url gate from Phase 7 included: every detection alert has one).
- [x] **Step 241** [W] ~20m: M-205: append `## Review cadence` (quarterly; owner Stevie; "Last reviewed 2026-10-27, S-WELL-SPRING Phase 11: <one line of what changed>") to each of the 8 `docs/security/*threat-model*.md` and `threat-register.md`; `threat-register.md` row for RT-32 CPU_MAP concurrent-write disposition (read `grep -rn 'CPU_MAP' tomb/grimoire/threat-models/*.md | head -3` and record what is true: single-writer today, lab-accepted, re-open with DISTRIBUTED); ADR-062 §"Existing campaign inventory": one row per `tomb/lich/LICH-*` directory missing from the table (`ls tomb/lich | grep -E '^LICH-[0-9]+' | sort` vs the table's ids) plus the planned `LICH-0NN-baseline-agent` row (Phase 13 Step 279, status planned); ADR-055 amendment from Step 237.
- [x] **Step 242** [V]+[C] ~2m: **PHASE 11 EXIT GATE** — Steps 232, 234, 240 passed (236 if E-D); `grep -L 'Review cadence' docs/security/*threat-model*.md docs/security/threat-register.md` prints nothing. Commit: `[PLAN S-WELL-SPRING] Steps 237-242: detection rules with producers, DNS detection marked UNTESTED, threat-model cadence, ADR-062/055 amendments`.

---

## PHASE 12: COMPLIANCE PAPERWORK THAT CAN BE SIGNED (Steps 243-264)

**Goal**: every human-attested control has a draft Stevie can sign in one command; the policy, posture, IR plan v2 with a tabletop, contingency plan, risk strategy and a generated SSP exist; the May-2026 matrices stop pretending; the privacy pack exists and the open decisions are listed with defaults (M-100..M-104, M-108, M-109). [P:11]
**Prerequisite**: Phase 0. Worktree `paperwork`.
**Time**: 5 h
**Agent**: Agent

- [x] **Step 243** [R] ~4m: `grep -n -B4 -A4 'kind: attestation' compliance/catalog/controls.yaml | head -60; grep -n 'policy\|attest' docs/adr/ADR-097*.md | head -12`: the exact refs the collector expects (file names) and the statements ADR-097 promised.
- [x] **Step 244** [W] ~12m: drafts in `docs/compliance/drafts/attestations/` (outside the collector's directory): `security-policy-review.yaml`, `ir-plan-review.yaml`, `access-review.yaml`, each with the ref-named `statement`, `attested_by: Stevie Bellis`, `attested_at: SET-ON-SIGNING`, `expires_at: SET-ON-SIGNING` (+365 d). `scripts/compliance/sign-attestation.sh <name>`: refuses unless `git config user.email` is `stevie@bellis.tech` and `git commit-tree -S` succeeds; copies the draft into `compliance/attestations/<name>.yaml` with today's dates; `git add` that one file; `git commit -S -m "attest: <name> reviewed <date>"`; prints the `%G?` of the new commit. No other file in the commit (the collector checks the last commit touching the file).
- [x] **Step 245** [B] ~2m: `cd /home/govan/tmp/unheaded && bash -n scripts/compliance/sign-attestation.sh && go run ./cmd/compliance-evidence 2>&1 | grep -iE 'attest|UH-(POL|IR|ACC)' | head -5; ls compliance/attestations/`
- [x] **Step 246** [V] ~1m: syntax ok; the attested controls remain NOT_ASSESSED (drafts are not read); `compliance/attestations/` still holds only README. (Signing is Stevie's: E-C at Step 130 is the hand-off; the helper makes the signed commit.)
- [x] **Step 247** [W] ~35m: `compliance/policies/information-security-policy.md` (scope; the single operator's roles; acceptable use; access control = SSH keys + GitHub + Well roles; change management = ledgered migrations, signed commits, CI gates, develop→staging→main; logging/monitoring = ADR-100/092; incident response → IR plan v2; vulnerability management = the gates and KEV poller; backup/restore = Well timers + weekly drill; cryptography = TLS 1.3 hybrid, ML-DSA-65 seals, ADR-105; exceptions = accepted rows in the ADR-098 register with dates; review = annual, attested) and `compliance/policies/single-operator-posture.md` (AT, PS, separation-of-duties and dual-control gaps accepted, with the compensating controls: signed commits, gates that cannot be silenced, attestations that expire, append-only journals). Terse manual style; each section cites the catalog ids it supports.
- [x] **Step 248** [W] ~30m: `runbooks/security/ir-plan-v2-2026-10-27.md` from v1 (§1 Suricata → Heimdall + Vector + `detection.yml`; severities = ADR-029 bands; contacts; evidence handling = journald + Vector rows + Well append-only tables; recovery → contingency plan); `docs/incidents/tabletop-2026-10-27.md`: the Wotan OOM postmortem replayed against v2 (what the plan says at each step, what we would actually do, the gaps found → action items with M-ids); `runbooks/security/incident-response.yaml` points at v2; v1 gets a "superseded by v2" banner.
- [x] **Step 249** [W] ~25m: `compliance/policies/contingency-plan.md`: assets (west, east, Wotan state dir, `/var/lib/unheaded`, the Well + `/srv/backups/unheaded-well` + east offsite, `/var/zhen/models`, LXD images/project `unheaded`, the GPG/ML-DSA keys' custody); RPO/RTO per service from what exists (Well RPO = nightly backup, RTO = the weekly drill's measured restore time: `grep -rn 'drill' monitoring/prometheus/rules/well.yml | head -2` names the metric); recovery order; dependencies; the one host-rebuild exercise defined as a runbook `runbooks/infra/host-rebuild-exercise.yaml` (status draft, to run under a future E-D) and recorded as "scheduled, not yet run".
- [x] **Step 250** [B] ~2m: `cd /home/govan/tmp/unheaded && bash scripts/check-runbooks.sh 2>&1 | tail -2 && for f in compliance/policies/*.md runbooks/security/ir-plan-v2-2026-10-27.md; do grep -oE '\]\(([^)#]+\.(md|yaml))\)' "$f" | tr -d '()]' | sed 's/^.*(//' | while read l; do [ -f "$(dirname $f)/$l" ] || [ -f "$l" ] || echo "BROKEN $f -> $l"; done; done | head`
- [x] **Step 251** [V] ~1m: runbooks 0 new findings; no `BROKEN` lines.
- [x] **Step 252** [C] ~1m: `[PLAN S-WELL-SPRING] Steps 243-251: attestation drafts + signing helper, infosec policy, posture page, IR plan v2 + tabletop, contingency plan`
- [x] **Step 253** [CODE][W] ~10m: `cmd/compliance-evidence/ssp_test.go`: `ssp -catalog <fixture> -evidence <fixture.json> -out <tmp>` renders deterministic Markdown: per control the statement, status (PASS/FAIL/NOT_ASSESSED with the evidence date), evidence refs, framework mappings; a header pointing at the risk strategy and the posture page; golden-file comparison on a 3-control fixture; running twice yields identical bytes.
- [x] **Step 254** [TEST][B] ~1m: red (`go test ./cmd/compliance-evidence/ -run TestSSP -count=1 2>&1 | tail -3`).
- [x] **Step 255** [CODE][W] ~25m: implement the `ssp` subcommand; write `compliance/policies/risk-management-strategy.md` (appetite: three statements with defaults, flagged for E-E; categorization FIPS-199-style per service: Well/Wotan/dashboard moderate, the rest low; criticality list; Statement of Applicability = the catalog's framework coverage table generated by the same command as `## SoA`).
- [x] **Step 256** [TEST][B] ~3m: `cd /home/govan/tmp/unheaded && go test ./cmd/compliance-evidence/ -race -count=1 2>&1 | tail -2 && go run ./cmd/compliance-evidence ssp -out docs/compliance/SSP-generated.md 2>&1 | tail -2 && grep -c '^### UH-' docs/compliance/SSP-generated.md && grep -c 'FAIL' docs/compliance/SSP-generated.md`
- [x] **Step 257** [V] ~1m: green; 50 control sections (49 + UH-SLO-01); the FAIL count equals the findings run's FAIL rows at that moment.
- [x] **Step 258** [W] ~25m: M-108: banner on every `docs/compliance/control-matrix/*-2026-05-06.md`: "Snapshot of 2026-05-06, before the 2026-10-26 source sweep. Rows that cite deleted code are historical. The live crosswalk is `compliance/catalog/controls.yaml`, rendered in `docs/compliance/SSP-generated.md`."; GDPR/CCPA PII rows MAPPED via `DataIsolationConfig` → "not evidenced (code deleted 2026-10-26); see privacy pack"; `docs/compliance/crosswalk-review-2026-10-27.md`: for each framework file under `compliance/catalog/frameworks/`, five mappings spot-checked against the framework's own control text (OLIR for 800-53, TSC 2017 points of focus, ISO 27001:2022 Annex A, CIS v8/Ubuntu benchmark), what was wrong, what was corrected in `controls.yaml` `rationale`.
- [x] **Step 259** [B] ~2m: `cd /home/govan/tmp/unheaded && go test ./pkg/compliance/... -count=1 2>&1 | tail -2 && go run ./cmd/compliance-evidence ssp -out docs/compliance/SSP-generated.md >/dev/null && git diff --stat docs/compliance/SSP-generated.md | tail -1`
- [x] **Step 260** [V] ~1m: loader green; SSP regenerated (diff only where rationales changed).
- [x] **Step 261** [W] ~40m: M-109 privacy pack under `docs/compliance/privacy/`: `pii-inventory-2026-10-27.md` (which columns and logs hold personal data: `grep -rnE '\b(email|user|assignee|author|src|remote_addr)\b' db/migrations/*.sql config/vector/vector.yaml | cut -d: -f1,3 | sort -u` → a table of column → class → retention class from Phase 4; names columns, never values); `privacy-notice.md` (what the platform stores about operators and about traffic: headers and metadata, never payloads); `processing-record.md` (Art. 30-style: purpose, categories, recipients = none external except the Anthropic API under T0-T2 redaction, retention, safeguards); `dpia-ebpf-and-zhen-capture.md` (eBPF reads headers + flow metadata, not payloads; Zhen capture is opt-in and redacted; risks, likelihood, mitigations, residual); `processor-agreement-template.md` (for any future processor; BAA clause optional); `supplier-inventory.md` (GitHub, Anthropic, Canonical, Docker Hub images, PyPI/crates.io/proxy.golang.org, from `scripts/lib/tool-pins.sh`, `THIRD_PARTY.md`, compose images); `decisions-for-stevie.md`: MFA scope (default: GitHub now; dashboard login when ADR-103 lands), external pentest (default: Lich now, external before any internet exposure), host AV/IDS (default: Heimdall + detection rules, no AV), FIPS build variant (default: not now; aws-lc-rs FIPS module later), independent assessment (default: not now), counsel/DFIR retainer (default: none; list two options), risk appetite (the three defaults from Step 255).
- [x] **Step 262** [B] ~2m: `cd /home/govan/tmp/unheaded && bash scripts/check-gitleaks.sh 2>&1 | tail -1 && grep -cE '[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}|@[a-z]+\.[a-z]+' docs/compliance/privacy/pii-inventory-2026-10-27.md`
- [x] **Step 263** [V] ~1m: gitleaks PASS; `0` (the inventory names columns, not values).
- [x] **Step 264** [V]+[C] ~2m: **PHASE 12 EXIT GATE** — Steps 246, 251, 257, 260, 263 passed. Commit: `[PLAN S-WELL-SPRING] Steps 253-264: SSP generator, risk strategy, matrix banners + crosswalk review, privacy pack, decisions list`.

---

## PHASE 13: HOSTS WITH JOURNALS, AGENT WITH A SPINE (Steps 265-282)

**Goal**: every fix on a host was applied by the typed executor and has a journal (UH-REMED-01); `remediate.sh` is retired on parity; the lab-hardened profile grows the fixes ADR-098 steps 2-6 named; Ansible is syntax-checked in CI; `unheaded-baseline` has an enforcing code path behind a flapping breaker, a preinst gate and a 7-day soak, exercised only in audit until E-E; the host window installs this sprint's units (M-105, M-106, M-107; decisions 26, 28). [SEQ]
**Prerequisite**: Phases 6, 9, 11 (units to install); S-CLEAN-SLATE M-041 (deb bundle recipe).
**Time**: 5 h (plus the window)
**Agent**: Coordinator

- [ ] **Step 265** [TEST][B] ~3m: ADR-099 Phase 2 bar: parity
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/unheaded-remediate/... -run 'Parity|Diff' -count=1 -v 2>&1 | grep -E '^(--- |ok|FAIL)' | tail -8; ls scripts/host/
  ```
- [ ] **Step 266** [V] ~1m: parity tests green for all 13 fixes (`model_parity_test.go`, `executor_diff_test.go`). If any fix lacks parity → add it before retiring the script (Step 270 stays blocked on this).
- [ ] **Step 267** [CODE][W] ~30m: new fixes in `compliance/remediation/fixes.yaml` with check/apply/undo in the typed action set: `rocm-log-perms` (mode 0640 on `/var/log/rocm_smi_lib/ROCm-SMI-lib.log` and `/var/log/amd_smi_lib/AMD-SMI-lib.log`; action `file-mode`), `aide-install` (package `aide` present, `aideinit` run once, weekly check unit; needs an `apt-install` action type and a `unit-enable` action type if absent: add them with executor tests, red→green), `apparmor-enforce` (profiles listed in `compliance/hosts/<host>.yaml` set to enforce; action `apparmor-mode`). Each fix gets `checks:` (the baseline check ids it satisfies) so enforcing mode (Step 276) can map deviation → fix. Also a `--record-existing <fix>` executor mode: verify the check already passes and write a journal entry `applied_by: remediate.sh (2026-10-01), recorded 2026-10-xx` without changing the host (this is how UH-REMED-01 gets its history for the eight lab fixes).
- [ ] **Step 268** [TEST][B] ~4m: `cd /home/govan/tmp/unheaded && go test ./cmd/unheaded-remediate/... -race -count=1 2>&1 | tail -3 && (grep -rn 'fixes.yaml' scripts/*.sh cmd/*/main.go Makefile | head -3) && go run ./cmd/compliance-evidence baseline --check 2>&1 | tail -2`
- [ ] **Step 269** [V] ~1m: green; the model loader accepts the three new fixes and the `checks:` field; baseline.yaml regenerated or still current.
- [ ] **Step 270** [W] ~8m: `compliance/profiles/profiles.yaml`: `lab-hardened` = lab + `journald-caps time-p2p auditd-rules rocm-log-perms aide-install apparmor-enforce`; hosts stay bound to `lab` until E-D flips them. `scripts/host/remediate.sh` → a 6-line deprecation shim that prints the equivalent `unheaded-remediate` command and exits 2 (retired; ADR-099 Phase 2); `remediate_test.go` deleted with it; references updated (`grep -rln 'remediate.sh' docs wiki runbooks | head`).
- [ ] **Step 271** [W] ~20m: M-107: `.github/ci-python/requirements.txt` gains `ansible-core` hash-pinned (generate inside the CI venv: `pip-compile --generate-hashes --only-binary=:all:`; pick the newest release ≥ 7 days old; one [COMPLIANCE] line in the dependency baseline if Python tools are covered, else in `docs/policy/python-audit-accepted.txt`'s neighbour file); `static-analysis.yml`: step `ansible-playbook --syntax-check -i deploy/ansible/compliance/inventory.yml deploy/ansible/compliance/profile-*.yml` in the existing Python job; roles `deploy/ansible/compliance/roles/{kingdom-base,huginn,victoria}` written from the live units (`deploy/systemd/huginn.service`, the victoria compose service) with `tasks/main.yml` + `README`; drift test: if `ls pkg/iac` shows the ADR-099 Phase 3 renderer, `go run ./cmd/compliance-evidence render-ansible --check`; if not, a line in ADR-099 Progress "Phase 3 renderer still open; roles hand-written 2026-10-xx, syntax-checked in CI".
- [ ] **Step 272** [B] ~4m: run the CI step locally in the pinned venv
  ```bash
  cd /home/govan/tmp/unheaded && S=/tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad && (test -x $S/civenv/bin/python3 || python3 -m venv $S/civenv) && $S/civenv/bin/pip install --quiet --require-hashes --no-deps --only-binary=:all: -r .github/ci-python/requirements.txt && $S/civenv/bin/ansible-playbook --syntax-check -i deploy/ansible/compliance/inventory.yml deploy/ansible/compliance/profile-*.yml 2>&1 | tail -6; bash scripts/check-dependency-baseline.sh 2>&1 | tail -2; bash scripts/check-tool-freshness.sh 2>&1 | grep -i ansible
  ```
- [ ] **Step 273** [V] ~1m: every playbook `syntax-check` clean; baseline gate PASS (or lists exactly the admitted `ansible-core` set, then `--update` is a shrink-only violation: admit it in the baseline file by hand with the ADR-096 line); freshness `[ok]`.
- [ ] **Step 274** [C] ~1m: `[PLAN S-WELL-SPRING] Steps 265-273: new fixes + record-existing, remediate.sh retired, lab-hardened profile, Ansible syntax-check in CI`
- [ ] **Step 275** [ESCALATE] **E-E** ~0m: Context: `docs/compliance/privacy/decisions-for-stevie.md` and `risk-management-strategy.md` carry a default for each. Questions: (1) Q5: may enforcing mode refuse to START Unheaded services on an out-of-baseline host, or only refuse installs (draft default)? (2) MFA scope; (3) external pentest timing; (4) host AV/IDS stance; (5) counsel/DFIR retainer; (6) the three risk-appetite statements. Why Stevie: availability vs integrity trade-off, spend, and legal posture.
- [ ] **Step 276** [CODE][W] ~45m: enforcing mode, code only (the mode file stays `audit` everywhere): move the executor into `pkg/remediation` (imported by `cmd/unheaded-remediate` and `cmd/unheaded-baseline`); `unheaded-baseline` with `mode=enforcing`: for each deviation whose check id maps to a fix via `checks:`, apply that fix through the executor as the typed action, journal it to `/var/lib/unheaded/remediate`, publish `compliance.deviation.<host>` (M-021) and emit the `would_enforce`/`enforced` journald line; **flapping breaker**: the same fix applied ≥ 3 times in 24 h drops to audit-only for that fix and raises a critical event; **preinst gate**: `debian/*/preinst` refuses the install when `report.json` says `mode_effective=enforcing` and lists critical deviations (installs-only, the Q5 draft default; a Q5 "starts too" answer adds an `ExecStartPre` to the units in a follow-on); tests with a fake executor: deviation→fix mapping, breaker trips on the 3rd apply, unmapped deviation stays audit, enforcing with a missing journal dir fails loud.
- [ ] **Step 277** [TEST][B] ~5m: `cd /home/govan/tmp/unheaded && go build ./... && go test ./pkg/remediation/... ./cmd/unheaded-baseline/... ./cmd/unheaded-remediate/... -race -count=1 2>&1 | tail -4 && go run ./cmd/compliance-evidence baseline --check 2>&1 | tail -1 && bash scripts/check-unit-binaries.sh 2>&1 | tail -1`
- [ ] **Step 278** [V] ~1m: green; baseline current; unit-binaries gate PASS.
- [ ] **Step 279** [W] ~15m: `docs/compliance/enforcing-soak.md` (7-day audit soak: `unheaded_baseline_would_enforce_total` must stay 0 for 7 days on a host before its mode file changes; where the metric comes from (textfile dir); the Grafana panel added to `08_slo_error_budget.json` or a `09_compliance.json`; the flip recipe and the undo); `tomb/lich/LICH-0NN-baseline-agent/CAMPAIGN.md` + `campaign_test.go` (tamper `baseline.yaml` perms, swap the mode file to enforcing, forge `report.json`, replay a stale report; exit criteria: every tamper is a critical event and `mode_effective` never flips without the soak); registered in ADR-062 (row reserved in Step 241; use the next free LICH number from `ls tomb/lich`).
- [ ] **Step 280** [AFTER-E-D][LIVE-RW][S][BARE-METAL][B] ~15m: **East first.** Install the rebuilt debs (reporter, heartbeat timer, vector-east, ci-status, well-retention are west-only or east-only as their units say), then journals and the new fixes
  ```bash
  ssh govan@east 'sudo dpkg -i /tmp/unheaded-debs/*.deb && sudo systemctl daemon-reload && for f in sysctl-cis apport-off kmod-cis cron-perms banner-cis apparmor-utils auditd sudo-cis; do sudo unheaded-remediate -host east --record-existing $f; done && sudo unheaded-remediate -host east apply journald-caps time-p2p auditd-rules rocm-log-perms && sudo systemctl enable --now unheaded-heartbeat.timer vector-east.service && ls /var/lib/unheaded/remediate/ | head' 2>&1 | tail -15
  ```
  (`rocm-log-perms` is a no-op on east if the ROCm logs are west-only; the check says so.) Then `compliance/hosts/east.yaml` → `lab-hardened`.
- [ ] **Step 281** [AFTER-E-D][LIVE-RW][S][BARE-METAL][B] ~15m: **West**, same sequence plus the west-only units (`unheaded-ci-status.timer`, `unheaded-well-retention.timer` stays per E-C, Step 131), `sudo docker compose up -d --build` so every service carries the reporter (Phase 11) and the journald driver (Phase 9); `compliance/hosts/west.yaml` → `lab-hardened`. Then the Step 235, 209 and 219 checks that were waiting on this window.
- [ ] **Step 282** [AFTER-E-D][LIVE-RO][V]+[C] ~5m: **PHASE 13 EXIT GATE** — `cd /home/govan/tmp/unheaded && go run ./cmd/compliance-evidence 2>&1 | grep -E 'UH-REMED-01|UH-BASE-01|UH-HOST-02|UH-MAC-01'`: UH-REMED-01 PASS (journals exist and match the profile), UH-BASE-01 PASS (both agents report, agree with the central pull), UH-HOST-02/UH-MAC-01 moved or still FAIL with the accepted-until date; `journalctl -u systemd-journald -n 1` on each host shows the new `SystemMaxUse`; `stat -c %a /var/log/rocm_smi_lib/ROCm-SMI-lib.log` = 640 on west. Steps 266, 269, 273, 278 passed. Commit: `[PLAN S-WELL-SPRING] Steps 275-282: enforcing code path behind breaker + preinst gate, soak doc, LICH baseline-agent campaign; hosts journaled`.

---

## PHASE 14: SECURITY AND COMPLIANCE REVIEW (Steps 283-288)

**Goal**: the attack surface this sprint added is written down and every gate is green; the new campaign and fuzz targets ran.
**Prerequisite**: Phases 1-13 (host steps may be [AFTER-E-*] pending).
**Time**: 1.5 h
**Agent**: Coordinator

- [ ] **Step 283** [SECURITY][W] ~15m: **Attack-surface table** (append to this file under Appendix E): new listeners (heimdall-daemon `127.0.0.1:19101`; alert-mailer `POST /heartbeat/<host>` token-gated, reachable from east over P2P; vector-east → ClickHouse over P2P with a password file), new Well roles and grants (`retention_runner` DELETE on four tables; `ops_writer` INSERT on `deploy_history`, `chaos_journal`, `slo_definitions`), the journald logging driver (container stdout now in the host journal, readable by `systemd-journal` group members: list who is in it on west/east), the `bench.perf` topic, the SSH metric labels (`user`, `src`: operator login + LAN address, no payload), the preinst gate (a tampered `report.json` could block installs: that is the fail-closed intent; the Lich campaign covers forging it), `--record-existing` (writes history without changing the host: journal entries are labelled as recorded). For each: threat, mitigation, residual, register row if any.
- [ ] **Step 284** [B] ~12m: every gate
  ```bash
  cd /home/govan/tmp/unheaded && for g in check-gitleaks check-gosec-ratchet check-dependency-baseline check-systemd-units check-runbooks check-well-lint check-well-migrations check-prometheus-rules check-compose-hardening check-compose-log-caps check-unit-binaries check-tool-freshness check-python-syntax; do printf "%-28s " $g; bash scripts/$g.sh >/tmp/gate-$g.log 2>&1 && echo PASS || { echo FAIL; tail -3 /tmp/gate-$g.log; }; done; (for f in $(git ls-files '*.go' | grep -v -e _test.go -e '/vendor/'); do head -3 "$f" | grep -q SPDX-License-Identifier || echo "$f"; done | head -5; echo "spdx: $(git ls-files '*.go' | xargs -I{} sh -c 'head -3 {} | grep -q SPDX || echo x' | wc -l) Go files missing"); go vet ./... 2>&1 | tail -2
  ```
- [ ] **Step 285** [V] ~2m: all PASS; `go vet` clean. A FAIL here is fixed in its phase, not baselined.
- [ ] **Step 286** [TEST][B] ~6m: campaign + fuzz
  ```bash
  cd /home/govan/tmp/unheaded && go test ./tomb/lich/LICH-*baseline-agent/... -count=1 -v 2>&1 | grep -E '^(--- |ok|FAIL)' | tail -8; go test ./cmd/alert-mailer/ -run '^$' -fuzz=FuzzHeartbeatRequest -fuzztime=60s 2>&1 | tail -2; go test ./cmd/well-retention/ -run '^$' -fuzz=FuzzClassesYAML -fuzztime=60s 2>&1 | tail -2
  ```
  (Write `FuzzHeartbeatRequest` and `FuzzClassesYAML` in Steps 149/116 if not already; both are 10-line fuzz targets on the parsers.)
- [ ] **Step 287** [V] ~1m: **PHASE 14 EXIT GATE** — every tamper case in the campaign is a critical event and `mode_effective` stays audit; both fuzzers: 0 crashes.
- [ ] **Step 288** [C] ~1m: `[PLAN S-WELL-SPRING] Steps 283-288: attack-surface review, all gates green, baseline-agent campaign + fuzz clean`

---

## PHASE 15: DOCS, REGISTERS, HANDOFF (Steps 289-294)

**Goal**: every downstream doc tells the truth about what this sprint changed; registers regenerated; the master list rows closed; next.md carries the handoff.
**Prerequisite**: Phase 14.
**Time**: 1.5 h
**Agent**: Coordinator (Librarian lens)

- [ ] **Step 289** [DOC-UPDATE][W] ~30m: wiki: `Alerting.md` (Watchdog, heartbeat, `runbook_url` gate, `ci-develop` SLO, dead alerts removed and why, error-budget dashboard), `The-Well.md` + `Well-Backups.md` (PG 18 if E-B, migrations 035-038, retention timer, `deploy_history`/`chaos_journal`/`slo_definitions` tables and who writes them), `Logging.md` (journald driver, Vector labels, east unit), `Port-Registry.md` (19101), the compliance page (policies, SSP, attestation signing recipe, enforcing soak), `Service-Wotan.md` (`system.health.remediate`). `CLAUDE.md`: "Cross-Service Health Monitoring" doctrine → reporters in every service, remediate topic; §Observability line for the one log path. ADR statuses: ADR-100 (Phases 1, 2, 5 built; 3 partial; 4 = `ci-develop` + Well/Wotan/dashboard SLOs present), ADR-099 (Phase 2 done, Phase 3 roles hand-written, Phase 4 code path exists behind the soak), ADR-098 (Phase 1 remainder: heartbeat built, receiver still email dry-run until FND-001), ADR-029 (reporters, remediate, ballot recorded), ADR-055/062/076/086/092 amendments already written in their phases; `docs/adr/ADR-INDEX.md` rows.
- [ ] **Step 290** [REGEN][B] ~5m
  ```bash
  cd /home/govan/tmp/unheaded && go run ./cmd/compliance-evidence findings -adr docs/adr/ADR-098-findings-register-and-baseline-modes.md 2>&1 | tail -2 && REGEN=$(grep -m1 -oE '`[^`]*unreached[^`]*`' docs/UNREACHED-GO-PACKAGES.md | tr -d '`') && echo "regen: $REGEN" && eval "$REGEN" 2>&1 | tail -2; go run ./cmd/crypto-inventory 2>&1 | tail -1; git status --short | head -8
  ```
- [ ] **Step 291** [V] ~1m: register regenerated with 0 untriaged rows (FND-015 fixed if E-B ran; the gitleaks job row from S-CLEAN-SLATE untouched); UNREACHED list shrank by `pkg/metrics` pqc entries; CBOM unchanged unless PG 18 landed.
- [ ] **Step 292** [DOC-UPDATE][W] ~8m: `references/timeline.md`: one dated entry for S-WELL-SPRING (what is live, what waits on which escalation) and mirrors regenerated the way `f83c9cde` did (`grep -n 'sync\|mirror' services/timeguru/cmd/timeguru/main.go | head -3` for the subcommand, or edit json/yaml/toml by the same parser); `bash scripts/check-timeline-freshness.sh | tail -2` FRESH.
- [ ] **Step 293** [AUDIT-UPDATE][W] ~10m: `docs/sweeps/OUTSTANDING-2026-10-27.md`: every M-id in Appendix D gets a status suffix (`DONE <commit>`, `LIVE-PENDING E-x`, `DEFERRED → S-<other>`); `~/tmp/next.md`: a "S-WELL-SPRING" section at the top (what shipped, the five escalations and which are answered, live state of west/east after the window, the Step 43 index-drop follow-on after 2026-11-15, the soak start date if E-E answered).
- [ ] **Step 294** [V]+[C] ~5m: **FINAL EXIT GATE** — every phase exit gate passed or its STUCK marker is in this file; `go test ./... 2>&1 | grep -E '^(FAIL|---)' | diff - docs/battle-plans/progress/S-WELL-SPRING-baseline.txt` empty (zero new failures); `git status --short` empty after the commit; worktrees removed (`git worktree list | grep -c wellspring` = 0). Commit: `[PLAN S-WELL-SPRING] Steps 289-294: docs, registers, timeline, handoff`.

---

## APPENDIX A: EMERGENCY PROCEDURES

**A1. `batch-apply.sh` fails mid-batch on the live Well**
1. Do not re-run. `psql -d <db> -c "SELECT filename, applied_at FROM well_schema_migrations ORDER BY applied_at DESC LIMIT 5"` to see where it stopped.
2. `bash scripts/well/batch-rollback.sh /srv/backups/unheaded-well/pre-batch-<ts>/` (restores both databases from the dump taken seconds before).
3. Verify the ledger tail is the pre-batch tail; restart kanban-app and timeguru; `curl :20001/health`.
4. Fix the failing file in the repo (it was never applied anywhere, so editing is allowed), re-run `scripts/well/test-migrate.sh`, re-ask E-A.

**A2. kanban-app or timeguru crash-loops after Phase 1 with `ErrMigrationMissing`**
1. `docker logs --tail 20 unheaded-kanban-app | grep migration`: the file name it wants.
2. `psql -d unheaded_app -c "SELECT filename FROM well_schema_migrations WHERE filename LIKE '03%'"`: if the file is there under a different name, fix the constant in `store.go`; if absent, the batch did not run: A1 or wait for E-A and run the old image meanwhile (`git stash` is not the tool; `docker compose up -d kanban-app` with the previous tag from `docker images | grep kanban`).

**A3. PG 18 upgrade: server will not start**
1. `docker logs unheaded-postgres | tail -30`.
2. Runbook `rollback:` section: stop, restore the 16 data volume from Step 139's dump into a fresh 16 container, start, verify `SHOW server_version` = 16.
3. Record in the progress file; STUCK Phase 5; the rest of the plan does not depend on 18.

**A4. Alertmanager pages HeartbeatMissing right after Phase 13**
1. East's timer: `ssh govan@east systemctl status unheaded-heartbeat.timer unheaded-heartbeat.service --no-pager | tail -12`.
2. Token mismatch → `curl -s -o /dev/null -w '%{http_code}' -X POST -H "X-Heartbeat-Token: $(sudo cat /etc/unheaded/heartbeat.token)" http://192.168.13.2:19130/heartbeat/east` from east; 401 means the two files differ; 404 means `HEARTBEAT_HOSTS` lacks `east` in west's compose.
3. P2P link: `ping -c3 192.168.13.2` from east. If down, this is exactly what the alert is for; silence with `scripts/monitoring/maintenance.sh` while fixing.

**A5. Vector stops shipping after the journald driver switch**
1. `sudo journalctl -o json -n 1 CONTAINER_NAME=unheaded-wotan | jq -r .CONTAINER_NAME` (empty → the driver did not apply: `docker inspect unheaded-wotan | jq '.[0].HostConfig.LogConfig'`).
2. `docker logs --tail 30 unheaded-vector` for a remap error; `bash scripts/test-vector-config.sh`.
3. Revert: remove the `x-logging-journald` merge from the one service, `docker compose up -d <svc>`; the old json-file caps return.

**A6. `check-prometheus-rules.sh` fails on `runbook_url` after someone adds a rule**
1. The gate prints the alert name. Add `annotations.runbook_url` pointing at an existing runbook or write one (runbook/v1, `status: draft`).
2. Never add a baseline for it; the gate is strict by design.

**A7. Enforcing mode applied something it should not have (after E-E)**
1. `sudo unheaded-remediate -host <h> undo <fix>` (every apply has an `undo.sh` under `/var/lib/unheaded/remediate/`).
2. Set `/etc/unheaded/baseline.conf` back to `mode=audit`; `systemctl restart unheaded-baseline.service`.
3. Check the breaker did its job (`journalctl -u unheaded-baseline | grep breaker`); if it did not trip after 3 applies, that is a P1 bug: STOP and fix before any host runs enforcing again.

**A8. The dead-alert deletion removed a rule Stevie wanted**
1. `git log --oneline -- monitoring/prometheus/rules/monad.yml | head -1` then `git show <sha>:monitoring/prometheus/rules/monad.yml > /tmp/monad.yml`.
2. Re-add only that rule into the matching live file WITH a producer (otherwise it is the same dead rule) and a unit test.

---

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Phase | Agent | Worktree | Parallel with | Depends on | Time |
|---|---|---|---|---|---|
| 0 Preflight | Coordinator | develop | — | S-CLEAN-SLATE | 45 m |
| 1 Ledger discipline | Coordinator (19-43 Agent) | well-store | — | 0 | 3 h (+E-A window) |
| 2 Deploy/chaos → Well | Agent | well-store | 3 | 1 (19-43) | 5 h |
| 3 SLOs as data | Agent | slo-data | 2 | 1 (19-43) | 3 h |
| 4 Retention | Agent (127-131 Coordinator) | well-store | 6, 7, 8 | 1 (19-43) | 3 h |
| 5 PG 18 | Coordinator | develop | — | 1; E-B | 1.5 h (+window) |
| 6 Watchdog + postmortems | Agent | sre-rules | 7, 8, 10, 12 | 0 | 2.5 h |
| 7 Alert rules | Agent | sre-rules | 6, 8, 10, 12 | 0 | 3.5 h |
| 8 Metrics | Agent | metrics-clean | 6, 7, 10, 12 | 0 | 2.5 h |
| 9 Log path + topology | Coordinator | develop | 10, 12 | 0; E-D for 208-209 | 2.5 h |
| 10 Measure | Agent (217-220 Coordinator) | measure | 6, 7, 8, 12 | 0; E-D for 219 | 4 h |
| 11 Akira + detection | Agent | akira | 12 | 0; E-D for 235-236 | 3.5 h |
| 12 Paperwork | Agent | paperwork | 6-11 | 0 | 5 h |
| 13 Hosts + enforcing | Coordinator | develop | — | 6, 9, 11; E-D, E-E | 5 h (+window) |
| 14 Security review | Coordinator | develop | — | 1-13 | 1.5 h |
| 15 Docs + handoff | Coordinator | develop | — | 14 | 1.5 h |

**Critical path**: 0 (incl. 18a) → 1 (19-43) → 2 → 4 → [E-A window: 45-49, 78-80, 105-106, 127-129] → 13 [E-D window: 280-282] → 14 → 15. Everything else hangs off Phase 0 and merges before 13.

**Dependency map (escalations):** E-A blocks Steps 45-49, 78-80, 105-106, 127-129 (live Well writes). E-B blocks 139-144. E-C blocks 131 and the attestation signing (Stevie runs `scripts/compliance/sign-attestation.sh` three times; nothing else waits on it). E-D blocks 208-209, 219-220, 235-236, 280-282. E-E blocks only the enforcing-mode flip (not in this plan; the code path lands regardless) and the six paperwork answers recorded in `decisions-for-stevie.md`. No step is blocked on two escalations.

**Merge order** (each worktree squash-merges to develop at its exit gate): well-store (after Phase 4) → slo-data → sre-rules (after 7) → metrics-clean → measure → akira → paperwork. Rebase on develop before each merge; the Well suites (`scripts/well/test-*.sh`) run after every merge that touched `db/migrations`.

---

## APPENDIX C: QUICK REFERENCE

**Well**: container `unheaded-postgres`; databases `unheaded_app` (kanban, zhen, timeline), `unheaded_ops` (compliance, llm_requests, deploy/chaos/slo/retention tables from this sprint), `postgres` (cluster files). Roles: owners (NOLOGIN, 025), `ops_writer` (`database.OpsWriterConfig`), `app_kanban`, `app_timeguru`, `retention_runner` (038). Passwords in `.env` (never committed); `WELL_MIGRATE_PASSWORD` for `well-migrate`. Suites: `scripts/well/test-*.sh`; gates `scripts/check-well-lint.sh`, `scripts/check-well-migrations.sh`. New this sprint: `035_app_timeguru_tables.sql` (if needed), `036_ops_deploy_chaos.sql`, `037_ops_slo_definitions.sql`, `038_ops_retention.sql`; `scripts/well/batch-apply.sh [--dry-run]`, `batch-rollback.sh <dir>`, `index-drops-after-2026-11-15.sh`.

**Ports** (`pkg/ports/ports.go`): Wotan 18000/18001, Sword 19011, Chaos 19012, Akira 19100, HeimdallMetrics 19101 (new), AlertMailer 19130, TraceCollectorMetrics 16671, kanban 20001, timeguru 19000, dashboard 20000, VictoriaMetrics 8428 (monitoring compose), Prometheus 9090.

**Monitoring**: stack `monitoring/docker-compose.yml` (prometheus, grafana, loki, victoriametrics, alertmanager, alert-mailer); rules `monitoring/prometheus/rules/*.yml` + `tests/*_test.yml`; policy `observability/alerting/policy.yaml` → `go run ./cmd/alert-policy` (regen) / `-check` / `-sync-well` (new); gate `scripts/check-prometheus-rules.sh` (now also `runbook_url`); silences `scripts/monitoring/maintenance.sh`; proof `scripts/monitoring/test-alert-path.sh`. New files: `watchdog.yml`, `heartbeat.yml`, `collector.yml`, `detection.yml`; deleted: `monad.yml`, `loadbalancers.yml`.

**Compliance**: `go run ./cmd/compliance-evidence` (collect + print), `findings -adr <ADR-098>` (register regen), `baseline --check`, `ssp -out docs/compliance/SSP-generated.md` (new). Catalog `compliance/catalog/controls.yaml` (49 → 50 controls); attestations `compliance/attestations/*.yaml` (signed by Stevie only; `scripts/compliance/sign-attestation.sh <name>`); fixes `compliance/remediation/fixes.yaml` (13 → 16); profiles `compliance/profiles/profiles.yaml`; hosts `compliance/hosts/{west,east}.yaml`; agent `cmd/unheaded-baseline` (`/etc/unheaded/baseline.conf` `mode=audit|enforcing`), executor `cmd/unheaded-remediate` (`-state /var/lib/unheaded/remediate`, `--record-existing`, `undo`).

**Runbooks**: `runbooks/<area>/*.yaml` (runbook/v1: `kind, metadata, notes, parameters, preconditions, steps, rollback`; `metadata.status` draft→approved by Stevie); gate `scripts/check-runbooks.sh`; runner `$(git ls-files | grep -E '(^|/)run-runbook\.py$')` `--dry-run`. New: `observe/heartbeat-missing.yaml`, `observe/trace-collector-down.yaml`, `observe/high-latency.yaml`, `observe/ci-red.yaml`, `security/ssh-login-new-source.yaml`, `security/outbound-volume-deviation.yaml`, `infra/host-rebuild-exercise.yaml`, `security/ir-plan-v2-2026-10-27.md`.

**Systemd** (`deploy/systemd/`): new `unheaded-well-retention.{service,timer}`, `unheaded-heartbeat.{service,timer}` (east), `unheaded-ci-status.{service,timer}` (west), `vector-east.service`; every unit its own nologin user with a `pkg/uids` entry; gate `scripts/check-systemd-units.sh`.

**Hosts**: west = dev box (this shell), east = `ssh govan@east` (P2P 192.168.13.1 east ↔ 192.168.13.2 west). Everything host-changing is [LIVE-RW] under E-D. Backups `/srv/backups/unheaded-well/`.

**Commit**: `git commit $GPGFLAG -m "[PLAN S-WELL-SPRING] Steps a-b: ..."` with the two trailer lines; `GPGFLAG=--no-gpg-sign` when Step 3 said `SIGN=no`. Never push.

---

## APPENDIX D: COVERAGE (master list → steps)

| M-id | Steps | Note |
|---|---|---|
| M-070 | 32-39, 44-50 | live apply under E-A |
| M-071 | 133-144 | live under E-B |
| M-072 | 51-86 | file mode kept as fallback |
| M-073 | 19-31 | |
| M-074 | 111-132 | ratify under E-C |
| M-075 | 40 | ADR-011 amendment; purge via retention |
| M-076 | 41-43 | date-gated script; follow-on after 2026-11-15 |
| M-077 | 87-110 | |
| M-080 | 145-155 | east timer installed in 280 |
| M-081 | 156-160 | |
| M-082 | 197-202, 205-209 | Vector restart under E-D |
| M-083 | 161-166 | |
| M-084 | 167-171 | |
| M-085 | 181-187 | |
| M-086 | 188-196 | |
| M-087 | 172-180 | |
| M-088 | 203-204 | re-scoped, not built (decision 27) |
| M-089 | 211-220, 225 | container-start under E-D |
| M-090 | 221-224 | |
| M-100 | 243-246, E-C | Stevie signs |
| M-101 | 247 | |
| M-102 | 248 | counsel/DFIR → E-E |
| M-103 | 249 | rebuild exercise scheduled, not run |
| M-104 | 253-257 | appetite → E-E |
| M-105 | 265-274, 280-282 | host apply under E-D |
| M-106 | 276-279 | flip not in this plan (soak + E-E) |
| M-107 | 271-273 | Phase 3 renderer still open if `pkg/iac` lacks it |
| M-108 | 258-260 | |
| M-109 | 261-263 | decisions → E-E |
| M-200 | 227-236 | ballot recorded under E-D |
| M-203 | 237-240 | DNS detections UNTESTED, source named |
| M-205 | 241 | |
| M-021 (partial) | 276 | `compliance.deviation.<host>` publish lands with enforcing path; the `.deb` part stays in S-CLEAN-SLATE |

Deferred out of this plan: `pkg/statefile` users architect/micromanager/unheaded-daemon (not deploy/chaos state; a Well home for them is an ADR-099 follow-on); ADR-099 Phase 5 dashboard admin (needs S-IRON-GATE auth); the enforcing-mode flip itself (soak + E-E); west-dies watchdog receiver outside the LAN (decision 22, "outside later").

---

## APPENDIX E: ATTACK-SURFACE TABLE

Filled in at Step 283.

---

*S-WELL-SPRING Battle Plan — Forged 2026-10-27*
*16 Phases. 294 Steps. The Well holds what the services used to keep in files; the alerts name only what exists; the paperwork can be signed.*
