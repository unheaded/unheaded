<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# UPC cartridge hooks for the Lich

Fuzz and differential surfaces that S5 (`docs/battle-plans/S-UPC-WIRE-COMPUTE-battle-plan.md`,
Phase 17) leaves for the Lich. The UPC sprint ships the hooks and runs them once;
campaigns, budgets and findings belong to the Lich (ADR-062). Cartridges: ADR-108.

## Engines and oracle

Three engines run MBC cartridges. Two of them share one source.

| Engine | Code | How it is driven |
|---|---|---|
| shared step | `ebpf/monad-common/src/cart.rs` (`monad_common::cart`) | host: `upc_cartridge::diff::lockstep`, `cart run` |
| host MBC emulator | `crates/monad-mbc/src/execute.rs` (`ExecCpu`) | `lockstep` |
| BPF interpreter | `ebpf/cartridge-ebpf` `cart_xdp` (the shared step compiled to BPF) | `pkg/cartridge` through `BPF_PROG_TEST_RUN`, nothing attached |

Oracle: the shared step and the host emulator run the same image in lockstep; after
every instruction the 16 registers, Z/N/C, the PC and the 64-word flow window must be
equal. The BPF engine must then reach the same verdict, the same `FLOW_STATE` state
hash and the same EMIT records. Not a divergence:

- a cartridge **fault** (out-of-window access, unknown FIELD id, bad VERDICT code):
  the shared step refuses, the host emulator has no windows. Comparison stops at the
  fault; BPF must store the shared step's state at the fault.
- **syscalls**: they are the cartridge's I/O, not ISA semantics. The harness services
  them on the host-emulator side from the same input (FIELD, NOW_NS_LO, EMIT,
  VERDICT); EMIT records are compared.

Which side is wrong is decided against `monad-cpu-ebpf`, the shipping engine: the
shared step copies its semantics; the host emulator is the one that drifted before
(S5 Phase 10: nine opcodes; MULHU missing until `4cbef086`).

## Targets

| Target | Kind | Input | Checks |
|---|---|---|---|
| `fuzz_cart_differential` | libFuzzer (`crates/monad-mbc/fuzz`) | bytes = entropy for a random validated XDP cartridge + its FIELD values and clock | lockstep agreement; encode/decode round trip; validator accepts |
| `fuzz_cart_image` | libFuzzer | arbitrary bytes as a `.cart` | `Image::decode` / `validate` never panic; decode is canonical; an accepted image runs (XDP one packet, tick one job) without panic |
| `fuzz_cart_step` | libFuzzer (Phase 10) | raw ROM/DATA/regs | the shared step never panics; budgets hold |
| `cart diff` | CLI (`crates/upc-cartridge`) | `--seed`, `--count` | seeded lockstep campaign; exit 3 on divergence |
| `TestDifferential_ThreeEngines` | Go, root-gated (`pkg/cartridge`) | `cart diff --out` cases | BPF verdict, state hash, EMITs == the two host engines |

Generator (`upc_cartridge::diff::gen_xdp_cartridge`): up to 48 words, forward
branches only (so no run reaches the 64-instruction XDP budget), register prologue,
ALU with edge operands (0, 0xFFFFFFFF, 0x80000000, shifts >= 32), LD/ST/LDB into the
flow window and DATA, FIELD/NOW_NS_LO/EMIT/VERDICT, and on purpose about one memory
access and one FIELD id in sixteen out of range so the fault paths run too.

## Running

```
# host differential (seeded, no root)
cargo build --release --manifest-path crates/upc-cartridge/Cargo.toml
crates/upc-cartridge/target/release/cart diff --seed 0x17 --count 20000

# three engines (root: loads cartridge-ebpf, test_run only)
(cd ebpf && cargo build --release -p cartridge-ebpf)
go test -c -o /tmp/cart.test ./pkg/cartridge/
(cd pkg/cartridge && sudo CART_DIFF_COUNT=1000 /tmp/cart.test -test.run ThreeEngines -test.v)

# fuzz (cargo fuzz, or without it: build with cargo-fuzz's RUSTFLAGS and run the binary)
cd crates/monad-mbc && cargo +nightly fuzz run fuzz_cart_differential -- -max_total_time=300
cargo +nightly fuzz run fuzz_cart_image ../../tomb/lich/seeds/cartridge -- -max_total_time=300
```

Seeds: `tomb/lich/seeds/cartridge/` (the three shipped images and six mutations, see
its README). The regression harness (`scripts/upc-regression.sh`) runs the
three-engine test with every cartridge root test.

## Confinement

The host targets are pure functions over memory: no files, no network, no
privileges. The BPF leg loads `cartridge-ebpf` and runs `cart_xdp` through
`BPF_PROG_TEST_RUN`: no attach, no interface, maps freed when the test exits. It is
the same path Phases 11–16 ran on west. It does **not** drive `monad-cpu-ebpf` (the
full UPC engine: privileged opcodes, MULHU in-engine, ascend); that arm stays on east
per LICH-015 and ADR-062's LICH-005 lesson.

## Results (S5, 2026-10-23, west)

- Lockstep, `cart diff --seed 0x17 --count 20000`: 296,740 instructions compared,
  **0 divergences** (11,212 halt, 4,320 verdict, 4,468 fault endings).
- `TestDifferential_ThreeEngines`, 1,000 cartridges (seed `0x17d1ff`): **1,000/1,000**
  agree across all three engines (577 halt, 190 verdict, 233 fault; 0 breaker trips).
- `fuzz_cart_differential`, 300 s: 3,046,713 runs, 0 crashes, 0 divergences
  (corpus 452).
- `fuzz_cart_image`, 300 s from the seed corpus: 95,882,146 runs, 0 crashes (corpus
  9 → 413; without the re-hash pass it stalled at 31, since the sha256 rejects
  almost every mutation at decode).
- Oracle self-test: a one-bit flip in the host arm after instruction 0 is reported at
  instruction 0 (`tests/diff.rs`).

## Integration with LICH-015 (branch `sprint/lich-awakens`, not merged here)

LICH-015 (`tomb/lich/LICH-015-mbc-differential/`) is the campaign these hooks belong
to; no new campaign number. On merge:

1. **Arm C.** LICH-015's `diff_harness` compares `ExecCpu` and the shared step on
   registers/flags/PC for ALU and branch programs. These hooks add memory windows,
   syscalls and EMIT/VERDICT, and the BPF `cart_xdp` engine. Add `cart diff` and the
   two fuzz targets to its README "Harness" section and `exit-criteria.md` coverage;
   add `crates/upc-cartridge` and `ebpf/cartridge-ebpf` to its re-run trigger.
2. **MULHU seed closes.** `seeds/lich_015_mbc_differential/mulhu_highword.mbc.hex`
   documents the host emulator NOP-ing MULHU. Fixed here in `4cbef086`. LICH-015's own
   harness, built against this branch from a scratch copy (no repo change):
   `diff_harness mulhu-probe` → `CONVERGED` (`r0 = 0x0b00ea4e` on both);
   `self-test` PASS; `run --iters 50000 --seed 0xc3c35eed` → 0 divergences. Re-run
   `mulhu-probe` after the merge and close the seed per its README.
3. **Lich MCP `run_harness`.** `cart diff` exits 0 (agree), 3 (divergence, cases
   printed with their image hex), 1 (usage or I/O). It fits the harness-command shape
   as is; a divergent case is written as `diverged-NNNN.cart` under `--out`.
4. **ADR-062 inventory.** These targets are LICH-015 rows, not a new campaign.
