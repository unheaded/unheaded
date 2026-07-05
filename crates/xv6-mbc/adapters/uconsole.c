// SPDX-License-Identifier: GPL-3.0-or-later
//
// uconsole.c — UNHEADED console driver top half.
// Track 2 Phase 2.3 (ADR-081 "evolve from xv6, never break the boot").
//
// Replaces MIT kernel/console.c in the Makefile.mbc OBJS list; the MIT file
// stays vendored, unwired. The bottom half (uartinit/uartputc_sync/uartwrite)
// has been ours since Phase 1.1 (adapters/console-mmio.c) — this completes
// the console stack.
//
// Live on the UPC: consoleinit (lock + uart + devsw wiring) and consputc —
// every kernel printf byte lands here on its way to the TTY MMIO. The phase22
// goldens gate that path byte-for-byte.
//
// Runtime-dead on the UPC, preserved at full stock strength: consoleread /
// consolewrite (devsw targets — the BPF ecall dispatch owns user fd I/O) and
// consoleintr (no UART interrupt; the KBD ring in BPF feeds read(5) directly).
// They come ALIVE in Phase 2.4 when the kernel core is ours, so their line
// discipline (^H/^U/^D/^P, echo, ring indices) and copy bounds are kept
// behavior-identical.

#include <stdarg.h>

#include "types.h"
#include "param.h"
#include "spinlock.h"
#include "sleeplock.h"
#include "fs.h"
#include "file.h"
#include "memlayout.h"
#include "riscv.h"
#include "defs.h"
#include "proc.h"

#define BACKSPACE 0x100        // erase the last output character
#define CTRL(x) ((x) - '@')    // Control-x

// Send one character to the uart without interrupts or sleep() — safe from
// interrupt context (printf, input echo). BACKSPACE erases on-screen by
// rubbing out the previous character with a space.
void
consputc(int c)
{
  if(c == BACKSPACE){
    uartputc_sync('\b');
    uartputc_sync(' ');
    uartputc_sync('\b');
  } else {
    uartputc_sync(c);
  }
}

// Console input line buffer. r chases w; e is the edit cursor beyond w
// while a line is still being typed.
#define INPUT_BUF_SIZE 128
struct {
  struct spinlock lock;
  char buf[INPUT_BUF_SIZE];
  uint r;  // Read index
  uint w;  // Write index
  uint e;  // Edit index
} cons;

// user write() syscalls to the console: batch bytes from user space to the
// uart, 32 at a time. Returns bytes written (short on copy failure).
int
consolewrite(int user_src, uint64 src, int n)
{
  char buf[32];
  int i = 0;

  while(i < n){
    int nn = sizeof(buf);
    if(nn > n - i)
      nn = n - i;
    if(either_copyin(buf, user_src, src+i, nn) == -1)
      break;
    uartwrite(buf, nn);
    i += nn;
  }

  return i;
}

// user read()s from the console: copy up to a whole input line to dst,
// sleeping until consoleintr has buffered one. user_dst says whether dst is
// a user or kernel address. Returns bytes read, or -1 if killed.
int
consoleread(int user_dst, uint64 dst, int n)
{
  uint target;
  int c;
  char cbuf;

  target = n;
  acquire(&cons.lock);
  while(n > 0){
    // block until the interrupt handler has buffered input
    while(cons.r == cons.w){
      if(killed(myproc())){
        release(&cons.lock);
        return -1;
      }
      sleep(&cons.r, &cons.lock);
    }

    c = cons.buf[cons.r++ % INPUT_BUF_SIZE];

    if(c == CTRL('D')){  // end-of-file
      if(n < target){
        // Save ^D for next time: the caller must see a 0-byte read.
        cons.r--;
      }
      break;
    }

    cbuf = c;
    if(either_copyout(user_dst, dst, &cbuf, 1) == -1)
      break;

    dst++;
    --n;

    if(c == '\n'){
      // whole line delivered — return to the user-level read()
      break;
    }
  }
  release(&cons.lock);

  return target - n;
}

// Erase one pending (unconsumed) character from the edit buffer, if any.
static void
erase_one(void)
{
  if(cons.e != cons.w){
    cons.e--;
    consputc(BACKSPACE);
  }
}

// The console input interrupt handler: uartintr() delivers each input
// character here. Line editing (^H/^U), ^P procdump, echo, and wakeup of
// consoleread() when a line (or ^D, or a full buffer) is ready.
void
consoleintr(int c)
{
  acquire(&cons.lock);

  switch(c){
  case CTRL('P'):  // print process list
    procdump();
    break;
  case CTRL('U'):  // kill the whole pending line
    while(cons.e != cons.w &&
          cons.buf[(cons.e-1) % INPUT_BUF_SIZE] != '\n')
      erase_one();
    break;
  case CTRL('H'):  // backspace
  case '\x7f':     // delete key
    erase_one();
    break;
  default:
    if(c != 0 && cons.e-cons.r < INPUT_BUF_SIZE){
      c = (c == '\r') ? '\n' : c;

      // echo back to the user
      consputc(c);

      // buffer for consumption by consoleread()
      cons.buf[cons.e++ % INPUT_BUF_SIZE] = c;

      if(c == '\n' || c == CTRL('D') || cons.e-cons.r == INPUT_BUF_SIZE){
        // a whole line (or EOF, or a full buffer) is ready
        cons.w = cons.e;
        wakeup(&cons.r);
      }
    }
    break;
  }

  release(&cons.lock);
}

void
consoleinit(void)
{
  initlock(&cons.lock, "cons");

  uartinit();

  // route the console device's read/write syscalls here
  devsw[CONSOLE].read = consoleread;
  devsw[CONSOLE].write = consolewrite;
}
