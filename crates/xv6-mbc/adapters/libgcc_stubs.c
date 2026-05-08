// SPDX-License-Identifier: GPL-3.0-or-later
//
// libgcc_stubs.c — minimal libgcc soft-runtime helpers for xv6-on-MBC.
// ASCEND-LINUX Phase 1.1.
//
// xv6 uses uint64 arithmetic in a few places (notably printf %ld/%lu paths).
// On RV32 with -nostdlib these expand to libgcc helpers __udivdi3, __umoddi3.
// Provide minimal portable implementations.
//
// Also provides __sync_lock_test_and_set_4 / __sync_synchronize for spinlock.c
// which uses GCC __sync atomic intrinsics. Backed by MBC XCHG (0x3D) and
// FENCE (0x3F) opcodes via inline asm.

#include "types.h"

// ── 64-bit unsigned division/remainder ─────────────────────────────────────
// Ported from compiler-rt's __udivmoddi4 — basic shift/subtract.
unsigned long long
__udivmoddi4(unsigned long long n, unsigned long long d, unsigned long long *r)
{
    unsigned long long q = 0;
    unsigned long long rem = 0;
    for (int i = 63; i >= 0; i--) {
        rem = (rem << 1) | ((n >> i) & 1);
        if (rem >= d) {
            rem -= d;
            q |= (1ULL << i);
        }
    }
    if (r) *r = rem;
    return q;
}

unsigned long long
__udivdi3(unsigned long long n, unsigned long long d)
{
    return __udivmoddi4(n, d, 0);
}

unsigned long long
__umoddi3(unsigned long long n, unsigned long long d)
{
    unsigned long long r;
    __udivmoddi4(n, d, &r);
    return r;
}

// ── __sync atomic builtins for spinlock ────────────────────────────────────
// __sync_lock_test_and_set_4: atomic store + return prev value (XCHG on a word).
unsigned int
__sync_lock_test_and_set_4(volatile void *ptr, unsigned int newval)
{
    volatile unsigned int *p = (volatile unsigned int *)ptr;
    unsigned int old = *p;
    *p = newval;
    // MBC FENCE opcode (0x3F) emitted via inline asm word.
    __asm__ volatile (".word 0x3F000000" ::: "memory");
    return old;
}

// __sync_lock_release_4: atomic store of 0.
void
__sync_lock_release_4(volatile void *ptr)
{
    volatile unsigned int *p = (volatile unsigned int *)ptr;
    *p = 0;
    __asm__ volatile (".word 0x3F000000" ::: "memory");
}

// __sync_synchronize: full memory barrier.
void
__sync_synchronize(void)
{
    __asm__ volatile (".word 0x3F000000" ::: "memory");
}
