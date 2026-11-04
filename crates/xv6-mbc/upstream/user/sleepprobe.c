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
