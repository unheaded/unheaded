<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# LICH-015 — MBC ISA-v2 reference-vs-eBPF differential

**Opened:** 2026-10-23 (S-LICH-AWAKENS Phase 7). **ADRs:** 062 (framework),
067 (MBC ISA v2 + UPC ABI v1), 080 (UPC programmatic API). **Coordination:**
the UPC sprint (`sprint/upc-wire-compute`) + Computermancer.

## Target

The MBC instruction set has two implementations that must agree:

- **Reference interpreter** — `crates/monad-mbc` (`execute.rs`, `monad_mbc::ExecCpu`),
  the userspace host emulator.
- **Shipping engine** — `ebpf/monad-cpu-ebpf`, the eBPF program that actually
  runs MBC on the UPC. In-process it is modelled by `monad_common::cart::step`,
  which copies the monad-cpu-ebpf ALU/flag semantics verbatim and is proven
  **`BPF == reference`** on a live kernel by `pkg/cartridge` through
  `BPF_PROG_TEST_RUN` (S5 Phase 11: 3/3 flow-state hashes, 21/21 events).

## Oracle

Run a bounded MBC program on both arms from the **same** initial register state
and compare, in lockstep after every instruction, the 16 registers, the three
condition flags (N/Z/C) and the program counter. The first instruction whose two
outcomes differ is the divergence. State-equivalence after a bounded run needs no
ground truth — just two implementations — which is the standard differential
oracle.

## Harness

`harnesses/diff_harness/` — a standalone Rust crate (path deps only; no new
third-party crate). Subcommands:

```
diff_harness run [--iters N] [--seed HEX] [--out DIR]   # campaign
diff_harness self-test                                   # injected-divergence proof
diff_harness mulhu-probe                                 # out-of-allowlist gap report
diff_harness corpus <file...>                            # replay seed programs (hex words)
```

Build + run:

```
cd tomb/lich/LICH-015-mbc-differential/harnesses/diff_harness
cargo build --release
./target/release/diff_harness self-test
./target/release/diff_harness run --iters 50000 --out ../../results/<ts>
```

## Scope & the two arms

The in-process arm B (`cart::step`) executes the **ADR-108 cartridge allowlist**
opcode set (ALU, shifts, branches, MUL/MULH, CMP, MOV/MOVI/LOAD_IMM32/ADDI).
Opcodes **outside** that allowlist cannot be driven through arm B in-process —
`cart::step` faults on them by design — so:

- **MULHU (0x3A)** and the ISA-v2 privileged opcodes (MRET/SRET/LR.W/SC.W/FENCE)
  are not in arm B. MULHU is handled by `diff_harness mulhu-probe` (reference-side,
  static). The privileged opcodes and the full-engine comparison are the
  **kernel-attached east arm** — never a dev host's live kernel unguarded
  (ADR-062 LICH-005's lesson).

## Results (2026-10-23, S-LICH-AWAKENS Phase 7, west)

- **Oracle self-test: PASS** — a known-equal program is clean; a single injected
  bit-flip on the reference arm is reported divergent. The oracle is not vacuous.
- **Campaign: 50,000 random programs, seed `0xc3c35eed`, 0 divergences** over the
  cart-allowlist opcode set. This locks in the S5 Phase 10 fix of nine host-emulator
  opcode drifts (MUL/shift C-flag, JP condition, MULH signedness) — a re-run would
  re-catch any regression. See `results/20261023_run1/`.
- **MULHU out-of-allowlist probe: DIVERGENT (expected).** The reference interpreter
  has no MULHU arm; its default opcode arm treats MULHU as a NOP ("fail-open for
  forward compat"), so `r0` is unchanged, while monad-cpu-ebpf computes the unsigned
  high word (ADR-067). **This is an EMULATOR GAP** — the reference interpreter is
  incomplete; the shipping engine is correct. It is **not** a bug in Stevie's shipping
  code and is **not** filed against it. Disposition: hand to the UPC sprint
  (ADR-067/080) + Computermancer to decide whether to complete the reference
  interpreter's MULHU arm (a fix to the emulator, not the ISA).

## Disposition rule (ADR-062)

A reference-vs-engine divergence is a protocol/ISA finding. This campaign **finds,
it does not fix** the ISA (that touches FROZEN-adjacent UPC internals — Computermancer's
lane, the UPC sprint's scope). Each unique divergence is staged pending-confirmation as
an `FND-NNN` with the program, both final states, and the opcode implicated, tagged for
the UPC sprint. A trivial harness bug is fixed in the harness, not the ISA.

See `exit-criteria.md` for the re-arm triggers and the severity gate.
