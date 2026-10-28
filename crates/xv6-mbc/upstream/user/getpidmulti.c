// getpidmulti.c — EVOLUTION-1 Stage 2 rung 0 (plan Step 62): live slots.
//
// The parent forks three children before waiting, so slots 1-3 are live at
// once. Each child calls getpid, pause(1), then getpid again: BPF schedules
// the others in between, so traps from different slots interleave (ADR-109 I1
// across slots). Option K: children are 2, 3 and 4; "again" equals "pid".
//
// Observables (fixed strings, SAY expands inline):
//   "GM: cK-pid=N"    child K's first getpid (K = 1..3, fork order)
//   "GM: cK-again=N"  child K's getpid after pause
//   "GM: fork-ret=N"  each fork()'s return in the parent
//   "GM: wait-ret=N"  each wait()'s return
//   "GM: done"        after the third wait; then init spins
#include "kernel/types.h"
#include "kernel/stat.h"
#include "user/user.h"

#define SAY(tag, v) do { int v_ = (v);                    \
    if(v_ == 1) printf(tag "=1\n");                       \
    else if(v_ == 2) printf(tag "=2\n");                  \
    else if(v_ == 3) printf(tag "=3\n");                  \
    else if(v_ == 4) printf(tag "=4\n");                  \
    else if(v_ == 5) printf(tag "=5\n");                  \
    else printf(tag "=other\n"); } while(0)

int
main(void)
{
  for(int k = 1; k <= 3; k++){
    int pid = fork();
    if(pid == 0){
      if(k == 1){ SAY("GM: c1-pid", getpid()); pause(1); SAY("GM: c1-again", getpid()); }
      else if(k == 2){ SAY("GM: c2-pid", getpid()); pause(1); SAY("GM: c2-again", getpid()); }
      else { SAY("GM: c3-pid", getpid()); pause(1); SAY("GM: c3-again", getpid()); }
      exit(0);
    }
    SAY("GM: fork-ret", pid);
  }
  for(int k = 0; k < 3; k++)
    SAY("GM: wait-ret", wait(0));
  printf("GM: done\n");
  for(;;)
    ;
}
