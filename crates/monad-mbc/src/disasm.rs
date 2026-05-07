//! MBC disassembler: converts bytecode back into human-readable assembly.

use monad_common::{mbc_opcodes as op, MbcInsn};

/// Disassemble a single 32-bit instruction word to a human-readable string.
///
/// # Arguments
/// * `word` - The 32-bit instruction word
/// * `addr` - The program counter (word address) for jump target calculation
///
/// # Returns
/// A string containing the disassembled instruction
pub fn disasm_insn(word: u32, addr: u32) -> String {
    let insn = MbcInsn(word);
    let opcode = insn.opcode();
    let dst = insn.dst();
    let src = insn.src();
    let imm = insn.imm16_signed();
    let imm_u = insn.imm16();

    match opcode {
        op::NOP => "NOP".to_string(),
        op::ADD => format!("ADD  r{}, r{}", dst, src),
        op::SUB => format!("SUB  r{}, r{}", dst, src),
        op::MUL => format!("MUL  r{}, r{}", dst, src),
        op::DIV => format!("DIV  r{}, r{}", dst, src),
        op::MOD => format!("MOD  r{}, r{}", dst, src),
        op::NEG => format!("NEG  r{}", dst),
        op::AND => format!("AND  r{}, r{}", dst, src),
        op::OR => format!("OR   r{}, r{}", dst, src),
        op::XOR => format!("XOR  r{}, r{}", dst, src),
        op::NOT => format!("NOT  r{}", dst),
        op::SHL => format!("SHL  r{}, {}", dst, imm_u),
        op::SHR => format!("SHR  r{}, {}", dst, imm_u),
        op::SAR => format!("SAR  r{}, {}", dst, imm_u),
        op::SHLR => format!("SHLR r{}, r{}", dst, src),
        op::SHRR => format!("SHRR r{}, r{}", dst, src),
        op::SARR => format!("SARR r{}, r{}", dst, src),
        op::MULH => format!("MULH r{}, r{}", dst, src),
        op::PUSH => format!("PUSH r{}", dst),
        op::POP => format!("POP  r{}", dst),
        op::LOAD_IMM32 => format!("LOAD_IMM32 r{}, 0x{:x}", dst, imm_u),
        op::ADDI => format!("ADDI r{}, {}", dst, imm),
        op::MOV => format!("MOV  r{}, r{}", dst, src),
        op::MOVI => format!("MOVI r{}, {}", dst, imm),
        op::CMP => format!("CMP  r{}, r{}", dst, src),

        // Jump instructions with 24-bit signed offset target calculation
        op::JMP | op::JZ | op::JNZ | op::JN | op::JP | op::JC | op::JNC => {
            let raw24 = word & 0x00FF_FFFF;
            let offset = if raw24 & 0x0080_0000 != 0 {
                (raw24 | 0xFF00_0000) as i32
            } else {
                raw24 as i32
            };
            let target = ((addr as i32 + 1) + offset) as u32;
            let mnemonic = match opcode {
                op::JMP => "JMP ",
                op::JZ => "JZ  ",
                op::JNZ => "JNZ ",
                op::JN => "JN  ",
                op::JP => "JP  ",
                op::JC => "JC  ",
                op::JNC => "JNC ",
                _ => unreachable!(),
            };
            format!("{} 0x{:x}", mnemonic, target)
        }

        op::CALL => {
            // Extended 24-bit absolute addressing
            let target = MbcInsn(word).0 & 0x00FF_FFFF;
            format!("CALL 0x{:x}", target)
        }
        op::RET => "RET".to_string(),
        op::JMPR => format!("JMPR r{}", dst),
        op::CALLR => format!("CALLR r{}", dst),

        op::LD => format!("LD   r{}, [r{}+{}]", dst, src, imm),
        op::ST => format!("ST   [r{}+{}], r{}", dst, imm, src),
        op::LDB => format!("LDB  r{}, [r{}+{}]", dst, src, imm),
        op::STB => format!("STB  [r{}+{}], r{}", dst, imm, src),
        op::LDH => format!("LDH  r{}, [r{}+{}]", dst, src, imm),
        op::STH => format!("STH  [r{}+{}], r{}", dst, imm, src),

        // Atomic operations (Level 6)
        op::CLI => "CLI".to_string(),
        op::STI => "STI".to_string(),
        op::XCHG => format!("XCHG r{}, [r{}+{}]", dst, src, imm),
        op::CAS => "CAS  r0, [r1], r2".to_string(),

        op::SYSCALL => {
            // Show named aliases for common syscall numbers
            match imm_u {
                0x01 => "DRAW_FRAME".to_string(),
                0x02 => "GET_KEY".to_string(),
                0x03 => "GET_TICKS".to_string(),
                0x04 => "SLEEP".to_string(),
                0xFF => "HALT_SYSCALL".to_string(),
                _ => format!("SYSCALL 0x{:x}", imm_u),
            }
        }
        op::HALT => "HALT".to_string(),

        _ => format!("UNKNOWN 0x{:02x}", opcode),
    }
}

/// Disassemble a complete program (slice of u32 words) into a vector of (address, instruction) pairs.
///
/// # Arguments
/// * `words` - Slice of 32-bit instruction words
///
/// # Returns
/// A vector of tuples containing (word address, disassembled instruction text)
pub fn disasm_program(words: &[u32]) -> Vec<(u32, String)> {
    words
        .iter()
        .enumerate()
        .map(|(addr, word)| (addr as u32, disasm_insn(*word, addr as u32)))
        .collect()
}

/// Format a complete disassembly listing with addresses and instructions.
///
/// # Arguments
/// * `words` - Slice of 32-bit instruction words
///
/// # Returns
/// A formatted string containing the complete disassembly listing
pub fn disasm_listing(words: &[u32]) -> String {
    let mut output = String::new();
    for (addr, insn) in disasm_program(words) {
        output.push_str(&format!("0x{:04x}: {}\n", addr, insn));
    }
    output
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_disasm_movi() {
        let insn = MbcInsn::encode(op::MOVI, 0, 0, 42);
        let text = disasm_insn(insn.0, 0);
        assert_eq!(text, "MOVI r0, 42");
    }

    #[test]
    fn test_disasm_add() {
        let insn = MbcInsn::encode(op::ADD, 0, 1, 0);
        let text = disasm_insn(insn.0, 0);
        assert_eq!(text, "ADD  r0, r1");
    }

    #[test]
    fn test_disasm_jmp() {
        // Jump at address 5 with offset 3 => target = (5+1)+3 = 9
        let insn = MbcInsn::encode(op::JMP, 0, 0, 3);
        let text = disasm_insn(insn.0, 5);
        assert!(text.contains("0x9"));
    }

    #[test]
    fn test_disasm_halt() {
        let insn = MbcInsn::encode(op::HALT, 0, 0, 0);
        let text = disasm_insn(insn.0, 0);
        assert_eq!(text, "HALT");
    }

    #[test]
    fn test_disasm_ld() {
        let insn = MbcInsn::encode(op::LD, 0, 1, 100);
        let text = disasm_insn(insn.0, 0);
        assert_eq!(text, "LD   r0, [r1+100]");
    }

    #[test]
    fn test_disasm_st() {
        let insn = MbcInsn::encode(op::ST, 0, 1, 50);
        let text = disasm_insn(insn.0, 0);
        assert_eq!(text, "ST   [r0+50], r1");
    }

    #[test]
    fn test_disasm_program() {
        let words = vec![
            MbcInsn::encode(op::MOVI, 0, 0, 10).0,
            MbcInsn::encode(op::HALT, 0, 0, 0).0,
        ];
        let result = disasm_program(&words);
        assert_eq!(result.len(), 2);
        assert_eq!(result[0].0, 0);
        assert!(result[0].1.contains("MOVI"));
        assert_eq!(result[1].0, 1);
        assert!(result[1].1.contains("HALT"));
    }

    #[test]
    fn test_disasm_listing() {
        let words = vec![
            MbcInsn::encode(op::MOVI, 0, 0, 42).0,
            MbcInsn::encode(op::HALT, 0, 0, 0).0,
        ];
        let listing = disasm_listing(&words);
        assert!(listing.contains("0x0000"));
        assert!(listing.contains("MOVI"));
        assert!(listing.contains("0x0001"));
        assert!(listing.contains("HALT"));
    }

    #[test]
    fn test_disasm_negative_immediate() {
        let insn = MbcInsn::encode(op::MOVI, 5, 0, (-50_i16) as u16);
        let text = disasm_insn(insn.0, 0);
        assert!(text.contains("r5"));
        assert!(text.contains("-50") || text.contains("65486")); // Either representation
    }

    #[test]
    fn test_disasm_nop() {
        let insn = MbcInsn::encode(op::NOP, 0, 0, 0);
        let text = disasm_insn(insn.0, 0);
        assert_eq!(text, "NOP");
    }

    #[test]
    fn test_disasm_push() {
        let insn = MbcInsn::encode(op::PUSH, 3, 0, 0);
        let text = disasm_insn(insn.0, 0);
        assert_eq!(text, "PUSH r3");
    }

    #[test]
    fn test_disasm_pop() {
        let insn = MbcInsn::encode(op::POP, 5, 0, 0);
        let text = disasm_insn(insn.0, 0);
        assert_eq!(text, "POP  r5");
    }

    #[test]
    fn test_disasm_addi() {
        let insn = MbcInsn::encode(op::ADDI, 1, 0, 42);
        let text = disasm_insn(insn.0, 0);
        assert_eq!(text, "ADDI r1, 42");
    }

    #[test]
    fn test_disasm_addi_negative() {
        let insn = MbcInsn::encode(op::ADDI, 2, 0, (-10_i16) as u16);
        let text = disasm_insn(insn.0, 0);
        assert_eq!(text, "ADDI r2, -10");
    }

    #[test]
    fn test_disasm_load_imm32() {
        let insn = MbcInsn::encode(op::LOAD_IMM32, 4, 0, 0x1234);
        let text = disasm_insn(insn.0, 0);
        assert_eq!(text, "LOAD_IMM32 r4, 0x1234");
    }

    #[test]
    fn test_disasm_all_arithmetic() {
        let ops = [
            op::ADD,
            op::SUB,
            op::MUL,
            op::DIV,
            op::MOD,
            op::AND,
            op::OR,
            op::XOR,
        ];
        for op_code in ops.iter() {
            let insn = MbcInsn::encode(*op_code, 1, 2, 0);
            let text = disasm_insn(insn.0, 0);
            assert!(text.contains("r1"));
            assert!(text.contains("r2"));
        }
    }
}
