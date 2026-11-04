<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-132: Fold the load balancer plane (xdpsync) into Cuirass

**Status:** Proposed (parked). No code until Stevie reads this.
**Date:** 2026-11-04
**Related:** DP-203, ADR-128, TRUE-WIRE C8.

## The pitch (decision pack, DP-203)

> ... loadbalancer into cuirass.

## What is there

`services/loadbalancer`: the one plane with real map code. `xdpsync.go` builds a Maglev table and `SyncToBPF` writes it through `ebpf.LoadPinnedMap` (cilium/ebpf), falling back quietly when the pin is absent. HTTP on 19110, reached by the arithmetic `ports.Firewall + 100`, with no constant of its own; no auth. `ebpf/maglev-ebpf`: `maglev_xdp`, 33,085 verified instructions, attached by nothing.

## Verdict: right, the cleanest of the eight

Backends are exactly what Cuirass knows: it runs the instances and owns the proxy device. The table computation is pure and belongs beside them.

## Decision direction

Flag `CUIRASS_FEATURE_LOADBALANCER=true`, default off: backends and distribution under `/api/v1/lb/`, and the table computed in Cuirass. The map write leaves Cuirass, which has no `CAP_BPF`: the table goes to ADR-128's node writer. If that writer is the Go one, `pkg/ebpf.PinnedMap` (DP-211) replaces cilium/ebpf here; a Rust loader would use aya instead. Either way 19110 stops existing.

## Open questions

1. The node writer, ADR-128.
2. Does Maglev serve anything before there is more than one backend per service?
