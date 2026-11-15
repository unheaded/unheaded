// SPDX-License-Identifier: GPL-3.0-or-later
// bigprobe.c: S-STAGE-2C Phase 8 (A-62). A file past one block, and past the
// direct blocks: 14,000 bytes crosses NDIRECT * BSIZE (12 * 1024 = 12,288), so
// its tail is in the single-indirect block. The kernel writes it (O_CREATE is
// a masked flag, ADR-135 D11), and a read-only open reads it back.
//
// Observables:
//   "BG: size=yes|no"       fstat says 14,000 after the writes
//   "BG: read-back=yes|no"  every byte read back is the one written
//   "BG: trunc=yes|no"      O_WRONLY|O_TRUNC leaves size 0
//   "BG: done"              init, after the child is reaped
#include "kernel/types.h"
#include "kernel/stat.h"
#include "kernel/fcntl.h"
#include "user/user.h"

#define SAY(t, c) do { if(c) printf(t "=yes\n"); else printf(t "=no\n"); } while(0)

#define TOTAL 14000
#define CHUNK 300

static char buf[CHUNK];

static char
at(int pos)
{
  return (char)(pos ^ (pos >> 8));
}

static int
fsize(char *path)
{
  struct stat st;
  int fd = open(path, O_RDONLY);
  if(fd < 0)
    return -1;
  int r = fstat(fd, &st) == 0 ? (int)st.size : -1;
  close(fd);
  return r;
}

int
main(void)
{
  if(fork() == 0){
    int fd = open("big", O_CREATE | O_WRONLY);
    int wrote = 0;
    while(wrote < TOTAL){
      int n = TOTAL - wrote < CHUNK ? TOTAL - wrote : CHUNK;
      for(int i = 0; i < n; i++)
        buf[i] = at(wrote + i);
      if(write(fd, buf, n) != n)
        break;
      wrote += n;
    }
    close(fd);
    SAY("BG: size", wrote == TOTAL && fsize("big") == TOTAL);

    fd = open("big", O_RDONLY);
    int pos = 0, n, same = fd >= 0;
    while(same && pos < TOTAL && (n = read(fd, buf, CHUNK)) > 0){
      for(int i = 0; i < n; i++)
        if(buf[i] != at(pos + i))
          same = 0;
      pos += n;
    }
    close(fd);
    SAY("BG: read-back", same && pos == TOTAL);

    fd = open("big", O_WRONLY | O_TRUNC);
    close(fd);
    SAY("BG: trunc", fd >= 0 && fsize("big") == 0);
    exit(0);
  }
  wait(0);
  printf("BG: done\n");
  for(;;)
    ;
}
