<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# ADR-147 trial integration (plans 1 to 4)

Branch `integrate/adr147`, worktree `/home/govan/tmp/wt/ADR147-INTEGRATE`, cut from develop `374f0dda` on 2026-11-15. A trial: nothing here is merged into develop, and the four sprint branches are untouched. After the evidence week (ends 2026-11-17 18:34Z) the real merge is the same four `git merge --no-ff` in this order, then the `integration:` commits below (cherry-pick them, or merge this branch).

| # | Branch | Head merged | Merge commit | Conflicts |
|---|---|---|---|---|
| 1 | `sprint/s-see-and-survive` | `674ca4fe` | `2ebf9fc0` | 0 |
| 2 | `sprint/s-waste-out` | `243ea83d` | `f1428230` | 8 (5 modify/delete, 3 content) |
| 3 | `sprint/s-zhen-switch` | `0c2f3a4f` | `ec7e9e89` | 14 |
| 4 | `sprint/s-west-ground` | `22c1d2e2` | `31675b9b` | 3 |

`sprint/s-west-ground-compose` (`dd590f29`) is not merged: held for its plan's Phase 7.

Merge commits 2, 3 and 4 were committed with `--no-verify`: the pre-commit hook's `go vet` failed on a semantic break that the next `integration:` commit fixes (kept separate so each fix is visible). Every commit on the branch is signed.

## Conflicts and how each was resolved

### Merge 2 (plan 2 into plan 1)

| File | Resolution |
|---|---|
| `deploy/systemd/unheaded-{architect,captain,micromanager,monad,sophia}.service` | modify (plan 1's start limits) / delete (plan 2): **delete** |
| `scripts/check-gates-can-fail.sh` | both: `provoke_unit_start_limits` (plan 1), `provoke_unit_ceilings_compose` and `_compose_yaml` (plan 2); registry rows from both |
| `scripts/qa-smoke.sh` | plan 2's service lists (five role services and cuirass out) with plan 1's `"${DOCKER[@]}"` and `https://` |
| `runbooks/MANIFEST.sha256` | regenerated (`scripts/runbook-manifest.sh --write`, 100) |

### Merge 3 (plan 3)

| File | Resolution |
|---|---|
| `.github/workflows/security.yml` | both steps (start limits, module binds) |
| `Makefile` | `build-services` without the role services (plan 2) and without `build-zhen-rag` (plan 3, now in `build-zhen`) |
| `cmd/dashboard-backend/internal/server/server.go` | plan 2 deleted the host gauges (metrics from VictoriaMetrics); kept only plan 3's `unheaded_dashboard_health_targets` |
| `config/logging/sources.yaml` | plan 3's two new rows (ioc-audit, module-guard); not its copies of the monad/sophia rows plan 2 deleted |
| `db/migrations/MANIFEST` | 050 (plan 2) before 051-053 (plan 3) |
| `deploy/systemd/{unheaded-llama,unheaded-zhen-app,unheaded-zhen-ui-front,zhen-agentd}.service` | plan 1's start limits and plan 3's `PartOf=unheaded-zhen.target`. zhen-agentd: plan 1's SS3 limit (90 starts, 30 min) replaces plan 3's unchanged `StartLimitIntervalSec=0` (plan 1 flagged it for Stevie) |
| `docker-compose.yml` (kanban-app volumes) | plan 2's removal of `kanban-data`, plan 3's `/run/unheaded/modules` mount |
| `monitoring/prometheus/prometheus.yml` | zhen-agentd job deleted (plan 3 moved it to `modules/zhen/prometheus/scrape/zhen.yml`), plan 2's keep anchor with it |
| `runbooks/observe/service-health-sweep.yaml` | plan 3's targets file (it already leaves out 19001-19006) |
| `scripts/check-gates-can-fail.sh` | both: `provoke_prom_keep_list` (plan 2), `provoke_module_rules`, `provoke_module_binds` (plan 3); all registry rows |
| `runbooks/MANIFEST.sha256` | regenerated (101) |

### Merge 4 (plan 4)

| File | Resolution |
|---|---|
| `observability/alerting/policy.yaml` | both promotion lists (plan 1's 16, plan 4's HostSettingsDrift, HostSettingsCheckStale) |
| `monitoring/alertmanager/alertmanager.yml` | regenerated (`go run ./cmd/alert-policy`); merge 3's text merge had left the routes out of the generator's order |
| `scripts/check-gates-can-fail.sh` | both: `provoke_host_settings` added after the others |
| `runbooks/MANIFEST.sha256` | regenerated (102) |

Auto-merged without conflict and checked: `docker-compose.yml` (rest), `monitoring/docker-compose.yml`, `configs/wotan.yaml`, `config/victoria/scrape.yml`, rule files, `cmd/akira`, `config/akira/akira.yaml`, `config/dashboard/services.txt`, `pkg/uids` and `deploy/sysusers.d/unheaded.conf` (no duplicate uid), `docs/adr/ADR-INDEX.md` (ADR-149 once), `CLAUDE.md`, wiki pages.

## Regenerated files

| File | How | When |
|---|---|---|
| `runbooks/MANIFEST.sha256` | `scripts/runbook-manifest.sh --write` | in merges 2, 3, 4 (102 runbooks; needs Stevie's seal) |
| `monitoring/alertmanager/alertmanager.yml` | `go run ./cmd/alert-policy` | merge 4 |
| `compliance/crypto/cbom.cdx.json` | `go run ./cmd/crypto-inventory scan --out compliance/crypto/cbom.cdx.json` | `e819bef7` |
| `monitoring/prometheus/rule-series.txt`, keep blocks in `prometheus.yml` | `scripts/gen-prom-keep-list.py` | `8b0d5079`, `5ffa0b66` (82 series) |
| `man/{akira,anomaly,huginn,zhen-agentd,zhen-claude}.1` | `scripts/gen-man.sh --write` | `e326510c` |
| `docs/policy/image-tag-baseline.txt` | `scripts/check-image-tags.sh --update` (shrink only, 4 rows) | `b17d47af` |

## Semantic fixes (`integration:` commits)

| Commit | Found by | Cause | Fix |
|---|---|---|---|
| `64a6e352` | vet | plan 2 gave alert-mailer's `handler()` a `*critPublisher`; plan 1's pipeline test called it with two arguments | pass `nil` (module-off path) |
| `5c5dee1e` | retired-name grep | plan 1's ContainerStopped promtool fixture used `unheaded-captain` | timeguru, as plan 2 did in `wotan_test.yml` |
| `bbefc635` | build | plan 2 dropped `pkg/database` from kanban-app; plan 3's run pool needs it | import restored |
| `a2d564a3` | vet | plan 4's kanban test set `Config.DataDir`, removed by plan 2 | field dropped |
| `b17d47af` | `check-image-tags.sh` | plan 2 deleted `docker/services/{monad,sophia}` and left four baseline rows (fails on plan 2's branch too) | baseline lowered |
| `8b0d5079` | `gen-prom-keep-list.py --check` | plan 2's keep-list predates plan 1's and plan 4's rules: 41 series they read would be dropped at scrape. Plan 1's new `victoria` job had no keep, so Prometheus would keep all of VictoriaMetrics' series 30 days | regenerated; `victoria` gets the anchor (its rule reads only `up`) |
| `b13c3d2b` | count | plans 3 and 4 each added a runbook without changing CLAUDE.md's count | 102 (infra 20, security 23) |
| `5cb01e35` | `go test` | plan 1's `TestValidateRunDays` copied only platform rules; plan 3 moved Zhen's rules (ZhenBackupStale) to `modules/zhen` | uses plan 3's `copyHandRules` |
| `1d93594e` | `go test` (`TestRepoSLOSourcesMatchPolicy`) | plan 1 added the store-backups SLO without a UH-SLO-01 source (fails on plan 1's branch too: its lane tested touched packages only) | source row added |
| `42230881` | golangci-lint | ST1022 on `KindHostService` (from develop; reported because plan 2 touched the package) | comment form |
| `5ffa0b66` | `test_platform_files_name_no_zhen` | after plan 3, plan 2's generator still pulled a Zhen series into `prometheus.yml` through the zhen-scheduler-stalled runbook | generator leaves series only module rules read off the list (`--modules`); the module's own job has no keep, so it is still kept |
| `e6b73f22` | pytest | plan 2 removed nginx-monad and nginx-sophia; `test_image_digests.py` expected 8 load balancer images (fails on plan 2's branch too) | 6 |
| `e326510c` | `gen-man.sh` | five binaries gained flags on the branches without a man rebuild | regenerated |

Checked and clean: `TestRetiredIdentitiesStayOut` (no retired identity back), plan 1's start-limit and ceiling gates over plan 3's and plan 4's units (23 restarting units, 21 Go units), `check-module-binds`, plan 4's `GatesRequest` against plan 2's dashboard (tests green), uid registry tests, migrations 050-053 order (`check-well-migrations`).

## Gates on the merged tree

| Gate | Result |
|---|---|
| `go build ./...`, `go vet ./...` | ok |
| `go test -race ./pkg/... ./cmd/... ./services/...` (nice, ionice, GOMAXPROCS 4) | 208 ok, 2 FAIL at `a2d564a3` (`TestRepoSLOSourcesMatchPolicy`, `TestValidateRunDays`); both fixed, each package re-run green |
| golangci-lint on 68 changed package directories | 0 issues after `42230881` |
| shellcheck `-S warning` on 52 changed scripts | clean (info/style notes only) |
| `check-ruff.sh` (civenv) | PASS |
| `tests/scripts` + `tests/runbooks` (civenv pytest) | 680 passed, 4 skipped (2 failed before `5ffa0b66`, `e6b73f22`) |
| raft tests (raftvenv) | 619 passed, 2 skipped |
| `check-well-lint.sh`, `check-well-migrations.sh` | PASS (93 pairs; 58 entries over 5 databases) |
| gosec ratchet | PASS |
| `check-gates-can-fail.sh` coverage (dirty-tree run) | PASS (no unclassified gate); `--only` PASS for check-unit-start-limits, check-unit-ceilings (3), check-module-binds, check-host-settings, check-runbooks |
| `alert-policy -check`, `-runbooks` | PASS |
| `gen-prom-keep-list.py --check`, `test-prom-keep-list.sh` | OK (82 series) |
| check-unit-start-limits, -ceilings, -binaries, -flags, check-systemd-units (69), check-log-sources, check-module-binds, check-host-settings, check-runbooks, check-scrape-publish, check-http-timeouts, check-image-tags, check-image-digests, check-actions-pinned, check-compose-hardening, -publish-bind, -log-caps, -bind-nesting, check-netip, check-spdx, check-crypto-inventory, check-version-flag (57), gen-man, check-json-responses, check-secrets-baseline, check-go-env, check-mod-integrity, check-sbom-trend, check-sbom-version, check-dependency-baseline, check-zhen-mcp, compliance/check-service-auth | PASS |
| test-planned-stop (18), test-host-state-textfile (12), test-check-host-settings (36) | PASS |
| `make -n build-services` | ok |

**2026-11-16:** merged `sprint/s-zhen-switch` again (its Steps 221 script half, 223 and 234 written ahead; the four Zhen provocations moved from 031/032/033/045 to 052, which would otherwise not have bitten after 053) and develop (ADR-147's dated note). `go vet -tags 'wellint integration'` on the six touched packages, shellcheck `-S warning` on `scripts/well/test-*.sh` and the meta-gate, `tests/scripts` 191 passed, `go test` on zhen-agentd, claudebridge, zhen-claude, champion: all pass.

**2026-11-16, meta-gate on the merged tree (fast rows; no containers).** The trial's gate list had missed four gates that failed on the clean merged tree; each is fixed where it belongs and merged here:

| Gate | Cause | Fix |
|---|---|---|
| `check-ci-verdicts` | `docker/Makefile` still listed monad and sophia (plan 2 removed their Dockerfiles) | plan 2 `730ec65c` |
| `test-check-ci-verdicts` | three cases anchored on monad's Dockerfile and `SERVICES := monad ...` | plan 2 `086b0601` (anchored on shield) |
| `check-nosec-ratchet` | no lane moved the baseline: 23 gone with the code, 13 new | `7a62fd9c` (3 fixed properly, 10 added by hand with reasons), then `1f69fbce` for develop's benchmark fix |
| `live-path-inventory` | develop lacked `cmd/unheaded-aide`; here also the five role services | develop `90eaac43`, here `1f69fbce` |

Develop itself was red in CI on two of these since earlier pushes, fixed there: `9a40bb93` (S-WATCH-TRUE's benchmark `#nosec` G115, now built from bytes) and `77a2f906` (`apt-repo.sh serve` and its test still assumed `/srv/unheaded-apt` after the vhost moved to west's `/var/lib/apt-repo` on 11-03). Also `well/test-zhen-database` gained a meta-gate row (plan 3).

After every 2026-11-16 change (head `8ad24b3c`): `go test -race ./cmd/... ./pkg/... ./services/...` 210 packages ok, 0 failed. Proved here: every fast row but two bites (one at a time with `--only`, behind a shim that refuses `docker run/build/create/exec` and `sudo`; log in `~/tmp/adr147-table/runs/`); `check-python-audit` stays INCONCLUSIVE (soupsieve, Stevie's yes). Not run: `check-prometheus-rules` (its fast row too runs promtool in a container; a `--quick` run reached it at 05:34Z and was stopped, recorded in S-E1-LIVE progress). Every slow row's provocation finds its anchor (edit only, gate not run). Six hand-run scripts with no row pass (host-settings, gpl classifier, prom keep-list, vor export, host-state textfile, planned-stop).

**2026-11-16 evening, independent review of the day's coordinator commits** (`fb0502c2`, `7a62fd9c`, `1f69fbce`, plan 3's `4bfa81dd`, `2c868be4`): three real defects, all in `fb0502c2`, fixed in `a48259c0` tests first: (1) the overnight unit passed no `-audit-wotan`, so the hold was nil and every night ran unheld; (2) a fresh tracker reads "ok" before its first `alerts.critical` message (no replay, snapshot every 5 min), so the first card of each night and every `-worker -once` skipped the hold: new `Tracker.HoldUntilHeard` (Sword and chaos-controller keep `Hold`); (3) held nights were reported as `gpu_lease` deferrals, now `critical_alert`. One minor trap in `test-backup-restore.sh` (`43d3834a`). Nothing found in the nosec, LIVE-PATHS or Zhen test-script commits (all five provocation anchors match once; DSN variables, database names and owners line up with 051/052). critalert and zhen-agentd `-race` ok, lint 0, module-manifest and overnight-abort pytest 14 passed. Also on develop (merge in before the final merge): `d21e3de2` Akira reaps its exec checks' process group (23 zombies on west), `67f8356e` eBPF size pin.

**2026-11-17, a sweep for what the removal of the five role services left behind** (a read-only agent over the merged tree, then fixes; Stevie away). What it found that the gates run so far could not see, each fixed here:

- **Two gates that would have failed after the merge, both needing docker and so not run in the week:** `docs/policy/hadolint-baseline.tsv` and `trivy-misconfig-baseline.tsv` still counted findings in the deleted `docker/services/{monad,sophia}/Dockerfile`, and both gates fail on a count below its baseline. Six rows removed by hand (`0437e77a`); the gates run after the week.
- **Code and config that still called, started or drew the five** (`2907316d`, test first: `pkg/ports` `TestRetiredPortsStayOut` reads the caller files for ports 19001-19005 and, where a file lists services by name, for the names): `raft/zhen_app.py` (four health maps), `scripts/east-bootstrap.sh` and `lxd/hosts/host-b/launch-minimal.sh` (wrote and started monad and sophia), `scripts/bare-metal/validate-cross-host.sh`, the dashboard filters and the old packet-flow canvas, `cmd/dashboard-backend/services.example`, the containerd gateway spec, `cmd/demo-trace-injector`, the Dark Mirror files under `tomb/`, host-a's reservations, README's layer line.
- **`nix/containers/gateway.nix` listed five dependencies and still said `totalServices = 10`** (`7e2d6c22`). `TestGatewayServiceCountMatchesDependencies` in `scripts/lib` fails on it. The race run recorded above was `./cmd/... ./pkg/... ./services/...` and so never ran `scripts/lib`; **today's is `go test -race ./...`: 222 packages ok, 0 failed** after the fix (`~/tmp/adr147-table/runs/race-integrate-2026-11-17.log` holds the run that found it).
- **Live docs** (an agent on `fix/adr147-stale-refs`, four commits `92f3f28f`..`0a57fecd`, merged `3b154408`): 52 files: QUICKSTART, SECURITY, VISION, nine wiki pages, the architecture docs and their diagrams, `docs/protocol/openapi.yaml` (the five services' servers, tags, 39 paths and 20 schemas out; it loads and has no dangling reference), `WOTAN_TOPICS.md` regenerated from `configs/wotan.yaml`, the load-balancer, container, docker, nix and lxd docs with their counts recounted from the tree, the two topology demos. CLAUDE.md's auth line now says 9 first-party services (2026-11-17, from `scripts/compliance/check-service-auth.sh`).
- Also on this branch: ADR-150 (`2eb90933`), the draft design ADR series row 6 asks for; from develop, the Kanban form's GUID row (`f337d056`) and doc truth fixes.

Left as found, with reasons: `scripts/three-crowns/lib.sh` keeps the old binary names in its "no process left" check (east runs three of them until plan 6). `db/migrations/006_seed_data.sql` seeds `service_registry` with the five (an applied migration; nothing reads the table). `cmd/trace-collector-go/enrichment.go` names service ids 0x02-0x06 after the five (dictionary data, Stevie's). `nix/containers/developer-service.nix` and its test use retired port 19004 for a placeholder. The Zhen app's health commands are wrong beyond the five (kanban on 16668, akira on 16699, plain http against TLS-only listeners), so they report the stack down; a design question for plan 3's owner. Dated notes under `skills/`, `nix/DELIVERY.md` and the retired `SERVICE_BREAKOUT_STRATEGY.md` body are records. The agent's list of older drift it met on the way (lxd resource totals, docker docs, January topic names in three diagrams) is in `~/tmp/next.md`.

After these: the gates named above that need no container pass on the merged tree (spdx, nosec ratchet 845, live-path inventory, runbook manifest, CI verdicts, pytest 680). **`~/tmp/adr147-table/runs/after-week.sh`** runs the table below in a clean worktree, one log a gate, and refuses to start before 2026-11-17T18:34Z; it leaves out the rows that touch the live stack.

## 2026-11-17, the live phases read against the merged tree

Stevie asked for sprints ready for the day the week ends. Four read-only passes compared each plan's remaining phases with this branch. Each plan was written to merge and deploy alone; one merged tree changes what several steps meet.

- **`docs/battle-plans/ADR147-LIVE-RUN-ORDER.md`** is the result: the order of the live blocks across plans 1 to 5, the one merge that replaces each plan's own, the rulings still open, and the steps to run differently, by plan. Each plan carries a pointer to it at its first live phase.
- **A trap in the Well sitting, fixed** (`2dcb1d68`): migrations 050 (plan 2) and 051 to 053 (plan 3) are in one manifest, and `batch-apply.sh` applied everything pending, so plan 2 Step 341 would have run on into 053, which refuses until the rows are copied. `well-migrate apply -through FILE` and `batch-apply.sh --through NNN` now stop a batch at one file. The Go test was red first; the dry run was exercised against a stub runner; the new cases in `scripts/well/test-batch-apply.sh` need a throwaway server and have not run.
- **Amended in the plans:** plan 2 Step 340 (it would have set five roles' passwords, not one) and Step 341 (`--through 050`); plan 3 Step 266 (the file is 053) and Step 274 (limit 20); plan 1 Step 308 (`2h` and `180`) and Phase 16 (its build and recreate are plan 2's).
- **S-LOG-WEEK-2 forged** (`docs/battle-plans/S-LOG-WEEK-2-battle-plan.md`, 8 phases, 96 steps), ahead of its turn: its Phase 1 is repository work for the day the week ends. Its grounding found that the probe of the Well's own latency runs after the three queries, so week 1's latency row may have measured the reading itself, and that the copy would reach the writer's 4 GiB ceiling inside any second week that kept week 1's rows.

## What still must run after the evidence week

| Gate | Plan |
|---|---|
| `scripts/check-hadolint.sh`, `scripts/check-trivy-misconfig.sh` (baselines edited by hand 2026-11-17, never run on this tree), `check-haproxy-cfg.sh`, `check-image-digests.sh`, `check-compose-log-caps.sh`, `check-compose-bind-nesting.sh`, `test-pihole-history.sh` (CI runs each; each needs docker) | 2, all |
| `scripts/check-prometheus-rules.sh` (promtool in a container): every new rule test file, `containers_test.yml` with the timeguru fixture, the module rule tests | 1, 2, 3, 4 |
| `scripts/ci-local.sh` (throwaway Wells, haproxy image) | all |
| `scripts/well/test-batch-apply.sh`: the `--through` cases added 2026-11-17 (`2dcb1d68`), never run | 2, 3 |
| `scripts/monitoring/test-alert-path.sh` (cases 6, 7) | 1 |
| `scripts/backup/test-store-backups.sh` | 1 |
| `scripts/qa-smoke.sh` (psql) | 1, 2 |
| `scripts/well/test-zhen-database.sh`, the 050 migration test, `scripts/well/test-health-bridge.sh` | 3, 2 |
| `scripts/well/test-zhen-{agent-runs,captures,confirm,conversations}.sh`, `test-retention.sh`, `test-llm-requests.sh`, `test-backup-restore.sh`, `test-migrate.sh` (moved to unheaded_zhen 2026-11-16 without a server: plan 3's progress file, entry of that date) and their four provocations, now on 052 | 3 |
| `scripts/test-zhen-switch.sh` (live) | 3 |
| `sudo scripts/check-ebpf-loads.sh` and `check-ebpf-insns.sh` (Phase 15 objects) | 2 |
| `check-gates-can-fail.sh` full run (slow provocations need docker and sudo) | all |
| `alert-policy -sync-well` (store-backups SLO, wotan text) | 1 |
| Rebuild dashboard-backend (any compose up from this tree does): the running image (built 2026-11-12) refuses develop's catalog since S-EVIDENCE-RUNS added evidence kind `host-service` (11-15 05:26Z), so `ComplianceReportBroken` fires and every other compliance series is frozen at its last good value (`ComplianceEvidenceSilent`, `-FindingsOverdue`, `-FindingsUntriaged` are stale, not read). Proven offline 2026-11-16: the 11-12 `crosswalk` refuses today's `compliance/`, today's reads it (24,108 records). Design note: a running service reads `compliance/` from the moving develop checkout, so a catalog change outruns its binary | all |
| Rebuild kanban-app (develop `f337d056`): the New Task form shows an empty read-only GUID box until then | all |
| Rebuild akira (develop `d21e3de2`): the running one, PID 1 in its container, holds 23 zombies from timed-out cert-expiry checks (32 timeouts since 11-11, most under load at 11-12 06Z and 11-16 00Z); they go when the container is recreated. Also: the cert-expiry check runs 30 s against a 30 s timeout under load, worth a look | all |
| `sprint/s-west-ground-compose`: merge develop in, rerun `check-compose-pids.sh` after plans 2 and 3 removed services | 4 Phase 7 |

## For Stevie and the coordinator

- zhen-agentd now gives up after 15 minutes (plan 1 SS3) instead of never (plan 3 kept `0`). Plan 1 asked whether Zhen should be the exception.
- Plan 2's handoff to plan 3, done here 2026-11-16 (`fb0502c2`): zhen-agentd's worker defers a card while a critical alert fires (tracker on the `-audit-wotan` client; without it, unknown allows). Yours: whether the scheduler's runbook runs should hold too (left running: they may be the diagnostics an outage needs).
- On the live day: plan 2 stops the role services and puts Cuirass behind `profiles: ["lxd"]`; their old containers will read as stopped to plan 1's ContainerStopped (a page) until removed (plan 2 Phase 19) or older than 7 days.
- `runbooks/MANIFEST.sha256` (102 runbooks) needs the seal again.
- `scripts/gen-prom-keep-list.py` gained `--modules` (`5ffa0b66`): an integration change to plan 2's tool, review it with plan 2.
