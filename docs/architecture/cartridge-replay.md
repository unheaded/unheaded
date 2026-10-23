<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Cartridge replay: the determinism contract

MBC cartridges (ADR-108) run in `ebpf/cartridge-ebpf` and replay off-line in
`crates/upc-cartridge` (`cart replay`, `upc-cartridgectl replay`). This note
states what makes a replay bit-exact, what a trace holds, and what a
divergence means.

## Contract

```
state_{n+1} = step(state_n, rom, data, inputs)
```

- `step` is one source, `monad_common::cart::run_one`, compiled into the BPF
  object and into the host tools. The BPF program drives it from a `bpf_loop`
  callback (4 steps per iteration); the host drives it from a plain loop. Both
  stop at the same instruction and call the same `run_out` at budget-out.
- The only inputs a run can read are the FIELD table (header metadata, ADR-108
  FIELD ids 0-12) and the run's clock value (`NOW_NS_LO`). Nothing else enters:
  no helper, map or packet byte is reachable from MBC.
- So the image (ROM + DATA), the starting state and those inputs determine
  every following state. `state_hash` (32-bit FNV-1a over regs, pc, flags and
  the 64-word flow window) is checked after every instruction.

## What a trace holds

Tracing is off by default. `CART_CFG[TRACE] = N` traces one run in N per CPU
(`upc-cartridgectl record --every N`). A traced run is:

| Record | Size | Content |
|---|---|---|
| head | 464 B | slot, generation, stage, run id, clock, flow key, FIELD table, starting `CartState` |
| step | 32 B | instruction index, pc, instruction word, step code, `state_hash` after it |
| end | 32 B | instructions executed, how the run ended, final `state_hash` |

All three are Anamnesis `EVENT_COMPUTED (0x01)` with the cartridge sub-kind
`0x43` (provisional, RFC Editor item 2). The file (`UCTR` v1) adds the
sha256 of the `.cart` each slot ran. A full ring drops records and counts
them (`StatTraceDropped`); replay then reports the run incomplete.

A trace carries header fields, flow keys and cartridge state: as sensitive as
flow metadata, never payload. Files are written 0600.

## Reading a replay

- **BIT-EXACT**: every step's pc, word, code and hash, and the end record,
  match.
- **DIVERGED at step i (what)**: the first instruction whose replay differs.
  `insn` at step 0 means another image; `hash` at the first instruction that
  reads an input means a corrupted input; `hash` elsewhere means the engines
  disagree (S5 Emergency A.7: field parsing or window bounds, since the
  arithmetic is one source).
- **INCOMPLETE**: records were dropped; nothing is claimed for that run.

Time travel: `--to N` prints the state before instruction N, `--watch rN` or
`--watch mem[ADDR]` prints every change of one location, `--steps` lists the
run. The library API (`upc_cartridge::replay::Replayer`) adds `step`, `back`,
`goto`, `regs`, `mem` and `inspect`.

Evidence: `references/s5-upc-wire-log.md` (S5 Phase 16).
