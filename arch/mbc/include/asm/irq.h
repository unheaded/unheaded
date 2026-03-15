/* SPDX-License-Identifier: GPL-2.0 */
/*
 * arch/mbc/include/asm/irq.h - Interrupt definitions for MBC
 *
 * The UPC uses INT instructions for software interrupts.
 * INT 0x80 is the syscall vector (Linux convention).
 * Hardware IRQs start at 0x20 (after CPU exceptions).
 */
#ifndef _ASM_MBC_IRQ_H
#define _ASM_MBC_IRQ_H

#define NR_IRQS			256

/* Hardware IRQ assignments */
#define IRQ_TIMER		0x20
#define IRQ_KEYBOARD		0x21

/* Software interrupt vectors */
#define MBC_SYSCALL_VECTOR	0x80

#endif /* _ASM_MBC_IRQ_H */
