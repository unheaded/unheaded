/* SPDX-License-Identifier: GPL-2.0 */
/*
 * arch/mbc/include/asm/ptrace.h - MBC register layout
 *
 * Matches the MbcCpuState structure from the UPC compute engine.
 * 16 general-purpose registers (r0-r15), where r15 doubles as SP.
 * Separate PC and flags registers.
 */
#ifndef _ASM_MBC_PTRACE_H
#define _ASM_MBC_PTRACE_H

#ifndef __ASSEMBLY__

struct pt_regs {
	unsigned long regs[16];	/* r0-r15, r15 = stack pointer */
	unsigned long pc;	/* program counter */
	unsigned long flags;	/* status flags */
};

#define user_mode(regs)			((regs)->flags & 0x10)
#define instruction_pointer(regs)	((regs)->pc)
#define user_stack_pointer(regs)	((regs)->regs[15])
#define profile_pc(regs)		instruction_pointer(regs)

/* Register indices */
#define MBC_REG_R0	0
#define MBC_REG_R1	1
#define MBC_REG_R2	2
#define MBC_REG_R3	3
#define MBC_REG_R4	4
#define MBC_REG_R5	5
#define MBC_REG_SP	15	/* r15 = stack pointer */

/* Syscall convention: r0=nr, r1-r5=args, r0=return */
#define syscall_get_nr(regs)		((regs)->regs[0])
#define syscall_get_return(regs)	((regs)->regs[0])

#endif /* __ASSEMBLY__ */
#endif /* _ASM_MBC_PTRACE_H */
