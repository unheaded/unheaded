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

- [ ] **Step 43** [DECIDE] ~10m: **How a trap finds its slot's trapframe. RECOMMENDATION: per-slot trapframes preallocated by the kernel, and BPF swaps `sscratch` on every context switch.** The kernel allocates 8 trapframe pages at boot (one per BPF slot) and publishes their physical addresses in a fixed table, `UPC_TF_TABLE`, in RAM (8 words; place it in the reserved low page, after the CSR window: pick a free address from `docs/doom/UPC_PAGE_TABLE_LAYOUT.md` and record it). `scheduler_context_switch` writes `sscratch = UPC_TF_TABLE[next]` into the CSR word; BPF fork does nothing new for the trapframe (the child's row is created on its first trap, Step 44). Rationale: `uservec` stays stock; no BPF access to kernel structures beyond one table read per switch; the trapframe is never shared. **Override ONLY if** Phase 1 shows the child's trap never reaches `uservec` (then the trap entry itself is the problem and this design does not apply).
- [ ] **Step 44** [DECIDE] ~10m: **How the kernel learns the running slot and creates rows. RECOMMENDATION: BPF publishes the running slot and a per-slot generation; the kernel creates or refreshes the slot's row lazily at trap entry.** BPF writes `UPC_CUR_SLOT` (one word next to `UPC_TF_TABLE`) on every switch and on boot, and increments `UPC_SLOT_GEN[slot]` (8 words) on every successful BPF fork. `myproc()` becomes: if `upc_slot_proc[cur]` is null or its `gen` differs from `UPC_SLOT_GEN[cur]`, (re)initialise that row: `pid = cur + 1` (option K), `state = RUNNING`, `trapframe = UPC_TF_TABLE[cur]`, `pagetable` = the slot's pgd per ADR-074, `parent` = the row of `PARENT_OF[cur]` (BPF publishes `PARENT_OF` in a third table, or the kernel leaves `parent` null until rung 7). Rationale: no kernel entry is needed at BPF fork time (the kernel cannot run then); rows cost nothing until a slot traps; a reused slot is detected by its generation, so no stale row survives a reap. **Override ONLY if** the verifier cost of the three table writes (Step 47) exceeds +5,000 on the ON build, then publish only `UPC_CUR_SLOT` and derive the generation from a kernel-side fork counter seen at trap entry.
- [ ] **Step 45** [DECIDE] ~5m: **Where the kernel's rows live. RECOMMENDATION: `proc[0..7]` are the slot rows (init's existing row stays `proc[0]`, slot 0).** `allocproc` is not used for them; `userinit` keeps creating init in `proc[0]`. Rows 8..63 stay unused until rung 7 decides S2-E2. **Override ONLY if** `userinit` cannot be made to land in `proc[0]` (then keep a separate `upc_slot_proc[8]` pointer table into `proc[]`).
- [ ] **Step 46** [DESIGN][W] ~15m: Invariants, each a check the gate runs: **I1** a trap from slot s saves into `UPC_TF_TABLE[s]`; **I2** `myproc()->pid == s + 1` inside any trap from slot s; **I3** after a fork the child's first trap sees a row with the new generation (no data from a previous owner of the slot); **I4** BPF remains the scheduler and lifetime authority: the kernel never changes `state` of a slot row except to mark it killed; **I5** init's flow (slot 0) is byte-identical in the corpus (goldens SAME); **I6** with the flag OFF nothing in this rung changes behaviour (the tables are written, nothing reads them).
- [ ] **Step 47** [B] ~10m: Verifier cost estimate: count the new BPF work (one RAM read + one CSR write in `scheduler_context_switch`, one RAM write of `UPC_CUR_SLOT` there and at boot, one increment in fork). Compare with the memory lesson (a second *variable* RAM_MAP offset inside a loop costs +119K): these are constant addresses outside loops. Write the estimate (expect ≤ +2,000) in `$LOG`.
- [ ] **Step 48** [W] ~25m: `docs/adr/ADR-109-upc-kernel-rows-per-bpf-slot.md` (Proposed): context (Phase 1 evidence), decision (Steps 43-45), invariants (Step 46), table addresses, cost estimate, what stays BPF-authoritative, rollback (`GETPID_INJECT` off; the tables are inert when nothing reads them), and what rung 7 inherits. `docs/adr/ADR-INDEX.md` gets the row.
- [ ] **Step 49** [B] ~2m: Link check of the ADR: `grep -oE '\]\([^)#]+' docs/adr/ADR-109-*.md | sed 's/](//' | while read -r p; do test -e "docs/adr/$p" || echo "BROKEN $p"; done`.
- [ ] **Step 50** [C] ~1m: **COMMIT CHECKPOINT** (`docs(adr): [STAGE2 43-50] ADR-109 kernel rows per BPF slot (Proposed)`).
- [ ] **Step 51** [R] ~5m: Re-read `docs/doom/UPC_PAGE_TABLE_LAYOUT.md` and `crates/xv6-mbc/adapters/kernel-mbc.ld` for the chosen table address: it must not overlap the kernel image, BootParams (0x100), the CSR window (0xF000-0xF0FF), the ramdisk (0x800000) or the per-pid slices.
- [ ] **Step 52** [V] ~1m: No overlap. Overlap → pick another address, amend ADR-109.
- [ ] **Step 53** [W] ~10m: Add the table addresses as named constants in both places they will be used: `ebpf/monad-cpu-ebpf/src/phase12.rs` (BPF) and a new `crates/xv6-mbc/adapters/upc_slots.h` (kernel), with a parity test in `cmd/upc-bootctl/tests/` (pattern: `tests/parity_slice_constants.rs`) that greps both files and fails on a mismatch.
- [ ] **Step 54** [B] ~20m: Independent review: one agent reads ADR-109 and Phases 3-5 below through the four lenses (Architect: invariants and authority split; Developer: test order and rollback; BlackMage: what a hostile user program can do with the new tables, e.g. write `UPC_TF_TABLE` from user mode; Scientist: whether the gate actually distinguishes "rows work" from "init happens to match") and returns findings. No code changes by the agent.
- [ ] **Step 55** [W] ~15m: Apply the review findings to ADR-109 and to Phases 3-5 of this plan (record each finding and its disposition in `$LOG`).
- [ ] **Step 56** [V] ~2m: The BlackMage finding about user access to the tables is answered: the tables sit in memory no user slice maps (user VA is translated into the per-pid slice; record why the table's physical address is unreachable from priv 3), or the plan adds a check.
- [ ] **Step 57** [C] ~1m: **COMMIT CHECKPOINT** (`docs(upc): [STAGE2 51-57] slot tables placed, parity test, review applied`).
- [ ] **Step 58** [V] ~1m: **PHASE 2 EXIT GATE**: ADR-109 committed with addresses, invariants I1-I6, cost estimate and review dispositions; the parity test exists and passes on the constants alone.
