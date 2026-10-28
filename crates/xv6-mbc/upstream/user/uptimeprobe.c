// uptimeprobe.c — EVOLUTION-1 Stage 2 rung 2 (ADR-110): uptime() in init and
// in a forked child, before and after busy work. With UPC_RUNG2 on, uptime is
// answered by the owned kernel from BPF's published tick count; off, BPF has
// no arm and uptime returns stale a0.
// Values print as fixed classes (the MBC userland varargs ABI is unreliable):
//   "UP: <who>-<when>=zero|small|large|neg"   small = 1..1000
//   "UP: <who>-moved=yes|no"   later call returned a larger value
//   "UP: child-mono=yes|no"    the child's first value >= the parent's last
#include "kernel/types.h"
#include "kernel/stat.h"
#include "user/user.h"

#define CLASS(tag, v) do { int v_ = (v);                  \
    if(v_ == 0) printf(tag "=zero\n");                    \
    else if(v_ < 0) printf(tag "=neg\n");                 \
    else if(v_ <= 1000) printf(tag "=small\n");           \
    else printf(tag "=large\n"); } while(0)

static volatile int sink;

static void
busy(void)
{
  for(int i = 0; i < 20000; i++)
    sink += i;
}

int
main(void)
{
  int a = uptime();
  busy();
  pause(1);
  int b = uptime();
  CLASS("UP: init-before", a);
  CLASS("UP: init-after", b);
  if(b > a) printf("UP: init-moved=yes\n"); else printf("UP: init-moved=no\n");
  int pid = fork();
  if(pid == 0){
    int c = uptime();
    busy();
    pause(1);
    int d = uptime();
    CLASS("UP: child-before", c);
    CLASS("UP: child-after", d);
    if(d > c) printf("UP: child-moved=yes\n"); else printf("UP: child-moved=no\n");
    if(c >= b) printf("UP: child-mono=yes\n"); else printf("UP: child-mono=no\n");
    exit(0);
  }
  wait(0);
  printf("UP: done\n");
  for(;;)
    ;
}
