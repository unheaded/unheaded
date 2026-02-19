//! MBC assembler: converts human-readable assembly text to bytecode.
//!
//! Supports standard MBC mnemonics with two-pass assembly:
//! - Pass 1: Collect label definitions and build instruction list
//! - Pass 2: Resolve label references and patch jump offsets

use monad_common::{mbc_opcodes as op, MbcInsn};
use std::collections::HashMap;
use thiserror::Error;

/// Errors that can occur during assembly.
#[derive(Error, Debug, Clone)]
pub enum AssembleError {
    #[error("Line {line}: {message}")]
    Line { line: usize, message: String },

    #[error("Undefined label: {label}")]
    UndefinedLabel { label: String },

    #[error("Invalid instruction: {message}")]
    InvalidInstruction { message: String },

    #[error("Register out of range: {register}")]
    RegisterOutOfRange { register: String },

    #[error("Immediate value out of range: {value}")]
    ImmediateOutOfRange { value: i32 },

    #[error("Invalid label definition")]
    InvalidLabel,
}

/// A single assembled instruction with mutable offset for label resolution.
struct AsmInsn {
    opcode: u8,
    dst: u8,
    src: u8,
    imm: i16,
    label_ref: Option<String>, // For jumps, which label to jump to
}

/// Parse a register name like "r0", "r15" and return its index.
fn parse_register(s: &str) -> Result<u8, AssembleError> {
    if !s.starts_with('r') {
        return Err(AssembleError::InvalidInstruction {
            message: format!("Expected register name, got '{}'", s),
        });
    }
    let num_str = &s[1..];
    let num: u8 = num_str.parse().map_err(|_| AssembleError::InvalidInstruction {
        message: format!("Invalid register: '{}'", s),
    })?;
    if num > 15 {
        return Err(AssembleError::RegisterOutOfRange {
            register: s.to_string(),
        });
    }
    Ok(num)
}

/// Parse an immediate value (decimal, hex with 0x prefix, or negative).
fn parse_immediate(s: &str) -> Result<i16, AssembleError> {
    let s = s.trim();
    let val: i64 = if s.starts_with("0x") || s.starts_with("0X") {
        i64::from_str_radix(&s[2..], 16).map_err(|_| AssembleError::InvalidInstruction {
            message: format!("Invalid hex immediate: '{}'", s),
        })?
    } else {
        s.parse().map_err(|_| AssembleError::InvalidInstruction {
            message: format!("Invalid immediate: '{}'", s),
        })?
    };

    // Accept both signed i16 range (-32768..32767) and unsigned u16 range (0..65535).
    // Values 32768..65535 (e.g. 0xC000) are stored as their u16 bit pattern reinterpreted as i16.
    if val < -32768 || val > 65535 {
        return Err(AssembleError::ImmediateOutOfRange { value: val as i32 });
    }
    Ok(val as u16 as i16)
}

/// Strip comments and whitespace from a line.
fn strip_comment(line: &str) -> &str {
    if let Some(pos) = line.find(';') {
        &line[..pos]
    } else if let Some(pos) = line.find('#') {
        &line[..pos]
    } else {
        line
    }
}

/// Tokenize a line into non-whitespace tokens.
fn tokenize(line: &str) -> Vec<&str> {
    line.split_whitespace().collect()
}

/// Parse a single instruction line. Returns the instruction and whether line was blank/comment.
fn parse_instruction(
    line: &str,
    line_num: usize,
) -> Result<Option<AsmInsn>, AssembleError> {
    let line = strip_comment(line).trim();
    if line.is_empty() {
        return Ok(None);
    }

    let tokens = tokenize(line);
    if tokens.is_empty() {
        return Ok(None);
    }

    let mnemonic = tokens[0].to_uppercase();

    macro_rules! require_tokens {
        ($count:expr) => {
            if tokens.len() < $count + 1 {
                return Err(AssembleError::Line {
                    line: line_num,
                    message: format!(
                        "{} requires {} operands, got {}",
                        mnemonic,
                        $count,
                        tokens.len() - 1
                    ),
                });
            }
        };
    }

    // Two-operand register instructions: ADD, SUB, MUL, etc.
    if matches!(
        mnemonic.as_str(),
        "ADD" | "SUB" | "MUL" | "DIV" | "MOD" | "AND" | "OR" | "XOR" | "MOV" | "CMP"
    ) {
        require_tokens!(2);
        let dst = parse_register(tokens[1].trim_end_matches(','))?;
        let src = parse_register(tokens[2].trim_end_matches(','))?;

        let opcode = match mnemonic.as_str() {
            "ADD" => op::ADD,
            "SUB" => op::SUB,
            "MUL" => op::MUL,
            "DIV" => op::DIV,
            "MOD" => op::MOD,
            "AND" => op::AND,
            "OR" => op::OR,
            "XOR" => op::XOR,
            "MOV" => op::MOV,
            "CMP" => op::CMP,
            _ => unreachable!(),
        };

        return Ok(Some(AsmInsn {
            opcode,
            dst,
            src,
            imm: 0,
            label_ref: None,
        }));
    }

    // Single-operand instructions: NEG, NOT, RET, HALT, SYSCALL
    if matches!(mnemonic.as_str(), "NEG" | "NOT") {
        require_tokens!(1);
        let dst = parse_register(tokens[1]).map_err(|_| AssembleError::Line {
            line: line_num,
            message: format!("Invalid register: '{}'", tokens[1]),
        })?;

        let opcode = match mnemonic.as_str() {
            "NEG" => op::NEG,
            "NOT" => op::NOT,
            _ => unreachable!(),
        };

        return Ok(Some(AsmInsn {
            opcode,
            dst,
            src: 0,
            imm: 0,
            label_ref: None,
        }));
    }

    if mnemonic == "RET" {
        return Ok(Some(AsmInsn {
            opcode: op::RET,
            dst: 0,
            src: 0,
            imm: 0,
            label_ref: None,
        }));
    }

    if mnemonic == "HALT" {
        return Ok(Some(AsmInsn {
            opcode: op::HALT,
            dst: 0,
            src: 0,
            imm: 0,
            label_ref: None,
        }));
    }

    if mnemonic == "SYSCALL" {
        // SYSCALL takes an immediate syscall number
        require_tokens!(1);
        let imm = parse_immediate(tokens[1].trim_end_matches(','))
            .map_err(|_| AssembleError::Line {
                line: line_num,
                message: format!("Invalid syscall number: '{}'", tokens[1]),
            })?;

        return Ok(Some(AsmInsn {
            opcode: op::SYSCALL,
            dst: 0,
            src: 0,
            imm,
            label_ref: None,
        }));
    }

    // SYSCALL named aliases (sugar for SYSCALL with specific imm16 values)
    if mnemonic == "DRAW_FRAME" {
        return Ok(Some(AsmInsn {
            opcode: op::SYSCALL,
            dst: 0,
            src: 0,
            imm: 0x01,
            label_ref: None,
        }));
    }

    if mnemonic == "GET_KEY" {
        return Ok(Some(AsmInsn {
            opcode: op::SYSCALL,
            dst: 0,
            src: 0,
            imm: 0x02,
            label_ref: None,
        }));
    }

    if mnemonic == "GET_TICKS" {
        return Ok(Some(AsmInsn {
            opcode: op::SYSCALL,
            dst: 0,
            src: 0,
            imm: 0x03,
            label_ref: None,
        }));
    }

    if mnemonic == "SLEEP" {
        return Ok(Some(AsmInsn {
            opcode: op::SYSCALL,
            dst: 0,
            src: 0,
            imm: 0x04,
            label_ref: None,
        }));
    }

    // Shift instructions: SHL, SHR, SAR
    if matches!(mnemonic.as_str(), "SHL" | "SHR" | "SAR") {
        require_tokens!(2);
        let dst = parse_register(tokens[1].trim_end_matches(',')).map_err(|_| AssembleError::Line {
            line: line_num,
            message: format!("Invalid register: '{}'", tokens[1]),
        })?;
        let imm = parse_immediate(tokens[2].trim_end_matches(','))
            .map_err(|_| AssembleError::Line {
                line: line_num,
                message: format!("Invalid immediate: '{}'", tokens[2]),
            })?;

        let opcode = match mnemonic.as_str() {
            "SHL" => op::SHL,
            "SHR" => op::SHR,
            "SAR" => op::SAR,
            _ => unreachable!(),
        };

        return Ok(Some(AsmInsn {
            opcode,
            dst,
            src: 0,
            imm,
            label_ref: None,
        }));
    }

    // MOVI instruction
    if mnemonic == "MOVI" {
        require_tokens!(2);
        let dst = parse_register(tokens[1].trim_end_matches(',')).map_err(|_| AssembleError::Line {
            line: line_num,
            message: format!("Invalid register: '{}'", tokens[1]),
        })?;
        let imm = parse_immediate(tokens[2].trim_end_matches(','))
            .map_err(|_| AssembleError::Line {
                line: line_num,
                message: format!("Invalid immediate: '{}'", tokens[2]),
            })?;

        return Ok(Some(AsmInsn {
            opcode: op::MOVI,
            dst,
            src: 0,
            imm,
            label_ref: None,
        }));
    }

    // Jump instructions with labels
    if matches!(
        mnemonic.as_str(),
        "JMP" | "JZ" | "JNZ" | "JN" | "JP" | "JC" | "JNC" | "CALL"
    ) {
        require_tokens!(1);
        let label = tokens[1].to_string();

        let opcode = match mnemonic.as_str() {
            "JMP" => op::JMP,
            "JZ" => op::JZ,
            "JNZ" => op::JNZ,
            "JN" => op::JN,
            "JP" => op::JP,
            "JC" => op::JC,
            "JNC" => op::JNC,
            "CALL" => op::CALL,
            _ => unreachable!(),
        };

        return Ok(Some(AsmInsn {
            opcode,
            dst: 0,
            src: 0,
            imm: 0,
            label_ref: Some(label),
        }));
    }

    // Memory instructions: LD, ST, LDB, STB, LDH, STH
    if matches!(
        mnemonic.as_str(),
        "LD" | "ST" | "LDB" | "STB" | "LDH" | "STH"
    ) {
        require_tokens!(2);

        // Parse memory operand format: [rN+M] or [rN-M]
        let mem_str = if mnemonic.starts_with('S') {
            // ST: [r0+N], r1
            tokens[1].trim_end_matches(',')
        } else {
            // LD: r0, [r1+N]
            tokens[2].trim_end_matches(',')
        };

        if !mem_str.starts_with('[') || !mem_str.ends_with(']') {
            return Err(AssembleError::Line {
                line: line_num,
                message: format!("Invalid memory operand: '{}'", mem_str),
            });
        }

        let inner = &mem_str[1..mem_str.len() - 1];
        let neg_offset;
        let (reg_str, offset_str) = if let Some(plus_pos) = inner.find('+') {
            let (r, o) = inner.split_at(plus_pos);
            (r.trim(), o[1..].trim())
        } else if let Some(minus_pos) = inner.find('-') {
            let (r, o) = inner.split_at(minus_pos);
            neg_offset = format!("-{}", o[1..].trim());
            (r.trim(), neg_offset.as_str())
        } else {
            return Err(AssembleError::Line {
                line: line_num,
                message: format!("Memory operand must have offset: '{}'", mem_str),
            });
        };

        let reg = parse_register(reg_str).map_err(|_| AssembleError::Line {
            line: line_num,
            message: format!("Invalid register in memory operand: '{}'", reg_str),
        })?;

        let offset = parse_immediate(offset_str).map_err(|_| AssembleError::Line {
            line: line_num,
            message: format!("Invalid offset: '{}'", offset_str),
        })?;

        if mnemonic.starts_with('S') {
            // ST: [r0+N], r1
            let base_reg = reg;
            let val_reg = parse_register(tokens[2].trim_end_matches(','))
                .map_err(|_| AssembleError::Line {
                    line: line_num,
                    message: format!("Invalid register: '{}'", tokens[2]),
                })?;

            let opcode = match mnemonic.as_str() {
                "ST" => op::ST,
                "STB" => op::STB,
                "STH" => op::STH,
                _ => unreachable!(),
            };

            return Ok(Some(AsmInsn {
                opcode,
                dst: base_reg,
                src: val_reg,
                imm: offset,
                label_ref: None,
            }));
        } else {
            // LD: r0, [r1+N]
            let val_reg = parse_register(tokens[1].trim_end_matches(',')).map_err(|_| AssembleError::Line {
                line: line_num,
                message: format!("Invalid register: '{}'", tokens[1]),
            })?;

            let opcode = match mnemonic.as_str() {
                "LD" => op::LD,
                "LDB" => op::LDB,
                "LDH" => op::LDH,
                _ => unreachable!(),
            };

            return Ok(Some(AsmInsn {
                opcode,
                dst: val_reg,
                src: reg,
                imm: offset,
                label_ref: None,
            }));
        }
    }

    Err(AssembleError::Line {
        line: line_num,
        message: format!("Unknown mnemonic: '{}'", mnemonic),
    })
}

/// Assemble MBC assembly source code into bytecode.
///
/// The assembler performs a two-pass process:
/// 1. First pass: Collect label definitions and build instruction list
/// 2. Second pass: Resolve label references and encode instructions
///
/// # Errors
/// Returns an AssembleError if parsing fails or labels are undefined.
pub fn assemble(source: &str) -> Result<Vec<u32>, AssembleError> {
    let mut labels: HashMap<String, u32> = HashMap::new();
    let mut instructions: Vec<AsmInsn> = Vec::new();
    let mut pc = 0u32;

    // Pass 1: Collect labels and instructions
    for (line_num_0, line) in source.lines().enumerate() {
        let line_num = line_num_0 + 1;
        let line = line.trim();

        // Check for label definition (ends with ':')
        if line.ends_with(':') && !line.starts_with(';') && !line.starts_with('#') {
            let label_part = strip_comment(line);
            let label_name = label_part.trim_end_matches(':').trim();

            if label_name.is_empty() {
                return Err(AssembleError::InvalidLabel);
            }

            // Check for valid label (alphanumeric, underscore, no spaces)
            if !label_name
                .chars()
                .all(|c| c.is_alphanumeric() || c == '_')
            {
                return Err(AssembleError::InvalidLabel);
            }

            labels.insert(label_name.to_string(), pc);
            continue;
        }

        if let Some(insn) = parse_instruction(line, line_num)? {
            instructions.push(insn);
            pc += 1;
        }
    }

    // Pass 2: Encode instructions with resolved labels
    let mut output = Vec::new();

    for (idx, insn) in instructions.iter().enumerate() {
        let imm = if let Some(ref label) = insn.label_ref {
            let label_pc = labels
                .get(label)
                .ok_or_else(|| AssembleError::UndefinedLabel {
                    label: label.clone(),
                })?;

            // Compute relative offset: target - (current_pc + 1)
            let current_pc = idx as u32;
            let relative_offset = (*label_pc as i32) - (current_pc as i32 + 1);

            if relative_offset < -32768 || relative_offset > 32767 {
                return Err(AssembleError::ImmediateOutOfRange {
                    value: relative_offset,
                });
            }

            relative_offset as i16
        } else {
            insn.imm
        };

        let word = MbcInsn::encode(insn.opcode, insn.dst, insn.src, imm as u16);
        output.push(word.0);
    }

    Ok(output)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_assemble_simple() {
        let code = "MOVI r0, 42\nHALT";
        let result = assemble(code).expect("assembly should succeed");
        assert_eq!(result.len(), 2);
    }

    #[test]
    fn test_assemble_with_labels() {
        let code = "START:\nMOVI r0, 10\nJMP START\nHALT";
        let result = assemble(code).expect("assembly should succeed");
        assert_eq!(result.len(), 3);
    }

    #[test]
    fn test_assemble_comments() {
        let code = "; this is a comment\nMOVI r0, 5 ; inline comment\nHALT";
        let result = assemble(code).expect("assembly should succeed");
        assert_eq!(result.len(), 2);
    }

    #[test]
    fn test_assemble_two_operand() {
        let code = "MOVI r0, 10\nMOVI r1, 20\nADD r0, r1\nHALT";
        let result = assemble(code).expect("assembly should succeed");
        assert_eq!(result.len(), 4);
    }

    #[test]
    fn test_assemble_undefined_label() {
        let code = "JMP UNDEFINED";
        let result = assemble(code);
        assert!(matches!(result, Err(AssembleError::UndefinedLabel { .. })));
    }

    #[test]
    fn test_assemble_invalid_register() {
        let code = "ADD r20, r0";
        let result = assemble(code);
        assert!(matches!(result, Err(AssembleError::RegisterOutOfRange { .. })));
    }

    #[test]
    fn test_assemble_memory_operations() {
        let code = "MOVI r0, 100\nMOVI r1, 42\nST [r0+0], r1\nLD r2, [r0+0]\nHALT";
        let result = assemble(code).expect("assembly should succeed");
        assert_eq!(result.len(), 5);
    }

    #[test]
    fn test_assemble_hex_immediate() {
        let code = "MOVI r0, 0xFF";
        let result = assemble(code).expect("assembly should succeed");
        assert_eq!(result.len(), 1);
    }

    #[test]
    fn test_assemble_negative_immediate() {
        let code = "MOVI r0, -100";
        let result = assemble(code).expect("assembly should succeed");
        assert_eq!(result.len(), 1);
    }

    #[test]
    fn test_assemble_all_mnemonics() {
        let code = r#"
            MOVI r0, 10
            MOVI r1, 20
            ADD r0, r1
            SUB r0, r1
            MUL r0, r1
            DIV r0, r1
            MOD r0, r1
            NEG r0
            AND r0, r1
            OR r0, r1
            XOR r0, r1
            NOT r0
            SHL r0, 5
            SHR r0, 3
            SAR r0, 2
            MOV r2, r0
            CMP r0, r1
            JMP END
            END:
            HALT
        "#;
        let result = assemble(code).expect("assembly should succeed");
        assert!(result.len() > 0);
    }

    #[test]
    fn test_syscall_encoding() {
        let code = "SYSCALL 0x01\nHALT";
        let result = assemble(code).expect("assembly should succeed");
        assert_eq!(result.len(), 2);

        // SYSCALL 0x01 should encode as opcode 0x40 with imm 0x01
        let insn = MbcInsn(result[0]);
        assert_eq!(insn.opcode(), op::SYSCALL);
        assert_eq!(insn.imm16(), 0x01);
    }

    #[test]
    fn test_draw_frame_alias() {
        let code = "DRAW_FRAME\nHALT";
        let result = assemble(code).expect("assembly should succeed");
        assert_eq!(result.len(), 2);

        let insn = MbcInsn(result[0]);
        assert_eq!(insn.opcode(), op::SYSCALL);
        assert_eq!(insn.imm16(), 0x01);
    }

    #[test]
    fn test_get_key_alias() {
        let code = "GET_KEY\nHALT";
        let result = assemble(code).expect("assembly should succeed");
        assert_eq!(result.len(), 2);

        let insn = MbcInsn(result[0]);
        assert_eq!(insn.opcode(), op::SYSCALL);
        assert_eq!(insn.imm16(), 0x02);
    }

    #[test]
    fn test_get_ticks_alias() {
        let code = "GET_TICKS\nHALT";
        let result = assemble(code).expect("assembly should succeed");
        assert_eq!(result.len(), 2);

        let insn = MbcInsn(result[0]);
        assert_eq!(insn.opcode(), op::SYSCALL);
        assert_eq!(insn.imm16(), 0x03);
    }

    #[test]
    fn test_sleep_alias() {
        let code = "SLEEP\nHALT";
        let result = assemble(code).expect("assembly should succeed");
        assert_eq!(result.len(), 2);

        let insn = MbcInsn(result[0]);
        assert_eq!(insn.opcode(), op::SYSCALL);
        assert_eq!(insn.imm16(), 0x04);
    }

    #[test]
    fn test_syscall_hex_immediate() {
        let code = "SYSCALL 0xFF\nHALT";
        let result = assemble(code).expect("assembly should succeed");
        assert_eq!(result.len(), 2);

        let insn = MbcInsn(result[0]);
        assert_eq!(insn.opcode(), op::SYSCALL);
        assert_eq!(insn.imm16(), 0xFF);
    }

    #[test]
    fn test_syscall_decimal_immediate() {
        let code = "SYSCALL 42\nHALT";
        let result = assemble(code).expect("assembly should succeed");
        assert_eq!(result.len(), 2);

        let insn = MbcInsn(result[0]);
        assert_eq!(insn.opcode(), op::SYSCALL);
        assert_eq!(insn.imm16(), 42);
    }
}
