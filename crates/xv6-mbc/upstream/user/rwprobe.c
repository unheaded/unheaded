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
//   "RW: slot0-literal-open=yes|no"       init itself opens a path whose bytes
//                               are in its first page, where init.c's literals
//                               are (decision pack 2026-11-18 row 25: copyinstr
//                               reads that page for slot 0, past BootParams),
//                               and reads the file
//   "RW: slot0-first-page-closed=yes|no"  the same page is still refused to
//                               every other copy: a read into it and a write
//                               from it both answer -1
//   "RW: done"                  init, after the child is reaped
#include "kernel/types.h"
#include "kernel/stat.h"
#include "kernel/fcntl.h"
#include "user/user.h"

#define SAY(t, c) do { if(c) printf(t "=yes\n"); else printf(t "=no\n"); } while(0)
#define FIRST_PAGE_STR 0x00000F00 // in slot 0's first page, past BootParams

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

  // Slot 0. init.c's and uinit.c's "console" literal sits in the first page
  // (their .rodata follows a .text shorter than a page). This image's .text is
  // longer than a page, so its literals are not there: the path is stored at
  // FIRST_PAGE_STR instead, which is RAM this image does not use (the code runs
  // from ROM; BootParams ends at 0x200) and which init reaches through its
  // identity map. The stored bytes are read back first, so the line cannot
  // pass on a store that did not land.
  char *lit = (char *)FIRST_PAGE_STR;
  char b[2];
  memmove(lit, "README", 7);
  int stored = lit[0] == 'R' && lit[5] == 'E' && lit[6] == 0;
  int lf = open(lit, O_RDONLY);
  SAY("RW: slot0-literal-open", stored && lf >= 0 && read(lf, b, 2) == 2);
  SAY("RW: slot0-first-page-closed", stored && read(lf, lit, 2) == -1
      && write(1, lit, 1) == -1 && lit[0] == 'R');
  printf("RW: done\n");
  for(;;)
    ;
}
