/* SPDX-License-Identifier: GPL-2.0 */
/*
 * arch/mbc/kernel/vmlinux.lds.S - MBC kernel linker script
 *
 * Lays out the kernel image sections for the UPC.  The bootloader
 * loads the kernel at 64 KB (0x00010000) and jumps to _start.
 *
 * Week 8 of S-LINUX: final linker script for a bootable kernel.
 */

ENTRY(_start)

SECTIONS
{
    . = 0x00010000;           /* Kernel loaded at 64 KB */

    /* ── Text (executable code) ────────────────────────── */
    .text : {
        _stext = .;
        *(.text .text.*)
        _etext = .;
    }

    /* ── Read-only data ────────────────────────────────── */
    .rodata : {
        *(.rodata .rodata.*)
    }

    /* ── Read-write data ───────────────────────────────── */
    .data : {
        _sdata = .;
        *(.data .data.*)
        _edata = .;
    }

    /* ── BSS (zero-initialized) ────────────────────────── */
    .bss : {
        __bss_start = .;
        *(.bss .bss.*)
        *(COMMON)
        __bss_end = .;
    }

    _end = .;

    /* ── Init section — freed after boot ───────────────── */
    .init : {
        __init_begin = .;
        *(.init.text)
        *(.init.data)
        *(.initcall*)
        __init_end = .;
    }

    /* ── Syscall table ─────────────────────────────────── */
    .syscall_table : {
        __syscall_table_start = .;
        *(.syscall_table)
        __syscall_table_end = .;
    }

    /* ── Discard debug sections for smaller image ──────── */
    /DISCARD/ : {
        *(.debug*)
        *(.comment)
        *(.note*)
    }
}
