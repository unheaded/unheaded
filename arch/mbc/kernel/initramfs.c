// SPDX-License-Identifier: GPL-2.0
/*
 * arch/mbc/kernel/initramfs.c - MBC initramfs support
 *
 * Loads an initramfs/initrd from the ramdisk into memory.
 * The boot loader places the initramfs at RAMDISK_START.
 * Linux unpacks it as the initial root filesystem.
 *
 * Week 6 of S-LINUX: root filesystem mount.
 */

#include <linux/init.h>
#include <linux/initrd.h>

#include <asm/setup.h>

/*
 * setup_initrd - set up initrd location from boot params
 * @params: boot parameter block placed by the UPC bootloader
 *
 * If the bootloader provided a non-zero ramdisk size, record
 * the start/end addresses so the kernel's initrd unpacker can
 * find the image during rootfs population.
 */
void __init setup_initrd(struct mbc_boot_params *params)
{
	if (params->initrd_size > 0) {
		initrd_start = params->initrd_start;
		initrd_end = params->initrd_start + params->initrd_size;
		pr_info("MBC: initrd at %08lx-%08lx (%lu bytes)\n",
			initrd_start, initrd_end,
			initrd_end - initrd_start);
	}
}
