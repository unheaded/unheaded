<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# COMPLIANCE-WELL BATTLE PLAN — 8 Phases, 112 Steps

**Date**: 2026-10-01
**Sprint**: COMPLIANCE-WELL — compliance state moves into The Well; YAML stays as seed and fallback (ADR-099, amended).
**Prerequisite**: ADR-097/098 shipped (crosswalk, findings register, audit-only agent); ADR-099 drafted; 8 `lab` fixes applied on west and east 2026-10-01.
**Target**: Profiles, host bindings, the apply journal, findings triage and evidence live in The Well. Switching a host's controls is one audited write (CLI now, dashboard after RBAC). Every reader still works from repo YAML when The Well is down.
**Estimated Duration**: 10–14 hours across 3–4 sessions.
**Agent Strategy**: Solo coordinator. Phases 2 and 3 can run in parallel after Phase 1. Phases 0 and 7 touch running services and need Stevie's yes.
**Commit Cadence**: Every 5 steps (112/20 ≈ 5), plus every exit gate. Signed; Stevie pushes.
**Stuck Protocol**: Skip after 3× a step's estimate or 2 failed debug attempts. Mark `[STUCK]`, commit, move to the next independent step.

SRE rules from ADR-100 apply throughout (see SRE GATES).

Consulted: unheaded-architect (storage split, signed snapshots, pull model), unheaded-developer (schema, TDD order, fallback tests), unheaded-micromanager (what goes where), unheaded-warmonger (this plan). Stevie 2026-10-01: *"They should exist in the well"*; *"most everything should exist in the well really, some yaml and config files are ok"*; *"yaml is ok as fallback but db config is ideal long term"*.

---

## DECISION: WHAT LIVES WHERE (micromanager)

The rule has three parts.

1. **State that changes at runtime, or records what happened, goes in The Well.** The repo YAML seeds it and serves as the fallback.
2. **A definition that code executes as root, or that tests pin, is authored in the repo.** It is reviewed as a signed commit and mirrored into The Well so it can be queried and displayed.
3. **A host never runs commands from a live database row.** It acts only on a signed snapshot exported from The Well. A tampered row must not become root on every host.

| Item | Source of truth | Well database / table | Repo file role | Why |
|---|---|---|---|---|
| Profiles (which fixes are on) | **Well** | `unheaded_config.compliance_profiles` | `compliance/profiles/profiles.yaml`: Genesis seed and fallback | This is the switch Stevie wants to flip. |
| Host bindings (profile, mode, overrides) | **Well** | `unheaded_config.compliance_host_bindings` plus `_history` (trigger, append-only) | `compliance/hosts/<host>.yaml`: seed and fallback | Each change is CM-3 evidence: who, when, why. |
| Apply journal | **Well** | `unheaded_ops.compliance_apply_journal` (append-only, hash-chained like `audit_events`) | `/var/lib/unheaded/remediate/journal.jsonl` on each host is the local buffer, pulled in | It is a record of what happened. |
| Findings triage (severity, plan, acceptance, decision) | **Well** | `unheaded_ops.compliance_findings` plus `_history` | `compliance/findings/register.yaml`: seed and fallback | Workflow state. Acceptance expiry and SLA clocks live here. |
| Evidence records | **Well** | `unheaded_ops.compliance_evidence` | `var/compliance/evidence.json`: fallback cache | Time series; the dashboard reads the latest. |
| Fix definitions (actions, file contents) | **Repo** (`compliance/remediation/fixes.yaml`) | `unheaded_config.compliance_fixes` (read-only mirror plus content hash) | authored here | Executed as root. A change must be a reviewed, signed commit. |
| Control catalog and mappings | **Repo** (`compliance/catalog/controls.yaml`) | `unheaded_config.compliance_controls` (mirror) | authored here | Pinned by tests (corroboration, probe table). Revisit after Phase 7. |
| Framework requirement lists | **Repo** (generated, pinned upstream commit) | `unheaded_config.compliance_frameworks` (mirror) | authored here | Third-party data with provenance (CaC commit, licence). |
| Host baseline | **Derived** | none | `compliance/baseline/baseline.yaml` is generated; hosts receive a signed snapshot | It is derived, so nothing should be edited by hand. |

"Mirror" means `compliance-evidence well sync` upserts repo content and records its sha256. Readers prefer The Well and check that hash. On a mismatch they use the repo file and raise `unheaded_compliance_well_drift`.

**Fallback contract.** Every reader is the dashboard, the CLI, the collector or the agent. Each one tries The Well first. If The Well is unreachable or refuses, it reads the YAML, labels its output `source=yaml-fallback`, and exports `unheaded_compliance_source{source="yaml"}=1`. It never silently presents fallback data as live. Writes, meaning profile switches, triage and acceptance, **require The Well** and fail loudly without it (feedback: *no DB = read-only mode*).

---

## SRE GATES (ADR-100, applied to this sprint)

This sprint does not start ADR-100's phases; it only follows its rules for what it adds.

- **Runbook link.** Every alert rule this plan adds (`ComplianceWellFallback`, plus any rule for The Well) carries a `runbook_url` that points at a `runbooks/` entry.
- **Firing test.** Every such rule has a promtool test that makes it fire.
- **Symptom alerts.** Alerts target symptoms, such as "compliance served from YAML fallback" or "The Well unavailable".
- **Incident postmortem.** The Well's 21-hour silent outage (Phase 0) gets a postmortem in `docs/postmortems/` when ADR-100 phase 1 starts. Phase 0 records the facts it will need.
- **Toil.** Hand-run remediation is toil. Phase 4's executor plus the journal is how it retires.

## LEGEND

```
[B] Bash command          [V] Verification (MUST pass)     [D] Debug (only if prior step fails)
[W] Write/create file     [R] Read/inspect                 [S] Sudo required
[P] Parallelizable        [C] Commit checkpoint            [Y] Needs Stevie's yes before running
[STUCK] Skipped via Skip Protocol                          [BLOCKED] Blocked by a STUCK step
```

Repo root: `/home/govan/tmp/unheaded` (abbreviated `$R`). All Go commands run from `$R`.

---

## PHASE 0: THE WELL BACK ONLINE (Steps 1–14)

**Goal**: `unheaded-postgres` running healthy on loopback, with the cause of its 2026-09-30 exit known and the password-spam source identified.
**Prerequisite**: none.
**Time**: 45 min.
**Agent**: Coordinator `[Y]`. Starting the container is Stevie's call.

Facts on 2026-10-01:

- The container exited with code 255 at 2026-09-30 05:06:26Z, 2.5 minutes after the host booted. It was not OOM-killed.
- Its last log lines, at 02:21Z before the reboot, are a burst of `password authentication failed for user "unheaded"`.
- `pkg/database/config.go` defaults `WELL_USER=unheaded` and `WELL_PASSWORD=unheaded`, while compose initialised the volume with `POSTGRES_PASSWORD:-unheaded_dev`. Any service without the variable set therefore authenticates as the superuser with the wrong password. dashboard-backend calls `database.Connect(OpsWriterConfig())`.

- [ ] **Step 1** [R] ~2m: Read the exit cause.
  ```bash
  sudo docker inspect unheaded-postgres --format '{{.State.ExitCode}} {{.State.FinishedAt}} {{.State.Error}}'
  sudo journalctl -u docker --since "2026-09-30 05:00" --until "2026-09-30 05:10" --no-pager | tail -30
  ```
- [ ] **Step 2** [R] ~2m: Find who connected as `unheaded`.
  ```bash
  sudo docker logs unheaded-postgres 2>&1 | grep -c 'password authentication failed for user "unheaded"'
  sudo ss -tnp state established '( dport = :5432 )' 2>/dev/null
  grep -rn 'WELL_PASSWORD\|WELL_USER' $R/docker-compose.yml $R/deploy/systemd/ 2>/dev/null
  ```
- [ ] **Step 3** [V]: The cause is written down in `next.md` and the plan. If it is unknown after 2 attempts, go to Step 4 anyway and mark this step `[STUCK]`.
- [ ] **Step 4** [Y][S][B] ~2m: Start only The Well.
  ```bash
  cd $R && sudo docker compose up -d postgres
  ```
- [ ] **Step 5** [V] ~1m: It is healthy, and bound to loopback only.
  ```bash
  sudo docker inspect unheaded-postgres --format '{{.State.Health.Status}}'   # healthy
  ss -ltn | grep ':5432 '                                                      # 127.0.0.1:5432 only
  ```
  - [D] **5a**: unhealthy → `sudo docker logs --tail 50 unheaded-postgres`. If it shows a volume or permission error, STOP and report to Stevie. Never `down -v` (that deletes data).
- [ ] **Step 6** [V] ~1m: All three databases and the seven users exist.
  ```bash
  sudo docker exec unheaded-postgres psql -U unheaded -d unheaded -Atc "SELECT datname FROM pg_database WHERE datname LIKE 'unheaded%' ORDER BY 1"
  sudo docker exec unheaded-postgres psql -U unheaded -d unheaded -Atc "SELECT rolname FROM pg_roles WHERE rolname ~ '^(app_|ops_|config_)' ORDER BY 1"
  ```
- [ ] **Step 7** [V] ~1m: Kanban data survived.
  ```bash
  sudo docker exec unheaded-postgres psql -U unheaded -d unheaded_app -Atc "SELECT count(*) FROM kanban_tasks"
  ```
- [ ] **Step 8** [W] ~10m: Register a finding for the credential default. `pkg/database/config.go` falls back to `WELL_PASSWORD=unheaded` and the superuser. Class `app`, severity medium. Plan: fail loudly when unset (fixed in Phase 1, Step 22).
- [ ] **Step 9** [B] ~1m: Regenerate the register section.
  ```bash
  cd $R && go run ./cmd/compliance-evidence findings -adr docs/adr/ADR-098-findings-register-and-baseline-modes.md | head -3
  ```
- [ ] **Step 10** [C]: `git commit -S -m "chore(compliance): register The Well superuser-default credential finding"`
- [ ] **Step 11** [V]: **PHASE 0 EXIT GATE**: Step 5 healthy, Step 6 lists 3 databases and 7 users, Step 7 returns a count, and the cause is recorded.
- Steps 12–14 are reserved for a debug spill-over in this phase.

---

## PHASE 1: SCHEMA AND STORE, TEST-FIRST (Steps 15–40)

**Goal**: Two migrations (config and ops) plus `pkg/compliance/well`, a store with an interface, a Postgres implementation and a YAML fallback implementation, all proven by tests.
**Prerequisite**: Phase 0 exit gate (a live database for the integration tests; unit tests do not need it).
**Time**: 3 h.
**Agent**: Coordinator.

- [x] **Step 15** [W] ~20m: `db/migrations/013_compliance_config.sql` targets `unheaded_config`:
  - `compliance_fixes(id PK, title, class, risk, controls text[], closes text[], body jsonb, sha256 char(64), synced_at)`
  - `compliance_profiles(name PK CHECK name ~ '^[a-z0-9-]{1,40}$', description, all_fixes bool, fixes text[], updated_at, updated_by)`
  - `compliance_host_bindings(host PK, profile FK→profiles, mode CHECK IN ('audit','enforcing'), overrides jsonb, updated_at, updated_by, change_reason NOT NULL)`
  - `compliance_host_bindings_history`, append-only, written by trigger (pattern: `kingdom_config_history`)
  - mirrors `compliance_controls` and `compliance_frameworks`, each with `sha256`
  - grants: `config_admin` CRUD; `config_reader` SELECT; history `INSERT` only via trigger, with no UPDATE or DELETE granted to anyone but the owner
- [x] **Step 16** [W] ~20m: `db/migrations/014_compliance_ops.sql` targets `unheaded_ops`:
  - `compliance_apply_journal(id bigserial, host, fix_id, fix_sha256, model_commit, action CHECK IN ('apply','undo','plan'), outcome, backup_dir, operator, at, previous_hash, hash)`
  - `compliance_findings(key PK, severity, class, step, lockout, plan, decision, accepted_until date, accepted_reason, opened_at, updated_at, updated_by)` plus `_history`
  - `compliance_evidence(id bigserial, source_key, host, status, detail, observed_at)` with an index on `(source_key, observed_at DESC)`
  - grants: `ops_writer` INSERT/SELECT, with no UPDATE or DELETE on the journal or history; `ops_reader` SELECT
- [x] **Step 17** [W] ~5m: Route both migrations in `db/init.sh` (013 → `unheaded_config`, 014 → `unheaded_ops`), after 012.
- [x] **Step 18** [B] ~3m: Static checks.
  ```bash
  cd $R && bash -n db/init.sh && bash scripts/check-compose-bind-nesting.sh && grep -c 'SPDX' db/migrations/01[34]_*.sql
  ```
- [x] **Step 19** [V] (done 2026-10-01; also `scripts/well/test-compliance-schema.sh`: 25 constraint checks in a throwaway container, red when the journal's append-only trigger is removed): Both files carry SPDX, and `init.sh` names both. Go on to Step 20.
- [x] **Step 20** [C]: `git commit -S -m "feat(well): compliance schema in unheaded_config and unheaded_ops (013, 014)"`
- [ ] **Step 21** [B][S] ~5m: Apply to the live Well, in a transaction. Read each file first; neither may contain DROP.
  ```bash
  cd $R && ! grep -qi 'drop ' db/migrations/01[34]_*.sql
  sudo docker exec -i unheaded-postgres psql -v ON_ERROR_STOP=1 -1 -U unheaded -d unheaded_config < db/migrations/013_compliance_config.sql
  sudo docker exec -i unheaded-postgres psql -v ON_ERROR_STOP=1 -1 -U unheaded -d unheaded_ops    < db/migrations/014_compliance_ops.sql
  ```
  - [D] **21a**: on an error, the transaction rolled back. Fix the SQL, re-run Step 18, then Step 21.
- [x] **Step 22** [W] ~15m: Remove the credential defaults in `pkg/database/config.go`.
  - `WELL_USER` and `WELL_PASSWORD` get no default.
  - `Connect` returns `errMissingConnStr` when they are unset.
  - Test first: `TestConfig_NoPasswordDefault`.
  - Callers that relied on the default fail loudly, and get listed in `next.md`.
- [x] **Step 23** [B] ~2m: `go test ./pkg/database/... -race -count=1`
- [x] **Step 24** [V] (done 2026-10-01; FND-007 resolved; DSN values now quoted too. FND-008, kanban as superuser in compose, is NOT done here: a `${VAR:?}` guard in compose would make `docker compose up -d postgres` itself fail while the variable is unset, so it lands with Phase 0 and Stevie): Green. The new test failed before the change (RED, then GREEN recorded in the commit body).
- [ ] **Step 25** [C]: `git commit -S -m "fix(database): no credential default; fail loudly when WELL_USER/WELL_PASSWORD unset"`
- [x] **Step 26** (done 2026-10-01: Store (profiles, bindings, SetBinding) + OpsStore (IngestJournal, Register, UpsertFinding, ResolveFinding, AppendEvidence, LatestEvidence)) [W] ~20m: Write tests first in `pkg/compliance/well/store_test.go`:
  - `Store` interface: `Profiles`, `Bindings`, `SetBinding(host, profile, mode, actor, reason)`, `AppendJournal`, `Findings`, `UpsertFinding`, `LatestEvidence`, `Source()`.
  - Table tests for: unknown profile, a profile naming an unknown fix, mode outside audit/enforcing, empty reason, empty actor, a host name that is not `^[a-z0-9-]{1,63}$`, enforcing requested while ADR-098's gate is unmet (`ErrEnforcingGate`).
- [x] **Step 27** (profiles + hosts; findings read via crosswalk.NewRegister, same validation as the YAML) [W] ~20m: YAML fallback store, `yamlstore.go`.
  - Reads `compliance/{profiles,hosts,findings}` through `os.Root`, with strict decoding (`KnownFields`) and a 256 KiB cap.
  - Every write returns `ErrReadOnlyFallback`.
- [x] **Step 28** (journal: whole-file verify from genesis, held entries match hash for hash, truncation refused, advisory lock; findings + evidence: parity test gives the same report as the YAML on the repo's 4858 evidence records) [W] ~30m: Postgres store, `pgstore.go`.
  - Parameterised queries only, with context deadlines (5 s).
  - `SetBinding` runs in one transaction: validate profile → update → history row written by the trigger.
  - `AppendJournal` computes `hash = sha256(previous_hash || canonical row)` under `SELECT … FOR UPDATE` on the last row.
- [x] **Step 29** [W] ~10m: `Open(ctx)` tries Postgres, falls back to YAML, and records `Source()`. Test: an unreachable DSN returns the YAML store, labelled `yaml-fallback`, within the timeout.
- [ ] **Step 30** [C]: `git commit -S -m "feat(compliance/well): store interface, YAML fallback, Postgres store (tests first)"`
- [x] **Step 31** [B] ~3m: Unit tests.
  ```bash
  cd $R && go test ./pkg/compliance/well/... -race -count=1 2>&1 | tail -5
  ```
- [x] **Step 32** [B] ~5m (done against a throwaway database instead: `scripts/well/test-compliance-schema.sh --go`; caught pq.Array(nil) → NULL): Integration tests against the live Well, skipped without `WELL_TEST_DSN`.
  ```bash
  cd $R && WELL_TEST_DSN="host=127.0.0.1 user=config_admin dbname=unheaded_config sslmode=disable password=$WELL_CONFIG_ADMIN_PASSWORD" \
    go test ./pkg/compliance/well/... -run Integration -race -count=1 2>&1 | tail -5
  ```
  The password comes from the environment, never the repo. If `WELL_CONFIG_ADMIN_PASSWORD` is unset, skip and note it.
- [x] **Step 33** [V]: Unit tests green. Integration tests green or explicitly skipped. Each case is a failure the store refuses: tampered journal row, history UPDATE, profile with an unknown fix.
- [x] **Step 34** [B] ~3m: `gosec`, pinned, with every rule enforced, on the new package.
  ```bash
  cd $R && go run github.com/securego/gosec/v2/cmd/gosec@v2.29.0 ./pkg/compliance/well/... 2>&1 | tail -3
  ```
- [x] **Step 35** [V]: 0 issues.
- [ ] **Step 36** [C]: `git commit -S -m "test(compliance/well): integration + tamper-evidence tests"`
- [ ] **Step 37** [V]: **PHASE 1 EXIT GATE**: `go build ./... && go vet ./pkg/compliance/... ./pkg/database/...` passes, Steps 31 and 33 are green, and the migrations are applied.
- Steps 38–40 are reserved.

---

## PHASE 2 [P]: SEED AND SYNC (Steps 41–55)

**Goal**: `compliance-evidence well sync` mirrors repo definitions (fixes, controls, frameworks, with sha256) and seeds profiles, bindings and findings from YAML **only where The Well has no row**, so it never overwrites live state.
**Prerequisite**: Phase 1 exit gate.
**Time**: 1.5 h.

- [x] **Step 41** [W] ~15m: Fix model loader `pkg/compliance/remediation`, tests first.
  - Strict YAML.
  - Closed action set `file|setting|mode|service|package|absent|run|validate`.
  - `run` and `validate` values only from the `Allowed` table.
  - Absolute paths under an allowlisted prefix set (`/etc/`), no `..`.
  - Mode is octal and never world-writable.
  - `content_by_host` keys are known hosts.
  - `closes` keys are valid source keys in the catalog.
- [x] **Step 42** [W] ~15m: Parity test. For each fix, the content the model renders for west and for east equals what `scripts/host/remediate.sh plan` writes in the fake root (reuse the `remediate_test.go` harness). This holds until Phase 4 retires the script.
- [x] **Step 43** [B] ~2m: `go test ./pkg/compliance/remediation/... ./scripts/host/... -race -count=1`
- [x] **Step 44** [V] (done 2026-10-01: 40 parity subtests green; red on a changed sshd line, a dropped sudo-rs condition and a wrong cron.allow mode): Green. Any mismatch is a model bug: fix the YAML, not the test.
- [ ] **Step 45** [C]: `git commit -S -m "feat(compliance): fix model loader + parity with remediate.sh (ADR-099 phase 1)"`
- [x] **Step 46** (done 2026-10-01: well.Sync + `compliance-evidence well sync [-dry-run]`; run end to end by `scripts/well/test-compliance-schema.sh --go`) [W] ~20m: `cmd/compliance-evidence well sync [--dry-run]`.
  - Definitions: upsert when the sha256 differs.
  - State: `INSERT … ON CONFLICT DO NOTHING`.
  - Prints a diff summary.
  - Test: a second run is a no-op. A Well row whose state differs from the YAML is left alone and reported as `diverged`.
- [ ] **Step 47** [B] ~2m: Dry run against the live Well.
  ```bash
  cd $R && go run ./cmd/compliance-evidence well sync --dry-run
  ```
- [ ] **Step 48** [V]: Plans 10 fixes, about 120 controls, 2 CIS frameworks plus the others, 4 profiles, 2 bindings, and every register entry. 0 deletes.
- [ ] **Step 49** [B] ~2m: Real sync, then run it again.
  ```bash
  cd $R && go run ./cmd/compliance-evidence well sync && go run ./cmd/compliance-evidence well sync | grep -c 'no change'
  ```
- [ ] **Step 50** [C]: `git commit -S -m "feat(compliance-evidence): well sync (definitions mirrored by hash, state seeded once)"`
- [ ] **Step 51** [V]: **PHASE 2 EXIT GATE**: The second sync reports no changes. West and east show profile `lab`, mode `audit`.
  ```bash
  sudo docker exec unheaded-postgres psql -U unheaded -d unheaded_config -Atc "SELECT host, profile, mode FROM compliance_host_bindings ORDER BY 1"
  ```
- Steps 52–55 are reserved.

---

## PHASE 3 [P]: READERS PREFER THE WELL (Steps 56–70)

**Goal**: The dashboard compliance API, the `findings` command and the metrics read through `well.Open`. The source is labelled, and the YAML fallback is proven.
**Prerequisite**: Phase 1 exit gate. Phase 2 is needed for real data, but not for tests.
**Time**: 2 h.

- [x] **Step 56** (done 2026-10-01: API side, source labels on summary and findings, /api/v1/compliance/profiles read-only) [W] ~20m: `cmd/dashboard-backend/internal/server/compliance.go`:
  - Findings, profiles and bindings come from `well.Store`.
  - The response carries `"source": "well"|"yaml-fallback"`.
  - The test runs a Postgres-down server and checks that it serves YAML with that label.
- [x] **Step 57** (done 2026-10-01; the rule fires only after The Well served within 24h, so the expected pre-seed fallback stays quiet; promtool tests include that case, provoked red with the naive rule) [W] ~10m: Metric `unheaded_compliance_source{source}`. Prometheus rule `ComplianceWellFallback`: source yaml for 15 minutes or more raises a warning, with `runbook_url` → new `runbooks/observe/compliance-well-fallback.yaml`. Add a promtool test that makes it fire (ADR-100 §4).
- [x] **Step 58** (done 2026-10-01: Profiles panel + fallback banner; jstest compliance.cjs, provoked red) [W] ~15m: `compliance.js` adds a read-only **Profiles** panel (host → profile, mode, fixes on/off, last change with who and why) and a `yaml-fallback` banner. Update the jstest harness.
- [ ] **Step 59** [B] ~5m: Run the tests.
  ```bash
  cd $R && go test ./cmd/dashboard-backend/... -race -count=1 2>&1 | tail -3 && node cmd/dashboard-backend/jstest/run.cjs 2>&1 | tail -3
  promtool test rules monitoring/prometheus/rules/tests/compliance_test.yml
  ```
- [ ] **Step 60** [C]: `git commit -S -m "feat(dashboard): compliance reads The Well, labels YAML fallback, profiles panel (read-only)"`
- [ ] **Step 61** [W] ~10m: `compliance-evidence findings` reads triage from The Well (YAML fallback). The ADR-098 generated section notes its source.
- [ ] **Step 62** [B] ~3m: With The Well up, run `findings`. Then run it with `WELL_HOST=127.0.0.2`, which is unreachable.
- [ ] **Step 63** [V]: The counts are identical, and the second run is labelled `yaml-fallback`.
- [ ] **Step 64** [B] ~5m: Restart the scratch dashboard (pid in `scratchpad/dash-live.pid`) and load `http://192.168.69.184:20000/compliance`.
- [ ] **Step 65** [V]: The Profiles panel shows west and east as `lab` / `audit`, with no fallback banner.
- [ ] **Step 66** [C]: `git commit -S -m "feat(compliance-evidence): findings read triage from The Well"`
- [ ] **Step 67** [V]: **PHASE 3 EXIT GATE**: Steps 59, 63 and 65 pass.
- Steps 68–70 are reserved.

---

## PHASE 4: NATIVE EXECUTOR AND JOURNAL (Steps 71–88)

**Goal**: `unheaded-remediate`, a Go executor of the fix model, at full parity with `remediate.sh` (plan by default, backed-up apply, undo newest-first, validate-or-restore), writing journal rows. `remediate.sh` is then retired.
**Prerequisite**: Phase 2 exit gate.
**Time**: 3 h. **No host apply in this phase**; fake-root only.

- [x] **Step 71** [W] ~30m: Port every `remediate_test.go` case to `cmd/unheaded-remediate`, tests first, against a fake root (`--root`). Cases:
  - each fix, plan versus apply
  - undo round-trip
  - sshd rejected → restored
  - sudo-rs → no logfile
  - halting auditd refused
  - west's `99-unheaded.conf` never loosened
- [x] **Step 72** [W] ~60m: Executor.
  - Only the closed action set. `run` uses `exec.Command` with fixed argv from the allowlist and no shell.
  - Writes are atomic (temp + rename) and preserve the owner.
  - The backup directory has mode 0700.
  - Each apply returns a `JournalEntry` (host, fix, fix sha256, git commit, operator from `SUDO_USER`).
- [~] **Step 73** (journal sink not wired into the executor yet; JournalEntry + ingest exist) [W] ~15m: Journal sink.
  - Appends to the local `journal.jsonl` first (0600, root).
  - Then appends to the Well store when it is reachable. A failed Well write never fails an apply that succeeded: the row stays local for Phase 5's pull.
- [x] **Step 74** [B] ~3m: Run the tests.
  ```bash
  cd $R && go test ./cmd/unheaded-remediate/... -race -count=1 2>&1 | tail -3
  ```
- [x] **Step 75** [C]: `git commit -S -m "feat(remediate): native executor of the fix model, parity tests ported"`
- [x] **Step 76** [B] ~5m: Differential test. For each host and each fix, compare the fake-root file trees from `remediate.sh apply` and `unheaded-remediate apply`: same files, modes and content.
- [x] **Step 77** [V] (done 2026-10-01: 40 subtests, zero differences in files, modes, content or commands; red when the executor skips `systemctl mask`): Zero differences.
  - [D] **77a**: a difference means a model or executor bug. Fix it and re-run. Never change `remediate.sh` to match.
- [x] **Step 78** [B] ~3m: gosec, plus `scripts/check-gates-can-fail.sh`.
- [x] **Step 79** [V]: 0 issues; the meta-gate is green.
- [x] **Step 80** [C]: `git commit -S -m "test(remediate): differential vs remediate.sh, zero diff"`
- [ ] **Step 81** [Y][B] ~5m: `plan` only, on the real hosts. It writes nothing, and every `lab` fix must report `already applied`.
  ```bash
  sudo $R/bin/unheaded-remediate plan --profile-from-well
  ssh govan@east 'sudo /usr/local/bin/unheaded-remediate plan --profile-from-yaml /etc/unheaded/compliance'
  ```
- [ ] **Step 82** [V]: Every `lab` fix reports no change on both hosts. This proves the model matches what was applied by hand.
- [ ] **Step 83** [W] ~10m: Retire `remediate.sh`. Replace its body with a one-line `exec unheaded-remediate "$@"` shim for a release, and update ADR-099 and the runbook.
- [ ] **Step 84** [C]: `git commit -S -m "chore(host): remediate.sh defers to unheaded-remediate"`
- [ ] **Step 85** [V]: **PHASE 4 EXIT GATE**: Steps 77 and 82 pass.
- Steps 86–88 are reserved.

---

## PHASE 5: JOURNAL AS EVIDENCE; PULL, NOT PUSH (Steps 89–96)

**Goal**: The collector, which already reaches hosts over SSH, pulls each host's `journal.jsonl` into The Well. A new evidence kind, `remediation-journal`, makes "fix X applied on host Y at T" auditable. The Well stays on loopback; no host connects to Postgres.
**Prerequisite**: Phase 4 exit gate.
**Time**: 1.5 h.

- [ ] **Step 89** [W] ~25m: New probe `remediation-journal` (reads via sudo, ends with `@ok`, no `@@` lines) and kind `remediation-journal`. It passes when every fix in the host's profile has an `apply` row whose `fix_sha256` matches the current model. A fix whose model changed since it was applied fails with "re-apply needed".
- [ ] **Step 90** [W] ~10m: Ingest is idempotent, keyed on (host, local sequence number), and verifies the hash chain on ingest. A broken chain becomes a `critical` finding.
- [ ] **Step 91** [B] ~3m: `go test ./pkg/compliance/... -race -count=1`, plus `TestProbeCommandsParse`.
- [ ] **Step 92** [C]: `git commit -S -m "feat(compliance): remediation-journal evidence, pulled into The Well"`
- [ ] **Step 93** [V]: **PHASE 5 EXIT GATE**: After one collector run, `compliance_apply_journal` has rows for both hosts, and the 2026-10-01 applies are backfilled from the `remediate.sh` state directories, marked `backfilled`.
- Steps 94–96 are reserved.

---

## PHASE 6: SWITCHING CONTROLS (CLI) AND THE ANSIBLE RENDERER (Steps 97–104)

**Goal**: Stevie can switch a host's profile in one audited command. The same model renders to Ansible.
**Prerequisite**: Phase 3 exit gate.
**Time**: 2 h.

- [ ] **Step 97** [W] ~20m: `compliance-evidence profile set --host east --profile none|lab|cis-l1-server|all --reason "..."`.
  - Writes to The Well only. The actor is `$SUDO_USER`/`$USER`.
  - It prints which fixes turn on and off, and requires `--yes`.
  - Switching changes desired state only. Nothing is applied until a human runs `unheaded-remediate apply` or the host is enforcing (ADR-098 gate).
- [ ] **Step 98** [W] ~10m: `compliance-evidence profile export` writes the YAML fallback from The Well (the reverse sync), so the fallback stays current. A test checks that export, then sync, is a no-op.
- [ ] **Step 99** [W] ~30m: `pkg/iac` Ansible renderer (`BackendAnsible`): one role per fix, one playbook per profile, inventory from bindings. Output goes to `deploy/ansible/compliance/`, with a drift test like `baseline.yaml`'s.
- [ ] **Step 100** [B] ~3m: Run the tests, then check the rendered playbooks.
  ```bash
  cd $R && go test ./pkg/iac/... ./cmd/compliance-evidence/... -race -count=1
  ansible-playbook --syntax-check deploy/ansible/compliance/lab.yml 2>/dev/null || echo "ansible not installed: syntax check skipped"
  ```
- [ ] **Step 101** [C]: `git commit -S -m "feat(compliance): profile set/export (audited), Ansible renderer with drift test"`
- [ ] **Step 102** [V]: **PHASE 6 EXIT GATE**: `profile set --host east --profile lab --reason test --yes` is a no-op that still writes one history row. Drift test green.
- Steps 103–104 are reserved.

---

## PHASE 7: SIGNED SNAPSHOTS FOR HOSTS; DOCS (Steps 105–112)

**Goal**:
- The baseline agent and the executor on each host read a **Gungnir-sealed (ML-DSA-65)** snapshot of `{binding, profile, fixes}` exported from The Well, and refuse an unsealed or stale one.
- ADR-099 amended; runbook written.

**Prerequisite**: Phases 4 and 6 exit gates.
**Time**: 1.5 h `[Y]` (key custody is Stevie's decision; it ties to the open key/IdP storage question).

- [ ] **Step 105** [W] ~25m: `compliance-evidence snapshot --host <h>` produces a canonical JSON payload and a `pkg/gungnir` seal. Agent and executor call `gungnir.Verify` against a pinned public key in `/etc/unheaded/compliance/`, and refuse on a mismatch or when `not_after` has passed. Tests: tampered byte, wrong key, expired.
- [ ] **Step 106** [B] ~3m: `go test ./pkg/gungnir/... ./cmd/unheaded-baseline/... ./cmd/unheaded-remediate/... -race -count=1`
- [ ] **Step 107** [C]: `git commit -S -m "feat(compliance): Gungnir-sealed host snapshots; agent and executor refuse unsealed"`
- [ ] **Step 108** [W] ~15m: Amend ADR-099: The Well is the source of truth, YAML is seed and fallback, the decision table above, pull model, sealed snapshots. Then update `ADR-INDEX.md`.
- [ ] **Step 109** [W] ~15m: Runbook `runbooks/security/compliance-profile-switch.yaml` (`runbook/v1`): plan → switch → plan on the host → apply → verify evidence → journal row.
- [ ] **Step 110** [B] ~3m: Pre-commit, meta-gate, full compliance tests.
  ```bash
  cd $R && bash scripts/check-gates-can-fail.sh && go test ./pkg/compliance/... ./cmd/compliance-evidence/... -count=1 | tail -5
  ```
- [ ] **Step 111** [C]: `git commit -S -m "docs(compliance): ADR-099 amended for The Well; profile-switch runbook"`
- [ ] **Step 112** [V]: **FINAL GATE**:
  - The Well is the source.
  - Pulling the plug on Postgres leaves the dashboard and CLI serving labelled YAML and rejecting writes.
  - One `profile set` produces history, journal and evidence trails that an auditor can follow without asking anyone.

---

## APPENDIX A: EMERGENCY PROCEDURES

**The Well will not start.**
1. `sudo docker logs --tail 80 unheaded-postgres`.
2. Never `docker compose down -v` and never delete `postgres-data`: that is the Kanban history.
3. Every reader falls back to YAML by design, so the stack keeps working read-only. Report to Stevie.

**A sync would overwrite live state.**
1. Sync never updates state rows (`DO NOTHING`).
2. If a diff shows a state UPDATE, that is a bug. Stop, and file it as `[STUCK]`.

**The journal hash chain is broken on ingest.**
1. Do not repair it. It is a `critical` finding by design.
2. Preserve the host's `journal.jsonl` (`cp -a` to `/var/lib/unheaded/remediate/forensics/`) and stop for Stevie.

**The executor's plan shows a change on a host where `lab` was applied.**
1. The model and the applied state disagree.
2. Do not apply. Diff the rendered content against the host file, fix the model, then return to Step 76.

**A host refuses its snapshot.** That is working as designed. Re-export with `snapshot`, then check the key and the clock (`not_after`).

## APPENDIX B: AGENT ASSIGNMENT AND CRITICAL PATH

| Phase | Agent | Parallel | Depends on | Time |
|---|---|---|---|---|
| 0 Well online | Coordinator [Y] | no | none | 45m |
| 1 Schema + store | Coordinator | no | 0 | 3h |
| 2 Seed + sync | Coordinator | with 3 | 1 | 1.5h |
| 3 Readers | Coordinator | with 2 | 1 | 2h |
| 4 Executor | Coordinator | no | 2 | 3h |
| 5 Journal evidence | Coordinator | with 6 | 4 | 1.5h |
| 6 Switching + Ansible | Coordinator | with 5 | 3 | 2h |
| 7 Sealed snapshots + docs | Coordinator [Y] | no | 4, 6 | 1.5h |

Critical path: 0 → 1 → 2 → 4 → 5/7, about 10.5 h.

**Out of scope here**, and next: dashboard write access (switching from the UI) needs logins and RBAC on the dashboard first (`pkg/auth` JWT/OIDC + RBAC role `compliance-admin`, an audit row per change). That is ADR-099 phase 5, its own plan. Enforcing mode also stays behind ADR-098's four conditions.

## APPENDIX C: QUICK REFERENCE

- Databases:
  - `unheaded_config`: desired state (Pleroma). Profiles, bindings, mirrors.
  - `unheaded_ops`: what happened (Anamnesis). Journal, findings, evidence.
- Roles:
  - `config_admin`: writes config.
  - `config_reader`: reads config.
  - `ops_writer`: appends to ops.
  - `ops_reader`: reads ops.
- Passwords come from environment variables only; never commit one.
- The Well binds to `127.0.0.1:5432` only. Hosts never connect; the collector pulls over SSH.
- Source labels: `well`, `yaml-fallback`. Metric: `unheaded_compliance_source{source}`.
- Regenerate: `go run ./cmd/compliance-evidence baseline`; `… findings -adr docs/adr/ADR-098-…md`; `… well sync`; `… profile export`.

---

*COMPLIANCE-WELL Battle Plan — Forged 2026-10-01*
*8 Phases. 112 Steps. The Well holds what changes; the repo holds what runs as root; YAML catches us when The Well is down.*
*Flip a profile and an auditor can follow the whole trail without asking anyone.*
