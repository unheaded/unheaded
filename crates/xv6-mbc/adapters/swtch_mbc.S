/* SPDX-License-Identifier: GPL-3.0-or-later
 *
 * swtch_mbc.S — RV32 context switch for xv6-on-UPC.
 * ASCEND-LINUX Phase 1.1 (per references/battle-plan-ascend-linux-2026-05-08.md).
 *
 * Replaces upstream/kernel/swtch.S which used RV64 sd/ld doubleword
 * instructions. We use RV32 sw/lw word instructions.
 *
 * void swtch(struct context *old, struct context *new);
 *   old (a0) = save current callee-saved registers here
 *   new (a1) = load callee-saved registers from here
 *
 * Saved registers (MBC-mappable subset of RV32 callee-saved):
 *   ra, sp, s0, s1.
 *
 * The full RV32 callee-saved set is ra, sp, s0-s11 (= x1, x2, x8, x9, x18-x27).
 * MBC exposes only x0-x15, so x18-x27 (s2-s11) are out of register range.
 * Makefile.mbc CFLAGS pin -ffixed-x18 ... -ffixed-x27 so the C compiler never
 * assigns code to those registers; therefore they cannot hold live values
 * across a swtch() call site, and saving/restoring them is dead work.
 *
 * Total saved = 4 registers × 4 bytes = 16 bytes (was 56 bytes pre-strip).
 * struct context in upstream/kernel/proc.h is sized to match.
 */

.globl swtch
swtch:
        sw ra, 0(a0)
        sw sp, 4(a0)
        sw s0, 8(a0)
        sw s1, 12(a0)

        lw ra, 0(a1)
        lw sp, 4(a1)
        lw s0, 8(a1)
        lw s1, 12(a1)

        ret
