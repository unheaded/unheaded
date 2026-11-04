<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# S-STAGE-2B BATTLE PLAN — 24 Phases, 282 Steps

**Date**: 2026-11-04
**Sprint**: S-STAGE-2B — EVOLUTION-1 Stage 2 past rung 1: flip rung 2, make the kernel surface honest, then land the dead syscalls in the owned kernel, one guarded rung at a time.
**Carries**: DP-717, DP-501, DP-213, DP-502, DP-503, DP-413, DP-414, DP-212, DP-718, DP-206 (prep only). DP-109 / ADR-109 D6 stays off (Stevie: not yet).
**Prerequisite**: this plan committed on `develop`; `develop` at or after `0ccb636b`; `monad_cpu` ascend 868,290 and default 758,199 in `docs/policy/ebpf-insns-baseline.tsv`; `upc0` up on west; no other lane booting a UPC.
**Target**: `UPC_RUNG2` on; `SYS_yield` minted; kernel builds answer `-ENOSYS` instead of fabricating; the RV text base travels in BootParams; sbrk, kill, mkdir, unlink, link and pause(n) served by the owned kernel, each behind its own const with a measured verifier delta. The fd table, chdir and pipe rungs follow.
**Estimated Duration**: Stops A-D about 11.5 h, plus 1.5 h for security, docs and close: one night. Stops E-H about 13.5 h more: a second night. See STOPPING POINTS.
**Branch**: `sprint/s-stage-2b` in worktree `~/tmp/wt/s-stage-2b` (series rule 1). Merge with `git merge --no-ff -S`. Never push.
**Agent Strategy**: Serial lane E, one phase at a time, no swarm (see SWARM TABLE). `upc0`, the verifier and `~/tmp/golden` are shared and single-tenant. Every eBPF, kernel, rung, ABI, root-script and `[S]` step is in an Opus phase (series rule 13). Sonnet takes the C probe sources and the doc ripple. Haiku takes the record and registry phases, which have exact old and new text.
**Commit Cadence**: at every const-off point and every gate, 4-5 steps apart where the work allows; `cmt` signs, stages named files only, and adds the two trailers. Message format `[PLAN S-STAGE-2B] Steps X-Y: ...`.
**Binding**: series rules 1-17, `docs/battle-plans/SPRINT-SERIES-2026-11-04.md`. Rule 13: cheaper models never touch a root script. Rule 14: never `pkill -f`; stop by recorded pid. Rule 15: merge develop in, rebuild, close-gate, then merge. Rule 17: `insrow` patches one baseline row and files the number under For Stevie.
**Stuck Protocol**: Skip Protocol (series rule 8): 3x a step's estimate or 2 failed debug attempts, mark `[STUCK]`, commit known-good state, move to the next independent step. Haiku and Sonnet agents never improvise (rule 12): any output the step did not predict means `[STUCK]`, paste the output into `$PROG`, go on. A rung whose gate cannot pass stops at its last Stop: flip its const off, rebuild, confirm the off object equals the previous count, and go to Phase 21.
**Standing rules**: no third-party anything; no host changes beyond boots inside `upc0`; no firewall, sshd, reboot or `compose up`; D6 stays false; Doom gets no effort beyond the `fuzix-permissive` feature; no LoC figures; temporary probes and mutants reverted before a commit.

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
| [DECIDE] | Decision with a recommendation: proceed unless the Override holds |
| [SECURITY] | Hostile-input probe or review |
| [STOP] | A clean stopping point (see STOPPING POINTS) |
| [STUCK] | Skipped via the Skip Protocol |
| [BLOCKED] | Blocked by an upstream [STUCK] |

Code blocks under a step are indented two spaces for Markdown: strip the indent when running them, heredoc terminators included.

**Model tags** (series rules 10-12): **Haiku** = exact file, exact old and new text, one verify command, no judgement. **Sonnet** = well-defined work with full commands, expected output, tests first. **Opus** = judgement: verifier, rung, ABI, concurrency, security.

---

## VARIABLES

Every Bash call is a new shell, so every step starts, from `~/tmp/wt/s-stage-2b`, with: `source <(sed -n '/^# VARS-BEGIN/,/^# VARS-END/p' docs/battle-plans/S-STAGE-2B-battle-plan.md)`. Values such as `offv` and `nv` live in `$S/v.env`, set by `keep`.

```bash
# VARS-BEGIN
export PROJECT_ROOT="$(git rev-parse --show-toplevel)"
export EBPF="$PROJECT_ROOT/ebpf" BC="$PROJECT_ROOT/cmd/upc-bootctl/target/release/upc-bootctl"
export UP="$PROJECT_ROOT/crates/xv6-mbc/upstream" AD="$PROJECT_ROOT/crates/xv6-mbc/adapters"
export K="$UP/target" GOLD="$HOME/tmp/golden" S="$HOME/tmp/s-stage-2b"
export LOG="$PROJECT_ROOT/references/s-stage-2b-log.md"
export PROG="$PROJECT_ROOT/docs/battle-plans/progress/S-STAGE-2B-progress.md"
export PLAN="$PROJECT_ROOT/docs/battle-plans/S-STAGE-2B-battle-plan.md"
export MAIN="$EBPF/monad-cpu-ebpf/src/main.rs"
export CARGO_BUILD_JOBS=4 GOMAXPROCS=4
mkdir -p "$S"
# Every Bash call is a new shell: keep carries a value to later steps; `. v.env` reloads them.
[ -f "$S/v.env" ] && . "$S/v.env"
keep()     { sed -i "/^$1=/d" "$S/v.env" 2>/dev/null; printf '%s="%s"\n' "$1" "$2" >> "$S/v.env"; export "$1=$2"; echo "$1=$2"; }
# Boot helpers (S-UPC-WIRE-COMPUTE and EVOLUTION-1 Stage 2, unchanged).
boot()     { sudo bash "$PROJECT_ROOT/scripts/upc-netns.sh" up >/dev/null && (cd "$EBPF" && sudo ip netns exec upc0 env UPC_DEMO_MODE=1 "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" \
             --userland "$K/init.mbc" --triggers 4000000 --instance 111 --input "$1" 2>&1); }
tty_of()   { grep -a 'ascii:' <<<"$1"; }
stats_of() { grep -aoE 'forks=[0-9]+|waitpid=[0-9]+|tty_r=[0-9]+' <<<"$1" | tr '\n' ' '; }
sweep()    { local f=0; for spec in "ls" "cat README" "cat BIGFILE" "cat sub/NOTE" "ls sub" "echo hello" \
             "echo peace and love" "wc README"; do out="$(boot "$spec"$'\n')"; s="$(tr ' /' '__' <<<"$spec")"; \
             diff -q <(tty_of "$out") "$GOLD/$s.tty" >/dev/null && diff -q <(stats_of "$out") "$GOLD/$s.stats" >/dev/null \
             && echo "SAME $s" || { echo "DIFF $s"; f=1; }; done; return $f; }
sweep12()  { sweep; local o; for spec in nosuch:$'nosuch\n' multi:$'echo alpha\necho omega\n' blank:$'\necho unbroken\n'; do
             o="$(boot "${spec#*:}")"; diff -q <(tty_of "$o") "$GOLD/${spec%%:*}.tty" >/dev/null && diff -q <(stats_of "$o") "$GOLD/${spec%%:*}.stats" >/dev/null \
               && echo "SAME ${spec%%:*}" || echo "DIFF ${spec%%:*}"; done
             o="$(cd "$EBPF" && sudo ip netns exec upc0 env UPC_DEMO_MODE=1 "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" \
               --userland "$K/uinit.mbc" --triggers 4000000 --instance 111 --input $'echo hello\n' 2>&1)"
             diff -q <(tty_of "$o") "$GOLD/uinit_echo.tty" >/dev/null && diff -q <(stats_of "$o") "$GOLD/uinit_echo.stats" >/dev/null \
               && echo "SAME uinit_echo" || echo "DIFF uinit_echo"; }
ub()       { local user="$1"; shift; (cd "$EBPF" && sudo ip netns exec upc0 env UPC_DEMO_MODE=1 "$@" "$BC" boot --kernel "$K/xv6-mbc.mbc" \
             --ramdisk "$K/fs.img" --userland "$K/$user.mbc" --triggers "${TRIG:-1500000}" --instance 111 2>&1); }
pl()       { grep -a 'ascii:' | sed -E 's/([0-9A-F]{4} )+//g' | sed 's/\xc2\xb7/\n/g' | grep -aoE "$1: [a-z0-9-]+(=[a-z0-9]+)?"; }
# Builds. ebuild is the ascend object every boot uses; a plain build replaces it with the Doom object.
ebuild()   { (cd "$EBPF" && cargo build --release -p monad-cpu-ebpf --features ascend-linux 2>&1 | tail -1); }
bbuild()   { (cd "$PROJECT_ROOT/cmd/upc-bootctl" && cargo build --release 2>&1 | tail -1); }
kbuild()   { (cd "$UP" && rm -f kernel/kernel.elf && make -f ../adapters/Makefile.mbc kernel 2>&1 | tail -1); }
gbuild()   { (cd "$UP" && rm -f target/fs.img && make -f ../adapters/Makefile.mbc-userland userland gates ramdisk 2>&1 | tail -1); }
# Measurement.
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
# Series rule 17: one baseline row only, numbers under For Stevie. --update would restamp every row.
insrow()   { sed -i -E "s/^(monad-cpu-ebpf\/monad_cpu\tascend-linux\t)[0-9]+(\t[^\t]+\t)[0-9-]+$/\1$1\2$(date +%F)/" "$PROJECT_ROOT/docs/policy/ebpf-insns-baseline.tsv"; grep -c "ascend-linux.$1" "$PROJECT_ROOT/docs/policy/ebpf-insns-baseline.tsv"; forstevie "ascend baseline row now $1 (net vs 921036: $((921036-$1)))" >/dev/null; }
# Exact-text edit: fails unless OLD occurs exactly once.
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
# Signed commit of named files. Unsigned only if gpg-agent is cold, and the handoff says so.
cmt()      { local m="$1"; shift; local t=$'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>\nClaude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp'
             git -C "$PROJECT_ROOT" add -- "$@" || return 1
             git -C "$PROJECT_ROOT" commit -S -m "$m" -m "$t" || { git -C "$PROJECT_ROOT" commit --no-gpg-sign -m "$m" -m "$t" && echo "- unsigned commit: $m" >> "$PROG"; }; }
# Append to the end of the "For Stevie" section of $PROG, oldest first.
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
# Idempotent edit for phases that run again on a second night: REGEX matched once
# means replace that with REDO; else OLD found once means replace that with FIRST.
upsub()    { python3 - "$@" <<'PY'
import re, sys
f, o, first, rx, redo = sys.argv[1:6]
s = open(f).read()
m = re.findall(rx, s, re.M)
if len(m) == 1:
    open(f, "w").write(re.sub(rx, lambda _: redo, s, flags=re.M)); print(f"UPSUB-REDO {f}")
elif not m and s.count(o) == 1:
    open(f, "w").write(s.replace(o, first)); print(f"UPSUB-FIRST {f}")
else:
    print(f"UPSUB-FAIL {f}: regex {len(m)}, old {s.count(o)}"); sys.exit(1)
PY
}
# VARS-END
```

---

## STOPPING POINTS

The rungs are ordered by value, cost and what they unblock. Each Stop leaves a mergeable tree: goldens 12/12, harness ALL GREEN, baseline row current. If the night is ending, or a rung will not gate, go to Phase 21 from the last Stop reached.

| Stop | After | State at that point | Cumulative time | Fits tonight? |
|---|---|---|---|---|
| A | Phase 3 | rung 2 on, ADR-110 Accepted, DP-212/718 recorded, DP-206 list handed over | ~2 h | yes |
| B | Phase 9 | SYS_yield minted, kernels answer `-ENOSYS`, RV base in BootParams | ~6.5 h | yes |
| C | Phase 11 | reply channel in, sbrk and malloc work (DP-213 "sbrk first") | ~10 h | yes |
| D | Phase 12 | kill works | ~11.5 h | likely; the planned end of night one |
| E | Phase 16 (mkdir and unlink each stop cleanly too) | user pointers safe, mkdir/unlink/link kernel-side (DP-502) | ~15 h | stretch, else second night |
| F | Phase 17 | pause(n) real; the retry hand-off exists | ~16.5 h | second night |
| G | Phase 18 | kernel fd table, O_CREATE and file writes | ~21 h | second night |
| H | Phase 20 | chdir and pipe | ~25 h | second night, may spill |

Times are cumulative wall clock, without the ~1.5 h of Phases 21-23 that every ending pays.

**Deadline.** Step 1 sets `DEADLINE`, an epoch, and the Stops compare against it. Default is 13 h from the start; the coordinator may pass a real one.

**Second night.** Same worktree and branch: `cd ~/tmp/wt/s-stage-2b`, source VARIABLES, `keep DEADLINE "$(( $(date +%s) + 13*3600 ))"`, `gate resume`, then the phase after the last Stop. Phases 21-23 run again at the end of that night; each has its own second-night notes, and every doc edit there uses `upsub`, so a re-run updates the existing S-STAGE-2B lines.

**Run cost.** Each `gate` is about 15 min (12 boots plus the harness) and rebuilds the Doom object inside the harness. A rung needs two gates plus two to four eBPF builds, so about an hour of machine time before any debugging. Nothing here runs in parallel with another boot.

## SWARM TABLE (series rule 10)

| Phases that can run at once | Why |
|---|---|
| None | Lane E is serial. Every phase shares the worktree, `upc0` or the tree a later phase builds; the Haiku and Sonnet phases run in plan order too. |

## MEMORY, VERIFIER AND SUDO

- West: 12 cores, 14 GiB, `MemAvailable` about 8 GiB at forge time, 7.2 GiB tmpfs on `/tmp`, swap never counted. No number for the verifier's kernel-side peak is in the tree; Step 9 measures it and the eBPF build's peak RSS, and the results go in `$LOG`.
- `memfloor` must print `MEM-OK` (at least 4 GiB available) before a `gate`. `gate` enforces it. On `MEM-LOW`, wait for the other lanes' builds, up to 30 min, then `[STUCK]` that step.
- All build output stays in the worktree (`ebpf/target` is about 770 MiB, `cmd/upc-bootctl/target` about 1.2 GiB, `crates/doom-runner/target` about 610 MiB, on the root volume with 114 GiB free). `CARGO_BUILD_JOBS=4` covers every cargo call, including those inside the harness.
- `monad_cpu` is loaded by every boot, so every boot pays the full verifier walk for an object near 900,000 instructions. Never run `check-ebpf-insns.sh` during a boot.
- Sudo: `[S]` steps only, in Opus phases only (series rule 13). `sudo -n true` must pass (Step 3). Boots, `sweep12`, `upc-regression.sh`, `upc-netns.sh` and `check-ebpf-loads.sh`/`check-ebpf-insns.sh` need root for `BPF_PROG_LOAD`, XDP attach and the netns. Builds, parity tests, docs and kernel or userland `make` never need sudo, and running them under sudo leaves root-owned files in `target/`.
- `Makefile.mbc clean` nukes `target/`. `kbuild` and `gbuild` do not use it. Never call it.

---

## GROUND TRUTH (read from the tree 2026-11-04)

| Fact | Where | Effect on this plan |
|---|---|---|
| `UPC_RUNG2` is false; OFF 868,290, ON 872,276 (+3,986), 12/12 and harness green both ways | `main.rs:446`; `references/evolution1-stage2-log.md` §Rung 2 | Phase 2 only flips and records |
| The net since rung 0 with rung 2 on is 921,036 − 872,276 = **48,760**; the log and DP-717 print a figure 986 too high | same; ADR-110 has 52,746 for rung 2 off | the ledger computes it from `vcount` |
| Budget gate is 900,000 (`check-ebpf-insns.sh`); the skeleton's 950,000 working limit is superseded by DP-717 | `scripts/check-ebpf-insns.sh:37` | `budget` checks both bars |
| A kernel-routed number shares the getpid inject arm through `kernel_routed_alias()`, and an empty arm keeps `parity_surface` true | `main.rs:3691`, `:2616` | every arrival repeats this; cost stays near one compare |
| `uvmalloc` writes xv6-format PTEs; a slot row's pagetable is the BPF pgd, opaque, "never walked" | `uproc.c:291`, `upc_slots.c:124`, ADR-109 D5 | sbrk must not reach `growproc` |
| Each child's VA [0, 8 MiB) is already mapped by two superpages at fork | `main.rs` fork arm, `phase12.rs:73` | sbrk is bookkeeping and zeroing, no page tables |
| Fork copies only VA [0, 256 KiB) and a 64 KiB stack window | `phase12.rs:57-64` | the heap must sit inside the low band, or fork loses it |
| `DEFAULT_PROGRAM_BREAK` is 0x400000, outside that band; exec resets to it | `monad-common/src/lib.rs:1305`, `main.rs:3305` | the kernel normalizes it to the window base |
| TTY MMIO 0xC001 and the CSR window up to 0xFFFF sit in the low band; user images end near 0x3000 | `kernel-mbc.ld`, `readelf` on `sh.elf` | heap window = [0x10000, 0x40000), 192 KiB |
| Slot 0 is identity-mapped; kernel `.rodata` is at physical 0x27900 | `readelf -S kernel.elf` | init gets no heap: `sbrk` returns -1 in slot 0 |
| A fork from init copies physical low memory into the child's band, kernel `.rodata` included | fork arm; S26 `child-tbase=same` | sbrk must zero what it grows |
| `copyin`/`copyout`/`copyinstr` call `walkaddr`/`walk` on the BPF pgd | `uvm.c:147,381,419,448` | Phase 13 comes before any pointer-taking rung |
| `kexit`, `sched`, `sleep`, `freeproc` panic on slot rows; `killed()` paths in `usertrap` reach `kexit` | `uproc.c:204,386,540,634`, `utrap.c:75,98` | blocking and kill need a way back to BPF: the reply channel |
| The kernel's FS code never sleeps here: one CPU, syscalls run to completion, no switch in S-mode (I7) | ADR-109 I7; `ulog.c` | mkdir/unlink/link need no hand-off |
| `SYS_WRITE_BLOCK` writes the same RAM the in-BPF `fs_walk` reads (word 0x200000 = byte 0x800000) | `main.rs:3308`, `monad-common lib.rs:1728` | committed FS writes are visible to BPF `open` at once |
| Rows have `cwd = 0`; `namex` does `idup(myproc()->cwd)` for relative paths | `upc_slots.c`, `ufs.c:699` | rows need a root cwd before any FS rung |
| BPF `open` walks from the root inode and ignores cwd; `fork` copies `FD_TABLE`, not kernel `ofile`; BPF `exit` never tells the kernel | `main.rs:2659`, `:2380`, `:2420` | chdir and pipe cannot be honest until the fd rung |
| The SRET arm prints `S` and arms a 100-PC tracer, and those bytes are in the goldens | `main.rs:1395`; `~/tmp/golden/*.tty` | any trap added to a corpus path moves goldens: D14 |
| BPF `wait` writes 0 to `*status` | `main.rs` wait arm | `killstatus` stays out until rung 7 |
| The FUZIX fakes (23 numbers) sit in the INT 0x80 chain, compiled out of the ascend object; a kernel-build INT 0x80 falls to IVT dispatch | `main.rs:1465`, `execute.rs:709` | DP-503 needs an ascend INT arm |
| In kernel builds the real lie is the ecall fall-through "Unknown syscall: silently ignore": `sbrk(n)` returns `n`, `pipe(p)` returns `p` | `main.rs:3343` | DP-503 fixes that too |
| `monad-mbc` has no `[features]` table; no test in `crates/monad-mbc/tests` touches the 23 | `Cargo.toml`, grep | the feature is additive; the only consumer is `demos/mbc/fuzix_compat` |
| ADR-078's "registry" is the a2/a3 re-entry table; no syscall-number registry exists | ADR-078 §3 | DP-413 adds one as ADR-078 §5 |
| Number 22 is the probe-only inject number | `main.rs:439`, `upc-rung0-assert.awk` | SYS_yield is **23** |
| `usys.o` links into every program, and the `ls` golden prints file sizes | `Makefile.mbc-userland` `USER_LIB_OBJS` | yield is an inline header stub, or the `ls` golden moves |
| `--bootstub` already stages the stub's `.data` sidecar; a failed or missing sidecar is only a warning | `main.rs:701,738-756` | the DP-414 `.rodata` half is small |
| RV base is `let text_rv_word_base: u32 = 0x8000` in the loader | `main.rs:772` | moves to one const and BootParams `reserved[1]` (byte 0x164) |
| No script or test checks BootParams against its C mirrors; only `parity_slot_tables.rs` pins `reserved[0]` | `cmd/upc-bootctl/tests` | Phase 9 creates the frozen-ABI check |
| DP-206 is approved, and the run has already happened once under guard (S0-S4, 2026-10-28) | `references/s-ascend-2-log.md` §E3 across hosts; pack "Still open" | Appendix D is the attended list for S-E1-LIVE |
| `upc-regression.sh` lines 39, 235, 251 ran `sudo pkill -9 -f` on every gate, and the harness never starts `doom-go-injector` | the script | Step 6 stops doom-runner by recorded pid (series rule 14) |
| A const that is true changes the alias in the Doom object too: `kernel_routed_alias` runs in both builds | `main.rs:2062` | every new alias row carries `cfg!(feature = "ascend-linux")` |

### Departures from the pack's order (all forced by the tree; see the rows above)

Pack order: sbrk, pipe, kill, mkdir, unlink, link, chdir, sleep. Plan order: **sbrk, kill, mkdir, unlink, link, sleep, fd table, chdir, pipe.** kill needs no user pointer. pipe needs user-pointer copyout, the kernel fd table, exit-time close and blocking, so it lands last. chdir means nothing while BPF `open` ignores cwd, so it waits for the fd rung. Recorded under "For Stevie" in `$PROG` at Step 19.

---

## KNOWN FAILURES BASELINE

Empty at forge time. Step 10 records anything else. A failure on that list is not this sprint's to fix unless a rung needs it.

---
## PHASE 0: PREFLIGHT, WORKTREE, BASELINES (Steps 1-12)

**Goal**: the worktree builds everything the boots use, the baselines match, and memory and verifier costs are measured.
**Prerequisite**: this plan committed on `develop`; no other lane booting.
**Time**: ~45 min
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 1** [B] ~1m: Create the worktree, the projects symlink (series rule 1) and the deadline.
  ```bash
  ln -sfn /home/govan/tmp/projects /home/govan/tmp/wt/projects
  git -C ~/tmp/unheaded worktree add ~/tmp/wt/s-stage-2b -b sprint/s-stage-2b develop && cd ~/tmp/wt/s-stage-2b && git log -1 --format='%h %G?'
  source <(sed -n '/^# VARS-BEGIN/,/^# VARS-END/p' docs/battle-plans/S-STAGE-2B-battle-plan.md); keep DEADLINE "$(( $(date +%s) + 13*3600 ))"
  ```
  Expected: `Preparing worktree`, one hash with `G`, `DEADLINE=<epoch>`. Branch exists already: `cd ~/tmp/wt/s-stage-2b`, carry on. Any other output: `[STUCK]`.
- [ ] **Step 2** [V][S] ~30s: From `~/tmp/wt/s-stage-2b`, source VARIABLES, then `type ub pl ebuild bbuild kbuild gbuild boot sweep sweep12 vcount budget memfloor flip gate ledger insrow rep cmt keep forstevie upsub | grep -c 'is a function'` → `21`. Any other number: `[STUCK]`.
- [ ] **Step 3** [B][S] ~1m: Tools and sudo.
  ```bash
  for t in riscv64-unknown-elf-gcc bpf-linker bpftool perl xxd readelf /usr/bin/time python3; do command -v "$t" >/dev/null && echo "ok $t" || echo "MISSING $t"; done; sudo -n true && echo sudo-ok; ip netns list | grep -c upc0; systemctl is-active unheaded-upc-netns.service; ls ~/tmp/projects/doom-related/DOOM.WAD
  ```
  Expected: eight `ok` lines, `sudo-ok`, `1`, `active`, the WAD path. `MISSING` or no `sudo-ok`: `[STUCK]`, and every later step that boots is `[BLOCKED]`. `0` for upc0: run `sudo bash "$PROJECT_ROOT/scripts/upc-netns.sh" up` once and repeat this step.
- [ ] **Step 4** [W] ~2m: Create `$LOG` and `$PROG`.
  ```bash
  { printf '# S-STAGE-2B log\n\nStarted 2026-11-04 on %s (branch sprint/s-stage-2b).\nPlan: docs/battle-plans/S-STAGE-2B-battle-plan.md\n\n## Ledger (verified insns, ascend)\n\n| Rung | OFF | ON | delta | net vs 921,036 | gate |\n|---|---|---|---|---|---|\n' "$(git rev-parse --short HEAD)"; } > "$LOG"
  { printf '<!--\nSPDX-License-Identifier: GPL-3.0-or-later\nCopyright (c) 2024-2026 Stevie Bellis.\n-->\n\n# S-STAGE-2B progress\n\nPlan: `docs/battle-plans/S-STAGE-2B-battle-plan.md`. Evidence: `references/s-stage-2b-log.md`.\n\nSkip Protocol acknowledged: skip after 3x the estimate or 2 failed debug attempts, mark [STUCK], commit known-good state, continue with the next independent step.\n\n## For Stevie\n\n'; } > "$PROG"; wc -l "$LOG" "$PROG"
  ```
  Expected: `9` and `13` lines, plus a total line. Any other count: `[STUCK]`.
- [ ] **Step 5** [R] ~2m: Hypotheses, one command.
  ```bash
  cd "$PROJECT_ROOT"; grep -cE '^const UPC_RUNG2: bool = false;|^const UPC_D6_MISS_FAULT: bool = false;|^const GETPID_INJECT: bool = true;|^const UPC_RUNG0: bool = true;|^const DEFERRED_SWITCH: bool = true;' "$MAIN"; grep -c 'SYS_yield' crates/xv6-mbc/upstream/kernel/syscall.h; grep -n 'text_rv_word_base: u32 = 0x8000' cmd/upc-bootctl/src/main.rs | wc -l; grep -c 'monad-cpu-ebpf/monad_cpu.ascend-linux.868290' docs/policy/ebpf-insns-baseline.tsv; grep -c 'monad-cpu-ebpf/monad_cpu.default.758199' docs/policy/ebpf-insns-baseline.tsv; grep -n '^default' ebpf/monad-cpu-ebpf/Cargo.toml; ls docs/adr | grep -oE '^ADR-[0-9]+' | sort -t- -k2 -n | tail -2 | tr '\n' ' '; echo
  ```
  Expected, in order: `5`, `0`, `1`, `1`, `1`, `13:default = []`, and a last ADR number at or above 119 (ADR-69420 aside). Different: paste into `$PROG`, `[STUCK]`, stop the phase.
- [ ] **Step 6** [CODE][W][S] ~10m: Series rule 14: the harness stops doom-runner by recorded pid. It runs as root, so this stays Opus. `sudo` drops the environment, so the pid file and the binary go to `sh` as arguments, not variables.
  ```bash
  python3 - <<'PY'
  p = "scripts/upc-regression.sh"
  s = open(p).read()
  def r(o, n):
      global s
      assert s.count(o) >= 1, o
      s = s.replace(o, n)
  r('LOG="$(mktemp)"\n', 'LOG="$(mktemp)"\nPIDF="$(mktemp)"\n')
  r('cleanup() {\n  { sudo pkill -9 -f "doom-runner run"; sudo pkill -9 -f doom-go-injector; } >/dev/null 2>&1\n',
    '# Stop doom-runner by its recorded pid (series rule 14).\ndoom_stop() { local p; p="$(cat "$PIDF" 2>/dev/null)"; [ -n "$p" ] && sudo kill -9 "$p"; : > "$PIDF"; }\n\ncleanup() {\n  doom_stop >/dev/null 2>&1\n')
  r('rm -f "$LOG"\n', 'rm -f "$LOG" "$PIDF"\n')
  r('{ sudo pkill -9 -f "doom-runner run"; sudo "$DR" ring teardown --hops 2; } >/dev/null 2>&1', '{ doom_stop; sudo "$DR" ring teardown --hops 2; } >/dev/null 2>&1')
  r('sudo UPC_VERIFIER_STATS=1 setsid "$DR" run --doom-mbc', 'sudo UPC_VERIFIER_STATS=1 setsid sh -c \'echo $$ > "$1"; shift; exec "$@"\' sh "$PIDF" "$DR" run --doom-mbc')
  open(p, "w").write(s); print("PIDFIX-OK")
  PY
  bash -n scripts/upc-regression.sh && echo syntax-ok; grep -c 'pkill' scripts/upc-regression.sh
  ```
  Expected: `PIDFIX-OK`, `syntax-ok`, `0`. Step 10's harness run proves it: `pgrep -c doom-runner` prints `0` after that gate.
- [ ] **Step 7** [BUILD][B] ~12m: Build everything, measuring the eBPF build.
  ```bash
  memfloor; (cd crates/monad-mbc && cargo build --release 2>&1 | tail -1); kbuild; gbuild; ( cd "$EBPF" && /usr/bin/time -v cargo build --release -p monad-cpu-ebpf --features ascend-linux > "$S/p0-ebuild-time.txt" 2>&1 ); grep -a 'Finished' "$S/p0-ebuild-time.txt"; bbuild; (cd crates/doom-runner && cargo build --release 2>&1 | tail -1); ls "$K"/*.rv2mbc.sha256 | wc -l; ls "$K"/getpidfork.mbc "$K"/fs.img "$BC" crates/doom-runner/target/release/doom-runner | wc -l; grep 'Maximum resident' "$S/p0-ebuild-time.txt"
  ```
  Expected: `MEM-OK`; each cargo line ends `Finished ...`; at least 12 sidecars; `4`; one `Maximum resident` line. `MEM-LOW`: wait, up to 30 min, repeat. Any `error`: Step 8.
- [ ] **Step 8** [D] ~5m: Only on a build error: `grep -nE '^error|No rule to make' "$S"/*.txt | head; (cd "$UP" && make -f ../adapters/Makefile.mbc kernel 2>&1 | tail -5)`. Paste into `$PROG`, `[STUCK]`, stop the phase. A fresh worktree that fails here is a tree fact, not a fix for this lane.
- [ ] **Step 9** [B][S] ~2m: One boot, sampling memory while the verifier walks.
  ```bash
  python3 - > "$S/p0-mem.txt" <<'PY' &
  import time
  lo = 1 << 60
  for _ in range(150):
      a = int([l for l in open('/proc/meminfo') if l.startswith('MemAvailable')][0].split()[1]) // 1024
      lo = min(lo, a); time.sleep(0.2)
  print(f"min MemAvailable during boot: {lo} MiB")
  PY
  before=$(awk '/MemAvailable/{print int($2/1024)}' /proc/meminfo); TRIG=3000 ub getpidfork UPC_VERIFIER_STATS=1 | grep -aE 'verified [0-9]+ insns' | tail -1; wait; echo "before: $before MiB"; cat "$S/p0-mem.txt"
  ```
  Expected: a `verified 868290 insns` line, `before:` and `min MemAvailable` lines. Append the last two and the `Maximum resident` line to `$LOG` under `## Run cost`. The count differs: Step 10 decides.
- [ ] **Step 10** [TEST][B][S] ~30m: The baseline gate (the first `check-ebpf-insns.sh` builds every object into `ebpf/target/load-gate`; later runs rebuild only `monad-cpu-ebpf`): `gate p0; bash scripts/check-ebpf-insns.sh 2>&1 | tail -2`.
  Expected: `gate p0: same=12 diff=0 rc=0 doom=1 ascend=868290 ...`, the ratchet ends in a PASS line, and `pgrep -c doom-runner` prints `0`. A gate is green only with `doom=1`: every later "`same=12 diff=0 rc=0`" means that too. `DIFF` lines or `rc=1`: copy `grep -a FAIL "$S/p0-harness.txt"` and the `DIFF` names into `$LOG` under `## Known failures`, `[STUCK]`, stop: a red baseline means no rung is measurable.
- [ ] **Step 11** [C] ~1m: `cmt "[PLAN S-STAGE-2B] Steps 1-11: worktree, harness stops by pid, baselines, run cost" scripts/upc-regression.sh references/s-stage-2b-log.md docs/battle-plans/progress/S-STAGE-2B-progress.md` → one commit; `git log -1 --format=%G?` → `G`, or `N` with a line in `$PROG`.
- [ ] **Step 12** [V] ~1m: **PHASE 0 EXIT GATE**: `grep -c 'same=12 diff=0 rc=0' "$S/gates.txt"` → at least `1`; `git status --short | wc -l` → `0`. Else stop.

---

## PHASE 1: RECORD DP-212, DP-718, DP-206 PREP (Steps 13-22)

**Goal**: the bootstub's scope is written where a reader finds it, and the east run's needs are handed to S-E1-LIVE.
**Prerequisite**: Phase 0.
**Time**: ~10 min
**Agent**: one agent, serial, no boot
**Model**: Haiku

Every `rep` prints `REP-OK <file>`. `REP-FAIL`: mark the step `[STUCK]`, go on.

- [ ] **Step 13** [W] ~1m: `crates/upc-bootstub/README.md`.
  ```bash
  cd "$PROJECT_ROOT"; rep crates/upc-bootstub/README.md '**Status:** P0 scaffolding (2026-05-11). Phase 2 day-1 fills in the build pipeline + integration.' '**Status:** Kept (DP-718, 2026-11-04). Done for xv6-class kernels (DP-212): stage 1 of Boot Protocol v2 (ADR-067), built end to end. No vendored Linux image is planned; the DIY kernel (ADR-081) is the rung.'
  ```
- [ ] **Step 14** [W] ~1m: `crates/upc-bootstub/src/lib.rs`.
  ```bash
  rep crates/upc-bootstub/src/lib.rs $'//! - NOT yet boot-integrated: the stub has no kernel to hand off to until\n//!   Phase 2.1 vendors one, and per the Phase 2.0 capacity audit\n//!   (`references/phase2-preflight-capacity-audit-2026-07-02.md`) the\n//!   code-store substrate can\'t hold a Linux image yet. Boot-path wiring\n//!   (`upc-bootctl --bootstub`) waits on that substrate decision.\n//! - Loader note for Phase 2.4: the ilp32e link keeps `.rodata` in the BOOT\n//!   text region AND the translator emits a `.data` sibling for it — decide\n//!   which the loader stages so the "BOOT FAIL" strings resolve once.' $'//! - Scope (DP-212, DP-718, 2026-11-04): done for xv6-class kernels and kept\n//!   as stage 1 of Boot Protocol v2 (`upc-bootctl --bootstub` loads it). No\n//!   vendored Linux image is planned: the code store cannot hold one\n//!   (`references/phase2-preflight-capacity-audit-2026-07-02.md`) and ADR-081\n//!   builds our own kernel instead.\n//! - `.rodata` (DP-414): the loader stages only the translator\'s `.data`\n//!   sidecar, so the "BOOT FAIL" strings resolve once.'
  ```
- [ ] **Step 15** [W] ~1m: `docs/adr/ADR-080-upc-programmatic-api.md`.
  ```bash
  rep docs/adr/ADR-080-upc-programmatic-api.md '| uClinux/Linux | Phase 2, blocked on code-store decision | (will extend the ASCEND surface) | bootstub → S-mode MRET |' '| uClinux/Linux | Dropped (ADR-081; DP-212, 2026-11-04): no vendored Linux image is planned; the bootstub is done for xv6-class kernels | n/a | bootstub → S-mode MRET |'
  ```
- [ ] **Step 16** [W] ~1m: `docs/adr/ADR-067-mbc-isa-v2-and-upc-abi-v1.md`: append.
  ```bash
  printf '\n## Decisions 2026-11-04 (decision pack)\n\n- DP-212: the bootstub is done for xv6-class kernels; the DIY tiny kernel (ADR-081) is the rung, not a vendored Linux. No multi-map code store is planned until our own kernel outgrows the map.\n- DP-718: `crates/upc-bootstub` is kept as stage 1 of Boot Protocol v2. This ADR is not amended.\n' >> docs/adr/ADR-067-mbc-isa-v2-and-upc-abi-v1.md; tail -1 docs/adr/ADR-067-mbc-isa-v2-and-upc-abi-v1.md | cut -c1-20
  ```
  Expected: `- DP-718: `crates/up`.
- [ ] **Step 17** [W] ~1m: `docs/sweeps/OUTSTANDING-2026-10-27.md`, row M-230.
  ```bash
  rep docs/sweeps/OUTSTANDING-2026-10-27.md '`crates/upc-bootstub` delete or keep; Phase 2.5 L6 brief' '`crates/upc-bootstub` kept (DP-718, 2026-11-04); Phase 2.5 L6 brief'
  ```
- [ ] **Step 18** [V] ~30s: `grep -c 'DP-212' docs/adr/ADR-081-unheaded-linux-from-scratch.md` → `1` or more (ADR-081 already records it; change nothing). `0`: `[STUCK]`.
- [ ] **Step 19** [W] ~1m: The questions for Stevie, oldest first.
  ```bash
  cat >> "$PROG" <<'TXT'
  1. (plan-time) Rung order differs from the pack: sbrk, kill, mkdir, unlink, link, sleep, fd table, chdir, pipe. pipe and chdir cannot be honest before the kernel fd table. Veto if you want otherwise.
  2. (plan-time) Heap: 192 KiB per process in VA [0x10000, 0x40000), and init (slot 0) gets none. A bigger heap needs a third fork-copy band.
  3. (plan-time) `cargo-fuzz` is not installed on west, so `fuzz_mbc_execute` records SKIP. Installing it is a third-party question.

  TXT
  grep -c 'plan-time' "$PROG"
  ```
  Expected: `3`.
- [ ] **Step 20** [W] ~1m: One-line pointer for S-E1-LIVE.
  ```bash
  rep "$PROG" '## For Stevie' $'DP-206 (approved, already run once under guard): the attended list is Appendix D of the plan, for S-E1-LIVE.\n\n## For Stevie'
  ```
- [ ] **Step 21** [C] ~1m: `cmt "[PLAN S-STAGE-2B] Steps 12-21: DP-212, DP-718 recorded; DP-206 pointer for S-E1-LIVE" crates/upc-bootstub/README.md crates/upc-bootstub/src/lib.rs docs/adr/ADR-080-upc-programmatic-api.md docs/adr/ADR-067-mbc-isa-v2-and-upc-abi-v1.md docs/sweeps/OUTSTANDING-2026-10-27.md docs/battle-plans/progress/S-STAGE-2B-progress.md`.
- [ ] **Step 22** [V] ~1m: **PHASE 1 EXIT GATE**: `(cd crates/upc-bootstub && cargo test 2>&1 | grep '^test result' | sort | uniq -c)` → every line has `0 failed`; `git status --short | wc -l` → `0`. Else `[STUCK]`.

---

## PHASE 2: RUNG 2, UPTIME — THE FLIP (Steps 23-35)

**Goal**: `UPC_RUNG2` on under the cumulative stop rule (DP-717), measured on this build.
**Prerequisite**: Phase 0 exit gate (gate p0 is this rung's before-gate).
**Time**: ~45 min
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 23** [V][S] ~1m: **BEFORE GATE**: `git rev-parse HEAD` differs from `$S/last-green` only by docs: `git diff --stat "$(cat "$S/last-green")" HEAD -- ebpf crates/xv6-mbc scripts cmd | wc -l` → `0`. Not 0: `gate rung2-before`.
- [ ] **Step 24** [CODE][B][S] ~6m: `flip UPC_RUNG2 false true` → `1`, then `Finished`. Then `keep v "$(vcount)"; echo "$v"; budget "$v"`.
- [ ] **Step 25** [V] ~1m: `872276` and `BUDGET-OK`. Toolchain drift is allowed: the delta from the p0 count must be within 3,986 ± 300. Outside that: Step 26.
- [ ] **Step 26** [D][S] ~10m: Compare on one build: `flip UPC_RUNG2 true false; vcount; flip UPC_RUNG2 false true; vcount`. OFF not equal to p0 means the object moved under the const: read `git diff "$(cat "$S/last-green")" -- ebpf`. Twice unexplained: `[STUCK]`, flip off, Stop A without the flip.
- [ ] **Step 27** [TEST][B][S] ~2m: `TRIG=2000000 ub uptimeprobe | pl UP | tr '\n' ' '`.
- [ ] **Step 28** [V] ~30s: `UP: init-moved=yes`, `UP: child-moved=yes`, `UP: child-mono=yes`, `UP: init-before=large` (ADR-110 Consequences). Else Step 26.
- [ ] **Step 29** [W][S] ~1m: The harness note (a root script, so here, not in a cheaper phase).
  ```bash
  rep scripts/upc-regression.sh 'uptime gate skipped: UPC_RUNG2 is off (ADR-110 waits on the stop-rule reading)' 'uptime gate skipped: UPC_RUNG2 is off (rollback build)'
  ```
- [ ] **Step 30** [W] ~1m: The object's own comment.
  ```bash
  rep ebpf/monad-cpu-ebpf/src/main.rs $'// kernel, and inject_strap publishes the tick count the kernel\'s clock follows.' $'// kernel, and inject_strap publishes the tick count the kernel\'s clock follows.\n// On since 2026-11-04 (DP-717). Rollback = this const off.'
  ```
- [ ] **Step 31** [TEST][B][S] ~15m: **AFTER GATE**: `ebuild; gate rung2-after`.
- [ ] **Step 32** [V] ~1m: `same=12 diff=0 rc=0`; `grep -a 'uptime: kernel clock' "$S/rung2-after-harness.txt" | grep -c PASS` → `1`; Doom line in the harness shows 758199.
- [ ] **Step 33** [W][S] ~2m: Ledger and baseline: `ledger 'rung 2 uptime (DP-717)' 868290 "$v" rung2-after; insrow "$v"; bash scripts/check-ebpf-insns.sh 2>&1 | tail -1`. Expected `1`, then a PASS line. Growth on this one row is what DP-717 authorizes; every other row still only shrinks (series rule 6). `echo "net since rung 0: $((921036-v))" >> "$LOG"`.
- [ ] **Step 34** [C][S] ~1m: `cmt "[PLAN S-STAGE-2B] Steps 22-34: UPC_RUNG2 on, uptime served by the owned kernel (DP-717)" ebpf/monad-cpu-ebpf/src/main.rs scripts/upc-regression.sh docs/policy/ebpf-insns-baseline.tsv references/s-stage-2b-log.md`.
- [ ] **Step 35** [V] ~1m: **PHASE 2 EXIT GATE**: `grep -c '^const UPC_RUNG2: bool = true;' "$MAIN"` → `1`; tree clean; `git rev-parse HEAD > "$S/last-green"`.

---

## PHASE 3: RECORD RUNG 2 (Steps 36-41)

**Goal**: ADR-110 and its neighbours say what is true.
**Prerequisite**: Phase 2.
**Time**: ~10 min
**Agent**: one agent, serial, no boot
**Model**: Haiku

Every `rep` prints `REP-OK <file>`. `REP-FAIL`: `[STUCK]`, go on. `$v` is the count Phase 2 kept.

- [ ] **Step 36** [W] ~1m: ADR-110 status.
  ```bash
  cd "$PROJECT_ROOT"; rep docs/adr/ADR-110-upc-kernel-time-from-bpf-ticks.md '**Status**: Proposed (2026-10-28)' '**Status**: Accepted (2026-11-04; DP-717, stop rule cumulative; `UPC_RUNG2` on)'
  ```
- [ ] **Step 37** [W] ~1m: ADR-110 stop-rule paragraph.
  ```bash
  rep docs/adr/ADR-110-upc-kernel-time-from-bpf-ticks.md $'- **E2\'s stop rule** says to stop at the first rung whose freed instructions are below zero. Read per rung, an\n  arrival can never pass it. Read cumulatively, the ladder has freed 52,746 so far (921,036 → 868,290). The flag\n  stays off until Stevie says which reading holds.' $'- **E2\'s stop rule** is cumulative (DP-717, 2026-11-04): a rung may cost as long as the ladder stays net negative\n  and under the 900,000 ceiling. Rung 2 costs +'"$((v-868290))"$'; the ladder stands at '"$v"$', '"$((921036-v))"$' below where it began\n  (921036). `UPC_RUNG2` is on.'
  ```
- [ ] **Step 38** [W] ~1m: The skeleton's stop rule.
  ```bash
  rep docs/battle-plans/EVOLUTION-1-STAGE-2-skeleton.md $'**Stop rule (E2 = (a)):** a migration rung with freed insns < 0 stops the push at that\nrung. It and the later migration rungs stay in BPF unless authenticity requires them, which\nis (c) for the remainder. Arrivals continue.' $'**Stop rule (E2 = (a), DP-717 2026-11-04):** cumulative. A rung may cost verified\ninstructions as long as the ladder stays net negative (below 921,036) and under the 900,000\nceiling. Forged past rung 2 in `S-STAGE-2B-battle-plan.md`.'
  ```
- [ ] **Step 39** [V] ~1m: `grep -c 'Accepted (2026-11-04' docs/adr/ADR-110-upc-kernel-time-from-bpf-ticks.md` → `1`.
- [ ] **Step 40** [C] ~1m: `cmt "[PLAN S-STAGE-2B] Steps 35-40: ADR-110 accepted; stop rule recorded as cumulative" docs/adr/ADR-110-upc-kernel-time-from-bpf-ticks.md docs/battle-plans/EVOLUTION-1-STAGE-2-skeleton.md`.
- [ ] **Step 41** [V][STOP] ~1m: **PHASE 3 EXIT GATE, STOP A**: `git status --short | wc -l` → `0`. This phase touched docs only.

---

## PHASE 4: PROBE SOURCES (Steps 42-54)

**Goal**: every rung's C probe exists and builds before any rung code. Tests first.
**Prerequisite**: Stop A.
**Time**: ~25 min
**Agent**: one agent, serial, no boot
**Model**: Sonnet

Sources are final: write them byte for byte, do not improve them. All paths are under `$UP/user`. Print only fixed strings (the MBC userland varargs ABI is unreliable).

- [ ] **Step 42** [W] ~2m: `crates/xv6-mbc/upstream/user/uyield.h`.
  ```c
  // SPDX-License-Identifier: GPL-3.0-or-later
  // uyield.h: yield() as an inline stub, SYS_yield = 23 (ADR-078 §5, DP-413).
  // Not in usys.pl on purpose: usys.o links into every program, and a bigger
  // text changes the file sizes the `ls` golden prints.
  #ifndef UYIELD_H
  #define UYIELD_H
  static inline int
  yield(void)
  {
    int r;
    __asm__ volatile("li a7, 23\n\tecall\n\tmv %0, a0" : "=r"(r) : : "a0", "memory");
    return r;
  }
  #endif
  ```
- [ ] **Step 43** [W] ~3m: `crates/xv6-mbc/upstream/user/sbrkprobe.c`.
  ```c
  // SPDX-License-Identifier: GPL-3.0-or-later
  // sbrkprobe.c: S-STAGE-2B sbrk rung. Runs as init; the heap belongs to a child.
  #include "kernel/types.h"
  #include "kernel/stat.h"
  #include "user/user.h"

  #define SAY(t, c) do { if(c) printf(t "=yes\n"); else printf(t "=no\n"); } while(0)

  int
  main(void)
  {
    SAY("SB: init-refused", sbrk(4096) == SBRK_ERROR);
    int pid = fork();
    if(pid == 0){
      char *a = sbrk(0);
      SAY("SB: base", a == (char *)0x10000);
      char *b = sbrk(0x19000); // past physical 0x27900: a fork from init copies kernel .rodata here
      SAY("SB: grow", b == a);
      SAY("SB: moved", sbrk(0) == a + 0x19000);
      int zero = 1;
      for(int i = 0; i < 0x19000; i++)
        if(b[i] != 0)
          zero = 0;
      SAY("SB: zeroed", zero);
      for(int i = 0; i < 4096; i++)
        b[i] = (char)(i * 7);
      int c = fork();
      if(c == 0){
        int same = 1;
        for(int i = 0; i < 4096; i++)
          if(b[i] != (char)(i * 7))
            same = 0;
        SAY("SB: fork-copied", same);
        exit(0);
      }
      wait(0);
      SAY("SB: shrink", sbrk(-0x19000) == a + 0x19000);
      SAY("SB: shrunk", sbrk(0) == a);
      SAY("SB: over-cap-refused", sbrk(0x30001) == SBRK_ERROR);
      SAY("SB: under-base-refused", sbrk(-1) == SBRK_ERROR);
      SAY("SB: intmin-refused", sbrk(-2147483647 - 1) == SBRK_ERROR);
      SAY("SB: unchanged", sbrk(0) == a);
      char *m = malloc(1000);
      SAY("SB: malloc", m != 0);
      if(m){
        memset(m, 0x5A, 1000);
        free(m);
      }
      exit(0);
    }
    wait(0);
    printf("SB: done\n");
    for(;;)
      ;
  }
  ```
- [ ] **Step 44** [W] ~3m: `crates/xv6-mbc/upstream/user/killprobe.c`.
  ```c
  // SPDX-License-Identifier: GPL-3.0-or-later
  // killprobe.c: S-STAGE-2B kill rung. Runs as init. Exit status is not checked:
  // BPF wait writes 0 until rung 7.
  #include "kernel/types.h"
  #include "kernel/stat.h"
  #include "user/user.h"
  #include "user/uyield.h"

  #define SAY(t, c) do { if(c) printf(t "=yes\n"); else printf(t "=no\n"); } while(0)

  static void
  spin(void)
  {
    for(;;)
      yield();
  }

  int
  main(void)
  {
    SAY("KL: zero-refused", kill(0) == -1);
    SAY("KL: neg-refused", kill(-1) == -1);
    SAY("KL: huge-refused", kill(999) == -1);
    SAY("KL: init-refused", kill(1) == -1);
    int c = fork();
    if(c == 0)
      spin();
    yield();
    SAY("KL: kill-ok", kill(c) == 0);
    SAY("KL: reaped", wait(0) == c);
    SAY("KL: dead-refused", kill(c) == -1);
    int s = fork();
    if(s == 0){
      kill(getpid());
      printf("KL: self-survived=yes\n");
      exit(0);
    }
    SAY("KL: self-reaped", wait(0) == s);
    int p = fork();
    if(p == 0){
      if(fork() == 0)
        spin();
      spin();
    }
    yield();
    yield();
    SAY("KL: parent-killed", kill(p) == 0);
    SAY("KL: parent-reaped", wait(0) == p);
    SAY("KL: orphan-killed", kill(p + 1) == 0);
    SAY("KL: orphan-reaped", wait(0) == p + 1);
    printf("KL: done\n");
    for(;;)
      ;
  }
  ```
- [ ] **Step 45** [W] ~3m: `crates/xv6-mbc/upstream/user/mkprobe.c`.
  ```c
  // SPDX-License-Identifier: GPL-3.0-or-later
  // mkprobe.c: S-STAGE-2B mkdir rung. The kernel writes; BPF open and fstat read it back.
  #include "kernel/types.h"
  #include "kernel/stat.h"
  #include "kernel/fcntl.h"
  #include "user/user.h"

  #define SAY(t, c) do { if(c) printf(t "=yes\n"); else printf(t "=no\n"); } while(0)

  int
  main(void)
  {
    if(fork() == 0){
      struct stat st;
      SAY("MK: made", mkdir("mkd") == 0);
      SAY("MK: dup-refused", mkdir("mkd") == -1);
      SAY("MK: nested", mkdir("mkd/in") == 0);
      int fd = open("mkd", O_RDONLY);
      SAY("MK: open", fd >= 0);
      SAY("MK: is-dir", fd >= 0 && fstat(fd, &st) == 0 && st.type == T_DIR);
      close(fd);
      fd = open("mkd/in", O_RDONLY);
      SAY("MK: nested-open", fd >= 0);
      close(fd);
      SAY("MK: noparent-refused", mkdir("nope/in") == -1);
      exit(0);
    }
    wait(0);
    printf("MK: done\n");
    for(;;)
      ;
  }
  ```
- [ ] **Step 46** [W] ~2m: `crates/xv6-mbc/upstream/user/rmprobe.c`.
  ```c
  // SPDX-License-Identifier: GPL-3.0-or-later
  // rmprobe.c: S-STAGE-2B unlink rung.
  #include "kernel/types.h"
  #include "kernel/stat.h"
  #include "kernel/fcntl.h"
  #include "user/user.h"

  #define SAY(t, c) do { if(c) printf(t "=yes\n"); else printf(t "=no\n"); } while(0)

  int
  main(void)
  {
    if(fork() == 0){
      mkdir("rmd");
      mkdir("rmd/in");
      SAY("RM: nonempty-refused", unlink("rmd") == -1);
      SAY("RM: inner", unlink("rmd/in") == 0);
      SAY("RM: inner-gone", open("rmd/in", O_RDONLY) < 0);
      SAY("RM: outer", unlink("rmd") == 0);
      SAY("RM: outer-gone", open("rmd", O_RDONLY) < 0);
      SAY("RM: missing-refused", unlink("nope") == -1);
      SAY("RM: dot-refused", unlink(".") == -1);
      SAY("RM: dotdot-refused", unlink("..") == -1);
      exit(0);
    }
    wait(0);
    printf("RM: done\n");
    for(;;)
      ;
  }
  ```
- [ ] **Step 47** [W] ~2m: `crates/xv6-mbc/upstream/user/lnprobe.c`.
  ```c
  // SPDX-License-Identifier: GPL-3.0-or-later
  // lnprobe.c: S-STAGE-2B link rung.
  #include "kernel/types.h"
  #include "kernel/stat.h"
  #include "kernel/fcntl.h"
  #include "user/user.h"

  #define SAY(t, c) do { if(c) printf(t "=yes\n"); else printf(t "=no\n"); } while(0)

  int
  main(void)
  {
    if(fork() == 0){
      char a[4], b[4];
      int fd = open("README", O_RDONLY);
      read(fd, a, 4);
      close(fd);
      SAY("LN: made", link("README", "lnk") == 0);
      SAY("LN: dup-refused", link("README", "lnk") == -1);
      SAY("LN: missing-refused", link("nope", "x") == -1);
      SAY("LN: dir-refused", link("sub", "subl") == -1);
      fd = open("lnk", O_RDONLY);
      SAY("LN: open", fd >= 0);
      SAY("LN: same-bytes", fd >= 0 && read(fd, b, 4) == 4 && memcmp(a, b, 4) == 0);
      close(fd);
      SAY("LN: unlinked", unlink("lnk") == 0);
      SAY("LN: gone", open("lnk", O_RDONLY) < 0);
      fd = open("README", O_RDONLY);
      SAY("LN: original-kept", fd >= 0);
      close(fd);
      exit(0);
    }
    wait(0);
    printf("LN: done\n");
    for(;;)
      ;
  }
  ```
- [ ] **Step 48** [W] ~2m: `crates/xv6-mbc/upstream/user/sleepprobe.c`.
  ```c
  // SPDX-License-Identifier: GPL-3.0-or-later
  // sleepprobe.c: S-STAGE-2B sleep rung. pause(n) means n UPC ticks (ADR-110).
  #include "kernel/types.h"
  #include "kernel/stat.h"
  #include "user/user.h"
  #include "user/uyield.h"

  #define SAY(t, c) do { if(c) printf(t "=yes\n"); else printf(t "=no\n"); } while(0)

  int
  main(void)
  {
    int a = uptime();
    SAY("SL: zero", pause(0) == 0);
    SAY("SL: neg", pause(-5) == 0);
    SAY("SL: returned", pause(300) == 0);
    SAY("SL: waited", uptime() - a >= 300);
    int c = fork();
    if(c == 0){
      pause(2000);
      printf("SL: child-woke\n");
      exit(0);
    }
    yield(); // the child is now inside pause(); this slot still runs
    printf("SL: parent-ran\n");
    wait(0);
    printf("SL: done\n");
    for(;;)
      ;
  }
  ```
- [ ] **Step 49** [W] ~2m: `crates/xv6-mbc/upstream/user/hostile_ptr.c`.
  ```c
  // SPDX-License-Identifier: GPL-3.0-or-later
  // hostile_ptr.c: S-STAGE-2B user-pointer rung. A child hands the kernel bad paths.
  #include "kernel/types.h"
  #include "kernel/stat.h"
  #include "user/user.h"

  #define SAY(t, c) do { if(c) printf(t "=yes\n"); else printf(t "=no\n"); } while(0)

  int
  main(void)
  {
    if(fork() == 0){
      volatile char *top = (volatile char *)0x007FFFF0;
      for(int i = 0; i < 16; i++)
        top[i] = 'x'; // no NUL before the ceiling
      SAY("HU: unterminated-refused", mkdir((char *)0x007FFFF0) == -1);
      SAY("HU: ceiling-refused", mkdir((char *)0x00800000) == -1);
      SAY("HU: kernel-refused", mkdir((char *)0x00F00000) == -1);
      SAY("HU: wrap-refused", mkdir((char *)0xFFFFFFF0) == -1);
      SAY("HU: alive", getpid() == 2);
      exit(0);
    }
    wait(0);
    printf("HU: done\n");
    for(;;)
      ;
  }
  ```
- [ ] **Step 50** [C] ~1m: `cmt "[PLAN S-STAGE-2B] Steps 41-50: rung probes written" crates/xv6-mbc/upstream/user/uyield.h crates/xv6-mbc/upstream/user/sbrkprobe.c crates/xv6-mbc/upstream/user/killprobe.c crates/xv6-mbc/upstream/user/mkprobe.c crates/xv6-mbc/upstream/user/rmprobe.c crates/xv6-mbc/upstream/user/lnprobe.c crates/xv6-mbc/upstream/user/sleepprobe.c crates/xv6-mbc/upstream/user/hostile_ptr.c`.
- [ ] **Step 51** [W] ~3m: Register them in `crates/xv6-mbc/adapters/Makefile.mbc-userland`. Four edits, each `REP-OK`.
  ```bash
  cd "$PROJECT_ROOT"; M=crates/xv6-mbc/adapters/Makefile.mbc-userland
  rep $M $'hostile_tf: $T/hostile_tf.mbc\n' $'hostile_tf: $T/hostile_tf.mbc\n# S-STAGE-2B rung probes.\nsbrkprobe: $T/sbrkprobe.mbc\nkillprobe: $T/killprobe.mbc\nmkprobe: $T/mkprobe.mbc\nrmprobe: $T/rmprobe.mbc\nlnprobe: $T/lnprobe.mbc\nsleepprobe: $T/sleepprobe.mbc\nhostile_ptr: $T/hostile_ptr.mbc\n'
  rep $M '             $T/uptimeprobe.mbc $T/hostile_pgd.mbc $T/hostile_slot.mbc $T/hostile_tf.mbc' $'             $T/uptimeprobe.mbc $T/hostile_pgd.mbc $T/hostile_slot.mbc $T/hostile_tf.mbc \\\n             $T/sbrkprobe.mbc $T/killprobe.mbc $T/mkprobe.mbc $T/rmprobe.mbc $T/lnprobe.mbc \\\n             $T/sleepprobe.mbc $T/hostile_ptr.mbc'
  rep $M '$T/uptimeprobe.* $T/hostile_pgd.* $T/hostile_slot.* $T/hostile_tf.* \' '$T/uptimeprobe.* $T/hostile_pgd.* $T/hostile_slot.* $T/hostile_tf.* $T/sbrkprobe.* $T/killprobe.* $T/mkprobe.* $T/rmprobe.* $T/lnprobe.* $T/sleepprobe.* $T/hostile_ptr.* \'
  rep $M '.PHONY: clean init uinit sh ' '.PHONY: clean sbrkprobe killprobe mkprobe rmprobe lnprobe sleepprobe hostile_ptr init uinit sh '
  ```
- [ ] **Step 52** [BUILD][B] ~3m: `gbuild; ls "$K"/{sbrkprobe,killprobe,mkprobe,rmprobe,lnprobe,sleepprobe,hostile_ptr}.mbc | wc -l; sha256sum "$K"/sh.mbc "$K"/ls.mbc | cut -c1-16 | tr '\n' ' '`. Expected: a last make line with no `Error`, `7`, two hashes (record in `$LOG`). `error:` from gcc: `[STUCK]`, paste it; do not edit the source.
- [ ] **Step 53** [C] ~1m: `cmt "[PLAN S-STAGE-2B] Steps 51-53: rung probes registered and built" crates/xv6-mbc/adapters/Makefile.mbc-userland references/s-stage-2b-log.md`.
- [ ] **Step 54** [V] ~1m: **PHASE 4 EXIT GATE**: `git status --short | wc -l` → `0`; `bash scripts/check-spdx.sh 2>&1 | tail -1` passes.

---

## PHASE 5: HARNESS GATES (Steps 55-59)

**Goal**: every rung has a harness gate, off until its const flips, so nothing here can fail the harness. The harness runs as root: rule 13 keeps this phase off the cheaper models.
**Prerequisite**: Phase 4.
**Time**: ~25 min
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 55** [W][S] ~3m: Add the gates to `scripts/upc-regression.sh`, right before the D6 block.
  ```bash
  python3 - <<'PY'
  p = "scripts/upc-regression.sh"
  s = open(p).read()
  a = "# ADR-109 D6 (M-247): a child stores to slot 0's pgd"
  assert s.count(a) == 1
  b = r'''# S-STAGE-2B rungs: each gate runs when its const is on; off, it is a note.
  rung_on() { grep -q "^const $1: bool = true;" "$EBPF/monad-cpu-ebpf/src/main.rs"; }
  probe_lines() { # $1 program, $2 triggers, $3 token prefix
    (cd "$EBPF" && sudo ip netns exec upc0 env UPC_DEMO_MODE=1 "$BC" boot --kernel "$K/xv6-mbc.mbc" \
      --ramdisk "$K/fs.img" --userland "$K/$1.mbc" --triggers "$2" --instance 111 2>&1) | grep -a 'ascii:' \
      | sed -E 's/([0-9A-F]{4} )+//g' | sed 's/\xc2\xb7/\n/g' | grep -aoE "$3: [a-z0-9-]+(=[a-z0-9]+)?"; }
  rung_gate() { # $1 const, $2 program, $3 triggers, $4 prefix, $5 how many =yes lines
    if ! rung_on "$1"; then note "$2 gate skipped: $1 is off"; return; fi
    local l; l="$(probe_lines "$2" "$3" "$4")"
    if [ "$(grep -c '=yes$' <<<"$l")" = "$5" ] && ! grep -q '=no$' <<<"$l" && grep -qx "$4: done" <<<"$l"; then
      pass "$2: $5 checks ($1)"; else fail "$2 ($1)"; note "$(tr '\n' ' ' <<<"$l")"; fi; }
  rung_gate UPC_RUNG_SBRK sbrkprobe 4000000 SB 13
  rung_gate UPC_RUNG_KILL killprobe 3000000 KL 12
  rung_gate UPC_RUNG_MKDIR mkprobe 4000000 MK 7
  rung_gate UPC_RUNG_UNLINK rmprobe 4000000 RM 8
  rung_gate UPC_RUNG_LINK lnprobe 4000000 LN 9
  rung_gate UPC_RUNG_MKDIR hostile_ptr 3000000 HU 5
  if rung_on UPC_RUNG_SLEEP; then
    SLL="$(probe_lines sleepprobe 3000000 SL)"
    if [ "$(grep -c '=yes$' <<<"$SLL")" = 4 ] && ! grep -q '=no$' <<<"$SLL" \
      && [ "$(grep -n 'parent-ran' <<<"$SLL" | cut -d: -f1)" -lt "$(grep -n 'child-woke' <<<"$SLL" | cut -d: -f1)" ]; then
      pass "sleepprobe: pause(n) waits, a sibling runs meanwhile (UPC_RUNG_SLEEP)"; else fail "sleepprobe (UPC_RUNG_SLEEP)"; fi
  else
    note "sleepprobe gate skipped: UPC_RUNG_SLEEP is off"
  fi

  '''
  import textwrap
  open(p, "w").write(s.replace(a, textwrap.dedent(b) + a))
  print("HARNESS-OK")
  PY
  bash -n scripts/upc-regression.sh && echo syntax-ok; grep -c 'rung_gate UPC_RUNG' scripts/upc-regression.sh
  ```
  Expected: `HARNESS-OK`, `syntax-ok`, `6`. Then `for f in sbrkprobe killprobe mkprobe rmprobe lnprobe hostile_ptr; do grep -c 'SAY("' crates/xv6-mbc/upstream/user/$f.c; done | tr '\n' ' '` → `13 12 7 8 9 5`, the counts in the `rung_gate` lines. Other numbers: `[STUCK]`.
- [ ] **Step 56** [TEST][B][S] ~15m: `gate probes`. Harness notes must show the skips: `grep -ac 'gate skipped: UPC_RUNG_' "$S/probes-harness.txt"`.
- [ ] **Step 57** [V] ~1m: `same=12 diff=0 rc=0`, `ascend` equal to Stop A's count, and the skip count `7`. Any `DIFF`: `[STUCK]`, paste the names into `$PROG`. Do not recapture a golden.
- [ ] **Step 58** [C] ~1m: `cmt "[PLAN S-STAGE-2B] Steps 54-58: rung gates in the harness, behind their consts" scripts/upc-regression.sh references/s-stage-2b-log.md`.
- [ ] **Step 59** [V] ~1m: **PHASE 5 EXIT GATE**: tree clean; `git rev-parse HEAD > "$S/last-green"`.

---
## PHASE 6: DP-413 — MINT SYS_YIELD (Steps 60-74)

**Goal**: yield has its own number; every gate that wanted a yield calls it; `pause` is free to mean pause.
**Prerequisite**: Phase 4.
**Time**: ~1 h
**Agent**: Coordinator
**Model**: Opus
**Rollback**: `git restore --source="$(cat "$S/last-green")" --staged --worktree -- ebpf crates cmd scripts docs/policy`, then `kbuild; gbuild; ebuild; bbuild; gate rollback-6`.

- [ ] **Step 60** [V][S] ~1m: **BEFORE GATE**: `git diff --stat "$(cat "$S/last-green")" HEAD -- ebpf crates/xv6-mbc scripts cmd | wc -l` → `0`, else `gate p413-before`. `keep offv "$(vcount)"`.
- [ ] **Step 61** [CODE][W] ~10m: The number. `crates/xv6-mbc/upstream/kernel/syscall.h`: add `#define SYS_yield  23` after `SYS_close` with the comment `// BPF only; 22 is the probe-only inject number`. `$MAIN`, `kernel_routed_alias`: add `23 if cfg!(feature = "ascend-linux") => 13, // SYS_yield shares the yield arm (ADR-078 §5)`. The cfg keeps the Doom object byte-equal, since the alias runs in both builds. After the `syscall_nr == 14` arm add an empty arm `} else if cfg!(feature = "ascend-linux") && syscall_nr == 23 {` with a comment saying the alias means it is never reached (`parity_surface` needs the arm). In the arm for 13, drop the "Phase-3 TODO: split a dedicated SYS_yield" lines and say 13 yields until the sleep rung.
- [ ] **Step 62** [CODE][W] ~5m: Surface: `ebpf/monad-cpu-ebpf/surface.rs` adds 23 to `SURFACE_ASCEND` and its doc comment. `cmd/upc-bootctl/src/main.rs`: `XV6_SYSCALLS` grows from 16 to 17 with `number: 23, name: "yield"`, in number order.
- [ ] **Step 63** [TEST][B] ~4m: `(cd cmd/upc-bootctl && cargo test --release --test parity_surface 2>&1 | grep '^test result' | sort | uniq -c); (cd cmd/upc-bootctl && cargo test --release 2>&1 | grep -E '^test result' | sort | uniq -c)`.
- [ ] **Step 64** [V] ~30s: `0 failed` on every line. A surface-id mismatch in a bootctl unit test means `ascend_facts()` needs the new id: it derives from surface.rs, so read the failure before editing.
- [ ] **Step 65** [CODE][W] ~5m: Callers. `crates/xv6-mbc/upstream/user/gate_nway.c`: add `#include "user/uyield.h"` after the `user/user.h` include, replace both `pause(0);` lines in the verify section with `yield();`, and change the header comment's "cooperatively yielding via pause()" to "cooperatively yielding via yield()" and "A second pause() is added" to "A second yield() is added". `getpidmulti.c`: same include, the three `pause(1);` become `yield();`, and its header comment follows. `uptimeprobe.c` keeps `pause(1)`: it is meant to wait. Check: `grep -ln 'pause(' "$UP"/user/*.c` lists dorphan, grind, forphan, uptimeprobe, sleepprobe, usertests and zombie; of those, only uptimeprobe and sleepprobe are in `GATE_PROGS`.
- [ ] **Step 66** [BUILD][B][S] ~6m: `ebuild; bbuild; gbuild; keep nv "$(vcount)"; echo "off=$offv new=$nv"; budget "$nv"`.
- [ ] **Step 67** [V] ~30s: delta within ±500 and `BUDGET-OK`. Over: Step 68.
- [ ] **Step 68** [D] ~10m: The alias adds a compare inside the execute loop. Move the 23 test into the existing arm for 13 as `syscall_nr == 13 || syscall_nr == 23` and teach `parity_surface` the `||`, or keep the alias and measure which is cheaper. Record both numbers.
- [ ] **Step 69** [TEST][B][S] ~3m: `ub gate_nway | grep -aoE 'NWAY-ok pid[123]' | sort -u | wc -l; ub getpidmulti | pl GM | sort | uniq -c | wc -l`.
- [ ] **Step 70** [V] ~30s: `3`, and `13`.
- [ ] **Step 71** [TEST][B][S] ~15m: **AFTER GATE**: `gate p413-after`.
- [ ] **Step 72** [V] ~1m: `same=12 diff=0 rc=0`. A `DIFF` on `ls` means a corpus binary changed size: the stub must stay out of `usys.pl`. Never recapture here.
- [ ] **Step 73** [C][S] ~2m: `ledger 'SYS_yield 23 (DP-413)' "$offv" "$nv" p413-after; insrow "$nv"; bash scripts/check-ebpf-insns.sh 2>&1 | tail -1; forstevie "DP-413 grew the ascend baseline row by $((nv-offv)): not a rung, so DP-717 does not name it; net vs 921036 is $((921036-nv))."`, then `cmt "[PLAN S-STAGE-2B] Steps 59-73: SYS_yield minted; pause no longer overloaded by its callers (DP-413)" crates/xv6-mbc/upstream/kernel/syscall.h ebpf/monad-cpu-ebpf/src/main.rs ebpf/monad-cpu-ebpf/surface.rs cmd/upc-bootctl/src/main.rs crates/xv6-mbc/upstream/user/gate_nway.c crates/xv6-mbc/upstream/user/getpidmulti.c docs/policy/ebpf-insns-baseline.tsv references/s-stage-2b-log.md`.
- [ ] **Step 74** [V] ~30s: **PHASE 6 EXIT GATE**: tree clean; `git rev-parse HEAD > "$S/last-green"`.

---

## PHASE 7: THE SYSCALL REGISTRY IN ADR-078 (Steps 75-77)

**Goal**: ADR-078 carries the number registry DP-413 asked for. Its §3 registry is the a2/a3 re-entry table, so this is a new §5.
**Prerequisite**: Phase 6.
**Time**: ~5 min
**Agent**: one agent, serial, no boot
**Model**: Haiku

- [ ] **Step 75** [W] ~2m: Append to `docs/adr/ADR-078-gate-d-fs-reader-and-exec-argv-abi.md`.
  ```bash
  cd "$PROJECT_ROOT"; cat >> docs/adr/ADR-078-gate-d-fs-reader-and-exec-argv-abi.md <<'TXT'

  ## 5. Syscall number registry (DP-413, 2026-11-04)

  One table for the ascend ecall surface. `surface.rs` and bootctl's `XV6_SYSCALLS` carry the same numbers; `parity_surface` holds them to the dispatch. A number with no row answers `-ENOSYS` (DP-503). Rungs move rows from BPF to kernel; a number is never reused.

  | Number | Name | Served by | Note |
  |---|---|---|---|
  | 1 | fork | BPF | |
  | 2 | exit | BPF | |
  | 3 | wait | BPF | status written as 0 until rung 7 |
  | 4 | pipe | none yet | |
  | 5 | read | BPF | |
  | 6 | kill | none yet | |
  | 7 | exec | BPF | |
  | 8 | fstat | BPF | |
  | 9 | chdir | none yet | |
  | 10 | dup | BPF | |
  | 11 | getpid | kernel | ADR-109 |
  | 12 | sbrk | none yet | |
  | 13 | pause | BPF | yields until the sleep rung, then waits n ticks |
  | 14 | uptime | kernel | ADR-110 |
  | 15 | open | BPF | |
  | 16 | write | BPF | |
  | 17 | mknod | BPF | stub |
  | 18 | unlink | none yet | |
  | 19 | link | none yet | |
  | 20 | mkdir | none yet | |
  | 21 | close | BPF | |
  | 22 | reserved | probe builds | `UPC_PROBE_INJECT22` |
  | 23 | yield | BPF | DP-413; a stub in `user/uyield.h`, not `usys.pl`, so corpus binaries keep their size |
  | 200, 201 | read block, write block | BPF | kernel-only, `monad_common::mbc_linux_syscalls` |
  TXT
  grep -c '^| 23 | yield' docs/adr/ADR-078-gate-d-fs-reader-and-exec-argv-abi.md
  ```
  Expected: `1`.
- [ ] **Step 76** [W] ~1m: Index row note.
  ```bash
  grep -n '^| \[078\]\|^| \[78\]' docs/adr/ADR-INDEX.md | cut -c1-80
  ```
  Expected: one line. Add nothing else to the index; the row's title still holds. No line: `[STUCK]`.
- [ ] **Step 77** [C] ~1m: `cmt "[PLAN S-STAGE-2B] Steps 74-77: ADR-078 gains the syscall number registry" docs/adr/ADR-078-gate-d-fs-reader-and-exec-argv-abi.md`. **PHASE 7 EXIT GATE**: tree clean.

---

## PHASE 8: DP-503 — KERNEL BUILDS ANSWER -ENOSYS (Steps 78-96)

**Goal**: the 23 fabricated calls sit behind `fuzix-permissive` in both interpreters, the Doom object stays byte-equal, and no ascend call returns a stale register.
**Prerequisite**: Phase 6.
**Time**: ~1.5 h
**Agent**: Coordinator
**Model**: Opus
**Rollback**: `git restore --source="$(cat "$S/last-green")" --staged --worktree -- ebpf crates cmd scripts docs/policy`, then `kbuild; gbuild; ebuild; bbuild; gate rollback-8`.

The set, from both chains: OPEN, CLOSE, GETUID, GETGID, GETEUID, GETEGID, SETUID, SETGID, DUP, DUP2, KILL, STAT, FSTAT, LSEEK, GETPPID, UMASK, SYNC, TIMES, IOCTL, PIPE, FCNTL, ACCESS, CHDIR. SIGNAL (a real table), TIME, BRK and the block calls do real work and stay.

- [ ] **Step 78** [V][S] ~1m: **BEFORE GATE** as in Phase 6; `keep offv "$(vcount)"`.
- [ ] **Step 79** [TEST][W] ~10m: Test first. `crates/monad-mbc/tests/permissive_set.rs`, with SPDX header, helpers `enc` and the run loop copied from `tests/os_primitives_test.rs`. `is_permissive` holds for exactly those 23 and not for EXIT, READ, WRITE, BRK, GETPID, FORK, SIGNAL, TIME, READ_BLOCK, WRITE_BLOCK. With `#[cfg(feature = "fuzix-permissive")]`, `INT 0x80` with SYS_OPEN leaves `regs[0] == 3`. With `#[cfg(not(...))]`, it leaves `regs[0] == (-38i32) as u32`.
- [ ] **Step 80** [TEST][B] ~2m: `(cd crates/monad-mbc && cargo test --release --test permissive_set 2>&1 | grep -E 'error\[|cannot find' | head -3)`.
- [ ] **Step 81** [V] ~30s: RED: `cannot find function is_permissive`.
- [ ] **Step 82** [CODE][W] ~10m: `ebpf/monad-common/src/lib.rs`, inside `mbc_linux_syscalls`: `pub const fn is_permissive(n: u32) -> bool` with a `matches!` over the 23 and a doc comment citing DP-503. `crates/monad-mbc/Cargo.toml`: `[features]`, `default = ["fuzix-permissive"]`, `fuzix-permissive = []`. `crates/xv6-mbc/Cargo.toml`: `monad-mbc = { path = "../monad-mbc", default-features = false }`. `crates/monad-mbc/src/execute.rs`: the INT 0x80 chain head `if syscall_nr == lsys::SYS_EXIT {` becomes `if !cfg!(feature = "fuzix-permissive") && lsys::is_permissive(syscall_nr) { self.state.regs[0] = (-(lsys::ENOSYS as i32)) as u32; } else if syscall_nr == lsys::SYS_EXIT {`.
- [ ] **Step 83** [TEST][B] ~5m: `(cd crates/monad-mbc && cargo test --release 2>&1 | grep -E '^test result' | sort | uniq -c; cargo test --release --no-default-features 2>&1 | grep -E '^test result' | sort | uniq -c)`.
- [ ] **Step 84** [V] ~30s: `0 failed` both ways. A failure under `--no-default-features` is a test that leaned on a fake: mark it `#[cfg(feature = "fuzix-permissive")]`, name it in `$LOG`.
- [ ] **Step 85** [C] ~1m: `cmt "[PLAN S-STAGE-2B] Steps 78-85: permissive set named; monad-mbc answers -ENOSYS without the feature" ebpf/monad-common/src/lib.rs crates/monad-mbc/Cargo.toml crates/monad-mbc/src/execute.rs crates/monad-mbc/tests/permissive_set.rs crates/xv6-mbc/Cargo.toml`.
- [ ] **Step 86** [CODE][W] ~15m: `ebpf/monad-cpu-ebpf/Cargo.toml`: `default = ["fuzix-permissive"]`, `fuzix-permissive = []` with a comment naming Doom and the FUZIX demos. `$MAIN`, same head edit on `if syscall_nr == lsys::SYS_EXIT {` in the INT arm, writing `cpu.regs[0]`. Then the kernel-build answers: (1) before `if !cfg!(feature = "ascend-linux") && vector == intr::VECTOR_SYSCALL {`, add `if cfg!(feature = "ascend-linux") && vector == intr::VECTOR_SYSCALL { cpu.regs[0] = (-(lsys::ENOSYS as i32)) as u32; } else ` so an ascend INT 0x80 no longer falls into IVT dispatch; (2) end the ecall chain with `else if cfg!(feature = "ascend-linux") { cpu.regs[8] = (-(lsys::ENOSYS as i32)) as u32; }`. Keep the comment line after it starting `// Unknown syscall: silently ignore`, exactly that prefix: `parity_surface.rs:65` finds the end of the chain by it. Make it `// Unknown syscall: silently ignore (default build only; fail-safe).` The default build keeps its silent ignore: Doom never issues one, and the ask is kernel builds.
- [ ] **Step 87** [CODE][W] ~5m: The empty arms for 14 and 23 stay; with `UPC_RUNG2` off the arm for 14 should answer `-ENOSYS`, not stale a0: set `cpu.regs[8]` there. Update the comments that say "silently ignore" in `surface.rs` and in the `XV6_SYSCALLS` doc comment in `cmd/upc-bootctl/src/main.rs`. `(cd cmd/upc-bootctl && cargo test --release --test parity_surface 2>&1 | grep '^test result' | sort | uniq -c)`; a bare `else if cfg!` arm may trip the parser: teach it to skip arms with no `syscall_nr ==`, as rung 2 did for the chain head.
- [ ] **Step 88** [BUILD][B][S] ~8m: `ebuild; bbuild; keep nv "$(vcount)"; echo "off=$offv new=$nv"; budget "$nv"; (cd "$EBPF" && cargo build --release -p monad-cpu-ebpf --no-default-features --target-dir target/honest 2>&1 | tail -1)`.
- [ ] **Step 89** [V] ~30s: the delta is small (bar +1,500) and `BUDGET-OK`, and the honest non-ascend object compiles. It is compile-checked only: nothing ships it. Over the bar: [DECIDE] drop the ascend INT arm first (no guest issues it, RECOMMENDATION keep the ecall arm regardless); Override ONLY if the ecall arm alone is over the bar, then measure `cpu.regs[8]` set before dispatch instead.
- [ ] **Step 90** [TEST][B][S] ~3m: `TRIG=3000000 ub sbrkprobe | pl SB | head -4 | tr '\n' ' '`.
- [ ] **Step 91** [V] ~30s: `SB: init-refused=no` and `SB: base=no`: sbrk now returns -38, not its argument. That is this phase's RED for the sbrk rung. Record it.
- [ ] **Step 92** [C] ~1m: `cmt "[PLAN S-STAGE-2B] Steps 86-92: kernel builds answer -ENOSYS in the object" ebpf/monad-cpu-ebpf/Cargo.toml ebpf/monad-cpu-ebpf/src/main.rs ebpf/monad-cpu-ebpf/surface.rs cmd/upc-bootctl/src/main.rs cmd/upc-bootctl/tests/parity_surface.rs`.
- [ ] **Step 93** [TEST][B][S] ~15m: **AFTER GATE**: `gate p503-after; grep -aoE 'Doom verifier: +verified [0-9]+' "$S/p503-after-harness.txt"`.
- [ ] **Step 94** [V] ~1m: `same=12 diff=0 rc=0` and Doom `758199`. Doom moved: the head guard is not short-circuiting; `cfg!(...)` must be the left operand.
- [ ] **Step 95** [C][S] ~2m: `ledger 'honest ENOSYS (DP-503)' "$offv" "$nv" p503-after; insrow "$nv"; bash scripts/check-ebpf-insns.sh 2>&1 | tail -1; forstevie "DP-503 grew the ascend baseline row by $((nv-offv)): not a rung, so DP-717 does not name it; net vs 921036 is $((921036-nv))."`, then `cmt "[PLAN S-STAGE-2B] Steps 93-95: DP-503 gated; baseline row" docs/policy/ebpf-insns-baseline.tsv references/s-stage-2b-log.md docs/battle-plans/progress/S-STAGE-2B-progress.md`.
- [ ] **Step 96** [V] ~30s: **PHASE 8 EXIT GATE**: tree clean; `git rev-parse HEAD > "$S/last-green"`; `git diff "$(git log --format=%H -4 | tail -1)" --stat -- crates/monad-mbc/Cargo.lock ebpf/Cargo.lock | wc -l` → `0` (no dependency moved).

---

## PHASE 9: DP-414 — SIDECAR ONLY, RV BASE IN BOOTPARAMS (Steps 97-114)

**Goal**: one source for the RV text base, checked by the kernel at boot; a missing `.data` sidecar refuses a live boot; a frozen-ABI check exists.
**Prerequisite**: Phase 8.
**Time**: ~1.5 h
**Agent**: Coordinator
**Model**: Opus
**Rollback**: `git restore --source="$(cat "$S/last-green")" --staged --worktree -- ebpf crates cmd scripts docs/policy`, then `kbuild; gbuild; ebuild; bbuild; gate rollback-9`.

**Frozen-ABI check** (no such gate exists today; this phase makes it `cmd/upc-bootctl/tests/parity_bootparams.rs` and checks the rest by hand at Step 110). Boot Protocol v2 is frozen by ADR-067 Decision 7. Consuming a `reserved` word is its stated compatible extension, as ADR-109 did with `reserved[0]`. Version stays 3. The Monad wire format is not involved, so the RFC desk is not.

- [ ] **Step 97** [V][S] ~1m: **BEFORE GATE** as in Phase 6; `keep offv "$(vcount)"`.
- [ ] **Step 98** [TEST][W] ~15m: `cmd/upc-bootctl/tests/parity_bootparams.rs` (copy the file-reading shape of `parity_slot_tables.rs`): parse the field names of `struct boot_params_v2` out of `crates/xv6-mbc/adapters/start_mbc.c` and `crates/upc-bootstub/src/start.c`, and of `BootParamsV2` out of `src/bootparams.rs`; assert the three orders agree (types ignored); assert `rv2mbc_sha256` at 64 and `reserved` at 96; assert `RV_TEXT_BASE_RESERVED_IDX == 1` and byte `0x100 + 96 + 4 == 0x164`; assert `XV6_TEXT_RV_WORD_BASE == 0x8000`, and that `kernel-mbc.ld` still says `.text 0x00020000`, so `0x20000 >> 2` equals it.
- [ ] **Step 99** [TEST][B] ~2m: `(cd cmd/upc-bootctl && cargo test --release --test parity_bootparams 2>&1 | grep -E 'error|panicked' | head -3)`.
- [ ] **Step 100** [V] ~30s: RED on the two new constants; the field-order asserts already hold.
- [ ] **Step 101** [CODE][W] ~15m: `cmd/upc-bootctl/src/bootparams.rs`: `pub const XV6_TEXT_RV_WORD_BASE: u32 = 0x8000;`, `pub const RV_TEXT_BASE_RESERVED_IDX: usize = 1;`. `for_xv6` builds `reserved` with `r[RV_TEXT_BASE_RESERVED_IDX] = XV6_TEXT_RV_WORD_BASE`. Extend its unit test, and note the word in the struct comment. `cmd/upc-bootctl/src/main.rs`: `let text_rv_word_base: u32 = bootparams::XV6_TEXT_RV_WORD_BASE;`, and use it wherever the kernel's RV2MBC base is declared (`grep -n '0x8000' cmd/upc-bootctl/src/main.rs`: RV-word uses change; `populate_rom_at(0x8000, ...)` is a ROM slot and stays). The comment "Phase 2 will pass a different base for the bootstub flow" goes.
- [ ] **Step 102** [CODE][W] ~10m: Kernel. `crates/xv6-mbc/adapters/kernel-mbc.ld`: `PROVIDE(_kernel_text = .);` first thing inside `.text`. `start_mbc.c`, after the version check: `extern char _kernel_text[]; uint32 base = bp->reserved[1]; if (base != 0 && base != ((uint32)_kernel_text >> 2)) { mmio_puts("BOOT FAIL: rv text base\n"); ebreak; while(1){} }` in the file's existing fail shape. 0 passes, so a v3 loader from before this change still boots. It prints nothing on success: a new TTY line would move every golden.
- [ ] **Step 103** [CODE][W] ~10m: Sidecar (DP-414 `.rodata`). `cmd/upc-bootctl/src/main.rs`, kernel data image: both `println!("  ⚠ ...")` branches become `bail!`; a live boot with no or a bad `.data` sidecar is refused. Bootstub sidecar: same for `load_data_image` failure, and a missing `bs_data_path` is refused. The translator stays the only source of `.rodata`; nothing else stages it (`grep -n 'rodata' cmd/upc-bootctl/src/main.rs` shows comments and these sites only). Check `upc-bootctl boot --dry-run` never reaches this code: it returns before `populate_ram`, or the bail is skipped when `dry_run`.
- [ ] **Step 104** [BUILD][B][S] ~6m: `kbuild; gbuild; bbuild; (cd cmd/upc-bootctl && cargo test --release 2>&1 | grep -E '^test result' | sort | uniq -c); keep nv "$(vcount)"; echo "off=$offv new=$nv"`.
- [ ] **Step 105** [V] ~30s: `0 failed` everywhere; `nv` equals `offv` exactly: this phase does not touch the object.
- [ ] **Step 106** [TEST][B][S] ~4m: Mutation, then revert: in `bootparams.rs` set `XV6_TEXT_RV_WORD_BASE` to `0x8001`; `bbuild; TRIG=3000 ub getpidfork | grep -ac 'BOOT FAIL: rv text base'`; restore it; `git diff --stat cmd/upc-bootctl/src/bootparams.rs` shows only this phase's edits; `bbuild`.
- [ ] **Step 107** [V] ~30s: `1` or more with the mutant (the parity test would also catch it; the boot proves the kernel check bites). `0`: the check is dead, read `start_mbc.c`'s order against the MRET.
- [ ] **Step 108** [TEST][B][S] ~2m: Sidecar refusal: `mv "$K/getpidfork.data" "$S/gp.data"; mv "$K/xv6-mbc.data" "$S/k.data"; TRIG=3000 ub getpidfork | tail -2; mv "$S/k.data" "$K/xv6-mbc.data"; mv "$S/gp.data" "$K/getpidfork.data"`. Expected: an error naming `.data`, exit before attach. `ub getpidfork | pl F | head -1` works again after.
- [ ] **Step 109** [W] ~5m: Docs.
  ```bash
  rep docs/doom/UPC_BOOT_PROTOCOL_V2.md '| 100..256 | `reserved[1..40]` | Zero |' $'| 100..104 | `reserved[1]` | RV text word base (DP-414): the loader writes it, 0x8000 for xv6. `start_mbc.c` refuses a non-zero value that differs from `_kernel_text >> 2`; 0 means not checked |\n| 104..256 | `reserved[2..40]` | Zero |'
  cat >> docs/adr/ADR-067-mbc-isa-v2-and-upc-abi-v1.md <<'TXT'
  - DP-414: the loader stages only the translator's `.data` sidecar; the RV text word base travels in BootParams `reserved[1]`, a compatible use of the reserved tail. Version stays 3.
  TXT
  ```
  `reserved[0]` stays the only kernel-written word; `reserved[1]` is loader-written. Say so in the §v3 "Kernel-written word" paragraph.
- [ ] **Step 110** [V] ~5m: **FROZEN-ABI CHECK**: `parity_bootparams` green; version still 3 (`grep -n 'BOOT_VERSION: u32 = 3' cmd/upc-bootctl/src/bootparams.rs`); offsets 0..100 unchanged (`git diff "$(cat "$S/last-green")" -- cmd/upc-bootctl/src/bootparams.rs | grep -c '^[-+] *pub [a-z_0-9]*:'` → `0`); `crates/upc-bootstub/src/start.c` still ignores `reserved`; Doom's v1 path untouched (`git diff --stat "$(cat "$S/last-green")" -- crates/doom-runner | wc -l` → `0`). Write the result in `$LOG`.
- [ ] **Step 111** [TEST][B][S] ~15m: **AFTER GATE**: `gate p414-after`.
- [ ] **Step 112** [V] ~1m: `same=12 diff=0 rc=0`, count unchanged.
- [ ] **Step 113** [C] ~2m: `ledger 'BootParams base, sidecar (DP-414)' "$offv" "$nv" p414-after`, then `cmt "[PLAN S-STAGE-2B] Steps 96-113: RV text base travels in BootParams; sidecar-only staging; frozen-ABI parity test (DP-414)" cmd/upc-bootctl/src/bootparams.rs cmd/upc-bootctl/src/main.rs cmd/upc-bootctl/tests/parity_bootparams.rs crates/xv6-mbc/adapters/kernel-mbc.ld crates/xv6-mbc/adapters/start_mbc.c docs/doom/UPC_BOOT_PROTOCOL_V2.md docs/adr/ADR-067-mbc-isa-v2-and-upc-abi-v1.md references/s-stage-2b-log.md`.
- [ ] **Step 114** [V][STOP] ~30s: **PHASE 9 EXIT GATE, STOP B**: tree clean; `git rev-parse HEAD > "$S/last-green"`. `echo $(( (DEADLINE-$(date +%s))/60 ))` minutes left: under `180`, go straight to Phase 22 (nothing has landed for Phase 21 to attack).

---

## PHASE 10: THE KERNEL'S REPLY CHANNEL — ADR FIRST (Steps 115-121)

**Goal**: one reviewed design for what the rungs share: how the kernel answers BPF, where the heap lives, how user pointers resolve, how a slot blocks, how a kill lands.
**Prerequisite**: Stop B. ADR-109 D5 says the first rung that needs more than rows writes its own ADR.
**Time**: ~1 h
**Agent**: Coordinator; one read-only review agent
**Model**: Opus

- [ ] **Step 115** [B] ~1m: The number, across every lane. ADR-125 to ADR-134 are reserved for S-WIRE-HONEST and S-G9-FOLD, so numbering starts at 135.
  ```bash
  { ls docs/adr; git for-each-ref refs/heads/sprint --format='%(refname)' | xargs -I{} git ls-tree --name-only {} docs/adr/ | xargs -n1 basename; } | grep -oE '^ADR-[0-9]+' | cut -d- -f2 | grep -vE '^(69420|12[5-9]|13[0-4])$' | sort -n | awk 'BEGIN{m=134} $1>m{m=$1} END{print m+1}' | tee "$S/adr-n"
  ```
- [ ] **Step 116** [DESIGN][W] ~30m: `docs/adr/ADR-$(cat "$S/adr-n")-upc-kernel-reply-channel.md`, **Status**: Proposed (implements DP-213, DP-501, DP-502), plus an `ADR-INDEX.md` row. Bare technical style. Decisions, as forged here; change one only with a recorded reason:
  - **D1 reply channel.** Three new table words after `ticks`: `brk_in` +0x810 (BPF writes at injection), `reply` +0x814, `reply_arg` +0x818, pad +0x81C; `UPC_ST_SIZE` 0x820. The kernel writes `UPC_TAG_REPLY (0x5250_0000) | flags`. BPF reads it in the SRET arm when `spp == 0`, clears it, acts. Flags: bit 0 SET_BRK, bit 1 YIELD, bit 2 KILL. One reply per trap. I7 holds: same process, U-mode restored first.
  - **D2 heap window.** VA [0x10000, 0x40000) in every child: above the CSR window, inside fork's low copy band, so fork needs no change. A bigger heap needs a third copy band and its verifier cost.
  - **D3 break authority.** BPF keeps `cpu.program_break`, which fork and context switch already carry. The kernel reads `brk_in` at each trap and normalizes anything outside the window to its base, which covers exec's 0x400000 with no exec-arm change. Growth goes back through SET_BRK.
  - **D4 init has no heap.** Slot 0 is identity-mapped, and physical 0x10000-0x40000 holds kernel `.rodata`. `sbrk` in slot 0 returns -1.
  - **D5 zero on growth.** The slice holds whatever fork copied, kernel `.rodata` included, or a dead process's bytes. The kernel zeroes the grown range through the flat physical address. Lazy sbrk is treated as eager: no fault path exists.
  - **D6 user pointers.** ADR-109 D5 leaves this to the first rung that dereferences one. `walkaddr` and copyout's `walk` get a slot-row branch: `pa = slice_base(s) + va` when `va < 0x800000`, else 0. The BPF pgd stays unwalked. Valid only inside a tagged trap (`upc_slot_active()`), so boot-time xv6 paths keep today's behaviour.
  - **D7 retry hand-off.** The rule ADR-109 I7 sketches: a syscall that cannot finish leaves `epc` on the ecall, keeps a0, and replies YIELD. BPF yields at SRET and the ecall runs again when the slot is next scheduled. Polling, about 1,350 MBC instructions per retry, no `sleep()` on a slot row.
  - **D8 kill.** The kernel validates the pid range (2..8; init refused) and replies KILL with the slot. BPF checks liveness, writes a0 = -1 if dead, else does what exit does: reparent, mark halted, and if the target is the caller, exit. A target dies at once, not at its next kernel entry; with cooperative scheduling that is the stricter answer. Status stays 0 until rung 7.
  - **D9 row cwd.** A refreshed row gets `namei("/")` once and keeps it; `cwd` is not inherited until the fd rung.
- [ ] **Step 117** [V] ~1m: `grep -c "ADR-$(cat "$S/adr-n")-" docs/adr/ADR-INDEX.md` → `1`; every path the ADR names exists (`grep -oE '[a-z/_-]+\.(rs|c|h|S)' <file> | sort -u | xargs -I{} sh -c 'ls {} >/dev/null 2>&1 || git ls-files | grep -q {} || echo MISSING {}'` prints nothing).
- [ ] **Step 118** [C] ~1m: `cmt "[PLAN S-STAGE-2B] Steps 114-118: reply channel ADR (Proposed)" "docs/adr/ADR-$(cat "$S/adr-n")-upc-kernel-reply-channel.md" docs/adr/ADR-INDEX.md`.
- [ ] **Step 119** [R] ~20m: Review agent, read-only, no sudo, no build; the coordinator waits. Brief: "Read this ADR, ADR-109, `ebpf/monad-cpu-ebpf/src/main.rs` SRET and fork arms, `crates/xv6-mbc/adapters/upc_slots.c`. Return findings as a list with severity, no edits. Answer explicitly: can a child forge the reply word; does a retry double-apply anything; can a heap byte leak between processes; what breaks if a kill lands while the target's fork copy is mid-flight (parent's fork re-enters across ticks)."
- [ ] **Step 120** [W] ~10m: Apply dispositions in the ADR, and list each in `$LOG`. A blocker with no fix inside this design: [DECIDE] land what it does not touch. RECOMMENDATION: sbrk needs only D1-D5, so proceed; Override ONLY if the finding is against D1.
- [ ] **Step 121** [C] ~1m: `cmt "[PLAN S-STAGE-2B] Steps 119-121: reply channel ADR reviewed" "docs/adr/ADR-$(cat "$S/adr-n")-upc-kernel-reply-channel.md" references/s-stage-2b-log.md`. **PHASE 10 EXIT GATE**: every finding has a disposition; tree clean.

---

## PHASE 11: RUNG SBRK — REPLY CHANNEL AND THE HEAP (Steps 122-145)

**Goal**: `malloc` works in a forked child; the channel every later rung uses is in and measured on its own.
**Prerequisite**: Phase 10.
**Time**: ~2.5 h
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 122** [V][S] ~1m: **BEFORE GATE** as in Phase 6; `keep offv "$(vcount)"`; `budget "$offv"`.
- [ ] **Step 123** [CODE][W] ~15m: Constants. `crates/xv6-mbc/adapters/upc_slots.h` and `ebpf/monad-cpu-ebpf/src/phase12.rs`: the D1 offsets, `UPC_TAG_REPLY`, `UPC_REPLY_SET_BRK 1`, `UPC_REPLY_YIELD 2`, `UPC_REPLY_KILL 4`, `UPC_HEAP_BASE 0x00010000`, `UPC_HEAP_TOP 0x00040000`. Struct gains `uint32 brk_in, reply, reply_arg, pad;`. The `_Static_assert`s follow; `phase12.rs`'s size assert goes from `+ 16` to `+ 32`. `cmd/upc-bootctl/tests/parity_slot_tables.rs` gains the new names. `(cd cmd/upc-bootctl && cargo test --release --test parity_slot_tables 2>&1 | grep '^test result' | sort | uniq -c)` → `0 failed`.
- [ ] **Step 124** [CODE][W] ~15m: Kernel. `upc_slots.c`:
  ```c
  static int upc_active;           // inside a tagged trap
  int upc_slot_active(void) { return upc_active; }
  void upc_slot_leave(void) { upc_active = 0; }
  void upc_reply(uint32 flags, uint32 arg) { upc_st.reply_arg = arg; upc_st.reply = UPC_TAG_REPLY | flags; }

  // ADR D2-D5. uint64 is 4 bytes under ilp32e; every bound below is 32-bit.
  uint64
  upc_sbrk(int n)
  {
    uint32 s = myproc() - proc;
    if(s == 0)
      return -1;
    uint32 cur = upc_st.brk_in;
    if(cur < UPC_HEAP_BASE || cur > UPC_HEAP_TOP)
      cur = UPC_HEAP_BASE;
    if(n > 0 && (uint32)n > UPC_HEAP_TOP - cur)
      return -1;
    if(n < 0 && (n == -2147483647 - 1 || (uint32)-n > cur - UPC_HEAP_BASE))
      return -1;
    if(n > 0)
      memset((void *)(UPC_SLICE_BASE(s) + cur), 0, n);
    upc_reply(UPC_REPLY_SET_BRK, cur + n);
    return cur;
  }
  ```
  `UPC_SLICE_BASE(s)` = `(s) == 0 ? 0 : 0x01000000 + ((s) - 1) * 0x00800000` in `upc_slots.h`, added to `parity_slice_constants.rs`. `upc_slot_enter()`: set `upc_active = 1` on the tagged path and clear `reply` on entry. `usertrap` calls `upc_slot_leave()` before `prepare_return()`. `usysproc.c` `sys_sbrk`: after the `argint`s, `if(upc_slot_active()) return upc_sbrk(n);`. It must never reach `growproc`.
- [ ] **Step 125** [BUILD][B][S] ~4m: `kbuild; gbuild; ebuild; bbuild; [ "$(vcount)" = "$offv" ] && echo object-equal`. Kernel changes alone leave the object equal.
- [ ] **Step 126** [V] ~30s: `object-equal`.
- [ ] **Step 127** [C] ~1m: Const-off point: `cmt "[PLAN S-STAGE-2B] Steps 122-127: slot table words and kernel side of sbrk; nothing routes yet" crates/xv6-mbc/adapters/upc_slots.h crates/xv6-mbc/adapters/upc_slots.c crates/xv6-mbc/adapters/utrap.c crates/xv6-mbc/adapters/usysproc.c ebpf/monad-cpu-ebpf/src/phase12.rs cmd/upc-bootctl/tests/parity_slot_tables.rs cmd/upc-bootctl/tests/parity_slice_constants.rs`.
- [ ] **Step 128** [CODE][W] ~15m: BPF, `UPC_REPLY`. `$MAIN`: `const UPC_REPLY: bool = false;` with a comment citing the ADR. `inject_strap`: `if UPC_REPLY { mem_write_word((base + phase12::UPC_ST_BRK_IN_OFF) >> 2, cpu.program_break); }`. SRET arm, after `cpu.reservation_address = 0xFFFF_FFFF;`:
  ```rust
  if cfg!(feature = "ascend-linux") && UPC_REPLY && spp == 0 {
      let base = mem_read_word(phase12::UPC_SLOT_TABLES_BP_BYTE >> 2);
      if base != 0 {
          let w = (base + phase12::UPC_ST_REPLY_OFF) >> 2;
          let r = mem_read_word(w);
          if (r & phase12::UPC_TAG_MASK) == phase12::UPC_TAG_REPLY {
              mem_write_word(w, 0);
              if r & phase12::UPC_REPLY_SET_BRK != 0 {
                  cpu.program_break = mem_read_word((base + phase12::UPC_ST_REPLY_ARG_OFF) >> 2);
              }
          }
      }
  }
  ```
- [ ] **Step 129** [BUILD][B][S] ~4m: `ebuild; [ "$(vcount)" = "$offv" ] && echo off-equal; flip UPC_REPLY false true; keep rv "$(vcount)"; echo "reply=$rv delta=$((rv-offv))"; budget "$rv"; flip UPC_REPLY true false`. Measured on, left off.
- [ ] **Step 130** [DECIDE] ~5m: **Reply read in the SRET arm, or after the loop?** RECOMMENDATION: keep it in the arm if the delta is at most +3,000. Over that, SRET sets a local `reply_req` and breaks, and the block moves beside `DEFERRED_SWITCH`, outside the execute loop: variable RAM offsets inside the loop are what once cost +119K. Costs one short tick per trap return. Override ONLY if the post-loop form measures worse.
- [ ] **Step 131** [C] ~1m: Const-off point: `cmt "[PLAN S-STAGE-2B] Steps 128-131: reply channel in the object, const off" ebpf/monad-cpu-ebpf/src/main.rs references/s-stage-2b-log.md`.
- [ ] **Step 132** [CODE][W] ~5m: `UPC_RUNG_SBRK`. `const UPC_RUNG_SBRK: bool = false;`; alias `12 if cfg!(feature = "ascend-linux") && UPC_RUNG_SBRK => 11,`; empty arm `syscall_nr == 12` that answers `-ENOSYS` with the const off; `surface.rs` and `XV6_SYSCALLS` (18 entries) gain 12; `parity_surface` green; ADR-078 §5 row 12 becomes `| 12 | sbrk | kernel | heap window, ADR-$(cat "$S/adr-n") |`.
- [ ] **Step 133** [BUILD][B][S] ~4m: `ebuild; bbuild; [ "$(vcount)" = "$offv" ] && echo off-equal`.
- [ ] **Step 134** [V] ~30s: `off-equal`: both consts off, the object is the Phase start's.
- [ ] **Step 135** [C] ~1m: Const-off point: `cmt "[PLAN S-STAGE-2B] Steps 132-135: sbrk arm wired, const off" ebpf/monad-cpu-ebpf/src/main.rs ebpf/monad-cpu-ebpf/surface.rs cmd/upc-bootctl/src/main.rs docs/adr/ADR-078-gate-d-fs-reader-and-exec-argv-abi.md`.
- [ ] **Step 136** [CODE][B][S] ~5m: `flip UPC_REPLY false true; flip UPC_RUNG_SBRK false true; keep nv "$(vcount)"; echo "sbrk=$nv delta=$((nv-rv))"; budget "$nv"`.
- [ ] **Step 137** [V] ~30s: `BUDGET-OK`. Over the ceiling: flip both off, `[STUCK]`, read Appendix A1.
- [ ] **Step 138** [TEST][B][S] ~4m: `TRIG=4000000 ub sbrkprobe > "$S/sbrk-on.txt"; pl SB < "$S/sbrk-on.txt" | tee "$S/sbrk-lines.txt" | grep -c '=yes$'; grep -c '=no$' "$S/sbrk-lines.txt"; grep -c 'SB: done' "$S/sbrk-lines.txt"`.
- [ ] **Step 139** [V] ~30s: `13`, `0`, `1`. Else Step 140.
- [ ] **Step 140** [D] ~15m: `SB: init-refused=no`: alias not live, check the surface list and `kernel_routed_alias`. `SB: zeroed=no`: `UPC_SLICE_BASE` is wrong for the slot. `SB: moved=no`: SET_BRK lost: the reply cleared before the read, or `brk_in` written after a context switch. `SB: fork-copied=no`: the heap escaped the low band. Halt `0x48`: SRET landed on an unmapped word. Boots print the `ROM-FAULT CTX` line; Appendix A2.
- [ ] **Step 141** [SECURITY][TEST][B][S] ~5m: Cross-process leak: a child dirties its heap and exits, the next child on that slot grows and reads. `sbrkprobe` covers growth over fork-copied bytes; extend it only if the review found more. Mutation: comment out the `memset`, `kbuild; gbuild; TRIG=4000000 ub sbrkprobe | pl SB | grep -c 'SB: zeroed=no'` → `1`; restore; `git diff --stat crates/xv6-mbc/adapters/upc_slots.c` shows no mutation left; `kbuild; gbuild`.
- [ ] **Step 142** [TEST][B][S] ~15m: **AFTER GATE**: `gate sbrk-after; grep -a 'sbrkprobe' "$S/sbrk-after-harness.txt" | grep -c PASS`.
- [ ] **Step 143** [V] ~1m: `same=12 diff=0 rc=0`, then `1`.
- [ ] **Step 144** [C][S] ~2m: `ledger 'reply channel' "$offv" "$rv" sbrk-after; ledger 'sbrk' "$rv" "$nv" sbrk-after; insrow "$nv"; bash scripts/check-ebpf-insns.sh 2>&1 | tail -1`. Record the MBC cost of one `sbrk(4096)` the way `references/evolution1-stage2-log.md` §Phase 5 recorded getpid's +1,348. Then `cmt "[PLAN S-STAGE-2B] Steps 136-144: reply channel and sbrk on; malloc works in a child" ebpf/monad-cpu-ebpf/src/main.rs docs/policy/ebpf-insns-baseline.tsv references/s-stage-2b-log.md`.
- [ ] **Step 145** [V][STOP] ~30s: **PHASE 11 EXIT GATE, STOP C**: tree clean; `git rev-parse HEAD > "$S/last-green"`; `budget` still OK. DP-213's "sbrk first" is met.

---

## PHASE 12: RUNG KILL (Steps 146-160)

**Goal**: `kill` ends a BPF-scheduled process, with reparenting, and reports dead pids.
**Prerequisite**: Stop C. No user pointer involved, so it runs before the pointer substrate.
**Time**: ~1.5 h
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 146** [V][S] ~1m: **BEFORE GATE** as in Phase 6; `keep offv "$(vcount)"`.
- [ ] **Step 147** [CODE][W] ~10m: Kernel. `upc_slots.c`: `uint64 upc_kill(int pid) { if(pid < 2 || pid > UPC_SLOTS) return -1; upc_reply(UPC_REPLY_KILL, pid - 1); return 0; }`. `usysproc.c` `sys_kill`: `if(upc_slot_active()) return upc_kill(pid);`. `kkill`'s `proc[]` walk is never used for slot rows: a row with no trap yet has pid 0.
- [ ] **Step 148** [CODE][W] ~20m: BPF. `const UPC_RUNG_KILL: bool = false;`; alias `6 if cfg!(feature = "ascend-linux") && UPC_RUNG_KILL => 11,`; empty arm for 6; surface and `XV6_SYSCALLS` (19). SRET reply block gains: target `t = arg & 7`; live means `t != 0 && t < num_processes`, halted bit clear. Not live: `cpu.regs[8] = -1`, overwriting the restored a0. Live: mark halted and reparent as the exit arm does; `t == current_pid`: `switch_req = SWITCH_EXIT; break;`.
- [ ] **Step 149** [DECIDE] ~5m: **Share the exit arm's reparent loop?** RECOMMENDATION: yes. Add a local `exit_slot: u32 = NONE`, have both the exit arm and the reply set it and break, and do the scan and halted write once, beside the `DEFERRED_SWITCH` block. That is the trick that freed 45,118, and it should pay for this rung. Override ONLY if the goldens' scheduling order moves.
- [ ] **Step 150** [BUILD][B][S] ~5m: `kbuild; gbuild; ebuild; bbuild; keep a "$(vcount)"; echo "off=$offv unflipped=$a"`.
- [ ] **Step 151** [C] ~1m: Const-off point: `cmt "[PLAN S-STAGE-2B] Steps 145-151: kill wired, const off" crates/xv6-mbc/adapters/upc_slots.c crates/xv6-mbc/adapters/upc_slots.h crates/xv6-mbc/adapters/usysproc.c ebpf/monad-cpu-ebpf/src/main.rs ebpf/monad-cpu-ebpf/surface.rs cmd/upc-bootctl/src/main.rs`.
- [ ] **Step 152** [CODE][B][S] ~4m: `flip UPC_RUNG_KILL false true; keep nv "$(vcount)"; echo "on=$nv"; budget "$nv"`.
- [ ] **Step 153** [V] ~30s: `BUDGET-OK`. With the shared loop `a` may differ from `offv`: the refactor is a measured change of its own, recorded as its own ledger row.
- [ ] **Step 154** [TEST][B][S] ~4m: `TRIG=3000000 ub killprobe | pl KL > "$S/kill-lines.txt"; grep -c '=yes$' "$S/kill-lines.txt"; grep -c '=no$' "$S/kill-lines.txt"; grep -c 'self-survived' "$S/kill-lines.txt"; grep -c 'KL: done' "$S/kill-lines.txt"`.
- [ ] **Step 155** [V] ~30s: `12`, `0`, `0`, `1`. Else Step 156.
- [ ] **Step 156** [D] ~15m: `kill-ok=no`: reply lost or liveness wrong (read `num_processes`, then the halted mask). Hang on `wait`: killed slot not halted, or the child's parent never reaped. `orphan-reaped=no`: reparent skipped. `self-survived=yes`: the break missing after `SWITCH_EXIT`. A kill during a fork copy (the review's question): the half-built child is not yet live, since `num_processes` moves at finalize.
- [ ] **Step 157** [TEST][B][S] ~15m: **AFTER GATE**: `gate kill-after; grep -a 'killprobe' "$S/kill-after-harness.txt" | grep -c PASS`.
- [ ] **Step 158** [V] ~1m: `same=12 diff=0 rc=0`, then `1`. gate_nway and forkloop lines PASS.
- [ ] **Step 159** [C][S] ~2m: `ledger 'kill' "$offv" "$nv" kill-after; insrow "$nv"; bash scripts/check-ebpf-insns.sh 2>&1 | tail -1`; ADR-078 §5 row 6 to `| 6 | kill | kernel validates, BPF acts | status 0 until rung 7 |`. `cmt "[PLAN S-STAGE-2B] Steps 152-159: kill on, served through the reply channel" ebpf/monad-cpu-ebpf/src/main.rs docs/adr/ADR-078-gate-d-fs-reader-and-exec-argv-abi.md docs/policy/ebpf-insns-baseline.tsv references/s-stage-2b-log.md`.
- [ ] **Step 160** [V][STOP] ~30s: **PHASE 12 EXIT GATE, STOP D**: tree clean; `git rev-parse HEAD > "$S/last-green"`. The planned end of the first night: `echo $(( (DEADLINE-$(date +%s))/60 ))` minutes left. Under `210`, go to Phase 21. Otherwise carry on, and stop at the next clean point when under `90` remain.

---

## PHASE 13: USER POINTERS AND THE ROW'S CWD (Steps 161-168)

**Goal**: `copyin`, `copyout` and `copyinstr` work on a slot row and refuse what is outside its slice; relative paths resolve. Kernel only: the object does not change.
**Prerequisite**: Stop D.
**Time**: ~45 min
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 161** [V][S] ~1m: **BEFORE GATE** as in Phase 6; `keep offv "$(vcount)"`.
- [ ] **Step 162** [CODE][W] ~15m: `uvm.c`. `walkaddr`, first lines: `if(upc_slot_active()) return upc_user_pa(va);`. `copyout`: skip the `walk(pagetable, va0, 0)` and `PTE_W` test when `upc_slot_active()`; the superpages are all writable. `vmfault`, first line: `if(upc_slot_active()) return 0;`. `upc_slots.c`:
  ```c
  uint64
  upc_user_pa(uint64 va)
  {
    uint32 s = myproc() - proc;
    if(s >= UPC_SLOTS || va >= UPC_USER_VA_CEILING)
      return 0;
    return UPC_SLICE_BASE(s) + va;
  }
  ```
  `UPC_USER_VA_CEILING 0x00800000` in `upc_slots.h`, matching `USER_VA_CEILING` at `main.rs:246`, in `parity_slot_tables.rs`. The first page of VA is allowed: VA 0 is mapped, so `pa == 0` only ever means refused, except slot 0, where `upc_user_pa(0)` is 0 and reads as refused. That is the safe way round.
- [ ] **Step 163** [CODE][W] ~5m: `upc_slot_enter()`, in the refresh block: `if(p->cwd == 0) p->cwd = namei("/");` (D9), once per row: the reference lives as long as the row. ADR-109 D5's stale-field note moves `cwd` to "set".
- [ ] **Step 164** [BUILD][B][S] ~4m: `kbuild; gbuild; (cd cmd/upc-bootctl && cargo test --release --test parity_slot_tables 2>&1 | grep '^test result' | sort | uniq -c); [ "$(vcount)" = "$offv" ] && echo object-equal`.
- [ ] **Step 165** [V] ~30s: `0 failed`, `object-equal`.
- [ ] **Step 166** [TEST][B][S] ~15m: **GATE**: `gate uptr`. Nothing routes here yet, so it proves the refresh change broke nothing.
- [ ] **Step 167** [V] ~1m: `same=12 diff=0 rc=0`, count unchanged. A panic in `namei` on first trap: `fsinit` has not run for that path, read `forkret` in `uproc.c`.
- [ ] **Step 168** [C] ~1m: `cmt "[PLAN S-STAGE-2B] Steps 160-168: slot rows resolve user pointers and a root cwd" crates/xv6-mbc/adapters/uvm.c crates/xv6-mbc/adapters/upc_slots.c crates/xv6-mbc/adapters/upc_slots.h cmd/upc-bootctl/tests/parity_slot_tables.rs docs/adr/ADR-109-upc-kernel-rows-per-bpf-slot.md`. **PHASE 13 EXIT GATE**: tree clean; `git rev-parse HEAD > "$S/last-green"`.

---

## PHASE 14: RUNG MKDIR (Steps 169-183)

**Goal**: the first kernel-side FS write (DP-502), visible to the in-BPF reader, safe against bad paths.
**Prerequisite**: Phase 13.
**Time**: ~1.25 h
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 169** [V][S] ~1m: **BEFORE GATE** as in Phase 6; `keep offv "$(vcount)"`.
- [ ] **Step 170** [CODE][W] ~5m: `const UPC_RUNG_MKDIR: bool = false;`; alias `20 if cfg!(feature = "ascend-linux") && UPC_RUNG_MKDIR => 11,`; empty arm for 20; surface and `XV6_SYSCALLS` (20); `parity_surface` green.
- [ ] **Step 171** [BUILD][B][S] ~4m: `ebuild; bbuild; [ "$(vcount)" = "$offv" ] && echo off-equal`.
- [ ] **Step 172** [C] ~1m: On `off-equal`, const-off point: `cmt "[PLAN S-STAGE-2B] Steps 169-172: mkdir arm wired, const off" ebpf/monad-cpu-ebpf/src/main.rs ebpf/monad-cpu-ebpf/surface.rs cmd/upc-bootctl/src/main.rs`.
- [ ] **Step 173** [CODE][B][S] ~4m: `flip UPC_RUNG_MKDIR false true; keep nv "$(vcount)"; echo "mkdir=$nv delta=$((nv-offv))"; budget "$nv"`.
- [ ] **Step 174** [V] ~30s: `BUDGET-OK`.
- [ ] **Step 175** [TEST][B][S] ~4m: `TRIG=4000000 ub mkprobe | pl MK > "$S/mk-lines.txt"; grep -c '=yes$' "$S/mk-lines.txt"; grep -c '=no$' "$S/mk-lines.txt"; grep -c 'MK: done' "$S/mk-lines.txt"`.
- [ ] **Step 176** [V] ~30s: `7`, `0`, `1`. Else Step 177.
- [ ] **Step 177** [D] ~20m: `panic: upc: slot row`: something slept. Find it: `log.c` `begin_op` under log pressure, or a held sleeplock. One syscall at a time means neither should happen, so a hit is a leaked `begin_op`. `made=yes` but `open=no`: BPF read the old root dir; check `end_op` ran `commit` and that `SYS_WRITE_BLOCK` writes sectors `2N, 2N+1` at word 0x200000. `is-dir=no`: BPF `fstat` type offset (ADR-078 §2). Kernel stack overrun looks like a wild trapframe: `create` plus `namex` fit one page in stock xv6, and under ilp32e they fit better.
- [ ] **Step 178** [SECURITY][TEST][B][S] ~3m: `TRIG=3000000 ub hostile_ptr | pl HU | tr '\n' ' '`.
- [ ] **Step 179** [V] ~30s: five `=yes`, no `=no`, `HU: done`. A kernel panic here is a real finding: stop the rung, fix `upc_user_pa` or `copyinstr`'s bound.
- [ ] **Step 180** [TEST][B][S] ~15m: **AFTER GATE**: `gate mkdir-after; grep -aE 'mkprobe|hostile_ptr' "$S/mkdir-after-harness.txt" | grep -c PASS`.
- [ ] **Step 181** [V] ~1m: `same=12 diff=0 rc=0`, then `2`. Every boot loads `fs.img` fresh into RAM, so a probe's mkdir cannot reach a golden: `ls` still SAME proves it.
- [ ] **Step 182** [C][S] ~2m: `ledger 'mkdir' "$offv" "$nv" mkdir-after; insrow "$nv"; bash scripts/check-ebpf-insns.sh 2>&1 | tail -1`; ADR-078 §5 row 20 to `| 20 | mkdir | kernel | DP-502 |`. `cmt "[PLAN S-STAGE-2B] Steps 173-182: mkdir in the owned kernel, read back through BPF" ebpf/monad-cpu-ebpf/src/main.rs docs/adr/ADR-078-gate-d-fs-reader-and-exec-argv-abi.md docs/policy/ebpf-insns-baseline.tsv references/s-stage-2b-log.md`.
- [ ] **Step 183** [V][STOP] ~30s: **PHASE 14 EXIT GATE**: tree clean; `git rev-parse HEAD > "$S/last-green"`. Clean stopping point (E-1).

---

## PHASE 15: RUNG UNLINK (Steps 184-194)

**Goal**: `unlink` kernel-side, including the empty-directory and dot checks.
**Prerequisite**: Phase 14.
**Time**: ~40 min
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 184** [V][S] ~1m: **BEFORE GATE** as in Phase 6; `keep offv "$(vcount)"`.
- [ ] **Step 185** [CODE][W] ~4m: `const UPC_RUNG_UNLINK: bool = false;`; alias `18 if cfg!(feature = "ascend-linux") && UPC_RUNG_UNLINK => 11,`; empty arm; surface and `XV6_SYSCALLS` (21); `parity_surface` green.
- [ ] **Step 186** [BUILD][B][S] ~4m: `ebuild; bbuild; [ "$(vcount)" = "$offv" ] && echo off-equal`.
- [ ] **Step 187** [C] ~1m: On `off-equal`, const-off point: `cmt "[PLAN S-STAGE-2B] Steps 183-187: unlink arm wired, const off" ebpf/monad-cpu-ebpf/src/main.rs ebpf/monad-cpu-ebpf/surface.rs cmd/upc-bootctl/src/main.rs`.
- [ ] **Step 188** [CODE][B][S] ~4m: `flip UPC_RUNG_UNLINK false true; keep nv "$(vcount)"; echo "unlink=$nv delta=$((nv-offv))"; budget "$nv"`.
- [ ] **Step 189** [TEST][B][S] ~4m: `TRIG=4000000 ub rmprobe | pl RM > "$S/rm-lines.txt"; grep -c '=yes$' "$S/rm-lines.txt"; grep -c '=no$' "$S/rm-lines.txt"; grep -c 'RM: done' "$S/rm-lines.txt"`.
- [ ] **Step 190** [V] ~30s: `BUDGET-OK`, `8`, `0`, `1`. `inner-gone=no`: BPF found a stale dirent: `unlink` zeroes the entry's inum, so check `fs_walk` treats inum 0 as empty. If it does not, that is a BPF read-path bug this rung owns: fix it there, and the count moves.
- [ ] **Step 191** [TEST][B][S] ~15m: **AFTER GATE**: `gate unlink-after; grep -a 'rmprobe' "$S/unlink-after-harness.txt" | grep -c PASS`.
- [ ] **Step 192** [V] ~1m: `same=12 diff=0 rc=0`, then `1`.
- [ ] **Step 193** [C][S] ~2m: `ledger 'unlink' "$offv" "$nv" unlink-after; insrow "$nv"; bash scripts/check-ebpf-insns.sh 2>&1 | tail -1`; ADR-078 §5 row 18 to `| 18 | unlink | kernel | DP-502 |`. `cmt "[PLAN S-STAGE-2B] Steps 188-193: unlink in the owned kernel" ebpf/monad-cpu-ebpf/src/main.rs docs/adr/ADR-078-gate-d-fs-reader-and-exec-argv-abi.md docs/policy/ebpf-insns-baseline.tsv references/s-stage-2b-log.md`.
- [ ] **Step 194** [V][STOP] ~30s: **PHASE 15 EXIT GATE**: tree clean; `git rev-parse HEAD > "$S/last-green"`. Clean stopping point (E-2).

---

## PHASE 16: RUNG LINK (Steps 195-205)

**Goal**: `link` kernel-side; a second name reads the same bytes through BPF.
**Prerequisite**: Phase 15.
**Time**: ~40 min
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 195** [V][S] ~1m: **BEFORE GATE** as in Phase 6; `keep offv "$(vcount)"`.
- [ ] **Step 196** [CODE][W] ~4m: `const UPC_RUNG_LINK: bool = false;`; alias `19 if cfg!(feature = "ascend-linux") && UPC_RUNG_LINK => 11,`; empty arm; surface and `XV6_SYSCALLS` (22); `parity_surface` green.
- [ ] **Step 197** [BUILD][B][S] ~4m: `ebuild; bbuild; [ "$(vcount)" = "$offv" ] && echo off-equal`.
- [ ] **Step 198** [C] ~1m: On `off-equal`, const-off point: `cmt "[PLAN S-STAGE-2B] Steps 194-198: link arm wired, const off" ebpf/monad-cpu-ebpf/src/main.rs ebpf/monad-cpu-ebpf/surface.rs cmd/upc-bootctl/src/main.rs`.
- [ ] **Step 199** [CODE][B][S] ~4m: `flip UPC_RUNG_LINK false true; keep nv "$(vcount)"; echo "link=$nv delta=$((nv-offv))"; budget "$nv"`.
- [ ] **Step 200** [TEST][B][S] ~4m: `TRIG=4000000 ub lnprobe | pl LN > "$S/ln-lines.txt"; grep -c '=yes$' "$S/ln-lines.txt"; grep -c '=no$' "$S/ln-lines.txt"; grep -c 'LN: done' "$S/ln-lines.txt"`.
- [ ] **Step 201** [V] ~30s: `BUDGET-OK`, `9`, `0`, `1`.
- [ ] **Step 202** [TEST][B][S] ~15m: **AFTER GATE**: `gate link-after; grep -a 'lnprobe' "$S/link-after-harness.txt" | grep -c PASS`.
- [ ] **Step 203** [V] ~1m: `same=12 diff=0 rc=0`, then `1`.
- [ ] **Step 204** [C][S] ~2m: `ledger 'link' "$offv" "$nv" link-after; insrow "$nv"; bash scripts/check-ebpf-insns.sh 2>&1 | tail -1`; ADR-078 §5 row 19 to `| 19 | link | kernel | DP-502 |`. `cmt "[PLAN S-STAGE-2B] Steps 199-204: link in the owned kernel; DP-502 directory writes done" ebpf/monad-cpu-ebpf/src/main.rs docs/adr/ADR-078-gate-d-fs-reader-and-exec-argv-abi.md docs/policy/ebpf-insns-baseline.tsv references/s-stage-2b-log.md`.
- [ ] **Step 205** [V][STOP] ~30s: **PHASE 16 EXIT GATE, STOP E**: tree clean; `git rev-parse HEAD > "$S/last-green"`. DP-502's directory writes are done. Go to Phase 21 if this is the end of the night.

---

## PHASE 17: RUNG SLEEP — PAUSE MEANS PAUSE (Steps 206-218)

**Goal**: `pause(n)` waits n UPC ticks, and the retry hand-off (D7) exists for every later blocking call. EVOLUTION-1's "sleep" arrival is this same syscall number.
**Prerequisite**: Stop E (Stop C at least; this phase does not depend on Phases 13-16).
**Time**: ~1.5 h
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 206** [V][S] ~1m: **BEFORE GATE** as in Phase 6; `keep offv "$(vcount)"`.
- [ ] **Step 207** [CODE][W] ~15m: Kernel. `upc_slots.c`:
  ```c
  static uint upc_wake[UPC_SLOTS];  // 0 = no deadline; survives retries, cleared on refresh

  // ADR D7: cannot finish now. Run the ecall again when this slot is next scheduled.
  void upc_retry(void) { myproc()->trapframe->epc -= 4; upc_reply(UPC_REPLY_YIELD, 0); }

  uint64
  upc_pause(int n)
  {
    uint32 s = myproc() - proc;
    if(n <= 0){ upc_wake[s] = 0; return 0; }
    uint t = ticks;
    if(upc_wake[s] == 0)
      upc_wake[s] = (t + n) ? (t + n) : 1;
    if(t >= upc_wake[s]){ upc_wake[s] = 0; return 0; }
    upc_retry();
    return n;   // a0 stays n for the re-run
  }
  ```
  The refresh block clears `upc_wake[s]`. `usysproc.c` `sys_pause`: after the clamp, `if(upc_slot_active()) return upc_pause(n);`. It must never reach `sleep()`.
- [ ] **Step 208** [CODE][W] ~10m: BPF. `const UPC_RUNG_SLEEP: bool = false;`; alias `13 if cfg!(feature = "ascend-linux") && UPC_RUNG_SLEEP => 11,` placed before the row for 23. The match returns once, so yield still reaches the arm for 13. SRET reply block: `if r & phase12::UPC_REPLY_YIELD != 0 && cpu.num_processes > 1 { switch_req = SWITCH_YIELD; break; }`. Check first that `epc - 4` resolves: `sepc` holds the ecall's byte address, and `RV2MBC_MAP` must have that word; a boot halting `0x48` means it does not, and the translator must map ecall words.
- [ ] **Step 209** [BUILD][B][S] ~5m: `kbuild; gbuild; ebuild; bbuild; keep a "$(vcount)"; echo "off=$offv unflipped=$a"`.
- [ ] **Step 210** [C] ~1m: Const-off point: `cmt "[PLAN S-STAGE-2B] Steps 205-210: pause(n) wired, const off" crates/xv6-mbc/adapters/upc_slots.c crates/xv6-mbc/adapters/upc_slots.h crates/xv6-mbc/adapters/usysproc.c ebpf/monad-cpu-ebpf/src/main.rs`.
- [ ] **Step 211** [CODE][B][S] ~4m: `flip UPC_RUNG_SLEEP false true; keep nv "$(vcount)"; echo "on=$nv"; budget "$nv"`.
- [ ] **Step 212** [V] ~30s: `BUDGET-OK`.
- [ ] **Step 213** [TEST][B][S] ~4m: `TRIG=3000000 ub sleepprobe | pl SL > "$S/sl-lines.txt"; cat "$S/sl-lines.txt" | tr '\n' ' '; TRIG=2000000 ub uptimeprobe | pl UP | grep -c 'moved=yes'; ub gate_nway | grep -aoE 'NWAY-ok pid[123]' | sort -u | wc -l`.
- [ ] **Step 214** [V] ~30s: four `=yes`, `SL: parent-ran` before `SL: child-woke`, `SL: done`; then `2`; then `3`. Boot too slow for 2,000 ticks: raise the `TRIG` here and in the harness line together, and record it.
- [ ] **Step 215** [TEST][B][S] ~15m: **AFTER GATE**: `gate sleep-after; grep -a 'sleepprobe' "$S/sleep-after-harness.txt" | grep -c PASS`.
- [ ] **Step 216** [V] ~1m: `same=12 diff=0 rc=0`, then `1`.
- [ ] **Step 217** [C][S] ~2m: `ledger 'pause(n), retry hand-off' "$offv" "$nv" sleep-after; insrow "$nv"; bash scripts/check-ebpf-insns.sh 2>&1 | tail -1`; ADR-078 §5 row 13 to `| 13 | pause | kernel | n UPC ticks (ADR-110), retry hand-off |`; ADR-110 T3 gets "Superseded by the sleep rung, 2026-11-04". `cmt "[PLAN S-STAGE-2B] Steps 211-217: pause(n) on; retry hand-off" ebpf/monad-cpu-ebpf/src/main.rs docs/adr/ADR-078-gate-d-fs-reader-and-exec-argv-abi.md docs/adr/ADR-110-upc-kernel-time-from-bpf-ticks.md docs/policy/ebpf-insns-baseline.tsv references/s-stage-2b-log.md`.
- [ ] **Step 218** [V][STOP] ~30s: **PHASE 17 EXIT GATE, STOP F**: tree clean; `git rev-parse HEAD > "$S/last-green"`.

---
## PHASE 18: RUNG FD TABLE — KERNEL FILES, O_CREATE, FILE WRITES (Steps 219-236)

**Goal**: a real fd table for everything BPF cannot serve: `open(O_CREATE|O_WRONLY|O_TRUNC)`, file `write`, and kernel-side `read`, `fstat`, `dup`, `close` on those fds (DP-503's "a real fd table is a Stage 2 rung", DP-502's file writes). BPF keeps fds below 16: the console and read-only files.
**Prerequisite**: Stop F. Second night.
**Time**: ~4.5 h
**Agent**: Coordinator; one review agent
**Model**: Opus

This phase changes fork and exit for every process, so it is the first rung that can move a golden. Read D14 before coding.

- [ ] **Step 219** [V][S] ~1m: **BEFORE GATE** as in Phase 6; `keep offv "$(vcount)"`.
- [ ] **Step 220** [DESIGN][W] ~30m: Amend the reply ADR:
  - **D10 number space.** Kernel fds are `16 + ofile index`. BPF's table is `NOFILE = 16`, so the spaces are disjoint and neither allocator sees the other. `argfd` and `fdalloc` translate when `upc_slot_active()`. Limit, stated in the ADR: `dup` of a kernel fd stays at or above 16, so shell redirection onto fds 0-2 waits for the full hand-off (rung 6).
  - **D11 routing.** `kernel_routed_alias` takes `cpu`: `5 | 8 | 10 | 16 | 21 if UPC_RUNG_FD && cpu.regs[8] >= 16 => 11`, and `15 if UPC_RUNG_FD && cpu.regs[9] & (O_WRONLY | O_CREATE | O_TRUNC) != 0 => 11`. `open("console", O_RDWR)` stays in BPF: O_RDWR alone is not in the mask, and read-only BPF `open` is unchanged.
  - **D12 fork note.** Lazy refresh is wrong for inheritance: a parent may close a fd or exit before its child's first trap. When BPF finalizes a fork under `UPC_RUNG_FD`, it sets `regs[1] = 24`, `regs[8] = child slot`, `pc -= 1`, so the parent's ecall re-runs as number 24, aliased into the kernel. `sys_upc_forknote` refreshes the child's row at once, `filedup`s `ofile`, `idup`s `cwd`, and returns the child's pid. It refuses unless the child's `fresh_mask` bit is set, so a user who issues 24 gets -1.
  - **D13 exit.** `2 if UPC_RUNG_FD => 11`. `sys_exit` on a slot row closes `ofile` and replies EXIT (bit 3); BPF then runs the shared exit path. `upc_kill` closes the target's `ofile` before replying KILL.
  - **D14 quiet trap returns.** The SRET arm writes `S` to the TTY and arms the 100-PC tracer, and those bytes are in the goldens. Corpus boots take no kernel trap today. With D12 and D13 every fork and exit does, so all 12 goldens would move. A tagged trap's `prepare_return` writes `UPC_TAG_REPLY` with no flags, and the SRET arm skips the tracer when it sees the tag. The reply read therefore moves ahead of the tracer.
- [ ] **Step 221** [R] ~20m: Review agent, read-only, same shape as Phase 10; the coordinator waits. Ask: can number 24 forge a child's row; what if the parent is killed between finalize and the note; does a re-run fork disturb a2's copy cursor (ADR-078 §3).
- [ ] **Step 222** [C] ~1m: Apply dispositions. `cmt "[PLAN S-STAGE-2B] Steps 218-222: fd table design" "docs/adr/ADR-$(cat "$S/adr-n")-upc-kernel-reply-channel.md" references/s-stage-2b-log.md`.
- [ ] **Step 223** [TEST][W] ~20m: `user/fdprobe.c` (`FD:` tokens, registered as in Phase 4, gated `rung_gate UPC_RUNG_FD fdprobe 5000000 FD <count>`), run in a child: create with `O_CREATE|O_WRONLY`, fd at least 16; write 300 bytes; close; `open` read-only through BPF and read them back; a second `O_TRUNC` open empties it; a forked child writes through the inherited fd and the parent sees the shared offset; `dup` of a kernel fd; `fstat` on a kernel fd; `close` twice gives -1; `read(99, ...)` gives -1; a child exits with the fd open, and the file stays right after.
- [ ] **Step 224** [CODE][W] ~45m: Kernel: `argfd`, `fdalloc`, `sys_upc_forknote` (`syscalls[24]`; `SYS_upc_forknote 24` in `syscall.h`), `upc_exit`, EXIT flag, the `prepare_return` tag. `upc_slot_enter`'s refresh stops clearing a row the note already filled.
- [ ] **Step 225** [CODE][W] ~45m: BPF: `const UPC_RUNG_FD: bool = false;`; alias takes `cpu`; fork finalize; reply block gains EXIT; D14 tracer guard. `$MAIN` alias call site, `surface.rs` and `XV6_SYSCALLS` (23 entries). Every alias row carries `cfg!(feature = "ascend-linux")`. The arms for 5, 8, 10, 15, 16, 21 stay: BPF still serves fds below 16.
- [ ] **Step 226** [BUILD][B][S] ~6m: `kbuild; gbuild; ebuild; bbuild; keep a "$(vcount)"; echo "off=$offv unflipped=$a"; sweep > "$S/fd-off-sweep8.txt" 2>&1; grep -c '^SAME' "$S/fd-off-sweep8.txt"`. Expected `8`: the const-off object keeps the corpus.
- [ ] **Step 227** [C] ~1m: Const-off point: `cmt "[PLAN S-STAGE-2B] Steps 223-227: fd table wired, const off" crates/xv6-mbc/adapters/usysfile.c crates/xv6-mbc/adapters/usyscall.c crates/xv6-mbc/adapters/usysproc.c crates/xv6-mbc/adapters/upc_slots.c crates/xv6-mbc/adapters/upc_slots.h crates/xv6-mbc/adapters/utrap.c crates/xv6-mbc/upstream/kernel/syscall.h crates/xv6-mbc/upstream/user/fdprobe.c crates/xv6-mbc/adapters/Makefile.mbc-userland scripts/upc-regression.sh ebpf/monad-cpu-ebpf/src/main.rs ebpf/monad-cpu-ebpf/src/phase12.rs ebpf/monad-cpu-ebpf/surface.rs cmd/upc-bootctl/src/main.rs`.
- [ ] **Step 228** [CODE][B][S] ~4m: `flip UPC_RUNG_FD false true; keep nv "$(vcount)"; echo "on=$nv"; budget "$nv"`.
- [ ] **Step 229** [V] ~1m: `BUDGET-OK`. If it is not: [DECIDE] drop D13's alias for slots that never held a kernel fd (a per-slot bit read only on the exit path). RECOMMENDATION: measure first. Over 900,000 ends the rung: flip off, Stop F stands, and "For Stevie" says what it would cost.
- [ ] **Step 230** [TEST][B][S] ~6m: `TRIG=5000000 ub fdprobe | pl FD > "$S/fd-lines.txt"; grep -c '=no$' "$S/fd-lines.txt"; grep -c 'FD: done' "$S/fd-lines.txt"; sweep > "$S/fd-sweep8.txt" 2>&1; grep -c '^SAME' "$S/fd-sweep8.txt"`.
- [ ] **Step 231** [V] ~1m: `0`, `1`, `8`. Any `DIFF`: Step 232.
- [ ] **Step 232** [D] ~20m: `diff <(tty_of "$(boot $'ls\n')") "$GOLD/ls.tty" | head`. Extra `S` or PC lines: D14 guard ordering. A changed `tty_r` or `forks`: stats drift from the extra trap ticks. Goldens are never recaptured to pass a rung: a policy recapture needs a recorded reason and HEAD (skeleton's rung template), and here it is `[STUCK]`, flip off.
- [ ] **Step 233** [TEST][B][S] ~15m: **AFTER GATE**: `gate fd-after`.
- [ ] **Step 234** [V] ~1m: `same=12 diff=0 rc=0`; `forkloop`, `gate_nway`, `gate2`, `ut_*` lines PASS.
- [ ] **Step 235** [C][S] ~2m: `ledger 'fd table' "$offv" "$nv" fd-after; insrow "$nv"; bash scripts/check-ebpf-insns.sh 2>&1 | tail -1`; ADR-078 §5 gains row 24 and a note on fds at or above 16. `cmt "[PLAN S-STAGE-2B] Steps 228-235: fd table on: O_CREATE and file writes" ebpf/monad-cpu-ebpf/src/main.rs docs/adr/ADR-078-gate-d-fs-reader-and-exec-argv-abi.md docs/policy/ebpf-insns-baseline.tsv references/s-stage-2b-log.md`.
- [ ] **Step 236** [V][STOP] ~30s: **PHASE 18 EXIT GATE, STOP G**: tree clean; `git rev-parse HEAD > "$S/last-green"`.

---

## PHASE 19: RUNG CHDIR (Steps 237-245)

**Goal**: `chdir` holds for every path call, including `open`.
**Prerequisite**: Stop G.
**Time**: ~1.25 h
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 237** [V][S] ~1m: **BEFORE GATE** as in Phase 6; `keep offv "$(vcount)"`.
- [ ] **Step 238** [DESIGN][W] ~10m: ADR **D15**: BPF `open` walks from the root inode, so a process off the root must open through the kernel. Table word `cwd_mask` at +0x81C (the pad): the kernel sets bit s when row s's cwd is not the root inode, the fork note copies it. Alias: `15 if UPC_RUNG_CHDIR && cwd bit set => 11`, a word read only when n is 15. `kernel_open` then returns a fd at or above 16, which `read` routes already.
- [ ] **Step 239** [TEST][W] ~10m: `user/chdirprobe.c` (`CD:` tokens, gated `rung_gate UPC_RUNG_CHDIR chdirprobe 5000000 CD <count>`), in a child: `mkdir("cdd")`, `chdir("cdd")`, `mkdir("in")` relative, `open("in", O_RDONLY)` relative, `chdir("nope")` gives -1, `chdir("README")` gives -1, a forked child inherits (`open("in")`), `chdir("..")` then `open("cdd/in")` through BPF.
- [ ] **Step 240** [CODE][W] ~25m: Kernel: `sys_chdir` is stock over a row with a real cwd; add the `cwd_mask` update and the fork-note copy. BPF: `const UPC_RUNG_CHDIR: bool = false;`, alias rows for `9` and the `15` case, each with `cfg!(feature = "ascend-linux")`, surface and `XV6_SYSCALLS`, mask offset in `phase12.rs`, `upc_slots.h`, parity. `XV6_SYSCALLS` has 24 entries after this rung.
- [ ] **Step 241** [BUILD][B][S] ~6m: `kbuild; gbuild; ebuild; bbuild; [ "$(vcount)" = "$offv" ] && echo off-equal; flip UPC_RUNG_CHDIR false true; keep nv "$(vcount)"; echo "chdir=$nv delta=$((nv-offv))"; budget "$nv"`; `TRIG=5000000 ub chdirprobe | pl CD | grep -c '=no$'`.
- [ ] **Step 242** [V] ~30s: `off-equal`, `BUDGET-OK`, `0`.
- [ ] **Step 243** [TEST][B][S] ~15m: **AFTER GATE**: `gate chdir-after`.
- [ ] **Step 244** [C][S] ~2m: On `same=12 diff=0 rc=0`: `ledger 'chdir' "$offv" "$nv" chdir-after; insrow "$nv"; bash scripts/check-ebpf-insns.sh 2>&1 | tail -1`, ADR-078 §5 row 9, and `cmt "[PLAN S-STAGE-2B] Steps 236-244: chdir, with open honouring cwd" crates/xv6-mbc/adapters/usysfile.c crates/xv6-mbc/adapters/upc_slots.c crates/xv6-mbc/adapters/upc_slots.h crates/xv6-mbc/upstream/user/chdirprobe.c crates/xv6-mbc/adapters/Makefile.mbc-userland scripts/upc-regression.sh ebpf/monad-cpu-ebpf/src/main.rs ebpf/monad-cpu-ebpf/src/phase12.rs ebpf/monad-cpu-ebpf/surface.rs cmd/upc-bootctl/src/main.rs cmd/upc-bootctl/tests/parity_slot_tables.rs docs/adr/ADR-078-gate-d-fs-reader-and-exec-argv-abi.md docs/policy/ebpf-insns-baseline.tsv references/s-stage-2b-log.md`.
- [ ] **Step 245** [V][STOP] ~30s: **PHASE 19 EXIT GATE**: tree clean; `git rev-parse HEAD > "$S/last-green"`. Clean stopping point (G-1).

---

## PHASE 20: RUNG PIPE (Steps 246-256)

**Goal**: a pipe between processes, blocking through the retry hand-off, EOF through exit-time close.
**Prerequisite**: Phase 19 or Stop G (needs D7, D10, D12, D13, D6).
**Time**: ~2.5 h
**Agent**: Coordinator
**Model**: Opus

- [ ] **Step 246** [V][S] ~1m: **BEFORE GATE** as in Phase 6; `keep offv "$(vcount)"`.
- [ ] **Step 247** [TEST][W] ~10m: `user/pipeprobe.c` (`PI:` tokens, gated `rung_gate UPC_RUNG_PIPE pipeprobe 8000000 PI <count>`): `pipe(p)` gives two fds at or above 16; a forked child writes 2,000 bytes (more than `PIPESIZE`, looping on short writes) and exits; the parent reads until 0 and checks every byte; EOF arrives only after the child's exit, proving D13; read on an empty pipe with a live writer blocks without spinning the parent past a sibling; write with no reader gives -1; a bad `pipe()` pointer at `0x00800000` gives -1, and no fd leaks (the next `pipe()` still works).
- [ ] **Step 248** [CODE][W] ~30m: Kernel. `upipe.c`, on slot rows: replace `sleep()` in `piperead` with: release the lock and `upc_retry()`, returning a new sentinel. `pipewrite`: if `i > 0`, return the short count; if `i == 0`, retry. Short writes are legal here, and the probe loops. `syscall()`: when a retry is pending, do not overwrite `trapframe->a0`, so fd, buffer and count survive the re-run. `killed()` checks stay. `wakeup()` calls stay: they find nothing and return.
- [ ] **Step 249** [CODE][W] ~10m: BPF: `const UPC_RUNG_PIPE: bool = false;`, alias `4 if cfg!(feature = "ascend-linux") && UPC_RUNG_PIPE => 11,`, empty arm, surface and `XV6_SYSCALLS` (25 entries). `read`, `write` and `close` already route for fds at or above 16.
- [ ] **Step 250** [BUILD][B][S] ~6m: `kbuild; gbuild; ebuild; bbuild; [ "$(vcount)" = "$offv" ] && echo off-equal; flip UPC_RUNG_PIPE false true; keep nv "$(vcount)"; echo "pipe=$nv delta=$((nv-offv))"; budget "$nv"`.
- [ ] **Step 251** [TEST][B][S] ~6m: `TRIG=8000000 ub pipeprobe | pl PI > "$S/pi-lines.txt"; grep -c '=no$' "$S/pi-lines.txt"; grep -c 'PI: done' "$S/pi-lines.txt"`.
- [ ] **Step 252** [V] ~30s: `off-equal`, `BUDGET-OK`, `0`, `1`. Else Step 253.
- [ ] **Step 253** [D] ~20m: A hang with both ends live: the retry never yields (`num_processes` guard), or `a0` was clobbered, so the re-run reads a wrong fd. Lost bytes: a retry after partial progress; `pipewrite` must return short. No EOF: exit did not close the inherited write end, so check D12's `filedup` count against D13's close.
- [ ] **Step 254** [TEST][B][S] ~15m: **AFTER GATE**: `gate pipe-after`.
- [ ] **Step 255** [C][S] ~2m: On `same=12 diff=0 rc=0`: `ledger 'pipe' "$offv" "$nv" pipe-after; insrow "$nv"; bash scripts/check-ebpf-insns.sh 2>&1 | tail -1`, ADR-078 §5 row 4, and `cmt "[PLAN S-STAGE-2B] Steps 245-255: pipe in the owned kernel" crates/xv6-mbc/adapters/upipe.c crates/xv6-mbc/adapters/usyscall.c crates/xv6-mbc/adapters/upc_slots.c crates/xv6-mbc/upstream/user/pipeprobe.c crates/xv6-mbc/adapters/Makefile.mbc-userland scripts/upc-regression.sh ebpf/monad-cpu-ebpf/src/main.rs ebpf/monad-cpu-ebpf/surface.rs cmd/upc-bootctl/src/main.rs docs/adr/ADR-078-gate-d-fs-reader-and-exec-argv-abi.md docs/policy/ebpf-insns-baseline.tsv references/s-stage-2b-log.md`.
- [ ] **Step 256** [V][STOP] ~30s: **PHASE 20 EXIT GATE, STOP H**: tree clean; `git rev-parse HEAD > "$S/last-green"`. Every syscall DP-213 named is alive.

---

## PHASE 21: SECURITY PASS (Steps 257-266)

**Goal**: whatever landed is attacked once more, and the new surface is written down. Runs from whichever Stop ended the night.
**Prerequisite**: any Stop from C on. From Stop A or B: skip to Phase 22.
**Time**: ~45 min
**Agent**: Coordinator
**Model**: Opus
**Second night**: Step 258 is skipped if `crates/xv6-mbc/upstream/user/hostile_reply.c` exists; append only the new rungs' rows to §Attack surface.

- [ ] **Step 257** [W] ~15m: `$LOG` §Attack surface, one row per landed rung: the new kernel reads of BPF-written words (`brk_in`, `reply`), the new user pointers, what bounds each, the probe that proves it. Kernel C now parses user pointers where the verifier used to bound every access; every row names a `hostile_ptr` line or a probe line.
- [ ] **Step 258** [SECURITY][B][S] ~3m: Forged reply, in a child, using the probe shape of `hostile_slot.c`: write `UPC_TAG_REPLY | SET_BRK` at the table address the child sees. Stores land in its own slice, as S26 found. Add it as `user/hostile_reply.c`, tokens `HR:`, and gate it unconditionally once `UPC_REPLY` is true. Expect `HR: heap-intact=yes`.
- [ ] **Step 259** [V] ~1m: D6 is untouched: `grep -c '^const UPC_D6_MISS_FAULT: bool = false;' "$MAIN"` → `1`. The route through another slot's pgd stays open while D6 is off (M-247), and now also reaches the reply word. Say so in the ADR's security notes and in "For Stevie". Do not flip it.
- [ ] **Step 260** [B] ~1m: `command -v cargo-fuzz || cargo +nightly fuzz --version 2>&1 | head -1`.
- [ ] **Step 261** [DECIDE] ~6m: **Fuzz.** RECOMMENDATION: if `cargo fuzz` exists, `(cd crates/monad-mbc && timeout 400 cargo +nightly fuzz run fuzz_mbc_execute -- -max_total_time=300 2>&1 | tail -3)` and no new artifact; if not, record SKIP in `$LOG` as Stage 2 Step 140 did. A SKIP is not a pass. Installing a tool is Stevie's call. Override: none.
- [ ] **Step 262** [C][S] ~1m: The meta-gate needs a clean tree (series rule 6). `cmt "[PLAN S-STAGE-2B] Steps 256-262: attack surface, forged-reply probe" crates/xv6-mbc/upstream/user/hostile_reply.c crates/xv6-mbc/adapters/Makefile.mbc-userland scripts/upc-regression.sh references/s-stage-2b-log.md`; `git status --short | wc -l` → `0`.
- [ ] **Step 263** [B][S] ~10m: Repo gates. Go is untouched, so `gofmt`, `go vet` and gosec are out of scope; confirm with `git diff --stat develop... -- '*.go' | wc -l` → `0`. Then `bash scripts/check-ebpf-insns.sh 2>&1 | tail -1; bash scripts/check-spdx.sh 2>&1 | tail -1; bash scripts/check-gitleaks.sh 2>&1 | tail -1; bash scripts/check-gates-can-fail.sh --only check-ebpf-insns 2>&1 | tail -1`.
- [ ] **Step 264** [V] ~1m: each ends in a pass; the meta-gate says it bites. Non-zero `.go` count: run the series rule 6 Go gates too.
- [ ] **Step 265** [W][S] ~3m: The reply ADR stays **Proposed**: acceptance is Stevie's. `forstevie "ADR-$(cat "$S/adr-n") is Proposed; gate commits: $(git log --format=%h --grep='PLAN S-STAGE-2B' -n 12 | tr '\n' ' ')"`.
- [ ] **Step 266** [C] ~1m: `cmt "[PLAN S-STAGE-2B] Steps 263-266: security pass" docs/battle-plans/progress/S-STAGE-2B-progress.md references/s-stage-2b-log.md`. **PHASE 21 EXIT GATE**: tree clean.

---

## PHASE 22: DOC RIPPLE (Steps 267-272)

**Goal**: the docs say what landed, nothing more.
**Prerequisite**: the last Stop reached, after Phase 21 where it applies.
**Time**: ~20 min
**Agent**: one agent, serial, no boot
**Model**: Sonnet
**Second night**: the same steps, unchanged. `upsub` prints `UPSUB-REDO` where the first night printed `UPSUB-FIRST`, and the `grep -q` guards skip what is already there.

First compute the facts every step uses: `keep LANDED "$(grep -oE '^const UPC_RUNG[A-Z_0-9]*: bool = true' "$MAIN" | grep -oE 'UPC_RUNG[A-Z_0-9]*' | tr '\n' ' ')"; keep COUNT "$(tail -1 "$S/gates.txt" | grep -oE 'ascend=[0-9]+' | cut -d= -f2)"; echo "$LANDED / $COUNT"`. Write only about rungs in `$LANDED`. `REP-FAIL` or `UPSUB-FAIL`: `[STUCK]`, go on.

- [ ] **Step 267** [W] ~3m: `CLAUDE.md`. Build the paragraph, then insert it. Keep the first three `X+=` lines only if `grep -c p414-after "$S/gates.txt"` is at least `1`; the sbrk line only if `UPC_RUNG_SBRK` is in `$LANDED`; the kill line only if `UPC_RUNG_KILL` is; the last line only if `UPC_RUNG_MKDIR`, `UPC_RUNG_UNLINK` and `UPC_RUNG_LINK` all are.
  ```bash
  P="**S-STAGE-2B (2026-11-04, \`sprint/s-stage-2b\`).** Rungs on: $LANDED. Ascend $COUNT verified, Doom 758,199. Plan \`docs/battle-plans/S-STAGE-2B-battle-plan.md\`, ledger \`references/s-stage-2b-log.md\`. D6 still off."
  X=""
  X+=' `SYS_yield` is 23 (ADR-078 §5).'
  X+=' Kernel builds answer -ENOSYS; the FUZIX fakes sit behind `fuzix-permissive`.'
  X+=' The RV text base travels in BootParams `reserved[1]`.'
  X+=' sbrk serves a 192 KiB window, VA 0x10000-0x40000, not in init.'
  X+=' kill goes through the reply channel.'
  X+=' mkdir, unlink and link run in the owned kernel; BPF reads the result.'
  upsub CLAUDE.md '**S-ASCEND-2 update (2026-10-28, `sprint/ascend-2`, local).**' "$P$X"$'\n\n**S-ASCEND-2 update (2026-10-28, `sprint/ascend-2`, local).**' '^\*\*S-STAGE-2B \(2026-11-04.*$' "$P$X"
  ```
- [ ] **Step 268** [W] ~3m: `wiki/UPC-Dream-Ladder.md`, the L5 row.
  ```bash
  N="since 2026-11-04 the owned kernel serves getpid, uptime and these rungs: $LANDED (S-STAGE-2B)."
  upsub wiki/UPC-Dream-Ladder.md 'N-way concurrent isolation gated since 2026-10-28.' "N-way concurrent isolation gated since 2026-10-28; $N" 'since 2026-11-04 the owned kernel serves [^|]*?\(S-STAGE-2B\)\.' "$N"
  ```
- [ ] **Step 269** [W] ~3m: `docs/sweeps/OUTSTANDING-2026-10-27.md`, rows M-221 and, only if `UPC_RUNG_MKDIR` is in `$LANDED`, M-226.
  ```bash
  O=docs/sweeps/OUTSTANDING-2026-10-27.md
  grep -q 'S-STAGE-2B carries the rungs' $O || rep $O 'rungs 0-1 done, freed 7,628; rung 2 (uptime/pause) next' 'rungs 0-2 done; DP-501 re-read D1 as (a); S-STAGE-2B carries the rungs (ledger in references/s-stage-2b-log.md)'
  grep -q 'directory writes kernel-side (S-STAGE-2B)' $O || rep $O 'DONE in part: console opens by name' 'DONE in part: directory writes kernel-side (S-STAGE-2B); console opens by name'
  ```
- [ ] **Step 270** [W] ~3m: The Stage 2 plan's pointer.
  ```bash
  grep -q 'Forged past rung 2 in' docs/battle-plans/EVOLUTION-1-STAGE-2-battle-plan.md || rep docs/battle-plans/EVOLUTION-1-STAGE-2-battle-plan.md '## RUNGS 2-8 (forge each when the one before it is green)' $'## RUNGS 2-8 (forge each when the one before it is green)\n\nForged past rung 2 in `S-STAGE-2B-battle-plan.md`. The arrivals moved ahead of rungs 3-7 (DP-501, DP-213).'
  ```
- [ ] **Step 271** [W] ~4m: `references/timeline.md`. The `.json`, `.yaml` and `.toml` mirrors are left alone: `-sync-once` does not exist, and timeguru's syncer has no command line. The coordinator hears about it at close.
  ```bash
  python3 - "$LANDED" "$COUNT" <<'PY'
  import sys
  p = "references/timeline.md"
  L = open(p).read().split("\n")
  e = f"- **S-STAGE-2B (2026-11-04, `sprint/s-stage-2b`).** Rungs on: {sys.argv[1].strip()}. Ascend {sys.argv[2]} verified instructions, Doom 758,199; D6 still off. Plan `docs/battle-plans/S-STAGE-2B-battle-plan.md`, ledger `references/s-stage-2b-log.md`."
  j = [k for k, l in enumerate(L) if l.startswith("- **S-STAGE-2B (")]
  i = [k for k, l in enumerate(L) if l.startswith("- **EVOLUTION-1 Stage 2 rungs 0-1:")]
  if len(j) == 1:
      L[j[0]] = e
  else:
      assert len(j) == 0 and len(i) == 1
      L.insert(i[0] + 1, e)
  open(p, "w").write("\n".join(L)); print("TIMELINE-OK")
  PY
  grep -c '^- \*\*S-STAGE-2B (' references/timeline.md; bash scripts/check-timeline-freshness.sh --report | tail -1
  ```
  Expected: `TIMELINE-OK`, `1`, a report line.
- [ ] **Step 272** [C] ~1m: `cmt "[PLAN S-STAGE-2B] Steps 267-272: docs ripple" CLAUDE.md wiki/UPC-Dream-Ladder.md docs/sweeps/OUTSTANDING-2026-10-27.md docs/battle-plans/EVOLUTION-1-STAGE-2-battle-plan.md references/timeline.md`. **PHASE 22 EXIT GATE**: tree clean.

---

## PHASE 23: CLOSE AND MERGE (Steps 273-282)

**Goal**: the branch is on `develop`, signed, unpushed; the coordinator has what it needs for `~/tmp/next.md`.
**Prerequisite**: Phase 22.
**Time**: ~45 min
**Agent**: Coordinator
**Model**: Opus
**Second night**: the same steps. `## Close` becomes `## Close, night 2`; the branch merges again, as a new merge commit.

- [ ] **Step 273** [B] ~3m: Bring `develop` into the sprint worktree first, so the close gate tests what will land.
  ```bash
  cd ~/tmp/wt/s-stage-2b; git merge --no-ff -S develop -m 'Merge develop into sprint/s-stage-2b' || { [ "$(git ls-files -u | wc -l)" != 0 ] && echo MERGE-CONFLICT; }
  ```
  Clean, or `Already up to date`: next step. `MERGE-CONFLICT`: `git merge --abort`, `[STUCK]`, no merge to `develop`; list the files in `$PROG`. Failure with no conflict means gpg is cold: `git merge --abort; git merge --no-ff --no-gpg-sign develop -m 'Merge develop into sprint/s-stage-2b' && echo '- unsigned merge of develop' >> "$PROG"`.
- [ ] **Step 274** [BUILD][B] ~12m: `kbuild; gbuild; ebuild; bbuild; [ -d crates/doom-runner ] && (cd crates/doom-runner && cargo build --release 2>&1 | tail -1); ls "$K"/*.rv2mbc.sha256 | wc -l`. Expected: `Finished` lines, at least 12 sidecars. No `crates/doom-runner` means S-MOVE-OUT landed: Appendix A10.
- [ ] **Step 275** [TEST][B][S] ~15m: `gate close`.
- [ ] **Step 276** [V] ~1m: `same=12 diff=0 rc=0`, and `budget` on the ascend count prints `BUDGET-OK`. Anything else: stop, write it to `$PROG`, no merge.
- [ ] **Step 277** [W] ~3m: `$PROG`, `## Close`: last Stop reached; `$LANDED`; the ledger's last row; every `[STUCK]` with its step number; any unsigned commit; the timeline mirrors left stale; next rung. `git log --oneline develop..HEAD | wc -l` for the commit count.
- [ ] **Step 278** [C] ~1m: `cmt "[PLAN S-STAGE-2B] Steps 273-278: close" docs/battle-plans/progress/S-STAGE-2B-progress.md docs/battle-plans/S-STAGE-2B-battle-plan.md references/s-stage-2b-log.md`.
- [ ] **Step 279** [B] ~2m: Dry merge: `git merge-tree --write-tree develop sprint/s-stage-2b >/dev/null; echo rc=$?`. `rc=0`: next step. `rc=1`: `develop` moved during the gate. `git merge-tree --write-tree --name-only develop sprint/s-stage-2b | tail -5`, `[STUCK]`, leave the branch unmerged; Appendix A9.
- [ ] **Step 280** [B] ~1m: From the main checkout, which must be clean: `git -C ~/tmp/unheaded status --short | wc -l` → `0`, then the merge, then `git -C ~/tmp/unheaded log -1 --format='%h %G?'`.
  ```bash
  git -C ~/tmp/unheaded merge --no-ff -S sprint/s-stage-2b -m 'Merge sprint/s-stage-2b into develop (S-STAGE-2B)' || { git -C ~/tmp/unheaded merge --abort; git -C ~/tmp/unheaded merge --no-ff --no-gpg-sign sprint/s-stage-2b -m 'Merge sprint/s-stage-2b into develop (S-STAGE-2B)' && echo "- unsigned merge" >> "$PROG"; }
  ```
  Expected: `G`, or `N` with the line in `$PROG`. Never push.
- [ ] **Step 281** [V][S] ~2m: Host clean: `sudo bpftool prog show | grep -ciE 'monad|cart|heimdall'` → `0`; `ip netns list | grep -c upc0` → `1`, left up as the unit intends; `pgrep -c upc-bootctl` → `0`.
- [ ] **Step 282** [V] ~1m: **SPRINT EXIT GATE**: merged or `[STUCK]`-logged; not pushed; `$PROG` has `## Close`. Report path, last Stop and open items to the coordinator, who folds `$PROG` into `~/tmp/next.md`. Keep the worktree: the second night resumes in it.

---

## APPENDIX A: EMERGENCY PROCEDURES

**A0. Rollback of a non-rung phase (Phases 6, 8, 9).** The phase's **Rollback** line: restores from `last-green`, rebuilds, gates.

**A1. The ascend count nears or passes 900,000.** Flip the rung off. Read the delta per edit with `vcount`. In-loop variable RAM offsets cost most: one once cost +119K, so hoist work out of the execute loop beside `DEFERRED_SWITCH` (that move freed 45,118). `#[inline(never)]` made it worse (+36,271). Under `ascend-linux`, a const-off object must equal the previous count; if not, the const guards too little. Past 921,036 the ladder is no longer net negative, and that rung stops whatever the ceiling says.

**A2. A boot halts or panics.** bootctl prints `ROM-FAULT CTX` with the code: `0x48` SRET unmapped, `0x11` inject STVEC unmapped, `0x80` plain HALT. `panic: upc: slot row` is ADR-109 I4/I7: a slot row reached `kexit`, `sched`, `sleep` or `freeproc`, so a path needs a reply, not a sleep. `panic: upc: sscratch`: I1 broke; the trap came from a slot other than `cur_slot`.

**A3. A probe prints nothing.** The SRET arm prints `S` and 100 traced PCs after every trap return, so a trapping probe floods the TTY. `pl` strips `([0-9A-F]{4} )+`. Hex digits followed by a space get eaten, which is why tokens are lower case and values are fixed words. `TRIG` too low looks the same: raise it by 1,000,000. Kernel `printf` bytes are char plus NULs: decode from `hex:` as `ugl` does in the Stage 2 plan.

**A4. `sweep12` shows DIFF.** Corpus output moved. Never recapture to pass a rung. `diff <(tty_of "$(boot $'ls\n')") "$GOLD/ls.tty"`. Common causes: a size change in a corpus binary (`usys.pl`, `ulib.c`, `printf.c` edited), or new trap returns printing `S` (D14). Revert or flip off, then `[STUCK]`.

**A5. `git commit -S` fails.** `cmt` falls back to `--no-gpg-sign` and writes a line in `$PROG`. Say so in the handoff; `scripts/resign-unpushed.py` re-signs later.

**A6. `upc0` is gone.** `sudo bash "$PROJECT_ROOT/scripts/upc-netns.sh" up`. The unit stays `active (exited)` with the namespace removed: it happened 2026-10-30.

**A7. Rolling back a rung.** `flip <CONST> true false`; `gate rollback-<rung>`. Same count as the rung's OFF number means the const covered everything. `insrow` back to that count. Kernel code stays: with no routed number it is dead. The rollback is one const, as every rung promises.

**A8. Memory pressure.** `memfloor` says `MEM-LOW`. Other lanes' builds finish first: never kill them. Wait up to 30 min, then `[STUCK]`. The ascend object's LTO link is the heavy build; the peak is in `$LOG` §Run cost.

**A9. Merge conflict on the baseline or timeline.** `ebpf-insns-baseline.tsv`: take develop's rows and this branch's `monad-cpu-ebpf/monad_cpu ascend-linux` row only. If another lane changed that object (S-WIRE-HONEST touches `hop-ebpf`, not this one), re-measure. Timeline: keep both entries, regenerate the mirrors. `Cargo.lock` conflicts: `[STUCK]`.

**A10. S-STAGE-2B lands first; S-MOVE-OUT follows it.** The harness's Doom variant calls `crates/doom-runner`, and `gate` is green only on `verified 758199 insns`. If a merge of `develop` removes it, S-MOVE-OUT owns that harness edit; a harness that no longer prints the count is `[STUCK]` at the gate, with the coordinator. `doom/doom.mbc`, `.rv2mbc` and `.sha256` stay as the fixture (DP-305). Spend nothing here on keeping Doom builds beyond `fuzix-permissive`.

## APPENDIX B: AGENT MATRIX AND CRITICAL PATH

| Phase | Name | Model | Agent | Depends on | Time |
|---|---|---|---|---|---|
| 0 | Preflight | Opus | Coordinator | plan on develop | 1h |
| 1 | DP-212/718/206 records | Haiku | agent | 0 | 10m |
| 2 | Rung 2 flip | Opus | Coordinator | 0 | 45m |
| 3 | Rung 2 records | Haiku | agent | 2 | 10m |
| 4 | Probe sources | Sonnet | agent | 3 | 25m |
| 5 | Harness gates | Opus | Coordinator | 4 | 25m |
| 6 | SYS_yield | Opus | Coordinator | 5 | 1h |
| 7 | ADR-078 registry | Haiku | agent | 6 | 5m |
| 8 | Honest ENOSYS | Opus | Coordinator | 6 | 1.5h |
| 9 | BootParams, sidecar | Opus | Coordinator | 8 | 1.5h |
| 10 | Reply ADR | Opus | Coordinator + review agent | 9 | 1h |
| 11 | sbrk | Opus | Coordinator | 10 | 2.5h |
| 12 | kill | Opus | Coordinator | 11 | 1.5h |
| 13 | User pointers | Opus | Coordinator | 12 | 45m |
| 14 | mkdir | Opus | Coordinator | 13 | 1.25h |
| 15 | unlink | Opus | Coordinator | 14 | 40m |
| 16 | link | Opus | Coordinator | 15 | 40m |
| 17 | pause(n) | Opus | Coordinator | 11 | 1.5h |
| 18 | fd table | Opus | Coordinator + review agent | 17, 13 | 4.5h |
| 19 | chdir | Opus | Coordinator | 18 | 1.25h |
| 20 | pipe | Opus | Coordinator | 18 | 2.5h |
| 21 | Security pass | Opus | Coordinator | last Stop | 45m |
| 22 | Doc ripple | Sonnet | agent | 21 | 20m |
| 23 | Close | Opus | Coordinator | 22 | 45m |

Critical path, first night, every phase in order: 0 → 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8 → 9 → 10 → 11 → 12 → 21 → 22 → 23. The second night resumes at 13 in the same worktree, and runs the same three closing phases at its end. Lane E is serial: no phases overlap.

## APPENDIX C: QUICK REFERENCE

- **Consts** (`$MAIN`): `GETPID_INJECT`, `UPC_RUNG0`, `DEFERRED_SWITCH` true; `UPC_D6_MISS_FAULT` false, stays false; `UPC_RUNG2` flips here; new: `UPC_REPLY`, `UPC_RUNG_SBRK`, `UPC_RUNG_KILL`, `UPC_RUNG_MKDIR`, `UPC_RUNG_UNLINK`, `UPC_RUNG_LINK`, `UPC_RUNG_SLEEP`, `UPC_RUNG_FD`, `UPC_RUNG_CHDIR`, `UPC_RUNG_PIPE`.
- **Adding an arrival**: const; alias row; empty arm answering `-ENOSYS`; `surface.rs`; `XV6_SYSCALLS` length; `parity_surface`; ADR-078 §5 row.
- **Slot tables** (base in BootParams `reserved[0]`, byte 0x160): trapframes +0x000 stride 0x100; `cur_slot` +0x800; `trap_a7` +0x804; `fresh_mask` +0x808; `ticks` +0x80C; `brk_in` +0x810; `reply` +0x814; `reply_arg` +0x818; `cwd_mask` +0x81C. Size 0x820.
- **Tags**: `UPC_TAG_SLOT` 0x5107_0000, `UPC_TAG_A7` 0xA700_0000, `UPC_TAG_REPLY` 0x5250_0000. Reply flags: SET_BRK 1, YIELD 2, KILL 4, EXIT 8.
- **User VA**: image from 0; heap 0x10000-0x40000; fork copies 0-0x40000 and 64 KiB around SP; stack top 0x500000; argv frame 0x600000; ceiling 0x800000.
- **Physical**: BootParams 0x100; TTY MMIO 0xC001; CSR n at 0xF000 + 4n; x16/x17 spill 0x64000/0x64004; kernel `.rodata` 0x27900; kernel `.data`/`.bss` from 0x100000; ramdisk 0x800000; per-slot pgds 0xF00000 + s*0x1000; child slices 0x1000000 + (s-1)*0x800000; ceiling 0x3800000, so slots 1-5 can be live.
- **Budgets**: ceiling 900,000; ladder start 921,036; Doom 758,199 must not move. `budget` checks the first two; the harness prints the third.
- **Booting by hand**: `TRIG=3000000 ub <probe> | pl <PREFIX>`. Verifier count: `vcount`. Gate: `gate <label>`, log in `$S/gates.txt`.

## APPENDIX D: EAST AS A NODE (DP-206), ATTENDED, FOR S-E1-LIVE

Not run by this plan. DP-206 is approved: all three, guarded like the chaos-guard and `upc0`. The run has already happened once under guard, on 2026-10-28 (`docs/battle-plans/S-ASCEND-2-EAST-NODE-plan.md`, `references/s-ascend-2-log.md` §"E3 across hosts"), and what is left of M-228 is LICH-015's full arm. S-E1-LIVE carries this as its Evening 11.

Before: Stevie in the office, a keyboard and monitor reachable for east; `enp2s0` is east's only live link. Re-check three things that may have moved: east's sshd port (DP-11 moves it to 7375), its firewall (a default-deny DP-22 must pass 26601-26602 on the P2P link, so run this before that apply or add the rule), and `scripts/node-guard.sh` still at the commit that was tested.

1. **Stage (S0).** On west, build `heimdall-daemon`, `gjallarhorn-sender`, `upc-cartridgectl`, `cart`, and the heimdall-bpf objects; seal a lab manifest over scratch files with a fresh lab key. `scp` to `east:/tmp/s6/`, the guard to `east:/tmp/s6-guard/`; compare sha256 on both ends. Never production keys. `AUTH_KEY_FILE` points at a missing file.
2. **Arm the guard.** `ssh east 'sudo bash /tmp/s6-guard/node-guard.sh arm --lease 1200'`. Peer lost for about 10 s, or lease out: kills lab processes and detaches XDP.
3. **Scratch Wotan on the P2P link, on west.** `wotan -bind 192.168.13.2 -http-port 26601 -grpc-port 26602 -topic-config <lab config> & echo $! > /var/tmp/s6-stage/wotan.pid`, auth off, lab only. `ss -ltn | grep -E '26601|26602'` shows 192.168.13.2 only.
4. **`heimdall-daemon` as root on east, kprobes only (S1).** Its pid goes in `/tmp/s6/heimdall.pid`. Prove: seal verified; drifts reach the west scratch Wotan signed; a tampered or expired seal stops the daemon.
5. **XDP generic on `enp2s0` (S2).** Renew the guard. Restart heimdall with `--xdp-iface enp2s0 --xdp-mode generic`. Program returns `XDP_PASS` on every path. Watch 2 minutes: ping, WireGuard, SSH. Send horns from west, then stop heimdall and confirm no XDP on `enp2s0`.
6. **Weights (S3).** `upc-cartridgectl weights publish` on west, `weights fetch` on east, same `cart tick` output on both. `cartridge-ebpf`, if loaded, only in a netns on east, never on `enp2s0`.
7. **Teardown (S4).** `kill "$(cat /var/tmp/s6-stage/wotan.pid)"` (the 10-28 teardown missed it by path). On east: `rm -rf /tmp/s6 /tmp/s6-guard`; no heimdall, cart or guard process; `bpftool prog show` lists none of ours; no XDP on `enp2s0`.

Abort at any point, by recorded pid: `ssh east 'sudo bash /tmp/s6-guard/node-guard.sh disarm; sudo kill -KILL "$(cat /tmp/s6/heimdall.pid)"; sudo ip link set dev enp2s0 xdpgeneric off'`. With SSH gone, the guard acts by itself. `scripts/node-guard.sh:60` still calls `pkill -KILL -f` in its own remediation: a rule 14 gap outside this plan.

## APPENDIX E: OUT OF SCOPE

- ADR-109 D6 (Stevie: not yet). The pgd route stays open, and this plan's new words sit behind it like the old ones.
- Rungs 3-7 of the skeleton as written: console `write` and `read` into the kernel, retiring `fs_walk`, `FD_INODE_MAP`, `PROC_TABLE`. The freed budget lives there. It is the plan after this one.
- Exit status through `wait`: rung 7. `killstatus` stays out of the gates; `sbrkmuch` too, since a 192 KiB window cannot pass it.
- Shell pipelines and redirection onto fds 0-2: they need `dup` across the fd boundary (D10).
- `usertests` as a binary: skeleton step 2 needs sbrk and exit status.
- Timer preemption, and any wire-format change. Nothing here touches Monad v0x01.

---

*S-STAGE-2B Battle Plan — Forged 2026-11-04*
*24 Phases. 282 Steps. The kernel learns to answer, and the dead syscalls start to breathe.*
*Stop at any Stop; the goldens hold at every one.*
