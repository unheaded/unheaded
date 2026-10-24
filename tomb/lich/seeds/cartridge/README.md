<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Cartridge seeds (S5 Phase 17)

Seed corpus for `fuzz_cart_image` (`crates/monad-mbc/fuzz`) and starting
images for any Lich campaign against the MBC cartridge loader (ADR-108). Hook
documentation: [`../../UPC-CARTRIDGE-HOOKS.md`](../../UPC-CARTRIDGE-HOOKS.md).

| File | Built from | What the loader does with it |
|---|---|---|
| `ratelimit.cart` | `cartridges/ratelimit.s` + `ratelimit.data.bin` (XDP) | accepts |
| `sampler.cart` | `cartridges/sampler.s` + `sampler.data.bin` (XDP) | accepts |
| `scorer.cart` | `cartridges/scorer.s` + `scorer.data.bin` (tick, `--loop-bound 16`) | accepts |
| `mut-ratelimit-bitflip.cart` | ratelimit, one ROM bit flipped, sha left stale | decode refuses: sha256 mismatch |
| `mut-sampler-truncated.cart` | sampler minus its last word | decode refuses: length |
| `mut-scorer-badopcode.cart` | scorer, word 0 opcode 0x04 (DIV), sha recomputed | validate refuses: opcode |
| `mut-ratelimit-backedge.cart` | ratelimit, word 5 = `JMP -2`, sha recomputed | validate refuses: XDP back-edge |
| `mut-sampler-maxinsn.cart` | sampler, `max_insn` 65, sha recomputed | validate refuses: max_insn |
| `mut-scorer-data.cart` | scorer, one DATA bit flipped, sha recomputed | accepts (a different legal scorer) |

The shipped images are rebuilt from source whenever a cartridge changes
(`cart build`, flags as in `cartridges/README.md`); `ratelimit.cart` was
rebuilt for the qos-ebpf remainder fix (`8ab534b3`). The mutations are
regenerated from the shipped images by flipping the bytes named above and,
where noted, recomputing the header sha256 over `header[0..20) || ROM || DATA`.

None of these files holds packet data: a `.cart` is code and model
parameters only.
