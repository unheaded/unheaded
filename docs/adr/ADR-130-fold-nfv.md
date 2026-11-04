<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-130: Fold the NFV plane: where, is a question for Stevie

**Status:** Proposed (parked). No code until Stevie reads this.
**Date:** 2026-11-04
**Related:** DP-203, ADR-108 (MBC cartridges), ADR-128, `docs/protocol/RFC-EDITOR-REQUESTS-WH.md`, TRUE-WIRE C3 and E3.

## The pitch (decision pack, DP-203)

> ... nfv ... into cuirass.

## What is there

`services/nfv`: HTTP on 19016, no auth; up to 64 chains of 10 functions. `POST /api/v1/functions/reload` takes **`prog_fd` as a JSON integer**, stores it, sleeps 200 ms and answers "reloaded". A file descriptor number means nothing across processes, and no program is swapped. `ebpf/nfv-ebpf`: `nfv_xdp` and four tail-called XDP functions (firewall, nat, rate limiter, LB route), attached by nothing.

## Verdict: Cuirass alone does not fit; the alternative is not a clean fit either

1. **The job is loading programs into a tail-call array.** That needs `CAP_BPF`. Cuirass has none and should not get it (ADR-128).
2. **The reload API is unsafe as written.** It must not move anywhere unchanged. An fd from a client is not a capability.
3. **A wire conflict.** `nfv-ebpf` carries `chain_id` in `circuit_state` bits 5-0 and an auth level in bits 7-6. foundation-06 defines `circuit_state` as an exponent-encoded field, and hop-ebpf writes it (`cs::OPEN`). That is a frozen-field overload: it needs the RFC desk before any code.

`cmd/upc-cartridgectl` looks close and is not the same thing. It loads signed MBC programs into one eBPF interpreter (`cartridge-ebpf`); it does not load eBPF. NFV's functions are tail-called XDP. It is ORPHAN, with no unit, and interpreted cartridges miss two of ADR-108's benchmark bars (rate limiter +193 ns against 150, sampler +216 ns against 120). What it has that NFV lacks is a signature check and a registry.

## Options

- **A. Definitions only in Cuirass, as asked.** `CUIRASS_FEATURE_NFV=true`, `/api/v1/nfv/`, `prog_fd` deleted. Chains are data; loading is the node writer's or nobody's.
- **B. Rewrite the functions as cartridges.** One signed path, but a different mechanism and slower.
- **C. A signed loader for XDP objects**, borrowing cartridgectl's Gungnir check. New work.

Nothing moves until Stevie has chosen. In all three, `prog_fd` goes.

## Open questions

1. Is NFV a separate thing from cartridges at all?
2. Where does a chain id live on a frozen wire?
