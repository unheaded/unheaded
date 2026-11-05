// SPDX-License-Identifier: GPL-3.0-or-later
// uyield.h: yield() as an inline stub, SYS_yield = 23 (ADR-078 §5, DP-413).
// Not in usys.pl on purpose: usys.o links into every program, and a bigger
// text changes the file sizes the `ls` golden prints.
#ifndef UYIELD_H
#define UYIELD_H
static inline int
yield(void)
{
  int r;
  __asm__ volatile("li a7, 23\n\tecall\n\tmv %0, a0" : "=r"(r) : : "a0", "memory");
  return r;
}
#endif
