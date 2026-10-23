// SPDX-License-Identifier: GPL-3.0-or-later
//! S5 Phase 1 (Steps 33-38): CSR scratch-register aliasing in the RV32I→MBC
//! translator — the S1.2 root cause.
//!
//! The translator's 0x73 arm used r12 (= RV a4) as the CSR address register,
//! r13 (= RV a5) as the read-modify-write temp, and r14 (= RV ra) as the
//! CSRRC mask. Every one of those is a live RISC-V register under
//! `map_register`. These tests translate real RV words, run them on the host
//! emulator (`ExecCpu`, which mirrors the BPF LD/ST semantics), and check the
//! architectural RISC-V result.
//!
//! CSR i lives at byte 0xF000 + i*4 (ADR-067 Decision 2):
//! sstatus 0x100 → 0xF400, sscratch 0x140 → 0xF500, satp 0x180 → 0xF600.

use monad_common::{mbc_opcodes as op, MbcInsn};
use monad_mbc::{ExecCpu, Translator};

const SSTATUS: u32 = 0xF400;
const SSCRATCH: u32 = 0xF500;
const SATP: u32 = 0xF600;

// MBC register numbers for the RV registers involved (translator map_register).
const R_RA: usize = 14; // x1
const R_TP: usize = 2; // x4
const R_A0: usize = 8; // x10
const R_A2: usize = 10; // x12
const R_A4: usize = 12; // x14
const R_A5: usize = 13; // x15

fn ram(cpu: &ExecCpu, byte_addr: u32) -> u32 {
    cpu.ram[(byte_addr >> 2) as usize]
}

fn set_ram(cpu: &mut ExecCpu, byte_addr: u32, v: u32) {
    cpu.ram[(byte_addr >> 2) as usize] = v;
}

/// Translate RV words, append HALT, run on the host emulator after `seed`.
fn run_rv(words: &[u32], seed: impl FnOnce(&mut ExecCpu)) -> ExecCpu {
    let mut mbc = Translator::translate_program(words).expect("translate");
    mbc.push(MbcInsn::encode(op::HALT, 0, 0, 0).0);
    let mut cpu = ExecCpu::new();
    cpu.load_rom(&mbc);
    seed(&mut cpu);
    cpu.run(1000).expect("run to HALT");
    cpu
}

/// Zicsr encoding: csr[31:20] rs1/uimm[19:15] funct3[14:12] rd[11:7] 0x73.
fn csr_insn(funct3: u32, rd: u32, rs1: u32, csr: u32) -> u32 {
    (csr << 20) | (rs1 << 15) | (funct3 << 12) | (rd << 7) | 0x73
}

// ── Step 33/34: H-S1 — the exact prepare_return sequence ────────────────────

#[test]
fn csr_alias_prepare_return_sscratch_gets_trapframe() {
    // lw a5,48(a0) ; csrr a2,satp ; csrw sscratch,a5
    let words = [0x0305_2783, 0x1800_2673, 0x1407_9073];
    let cpu = run_rv(&words, |c| {
        c.state.regs[R_A0] = 0x1000;
        set_ram(c, 0x1000 + 48, 0x007b_e000); // p->trapframe
        set_ram(c, SATP, 0x0000_07ff); // kernel satp (ppn of 0x7FF000)
    });
    assert_eq!(ram(&cpu, SSCRATCH), 0x007b_e000, "sscratch = p->trapframe");
    assert_eq!(
        cpu.state.regs[R_A5], 0x007b_e000,
        "a5 must survive csrr a2,satp"
    );
    assert_eq!(cpu.state.regs[R_A2], 0x7ff, "a2 = satp");
    assert_eq!(ram(&cpu, SATP), 0x7ff, "csrr must not change satp");
}

// ── Step 36/37: H-S2 — a4 is the CSR pointer ────────────────────────────────

#[test]
fn csr_alias_csrw_from_a4_stores_a4_value() {
    // mv a4,tp ; csrw sstatus,a4   (prepare_return 0x23088 / 0x230a0 shape)
    let words = [0x0002_0713, 0x1007_1073];
    let cpu = run_rv(&words, |c| {
        c.state.regs[R_TP] = 0x0000_0122;
    });
    assert_eq!(
        ram(&cpu, SSTATUS),
        0x122,
        "sstatus must receive a4's value, not the CSR address"
    );
    assert_eq!(cpu.state.regs[R_A4], 0x122, "a4 = tp");
}

#[test]
fn csr_alias_unrelated_csrr_preserves_a4_a5() {
    // csrr a2,satp with live a4/a5
    let cpu = run_rv(&[0x1800_2673], |c| {
        c.state.regs[R_A4] = 0xA4A4_A4A4;
        c.state.regs[R_A5] = 0xA5A5_A5A5;
        set_ram(c, SATP, 0x7ff);
    });
    assert_eq!(cpu.state.regs[R_A4], 0xA4A4_A4A4, "a4 clobbered by csrr");
    assert_eq!(cpu.state.regs[R_A5], 0xA5A5_A5A5, "a5 clobbered by csrr");
}

// ── Step 38/39: CSRRC mask in ra; CSRRW rd==rs1 ─────────────────────────────

#[test]
fn csr_alias_csrrc_preserves_ra() {
    // csrc sstatus, a2  (csrrc x0, sstatus, a2). Phase 2 rejects set/clear
    // with rs1!=x0 fail-closed (no image uses it, H-S3); if it ever
    // translates again, it must be correct and leave ra alone.
    let w = csr_insn(3, 0, 12, 0x100);
    if Translator::translate_program(&[w]).is_err() {
        return;
    }
    let cpu = run_rv(&[w], |c| {
        c.state.regs[R_RA] = 0x0002_3456;
        c.state.regs[R_A2] = 0x2;
        set_ram(c, SSTATUS, 0x22);
    });
    assert_eq!(ram(&cpu, SSTATUS), 0x20, "sstatus &= ~a2");
    assert_eq!(
        cpu.state.regs[R_RA], 0x0002_3456,
        "ra clobbered by CSRRC mask temp"
    );
}

#[test]
fn csr_alias_csrrw_rd_eq_rs1_swaps() {
    // csrrw a0, sscratch, a0 — RISC-V: a0 ← old sscratch, sscratch ← old a0.
    let w = csr_insn(1, 10, 10, 0x140);
    let r = Translator::translate_program(&[w]);
    if r.is_err() {
        return; // fail-closed rejection is an accepted Phase 2 outcome (Step 52)
    }
    let cpu = run_rv(&[w], |c| {
        c.state.regs[R_A0] = 0x1111_1111;
        set_ram(c, SSCRATCH, 0x2222_2222);
    });
    assert_eq!(cpu.state.regs[R_A0], 0x2222_2222, "a0 = old sscratch");
    assert_eq!(
        ram(&cpu, SSCRATCH),
        0x1111_1111,
        "sscratch = old a0 (source lost)"
    );
}

// ── Step 60: exhaustive form table ──────────────────────────────────────────

/// RV register → MBC register (mirror of `Translator::map_register` for the
/// registers the table uses).
fn mbc_of(x: u32) -> usize {
    match x {
        0 => 0,
        1 => 14,
        10 => 8,
        11 => 9,
        14 => 12,
        15 => 13,
        _ => unreachable!(),
    }
}

/// Every CSR form x rd x rs1 x {low, high} CSR: either the translator rejects
/// it, or the host run matches the RISC-V spec, leaves every other register
/// alone, and ends with r0 == 0. The two forms the kernel uses (csrr, csrw
/// with a register) must translate.
#[test]
fn csr_alias_exhaustive_form_table() {
    const OLD: u32 = 0x5A5A_0F0F;
    let mut checked = 0;
    for &csr in &[0x140u32, 0xF14] {
        let addr = 0xF000 + csr * 4;
        for &funct3 in &[1u32, 2, 3, 5, 6, 7] {
            for &rd in &[0u32, 14, 15, 1, 10] {
                for &rs1 in &[0u32, 14, 15, 11] {
                    let w = csr_insn(funct3, rd, rs1, csr);
                    let must_translate =
                        (funct3 == 2 && rs1 == 0) || (funct3 == 1 && rd == 0 && rs1 != 0);
                    let tr = Translator::translate_program(&[w]);
                    if tr.is_err() {
                        assert!(
                            !must_translate,
                            "form used by the kernel rejected: f3={funct3} rd=x{rd} rs1=x{rs1} csr={csr:#x}"
                        );
                        continue;
                    }
                    let mut seed = [0u32; 16];
                    for (i, v) in seed.iter_mut().enumerate().skip(1) {
                        *v = 0x1000_0000 + (i as u32) * 0x0101_0101;
                    }
                    let cpu = run_rv(&[w], |c| {
                        c.state.regs[..16].copy_from_slice(&seed);
                        set_ram(c, addr, OLD);
                    });
                    let src = if funct3 >= 5 { rs1 } else { seed[mbc_of(rs1)] };
                    let new = match funct3 {
                        1 | 5 => src,
                        2 | 6 => OLD | src,
                        _ => OLD & !src,
                    };
                    let ctx = format!("f3={funct3} rd=x{rd} rs1=x{rs1} csr={csr:#x}");
                    assert_eq!(ram(&cpu, addr), new, "CSR value, {ctx}");
                    assert_eq!(cpu.state.regs[0], 0, "r0 must be 0 after, {ctx}");
                    for i in 1..16 {
                        let want = if rd != 0 && i == mbc_of(rd) {
                            OLD
                        } else {
                            seed[i]
                        };
                        assert_eq!(cpu.state.regs[i], want, "r{i}, {ctx}");
                    }
                    checked += 1;
                }
            }
        }
    }
    assert!(checked > 0);
}
