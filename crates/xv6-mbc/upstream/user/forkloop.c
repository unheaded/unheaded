// forkloop.c — S-ASCEND-2 Phase 3 (Step 81): the looped-fork gate.
//
// gate_nway.c's header says a C for-loop around fork() fork-bombs: forked
// children re-enter the loop and fork again. This is that shape, three
// iterations, each forking one child and waiting for it before the next.
//
// Expected: one "FL: child" per iteration (3) and "FL: parent-done" once.
// Fixed strings only (gate2-style: the MBC userland varargs ABI is not
// reliable). Runs as init, so the parent spins after its last line instead
// of exiting.
//
//   upc-bootctl boot ... --userland target/forkloop.mbc

#include "kernel/types.h"
#include "kernel/stat.h"
#include "user/user.h"

int
main(void)
{
  for(int i = 0; i < 3; i++){
    int pid = fork();
    if(pid == 0){
      printf("FL: child\n");
      exit(0);
    }
    if(pid < 0)
      printf("FL: fork-failed\n");
    wait(0);
  }
  printf("FL: parent-done\n");
  for(;;)
    ;
}
