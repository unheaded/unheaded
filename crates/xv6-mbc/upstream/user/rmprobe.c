// SPDX-License-Identifier: GPL-3.0-or-later
// rmprobe.c: S-STAGE-2B unlink rung.
#include "kernel/types.h"
#include "kernel/stat.h"
#include "kernel/fcntl.h"
#include "user/user.h"

#define SAY(t, c) do { if(c) printf(t "=yes\n"); else printf(t "=no\n"); } while(0)

int
main(void)
{
  if(fork() == 0){
    mkdir("rmd");
    mkdir("rmd/in");
    SAY("RM: nonempty-refused", unlink("rmd") == -1);
    SAY("RM: inner", unlink("rmd/in") == 0);
    SAY("RM: inner-gone", open("rmd/in", O_RDONLY) < 0);
    SAY("RM: outer", unlink("rmd") == 0);
    SAY("RM: outer-gone", open("rmd", O_RDONLY) < 0);
    SAY("RM: missing-refused", unlink("nope") == -1);
    SAY("RM: dot-refused", unlink(".") == -1);
    SAY("RM: dotdot-refused", unlink("..") == -1);
    exit(0);
  }
  wait(0);
  printf("RM: done\n");
  for(;;)
    ;
}
