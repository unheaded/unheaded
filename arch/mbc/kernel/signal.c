// SPDX-License-Identifier: GPL-2.0
/*
 * arch/mbc/kernel/signal.c - MBC signal delivery
 *
 * Sets up signal frames on the user stack and handles
 * sigreturn to restore saved register state.
 *
 * Signal delivery flow:
 *   1. Kernel detects pending signal on return-to-userspace path
 *   2. do_signal() calls setup_frame() to push a sigframe
 *   3. Userspace handler runs with signal number in r0
 *   4. Handler returns via sigreturn trampoline (INT 0x80)
 *   5. sys_sigreturn() restores saved pt_regs
 */

#include <linux/signal.h>
#include <linux/sched/signal.h>
#include <linux/string.h>

#include <asm/ptrace.h>

/*
 * struct sigframe - signal context pushed onto the user stack
 *
 * When a signal is delivered the kernel pushes this frame below
 * the current user SP.  The retcode field contains a small
 * trampoline that issues the sigreturn syscall.
 */
struct sigframe {
	unsigned long retcode;		/* sigreturn trampoline */
	unsigned long signal;		/* signal number */
	struct pt_regs saved_regs;	/* saved register state */
};

/*
 * setup_frame - build a signal frame on the user stack
 * @ksig: kernel signal information
 * @regs: interrupted register state
 *
 * Pushes a sigframe, redirects PC to the signal handler,
 * and puts the signal number in r0 (first argument).
 */
static int setup_frame(struct ksignal *ksig, struct pt_regs *regs)
{
	struct sigframe *frame;
	unsigned long sp = regs->regs[MBC_REG_SP];

	/* Allocate space on the user stack */
	sp -= sizeof(struct sigframe);
	frame = (struct sigframe *)sp;

	/* Save current register state for sigreturn */
	frame->saved_regs = *regs;
	frame->signal = ksig->sig;

	/*
	 * Set up return trampoline.  When the signal handler does
	 * a normal return, execution hits this code which issues
	 * INT 0x80 with r0 = __NR_sigreturn.
	 *
	 * The trampoline encodes:
	 *   MOVI r0, __NR_sigreturn
	 *   INT  0x80
	 *
	 * For now we store a placeholder — the actual encoding
	 * depends on the MBC instruction format.
	 */
	frame->retcode = 0;

	/* Redirect execution to the signal handler */
	regs->pc = (unsigned long)ksig->ka.sa.sa_handler;
	regs->regs[0] = ksig->sig;	/* signal number as arg0 */
	regs->regs[MBC_REG_SP] = sp;	/* update user stack pointer */

	return 0;
}

/*
 * do_signal - process pending signals on return to userspace
 * @regs: register state from the interrupted context
 *
 * Called from the return-to-userspace path in entry.S when
 * TIF_SIGPENDING is set.
 */
void do_signal(struct pt_regs *regs)
{
	struct ksignal ksig;

	if (get_signal(&ksig)) {
		setup_frame(&ksig, regs);
		signal_setup_done(0, &ksig, 0);
	}
}

/*
 * sys_sigreturn - restore register state after signal handler
 *
 * The sigreturn trampoline (or an explicit sigreturn call)
 * invokes this syscall.  We read the sigframe from the user
 * stack and restore the saved pt_regs.
 */
long sys_sigreturn(void)
{
	struct pt_regs *regs = current_pt_regs();
	struct sigframe *frame = (struct sigframe *)regs->regs[MBC_REG_SP];

	/* Restore the register state saved at signal delivery time */
	*regs = frame->saved_regs;

	return regs->regs[0];
}
