<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-133: Fold version translation into monad, and leave it thin

**Status:** Proposed (parked). No code until Stevie reads this.
**Date:** 2026-11-04
**Related:** DP-203, wire freeze v0x01, TRUE-WIRE C6 and E6.

## The pitch (decision pack, DP-203)

> ... version into monad ...

## What is there

`services/version`: HTTP on 19017, no auth; translation rules, per-service versions, rollout events, statistics nothing feeds. `ebpf/version-ebpf`: the heaviest of the G9 halves, `version_xdp_ingress` 243,780 and `version_tc_egress` 213,386 verified instructions, the latter an `#[xdp]` program whose name says TC. Attached by nothing.

## Verdict: right by name, with nothing yet to do

The Monad service is the natural home for Monad version rules. But **the wire is frozen at v0x01**: only one version exists, so `VERSION_MATRIX` can hold no real row, and any rule that moves a field touches the frozen format. The monad unit has an empty `CapabilityBoundingSet`, so it can never write those maps itself; `/api/v1/stats` already exists in `cmd/monad`, so routes need a prefix.

## Decision direction

Flag `MONAD_FEATURE_VERSION=true`, default off: a rules store under `/api/v1/version/`, no map writes, every rule refused until a second version exists. Spec compliance gate and RFC desk before a rule can be accepted. This fold is cheap and can come last.

## Open questions

1. Worth folding before v0x02 is even a draft?
2. 243,780 instructions for a path that cannot fire: keep it in the load gate?
