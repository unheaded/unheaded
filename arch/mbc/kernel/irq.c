// SPDX-License-Identifier: GPL-2.0
/*
 * arch/mbc/kernel/irq.c - MBC interrupt controller
 *
 * Simple interrupt controller that routes IVT vectors to the
 * Linux IRQ subsystem.  The UPC uses software interrupts (INT n)
 * for both hardware-style events and syscalls:
 *
 *   0x00-0x1F  CPU exceptions (reserved)
 *   0x20       Timer (BPF timer callback)
 *   0x21       Keyboard (KBD_MAP ring buffer)
 *   0x80       Syscall entry (INT 0x80)
 *
 * NR_IRQS is defined in asm/irq.h (256 vectors).
 */

#include <linux/init.h>
#include <linux/interrupt.h>
#include <linux/irq.h>

#include <asm/irq.h>

/*
 * init_IRQ - initialize the MBC interrupt controller
 *
 * Called from start_kernel() during boot.  On real hardware this
 * would program the PIC/APIC; on the UPC it simply logs readiness
 * since the BPF runtime delivers interrupts directly.
 */
void __init init_IRQ(void)
{
	pr_info("MBC: IRQ controller initialized (%d vectors)\n", NR_IRQS);
}
