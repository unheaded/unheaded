// hostile_slot.c — EVOLUTION-1 Stage 2 (plan Step 134): a child forges the
// ADR-109 handoff words. It reads the slot-table address at VA 0x160 in its own
// address space, writes a cur_slot-shaped value naming slot 5 and a
// trap_a7-shaped value naming getpid at that table address as it sees it, then
// calls getpid. The kernel must still answer with the child's own pid, and
// init's getpid afterwards must still be 1.
//
// Run as the userland directly (this program is init).
// Observables (fixed classes; the MBC userland varargs ABI is unreliable):
//   "HS: child-tbase=zero|same|other"  what the child reads at VA 0x160,
//                                      against init's value
//   "HS: child-pid=2|other"            getpid after the forged words
//   "HS: init-pid=1|other"             init's getpid after wait()
//   "HS: done"
#include "kernel/types.h"
#include "kernel/stat.h"
#include "user/user.h"

#define BP_SLOT_TABLES  ((volatile uint *)0x160)
#define ST_CUR_SLOT_OFF 0x800
#define ST_TRAP_A7_OFF  0x804
#define TAG_SLOT        0x51070000u
#define TAG_A7          0xA7000000u

int
main(void)
{
  uint init_tbase = *BP_SLOT_TABLES;
  int pid = fork();
  if(pid == 0){
    uint t = *BP_SLOT_TABLES;
    if(t == 0) printf("HS: child-tbase=zero\n");
    else if(t == init_tbase) printf("HS: child-tbase=same\n");
    else printf("HS: child-tbase=other\n");
    if(t){
      *(volatile uint *)(t + ST_CUR_SLOT_OFF) = TAG_SLOT | 5;
      *(volatile uint *)(t + ST_TRAP_A7_OFF) = TAG_A7 | 11;
    }
    if(getpid() == 2) printf("HS: child-pid=2\n"); else printf("HS: child-pid=other\n");
    exit(0);
  }
  wait(0);
  if(getpid() == 1) printf("HS: init-pid=1\n"); else printf("HS: init-pid=other\n");
  printf("HS: done\n");
  for(;;)
    ;
}
