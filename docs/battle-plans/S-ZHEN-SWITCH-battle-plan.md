<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# S-ZHEN-SWITCH BATTLE PLAN: 11 Phases, 330 Steps

**Date**: 2026-11-15
**Sprint**: S-ZHEN-SWITCH, plan 3 of the ADR-147 series (`docs/battle-plans/SPRINT-SERIES-2026-11-15-ADR-147.md`). The platform is whole without Zhen; one switch turns Zhen on as an optional module; Zhen's API and UI never answer on the public side of a host.
**Prerequisite**: ADR-147 decision 15 (Stevie, 2026-11-15, verbatim in the ADR). The eight choices in "Decisions" below are asked at Steps 5 and 6. Live steps wait for gate **G-EW** (the log store week has ended, Step 18). Zhen's model server (llama-server) stays paused on west: no step starts it.
**Target**: With the switch off on west: no Zhen unit runs, no Zhen scrape job or rule is loaded, no Zhen health target is registered, role `app_zhen` holds no connection, and no platform log line names Zhen. With it on, all of those appear together within 90 s. `scripts/test-zhen-switch.sh` proves both states. The health sweep runs on a platform timer and can pass; the IOC audit has a platform timer that Stevie's approval enables. Zhen's tables live in `unheaded_zhen` with their own backup set and a connection limit. Two gates fail if a Zhen listener is on a wildcard or globally routable address.
**Estimated duration**: 30 to 36 h of work over 6 or 7 sessions (Appendix B), plus one attended sitting of about 2 h for the Well (Phase 8B).
**Agent strategy**: Coordinator (Opus) for Phases 0, 1, 5, 8, 9, 10. Phases 2, 3, 6 and 7 run as `[P]` lanes in their own worktrees after Phase 1; Phase 4 follows Phases 2 and 3 (it moves `rules/zhen.yml` after Phase 2 edits it, and its test reads what Phase 3 removed). At most three builders at once (series rule 3). Phase 7's throwaway-Well tests write to Docker's data on the spinning disk, so they wait for G-EW like every live step.
**Commit cadence**: every 5 steps, at every exit gate, before every skip. Signed; `--no-gpg-sign` if gpg-agent has timed out, logged in the progress file for `scripts/resign-unpushed.py`. Named files only. Message `[PLAN S-ZHEN-SWITCH] Steps X-Y: ...` plus the two trailers the executing session's attribution reminder gives. Stevie pushes.
**Stuck protocol**: skip after 3x the step estimate or two failed debug attempts. Mark the step `[STUCK]`, its dependents `[BLOCKED]`, commit, go to the next independent phase. Questions go under "For Stevie" in the progress file.

## Grounding (checked 2026-11-15 ~20:00Z against develop `f9b08c88` and the live west, read-only; every row re-checked ~21:30Z against develop `1393e5ee`, whose only change is ADR-113's dated update; rows G3, G19 corrected and G20-G25 added then)

| # | Claim (seat, row) | True now | Plan does |
|---|---|---|---|
| G1 | Akira's default targets name `zhenai`, `inference`, `gateway` (Architect A7, Micromanager D11) | True: `cmd/akira/main.go:194,197,199`. West's compose Akira runs with `config/akira/akira.yaml`, which names no Zhen target, so the defaults bite only where Akira runs with no `--config` (east). **Missed by the seats:** `configs/akira-west.yaml:6-7` names `inference` 20100 | Remove the three defaults and the `inference` row (Phase 3). East gets them with its next debs (plan 6) |
| G2 | dashboard-backend's health list (Developer 1.7) | `DefaultServiceEndpoints` has no Zhen entry. `config/dashboard/services.txt:25,29` adds `zhenai` and `llama-server`. One unhealthy target makes the whole dashboard `unhealthy` (`internal/health/health.go:778-781`) | The two rows move to a module fragment; module targets stop deciding the overall state (Phase 3) |
| G3 | 10 of 13 tables in `unheaded_app` are `zhen_*`; `app_zhen` holds 3 connections; no limit (Erda F8) | Migrations create 8 `zhen_*` tables; Erda measured 10 live (not re-read: evidence week; Step 19 lists them). **Missed by the seats:** `zhen_agent_runs.card_id REFERENCES kanban_tasks` (`032:54`); the worker claims a card in one transaction over three tables, `kanban_tasks`, `kanban_zhen_optin` (049) and `zhen_agent_runs` (`pkg/zhenagent/sqlstore.go:72-113`); kanban-app reads `zhen_agent_runs` (`041`, `cmd/kanban-app/runbooks.go:433-440`); `cmd/well-retention/targets.go:46-121` purges `zhen_conversations` and `zhen_captures`, partition-checks `zhen_agent_runs`, and holds every soft-deleted card a run names with a sub-select on `zhen_agent_runs`; `zhen-agentd` opens seven `app_zhen` pools, 21 connections (Erda F6; `cmd/zhen-agentd/{audit,memory,scheduler,main,worker_main}.go`). A second database cuts through all of them | Own database with a two-step claim (decision D2), Phase 7; live in Phase 8B. `kanban_zhen_optin` stays in `unheaded_app`: the board owns it and is its only writer |
| G4 | Zhen's tables ride in the nightly dump (Erda F8) | `scripts/db-backup.sh:118-119` dumps every database that accepts connections, so a new database would ride along too | An exclude list for the platform's set, a set of its own for Zhen (Phase 7) |
| G5 | 2 GiB of huge pages for the model server (Architect A3, Archimedes F5) | `scripts/bootstrap-llm-lab.sh:866`, `/etc/sysctl.d/99-unheaded.conf` | Handoff to plan 4 (decision 12). Not touched here |
| G6 | Zhen's rules sit in the platform's rule set; `ZhenSchedulerStalled` fired 3,617 min (Micromanager D11, BlackMage F17) | `monitoring/prometheus/rules/zhen.yml`: 3 rules, loaded by `rule_files: 'rules/*.yml'`. Not firing now: the daemon has run since 2026-11-12 06:41Z | Rules move to the module and load only while the switch is on (Phase 4) |
| G7 | A scrape job assumes Zhen (Developer 1.7) | `monitoring/prometheus/prometheus.yml:116` and `config/victoria/scrape.yml:253`; `up` is 1 | Both jobs move to module fragments (Phase 4) |
| G8 | The IOC audit and the health sweep are scheduled inside `zhen-agentd -scheduler` (Developer D23, BlackMage F17) | **Worse than written. Contradicts ADR-147's wording.** The IOC audit is scheduled nowhere: its job is commented out in `configs/zhen-schedules.yaml:38-48` (the runbook is `status: draft`, approval open since 2026-11-12), no `schedule="ioc-audit"` series exists, so `IOCAuditNotSucceeding` cannot fire. The health sweep is a job, but the scheduler has been paused since 2026-11-12 (`~/tmp/next.md`): its last success is the load time, 306,110 s ago. Neither check has run for 3.5 days. The sweep also cannot pass: it probes plain http and west's listeners are TLS (18000 and 20000: http 400, https 200), and it names `zhen-inference` and `zhen-ui-front` | Platform timers; the sweep is fixed so it can pass (D3); the audit's timer ships disabled until Stevie approves the runbook (D7). Phase 2 |
| G9 | The vor unit is "for Zhen" (Micromanager D11) | `deploy/systemd/unheaded-vor.service:2,19`. ADR-147 names vor one of Stevie's three real applications | Description corrected; vor stays outside the switch (D4) |
| G10 | The navigation bar always links Zhenai (Developer 1.7) | `pkg/bff/nav.go:46` | Entry only while the module is on (Phase 3) |
| G11 | Kanban carries a Zhen opt-in (Developer 1.7) | `cmd/kanban-app/zhenoptin.go`; `agentRunFor` warns once when `zhen_agent_runs` cannot be read (`runbooks.go:484`): after the move that would be a warning about Zhen being absent | Both silent and hidden while the module is off (Phase 3) |
| G12 | Where Zhen listens (decision 15c) | See "Where Zhen listens today" below. No check fails if a drop-in or a flag moves a listener. `zhen-agentd -host 0.0.0.0` is accepted. West's wifi interface holds global IPv6 addresses, so a wildcard bind is a public bind today. `deploy/nftables/west.nft:58,87` admits 20103 from the house LAN (Stevie 2026-11-03; A-16 "prove it, then open"); the file is not applied (plan 4) | A bind rule in the binaries, a static gate, a live guard with an alert (Phase 6) |
| G13 | "No Zhen container" (the brief) | Zhen has no container. It is host units: `zhen-agentd` and `unheaded-zhen-ui-front` run; `unheaded-llama` and `unheaded-zhen-app` are not installed on west. A compose profile would switch nothing | The switch is a systemd target (D1) |
| G14 | (own sweep) | `Makefile:76`: `build-services` builds `zhen-rag` | A `build-zhen` target; out of `build-services` (Phase 3) |
| G15 | Allowed-caller lists name Zhen (Developer 1.7, Architect) | `configs/wotan.yaml:52-53,73-74,218`, `docker-compose.yml:767,1428`, `deploy/systemd/unheaded-auth.service:36`. An entry with no caller raises nothing | Stay. Listed in the ADR as "permitted, not required" |
| G16 | `pkg/ports`, `pkg/uids` name Zhen (Developer 1.7) | Registries | Stay |
| G17 | (own sweep) | `zhen-mcp`, `zhen-cli`, `zhen-claude` are tools a person starts; `config/logging/sources.yaml` is an inventory | Stay outside the switch |
| G18 | (own sweep) | `services.txt` reaches `zhenai`, `llama-server`, `vor` and `wiki` at `172.28.1.1`; on 2026-11-15 nothing listened on that address | Measured at Step 15; a red row that is not Zhen's goes to plan 1 under "For Stevie" |
| G19 | (own sweep) | **Corrected 21:30Z:** ADR-148 is free on `1393e5ee` (a Bazel ADR was written and removed from history the same day; `~/tmp/next.md` names 148 the next free number). Migration `049` is the last; this plan takes 050, 051, 052. uids 16714 to 16719 are free (S-WASTE-OUT retires 16723-16727, 16750, 16751; S-ZHEN-FRONT holds 16764, 16792) | Take the next free number at execution and record it |
| G20 | (own sweep) | S-ZHEN-FRONT Phases 10-12 are open and attended: they install `unheaded-zhen-app` and `unheaded-llama` (disabled), start them when Stevie lifts the pause, open 20103 on the house LAN, and prove "comes back after a boot" | Phase 8 rewires whichever of those units is installed; a unit installed later takes this plan's unit file. The LAN address goes on west's bind allowlist when S-ZHEN-FRONT opens it (D5) |
| G21 | (own sweep) | `cmd/alert-policy` requires `urgency: none` on every hand-written rule and gives urgency only through `promote:` in `observability/alerting/policy.yaml`; it reads only `monitoring/prometheus/rules/*.yml` (`cmd/alert-policy/main.go:375-415`). `scripts/check-prometheus-rules.sh` reads only that directory too | Steps 62 and 65 use `urgency: none` plus a promotion; Phase 4 teaches both tools the module rule directories |
| G22 | (own sweep) | Prometheus v3.15.1 runs without the lifecycle API ("reload with `docker kill -s HUP`", `monitoring/docker-compose.yml`); VictoriaMetrics v1.155.0 reads `-promscrape.config` once | Phase 4: Prometheus's `auto-reload-config` feature and VictoriaMetrics's `-promscrape.configCheckInterval`, both checked against the pinned images first; HUP from the switch's oneshot is the fallback |
| G23 | (own sweep) | `unheaded_ops.llm_requests` (030, 044) audits every model call, Claude's included; it exists empty without Zhen | Stays (permitted, not required); named in ADR-ZS |
| G24 | (own sweep) | The throwaway Postgres servers of `scripts/well/test-*.sh` keep their data in Docker's storage, which is on the spinning disk | Those tests wait for G-EW |
| G25 | (own sweep) | The dashboard and kanban run in containers; `/run/unheaded/modules` must be bind-mounted into them, and into Prometheus and VictoriaMetrics, for any of them to see a module | Phase 4 adds the mounts; Phase 8A recreates the four containers one at a time |

### Where Zhen listens today (measured with `ss -ltnH`, and read from the units)

| Listener | Bind | What holds it there |
|---|---|---|
| llama-server | `127.0.0.1:8081` (not installed on west) | `--host 127.0.0.1`; `IPAddressDeny=any`, `IPAddressAllow=localhost`, `SocketBindAllow=tcp:8081` |
| Zhen app (Flask) | Unix socket `/run/unheaded-zhen-ui/app.sock`; no TCP (ADR-138) | `IPAddressAllow=localhost`; socket directory `2750` |
| zhen-ui-front | `127.0.0.1:20103` (TLS 1.3), `127.0.0.1:20112` (ops) | `IPAddressAllow=localhost`; `SocketBindAllow` for the two ports; the binary refuses a non-loopback ops address. `--listen` may repeat with a LAN address (by design) |
| zhen-agentd API | `127.0.0.1:20111`, a token on every route | the unit's `-host 127.0.0.1` only: no address filter, and the binary accepts `0.0.0.0` |
| zhen-agentd metrics | `[fd00:dead:beef::1]:20111` (WireGuard), `/metrics` only | `ZHEN_METRICS_ADDR` |
| vor | `127.0.0.1:20110` | `IPAddressAllow=localhost` |
| compose | no Zhen port published | — |

**Marshal (scope).** IN: every row above with a "Plan does". OUT, with a home: alerts and instruments in general, the urgency of rules, per-unit memory series (plan 1); removing role services and their rows in the same files, Wotan's client (plan 2); huge pages, swap, the input filter (plan 4); the LXD instance, slices and shares (plans 7 and 8). No interface work: a result is read from a metric or a terminal (series rule 22). No new third-party code: systemd, bash, Python's standard library and PyYAML (already on the host), Go's standard library, Prometheus and VictoriaMetrics flags that ship in the pinned images. No numbers for shares (series rule 20): unit limits are restated from the unit files, never changed.

**Micromanager (QA / DoD).** Code is red then green under `go test -race`. Every fix has a before and an after in `$PROG`, measured the same way (series rule 15). Rules pass `scripts/check-prometheus-rules.sh`. Runbook edits pass their pytest files and `scripts/runbook-manifest.sh --write`. Each new gate is registered in `scripts/check-gates-can-fail.sh` and seen to fail. Done means `scripts/test-zhen-switch.sh --expect off` and `--expect on` both pass on west, with the numbers in `$PROG`.

**Busboy (handoffs).** Plans 2 (S-WASTE-OUT) and 4 (S-WEST-GROUND) are on disk beside this one; plan 1 (S-SEE-AND-SURVIVE) was not at 21:30Z. Shared with plan 2: `cmd/akira/main.go` `defaultTargets` and `main_test.go:141-144`, `config/dashboard/services.txt`, `Makefile:76`, `config/victoria/scrape.yml`, `monitoring/prometheus/prometheus.yml`, `docker-compose.yml`, `pkg/uids` (it retires seven ids), `cmd/zhen-agentd` (its Step 143 holds unattended runs while `alerts.critical` is active): keep both sides of every conflict. Shared with plan 4: huge pages (Z11 is theirs), `deploy/nftables/west.nft` (they leave 20103 on the house LAN, which D5 (a) allows only with the address on west's allowlist), the backup sets' disk rule (`WELL_BACKUP_OTHER_DISK=1`: Zhen's set follows whatever plan 4 sets for the platform's), and their database move: Phase 8B and their move are never in the same sitting. Shared with plan 1: rule urgencies, `policy.yaml`, the textfile directory's ACL. `db/migrations/MANIFEST` and `pkg/uids`: take the next free number, never the one written here if it is gone. `runbooks/security/ioc-audit.yaml` stays `status: draft`: the approval is Stevie's (A-1 of 2026-11-12, asked again as D7). The switch test stops `zhen-agentd` and the UI front for a few minutes and returns west to the state it found (D8).

## Decisions (asked at Steps 5 and 6; the plan continues on independent work)

| # | Question | Options | Recommendation | Blocks |
|---|---|---|---|---|
| D1 | What is the switch? | (a) the systemd target `unheaded-zhen.target`: Zhen's units are part of it, and a oneshot publishes the module's scrape, rule and health fragments under `/run/unheaded/modules/zhen/` while it is active; (b) a compose profile; (c) a key in `/etc/unheaded` that every service reads | **(a)**. Zhen is host units, not containers (G13); a target is one object with one state; `/run` cannot disagree with it after a reboot; plan 7 swaps the units behind the target for an LXD instance and keeps the rest | Phases 3 (module-aware steps), 4, 5 |
| D2 | Where do Zhen's tables go? | (a) database `unheaded_zhen`; the claim becomes two steps (run row first, card second, undone on a miss); kanban reads runs from it only while the module is on; (b) a schema `zhen` inside `unheaded_app`: keeps the foreign key and the transaction, shares the database; (c) own database and Zhen reaches the board only through kanban-app's API | **(a)**: decision 15 says a database, and it can move into Zhen's instance later (plan 7). (c) is the cleaner end state and a larger change: park it as an ADR | Phase 7, Phase 8B |
| D3 | The health sweep | (a) fix it (targets from a file, the right scheme, a failing exit code) and run it on a platform timer; (b) retire it: Akira sweeps every 30 s | **(a)**: decision 15 names it | Steps 49-57 |
| D4 | vor | (a) its own application, outside the switch; Zhen is one client; (b) part of the Zhen module | **(a)**: ADR-147 item 12 names vor as one of three real applications; the wiki's search uses it with Zhen off | Step 104 |
| D5 | What is the "public side" for the gates? | (a) a wildcard bind or a globally routable address is public and always fails; a private non-loopback address passes only if it is on a host allowlist, empty by default (west gets the WireGuard metrics address now, the house LAN address only when S-ZHEN-FRONT opens it, A-16); (b) loopback and WireGuard only, everywhere, which withdraws A-16 | **(a)** | Steps 162-189, 262 |
| D6 | `app_zhen`'s connection limit; does Zhen's backup set go to east? | A number; off-site yes or no | **12** (the pools' caps sum to 10 after Phase 7, plus 2; a cap, not a share: re-read in plan 8). **Local only** until plan 7 gives Zhen its instance | Phase 7, Phase 8B |
| D7 | Approve `runbooks/security/ioc-audit.yaml` and seal the manifest? | yes / not yet | Stevie's (open since 2026-11-12). Until yes the audit's timer is installed and disabled | Step 308 |
| D8 | The state the switch is left in on west | on, as found / off | **On, as found**: Stevie paused the model server, not the daemon | Step 310 |

Coding on a recommendation is allowed in the sprint worktree only. No merge to develop and no live step rests on an unanswered decision.

---

## LEGEND

- `[B]`: bash command, run directly
- `[V]`: verification; must pass before going on
- `[D]`: debug step, only if the step before it failed
- `[W]`: write or create a file
- `[R]`: read or inspect a file
- `[S]`: needs sudo (always Opus)
- `[P]`: can run in parallel with other `[P]` phases
- `[C]`: commit checkpoint (signed, named files)
- `[STEVIE]`: Stevie runs it or says yes first; `[STEVIE][DECIDE]`: a choice with options and a recommendation
- `[STUCK]` / `[BLOCKED]`: set by the Skip Protocol

`~Nm` is the expected wall-clock time. `REPO=/home/govan/tmp/unheaded`, `WT=/home/govan/tmp/wt/S-ZHEN-SWITCH` (lanes: `$WT-p2`, `-p3`, `-p4`, `-p6`, `-p7`, branches `sprint/s-zhen-switch-pN`), `PROG=docs/battle-plans/progress/S-ZHEN-SWITCH-progress.md` inside the worktree in use, `PYTEST=/var/tmp/unheaded-sprint/civenv/bin/pytest`, `ADR-ZS` = the ADR written at Step 24. A lane starts with `git worktree add $WT-pN -b sprint/s-zhen-switch-pN sprint/s-zhen-switch` and ends merged into `sprint/s-zhen-switch` with `git merge --ff-only` or `git merge -S`.

The VictoriaMetrics reader used throughout:
```bash
q(){ curl -s -m5 -G http://127.0.0.1:8428/api/v1/query --data-urlencode "query=$1" | python3 -c 'import json,sys; d=json.load(sys.stdin)["data"]["result"]; [print(r["metric"], r["value"][1]) for r in d] or print("(no series)")'; }
```

**Code gates (series 2026-11-04 rule 6), run at the end of every code phase from the lane's worktree:**
```bash
test -z "$(gofmt -l cmd pkg services)" && go vet ./... && GOMAXPROCS=4 go test -race -count=1 ./cmd/... ./pkg/... 2>&1 | grep -v '^ok\|no test files' | tail -15; ./scripts/check-gosec-ratchet.sh 2>&1 | tail -2
```
Expected: no `FAIL` line; the ratchet prints `[PASS]`. Any other output: fix it in this phase's files, two tries, then `[STUCK]`.

---

## PHASE 0: PRECONDITIONS, DECISIONS, THE "BEFORE" READINGS (Steps 1-21)

**Goal**: Branch ready, decisions asked, and every assumption measured before anything changes.
**Prerequisite**: west up with the stack (VictoriaMetrics on `127.0.0.1:8428`, Prometheus on `127.0.0.1:9090`).
**Time**: 60 min.
**Agent**: Coordinator.
**Model**: Opus.

- [ ] **Step 1** [R] ~15m: Read ADR-147 ("Zhen assumed", decision 15 with Stevie's words), this plan's Grounding, `docs/adr/ADR-138-*.md`, `deploy/systemd/{zhen-agentd,unheaded-zhen-ui-front,unheaded-zhen-app,unheaded-llama,unheaded-vor,unheaded-ci-status}.service`, `configs/zhen-schedules.yaml`, `monitoring/prometheus/rules/zhen.yml`, `db/migrations/MANIFEST`, `db/migrations/032_app_zhen_agent_runs.sql`, `pkg/zhenagent/sqlstore.go`, `scripts/db-backup.sh`, the "Zhen" lines of `~/tmp/next.md`.
- [ ] **Step 2** [B] ~1m: Worktree off develop.
  ```bash
  cd /home/govan/tmp/unheaded && git switch develop && test -L /home/govan/tmp/wt/projects || ln -sfn /home/govan/tmp/projects /home/govan/tmp/wt/projects; git worktree add /home/govan/tmp/wt/S-ZHEN-SWITCH -b sprint/s-zhen-switch develop && git -C /home/govan/tmp/wt/S-ZHEN-SWITCH log --oneline -1
  ```
- [ ] **Step 3** [W] ~8m: Create `$PROG`: header; "Decisions" (D1-D8, empty rulings); "Findings" with the columns `id | seat and row | evidence | fix | owner | before | after | status` and the 15 rows of Appendix C's findings table, `before` and `after` empty; "Log"; "For Stevie"; "Handoffs" (plans 1, 2, 4, 6, 7).
- [ ] **Step 4** [V] ~1m: `cd /home/govan/tmp/wt/S-ZHEN-SWITCH && grep -c '^| Z' docs/battle-plans/progress/S-ZHEN-SWITCH-progress.md` prints 15.
- [ ] **Step 5** [STEVIE][DECIDE] ~5m: Ask D1, D2, D3, D4 as one AskUserQuestion round (four questions, the recommended option first and marked Recommended, options and reasons from the Decisions table).
- [ ] **Step 6** [STEVIE][DECIDE] ~5m: Ask D5, D6, D7, D8 as a second round.
- [ ] **Step 7** [W] ~3m: Write each answer verbatim under "Decisions" in `$PROG`. Stevie away: write "unanswered, <date>" beside each, put the eight questions under "For Stevie", and go on: every phase may be coded on the recommendation in its worktree; nothing merges to develop and nothing live runs on an unanswered decision ("Blocks" column). An answer that differs from the recommendation: stop the phases it blocks, mark their steps `[BLOCKED]`, write under "For Stevie" that the plan needs an amendment for that answer.
- [ ] **Step 8** [B] ~1m: Tools present (never install a missing one).
  ```bash
  for t in go docker python3 curl ss systemctl promtool shellcheck; do command -v $t >/dev/null && echo "$t ok" || echo "$t MISSING"; done; test -x /var/tmp/unheaded-sprint/civenv/bin/pytest && echo "pytest ok"; python3 -c 'import yaml; print("yaml", yaml.__version__)'
  ```
- [ ] **Step 9** [V] ~1m: `go`, `docker`, `python3`, `curl`, `ss`, `systemctl`, `pytest ok`, `yaml 6.x`. `promtool` and `shellcheck` may be MISSING: the rule gate runs promtool in a container; skip shellcheck lines where they appear. Log the list.
- [ ] **Step 10** [B] ~2m: Z1 before.
  ```bash
  cd /home/govan/tmp/wt/S-ZHEN-SWITCH && sed -n '/^var defaultTargets/,/^}/p' cmd/akira/main.go | grep -c -E '"(zhenai|inference|gateway)"'; grep -c 'inference' configs/akira-west.yaml; ssh east 'journalctl -u unheaded-akira --since "-10 min" --no-pager -o cat 2>/dev/null | grep -c -E "zhenai|inference|gateway"'
  ```
  Expected: `3`, `1`, and a count above 0 from east (Architect 1.6: each is refused every sweep). Any other output from east (0, or an error): log it as it is; it is a reading, not a gate.
- [ ] **Step 11** [B] ~2m: Static befores (Z2, Z4, Z7, Z8, Z10, Z3).
  ```bash
  cd /home/govan/tmp/wt/S-ZHEN-SWITCH && grep -c -E '^(zhenai|llama-server)=' config/dashboard/services.txt; grep -c 'for Zhen' deploy/systemd/unheaded-vor.service; grep -c 'ID: "zhenai"' pkg/bff/nav.go; sed -n '/^build-services:/p' Makefile | grep -c 'build-zhen-rag'; grep -c -i zhen monitoring/prometheus/prometheus.yml config/victoria/scrape.yml; grep -c 'alert:' monitoring/prometheus/rules/zhen.yml; grep -c -E '^  - name: service-health-sweep' configs/zhen-schedules.yaml
  ```
  Expected: `2`, `1`, `1`, `1`, two `file:count` lines with counts above 0, `3`, `1`.
- [ ] **Step 12** [B] ~2m: Z3 before: what Zhen's scheduler has run.
  ```bash
  q 'time() - zhen_scheduler_job_last_success_timestamp_seconds'; q 'time() - zhen_scheduler_heartbeat_timestamp_seconds'; systemctl show zhen-agentd -p ActiveEnterTimestamp; systemctl list-timers --all --no-pager | grep -c -E 'health-sweep|ioc-audit'
  ```
  Reference 2026-11-15 20:00Z: no `schedule="ioc-audit"` series; `service-health-sweep` 306,110 s, equal to the time since the daemon started 2026-11-12 06:41Z (never run: the scheduler is paused); heartbeat under 60 s; timers `0`.
- [ ] **Step 13** [B] ~2m: Z4 before: the rule group and how long its alert has fired.
  ```bash
  curl -s -m5 http://127.0.0.1:9090/api/v1/rules | python3 -c 'import json,sys; print(sorted((g["name"], len(g["rules"])) for g in json.load(sys.stdin)["data"]["groups"] if g["name"]=="zhen"))'; q 'count_over_time(ALERTS{alertname="ZhenSchedulerStalled",alertstate="firing"}[14d])'; q 'up{job="zhen-agentd"}'
  ```
  Expected: `[('zhen', 3)]`; a sample count (one a minute while firing; the seat counted 3,617 minutes); `up` 1.
- [ ] **Step 14** [B] ~1m: Z6 before: Zhen's listeners.
  ```bash
  ss -ltnH | awk '{print $4}' | grep -E ':(8081|20100|20103|20110|20111|20112)$' | sort
  ```
  Reference: `127.0.0.1:20103`, `127.0.0.1:20110`, `127.0.0.1:20111`, `127.0.0.1:20112`, `[fd00:dead:beef::1]:20111`. A line starting `*:`, `0.0.0.0:` or `[::]:` is a finding: write it at the top of "For Stevie" and go on.
- [ ] **Step 15** [S][B] ~3m: Z2, Z9 before: platform log lines that name Zhen, 10 minutes.
  ```bash
  for c in unheaded-dashboard unheaded-akira unheaded-kanban; do printf '%s ' $c; sudo docker logs --since 10m $c 2>&1 | grep -c -i -E 'zhen|llama|inference'; done; U=$(systemctl list-units 'unheaded-*' --all --no-legend --plain | awk '{print $1}' | grep -v -E 'zhen|llama|vor' | sed 's/^/-u /' | tr '\n' ' '); printf 'units '; sudo journalctl --since "-10 min" --no-pager -o cat $U 2>/dev/null | grep -c -i -E 'zhen|llama'
  ```
  Zhen's own units are left out (their lines are the module's), and so is vor (its corpus source is named `zhen-taught`: a data name, not an assumption). Log the four counts. If the dashboard's lines also name `vor` or `wiki` as unhealthy (G18), copy one line of each to "For Stevie" for plan 1.
- [ ] **Step 16** [B] ~3m: Z13 before: the sweep as written, once, by hand (18 local GETs).
  ```bash
  cd /home/govan/tmp/unheaded && python3 -B scripts/run-runbook.py runbooks/observe/service-health-sweep.yaml 2>&1 | grep -E '\[(OK|FAIL)\]|RESULT' | sed 's/ —.*//' | head -25
  ```
  Reference: `[FAIL]` for every TLS listener (wotan, dashboard-backend) and for `zhen-inference`. Log the `RESULT` line.
- [ ] **Step 17** [B] ~1m: Z5 before, without touching Postgres: `q 'unheaded_well_backup_databases'; q 'unheaded_well_backup_rows'`. Log both.
- [ ] **Step 18** [V] ~2m: **Gate G-EW.** `date -u +%F; grep -n -i 'log store week' /home/govan/tmp/unheaded/docs/battle-plans/progress/S-E1-LIVE-progress.md | tail -3`. G-EW is **open** only if that file records the week's end (S-E1-LIVE Step 284 closed) or Stevie says so in this session. A date alone does not open it. Write "G-EW open" or "G-EW closed" with the evidence in `$PROG`.
- [ ] **Step 19** [S][B] ~2m: Z5 before, one catalog read. **Only if G-EW is open.** Closed: write Erda's figures as the before (10 of 13 tables, 3 sessions, limit -1, measured 2026-11-15) and "re-read at Step 268".
  ```bash
  sudo docker exec -i unheaded-postgres psql -U unheaded -X -At -d unheaded_app -c "SELECT 'tables', count(*) FILTER (WHERE c.relname LIKE 'zhen\_%'), count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = 'public' AND c.relkind IN ('r','p')" -c "SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = 'public' AND c.relkind IN ('r','p') AND c.relname LIKE 'zhen\_%' ORDER BY 1" -c "SELECT 'limit', rolconnlimit FROM pg_roles WHERE rolname = 'app_zhen'" -c "SELECT 'sessions', count(*) FROM pg_stat_activity WHERE usename = 'app_zhen'"
  ```
  Reference: `tables|10|13`, the table names, `limit|-1`, `sessions|3`. The migrations create 8 names (`zhen_memories`, `zhen_actions`, `zhen_action_snapshots`, `zhen_conversations`, `zhen_pending_confirmations`, `zhen_agent_runs`, `zhen_captures`, `zhen_capture_labels`): write the names of any others in `$PROG`; Phase 7 moves what is there.
- [ ] **Step 20** [W] ~5m: Fill the `before` column of Z1-Z15 in `$PROG` from Steps 10-19, each with its command's step number and the time read.
- [ ] **Step 21** [C][V] ~1m: **PHASE 0 EXIT GATE**: `$PROG` has Decisions (answered or "unanswered"), a `before` in every row except Z11 (a handoff), and G-EW's state.
  ```bash
  cd /home/govan/tmp/wt/S-ZHEN-SWITCH && git add docs/battle-plans/progress/S-ZHEN-SWITCH-progress.md && git commit -m "[PLAN S-ZHEN-SWITCH] Steps 1-21: decisions and the before readings"
  ```

---

## PHASE 1: THE SWITCH ON PAPER AND IN ONE PACKAGE (Steps 22-41)

**Goal**: The design is an ADR. The module has a manifest. Platform code has one way to ask "is a module on": `pkg/modules`. The four new accounts are registered once, so no later lane edits `pkg/uids`.
**Prerequisite**: Phase 0 gate. D1 answered (a), or coded on the recommendation.
**Time**: 2 h.
**Agent**: Coordinator.
**Model**: Opus (the ADR records design calls; the code steps are Sonnet-exact).

- [ ] **Step 22** [R] ~3m: `ls /home/govan/tmp/wt/S-ZHEN-SWITCH/docs/adr | sort -V | tail -4` and `tail -5 docs/adr/ADR-INDEX.md`. The next free number is `N` (148 on `1393e5ee`, unless a sibling plan took it since). Read `pkg/uids/uids.go:70-76,318-325`, `pkg/uids/uids_test.go:120-135`, `deploy/sysusers.d/unheaded.conf` (the `unheaded-ci-status` line): the four places an account is registered.
- [ ] **Step 23** [V] ~1m: `grep -c -E '= 1671[4-7]$' pkg/uids/uids.go` prints `0` (the four ids are free). Any taken: use the next free ids above 16713 and write the mapping in `$PROG`; every later step that names an id uses the mapping.
- [ ] **Step 24** [W] ~25m: `docs/adr/ADR-<N>-zhen-is-a-module-behind-one-switch.md` (SPDX header as ADR-147's). Status: Proposed; "implements ADR-147 decision 15". Sections, each from this plan and no longer than it needs: **Context** (rows G1-G13, two lines each); **Decision** 1 the switch is `unheaded-zhen.target` (D1), 2 a module's fragments live under `/run/unheaded/modules/<name>/` while its target is active and nowhere else, 3 the readers (Prometheus and VictoriaMetrics by glob and timed re-read, dashboard-backend every 30 s, the navigation bar and kanban per request, all through `pkg/modules`), 4 platform checks run on platform timers; a module may author or drive runbooks, it is never the clock, 5 a module's data is its own database, backup set and connection limit (D2, D6), 6 a module's listeners are loopback, a Unix socket or an allowlisted private address, never a wildcard or a globally routable address, held by a rule in the binaries, a static gate and a live guard (D5), 7 permitted-not-required lists (G15) stay, 8 plan 7: the target's units become an LXD instance; fragments, database and gates stay; **Alternatives rejected** (D1 b and c, D2 b and c, one line each); **Consequences** (flipping the switch needs root; a module's health never decides the platform's overall state). Then one row in `docs/adr/ADR-INDEX.md` and one in `wiki/ADR-Index.md`, in number order.
- [ ] **Step 25** [V] ~1m: `grep -c "ADR-<N>" docs/adr/ADR-INDEX.md wiki/ADR-Index.md` prints 1 for each file. Write `ADR-ZS = ADR-<N>` in `$PROG`.
- [ ] **Step 26** [W] ~5m: `modules/zhen/module.yaml`, exactly (replace `<N>`):
  ```yaml
  # SPDX-License-Identifier: GPL-3.0-or-later
  # Zhen (Zhenai): the optional AI module (ADR-<N>). The platform runs whole
  # without it. One switch: the systemd target named below. Limits are
  # restated from the unit files; tests/scripts/test_module_manifest.py
  # fails when the two differ.
  name: zhen
  switch: unheaded-zhen.target
  units:
    - {unit: zhen-agentd.service,            memory_max: 512M, tasks_max: 512}
    - {unit: unheaded-zhen-ui-front.service, memory_max: 128M}
    - {unit: unheaded-zhen-app.service,      memory_max: 3G,   tasks_max: 256}
    - {unit: unheaded-llama.service,         memory_max: 6G,   tasks_max: 256}
  ports: [8081, 20103, 20111, 20112]
  database: {name: unheaded_zhen, owner: well_zhen_owner, role: app_zhen, connection_limit: 12}
  fragments:
    - prometheus/rules/zhen.yml
    - prometheus/scrape/zhen.yml
    - victoria/scrape/zhen.yml
    - dashboard/services.txt
  ```
- [ ] **Step 27** [W] ~10m: Red. `tests/scripts/test_module_manifest.py` (SPDX header; `REPO = pathlib.Path(__file__).resolve().parents[2]`):
  - `test_units_exist`: every `unit` is a file under `deploy/systemd/`.
  - `test_limits_match_the_units`: for each unit, the file's `MemoryMax=` equals `memory_max`, and `TasksMax=` equals `tasks_max` where the manifest gives one.
  - `test_ports_are_registered`: every port appears as `= <port>` in `pkg/ports/ports.go`, except 8081 (llama-server's, named in `deploy/systemd/unheaded-llama.service`: assert `tcp:8081` is in that file).
  - `test_name_and_switch`: `name == "zhen"`, `switch == "unheaded-zhen.target"`, `database.connection_limit` is an int between 1 and 20.
- [ ] **Step 28** [B] ~1m: `cd /home/govan/tmp/wt/S-ZHEN-SWITCH && /var/tmp/unheaded-sprint/civenv/bin/pytest -q tests/scripts/test_module_manifest.py 2>&1 | tail -3`
- [ ] **Step 29** [V] ~1m: `4 passed` (the manifest of Step 26 is already true of the units). A failure names a limit that differs: correct the manifest to the unit file, never the unit file.
- [ ] **Step 30** [C] ~1m: **COMMIT** `docs/adr/ADR-<N>-*.md docs/adr/ADR-INDEX.md wiki/ADR-Index.md modules/zhen/module.yaml tests/scripts/test_module_manifest.py`.
- [ ] **Step 31** [W] ~10m: Red. `pkg/modules/modules_test.go` (package `modules`, SPDX header). Each test sets `t.Setenv("UNHEADED_MODULES_DIR", dir)` with `dir := t.TempDir()`:
  - `TestDir_Default`: with the variable empty, `Dir()` is `/run/unheaded/modules`.
  - `TestEnabled`: nothing in `dir` → `Enabled("zhen")` false; `os.Mkdir(dir+"/zhen")` → true; a regular file named `zhen` instead → false.
  - `TestEnabled_RefusesBadNames`: `"../x"`, `""`, `"Zhen"`, `"a/b"`, a 33-character name → false, and no file-system call escapes `dir` (create `dir/../x` first and assert false).
  - `TestFragments`: with `dir/zhen/dashboard/services.txt` and `dir/other/dashboard/services.txt` → `Fragments("dashboard/services.txt")` returns both paths, sorted; a symlink in place of one file is left out; `Fragments("../x")` returns nil.
  - `TestModuleOf`: `ModuleOf(dir+"/zhen/dashboard/services.txt")` is `"zhen"`; a path outside `dir` gives `""`.
- [ ] **Step 32** [B] ~1m: `cd /home/govan/tmp/wt/S-ZHEN-SWITCH && go test ./pkg/modules/ 2>&1 | tail -3`
- [ ] **Step 33** [V] ~1m: Red: `undefined: Dir` (or "no non-test Go files").
- [ ] **Step 34** [W] ~15m: Green. `pkg/modules/modules.go` (SPDX header; package comment: what a module is, ADR-<N>, that the directory exists only while a module's target is active):
  ```go
  const EnvDir = "UNHEADED_MODULES_DIR"
  const DefaultDir = "/run/unheaded/modules"
  func Dir() string                    // $UNHEADED_MODULES_DIR, else DefaultDir
  func Enabled(name string) bool       // name matches ^[a-z][a-z0-9-]{0,31}$ and Dir()/name is a directory (os.Lstat, not a symlink)
  func Fragments(rel string) []string  // sorted Dir()/<module>/rel that are regular files (Lstat); rel is cleaned and refused if it is absolute or starts with ".."
  func ModuleOf(path string) string    // the <module> element of a path Fragments returned, "" otherwise
  ```
  Standard library only. No logging: a missing directory is the normal "no module is on" state.
- [ ] **Step 35** [B] ~1m: `go test -race -count=1 ./pkg/modules/ 2>&1 | tail -2`
- [ ] **Step 36** [V] ~1m: `ok`. Fail → fix against the test, two tries, then `[STUCK]` and Phases 3 (Steps 86-103), 4 and 5 are `[BLOCKED]`.
- [ ] **Step 37** [W] ~10m: The four accounts, each in the four places `ci-status` has (Step 22): `pkg/uids/uids.go` constants with a two-line comment each and a `Registry` entry; `pkg/uids/uids_test.go`'s host-unit map; `deploy/sysusers.d/unheaded.conf`.

  | Constant | id | Registry key | Account | sysusers description |
  |---|---|---|---|---|
  | `HealthSweep` | 16714 | `health-sweep` | `unheaded-health-sweep` | `Unheaded health sweep (platform timer)` |
  | `IOCAudit` | 16715 | `ioc-audit` | `unheaded-ioc-audit` | `Unheaded IOC audit (platform timer)` |
  | `Modules` | 16716 | `modules` | `unheaded-modules` | `Unheaded module fragments (ADR-<N>)` |
  | `ModuleGuard` | 16717 | `module-guard` | `unheaded-module-guard` | `Unheaded module listener guard` |
- [ ] **Step 38** [W] ~2m: `deploy/tmpfiles.d/unheaded-modules.conf` (SPDX comment, then one line): `d     /run/unheaded/modules 0755 unheaded-modules unheaded-modules -`. A comment above it: the directory must exist before Docker mounts it into a container, or Docker makes it itself as root; it is owned by the account of the switch's oneshot (Step 142), which is the only writer; every reader needs only the `0755`.
- [ ] **Step 39** [B] ~3m: `go test -race -count=1 ./pkg/uids/ ./pkg/modules/ 2>&1 | tail -3`, then the Code gates block from the Legend.
- [ ] **Step 40** [V] ~1m: Both `ok`; gates clean. A `pkg/uids` test that names a file it expects for a new account (a unit that does not exist yet): the units arrive in Phases 2, 5 and 6; if the test cannot wait, move that account's four lines to the phase that writes its unit and note it in `$PROG`.
- [ ] **Step 41** [C][V] ~1m: **PHASE 1 EXIT GATE**: ADR indexed; manifest test green; `pkg/modules` green under `-race`; accounts registered. **COMMIT** `pkg/modules/modules.go pkg/modules/modules_test.go pkg/uids/uids.go pkg/uids/uids_test.go deploy/sysusers.d/unheaded.conf deploy/tmpfiles.d/unheaded-modules.conf docs/battle-plans/progress/S-ZHEN-SWITCH-progress.md`.

---

## PHASE 2: THE PLATFORM OWNS ITS SCHEDULES (Steps 42-77) [P]

**Goal**: The health sweep and the IOC audit run from systemd timers the platform owns, each leaving a last-success series. The sweep can pass. `IOCAuditNotSucceeding` reads the platform's series. Zhen's schedule file holds no platform check.
**Prerequisite**: Phase 1 gate. D3 answered (a) for Steps 49-57; with (b), skip them, delete the `unheaded-health-sweep` unit pair and the `HealthSweepNotSucceeding` rule from Steps 58-66, and write "retired, D3 (b)" in row Z13.
**Time**: 3.5 h.
**Agent**: Developer agent `[P]`, worktree `-p2`.
**Model**: Sonnet. No step in this phase runs as root or touches a host; the install is Phase 8.

### The wrapper

- [ ] **Step 42** [R] ~5m: `scripts/monitoring/ci-status-textfile.sh` (the textfile idiom: write a temp file in the target directory, `chmod 0644`, `mv -f`), `scripts/run-runbook.py:556-610` (arguments; a `draft` is refused without `--dry-run`), `tests/scripts/test_image_digests.py` (the test layout).
- [ ] **Step 43** [W] ~15m: Red. `tests/scripts/test_platform_schedule.py`. A fixture writes a stub runner (`#!/bin/sh`, `exit ${STUB_RC:-0}`) into `tmp_path` and runs `scripts/monitoring/platform-schedule.sh <name> <runbook> <dir>` with `PLATFORM_SCHEDULE_RUNNER=<stub>`; `<runbook>` is an empty file in `tmp_path`, `<dir>` a fresh directory. Cases:
  1. success: exit 0; `<dir>/schedule_ioc-audit.prom` holds exactly three sample lines, `unheaded_schedule_last_run_timestamp_seconds{schedule="ioc-audit"}`, `unheaded_schedule_last_exit_code{schedule="ioc-audit"} 0`, `unheaded_schedule_last_success_timestamp_seconds{schedule="ioc-audit"}`; the two timestamps are equal and within 5 s of now; file mode `0644`.
  2. a later failure (`STUB_RC=1`, the file from case 1 edited so its success value is `1000`): exit 1; `last_exit_code` is `1`; `last_success` is still `1000`; `last_run` is now.
  3. a first run that fails (no file yet): exit 1; `last_success` equals `last_run` (the three-day clock starts at install).
  4. name `a b`: exit 2, no file. Name of 65 characters: exit 2.
  5. a runbook path that does not exist: exit 2, no file.
  6. no file matching `.schedule_*` is left in `<dir>` after any case.
- [ ] **Step 44** [B] ~1m: `cd /home/govan/tmp/wt/S-ZHEN-SWITCH-p2 && /var/tmp/unheaded-sprint/civenv/bin/pytest -q tests/scripts/test_platform_schedule.py 2>&1 | tail -3`
- [ ] **Step 45** [V] ~1m: Red: 6 failed, "No such file" for `platform-schedule.sh`.
- [ ] **Step 46** [W] ~15m: Green. `scripts/monitoring/platform-schedule.sh` (mode 0755, SPDX header, `set -uo pipefail`). Header comment: what it is (the clock for platform checks, ADR-<N>), the usage line, the three series, that it runs as a nologin account and never as root. Body: check three arguments; name against `^[a-z0-9-]{1,64}$` (exit 2); runbook is a readable file (exit 2); directory is writable (exit 2); runner is `${PLATFORM_SCHEDULE_RUNNER:-/usr/local/lib/unheaded/schedules/run-runbook.py}`; run `python3 -B "$runner" "$runbook"` when the runner ends in `.py`, else run it directly (the stub); keep its exit code; read the old success value from the existing file with `awk` (default: now); on exit 0 the success value is now; write the three lines to `<dir>/.schedule_<name>.tmp`, `chmod 0644`, `mv -f` to `schedule_<name>.prom`; exit with the runner's code.
- [ ] **Step 47** [B] ~1m: Step 44 again; then `bash -n scripts/monitoring/platform-schedule.sh && (command -v shellcheck >/dev/null && shellcheck scripts/monitoring/platform-schedule.sh || echo "shellcheck absent")`.
- [ ] **Step 48** [C][V] ~1m: `6 passed`, no syntax error, no shellcheck finding. **COMMIT** `scripts/monitoring/platform-schedule.sh tests/scripts/test_platform_schedule.py`.

### The sweep can pass (D3 a)

- [ ] **Step 49** [R] ~3m: `runbooks/observe/service-health-sweep.yaml` (all), `tests/runbooks/test_ioc_audit.py:1-60` and its way of running one step's `command` with `subprocess` and an altered environment, `tests/runbooks/harness.py`.
- [ ] **Step 50** [W] ~20m: Red. `tests/runbooks/test_service_health_sweep.py`. A fixture puts a fake `curl` first on `PATH` (a shell script: prints `200` when its last argument starts with a URL listed in `$FAKE_OK`, else `400`) and runs the command of step `sweep-all-services` with bash. Cases:
  1. targets file `wotan 18000 https` and `trace-collector 16671 http`, `FAKE_OK="https://localhost:18000/health http://localhost:16671/health"`: exit 0; output has `[OK]   wotan` and `[OK]   trace-collector`; `RESULT: 2/2 healthy`.
  2. the same file with `FAKE_OK` holding only the first URL: exit 1; one `[FAIL]` line naming `trace-collector`.
  3. `SWEEP_TARGETS_FILE` naming a missing file: exit 1; output `no targets file`.
  4. a line `x 1 gopher`: exit 1; `[FAIL] x: scheme 'gopher' is not http or https`.
  5. comment and blank lines in the file are skipped (`TOTAL` counts only target lines).
  6. the runbook's text names no module: `"zhen" not in text.lower()` and `"inference" not in text`.
  7. `harness.dry_run(path)` lists both step names.
- [ ] **Step 51** [B] ~1m: `/var/tmp/unheaded-sprint/civenv/bin/pytest -q tests/runbooks/test_service_health_sweep.py 2>&1 | tail -3`
- [ ] **Step 52** [V] ~1m: Red on cases 1 to 6 (the runbook has a fixed list and plain http).
- [ ] **Step 53** [W] ~15m: Green. In `runbooks/observe/service-health-sweep.yaml`, step `sweep-all-services`: replace everything from the comment line `# HTTP /health ports from pkg/ports/ports.go.` through the `done` that closes the `for entry in $SERVICES` loop with:
  ```bash
        # What this host runs comes from a file, one "name port scheme" per
        # line (configs/health-sweep-<host>.targets, installed as
        # /etc/unheaded/health-sweep.targets). First-party listeners are TLS
        # under UNHEADED_TLS_ENABLED, so the scheme is per target and https
        # is verified against the deployment CA. No module is listed here:
        # a module's health is its own alert (ADR-<N>).
        TARGETS="${SWEEP_TARGETS_FILE:-/etc/unheaded/health-sweep.targets}"
        CA="${UNHEADED_TLS_CA_CERT:-/etc/unheaded/tls/ca.pem}"
        [ -r "$TARGETS" ] || { echo "no targets file: $TARGETS"; exit 1; }
        while read -r NAME PORT SCHEME _; do
          case "$NAME" in ''|'#'*) continue ;; esac
          TOTAL=$((TOTAL + 1))
          case "$SCHEME" in
            http|https) ;;
            *) echo "  [FAIL] $NAME: scheme '$SCHEME' is not http or https"; UNHEALTHY=$((UNHEALTHY + 1)); continue ;;
          esac
          STATUS=$(curl -s -o /dev/null -w "%{http_code}" --max-time 2 --cacert "$CA" "$SCHEME://localhost:$PORT/health" 2>/dev/null)
          if [ "$STATUS" = "200" ]; then
            echo "  [OK]   $NAME (:$PORT)"
            HEALTHY=$((HEALTHY + 1))
          else
            echo "  [FAIL] $NAME (:$PORT): HTTP $STATUS"
            UNHEALTHY=$((UNHEALTHY + 1))
          fi
        done < "$TARGETS"
  ```
  Keep the indentation of the block it replaces. After the last `echo "=============================================="` of that step add the line `[ "$UNHEALTHY" -eq 0 ]`. Leave step `check-system-resources` as it is. Update `metadata.description` to `Health check the HTTP services this host runs (targets file) and report status`.
- [ ] **Step 54** [B] ~3m: Build west's targets file from what answers now (one pass, two GETs a port).
  ```bash
  CA=/etc/unheaded/tls/ca.pem; test -r $CA || CA=/home/govan/tmp/unheaded/configs/certs/dev/ca.pem
  for p in $(ss -ltnH | awk '{print $4}' | grep -E '^127\.0\.0\.1:(1[6-9]|2[0-6])[0-9]{3}$' | cut -d: -f2 | sort -un); do printf '%s https=%s http=%s\n' $p "$(curl -s -o /dev/null -m2 -w '%{http_code}' --cacert $CA https://localhost:$p/health)" "$(curl -s -o /dev/null -m2 -w '%{http_code}' http://localhost:$p/health)"; done
  ```
  Reference 2026-11-15: 18000 and 20000 answer `https=200 http=400`.
- [ ] **Step 55** [W] ~5m: `configs/health-sweep-west.targets` (SPDX comment; a comment line giving the format and the date read): one line `name port scheme` for every port of Step 54 where exactly one scheme answered 200. The name is the port's service in `pkg/ports/ports.go` (`grep -n "= <port>" pkg/ports/ports.go`), in the spelling of the old list where it had one. **Leave out**, whatever they answered: 19001, 19002, 19003, 19004, 19005, 19006 (decision 7 removes or stops them: plan 2), and every port in `modules/zhen/module.yaml`. A port where neither or both schemes answered 200: leave it out and write the port and both codes under "For Stevie".
- [ ] **Step 56** [B] ~2m: Step 51 again; `scripts/runbook-manifest.sh --write && git diff --stat runbooks/MANIFEST.sha256`.
- [ ] **Step 57** [C][V] ~1m: `7 passed`; the manifest diff is one line (`observe/service-health-sweep.yaml`). **COMMIT** `runbooks/observe/service-health-sweep.yaml runbooks/MANIFEST.sha256 tests/runbooks/test_service_health_sweep.py configs/health-sweep-west.targets`.

### Units and the rule

- [ ] **Step 58** [W] ~20m: Four unit files. Each `.service` starts with an SPDX line and a comment block (what it runs, its account and id, the install lines of Steps 243-247, that it replaces a job in `zhen-agentd -scheduler`, ADR-<N>), then:
  `deploy/systemd/unheaded-health-sweep.service`:
  ```ini
  [Unit]
  Description=Unheaded health sweep of this host's services (platform timer)
  Documentation=https://github.com/unheaded/unheaded/blob/develop/runbooks/observe/service-health-sweep.yaml

  [Service]
  Type=oneshot
  ExecStart=/usr/local/lib/unheaded/monitoring/platform-schedule.sh service-health-sweep /usr/local/lib/unheaded/schedules/runbooks/observe/service-health-sweep.yaml /var/lib/unheaded/textfile
  Environment=SWEEP_TARGETS_FILE=/etc/unheaded/health-sweep.targets
  Environment=UNHEADED_TLS_CA_CERT=/etc/unheaded/tls/ca.pem
  TimeoutStartSec=5min
  User=unheaded-health-sweep
  Group=unheaded-health-sweep
  UMask=0022
  MemoryMax=128M
  TasksMax=64
  IPAddressDeny=any
  IPAddressAllow=localhost
  ReadWritePaths=/var/lib/unheaded/textfile
  ```
  followed by the hardening lines of `deploy/systemd/unheaded-ci-status.service` from `NoNewPrivileges=yes` through `SystemCallErrorNumber=EPERM`, copied verbatim except its `ReadWritePaths=` line (already above).
  `deploy/systemd/unheaded-health-sweep.timer`: `[Unit]` `Description=Unheaded health sweep every 6 hours`; `[Timer]` `OnCalendar=*-*-* 00/6:10:00`, `RandomizedDelaySec=5min`, `Persistent=true`; `[Install]` `WantedBy=timers.target`.
  `deploy/systemd/unheaded-ioc-audit.service`: the same shape with `Description=Unheaded daily indicator-of-compromise audit (platform timer)`, `Documentation=.../runbooks/security/ioc-audit.yaml`, `ExecStart=/usr/local/lib/unheaded/monitoring/platform-schedule.sh ioc-audit /usr/local/lib/unheaded/schedules/runbooks/security/ioc-audit.yaml /var/lib/unheaded/textfile`, no `Environment=` lines, `TimeoutStartSec=10min`, `User=`/`Group=unheaded-ioc-audit`.
  `deploy/systemd/unheaded-ioc-audit.timer`: `Description=Unheaded IOC audit, daily; a missed day runs at the next boot`; `OnCalendar=*-*-* 02:00:00`, `RandomizedDelaySec=10min`, `Persistent=true`; `WantedBy=timers.target`. A comment: installed disabled until the runbook is approved (`status: approved`); the runner refuses a draft.
- [ ] **Step 59** [B] ~2m: `cd /home/govan/tmp/wt/S-ZHEN-SWITCH-p2 && for s in scripts/check-unit-ceilings.sh scripts/check-unit-binaries.sh scripts/check-unit-flags.sh; do bash $s 2>&1 | tail -2; done; go test -count=1 ./pkg/uids/ 2>&1 | tail -1`
- [ ] **Step 60** [V] ~1m: Each prints its pass line; `ok`.
  - A gate names one of the new units → Step 61.
- [ ] **Step 61** [D] ~5m: Read the gate's message. It names a list or a pattern the unit must be in (a script that is not a Go binary, an account with no unit): add the unit where the message says, as `unheaded-ci-status.service` is listed there (`grep -n ci-status <the gate's file>`). Two tries, then `[STUCK]`.
- [ ] **Step 62** [W] ~15m: Red. `monitoring/prometheus/rules/tests/schedules_test.yml`: `rule_files: [../schedules.yml]`, `evaluation_interval: 1m`. Move into it the two test groups for `IOCAuditNotSucceeding` from `monitoring/prometheus/rules/tests/zhen_test.yml` (from the comment that introduces the first one to the end of that file) and delete them there. In the moved text replace every `zhen_scheduler_job_last_success_timestamp_seconds{job="zhen-agentd",instance="[fd00:dead:beef::1]:20111",schedule="X"}` with `unheaded_schedule_last_success_timestamp_seconds{job="huginn",host="west",schedule="X"}`, every `exp_labels` with `{severity: warning, urgency: none, host: west, schedule: ioc-audit}`, and in `exp_annotations.description` the words "Read the newest scheduled-run card in Review, or run the runbook by hand" with "Read `journalctl -u unheaded-ioc-audit`, or run the runbook by hand". Add one group for `HealthSweepNotSucceeding`: series `unheaded_schedule_last_success_timestamp_seconds{job="huginn",host="west",schedule="service-health-sweep"}` with values `0x1600`; quiet at `23h`, quiet at `24h20m`, firing at `25h` with labels `{severity: warning, urgency: none, host: west, schedule: service-health-sweep}` (hand-written rules carry `urgency: none`; the ticket comes from `promote:`, G21), summary `The health sweep has not succeeded for over a day`, description `Every run since then failed or none ran: a service on this host does not answer /health, or the timer stopped. Read journalctl -u unheaded-health-sweep`, runbook_url `runbooks/observe/service-health-sweep.yaml`.
- [ ] **Step 63** [B] ~2m: `scripts/check-prometheus-rules.sh 2>&1 | tail -4`
- [ ] **Step 64** [V] ~1m: Red: `schedules.yml` does not exist.
- [ ] **Step 65** [W] ~10m: Green. `monitoring/prometheus/rules/schedules.yml` (header comment in the house voice: platform checks run on systemd timers, each writes `unheaded_schedule_*` through `scripts/monitoring/platform-schedule.sh`, Huginn forwards the textfile; a host that was off catches up by `Persistent=`, and the 30m hold lets that run land). Group `schedules`, `interval: 1m`, two rules: `IOCAuditNotSucceeding` with `expr: time() - max by (host, schedule) (unheaded_schedule_last_success_timestamp_seconds{schedule="ioc-audit"}) > 3 * 86400`, `for: 30m`, labels and annotations as the moved tests expect; `HealthSweepNotSucceeding` with the same shape, `schedule="service-health-sweep"`, `> 86400`, `for: 30m`. In `monitoring/prometheus/rules/zhen.yml` delete the `IOCAuditNotSucceeding` rule and the header paragraph that begins `# One job has an alarm of its own.` In `observability/alerting/policy.yaml` add, under `promote:` after the `IOCAuditNotSucceeding` entry, an entry of the same shape: `alert: HealthSweepNotSucceeding`, `urgency: ticket`, `reason:` "The platform's own sweep of this host's services, every 6 hours. A day with no success means a service is down and stayed down, or the timer stopped: with restart `no` (ADR-147 decision 1) nothing brings either back."
- [ ] **Step 66** [B] ~2m: Step 63 again; `grep -rn 'IOCAuditNotSucceeding\|HealthSweepNotSucceeding' --include='*.go' --include='*.py' --include='*.sh' . | grep -v '^./docs/' | cut -c1-140`.
- [ ] **Step 67** [V] ~2m: `[PASS]`. Every file the `grep` lists that pins the rule's file or labels is updated in Steps 69-70; a file not named there (a Go or shell test of the alert policy): give `HealthSweepNotSucceeding` the same treatment `IOCAuditNotSucceeding` has in it.
- [ ] **Step 68** [C] ~1m: **COMMIT** the four units, `monitoring/prometheus/rules/schedules.yml monitoring/prometheus/rules/zhen.yml monitoring/prometheus/rules/tests/schedules_test.yml monitoring/prometheus/rules/tests/zhen_test.yml observability/alerting/policy.yaml`.

### Zhen is not the clock

- [ ] **Step 69** [W] ~10m: `configs/zhen-schedules.yaml`: delete every line from `  # The Python scheduler's one enabled default (every 30m there). Six hours` through `  #   catch_up: true` (lines 27-46 on `1393e5ee`: the `service-health-sweep` job with its three comment lines, the blank line, the nine comment lines of the `ioc-audit` block and its four commented job lines), and the blank line after them. In their place, three comment lines: platform checks are not Zhen's; the health sweep and the IOC audit run on the platform's timers (`deploy/systemd/unheaded-health-sweep.timer`, `unheaded-ioc-audit.timer`, ADR-<N>); Zhen may run a runbook when asked, it is not their clock. The one job left is `conversation-memory-maintenance` (`enabled: false`).
- [ ] **Step 70** [W] ~15m: `tests/runbooks/test_ioc_audit.py`: `RULES = "monitoring/prometheus/rules/schedules.yml"`; delete the constant `SCHEDULES` and the test `test_the_shipped_schedule_names_the_job_for_after_approval`; add:
  ```python
  def test_the_platform_owns_the_schedule():
      jobs = yaml.safe_load((harness.REPO / "configs/zhen-schedules.yaml").read_text())["jobs"]
      assert not [j for j in jobs if j["name"] in ("ioc-audit", "service-health-sweep")]
      timer = (harness.REPO / "deploy/systemd/unheaded-ioc-audit.timer").read_text()
      assert "OnCalendar=*-*-* 02:00:00" in timer and "Persistent=true" in timer
      unit = (harness.REPO / "deploy/systemd/unheaded-ioc-audit.service").read_text()
      assert "platform-schedule.sh ioc-audit " in unit and "User=unheaded-ioc-audit" in unit
  ```
  In `runbooks/security/ioc-audit.yaml` replace `monitoring/prometheus/rules/zhen.yml` with `monitoring/prometheus/rules/schedules.yml`, and any sentence in its `description` or notes that says Zhen or `zhen-agentd` schedules it with "runs daily from `unheaded-ioc-audit.timer`". `status` stays `draft`. Then `scripts/runbook-manifest.sh --write`.
- [ ] **Step 71** [B] ~1m: `grep -rln 'zhen-schedules.yaml' --include='*_test.go' --include='*.py' . | grep -v '^./docs/'`
- [ ] **Step 72** [W] ~10m: For each file Step 71 lists other than `tests/runbooks/test_ioc_audit.py`: if a test loads the shipped `configs/zhen-schedules.yaml` and expects the job `service-health-sweep` in it, change the expectation to the one job that is left; a test that uses the name only inside its own fixture text stays as it is.
- [ ] **Step 73** [B] ~4m: `/var/tmp/unheaded-sprint/civenv/bin/pytest -q tests/runbooks tests/scripts/test_platform_schedule.py 2>&1 | tail -3; GOMAXPROCS=4 go test -race -count=1 ./cmd/zhen-agentd/... ./cmd/kanban-app/... 2>&1 | tail -3; scripts/check-runbooks.sh 2>&1 | tail -2`
- [ ] **Step 74** [V] ~1m: All pass. The manifest diff since Step 57 is one line (`security/ioc-audit.yaml`).
- [ ] **Step 75** [B] ~5m: The Code gates block from the Legend.
- [ ] **Step 76** [W] ~2m: Log in `$PROG`: test counts, the targets file's lines, any port left out.
- [ ] **Step 77** [C][V] ~1m: **PHASE 2 EXIT GATE**: wrapper, sweep and rule tests green; unit gates green; `configs/zhen-schedules.yaml` names no platform check. **COMMIT** `configs/zhen-schedules.yaml tests/runbooks/test_ioc_audit.py runbooks/security/ioc-audit.yaml runbooks/MANIFEST.sha256`, any file of Step 72, `$PROG`. Merge the lane into `sprint/s-zhen-switch`.

---

## PHASE 3: ZHEN OUT OF THE PLATFORM'S DEFAULTS (Steps 78-109) [P]

**Goal**: With no module on, Akira, the dashboard, the navigation bar, kanban and the build name nothing of Zhen's and log nothing about it. With the module on, the dashboard lists Zhen's targets within 30 s and they never decide the platform's overall state.
**Prerequisite**: Phase 1 gate. D4 for Step 104.
**Time**: 3.5 h.
**Agent**: Developer agent `[P]`, worktree `-p3`.
**Model**: Sonnet.

### Akira

- [ ] **Step 78** [W] ~5m: Red. In `cmd/akira/main_test.go`, after the last `defaultTargets` test:
  ```go
  // A module or a service no deployment runs must not be a built-in target:
  // a host without it would report a failure every sweep (ADR-147, A7).
  func TestDefaultTargets_NameNoModuleOrAbsentService(t *testing.T) {
  	for _, tgt := range defaultTargets {
  		switch tgt.Name {
  		case "zhenai", "inference", "gateway":
  			t.Errorf("default target %q: not part of every host", tgt.Name)
  		}
  		switch tgt.Port {
  		case 20100, 20103, 21000:
  			t.Errorf("default target %q on port %d: not part of every host", tgt.Name, tgt.Port)
  		}
  	}
  }
  ```
- [ ] **Step 79** [B] ~1m: `cd /home/govan/tmp/wt/S-ZHEN-SWITCH-p3 && go test ./cmd/akira/ -run TestDefaultTargets -count=1 2>&1 | tail -8`
- [ ] **Step 80** [V] ~1m: Red: six errors (three names, three ports).
- [ ] **Step 81** [W] ~5m: Green. In `cmd/akira/main.go` `defaultTargets`: delete the five comment lines that begin `// The Zhen web UI is https whatever` and the `zhenai` entry; delete the two comment lines that begin `// Not first-party Go listeners` and the `inference` entry; delete the `gateway` entry. `daemon` stays. If plan 2 has already removed other entries, leave its result as it is. In `configs/akira-west.yaml` delete the two lines `  - name: inference` and `    port: 20100`.
- [ ] **Step 82** [B] ~2m: `go test -race -count=1 ./cmd/akira/ 2>&1 | tail -3`
- [ ] **Step 83** [V] ~1m: `ok`. Another test that counted the defaults fails → change its number to the new count, nothing else.
- [ ] **Step 84** [C] ~1m: **COMMIT** `cmd/akira/main.go cmd/akira/main_test.go configs/akira-west.yaml`.

### The dashboard's health list

- [ ] **Step 85** [R] ~8m: `cmd/dashboard-backend/internal/health/health.go:105-160, 320-395, 742-796, 880-945`, its `health_test.go` (how a test sets a target's status), `cmd/dashboard-backend/main.go:185-200, 400-440`.
- [ ] **Step 86** [W] ~20m: Red. In `cmd/dashboard-backend/internal/health/health_test.go`:
  - `TestSyncModuleServices_AddsAndRemoves`: a monitor with one kingdom target `wotan`; `SyncModuleServices(map[string]map[string]string{"zhen": {"zhenai": "127.0.0.1:1"}})` returns `(1, 0)`; `GetTargets()` holds `zhenai` with `Tags == []string{"module:zhen"}`; a second identical call returns `(0, 0)`; `SyncModuleServices(nil)` returns `(0, 1)`; `zhenai` is gone, `wotan` is still there.
  - `TestSyncModuleServices_NeverTakesAKingdomName`: syncing `{"zhen": {"wotan": "127.0.0.1:1"}}` returns `(0, 0)` and `wotan` keeps the tag `kingdom`.
  - `TestSystemHealth_ModuleTargetDoesNotDecideOverall`: `wotan` healthy, module target `zhenai` unhealthy → `GetSystemHealth()` has `Status == StatusHealthy`, `TotalServices == 1`, `UnhealthyCount == 0`, and `Services` holds both names.
  - `TestParseServiceEndpoints`: the `name=host:port` parser accepts the two-line module file and refuses a line with no `=`.
- [ ] **Step 87** [B] ~1m: `go test ./cmd/dashboard-backend/internal/health/ -run 'SyncModule|ModuleTarget|ParseServiceEndpoints' -count=1 2>&1 | tail -5`
- [ ] **Step 88** [V] ~1m: Red: `undefined: SyncModuleServices` (or `m.SyncModuleServices undefined`).
- [ ] **Step 89** [W] ~30m: Green.
  - `health.go`: `const ModuleTagPrefix = "module:"`; `func ParseServiceEndpoints(r io.Reader) (map[string]string, error)` (the loop now inside `main.go`'s `loadServiceEndpoints`, moved here; `loadServiceEndpoints` opens the file and calls it); `func (m *Monitor) SyncModuleServices(byModule map[string]map[string]string) (added, removed int)`: for each module and endpoint not registered, `RegisterTarget` with a `ServiceTarget` built exactly as `RegisterKingdomServices` builds one (`base := utls.EndpointURL(hostPort, FirstParty[name])`, `HealthURL` and `ReadyURL` = `base` + the path from `serviceHealthPaths`, default `/health`) and `Tags: []string{ModuleTagPrefix + module}`; a name a non-module target already holds is skipped; a registered target whose tag starts with `ModuleTagPrefix` and is no longer wanted is removed with `UnregisterTarget`. In `GetSystemHealth`, a service whose `Tags` hold a `module:` tag is copied into `Services` and left out of `TotalServices` and the four counters.
  - A gauge registered beside the monitor's other metrics: `unheaded_dashboard_health_targets{module}`: the number of registered targets, `module=""` for the platform's, set at the end of every sync (the switch test reads it from `/metrics`).
  - `main.go`: after `RegisterKingdomServices`, a goroutine that, at start and every 30 s until the context ends, builds the map from `modules.Fragments("dashboard/services.txt")` (module name from `modules.ModuleOf`, endpoints from `ParseServiceEndpoints`; a file that does not parse is skipped with one `log.Warn` per path and content hash) and calls `SyncModuleServices`. Log at Info only when `added+removed > 0`.
- [ ] **Step 90** [B] ~2m: `go test -race -count=1 ./cmd/dashboard-backend/... 2>&1 | tail -4`
- [ ] **Step 91** [V] ~1m: All `ok`.
- [ ] **Step 92** [W] ~5m: `config/dashboard/services.txt`: delete the four lines from `# The Zhen web UI is https with hybrid key exchange only (ADR-138), so this is` through `zhenai=172.28.1.1:20112`, and the line `llama-server=172.28.1.1:8081`. In the comment above `wiki=`, change `so vor + llama-server (bound to 127.0.0.1` to `so vor (bound to 127.0.0.1` and `are fronted by a socat proxy` to `is fronted by a socat proxy`. Create `modules/zhen/dashboard/services.txt`: an SPDX comment; a comment "Read by dashboard-backend while the Zhen module is on (ADR-<N>). Same format as config/dashboard/services.txt."; the four deleted `zhenai` lines; `llama-server=172.28.1.1:8081`.
- [ ] **Step 93** [V] ~1m: `grep -c -i -E 'zhen|llama' config/dashboard/services.txt` prints `0`; `grep -c -E '^(zhenai|llama-server)=' modules/zhen/dashboard/services.txt` prints `2`.
- [ ] **Step 94** [C] ~1m: **COMMIT** `cmd/dashboard-backend/internal/health/health.go cmd/dashboard-backend/internal/health/health_test.go cmd/dashboard-backend/main.go config/dashboard/services.txt modules/zhen/dashboard/services.txt`.

### The navigation bar

- [ ] **Step 95** [R] ~3m: `pkg/bff/nav.go` (all), `pkg/bff/nav_test.go`.
- [ ] **Step 96** [W] ~10m: Red. In `pkg/bff/nav_test.go`: `TestKingdomNav_ZhenOnlyWhileItsModuleIsOn`: with `t.Setenv("UNHEADED_MODULES_DIR", t.TempDir())`, no entry of `KingdomNav()` has `ID == "zhenai"` and the body `ServeNavJS` writes does not contain `zhenai`; after `os.Mkdir(dir+"/zhen", 0o755)` both do, in the same process, and after `os.Remove` neither does again. Existing tests that expect the Zhenai entry set the variable to a directory holding `zhen/`.
- [ ] **Step 97** [B][V] ~1m: `go test ./pkg/bff/ -run TestKingdomNav -count=1 2>&1 | tail -4`. Red: `zhenai` is present with no module on.
- [ ] **Step 98** [W] ~10m: Green. `KingdomNav()` appends the `zhenai` entry only when `modules.Enabled("zhen")`. Replace `navOnce`/`navBody` with a body cached per state: a mutex, the last `modules.Enabled("zhen")` value and the body rendered for it; `ServeNavJS` renders again when the value changed. Change the comment `// KingdomNav is the menu, in the order the pages show it.` to add: "The Zhenai entry is there only while the Zhen module is on (ADR-<N>)."
- [ ] **Step 99** [B][V] ~2m: `go test -race -count=1 ./pkg/bff/ 2>&1 | tail -2` prints `ok`.

### Kanban

- [ ] **Step 100** [R] ~5m: `cmd/kanban-app/zhenoptin.go` (all), `cmd/kanban-app/runbooks.go:405-492, 630-650`, `cmd/kanban-app/static/js/runbooks.js:145-230` (both sections stay hidden when the field is absent: confirm `renderAgentRun(null)` and `renderZhenOptIn` without the field hide their section; if either does not, add the two-line guard and say so in `$PROG`).
- [ ] **Step 101** [W] ~15m: Red. `cmd/kanban-app/zhenmodule_test.go`, each test with `UNHEADED_MODULES_DIR` set to an empty temp directory:
  - `TestTaskDetail_NoZhenFieldsWhileTheModuleIsOff`: the JSON of `GET /api/v1/tasks/{id}` has no key `zhen_opt_in` and no key `agent_run` (use the store the existing detail tests use).
  - `TestZhenOptIn_RefusedWhileTheModuleIsOff`: `POST /api/v1/tasks/{id}/zhen-opt-in` with an operator's login answers 404 with body `{"error":"the zhen module is off"}`; the store is not written.
  - `TestAgentRun_NoQueryNoWarningWhileTheModuleIsOff`: with a store whose `LatestAgentRun` fails the test if called, `agentRunFor` returns nil and `agentRunWarned` stays false.
  - `TestZhenFieldsReturnWithTheModule`: after creating `zhen/` in the directory, the detail carries `zhen_opt_in` as before (the existing opt-in tests, which now set the variable to such a directory, cover the rest).
- [ ] **Step 102** [B][V] ~1m: `go test ./cmd/kanban-app/ -run 'ZhenModule|WhileTheModuleIsOff|ReturnWithTheModule' -count=1 2>&1 | tail -6`. Red on the first three.
- [ ] **Step 103** [W] ~15m: Green. `zhenOptInFor` and `agentRunFor` return nil first thing when `!modules.Enabled("zhen")`; `handleZhenOptIn` answers the 404 above before reading the body. Existing tests of the opt-in and the agent run get `t.Setenv("UNHEADED_MODULES_DIR", dirWithZhen(t))` through one helper `dirWithZhen` in the new test file. `go test -race -count=1 ./cmd/kanban-app/ 2>&1 | tail -2` prints `ok`.

### vor, the build, the gates

- [ ] **Step 104** [W] ~5m: (D4 a) `deploy/systemd/unheaded-vor.service`: line 2 `# vor: the Zhen retrieval corpus server (DP-712). vor lives in its own` becomes `# vor: the sheet and corpus server (DP-712). The wiki's search, the MCP`, and line 3 `# repository; this unit runs the installed binary.` becomes `# server and Zhen are its clients. vor lives in its own repository; this`, then a new line 4 `# unit runs the installed binary.`; `Description=vor retrieval corpus server for Zhen` becomes `Description=vor sheet and corpus server`. With D4 (b): leave the unit, add `unheaded-vor.service` to `modules/zhen/module.yaml` `units` and 20110 to `ports`, and write "D4 (b)" in row Z7.
- [ ] **Step 105** [W] ~5m: `Makefile`: on the `build-services:` line delete ` build-zhen-rag`. After the `build-zhen-codeindex` recipe add:
  ```make
  build-zhen: build-zhen-rag build-zhen-agent build-zhen-agentd build-zhen-mcp build-zhen-ui-front build-zhen-codeindex ## Build the optional Zhen module's binaries (ADR-<N>)
  ```
  and add `build-zhen` to the `.PHONY` list where the other `build-zhen-*` names are (if they are not listed there, leave `.PHONY` alone).
- [ ] **Step 106** [B] ~3m: `make -n build-services 2>&1 | grep -c zhen; make -n build-zhen 2>&1 | grep -c 'go build'; grep -rn 'build-zhen-rag' scripts .github Makefile | grep -v '^Makefile' | cut -c1-120`
- [ ] **Step 107** [V] ~1m: `0`; `6`; a script or workflow that reached `zhen-rag` only through `build-services` (it calls `make build-services` or `make build` and then runs `bin/zhen-rag`): add `make build-zhen-rag` before that use. None is expected: `coding-gate-ci` depends on `build-zhen-rag` by name.
- [ ] **Step 108** [B] ~5m: The Code gates block; `/var/tmp/unheaded-sprint/civenv/bin/pytest -q tests/scripts/test_module_manifest.py 2>&1 | tail -1`.
- [ ] **Step 109** [C][V] ~1m: **PHASE 3 EXIT GATE**: Akira, dashboard, nav and kanban tests green under `-race`; `services.txt`, Akira's defaults and `build-services` name nothing of Zhen's. **COMMIT** `pkg/bff/nav.go pkg/bff/nav_test.go cmd/kanban-app/zhenoptin.go cmd/kanban-app/runbooks.go cmd/kanban-app/zhenmodule_test.go deploy/systemd/unheaded-vor.service Makefile`, any file of Steps 100 and 107, `$PROG`. Merge the lane into `sprint/s-zhen-switch`.

---

## PHASE 4: ZHEN'S MONITORING BECOMES THE MODULE'S (Steps 110-139) [P]

**Goal**: Zhen's rules and scrape jobs live in `modules/zhen/`, load only while `/run/unheaded/modules/zhen/` exists, and are checked by the same gates as the platform's. The platform's files name nothing of Zhen's.
**Prerequisite**: Phases 2 and 3 merged into `sprint/s-zhen-switch`; the lane starts from it.
**Time**: 3 h.
**Agent**: Developer agent `[P]`, worktree `-p4`.
**Model**: Sonnet. Nothing here runs as root or touches a host; Step 111 runs the pinned images with `--network none` to read their flags.

- [ ] **Step 110** [R] ~10m: `monitoring/prometheus/prometheus.yml` (the `zhen-agentd` job, lines 110-119, and `rule_files`, line 146), `config/victoria/scrape.yml:245-262`, `monitoring/docker-compose.yml` (the `prometheus` service), `docker-compose.yml:310-350` (`victoria`), `:1285-1330` (`dashboard-backend`), `:1396-1460` (`kanban-app`), `scripts/check-prometheus-rules.sh` (all), `cmd/alert-policy/main.go:375-415` and the function behind `-runbooks`.
- [ ] **Step 111** [B] ~3m: The reload flags exist in the pinned images (the digests are the ones in the two compose files).
  ```bash
  cd /home/govan/tmp/wt/S-ZHEN-SWITCH-p4 && P=$(grep -o 'prom/prometheus:[^ ]*' monitoring/docker-compose.yml | head -1); V=$(grep -o 'victoriametrics/victoria-metrics:[^ ]*' docker-compose.yml | head -1); docker run --rm --network none --entrypoint /bin/prometheus "$P" --help 2>&1 | grep -E 'auto-reload-interval|enable-feature' | head -3; docker run --rm --network none "$V" --help 2>&1 | grep -E 'promscrape.configCheckInterval|promscrape.config.dryRun' | head -3
  ```
- [ ] **Step 112** [V] ~1m: Prometheus prints `--config.auto-reload-interval` and an `--enable-feature` line naming `auto-reload-config`; VictoriaMetrics prints both flags. Write "reload: auto" in `$PROG`.
  - Prometheus lacks the flag → Step 113. VictoriaMetrics lacks either → `[STUCK]` (the module's scrape job cannot follow the switch) and Phases 5, 8, 9 are `[BLOCKED]`.
- [ ] **Step 113** [D] ~3m: Write "reload: HUP" in `$PROG`. Step 126 then adds no Prometheus flags, and Step 142 adds the two `docker kill -s HUP` lines. Go on.
- [ ] **Step 114** [B] ~1m: Move the rule file and its test into the module.
  ```bash
  cd /home/govan/tmp/wt/S-ZHEN-SWITCH-p4 && mkdir -p modules/zhen/prometheus/rules/tests && git mv monitoring/prometheus/rules/zhen.yml modules/zhen/prometheus/rules/zhen.yml && git mv monitoring/prometheus/rules/tests/zhen_test.yml modules/zhen/prometheus/rules/tests/zhen_test.yml && grep -n 'rule_files' -A2 modules/zhen/prometheus/rules/tests/zhen_test.yml
  ```
  The test keeps `rule_files: [../zhen.yml]` (the relative layout is the same).
- [ ] **Step 115** [B][V] ~2m: Red: `scripts/check-prometheus-rules.sh 2>&1 | tail -2` passes with one rule file fewer: nothing checks the module's rules now. Log the count.
- [ ] **Step 116** [W] ~20m: `scripts/check-prometheus-rules.sh`: after the platform's pass, for each directory matching `modules/*/prometheus/rules` (sorted): the same coverage check (every `*.yml` there is named by a `tests/*_test.yml` of that directory as `  - ../<file>`), then `promtool check rules` and `promtool test rules` with that directory mounted at `/rules` exactly as the platform's run does. A module rules directory with no `tests/` is `[FAIL]`. Its own line: `[PASS] module <name>: <n> rule files parse, <m> test files pass`. Header comment: modules' rules are checked here and loaded by Prometheus only while the module is on (ADR-<N>).
- [ ] **Step 117** [B][V] ~3m: `scripts/check-prometheus-rules.sh 2>&1 | tail -3` → the platform's `[PASS]` and `[PASS] module zhen: 1 rule files parse, 1 test files pass`.
- [ ] **Step 118** [W] ~8m: `scripts/check-gates-can-fail.sh`: a function `provoke_module_rules`, modelled on `provoke_unit_ceilings`, that backs up `modules/zhen/prometheus/rules/tests/zhen_test.yml` and changes its first `reason: absent` to `reason: absentX`; a registry row in the format of the `check-prometheus-rules|provoke_prometheus_rules|...` row (`grep -n '^check-prometheus-rules|' scripts/check-gates-can-fail.sh`): `check-prometheus-rules|provoke_module_rules|slow|a module rule test expects the wrong label`. If the registry refuses a second row for one gate name, `[STUCK]` this step only.
- [ ] **Step 119** [W] ~12m: Red. In `cmd/alert-policy/main_test.go` (`t.TempDir()` as the root, files written by the test):
  - `TestCheckHandRules_ReadsModuleRules`: `monitoring/prometheus/rules/a.yml` with alert `A` (`urgency: none`) and `modules/zhen/prometheus/rules/z.yml` with alert `Z` (`urgency: ticket`) → one error naming `modules/zhen/prometheus/rules/z.yml`.
  - `TestCheckHandRules_PromotesAModuleAlert`: `Z` with `urgency: none` and a policy that promotes `Z` → no error; a policy promoting `Q`, in neither directory → one error.
  - The `-runbooks` check, given `Z` with `runbook_url: runbooks/none.yaml`, reports it.
- [ ] **Step 120** [B][V] ~1m: `go test ./cmd/alert-policy/ -run 'ModuleRules|PromotesAModule' -count=1 2>&1 | tail -6`. Red: no error for `z.yml`.
- [ ] **Step 121** [W] ~10m: Green. `cmd/alert-policy/main.go`: `const moduleRulesGlob = "modules/*/prometheus/rules/*.yml"`; `checkHandRules` and the `-runbooks` check read the files of both globs (sorted, platform first). The error for `promote:` says "is not an alert in monitoring/prometheus/rules or modules/*/prometheus/rules".
- [ ] **Step 122** [B][V] ~2m: `go test -race -count=1 ./cmd/alert-policy/ 2>&1 | tail -1; go run ./cmd/alert-policy 2>&1 | tail -1; go run ./cmd/alert-policy -runbooks 2>&1 | tail -1` → `ok`, `[PASS]`, `[PASS]`.
- [ ] **Step 123** [C] ~1m: **COMMIT** `modules/zhen/prometheus/rules/zhen.yml modules/zhen/prometheus/rules/tests/zhen_test.yml scripts/check-prometheus-rules.sh scripts/check-gates-can-fail.sh cmd/alert-policy/main.go cmd/alert-policy/main_test.go` and the two removals.
- [ ] **Step 124** [W] ~10m: Scrape fragments. `modules/zhen/prometheus/scrape/zhen.yml`: an SPDX comment, a comment "Loaded by Prometheus through scrape_config_files while the Zhen module is on (ADR-<N>).", `scrape_configs:`, then the `zhen-agentd` job moved from `monitoring/prometheus/prometheus.yml` with its five comment lines (from `  # zhen-agentd on west: its API stays on 127.0.0.1:20111;` through the `labels: {host: west}` line), indented as a list item of `scrape_configs`. `modules/zhen/victoria/scrape/zhen.yml`: the same shape with the `zhen-agentd` job moved from `config/victoria/scrape.yml` (from `  - job_name: "zhen-agentd"` through its `labels:` line). Delete both jobs and the blank line after each from the platform files.
- [ ] **Step 125** [W] ~5m: `monitoring/prometheus/prometheus.yml`: under `rule_files:` add `  - '/etc/unheaded-modules/*/prometheus/rules/*.yml'` with the comment `# A module's rules, present only while its switch is on (/run/unheaded/modules, ADR-<N>).`; add at top level, after `scrape_configs`' last job, `scrape_config_files:` with `  - '/etc/unheaded-modules/*/prometheus/scrape/*.yml'` and the same comment. `config/victoria/scrape.yml`: top level `scrape_config_files:` with `  - /etc/unheaded-modules/*/victoria/scrape/*.yml` and the comment.
- [ ] **Step 126** [W] ~10m: Compose. `monitoring/docker-compose.yml`, service `prometheus`: volume `      - /run/unheaded/modules:/etc/unheaded-modules:ro   # modules' rules and scrape jobs while their switch is on (ADR-<N>)`; under "reload: auto" also the commands `      - '--enable-feature=auto-reload-config'` and `      - '--config.auto-reload-interval=30s'`, and change the comment "Reload rules with `docker kill -s HUP unheaded-prometheus`" to add "; module files are picked up within 30 s on their own". `docker-compose.yml`: `victoria` gets the same volume line and the command `      - "-promscrape.configCheckInterval=30s"`; `dashboard-backend` and `kanban-app` get `      - /run/unheaded/modules:/run/unheaded/modules:ro   # pkg/modules (ADR-<N>)`.
- [ ] **Step 127** [B] ~3m: Prometheus accepts the config with the module present and with no module.
  ```bash
  cd /home/govan/tmp/wt/S-ZHEN-SWITCH-p4 && P=$(grep -o 'prom/prometheus:[^ ]*' monitoring/docker-compose.yml | head -1); E=$(mktemp -d); for M in "$PWD/modules" "$E"; do docker run --rm --network none -v "$PWD/monitoring/prometheus:/etc/prometheus:ro" -v "$M:/etc/unheaded-modules:ro" -v /home/govan/tmp/unheaded/configs/certs/dev/ca.pem:/etc/prometheus-tls/ca.pem:ro --entrypoint promtool "$P" check config /etc/prometheus/prometheus.yml 2>&1 | grep -E 'SUCCESS|FAILED|zhen' | head -4; echo "-- $M"; done; rmdir "$E"
  ```
- [ ] **Step 128** [V] ~1m: Both runs `SUCCESS`; the first also names `zhen.yml`. A `FAILED` on the CA path only: drop that `-v` and run again (promtool does not open it); any other `FAILED`: fix Step 125, two tries, then `[STUCK]`.
- [ ] **Step 129** [B] ~2m: VictoriaMetrics accepts its config both ways.
  ```bash
  cd /home/govan/tmp/wt/S-ZHEN-SWITCH-p4 && V=$(grep -o 'victoriametrics/victoria-metrics:[^ ]*' docker-compose.yml | head -1); E=$(mktemp -d); for M in "$PWD/modules" "$E"; do docker run --rm --network none -v "$PWD/config/victoria/scrape.yml:/etc/victoria/scrape.yml:ro" -v "$M:/etc/unheaded-modules:ro" -v /home/govan/tmp/unheaded/configs/certs/dev/ca.pem:/etc/victoria/tls/ca.pem:ro "$V" -promscrape.config=/etc/victoria/scrape.yml -promscrape.config.dryRun; echo "exit=$? -- $M"; done; rmdir "$E"
  ```
- [ ] **Step 130** [V] ~1m: `exit=0` twice, no line with `error` or `cannot`. Else fix Step 125, two tries, then `[STUCK]`.
- [ ] **Step 131** [B] ~3m: `for g in scripts/check-compose-hardening.sh scripts/check-compose-bind-nesting.sh scripts/check-compose-log-caps.sh; do bash $g 2>&1 | tail -1; done`
- [ ] **Step 132** [V] ~1m: Three pass lines. A bind-nesting failure names the new mount: read the gate's allowlist comment and add `/run/unheaded/modules` the way the other host-path mounts are listed; two tries.
- [ ] **Step 133** [W] ~15m: `tests/scripts/test_module_fragments.py` (SPDX header; `REPO` as in Step 27):
  - `test_platform_files_name_no_zhen`: in `monitoring/prometheus/prometheus.yml`, every `monitoring/prometheus/rules/*.yml`, `config/victoria/scrape.yml`, `config/dashboard/services.txt`, `config/akira/akira.yaml`, `configs/akira-west.yaml`, no line outside a `#` comment contains `zhen` or `llama` (case-insensitive).
  - `test_every_fragment_exists`: each `fragments` entry of `modules/zhen/module.yaml` is a file under `modules/zhen/`.
  - `test_scrape_fragments_shape`: each `*/scrape/*.yml` under `modules/zhen/` loads as YAML with only the key `scrape_configs`, a non-empty list whose `job_name`s all start with `zhen`.
  - `test_platform_reads_module_globs`: `prometheus.yml`'s `rule_files` holds `/etc/unheaded-modules/*/prometheus/rules/*.yml`, its `scrape_config_files` holds `/etc/unheaded-modules/*/prometheus/scrape/*.yml`; `scrape.yml`'s `scrape_config_files` holds `/etc/unheaded-modules/*/victoria/scrape/*.yml`.
- [ ] **Step 134** [B][V] ~1m: `/var/tmp/unheaded-sprint/civenv/bin/pytest -q tests/scripts/test_module_fragments.py tests/scripts/test_module_manifest.py 2>&1 | tail -1` → all passed, none failed (4 in the new file). A failure in the first test names a file and a line: it is a Zhen assumption this plan missed; move it into the module or delete it, and add a row to `$PROG` "Findings" (id Z16 and on, seat "own sweep").
- [ ] **Step 135** [W] ~10m: Every other reference to the old paths: `git grep -n -E 'rules/zhen\.yml|rules/tests/zhen_test\.yml' -- . ':!docs/battle-plans' ':!docs/adr' ':!docs/sweeps' ':!docs/history' ':!CHANGELOG.md'`. In each file listed (runbooks, wiki pages, comments), write the module path `modules/zhen/prometheus/rules/zhen.yml`. Then `scripts/runbook-manifest.sh --write`.
- [ ] **Step 136** [B][V] ~1m: Step 135's `git grep` again prints nothing.
- [ ] **Step 137** [B] ~6m: `scripts/check-prometheus-rules.sh 2>&1 | tail -2; go run ./cmd/alert-policy 2>&1 | tail -1; go run ./cmd/alert-policy -runbooks 2>&1 | tail -1; scripts/check-runbooks.sh 2>&1 | tail -1`, then the Code gates block.
- [ ] **Step 138** [W] ~3m: `$PROG`: Z4's "fix" column (rules, scrape jobs, mounts, gates), the reload mode, the files of Step 135.
- [ ] **Step 139** [C][V] ~1m: **PHASE 4 EXIT GATE**: Steps 117, 122, 128, 130, 134, 137 green. **COMMIT** `modules/zhen/prometheus/scrape/zhen.yml modules/zhen/victoria/scrape/zhen.yml monitoring/prometheus/prometheus.yml config/victoria/scrape.yml monitoring/docker-compose.yml docker-compose.yml tests/scripts/test_module_fragments.py runbooks/MANIFEST.sha256`, the files of Step 135, `$PROG`. Merge the lane into `sprint/s-zhen-switch`.

---

## PHASE 5: THE SWITCH (Steps 140-161)

**Goal**: One systemd target is the switch. Starting it starts Zhen's units and publishes the module's fragments; stopping it stops both. A checker reads both states from the outside.
**Prerequisite**: Phases 1 and 4 merged into `sprint/s-zhen-switch`; D1 (a).
**Time**: 3 h.
**Agent**: Coordinator, main sprint worktree.
**Model**: Opus (units that root installs; a checker that runs as root).

- [ ] **Step 140** [R] ~8m: The `[Unit]` and `[Install]` sections of `deploy/systemd/{zhen-agentd,unheaded-zhen-ui-front,unheaded-zhen-app,unheaded-llama}.service`, their install comment blocks, `deploy/tmpfiles.d/unheaded-zhen-ui.conf`, S-ZHEN-FRONT Phases 10-12 (`docs/battle-plans/S-ZHEN-FRONT-battle-plan.md` Steps 143-178: what it installs and enables).
- [ ] **Step 141** [W] ~5m: `deploy/systemd/unheaded-zhen.target`:
  ```ini
  # SPDX-License-Identifier: GPL-3.0-or-later
  # The Zhen module's one switch (ADR-<N>, ADR-147 decision 15). The platform
  # runs whole while it is off. Every Zhen unit is WantedBy= and PartOf= this
  # target, so the target's state is the module's state.
  #   on:  sudo systemctl enable --now unheaded-zhen.target
  #   off: sudo systemctl disable --now unheaded-zhen.target
  # Check either state: sudo scripts/test-zhen-switch.sh --expect on|off
  [Unit]
  Description=Unheaded Zhen module (optional AI, ADR-<N>)
  Documentation=https://github.com/unheaded/unheaded/blob/develop/wiki/Zhen-AI.md
  Wants=unheaded-zhen-module.service
  After=network-online.target docker.service

  [Install]
  WantedBy=multi-user.target
  ```
- [ ] **Step 142** [W] ~10m: `deploy/systemd/unheaded-zhen-module.service`: SPDX line; comment (publishes `modules/zhen` fragments under `/run/unheaded/modules/zhen` while the switch is on and withdraws them when it goes off; account `unheaded-modules`, uid 16716; ADR-<N>); then
  ```ini
  [Unit]
  Description=Unheaded Zhen module fragments (rules, scrape jobs, health targets)
  PartOf=unheaded-zhen.target
  After=systemd-tmpfiles-setup.service

  [Service]
  Type=oneshot
  RemainAfterExit=yes
  ExecStart=/usr/local/lib/unheaded/modules/module-fragments.sh publish zhen
  ExecStop=/usr/local/lib/unheaded/modules/module-fragments.sh withdraw zhen
  User=unheaded-modules
  Group=unheaded-modules
  UMask=0022
  MemoryMax=32M
  TasksMax=16
  IPAddressDeny=any
  ReadWritePaths=/run/unheaded/modules /var/lib/unheaded/textfile

  [Install]
  WantedBy=unheaded-zhen.target
  ```
  with the hardening lines of `deploy/systemd/unheaded-ci-status.service` from `NoNewPrivileges=yes` through `SystemCallErrorNumber=EPERM`, verbatim except its `ReadWritePaths=` line, inserted before `[Install]`. Under "reload: HUP" (Step 113) also `ExecStartPost=+/usr/bin/docker kill -s HUP unheaded-prometheus` and `ExecStopPost=+/usr/bin/docker kill -s HUP unheaded-prometheus` (`+` runs that one line as root).
- [ ] **Step 143** [W] ~10m: Each of the four Zhen units: add `PartOf=unheaded-zhen.target` as the last line of `[Unit]`; in `[Install]` replace `WantedBy=multi-user.target` with `WantedBy=unheaded-zhen.target`; in its install comment block replace the `systemctl enable --now <unit>` line with `systemctl enable <unit> (the switch starts it: unheaded-zhen.target, ADR-<N>)`. In `tests/scripts/test_module_manifest.py` add `test_units_belong_to_the_switch`: every unit of the manifest, and `unheaded-zhen-module.service`, has `PartOf=unheaded-zhen.target` and `WantedBy=unheaded-zhen.target`, and none has `WantedBy=multi-user.target`.
- [ ] **Step 144** [W] ~15m: Red. `tests/scripts/test_module_fragments_script.py`, running `scripts/modules/module-fragments.sh` with `MODULES_SRC`, `MODULES_RUN` and `MODULES_TEXTFILE` set to directories in `tmp_path` (the source holds `zhen/module.yaml`, `zhen/prometheus/rules/zhen.yml`, `zhen/victoria/scrape/zhen.yml`, `zhen/dashboard/services.txt`, `zhen/hosts/west/x.conf`):
  1. `publish zhen`: exit 0; `RUN/zhen` holds exactly the `prometheus`, `victoria` and `dashboard` trees (not `module.yaml`, not `hosts`); files `0644`, directories `0755`; `TEXTFILE/module_zhen.prom` holds `unheaded_module_enabled{module="zhen"} 1` and `unheaded_module_changed_timestamp_seconds{module="zhen"}` within 5 s of now.
  2. `publish zhen` again after editing the source rule file: `RUN/zhen` holds the new text.
  3. `withdraw zhen`: exit 0; `RUN/zhen` is gone; the textfile says `0`. A second `withdraw`: exit 0, still `0`.
  4. A symlink in the source tree is not copied.
  5. `publish ../x`, `publish Zhen`, `publish ""`: exit 2, nothing under `RUN` or `TEXTFILE` changed. `publish nosuch`: exit 1, nothing changed. One argument: exit 2.
  6. No name starting with `.zhen` and no `.module_` temp file is left in `RUN` or `TEXTFILE` after any case.
- [ ] **Step 145** [B][V] ~1m: `cd /home/govan/tmp/wt/S-ZHEN-SWITCH && /var/tmp/unheaded-sprint/civenv/bin/pytest -q tests/scripts/test_module_fragments_script.py 2>&1 | tail -2`. Red: "No such file".
- [ ] **Step 146** [W] ~20m: Green. `scripts/modules/module-fragments.sh` (0755, SPDX, `set -euo pipefail`, header comment: what it does, its two verbs, the three variables and their defaults `/usr/local/lib/unheaded/modules`, `/run/unheaded/modules`, `/var/lib/unheaded/textfile`, ADR-<N>). Name against `^[a-z][a-z0-9-]{0,31}$`. `publish`: copy only regular files of the three trees (`find -type f`, `install -D -m 0644`) into `RUN/.<name>.new`, then `rm -rf RUN/.<name>.old; [ -d RUN/<name> ] && mv RUN/<name> RUN/.<name>.old; mv RUN/.<name>.new RUN/<name>; rm -rf RUN/.<name>.old`. `withdraw`: `mv RUN/<name> RUN/.<name>.old` if present, then remove it. Both end by writing the textfile through `TEXTFILE/.module_<name>.prom.tmp`, `chmod 0644`, `mv -f` (the idiom of Step 42).
- [ ] **Step 147** [B][V] ~2m: Step 145 → `6 passed`; `bash -n scripts/modules/module-fragments.sh`; shellcheck clean where present.
- [ ] **Step 148** [C] ~1m: **COMMIT** `deploy/systemd/unheaded-zhen.target deploy/systemd/unheaded-zhen-module.service deploy/systemd/zhen-agentd.service deploy/systemd/unheaded-zhen-ui-front.service deploy/systemd/unheaded-zhen-app.service deploy/systemd/unheaded-llama.service scripts/modules/module-fragments.sh tests/scripts/test_module_fragments_script.py tests/scripts/test_module_manifest.py`.
- [ ] **Step 149** [W] ~25m: Red. `tests/scripts/test_zhen_switch.py` for `scripts/test-zhen-switch.sh --expect on|off [--wait N] [--no-well]`. Fakes first on `PATH` in `tmp_path`: `systemctl` (answers `is-active`/`is-enabled` from files `STATE/<unit>`), `curl` (prints the file `URLS/<sha1 of the URL>`), `ss` (prints `STATE/ss`), `docker` (prints `STATE/sessions` for the `pg_stat_activity` query); `MODULES_RUN` points at a temp directory. The checks, each printing one `PASS`, `FAIL` or `SKIP` line with its id:
  | id | on | off | source |
  |---|---|---|---|
  | C1 | `unheaded-zhen.target` active | inactive | `systemctl is-active` |
  | C2 | `zhen-agentd`, `unheaded-zhen-ui-front` active; `unheaded-zhen-app`, `unheaded-llama` active if `is-enabled` says enabled, else SKIP | all four not active | `systemctl` |
  | C3 | `$MODULES_RUN/zhen` is a directory | absent | file system |
  | C4 | rule group `zhen` loaded | absent | `http://127.0.0.1:9090/api/v1/rules` |
  | C5 | an active target with `job="zhen-agentd"` | none | `http://127.0.0.1:9090/api/v1/targets` |
  | C6 | the same in VictoriaMetrics | none | `http://127.0.0.1:8428/api/v1/targets` |
  | C7 | `unheaded_dashboard_health_targets{module="zhen"}` is 2 | 0 or absent | `https://127.0.0.1:20000/metrics`, `--cacert ${UNHEADED_TLS_CA_CERT:-/etc/unheaded/tls/ca.pem}` |
  | C8 | `"zhenai"` in the menu | not | `https://127.0.0.1:20000/auth/nav.js` |
  | C9 | 20103 and 20111 listen | none of 8081, 20103, 20111, 20112 | `ss -ltnH` |
  | C10 | not checked (idle pools) | 0 sessions of `app_zhen` | `docker exec -i unheaded-postgres psql -X -At -U ${WELL_SUPERUSER:-unheaded} -d postgres -c "SELECT count(*) FROM pg_stat_activity WHERE usename = 'app_zhen'"`; SKIP with `--no-well` |
  | C11 | `unheaded_module_enabled{module="zhen"}` is 1 | 0 | VictoriaMetrics query |
  Cases: every fixture "on" with `--expect on` → exit 0, 10 `PASS` and 1 `SKIP`-or-`PASS` line, a last line `RESULT on: pass, ELAPSED=<s>`; the same with the rule group missing → exit 1 and the `FAIL` line names C4; every fixture "off" with `--expect off` → exit 0; `--expect off --wait 2` with C3 still present → exit 1 after 2 to 4 s; `--expect maybe` → exit 2; `--no-well` → C10 `SKIP`.
- [ ] **Step 150** [B][V] ~1m: `/var/tmp/unheaded-sprint/civenv/bin/pytest -q tests/scripts/test_zhen_switch.py 2>&1 | tail -2`. Red: "No such file".
- [ ] **Step 151** [W] ~30m: Green. `scripts/test-zhen-switch.sh` (0755, SPDX, `set -uo pipefail`): header comment (what it proves, that it reads and never changes anything, that it runs with `sudo` for C10 and the CA, the checks table in short); argument parsing; one function per check; JSON read with `python3 -c`; `--wait N` repeats the whole set every 5 s until all pass or N seconds; exit 0, 1 or 2.
- [ ] **Step 152** [B][V] ~2m: Step 150 → all pass; `bash -n`; shellcheck clean where present.
- [ ] **Step 153** [C] ~1m: **COMMIT** `scripts/test-zhen-switch.sh tests/scripts/test_zhen_switch.py`.
- [ ] **Step 154** [B] ~3m: `cd /home/govan/tmp/wt/S-ZHEN-SWITCH && for s in scripts/check-unit-ceilings.sh scripts/check-unit-binaries.sh scripts/check-unit-flags.sh; do bash $s 2>&1 | tail -1; done; systemd-analyze verify deploy/systemd/unheaded-zhen.target deploy/systemd/unheaded-zhen-module.service 2>&1 | grep -v -E 'is not executable|No such file|not found' | head`
- [ ] **Step 155** [V] ~1m: Three pass lines; `systemd-analyze` prints nothing beyond missing installed paths (filtered).
  - A unit gate names the oneshot (a script, not a Go binary) → Step 156.
- [ ] **Step 156** [D] ~5m: As Step 61: add the unit where the gate's message says, as `unheaded-ci-status.service` is listed (`grep -n ci-status <the gate>`). Two tries, then `[STUCK]`.
- [ ] **Step 157** [B] ~2m: `go test -count=1 ./pkg/uids/ 2>&1 | tail -1` (the `modules` account now has its unit).
- [ ] **Step 158** [W] ~5m: `$PROG` "Handoffs": "S-ZHEN-FRONT Phases 10-12: the units they install take this plan's unit files (WantedBy and PartOf the target). Their Step 'comes back after a boot' now means: `unheaded-zhen.target` is enabled. Opening 20103 on the house LAN needs the LAN address on west's `bind_allow` (Phase 6, D5)."
- [ ] **Step 159** [B] ~5m: The Code gates block; `/var/tmp/unheaded-sprint/civenv/bin/pytest -q tests/scripts 2>&1 | tail -1`.
- [ ] **Step 160** [V] ~1m: Green.
- [ ] **Step 161** [C][V] ~1m: **PHASE 5 EXIT GATE**: the target, the oneshot and the checker exist and are tested; every Zhen unit belongs to the target. **COMMIT** `$PROG` and any file of Step 156.

---

## PHASE 6: ZHEN NEVER LISTENS ON A PUBLIC SIDE (Steps 162-189) [P]

**Goal**: A module listener is loopback, a Unix socket, or a private address on the host's allowlist. The binaries refuse anything else at start, a static gate refuses it in the repository, and a live guard raises a page if it happens anyway.
**Prerequisite**: Phase 1 gate; D5 (a).
**Time**: 4 h.
**Agent**: Developer agent `[P]`, worktree `-p6`.
**Model**: Opus (security logic; the rule decides what is public).

- [ ] **Step 162** [R] ~8m: `cmd/zhen-agentd/main.go:70-130, 320-370`; `cmd/zhen-ui-front/main.go:150-200` and `parseBindAddr`; `deploy/systemd/zhen-agentd.service:65-100`; `deploy/systemd/zhen-agentd.env.example:30-45`; `modules/zhen/module.yaml`.
- [ ] **Step 163** [W] ~20m: Red. `pkg/modules/bind_test.go`, table `TestCheckBind` (`allow` parsed by `ParseBindAllow`):
  | address | allow | result |
  |---|---|---|
  | `127.0.0.1:20111`, `[::1]:1` | none | ok |
  | `0.0.0.0:20111`, `[::]:1`, `:20111` | none | error containing `wildcard` |
  | `8.8.8.8:1` | none | error containing `globally routable` |
  | `[2606:4700::1]:1` | none | error containing `globally routable` |
  | `192.168.69.130:20103` | none | error containing `not on this host's allowlist` |
  | `192.168.69.130:20103` | `192.168.69.128/25` | ok |
  | `[fd00:dead:beef::1]:20111` | none / `fd00:dead:beef::1/128` | error / ok |
  | `169.254.1.1:1`, `[fe80::1%eth0]:1` | `169.254.0.0/16` refused by the parser | error containing `link-local` |
  | `localhost:1` | none | error containing `IP literal` |
  `TestParseBindAllow`: `"fd00:dead:beef::1/128, 192.168.69.128/25"` → two prefixes; `""` → none, no error; `"10.0.0.0/8,garbage"` → error; `"0.0.0.0/0"`, `"8.8.8.0/24"`, `"169.254.0.0/16"` → error containing `not private`.
  Signatures:
  ```go
  const EnvBindAllow = "UNHEADED_MODULE_BIND_ALLOW"
  func ParseBindAllow(s string) ([]netip.Prefix, error) // comma list; each prefix's network is private (netip.Addr.IsPrivate) and not link-local
  func CheckBind(hostport string, allow []netip.Prefix) error
  ```
- [ ] **Step 164** [B][V] ~1m: `cd /home/govan/tmp/wt/S-ZHEN-SWITCH-p6 && go test ./pkg/modules/ -run 'CheckBind|ParseBindAllow' -count=1 2>&1 | tail -3`. Red: `undefined: CheckBind`.
- [ ] **Step 165** [W] ~20m: Green. `pkg/modules/bind.go` (SPDX; comment: the rule of ADR-<N> decision 6, D5 (a), why a globally routable address is refused even when listed: the module never answers the public side, ADR-147 decision 15 (c)). Order of checks: split host and port (`net.SplitHostPort`); empty host or unspecified → wildcard; `netip.ParseAddr` fails → IP literal; loopback → ok; link-local unicast → error; `IsPrivate` and inside an allowed prefix → ok; `IsPrivate` → not on allowlist; else → globally routable. Standard library only.
- [ ] **Step 166** [B][V] ~1m: `go test -race -count=1 ./pkg/modules/ 2>&1 | tail -1` → `ok`.
- [ ] **Step 167** [W] ~15m: Red. `cmd/zhen-agentd/bind_test.go`: `TestCheckListenAddrs` on a new `func checkListenAddrs(host string, port int, metricsAddr, allowEnv string) error`: host `0.0.0.0` → error; host `127.0.0.1` with metrics `[fd00:dead:beef::1]:20111` and `allowEnv` `""` → error; the same with `allowEnv` `fd00:dead:beef::1/128` → nil; `allowEnv` `8.8.8.0/24` → error. `cmd/zhen-ui-front/main_test.go`: `--listen 192.168.69.130:20103` with `UNHEADED_MODULE_BIND_ALLOW` unset → `parseConfig` error; set to `192.168.69.128/25` → ok; `--listen 8.8.8.8:20103` with `8.8.8.0/24` set → error.
- [ ] **Step 168** [B][V] ~1m: `go test ./cmd/zhen-agentd/ ./cmd/zhen-ui-front/ -run 'CheckListenAddrs|Listen' -count=1 2>&1 | tail -5`. Red.
- [ ] **Step 169** [W] ~15m: Green. `zhen-agentd`: `checkListenAddrs` reads the allowlist with `ParseBindAllow` and checks `host:port` and, when set, `-metrics-addr`, with `modules.CheckBind`; `main` calls it before opening any listener and exits 2 with `zhen-agentd: <addr>: <reason> (ADR-<N>: a module listens on loopback or an allowlisted private address)`. The `-host` help drops "use 0.0.0.0 for non-loopback". `zhen-ui-front`: `parseConfig` calls `modules.CheckBind` for every `--listen` with the allowlist from `os.Getenv(modules.EnvBindAllow)`; `--metrics` keeps its own loopback rule.
- [ ] **Step 170** [B][V] ~2m: `go test -race -count=1 ./cmd/zhen-agentd/ ./cmd/zhen-ui-front/ ./pkg/modules/ 2>&1 | tail -3` → three `ok`.
- [ ] **Step 171** [C] ~1m: **COMMIT** `pkg/modules/bind.go pkg/modules/bind_test.go cmd/zhen-agentd/main.go cmd/zhen-agentd/bind_test.go cmd/zhen-ui-front/main.go cmd/zhen-ui-front/main_test.go`.
- [ ] **Step 172** [W] ~8m: `modules/zhen/module.yaml`: add `bind_allow: {west: ["fd00:dead:beef::1/128"]}` with the comment "per host; private prefixes only; the house LAN address is added when S-ZHEN-FRONT opens 20103 (A-16)". `modules/zhen/hosts/west/zhen-agentd.service.d/bind-allow.conf` and `modules/zhen/hosts/west/unheaded-zhen-ui-front.service.d/bind-allow.conf`, each: an SPDX comment, a comment naming `module.yaml` as the source, `[Service]`, `Environment=UNHEADED_MODULE_BIND_ALLOW=fd00:dead:beef::1/128`. In `tests/scripts/test_module_manifest.py` add `test_bind_allow_is_private_and_matches_the_dropins`: every prefix parses with `ipaddress.ip_network(strict=False)`, `is_private` and not `is_link_local`, not `0.0.0.0/0`; for each host directory under `modules/zhen/hosts/`, each drop-in's value equals that host's list joined by `,`.
- [ ] **Step 173** [B][V] ~1m: `/var/tmp/unheaded-sprint/civenv/bin/pytest -q tests/scripts/test_module_manifest.py 2>&1 | tail -1` → passes.
- [ ] **Step 174** [W] ~20m: Red. `tests/scripts/test_check_module_binds.py`: builds a fake repository in `tmp_path` (`modules/x/module.yaml` naming unit `x.service` and `bind_allow: {west: ["192.168.1.0/24"]}`, `deploy/systemd/x.service`) and runs `scripts/check-module-binds.sh --root <tmp>`. Exit 1 for an `ExecStart` holding `--listen 0.0.0.0:1`, `-host 0.0.0.0`, `--host=::`, `--listen 8.8.8.8:1`, `--listen 10.9.9.9:1` (private, on no host's list), or an `Environment=X_ADDR=0.0.0.0:1`; exit 0 for `-host 127.0.0.1`, `--listen [::1]:1`, `--listen 192.168.1.5:1`, a unit with no address; a unit not named by any module is never read (put `-host 0.0.0.0` in `deploy/systemd/y.service`: exit 0); a continuation line (`\` at the end) is joined. The `[FAIL]` line names the unit, the flag and the address.
- [ ] **Step 175** [B][V] ~1m: Red: "No such file".
- [ ] **Step 176** [W] ~25m: Green. `scripts/check-module-binds.sh` (0755, SPDX, header comment: the rule, ADR-<N>, D5 (a), what it reads), bash that runs `python3 -B - "$ROOT"` with the check inline (standard library and PyYAML): for each `modules/*/module.yaml`, for each unit, join `ExecStart=` continuation lines, take the value after `-host`, `--host`, `-listen`, `--listen`, `-metrics-addr`, `--metrics-addr`, `--metrics` (space or `=`), and every `Environment=` value whose name ends in `_ADDR` or `_LISTEN`; classify each with `ipaddress` by the same order as Step 165 against the union of the module's `bind_allow` lists. Prints `[PASS] module binds: <u> units, <a> addresses, none public` or one `[FAIL]` per address; exit 0 or 1; 2 on usage.
- [ ] **Step 177** [B][V] ~2m: Step 175's pytest → all pass; `scripts/check-module-binds.sh` on the lane → `[PASS] module binds: 4 units, ...`.
- [ ] **Step 178** [W] ~10m: `scripts/check-gates-can-fail.sh`: `provoke_module_binds` (backs up `deploy/systemd/zhen-agentd.service`, changes `-host 127.0.0.1` to `-host 0.0.0.0`) and the row `check-module-binds|provoke_module_binds|fast|zhen-agentd bound to 0.0.0.0`. Add the gate where `check-unit-ceilings.sh` runs: `grep -n 'check-unit-ceilings' scripts/ci-local.sh .github/workflows/*.yml`, one line beside each, same shape.
- [ ] **Step 179** [C] ~1m: **COMMIT** `modules/zhen/module.yaml modules/zhen/hosts/west/zhen-agentd.service.d/bind-allow.conf modules/zhen/hosts/west/unheaded-zhen-ui-front.service.d/bind-allow.conf tests/scripts/test_module_manifest.py scripts/check-module-binds.sh tests/scripts/test_check_module_binds.py scripts/check-gates-can-fail.sh scripts/ci-local.sh` and the workflow files of Step 178.
- [ ] **Step 180** [W] ~20m: Red. `tests/scripts/test_module_listener_guard.py` for `scripts/monitoring/module-listener-guard.sh <modules-src> <textfile-dir>`, with a fake `ss` on `PATH` printing `STATE/ss` and `GUARD_HOST=west`; the source holds `zhen/module.yaml` (`ports: [8081, 20103, 20111, 20112]`, `bind_allow: {west: ["fd00:dead:beef::1/128"]}`). `module_listeners.prom` must hold `unheaded_module_listener_public_total{module="zhen"}`, one `unheaded_module_listener_public{module="zhen",port="<p>",addr="<a>"} 1` per offending listener, and `unheaded_module_listener_guard_last_run_timestamp_seconds` within 5 s of now. Cases: `127.0.0.1:20111` and `[fd00:dead:beef::1]:20111` → total 0, no per-listener line; `0.0.0.0:20103` → total 1 naming 20103 and `0.0.0.0`; `*:20111` → 1; `[::]:8081` → 1; `192.168.69.130:20103` → 1 (not on west's list); `0.0.0.0:22` → 0 (not a module port); exit 0 in every case (the guard reports, the alert acts); a missing argument → exit 2 and no file.
- [ ] **Step 181** [B][V] ~1m: Red.
- [ ] **Step 182** [W] ~20m: Green. `scripts/monitoring/module-listener-guard.sh` (0755, SPDX, header: what it watches, account `unheaded-module-guard` uid 16717, that it reads `ss` and changes nothing, ADR-<N>): the same classification as Step 176 (share it: put the Python in `scripts/lib/module_binds.py` and have both scripts run it, so one rule exists); host from `${GUARD_HOST:-$(hostname -s)}`; write through `.module_listeners.prom.tmp`, `chmod 0644`, `mv -f`.
- [ ] **Step 183** [B][V] ~2m: Steps 175 and 181 pytest files both pass.
- [ ] **Step 184** [W] ~15m: `deploy/systemd/unheaded-module-guard.service` (oneshot, `User=`/`Group=unheaded-module-guard`, `ExecStart=/usr/local/lib/unheaded/monitoring/module-listener-guard.sh /usr/local/lib/unheaded/modules /var/lib/unheaded/textfile`, `MemoryMax=32M`, `TasksMax=16`, `IPAddressDeny=any`, `ReadWritePaths=/var/lib/unheaded/textfile`, the ci-status hardening lines verbatim except `ReadWritePaths=` and with `RestrictAddressFamilies=AF_UNIX AF_NETLINK` in place of its own `RestrictAddressFamilies=` line: `ss` reads sockets over netlink) and `unheaded-module-guard.timer` (`OnBootSec=2min`, `OnUnitActiveSec=5min`, `WantedBy=timers.target`). The guard is the platform's, not the module's: it is not part of the switch.
- [ ] **Step 185** [W] ~20m: `monitoring/prometheus/rules/modules.yml` (header in the house voice) with group `modules`, `interval: 1m`: `ModuleListenerPublic`: `max by (host, module) (unheaded_module_listener_public_total) > 0`, `for: 1m`, labels `severity: critical`, `urgency: none`, annotations summary "A module listens where the public side can reach it", description "Read unheaded_module_listener_public for the port and address; stop the module (systemctl disable --now unheaded-zhen.target) before anything else", `runbook_url: runbooks/security/module-listener-public.yaml`; `ModuleListenerGuardNotRunning`: `time() - max by (host) (unheaded_module_listener_guard_last_run_timestamp_seconds) > 1800`, `for: 10m`, `severity: warning`, `urgency: none`, same runbook. `monitoring/prometheus/rules/tests/modules_test.yml` with `rule_files: [../modules.yml]`: total 0 → quiet; total 1 for 2 min → firing with labels `{severity: critical, urgency: none, host: west, module: zhen}`; guard series at 0 for 45 min → `ModuleListenerGuardNotRunning` firing; for 20 min → quiet. `runbooks/security/module-listener-public.yaml` (`metadata.status: draft`, two read-only steps: `ss -ltnp` filtered to the module's ports, `systemctl show -p ExecStart` of the module's units), in the shape of an existing draft runbook. `observability/alerting/policy.yaml` `promote:`: `ModuleListenerPublic` `urgency: page`, reason "ADR-147 decision 15 (c): a module is never reachable from the public side; the guard saw it happen"; `ModuleListenerGuardNotRunning` `urgency: ticket`, reason "the guard above is the only live check of that rule".
- [ ] **Step 186** [B] ~5m: `scripts/check-prometheus-rules.sh 2>&1 | tail -2; go run ./cmd/alert-policy 2>&1 | tail -1; go run ./cmd/alert-policy -runbooks 2>&1 | tail -1; scripts/runbook-manifest.sh --write && scripts/check-runbooks.sh 2>&1 | tail -1; for s in scripts/check-unit-ceilings.sh scripts/check-unit-binaries.sh scripts/check-unit-flags.sh; do bash $s 2>&1 | tail -1; done`, then the Code gates block.
- [ ] **Step 187** [V] ~1m: All pass. A unit gate naming the guard → Step 61's fix.
- [ ] **Step 188** [W] ~3m: `$PROG` row Z6: fix column (binaries, gate, guard, rule).
- [ ] **Step 189** [C][V] ~1m: **PHASE 6 EXIT GATE**: bind tests, static gate, guard tests, rule tests green; the provocation is registered. **COMMIT** `scripts/lib/module_binds.py scripts/monitoring/module-listener-guard.sh tests/scripts/test_module_listener_guard.py deploy/systemd/unheaded-module-guard.service deploy/systemd/unheaded-module-guard.timer monitoring/prometheus/rules/modules.yml monitoring/prometheus/rules/tests/modules_test.yml runbooks/security/module-listener-public.yaml runbooks/MANIFEST.sha256 observability/alerting/policy.yaml scripts/check-module-binds.sh`, `$PROG`. Merge the lane into `sprint/s-zhen-switch`.

---

## PHASE 7: ZHEN'S OWN DATABASE, IN CODE (Steps 190-237) [P]

**Goal**: A fresh Well holds Zhen's tables in `unheaded_zhen` and none in `unheaded_app`; Zhen's code uses two pools (its tables, the board); the claim is two steps; kanban, retention and backups know the split; Zhen has its own backup set and alert. Nothing live changes here.
**Prerequisite**: Phase 1 gate; D2 (a) and D6 for the merge (coding may run on the recommendation). Steps 193, 198-202, 224-226 and 232-234 start throwaway Postgres servers whose data sits on the spinning disk: they wait for G-EW (G24). Before it, write the code and the Go unit tests, mark those steps `[BLOCKED]` with "G-EW", and come back.
**Time**: 7 h.
**Agent**: Developer agent `[P]`, worktree `-p7`.
**Model**: Opus (data layout, a claim across two databases, backups).

### The database and its schema

- [ ] **Step 190** [R] ~25m: `db/migrations/002_create_databases.sql`, `025_well_owner_roles.sql`, `029_template1_no_public_connect.sql`, every file that creates or changes a `zhen_*` object (008, 009, 010, 017, 031, 032, 033, 037, 041, 042 lines 50-60, 045, 046, 047, 049), `db/migrations/MANIFEST`, `db/init.sh`, `pkg/database/migrate/manifest.go:30-100`, `scripts/well/test-zhen-agent-runs.sh` (all: the throwaway-server pattern), `scripts/well/test-migrate.sh:80-100`.
- [ ] **Step 191** [W] ~25m: Red. `scripts/well/zhen-schema-fingerprint.sql`: one query, sorted rows `kind|name|definition`, over objects in schema `public` of the current database whose table or function name starts with `zhen_`: columns (`table.column`, `format_type`, `attnotnull`, `pg_get_expr` of the default), constraints (`pg_get_constraintdef`), indexes (`pg_get_indexdef`), triggers (`pg_get_triggerdef`), functions (`pg_get_functiondef` of every function a trigger on such a table calls, and of every `zhen_%` function), table and column privileges of `app_zhen`, `app_kanban` and `retention_runner` (`information_schema.role_table_grants`, `column_privileges`). No owner names (they differ by database on purpose). Then `scripts/well/test-zhen-database.sh` (SPDX; header listing its checks, as `test-zhen-agent-runs.sh` does; same throwaway-server functions and image default):
  1. A server bootstrapped by `db/init.sh` from the repository: `unheaded_zhen` exists, owner `well_zhen_owner`; `PUBLIC` holds no CONNECT on it; `rolconnlimit` of `app_zhen` is 12; `unheaded_app` holds no table or function named `zhen_%`; `kanban_zhen_optin` is in `unheaded_app` and `app_zhen` may SELECT it and `kanban_tasks`.
  2. A second server bootstrapped from a copy of `db/migrations` whose MANIFEST stops at `049`: its fingerprint of `unheaded_app` and the first server's fingerprint of `unheaded_zhen` differ by exactly one line, the constraint `REFERENCES kanban_tasks(id)` of `zhen_agent_runs`.
  3. On the second server, apply 050 and 051 with `well-migrate`, insert one `zhen_memories` row in `unheaded_app`, apply 052: it fails with `copy the rows`; `ALTER DATABASE unheaded_app SET unheaded.zhen_rows_copied = '2026-01-01'` and apply 052 again: it passes, and the setting is gone afterwards.
  4. With `WELL_TEST_ZHEN_DSN`, `WELL_TEST_APP_DSN` and `WELL_TEST_KANBAN_DSN` pointing at the first server, `go test -tags wellint -count=1 -run Split ./pkg/zhenagent/` passes and does not skip.
- [ ] **Step 192** [B] ~1m: `cd /home/govan/tmp/wt/S-ZHEN-SWITCH-p7 && bash -n scripts/well/test-zhen-database.sh && echo syntax-ok`
- [ ] **Step 193** [B][V] ~4m: (G-EW) `scripts/well/test-zhen-database.sh 2>&1 | tail -5`. Red at check 1: `unheaded_zhen` does not exist.
- [ ] **Step 194** [W] ~15m: `db/migrations/050_zhen_database.sql`, run on `postgres` statement by statement (header: ADR-<N>, ADR-147 decision 15, D2, D6; why notx): `CREATE ROLE well_zhen_owner NOLOGIN;` `CREATE DATABASE unheaded_zhen OWNER well_zhen_owner;` then, for `unheaded_zhen`, every database-level statement 002, 025 and 029 make for `unheaded_app` (CONNECT revoked from PUBLIC, the same `ALTER DATABASE ... SET` lines), with CONNECT granted to `app_zhen`, `app_kanban`, `retention_runner`, `well_backup` and `huginn_reader`; `ALTER ROLE app_zhen CONNECTION LIMIT 12;` (D6). If an owner role needs the grants 025 gives `well_app_owner`, give them to `well_zhen_owner` the same way.
- [ ] **Step 195** [W] ~60m: `db/migrations/051_zhen_schema.sql`, on `unheaded_zhen` (header: what it holds; that it is the state of the `zhen_*` objects after 049, written from the files of Step 190 in their order; that `zhen_agent_runs.card_id` keeps its type and loses its foreign key because the board lives in `unheaded_app`, and the worker keeps the link (Step 207); that copying rows on a live Well is the attended Phase 8B, never a migration). Every table, column, default, check, index, function, trigger and comment of the eight tables, and the grants `app_zhen`, `app_kanban` (041's columns) and `retention_runner` (042's) hold on them after 049.
- [ ] **Step 196** [W] ~15m: `db/migrations/052_app_drop_zhen.sql`, on `unheaded_app`. First a `DO` block: if any `zhen_%` table in `public` holds a row and `current_setting('unheaded.zhen_rows_copied', true)` is null or empty, `RAISE EXCEPTION 'zhen tables still hold rows: copy the rows to unheaded_zhen first (S-ZHEN-SWITCH Phase 8B) and set unheaded.zhen_rows_copied'`. Then `DROP TABLE` the eight tables and `DROP FUNCTION` the Zhen functions in dependency order (no `CASCADE`: a dependency this plan did not see must fail the migration), then `ALTER DATABASE unheaded_app RESET unheaded.zhen_rows_copied`.
- [ ] **Step 197** [W] ~3m: `db/migrations/MANIFEST`, after the 049 line, in its column layout: `postgres         050_zhen_database.sql   notx`, `unheaded_zhen    051_zhen_schema.sql`, `unheaded_app     052_app_drop_zhen.sql`. Then `scripts/check-well-migrations.sh 2>&1 | tail -1` → pass. A refusal of the database name `unheaded_zhen` → the manifest parser has a fixed list: add it there with a test, as the other three are.
- [ ] **Step 198** [B][V] ~6m: (G-EW) `scripts/well/test-zhen-database.sh 2>&1 | tail -8`: checks 1, 2, 3 pass; check 4 fails (Step 205 writes it).
- [ ] **Step 199** [D] ~15m: Check 2 shows more than the foreign key → each extra line names the object: write it in 051 as the source migration has it. Two tries, then `[STUCK]`; Phases 7 and 8B `[BLOCKED]`.
- [ ] **Step 200** [B] ~4m: (G-EW) `scripts/well/test-migrate.sh 2>&1 | tail -3`. Red: its database list lacks `unheaded_zhen` (line 93).
- [ ] **Step 201** [W] ~5m: `scripts/well/test-migrate.sh`: the expected list `postgres unheaded_app unheaded_config unheaded_ops unheaded_zhen`; the per-database loop (line 87) gains `unheaded_zhen`.
- [ ] **Step 202** [B][V] ~4m: (G-EW) Step 200 again → passes.
- [ ] **Step 203** [C] ~1m: **COMMIT** `db/migrations/050_zhen_database.sql db/migrations/051_zhen_schema.sql db/migrations/052_app_drop_zhen.sql db/migrations/MANIFEST scripts/well/zhen-schema-fingerprint.sql scripts/well/test-zhen-database.sh scripts/well/test-migrate.sh`.

### Zhen's code on two pools

- [ ] **Step 204** [B] ~5m: The map of every query. `git grep -n -E 'zhen_(memories|actions|action_snapshots|conversations|pending_confirmations|agent_runs|captures|capture_labels)|kanban_(tasks|zhen_optin)|WELL_APP_ZHEN_DSN' -- 'cmd/zhen-*' 'pkg/zhenagent' 'pkg/champion' 'pkg/claudebridge' 'pkg/zhencapture' 'pkg/database/zhen.go' ':!*_test.go'`. Write the list in `$PROG` with the pool each line will use: a `zhen_*` table → the Zhen pool (`WELL_ZHEN_DSN`, database `unheaded_zhen`); `kanban_*` → the board pool (`WELL_APP_ZHEN_DSN`, `unheaded_app`, unchanged). A single statement naming both is a line to split.
- [ ] **Step 205** [W] ~25m: Red. `pkg/zhenagent/sqlstore_split_wellint_test.go` (build tag `wellint`; DSNs from `WELL_TEST_ZHEN_DSN`, `WELL_TEST_APP_DSN`, `WELL_TEST_KANBAN_DSN`; a missing one is `t.Fatal`, never a skip):
  - `TestSplit_ClaimRace`: an opted-in todo card (made as `app_kanban`, as the existing wellint tests do); eight goroutines `Claim` it at once → exactly one `nil`, seven `ErrClaimConflict`; in `unheaded_zhen` exactly one row for the card has status `claimed` and every other row for it is `cancelled`; the card is `in-progress`.
  - `TestSplit_NotOptedIn`: a card with no opt-in → `ErrClaimConflict`; one `cancelled` row; the card is still `todo`.
  - `TestSplit_Lifecycle`: claimed → running → succeeded moves the card as the single-database store did (the existing lifecycle test's assertions).
  - `TestSplit_BoardDown`: the board pool closed → `Claim` returns an error; the run row is `cancelled`.
  And a unit test `TestNewSQLStore_NeedsBothPools` (no tag): `NewSQLStore(nil, db)` and `NewSQLStore(db, nil)` return an error.
- [ ] **Step 206** [B][V] ~2m: `go vet -tags wellint ./pkg/zhenagent/ 2>&1 | tail -3`. Red: `NewSQLStore` undefined. (G-EW for running it: Step 232.)
- [ ] **Step 207** [W] ~40m: Green. `pkg/zhenagent/sqlstore.go`: `type SQLStore struct{ Board, Runs *sql.DB }` and `func NewSQLStore(board, runs *sql.DB) (SQLStore, error)`. `Claim`: (1) on `Runs`, `INSERT` the run (`23505` → `ErrClaimConflict`); (2) on `Board`, one transaction: the existing lock-and-check `SELECT ... JOIN kanban_zhen_optin ... FOR UPDATE OF t SKIP LOCKED`, then the `UPDATE kanban_tasks` to `in-progress`, commit; (3) if (2) found no row or failed, on `Runs` `UPDATE zhen_agent_runs SET status = 'cancelled' WHERE id = $1 AND status = 'claimed'` and return `ErrClaimConflict` (no row) or the error. Every other method sends `zhen_agent_runs` statements to `Runs` and `kanban_tasks` statements to `Board`; where one method changed both in one transaction, it writes the run first and the card second and returns an error naming both when the card write fails (the run is the record; the card is put right by `scripts/well-reconcile-kanban.sh`, named in the comment). A comment above `Claim` gives the reason for two steps (ADR-<N>, D2 (a)) and why `cancelled`: it is not counted by `Attempts`.
- [ ] **Step 208** [B][V] ~2m: `go test -race -count=1 ./pkg/zhenagent/ 2>&1 | tail -1; go vet -tags wellint ./pkg/zhenagent/` → `ok`, no vet output.
- [ ] **Step 209** [W] ~40m: `cmd/zhen-agentd`: Red first, `pools_test.go`: `TestOpenPools_NeedsZhenDSN` (with only `WELL_APP_ZHEN_DSN` set and `-memory-store pg`, start-up fails naming `WELL_ZHEN_DSN`) and `TestPoolCaps` (`zhenPoolMax == 6`, `boardPoolMax == 2`). Then green: `main` opens two `app_zhen` pools once, `zhenDB` from `WELL_ZHEN_DSN` (`SetMaxOpenConns(zhenPoolMax)`) and `boardDB` from `WELL_APP_ZHEN_DSN` (`boardPoolMax`), and hands them to the audit store, the confirm queue, the memory store, the scheduler store and the worker store (`zhenagent.NewSQLStore(boardDB, zhenDB)`); the five `sql.Open` of those stores go. The `app_kanban` pool (`main.go:500`) stays. Each flag that needs the Well and finds `WELL_ZHEN_DSN` unset fails with `-<flag> needs WELL_ZHEN_DSN (unheaded_zhen as app_zhen)`. `confirm_queue_test.go` and any test setting `WELL_APP_ZHEN_DSN` for a `zhen_*` table set `WELL_ZHEN_DSN`. `deploy/systemd/zhen-agentd.env.example`: a `WELL_ZHEN_DSN=` line under `WELL_APP_ZHEN_DSN=`, with the comment "Zhen's own tables (unheaded_zhen, ADR-<N>); WELL_APP_ZHEN_DSN is the board only". `deploy/systemd/user/zhen-agentd-overnight.service`: wherever it names `WELL_APP_ZHEN_DSN`, name `WELL_ZHEN_DSN` the same way.
- [ ] **Step 210** [B][V] ~3m: `go test -race -count=1 ./cmd/zhen-agentd/ 2>&1 | tail -1` → `ok`; `grep -c 'sql.Open("postgres"' $(ls cmd/zhen-agentd/*.go | grep -v _test)` sums to 3 (Zhen pool, board pool, the kanban writer).
- [ ] **Step 211** [W] ~25m: The other readers of Step 204's list: `pkg/claudebridge/confirm_sql.go` and `cmd/zhen-claude/{confirm_cmd,review_cmd,main}.go` open `WELL_ZHEN_DSN` for `zhen_*` tables (and the board DSN only where a statement names `kanban_*`), at most 2 connections; `cmd/zhen-cli/main.go`, `pkg/champion/pgstore`, `pkg/zhencapture`, `pkg/database/zhen.go`: each takes the pool it is given; a caller that built it from `WELL_APP_ZHEN_DSN` for a `zhen_*` table now uses `WELL_ZHEN_DSN`. Their tests set the new variable. Help texts and error messages say `unheaded_zhen`.
- [ ] **Step 212** [B][V] ~4m: `GOMAXPROCS=4 go test -race -count=1 ./cmd/zhen-claude/... ./cmd/zhen-cli/... ./pkg/claudebridge/... ./pkg/champion/... ./pkg/zhencapture/... ./pkg/database/... 2>&1 | grep -v '^ok\|no test files' | tail -5` → nothing; `go vet -tags 'wellint integration' ./pkg/... ./cmd/... 2>&1 | tail -3` → nothing.
- [ ] **Step 213** [C] ~1m: **COMMIT** the files of Steps 205-212 by name.

### kanban reads runs from Zhen's database while the module is on

- [ ] **Step 214** [W] ~15m: Red. `cmd/kanban-app/zhenruns_test.go`: `TestRunsPool_OnlyWithTheModule`: with an empty modules directory, `s.zhenRuns(ctx)` returns nil and the injected opener is called 0 times; with `zhen/` present, it is called once, with `DBName` from `WELL_ZHEN_DB` (default `unheaded_zhen`) and every other field of the board's config, and a second call reuses the pool; an opener error → nil, one warning per process (`agentRunWarned`).
- [ ] **Step 215** [B][V] ~1m: `go test ./cmd/kanban-app/ -run RunsPool -count=1 2>&1 | tail -3`. Red.
- [ ] **Step 216** [W] ~25m: Green. `LatestAgentRun` runs on the Zhen pool (`SetMaxOpenConns(2)`), opened lazily through `zhenRuns`; `agentRunFor` (Step 103's guard first) uses it. `docker-compose.yml` `kanban-app` environment: `WELL_ZHEN_DB: "unheaded_zhen"   # Zhen's runs, read only while the module is on (ADR-<N>)`.
- [ ] **Step 217** [B][V] ~2m: `go test -race -count=1 ./cmd/kanban-app/ 2>&1 | tail -1; go vet -tags wellint ./cmd/kanban-app/` → `ok`, nothing.
- [ ] **Step 218** [C] ~1m: **COMMIT** `cmd/kanban-app/runbooks.go cmd/kanban-app/zhenruns_test.go docker-compose.yml` and any kanban file Step 216 changed.

### Retention and backups

- [ ] **Step 219** [W] ~15m: Red. `cmd/well-retention/targets_test.go`: `TestTargets_ZhenTablesInZhenDB` (`zhen_conversations`, `zhen_captures`, `zhen_agent_runs` have `db == dbZhen`, `"unheaded_zhen"`; no `kanban_tasks` query names `zhen_agent_runs`; its `count`, `held` and `archive` queries take `$3`); `TestHeldCards_NoZhenDatabase` (a reader that fails with `database "unheaded_zhen" does not exist` → empty set, no error, one log line).
- [ ] **Step 220** [B][V] ~1m: `go test ./cmd/well-retention/ -run 'ZhenTables|HeldCards' -count=1 2>&1 | tail -3`. Red.
- [ ] **Step 221** [W] ~30m: Green. `targets.go`: `dbZhen = "unheaded_zhen"`; the three targets move to it; `kanban_tasks` takes the held card ids as `$3::text[]` (`AND NOT (k.id = ANY($3))`, `AND k.id = ANY($3)`). `main.go`/`run.go`: `zhenCfg.DBName = getenv("WELL_ZHEN_DB", dbZhen)` beside the app and ops configs; `heldCards` reads `SELECT DISTINCT card_id FROM zhen_agent_runs` on the Zhen connection before the `kanban_tasks` class; a missing database or table is an empty set and one log line. `scripts/well/test-retention.sh`: the Zhen classes on `unheaded_zhen`, a held card still held across the two databases.
- [ ] **Step 222** [B][V] ~2m: `go test -race -count=1 ./cmd/well-retention/ 2>&1 | tail -1` → `ok`.
- [ ] **Step 223** [W] ~20m: Red. `scripts/well/test-backup-restore.sh`, new cases on a source server that has `unheaded_zhen` with a row: (a) a default run: the set has no `unheaded_zhen.dump`, `rowcounts.tsv` names no `unheaded_zhen` table, `well_backup.prom` is written as before; (b) `WELL_BACKUP_DATABASES=unheaded_zhen WELL_BACKUP_EXCLUDE= WELL_BACKUP_SET=zhen`: the set holds `globals.sql`, `cluster.env`, `unheaded_zhen.dump`, `rowcounts.tsv`, `SHA256SUMS` and nothing else; `zhen_backup.prom` holds `unheaded_zhen_backup_last_success_timestamp_seconds`; `restore-drill.sh` passes on it; (c) `WELL_BACKUP_DATABASES=nosuch` → exit 1 naming `nosuch`; (d) `WELL_BACKUP_SET=../x` → exit 2.
- [ ] **Step 224** [B][V] ~6m: (G-EW) `scripts/well/test-backup-restore.sh 2>&1 | tail -4`. Red on (a) to (d).
- [ ] **Step 225** [W] ~25m: Green. `scripts/db-backup.sh`, after the database list is read: `WELL_BACKUP_DATABASES` (space-separated; when set, keep only those, each must exist or exit 1 naming it) else drop the names in `WELL_BACKUP_EXCLUDE` (default `unheaded_zhen`; empty keeps all). `WELL_BACKUP_SET` checked against `^[a-z]{1,16}$` (exit 2): empty → `well_backup.prom` and `unheaded_well_backup_*` exactly as today; set → `<set>_backup.prom` and `unheaded_<set>_backup_*`. The header's "Environment" block documents the three, and the line "<db>.dump pg_dump -Fc of every database that accepts connections" gains "except WELL_BACKUP_EXCLUDE (Zhen's database has its own set, ADR-<N>)".
- [ ] **Step 226** [B][V] ~6m: (G-EW) Step 224 → passes, including the old cases.
- [ ] **Step 227** [W] ~15m: `deploy/systemd/unheaded-zhen-backup.service`: `deploy/systemd/unheaded-well-backup.service` copied, with `Description=Zhen module: backup set of unheaded_zhen (ADR-<N>)`, `PartOf=unheaded-zhen.target` in `[Unit]`, no `OnSuccess=` line (D6: local only until plan 7), `Environment=WELL_BACKUP_DIR=/srv/backups/unheaded-zhen`, `Environment=WELL_BACKUP_DATABASES=unheaded_zhen`, `Environment=WELL_BACKUP_EXCLUDE=`, `Environment=WELL_BACKUP_SET=zhen`, `Environment=DOCKER_CONFIG=/run/unheaded-zhen-backup`, `RuntimeDirectory=unheaded-zhen-backup`, `ReadWritePaths=/srv/backups/unheaded-zhen /var/lib/unheaded/textfile`, every other line as the platform's. `deploy/systemd/unheaded-zhen-backup.timer`: the platform timer's schedule 30 minutes later, `PartOf=unheaded-zhen.target`, `[Install] WantedBy=unheaded-zhen.target`. `modules/zhen/module.yaml`: `timers: [unheaded-zhen-backup.timer]`; `tests/scripts/test_module_manifest.py`: the timer exists and belongs to the switch as in Step 143.
- [ ] **Step 228** [W] ~15m: (Phase 4 merged first: `git -C /home/govan/tmp/wt/S-ZHEN-SWITCH-p7 merge sprint/s-zhen-switch`; Phase 4 not done yet → this step and Steps 229-230 wait.) `modules/zhen/prometheus/rules/zhen.yml`: `ZhenBackupStale`, `time() - max by (host) (unheaded_zhen_backup_last_success_timestamp_seconds) > 26 * 3600`, `for: 30m`, `severity: warning`, `urgency: none`, summary "Zhen's database has had no backup set for over a day", `runbook_url: wiki/Well-Backups.md`; tests in `modules/zhen/prometheus/rules/tests/zhen_test.yml`: quiet at 25 h, firing at 27 h. `observability/alerting/policy.yaml` `promote:` `ZhenBackupStale`, `urgency: ticket`, reason "Zhen is a production application with its own data and backup scope (ADR-147 decision 15 (b)); a day without a set is a ticket".
- [ ] **Step 229** [B] ~5m: `scripts/check-prometheus-rules.sh 2>&1 | tail -2; go run ./cmd/alert-policy 2>&1 | tail -1; go run ./cmd/alert-policy -runbooks 2>&1 | tail -1; /var/tmp/unheaded-sprint/civenv/bin/pytest -q tests/scripts/test_module_manifest.py 2>&1 | tail -1; for s in scripts/check-unit-ceilings.sh scripts/check-unit-binaries.sh scripts/check-unit-flags.sh; do bash $s 2>&1 | tail -1; done`
- [ ] **Step 230** [V] ~1m: All pass.
- [ ] **Step 231** [C] ~1m: **COMMIT** `cmd/well-retention/targets.go cmd/well-retention/main.go cmd/well-retention/run.go cmd/well-retention/targets_test.go scripts/well/test-retention.sh scripts/db-backup.sh scripts/well/test-backup-restore.sh deploy/systemd/unheaded-zhen-backup.service deploy/systemd/unheaded-zhen-backup.timer modules/zhen/module.yaml tests/scripts/test_module_manifest.py modules/zhen/prometheus/rules/zhen.yml modules/zhen/prometheus/rules/tests/zhen_test.yml observability/alerting/policy.yaml`.

### Every throwaway-Well test on the new layout

- [ ] **Step 232** [B] ~20m: (G-EW) One at a time: `for t in test-zhen-database test-zhen-agent-runs test-zhen-captures test-zhen-confirm test-zhen-conversations test-retention test-migrate test-backup-restore; do echo "== $t"; scripts/well/$t.sh 2>&1 | tail -2; done`
- [ ] **Step 233** [V] ~2m: Each ends with its pass line. A `FAIL` naming a `zhen_*` relation in `unheaded_app` → Step 234.
- [ ] **Step 234** [D] ~20m: In the failing script, `app_zhen` connections for `zhen_*` tables go to `unheaded_zhen` (export `WELL_TEST_ZHEN_DSN` beside the existing DSNs); `test-zhen-agent-runs.sh` check 7 ("app_zhen moves a card") stays on `unheaded_app`; its check 10 (041) reads `zhen_agent_runs` in `unheaded_zhen`; its check 11 (049) is unchanged. Two tries per script, then `[STUCK]` with the output.
- [ ] **Step 235** [B] ~5m: The Code gates block, then `go vet -tags 'wellint integration' ./... 2>&1 | tail -3`.
- [ ] **Step 236** [W] ~5m: `$PROG`: fix columns of Z5, Z14, Z15; the pool caps (`zhen-agentd` 6 + 2, `zhen-claude` 2: 10 against the limit 12).
- [ ] **Step 237** [C][V] ~1m: **PHASE 7 EXIT GATE**: Step 232 all pass; the fingerprint differs by the foreign key only; Go green. **COMMIT** the scripts of Step 234 and `$PROG`. Merge the lane into `sprint/s-zhen-switch`.

---

## PHASE 8: LIVE ON WEST (Steps 238-297)

**Goal**: 8A installs the switch, the platform timers, the guard and the readers on west and leaves Zhen on as found. 8B, attended, moves Zhen's tables into `unheaded_zhen` and gives them their own backup set.
**Prerequisite**: G-EW open (Step 18 rerun shows it). Phases 1-7 gates. D1, D3, D4, D5 answered for 8A; D2, D6 for 8B. An unanswered or different answer: the steps it blocks are `[BLOCKED]`.
**Time**: 8A 90 min, unattended. 8B about 2 h with Stevie; Zhen is stopped for about 20 min of it.
**Agent**: Coordinator, sudo.
**Model**: Opus.

**Rollback directory**: `RB=/var/backups/zhen-switch-<YYYY-MM-DD>` (made at Step 242; every replaced file goes there first).

### 8A: the switch on west (unattended)

- [ ] **Step 238** [V] ~2m: G-EW open in `$PROG` and Step 18's command still says so; `$PROG` "Decisions" shows D1, D3, D4, D5 answered as recommended. Either not true → stop 8A.
- [ ] **Step 239** [B] ~20m: Series rule 15 of 2026-11-04: merge develop into the sprint worktree, rebuild, run the plan's closing set there.
  ```bash
  cd /home/govan/tmp/wt/S-ZHEN-SWITCH && git merge --no-ff -S develop -m 'Merge develop into sprint/s-zhen-switch' && go build ./... && GOMAXPROCS=4 go test -race -count=1 ./cmd/... ./pkg/... 2>&1 | grep -v '^ok\|no test files' | tail -10; /var/tmp/unheaded-sprint/civenv/bin/pytest -q tests/scripts tests/runbooks 2>&1 | tail -1; scripts/check-prometheus-rules.sh 2>&1 | tail -2; go run ./cmd/alert-policy 2>&1 | tail -1; scripts/check-module-binds.sh | tail -1; scripts/check-well-migrations.sh | tail -1; ./scripts/check-gosec-ratchet.sh 2>&1 | tail -1
  ```
- [ ] **Step 240** [V] ~2m: No `FAIL`, every gate's pass line. A merge conflict outside `docs/adr/ADR-INDEX.md` and `wiki/ADR-Index.md` (keep both rows in number order) → `[STUCK]`, no merge.
- [ ] **Step 241** [B] ~2m: Merge to develop. 050-052 reach develop here and are not applied until 8B: write under "For Stevie" that nobody runs `well-migrate apply` without `-db` until 8B, and that `zhen-agentd` built from develop needs `WELL_ZHEN_DSN` (installed only in 8B).
  ```bash
  cd /home/govan/tmp/unheaded && git switch develop && { git merge --no-ff -S sprint/s-zhen-switch -m 'Merge sprint/s-zhen-switch: Zhen is a module behind one switch (ADR-147 decision 15)' || { git merge --abort; git merge --no-ff --no-gpg-sign sprint/s-zhen-switch -m 'Merge sprint/s-zhen-switch: Zhen is a module behind one switch (ADR-147 decision 15)'; }; } && git log --oneline -1
  ```
- [ ] **Step 242** [S][B] ~3m: Rollback copies.
  ```bash
  RB=/var/backups/zhen-switch-$(date -u +%F); sudo install -d -m 0750 $RB && sudo cp -p /etc/systemd/system/zhen-agentd.service /etc/systemd/system/unheaded-zhen-ui-front.service /etc/systemd/system/unheaded-vor.service /etc/sysusers.d/unheaded.conf /usr/local/bin/zhen-agentd /usr/local/bin/zhen-ui-front /usr/local/bin/wiki-server /usr/local/lib/unheaded/well/db-backup.sh $RB/ && for i in unheaded-dashboard unheaded-kanban; do sudo docker tag "$(sudo docker inspect -f '{{.Image}}' $i)" "$i:pre-zhen-switch"; done; sudo ls $RB; sudo docker image ls | grep -c pre-zhen-switch
  ```
  Expected: eight files; `2`.
- [ ] **Step 243** [S][B] ~3m: Accounts, the runtime directory, the textfile ACL.
  ```bash
  cd /home/govan/tmp/unheaded && sudo install -m 0644 deploy/sysusers.d/unheaded.conf /etc/sysusers.d/unheaded.conf && sudo systemd-sysusers && sudo install -m 0644 deploy/tmpfiles.d/unheaded-modules.conf /etc/tmpfiles.d/ && sudo systemd-tmpfiles --create /etc/tmpfiles.d/unheaded-modules.conf && sudo setfacl -m u:unheaded-health-sweep:rwx,u:unheaded-ioc-audit:rwx,u:unheaded-modules:rwx,u:unheaded-module-guard:rwx /var/lib/unheaded/textfile
  ```
- [ ] **Step 244** [V] ~1m: `getent passwd unheaded-health-sweep unheaded-ioc-audit unheaded-modules unheaded-module-guard | cut -d: -f1,3` → four lines, 16714-16717 (or Step 23's mapping); `stat -c '%U %a' /run/unheaded/modules` → `unheaded-modules 755`; `getfacl -p /var/lib/unheaded/textfile | grep -c -E 'unheaded-(health-sweep|ioc-audit|modules|module-guard)'` → 4.
- [ ] **Step 245** [S][B] ~3m: Scripts, the module tree, the schedules' runner and runbooks, the targets file.
  ```bash
  cd /home/govan/tmp/unheaded && L=/usr/local/lib/unheaded && sudo install -D -m 0755 scripts/monitoring/platform-schedule.sh $L/monitoring/platform-schedule.sh && sudo install -D -m 0755 scripts/monitoring/module-listener-guard.sh $L/monitoring/module-listener-guard.sh && sudo install -D -m 0644 scripts/lib/module_binds.py $L/lib/module_binds.py && sudo install -D -m 0755 scripts/modules/module-fragments.sh $L/modules/module-fragments.sh && sudo rm -rf $L/modules/zhen && sudo cp -r modules/zhen $L/modules/zhen && sudo find $L/modules/zhen -type d -exec chmod 0755 {} + && sudo find $L/modules/zhen -type f -exec chmod 0644 {} + && sudo install -D -m 0644 scripts/run-runbook.py $L/schedules/run-runbook.py && sudo install -D -m 0644 scripts/svc_token.py $L/schedules/svc_token.py && sudo install -D -m 0644 runbooks/observe/service-health-sweep.yaml $L/schedules/runbooks/observe/service-health-sweep.yaml && sudo install -D -m 0644 runbooks/security/ioc-audit.yaml $L/schedules/runbooks/security/ioc-audit.yaml && sudo install -m 0644 configs/health-sweep-west.targets /etc/unheaded/health-sweep.targets
  ```
  If `module-listener-guard.sh` finds `module_binds.py` by a path relative to itself, install the pair so that path holds (read its first lines); write what was done in `$PROG`.
- [ ] **Step 246** [V] ~2m: `for f in scripts/monitoring/platform-schedule.sh scripts/modules/module-fragments.sh scripts/run-runbook.py; do sudo sha256sum /usr/local/lib/unheaded/*/$(basename $f) $f | awk '{print $1}' | uniq | wc -l; done` prints `1` three times; `sudo diff -r modules/zhen /usr/local/lib/unheaded/modules/zhen && echo same`.
- [ ] **Step 247** [S][B] ~5m: Units. First the host copies against the repository's previous version, so host drift is not overwritten blind:
  ```bash
  cd /home/govan/tmp/unheaded && for u in zhen-agentd unheaded-zhen-ui-front unheaded-vor; do echo "== $u"; git show HEAD^1:deploy/systemd/$u.service | diff - /etc/systemd/system/$u.service | head -20; done
  ```
  Any difference: the host carries a change the repository does not; write it under "For Stevie", carry it into the repository's file in a follow-up commit on develop (signed), and only then go on. Then:
  ```bash
  cd /home/govan/tmp/unheaded && for f in unheaded-health-sweep.service unheaded-health-sweep.timer unheaded-ioc-audit.service unheaded-ioc-audit.timer unheaded-module-guard.service unheaded-module-guard.timer unheaded-zhen.target unheaded-zhen-module.service zhen-agentd.service unheaded-zhen-ui-front.service unheaded-vor.service; do sudo install -m 0644 deploy/systemd/$f /etc/systemd/system/$f; done && sudo systemctl daemon-reload && sudo systemctl enable --now unheaded-health-sweep.timer unheaded-module-guard.timer && systemctl is-enabled unheaded-ioc-audit.timer
  ```
  The last prints `disabled` (D7). `unheaded-zhen-app` and `unheaded-llama`: only if `systemctl cat` finds them installed (S-ZHEN-FRONT Phase 10), install their repository files the same way and record `systemctl is-enabled` of each before and after in `$PROG`.
- [ ] **Step 248** [S][B] ~2m: Wire the switch without restarting anything.
  ```bash
  systemctl show -p ActiveEnterTimestamp zhen-agentd unheaded-zhen-ui-front; sudo systemctl disable zhen-agentd.service unheaded-zhen-ui-front.service && sudo systemctl enable zhen-agentd.service unheaded-zhen-ui-front.service unheaded-zhen-module.service && sudo systemctl enable --now unheaded-zhen.target; systemctl show -p ActiveEnterTimestamp zhen-agentd unheaded-zhen-ui-front
  ```
  A unit of Step 247's last sentence that was enabled before is enabled again here.
- [ ] **Step 249** [V] ~2m: `systemctl is-enabled unheaded-zhen.target` and `is-active` → `enabled`, `active`; `ls /etc/systemd/system/unheaded-zhen.target.wants/` lists the three units; `ls /etc/systemd/system/multi-user.target.wants/ | grep -E 'zhen|llama'` → only `unheaded-zhen.target`; both `ActiveEnterTimestamp` pairs equal (nothing restarted); `ls /run/unheaded/modules/zhen` → `dashboard prometheus victoria`.
- [ ] **Step 250** [D] ~5m: The oneshot failed: `journalctl -u unheaded-zhen-module -n 20 --no-pager`. "Permission denied" on `/run/unheaded/modules` → Step 243's tmpfiles line did not apply (owner); on the textfile → the ACL. Two tries, then Appendix A procedure 2.
- [ ] **Step 251** [S][B] ~3m: Prometheus with the mount and the reload flags.
  ```bash
  cd /home/govan/tmp/unheaded && sudo docker compose -f monitoring/docker-compose.yml up -d --no-deps prometheus && sleep 60 && curl -s -m5 http://127.0.0.1:9090/api/v1/status/flags | python3 -c 'import json,sys; f=json.load(sys.stdin)["data"]; print(f.get("config.auto-reload-interval"), f.get("enable-feature"))'
  ```
- [ ] **Step 252** [V] ~2m: `30s auto-reload-config` (or, under "reload: HUP", `None`); Step 13's command prints `[('zhen', 3)]` (two `ZhenSchedulerStalled` and `ZhenBackupStale`); `curl -s 127.0.0.1:9090/api/v1/rules | grep -o '"name":"\(schedules\|modules\)"' | sort -u` → both; `curl -s 127.0.0.1:9090/api/v1/targets | grep -c '"job":"zhen-agentd"'` ≥ 1.
- [ ] **Step 253** [D] ~5m: Group `zhen` missing: `sudo docker exec unheaded-prometheus ls /etc/unheaded-modules/zhen/prometheus/rules/` (empty → the mount was made before the directory existed: `up -d --no-deps --force-recreate prometheus`); `sudo docker logs --since 5m unheaded-prometheus 2>&1 | grep -i -E 'error|rule' | head`. Two tries, then Appendix A procedure 3.
- [ ] **Step 254** [S][B] ~3m: VictoriaMetrics with the mount and the re-read interval (a scrape gap of under a minute).
  ```bash
  cd /home/govan/tmp/unheaded && sudo docker compose up -d --no-deps victoria && sleep 90 && curl -s -m5 http://127.0.0.1:8428/api/v1/targets | python3 -c 'import json,sys; print([t["health"] for t in json.load(sys.stdin)["data"]["activeTargets"] if t["labels"].get("job")=="zhen-agentd"])'
  ```
- [ ] **Step 255** [V] ~1m: `['up']`; `q 'count(up)'` equals its value before the restart (read it first, in the same shell). Else Appendix A procedure 3.
- [ ] **Step 256** [S][B] ~10m: dashboard-backend (Phase 3's code, the module mount) and the wiki server (the menu).
  ```bash
  cd /home/govan/tmp/unheaded && sudo docker compose build dashboard-backend && sudo docker compose up -d --no-deps dashboard-backend && go build -o /var/tmp/zhen-switch/wiki-server ./cmd/wiki-server && sudo install -m 0755 /var/tmp/zhen-switch/wiki-server /usr/local/bin/wiki-server && sudo systemctl restart unheaded-wiki-server && systemctl is-active unheaded-wiki-server
  ```
  (Build it the way `deploy/systemd/unheaded-wiki-server.service`'s header says if that differs: ldflags with the version.)
- [ ] **Step 257** [V] ~2m: `curl -s --cacert /etc/unheaded/tls/ca.pem https://127.0.0.1:20000/metrics | grep '^unheaded_dashboard_health_targets'` → a `module="zhen"` line with `2` and a `module=""` line; `curl -s --cacert /etc/unheaded/tls/ca.pem https://127.0.0.1:20000/auth/nav.js | grep -c zhenai` → 1; `curl -s http://127.0.0.1:20002/auth/nav.js | grep -c zhenai` → 1. (Use the CA path Step 54 found.)
- [ ] **Step 258** [S][B] ~3m: The platform's sweep, once now.
  ```bash
  sudo systemctl start unheaded-health-sweep.service; systemctl show -p Result unheaded-health-sweep.service; journalctl -u unheaded-health-sweep --since -5min --no-pager -o cat | grep -E 'RESULT|FAIL' | head; cat /var/lib/unheaded/textfile/schedule_service-health-sweep.prom
  ```
- [ ] **Step 259** [V] ~1m: `Result=success`; `unheaded_schedule_last_exit_code{schedule="service-health-sweep"} 0`. A `[FAIL]` line names a platform service that does not answer: that is a finding for plan 1, written under "For Stevie"; the timer stays. A targets-file error: correct `configs/health-sweep-west.targets` on develop (signed commit), reinstall it, run again.
- [ ] **Step 260** [B] ~6m: The guard has run (its timer fires 2 minutes after the enable): `systemctl list-timers unheaded-module-guard.timer --no-pager | tail -2; cat /var/lib/unheaded/textfile/module_listeners.prom`
- [ ] **Step 261** [V] ~1m: `unheaded_module_listener_public_total{module="zhen"} 0`. Above 0 → the per-listener line names the port and address: write it at the top of "For Stevie"; `ModuleListenerPublic` will page, which is the design.
- [ ] **Step 262** [S][B] ~2m: The allowlist drop-ins (read by the new binaries from 8B on; the running ones ignore them).
  ```bash
  cd /home/govan/tmp/unheaded && for u in zhen-agentd unheaded-zhen-ui-front; do sudo install -D -m 0644 modules/zhen/hosts/west/$u.service.d/bind-allow.conf /etc/systemd/system/$u.service.d/bind-allow.conf; done && sudo systemctl daemon-reload && systemctl show -p Environment zhen-agentd | grep -o 'UNHEADED_MODULE_BIND_ALLOW=[^ ]*'
  ```
  Expected: `UNHEADED_MODULE_BIND_ALLOW=fd00:dead:beef::1/128`.
- [ ] **Step 263** [V] ~2m: `sudo /home/govan/tmp/unheaded/scripts/test-zhen-switch.sh --expect on --wait 90` → exit 0 (C10 is not checked when on).
- [ ] **Step 264** [C] ~1m: **COMMIT** `$PROG` on develop (main checkout, named file): "[PLAN S-ZHEN-SWITCH] Steps 238-264: the switch, the platform timers and the guard on west; Zhen on as found".

### 8B: Zhen's tables move (ATTENDED)

- [ ] **Step 265** [STEVIE] ~1m: "Move Zhen's tables into their own database now? About 2 hours; Zhen stops for about 20 minutes; nothing else stops. S-WEST-GROUND's database move is not in this sitting." A no: Steps 266-296 are `[BLOCKED]`; Z5, Z14, Z15 stay open with that reason; go to Phase 9.
- [ ] **Step 266** [B] ~3m: Preconditions. `git -C /home/govan/tmp/unheaded log --oneline -1 -- db/migrations/052_app_drop_zhen.sql` (on develop); `tail -5 /home/govan/tmp/unheaded/docs/battle-plans/progress/S-WEST-GROUND-progress.md` (no move in flight). Stevie puts the migration password in the shell without echo: `read -rs WELL_MIGRATE_PASSWORD && export WELL_MIGRATE_PASSWORD`.
- [ ] **Step 267** [B] ~2m: `cd /home/govan/tmp/unheaded && go run ./cmd/well-migrate status; echo exit=$?`
  Expected: `exit=3` with 050 pending in `postgres`, 052 in `unheaded_app`, and `unheaded_zhen` reported missing or with 051 pending. Any other pending file → stop: it is not this plan's; "For Stevie".
- [ ] **Step 268** [S][B] ~3m: Z5 and Z14 before (Step 19's command, if Step 19 used Erda's figures) and exact counts per table, into the rollback directory (counts only, no rows):
  ```bash
  RB=/var/backups/zhen-switch-$(date -u +%F); for t in $(sudo docker exec -i unheaded-postgres psql -U unheaded -X -At -d unheaded_app -c "SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relkind IN ('r','p') AND c.relname LIKE 'zhen\_%' ORDER BY 1"); do printf '%s\t%s\n' $t "$(sudo docker exec -i unheaded-postgres psql -U unheaded -X -At -d unheaded_app -c "SELECT count(*) FROM $t")"; done | sudo tee $RB/counts-before.tsv
  ```
- [ ] **Step 269** [S][B] ~15m: A fresh platform set and its drill: the rollback point.
  ```bash
  sudo systemctl start unheaded-well-backup.service && S=$(sudo cat /srv/backups/unheaded-well/LATEST) && echo "$S" && sudo /usr/local/lib/unheaded/well/restore-drill.sh "/srv/backups/unheaded-well/${S#/srv/backups/unheaded-well/}" 2>&1 | tail -3
  ```
- [ ] **Step 270** [V] ~1m: The drill passes. Write the set's name in `$PROG` as "rollback set". A failure: stop 8B (Appendix A procedure 4 needs a good set).
- [ ] **Step 271** [S][B] ~2m: Stop Zhen without disabling the switch: `sudo systemctl stop unheaded-zhen.target; systemctl is-active zhen-agentd unheaded-zhen-ui-front`, then Step 19's sessions query.
- [ ] **Step 272** [V] ~1m: `inactive` twice; `sessions|0`. Above 0 → `SELECT application_name, client_addr, backend_start FROM pg_stat_activity WHERE usename = 'app_zhen'`: a `zhen-claude` or `zhen-cli` in a shell; Stevie closes it.
- [ ] **Step 273** [B] ~2m: `cd /home/govan/tmp/unheaded && go run ./cmd/well-migrate apply -db postgres -yes 2>&1 | tail -3 && go run ./cmd/well-migrate apply -db unheaded_zhen -yes 2>&1 | tail -3`
- [ ] **Step 274** [V] ~1m: Both report their file applied; `go run ./cmd/well-migrate status -db unheaded_zhen` → clean; `sudo docker exec -i unheaded-postgres psql -U unheaded -X -At -d postgres -c "SELECT datname, pg_get_userbyid(datdba) FROM pg_database WHERE datname='unheaded_zhen'" -c "SELECT rolconnlimit FROM pg_roles WHERE rolname='app_zhen'"` → `unheaded_zhen|well_zhen_owner`, `12`.
- [ ] **Step 275** [D] ~5m: 050 failed part way (notx): the runner reports which statement. Fix forward only if the cause is clear (a role name that differs live); otherwise Appendix A procedure 4 (before 052: drop what 050 made) and `[STUCK]`.
- [ ] **Step 276** [S][B] ~5m: Copy the rows, triggers off for the load, one transaction. (Amended 2026-11-15, REV-6: 052 tightens columns, so a plain `pg_dump --data-only` no longer fits; the script maps each old column to the new one, a NULL to the column's default, a '' trace id to NULL, checks counts on both sides and records them for 053.)
  ```bash
  cd /home/govan/tmp/unheaded && sudo scripts/well/zhen-copy-rows.sh; echo "exit=$?"
  ```
- [ ] **Step 277** [V] ~3m: `exit=0`, and the script's eight `table<TAB>count` lines. Step 268's loop with `-d unheaded_zhen` into `$RB/counts-after.tsv`; `sudo diff $RB/counts-before.tsv $RB/counts-after.tsv && echo same` → `same`. For every sequence in `unheaded_zhen`: `SELECT last_value` is at least the `max` of its column.
- [ ] **Step 278** [D] ~5m: A count differs → something wrote during the copy: back to Step 271 after `TRUNCATE` of the tables in `unheaded_zhen` only (`psql -d unheaded_zhen`, superuser; 051's triggers refuse it otherwise: `SET session_replication_role = replica` first). A sequence behind → `SELECT setval('<seq>', (SELECT max(<col>) FROM <table>))` in `unheaded_zhen`.
- [ ] **Step 279** [S][B] ~2m: The fingerprint, live.
  ```bash
  cd /home/govan/tmp/unheaded && for d in unheaded_app unheaded_zhen; do sudo docker exec -i unheaded-postgres psql -U unheaded -X -At -d $d < scripts/well/zhen-schema-fingerprint.sql > /var/tmp/zhen-switch/fp-$d.txt; done; diff /var/tmp/zhen-switch/fp-unheaded_app.txt /var/tmp/zhen-switch/fp-unheaded_zhen.txt
  ```
  Then `python3 -I scripts/well/zhen_schema_diff.py /var/tmp/zhen-switch/fp-unheaded_app.txt /var/tmp/zhen-switch/fp-unheaded_zhen.txt` (REV-6). Expected: `same but for the <n> listed change(s)` (`scripts/well/zhen-schema-changes.txt`: the foreign key to `kanban_tasks` and 052's tightened columns). An `unexpected:` line → the live `unheaded_app` differs from the migrations: stop, `[STUCK]`, "For Stevie" (Appendix A procedure 4, before 053).
- [ ] **Step 280** [V] ~1m: The copy recorded itself for 053's guard (REV-6: the script sets `unheaded.zhen_rows_copied` to `copied <time>: <table>=<count>,...`; 053 drops nothing unless every zhen_* table still holds its count): `sudo docker exec -i unheaded-postgres psql -U unheaded -X -At -d postgres -c "SELECT array_to_string(s.setconfig, ',') FROM pg_db_role_setting s JOIN pg_database d ON d.oid = s.setdatabase WHERE d.datname = 'unheaded_app'"` names all eight tables.
- [ ] **Step 281** [B] ~2m: `cd /home/govan/tmp/unheaded && go run ./cmd/well-migrate apply -db unheaded_app -yes 2>&1 | tail -3`
- [ ] **Step 282** [V] ~1m: `go run ./cmd/well-migrate status; echo exit=$?` → `exit=0`; Step 19's table query on `unheaded_app` → `tables|0|<13 minus the moved>`; `kanban_zhen_optin` still there (`\dt kanban_zhen_optin`); `SELECT count(*) FROM pg_db_role_setting s JOIN pg_database d ON d.oid=s.setdatabase WHERE d.datname='unheaded_app' AND array_to_string(s.setconfig, ',') LIKE '%zhen_rows_copied%'` → 0.
- [ ] **Step 283** [S][B] ~3m: Zhen's DSN for its own tables, written without printing the file.
  ```bash
  F=/etc/unheaded/zhen-agentd.env; RB=/var/backups/zhen-switch-$(date -u +%F); sudo cp -p $F $RB/zhen-agentd.env && sudo sh -c "grep -q '^WELL_ZHEN_DSN=' $F || sed -n '/^WELL_APP_ZHEN_DSN=/{s/^WELL_APP_ZHEN_DSN=/WELL_ZHEN_DSN=/;s/unheaded_app/unheaded_zhen/;p}' $F >> $F" && sudo grep -c '^WELL_ZHEN_DSN=.*unheaded_zhen' $F && sudo grep -c '^WELL_APP_ZHEN_DSN=.*unheaded_app' $F
  ```
  Expected: `1`, `1`. Stevie's own shell for `zhen-claude` needs the same variable: "For Stevie".
- [ ] **Step 284** [S][B] ~10m: The new binaries, scripts and units.
  ```bash
  cd /home/govan/tmp/unheaded && make build-zhen-agentd build-zhen-ui-front 2>&1 | tail -2 && go build -o /var/tmp/zhen-switch/well-retention ./cmd/well-retention && sudo cp -p /usr/local/bin/well-retention /var/backups/zhen-switch-$(date -u +%F)/ && sudo install -m 0755 bin/zhen-agentd bin/zhen-ui-front /var/tmp/zhen-switch/well-retention /usr/local/bin/ && sudo install -m 0755 scripts/db-backup.sh /usr/local/lib/unheaded/well/db-backup.sh && for f in unheaded-zhen-backup.service unheaded-zhen-backup.timer; do sudo install -m 0644 deploy/systemd/$f /etc/systemd/system/$f; done && sudo install -d -m 0700 /srv/backups/unheaded-zhen && sudo systemctl daemon-reload && sudo systemctl enable unheaded-zhen-backup.timer
  ```
  (Build `well-retention` with the ldflags its unit header names if it gives any.)
- [ ] **Step 285** [S][B] ~6m: kanban-app with the runs pool: `cd /home/govan/tmp/unheaded && sudo docker compose build kanban-app && sudo docker compose up -d --no-deps kanban-app && sleep 20 && sudo docker logs --since 1m unheaded-kanban 2>&1 | grep -i -E 'error|agent run' | head -5`
- [ ] **Step 286** [S][B] ~2m: Zhen back on: `sudo systemctl start unheaded-zhen.target && sleep 30 && systemctl is-active zhen-agentd unheaded-zhen-ui-front unheaded-zhen-backup.timer && journalctl -u zhen-agentd --since -2min --no-pager -o cat | grep -i -E 'error|refus|WELL_|ADR-' | head -5`
- [ ] **Step 287** [V] ~2m: `active` three times; no line; `sudo docker exec -i unheaded-postgres psql -U unheaded -X -At -d postgres -c "SELECT datname, count(*) FROM pg_stat_activity WHERE usename='app_zhen' GROUP BY 1 ORDER BY 1"` → rows only for `unheaded_app` and `unheaded_zhen`, total at most 12; `ss -ltnH | awk '{print $4}' | grep -E ':(20103|20111|20112)$'` → Step 14's reference.
- [ ] **Step 288** [D] ~10m: `zhen-agentd` exits: its message names `WELL_ZHEN_DSN` (Step 283) or a listener refused by the bind rule (the drop-in of Step 262, or a `ZHEN_METRICS_ADDR` that is not on the list: the env file's value is read with `sudo grep -c '^ZHEN_METRICS_ADDR=\[fd00:dead:beef::1\]:20111$'`, never printed). Two tries, then Appendix A procedure 5.
- [ ] **Step 289** [S][B] ~10m: Zhen's first set and its drill.
  ```bash
  sudo systemctl start unheaded-zhen-backup.service && S=$(sudo cat /srv/backups/unheaded-zhen/LATEST) && sudo /usr/local/lib/unheaded/well/restore-drill.sh "/srv/backups/unheaded-zhen/${S#/srv/backups/unheaded-zhen/}" 2>&1 | tail -3; cat /var/lib/unheaded/textfile/zhen_backup.prom
  ```
- [ ] **Step 290** [V] ~1m: The drill passes; `zhen_backup.prom` holds the five `unheaded_zhen_backup_*` series. A refusal by `WELL_BACKUP_OTHER_DISK=1` → plan 4 has moved the databases to the NVMe: use the platform set's current rule (read `systemctl cat unheaded-well-backup`) and write it in `$PROG`.
- [ ] **Step 291** [S][B] ~15m: Z15 after: the platform's set without Zhen. `sudo systemctl start unheaded-well-backup.service && sleep 5 && cat /var/lib/unheaded/textfile/well_backup.prom | grep -E 'databases|rows'` (its offsite copy follows, as every night).
- [ ] **Step 292** [V] ~1m: `unheaded_well_backup_databases` equals Step 17's before (the platform's databases; `unheaded_zhen` is not among them); `unheaded_well_backup_rows` fell by about the sum of `counts-before.tsv` (live churn aside). `sudo ls /srv/backups/unheaded-well/$(sudo cat /srv/backups/unheaded-well/LATEST | sed 's|^/srv/backups/unheaded-well/||') | grep -c zhen` → 0.
- [ ] **Step 293** [B] ~3m: Retention against the new layout, counting only: run `well-retention -dry-run` the way its unit runs it (`systemctl cat unheaded-well-retention` gives the account and the credential lines; use `sudo systemd-run --wait --pipe --collect` with the same `User=` and credential properties). If that cannot be done without reading a secret, leave it to the timer's next run and read its journal in Phase 10.
- [ ] **Step 294** [V] ~1m: The run line `dry_run=true`; the three Zhen classes count against `unheaded_zhen`; no error naming `zhen_agent_runs`.
- [ ] **Step 295** [W] ~5m: `$PROG`: Z5, Z14, Z15 after (Steps 282, 287, 292), the counts, the rollback set, the time Zhen was stopped (Steps 271 to 286).
- [ ] **Step 296** [C] ~1m: **COMMIT** `$PROG` on develop: "[PLAN S-ZHEN-SWITCH] Steps 265-296: Zhen's tables in their own database on west".
- [ ] **Step 297** [V] ~1m: **PHASE 8 EXIT GATE**: Step 263 passed; 8B passed, or is `[BLOCKED]` by Stevie's no with Z5, Z14, Z15 open and the reason written. `unset WELL_MIGRATE_PASSWORD`.

---

## PHASE 9: THE SWITCH PROVED, AND EVERY "AFTER" (Steps 298-317)

**Goal**: The switch is flipped off and on, west's platform is seen whole while Zhen is off, both states pass within 90 s, and every row has its after.
**Prerequisite**: Phase 8A gate (8B passed or blocked). Not during an attended change of another plan.
**Time**: 75 min (10 of them with Zhen off).
**Agent**: Coordinator, sudo.
**Model**: Opus.

- [ ] **Step 298** [B] ~2m: The state as found: `sudo /home/govan/tmp/unheaded/scripts/test-zhen-switch.sh --expect on; q 'ALERTS{alertstate="firing"}' | sort > /var/tmp/zhen-switch/alerts-on.txt; wc -l < /var/tmp/zhen-switch/alerts-on.txt`
- [ ] **Step 299** [S][B] ~3m: Off.
  ```bash
  T0=$(date +%s); sudo systemctl disable --now unheaded-zhen.target; sudo /home/govan/tmp/unheaded/scripts/test-zhen-switch.sh --expect off --wait 90; echo "rc=$? took=$(( $(date +%s) - T0 ))s"
  ```
- [ ] **Step 300** [V] ~1m: `rc=0`, `took` at most 90 s. Log both.
  - A `FAIL` → Step 301.
- [ ] **Step 301** [D] ~10m: By check: C4 or C5 still present → Prometheus did not re-read (Step 252's flags; under auto mode, `curl -s 127.0.0.1:9090/api/v1/status/config | grep -c unheaded-modules`; switch to "reload: HUP" by Steps 113 and 142, merged and reinstalled); C6 → VictoriaMetrics' interval; C7 → the dashboard's 30 s loop (run again with `--wait 120`, log the time); C9 → a Zhen process outside its unit (`ss -ltnp` for the port, as root). Two tries, then Step 306 (switch back on) and `[STUCK]`.
- [ ] **Step 302** [B] ~11m: The platform whole without Zhen: wait 10 minutes, then `q 'ALERTS{alertstate="firing"}' | sort > /var/tmp/zhen-switch/alerts-off.txt; diff /var/tmp/zhen-switch/alerts-on.txt /var/tmp/zhen-switch/alerts-off.txt; sudo systemctl start unheaded-health-sweep.service; systemctl show -p Result unheaded-health-sweep.service`
- [ ] **Step 303** [V] ~1m: The diff shows no new alert (a `Zhen*` alert leaving is the design); `Result=success`.
- [ ] **Step 304** [S][B] ~3m: Z2 and Z9 after: Step 15's command, over the 10 minutes with Zhen off.
- [ ] **Step 305** [V] ~1m: Four `0`s. A line that names Zhen from a platform unit or container is a Zhen assumption: copy it to `$PROG` as a new row (Z16 and on), fix it in that program's file on develop (test first, signed commit), and repeat Steps 299-305 once.
- [ ] **Step 306** [S][B] ~3m: On.
  ```bash
  T0=$(date +%s); sudo systemctl enable --now unheaded-zhen.target; sudo /home/govan/tmp/unheaded/scripts/test-zhen-switch.sh --expect on --wait 90; echo "rc=$? took=$(( $(date +%s) - T0 ))s"
  ```
- [ ] **Step 307** [V] ~6m: `rc=0`, at most 90 s; five minutes later `q 'ALERTS{alertname="ZhenSchedulerStalled",alertstate="firing"}'` → `(no series)`.
- [ ] **Step 308** [STEVIE][DECIDE] ~3m: D7, if still open: "Approve `runbooks/security/ioc-audit.yaml` and seal the manifest, so its platform timer can run daily?" Yes, once Stevie's approval and seal are a commit on develop: `cd /home/govan/tmp/unheaded && sudo install -m 0644 runbooks/security/ioc-audit.yaml /usr/local/lib/unheaded/schedules/runbooks/security/ioc-audit.yaml && sudo systemctl enable --now unheaded-ioc-audit.timer && sudo systemctl start unheaded-ioc-audit.service; systemctl show -p Result unheaded-ioc-audit.service`. Not yet: the timer stays disabled; Z12 "waiting on D7".
- [ ] **Step 309** [V] ~1m: (if enabled) `schedule_ioc-audit.prom` exists; `Result=success`, or `exit-code` with an indicator in `journalctl -u unheaded-ioc-audit` (a finding, read by Stevie); `q 'unheaded_schedule_last_success_timestamp_seconds{schedule="ioc-audit"}'` has a series.
- [ ] **Step 310** [V] ~1m: D8: the switch is in the state Stevie chose (default: on, as found): `systemctl is-enabled unheaded-zhen.target; systemctl is-active unheaded-zhen.target`.
- [ ] **Step 311** [B] ~2m: Z1 after: Step 10's first two commands on develop → `0`, `0`. East: "with plan 6's debs" (Step 10's third reading is unchanged by design).
- [ ] **Step 312** [B] ~3m: Z3, Z4, Z7, Z8, Z10, Z12, Z13 after, on develop and west.
  ```bash
  cd /home/govan/tmp/unheaded && grep -c -E '^(zhenai|llama-server)=' config/dashboard/services.txt; grep -c 'for Zhen' deploy/systemd/unheaded-vor.service; sed -n '/^build-services:/p' Makefile | grep -c 'build-zhen-rag'; test -e monitoring/prometheus/rules/zhen.yml && echo present || echo absent; grep -c -E 'name: (service-health-sweep|ioc-audit)' configs/zhen-schedules.yaml; q 'time() - unheaded_schedule_last_success_timestamp_seconds'; systemctl list-timers --all --no-pager | grep -c -E 'health-sweep|ioc-audit'
  ```
  Expected: `0`, `0`, `0`, `absent`, `0`, a series for `service-health-sweep` under 6 h (and `ioc-audit` if D7 is yes), `2`. Z8's after is C8 of Steps 299 and 306 (absent off, present on).
- [ ] **Step 313** [B] ~3m: Z6 after: Step 14's command; `cat /var/lib/unheaded/textfile/module_listeners.prom | grep total`; with a clean tree, `cd /home/govan/tmp/unheaded && scripts/check-gates-can-fail.sh --only check-module-binds 2>&1 | tail -2`.
- [ ] **Step 314** [V] ~1m: The Step 14 reference; total `0`; the gate is shown to fail when provoked.
- [ ] **Step 315** [W] ~10m: `$PROG`: every row Z1-Z15 with its after, its status (`fixed`, `handoff` with the plan, `waiting on Stevie` with the decision), and the switch's two times.
- [ ] **Step 316** [C] ~1m: **COMMIT** `$PROG` on develop: "[PLAN S-ZHEN-SWITCH] Steps 298-316: the switch proved off and on; after readings".
- [ ] **Step 317** [V] ~1m: **PHASE 9 EXIT GATE**: Steps 300 and 307 passed; every row has an after or a written reason.

---

## PHASE 10: CLOSE (Steps 318-330)

**Goal**: Gates green together on develop, the record and the wiki say what was built, the handoff is written.
**Prerequisite**: Phases 0-9 gates, or their `[STUCK]`/`[BLOCKED]` markers.
**Time**: 60 min.
**Agent**: Coordinator.
**Model**: Opus.

- [ ] **Step 318** [B] ~15m: The full local CI, the CI venv last on `PATH`: `cd /home/govan/tmp/unheaded && PATH=$PATH:/var/tmp/unheaded-sprint/civenv/bin scripts/ci-local.sh 2>&1 | tail -25`
- [ ] **Step 319** [V] ~2m: Every gate passes, or each `FAIL` is shown to fail on `1393e5ee` too (a clean clone of that commit, the same gate).
- [ ] **Step 320** [D] ~10m: A `FAIL` this plan caused: fix it in its phase's files, signed commit, Step 318 again. Two tries.
- [ ] **Step 321** [W] ~10m: `docs/adr/ADR-<N>-*.md`: a dated section "Implemented on west, <date> (S-ZHEN-SWITCH)" with the switch's two times and the progress file's path; status stays Proposed (acceptance is Stevie's). `docs/adr/ADR-147-every-service-earns-its-place.md`: a dated update at the end, "decision 15 carried out by S-ZHEN-SWITCH: <one line per row: fixed or handed off>"; nothing above it is edited. `docs/battle-plans/SPRINT-SERIES-2026-11-15-ADR-147.md`: plan 3's State → "executed <date>; 8B <done or waiting>".
- [ ] **Step 322** [W] ~20m: Librarian ripple, terse: `wiki/Zhen-AI.md` (the switch, its two commands and the checker; the Scheduler section: the sweep and the audit are the platform's, on timers; the bind rule); `wiki/Systemd-Units.md` (the target, the oneshot, the three timers, the four accounts with ids); `wiki/The-Well.md` (`unheaded_zhen`, its owner, `app_zhen`'s limit 12, the two DSNs); `wiki/Well-Backups.md` (Zhen's set at `/srv/backups/unheaded-zhen`, local only, its drill); `wiki/Alerting.md` (`HealthSweepNotSucceeding`, `IOCAuditNotSucceeding` on the platform's series, `ModuleListenerPublic`, `ModuleListenerGuardNotRunning`, `ZhenBackupStale`, Zhen's rules only while on). `git grep -n -E 'zhen-schedules|IOC audit|ioc-audit' -- CLAUDE.md wiki` and correct any line that says Zhen schedules them.
- [ ] **Step 323** [B] ~2m: `cd /home/govan/tmp/unheaded && git grep -n -i -E 'scheduled (by|inside) zhen|zhen.?s scheduler runs (the )?(ioc|health)' -- wiki CLAUDE.md | head`; the wiki's link check (`ls scripts | grep -i -E 'wiki.*(link|check)'`, run it).
- [ ] **Step 324** [V] ~1m: No such line; the link check passes.
- [ ] **Step 325** [C] ~1m: **COMMIT** the ADR files, `docs/adr/ADR-INDEX.md` if its status cell changed, the series index, the wiki pages by name.
- [ ] **Step 326** [B] ~1m: `cd /home/govan/tmp/unheaded && for w in -p2 -p3 -p4 -p6 -p7 ""; do git worktree remove /home/govan/tmp/wt/S-ZHEN-SWITCH$w 2>/dev/null; done; git worktree list | grep -c S-ZHEN-SWITCH; rm -rf /var/tmp/zhen-switch`
- [ ] **Step 327** [V] ~1m: `0`. The rollback directory and the two `pre-zhen-switch` image tags stay: removing them is under "For Stevie / housekeeping", after 7 days of the switch working.
- [ ] **Step 328** [W] ~10m: `~/tmp/next.md`, a section at the top: before and after per row, the switch's times, what waits on Stevie (D7, any unanswered decision, 8B if blocked, `zhen-claude`'s `WELL_ZHEN_DSN` in his shell, the rollback housekeeping), the handoffs (Z11 to plan 4; east's Akira to plan 6; Zhen into an LXD instance, plan 7), any `[STUCK]`, the push count.
- [ ] **Step 329** [V] ~1m: `git status --short` shows no tracked change; `git log origin/develop..develop --format='%G? %s' | grep -vc '^G'` is 0, or the unsigned commits are listed for `scripts/resign-unpushed.py`.
- [ ] **Step 330** [V] ~1m: **PHASE 10 EXIT GATE**: develop holds the plan, gates green, the record written. Stevie pushes.

---

## Appendix A: Emergency procedures

1. **The switch turned off something it should not have.** `sudo systemctl enable --now unheaded-zhen.target` puts it back. If the platform itself lost a unit, `systemctl list-dependencies --reverse <unit>` shows whether it was wired to the target by mistake: only the units of `modules/zhen/module.yaml` and the oneshot may be.
2. **The oneshot or the target misbehaves (Step 250).** Undo 8A's switch wiring: `sudo systemctl disable --now unheaded-zhen.target unheaded-zhen-module.service; sudo rm -rf /run/unheaded/modules/zhen; sudo cp -p $RB/zhen-agentd.service $RB/unheaded-zhen-ui-front.service /etc/systemd/system/; sudo systemctl daemon-reload; sudo systemctl enable --now zhen-agentd unheaded-zhen-ui-front`. West is then as before 8A for Zhen; the platform timers can stay.
3. **Prometheus or VictoriaMetrics does not come back right (Steps 253, 255).** `sudo docker logs --tail 50 unheaded-prometheus` (or `unheaded-victoria`). A config refused at start: `git revert` (signed) the Phase 4 commit's changes to `monitoring/prometheus/prometheus.yml`, `config/victoria/scrape.yml` and the two compose files on develop, then `up -d --no-deps` the container again; Zhen's jobs and rules are then absent while it runs, which is the off state, not a loss of the platform's. Prometheus keeps its data volume through a recreate.
4. **The Well move goes wrong in 8B.** Before 052 (Steps 273-280): nothing in `unheaded_app` changed. Undo: stop Zhen, then as superuser `DROP DATABASE unheaded_zhen; DROP ROLE well_zhen_owner; ALTER ROLE app_zhen CONNECTION LIMIT -1; ALTER DATABASE unheaded_app RESET unheaded.zhen_rows_copied;` and in `postgres` `DELETE FROM well_schema_migrations WHERE filename = '050_zhen_database.sql'`; put back `$RB/zhen-agentd.env` and the old `/usr/local/bin/zhen-agentd`; start Zhen. After 052: the tables are gone from `unheaded_app` but whole in `unheaded_zhen` (Step 277). Going back means restoring the eight tables from the rollback set into `unheaded_app` (`pg_restore -d unheaded_app -t <table>` from `<set>/unheaded_app.dump`, superuser, triggers as dumped) and deleting 052's ledger row; that is an attended decision for Stevie, never automatic. Forward fixes are preferred after 052.
5. **zhen-agentd will not start after 8B (Step 288).** `journalctl -u zhen-agentd -n 30`. Missing DSN → Step 283. A bind refusal → the drop-in or `ZHEN_METRICS_ADDR` (never printed; compare with `grep -c`). Still down after two tries: the old binary and env from `$RB` with the old schema is not possible after 052, so stop the switch (`disable --now`), which is the platform's normal state, and write `[STUCK]`. The platform does not need Zhen.
6. **`ModuleListenerPublic` pages.** `sudo systemctl disable --now unheaded-zhen.target` first, then read `unheaded_module_listener_public` and `sudo ss -ltnp` for the port. Do not edit the allowlist to silence it: a global address can never be listed.
7. **A gate was red on develop before this plan.** Prove it in a clean clone at `1393e5ee`, write it under "For Stevie", do not fix it here.
8. **A sibling plan changed a shared file first** (Busboy list). Merge, keep both sides; for `pkg/uids` and `db/migrations/MANIFEST`, renumber this plan's entry to the next free one and record the mapping in `$PROG`.

## Appendix B: Agent assignment and dependencies

| Phase | Agent | Model | Parallel | Depends on | Time |
|---|---|---|---|---|---|
| 0 Before readings | Coordinator | Opus | no | — | 1h |
| 1 ADR, manifest, `pkg/modules`, accounts | Coordinator | Opus | no | 0 | 2h |
| 2 Platform schedules | Developer, `-p2` | Sonnet | [P] | 1 | 3.5h |
| 3 Out of the defaults | Developer, `-p3` | Sonnet | [P] | 1 | 3.5h |
| 4 Monitoring to the module | Developer, `-p4` | Sonnet | [P] with 6, 7 | 2, 3 | 3h |
| 5 The switch | Coordinator | Opus | beside 6, 7 | 1, 4 | 3h |
| 6 Never public | Developer, `-p6` | Opus | [P] | 1 | 4h |
| 7 Own database (code) | Developer, `-p7` | Opus | [P]; its Postgres tests after G-EW | 1 | 7h |
| 8A West, unattended | Coordinator, sudo | Opus | no | 1-7, G-EW | 1.5h |
| 8B The Well, attended | Coordinator + Stevie | Opus | no | 8A, D2, D6 | 2h |
| 9 Proof and afters | Coordinator, sudo | Opus | no | 8A (8B or its no) | 1.25h |
| 10 Close | Coordinator | Opus | no | all | 1h |

**Swarm.** After Phase 1: lanes 2, 3, 6 and 7 touch disjoint files except `scripts/check-gates-can-fail.sh` (Phases 4 and 6), `observability/alerting/policy.yaml` (Phases 2, 6, 7), `docker-compose.yml` (Phases 4 and 7), `tests/scripts/test_module_manifest.py` and `modules/zhen/module.yaml` (Phases 5, 6, 7) and `modules/zhen/prometheus/rules/zhen.yml` (Phases 4 and 7): merge those lanes one at a time and keep both sides. At most three of them build at once. Phase 4 starts when 2 and 3 are merged; Phase 5 when 4 is.

**Critical path**: 0 → 1 → 2 and 3 → 4 → 5 → 8A → 8B → 9 → 10, about 17 h of work. Phase 7 (7 h) runs beside 2 to 5 and its Postgres tests wait for G-EW, so after the evidence week the path is G-EW → 7's tests → 8A.

## Appendix C: Findings and quick reference

**Findings** (the rows Step 3 copies into `$PROG`; `before` and `after` are filled from the steps named)

| id | seat and row | evidence | fix | phase | before | after |
|---|---|---|---|---|---|---|
| Z1 | Architect A7, Micromanager D11 | `cmd/akira/main.go:194,197,199`; `configs/akira-west.yaml:6-7` | defaults and the `inference` row gone | 3 | Step 10 | Step 311 |
| Z2 | Developer 1.7 | `config/dashboard/services.txt:22-29`; `health.go:778-781` | module fragment; module targets never decide the overall state | 3 | Steps 11, 15 | Steps 257, 299, 304 |
| Z3 | Developer D23, BlackMage F17 | `configs/zhen-schedules.yaml:27-33` | platform timer `unheaded-health-sweep` | 2 | Steps 11, 12 | Steps 258, 312 |
| Z4 | Micromanager D11, BlackMage F17, Developer 1.7 | `rules/zhen.yml`; `prometheus.yml:110-119`; `scrape.yml:253-257` | rules and scrape jobs in `modules/zhen`, loaded only while on | 4 | Steps 11, 13 | Steps 252, 299, 312 |
| Z5 | Erda F8 | 10 of 13 tables in `unheaded_app` | `unheaded_zhen`, two-step claim | 7, 8B | Step 19 or 268 | Step 282 |
| Z6 | decision 15 (c), BlackMage | no check holds a bind; `-host 0.0.0.0` accepted | bind rule, static gate, guard, page | 6 | Step 14 | Steps 261, 313 |
| Z7 | Micromanager D11, Developer 1.7 | `unheaded-vor.service:2,19` | description corrected (D4 a) | 3 | Step 11 | Step 312 |
| Z8 | Developer 1.7 | `pkg/bff/nav.go:46` | entry only while on | 3 | Step 11 | C8 in Steps 299, 306 |
| Z9 | Developer 1.7 | `zhenoptin.go`; `runbooks.go:474-490` | silent and hidden while off | 3 | Step 15 | Step 304 |
| Z10 | own sweep | `Makefile:76` | `build-zhen`, out of `build-services` | 3 | Step 11 | Step 312 |
| Z11 | Architect A3, Archimedes F5 | 2 GiB huge pages for the model server | handoff to plan 4 (decision 12) | — | none | plan 4 |
| Z12 | Developer 1.7, grounding G8 | IOC audit scheduled nowhere | platform timer, enabled on D7 | 2, 9 | Step 12 | Steps 309, 312 |
| Z13 | grounding G8 | the sweep probes plain http against TLS | targets file, scheme per target, exit code | 2 | Step 16 | Steps 259, 302 |
| Z14 | Erda F8, F6 | `app_zhen` no limit; 7 pools, 21 connections | limit 12; two pools, 8 | 7, 8B | Step 19 or 268 | Step 287 |
| Z15 | Erda F8 | Zhen's rows ride the platform's set and its copy on east | excluded; own set, local, drilled | 7, 8B | Step 17 | Steps 290, 292 |

**New series**: `unheaded_schedule_last_run_timestamp_seconds`, `unheaded_schedule_last_exit_code`, `unheaded_schedule_last_success_timestamp_seconds` (`schedule`); `unheaded_dashboard_health_targets` (`module`); `unheaded_module_enabled`, `unheaded_module_changed_timestamp_seconds` (`module`); `unheaded_module_listener_public` (`module, port, addr`), `unheaded_module_listener_public_total` (`module`), `unheaded_module_listener_guard_last_run_timestamp_seconds`; `unheaded_zhen_backup_*` (the five of `well_backup.prom`).

**Alerts**: platform `HealthSweepNotSucceeding` (ticket), `IOCAuditNotSucceeding` (ticket, now on the platform's series), `ModuleListenerPublic` (page), `ModuleListenerGuardNotRunning` (ticket); module `ZhenSchedulerStalled` (none), `ZhenBackupStale` (ticket).

**Units**: `unheaded-zhen.target` (the switch); `unheaded-zhen-module.service`; `unheaded-zhen-backup.{service,timer}` (in the switch); `unheaded-health-sweep.{service,timer}`, `unheaded-ioc-audit.{service,timer}` (installed disabled until D7), `unheaded-module-guard.{service,timer}` (the platform's). Accounts 16714 `unheaded-health-sweep`, 16715 `unheaded-ioc-audit`, 16716 `unheaded-modules`, 16717 `unheaded-module-guard`.

**Paths**: fragments source `/usr/local/lib/unheaded/modules/zhen/`, live `/run/unheaded/modules/zhen/` (in containers `/etc/unheaded-modules` for Prometheus and VictoriaMetrics, `/run/unheaded/modules` for the dashboard and kanban); schedules `/usr/local/lib/unheaded/schedules/`; sweep targets `/etc/unheaded/health-sweep.targets`; textfile `/var/lib/unheaded/textfile`; Zhen's sets `/srv/backups/unheaded-zhen/`; rollback `/var/backups/zhen-switch-<date>/`.

**Variables**: `UNHEADED_MODULES_DIR`, `UNHEADED_MODULE_BIND_ALLOW`, `WELL_ZHEN_DSN` (Zhen's tables), `WELL_APP_ZHEN_DSN` (the board only), `WELL_ZHEN_DB` (kanban, retention), `WELL_BACKUP_DATABASES`, `WELL_BACKUP_EXCLUDE`, `WELL_BACKUP_SET`.

**Numbers taken**: ADR-<N> (148 if free), migrations 050-052, uids 16714-16717.

**Gates**: the Code gates block, `scripts/check-prometheus-rules.sh` (platform and modules), `go run ./cmd/alert-policy` and `-runbooks`, `scripts/check-module-binds.sh`, `scripts/check-unit-{ceilings,binaries,flags}.sh`, `scripts/check-well-migrations.sh`, `scripts/check-runbooks.sh`, `scripts/runbook-manifest.sh --write`, `scripts/well/test-*.sh` (after G-EW), `scripts/test-zhen-switch.sh --expect on|off`, `scripts/ci-local.sh`.

---

*S-ZHEN-SWITCH Battle Plan, forged 2026-11-15; finished the same night from a partial draft (Phases 0-3), re-grounded on develop `1393e5ee`*
*11 Phases. 330 Steps. The platform stands on its own; Zhen comes when it is called and never faces the street.*
