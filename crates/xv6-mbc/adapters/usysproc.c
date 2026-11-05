// SPDX-License-Identifier: GPL-3.0-or-later
//
// usysproc.c — UNHEADED process syscall wrappers (sys_exit … sys_uptime).
// Track 2 Phase 2.4 (ADR-081 "evolve from xv6, never break the boot").
//
// Replaces MIT kernel/sysproc.c in the Makefile.mbc OBJS list; the MIT file
// stays vendored, unwired.
//
// RUNTIME-DEAD ON THE UPC, BY ARCHITECTURE: user ecalls are serviced by the
// ascend BPF dispatch, so these wrappers never run — they are linked because
// usyscall.c's syscalls[] table references every sys_*. They come alive if
// the trap path flips to in-kernel dispatch (T4 decision). Slot rows reach the
// kernel-routed ones (getpid, uptime; sbrk, exit under ADR-135). Kept at stock
// strength for that day: sys_sbrk's lazy-path overflow guards
// (addr + n < addr, addr + n > TRAPFRAME) and sys_pause's killed() check.
//
#include "types.h"
#include "riscv.h"
#include "defs.h"
#include "param.h"
#include "memlayout.h"
#include "spinlock.h"
#include "proc.h"
#include "vm.h"
#include "upc_slots.h"

uint64
sys_exit(void)
{
  int n;
  argint(0, &n);
  if(upc_slot_active())
    return upc_exit(n); // ADR-135 D13: kexit panics on a slot row and on slot 0
  kexit(n);
  return 0;  // not reached
}

// ADR-135 D12: BPF re-runs a finished fork's ecall as this number.
uint64
sys_upc_forknote(void)
{
  int s;

  argint(0, &s);
  if(!upc_slot_active())
    return -1;
  return upc_forknote(s);
}

uint64
sys_getpid(void)
{
  return myproc()->pid;
}

uint64
sys_fork(void)
{
  return kfork();
}

uint64
sys_wait(void)
{
  uint64 p;
  argaddr(0, &p);
  return kwait(p);
}

uint64
sys_sbrk(void)
{
  uint64 addr;
  int t;
  int n;

  argint(0, &n);
  argint(1, &t);
  if(upc_slot_active())
    return upc_sbrk(n); // ADR-135 D3: a slot row must never reach growproc
  addr = myproc()->sz;

  if(t == SBRK_EAGER || n < 0) {
    if(growproc(n) < 0) {
      return -1;
    }
  } else {
    // Lazily allocate memory for this process: increase its memory
    // size but don't allocate memory. If the processes uses the
    // memory, vmfault() will allocate it.
    if(addr + n < addr)
      return -1;
    if(addr + n > TRAPFRAME)
      return -1;
    myproc()->sz += n;
  }
  return addr;
}

uint64
sys_pause(void)
{
  int n;
  uint ticks0;

  argint(0, &n);
  if(n < 0)
    n = 0;
  if(upc_slot_active())
    return upc_pause(n); // ADR-135 D7: never sleep() on a slot row
  acquire(&tickslock);
  ticks0 = ticks;
  while(ticks - ticks0 < n){
    if(killed(myproc())){
      release(&tickslock);
      return -1;
    }
    sleep(&ticks, &tickslock);
  }
  release(&tickslock);
  return 0;
}

uint64
sys_kill(void)
{
  int pid;

  argint(0, &pid);
  if(upc_slot_active())
    return upc_kill(pid); // ADR-135 D8: kkill never sees a slot row
  return kkill(pid);
}

// return how many clock tick interrupts have occurred
// since start.
uint64
sys_uptime(void)
{
  uint xticks;

  acquire(&tickslock);
  xticks = ticks;
  release(&tickslock);
  return xticks;
}
