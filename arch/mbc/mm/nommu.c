// SPDX-License-Identifier: GPL-2.0
/*
 * arch/mbc/mm/nommu.c - MBC nommu memory operations
 *
 * Flat memory model — no virtual address translation.
 * All addresses are physical addresses into RAM_MAP.
 *
 * Week 5 of S-LINUX: nommu memory management.
 */

#include <linux/mm.h>
#include <linux/mman.h>
#include <linux/slab.h>

/* Memory regions for the flat address space */
#define MBC_KERNEL_START	0x00000000
#define MBC_KERNEL_END		0x00100000	/* 1MB for kernel */
#define MBC_USER_START		0x00100000
#define MBC_USER_END		0x00F00000	/* 14MB for user */
#define MBC_STACK_TOP		0x00F00000
#define MBC_RAMDISK_START	0x00800000	/* 8MB offset (RAMDISK_BASE_WORD * 4) */

/* Simple bump allocator for nommu mmap */
static unsigned long mbc_brk_current = MBC_USER_START;

/*
 * arch_do_brk - brk() implementation for nommu
 * @addr: current break address
 * @len:  number of bytes to extend
 *
 * Extends the process data segment.  On nommu there is no
 * page table to update — just advance the watermark and
 * check the limit.
 */
unsigned long arch_do_brk(unsigned long addr, unsigned long len)
{
	unsigned long new_brk = addr + len;

	if (new_brk > MBC_USER_END)
		return -ENOMEM;

	if (new_brk > mbc_brk_current)
		mbc_brk_current = new_brk;

	return addr;
}

/*
 * arch_get_unmapped_area - nommu mmap allocation
 * @filp:  file being mapped (may be NULL for anonymous)
 * @addr:  hint address (ignored)
 * @len:   mapping length
 * @pgoff: page offset (ignored)
 * @flags: MAP_* flags (ignored)
 *
 * Returns a page-aligned region from the flat user address
 * space.  This is a simple bump allocator — sufficient for
 * initial boot and the bFLT loader.
 */
unsigned long arch_get_unmapped_area(struct file *filp, unsigned long addr,
				     unsigned long len, unsigned long pgoff,
				     unsigned long flags)
{
	unsigned long result = mbc_brk_current;

	/* Align to page boundary */
	result = PAGE_ALIGN(result);

	if (result + len > MBC_USER_END)
		return -ENOMEM;

	mbc_brk_current = result + len;
	return result;
}
