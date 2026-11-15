// SPDX-License-Identifier: GPL-3.0-or-later
// hostile_reply.c: S-STAGE-2B Step 258, ADR-135 Security notes (R-F9). A child
// forges the kernel's reply word.
//
// Run as the userland directly, so this program is init. The table links below
// 8 MiB, so a store at the address the child reads from VA 0x160 lands in its own
// slice and tests nothing (the first check below, hostile_slot's shape). To reach
// the real table the child adds a PDE to its own pgd, which a store past 8 MiB can
// do while ADR-109 D6 is off, then forges through that alias three ways:
//   ecall   word stored ahead of a trap: upc_slot_enter must wipe it;
//   24      fresh_mask set by hand, then the fork note on a free slot;
//   sret    a U-mode sret reads the reply with no trap entry, so no wipe.
// The ecall route must hold. The other two are expected to forge while D6 is off
// and SRET is unchecked: the harness pins that, as it pins hostile_pgd, so a change
// shows up. KILL of init must still be refused on the sret route, and init prints
// the last line. With D6 on the PDE store is dropped, and the route ends at closed.
//
// S-STAGE-2C Phase 2 (ADR-109 D6 2026-11-15, ADR-135 R-F1): before the PDE route,
// two children each run one privileged return at U-mode with no alias at all, an
// mret and an sret. Refused means the opcode did nothing: the child is still at
// U-mode on the next instruction. An mret that is taken leaves for MEPC at MPP's
// privilege and never comes back; an sret that is taken lands on SEPC, the word
// after the child's last ecall, and comes through here a second time. With both
// refused, the sret route below cannot forge either: "sret-forged" then says no
// and the KILL forgery, which rides the same sret, is not tried.
//
// Observables (fixed classes; the MBC userland varargs ABI is unreliable):
//   "HR: own-slice-intact=yes|no"  store in the child's own slice changed nothing
//   "HR: table-reached=yes|no"     the alias holds the word; the own-slice copy doesn't
//   "HR: route=closed"             no alias: the forged routes were not tried
//   "HR: heap-intact=yes|no"       ecall route: break and heap bytes unchanged. This is
//                                  the line the wipe protects: with it removed, the
//                                  forged SET_BRK takes effect and this says no
//   "HR: note-refused=yes|no"      24 with the fresh bit clear answers -1
//   "HR: note-forged=yes|no"       24 with a forged fresh bit answers the slot's pid
//   "HR: mret-refused=yes|no"      a U-mode mret did nothing: own pid, own slice, same break
//   "HR: sret-refused=yes|no"      a U-mode sret did nothing, likewise
//   "HR: sret-forged=yes|no"       sret route: the forged break took effect
//   "HR: kill-init-refused=yes|no" sret route: forged KILL of slot 0 answered -1
//                                  (printed only when the sret route forged)
//   "HR: done"                     init, after the child is reaped
#include "kernel/types.h"
#include "kernel/stat.h"
#include "user/user.h"

#define SAY(t, c) do { if(c) printf(t "=yes\n"); else printf(t "=no\n"); } while(0)

#define BP_SLOT_TABLES  ((volatile uint *)0x160)
#define ST_FRESH_OFF    0x808
#define ST_REPLY_OFF    0x814
#define ST_ARG_OFF      0x818
#define TAG_REPLY       0x52500000u
#define REPLY_SET_BRK   1u
#define REPLY_KILL      4u
#define PGD_BASE        0x00F00000u
#define ALIAS           0x00800000u  // PDE 2: free in a child, which maps PDEs 0 and 1
#define ALIAS_PDE       2
#define ALIAS_ENTRY     0x840u       // PTE_PRESENT | PTE_LEAF: 4 MiB superpage at physical 0
#define CSR_SSTATUS     (0xF000u + 0x100 * 4)
#define CSR_SEPC        (0xF000u + 0x141 * 4)
#define SSTATUS_SPP     (1u << 8)
#define FORGED_BRK      0x00020000u  // inside the heap window, above the real break
#define HEAP_BYTES      4096
#define FREE_SLOT       2

static volatile uint *
phys(uint pa)
{
  return (volatile uint *)(ALIAS + pa);
}

// SYS_upc_forknote from user code, as in fdprobe.
static int
note(int a)
{
  register int a0 __asm__("a0") = a;
  __asm__ volatile("li a7, 24\n\tecall" : "+r"(a0) : : "memory");
  return a0;
}

// A U-mode sret to the next instruction, through the real SEPC. Returns a0 after it.
static int
sret_here(void)
{
  register int a0 __asm__("a0") = 0;
  *phys(CSR_SSTATUS) &= ~SSTATUS_SPP;
  __asm__ volatile("la t0, 1f\n\tsw t0, 0(%1)\n\tsret\n1:"
                   : "+r"(a0) : "r"(phys(CSR_SEPC)) : "t0", "memory");
  return a0;
}

// One privileged return at U-mode, in a child of its own. The last trap before it
// is getpid, so an sret that is taken resumes after that ecall, returns here and
// counts a second pass. Still at U-mode means: the next ecall answers this child's
// pid, a word stored at the table's address stays where the child put it (in S-mode
// the address is the real reply word, which trap entry wipes), and the break holds.
static void
ret_child(uint t, int is_mret)
{
  volatile int passes = 0;
  volatile uint *own = (volatile uint *)(t + ST_REPLY_OFF);
  uint brk = (uint)sbrk(0);
  int me = getpid();
  passes++;
  if(passes > 1){
    if(is_mret)
      printf("HR: mret-refused=no\n");
    else
      printf("HR: sret-refused=no\n");
    return;
  }
  if(is_mret)
    __asm__ volatile("mret");
  else
    __asm__ volatile("sret");
  own[1] = FORGED_BRK;
  own[0] = TAG_REPLY | REPLY_SET_BRK;
  int ok = getpid() == me && own[0] == (TAG_REPLY | REPLY_SET_BRK);
  own[0] = 0;
  ok = ok && (uint)sbrk(0) == brk;
  if(is_mret)
    SAY("HR: mret-refused", ok);
  else
    SAY("HR: sret-refused", ok);
}

static void
ret_route(uint t, int is_mret)
{
  if(fork() == 0){
    ret_child(t, is_mret);
    exit(0);
  }
  wait(0);
}

static int
heap_same(char *h)
{
  for(int i = 0; i < HEAP_BYTES; i++)
    if(h[i] != (char)(i * 7))
      return 0;
  return 1;
}

static void
child(uint t)
{
  char *h = sbrk(HEAP_BYTES);
  for(int i = 0; i < HEAP_BYTES; i++)
    h[i] = (char)(i * 7);
  uint brk = (uint)sbrk(0);

  volatile uint *own = (volatile uint *)(t + ST_REPLY_OFF);
  own[1] = FORGED_BRK;
  own[0] = TAG_REPLY | REPLY_SET_BRK;
  SAY("HR: own-slice-intact", (uint)sbrk(0) == brk && heap_same(h));
  own[0] = 0;

  *(volatile uint *)(PGD_BASE + (getpid() - 1) * 4096 + 4 * ALIAS_PDE) = ALIAS_ENTRY;
  volatile uint *reply = phys(t + ST_REPLY_OFF);
  volatile uint *arg = phys(t + ST_ARG_OFF);

  // ecall route. The trap is getpid because its handler sends no reply: sbrk's own
  // reply would overwrite the word, and the check would pass with no wipe at all.
  // Nothing may trap between the store and the getpid.
  *arg = FORGED_BRK;
  *reply = TAG_REPLY | REPLY_SET_BRK;
  int reached = *reply == (TAG_REPLY | REPLY_SET_BRK) && own[0] == 0;
  getpid();

  SAY("HR: table-reached", reached);
  if(!reached)
    printf("HR: route=closed\n");
  SAY("HR: heap-intact", (uint)sbrk(0) == brk && heap_same(h));
  if(!reached)
    return;

  // 24 route.
  SAY("HR: note-refused", note(FREE_SLOT) == -1);
  *phys(t + ST_FRESH_OFF) |= 1u << FREE_SLOT;
  SAY("HR: note-forged", note(FREE_SLOT) == FREE_SLOT + 1);

  // sret route.
  *arg = FORGED_BRK;
  *reply = TAG_REPLY | REPLY_SET_BRK;
  sret_here();
  int forged = (uint)sbrk(0) == FORGED_BRK;
  SAY("HR: sret-forged", forged);
  if(!forged)
    return;  // sret refused at U-mode: the KILL forgery has no route either

  *arg = 0;
  *reply = TAG_REPLY | REPLY_KILL;
  SAY("HR: kill-init-refused", sret_here() == -1);
}

int
main(void)
{
  uint t = *BP_SLOT_TABLES;
  if(t){
    ret_route(t, 1);
    ret_route(t, 0);
  }
  if(fork() == 0){
    if(t)
      child(t);
    exit(0);
  }
  wait(0);
  printf("HR: done\n");
  for(;;)
    ;
}
