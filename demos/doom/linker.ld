/* MBC Memory Layout for Doom-over-IPv6
 *
 * This layout maps to BPF maps managed by doom-loader:
 *   ROM_MAP  → .text + .rodata (code)
 *   RAM_MAP  → .data + .bss + heap
 *   SCREEN   → framebuffer at fixed address (320x200)
 *   WAD      → memory-mapped doom1.wad
 *
 * Computermancer note: Double-buffer framebuffer (option C).
 * Game renders to RAM, DMA-flush to SCREEN on frame complete.
 */
MEMORY
{
    ROM    (rx)  : ORIGIN = 0x00000000, LENGTH = 1M      /* MBC ROM: code + rodata */
    RAM    (rw)  : ORIGIN = 0x00100000, LENGTH = 768K    /* MBC RAM: data + bss + stack */
    SCREEN (rw)  : ORIGIN = 0x00070000, LENGTH = 64000   /* Screen: maps to SCREEN_MAP BPF array */
    WAD    (r)   : ORIGIN = 0x00800000, LENGTH = 4M      /* WAD data (memory-mapped) */
    HEAP   (rw)  : ORIGIN = 0x00C00000, LENGTH = 16M     /* Heap: AFTER WAD, no overlap */
}

ENTRY(_start)

SECTIONS
{
    .text : {
        *(.text.startup)
        *(.text*)
    } > ROM

    .rodata : {
        *(.rodata*)
    } > ROM

    .data : {
        *(.data*)
    } > RAM

    .bss : {
        __bss_start = .;
        *(.bss*)
        *(.sbss*)
        *(COMMON)
        __bss_end = .;
    } > RAM

    .heap (NOLOAD) : {
        __heap_start = .;
        . = . + LENGTH(HEAP);
        __heap_end = .;
    } > HEAP
}

_stack_top = ORIGIN(HEAP) + LENGTH(HEAP);
