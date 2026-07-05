// SPDX-License-Identifier: GPL-3.0-or-later
//
// usyscall.c — UNHEADED kernel syscall dispatch + user-argument fetchers.
// Track 2 Phase 2.3 (ADR-081 "evolve from xv6, never break the boot").
//
// Replaces MIT kernel/syscall.c in the Makefile.mbc OBJS list; the MIT file
// stays vendored, unwired.
//
// RUNTIME-DEAD ON THE UPC, BY ARCHITECTURE: user ecalls never reach this
// code — the ascend BPF ecall dispatch (ebpf/monad-cpu-ebpf/src/main.rs,
// SYS_* handlers) owns the entire syscall surface and services each call
// in-BPF. This file is linked because trap.c references syscall() and
// sysproc.c/sysfile.c use the arg*/fetch* helpers; it comes ALIVE in Phase
// 2.4 when the kernel core (trap path included) becomes ours. Every bounds
// check is therefore preserved at full stock strength — dead code today is
// the attack surface of tomorrow.

#include "types.h"
#include "param.h"
#include "memlayout.h"
#include "riscv.h"
#include "spinlock.h"
#include "proc.h"
#include "syscall.h"
#include "defs.h"

// Copy the uint64 at user address addr into *ip.
// Both guards are required: `addr >= p->sz` alone misses the case where
// addr+8 wraps past p->sz via overflow.
int
fetchaddr(uint64 addr, uint64 *ip)
{
  struct proc *p = myproc();

  if(addr >= p->sz || addr + sizeof(uint64) > p->sz)
    return -1;
  if(copyin(p->pagetable, (char *)ip, addr, sizeof(*ip)) != 0)
    return -1;
  return 0;
}

// Copy the NUL-terminated string at user address addr into buf (at most max
// bytes). Returns the string length excluding the NUL, or -1 on error.
// copyinstr does the page-table legality checking.
int
fetchstr(uint64 addr, char *buf, int max)
{
  struct proc *p = myproc();

  if(copyinstr(p->pagetable, buf, addr, max) < 0)
    return -1;
  return strlen(buf);
}

// The nth syscall argument lives in trapframe a0..a5.
static uint64
argraw(int n)
{
  struct proc *p = myproc();

  switch(n){
  case 0: return p->trapframe->a0;
  case 1: return p->trapframe->a1;
  case 2: return p->trapframe->a2;
  case 3: return p->trapframe->a3;
  case 4: return p->trapframe->a4;
  case 5: return p->trapframe->a5;
  }
  panic("argraw");
  return -1;
}

// Fetch the nth 32-bit syscall argument.
void
argint(int n, int *ip)
{
  *ip = argraw(n);
}

// Fetch the nth syscall argument as a user pointer. Legality is NOT checked
// here — copyin/copyout validate against the page table at use time.
void
argaddr(int n, uint64 *ip)
{
  *ip = argraw(n);
}

// Fetch the nth syscall argument as a NUL-terminated string into buf
// (at most max bytes). Returns the length on success, -1 on error.
int
argstr(int n, char *buf, int max)
{
  uint64 addr;

  argaddr(n, &addr);
  return fetchstr(addr, buf, max);
}

// Handlers, implemented across sysproc.c / sysfile.c / proc.c.
extern uint64 sys_fork(void);
extern uint64 sys_exit(void);
extern uint64 sys_wait(void);
extern uint64 sys_pipe(void);
extern uint64 sys_read(void);
extern uint64 sys_kill(void);
extern uint64 sys_exec(void);
extern uint64 sys_fstat(void);
extern uint64 sys_chdir(void);
extern uint64 sys_dup(void);
extern uint64 sys_getpid(void);
extern uint64 sys_sbrk(void);
extern uint64 sys_pause(void);
extern uint64 sys_uptime(void);
extern uint64 sys_open(void);
extern uint64 sys_write(void);
extern uint64 sys_mknod(void);
extern uint64 sys_unlink(void);
extern uint64 sys_link(void);
extern uint64 sys_mkdir(void);
extern uint64 sys_close(void);

// Syscall number (kernel/syscall.h) → handler. Gaps stay NULL and are
// rejected by dispatch below.
static uint64 (*syscalls[])(void) = {
[SYS_fork]    sys_fork,
[SYS_exit]    sys_exit,
[SYS_wait]    sys_wait,
[SYS_pipe]    sys_pipe,
[SYS_read]    sys_read,
[SYS_kill]    sys_kill,
[SYS_exec]    sys_exec,
[SYS_fstat]   sys_fstat,
[SYS_chdir]   sys_chdir,
[SYS_dup]     sys_dup,
[SYS_getpid]  sys_getpid,
[SYS_sbrk]    sys_sbrk,
[SYS_pause]   sys_pause,
[SYS_uptime]  sys_uptime,
[SYS_open]    sys_open,
[SYS_write]   sys_write,
[SYS_mknod]   sys_mknod,
[SYS_unlink]  sys_unlink,
[SYS_link]    sys_link,
[SYS_mkdir]   sys_mkdir,
[SYS_close]   sys_close,
};

// Look up a syscall number; NULL for out-of-range numbers and table gaps.
static uint64 (*handler_for(int num))(void)
{
  if(num > 0 && num < NELEM(syscalls))
    return syscalls[num];
  return 0;
}

// Dispatch the syscall in trapframe a7; result (or -1) lands in a0.
void
syscall(void)
{
  int num;
  uint64 (*fn)(void);
  struct proc *p = myproc();

  num = p->trapframe->a7;
  fn = handler_for(num);
  if(fn){
    p->trapframe->a0 = fn();
  } else {
    printf("%d %s: unknown sys call %d\n",
            p->pid, p->name, num);
    p->trapframe->a0 = -1;
  }
}
