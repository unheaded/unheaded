/* Minimal linker script for MBC cross-compilation.
 *
 * Produces a flat binary with .text at address 0, followed by .rodata,
 * .data, and .bss. Stack grows down from _stack_top.
 *
 * This script is used by both test programs and the doomgeneric port.
 */
ENTRY(_start)

SECTIONS {
    . = 0x0;

    .text : {
        *(.text.startup)
        *(.text*)
    }

    .rodata : {
        *(.rodata*)
        *(.srodata*)
    }

    .data : {
        *(.data*)
        *(.sdata*)
    }

    .bss (NOLOAD) : {
        __bss_start = .;
        *(.bss*)
        *(.sbss*)
        *(COMMON)
        __bss_end = .;
    }

    _stack_top = 0x10000;
}
