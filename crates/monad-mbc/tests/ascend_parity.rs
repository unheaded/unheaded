// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.
//! M-186.1 (decision pack row 41, ADR-135 RV-3 first half): the host engine
//! `execute.rs` against what the ascend object does at U-mode
//! (`ebpf/monad-cpu-ebpf/src/main.rs`, feature `ascend-linux`):
//!
//! - MRET and SRET at priv 3 do nothing: the arms do not match and the opcode
//!   is the unknown-opcode NOP (ADR-109 D6, ADR-135 R-F1).
//! - D6: a U-mode page-walk miss with a page directory resolves to
//!   `D6_MISS_ADDR` (0xFFFF_FFFC), outside RAM, so a load reads 0 and a store
//!   is dropped, instead of reaching the physical address that equals the VA.
//! - A PDE with `PTE_LEAF` is a 4 MiB superpage (ADR-074 large pages).
//! - XCHG, CAS, LR.W and SC.W at U-mode go through the same translation
//!   (`UPC_ATOMIC_TRANSLATE`, ADR-135 D17 part 2).
//!
//! These pin the BPF behaviour as written in main.rs; the BPF side is run by
//! the harness (`scripts/upc-regression.sh`, hostile_reply, hostile_pgd).

use monad_common::{mbc_mmu as mmu, mbc_opcodes as op, MbcInsn};
use monad_mbc::execute::Cpu;

const MEPC: usize = (0xF000 + 0x341 * 4) >> 2;
const MSTATUS: usize = (0xF000 + 0x300 * 4) >> 2;
const SEPC: usize = (0xF000 + 0x141 * 4) >> 2;
const SSTATUS: usize = (0xF000 + 0x100 * 4) >> 2;

/// A slot's page directory (inside the 0x00F00000 pgd region) and one second-
/// level table for it.
const PGD: u32 = 0x00F0_1000;
const PT: u32 = 0x00F2_0000;
/// User VA that maps (PDE 0, PTE 0x10) and its physical page.
const VA_MAPPED: u32 = 0x0001_0000;
const PA_MAPPED: u32 = 0x0050_0000;
/// User VA with no PDE (PDE 2), and the physical word an identity fall-through
/// would reach.
const VA_NO_PDE: u32 = 0x0080_0000;
/// User VA under PDE 0 whose PTE is clear.
const VA_NO_PTE: u32 = 0x0002_0000;

fn enc(o: u8, d: u8, s: u8, imm: u16) -> u32 {
    MbcInsn::encode(o, d, s, imm).0
}

fn w(addr: u32) -> usize {
    (addr >> 2) as usize
}

/// A U-mode CPU with mmu on and a page directory: PDE 0 points at `PT`, whose
/// entry 0x10 maps `VA_MAPPED` to `PA_MAPPED`; PDE 2 and PTE 0x20 are clear.
fn user_cpu() -> Cpu {
    let mut cpu = Cpu::new();
    // The host's default RAM ends at 12 MiB; the pgd region is at 15 MiB.
    cpu.ram.resize(0x0100_0000 >> 2, 0);
    cpu.state.regs[15] = 0x0040_0000;
    cpu.state.mmu_enabled = 1;
    cpu.state.priv_level = 3;
    cpu.state.page_dir_base = PGD;
    cpu.ram[w(PGD)] = PT | mmu::PTE_PRESENT | mmu::PTE_USER | mmu::PTE_WRITE;
    cpu.ram[w(PT) + 0x10] = PA_MAPPED | mmu::PTE_PRESENT | mmu::PTE_USER | mmu::PTE_WRITE;
    cpu
}

fn run(cpu: &mut Cpu, rom: &[u32]) {
    cpu.load_rom(rom);
    let _ = cpu.run(100);
}

// ── MRET / SRET at priv 3 ───────────────────────────────────────────────────

#[test]
fn mret_at_user_mode_does_nothing() {
    let mut cpu = user_cpu();
    cpu.ram[MEPC] = 4 * 3; // would land on the HALT at 3, skipping the ADDIs
    cpu.ram[MSTATUS] = 0b11 << 11; // MPP = M
    cpu.state.reservation_address = 0x1234;
    run(
        &mut cpu,
        &[
            enc(op::MRET, 0, 0, 0),
            enc(op::ADDI, 0, 0, 1),
            enc(op::ADDI, 0, 0, 1),
            enc(op::HALT, 0, 0, 0),
        ],
    );
    assert_eq!(
        cpu.state.priv_level, 3,
        "a U-mode mret took MPP's privilege"
    );
    assert_eq!(cpu.state.regs[0], 2, "a U-mode mret jumped to MEPC");
    assert_eq!(
        cpu.state.reservation_address, 0x1234,
        "a refused mret is a NOP"
    );
}

#[test]
fn sret_at_user_mode_does_nothing() {
    let mut cpu = user_cpu();
    cpu.ram[SEPC] = 4 * 3;
    cpu.ram[SSTATUS] = 1 << 8; // SPP = S
    cpu.state.reservation_address = 0x1234;
    run(
        &mut cpu,
        &[
            enc(op::SRET, 0, 0, 0),
            enc(op::ADDI, 0, 0, 1),
            enc(op::ADDI, 0, 0, 1),
            enc(op::HALT, 0, 0, 0),
        ],
    );
    assert_eq!(
        cpu.state.priv_level, 3,
        "a U-mode sret took SPP's privilege"
    );
    assert_eq!(cpu.state.regs[0], 2, "a U-mode sret jumped to SEPC");
    assert_eq!(
        cpu.state.reservation_address, 0x1234,
        "a refused sret is a NOP"
    );
}

#[test]
fn sret_at_supervisor_mode_still_returns() {
    let mut cpu = Cpu::new();
    cpu.state.priv_level = 1;
    cpu.ram[SEPC] = 4 * 2;
    cpu.ram[SSTATUS] = 0;
    run(
        &mut cpu,
        &[
            enc(op::SRET, 0, 0, 0),
            enc(op::ADDI, 0, 0, 1),
            enc(op::HALT, 0, 0, 0),
        ],
    );
    assert_eq!(cpu.state.priv_level, 3);
    assert_eq!(cpu.state.regs[0], 0);
}

// ── D6: a U-mode page-walk miss goes nowhere ────────────────────────────────

#[test]
fn mapped_user_load_reads_the_physical_page() {
    let mut cpu = user_cpu();
    cpu.ram[w(PA_MAPPED)] = 0xC0FF_EE00;
    cpu.state.regs[1] = VA_MAPPED;
    run(&mut cpu, &[enc(op::LD, 0, 1, 0), enc(op::HALT, 0, 0, 0)]);
    assert_eq!(cpu.state.regs[0], 0xC0FF_EE00);
}

#[test]
fn user_load_with_no_pde_reads_zero() {
    let mut cpu = user_cpu();
    cpu.ram[w(VA_NO_PDE)] = 0xBAD0_0001; // the identity fall-through target
    cpu.state.regs[1] = VA_NO_PDE;
    cpu.state.regs[0] = 7;
    run(&mut cpu, &[enc(op::LD, 0, 1, 0), enc(op::HALT, 0, 0, 0)]);
    assert_eq!(
        cpu.state.regs[0], 0,
        "a U-mode PDE miss read physical memory"
    );
}

#[test]
fn user_load_with_no_pte_reads_zero() {
    let mut cpu = user_cpu();
    cpu.ram[w(VA_NO_PTE)] = 0xBAD0_0002;
    cpu.state.regs[1] = VA_NO_PTE;
    cpu.state.regs[0] = 7;
    run(&mut cpu, &[enc(op::LDB, 0, 1, 0), enc(op::HALT, 0, 0, 0)]);
    assert_eq!(
        cpu.state.regs[0], 0,
        "a U-mode PTE miss read physical memory"
    );
}

#[test]
fn user_store_with_no_pde_is_dropped() {
    let mut cpu = user_cpu();
    cpu.state.regs[1] = VA_NO_PDE;
    cpu.state.regs[2] = 0xBAD0_0003;
    run(
        &mut cpu,
        &[
            enc(op::ST, 1, 2, 0),
            enc(op::STH, 1, 2, 8),
            enc(op::HALT, 0, 0, 0),
        ],
    );
    assert_eq!(
        cpu.ram[w(VA_NO_PDE)],
        0,
        "a U-mode PDE miss wrote physical memory"
    );
    assert_eq!(
        cpu.ram[w(VA_NO_PDE) + 2],
        0,
        "a U-mode PDE miss wrote physical memory"
    );
    assert!(
        !cpu.ram.contains(&0xBAD0_0003),
        "the dropped store landed somewhere"
    );
}

#[test]
fn kernel_miss_still_falls_through() {
    // D6 is U-mode only: S-mode keeps the identity fall-through.
    let mut cpu = user_cpu();
    cpu.state.priv_level = 1;
    cpu.ram[w(VA_NO_PDE)] = 0x0000_5EED;
    cpu.state.regs[1] = VA_NO_PDE;
    run(&mut cpu, &[enc(op::LD, 0, 1, 0), enc(op::HALT, 0, 0, 0)]);
    assert_eq!(cpu.state.regs[0], 0x0000_5EED);
}

#[test]
fn user_miss_without_page_directory_falls_through() {
    // d6_user_miss needs page_dir_base != 0, as in main.rs.
    let mut cpu = user_cpu();
    cpu.state.page_dir_base = 0;
    cpu.ram[w(VA_NO_PDE)] = 0x0000_5EED;
    cpu.state.regs[1] = VA_NO_PDE;
    run(&mut cpu, &[enc(op::LD, 0, 1, 0), enc(op::HALT, 0, 0, 0)]);
    assert_eq!(cpu.state.regs[0], 0x0000_5EED);
}

// ── Superpage leaf ──────────────────────────────────────────────────────────

#[test]
fn superpage_pde_maps_four_mib() {
    // hostile_reply's alias shape: PDE 2 = PRESENT | LEAF at physical 0.
    let mut cpu = user_cpu();
    cpu.ram[w(PGD) + 2] = mmu::PTE_PRESENT | mmu::PTE_LEAF;
    cpu.ram[w(0x0000_0160)] = 0x7AB1_E000;
    cpu.state.regs[1] = VA_NO_PDE + 0x160;
    run(&mut cpu, &[enc(op::LD, 0, 1, 0), enc(op::HALT, 0, 0, 0)]);
    assert_eq!(cpu.state.regs[0], 0x7AB1_E000, "superpage PDE not resolved");
}

// ── Atomics at U-mode go through the translation ────────────────────────────

#[test]
fn user_xchg_with_no_pde_touches_nothing() {
    let mut cpu = user_cpu();
    cpu.ram[w(VA_NO_PDE)] = 0x0BAD_0004;
    cpu.state.regs[1] = VA_NO_PDE;
    cpu.state.regs[0] = 0x1111_1111;
    run(&mut cpu, &[enc(op::XCHG, 0, 1, 0), enc(op::HALT, 0, 0, 0)]);
    assert_eq!(
        cpu.ram[w(VA_NO_PDE)],
        0x0BAD_0004,
        "XCHG wrote physical memory"
    );
    assert_eq!(cpu.state.regs[0], 0, "XCHG read physical memory");
}

#[test]
fn user_xchg_on_a_mapped_page_uses_the_physical_page() {
    let mut cpu = user_cpu();
    cpu.ram[w(PA_MAPPED)] = 0x2222_2222;
    cpu.state.regs[1] = VA_MAPPED;
    cpu.state.regs[0] = 0x1111_1111;
    run(&mut cpu, &[enc(op::XCHG, 0, 1, 0), enc(op::HALT, 0, 0, 0)]);
    assert_eq!(cpu.ram[w(PA_MAPPED)], 0x1111_1111);
    assert_eq!(cpu.state.regs[0], 0x2222_2222);
}

#[test]
fn user_cas_and_lr_sc_with_no_pde_touch_nothing() {
    let mut cpu = user_cpu();
    cpu.ram[w(VA_NO_PDE)] = 0x0BAD_0005;
    cpu.state.regs[1] = VA_NO_PDE;
    cpu.state.regs[0] = 0x0BAD_0005; // CAS expected: matches the physical word
    cpu.state.regs[2] = 0x3333_3333;
    cpu.state.regs[3] = VA_NO_PDE;
    cpu.state.regs[4] = 0x4444_4444;
    run(
        &mut cpu,
        &[
            enc(op::CAS, 0, 0, 0),
            enc(op::LR_W, 5, 3, 0),
            enc(op::SC_W, 6, 3, 4),
            enc(op::HALT, 0, 0, 0),
        ],
    );
    assert_eq!(
        cpu.ram[w(VA_NO_PDE)],
        0x0BAD_0005,
        "an atomic wrote physical memory"
    );
    assert_eq!(cpu.state.regs[0], 0, "CAS read physical memory");
    assert_eq!(cpu.state.regs[5], 0, "LR.W read physical memory");
}

#[test]
fn kernel_atomics_stay_physical() {
    let mut cpu = user_cpu();
    cpu.state.priv_level = 1;
    cpu.ram[w(VA_MAPPED)] = 0x2222_2222;
    cpu.state.regs[1] = VA_MAPPED;
    cpu.state.regs[0] = 0x1111_1111;
    run(&mut cpu, &[enc(op::XCHG, 0, 1, 0), enc(op::HALT, 0, 0, 0)]);
    assert_eq!(
        cpu.ram[w(VA_MAPPED)],
        0x1111_1111,
        "S-mode XCHG was translated"
    );
}
