// ut_twochildren.c — S26 Step 51: xv6 usertests `twochildren`, standalone.
//
// Body extracted from user/usertests.c (xv6, MIT; see upstream/LICENSE):
// "what if two children exit() at the same time?". Each iteration forks two
// children that both exit at once, then the parent waits twice. usertests
// checks only that both forks succeed; the wait() returns are not checked,
// so they are not checked here either. A hang (no verdict) is the other
// failure mode, as in usertests.
//
// Adapted:
//   * N = 2 iterations (usertests: 1000), four forks, so it ends inside one
//     TRIG=1500000 boot. Raise N with the trigger count.
//   * Fixed-string output (the MBC userland varargs ABI is not reliable), and
//     it runs as init, so it spins after its verdict instead of exiting.
//
// Expected: "UT-twochildren: ok". Failure lines:
//   "UT-twochildren: fail fork1"   the first fork() returned < 0
//   "UT-twochildren: fail fork2"   the second fork() returned < 0
//
//   upc-bootctl boot ... --userland target/ut_twochildren.mbc

#include "kernel/types.h"
#include "kernel/stat.h"
#include "user/user.h"

enum { N = 2 };

int
main(void)
{
  for(int i = 0; i < N; i++){
    int pid1 = fork();
    if(pid1 < 0){
      printf("UT-twochildren: fail fork1\n");
      printf("UT-twochildren: fail\n");
      for(;;)
        ;
    }
    if(pid1 == 0){
      exit(0);
    } else {
      int pid2 = fork();
      if(pid2 < 0){
        printf("UT-twochildren: fail fork2\n");
        printf("UT-twochildren: fail\n");
        for(;;)
          ;
      }
      if(pid2 == 0){
        exit(0);
      } else {
        wait(0);
        wait(0);
      }
    }
  }
  printf("UT-twochildren: ok\n");
  for(;;)
    ;
}
