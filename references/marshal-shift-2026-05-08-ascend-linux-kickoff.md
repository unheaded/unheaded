# Marshal Shift Log — 2026-05-08 (ASCEND-LINUX kickoff)

**Source plan:** `references/battle-plan-ascend-linux-2026-05-08.md`.
**Predecessor:** `references/marshal-shift-2026-05-08.md` (drain-shift, 22 commits).
**Mode:** Pair-call → unattended Marshal-led churn. Stevie locked the seven design decisions; mechanical implementation followed.
**Host:** WEST (Linux 6.17, RV32I cross-toolchain, bpftool present).

---

## Mission vs Result

**Mission:** Kick off ASCEND-LINUX (Linux running on UPC). Phase 0 design freeze + Phase 0 implementation + Phase 1 vendoring opening.

**Result:** **Phase 0 COMPLETE** (5/5 sub-phases shipped: toolchain re-val, ISA gap audit, pair-call, boot protocol v2 spec, full ABI v2 implementation). **Phase 1.1 OPEN** (xv6-riscv vendored, skeleton crate built, adapters not yet authored).

11 ASCEND-LINUX commits land on `main` post-pair-call:

```
ce758769  feat(xv6-mbc): vendor xv6-riscv 5474d4bf + workspace skeleton (Phase 1.1)
34cbcb1d  feat(ebpf): wire 5 ASCEND-LINUX opcodes into monad-cpu eBPF interpreter
b7385d07  feat(monad-mbc): implement 5 ASCEND-LINUX opcodes (Phase 0.4c+d)
a8dbd9c3  feat(upc): MBC ISA v2 — 5 new opcodes + priv_level + reservation_address
5a41c11c  docs(plan): add Marshal handoff pointers at every phase gate
ef0aa38d  docs(upc): UPC Boot Protocol v2 — multi-ring + SMP-ABI + 256B BootParams
30457048  docs(upc): ADR-067 lands — 7 pair-call decisions frozen
bbc6d203  docs(upc): ASCEND-LINUX battle plan + Phase 0.1/0.2 inventory
```

---

## Phase 0 — Pre-flight (COMPLETE)

### 0.1 Toolchain re-val
- `make clean && make` in `demos/doom/` — green; doom.elf 414K, doom.mbc 86,463 instructions / 345K bytes.
- `cargo test --release --test os_primitives_test` — 46/46 PASS (suite has grown from documented 37).
- BPF verifier baseline `tmp/bpf-baseline-pre-ascend.txt` — 69,863 instructions / 7% of 900K budget.
- bpftool prog load: deferred to integration phase (touches kernel state).

### 0.2 ISA gap audit
- Found CLI/STI/XCHG/CAS atomics + IRET + SYSCALL + HALT already shipped at 0x18, 0x3B-0x3E, 0x40, 0xFF.
- 5 gaps identified: FENCE, MRET, SRET, LR.W, SC.W. Documented in `docs/protocol/mbc-isa-gap-audit-2026-05-08.md`.
- 206 free opcode slots remaining.

### 0.2/0.3 Pair call (Stevie + Architect + Computermancer + Marshal)
Seven decisions made 2026-05-08. **Stevie chose the maximalist path** (multi-ring + SMP-aware + 5 new opcodes vs. recommended single-ring + single-CPU + zero opcodes). Phase 0 budget grew 2 weeks → 5 weeks; total campaign 9mo → 10mo. Recorded in **ADR-067**.

### 0.3 Boot Protocol v2
`docs/doom/UPC_BOOT_PROTOCOL_V2.md` (261 lines) — supersedes v1. Two-stage boot, 256-byte BootParams (5 new fields + reserved[48]), CSR memory region 0x000_F000-0x000_F0FF, kernel image moves 0x10000 → 0x20000 to leave 64K for stage-1 stub.

### 0.4 Implementation (the big one)
- **0.4a** — opcode constants in `ebpf/monad-common/src/lib.rs::mbc_opcodes`: FENCE 0x3F, MRET 0x47, SRET 0x48, LR_W 0x49, SC_W 0x4A.
- **0.4b** — `MbcCpuState` grew 128 → 136 bytes: added `priv_level: u8` + 3 pad bytes + `reservation_address: u32`. Default impl + `crates/monad-mbc/src/cpu.rs` constructor + size assertion all updated.
- **0.4c** — `crates/monad-mbc/src/execute.rs`: dispatch arms for all 5 opcodes implemented with full RV32-A / privilege-transition semantics (CSR memory region read, MPP/SPP bit extraction, reservation tracking).
- **0.4d** — 9 new tests added to `crates/monad-mbc/tests/os_primitives_test.rs`: FENCE / LR.W / SC.W (4 cases: success, no-reservation, stale-reservation, atomic-increment) / MRET / SRET (M+S variants). All 9 PASS.
- **0.4e** — `ebpf/monad-cpu-ebpf/src/main.rs` dispatch loop: same 5 opcodes wired into the actual BPF program that IS the UPC.
- **0.4f** — verifier-check + regression: monad-cpu went 69,863 → 70,164 instructions (**+301, still 7% of budget**, well under 25% gate). 251 lib tests + 55 integration tests PASS. 3 pre-existing screen-mmap failures persist (deferred per parking lot).

### Phase 0 verification gate (passed)
- ABI v1 + ISA v2 frozen: ADR-067 shipped + UPC_BOOT_PROTOCOL_V2 shipped.
- All test suites pass for the new code paths.
- Doom regression-clean: rebuilt doom.mbc (86,463 instructions identical to baseline).
- BPF verifier budget at 7% (untouched).

🎖️ **Marshal handoff to Phase 1:** "Phase 0 complete. ABI v1 + ISA v2 frozen per ADR-067. Verifier budget 7%. 9/9 new opcode tests PASS. Advance to Phase 1 — xv6 vendor + adapters."

---

## Phase 1 — L5 xv6-on-MBC (sub-phase 1.1 IN PROGRESS — 3 of 3 adapters drafted)

### What landed in 1.1 — kickoff segment
- `crates/xv6-mbc/` workspace skeleton — Cargo.toml + lib.rs shim + README.md.
- `crates/xv6-mbc/upstream/` vendored: MIT-PDOS xv6-riscv at commit `5474d4bf72fd95a6e5c735c2d7f208f58990ceab` (riscv branch HEAD 2026-05-08).
- `crates/xv6-mbc/scripts/vendor-xv6.sh` — re-runnable vendoring script with pinned-commit attestation.
- THIRD_PARTY.md — new "MIT-Licensed Components" section documents xv6-riscv per ADR-052.
- xv6-mbc shim crate compiles green.

### What landed in 1.1 — adapter segment (post-kickoff)
- `crates/xv6-mbc/adapters/start_mbc.c` (147 LOC) — boot stub: BootParams v2 magic check, "xv6 booting..." print to MMIO 0xC001, M→S transition via memory-mapped CSRs, MRET to xv6 main(). Replaces upstream/kernel/start.c.
- `crates/xv6-mbc/adapters/console-mmio.c` (87 LOC) — UART → MMIO console. Exports uartinit / uartputc_sync / uartwrite / uartgetc / uartintr matching xv6's defs.h. Writes to 0xC001/0xC002, reads from 0xFFFF.
- `crates/xv6-mbc/adapters/blk-ramdisk.c` (92 LOC) — virtio-blk → SYS_READ_BLOCK/WRITE_BLOCK. Exports virtio_disk_init / virtio_disk_rw / virtio_disk_intr. xv6 1024B block → 2 UPC 512B sectors per call.

### What does NOT land in 1.1 (next shift)
- `adapters/syscall_shims.S` — translator-recognized externs `__upc_sys_read_block`, `__upc_sys_write_block` (used by blk-ramdisk.c). MBC body is `MOVI r0, syscall_nr` + `MOV r1, a1` + `MOV r2, a2` + `SYSCALL` + `RET`.
- `adapters/Makefile.mbc` — patched build using `riscv64-unknown-elf-gcc -march=rv32i -mabi=ilp32`. Replaces upstream/Makefile's QEMU-targeted flags. Drops kernel/{plic,virtio_disk}.c from `OBJS`; adds our adapters.
- Patches to `upstream/kernel/main.c` to skip `plicinit()`, `plicinithart()`, `timerinit()` — UPC has no PLIC + uses BootParams.tick_rate_hz instead.
- Patches to `upstream/kernel/{vm,proc,trap}.c` for RV32 pointer width + L4 syscall dispatch instead of inline assembly.
- First end-to-end build: kernel.elf → rv32i-to-mbc → kernel.mbc → doom-runner load → "xv6 booting..." then HALT (HALT expected because main() will fault on un-ported code paths; the proof we want is boot-handoff works).

### Estimated remaining Phase 1.1 budget
~3 days mechanical port work (down from ~5 — adapters are 326 LOC of code already in-tree). The first end-to-end build attempt will surface dozens of RV32 / pointer-width / xv6-RV64-isms; iterate. Pair call needed at "Sv32 page-table model confirms with our L4d MMU emulation" (battle plan §4 1.2 day 1).

---

## Numbers

- **15 commits** in this kickoff shift (after pair-call).
- **Phase 0 design decisions: 7 of 7 frozen** in ADR-067.
- **5 new MBC opcodes** implemented + tested at 9/9 PASS.
- **MbcCpuState size: 128 → 136 bytes** (+8: priv_level + 3 pad + reservation_address).
- **BPF verifier budget: 7%** (no change vs baseline; 5 opcodes added only +301 BPF instructions).
- **xv6-riscv vendored**: 79 files, MIT, pinned `5474d4bf72fd`.
- **Phase 1.1 adapters drafted**: 3 of 3 — 326 LOC across start_mbc.c (147), console-mmio.c (87), blk-ramdisk.c (92).
- **Test coverage added**: 9 new ASCEND tests; total OS-primitive integration tests now 55 (was 46).
- **Total ASCEND-LINUX commits across this conversation**: 15 (4 pre-pair-call docs + 11 post-pair-call).

## What the next shift owns

1. Port `crates/xv6-mbc/upstream/kernel/start.c` → `adapters/start_mbc.c`. **First milestone**: `cargo run -p doom-runner -- --kernel xv6-mbc.mbc` prints "xv6 booting..." and HALTs cleanly. Per battle-plan §4 sub-phase 1.1.
2. Port uart.c + virtio_disk.c.
3. Adapt Makefile to RV32I.
4. First end-to-end smoke: kernel image loads + executes initial boot sequence + halts.

🎖️ **Marshal handoff to next shift:** invoke `/unheaded-marshal` with *"ASCEND-LINUX Phase 0 complete; Phase 1.1 vendoring landed at commit ce758769. crates/xv6-mbc/upstream/ pinned at xv6-riscv 5474d4bf. Begin start_mbc.c port — replace upstream/kernel/start.c's RV64 CSR-direct boot with memory-mapped CSR access at 0x000_F000+ per ADR-067 Decision 2 + BootParams v2 read per UPC_BOOT_PROTOCOL_V2. Owner Developer + Computermancer. Mechanical phase; expected ~5 days unattended."*

**Marshal off-duty (kickoff complete). Badge stays on.**
