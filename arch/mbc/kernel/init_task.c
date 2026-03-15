// SPDX-License-Identifier: GPL-2.0
/*
 * arch/mbc/kernel/init_task.c - Initial task (swapper/idle)
 *
 * Defines the statically-allocated task_struct for process 0 — the
 * idle/swapper task.  Every other task is ultimately forked from this.
 * The INIT_TASK macro populates all scheduler, signal, mm, and
 * accounting fields to safe defaults.
 *
 * Week 8 of S-LINUX: the kernel needs PID 0 to bootstrap PID 1.
 */

#include <linux/init_task.h>
#include <linux/sched.h>

/*
 * init_task — process 0 (swapper/idle)
 *
 * This is the ancestor of every process.  start_kernel() runs in
 * the context of init_task.  After setting up the system, it
 * calls kernel_init() which fork+exec's /init (PID 1) and then
 * falls into cpu_idle().
 */
struct task_struct init_task = INIT_TASK(init_task);
