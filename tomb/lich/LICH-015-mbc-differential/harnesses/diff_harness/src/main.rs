// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.
//
// LICH-015 differential harness (S-LICH-AWAKENS Phase 7, ADR-062/067/080).
//
// Oracle: run a bounded MBC program on two implementations from the SAME
// initial register state and compare, in lockstep, the registers, the three
// condition flags (N/Z/C), and the program counter after every instruction.
// The first instruction whose two outcomes differ is the divergence.
//
//   arm A  reference interpreter : monad_mbc::ExecCpu          (execute.rs)
//   arm B  shipping-engine model : monad_common::cart::step    (= monad-cpu-ebpf)
//
// arm B copies the monad-cpu-ebpf ALU/flag semantics verbatim and is proven
// `BPF == reference` on a live kernel by pkg/cartridge (S5 Phase 11: 3/3 flow
// hashes, 21/21 events through BPF_PROG_TEST_RUN). A divergence here is
// therefore a reference-interpreter drift, not a shipping-engine bug — unless
// the reference is the correct one and the engine drifted, which the triage
// step decides.
//
// Scope note: arm B is the ADR-108 cartridge opcode allowlist (ALU, shifts,
// branches, MUL/MULH, CMP, MOV/MOVI/LOAD_IMM32/ADDI). Opcodes OUTSIDE that
// allowlist (MULHU, the ISA-v2 privileged ops MRET/SRET/LR.W/SC.W/FENCE) cannot
// be driven through arm B in-process — cart::step faults on them by design — so
// those are handled by `--mulhu-probe` (static, reference-side) and are deferred
// to the kernel-attached east arm per ADR-062 LICH-005's lesson.
//
// Subcommands:
//   diff_harness run      [--iters N] [--seed HEX] [--out DIR]
//   diff_harness self-test                       (injected-divergence proof)
//   diff_harness mulhu-probe                      (out-of-allowlist gap report)
//   diff_harness corpus   <file...>               (replay seed programs)

use std::io::Write;

use monad_common::cart::{begin_run, step, CartCfg, CartIo, CartState, Step, STAGE_TICK};
use monad_common::{mbc_opcodes as op, MbcInsn};
use monad_mbc::ExecCpu;

/// No I/O for the differential: pure ALU/branch programs, no fields, no emit.
struct NoIo;
impl CartIo for NoIo {
    fn field(&mut self, _: u32) -> Option<u32> {
        None
    }
    fn now_ns_lo(&mut self) -> u32 {
        0
    }
    fn emit(&mut self, _: u32, _: u32, _: u32) {}
    fn queue(&mut self, _: &[u32; 8]) {}
}

/// xorshift64* — deterministic, matches crates/upc-cartridge/tests/differential.rs.
struct Rng(u64);
impl Rng {
    fn next(&mut self) -> u64 {
        let mut x = self.0;
        x ^= x >> 12;
        x ^= x << 25;
        x ^= x >> 27;
        self.0 = x;
        x.wrapping_mul(0x2545_F491_4F6C_DD1D)
    }
    fn below(&mut self, n: u64) -> u64 {
        self.next() % n
    }
}

/// The ADR-108 cartridge allowlist ALU/data opcodes arm B can execute.
const ALU: &[u8] = &[
    op::ADD,
    op::SUB,
    op::MUL,
    op::AND,
    op::OR,
    op::XOR,
    op::NOT,
    op::SHL,
    op::SHR,
    op::SAR,
    op::MOV,
    op::MOVI,
    op::CMP,
    op::LOAD_IMM32,
    op::ADDI,
    op::SHLR,
    op::SHRR,
    op::SARR,
    op::MULH,
    op::NOP,
];
const BR: &[u8] = &[op::JMP, op::JZ, op::JNZ, op::JN, op::JP, op::JC, op::JNC];

fn random_program(rng: &mut Rng) -> Vec<u32> {
    let n = 1 + rng.below(30) as usize;
    let mut rom = Vec::with_capacity(n + 1);
    for i in 0..n {
        if rng.below(5) == 0 {
            let o = BR[rng.below(BR.len() as u64) as usize];
            let target = i + 1 + rng.below((n - i) as u64) as usize;
            let off = (target - (i + 1)) as u32;
            rom.push(((o as u32) << 24) | off);
        } else {
            let o = ALU[rng.below(ALU.len() as u64) as usize];
            let imm = match rng.below(4) {
                0 => rng.below(40) as u16,
                1 => 0xFFFF,
                _ => rng.next() as u16,
            };
            rom.push(MbcInsn::encode(o, rng.below(16) as u8, rng.below(16) as u8, imm).0);
        }
    }
    rom.push(MbcInsn::encode(op::HALT, 0, 0, 0).0);
    rom
}

fn interesting(rng: &mut Rng) -> u32 {
    match rng.below(6) {
        0 => 0,
        1 => 0xFFFF_FFFF,
        2 => 0x8000_0000,
        3 => rng.below(64) as u32,
        _ => rng.next() as u32,
    }
}

/// One divergence record.
struct Divergence {
    opcode: u8,
    word: u32,
    seed: u64,
    detail: String,
}

/// Run one program through both arms in lockstep. `corrupt` injects a one-bit
/// fault into arm A's register file after the first executed instruction — the
/// self-test hook that proves the comparator actually catches a difference.
fn run_one(rom: &[u32], regs: [u32; 16], seed: u64, corrupt: bool) -> Option<Divergence> {
    let cfg = CartCfg {
        stage: STAGE_TICK,
        max_insn: 256,
        rom_words: rom.len() as u16,
        data_words: 0,
        entry: 0,
        loop_bound: 0,
    };
    let mut st = CartState::zeroed();
    begin_run(&mut st, 0);
    st.regs = regs;
    let mut host = ExecCpu::new();
    host.load_rom(rom);
    host.state.regs = regs;
    host.state.flags = 0;

    for tick in 0..64usize {
        let pc = st.pc as usize;
        if pc >= rom.len() {
            break;
        }
        let w = rom[pc];
        let r = step(&mut st, &cfg, rom, &[][..], &mut NoIo);
        let h = host.step();
        if corrupt && tick == 0 {
            // Inject a single-bit fault into the reference arm so a known-equal
            // program MUST now report divergent. Proves the oracle is live.
            host.state.regs[0] ^= 1;
        }
        let hflags = host.state.flags & 0x07;
        if st.regs != host.state.regs || st.flags != hflags || st.pc as u32 != host.state.pc {
            let o = MbcInsn(w).opcode();
            let d = MbcInsn(w).dst() as usize;
            return Some(Divergence {
                opcode: o,
                word: w,
                seed,
                detail: format!(
                    "engine regs[{d}]={:#x} flags={:#x} pc={} | reference regs[{d}]={:#x} flags={:#x} pc={}",
                    st.regs[d], st.flags, st.pc, host.state.regs[d], hflags, host.state.pc
                ),
            });
        }
        match r {
            Step::Continue => {
                if h.is_err() {
                    // Arm B continued; arm A errored. Report as a divergence.
                    return Some(Divergence {
                        opcode: MbcInsn(w).opcode(),
                        word: w,
                        seed,
                        detail: "reference arm errored where engine continued".into(),
                    });
                }
            }
            Step::Halt => break,
            other => {
                return Some(Divergence {
                    opcode: MbcInsn(w).opcode(),
                    word: w,
                    seed,
                    detail: format!("engine returned {other:?}"),
                });
            }
        }
    }
    None
}

fn cmd_run(iters: u64, seed0: u64, out: Option<&str>) -> i32 {
    let mut uniq: std::collections::BTreeMap<u8, Divergence> = Default::default();
    let mut total = 0u64;
    for i in 0..iters {
        let seed = seed0.wrapping_add(i).wrapping_mul(0x9E37_79B9_7F4A_7C15) | 1;
        let mut rng = Rng(seed);
        let rom = random_program(&mut rng);
        let regs: [u32; 16] = core::array::from_fn(|_| interesting(&mut rng));
        total += 1;
        if let Some(div) = run_one(&rom, regs, seed, false) {
            uniq.entry(div.opcode).or_insert(div);
        }
    }
    report(total, &uniq, out, seed0);
    if uniq.is_empty() {
        0
    } else {
        2
    }
}

fn report(
    total: u64,
    uniq: &std::collections::BTreeMap<u8, Divergence>,
    out: Option<&str>,
    seed0: u64,
) {
    println!(
        "LICH-015 differential: {total} programs, {} unique divergence(s) (cart-allowlist opcode set)",
        uniq.len()
    );
    for (o, d) in uniq {
        println!(
            "  opcode {o:#04x} word {:#010x} seed {:#x}: {}",
            d.word, d.seed, d.detail
        );
    }
    if let Some(dir) = out {
        if let Err(e) = write_results(dir, total, uniq, seed0) {
            eprintln!("warning: could not write results to {dir}: {e}");
        } else {
            println!("results written under {dir}");
        }
    }
}

fn write_results(
    dir: &str,
    total: u64,
    uniq: &std::collections::BTreeMap<u8, Divergence>,
    seed0: u64,
) -> std::io::Result<()> {
    std::fs::create_dir_all(dir)?;
    // divergences.json — hand-rolled (no serde dep; keep the tree dep-clean).
    let mut j = std::fs::File::create(format!("{dir}/divergences.json"))?;
    write!(j, "{{\"campaign\":\"LICH-015\",\"programs\":{total},\"seed0\":\"{seed0:#x}\",\"divergences\":[")?;
    for (i, (o, d)) in uniq.iter().enumerate() {
        if i > 0 {
            write!(j, ",")?;
        }
        write!(
            j,
            "{{\"opcode\":\"{o:#04x}\",\"word\":\"{:#010x}\",\"seed\":\"{:#x}\",\"detail\":{:?}}}",
            d.word, d.seed, d.detail
        )?;
    }
    writeln!(j, "]}}")?;
    // summary.md
    let mut s = std::fs::File::create(format!("{dir}/summary.md"))?;
    writeln!(s, "# LICH-015 differential run\n")?;
    writeln!(s, "- programs: {total}")?;
    writeln!(s, "- seed0: {seed0:#x}")?;
    writeln!(
        s,
        "- unique divergences (cart-allowlist opcode set): {}",
        uniq.len()
    )?;
    writeln!(s, "- arm A: monad_mbc::ExecCpu (reference interpreter)")?;
    writeln!(s, "- arm B: monad_common::cart::step (= monad-cpu-ebpf; BPF==reference on live kernel, S5 Phase 11)\n")?;
    if uniq.is_empty() {
        writeln!(
            s,
            "Verdict: **CLEAN** — no divergence over the cart-allowlist opcode set."
        )?;
    } else {
        writeln!(
            s,
            "Verdict: **DIVERGENT** — triage required (see divergences.json)."
        )?;
    }
    writeln!(
        s,
        "\nOut-of-allowlist opcodes (MULHU, MRET/SRET/LR.W/SC.W/FENCE) are not driven \
         through arm B in-process; see `diff_harness mulhu-probe` and the east \
         kernel-attached arm (ADR-062 LICH-005)."
    )?;
    Ok(())
}

/// Step 176 self-test: an injected one-bit fault on a known-equal program MUST
/// be reported divergent, and the same program WITHOUT the fault MUST be clean.
/// Proves the oracle is not vacuously passing.
fn cmd_self_test() -> i32 {
    let seed = 0xC3C3_5EEDu64;
    let mut rng = Rng(seed);
    let rom = random_program(&mut rng);
    let regs: [u32; 16] = core::array::from_fn(|_| interesting(&mut rng));

    let clean = run_one(&rom, regs, seed, false);
    let faulted = run_one(&rom, regs, seed, true);

    match (clean, faulted) {
        (None, Some(_)) => {
            println!(
                "self-test PASS: equal program clean; injected 1-bit fault detected divergent"
            );
            0
        }
        (Some(d), _) => {
            eprintln!(
                "self-test FAIL: baseline program already divergent: {}",
                d.detail
            );
            1
        }
        (None, None) => {
            eprintln!("self-test FAIL: injected 1-bit fault NOT detected (oracle is blind)");
            1
        }
    }
}

/// MULHU (0x3A) is in ADR-067 and the translator emits it (for __divdi3 →
/// FixedDiv), and monad-cpu-ebpf implements it (main.rs: unsigned high word).
/// The reference interpreter monad_mbc::ExecCpu has NO MULHU arm — its default
/// opcode arm treats it as a NOP ("fail-open for forward compat"). This probe
/// documents that reference-interpreter GAP without driving MULHU through arm B
/// (cart::step faults on MULHU by ADR-108 allowlist design).
fn cmd_mulhu_probe() -> i32 {
    // a * b where the product's high word is non-zero.
    let a: u32 = 0x1234_5678;
    let b: u32 = 0x9ABC_DEF0;
    let expected_hi = (((a as u64) * (b as u64)) >> 32) as u32; // monad-cpu-ebpf MULHU

    let rom = [
        MbcInsn::encode(op::MULHU, 0, 1, 0).0,
        MbcInsn::encode(op::HALT, 0, 0, 0).0,
    ];
    let mut host = ExecCpu::new();
    host.load_rom(&rom);
    host.state.regs[0] = a;
    host.state.regs[1] = b;
    let r0_before = host.state.regs[0];
    let _ = host.step(); // execute MULHU on the reference interpreter
    let r0_after = host.state.regs[0];

    println!("LICH-015 MULHU out-of-allowlist probe (ADR-067 opcode 0x3A):");
    println!("  inputs  a={a:#010x} b={b:#010x}");
    println!("  monad-cpu-ebpf (shipping engine) MULHU -> r0 = {expected_hi:#010x}");
    println!(
        "  monad_mbc::ExecCpu (reference)   MULHU -> r0 = {r0_after:#010x} (was {r0_before:#010x})"
    );
    if r0_after == r0_before && expected_hi != r0_before {
        println!(
            "  VERDICT: DIVERGENT — reference interpreter treats MULHU as NOP (missing arm). \
             This is an EMULATOR GAP (reference incomplete), not a shipping-engine bug: \
             monad-cpu-ebpf is correct per ADR-067. Record, do not file against shipping code."
        );
        // exit 0: the gap is expected/known and documented, not a test failure.
        0
    } else if r0_after == expected_hi {
        println!("  VERDICT: CONVERGED — reference interpreter now implements MULHU correctly.");
        0
    } else {
        println!(
            "  VERDICT: UNEXPECTED — reference produced a non-NOP, non-correct value; triage."
        );
        2
    }
}

fn cmd_corpus(files: &[String]) -> i32 {
    let mut uniq: std::collections::BTreeMap<u8, Divergence> = Default::default();
    let mut total = 0u64;
    for f in files {
        let text = match std::fs::read_to_string(f) {
            Ok(t) => t,
            Err(e) => {
                eprintln!("skip {f}: {e}");
                continue;
            }
        };
        let mut rom: Vec<u32> = Vec::new();
        for tok in text.split_whitespace() {
            let tok = tok.trim_start_matches("0x");
            if let Ok(w) = u32::from_str_radix(tok, 16) {
                rom.push(w);
            }
        }
        if rom.is_empty() {
            continue;
        }
        // Ensure a HALT terminates the program.
        rom.push(MbcInsn::encode(op::HALT, 0, 0, 0).0);
        let regs = [0u32; 16];
        total += 1;
        if let Some(div) = run_one(&rom, regs, 0, false) {
            uniq.entry(div.opcode).or_insert(div);
        }
    }
    report(total, &uniq, None, 0);
    if uniq.is_empty() {
        0
    } else {
        2
    }
}

fn main() {
    let args: Vec<String> = std::env::args().collect();
    let cmd = args.get(1).map(String::as_str).unwrap_or("run");
    let code = match cmd {
        "run" => {
            let mut iters = 10_000u64;
            let mut seed = 0xC3C3_5EEDu64;
            let mut out: Option<String> = None;
            let mut i = 2;
            while i < args.len() {
                match args[i].as_str() {
                    "--iters" => {
                        i += 1;
                        iters = args.get(i).and_then(|s| s.parse().ok()).unwrap_or(iters);
                    }
                    "--seed" => {
                        i += 1;
                        seed = args
                            .get(i)
                            .map(|s| s.trim_start_matches("0x"))
                            .and_then(|s| u64::from_str_radix(s, 16).ok())
                            .unwrap_or(seed);
                    }
                    "--out" => {
                        i += 1;
                        out = args.get(i).cloned();
                    }
                    other => eprintln!("ignoring unknown arg {other}"),
                }
                i += 1;
            }
            cmd_run(iters, seed, out.as_deref())
        }
        "self-test" => cmd_self_test(),
        "mulhu-probe" => cmd_mulhu_probe(),
        "corpus" => cmd_corpus(&args[2..]),
        other => {
            eprintln!("unknown subcommand {other}; use run|self-test|mulhu-probe|corpus");
            64
        }
    };
    std::process::exit(code);
}
