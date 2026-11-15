// SPDX-License-Identifier: GPL-3.0-or-later
// sleepprobe.c: S-STAGE-2B sleep rung. pause(n) means n UPC ticks (ADR-110).
#include "kernel/types.h"
#include "kernel/stat.h"
#include "user/user.h"
#include "user/uyield.h"

#define SAY(t, c) do { if(c) printf(t "=yes\n"); else printf(t "=no\n"); } while(0)

int
main(void)
{
  int a = uptime();
  SAY("SL: zero", pause(0) == 0);
  SAY("SL: neg", pause(-5) == 0);
  SAY("SL: returned", pause(300) == 0);
  SAY("SL: waited", uptime() - a >= 300);
  // S-STAGE-2C Phase 8 (A-62): the tick count across one pause, and how late it woke.
  int t0 = uptime();
  pause(50);
  int t1 = uptime();
  SAY("SL: pause-ticks", t1 - t0 >= 50);
  printf("SL: late-by=%d\n", t1 - t0 - 50);
  int c = fork();
  if(c == 0){
    pause(2000);
    printf("SL: child-woke\n");
    exit(0);
  }
  yield(); // the child is now inside pause(); this slot still runs
  printf("SL: parent-ran\n");
  wait(0);
  printf("SL: done\n");
  for(;;)
    ;
}
