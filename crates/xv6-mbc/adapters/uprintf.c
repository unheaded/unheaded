// SPDX-License-Identifier: GPL-3.0-or-later
//
// uprintf.c — UNHEADED kernel formatted output: printf, panic.
// Track 2 Phase 2.3 (ADR-081 "evolve from xv6, never break the boot").
//
// Replaces MIT kernel/printf.c in the Makefile.mbc OBJS list; the MIT file
// stays vendored, unwired. Every kernel boot print flows through here down
// to consputc — the phase22 golden captures gate the output byte-for-byte.
//
// Output-identical to stock by requirement, including:
//   * the format set: %d %ld %lld %u %lu %llu %x %lx %llx %p %c %s %%,
//     unknown specifiers echoed as '%' + char, bare trailing '%' dropped;
//   * per-specifier va_arg types preserved EXACTLY — under ilp32e uint64 is
//     4 bytes ("unsigned long"), so decode widths are ABI, not style;
//   * %p prints sizeof(uint64)*2 nibbles (8 on ilp32e) after "0x";
//   * the panicking/panicked pair: panicking skips the pr lock during a
//     panic print, panicked freezes console output on other harts.

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

volatile int panicking = 0; // printing a panic message
volatile int panicked = 0;  // spinning forever at end of a panic

// Serializes concurrent printf's so lines don't interleave.
static struct {
  struct spinlock lock;
} pr;

static char digits[] = "0123456789abcdef";

// Emit x in the given base; sign selects signed rendering.
static void
printint(long long xx, int base, int sign)
{
  char buf[20];
  int i;
  unsigned long long x;

  if(sign && (sign = (xx < 0)))
    x = -xx;
  else
    x = xx;

  i = 0;
  do {
    buf[i++] = digits[x % base];
  } while((x /= base) != 0);

  if(sign)
    buf[i++] = '-';

  while(--i >= 0)
    consputc(buf[i]);
}

// Emit a pointer as 0x + one nibble per 4 bits of uint64, MSB first.
static void
printptr(uint64 x)
{
  int i;

  consputc('0');
  consputc('x');
  for(i = 0; i < (sizeof(uint64) * 2); i++, x <<= 4)
    consputc(digits[x >> (sizeof(uint64) * 8 - 4)]);
}

// Emit a NUL-terminated string; NULL renders as "(null)".
static void
printstr(char *s)
{
  if(s == 0)
    s = "(null)";
  for(; *s; s++)
    consputc(*s);
}

// Print to the console.
int
printf(char *fmt, ...)
{
  va_list ap;
  int i, cx, c0, c1, c2;

  if(panicking == 0)
    acquire(&pr.lock);

  va_start(ap, fmt);
  for(i = 0; (cx = fmt[i] & 0xff) != 0; i++){
    if(cx != '%'){
      consputc(cx);
      continue;
    }
    // Peek up to three chars of specifier (l / ll length prefixes).
    i++;
    c0 = fmt[i+0] & 0xff;
    c1 = c2 = 0;
    if(c0) c1 = fmt[i+1] & 0xff;
    if(c1) c2 = fmt[i+2] & 0xff;

    if(c0 == 'd'){
      printint(va_arg(ap, int), 10, 1);
    } else if(c0 == 'l' && c1 == 'd'){
      printint(va_arg(ap, uint64), 10, 1);
      i += 1;
    } else if(c0 == 'l' && c1 == 'l' && c2 == 'd'){
      printint(va_arg(ap, uint64), 10, 1);
      i += 2;
    } else if(c0 == 'u'){
      printint(va_arg(ap, uint32), 10, 0);
    } else if(c0 == 'l' && c1 == 'u'){
      printint(va_arg(ap, uint64), 10, 0);
      i += 1;
    } else if(c0 == 'l' && c1 == 'l' && c2 == 'u'){
      printint(va_arg(ap, uint64), 10, 0);
      i += 2;
    } else if(c0 == 'x'){
      printint(va_arg(ap, uint32), 16, 0);
    } else if(c0 == 'l' && c1 == 'x'){
      printint(va_arg(ap, uint64), 16, 0);
      i += 1;
    } else if(c0 == 'l' && c1 == 'l' && c2 == 'x'){
      printint(va_arg(ap, uint64), 16, 0);
      i += 2;
    } else if(c0 == 'p'){
      printptr(va_arg(ap, uint64));
    } else if(c0 == 'c'){
      consputc(va_arg(ap, uint));
    } else if(c0 == 's'){
      printstr(va_arg(ap, char*));
    } else if(c0 == '%'){
      consputc('%');
    } else if(c0 == 0){
      // Bare '%' at end of format: emit nothing.
      break;
    } else {
      // Unknown % sequence — echo it to draw attention.
      consputc('%');
      consputc(c0);
    }
  }
  va_end(ap);

  if(panicking == 0)
    release(&pr.lock);

  return 0;
}

void
panic(char *s)
{
  panicking = 1;          // let the panic message bypass pr.lock
  printf("panic: ");
  printf("%s\n", s);
  panicked = 1;           // freeze console output from other CPUs
  for(;;)
    ;
}

void
printfinit(void)
{
  initlock(&pr.lock, "pr");
}
