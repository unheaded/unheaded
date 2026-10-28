// getpidloop.c — EVOLUTION-1 Stage 2 rung 0 (plan Step 61): slot reuse.
//
// Three sequential forks, one child live at a time, so BPF reuses slot 1 for
// each child. With the flag ON and ADR-109 in place, each child's first trap
// must refresh kernel row 1 (I3); every child's getpid must equal fork()'s
// return in the parent (option K: 2 every time).
//
// Observables (fixed strings, getpidfork.c style; SAY expands inline so no
// helper takes a pointer, which the MBC userland ABI does not survive):
//   "GL: child-pid=N"  the child's getpid
//   "GL: fork-ret=N"   fork()'s return in the parent
//   "GL: wait-ret=N"   wait()'s return
//   "GL: done"         after the third iteration; then init spins
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
  for(int i = 0; i < 3; i++){
    int pid = fork();
    if(pid == 0){
      SAY("GL: child-pid", getpid());
      exit(0);
    }
    SAY("GL: fork-ret", pid);
    SAY("GL: wait-ret", wait(0));
  }
  printf("GL: done\n");
  for(;;)
    ;
}
