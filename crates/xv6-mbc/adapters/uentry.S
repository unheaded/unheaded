# SPDX-License-Identifier: GPL-3.0-or-later
#
# uentry.S — UNHEADED kernel entry: the first instructions the UPC executes.
# Track 2 Phase 2.3 (ADR-081 "evolve from xv6, never break the boot").
#
# Replaces MIT kernel/entry.S in the Makefile.mbc OBJS list; the MIT file
# stays vendored, unwired. Near-merger-doctrine territory: a boot entry that
# must (a) give C a stack and (b) jump to start() has essentially one correct
# shape, so the instruction sequence matches stock by construction — the
# authorship is in choosing to keep the per-hart computation live.
#
# Behavior, identical to stock by requirement:
#   sp = stack0 + (mhartid + 1) * 4096
#   call start()          # start_mbc.c — ours since Phase 1.1
#   spin if start returns # it never does
#
# stack0 is declared in adapters/start_mbc.c (4096 bytes per hart). The UPC
# runs a single hart, so mhartid reads 0 and sp lands at stack0 + 4096; the
# csrr is kept (translator maps CSRs to the 0x000_F000 region per ADR-067)
# so the computation stays bit-equal to stock on any hart count.

.section .text
.global _entry
_entry:
        # carve this hart's stack out of stack0: sp = stack0 + 4096*(mhartid+1)
        la sp, stack0
        li a0, 1024*4
        csrr a1, mhartid
        addi a1, a1, 1
        mul a0, a0, a1
        add sp, sp, a0
        # hand off to C — start() in start_mbc.c (M-mode setup, then MRET)
        call start
halt:
        # unreachable: start() never returns. Park the hart if it ever does.
        j halt
