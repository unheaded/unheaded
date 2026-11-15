// SPDX-License-Identifier: GPL-3.0-or-later
// bssprobe.c: S-STAGE-2C Phase 5, ADR-135 R-NC (ADR-077 exec ABI). An exec'd
// image starts with its .bss zero. usertests' bsstest, in probe form.
//
// Booted as the userland it is init (argc 1): it dirties its own .bss, forks, and
// the child execs "bssprobe" with one argument. exec copies the resident program's
// data over the child's slice, which fork filled with the parent's dirty bytes.
// With an argument the program is that exec'd image: every word of its .bss array
// must be 0, and malloc, whose free list lives in .bss, must work.
//
// Observables:
//   "BS: bss-zero=yes|no"  the exec'd image's .bss array is all 0
//   "BS: malloc=yes|no"    malloc in the exec'd image returns usable memory
//   "BS: done"             init, after the child is reaped
#include "kernel/types.h"
#include "kernel/stat.h"
#include "user/user.h"

#define WORDS 256
#define DIRT  0xA5A5A5A5u

static uint bss[WORDS]; // no initializer: .bss

int
main(int argc, char *argv[])
{
  if(argc > 1){
    int zero = 1;
    for(int i = 0; i < WORDS; i++)
      if(bss[i] != 0)
        zero = 0;
    if(zero)
      printf("BS: bss-zero=yes\n");
    else
      printf("BS: bss-zero=no\n");

    char *m = malloc(64);
    int ok = m != 0;
    if(ok){
      for(int i = 0; i < 64; i++)
        m[i] = (char)i;
      for(int i = 0; i < 64; i++)
        if(m[i] != (char)i)
          ok = 0;
      free(m);
    }
    if(ok)
      printf("BS: malloc=yes\n");
    else
      printf("BS: malloc=no\n");
    exit(0);
  }

  for(int i = 0; i < WORDS; i++)
    bss[i] = DIRT;
  // malloc's statics are .bss too: one allocation dirties them. Init has no heap
  // (ADR-135 D4), so the dirtying happens in a child, which then forks the exec.
  if(fork() == 0){
    char *m = malloc(32);
    if(m)
      m[0] = 1;
    if(fork() == 0){
      char *args[] = { "bssprobe", "x", 0 };
      exec("bssprobe", args);
      printf("BS: bss-zero=no\n"); // exec failed
      exit(1);
    }
    wait(0);
    exit(0);
  }
  wait(0);
  printf("BS: done\n");
  for(;;)
    ;
}
