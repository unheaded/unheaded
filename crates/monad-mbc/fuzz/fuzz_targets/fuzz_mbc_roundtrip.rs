#![no_main]

use libfuzzer_sys::fuzz_target;
use monad_common::{MbcInsn, mbc_opcodes as op};
use monad_mbc::instruction::decode_checked;

/// All valid MBC opcodes (from monad_common::mbc_opcodes).
const VALID_OPCODES: &[u8] = &[
    op::ADD, op::SUB, op::MUL, op::DIV, op::MOD, op::NEG,
    op::AND, op::OR, op::XOR, op::NOT, op::SHL, op::SHR, op::SAR,
    op::MOV, op::MOVI, op::CMP,
    op::JMP, op::JZ, op::JNZ, op::JN, op::JP, op::JC, op::JNC,
    op::CALL, op::RET, op::JMPR, op::CALLR,
    op::LD, op::ST, op::LDB, op::STB, op::LDH, op::STH,
    op::SHLR, op::SHRR, op::SARR, op::MULH,
    op::SYSCALL, op::HALT,
];

fuzz_target!(|data: &[u8]| {
    // We need at least 4 bytes: 1 for opcode index, 1 for dst, 1 for src, 2 for imm16
    if data.len() < 5 {
        return;
    }

    // Select a valid opcode from the fuzzer byte
    let opcode = VALID_OPCODES[data[0] as usize % VALID_OPCODES.len()];
    let dst = data[1] & 0x0F;
    let src = data[2] & 0x0F;
    let imm = u16::from_le_bytes([data[3], data[4]]);

    // Encode
    let insn = MbcInsn::encode(opcode, dst, src, imm);

    // Decode -- must succeed for valid opcodes
    let decoded = decode_checked(insn.0)
        .expect("decode_checked must succeed for a valid opcode");

    // Round-trip assertions
    assert_eq!(decoded.opcode(), opcode, "opcode mismatch");
    assert_eq!(decoded.dst(), dst, "dst mismatch");
    assert_eq!(decoded.src(), src, "src mismatch");
    assert_eq!(decoded.imm16(), imm, "imm16 mismatch");
});
