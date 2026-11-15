// SPDX-License-Identifier: GPL-3.0-or-later
// hostile_atomic.c: S-STAGE-2C Phase 4, ADR-135 D17 (R-S4). The source of a
// hostile image, not a probe that runs as built.
//
// A child loads the word at VA 0x00800400. That is above its 8 MiB ceiling, so
// with ADR-109 D6 on a translated load reads 0. Physical 0x00800400 is inside
// the ramdisk (fs.img sits at 0x00800000), where the superblock's bytes are not
// 0. Built normally the load is a plain LD and reads 0.
//
// scripts/upc-mbc-patch-opcode.py turns that one LD into XCHG in a copy of the
// image (the translator emits no atomic, so none can be compiled). An XCHG that
// is not translated reads the ramdisk's word. bootctl's opcode scan refuses the
// patched copy at load; the harness checks that refusal and never boots it.
//
// Observables:
//   "HA: xchg-translated=yes|no"  the word read was 0
//   "HA: done"                    init, after the child is reaped
#include "kernel/types.h"
#include "kernel/stat.h"
#include "user/user.h"

#define MARKER_VA 0x00800400u

int
main(void)
{
  if(fork() == 0){
    volatile uint marker = *(volatile uint *)MARKER_VA;
    if(marker == 0)
      printf("HA: xchg-translated=yes\n");
    else
      printf("HA: xchg-translated=no\n");
    exit(0);
  }
  wait(0);
  printf("HA: done\n");
  for(;;)
    ;
}
