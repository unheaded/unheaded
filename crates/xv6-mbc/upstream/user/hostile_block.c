// SPDX-License-Identifier: GPL-3.0-or-later
// hostile_block.c: S-STAGE-2C, ADR-135 Step 29 review finding RV-1. The block
// device's two syscalls, 200 (read a ramdisk block into memory) and 201 (write
// memory to a ramdisk block), are the kernel's: its buffer cache calls them from
// S-mode with physical addresses. Nothing kept a user program from making the
// same ecall, with any physical address as the buffer.
//
// Run as the userland directly, so this program is init (slot 0). Init's base is
// 0, so the address of one of its own buffers is that buffer's physical address,
// and a forked child sees the same number.
//
//   write  The child asks 200 to read the superblock's block into init's canary:
//          a store outside the child's slice.
//   read   The child asks 201 to write init's secret into a spare ramdisk block,
//          then 200 to read that block into its own buffer by physical address:
//          a load from outside its slice.
//
// Observables:
//   "HB: calls-refused=yes|no"          all three ecalls answered a negative number
//   "HB: read-outside-refused=yes|no"   the child does not hold init's secret
//   "HB: write-outside-refused=yes|no"  init's canary is as init left it
//   "HB: done"                          init, after the child is reaped
#include "kernel/types.h"
#include "kernel/stat.h"
#include "user/user.h"

#define SAY(t, c) do { if(c) printf(t "=yes\n"); else printf(t "=no\n"); } while(0)

#define WORDS        128          // one 512-byte block
#define SUPERBLOCK   1
#define SPARE_BLOCK  8000         // past fs.img (2 MiB), inside the 4 MiB ramdisk
#define CHILD_BASE   0x01000000u  // slot 1's slice: phase12 USER_CHILD_SLICE

static uint canary[WORDS];  // init writes it, the child aims 200 at it
static uint secret[WORDS];  // init writes it, the child aims 201 at it
static uint mine[WORDS];    // the child's own buffer

// a7 = number, a0 = block, a1 = buffer byte address, a2 = 0 (the call's cursor).
static int
blockcall(int nr, uint block, uint addr)
{
  register int a0 __asm__("a0") = block;
  register uint a1 __asm__("a1") = addr;
  register uint a2 __asm__("a2") = 0;
  register int a7 __asm__("a7") = nr;
  __asm__ volatile("ecall" : "+r"(a0), "+r"(a2) : "r"(a1), "r"(a7) : "memory");
  return a0;
}

int
main(void)
{
  for(uint i = 0; i < WORDS; i++){
    canary[i] = 0xCA5A0000u | i;
    secret[i] = 0x5EC20000u | i;
  }
  if(fork() == 0){
    for(uint i = 0; i < WORDS; i++){
      mine[i] = 0;
      secret[i] = 0; // the child's copy: what it may know is only what it steals
    }
    int w = blockcall(200, SUPERBLOCK, (uint)canary);
    int s = blockcall(201, SPARE_BLOCK, (uint)secret);
    int r = blockcall(200, SPARE_BLOCK, CHILD_BASE + (uint)mine);
    SAY("HB: calls-refused", w < 0 && s < 0 && r < 0);
    int stolen = 1;
    for(uint i = 0; i < WORDS; i++)
      if(mine[i] != (0x5EC20000u | i))
        stolen = 0;
    SAY("HB: read-outside-refused", !stolen);
    exit(0);
  }
  wait(0);
  int intact = 1;
  for(uint i = 0; i < WORDS; i++)
    if(canary[i] != (0xCA5A0000u | i))
      intact = 0;
  SAY("HB: write-outside-refused", intact);
  printf("HB: done\n");
  for(;;)
    ;
}
