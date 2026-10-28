<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# S26 STAGE 2 SESSION BATTLE PLAN — 7 Phases, 71 Steps

**Date**: 2026-10-28
**Sprint**: S26 — finish forging EVOLUTION-1 Stage 2 (rungs 0 and 1 in full), then execute rung 0's research
and design and as much of rung 0's code as the session allows.
**Prerequisite**: branch `sprint/evolution1-stage2` at `dffd9254` or later (cut from `sprint/ascend-2`); Stage 2
Phase 0 done (`references/evolution1-stage2-log.md`); west's `unheaded-upc-netns.service` active; nothing pushed.
**Target**: `EVOLUTION-1-STAGE-2-battle-plan.md` has Phases 3-7 and appendices written and reviewed; its Phase 1
(the child's -1 explained) and Phase 2 (ADR-109) are executed; rung 0's probes exist and are RED in the harness.
**Estimated Duration**: 6-9 hours in one session; rung 0 code lands only if time remains.
**Agent Strategy**: Coordinator for every boot and kernel step. Phases 3-7 of the Stage 2 plan are written by the
coordinator in four appends (not one): the previous session lost two long writes mid-call. One review agent (Phase 2
here), one usertests-extraction agent (Phase 5 here), each in its own worktree.
**Commit Cadence**: every 4 steps and at every gate; `git commit -S`, staged by name; trailers
`Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>` and
`Claude-Session: https://claude.ai/code/session_01TzL3Tqd1uRkWQZikhnEXZd`.
**Stuck Protocol**: skip after 3x a step's estimate or 2 failed debug attempts; log STUCK; move to the next
independent step.

**Standing rules**: never push; no host changes beyond boots inside `upc0` (its unit owns it); never `docker compose`
on west; credentials never in the repo; no sign-off mantras in commit bodies; Doom untouched; revert every temporary
kernel probe before a commit (`git status --short crates/xv6-mbc | wc -l` → 0).

---

## LEGEND

| Tag | Meaning |
|---|---|
| [B] | Bash command |
| [V] | Verification: must pass before proceeding |
| [D] | Debug: only if the previous [V] failed |
| [W] | Write or append a file |
| [R] | Read or inspect |
| [S] | Needs sudo |
| [C] | Commit checkpoint |
| [CODE] | Code change |
| [TEST] | Test run |
| [DESIGN] | Design decision written down |
| [DECIDE] | Decision with a recommendation; proceed unless the Override holds |
| [ESCALATE] | Needs Stevie; continue with independent steps |
| [AGENT] | Delegated to a subagent in its own worktree |
| [STUCK] / [BLOCKED] | Skip Protocol markers |

## VARIABLES

```bash
cd /home/govan/tmp/unheaded/.claude/worktrees/agent-afe8185c8169a9145 && git switch sprint/evolution1-stage2 && \
source <(sed -n '/^export PROJECT_ROOT/,/^commitp/p' docs/battle-plans/S-UPC-WIRE-COMPUTE-battle-plan.md) && \
export S7="$HOME/tmp/evolution1-stage2" LOG="$PROJECT_ROOT/references/evolution1-stage2-log.md" \
       PLAN="$PROJECT_ROOT/docs/battle-plans/EVOLUTION-1-STAGE-2-battle-plan.md"
ub() { local user="$1"; shift; (cd "$EBPF" && sudo ip netns exec upc0 env UPC_DEMO_MODE=1 "$@" "$BC" boot \
  --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" --userland "$K/$user.mbc" --triggers "${TRIG:-800000}" --instance 111 2>&1); }
gf() { grep -a 'ascii:' | sed -E 's/([0-9A-F]{4} )+//g' | grep -aoE '[A-Z]+: [a-z-]+=[0-9a-z]+|parent-after-wait|done'; }
flag() { sed -i "s/const GETPID_INJECT: bool = $1;/const GETPID_INJECT: bool = $2;/" "$EBPF/monad-cpu-ebpf/src/main.rs" \
  && (cd "$EBPF" && cargo build --release -p monad-cpu-ebpf --features ascend-linux 2>&1 | tail -1); }
```

`flag false true` turns the inject on and rebuilds; `flag true false` restores it. Every live boot runs inside `upc0`.

## SITUATION (at the context reset, 2026-10-28)

| Fact | Evidence |
|---|---|
| Option K in: BPF reports pid = slot + 1; `getpidfork` is a harness gate (parent 1, child 2, fork 2, wait 2) | `sprint/ascend-2` `a4c718fd` |
| Flag ON: the child's getpid is -1; parent correct | Stage 2 log Phase 0 |
| H1-H5, H7 hold; H6 amended: BPF `wait` writes 0 as the exit status | Stage 2 log Phase 0 |
| The Stage 2 plan has Phases 0-2 (Steps 1-58) only; Phase 1 Steps 23-24 can start cold; Steps 25-34 define a temporary kernel probe in `utrap.c` + `usyscall.c` printing `UT:`/`SC:` tokens | `$PLAN` |
| A first probe run (reverted, unlogged) showed: both traps enter `usertrap` with `p = proc[0]` (pid field read 0 through the hex helper, needs a check), the same trapframe for parent and child, `SC: num=11 fn` for the parent and `SC: num=03 fn` for the child | this session, Phase 1 attempt |
| Budgets: ascend 924,950, Doom 758,199, working limit 950,000 | baseline tsv |
| Trigger halving is inside `monad_cpu` (all frames arrive at `veth-upc0`); lead: the `XDP_TX` tail-call chain | Stage 2 log side findings |
| Open for Stevie: retire `crates/upc-bootstub` (changes frozen Boot Protocol v2); S-ASCEND-2 E5 attended run | `~/tmp/next.md` |

---

## PHASE 0: RESUME (Steps 1-6)

**Goal**: the session starts from the committed state with every helper working.
**Time**: ~10 min
**Agent**: Coordinator

- [x] **Step 1** [R] ~5m: Read `~/tmp/next.md` START HERE, `$LOG`, and `$PLAN` Phases 1-2 (Steps 23-58).
- [x] **Step 2** [B] ~1m: Source VARIABLES; `git status --short | wc -l` → 0; `git log --oneline -1`.
- [x] **Step 3** [B][S] ~1m: `systemctl is-active unheaded-upc-netns.service; sudo /usr/local/libexec/unheaded/upc-netns.sh status | head -3`.
- [x] **Step 4** [V] ~30s: `active`, `upc0` present, both veths up. Else `sudo bash scripts/upc-netns.sh up`.
- [x] **Step 5** [TEST][B][S] ~3m: `ub getpidfork | gf | tr '\n' ' '` → `F: parent-pid=1 F: fork-ret=2 F: child-pid=2 F: wait-ret=2 parent-after-wait`; `grep -n 'const GETPID_INJECT' "$EBPF/monad-cpu-ebpf/src/main.rs"` → `false`.
- [x] **Step 6** [V] ~30s: **PHASE 0 EXIT GATE**: Steps 2-5 as stated. Different → read the Stage 2 log's Phase 0 and find what moved before continuing.

---

## PHASE 1: EXECUTE STAGE 2 PHASE 1 — THE CHILD'S -1 (Steps 7-18)

**Goal**: `$PLAN` Steps 23-42 done: one mechanism for the -1, with probe evidence, the probe reverted.
**Prerequisite**: Phase 0.
**Time**: ~1.5 h
**Agent**: Coordinator

- [x] **Step 7** [R][W] ~15m: `$PLAN` Steps 23-24: read the trap path, write §"-1 hypotheses" M1-M4 in `$LOG`.
- [x] **Step 8** *(decimal `;` probe; the tracer strip had eaten hex digits)* [CODE] ~15m: `$PLAN` Step 25, the probe, with one addition: print `p->pid` through a decimal helper as well as hex (the first run's `pid=0000` for init, whose pid is 1, suggests the hex helper or the field read was wrong; settle that first). Keep every added line under `#ifdef UPC_STAGE2_PROBE`; pass `-DUPC_STAGE2_PROBE` by editing `CFLAGS` in `crates/xv6-mbc/adapters/Makefile.mbc` for the probe build only.
- [x] **Step 9** [B] ~5m: `$PLAN` Steps 26-27: probe kernel built (`kbuild`), userland + gates rebuilt (`make -f ../adapters/Makefile.mbc-userland userland gates ramdisk`), tokens present in the image.
- [x] **Step 10** [TEST][B][S] ~5m: `flag false true`; `TRIG=900000 ub getpidfork > "$S7/p1-probe-on.txt"`; extract `UT:`, `SC:` and `F:` lines in order (`$PLAN` Step 28).
- [x] **Step 11** *(M1 + M2; a7 read from init's x17 spill at 0x64004)* [V] ~5m: `$PLAN` Step 29: exactly one of M1-M4 fits. Expected from the first run: M2 variant, both traps resolve `myproc()` to `proc[0]` and share init's trapframe; the child's `SC: num=03` (not 11) says the child's a7 was read from the wrong trapframe, so the kernel ran `sys_wait`-shaped logic for a getpid, returning -1. Confirm or refute with the decimal pid print and the `tf=`/`ss=` values. Does not fit → Step 12.
- [x] **Step 12** *(not needed)* [D] ~15m: `$PLAN` Step 30 (trapframe dump at `usertrapret`, `sepc` at `uservec`). Two attempts, then STUCK Steps 13-14 and record what was seen.
- [x] **Step 13** [W] ~5m: `$LOG` §"-1 explained" (`$PLAN` Step 31), probe lines verbatim.
- [x] **Step 14** [TEST][B][S] ~4m: `flag true false`; probe boot again (`$PLAN` Steps 32-33): no `UT:` for the child's getpid, parent tokens unchanged.
- [x] **Step 15** [B] ~5m: Revert (`$PLAN` Step 34): `git checkout -- crates/xv6-mbc/adapters/ crates/xv6-mbc/upstream/kernel/ ebpf/monad-cpu-ebpf/src/main.rs`; `kbuild`; userland + gates + ramdisk; ascend object.
- [x] **Step 16** [V] ~1m: `git status --short crates/xv6-mbc ebpf | wc -l` → 0; `sha256sum "$K/xv6-mbc.mbc" | cut -c1-16` → `4e39c82f9ba56079`; `ub getpidfork | gf` → the Step 5 lines.
- [x] **Step 17** *(1d78799b)* [C] ~1m: **COMMIT CHECKPOINT** `docs(upc): [STAGE2 23-41] rung 0 research: the child's -1 explained` (`$LOG` only). Tick `$PLAN` Steps 23-42 as `- [x]` with one-line notes in the same commit.
- [x] **Step 18** [V] ~1m: **PHASE 1 EXIT GATE**: §"-1 explained" names one mechanism with evidence (or a STUCK record with what was seen); tree clean.

---

## PHASE 2: EXECUTE STAGE 2 PHASE 2 — ADR-109 (Steps 19-30)

**Goal**: `$PLAN` Steps 43-58 done: ADR-109 (Proposed) with addresses, invariants I1-I6, cost estimate, review
dispositions, and the constants parity test.
**Prerequisite**: Phase 1 (or its STUCK record: the design does not depend on which of M1-M4, only the ADR's context
paragraph does).
**Time**: ~2 h
**Agent**: Coordinator; one review agent at Step 26

- [x] **Step 19** *(D1-D5 in ADR-109; no Override held)* [DECIDE] ~10m: `$PLAN` Steps 43-45 as written (per-slot trapframes published in `UPC_TF_TABLE`; BPF publishes `UPC_CUR_SLOT` and `UPC_SLOT_GEN[8]`; rows are `proc[0..7]`). Record each decision and whether its Override condition held.
- [x] **Step 20** *(0xF100 rejected: low page not provably free; tables in kernel .bss, address in BootParams reserved[0] at 0x160)* [R] ~10m: `$PLAN` Step 51: a free, user-unreachable address for the three tables (8 + 1 + 8 words): read `docs/doom/UPC_PAGE_TABLE_LAYOUT.md`, `crates/xv6-mbc/adapters/kernel-mbc.ld`, the CSR window (`0xF000-0xF0FF`), BootParams (`0x100`), and `phase12.rs` for the user slices. Candidate: `0xF100` (right after the CSR window, inside the reserved low page). Record the choice and why no user slice maps it.
- [x] **Step 21** [W] ~10m: `$PLAN` Step 46 (invariants) and Step 47 (cost estimate) in `$LOG`.
- [x] **Step 22** *(19dd047c)* [W] ~25m: `docs/adr/ADR-109-upc-kernel-rows-per-bpf-slot.md` (Proposed) + the `ADR-INDEX.md` row (`$PLAN` Step 48). Bare technical style.
- [x] **Step 23** [B] ~2m: Link check (`$PLAN` Step 49).
- [x] **Step 24** *(19dd047c)* [C] ~1m: **COMMIT CHECKPOINT** `docs(adr): [STAGE2 43-50] ADR-109 kernel rows per BPF slot (Proposed)`.
- [x] **Step 25** *(43d4d31f; struct offsets instead of three byte addresses)* [CODE][W] ~15m: `$PLAN` Step 53: constants in `ebpf/monad-cpu-ebpf/src/phase12.rs` (`UPC_TF_TABLE_BYTE`, `UPC_CUR_SLOT_BYTE`, `UPC_SLOT_GEN_BYTE`) and `crates/xv6-mbc/adapters/upc_slots.h`; parity test `cmd/upc-bootctl/tests/parity_slot_tables.rs` (copy the shape of `tests/parity_slice_constants.rs`); `cd cmd/upc-bootctl && cargo test --release --test parity_slot_tables`.
- [x] **Step 26** *(15 findings, 3 blockers)* [AGENT] ~20m: `$PLAN` Step 54, the four-lens review of ADR-109. Brief: read-only, own worktree, return findings as a list with severity; no edits. Include the BlackMage question explicitly: can user code at priv 3 reach the table address, and can a user program forge a generation.
- [x] **Step 27** [W] ~15m: Apply dispositions (`$PLAN` Step 55); record each in `$LOG`.
- [x] **Step 28** *(D6 + qualified security note)* [V] ~2m: `$PLAN` Step 56: the user-reachability finding is answered in the ADR.
- [x] **Step 29** [C] ~1m: **COMMIT CHECKPOINT** `docs(upc): [STAGE2 51-57] slot tables placed, parity test, review applied`; tick `$PLAN` Steps 43-58.
- [x] **Step 30** [V] ~1m: **PHASE 2 EXIT GATE** = `$PLAN` Step 58.

---

## PHASE 3: FORGE STAGE 2 PHASES 3-4 — RUNG 0 KERNEL AND BPF (Steps 31-40)

**Goal**: `$PLAN` gains Phase 3 (rung 0, kernel side) and Phase 4 (rung 0, BPF side), numbered Steps 59-110, each
step with its command, in two separate appends.
**Prerequisite**: Phase 2 (the design the steps implement).
**Time**: ~1.5 h
**Agent**: Coordinator

- [x] **Step 31** [DESIGN] ~10m: Phase 3 outline in `$LOG` first (so a lost write loses nothing): probes `getpidloop` (3 sequential forks, slot reuse → generation check) and `getpidmulti` (3 concurrent children); `upc_slots.c` with `upc_slots_init()` (8 trapframe pages via `kalloc`, written to `UPC_TF_TABLE`), `upc_slot_proc()` (lazy row init keyed by `UPC_CUR_SLOT` and generation; pid = slot + 1; trapframe from the table; pagetable per ADR-074; parent null until rung 7); `myproc()` returns `upc_slot_proc()` when the trap came from user mode; `userinit` keeps `proc[0]`; `usertrapret` writes `sscratch` from the slot's trapframe; a kernel self-check at boot printing nothing on success.
- [x] **Step 32** [W] ~25m: Append Phase 3 to `$PLAN` (Steps 59-84): test-first (probes, RED gates behind `UPC_GATE_RUNG0=1`), kernel changes, build, flag-OFF corpus unchanged (sweep 8/8), `[C]` every 4 steps, exit gate "flag OFF: sweep 8/8, harness green, both probes RED with the flag ON recorded".
- [x] **Step 33** *(22 → 48)* [V] ~2m: `grep -c '^- \[ \] \*\*Step' "$PLAN"` grew by 26; every `[B]` has a following `[V]`; the phase ends in an exit gate.
- [x] **Step 34** [C] ~1m: **COMMIT CHECKPOINT** `docs(upc): [STAGE2 plan] Phase 3 forged (rung 0 kernel side)`.
- [x] **Step 35** [DESIGN] ~10m: Phase 4 outline in `$LOG`: BPF writes `UPC_CUR_SLOT` at boot (in `boot_prepare`'s CPU init, host side, and in the program on every switch), `scheduler_context_switch` sets `sscratch = UPC_TF_TABLE[next]` (one `mem_read_word` + one `mem_write_word`, constant addresses) and `UPC_CUR_SLOT = next`; `SYS_FORK` increments `UPC_SLOT_GEN[child]`; the hijack-probe shape for a verifier delta; the emulator mirrors the same three writes with unit tests.
- [x] **Step 36** [W] ~25m: Append Phase 4 to `$PLAN` (Steps 85-110): emulator tests first (RED), eBPF change, verifier count recorded (bar: ≤ +2,000 over 924,950, else Override from `$PLAN` Step 44), flag ON: `getpidfork`, `getpidloop`, `getpidmulti` all expected lines; `gate2`, `gate_nway`, forkloop, devprobe still green; sweep 8/8 with the flag OFF; `UPC_GATE_RUNG0` gates turned on; exit gate = rung 0 gate from the skeleton.
- [x] **Step 37** *(48 → 74; gate text as amended)* [V] ~2m: Step count grew by 26; the rung 0 gate text matches the skeleton's (child pid equals `fork()`'s return; ON and OFF print the same lines; no leaked row across forkloop's three iterations).
- [x] **Step 38** [C] ~1m: **COMMIT CHECKPOINT** `docs(upc): [STAGE2 plan] Phase 4 forged (rung 0 BPF side and gate)`.
- [ ] **Step 39** [B] ~1m: `grep -n '^## PHASE' "$PLAN"` lists Phases 0-4.
- [ ] **Step 40** [V] ~30s: **PHASE 3 EXIT GATE**: Phases 3-4 committed, each with a gate; tree clean.

---

## PHASE 4: FORGE STAGE 2 PHASES 5-7 AND APPENDICES (Steps 41-50)

**Goal**: `$PLAN` gains Phase 5 (rung 1: flip `GETPID_INJECT` on by default, freed-insns pair, docs), Phase 6
(rung 0-1 security review: hostile user programs against the slot tables and the trap path, fuzz where bytes are
parsed), Phase 7 (docs, registers, handoff), and Appendices A-C. Rungs 2-8 stay as the skeleton's text plus a
one-paragraph "forge when rung 1 is green" note each.
**Prerequisite**: Phase 3.
**Time**: ~1.5 h
**Agent**: Coordinator

- [x] **Step 41** [W] ~20m: Append Phase 5 (Steps 111-130): flip the const, OFF/ON verifier pair on one build recorded, `getpidprobe` + `getpidfork` identical lines, harness + sweep, `EVOLUTION-1-TRAP-FLIP.md` §Stage 1 "CLOSED <date>", master plan 1.3.5 `[x]`, `wiki/UPC-Dream-Ladder.md` L5 row, CLAUDE.md one sentence; exit gate.
- [x] **Step 42** *(76d5bb3a)* [C] ~1m: **COMMIT CHECKPOINT** `docs(upc): [STAGE2 plan] Phase 5 forged (rung 1 flip)`.
- [x] **Step 43** [W] ~20m: Append Phase 6 (Steps 131-146): attack-surface table (new kernel reads of BPF-written words; a user program that forges `sscratch`-adjacent memory cannot, since the table is outside every slice: prove with a probe that reads the table VA and faults/returns 0), a hostile probe per new syscall path, `fuzz_mbc_execute` 300 s unchanged, repo gates; exit gate.
- [x] **Step 44** *(plus `sweep12`: the harness checks no goldens)* [W] ~15m: Append Phase 7 (Steps 147-160): `$LOG` close, timeline entry + mirrors (the timeguru syncer pattern from S-ASCEND-2 Step 306), OUTSTANDING M-220/M-221 rows, ADR-109 → Accepted if the rung 0 gate passed, `~/tmp/next.md`, final regression; exit gate = Stage 2 rungs 0-1 exit.
- [x] **Step 45** [C] ~1m: **COMMIT CHECKPOINT** `docs(upc): [STAGE2 plan] Phases 6-7 forged`.
- [x] **Step 46** [W] ~20m: Appendices: A emergency procedures (verifier over 950,000: the Override; a boot that halts at `uservec`: `ROM-FAULT CTX` reading; a probe that prints nothing: the PC tracer strip and `TRIG`; a kernel that panics at `upc_slots_init`: `kalloc` exhaustion under `PHYSTOP=0x800000`); B agent matrix + critical path (Phase 1 → 2 → 3 → 4 → 5 → 7; 6 parallel to 5); C quick reference (CSR window addresses, PROC_TABLE layout, the three slot tables, `ub`/`gf`/`flag`, the harness gate list).
- [x] **Step 47** *(no rung headings existed in the plan; one RUNGS 2-8 section added)* [W] ~5m: Rungs 2-8: under each skeleton rung heading in `$PLAN`, one paragraph "forge when rung 1 is green; prerequisites and gate as in the skeleton", and the forge stamp.
- [x] **Step 48** *(160 steps, 8 exit gates, FORGED)* [V] ~3m: `grep -c '^- \[ \] \*\*Step' "$PLAN"` ≥ 160; every phase has an exit gate (`grep -c 'EXIT GATE' "$PLAN"` ≥ 8); `$PLAN` header Status line changed from DRAFT to FORGED with the date.
- [x] **Step 49** [C] ~1m: **COMMIT CHECKPOINT** `docs(upc): [STAGE2 plan] appendices; Stage 2 plan forged (rungs 0-1 in full)`.
- [x] **Step 50** [V] ~30s: **PHASE 4 EXIT GATE**: the plan is complete for rungs 0-1; master plan 1.6.4 `[x]` in the same commit.

---

## PHASE 5: USERTESTS EXTRACTION, IN PARALLEL (Steps 51-56)

**Goal**: the skeleton's usertests plan step 1 starts: the rung 7 and rung 8 test bodies the harness will need exist
as gate2-style probes, built and registered, not yet gated (their syscalls are not kernel-side yet).
**Prerequisite**: Phase 0 (independent of Phases 1-4).
**Time**: ~1 h agent time
**Agent**: one agent, own worktree, no sudo

- [x] **Step 51** *(a7fd6250)* [AGENT] ~45m: Brief: create worktree from `sprint/evolution1-stage2`; read `crates/xv6-mbc/upstream/user/usertests.c` and extract `exitwait`, `twochildren`, `forktest`, `reparent2` into `user/ut_<name>.c` with fixed-string verdicts (`UT-<name>: ok` / `UT-<name>: fail`), each spinning at the end (init never exits); register in `Makefile.mbc-userland` (`GATE_PROGS`, `.PHONY`, `clean`); build them; do not add harness gates; commit signed with the trailers; report which tests needed `sbrk` or an exit status and were skipped.
- [x] **Step 52** [V] ~5m: The agent's branch builds the probes (`ls "$K"/ut_*.mbc` after `make … gates` in its worktree) and its report lists skipped tests with reasons.
- [x] **Step 53** *(0432192f)* [B] ~3m: Merge the agent branch into `sprint/evolution1-stage2` (`git merge -S --no-ff`); rebuild gates in the main worktree.
- [x] **Step 54** *(exitwait, twochildren ok; forktest and reparent2 hung: BPF wait bug, fixed 3f64b2f4, now gated)* [TEST][B][S] ~6m: Boot each `ut_*` probe once with the flag OFF: `for p in exitwait twochildren forktest reparent2; do echo "== $p"; TRIG=1500000 ub ut_$p | gf; done`. Record which pass today on BPF (they run against BPF fork/wait).
- [x] **Step 55** *(3f64b2f4: probes gated after the fix)* [C] ~1m: **COMMIT CHECKPOINT** `test(upc): [STAGE2] usertests probes exitwait, twochildren, forktest, reparent2 (not gated)`.
- [x] **Step 56** [V] ~1m: **PHASE 5 EXIT GATE**: probes committed and built; results recorded in `$LOG`.

---

## PHASE 6: EXECUTE STAGE 2 PHASE 3 — RUNG 0 KERNEL SIDE (Steps 57-64)

**Goal**: if time remains, `$PLAN` Phase 3 (Steps 59-84) runs: probes RED, kernel changes in, flag-OFF corpus
unchanged.
**Prerequisite**: Phases 2 and 3; at least 2 hours left in the session.
**Time**: ~2.5 h
**Agent**: Coordinator

- [ ] **Step 57** [DECIDE] ~2m: **Start Phase 3 of `$PLAN` now? RECOMMENDATION: yes if ≥ 2 h remain and Phases 1-4 here are committed; else stop at Phase 7.** Override ONLY if Phase 1 ended STUCK (the design would be built on an unexplained -1).
- [ ] **Step 58** [B] ~60m: Execute `$PLAN` Steps 59-66 (probes, RED, harness gates behind `UPC_GATE_RUNG0=1`).
- [ ] **Step 59** [C] ~1m: **COMMIT CHECKPOINT** (the `$PLAN` Step 66 commit).
- [ ] **Step 60** [B] ~60m: Execute `$PLAN` Steps 67-78 (kernel changes, build, flag-OFF corpus).
- [ ] **Step 61** [V] ~5m: Sweep 8/8 SAME with the flag OFF; harness green; `git status --short crates/xv6-mbc | wc -l` → only the intended files.
- [ ] **Step 62** [C] ~1m: **COMMIT CHECKPOINT** (the `$PLAN` Phase 3 commit).
- [ ] **Step 63** [B] ~20m: Execute the rest of `$PLAN` Phase 3 to its exit gate, or STUCK-log where it stops.
- [ ] **Step 64** [V] ~1m: **PHASE 6 EXIT GATE** = `$PLAN` Phase 3 exit gate, or a STUCK record naming the step.

---

## PHASE 7: CLOSE AND HANDOFF (Steps 65-71)

**Goal**: the next session can cold-start.
**Prerequisite**: whatever ran.
**Time**: ~25 min
**Agent**: Coordinator

- [ ] **Step 65** [TEST][B][S] ~12m: Final regression on the final tree: `bash scripts/upc-regression.sh 2>&1 | grep -aE 'FAIL|verifier|ALL'`; `sweep | grep -c SAME`; `go test ./... 2>&1 | grep -c '^FAIL'`.
- [ ] **Step 66** [V] ~1m: ALL GREEN, 8/8, 0; ascend ≤ 950,000 recorded in `$LOG` §Close with the flag state.
- [ ] **Step 67** [B][S] ~1m: Host clean: `sudo bpftool prog show | grep -ciE 'monad|cart|heimdall'` → 0; `ip netns list` → `upc0` only; `git status --short | wc -l` → 0.
- [ ] **Step 68** [W] ~10m: `~/tmp/next.md` START HERE rewritten: branch and tip, which `$PLAN` phases are executed vs forged vs unforged, STUCK records, the two open decisions for Stevie (bootstub; E5), the trigger-halving lead, budgets.
- [ ] **Step 69** [W] ~5m: `$LOG` §Close: steps done here, commit count (`git log --oneline sprint/ascend-2..HEAD | wc -l`).
- [ ] **Step 70** [C] ~1m: **FINAL COMMIT** `docs(upc): [S26] Stage 2 session close; handoff`.
- [ ] **Step 71** [V] ~1m: **SESSION EXIT GATE**: tree clean, not pushed, `next.md` written, every phase here passed or STUCK-logged.

---

## APPENDIX A: EMERGENCY PROCEDURES

**A1. A long plan append is cut off mid-write.** `tail -c 400 "$PLAN"` to see where it stopped; `git diff --stat`
shows whether anything landed. Re-append the missing part in a smaller piece (one phase per write, at most ~30
steps). Never rewrite the whole file.

**A2. The probe kernel does not build.** `grep -iE ' error' "$S7"/*kbuild*.log`; the usual cause is a missing
`extern` for `mmio_puts`/`mmio_putc` (declared in `adapters/start_mbc.c`, not a header) or `uint32` vs `uint64`
(4 bytes on ilp32e, `types.h` asserts it).

**A3. A boot prints no probe tokens.** The TTY ring is 2208 bytes and the Phase 1.4 PC tracer prints 100 PCs after
every SRET; raise or lower `TRIG` by 100,000 to move the window, and strip with `sed -E 's/([0-9A-F]{4} )+//g'`.

**A4. The verifier rejects or exceeds 950,000 after the BPF change.** Bisect with `UPC_VERIFIER_STATS=1` per edit;
never add a second variable RAM_MAP offset inside a loop (memory lesson, +119K); the slot tables are constant
addresses, so a jump of more than ~3,000 means a loop picked them up.

**A5. `git commit -S` fails.** The key is not cached: stop and report; do not commit unsigned (`commitp` would).

**A6. upc0 is gone.** `sudo systemctl restart unheaded-upc-netns.service` (west owns it); for a scratch host `sudo
bash scripts/upc-netns.sh up`.

## APPENDIX B: AGENT MATRIX AND CRITICAL PATH

| Phase | Agent | Parallel with | Depends on | Time |
|---|---|---|---|---|
| 0 Resume | Coordinator | - | - | 10 min |
| 1 Stage 2 Ph1 (the -1) | Coordinator (sudo) | 5 | 0 | 1.5 h |
| 2 Stage 2 Ph2 (ADR-109) | Coordinator + review agent | 5 | 1 | 2 h |
| 3 Forge Ph3-4 | Coordinator | 5 | 2 | 1.5 h |
| 4 Forge Ph5-7 + appendices | Coordinator | 5 | 3 | 1.5 h |
| 5 usertests probes | Agent | 1-4 | 0 | 1 h |
| 6 Execute Ph3 (optional) | Coordinator (sudo) | - | 3 | 2.5 h |
| 7 Close | Coordinator | - | all | 25 min |

Critical path: 0 → 1 → 2 → 3 → 4 → 7 (about 7 h); Phase 6 only if time remains.

## APPENDIX C: QUICK REFERENCE

- CSR window: CSR n at RAM byte `0xF000 + 4n`; sstatus 0x100, stvec 0x105, sscratch 0x140, sepc 0x141,
  scause 0x142. BootParams at 0x100. Ramdisk at 0x800000. `PHYSTOP=0x800000`.
- PROC_TABLE: 8 slots × 22 words (r0-r15, PC 16, flags 17, SP copy 18, brk 19, pgd 20, rv2mbc base 21).
- Pids: slot + 1 (option K). Init = slot 0 = pid 1.
- Flag: `const GETPID_INJECT: bool` in `ebpf/monad-cpu-ebpf/src/main.rs` (~line 428).
- Harness gates today: 12 xv6 corpus lines, gate2, gate_nway, forkloop, devprobe, getpidfork, uinit ×2, Doom load,
  cartridge ×3. `bash scripts/upc-regression.sh`.
- Verified budgets: ascend 924,950 / Doom 758,199 / cart 42,743 + 146,897 (`docs/policy/ebpf-insns-baseline.tsv`;
  `scripts/check-ebpf-insns.sh --update --from` after a reviewed change).
- Live boot: inside `upc0`, `UPC_DEMO_MODE=1` inside `sudo env`, `.rv2mbc.sha256` sidecars from the Makefiles.

---

*S26 Battle Plan — Forged 2026-10-28*
*7 Phases. 71 Steps. The ladder gets its first rung written down to the command, and the kernel learns which process is trapping.*
