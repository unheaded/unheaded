// getpidprobe.c — EVOLUTION-1 Stage 1 (S1.2) getpid proof-of-life probe.
//
// Runs as the userland directly (like gate2.c — it is pid 0/init and exits,
// so the kernel restarts it; expect the lines below once per reboot):
//   upc-bootctl boot ... --userland target/getpidprobe.mbc
//
// Purpose: exercise exactly one syscall — getpid (nr 11) — and report the
// OBSERVED pid. Under the S1.1 guard flag (GETPID_INJECT=true), getpid is
// serviced by the OWNED kernel via a real injected trap
// (uservec -> usertrap -> syscall() -> sys_getpid -> usertrapret -> SRET).
//
// The observables (fixed strings only — the MBC userland varargs ABI is not
// reliable, so the printed token IS the value, gate2-style):
//   * "GP: probe-start"     — the probe reached main and printed once.
//   * "GP: getpid-returned" — control returned from getpid(). Printing THIS
//     line is the headline result: the injected entry-half round-tripped
//     (save regs -> kernel -> restore -> SRET back to user) without halting.
//   * "GP: pid=N" (N in {0,1,2,other}) — the observed pid. A value matching
//     the running process proves kernel/BPF process state is in sync for this
//     path; a stale value (e.g. init's) PROVES the dual-books diagnosis.
//     Either is informative — we assert what is OBSERVED, not what is assumed.
//
// With the flag OFF (default), getpid is serviced in BPF and the same three
// lines print — so this probe is also a control: identical output either way
// except for the value only the kernel path can reveal about dual-books.

#include "kernel/types.h"
#include "kernel/stat.h"
#include "user/user.h"

int
main(void)
{
  printf("GP: probe-start\n");
  int pid = getpid();
  printf("GP: getpid-returned\n");
  if(pid == 0)
    printf("GP: pid=0\n");
  else if(pid == 1)
    printf("GP: pid=1\n");
  else if(pid == 2)
    printf("GP: pid=2\n");
  else
    printf("GP: pid=other\n");
  exit(0);
}
