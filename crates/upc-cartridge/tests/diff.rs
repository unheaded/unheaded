// SPDX-License-Identifier: GPL-3.0-or-later
//! S5 Phase 17 (Steps 420, 424): the cartridge differential oracle the Lich
//! sprint inherits. Random validated XDP-stage cartridges (ALU, forward
//! branches, loads/stores into both memory windows, FIELD / NOW / EMIT /
//! VERDICT syscalls, rare out-of-window accesses) run in lockstep on the
//! shared step (`monad_common::cart`, the code the BPF object runs) and on the
//! host MBC emulator (`monad-mbc` execute.rs). Registers, flags, PC and the
//! flow window are compared after every instruction.

use upc_cartridge::diff::{
    bpf_case_input, gen_input, gen_xdp_cartridge, lockstep, lockstep_with, End, Entropy, Options,
    XorShift,
};
use upc_cartridge::reference::run_packet;
use upc_cartridge::{state_hash, validate, CartState, Image};

#[test]
fn generated_cartridges_always_validate_and_roundtrip() {
    let mut e = XorShift::new(0x5EED_0017);
    for _ in 0..2_000 {
        let img = gen_xdp_cartridge(&mut e);
        validate(&img).expect("generator emitted an image the validator rejects");
        assert_eq!(Image::decode(&img.encode()).unwrap(), img);
        assert!(img.rom.len() <= 48, "rom {} words", img.rom.len());
    }
}

#[test]
fn lockstep_campaign_has_no_divergence() {
    let mut e = XorShift::new(0xC3C3_5EED);
    let (mut halts, mut verdicts, mut faults, mut mem_ops, mut syscalls) = (0, 0, 0, 0, 0);
    for i in 0..10_000 {
        let img = gen_xdp_cartridge(&mut e);
        let input = gen_input(&mut e);
        let a = lockstep(&img, &input)
            .unwrap_or_else(|d| panic!("case {i}: {d}\nimage {}", hex(&img.encode())));
        match a.end {
            End::Halt => halts += 1,
            End::Verdict(_) => verdicts += 1,
            End::Fault(_) => faults += 1,
        }
        mem_ops += a.mem_ops;
        syscalls += a.syscalls;
    }
    eprintln!("lockstep: 10000 cartridges, halt {halts} verdict {verdicts} fault {faults}, {mem_ops} memory ops, {syscalls} syscalls, 0 divergences");
    // The generator really reaches every ending and both memory windows.
    assert!(
        halts > 1_000 && verdicts > 500 && faults > 200,
        "halt {halts} verdict {verdicts} fault {faults}"
    );
    assert!(
        mem_ops > 10_000 && syscalls > 5_000,
        "mem ops {mem_ops} syscalls {syscalls}"
    );
}

#[test]
fn lockstep_driver_matches_the_reference_runner() {
    // The lockstep loop drives `run_one` itself; it must end exactly where
    // `reference::run_packet` (the `cart run` path) ends.
    let mut e = XorShift::new(0x0BAD_CAFE);
    for _ in 0..2_000 {
        let img = gen_xdp_cartridge(&mut e);
        let input = gen_input(&mut e);
        let a = lockstep(&img, &input).unwrap();
        let mut st = CartState::zeroed();
        let out = run_packet(&img, &mut st, &input);
        assert_eq!(state_hash(&st), state_hash(&a.state));
        assert_eq!(out.emits, a.emits);
        assert_eq!(out.step.map(End::from_step), Some(Some(a.end)));
    }
}

#[test]
fn oracle_self_test_catches_an_injected_bit_flip() {
    // Not vacuous: a one-bit fault in the host arm after the first
    // instruction must be reported, at that instruction.
    let mut e = XorShift::new(0x1234_5678);
    let mut caught = 0;
    for _ in 0..200 {
        let img = gen_xdp_cartridge(&mut e);
        let input = gen_input(&mut e);
        let clean = lockstep(&img, &input).unwrap();
        if clean.steps < 2 {
            continue;
        }
        let d = lockstep_with(
            &img,
            &input,
            Options {
                flip_host_bit_after: Some(0),
            },
        )
        .expect_err("injected divergence not detected");
        assert_eq!(d.step, 0, "{d}");
        caught += 1;
    }
    assert!(caught > 100);
}

#[test]
fn bpf_case_input_matches_the_test_packet() {
    // pkg/cartridge sends IPv4Packet(17, 10.0.0.1, 10.0.0.2, sport, 53, 80)
    // as the first packet of a fresh flow at a fixed clock.
    let inp = bpf_case_input(12_345, 7_000_000_000);
    assert_eq!(
        inp.fields,
        [4, 17, 12_345, 53, 80, 64, 0, 0, 1, 80, 0, 0, 0]
    );
    assert_eq!(inp.now_ns, 7_000_000_000);
}

#[test]
fn entropy_from_bytes_terminates_on_exhaustion() {
    // The fuzz target feeds raw bytes; an empty stream reads as zeros and the
    // generator still ends with a valid image.
    let mut e = upc_cartridge::diff::Bytes::new(&[]);
    let img = gen_xdp_cartridge(&mut e);
    validate(&img).unwrap();
    assert_eq!(e.next_u64(), 0);
}

fn hex(b: &[u8]) -> String {
    b.iter().map(|x| format!("{x:02x}")).collect()
}
