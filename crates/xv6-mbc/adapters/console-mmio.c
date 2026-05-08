// SPDX-License-Identifier: GPL-3.0-or-later
//
// console-mmio.c — MBC-native replacement for upstream/kernel/uart.c.
// ASCEND-LINUX Phase 1.1 (per references/battle-plan-ascend-linux-2026-05-08.md).
//
// upstream/kernel/uart.c (161 LOC) targets QEMU virt machine UART at
// 0x10000000 with NS16550A register layout and DLAB-style baud-rate setup.
//
// We replace it with direct writes to UPC's MMIO TTY at 0xC001 (per
// docs/doom/UPC_OS_PRIMITIVES.md L4f). The MBC dispatch layer routes
// 0xC001 writes to the Busboy topic `compute.tty.{label}` which is then
// fan-out to:
//
//   - WebSocket xterm bridge (Mode A demo per battle-plan §1)
//   - Direct host pty (Mode B)
//   - SSH session (Mode C, post-Phase-4)
//
// Exported API (matches upstream/kernel/defs.h):
//   void uartinit(void)             — no-op; MMIO is always available
//   void uartintr(void)             — RX interrupt handler stub
//   void uartwrite(char buf[], int) — bulk write loop
//   void uartputc_sync(int c)       — single byte write (used by panic)
//   int  uartgetc(void)             — RX read; 0xFF if empty
//
// xv6's printf.c, console.c, and panic call into these symbols. The
// rest of the kernel doesn't change.

#include "types.h"

// ── UPC MMIO TTY (L4f convention, matches start_mbc.c) ────────────────────
#define UPC_TTY_DATA_ADDR    0xC001    // write byte; published to Busboy
#define UPC_TTY_STATUS_ADDR  0xC002    // read: bit 0 = RX ready, bit 1 = TX busy
#define UPC_TTY_KBD_ADDR     0xFFFF    // read keyboard byte from KBD_MAP

#define UPC_TTY_DATA   (*(volatile uint32 *)(UPC_TTY_DATA_ADDR))
#define UPC_TTY_STATUS (*(volatile uint32 *)(UPC_TTY_STATUS_ADDR))
#define UPC_TTY_KBD    (*(volatile uint32 *)(UPC_TTY_KBD_ADDR))

#define TTY_STATUS_RX_READY  0x01
#define TTY_STATUS_TX_BUSY   0x02

void
uartinit(void)
{
    // No initialization needed — MMIO is wired by the bootloader before
    // start_mbc.c runs. Kept as a symbol so xv6's main.c doesn't need patching.
}

void
uartputc_sync(int c)
{
    // Spin until TX not busy (the BPF interpreter keeps this very fast since
    // the write is a BPF map update, not a real serial line).
    while (UPC_TTY_STATUS & TTY_STATUS_TX_BUSY) {
        // busy-wait
    }
    UPC_TTY_DATA = (uint32)(c & 0xFF);
}

void
uartwrite(char buf[], int n)
{
    for (int i = 0; i < n; i++) {
        uartputc_sync((int)(unsigned char)buf[i]);
    }
}

int
uartgetc(void)
{
    if (UPC_TTY_STATUS & TTY_STATUS_RX_READY) {
        return (int)(UPC_TTY_KBD & 0xFF);
    }
    return -1;  // xv6 console.c interprets -1 as "no input"
}

void
uartintr(void)
{
    // RX interrupt: drain the keyboard buffer into xv6's console layer.
    // xv6's console.c provides consoleintr(int c).
    extern void consoleintr(int);
    int c;
    while ((c = uartgetc()) != -1) {
        consoleintr(c);
    }
}
