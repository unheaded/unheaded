<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# EVOLUTION-1 STAGE 2 BATTLE PLAN: the migration ladder, rung 0 first

**Date**: 2026-10-28
**Status**: DRAFT. Phases 0-2 (preflight, rung 0 research, rung 0 design and ADR-109) are written. The later phases (rung 0 kernel and BPF changes and gate, rung 1 flip, rungs 2-8, usertests, security, docs) are not written yet; Phase 2 ends with the review that shapes them.
**Sprint**: EVOLUTION-1 Stage 2 (master plan Epic 1.6, 1.6.3-1.6.4)
**Forged from**: [EVOLUTION-1-STAGE-2-skeleton.md](EVOLUTION-1-STAGE-2-skeleton.md); decisions E1 = option K (one pid
numbering, init = 1; BPF reports slot + 1) and E2 = (a) (push toward kernel-authoritative syscalls, rung by rung, with
the stop rule), both answered by Stevie 2026-10-28.
**Prerequisite**: `sprint/ascend-2` at or after `bf5e5f1d` (scheduler zombie fix, option K, gates gate_nway, forkloop,
devprobe and getpidfork in the harness; ascend 924,950 verified instructions).
**Target**: every planned rung of the ladder gated green, starting with rung 0 (the owned kernel has a process for
every BPF-forked child) and rung 1 (`GETPID_INJECT` on by default), each rung's freed or added instructions measured as
an ON/OFF pair on one build, the usertests subset for each rung in the harness.
**Estimated Duration**: rungs 0-1 about 10 h; each later rung 3-8 h; the whole ladder several sessions.
**Agent Strategy**: Coordinator for every kernel or eBPF step (sudo, boots, verifier). Agents only for host-side
probes, docs and the usertests extraction (Phase 14), each in its own worktree.
**Commit Cadence**: every 5 steps and at every gate (`[C]` steps), signed, staged by name.
**Stuck Protocol**: skip after 3x a step's estimate or 2 failed debug attempts; log it STUCK; continue with the next
independent step; a rung whose gate cannot pass stops the ladder at that rung (the stop rule).

---

## LEGEND

| Tag | Meaning |
|---|---|
| [B] | Bash command |
| [V] | Verification: must pass before proceeding |
| [D] | Debug: only if the previous [V] failed |
| [W] | Write or create a file |
| [R] | Read or inspect |
| [S] | Needs sudo |
| [P] | Parallelizable with other [P] steps in the phase |
| [C] | Commit checkpoint |
| [CODE] | Code change |
| [TEST] | Test run |
| [BUILD] | Build |
| [DESIGN] | Design decision written down |
| [DECIDE] | Decision with a recommendation: proceed with it unless the Override condition holds |
| [ESCALATE] | Needs Stevie: stop the dependent steps, continue independent ones |
| [PREFLIGHT] | Hypothesis check (Phase 0) |
| [STUCK] | Skipped via the Stuck Protocol |
| [BLOCKED] | Blocked by an upstream [STUCK] |
| [DOC-UPDATE] | Downstream docs |
| [SECURITY] | Security review or hostile-input probe |
| [BARE-METAL] | Needs the real kernel and BPF on west |

**[DECIDE] format**: RECOMMENDATION, rationale, and "Override ONLY if". **[ESCALATE] format**: context, and why no
recommendation.

---

## VARIABLES

```bash
cd "$(git rev-parse --show-toplevel)" && source <(sed -n '/^export PROJECT_ROOT/,/^commitp/p' docs/battle-plans/S-UPC-WIRE-COMPUTE-battle-plan.md)
export S7="$HOME/tmp/evolution1-stage2" LOG="$PROJECT_ROOT/references/evolution1-stage2-log.md" GOLD="$HOME/tmp/golden"
# One live boot inside upc0 (the S-ASCEND-2 default path; upc0 is kept by unheaded-upc-netns.service on west).
ub() { local user="$1"; shift; (cd "$EBPF" && sudo ip netns exec upc0 env UPC_DEMO_MODE=1 "$@" "$BC" boot \
  --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" --userland "$K/$user.mbc" --triggers "${TRIG:-800000}" --instance 111 2>&1); }
gf() { grep -a 'ascii:' | sed -E 's/([0-9A-F]{4} )+//g' | grep -aoE 'F: [a-z-]+=[0-9a-z]+|parent-after-wait'; }
```

`ub getpidfork` boots the probe; `ub getpidfork UPC_VERIFIER_STATS=1` also prints the verified count. `gf` reads the
probe lines past the Phase 1.4 PC tracer. Commits use `git commit -S` directly (not `commitp`, which falls back to
unsigned), with the two attribution trailers.

---

## SITUATION (verified 2026-10-28)

| Fact | Where |
|---|---|
| User `fork()`, `exit()`, `wait()`, `getpid()` (flag off), `read`, `write`, `open`, `fstat`, `dup`, `close`, `exec`, `pause` are serviced in BPF | `ebpf/monad-cpu-ebpf/src/main.rs`, RV32I ecall path (`syscall_nr == 1..21`) |
| BPF slots: `PROC_TABLE` 8 entries of 22 words; `MAX_PROCESSES = 8` | main.rs:298; `monad-common` lib.rs:1808 |
| Kernel `proc[]` has `NPROC = 64` rows; only `userinit` calls `allocproc`, so only init has a row | `adapters/uproc.c`; `upstream/kernel/param.h` |
| Option K: BPF reports pid = slot + 1 from getpid, fork and wait; the kernel's `nextpid` starts at 1 | main.rs (fork, wait, getpid arms); S-ASCEND-2 E1 |
| `GETPID_INJECT` = false. With it on, getpid traps into the kernel: parent reads 1, a BPF-forked child reads -1 | main.rs:413; S-ASCEND-2 log Phase 1 and E1 |
| The injected trap enters `uservec`, which saves user registers at `sscratch`; `sscratch` is one CSR in the shared CSR window, written by `usertrapret` (`w_sscratch(p->trapframe)`) for the last kernel-returned process | `adapters/trampoline_mbc.S:22-60`; `adapters/utrap.c:148` |
| `usertrap` resolves `myproc()` = `mycpu()->proc`, which the kernel scheduler set to init | `adapters/utrap.c:65`; `adapters/uproc.c:477-506` |
| Kernel code in M/S mode runs flat: translation applies only at `priv_level == 3` | main.rs `translate_address` (~3625) |
| CSR window: CSR n at RAM byte `0xF000 + 4n` (sepc 0x141, scause 0x142, sstatus 0x100, stvec 0x105, sscratch 0x140) | main.rs getpid-inject arm |
| No timer interrupt is delivered to the xv6 kernel in the ascend build | main.rs ~705-722 (timer behind feature) |
| BPF wait returns slot + 1; whether it writes the exit status to `*status` is unverified | main.rs `syscall_nr == 3` |
| Budgets: ascend 924,950 (flag off), flag-on build 910,133; working limit 950,000; Doom 758,199 | `docs/policy/ebpf-insns-baseline.tsv` |

### Escalations

| ID | Question | Blocks | Fallback while open |
|---|---|---|---|
| S2-E1 | Rung 7 (fork/exec/wait to the kernel) changes init's exit from "halts the CPU" to stock xv6's "panic: init exiting". Accept the panic, or keep a halt? | Phase 12 gate wording | gates that let init exit keep spinning instead |
| S2-E2 | The kernel's `NPROC` (64) vs BPF's 8 slots: keep 8 (shadow rows only for slots 0-7) or grow `PROC_TABLE`? | Phase 12 | 8 |

---

## PHASE 0: PREFLIGHT, BASELINES, HYPOTHESES (Steps 1-22)

**Goal**: the tree, tools and baselines are as this plan assumes; every SITUATION row is checked.
**Prerequisite**: none.
**Time**: ~40 min
**Agent**: Coordinator

- [ ] **Step 1** [B] ~1m: Branch and scratch
  ```bash
  cd "$(git rev-parse --show-toplevel)" && git switch sprint/ascend-2 && git switch -c sprint/evolution1-stage2 && mkdir -p "$HOME/tmp/evolution1-stage2"
  ```
- [ ] **Step 2** [B] ~1m: Source VARIABLES (above) in this and every new shell; `type ub gf kbuild ubuild sweep | grep -c function` → 5.
- [ ] **Step 3** [V] ~30s: `5`. Fewer → the S5 helper block moved: `grep -n '^export PROJECT_ROOT' docs/battle-plans/S-UPC-WIRE-COMPUTE-battle-plan.md`.
- [ ] **Step 4** [W] ~3m: `$LOG` with a header, the HEAD and the date.
- [ ] **Step 5** [B][S] ~3m: upc0 present (the unit owns it on west)
  ```bash
  systemctl is-active unheaded-upc-netns.service; sudo /usr/local/libexec/unheaded/upc-netns.sh status | head -4
  ```
- [ ] **Step 6** [V] ~30s: `active`, both veths up. Not installed → `sudo bash "$PROJECT_ROOT/scripts/upc-netns.sh" up` for this session.
- [ ] **Step 7** [BUILD][B] ~6m: Build everything the boots use
  ```bash
  (cd crates/monad-mbc && cargo build --release 2>&1 | tail -1); kbuild >/dev/null 2>&1; (cd "$UP" && make -f ../adapters/Makefile.mbc-userland userland gates ramdisk 2>&1 | tail -1); (cd "$EBPF" && cargo build --release -p monad-cpu-ebpf --features ascend-linux 2>&1 | tail -1); (cd cmd/upc-bootctl && cargo build --release 2>&1 | tail -1); ls "$K"/*.rv2mbc.sha256 | wc -l
  ```
- [ ] **Step 8** [V] ~30s: Builds finish; ≥ 12 sidecars.
- [ ] **Step 9** [TEST][B][S] ~12m: Baselines
  ```bash
  bash scripts/upc-regression.sh > "$S7/p0-regression.log" 2>&1; echo rc=$?; grep -aE 'FAIL|verifier' "$S7/p0-regression.log"; sweep > "$S7/p0-sweep.txt" 2>&1; grep -c SAME "$S7/p0-sweep.txt"; go test ./... 2>&1 | grep -E '^(FAIL|--- FAIL)' > "$S7/p0-go-fail.txt"; wc -l < "$S7/p0-go-fail.txt"
  ```
- [ ] **Step 10** [V] ~1m: rc=0, ascend 924,950 (± toolchain drift; record), Doom 758,199, 8/8 SAME, 0 Go failures. Anything else is the KNOWN FAILURES baseline: record it in `$LOG` and do not "fix" it in this sprint unless a rung needs it.
- [ ] **Step 11** [TEST][B][S] ~3m: The probe, both flag states (the rung 0 target, measured before any change)
  ```bash
  ub getpidfork | gf | tee "$S7/p0-gf-off.txt"
  sed -i 's/const GETPID_INJECT: bool = false;/const GETPID_INJECT: bool = true;/' "$EBPF/monad-cpu-ebpf/src/main.rs" && (cd "$EBPF" && cargo build --release -p monad-cpu-ebpf --features ascend-linux 2>&1 | tail -1) && ub getpidfork UPC_VERIFIER_STATS=1 | tee "$S7/p0-on-raw.txt" | gf | tee "$S7/p0-gf-on.txt"
  sed -i 's/const GETPID_INJECT: bool = true;/const GETPID_INJECT: bool = false;/' "$EBPF/monad-cpu-ebpf/src/main.rs" && (cd "$EBPF" && cargo build --release -p monad-cpu-ebpf --features ascend-linux 2>&1 | tail -1); git status --short ebpf/ | wc -l
  ```
- [ ] **Step 12** [V] ~1m: OFF: `parent-pid=1 fork-ret=2 child-pid=2 wait-ret=2 parent-after-wait`. ON: the same except `child-pid=other`. `0` modified files under `ebpf/`. Different → the premise moved; record and re-read SITUATION before Phase 1.

### Hypotheses (one [PREFLIGHT] per SITUATION row that a rung depends on)

- [ ] **Step 13** [PREFLIGHT][R] ~3m: H1 `usertrapret` writes `sscratch` from `p->trapframe`: `grep -n 'w_sscratch' crates/xv6-mbc/adapters/utrap.c`.
- [ ] **Step 14** [PREFLIGHT][R] ~3m: H2 `uservec` reads `sscratch` and saves 31 registers there: `sed -n 20,70p crates/xv6-mbc/adapters/trampoline_mbc.S`.
- [ ] **Step 15** [PREFLIGHT][R] ~3m: H3 BPF never writes the `sscratch` CSR word: `grep -n '0x140' ebpf/monad-cpu-ebpf/src/main.rs` → none.
- [ ] **Step 16** [PREFLIGHT][R] ~3m: H4 `scheduler_context_switch` saves and restores registers, PC, flags, SP, break, pgd and rv2mbc base, and nothing in the CSR window: `grep -n 'fn scheduler_context_switch' -A90 ebpf/monad-cpu-ebpf/src/main.rs | grep -n 'mem_write_word\|save_state\['`.
- [ ] **Step 17** [PREFLIGHT][R] ~3m: H5 kernel S-mode is flat: `grep -n 'priv_level != 3' ebpf/monad-cpu-ebpf/src/main.rs`.
- [ ] **Step 18** [PREFLIGHT][R] ~5m: H6 BPF `wait` and `status`: read the `syscall_nr == 3` arm; record whether `*status` is written (rung 7 needs it).
- [ ] **Step 19** [PREFLIGHT][R] ~5m: H7 no timer interrupt reaches the ascend kernel: read main.rs ~700-730 and `adapters/utrap.c` `devintr`; record (rung 2 needs it).
- [ ] **Step 20** [W] ~5m: `$LOG` §"Hypotheses": H1-H7 match / amended, with line numbers.
- [ ] **Step 21** [C] ~1m: **COMMIT CHECKPOINT** (`docs(upc): [STAGE2 1-21] preflight, baselines, hypotheses`; only `$LOG`).
- [ ] **Step 22** [V] ~1m: **PHASE 0 EXIT GATE**: Steps 10 and 12 as expected, H1-H7 recorded. Fail → stop and re-plan (the plan assumes them).

---

## PHASE 1: RUNG 0 RESEARCH — WHERE THE CHILD'S -1 COMES FROM (Steps 23-42)

**Goal**: the exact path of a BPF-forked child's injected getpid is known: which trapframe `uservec` used, what
`myproc()` returned, what `syscall()` read as the number, and why the result is -1. Every probe is reverted.
**Prerequisite**: Phase 0.
**Time**: ~1.5 h
**Agent**: Coordinator

- [x] **Step 23** [R] ~10m: Read `adapters/utrap.c` (`usertrap`, `usertrapret`, `prepare_return`), `upstream/kernel/syscall.c` (`syscall`, `argraw`, the unknown-syscall branch), `adapters/uproc.c` (`myproc`, `mycpu`, `scheduler`) and the getpid-inject arm in main.rs. Note every value that could turn into -1.
- [x] **Step 24** *(M1-M4 in the log)* [DESIGN][W] ~10m: `$LOG` §"-1 hypotheses", each falsifiable with one probe: **M1** `syscall()` reads a garbage a7 (unknown-syscall branch, a0 = -1); **M2** `sys_getpid` runs but `myproc()` is 0 or another row; **M3** the result is written to init's trapframe and `userret` restores the child's a0 from somewhere else; **M4** `usertrap` takes a non-syscall branch (scause not 8) and `setkilled`, and the child sees -1 from a later path.
- [x] **Step 25** *(decimal, `;`-terminated: the tracer strip ate hex digits)* [CODE] ~15m: Temporary kernel probe (reverted at Step 34): at `usertrap` entry, after `myproc()`, print fixed tokens through the console MMIO (`mmio_puts` + a hex helper if present: `grep -n 'mmio_puthex\|mmio_puts' crates/xv6-mbc/adapters/*.c | head`): `UT: p=` (p->pid), `tf=` (p->trapframe), `ss=` (r_sscratch()), `a7=` (p->trapframe->a7), `sc=` (r_scause()); in `syscall()` print `SC: num=` and the return. Guard it with `#ifdef UPC_STAGE2_PROBE` and build with that define only for the probe.
- [x] **Step 26** *(define added to Makefile.mbc for the probe build)* [BUILD][B] ~4m: `(cd "$UP" && make -f ../adapters/Makefile.mbc clean kernel CFLAGS_EXTRA=-DUPC_STAGE2_PROBE 2>&1 | tail -2)` (check the variable name the Makefile accepts: `grep -n 'CFLAGS' crates/xv6-mbc/adapters/Makefile.mbc | head`); rebuild userland + gates (a kernel clean wipes `target/`).
- [x] **Step 27** [V] ~1m: Kernel builds; the probe tokens are in the image (`strings -a "$K/xv6-mbc.data" | grep -c 'UT: p='` ≥ 1, or the .data dump shows them).
- [x] **Step 28** [TEST][B][S] ~4m: Flag ON (Step 11's sed), probe boot, save
  ```bash
  ub getpidfork > "$S7/p1-probe-on.txt"; grep -a 'ascii:' "$S7/p1-probe-on.txt" | sed -E 's/([0-9A-F]{4} )+//g' | tr '·' '\n' | grep -aE 'UT:|SC:|F: ' | head -20
  ```
- [x] **Step 29** *(M1 + M2: init row and trapframe; a7 read as 3 from init's x17 spill)* [V] ~5m: Two `UT:` blocks: the parent's (p=1, tf = init's trapframe, a7 = 11) and the child's. Record the child's p, tf, ss, a7, sc. Exactly one of M1-M4 matches. None → Step 30.
- [x] **Step 30** *(not needed)* [D] ~15m: Add a dump of the full child trapframe (a0, a7, epc) at `usertrapret` exit and of `sepc` at `uservec` entry (`csrr` into a saved slot). Re-run Step 28. Two attempts, then STUCK Steps 31-42 and record what was seen: Phase 2 can still decide on the design (it does not depend on which of M1-M4), but its exit gate needs the explanation.
- [x] **Step 31** [W] ~5m: `$LOG` §"-1 explained": the mechanism, the probe lines verbatim.
- [x] **Step 32** *(0 UT lines with the flag OFF)* [B][S] ~3m: Same probe with the flag OFF (the child's getpid then stays in BPF): confirm no `UT:` for the child's getpid (the kernel is not entered) and the parent's tokens are unchanged.
- [x] **Step 33** [V] ~1m: As described. Otherwise the flag does not control what it claims: STOP, record, escalate.
- [x] **Step 34** [B] ~5m: Revert the probe completely: `git checkout -- crates/xv6-mbc/adapters/ crates/xv6-mbc/upstream/kernel/`; flag back OFF; rebuild kernel (no define), userland, gates, ascend object.
- [x] **Step 35** *(clean; kernel 4e39c82f)* [V] ~1m: `git status --short crates/xv6-mbc ebpf | wc -l` → `0`; `sha256sum "$K/xv6-mbc.mbc"` equals Phase 0's.
- [x] **Step 36** [TEST][B][S] ~2m: `ub getpidfork | gf` → the Step 12 OFF lines.
- [x] **Step 37** [V] ~30s: Identical.
- [x] **Step 38** *(no kernel hook on BPF fork)* [R] ~10m: Read how the child reaches user mode after BPF fork: BPF copies the parent's registers into the child's PROC_TABLE row and sets the child's a0 = 0; nothing tells the kernel. Confirm no kernel hook runs on BPF fork (`grep -n 'SYS_FORK\|syscall_nr == 1 ' -A5 ebpf/monad-cpu-ebpf/src/main.rs | head -30`).
- [x] **Step 39** *(four items; item 4 is the x16/x17 spill)* [W] ~5m: `$LOG` §"What rung 0 must change", in three lines: (1) the trapframe `uservec` saves into must be the running slot's; (2) `myproc()` must return the running slot's row; (3) the row must exist before the slot's first trap, or be created on it.
- [x] **Step 40** [C] ~1m: **COMMIT CHECKPOINT** (`docs(upc): [STAGE2 23-40] rung 0 research: the child's -1 explained`; `$LOG` only).
- [x] **Step 41** [V] ~30s: Tree clean except `$LOG` committed.
- [x] **Step 42** [V] ~1m: **PHASE 1 EXIT GATE**: §"-1 explained" names one mechanism with probe evidence (or the STUCK record), the tree is back to Phase 0 state.

---

## PHASE 2: RUNG 0 DESIGN AND ADR-109 (Steps 43-58)

**Goal**: one design for kernel rows per BPF slot, written as ADR-109 (Proposed), with its verifier cost estimated and
its invariants listed as checks the gate will run.
**Prerequisite**: Phase 1.
**Time**: ~1.5 h
**Agent**: Coordinator (design); one review agent (Step 54)

- [x] **Step 43** *(ADR-109 D2; trapframes now inside the table at a 256-byte stride, after review)* [DECIDE] ~10m: **How a trap finds its slot's trapframe. RECOMMENDATION: per-slot trapframes preallocated by the kernel, and BPF swaps `sscratch` on every context switch.** The kernel allocates 8 trapframe pages at boot (one per BPF slot) and publishes their physical addresses in a fixed table, `UPC_TF_TABLE`, in RAM (8 words; place it in the reserved low page, after the CSR window: pick a free address from `docs/doom/UPC_PAGE_TABLE_LAYOUT.md` and record it). `scheduler_context_switch` writes `sscratch = UPC_TF_TABLE[next]` into the CSR word; BPF fork does nothing new for the trapframe (the child's row is created on its first trap, Step 44). Rationale: `uservec` stays stock; no BPF access to kernel structures beyond one table read per switch; the trapframe is never shared. **Override ONLY if** Phase 1 shows the child's trap never reaches `uservec` (then the trap entry itself is the problem and this design does not apply).
- [x] **Step 44** *(revised: tagged `cur_slot` + `trap_a7` written at injection, `fresh_mask` set at fork; no `gen[8]`)* [DECIDE] ~10m: **How the kernel learns the running slot and creates rows. RECOMMENDATION: BPF publishes the running slot and a per-slot generation; the kernel creates or refreshes the slot's row lazily at trap entry.** BPF writes `UPC_CUR_SLOT` (one word next to `UPC_TF_TABLE`) on every switch and on boot, and increments `UPC_SLOT_GEN[slot]` (8 words) on every successful BPF fork. `myproc()` becomes: if `upc_slot_proc[cur]` is null or its `gen` differs from `UPC_SLOT_GEN[cur]`, (re)initialise that row: `pid = cur + 1` (option K), `state = RUNNING`, `trapframe = UPC_TF_TABLE[cur]`, `pagetable` = the slot's pgd per ADR-074, `parent` = the row of `PARENT_OF[cur]` (BPF publishes `PARENT_OF` in a third table, or the kernel leaves `parent` null until rung 7). Rationale: no kernel entry is needed at BPF fork time (the kernel cannot run then); rows cost nothing until a slot traps; a reused slot is detected by its generation, so no stale row survives a reap. **Override ONLY if** the verifier cost of the three table writes (Step 47) exceeds +5,000 on the ON build, then publish only `UPC_CUR_SLOT` and derive the generation from a kernel-side fork counter seen at trap entry.
- [x] **Step 45** *(D5)* [DECIDE] ~5m: **Where the kernel's rows live. RECOMMENDATION: `proc[0..7]` are the slot rows (init's existing row stays `proc[0]`, slot 0).** `allocproc` is not used for them; `userinit` keeps creating init in `proc[0]`. Rows 8..63 stay unused until rung 7 decides S2-E2. **Override ONLY if** `userinit` cannot be made to land in `proc[0]` (then keep a separate `upc_slot_proc[8]` pointer table into `proc[]`).
- [x] **Step 46** *(I1-I8 after review)* [DESIGN][W] ~15m: Invariants, each a check the gate runs: **I1** a trap from slot s saves into `UPC_TF_TABLE[s]`; **I2** `myproc()->pid == s + 1` inside any trap from slot s; **I3** after a fork the child's first trap sees a row with the new generation (no data from a previous owner of the slot); **I4** BPF remains the scheduler and lifetime authority: the kernel never changes `state` of a slot row except to mark it killed; **I5** init's flow (slot 0) is byte-identical in the corpus (goldens SAME); **I6** with the flag OFF nothing in this rung changes behaviour (the tables are written, nothing reads them).
- [x] **Step 47** *(estimate replaced by a measured bar, ADR Consequences)* [B] ~10m: Verifier cost estimate: count the new BPF work (one RAM read + one CSR write in `scheduler_context_switch`, one RAM write of `UPC_CUR_SLOT` there and at boot, one increment in fork). Compare with the memory lesson (a second *variable* RAM_MAP offset inside a loop costs +119K): these are constant addresses outside loops. Write the estimate (expect ≤ +2,000) in `$LOG`.
- [x] **Step 48** *(19dd047c, revised in the Step 57 commit)* [W] ~25m: `docs/adr/ADR-109-upc-kernel-rows-per-bpf-slot.md` (Proposed): context (Phase 1 evidence), decision (Steps 43-45), invariants (Step 46), table addresses, cost estimate, what stays BPF-authoritative, rollback (`GETPID_INJECT` off; the tables are inert when nothing reads them), and what rung 7 inherits. `docs/adr/ADR-INDEX.md` gets the row.
- [x] **Step 49** [B] ~2m: Link check of the ADR: `grep -oE '\]\([^)#]+' docs/adr/ADR-109-*.md | sed 's/](//' | while read -r p; do test -e "docs/adr/$p" || echo "BROKEN $p"; done`.
- [x] **Step 50** *(19dd047c)* [C] ~1m: **COMMIT CHECKPOINT** (`docs(adr): [STAGE2 43-50] ADR-109 kernel rows per BPF slot (Proposed)`).
- [x] **Step 51** *(kernel .bss, address in BootParams reserved[0] = 0x160)* [R] ~5m: Re-read `docs/doom/UPC_PAGE_TABLE_LAYOUT.md` and `crates/xv6-mbc/adapters/kernel-mbc.ld` for the chosen table address: it must not overlap the kernel image, BootParams (0x100), the CSR window (0xF000-0xF0FF), the ramdisk (0x800000) or the per-pid slices.
- [x] **Step 52** *(0x160 follows v3's hash at 64..96)* [V] ~1m: No overlap. Overlap → pick another address, amend ADR-109.
- [x] **Step 53** *(43d4d31f; layout revised after review)* [W] ~10m: Add the table addresses as named constants in both places they will be used: `ebpf/monad-cpu-ebpf/src/phase12.rs` (BPF) and a new `crates/xv6-mbc/adapters/upc_slots.h` (kernel), with a parity test in `cmd/upc-bootctl/tests/` (pattern: `tests/parity_slice_constants.rs`) that greps both files and fails on a mismatch.
- [x] **Step 54** *(15 findings, 3 blockers)* [B] ~20m: Independent review: one agent reads ADR-109 and Phases 3-5 below through the four lenses (Architect: invariants and authority split; Developer: test order and rollback; BlackMage: what a hostile user program can do with the new tables, e.g. write `UPC_TF_TABLE` from user mode; Scientist: whether the gate actually distinguishes "rows work" from "init happens to match") and returns findings. No code changes by the agent.
- [x] **Step 55** *(all applied; table in the log)* [W] ~15m: Apply the review findings to ADR-109 and to Phases 3-5 of this plan (record each finding and its disposition in `$LOG`).
- [x] **Step 56** *(answered differently: children CAN reach low memory today through the page-walk fallthrough; D6 closes it)* [V] ~2m: The BlackMage finding about user access to the tables is answered: the tables sit in memory no user slice maps (user VA is translated into the per-pid slice; record why the table's physical address is unreachable from priv 3), or the plan adds a check.
- [x] **Step 57** [C] ~1m: **COMMIT CHECKPOINT** (`docs(upc): [STAGE2 51-57] slot tables placed, parity test, review applied`).
- [x] **Step 58** [V] ~1m: **PHASE 2 EXIT GATE**: ADR-109 committed with addresses, invariants I1-I6, cost estimate and review dispositions; the parity test exists and passes on the constants alone.

---

## PHASE 3: RUNG 0 KERNEL SIDE, TEST FIRST (Steps 59-84)

**Goal**: The kernel implements ADR-109 D1, D2, D4 and D5, and nothing changes with the flag off.
**Prerequisite**: Phase 2 exit gate; ADR-109 as revised (79ec42ef and later).
**Time**: ~3 h
**Agent**: Coordinator (live boots need sudo inside `upc0`)

Helpers, in addition to VARIABLES:
```bash
flag() { sed -i "s/const GETPID_INJECT: bool = $1;/const GETPID_INJECT: bool = $2;/" "$EBPF/monad-cpu-ebpf/src/main.rs" \
  && (cd "$EBPF" && cargo build --release -p monad-cpu-ebpf --features ascend-linux 2>&1 | tail -1); }
ugl() { grep -a 'ascii:' | sed -E 's/([0-9A-F]{4} )+//g' | grep -aoE '(GL|GM|UT|SC|SP|PG): [^ ]*'; }
gates() { (cd "$UP" && make -f ../adapters/Makefile.mbc-userland userland gates ramdisk 2>&1 | tail -1); }
```
Probe values print in decimal and end in `;`, because the PC-tracer strip eats hex digits followed by a space (log,
Phase 1).

### Preflight

- [ ] **Step 59** [B][S] ~3m: Baseline:
  ```bash
  git status --short | wc -l; sha256sum "$K/xv6-mbc.mbc" | cut -c1-16
  ub getpidfork UPC_VERIFIER_STATS=1 | grep -aiE 'verified|processed' | tail -1
  ```
- [ ] **Step 60** [V] ~30s: 0 changed files, hash `4e39c82f9ba56079`, ascend count 921,036 (the I6 reference count). A
  different count → record it in `$LOG` as the reference and continue.

### Probes (RED first)

- [ ] **Step 61** [CODE][W] ~15m: `crates/xv6-mbc/upstream/user/getpidloop.c`, in the shape of `getpidfork.c` (fixed
  strings in if-chains, spin at the end). Three iterations of fork → child prints `GL: child-pid=N` and exits →
  parent prints `GL: fork-ret=N` and `GL: wait-ret=N`. Then `GL: done`. Under option K, with one child live at a time,
  slot 1 is reused and every N is 2.
- [ ] **Step 62** [CODE][W] ~20m: `crates/xv6-mbc/upstream/user/getpidmulti.c`.
  - The parent forks three children before waiting.
  - Child k prints `GM: cK-pid=N`, calls `pause(1)`, then prints `GM: cK-again=N` and exits. Traps from slots 1-3
    then interleave.
  - The parent prints `GM: fork-ret=N` for each fork, waits three times (`GM: wait-ret=N`), then prints `GM: done`.
  - Expected: children are 2, 3 and 4, and `again` equals `pid` for each child.
- [ ] **Step 63** [W] ~3m: Register both in `crates/xv6-mbc/adapters/Makefile.mbc-userland` (`GATE_PROGS`, `.PHONY`,
  `clean`), as the `ut_*` probes are.
- [ ] **Step 64** [BUILD][B] ~3m: `gates; ls "$K"/getpidloop.mbc "$K"/getpidmulti.mbc`.
- [ ] **Step 65** [V] ~30s: both files exist; `strings "$K/getpidmulti.mbc.data" 2>/dev/null | grep -c 'GM:'` ≥ 5, or
  the tokens are found with `grep -a 'GM:'` on the image.
- [ ] **Step 66** [TEST][B][S] ~6m: Flag OFF, so BPF answers every getpid:
  ```bash
  for p in getpidloop getpidmulti; do echo "== $p"; TRIG=1500000 ub $p | ugl | tr '\n' ' '; echo; done | tee "$S7/p3-off.txt"
  ```
- [ ] **Step 67** [V] ~1m: `getpidloop` prints child-pid=2, fork-ret=2 and wait-ret=2 three times, then `done`.
  `getpidmulti` prints three distinct child pids, with `again` equal to `pid` for each, and the wait-rets name the
  same set.
  - [D] `getpidmulti` stalls → raise `TRIG` to 3000000 (reparent2 needed 3M), then check `pause` is serviced in BPF:
    `grep -n 'syscall_nr == 13\|SYS_PAUSE\|pause' "$EBPF/monad-cpu-ebpf/src/main.rs" | head`. Two attempts, then
    replace `pause(1)` with a yield-free busy loop of 2000 iterations and record the change.
- [ ] **Step 68** [TEST][B][S] ~6m: Flag ON, today's kernel. This is the RED record:
  `flag false true; for p in getpidloop getpidmulti; do echo "== $p"; TRIG=1500000 ub $p | ugl | tr '\n' ' '; echo; done | tee "$S7/p3-red.txt"; flag true false`.
- [ ] **Step 69** [V] ~1m: Every child's pid is wrong in `p3-red.txt` (`other`, the -1 from Phase 1). The parent's
  lines match `p3-off.txt`.
- [ ] **Step 70** [CODE][W] ~10m: Harness gates in `scripts/upc-regression.sh`, in the getpidfork gate's shape, run only
  when `UPC_GATE_RUNG0=1` (else `skip "rung 0 gates (UPC_GATE_RUNG0 unset)"`): `getpidfork` with child-pid equal to
  fork-ret; `getpidloop` with the Step 67 lines; `getpidmulti` with the Step 67 lines. The expected lines are absolute
  values, not "same as OFF".
- [ ] **Step 71** [C] ~1m: **COMMIT CHECKPOINT** `test(upc): [STAGE2 61-70] getpidloop, getpidmulti probes; rung 0 gates (off by default)`.

### Kernel changes (ADR-109 D1, D2, D4, D5)

- [ ] **Step 72** [CODE][W] ~30m: `crates/xv6-mbc/adapters/upc_slots.c`, included after `proc.h` and `upc_slots.h`:
  - `static struct upc_slot_tables upc_st __attribute__((aligned(256)));`
  - `upc_slots_init()` (D2):
    - for each s: `kernel_sp = proc[s].kstack + PGSIZE`, `kernel_trap = (uint32)usertrap`, `kernel_hartid = 0`,
      `kernel_satp` as `prepare_return` writes it;
    - copy `*proc[0].trapframe` into `upc_st.tf[0].tf`, set `proc[0].trapframe = &upc_st.tf[0].tf`, `kfree` the old
      page;
    - `__sync_synchronize()`, then `*(volatile uint32 *)UPC_SLOT_TABLES_BP_BYTE = (uint32)&upc_st`.
  - `upc_slot_enter()` (D5), as the ADR states it:
    - read `cur_slot` and `trap_a7`, then write both as 0;
    - no tag → return;
    - `s >= UPC_SLOTS` → `panic("upc: cur_slot")`;
    - `r_sscratch() != (uint32)&upc_st.tf[s].tf` → `panic("upc: sscratch")`;
    - refresh when `fresh_mask & (1u << s)` or `proc[s].pid == 0` (rows 1-7 only; s = 0 is init's row and is never
      refreshed);
    - `mycpu()->proc = &proc[s]`;
    - a7 from `trap_a7` when tagged.
  - `int upc_is_slot_row(struct proc *p)`: true for rows 1-7.
  - Add `$A/upc_slots.o` to `OBJS` in `crates/xv6-mbc/adapters/Makefile.mbc`.
- [ ] **Step 73** [CODE][W] ~10m: Wire it in:
  - `crates/xv6-mbc/adapters/umain.c`: `upc_slots_init();` right after `userinit();`, before `__sync_synchronize()`.
  - `crates/xv6-mbc/adapters/utrap.c`: `upc_slot_enter();` as the first statement of `usertrap`, before `myproc()` is
    read.
  - `defs.h` prototypes, in the adapter-local header if there is one; otherwise at the end of `kernel/defs.h` under
    an `// ADR-109` comment.
- [ ] **Step 74** [CODE][W] ~15m: `crates/xv6-mbc/adapters/uproc.c` (D5 guards):
  - `allocproc` starts its scan at row 0 for `userinit` only, and skips rows 1-7 after that. Simplest: skip
    `p > &proc[0] && p < &proc[UPC_SLOTS]`.
  - `kexit`, `sched`, `sleep`, `freeproc`: `if(upc_is_slot_row(p)) panic("upc: slot row");` at entry, with `p` the
    process concerned.
- [ ] **Step 75** [CODE][W] ~10m: `crates/xv6-mbc/adapters/trampoline_mbc.S` (D4): delete the a6 store in `uservec` and
  the a6 and a7 loads in `userret`. Keep the a7 store in `uservec`. Write one comment line citing ADR-109 D4 where
  they were.
- [ ] **Step 76** [CODE][W] ~15m: Probe-build prints under `#ifdef UPC_STAGE2_PROBE` in `upc_slots.c`, decimal and
  `;`-terminated:
  - `UT:s=..;pid=..;ss=..;a7=..;rf=..;` on every tagged trap;
  - `SP:pre=..;..;` with the words at `0x64000`/`0x64004` on entry to a tagged trap, and `SP:post=..;..;` in
    `usertrapret` before `userret`;
  - `PG:f01000-nonzero=..;`, the count of non-zero words in `0xF01000..0xF01FFC`, once in `upc_slots_init`.
- [ ] **Step 77** [BUILD][B] ~4m: `kbuild 2>&1 | grep -E 'error|warning: unused' | head; gates; sha256sum "$K/xv6-mbc.mbc" | cut -c1-16 | tee "$S7/p3-khash.txt"; (cd "$PROJECT_ROOT/cmd/upc-bootctl" && cargo test --release --test parity_slot_tables 2>&1 | grep 'test result')`.
- [ ] **Step 78** [V] ~1m: no errors; a new hash (expected, since the kernel changed); parity 2 passed. On a
  `_Static_assert` failure, fix `upc_slots.h` and phase12 together, never one side.

### Flag OFF: nothing moves

- [ ] **Step 79** [TEST][B][S] ~12m: `sweep | grep -c SAME; bash scripts/upc-regression.sh 2>&1 | grep -aE 'FAIL|verifier|ALL'; ub getpidfork | gf | tr '\n' ' '`.
- [ ] **Step 80** [V] ~1m: 8 SAME; ALL GREEN; the count equals Step 60's, because the object is unchanged; getpidfork
  prints the Phase 0 lines.
  - [D] A golden differs → it is a kernel-side TTY change. Diff the `.tty` files. A new boot line (for example a
    `PG:` print leaking out of the probe build) moves behind `UPC_STAGE2_PROBE`. Two attempts, then STUCK.
- [ ] **Step 81** [TEST][B][S] ~6m: Compat check (ADR-109 finding 6): flag ON with today's BPF (no `UPC_RUNG0` yet),
  new kernel. No tag ever arrives, so the kernel must take today's path:
  `flag false true; for p in getpidfork getpidloop getpidmulti; do TRIG=1500000 ub $p | ugl | tr '\n' ' '; echo; done > "$S7/p3-compat.txt"; flag true false`.
- [ ] **Step 82** [V] ~1m: `diff <(grep -v '^==' "$S7/p3-red.txt") <(sed -n '2,3p' "$S7/p3-compat.txt")` is empty
  (`p3-red.txt` holds the getpidloop and getpidmulti lines; `p3-compat.txt` lines 2-3 are the same two probes) (the RED results repeat exactly). Init's getpid still works with the flag
  on. A difference means the no-tag path is not today's path: re-read Step 72's early return.
- [ ] **Step 83** [C] ~1m: **COMMIT CHECKPOINT** `feat(upc): [STAGE2 72-82] ADR-109 kernel side: slot tables, row refresh, guards, trampoline a6/a7`
  (the record goes in `$LOG` §Phase 3: hash, counts, the `PG:` value).
- [ ] **Step 84** [V] ~1m: **PHASE 3 EXIT GATE**: with the flag OFF, sweep 8/8 SAME, harness ALL GREEN and the count
  unchanged. The compat check repeats the RED lines. `p3-red.txt` is recorded. `GETPID_INJECT` is `false` in the tree.
  If any of this fails, do not proceed to Phase 4.

---

## PHASE 4: RUNG 0 BPF SIDE AND THE GATE (Steps 85-110)

**Goal**: BPF implements ADR-109 D3, D6 and I7 behind consts, and the rung 0 gate passes, including the mutation
check.
**Prerequisite**: Phase 3 exit gate.
**Time**: ~3-4 h
**Agent**: Coordinator

No host emulator models the injection arm (checked 2026-10-28: only `ebpf/monad-cpu-ebpf/src/main.rs` names
`GETPID_INJECT`), so the probe-build lines are the tests. Step 86 writes their asserter before the BPF code exists.

Helper:
```bash
rung() { sed -i "s/const UPC_RUNG0: bool = $1;/const UPC_RUNG0: bool = $2;/" "$EBPF/monad-cpu-ebpf/src/main.rs"; }
d6()   { sed -i "s/const UPC_D6_MISS_FAULT: bool = $1;/const UPC_D6_MISS_FAULT: bool = $2;/" "$EBPF/monad-cpu-ebpf/src/main.rs"; }
obj()  { (cd "$EBPF" && cargo build --release -p monad-cpu-ebpf --features ascend-linux 2>&1 | tail -1); }
vcount() { ub getpidprobe UPC_VERIFIER_STATS=1 | grep -aiE 'verified|processed' | tail -1; }
```

### Read, then write the asserter

- [ ] **Step 85** [R] ~10m: Read, with current line numbers recorded in `$LOG`:
  - the injection arm (`if GETPID_INJECT`, about `main.rs:2520`) down to its STVEC jump;
  - the fork arm (`syscall_nr == 1`, about `:2193`) at the point where the child slot is known;
  - `translate_address` (about `:3630`), both "return vaddr" sites;
  - `scheduler_context_switch` (about `:3450`);
  - how `mem_write_word` bounds an address.
- [ ] **Step 86** [W] ~20m: `scripts/upc-rung0-assert.awk` reads `ugl` output and exits non-zero on any violation:
  - every `UT:` line has `s` in 0..7, `pid == s + 1`, and `ss` equal to `base + 256*s`. The base is the `ss` of the
    first `s=0` line, because `tf[0]` sits at the base.
  - each s>0 has `rf=1` on its first `UT:` line after each `GL:`/`GM: fork-ret` line, and `rf=0` on any later line
    before the next fork;
  - `a7` is 11 on getpid traps and 22 on the injected-22 traps (Step 99);
  - each `SP:pre` equals the following `SP:post`.

  Print one summary line, `RUNG0-ASSERT pass N` or `RUNG0-ASSERT fail <reason>`.
- [ ] **Step 87** [TEST][B] ~3m: Feed the asserter `"$S7/p3-compat.txt"`. It has no `UT:` lines, so it must print
  `fail no UT lines`. Then feed it a hand-made line with `pid=1` for `s=1`: it must fail on pid. This proves the
  asserter can fail.

### BPF changes

- [ ] **Step 88** [CODE] ~10m: `main.rs`, next to `GETPID_INJECT`: `const UPC_RUNG0: bool = false;` and
  `const UPC_D6_MISS_FAULT: bool = false;`, each with a one-line ADR-109 comment.
- [ ] **Step 89** [CODE] ~25m: `#[inline(always)] fn inject_strap(cpu: &MbcCpuState)`:
  - `base = mem_read_word(phase12::UPC_SLOT_TABLES_BP_BYTE >> 2)`; if 0, return;
  - `s = cpu.current_pid as u32 & 7`;
  - write the sscratch CSR word, `(0xF000 + 4*0x140) >> 2`, as `base + (s << 8)`;
  - write `(base + UPC_ST_CUR_SLOT_OFF) >> 2` as `UPC_TAG_SLOT | s`;
  - write `(base + UPC_ST_TRAP_A7_OFF) >> 2` as `UPC_TAG_A7 | (cpu.regs[1] & 0xFFFF)`.

  Call it inside the `if GETPID_INJECT` arm, just before the STVEC jump, as `if UPC_RUNG0 { inject_strap(cpu); }`.
  Copy the CSR word address from the existing `CSR_SSCRATCH` constant if there is one, not the literal.
- [ ] **Step 90** [CODE] ~10m: Fork arm: `if UPC_RUNG0 { let b = mem_read_word(...BP_BYTE >> 2); if b != 0 { let w = (b + UPC_ST_FRESH_MASK_OFF) >> 2; mem_write_word(w, mem_read_word(w) | (1u32 << (child_pid & 7))); } }`.
  Place it after the child slot is final.
- [ ] **Step 91** [CODE] ~5m: I7: first line of `scheduler_context_switch`:
  `if UPC_RUNG0 && cpu.priv_level != 3 { return; }`. Use the field's actual name from `MbcCpuState`.
- [ ] **Step 92** [CODE] ~10m: D6: in `translate_address`, at both "return vaddr" miss sites:
  `if UPC_D6_MISS_FAULT && cpu.priv_level == 3 && cpu.page_dir_base != 0 { return 0xFFFF_FFFC; }`. Doom builds without
  `ascend-linux` must not change, so wrap the condition in `cfg!(feature = "ascend-linux")` as well.

### Verified counts (I6 and the cost bar)

- [ ] **Step 93** [BUILD][B][S] ~6m: Three objects, each counted:
  ```bash
  obj; vcount | tee "$S7/p4-v-off.txt"
  rung false true; obj; vcount | tee "$S7/p4-v-rung.txt"
  d6 false true; obj; vcount | tee "$S7/p4-v-rung-d6.txt"
  d6 true false; rung true false; obj
  ```
  Then the Doom object: `cargo build --release -p monad-cpu-ebpf` (no feature), with the count from
  `scripts/check-ebpf-insns.sh`.
- [ ] **Step 94** [V] ~2m: All four counts meet their bar:
  - OFF equals Step 60's count exactly (I6);
  - RUNG0 is at most +2,000 over OFF;
  - RUNG0 + D6 is at most 950,000;
  - Doom is 758,199, unchanged.

  Record all four in `$LOG`.
  - Over +5,000 for RUNG0 → [DECIDE] take the ADR-109 fallback: `cur_slot` only, with an amendment, and re-plan
    Steps 89-90. Between +2,000 and +5,000 → record it and continue.
  - OFF differs → a const guard is missing; find the unguarded write.
- [ ] **Step 95** [B] ~2m: `bash scripts/check-ebpf-insns.sh 2>&1 | tail -3` with the OFF object.
- [ ] **Step 96** [V] ~30s: the ratchet passes, since the count is unchanged.
- [ ] **Step 97** [C] ~1m: **COMMIT CHECKPOINT** `feat(upc): [STAGE2 85-96] ADR-109 BPF side behind UPC_RUNG0 and UPC_D6_MISS_FAULT (off); asserter`.

### The gate

- [ ] **Step 98** [TEST][B][S] ~10m: Flag and `UPC_RUNG0` on, normal kernel:
  ```bash
  flag false true; rung false true; obj
  for p in getpidfork getpidloop getpidmulti; do echo "== $p"; TRIG=1500000 ub $p | ugl | tr '\n' ' '; echo; done | tee "$S7/p4-on.txt"
  ```
  Then, with the flag off for comparison: `diff <(grep -v '^==' "$S7/p4-on.txt" | grep -oE '(GL|GM): [^ ]*') <(grep -v '^==' "$S7/p3-off.txt" | grep -oE '(GL|GM): [^ ]*')`.
  The getpidfork line is compared separately, against the Phase 0 lines.
- [ ] **Step 99** [V] ~2m: Each probe prints its own Step 67 lines with the flag on, and getpidfork prints child-pid=2
  and fork-ret=2.
  - [D] A child is still `other` → check `UT:` exists in a probe build (Step 100). If there is no `UT:` line, BPF's
    write did not land: dump `sscratch` and the table word in the probe print. Two attempts, then STUCK, recording
    the lines.
- [ ] **Step 100** [TEST][B][S] ~10m: The probe kernel, with `-DUPC_STAGE2_PROBE` added to `CFLAGS` in
  `crates/xv6-mbc/adapters/Makefile.mbc` for this build only:
  - `kbuild; gates`;
  - `for p in getpidloop getpidmulti; do TRIG=1500000 ub $p | ugl; done | tee "$S7/p4-probe.txt" | awk -f scripts/upc-rung0-assert.awk`.

  Also make the probe build inject 22: under `UPC_STAGE2_PROBE` only, `getpidmulti`'s children call `syscall22()`, a
  `usys`-style stub, once before `pause`. BPF's inject arm must route 22 as it does 11. Where the arm tests
  `syscall_nr == 11`, the probe object takes `11 | 22` behind a third const `UPC_PROBE_INJECT22` (default false). The
  kernel prints `pid N ...: unknown sys call 22`.
- [ ] **Step 101** [V] ~2m: `RUNG0-ASSERT pass` with N ≥ 9. Every `unknown sys call 22` line names pid 2, 3 or 4,
  never 1. The `PG:` value is recorded in `$LOG`: non-zero means the init-pgd lead is real, so write it up as a
  finding for whichever rung first maps above 8 MiB.
- [ ] **Step 102** [TEST][B][S] ~20m: D6.
  - Write `crates/xv6-mbc/upstream/user/hostile_pgd.c`:
    - the child stores `0x5A5A5A5A` to VA `0x00F00000 + 4*1023` (the last PDE of slot 0's pgd) and to the table
      address read from VA `0x160`;
    - the child exits; init (identity map) reads both words and prints `HP: pgd=same|changed` and
      `HP: tables=same|changed`.
  - Run it with D6 off, then with D6 on: `d6 false true; obj`, then `d6 true false`.
- [ ] **Step 103** [V] ~2m: D6 off prints `changed` for both, the hole is real and recorded. D6 on prints `same` for
  both. With D6 on, `sweep | grep -c SAME` is 8.
  - A golden changes with D6 on → [ESCALATE] D6 alone. Leave D6 off, record the golden diff in `$LOG`, and continue
    the rung without it; the hostile check stays RED and is reported to Stevie. Every other part of the gate still
    applies.
- [ ] **Step 104** [TEST][B][S] ~30m: Mutation check, in the probe kernel. Four variants, one `-D` each in `CFLAGS`:
  - `UPC_MUT_SHARED_TF`: `upc_slot_enter` uses `tf[0]` for every s;
  - `UPC_MUT_NO_A7`: ignores `trap_a7`;
  - `UPC_MUT_NO_REFRESH`: never refreshes;
  - `UPC_MUT_RESTORE_A67`: `trampoline_mbc.S` restores a6/a7 again.

  For each: `kbuild; gates; for p in getpidloop getpidmulti; do TRIG=1500000 ub $p | ugl; done | awk -f scripts/upc-rung0-assert.awk; echo "rc=$?"`.
- [ ] **Step 105** [V] ~2m: All four print `RUNG0-ASSERT fail` with a reason that names the broken property. A variant
  that passes means the gate is blind there: strengthen the asserter or the probe, re-run, and record what changed.
  Afterwards rebuild the normal kernel (no `-D`) and check the Phase 3 hash.
- [ ] **Step 106** [C] ~1m: **COMMIT CHECKPOINT** `test(upc): [STAGE2 98-105] rung 0 gate: probe asserts, injected 22, D6 hostile probe, mutation check`.

### Corpus, both sides

- [ ] **Step 107** [TEST][B][S] ~15m: Flags ON (`GETPID_INJECT`, `UPC_RUNG0`, and D6 if Step 103 kept it):
  - `for p in gate2 gate_nway forkloop devprobe ut_exitwait ut_twochildren ut_forktest ut_reparent2; do echo "== $p"; TRIG=3000000 ub $p | gf | tail -3; done`;
  - `sweep | grep -c SAME`;
  - `UPC_GATE_RUNG0=1 bash scripts/upc-regression.sh 2>&1 | grep -aE 'FAIL|rung 0|ALL'`.

  Then restore everything OFF: `flag true false; rung true false; d6 true false; obj`.
- [ ] **Step 108** [V] ~2m: Every probe prints the same verdict as with the flags off (the Phase 0 and Phase 5
  records). Sweep is 8. The harness is ALL GREEN with the rung 0 gates passing. With the flags back off, the count
  equals Step 60's and `git diff --stat` shows the consts `false`.
- [ ] **Step 109** [C] ~1m: **COMMIT CHECKPOINT** `test(upc): [STAGE2 107-108] rung 0 gate passed; consts off in tree`.
  `$LOG` §Phase 4 records the counts, the `PG:` value, the D6 result and the four mutation results. ADR-109 gets
  `Rung 0 gate passed <date>`; Accepted waits for Phase 7.
- [ ] **Step 110** [V] ~1m: **PHASE 4 EXIT GATE**: the skeleton's rung 0 gate as amended:
  - child pid equals `fork()`'s return;
  - ON and OFF lines agree;
  - `gate2`, `gate_nway` and forkloop pass;
  - every reused slot refreshed exactly once;
  - the asserter passes and all four mutants fail;
  - D6 is GREEN, or escalated by itself.

  Tree clean; the consts are `false`.
