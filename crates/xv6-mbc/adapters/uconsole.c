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
// consolewrite is live under UPC_RUNG3 (ADR-135 D19): sys_write calls it for a
// slot row's fd below 16, which BPF routes here when the fd is not a file.
//
// upc_consoleread is live under UPC_RUNG4 (ADR-135 D20): sys_read calls it for
// a slot row's fd below 16, which BPF routes here when the fd is the console.
// It polls: there is no UART interrupt, so each trap takes the one byte BPF
// offers and feeds it to consoleintr, quietly (BPF never echoed typed input,
// and the goldens hold what is printed). The line discipline is consoleintr's.
//
// Runtime-dead on the UPC, preserved at full stock strength: consoleread
// (a devsw target: the console is implicit in BPF and has no node in fs.img,
// so no kernel fd reaches it, ADR-135 D11, D15). Its line discipline
// (^H/^U/^D/^P, ring indices) and copy bounds are kept behavior-identical;
// consoleintr's echo is the one thing the polled path switches off.

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
#include "upc_slots.h"

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

// console-mmio.c: a one-byte store to the TTY (ADR-135 D19).
void uartputc_byte(int c);

// user write() syscalls to the console: batch bytes from user space to the
// uart, 32 at a time. Returns bytes written (short on copy failure). Each byte
// goes out as one byte (uartputc_byte), not as uartwrite's word per character:
// under UPC_RUNG3 this is what a user's write(1, ...) prints through.
// One call takes at most CONSOLE_WRITE_MAX bytes and answers that count (a
// short write), as BPF's console arm did: without it a single
// write(1, p, 0x7fffffff) prints the caller's whole slice inside one trap
// (security pass review, finding 5, 2026-11-15).
#define CONSOLE_WRITE_MAX 4096
int
consolewrite(int user_src, uint64 src, int n)
{
  char buf[32];
  int i = 0;

  if(n > CONSOLE_WRITE_MAX)
    n = CONSOLE_WRITE_MAX;

  while(i < n){
    int nn = sizeof(buf);
    if(nn > n - i)
      nn = n - i;
    if(either_copyin(buf, user_src, src+i, nn) == -1)
      break;
    for(int j = 0; j < nn; j++)
      uartputc_byte((int)(unsigned char)buf[j]);
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

// Set while upc_consoleread feeds consoleintr: no echo, no backspace print.
static int cons_quiet;

// Erase one pending (unconsumed) character from the edit buffer, if any.
static void
erase_one(void)
{
  if(cons.e != cons.w){
    cons.e--;
    if(!cons_quiet)
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
      if(!cons_quiet)
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

// ADR-135 D20 (rung 4): a slot row's read() on the console. One trap, one byte:
// with no complete line buffered, take the byte BPF offered and run it through
// consoleintr; still no line, take the retry hand-off (D7) and never sleep.
// With a line buffered, copy out as consoleread does: up to n bytes, stopping
// after a newline, 0 at ^D. The pointer is judged before any wait, so a bad
// one answers -1 with or without input. A copy that fails puts its byte back.
// M-186.8: every call clears the cheap-wait mark; a retry with no line and no
// byte offered sets it, for this slot and fd (upc_cons_wait, upc_slots.c).
int
upc_consoleread(uint64 dst, int n)
{
  uint target;
  int c = 0, have;
  char cbuf;

#if UPC_CHEAP_WAIT
  upc_cons_wait(0);
#endif
  if(n < 0 || upc_user_pa(dst) == 0)
    return -1;
  if(n == 0)
    return 0;

  acquire(&cons.lock);
  have = cons.r != cons.w;
  release(&cons.lock);
  if(!have){
    if((c = upc_kbd_take()) >= 0){
      cons_quiet = 1;
      consoleintr(c);
      cons_quiet = 0;
    }
    acquire(&cons.lock);
    have = cons.r != cons.w;
    release(&cons.lock);
    if(!have){
#if UPC_CHEAP_WAIT
      if(c < 0)
        upc_cons_wait(1);  // nothing buffered and nothing offered: BPF may hold
#endif
      upc_retry();
      return UPC_RETRY_RC;
    }
  }

  target = n;
  acquire(&cons.lock);
  while(n > 0 && cons.r != cons.w){
    c = cons.buf[cons.r++ % INPUT_BUF_SIZE];
    if(c == CTRL('D')){
      if(n < target)
        cons.r--;  // the caller sees a 0-byte read next time
      break;
    }
    cbuf = c;
    if(either_copyout(1, dst, &cbuf, 1) == -1){
      cons.r--;
      break;
    }
    dst++;
    --n;
    if(c == '\n')
      break;
  }
  release(&cons.lock);

  if(n == target && c != CTRL('D'))
    return -1;  // nothing copied and not end-of-file: the copy failed
  return target - n;
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
