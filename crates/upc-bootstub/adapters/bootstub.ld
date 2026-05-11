/* SPDX-License-Identifier: GPL-3.0-or-later
 *
 * bootstub.ld — Linker script for upc-bootstub.
 *
 * Layout: stage-1 stub lives at byte 0x10000 (matches the slot xv6's
 * start_mbc.c occupies for the Phase 1.1 boot path). The MBC image
 * produced from this linkage is loaded by cmd/upc-bootctl into ROM_MAP
 * starting at the byte address; the eBPF interpreter sets initial CPU.PC
 * = 0x4000 (= byte addr / 4) and the stub takes over.
 *
 * Per crates/upc-bootstub/src/lib.rs constants:
 *   BOOTSTUB_LOAD_ADDR = 0x00010000
 *   BOOTSTUB_ENTRY_PC  = 0x4000
 */

OUTPUT_ARCH( "riscv" )
ENTRY( start )

MEMORY
{
    /* Stage-1 region — same as xv6's .stage1 in kernel-mbc.ld. 64 KB
     * gives plenty of headroom for the verify+zero+MRET stub. */
    BOOT (rx) : ORIGIN = 0x00010000, LENGTH = 64K
}

SECTIONS
{
    .text 0x00010000 : {
        *(.text)
        *(.text.*)
    } > BOOT

    .rodata : {
        . = ALIGN(16);
        *(.srodata .srodata.*)
        . = ALIGN(16);
        *(.rodata .rodata.*)
    } > BOOT

    /* The bootstub does NOT have its own .data or .bss — every
     * mutable byte it touches lives in either RAM_MAP (BootParams +
     * scratch) or the CSR region. If a future feature needs them,
     * add the sections here AND update upc-bootctl's loader to
     * mirror them into RAM_MAP at the correct VMA addresses. */

    /DISCARD/ : { *(.eh_frame) *(.eh_frame.*) *(.note.*) *(.comment) }
}
