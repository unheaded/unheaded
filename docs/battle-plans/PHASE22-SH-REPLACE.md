# PHASE 2.2 FINALE — SH REPLACE BATTLE PLAN — 10 Phases, 110 Steps

**Date**: 2026-07-05
**Sprint**: Track 2 Phase 2.2 finale — replace the last MIT program in the exec set
(`sh`) with our own `user/ush.c` (ADR-081, "evolve from xv6, never break the boot")
**Prerequisite**: Phase 2.2 coreutils complete (`ac8e3801`); tree clean on `main`;
`scripts/upc-regression.sh` == ALL GREEN
**Target**: Every harness command runs through OUR sh with byte-identical TTY
output; exec set 100% Unheaded-authored; zero eBPF change (ascend verifier stays
EXACTLY 900,031 / 90.0%, Doom 737,087); Phase 2.2 COMPLETE
**Estimated Duration**: 3-5 hours, single session
**Agent Strategy**: Coordinator solo (sudo boots + iterative debugging; phases are
sequential — the boot is the shared resource, nothing parallelizes)
**Commit Cadence**: 3 atomic GREEN commits (gates → swap → docs). The generic
3-5-step formula is overridden by the Track 2 doctrine "staying green every
commit": a mid-swap commit (Makefile without ush.c, or vice versa) breaks the boot.
Never commit red.
**Stuck Protocol**: Skip after 3x time estimate or 2 failed debug attempts; commit
last green state first; STUCK marker + report

---

## VARIABLES

```bash
ROOT="$(git rev-parse --show-toplevel)"        # the unheaded checkout
EBPF="$ROOT/ebpf"                              # bootctl MUST run from here (CWD-relative object)
BC="$ROOT/cmd/upc-bootctl/target/release/upc-bootctl"
UP="$ROOT/crates/xv6-mbc/upstream"             # vendored xv6 + our u*.c
AD="$ROOT/crates/xv6-mbc/adapters"             # Makefile.mbc-userland etc.
K="$UP/target"                                 # kernel/userland artifacts
GOLD="${GOLD:-/tmp/phase22-golden}"            # golden TTY captures (session-temp)
boot() { (cd "$EBPF" && sudo "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" \
  --userland "$K/init.mbc" --triggers 4000000 --instance 111 --input "$1" 2>&1); }
tty_of() { grep -a 'ascii:' <<<"$1"; }
stats_of() { grep -aoE 'forks=[0-9]+|waitpid=[0-9]+|tty_r=[0-9]+' <<<"$1" | tr '\n' ' '; }
```

## LEGEND

`[B]` bash · `[V]` verify (must pass) · `[D]` debug (only on fail) · `[W]` write
file · `[R]` read · `[S]` sudo · `[CODE]` implementation · `[TEST]` test run ·
`[C]` commit checkpoint · `[DECIDE]` pre-seeded decision (proceed autonomously) ·
`[SEC]` security review · `[GATE]` phase exit gate · `[DOC-UPDATE]` doc ripple

---

## SITUATION

**Inherited**: Gate C (2026-06-26) proved the interactive sh lifecycle: prompt →
KBD-ring line → fork → exec → child exit → parent-filtered wait → fresh `$ `.
Gate D (2026-07-02) gave sh a real FS to exec against and fixed the argv-on-stack
harvest (frame at VA 0x600000), multi-byte write, ilp32e `struct stat`, and the
RET-floor (0x20000). Phase 2.1/2.2 (2026-07-04/05) replaced init + all four
coreutils via the explicit-Makefile-rule mechanism. **sh is the last MIT program
in the exec set.**

**Architecture (verified against code, not assumed)**:
- The ONLY consumer of sh is `scripts/upc-regression.sh` (helper `xv6_expect`,
  lines 46-52). Corpus inputs: `ls`, `cat README`, `cat BIGFILE`, `cat sub/NOTE`,
  `ls sub`, `echo hello`, `echo peace and love`, `wc README`, plus the uinit boot
  (`echo hello`). **Every line is `cmd arg…\n` — the corpus is EXEC-only.**
- Dead syscalls: pipe(4), chdir(9), sbrk(12) have no arm in the ascend ecall
  dispatch (`ebpf/monad-cpu-ebpf/src/main.rs:1990`+; unknown → silent no-op
  fall-through at :3191). PIPE/REDIR/LIST/BACK clauses and the `cd` builtin are
  unimplementable dead code.
- **The malloc landmine**: stock sh.c mallocs every parsed cmd node
  (constructors, sh.c:201-264) → umalloc `morecore` → `sbrk(12)` → silent no-op
  returning garbage a0 → umalloc treats it as a heap pointer. Stock sh works
  today **by accident** (the garbage lands in unused low VA). Our replacement
  must be malloc-free like every other u*.c.
- exec(7) argv-harvest ABI (main.rs:3056-3106): **ARGV_MAX=8** args,
  **ARG_CAP=16** (15 chars + NUL, silent truncation), strings copied to VA
  0x600000 / pointer array 0x600080, a0=argc a1=0x600080, SP top 0x500000.
  KBD ring caps input at **64 bytes** including `\n` (runner.rs:466 bails).
- write(16) is short-write-safe (re-entrant 0xD0C0 cursor, always returns full
  n) — no userland handling needed.
- `sh` is PROGRAM_TABLE slot 0 (`["sh","echo","ls","cat","wc"]`, upc-bootctl
  main.rs:818); staging keys on FNV-1a of the basename, so replacing the source
  behind `target/sh.mbc` needs **zero bootctl/BPF changes**. Boot-log `hash`
  stays constant by design; replacement proof is the ELF FILE symbol.
- `ls` gates grep only `README` / `NOTE           2` — sh's binary size change
  inside fs.img is harmless.

**Decision — Design A (EXEC-only ush) vs Design B (full parser port)**:
**A wins.** The corpus exercises only EXEC; pipe/chdir/sbrk are dead syscalls, so
REDIR/PIPE/LIST/BACK and `cd` cannot function regardless of parser support.
Porting them (Design B) would ship ~350 lines of dead, untestable, malloc-dependent
code. Design A is ~110 lines, malloc-free, and 100% corpus-covered. Rejected
alternative B is revisitable when pipe(4)/chdir(9)/sbrk(12) handlers exist
(Phase 2.3+ / Epic 1.3.2 territory).

**Four-lens panel (Architect / BlackMage / Developer / Scientist, per the Epic
1.2 precedent)**:
- **Architect**: blast radius = one .c file + one Makefile rule; boot path,
  bootctl, BPF untouched. Sequencing: behavior-lock gates land BEFORE the swap so
  the swap commit is provably identical-observable. Approved.
- **BlackMage**: sh is the largest input-facing surface in the guest. Demands:
  hard argc cap (8), explicit metachar rejection (no silent reinterpretation),
  in-place tokenization only within the 100-byte static buffer already bounded by
  the 64-byte ring, no malloc, no new syscalls. Panic paths must exit the CHILD
  only (shell survives hostile lines). Approved with §PHASE 7 checklist.
- **Developer**: TDD order — gates first (red-bar impossible: they must be GREEN
  against stock sh, proving they lock behavior, not implementation), then swap,
  then the same gates prove equivalence. Stock-parity list: `$ ` prompt via
  `write(2,…,2)`, getcmd shape (memset + gets), blank-line skip IN PARENT
  (stock sh.c:160-165 skips too — no fork on blank), `exec %s failed\n` then
  child exit(0) (stock runcmd tail), panic messages `syntax` / `too many args` /
  `fork` via `fprintf(2,"%s\n")` + exit(1). Approved.
- **Scientist**: hypotheses H1-H4 below must be verified before any change;
  golden captures make "identical observable behavior" falsifiable byte-by-byte.
  Predicted divergences (all invisible to the corpus, all documented): `cd x` →
  `exec cd failed` (stock: silent bogus chdir "success"); metachar lines →
  clean `syntax` (stock: undefined dead-pipe behavior); args 9+ → `too many
  args` at 8 not 10 (harvester drops past 8 anyway). Approved.

## PREFLIGHT HYPOTHESES

| # | Hypothesis | Verify via | If false |
|---|-----------|-----------|----------|
| H1 | Stock `target/sh.elf` links from MIT `sh.c` (FILE symbol) | Step 8 | Investigate — tree not in expected state; STOP |
| H2 | Baseline budgets: ascend 900,031 (90.0%), Doom 737,087 | Steps 3-5 | STOP — baseline drifted; diagnose before any change |
| H3 | Multi-line `--input` (`$'echo alpha\necho omega\n'`) runs BOTH commands through stock sh (KBD ring holds 2 lines, prompt loop survives) | Step 12 | [DECIDE] drop gate N2 (multi-command); keep N1+N3 |
| H4 | `nosuch\n` through stock sh → `exec nosuch failed` on TTY | Step 13 | [DECIDE] adjust N1's expected token to observed stock behavior |
| H5 | Blank-then-command `$'\necho unbroken\n'` → `unbroken` (blank skipped, loop survives) | Step 14 | [DECIDE] drop gate N3 |

## KNOWN FAILURES BASELINE

Recorded at Step 2. Expected: **none** (harness == ALL GREEN, 12/12). gate_nway
NWAY-FAIL is pre-existing and OUTSIDE the harness — not tracked here.

---

## PHASE 0: PREFLIGHT — GREEN BASELINE + GOLDEN CAPTURES (Steps 1-16)

**Goal**: Prove the starting state is green, verify all hypotheses, and capture
byte-exact golden TTY output for every sh-exercising corpus line against STOCK sh.
**Prerequisite**: none (this IS the starting gate)
**Time**: ~30m (dominated by the full harness run + 8 golden boots)
**Agent**: Coordinator

- [ ] **Step 1** [B] ~1m: Tree clean, on main, variables resolve
  ```bash
  cd "$(git rev-parse --show-toplevel)" && git status --short && git branch --show-current && mkdir -p "$GOLD"
  ```
- [ ] **Step 2** [B][S][TEST] ~5m: Full baseline harness
  ```bash
  bash scripts/upc-regression.sh 2>&1 | tail -25
  ```
- [ ] **Step 3** [V]: **BASELINE GATE** — output ends `== ALL GREEN ==`
  - If fail → STOP per the handoff ("if not green, stop and diagnose"). Do not
    proceed; this plan assumes a green base.
- [ ] **Step 4** [V]: Harness `note` lines show `ascend verifier: verified 900031
  insns (90.0% …)` (H2a)
- [ ] **Step 5** [V]: Harness shows `Doom verifier: verified 737087 insns` and
  `Doom object loads` PASS (H2b)
  - If 4/5 drifted → STOP; budgets moved outside this plan's control.
- [ ] **Step 6** [B][S] ~10m: Golden captures — all 8 stock-init corpus lines
  ```bash
  for spec in "ls" "cat README" "cat BIGFILE" "cat sub/NOTE" "ls sub" "echo hello" "echo peace and love" "wc README"; do
    out="$(boot "$spec"$'\n')"; slug="$(tr ' /' '__' <<<"$spec")"
    printf '%s\n' "$out" > "$GOLD/$slug.full"
    tty_of "$out" > "$GOLD/$slug.tty"; stats_of "$out" > "$GOLD/$slug.stats"
  done; ls -la "$GOLD"
  ```
- [ ] **Step 7** [V]: All 8 `.tty` files non-empty and contain their corpus token
  (`grep -l 0xGA7ED-D15C-READER "$GOLD"/cat_README.tty` etc.)
  - If any empty → [D] re-run that boot once; twice-empty → STOP (boot flaking,
    fix environment first).
- [ ] **Step 8** [B] ~1m: H1 — stock sh links from MIT sh.c
  ```bash
  riscv64-unknown-elf-objdump -t "$K/sh.elf" | grep 'df \*ABS\*'
  ```
- [ ] **Step 9** [V]: FILE symbol shows `sh.c` (NOT ush.c)
- [ ] **Step 10** [B][S] ~2m: Golden capture — uinit boot (`echo hello` via OUR PID 1)
  ```bash
  out="$(cd "$EBPF" && sudo "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" \
    --userland "$K/uinit.mbc" --triggers 4000000 --instance 111 --input $'echo hello\n' 2>&1)"
  printf '%s\n' "$out" > "$GOLD/uinit_echo.full"; tty_of "$out" > "$GOLD/uinit_echo.tty"; stats_of "$out" > "$GOLD/uinit_echo.stats"
  grep -ao '0xP1D1-0UR5' "$GOLD/uinit_echo.tty"
  ```
- [ ] **Step 11** [V]: uinit golden contains banner token + `hello`
- [ ] **Step 12** [B][S] ~2m: H3 — multi-line input against stock sh
  ```bash
  out="$(boot $'echo alpha\necho omega\n')"; tty_of "$out"
  printf '%s\n' "$out" > "$GOLD/multi.full"; tty_of "$out" > "$GOLD/multi.tty"; stats_of "$out" > "$GOLD/multi.stats"
  ```
  - [V] TTY contains BOTH `alpha` and `omega`. If only `alpha` → H3 false →
    [DECIDE] drop gate N2, note in plan, continue.
- [ ] **Step 13** [B][S] ~2m: H4 — unknown command against stock sh
  ```bash
  out="$(boot $'nosuch\n')"; tty_of "$out" | tee "$GOLD/nosuch.tty"; printf '%s\n' "$out" > "$GOLD/nosuch.full"
  ```
  - [V] TTY contains `exec nosuch failed`. If different text → H4 false →
    [DECIDE] lock gate N1 to the OBSERVED stock text instead.
- [ ] **Step 14** [B][S] ~2m: H5 — blank line then command against stock sh
  ```bash
  out="$(boot $'\necho unbroken\n')"; tty_of "$out" | tee "$GOLD/blank.tty"; printf '%s\n' "$out" > "$GOLD/blank.full"
  ```
  - [V] TTY contains `unbroken`. If not → H5 false → [DECIDE] drop gate N3.
- [ ] **Step 15** [B] ~30s: Snapshot goldens read-only
  ```bash
  chmod -w "$GOLD"/* && ls "$GOLD" | wc -l
  ```
- [ ] **Step 16** [V][GATE]: **PHASE 0 EXIT GATE** — ALL GREEN baseline, H1-H2
  confirmed, 8 corpus + uinit goldens captured, H3/H4/H5 resolved (true or
  DECIDEd). **Definition of Done**: goldens immutable on disk; zero repo changes
  yet (`git status --short` empty).

## PHASE 1: BEHAVIOR LOCK — NEW PERMANENT GATES vs STOCK SH (Steps 17-28)

**Goal**: Land the new sh gates in `scripts/upc-regression.sh` and prove them
GREEN against STOCK sh — they lock behavior, not implementation. Commit 1.
**Prerequisite**: Phase 0 gate
**Time**: ~25m
**Agent**: Coordinator

- [ ] **Step 17** [DECIDE]: Gate set (pre-seeded; prune only per H3/H4/H5 results):
  - **N1** `xv6_expect 'nosuch' 'exec nosuch failed' 'sh reports exec failure (unknown command)'`
  - **N2** custom block (like the uinit block): input `$'echo alpha\necho omega\n'`,
    TWO asserts — `alpha` (first command) and `omega` (prompt loop survives to a
    second command)
  - **N3** `xv6_expect $'\necho unbroken' 'unbroken' 'sh skips blank line, loop survives'`
    (xv6_expect appends the final `\n`)
- [ ] **Step 18** [W] ~5m: Edit `scripts/upc-regression.sh` — insert N1-N3 after
  the `wc README` gate (line 75), before the gate2 block, with a header comment:
  `# Phase 2.2 finale (ADR-081): sh behavior lock — landed GREEN against stock`
  `# sh BEFORE the ush swap, so they gate behavior, not implementation.`
- [ ] **Step 19** [B] ~30s: Lint the edit
  ```bash
  bash -n scripts/upc-regression.sh && git diff --stat
  ```
- [ ] **Step 20** [V]: Syntax OK; diff touches ONLY upc-regression.sh
- [ ] **Step 21** [B][S][TEST] ~6m: Full harness with new gates, STOCK sh still wired
  ```bash
  bash scripts/upc-regression.sh 2>&1 | tail -30
  ```
- [ ] **Step 22** [V]: `== ALL GREEN ==` with 15 PASS lines (12 old + 3 new; 14 if
  a gate was DECIDEd out)
  - If a NEW gate fails → [D] Step 23. If an OLD gate fails → STOP, `git checkout
    scripts/upc-regression.sh`, re-run, diagnose.
- [ ] **Step 23** [D] ~5m: New-gate failure — compare its boot output to the
  Phase 0 probe (`$GOLD/nosuch.full` / `multi.full` / `blank.full`); if stock
  behavior differs from probe (flaky), re-run once; if stable-different, fix the
  gate's expected token to match stock (behavior lock = whatever stock does).
  Max 2 attempts then Skip Protocol (drop the gate, note it).
- [ ] **Step 24** [B] ~1m: Stage exactly one file
  ```bash
  git add scripts/upc-regression.sh && git status --short
  ```
- [ ] **Step 25** [W] ~2m: Commit message to a temp file (backticks — use -F):
  subject `test(upc): Track 2 Phase 2.2 — sh behavior-lock gates before the swap`,
  body: gates N1-N3 landed GREEN against STOCK sh; they lock the fork/exec/wait
  loop's observable behavior (exec-failure text, prompt-loop survival across
  multiple commands, blank-line skip) so the ush swap commit can prove identical
  behavior; trailer `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`
- [ ] **Step 26** [C][B] ~30s: **COMMIT 1**
  ```bash
  git commit --no-gpg-sign -F /tmp/phase22-commit1.txt && git log --oneline -1
  ```
- [ ] **Step 27** [V]: Commit landed; tree clean
- [ ] **Step 28** [V][GATE]: **PHASE 1 EXIT GATE** — new gates in main, harness
  ALL GREEN against stock sh. **DoD**: behavior locked before any implementation
  change; only file touched is the harness.

## PHASE 2: WRITE ush.c (Steps 29-36)

**Goal**: Write `crates/xv6-mbc/upstream/user/ush.c` — EXEC-only, malloc-free,
stock-parity where reachable.
**Prerequisite**: Phase 1 gate
**Time**: ~30m
**Agent**: Coordinator

- [ ] **Step 29** [R] ~3m: Re-read parity sources: stock `sh.c` main/getcmd/runcmd
  (lines 57-196) + `uls.c` header style (the most recent u*.c)
- [ ] **Step 30** [W][CODE] ~15m: Write `$UP/user/ush.c` to this spec:
  - SPDX `GPL-3.0-or-later`; header comment: UNHEADED sh, Track 2 Phase 2.2
    finale (ADR-081), built AS `target/sh.mbc` (PROGRAM_TABLE basename), MIT
    sh.c stays vendored unwired; scope EXEC-only with the three documented
    divergences (no cd, metachar reject, argc cap 8) and WHY (dead syscalls
    pipe/chdir/sbrk; malloc landmine; harvester ABI)
  - `#include "kernel/types.h"`, `"user/user.h"`, `"kernel/fcntl.h"`
  - `#define USH_MAXARGS 8` (exec harvester ARGV_MAX)
  - `static char whitespace[] = " \t\r\n\v";` `static char metachars[] = "<|>&;()";`
    hand-rolled `in_set()` (NOT xv6 strchr — its never-matches-NUL quirk is a
    footgun here, unlike wc where the quirk was the contract)
  - `panic(s)` → `fprintf(2, "%s\n", s); exit(1);` — stock-identical
  - `fork1()` — stock-identical
  - `tokenize(buf, argv)`: in-place NUL-split on whitespace; `panic("syntax")` on
    any metachar (start or mid-word); `panic("too many args")` at argc==8;
    returns argc; argv[argc]=0
  - `runcmd(buf)` `__attribute__((noreturn))`, runs IN THE CHILD (panics kill the
    child only): tokenize; argc==0 → `exit(1)` (stock EXEC-with-empty-argv
    parity); `exec(argv[0], argv);` then `fprintf(2, "exec %s failed\n",
    argv[0]); exit(0);` — exit(0) is stock parity (runcmd tail)
  - `getcmd(buf, nbuf)` — stock-identical: `write(2, "$ ", 2); memset; gets;
    buf[0]==0 → -1`
  - `main()` — stock-identical shape: fd-ensure loop (`open("console", O_RDWR)`
    until fd>=3), getcmd loop, leading space/tab skip, `*cmd=='\n' || *cmd==0`
    → continue (blank skip; the `||*cmd==0` is defensive — unreachable while the
    KBD ring caps lines under the 100-byte buffer), NO cd branch, `fork1()==0 →
    runcmd(cmd)`, parent `wait(0)`
- [ ] **Step 31** [B] ~1m: Host-side syntax sanity (not the real target — just
  catches typos before the cross build)
  ```bash
  gcc -fsyntax-only -nostdinc -I"$UP" "$UP/user/ush.c" 2>&1 | head; echo "expect only header-related noise or silence"
  ```
  - [D] If real syntax errors (not missing-header noise) → fix; this check is
    advisory only, the real gate is Step 34.
- [ ] **Step 32** [R] ~2m: Self-review against the BlackMage checklist (Phase 7
  list) — every buffer static and bounded, no pointer into freed/child-shared
  state, panics child-side only
- [ ] **Step 33** [B] ~1m: Compile with the REAL flags via the userland Makefile's
  generic %.o rule
  ```bash
  cd "$UP" && make -f ../adapters/Makefile.mbc-userland user/ush.o && riscv64-unknown-elf-objdump -h user/ush.o | head -15
  ```
- [ ] **Step 34** [V]: ush.o builds clean under `-mabi=ilp32e -ffixed-x16..x31
  -O2` with zero warnings
  - [D] Warnings/errors → fix source; 2 failed attempts → Skip Protocol (unlikely;
    this is plain C89-ish xv6 style).
- [ ] **Step 35** [B] ~30s: Confirm no accidental libc/malloc references
  ```bash
  riscv64-unknown-elf-objdump -t "$UP/user/ush.o" | grep -E 'UND' | grep -vE 'write|read|open|close|fork|exec|wait|exit|gets|memset|fprintf|printf' || echo "CLEAN"
  ```
- [ ] **Step 36** [V][GATE]: **PHASE 2 EXIT GATE** — ush.c compiles clean; no
  malloc/sbrk symbols; undefined symbols limited to the expected
  syscall/ulib/printf set. **DoD**: source complete, self-reviewed, not yet wired.

## PHASE 3: MAKEFILE SWAP + BUILD (Steps 37-50)

**Goal**: Wire `target/sh.elf` to link from `ush.o` (explicit rule overriding the
pattern rule), rebuild sh.mbc + siblings + fs.img.
**Prerequisite**: Phase 2 gate
**Time**: ~20m
**Agent**: Coordinator

- [ ] **Step 37** [W] ~3m: Edit `$AD/Makefile.mbc-userland` — append after the
  `$T/ls.elf` rule (line 97-99), same comment style:
  ```make
  # Phase 2.2 finale: sh itself is OURS. Last MIT program out of the exec set.
  $T/sh.elf: $A/crt0_mbc.o $U/ush.o $(USER_LIB_OBJS) $U/user.ld
  	mkdir -p $T
  	$(LD) $(LDFLAGS) -T $U/user.ld -o $@ $A/crt0_mbc.o $U/ush.o $(USER_LIB_OBJS)
  ```
  (crt0_mbc.o FIRST — `_start` must land at RV byte 0; Gate C root cause)
- [ ] **Step 38** [B] ~30s: Diff check — only the one rule added
  ```bash
  git diff --stat "$AD/Makefile.mbc-userland" && git diff "$AD/Makefile.mbc-userland" | head -20
  ```
- [ ] **Step 39** [B][BUILD] ~2m: Force-rebuild sh from scratch
  ```bash
  cd "$UP" && rm -f target/sh.elf target/sh.mbc target/sh.rv2mbc target/sh.data user/sh.o && make -f ../adapters/Makefile.mbc-userland sh
  ```
- [ ] **Step 40** [V]: Build succeeds; link line in output shows `ush.o` not `sh.o`
- [ ] **Step 41** [B] ~30s: **REPLACEMENT PROOF** — FILE symbol
  ```bash
  riscv64-unknown-elf-objdump -t "$K/sh.elf" | grep 'df \*ABS\*'
  ```
- [ ] **Step 42** [V]: Shows `ush.c` (and NOT `sh.c`)
  - [D] Still sh.c → stale sh.o linked; `rm -f "$UP"/user/sh.o "$K"/sh.*` and
    rebuild; verify the explicit rule really overrides (make -n).
- [ ] **Step 43** [B] ~30s: Entry-point proof (Gate C landmine)
  ```bash
  riscv64-unknown-elf-objdump -d "$K/sh.elf" | head -20
  ```
- [ ] **Step 44** [V]: `_start` is the symbol at address 0x0 (crt0 first)
- [ ] **Step 45** [B] ~30s: Siblings emitted
  ```bash
  ls -la "$K"/sh.mbc "$K"/sh.rv2mbc "$K"/sh.data && echo OK
  ```
- [ ] **Step 46** [V]: All three exist with fresh timestamps
- [ ] **Step 47** [B][BUILD] ~2m: Regenerate fs.img (sh is baked into the ramdisk
  program set)
  ```bash
  cd "$UP" && rm -f target/fs.img && make -f ../adapters/Makefile.mbc-userland ramdisk && ls -la target/fs.img
  ```
- [ ] **Step 48** [V]: fs.img regenerated; mkfs output lists `sh` among packed
  programs
- [ ] **Step 49** [B] ~1m: Size sanity — ush should be SMALLER than stock sh
  ```bash
  riscv64-unknown-elf-size "$K/sh.elf" 2>/dev/null || wc -c "$K/sh.mbc"
  ```
- [ ] **Step 50** [V][GATE]: **PHASE 3 EXIT GATE** — sh.mbc + rv2mbc + data built
  from ush.c, `_start` at 0, fs.img fresh. **DoD**: the swap is complete on disk,
  unproven in boot. Do NOT commit yet (green-commit doctrine — boot proof first).

## PHASE 4: FIRST BOOT SMOKE (Steps 51-58)

**Goal**: The "first heartbeat" — one command through OUR sh, byte-identical TTY.
**Prerequisite**: Phase 3 gate
**Time**: ~15m (more if debug branches fire)
**Agent**: Coordinator

- [ ] **Step 51** [B][S] ~2m: Boot `echo hello` through ush
  ```bash
  out="$(boot $'echo hello\n')"; printf '%s\n' "$out" > /tmp/phase22-smoke.full; tty_of "$out"
  ```
- [ ] **Step 52** [V]: **FIRST HEARTBEAT** — TTY ascii line is byte-identical to
  golden
  ```bash
  diff <(tty_of "$(cat /tmp/phase22-smoke.full)") "$GOLD/echo_hello.tty" && echo IDENTICAL
  ```
  - If IDENTICAL → Step 57. If not → [D] Steps 53-56 (max 2 full debug cycles,
    then Skip Protocol → restore stock via `git checkout` of the Makefile +
    rebuild, commit nothing, STUCK report).
- [ ] **Step 53** [D] ~5m: No `$ ` prompt at all → entry point. Re-verify Step
  43-44; check `init: starting sh` printed (init side OK); suspect exec staging:
  boot-log `Program[0] 'sh'` line shows fresh sizes + `hash 0x…` unchanged.
- [ ] **Step 54** [D] ~5m: Prompt but no `hello` → exec/argv path. Check TTY for
  `exec echo failed` (PROGRAM_TABLE miss — rebuild echo.mbc siblings) vs silence
  (child crashed — check STATS forks=/waitpid= vs `$GOLD/echo_hello.stats`).
- [ ] **Step 55** [D] ~5m: Garbled/duplicated output → tokenize wrote NULs wrong
  or blank-skip diverged; replay input `$'echo hello\n'` mentally through
  tokenize(); fix source, rebuild (Steps 39-48 fast path), retry Step 51.
- [ ] **Step 56** [D] ~5m: Boot halts/restarts after prompt → the Gate C/D
  classics: `_start` not at 0 (Step 44), or RET-floor misparse (ROM base is
  assigned per slot — sh slot 0 base 0x6000; floor 0x20000 concerns are for NEW
  bases only — if suspected, compare `Program[0]` ROM base to stock boot log).
- [ ] **Step 57** [B] ~1m: STATS parity
  ```bash
  diff <(stats_of "$(cat /tmp/phase22-smoke.full)") "$GOLD/echo_hello.stats" && echo STATS-IDENTICAL
  ```
  - [V] forks=/waitpid=/tty_r= identical (ush mirrors stock's blank-skip and
    fork discipline, so counters must match). If TTY identical but stats differ →
    investigate before proceeding (hidden extra fork = structural divergence).
- [ ] **Step 58** [V][GATE]: **PHASE 4 EXIT GATE** — `echo hello` byte-identical
  TTY + STATS through OUR sh. **DoD**: first life proven; corpus sweep unlocked.

## PHASE 5: CLAUSE-BY-CLAUSE CORPUS SWEEP (Steps 59-72)

**Goal**: Every golden capture reproduced byte-identically through OUR sh —
the "identical observable behavior" gate, clause by clause.
**Prerequisite**: Phase 4 gate
**Time**: ~25m (9 boots)
**Agent**: Coordinator

- [ ] **Step 59** [B][S][TEST] ~12m: Sweep the 8 stock-init corpus lines
  ```bash
  RED=0
  for spec in "ls" "cat README" "cat BIGFILE" "cat sub/NOTE" "ls sub" "echo hello" "echo peace and love" "wc README"; do
    out="$(boot "$spec"$'\n')"; slug="$(tr ' /' '__' <<<"$spec")"
    if diff -q <(tty_of "$out") "$GOLD/$slug.tty" >/dev/null \
       && diff -q <(stats_of "$out") "$GOLD/$slug.stats" >/dev/null; then
      echo "IDENTICAL  $spec"
    else
      echo "DIVERGED   $spec"; printf '%s\n' "$out" > "/tmp/phase22-diverged-$slug.full"; RED=1
    fi
  done; echo "sweep RED=$RED"
  ```
- [ ] **Step 60** [V]: All 8 IDENTICAL (RED=0)
  - If any DIVERGED → [D] Step 61 per line; 2 failed fix cycles on any one line →
    Skip Protocol.
- [ ] **Step 61** [D] ~5m/line: `diff <(tty_of "$(cat /tmp/phase22-diverged-*.full)") "$GOLD/<slug>.tty"`
  — classify: missing output (exec/fd issue), extra bytes (prompt count — check
  blank-skip parity), reordered (write path — should be impossible, single
  in-order TTY). Fix ush.c, rebuild (Steps 39+47 fast path), re-run just that line.
- [ ] **Step 62** [B][S] ~2m: uinit round-trip through OUR sh
  ```bash
  out="$(cd "$EBPF" && sudo "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" \
    --userland "$K/uinit.mbc" --triggers 4000000 --instance 111 --input $'echo hello\n' 2>&1)"
  diff <(tty_of "$out") "$GOLD/uinit_echo.tty" && diff <(stats_of "$out") "$GOLD/uinit_echo.stats" && echo UINIT-IDENTICAL
  ```
- [ ] **Step 63** [V]: UINIT-IDENTICAL (banner token + hello + same counters)
- [ ] **Step 64** [B][S] ~2m: New-gate probes through ush — unknown command
  ```bash
  out="$(boot $'nosuch\n')"; diff <(tty_of "$out") "$GOLD/nosuch.tty" && echo N1-IDENTICAL
  ```
- [ ] **Step 65** [V]: N1-IDENTICAL (`exec nosuch failed`, same prompt shape)
- [ ] **Step 66** [B][S] ~2m: Multi-command through ush (if H3 held)
  ```bash
  out="$(boot $'echo alpha\necho omega\n')"; diff <(tty_of "$out") "$GOLD/multi.tty" && echo N2-IDENTICAL
  ```
- [ ] **Step 67** [V]: N2-IDENTICAL
- [ ] **Step 68** [B][S] ~2m: Blank-line resilience through ush (if H5 held)
  ```bash
  out="$(boot $'\necho unbroken\n')"; diff <(tty_of "$out") "$GOLD/blank.tty" && echo N3-IDENTICAL
  ```
- [ ] **Step 69** [V]: N3-IDENTICAL
- [ ] **Step 70** [B][S] ~2m: Divergence documentation probe — `cd` through ush
  (EXPECTED divergence, record actual text for the session log)
  ```bash
  out="$(boot $'cd sub\n')"; tty_of "$out" | tee /tmp/phase22-cd-divergence.txt
  ```
- [ ] **Step 71** [V]: ush prints `exec cd failed` and survives to a fresh prompt
  (this is the DOCUMENTED divergence — stock silently no-ops; ours is honest)
- [ ] **Step 72** [V][GATE]: **PHASE 5 EXIT GATE** — 8 corpus lines + uinit +
  N1-N3 all byte-identical to golden; cd divergence recorded. **DoD**: every
  clause the corpus exercises is proven through OUR sh.

## PHASE 6: FULL REGRESSION + LOAD (Steps 73-80)

**Goal**: The whole harness — both build variants, Doom load test, budgets
EXACTLY unchanged.
**Prerequisite**: Phase 5 gate
**Time**: ~10m
**Agent**: Coordinator

- [ ] **Step 73** [B][S][TEST] ~6m: Full harness
  ```bash
  bash scripts/upc-regression.sh 2>&1 | tee /tmp/phase22-final-harness.txt | tail -30
  ```
- [ ] **Step 74** [V]: `== ALL GREEN ==` — all 15 gates (12 legacy + N1-N3)
  - Any red → [D] the failing gate's boot is reproducible via Phase 5 commands;
    2 failed cycles → Skip Protocol (restore stock Makefile wiring, keep gates,
    STUCK report).
- [ ] **Step 75** [V]: **ZERO-eBPF-CHANGE GATE** — harness notes show ascend
  verifier EXACTLY `verified 900031 insns (90.0% of 1M ceiling)`
- [ ] **Step 76** [V]: Doom verifier EXACTLY `verified 737087 insns` + `Doom
  object loads` PASS
  - 75/76 changed → something touched eBPF; `git status ebpf/` must be empty —
    if budgets moved with a clean ebpf/ tree, STOP and investigate the build
    cache before believing any number.
- [ ] **Step 77** [B] ~30s: Confirm the working tree diff is EXACTLY the swap
  ```bash
  git status --short && git diff --stat
  ```
- [ ] **Step 78** [V]: Changes = `user/ush.c` (new) + `Makefile.mbc-userland`
  only (harness gates already committed in Phase 1)
- [ ] **Step 79** [B] ~1m: Re-verify replacement proof post-harness (the harness
  cleanup rebuilds objects — make sure sh.elf is still ours)
  ```bash
  riscv64-unknown-elf-objdump -t "$K/sh.elf" | grep 'df \*ABS\*'
  ```
- [ ] **Step 80** [V][GATE]: **PHASE 6 EXIT GATE** — ALL GREEN, budgets frozen,
  diff minimal, FILE symbol = ush.c. **DoD**: ship-ready state proven.

## PHASE 7: SECURITY REVIEW GATE — BlackMage (Steps 81-88)

**Goal**: Adversarial review of the new input-facing surface.
**Prerequisite**: Phase 6 gate
**Time**: ~15m
**Agent**: Coordinator (checklist review + probe boots)

- [ ] **Step 81** [SEC][R] ~5m: Checklist against ush.c source:
  - [ ] All buffers static + fixed (buf[100], argv[9]); no VLA, no malloc, no sbrk
  - [ ] Tokenizer writes only within buf (in-place NUL split; gets() bounds nbuf)
  - [ ] argc hard-capped at 8 BEFORE array write (no off-by-one on argv[8]=0)
  - [ ] Metachars rejected at first sight — start-of-word AND mid-word
  - [ ] All panics run in the CHILD → hostile line cannot kill the shell
  - [ ] No new syscalls used (fork/exec/wait/open/close/read/write only)
- [ ] **Step 82** [B][S][SEC] ~2m: Hostile probe — metachar line
  ```bash
  out="$(boot $'echo a | cat\n')"; tty_of "$out"
  ```
- [ ] **Step 83** [V]: TTY shows `syntax` and a surviving fresh prompt (shell not
  killed); no partial exec of `echo`
- [ ] **Step 84** [B][S][SEC] ~2m: Hostile probe — arg flood (9 args > cap 8)
  ```bash
  out="$(boot $'echo a b c d e f g h i\n')"; tty_of "$out"
  ```
- [ ] **Step 85** [V]: TTY shows `too many args` and a surviving prompt
- [ ] **Step 86** [B][S][SEC] ~2m: Hostile probe — 63-byte max-length line (ring
  boundary; 64th byte is the newline)
  ```bash
  out="$(boot "echo $(printf 'x%.0s' {1..57})"$'\n')"; tty_of "$out"
  ```
- [ ] **Step 87** [V]: Prints the x-run (truncated to 15 chars by the HARVESTER,
  not by ush — expected, pre-existing ABI) and survives; no crash/restart
- [ ] **Step 88** [V][GATE]: **PHASE 7 EXIT GATE** — checklist all checked, 3
  hostile probes survived. **DoD**: attack surface reviewed; divergence notes
  captured for the session log.

## PHASE 8: COMPLIANCE GATE (Steps 89-93)

**Goal**: License boundary clean.
**Prerequisite**: Phase 7 gate
**Time**: ~5m
**Agent**: Coordinator

- [ ] **Step 89** [B][COMPLIANCE] ~30s: SPDX on the new file
  ```bash
  head -1 "$UP/user/ush.c" | grep 'GPL-3.0-or-later' && echo SPDX-OK
  ```
- [ ] **Step 90** [V]: SPDX-OK
- [ ] **Step 91** [B][COMPLIANCE] ~30s: MIT sh.c untouched, still vendored
  ```bash
  git diff --stat HEAD -- "$UP/user/sh.c" | wc -l | grep -q '^0$' && echo SH-C-UNTOUCHED
  ```
- [ ] **Step 92** [V]: SH-C-UNTOUCHED; zero new dependencies (ush.c includes only
  the three vendored xv6 headers)
- [ ] **Step 93** [V][GATE]: **PHASE 8 EXIT GATE** — GPL/MIT boundary identical
  to the four prior coreutil swaps. **DoD**: nothing for the Barrister.

## PHASE 9: SHIP + DOC RIPPLES (Steps 94-110)

**Goal**: Commit 2 (the swap + core docs), commit 3 (session log + handoff).
**Prerequisite**: Phase 8 gate
**Time**: ~30m
**Agent**: Coordinator

- [ ] **Step 94** [DOC-UPDATE][W] ~3m: ADR-081 `## Progress` — append bullet:
  `**2026-07-05 — Phase 2.2 sh replaced. Phase 2.2 COMPLETE.** user/ush.c
  (EXEC-only fork/exec/wait loop, malloc-free — stock sh's parser mallocs via
  the unimplemented sbrk(12) and worked by accident; PIPE/REDIR/LIST/BACK + cd
  are dead code behind unimplemented pipe(4)/chdir(9)). Corpus + uinit
  round-trip byte-identical; 3 new behavior-lock gates landed green against
  stock BEFORE the swap. The exec set is now 100% Unheaded-authored. Phase 2.3
  (kernel edges) is next.`
- [ ] **Step 95** [DOC-UPDATE][W] ~3m: Master plan
  (`docs/battle-plans/UPC-LINUX-MASTER-BATTLE-PLAN.md`) — check the sh bullet
  `[x]` with a one-line summary + date + pointer to this plan; mark Phase 2.2
  COMPLETE in its heading line
- [ ] **Step 96** [B] ~30s: Doc-only diff sanity
  ```bash
  git diff --stat
  ```
- [ ] **Step 97** [V]: Diff = ush.c + Makefile + 2 docs (+ this plan file if
  updated). Nothing else.
- [ ] **Step 98** [W] ~3m: Commit-2 message file (use -F; backticks in body):
  subject `feat(upc): Track 2 Phase 2.2 — sh replaced (ush). Phase 2.2 COMPLETE`,
  body per the house coreutil template: mechanism (explicit Makefile rule →
  target/sh.mbc, MIT sh.c vendored unwired), the EXEC-only scope + WHY (dead
  syscalls, malloc landmine), RV→MBC instruction counts (from Step 39 output),
  ELF FILE symbol proof, gate evidence (corpus + uinit byte-identical, N1-N3,
  ALL GREEN), `Zero eBPF change: ascend verifier 900,031 (90.0%) unchanged,
  Doom green.`, `Remaining MIT userland in the exec set: none — exec set 100%
  Unheaded-authored.`, files list, trailer
  `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`
- [ ] **Step 99** [C][B] ~1m: **COMMIT 2 — THE SHIP COMMIT**
  ```bash
  git add "$UP/user/ush.c" "$AD/Makefile.mbc-userland" docs/adr/ADR-081-unheaded-linux-from-scratch.md docs/battle-plans/UPC-LINUX-MASTER-BATTLE-PLAN.md docs/battle-plans/PHASE22-SH-REPLACE.md
  git commit --no-gpg-sign -F /tmp/phase22-commit2.txt && git log --oneline -1
  ```
- [ ] **Step 100** [V]: Commit landed; `git status --short` clean
- [ ] **Step 101** [B][S][TEST] ~6m: Post-commit harness (prove the committed
  tree is green, not just the pre-commit working tree)
  ```bash
  bash scripts/upc-regression.sh 2>&1 | tail -5
  ```
- [ ] **Step 102** [V]: `== ALL GREEN ==` on the committed tree
- [ ] **Step 103** [W] ~10m: Session log
  `references/phase22-sh-shipped-2026-07-05.md` — header per phase17 precedent
  (`**Session**/**Battle plan**/**Result**`), commits list, the malloc/sbrk
  root-cause narrative, the four-lens decisions, divergence table (cd /
  metachars / argc cap) with observed stock-vs-ush behavior from Steps 70-71 +
  82-87, golden-capture method note
- [ ] **Step 104** [W] ~3m: SHIPPED banner atop THIS plan (blockquote: gate
  results, commit range, one-line root cause, session-log path, ADR path)
- [ ] **Step 105** [W] ~5m: Refresh `~/tmp/next.md` — Phase 2.2 COMPLETE
  (uinit + echo/cat/wc/ls/sh all ours), next candidates: Phase 2.3 kernel edges
  / Epic 1.3.1 device inodes / verifier-headroom spike; carry forward footguns
  verbatim
- [ ] **Step 106** [W] ~2m: Commit-3 message file: subject `docs(upc): Phase 2.2
  sh ship — session log, SHIPPED banner, handoff refresh`, trailer as always
- [ ] **Step 107** [C][B] ~1m: **COMMIT 3**
  ```bash
  git add references/phase22-sh-shipped-2026-07-05.md docs/battle-plans/PHASE22-SH-REPLACE.md
  git commit --no-gpg-sign -F /tmp/phase22-commit3.txt && git log --oneline -3
  ```
  (next.md lives OUTSIDE the repo at ~/tmp/next.md — edited, not committed)
- [ ] **Step 108** [V]: 3 commits total on main for this sprint; tree clean
- [ ] **Step 109** [B] ~30s: Leave the on-disk eBPF object in the dev-default
  ascend state (the harness cleanup already does this; verify)
  ```bash
  ls -la "$EBPF/target/bpfel-unknown-none/release/monad-cpu-ebpf" && echo "harness cleanup rebuilt ascend — OK"
  ```
- [ ] **Step 110** [V][GATE]: **PHASE 9 EXIT GATE / SPRINT COMPLETE** — 3 green
  commits, docs rippled, handoff refreshed, exec set 100% Unheaded-authored.
  **DoD**: a fresh session can pick up Phase 2.3 from next.md alone.

---

## APPENDIX A: EMERGENCY PROCEDURES

**A1 — Boot prints nothing / `BOOT FAIL: MRET fall-through`**: wrong eBPF build
variant on disk. `cd "$EBPF" && cargo build --release -p monad-cpu-ebpf
--features ascend-linux`. (`invalid value size 144/136` = same disease.)

**A2 — Post-`$` halt + reboot loop**: entry point regression. Verify `_start` at
address 0 in sh.elf (Step 43); verify crt0_mbc.o is FIRST in the link line;
verify `user.ld` still has `ENTRY(_start)` + `*(.text._start)` first.

**A3 — `exec sh failed` from init**: PROGRAM_TABLE staging miss — sh.mbc /
sh.rv2mbc / sh.data siblings missing or stale next to --userland. Rebuild:
`cd "$UP" && make -f ../adapters/Makefile.mbc-userland sh ramdisk`.

**A4 — Blank names / size 0 in ls output**: ilp32e `struct stat` regression
(16-byte layout, size at offset 12) — NOT a sh problem; do not chase it in ush.c.

**A5 — Runaway execution / PC in the weeds after a command**: RET-floor
disambiguation (0x20000). sh is slot 0 (ROM base 0x6000) so this should not fire;
if it does, compare the boot log `Program[0]` line against a stock boot.

**A6 — Harness red on a gate that Phase 5 proved IDENTICAL**: the harness
cleanup/rebuild raced your manual builds. Re-run the harness once from a quiet
tree before debugging.

**A7 — Full rollback**: `git checkout -- "$AD/Makefile.mbc-userland"` (leave
ush.c in place, it's inert unwired), rebuild `sh ramdisk`, re-run harness →
stock sh restored, gates N1-N3 still green (they were locked against stock).

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Phase | Agent | Parallel? | Depends on | Est |
|-------|-------|-----------|------------|-----|
| 0 Preflight | Coordinator | no | — | 30m |
| 1 Behavior lock | Coordinator | no | 0 | 25m |
| 2 ush.c | Coordinator | no | 1 | 30m |
| 3 Makefile+build | Coordinator | no | 2 | 20m |
| 4 Smoke | Coordinator | no | 3 | 15m |
| 5 Corpus sweep | Coordinator | no | 4 | 25m |
| 6 Full regression | Coordinator | no | 5 | 10m |
| 7 Security | Coordinator | no | 6 | 15m |
| 8 Compliance | Coordinator | no | 7 | 5m |
| 9 Ship+docs | Coordinator | no | 8 | 30m |

Critical path = all of it (strictly sequential; the boot is the shared resource).
Total ≈ 3h25m nominal, 5h with debug branches.

## APPENDIX C: QUICK REFERENCE

**Boot (from ebpf/ ALWAYS)**:
`sudo $BC boot --kernel $K/xv6-mbc.mbc --ramdisk $K/fs.img --userland $K/init.mbc --triggers 4000000 --instance 111 --input $'CMD\n'`

**Userland rebuild**: `cd $UP && make -f ../adapters/Makefile.mbc-userland sh ramdisk`

**ABI limits**: line ≤64B incl `\n` (KBD ring) · 8 args max (ARGV_MAX) · 15
chars/arg + NUL (ARG_CAP=16) · argv strings @ VA 0x600000, array @ 0x600080 ·
a0=argc a1=0x600080 · SP top 0x500000 · prompt `write(2,"$ ",2)`

**Dead syscalls (silent no-op)**: pipe(4), chdir(9), sbrk(12), kill(6),
uptime(14), unlink(18), link(19), mkdir(20)

**Live syscalls**: fork(1) exit(2) wait(3) read(5) exec(7) fstat(8) chdir— no —
dup(10) getpid(11) pause(13) open(15) write(16) mknod(17) close(21)

**Replacement proof**: `riscv64-unknown-elf-objdump -t $K/sh.elf | grep 'df \*ABS\*'` → `ush.c`

**Budgets (frozen this sprint)**: ascend 900,031 (90.0%) · Doom 737,087

**Commit style**: `-F <file>`, `--no-gpg-sign`, trailer
`Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`

---

*Phase 2.2 Finale Battle Plan — Forged 2026-07-05*
*10 Phases. 110 Steps. The last MIT program leaves the exec set.*
*PID 1 is ours; the coreutils are ours; tonight, the shell.*
