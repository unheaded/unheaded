# Game Days

A game day here is one planned, written-down window in which a real fault is injected into a real deployment on west (and east) and the record shows what the platform did about it, with numbers from CSVs, Sword and chaos records, or VictoriaMetrics. Nothing in a results table is estimated: a cell without a source stays empty. Alerts are silenced for the window (ADR-100: no self-made noise) and the blast radius is whatever the chaos allowlist resolves, not what the plan intends ([[Chaos Engineering|Chaos-Engineering]]).

| | |
|---|---|
| Template | `docs/gamedays/TEMPLATE.md`: scope, exact blast radius, abort criteria, scenarios (hypothesis / inject / success / watch), timeline, results, post-state, lessons, action items |
| Records | `docs/gamedays/<YYYY-MM-DD>-<slug>.md`; raw numbers under `docs/gamedays/data/<slug>/` (counts, latencies, timestamps; never payloads) |
| Game day 1 | [`docs/gamedays/2026-10-26-real-steel-1.md`](../docs/gamedays/2026-10-26-real-steel-1.md), data in `docs/gamedays/data/gameday1-2026-10-26/`; S-REAL-STEEL Phase 14 |
| Workload | [[Steel Echo|Steel-Echo]] |
| Deploy | [[Sword|Service-Sword]] through [[Cuirass|Containers]] |

## Game day 1 (2026-10-26), in one line each

| Scenario | Result |
|---|---|
| A canary hurt by real latency, rolled back by Sword (the headline) | run 3: netem 300 ms on the canary's host veth at 23:19:13.853; Sword `rolled_back` / `analysis_failed` "P99 latency 310 ms exceeds 100 ms" (client-observed; histogram said 5 ms); rollback complete 23:20:16.209 (62.4 s); availability over the scenario 0.9985; 2320/2320 for 120 s after. Run 1 had promoted the canary (Sword read only the server histogram: fixed `d90d613d`); run 2 was refused by the concurrency limit while B's kill revert was stuck (fixed `38f27605`) |
| B rolling deploy with a replica killed mid-rollout | Cuirass restarted the killed instance in 1.7 s; rollout `succeeded`; `lxc list` = desired set exactly; availability 0.9829 (FND-023 + rolling deletions) |
| C east: `service_stop` under polkit | timeguru stopped 60 s, east dashboard showed `degraded`, active again at +80 s; SIGKILL: `Restart=` back at +8 s; nothing else on east changed |
| D abort drill | HTTP abort reverted a 10 min netem in 0.12 s, the ABORT file in 0.88 s |
| E packet chaos on the lab netns | +104 ms on flow 0x5EE1 only, 0x5EE2 unchanged; no program on a physical NIC |
| Kingdom SLOs | `slo:error_ratio:5m` wotan / dashboard / well 0 before and after |

Post-state: steel-echo v2 x3, pihole untouched, `c4a0` count 0 on both hosts, journals empty, east unit list equal to the baseline, no lab netns, Cuirass `chaos: false`.

## Window recipe

Every command below is in S-REAL-STEEL Phase 14 and Appendix C; the live details (addresses, keys) come from host env files, never the repo.

1. **Plan**: copy the template, fill scope, blast radius, abort criteria and scenarios; commit before the window.
2. **Silence**: `scripts/monitoring/maintenance.sh 90m "<reason>"`; confirm both silences `active` in Alertmanager.
3. **Stack + override**: `sudo CUIRASS_ENFORCE=true CUIRASS_PRUNE=true CUIRASS_CHAOS=true docker compose -f docker-compose.yml -f docker/compose.cuirass-lxd.yml up -d --build cuirass sword`; the monitoring compose stack up (FND-024) so `slo:error_ratio:5m` exists; chaos-controller active on west (and east for Scenario C).
4. **Pre-flight**: `slo:error_ratio:5m` present for every `page: true` SLO; workload healthy; chaos journals empty on both hosts; pihole `Last Used` noted; east unit list hashed.
5. **Meter** for the whole window: `steel-echo load --discover http://127.0.0.1:19006 --svc steel-echo --rps 20 --out gameday.csv`.
6. **Scenarios**, one at a time (`max_concurrent: 1`), recording fault start, decision and revert timestamps from the Sword and chaos records.
7. **Teardown**: meter off; Cuirass restarted without `CUIRASS_CHAOS`; workload left at its end state or removed the managed way; `maintenance.sh --end`.
8. **Post-state**: `lxc list --project unheaded` = the doc's end state; pihole unchanged; `tc qdisc show | grep -c c4a0` = 0 on each host; journals empty; east unit list = baseline; `bpftool net show dev <nic>` has no yaldabaoth program; no lab netns.
9. **Write-up**: timeline, results, lessons, action items. Every action item becomes an ADR-098 register finding or a Kanban card, never a loose TODO.

Procedure notes from game day 1: `systemctl kill -s SIGKILL` also kills `ExecStopPost`, use `kill -9 $MainPID` for the crash drill; a kill experiment's revert window blocks the next injection; size the canary's `min_sample_size` to the meter.

---

> **Source:** [docs/gamedays](../docs/gamedays) · [S-REAL-STEEL battle plan](../docs/battle-plans/S-REAL-STEEL-battle-plan.md) (Phase 14, Appendices A and C) · [ADR-107](../docs/adr/ADR-107-chaos-safety-envelope.md) · [ADR-100](../docs/adr/ADR-100-sre-alongside-compliance.md)
