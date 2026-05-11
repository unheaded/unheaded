/* SPDX-License-Identifier: GPL-3.0-or-later
 *
 * start.c — UPC stage-1 boot stub C source (compiled to MBC bytecode).
 *
 * ASCEND-LINUX Phase 2 (task #63 closure scaffolding).
 *
 * This file is the C source that, when translated through rv32i_to_mbc,
 * produces upc-bootstub.mbc — the MBC bytecode that runs first when
 * cmd/upc-bootctl boots a uClinux or full-Linux kernel into the UPC.
 *
 * Contract per crates/upc-bootstub/src/lib.rs §"Phase 2 contract":
 *   1. Verify BootParams magic + version (fail-fast on mismatch).
 *   2. Zero [bss_start, bss_end) byte range from BootParamsV2.
 *   3. (Phase 2.4) CPIO-unpack initramfs to known addresses; for now
 *      assume the loader pre-unpacked it.
 *   4. Set MEPC = kernel_entry, MSTATUS.MPP = S-mode.
 *   5. MRET — kernel begins at supervisor privilege.
 *
 * On any verify failure: emit "BOOT FAIL: <reason>\n" via MMIO 0xC001
 * and HALT (ebreak -> MBC HALT 0xFF).
 *
 * Status: SCAFFOLDING. The function bodies are stubs; Phase 2 day-1
 * implementation fills them in along with the Makefile.bootstub build
 * rule that mirrors crates/xv6-mbc/adapters/Makefile.mbc.
 */

#include <stdint.h>

/* ── BootParamsV2 layout (must match docs/doom/UPC_BOOT_PROTOCOL_V2.md) ─── */

#define BOOT_PARAMS_ADDR  ((volatile struct boot_params_v2 *)0x00000100)
#define BOOT_MAGIC        0x554E4844u  /* canonical hex per ADR-072 */
#define BOOT_VERSION_V2   2u

struct boot_params_v2 {
    uint32_t magic;
    uint32_t version;
    uint32_t memory_size;
    uint32_t ramdisk_addr;
    uint32_t ramdisk_size;
    uint32_t kernel_addr;
    uint32_t kernel_size;
    uint32_t boot_args_addr;
    uint32_t boot_args_len;
    uint32_t num_cpus;
    uint32_t tick_rate_hz;
    uint32_t bss_start;
    uint32_t bss_end;
    uint32_t initrd2_addr;
    uint32_t cmd_line_args_ptr;
    uint32_t boot_random_seed;
    uint32_t reserved[48];
};

/* ── CSR memory-mapped region (ADR-067 Decision 2) ────────────────────── */

#define CSR_BASE      0x0000F000u
#define CSR_REG(idx)  (*(volatile uint32_t *)(CSR_BASE + ((idx) * 4)))
#define CSR_MEPC      0x341
#define CSR_MSTATUS   0x300
#define MSTATUS_MPP_S (1u << 11)  /* MPP = 0b01 (S-mode) */

/* ── MMIO TTY (UPC L4f convention) ────────────────────────────────────── */

#define UPC_TTY_DATA_ADDR  0xC001u
#define UPC_TTY_REG        (*(volatile uint32_t *)(UPC_TTY_DATA_ADDR))

static inline void mmio_putc(char c) {
    UPC_TTY_REG = (uint32_t)c;
}

static void mmio_puts(const char *s) {
    while (*s) {
        mmio_putc(*s++);
    }
}

/* ── BSS zero-fill ─────────────────────────────────────────────────────── */

static void zero_bss(uint32_t bss_start, uint32_t bss_end) {
    /* Write zeros in 4-byte chunks. Both bss_start and bss_end MUST be
     * 4-byte aligned; the linker-script PROVIDE() statements in the
     * kernel ELF guarantee this for every Linux config we target. */
    volatile uint32_t *p = (volatile uint32_t *)bss_start;
    volatile uint32_t *end = (volatile uint32_t *)bss_end;
    while (p < end) {
        *p++ = 0;
    }
}

/* ── Boot stub entry ───────────────────────────────────────────────────── */

void start(void) {
    volatile struct boot_params_v2 *bp = BOOT_PARAMS_ADDR;

    /* Step 1: verify boot params. */
    if (bp->magic != BOOT_MAGIC) {
        mmio_puts("BOOT FAIL: bad magic\n");
        __asm__ volatile ("ebreak");
        while (1) {}
    }
    if (bp->version != BOOT_VERSION_V2) {
        mmio_puts("BOOT FAIL: bad version\n");
        __asm__ volatile ("ebreak");
        while (1) {}
    }

    /* Step 2: zero BSS. */
    if (bp->bss_end > bp->bss_start) {
        zero_bss(bp->bss_start, bp->bss_end);
    }

    /* Step 3: initramfs handoff is loader-side for Phase 2 day-1.
     * Phase 2.4 will move CPIO unpacking here. */

    /* Step 4: set MEPC + MSTATUS.MPP for S-mode entry. */
    CSR_REG(CSR_MEPC) = bp->kernel_addr;
    uint32_t mstatus = CSR_REG(CSR_MSTATUS);
    mstatus &= ~(0x3u << 11);   /* clear MPP field */
    mstatus |= MSTATUS_MPP_S;   /* set MPP = 0b01 (S-mode) */
    CSR_REG(CSR_MSTATUS) = mstatus;

    mmio_puts("upc-bootstub: handoff to kernel\n");

    /* Step 5: MRET — kernel takes over at supervisor privilege. */
    __asm__ volatile ("mret");

    /* Should not reach here; if MRET fall-through, halt with diagnostic. */
    mmio_puts("BOOT FAIL: MRET fall-through\n");
    __asm__ volatile ("ebreak");
    while (1) {}
}
