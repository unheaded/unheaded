<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-149: Zhen is a module behind one switch

**Status:** Proposed. Implements ADR-147 decision 15. Coded on branch `sprint/s-zhen-switch` (plan `docs/battle-plans/S-ZHEN-SWITCH-battle-plan.md`); decisions D1-D6 of that plan were taken on its recommendations and wait for Stevie's confirmation.
**Date:** 2026-11-15
**Related:** ADR-147 (every service earns its place; decision 15, Stevie's words there), ADR-138 (the Zhen UI's TLS front), ADR-100 (alerts and textfiles), ADR-029 (health consensus), ADR-103 (service identities).

## Context

Measured on west and develop on 2026-11-15 (plan grounding rows G1-G13).

| # | What assumed Zhen |
|---|---|
| G1 | Akira's built-in targets name `zhenai`, `inference`, `gateway`; west's `configs/akira-west.yaml` names `inference` 20100. East's Akira, which runs on the defaults, refuses them every sweep. |
| G2 | `config/dashboard/services.txt` lists `zhenai` and `llama-server`; one unhealthy target turns the whole dashboard `unhealthy`. |
| G3 | Eight `zhen_*` tables live in `unheaded_app` beside the board; the worker claims a card in one transaction across both; seven `app_zhen` pools, 21 connections, no limit. |
| G4 | The nightly Well set dumps every database, so Zhen's rows ride the platform's set and its copy on east. |
| G6, G7 | Zhen's rules and scrape jobs sit in the platform's rule set and scrape files; `ZhenSchedulerStalled` fired for days. |
| G8 | The IOC audit is scheduled nowhere; the health sweep is a job inside `zhen-agentd -scheduler`, paused since 2026-11-12, and it cannot pass (plain http against TLS listeners). |
| G9 | vor's unit calls it "for Zhen"; it is one of the three real applications (ADR-147 item 12). |
| G10, G11 | The navigation bar always links Zhenai; the board always carries the Zhen opt-in and warns when Zhen's runs cannot be read. |
| G12 | No check holds a Zhen listener to loopback; `zhen-agentd -host 0.0.0.0` is accepted, and west's wifi holds global IPv6 addresses. |
| G13 | Zhen has no container: it is host units. A compose profile would switch nothing. |

## Decision

1. **The switch is `unheaded-zhen.target`** (plan D1 a). Every Zhen unit is `WantedBy=` and `PartOf=` it, so the target's state is the module's state. On: `systemctl enable --now unheaded-zhen.target`; off: `disable --now`.
2. **A module's fragments live under `/run/unheaded/modules/<name>/` while its target is active, and nowhere else.** A oneshot in the target (`unheaded-zhen-module.service`, account `unheaded-modules`) copies them from `/usr/local/lib/unheaded/modules/<name>/` on start and removes them on stop. `/run` cannot disagree with the target after a reboot.

   **Note 2026-11-18.** Prometheus v3 (v3.15.1 here) accepts a wildcard only in the last path component of `rule_files` and `scrape_config_files`; with a glob across module directories it refused the whole configuration. So `scripts/modules/module-fragments.sh publish <name>` still writes `/run/unheaded/modules/<name>/` and also copies each `*.yml` directly under the module's `prometheus/rules` and `prometheus/scrape` to `/run/unheaded/modules/_prometheus/{rules,scrape}/<name>.<file>`; `withdraw <name>` removes those copies (commit `85662feb`). `monitoring/prometheus/prometheus.yml` globs the two flat directories, so a module's rules and scrape jobs still load only while its target is active. "Under `<name>/` and nowhere else" holds for everything else: VictoriaMetrics' scrape files, the dashboard's health targets and what `pkg/modules` reads. `_prometheus` cannot be a module: a module's name starts with a letter.
3. **The readers.** Prometheus and VictoriaMetrics read the fragments by glob (`rule_files`, `scrape_config_files`) and re-read on a timer; dashboard-backend re-reads its module health targets every 30 s; the navigation bar and kanban ask per request. Go code asks through one package, `pkg/modules`.
4. **Platform checks run on platform timers.** The health sweep and the IOC audit run from `unheaded-health-sweep.timer` and `unheaded-ioc-audit.timer` through `scripts/monitoring/platform-schedule.sh`, each leaving `unheaded_schedule_*` series. A module may author or drive runbooks; it is never their clock.
5. **A module's data is its own database, backup set and connection limit** (plan D2 a, D6): `unheaded_zhen`, owner `well_zhen_owner`, `app_zhen` limited to 20 connections (every app_zhen client at its cap, arithmetic in migration 051; the plan said 12), its own backup set at `/srv/backups/unheaded-zhen` (local until ADR-147 plan 7), excluded from the platform's set. The board's opt-in table stays in `unheaded_app`; a claim is two steps (run row first, card second, the run cancelled on a miss).
6. **A module's listeners are loopback, a Unix socket, or an allowlisted private address** (plan D5 a), never a wildcard or a globally routable address. Held three ways: the binaries refuse at start (`pkg/modules.CheckBind`, `UNHEADED_MODULE_BIND_ALLOW`), a static gate refuses it in the repository (`scripts/check-module-binds.sh`), and a live guard (`unheaded-module-guard.timer`) raises `ModuleListenerPublic`, which pages.
7. **Permitted, not required.** Allowed-caller lists that name Zhen (`configs/wotan.yaml`, compose, `unheaded-auth.service`), the registries `pkg/ports` and `pkg/uids`, and `unheaded_ops.llm_requests` stay: an entry with no caller raises nothing.
8. **Later (ADR-147 plan 7):** the target's units become an LXD instance. The fragments, the database and the gates stay as they are.

## Alternatives rejected

- A compose profile (D1 b): Zhen is not in compose; a profile switches nothing.
- A key in `/etc/unheaded` every service reads (D1 c): many readers, no single state, and it survives a reboot that the units do not.
- A schema `zhen` inside `unheaded_app` (D2 b): keeps the foreign key and shares the database, its connections and its backup; decision 15 asks for a database.
- Zhen reaches the board only through kanban-app's API (D2 c): the cleaner end state and a larger change; a parked follow-up.

## Consequences

- Flipping the switch needs root (systemd).
- A module's health never decides the platform's overall state: the dashboard lists module targets beside the platform's and leaves them out of its counts.
- With the switch off: no Zhen unit runs, no Zhen scrape job or rule is loaded, no Zhen health target is registered, the menu has no Zhenai entry, and the board shows no Zhen fields. `scripts/test-zhen-switch.sh --expect on|off` reads both states from outside.
- Zhen built from this branch needs `WELL_ZHEN_DSN` (its own tables) beside `WELL_APP_ZHEN_DSN` (the board only).
