/**
 * doomgeneric_unheaded.c — MBC platform layer for Doom-over-IPv6
 *
 * Implements the doomgeneric platform interface using MBC SYSCALLs.
 * These stubs are compiled with the MBC cross-compile pipeline:
 *   C → RV32I ELF → MBC binary → BPF ROM_MAP → XDP execution
 *
 * SYSCALL interface (from monad-common/src/lib.rs mbc_syscalls):
 *   0x00 SYS_DRAW_FRAME  — Push framebuffer to screen
 *   0x01 SYS_GET_KEY     — Read keyboard state
 *   0x02 SYS_GET_TICKS   — Get virtual time in milliseconds
 *   0x03 SYS_SLEEP       — Sleep for N milliseconds (virtual)
 *
 * Memory layout:
 *   0x000000 - 0x00BFFF  Code + data (ROM)
 *   0x00C000 - 0x01B9FF  Screen buffer (320x200 = 64000 bytes)
 *   0x100000 - 0x1FFFFF  WAD data (mapped from WAD file)
 *   0x200000 - 0x3FFFFF  Heap
 *   0xFFFF0000           Stack top
 *
 * Status: SCAFFOLDED — stubs only, not yet wired to full Doom source.
 */

/* Screen buffer pointer — in full Doom build this comes from doomgeneric.h.
 * For standalone stub testing we define it here. */
unsigned char *DG_ScreenBuffer;

/* MBC SYSCALL numbers */
#define SYS_DRAW_FRAME  0x00
#define SYS_GET_KEY     0x01
#define SYS_GET_TICKS   0x02
#define SYS_SLEEP       0x03

/* Screen dimensions */
#define SCREEN_WIDTH  320
#define SCREEN_HEIGHT 200
#define SCREEN_SIZE   (SCREEN_WIDTH * SCREEN_HEIGHT)
#define SCREEN_BASE   0xC000

/* Virtual time counter (incremented by SYS_GET_TICKS) */
static unsigned int virtual_ticks = 0;

/**
 * MBC SYSCALL inline helper.
 *
 * In the real BPF pipeline, SYSCALL is an MBC opcode (0x40) that the
 * BPF CPU program handles. In the C cross-compile path, we use the
 * RISC-V ECALL instruction which the translator maps to MBC SYSCALL.
 *
 * r0 (a0) = syscall number
 * r1 (a1) = arg1
 * r2 (a2) = arg2
 * Return in r0 (a0)
 */
static inline unsigned int mbc_syscall(unsigned int num, unsigned int arg1, unsigned int arg2) {
    register unsigned int a0 __asm__("a0") = num;
    register unsigned int a1 __asm__("a1") = arg1;
    register unsigned int a2 __asm__("a2") = arg2;
    __asm__ volatile("ecall" : "+r"(a0) : "r"(a1), "r"(a2) : "memory");
    return a0;
}

/**
 * DG_Init — Initialize the Doom platform layer.
 *
 * Sets up the screen buffer pointer to the MBC screen memory region.
 * In the BPF pipeline, SCREEN_BASE maps to the SCREEN_MAP BPF array.
 */
void DG_Init(void) {
    DG_ScreenBuffer = (unsigned char *)SCREEN_BASE;
}

/**
 * DG_DrawFrame — Push the current framebuffer to the display.
 *
 * Fires SYS_DRAW_FRAME syscall which signals the BPF program to
 * make the SCREEN_MAP visible to the doom-bridge for WebSocket streaming.
 */
void DG_DrawFrame(void) {
    mbc_syscall(SYS_DRAW_FRAME, SCREEN_BASE, 0);
}

/**
 * DG_SleepMs — Sleep for the given number of milliseconds.
 *
 * In virtual time mode, this is a no-op (time advances by tick injection).
 * In wall-clock mode, fires SYS_SLEEP which stalls the BPF CPU.
 */
void DG_SleepMs(unsigned int ms) {
    (void)ms;
    /* No-op in virtual time mode — time advances via packet injection */
}

/**
 * DG_GetTicksMs — Get current time in milliseconds.
 *
 * Returns a monotonically increasing virtual time counter.
 * Each call increments by 1ms, giving Doom a consistent time base
 * regardless of actual packet injection rate.
 */
unsigned int DG_GetTicksMs(void) {
    return virtual_ticks++;
}

/**
 * DG_GetKey — Poll for keyboard input.
 *
 * Reads the KBD_MAP register via SYS_GET_KEY syscall.
 * Returns 1 if a key event is available, 0 otherwise.
 *
 * @param pressed  Output: 1 if key pressed, 0 if released
 * @param doomKey  Output: Doom key code
 * @return 1 if event available, 0 if no event
 */
int DG_GetKey(int *pressed, unsigned char *doomKey) {
    unsigned int result = mbc_syscall(SYS_GET_KEY, 0, 0);
    if (result == 0) {
        return 0; /* No key event */
    }
    *pressed = (result >> 8) & 1;
    *doomKey = result & 0xFF;
    return 1;
}

/**
 * DG_SetWindowTitle — Set the window title (no-op in BPF).
 */
void DG_SetWindowTitle(const char *title) {
    (void)title;
}

/**
 * Standalone main — exercises the platform stubs.
 * In the full Doom build, main() comes from i_main.c.
 */
#ifdef STANDALONE_STUBS
void main(void) {
    DG_Init();

    /* Draw one frame */
    DG_DrawFrame();

    /* Read time */
    unsigned int t = DG_GetTicksMs();
    (void)t;

    /* Poll input */
    int pressed;
    unsigned char key;
    DG_GetKey(&pressed, &key);
}
#endif
