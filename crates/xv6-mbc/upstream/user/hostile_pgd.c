// hostile_pgd.c — EVOLUTION-1 Stage 2, ADR-109 D6 (plan Steps 102-103,
// OUTSTANDING M-247): can a BPF-forked child write memory outside its slice
// through a U-mode page-walk miss?
//
// Run as the userland directly, so this program is init (identity map) and
// reads physical memory. Init records two words, forks one child, and the
// child stores 0x5A5A5A5A to each through its own page tables:
//   pgd     VA 0x00F00FFC, the last PDE of slot 0's per-pid pgd (15 MiB up,
//           where the child's pgd has no entry);
//   tables  VA = the slot-table address from BootParams 0x160, plus a word
//           inside slot 7's trapframe (unused with one child).
// After wait(), init reads both words physically, prints whether each
// changed, and puts the old values back.
//
// With UPC_D6_MISS_FAULT off, a miss resolves to the physical address, so a
// change means the child reached memory outside its slice. With it on, a
// user-mode miss resolves to 0xFFFFFFFC, outside RAM_MAP, and the store is
// dropped. A store whose VA is mapped lands in the child's own slice and
// changes nothing here either way.
//
// Observables (fixed classes; the MBC userland varargs ABI is unreliable):
//   "HP: tbase=zero|low|high"   table address below or at/above 8 MiB
//   "HP: child=stored"          the child ran both stores
//   "HP: pgd=same|changed"
//   "HP: tables=same|changed|skip"
//   "HP: done"
#include "kernel/types.h"
#include "kernel/stat.h"
#include "user/user.h"

#define BP_SLOT_TABLES ((volatile uint *)0x160)
#define PGD0_LAST      ((volatile uint *)(0x00F00000u + 4 * 1023))
#define TF7_WORD_OFF   (7 * 0x100 + 0x40)
#define POISON         0x5A5A5A5Au

int
main(void)
{
  uint tbase = *BP_SLOT_TABLES;
  volatile uint *tw = tbase ? (volatile uint *)(tbase + TF7_WORD_OFF) : 0;

  if(tbase == 0) printf("HP: tbase=zero\n");
  else if(tbase < 0x00800000u) printf("HP: tbase=low\n");
  else printf("HP: tbase=high\n");

  uint pgd_old = *PGD0_LAST;
  uint tab_old = tw ? *tw : 0;

  int pid = fork();
  if(pid == 0){
    *PGD0_LAST = POISON;
    if(tw)
      *tw = POISON;
    printf("HP: child=stored\n");
    exit(0);
  }
  wait(0);

  if(*PGD0_LAST == pgd_old) printf("HP: pgd=same\n"); else printf("HP: pgd=changed\n");
  if(!tw) printf("HP: tables=skip\n");
  else if(*tw == tab_old) printf("HP: tables=same\n");
  else printf("HP: tables=changed\n");

  *PGD0_LAST = pgd_old;
  if(tw)
    *tw = tab_old;
  printf("HP: done\n");
  for(;;)
    ;
}
