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

void I_InitGraphics(void)
{
    // Allocate the main screen buffer (320x200 bytes)
    // id DOOM renders here, then we copy to SCREEN_BASE
    screens[0] = (unsigned char *)malloc(SCREEN_SIZE);
    if (screens[0])
        memset(screens[0], 0, SCREEN_SIZE);
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
    // Poll MBC keyboard via syscall
    // Returns 0 if no key, or packed key event
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

    unsigned int key = result & 0xFF;
    // Simple scancode to Doom key mapping
    switch (key) {
    case 0x48: event.data1 = KEY_UPARROW; break;
    case 0x50: event.data1 = KEY_DOWNARROW; break;
    case 0x4B: event.data1 = KEY_LEFTARROW; break;
    case 0x4D: event.data1 = KEY_RIGHTARROW; break;
    case 0x01: event.data1 = KEY_ESCAPE; break;
    case 0x1C: event.data1 = KEY_ENTER; break;
    case 0x0F: event.data1 = KEY_TAB; break;
    case 0x39: event.data1 = ' '; break;
    case 0x1D: event.data1 = KEY_RCTRL; break;   // fire
    case 0x38: event.data1 = KEY_RALT; break;     // strafe
    case 0x36: event.data1 = KEY_RSHIFT; break;   // run
    default:
        // Pass through printable ASCII
        if (key >= 0x20 && key <= 0x7E)
            event.data1 = key;
        else
            return; // unknown key, ignore
        break;
    }
    event.data2 = 0;
    event.data3 = 0;

    D_PostEvent(&event);
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
    // Copy rendered screen to MBC framebuffer via memcpy (word stores).
    // Word stores go to RAM_MAP. The bridge reads from RAM_MAP at SCREEN_BASE.
    // This is 4x faster than byte stores (16K words vs 64K bytes).
    if (screens[0])
        memcpy((void *)SCREEN_BASE, screens[0], SCREEN_SIZE);

    // Signal MBC to present the frame
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
    // In the future, could write palette to a MBC palette register.
    // For now, the doom-runner handles palette lookup on the host side.
    (void)palette;
}
