<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-120: Estimated energy per LLM request, recorded in the Well

**Status:** Proposed (Stevie, 2026-11-04: "let's record to the well", "estimated power consumption"). Carried by S-ZHEN-POLISH.
**Date:** 2026-11-04
**Related:** ADR-104 (Claude bridge), ADR-106 (two brains), migration 030 (`unheaded_ops.llm_requests`), `pkg/claudebridge/audit_sinks.go`.

## Why

The local-first design exists partly to reduce dependence on commercial models and to burn less water and electricity. That second claim should be measured, not assumed. A small model on a consumer GPU avoids a data-centre round trip, but large inference clusters are efficient per token, so local is not automatically the lower-energy choice.

## What exists

- `llm_requests` already records every call: backend (`claude` or `local`), model, task family, route reason, tokens, cost and `duration_ms`.
- West's RX 7700 XT exposes `power1_average` in microwatts under `/sys/class/drm/card1/device/hwmon/hwmon4`; it has no `energy1_input` counter, so energy must be integrated from samples.
- The CPU package exposes `/sys/class/powercap/intel-rapl:0/energy_uj`, readable by root only.

## Decision direction

Add to `llm_requests`, in a new migration 044 (030 stays as it is):

- `energy_joules_est DOUBLE PRECISION` (NULL means no estimate; never 0 for "unknown");
- `energy_method TEXT` with a CHECK: `''`, `gpu_sampled`, `gpu_cpu_sampled`, `published_figure`.

**Local requests.** A sampler reads GPU power about every 250 ms while the request runs and integrates it, subtracting the idle baseline measured just before. The row records the estimate and its method. When the GPU lease is shared, the figure covers the whole card, so concurrent work inflates it; the sampler notes that in `energy_method` by not claiming a per-request exact value. CPU energy joins only through a small root-owned helper or a unit credential, never by widening `energy_uj` permissions.

**Claude requests.** Anthropic publishes no per-request energy figure that this ADR relies on, so the value stays NULL. If a defensible public joules-per-token figure appears, it goes in configuration with its source, and rows carry `published_figure`. No invented number enters the table.

**Water.** Derived later from energy with a stated factor at query time, never stored as a measurement.

Reporting: a view by backend, model and task family giving joules per answer beside quality from the routing evals, so a family moves local on evidence.

## Honest limits

These are estimates: sampling misses short spikes, wall power is higher than the sensor reports, and the local and Claude columns will not be comparable until a real Claude figure exists. The dashboard labels them "estimated".

## Open questions

1. The idle baseline: per request, or a rolling figure.
2. Whether embeddings calls are recorded the same way.
3. Stevie, 2026-11-04: energy may become a routing input, offloading to Claude when that is better for the environment. It cannot decide anything until a defensible Claude figure exists; until then routing stays on quality.

## Decision

Build as above in S-ZHEN-POLISH; the migration applies live at E1 with the rest of the batch.
