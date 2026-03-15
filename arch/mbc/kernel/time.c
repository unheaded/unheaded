// SPDX-License-Identifier: GPL-2.0
/*
 * arch/mbc/kernel/time.c - MBC timer driver
 *
 * Maps UPC timer interrupt to Linux jiffies.
 *
 * The UPC fires timer interrupts at ~12Hz via BPF timer callback.
 * Vector 0x20 (IRQ_TIMER) triggers do_timer() to advance jiffies
 * and update_process_times() to drive the scheduler tick.
 */

#include <linux/init.h>
#include <linux/interrupt.h>
#include <linux/time.h>
#include <linux/timex.h>
#include <linux/clocksource.h>

#include <asm/irq.h>

#define MBC_TIMER_HZ	12	/* matches BPF timer divisor */

/*
 * mbc_timer_interrupt - handle the UPC timer tick
 * @irq:    interrupt number (IRQ_TIMER / 0x20)
 * @dev_id: unused
 *
 * Called ~12 times per second by the UPC's BPF timer callback.
 * Advances jiffies and kicks the scheduler.
 */
static irqreturn_t mbc_timer_interrupt(int irq, void *dev_id)
{
	/* Update jiffies + run scheduler tick */
	do_timer(1);
	update_process_times(user_mode(get_irq_regs()));
	return IRQ_HANDLED;
}

/*
 * time_init - register timer IRQ and clocksource
 *
 * Called from start_kernel() during boot.  Hooks IRQ_TIMER
 * to mbc_timer_interrupt and registers the MBC clocksource.
 */
void __init time_init(void)
{
	if (request_irq(IRQ_TIMER, mbc_timer_interrupt,
			IRQF_TIMER, "mbc_timer", NULL))
		pr_err("MBC: failed to register timer IRQ 0x%02x\n",
		       IRQ_TIMER);

	pr_info("MBC: timer initialized at %d Hz\n", MBC_TIMER_HZ);
}

/*
 * mbc_clocksource_read - read current cycle count
 * @cs: clocksource descriptor
 *
 * The UPC exposes nanosecond time via SYS_CLOCK_GETTIME writing
 * to a known RAM address.  For this skeleton, fall back to the
 * jiffies counter until the memory-mapped ns register is wired.
 */
static u64 mbc_clocksource_read(struct clocksource *cs)
{
	return jiffies;
}

static struct clocksource mbc_clocksource = {
	.name	= "mbc",
	.rating	= 100,
	.read	= mbc_clocksource_read,
	.mask	= CLOCKSOURCE_MASK(32),
	.flags	= CLOCK_SOURCE_IS_CONTINUOUS,
};
