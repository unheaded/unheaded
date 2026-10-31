<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# LICH-015 status

| Field | Value |
|---|---|
| State | ACTIVE (opened 2026-10-23, S-LICH-AWAKENS Phase 7) |
| Oracle self-test | PASS (injected bit-flip detected; equal program clean) |
| Last run | 2026-10-30, west: 50,000 programs seed `0xc3c35eed` (same seed as run 1) and 500,000 seed `0x20261030` |
| Divergences (cart-allowlist set) | 0 |
| Known out-of-allowlist gap | none: MULHU 0x3A CONVERGED (the reference interpreter implements it; `mulhu-probe` 2026-10-30) |
| eBPF full-engine arm | deferred to east (ADR-062 LICH-005 lesson) |
| Staged findings | none |
| Re-run trigger | any change to monad-mbc, monad-cpu-ebpf, or monad_common::cart |

## Log

- 2026-10-23: campaign scaffolded; `diff_harness` built; self-test PASS; 50k-program
  campaign CLEAN; MULHU emulator gap documented and handed to the UPC sprint
  (ADR-067/080) + Computermancer. No divergence attributable to a shipping-engine bug.
- 2026-10-30: re-run after Stage 2 and S-ASCEND-2 changed monad-mbc and monad-cpu-ebpf (the re-run
  trigger). Self-test PASS; `mulhu-probe` CONVERGED (reference and engine both 0x0b00ea4e); 50,000 programs
  (seed `0xc3c35eed`) and 500,000 (seed `0x20261030`): 0 divergences (`results/20261030_run2`, `_run3`).
  The full-engine arm with privileged opcodes (S-ASCEND-2 Steps 226-232) is still not run: the README puts it
  on east under guard, which needs Stevie's go-ahead.
