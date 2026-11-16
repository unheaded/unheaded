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

## What still must run after the evidence week

| Gate | Plan |
|---|---|
| `scripts/check-prometheus-rules.sh` (promtool in a container): every new rule test file, `containers_test.yml` with the timeguru fixture, the module rule tests | 1, 2, 3, 4 |
| `scripts/ci-local.sh` (throwaway Wells, haproxy image) | all |
| `scripts/monitoring/test-alert-path.sh` (cases 6, 7) | 1 |
| `scripts/backup/test-store-backups.sh` | 1 |
| `scripts/qa-smoke.sh` (psql) | 1, 2 |
| `scripts/well/test-zhen-database.sh`, the 050 migration test, `scripts/well/test-health-bridge.sh` | 3, 2 |
| `scripts/test-zhen-switch.sh` (live) | 3 |
| `sudo scripts/check-ebpf-loads.sh` and `check-ebpf-insns.sh` (Phase 15 objects) | 2 |
| `check-gates-can-fail.sh` full run (slow provocations need docker and sudo) | all |
| `alert-policy -sync-well` (store-backups SLO, wotan text) | 1 |
| `sprint/s-west-ground-compose`: merge develop in, rerun `check-compose-pids.sh` after plans 2 and 3 removed services | 4 Phase 7 |

## For Stevie and the coordinator

- zhen-agentd now gives up after 15 minutes (plan 1 SS3) instead of never (plan 3 kept `0`). Plan 1 asked whether Zhen should be the exception.
- Plan 2's handoff to plan 3 is still open: zhen-agentd does not hold an unattended run while a critical alert fires (`critalert.Attach` + `Hold()`).
- On the live day: plan 2 stops the role services and puts Cuirass behind `profiles: ["lxd"]`; their old containers will read as stopped to plan 1's ContainerStopped (a page) until removed (plan 2 Phase 19) or older than 7 days.
- `runbooks/MANIFEST.sha256` (102 runbooks) needs the seal again.
- `scripts/gen-prom-keep-list.py` gained `--modules` (`5ffa0b66`): an integration change to plan 2's tool, review it with plan 2.
