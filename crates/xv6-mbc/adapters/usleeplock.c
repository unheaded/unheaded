// SPDX-License-Identifier: GPL-3.0-or-later
//
// usleeplock.c — UNHEADED sleeping locks (long-held locks that yield).
// Track 2 Phase 2.4 (ADR-081 "evolve from xv6, never break the boot").
//
// Replaces MIT kernel/sleeplock.c in the Makefile.mbc OBJS list; the MIT file
// stays vendored, unwired.
//
// Boot-live in bio/log/inode init paths (binit/iinit take these at boot);
// contended sleep/wakeup traffic stays dormant until multi-process file I/O.
//
// Quirk kept verbatim: initsleeplock names the INNER spinlock with the
// literal "sleep lock" — every sleeplock's guard shares that name; only
// lk->name carries the caller's name. Stock behavior, not a bug to fix here.

#include "types.h"
#include "riscv.h"
#include "defs.h"
#include "param.h"
#include "memlayout.h"
#include "spinlock.h"
#include "proc.h"
#include "sleeplock.h"

void
initsleeplock(struct sleeplock *lk, char *name)
{
  initlock(&lk->lk, "sleep lock");
  lk->name = name;
  lk->locked = 0;
  lk->pid = 0;
}

void
acquiresleep(struct sleeplock *lk)
{
  acquire(&lk->lk);
  while (lk->locked) {
    sleep(lk, &lk->lk);
  }
  lk->locked = 1;
  lk->pid = myproc()->pid;
  release(&lk->lk);
}

void
releasesleep(struct sleeplock *lk)
{
  acquire(&lk->lk);
  lk->locked = 0;
  lk->pid = 0;
  wakeup(lk);
  release(&lk->lk);
}

int
holdingsleep(struct sleeplock *lk)
{
  int r;

  acquire(&lk->lk);
  r = lk->locked && (lk->pid == myproc()->pid);
  release(&lk->lk);
  return r;
}
