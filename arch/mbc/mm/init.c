// SPDX-License-Identifier: GPL-2.0
/*
 * arch/mbc/mm/init.c - MBC memory initialization (nommu)
 *
 * MBC uses a flat address space with no MMU.  This file
 * sets up the memory map for the kernel's page allocator
 * so that kmalloc / vmalloc / etc. work correctly.
 *
 * RAM layout:
 *   0x00000000 - 0x00FFFFFF  16 MB physical RAM
 *   IVT at 0x00000000 (first 1 KB)
 *   Kernel text/data loaded by bootloader
 *   Free pages managed by the buddy allocator
 */

#include <linux/init.h>
#include <linux/mm.h>
#include <linux/bootmem.h>
#include <linux/kernel.h>

#define MBC_RAM_BASE	0x00000000
#define MBC_RAM_SIZE	(16 * 1024 * 1024)	/* 16 MB */

/*
 * mem_init - hand free pages to the buddy allocator
 *
 * Called once during boot after bootmem has reserved the
 * kernel image and page tables.  We tell the page allocator
 * how much RAM is available.
 */
void __init mem_init(void)
{
	unsigned long total_pages = MBC_RAM_SIZE >> PAGE_SHIFT;

	high_memory = (void *)(MBC_RAM_BASE + MBC_RAM_SIZE);
	totalram_pages_add(total_pages);

	pr_info("MBC: Memory: %luK available\n",
		totalram_pages() << (PAGE_SHIFT - 10));
}

/*
 * paging_init - set up page tables
 *
 * No-op on MBC: there is no MMU, so the entire address
 * space is identity-mapped.
 */
void __init paging_init(void)
{
	/* No paging for nommu — flat address space */
}
