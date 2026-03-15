// SPDX-License-Identifier: GPL-2.0
/*
 * arch/mbc/kernel/setup.c - MBC architecture setup
 *
 * Called early in boot to configure architecture-specific state.
 * Reads boot parameters from the UPC bootloader, initializes
 * memory regions, sets up initrd, and prints the UPC banner.
 */

#include <linux/init.h>
#include <linux/kernel.h>
#include <linux/string.h>

#include <asm/setup.h>
#include <asm/unistd.h>

/* Defined in initramfs.c */
extern void setup_initrd(struct mbc_boot_params *params);

static struct mbc_boot_params *boot_params;

void __init setup_arch(char **cmdline_p)
{
	boot_params = (struct mbc_boot_params *)BOOT_PARAMS_ADDR;

	pr_info("UPC: Unheaded Protocol Computer\n");
	pr_info("UPC: MBC ISA, %d syscalls, nommu\n", __NR_syscalls);

	if (boot_params->magic == BOOT_MAGIC) {
		pr_info("UPC: %lu KB RAM detected\n",
			boot_params->mem_size / 1024);
		strlcpy(boot_command_line, boot_params->cmdline,
			COMMAND_LINE_SIZE);

		/* Set up initrd if the bootloader provided one */
		setup_initrd(boot_params);
	} else {
		pr_warn("UPC: no valid boot params (magic=%08lx)\n",
			boot_params->magic);
	}

	*cmdline_p = boot_command_line;
}
