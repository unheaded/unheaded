/* SPDX-License-Identifier: GPL-2.0 */
/*
 * arch/mbc/include/asm/page.h - MBC page size definitions
 *
 * The UPC uses 4KB pages, matching the BPF TLB map entry size
 * and standard Linux page granularity.
 */
#ifndef _ASM_MBC_PAGE_H
#define _ASM_MBC_PAGE_H

#define PAGE_SHIFT	12
#define PAGE_SIZE	(1UL << PAGE_SHIFT)
#define PAGE_MASK	(~(PAGE_SIZE - 1))

#ifndef __ASSEMBLY__

#define clear_page(page)	memset((page), 0, PAGE_SIZE)
#define copy_page(to, from)	memcpy((to), (from), PAGE_SIZE)

typedef unsigned long pte_t;
typedef unsigned long pgd_t;
typedef unsigned long pgprot_t;

#endif /* __ASSEMBLY__ */
#endif /* _ASM_MBC_PAGE_H */
