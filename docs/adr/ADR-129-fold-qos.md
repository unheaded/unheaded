<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-129: Fold the QoS plane into Cuirass, without its 10 Hz loop

**Status:** Proposed (parked). No code until Stevie reads this.
**Date:** 2026-11-04
**Related:** DP-203, ADR-128 (the node writer), ADR-108 (cartridges), TRUE-WIRE C4 and E4.

## The pitch (decision pack, DP-203)

> ... qos ... into cuirass.

## What is there

`services/qos`: HTTP on 19013, no auth; per-service policies with CoDel parameters, `/api/v1/congestion`, and a loop that publishes `qos.statistics` to Wotan **ten times a second** once it has stats. `UpdateStats` has no caller, so today it finds none and stays silent. No BPF map is touched. `ebpf/qos-ebpf`: `qos_xdp`, 55,684 verified instructions, attached by nothing; it is also the cartridge rate limiter's parity reference (`pkg/cartridge/ratelimit_linux_test.go`).

## Verdict: right, with one part removed

Per-service policy is desired state: Cuirass is the right home. The 10 Hz publisher is not: it reports node queues, which Cuirass cannot see, and fed for real it is ten Wotan messages a second from the control plane. It stays out until a node source exists.

## Decision direction

Flag `CUIRASS_FEATURE_QOS=true`, default off: policy routes under `/api/v1/qos/`. No aggregation loop. Map writes wait on ADR-128's node writer. `qos-ebpf` must keep its parity role untouched.

## Open questions

1. With cartridges able to rate-limit per flow (ADR-108), is `qos-ebpf` the enforcement path or only the reference?
