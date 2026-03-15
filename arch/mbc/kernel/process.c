// SPDX-License-Identifier: GPL-2.0
/*
 * arch/mbc/kernel/process.c - MBC process management
 *
 * Thread creation, context switching, and process startup
 * for the MBC architecture.  r15 is the stack pointer by
 * convention (matching MbcCpuState in the compute engine).
 *
 * Week 4 of S-LINUX: fork/exec/wait support.
 */

#include <linux/sched.h>
#include <linux/sched/task_stack.h>
#include <linux/kernel.h>
#include <linux/mm.h>
#include <linux/elfcore.h>
#include <linux/string.h>

#include <asm/ptrace.h>
#include <asm/thread_info.h>

/* Defined in entry.S — child starts execution here after fork */
extern void ret_from_fork(void);

/*
 * copy_thread - copy parent's register state for fork
 * @p:    child task
 * @args: clone arguments (includes kernel thread fn if applicable)
 *
 * Sets up the child's kernel stack so that when it is scheduled
 * for the first time, it enters ret_from_fork in entry.S with
 * r0 = 0 (child's fork return value).
 */
int copy_thread(struct task_struct *p, const struct kernel_clone_args *args)
{
	struct pt_regs *childregs = task_pt_regs(p);
	struct pt_regs *parentregs = current_pt_regs();

	/* Copy parent's full register frame to child */
	*childregs = *parentregs;

	/* Child returns 0 from fork */
	childregs->regs[0] = 0;

	/* Point child's saved SP/PC at the new kernel stack frame */
	p->thread.sp = (unsigned long)childregs;
	p->thread.pc = (unsigned long)ret_from_fork;

	if (args->fn) {
		/*
		 * Kernel thread: override entry point.  The fn pointer
		 * goes in PC and its argument in r0.  ret_from_fork
		 * will call schedule_tail() then jump to thread.pc.
		 */
		childregs->regs[0] = (unsigned long)args->fn_arg;
		p->thread.pc = (unsigned long)args->fn;
	}

	return 0;
}

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
 * flush_thread - reset thread state on exec
 *
 * Called during execve() to clear architecture-specific state
 * (FPU, debug registers, etc.) that must not leak across exec.
 */
void flush_thread(void)
{
	memset(&current->thread, 0, sizeof(struct thread_struct));
}

/*
 * dump_thread - save registers for core dump
 * @regs: current register state
 * @dump: user struct to fill in
 *
 * Copies pt_regs into the user struct so that a core dump
 * contains the register state at the time of the fault.
 */
void dump_thread(struct pt_regs *regs, struct user *dump)
{
	memcpy(&dump->regs, regs, sizeof(struct pt_regs));
}

/*
 * cpu_idle - idle loop for MBC
 *
 * Executes HLT (halt until interrupt) in a loop.  The timer
 * IRQ will wake the CPU and the scheduler can pick a runnable
 * task.
 */
void cpu_idle(void)
{
	while (1) {
		/* MBC HLT instruction — waits for next interrupt */
		asm volatile("HLT");
	}
}

/*
 * machine_halt - stop the CPU permanently
 */
void machine_halt(void)
{
	pr_info("MBC: System halted\n");
	asm volatile("HLT");
	while (1)
		;
}

/*
 * machine_restart - reset the CPU
 * @cmd: restart command string (unused on MBC)
 *
 * Jumps to address 0 to re-enter the bootloader.
 */
void machine_restart(char *cmd)
{
	pr_info("MBC: System restart requested\n");
	/* Reset PC to 0 — re-enter bootloader */
	asm volatile("MOVI r15, 0; JMP 0");
	while (1)
		;
}

/*
 * machine_power_off - power down (identical to halt on MBC)
 */
void machine_power_off(void)
{
	machine_halt();
}
