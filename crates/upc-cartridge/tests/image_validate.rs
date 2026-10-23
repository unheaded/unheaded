// SPDX-License-Identifier: GPL-3.0-or-later
//! .cart format + validator tests (S5 Step 259). Written before the code.

use monad_common::cart::{STAGE_TICK, STAGE_XDP};
use monad_common::{mbc_opcodes as op, MbcInsn};
use upc_cartridge::{validate, Image, ImageError, ValidateError, HEADER_LEN};

fn enc(o: u8, d: u8, s: u8, imm: u16) -> u32 {
    MbcInsn::encode(o, d, s, imm).0
}
fn br(o: u8, off: i32) -> u32 {
    ((o as u32) << 24) | ((off as u32) & 0x00FF_FFFF)
}
fn halt() -> u32 {
    enc(op::HALT, 0, 0, 0)
}

fn xdp(rom: Vec<u32>) -> Image {
    Image {
        stage: STAGE_XDP,
        max_insn: 64,
        entry: 0,
        loop_bound: 0,
        flags: 0,
        rom,
        data: vec![],
    }
}

#[test]
fn roundtrip_and_layout() {
    let img = Image {
        stage: STAGE_TICK,
        max_insn: 200,
        entry: 1,
        loop_bound: 16,
        flags: 1,
        rom: vec![enc(op::NOP, 0, 0, 0), halt()],
        data: vec![0xDEAD_BEEF, 7, 9],
    };
    let b = img.encode();
    assert_eq!(b.len(), HEADER_LEN + 4 * 5);
    assert_eq!(HEADER_LEN, 52);
    assert_eq!(&b[0..4], b"MBCC");
    assert_eq!(b[4], 0);
    assert_eq!(b[5], STAGE_TICK);
    assert_eq!(u16::from_le_bytes([b[6], b[7]]), 200);
    assert_eq!(u16::from_le_bytes([b[8], b[9]]), 2);
    assert_eq!(u16::from_le_bytes([b[10], b[11]]), 3);
    assert_eq!(u16::from_le_bytes([b[12], b[13]]), 1);
    assert_eq!(u16::from_le_bytes([b[14], b[15]]), 16);
    assert_eq!(u16::from_le_bytes([b[16], b[17]]), 1);
    assert_eq!(&b[52..56], &enc(op::NOP, 0, 0, 0).to_le_bytes());
    assert_eq!(&b[60..64], &0xDEAD_BEEFu32.to_le_bytes());
    assert_eq!(Image::decode(&b).unwrap(), img);
}

#[test]
fn decode_rejects_hostile_bytes() {
    let good = xdp(vec![halt()]).encode();
    let mut cases: Vec<(&str, Vec<u8>, ImageError)> = vec![];
    cases.push(("empty", vec![], ImageError::Truncated));
    cases.push(("short header", good[..51].to_vec(), ImageError::Truncated));
    let mut b = good.clone();
    b[0] = b'X';
    cases.push(("magic", b, ImageError::BadMagic));
    let mut b = good.clone();
    b[4] = 1;
    cases.push(("version", b, ImageError::BadVersion(1)));
    let mut b = good.clone();
    b[5] = 2;
    cases.push(("stage", b, ImageError::BadStage(2)));
    let mut b = good.clone();
    b.push(0);
    cases.push(("trailing byte", b, ImageError::Length { want: 56, got: 57 }));
    let mut b = good.clone();
    b.truncate(55);
    cases.push(("truncated rom", b, ImageError::Length { want: 56, got: 55 }));
    let mut b = good.clone();
    b[8] = 0;
    cases.push(("zero rom", b, ImageError::RomSize(0)));
    let mut b = good.clone();
    b[18] = 1;
    cases.push(("reserved", b, ImageError::Reserved));
    let mut b = good.clone();
    b[16] = 2;
    cases.push(("unknown flag", b, ImageError::Reserved));
    let mut b = good.clone();
    b[12] = 1;
    cases.push(("entry past rom", b, ImageError::Entry(1)));
    let mut b = good.clone();
    b[52] ^= 1; // ROM byte changed, sha not updated
    cases.push(("sha", b, ImageError::Sha));
    for (name, bytes, want) in cases {
        assert_eq!(Image::decode(&bytes), Err(want), "{name}");
    }
    // Size limits are checked from the header before trusting the length.
    let mut b = good.clone();
    b[8..10].copy_from_slice(&1025u16.to_le_bytes());
    assert_eq!(Image::decode(&b), Err(ImageError::RomSize(1025)));
    let mut b = good;
    b[10..12].copy_from_slice(&1025u16.to_le_bytes());
    assert_eq!(Image::decode(&b), Err(ImageError::DataSize(1025)));
}

#[test]
fn decode_never_panics_on_prefixes_and_flips() {
    let img = Image {
        stage: STAGE_XDP,
        max_insn: 8,
        entry: 0,
        loop_bound: 0,
        flags: 0,
        rom: vec![halt(); 3],
        data: vec![1, 2],
    };
    let b = img.encode();
    for n in 0..b.len() {
        let _ = Image::decode(&b[..n]);
    }
    for i in 0..b.len() {
        for bit in 0..8 {
            let mut c = b.clone();
            c[i] ^= 1 << bit;
            assert!(
                Image::decode(&c).is_err(),
                "flip byte {i} bit {bit} accepted"
            );
        }
    }
}

#[test]
fn validate_accepts_a_forward_xdp_cartridge() {
    // MOVI r1,3 ; SYSCALL FIELD ; CMP r0,r2 ; JZ +2 ; MOVI r1,0 ; SYSCALL VERDICT ; HALT
    let img = xdp(vec![
        enc(op::MOVI, 1, 0, 3),
        enc(op::SYSCALL, 0, 0, 0),
        enc(op::CMP, 0, 2, 0),
        br(op::JZ, 2),
        enc(op::MOVI, 1, 0, 0),
        enc(op::SYSCALL, 0, 0, 2),
        halt(),
    ]);
    let r = validate(&img).expect("valid");
    assert_eq!(r.max_insn, 64);
    assert!(!r.has_back_edges);
    assert_eq!(r.syscalls, vec![0, 2]);
}

#[test]
fn validate_rejects() {
    let v = |img: Image| validate(&img).unwrap_err();
    assert_eq!(
        v(xdp(vec![enc(op::DIV, 1, 2, 0), halt()])),
        ValidateError::Opcode {
            pc: 0,
            opcode: op::DIV
        }
    );
    assert_eq!(
        v(xdp(vec![enc(op::CALL, 0, 0, 0)])),
        ValidateError::Opcode {
            pc: 0,
            opcode: op::CALL
        }
    );
    assert_eq!(
        v(xdp(vec![br(op::JMP, 5), halt()])),
        ValidateError::BranchTarget { pc: 0, target: 6 }
    );
    assert_eq!(
        v(xdp(vec![halt(), br(op::JMP, -3)])),
        ValidateError::BranchTarget { pc: 1, target: -1 }
    );
    assert_eq!(
        v(xdp(vec![halt(), br(op::JNZ, -2)])),
        ValidateError::BackEdge { pc: 1, target: 0 }
    );
    assert_eq!(
        v(xdp(vec![br(op::JMP, -1)])),
        ValidateError::BackEdge { pc: 0, target: 0 }
    );
    assert_eq!(
        v(xdp(vec![enc(op::SYSCALL, 0, 0, 5), halt()])),
        ValidateError::Syscall { pc: 0, number: 5 }
    );
    assert_eq!(
        v(xdp(vec![enc(op::SYSCALL, 0, 0, 99), halt()])),
        ValidateError::Syscall { pc: 0, number: 99 }
    );
    let mut img = xdp(vec![halt()]);
    img.max_insn = 65;
    assert_eq!(
        v(img),
        ValidateError::MaxInsn {
            stage: STAGE_XDP,
            max_insn: 65
        }
    );
    let mut img = xdp(vec![halt()]);
    img.max_insn = 0;
    assert_eq!(
        v(img),
        ValidateError::MaxInsn {
            stage: STAGE_XDP,
            max_insn: 0
        }
    );
    let mut img = xdp(vec![halt()]);
    img.loop_bound = 1;
    assert_eq!(v(img), ValidateError::LoopBound);
    // Tick stage: back-edges fine, FIELD/VERDICT/QUEUE are not.
    let tick = |rom: Vec<u32>| Image {
        stage: STAGE_TICK,
        max_insn: 256,
        entry: 0,
        loop_bound: 8,
        flags: 0,
        rom,
        data: vec![],
    };
    assert!(
        validate(&tick(vec![
            enc(op::ADDI, 1, 0, 0xFFFF),
            br(op::JNZ, -2),
            halt()
        ]))
        .unwrap()
        .has_back_edges
    );
    for n in [0u16, 2, 4] {
        assert_eq!(
            v(tick(vec![enc(op::SYSCALL, 0, 0, n), halt()])),
            ValidateError::Syscall { pc: 0, number: n }
        );
    }
    let mut img = tick(vec![halt()]);
    img.max_insn = 257;
    assert_eq!(
        v(img),
        ValidateError::MaxInsn {
            stage: STAGE_TICK,
            max_insn: 257
        }
    );
}
