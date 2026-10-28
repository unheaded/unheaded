// ut_reparent2.c — S26 Step 51: xv6 usertests `reparent2`, standalone.
//
// Body extracted from user/usertests.c (xv6, MIT; see upstream/LICENSE):
// "does reparent() violate the parent-then-child locking order when giving
// away a child to init, so that exit() deadlocks against init's wait()?".
// Each iteration forks pid1; pid1 forks twice (up to 3 descendants, fork
// returns unchecked, as in usertests) and exits, orphaning them to init; the
// test waits for pid1. usertests fails only when the pid1 fork fails; a
// deadlock shows as a hang (no verdict).
//
// Adapted:
//   * Shape. In usertests the body runs in a child of usertests' run(), and
//     the real init reaps the orphans. Here the probe IS init, so main()
//     plays both parts: it forks a runner child that runs the body (run()'s
//     child), then loops on wait(0) until -1 like xv6 init.c, reaping the
//     runner and every orphan. Without that loop the orphans would never be
//     reaped and fill the 8-slot PROC_TABLE.
//   * No exit status: the runner prints its own verdict (usertests' run()
//     reads it from wait(&xstatus); the BPF wait writes 0 today).
//   * N = 2 iterations (usertests: 800), so it ends inside one boot (about
//     ten forks; TRIG may need raising above 1500000).
//   * Fixed-string output (the MBC userland varargs ABI is not reliable);
//     init spins after its last line instead of exiting.
//
// Expected, in order:
//   "UT-reparent2: ok"         the runner finished N iterations
//   "UT-reparent2: reaped-all" init's wait() returned -1: runner and orphans
//                              all reaped, none left behind (it follows a
//                              runner "fail" line too: it is about reaping)
// Failure lines:
//   "UT-reparent2: fail fork"           the runner's pid1 fork() returned < 0
//   "UT-reparent2: fail runner-fork"    init could not fork the runner
//   "UT-reparent2: fail runner-unreaped" wait() returned -1 before the runner
//
//   upc-bootctl boot ... --userland target/ut_reparent2.mbc

#include "kernel/types.h"
#include "kernel/stat.h"
#include "user/user.h"

enum { N = 2 };

int
main(void)
{
  int runner = fork();
  if(runner < 0){
    printf("UT-reparent2: fail runner-fork\n");
    printf("UT-reparent2: fail\n");
    for(;;)
      ;
  }
  if(runner == 0){
    // The usertests body (run()'s child).
    for(int i = 0; i < N; i++){
      int pid1 = fork();
      if(pid1 < 0){
        printf("UT-reparent2: fail fork\n");
        printf("UT-reparent2: fail\n");
        exit(1);
      }
      if(pid1 == 0){
        fork();
        fork();
        exit(0);
      }
      wait(0);
    }
    printf("UT-reparent2: ok\n");
    exit(0);
  }

  // init's part: reap the runner and every orphan handed to init.
  int runner_reaped = 0;
  for(;;){
    int w = wait(0);
    if(w < 0)
      break;
    if(w == runner)
      runner_reaped = 1;
  }
  if(!runner_reaped){
    printf("UT-reparent2: fail runner-unreaped\n");
    printf("UT-reparent2: fail\n");
    for(;;)
      ;
  }
  printf("UT-reparent2: reaped-all\n");
  for(;;)
    ;
}
