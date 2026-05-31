// gate2.c — ASCEND-LINUX Phase 1.2 Gate 2 isolation proof (ADR-074).
//
// The hard safety property: after fork(), parent and child reference the
// SAME virtual address but live in DISJOINT physical slices (per-pid pgd
// → pid_phys_offset). fork() gives the child a COPY; the child then
// clobbers its own copy, and the parent's value MUST survive untouched.
//
// Uses printf with FIXED strings only (no %-format args — the MBC userland
// varargs ABI is not yet reliable, but plain-string printf is the same path
// init.c uses to print "init: starting sh"). The PASS/FAIL branch is driven
// by the volatile read, so the printed string IS the verdict.
//
// Run as the userland directly:
//   upc-bootctl boot ... --userland target/gate2.mbc
//
// gate2 is pid 0 and exits, so the kernel restarts it — expect the three
// lines below to repeat once per reboot:
//   C: inherited-ok
//   C: clobbered-own-slice
//   P: ISOLATION-PASS

#include "kernel/types.h"
#include "kernel/stat.h"
#include "user/user.h"

volatile int sentinel;

int
main(void)
{
  sentinel = 0xCAFE;            // parent stamps the shared VA
  int pid = fork();
  if(pid == 0){
    // Child inherits a COPY of the parent's address space → 0xCAFE.
    if(sentinel == 0xCAFE)
      printf("C: inherited-ok\n");
    else
      printf("C: inherit-BROKEN\n");
    sentinel = 0xBEEF;          // clobber — must land in the child's slice only
    printf("C: clobbered-own-slice\n");
    exit(0);
  }
  wait(0);                      // reap the child
  // The child's 0xBEEF went to a disjoint physical slice; the parent's
  // VA must still read 0xCAFE. Anything else is a cross-process leak.
  if(sentinel == 0xCAFE)
    printf("P: ISOLATION-PASS\n");
  else
    printf("P: ISOLATION-FAIL-leak\n");
  exit(0);
}
