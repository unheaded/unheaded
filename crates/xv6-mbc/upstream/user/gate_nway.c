// gate_nway.c — ASCEND-LINUX Phase 1.2 "Gate 4": N-way CONCURRENT isolation
// (ADR-074 scoped 3-way, 2026-05-31). The adversarial successor to gate2.c.
//
// PROPERTY UNDER TEST (stronger than Gate 2's sequential proof): per-pid pgds
// give DISJOINT physical slices to processes that are LIVE AT THE SAME TIME, so
// a real shell with pipes / background jobs cannot leak between co-resident
// processes. Gate 2 only ever had one non-init process live at a time.
//
// WHY THIS TEST CAN FAIL (BlackMage's concurrence condition — it is a
// leak-FALSIFICATION test, not a confirmation): each pid's VA[0,8 MiB) is mapped
// by two 4 MiB superpage leaves onto phys [base, base+8 MiB), base =
// USER_CHILD_SLICE + (pid-1)*SLICE_STRIDE. If SLICE_STRIDE < 8 MiB, pid N's
// upper leaf OVERLAPS pid N+1's lower leaf (leak primitive #1). We probe both
// the low band [δ] and the high band [HIGH_BASE+δ] with the SAME offset δ, where
// HIGH_BASE = 6 MiB = the falsification stride: under a 6 MiB stride, pid N's
// high probe VA(HIGH_BASE+δ) → phys(base_N + 6 MiB + δ) == phys(base_{N+1} + δ)
// == pid N+1's low probe — the SAME physical word. Two siblings write different
// canaries there; whoever reads last sees a FOREIGN canary → NWAY-FAIL. Under
// the shipped 8 MiB stride the bands are 2 MiB apart → disjoint → NWAY-ok.
//
// LIVENESS: ascend-linux has no timer preemption, so children co-schedule by
// cooperatively yielding via pause() (mapped to scheduler_context_switch in the
// BPF RV32I dispatch). Each child WRITEs its canaries, yields once (by which
// time every sibling has also written — see the round-robin trace below), then
// VERIFIEs — so every clobber has happened before any verify.
//   parent → c1.write,yield → c2.write,yield → c3.write,yield → (parent re-wait)
//          → c1.verify,exit → c2.verify,exit → c3.verify,exit → parent.reap×3
// A second pause() is added for margin (extra yields are harmless: memory is
// stable once all canaries are written).
//
// FULLY UNROLLED — no `for` loop anywhere. A C `for`-loop wrapped around fork()
// triggers a substrate bug where forked children re-enter the loop and re-fork
// (a runaway fork-bomb: num_proc climbs to the table cap, every fork bails
// -1). Unrolled fork/yield/verify sidesteps it; the isolation property is
// independent of how the parent spawns its children. (See the session note —
// the looped-fork translation bug is a separate Phase-3 substrate finding.)
//
// VERDICT is per-child (the xv6 wait() status path is stubbed, so the parent
// cannot read child exit codes): each child prints exactly one authoritative
// line. PASS for the whole gate ⇔ NKIDS× "NWAY-ok pidN" present AND zero
// "NWAY-FAIL". Fixed-string printf only (the %-format varargs ABI is unreliable
// on the MBC userland — see gate2.c).
//
//   upc-bootctl boot ... --userland target/gate_nway.mbc
//
// gate_nway runs as init (pid 0) and exits, so the kernel restarts it — expect
// the verdict block to repeat once per reboot.

#include "kernel/types.h"
#include "kernel/stat.h"
#include "user/user.h"

// Probe geometry (must mirror phase12.rs SLICE_STRIDE / superpage layout).
#define HIGH_BASE 0x00600000u    // 6 MiB — equals the falsification stride
#define D0        0x00100000u    // 1.0 MiB  (low band: free, above heap/below stack)
#define D1        0x00180000u    // 1.5 MiB
// Low probes live at D0/D1; high probes at HIGH_BASE+D0 (7 MiB) / HIGH_BASE+D1
// (7.5 MiB) — both above the ~5 MiB stack top, below the 8 MiB mapping ceiling.

static void
put32(unsigned va, unsigned val)
{
  *(volatile unsigned *)va = val;
}

static unsigned
get32(unsigned va)
{
  return *(volatile unsigned *)va;
}

// One child. expect_pid ∈ {1,2,3} (= idx+1); me = this child's unique canary.
// Never returns (always exit()s). No loops → no looped-fork hazard.
static void
child(int label, unsigned me)
{
  // The isolation property is pid-agnostic: "I must read back MY OWN unique
  // canary, never a co-resident sibling's." `label` is cosmetic (the Nth
  // child); it does NOT have to equal getpid() — under reboot churn the
  // lowest-free-pid allocator may hand the Nth child a recycled pid, so an
  // identity assertion would flake without bearing on isolation. (getpid()
  // exists and is correct; it is simply not the property under test here.)
  int ok = 1;

  // Stamp our canary across the low and high overlap bands.
  put32(D0, me);
  put32(D1, me);
  put32(HIGH_BASE + D0, me);
  put32(HIGH_BASE + D1, me);

  // Yield so every sibling completes its writes before we verify. One yield is
  // sufficient given the round-robin; a second is margin.
  pause(0);
  pause(0);

  // Verify — any foreign canary means a co-resident sibling's slice overlapped
  // ours (or our own slice was clobbered): isolation FAILED.
  if(get32(D0) != me) ok = 0;
  if(get32(D1) != me) ok = 0;
  if(get32(HIGH_BASE + D0) != me) ok = 0;
  if(get32(HIGH_BASE + D1) != me) ok = 0;

  if(ok){
    if(label == 1)      printf("NWAY-ok pid1\n");
    else if(label == 2) printf("NWAY-ok pid2\n");
    else                printf("NWAY-ok pid3\n");
    exit(0);
  } else {
    if(label == 1)      printf("NWAY-FAIL pid1\n");
    else if(label == 2) printf("NWAY-FAIL pid2\n");
    else                printf("NWAY-FAIL pid3\n");
    exit(1);
  }
}

int
main(void)
{
  // Fork three children WITHOUT waiting between forks, so all are live at once
  // (the lowest-free-pid allocator hands out pids 1,2,3 in order). Unrolled.
  int p1 = fork();
  if(p1 == 0) child(1, 0xA1A1A1A1u);   // pid 1 — never returns
  if(p1 < 0)  printf("NWAY-FORK-FAIL\n");

  int p2 = fork();
  if(p2 == 0) child(2, 0xB2B2B2B2u);   // pid 2
  if(p2 < 0)  printf("NWAY-FORK-FAIL\n");

  int p3 = fork();
  if(p3 == 0) child(3, 0xC3C3C3C3u);   // pid 3
  if(p3 < 0)  printf("NWAY-FORK-FAIL\n");

  // Reap all. The kids are authoritative on isolation; this line is structural:
  // it asserts the parent forked and reaped the whole concurrent set.
  wait(0);
  wait(0);
  wait(0);

  printf("NWAY-PARENT-REAPED-ALL\n");
  exit(0);
}
