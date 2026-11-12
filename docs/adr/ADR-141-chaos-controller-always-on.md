<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-141: The chaos controller runs all the time, slowly, after 1.0.0

**Status:** Proposed (parked). Not before 1.0.0; no plan, no code.
**Date:** 2026-11-12 (pitch of 2026-11-11)
**Related:** ADR-107 (Yaldabaoth inside its envelope: allowlist, three abort paths, revert journal), ADR-029 (Akira consensus and remediation), ADR-100 (SLOs and error budgets), ADR-116 (monitor the monitors), ADR-112 (plan freeze), `wiki/Chaos-Engineering.md`, `wiki/Game-Days.md`.

## The pitch (Stevie, 2026-11-11, verbatim)

> chaos controller should be always running slowly after 1.0.0 it randomly blows things up every n minutes, stalls and reports ----- if our own redundant infra brings it back we also note that and restart chaos controller to keep shutting down random services.

## As understood

A slow, continuous loop once the kingdom is at 1.0.0: pick a random service from an allowlist, take it down, wait, record whether the kingdom's own redundancy and remediation (Akira, Sword, the units' `Restart=`) brought it back and how long that took, or whether it stayed down; then pick the next one. The record is the proof that the redundancy works, taken every day rather than on game days.

## What is there

- `cmd/chaos-controller` (19012, host unit `unheaded-chaos` on west and east): experiments run only inside the ADR-107 envelope: an allowlist (`/etc/unheaded/chaos/allowlist.yaml`), three abort paths including `/run/unheaded/chaos/ABORT`, a revert journal and `ExecStopPost revert-all`, injectors for netem, container kill, systemd stop and kill (east, polkit) and `packet_chaos`. Game day 1 (`docs/gamedays/2026-10-26-real-steel-1.md`): a canary hurt by 300 ms of netem rolled back on its own in 62.4 s.
- Schedules exist (`chaos.schedules` on Wotan, `gameday.go`), driven by a person. Nothing runs unattended today.
- Akira publishes signed `system.health.consensus` and `system.health.remediate`; remediation is off unless Akira runs with `--remediate`.

## To settle in this ADR before any plan

1. **The envelope stays.** The loop is a schedule inside ADR-107, never a new path around the allowlist, the abort paths or the journal.
2. **Never a target:** the Well, the chaos controller and its abort path, the alert path (Prometheus, Alertmanager, alert-mailer, `AlertPipelineHeartbeat`, the west/east heartbeat), the login issuer, Wotan while it is the only bus, and anything a game day has not already taken down once by hand.
3. **"n":** the interval, and a floor on it; the loop must never start the next fault while the last has not reverted or been recorded.
4. **The record:** one row per fault in the Well (`ops`), with the service, the fault, who brought it back (Akira, a unit's restart, Sword, nobody) and the time to recovery; a dashboard panel and, later, an SLO on recovery time.
5. **When something does not come back:** stop the loop, revert, alert (page urgency), and stay stopped until a person restarts it. The pitch's "restart chaos controller to keep shutting down" applies only after a recovery was recorded, never after a failure.
6. **Budget:** every fault spends error budget (ADR-100); the loop pauses when a budget is below its threshold.
7. **Hosts:** west and east take turns, or east only, while west holds the Well and the alert path.

## Decision

None. Parked under ADR-112; opens after 1.0.0.
