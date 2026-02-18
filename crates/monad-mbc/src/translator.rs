//\! RV32I to MBC translator: converts RISC-V 32-bit instructions to MBC bytecode.
//\!
//\! This module translates RV32I binary instructions to MBC format.
//\! Register mapping:
//\! - x0 (zero) → r0 (special handling: reads return 0, writes are discarded + r0 reset to 0)
//\! - x1 (ra) → r14
//\! - x2 (sp) → r15
//\! - x3-x15 → r1-r13
//\! - x16-x31 → unsupported

use monad_common::{mbc_opcodes as op, MbcInsn};
use thiserror::Error;

/// Errors that can occur during RV32I to MBC translation.
#[derive(Error, Debug, Clone)]
pub enum TranslateError {
    #[error("Unsupported register: x{register} (only x0-x15 supported)")]
    UnsupportedRegister { register: u8 },

    #[error("Unsupported RV32I instruction at PC 0x{pc:x}: opcode=0x{opcode:02x} funct3={funct3} funct7=0x{funct7:02x}")]
    UnsupportedInstruction {
        pc: u32,
        opcode: u8,
        funct3: u8,
        funct7: u8,
    },

    #[error("Immediate out of range: {value}")]
    ImmediateOutOfRange { value: i32 },

    #[error("Unsupported feature: {message}")]
    UnsupportedFeature { message: String },
}

/// RV32I to MBC translator state.
pub struct Translator {
    /// Accumulated output instructions
    pub output: Vec<u32>,
    /// Accumulated errors (non-fatal, translation continues)
    pub errors: Vec<TranslateError>,
}

impl Translator {
    /// Create a new translator with empty output and error lists.
    pub fn new() -> Self {
        Translator {
            output: Vec::new(),
            errors: Vec::new(),
        }
    }

    /// Translate a complete RV32I program to MBC bytecode.
    ///
    /// # Arguments
    /// * `rv32i_words` - Slice of RV32I instruction words (32-bit little-endian)
    ///
    /// # Returns
    /// * `Ok(Vec<u32>)` if translation succeeded
    /// * `Err(Vec<TranslateError>)` if any errors occurred
    pub fn translate_program(rv32i_words: &[u32]) -> Result<Vec<u32>, Vec<TranslateError>> {
        let mut translator = Translator::new();

        for (idx, word) in rv32i_words.iter().enumerate() {
            let pc = (idx * 4) as u32;
            if let Err(e) = translator.translate_insn(*word, pc) {
                translator.errors.push(e);
            }
        }

        if translator.errors.is_empty() {
            Ok(translator.output)
        } else {
            Err(translator.errors)
        }
    }

    /// Map an RV32I register (x0-x31) to an MBC register (r0-r15).
    fn map_register(&self, xreg: u8) -> Result<u8, TranslateError> {
        match xreg {
            0 => Ok(0),   // x0 (zero) → r0
            1 => Ok(14),  // x1 (ra) → r14
            2 => Ok(15),  // x2 (sp) → r15
            3 => Ok(1),   // x3 (gp) → r1
            4 => Ok(2),   // x4 (tp) → r2
            5 => Ok(3),   // x5 (t0) → r3
            6 => Ok(4),   // x6 (t1) → r4
            7 => Ok(5),   // x7 (t2) → r5
            8 => Ok(6),   // x8 (s0) → r6
            9 => Ok(7),   // x9 (s1) → r7
            10 => Ok(8),  // x10 (a0) → r8
            11 => Ok(9),  // x11 (a1) → r9
            12 => Ok(10), // x12 (a2) → r10
            13 => Ok(11), // x13 (a3) → r11
            14 => Ok(12), // x14 (a4) → r12
            15 => Ok(13), // x15 (a5) → r13
            _ => Err(TranslateError::UnsupportedRegister { register: xreg }),
        }
    }

    /// Emit an MBC instruction to the output.
    fn emit(&mut self, opcode: u8, dst: u8, src: u8, imm: u16) {
        let insn = MbcInsn::encode(opcode, dst, src, imm);
        self.output.push(insn.0);
    }

    /// Translate a single RV32I instruction to MBC bytecode.
    ///
    /// # Arguments
    /// * `insn` - The 32-bit RV32I instruction word
    /// * `pc` - Program counter (word-aligned, in bytes)
    pub fn translate_insn(&mut self, insn: u32, pc: u32) -> Result<(), TranslateError> {
        let opcode = insn & 0x7F;
        let rd = ((insn >> 7) & 0x1F) as u8;
        let rs1 = ((insn >> 15) & 0x1F) as u8;
        let rs2 = ((insn >> 20) & 0x1F) as u8;
        let funct3 = ((insn >> 12) & 0x07) as u8;
        let funct7 = ((insn >> 25) & 0x7F) as u8;

        match opcode {
            // OP (R-type): ADD, SUB, AND, OR, XOR, SLL, SRL, SRA
            0x33 => {
                let mbc_rd = self.map_register(rd)?;
                let mbc_rs1 = self.map_register(rs1)?;
                let mbc_rs2 = self.map_register(rs2)?;

                match (funct3, funct7) {
                    (0, 0) => {
                        // ADD: rd = rs1 + rs2
                        if rs1 == 0 {
                            self.emit(op::MOVI, mbc_rd, 0, 0);
                        }
                        if rs2 == 0 {
                            self.emit(op::MOVI, mbc_rd, 0, 0);
                        }
                        // rd = rs1; rd += rs2
                        self.emit(op::MOV, mbc_rd, mbc_rs1, 0);
                        self.emit(op::ADD, mbc_rd, mbc_rs2);

                        if rd == 0 {
                            self.emit(op::MOVI, 0, 0, 0);
                        }
                    }
                    (0, 32) => {
                        // SUB: rd = rs1 - rs2
                        if rs1 == 0 {
                            self.emit(op::MOVI, mbc_rd, 0, 0);
                        }
                        if rs2 == 0 {
                            self.emit(op::MOVI, mbc_rd, 0, 0);
                        }
                        // rd = rs1; rd -= rs2
                        self.emit(op::MOV, mbc_rd, mbc_rs1, 0);
                        self.emit(op::SUB, mbc_rd, mbc_rs2);

                        if rd == 0 {
                            self.emit(op::MOVI, 0, 0, 0);
                        }
                    }
                    (7, 0) => {
                        // AND: rd = rs1 & rs2
                        if rs1 == 0 {
                            self.emit(op::MOVI, mbc_rd, 0, 0);
                        }
                        if rs2 == 0 {
                            self.emit(op::MOVI, mbc_rd, 0, 0);
                        }
                        self.emit(op::MOV, mbc_rd, mbc_rs1, 0);
                        self.emit(op::AND, mbc_rd, mbc_rs2);

                        if rd == 0 {
                            self.emit(op::MOVI, 0, 0, 0);
                        }
                    }
                    (6, 0) => {
                        // OR: rd = rs1 | rs2
                        if rs1 == 0 {
                            self.emit(op::MOVI, mbc_rd, 0, 0);
                        }
                        if rs2 == 0 {
                            self.emit(op::MOVI, mbc_rd, 0, 0);
                        }
                        self.emit(op::MOV, mbc_rd, mbc_rs1, 0);
                        self.emit(op::OR, mbc_rd, mbc_rs2);

                        if rd == 0 {
                            self.emit(op::MOVI, 0, 0, 0);
                        }
                    }
                    (4, 0) => {
                        // XOR: rd = rs1 ^ rs2
                        if rs1 == 0 {
                            self.emit(op::MOVI, mbc_rd, 0, 0);
                        }
                        if rs2 == 0 {
                            self.emit(op::MOVI, mbc_rd, 0, 0);
                        }
                        self.emit(op::MOV, mbc_rd, mbc_rs1, 0);
                        self.emit(op::XOR, mbc_rd, mbc_rs2);

                        if rd == 0 {
                            self.emit(op::MOVI, 0, 0, 0);
                        }
                    }
                    (1, 0) => {
                        // SLL: rd = rs1 << rs2
                        if rs2 \!= 0 {
                            if rs1 == 0 {
                                self.emit(op::MOVI, mbc_rd, 0, 0);
                            }
                            self.emit(op::MOV, mbc_rd, mbc_rs1, 0);
                            // MBC only supports immediate shift, not register shift
                            return Err(TranslateError::UnsupportedFeature {
                                message: "register-based SLL not supported".to_string(),
                            });
                        }
                        if rd == 0 {
                            self.emit(op::MOVI, 0, 0, 0);
                        }
                    }
                    (5, 0) => {
                        // SRL: rd = rs1 >> rs2 (logical)
                        if rs2 \!= 0 {
                            if rs1 == 0 {
                                self.emit(op::MOVI, mbc_rd, 0, 0);
                            }
                            self.emit(op::MOV, mbc_rd, mbc_rs1, 0);
                            return Err(TranslateError::UnsupportedFeature {
                                message: "register-based SRL not supported".to_string(),
                            });
                        }
                        if rd == 0 {
                            self.emit(op::MOVI, 0, 0, 0);
                        }
                    }
                    (5, 32) => {
                        // SRA: rd = rs1 >> rs2 (arithmetic)
                        if rs2 \!= 0 {
                            if rs1 == 0 {
                                self.emit(op::MOVI, mbc_rd, 0, 0);
                            }
                            self.emit(op::MOV, mbc_rd, mbc_rs1, 0);
                            return Err(TranslateError::UnsupportedFeature {
                                message: "register-based SRA not supported".to_string(),
                            });
                        }
                        if rd == 0 {
                            self.emit(op::MOVI, 0, 0, 0);
                        }
                    }
                    _ => {
                        return Err(TranslateError::UnsupportedInstruction {
                            pc,
                            opcode: opcode as u8,
                            funct3,
                            funct7,
                        })
                    }
                }
            }

            // OP-IMM (I-type): ADDI, ANDI, ORI, XORI, SLLI, SRLI, SRAI
            0x13 => {
                let mbc_rd = self.map_register(rd)?;
                let mbc_rs1 = self.map_register(rs1)?;
                let imm12 = (insn >> 20) as i16;

                match funct3 {
                    0 => {
                        // ADDI: rd = rs1 + imm12
                        if rs1 == 0 {
                            self.emit(op::MOVI, mbc_rd, 0, 0);
                        }
                        self.emit(op::MOV, mbc_rd, mbc_rs1, 0);

                        if imm12 >= 0 {
                            self.emit(op::MOVI, 0, 0, imm12 as u16);
                            self.emit(op::ADD, mbc_rd, 0);
                        } else {
                            let abs_imm = (-imm12) as u16;
                            self.emit(op::MOVI, 0, 0, abs_imm);
                            self.emit(op::NEG, 0);
                            self.emit(op::ADD, mbc_rd, 0);
                        }
                        self.emit(op::MOVI, 0, 0, 0);

                        if rd == 0 {
                            self.emit(op::MOVI, 0, 0, 0);
                        }
                    }
                    7 => {
                        // ANDI: rd = rs1 & imm12
                        if rs1 == 0 {
                            self.emit(op::MOVI, mbc_rd, 0, 0);
                        } else {
                            self.emit(op::MOV, mbc_rd, mbc_rs1, 0);
                            self.emit(op::MOVI, 0, 0, imm12 as u16);
                            self.emit(op::AND, mbc_rd, 0);
                            self.emit(op::MOVI, 0, 0, 0);
                        }

                        if rd == 0 {
                            self.emit(op::MOVI, 0, 0, 0);
                        }
                    }
                    6 => {
                        // ORI: rd = rs1 | imm12
                        if rs1 == 0 {
                            self.emit(op::MOVI, mbc_rd, 0, 0, imm12 as u16);
                        } else {
                            self.emit(op::MOV, mbc_rd, mbc_rs1, 0);
                            self.emit(op::MOVI, 0, 0, imm12 as u16);
                            self.emit(op::OR, mbc_rd, 0);
                            self.emit(op::MOVI, 0, 0, 0);
                        }

                        if rd == 0 {
                            self.emit(op::MOVI, 0, 0, 0);
                        }
                    }
                    4 => {
                        // XORI: rd = rs1 ^ imm12
                        if rs1 == 0 {
                            self.emit(op::MOVI, mbc_rd, 0, 0, imm12 as u16);
                        } else {
                            self.emit(op::MOV, mbc_rd, mbc_rs1, 0);
                            self.emit(op::MOVI, 0, 0, imm12 as u16);
                            self.emit(op::XOR, mbc_rd, 0);
                            self.emit(op::MOVI, 0, 0, 0);
                        }

                        if rd == 0 {
                            self.emit(op::MOVI, 0, 0, 0);
                        }
                    }
                    1 => {
                        // SLLI: rd = rs1 << imm12[4:0]
                        let shamt = (imm12 & 0x1F) as u16;
                        if rs1 == 0 {
                            self.emit(op::MOVI, mbc_rd, 0, 0);
                        } else {
                            self.emit(op::MOV, mbc_rd, mbc_rs1, 0);
                            self.emit(op::SHL, mbc_rd, shamt);
                        }

                        if rd == 0 {
                            self.emit(op::MOVI, 0, 0, 0);
                        }
                    }
                    5 => {
                        // SRLI or SRAI based on funct7[5]
                        let shamt = (imm12 & 0x1F) as u16;
                        if (funct7 >> 5) & 1 == 0 {
                            // SRLI: rd = rs1 >> imm12[4:0] (logical)
                            if rs1 == 0 {
                                self.emit(op::MOVI, mbc_rd, 0, 0);
                            } else {
                                self.emit(op::MOV, mbc_rd, mbc_rs1, 0);
                                self.emit(op::SHR, mbc_rd, shamt);
                            }
                        } else {
                            // SRAI: rd = rs1 >> imm12[4:0] (arithmetic)
                            if rs1 == 0 {
                                self.emit(op::MOVI, mbc_rd, 0, 0);
                            } else {
                                self.emit(op::MOV, mbc_rd, mbc_rs1, 0);
                                self.emit(op::SAR, mbc_rd, shamt);
                            }
                        }

                        if rd == 0 {
                            self.emit(op::MOVI, 0, 0, 0);
                        }
                    }
                    _ => {
                        return Err(TranslateError::UnsupportedInstruction {
                            pc,
                            opcode: opcode as u8,
                            funct3,
                            funct7,
                        })
                    }
                }
            }

            // LOAD (I-type): LW, LH, LB, LHU, LBU
            0x03 => {
                let mbc_rd = self.map_register(rd)?;
                let mbc_rs1 = self.map_register(rs1)?;
                let imm12 = (insn >> 20) as i16;

                match funct3 {
                    2 => {
                        // LW: rd = mem[rs1 + imm12] (32-bit load)
                        if rs1 == 0 {
                            self.emit(op::MOVI, mbc_rd, 0, 0);
                        }
                        self.emit(op::LD, mbc_rd, mbc_rs1, imm12 as u16);

                        if rd == 0 {
                            self.emit(op::MOVI, 0, 0, 0);
                        }
                    }
                    1 => {
                        // LH: rd = mem[rs1 + imm12] sign-extended (16-bit load)
                        if rs1 == 0 {
                            self.emit(op::MOVI, mbc_rd, 0, 0);
                        }
                        self.emit(op::LDH, mbc_rd, mbc_rs1, imm12 as u16);

                        if rd == 0 {
                            self.emit(op::MOVI, 0, 0, 0);
                        }
                    }
                    0 => {
                        // LB: rd = mem[rs1 + imm12] sign-extended (8-bit load)
                        if rs1 == 0 {
                            self.emit(op::MOVI, mbc_rd, 0, 0);
                        }
                        self.emit(op::LDB, mbc_rd, mbc_rs1, imm12 as u16);

                        if rd == 0 {
                            self.emit(op::MOVI, 0, 0, 0);
                        }
                    }
                    _ => {
                        return Err(TranslateError::UnsupportedInstruction {
                            pc,
                            opcode: opcode as u8,
                            funct3,
                            funct7,
                        })
                    }
                }
            }

            // STORE (S-type): SW, SH, SB
            0x23 => {
                let mbc_rs1 = self.map_register(rs1)?;
                let mbc_rs2 = self.map_register(rs2)?;
                let imm_s = (((insn >> 25) << 5) | ((insn >> 7) & 0x1F)) as i32 as i16;

                if rs2 == 0 {
                    self.emit(op::MOVI, mbc_rs2, 0, 0);
                }

                match funct3 {
                    2 => {
                        // SW: mem[rs1 + imm_s] = rs2 (32-bit store)
                        if rs1 == 0 {
                            self.emit(op::MOVI, mbc_rs1, 0, imm_s as u16);
                        }
                        self.emit(op::ST, mbc_rs1, mbc_rs2, imm_s as u16);
                    }
                    1 => {
                        // SH: mem[rs1 + imm_s] = rs2 (16-bit store)
                        if rs1 == 0 {
                            self.emit(op::MOVI, mbc_rs1, 0, imm_s as u16);
                        }
                        self.emit(op::STH, mbc_rs1, mbc_rs2, imm_s as u16);
                    }
                    0 => {
                        // SB: mem[rs1 + imm_s] = rs2 (8-bit store)
                        if rs1 == 0 {
                            self.emit(op::MOVI, mbc_rs1, 0, imm_s as u16);
                        }
                        self.emit(op::STB, mbc_rs1, mbc_rs2, imm_s as u16);
                    }
                    _ => {
                        return Err(TranslateError::UnsupportedInstruction {
                            pc,
                            opcode: opcode as u8,
                            funct3,
                            funct7,
                        })
                    }
                }
            }

            // BRANCH (B-type): BEQ, BNE, BLT, BGE, BLTU, BGEU
            0x63 => {
                let mbc_rs1 = self.map_register(rs1)?;
                let mbc_rs2 = self.map_register(rs2)?;

                if rs1 == 0 {
                    self.emit(op::MOVI, mbc_rs1, 0, 0);
                }
                if rs2 == 0 {
                    self.emit(op::MOVI, mbc_rs2, 0, 0);
                }

                // Compare
                self.emit(op::CMP, mbc_rs1, mbc_rs2);

                // Compute branch offset: imm_b = (insn[31] | insn[7] | insn[30:25] | insn[11:8]) << 1
                let imm_b = ((insn >> 31) << 12)
                    | ((insn >> 7) << 11)
                    | ((insn >> 25) << 5)
                    | ((insn >> 8) << 1);
                let imm_b = if imm_b & 0x1000 \!= 0 {
                    (imm_b | 0xFFFF_E000) as i32
                } else {
                    imm_b as i32
                };
                let imm_b = (imm_b >> 2) as i16; // Convert from byte offset to word offset

                match funct3 {
                    0 => {
                        // BEQ: if rs1 == rs2, branch
                        self.emit(op::JZ, 0, 0, imm_b as u16);
                    }
                    1 => {
                        // BNE: if rs1 \!= rs2, branch
                        self.emit(op::JNZ, 0, 0, imm_b as u16);
                    }
                    _ => {
                        return Err(TranslateError::UnsupportedInstruction {
                            pc,
                            opcode: opcode as u8,
                            funct3,
                            funct7,
                        })
                    }
                }
            }

            // JAL (J-type): unconditional jump and link
            0x6F => {
                let mbc_rd = self.map_register(rd)?;

                // Compute offset: imm_j = (insn[31] | insn[19:12] | insn[20] | insn[30:21]) << 1
                let imm_j = ((insn >> 31) << 20)
                    | ((insn >> 12) << 12)
                    | ((insn >> 20) << 11)
                    | ((insn >> 21) << 1);
                let imm_j = if imm_j & 0x100000 \!= 0 {
                    (imm_j | 0xFFF0_0000) as i32
                } else {
                    imm_j as i32
                };
                let imm_j = (imm_j >> 2) as i16; // Convert to word offset

                // JAL: rd = PC + 4; PC += offset
                self.emit(op::CALL, 0, 0, imm_j as u16);

                if rd == 0 {
                    self.emit(op::MOVI, 0, 0, 0);
                }
            }

            // JALR (I-type): register-based jump and link
            0x67 => {
                let mbc_rs1 = self.map_register(rs1)?;
                let imm12 = (insn >> 20) as i16;

                if rs1 == 0 {
                    self.emit(op::MOVI, mbc_rs1, 0, 0);
                }

                // JALR requires special handling (indirect jump)
                // MBC doesn't directly support this; would need register-indirect jump
                return Err(TranslateError::UnsupportedFeature {
                    message: "JALR (indirect jump) not supported".to_string(),
                });
            }

            // LUI (U-type): load upper immediate
            0x37 => {
                let mbc_rd = self.map_register(rd)?;
                let imm_u = ((insn >> 12) & 0xFFFFF) as u16;
                self.emit(op::MOVI, mbc_rd, 0, imm_u);

                if rd == 0 {
                    self.emit(op::MOVI, 0, 0, 0);
                }
            }

            // AUIPC (U-type): add upper immediate to PC
            0x17 => {
                return Err(TranslateError::UnsupportedFeature {
                    message: "AUIPC (add upper immediate to PC) not supported".to_string(),
                });
            }

            // SYSTEM (I-type): ECALL, EBREAK
            0x73 => {
                match insn {
                    0x00000073 => {
                        // ECALL
                        self.emit(op::SYSCALL, 0, 0, 0);
                    }
                    0x00100073 => {
                        // EBREAK
                        self.emit(op::HALT, 0, 0, 0);
                    }
                    _ => {
                        return Err(TranslateError::UnsupportedInstruction {
                            pc,
                            opcode: opcode as u8,
                            funct3,
                            funct7,
                        })
                    }
                }
            }

            _ => {
                return Err(TranslateError::UnsupportedInstruction {
                    pc,
                    opcode: opcode as u8,
                    funct3,
                    funct7,
                })
            }
        }

        Ok(())
    }
}

impl Default for Translator {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_translate_simple_movi() {
        // LUI r0, 0x12345 (load upper immediate into x10/a0 → r8)
        // Opcode 0x37, rd=10, imm20=0x12345
        let insn = (0x12345 << 12) | (10 << 7) | 0x37;
        let result = Translator::translate_program(&[insn]);
        assert\!(result.is_ok());
    }

    #[test]
    fn test_translate_add() {
        // ADD x1, x2, x3 (x2 → r15, x3 → r1, x1 → r14)
        // R-type: funct7=0, rs2=3, rs1=2, funct3=0, rd=1, opcode=0x33
        let insn = (0 << 25) | (3 << 20) | (2 << 15) | (0 << 12) | (1 << 7) | 0x33;
        let result = Translator::translate_program(&[insn]);
        assert\!(result.is_ok());
    }

    #[test]
    fn test_translate_addi() {
        // ADDI x1, x2, 100
        // I-type: imm12=100, rs1=2, funct3=0, rd=1, opcode=0x13
        let insn = (100 << 20) | (2 << 15) | (0 << 12) | (1 << 7) | 0x13;
        let result = Translator::translate_program(&[insn]);
        assert\!(result.is_ok());
    }

    #[test]
    fn test_translate_addi_negative() {
        // ADDI x1, x2, -50
        // I-type: imm12=-50 (sign-extended), rs1=2, funct3=0, rd=1, opcode=0x13
        let imm = ((-50i32) & 0xFFF) as u32;
        let insn = (imm << 20) | (2 << 15) | (0 << 12) | (1 << 7) | 0x13;
        let result = Translator::translate_program(&[insn]);
        assert\!(result.is_ok());
    }

    #[test]
    fn test_translate_lw() {
        // LW x1, 0(x2)
        // I-type: imm12=0, rs1=2, funct3=2, rd=1, opcode=0x03
        let insn = (0 << 20) | (2 << 15) | (2 << 12) | (1 << 7) | 0x03;
        let result = Translator::translate_program(&[insn]);
        assert\!(result.is_ok());
    }

    #[test]
    fn test_translate_sw() {
        // SW x3, 0(x2)
        // S-type: imm[11:5]=0, rs2=3, rs1=2, funct3=2, imm[4:0]=0, opcode=0x23
        let insn = (0 << 25) | (3 << 20) | (2 << 15) | (2 << 12) | (0 << 7) | 0x23;
        let result = Translator::translate_program(&[insn]);
        assert\!(result.is_ok());
    }

    #[test]
    fn test_translate_register_out_of_range() {
        // Try to access x20 (unsupported)
        let insn = (0 << 25) | (20 << 20) | (2 << 15) | (0 << 12) | (1 << 7) | 0x33;
        let result = Translator::translate_program(&[insn]);
        assert\!(result.is_err());
    }

    #[test]
    fn test_translate_beq() {
        // BEQ x1, x2, 0 (forward by 0)
        // B-type: imm[12|10:5|4:1|11]
        let insn = (0 << 25) | (2 << 20) | (1 << 15) | (0 << 12) | (0 << 7) | 0x63;
        let result = Translator::translate_program(&[insn]);
        assert\!(result.is_ok());
    }

    #[test]
    fn test_translate_program_multiple() {
        let program = vec\![
            // ADDI x1, x0, 10
            (10 << 20) | (0 << 15) | (0 << 12) | (1 << 7) | 0x13,
            // ADDI x2, x0, 20
            (20 << 20) | (0 << 15) | (0 << 12) | (2 << 7) | 0x13,
            // ADD x3, x1, x2
            (0 << 25) | (2 << 20) | (1 << 15) | (0 << 12) | (3 << 7) | 0x33,
        ];
        let result = Translator::translate_program(&program);
        assert\!(result.is_ok());
    }
}
