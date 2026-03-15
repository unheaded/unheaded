// SPDX-License-Identifier: GPL-2.0
/*
 * arch/mbc/kernel/traps.c - MBC exception / trap handlers
 *
 * Installs exception vectors and provides die() for fatal
 * faults (illegal instruction, bus error, etc.).
 */

#include <linux/init.h>
#include <linux/kernel.h>
#include <linux/signal.h>
#include <linux/sched/signal.h>

#include <asm/ptrace.h>

/*
 * trap_init - install exception handlers
 *
 * Called once during boot.  On MBC the IVT is set up in
 * entry.S (setup_ivt); this function handles any remaining
 * kernel-side initialization.
 */
void __init trap_init(void)
{
	pr_info("MBC: Exception handlers installed\n");
}

/*
 * die - print registers and terminate the current task
 * @str:  human-readable fault description
 * @regs: register state at time of fault
 * @err:  error code / trap number
 *
 * Dumps the full register file so the developer can
 * diagnose the crash, then kills the task with SIGSEGV.
 */
void die(const char *str, struct pt_regs *regs, long err)
{
	int i;

	pr_emerg("MBC: %s [#%ld]\n", str, err);
	pr_emerg("  PC: %08lx  SP: %08lx  FLAGS: %08lx\n",
		 regs->pc, regs->regs[MBC_REG_SP], regs->flags);

	/* Dump all 16 general-purpose registers, four per line */
	for (i = 0; i < 16; i += 4)
		pr_emerg("  r%-2d: %08lx  r%-2d: %08lx  r%-2d: %08lx  r%-2d: %08lx\n",
			 i,   regs->regs[i],
			 i+1, regs->regs[i+1],
			 i+2, regs->regs[i+2],
			 i+3, regs->regs[i+3]);

	do_exit(SIGSEGV);
}
