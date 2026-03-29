/* MBC Memory Layout for Doom-over-IPv6
 *
 * This layout maps to BPF maps managed by doom-runner:
 *   ROM_MAP  -> .text + .rodata (code)
 *   RAM_MAP  -> .data + .bss + heap
 *   SCREEN   -> framebuffer at fixed address (320x200)
 *   WAD      -> memory-mapped DOOM.WAD (retail, 12,408,292 bytes)
 *
 * Computermancer note: Double-buffer framebuffer (option C).
 * Game renders to RAM, DMA-flush to SCREEN on frame complete.
 */
MEMORY
{
    ROM    (rx)  : ORIGIN = 0x00000000, LENGTH = 1M      /* MBC ROM: code + rodata */
    RAM    (rw)  : ORIGIN = 0x00100000, LENGTH = 768K    /* MBC RAM: data + bss */
    SCREEN (rw)  : ORIGIN = 0x00170000, LENGTH = 64000   /* Screen: maps to SCREEN_MAP BPF array */
    HEAP   (rw)  : ORIGIN = 0x001C0000, LENGTH = 26M     /* Heap: 26MB (~2x Doom's needs, JVM-style generous) */
    WAD    (r)   : ORIGIN = 0x01C00000, LENGTH = 16M     /* WAD: doom-runner loads retail DOOM.WAD here */
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
        *(.sdata*)
    } > RAM

    .srodata : {
        *(.srodata*)
    } > ROM

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

/* Stack at 0x3100000 -- above WAD end. Matches doom-runner memory.rs. */
_stack_top = 0x03100000;
