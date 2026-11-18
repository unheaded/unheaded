<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# S-WASTE-OUT BATTLE PLAN: 24 Phases, 414 Steps

**Date**: 2026-11-15
**Sprint**: S-WASTE-OUT, plan 2 of the ADR-147 series (`docs/battle-plans/SPRINT-SERIES-2026-11-15-ADR-147.md`). What was built wrong or does nothing is removed, each fix measured before and after.
**Prerequisite**: ADR-147 "Stevie's answers to the decisions" (2026-11-15): 7, 8, 9, 10, 19, 24, timeguru, and the two fixes of decision 5. Eight design questions this plan found are open (table "Decisions", D1 to D8); the phases that need one wait for it, the rest run.
**Target**: On west: five containers fewer; Cuirass runs only against a real LXD; Wotan answers under 1 `GET /api/v1/topics/` a second (68 today) with at most one client per process; `alerts.critical` and service discovery each have a producer and a reader that do work; one topic per trace event; dashboard-backend holds no time series; Akira is throttled in under 1% of its periods (44% today); health checks write no info lines; `FLOW_STATE` cannot fill silently; timeguru and kanban have one home each. Last: Vector inserts in batches and Postgres has a budget for its data, both measured, handed to S-LOG-WEEK-2. Every row of the findings table in the progress file has a before, an after and a status.
**Estimated duration**: 55 to 70 h of work over 10 to 14 sessions, plus three 24 h waits for after-readings. Phases 0 to 16 are code and run before the evidence week ends. Phases 17 to 22 change west and wait for it (the E-GATE); Phases 19 and 22 are attended. East is not changed by this plan.
**Agent strategy**: Coordinator (Opus) for Phases 0, 1, 2, 17 to 23. Code lanes in their own worktrees, at most three building at once (Appendix B). Lanes merge into `sprint/s-waste-out` one at a time; nothing merges to `develop` before the E-GATE.
**Commit cadence**: every 5 steps, at every exit gate, before every skip. Signed (`-S`); if gpg-agent has timed out and Stevie is away, `--no-gpg-sign`, logged in the progress file for `scripts/resign-unpushed.py`. Named files only. Message `[PLAN S-WASTE-OUT] Steps X-Y: ...`, then the trailers the executing session's attribution reminder gives. Stevie pushes.
**Stuck protocol**: skip after 3x the step estimate or two failed debug attempts. Mark the step `[STUCK]`, its dependents `[BLOCKED]`, commit, go on to the next independent phase. Haiku and Sonnet agents never improvise past a stop line (series 2026-11-04 rule 12).

## Grounding (checked 2026-11-15 ~20:00Z against develop `f9b08c88`, read-only; east read over ssh. Re-checked ~21:30Z at `1393e5ee`: the one commit between is an ADR-113 document; every `file:line` below still holds)

Seat rows are cited as seat and row id (reports in `~/tmp/adr147-table/` on west). "Read" means measured again by the forger.

| Row | Seat or ADR said | Found | Plan does |
|---|---|---|---|
| Developer D1, D2; BlackMage F4 | A stream is a 500 ms poll unless gRPC is used; one gRPC failure polls for ever; health says stream | True. `pkg/wotan-client/client.go:776-826` (`StreamMessages`), `:834-861` (`relayGRPC` ends in `pollMessagesFrom`), `:925-984`; `reprobe.go:49-52` says "Streams already polling over HTTP keep polling"; `stream_health.go:76,107` reports the client, not the stream. Read: 68.8 GET/s, 17 gRPC streams, 63 approved members, 25 rooms | Phase 6 |
| Developer D5 | Three stream clients, different recovery | True. `TopicStreamClient.pollHTTPFallback` (`topic_client.go:674-712`) returns to gRPC after a 30 s probe; `Client` never does | Phase 6: one loop |
| Developer D6 | Up to four clients per process | True. `pkg/discovery/wotan.go:77`, `pkg/healthreport/wotan.go:28` and `pkg/logagg/setup.go:53` each build their own; `transport.Config` already carries both Wotan addresses | Phase 7 |
| Developer D3 | Discovery fills a registry nobody reads | True: every caller drops `discovery.Start`'s result; no `Resolve` or `NewResolver` call outside `pkg/discovery` | Phase 7 (announce only), Phase 9 (the reader, D3) |
| Developer D4 | `alerts.critical`: no publisher | True, and more: it is a signed prefix (`pkg/wotansig/keys.go:40`, `configs/wotan.yaml:113`, `keys: []`), so a publisher needs an ML-DSA-65 key enrolled: custody. After the removals the only running subscriber is timeguru; ten other packages under `services/` subscribe and are not run. alert-mailer, which holds every critical alert, has no Wotan client and sits on the `monitoring` network only | Phase 8 (D1, D2); the key is a `[STEVIE]` step |
| Developer D7; Archimedes F6 | Every trace event published twice | True for flow and latency: one payload queued on two topics (`cmd/trace-collector-go/publisher.go:240-252`). Packet events are two shapes (`:209-222`); `main.go:1181` is a third path. dashboard-backend reads both names, with two clients | Phase 10 |
| ADR-147 scope | Scrape jobs of the five "in both scrape lists" | Only `config/victoria/scrape.yml` (lines 49-98) has them. Prometheus has no job for any of the five. Cuirass's west target is in both | Phase 3 edits what exists |
| Decisions 9, 10 | "Prometheus keeps only what rule evaluation needs (hours, not 30 days)" | **Contradiction.** Nine rules read 28 or 30 days (`rules/slo.yml` 8 times `[28d]`, `rules/detection.yml` once `[30d]`). VictoriaMetrics offers no remote read to Prometheus, and vmalert is ruled out (no new third-party tool). Hours of retention would empty those nine rules. Read: 13,730 head series | D4; Phase 12 |
| Developer D11, D12 | dashboard-backend keeps a metrics store; three health pollers | True. The store has 14 call sites in `internal/server/server.go`. `pkg/sloguard/sloguard.go:177` already queries VictoriaMetrics over HTTP | Phase 13 |
| Developer D13, D15 | The page polls on 401; health checks make the log lines | True, and they are one defect: every refused request is one audit line on stderr (`pkg/auth/audit.go:147-155`). Read (`unheaded_log_lines_total`, 1 h): west writes 241 lines a minute; `unheaded-dashboard` 115 (48%), `unheaded-sophia` 58, `unheaded-kanban` 28, `unheaded-cuirass` 10. kanban logs every request at info (`cmd/kanban-app/main.go:440-457`). The audit line is right; the polling is the fault | Phase 13 (page), Phase 14 (kanban) |
| Developer D8, D9, D10 | Akira: interpreter every 30 s, zombies, verdicts refused | True. The interval is one config line (`config/akira/akira.yaml:22`); a per-target interval is honoured (`pkg/health/akira.go:198`). No `init` in its compose block. No `HEALTH_SIGNING_KEY_FILE` | Phase 14; key `[STEVIE]` |
| Developer D16; Erda F3 | Each health report is a durable transaction | True: `pkg/database/bridge.go:62-118` | Phase 14 |
| Archimedes F4 | The health probe costs more than a small service | `cmd/healthprobe` is a Go binary doing a TLS 1.3 hybrid handshake; compose runs it every 10 s on every first-party service | Phase 14 |
| Developer D17 | Cuirass on west reconciles a mock | True: the mock is the default when `LXD_BACKEND` is unset (`cmd/unheaded-daemon/main.go:229,414`). A real-LXD overlay exists (`docker/compose.cuirass-lxd.yml`); west's LXD is active | Phase 5 |
| BlackMage F7 | `FLOW_STATE`: plain hash, insert failure ignored, "no reader found" | True, and settled: **nothing reads it, in userspace or in the program.** `GetFlowStateMap` reads flow-tracker's `FLOWS` (`cmd/trace-collector-go/loader_native.go:207-208`); packet-marker touches the map only in `update_flow_state` (`ebpf/packet-marker/src/main.rs:316-342`). West's installed `packet-marker` and `flow-tracker` objects are of 2026-10-18; the tree has moved since | D7; Phase 15; ship together (Phase 19) |
| BlackMage F8 | `FLOW_EVENTS` not drained, 94% dropped | True: no reader in the tree. 2,340 sent is the ring filled once (256 KiB, 112 bytes a record) | D6; Phase 15 |
| Developer D22, 1.6 | Ten Go containers without `GOMEMLIMIT`; GOMAXPROCS unknown | `GOMEMLIMIT` only at `docker-compose.yml:773,1330,1659`. `go.mod` says `go 1.26.0`: since Go 1.25 the runtime sets default GOMAXPROCS from the cgroup's CPU limit, rounded up. So 12 is not expected in a limited container. Not measured: no service exports it (`pkg/metrics/gocollector.go` reads no `/sched/gomaxprocs` key). After the removals six first-party containers have a limit and no `GOMEMLIMIT`: timeguru, akira, kanban-app, cuirass, auth, sword; `check-unit-ceilings.sh` reads units only | Phase 14 exports GOMAXPROCS, GOMEMLIMIT and GOGC, and sets `GOMEMLIMIT` at 85% of each limit with a gate on compose; a GOMAXPROCS value is set only if the reading calls for one (plan 9) |
| Erda F9 | timeguru: SQLite on tmpfs; the Well's table is empty | True, and the table does not fit: timeguru's statuses are `pending, in_progress, completed, blocked`, the table's CHECK allows `planned, in-progress, completed, deferred`; ids are strings against `BIGSERIAL`; `milestone_history` is written and never read | Phase 16: a migration, then the Well |
| Erda F10 | kanban falls back to a stale SQLite file | True: `cmd/kanban-app/main.go:186-204`. `modernc.org/sqlite` stays in `go.mod` (three test files use it) | Phase 16 |
| Erda F1 | Vector's ClickHouse sink has no `batch` block | True: `config/vector/vector.yaml:431-455` | Phase 21, after the E-GATE |
| Erda F2; Archimedes F3 | Postgres: 256 MiB for 1.5 GB | True: `docker-compose.yml:464`; `db/postgresql.conf:8,29` | D8; Phase 22, after the E-GATE |
| Erda F12 | Vector's own series carry the container id as `host`; VictoriaMetrics has no series limit | `vector_metrics` is a bare `internal_metrics` source (`vector.yaml:34-35`); no `-storage.max*Series` flag on `victoria` | Phase 21 |
| Erda F16 | Orphan volume, three ClickHouse tables, three empty volumes | Not read again (it means Docker's data or a SQL session). Re-read at execution, after the E-GATE | Phase 22 |
| Archimedes F7 | Room count times room size allows 78 GiB | Bounded already: `-buffer-bytes-total` 64 MiB (`services/wotan/cmd/wotan/main.go:485`). No fix; closed with this reason | Row closed in the progress file |
| BlackMage F5, F11 | Wotan's rate limit is keyed on a shared address; a failed audit write is dropped uncounted | True: `services/wotan/internal/middleware/ratelimit.go:80-85`; `pkg/auth/audit.go:101` | Phase 11 |
| Decision 17 | East's role services | East runs `unheaded-architect`, `unheaded-captain`, `unheaded-micromanager` as host units (read 2026-11-15). No monad or sophia unit there | Handed to plan 6 (Phase 23) |
| (found) | | `scripts/check-gates-can-fail.sh:717` uses `deploy/systemd/unheaded-sophia.service` as its test subject | Phase 3 repoints it |
| (found) | | West's host-local `docker-compose.override.yml` (gitignored, merged into every compose run) names all five (read 21:30Z: entries for akira, architect, captain, clickhouse, coredns, cuirass, dashboard-backend, grafana, kanban-app, micromanager, monad, postgres, sophia, timeguru, traefik, vector, victoria, wotan); an entry for a service that no longer exists makes every compose command fail | Asked in Phase 17 (Step 306), edited in Phase 19 just before `up` |
| (found) | | The compose `data` network is `172.28.1.0/24` (`docker-compose.yml:23-27`); Akira sits on `data` only. Phase 9's draft subnet `172.28.0.0/16` was wider than the network | Step 154 corrected |
| (found) | | `pkg/wotan-client/stream_health.go` registers through `pkg/metrics/auto` with `prom` options; no registry is passed | Steps 126 and 150 corrected |
| (found) | | `configs/wotan.yaml` is bind-mounted into Wotan from the repository (`docker-compose.yml:783`); west has no host-local copy of the topic config to enrol a public key in | Steps 138 and 317: where a key goes is Stevie's choice |
| (found) | | `huginn_pg_commits_total` exists in VictoriaMetrics; it carries Huginn's own polls too, so the health upsert's exact rate needs `pg_stat_statements` after the E-GATE | Step 13 (approximate), Step 309 (exact) |
| (found) | | Wotan publishes gRPC 18001 on `127.0.0.1` only (`docker-compose.yml:775`); alert-mailer, on the `monitoring` network, cannot reach it. 18000 already has an overlay bind for Prometheus. Host units reach `localhost:18001`, the `pkg/transport` default | Step 131 adds the overlay bind for 18001 |
| (found) | | Wotan's 429s are visible: `increase(unheaded_http_requests_total{service="wotan",status="429"}[7d])` = 367 at 21:20Z | W-07's before (Step 8) |
| (found) | | timeguru and postgres share the `data` network; dashboard-backend and victoria share `observe`; kanban's `DATA_DIR` and volume `kanban-data` exist only for `kanban.db` | Phases 13, 16, 22 |
| (found) | | Gates that run a container (`check-prometheus-rules.sh`, `test-vector-config.sh`, promtool, `scripts/well/test-*.sh`) write Docker's layer on `/var` | Before the E-GATE only the promtool and Vector checks run (seconds); throwaway-Postgres tests wait for Step 321 |

**Marshal (scope).** IN: the twelve items of the plan's brief, as the table above found them. OUT, with a home: alerts, SLOs, latency buckets, backups (plan 1); Zhen's separation, including Akira's and the dashboard's Zhen targets (plan 3); disks, swap, pid limits, the input filter (plan 4); every number for the shares, CPU quotas on host units, right-sizing limits to peaks (plan 8). PARKED, each to a `Proposed` ADR if Stevie wants it: reading `FLOW_EVENTS` and `LATENCY_EVENTS` instead of sweeping maps (ADR-144, third option); Wotan's 1 s room re-list per pattern stream (`topic_service.go:507`); the trace collector's 50 ms flush tickers (Developer D24); the dashboard's snapshots pushed on `/ws` (rule 22: no page work); `node_systemd_unit_state` (Archimedes F10); the ten unrun packages under `services/` that still carry their own alert listener; ClickHouse's cache and pool settings against its container (Erda F13, to plan 9); the connection budget per role (Erda F6, F7). Ruling: no new module, crate, image or tool. `init: true` is Docker's own. Nothing here touches Jenkins (removed 2026-11-15).

**Micromanager (QA / DoD).** Code is red then green under `go test -race`. Every code phase closes on `gofmt -l`, `go vet`, `go test -race` for the packages it touched and `./scripts/check-gosec-ratchet.sh`. eBPF passes `check-ebpf-loads.sh` (root) and `check-ebpf-insns.sh`. Rules pass `check-prometheus-rules.sh`; Vector passes `test-vector-config.sh`; the Well passes `scripts/well/test-config.sh` and `test-migrate.sh`. A new gate is registered in `check-gates-can-fail.sh` and shown to fail. Done is a row in the progress file with a before and an after measured the same way, not a green test. A fix whose after is worse than its before is rolled back and its row says so.

**Busboy (handoffs).**
- **The evidence week** (S-E1-LIVE Step 284, about 2026-11-17 18:34Z). Until the E-GATE (Step 305): no merge to `develop`, no image build, no container or unit restart, no SQL session on the Well or ClickHouse, nothing written under `/var` (scratch is on the NVMe). Removing a container changes the week's log volume, so that waits too. The one exception: the promtool and Vector config checks run a throwaway container for seconds; throwaway-Postgres tests wait for Step 321.
- **Plan 1** adds the rule for a stopped container: it must not expect the five, nor Cuirass on west. It also edits `cmd/alert-mailer` (dry-run off) and `pkg/httputil/middleware.go` (buckets): this plan adds a new file beside the second and touches the first in Phase 8. Whoever merges second merges `develop` in first; a conflict there is `[STUCK]`.
- **Plan 3** edits the same lists (Akira's default targets, `config/dashboard/services.txt`, `configs/wotan.yaml`): keep both sides.
- **Plan 4** moves the database volumes and stops the stack. If it runs between a before and an after of Phases 21 and 22, the before is taken again on the new disk.
- **Plan 5** measures what Phases 21 and 22 change. **Plan 6** removes east's three role units and installs east's Akira config. **Plan 7** starts Cuirass against the real LXD.
- This plan changes `runbooks/` bytes: `scripts/runbook-manifest.sh --write` only; the seal is Stevie's.

## Decisions

Decided here (the brief asked for it): **the removed code goes to `~/tmp/projects/unheaded-role-services`**, one sibling repository with history for all five services, made by the S-MOVE-OUT method the WAF used on 2026-11-10 (`filter-branch` on a fresh clone; no remote; Stevie publishes it or not).

Open. Asked at Step 18, four at a time. "Away" is what happens if Stevie is not there.

| # | Question | Options | Recommended | Away |
|---|---|---|---|---|
| D1 | Who publishes `alerts.critical`? Under "build it as if we have a real production target" its job is: every service learns, on the bus, that a critical alert is firing | (A) alert-mailer, for each Alertmanager notification with `severity="critical"`, firing and resolved, plus a snapshot every 5 min. (B) Akira, on a CRITICAL or PANIC band. (C) both | **A**: one meaning of "critical", the one a person is mailed about; a second Alertmanager on east (plan 6) feeds the same topic | Phase 8 Steps 131-140 `[BLOCKED]` |
| D2 | What does a subscriber do with it? | (A) Record: the active set, a gauge, a `/health` detail, one warn line per change. (B) A, and change is held: Sword starts no deploy, chaos-controller starts no experiment, zhen-agentd starts no unattended run while one fires. (C) A, and each service sheds optional work | **B**: it is what a production change freeze is. If the publisher is silent for 15 min the state is `unknown`: allow, with a warn line | A is built; B waits |
| D3 | Who reads service discovery? Its job: a new host or instance becomes known without editing a file on another host | (A) Akira: it also checks every admitted announcement that its config does not list. (B) dashboard-backend's one health poller. (C) both | **A**: the arbiter then covers east's services (plan 6) with no file edit. Announcements are admitted only from Wotan's verified sender, inside listed subnets, at most 256 | Phase 9 `[BLOCKED]`; discovery stays announce-only |
| D4 | Prometheus (the contradiction above) | (A) Keep 30 days, of only the series a rule reads: a generated keep-list, and a gate that fails when a rule reads a series outside it. (B) Hours of retention, and the nine long rules are rewritten (they are plan 1's) | **A** | Phase 12 `[BLOCKED]` |
| D5 | Akira's certificate check runs every 30 s so a dev certificate "flips within a minute" | (A) `interval: 1h`, as the hosts run it. (B) keep 30 s, parse the certificates in Go | **A**: one line, no new code | A |
| D6 | `FLOW_EVENTS`, the ring nobody drains | (A) flow-tracker stops writing it and the ring goes; `FLOWS` and its batch reader already carry the state. (B) drain it (ADR-144's parked reader) | **A** | Step 269 and the flow-tracker halves of Steps 265 and 271 `[BLOCKED]` |
| D7 | `FLOW_STATE`: decision 19 said LRU. Nothing reads it | (A) LRU with a counted insert, as decided. (B) delete the map (9.96 MB locked per host, one map write per packet) | A stands unless Stevie says B | A |
| D8 | Postgres's budget for the fair week | (A) 2 GiB and 1.0 CPU, ClickHouse's limits; `shared_buffers` 512MB, `effective_cache_size` 1536MB. (B) 1 GiB, 0.5 CPU (Archimedes' floor); 256MB, 768MB | **A**: equal limits are what makes the week fair | Phase 22 Steps 384-393 `[BLOCKED]` |

Default without a question (written under "For Stevie"): health probes every 30 s with 3 retries, so a dead service is `unhealthy` after 90 s where it is 50 s today.

---

## LEGEND

- `[B]`: bash command, run directly
- `[V]`: verification; must pass before going on
- `[D]`: debug step, only if the step before it failed
- `[W]`: write or change a file
- `[R]`: read or inspect
- `[S]`: needs sudo (always Opus)
- `[P]`: can run beside other `[P]` phases
- `[C]`: commit checkpoint (signed, named files)
- `[STEVIE]`: Stevie runs it or says yes first. `[STEVIE][DECIDE]`: he chooses
- `[STUCK]` / `[BLOCKED]`: set by the Skip Protocol

`~Nm` is the expected wall-clock time. Paths, written out in every command:
`REPO=/home/govan/tmp/unheaded`; `WT=/home/govan/tmp/wt/S-WASTE-OUT` (branch `sprint/s-waste-out`); lane worktrees `/home/govan/tmp/wt/S-WASTE-OUT-<lane>`; `S=/home/govan/tmp/wt/S-WASTE-OUT-scratch` (NVMe; never `/var/tmp` or `/tmp` before the E-GATE); `PROG=docs/battle-plans/progress/S-WASTE-OUT-progress.md` in whichever tree is being committed; `LIB=/home/govan/tmp/wt/S-WASTE-OUT-scratch/lib.sh` (Step 3; shell functions do not survive between commands, so each block sources it).

**GO-GATE `<dirs>`** means, with `<dirs>` as `./pkg/x/... ./cmd/y/...`:
```bash
cd /home/govan/tmp/wt/S-WASTE-OUT && test -z "$(gofmt -l $(echo <dirs> | sed 's#/\.\.\.##g'))" && go vet <dirs> && GOMAXPROCS=4 go test -race -count=1 <dirs> 2>&1 | tail -8
```
Expected: no `FAIL`, every line `ok` or `no test files`. In a lane, the same from the lane's worktree.

---

## PHASE 0: PRECONDITIONS AND THE "BEFORE" READINGS (Steps 1-17)

**Goal**: Branch, progress file and every before that can be read without touching Vector, ClickHouse, Postgres or `/var`.
**Prerequisite**: west up with the stack; VictoriaMetrics on `127.0.0.1:8428`.
**Time**: 60 min.
**Model**: Opus. **Agent**: Coordinator.

- [ ] **Step 1** [R] ~20m: Read ADR-147 from "The table's answers" to the end; both series files; this plan's Grounding and Decisions; `/home/govan/tmp/adr147-table/developer.md` (all); `erda.md` rows F1, F2, F3, F9, F10, F12, F16 and section 4; `blackmage.md` F4, F5, F7, F8, F11; `archimedes.md` F4, F6, F7 and section 1.5; S-WATCH-TRUE Phase 1 and "For Phase 4" in its progress file.
- [ ] **Step 2** [B] ~1m: Worktree and scratch.
  ```bash
  cd /home/govan/tmp/unheaded && git switch develop && git worktree add /home/govan/tmp/wt/S-WASTE-OUT -b sprint/s-waste-out develop && ln -sfn /home/govan/tmp/projects /home/govan/tmp/wt/projects && mkdir -p /home/govan/tmp/wt/S-WASTE-OUT-scratch && git -C /home/govan/tmp/wt/S-WASTE-OUT log --oneline -1
  ```
- [ ] **Step 3** [W] ~3m: `/home/govan/tmp/wt/S-WASTE-OUT-scratch/lib.sh`, exactly:
  ```bash
  # q PROMQL: one line per series from VictoriaMetrics: labels, then the value.
  q(){ curl -s -m5 -G http://127.0.0.1:8428/api/v1/query --data-urlencode "query=$1" | python3 -c 'import json,sys
  for r in json.load(sys.stdin)["data"]["result"]: print(" ".join(k+"="+v for k,v in sorted(r["metric"].items()) if k!="__name__"), r["value"][1])'; }
  # cg CONTAINER: CPU, throttling, memory and limit hits from the cgroup files.
  cg(){ local id d; id=$(sudo docker inspect -f '{{.Id}}' "$1") || return 1; d=/sys/fs/cgroup/system.slice/docker-$id.scope; echo "== $1 $(date -u +%FT%TZ)"; grep -E '^(usage_usec|nr_periods|nr_throttled|throttled_usec) ' $d/cpu.stat; echo "memory.current $(cat $d/memory.current)"; echo "memory.peak $(cat $d/memory.peak)"; grep -E '^(max|oom_kill) ' $d/memory.events; }
  ```
- [ ] **Step 4** [W] ~10m: Create `/home/govan/tmp/wt/S-WASTE-OUT/docs/battle-plans/progress/S-WASTE-OUT-progress.md`: SPDX header as `S-WATCH-TRUE-progress.md` has; `## Decisions` (the D1-D8 table, empty rulings); `## Findings` with the columns `id | seat and row | evidence | fix | owner | before | after | status` and the 31 rows of Appendix C's findings table (id, seat and row, evidence, fix and owner filled; before, after and status empty; W-27's status `closed: bounded already`); `## Log`; `## Handoffs`; `## For Stevie`.
- [ ] **Step 5** [V] ~1m: `grep -c '^| W-' /home/govan/tmp/wt/S-WASTE-OUT/docs/battle-plans/progress/S-WASTE-OUT-progress.md` prints `31`. Other → fix the table.
- [ ] **Step 6** [B] ~1m: Tools (never install a missing one).
  ```bash
  for t in go cargo docker bpftool python3 curl git; do command -v $t >/dev/null && echo "$t ok" || echo "$t MISSING"; done; go version; docker compose version | head -1
  ```
  Expected: all ok; Go 1.27.x; Compose v5.x. `MISSING` → `[STUCK]` for the phases that need it.
- [ ] **Step 7** [B] ~1m: The E-GATE's state.
  ```bash
  grep -n 'Step 284' /home/govan/tmp/unheaded/docs/battle-plans/progress/S-E1-LIVE-progress.md | tail -3; date -u +%FT%TZ
  ```
  Log in `PROG`: "E-GATE open" unless a Step 284 entry says the week was read. The date alone never closes it.
- [ ] **Step 8** [B] ~2m: Wotan, before.
  ```bash
  . /home/govan/tmp/wt/S-WASTE-OUT-scratch/lib.sh
  q 'sum(rate(unheaded_http_requests_total{service="wotan",method="GET"}[30m]))'
  q 'sum(rate(unheaded_http_requests_total{service="wotan",method="POST"}[30m]))'
  q 'unheaded_chat_members_approved'; q 'unheaded_chat_rooms_total'; q 'unheaded_chat_streams_active'
  q 'sum(unheaded_chat_room_buffer_bytes)'; q 'unheaded_chat_room_buffer_bytes' | grep -E 'traces\.|ebpf\.'
  q 'process_resident_memory_bytes{job="wotan"}'
  q 'sum(increase(unheaded_http_requests_total{service="wotan",status="429"}[7d]))'
  q 'unheaded_chat_room_buffer_bytes' | grep -E 'alerts\.critical|cuirass\.' ; echo "alerts.critical/cuirass rooms listed above (none = no room)"
  ```
  Reference (2026-11-15): 68.8 GET/s, 3.2 POST/s, 63 members, 25 rooms, 17 streams, six trace rooms 39.3 MB of 52.8 MB, 367 answers 429 in 7 days, no `alerts.critical` room, `cuirass.drift` and `cuirass.metrics` rooms present.
- [ ] **Step 9** [B] ~2m: The services, before.
  ```bash
  . /home/govan/tmp/wt/S-WASTE-OUT-scratch/lib.sh
  q 'process_resident_memory_bytes{job=~"architect|captain|micromanager|monad|sophia|unheaded-daemon|timeguru|akira|kanban-app"}'
  q 'sum by (job) (rate(unheaded_http_requests_total{path=~"/health|/metrics|/ready|/live"}[1h]))'
  q 'sum by (unit) (rate(unheaded_log_lines_total{host="west"}[1h])) * 60'
  q 'sum(rate(unheaded_log_lines_total{host="west"}[1h])) * 60'
  ```
  Reference: 15 MiB each; 0.97 health and scrape requests a second per role service; lines a minute: all 241, dashboard 115, sophia 58, kanban 28, cuirass 10.
- [ ] **Step 10** [B] ~2m: dashboard-backend and Prometheus, before.
  ```bash
  . /home/govan/tmp/wt/S-WASTE-OUT-scratch/lib.sh
  q 'process_resident_memory_bytes{job="dashboard-backend"}'; q 'rate(go_memstats_alloc_bytes_total{job="dashboard-backend"}[6h])'; q 'rate(process_cpu_seconds_total{job="dashboard-backend"}[6h])'
  q 'sum by (status) (increase(unheaded_http_requests_total{service="dashboard"}[7d]))'
  q 'prometheus_tsdb_head_series'; q 'prometheus_tsdb_storage_blocks_bytes'; q 'process_resident_memory_bytes{job="prometheus"}'
  q 'akira_signed_publish_refused_total'
  ```
  Reference: west's dashboard 289 MiB, 2.6% of a core; 98% of its requests 401; Prometheus 13,730 series, 801 MB, 137 MiB; Akira 49 and 12 refused. Two `dashboard-backend` series means west and east: record west's (`instance` holds `::1` or `host.docker.internal`).
- [ ] **Step 11** [S][B] ~3m: cgroup counters, before (files under `/sys`, not `/var`).
  ```bash
  . /home/govan/tmp/wt/S-WASTE-OUT-scratch/lib.sh
  for c in unheaded-akira unheaded-dashboard unheaded-wotan unheaded-kanban unheaded-timeguru unheaded-postgres unheaded-vector unheaded-clickhouse unheaded-prometheus; do cg $c; done
  sudo docker top unheaded-akira -eo pid,stat,comm | awk '$2 ~ /^Z/' | wc -l
  sudo docker ps --format '{{.Names}}' | wc -l
  ```
  Reference: akira throttled 44% of periods, `max` 25,937, 18 zombies; postgres `max` 52,740; 23 containers.
- [ ] **Step 12** [S][B] ~3m: eBPF maps, before.
  ```bash
  . /home/govan/tmp/wt/S-WASTE-OUT-scratch/lib.sh
  q 'trace_collector_flow_map_entries'
  sudo bpftool map show name FLOW_STATE
  sudo bpftool -j map dump name FLOW_STATE | python3 -c 'import json,sys; print("FLOW_STATE entries", len(json.load(sys.stdin)))'
  sudo bpftool -j map dump name FLOW_STATS | python3 -c 'import json,sys; print([ (e["key"], e["value"]) for e in json.load(sys.stdin)][6:9])'
  ls -la /usr/local/lib/unheaded/ebpf/
  ```
  Reference: `FLOW_STATE` type hash, max 65,536, 18,067 entries and rising 1,564 an hour (full on west from about 2026-11-16 23:40Z); `FLOW_STATS` keys 7 and 8: 2,340 sent, 35,416 dropped.
- [ ] **Step 13** [B] ~2m: Postgres and Vector, from series only (no SQL before the E-GATE).
  ```bash
  . /home/govan/tmp/wt/S-WASTE-OUT-scratch/lib.sh
  q 'huginn_pg_cache_hit_ratio'; q 'rate(vector_component_sent_events_total{component_id="clickhouse"}[6h])'
  q 'rate(huginn_pg_commits_total[6h])'
  q 'sum(rate(unheaded_http_requests_total{path="/health"}[1h]))'
  curl -s -m5 -G http://127.0.0.1:8428/api/v1/label/__name__/values --data-urlencode 'match[]={job="vector",component_id="clickhouse"}' | python3 -c 'import json,sys; print(" ".join(json.load(sys.stdin)["data"]))'
  ```
  Reference: cache hit 0.683 on `well-ops`; 2.73 rows a second; commits 2.06 a second on `db="well-ops"` (two series each: one with `job="huginn"`, one without; read the `job="huginn"` one; Huginn's own polls included, so W-20's exact before is Step 309); 6.46 `/health` requests a second across all services (W-14, W-22; read 21:30Z). The `label/__name__` line lists the series Phase 21 reads; pick the request counter from it and log its name.
- [ ] **Step 14** [W] ~10m: Fill the `before` column of rows W-01 to W-26 in `PROG` from Steps 8-13, each with its reading time. Rows whose before is a code fact (W-02, W-04, W-05, W-08, W-21, W-22, W-25, W-26) take the command in Appendix C's "Before" column and its output. W-27 is closed with its reason. W-28 to W-31, and W-20's exact figure, get their before in Phase 17 (Steps 309-314), after the E-GATE.
- [ ] **Step 15** [V] ~2m: Each reading is within a factor of 2 of its reference, or the row has a note saying what changed (a logged-in tab, a restart). A missing series → note the name; do not stop.
- [ ] **Step 16** [W] ~2m: `PROG`, "Log": which seat figures did not reproduce.
- [ ] **Step 17** [C][V] ~1m: **PHASE 0 EXIT GATE**: `PROG` has 26 rows with a before (W-01 to W-26) and W-27 closed.
  ```bash
  cd /home/govan/tmp/wt/S-WASTE-OUT && git add docs/battle-plans/progress/S-WASTE-OUT-progress.md && git commit -S -m "[PLAN S-WASTE-OUT] Steps 1-17: the before readings"
  ```

---

## PHASE 1: THE EIGHT QUESTIONS (Steps 18-21)

**Goal**: D1 to D8 answered, or marked away.
**Prerequisite**: Phase 0 gate.
**Time**: 15 min.
**Model**: Opus. **Agent**: Coordinator.

- [ ] **Step 18** [STEVIE][DECIDE] ~8m: If Stevie is present: two AskUserQuestion rounds, D1-D4 then D5-D8, options as in the Decisions table, the recommended one first and marked. If he is away: nothing is asked.
- [ ] **Step 19** [W] ~3m: Copy each ruling verbatim into `PROG` "Decisions". Away → write "away" and the table's Away cell; list the eight under "For Stevie", oldest first.
- [ ] **Step 20** [W] ~2m: In `PROG` "Log", list the steps now `[BLOCKED]` (from the Away column).
- [ ] **Step 21** [C][V] ~1m: **PHASE 1 EXIT GATE**: eight rows have a ruling or "away". Commit `PROG`.

---

## PHASE 2: THE ARCHIVE (Steps 22-32)

**Goal**: The five services live, with their history, in `~/tmp/projects/unheaded-role-services`.
**Prerequisite**: Phase 0 gate. Independent of Phase 1.
**Time**: 40 min.
**Model**: Opus (history rewrite on a clone; series 2026-11-04 rule 13). **Agent**: Coordinator.

- [ ] **Step 22** [W] ~3m: `/home/govan/tmp/wt/S-WASTE-OUT-scratch/role-paths.txt`, one path a line:
  ```
  services/architect
  services/captain
  services/micromanager
  services/monad
  services/sophia
  cmd/monad
  cmd/sophia
  deploy/systemd/unheaded-architect.service
  deploy/systemd/unheaded-captain.service
  deploy/systemd/unheaded-micromanager.service
  deploy/systemd/unheaded-monad.service
  deploy/systemd/unheaded-sophia.service
  nix/containers/architect.nix
  nix/containers/architect-service.nix
  nix/containers/captain.nix
  nix/containers/captain-service.nix
  nix/containers/micromanager.nix
  nix/containers/micromanager-service.nix
  nix/containers/monad-service.nix
  nix/containers/sophia-service.nix
  nixos/modules/services/architect.nix
  nixos/modules/services/captain.nix
  nixos/modules/services/micromanager.nix
  nixos/modules/services/monad.nix
  nixos/modules/services/sophia.nix
  containers/lxd/instances/architect.yaml
  containers/lxd/instances/captain.yaml
  containers/lxd/instances/micromanager.yaml
  containers/lxd/instances/monad.yaml
  containers/lxd/instances/sophia.yaml
  containers/containerd/services/architect.json
  containers/containerd/services/captain.json
  containers/containerd/services/micromanager.json
  containers/containerd/services/monad.json
  containers/containerd/services/sophia.json
  docker/services/monad
  docker/services/sophia
  docker/nginx/nginx-monad.conf
  docker/nginx/nginx-sophia.conf
  man/monad.1
  man/sophia.1
  wiki/Service-Architect.md
  wiki/Service-Captain.md
  wiki/Service-Micromanager.md
  ```
  Not on the list, and they stay: `pkg/monad`, `pkg/sophia`, `ebpf/monad-*`, `crates/monad-mbc`, `deploy/sophia-eye`, `wiki/Sophia-Dictionaries.md` (the wire format, the dictionaries, the maps).
- [ ] **Step 23** [V] ~1m: Every path is tracked.
  ```bash
  cd /home/govan/tmp/unheaded && while read -r p; do git ls-files -- "$p" | grep -q . || echo "MISSING $p"; done < /home/govan/tmp/wt/S-WASTE-OUT-scratch/role-paths.txt; git ls-files -- $(tr '\n' ' ' < /home/govan/tmp/wt/S-WASTE-OUT-scratch/role-paths.txt) | wc -l
  ```
  Expected: no `MISSING`; a file count (log it as N). A `MISSING` line → delete that line from the list, log it, rerun.
- [ ] **Step 24** [R] ~5m: Nothing else imports them: `cd /home/govan/tmp/unheaded && for s in architect captain micromanager monad sophia; do grep -rln "\"unheaded/services/$s[\"/]" --include=*.go . | grep -v "^./services/$s/\|^./cmd/$s/"; done`. Expected: no output. Any line → `[STUCK]`: that importer is not in this plan.
- [ ] **Step 25** [B] ~4m: Clone and filter (`develop` only; the clone has no link back).
  ```bash
  set -euo pipefail
  test ! -e /home/govan/tmp/projects/unheaded-role-services
  cd /home/govan/tmp/projects && git clone -q --no-local --single-branch --branch develop /home/govan/tmp/unheaded unheaded-role-services && cd unheaded-role-services && git remote remove origin
  [ "$(basename "$(git rev-parse --show-toplevel)")" = unheaded-role-services ]
  [ -z "$(git remote)" ]
  git rev-parse HEAD > /home/govan/tmp/wt/S-WASTE-OUT-scratch/role-services.source
  export FILTER_BRANCH_SQUELCH_WARNING=1
  P=$(tr '\n' ' ' < /home/govan/tmp/wt/S-WASTE-OUT-scratch/role-paths.txt)
  git filter-branch -f --prune-empty --index-filter "git read-tree --empty; git reset -q \$GIT_COMMIT -- $P 2>/dev/null; true" -- develop 2>&1 | tail -1
  rm -rf .git/refs/original && git reflog expire --expire=now --all && git gc -q --prune=now
  git branch -m develop main && git rev-list --count main && git ls-files | wc -l
  ```
- [ ] **Step 26** [V] ~1m: `Ref 'refs/heads/develop' was rewritten`; a commit count above 50; the file count equals N of Step 23. The two bracket lines are guards: the block stops unless it stands in the new clone.
  - The first `test` fails (the directory exists) → Step 27.
- [ ] **Step 27** [D] ~3m: `git -C /home/govan/tmp/projects/unheaded-role-services log --oneline -1` and `git -C ... remote`. A finished archive from an earlier run (no remote, file count N) → go to Step 28. Anything else → `[STUCK]`; never delete under `~/tmp/projects` without Stevie.
- [ ] **Step 28** [W] ~8m: `/home/govan/tmp/projects/unheaded-role-services/README.md`, terse: what it is (five services removed from Unheaded on ADR-147 decision 7); the source commit (the content of `role-services.source`); one row per service from Developer 1.2 (what it did, its callers, its state: architect, captain, micromanager held empty data directories; monad ran `echo`, `ping`, `transform`, `log`; sophia's inference loop had no body); what does not build here (every package imports `unheaded/pkg/...` of the source commit; `go build` works only inside that tree); what was never exercised; history is unsigned (`filter-branch` strips signatures). No taglines.
- [ ] **Step 29** [B] ~1m: `cd /home/govan/tmp/projects/unheaded-role-services && git add README.md && (git commit -q -S -m "README: what this archive is" || git commit -q --no-gpg-sign -m "README: what this archive is") && git log --oneline -1 && git status --short | wc -l`
- [ ] **Step 30** [V] ~1m: A commit; `0`.
- [ ] **Step 31** [W] ~2m: `PROG` "Log": archive path, its HEAD, the source commit, file and commit counts. "For Stevie": "`unheaded-role-services` has no remote; create one and push if you want it public."
- [ ] **Step 32** [C][V] ~1m: **PHASE 2 EXIT GATE**: the archive exists with no remote and N files. Commit `PROG`.

---

## PHASE 3: THE FIVE LEAVE THE TREE (Steps 33-58)

**Goal**: No file that builds, starts, scrapes, probes or allows architect, captain, micromanager, the monad service or the sophia service.
**Prerequisite**: Phase 2 gate.
**Time**: 4 h.
**Model**: Opus (build files, compose, auth lists, a gate's test subject). **Agent**: Coordinator or one agent in `WT`.

- [ ] **Step 33** [B] ~1m: Delete the listed paths.
  ```bash
  cd /home/govan/tmp/wt/S-WASTE-OUT && git rm -r -q $(tr '\n' ' ' < /home/govan/tmp/wt/S-WASTE-OUT-scratch/role-paths.txt) && git status --short | grep -c '^D'
  ```
  Expected: N of Step 23.
- [ ] **Step 34** [W] ~15m: `docker-compose.yml`. Delete: the `captain:`, `architect:` and `micromanager:` service blocks with the comment lines above each (from the comment above `captain:` to the line before the comment above `akira:`); the `monad:` and `sophia:` blocks (from the comment above `monad:` to the line before the comment above `dashboard-backend:`); volumes `captain-data:`, `architect-data:`, `micromanager-data:`; the five `svc_key_<name>:` entries under `secrets:`; in `akira` the five `tls-watch/<name>/<name>.pem` mounts; in every `AUTH_ALLOW_CALLERS` value the names `architect`, `captain`, `micromanager`, `monad`, `sophia` (wotan's list; timeguru's becomes `"kanban-app"`); any `depends_on` entry naming one of the five.
- [ ] **Step 35** [V] ~1m: `cd /home/govan/tmp/wt/S-WASTE-OUT && python3 -c 'import yaml; d=yaml.safe_load(open("docker-compose.yml")); s=d["services"]; print(len(s), sorted(set(s)&{"architect","captain","micromanager","monad","sophia"}), [k for k in d["volumes"] if k.split("-")[0] in ("captain","architect","micromanager")])'` prints `18 [] []`. Other → reread Step 34.
- [ ] **Step 36** [W] ~10m: Monitoring and health lists.
  - `config/victoria/scrape.yml`: delete the jobs `architect`, `captain`, `micromanager`, `monad`, `sophia` (each from `- job_name:` to the blank line before the next job).
  - `config/akira/akira.yaml`: delete the five target lines; in the header write "the five services" for "the ten services".
  - `configs/akira-east.yaml`: delete the targets `architect`, `captain`, `micromanager` (each `- name:` and its following lines). East installs it in plan 6.
  - `cmd/akira/main.go`: delete the five `defaultTargets` rows (`architect` to `sophia`). `cmd/akira/main_test.go:141-144`: the fixture `{Name: "monad", Port: 19004}` becomes `{Name: "kanban", Port: 20001}`, with the two `19004` and the two `monad` in the assertion changed to match.
  - `config/dashboard/services.txt`: delete the lines `captain=`, `architect=`, `micromanager=`, `monad=`, `sophia=`.
  - `cmd/dashboard-backend/internal/health/health.go`: delete the three rows at 885-887 and the five names from the `FirstParty` set at 900-901. `internal/scraper/scraper.go:933-943`: the same names. `internal/server/server.go:2205-2209`: the five rows.
  - `cmd/zhen-mcp/main.go:116-120`: the five `local(...)` rows.
  - `pkg/compliance/crosswalk/hostservice.go`: the five `HostServices` rows; its test's expected set.
- [ ] **Step 37** [W] ~10m: Registries.
  - `pkg/ports/ports.go`: delete `Architect`, `Captain`, `Micromanager`, `Monad`, `Sophia`, `NginxMonad`, `NginxSophia` and their registry rows; add where they stood: `// 19001-19005, 19080, 19081 retired 2026-11 (ADR-147 decision 7): architect, captain, micromanager, the monad and sophia services and their sidecars. Not reused.`
  - `pkg/uids/uids.go`: delete the same seven constants and map rows; add `// 16723-16727, 16750, 16751 retired 2026-11 (ADR-147 decision 7). Not reused.` in the style of line 205. Fix `uids_test.go`.
  - `deploy/sysusers.d/unheaded.conf`: delete lines 14-18.
  - `configs/wotan.yaml`: delete from `auto_approve` the entries `architect`, `captain`, `micromanager`, `monad`, `sophia`, `architect-service`, `micromanager-service`, `monad-service`, `sophia-service`; delete the `permissions` blocks `architect:`, `captain:`, `micromanager:`, `monad:`, `sophia:`.
  - `configs/services.yaml`, `configs/port-registry.yaml`, `config/logging/sources.yaml`: delete each entry whose key or `name` is one of the five (in `sources.yaml` only entries for the five services' containers or units; lines about the Monad protocol or Sophia dictionaries stay).
- [ ] **Step 38** [B] ~3m: First build. `cd /home/govan/tmp/wt/S-WASTE-OUT && go build ./... 2>&1 | head -30`
- [ ] **Step 39** [V] ~1m: No output.
  - `undefined: ports.Monad` and the like → Step 40.
- [ ] **Step 40** [D] ~20m: Each compile error names a file that used a deleted constant. Rule: a test or tool that dialled the service is deleted with it; a list of services loses the row; a client of the Sophia HTTP service (`cmd/upc-cartridgectl/registry.go:246,263`) loses that registry kind and keeps the file registry, with its flag refusing the old value by name. `services/gateway/config/config.go` loses the routes. Rebuild. Two rounds with errors left → `[STUCK]` with the list.
- [ ] **Step 41** [C] ~1m: **COMMIT** (stage what changed; nothing untracked is added).
  ```bash
  cd /home/govan/tmp/wt/S-WASTE-OUT && git status --short | grep '^??' ; git diff --name-only -z | xargs -0 git add -- && git commit -S -m "[PLAN S-WASTE-OUT] Steps 33-41: architect, captain, micromanager and the monad and sophia services leave compose, the lists and the registries"
  ```
  A `??` line is a new file: add it by name only if this phase made it.
- [ ] **Step 42** [W] ~15m: Build files.
  - `Dockerfile`: delete the stages `captain`, `architect`, `micromanager`, `monad`, `sophia` (each from its `FROM runtime AS <name>` to the line before the next `FROM`) and, in the `builder` stage, each `go build` line whose output is one of the five binaries.
  - `Makefile`: line 76, drop `build-captain build-architect build-micromanager build-monad build-sophia`; delete the five targets at 155-178; line 320, drop the five from `NIX_CONTAINERS`; line 410, drop the five `name:port` pairs. `ebpf-monad-cpu*`, `build-monad-mbc` and `test-rust-monad-mbc` stay.
  - `debian/control`: delete the stanzas `Package: unheaded-captain`, `-architect`, `-micromanager`, `-monad`, `-sophia`; drop the five from any `Depends:` or `Recommends:`. Then `grep -rln 'unheaded-\(captain\|architect\|micromanager\|monad\|sophia\)' debian/ packaging/ 2>/dev/null` and delete each maintainer file of the five.
  - `nix/Makefile`, `nix/tests/container_test.go`, `nix/containers/cuirass.nix`, `nix/containers/gateway.nix`, `nixos/modules/services/README.md`, `containers/lxd/instances/gateway.yaml`: drop the five from lists and routes.
  - `docker/docker-compose.loadbalancers.yml`: delete the services `nginx-monad` and `nginx-sophia`; `docker/haproxy/internal/haproxy.cfg` and `docker/haproxy/prometheus-haproxy-scrape.yml`: delete their backends and targets.
- [ ] **Step 43** [W] ~10m: Scripts.
  - `scripts/kingdom-startup.sh`: delete lines 98-102 and 114-118 (the five names, the five `name:port`).
  - `scripts/qa-smoke.sh` 75-76, 102-103, 110-111 and `scripts/wait-for-healthy.sh` 55-59: drop the five.
  - `scripts/check-gates-can-fail.sh:717`: `deploy/systemd/unheaded-sophia.service` → `deploy/systemd/unheaded-timeguru.service` (it carries the same `SystemCallFilter=~@privileged @resources @reboot @module @swap` line); line 1323: `unheaded-sophia.service` → `unheaded-timeguru.service`.
  - `scripts/check-version-flag.sh:18` and `pkg/cmdline/policy_test.go:30`: the comment's list becomes "(wotan, timeguru)".
  - `cmd/unheaded-cli/cmd/generate.go`, `cmd/unheaded-cli/cmd/observe.go`: drop the five from their service lists.
- [ ] **Step 44** [W] ~15m: `runbooks/`: in `deploy/apt-repo-publish.yaml`, `deploy/canary-deploy.yaml`, `deploy/full-stack-deploy.yaml`, `deploy/rolling-upgrade.yaml`, `infra/service-deploy.yaml`, `network/network-diagnostics.yaml`, `observe/prometheus-scrape-check.yaml`, `observe/service-health-sweep.yaml`: drop the five from every service, port and package list. Then:
  ```bash
  cd /home/govan/tmp/wt/S-WASTE-OUT && scripts/runbook-manifest.sh --write 2>&1 | tail -2 && scripts/check-runbooks.sh 2>&1 | tail -2
  ```
  Expected: the manifest is rewritten; `check-runbooks` passes. The seal is not touched: note under "For Stevie" that `runbooks/MANIFEST.sha256` needs his seal again.
- [ ] **Step 45** [W] ~5m: `config/grafana/provisioning/dashboards/kingdom/00_kingdom_services.json`: `grep -n -iE 'architect|captain|micromanager|monad|sophia' <file>`; in each of the four hits drop the five names from the `job=~"..."` or `service=~"..."` alternation. A panel that showed only those five is deleted. `monitoring/prometheus/rules/tests/wotan_test.yml`: a removed name used as a `caller` fixture becomes `timeguru`.
- [ ] **Step 46** [B] ~5m: Build and the touched packages.
  ```bash
  cd /home/govan/tmp/wt/S-WASTE-OUT && go build ./... && go vet ./... 2>&1 | tail -5 && GOMAXPROCS=4 go test -race -count=1 ./pkg/ports/... ./pkg/uids/... ./pkg/compliance/... ./pkg/cmdline/... ./pkg/discovery/... ./pkg/healthreport/... ./cmd/akira/... ./cmd/zhen-mcp/... ./cmd/dashboard-backend/... ./cmd/upc-cartridgectl/... ./cmd/unheaded-cli/... ./services/gateway/... ./nix/... 2>&1 | grep -v '^ok\|no test files' | head -20
  ```
- [ ] **Step 47** [V] ~1m: No line printed.
  - A `FAIL` → Step 48.
- [ ] **Step 48** [D] ~15m: Read the first failure. A test that listed the five as expected services loses them. A generic test that uses "captain" or `19002` as sample data (`pkg/discovery/*_test.go`, `pkg/healthreport/reporter_test.go`, `pkg/iac/*_test.go`, `tests/s77`) stays as it is unless it reads a deleted constant or file. Two tries, then `[STUCK]`.
- [ ] **Step 49** [C] ~1m: **COMMIT** as Step 41: "Steps 42-49: build files, scripts, runbooks and panels without the five".
- [ ] **Step 50** [B] ~2m: Gate G1 (no code names them), G2 (no operational file names them), G3 (no port left as a target).
  ```bash
  cd /home/govan/tmp/wt/S-WASTE-OUT
  git grep -nE '(ports|uids)\.(Architect|Captain|Micromanager|Monad|Sophia|NginxMonad|NginxSophia)\b' -- '*.go' | head
  git grep -nE 'unheaded-(architect|captain|micromanager|monad|sophia)\b|svc_key_(architect|captain|micromanager|monad|sophia)|nginx-(monad|sophia)' -- docker-compose.yml monitoring config configs deploy debian scripts Makefile Dockerfile docker nix nixos containers cmd pkg services runbooks | head
  git grep -nE '(:|port: ?|PORT=|localhost:)(1900[1-5]|1908[01])\b' -- docker-compose.yml monitoring config configs deploy scripts runbooks Makefile docker | head
  ```
- [ ] **Step 51** [V] ~1m: Three empty outputs.
  - A hit → fix the file by the rule of its step above, rerun. A hit that must stay (a retirement comment) → narrow nothing: reword the comment so the pattern does not match.
- [ ] **Step 52** [B] ~5m: Repository gates this phase can break.
  ```bash
  cd /home/govan/tmp/wt/S-WASTE-OUT && for g in check-compose-hardening.sh check-compose-publish-bind.sh check-compose-log-caps.sh check-compose-bind-nesting.sh check-image-digests.sh check-version-flag.sh check-unit-ceilings.sh check-prometheus-rules.sh compliance/check-service-auth.sh; do printf '%s: ' $g; scripts/$g >/dev/null 2>&1 && echo PASS || echo FAIL; done; scripts/gen-man.sh 2>&1 | tail -1
  ```
- [ ] **Step 53** [V] ~1m: Every line `PASS`.
  - `FAIL` → run that script alone, read its message. A count that names the old number of services is updated in the script's expected set, never by loosening the check. `check-version-flag.sh` needs `make build-all` first.
- [ ] **Step 54** [B] ~3m: The repointed provocation still fails its gate (needs a clean tree: commit first if Step 53 changed a file).
  ```bash
  cd /home/govan/tmp/wt/S-WASTE-OUT && git status --short | wc -l && scripts/check-gates-can-fail.sh --only check-systemd-units 2>&1 | tail -3
  ```
- [ ] **Step 55** [V] ~1m: `0`, then the gate reports that the provoked tree failed as it should. "provocation anchor missing" → the timeguru unit lacks the line: `grep -n '^SystemCallFilter=~' deploy/systemd/*.service`, pick a unit that has it, repeat Step 43's edit.
- [ ] **Step 56** [B] ~2m: `cd /home/govan/tmp/wt/S-WASTE-OUT && ./scripts/check-gosec-ratchet.sh 2>&1 | tail -2 && scripts/check-sbom-trend.sh 2>&1 | tail -1`
- [ ] **Step 57** [V] ~1m: Both pass (counts may fall; a baseline only shrinks: patch only this plan's row, numbers under "For Stevie").
- [ ] **Step 58** [C][V] ~1m: **PHASE 3 EXIT GATE**: build, vet, the listed tests and gates green; G1-G3 empty. Commit and log the phase in `PROG`.

---

## PHASE 4: THE DOCUMENTS SAY SO (Steps 59-68) [P]

**Goal**: Live documents no longer describe five services that do not exist. Dated records are untouched.
**Prerequisite**: Phase 3 gate.
**Time**: 2 h.
**Model**: Sonnet. Stop line: a sentence whose meaning is unclear without the five is marked in `PROG` and left. **Agent**: `[P]`, worktree `/home/govan/tmp/wt/S-WASTE-OUT-docs` off `sprint/s-waste-out`.

- [ ] **Step 59** [B] ~1m: `cd /home/govan/tmp/unheaded && git worktree add /home/govan/tmp/wt/S-WASTE-OUT-docs -b sprint/s-waste-out-docs sprint/s-waste-out`
- [ ] **Step 60** [B] ~1m: The work list.
  ```bash
  cd /home/govan/tmp/wt/S-WASTE-OUT-docs && git grep -lE '1900[1-5]|\b(architect|captain|micromanager)\b.*(service|container|:190)|services/(architect|captain|micromanager|monad|sophia)|cmd/(monad|sophia)\b|Service-(Architect|Captain|Micromanager)' -- CLAUDE.md README.md QUICKSTART.md wiki docs/ARCHITECTURE.md docs/architecture docs/services docker/README.md docker/QUICKSTART.md docker/INDEX.md deploy/systemd/README.md nix/README.md nix/DEPLOYMENT.md nixos/modules/services/README.md lxd/hosts/DEPLOYMENT_CHECKLIST.md | sort > /home/govan/tmp/wt/S-WASTE-OUT-scratch/doc-list.txt; wc -l < /home/govan/tmp/wt/S-WASTE-OUT-scratch/doc-list.txt
  ```
- [ ] **Step 61** [R] ~2m: Never edited: `docs/adr/`, `docs/battle-plans/`, `docs/sweeps/`, `docs/history/`, `docs/gamedays/`, `CHANGELOG.md`, `db/migrations/`, `references/`, `tomb/`, `sbom/`, `skills/`, `ietf-submission/`, `docs/GEMINI.md`. A skill is not a service: "unheaded-architect", "the Architect seat", `owner: architect` stay everywhere.
- [ ] **Step 62** [W] ~30m: `CLAUDE.md`:
  - "The 6 Layers": line `Layer 4: Application Services (timeguru, captain, micromanager, architect)` → `Layer 4: Application Services (timeguru)`.
  - Port Allocation table, row "Core Services": `timeguru (19000), architect (19001), captain (19002), micromanager (19003), monad (19004), sophia (19005), cuirass (19006), ...` → `timeguru (19000), cuirass (19006), sword (19011), chaos-controller (19012); 19001-19005 retired`.
  - Project Structure: delete the tree lines `captain/`, `micromanager/`, `architect/`, `monad/`, `sophia/` under `services/`.
  - "Go Services (timeguru, captain, micromanager, architect)" → "Go Services (timeguru)".
  - Command-line shape paragraph: "the five service binaries built from `services/` (wotan, timeguru, captain, architect, micromanager)" → "the two service binaries built from `services/` (wotan, timeguru)".
  - "**Services:** 10 core + Zhen AI (...)" → "**Services:** timeguru, dashboard-backend, kanban-app, wotan, unheaded-daemon (Cuirass), akira; Zhen is a module (ADR-147)".
  - Port Registry table: delete the rows `architect`, `captain`, `micromanager`, `monad`, `sophia`, `nginx-monad`, `nginx-sophia`; add one row `| retired | 19001-19005, 19080, 19081 | | removed 2026-11 (ADR-147 decision 7), archived in unheaded-role-services; not reused |`.
  - Strategic Decisions list: add after the WAF bullet: `- **Role services**: architect, captain, micromanager and the monad and sophia HTTP services were removed 2026-11 (ADR-147 decision 7) and are archived with their history in \`unheaded-role-services\`. The Monad wire format, the Sophia dictionaries and the eBPF maps are not touched.`
  - "Wotan Topics / Required subscriptions" is rewritten in Phase 8, not here.
  - Sentences dated before 2026-11 that list the services as history ("What's Done", S51) stay.
- [ ] **Step 63** [W] ~40m: Each other file of `doc-list.txt`. Rules, in order: (1) a table row or list item that is only about one of the five is deleted; (2) a list of services loses the five names; (3) a port table gets the retired row of Step 62 once; (4) a link to `Service-Architect`, `Service-Captain` or `Service-Micromanager` is deleted with its list item; (5) `wiki/Microservices.md`, `wiki/Port-Registry.md` and `wiki/Architecture.md` each get one line: "architect, captain, micromanager and the monad and sophia services were removed in 2026-11 (ADR-147) and archived in `unheaded-role-services`"; (6) prose about the Monad protocol or Sophia dictionaries is not touched. `wiki/_Sidebar.md` and `wiki/Home.md`: delete the three page links.
- [ ] **Step 64** [B] ~1m: `cd /home/govan/tmp/wt/S-WASTE-OUT-docs && git grep -nE 'Service-(Architect|Captain|Micromanager)' -- wiki README.md CLAUDE.md | head; git grep -nE '\b1900[1-5]\b' -- wiki/Port-Registry.md CLAUDE.md README.md QUICKSTART.md | grep -v retired | head`
- [ ] **Step 65** [V] ~1m: Both empty. A hit → apply the rule of Step 63.
- [ ] **Step 66** [B] ~2m: `cd /home/govan/tmp/wt/S-WASTE-OUT-docs && scripts/check-spdx.sh 2>&1 | tail -1; ls scripts | grep -E '^check-(wiki|doc|links)' | while read g; do printf '%s: ' $g; scripts/$g >/dev/null 2>&1 && echo PASS || echo FAIL; done`
- [ ] **Step 67** [V] ~1m: No `FAIL` that names a file this phase touched.
- [ ] **Step 68** [C][V] ~3m: **PHASE 4 EXIT GATE**: commit the files of `doc-list.txt` by name; merge the lane.
  ```bash
  cd /home/govan/tmp/wt/S-WASTE-OUT && git merge --no-ff -S sprint/s-waste-out-docs -m "[PLAN S-WASTE-OUT] Steps 59-68: live documents without the five role services" || { git merge --abort; echo CONFLICT; }
  ```
  `CONFLICT` → `[STUCK]`; the lane stays unmerged.

---

## PHASE 5: CUIRASS RUNS ONLY AGAINST A REAL LXD (Steps 69-80)

**Goal**: A default `compose up` on west starts no Cuirass, and Cuirass never falls into the mock because a variable is missing.
**Prerequisite**: Phase 3 gate.
**Time**: 1.5 h.
**Model**: Opus (the compliance probe table and a control loop's start-up). **Agent**: Coordinator in `WT`.

- [ ] **Step 69** [R] ~5m: `cd /home/govan/tmp/wt/S-WASTE-OUT && grep -n 'LXDBackend\|NewMockLXDClient' cmd/unheaded-daemon/*.go`; read the function that switches on `LXDBackend` and `main.go:225-245, 405-430`; `docker/compose.cuirass-lxd.yml`.
- [ ] **Step 70** [W] ~10m: Red. In `cmd/unheaded-daemon`, a test `TestMockBackendMustBeAskedFor` on that switch function:
  - `LXDBackend == ""` → an error containing `LXD_BACKEND is not set`; no client.
  - `"mock"` → the mock client (tests and dev runs keep working).
  - `"unix"` and `"https"` → as today.
- [ ] **Step 71** [B] ~1m: `cd /home/govan/tmp/wt/S-WASTE-OUT && go test -count=1 ./cmd/unheaded-daemon -run TestMockBackendMustBeAskedFor 2>&1 | tail -4`
- [ ] **Step 72** [V] ~1m: Red: the empty case returns the mock.
- [ ] **Step 73** [W] ~10m: Green. The empty value returns the error; `main` exits 1 with it. The `"mock"` branch logs once at warn: `mock LXD backend: nothing here is reconciled against a real LXD`. Update the flag or env help text and any test that relied on the empty default (they set `"mock"`).
- [ ] **Step 74** [B][V] ~3m: GO-GATE `./cmd/unheaded-daemon/...`. Green.
- [ ] **Step 75** [W] ~15m: Lists.
  - `docker-compose.yml`, service `cuirass`: add `profiles: ["lxd"]` under `container_name`, with the comment `# ADR-147 decisions 7, 14: runs only with docker/compose.cuirass-lxd.yml (a real LXD); a plain "up" does not start it`. In `sword` (profile `deploy`), beside its `depends_on: cuirass`, the comment `# needs --profile lxd as well`.
  - `docker/compose.cuirass-lxd.yml`: the usage line in the header becomes `sudo docker compose -f docker-compose.yml -f docker/compose.cuirass-lxd.yml --profile lxd up -d cuirass`; delete the line "Back to the mock backend: bring the stack up without this file."
  - `config/akira/akira.yaml`: delete the `cuirass` target. `config/dashboard/services.txt`: delete `cuirass=cuirass:19006`.
  - `config/victoria/scrape.yml`, job `unheaded-daemon`: delete the target `host.docker.internal:19006`; if the job has no target left, delete the job. `monitoring/prometheus/prometheus.yml`, job `unheaded-daemon`: delete the static config whose target is `[fd00:dead:beef::1]:19006`; east's `:17000` stays.
  - `scripts/kingdom-startup.sh`, `scripts/qa-smoke.sh`, `scripts/wait-for-healthy.sh`, `Makefile` (the `name:port` loop): drop `cuirass` and `cuirass:19006`.
  - `pkg/compliance/crosswalk/hostservice.go`, row `cuirass`: it runs on no host until plan 7. Remove `Runs: onWest("unheaded-cuirass")`; if the table's test demands a run for every row, make the test accept a row with no run and a `Why:` ("not run on west until a tenant is placed: ADR-147 decisions 7 and 14"). `scripts/compliance/check-service-auth.sh` still counts it (it is a compose service).
- [ ] **Step 76** [B] ~2m:
  ```bash
  cd /home/govan/tmp/wt/S-WASTE-OUT && python3 -c 'import yaml; s=yaml.safe_load(open("docker-compose.yml"))["services"]; print(s["cuirass"].get("profiles"), [k for k,v in s.items() if "cuirass" in str(v.get("depends_on",""))])' && git grep -nE 'cuirass:19006|:19006\b' -- config monitoring scripts Makefile | head
  ```
- [ ] **Step 77** [V] ~1m: `['lxd'] ['sword']`; no grep hit except east's or a comment.
- [ ] **Step 78** [W] ~5m: `wiki/Service-Sword.md` and `CLAUDE.md` (the "Real deploy via Sword + Cuirass" bullet): one clause, "Cuirass is started with `--profile lxd` and the real-LXD overlay; it has no default backend". `PROG` "Handoffs", for plan 7: the start command above; `CUIRASS_ENFORCE` and `CUIRASS_PRUNE` stay unset until a tenant exists; the volume `cuirass-state` holds a stale desired state (three `steel-echo-v2-*` containers) to clear before the first enforce.
- [ ] **Step 79** [B][V] ~3m: `cd /home/govan/tmp/wt/S-WASTE-OUT && GOMAXPROCS=4 go test -race -count=1 ./pkg/compliance/... ./cmd/unheaded-daemon/... 2>&1 | tail -4; for g in check-compose-hardening.sh check-prometheus-rules.sh compliance/check-service-auth.sh; do printf '%s: ' $g; scripts/$g >/dev/null 2>&1 && echo PASS || echo FAIL; done`. All `ok` and `PASS`.
- [ ] **Step 80** [C][V] ~1m: **PHASE 5 EXIT GATE**: an unset backend refuses; compose holds Cuirass behind a profile. Commit as Step 41: "Steps 69-80: Cuirass has no default backend and no place in a plain compose up".

---

## PHASE 6: ONE STREAM LOOP, ONE WAY BACK, HEALTH THAT SAYS WHICH (Steps 81-101)

**Goal**: A stream on `wotanClient.Client` is a gRPC stream. If gRPC fails it polls, and it returns to gRPC when the probe answers. Health and metrics name the transport of each stream. A client with no gRPC address refuses to stream unless told to poll.
**Prerequisite**: Phase 3 gate (in `sprint/s-waste-out`).
**Time**: 5 h.
**Model**: Opus (concurrency, reconnect, cursor handling). **Agent**: lane `wotan`: worktree `/home/govan/tmp/wt/S-WASTE-OUT-wotan`, which also runs Phases 7 to 11 in order (same packages).

- [ ] **Step 81** [B] ~1m: `cd /home/govan/tmp/unheaded && git worktree add /home/govan/tmp/wt/S-WASTE-OUT-wotan -b sprint/s-waste-out-wotan sprint/s-waste-out`
- [ ] **Step 82** [R] ~20m: In `pkg/wotan-client`: `client.go:160-250, 267-375, 776-990`; `reprobe.go`; `stream_health.go`; `topic_client.go:460-770`; `grpc_client.go:268-430`; the tests `reprobe_test.go`, `s77_transport_state_test.go`, `fallback_recovery_test.go`, `restart_resume_test.go`, `stream_health_test.go`; `wotantest/wotantest.go` (the fake server).
- [ ] **Step 83** [W] ~40m: Red. New `pkg/wotan-client/stream_return_test.go`, against the `wotantest` server (add to it a switch that makes its gRPC listener refuse and accept again, if it has none):
  - `TestStreamReturnsToGRPCAfterOutage`: client from `NewClientWithGRPC`, re-probe interval 50 ms. Publish seq 1-5 over gRPC; refuse gRPC until the client polls (`unheaded_wotan_client_polls_total{topic}` rises); publish 6-10; accept gRPC again; publish 11-15. Assert: every seq 1-15 is delivered once, in order; within 10 re-probe intervals the poll counter stops rising across 4 poll intervals.
  - `TestHealthNamesPollingStream`: during the outage `Health().StreamTransport[topic] == "http-poll"` and `Health().Transport == "HTTP"`; after the return `"grpc"` and `"gRPC"`.
  - `TestStreamWithoutGRPCRefused`: `NewClient(addr)` then `StreamMessages` → `errors.Is(err, ErrNoStreamTransport)`. With `WithHTTPPollStreams()` it polls as today.
  - `TestPollCounterCountsEachPoll`: N polls → the counter rose by N.
  - `TestStreamTransportGauge`: `unheaded_wotan_client_stream_transport{topic,transport}` is 1 for the active transport and absent for the other.
- [ ] **Step 84** [B] ~2m: `cd /home/govan/tmp/wt/S-WASTE-OUT-wotan && go test -count=1 ./pkg/wotan-client -run 'TestStreamReturns|TestHealthNames|TestStreamWithoutGRPC|TestPollCounter|TestStreamTransportGauge' 2>&1 | tail -6`
- [ ] **Step 85** [V] ~1m: Red: `undefined: ErrNoStreamTransport`, `WithHTTPPollStreams`, `StreamTransport`.
- [ ] **Step 86** [W] ~90m: Green, the loop. One function in the package runs a topic's stream for its whole life: gRPC from the last delivered seq; on the failure that today ends in `relayGRPC`'s `pollMessagesFrom`, poll every `pollBackoffMin` and start the re-probe; when the probe promotes gRPC, stop polling and open gRPC from the last delivered seq. `TopicStreamClient.streamLoop` with `pollHTTPFallback` already has this shape: make `Client.StreamMessages` use that loop (the `Client` is its own HTTP fallback) and delete `relayGRPC`'s permanent poll and `GRPCClient`'s separate retry loop if nothing else calls it. Keep: `nextCursor` and `streamCursor` rules (never deliver at or below the cursor); `safeChannel` close semantics; `SubscribeWithRetry`; the 429 and backoff handling. Rewrite the `reprobe.go` comment at lines 49-52.
- [ ] **Step 87** [W] ~20m: Green, the rest. `var ErrNoStreamTransport = errors.New("wotan client: no gRPC address; streaming needs one (WithHTTPPollStreams for a tool that must poll)")`; `func WithHTTPPollStreams() ClientOption`. `Health` gains `StreamTransport map[string]string \`json:"stream_transport"\``; `Transport` is `"gRPC"` only while no stream polls. Counter `unheaded_wotan_client_polls_total{topic}` and gauge `unheaded_wotan_client_stream_transport{topic,transport}` beside `wotanClientStreamUp`, forgotten in `forgetStream`.
- [ ] **Step 88** [W] ~30m: Existing tests that assert the old behaviour change with it: any test expecting a degraded stream to keep polling after promotion now expects the return; poll-path tests built on `NewClient` add `WithHTTPPollStreams()`. No test is deleted to get green: each changed assertion is listed in `PROG`.
- [ ] **Step 89** [B] ~5m: `cd /home/govan/tmp/wt/S-WASTE-OUT-wotan && GOMAXPROCS=4 go test -race -count=1 ./pkg/wotan-client/... 2>&1 | tail -6`
- [ ] **Step 90** [V] ~1m: `ok`.
  - Fail → Step 91.
- [ ] **Step 91** [D] ~20m: A duplicate or missing seq at the hand-over is the cursor: the gRPC stream must be opened with the seq the poller last delivered, after the poller has stopped (wait for its goroutine). A data race in `streams` → the map is guarded by `streamsMu` only. Two tries, then `[STUCK]`: Phases 7-10 are `[BLOCKED]`; go to Phase 11.
- [ ] **Step 92** [C] ~1m: **COMMIT** the changed files of `pkg/wotan-client` by name: "Steps 81-92: a Wotan stream returns to gRPC; health and metrics name each stream's transport".
- [ ] **Step 93** [W] ~15m: Red then green. `pkg/wotan-client/policy_test.go`:
  - `TestFirstPartyBinariesDoNotPoll`: parse every non-test `.go` under `cmd/` and `services/*/cmd/`; fail on the identifier `WithHTTPPollStreams`.
  - `TestOneStreamOpener`: parse the package's non-test files; exactly one function calls the generated client method that opens the topic stream (the one `streamSingle` calls today).
- [ ] **Step 94** [B][V] ~2m: `go test -count=1 ./pkg/wotan-client -run 'TestFirstParty|TestOneStreamOpener'` → `ok`. A first-party binary that streams from `NewClient` fails at run time, not here: Phase 7 moves each one to the gRPC constructor.
- [ ] **Step 95** [W] ~15m: Red then green, `pkg/transport/health.go`: `NewHealthServer` no longer sets `grpcOK: true`. The document says `"grpc": null` until `SetGRPCStatus` is called (a `*bool`, or a `set` flag). Test `TestHealthServerDoesNotClaimGRPC`: a fresh server's JSON has no `"grpc":true`. Callers that never set it (micromanager is gone; cuirass) now report nothing instead of a guess.
- [ ] **Step 96** [B][V] ~3m: GO-GATE `./pkg/wotan-client/... ./pkg/transport/...` from the lane. Green.
- [ ] **Step 97** [B] ~3m: `cd /home/govan/tmp/wt/S-WASTE-OUT-wotan && go test -run '^$' -bench . -benchmem ./pkg/wotan-client 2>&1 | grep -E '^Benchmark' | head -8`; log the lines in `PROG` beside develop's (`cd /home/govan/tmp/unheaded && ` the same command).
- [ ] **Step 98** [V] ~1m: No benchmark more than 20% slower than develop. Slower → note it under "For Stevie"; not a stop.
- [ ] **Step 99** [B] ~5m: Everything that imports the client still builds and passes: `cd /home/govan/tmp/wt/S-WASTE-OUT-wotan && go build ./... && GOMAXPROCS=4 go test -race -count=1 ./pkg/discovery/... ./pkg/healthreport/... ./pkg/logagg/... ./cmd/akira/... ./cmd/chaos-controller/... ./services/yaldabaoth/... 2>&1 | grep -v '^ok\|no test files' | head`
- [ ] **Step 100** [V] ~1m: No line. A failure with `ErrNoStreamTransport` names a caller Phase 7 converts: note it and go on; any other failure → fix here.
- [ ] **Step 101** [C][V] ~3m: **PHASE 6 EXIT GATE**: the five new tests and the policy tests pass under `-race`. `wiki/Transport-Cascade.md` and `wiki/Service-Wotan.md` say: a stream is gRPC; polling is the fallback and ends when the probe answers; `stream_transport` in `/health`; the two new series. Commit.

---

## PHASE 7: ONE WOTAN CLIENT PER PROCESS; DISCOVERY ANNOUNCES ONLY (Steps 102-122)

**Goal**: Each binary builds one client and hands it to discovery, health reporting and log forwarding. Announcing a service opens no stream.
**Prerequisite**: Phase 6 gate.
**Time**: 4 h.
**Model**: Opus. **Agent**: lane `wotan`.

- [ ] **Step 102** [B] ~1m: The call sites.
  ```bash
  cd /home/govan/tmp/wt/S-WASTE-OUT-wotan && grep -rnE 'wotan[Cc]lient\.(NewClient|NewClientWithGRPC|NewClientWithTLS|NewTopicStreamClient|NewTopicStreamClientWithTLS|NewGRPCClient)\b' --include=*.go cmd services pkg | grep -v _test.go | grep -v '^pkg/wotan-client/' > /home/govan/tmp/wt/S-WASTE-OUT-scratch/client-sites.txt; wc -l < /home/govan/tmp/wt/S-WASTE-OUT-scratch/client-sites.txt
  ```
- [ ] **Step 103** [W] ~20m: Red. In `pkg/wotan-client/policy_test.go`:
  - `TestLibrariesBuildNoClient`: no call to a constructor above in any non-test file under `pkg/` except `pkg/wotan-client` and `pkg/transport`.
  - `TestOneWotanClientPerProcess`: for each directory under `cmd/` and `services/*/cmd/` holding a `main` package, at most one call to a constructor above or to `transport.NewWotanClient`. Allowlist, in the test, that may only shrink: `cmd/upc-cartridgectl: 2` (two subcommands; one runs per process).
- [ ] **Step 104** [B][V] ~2m: `go test -count=1 ./pkg/wotan-client -run 'TestLibrariesBuildNoClient|TestOneWotanClientPerProcess'`. Red, naming `pkg/discovery/wotan.go`, `pkg/healthreport/wotan.go`, `pkg/logagg/setup.go`, `pkg/transport/http.go`, `pkg/transport/grpc.go` and the mains with more than one. Log the list.
- [ ] **Step 105** [W] ~10m: `pkg/transport/wotan.go`: `func NewWotanClient(cfg Config, opts ...wotanClient.ClientOption) (*wotanClient.Client, error)`: `wotanClient.NewClientWithGRPC(cfg.WotanHTTPAddr, cfg.WotanGRPCAddr, opts...)`; an empty `WotanGRPCAddr` is an error naming `WOTAN_GRPC_ADDR`. Test: both addresses → a client whose `Health().Transport` is defined; empty gRPC → error.
- [ ] **Step 106** [W] ~30m: `pkg/discovery/wotan.go`: `func Start(ctx context.Context, c Announcer, name string, port int)` where `type Announcer interface { Subscribe(ctx context.Context, topic, displayName string) (*wotanClient.Subscriber, error); Publish(ctx context.Context, topic string, payload []byte) error }`. It joins `system.discovery` and announces every `AnnounceInterval`; it opens no stream, builds no registry and returns nothing. The registry, its reaper and `Watch` stay in the package for the reader (Phase 9). Tests: `TestStartAnnouncesAndDoesNotStream` (a fake `Announcer` records two publishes in two intervals; it has no `StreamMessages` method, so a stream cannot compile); `callers_test.go` keeps working (four arguments, the port still last).
- [ ] **Step 107** [W] ~20m: `pkg/healthreport/wotan.go`: `func StartService(ctx context.Context, c joiner, displayName, healthURL string)` (the client comes in; `cfg` goes). `pkg/logagg/setup.go`: `func Connect(ctx context.Context, c *wotanClient.Client, httpAddr, serviceName string) (transport.Connection, error)`. `pkg/transport/http.go:25` and `grpc.go:25`: take a client instead of building one, or are deleted if nothing calls them (`grep -rn 'transport\.\(NewHTTP\|NewGRPC\|WrapHTTPClient\)' --include=*.go cmd services pkg`).
- [ ] **Step 108** [B][V] ~3m: GO-GATE `./pkg/discovery/... ./pkg/healthreport/... ./pkg/logagg/... ./pkg/transport/...` from the lane. Green (the mains do not build yet: that is Step 110).
- [ ] **Step 109** [C] ~1m: **COMMIT**: "Steps 102-109: discovery, health reports and log forwarding take the process's client".
- [ ] **Step 110** [W] ~60m: The mains. In each, build one client right after `transportCfg` is known, give it the identity, pass it on:
  ```go
  wc, err := transport.NewWotanClient(transportCfg)
  if err != nil { /* as the file handles a missing Wotan today */ }
  wc.InitServiceIdentity("<identity>")           // the identity the file already uses
  discovery.Start(ctx, wc, "<name>", ports.PortOf(...))
  healthreport.StartService(ctx, wc, "<name>", healthreport.HealthURL(...))
  logConn, err = logagg.Connect(ctx, wc, transportCfg.WotanHTTPAddr, "<name>")
  ```
  Files: `services/timeguru/cmd/timeguru/main.go` (drop `initWotan`'s HTTP-only branch); `cmd/akira/main.go:270-281` (always the gRPC constructor; the signing key decides only what is published); `cmd/unheaded-daemon/main.go:567-581, 765`; `cmd/sword/main.go:167-169, 249-250`; `cmd/chaos-controller/main.go:221-223, 585-586` and `services/yaldabaoth/abort.go:125` (the abort listener takes the same client); `cmd/kev-poller/main.go:296`; `cmd/anomaly/main.go:115,135`.
- [ ] **Step 111** [W] ~30m: `cmd/trace-collector-go`: `main.go:353, 380, 385`, `publisher.go:172, 178` and lines 1258, 1288-1289 share one client. Publishing keeps its gRPC path.
- [ ] **Step 112** [W] ~45m: `cmd/dashboard-backend`: one client from `main.go`, given to the eBPF ingestor (`main.go:272-273`), the event streamer (`internal/events/events.go:446`: `connect` takes the client; its reconnect loop goes, the client reconnects itself) and the health persister (`internal/server/server.go:3547`). `getEventTopics` (`main.go:383-401`) and the default at `events.go:92` keep only what a running binary publishes: `timeline.*`, `tasks.*`, `system.health.consensus`, `traces.packet`, `traces.flow`, `traces.latency`, `traces.correlated`. Dropped: `metrics.*`, `health.*`, `alerts.*`, `decisions.*`, `state.*`, `architecture.*`. Delete `PublishAlertEvent` and any other `Publish*Event` in `events.go` that `grep` shows has no caller.
- [ ] **Step 113** [W] ~30m: `cmd/kanban-app/main.go:1459-1544`: one client for `tasks.*`, `timeline.updates`, runbook progress, discovery, health reports and logs.
- [ ] **Step 114** [B] ~5m: `cd /home/govan/tmp/wt/S-WASTE-OUT-wotan && go build ./... && GOMAXPROCS=4 go test -race -count=1 ./cmd/... ./services/... ./pkg/discovery/... ./pkg/healthreport/... ./pkg/logagg/... 2>&1 | grep -v '^ok\|no test files' | head -20`
- [ ] **Step 115** [V] ~1m: No line.
  - Fail → Step 116.
- [ ] **Step 116** [D] ~20m: A test that started a service with only an HTTP Wotan address now needs the `wotantest` server's gRPC address too. A test fake that implemented the old three-method `topicClient` implements `Announcer`. Two tries per package, then `[STUCK]` for that binary only (its allowlist entry stays, logged).
- [ ] **Step 117** [B][V] ~1m: Step 104's command. Green; the allowlist holds only `cmd/upc-cartridgectl`, plus any `[STUCK]` binary named in `PROG`.
- [ ] **Step 118** [W] ~10m: `configs/wotan.yaml`, `permissions`: `dashboard-backend` `subscribe` loses `metrics.*`, `health.*`, `alerts.*`, `decisions.*`, `state.*`, `architecture.*`, and `publish` loses `metrics.*`, `health.*`, `alerts.*`; the comment at 164-165 lists what is left. No identity needs `subscribe: ["system.discovery"]` any more: delete it wherever it appears; `publish` keeps it.
- [ ] **Step 119** [B][V] ~2m: `cd /home/govan/tmp/wt/S-WASTE-OUT-wotan && go test -count=1 ./services/wotan/internal/api -run 'RepoConfig|Permission' 2>&1 | tail -3` → `ok`. A test that lists each binary's topics is updated to the new lists.
- [ ] **Step 120** [W] ~10m: `CLAUDE.md` "Service Discovery": the paragraph "12 services call `discovery.Start(ctx, transportCfg, name, port)`" becomes: "Each service calls `discovery.Start(ctx, wc, name, port)` with its one Wotan client (`transport.NewWotanClient`): it announces on `system.discovery` every 15 s and opens no stream. The same client carries its health reports and its logs; `pkg/wotan-client/policy_test.go` fails a binary that builds a second client or a library that builds one." `wiki/Service-Discovery.md`: the same.
- [ ] **Step 121** [B][V] ~3m: GO-GATE `./pkg/... ./cmd/... ./services/...` from the lane, then `./scripts/check-gosec-ratchet.sh 2>&1 | tail -2`. Green.
- [ ] **Step 122** [C][V] ~1m: **PHASE 7 EXIT GATE**: both policy tests green; the tree builds. Commit: "Steps 110-122: one Wotan client per process; discovery announces and does not stream".

---

## PHASE 8: `alerts.critical` DOES WORK (Steps 123-146)

**Goal**: A critical alert reaches every service on the bus, signed, and each service knows it. With D2 = B, change is held while one fires.
**Prerequisite**: Phase 7 gate. Steps 131-140 need D1; Steps 141-145 need D2 = B.
**Time**: 5 h.
**Model**: Opus (a signed topic, a hold on deploys). **Agent**: lane `wotan`.

**What it is for.** A second host or a tenant can then act on "the kingdom is in trouble" without asking Alertmanager. The message is metadata only: `{"fingerprint","alertname","severity","status":"firing|resolved","host","starts_at"}` and, every 5 min, `{"snapshot":[fingerprints]}`. No annotation text, no label outside that list.

- [ ] **Step 123** [R] ~10m: `pkg/healthreport/verified.go` (how a sender is admitted); `cmd/akira/publish.go` (signed publish, refusal counter); `pkg/wotan-client/publish_signed.go:77-118`; `cmd/alert-mailer/main.go` and `mailer.go` (the webhook handler and where `severity` is read); `monitoring/docker-compose.yml` (alert-mailer's block); `monitoring/alertmanager/alertmanager.yml` (`repeat_interval` 24h).
- [ ] **Step 124** [W] ~30m: Red. New package `pkg/critalert`, file `critalert_test.go`:
  - `TestFiringThenResolved`: a firing message → `Active()` has it, `Firing()` true, gauge `unheaded_critical_alerts_active` 1; its resolved message → empty, false, 0.
  - `TestSnapshotDropsWhatItOmits`: two active; a snapshot naming one → the other is dropped.
  - `TestSilentPublisherIsUnknown`: no message for 15 min (injected clock) → `State()` is `Unknown`, `Firing()` false, `unheaded_critical_alerts_state{state="unknown"}` 1, one warn line.
  - `TestBounded`: the 257th distinct fingerprint is refused and counted (`unheaded_critical_alerts_refused_total{reason="full"}`).
  - `TestBadPayloadCounted`: not JSON, an unknown field, a `status` outside the two values, a field over 128 bytes → refused, counted by `reason`, state unchanged.
  - `TestOneLogLinePerChange`: the same firing message 10 times → one warn line.
- [ ] **Step 125** [B][V] ~1m: `cd /home/govan/tmp/wt/S-WASTE-OUT-wotan && go test -count=1 ./pkg/critalert 2>&1 | tail -3`. Red: no non-test files.
- [ ] **Step 126** [W] ~45m: Green. `pkg/critalert/critalert.go`:
  ```go
  const Topic = "alerts.critical"
  type Alert struct { Fingerprint, Name, Severity, Status, Host string; StartsAt time.Time }
  type State int // OK, Firing, Unknown
  type Subscriber interface { SubscribeWithRetry(ctx context.Context, topic, name string, onMsg func(*wotanClient.Message)) error }
  func Attach(ctx context.Context, c Subscriber, name string) *Tracker
  func (t *Tracker) Active() []Alert
  func (t *Tracker) State() State
  func (t *Tracker) Firing() bool
  func (t *Tracker) Health() map[string]any // {"state":"ok|firing|unknown","active":N}
  ```
  Strict decode (`DisallowUnknownFields`), the map capped at 256, the clock injectable. Metrics through `unheaded/pkg/metrics/auto` with `unheaded/pkg/metrics/prom` options, as `pkg/wotan-client/stream_health.go:27-34` registers its own; drop the `reg` parameter from `Attach` (and from the call sites in Step 128).
- [ ] **Step 127** [B][V] ~2m: GO-GATE `./pkg/critalert/...` from the lane. Green, 6 tests.
- [ ] **Step 128** [W] ~40m: Every binary that holds a Wotan client attaches, right after `discovery.Start`: `crit := critalert.Attach(ctx, wc, "<name>")`, and adds `crit.Health()` to its `/health` detail under `critical_alerts`. Binaries: timeguru, akira, unheaded-daemon, sword, chaos-controller, kev-poller, trace-collector, dashboard-backend, kanban-app. Delete timeguru's `handleAlert` and its own `alerts.critical` subscription (`services/timeguru/cmd/timeguru/main.go:517, 570-577`). Test in `pkg/critalert/callers_test.go`, modelled on `pkg/healthreport/callers_test.go`: a `main` package that calls `discovery.Start` and not `critalert.Attach` fails.
- [ ] **Step 129** [B][V] ~3m: `go build ./... && go test -count=1 ./pkg/critalert/... ./pkg/healthreport/...` → `ok`.
- [ ] **Step 130** [C] ~1m: **COMMIT**: "Steps 123-130: every service on the bus tracks critical alerts (pkg/critalert)".
- [ ] **Step 131** [R] ~10m: (D1 = A.) How alert-mailer can reach Wotan: it is on network `monitoring` only. Use the address Prometheus scrapes Wotan on (`monitoring/prometheus/prometheus.yml`, job `wotan`: `[fd00:dead:beef::1]:18000`, the overlay bind) for HTTP. Grounding (2026-11-15): Wotan publishes gRPC 18001 on `127.0.0.1` only (`docker-compose.yml:775`), which a container cannot reach. So add, in `wotan`'s `ports`, beside the overlay line for 18000: `- "${UNHEADED_OVERLAY_BIND:-127.0.0.3}:18001:18001"   # alert-mailer (monitoring network) publishes alerts.critical; WireGuard only (decision 16)`, and use `[fd00:dead:beef::1]:18001`. No second network. Then `scripts/check-compose-publish-bind.sh` must pass; a failure that names this line → `[STUCK]`.
- [ ] **Step 132** [W] ~30m: Red. `cmd/alert-mailer/critical_test.go`, with a fake signed publisher:
  - `TestCriticalNotificationPublishes`: a webhook with one firing alert `severity="critical"` → one signed publish on `alerts.critical` with exactly the six fields; the same alert resolved → `status":"resolved"`.
  - `TestOtherSeveritiesDoNot`: `warning` → no publish.
  - `TestNoKeyRefusesAndCounts`: no signer → no publish, `alert_mailer_critical_publish_refused_total` +1, and the mail path's result is unchanged.
  - `TestSnapshotEvery5m`: with two firing, the injected ticker fires → one snapshot with both fingerprints; with none firing → an empty snapshot (subscribers stay out of `unknown`).
  - `TestPublishFailureDoesNotFailTheWebhook`: the publisher errors → the handler still answers as before; `alert_mailer_critical_publish_errors_total` +1.
  - `TestWotanOffIsQuiet`: no `WOTAN_ADDR` → no client is built, nothing is counted.
- [ ] **Step 133** [B][V] ~1m: `go test -count=1 ./cmd/alert-mailer -run 'TestCritical|TestOtherSeverities|TestNoKey|TestSnapshot|TestPublishFailure|TestWotanOff'`. Red: undefined.
- [ ] **Step 134** [W] ~60m: Green. `cmd/alert-mailer/critical.go`: one client (`transport.NewWotanClient`, identity `alert-mailer`); the signer from `ALERTS_SIGNING_KEY_FILE` through `wotansig.LoadSignerFile`, as `cmd/akira/publish.go:80` loads its own; `PublishSigned`; the active set kept in memory and rebuilt from the mailer's own open-alert state at start if it has one. The mail path is not reordered: publish after the mail decision, never before, never blocking it (a 5 s timeout).
- [ ] **Step 135** [B][V] ~3m: GO-GATE `./cmd/alert-mailer/...` from the lane. Green.
- [ ] **Step 136** [W] ~15m: Config.
  - `monitoring/docker-compose.yml`, alert-mailer: `WOTAN_ADDR`, `WOTAN_GRPC_ADDR` (Step 131's addresses), its service key secret as the other services have (`svc_key_alert-mailer`, `AUTH_KEY_FILE`), `GOMEMLIMIT: "54MiB"`; a comment: `# ALERTS_SIGNING_KEY_FILE is not set here: without it alerts.critical is refused, never sent unsigned (enrol per deployment)`.
  - `configs/wotan.yaml`: `auto_approve` gains `"alert-mailer"`; `permissions` gains `alert-mailer: {publish: ["alerts.critical"]}`; each identity of Step 128 has `alerts.critical` in `subscribe`; the comment at line 113 becomes `# alert-mailer publishes (ALERTS_SIGNING_KEY_FILE); every service on the bus subscribes`.
  - `docker-compose.yml`, wotan's `AUTH_ALLOW_CALLERS`: add `alert-mailer`.
- [ ] **Step 137** [B][V] ~2m: Step 119's command, and `scripts/check-compose-hardening.sh >/dev/null 2>&1 && echo PASS`. `ok`, `PASS`.
- [ ] **Step 138** [W] ~10m: `wiki/Wotan-Topic-Signing.md`, the table row for `alerts.critical.`: producer alert-mailer, key `ALERTS_SIGNING_KEY_FILE`, subscribers every service on the bus. Below it, the enrolment steps: `unheaded-keygen -algo mldsa65 -owner <alert-mailer's account> alerts-critical` on the host; paste the public key under the `alerts.critical.` prefix in the topic config Wotan reads on that host (on west that file is bind-mounted from the repository today: where a deployment's keys go is Stevie's choice at Step 317); restart Wotan, then alert-mailer.
- [ ] **Step 139** [W] ~5m: `CLAUDE.md` "Wotan Topics", replace the "Required subscriptions" bullet with: "`alerts.critical`: alert-mailer publishes each critical alert, signed (ML-DSA-65), with a snapshot every 5 min. Every service that holds a Wotan client attaches `pkg/critalert` (`critalert.Attach`); a test fails one that does not. The state is in `/health` under `critical_alerts`." `wiki/Fae-Chamber-Contracts.md` and `wiki/Architecture.md` (the `alerts.critical` row): the same publisher and subscribers.
- [ ] **Step 140** [C] ~1m: **COMMIT**: "Steps 131-140: alert-mailer publishes critical alerts to the bus, signed or not at all".
- [ ] **Step 141** [W] ~30m: (D2 = B.) Red. `cmd/sword`: `TestDeployRefusedWhileCritical` (tracker firing → a deploy start answers 409 with `critical alert firing: <names>`, nothing is started, `sword_deploys_held_total` +1; tracker `Unknown` → starts, with one warn line; a running deploy is not stopped). `cmd/chaos-controller`: `TestExperimentRefusedWhileCritical`, the same three cases, counted as `chaos_refused` with reason `critical_alert`.
- [ ] **Step 142** [B][V] ~1m: Red on both.
- [ ] **Step 143** [W] ~40m: Green: the check sits at the one place each binary accepts a start (Sword's deploy handler; chaos-controller's experiment admission, beside its allowlist check). `cmd/zhen-agentd`: the same check before an unattended run starts, behind its existing gates; if plan 3 has moved or removed that path, skip it and note it for plan 3.
- [ ] **Step 144** [B][V] ~3m: GO-GATE `./cmd/sword/... ./cmd/chaos-controller/... ./cmd/zhen-agentd/...` from the lane. Green.
- [ ] **Step 145** [C] ~3m: **COMMIT**; `wiki/Service-Sword.md` and `wiki/Chaos-Engineering.md` get one line each: "No deploy (experiment) starts while a critical alert fires; state unknown allows, with a warning."
- [ ] **Step 146** [V] ~2m: **PHASE 8 EXIT GATE**: `pkg/critalert` green; the attach test green; with D1, the publisher's six tests green; with D2 = B, the two hold tests green. Unanswered parts are `[BLOCKED]` in `PROG`, not skipped silently.

---

## PHASE 9: DISCOVERY HAS A READER (Steps 147-160)

**Goal**: (D3 = A.) Akira checks every service that announces itself and is admitted, as well as the ones its config lists.
**Prerequisite**: Phase 7 gate; D3 answered. Away → the whole phase is `[BLOCKED]`.
**Time**: 3 h.
**Model**: Opus (a daemon that dials addresses it was told on a bus). **Agent**: lane `wotan`.

**What it is for.** With east as a second home (plan 6), a service that starts there is health-checked by west's Akira, and one that starts on west by east's, with no target file edited on the other host. One stream in one process, not twelve.

- [ ] **Step 147** [R] ~10m: `pkg/discovery/discovery.go` (Registry, TTL, reaper), `resolver.go`; `pkg/healthreport/verified.go:40-100` (`ownsService`, `Admit`); `cmd/akira/main.go:100-160` (config loading) and `pkg/health/akira.go:180-285` (the sweep).
- [ ] **Step 148** [W] ~30m: Red. `pkg/discovery/watch_test.go`:
  - `TestWatchAdmitsVerifiedSender`: an announcement whose `Name` equals the message's verified sender is in the registry; one whose name differs is refused and counted (`unheaded_discovery_refused_total{reason="sender"}`).
  - `TestWatchRefusesAddressOutsideSubnets`: `AllowSubnets` `10.10.20.0/24`: `10.10.20.7` admitted; `192.0.2.1`, a hostname that resolves outside, an empty address → refused, `reason="address"`. The address kept is the checked IP, not the name.
  - `TestWatchBounded`: the 257th entry refused, `reason="full"`.
  - `TestWatchReaps`: an entry not announced for 3 x `DefaultTTL` is gone.
  - `TestWatchOneStream`: the fake client saw exactly one `StreamMessages` call.
  - `TestNoSubnetsMeansOff`: an empty `AllowSubnets` → `Watch` returns an error; nothing is subscribed.
- [ ] **Step 149** [B][V] ~1m: `go test -count=1 ./pkg/discovery -run TestWatch`. Red: undefined `Watch`.
- [ ] **Step 150** [W] ~45m: Green. `pkg/discovery/watch.go`:
  ```go
  type Admit struct { AllowSubnets []netip.Prefix; MaxEntries int; Resolve func(ctx context.Context, host string) ([]netip.Addr, error) }
  func Watch(ctx context.Context, c Streamer, name string, a Admit) (*Registry, error)
  ```
  `net/netip` only. Counters through `pkg/metrics/auto`, as Step 126. The sender comes from the field Wotan stamps (`pkg/healthreport.Admit` reads the same one).
- [ ] **Step 151** [B][V] ~2m: GO-GATE `./pkg/discovery/...` from the lane. Green.
- [ ] **Step 152** [W] ~30m: Red then green, Akira. Config key `discover: {subnets: ["..."]}` in the YAML and flag `--discover-subnets`; absent → off, as today. On each sweep the target list is the configured targets plus each registry entry whose name is not configured, checked at `https://<ip>:<port>/health` with the entry's name as the TLS server name. A discovered target votes like any other; it appears in `GET /api/v1/status` with `"source":"discovered"`. Tests in `cmd/akira` and `pkg/health`: `TestSweepIncludesDiscovered`, `TestConfiguredWinsOverDiscovered`, `TestDiscoveryOffByDefault`.
- [ ] **Step 153** [B][V] ~3m: GO-GATE `./cmd/akira/... ./pkg/health/...`. Green.
- [ ] **Step 154** [W] ~5m: `config/akira/akira.yaml`: `discover: {subnets: ["172.28.1.0/24"]}` with the comment `# the compose data network (pinned in docker-compose.yml): a service that announces from it is checked without a line here`. Confirm the subnet first: `sed -n '/^  data:/,/subnet/p' /home/govan/tmp/wt/S-WASTE-OUT-wotan/docker-compose.yml | tail -1` prints `- subnet: 172.28.1.0/24` (2026-11-15). Another value → use it. `configs/wotan.yaml`: `akira` gains `subscribe: ["system.discovery"]`.
- [ ] **Step 155** [B][V] ~2m: Step 119's command → `ok`.
- [ ] **Step 156** [W] ~10m: `wiki/Service-Discovery.md` and `wiki/Service-Akira.md`: who announces, who reads, the three admission rules, the off switch. `CLAUDE.md` "Service Discovery": add "Akira is the reader (`discovery.Watch`): it checks admitted announcements it has no line for."
- [ ] **Step 157** [B] ~2m: `cd /home/govan/tmp/wt/S-WASTE-OUT-wotan && go build ./... && ./scripts/check-gosec-ratchet.sh 2>&1 | tail -2 && scripts/check-netip.sh 2>&1 | tail -1`
- [ ] **Step 158** [V] ~1m: All pass (`net.IP` count did not rise).
- [ ] **Step 159** [W] ~2m: `PROG` "Handoffs", plan 6: east's Akira gets `discover.subnets` for the link and overlay addresses when east has its Wotan back.
- [ ] **Step 160** [C][V] ~1m: **PHASE 9 EXIT GATE**: six `Watch` tests and three Akira tests green. Commit: "Steps 147-160: Akira reads service discovery; announcements are admitted by sender, subnet and count".

---

## PHASE 10: ONE TOPIC PER TRACE EVENT (Steps 161-176)

**Goal**: The collector publishes each event once, on `traces.*`. The dashboard's two consumers read that one topic. `ebpf.packet.events`, `ebpf.flow.events` and `ebpf.latency.events` end.
**Prerequisite**: Phase 7 gate (the dashboard's one client).
**Time**: 3 h.
**Model**: Opus (two payload shapes for packets; a live view that must not show an event twice). **Agent**: lane `wotan`.

**Why `traces.*` is the name kept.** It is the documented family; `traces.stats` and `traces.correlated` have no `ebpf.` twin; the code calls the others "mirror topics for dashboard ingestor compatibility" (`publisher.go:56`).

- [ ] **Step 161** [R] ~15m: `cmd/trace-collector-go/publisher.go:36-60, 184-196, 208-262`, `main.go:386-395, 1170-1195`, `packet_event.go:50-130`; `cmd/dashboard-backend/internal/ebpf/ingestor.go:185-260` (topics and how each is decoded); `internal/server/server.go:1908-1930, 2028-2050` (`broadcastTraceEvents`, `parseTraceTopicEvent`); `cmd/demo-trace-injector/main.go:120-170`; `cmd/perf-benchmark` (grep `ebpf\.` and `traces\.`).
- [ ] **Step 162** [W] ~30m: Red, collector. `publisher_test.go`: the tests at lines 205, 589 and 668 that expect two batches per event expect one, on `traces.packet`, `traces.flow`, `traces.latency`. New `TestNoEBPFMirrorTopics`: after one event of each kind no publish names a topic matching `^ebpf\.(packet|flow|latency)\.events$`. `TestPacketPayloadIsTheDashboardShape`: the one packet payload decodes as the dashboard's `ebpf.PacketEvent` (the shape `marshalDashboardPacket` writes) and still carries `trace_id`.
- [ ] **Step 163** [B][V] ~1m: `go test -count=1 ./cmd/trace-collector-go -run 'TestNoEBPFMirror|TestPacketPayload|Publish' 2>&1 | tail -5`. Red.
- [ ] **Step 164** [W] ~30m: Green, collector. `PublishFlowEvent` and `PublishLatencyEvent` queue once. `PublishPacketEvent` queues the dashboard shape on `traces.packet`; the old `traces.packet` shape goes unless Step 161 found a reader that needs a field the dashboard shape lacks: then add that field to the one shape. `main.go:1181` publishes its flow JSON on `traces.flow`. Delete `TopicEBPFPacket`, `TopicEBPFFlow`, `TopicEBPFLatency` and the three names at `main.go:393`.
- [ ] **Step 165** [B][V] ~3m: GO-GATE `./cmd/trace-collector-go/...`. Green.
- [ ] **Step 166** [C] ~1m: **COMMIT**: "Steps 161-166: the collector publishes each trace event once".
- [ ] **Step 167** [W] ~30m: Red, dashboard. `internal/ebpf`: `TestIngestorReadsTraces`: `Topics` holds `traces.packet`, `traces.flow`, `traces.latency` and none of the three `ebpf.` names; a flow payload on `traces.flow` is ingested once. `internal/server`: `TestTraceEventReachesBothConsumersOnce`: one message on `traces.flow` → the flow graph counts one flow and the trace buffer holds one trace event; the WebSocket sends each type once.
- [ ] **Step 168** [B][V] ~1m: Red.
- [ ] **Step 169** [W] ~45m: Green. One subscription per trace topic on the dashboard's one client; its messages fan out in the process to the ingestor and to `broadcastTraceEvents`. `getEventTopics` loses the three `traces.` names it shares with the ingestor (the comment at `main.go:394-397` already states the rule: one subscriber per topic). `ebpf.syscall.events`, `compute.*`, `anamnesis.*` and `bench.perf.packet` are untouched.
- [ ] **Step 170** [W] ~10m: `cmd/demo-trace-injector/main.go:128, 166`: it publishes on the `traces.` names. `cmd/perf-benchmark`: unchanged if it uses `bench.perf.packet`.
- [ ] **Step 171** [B][V] ~3m: GO-GATE `./cmd/dashboard-backend/... ./cmd/demo-trace-injector/... ./cmd/perf-benchmark/...`. Green.
- [ ] **Step 172** [W] ~5m: `configs/wotan.yaml`: `trace-collector` `publish` loses `"ebpf.*.events"`; `dashboard-backend`'s comment and `subscribe` lose `ebpf.*.events` only if no remaining topic matches it (`ebpf.syscall.events` does: then keep the pattern and say why).
- [ ] **Step 173** [B][V] ~2m: Step 119's command → `ok`.
- [ ] **Step 174** [B] ~1m: `git grep -nE 'ebpf\.(packet|flow|latency)\.events' -- cmd pkg services configs wiki CLAUDE.md | head`
- [ ] **Step 175** [V] ~1m: No hit outside a test that asserts the names are gone. A wiki hit → reword to `traces.*`.
- [ ] **Step 176** [C][V] ~1m: **PHASE 10 EXIT GATE**: collector and dashboard green; no mirror topic left. Commit: "Steps 167-176: the dashboard reads traces.* once for both its views".

---

## PHASE 11: WOTAN'S RATE LIMIT PER CALLER; NO AUDIT LINE LOST UNCOUNTED (Steps 177-190)

**Goal**: Wotan's rate limit is one bucket per verified caller, not one per proxy address. A failed audit write is counted and can page.
**Prerequisite**: Phase 10 gate (lane `wotan`).
**Time**: 2 h.
**Model**: Opus (auth context in a limiter). **Agent**: lane `wotan`.

- [ ] **Step 177** [R] ~10m: `services/wotan/internal/middleware/ratelimit.go:80-104` (bucket keyed on `clientIP(r.RemoteAddr)`); `services/wotan/cmd/wotan/main.go:240-262` (the limiter sits inside `wotanAuth`, so the request context already carries the identity) and `:599-611`; `pkg/auth/auth.go:49` (`ServiceSubject`) and `:238` (`IdentityFromContext`); `pkg/auth/audit.go:84-104` (`_, _ = a.w.Write(data)`).
- [ ] **Step 178** [W] ~20m: Red. In `services/wotan/internal/middleware/ratelimit_test.go`, rate 1, burst 1:
  - `TestBucketPerVerifiedCaller`: two requests from one `RemoteAddr` (`172.28.1.1:5000`), the first with identity `kanban-app`, the second with `dashboard-backend` in the context → both 200; a second `kanban-app` request → 429.
  - `TestNoIdentityFallsBackToIP`: two requests with no identity from one IP → the second 429.
  - `TestKeysDoNotCollide`: an identity whose subject is `172.28.1.1` and an unauthenticated request from `172.28.1.1` use different buckets (keys are prefixed `sub:` and `ip:`).
- [ ] **Step 179** [B][V] ~1m: `cd /home/govan/tmp/wt/S-WASTE-OUT-wotan && go test -count=1 ./services/wotan/internal/middleware -run 'TestBucketPerVerifiedCaller|TestNoIdentityFallsBackToIP|TestKeysDoNotCollide' 2>&1 | tail -4`. Red: the second caller is refused.
- [ ] **Step 180** [W] ~15m: Green. `func limitKey(r *http.Request) string`: `"sub:"+id.ServiceSubject()` when `auth.IdentityFromContext(r.Context())` is non-nil and its subject non-empty, else `"ip:"+clientIP(r.RemoteAddr)`. Rewrite the comment at lines 82-84. `ratelimit_ip_test.go` and the fuzz test keep passing (they send no identity).
- [ ] **Step 181** [B][V] ~3m: GO-GATE `./services/wotan/...` from the lane. Green.
- [ ] **Step 182** [W] ~15m: Red. `pkg/auth/audit_test.go`: `TestFailedAuditWriteCounted` (a writer that returns an error → `unheaded_auth_audit_write_failures_total` +1, the call does not panic or block); `TestShortAuditWriteCounted` (a writer that writes half and returns nil → +1).
- [ ] **Step 183** [B][V] ~1m: `go test -count=1 ./pkg/auth -run 'TestFailedAuditWrite|TestShortAuditWrite'`. Red.
- [ ] **Step 184** [W] ~10m: Green. In `Log`: `n, err := a.w.Write(data)`; `err != nil || n < len(data)` → `auditWriteFailures.Inc()`. The counter through `pkg/metrics/auto` with `prom` options. The marshal error at line 96 counts too.
- [ ] **Step 185** [B][V] ~3m: `cd /home/govan/tmp/wt/S-WASTE-OUT-wotan && go build ./... && ` GO-GATE `./pkg/auth/...`. Green. `import cycle not allowed` → the counter moves to a `pkg/auth/auditmetrics.go` that imports only `pkg/metrics/auto` and `prom`; still a cycle → `[STUCK]` for Steps 182-187, go on.
- [ ] **Step 186** [W] ~15m: `monitoring/prometheus/rules/compliance.yml`: rule `AuditWriteFailing`, `expr: sum by (job) (increase(unheaded_auth_audit_write_failures_total[15m])) > 0`, `for: 0m`, labels and annotations in the shape of the nearest rule in that file (severity `warning`, an `urgency` that mails, per decision 2), summary "An audit line could not be written (NIST AU-5)". A promtool case in the file's test under `rules/tests/` (one failure → fires; none → silent).
- [ ] **Step 187** [B][V] ~3m: `cd /home/govan/tmp/wt/S-WASTE-OUT-wotan && scripts/check-prometheus-rules.sh 2>&1 | tail -3`. Pass.
- [ ] **Step 188** [C] ~1m: **COMMIT** the named files: "Steps 177-188: Wotan rate-limits per verified caller; a failed audit write is counted and alerts".
- [ ] **Step 189** [W] ~5m: `wiki/Service-Wotan.md`: one line, "rate limit: one bucket per verified service identity; unauthenticated requests per source address". `PROG` "Log": the Phase 6-11 commits.
- [ ] **Step 190** [C][V] ~5m: **PHASE 11 EXIT GATE**: Steps 179 and 183 green; lane `wotan` merges.
  ```bash
  cd /home/govan/tmp/wt/S-WASTE-OUT && git merge --no-ff -S sprint/s-waste-out-wotan -m "[PLAN S-WASTE-OUT] Steps 81-190: Wotan lane (streams, one client, alerts.critical, discovery reader, one trace topic, rate limit)" || { git merge --abort; echo CONFLICT; }
  ```
  `CONFLICT` → `[STUCK]`; Phases 13 to 16 are `[BLOCKED]` until it merges.

---

## PHASE 12: PROMETHEUS KEEPS WHAT ITS RULES READ (D4 = A) (Steps 191-205) [P]

**Goal**: Prometheus stores, for 30 days, only the series a rule reads. A gate fails when a rule reads a series the keep-list lacks.
**Prerequisite**: Phase 3 gate; D4 = A. D4 = B or away → the phase is `[BLOCKED]` and its row says why.
**Time**: 2.5 h.
**Model**: Sonnet for Steps 191-199 (stop line: a rule the generator cannot parse is listed in `PROG`, never guessed); Opus for Steps 200-205. **Agent**: lane `mon`, worktree `/home/govan/tmp/wt/S-WASTE-OUT-mon`.

**How.** A `metric_relabel_configs` keep on `__name__` in every scrape job except `prometheus` and `alert-mailer` (remote_write sends those two whole to VictoriaMetrics, which cannot scrape them). The list is generated from `rules/*.yml`, never written by hand. Recording-rule outputs (names with a colon) and `ALERTS` are made by Prometheus, not scraped, so they need no entry.

- [ ] **Step 191** [B] ~1m: `cd /home/govan/tmp/unheaded && git worktree add /home/govan/tmp/wt/S-WASTE-OUT-mon -b sprint/s-waste-out-mon sprint/s-waste-out`
- [ ] **Step 192** [R] ~10m: `monitoring/prometheus/prometheus.yml` (12 jobs, the `remote_write` keep at the end); `scripts/check-prometheus-rules.sh`; `grep -c 'expr:' monitoring/prometheus/rules/*.yml` (122 on 2026-11-15; none uses `__name__`). Readers of Prometheus' own API: `cd /home/govan/tmp/wt/S-WASTE-OUT-mon && grep -rnE '127\.0\.0\.1:9090|:9090/api' --include=*.go --include=*.sh --include=*.py cmd pkg scripts | grep -v _test`. A reader that queries a series no rule reads → its names go in `monitoring/prometheus/keep-extra.txt`, one per line with `# reader: <file>`.
- [ ] **Step 193** [W] ~20m: Red. `scripts/testdata/prom-keep/rules/a.yml` holding one group with `expr: sum by (job) (rate(foo_total{a="b"}[5m])) / on(job) group_left bar_seconds_count > 0 and absent(baz) unless ignoring(x) qux:ratio5m`; `scripts/test-prom-keep-list.sh` runs `python3 scripts/gen-prom-keep-list.py --rules scripts/testdata/prom-keep/rules --print` and asserts the output is exactly `bar_seconds_count`, `baz`, `foo_total` (sorted; no `sum`, `rate`, `job`, `a`, `on`, `group_left`, `absent`, `ignoring`, `x`, no `qux:ratio5m`). A second case: a rule file with `{__name__=~"x.*"}` → exit 1 naming the file.
- [ ] **Step 194** [B][V] ~1m: `cd /home/govan/tmp/wt/S-WASTE-OUT-mon && scripts/test-prom-keep-list.sh; echo rc=$?`. Red: no generator.
- [ ] **Step 195** [W] ~40m: Green. `scripts/gen-prom-keep-list.py` (SPDX header; stdlib and PyYAML, which `scripts/check-runbooks.py` already uses). It reads each `expr`, drops string literals, the contents of `{...}`, of `[...]` and of `by|without|on|ignoring|group_left|group_right (...)`, takes every identifier matching `[a-zA-Z_][a-zA-Z0-9_]*` not followed by `(` and not a PromQL keyword (`and or unless offset bool by without on ignoring group_left group_right`), skips names with a colon, adds `keep-extra.txt`, sorts. Modes: `--print`; default writes `monitoring/prometheus/rule-series.txt` and rewrites, in `prometheus.yml`, the text between `# BEGIN rule-series (scripts/gen-prom-keep-list.py; do not edit)` and `# END rule-series`: an anchored block `metric_relabel_configs: &rule_series [{source_labels: [__name__], regex: '<a|b|...>', action: keep}]` in the first job and `metric_relabel_configs: *rule_series` in every other job but `prometheus` and `alert-mailer`; `--check` regenerates in memory and exits 1 with a diff when either file differs.
- [ ] **Step 196** [B][V] ~1m: Step 194's command. `rc=0`.
- [ ] **Step 197** [W] ~10m: Put the two marker comments in `prometheus.yml` (one pair per job that gets the block), then `python3 scripts/gen-prom-keep-list.py`. `rule-series.txt` gets an SPDX header comment line the generator writes and keeps.
- [ ] **Step 198** [B] ~2m: The config still loads.
  ```bash
  cd /home/govan/tmp/wt/S-WASTE-OUT-mon && IMG=$(awk '/image: *prom\/prometheus/ {print $2; exit}' monitoring/docker-compose.yml) && sudo docker run --rm --entrypoint promtool -v "$PWD/monitoring/prometheus:/etc/prometheus:ro" -v /home/govan/tmp/unheaded/configs/certs/dev/ca.pem:/etc/prometheus-tls/ca.pem:ro "$IMG" check config /etc/prometheus/prometheus.yml 2>&1 | tail -4; wc -l < monitoring/prometheus/rule-series.txt
  ```
- [ ] **Step 199** [V] ~1m: `SUCCESS` for the config and every rule file; the list holds 100 to 600 names (log the count). `FAILED` naming the relabel block → the anchor sits in a job the YAML parser reads after an alias: move the anchor to the first job in file order.
- [ ] **Step 200** [W] ~15m: The gate. `scripts/check-prometheus-rules.sh` runs `python3 "$REPO_ROOT/scripts/gen-prom-keep-list.py" --check` before promtool, and `scripts/test-prom-keep-list.sh`. In `scripts/check-gates-can-fail.sh` a provocation in the file's own pattern (`provoke_*` function, `backup`, the REGISTRY line `check-prometheus-rules|provoke_prom_keep_list|fast|a rule reads a series the keep-list lacks`): append to a rule file's last `expr` the text ` or provoke_unlisted_total`.
- [ ] **Step 201** [B] ~5m: `cd /home/govan/tmp/wt/S-WASTE-OUT-mon && git add -N scripts/gen-prom-keep-list.py scripts/test-prom-keep-list.sh && scripts/check-prometheus-rules.sh 2>&1 | tail -2; scripts/check-gates-can-fail.sh --only check-prometheus-rules 2>&1 | tail -3`
- [ ] **Step 202** [V] ~1m: The gate passes on the tree; the provoked tree fails it. Either other → read the message; two tries, then `[STUCK]`.
- [ ] **Step 203** [W] ~10m: `prometheus.yml` header: "It keeps 30 days of the series its rules read (rule-series.txt, generated); everything else is VictoriaMetrics' alone (ADR-147 decisions 9, 10)." `monitoring/docker-compose.yml` retention comment: the same clause. `wiki/` monitoring page (`grep -l 'rule_files\|Prometheus' wiki/*.md | head -3`, the one about the alert path): how to add a rule (regenerate the list; the gate says so).
- [ ] **Step 204** [C] ~1m: **COMMIT** by name: the generator, its test and fixture, `prometheus.yml`, `rule-series.txt`, `keep-extra.txt` if made, the two gate scripts, the wiki page: "Steps 191-204: Prometheus keeps only the series its rules read".
- [ ] **Step 205** [C][V] ~3m: **PHASE 12 EXIT GATE**: `--check` passes; promtool `SUCCESS`; provocation fails. Merge `sprint/s-waste-out-mon` into `sprint/s-waste-out` as Step 190 does (`CONFLICT` → `[STUCK]`).

---

## PHASE 13: DASHBOARD-BACKEND READS VICTORIAMETRICS; ONE POLLER; A LOGGED-OUT PAGE STOPS (Steps 206-232)

**Goal**: dashboard-backend keeps no time series of its own (decision 9), runs one health poller, and its page stops polling on 401.
**Prerequisite**: Phase 11 gate (lane `wotan` merged: Phases 7 and 10 changed the same files).
**Time**: 6 h.
**Model**: Opus (a large handler change; what the page reads must not change shape). **Agent**: lane `dash`, worktree `/home/govan/tmp/wt/S-WASTE-OUT-dash`.

- [ ] **Step 206** [B] ~1m: `cd /home/govan/tmp/unheaded && git worktree add /home/govan/tmp/wt/S-WASTE-OUT-dash -b sprint/s-waste-out-dash sprint/s-waste-out`
- [ ] **Step 207** [R] ~30m: In `cmd/dashboard-backend`: every `s.scraper.` and `s.metricsAggregator.` call in `internal/server/server.go` (14 and 5 on 2026-11-15: lines 843-852, 1025, 1037, 1064, 1114, 1142, 1259, 1505-1507, 1519, 1739, 1753, 1795-1799, 2473, 2528, 2969); `internal/scraper/summary.go` (the `PanelSummary` fields); `serviceFlowGenerator` (`server.go:2191-2252`, `serviceLatencies` at 291); `collectHostMetrics` and `doCollectHostMetrics` (`server.go:2765-2990`). In `static/dashboard.js` and `static/js/*.js`, which JSON fields each consumer of `/api/v1/metrics`, `/api/v1/metrics/summary`, `/api/v1/metrics/query`, `/api/v1/services`, `/api/v1/hosts`, `/api/v1/aggregated` and the latency endpoint reads. Write the list (endpoint → fields) to `/home/govan/tmp/wt/S-WASTE-OUT-scratch/dash-fields.txt`.
- [ ] **Step 208** [B] ~3m: The memory evidence the seat lacked (no live profile: no restart before the E-GATE).
  ```bash
  cd /home/govan/tmp/wt/S-WASTE-OUT-dash && go test -count=1 -v -run 'Memory|Retain|Bound' ./cmd/dashboard-backend/internal/scraper 2>&1 | grep -E '^(=== RUN|--- |\s+\S+_test.go)' | head -20
  ```
  Log in `PROG` (W-13) the bytes per series the test reports; with 7,000 series that is the store's share of the 289 MiB.
- [ ] **Step 209** [R] ~10m: Who reads the dashboard's own host gauges (Approach A): `grep -nE 'Name: *"[a-z_]*host[a-z_]*"' cmd/dashboard-backend/internal/server/*.go | grep -v _test`, then each name in `config/grafana monitoring/prometheus/rules config/victoria cmd pkg scripts`. Log every reader. A reader → in Step 218 it is repointed to the node_exporter or huginn series for the same quantity, never left reading a series that stops.
- [ ] **Step 210** [W] ~20m: Red. `pkg/vmquery/vmquery_test.go` against an `httptest` VictoriaMetrics: `TestInstantVector` (decodes `metric` and `value`), `TestRange` (`query_range`, step and window passed), `TestAnswerCapped` (a body over the cap → error, nothing decoded), `TestNon200` (500 → error naming the status), `TestTimeout` (a slow server → `context.DeadlineExceeded` within the timeout), `TestNoBaseURL` (empty → `ErrNoVictoria`).
- [ ] **Step 211** [B][V] ~1m: `cd /home/govan/tmp/wt/S-WASTE-OUT-dash && go test -count=1 ./pkg/vmquery 2>&1 | tail -3`. Red: no package.
- [ ] **Step 212** [W] ~30m: Green. `pkg/vmquery`: `New(baseURL string, client *http.Client) (*Client, error)`, `Instant(ctx, promql) ([]Sample, error)`, `Range(ctx, promql, start, end, step) ([]Series, error)`; 5 s timeout, a 4 MiB answer cap, `net/url` encoding only. `pkg/sloguard/sloguard.go:173-200` (`query`) calls `vmquery.Instant` instead of its own copy; its tests stay green.
- [ ] **Step 213** [B][V] ~3m: GO-GATE `./pkg/vmquery/... ./pkg/sloguard/...` from the lane. Green.
- [ ] **Step 214** [C] ~1m: **COMMIT**: "Steps 206-214: pkg/vmquery; sloguard uses it".
- [ ] **Step 215** [W] ~40m: Red, in `cmd/dashboard-backend/internal/server` with a fake VictoriaMetrics that answers fixed series:
  - `TestMetricsEndpointsReadVictoria`: each endpoint of `dash-fields.txt` returns every field the page reads, with the fake's values.
  - `TestVictoriaDownIsVisible`: the fake answers 500 → those endpoints answer 503 `{"error":"victoriametrics unavailable"}`, never 200 with zeros.
  - `TestHostsFromVictoria`: `/api/v1/hosts` lists the hosts the fake's `node_memory_MemTotal_bytes` names.
  - `TestNoTargetScrapes`: a fake target counts `GET /metrics` and `GET /health`; after the server runs 3 s it saw no `/metrics` GET and no `/health` GET at a 2 s period.
- [ ] **Step 216** [B][V] ~2m: `go test -count=1 ./cmd/dashboard-backend/internal/server -run 'TestMetricsEndpointsReadVictoria|TestVictoriaDownIsVisible|TestHostsFromVictoria|TestNoTargetScrapes' 2>&1 | tail -6`. Red.
- [ ] **Step 217** [W] ~90m: Green. One file, `internal/server/vmqueries.go`, holds every PromQL the dashboard runs, one constant per field of `dash-fields.txt`, for example: request rate `sum by (service) (rate(unheaded_http_requests_total[1m]))`; errors the same with `status=~"5.."`; p50 and p99 `histogram_quantile(0.99, sum by (service, le) (rate(unheaded_http_request_duration_seconds_bucket[5m])))`; memory `process_resident_memory_bytes`; CPU `rate(process_cpu_seconds_total[1m])`; host CPU `1 - avg by (host) (rate(node_cpu_seconds_total{mode="idle"}[1m]))`; host memory `node_memory_MemAvailable_bytes` and `node_memory_MemTotal_bytes`. The handlers build the same JSON from `vmquery` answers. The latency endpoint uses BPF data when it has it, else the p50/p99 query.
- [ ] **Step 218** [W] ~30m: Delete: `internal/scraper/` (all of it); `internal/metrics/` if Step 207 found no writer but `server.go:2473` and nothing reads its series but the status counts (else keep it and log why); `serviceFlowGenerator`, `serviceLatencies` and their test fixtures; `collectHostMetrics`, its three approaches and their gauges (readers from Step 209 repointed first). The status JSON loses `scraper_series`, `metrics_series`, `series_count`. `main.go:112` help text: "VictoriaMetrics base URL every metric is read from". The health monitor (`health/health.go:444`, 15 s) is the one poller left.
- [ ] **Step 219** [B][V] ~5m: GO-GATE `./cmd/dashboard-backend/... ./pkg/vmquery/...` from the lane, then `go build ./...`. Green.
- [ ] **Step 220** [D] ~20m: A test that used scraper fixtures → it is rewritten against the fake VictoriaMetrics, or deleted with the code it tested (named in `PROG`). A JSON shape test fails → the field names come from `dash-fields.txt`, not from the old struct. Two tries, then `[STUCK]` (the lane stays unmerged; W-13 and W-14 wait).
- [ ] **Step 221** [C] ~1m: **COMMIT**: "Steps 215-221: dashboard-backend reads VictoriaMetrics; its scraper, second store and 2 s health poller are gone".
- [ ] **Step 222** [W] ~25m: Red. Read `cmd/dashboard-backend/jstest/harness.cjs` (routes, `settle`, timers). New `jstest/logged_out.cjs` (script `../static/dashboard.js`): every API route answers `{__status: 401}`; after `settle()` and advancing the clock past the longest refresh interval twice, the count of fetches equals the first round (nine) and the page's connection-status element (find its id with `grep -n 'status' static/dashboard.js | head`) says the session ended. If the harness has no clock control, add `advance(ms)` to it, used by this test only.
- [ ] **Step 223** [B][V] ~1m: `cd /home/govan/tmp/wt/S-WASTE-OUT-dash && go test -count=1 ./cmd/dashboard-backend/jstest/ 2>&1 | tail -4`. Red on `logged_out.cjs`.
- [ ] **Step 224** [W] ~20m: Green. `init` keeps the ids `setInterval` returns; `fetchJSON` on status 401 calls `stopPolling()` once: clears every interval, closes `/ws` and stops its reconnect, writes "Logged out: log in and reload" in the existing status element. No other change to the page (series rule 22).
- [ ] **Step 225** [B][V] ~1m: Step 223's command → `ok`; the other `.cjs` checks still pass.
- [ ] **Step 226** [W] ~5m: `docker-compose.yml`, `dashboard-backend`: pass `--vm-url=http://victoria:8428` the way its other flags are passed (its `command`, or the environment variable `main.go` maps to the flag: read `main.go:100-130`), with the comment `# ADR-147 decision 9: every graph reads VictoriaMetrics (shared network: observe)`.
- [ ] **Step 227** [B] ~3m: `cd /home/govan/tmp/wt/S-WASTE-OUT-dash && for g in check-compose-hardening.sh check-http-timeouts.sh check-netip.sh check-gosec-ratchet.sh; do printf '%s: ' $g; scripts/$g >/dev/null 2>&1 && echo PASS || echo FAIL; done`
- [ ] **Step 228** [V] ~1m: Four `PASS`.
- [ ] **Step 229** [W] ~10m: The dashboard's wiki page (`grep -l 'dashboard-backend' wiki/*.md | head -3`; the service page): what it reads (VictoriaMetrics for every metric, Wotan for events, its health monitor every 15 s), what it no longer keeps, the 503 when VictoriaMetrics is down, the page stopping on a logged-out session.
- [ ] **Step 230** [C] ~1m: **COMMIT**: "Steps 222-230: a logged-out dashboard page stops polling; compose passes the VictoriaMetrics URL".
- [ ] **Step 231** [W] ~3m: `PROG`: W-13 to W-16, "code done in `sprint/s-waste-out-dash`; after-readings in Phase 20".
- [ ] **Step 232** [C][V] ~3m: **PHASE 13 EXIT GATE**: the four server tests, the `vmquery` tests and `logged_out.cjs` green. Merge `sprint/s-waste-out-dash` into `sprint/s-waste-out` as Step 190 does.

---

## PHASE 14: QUIET HEALTH, A LIGHTER AKIRA, A HEARTBEAT THAT IS NOT A COMMIT, THE RUNTIME TOLD (Steps 233-262)

**Goal**: Probe paths log at debug. Akira checks certificates hourly, reaps its children, and every container is probed every 30 s. A steady health report writes the Well at most every 5 minutes, without a WAL flush. Every Go service exports its GOMAXPROCS, memory limit and GOGC, and every first-party container tells the runtime its limit.
**Prerequisite**: Phase 11 gate (lane `wotan` merged).
**Time**: 5 h.
**Model**: Opus. **Agent**: Coordinator or one agent in `WT`; may run beside Phases 13 and 15 (different files; the compose edits here and in Phase 13 touch different services).

### Probe paths at debug

- [ ] **Step 233** [R] ~10m: `cd /home/govan/tmp/wt/S-WASTE-OUT && grep -rnE 'Msg\("(request|http_request|request_complete|request completed)"\)' --include=*.go cmd services pkg | grep -v _test`; for each hit, the level it logs at. `cmd/kanban-app/main.go:439-457` logs every request at info. Wotan's logs at debug (`services/wotan/internal/middleware/middleware.go:95`).
- [ ] **Step 234** [W] ~15m: Red. `pkg/httputil/accesslog_test.go` (a zerolog writer into a buffer): `TestProbePathsAtDebug` (`/health`, `/ready`, `/live`, `/metrics` answered 200 → a debug line; `/api/v1/tasks` → info); `TestFailingProbeStillWarns` (`/health` answered 503 → warn); `TestAccessLogFields` (method, path, status, duration, request id as kanban writes them today).
- [ ] **Step 235** [B][V] ~1m: `go test -count=1 ./pkg/httputil -run 'TestProbePaths|TestFailingProbe|TestAccessLogFields'`. Red.
- [ ] **Step 236** [W] ~15m: Green. `pkg/httputil/accesslog.go`: `func AccessLog(next http.Handler, l zerolog.Logger, requestID func(context.Context) string) http.Handler`. A new file beside `middleware.go`, which plan 1 edits (Busboy).
- [ ] **Step 237** [W] ~15m: `cmd/kanban-app/main.go`: `loggingMiddleware` becomes `httputil.AccessLog(next, log.Logger, getRequestID)`; the old function and its `statusWriter` go if nothing else uses them. Every other hit of Step 233 that logs probe paths at info uses the same.
- [ ] **Step 238** [B][V] ~3m: GO-GATE `./pkg/httputil/... ./cmd/kanban-app/...`. Green.
- [ ] **Step 239** [C] ~1m: **COMMIT**: "Steps 233-239: probe paths log at debug (pkg/httputil.AccessLog)".

### Akira and the probes

- [ ] **Step 240** [W] ~5m: (D5 = A.) `config/akira/akira.yaml`, target `cert-expiry`: `interval: 1h`; the comment above it becomes "hourly, as the hosts run it; a certificate issued short is noticed within the hour (ADR-147, Developer D8: every 30 s cost 2,880 interpreter starts a day in a 0.10-core container)". `configs/akira-east.yaml`: the same if it has the check. D5 = B → this step is `[BLOCKED]`; the Go parser is a parked item.
- [ ] **Step 241** [W] ~3m: `docker-compose.yml`, `akira`: `init: true` under `container_name`, comment `# reaps the exec checks' orphans; Akira is otherwise PID 1 (18 zombies 2026-11-15)`.
- [ ] **Step 242** [W] ~10m: `docker-compose.yml`: every healthcheck whose `test` runs `/usr/local/bin/healthprobe` gets `interval: 30s` and `retries: 3` (timeout and start_period unchanged), with one comment at the first: `# 30 s x 3: unhealthy after 90 s (was 10 s x 5, 50 s); ADR-147 Archimedes F4: a probe cost more CPU than a small service`.
- [ ] **Step 243** [B][V] ~1m:
  ```bash
  cd /home/govan/tmp/wt/S-WASTE-OUT && python3 -c 'import yaml; s=yaml.safe_load(open("docker-compose.yml"))["services"]; bad=[k for k,v in s.items() if "healthprobe" in str(v.get("healthcheck",{}).get("test","")) and (v["healthcheck"].get("interval")!="30s" or v["healthcheck"].get("retries")!=3)]; print("bad", bad, "akira init", s["akira"].get("init"))'
  ```
  Expected `bad [] akira init True`.

### The heartbeat

- [ ] **Step 244** [W] ~25m: Red. `pkg/database/heartbeat_test.go`, an injected clock, no database:
  - `TestHeartbeatWritesOnChange`: first report → write; same status 10 s later → skip; status changed → write (transition); same status 5 min 1 s after the last write → write (heartbeat).
  - `TestHeartbeatPerRow`: two reporters for one service are two rows, each with its own clock.
  - `TestHeartbeatBounded`: past 4,096 rows a new row always writes and `unheaded_well_health_heartbeat_rows_full_total` counts it.
  - `TestHeartbeatOnlyIsAsync`: the statement list for a heartbeat-only write starts with `SET LOCAL synchronous_commit = off`; for a transition it does not.
- [ ] **Step 245** [B][V] ~1m: `go test -count=1 ./pkg/database -run 'TestHeartbeat'`. Red.
- [ ] **Step 246** [W] ~40m: Green. `pkg/database/heartbeat.go`: `heartbeatGate` (map keyed service and reporter → last written status and time; injectable clock; cap 4,096). `HandleHealthReport` (`bridge.go:62-118`) asks the gate before `BeginTx`; a skipped report increments `unheaded_well_health_reports_skipped_total`; a heartbeat-only write runs `SET LOCAL synchronous_commit = off` first inside its transaction; a transition stays synchronous. Comment: "last_check is up to 5 minutes old for a service whose status has not changed (ADR-147, Erda F3: 0.68 commits a second, 73% of the WAL flushes)". `scripts/well/test-health-bridge.sh`: a copy of `scripts/well/test-api-keys.sh`'s throwaway-server setup that runs `go test -count=1 -run TestIntegrationHealthBridge ./pkg/database` with `WELL_TEST_OPS_DSN` (not run now: it starts a container; Step 321 runs it).
- [ ] **Step 247** [R] ~5m: Readers of `last_check`: `grep -rn 'last_check' --include=*.go --include=*.sql --include=*.json --include=*.yml cmd pkg config monitoring scripts | grep -v _test | grep -v 'db/migrations'`. On 2026-11-15 only the writer and the unused `HealthStore.GetAllCurrent`. A reader with a freshness bound under 5 minutes → log it in `PROG` and raise it under "For Stevie"; the bound is not changed here.
- [ ] **Step 248** [B][V] ~3m: GO-GATE `./pkg/database/... ./cmd/dashboard-backend/...`. Green (the integration test skips without its DSN).
- [ ] **Step 249** [C] ~1m: **COMMIT**: "Steps 244-249: a steady health report writes the Well at most every 5 min, without a WAL flush".

### The runtime told

- [ ] **Step 250** [W] ~15m: Red. Read `pkg/metrics/prom/parity_test.go` first (names it compares with client_golang). `pkg/metrics/gocollector_test.go`: `TestGoCollectorRuntimeLimits`: a gather holds `go_sched_gomaxprocs_threads` equal to `runtime.GOMAXPROCS(0)`, `go_gc_gomemlimit_bytes` equal to `debug.SetMemoryLimit(-1)`, `go_gc_gogc_percent` equal to 100 with `GOGC` unset.
- [ ] **Step 251** [B][V] ~1m: `go test -count=1 ./pkg/metrics -run TestGoCollectorRuntimeLimits`. Red.
- [ ] **Step 252** [W] ~20m: Green. Add the keys `/sched/gomaxprocs:threads`, `/gc/gomemlimit:bytes`, `/gc/gogc:percent` to `goRMKeys` (`gocollector.go:78-83`) and three gauge families with client_golang's names and help text.
- [ ] **Step 253** [B][V] ~3m: GO-GATE `./pkg/metrics/...`. Green, parity test included.
- [ ] **Step 254** [W] ~10m: `docker-compose.yml`: `GOMEMLIMIT` at 85% of the memory limit, rounded down to MiB, on every first-party Go service that lacks it: timeguru `217MiB` (256M), akira `54MiB` (64M), kanban-app `108MiB` (128M), cuirass `217MiB` (256M), auth `217MiB` (256M), sword `108MiB` (128M). Comment at the first: `# GOMEMLIMIT = 85% of the limit: the runtime collects before the kernel kills (ADR-147, Developer D22)`. alert-mailer got its own in Step 136.
- [ ] **Step 255** [W] ~20m: The gate. `scripts/check-unit-ceilings.sh` gains a compose section: in `docker-compose.yml` and `monitoring/docker-compose.yml`, every service with `build.target` (built from this repository's Dockerfile: our Go binaries) and a memory limit must set `GOMEMLIMIT` to a value below that limit; a failure prints `FAIL <file>: <service> ...`; the OK line counts units and services. In `scripts/check-gates-can-fail.sh`: `provoke_unit_ceilings_compose` (delete akira's `GOMEMLIMIT` line) and its REGISTRY line `check-unit-ceilings|provoke_unit_ceilings_compose|fast|GOMEMLIMIT removed from the akira service`.
- [ ] **Step 256** [B] ~3m: `cd /home/govan/tmp/wt/S-WASTE-OUT && scripts/check-unit-ceilings.sh 2>&1 | tail -2; git stash list | wc -l; scripts/check-gates-can-fail.sh --only check-unit-ceilings 2>&1 | tail -3` (commit Steps 254-255 first if the provocation needs a clean tree).
- [ ] **Step 257** [V] ~1m: The gate passes on the tree and fails on both provocations (the unit one and the compose one).
- [ ] **Step 258** [C] ~1m: **COMMIT**: "Steps 250-258: GOMAXPROCS, GOMEMLIMIT and GOGC exported; every first-party container tells the runtime its limit; the ceiling gate reads compose".
- [ ] **Step 259** [W] ~10m: `wiki/Profiling.md`: the three runtime series. `CLAUDE.md`, ratchet list: `check-unit-ceilings.sh` "(`MemoryMax` + `GOMEMLIMIT` on every Go unit and every first-party compose service)". The Akira wiki page: hourly certificate check, `init`, 30 s probes.
- [ ] **Step 260** [B] ~5m: `cd /home/govan/tmp/wt/S-WASTE-OUT && go build ./... && go vet ./... 2>&1 | tail -3 && ./scripts/check-gosec-ratchet.sh 2>&1 | tail -2 && scripts/check-compose-hardening.sh >/dev/null 2>&1 && echo PASS`
- [ ] **Step 261** [V] ~1m: Clean vet, gosec within its baseline, `PASS`.
- [ ] **Step 262** [C][V] ~1m: **PHASE 14 EXIT GATE**: Steps 238, 243, 248, 253 and 257 green. Commit the wiki and `CLAUDE.md` by name.

---

## PHASE 15: `FLOW_STATE` BOUNDED AND COUNTED; `FLOW_EVENTS` GONE (D7 = A, D6 = A) (Steps 263-282) [P]

**Goal**: packet-marker's `FLOW_STATE` is an LRU whose failed inserts are counted and visible. flow-tracker writes no ring nobody drains.
**Prerequisite**: Phase 11 gate (Phase 10 changed `cmd/trace-collector-go`). D7 = B → Step 268 deletes the map and its write instead. D6 away → Step 269 and the flow-tracker halves of Steps 265 and 271 are `[BLOCKED]`.
**Time**: 3 h.
**Model**: Opus (eBPF, verifier, root). **Agent**: lane `ebpf`, worktree `/home/govan/tmp/wt/S-WASTE-OUT-ebpf`.

- [ ] **Step 263** [B] ~1m: `cd /home/govan/tmp/unheaded && git worktree add /home/govan/tmp/wt/S-WASTE-OUT-ebpf -b sprint/s-waste-out-ebpf sprint/s-waste-out`
- [ ] **Step 264** [R] ~15m: `ebpf/packet-marker/src/main.rs:30-60, 314-342` (`FLOW_STATE`, `STATS` keys 0-7, `update_flow_state`); `ebpf/flow-tracker/src/main.rs:40-75, 260-390, 564-585` (`FLOW_EVENTS`, five `send_flow_event` calls, keys 7 and 8); `ebpf/common/src/lib.rs:237` (`MAX_FLOWS` 65,536); users of `FlowEvent` and `FlowEventType`: `grep -rn 'FlowEvent\b\|FlowEventType' ebpf cmd --include=*.rs --include=*.go`; `cmd/trace-collector-go/loader_native.go:200-215`, `loader.go:31-35` (the comment at 34 says `FLOW_STATE` and the reader opens flow-tracker's `FLOWS`), `flow_stats.go`; `latency_kprobe_test.go:75-110` (`TestBuiltLatencyProbeSpec`, the model).
- [ ] **Step 265** [W] ~20m: Red. `cmd/trace-collector-go/built_objects_test.go`, modelled on `TestBuiltLatencyProbeSpec` (skip unless the env names an object):
  - `TestBuiltPacketMarkerFlowStateIsLRU` (`PACKET_MARKER_OBJ`): `FLOW_STATE` is `ebpf.LRUHash`, max 65,536; `STATS` max at least 9.
  - `TestBuiltFlowTrackerHasNoEventsRing` (`FLOW_TRACKER_OBJ`): the spec has no `FLOW_EVENTS` map.
- [ ] **Step 266** [B] ~5m: Build both objects.
  ```bash
  cd /home/govan/tmp/wt/S-WASTE-OUT-ebpf/ebpf && CARGO_BUILD_JOBS=4 CARGO_TARGET_DIR=target/load-gate/default cargo build --release -p packet-marker -p flow-tracker 2>&1 | tail -2
  ```
- [ ] **Step 267** [B][V] ~2m: Red against the current code.
  ```bash
  cd /home/govan/tmp/wt/S-WASTE-OUT-ebpf && O=ebpf/target/load-gate/default/bpfel-unknown-none/release && PACKET_MARKER_OBJ=$PWD/$O/packet-marker FLOW_TRACKER_OBJ=$PWD/$O/flow-tracker go test -count=1 ./cmd/trace-collector-go -run 'TestBuiltPacketMarker|TestBuiltFlowTracker' 2>&1 | tail -5
  ```
  Expected: both fail (a hash, and a `FLOW_EVENTS` map).
- [ ] **Step 268** [W] ~15m: packet-marker. `FLOW_STATE: LruHashMap<FlowKey, FlowState>` (import from `aya_ebpf::maps`), comment `/// LRU (ADR-147 decision 19): written per flow, read by nothing; eviction bounds it, a failed insert is counted (STATS key 8).`; `const STAT_FLOW_STATE_INSERT_FAILED: u32 = 8;`; in `update_flow_state` the `let _ = FLOW_STATE.insert(...)` becomes `if FLOW_STATE.insert(flow_key, &state, 0).is_err() { increment_stat(STAT_FLOW_STATE_INSERT_FAILED); }`.
- [ ] **Step 269** [W] ~15m: (D6 = A.) flow-tracker. Delete `FLOW_EVENTS`, `send_flow_event` and its five calls (the `increment_stat` calls beside them stay). Above the stat constants: `// 7, 8 retired 2026-11: FLOW_EVENTS removed, 94% of its events were dropped and nothing read it (ADR-147, BlackMage F8). Not reused.` and delete `STAT_EVENTS_SENT`, `STAT_EVENTS_DROPPED`. `FlowEvent` and `FlowEventType` in `ebpf/common` go if Step 264 found no other user.
- [ ] **Step 270** [B][V] ~5m: Step 266, then Step 267's command. Both tests `ok`.
- [ ] **Step 271** [W] ~30m: The collector, red then green. `flow_stats.go`: the two fields for keys 7 and 8 leave `FlowStats` (D6 = A). A new gauge `trace_collector_flow_state_insert_failures` set each stats tick from packet-marker's `STATS` key 8 (`n.mapReader("packet_marker", "STATS")` beside `GetStatsMap`; a mock loader without it reports nothing, never 0). `loader.go:34`: the comment names flow-tracker's `FLOWS`. Tests: `TestFlowStatsHasNoEventFields`, `TestFlowStateFailuresGauge` (a mock `MapReader` with key 8 = 3 → the gauge reads 3).
- [ ] **Step 272** [B][V] ~3m: GO-GATE `./cmd/trace-collector-go/...` from the lane. Green.
- [ ] **Step 273** [S][B] ~10m: The verifier, with the repository's own gate (it loads, never attaches).
  ```bash
  cd /home/govan/tmp/wt/S-WASTE-OUT-ebpf && sudo EBPF_INSNS_OUT=/home/govan/tmp/wt/S-WASTE-OUT-scratch/insns.tsv scripts/check-ebpf-loads.sh 2>&1 | tail -8
  ```
- [ ] **Step 274** [V] ~1m: `PASS` lines for `packet-marker/packet_marker`, `flow-tracker/flow_tracker_ingress` and `flow_tracker_egress`; the Go loader check passes.
  - A refusal naming `FLOW_STATE` → Step 275.
- [ ] **Step 275** [D] ~10m: LRU hash maps load on 6.17 with no flag (S-WATCH-TRUE's latency-probe moved to one). Read the verifier line: a refusal on `get_ptr_mut` of an LRU map is the usual cause; use `get_ptr_mut` as `latency-probe` does now (`grep -n 'LruHashMap\|get_ptr' ebpf/latency-probe/src/main.rs`). Two tries, then `[STUCK]`: keep the hash and add only the counted insert (Appendix A, procedure 6).
- [ ] **Step 276** [B] ~2m: `cd /home/govan/tmp/wt/S-WASTE-OUT-ebpf && scripts/check-ebpf-insns.sh --from /home/govan/tmp/wt/S-WASTE-OUT-scratch/insns.tsv 2>&1 | tail -5`
- [ ] **Step 277** [V] ~1m: Pass. A shrink or a reviewed growth under 10% for the three programs → `scripts/check-ebpf-insns.sh --update --from /home/govan/tmp/wt/S-WASTE-OUT-scratch/insns.tsv` and the baseline diff goes in the next commit. Growth over 10% → `[STUCK]` with the numbers.
- [ ] **Step 278** [R] ~5m: `grep -rn 'events_sent\|events_dropped' cmd/dashboard-backend cmd/trace-collector-go --include=*.go --include=*.js | grep -v _test`. The server stops forwarding fields that no longer exist; the page renders a missing number as `--` (`fixedOrDash`); no page edit.
- [ ] **Step 279** [W] ~10m: The eBPF wiki page (`grep -l 'flow-tracker\|packet-marker' wiki/*.md | head -3`): `FLOW_STATE` is an LRU with no reader, its failures on `trace_collector_flow_state_insert_failures`; `FLOW_EVENTS` removed; flows reach userspace through `FLOWS` and its batch reader.
- [ ] **Step 280** [C] ~1m: **COMMIT** by name: the two `main.rs`, `ebpf/common` if changed, the collector files and tests, the baseline if updated, the wiki page: "Steps 263-280: FLOW_STATE is a counted LRU; FLOW_EVENTS removed".
- [ ] **Step 281** [W] ~2m: `PROG` "Handoffs", plan 6: east runs the 2026-10-18 `packet-marker` and `flow-tracker` (its `FLOW_STATE` is full); install this pair with east's collector in Evening 10B.
- [ ] **Step 282** [C][V] ~3m: **PHASE 15 EXIT GATE**: the built-object tests, the collector tests, the loads and the instruction ratchet pass. Merge `sprint/s-waste-out-ebpf` into `sprint/s-waste-out` as Step 190 does.

---

## PHASE 16: TIMEGURU AND KANBAN, ONE HOME EACH (Steps 283-304)

**Goal**: timeguru keeps its milestones in the Well's `timeline_milestones` and has no SQLite. kanban has no SQLite fallback: without the Well it waits, read-only, and says so.
**Prerequisite**: Phase 14 gate (the same `cmd/kanban-app/main.go`).
**Time**: 5 h.
**Model**: Opus (a migration, two start-up paths). **Agent**: Coordinator in `WT`.

- [ ] **Step 283** [R] ~20m: `services/timeguru/internal/storage/storage.go` (the one-row `timeline` blob, `milestone_history` written and never read), `internal/timeline/timeline.go:33-60` (`Milestone`, its `Validate`), `internal/api/handlers.go:40-60, 440-490`, `cmd/timeguru/main.go:80-120, 300-380`; `db/migrations/003_app_schema.sql:58-82, 110-125`, `025_well_owner_roles.sql`, `049_app_kanban_zhen_optin.sql` (the header a migration carries, its owner and grants), `db/migrations/MANIFEST`; `pkg/database/config.go:125-160`; `cmd/kanban-app/main.go:160-205, 1560-1600`, `store.go`.
- [ ] **Step 284** [W] ~40m: `db/migrations/050_app_timeline_milestones.sql`, header in 049's form (writer timeguru as `app_timeguru`; readers kanban, Zhen; volume: one row per milestone of `references/timeline.md`, under a hundred; retention: rows follow the file; lock plan: the table is empty, so drop and create). Body:
  ```sql
  DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM timeline_milestones) THEN
      RAISE EXCEPTION 'timeline_milestones is not empty: read it before 050 replaces it';
    END IF;
  END $$;
  DROP TABLE timeline_milestones;
  CREATE TABLE timeline_milestones (
      milestone_key  VARCHAR(64) PRIMARY KEY,
      name           TEXT NOT NULL CHECK (length(name) <= 500),
      status         VARCHAR(20) NOT NULL CHECK (status IN ('pending', 'in_progress', 'completed', 'blocked')),
      progress_pct   SMALLINT NOT NULL DEFAULT 0 CHECK (progress_pct BETWEEN 0 AND 100),
      eta            TIMESTAMPTZ,
      owner          TEXT NOT NULL DEFAULT '',
      risk           VARCHAR(10) NOT NULL DEFAULT '' CHECK (risk IN ('', 'low', 'medium', 'high')),
      description    TEXT NOT NULL DEFAULT '',
      updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
  );
  ```
  then the owner as 025 sets it for `unheaded_app` tables, and the grants of 003 lines 116-121 for this table (SELECT for `app_kanban` and `app_zhen`; SELECT, INSERT, UPDATE, DELETE for `app_timeguru`). The key's `CHECK` follows `Milestone.Validate`'s id rule (add one if `Validate` has it).
- [ ] **Step 285** [W] ~2m: `db/migrations/MANIFEST`: `unheaded_app     050_app_timeline_milestones.sql` after the 049 line.
- [ ] **Step 286** [B] ~2m: `cd /home/govan/tmp/wt/S-WASTE-OUT && python3 scripts/well/well_lint.py db/migrations/050_app_timeline_milestones.sql 2>&1 | tail -3` (read its usage line first if it takes no path). `scripts/well/test-migrate.sh` starts a server in a container: it runs at Step 321.
- [ ] **Step 287** [V] ~1m: Lint clean.
- [ ] **Step 288** [W] ~30m: `pkg/database`: `AppTimeguruConfig()` (as `AppKanbanConfig`, default user `app_timeguru`, database `unheaded_app`, no default password) and `TimelineStore` with `ReplaceMilestones(ctx, rows []MilestoneRow) error` (one transaction: upsert every row, delete rows not in the list), `UpdateMilestone(ctx, key string, progress int, status string) error`, `NewerThan(ctx, t time.Time) (map[string]MilestoneRow, error)`.
- [ ] **Step 289** [W] ~30m: Red. `services/timeguru` against a fake of that store:
  - `TestFileIsTheSource`: loading a fixture `timeline.md` calls `ReplaceMilestones` with every milestone.
  - `TestUpdateSurvivesRestartWhenNewer`: a row updated after the file's modification time keeps its status and progress over the file's at start; a row older than the file takes the file's.
  - `TestWellDownServesReadOnly`: the store errors at start → `GET /timeline` serves the parsed file, `POST /milestones/{id}/update` answers 503 `{"error":"the Well is unavailable"}`, `/health` detail `"well":"waiting"`, and a retry every 1 s to 30 s connects later.
  - `TestNoSQLite` (policy): no non-test file under `services/timeguru` imports `modernc.org/sqlite`.
- [ ] **Step 290** [B][V] ~1m: `go test -count=1 ./services/timeguru/... 2>&1 | tail -6`. Red.
- [ ] **Step 291** [W] ~60m: Green. `internal/storage` becomes the Well store behind the handler's existing interface (`handlers.go:47`); the timeline is held in memory, rebuilt from the file at start and on change, with `NewerThan` overlaid; the SQLite schema, `milestone_history` and the `DB_PATH` setting go. `main.go` connects with `database.Connect(ctx, database.AppTimeguruConfig())` and `pkg/database.RequireMigrations` for 050, as kanban requires its own.
- [ ] **Step 292** [B][V] ~3m: GO-GATE `./services/timeguru/... ./pkg/database/...`. Green.
- [ ] **Step 293** [W] ~10m: `docker-compose.yml`, `timeguru`: delete `DB_PATH` and the `/data` tmpfs; add `WELL_HOST: "postgres"`, `WELL_USER: "app_timeguru"`, `WELL_PASSWORD: "${APP_TIMEGURU_PASSWORD:?set APP_TIMEGURU_PASSWORD in .env (credentials are not stored in this repo)}"`, `WELL_DB: "unheaded_app"`, `WELL_SSLMODE` and `WELL_SSLROOTCERT` and the CA mount exactly as `kanban-app` has them, and `depends_on: postgres: {condition: service_healthy}`; comment `# ADR-147: one home, the Well (migration 050); needs app_timeguru's password (Step 340)`.
- [ ] **Step 294** [C] ~1m: **COMMIT**: "Steps 283-294: timeguru keeps its milestones in the Well (migration 050); no SQLite".
- [ ] **Step 295** [W] ~25m: Red, `cmd/kanban-app/well_wait_test.go`:
  - `TestNoSQLiteFallback` (policy): no non-test file in `cmd/kanban-app` imports `modernc.org/sqlite` or contains `kanban.db`.
  - `TestWellLateArrival`: a connector that fails twice then succeeds → before it, task routes answer 503 `{"error":"the Well is unavailable"}` and `/health` detail says `"well":"waiting"`; after it, 200.
  - `TestWellNotMigratedStillFatal`: the existing refusal (`main.go:182`) is unchanged.
- [ ] **Step 296** [B][V] ~1m: `go test -count=1 ./cmd/kanban-app -run 'TestNoSQLiteFallback|TestWellLateArrival|TestWellNotMigrated'`. Red.
- [ ] **Step 297** [W] ~40m: Green. Both start paths (`main.go:180-205` and `1565-1595`) use one `waitForWell(ctx, connect, 1*time.Second, 30*time.Second)`; `openSQLiteStore` and its file go (`grep -rn 'openSQLiteStore\|sqliteStore' cmd/kanban-app`); `Config.DataDir` and `DATA_DIR` go if nothing else uses them. `docker-compose.yml`, `kanban-app`: delete `DATA_DIR` and the `kanban-data:/data` mount; under `volumes:` delete `kanban-data:` (the volume itself is removed in Phase 22).
- [ ] **Step 298** [B][V] ~3m: GO-GATE `./cmd/kanban-app/...`. Green.
- [ ] **Step 299** [B] ~3m: `cd /home/govan/tmp/wt/S-WASTE-OUT && scripts/check-dependency-baseline.sh 2>&1 | tail -1; scripts/check-sbom-trend.sh 2>&1 | tail -1; git grep -l 'modernc.org/sqlite' -- '*.go' | grep -v _test.go`
- [ ] **Step 300** [V] ~1m: Both pass; the last command prints nothing (the module stays for test files only; `go mod tidy` keeps it).
- [ ] **Step 301** [W] ~10m: `wiki/The-Well.md`: `timeline_milestones`, writer timeguru (migration 050); line 81's sentence about SQLite becomes "No service keeps a local SQLite copy of Well data (kanban-app and timeguru wait for the Well, read-only, ADR-147)". The timeguru and kanban wiki pages: the same.
- [ ] **Step 302** [W] ~5m: `docs/architecture/THE_WELL_OPERATIONS.md`, the connection budget table: a row for `app_timeguru` with the pool size `pkg/database.Connect` opens (read it from the code).
- [ ] **Step 303** [C] ~1m: **COMMIT**: "Steps 295-303: kanban-app waits for the Well instead of a SQLite board; documents".
- [ ] **Step 304** [V] ~1m: **PHASE 16 EXIT GATE**: Steps 292 and 298 green; Step 300 clean. Every code phase (3 to 16) has its gate or its `[STUCK]` marker in `PROG`.

---

## PHASE 17: THE E-GATE AND THE LIVE BEFORES (Steps 305-319)

**Amended 2026-11-17: read `docs/battle-plans/ADR147-LIVE-RUN-ORDER.md` before running any step from Step 305 on.** The four plans were merged as one branch, so this plan's own merge steps are replaced by that page's block C, its live phases run in that page's order, and the steps that page lists under "Amendments" run as amended there.

**Goal**: The evidence week is over by Stevie's word; the befores that need SQL, ClickHouse or a fresh reading are taken.
**Prerequisite**: Phase 16 gate. Not before 2026-11-17 18:34Z.
**Time**: 1.5 h, with two 1 h gaps for deltas (do Phase 18's builds meanwhile).
**Model**: Opus. **Agent**: Coordinator; Stevie for Steps 305, 306 and 317.

- [ ] **Step 305** [STEVIE][V] ~2m: **THE E-GATE.**
  ```bash
  date -u +%FT%TZ; grep -n 'Step 284' /home/govan/tmp/unheaded/docs/battle-plans/progress/S-E1-LIVE-progress.md | tail -2; grep -n -i 'week' /home/govan/tmp/unheaded/docs/battle-plans/progress/S-WEST-GROUND-progress.md 2>/dev/null | head -3
  ```
  Ask: "The log store week is closed as it stands (decision 5). May this plan now read the Well and ClickHouse with SQL, restart containers and units on west, and later change Vector, VictoriaMetrics and Postgres?" Yes, with a Step 284 entry or S-WEST-GROUND's Step 6 answer → log "E-GATE closed <time>" in `PROG`. No or away → stop; the plan waits here.
- [ ] **Step 306** [STEVIE] ~1m: "West's `docker-compose.override.yml` names architect, captain, micromanager, monad and sophia. May I delete those five entries in Phase 19, just before the stack is brought up? Every other entry stays." Record the answer. No → Phase 19 Step 348 is `[BLOCKED]` and so is every `compose up` after it (Appendix A, procedure 1).
- [ ] **Step 307** [W] ~3m: Append to `/home/govan/tmp/wt/S-WASTE-OUT-scratch/lib.sh`:
  ```bash
  # pg DB SQL: one read-only statement in the Well, as the superuser over the container's socket.
  pg(){ sudo docker exec -i unheaded-postgres sh -c 'psql -X -q -At -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$1" -c "SET default_transaction_read_only = on" -c "$2"' _ "$1" "$2"; }
  # ch SQL: one read-only query in ClickHouse; the password stays inside the container.
  ch(){ sudo docker exec unheaded-clickhouse sh -c 'clickhouse-client --user unheaded --password "$CLICKHOUSE_PASSWORD" --readonly 1 -q "$1"' _ "$1"; }
  # cgio CONTAINER: I/O pressure and bytes on sdb (8:16) from the cgroup files.
  cgio(){ local id d; id=$(sudo docker inspect -f '{{.Id}}' "$1") || return 1; d=/sys/fs/cgroup/system.slice/docker-$id.scope; echo "== $1 $(date -u +%FT%TZ)"; grep '^some' $d/io.pressure; grep '^8:16 ' $d/io.stat; }
  ```
- [ ] **Step 308** [B] ~10m: Steps 8 to 13 again (the before nearest the deploy). Log each under "before (E-GATE)"; a row's before is now this reading, the Phase 0 one kept beside it.
- [ ] **Step 309** [B] ~3m: W-20, exact. Run now and again 10 minutes later; log both with their times.
  ```bash
  . /home/govan/tmp/wt/S-WASTE-OUT-scratch/lib.sh
  pg unheaded_ops "select calls from pg_stat_statements where query like 'INSERT INTO service_health_current%'"; pg unheaded_ops "select wal_sync from pg_stat_wal"; date -u +%s
  ```
  Delta over seconds = upserts a second and WAL syncs a second (reference 0.68 and 0.93). `relation "pg_stat_statements" does not exist` → run it with `postgres` as the database.
- [ ] **Step 310** [B] ~2m: W-25 and W-26.
  ```bash
  . /home/govan/tmp/wt/S-WASTE-OUT-scratch/lib.sh
  pg unheaded_app "select count(*) from timeline_milestones"; sudo docker inspect unheaded-timeguru --format '{{json .HostConfig.Tmpfs}}'; sudo ls -la /var/lib/docker/volumes/unheaded-dev_kanban-data/_data/
  ```
  Reference: `0`; a `/data` tmpfs; `kanban.db` of 2026-10-28. A count above 0 → Step 284's guard will refuse 050: `[STUCK]` for Steps 340-342, read the rows with Stevie.
- [ ] **Step 311** [B] ~5m: W-28, ClickHouse and the disk.
  ```bash
  . /home/govan/tmp/wt/S-WASTE-OUT-scratch/lib.sh
  ch "SELECT count(), round(avg(written_rows),1) FROM system.query_log WHERE type='QueryFinish' AND query_kind='Insert' AND event_time > now() - INTERVAL 1 DAY"
  ch "SELECT count(), formatReadableSize(sum(size_in_bytes)) FROM system.part_log WHERE event_type='MergeParts' AND event_time > now() - INTERVAL 1 DAY"
  cgio unheaded-clickhouse
  q 'rate(node_disk_written_bytes_total{device="sdb"}[24h]) * 86400'
  ```
  Reference: about 47,000 inserts of 5 rows; about 37,700 merges, 42 GiB; ClickHouse 71 GB a day; `sdb` about 104 GB a day. Read `cgio` again 1 h later for a per-hour delta.
- [ ] **Step 312** [B] ~2m: W-29.
  ```bash
  . /home/govan/tmp/wt/S-WASTE-OUT-scratch/lib.sh
  q 'count(count by (host) ({job="vector",__name__=~"vector_.+"}))'; q 'sum(increase(vm_new_timeseries_created_total[7d]))'
  sudo docker inspect unheaded-victoria --format '{{json .Args}}' | tr ',' '\n' | grep -c -i 'maxhourlyseries\|maxdailyseries'
  ```
  Reference: 4 `host` values in 7 days (one per container id), about 40,000 new series a week, `0` limit flags.
- [ ] **Step 313** [B] ~5m: W-30, Postgres. Run now and again 1 hour later.
  ```bash
  . /home/govan/tmp/wt/S-WASTE-OUT-scratch/lib.sh
  cg unheaded-postgres; cgio unheaded-postgres
  pg unheaded_ops "select blks_read, blks_hit from pg_stat_database where datname='unheaded_ops'"
  pg unheaded_ops "select calls, round(mean_exec_time) from pg_stat_statements where query ilike '%from log_events%' and query not ilike 'insert%' order by mean_exec_time desc limit 3"
  q 'huginn_pg_cache_hit_ratio{db="well-ops",job="huginn"}'
  ```
  Per hour: `max` events, `io.pressure` some microseconds, `blks_read`. Reference: about 640 `max` events an hour (52,740 in 82 h), cache hit 0.683, slow statements 3 to 6 s mean.
- [ ] **Step 314** [B] ~3m: W-31, the orphans.
  ```bash
  . /home/govan/tmp/wt/S-WASTE-OUT-scratch/lib.sh
  sudo docker volume inspect unheaded-well-data --format '{{.CreatedAt}} {{.Mountpoint}}'; sudo docker ps -aq --filter volume=unheaded-well-data | wc -l
  ch "SELECT table, formatReadableSize(sum(bytes_on_disk)), min(min_time) FROM system.parts WHERE database='system' AND match(table, '_0$') GROUP BY table"
  for v in captain architect micromanager; do sudo ls -A /var/lib/docker/volumes/unheaded-dev_${v}-data/_data | wc -l; done
  ```
  Reference: created 2026-03-19, `0` users; three `_0` tables, 12 MiB, oldest 2026-02-27; `0` entries in each volume.
- [ ] **Step 315** [W] ~10m: `PROG`: the before of W-20 (exact), W-25, W-26, W-28 to W-31, each with its time.
- [ ] **Step 316** [V] ~1m: Every row W-01 to W-31 but W-27 has a before. A missing one → its step again; a reading that cannot be had → the row says why.
- [ ] **Step 317** [STEVIE][DECIDE] ~5m: The two signing keys (Akira's `HEALTH_SIGNING_KEY_FILE`; alert-mailer's `ALERTS_SIGNING_KEY_FILE`, D1 = A). Where a deployment's public keys live, since west's Wotan reads the repository's `configs/wotan.yaml`: (A) commit the two public keys into `configs/wotan.yaml` (public halves only; the repository then names west's keys); (B) a host-local copy, `/etc/unheaded/wotan/wotan.yaml`, mounted by the gitignored override in place of the repository file; (C) not now: W-05's live half and W-19 wait. **Recommended B**: per deployment, as `wiki/Wotan-Topic-Signing.md` says. The private halves are made on the host with `unheaded-keygen -algo mldsa65` by Stevie, never by an agent, never printed.
- [ ] **Step 318** [C] ~1m: **COMMIT** `PROG` (in `WT`).
- [ ] **Step 319** [V] ~1m: **PHASE 17 EXIT GATE**: E-GATE closed in writing; befores complete; Step 306 and 317 answered or marked.

---

## PHASE 18: MERGE, THE DEFERRED GATES, BUILD, KEEP THE OLD (Steps 320-337)

**Goal**: `develop` holds the sprint; every gate is green; the new images and binaries are built; the running ones are kept for rollback.
**Prerequisite**: Phase 17 gate.
**Time**: 2 h.
**Model**: Opus. **Agent**: Coordinator, sudo.

- [ ] **Step 320** [B] ~1m: `cd /home/govan/tmp/wt/S-WASTE-OUT && git branch --no-merged sprint/s-waste-out | grep s-waste-out; git log --oneline develop..sprint/s-waste-out | wc -l`
- [ ] **Step 321** [B] ~15m: The gates that start a container.
  ```bash
  cd /home/govan/tmp/wt/S-WASTE-OUT && scripts/well/test-migrate.sh 2>&1 | tail -3; scripts/well/test-health-bridge.sh 2>&1 | tail -3; scripts/test-vector-config.sh 2>&1 | tail -2; scripts/check-prometheus-rules.sh 2>&1 | tail -2
  ```
- [ ] **Step 322** [V] ~1m: No lane listed in Step 320; all four pass (050 applies on a fresh server; the bridge integration test passes). A failure → fix in the lane's files, commit, rerun; two tries, then `[STUCK]` and Phase 19 waits.
- [ ] **Step 323** [B] ~20m: The full local CI with the CI venv last on PATH: `cd /home/govan/tmp/wt/S-WASTE-OUT && PATH=$PATH:/var/tmp/unheaded-sprint/civenv/bin scripts/ci-local.sh 2>&1 | tail -25`
- [ ] **Step 324** [V] ~2m: Every gate passes, or each failure is shown red on `develop` before the sprint (the same gate in a clean clone of `1393e5ee`) and logged under "For Stevie".
- [ ] **Step 325** [B] ~2m: Merge. The live configs are bind-mounted from the main checkout: from here on a container restart picks up the new files.
  ```bash
  cd /home/govan/tmp/unheaded && git switch develop && git merge --no-ff -S sprint/s-waste-out -m "[PLAN S-WASTE-OUT] Steps 1-324: waste out (ADR-147 decisions 7, 8, 9, 10, 19, 24)" && git log --oneline -1
  ```
- [ ] **Step 326** [V] ~1m: A merge commit. A conflict → `git merge --abort`, merge `develop` into the sprint branch in `WT`, rerun Steps 321-324, then this step. A second conflict → `[STUCK]`.
- [ ] **Step 327** [S][B] ~2m: Tag every running first-party image for rollback.
  ```bash
  sudo docker ps --format '{{.Image}}' | sort -u | grep -E '^(unheaded-dev|monitoring)-' | while read -r i; do sudo docker tag "$i" "${i%%:*}:pre-s-waste-out" && echo "kept $i"; done | tee /home/govan/tmp/wt/S-WASTE-OUT-scratch/kept-images.txt | wc -l
  ```
- [ ] **Step 328** [V] ~1m: One `kept` line per first-party container of Step 308's `docker ps` (the five role services and cuirass included). Fewer → read the image names (`sudo docker ps --format '{{.Names}} {{.Image}}'`) and fix the pattern.
- [ ] **Step 329** [S][B] ~1m: Keep the host binaries and objects.
  ```bash
  sudo install -d -m 0750 /var/backups/s-waste-out && sudo cp -p /usr/local/bin/trace-collector-go /usr/local/bin/chaos-controller /usr/local/bin/kev-poller /usr/local/lib/unheaded/ebpf/packet-marker /usr/local/lib/unheaded/ebpf/flow-tracker /var/backups/s-waste-out/ && sudo ls -la /var/backups/s-waste-out/
  ```
- [ ] **Step 330** [S][B] ~15m: Build the images (no start).
  ```bash
  cd /home/govan/tmp/unheaded && sudo docker compose build wotan timeguru akira dashboard-backend kanban-app auth logstore-writer sword cuirass 2>&1 | tail -4 && (cd monitoring && sudo docker compose build alert-mailer 2>&1 | tail -2)
  ```
- [ ] **Step 331** [V] ~1m: Every build ends without `ERROR`. A failure → read it; a Go compile error means Step 322 missed it: `[STUCK]`.
- [ ] **Step 332** [B] ~5m: Host binaries, with the version the Makefile stamps.
  ```bash
  cd /home/govan/tmp/unheaded && mkdir -p /home/govan/tmp/wt/S-WASTE-OUT-scratch/bin && V=$(git describe --always --dirty) && for p in trace-collector-go chaos-controller kev-poller; do go build -ldflags "-X unheaded/pkg/cmdline.Version=$V" -o /home/govan/tmp/wt/S-WASTE-OUT-scratch/bin/$p ./cmd/$p || echo "FAIL $p"; done; ls -la /home/govan/tmp/wt/S-WASTE-OUT-scratch/bin/; grep -n 'WOTAN' deploy/systemd/unheaded-chaos-controller.service deploy/systemd/unheaded-kev-poller.service deploy/systemd/unheaded-trace-collector.service
  ```
  The units take Wotan's gRPC address from `pkg/transport`'s default `localhost:18001` (published on 127.0.0.1) unless they set `WOTAN_GRPC_ADDR`.
- [ ] **Step 333** [B] ~5m: The eBPF pair from `develop`: Step 266's command with `/home/govan/tmp/unheaded/ebpf`.
- [ ] **Step 334** [V] ~2m: Step 267's tests against `/home/govan/tmp/unheaded/ebpf/target/load-gate/default/bpfel-unknown-none/release/` → `ok` (with D6 away, only the packet-marker test).
- [ ] **Step 335** [W] ~3m: `PROG`: the merge commit, `sha256sum` of the three binaries and two objects, the image ids (`sudo docker images --format '{{.Repository}}:{{.Tag}} {{.ID}}' | grep -E 'unheaded-dev|monitoring'`).
- [ ] **Step 336** [C] ~1m: **COMMIT** `PROG` on `develop` (named file).
- [ ] **Step 337** [V] ~1m: **PHASE 18 EXIT GATE**: develop holds the sprint; gates green; images, binaries and objects built; the old ones kept.

---

## PHASE 19: DEPLOY ON WEST (ATTENDED) (Steps 338-358)

**Goal**: West runs the sprint: the six containers gone, every changed service and host unit on the new build, timeguru on the Well.
**Prerequisite**: Phase 18 gate. Stevie present (the Well changes; many containers restart).
**Time**: 1.5 h.
**Model**: Opus. **Agent**: Coordinator with Stevie.

**Order and why.** The Well first (timeguru's new image needs 050 and its role's password). The trace collector before Wotan: the new collector publishes only `traces.*`, which the old Wotan allows, so the restarted Wotan refuses nothing. The six leave before Wotan restarts, so no old client reconnects to a permission list that no longer names it. `--no-deps` on every `up`: Postgres, ClickHouse, Vector and VictoriaMetrics are never recreated here.

- [ ] **Step 338** [STEVIE] ~1m: "Deploy S-WASTE-OUT on west now? Containers restart for about 15 minutes; the Well gets migration 050." No → the phase waits.
- [ ] **Step 339** [B] ~1m: `sudo docker ps --format '{{.Names}}' | sort > /home/govan/tmp/wt/S-WASTE-OUT-scratch/ps-before.txt; wc -l < /home/govan/tmp/wt/S-WASTE-OUT-scratch/ps-before.txt; date -u +%FT%TZ`
- [ ] **Step 340** [STEVIE][S] ~5m: `app_timeguru`'s password. Stevie puts `APP_TIMEGURU_PASSWORD` in `/home/govan/tmp/unheaded/.env` if it is not there, then runs, touching that role only:
  ```bash
  cd /home/govan/tmp/unheaded && ( set -a && . ./.env && set +a && for v in $(compgen -v | grep -E '_PASSWORD$' | grep -vx APP_TIMEGURU_PASSWORD); do unset "$v"; done; scripts/well/set-role-passwords.sh 2>&1 | tail -2 )
  ```
  Expect one line, `[well] app_timeguru: password set from APP_TIMEGURU_PASSWORD`. No value is printed by anyone. (Amended 2026-11-17: the first version unset eight variables by name and missed `POSTGRES_PASSWORD`, `WELL_BACKUP_PASSWORD`, `RETENTION_RUNNER_PASSWORD` and `LOG_WRITER_PASSWORD`, so it would have set five roles, not one.)
- [ ] **Step 341** [STEVIE][S] ~5m: Migration 050, dry run then apply, with a backup of `unheaded_app` first.
  ```bash
  sudo install -d -o govan -g govan -m 0700 /srv/backups/unheaded-well/pre-050 && cd /home/govan/tmp/unheaded && set -a && . ./.env && set +a && WELL_MIGRATE_PASSWORD=$POSTGRES_PASSWORD scripts/well/batch-apply.sh --dry-run --from 050 --through 050 2>&1 | tail -4
  ```
  Expect `pending (1) through 050:` naming `050_app_timeline_milestones.sql`, and `left pending for a later batch (3)` naming 051, 052 and 053 (S-ZHEN-SWITCH's, applied in its Phase 8B). Then the same with `WELL_BATCH_BACKUP_ROOT=/srv/backups/unheaded-well/pre-050` and without `--dry-run`; it ends `done through 050; 3 later file(s) left pending`. A file below 050 pending, or any other left pending → stop, read the ledger, apply nothing. (Amended 2026-11-17: `--through` was added that day, integration commit `18fae670`; without it the batch would run on into 053, which refuses until the rows are copied.)
- [ ] **Step 342** [V] ~1m: `. /home/govan/tmp/wt/S-WASTE-OUT-scratch/lib.sh; pg unheaded_app "select filename from well_schema_migrations where filename like '050%'"; pg unheaded_app "select string_agg(column_name, ',' order by ordinal_position) from information_schema.columns where table_name='timeline_milestones'"` → the file; `milestone_key,name,status,progress_pct,eta,owner,risk,description,updated_at`. Else → `scripts/well/batch-rollback.sh` with the pre-050 backup (read its usage line), and timeguru keeps its old image.
- [ ] **Step 343** [S][B] ~3m: The trace collector and the eBPF pair, together.
  ```bash
  O=/home/govan/tmp/unheaded/ebpf/target/load-gate/default/bpfel-unknown-none/release && sudo install -m 0755 /home/govan/tmp/wt/S-WASTE-OUT-scratch/bin/trace-collector-go /usr/local/bin/trace-collector-go && sudo install -m 0644 $O/packet-marker $O/flow-tracker /usr/local/lib/unheaded/ebpf/ && sudo systemctl restart unheaded-trace-collector && date -u +%FT%TZ
  ```
- [ ] **Step 344** [V] ~2m:
  ```bash
  systemctl is-active unheaded-trace-collector; journalctl -u unheaded-trace-collector --since -2min --no-pager | grep -E 'NOT loaded|error|refused' | head -5; sudo bpftool map show name FLOW_STATE | head -1; sudo bpftool map show name FLOW_EVENTS | wc -l; curl -s 127.0.0.1:16671/metrics | grep -c '^trace_collector_flow_state_insert_failures'
  ```
  Expected: `active`; no error line; `lru_hash`; `0` (D6 = A); `1`. Else → Step 345.
- [ ] **Step 345** [D] ~5m: Read the error. Two tries, then roll back: `sudo cp -p /var/backups/s-waste-out/trace-collector-go /usr/local/bin/ && sudo cp -p /var/backups/s-waste-out/packet-marker /var/backups/s-waste-out/flow-tracker /usr/local/lib/unheaded/ebpf/ && sudo systemctl restart unheaded-trace-collector`; W-06, W-23, W-24 `[STUCK]`. The new Wotan then refuses the old collector's `ebpf.*` publishes: keep `"ebpf.*.events"` in its `publish` list on `develop` (a signed commit) before Step 351.
- [ ] **Step 346** [S][B] ~2m: `for b in chaos-controller kev-poller; do sudo install -m 0755 /home/govan/tmp/wt/S-WASTE-OUT-scratch/bin/$b /usr/local/bin/$b; done && sudo systemctl restart unheaded-chaos-controller unheaded-kev-poller && systemctl is-active unheaded-chaos-controller unheaded-kev-poller`
- [ ] **Step 347** [V] ~2m: Two `active`; `journalctl -u unheaded-chaos-controller -u unheaded-kev-poller --since -2min --no-pager | grep -iE 'wotan|error' | head -6` shows a gRPC connection and no `ErrNoStreamTransport`. Else → roll back that binary from `/var/backups/s-waste-out/`.
- [ ] **Step 348** [W] ~2m: (Step 306 = yes.) `/home/govan/tmp/unheaded/docker-compose.override.yml`: delete the entries `architect:`, `captain:`, `micromanager:`, `monad:` and `sophia:`, each with its indented lines, with the Edit tool. If Step 317 chose B and Stevie has made the host file: add under `wotan:` the mount `- /etc/unheaded/wotan/wotan.yaml:/etc/wotan/wotan.yaml:ro` (Stevie's file holds the two public keys).
- [ ] **Step 349** [S][B] ~2m: The six leave, with the three empty volumes.
  ```bash
  sudo docker stop unheaded-architect unheaded-captain unheaded-micromanager unheaded-monad unheaded-sophia unheaded-cuirass && sudo docker rm unheaded-architect unheaded-captain unheaded-micromanager unheaded-monad unheaded-sophia unheaded-cuirass && sudo docker volume rm unheaded-dev_captain-data unheaded-dev_architect-data unheaded-dev_micromanager-data
  ```
  A volume that Step 314 found not empty is left and logged. `cuirass-state` stays for plan 7.
- [ ] **Step 350** [V] ~1m: `cd /home/govan/tmp/unheaded && sudo docker compose config --services | sort | tr '\n' ' '`: no error and none of the five (cuirass is listed: it has a profile). An error naming one of the five → the override still names it (Step 348).
- [ ] **Step 351** [S][B] ~2m: `cd /home/govan/tmp/unheaded && sudo docker compose up -d --no-build --no-deps wotan && date -u +%FT%TZ`. Wait for it: `sudo docker ps --filter name=unheaded-wotan --format '{{.Status}}'` until `(healthy)`, at most 3 minutes.
- [ ] **Step 352** [V] ~2m: `(healthy)`; `sudo docker logs unheaded-wotan --since 3m 2>&1 | grep -iE 'error|refus' | head -5` shows no config error. Unhealthy → Appendix A, procedure 2.
- [ ] **Step 353** [S][B] ~5m: The rest. `ps-before.txt` decides `sword`: add `--profile deploy sword` only if `unheaded-sword` is in it.
  ```bash
  cd /home/govan/tmp/unheaded && sudo docker compose up -d --no-build --no-deps timeguru akira dashboard-backend kanban-app auth logstore-writer 2>&1 | tail -8
  ```
- [ ] **Step 354** [V] ~3m: Within 3 minutes every one is `(healthy)`: `sudo docker ps --format '{{.Names}} {{.Status}}' | grep -E 'timeguru|akira|dashboard|kanban|auth|logstore' `. A container that exited stays down (restart `no`): `sudo docker logs <name> --tail 30`, fix or roll back that one service (Appendix A, procedure 3).
- [ ] **Step 355** [S][B] ~2m: The monitoring side.
  ```bash
  cd /home/govan/tmp/unheaded/monitoring && sudo docker compose up -d --no-build --no-deps alert-mailer && sudo docker kill -s HUP unheaded-prometheus && sudo docker kill -s HUP unheaded-victoria && date -u +%FT%TZ
  ```
- [ ] **Step 356** [V] ~3m: After 2 minutes: `curl -s 127.0.0.1:9090/api/v1/targets | python3 -c 'import json,sys; t=json.load(sys.stdin)["data"]["activeTargets"]; print(len(t), [x["labels"]["job"] for x in t if x["health"]!="up"])'` lists no job down that was up before; `curl -s -G 127.0.0.1:8428/api/v1/targets | grep -cE '"job":"(architect|captain|micromanager|monad|sophia)"'` prints `0`; `sudo docker ps --format '{{.Names}}' | sort | comm -23 /home/govan/tmp/wt/S-WASTE-OUT-scratch/ps-before.txt -` lists exactly the six (write the `docker ps` output to a file first if `comm` refuses a pipe).
- [ ] **Step 357** [C] ~2m: **COMMIT** `PROG` on `develop`: the deploy times of Steps 343 to 355, the six removed, anything rolled back. Plan 5's baseline note in "Handoffs": "S-WASTE-OUT deployed <time>".
- [ ] **Step 358** [V] ~1m: **PHASE 19 EXIT GATE**: every changed container healthy and every changed unit active; the six gone; nothing rolled back without its row saying so.

---

## PHASE 20: THE AFTER READINGS (Steps 359-366)

**Goal**: Every row deployed in Phase 19 has an after, measured as its before was, and a status.
**Prerequisite**: Phase 19 gate. Readings at 1 h and 24 h after Step 355.
**Time**: 1 h of work across 24 h. Phases 21 and 22 may start after the 1 h reading.
**Model**: Opus. **Agent**: Coordinator.

**Pass bars.** A row whose after is worse than its before is rolled back and says so (Micromanager).

| Row | Reading | Before (2026-11-15) | Bar |
|---|---|---|---|
| W-01 | Wotan `GET` a second, 30 m rate | 68.8 | **≤ 1.0** |
| W-02 | `/health` of kanban-app: `stream_transport` present, no `"grpc":true` unless checked | the default claimed gRPC | **as stated** |
| W-03 | `unheaded_chat_members_approved` | 63 | **≤ 25** |
| W-04 | streams open on `system.discovery` (`unheaded_wotan_client_stream_up{topic="system.discovery"}`) | 12 polls | **1** (Akira, D3 = A) or **0** |
| W-05 | `unheaded_critical_alerts_state` series | none | **one per client binary**; the `alerts.critical` room exists once the key is enrolled |
| W-06 | trace rooms' bytes | 39.3 MB of 52.8 | **no `ebpf.*.events` room; ≤ 55% of before** |
| W-07 | Wotan 429s in 24 h | 367 in 7 d | **0** |
| W-08 | `unheaded_auth_audit_write_failures_total` | not counted | **present, 0** |
| W-09, W-10, W-11 | the six containers | running | **absent** |
| W-12 | `prometheus_tsdb_head_series`, 3 h after the HUP | 13,730 | **≤ 4,000** |
| W-13 | dashboard RSS; allocation rate (6 h) | 289 MiB; 13.2 MB/s | **≤ 120 MiB; ≤ 2 MB/s** |
| W-14, W-22 | `/health` requests a second, all services | 6.46 | **≤ 2.6** |
| W-15 | dashboard 401s a second, no logged-out tab open | 0.27 | **≤ 0.01** |
| W-16 | west log lines a minute | 241 | **≤ 60** |
| W-17 | Akira throttled periods over 1 h | 44% | **≤ 1%** |
| W-18 | Akira zombies | 18 | **0** |
| W-19 | `akira_signed_publish_refused_total` rising | 49 and 12 | **flat** (key enrolled) or waits |
| W-20 | health upserts a second; WAL syncs a second | 0.68; 0.93 | **≤ 0.1; ≤ 0.4** |
| W-21 | `go_sched_gomaxprocs_threads` by job | absent | **present for every first-party job** |
| W-23 | `FLOW_STATE` type; insert-failure gauge | hash, full; none | **lru_hash; present** |
| W-24 | `FLOW_EVENTS` loaded | yes | **no** |
| W-25 | `timeline_milestones` rows; timeguru tmpfs | 0; `/data` | **= milestones in `references/timeline.md`; none** |
| W-26 | `kanban.db` path in the running image | present | **absent** (`strings` finds no `kanban.db`) |

- [ ] **Step 359** [B] ~10m: **1 h after.** Steps 8 to 12 again, west only, and `. /home/govan/tmp/wt/S-WASTE-OUT-scratch/lib.sh; q 'count by (job) (go_sched_gomaxprocs_threads)'; q 'count by (job) (unheaded_critical_alerts_state)'; pg unheaded_app "select count(*) from timeline_milestones"; sudo docker logs unheaded-timeguru 2>&1 | grep 'loaded timeline from file' | tail -1`. The row count equals the log line's `milestones` value. Log each reading against its row.
- [ ] **Step 360** [B] ~5m: Step 309's two readings, 10 minutes apart (W-20). `curl -sk https://127.0.0.1:20001/health | python3 -m json.tool | grep -E 'stream_transport|grpc' | head` (W-02). W-26: `P=$(sudo docker inspect unheaded-kanban --format '{{index .Config.Entrypoint 0}}') && sudo docker cp "unheaded-kanban:$P" /home/govan/tmp/wt/S-WASTE-OUT-scratch/kanban-app.bin && grep -c 'kanban.db' /home/govan/tmp/wt/S-WASTE-OUT-scratch/kanban-app.bin` → `0` (an empty `Entrypoint` → use `.Config.Cmd`).
- [ ] **Step 361** [B] ~10m: **24 h after.** Step 359 and Step 360 again; Step 11's `cg unheaded-akira` twice, 1 h apart, for the throttled share of that hour (W-17).
- [ ] **Step 362** [V] ~5m: Each row against its bar. All met → status `done` with the after. Any other → Step 363.
- [ ] **Step 363** [D] ~20m: One look per row, then a status. W-01 over the bar: `q 'sum by (instance, topic) (rate(unheaded_wotan_client_polls_total[30m]))'` names who polls and what; a poller that is a first-party binary is a bug in Phase 6 or 7: `[STUCK]` with the name. W-13 over: `PPROF_ADDR` on loopback for one restart of the dashboard, a 30 s heap profile, the top 5 in `PROG`. W-16 over: `q 'topk(5, sum by (unit) (rate(unheaded_log_lines_total{host="west"}[1h])) * 60)'`. A row worse than its before → roll that change back (Appendix A) and say so. Every row ends `done`, `missed: <reading>` or `[STUCK]`.
- [ ] **Step 364** [W] ~10m: `PROG`: after and status for W-01 to W-26.
- [ ] **Step 365** [C] ~1m: **COMMIT** `PROG` on `develop`.
- [ ] **Step 366** [V] ~1m: **PHASE 20 EXIT GATE**: every deployed row has an after and a status.

---

## PHASE 21: VECTOR BATCHES; VICTORIAMETRICS BOUNDED (Erda F1, F12) (Steps 367-382)

**Goal**: Vector inserts into ClickHouse every 30 s, not every second. Vector's own series have one `host`. VictoriaMetrics refuses a series explosion and says so.
**Prerequisite**: Phase 17 gate (the E-GATE) and Phase 19 gate. If plan 4 moves the volumes between this phase's before (Step 311) and its after, Step 311 is taken again on the new disk first.
**Time**: 1.5 h, then 24 h.
**Model**: Opus. **Agent**: Coordinator in `WT` (rebased on `develop`), sudo.

- [ ] **Step 367** [B] ~2m: `cd /home/govan/tmp/wt/S-WASTE-OUT && git merge --ff-only develop && git log --oneline -1`
- [ ] **Step 368** [W] ~10m: `config/vector/vector.yaml`, sink `clickhouse`, after `request:`:
  ```yaml
      # One insert per 30 s, not per second (ADR-147, Erda F1): 47,000 inserts
      # a day of 5 rows made ClickHouse merge 42 GiB a day to keep 127 MiB.
      # Rows become readable up to 30 s later; the disk buffer above keeps
      # durability as it was.
      batch:
        max_events: 100000
        max_bytes: 10485760
        timeout_secs: 30
  ```
- [ ] **Step 369** [B] ~3m: `cd /home/govan/tmp/wt/S-WASTE-OUT && scripts/test-vector-config.sh 2>&1 | tail -3`
- [ ] **Step 370** [V] ~1m: Pass. A refusal of a `batch` key → the 0.43 sink's key names are in `vector validate`'s message; fix the name, never drop the block.
- [ ] **Step 371** [W] ~15m: VictoriaMetrics.
  - `config/victoria/scrape.yml`, job `vector`: `metric_relabel_configs: [{source_labels: [__name__], regex: 'vector_.+', target_label: host, replacement: west}]` with the comment `# Vector's own series carry the container id as host; each recreate minted ~1,400 series (ADR-147, Erda F12). Its journal_* series keep the journal's host.`
  - `docker-compose.yml`, `victoria` `command`: `- "-storage.maxHourlySeries=100000"` and `- "-storage.maxDailySeries=300000"`, comment `# 16,971 active and ~5,700 new a day on 2026-11-15: a label explosion is refused and counted, not an OOM`.
  - The alert. `grep -n 'job_name: *victoria\|8428' monitoring/prometheus/prometheus.yml`: if plan 1 added a job that scrapes VictoriaMetrics' `/metrics`, use it; else add job `victoria` (target `[fd00:dead:beef::1]:8428`, `metrics_path: /metrics`). Rule `VictoriaMetricsSeriesLimitReached`: `expr: increase(vm_hourly_series_limit_rows_dropped_total[15m]) > 0 or increase(vm_daily_series_limit_rows_dropped_total[15m]) > 0`, in the rule file plan 1 uses for VictoriaMetrics, else `rules/host.yml`, with a promtool case. Then `python3 scripts/gen-prom-keep-list.py`.
- [ ] **Step 372** [B] ~3m: `cd /home/govan/tmp/wt/S-WASTE-OUT && scripts/check-prometheus-rules.sh 2>&1 | tail -2 && scripts/check-compose-hardening.sh >/dev/null 2>&1 && echo PASS`, then Step 198's promtool `check config`.
- [ ] **Step 373** [V] ~1m: All pass.
- [ ] **Step 374** [C] ~1m: **COMMIT** by name: "Steps 367-374: Vector batches ClickHouse inserts; VictoriaMetrics bounds new series; Vector's host label fixed".
- [ ] **Step 375** [B] ~1m: `cd /home/govan/tmp/unheaded && git merge --ff-only sprint/s-waste-out && git log --oneline -1`. "Not possible to fast-forward" (a `PROG` commit landed on `develop` meanwhile) → `git -C /home/govan/tmp/wt/S-WASTE-OUT rebase -S develop` (re-signs the moved commits), then this step again. The same holds for Step 387.
- [ ] **Step 376** [S][B] ~3m: Apply. Vector reads one bind-mounted file: restart, never HUP. VictoriaMetrics gets new flags: recreate.
  ```bash
  cd /home/govan/tmp/unheaded && sudo docker restart unheaded-vector && sudo docker compose up -d --no-build --no-deps victoria && sudo docker kill -s HUP unheaded-prometheus && date -u +%FT%TZ
  ```
- [ ] **Step 377** [V] ~5m: After 2 minutes: `sudo docker logs unheaded-vector --since 3m 2>&1 | grep -iE 'error|invalid' | head -5` empty; `. /home/govan/tmp/wt/S-WASTE-OUT-scratch/lib.sh; q 'rate(vector_component_sent_events_total{component_id="clickhouse"}[5m])'` above 0; `sudo docker inspect unheaded-victoria --format '{{json .Args}}' | grep -c maxHourlySeries` = 1; `sudo docker ps --filter name=unheaded-victoria --format '{{.Status}}'` healthy.
- [ ] **Step 378** [D] ~5m: Vector errors → `git revert --no-edit -S <Step 374's commit>` on `develop`, `sudo docker restart unheaded-vector`, read the error with Step 369. VictoriaMetrics unhealthy → the same revert, `sudo docker compose up -d --no-build --no-deps victoria`. Two tries, then `[STUCK]`; W-28 or W-29 says so.
- [ ] **Step 379** [B] ~10m: **24 h after.** Steps 311 and 312 again, and `ch "SELECT count() FROM system.parts WHERE database='unheaded_logs' AND active"`.
- [ ] **Step 380** [V] ~2m: Bars. W-28: inserts a day **≤ 3,500** (47,386); merged a day **≤ 5 GiB** (41.9); ClickHouse written a day **≤ 15 GB** (71.3). W-29: one `host` value for `vector_*` (`west`) across the restart; both flags set. A bar missed → Step 363's rule.
- [ ] **Step 381** [W] ~5m: `PROG`: W-28 and W-29 after and status. "Handoffs", plan 5: "Vector batched (30 s) since <Step 376 time>; VictoriaMetrics series limits since the same time." `wiki/Log-Aggregation.md` (or the ClickHouse page): the batch and its 30 s delay.
- [ ] **Step 382** [C][V] ~1m: **PHASE 21 EXIT GATE**: both rows have an after; committed on `develop` by name.

---

## PHASE 22: THE WELL SITTING: POSTGRES'S BUDGET; THE ORPHANS (ATTENDED) (Steps 383-400)

**Goal**: Postgres has a memory and CPU budget for the data it now holds (D8), measured before and after; the orphans of decision 24 are gone.
**Prerequisite**: Phase 17 gate; D8 answered for Steps 384-393 (away → `[BLOCKED]`); Stevie present. Plan 4's note applies (Phase 21).
**Time**: 1 h, then 24 h.
**Model**: Opus. **Agent**: Coordinator with Stevie.

- [ ] **Step 383** [STEVIE] ~1m: "Restart Postgres now with the D8 budget? About 10 s down; dashboard, kanban, timeguru, logstore-writer, auth, Huginn and zhen-agentd reconnect." No → Steps 384-393 wait; Steps 394-399 may still run.
- [ ] **Step 384** [W] ~10m: (D8 = A.) In `WT`, after `git -C /home/govan/tmp/wt/S-WASTE-OUT merge --ff-only develop`: `docker-compose.yml`, `postgres`: `memory: 2G`, `cpus: "1.0"`, comment `# ClickHouse's limits, so the second log store week is fair (ADR-147 decision 5, Erda F2: the 256M limit was hit 52,740 times)`. `db/postgresql.conf`: the header lines 7-9 become "Sized for the compose limit: 2G memory, 1.0 CPU (ADR-147 D8)."; `shared_buffers = 512MB`; `effective_cache_size = 1536MB`. D8 = B: `1G`, `"0.5"`, `256MB`, `768MB`.
- [ ] **Step 385** [B] ~5m: `cd /home/govan/tmp/wt/S-WASTE-OUT && scripts/well/test-config.sh 2>&1 | tail -3`
- [ ] **Step 386** [V] ~1m: Pass: a throwaway server starts with the file.
- [ ] **Step 387** [C] ~1m: **COMMIT** both files in `WT`, then `cd /home/govan/tmp/unheaded && git merge --ff-only sprint/s-waste-out`.
- [ ] **Step 388** [S][B] ~2m: `sudo docker ps --format '{{.Names}}' | sort > /home/govan/tmp/wt/S-WASTE-OUT-scratch/ps-pg.txt && cd /home/govan/tmp/unheaded && sudo docker compose up -d --no-build --no-deps postgres && date -u +%FT%TZ`
- [ ] **Step 389** [V] ~3m: Within 60 s `(healthy)`; `. /home/govan/tmp/wt/S-WASTE-OUT-scratch/lib.sh; pg postgres "show shared_buffers"` → `512MB`; every name in `ps-pg.txt` still running (`sudo docker ps --format '{{.Names}}' | sort | comm -13 - /home/govan/tmp/wt/S-WASTE-OUT-scratch/ps-pg.txt` empty). A container that exited stays down by policy: `sudo docker compose up -d --no-build --no-deps <name>`, and log it as evidence for plan 1's stopped-container alert.
- [ ] **Step 390** [D] ~5m: Postgres does not come up → `sudo docker logs unheaded-postgres --tail 30`; revert Step 387's commit on `develop` (signed), `sudo docker compose up -d --no-build --no-deps postgres`. Then `[STUCK]`.
- [ ] **Step 391** [B] ~2m: `systemctl is-active zhen-agentd; journalctl -u zhen-agentd --since -5min --no-pager | grep -ciE 'database|postgres|connection'; . /home/govan/tmp/wt/S-WASTE-OUT-scratch/lib.sh; q 'huginn_pg_up'` → active; reconnect lines only; `1`.
- [ ] **Step 392** [B] ~5m: **24 h after.** Step 313, twice, 1 hour apart.
- [ ] **Step 393** [V] ~2m: Bars for W-30: `max` events in the hour **0** (about 640); cache hit **≥ 0.95** (0.683); `blks_read` in the hour **≤ 20%** of before; Postgres `io.pressure` some in the hour **≤ 20%** of before; the slow log statements' mean recorded (no bar: plan 5 judges the store). `PROG` row and status; "Handoffs", plan 5: "Postgres at 2G / 1.0 CPU since <Step 388 time>".
- [ ] **Step 394** [STEVIE] ~1m: "Delete now, per decision 24: volume `unheaded-well-data` (a March Postgres cluster, mounted by nothing), ClickHouse's `system.query_log_0`, `system.error_log_0`, `system.query_metric_log_0`, and volume `unheaded-dev_kanban-data` (only `kanban.db` of 2026-10-28)?" Each item yes or no.
- [ ] **Step 395** [S][B] ~1m: For each yes: `sudo docker ps -aq --filter volume=unheaded-well-data | wc -l` is `0`, then `sudo docker volume rm unheaded-well-data`; `sudo docker ps -aq --filter volume=unheaded-dev_kanban-data | wc -l` is `0` (kanban-app was recreated without it in Step 353), then `sudo docker volume rm unheaded-dev_kanban-data`.
- [ ] **Step 396** [S][B] ~2m: ClickHouse (not read-only: the one write this plan makes there).
  ```bash
  for t in query_log_0 error_log_0 query_metric_log_0; do sudo docker exec unheaded-clickhouse sh -c 'clickhouse-client --user unheaded --password "$CLICKHOUSE_PASSWORD" -q "DROP TABLE IF EXISTS system.$1"' _ "$t"; done
  ```
- [ ] **Step 397** [V] ~1m: Step 314's commands: `No such volume` for `unheaded-well-data`; no `_0` table; `sudo docker volume ls -q | grep -c kanban-data` is `0`.
- [ ] **Step 398** [W] ~5m: `PROG`: W-31 after and status (what went, its size, the time). `wiki/The-Well.md` and `wiki/Well-Backups.md`: the memory budget and the orphan volume's removal (an ops fact, in the wiki).
- [ ] **Step 399** [C] ~1m: **COMMIT** `PROG` and the two wiki pages on `develop` by name.
- [ ] **Step 400** [V] ~1m: **PHASE 22 EXIT GATE**: W-30 and W-31 have an after, or their answer of Step 383 or 394 is recorded.

---

## PHASE 23: CLOSE (Steps 401-414)

**Goal**: Gates green on `develop`, the record says what changed, every handoff is written, next.md is current.
**Prerequisite**: Phases 20 to 22 gates (or their markers).
**Time**: 1.5 h.
**Model**: Opus. **Agent**: Coordinator.

- [ ] **Step 401** [B] ~20m: `cd /home/govan/tmp/unheaded && PATH=$PATH:/var/tmp/unheaded-sprint/civenv/bin scripts/ci-local.sh 2>&1 | tail -25`
- [ ] **Step 402** [V] ~2m: As Step 324.
- [ ] **Step 403** [W] ~15m: `docs/adr/ADR-147-every-service-earns-its-place.md`: a dated section at the end, "Update <date> (S-WASTE-OUT closed)": one table, row id, finding, before, after, status, from `PROG`; one line on what is open and with whom. Nothing above it is edited. `ADR-INDEX.md`: status unchanged.
- [ ] **Step 404** [W] ~10m: Librarian sweep: `git grep -nE 'ebpf\.(packet|flow|latency)\.events|kanban\.db|DB_PATH|timeguru\.db|19001|19002|19003|19004|19005|alerts\.critical' -- wiki CLAUDE.md README.md QUICKSTART.md | head -30`. Each hit that describes the old state is reworded; retirement lines stay.
- [ ] **Step 405** [V] ~1m: Every remaining hit is a retirement note or the new `alerts.critical` text.
- [ ] **Step 406** [C] ~1m: **COMMIT** the ADR and the swept files by name.
- [ ] **Step 407** [W] ~10m: `PROG` "Handoffs", final: **plan 1**: the stopped-container rule must not list the six; `AuditWriteFailing` and `VictoriaMetricsSeriesLimitReached` exist. **plan 3**: Akira's `zhenai` default target; zhen-agentd's hold on `alerts.critical` (Step 143). **plan 4**: images tagged `pre-s-waste-out`, the Postgres budget. **plan 5**: the times of Steps 376 and 388. **plan 6**: east's three role units, `configs/akira-east.yaml`, the eBPF pair and the collector, Akira's `discover.subnets`, alert-mailer's twin on east, east's chaos-controller and kev-poller binaries. **plan 7**: Cuirass's start command, `CUIRASS_ENFORCE` and `CUIRASS_PRUNE` unset, `cuirass-state` to clear. **plan 9**: the GOMAXPROCS readings, right-sizing limits, ClickHouse's caches (Erda F13).
- [ ] **Step 408** [W] ~5m: `PROG` "For Stevie": the runbook manifest's seal; the archive's remote; the two keys if Step 317 was C; images `:pre-s-waste-out` and `/var/backups/s-waste-out/` to remove after 3 days of green (`sudo docker images | grep pre-s-waste-out`, `sudo rm -r /var/backups/s-waste-out`), and `/srv/backups/unheaded-well/pre-050`; any `[STUCK]`, oldest first.
- [ ] **Step 409** [C] ~1m: **COMMIT** `PROG` by name.
- [ ] **Step 410** [B] ~1m: `for w in -docs -wotan -mon -dash -ebpf ""; do git -C /home/govan/tmp/unheaded worktree remove /home/govan/tmp/wt/S-WASTE-OUT$w 2>/dev/null; done; git -C /home/govan/tmp/unheaded worktree list | grep -c S-WASTE-OUT`
- [ ] **Step 411** [V] ~1m: `0`. A worktree refuses (untracked files) → list them; nothing is forced.
- [ ] **Step 412** [W] ~10m: `~/tmp/next.md`: a section at the top: the rows done, missed and stuck with their numbers; what waits on Stevie (Step 408's list); the handoffs by plan; the push count (`git -C /home/govan/tmp/unheaded log --oneline origin/develop..develop | wc -l`).
- [ ] **Step 413** [V] ~1m: `git -C /home/govan/tmp/unheaded status --short | grep -v '^??' | wc -l` is `0`; `git -C /home/govan/tmp/unheaded log origin/develop..develop --format='%G? %s' | grep -vc '^G'` is `0`, or the unsigned ones are listed for `scripts/resign-unpushed.py`.
- [ ] **Step 414** [V] ~1m: **PHASE 23 EXIT GATE**: develop holds the sprint, gates green, every row W-01 to W-31 has a status, next.md written. Stevie pushes.

---

## Appendix A: Emergency procedures

1. **A compose command fails: "service X has neither an image nor a build context".** The override still names a removed service. Step 348's edit; if Stevie said no at Step 306, every `compose up` waits for him.
2. **Wotan does not come back healthy (Step 352).** `sudo docker logs unheaded-wotan --tail 50`. A refused topic config → the merged `configs/wotan.yaml` is the cause: `git -C /home/govan/tmp/unheaded diff <Step 325's merge>^1 -- configs/wotan.yaml`, fix on `develop` (signed), `sudo docker compose up -d --no-build --no-deps wotan`. Not fixed in two tries → roll back: `sudo docker tag unheaded-dev-wotan:pre-s-waste-out unheaded-dev-wotan:latest && sudo docker compose up -d --no-build --no-deps wotan`, and revert the merge's `configs/wotan.yaml` hunk. Every service then runs against the old Wotan, which is compatible (the new clients speak the same API).
3. **A service exits after a restart and stays down.** Restart policy is `no` (decision 1): nothing brings it back. `sudo docker logs <name> --tail 30`. Its own fix in two tries, else its image back: `sudo docker tag unheaded-dev-<service>:pre-s-waste-out unheaded-dev-<service>:latest && sudo docker compose up -d --no-build --no-deps <service>`. A rolled-back service keeps its old Wotan client: it may poll again (W-01 says so).
4. **Streams still poll after the deploy (W-01 over the bar).** `q 'sum by (instance, topic) (rate(unheaded_wotan_client_polls_total[30m]))'` and each service's `/health` `stream_transport`. A stream that polls because Wotan's gRPC is refused → `sudo docker logs unheaded-wotan --since 10m | grep -i grpc`; TLS profile Strict on 18001 needs a hybrid-capable client (every first-party one is).
5. **The dashboard answers 503 on every metric.** VictoriaMetrics unreachable from it: `sudo docker exec unheaded-dashboard wget -q -O- http://victoria:8428/health` (or the image's equivalent); both must be on `observe`. VictoriaMetrics down → it is the store for everything: start it, `sudo docker compose up -d --no-build --no-deps victoria`.
6. **The verifier refuses the LRU `FLOW_STATE` (Step 274).** Keep `HashMap` and only count the failed insert (STATS key 8); W-23's status says "counted, not bounded". Never ship an object `check-ebpf-loads.sh` did not pass.
7. **The trace collector will not start after Step 343.** Step 345's rollback. The old pair loads; it fills `FLOW_STATE` and drops `FLOW_EVENTS` as before.
8. **Vector refuses the batched config.** Step 378. Vector stopped means the journal keeps every line and Vector catches up from its cursor on restart; `JournalNotShipping` may fire.
9. **Postgres will not start with the new budget.** Step 390. The old limits are known to start.
10. **Migration 050 fails or leaves the wrong shape.** `scripts/well/batch-rollback.sh` with `/srv/backups/unheaded-well/pre-050` (read its usage line); timeguru stays on `unheaded-dev-timeguru:pre-s-waste-out`, which needs no Well.
11. **A gate is red on develop before the sprint.** Prove it in a clean clone at `1393e5ee`, log it under "For Stevie", do not fix it here.
12. **`WotanTopicPermissionRefused` fires after the deploy.** `q 'sum by (caller, action) (increase(unheaded_wotan_topic_permission_refusals_total[15m]))'`. An east unit or an old binary on a removed permission → it is plan 6's or a rolled-back service's; add the permission back on `develop` only with the caller named in the commit, and hand the removal to its plan.

## Appendix B: Agent assignment and dependencies

| Phase | Agent | Lane / worktree | Parallel | Depends on | Time |
|---|---|---|---|---|---|
| 0 Befores | Coordinator | `WT` | no | — | 1h |
| 1 Questions | Coordinator + Stevie | — | yes | 0 | 15m |
| 2 Archive | Coordinator | scratch, `~/tmp/projects` | yes | 0 | 40m |
| 3 The five leave | Coordinator | `WT` | no | 2 | 4h |
| 4 Documents | Sonnet | `-docs` | [P] | 3 | 2h |
| 5 Cuirass | Coordinator | `WT` | after 3 | 3 | 1.5h |
| 6-11 Wotan lane | Opus | `-wotan` | [P] with 4, 12 | 3 (D1-D3 for 8, 9) | 22h |
| 12 Prometheus | Sonnet, then Opus | `-mon` | [P] | 3, D4 | 2.5h |
| 13 Dashboard | Opus | `-dash` | [P] with 14, 15 | 11 | 6h |
| 14 Health, Akira, runtime | Opus | `WT` | [P] with 13, 15 | 11 | 5h |
| 15 eBPF | Opus (root) | `-ebpf` | [P] with 13, 14 | 11, D6, D7 | 3h |
| 16 timeguru, kanban | Opus | `WT` | no | 14 | 5h |
| 17 E-GATE, live befores | Coordinator + Stevie | — | no | 16, the week's end | 1.5h |
| 18 Merge, build | Coordinator (sudo) | main checkout | no | 17 | 2h |
| 19 Deploy | Coordinator + Stevie | main checkout | no | 18 | 1.5h |
| 20 Afters | Coordinator | — | beside 21, 22 | 19 | 1h over 24h |
| 21 Vector, VictoriaMetrics | Coordinator (sudo) | `WT` | after 20's 1 h reading | 19 | 1.5h + 24h |
| 22 The Well sitting | Coordinator + Stevie | `WT` | beside 21 | 19, D8 | 1h + 24h |
| 23 Close | Coordinator | main checkout | no | 20-22 | 1.5h |

At most three lanes build at once (13, 14 and 15 are the widest point). Lanes merge into `sprint/s-waste-out` one at a time; each merge reruns the merged lane's GO-GATE.

**Critical path**: 0 → 2 → 3 → 6 → 7 → 8 → 9 → 10 → 11 → 13 → 16 → (the week's end) → 17 → 18 → 19 → 20 → 23: about 50 h of work plus the waits. Phases 4, 5 and 12 fit beside the Wotan lane; 14 and 15 beside 13; 21 and 22 inside 20's 24 h.

## Appendix C: The findings, and a quick reference

**Findings** (the progress file's rows; owner "this plan" unless named)

| id | seat and row | evidence (2026-11-15) | fix | before | after |
|---|---|---|---|---|---|
| W-01 | Developer D1, D2, D5; BlackMage F4 | 68.8 GET/s on Wotan; a fallen-back stream polls for ever; two recovery paths | Phases 6, 7 | Step 8 | Steps 359-361 (bar ≤ 1/s) |
| W-02 | Developer D18 | `grpcOK: true` by default (`pkg/transport/health.go:36`) | Step 95 | `grep -c 'grpcOK: *true' pkg/transport/health.go` → 1 | the same → 0; Step 360 |
| W-03 | Developer D6 | 63 approved members for 14 processes | Phase 7 | Step 8 | Step 359 |
| W-04 | Developer D3 | 12 discovery polls (24 GET/s); 0 readers | Phases 7, 9 | `grep -rn 'discovery.Start(' --include=*.go cmd services \| grep -v _test \| wc -l` → 12; `grep -rnE 'discovery\.(Watch\|NewResolver)' --include=*.go cmd services \| grep -v _test \| wc -l` → 0 | Step 359 (streams on `system.discovery`) |
| W-05 | Developer D4 | 6 subscribers, no publisher, no room | Phase 8; keys Step 317 | Step 8 (no room) | Step 359 |
| W-06 | Developer D7; Archimedes F6 | six trace rooms, 39.3 of 52.8 MB | Phase 10 | Step 8 | Step 359 |
| W-07 | BlackMage F5 | 367 answers 429 in 7 d | Phase 11 | Step 8 | Step 361 |
| W-08 | BlackMage F11 | `_, _ = a.w.Write(data)` (`pkg/auth/audit.go:101`) | Phase 11 | `grep -c '_, _ = a.w.Write' pkg/auth/audit.go` → 1 | Step 359 (series present) |
| W-09 | Developer D19, D21; decision 7 | architect, captain, micromanager: empty data, probes only | Phases 2-4, 19 | Steps 9, 11 | Step 356 |
| W-10 | Developer D20; decision 7 | monad demo handlers; sophia's empty loop, 60 log lines a minute | Phases 2-4, 19 | Steps 9, 11 | Step 356 |
| W-11 | Developer D17; decisions 7, 14 | Cuirass reconciles a mock; `cuirass.*` rooms nobody reads | Phases 5, 19 | Step 8 | Step 356 |
| W-12 | Erda F4; decisions 9, 10 | Prometheus 13,730 series, 801 MB, 30 d, beside VictoriaMetrics | Phase 12 | Step 10 | Step 361 |
| W-13 | Developer D11 | dashboard 289 MiB, 13.2 MB/s allocated, a private series store | Phase 13 | Step 10; Step 208 | Step 361 |
| W-14 | Developer D12, D14 | three health pollers; host metrics stored three ways | Phase 13 | Step 13 (`/health` 6.46/s) | Step 361 |
| W-15 | Developer D13 | 98% of dashboard requests are 401 from a polling page | Phase 13 | Step 10 | Step 361 |
| W-16 | Developer D15 | 241 log lines a minute on west, mostly probes and refusals | Phases 13, 14 | Step 9 | Step 361 |
| W-17 | Developer D8; Archimedes F4 | Akira throttled 44%; a Python start every 30 s | Phase 14 | Step 11 | Step 361 |
| W-18 | Developer D9 | 18 zombies in Akira | Phase 14 | Step 11 | Step 359 |
| W-19 | Developer D10 | consensus and remediation refused 49 and 12 times | Step 317 (Stevie's keys) | Step 10 | Step 361 |
| W-20 | Erda F3; Developer D16 | 0.68 health upserts a second, 73% of WAL flushes | Phase 14 | Step 13; Step 309 | Step 360 |
| W-21 | Developer D22, 1.6 | GOMAXPROCS unexported; six containers without `GOMEMLIMIT` after the removals | Phase 14 | `python3 -c 'import yaml; s=yaml.safe_load(open("docker-compose.yml"))["services"]; print([k for k,v in s.items() if "build" in v and (v.get("deploy") or {}).get("resources",{}).get("limits",{}).get("memory") and "GOMEMLIMIT" not in str(v.get("environment",""))])'` → 11 names (6 after the removals); `go_sched_gomaxprocs_threads` absent | Steps 257, 359 |
| W-22 | Archimedes F4 | `healthprobe` every 10 s on 14 containers | Phase 14 | `grep -c 'bin/healthprobe' docker-compose.yml` → 14, all 10 s | Step 243; Step 361 |
| W-23 | BlackMage F7; decision 19 | `FLOW_STATE` hash, full on east, ~30 h on west, failures ignored | Phase 15 | Step 12 | Step 344 |
| W-24 | BlackMage F8 | `FLOW_EVENTS` undrained, 94% dropped | Phase 15 (D6) | Step 12 | Step 344 |
| W-25 | Erda F9 | timeguru on tmpfs SQLite; the Well's table empty, wrong shape | Phase 16 | `grep -n 'DB_PATH' docker-compose.yml`; Step 310 | Step 359 |
| W-26 | Erda F10; decision 24 | kanban's SQLite fallback of 2026-10-28 | Phases 16, 22 | `grep -c 'openSQLiteStore(' cmd/kanban-app/main.go` → 2; Step 310 | Step 360 |
| W-27 | Archimedes F7 | rooms × room size allows 78 GiB | none: `-buffer-bytes-total` 64 MiB bounds it (`services/wotan/cmd/wotan/main.go:485`) | — | closed |
| W-28 | Erda F1, Archimedes F3 | Vector inserts every second: 47,386 inserts, 42 GiB merged, 71 GB written a day | Phase 21 | Step 311 | Step 379 |
| W-29 | Erda F12 | Vector's `host` = container id; no series limit | Phase 21 | Step 312 | Step 379 |
| W-30 | Erda F2; Archimedes F3; decision 5 | Postgres 256 MiB: limit hit 52,740 times, cache hit 0.683 | Phase 22 (D8) | Step 313 | Step 392 |
| W-31 | Erda F16; decision 24 | `unheaded-well-data`, three `_0` tables, three empty volumes, `kanban.db` | Phases 19, 22 | Step 314 | Step 397 |

**New series**

| Series | Type | From |
|---|---|---|
| `unheaded_wotan_client_polls_total{topic}` | counter | Phase 6 |
| `unheaded_wotan_client_stream_transport{topic,transport}` | gauge | Phase 6 |
| `unheaded_critical_alerts_active`, `unheaded_critical_alerts_state{state}`, `unheaded_critical_alerts_refused_total{reason}` | gauge, gauge, counter | Phase 8 |
| `alert_mailer_critical_publish_refused_total`, `alert_mailer_critical_publish_errors_total` | counter | Phase 8 |
| `sword_deploys_held_total` | counter | Phase 8 (D2 = B) |
| `unheaded_discovery_refused_total{reason}` | counter | Phase 9 |
| `unheaded_auth_audit_write_failures_total` | counter | Phase 11 |
| `unheaded_well_health_reports_skipped_total`, `unheaded_well_health_heartbeat_rows_full_total` | counter | Phase 14 |
| `go_sched_gomaxprocs_threads`, `go_gc_gomemlimit_bytes`, `go_gc_gogc_percent` | gauge | Phase 14 |
| `trace_collector_flow_state_insert_failures` | gauge | Phase 15 |

**New rules**: `AuditWriteFailing` (Phase 11), `VictoriaMetricsSeriesLimitReached` (Phase 21). **Removed**: topics `ebpf.packet.events`, `ebpf.flow.events`, `ebpf.latency.events`; map `FLOW_EVENTS`; the dashboard's `scraper_series`, `metrics_series`, `series_count` status fields.

**Paths**: `WT=/home/govan/tmp/wt/S-WASTE-OUT`; lanes `-docs -wotan -mon -dash -ebpf`; scratch `/home/govan/tmp/wt/S-WASTE-OUT-scratch` (`lib.sh`, `role-paths.txt`, `dash-fields.txt`, `bin/`, `ps-before.txt`); archive `/home/govan/tmp/projects/unheaded-role-services`; rollback images `unheaded-dev-<service>:pre-s-waste-out`; host rollback `/var/backups/s-waste-out/`; Well backup before 050 `/srv/backups/unheaded-well/pre-050`; eBPF objects built in `ebpf/target/load-gate/default/bpfel-unknown-none/release/`, installed in `/usr/local/lib/unheaded/ebpf/`; VictoriaMetrics `127.0.0.1:8428`; Prometheus `127.0.0.1:9090`; collector metrics `127.0.0.1:16671`.

**Containers**: `unheaded-wotan`, `unheaded-timeguru`, `unheaded-akira`, `unheaded-dashboard`, `unheaded-kanban`, `unheaded-auth`, `unheaded-logstore-writer`, `unheaded-sword`, `unheaded-postgres`, `unheaded-clickhouse`, `unheaded-vector`, `unheaded-victoria`, `unheaded-prometheus`, `unheaded-alert-mailer`. Host units: `unheaded-trace-collector`, `unheaded-chaos-controller`, `unheaded-kev-poller`.

**Gates**: GO-GATE (Legend); `scripts/check-gosec-ratchet.sh`; `scripts/check-prometheus-rules.sh` (with the keep-list `--check`); `scripts/test-prom-keep-list.sh`; `scripts/check-unit-ceilings.sh` (units and compose); `scripts/check-ebpf-loads.sh` (root), `scripts/check-ebpf-insns.sh --from`; `scripts/test-vector-config.sh`; `scripts/well/test-config.sh`, `test-migrate.sh`, `test-health-bridge.sh`; `scripts/check-gates-can-fail.sh --only <gate>`; `scripts/ci-local.sh`.

---

*S-WASTE-OUT Battle Plan, forged 2026-11-15 (Phases 0-10); completed 2026-11-15 (Phases 11-23, appendices), re-grounded at `1393e5ee`*
*24 Phases. 414 Steps. Five services and a mock leave, every poll becomes a stream, and each fix carries its own before and after.*
