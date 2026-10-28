// getpidx.c — EVOLUTION-1 Stage 2 rung 0 (plan Step 100): a second injected
// syscall number. Two live children each call getpid, then raw syscall 22
// (no xv6 syscall has that number), then getpid again. Only an object built
// with UPC_PROBE_INJECT22 routes 22 to the owned kernel, which prints
// "<pid> slot: unknown sys call 22" and returns -1. The pid in that line must
// be the child's (2 or 3), and a7 must reach the kernel as 22 although the
// child's previous syscall was getpid (11). Probe only; not a harness gate.
//
// Observables: "GX: cK-pid=N", "GX: cK-r22=neg1|other", "GX: cK-again=N",
// "GX: fork-ret=N", "GX: wait-ret=N", "GX: done".
#include "kernel/types.h"
#include "kernel/stat.h"
#include "user/user.h"

#define SAY(tag, v) do { int v_ = (v);                    \
    if(v_ == 1) printf(tag "=1\n");                       \
    else if(v_ == 2) printf(tag "=2\n");                  \
    else if(v_ == 3) printf(tag "=3\n");                  \
    else if(v_ == -1) printf(tag "=neg1\n");              \
    else printf(tag "=other\n"); } while(0)

static int
sys22(void)
{
  register int a0 asm("a0");
  asm volatile("li a7, 22\n\tecall" : "=r"(a0) : : "a7", "memory");
  return a0;
}

int
main(void)
{
  for(int k = 1; k <= 2; k++){
    int pid = fork();
    if(pid == 0){
      if(k == 1){ SAY("GX: c1-pid", getpid()); SAY("GX: c1-r22", sys22()); SAY("GX: c1-again", getpid()); }
      else { SAY("GX: c2-pid", getpid()); SAY("GX: c2-r22", sys22()); SAY("GX: c2-again", getpid()); }
      exit(0);
    }
    SAY("GX: fork-ret", pid);
  }
  for(int k = 0; k < 2; k++)
    SAY("GX: wait-ret", wait(0));
  printf("GX: done\n");
  for(;;)
    ;
}
