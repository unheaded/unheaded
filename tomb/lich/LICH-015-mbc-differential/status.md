<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# LICH-015 status

| Field | Value |
|---|---|
| State | ACTIVE (opened 2026-10-23, S-LICH-AWAKENS Phase 7) |
| Oracle self-test | PASS (injected bit-flip detected; equal program clean) |
| Last run | 2026-10-23, west, 50,000 programs, seed `0xc3c35eed` |
| Divergences (cart-allowlist set) | 0 |
| Known out-of-allowlist gap | MULHU 0x3A — reference interpreter NOP vs engine unsigned-high-word (EMULATOR GAP, shipping engine correct; not filed against shipping code) |
| eBPF full-engine arm | deferred to east (ADR-062 LICH-005 lesson) |
| Staged findings | none against shipping code this session (MULHU is an emulator gap, handed to the UPC sprint) |
| Re-run trigger | any change to monad-mbc, monad-cpu-ebpf, or monad_common::cart |

## Log

- 2026-10-23: campaign scaffolded; `diff_harness` built; self-test PASS; 50k-program
  campaign CLEAN; MULHU emulator gap documented and handed to the UPC sprint
  (ADR-067/080) + Computermancer. No divergence attributable to a shipping-engine bug.
