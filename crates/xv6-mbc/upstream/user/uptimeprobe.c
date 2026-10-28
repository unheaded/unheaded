// uptimeprobe.c — EVOLUTION-1 Stage 2 rung 2 research: what uptime() returns
// today, and whether it moves across pause(1), in init and in a forked child.
// Values print as fixed classes (the MBC userland varargs ABI is unreliable):
//   "UP: <who>-<when>=zero|small|large|neg"   small = 1..1000
//   "UP: <who>-moved=yes|no"                   after != before
#include "kernel/types.h"
#include "kernel/stat.h"
#include "user/user.h"

#define CLASS(tag, v) do { int v_ = (v);                  \
    if(v_ == 0) printf(tag "=zero\n");                    \
    else if(v_ < 0) printf(tag "=neg\n");                 \
    else if(v_ <= 1000) printf(tag "=small\n");           \
    else printf(tag "=large\n"); } while(0)

int
main(void)
{
  int a = uptime();
  pause(1);
  int b = uptime();
  CLASS("UP: init-before", a);
  CLASS("UP: init-after", b);
  if(a != b) printf("UP: init-moved=yes\n"); else printf("UP: init-moved=no\n");
  int pid = fork();
  if(pid == 0){
    int c = uptime();
    pause(1);
    int d = uptime();
    CLASS("UP: child-before", c);
    CLASS("UP: child-after", d);
    if(c != d) printf("UP: child-moved=yes\n"); else printf("UP: child-moved=no\n");
    exit(0);
  }
  wait(0);
  printf("UP: done\n");
  for(;;)
    ;
}
