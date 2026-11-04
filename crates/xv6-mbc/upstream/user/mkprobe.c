// SPDX-License-Identifier: GPL-3.0-or-later
// mkprobe.c: S-STAGE-2B mkdir rung. The kernel writes; BPF open and fstat read it back.
#include "kernel/types.h"
#include "kernel/stat.h"
#include "kernel/fcntl.h"
#include "user/user.h"

#define SAY(t, c) do { if(c) printf(t "=yes\n"); else printf(t "=no\n"); } while(0)

int
main(void)
{
  if(fork() == 0){
    struct stat st;
    SAY("MK: made", mkdir("mkd") == 0);
    SAY("MK: dup-refused", mkdir("mkd") == -1);
    SAY("MK: nested", mkdir("mkd/in") == 0);
    int fd = open("mkd", O_RDONLY);
    SAY("MK: open", fd >= 0);
    SAY("MK: is-dir", fd >= 0 && fstat(fd, &st) == 0 && st.type == T_DIR);
    close(fd);
    fd = open("mkd/in", O_RDONLY);
    SAY("MK: nested-open", fd >= 0);
    close(fd);
    SAY("MK: noparent-refused", mkdir("nope/in") == -1);
    exit(0);
  }
  wait(0);
  printf("MK: done\n");
  for(;;)
    ;
}
