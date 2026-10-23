<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# LICH-015 exit criteria (ADR-062)

A campaign closes when its exit criteria are met; it re-arms on its re-run trigger.

## Coverage threshold

- ≥ 80% of the ISA-v2 opcode **decode paths** exercised across the cart-allowlist
  opcode set by the lockstep oracle. The generator emits ADD/SUB/MUL/AND/OR/XOR/NOT,
  SHL/SHR/SAR, SHLR/SHRR/SARR, MOV/MOVI/LOAD_IMM32/ADDI, CMP, MULH, NOP and the seven
  branch forms, with interesting operands (0, 0xFFFFFFFF, 0x80000000, small shift
  counts, random), so the allowlist is fully covered.
- The out-of-allowlist opcodes (MULHU, MRET/SRET/LR.W/SC.W/FENCE) are covered by
  `mulhu-probe` (reference-side) and the kernel-attached east arm respectively.

## Crash / divergence budget

- **24 h continuous with zero net-new divergences** over the cart-allowlist set.
  (First run: 50,000 programs clean.)
- A pre-existing, documented divergence (the MULHU emulator gap) is **not** net-new.

## Severity gate

- Any **new** reference-vs-engine divergence over the cart-allowlist set is **HIGH+**
  and **blocks a UPC release** until triaged: if the engine (monad-cpu-ebpf) is the
  one that drifted, it is a shipping bug (file against shipping code); if the reference
  is incomplete/wrong, it is an emulator gap (fix the emulator, do not file against
  shipping code).
- The MULHU emulator gap is **LOW** (reference-interpreter completeness), tracked for
  the UPC sprint; it does not block a release because the shipping engine is correct.

## Re-run trigger

- **Any change to `crates/monad-mbc` or `ebpf/monad-cpu-ebpf` / `ebpf/monad-common::cart`**
  re-arms this campaign (ADR-062 weekly replay + on-change). The regression seeds under
  `tomb/lich/seeds/lich_015_mbc_differential/` and the fixed-seed campaign (`--seed
  0xc3c35eed`) reproduce deterministically.

## Close condition

- Oracle self-test PASS **and** 24 h / weekly replay with zero net-new divergences over
  the allowlist set **and** every staged divergence finding closed on passing evidence
  (its regression seed re-runs clean against the fixed implementation).
