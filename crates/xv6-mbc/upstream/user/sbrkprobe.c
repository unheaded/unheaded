// SPDX-License-Identifier: GPL-3.0-or-later
// sbrkprobe.c: S-STAGE-2B sbrk rung. Runs as init; the heap belongs to a child.
#include "kernel/types.h"
#include "kernel/stat.h"
#include "user/user.h"

#define SAY(t, c) do { if(c) printf(t "=yes\n"); else printf(t "=no\n"); } while(0)

int
main(void)
{
  SAY("SB: init-refused", sbrk(4096) == SBRK_ERROR);
  int pid = fork();
  if(pid == 0){
    char *a = sbrk(0);
    SAY("SB: base", a == (char *)0x10000);
    char *b = sbrk(0x19000); // past physical 0x27900: a fork from init copies kernel .rodata here
    SAY("SB: grow", b == a);
    SAY("SB: moved", sbrk(0) == a + 0x19000);
    int zero = 1;
    for(int i = 0; i < 0x19000; i++)
      if(b[i] != 0)
        zero = 0;
    SAY("SB: zeroed", zero);
    for(int i = 0; i < 4096; i++)
      b[i] = (char)(i * 7);
    int c = fork();
    if(c == 0){
      int same = 1;
      for(int i = 0; i < 4096; i++)
        if(b[i] != (char)(i * 7))
          same = 0;
      SAY("SB: fork-copied", same);
      exit(0);
    }
    wait(0);
    SAY("SB: shrink", sbrk(-0x19000) == a + 0x19000);
    SAY("SB: shrunk", sbrk(0) == a);
    SAY("SB: over-cap-refused", sbrk(0x30001) == SBRK_ERROR);
    SAY("SB: under-base-refused", sbrk(-1) == SBRK_ERROR);
    SAY("SB: intmin-refused", sbrk(-2147483647 - 1) == SBRK_ERROR);
    SAY("SB: unchanged", sbrk(0) == a);
    char *m = malloc(1000);
    SAY("SB: malloc", m != 0);
    if(m){
      memset(m, 0x5A, 1000);
      free(m);
    }
    exit(0);
  }
  wait(0);
  printf("SB: done\n");
  for(;;)
    ;
}
