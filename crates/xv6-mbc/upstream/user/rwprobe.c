// SPDX-License-Identifier: GPL-3.0-or-later
// rwprobe.c: S-STAGE-2C rungs 5+6 (ADR-135 D21). With UPC_RUNG56 on every fd
// is the owned kernel's: open, read, write, fstat, dup and close all go through
// its file table, and BPF keeps none. Three things that were not so before:
// a plain O_RDWR open of a regular file is a file that can be written (T-F3
// closes), dup can land on 0, 1 or 2, and the console is a kernel file like
// any other.
//
// Run as the userland directly; the work is a child's, so its literal paths
// are in its own slice (ADR-135 D6). fds 0-2 are the console the kernel gave
// the first image.
//
// Observables:
//   "RW: write-lands=yes|no"    open("README", O_RDWR); write of 2 bytes at
//                               offset 0 answers 2
//   "RW: read-back=yes|no"      a second fd reads those 2 bytes
//   "RW: restored=yes|no"       the first 2 bytes written back and read again
//   "RW: console-fstat=yes|no"  fstat on the console answers (-1: it has no
//                               inode) and the kernel goes on
//   "RW: dup-to-zero=yes|no"    close(0); dup(fd) answers 0
//   "RW: done"                  init, after the child is reaped
#include "kernel/types.h"
#include "kernel/stat.h"
#include "kernel/fcntl.h"
#include "user/user.h"

#define SAY(t, c) do { if(c) printf(t "=yes\n"); else printf(t "=no\n"); } while(0)

static int
first2(char *b)
{
  int r = open("README", O_RDONLY);
  int n = read(r, b, 2);
  close(r);
  return n;
}

int
main(void)
{
  if(fork() == 0){
    char was[2], got[2];
    struct stat st;

    first2(was);
    int fd = open("README", O_RDWR);
    SAY("RW: write-lands", fd >= 0 && write(fd, "zz", 2) == 2);
    SAY("RW: read-back", first2(got) == 2 && got[0] == 'z' && got[1] == 'z');
    close(fd);

    fd = open("README", O_RDWR);
    write(fd, was, 2);
    SAY("RW: restored", first2(got) == 2 && got[0] == was[0] && got[1] == was[1]
        && !(was[0] == 'z' && was[1] == 'z'));

    SAY("RW: console-fstat", fstat(1, &st) == -1);

    close(0);
    SAY("RW: dup-to-zero", dup(fd) == 0);
    exit(0);
  }
  wait(0);
  printf("RW: done\n");
  for(;;)
    ;
}
