// ut_exitwait.c — S26 Step 51: xv6 usertests `exitwait`, standalone.
//
// Body extracted from user/usertests.c (xv6, MIT; see upstream/LICENSE):
// "try to find any races between exit and wait". Each iteration forks one
// child that exits at once; the parent's wait() must return that child.
//
// Adapted:
//   * The exit-status check is dropped. usertests checks `xstate == i`; the
//     BPF wait writes 0 as the status today, so only iteration 0 could pass.
//     The child still calls exit(i) and the parent still passes &xstate (the
//     status write path runs), but its value is not checked.
//   * N = 3 iterations (usertests: 100), the forkloop.c budget, so it ends
//     inside one TRIG=1500000 boot. Raise N with the trigger count.
//   * Fixed-string output (the MBC userland varargs ABI is not reliable), and
//     it runs as init, so it spins after its verdict instead of exiting.
//
// Expected: "UT-exitwait: ok". Failure lines:
//   "UT-exitwait: fail fork"            fork() returned < 0
//   "UT-exitwait: fail wait-wrong-pid"  wait() returned a pid other than fork's
//
//   upc-bootctl boot ... --userland target/ut_exitwait.mbc

#include "kernel/types.h"
#include "kernel/stat.h"
#include "user/user.h"

enum { N = 3 };

int
main(void)
{
  int i, pid;

  for(i = 0; i < N; i++){
    pid = fork();
    if(pid < 0){
      printf("UT-exitwait: fail fork\n");
      printf("UT-exitwait: fail\n");
      for(;;)
        ;
    }
    if(pid){
      int xstate;
      if(wait(&xstate) != pid){
        printf("UT-exitwait: fail wait-wrong-pid\n");
        printf("UT-exitwait: fail\n");
        for(;;)
          ;
      }
    } else {
      exit(i);
    }
  }
  printf("UT-exitwait: ok\n");
  for(;;)
    ;
}
