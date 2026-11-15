// SPDX-License-Identifier: GPL-3.0-or-later
// lnprobe.c: S-STAGE-2B link rung.
#include "kernel/types.h"
#include "kernel/stat.h"
#include "kernel/fcntl.h"
#include "user/user.h"

#define SAY(t, c) do { if(c) printf(t "=yes\n"); else printf(t "=no\n"); } while(0)

static int
nlink(char *path)
{
  struct stat st;
  int fd = open(path, O_RDONLY);
  if(fd < 0)
    return -1;
  int r = fstat(fd, &st) == 0 ? st.nlink : -1;
  close(fd);
  return r;
}

// S-STAGE-2C Phase 8 (A-62): link's count, read back through fstat, and the
// bytes still there under the second name once the first is gone.
static void
counts(void)
{
  char got[32]; // BPF's file read copies a full 16-byte chunk (see rmprobe)
  int fd = open("lnsrc", O_CREATE | O_WRONLY);
  write(fd, "linkdata", 8);
  close(fd);
  link("lnsrc", "lndst");
  SAY("LN: nlink-two", nlink("lnsrc") == 2 && nlink("lndst") == 2);
  unlink("lnsrc");
  SAY("LN: nlink-one", nlink("lndst") == 1);
  fd = open("lndst", O_RDONLY);
  SAY("LN: bytes-after-unlink", fd >= 0 && read(fd, got, 8) == 8 && memcmp(got, "linkdata", 8) == 0);
  close(fd);
}

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
    counts();
    exit(0);
  }
  wait(0);
  printf("LN: done\n");
  for(;;)
    ;
}
