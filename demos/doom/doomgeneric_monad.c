// SPDX-License-Identifier: GPL-3.0-or-later
// doomgeneric_monad.c — Doom platform layer for MBC VM (Monad Bytecode)
//
// Implements the doomgeneric interface using MBC syscalls.
// The MBC VM maps these to BPF operations on the packet circulation ring.
//
// Computermancer: Option C framebuffer strategy — render to RAM,
// DMA-flush to SCREEN_BASE on frame complete. One helper call per frame.

#include "doomgeneric.h"

// Screen buffer at MBC memory-mapped address SCREEN_BASE
// 320x200 8-bit palette indices
// Must match mbc_mmap::SCREEN_BASE in monad-common (0x70000)
#define SCREEN_BASE ((volatile uint8_t*)0x00070000)

// MBC syscall numbers
#define SYS_DRAW_FRAME  0x01
#define SYS_GET_KEY     0x02
#define SYS_GET_TICKS   0x03
#define SYS_SLEEP       0x04

// Static screen buffer — avoids relying on malloc from doomgeneric_Create
// which can fail if Z_Init errors are made non-fatal.
// doomgeneric.c also defines DG_ScreenBuffer; we override with ours.
static uint32_t static_screen_buffer[320 * 200];
pixel_t* DG_ScreenBuffer = (pixel_t*)static_screen_buffer;

// doomgeneric entry points
void doomgeneric_Create(int argc, char **argv);
void doomgeneric_Tick(void);

// Issue an MBC syscall via RISC-V ecall
// The RV32I->MBC translator maps ecall to SYSCALL with imm16 from register
static inline uint32_t mbc_syscall(uint32_t num) {
    register uint32_t a7 asm("a7") = num;
    register uint32_t a0 asm("a0");
    asm volatile("ecall" : "=r"(a0) : "r"(a7));
    return a0;
}

static inline uint32_t mbc_syscall_arg(uint32_t num, uint32_t arg) {
    register uint32_t a7 asm("a7") = num;
    register uint32_t a0 asm("a0") = arg;
    asm volatile("ecall" : "+r"(a0) : "r"(a7));
    return a0;
}

// Simple ARGB to palette conversion
// For now: luminance-based mapping
// TODO: Proper color quantization using Doom's PLAYPAL
static uint8_t argb_to_palette(uint32_t argb) {
    uint8_t r = (argb >> 16) & 0xFF;
    uint8_t g = (argb >> 8) & 0xFF;
    uint8_t b = argb & 0xFF;
    // Simple luminance-based mapping to palette range
    return (uint8_t)(((r * 77 + g * 150 + b * 29) >> 8));
}

// Doom's I_VideoBuffer — palette-indexed pixels rendered by the engine.
extern unsigned char *I_VideoBuffer;

// Static video buffer — ensures I_VideoBuffer is valid even if Z_Malloc fails.
static unsigned char static_ivb[320 * 200];

// Doom globals we need to initialize early
extern int setsizeneeded;
extern int screenvisible;
extern int setblocks;
extern int detailLevel;

// Defined in libc_stubs.c — writes milestone IDs to fixed RAM for post-mortem
extern void debug_breadcrumb(uint32_t milestone);

void DG_Init(void) {
    debug_breadcrumb(0x0010); // DG_Init entered
    // Initialize I_VideoBuffer early — before D_DoomMain's I_InitGraphics.
    if (!I_VideoBuffer) {
        I_VideoBuffer = static_ivb;
    }
    // Force these so the game loop renders even if full init fails:
    screenvisible = 1;   // Enable D_Display in game loop
    setsizeneeded = 1;   // Trigger R_ExecuteSetViewSize → R_InitBuffer
    setblocks = 10;      // Full-screen view (0 = nothing, 10 = full, 11 = full+no status bar)
    detailLevel = 0;     // High detail
    // Clear screen
    for (int i = 0; i < DOOMGENERIC_RESX * DOOMGENERIC_RESY; i++) {
        SCREEN_BASE[i] = 0;
    }
    debug_breadcrumb(0x0011); // DG_Init complete
}

// ylookup table used by renderer — if NULL, fix it
extern unsigned char *ylookup[];

void DG_DrawFrame(void) {
    // Fix ylookup if renderer never initialized it
    if (I_VideoBuffer && ylookup[0] == 0) {
        for (int i = 0; i < DOOMGENERIC_RESY; i++) {
            ylookup[i] = I_VideoBuffer + i * DOOMGENERIC_RESX;
        }
    }
    // Copy palette-indexed pixels directly from I_VideoBuffer to SCREEN_MAP.
    if (I_VideoBuffer) {
        for (int i = 0; i < DOOMGENERIC_RESX * DOOMGENERIC_RESY; i++) {
            SCREEN_BASE[i] = I_VideoBuffer[i];
        }
    }
    // Signal the BPF VM that a frame is ready
    mbc_syscall(SYS_DRAW_FRAME);
}

int DG_GetKey(int* pressed, unsigned char* key) {
    // MBC GET_KEY returns scancode in a0, pressed in a1
    register uint32_t a7 asm("a7") = SYS_GET_KEY;
    register uint32_t a0 asm("a0");
    register uint32_t a1 asm("a1");
    asm volatile("ecall" : "=r"(a0), "=r"(a1) : "r"(a7));

    if (a0 == 0) return 0;  // No key event

    *key = (unsigned char)a0;
    *pressed = (int)a1;
    return 1;
}

void DG_SetWindowTitle(const char* title) {
    // No-op on MBC — no window title support
    (void)title;
}

uint32_t DG_GetTicksMs(void) {
    return mbc_syscall(SYS_GET_TICKS);
}

void DG_SleepMs(uint32_t ms) {
    mbc_syscall_arg(SYS_SLEEP, ms);
}

// Entry point — called from crt0.S
int main(void) {
    debug_breadcrumb(0x0001); // main() entered

    // -mb 2: request 2MB zone memory. Doom shareware needs ~1.5MB.
    // Reduced from 3: bump allocator uses ~24.2MB of 26MB before Z_Init
    // (calloc for lumpinfo, stdc_wad_file_t, argv, etc.), leaving ~1.8MB.
    // With -mb 2, AutoAllocMemory tries 2MB first, succeeds.
    // -iwad: WAD file (fopen stub maps .wad to memory-mapped region)
    char *argv[] = { "doom", "-mb", "2", "-iwad", "doom1.wad", 0 };
    debug_breadcrumb(0x0002); // about to call doomgeneric_Create
    doomgeneric_Create(5, argv);

    debug_breadcrumb(0x0050); // game loop reached!
    while (1) {
        doomgeneric_Tick();
    }

    return 0;
}
