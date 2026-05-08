//! MBC instruction decoder/encoder for the Doom-over-IPv6 CPU.
//!
//! This module provides validated instruction encoding/decoding with comprehensive
//! error handling and testing capabilities.

use monad_common::{mbc_opcodes as op, MbcInsn};
use std::fmt;

/// Error types for instruction decoding operations.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum DecodeError {
    /// The opcode byte is not recognized as a valid MBC opcode.
    InvalidOpcode(u8),
}

impl fmt::Display for DecodeError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            DecodeError::InvalidOpcode(opcode) => {
                write!(f, "invalid opcode: 0x{:02X}", opcode)
            }
        }
    }
}

impl std::error::Error for DecodeError {}

/// Checks if the given byte is a valid MBC opcode.
///
/// This function validates against all known opcodes in monad_common::mbc_opcodes.
#[inline]
pub fn is_valid_opcode(opcode: u8) -> bool {
    matches!(
        opcode,
        // No-op
        op::NOP |
        // Arithmetic operations
        op::ADD | op::SUB | op::MUL | op::DIV | op::MOD | op::NEG |
        // Bitwise operations
        op::AND | op::OR | op::XOR | op::NOT | op::SHL | op::SHR | op::SAR |
        // Stack operations
        op::PUSH | op::POP |
        // Extended immediate
        op::LOAD_IMM32 | op::ADDI |
        // Register bitwise shifts
        op::SHLR | op::SHRR | op::SARR |
        // Multiplication variants
        op::MULH |
        // Data movement
        op::MOV | op::MOVI | op::CMP |
        // Jumps and branches
        op::JMP | op::JZ | op::JNZ | op::JN | op::JP | op::JC | op::JNC |
        // Call and return
        op::CALL | op::RET | op::JMPR | op::CALLR |
        // Memory operations
        op::LD | op::ST | op::LDB | op::STB | op::LDH | op::STH |
        // Atomic operations (Level 6)
        op::CLI | op::STI | op::XCHG | op::CAS |
        // ASCEND-LINUX (ADR-067): memory ordering + privilege + LR/SC
        op::FENCE | op::MRET | op::SRET | op::LR_W | op::SC_W |
        // System operations
        op::SYSCALL | op::HALT
    )
}

/// Decodes a 32-bit instruction value with opcode validation.
///
/// This function extracts the opcode from the instruction and validates it against
/// all known MBC opcodes. The register fields (dst, src) are automatically masked
/// to 4 bits during extraction by MbcInsn.
///
/// # Arguments
///
/// * `insn_word` - The 32-bit instruction word to decode
///
/// # Returns
///
/// * `Ok(MbcInsn)` if the opcode is valid
/// * `Err(DecodeError::InvalidOpcode)` if the opcode is not recognized
///
/// # Examples
///
/// ```
/// use monad_mbc::instruction::{decode_checked, DecodeError};
/// use monad_common::{MbcInsn, mbc_opcodes as op};
///
/// // Valid instruction: ADD r0, r1, 0x1234
/// let insn = MbcInsn::encode(op::ADD, 0, 1, 0x1234);
/// assert!(decode_checked(insn.0).is_ok());
///
/// // NOP (opcode 0x00) is a valid opcode
/// assert!(decode_checked(0x00000000).is_ok());
///
/// // Invalid instruction: opcode 0x11
/// assert!(decode_checked(0x11000000).is_err());
/// ```
pub fn decode_checked(insn_word: u32) -> Result<MbcInsn, DecodeError> {
    let insn = MbcInsn(insn_word);
    let opcode = insn.opcode();

    if is_valid_opcode(opcode) {
        Ok(insn)
    } else {
        Err(DecodeError::InvalidOpcode(opcode))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // ========== Arithmetic Operations ==========

    #[test]
    fn test_encode_decode_add() {
        let insn = MbcInsn::encode(op::ADD, 1, 2, 0x1234);
        let decoded = decode_checked(insn.0).expect("ADD should be valid");
        assert_eq!(decoded.opcode(), op::ADD);
        assert_eq!(decoded.dst(), 1);
        assert_eq!(decoded.src(), 2);
        assert_eq!(decoded.imm16(), 0x1234);
    }

    #[test]
    fn test_encode_decode_sub() {
        let insn = MbcInsn::encode(op::SUB, 3, 4, 0x5678);
        let decoded = decode_checked(insn.0).expect("SUB should be valid");
        assert_eq!(decoded.opcode(), op::SUB);
        assert_eq!(decoded.dst(), 3);
        assert_eq!(decoded.src(), 4);
        assert_eq!(decoded.imm16(), 0x5678);
    }

    #[test]
    fn test_encode_decode_mul() {
        let insn = MbcInsn::encode(op::MUL, 5, 6, 0x9ABC);
        let decoded = decode_checked(insn.0).expect("MUL should be valid");
        assert_eq!(decoded.opcode(), op::MUL);
        assert_eq!(decoded.dst(), 5);
        assert_eq!(decoded.src(), 6);
    }

    #[test]
    fn test_encode_decode_div() {
        let insn = MbcInsn::encode(op::DIV, 7, 8, 0);
        let decoded = decode_checked(insn.0).expect("DIV should be valid");
        assert_eq!(decoded.opcode(), op::DIV);
        assert_eq!(decoded.dst(), 7);
        assert_eq!(decoded.src(), 8);
    }

    #[test]
    fn test_encode_decode_mod() {
        let insn = MbcInsn::encode(op::MOD, 9, 10, 0);
        let decoded = decode_checked(insn.0).expect("MOD should be valid");
        assert_eq!(decoded.opcode(), op::MOD);
    }

    #[test]
    fn test_encode_decode_neg() {
        let insn = MbcInsn::encode(op::NEG, 11, 12, 0);
        let decoded = decode_checked(insn.0).expect("NEG should be valid");
        assert_eq!(decoded.opcode(), op::NEG);
    }

    // ========== Bitwise Operations ==========

    #[test]
    fn test_encode_decode_and() {
        let insn = MbcInsn::encode(op::AND, 0, 1, 0xFFFF);
        let decoded = decode_checked(insn.0).expect("AND should be valid");
        assert_eq!(decoded.opcode(), op::AND);
        assert_eq!(decoded.imm16(), 0xFFFF);
    }

    #[test]
    fn test_encode_decode_or() {
        let insn = MbcInsn::encode(op::OR, 2, 3, 0xAAAA);
        let decoded = decode_checked(insn.0).expect("OR should be valid");
        assert_eq!(decoded.opcode(), op::OR);
    }

    #[test]
    fn test_encode_decode_xor() {
        let insn = MbcInsn::encode(op::XOR, 4, 5, 0x5555);
        let decoded = decode_checked(insn.0).expect("XOR should be valid");
        assert_eq!(decoded.opcode(), op::XOR);
    }

    #[test]
    fn test_encode_decode_not() {
        let insn = MbcInsn::encode(op::NOT, 6, 7, 0);
        let decoded = decode_checked(insn.0).expect("NOT should be valid");
        assert_eq!(decoded.opcode(), op::NOT);
    }

    #[test]
    fn test_encode_decode_shl() {
        let insn = MbcInsn::encode(op::SHL, 8, 9, 5);
        let decoded = decode_checked(insn.0).expect("SHL should be valid");
        assert_eq!(decoded.opcode(), op::SHL);
    }

    #[test]
    fn test_encode_decode_shr() {
        let insn = MbcInsn::encode(op::SHR, 10, 11, 3);
        let decoded = decode_checked(insn.0).expect("SHR should be valid");
        assert_eq!(decoded.opcode(), op::SHR);
    }

    #[test]
    fn test_encode_decode_sar() {
        let insn = MbcInsn::encode(op::SAR, 12, 13, 4);
        let decoded = decode_checked(insn.0).expect("SAR should be valid");
        assert_eq!(decoded.opcode(), op::SAR);
    }

    #[test]
    fn test_encode_decode_shlr() {
        let insn = MbcInsn::encode(op::SHLR, 0, 1, 0);
        let decoded = decode_checked(insn.0).expect("SHLR should be valid");
        assert_eq!(decoded.opcode(), op::SHLR);
    }

    #[test]
    fn test_encode_decode_shrr() {
        let insn = MbcInsn::encode(op::SHRR, 2, 3, 0);
        let decoded = decode_checked(insn.0).expect("SHRR should be valid");
        assert_eq!(decoded.opcode(), op::SHRR);
    }

    #[test]
    fn test_encode_decode_sarr() {
        let insn = MbcInsn::encode(op::SARR, 4, 5, 0);
        let decoded = decode_checked(insn.0).expect("SARR should be valid");
        assert_eq!(decoded.opcode(), op::SARR);
    }

    #[test]
    fn test_encode_decode_mulh() {
        let insn = MbcInsn::encode(op::MULH, 6, 7, 0);
        let decoded = decode_checked(insn.0).expect("MULH should be valid");
        assert_eq!(decoded.opcode(), op::MULH);
    }

    // ========== Data Movement ==========

    #[test]
    fn test_encode_decode_mov() {
        let insn = MbcInsn::encode(op::MOV, 0, 1, 0);
        let decoded = decode_checked(insn.0).expect("MOV should be valid");
        assert_eq!(decoded.opcode(), op::MOV);
        assert_eq!(decoded.dst(), 0);
        assert_eq!(decoded.src(), 1);
    }

    #[test]
    fn test_encode_decode_movi() {
        let insn = MbcInsn::encode(op::MOVI, 2, 3, 0x4567);
        let decoded = decode_checked(insn.0).expect("MOVI should be valid");
        assert_eq!(decoded.opcode(), op::MOVI);
        assert_eq!(decoded.imm16(), 0x4567);
    }

    #[test]
    fn test_encode_decode_cmp() {
        let insn = MbcInsn::encode(op::CMP, 4, 5, 0x89AB);
        let decoded = decode_checked(insn.0).expect("CMP should be valid");
        assert_eq!(decoded.opcode(), op::CMP);
    }

    // ========== Jumps and Branches ==========

    #[test]
    fn test_encode_decode_jmp() {
        let insn = MbcInsn::encode(op::JMP, 0, 0, 0x0100);
        let decoded = decode_checked(insn.0).expect("JMP should be valid");
        assert_eq!(decoded.opcode(), op::JMP);
        assert_eq!(decoded.imm16(), 0x0100);
    }

    #[test]
    fn test_encode_decode_jz() {
        let insn = MbcInsn::encode(op::JZ, 0, 0, 0x0200);
        let decoded = decode_checked(insn.0).expect("JZ should be valid");
        assert_eq!(decoded.opcode(), op::JZ);
    }

    #[test]
    fn test_encode_decode_jnz() {
        let insn = MbcInsn::encode(op::JNZ, 0, 0, 0x0300);
        let decoded = decode_checked(insn.0).expect("JNZ should be valid");
        assert_eq!(decoded.opcode(), op::JNZ);
    }

    #[test]
    fn test_encode_decode_jn() {
        let insn = MbcInsn::encode(op::JN, 0, 0, 0x0400);
        let decoded = decode_checked(insn.0).expect("JN should be valid");
        assert_eq!(decoded.opcode(), op::JN);
    }

    #[test]
    fn test_encode_decode_jp() {
        let insn = MbcInsn::encode(op::JP, 0, 0, 0x0500);
        let decoded = decode_checked(insn.0).expect("JP should be valid");
        assert_eq!(decoded.opcode(), op::JP);
    }

    #[test]
    fn test_encode_decode_jc() {
        let insn = MbcInsn::encode(op::JC, 0, 0, 0x0600);
        let decoded = decode_checked(insn.0).expect("JC should be valid");
        assert_eq!(decoded.opcode(), op::JC);
    }

    #[test]
    fn test_encode_decode_jnc() {
        let insn = MbcInsn::encode(op::JNC, 0, 0, 0x0700);
        let decoded = decode_checked(insn.0).expect("JNC should be valid");
        assert_eq!(decoded.opcode(), op::JNC);
    }

    // ========== Call and Return ==========

    #[test]
    fn test_encode_decode_call() {
        let insn = MbcInsn::encode(op::CALL, 0, 0, 0x0800);
        let decoded = decode_checked(insn.0).expect("CALL should be valid");
        assert_eq!(decoded.opcode(), op::CALL);
    }

    #[test]
    fn test_encode_decode_ret() {
        let insn = MbcInsn::encode(op::RET, 0, 0, 0);
        let decoded = decode_checked(insn.0).expect("RET should be valid");
        assert_eq!(decoded.opcode(), op::RET);
    }

    #[test]
    fn test_encode_decode_jmpr() {
        let insn = MbcInsn::encode(op::JMPR, 1, 0, 0);
        let decoded = decode_checked(insn.0).expect("JMPR should be valid");
        assert_eq!(decoded.opcode(), op::JMPR);
        assert_eq!(decoded.dst(), 1);
    }

    #[test]
    fn test_encode_decode_callr() {
        let insn = MbcInsn::encode(op::CALLR, 2, 0, 0);
        let decoded = decode_checked(insn.0).expect("CALLR should be valid");
        assert_eq!(decoded.opcode(), op::CALLR);
    }

    // ========== Memory Operations ==========

    #[test]
    fn test_encode_decode_ld() {
        let insn = MbcInsn::encode(op::LD, 0, 1, 0);
        let decoded = decode_checked(insn.0).expect("LD should be valid");
        assert_eq!(decoded.opcode(), op::LD);
    }

    #[test]
    fn test_encode_decode_st() {
        let insn = MbcInsn::encode(op::ST, 2, 3, 0);
        let decoded = decode_checked(insn.0).expect("ST should be valid");
        assert_eq!(decoded.opcode(), op::ST);
    }

    #[test]
    fn test_encode_decode_ldb() {
        let insn = MbcInsn::encode(op::LDB, 4, 5, 0);
        let decoded = decode_checked(insn.0).expect("LDB should be valid");
        assert_eq!(decoded.opcode(), op::LDB);
    }

    #[test]
    fn test_encode_decode_stb() {
        let insn = MbcInsn::encode(op::STB, 6, 7, 0);
        let decoded = decode_checked(insn.0).expect("STB should be valid");
        assert_eq!(decoded.opcode(), op::STB);
    }

    #[test]
    fn test_encode_decode_ldh() {
        let insn = MbcInsn::encode(op::LDH, 8, 9, 0);
        let decoded = decode_checked(insn.0).expect("LDH should be valid");
        assert_eq!(decoded.opcode(), op::LDH);
    }

    #[test]
    fn test_encode_decode_sth() {
        let insn = MbcInsn::encode(op::STH, 10, 11, 0);
        let decoded = decode_checked(insn.0).expect("STH should be valid");
        assert_eq!(decoded.opcode(), op::STH);
    }

    // ========== System Operations ==========

    #[test]
    fn test_encode_decode_syscall() {
        let insn = MbcInsn::encode(op::SYSCALL, 0, 0, 0x0001);
        let decoded = decode_checked(insn.0).expect("SYSCALL should be valid");
        assert_eq!(decoded.opcode(), op::SYSCALL);
    }

    #[test]
    fn test_encode_decode_halt() {
        let insn = MbcInsn::encode(op::HALT, 0, 0, 0);
        let decoded = decode_checked(insn.0).expect("HALT should be valid");
        assert_eq!(decoded.opcode(), op::HALT);
    }

    // ========== Invalid Opcodes ==========

    #[test]
    fn test_valid_opcode_nop_0x00() {
        let result = decode_checked(0x00000000);
        assert!(result.is_ok(), "NOP (0x00) should be valid");
        assert_eq!(result.unwrap().opcode(), op::NOP);
    }

    #[test]
    fn test_invalid_opcode_0x11() {
        // 0x11 is in the reserved range 0x11-0x19
        let insn = MbcInsn::encode(0x11, 0, 0, 0);
        let result = decode_checked(insn.0);
        assert!(result.is_err());
        assert_eq!(result.unwrap_err(), DecodeError::InvalidOpcode(0x11));
    }

    #[test]
    fn test_invalid_opcode_0x1F() {
        // 0x1F is in the reserved range 0x1E-0x1F
        let insn = MbcInsn::encode(0x1F, 0, 0, 0);
        let result = decode_checked(insn.0);
        assert!(result.is_err());
        assert_eq!(result.unwrap_err(), DecodeError::InvalidOpcode(0x1F));
    }

    #[test]
    fn test_valid_opcode_push() {
        let insn = MbcInsn::encode(op::PUSH, 3, 0, 0);
        let decoded = decode_checked(insn.0).expect("PUSH should be valid");
        assert_eq!(decoded.opcode(), op::PUSH);
        assert_eq!(decoded.dst(), 3);
    }

    #[test]
    fn test_valid_opcode_pop() {
        let insn = MbcInsn::encode(op::POP, 5, 0, 0);
        let decoded = decode_checked(insn.0).expect("POP should be valid");
        assert_eq!(decoded.opcode(), op::POP);
        assert_eq!(decoded.dst(), 5);
    }

    #[test]
    fn test_valid_opcode_load_imm32() {
        let insn = MbcInsn::encode(op::LOAD_IMM32, 2, 0, 0xABCD);
        let decoded = decode_checked(insn.0).expect("LOAD_IMM32 should be valid");
        assert_eq!(decoded.opcode(), op::LOAD_IMM32);
        assert_eq!(decoded.dst(), 2);
        assert_eq!(decoded.imm16(), 0xABCD);
    }

    #[test]
    fn test_valid_opcode_addi() {
        let insn = MbcInsn::encode(op::ADDI, 1, 0, 42);
        let decoded = decode_checked(insn.0).expect("ADDI should be valid");
        assert_eq!(decoded.opcode(), op::ADDI);
        assert_eq!(decoded.dst(), 1);
        assert_eq!(decoded.imm16(), 42);
    }

    #[test]
    fn test_invalid_opcode_0x2B() {
        // 0x2B is in the reserved range 0x2B-0x2F
        let insn = MbcInsn::encode(0x2B, 0, 0, 0);
        let result = decode_checked(insn.0);
        assert!(result.is_err());
        assert_eq!(result.unwrap_err(), DecodeError::InvalidOpcode(0x2B));
    }

    #[test]
    fn test_invalid_opcode_0x2F() {
        // 0x2F is the last invalid opcode in range 0x2B-0x2F
        let insn = MbcInsn::encode(0x2F, 0, 0, 0);
        let result = decode_checked(insn.0);
        assert!(result.is_err());
        assert_eq!(result.unwrap_err(), DecodeError::InvalidOpcode(0x2F));
    }

    #[test]
    fn test_invalid_opcode_0x3A() {
        // 0x3A-0x3F are reserved
        let insn = MbcInsn::encode(0x3A, 0, 0, 0);
        let result = decode_checked(insn.0);
        assert!(result.is_err());
        assert_eq!(result.unwrap_err(), DecodeError::InvalidOpcode(0x3A));
    }

    #[test]
    fn test_invalid_opcode_0x41() {
        // 0x41-0xFE are reserved
        let insn = MbcInsn::encode(0x41, 0, 0, 0);
        let result = decode_checked(insn.0);
        assert!(result.is_err());
        assert_eq!(result.unwrap_err(), DecodeError::InvalidOpcode(0x41));
    }

    // ========== Register Field Masking ==========

    #[test]
    fn test_register_masking_dst() {
        // Test that 0xFF input is masked to 0x0F output
        let insn = MbcInsn::encode(op::MOV, 0xFF, 0, 0);
        let decoded = decode_checked(insn.0).expect("Valid opcode");
        assert_eq!(decoded.dst(), 0x0F, "dst field should be masked to 4 bits");
    }

    #[test]
    fn test_register_masking_src() {
        // Test that 0xFF input is masked to 0x0F output
        let insn = MbcInsn::encode(op::MOV, 0, 0xFF, 0);
        let decoded = decode_checked(insn.0).expect("Valid opcode");
        assert_eq!(decoded.src(), 0x0F, "src field should be masked to 4 bits");
    }

    #[test]
    fn test_register_masking_both() {
        // Test both registers with high bits set
        let insn = MbcInsn::encode(op::ADD, 0xAB, 0xCD, 0);
        let decoded = decode_checked(insn.0).expect("Valid opcode");
        assert_eq!(decoded.dst(), 0x0B);
        assert_eq!(decoded.src(), 0x0D);
    }

    #[test]
    fn test_register_field_isolation() {
        // Verify that register fields don't interfere with each other
        let insn = MbcInsn::encode(op::MOV, 0x0F, 0x0E, 0);
        let decoded = decode_checked(insn.0).expect("Valid opcode");
        assert_eq!(decoded.dst(), 0x0F);
        assert_eq!(decoded.src(), 0x0E);
    }

    // ========== Sign Extension of imm16 ==========

    #[test]
    fn test_imm16_sign_extension_all_ones() {
        // 0xFFFF should sign-extend to -1i16
        let insn = MbcInsn::encode(op::MOVI, 0, 0, 0xFFFF);
        let decoded = decode_checked(insn.0).expect("Valid opcode");
        assert_eq!(decoded.imm16(), 0xFFFF);
        assert_eq!(decoded.imm16_signed(), -1i16);
    }

    #[test]
    fn test_imm16_sign_extension_sign_bit_set() {
        // 0x8000 should sign-extend to -32768i16
        let insn = MbcInsn::encode(op::MOVI, 0, 0, 0x8000);
        let decoded = decode_checked(insn.0).expect("Valid opcode");
        assert_eq!(decoded.imm16(), 0x8000);
        assert_eq!(decoded.imm16_signed(), -32768i16);
    }

    #[test]
    fn test_imm16_sign_extension_zero() {
        // 0x0000 should remain 0i16
        let insn = MbcInsn::encode(op::MOVI, 0, 0, 0x0000);
        let decoded = decode_checked(insn.0).expect("Valid opcode");
        assert_eq!(decoded.imm16(), 0x0000);
        assert_eq!(decoded.imm16_signed(), 0i16);
    }

    #[test]
    fn test_imm16_sign_extension_positive() {
        // 0x7FFF should remain 32767i16 (largest positive i16)
        let insn = MbcInsn::encode(op::MOVI, 0, 0, 0x7FFF);
        let decoded = decode_checked(insn.0).expect("Valid opcode");
        assert_eq!(decoded.imm16(), 0x7FFF);
        assert_eq!(decoded.imm16_signed(), 32767i16);
    }

    #[test]
    fn test_imm16_sign_extension_negative_various() {
        // Test various negative values
        let test_cases = vec![
            (0xFFFF, -1i16),
            (0xFFFE, -2i16),
            (0x8001, -32767i16),
            (0x8000, -32768i16),
            (0xC000, -16384i16),
        ];

        for (unsigned, signed) in test_cases {
            let insn = MbcInsn::encode(op::MOVI, 0, 0, unsigned);
            let decoded = decode_checked(insn.0).expect("Valid opcode");
            assert_eq!(
                decoded.imm16_signed(),
                signed,
                "0x{:04X} should sign-extend to {}",
                unsigned,
                signed
            );
        }
    }

    // ========== is_valid_opcode Function ==========

    #[test]
    fn test_is_valid_opcode_all_valid() {
        let valid_opcodes = vec![
            op::NOP,
            op::ADD,
            op::SUB,
            op::MUL,
            op::DIV,
            op::MOD,
            op::NEG,
            op::AND,
            op::OR,
            op::XOR,
            op::NOT,
            op::SHL,
            op::SHR,
            op::SAR,
            op::PUSH,
            op::POP,
            op::LOAD_IMM32,
            op::ADDI,
            op::SHLR,
            op::SHRR,
            op::SARR,
            op::MULH,
            op::MOV,
            op::MOVI,
            op::CMP,
            op::JMP,
            op::JZ,
            op::JNZ,
            op::JN,
            op::JP,
            op::JC,
            op::JNC,
            op::CALL,
            op::RET,
            op::JMPR,
            op::CALLR,
            op::LD,
            op::ST,
            op::LDB,
            op::STB,
            op::LDH,
            op::STH,
            op::SYSCALL,
            op::HALT,
        ];

        for opcode in valid_opcodes {
            assert!(
                is_valid_opcode(opcode),
                "Opcode 0x{:02X} should be valid",
                opcode
            );
        }
    }

    #[test]
    fn test_is_valid_opcode_all_invalid() {
        // ADR-067 (ASCEND-LINUX): 0x18 IRET, 0x3A MULHU, 0x3F FENCE, 0x47 MRET,
        // 0x48 SRET, 0x49 LR.W, 0x4A SC.W are NOW valid; removed from this list.
        let invalid_opcodes: Vec<u8> = vec![
            0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x19, 0x1E, 0x1F, 0x2B, 0x2C, 0x2D,
            0x2E, 0x2F, 0x41, 0x42, 0x43, 0x44, 0x45, 0x46, 0x4B, 0x50, 0xFE,
        ];

        for opcode in invalid_opcodes {
            assert!(
                !is_valid_opcode(opcode),
                "Opcode 0x{:02X} should be invalid",
                opcode
            );
        }
    }

    // ========== Round-trip Encode/Decode with Various Immediate Values ==========

    #[test]
    fn test_roundtrip_with_various_immediates() {
        let immediates = vec![
            0x0000, 0x0001, 0x00FF, 0x0100, 0x7FFF, 0x8000, 0x8001, 0xFFFF,
        ];

        for imm in immediates {
            let insn = MbcInsn::encode(op::MOVI, 5, 0, imm);
            let decoded = decode_checked(insn.0).expect("Valid opcode");
            assert_eq!(
                decoded.imm16(),
                imm,
                "Immediate should round-trip correctly"
            );
        }
    }

    #[test]
    fn test_roundtrip_with_all_registers() {
        for dst in 0..16 {
            for src in 0..16 {
                let insn = MbcInsn::encode(op::ADD, dst as u8, src as u8, 0x1234);
                let decoded = decode_checked(insn.0).expect("Valid opcode");
                assert_eq!(decoded.dst(), dst as u8);
                assert_eq!(decoded.src(), src as u8);
            }
        }
    }
}
