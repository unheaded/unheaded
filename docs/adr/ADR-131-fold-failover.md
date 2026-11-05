<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-131: Fold failover (services/selfheal) into Cuirass, deciding nothing Akira decides

**Status:** Proposed (parked). No code until Stevie reads this.
**Date:** 2026-11-04
**Related:** DP-203, ADR-029 (health consensus), ADR-128, TRUE-WIRE C5 and E5.

## The pitch (decision pack, DP-203)

> ... failover (services/selfheal) ... into cuirass.

## What is there

`services/selfheal`: HTTP on 19014, under the constant `ports.BackupMgr`; no auth. Endpoint health scores, a circuit-breaker state machine, backup rosters, manual and automatic failover. **No probe:** `evaluateEndpoint` scores fields nothing sets, so no real endpoint ever fails. Failover only writes events to a slice. `ebpf/failover-ebpf`: `failover_xdp`, 29,125 verified instructions, its own scoring in the kernel, attached by nothing.

## Verdict: right destination, half the code should not come

Cuirass already probes instance health and owns the proxy device that gives a stable endpoint, so it is the right actuator. But **deciding** something is unhealthy is Akira's job under ADR-029: percentage consensus, advisory `system.health.remediate`, three restarts then escalate. Two brains deciding one outage is how failovers fight. hop-ebpf's `CIRCUIT_ERRORS` is a third circuit-breaker view.

## Decision direction

Flag `CUIRASS_FEATURE_FAILOVER=true`, default off. Consume `system.health.remediate`, act through the proxy device; rosters under `/api/v1/failover/`. Do not bring `healthMonitorLoop` or the scoring: Akira and Cuirass's own probes already exist. Rename the port constant only if a listener survives; it should not.

## Open questions

1. Which circuit breaker is the truth: `failover-ebpf`'s, hop-ebpf's `CIRCUIT_ERRORS`, or Akira's severity band?
2. Does kernel-side failover (XDP_TX to a backup) ever run ahead of consensus?
