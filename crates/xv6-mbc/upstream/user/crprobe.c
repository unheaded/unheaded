// SPDX-License-Identifier: GPL-3.0-or-later
// crprobe.c: S-STAGE-2C rung 4 (ADR-135 D20). With UPC_RUNG4 on, read() on a
// console fd is answered by the owned kernel: BPF routes the call and offers
// one keyboard-ring byte per trap, the kernel runs it through consoleintr's
// line buffer and returns a line at a time, as stock xv6 does. Before the
// rung, BPF's own branch returned one byte per call.
//
// Boot with --input $'init\nhello\nagain\n'. Run as the userland directly, so
// this program is init (slot 0). The first read is init's own: slot 0 is the
// one row the kernel's sleep() does not refuse, so a console read that reached
// the sleeping consoleread from it would stop the machine (ADR-135 D21, review
// F1). The other reads are a child's, like sh's are.
//
// Observables:
//   "CR: slot0-line=yes|no"   init's read(fd, buf, 64) returns "init\n", 5 bytes
//   "CR: line=yes|no"         read(0, buf, 64) returns "hello\n", 6 bytes
//   "CR: second-line=yes|no"  the next read returns "again\n", 6 bytes
//   "CR: badptr=yes|no"       a read into an unmapped address returns -1,
//                             with no input left to wait for
//   "CR: done"                init, after the child is reaped
#include "kernel/types.h"
#include "kernel/stat.h"
#include "kernel/fcntl.h"
#include "user/user.h"

#define SAY(t, c) do { if(c) printf(t "=yes\n"); else printf(t "=no\n"); } while(0)

#define UNMAPPED   ((char *)0x00900000)  // past a child's 8 MiB of mapped VA

// In .data, not a string literal: this open is init's, and the path is read
// from init's image (see devprobe.c).
static char con[] = "console";

int
main(void)
{
  char first[64];              // on init's stack, inside slot 0's window (D16)
  int fd = open(con, O_RDWR);  // the console: BPF's fd 0, or the kernel's lowest free (D21)
  int n0 = read(fd, first, sizeof(first));
  SAY("CR: slot0-line", n0 == 5 && memcmp(first, "init\n", 5) == 0);
  if(fork() == 0){
    char buf[64];
    int n = read(fd, buf, sizeof(buf));
    SAY("CR: line", n == 6 && memcmp(buf, "hello\n", 6) == 0);
    n = read(fd, buf, sizeof(buf));
    SAY("CR: second-line", n == 6 && memcmp(buf, "again\n", 6) == 0);
    SAY("CR: badptr", read(fd, UNMAPPED, 8) == -1);
    exit(0);
  }
  wait(0);
  printf("CR: done\n");
  for(;;)
    ;
}
