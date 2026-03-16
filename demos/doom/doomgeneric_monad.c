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
#define SCREEN_BASE ((volatile uint8_t*)0x007C0000)

// MBC syscall numbers
#define SYS_DRAW_FRAME  0x01
#define SYS_GET_KEY     0x02
#define SYS_GET_TICKS   0x03
#define SYS_SLEEP       0x04

// doomgeneric.c defines DG_ScreenBuffer and allocates it via malloc in doomgeneric_Create.
// We reference it here for the framebuffer flush.
extern pixel_t* DG_ScreenBuffer;

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

void DG_Init(void) {
    // Clear screen
    for (int i = 0; i < DOOMGENERIC_RESX * DOOMGENERIC_RESY; i++) {
        SCREEN_BASE[i] = 0;
    }
}

void DG_DrawFrame(void) {
    // Computermancer: scanline batching — convert and write 200 scanlines
    // instead of 64K individual pixel writes
    uint32_t *buf = (uint32_t *)DG_ScreenBuffer;
    for (int i = 0; i < DOOMGENERIC_RESX * DOOMGENERIC_RESY; i++) {
        SCREEN_BASE[i] = argb_to_palette(buf[i]);
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
    // -mb 4: request 4MB zone memory (fits in our 6MB heap)
    // -iwad: WAD file (fopen stub maps .wad to memory-mapped region)
    char *argv[] = { "doom", "-mb", "4", "-iwad", "doom1.wad", 0 };
    doomgeneric_Create(5, argv);

    while (1) {
        doomgeneric_Tick();
    }

    return 0;
}
