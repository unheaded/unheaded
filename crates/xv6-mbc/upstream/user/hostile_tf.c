// hostile_tf.c — EVOLUTION-1 Stage 2 (plan Step 135): a child scribbles over
// what would be slot 2's trapframe (table address + 2 * 0x100, as seen from its
// own address space), then forks; the grandchild takes slot 2, calls getpid,
// and checks a canary held in a register across the trap. Its pid must be
// correct and its registers intact.
//
// Run as the userland directly (this program is init).
// Observables (fixed classes):
//   "HT: child-pid=2|other"
//   "HT: gc-pid=3|other"           the grandchild's getpid
//   "HT: gc-canary=ok|bad"         s1 survives the getpid trap
//   "HT: done"
#include "kernel/types.h"
#include "kernel/stat.h"
#include "user/user.h"

#define BP_SLOT_TABLES ((volatile uint *)0x160)
#define TF_STRIDE      0x100
#define CANARY         0x0C0FFEE1u

int
main(void)
{
  int pid = fork();
  if(pid == 0){
    uint t = *BP_SLOT_TABLES;
    if(t)
      for(uint off = 0; off < TF_STRIDE; off += 4)
        *(volatile uint *)(t + 2 * TF_STRIDE + off) = 0xDEAD0000u | off;
    if(getpid() == 2) printf("HT: child-pid=2\n"); else printf("HT: child-pid=other\n");
    int g = fork();
    if(g == 0){
      register uint s1 asm("s1") = CANARY;
      asm volatile("" : "+r"(s1));
      int me = getpid();
      asm volatile("" : "+r"(s1));
      if(me == 3) printf("HT: gc-pid=3\n"); else printf("HT: gc-pid=other\n");
      if(s1 == CANARY) printf("HT: gc-canary=ok\n"); else printf("HT: gc-canary=bad\n");
      exit(0);
    }
    wait(0);
    exit(0);
  }
  wait(0);
  printf("HT: done\n");
  for(;;)
    ;
}
