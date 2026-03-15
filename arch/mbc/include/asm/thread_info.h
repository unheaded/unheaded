/* SPDX-License-Identifier: GPL-2.0 */
/*
 * arch/mbc/include/asm/thread_info.h - MBC thread state
 *
 * Defines the per-thread CPU state saved/restored during context
 * switches.  The switch_to() code in switch.S saves callee-saved
 * registers (r4-r14), SP, and PC into this structure; entry.S
 * saves/restores the full register set for syscall and IRQ frames.
 *
 * Register conventions:
 *   r0-r3   caller-saved (scratch / arguments)
 *   r4-r12  callee-saved (preserved across calls)
 *   r13     LR (link register)
 *   r14     FP (frame pointer, callee-saved)
 *   r15     SP (stack pointer)
 */
#ifndef _ASM_MBC_THREAD_INFO_H
#define _ASM_MBC_THREAD_INFO_H

#include <asm/page.h>

/*
 * Thread flags — stored in thread_info.flags, tested on
 * return-to-userspace paths in entry.S.
 */
#define TIF_NEED_RESCHED	0	/* rescheduling requested */
#define TIF_SIGPENDING		1	/* signal pending */
#define TIF_SYSCALL_TRACE	2	/* ptrace syscall tracing */
#define TIF_NOTIFY_RESUME	3	/* callback before return to user */

#define _TIF_NEED_RESCHED	(1 << TIF_NEED_RESCHED)
#define _TIF_SIGPENDING		(1 << TIF_SIGPENDING)
#define _TIF_SYSCALL_TRACE	(1 << TIF_SYSCALL_TRACE)
#define _TIF_NOTIFY_RESUME	(1 << TIF_NOTIFY_RESUME)

/* Mask of flags that require work on return to userspace */
#define _TIF_WORK_MASK		(_TIF_NEED_RESCHED | _TIF_SIGPENDING | \
				 _TIF_NOTIFY_RESUME)

#ifndef __ASSEMBLY__

/*
 * struct thread_struct - per-thread CPU state for context switching
 *
 * Saved by switch_to() in switch.S.  Layout must match the
 * offsets (THREAD_SP, THREAD_PC, THREAD_REGS, THREAD_FLAGS)
 * used by the assembly code.
 *
 * Size: 4 + 4 + 64 + 4 = 76 bytes (19 words)
 */
struct thread_struct {
	unsigned long sp;	/* +0x00: saved stack pointer */
	unsigned long pc;	/* +0x04: saved program counter */
	unsigned long regs[16];	/* +0x08: saved registers r0-r15 */
	unsigned long flags;	/* +0x48: saved CPU flags register */
};

#define INIT_THREAD { \
	.sp	= 0, \
	.pc	= 0, \
	.regs	= { 0 }, \
	.flags	= 0, \
}

/*
 * Kernel stack size — 8KB (2 pages).  Each task gets its own
 * kernel stack with thread_info at the bottom.
 */
#define THREAD_SIZE_ORDER	1
#define THREAD_SIZE		(PAGE_SIZE << THREAD_SIZE_ORDER)

/*
 * struct thread_info - low-level thread state at bottom of kernel stack
 *
 * Lives at the lowest address of the kernel stack.  The current
 * thread_info pointer is derived by masking SP:
 *   current_thread_info() = SP & ~(THREAD_SIZE - 1)
 */
struct thread_info {
	struct task_struct	*task;		/* main task structure */
	unsigned long		flags;		/* thread flags (TIF_*) */
	int			preempt_count;	/* preemption nesting depth */
	int			cpu;		/* current CPU */
};

#define INIT_THREAD_INFO(tsk) {		\
	.task		= &(tsk),	\
	.flags		= 0,		\
	.preempt_count	= 0,		\
	.cpu		= 0,		\
}

/*
 * current_thread_info - get thread_info for the running thread
 *
 * The thread_info sits at the bottom of the kernel stack.
 * Mask SP to find the stack base.
 */
static inline struct thread_info *current_thread_info(void)
{
	unsigned long sp;

	/*
	 * In a real MBC port this would be an inline asm reading r15.
	 * For now we use GCC's __builtin_frame_address as a proxy.
	 */
	sp = (unsigned long)__builtin_frame_address(0);
	return (struct thread_info *)(sp & ~(THREAD_SIZE - 1));
}

/*
 * test_thread_flag / set_thread_flag / clear_thread_flag
 *
 * Operate on current_thread_info()->flags.
 */
static inline int test_ti_thread_flag(struct thread_info *ti, int flag)
{
	return ti->flags & (1UL << flag);
}

static inline void set_ti_thread_flag(struct thread_info *ti, int flag)
{
	ti->flags |= (1UL << flag);
}

static inline void clear_ti_thread_flag(struct thread_info *ti, int flag)
{
	ti->flags &= ~(1UL << flag);
}

#endif /* __ASSEMBLY__ */
#endif /* _ASM_MBC_THREAD_INFO_H */
