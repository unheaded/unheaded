// SPDX-License-Identifier: GPL-3.0-or-later
// killprobe.c: S-STAGE-2B kill rung. Runs as init. Exit status is not checked:
// BPF wait writes 0 until rung 7.
#include "kernel/types.h"
#include "kernel/stat.h"
#include "user/user.h"
#include "user/uyield.h"

#define SAY(t, c) do { if(c) printf(t "=yes\n"); else printf(t "=no\n"); } while(0)

static void
spin(void)
{
  for(;;)
    yield();
}

int
main(void)
{
  SAY("KL: zero-refused", kill(0) == -1);
  SAY("KL: neg-refused", kill(-1) == -1);
  SAY("KL: huge-refused", kill(999) == -1);
  SAY("KL: init-refused", kill(1) == -1);
  int c = fork();
  if(c == 0)
    spin();
  yield();
  SAY("KL: kill-ok", kill(c) == 0);
  SAY("KL: reaped", wait(0) == c);
  SAY("KL: dead-refused", kill(c) == -1);
  int s = fork();
  if(s == 0){
    kill(getpid());
    printf("KL: self-survived=yes\n");
    exit(0);
  }
  SAY("KL: self-reaped", wait(0) == s);
  int p = fork();
  if(p == 0){
    if(fork() == 0)
      spin();
    spin();
  }
  yield();
  yield();
  SAY("KL: parent-killed", kill(p) == 0);
  SAY("KL: parent-reaped", wait(0) == p);
  SAY("KL: orphan-killed", kill(p + 1) == 0);
  SAY("KL: orphan-reaped", wait(0) == p + 1);
  printf("KL: done\n");
  for(;;)
    ;
}
