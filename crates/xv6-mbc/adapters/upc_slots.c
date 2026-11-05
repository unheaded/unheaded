// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.
//
// upc_slots.c — ADR-109 (EVOLUTION-1 Stage 2 rung 0): one kernel process row
// per BPF process slot. BPF schedules and owns process lifetime; the kernel
// mirrors slot s as proc[s] and refreshes the row on the slot's first trap
// after a fork. Layout shared with BPF: upc_slots.h.
//
// Mutation builds for the rung 0 gate (plan Step 104), probe kernel only:
//   UPC_MUT_SHARED_TF   every slot row uses tf[0]
//   UPC_MUT_NO_A7       trap_a7 ignored
//   UPC_MUT_NO_REFRESH  rows never refreshed
//   UPC_MUT_RESTORE_A67 (trampoline_mbc.S) userret restores a6/a7 again
#include "types.h"
#include "param.h"
#include "memlayout.h"
#include "riscv.h"
#include "spinlock.h"
#include "proc.h"
#include "defs.h"
#include "upc_slots.h"

extern struct proc proc[NPROC];
extern uint64 usertrap(void);

static struct upc_slot_tables upc_st __attribute__((aligned(UPC_TF_STRIDE)));

#define UPC_PER_SLOT_PGD(s) (0x00F00000u + (uint32)(s) * 4096u)

static int upc_active; // inside a tagged trap: upc_slot_enter to upc_slot_leave

int
upc_slot_active(void)
{
  return upc_active;
}

// ADR-135 D14: a tagged trap marks its return, so BPF skips the S and the tracer.
// Before upc_active clears, and never over a handler's reply (D1).
void
upc_slot_leave(void)
{
  if(upc_active && upc_st.reply == 0)
    upc_st.reply = UPC_TAG_REPLY;
  upc_active = 0;
}

// ADR-135 D1: the only writer of reply. One reply per trap, one flag at most.
void
upc_reply(uint32 flags, uint32 arg)
{
  upc_st.reply_arg = arg;
  upc_st.reply = UPC_TAG_REPLY | flags;
}

// ADR-135 D2-D5. uint64 is 4 bytes under ilp32e; every bound below is 32-bit.
uint64
upc_sbrk(int n)
{
  uint32 s = myproc() - proc;
  if(s == 0)
    return -1;
  uint32 cur = upc_st.brk_in;
  if(cur < UPC_HEAP_BASE || cur > UPC_HEAP_TOP)
    cur = UPC_HEAP_BASE;
  if(n > 0 && (uint32)n > UPC_HEAP_TOP - cur)
    return -1;
  if(n < 0 && (n == -2147483647 - 1 || (uint32)-n > cur - UPC_HEAP_BASE))
    return -1;
  if(n > 0)
    memset((void *)(UPC_SLICE_BASE(s) + cur), 0, n);
  upc_reply(UPC_REPLY_SET_BRK, cur + n);
  return cur;
}

// ADR-135 D13. fileclose takes a log transaction; one process is in the kernel at
// a time, so begin_op never waits.
static void
upc_close_row(struct proc *p)
{
  for(int i = 0; i < NOFILE; i++){
    struct file *f = p->ofile[i];
    if(f){
      p->ofile[i] = 0;
      fileclose(f);
    }
  }
}

// ADR-135 D8. Range only: BPF checks liveness and acts. A row with no trap yet
// has pid 0, so kkill's proc[] walk is never used for slot rows. D13: a dead slot's
// ofile is empty, so closing ahead of BPF's -1 changes nothing there.
uint64
upc_kill(int pid)
{
  if(pid < 2 || pid > UPC_SLOTS)
    return -1;
  upc_close_row(&proc[pid - 1]);
  upc_reply(UPC_REPLY_KILL, pid - 1);
  return 0;
}

// ADR-135 D13. BPF ends the slot at SRET; a0 carries the status there. cwd stays (D9).
uint64
upc_exit(int n)
{
  upc_close_row(myproc());
  upc_reply(UPC_REPLY_EXIT, 0);
  return n;
}

static uint upc_wake[UPC_SLOTS];  // 0 = no deadline; survives retries, cleared on refresh

// ADR-135 D7: cannot finish now. Run the ecall again when this slot is next scheduled.
void
upc_retry(void)
{
  myproc()->trapframe->epc -= 4;
  upc_reply(UPC_REPLY_YIELD, 0);
}

// ADR-135 D7: this trap's handler asked to run again, so a0 is still the ecall's
// argument and the handler's return is not a result. upc_slot_enter clears reply.
int
upc_retry_pending(void)
{
  uint32 want = UPC_TAG_REPLY | UPC_REPLY_YIELD;
  return (upc_st.reply & (UPC_TAG_MASK | UPC_REPLY_YIELD)) == want;
}

// ADR-110, ADR-135 D7. n UPC ticks. Decides before it changes anything, holds no
// lock, and returns the a0 it was given: the ecall runs from the top each time.
uint64
upc_pause(int n)
{
  uint32 s = myproc() - proc;
  if(n <= 0){
    upc_wake[s] = 0;
    return 0;
  }
  uint t = ticks;
  if(upc_wake[s] == 0)
    upc_wake[s] = (t + n) ? (t + n) : 1;
  if(t >= upc_wake[s]){
    upc_wake[s] = 0;
    return 0;
  }
  upc_retry();
  return n;   // a0 stays n for the re-run
}

// ADR-135 D6. A slot row's user VA to its physical address, 0 when refused.
// Only inside a tagged trap. VA 0 is mapped, so for slots 1-7 0 only ever means
// refused; slot 0's base is 0, which refuses its first page, the safe way round.
uint64
upc_user_pa(uint64 va)
{
  uint32 s = myproc() - proc;
  if(s >= UPC_SLOTS || va >= UPC_USER_VA_CEILING)
    return 0;
  return UPC_SLICE_BASE(s) + va;
}

// ADR-135 D15: BPF open walks from the root, so it has to know who is off it.
void
upc_cwd_mark(struct proc *p, int off_root)
{
  uint32 s = p - proc;
  if(off_root)
    upc_st.cwd_mask |= 1u << s;
  else
    upc_st.cwd_mask &= ~(1u << s);
}

int
upc_is_slot_row(struct proc *p)
{
  return p > &proc[0] && p < &proc[UPC_SLOTS];
}

// ADR-109 D2. After userinit(), before the scheduler first runs init.
void
upc_slots_init(void)
{
  // Init's trapframe moves into tf[0]; userinit already filled its user state.
  upc_st.tf[0].tf = *proc[0].trapframe;
  kfree((void *)proc[0].trapframe);
  proc[0].trapframe = &upc_st.tf[0].tf;

  for(int s = 0; s < UPC_SLOTS; s++){
    struct trapframe *tf = &upc_st.tf[s].tf;
    tf->kernel_satp = r_satp();
    tf->kernel_sp = proc[s].kstack + PGSIZE;
    tf->kernel_trap = (uint64)usertrap;
    tf->kernel_hartid = 0; // literal: tp is r2, which x16 shares
  }

#ifdef UPC_STAGE2_PROBE
  int nz = 0, first = -1;
  uint32 firstval = 0;
  for(uint32 a = UPC_PER_SLOT_PGD(1); a < UPC_PER_SLOT_PGD(2); a += 4)
    if(*(volatile uint32 *)a != 0){
      if(first < 0){ first = (a - UPC_PER_SLOT_PGD(1)) / 4; firstval = *(volatile uint32 *)a; }
      nz++;
    }
  printf("PG:f01000-nonzero=%d;first=%d;val=%d;\n", nz, first, (int)firstval);
#endif

  __sync_synchronize();
  *(volatile uint32 *)UPC_SLOT_TABLES_BP_BYTE = (uint32)&upc_st;
}

#ifdef UPC_STAGE2_PROBE
// I8 probe. On a child's trap, print init's x17 spill word (physical 0x64004 in
// flat S-mode), then plant a sentinel. A later child trap sees the sentinel, or
// a number init's own syscall stub stored; 11 there means userret wrote a
// child's a7 back (ADR-109 D4 broken).
#define UPC_SPILL_SENTINEL 24151u
static void
upc_probe_spill(uint32 s)
{
  if(s == 0)
    return;
  volatile uint32 *x17 = (volatile uint32 *)0x64004;
  printf("SP:x17=%d;\n", (int)*x17);
  *x17 = UPC_SPILL_SENTINEL;
}
#endif

// ADR-109 D5: slot s now holds a new process. ofile and cwd are the callers'.
static void
upc_row_refresh(uint32 s)
{
  struct proc *p = &proc[s];
  upc_st.fresh_mask &= ~(1u << s);
  upc_wake[s] = 0;
  p->pid = s + 1;
#ifdef UPC_MUT_SHARED_TF
  p->trapframe = &upc_st.tf[0].tf;
#else
  p->trapframe = &upc_st.tf[s].tf;
#endif
  p->pagetable = (pagetable_t)UPC_PER_SLOT_PGD(s); // opaque: BPF format, never walked
  p->state = RUNNING;
  p->killed = 0;
  p->xstate = 0;
  p->chan = 0;
  p->parent = 0;
  p->sz = 0;
  safestrcpy(p->name, "slot", sizeof(p->name));
}

// ADR-135 D12. The parent's fork, run again as number 24 once BPF has finalized:
// a is the child's slot. The fresh bit is set only from finalize to here, and the
// parent runs nothing in U-mode between, so a user's 24 finds it clear. The row
// takes the parent's fds at once, not at the child's first trap, because the parent
// may close one or exit first. A forged reply can end a slot without D13's close,
// so what is left in ofile is closed, not overwritten.
uint64
upc_forknote(int a)
{
  if(a < 1 || a >= UPC_SLOTS || (upc_st.fresh_mask & (1u << a)) == 0)
    return -1;
  struct proc *p = myproc();
  struct proc *c = &proc[a];
  upc_row_refresh(a);
  upc_close_row(c);
  for(int i = 0; i < NOFILE; i++)
    if(p->ofile[i])
      c->ofile[i] = filedup(p->ofile[i]);
  struct inode *old = c->cwd;
  c->cwd = idup(p->cwd);
  upc_cwd_mark(c, (upc_st.cwd_mask >> (p - proc)) & 1); // ADR-135 D15
  if(old){
    begin_op();
    iput(old);
    end_op();
  }
  return a + 1;
}

// ADR-109 D5. First call in usertrap. Without a tag (const off, or an old
// object) nothing changes and today's path runs. With the fd rung on the note has
// already filled a child's row, so this finds the bit clear and pid set.
void
upc_slot_enter(void)
{
  uint32 cs = upc_st.cur_slot;
  uint32 a7 = upc_st.trap_a7;
  upc_st.cur_slot = 0;
  upc_st.trap_a7 = 0;
  upc_st.reply = 0;     // ADR-135 D1: no word written in U-mode survives entry
  upc_st.reply_arg = 0;
  if((cs & UPC_TAG_MASK) != UPC_TAG_SLOT)
    return;

  uint32 s = cs & ~UPC_TAG_MASK;
  if(s >= UPC_SLOTS)
    panic("upc: cur_slot");
  if(r_sscratch() != (uint64)&upc_st.tf[s].tf)
    panic("upc: sscratch");

  // ADR-110 T1: the kernel's clock follows BPF's tick count. It only moves
  // forward (a hostile or stale value cannot rewind it); 0 when BPF does not
  // publish it (UPC_RUNG2 off) leaves ticks as it was.
  acquire(&tickslock);
  if(upc_st.ticks > ticks)
    ticks = upc_st.ticks;
  release(&tickslock);

  struct proc *p = &proc[s];
  int rf = 0;
#ifndef UPC_MUT_NO_REFRESH
  if(s != 0 && ((upc_st.fresh_mask & (1u << s)) || p->pid == 0)){
    upc_row_refresh(s);
    if(p->cwd == 0)
      p->cwd = namei("/"); // ADR-135 D9: once per row, kept across slot reuse
    rf = 1;
  }
#endif
  mycpu()->proc = p;
  upc_active = 1;

#ifndef UPC_MUT_NO_A7
  if((a7 & UPC_TAG_MASK) == UPC_TAG_A7)
    p->trapframe->a7 = a7 & ~UPC_TAG_MASK;
#endif

#ifdef UPC_STAGE2_PROBE
  printf("UT:s=%d;pid=%d;ss=%d;tf=%d;a7=%d;rf=%d;\n", (int)s, p->pid, (int)r_sscratch(),
         (int)p->trapframe, (int)p->trapframe->a7, rf);
  upc_probe_spill(s);
#endif
  (void)rf;
}
