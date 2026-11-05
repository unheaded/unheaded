// SPDX-License-Identifier: GPL-3.0-or-later
// hostile_ptr.c: S-STAGE-2B user-pointer rung. A child hands the kernel bad paths.
#include "kernel/types.h"
#include "kernel/stat.h"
#include "user/user.h"

#define SAY(t, c) do { if(c) printf(t "=yes\n"); else printf(t "=no\n"); } while(0)

int
main(void)
{
  if(fork() == 0){
    volatile char *top = (volatile char *)0x007FFFF0;
    for(int i = 0; i < 16; i++)
      top[i] = 'x'; // no NUL before the ceiling
    SAY("HU: unterminated-refused", mkdir((char *)0x007FFFF0) == -1);
    SAY("HU: ceiling-refused", mkdir((char *)0x00800000) == -1);
    SAY("HU: kernel-refused", mkdir((char *)0x00F00000) == -1);
    SAY("HU: wrap-refused", mkdir((char *)0xFFFFFFF0) == -1);
    SAY("HU: alive", getpid() == 2);
    exit(0);
  }
  wait(0);
  printf("HU: done\n");
  for(;;)
    ;
}
