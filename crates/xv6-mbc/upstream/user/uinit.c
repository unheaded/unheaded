// SPDX-License-Identifier: GPL-3.0-or-later
//
// uinit.c — UNHEADED-INIT: our own PID 1 (Track 2 Phase 2.1, ADR-081).
//
// The first Unheaded-authored program to run as PID 1 on the xv6-mbc
// kernel — the "evolve from xv6, never break the boot" track starts here.
// Talks to the kernel exclusively through the syscall ABI (user/user.h
// stubs); no xv6 userland logic is reused.
//
// Duties of PID 1 (kernel contract):
//   1. Give itself a console: open("console") — created via mknod on the
//      very first boot — then dup twice so fd 0/1/2 all reach the TTY.
//   2. Print the proof banner. The Phase 2.1 gate greps for the token
//      0xP1D1-0UR5: it proves OUR init ran, not xv6's.
//   3. Spawn the shell, then reap every zombie forever: orphaned
//      processes reparent to PID 1, so the wait() loop here is what keeps
//      the proc table from leaking. Restart sh whenever the process that
//      exited was the shell itself.
//
// Boot: upc-bootctl boot ... --userland target/uinit.mbc
// (same kernel + fs.img; only the resident PID 1 program changes).

#include "kernel/types.h"
#include "kernel/fcntl.h"
#include "user/user.h"

// Kernel ABI: device major number of the console device (kernel/file.h).
#define CONSOLE_MAJOR 1

char *sh_argv[] = { "sh", 0 };

int
main(void)
{
  // fd 0: the console. The very first boot has no inode for it yet —
  // create the device node, then retry.
  if(open("console", O_RDWR) < 0){
    mknod("console", CONSOLE_MAJOR, 0);
    if(open("console", O_RDWR) < 0)
      exit(1);              // no console — nothing sane left to do
  }
  dup(0);                   // fd 1: stdout
  dup(0);                   // fd 2: stderr

  // Fixed strings only — the same plain-string printf path init.c proved
  // at Phase 1.6.
  printf("UNHEADED-INIT 0xP1D1-0UR5: own PID 1 on the UPC\n");

  for(;;){
    printf("uinit: starting sh\n");
    int sh_pid = fork();
    if(sh_pid < 0){
      printf("uinit: fork failed\n");
      exit(1);
    }
    if(sh_pid == 0){
      exec("sh", sh_argv);
      printf("uinit: exec sh failed\n");
      exit(1);
    }
    // Reap until the shell itself dies; every other corpse is an orphan
    // reparented to us — collect it and keep waiting.
    for(;;){
      int dead = wait(0);
      if(dead == sh_pid)
        break;              // the shell exited — outer loop restarts it
      if(dead < 0){
        printf("uinit: wait error\n");
        exit(1);
      }
    }
  }
}
