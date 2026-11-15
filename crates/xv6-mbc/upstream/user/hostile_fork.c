// SPDX-License-Identifier: GPL-3.0-or-later
// hostile_fork.c: S-STAGE-2C Phase 6, ADR-135 D18 (R-F3). Fork's copy cannot be
// skipped by a page-directory word or by the registers it uses as scratch.
//
// Run as the userland directly, so this program is init (slot 0). Init forks the
// worker (slot 1). The worker grows its heap and writes a canary there, inside
// the band fork copies.
//
//   plant  The worker forks a planter (slot 2), which stores a present bit into
//          pde 0 of slot 3's page directory, the next free slot, and waits. The
//          worker then forks into slot 3. Fork used to take "pde 0 present" as
//          "the copy is under way" and the caller's a2 as its cursor.
//   a2     The worker forks by hand with a2 past the end of the copy, once with
//          a3 clear and once with a3 all ones. Neither register is consulted.
//
// Observables (fixed classes; the MBC userland varargs ABI is unreliable):
//   "HF: copy-complete=yes|no"      the child in the planted slot has the whole canary
//   "HF: pde-plant-dropped=yes|no"  that child reads 0 at its own pde 0: with ADR-109
//                                   D6 on the directory is out of reach; with D6
//                                   off it is not, and this says no
//   "HF: a2-ignored=yes|no"         both hand-made forks gave a whole canary
//   "HF: done"                      init, after the worker is reaped
#include "kernel/types.h"
#include "kernel/stat.h"
#include "user/user.h"

#define SAY(t, c) do { if(c) printf(t "=yes\n"); else printf(t "=no\n"); } while(0)

#define PGD_BASE     0x00F00000u
#define PTE_PRESENT  0x800u       // mmu::PTE_PRESENT, as hostile_reply's alias entry has it
#define PLANT_SLOT   3
#define CANARY_VA    0x00014000u  // heap window, inside fork's low copy band
#define CANARY_WORDS 64
#define PAST_COPY    0x0003F000u  // a cursor beyond the last word fork copies

#define CANARY_FIRST  0xC0DE0000u
#define CANARY_SECOND 0xFACE0000u  // written before the a2 forks (see worker)

static void
canary_set(uint tag)
{
  volatile uint *c = (volatile uint *)CANARY_VA;
  for(uint i = 0; i < CANARY_WORDS; i++)
    c[i] = tag | i;
}

static int
canary_whole(uint tag)
{
  volatile uint *c = (volatile uint *)CANARY_VA;
  for(uint i = 0; i < CANARY_WORDS; i++)
    if(c[i] != (tag | i))
      return 0;
  return 1;
}

// SYS_fork with chosen a2 and a3.
static int
fork_with(uint a2v, uint a3v)
{
  register int a0 __asm__("a0");
  register uint a2 __asm__("a2") = a2v;
  register uint a3 __asm__("a3") = a3v;
  __asm__ volatile("li a7, 1\n\tecall" : "=r"(a0), "+r"(a2), "+r"(a3) : : "a7", "memory");
  return a0;
}

static void
worker(void)
{
  // Heap up to past the canary, then the canary itself.
  sbrk(0x5000);
  canary_set(CANARY_FIRST);

  // plant
  int planter = fork();
  if(planter == 0){
    *(volatile uint *)(PGD_BASE + PLANT_SLOT * 4096) = PTE_PRESENT;
    pause(400); // stay alive, so the worker's next fork takes slot 3
    exit(0);
  }
  pause(20); // let the planter run first
  if(fork() == 0){
    SAY("HF: copy-complete", canary_whole(CANARY_FIRST));
    SAY("HF: pde-plant-dropped", *(volatile uint *)(PGD_BASE + PLANT_SLOT * 4096) == 0);
    exit(0);
  }
  wait(0);       // the child in slot 3 is the only one that can have ended
  kill(planter);
  wait(0);

  // a2: the first child speaks only if its copy is short, the second always.
  // The slots these forks reuse held the first canary (their last occupants
  // were copies of this worker), so a skipped copy would still show it whole:
  // the worker changes its canary first, and only a real copy carries the new
  // one (security pass review, finding 4).
  canary_set(CANARY_SECOND);
  if(fork_with(PAST_COPY, 0) == 0){
    if(!canary_whole(CANARY_SECOND))
      printf("HF: a2-ignored=no\n");
    exit(0);
  }
  wait(0);
  if(fork_with(PAST_COPY, 0xFFFFFFFFu) == 0){
    SAY("HF: a2-ignored", canary_whole(CANARY_SECOND));
    exit(0);
  }
  wait(0);
}

int
main(void)
{
  if(fork() == 0){
    worker();
    exit(0);
  }
  wait(0);
  printf("HF: done\n");
  for(;;)
    ;
}
