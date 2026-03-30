// SPDX-License-Identifier: GPL-3.0-or-later
// i_video_mbc.c — MBC framebuffer video for id DOOM
//
// Replaces linuxdoom-1.10/i_video.c (X11) with MBC memory-mapped framebuffer.
// Screen is 320x200 8-bit indexed color at SCREEN_BASE (0x170000).

#include <stdlib.h>
#include <string.h>

#include "doomstat.h"
#include "i_system.h"
#include "v_video.h"
#include "m_argv.h"
#include "d_main.h"
#include "doomdef.h"

// MBC memory-mapped screen address (matches doom-runner memory.rs)
#define SCREEN_BASE 0x00070000
#define SCREEN_SIZE (SCREENWIDTH * SCREENHEIGHT)  // 320 * 200 = 64000

// MBC syscall for frame presentation
#define SYS_DRAW_FRAME 0x01
#define SYS_GET_KEY    0x02

static inline unsigned int mbc_syscall(unsigned int num) {
    register unsigned int a0 __asm__("a0") = num;
    register unsigned int a7 __asm__("a7") = num;
    __asm__ volatile("ecall" : "+r"(a0) : "r"(a7) : "memory");
    return a0;
}

// ============================================================
// I_InitGraphics — allocate screen buffer
// ============================================================

// Fixed screen buffer address — bridge reads directly from here.
// No memcpy needed: Doom renders into screens[0] which IS at SCREEN_BASE.
// This saves 16K word stores per frame (~35% fps boost).
#define PALETTE_ADDR ((volatile unsigned char *)0x00060000) // 768 bytes below screen

void I_InitGraphics(void)
{
    // Point screens[0] directly at SCREEN_BASE in MBC address space.
    // Doom renders directly into the screen buffer that the bridge reads.
    screens[0] = (unsigned char *)SCREEN_BASE;
    memset(screens[0], 0, SCREEN_SIZE);

    // Doom uses screens[1-4] for status bar, wipe effects, temp buffers.
    // If not allocated, V_CopyRect and wipe code write to NULL → corruption.
    for (int i = 1; i < 5; i++) {
        screens[i] = (unsigned char *)malloc(SCREEN_SIZE);
        if (screens[i])
            memset(screens[i], 0, SCREEN_SIZE);
    }
}

// ============================================================
// I_ShutdownGraphics — no-op
// ============================================================

void I_ShutdownGraphics(void)
{
}

// ============================================================
// I_StartFrame — called at frame start, no-op
// ============================================================

void I_StartFrame(void)
{
}

// ============================================================
// I_StartTic — poll MBC keyboard, post events
// ============================================================

// Doom key definitions from doomdef.h
#define KEY_RIGHTARROW  0xae
#define KEY_LEFTARROW   0xac
#define KEY_UPARROW     0xad
#define KEY_DOWNARROW   0xaf
#define KEY_ESCAPE      27
#define KEY_ENTER       13
#define KEY_TAB         9
#define KEY_F1          (0x80+0x3b)
#define KEY_F2          (0x80+0x3c)
#define KEY_F3          (0x80+0x3d)
#define KEY_F4          (0x80+0x3e)
#define KEY_F5          (0x80+0x3f)
#define KEY_F6          (0x80+0x40)
#define KEY_F7          (0x80+0x41)
#define KEY_F8          (0x80+0x42)
#define KEY_F9          (0x80+0x43)
#define KEY_F10         (0x80+0x44)
#define KEY_F11         (0x80+0x57)
#define KEY_F12         (0x80+0x58)
#define KEY_BACKSPACE   127
#define KEY_PAUSE       0xff
#define KEY_EQUALS      0x3d
#define KEY_MINUS       0x2d
#define KEY_RSHIFT      (0x80+0x36)
#define KEY_RCTRL       (0x80+0x1d)
#define KEY_RALT        (0x80+0x38)

void I_StartTic(void)
{
    // Drain ALL pending key events (not just one).
    // Doom calls I_StartTic once per tic (35 Hz). If keys queue up,
    // we need to process them all to prevent input lag and flickering.
    for (int poll = 0; poll < 8; poll++) {
    unsigned int result = mbc_syscall(SYS_GET_KEY);
    if (result == 0)
        return;

    event_t event;
    // Bit 31: 1=keydown, 0=keyup
    // Bits 0-7: key scancode
    if (result & 0x80000000)
        event.type = ev_keydown;
    else
        event.type = ev_keyup;

    unsigned int key = result & 0xFFFF;
    // JavaScript keyCode → Doom key mapping
    // Browser sends e.keyCode values, not PC scancodes
    switch (key) {
    case 38: event.data1 = KEY_UPARROW; break;    // ArrowUp
    case 40: event.data1 = KEY_DOWNARROW; break;   // ArrowDown
    case 37: event.data1 = KEY_LEFTARROW; break;   // ArrowLeft
    case 39: event.data1 = KEY_RIGHTARROW; break;  // ArrowRight
    // WASD
    case 87: event.data1 = KEY_UPARROW; break;     // W = forward
    case 83: event.data1 = KEY_DOWNARROW; break;    // S = backward
    case 65: event.data1 = KEY_LEFTARROW; break;    // A = turn left
    case 68: event.data1 = KEY_RIGHTARROW; break;   // D = turn right
    case 27: event.data1 = KEY_ESCAPE; break;      // Escape
    case 13: event.data1 = KEY_ENTER; break;       // Enter
    case 9:  event.data1 = KEY_TAB; break;         // Tab
    case 32: event.data1 = ' '; break;             // Space
    case 17: event.data1 = KEY_RCTRL; break;       // Ctrl (fire)
    case 18: event.data1 = KEY_RALT; break;        // Alt (strafe)
    case 16: event.data1 = KEY_RSHIFT; break;      // Shift (run)
    case 188: event.data1 = ','; break;            // < (strafe left)
    case 190: event.data1 = '.'; break;            // > (strafe right)
    // Number keys for weapon select
    case 49: event.data1 = '1'; break;
    case 50: event.data1 = '2'; break;
    case 51: event.data1 = '3'; break;
    case 52: event.data1 = '4'; break;
    case 53: event.data1 = '5'; break;
    case 54: event.data1 = '6'; break;
    case 55: event.data1 = '7'; break;
    // F-keys
    case 112: event.data1 = KEY_F1; break;
    case 113: event.data1 = KEY_F2; break;
    case 114: event.data1 = KEY_F3; break;
    case 115: event.data1 = KEY_F4; break;
    case 116: event.data1 = KEY_F5; break;
    case 117: event.data1 = KEY_F6; break;
    case 118: event.data1 = KEY_F7; break;
    case 119: event.data1 = KEY_F8; break;
    case 120: event.data1 = KEY_F9; break;
    case 121: event.data1 = KEY_F10; break;
    case 122: event.data1 = KEY_F11; break;
    case 123: event.data1 = KEY_F12; break;
    case 189: event.data1 = KEY_MINUS; break;      // -
    case 187: event.data1 = KEY_EQUALS; break;     // =
    case 8:   event.data1 = KEY_BACKSPACE; break;  // Backspace
    case 19:  event.data1 = KEY_PAUSE; break;      // Pause
    case 76:  event.data1 = KEY_RCTRL; break;      // L = fire
    default:
        // Pass through letter keys as lowercase
        if (key >= 65 && key <= 90)
            event.data1 = key + 32; // A-Z → a-z
        else if (key >= 48 && key <= 57)
            event.data1 = key; // 0-9
        else
            continue; // unknown key, skip to next event
        break;
    }
    event.data2 = 0;
    event.data3 = 0;

    D_PostEvent(&event);
    } // end for(poll)
}

// ============================================================
// I_UpdateNoBlit — no-op
// ============================================================

void I_UpdateNoBlit(void)
{
}

// ============================================================
// I_FinishUpdate — copy framebuffer to SCREEN_BASE, signal draw
// ============================================================

void I_FinishUpdate(void)
{
    // screens[0] IS SCREEN_BASE — no copy needed!
    // Doom renders directly into the buffer the bridge reads.
    // Just signal frame ready.
    mbc_syscall(SYS_DRAW_FRAME);
}

// ============================================================
// I_ReadScreen — copy screen contents
// ============================================================

void I_ReadScreen(byte *scr)
{
    memcpy(scr, screens[0], SCREEN_SIZE);
}

// ============================================================
// I_SetPalette — store palette (no-op for now, 8-bit indexed)
// ============================================================

void I_SetPalette(byte *palette)
{
    // Write 768 bytes (256 × RGB) to PALETTE_ADDR in RAM_MAP.
    // The bridge reads this to apply correct Doom colors.
    volatile unsigned char *dst = PALETTE_ADDR;
    for (int i = 0; i < 768; i++) {
        dst[i] = palette[i];
    }
}
