// SPDX-License-Identifier: GPL-3.0-or-later
// conprobe.c: S-STAGE-2C rung 3 (ADR-135 D19). With UPC_RUNG3 on, write() on a
// console fd is answered by the owned kernel: BPF routes the call, sys_write
// copies the bytes in through the slot's page table and consolewrite stores
// them on the TTY one byte each. Before the rung, BPF's own loop did it.
//
// Run as the userland directly, so this program is init (slot 0). Slot 0's
// first page reads as physical 0 to the kernel's copyin, which refuses it
// (ADR-135 D6), so every buffer here is in .bss, never a string literal.
//
// Observables:
//   "CW: ret-four=yes|no"    write(1, "abc\n", 4) returns 4
//   "CW: long-write=yes|no"  one 600-byte write returns 600
//   "CW: zero=yes|no"        a write of 0 bytes returns 0
//   "CW: badptr=yes|no"      a child's write from an unmapped address returns -1
//   "CW: done"               init, after the child is reaped
#include "kernel/types.h"
#include "kernel/stat.h"
#include "user/user.h"

#define SAY(t, c) do { if(c) printf(t "=yes\n"); else printf(t "=no\n"); } while(0)

#define LONG       600
#define UNMAPPED   ((char *)0x00900000)  // past a child's 8 MiB of mapped VA

static char abc[4];
static char big[LONG];

int
main(void)
{
  abc[0] = 'a'; abc[1] = 'b'; abc[2] = 'c'; abc[3] = '\n';
  for(int i = 0; i < LONG - 1; i++)
    big[i] = 'x';
  big[LONG - 1] = '\n';

  int four = write(1, abc, 4);
  SAY("CW: ret-four", four == 4);
  int six = write(1, big, LONG);
  SAY("CW: long-write", six == LONG);
  SAY("CW: zero", write(1, abc, 0) == 0);

  if(fork() == 0){
    SAY("CW: badptr", write(1, UNMAPPED, 4) == -1);
    exit(0);
  }
  wait(0);
  printf("CW: done\n");
  for(;;)
    ;
}
