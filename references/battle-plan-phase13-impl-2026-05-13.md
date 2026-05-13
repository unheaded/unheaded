# Battle Plan — Phase 1.3 IMPL (xv6 process model bring-up)

**Codename:** ASCEND-LINUX Phase 1.3 IMPL
**Authored:** 2026-05-13 (Marshal-led continuation, post Phase B + C autonomous close)
**Predecessors:**
- Phase 1.2 IMPL CLOSED (`b9572c26`)
- Phase 1.3 PRE-WORK kickoff (`abb84cfb`)
- Phase 1.3 AP-2 MRET/SRET fix SHIPPED (`73834054`)
- AP-1, AP-3, AP-4, AP-5, AP-6 documentation (`dcc9228e`, `52a075a8`, `0089a3ff`, `0459efe7`, `65c02c26`)
- **ADR-075 ACCEPTED** (`71a04796`)
**Status:** READY TO EXECUTE
**Scope target:** 5-7 days unattended (per parent battle plan §1.3) once a fresh session begins.

---

## 1. Scope & posture

Phase 1.3 wires xv6 into a real process model: SYS_FORK/SYS_EXECVE/SYS_WAITPID/SYS_EXIT/SYS_SCHED_YIELD on the live BPF interpreter. Phase 1.2 + AP-2 made this possible by landing the per-pid pgd substrate and unblocking MRET. This plan executes ADR-075's six decisions.

### What's already true (don't re-derive)

- **PROC_TABLE** = `Array<[u32; 21]> x 4` with slot[20] = page_dir_base (Phase 1.2 IMPL).
- **MRET/SRET** translate via RV2MBC_MAP (AP-2 ship `73834054`).
- **xv6 vm.c `uvmcreate(int pid)`** returns per-pid pgd for pid 0..3 (Phase 1.2 IMPL).
- **xv6 proc.c `proc_pagetable()`** passes p->pid (Phase 1.2 IMPL).
- **xv6 user/ tree vendored** at `crates/xv6-mbc/upstream/user/` (24 files: init, sh, cat, echo, ls, mkdir, kill, ln, rm, wc, plus tests).
- **mkfs ramdisk builder** at `crates/xv6-mbc/upstream/mkfs/mkfs.c` (host tool, builds ramdisk image from user/ binaries).
- **trampoline / swtch / kernelvec** adapters work as-is (AP-3 decision: keep xv6's pattern).

### What this plan delivers

1. **PROC_TABLE 4 → 8 slots** in both BPF and host emulator (D-1).
2. **PROC_TABLE MMIO isolation** verified (security item from ADR-075 §Security Review #2).
3. **SYS_EXECVE handler skeleton** in BPF (D-6 budgeted +800 insns).
4. **SYS_WAITPID handler skeleton** in BPF (+400 insns).
5. **SYS_EXIT handler** wired to slot cleanup (+300 insns).
6. **SYS_SCHED_YIELD** dispatch to existing `scheduler_context_switch` (+100 insns).
7. **RV2MBC integrity gate** in `upc-bootctl` (ADR-075 §Security #1).
8. **LR.W/SC.W priv-transition falsification tests** (ADR-075 §Security #3).
9. **xv6 user/init.c compile + embed** via mkfs into ramdisk-stub.
10. **Live boot regression**: xv6 reaches userinit → init prints "init starting" → forks sh.

### Posture

- **Marshal-safe autonomous** for sub-phases A through D; pair-call needed before Phase E (live demo) if the user-mode reaches keyboard input.
- ADR-073 zero-lint ratchet must hold for every commit.
- `--no-gpg-sign` per `feedback_unsigned_commits_when_afk`.
- No new MBC opcodes (Phase 1.3 reuses what ADR-067 / ABI v1 locked in).

---

## 2. Phase structure (6 sub-phases, 11 steps)

| Sub-phase | Steps | Time | Goal |
|-----------|-------|------|------|
| **A — Slot widening** | 1-2 | ~60 min | PROC_TABLE 4 → 8 in BPF + host + tests |
| **B — Syscall handlers** | 3-5 | ~3-4h | EXECVE, WAITPID, EXIT, SCHED_YIELD in BPF |
| **C — Security gates** | 6-7 | ~90 min | RV2MBC integrity + PROC_TABLE MMIO isolation + LR.W/SC.W falsification |
| **D — Userland build** | 8 | ~2h | xv6 user/init.c + sh.c compile to MBC, ramdisk embed |
| **E — Live boot** | 9-10 | ~60 min | xv6 advances to userinit, init prints, forks sh |
| **F — Gates + commit + report** | 11 | ~30 min | Full kingdom regression, commit chain, IMPL complete report |

Total: 5-7 days for a careful single-author shift (per parent battle plan §1.3 estimate); ~2-3 days for an aggressive paralleled shift.

---

## 3. Sub-phase A — Slot widening (Steps 1-2)

### Step 1 [W][C] — PROC_TABLE 4 → 8 in BPF

```rust
// ebpf/monad-cpu-ebpf/src/main.rs:160-168
- static PROC_TABLE: Array<[u32; 21]> = Array::with_max_entries(4, 0);
+ static PROC_TABLE: Array<[u32; 21]> = Array::with_max_entries(8, 0);
```

```rust
// ebpf/monad-cpu-ebpf/src/phase12.rs:29-31
- pub const MAX_PROCESSES: u32 = 4;
+ pub const MAX_PROCESSES: u32 = 8;
```

Update scheduler skip-mask logic in `scheduler_context_switch` (`main.rs:1725-1819`) — the `while attempts < 4` bound widens to `< 8`. The skip_mask is a `u32`, so bits 0..7 are usable without struct change.

**Verification:**
```bash
cd ebpf && cargo build --release --target=bpfel-unknown-none -Z build-std=core -p monad-cpu-ebpf --features=ascend-linux 2>&1 | tail -1
bash scripts/bpf-verifier-check.sh 2>&1 | tail -6  # expect 8.50%+/- 0.05%
```

**Cut-point:** verifier delta > 1% from baseline (which is ~+9000 insns) → revert, mark D-1 STUCK in IMPL report.

### Step 2 [W][C] — Host emulator parity + falsification test

Mirror in `crates/monad-mbc/src/execute.rs` (find `MAX_PROCESSES` constant; widen). Add a regression test:

```rust
// crates/monad-mbc/tests/os_primitives_test.rs
#[test]
fn phase13_proc_table_supports_8_slots() {
    let mut cpu = cpu_with_safe_sp();
    for pid in 0..8u8 {
        cpu.allocate_proc_slot(pid).expect("pid {pid} should fit");
    }
    assert_eq!(cpu.allocate_proc_slot(8), Err(NoSlot), "9th slot must refuse");
}
```

**Verification:** all monad-mbc tests pass. Commit: `feat(upc): Phase 1.3 Step 1-2 — PROC_TABLE 4 → 8 slots`.

---

## 4. Sub-phase B — Syscall handlers (Steps 3-5)

### Step 3 [W][C] — sys_exit + sys_sched_yield (~+400 insns)

These are the simplest, do them first. `sys_exit_handler` marks the PROC_TABLE slot UNUSED + decrements `cpu.num_processes` + clears any wait-mask entries. `sys_sched_yield_handler` just invokes `scheduler_context_switch(cpu, flow_label, hop_id)`.

Both go in the existing `INT 0x80` dispatch in `main.rs` (around line 941+) alongside SYS_WRITE and SYS_FORK.

**Verification:** verifier ≤ 8.55%. Falsification tests: `phase13_sys_exit_clears_slot`, `phase13_sys_sched_yield_switches`.

Commit: `feat(ebpf): Phase 1.3 Step 3 — SYS_EXIT + SYS_SCHED_YIELD handlers`.

### Step 4 [W][C] — sys_waitpid (~+400 insns)

Per-pid wait queue: `WAIT_MASK: Array<u32> x 1` where bit `i` set = pid `i` is waiting for a child to exit. `sys_waitpid_handler` sets the bit and suspends (via existing SCHED_STATE suspended_mask). `sys_exit_handler` (already wrote in Step 3) checks parent's wait bit and clears it.

**Verification:** verifier ≤ 8.60%. Falsification: `phase13_waitpid_unblocks_on_exit`.

Commit: `feat(ebpf): Phase 1.3 Step 4 — SYS_WAITPID + wait-queue interaction`.

### Step 5 [W][C] — sys_execve (~+800 insns)

Most complex. Open question per AP-5: does it parse an ELF in-kernel, or expect pre-translated images? **Decision for Phase 1.3**: expect pre-translated MBC images keyed by filename in a RAMDISK_MAP that the bootctl pre-populates from the ramdisk image. Skip ELF parsing entirely.

Handler shape: take `r1 = path_addr`, look up filename → MBC base offset in RAMDISK_MAP (BTF map: `HashMap<[u8; 16], u32>` filename → ROM_MAP slot). Reset cpu.regs[1..15] = 0, set cpu.pc = looked-up slot, retain page_dir_base (since this is "exec", not fork).

**Verification:** verifier ≤ 8.67% (the budget cap from AP-6). Falsification: `phase13_sys_execve_swaps_image`.

**Cut-point:** if verifier exceeds 9%, drop EXECVE scope to "noop returns ENOSYS" for Phase 1.3 and defer real EXECVE to Phase 1.4 alongside the FS work.

Commit: `feat(ebpf): Phase 1.3 Step 5 — SYS_EXECVE via pre-translated image table`.

---

## 5. Sub-phase C — Security gates (Steps 6-7)

### Step 6 [W][C] — RV2MBC integrity gate

ADR-075 §Security #1. Add SHA-256 of `.rv2mbc` to `BootParams v2` reserved bytes (the first 32 bytes of `reserved: [u32; 48]` per `cmd/upc-bootctl/src/bootparams.rs`). `populate_rv2mbc` computes SHA-256 of the bytes, compares against the BootParams field. Mismatch → fail-fast.

Build pipeline change: `crates/xv6-mbc/adapters/Makefile.mbc` adds a `target/xv6-mbc.bootparams` artifact that bakes the SHA into the BootParams blob. The bootctl reads both.

**Verification:** falsification test in `cmd/upc-bootctl/src/runner.rs::tests`: load a tampered `.rv2mbc` (flip one byte) → expect Err on populate.

Commit: `feat(upc): Phase 1.3 Step 6 — RV2MBC SHA-256 integrity gate (ADR-075 sec #1)`.

### Step 7 [W][C] — LR.W/SC.W priv-transition falsification

ADR-075 §Security #3. Add two new tests to `tests/os_primitives_test.rs`:

```rust
#[test]
fn phase13_lr_sc_survives_context_switch() {
    // LR.W reserves an address in M-mode
    // scheduler_context_switch switches to a different process
    // SC.W must FAIL (return 1) because reservation belongs to old pid
}

#[test]
fn phase13_lr_sc_survives_priv_transition() {
    // LR.W in U-mode → MRET to S-mode → SC.W in S-mode
    // Reservation must invalidate; SC.W returns 1
}
```

Both tests verify that `reservation_address = 0xFFFF_FFFF` after any priv transition or context switch (MRET/SRET handlers already clear it; this test asserts it).

Commit: `test(monad-mbc): Phase 1.3 Step 7 — LR.W/SC.W priv-transition falsification`.

---

## 6. Sub-phase D — Userland build (Step 8)

### Step 8 [W][BUILD][C] — Compile xv6 user/init.c + sh.c

`crates/xv6-mbc/upstream/user/init.c` and `user/sh.c` build via xv6's existing Makefile pattern but need a Makefile.mbc-userland sibling. Pattern:

1. Compile with `-march=rv32i_zicsr_zmmul -mabi=ilp32e -nostdlib -nostdinc -ffreestanding` (same flags as kernel-side).
2. Link with `user/user.ld` (already present in tree).
3. Translate ELF → MBC via the rv32i-to-mbc binary.
4. Embed the resulting `.mbc` + `.rv2mbc` into a `ramdisk.img` built by `mkfs` (host tool, x86-64 binary).
5. Bootctl loads `ramdisk.img` into RAM_MAP at `0x00800000` (per UPC_PAGE_TABLE_LAYOUT.md).

**Cut-point:** if `user/sh.c` references xv6 userspace syscalls our SYSCALL handler doesn't yet implement (open/read/write/fork/exec/wait/dup/pipe/close), stub them in BPF with `return -ENOSYS` and document the gap. Pure exec of init.c can proceed; sh requires the missing syscalls.

Commit: `feat(xv6-mbc): Phase 1.3 Step 8 — userland init + sh build pipeline`.

---

## 7. Sub-phase E — Live boot (Steps 9-10)

### Step 9 [V][LIVE] — Boot to userinit

Rebuild `xv6-mbc.mbc` with Step 1-7 changes, regenerate `ramdisk.img`, live boot:

```bash
cd /home/govan/tmp/unheaded/ebpf
sudo /home/govan/tmp/unheaded/cmd/upc-bootctl/target/release/upc-bootctl boot \
  --kernel /home/govan/tmp/unheaded/crates/xv6-mbc/upstream/target/xv6-mbc.mbc \
  --ramdisk /home/govan/tmp/unheaded/crates/xv6-mbc/upstream/target/ramdisk.img \
  --instance 222 2>&1 | tail -30
```

**Pass criterion:** TTY emits `"init starting\n"` (init.c first print). `priv` reaches 3 (U-mode) at least transiently. `CPU_MAP.num_processes >= 2` (init + the parent).

### Step 10 [V][LIVE] — init forks sh, sh prompts

**Pass criterion:** TTY emits `"$ "` (sh prompt). `CPU_MAP.num_processes >= 2`. PROC_TABLE[0..2] populated.

**Cut-point:** if sh halts on missing syscall, capture which syscall + commit a stub returning -ENOSYS + document the gap as a Phase 1.4 carry-over.

Commit (or rollback) per result. If passing, commit: `feat(ascend-linux): Phase 1.3 SHIP — xv6 init starts, sh prompts`.

---

## 8. Sub-phase F — Gates + commit + report (Step 11)

### Step 11 [V][C][DOC] — Full kingdom regression + IMPL complete report

```bash
cd /home/govan/tmp/unheaded
golangci-lint run ./... 2>&1 | tail -1
go build ./... && go test -short -count=1 -timeout 120s ./pkg/runtime/... ./pkg/ebpf/... 2>&1 | tail -5
(cd crates/monad-mbc && cargo test --release --tests 2>&1 | grep -E "^test result" | head -10)
(cd ebpf && for FEAT in "" --features=ascend-linux --features=phase12-option-a; do cargo build --release --target=bpfel-unknown-none -Z build-std=core -p monad-cpu-ebpf $FEAT 2>&1 | tail -1; done)
bash scripts/bpf-verifier-check.sh 2>&1 | tail -6
```

Author `references/phase13-impl-complete-YYYY-MM-DD.md` with:
- Steps landed (1-10) with commit refs
- Final verifier budget %
- Live boot TTY capture
- Cut-points hit (if any)
- Carry-overs to Phase 1.4

---

## 9. Risk register

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Step 5 SYS_EXECVE blows verifier budget past 9% | Medium | High | Cut-point in Step 5 — drop to ENOSYS stub |
| sh.c references unimplemented syscalls | High | Medium | Step 8 cut-point — ENOSYS stubs + Phase 1.4 carry-over |
| LR.W/SC.W priv-transition test reveals real bug | Low | High | Step 7 is a real test — if it catches something, branch to a fix shift |
| RV2MBC SHA gate breaks dev-loop ergonomics | Medium | Low | Provide `UPC_SKIP_RV2MBC_INTEGRITY=1` escape hatch for dev |
| 8-slot widening trips verifier state-explosion | Low | Medium | Step 1 cut-point — revert + investigate |
| Ramdisk build pipeline (mkfs host binary) fragile | Medium | Medium | Document mkfs invocation in Makefile.mbc; commit the binary artifact path |

---

## 10. Cut-points

- **Step 1 verifier > 9%:** revert; mark D-1 STUCK; Phase 1.3 stays at 4 slots.
- **Step 5 verifier > 9%:** EXECVE drops to ENOSYS; defer real EXECVE to Phase 1.4 with FS work.
- **Step 8 sh.c needs missing syscalls:** stub them ENOSYS; phase 1.3 SHIP narrows to "init runs, sh would prompt if fs works."
- **Step 9 boot regresses (no banner):** revert ALL Phase 1.3 changes; Phase 1.2 substrate stays.
- **Step 10 sh halts on missing syscall:** commit ENOSYS stub for the specific syscall + document; consider SHIP gate met if at least init.c successfully exec'd.

Every cut-point leaves a clean committed state.

---

## 11. Execution mode

- **Marshal-safe autonomous** for steps 1-7 (deterministic + verifiable).
- Step 8 may need an interactive build-debugging pass on the userland Makefile.
- Steps 9-10 (live boot) need sudo + the BPF interpreter built with `ascend-linux` feature.
- Commit cadence: one per step (11 commits expected).
- Final report: `references/phase13-impl-complete-2026-MM-DD.md`.

---

## 12. Critical files

### Modified
- `ebpf/monad-cpu-ebpf/src/main.rs` (PROC_TABLE size, syscall handlers, scheduler bounds)
- `ebpf/monad-cpu-ebpf/src/phase12.rs` (MAX_PROCESSES=8)
- `crates/monad-mbc/src/execute.rs` (host emulator MAX_PROCESSES parity)
- `crates/monad-mbc/tests/os_primitives_test.rs` (3+ new tests)
- `cmd/upc-bootctl/src/runner.rs` (RV2MBC SHA-256 integrity check)
- `cmd/upc-bootctl/src/bootparams.rs` (SHA field in reserved region)
- `crates/xv6-mbc/adapters/Makefile.mbc` (userland build target)

### Created
- `crates/xv6-mbc/adapters/Makefile.mbc-userland` (or extension to existing Makefile)
- `crates/xv6-mbc/upstream/target/ramdisk.img` (build artifact, .gitignored)
- `crates/xv6-mbc/upstream/target/init.mbc + init.rv2mbc` (build artifacts)
- `crates/xv6-mbc/upstream/target/sh.mbc + sh.rv2mbc` (build artifacts)
- `references/phase13-impl-complete-YYYY-MM-DD.md` (final report)

---

## 13. Existing functions/utilities to reuse

- `phase12::pgd_base_for_pid()` — computes 0x00F00000 + pid*0x1000
- `scheduler_context_switch()` — handles save/load including page_dir_base
- `phase12::phase12_option_a_on_context_switch()` — feature-gated TLB flush hook
- `uvmcreate(int pid)` — per-pid pgd allocator
- `populate_rv2mbc(bytes, base)` in upc-bootctl — already takes a base offset; userland uses a DIFFERENT base
- `BootParams::for_xv6` — extend with sha2 field
- `mkfs.c` — already a working ramdisk builder

---

## 14. Verification — end-to-end after all steps

```bash
cd /home/govan/tmp/unheaded
git log --oneline -15  # Steps 1-10 commits present

# Full kingdom regression
golangci-lint run ./... 2>&1 | tail -1
go build ./... && go test -short -count=1 -timeout 120s ./pkg/runtime/... ./pkg/ebpf/...
(cd crates/monad-mbc && cargo test --release --tests 2>&1 | grep -E "^test result")
(cd ebpf && for F in "" --features=ascend-linux --features=phase12-option-a; do cargo build --release --target=bpfel-unknown-none -Z build-std=core -p monad-cpu-ebpf $F; done)
bash scripts/bpf-verifier-check.sh

# Live boot
sudo /home/govan/tmp/unheaded/cmd/upc-bootctl/target/release/upc-bootctl boot \
  --kernel /home/govan/tmp/unheaded/crates/xv6-mbc/upstream/target/xv6-mbc.mbc \
  --ramdisk /home/govan/tmp/unheaded/crates/xv6-mbc/upstream/target/ramdisk.img \
  --instance 222 2>&1 | tail -30
# Expect: "init starting\n$ " in TTY output
```

If all pass, Phase 1.3 SHIPS.

---

*Free to use. Free to share.*
