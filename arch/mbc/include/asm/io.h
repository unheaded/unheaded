/* SPDX-License-Identifier: GPL-2.0 */
/*
 * arch/mbc/include/asm/io.h - I/O operations for MBC
 *
 * The UPC is a nommu architecture with memory-mapped I/O.
 * These are direct memory access macros — no port I/O
 * (NO_IOPORT_MAP is selected in Kconfig).
 */
#ifndef _ASM_MBC_IO_H
#define _ASM_MBC_IO_H

#define readb(addr)		(*(volatile unsigned char *)(addr))
#define readw(addr)		(*(volatile unsigned short *)(addr))
#define readl(addr)		(*(volatile unsigned long *)(addr))

#define writeb(b, addr)		(*(volatile unsigned char *)(addr) = (b))
#define writew(b, addr)		(*(volatile unsigned short *)(addr) = (b))
#define writel(b, addr)		(*(volatile unsigned long *)(addr) = (b))

/* No port I/O on MBC — all I/O is memory-mapped */

#endif /* _ASM_MBC_IO_H */
