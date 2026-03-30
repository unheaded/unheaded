// SPDX-License-Identifier: GPL-3.0-or-later
// i_system_mbc.c — MBC platform layer for id DOOM i_system
//
// Replaces linuxdoom-1.10/i_system.c (which uses gettimeofday, exit, etc.)
// with MBC syscalls and bare-metal equivalents.

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdarg.h>

#include "doomdef.h"
#include "m_misc.h"
#include "i_video.h"
#include "i_sound.h"
#include "d_net.h"
#include "g_game.h"

#include "i_system.h"

// ============================================================
// MBC syscall interface
// ============================================================

#define SYS_HALT        0x00
#define SYS_DRAW_FRAME  0x01
#define SYS_GET_KEY     0x02
#define SYS_GET_TICKS   0x03
#define SYS_SLEEP       0x04

static inline unsigned int mbc_syscall(unsigned int num) {
    register unsigned int a0 __asm__("a0") = num;
    register unsigned int a7 __asm__("a7") = num;
    __asm__ volatile("ecall" : "+r"(a0) : "r"(a7) : "memory");
    return a0;
}

static inline unsigned int mbc_syscall_arg(unsigned int num, unsigned int arg) {
    register unsigned int a0 __asm__("a0") = num;
    register unsigned int a1 __asm__("a1") = arg;
    register unsigned int a7 __asm__("a7") = num;
    __asm__ volatile("ecall" : "+r"(a0) : "r"(a1), "r"(a7) : "memory");
    return a0;
}

// ============================================================
// Debug output (to fixed RAM region for post-mortem via bpftool)
// ============================================================

#define DEBUG_MSG_ADDR ((volatile char *)0x03200000)
#define DEBUG_MSG_MAX  256

static void debug_write(const char *s) {
    volatile char *dst = DEBUG_MSG_ADDR;
    int i = 0;
    while (s[i] && i < DEBUG_MSG_MAX - 1) {
        dst[i] = s[i];
        i++;
    }
    dst[i] = '\0';
}

// ============================================================
// Zone memory
// ============================================================

int mb_used = 12; // Retail DOOM.WAD needs more zone for texture cache in later levels

// ============================================================
// I_GetTime — returns time in 1/35s tics (TICRATE = 35)
// ============================================================

static int basetime = 0;

int I_GetTime(void)
{
    unsigned int ms = mbc_syscall(SYS_GET_TICKS);
    if (!basetime)
        basetime = ms;
    // Convert milliseconds to 1/35s tics
    return ((ms - basetime) * TICRATE) / 1000;
}

// ============================================================
// I_Init
// ============================================================

void I_Init(void)
{
    I_InitSound();
}

// ============================================================
// I_Quit
// ============================================================

void I_Quit(void)
{
    D_QuitNetGame();
    I_ShutdownSound();
    I_ShutdownMusic();
    M_SaveDefaults();
    I_ShutdownGraphics();
    __asm__ volatile("ebreak");
    while (1) {}
}

// ============================================================
// I_WaitVBL
// ============================================================

void I_WaitVBL(int count)
{
    // Sleep for count * (1000/70) ms = ~14ms per count
    unsigned int ms = count * 14;
    if (ms > 0)
        mbc_syscall_arg(SYS_SLEEP, ms);
}

// ============================================================
// I_BeginRead / I_EndRead — no-op
// ============================================================

void I_BeginRead(void) {}
void I_EndRead(void) {}

// ============================================================
// I_Tactile — no-op (force feedback)
// ============================================================

void I_Tactile(int on, int off, int total)
{
    (void)on; (void)off; (void)total;
}

// ============================================================
// I_BaseTiccmd
// ============================================================

ticcmd_t emptycmd;

ticcmd_t *I_BaseTiccmd(void)
{
    return &emptycmd;
}

// ============================================================
// I_GetHeapSize / I_ZoneBase
// ============================================================

int I_GetHeapSize(void)
{
    return mb_used * 1024 * 1024;
}

byte *I_ZoneBase(int *size)
{
    *size = mb_used * 1024 * 1024;
    return (byte *)malloc(*size);
}

// ============================================================
// I_AllocLow
// ============================================================

byte *I_AllocLow(int length)
{
    byte *mem;
    mem = (byte *)malloc(length);
    memset(mem, 0, length);
    return mem;
}

// ============================================================
// I_Error
// ============================================================

extern boolean demorecording;

void I_Error(char *error, ...)
{
    va_list argptr;
    char buf[256];

    va_start(argptr, error);
    vsprintf(buf, error, argptr);
    va_end(argptr);

    // Write expanded error message to debug region
    debug_write(buf);

    if (demorecording)
        G_CheckDemoStatus();

    D_QuitNetGame();
    I_ShutdownGraphics();

    __asm__ volatile("ebreak");
    while (1) {}
}

// ============================================================
// main() — entry point, calls D_DoomMain()
// ============================================================

extern void D_DoomMain(void);

int main(void)
{
    // Set up minimal argc/argv for D_DoomMain
    // Let IdentifyVersion find the WAD via getenv("DOOMWADDIR") + access()
    // Our stubs: getenv("DOOMWADDIR") returns ".", access("./doomu.wad") returns 0
    static char *argv[] = {
        "doom",
        (char *)0
    };
    extern int myargc;
    extern char **myargv;

    // Force heap_ptr to HEAP_START (defense against wild writes)
    extern void debug_breadcrumb(unsigned int);
    debug_breadcrumb(0x0001);  // main entered

    myargc = 1;
    myargv = argv;

    // Test malloc + byte store/load at heap address
    char *test = (char *)malloc(16);
    if (test) {
        debug_breadcrumb(0x0002);  // malloc works
        // Step 1: verify byte store/load roundtrip
        test[0] = 'H'; test[1] = 'I'; test[2] = '\0';
        if (test[0] == 'H' && test[1] == 'I') {
            debug_breadcrumb(0x0003);  // byte store/load WORKS
        } else {
            debug_breadcrumb(0x00FC);  // byte store/load BROKEN
            // Capture what we read back + the pointer address
            volatile unsigned int *DBG = (volatile unsigned int *)0x03202000;
            DBG[0] = (unsigned char)test[0];
            DBG[1] = (unsigned char)test[1];
            DBG[2] = (unsigned int)(unsigned long)test;
        }
        // Step 2a: verify strcpy at heap address
        char *buf = (char *)malloc(32);
        if (buf) {
            strcpy(buf, "hello");
            if (strlen(buf) == 5) {
                debug_breadcrumb(0x0004);  // strcpy works
            } else {
                debug_breadcrumb(0x00FB);  // strcpy BROKEN
            }
            // Step 2b: verify sprintf with different patterns
            char *sbuf = (char *)malloc(64);
            if (sbuf) {
                // Test 1: simple format, no args
                sprintf(sbuf, "abc");
                volatile unsigned int *DBG = (volatile unsigned int *)0x03202000;
                DBG[0] = strlen(sbuf);  // should be 3
                DBG[1] = (unsigned char)sbuf[0]; // 'a'
                DBG[2] = (unsigned char)sbuf[1]; // 'b'
                if (strlen(sbuf) == 3) {
                    debug_breadcrumb(0x0005);  // simple sprintf works
                    // Test 2: %s format
                    sprintf(sbuf, "X%sY", ".");
                    DBG[3] = strlen(sbuf);  // should be 3 "X.Y"
                    DBG[4] = (unsigned char)sbuf[0]; // 'X'
                    DBG[5] = (unsigned char)sbuf[1]; // '.'
                    DBG[6] = (unsigned char)sbuf[2]; // 'Y'
                    if (strlen(sbuf) == 3) {
                        debug_breadcrumb(0x0006);  // %s works!
                    } else {
                        debug_breadcrumb(0x00F9);  // %s broken
                    }
                } else {
                    debug_breadcrumb(0x00FA);  // simple sprintf broken
                }
            }
        }
    } else {
        debug_breadcrumb(0x00FE);  // malloc BROKEN
    }

    D_DoomMain();

    // Should never return
    return 0;
}
