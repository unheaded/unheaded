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

    #[error("Line {line}: invalid directive: {message}")]
    InvalidDirective { line: usize, message: String },

    #[error(".org address 0x{address:X} is behind current PC 0x{current_pc:X}")]
    OrgBehindPC { address: u32, current_pc: u32 },
}

/// A single assembled item: either an instruction or a raw data word.
enum AsmItem {
    /// A parsed instruction that may need label resolution.
    Insn(AsmInsn),
    /// A raw 32-bit word (from .data or NOP fill from .org).
    RawWord(u32),
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
    if !(-32768..=65535).contains(&val) {
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

    // Single-operand instructions: NEG, NOT, PUSH, POP
    if matches!(mnemonic.as_str(), "NEG" | "NOT" | "PUSH" | "POP") {
        require_tokens!(1);
        let dst = parse_register(tokens[1]).map_err(|_| AssembleError::Line {
            line: line_num,
            message: format!("Invalid register: '{}'", tokens[1]),
        })?;

        let opcode = match mnemonic.as_str() {
            "NEG" => op::NEG,
            "NOT" => op::NOT,
            "PUSH" => op::PUSH,
            "POP" => op::POP,
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

    if mnemonic == "NOP" {
        return Ok(Some(AsmInsn {
            opcode: op::NOP,
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

    // ADDI instruction: ADDI rN, imm
    if mnemonic == "ADDI" {
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
            opcode: op::ADDI,
            dst,
            src: 0,
            imm,
            label_ref: None,
        }));
    }

    // LOAD_IMM32 instruction: LOAD_IMM32 rN, imm (sets upper 16 bits)
    if mnemonic == "LOAD_IMM32" {
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
            opcode: op::LOAD_IMM32,
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

/// Parse a single byte value (decimal or hex).
fn parse_byte_value(s: &str) -> Result<u8, AssembleError> {
    let s = s.trim().trim_end_matches(',');
    let val: u64 = if s.starts_with("0x") || s.starts_with("0X") {
        u64::from_str_radix(&s[2..], 16).map_err(|_| AssembleError::InvalidInstruction {
            message: format!("Invalid hex byte: '{}'", s),
        })?
    } else {
        s.parse().map_err(|_| AssembleError::InvalidInstruction {
            message: format!("Invalid byte value: '{}'", s),
        })?
    };
    if val > 255 {
        return Err(AssembleError::InvalidInstruction {
            message: format!("Byte value out of range (0-255): {}", val),
        });
    }
    Ok(val as u8)
}

/// Parse a 32-bit word value (decimal or hex).
fn parse_word_value(s: &str) -> Result<u32, AssembleError> {
    let s = s.trim().trim_end_matches(',');
    let val: u64 = if s.starts_with("0x") || s.starts_with("0X") {
        u64::from_str_radix(&s[2..], 16).map_err(|_| AssembleError::InvalidInstruction {
            message: format!("Invalid hex word: '{}'", s),
        })?
    } else {
        s.parse().map_err(|_| AssembleError::InvalidInstruction {
            message: format!("Invalid word value: '{}'", s),
        })?
    };
    if val > 0xFFFF_FFFF {
        return Err(AssembleError::InvalidInstruction {
            message: format!("Word value out of range (0-0xFFFFFFFF): {}", val),
        });
    }
    Ok(val as u32)
}

/// Parse a 16-bit half-word value (decimal or hex).
fn parse_half_value(s: &str) -> Result<u16, AssembleError> {
    let s = s.trim().trim_end_matches(',');
    let val: u64 = if s.starts_with("0x") || s.starts_with("0X") {
        u64::from_str_radix(&s[2..], 16).map_err(|_| AssembleError::InvalidInstruction {
            message: format!("Invalid hex half: '{}'", s),
        })?
    } else {
        s.parse().map_err(|_| AssembleError::InvalidInstruction {
            message: format!("Invalid half value: '{}'", s),
        })?
    };
    if val > 0xFFFF {
        return Err(AssembleError::InvalidInstruction {
            message: format!("Half value out of range (0-0xFFFF): {}", val),
        });
    }
    Ok(val as u16)
}

/// Pack a slice of bytes into 32-bit little-endian words, padding with zeros.
fn bytes_to_words(bytes: &[u8]) -> Vec<u32> {
    let mut words = Vec::new();
    let mut i = 0;
    while i < bytes.len() {
        let mut word_bytes = [0u8; 4];
        for j in 0..4 {
            if i + j < bytes.len() {
                word_bytes[j] = bytes[i + j];
            }
        }
        words.push(u32::from_le_bytes(word_bytes));
        i += 4;
    }
    words
}

/// Assemble MBC assembly source code into bytecode.
///
/// The assembler performs a two-pass process:
/// 1. First pass: Collect label definitions and build item list
/// 2. Second pass: Resolve label references and encode instructions
///
/// ## Directives
///
/// - `.data` — switch to data mode (subsequent `.byte`, `.half`, `.word` embed raw values)
/// - `.text` — switch back to code mode (default)
/// - `.org <address>` — set assembly origin; gaps are filled with NOP (0x00000000)
/// - `.byte <val>, ...` — emit raw bytes (packed into 32-bit words, LE, zero-padded)
/// - `.half <val>, ...` — emit raw 16-bit half-words (packed into 32-bit words, LE)
/// - `.word <val>, ...` — emit raw 32-bit words
///
/// # Errors
/// Returns an AssembleError if parsing fails or labels are undefined.
pub fn assemble(source: &str) -> Result<Vec<u32>, AssembleError> {
    let mut labels: HashMap<String, u32> = HashMap::new();
    let mut items: Vec<AsmItem> = Vec::new();
    let mut pc = 0u32;
    let mut in_data_section = false;

    // Pass 1: Collect labels and items
    for (line_num_0, line) in source.lines().enumerate() {
        let line_num = line_num_0 + 1;
        let stripped = strip_comment(line).trim().to_string();
        let trimmed = stripped.as_str();

        if trimmed.is_empty() {
            continue;
        }

        // Handle directives (lines starting with '.')
        if trimmed.starts_with('.') {
            let tokens: Vec<&str> = trimmed.split_whitespace().collect();
            let directive = tokens[0].to_uppercase();

            // .data — switch to data section
            if directive == ".DATA" {
                in_data_section = true;
                continue;
            }

            // .text — switch back to code section
            if directive == ".TEXT" {
                in_data_section = false;
                continue;
            }

            // .org <address> — set assembly origin
            if directive == ".ORG" {
                if tokens.len() < 2 {
                    return Err(AssembleError::InvalidDirective {
                        line: line_num,
                        message: ".org requires an address argument".to_string(),
                    });
                }
                let addr = parse_word_value(tokens[1]).map_err(|_| {
                    AssembleError::InvalidDirective {
                        line: line_num,
                        message: format!("Invalid .org address: '{}'", tokens[1]),
                    }
                })?;

                if addr < pc {
                    return Err(AssembleError::OrgBehindPC {
                        address: addr,
                        current_pc: pc,
                    });
                }

                // Fill gap with NOP words
                while pc < addr {
                    items.push(AsmItem::RawWord(0x00000000)); // NOP encoding
                    pc += 1;
                }
                continue;
            }

            // .byte <val>, <val>, ...
            if directive == ".BYTE" {
                if tokens.len() < 2 {
                    return Err(AssembleError::InvalidDirective {
                        line: line_num,
                        message: ".byte requires at least one value".to_string(),
                    });
                }
                let mut bytes = Vec::new();
                // Re-split on commas/whitespace to handle "0x48, 0x65, 0x6C"
                let values_str = &trimmed[tokens[0].len()..];
                for part in values_str.split(|c: char| c == ',' || c.is_whitespace()) {
                    let part = part.trim();
                    if part.is_empty() {
                        continue;
                    }
                    let byte_val = parse_byte_value(part).map_err(|_| {
                        AssembleError::InvalidDirective {
                            line: line_num,
                            message: format!("Invalid .byte value: '{}'", part),
                        }
                    })?;
                    bytes.push(byte_val);
                }
                let words = bytes_to_words(&bytes);
                for w in words {
                    items.push(AsmItem::RawWord(w));
                    pc += 1;
                }
                continue;
            }

            // .half <val>, <val>, ...
            if directive == ".HALF" {
                if tokens.len() < 2 {
                    return Err(AssembleError::InvalidDirective {
                        line: line_num,
                        message: ".half requires at least one value".to_string(),
                    });
                }
                let mut halves = Vec::new();
                let values_str = &trimmed[tokens[0].len()..];
                for part in values_str.split(|c: char| c == ',' || c.is_whitespace()) {
                    let part = part.trim();
                    if part.is_empty() {
                        continue;
                    }
                    let half_val = parse_half_value(part).map_err(|_| {
                        AssembleError::InvalidDirective {
                            line: line_num,
                            message: format!("Invalid .half value: '{}'", part),
                        }
                    })?;
                    halves.push(half_val);
                }
                // Pack halves into words (2 halves per word, LE)
                let mut bytes = Vec::new();
                for h in halves {
                    bytes.push((h & 0xFF) as u8);
                    bytes.push(((h >> 8) & 0xFF) as u8);
                }
                let words = bytes_to_words(&bytes);
                for w in words {
                    items.push(AsmItem::RawWord(w));
                    pc += 1;
                }
                continue;
            }

            // .word <val>, <val>, ...
            if directive == ".WORD" {
                if tokens.len() < 2 {
                    return Err(AssembleError::InvalidDirective {
                        line: line_num,
                        message: ".word requires at least one value".to_string(),
                    });
                }
                let values_str = &trimmed[tokens[0].len()..];
                for part in values_str.split(|c: char| c == ',' || c.is_whitespace()) {
                    let part = part.trim();
                    if part.is_empty() {
                        continue;
                    }
                    let word_val = parse_word_value(part).map_err(|_| {
                        AssembleError::InvalidDirective {
                            line: line_num,
                            message: format!("Invalid .word value: '{}'", part),
                        }
                    })?;
                    items.push(AsmItem::RawWord(word_val));
                    pc += 1;
                }
                continue;
            }

            // Unknown directive
            return Err(AssembleError::InvalidDirective {
                line: line_num,
                message: format!("Unknown directive: '{}'", tokens[0]),
            });
        }

        // Check for label definition (ends with ':')
        if trimmed.ends_with(':') && !trimmed.starts_with(';') && !trimmed.starts_with('#') {
            let label_name = trimmed.trim_end_matches(':').trim();

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

        // In data section, only directives and labels are allowed
        if in_data_section {
            return Err(AssembleError::Line {
                line: line_num,
                message: format!("Instructions not allowed in .data section: '{}'", trimmed),
            });
        }

        if let Some(insn) = parse_instruction(trimmed, line_num)? {
            items.push(AsmItem::Insn(insn));
            pc += 1;
        }
    }

    // Pass 2: Encode items with resolved labels
    let mut output = Vec::new();
    let mut insn_index = 0u32; // Track position for label resolution

    for item in &items {
        match item {
            AsmItem::RawWord(word) => {
                output.push(*word);
                insn_index += 1;
            }
            AsmItem::Insn(insn) => {
                // Check if this is a branch/jump or CALL instruction with a label ref.
                // These need special 24-bit encoding: [opcode:8][offset_or_target:24]
                // where the 24-bit value spans dst(4)+src(4)+imm16(16).
                let is_branch = matches!(
                    insn.opcode,
                    op::JMP | op::JZ | op::JNZ | op::JN | op::JP | op::JC | op::JNC
                );
                let is_call = insn.opcode == op::CALL;

                if let Some(ref label) = insn.label_ref {
                    let label_pc = labels
                        .get(label)
                        .ok_or_else(|| AssembleError::UndefinedLabel {
                            label: label.clone(),
                        })?;

                    if is_call {
                        // CALL uses 24-bit absolute addressing: target PC in lower 24 bits
                        let target = *label_pc;
                        if target > 0x00FF_FFFF {
                            return Err(AssembleError::ImmediateOutOfRange {
                                value: target as i32,
                            });
                        }
                        let word = ((insn.opcode as u32) << 24) | (target & 0x00FF_FFFF);
                        output.push(word);
                    } else if is_branch {
                        // Branches use 24-bit signed relative offset: target - (current + 1)
                        let current_pc = insn_index;
                        let relative_offset = (*label_pc as i32) - (current_pc as i32 + 1);

                        // 24-bit signed range: -8388608 to +8388607
                        if !(-8_388_608..=8_388_607).contains(&relative_offset) {
                            return Err(AssembleError::ImmediateOutOfRange {
                                value: relative_offset,
                            });
                        }

                        let off24 = (relative_offset as u32) & 0x00FF_FFFF;
                        let word = ((insn.opcode as u32) << 24) | off24;
                        output.push(word);
                    } else {
                        // Non-branch label ref (shouldn't happen, but handle gracefully)
                        let current_pc = insn_index;
                        let relative_offset = (*label_pc as i32) - (current_pc as i32 + 1);
                        if !(-32768..=32767).contains(&relative_offset) {
                            return Err(AssembleError::ImmediateOutOfRange {
                                value: relative_offset,
                            });
                        }
                        let word = MbcInsn::encode(insn.opcode, insn.dst, insn.src, relative_offset as u16);
                        output.push(word.0);
                    }
                } else {
                    // No label ref: encode normally
                    let word = MbcInsn::encode(insn.opcode, insn.dst, insn.src, insn.imm as u16);
                    output.push(word.0);
                }
                insn_index += 1;
            }
        }
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

    #[test]
    fn test_nop_mnemonic() {
        let code = "NOP\nHALT";
        let result = assemble(code).expect("assembly should succeed");
        assert_eq!(result.len(), 2);

        let insn = MbcInsn(result[0]);
        assert_eq!(insn.opcode(), op::NOP);
    }

    #[test]
    fn test_push_mnemonic() {
        let code = "PUSH r3\nHALT";
        let result = assemble(code).expect("assembly should succeed");
        assert_eq!(result.len(), 2);

        let insn = MbcInsn(result[0]);
        assert_eq!(insn.opcode(), op::PUSH);
        assert_eq!(insn.dst(), 3);
    }

    #[test]
    fn test_pop_mnemonic() {
        let code = "POP r5\nHALT";
        let result = assemble(code).expect("assembly should succeed");
        assert_eq!(result.len(), 2);

        let insn = MbcInsn(result[0]);
        assert_eq!(insn.opcode(), op::POP);
        assert_eq!(insn.dst(), 5);
    }

    #[test]
    fn test_addi_mnemonic() {
        let code = "ADDI r0, 42\nHALT";
        let result = assemble(code).expect("assembly should succeed");
        assert_eq!(result.len(), 2);

        let insn = MbcInsn(result[0]);
        assert_eq!(insn.opcode(), op::ADDI);
        assert_eq!(insn.dst(), 0);
        assert_eq!(insn.imm16(), 42);
    }

    #[test]
    fn test_addi_negative_mnemonic() {
        let code = "ADDI r1, -10\nHALT";
        let result = assemble(code).expect("assembly should succeed");
        assert_eq!(result.len(), 2);

        let insn = MbcInsn(result[0]);
        assert_eq!(insn.opcode(), op::ADDI);
        assert_eq!(insn.dst(), 1);
        assert_eq!(insn.imm16_signed(), -10);
    }

    #[test]
    fn test_load_imm32_mnemonic() {
        let code = "LOAD_IMM32 r2, 0xABCD\nHALT";
        let result = assemble(code).expect("assembly should succeed");
        assert_eq!(result.len(), 2);

        let insn = MbcInsn(result[0]);
        assert_eq!(insn.opcode(), op::LOAD_IMM32);
        assert_eq!(insn.dst(), 2);
        assert_eq!(insn.imm16(), 0xABCD);
    }

    // ---- .data directive tests (Step 111) ----

    #[test]
    fn test_data_byte_directive() {
        let code = ".data\n.byte 0x48, 0x65, 0x6C\n.text\nHALT";
        let result = assemble(code).expect("assembly should succeed");
        // 3 bytes -> 1 word (padded) + HALT = 2 words
        assert_eq!(result.len(), 2);
        // First word: 0x48, 0x65, 0x6C, 0x00 in LE = 0x006C6548
        assert_eq!(result[0], 0x006C6548);
    }

    #[test]
    fn test_data_byte_multiple_words() {
        let code = ".byte 0x01, 0x02, 0x03, 0x04, 0x05";
        let result = assemble(code).expect("assembly should succeed");
        // 5 bytes -> 2 words (4 bytes + 1 byte padded)
        assert_eq!(result.len(), 2);
        assert_eq!(result[0], 0x04030201); // bytes 1-4 LE
        assert_eq!(result[1], 0x00000005); // byte 5 + padding
    }

    #[test]
    fn test_data_word_directive() {
        let code = ".word 0xDEADBEEF, 0xCAFEBABE";
        let result = assemble(code).expect("assembly should succeed");
        assert_eq!(result.len(), 2);
        assert_eq!(result[0], 0xDEADBEEF);
        assert_eq!(result[1], 0xCAFEBABE);
    }

    #[test]
    fn test_data_half_directive() {
        let code = ".half 0x1234, 0x5678";
        let result = assemble(code).expect("assembly should succeed");
        // 2 halves = 4 bytes = 1 word
        assert_eq!(result.len(), 1);
        // LE: 0x34, 0x12, 0x78, 0x56 -> 0x56781234
        assert_eq!(result[0], 0x56781234);
    }

    #[test]
    fn test_data_section_with_label() {
        let code = r#"
            JMP code_start
            data_label:
            .data
            .word 0xDEADBEEF
            .text
            code_start:
            MOVI r0, 42
            HALT
        "#;
        let result = assemble(code).expect("assembly should succeed");
        // JMP + data word + MOVI + HALT = 4 words
        assert_eq!(result.len(), 4);
        assert_eq!(result[1], 0xDEADBEEF);
    }

    #[test]
    fn test_data_section_no_instructions() {
        let code = ".data\nMOVI r0, 42";
        let result = assemble(code);
        assert!(result.is_err(), "instructions should not be allowed in .data section");
    }

    // ---- .org directive tests (Step 112) ----

    #[test]
    fn test_org_directive() {
        let code = "MOVI r0, 1\n.org 4\nMOVI r1, 2\nHALT";
        let result = assemble(code).expect("assembly should succeed");
        // MOVI at 0, NOP at 1, NOP at 2, NOP at 3, MOVI at 4, HALT at 5
        assert_eq!(result.len(), 6);
        // Words at index 1, 2, 3 should be NOP (0x00000000)
        assert_eq!(result[1], 0x00000000);
        assert_eq!(result[2], 0x00000000);
        assert_eq!(result[3], 0x00000000);
    }

    #[test]
    fn test_org_at_current_pc() {
        // .org at current PC should be a no-op
        let code = "MOVI r0, 1\n.org 1\nHALT";
        let result = assemble(code).expect("assembly should succeed");
        assert_eq!(result.len(), 2);
    }

    #[test]
    fn test_org_behind_pc_error() {
        let code = "MOVI r0, 1\nMOVI r1, 2\n.org 0";
        let result = assemble(code);
        assert!(result.is_err(), ".org behind current PC should fail");
    }

    #[test]
    fn test_org_with_label() {
        let code = r#"
            .org 3
            entry:
            MOVI r0, 42
            JMP entry
            HALT
        "#;
        let result = assemble(code).expect("assembly should succeed");
        // 3 NOP words + MOVI + JMP + HALT = 6 words
        assert_eq!(result.len(), 6);
        assert_eq!(result[0], 0x00000000); // NOP fill
        assert_eq!(result[1], 0x00000000);
        assert_eq!(result[2], 0x00000000);
    }

    #[test]
    fn test_org_hex_address() {
        let code = ".org 0x10\nHALT";
        let result = assemble(code).expect("assembly should succeed");
        // 16 NOP words + HALT = 17 words
        assert_eq!(result.len(), 17);
    }

    #[test]
    fn test_data_and_org_combined() {
        let code = r#"
            .org 2
            .data
            .byte 0xAA, 0xBB, 0xCC, 0xDD
            .text
            HALT
        "#;
        let result = assemble(code).expect("assembly should succeed");
        // 2 NOP (from .org) + 1 data word + HALT = 4 words
        assert_eq!(result.len(), 4);
        assert_eq!(result[0], 0x00000000);
        assert_eq!(result[1], 0x00000000);
        assert_eq!(result[2], 0xDDCCBBAA);
    }

    #[test]
    fn test_byte_directive_decimal() {
        let code = ".byte 72, 101, 108";
        let result = assemble(code).expect("assembly should succeed");
        assert_eq!(result.len(), 1);
        // 72=0x48, 101=0x65, 108=0x6C -> LE word
        assert_eq!(result[0], 0x006C6548);
    }

    #[test]
    fn test_word_directive_single() {
        let code = ".word 12345";
        let result = assemble(code).expect("assembly should succeed");
        assert_eq!(result.len(), 1);
        assert_eq!(result[0], 12345);
    }
}
