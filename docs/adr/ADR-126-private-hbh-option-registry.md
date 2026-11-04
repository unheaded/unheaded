<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-126: A private Hop-by-Hop option registry

**Status:** Proposed (parked). No code. Stevie accepts it.
**Date:** 2026-11-04
**Related:** DP-412, DP-606, DP-209, foundation-06 "IPv6 Hop-by-Hop Option Type", `docs/protocol/RFC-EDITOR-REQUESTS-WH.md` item 5.

## The pitch (decision pack, DP-412)

> Private HbH option registry in an ADR now, IANA with the resubmission.

## Context

The pack calls 0x3E "zhend's". It is also the Monad's. `ebpf/monad-common/src/lib.rs:56` has `MONAD_OPT_TYPE = 0x3E`; `crates/zhend/src/monad/hbh.rs:23` has `ZHEN_OPTION_TYPE = 0x3E`, marked "placeholder". Only the option length tells them apart: Monad parsers want exactly 20, `parse_zhen_option` wants at least 32. Today that is harmless. zhend is single-node and gRPC-only (DP-209), so its HbH path never runs on a live packet. A third option with a length of 20 or 32 would be misread.

0x3E is one of the eight values RFC 4727 sets aside for experiments (0x1E, 0x3E, 0x5E, 0x7E, 0x9E, 0xBE, 0xDE, 0xFE), so IANA will not assign it. It is correct for a limited domain and wrong as the value foundation-06 suggests.

## Decision

Keep this table as the registry. A new option gets a row before any code uses it. A value is never reused. Two rows never share a value.

| Value | act | chg | Data length | Owner | Status |
|---|---|---|---|---|---|
| 0x3E | 00 skip | 1 mutable | 20 exactly | Monad, foundation-06 | In use; IANA value requested at the resubmission (DP-606) |
| 0x1E | 00 skip | 0 immutable | 32 or more | Zhen fragment advertisement | Reserved here; `hbh.rs` still says 0x3E until Stevie accepts this |

0x1E is the RFC 4727 experimental value with the same skip behaviour. It needs Zhen's option to be immutable en route: the file comment says "Mutable: YES", but nothing rewrites it and it carries a hash of the fragment.

## Consequences

When accepted: one constant in `hbh.rs` and its tests, plus a note in `crates/zhend`. No wire change: Zhen's option has never been on a wire. The IANA ask goes with the resubmission, as answered.

## Open questions

1. Does Zhen want a real IANA value, or stay private to the limited domain?
2. Any other option on the roadmap that should reserve a row now?
