// devprobe.c — S-ASCEND-2 Phase 7 (Step 184): open the console by name.
//
// Runs as init (pid 1, BPF slot 0), with no fd open yet. Opens "console" read-write and
// writes a token through the fd that open() returned (not a hard-wired 1),
// then a second token through a dup of it. Fixed strings only.
//
// Expected: "DEV: console-open-ok" and "DEV: console-dup-ok", once each.
// The parent spins after its last line (init never exits).
//
// The tokens are in .data, not string literals. Since rung 3 (ADR-135 D19) the
// owned kernel copies a console write's bytes in, and it refuses init's first
// page (D6, D16), where user.ld puts .rodata. A literal here would print
// nothing, which is the slot 0 limit D6 records, not what this probe is for.
//
//   upc-bootctl boot ... --userland target/devprobe.mbc

#include "kernel/types.h"
#include "kernel/stat.h"
#include "kernel/fcntl.h"
#include "user/user.h"

static char open_ok[] = "DEV: console-open-ok\n";
static char dup_ok[] = "DEV: console-dup-ok\n";

int
main(void)
{
  int fd = open("console", O_RDWR);
  if(fd < 0){
    // No other fd is open, so nothing can print this; the missing token is
    // the failure signal.
    for(;;)
      ;
  }
  write(fd, open_ok, 21);
  int fd2 = dup(fd);
  if(fd2 >= 0 && fd2 != fd)
    write(fd2, dup_ok, 20);
  for(;;)
    ;
}
