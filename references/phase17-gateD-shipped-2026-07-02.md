# Phase 1.7 Gate D — FS Reader SHIPPED — session log 2026-07-02

**Session**: Fable 5, attended churn (continuation of the 2026-06-28→30 Gate D sessions)
**Battle plan**: `docs/battle-plans/PHASE17-GATE-D-FS-READER.md` (phases 0–7)
**Result**: **GATE D COMPLETE.** `ls` lists the real root dir, `cat README` streams the
283-byte fixture (proof token `0xGA7ED-D15C-READER`), `echo hello` prints `hello`,
fresh `$ ` prompt after each. Zero regression to Gates A/B/C.

## Commits (main, unsigned)

- `8c481a3d` fix(upc): fstat writes the ilp32e 16-byte struct stat — ls lists the real
  root dir (Gate D Phase 5)
- `e47bf2c5` feat(upc): argv-on-stack in exec(7) + multi-byte write(16) + 64-byte KBD
  ring (Gate D Phase 6)

(Phases 0–4 + wip-5 were `f621c662`, `cc1b526c`, `29bc6f14`, `bfeb620d`, `99a37989`,
`eb380e08` from the prior sessions.)

## Phase 5 root cause — ONE bug, four symptoms

The inherited state: `ls` ran its full lifecycle but printed
`$⟨15 spaces⟩1 1 0` then 8× `ls: cannot stat ⟨nothing⟩`, and the handoff framed it as
"fstat and read disagree about the same dinode" (fstat size=0, read delivered 9 dirents).

**The dinode arithmetic was never wrong.** xv6 `kernel/types.h` does
`typedef unsigned long uint64;` — under `-mabi=ilp32e` that is **4 bytes** — so the
guest's `struct stat` is **16 bytes with `size` at offset 12**, not the RV64 24-byte /
offset-16 layout the fstat(8) handler wrote. Two consequences:

1. The guest read `st.size` from offset 12, where the handler had written its
   alignment pad → **size 0** (the handler had computed 144/1024 correctly and put it
   where nobody looks).
2. The handler's bytes 16–23 landed **past the real struct**, zeroing the next two
   words of the caller's frame — which in `ls()` is `buf[0..7]`. Every `stat()` call
   re-zeroed `buf[0..3]`:
   - iteration 1: `buf="./."` resolves fine (ino=1, type=1) but the fstat inside
     `stat()` zeroes `buf` *before* `printf(fmtname(buf),…)` → `fmtname("")` = 14
     spaces → the missing dot;
   - iterations 2–9: `strcpy(buf,path)` is outside the loop, so `buf[0]` stays 0;
     `memmove` only writes `buf+2` → `stat("")` → open scans for an empty basename →
     -1 → `cannot stat` with an empty `%s`. Exactly 8 of them.

Every observed byte (the 15 spaces, the `1 1 0`, the empty `%s`, the 1-success/8-fail
pattern) falls out of this one layout bug with zero free parameters — which is why the
fix was applied without instrumentation and worked on the first boot.

**Falsified along the way** (for the record): pid2-slice/ramdisk aliasing (slices start
at 16 MiB, stride 8 MiB — no overlap), read(5) copy source/dest arithmetic, user
load/store visibility, and the open-cursor staleness (real hazard, see below, but not
this bug).

Fix shape: fstat now writes 4 words (`dev`, `ino`, `type|nlink<<16`, `size`) through
`user_phys(pid, st_va, 16)` — two writes FEWER than before (verifier budget improved).

**Hardening bonus**: open(15) now clears its a3 `OPEN_MAGIC` cookie on both scan
completions (found / exhausted). Before, a stale magic + whatever the user last left in
a2 (`read`'s n=16, `memmove`'s 14…) could seed the next open's cursor and silently skip
dirents. It never bit in the observed runs — compiled register usage got lucky — but it
was a landmine.

## Phase 6 — argv-on-stack + multi-byte write + 64-byte KBD ring

- **KBD ring 8→64** (Step 40): `KBD_MAP` max_entries, drain mask `&7`→`&63`,
  `write_kbd` cap + CLI docs. `cat README\n` (11 B) now fits `--input`.
- **exec(7) PHASE A — argv harvest** (Step 42): re-entrant, ONE arg per tick (open's
  proven 1-item/tick shape). Per tick: read `argv[i]`, fixed 16-byte string copy
  (read(5)'s proven no-guard shape) into an 8×16 B slot region at **VA 0x0060_0000**,
  pointer word into the NULL-terminated array at **VA 0x0060_0080**; then hand off to
  the existing PHASE B .data copy with **argc stashed in a2's high 16 bits** (the .data
  cursor needs <16 bits). Completion sets `a0=argc`, `a1=0x600080`, `sp=0x500000`
  (unchanged). `crt0_mbc.S` `_start: call main` forwards a0/a1 untouched (Step 43
  verified by inspection).
  - **Placement rationale**: the frame must NOT go just below the stack top —
    `init.c`'s `char *argv[] = {"sh", 0}` is a stack local living exactly there, and
    phase A must not clobber what it is still reading. [0x500000, 0x800000) is virgin
    VA between stack top and the 8 MiB `USER_VA_CEILING`.
  - Hardening: argv table / string pointers H-bounded to the pid slice (treated as
    terminators when out of range); re-entry cursor clamped (hostile a2 can't fabricate
    unbounded argc); slot tail force-NUL'd (a ≥15-byte arg truncates, never
    unterminated).
- **write(16) honors n**: xv6 `echo` never checks the short-write return (printed ONE
  byte of "hello"); `cat` treats a short write as fatal. Now: n==1 keeps the old
  single-tick hot path (printf's putc); n>1 re-enters one byte per tick with the cursor
  in a3's low 16 bits under a `0xD0C0` tag (a2 holds n, so the bare-a3 magic trick
  doesn't fit); n clamped to 4096. Side effect: sh's `write(2, "$ ", 2)` prompt now
  prints both bytes — the trailing space was being truncated since Gate B.

## Acceptance (Phase 7)

| Gate | Command | Result |
|------|---------|--------|
| E1 | `--input $'ls\n'` | `. .. init sh ls cat echo wc README` with types+inums+real sizes (root=1024: mkfs rounds dir size to a block; ls skips the 55 free dirents) ✅ |
| E2 | `--input $'cat README\n'` | full 283-byte fixture, exact ASCII incl. `0xGA7ED-D15C-READER` ✅ |
| E3 | `--input $'echo hello\n'` | `hello` ✅ |
| E4 | regressions | Gate C `echo\n` stats identical (`forks=2 tty_r=5 waitpid=1 halt=0x0`, ends `$$`); gate2 `ISOLATION-PASS`; gate_nway `NWAY-FAIL pid2` pre-existing; monad-common 85+1 tests pass; both eBPF configs compile, ascend rebuilt LAST; XDP attach passes (under the 1M ceiling) ✅ |

## Known issue → Gate D.1

**`wc README` spins.** wc's child (pid 2) runs away in user code: PC≈0x997, SP descends
~270 KB below 0x500000 (deep recursion or runaway loop), kernel RET-anomaly tracer
emits `<2585>`, sh never reaps (`waitpid=0`). NOT a regression — wc was unreachable
before argv existed (argc was always 0 → wc read stdin → blocked). wc is not in the
Gate D acceptance set. First lead for D.1: wc is the only corpus program whose printf
prints THREE `%d`s from accumulated counters — but ls prints `%d` sizes fine; second
lead: wc's `while((n = read(fd, buf, 512)) > 0)` short-read loop interleaving with the
new re-entrant write cursor (both park state in a3 across ecall re-entries — check for
a collision when read's console-block path and write's cursor path alternate).

## Verifier-budget ledger (what fit)

- fstat: −2 word writes (16-byte struct).
- open: +2 register clears (cookie retire).
- exec: +PHASE A (1 ptr read + 15 byte copies + 2 word writes + clamps per tick).
- write: +cursor decode branch on the n>1 path only.
- Net: XDP attach still passes on kernel 6.17. The 16-byte no-guard copy and
  1-item/tick re-entry shapes remain the proven idioms.

Peace and love. 🌀🐕
