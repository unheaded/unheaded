<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# S-STAGE-2C BATTLE PLAN — 19 Phases, 318 Steps

**Date**: 2026-11-12
**Sprint**: S-STAGE-2C. Close the confinement holes ADR-135's review left open, build the probes it offered, then climb EVOLUTION-1 Stage 2 rungs 3 to 7: console write, console read, fstat/dup/close with the `fs_walk` hand-off, and fork/exit/wait into the kernel's authority.
**Carries**: backlog sweep bundle 5 (`docs/sweeps/BACKLOG-SWEEP-2026-11-12.md` rows D-25, D-27), and Stevie's answers of 2026-11-12 (`~/tmp/next.md`, "Session 2026-11-12 evening", rounds 2 and 3): A-60 enable D6 and reject MRET/SRET at priv 3 as the first phase; A-61 zero `.bss` at exec, route LR_W/SC_W/XCHG/CAS through `translate_address` with an opcode-scan tripwire, O_RDWR on a regular file takes the fs path, a bad `.data` sidecar fails the load; A-62 every offered probe; ADR-135 F7 and F8 as parked ADRs; a `cd` builtin in ush.
**Prerequisite**: this plan committed on `develop`; `develop` at or after `59a8fbd2`; ascend 889,941 and default 758,199 in `docs/policy/ebpf-insns-baseline.tsv`; `upc0` up on west; `~/tmp/projects/unheaded-doom` doom-runner built; no other lane booting a UPC.
**Target**: `UPC_D6_MISS_FAULT` on with MRET/SRET refused at priv 3, `hostile_reply` closed on every route; every A-61 fix and A-62 probe in the harness; `UPC_RUNG3`, `UPC_RUNG4`, `UPC_RUNG56` and `UPC_RUNG7` on, each behind its own const with a measured verifier delta; `fs_walk` and `FD_INODE_MAP` out of the ascend object; `cd` in ush.
**Estimated Duration**: Stops A-D (hardening and probes) about 14 h: one long night. Stops E-F (console rungs) about 7 h. Stop G (rungs 5+6) about 9 h. Stop H (rung 7) about 9 h. Three to four nights. See STOPPING POINTS.
**Branch**: `sprint/s-stage-2c` in worktree `~/tmp/wt/s-stage-2c` (series rule 1). Merge with `git merge --no-ff -S`. Never push.
**Agent Strategy**: Serial lane, one phase at a time, no swarm. `upc0`, the verifier and `~/tmp/golden` are single-tenant. Every eBPF, kernel, rung, ABI, root-script and `[S]` step is Opus (series rule 13). Sonnet takes the probe sources, the parked ADRs and the doc ripple. Haiku takes nothing here: every phase touches a boot or a design call.
**Commit Cadence**: every 5 steps where the work allows, at every const-off point, every gate and before every skip; `cmt` signs, stages named files only, adds the two trailers. Message `[PLAN S-STAGE-2C] Steps X-Y: ...`.
**Binding**: series rules 1-17, `docs/battle-plans/SPRINT-SERIES-2026-11-04.md`. Rule 13: cheaper models never touch a root script. Rule 14: never `pkill -f`; stop by recorded pid. Rule 15: merge develop in, rebuild, close-gate, then merge. Rule 17: `insrow` patches one baseline row and files the number under For Stevie.
**Stuck Protocol**: 3x a step's estimate or 2 failed debug attempts: mark `[STUCK]`, commit known-good state, move to the next independent step. A rung whose gate cannot pass stops at its last Stop: flip its const off, rebuild, confirm the off object equals the previous count, go to Phase 16.
**Standing rules**: no third-party anything (cargo-fuzz stays absent: SKIP is not a pass); no host changes beyond boots inside `upc0`; no firewall, sshd, reboot or `compose up`; Doom gets nothing beyond keeping 758,199; no LoC figures; probes and mutants that are not the deliverable are reverted before a commit; goldens are never recaptured to pass a rung (one named policy recapture, Phase 15, with reason and HEAD).

---

## LEGEND

| Tag | Meaning |
|---|---|
| [B] | Bash command |
| [V] | Verification: must pass before proceeding |
| [D] | Debug: only if the previous [V] failed, at most 2 attempts |
| [W] | Write or edit a file |
| [R] | Read or inspect |
| [S] | Needs sudo, so Opus only (every boot, `vcount`, `sweep12`, `gate`, the harness, `check-ebpf-insns.sh`, `upc-netns.sh`) |
| [C] | Commit checkpoint |
| [CODE] / [TEST] / [BUILD] | Code change / test run / build |
| [DESIGN] | Design decision written down |
| [DECIDE] | Decision with a recommendation: proceed with it unless the Override holds |
| [SECURITY] | Hostile-input probe or review |
| [STOP] | A clean stopping point (see STOPPING POINTS) |
| [STUCK] | Skipped via the Skip Protocol |
| [BLOCKED] | Blocked by an upstream [STUCK] |
| [P] | Parallelizable (none in this plan; kept for the legend's contract) |

Code blocks under a step are indented two spaces for Markdown: strip the indent when running them, heredoc terminators included.

**Model tags** (series rules 10-12): **Sonnet** = well-defined work with full commands, expected output, tests first. **Opus** = judgement: verifier, rung, ABI, concurrency, security.

---

## VARIABLES

Every Bash call is a new shell, so every step starts, from `~/tmp/wt/s-stage-2c`, with: `source <(sed -n '/^# VARS-BEGIN/,/^# VARS-END/p' docs/battle-plans/S-STAGE-2C-battle-plan.md)`. Values such as `offv` and `nv` live in `$S/v.env`, set by `keep`.

Changed from S-STAGE-2B's block: every boot passes `--allow-untrusted-ebpf-obj-dir` (bootctl L17, `d126c437`, refuses an object directory a non-root user can write, and the worktree's `ebpf/target` is govan's; the harness already passes it), paths and the session trailer.

```bash
# VARS-BEGIN
export PROJECT_ROOT="$(git rev-parse --show-toplevel)"
export EBPF="$PROJECT_ROOT/ebpf" BC="$PROJECT_ROOT/cmd/upc-bootctl/target/release/upc-bootctl"
export UP="$PROJECT_ROOT/crates/xv6-mbc/upstream" AD="$PROJECT_ROOT/crates/xv6-mbc/adapters"
export K="$UP/target" GOLD="$HOME/tmp/golden" S="$HOME/tmp/s-stage-2c"
export LOG="$PROJECT_ROOT/references/s-stage-2c-log.md"
export PROG="$PROJECT_ROOT/docs/battle-plans/progress/S-STAGE-2C-progress.md"
export PLAN="$PROJECT_ROOT/docs/battle-plans/S-STAGE-2C-battle-plan.md"
export MAIN="$EBPF/monad-cpu-ebpf/src/main.rs"
export UT="--allow-untrusted-ebpf-obj-dir"
export CARGO_BUILD_JOBS=4 GOMAXPROCS=4
mkdir -p "$S"
[ -f "$S/v.env" ] && . "$S/v.env"
keep()     { sed -i "/^$1=/d" "$S/v.env" 2>/dev/null; printf '%s="%s"\n' "$1" "$2" >> "$S/v.env"; export "$1=$2"; echo "$1=$2"; }
boot()     { sudo bash "$PROJECT_ROOT/scripts/upc-netns.sh" up >/dev/null && (cd "$EBPF" && sudo ip netns exec upc0 env UPC_DEMO_MODE=1 "$BC" boot $UT --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" \
             --userland "$K/init.mbc" --triggers "${CTRIG:-4000000}" --instance 111 --input "$1" 2>&1); }
tty_of()   { grep -a 'ascii:' <<<"$1"; }
stats_of() { grep -aoE 'forks=[0-9]+|waitpid=[0-9]+|tty_r=[0-9]+' <<<"$1" | tr '\n' ' '; }
sweep()    { local f=0; for spec in "ls" "cat README" "cat BIGFILE" "cat sub/NOTE" "ls sub" "echo hello" \
             "echo peace and love" "wc README"; do out="$(boot "$spec"$'\n')"; s="$(tr ' /' '__' <<<"$spec")"; \
             diff -q <(tty_of "$out") "$GOLD/$s.tty" >/dev/null && diff -q <(stats_of "$out") "$GOLD/$s.stats" >/dev/null \
             && echo "SAME $s" || { echo "DIFF $s"; f=1; }; done; return $f; }
sweep12()  { sweep; local o; for spec in nosuch:$'nosuch\n' multi:$'echo alpha\necho omega\n' blank:$'\necho unbroken\n'; do
             o="$(boot "${spec#*:}")"; diff -q <(tty_of "$o") "$GOLD/${spec%%:*}.tty" >/dev/null && diff -q <(stats_of "$o") "$GOLD/${spec%%:*}.stats" >/dev/null \
               && echo "SAME ${spec%%:*}" || echo "DIFF ${spec%%:*}"; done
             o="$(cd "$EBPF" && sudo ip netns exec upc0 env UPC_DEMO_MODE=1 "$BC" boot $UT --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" \
               --userland "$K/uinit.mbc" --triggers "${CTRIG:-4000000}" --instance 111 --input $'echo hello\n' 2>&1)"
             diff -q <(tty_of "$o") "$GOLD/uinit_echo.tty" >/dev/null && diff -q <(stats_of "$o") "$GOLD/uinit_echo.stats" >/dev/null \
               && echo "SAME uinit_echo" || echo "DIFF uinit_echo"; }
ub()       { local user="$1"; shift; (cd "$EBPF" && sudo ip netns exec upc0 env UPC_DEMO_MODE=1 "$@" "$BC" boot $UT --kernel "$K/xv6-mbc.mbc" \
             --ramdisk "$K/fs.img" --userland "$K/$user.mbc" --triggers "${TRIG:-1500000}" --instance 111 2>&1); }
pl()       { grep -a 'ascii:' | sed -E 's/([0-9A-F]{4} )+//g' | sed 's/\xc2\xb7/\n/g' | grep -aoE "$1: [a-z0-9-]+(=[a-z0-9]+)?"; }
ebuild()   { (cd "$EBPF" && cargo build --release -p monad-cpu-ebpf --features ascend-linux 2>&1 | tail -1); }
dbuild()   { (cd "$EBPF" && cargo build --release -p monad-cpu-ebpf 2>&1 | tail -1); }
bbuild()   { (cd "$PROJECT_ROOT/cmd/upc-bootctl" && cargo build --release 2>&1 | tail -1); }
kbuild()   { (cd "$UP" && rm -f kernel/kernel.elf && make -f ../adapters/Makefile.mbc kernel 2>&1 | tail -1); }
gbuild()   { (cd "$UP" && rm -f target/fs.img && make -f ../adapters/Makefile.mbc-userland userland gates ramdisk 2>&1 | tail -1); }
vcount()   { ub getpidfork UPC_VERIFIER_STATS=1 | grep -aoE 'verified [0-9]+ insns' | tail -1 | grep -oE '[0-9]+'; }
budget()   { if [ "$1" -le 900000 ] && [ "$1" -lt 921036 ]; then echo "BUDGET-OK $1 (headroom $((900000-$1)), ladder net $((921036-$1)))"; else echo "BUDGET-FAIL $1"; return 1; fi; }
memfloor() { awk '/MemAvailable/{exit ($2/1024 < 4096)}' /proc/meminfo && echo MEM-OK || { echo "MEM-LOW $(awk '/MemAvailable/{print int($2/1024)}' /proc/meminfo) MiB"; return 1; }; }
flip()     { sed -i "s/^const $1: bool = $2;/const $1: bool = $3;/" "$MAIN" && grep -c "^const $1: bool = $3;" "$MAIN" && ebuild; }
gate()     { local L="$1" rc same diff v; memfloor || return 1
             sweep12 > "$S/$L-sweep12.txt" 2>&1
             bash "$PROJECT_ROOT/scripts/upc-regression.sh" > "$S/$L-harness.txt" 2>&1; rc=$?
             v="$(vcount)"; same=$(grep -c '^SAME' "$S/$L-sweep12.txt"); diff=$(grep -c '^DIFF' "$S/$L-sweep12.txt")
             doom=$(grep -ac 'verified 758199 insns' "$S/$L-harness.txt")
             echo "gate $L: same=$same diff=$diff rc=$rc doom=$doom ascend=$v head=$(git -C "$PROJECT_ROOT" rev-parse --short HEAD)" | tee -a "$S/gates.txt"
             [ "$same" = 12 ] && [ "$rc" = 0 ] && grep -aq 'verified 758199 insns' "$S/$L-harness.txt" && git -C "$PROJECT_ROOT" rev-parse HEAD > "$S/last-green"; }
ledger()   { printf '| %s | %s | %s | %+d | %d | %s |\n' "$1" "$2" "$3" "$(($3-$2))" "$((921036-$3))" "$4" >> "$LOG"; }
insrow()   { sed -i -E "s/^(monad-cpu-ebpf\/monad_cpu\tascend-linux\t)[0-9]+(\t[^\t]+\t)[0-9-]+$/\1$1\2$(date +%F)/" "$PROJECT_ROOT/docs/policy/ebpf-insns-baseline.tsv"; grep -c "ascend-linux.$1" "$PROJECT_ROOT/docs/policy/ebpf-insns-baseline.tsv"; forstevie "ascend baseline row now $1 (net vs 921036: $((921036-$1)))" >/dev/null; }
rep()      { python3 - "$1" "$2" "$3" <<'PY'
import sys
f, o, n = sys.argv[1:4]
s = open(f).read()
c = s.count(o)
if c != 1:
    print(f"REP-FAIL {f}: old text found {c} times"); sys.exit(1)
open(f, "w").write(s.replace(o, n)); print(f"REP-OK {f}")
PY
}
cmt()      { local m="$1"; shift; local t=$'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>\nClaude-Session: https://claude.ai/code/session_01TzL3TtXpgkofCw7tn2rc3g'
             git -C "$PROJECT_ROOT" add -- "$@" || return 1
             git -C "$PROJECT_ROOT" commit -S -m "$m" -m "$t" || { git -C "$PROJECT_ROOT" commit --no-gpg-sign -m "$m" -m "$t" && echo "- unsigned commit: $m" >> "$PROG"; }; }
forstevie() { python3 - "$PROG" "$1" <<'PY'
import sys
f, t = sys.argv[1:3]
L = open(f).read().split("\n")
i = L.index("## For Stevie") + 1
while i < len(L) and not L[i].startswith("## "):
    i += 1
while i > 0 and L[i - 1] == "":
    i -= 1
L.insert(i, "- " + t)
open(f, "w").write("\n".join(L)); print("FORSTEVIE-OK")
PY
}
# Rung boilerplate: BEFORE GATE for any rung phase.
before()   { gate "$1-before" && keep offv "$(vcount)"; }
# VARS-END
```

---

## STOPPING POINTS

Each Stop leaves a mergeable tree: goldens 12/12, harness ALL GREEN, Doom 758,199, baseline row current. If the night is ending or a rung will not gate, go to Phase 16 from the last Stop reached.

| Stop | After | State at that point | Cumulative |
|---|---|---|---|
| A | Phase 2 | D6 on; MRET/SRET refused at priv 3; `hostile_reply` closed on every route | ~5 h |
| B | Phase 5 | bounded user paths, slot 0 bounded, pipe bad buffer -1, atomics translated, opcode scan, `.bss` zeroed at exec, sidecar fatal | ~10 h |
| C | Phase 7 | fork tagged cursor, slot reservation, I9 tripwire; BPF refuses a write to a file fd (T-F3 interim) | ~12 h |
| D | Phase 8 | every A-62 probe gated; `upc-rung0-assert.awk` in the harness | ~14 h |
| E | Phase 9 | rung 3: console write in the kernel | ~18 h |
| F | Phase 10 | rung 4: console read in the kernel | ~21 h |
| G | Phase 12 | rungs 5+6: kernel fd table is the only one; `fs_walk` and `FD_INODE_MAP` out | ~30 h |
| H | Phase 14 | rung 7: fork, exit and wait under the kernel's authority; exit status through wait | ~39 h |
| I | Phase 15 | `cd` in ush, one named policy recapture of `ls` | ~40 h |

Times exclude the ~2 h of Phases 16-18 that every ending pays.

**Deadline.** Step 1 sets `DEADLINE`, an epoch: 13 h from start by default. **Next night**: `cd ~/tmp/wt/s-stage-2c`, source VARIABLES, `keep DEADLINE "$(( $(date +%s) + 13*3600 ))"`, `gate resume`, then the phase after the last Stop.

**Run cost.** A `gate` is ~15 min (12 boots plus the harness, which rebuilds the Doom object). A rung needs two gates and two to four eBPF builds: about an hour of machine time before any debugging.

## SWARM TABLE (series rule 10)

| Phases that can run at once | Why |
|---|---|
| None | The lane is serial: every phase shares the worktree, `upc0` and the object a later phase builds. Phase 1's two ADRs touch no code but share `ADR-INDEX.md`, so they run in order too. |

## MEMORY, VERIFIER AND SUDO

- Same as S-STAGE-2B: `memfloor` before every `gate` (4 GiB available, swap never counted); `CARGO_BUILD_JOBS=4`; never run `check-ebpf-insns.sh` during a boot; never `Makefile.mbc clean`.
- Sudo only in `[S]` steps, Opus only. Builds, parity tests, docs and `make` never under sudo (root-owned files in `target/`).
- **Verifier headroom at forge time is 10,059** (889,941 against 900,000). Phases 2-7 add checks before any rung frees anything. Appendix A1 is the plan's answer; the order fallback is in Step 30's [DECIDE].

---

## GROUND TRUTH (read from the tree 2026-11-12 at `59a8fbd2`)

| Fact | Where | Effect on this plan |
|---|---|---|
| Rung consts `UPC_RUNG2` through `UPC_RUNG_PIPE`, `UPC_REPLY`, `GETPID_INJECT`, `UPC_RUNG0`, `DEFERRED_SWITCH` are true; `UPC_D6_MISS_FAULT` and `UPC_PROBE_INJECT22` false | `main.rs:428-489` | Phase 2 flips D6; every new rung adds one const |
| Baseline rows: ascend 889,941, default 758,199, dated 2026-11-09 | `docs/policy/ebpf-insns-baseline.tsv` | Step 5 checks both |
| `MRET` (`main.rs:1415`) and `SRET` (`:1440`) arms have no privilege test. At priv 3 an opcode with no arm falls to "Unknown opcode: treat as NOP" (`:3474`) | the arms | R-F1's fix: add `cpu.priv_level != 3` to both arm conditions, so a U-mode MRET/SRET is a NOP. Step 26 decides NOP against a trap |
| `XCHG` (`:1375`) and `CAS` (`:1382`) are not `cfg`-gated: they are in the Doom object too; `LR_W` (`:1492`) and `SC_W` (`:1498`) are ascend-only. None calls `translate_address` | the arms | R-S4's fix must be `cfg!(feature = "ascend-linux")`-guarded or Doom's 758,199 moves |
| `d6_user_miss()` sends a U-mode page-walk miss to `0xFFFF_FFFC`, outside `RAM_MAP` (`:3942`); `translate_address` at `:3991` | | D6 is one const; its cost is unmeasured |
| BPF `open` (`:2796`) and `exec` (`:3250`) compute `pid_phys_offset(pid) + path_va` with no bound. `user_phys()` (`:257`) already bounds `[va, va+len)` under `USER_VA_CEILING` | | R-F5's fix is `user_phys(pid, path_va, PATH_SPAN)`, -1 on `None` |
| Harness: `hostile_reply` has a D6-on branch (`upc-regression.sh:259-262`) expecting `own-slice-intact`, `table-reached=no`, `route=closed`, `heap-intact`, `done`; no line covers a direct U-mode `sret`/`mret` | | Phase 2 adds `sret-refused` and `mret-refused` to the probe and both branches |
| `upc_user_pa()`: slot 0's base is 0, so its only bounds are the first page and the 8 MiB ceiling | `upc_slots.c:156-162` | D-27: Phase 3 bounds slot 0 to init's own ranges |
| `pipewrite` breaks out of its loop on a failed `copyin` and returns `i`, which is 0 for a bad first byte | `upipe.c:116-125` | D-27: return -1 when `i == 0` |
| Fork's first entry is keyed on child pde 0 not present (`:2352`); exec keys on a tagged a3 (`EXEC_ARGV_MAGIC`, `:3314`) | fork arm | R-F3: key fork on a tagged a3 as well |
| The only `interrupt_pending = 1` setter is the timer block behind `cfg!(not(feature = "ascend-linux"))` (`:777-782`); nothing ties I9 to it | | R-F6: a `const _: () = assert!(...)` tripwire |
| The user `--userland` `.data` sidecar only warns on failure (`upc-bootctl/src/main.rs:948-960`); a resident program's bad sidecar already fails through `?` (`:540-545`), but a missing one silently gives `(0, 0)` | bootctl | A-61: init's bad sidecar fatal; a missing resident sidecar fatal when the ELF has `ALLOC PROGBITS` data |
| `crt0_mbc.S` calls `main` with no `.bss` pass; exec copies `data_len_words` only; PROGRAM_TABLE word 7 is reserved | `crt0_mbc.S`, `main.rs:361` | R-NC: Step 82 decides between a zero tail in the staged blob (no BPF change) and a BPF zero pass |
| `UPC_TTY_DATA` is a `uint32` store in `console-mmio.c`, which is why kernel `printf` shows char plus NULs in the TTY | `console-mmio.c` | rung 3 must store one byte per char, or every golden moves |
| The `ls` golden prints `sh 2 3 7496`: corpus binary sizes are in it | `~/tmp/golden/ls.tty` | `cd` in ush changes one number: Phase 15's named recapture |
| `usertests.c` is not built; `ut_exitwait`, `ut_forktest`, `ut_reparent2`, `ut_twochildren` exist as probes | `user/` | rung 7 gates add `ut_*` probes, not the binary |
| `scripts/upc-rung0-assert.awk` exists; `upc-regression.sh` never calls it | grep | A-62 item 7 |
| `rung_gate` lines and counts: SB 13, KL 12, MK 7, RM 8, LN 9, FD 49, CD 24, PI 20, HU 5 | `upc-regression.sh:207-215` | every extended probe updates its count in the same commit |
| doom-runner lives in `~/tmp/projects/unheaded-doom` since DP-305; the harness finds it by `$DR` | `upc-regression.sh:28-29` | a gate is green only with `doom=1`: Step 3 checks the runner exists |
| ADR-135's F7 and F8 are the first review's (Step 119), recorded in `references/s-stage-2b-log.md:63-64`, not its R-F7/R-F8 | log | Phase 1 parks those two |
| The last ADR is 144; S-WATCH-TRUE and S-RESIDUE edit ADR-144 and the index but add no ADR at forge time | `docs/adr/` | Phase 1 takes the next two free numbers at run time |
| `~/tmp/wt/s-stage-2b` still exists (merged) | `git worktree list` | leave it; this plan has its own worktree |

### Departures from the skeleton (all forced by the tree)

- **Exec stays a resident-program lookup at rung 7.** The kernel cannot make MBC: `.rv2mbc` translation is host-side, and `PROGRAM_TABLE` is how exec finds code. Rung 7 moves fork, exit, wait and exit status under the kernel's authority; exec's path read, `.bss` zero and argv stay in BPF. Phase 13 records this in the ADR.
- **Rungs 5 and 6 land as one gate** (skeleton's first kickoff option): splitting the FS-metadata bundle is the D1 split brain.
- **Kernel fd numbers drop D10's +16 offset at rung 6**: fds 0-2 must be kernel console fds once BPF's table retires. `fdprobe`, `chdirprobe` and `pipeprobe` lines that say "at or above 16" change in the same commit (Step 216).

---

## GROUNDING (Marshal, Busboy, Micromanager, Computermancer passes, 2026-11-12)

**Rows checked against the tree.** D-25 holds as the sweep words it. D-27 holds, with three refinements: R-F5's bound already exists as `user_phys()` and the two arms skip it; slot 0 is also refused its first page, not only the ceiling; `pipewrite` returns 0 rather than -1 because it returns its byte count. A-61's sidecar row is narrower than worded: a resident program's bad sidecar already fails; the gaps are init's (warn only) and a missing resident sidecar (silent). A-62's "forged replies with D6 on" already has a harness branch; what it lacks is a line for a direct U-mode `sret`/`mret`. Not in the sweep, found here: `XCHG`/`CAS` sit in the Doom object; kernel console output is a word store; `cd` moves the `ls` golden.

**Computermancer.** Order is right: D6 is the confinement boundary, and every later rung adds kernel C that parses user pointers, so the boundary comes first. The risk is verifier budget: 10,059 headroom and Phases 2-7 only add. Rungs 3 and 5+6 free instructions only if their const compiles the BPF arm out (`!UPC_RUNGn && syscall_nr == ...`), not merely aliases around it; Steps 166 and 218 say so. Read each edit with `vcount`; S-STAGE-2B showed deltas are not additive. Anything touching a shared opcode carries `cfg!(feature = "ascend-linux")` so Doom stays at 758,199.

**Marshal.** In scope: D-25, D-27, the A-60/A-61/A-62 answers, the F7 and F8 parked ADRs, `cd` in ush. Out, and parked where named: ush pipeline syntax (rung 6 makes `dup` onto 0-2 possible; the shell grammar is a separate item, For Stevie at Step 241); the `usertests` binary; timer preemption; exec scrub R-F4 and F8's reap scrub (ADR only); D14 changes; installing cargo-fuzz; any wire-format change; Doom. Rung 7 may come back from its design review as its own plan: Step 248's [DECIDE] allows that, and Stop G is then the end.

**Busboy.** File overlap with other forged plans: S-RESIDUE edits `docs/adr/ADR-INDEX.md` and the timeline mirrors, and adds `scripts/timeline-mirrors.py` (Step 304 uses it if it has landed); S-WATCH-TRUE edits `ADR-INDEX.md`, `compliance/findings/register.yaml` and the `latency-probe` row of `ebpf-insns-baseline.tsv`; S-DOOM-LAST edits `upc-regression.sh` Variant 2 (Doom is last, so no overlap in time). Merge rule A9 covers the TSV; ADR numbers are taken at run time (Step 15). `upc0` is single-tenant: no other lane boots while this one runs.

**Micromanager.** A rung is done when: corpus 12/12 SAME, harness ALL GREEN with `doom=1`, `budget` OK, the const-off object equals the previous count, the rung's probe 0 `=no` lines, its mutants fail as predicted, the ADR amended (status stays Proposed: acceptance is Stevie's), the ledger row and baseline row written. Security fixes need a probe line that fails when the fix is reverted (a mutant), not only a passing line.

---

## KNOWN FAILURES BASELINE

Empty at forge time. Step 10 records anything else. A failure on that list is not this sprint's to fix unless a rung needs it.

---

## PHASE 0: PREFLIGHT, WORKTREE, BASELINES (Steps 1-14)

**Goal**: the worktree builds everything the boots use, the baselines match, and the gate is green before any change.
**Prerequisite**: this plan committed on `develop`; no other lane booting.
**Time**: ~1 h
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 1** [B] ~1m: Worktree, projects symlink, deadline.
  ```bash
  ln -sfn /home/govan/tmp/projects /home/govan/tmp/wt/projects
  git -C ~/tmp/unheaded worktree add ~/tmp/wt/s-stage-2c -b sprint/s-stage-2c develop && cd ~/tmp/wt/s-stage-2c && git log -1 --format='%h %G?'
  source <(sed -n '/^# VARS-BEGIN/,/^# VARS-END/p' docs/battle-plans/S-STAGE-2C-battle-plan.md); keep DEADLINE "$(( $(date +%s) + 13*3600 ))"
  ```
  Expected: `Preparing worktree`, one hash with `G`, `DEADLINE=<epoch>`. Branch exists: `cd ~/tmp/wt/s-stage-2c`, carry on. Else `[STUCK]`.
- [ ] **Step 2** [V] ~30s: `type ub pl ebuild dbuild bbuild kbuild gbuild boot sweep sweep12 vcount budget memfloor flip gate ledger insrow rep cmt keep forstevie before | grep -c 'is a function'` → `22`. Else `[STUCK]`.
- [ ] **Step 3** [B][S] ~1m: Tools, sudo, netns, Doom runner.
  ```bash
  for t in riscv64-unknown-elf-gcc bpf-linker bpftool perl xxd readelf /usr/bin/time python3; do command -v "$t" >/dev/null && echo "ok $t" || echo "MISSING $t"; done; sudo -n true && echo sudo-ok; ip netns list | grep -c upc0; ls ~/tmp/projects/doom-related/DOOM.WAD ~/tmp/projects/unheaded-doom/crates/doom-runner/target/release/doom-runner ~/tmp/projects/unheaded-doom/doom/doom.elf | wc -l
  ```
  Expected: eight `ok`, `sudo-ok`, `1`, `3`. `0` for upc0: `sudo bash "$PROJECT_ROOT/scripts/upc-netns.sh" up` and repeat. Fewer than `3`: the gate cannot see `doom=1`; `[STUCK]`, every gate is `[BLOCKED]`.
- [ ] **Step 4** [W] ~2m: Create `$LOG` and `$PROG`.
  ```bash
  { printf '# S-STAGE-2C log\n\nStarted %s on %s (branch sprint/s-stage-2c).\nPlan: docs/battle-plans/S-STAGE-2C-battle-plan.md\n\n## Ledger (verified insns, ascend)\n\n| Change | OFF | ON | delta | net vs 921,036 | gate |\n|---|---|---|---|---|---|\n' "$(date +%F)" "$(git rev-parse --short HEAD)"; } > "$LOG"
  { printf '<!--\nSPDX-License-Identifier: GPL-3.0-or-later\nCopyright (c) 2024-2026 Stevie Bellis.\n-->\n\n# S-STAGE-2C progress\n\nPlan: `docs/battle-plans/S-STAGE-2C-battle-plan.md`. Evidence: `references/s-stage-2c-log.md`.\n\nSkip Protocol acknowledged: skip after 3x the estimate or 2 failed debug attempts, mark [STUCK], commit known-good state, continue with the next independent step.\n\n## For Stevie\n\n'; } > "$PROG"; wc -l "$LOG" "$PROG"
  ```
  Expected: `9` and `13` lines plus a total. Else `[STUCK]`.
- [ ] **Step 5** [R] ~2m: Hypotheses, one command.
  ```bash
  cd "$PROJECT_ROOT"; grep -cE '^const UPC_D6_MISS_FAULT: bool = false;|^const UPC_RUNG_PIPE: bool = true;|^const UPC_RUNG_FD: bool = true;|^const UPC_REPLY: bool = true;' "$MAIN"; grep -cP 'monad_cpu\tascend-linux\t889941\t' docs/policy/ebpf-insns-baseline.tsv; grep -cP 'monad_cpu\tdefault\t758199\t' docs/policy/ebpf-insns-baseline.tsv; grep -c 'upc-rung0-assert' scripts/upc-regression.sh; grep -c 'priv_level != 3' "$MAIN"; sed -n '/opc == op::MRET/p;/opc == op::SRET/p' "$MAIN" | grep -c priv_level
  ```
  Expected, in order: `4`, `1`, `1`, `0`, a number (record it as `p3`), `0`. Different: paste into `$PROG`, `[STUCK]`, stop the phase.
- [ ] **Step 6** [B] ~1m: `keep p3 "$(grep -c 'priv_level != 3' "$MAIN")"; keep adr_last "$(ls docs/adr | grep -oE '^ADR-[0-9]+' | grep -v 69420 | sort -t- -k2 -n | tail -1 | cut -d- -f2)"`. Expected `adr_last=144` or higher (another plan may have taken one).
- [ ] **Step 7** [BUILD][B] ~12m: Build everything, measuring the eBPF build.
  ```bash
  memfloor; (cd crates/monad-mbc && cargo build --release 2>&1 | tail -1); kbuild; gbuild; ( cd "$EBPF" && /usr/bin/time -v cargo build --release -p monad-cpu-ebpf --features ascend-linux > "$S/p0-ebuild-time.txt" 2>&1 ); grep -a 'Finished' "$S/p0-ebuild-time.txt"; bbuild; ls "$K"/*.rv2mbc.sha256 | wc -l; ls "$K"/getpidfork.mbc "$K"/fs.img "$BC" | wc -l; grep 'Maximum resident' "$S/p0-ebuild-time.txt"
  ```
  Expected: `MEM-OK`; each cargo line `Finished ...`; at least 35 sidecars (S-STAGE-2B close); `3`; one `Maximum resident` line. Error: Step 8.
- [ ] **Step 8** [D] ~5m: Only on a build error: `grep -nE '^error|No rule to make' "$S"/*.txt | head; (cd "$UP" && make -f ../adapters/Makefile.mbc kernel 2>&1 | tail -5)`. Paste into `$PROG`, `[STUCK]`, stop. A fresh worktree that fails is a tree fact, not this lane's fix.
- [ ] **Step 9** [B][S] ~2m: One boot: `TRIG=3000 ub getpidfork UPC_VERIFIER_STATS=1 | grep -aE 'verified [0-9]+ insns' | tail -1`. Expected `verified 889941 insns`. A different count: Step 10 decides; record it in `$LOG` under `## Run cost` either way.
- [ ] **Step 10** [TEST][B][S] ~30m: The baseline gate: `gate p0; bash scripts/check-ebpf-insns.sh 2>&1 | tail -2`. Expected `gate p0: same=12 diff=0 rc=0 doom=1 ascend=889941 ...`, the ratchet PASS, `pgrep -c doom-runner` → `0`. `DIFF` or `rc=1`: copy `grep -a FAIL "$S/p0-harness.txt"` and the `DIFF` names into `$LOG` under `## Known failures`, `[STUCK]`, stop: a red baseline means no change is measurable.
- [ ] **Step 11** [B][S] ~3m: Record the harness's own lines for the probes this plan extends: `grep -aE 'hostile_reply|hostile_ptr|rmprobe|lnprobe|sleepprobe|fdprobe|pipeprobe|chdirprobe|hostile_pgd' "$S/p0-harness.txt" > "$S/p0-probes.txt"; wc -l < "$S/p0-probes.txt"`. Expected at least 9 lines; `hostile_reply` says D6 is off and both forgeries land (the open hole, pinned).
- [ ] **Step 12** [W] ~2m: `$LOG` §Before: the p0 gate line, the probe lines, `p3`, `adr_last`, `Maximum resident`.
- [ ] **Step 13** [C] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 1-13: worktree, baselines, run cost" references/s-stage-2c-log.md docs/battle-plans/progress/S-STAGE-2C-progress.md` → one commit; `git log -1 --format=%G?` → `G`, or `N` with a line in `$PROG`.
- [ ] **Step 14** [V] ~1m: **PHASE 0 EXIT GATE**: `grep -c 'same=12 diff=0 rc=0 doom=1' "$S/gates.txt"` → at least `1`; `git status --short | wc -l` → `0`. Else stop.

---

## PHASE 1: PARKED ADRS F7 AND F8, ANSWERS ON RECORD (Steps 15-24)

**Goal**: ADR-135's first-review ideas F7 (link-time check that user images end below 0x10000) and F8 (scrub a slice at reap) exist as Proposed ADRs, parked; Stevie's 2026-11-12 answers are in `$PROG`.
**Prerequisite**: Phase 0.
**Time**: ~40 min
**Agent**: Coordinator (Sonnet for the text)
**Model**: Sonnet

- [ ] **Step 15** [B] ~30s: `keep adr_f7 "$((adr_last+1))"; keep adr_f8 "$((adr_last+2))"; ls docs/adr | grep -cE "^ADR-($adr_f7|$adr_f8)-"` → `0`. Not `0`: another lane took the number; `keep adr_last` to the new last and repeat.
- [ ] **Step 16** [R] ~5m: Read `references/s-stage-2b-log.md:52,63,64`, ADR-135 D2, D5 and Security notes ("Bytes between processes"), and one recent parked ADR for format (`docs/adr/ADR-142-wotan-as-a-redis-class-store.md` header).
- [ ] **Step 17** [W] ~10m: `docs/adr/ADR-$adr_f7-upc-user-image-ceiling-check.md`. Status `Proposed (parked 2026-11-12, Stevie's answer A-61)`. Context: ADR-135 D2's heap window starts at 0x10000 and assumes every user image ends below it; measured largest `sh` 0x2084 (log line 63); nothing checks it. Decision (proposed, not built): `Makefile.mbc-userland` fails a link whose `_end` reaches 0x10000, and bootctl refuses an image whose `.data` sidecar records an address at or above 0x10000. Consequences: a growing program fails at build, not as heap corruption. Not built until opened. SPDX header, terse.
- [ ] **Step 18** [W] ~10m: `docs/adr/ADR-$adr_f8-upc-scrub-slice-at-reap.md`. Status as Step 17. Context: ADR-135 Security notes: nothing scrubs a slice at reap, so a slot's next occupant reads the last one's stack and argv frame; D5's zeroing is correctness, not confidentiality (log line 64). Decision (proposed): the reap path zeroes the dead slot's stack window and argv frame (and, with R-F4, the band exec does not overwrite), chunked across ticks as sbrk is; measure the verifier and per-reap cost first. Consequences: closes the dead-to-next leak; with D6 on (Phase 2) the live-to-live route is already closed, so this is the remaining leak. Not built until opened.
- [ ] **Step 19** [W] ~5m: Two rows in `docs/adr/ADR-INDEX.md` in number order, Proposed count +2, total +2; same two rows in `wiki/ADR-Index.md` and its counts (the wiki was behind by one at S-STAGE-2B's close: add the ADR-135 row there too if still missing). `grep -cE "\[($adr_f7|$adr_f8)\]" docs/adr/ADR-INDEX.md wiki/ADR-Index.md` → `2` each.
- [ ] **Step 20** [W] ~3m: ADR-135 §Review gains one dated line under the first review's table: "F7 and F8 parked as ADR-$adr_f7 and ADR-$adr_f8 (2026-11-12, Stevie)". `grep -c "ADR-$adr_f7" docs/adr/ADR-135-upc-kernel-reply-channel.md` → `1`.
- [ ] **Step 21** [W] ~3m: `forstevie` once per answer, quoting `~/tmp/next.md` rounds 2 and 3: A-60, A-61 (four parts), A-62 (all probes), F7/F8 parked, `cd` in ush.
- [ ] **Step 22** [B] ~1m: `bash scripts/check-spdx.sh 2>&1 | tail -1` → PASS.
- [ ] **Step 23** [C] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 15-23: ADR-$adr_f7 and ADR-$adr_f8 parked (ADR-135 F7, F8); answers recorded" docs/adr/ADR-$adr_f7-upc-user-image-ceiling-check.md docs/adr/ADR-$adr_f8-upc-scrub-slice-at-reap.md docs/adr/ADR-INDEX.md wiki/ADR-Index.md docs/adr/ADR-135-upc-kernel-reply-channel.md docs/battle-plans/progress/S-STAGE-2C-progress.md`.
- [ ] **Step 24** [V] ~30s: **PHASE 1 EXIT GATE**: tree clean; both files exist with `Status` Proposed.

---

## PHASE 2: D6 ON, MRET AND SRET REFUSED AT PRIV 3 (Steps 25-52)

**Goal**: a child cannot leave U-mode except by `ecall`, and cannot reach memory outside its slice. `hostile_reply` closes on every route; `hostile_pgd` passes its D6-on branch.
**Prerequisite**: Phase 1. A-60 answered (enable D6 and reject at priv 3, first).
**Time**: ~3.5 h
**Agent**: Coordinator; one review agent at Step 29
**Model**: Opus

- [ ] **Step 25** [V][S] ~1m: `keep offv "$(vcount)"`; `echo $offv` → `889941`.
- [ ] **Step 26** [DECIDE][DESIGN][W] ~20m: ADR-109 gains a dated D6 section, ADR-135 Security notes a dated line. **MRET/SRET at priv 3: NOP or trap?** RISC-V raises illegal-instruction. Here a trap needs a kill path from a slot row; `kexit` panics there (I4), and exit through the kernel arrives at rung 7. A NOP keeps `priv_level` at 3 and `pc` advancing: no escalation, no new path, near zero verifier cost, and the existing "Unknown opcode" fall-through does it. RECOMMENDATION: NOP now (add `&& cpu.priv_level != 3` to both arms), revisited at rung 7 (Step 250 decides). Override: Step 33 measures more than +3,000 for the two tests, then try the shared form in Step 34 before deciding. Write: D6 on means a U-mode miss goes to `D6_MISS_ADDR` (reads 0, stores dropped); the tagged `ret` to a ROM MRET word is covered because the test is on the opcode, wherever it is reached; R-S1 (`main` prologue after `mret`) closes with it.
- [ ] **Step 27** [TEST][W] ~25m: Extend `user/hostile_reply.c` (red first): two new routes in their own child each, run before the PDE route: a direct inline `mret` and a direct inline `sret` at U-mode, each followed by a `getpid()`; tokens `HR: mret-refused=yes` when the child is still at U-mode afterwards (its next `ecall` round-trips and returns its own pid, and a store to a kernel `.bss` VA lands in its own slice) and `HR: sret-refused=yes` likewise. With today's tree both print `=no` (the child jumps into kernel `main`/a chosen PC). Keep every existing line.
- [ ] **Step 28** [W] ~10m: `scripts/upc-regression.sh` `hostile_reply` block: D6-on branch wants `HR: mret-refused=yes` 1 and `HR: sret-refused=yes` 1 added to its list; the D6-off branch notes them, does not require them (a D6-off tree with the opcode fix still refuses; one without it does not). `bash -n scripts/upc-regression.sh && echo syntax-ok`.
- [ ] **Step 29** [R] ~20m: Review agent, read-only, given Steps 26-28 and `main.rs` MRET/SRET/`translate_address`/`d6_user_miss`. Ask: does any legitimate path execute MRET or SRET at priv 3 (`start_mbc.c`, `uentry.S`, the bootstub); does D6 drop a store the corpus makes (an exec'd program touching VA ≥ 8 MiB: argv frame 0x600000 and stack top 0x500000 are below, check); does `hostile_slot`/`hostile_tf` change meaning with D6 on. Coordinator waits; dispositions into `$LOG` §Phase 2 review.
- [ ] **Step 30** [DECIDE] ~5m: **Order under budget.** RECOMMENDATION: proceed with D6 first, as Stevie asked. Override: if Step 37's `nv` is BUDGET-FAIL, Phases 3-8 cannot land before a rung frees instructions; then run Phase 9 (rung 3, which compiles out the BPF write arm) next and return to Phase 3 after it, recording the reorder in `$PROG`.
- [ ] **Step 31** [CODE][W] ~10m: `$MAIN`: `opc == op::MRET` arm condition becomes `cfg!(feature = "ascend-linux") && opc == op::MRET && cpu.priv_level != 3`; same for SRET. Comment cites ADR-109 D6 (2026-11-12) and R-F1.
- [ ] **Step 32** [BUILD][B][S] ~5m: `ebuild; keep a "$(vcount)"; echo "priv3 check: $a delta $((a-offv))"; budget "$a"`.
- [ ] **Step 33** [V] ~30s: `BUDGET-OK`; delta recorded. Over +3,000: Step 34.
- [ ] **Step 34** [D] ~15m: Hoist the test: one `let user_ret = cpu.priv_level == 3;` before the opcode chain and `&& !user_ret` in both arms; rebuild, `vcount`. Still over +3,000 with headroom gone: `[STUCK]`, revert Step 31, Stop A is not reachable; go to Step 30's override.
- [ ] **Step 35** [TEST][B][S] ~3m: D6 still off: `TRIG=3000000 ub hostile_reply | pl HR > "$S/hr-p3.txt"; grep -cE 'HR: (mret|sret)-refused=yes' "$S/hr-p3.txt"; grep -c 'HR: done' "$S/hr-p3.txt"`. Expected `2`, `1`.
- [ ] **Step 36** [TEST][B] ~5m: Mutant: drop `&& cpu.priv_level != 3` from SRET only, `ebuild`, rerun Step 35 → `sret-refused=yes` count `0`. Restore, `ebuild`, `[ "$(vcount)" = "$a" ] && echo restored`.
- [ ] **Step 37** [CODE][B][S] ~5m: `flip UPC_D6_MISS_FAULT false true; keep nv "$(vcount)"; echo "d6=$nv delta=$((nv-a))"; budget "$nv"`.
- [ ] **Step 38** [V] ~30s: `BUDGET-OK`. FAIL: Step 30's override; flip D6 back, commit Step 31's part alone at Step 40 and record.
- [ ] **Step 39** [TEST][B][S] ~8m: `TRIG=3000000 ub hostile_reply | pl HR > "$S/hr-d6.txt"; TRIG=1500000 ub hostile_pgd | pl HP > "$S/hp-d6.txt"; cat "$S/hr-d6.txt" "$S/hp-d6.txt"; sweep > "$S/d6-sweep8.txt" 2>&1; grep -c '^SAME' "$S/d6-sweep8.txt"`.
- [ ] **Step 40** [C] ~1m: Const-off point is not this phase's shape (D6 is an existing const). Commit the probe and harness first: `cmt "[PLAN S-STAGE-2C] Steps 25-36: MRET and SRET refused at priv 3; hostile_reply covers both" ebpf/monad-cpu-ebpf/src/main.rs crates/xv6-mbc/upstream/user/hostile_reply.c scripts/upc-regression.sh docs/adr/ADR-109-*.md docs/adr/ADR-135-upc-kernel-reply-channel.md references/s-stage-2c-log.md` with D6 flipped back to false for this commit (`flip UPC_D6_MISS_FAULT true false` before, `flip ... false true` after; `vcount` equals `a` and `nv` respectively).
- [ ] **Step 41** [V] ~1m: Step 39: `hr-d6.txt` holds `own-slice-intact=yes`, `table-reached=no`, `route=closed`, `heap-intact=yes`, `mret-refused=yes`, `sret-refused=yes`, `done`, and no `forged=yes`; `hp-d6.txt` shows the stores dropped; sweep `8`. Else Step 42.
- [ ] **Step 42** [D] ~20m: A corpus `DIFF` with D6 on: a corpus program touches a VA with no PDE. `diff <(tty_of "$(boot $'ls\n')") "$GOLD/ls.tty" | head`; read the halt or the missing bytes; the likely cause is a VA the fork copy or the argv frame expects mapped. Fix in the pgd setup only if Step 29's review named it; otherwise `[STUCK]`, D6 back off, and Stop A carries the priv-3 fix alone.
- [ ] **Step 43** [TEST][B] ~5m: Mutant: D6 on, `d6_user_miss` forced false (edit the body to `false`), `ebuild`, Step 39's `hostile_pgd` line → stores land. Restore; `vcount` equals `nv`.
- [ ] **Step 44** [TEST][B][S] ~15m: **AFTER GATE**: `gate d6-after`.
- [ ] **Step 45** [V] ~1m: `same=12 diff=0 rc=0 doom=1`; harness lines `hostile_reply: D6 closes the route to the reply word` and `hostile_pgd: a child's stores outside its slice are dropped (D6)` PASS.
- [ ] **Step 46** [D] ~10m: Harness red on `hostile_slot` or `hostile_tf` only: those probes store at VAs below 8 MiB (own slice) and must hold either way; read their lines against Step 11's. Different meaning, not a fault: amend the harness comment only with the review's disposition. A real fault: `[STUCK]`.
- [ ] **Step 47** [W] ~10m: ADR-109: D6 status "on since 2026-11-12, S-STAGE-2C Phase 2", measured cost. ADR-135 Security notes: dated paragraph "Closed 2026-11-12: D6 on and MRET/SRET at priv 3 are NOPs; `hostile_reply` closes on all routes; R-F1, R-F2's `sret` route, R-S1 closed; F1's live-to-live heap route closed; R-F5 remains until Phase 3."
- [ ] **Step 48** [B][S] ~3m: `ledger 'priv3 + D6' "$offv" "$nv" d6-after; insrow "$nv"; bash scripts/check-ebpf-insns.sh 2>&1 | tail -1`.
- [ ] **Step 49** [C] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 37-49: ADR-109 D6 on; hostile_reply closed on every route" ebpf/monad-cpu-ebpf/src/main.rs docs/adr/ADR-109-*.md docs/adr/ADR-135-upc-kernel-reply-channel.md docs/policy/ebpf-insns-baseline.tsv references/s-stage-2c-log.md docs/battle-plans/progress/S-STAGE-2C-progress.md`.
- [ ] **Step 50** [V] ~30s: `grep -c '^const UPC_D6_MISS_FAULT: bool = true;' "$MAIN"` → `1`; tree clean.
- [ ] **Step 51** [W] ~2m: `forstevie "D6 on and MRET/SRET at priv 3 are NOPs (not an illegal-instruction kill; Step 250 revisits at rung 7). Cost: $((nv-offv)). hostile_reply closed on every route."`; commit with Step 52.
- [ ] **Step 52** [V][STOP][C] ~1m: **PHASE 2 EXIT GATE, STOP A**: `cmt "[PLAN S-STAGE-2C] Step 51: Stop A recorded" docs/battle-plans/progress/S-STAGE-2C-progress.md`; tree clean; `git rev-parse HEAD > "$S/last-green"`.

---
## PHASE 3: BOUNDED USER PATHS, SLOT 0 BOUNDED, PIPE BAD BUFFER (Steps 53-72)

**Goal**: BPF `open` and `exec` read user paths only inside the caller's slice (R-F5); a kernel-routed pointer from slot 0 cannot reach kernel memory (D-27); `write()` from a bad buffer on a pipe returns -1 (D-27).
**Prerequisite**: Stop A.
**Time**: ~2 h
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 53** [V][S] ~1m: `keep offv "$(vcount)"`; equals Phase 2's `nv`.
- [ ] **Step 54** [R] ~10m: Slot 0's real VA use, from the tree: `for f in "$K"/init.elf "$K"/getpidfork.elf; do readelf -lW "$f" | awk '/LOAD/{print FILENAME": "$3, $6}' FILENAME="$f"; nm "$f" | grep -E ' _end$'; done; grep -nE 'sp|stack' crates/xv6-mbc/adapters/uexec.c | head -20; readelf -SW "$K"/../kernel/kernel.elf | grep -E '\.rodata|\.data|\.bss'`. Write to `$LOG` §Slot 0: image end, init's stack window, kernel `.rodata`/`.data`/`.bss` ranges.
- [ ] **Step 55** [DESIGN][W] ~15m: ADR-135 gains a dated **D16. Bounded user paths and slot 0's window**: BPF `open` and `exec` take `user_phys(pid, path_va, MAXPATH)` (128, `param.h`) and answer -1 on `None`, so a path that starts within 128 bytes of the ceiling is refused (no corpus path sits there); `upc_user_pa()` for slot 0 accepts only init's image and stack windows from Step 54 and refuses everything else, the kernel's ranges included; `pipewrite` returns -1 when its first `copyin` fails and the short count otherwise, as Linux does. RECOMMENDATION as written. Override: Step 54 shows init's stack inside a kernel range: `[STUCK]`, the slot 0 half waits, the other two land.
- [ ] **Step 56** [TEST][W] ~25m: `user/hostile_path.c`, tokens `HQ:`, registered in `Makefile.mbc-userland` gates as S-STAGE-2B Phase 4 did. Child: `open((char*)0x00900000, 0)` → -1 (`HQ: open-wild-refused=yes`); `exec((char*)0x00900000, argv)` → -1 (`HQ: exec-wild-refused=yes`); a path at `0x007FFFF8` → -1 (`HQ: open-ceiling-refused=yes`); `write(p[1], (char*)0x00900000, 4)` on a pipe → -1 (`HQ: pipe-badbuf-refused=yes`). Parent (slot 0): `mkdir((char*)<kernel .rodata from Step 54>)` → -1 and no new directory (`HQ: slot0-kernel-name-refused=yes`); `pipe((int*)<kernel .data base>)` → -1 (`HQ: slot0-copyout-refused=yes`). Then `HQ: done`. Six `=yes` lines.
- [ ] **Step 57** [CODE][W] ~15m: `$MAIN` open arm (`:2796`) and exec arm (`:3250`): `let path_phys = match user_phys(pid, path_va, MAXPATH_BYTES) { Some(p) => p, None => { cpu.regs[8] = (-1i32) as u32; /* the arm's existing return */ } };` with `const MAXPATH_BYTES: u32 = 128;`. Both arms are ascend-only, so Doom is untouched.
- [ ] **Step 58** [CODE][W] ~15m: `upc_slots.c` `upc_user_pa()`: slot 0 window per D16; `upipe.c` `pipewrite`: on `copyin` failure, `if(i == 0) { release(&pi->lock); return -1; }` before the break.
- [ ] **Step 59** [BUILD][B][S] ~6m: `kbuild; gbuild; ebuild; bbuild; keep nv "$(vcount)"; echo "paths delta=$((nv-offv))"; budget "$nv"`.
- [ ] **Step 60** [V] ~30s: `BUDGET-OK`. FAIL: Appendix A1; the exec bound is the cheaper half, land it alone and `[STUCK]` the open half.
- [ ] **Step 61** [TEST][B][S] ~4m: `TRIG=4000000 ub hostile_path | pl HQ > "$S/hq.txt"; grep -c '=yes$' "$S/hq.txt"; grep -c '=no$' "$S/hq.txt"; grep -c 'HQ: done' "$S/hq.txt"; sweep > "$S/hq-sweep8.txt" 2>&1; grep -c '^SAME' "$S/hq-sweep8.txt"`. Expected `6`, `0`, `1`, `8`.
- [ ] **Step 62** [D] ~15m: `slot0-*=no`: the kernel-routed call took the BPF path (check the alias for 20 and the fd routing for `pipe`), or the window admits the address; read `upc_user_pa` against Step 54. `open-ceiling=no`: `user_phys` uses `checked_add` and `end > USER_VA_CEILING`; a 128-byte span from `0x007FFFF8` must fail. A corpus `DIFF`: a corpus path within 128 bytes of the ceiling would be a surprise; print the path VA and `[STUCK]`.
- [ ] **Step 63** [TEST][B] ~10m: Three mutants, one at a time, each rebuilt and booted, each restored: open arm back to `pid_phys_offset + path_va` → `open-wild-refused=no`; slot 0 window removed → `slot0-kernel-name-refused=no`; `pipewrite` -1 removed → `pipe-badbuf-refused=no`. After restoring: `vcount` equals `nv`.
- [ ] **Step 64** [W] ~3m: `scripts/upc-regression.sh`: `rung_gate UPC_D6_MISS_FAULT hostile_path 4000000 HQ 6` after the `hostile_ptr` line. `bash -n` → syntax-ok.
- [ ] **Step 65** [C] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 53-65: open and exec paths bounded, slot 0 window, pipe bad buffer -1" ebpf/monad-cpu-ebpf/src/main.rs crates/xv6-mbc/adapters/upc_slots.c crates/xv6-mbc/adapters/upipe.c crates/xv6-mbc/upstream/user/hostile_path.c crates/xv6-mbc/adapters/Makefile.mbc-userland scripts/upc-regression.sh docs/adr/ADR-135-upc-kernel-reply-channel.md references/s-stage-2c-log.md`.
- [ ] **Step 66** [TEST][B][S] ~15m: **AFTER GATE**: `gate paths-after`.
- [ ] **Step 67** [V] ~1m: `same=12 diff=0 rc=0 doom=1`; `hostile_path: 6 checks` PASS.
- [ ] **Step 68** [D] ~10m: Harness red elsewhere: `hostile_ptr`'s mkdir lines use a pointer the new slot 0 window now refuses differently (both answer -1, so a changed line means a changed path); compare with `$S/p0-probes.txt`.
- [ ] **Step 69** [W] ~5m: ADR-135 Security notes, dated: R-F5 closed (R-S2's oracle with it), slot 0 bounded, pipe write -1. Review table rows R-F5 and R-S2 get "Closed 2026-11-12, S-STAGE-2C Phase 3".
- [ ] **Step 70** [B][S] ~2m: `ledger 'bounded paths' "$offv" "$nv" paths-after; insrow "$nv"; bash scripts/check-ebpf-insns.sh 2>&1 | tail -1`.
- [ ] **Step 71** [C] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 66-71: paths gate green; R-F5 closed" docs/adr/ADR-135-upc-kernel-reply-channel.md docs/policy/ebpf-insns-baseline.tsv references/s-stage-2c-log.md docs/battle-plans/progress/S-STAGE-2C-progress.md`.
- [ ] **Step 72** [V] ~30s: **PHASE 3 EXIT GATE**: tree clean; `git rev-parse HEAD > "$S/last-green"`.

---

## PHASE 4: ATOMICS THROUGH TRANSLATE_ADDRESS, OPCODE SCAN (Steps 73-90)

**Goal**: `LR_W`, `SC_W`, `XCHG`, `CAS` reach memory only through `translate_address` in ascend builds (R-S4), and bootctl refuses a user image that carries any of them or `MRET`/`SRET` (the tripwire).
**Prerequisite**: Phase 3.
**Time**: ~2.5 h
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 73** [V][S] ~1m: `keep offv "$(vcount)"`.
- [ ] **Step 74** [R] ~5m: `grep -nE 'LR_W|SC_W|XCHG|CAS|0x3D|0x3E|0x49|0x4A' crates/monad-mbc/src/translator.rs | head; grep -nE 'pub const (LD|XCHG|CAS|MRET|SRET|LR_W|SC_W):' ebpf/monad-common/src/lib.rs`. Expected: the translator emits none of the four atomics (R-S4's reading); opcodes LD 0x30, XCHG 0x3D, CAS 0x3E, MRET 0x47, SRET 0x48, LR_W 0x49, SC_W 0x4A. Different: write what the translator emits into `$LOG` and re-read Step 75's design with it.
- [ ] **Step 75** [DESIGN][W] ~10m: ADR-135 dated **D17. Atomics translate; user images carry no privileged or untranslated opcode**. In BPF, each atomic's address goes through `translate_address(cpu, addr)` under `cfg!(feature = "ascend-linux")` (the Doom object keeps flat addressing and its count); the reservation keeps the virtual address, the access uses the physical one. In bootctl, the user image and every resident program are scanned word by word; any of 0x3D, 0x3E, 0x47, 0x48, 0x49, 0x4A refuses the load (`UserImageOpcode { name, word, opcode }`). The kernel and bootstub images are not scanned (they need MRET/SRET).
- [ ] **Step 76** [W] ~20m: `scripts/upc-mbc-patch-opcode.py` (SPDX, stdlib only, `--word-of-marker`, `--from 0x30 --to 0x3D`, writes a copy, never in place). It finds the first `LD` emitted for a marker load in a compiled probe and rewrites its opcode byte, keeping dst, src and imm16. Test: `tests/scripts/test_upc_mbc_patch_opcode.py` with a synthetic 4-word image (red, then green): `python3 -m pytest -q tests/scripts/test_upc_mbc_patch_opcode.py`.
- [ ] **Step 77** [TEST][W] ~15m: `user/hostile_atomic.c` (`HA:`): a child reads `*(volatile unsigned *)0x00800400` (above the ceiling: with D6 on a translated access reads 0) into a marker variable, prints `HA: xchg-translated=yes` if the value is 0 and `=no` otherwise, then `HA: done`. Built normally it is a plain LD and passes; the patched copy turns that LD into XCHG. Register it as a userland program, not a gate.
- [ ] **Step 78** [CODE][W] ~15m: `$MAIN`: XCHG and CAS compute `let addr = if cfg!(feature = "ascend-linux") { translate_address(cpu, a) } else { a };`; LR_W and SC_W (ascend-only already) translate the access address and compare the reservation on the virtual one.
- [ ] **Step 79** [BUILD][B][S] ~6m: `gbuild; ebuild; bbuild; keep nv "$(vcount)"; echo "atomics delta=$((nv-offv))"; budget "$nv"; python3 scripts/upc-mbc-patch-opcode.py "$K/hostile_atomic.mbc" --from 0x30 --to 0x3D --out "$S/hostile_atomic_x.mbc"; cp "$K/hostile_atomic.data" "$S/hostile_atomic_x.data" 2>/dev/null; ls "$S"/hostile_atomic_x.*`.
- [ ] **Step 80** [TEST][B][S] ~4m: Boot the patched copy (the scan is not in yet): `(cd "$EBPF" && sudo ip netns exec upc0 env UPC_DEMO_MODE=1 "$BC" boot $UT --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" --userland "$S/hostile_atomic_x.mbc" --triggers 3000000 --instance 111 2>&1) | pl HA`. Expected `HA: xchg-translated=yes`, `HA: done`.
- [ ] **Step 81** [TEST][B] ~5m: Mutant: XCHG's `cfg` translation removed, `ebuild`, Step 80 → `xchg-translated=no` (the access reads fs.img bytes at physical 0x800400; Step 80's `[R]`: `xxd -s 0x400 -l 4 "$K/fs.img"` is nonzero). Restore; `vcount` equals `nv`. Step 80's result and the mutant go in `$LOG`: once the scan lands this proof cannot be booted again from a stock tree.
- [ ] **Step 82** [C] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 73-82: atomics go through translate_address (R-S4)" ebpf/monad-cpu-ebpf/src/main.rs scripts/upc-mbc-patch-opcode.py tests/scripts/test_upc_mbc_patch_opcode.py crates/xv6-mbc/upstream/user/hostile_atomic.c crates/xv6-mbc/adapters/Makefile.mbc-userland docs/adr/ADR-135-upc-kernel-reply-channel.md references/s-stage-2c-log.md`.
- [ ] **Step 83** [TEST][W] ~20m: `cmd/upc-bootctl/tests/opcode_scan.rs` (red first): a user image with one word of each forbidden opcode is refused with `UserImageOpcode` naming it; a clean image passes; a kernel image with MRET/SRET passes; a resident program with XCHG is refused. `(cd cmd/upc-bootctl && cargo test --release --test opcode_scan 2>&1 | tail -3)` → fails to compile or fails, as expected.
- [ ] **Step 84** [CODE][W] ~20m: bootctl: `fn scan_user_image(words: &[u32]) -> Result<(), BootError>` called on the `--userland` image and in `load_resident_program` before `write_image`. Opcode values from `monad_common::op`, not literals.
- [ ] **Step 85** [TEST][B] ~5m: `(cd cmd/upc-bootctl && cargo test --release 2>&1 | grep -E '^test result' | sort | uniq -c)` → all `ok`; `"$BC" validate --kernel "$K/xv6-mbc.mbc" --userland "$S/hostile_atomic_x.mbc" 2>&1 | grep -c UserImageOpcode` → `1` (if `validate` takes no `--userland`, use `boot --dry-run`).
- [ ] **Step 86** [W] ~5m: Harness: build the patched copy into its tmp dir and expect the refusal (no boot): one `pass "opcode scan refuses a user image with XCHG"` line, `fail` otherwise. `bash -n` → syntax-ok.
- [ ] **Step 87** [TEST][B][S] ~15m: **AFTER GATE**: `bbuild; gate atomics-after`. Expected `same=12 diff=0 rc=0 doom=1`, the scan line PASS. No corpus image is refused (the 12 boots prove it).
- [ ] **Step 88** [W][B][S] ~3m: `ledger 'atomics translate' "$offv" "$nv" atomics-after; insrow "$nv"`; ADR-135 Review row R-S4 "Closed 2026-11-12, S-STAGE-2C Phase 4 (translate plus load-time scan)".
- [ ] **Step 89** [C] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 83-89: bootctl refuses user images with privileged or atomic opcodes" cmd/upc-bootctl/src/main.rs cmd/upc-bootctl/tests/opcode_scan.rs scripts/upc-regression.sh docs/adr/ADR-135-upc-kernel-reply-channel.md docs/policy/ebpf-insns-baseline.tsv references/s-stage-2c-log.md docs/battle-plans/progress/S-STAGE-2C-progress.md` (plus any other bootctl source file the scan landed in).
- [ ] **Step 90** [V] ~30s: **PHASE 4 EXIT GATE**: tree clean; `git rev-parse HEAD > "$S/last-green"`.

---

## PHASE 5: EXEC ZEROES .BSS, THE SIDECAR IS FATAL (Steps 91-112)

**Goal**: an exec'd image starts with `.bss` zero (R-NC); a bad or missing userland `.data` sidecar refuses the boot (A-61).
**Prerequisite**: Phase 4.
**Time**: ~2.5 h
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 91** [V][S] ~1m: `keep offv "$(vcount)"`.
- [ ] **Step 92** [R] ~10m: How sidecars are made and what fs.img holds: `grep -nE 'data|NOBITS|PROGBITS' crates/monad-mbc/src/bin/rv32i_to_mbc.rs | head -20; grep -nE 'mkfs|fs.img|\.mbc|\.elf' crates/xv6-mbc/adapters/Makefile.mbc-userland | head; readelf -SW "$K/sh.elf" | grep -E '\.s?bss|\.s?data'; ls "$K"/*.mbc | wc -l; ls "$K"/*.data | wc -l`. Record: `.bss` size per program, whether fs.img packs `.mbc` or `.elf` files, the two counts (24 and 24 at forge time: every program has a sidecar).
- [ ] **Step 93** [DECIDE][DESIGN][W] ~15m: **Where `.bss` gets its zeros.** (A) `rv32i_to_mbc --with-bss` writes each `ALLOC` `NOBITS` section (`.bss`, `.sbss`) as a zero-filled record in the userland `.data` sidecar; exec's existing chunked copy then writes the zeros; no BPF change; the `.mbc` files and fs.img do not change, so no golden moves. (B) PROGRAM_TABLE word 7 carries `bss_len_words` and the exec arm grows a zero pass. RECOMMENDATION: (A), userland only (the kernel zeroes its own `.bss`; the kernel sidecar stays as it is). Override: Step 92 shows a program whose `.bss` is not contiguous with its data region by more than one page, or the staged blobs would overrun the staging area (bootctl prints each blob's size: sum them): then (B). Write ADR-077 a dated note (exec ABI: `.bss` arrives zeroed) and close R-NC in ADR-135.
- [ ] **Step 94** [TEST][W] ~20m: `user/bssprobe.c` (`BS:`), a resident program (registered with sh, ls, cat: PROGRAM_TABLE has 16 slots; count the registered ones first, `grep -c` in bootctl's resident list). Run as `bssprobe` with no argument: fill its own `.bss` (a 256-word static array and malloc's statics) with `0xA5A5A5A5`, fork, child `exec("bssprobe", {"bssprobe", "x", 0})`; with the argument: every word of the array is 0 (`BS: bss-zero=yes`), `malloc(64)` returns a usable pointer and `free` it (`BS: malloc=yes`), then `BS: done`. This is usertests' `bsstest` in probe form.
- [ ] **Step 95** [TEST][W] ~10m: `crates/monad-mbc` unit test (red first): an ELF fixture with `.data` and `.bss` produces a TLV whose last record covers `.bss` with zeros under `--with-bss`, and no such record without it. `(cd crates/monad-mbc && cargo test --release with_bss 2>&1 | tail -3)`.
- [ ] **Step 96** [CODE][W] ~25m: `rv32i_to_mbc`: `--with-bss`; `Makefile.mbc-userland` passes it for user programs only.
- [ ] **Step 97** [BUILD][B][S] ~6m: Before/after identity of everything a golden can see: `sha256sum "$K"/*.mbc "$K"/fs.img > "$S/bss-before.sha"` (from a build before Step 96, taken at Step 92's tree: run this line first, then `gbuild`), then `sha256sum -c "$S/bss-before.sha" 2>&1 | grep -c ': OK'` → every line OK. `bbuild; keep nv "$(vcount)"; [ "$nv" = "$offv" ] && echo bpf-unchanged`.
- [ ] **Step 98** [V] ~30s: all `.mbc` and fs.img unchanged; `bpf-unchanged` (option A). Option B instead: `budget "$nv"`.
- [ ] **Step 99** [TEST][B][S] ~4m: `TRIG=6000000 ub bssprobe | pl BS`. Expected `bss-zero=yes`, `malloc=yes`, `done`.
- [ ] **Step 100** [TEST][B] ~5m: Mutant: `gbuild` without `--with-bss` → `bss-zero=no`. Restore and rebuild.
- [ ] **Step 101** [C] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 91-101: exec'd images start with .bss zeroed (R-NC)" crates/monad-mbc/src/bin/rv32i_to_mbc.rs crates/monad-mbc/tests crates/xv6-mbc/adapters/Makefile.mbc-userland crates/xv6-mbc/upstream/user/bssprobe.c cmd/upc-bootctl/src/main.rs docs/adr/ADR-077-*.md docs/adr/ADR-135-upc-kernel-reply-channel.md references/s-stage-2c-log.md` (name the exact test file Step 95 wrote; bootctl only if bssprobe's registration lives there).
- [ ] **Step 102** [TEST][W] ~15m: `cmd/upc-bootctl/tests/sidecar_fatal.rs` (red first): a truncated `--userland` `.data` refuses the boot before any BPF load (dry run is enough); a resident program with no `.data` refuses; an empty-but-valid TLV is accepted.
- [ ] **Step 103** [CODE][W] ~15m: bootctl `main.rs:948-960`: the warning becomes `bail!` with the path and the error; a missing `--userland` sidecar is `bail!` too. `load_resident_program`: a missing sidecar is `bail!("{name}: no .data sidecar at ...")` instead of `(0, 0)`. Step 92's counts make this safe: every program has one.
- [ ] **Step 104** [TEST][B] ~5m: `(cd cmd/upc-bootctl && cargo test --release 2>&1 | grep -E '^test result' | sort | uniq -c)` → all ok.
- [ ] **Step 105** [C] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 102-105: a bad or missing userland .data sidecar refuses the boot" cmd/upc-bootctl/src/main.rs cmd/upc-bootctl/tests/sidecar_fatal.rs`.
- [ ] **Step 106** [W] ~3m: Harness: `rung_gate UPC_RUNG_SBRK bssprobe 6000000 BS 2`. `bash -n` → syntax-ok.
- [ ] **Step 107** [TEST][B][S] ~15m: **AFTER GATE**: `bbuild; gate bss-after`.
- [ ] **Step 108** [V] ~1m: `same=12 diff=0 rc=0 doom=1`; `bssprobe: 2 checks` PASS.
- [ ] **Step 109** [D] ~10m: A corpus boot refused for a sidecar: the program was missing one after all (Step 92 counted the target, not what bootctl loads); list what bootctl loads from `boot` output and compare. Fix the Makefile, never relax the check.
- [ ] **Step 110** [W][B] ~3m: `ledger 'bss + sidecar' "$offv" "$nv" bss-after`; `forstevie "R-NC closed: .bss arrives zeroed through the .data sidecar (option $([ "$nv" = "$offv" ] && echo A || echo B)); bad or missing userland sidecars now refuse the boot."`.
- [ ] **Step 111** [C] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 106-111: bss gate green" scripts/upc-regression.sh references/s-stage-2c-log.md docs/battle-plans/progress/S-STAGE-2C-progress.md`.
- [ ] **Step 112** [V][STOP] ~30s: **PHASE 5 EXIT GATE, STOP B**: tree clean; `git rev-parse HEAD > "$S/last-green"`.

---

## PHASE 6: FORK'S TAGGED CURSOR, SLOT RESERVATION, I9 TRIPWIRE (Steps 113-128)

**Goal**: fork's first entry is keyed on a tagged cursor, as exec and open are (R-F3); a child slot is reserved from the first copy tick (R-F6); a build that turns on a timer switch under `ascend-linux` fails to compile (I9).
**Prerequisite**: Stop B.
**Time**: ~2.5 h
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 113** [V][S] ~1m: `keep offv "$(vcount)"`.
- [ ] **Step 114** [DESIGN][W] ~20m: ADR-135 dated **D18**: (1) fork's first entry happens when a3 does not carry `FORK_TAG | child_slot` (a new const beside `EXEC_ARGV_MAGIC`), not when child pde 0 is absent; the first entry writes the tag and zeroes a2; later ticks trust a2 only under the tag. (2) The chosen slot is marked reserved in its PROC_TABLE row at first entry (a bit in the existing state word, named in `phase12.rs`); slot choice skips reserved rows; finalize or the parent's death clears it. (3) The timer block's `cfg!(not(feature = "ascend-linux"))` becomes `const TIMER_SWITCH: bool = !cfg!(feature = "ascend-linux");` and `const _: () = assert!(!(cfg!(feature = "ascend-linux") && TIMER_SWITCH), "ADR-135 I9: ascend fork assumes no timer switch; add slot reservation and the tagged cursor review first");`. RECOMMENDATION as written. Override: none.
- [ ] **Step 115** [TEST][W] ~25m: `user/hostile_fork.c` (`HF:`): (a) a child stores `PTE_PRESENT` into pde 0 of a free slot's pgd at `0xF00000 + slot*0x1000`, then its parent forks into that slot; the grandchild checks a 64-word canary the parent wrote at 0x14000 (`HF: copy-complete=yes`) and reads the planted word back (`HF: pde-plant-dropped=yes` with D6 on; with D6 off the probe prints `HF: pde-plant-dropped=no` and that line is expected only in Step 120's experiment); (b) an `ecall` fork with a2 = 0x3F000 and a3 = 0 by inline asm: the child's canary is complete (`HF: a2-ignored=yes`); `HF: done`.
- [ ] **Step 116** [CODE][W] ~30m: `$MAIN` fork arm (`:2350-2410`), `phase12.rs` (tag, reserved bit), the timer block (`:777`), the assertion. Every touched line in a shared path carries `cfg!(feature = "ascend-linux")` so Doom stays 758,199.
- [ ] **Step 117** [BUILD][B][S] ~6m: `gbuild; ebuild; bbuild; keep nv "$(vcount)"; echo "fork delta=$((nv-offv))"; budget "$nv"; dbuild`.
- [ ] **Step 118** [V] ~30s: `BUDGET-OK`; `dbuild` `Finished`. FAIL: the reservation is the expensive half; land the tag and tripwire, `[STUCK]` the reservation with its number (preemption, which needs it, is out of scope).
- [ ] **Step 119** [TEST][B][S] ~5m: `TRIG=4000000 ub hostile_fork | pl HF`. Expected `copy-complete=yes`, `pde-plant-dropped=yes`, `a2-ignored=yes`, `done`. `forkloop` and `gate_nway` are read from the gate at Step 124.
- [ ] **Step 120** [TEST][B] ~15m: The fix against the hole, with D6 off for the experiment only: `flip UPC_D6_MISS_FAULT true false`; boot `hostile_fork` → `pde-plant-dropped=no` (the plant landed), `copy-complete=yes`; mutant (first entry keyed on pde 0 again) → `copy-complete=no`; restore the fork code, `flip UPC_D6_MISS_FAULT false true`; `vcount` equals `nv`. Results to `$LOG`.
- [ ] **Step 121** [TEST][B] ~3m: Tripwire bites: set `TIMER_SWITCH` to `true` by hand, `ebuild 2>&1 | grep -c 'ADR-135 I9'` → at least `1`; restore; `ebuild` Finished.
- [ ] **Step 122** [W] ~3m: Harness: `rung_gate UPC_D6_MISS_FAULT hostile_fork 4000000 HF 3` (`copy-complete`, `pde-plant-dropped`, `a2-ignored`). `bash -n` → syntax-ok.
- [ ] **Step 123** [C] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 113-123: fork keyed on a tagged cursor, slot reserved, I9 tripwire" ebpf/monad-cpu-ebpf/src/main.rs ebpf/monad-cpu-ebpf/src/phase12.rs crates/xv6-mbc/upstream/user/hostile_fork.c crates/xv6-mbc/adapters/Makefile.mbc-userland scripts/upc-regression.sh docs/adr/ADR-135-upc-kernel-reply-channel.md references/s-stage-2c-log.md`.
- [ ] **Step 124** [TEST][B][S] ~15m: **AFTER GATE**: `gate fork-after`.
- [ ] **Step 125** [V] ~1m: `same=12 diff=0 rc=0 doom=1`; `forkloop`, `gate_nway`, `gate2`, every `ut_*` and `hostile_fork: 3 checks` PASS.
- [ ] **Step 126** [D] ~20m: `forkloop` red: a reused slot keeps its reserved bit (finalize did not clear it) or the tag survives into the next fork (a3 is caller-saved: the stub must not leave it set; clear it at finalize). `gate_nway` red: slot choice now skips a slot it used to take; print the reserved bits after each fork.
- [ ] **Step 127** [W][B][S] ~3m: ADR-135 Review rows R-F3, R-F6, I9 "Closed 2026-11-12, S-STAGE-2C Phase 6". `ledger 'fork tag + reserve' "$offv" "$nv" fork-after; insrow "$nv"`.
- [ ] **Step 128** [C][V] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 124-128: fork gate green" docs/adr/ADR-135-upc-kernel-reply-channel.md docs/policy/ebpf-insns-baseline.tsv references/s-stage-2c-log.md docs/battle-plans/progress/S-STAGE-2C-progress.md`. **PHASE 6 EXIT GATE**: tree clean.

---

## PHASE 7: T-F3 INTERIM, A WRITE TO A FILE FD IN BPF IS REFUSED (Steps 129-138)

**Goal**: until rung 6 makes `open(O_RDWR)` on a file kernel-side (Phase 12), BPF `write` on an inode fd answers -1 instead of printing the bytes to the TTY and returning n.
**Prerequisite**: Phase 6.
**Time**: ~1.25 h
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 129** [V][S] ~1m: `keep offv "$(vcount)"`.
- [ ] **Step 130** [R] ~5m: The BPF write arm (`syscall_nr == 16`, `main.rs:2232`) and how it tells a console fd from an inode fd (`FD_TABLE` kind, `fd_inode_get`). Write the test it needs into `$LOG`.
- [ ] **Step 131** [TEST][W] ~10m: `fdprobe` gains: `open("README", O_RDWR)` (BPF, since O_RDWR alone is not in `OPEN_KERNEL_FLAGS`), `write(fd, "zz", 2)` → -1 (`FD: rdwr-file-write-refused=yes`), README's first bytes unchanged on read-back (`FD: readme-intact=yes`); `open("console", O_RDWR)` then `write` still prints (an existing line). FD count 49 → 51 in the harness.
- [ ] **Step 132** [CODE][W] ~10m: `$MAIN` write arm: an inode-kind fd below 16 answers -1 before any TTY store. Ascend-only arm.
- [ ] **Step 133** [BUILD][B][S] ~6m: `gbuild; ebuild; keep nv "$(vcount)"; budget "$nv"; TRIG=5000000 ub fdprobe | pl FD > "$S/fd2.txt"; grep -c '=yes$' "$S/fd2.txt"; grep -c '=no$' "$S/fd2.txt"`. Expected `BUDGET-OK`, `51`, `0`.
- [ ] **Step 134** [TEST][B] ~5m: Mutant: the refusal removed → `rdwr-file-write-refused=no`. Restore; `vcount` equals `nv`.
- [ ] **Step 135** [TEST][B][S] ~15m: **AFTER GATE**: `gate rdwr-after` → `same=12 diff=0 rc=0 doom=1`, `fdprobe: 51 checks` PASS.
- [ ] **Step 136** [W] ~3m: ADR-135 D10 and Review row T-F3: "Interim 2026-11-12: BPF refuses a write to a file fd; the write lands in the kernel at rung 6 (Phase 12)". `ledger 'T-F3 interim' "$offv" "$nv" rdwr-after; insrow "$nv"`.
- [ ] **Step 137** [C] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 129-137: a write to a file fd in BPF is refused, not printed (T-F3 interim)" ebpf/monad-cpu-ebpf/src/main.rs crates/xv6-mbc/upstream/user/fdprobe.c scripts/upc-regression.sh docs/adr/ADR-135-upc-kernel-reply-channel.md docs/policy/ebpf-insns-baseline.tsv references/s-stage-2c-log.md docs/battle-plans/progress/S-STAGE-2C-progress.md`.
- [ ] **Step 138** [V][STOP] ~30s: **PHASE 7 EXIT GATE, STOP C**: tree clean; `git rev-parse HEAD > "$S/last-green"`.

---

## PHASE 8: THE PROBES ADR-135'S REVIEW OFFERED (Steps 139-160)

**Goal**: every A-62 probe is built and gated: unlink with data, link's nlink, pause's tick count, a file past one block, `hostile_ptr` for unlink/link/chdir, `upc-rung0-assert.awk` in the harness. (Forged replies with D6 on: done in Phase 2.)
**Prerequisite**: Stop C.
**Time**: ~2.5 h
**Agent**: Coordinator (Sonnet writes the probe sources, Opus boots)
**Model**: Sonnet, then Opus

A probe that finds a real kernel bug is a finding, not a test bug: fix it here only if the fix is small and has its own probe line; otherwise `[STUCK]` the line with the finding, `forstevie`, and keep the probe printing `=no` out of the harness count.

- [ ] **Step 139** [V][S] ~1m: `keep offv "$(vcount)"`.
- [ ] **Step 140** [TEST][W] ~10m: `rmprobe`: create `rmf` with `O_CREATE|O_WRONLY`, write 2,500 bytes (three blocks), close, `unlink("rmf")` → 0 (`RM: file-with-data-unlinked=yes`), `open("rmf", 0)` → -1 (`RM: gone=yes`); create `rmg`, write 2,500 different bytes, read back exactly (`RM: blocks-reused-clean=yes`). RM 8 → 11.
- [ ] **Step 141** [TEST][W] ~10m: `lnprobe`: after `link("lnsrc", "lndst")`, `fstat` on either gives nlink 2 (`LN: nlink-two=yes`); `unlink("lnsrc")` then `fstat("lndst")` nlink 1 (`LN: nlink-one=yes`) and its bytes read back (`LN: bytes-after-unlink=yes`). LN 9 → 12.
- [ ] **Step 142** [TEST][W] ~10m: `sleepprobe`: `t0 = uptime(); pause(50); t1 = uptime();` `t1 - t0 >= 50` (`SL: pause-ticks=yes`); print `SL: late-by=<t1-t0-50>` as a note (decimal digits only, lower case, so `pl` keeps it). The harness block for sleepprobe (`upc-regression.sh:217-222`) gains the one line.
- [ ] **Step 143** [TEST][W] ~15m: `user/bigprobe.c` (`BG:`): write 14,000 bytes in 300-byte chunks with a position pattern to `big` (crosses `NDIRECT * BSIZE` = 12,288 into the single-indirect block), `fstat` size 14,000 (`BG: size=yes`), close, reopen read-only (BPF open, BPF `fs_walk` reads the indirect block) and compare every byte (`BG: read-back=yes`), reopen with `O_WRONLY|O_TRUNC` and close: size 0 (`BG: trunc=yes`). Three `=yes`.
- [ ] **Step 144** [TEST][W] ~10m: `hostile_ptr`: `unlink((char*)0x00800000)`, `link((char*)0x00800000, "x")`, `link("README", (char*)0x00800000)`, `chdir((char*)0x00800000)`, each -1 with nothing created (`HU: unlink-bad=yes`, `HU: link-old-bad=yes`, `HU: link-new-bad=yes`, `HU: chdir-bad=yes`). HU 5 → 9.
- [ ] **Step 145** [R] ~10m: The asserter's input contract (`scripts/upc-rung0-assert.awk` header) and how S-STAGE-2B fed it: a kernel built with `-DUPC_STAGE2_PROBE` prints `UT:` lines as kernel `printf` (char plus NUL), decoded by `ugl` (`docs/battle-plans/EVOLUTION-1-STAGE-2-battle-plan.md:227`). Note the make variable that passes the define (`grep -n CFLAGS crates/xv6-mbc/adapters/Makefile.mbc`).
- [ ] **Step 146** [W] ~20m: Harness block "rung 0 asserter": build the probe kernel into the harness's own tmp dir (never over `$K/xv6-mbc.mbc`), boot `getpidfork` and `forkloop` on it, decode as `ugl` does, pipe into the awk, require `RUNG0-ASSERT pass`. One mutant check in the same block: feed the awk a copy with one `pid=` field changed and require `fail`. `bash -n` → syntax-ok.
- [ ] **Step 147** [W] ~5m: Register `bigprobe` (gates list); harness lines `rung_gate UPC_RUNG_FD bigprobe 8000000 BG 3`, RM 11, LN 12, HU 9.
- [ ] **Step 148** [BUILD][B][S] ~4m: `gbuild; [ "$(vcount)" = "$offv" ] && echo bpf-unchanged`.
- [ ] **Step 149** [TEST][B][S] ~12m: For each of `rmprobe RM`, `lnprobe LN`, `sleepprobe SL`, `bigprobe BG`, `hostile_ptr HU`: `TRIG=8000000 ub <p> | pl <X> > "$S/p8-<p>.txt"; grep -c '=no$' "$S/p8-<p>.txt"`. Expected `0` each.
- [ ] **Step 150** [D] ~25m: A `=no`: read the line against the kernel path (`ufs.c` `itrunc`/`bfree` for RM, `sys_link` `nlink` for LN, `ticks` for SL, `bmap` indirect for BG, `argstr` for HU). Small fix with the probe line proving it: land it in this phase. Otherwise the paragraph above the steps.
- [ ] **Step 151** [C] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 139-151: probes for unlink with data, nlink, pause ticks, a file past one block, bad pointers" crates/xv6-mbc/upstream/user/rmprobe.c crates/xv6-mbc/upstream/user/lnprobe.c crates/xv6-mbc/upstream/user/sleepprobe.c crates/xv6-mbc/upstream/user/bigprobe.c crates/xv6-mbc/upstream/user/hostile_ptr.c crates/xv6-mbc/adapters/Makefile.mbc-userland scripts/upc-regression.sh references/s-stage-2c-log.md` (plus any kernel file Step 150 fixed).
- [ ] **Step 152** [TEST][B][S] ~10m: Run only the asserter block by hand (copy it to `$S/assert.sh` with the harness's variables): `RUNG0-ASSERT pass N` with N > 0, and the mutant line `fail`.
- [ ] **Step 153** [D] ~15m: `fail` on the real input: the awk predates something (D12's notes are covered; D13's exit a7=2 is covered). Read the reason it prints; the asserter is the spec of ADR-109 I1-I8, so a real violation is a finding: `[STUCK]`, `forstevie`. A decode problem (NULs, split lines) is fixed in the block.
- [ ] **Step 154** [TEST][B][S] ~15m: **AFTER GATE**: `gate probes-after`.
- [ ] **Step 155** [V] ~1m: `same=12 diff=0 rc=0 doom=1`; each new or extended probe line PASS; the asserter line PASS.
- [ ] **Step 156** [W] ~10m: `$LOG` §Attack surface: one row per probe: what it attacks, what bounds it, its line. ADR-135's "not booted" items that these probes now boot get a dated "booted" line each (unlink with data, nlink read back, pause tick count, file past one block, bad pointers for unlink/link/chdir, the asserter against a probe kernel).
- [ ] **Step 157** [W][B] ~2m: `ledger 'probes' "$offv" "$offv" probes-after` (no BPF change).
- [ ] **Step 158** [C] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 152-158: rung 0 asserter in the harness; probes gate green" scripts/upc-regression.sh docs/adr/ADR-135-upc-kernel-reply-channel.md references/s-stage-2c-log.md docs/battle-plans/progress/S-STAGE-2C-progress.md`.
- [ ] **Step 159** [W] ~2m: `forstevie "Stop D: every A-62 probe gated; $(grep -c '^\[STUCK\]' "$PROG") stuck lines. Hardening done before any rung: D6, priv-3 returns, bounded paths, slot 0, atomics + scan, .bss, sidecars, fork tag/reserve/I9, T-F3 interim."`; commit with the progress file.
- [ ] **Step 160** [V][STOP] ~30s: **PHASE 8 EXIT GATE, STOP D**: tree clean; `git rev-parse HEAD > "$S/last-green"`.

---

## PHASE 9: RUNG 3, CONSOLE WRITE IN THE KERNEL (Steps 161-186)

**Goal**: `write` on a console fd is served by the owned kernel (devsw, `uconsole`, `console-mmio`) and the BPF console byte loop is compiled out under the rung's const.
**Prerequisite**: Stop D. Rung 0 (rows exist).
**Time**: ~4 h
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 161** [V][S] ~16m: **BEFORE GATE**: `before rung3` → `same=12 diff=0 rc=0 doom=1`; `offv` kept.
- [ ] **Step 162** [R][S] ~10m: Today's cost, for the rung's cycle record: for `ls`, `cat BIGFILE` and `echo peace and love`, the MBC instruction count and trigger use bootctl prints at the end of a boot (`boot $'ls\n' | grep -aE 'insn|trigger' | tail -3`), and how many `write` ecalls each makes (xv6 user `printf` writes one byte per call; `cat` writes up to 512). Into `$LOG` §Run cost, rung 3 before.
- [ ] **Step 163** [DESIGN][W] ~20m: ADR-135 dated **D19. Rung 3**: alias `16 if cfg!(feature = "ascend-linux") && UPC_RUNG3 && cpu.regs[8] < 16 && fd is console-kind => 11` (the inode branch from Phase 7 stays in BPF until rung 6). Kernel `sys_write` on a slot row with fd below 16 writes through `consolewrite(1, addr, n)`; only BPF routes such an fd there. `console-mmio.c` gains `uartputc_byte(int c)`, a one-byte store to 0xC001, used by `consolewrite`'s path and not by kernel `printf` (kernel boot lines keep their char-plus-NUL bytes, which are in the goldens). The BPF console loop is guarded `!UPC_RUNG3 &&`, so the on object drops it. The SRET `S` stays quiet for U-mode returns (D14, under `UPC_RUNG_FD`). Every write is now one trap, so the rung's cost is per call, not per byte; Step 179 decides if the corpus still fits its trigger budgets.
- [ ] **Step 164** [TEST][W] ~20m: `user/conprobe.c` (`CW:`), registered as a gate, and its harness line `rung_gate UPC_RUNG3 conprobe 4000000 CW 4`: `write(1, "abc\n", 4)` returns 4 (`CW: ret-four=yes`); one 600-byte `write` returns 600 (`CW: long-write=yes`); `write(1, (char*)0x00900000, 4)` → -1 (`CW: badptr=yes`); `write(1, buf, 0)` → 0 (`CW: zero=yes`); `CW: done`. The harness also greps the TTY for the literal `abc` once.
- [ ] **Step 165** [CODE][W] ~25m: Kernel: `console-mmio.c` `uartputc_byte`; `uconsole.c` `consolewrite` writes through it (the stock body stays, the store changes); `usysfile.c` `sys_write`: when `upc_slot_active()` and the fd is below 16, `return consolewrite(1, p, n);` after `argaddr`/`argint`, with -1 on a bad pointer from `either_copyin`.
- [ ] **Step 166** [CODE][W] ~25m: BPF: `const UPC_RUNG3: bool = false;` beside the others; the alias row (with `cfg!(feature = "ascend-linux")`); the console byte loop of the write arm guarded `!UPC_RUNG3 &&` so the const compiles it out; `surface.rs` and `XV6_SYSCALLS` unchanged in length (16 is already routed for fds at or above 16; the row's comment gains console fds); `parity_surface` passes.
- [ ] **Step 167** [BUILD][B][S] ~8m: `kbuild; gbuild; ebuild; bbuild; [ "$(vcount)" = "$offv" ] && echo off-equal; sweep > "$S/r3-off-sweep8.txt" 2>&1; grep -c '^SAME' "$S/r3-off-sweep8.txt"`. Expected `off-equal`, `8`.
- [ ] **Step 168** [D] ~10m: Not `off-equal`: a guard does not fold (the `!UPC_RUNG3` sits behind a runtime test; move it first in the condition, as S-STAGE-2B Phase 11 learned). Not `8`: the kernel change moved kernel boot bytes (`uartputc_byte` leaked into `printf`): diff `ls`.
- [ ] **Step 169** [C] ~1m: Const-off point: `cmt "[PLAN S-STAGE-2C] Steps 161-169: rung 3 wired, const off" crates/xv6-mbc/adapters/console-mmio.c crates/xv6-mbc/adapters/uconsole.c crates/xv6-mbc/adapters/usysfile.c crates/xv6-mbc/upstream/user/conprobe.c crates/xv6-mbc/adapters/Makefile.mbc-userland scripts/upc-regression.sh ebpf/monad-cpu-ebpf/src/main.rs ebpf/monad-cpu-ebpf/surface.rs cmd/upc-bootctl/src/main.rs docs/adr/ADR-135-upc-kernel-reply-channel.md references/s-stage-2c-log.md`.
- [ ] **Step 170** [CODE][B][S] ~5m: `flip UPC_RUNG3 false true; keep nv "$(vcount)"; echo "rung3=$nv delta=$((nv-offv))"; budget "$nv"`.
- [ ] **Step 171** [V] ~30s: `BUDGET-OK`; a negative delta is the freed loop (the skeleton's "freed insns"). Positive is allowed while the ladder is net negative.
- [ ] **Step 172** [TEST][B][S] ~8m: `TRIG=4000000 ub conprobe | pl CW > "$S/cw.txt"; grep -c '=yes$' "$S/cw.txt"; grep -c '=no$' "$S/cw.txt"; sweep > "$S/r3-sweep8.txt" 2>&1; grep -c '^SAME' "$S/r3-sweep8.txt"`.
- [ ] **Step 173** [V] ~30s: `4`, `0`, `8`. Else Step 174.
- [ ] **Step 174** [D] ~20m: NULs between characters: the word store is still on the path. Missing tail of output: the boot ran out of triggers before the corpus finished (Step 179). Extra `S`: D14 guard. `tty_r` or `forks` moved: a write that now traps changed scheduling order; read the stats lines and the `full` capture side by side.
- [ ] **Step 175** [D] ~20m: Second attempt only: boot the failing spec with `CTRIG=8000000`. If it passes there, the cause is the budget: go to Step 179 with the numbers. If not, `[STUCK]`, flip off (A7).
- [ ] **Step 176** [TEST][B] ~10m: Mutants: `consolewrite` back to the word store → `sweep` shows `DIFF` (proves the goldens see it); the alias row removed with the const on → `conprobe` still passes through BPF but the loop is compiled out, so `CW: long-write=no` (proves the route). Restore; `vcount` equals `nv`.
- [ ] **Step 177** [TEST][B][S] ~16m: **AFTER GATE**: `gate rung3-after`.
- [ ] **Step 178** [V] ~1m: `same=12 diff=0 rc=0 doom=1`; `conprobe: 4 checks` PASS.
- [ ] **Step 179** [DECIDE] ~10m: **Trigger budgets.** Compare Step 162 with the same readings now. RECOMMENDATION: if all 12 corpus boots finish within the harness's existing `--triggers`, proceed, and record the per-call cycle cost. Override: any boot needs more: flip the const off (A7), Stop D stands, and `forstevie` the per-call cost and the budgets the rung would need. The skeleton says new budgets need acceptance; that is Stevie's.
- [ ] **Step 180** [B][S] ~3m: `ledger 'rung 3 console write' "$offv" "$nv" rung3-after; insrow "$nv"; bash scripts/check-ebpf-insns.sh 2>&1 | tail -1`.
- [ ] **Step 181** [W] ~10m: ADR-078 §5: row 16 notes console fds under `UPC_RUNG3`. `docs/battle-plans/EVOLUTION-1-STAGE-2-skeleton.md` rung 3: a dated status line (gate, delta, cycle cost). `$LOG` §Run cost rung 3 after.
- [ ] **Step 182** [C] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 170-182: rung 3 on: console write in the owned kernel" ebpf/monad-cpu-ebpf/src/main.rs docs/adr/ADR-078-gate-d-fs-reader-and-exec-argv-abi.md docs/battle-plans/EVOLUTION-1-STAGE-2-skeleton.md docs/policy/ebpf-insns-baseline.tsv references/s-stage-2c-log.md docs/battle-plans/progress/S-STAGE-2C-progress.md`.
- [ ] **Step 183** [R] ~5m: `grep -c '^const UPC_RUNG3: bool = true;' "$MAIN"` → `1`; `git log -1 --format=%G?` → `G` or a `$PROG` line.
- [ ] **Step 184** [W] ~2m: `forstevie "Rung 3 on at $nv (delta $((nv-offv))); console write is kernel-side; per-call cost in the log."`.
- [ ] **Step 185** [C] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 183-185: rung 3 recorded" docs/battle-plans/progress/S-STAGE-2C-progress.md`.
- [ ] **Step 186** [V][STOP] ~30s: **PHASE 9 EXIT GATE, STOP E**: tree clean; `git rev-parse HEAD > "$S/last-green"`.

---

## PHASE 10: RUNG 4, CONSOLE READ IN THE KERNEL (Steps 187-206)

**Goal**: `read` on a console fd is served by the kernel, polling the KBD ring through `uartgetc`; an empty ring takes the retry hand-off (ADR-135 D7), never `sleep`.
**Prerequisite**: Stop E.
**Time**: ~3 h
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 187** [V][S] ~16m: **BEFORE GATE**: `before rung4`.
- [ ] **Step 188** [R] ~15m: The BPF read arm's console branch (KBD ring, the drain the goldens' `--input` feeds, the BPF `TTY_READS` stat that `stats_of` reads as `tty_r`); `uconsole.c` `consoleread`/`consoleintr`; `console-mmio.c` `uartgetc` (KBD at 0xFFFF); whether BPF echoes typed input (the goldens say what is printed).
- [ ] **Step 189** [DESIGN][W] ~25m: ADR-135 dated **D20. Rung 4**: alias `5 if ... UPC_RUNG4 && fd < 16 && console-kind => 11`. Kernel `sys_read` on a slot row, fd below 16: pull bytes with `uartgetc()` into the stock line buffer by calling `consoleintr` per byte (no interrupt: the read path polls, a departure from the skeleton's "KBD ring as interrupt source", recorded); return a line as stock `consoleread` does; an empty ring with no complete line releases the lock and `upc_retry()`s (D7), keeping a0. No echo unless BPF echoed (Step 188 decides; the goldens rule). Stats parity: BPF bumps `TTY_READS` when it routes a console read, and not on a retry re-run (the retry sentinel tells it), so `tty_r` keeps its meaning. The BPF console read branch is guarded `!UPC_RUNG4 &&`.
- [ ] **Step 190** [TEST][W] ~20m: `user/crprobe.c` (`CR:`): `read(0, buf, 64)` returns `hello\n` (6) (`CR: line=yes`); a second line `again\n` arrives in a second read (`CR: second-line=yes`); `read(0, (char*)0x00900000, 8)` → -1 (`CR: badptr=yes`); `CR: done`. Harness block (not `rung_gate`: it needs input): boot with `--input $'hello\nagain\n'` under `rung_on UPC_RUNG4`, want 3 `=yes`, no `=no`, `done`.
- [ ] **Step 191** [CODE][W] ~30m: Kernel `usysfile.c` `sys_read` branch; `uconsole.c` polling read using `consoleintr` per byte; retry on empty.
- [ ] **Step 192** [CODE][W] ~20m: BPF: `const UPC_RUNG4: bool = false;`, alias row, `!UPC_RUNG4 &&` on the console read branch, `TTY_READS` bump at routing (not on a retry), surface and parity.
- [ ] **Step 193** [BUILD][B][S] ~8m: `kbuild; gbuild; ebuild; bbuild; [ "$(vcount)" = "$offv" ] && echo off-equal; sweep > "$S/r4-off-sweep8.txt" 2>&1; grep -c '^SAME' "$S/r4-off-sweep8.txt"` → `off-equal`, `8`.
- [ ] **Step 194** [C] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 187-194: rung 4 wired, const off" crates/xv6-mbc/adapters/usysfile.c crates/xv6-mbc/adapters/uconsole.c crates/xv6-mbc/upstream/user/crprobe.c crates/xv6-mbc/adapters/Makefile.mbc-userland scripts/upc-regression.sh ebpf/monad-cpu-ebpf/src/main.rs ebpf/monad-cpu-ebpf/surface.rs cmd/upc-bootctl/src/main.rs docs/adr/ADR-135-upc-kernel-reply-channel.md references/s-stage-2c-log.md`.
- [ ] **Step 195** [CODE][B][S] ~5m: `flip UPC_RUNG4 false true; keep nv "$(vcount)"; echo "rung4=$nv delta=$((nv-offv))"; budget "$nv"`.
- [ ] **Step 196** [TEST][B][S] ~15m: `crprobe` by the harness block's command; `sweep12 > "$S/r4-sweep12.txt" 2>&1; grep -c '^SAME' "$S/r4-sweep12.txt"` (multi-line and blank-line inputs live in the last four).
- [ ] **Step 197** [V] ~30s: `BUDGET-OK`; crprobe 3 `=yes`, 0 `=no`; `12`. Else Step 198.
- [ ] **Step 198** [D] ~20m: `tty_r` differs only: the bump fires on retries or not at all; print the stat after each boot. A hang at the prompt: the retry never yields while the ring is empty (`num_processes` guard) or the ring is drained by BPF before the kernel sees it. Lost characters: two readers of the ring.
- [ ] **Step 199** [D] ~20m: Second attempt only, then `[STUCK]`, flip off (A7), Stop E stands.
- [ ] **Step 200** [TEST][B][S] ~5m: The skeleton's live check: `printf 'echo hi\n' | sudo ip netns exec upc0 env UPC_DEMO_MODE=1 "$BC" console $UT --stdin --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" --userland "$K/init.mbc" --triggers 4000000 --instance 111 2>&1 | grep -ac 'hi'` → at least `1` (check `console --help` for its flags first; adjust and record).
- [ ] **Step 201** [TEST][B] ~10m: Mutant: the retry replaced by `return 0` on an empty ring → `CR: second-line=no` or a golden `DIFF`. Restore; `vcount` equals `nv`.
- [ ] **Step 202** [TEST][B][S] ~16m: **AFTER GATE**: `gate rung4-after` → `same=12 diff=0 rc=0 doom=1`, the crprobe block PASS.
- [ ] **Step 203** [B][S] ~3m: `ledger 'rung 4 console read' "$offv" "$nv" rung4-after; insrow "$nv"; bash scripts/check-ebpf-insns.sh 2>&1 | tail -1`.
- [ ] **Step 204** [W] ~10m: ADR-078 §5 row 5 note; skeleton rung 4 dated status (including the polling departure); `$LOG` run cost.
- [ ] **Step 205** [C] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 195-205: rung 4 on: console read in the owned kernel" ebpf/monad-cpu-ebpf/src/main.rs docs/adr/ADR-078-gate-d-fs-reader-and-exec-argv-abi.md docs/battle-plans/EVOLUTION-1-STAGE-2-skeleton.md docs/policy/ebpf-insns-baseline.tsv references/s-stage-2c-log.md docs/battle-plans/progress/S-STAGE-2C-progress.md`.
- [ ] **Step 206** [V][STOP] ~30s: **PHASE 10 EXIT GATE, STOP F**: tree clean; `git rev-parse HEAD > "$S/last-green"`; `forstevie "Rung 4 on at $nv."` committed with the next commit.

---

## PHASE 11: RUNGS 5+6 DESIGN, THE FS_WALK HAND-OFF (Steps 207-214)

**Goal**: one written design for moving open, fstat, dup, close and file read/write into the kernel and retiring `fs_walk` and `FD_INODE_MAP`, reviewed before any code.
**Prerequisite**: Stop F.
**Time**: ~1.5 h
**Agent**: Coordinator; one review agent
**Model**: Opus

- [ ] **Step 207** [R] ~15m: `grep -nE 'fd_inode_|FD_INODE_MAP|fs_walk|FD_TABLE' "$MAIN" | wc -l; grep -nE 'fd_inode_|fs_walk|FD_TABLE' "$MAIN" | awk -F: '{print $1}' | tr '\n' ' '`: every BPF site that reads the BPF fd table or walks the FS (open 15, read 5 inode branch, fstat 8, dup 10, close 21, write 16 inode refusal, fork's `FD_TABLE` copy, exit's close). Into `$LOG`.
- [ ] **Step 208** [R] ~15m: Kernel side: `usysfile.c` `sys_open`/`sys_fstat`/`sys_dup`/`sys_close`/`sys_read`/`sys_write` on slot rows, `fdalloc`/`fduser` (D10's +16), init's (slot 0) `open("console")`, `dup` x2, and the absence of a T_DEVICE inode in fs.img (`ls` golden lists none).
- [ ] **Step 209** [DESIGN][W] ~30m: ADR-135 dated **D21. Rungs 5+6** (one const `UPC_RUNG56`, one gate, the skeleton's first kickoff option):
  1. With the const on, 15, 5, 8, 10, 21 and 16 alias to 11 for every fd; each BPF arm or branch that reads `FD_TABLE`/`FD_INODE_MAP` or calls `fs_walk` is guarded `!UPC_RUNG56 &&`, and the fork copy of `FD_TABLE` too, so the on object carries none of them.
  2. Kernel fds are the `ofile` index: `fduser()` becomes the identity under the const (D10's +16 offset ends). Probe lines that say "at or above 16" change in Step 216.
  3. **[DECIDE] the console.** Kernel `sys_open` maps the names `console` and `/console` to the console device without a disk inode, as BPF does today. RECOMMENDATION: this (fs.img and the `ls` golden stay as they are). Override: none here; on-disk device nodes and `mknod` are a later ADR, written in Step 212 as a parked one-liner in `$PROG` For Stevie.
  4. Slot 0 (init) opens the console through the kernel: its row is the real `userinit` process, so `ofile` is its own.
  5. Fork: the fork note (24) already `filedup`s `ofile`; exit already closes it (D13). Nothing else moves.
  6. `open(O_RDWR)` on a regular file is a kernel file: T-F3 closes; Phase 7's BPF refusal goes with the compiled-out arm.
  7. Exec stays a BPF resident-program lookup (departure, GROUND TRUTH); it reads no fd.
  8. After this rung `dup` can land on 0-2, which is what ush pipelines and redirection wait on; the shell grammar is not this plan's (Step 241).
- [ ] **Step 210** [R] ~25m: Review agent, read-only, given D21 and the Step 207/208 lists. Ask: does any BPF path still read `FD_TABLE` with the const on (TTY drain, exec, `cwd_mask`); can a file named `console` on disk be shadowed or used to confuse the name match; what does slot 0 hold in `ofile` before init's first open; does D7's retry keep a0 for read/write on files; which goldens can move and why. The coordinator waits.
- [ ] **Step 211** [W] ~15m: Dispositions into ADR-135 (a third review table, dated) and `$LOG`.
- [ ] **Step 212** [W] ~3m: `forstevie "Rungs 5+6: the console is matched by name in the kernel, no on-disk device node; mknod and T_DEVICE inodes are a later parked ADR if you want them."`.
- [ ] **Step 213** [C] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 207-213: rungs 5+6 design (ADR-135 D21) and review" docs/adr/ADR-135-upc-kernel-reply-channel.md references/s-stage-2c-log.md docs/battle-plans/progress/S-STAGE-2C-progress.md`.
- [ ] **Step 214** [V] ~30s: **PHASE 11 EXIT GATE**: no review finding left without a disposition; tree clean.

---

## PHASE 12: RUNGS 5+6, THE KERNEL FD TABLE IS THE ONLY ONE (Steps 215-244)

**Goal**: `UPC_RUNG56` on: every fd call is the kernel's; `fs_walk` and `FD_INODE_MAP` are out of the ascend object; the four read-path goldens hold; T-F3 closed.
**Prerequisite**: Phase 11.
**Time**: ~7 h
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 215** [V][S] ~16m: **BEFORE GATE**: `before rung56`.
- [ ] **Step 216** [TEST][W] ~30m: Probe changes, each recorded in `$LOG` with old and new text (a probe change with a reason, not a recapture): `fdprobe`, `chdirprobe`, `pipeprobe` lines that assert "fd at or above 16" assert "lowest free fd" instead. New `user/rwprobe.c` (`RW:`): `open("README", O_RDWR)`, write 2 bytes at offset 0 → 2 (`RW: write-lands=yes`), a second fd reads them back (`RW: read-back=yes`), the original 2 bytes written back (`RW: restored=yes`); `close(0); dup(fd)` → 0 (`RW: dup-to-zero=yes`); `fstat` on the console fd answers without error (`RW: console-fstat=yes`); `RW: done`. Harness `rung_gate UPC_RUNG56 rwprobe 6000000 RW 5`.
- [ ] **Step 217** [CODE][W] ~45m: Kernel: `fduser()`/`argfd`/`fdalloc` without the +16 under the rung (a kernel-side `#define` set by the build, mirrored to BPF's const by `parity_slot_tables`), `sys_open` console name match, slot 0 path through `sys_open`/`sys_dup`.
- [ ] **Step 218** [CODE][W] ~60m: BPF: `const UPC_RUNG56: bool = false;`; alias rows; `!UPC_RUNG56 &&` on the open arm, the read inode branch, fstat, dup, close, the write inode refusal, the fork `FD_TABLE` copy and the `fd_inode_*` accessors' callers; Phase 7's refusal goes with its branch.
- [ ] **Step 219** [CODE][W] ~20m: `surface.rs`, `XV6_SYSCALLS`, `parity_surface`, `parity_slot_tables` (the fd-offset mirror).
- [ ] **Step 220** [BUILD][B][S] ~8m: `kbuild; gbuild; ebuild; bbuild; [ "$(vcount)" = "$offv" ] && echo off-equal; sweep > "$S/r56-off-sweep8.txt" 2>&1; grep -c '^SAME' "$S/r56-off-sweep8.txt"` → `off-equal`, `8`.
- [ ] **Step 221** [D] ~15m: Not off-equal: a guard that does not fold, or the kernel `#define` leaking into the off build. Not 8: the kernel console name match changed init's open with the rung off (it must be dead code then).
- [ ] **Step 222** [C] ~1m: Const-off point: `cmt "[PLAN S-STAGE-2C] Steps 215-222: rungs 5+6 wired, const off" crates/xv6-mbc/adapters/usysfile.c crates/xv6-mbc/adapters/upc_slots.c crates/xv6-mbc/adapters/upc_slots.h crates/xv6-mbc/upstream/user/rwprobe.c crates/xv6-mbc/upstream/user/fdprobe.c crates/xv6-mbc/upstream/user/chdirprobe.c crates/xv6-mbc/upstream/user/pipeprobe.c crates/xv6-mbc/adapters/Makefile.mbc-userland scripts/upc-regression.sh ebpf/monad-cpu-ebpf/src/main.rs ebpf/monad-cpu-ebpf/src/phase12.rs ebpf/monad-cpu-ebpf/surface.rs cmd/upc-bootctl/src/main.rs cmd/upc-bootctl/tests/parity_slot_tables.rs references/s-stage-2c-log.md`.
- [ ] **Step 223** [CODE][B][S] ~5m: `flip UPC_RUNG56 false true; keep nv "$(vcount)"; echo "rung56=$nv delta=$((nv-offv))"; budget "$nv"`.
- [ ] **Step 224** [V] ~30s: `BUDGET-OK`; the delta is expected well below zero (the `fs_walk` dirent scan was the largest xv6-only chunk). A positive delta means an arm did not compile out: Step 221's reading again before going on.
- [ ] **Step 225** [TEST][B][S] ~15m: `for p in "rwprobe RW" "fdprobe FD" "chdirprobe CD" "pipeprobe PI" "bigprobe BG"; do set -- $p; TRIG=8000000 ub "$1" | pl "$2" > "$S/r56-$1.txt"; echo "$1 no=$(grep -c '=no$' "$S/r56-$1.txt") done=$(grep -c "$2: done" "$S/r56-$1.txt")"; done; sweep > "$S/r56-sweep8.txt" 2>&1; grep -c '^SAME' "$S/r56-sweep8.txt"`.
- [ ] **Step 226** [V] ~30s: every probe `no=0 done=1`; `8` (the skeleton's four read-path goldens, README, BIGFILE, `sub/NOTE`, `ls sub`, are among them).
- [ ] **Step 227** [D] ~25m: `ls` `DIFF`: compare the line for each entry: inum and size come from the kernel's `stat` now; a different type number for a directory or a missing `.`/`..` points at `sys_fstat`'s ilp32e layout (16 bytes, size at offset 12: Gate D's lesson). Extra `S`: D14. `tty_r` moved: rung 4's bump.
- [ ] **Step 228** [D] ~25m: Second attempt only, then `[STUCK]`, flip off (A7), Stop F stands; the design stays in ADR-135.
- [ ] **Step 229** [TEST][B] ~15m: Mutants: the console name match removed → init has no stdio and every golden `DIFF`s (the goldens see it); `fduser` keeps +16 → `RW: dup-to-zero=no`. Restore; `vcount` equals `nv`.
- [ ] **Step 230** [C] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 223-230: rungs 5+6 on: fs_walk and FD_INODE_MAP out of the ascend object" ebpf/monad-cpu-ebpf/src/main.rs references/s-stage-2c-log.md`.
- [ ] **Step 231** [TEST][W] ~40m: The usertests bodies this rung makes reachable, as probes (skeleton's subset plan, step 1): `ut_writetest`, `ut_createtest`, `ut_truncate1`, `ut_writebig`, `ut_bigwrite` (rung 3's list, FS writes now exist), `ut_opentest`, `ut_copyinstr1` (rung 6's). Each body copied from `user/usertests.c` into `user/ut_<name>.c` with a fixed-string verdict as the four existing `ut_*` do; added to the harness's `ut_*` loop and the Makefile. A body too big for the 192 KiB heap window or the trigger budget is listed, not forced.
- [ ] **Step 232** [TEST][B][S] ~20m: Run each new `ut_*` by the harness loop's own command; record pass/fail per name in `$LOG`.
- [ ] **Step 233** [D] ~20m: A failing body is a finding about the owned kernel or the UPC, not about the probe: read its first failing check; fix only if small and the body proves it; otherwise `[STUCK]` that name, drop it from the loop with a `note` line, `forstevie`.
- [ ] **Step 234** [C] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 231-234: usertests bodies for rungs 3 and 6 as probes" crates/xv6-mbc/upstream/user/ut_*.c crates/xv6-mbc/adapters/Makefile.mbc-userland scripts/upc-regression.sh references/s-stage-2c-log.md` (name each new file; a glob is expanded by the shell to named files, which is the rule's intent: check `git status --short` shows only those).
- [ ] **Step 235** [TEST][B][S] ~16m: **AFTER GATE**: `gate rung56-after`.
- [ ] **Step 236** [V] ~1m: `same=12 diff=0 rc=0 doom=1`; `rwprobe: 5 checks`, the updated FD/CD/PI counts and every `ut_*` PASS.
- [ ] **Step 237** [B][S] ~3m: `ledger 'rungs 5+6 fs hand-off' "$offv" "$nv" rung56-after; insrow "$nv"; bash scripts/check-ebpf-insns.sh 2>&1 | tail -1` (a shrink passes the ratchet).
- [ ] **Step 238** [W] ~15m: ADR-078 §5 rows 5, 8, 10, 15, 16, 21 under `UPC_RUNG56`; ADR-078 Gate D's in-BPF reader marked retired by this rung; ADR-135 D10 superseded under the const, T-F3 closed; skeleton rungs 5 and 6 dated status lines.
- [ ] **Step 239** [C] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 235-239: rungs 5+6 gate green; records" docs/adr/ADR-078-gate-d-fs-reader-and-exec-argv-abi.md docs/adr/ADR-135-upc-kernel-reply-channel.md docs/battle-plans/EVOLUTION-1-STAGE-2-skeleton.md docs/policy/ebpf-insns-baseline.tsv references/s-stage-2c-log.md docs/battle-plans/progress/S-STAGE-2C-progress.md`.
- [ ] **Step 240** [R] ~5m: `grep -c 'fs_walk' "$MAIN"` (the code stays in the source for the off build; the on object is what lost it: record the object size before and after from `ls -l` of the built object).
- [ ] **Step 241** [W] ~3m: `forstevie "Rungs 5+6 on at $nv (delta $((nv-offv))). dup onto 0-2 now works, which is what ush pipelines and redirection waited on; the ush grammar for | and > is not in this plan. Say if you want it planned."`.
- [ ] **Step 242** [W] ~5m: `$LOG` §Run cost rungs 5+6 (object size, delta, any trigger change).
- [ ] **Step 243** [C] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 240-243: rungs 5+6 recorded" references/s-stage-2c-log.md docs/battle-plans/progress/S-STAGE-2C-progress.md`.
- [ ] **Step 244** [V][STOP] ~30s: **PHASE 12 EXIT GATE, STOP G**: tree clean; `git rev-parse HEAD > "$S/last-green"`.

---

## PHASE 13: RUNG 7 DESIGN, PROCESS AUTHORITY (Steps 245-252)

**Goal**: a reviewed design that makes the kernel's `proc[]` the authority for pid, parent, state, exit status and wait, with BPF keeping the copy engine and the run queue; or a recorded decision that rung 7 needs its own plan.
**Prerequisite**: Stop G.
**Time**: ~2 h
**Agent**: Coordinator; one review agent
**Model**: Opus

- [ ] **Step 245** [R] ~20m: Every BPF use of `PROC_TABLE` (fork arm, wait arm writing status 0, exit arm, scheduler, kill's liveness, reap), the kernel's rows (ADR-109, `upc_slots.c`), `NPROC` 64 against 8 slots, what init's exit does today (D13 routes slot 0 through `upc_exit`; gate programs exit in slot 0), and the BPF stats `forks` and `waitpid` that the goldens read.
- [ ] **Step 246** [DESIGN][W] ~40m: A new ADR, number `$((adr_f8+1))` at run time (`keep adr_r7 ...`), "UPC rung 7: process authority in the owned kernel", Proposed:
  - The kernel decides: pid (I2's `pid == slot + 1` stays), parent, `ZOMBIE`, exit status, reparenting to init, and wait's answer. `wait` aliases to 11 (`3 => 11` under `UPC_RUNG7`), copies the status out, and retries (D7) while a child lives and none has exited.
  - BPF keeps what only it can do: copying a slice at fork, page directories, choosing the next runnable slot, and freeing a slot once the kernel says reaped (a new reply flag `REAP`, slot in `reply_arg`, bounded like KILL).
  - `PROC_TABLE` state is written from the kernel's replies, never from user input; slot reuse waits for `REAP`.
  - Kernel `proc[]` is limited to `UPC_SLOTS` (8) rows for slot-backed processes.
  - Init: slot 0's exit keeps D13's route (gate programs exit there); a real init that exits panics "init exiting" as stock xv6, only in a boot whose init is `init.mbc` (the skeleton's note: gates that let init exit change in the same commit).
  - Exec stays in BPF (GROUND TRUTH departure).
  - Stats parity: BPF bumps `forks` at fork as today and `waitpid` when it routes a wait, not on a retry.
  - **MRET/SRET at priv 3, revisited** (Step 26 deferred it here): with an exit path in the kernel, a U-mode MRET/SRET can kill the process (status -1) as RISC-V's illegal-instruction trap would. Written as an option; Step 250 decides.
- [ ] **Step 247** [R] ~25m: Review agent, read-only, given the ADR and Step 245's list. Ask: can a forged `REAP` free a live slot (D6 on: is any route left); what if a parent is killed between fork finalize and the note; is reparenting visible to BPF's scheduler; does wait's retry starve a sibling; does anything still read the old BPF wait arm. The coordinator waits.
- [ ] **Step 248** [DECIDE] ~10m: **Rung 7 here, or its own plan?** RECOMMENDATION: proceed in Phase 14 when the review's findings change only the fork, wait, exit and reap paths and `uproc.c`/`upc_slots.c`. Override: the review finds the change needs scheduler rework, preemption, or a frozen ABI change (BootParams, slot table layout beyond one word): Stop G is this plan's end; Phase 14 is skipped, go to Phase 15, and `forstevie` says rung 7 needs its own plan, with the ADR as its start.
- [ ] **Step 249** [W] ~15m: Dispositions into the new ADR and `$LOG`.
- [ ] **Step 250** [DECIDE] ~5m: **MRET/SRET at priv 3: kill or NOP.** RECOMMENDATION: kill, in Phase 14 (Step 265), if its measured cost is below a quarter of the headroom left after Step 260; else NOP stays and the ADR says why. Override: none.
- [ ] **Step 251** [C] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 245-251: rung 7 design (ADR-$adr_r7) and review" docs/adr/ADR-$adr_r7-*.md docs/adr/ADR-INDEX.md wiki/ADR-Index.md references/s-stage-2c-log.md docs/battle-plans/progress/S-STAGE-2C-progress.md` (index rows and counts added for the new ADR first).
- [ ] **Step 252** [V] ~30s: **PHASE 13 EXIT GATE**: every finding has a disposition; Step 248's choice is written in `$PROG`; tree clean.

---

## PHASE 14: RUNG 7, FORK, EXIT AND WAIT UNDER THE KERNEL'S AUTHORITY (Steps 253-278)

**Goal**: `UPC_RUNG7` on: wait passes the exit status, orphans reparent to init, slots free on the kernel's `REAP`; `exitwait`, `twochildren`, `forktest`, `forkfork`, `forkforkfork`, `reparent2` pass as probes.
**Prerequisite**: Phase 13 with Step 248 "proceed".
**Time**: ~7 h
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 253** [V][S] ~16m: **BEFORE GATE**: `before rung7`.
- [ ] **Step 254** [TEST][W] ~30m: `user/waitprobe.c` (`WT:`): child `exit(7)`, parent `wait(&st)` gives 7 (`WT: status=yes`); usertests' `killstatus` body (`WT: killstatus=yes`); `wait` with no children → -1 (`WT: nochild=yes`); `wait((int*)0x00900000)` with a zombie child → -1 and the child still reapable (`WT: badptr=yes`); a grandchild orphaned by its parent's exit is reaped by init's wait (`WT: reparent=yes`); `WT: done`. Harness `rung_gate UPC_RUNG7 waitprobe 8000000 WT 5`. Bodies `ut_forkfork`, `ut_forkforkfork` from `usertests.c` as in Step 231, added to the `ut_*` loop.
- [ ] **Step 255** [CODE][W] ~60m: Kernel (`uproc.c`, `upc_slots.c`, `usysproc.c`): `sys_wait` on slot rows (status copyout through `upc_user_pa`, D7 retry), exit marks `ZOMBIE` and keeps the status, reparent to init's row, the `REAP` reply when a parent's wait collects a child.
- [ ] **Step 256** [CODE][W] ~45m: BPF: `const UPC_RUNG7: bool = false;`; alias `3 => 11`; the wait arm guarded `!UPC_RUNG7 &&`; the reply block gains `REAP` (bounded: slot masked, must be a zombie child of the replying slot's process in `PROC_TABLE`); slot reuse waits for it; `waitpid` stat bumped at routing.
- [ ] **Step 257** [CODE][W] ~20m: `surface.rs`, `XV6_SYSCALLS`, `parity_surface`, `parity_slot_tables` (`UPC_REPLY_REAP`), `phase12.rs`.
- [ ] **Step 258** [BUILD][B][S] ~8m: `kbuild; gbuild; ebuild; bbuild; [ "$(vcount)" = "$offv" ] && echo off-equal; sweep > "$S/r7-off-sweep8.txt" 2>&1; grep -c '^SAME' "$S/r7-off-sweep8.txt"` → `off-equal`, `8`.
- [ ] **Step 259** [C] ~1m: Const-off point: `cmt "[PLAN S-STAGE-2C] Steps 253-259: rung 7 wired, const off" crates/xv6-mbc/adapters/uproc.c crates/xv6-mbc/adapters/upc_slots.c crates/xv6-mbc/adapters/upc_slots.h crates/xv6-mbc/adapters/usysproc.c crates/xv6-mbc/upstream/user/waitprobe.c crates/xv6-mbc/upstream/user/ut_forkfork.c crates/xv6-mbc/upstream/user/ut_forkforkfork.c crates/xv6-mbc/adapters/Makefile.mbc-userland scripts/upc-regression.sh ebpf/monad-cpu-ebpf/src/main.rs ebpf/monad-cpu-ebpf/src/phase12.rs ebpf/monad-cpu-ebpf/surface.rs cmd/upc-bootctl/src/main.rs cmd/upc-bootctl/tests/parity_slot_tables.rs references/s-stage-2c-log.md`.
- [ ] **Step 260** [CODE][B][S] ~5m: `flip UPC_RUNG7 false true; keep nv "$(vcount)"; echo "rung7=$nv delta=$((nv-offv))"; budget "$nv"`.
- [ ] **Step 261** [TEST][B][S] ~20m: `TRIG=8000000 ub waitprobe | pl WT`; each `ut_*` by the harness loop's command; `TRIG=8000000 ub forkloop | tail -5`; `sweep > "$S/r7-sweep8.txt" 2>&1; grep -c '^SAME' "$S/r7-sweep8.txt"`.
- [ ] **Step 262** [V] ~30s: `BUDGET-OK`; waitprobe 5 `=yes`, 0 `=no`; every `ut_*` pass; `8`. Else Step 263.
- [ ] **Step 263** [D] ~25m: A hang in `wait`: the retry never yields, or `REAP` never comes so the slot never frees (forkloop runs out of slots). Status 0: the copyout went to the wrong slice (slot base). `waitpid` stat moved: the bump fires on retries.
- [ ] **Step 264** [D] ~25m: Second attempt only, then `[STUCK]`, flip off (A7), Stop G stands.
- [ ] **Step 265** [CODE][B][S] ~30m: Only if Step 250 chose kill: MRET/SRET at priv 3 end the process with status -1 through the rung-7 exit path (an injected exit, as kill's reply does), `hostile_reply`'s two lines stay `mret-refused=yes`/`sret-refused=yes` and gain the parent's view `HR: priv-ret-status=yes` (wait gives -1); harness count +1. `ebuild; keep nv "$(vcount)"; budget "$nv"`.
- [ ] **Step 266** [TEST][B][S] ~5m: `TRIG=3000000 ub hostile_reply | pl HR` → every line `=yes`, `done`.
- [ ] **Step 267** [TEST][B] ~10m: Mutants: wait's status copyout skipped → `WT: status=no`; `REAP`'s zombie check removed → a probe line in waitprobe for a forged REAP is not reachable with D6 on, so this mutant is read from the code and recorded, not booted. Restore; `vcount` equals `nv`.
- [ ] **Step 268** [C] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 260-268: rung 7 on: wait passes exit status, slots free on REAP" ebpf/monad-cpu-ebpf/src/main.rs crates/xv6-mbc/upstream/user/hostile_reply.c scripts/upc-regression.sh references/s-stage-2c-log.md`.
- [ ] **Step 269** [TEST][B][S] ~16m: **AFTER GATE**: `gate rung7-after`.
- [ ] **Step 270** [V] ~1m: `same=12 diff=0 rc=0 doom=1`; `waitprobe: 5 checks`, `forkloop`, `gate_nway`, `gate2`, every `ut_*`, `hostile_reply` PASS.
- [ ] **Step 271** [D] ~15m: A gate that lets init exit fails: per the skeleton such gates change in the same commit; read which and why, change them with the reason in `$LOG`, never by recapturing a golden.
- [ ] **Step 272** [B][S] ~3m: `ledger 'rung 7 process authority' "$offv" "$nv" rung7-after; insrow "$nv"; bash scripts/check-ebpf-insns.sh 2>&1 | tail -1`.
- [ ] **Step 273** [W] ~15m: ADR-078 §5 row 3; ADR-109 (authority moved: BPF stays authoritative for scheduling only); ADR-135 Security notes (REAP's bounds; priv-3 kill if landed); ADR-`$adr_r7` Consequences with the measured numbers; skeleton rung 7 dated status.
- [ ] **Step 274** [C] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 269-274: rung 7 gate green; records" docs/adr/ADR-078-gate-d-fs-reader-and-exec-argv-abi.md docs/adr/ADR-109-*.md docs/adr/ADR-135-upc-kernel-reply-channel.md docs/adr/ADR-$adr_r7-*.md docs/battle-plans/EVOLUTION-1-STAGE-2-skeleton.md docs/policy/ebpf-insns-baseline.tsv references/s-stage-2c-log.md docs/battle-plans/progress/S-STAGE-2C-progress.md`.
- [ ] **Step 275** [W] ~5m: `$LOG` §Run cost rung 7.
- [ ] **Step 276** [W] ~2m: `forstevie "Rung 7 on at $nv (delta $((nv-offv))). Exit status reaches wait; killstatus passes. Exec stays a BPF resident lookup: the kernel cannot translate RV to MBC."`.
- [ ] **Step 277** [C] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 275-277: rung 7 recorded" references/s-stage-2c-log.md docs/battle-plans/progress/S-STAGE-2C-progress.md`.
- [ ] **Step 278** [V][STOP] ~30s: **PHASE 14 EXIT GATE, STOP H**: tree clean; `git rev-parse HEAD > "$S/last-green"`.

---

## PHASE 15: CD IN USH, ONE NAMED RECAPTURE (Steps 279-290)

**Goal**: ush has a `cd` builtin over `chdir(9)`; the `ls` golden is recaptured once, for the one number that changes (sh's size), with the reason and HEAD on record.
**Prerequisite**: any Stop from D on (chdir works since S-STAGE-2B; after Stop G, `ls` in a subdirectory opens `.` through the kernel). Runs last because it is the only phase that moves a golden.
**Time**: ~1.5 h
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 279** [V][S] ~1m: `keep offv "$(vcount)"`; tree clean.
- [ ] **Step 280** [R] ~5m: `sed -n '1,40p' crates/xv6-mbc/upstream/user/ush.c` (the header says `cd` is missing, lines 19 and 27), its builtin dispatch, and `grep -o 'sh  *2 3 [0-9]*' "$GOLD/ls.tty"` → `sh             2 3 7496`.
- [ ] **Step 281** [DECIDE][W] ~5m: **The `ls` golden moves.** `cd` grows `sh`, and `ls` prints sizes. RECOMMENDATION: one recapture of `ls.tty` and `ls.full` (not `ls.stats`), allowed only if the diff is exactly sh's size field; reason "ush gains cd (Stevie 2026-11-12, round 3)", with HEAD, in `$GOLD/README.md` and `$LOG`. Override: any other byte differs: `[STUCK]`, no recapture, `cd` waits.
- [ ] **Step 282** [CODE][W] ~20m: ush: `cd <dir>` runs `chdir` in the shell itself and prints `cannot cd <dir>` on failure, as stock xv6's sh does; header comments at lines 19 and 27 updated.
- [ ] **Step 283** [BUILD][B][S] ~5m: `gbuild; sweep12 > "$S/cd-sweep12.txt" 2>&1; grep '^DIFF' "$S/cd-sweep12.txt"`. Expected exactly `DIFF ls`.
- [ ] **Step 284** [V] ~2m: `diff <(tty_of "$(boot $'ls\n')" | sed -E 's/(sh +2 3 )[0-9]+/\1N/') <(sed -E 's/(sh +2 3 )[0-9]+/\1N/' "$GOLD/ls.tty") && echo only-sh-size`. Expected `only-sh-size`. Else Step 281's override.
- [ ] **Step 285** [B] ~3m: Recapture: `for f in tty full; do cp "$GOLD/ls.$f" "$GOLD/ls.$f.pre-cd"; done; o="$(boot $'ls\n')"; tty_of "$o" > "$GOLD/ls.tty"; printf '%s\n' "$o" > "$GOLD/ls.full"; printf '\n- %s: ls.tty and ls.full recaptured: ush gains cd (S-STAGE-2C Phase 15, Stevie 2026-11-12); only sh size changed; HEAD %s\n' "$(date +%F)" "$(git rev-parse --short HEAD)" >> "$GOLD/README.md"`. (`~/tmp/golden` is outside the repository; the `.pre-cd` copies are its rollback.)
- [ ] **Step 286** [TEST][W] ~10m: Harness block "ush cd" (boot with input): `cd sub` then `ls` lists `NOTE`; `cd ..` then `ls` lists `README`; `cd nosuch` prints `cannot cd nosuch`. `bash -n` → syntax-ok.
- [ ] **Step 287** [TEST][B][S] ~16m: **AFTER GATE**: `gate cd-after` → `same=12 diff=0 rc=0 doom=1`; the cd block PASS.
- [ ] **Step 288** [W] ~3m: `$LOG` §Golden policy: the recapture line, old and new sh size.
- [ ] **Step 289** [C] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 279-289: ush gains cd; ls golden recaptured for sh's size only" crates/xv6-mbc/upstream/user/ush.c scripts/upc-regression.sh references/s-stage-2c-log.md docs/battle-plans/progress/S-STAGE-2C-progress.md`.
- [ ] **Step 290** [V][STOP] ~30s: **PHASE 15 EXIT GATE, STOP I**: tree clean; `git rev-parse HEAD > "$S/last-green"`.

---

## PHASE 16: SECURITY PASS (Steps 291-300)

**Goal**: whatever landed is attacked once more and written down. Runs from whichever Stop ended the work.
**Prerequisite**: any Stop from A on.
**Time**: ~1.5 h
**Agent**: Coordinator; one review agent
**Model**: Opus

- [ ] **Step 291** [W] ~15m: `$LOG` §Attack surface, one row per landed change: the new kernel pointer parses (console read/write, every fd call after rung 6, wait's status), the new reply flag (`REAP`), the BPF bounds added, and the probe line that proves each.
- [ ] **Step 292** [SECURITY][R] ~10m: Every row names a harness line: `grep -c` each named line in `scripts/upc-regression.sh`; a row with none gets one added to its probe (a bad pointer for that call), booted, counted.
- [ ] **Step 293** [R] ~25m: Review agent, read-only, over `git diff develop...HEAD -- crates/xv6-mbc ebpf/monad-cpu-ebpf cmd/upc-bootctl scripts/upc-regression.sh`: kernel C parsing user pointers, BPF guards that might not fold, D6 and the priv-3 test, the opcode scan's coverage (resident programs, `--userland`), the sidecar refusal. The coordinator waits.
- [ ] **Step 294** [W] ~10m: Dispositions into ADR-135's latest review table and `$LOG`.
- [ ] **Step 295** [V] ~1m: `grep -c '^const UPC_D6_MISS_FAULT: bool = true;' "$MAIN"` → `1`; `grep -cE 'op::(MRET|SRET) && cpu.priv_level != 3|!user_ret' "$MAIN"` → at least `2`.
- [ ] **Step 296** [DECIDE] ~5m: **Fuzz.** `command -v cargo-fuzz || cargo +nightly fuzz --version 2>&1 | head -1`. RECOMMENDATION: run `fuzz_mbc_execute` for 300 s if present; else record SKIP in `$LOG`. A SKIP is not a pass; installing a tool is Stevie's call. Override: none.
- [ ] **Step 297** [C] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 291-297: attack surface, review dispositions" docs/adr/ADR-135-upc-kernel-reply-channel.md references/s-stage-2c-log.md` (plus any probe Step 292 extended); `git status --short | wc -l` → `0`.
- [ ] **Step 298** [B][S] ~10m: Repo gates: `git diff --stat develop... -- '*.go' | tail -1` (expected none); `bash scripts/check-ebpf-insns.sh 2>&1 | tail -1; bash scripts/check-spdx.sh 2>&1 | tail -1; bash scripts/check-gitleaks.sh 2>&1 | tail -1; bash scripts/check-gates-can-fail.sh --only check-ebpf-insns 2>&1 | tail -1; (cd cmd/upc-bootctl && cargo clippy --release -- -D warnings 2>&1 | tail -1); /var/tmp/unheaded-sprint/civenv/bin/ruff check scripts/upc-mbc-patch-opcode.py tests/scripts/test_upc_mbc_patch_opcode.py`.
- [ ] **Step 299** [V] ~1m: each ends in a pass; read each result on its own line before going on. A `.go` change: run the Go gates of series rule 6 too.
- [ ] **Step 300** [W][C] ~2m: ADR-135 and ADR-`$adr_r7` stay **Proposed**: acceptance is Stevie's. `forstevie "ADR-135 (amended D16-D21) and ADR-${adr_r7:-none} are Proposed; gate commits: $(git log --format=%h --grep='PLAN S-STAGE-2C' -n 14 | tr '\n' ' ')"`; `cmt "[PLAN S-STAGE-2C] Steps 298-300: security pass" docs/battle-plans/progress/S-STAGE-2C-progress.md references/s-stage-2c-log.md`. **PHASE 16 EXIT GATE**: tree clean.

---

## PHASE 17: DOC RIPPLE (Steps 301-308)

**Goal**: every document that states UPC facts says what is now true.
**Prerequisite**: Phase 16.
**Time**: ~45 min
**Agent**: agent
**Model**: Sonnet

- [ ] **Step 301** [W] ~10m: `CLAUDE.md` ASCEND-LINUX section: one S-STAGE-2C paragraph in the house style of the S-STAGE-2B one: consts on, ascend and Doom counts, D6 on, MRET/SRET at priv 3, what moved into the kernel, exec still in BPF. Replace the S-STAGE-2B paragraph's "D6 still off" with a pointer to the new paragraph (it is not a dated record).
- [ ] **Step 302** [W] ~10m: `wiki/Linux-on-UPC.md`: a dated section with the same facts and the probe list.
- [ ] **Step 303** [W] ~5m: `docs/upc/` files that describe the syscall split or D6 (`grep -rlE 'D6|FD_INODE_MAP|fs_walk' docs/upc`): a dated line each, never an edit of a dated record.
- [ ] **Step 304** [W] ~10m: `references/timeline.md`: one dated S-STAGE-2C entry. Mirrors: if `scripts/timeline-mirrors.py` exists (S-RESIDUE), `python3 scripts/timeline-mirrors.py --write && python3 scripts/timeline-mirrors.py --check`; if not, `forstevie` that the JSON/YAML/TOML mirrors lag, as S-STAGE-2B's close did.
- [ ] **Step 305** [V] ~3m: `docs/battle-plans/EVOLUTION-1-STAGE-2-skeleton.md` has a dated status line for every rung that landed (Steps 181, 204, 238, 273).
- [ ] **Step 306** [V] ~3m: `docs/adr/ADR-INDEX.md` and `wiki/ADR-Index.md` list the new ADRs with matching counts; ADR-135's row reflects its amendments (status Proposed).
- [ ] **Step 307** [B] ~2m: `bash scripts/check-spdx.sh 2>&1 | tail -1` → PASS.
- [ ] **Step 308** [C] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 301-308: docs ripple" CLAUDE.md wiki/Linux-on-UPC.md references/timeline.md docs/adr/ADR-INDEX.md wiki/ADR-Index.md` plus each `docs/upc` file and mirror changed. **PHASE 17 EXIT GATE**: tree clean.

---

## PHASE 18: CLOSE AND MERGE (Steps 309-318)

**Goal**: develop merged in, the close gate green, the branch merged into develop signed, the record complete.
**Prerequisite**: Phase 17.
**Time**: ~1 h
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 309** [B] ~5m: Series rule 15: `git merge --no-ff -S develop -m "Merge develop into sprint/s-stage-2c"`. Conflicts: Appendix A9; anything else `[STUCK]`.
- [ ] **Step 310** [BUILD][B] ~12m: `kbuild; gbuild; ebuild; bbuild; dbuild`.
- [ ] **Step 311** [TEST][B][S] ~16m: **CLOSE GATE**: `gate close; bash scripts/check-ebpf-insns.sh 2>&1 | tail -1`.
- [ ] **Step 312** [V] ~1m: `same=12 diff=0 rc=0 doom=1`, `BUDGET-OK` for `$(vcount)`, the ratchet PASS. Else: the merge brought it; find the commit, `[STUCK]`, do not merge.
- [ ] **Step 313** [W] ~10m: `$PROG` `## Close`: last Stop reached; consts landed; the ledger's last row; `[STUCK]` list with downstream impact, highest first (the Stuck Report shape); unsigned commits (if any) for `scripts/resign-unpushed.py`; the open calls for Stevie, newest last.
- [ ] **Step 314** [C] ~1m: `cmt "[PLAN S-STAGE-2C] Steps 309-314: close" docs/battle-plans/progress/S-STAGE-2C-progress.md references/s-stage-2c-log.md`.
- [ ] **Step 315** [B] ~2m: `git -C ~/tmp/unheaded checkout develop && git -C ~/tmp/unheaded merge --no-ff -S sprint/s-stage-2c -m "Merge sprint/s-stage-2c: S-STAGE-2C (ADR-135 hardening, Stage 2 rungs 3-7)"`. Never push.
- [ ] **Step 316** [V] ~1m: `git -C ~/tmp/unheaded log -1 --format='%h %G?'` → `G` (or `N` with a `$PROG` line); `git -C ~/tmp/unheaded status --short | grep -v '^??' | wc -l` → `0`.
- [ ] **Step 317** [B] ~1m: `git -C ~/tmp/unheaded worktree remove ~/tmp/wt/s-stage-2c` only after Step 316 passed (a merged worktree left behind is S-CLEAN-CUT's Step 128 lesson); the branch stays.
- [ ] **Step 318** [W] ~5m: `~/tmp/next.md` (outside the repository): a dated S-STAGE-2C paragraph: Stop reached, develop's new head, the open calls for Stevie, and "pushing is Stevie's".

---

## APPENDIX A: EMERGENCY PROCEDURES

**A1. The ascend count nears or passes 900,000.** Flip the change off. Read the delta per edit with `vcount`. In-loop variable RAM offsets cost most; hoist work out of the execute loop beside `DEFERRED_SWITCH`; `#[inline(never)]` made it worse once (+36,271). A const-off object must equal the previous count; if not, the guard covers too little (put `!UPC_RUNGn` first in the condition). Phases 2-8 only add: if headroom runs out before rung 3, run Phase 9 (which removes the BPF console write loop) and come back (Step 30's override). Past 921,036 the ladder is no longer net negative, and that change stops whatever the ceiling says.

**A2. A boot halts or panics.** bootctl prints `ROM-FAULT CTX` with the code: `0x47` MRET unmapped, `0x48` SRET unmapped, `0x11` inject STVEC unmapped, `0x80` plain HALT. `panic: upc: slot row`: a slot row reached `kexit`, `sched`, `sleep` or `freeproc` (ADR-109 I4/I7): a path needs a retry or a reply, not a sleep. With D6 on, a new halt at a user VA is a page the program expected mapped: print the VA, compare with the pgd setup.

**A3. A probe prints nothing.** `pl` strips `([0-9A-F]{4} )+`, so tokens are lower case and values fixed words or decimal digits. `TRIG` too low looks the same: raise it by 1,000,000. Kernel `printf` bytes are char plus NUL: decode from `hex:` as `ugl` does.

**A4. `sweep12` shows DIFF.** Never recapture to pass a rung (Phase 15's one named recapture is the only one). `diff <(tty_of "$(boot $'ls\n')") "$GOLD/ls.tty"`. Common causes: a corpus binary's size (an edit to `ulib.c`, `printf.c`, `usys.pl`), new `S` bytes (D14), a word store on the console path (rung 3), `tty_r` counted on retries (rung 4). Revert or flip off, then `[STUCK]`.

**A5. `git commit -S` fails.** `cmt` falls back to `--no-gpg-sign` and writes a line in `$PROG`; `scripts/resign-unpushed.py` re-signs later (probe with `git commit-tree -S`, not `gpg --clearsign`).

**A6. `upc0` is gone.** `sudo bash "$PROJECT_ROOT/scripts/upc-netns.sh" up`. The unit can stay `active (exited)` with the namespace removed (2026-10-30).

**A7. Rolling back a rung.** `flip <CONST> true false`; `gate rollback-<rung>`. Same count as the rung's OFF number means the const covered everything; `insrow` back to it. Kernel code stays; with no routed number it is dead. D6 rolls back the same way (`flip UPC_D6_MISS_FAULT true false`), and Stop A's priv-3 test can stay on by itself.

**A8. Memory pressure.** `memfloor` says `MEM-LOW`: wait for other lanes' builds, up to 30 min, then `[STUCK]`. Never kill another lane's build.

**A9. Merge conflicts.** `ebpf-insns-baseline.tsv`: develop's rows plus this branch's `monad-cpu-ebpf/monad_cpu ascend-linux` row only (S-WATCH-TRUE changes `latency-probe` rows: keep theirs). `ADR-INDEX.md`/`wiki/ADR-Index.md`: both sides' rows in number order, counts recomputed. Timeline: keep both entries; mirrors regenerated. `Cargo.lock`: `[STUCK]`.

**A10. The opcode scan refuses a corpus image.** The translator started emitting an atomic (a toolchain change). Do not relax the scan: `[STUCK]`, record which word and opcode, `forstevie`.

## APPENDIX B: AGENT MATRIX AND CRITICAL PATH

| Phase | Name | Model | Agent | Depends on | Time |
|---|---|---|---|---|---|
| 0 | Preflight | Opus | Coordinator | plan on develop | 1h |
| 1 | F7/F8 parked ADRs | Sonnet | Coordinator | 0 | 40m |
| 2 | D6, priv-3 returns | Opus | Coordinator + review agent | 1 | 3.5h |
| 3 | Bounded paths, slot 0, pipe | Opus | Coordinator | 2 | 2h |
| 4 | Atomics, opcode scan | Opus | Coordinator | 3 | 2.5h |
| 5 | .bss, sidecars | Opus | Coordinator | 4 | 2.5h |
| 6 | Fork tag, reserve, I9 | Opus | Coordinator | 5 | 2.5h |
| 7 | T-F3 interim | Opus | Coordinator | 6 | 1.25h |
| 8 | A-62 probes | Sonnet, Opus | Coordinator | 7 | 2.5h |
| 9 | Rung 3 | Opus | Coordinator | 8 | 4h |
| 10 | Rung 4 | Opus | Coordinator | 9 | 3h |
| 11 | Rungs 5+6 design | Opus | Coordinator + review agent | 10 | 1.5h |
| 12 | Rungs 5+6 | Opus | Coordinator | 11 | 7h |
| 13 | Rung 7 design | Opus | Coordinator + review agent | 12 | 2h |
| 14 | Rung 7 | Opus | Coordinator | 13 (proceed) | 7h |
| 15 | cd in ush | Opus | Coordinator | any Stop ≥ D | 1.5h |
| 16 | Security pass | Opus | Coordinator + review agent | last Stop | 1.5h |
| 17 | Doc ripple | Sonnet | agent | 16 | 45m |
| 18 | Close | Opus | Coordinator | 17 | 1h |

Critical path, every phase in order: 0 → 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8 → 9 → 10 → 11 → 12 → 13 → 14 → 15 → 16 → 17 → 18. The lane is serial; each night ends with 16 → 17 → 18 from its last Stop, and the next night re-opens the worktree (STOPPING POINTS). Only Phase 15 may move up, to right after any Stop from D on.

## APPENDIX C: QUICK REFERENCE

- **Consts** (`$MAIN`): on at start: `GETPID_INJECT`, `UPC_RUNG0`, `DEFERRED_SWITCH`, `UPC_RUNG2`, `UPC_REPLY`, `UPC_RUNG_SBRK`, `_KILL`, `_MKDIR`, `_UNLINK`, `_LINK`, `_SLEEP`, `_FD`, `_CHDIR`, `_PIPE`. Flipped here: `UPC_D6_MISS_FAULT`. New: `UPC_RUNG3`, `UPC_RUNG4`, `UPC_RUNG56`, `UPC_RUNG7`, `TIMER_SWITCH` (derived, asserted).
- **Opcodes** (`monad_common::op`): LD 0x30, XCHG 0x3D, CAS 0x3E, MRET 0x47, SRET 0x48, LR_W 0x49, SC_W 0x4A.
- **Adding a routed number**: const; alias row with `cfg!(feature = "ascend-linux")`; the BPF arm guarded `!UPC_RUNGn &&` first; `surface.rs`; `XV6_SYSCALLS`; `parity_surface`; ADR-078 §5 row.
- **Slot tables** (BootParams `reserved[0]`, byte 0x160): trapframes +0x000 stride 0x100; `cur_slot` +0x800; `trap_a7` +0x804; `fresh_mask` +0x808; `ticks` +0x80C; `brk_in` +0x810; `reply` +0x814; `reply_arg` +0x818; `cwd_mask` +0x81C. Reply flags: SET_BRK 1, YIELD 2, KILL 4, EXIT 8; `REAP` takes the next free bit (Phase 14).
- **User VA**: image from 0; heap 0x10000-0x40000; fork copies 0-0x40000 and 64 KiB around SP; stack top 0x500000; argv frame 0x600000; ceiling 0x800000; MAXPATH 128.
- **Physical**: BootParams 0x100; TTY MMIO 0xC001 (one byte per char for user output); KBD 0xFFFF; CSR n at 0xF000 + 4n; kernel `.rodata` 0x27900; kernel `.data`/`.bss` from 0x100000; ramdisk 0x800000; pgds 0xF00000 + s*0x1000; child slices 0x1000000 + (s-1)*0x800000.
- **FS**: BSIZE 1024, NDIRECT 12 (12,288 bytes direct), single indirect after.
- **Budgets**: ceiling 900,000; ladder start 921,036; Doom 758,199 must not move.
- **Booting by hand**: `TRIG=3000000 ub <probe> | pl <PREFIX>`; with input, `boot $'line\n'`. Verifier count `vcount`. Gate `gate <label>`, log in `$S/gates.txt`.

## APPENDIX D: OUT OF SCOPE

- ush pipeline and redirection grammar (rung 6 makes them possible; Step 241 asks).
- The `usertests` binary and `usertests -q` (skeleton steps 2-3): only bodies as probes here.
- Timer preemption: I9's tripwire refuses it until a plan adds it.
- Exec into the kernel: translation is host-side.
- ADR-$adr_f7 and ADR-$adr_f8 implementation (parked); R-F4 exec scrub (F8's ADR covers the reap half).
- On-disk device nodes and `mknod` (Step 212).
- Installing cargo-fuzz; any wire-format change; Doom beyond its fixture.

---

*S-STAGE-2C Battle Plan — Forged 2026-11-12*
*19 Phases. 318 Steps. The child stays in its slice, and the kernel takes the console, the files and the dead.*
*Every Stop holds the goldens; one number in one golden moves, by name.*
