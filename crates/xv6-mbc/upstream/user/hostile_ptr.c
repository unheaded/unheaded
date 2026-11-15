// SPDX-License-Identifier: GPL-3.0-or-later
// hostile_ptr.c: S-STAGE-2B user-pointer rung. A child hands the kernel bad paths.
//
// The child is slot 1. A path at or past its ceiling only proves something if
// the kernel would have found a name there with no bound, so a grandchild (slot
// 2, the next slice up) first leaves real names at its VA 0, 16, 32 and 48: the
// child's VA 0x800000 and up are those bytes when the pointer is added to the
// slice base unchecked (security pass review, finding 4: the next slot's VA 0
// held a NUL, and the empty path fails by itself). README is linked before it is
// unlinked, so a kernel that follows the pointers fails every line.
#include "kernel/types.h"
#include "kernel/stat.h"
#include "user/user.h"

#define SAY(t, c) do { if(c) printf(t "=yes\n"); else printf(t "=no\n"); } while(0)

static void
put(volatile char *d, const char *s)
{
  int i = 0;
  do
    d[i] = s[i];
  while(s[i++]);
}

int
main(void)
{
  if(fork() == 0){
    if(fork() == 0){
      put((volatile char *)0, "README");
      put((volatile char *)16, "zz");
      put((volatile char *)32, "sub");
      put((volatile char *)48, "qq");
      exit(0);
    }
    wait(0);
    volatile char *top = (volatile char *)0x007FFFF0;
    for(int i = 0; i < 16; i++)
      top[i] = 'x'; // no NUL before the ceiling
    SAY("HU: unterminated-refused", mkdir((char *)0x007FFFF0) == -1);
    SAY("HU: ceiling-refused", mkdir((char *)0x00800030) == -1); // "qq"
    SAY("HU: kernel-refused", mkdir((char *)0x00F00000) == -1);
    SAY("HU: wrap-refused", mkdir((char *)0xFFFFFFF0) == -1);
    // S-STAGE-2C Phase 8 (A-62): the other calls that take a path.
    char *bad = (char *)0x00800000;                                       // "README"
    SAY("HU: link-old-bad", link(bad, "x") == -1 && open("x", 0) == -1);
    SAY("HU: link-new-bad", link("README", (char *)0x00800010) == -1);    // "zz"
    SAY("HU: unlink-bad", unlink(bad) == -1);
    SAY("HU: chdir-bad", chdir((char *)0x00800020) == -1);                // "sub"
    SAY("HU: alive", getpid() == 2);
    exit(0);
  }
  wait(0);
  printf("HU: done\n");
  for(;;)
    ;
}
