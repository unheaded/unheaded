// SPDX-License-Identifier: GPL-3.0-or-later
// lnprobe.c: S-STAGE-2B link rung.
#include "kernel/types.h"
#include "kernel/stat.h"
#include "kernel/fcntl.h"
#include "user/user.h"

#define SAY(t, c) do { if(c) printf(t "=yes\n"); else printf(t "=no\n"); } while(0)

int
main(void)
{
  if(fork() == 0){
    char a[4], b[4];
    int fd = open("README", O_RDONLY);
    read(fd, a, 4);
    close(fd);
    SAY("LN: made", link("README", "lnk") == 0);
    SAY("LN: dup-refused", link("README", "lnk") == -1);
    SAY("LN: missing-refused", link("nope", "x") == -1);
    SAY("LN: dir-refused", link("sub", "subl") == -1);
    fd = open("lnk", O_RDONLY);
    SAY("LN: open", fd >= 0);
    SAY("LN: same-bytes", fd >= 0 && read(fd, b, 4) == 4 && memcmp(a, b, 4) == 0);
    close(fd);
    SAY("LN: unlinked", unlink("lnk") == 0);
    SAY("LN: gone", open("lnk", O_RDONLY) < 0);
    fd = open("README", O_RDONLY);
    SAY("LN: original-kept", fd >= 0);
    close(fd);
    exit(0);
  }
  wait(0);
  printf("LN: done\n");
  for(;;)
    ;
}
