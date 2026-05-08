/* SPDX-License-Identifier: GPL-3.0-or-later
 *
 * kernel-mbc.ld — Linker script for xv6 on UPC.
 * ASCEND-LINUX Phase 1.1.
 *
 * Memory layout per docs/doom/UPC_BOOT_PROTOCOL_V2.md:
 *   0x00000-0x003FF  IVT (256 vectors × 4 bytes)
 *   0x00100-0x001FF  BootParams v2 (256 bytes)
 *   0x00200-0x003FF  Kernel command line
 *   0x00400-0xEFFF   Reserved / kernel scratch
 *   0xF000-0xF0FF    CSR memory region (64 × 4 bytes)
 *   0x10000-0x1FFFF  Stage-1 boot stub (start_mbc.c lives here)
 *   0x20000+         Kernel image (.text, .rodata)
 *   ramdisk at 0x800000+ (per BootParams.ramdisk_addr; bootloader-placed)
 *   0x03F00000       Initial stack top (grows down)
 *
 * MEMORY regions correspond to BPF maps in doom-runner:
 *   STAGE1 → ROM_MAP slots 0-0x3FFF (boot stub)
 *   ROM    → ROM_MAP slots 0x4000+   (kernel code)
 *   RAM    → RAM_MAP at virtual 0x00100000+
 */

OUTPUT_ARCH( "riscv" )
ENTRY( start )

MEMORY
{
    /* Boot regions (ROM_MAP slots 0-0x3FFF, byte addresses 0-0xFFFF) */
    BOOT   (rx)  : ORIGIN = 0x00000000, LENGTH = 64K     /* IVT + BootParams + boot stub */

    /* Kernel image region (ROM_MAP slots 0x8000+) */
    KERN   (rx)  : ORIGIN = 0x00020000, LENGTH = 1024K   /* xv6 .text + .rodata */

    /* Working memory (RAM_MAP) */
    RAM    (rw)  : ORIGIN = 0x00100000, LENGTH = 16M     /* xv6 .data + .bss + early heap */

    /* Initramfs is bootloader-placed at 0x800000; not in linker scope. */
}

SECTIONS
{
    /* Stage 1 — start_mbc.c at 0x10000 entry. */
    .stage1 0x00010000 : {
        *(.text.start_mbc)
        *(.text.boot)
    } > BOOT

    /* xv6 main kernel — .text + .rodata. */
    .text 0x00020000 : {
        *(.text)
        *(.text.*)
        . = ALIGN(0x100);
        _trampoline = .;
        *(trampsec)
        . = ALIGN(0x100);
        PROVIDE(etext = .);
    } > KERN

    .rodata : {
        . = ALIGN(16);
        *(.srodata .srodata.*)
        . = ALIGN(16);
        *(.rodata .rodata.*)
    } > KERN

    /* Working memory: .data + .bss + heap. */
    .data : {
        . = ALIGN(16);
        PROVIDE(_data_start = .);
        *(.sdata .sdata.*)
        *(.data .data.*)
        PROVIDE(_data_end = .);
    } > RAM

    .bss : {
        . = ALIGN(16);
        PROVIDE(_bss_start = .);
        *(.sbss .sbss.*)
        *(.bss .bss.*)
        *(COMMON)
        PROVIDE(_bss_end = .);
    } > RAM

    /* Stack grows down from end of RAM region. xv6's main() relies on
     * cpus[0].stack[] which is statically allocated in .bss; we don't
     * need a separate stack symbol. */

    /DISCARD/ : { *(.eh_frame) *(.eh_frame.*) *(.note.*) *(.comment) }
}
