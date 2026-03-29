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
    RAM    (rw)  : ORIGIN = 0x00100000, LENGTH = 768K    /* MBC RAM: data + bss */
    SCREEN (rw)  : ORIGIN = 0x00070000, LENGTH = 64000   /* Screen: maps to SCREEN_MAP BPF array */
    HEAP   (rw)  : ORIGIN = 0x001C0000, LENGTH = 10M     /* Heap: 10MB, BEFORE WAD */
    WAD    (r)   : ORIGIN = 0x00C00000, LENGTH = 4M      /* WAD: doom-runner loads here */
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

/* Stack MUST be above WAD end. doom1.wad is ~4,196,020 bytes (ends at ~0x1001014).
 * doom-runner memory.rs defines STACK_TOP = RAM_BASE + RAM_SIZE = 0x1100000.
 * The previous value of 0x1000000 placed the stack INSIDE the WAD region,
 * causing stack frames to corrupt WAD lump data during deep call chains.
 */
_stack_top = 0x01100000; /* matches doom-runner memory.rs STACK_TOP */
