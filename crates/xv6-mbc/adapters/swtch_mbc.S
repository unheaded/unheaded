/* SPDX-License-Identifier: GPL-3.0-or-later
 *
 * swtch_mbc.S — RV32 context switch for xv6-on-UPC.
 * ASCEND-LINUX Phase 1.1 (per references/battle-plan-ascend-linux-2026-05-08.md).
 *
 * Replaces upstream/kernel/swtch.S which used RV64 sd/ld doubleword
 * instructions. We use RV32 sw/lw word instructions; struct context in
 * upstream/kernel/proc.h is patched from 14×uint64 to 14×uint32, total
 * 56 bytes (was 112 bytes).
 *
 * void swtch(struct context *old, struct context *new);
 *   old (a0) = save current callee-saved registers here
 *   new (a1) = load callee-saved registers from here
 *
 * Saved registers (RV32 callee-saved): ra, sp, s0-s11.
 * Total = 14 registers × 4 bytes each = 56 bytes.
 */

.globl swtch
swtch:
        sw ra, 0(a0)
        sw sp, 4(a0)
        sw s0, 8(a0)
        sw s1, 12(a0)
        sw s2, 16(a0)
        sw s3, 20(a0)
        sw s4, 24(a0)
        sw s5, 28(a0)
        sw s6, 32(a0)
        sw s7, 36(a0)
        sw s8, 40(a0)
        sw s9, 44(a0)
        sw s10, 48(a0)
        sw s11, 52(a0)

        lw ra, 0(a1)
        lw sp, 4(a1)
        lw s0, 8(a1)
        lw s1, 12(a1)
        lw s2, 16(a1)
        lw s3, 20(a1)
        lw s4, 24(a1)
        lw s5, 28(a1)
        lw s6, 32(a1)
        lw s7, 36(a1)
        lw s8, 40(a1)
        lw s9, 44(a1)
        lw s10, 48(a1)
        lw s11, 52(a1)

        ret
