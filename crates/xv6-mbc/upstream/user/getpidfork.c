// getpidfork.c — S-ASCEND-2 Phase 1 (Step 27): a FORKED CHILD calls getpid.
//
// EVOLUTION-1's "next probe". getpidprobe.c only ever asks as init; the
// dual-books question is what a child sees. Run as the userland directly:
//   upc-bootctl boot ... --userland target/getpidfork.mbc
// It runs as init (pid 1, BPF slot 0) and spins after its last line instead of exiting.
// Before the S-ASCEND-2 Phase 3 scheduler fix, init's exit resumed the reaped
// child, which ran the parent's post-wait code (the first runs of this probe
// showed exactly that); spinning keeps the output the same on both sides of
// the fix.
//
// Observables (fixed strings only, gate2-style: the MBC userland varargs ABI
// is not reliable, so the printed token IS the value):
//   "GF: parent-pid=N"     the parent's getpid before fork, N in {0,1,2,other}
//   "GF: child-pid=N"      the child's getpid, then the child exits
//   "GF: fork-ret=N"       fork()'s return in the parent
//   "GF: wait-ret=N"       wait()'s return (the reaped child)
//   "GF: parent-after-wait" the parent reaped the child and kept running
// With option K (S-ASCEND-2 E1, 2026-10-28) the expected values are
// parent 1, child 2, fork-ret 2, wait-ret 2: one numbering, init = 1.
//
// fork() is called ONCE (loops are covered by forkloop.c).
// With GETPID_INJECT off, BPF answers getpid with slot + 1 (option K, E1);
// with it on, the owned kernel answers from p->pid. The kernel numbers init 1
// too, so both books agree on init; a BPF-forked child has no kernel process
// yet (EVOLUTION-1 Stage 2 rung 0), so with the flag on it still reads -1.

#include "kernel/types.h"
#include "kernel/stat.h"
#include "user/user.h"

int
main(void)
{
  // Fixed strings in if-chains, no helper taking a pointer: the MBC userland
  // calling convention is not reliable for that (the first draft passed the
  // role as a string and the child printed the parent's strings).
  int me = getpid();
  if(me == 0) printf("GF: parent-pid=0\n");
  else if(me == 1) printf("GF: parent-pid=1\n");
  else if(me == 2) printf("GF: parent-pid=2\n");
  else printf("GF: parent-pid=other\n");
  int pid = fork();
  if(pid == 0){
    int c = getpid();
    if(c == 0) printf("GF: child-pid=0\n");
    else if(c == 1) printf("GF: child-pid=1\n");
    else if(c == 2) printf("GF: child-pid=2\n");
    else printf("GF: child-pid=other\n");
    exit(0);
  }
  // fork()'s and wait()'s return values must name the same child that the
  // child's own getpid() reports (S-ASCEND-2 E1: one numbering, init = 1).
  if(pid == 1) printf("GF: fork-ret=1\n");
  else if(pid == 2) printf("GF: fork-ret=2\n");
  else printf("GF: fork-ret=other\n");
  int w = wait(0);
  if(w == 1) printf("GF: wait-ret=1\n");
  else if(w == 2) printf("GF: wait-ret=2\n");
  else printf("GF: wait-ret=other\n");
  printf("GF: parent-after-wait\n");
  for(;;)
    ;
}
