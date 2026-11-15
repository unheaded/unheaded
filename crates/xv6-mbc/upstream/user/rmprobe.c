// SPDX-License-Identifier: GPL-3.0-or-later
// rmprobe.c: S-STAGE-2B unlink rung.
#include "kernel/types.h"
#include "kernel/stat.h"
#include "kernel/fcntl.h"
#include "user/user.h"

#define SAY(t, c) do { if(c) printf(t "=yes\n"); else printf(t "=no\n"); } while(0)

#define DATA 2500 // three blocks: a file with data, not an empty inode

#define PAD 32    // guard bytes after the read buffer
#define GUARD 0x5A

static char blk[DATA];
static char got[DATA + PAD];

// S-STAGE-2C Phase 8 (A-62): unlink of a file that holds blocks frees them, and
// the next file to take them reads back its own bytes, not the old ones.
static void
with_data(void)
{
  for(int i = 0; i < DATA; i++)
    blk[i] = 'a' + i % 23;
  int fd = open("rmf", O_CREATE | O_WRONLY);
  write(fd, blk, DATA);
  close(fd);
  SAY("RM: file-with-data-unlinked", unlink("rmf") == 0);
  SAY("RM: gone", open("rmf", O_RDONLY) == -1);
  for(int i = 0; i < DATA; i++)
    blk[i] = 'A' + i % 19;
  fd = open("rmg", O_CREATE | O_WRONLY);
  write(fd, blk, DATA);
  close(fd);
  for(int i = 0; i < PAD; i++)
    got[DATA + i] = GUARD;
  fd = open("rmg", O_RDONLY);
  int n = 0, r;
  while(n < DATA && (r = read(fd, got + n, DATA - n)) > 0)
    n += r;
  close(fd);
  SAY("RM: blocks-reused-clean", n == DATA && memcmp(blk, got, DATA) == 0);
  // A note, not a check: how many bytes a read wrote past the count it
  // returned. BPF's file read copies a fixed 16-byte chunk (main.rs, H-FS6),
  // so the last short read of a file writes up to 15 bytes past the end of
  // what it reports, taken from the block past the file's size. The kernel's
  // read (rungs 5+6) copies exactly n; this line reads 0 then.
  int past = 0;
  for(int i = 0; i < PAD; i++)
    if(got[DATA + i] != GUARD)
      past++;
  printf("RM: read-past-end=%d\n", past);
}

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
    with_data();
    exit(0);
  }
  wait(0);
  printf("RM: done\n");
  for(;;)
    ;
}
