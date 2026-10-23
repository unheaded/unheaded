<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# LICH-015 regression seeds

Each `*.mbc.hex` file is one MBC program: whitespace-separated 32-bit instruction
words in hex (a trailing HALT is appended automatically). Replay them against both
differential arms with:

```
cd ../../LICH-015-mbc-differential/harnesses/diff_harness
cargo build --release
./target/release/diff_harness corpus ../../../seeds/lich_015_mbc_differential/*.mbc.hex
```

These re-arm the campaign on any future change to `crates/monad-mbc`,
`ebpf/monad-cpu-ebpf`, or `ebpf/monad-common::cart` (ADR-062 re-run trigger). A seed
stays checked in until its finding closes on passing evidence.

- `mulhu_highword.mbc.hex` — the documented out-of-allowlist MULHU gap: `MULHU r0, r1`
  with `r0`/`r1` chosen so the product's high word is non-zero. The reference
  interpreter NOPs it (regs unchanged); monad-cpu-ebpf writes the unsigned high word.
  This is an EMULATOR GAP (reference incomplete), not a shipping-engine bug. The `corpus`
  subcommand runs only arm A (reference) + arm B (cart::step), and cart::step faults on
  MULHU by ADR-108 design, so use `diff_harness mulhu-probe` for this seed; it is kept
  here as the canonical reproducer for the gap.
