/* MBC Memory Layout for Doom */
MEMORY
{
    ROM (rx)  : ORIGIN = 0x00000000, LENGTH = 256K   /* MBC ROM: 64K instructions */
    RAM (rw)  : ORIGIN = 0x00000000, LENGTH = 48K    /* MBC RAM: 0x0000-0xBFFF */
    SCREEN(rw): ORIGIN = 0x0000C000, LENGTH = 64000  /* Screen: 320x200 */
    WAD (r)   : ORIGIN = 0x00010000, LENGTH = 4M     /* WAD data */
}

ENTRY(_start)

SECTIONS
{
    .text : { *(.text*) } > ROM
    .rodata : { *(.rodata*) } > ROM
    .data : { *(.data*) } > RAM
    .bss : { *(.bss*) *(COMMON) } > RAM
}
