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
  int nz = 0;
  for(uint32 a = UPC_PER_SLOT_PGD(1); a < UPC_PER_SLOT_PGD(2); a += 4)
    if(*(volatile uint32 *)a != 0)
      nz++;
  printf("PG:f01000-nonzero=%d;\n", nz);
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

// ADR-109 D5. First call in usertrap. Without a tag (const off, or an old
// object) nothing changes and today's path runs.
void
upc_slot_enter(void)
{
  uint32 cs = upc_st.cur_slot;
  uint32 a7 = upc_st.trap_a7;
  upc_st.cur_slot = 0;
  upc_st.trap_a7 = 0;
  if((cs & UPC_TAG_MASK) != UPC_TAG_SLOT)
    return;

  uint32 s = cs & ~UPC_TAG_MASK;
  if(s >= UPC_SLOTS)
    panic("upc: cur_slot");
  if(r_sscratch() != (uint64)&upc_st.tf[s].tf)
    panic("upc: sscratch");

  struct proc *p = &proc[s];
  int rf = 0;
#ifndef UPC_MUT_NO_REFRESH
  if(s != 0 && ((upc_st.fresh_mask & (1u << s)) || p->pid == 0)){
    upc_st.fresh_mask &= ~(1u << s);
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
    rf = 1;
  }
#endif
  mycpu()->proc = p;

#ifndef UPC_MUT_NO_A7
  if((a7 & UPC_TAG_MASK) == UPC_TAG_A7)
    p->trapframe->a7 = a7 & ~UPC_TAG_MASK;
#endif

#ifdef UPC_STAGE2_PROBE
  printf("UT:s=%d;pid=%d;ss=%d;a7=%d;rf=%d;\n", (int)s, p->pid, (int)r_sscratch(),
         (int)p->trapframe->a7, rf);
  upc_probe_spill(s);
#endif
  (void)rf;
}
