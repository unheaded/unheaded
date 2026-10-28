// ut_forktest.c — S26 Step 51: xv6 usertests `forktest`, standalone.
//
// Body extracted from user/usertests.c (xv6, MIT; see upstream/LICENSE):
// "test that fork fails gracefully". Fork until fork() fails (children exit
// at once and stay zombies until reaped), then wait() once per child, then
// one more wait() must return -1. Not adapted beyond the output: N stays
// 1000; on the UPC the 8-slot PROC_TABLE (init + 7) or the physical slice
// ceiling makes fork fail long before that.
//
// Fixed-string output (the MBC userland varargs ABI is not reliable), and it
// runs as init, so it spins after its verdict instead of exiting.
//
// Before the verdict, one informational line names how many forks succeeded
// (not a check; usertests does not print it): "UT-forktest: forks=N", N in
// 1..7, or "forks=other". With 8 PROC_TABLE slots, expect forks=7.
//
// Expected: "UT-forktest: forks=7" then "UT-forktest: ok". Failure lines:
//   "UT-forktest: fail no-fork"          the first fork() already failed
//   "UT-forktest: fail fork-never-failed" 1000 forks all succeeded
//   "UT-forktest: fail wait-early"       wait() returned < 0 with children left
//   "UT-forktest: fail wait-too-many"    the extra wait() did not return -1
//
//   upc-bootctl boot ... --userland target/ut_forktest.mbc

#include "kernel/types.h"
#include "kernel/stat.h"
#include "user/user.h"

enum { N = 1000 };

int
main(void)
{
  int n, pid;

  for(n = 0; n < N; n++){
    pid = fork();
    if(pid < 0)
      break;
    if(pid == 0)
      exit(0);
  }

  if(n == 1) printf("UT-forktest: forks=1\n");
  else if(n == 2) printf("UT-forktest: forks=2\n");
  else if(n == 3) printf("UT-forktest: forks=3\n");
  else if(n == 4) printf("UT-forktest: forks=4\n");
  else if(n == 5) printf("UT-forktest: forks=5\n");
  else if(n == 6) printf("UT-forktest: forks=6\n");
  else if(n == 7) printf("UT-forktest: forks=7\n");
  else printf("UT-forktest: forks=other\n");

  if(n == 0){
    printf("UT-forktest: fail no-fork\n");
    printf("UT-forktest: fail\n");
    for(;;)
      ;
  }

  if(n == N){
    printf("UT-forktest: fail fork-never-failed\n");
    printf("UT-forktest: fail\n");
    for(;;)
      ;
  }

  for(; n > 0; n--){
    if(wait(0) < 0){
      printf("UT-forktest: fail wait-early\n");
      printf("UT-forktest: fail\n");
      for(;;)
        ;
    }
  }

  if(wait(0) != -1){
    printf("UT-forktest: fail wait-too-many\n");
    printf("UT-forktest: fail\n");
    for(;;)
      ;
  }

  printf("UT-forktest: ok\n");
  for(;;)
    ;
}
