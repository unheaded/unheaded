// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef _SETJMP_H
#define _SETJMP_H

// RV32I with 16 registers: save callee-saved regs + sp + ra + pc
typedef unsigned int jmp_buf[16];

int setjmp(jmp_buf env);
void longjmp(jmp_buf env, int val);

#endif
