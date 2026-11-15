<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# S-WASTE-OUT progress

Plan: `docs/battle-plans/S-WASTE-OUT-battle-plan.md` (24 phases, 414 steps). Branch `sprint/s-waste-out`, worktree `/home/govan/tmp/wt/S-WASTE-OUT`, off develop `8d35785d`. Executed unattended under the ADR-147 lane brief (`~/tmp/adr147-table/LANE-BRIEF.md`): Phases 0 to 16 only; Phase 17 (the E-GATE) and later wait.

## Decisions

Stevie was not asked (lane brief: "no questions for now"). Each ruling that only shapes code on this branch is taken on the plan's recommendation, Stevie to confirm. Anything live, irreversible, or about keys or the Well is under "For Stevie".

| # | Question | Ruling | Basis |
|---|---|---|---|
| D1 | Who publishes `alerts.critical` | A: alert-mailer, firing and resolved, plus a 5 min snapshot | taken on the plan's recommendation, Stevie to confirm. The signing key (enrolment) is his: For Stevie |
| D2 | What a subscriber does | B: record, and hold change (Sword, chaos-controller, zhen-agentd) while one fires; unknown allows with a warn | taken on the plan's recommendation, Stevie to confirm |
| D3 | Who reads service discovery | A: Akira, admitted by sender, subnet and count | taken on the plan's recommendation, Stevie to confirm |
| D4 | Prometheus retention | A: keep 30 days of only the series a rule reads; a generated keep-list and a gate | taken on the plan's recommendation, Stevie to confirm |
| D5 | Akira's certificate check | A: `interval: 1h` | taken on the plan's recommendation, Stevie to confirm |
| D6 | `FLOW_EVENTS` | A: flow-tracker stops writing it; the ring goes | taken on the plan's recommendation, Stevie to confirm |
| D7 | `FLOW_STATE` | A: LRU with a counted insert (decision 19) | taken on the plan's recommendation (the plan's own default), Stevie to confirm |
| D8 | Postgres's budget for the fair week | not taken | live (Phase 22, after the E-GATE): For Stevie |

## Findings

Before readings of 2026-11-15T21:41Z (VictoriaMetrics on `127.0.0.1:8428`, cgroup files, `bpftool` reads; nothing restarted or written). After readings are Phase 20, after the E-GATE: every "after" below is pending until then.

| id | seat and row | evidence | fix | owner | before | after | status |
|---|---|---|---|---|---|---|---|
| W-01 | Developer D1, D2, D5; BlackMage F4 | 68.8 GET/s on Wotan; a fallen-back stream polls for ever; two recovery paths | Phases 6, 7 | this plan | 21:41Z: 68.86 GET/s, 3.15 POST/s on Wotan (30 m rate) | pending (Steps 359-361, bar ≤ 1/s) | |
| W-02 | Developer D18 | `grpcOK: true` by default (`pkg/transport/health.go:36`) | Step 95 | this plan | `grep -c 'grpcOK: *true' pkg/transport/health.go` → 1 | | |
| W-03 | Developer D6 | 63 approved members for 14 processes | Phase 7 | this plan | 21:41Z: 63 approved members, 25 rooms, 17 gRPC streams | pending (Step 359) | |
| W-04 | Developer D3 | 12 discovery polls (24 GET/s); 0 readers | Phases 7, 9 | this plan | `discovery.Start(` call sites outside tests: 12; `discovery.(Watch\|NewResolver)` in cmd, services: 0 | pending (Step 359) | |
| W-05 | Developer D4 | 6 subscribers, no publisher, no room | Phase 8; keys Step 317 | this plan; key Stevie | 21:41Z: no `alerts.critical` room | pending (Step 359) | |
| W-06 | Developer D7; Archimedes F6 | six trace rooms, 39.3 of 52.8 MB | Phase 10 | this plan | 21:41Z: six mirrored trace rooms 37.49 MB (`traces.stats` 1.67 MB more) of 52.15 MB | pending (Step 359) | |
| W-07 | BlackMage F5 | 367 answers 429 in 7 d | Phase 11 | this plan | 21:41Z: 367 answers 429 in 7 d | pending (Step 361) | |
| W-08 | BlackMage F11 | `_, _ = a.w.Write(data)` (`pkg/auth/audit.go:101`) | Phase 11 | this plan | `grep -c '_, _ = a.w.Write' pkg/auth/audit.go` → 1 | pending (Step 359, series present) | |
| W-09 | Developer D19, D21; decision 7 | architect, captain, micromanager: empty data, probes only | Phases 2-4, 19 | this plan | 21:41Z: RSS 15.5, 14.8, 14.0 MB; 0.966 health and scrape requests a second each; 0 log lines | pending (Step 356) | |
| W-10 | Developer D20; decision 7 | monad demo handlers; sophia's empty loop, 60 log lines a minute | Phases 2-4, 19 | this plan | 21:41Z: RSS monad 14.5, sophia 14.5 MB; 0.966 req/s each; sophia 58.0 log lines a minute | pending (Step 356) | |
| W-11 | Developer D17; decisions 7, 14 | Cuirass reconciles a mock; `cuirass.*` rooms nobody reads | Phases 5, 19 | this plan | 21:41Z: rooms `cuirass.drift` 2.36 MB, `cuirass.metrics` 0.41 MB; Cuirass 10.0 log lines a minute | pending (Step 356) | |
| W-12 | Erda F4; decisions 9, 10 | Prometheus 13,730 series, 801 MB, 30 d, beside VictoriaMetrics | Phase 12 | this plan | 21:41Z: 13,829 head series, blocks 770.6 MB, RSS 171.3 MB | pending (Step 361) | |
| W-13 | Developer D11 | dashboard 289 MiB, 13.2 MB/s allocated, a private series store | Phase 13 | this plan | 21:41Z: RSS 298.9 MB (285 MiB); allocation 2.23 MB/s and CPU 0.008 core (6 h rates); Step 208 adds bytes per series | pending (Step 361) | |
| W-14 | Developer D12, D14 | three health pollers; host metrics stored three ways | Phase 13 | this plan | 21:41Z: `/health` 6.46 requests a second across all services (1 h) | pending (Step 361) | |
| W-15 | Developer D13 | 98% of dashboard requests are 401 from a polling page | Phase 13 | this plan | 21:41Z, 7 d: 197,379 answers 401 of 351,818 (56%); 154,408 answered 200 | pending (Step 361) | |
| W-16 | Developer D15 | 241 log lines a minute on west, mostly probes and refusals | Phases 13, 14 | this plan | 21:41Z, 1 h: 247.2 lines a minute; dashboard 113.6, sophia 58.0, kanban 28.0, baseline 11.9, cuirass 10.0, sudo 6.5 | pending (Step 361) | |
| W-17 | Developer D8; Archimedes F4 | Akira throttled 44%; a Python start every 30 s | Phase 14 | this plan | 21:41Z cgroup: 341,683 of 768,754 periods throttled (44.4%); memory `max` 27,770 | pending (Step 361) | |
| W-18 | Developer D9 | 18 zombies in Akira | Phase 14 | this plan | 21:41Z: 0 zombies in `docker top` (seat read 18) | pending (Step 359) | |
| W-19 | Developer D10 | consensus and remediation refused 49 and 12 times | Step 317 (Stevie's keys) | Stevie | 21:41Z: `akira_signed_publish_refused_total` consensus 51, remediate 13 | pending | |
| W-20 | Erda F3; Developer D16 | 0.68 health upserts a second, 73% of WAL flushes | Phase 14 | this plan | 21:41Z approximate: `huginn_pg_commits_total` well-ops 2.07/s (6 h; includes Huginn's own polls); exact figure Step 309 after the E-GATE | pending (Step 360) | |
| W-21 | Developer D22, 1.6 | GOMAXPROCS unexported; six containers without `GOMEMLIMIT` after the removals | Phase 14 | this plan | compose: 11 services with a build and a memory limit lack `GOMEMLIMIT` (timeguru, captain, architect, micromanager, akira, monad, sophia, kanban-app, cuirass, auth, sword); `go_sched_gomaxprocs_threads` absent | | |
| W-22 | Archimedes F4 | `healthprobe` every 10 s on 14 containers | Phase 14 | this plan | `grep -c 'bin/healthprobe' docker-compose.yml` → 14, each `interval: 10s`, `retries: 5` | pending (Step 361) | |
| W-23 | BlackMage F7; decision 19 | `FLOW_STATE` hash, full on east, ~30 h on west, failures ignored | Phase 15 | this plan | 21:41Z west: `FLOW_STATE` type hash, max 65,536, 24,473 entries; installed objects of 2026-10-18 | pending (Step 344) | |
| W-24 | BlackMage F8 | `FLOW_EVENTS` undrained, 94% dropped | Phase 15 (D6) | this plan | 21:41Z west `FLOW_STATS`: key 7 (sent) 2,340, key 8 (dropped) 50,101 (95.5% dropped) | pending (Step 344) | |
| W-25 | Erda F9 | timeguru on tmpfs SQLite; the Well's table empty, wrong shape | Phase 16 | this plan | `docker-compose.yml:840 DB_PATH: "/data/timeguru.db"`; Step 310 after the E-GATE | pending (Step 359) | |
| W-26 | Erda F10; decision 24 | kanban's SQLite fallback of 2026-10-28 | Phases 16, 22 | this plan | `grep -c 'openSQLiteStore(' cmd/kanban-app/main.go` → 2; Step 310 after the E-GATE | pending (Step 360) | |
| W-27 | Archimedes F7 | rooms × room size allows 78 GiB | none: `-buffer-bytes-total` 64 MiB bounds it (`services/wotan/cmd/wotan/main.go:485`) | — | — | — | closed: bounded already |
| W-28 | Erda F1, Archimedes F3 | Vector inserts every second: 47,386 inserts, 42 GiB merged, 71 GB written a day | Phase 21 | this plan | Step 311, after the E-GATE (21:41Z series only: Vector's ClickHouse sink sent 4.24 events a second, 6 h) | pending | |
| W-29 | Erda F12 | Vector's `host` = container id; no series limit | Phase 21 | this plan | Step 312, after the E-GATE (21:41Z: Vector's sink series carry `host=2be6c1c1b20f`) | pending | |
| W-30 | Erda F2; Archimedes F3; decision 5 | Postgres 256 MiB: limit hit 52,740 times, cache hit 0.683 | Phase 22 (D8) | this plan; D8 Stevie | Step 313, after the E-GATE (21:41Z: memory `max` 81,873; well-ops cache hit 0.610) | pending | |
| W-31 | Erda F16; decision 24 | `unheaded-well-data`, three `_0` tables, three empty volumes, `kanban.db` | Phases 19, 22 | this plan | Step 314, after the E-GATE | pending | |

## Log

- 2026-11-15T21:40Z Step 2: worktree `/home/govan/tmp/wt/S-WASTE-OUT`, branch `sprint/s-waste-out` off develop `8d35785d`; scratch `/home/govan/tmp/wt/S-WASTE-OUT-scratch`.
- Step 6: go 1.27.4, cargo, docker (Compose v5.4.0), bpftool, python3, curl, git: all present.
- Step 7: **E-GATE open.** `S-E1-LIVE-progress.md` has no Step 284 entry saying the week was read; the week runs to 2026-11-17 18:34Z.
- Steps 8-13: befores read at 21:41Z (Findings, "before"). Step 11 and 12 read cgroup files under `/sys` and BPF maps with `sudo` reads only (`docker inspect -f '{{.Id}}'`, `docker top`, `bpftool map show/dump`); nothing changed.
- Steps 15-16, seat figures that did not reproduce within a factor of 2:
  - W-13: dashboard allocation 2.23 MB/s (seat 13.2 MB/s) and CPU 0.008 core (seat 2.6%) over 6 h; RSS reproduces (285 vs 289 MiB).
  - W-15: 401 share 56% over 7 d (seat 98%); 154,408 answers 200 suggest a logged-in tab for part of the week.
  - W-18: 0 zombies in Akira (seat 18). The container's process table shows only `akira`; the exec checks' orphans may have been reaped since, or the seat read a different moment. `init: true` (Step 241) is still taken: it costs nothing and closes the cause.
  - W-30: Postgres memory `max` 81,873 (seat 52,740): it has risen, not fallen.
  - Reproduced: Wotan GET 68.86/s, members 63, rooms 25, streams 17, 429s 367; Prometheus 13,829 series; Akira throttling 44.4%; role services 14-15.5 MB each and 0.966 req/s; log lines 247/min; `/health` 6.46/s.
- Step 13: the Vector request counter Phase 21 reads is `vector_http_client_requests_sent_total` (present for `component_id="clickhouse"`).
- Phase 2 (Steps 22-32): archive `/home/govan/tmp/projects/unheaded-role-services`, HEAD `f82e827` (README, signed), source commit `8d35785d83683d1d5d9e7db5a9b4b38bab942cd6`, 236 commits, 112 files (N), no remote. Its history is unsigned (filter-branch).
- Phase 3 (Steps 33-58), commits `7154b791`, `3162eaa0` and the Steps 50-53 commit. 112 files deleted (Step 33). Found while grounding and fixed beyond the plan's list (the plan's rule "no file that builds, starts, scrapes, probes or allows" them): `nix/flake.nix`, `nixos/modules/services/default.nix`, `nix/packages/{architect,captain,micromanager}.nix` (deleted), `scripts/build-binaries.sh`, `scripts/build.sh`, `scripts/lib/deb-packages.sh`, `scripts/deploy-alpha.sh`, `scripts/kingdom-shutdown.sh`, `scripts/three-crowns/*`, `lxd/hosts/host-a/launch-all.sh`, `scripts/bare-metal/validate-host-{a,b}.sh`, `config/supervisor/all-in-one.ini`, `compliance/catalog/controls.yaml` (UH-AUTHN-01 host-service refs), `cmd/crypto-inventory` (the sophia HKDF own-primitive row and fixture; CBOM regenerated), `runbooks/` (14 files; manifest rewritten), `services/wotan/cmd/wotan/allowlist_test.go`, two crosswalk thresholds (12 → 7 west host-service refs).
- Step 40: no compile error, but `cmd/upc-cartridgectl`'s Sophia HTTP registry was a client of the removed service: the backend and its two tests are deleted, `--sophia` is refused by name (`TestSophiaFlagRefused`), man page regenerated.
- Step 37, `config/logging/sources.yaml`: west's compose entries and `unit/unheaded-{monad,sophia}` are deleted. **Deviation:** `east/unheaded-{architect,captain,micromanager}` stay, with a note: east still runs those units until plan 6 removes them; a source inventory that drops a running unit would be false. They are the only G2 hits left in a non-Markdown file, with `nix/yggdrasil/overlay/patches/*` ("by unheaded-architect": the skill, not the service) and `scripts/firewall/setup-ipfire.sh` (`unheaded-monad.nft`: the Monad wire format's HbH passthrough, not the service).
- Step 50, G2 ran on non-Markdown files only; Markdown under `docker/`, `deploy/`, `nix/` is Phase 4's.
- Step 52: `check-prometheus-rules.sh` **not run** (it starts a container: lane brief, evidence week); `wotan_test.yml`'s caller fixture changed from sophia to timeguru, unchecked by promtool until after the E-GATE. PASS: compose-hardening, compose-publish-bind, compose-log-caps, compose-bind-nesting, image-digests, unit-ceilings, check-service-auth, log-sources, crypto-inventory, version-flag (57 binaries), gen-man (one page rewritten).
- Steps 54-57: `check-gates-can-fail --only check-systemd-units` PASS on `unheaded-timeguru.service`; gosec ratchet PASS; `check-sbom-trend` OK (python 71, baseline 72: not this plan's; baseline not lowered).
- Phase 3 exit gate: build, vet, the listed tests (plus `pkg/logsources`, `cmd/crypto-inventory`, `services/wotan`) green under `-race`; G1 and G3 empty; G2 as above.
- Phase 4 (Steps 59-68), commit `f1447210`: 27 live documents (`doc-list.txt`) edited; dated records untouched. No `check-wiki*`/`check-doc*`/`check-links*` gate exists in `scripts/`; `check-spdx` OK. `CLAUDE.md:790` ("Monad (port 19004) and Sophia (port 19005) services created") is a dated history line and stays (Step 62). `docs/legal/scancode-full.json` names the three wiki pages: generated, not edited.
- Phase 5 (Steps 69-80): `TestMockBackendMustBeAskedFor` red (3 subtests) then green; `LXD_BACKEND` has no default, unset refuses with `LXD_BACKEND is not set`, `mock` warns once. `/api/v1/info` reports `"lxd_backend":"unset"` (was "mock") for an unset backend (`observability_test.go` changed with it). Cuirass is behind `profiles: ["lxd"]`; off Akira's list, the dashboard's list, both scrape files' west target, the scripts and the crosswalk's west refs (thresholds 7 → 6; `UH-AUTHN-01` loses `west:cuirass`). `check-prometheus-rules.sh` not run (container). CBOM regenerated (compose line numbers).
- Lanes: the plan's lanes (`-docs`, `-wotan`, `-mon`, `-dash`, `-ebpf`) exist for parallel agents. This lane is one executor and the brief forbids spawning agents, so their phases run in order in `WT` on `sprint/s-waste-out` itself; there is nothing to merge back.

## Handoffs

- **Plan 7 (S-TENANT-DOOR), Cuirass:** start it with `sudo docker compose -f docker-compose.yml -f docker/compose.cuirass-lxd.yml --profile lxd up -d cuirass`. `CUIRASS_ENFORCE` and `CUIRASS_PRUNE` stay unset until a tenant exists. The volume `cuirass-state` holds a stale desired state (three `steel-echo-v2-*` containers) to clear before the first enforce. Sword's window needs `--profile deploy --profile lxd`.
- **Plan 6 (S-EAST-PAIR), Cuirass on east:** `LXD_BACKEND` has no default any more (Phase 5). East's `unheaded-daemon` host unit sets none in the repository; a new binary there refuses to start until its environment names `https`, `unix` or `mock`. `scripts/install-chaos-west.sh` still points chaos-controller at `http://127.0.0.1:19006` (unchanged: it is the client's address for when Cuirass runs).
- **Plan 6, the role units on east:** `config/logging/sources.yaml` keeps `east/unheaded-{architect,captain,micromanager}` until those units are removed there; delete the three entries with them.

## For Stevie

1. D1 to D8 were not asked (lane brief). D1-D7 are taken on the plan's recommendation (table "Decisions"); confirm or overturn. D8 (Postgres's budget, Phase 22) waits for you.
2. `unheaded-role-services` (`~/tmp/projects`) has no remote; create one and push if you want it public.
3. `runbooks/MANIFEST.sha256` was rewritten (Step 44: 14 runbooks lost the five); it needs your seal again.
4. Default without a question (plan): health probes every 30 s with 3 retries, so a dead service is `unhealthy` after 90 s where it is 50 s today (Phase 14).

## Blocked
