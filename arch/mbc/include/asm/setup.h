/* SPDX-License-Identifier: GPL-2.0 */
/*
 * arch/mbc/include/asm/setup.h - Boot parameters for MBC
 *
 * The UPC bootloader places parameters at a fixed address.
 * BOOT_MAGIC ("UNHD") is verified by setup_arch() before
 * trusting the parameter block.
 */
#ifndef _ASM_MBC_SETUP_H
#define _ASM_MBC_SETUP_H

#define BOOT_PARAMS_ADDR	0x0100
#define BOOT_MAGIC		0x554E4844	/* "UNHD" in ASCII */

#define COMMAND_LINE_SIZE	256

#ifndef __ASSEMBLY__

struct mbc_boot_params {
	unsigned long magic;		/* must be BOOT_MAGIC */
	unsigned long mem_size;		/* total RAM in bytes */
	unsigned long initrd_start;	/* initrd physical address */
	unsigned long initrd_size;	/* initrd size in bytes */
	char cmdline[COMMAND_LINE_SIZE];
};

#endif /* __ASSEMBLY__ */
#endif /* _ASM_MBC_SETUP_H */
