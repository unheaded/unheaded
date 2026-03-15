// SPDX-License-Identifier: GPL-2.0
/*
 * arch/mbc/kernel/process.c - MBC process management
 *
 * Thread creation, context switching, and process startup
 * for the MBC architecture.  r15 is the stack pointer by
 * convention (matching MbcCpuState in the compute engine).
 */

#include <linux/kernel.h>
#include <linux/sched.h>

#include <asm/ptrace.h>

/*
 * start_thread - set up a new userspace thread
 * @regs: register state to initialize
 * @pc:   entry point address
 * @sp:   initial stack pointer
 *
 * Called by load_elf_binary() (or bFLT loader for nommu) to
 * prepare register state before returning to userspace.
 */
void start_thread(struct pt_regs *regs, unsigned long pc, unsigned long sp)
{
	memset(regs, 0, sizeof(*regs));
	regs->pc = pc;
	regs->regs[MBC_REG_SP] = sp;	/* r15 = stack pointer */
	regs->flags = 0x10;		/* user mode */
}

/*
 * Placeholder for context switch — the actual register save/restore
 * will be implemented in entry.S once we have the full kernel tree.
 */
