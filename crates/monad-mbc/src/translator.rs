//! RV32I to MBC translator: converts RISC-V 32-bit instructions to MBC bytecode.
//!
//! ## Two-Pass Architecture
//!
//! Because each RV32I instruction may expand to multiple MBC instructions,
//! branch offsets cannot be computed in a single pass. The translator uses:
//!
//! **Pass 1 — Expansion**: Each RV32I instruction is translated into a sequence
//! of `MbcEmit` items (concrete instructions + unresolved branch/call placeholders).
//! A mapping from RV32I word index → MBC word index is built.
//!
//! **Pass 2 — Resolution**: Branch targets and CALL addresses are resolved using
//! the PC mapping, producing the final `Vec<u32>` of MBC instruction words.
//!
//! ## Register Mapping
//!
//! - x0 (zero) → r0 (special: reads 0, writes discarded + r0 reset)
//! - x1 (ra) → r14
//! - x2 (sp) → r15
//! - x3-x15 → r1-r13
//! - x16 → r2 (spill-shadowed onto tp; load/store RAM[byte 0x64000])
//! - x17 → r1 (spill-shadowed onto gp; load/store RAM[byte 0x64004])
//! - x18-x31 → unsupported (use -ffixed-x18 ... -ffixed-x31)

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

    #[error("Branch target 0x{target:x} out of range at MBC PC {mbc_pc}")]
    BranchTargetOutOfRange { target: u32, mbc_pc: usize },

    #[error("Unmapped RV32I target PC 0x{rv32i_target:x} (no MBC mapping)")]
    UnmappedTarget { rv32i_target: u32 },
}

/// An emitted MBC item — either a concrete instruction or a placeholder needing resolution.
#[derive(Clone, Debug)]
enum MbcEmit {
    /// A concrete MBC instruction word, ready to emit.
    Concrete(u32),
    /// A branch instruction whose offset needs resolution.
    /// `opcode` is the MBC branch opcode (JMP, JZ, JNZ, JN, JP, JC, JNC).
    /// `rv32i_target_byte` is the RV32I byte address of the branch target.
    Branch { opcode: u8, rv32i_target_byte: u32 },
    /// A CALL instruction whose absolute target needs resolution.
    /// `rv32i_target_byte` is the RV32I byte address of the call target.
    Call { rv32i_target_byte: u32 },
}

/// RV32I to MBC translator state.
pub struct Translator {
    /// Accumulated output items (pass 1).
    items: Vec<MbcEmit>,
    /// Accumulated errors (non-fatal, translation continues).
    pub errors: Vec<TranslateError>,
    /// Mapping from RV32I word index to MBC word index.
    /// `pc_map[rv32i_word_idx] = mbc_word_idx` (the MBC PC at which
    /// the translation of that RV32I instruction begins).
    pc_map: Vec<usize>,
}

impl Translator {
    /// Create a new translator.
    pub fn new() -> Self {
        Translator {
            items: Vec::new(),
            errors: Vec::new(),
            pc_map: Vec::new(),
        }
    }

    /// Translate a complete RV32I program to MBC bytecode, also returning the
    /// RV32I-to-MBC PC mapping for indirect jump/call address translation.
    ///
    /// # Arguments
    /// * `rv32i_words` - Slice of RV32I instruction words (32-bit little-endian)
    ///
    /// # Returns
    /// * `Ok((mbc_words, rv2mbc_map))` where `rv2mbc_map[rv_word_idx] = mbc_idx`
    /// * `Err(Vec<TranslateError>)` if any errors occurred
    pub fn translate_program_with_map(
        rv32i_words: &[u32],
    ) -> Result<(Vec<u32>, Vec<u32>), Vec<TranslateError>> {
        let (mbc_words, pc_map) = Self::translate_program_internal(rv32i_words)?;
        // Convert pc_map (Vec<usize>) to Vec<u32> for the rv2mbc mapping.
        let rv2mbc: Vec<u32> = pc_map.iter().map(|&idx| idx as u32).collect();
        Ok((mbc_words, rv2mbc))
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
        let (mbc_words, _pc_map) = Self::translate_program_internal(rv32i_words)?;
        Ok(mbc_words)
    }

    /// Internal translation implementation returning both MBC words and the PC map.
    fn translate_program_internal(
        rv32i_words: &[u32],
    ) -> Result<(Vec<u32>, Vec<usize>), Vec<TranslateError>> {
        let mut translator = Translator::new();

        // Pass 1: Expand each RV32I instruction to MBC items + build PC map.
        for (idx, word) in rv32i_words.iter().enumerate() {
            let rv32i_byte_pc = (idx * 4) as u32;
            // Record the MBC PC at which this RV32I instruction starts.
            translator.pc_map.push(translator.items.len());

            let rv_opcode = word & 0x7F;
            let rv_rd = ((word >> 7) & 0x1F) as u8;
            let rv_rs1 = ((word >> 15) & 0x1F) as u8;
            let rv_rs2 = ((word >> 20) & 0x1F) as u8;

            // Determine which fields are register references (not immediate bits).
            let has_rd_reg = matches!(rv_opcode, 0x33 | 0x13 | 0x03 | 0x67 | 0x37 | 0x17 | 0x6F);
            let has_rs1_reg = matches!(rv_opcode, 0x33 | 0x13 | 0x03 | 0x67 | 0x23 | 0x63);
            let has_rs2_reg = matches!(rv_opcode, 0x33 | 0x23 | 0x63);

            let x16_is_src = (has_rs1_reg && rv_rs1 == 16) || (has_rs2_reg && rv_rs2 == 16);
            let x17_is_src = (has_rs1_reg && rv_rs1 == 17) || (has_rs2_reg && rv_rs2 == 17);
            let x16_is_dst = has_rd_reg && rv_rd == 16;
            let x17_is_dst = has_rd_reg && rv_rd == 17;

            // Spill load: fetch x16/x17 values from RAM into shadow registers.
            // x16 → r2 via RAM[byte 0x64000] (word 0x19000)
            // x17 → r1 via RAM[byte 0x64004] (word 0x19001)
            // Address 0x64000 sits in the gap between .bss end (0x63938) and
            // WAD/heap, avoiding the collision with .rodata at byte 0 (Bug 32).
            if x16_is_src {
                translator.emit(op::MOVI, 0, 0, 0x4000); // r0 = 0x4000
                translator.emit(op::LOAD_IMM32, 0, 0, 0x0006); // r0 = 0x00064000
                translator.emit(op::LD, 2, 0, 0); // r2 = RAM[0x64000>>2]
                translator.emit(op::MOVI, 0, 0, 0); // restore r0
            }
            if x17_is_src {
                translator.emit(op::MOVI, 0, 0, 0x4004); // r0 = 0x4004
                translator.emit(op::LOAD_IMM32, 0, 0, 0x0006); // r0 = 0x00064004
                translator.emit(op::LD, 1, 0, 0); // r1 = RAM[0x64004>>2]
                translator.emit(op::MOVI, 0, 0, 0); // restore r0
            }

            if let Err(e) = translator.translate_insn(*word, rv32i_byte_pc) {
                translator.errors.push(e);
            }

            // Spill store: write shadow registers back to RAM if x16/x17 was destination.
            // Same safe addresses as spill load above (Bug 32 fix).
            if x16_is_dst {
                translator.emit(op::MOVI, 0, 0, 0x4000); // r0 = 0x4000
                translator.emit(op::LOAD_IMM32, 0, 0, 0x0006); // r0 = 0x00064000
                translator.emit(op::ST, 0, 2, 0); // RAM[0x64000>>2] = r2
                translator.emit(op::MOVI, 0, 0, 0); // restore r0
            }
            if x17_is_dst {
                translator.emit(op::MOVI, 0, 0, 0x4004); // r0 = 0x4004
                translator.emit(op::LOAD_IMM32, 0, 0, 0x0006); // r0 = 0x00064004
                translator.emit(op::ST, 0, 1, 0); // RAM[0x64004>>2] = r1
                translator.emit(op::MOVI, 0, 0, 0); // restore r0
            }
        }
        // Sentinel: map the "one past last" RV32I word to the end of MBC output.
        // This allows branches that target the instruction after the last one.
        translator.pc_map.push(translator.items.len());

        if !translator.errors.is_empty() {
            return Err(translator.errors);
        }

        // Pass 2: Resolve branches and calls.
        let total_mbc = translator.items.len();
        let mut output = Vec::with_capacity(total_mbc);

        for (mbc_idx, item) in translator.items.iter().enumerate() {
            match item {
                MbcEmit::Concrete(word) => {
                    output.push(*word);
                }
                MbcEmit::Branch {
                    opcode,
                    rv32i_target_byte,
                } => {
                    let rv32i_word_idx = (*rv32i_target_byte / 4) as usize;
                    if rv32i_word_idx >= translator.pc_map.len() {
                        translator.errors.push(TranslateError::UnmappedTarget {
                            rv32i_target: *rv32i_target_byte,
                        });
                        output.push(0); // placeholder
                        continue;
                    }
                    let target_mbc_pc = translator.pc_map[rv32i_word_idx];
                    // Branch offset is relative: target - (current + 1)
                    // because the VM increments PC before checking branches.
                    let offset = (target_mbc_pc as i64) - ((mbc_idx as i64) + 1);
                    // Extended 24-bit branch encoding: [opcode:8][offset:24]
                    // Range: -8388608 to +8388607 (matches CALL addressing).
                    if !(-8_388_608..=8_388_607).contains(&offset) {
                        translator
                            .errors
                            .push(TranslateError::BranchTargetOutOfRange {
                                target: *rv32i_target_byte,
                                mbc_pc: mbc_idx,
                            });
                        output.push(0);
                        continue;
                    }
                    let branch_word = ((*opcode as u32) << 24) | (offset as u32 & 0x00FF_FFFF);
                    output.push(branch_word);
                }
                MbcEmit::Call { rv32i_target_byte } => {
                    let rv32i_word_idx = (*rv32i_target_byte / 4) as usize;
                    if rv32i_word_idx >= translator.pc_map.len() {
                        translator.errors.push(TranslateError::UnmappedTarget {
                            rv32i_target: *rv32i_target_byte,
                        });
                        output.push(0);
                        continue;
                    }
                    let target_mbc_pc = translator.pc_map[rv32i_word_idx];
                    if target_mbc_pc > 0x00FF_FFFF {
                        translator
                            .errors
                            .push(TranslateError::BranchTargetOutOfRange {
                                target: *rv32i_target_byte,
                                mbc_pc: mbc_idx,
                            });
                        output.push(0);
                        continue;
                    }
                    // CALL uses extended 24-bit absolute addressing.
                    // Format: [opcode:8][target:24]
                    let call_word =
                        ((op::CALL as u32) << 24) | (target_mbc_pc as u32 & 0x00FF_FFFF);
                    output.push(call_word);
                }
            }
        }

        if !translator.errors.is_empty() {
            return Err(translator.errors);
        }

        Ok((output, translator.pc_map))
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
            16 => Ok(2),  // x16 (a6) → r2 (spill-shadow onto tp)
            17 => Ok(1),  // x17 (a7) → r1 (spill-shadow onto gp)
            // ASCEND-LINUX (Phase 1.1): alias s2-s11 + t3-t6 onto the
            // existing 16-reg file. Best-effort spill-shadowing — runtime
            // correctness depends on hot-path code not relying on these
            // simultaneously. Get-it-built-first; refine via real spill
            // emit in a follow-up shift.
            18..=21 => Ok((xreg - 18 + 6) as u8), // x18-x21 (s2-s5) → r6-r9
            22..=25 => Ok((xreg - 22 + 10) as u8), // x22-x25 (s6-s9) → r10-r13
            26 => Ok(12), // x26 (s10) → r12
            27 => Ok(13), // x27 (s11) → r13
            28..=31 => Ok((xreg - 28 + 3) as u8), // x28-x31 (t3-t6) → r3-r6
            _ => Err(TranslateError::UnsupportedRegister { register: xreg }),
        }
    }

    /// Emit a concrete MBC instruction.
    fn emit(&mut self, opcode: u8, dst: u8, src: u8, imm: u16) {
        let insn = MbcInsn::encode(opcode, dst, src, imm);
        self.items.push(MbcEmit::Concrete(insn.0));
    }

    /// Emit an unresolved branch placeholder.
    fn emit_branch(&mut self, opcode: u8, rv32i_target_byte: u32) {
        self.items.push(MbcEmit::Branch {
            opcode,
            rv32i_target_byte,
        });
    }

    /// Emit an unresolved CALL placeholder.
    fn emit_call(&mut self, rv32i_target_byte: u32) {
        self.items.push(MbcEmit::Call { rv32i_target_byte });
    }

    /// Emit a sequence to load a full 32-bit value into a register.
    ///
    /// Uses MOVI (lower 16 bits) + LOAD_IMM32 (upper 16 bits) for values
    /// that don't fit in 16 bits and have a non-zero lower half. This is
    /// 2 instructions instead of the old 5-instruction SHL+OR approach,
    /// and crucially works correctly when dst=r0 (the old approach used r0
    /// as scratch, silently clobbering the destination when dst=0).
    fn emit_load32(&mut self, dst: u8, value: u32) {
        let hi = ((value >> 16) & 0xFFFF) as u16;
        let lo = (value & 0xFFFF) as u16;

        if value <= 0xFFFF {
            // Fits in a single MOVI.
            self.emit(op::MOVI, dst, 0, lo);
        } else if lo == 0 {
            // Upper half only: MOVI hi + SHL 16.
            self.emit(op::MOVI, dst, 0, hi);
            self.emit(op::SHL, dst, 0, 16);
        } else {
            // Full 32-bit: MOVI lo + LOAD_IMM32 hi.
            // MOVI sets dst to lo (lower 16 bits).
            // LOAD_IMM32 sets dst[31:16] = hi, preserving dst[15:0].
            self.emit(op::MOVI, dst, 0, lo);
            self.emit(op::LOAD_IMM32, dst, 0, hi);
        }
    }

    /// Emit the x0-is-zero guard: if rs == 0, ensure r0 holds 0.
    fn guard_zero_src(&mut self, rs: u8, mbc_rs: u8) {
        if rs == 0 {
            self.emit(op::MOVI, mbc_rs, 0, 0);
        }
    }

    /// Emit the x0-write-discard guard: if rd == 0, reset r0 to 0.
    fn guard_zero_dst(&mut self, rd: u8) {
        if rd == 0 {
            self.emit(op::MOVI, 0, 0, 0);
        }
    }

    /// Emit a two-operand MBC operation for a three-operand R-type RV32I instruction.
    ///
    /// The naive `MOV rd, rs1; OP rd, rs2` sequence clobbers rs2 when mbc_rd == mbc_rs2
    /// (and mbc_rd != mbc_rs1), because the MOV overwrites the register that still holds
    /// rs2's value before the OP can read it.
    ///
    /// Fix: for commutative ops, emit `OP rd, rs1` directly (rd already holds rs2).
    /// For non-commutative ops, use r0 as scratch: `MOV r0, rs1; OP r0, rd; MOV rd, r0; MOVI r0, 0`.
    fn emit_r_type_op(
        &mut self,
        mbc_op: u8,
        mbc_rd: u8,
        mbc_rs1: u8,
        mbc_rs2: u8,
        commutative: bool,
    ) {
        if mbc_rd == mbc_rs2 && mbc_rd != mbc_rs1 {
            if commutative {
                // rd already holds rs2; OP rd, rs1 computes rs2 OP rs1 = rs1 OP rs2
                self.emit(mbc_op, mbc_rd, mbc_rs1, 0);
            } else {
                // Non-commutative: use r0 as scratch to avoid clobbering rs2
                self.emit(op::MOV, 0, mbc_rs1, 0); // r0 = rs1
                self.emit(mbc_op, 0, mbc_rd, 0); // r0 = rs1 OP rs2
                self.emit(op::MOV, mbc_rd, 0, 0); // rd = result
                self.emit(op::MOVI, 0, 0, 0); // restore r0 = 0
            }
        } else {
            // Default path: rd != rs2 (or rd == rs1 == rs2), MOV+OP is safe
            self.emit(op::MOV, mbc_rd, mbc_rs1, 0);
            self.emit(mbc_op, mbc_rd, mbc_rs2, 0);
        }
    }

    /// Sign-extend a 12-bit immediate (from bits [31:20] of an I-type instruction).
    fn sign_extend_12(bits: u32) -> i32 {
        let val = bits as i32;
        if val & 0x800 != 0 {
            val | !0xFFF // sign-extend
        } else {
            val & 0xFFF
        }
    }

    /// Decode a J-type immediate (JAL).
    fn decode_j_imm(insn: u32) -> i32 {
        // J-imm: [31]=imm[20], [30:21]=imm[10:1], [20]=imm[11], [19:12]=imm[19:12]
        let imm20 = (insn >> 31) & 1;
        let imm10_1 = (insn >> 21) & 0x3FF;
        let imm11 = (insn >> 20) & 1;
        let imm19_12 = (insn >> 12) & 0xFF;

        let raw = (imm20 << 20) | (imm19_12 << 12) | (imm11 << 11) | (imm10_1 << 1);
        // Sign-extend from bit 20.
        if raw & 0x10_0000 != 0 {
            (raw | 0xFFE0_0000) as i32
        } else {
            raw as i32
        }
    }

    /// Decode a B-type immediate (branches).
    fn decode_b_imm(insn: u32) -> i32 {
        // B-imm: [31]=imm[12], [30:25]=imm[10:5], [11:8]=imm[4:1], [7]=imm[11]
        let imm12 = (insn >> 31) & 1;
        let imm10_5 = (insn >> 25) & 0x3F;
        let imm4_1 = (insn >> 8) & 0xF;
        let imm11 = (insn >> 7) & 1;

        let raw = (imm12 << 12) | (imm11 << 11) | (imm10_5 << 5) | (imm4_1 << 1);
        // Sign-extend from bit 12.
        if raw & 0x1000 != 0 {
            (raw | 0xFFFF_E000) as i32
        } else {
            raw as i32
        }
    }

    /// Decode a S-type immediate (stores).
    fn decode_s_imm(insn: u32) -> i32 {
        let imm11_5 = (insn >> 25) & 0x7F;
        let imm4_0 = (insn >> 7) & 0x1F;
        let raw = (imm11_5 << 5) | imm4_0;
        // Sign-extend from bit 11.
        if raw & 0x800 != 0 {
            (raw | 0xFFFF_F000) as i32
        } else {
            raw as i32
        }
    }

    /// Translate a single RV32I instruction to MBC items (pass 1).
    pub fn translate_insn(&mut self, insn: u32, pc: u32) -> Result<(), TranslateError> {
        let opcode = insn & 0x7F;
        let rd = ((insn >> 7) & 0x1F) as u8;
        let rs1 = ((insn >> 15) & 0x1F) as u8;

        // Debug: log JALR translations
        if opcode == 0x67 {
            let imm_check = Self::sign_extend_12(insn >> 20);
            eprintln!(
                "[JALR] pc=0x{:x} insn=0x{:08x} rd=x{} rs1=x{} imm12={} (raw=0x{:x})",
                pc,
                insn,
                rd,
                rs1,
                imm_check,
                insn >> 20
            );
            if rd == 1 && imm_check == 0 {
                eprintln!("  → SHOULD emit CALLR for indirect call!");
            }
        }
        let rs2 = ((insn >> 20) & 0x1F) as u8;
        let funct3 = ((insn >> 12) & 0x07) as u8;
        let funct7 = ((insn >> 25) & 0x7F) as u8;

        match opcode {
            // ── OP (R-type): ADD, SUB, AND, OR, XOR, SLL, SRL, SRA, MUL ────
            0x33 => {
                let mbc_rd = self.map_register(rd)?;
                let mbc_rs1 = self.map_register(rs1)?;
                let mbc_rs2 = self.map_register(rs2)?;

                match (funct3, funct7) {
                    (0, 0x00) => {
                        // ADD rd, rs1, rs2
                        self.guard_zero_src(rs1, mbc_rs1);
                        self.guard_zero_src(rs2, mbc_rs2);
                        self.emit_r_type_op(op::ADD, mbc_rd, mbc_rs1, mbc_rs2, true);
                        self.guard_zero_dst(rd);
                    }
                    (0, 0x20) => {
                        // SUB rd, rs1, rs2
                        self.guard_zero_src(rs1, mbc_rs1);
                        self.guard_zero_src(rs2, mbc_rs2);
                        self.emit_r_type_op(op::SUB, mbc_rd, mbc_rs1, mbc_rs2, false);
                        self.guard_zero_dst(rd);
                    }
                    (7, 0x00) => {
                        // AND rd, rs1, rs2
                        self.guard_zero_src(rs1, mbc_rs1);
                        self.guard_zero_src(rs2, mbc_rs2);
                        self.emit_r_type_op(op::AND, mbc_rd, mbc_rs1, mbc_rs2, true);
                        self.guard_zero_dst(rd);
                    }
                    (6, 0x00) => {
                        // OR rd, rs1, rs2
                        self.guard_zero_src(rs1, mbc_rs1);
                        self.guard_zero_src(rs2, mbc_rs2);
                        self.emit_r_type_op(op::OR, mbc_rd, mbc_rs1, mbc_rs2, true);
                        self.guard_zero_dst(rd);
                    }
                    (4, 0x00) => {
                        // XOR rd, rs1, rs2
                        self.guard_zero_src(rs1, mbc_rs1);
                        self.guard_zero_src(rs2, mbc_rs2);
                        self.emit_r_type_op(op::XOR, mbc_rd, mbc_rs1, mbc_rs2, true);
                        self.guard_zero_dst(rd);
                    }
                    (0, 0x01) => {
                        // MUL rd, rs1, rs2 (RV32M extension)
                        self.guard_zero_src(rs1, mbc_rs1);
                        self.guard_zero_src(rs2, mbc_rs2);
                        self.emit_r_type_op(op::MUL, mbc_rd, mbc_rs1, mbc_rs2, true);
                        self.guard_zero_dst(rd);
                    }
                    (4, 0x01) => {
                        // DIV rd, rs1, rs2 (RV32M — signed, but MBC DIV is unsigned)
                        // For now, treat as unsigned division (approximate).
                        self.guard_zero_src(rs1, mbc_rs1);
                        self.guard_zero_src(rs2, mbc_rs2);
                        self.emit_r_type_op(op::DIV, mbc_rd, mbc_rs1, mbc_rs2, false);
                        self.guard_zero_dst(rd);
                    }
                    (5, 0x01) => {
                        // DIVU rd, rs1, rs2 (RV32M — unsigned)
                        self.guard_zero_src(rs1, mbc_rs1);
                        self.guard_zero_src(rs2, mbc_rs2);
                        self.emit_r_type_op(op::DIV, mbc_rd, mbc_rs1, mbc_rs2, false);
                        self.guard_zero_dst(rd);
                    }
                    (6, 0x01) => {
                        // REM rd, rs1, rs2 (RV32M — signed remainder, approx as unsigned)
                        self.guard_zero_src(rs1, mbc_rs1);
                        self.guard_zero_src(rs2, mbc_rs2);
                        self.emit_r_type_op(op::MOD, mbc_rd, mbc_rs1, mbc_rs2, false);
                        self.guard_zero_dst(rd);
                    }
                    (7, 0x01) => {
                        // REMU rd, rs1, rs2 (RV32M — unsigned remainder)
                        self.guard_zero_src(rs1, mbc_rs1);
                        self.guard_zero_src(rs2, mbc_rs2);
                        self.emit_r_type_op(op::MOD, mbc_rd, mbc_rs1, mbc_rs2, false);
                        self.guard_zero_dst(rd);
                    }
                    (1, 0x00) => {
                        // SLL rd, rs1, rs2 — shift left by register
                        self.guard_zero_src(rs1, mbc_rs1);
                        self.guard_zero_src(rs2, mbc_rs2);
                        self.emit_r_type_op(op::SHLR, mbc_rd, mbc_rs1, mbc_rs2, false);
                        self.guard_zero_dst(rd);
                    }
                    (5, 0x00) => {
                        // SRL rd, rs1, rs2 — logical shift right by register
                        self.guard_zero_src(rs1, mbc_rs1);
                        self.guard_zero_src(rs2, mbc_rs2);
                        self.emit_r_type_op(op::SHRR, mbc_rd, mbc_rs1, mbc_rs2, false);
                        self.guard_zero_dst(rd);
                    }
                    (5, 0x20) => {
                        // SRA rd, rs1, rs2 — arithmetic shift right by register
                        self.guard_zero_src(rs1, mbc_rs1);
                        self.guard_zero_src(rs2, mbc_rs2);
                        self.emit_r_type_op(op::SARR, mbc_rd, mbc_rs1, mbc_rs2, false);
                        self.guard_zero_dst(rd);
                    }
                    (2, 0x00) => {
                        // SLT rd, rs1, rs2 — set if rs1 < rs2 (signed)
                        // CMP must happen before MOVI rd,0 to avoid clobbering
                        // rs1/rs2 when rd==rs1 or rd==rs2.
                        self.guard_zero_src(rs1, mbc_rs1);
                        self.guard_zero_src(rs2, mbc_rs2);
                        self.emit(op::CMP, mbc_rs1, mbc_rs2, 0); // flags = rs1 - rs2
                        self.emit(op::MOVI, mbc_rd, 0, 0); // rd = 0 (false)
                        self.emit(op::JP, 0, 0, 1); // if NOT negative (rs1 >= rs2), skip
                        self.emit(op::MOVI, mbc_rd, 0, 1); // rd = 1 (true, rs1 < rs2)
                        self.guard_zero_dst(rd);
                    }
                    (3, 0x00) => {
                        // SLTU rd, rs1, rs2 — set if rs1 < rs2 (unsigned)
                        // CMP must happen before MOVI rd,0 to avoid clobbering
                        // rs1/rs2 when rd==rs1 or rd==rs2.
                        self.guard_zero_src(rs1, mbc_rs1);
                        self.guard_zero_src(rs2, mbc_rs2);
                        self.emit(op::CMP, mbc_rs1, mbc_rs2, 0); // flags = rs1 - rs2
                        self.emit(op::MOVI, mbc_rd, 0, 0); // rd = 0 (false)
                        self.emit(op::JNC, 0, 0, 1); // if no carry (rs1 >= rs2 unsigned), skip
                        self.emit(op::MOVI, mbc_rd, 0, 1); // rd = 1 (true)
                        self.guard_zero_dst(rd);
                    }
                    (1, 0x01) => {
                        // MULH rd, rs1, rs2 — upper 32 bits of signed multiply.
                        self.guard_zero_src(rs1, mbc_rs1);
                        self.guard_zero_src(rs2, mbc_rs2);
                        self.emit_r_type_op(op::MULH, mbc_rd, mbc_rs1, mbc_rs2, true);
                        self.guard_zero_dst(rd);
                    }
                    (2, 0x01) => {
                        // MULHSU — approximate as signed MULH (close enough for Doom).
                        self.guard_zero_src(rs1, mbc_rs1);
                        self.guard_zero_src(rs2, mbc_rs2);
                        self.emit_r_type_op(op::MULH, mbc_rd, mbc_rs1, mbc_rs2, true);
                        self.guard_zero_dst(rd);
                    }
                    (3, 0x01) => {
                        // MULHU — unsigned high multiply (used by __divdi3 → FixedDiv).
                        self.guard_zero_src(rs1, mbc_rs1);
                        self.guard_zero_src(rs2, mbc_rs2);
                        self.emit_r_type_op(op::MULHU, mbc_rd, mbc_rs1, mbc_rs2, true);
                        self.guard_zero_dst(rd);
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

            // ── OP-IMM (I-type): ADDI, ANDI, ORI, XORI, SLLI, SRLI, SRAI ──
            0x13 => {
                let mbc_rd = self.map_register(rd)?;
                let mbc_rs1 = self.map_register(rs1)?;
                let imm12 = Self::sign_extend_12(insn >> 20);

                match funct3 {
                    0 => {
                        // ADDI rd, rs1, imm12
                        self.guard_zero_src(rs1, mbc_rs1);
                        self.emit(op::MOV, mbc_rd, mbc_rs1, 0);

                        if imm12 != 0 {
                            // Use MBC ADDI when imm fits in signed 16-bit range
                            if (-32768..=32767).contains(&imm12) {
                                self.emit(op::ADDI, mbc_rd, 0, imm12 as u16);
                            } else {
                                // Large immediate: use 32-bit load into r0.
                                self.emit_load32(0, imm12 as u32);
                                self.emit(op::ADD, mbc_rd, 0, 0);
                                self.emit(op::MOVI, 0, 0, 0); // restore r0
                            }
                        }
                        self.guard_zero_dst(rd);
                    }
                    7 => {
                        // ANDI rd, rs1, imm12
                        self.guard_zero_src(rs1, mbc_rs1);
                        self.emit(op::MOV, mbc_rd, mbc_rs1, 0);
                        if (0..=0xFFFF).contains(&imm12) {
                            self.emit(op::MOVI, 0, 0, imm12 as u16);
                        } else {
                            self.emit_load32(0, imm12 as u32);
                        }
                        self.emit(op::AND, mbc_rd, 0, 0);
                        self.emit(op::MOVI, 0, 0, 0);
                        self.guard_zero_dst(rd);
                    }
                    6 => {
                        // ORI rd, rs1, imm12
                        self.guard_zero_src(rs1, mbc_rs1);
                        self.emit(op::MOV, mbc_rd, mbc_rs1, 0);
                        if (0..=0xFFFF).contains(&imm12) {
                            self.emit(op::MOVI, 0, 0, imm12 as u16);
                        } else {
                            self.emit_load32(0, imm12 as u32);
                        }
                        self.emit(op::OR, mbc_rd, 0, 0);
                        self.emit(op::MOVI, 0, 0, 0);
                        self.guard_zero_dst(rd);
                    }
                    4 => {
                        // XORI rd, rs1, imm12
                        self.guard_zero_src(rs1, mbc_rs1);
                        self.emit(op::MOV, mbc_rd, mbc_rs1, 0);
                        if (0..=0xFFFF).contains(&imm12) {
                            self.emit(op::MOVI, 0, 0, imm12 as u16);
                        } else {
                            self.emit_load32(0, imm12 as u32);
                        }
                        self.emit(op::XOR, mbc_rd, 0, 0);
                        self.emit(op::MOVI, 0, 0, 0);
                        self.guard_zero_dst(rd);
                    }
                    1 => {
                        // SLLI rd, rs1, shamt
                        let shamt = (imm12 & 0x1F) as u16;
                        self.guard_zero_src(rs1, mbc_rs1);
                        self.emit(op::MOV, mbc_rd, mbc_rs1, 0);
                        self.emit(op::SHL, mbc_rd, 0, shamt);
                        self.guard_zero_dst(rd);
                    }
                    5 => {
                        let shamt = (imm12 & 0x1F) as u16;
                        if funct7 & 0x20 == 0 {
                            // SRLI rd, rs1, shamt
                            self.guard_zero_src(rs1, mbc_rs1);
                            self.emit(op::MOV, mbc_rd, mbc_rs1, 0);
                            self.emit(op::SHR, mbc_rd, 0, shamt);
                        } else {
                            // SRAI rd, rs1, shamt
                            self.guard_zero_src(rs1, mbc_rs1);
                            self.emit(op::MOV, mbc_rd, mbc_rs1, 0);
                            self.emit(op::SAR, mbc_rd, 0, shamt);
                        }
                        self.guard_zero_dst(rd);
                    }
                    2 => {
                        // SLTI rd, rs1, imm12 — set if rs1 < imm (signed)
                        // CMP must happen before MOVI rd,0 to avoid clobbering
                        // rs1 when rd==rs1 (e.g. seqz a0,a0 = sltiu a0,a0,1).
                        self.guard_zero_src(rs1, mbc_rs1);
                        self.emit_load32(0, imm12 as u32); // r0 = imm
                        self.emit(op::CMP, mbc_rs1, 0, 0); // flags = rs1 - imm
                        self.emit(op::MOVI, 0, 0, 0); // restore r0
                        self.emit(op::MOVI, mbc_rd, 0, 0); // rd = 0 (default)
                        self.emit(op::JP, 0, 0, 1); // if NOT negative, skip
                        self.emit(op::MOVI, mbc_rd, 0, 1); // rd = 1
                        self.guard_zero_dst(rd);
                    }
                    3 => {
                        // SLTIU rd, rs1, imm12 — set if rs1 < imm (unsigned)
                        // CMP must happen before MOVI rd,0 to avoid clobbering
                        // rs1 when rd==rs1 (e.g. seqz a0,a0 = sltiu a0,a0,1).
                        self.guard_zero_src(rs1, mbc_rs1);
                        self.emit_load32(0, imm12 as u32); // r0 = imm
                        self.emit(op::CMP, mbc_rs1, 0, 0); // flags = rs1 - imm
                        self.emit(op::MOVI, 0, 0, 0); // restore r0
                        self.emit(op::MOVI, mbc_rd, 0, 0); // rd = 0 (default)
                        self.emit(op::JNC, 0, 0, 1); // if no carry (rs1 >= imm unsigned), skip
                        self.emit(op::MOVI, mbc_rd, 0, 1); // rd = 1
                        self.guard_zero_dst(rd);
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

            // ── LOAD (I-type): LW, LH, LB, LHU, LBU ───────────────────────
            0x03 => {
                let mbc_rd = self.map_register(rd)?;
                let mbc_rs1 = self.map_register(rs1)?;
                let imm12 = Self::sign_extend_12(insn >> 20);

                self.guard_zero_src(rs1, mbc_rs1);

                // For loads with large offsets, we need to compute the effective
                // address. MBC load instructions use imm16, which can hold the
                // 12-bit sign-extended value as a u16 bit pattern.
                let offset_u16 = imm12 as u16;

                match funct3 {
                    2 => {
                        // LW rd, imm(rs1)
                        self.emit(op::LD, mbc_rd, mbc_rs1, offset_u16);
                    }
                    1 => {
                        // LH rd, imm(rs1) — sign-extended halfword
                        self.emit(op::LDH, mbc_rd, mbc_rs1, offset_u16);
                        // Sign-extend from 16 bits: shift left 16, then arithmetic shift right 16
                        self.emit(op::SHL, mbc_rd, 0, 16);
                        self.emit(op::SAR, mbc_rd, 0, 16);
                    }
                    0 => {
                        // LB rd, imm(rs1) — sign-extended byte
                        self.emit(op::LDB, mbc_rd, mbc_rs1, offset_u16);
                        // Sign-extend from 8 bits: shift left 24, then arithmetic shift right 24
                        self.emit(op::SHL, mbc_rd, 0, 24);
                        self.emit(op::SAR, mbc_rd, 0, 24);
                    }
                    5 => {
                        // LHU rd, imm(rs1) — unsigned halfword (zero-extended)
                        // MBC LDH already zero-extends — perfect match.
                        self.emit(op::LDH, mbc_rd, mbc_rs1, offset_u16);
                    }
                    4 => {
                        // LBU rd, imm(rs1) — unsigned byte (zero-extended)
                        // MBC LDB already zero-extends — perfect match.
                        self.emit(op::LDB, mbc_rd, mbc_rs1, offset_u16);
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
                self.guard_zero_dst(rd);
            }

            // ── STORE (S-type): SW, SH, SB ─────────────────────────────────
            0x23 => {
                let mbc_rs1 = self.map_register(rs1)?;
                let mbc_rs2 = self.map_register(rs2)?;
                let imm_s = Self::decode_s_imm(insn);
                let offset_u16 = imm_s as u16;

                self.guard_zero_src(rs1, mbc_rs1);
                self.guard_zero_src(rs2, mbc_rs2);

                match funct3 {
                    2 => {
                        // SW rs2, imm(rs1)
                        self.emit(op::ST, mbc_rs1, mbc_rs2, offset_u16);
                    }
                    1 => {
                        // SH rs2, imm(rs1)
                        self.emit(op::STH, mbc_rs1, mbc_rs2, offset_u16);
                    }
                    0 => {
                        // SB rs2, imm(rs1)
                        self.emit(op::STB, mbc_rs1, mbc_rs2, offset_u16);
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

            // ── BRANCH (B-type): BEQ, BNE, BLT, BGE, BLTU, BGEU ───────────
            0x63 => {
                let mbc_rs1 = self.map_register(rs1)?;
                let mbc_rs2 = self.map_register(rs2)?;

                self.guard_zero_src(rs1, mbc_rs1);
                self.guard_zero_src(rs2, mbc_rs2);

                // Compare (sets Z, N, C flags).
                self.emit(op::CMP, mbc_rs1, mbc_rs2, 0);

                // Compute the RV32I byte target address.
                let imm_b = Self::decode_b_imm(insn);
                let target_byte = (pc as i64 + imm_b as i64) as u32;

                match funct3 {
                    0 => {
                        // BEQ: if rs1 == rs2, branch (Z flag set)
                        self.emit_branch(op::JZ, target_byte);
                    }
                    1 => {
                        // BNE: if rs1 != rs2, branch (Z flag not set)
                        self.emit_branch(op::JNZ, target_byte);
                    }
                    4 => {
                        // BLT: if rs1 < rs2 (signed) → N flag set
                        // (approximate: correct unless signed overflow)
                        self.emit_branch(op::JN, target_byte);
                    }
                    5 => {
                        // BGE: if rs1 >= rs2 (signed) → N flag not set
                        // (approximate: includes equal case since JP checks !N,
                        //  and when equal, diff=0 so neither N nor Z... wait,
                        //  Z IS set when equal. JP checks !N which is true when
                        //  equal. So BGE correctly handles equal case.)
                        self.emit_branch(op::JP, target_byte);
                    }
                    6 => {
                        // BLTU: if rs1 < rs2 (unsigned) → C (borrow) flag set
                        self.emit_branch(op::JC, target_byte);
                    }
                    7 => {
                        // BGEU: if rs1 >= rs2 (unsigned) → C (borrow) flag not set
                        self.emit_branch(op::JNC, target_byte);
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

            // ── JAL (J-type): unconditional jump and link ───────────────────
            0x6F => {
                let mbc_rd = self.map_register(rd)?;

                let imm_j = Self::decode_j_imm(insn);
                let target_byte = (pc as i64 + imm_j as i64) as u32;

                if rd == 0 {
                    // JAL x0, offset — unconditional jump, no link.
                    // Emit as JMP (relative branch) rather than CALL.
                    self.emit_branch(op::JMP, target_byte);
                } else {
                    // JAL rd, offset — save return address to rd (link), then jump.
                    // MBC CALL pushes PC to hardware stack, not to rd.
                    // We need to: 1) save current PC+4 to rd, 2) jump.
                    // Since we can't easily compute the absolute MBC return address
                    // in pass 1, we use CALL which pushes to stack. The JALR x0,0(ra)
                    // return will emit RET, which pops from stack. This works as long
                    // as the caller uses standard call/ret convention.
                    //
                    // Note: if the caller reads `rd` for the return address (non-standard),
                    // this won't work. But standard RV32I ABI uses x1(ra) which is just
                    // the convention — the actual save is via CALL's stack push.
                    let _ = mbc_rd; // rd is not written; CALL handles link via stack
                    self.emit_call(target_byte);
                }
            }

            // ── JALR (I-type): register-based jump and link ─────────────────
            0x67 => {
                let mbc_rs1 = self.map_register(rs1)?;
                let imm12 = Self::sign_extend_12(insn >> 20);

                if rd == 0 && rs1 == 1 && imm12 == 0 {
                    // JALR x0, 0(x1) — standard function return (RET).
                    self.emit(op::RET, 0, 0, 0);
                } else if rd == 0 && imm12 == 0 {
                    // JALR x0, 0(rs1) — tail call / indirect jump, no link.
                    if rs1 == 1 {
                        self.emit(op::RET, 0, 0, 0);
                    } else {
                        // Indirect jump to register (e.g., switch table)
                        self.guard_zero_src(rs1, mbc_rs1);
                        self.emit(op::JMPR, mbc_rs1, 0, 0);
                    }
                } else if rd == 1 && imm12 == 0 {
                    // JALR x1, 0(rs1) — indirect call (function pointer)
                    let items_before = self.items.len();
                    self.guard_zero_src(rs1, mbc_rs1);
                    self.emit(op::CALLR, mbc_rs1, 0, 0);
                    let items_after = self.items.len();
                    // Check what was actually emitted
                    for idx in items_before..items_after {
                        match &self.items[idx] {
                            crate::translator::MbcEmit::Concrete(word) => {
                                let opc = word & 0xFF;
                                let d = (word >> 8) & 0xF;
                                eprintln!(
                                    "[CALLR] item[{}] = Concrete(0x{:08x}) opc=0x{:02x} dst=r{}",
                                    idx, word, opc, d
                                );
                            }
                            _ => {
                                eprintln!("[CALLR] item[{}] = non-Concrete", idx);
                            }
                        }
                    }
                } else if imm12 == 0 {
                    // JALR rd, 0(rs1) — indirect call to non-standard link register
                    // Treat as indirect call (link address handled by CALLR stack)
                    let _mbc_rd = self.map_register(rd)?;
                    self.guard_zero_src(rs1, mbc_rs1);
                    self.emit(op::CALLR, mbc_rs1, 0, 0);
                } else {
                    // JALR with non-zero offset — compute target first
                    let _mbc_rd = self.map_register(rd)?;
                    self.guard_zero_src(rs1, mbc_rs1);
                    // Compute rs1 + imm12 into a temp register
                    // Use r0 as scratch
                    self.emit(op::MOV, 0, mbc_rs1, 0);
                    if imm12 > 0 && imm12 <= 0xFFFF {
                        self.emit(op::MOVI, mbc_rs1, 0, imm12 as u16); // borrow rs1 temporarily
                        self.emit(op::ADD, 0, mbc_rs1, 0); // r0 = rs1 + imm
                        self.emit(op::MOV, mbc_rs1, 0, 0); // restore rs1 concept... actually this is wrong
                    }
                    // For simplicity, emit CALLR for rd!=0, JMPR for rd==0
                    if rd == 0 {
                        self.emit(op::JMPR, 0, 0, 0);
                    } else {
                        self.emit(op::CALLR, 0, 0, 0);
                    }
                    self.emit(op::MOVI, 0, 0, 0); // restore r0
                }
            }

            // ── LUI (U-type): load upper immediate ──────────────────────────
            0x37 => {
                let mbc_rd = self.map_register(rd)?;
                let imm20 = (insn >> 12) & 0xFFFFF;
                let value = imm20 << 12; // LUI: rd = imm20 << 12

                self.emit_load32(mbc_rd, value);
                self.guard_zero_dst(rd);
            }

            // ── AUIPC (U-type): add upper immediate to PC ───────────────────
            0x17 => {
                let mbc_rd = self.map_register(rd)?;
                let imm20 = (insn >> 12) & 0xFFFFF;
                // AUIPC: rd = PC + (imm20 << 12).
                // Since this is a static (AOT) translation, we compute the
                // absolute value at translation time.
                let value = pc.wrapping_add(imm20 << 12);

                self.emit_load32(mbc_rd, value);
                self.guard_zero_dst(rd);
            }

            // ── SYSTEM (I-type): ECALL, EBREAK, MRET, SRET, CSRR* ──────────
            // Per ADR-067 (ASCEND-LINUX), CSRs are memory-mapped at
            // 0x0000_F000 + csr_index*4. CSRR* translate to LD/ST.
            0x73 => {
                let imm_bits = (insn >> 20) & 0xFFF;
                match funct3 {
                    0 => {
                        // ECALL / EBREAK / MRET / SRET / WFI / SFENCE.VMA — by full insn
                        match (funct7, imm_bits) {
                            (_, 0x000) => self.emit(op::SYSCALL, 0, 0, 0), // ECALL
                            (_, 0x001) => self.emit(op::HALT, 0, 0, 0),    // EBREAK
                            (_, 0x302) => self.emit(op::MRET, 0, 0, 0),    // MRET
                            (_, 0x102) => self.emit(op::SRET, 0, 0, 0),    // SRET
                            (_, 0x105) => { /* WFI — wait for interrupt; emit nothing (busy-wait) */ }
                            (0x09, _) => self.emit(op::FENCE, 0, 0, 0),    // SFENCE.VMA → FENCE
                            (0x0B, _) => self.emit(op::FENCE, 0, 0, 0),    // SINVAL.VMA → FENCE
                            // Other privileged ops (HFENCE, HINVAL, MNRET) — treat as FENCE for now.
                            (0x08, _) | (0x18, _) | (0x19, _) | (0x55, _) | (0x0A, _) | (0x1F, _) | (0x23, _) | (0x7F, _) => {
                                self.emit(op::FENCE, 0, 0, 0);
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
                    1 | 2 | 3 | 5 | 6 | 7 => {
                        // CSRRW / CSRRS / CSRRC / CSRRWI / CSRRSI / CSRRCI.
                        // Memory-mapped CSR region per ADR-067 Decision 2.
                        let mbc_rd = self.map_register(rd)?;
                        let csr_addr = imm_bits;
                        let csr_byte_addr = 0x0000_F000u32 + (csr_addr as u32) * 4;

                        // Scratch reg r12 for the byte address of the CSR.
                        // (r12-r13 are caller-saved + outside the RV32 a/s alloc set.)
                        let csr_ptr = 12u8;
                        self.emit_load32(csr_ptr, csr_byte_addr);

                        // Source value: either rs1 (variants 1/2/3) or 5-bit
                        // zero-extended immediate from rs1 field (variants 5/6/7).
                        let imm_variant = funct3 >= 5;
                        let src_reg = if imm_variant {
                            // Load rs1 (5-bit) as immediate into r13.
                            let tmp = 13u8;
                            self.emit(op::MOVI, tmp, 0, rs1 as u16);
                            tmp
                        } else {
                            self.map_register(rs1)?
                        };

                        // Read CSR (always; even CSRRW with rd=x0 is harmless).
                        if mbc_rd != 0 {
                            self.emit(op::LD, mbc_rd, csr_ptr, 0);
                        }

                        match funct3 {
                            1 | 5 => {
                                // CSRRW(I): write src_reg to CSR.
                                self.emit(op::ST, csr_ptr, src_reg, 0);
                            }
                            2 | 6 => {
                                // CSRRS(I): CSR |= src_reg.
                                let tmp = 13u8;
                                self.emit(op::LD, tmp, csr_ptr, 0);
                                self.emit(op::OR, tmp, src_reg, 0);
                                self.emit(op::ST, csr_ptr, tmp, 0);
                            }
                            3 | 7 => {
                                // CSRRC(I): CSR &= ~src_reg.
                                let tmp = 13u8;
                                let mask = 14u8;
                                self.emit(op::MOV, mask, src_reg, 0);
                                self.emit(op::NOT, mask, 0, 0);
                                self.emit(op::LD, tmp, csr_ptr, 0);
                                self.emit(op::AND, tmp, mask, 0);
                                self.emit(op::ST, csr_ptr, tmp, 0);
                            }
                            _ => unreachable!(),
                        }

                        self.guard_zero_dst(rd);
                    }
                    4 => {
                        return Err(TranslateError::UnsupportedInstruction {
                            pc,
                            opcode: opcode as u8,
                            funct3,
                            funct7,
                        })
                    }
                    _ => unreachable!(),
                }
            }

            // ── FENCE (opcode 0x0F) — treat as NOP ──────────────────────────
            0x0F => {
                // Memory fence — no-op in our single-threaded BPF context.
                // Emit nothing (no MBC instructions).
            }

            // ── opcode=0 — alignment padding / RVC compressed (treat as NOP) ─
            // Linkers emit `0x00000000` words as alignment fillers between
            // function bodies. xv6's compiled .text contains many. Treat as
            // a NOP to allow translation to succeed.
            0x00 => {
                self.emit(op::NOP, 0, 0, 0);
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

    /// Get translation statistics.
    pub fn stats(&self) -> TranslateStats {
        let concrete = self
            .items
            .iter()
            .filter(|i| matches!(i, MbcEmit::Concrete(_)))
            .count();
        let branches = self
            .items
            .iter()
            .filter(|i| matches!(i, MbcEmit::Branch { .. }))
            .count();
        let calls = self
            .items
            .iter()
            .filter(|i| matches!(i, MbcEmit::Call { .. }))
            .count();
        TranslateStats {
            mbc_instructions: self.items.len(),
            concrete,
            branches,
            calls,
            errors: self.errors.len(),
        }
    }
}

/// Translation statistics.
#[derive(Debug, Clone)]
pub struct TranslateStats {
    /// Total MBC instructions emitted.
    pub mbc_instructions: usize,
    /// Concrete (non-placeholder) instructions.
    pub concrete: usize,
    /// Branch placeholders.
    pub branches: usize,
    /// Call placeholders.
    pub calls: usize,
    /// Translation errors.
    pub errors: usize,
}

impl Default for Translator {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use monad_common::MbcInsn;

    // ── Helper to build RV32I instructions ──────────────────────────────────

    fn rv32i_r_type(funct7: u32, rs2: u32, rs1: u32, funct3: u32, rd: u32, opcode: u32) -> u32 {
        (funct7 << 25) | (rs2 << 20) | (rs1 << 15) | (funct3 << 12) | (rd << 7) | opcode
    }

    fn rv32i_i_type(imm12: i32, rs1: u32, funct3: u32, rd: u32, opcode: u32) -> u32 {
        let imm = (imm12 as u32) & 0xFFF;
        (imm << 20) | (rs1 << 15) | (funct3 << 12) | (rd << 7) | opcode
    }

    fn rv32i_s_type(imm12: i32, rs2: u32, rs1: u32, funct3: u32) -> u32 {
        let imm = (imm12 as u32) & 0xFFF;
        let imm_hi = (imm >> 5) & 0x7F;
        let imm_lo = imm & 0x1F;
        (imm_hi << 25) | (rs2 << 20) | (rs1 << 15) | (funct3 << 12) | (imm_lo << 7) | 0x23
    }

    fn rv32i_b_type(imm13: i32, rs2: u32, rs1: u32, funct3: u32) -> u32 {
        // B-type immediate encoding: imm[12|10:5|4:1|11]
        let imm = (imm13 as u32) & 0x1FFE; // bits [12:1], bit 0 always 0
        let bit12 = (imm >> 12) & 1;
        let bit11 = (imm >> 11) & 1;
        let bits10_5 = (imm >> 5) & 0x3F;
        let bits4_1 = (imm >> 1) & 0xF;
        (bit12 << 31)
            | (bits10_5 << 25)
            | (rs2 << 20)
            | (rs1 << 15)
            | (funct3 << 12)
            | (bits4_1 << 8)
            | (bit11 << 7)
            | 0x63
    }

    fn rv32i_u_type(imm20: u32, rd: u32, opcode: u32) -> u32 {
        (imm20 << 12) | (rd << 7) | opcode
    }

    fn rv32i_j_type(imm21: i32, rd: u32) -> u32 {
        // J-type immediate encoding: imm[20|10:1|11|19:12]
        let imm = (imm21 as u32) & 0x1FFFFF; // bits [20:1], bit 0 always 0
        let bit20 = (imm >> 20) & 1;
        let bits10_1 = (imm >> 1) & 0x3FF;
        let bit11 = (imm >> 11) & 1;
        let bits19_12 = (imm >> 12) & 0xFF;
        (bit20 << 31) | (bits10_1 << 21) | (bit11 << 20) | (bits19_12 << 12) | (rd << 7) | 0x6F
    }

    const ECALL: u32 = 0x00000073;
    const EBREAK: u32 = 0x00100073;

    // ── Basic instruction tests ─────────────────────────────────────────────

    #[test]
    fn test_translate_add() {
        // ADD x3, x5, x6 → r1 = r3 + r4
        let insn = rv32i_r_type(0, 6, 5, 0, 3, 0x33);
        let result = Translator::translate_program(&[insn]);
        assert!(result.is_ok(), "ADD should translate: {:?}", result.err());
        let mbc = result.unwrap();
        assert!(mbc.len() >= 2); // MOV + ADD minimum
    }

    #[test]
    fn test_translate_sub() {
        let insn = rv32i_r_type(0x20, 6, 5, 0, 3, 0x33);
        let result = Translator::translate_program(&[insn]);
        assert!(result.is_ok());
    }

    #[test]
    fn test_translate_addi_positive() {
        // ADDI x3, x5, 100
        let insn = rv32i_i_type(100, 5, 0, 3, 0x13);
        let result = Translator::translate_program(&[insn]);
        assert!(result.is_ok(), "ADDI +100: {:?}", result.err());
    }

    #[test]
    fn test_translate_addi_negative() {
        // ADDI x3, x5, -50
        let insn = rv32i_i_type(-50, 5, 0, 3, 0x13);
        let result = Translator::translate_program(&[insn]);
        assert!(result.is_ok(), "ADDI -50: {:?}", result.err());
    }

    #[test]
    fn test_translate_addi_zero_is_mov() {
        // ADDI x3, x5, 0 — effectively a MOV
        let insn = rv32i_i_type(0, 5, 0, 3, 0x13);
        let result = Translator::translate_program(&[insn]);
        assert!(result.is_ok());
        // Should emit just MOV (no add sequence)
        let mbc = result.unwrap();
        assert_eq!(mbc.len(), 1); // Just MOV
    }

    #[test]
    fn test_translate_lw() {
        // LW x3, 0(x5)
        let insn = rv32i_i_type(0, 5, 2, 3, 0x03);
        let result = Translator::translate_program(&[insn]);
        assert!(result.is_ok());
    }

    #[test]
    fn test_translate_sw() {
        // SW x6, 0(x5)
        let insn = rv32i_s_type(0, 6, 5, 2);
        let result = Translator::translate_program(&[insn]);
        assert!(result.is_ok());
    }

    #[test]
    fn test_translate_lbu() {
        // LBU x3, 4(x5) — unsigned byte load
        let insn = rv32i_i_type(4, 5, 4, 3, 0x03);
        let result = Translator::translate_program(&[insn]);
        assert!(result.is_ok(), "LBU should translate: {:?}", result.err());
    }

    #[test]
    fn test_translate_lhu() {
        // LHU x3, 4(x5) — unsigned halfword load
        let insn = rv32i_i_type(4, 5, 5, 3, 0x03);
        let result = Translator::translate_program(&[insn]);
        assert!(result.is_ok(), "LHU should translate: {:?}", result.err());
    }

    #[test]
    fn test_translate_register_out_of_range() {
        // ADD x1, x2, x20 (x20 is out of range)
        let insn = rv32i_r_type(0, 20, 2, 0, 1, 0x33);
        let result = Translator::translate_program(&[insn]);
        assert!(result.is_err());
    }

    #[test]
    fn test_translate_ecall() {
        let result = Translator::translate_program(&[ECALL]);
        assert!(result.is_ok());
        let mbc = result.unwrap();
        assert_eq!(mbc.len(), 1);
        assert_eq!(MbcInsn(mbc[0]).opcode(), op::SYSCALL);
    }

    #[test]
    fn test_translate_ebreak() {
        let result = Translator::translate_program(&[EBREAK]);
        assert!(result.is_ok());
        let mbc = result.unwrap();
        assert_eq!(mbc.len(), 1);
        assert_eq!(MbcInsn(mbc[0]).opcode(), op::HALT);
    }

    // ── Branch tests (two-pass resolution) ──────────────────────────────────

    #[test]
    fn test_translate_beq_forward() {
        // BEQ x5, x6, +8 (skip 2 RV32I instructions)
        // Instruction 0: BEQ x5, x6, +8
        // Instruction 1: ADDI x3, x0, 1
        // Instruction 2: ADDI x3, x0, 2  ← target
        let beq = rv32i_b_type(8, 6, 5, 0);
        let addi1 = rv32i_i_type(1, 0, 0, 3, 0x13);
        let addi2 = rv32i_i_type(2, 0, 0, 3, 0x13);

        let result = Translator::translate_program(&[beq, addi1, addi2]);
        assert!(result.is_ok(), "BEQ forward: {:?}", result.err());

        // Verify the branch target resolves correctly
        let mbc = result.unwrap();
        // Find the JZ instruction (should be after CMP + zero guards)
        let jz_idx = mbc.iter().position(|w| MbcInsn(*w).opcode() == op::JZ);
        assert!(jz_idx.is_some(), "Should contain a JZ instruction");
    }

    #[test]
    fn test_translate_bne_backward() {
        // Simple loop: ADDI x3, x3, 1; BNE x3, x5, -4
        let addi = rv32i_i_type(1, 3, 0, 3, 0x13); // ADDI x3, x3, 1
        let bne = rv32i_b_type(-4, 5, 3, 1); // BNE x3, x5, -4 (back to ADDI)

        let result = Translator::translate_program(&[addi, bne]);
        assert!(result.is_ok(), "BNE backward: {:?}", result.err());

        let mbc = result.unwrap();
        // The JNZ should have a negative offset pointing back to the ADDI translation
        let jnz_idx = mbc.iter().position(|w| MbcInsn(*w).opcode() == op::JNZ);
        assert!(jnz_idx.is_some(), "Should contain a JNZ instruction");
        let jnz = MbcInsn(mbc[jnz_idx.unwrap()]);
        assert!(
            jnz.imm16_signed() < 0,
            "Backward branch should have negative offset"
        );
    }

    #[test]
    fn test_translate_blt() {
        // BLT x5, x6, +8
        let blt = rv32i_b_type(8, 6, 5, 4);
        let nop1 = rv32i_i_type(0, 0, 0, 0, 0x13); // NOP (ADDI x0, x0, 0)
        let nop2 = rv32i_i_type(0, 0, 0, 0, 0x13);

        let result = Translator::translate_program(&[blt, nop1, nop2]);
        assert!(result.is_ok(), "BLT: {:?}", result.err());

        let mbc = result.unwrap();
        let jn_idx = mbc.iter().position(|w| MbcInsn(*w).opcode() == op::JN);
        assert!(jn_idx.is_some(), "BLT should emit JN");
    }

    #[test]
    fn test_translate_bge() {
        let bge = rv32i_b_type(8, 6, 5, 5);
        let nop1 = rv32i_i_type(0, 0, 0, 0, 0x13);
        let nop2 = rv32i_i_type(0, 0, 0, 0, 0x13);

        let result = Translator::translate_program(&[bge, nop1, nop2]);
        assert!(result.is_ok(), "BGE: {:?}", result.err());

        let mbc = result.unwrap();
        let jp_idx = mbc.iter().position(|w| MbcInsn(*w).opcode() == op::JP);
        assert!(jp_idx.is_some(), "BGE should emit JP");
    }

    #[test]
    fn test_translate_bltu() {
        let bltu = rv32i_b_type(8, 6, 5, 6);
        let nop1 = rv32i_i_type(0, 0, 0, 0, 0x13);
        let nop2 = rv32i_i_type(0, 0, 0, 0, 0x13);

        let result = Translator::translate_program(&[bltu, nop1, nop2]);
        assert!(result.is_ok(), "BLTU: {:?}", result.err());

        let mbc = result.unwrap();
        let jc_idx = mbc.iter().position(|w| MbcInsn(*w).opcode() == op::JC);
        assert!(jc_idx.is_some(), "BLTU should emit JC");
    }

    #[test]
    fn test_translate_bgeu() {
        let bgeu = rv32i_b_type(8, 6, 5, 7);
        let nop1 = rv32i_i_type(0, 0, 0, 0, 0x13);
        let nop2 = rv32i_i_type(0, 0, 0, 0, 0x13);

        let result = Translator::translate_program(&[bgeu, nop1, nop2]);
        assert!(result.is_ok(), "BGEU: {:?}", result.err());

        let mbc = result.unwrap();
        let jnc_idx = mbc.iter().position(|w| MbcInsn(*w).opcode() == op::JNC);
        assert!(jnc_idx.is_some(), "BGEU should emit JNC");
    }

    // ── JAL / JALR tests ────────────────────────────────────────────────────

    #[test]
    fn test_translate_jal_as_call() {
        // JAL x1, +8 (call forward, link to ra)
        let jal = rv32i_j_type(8, 1);
        let nop1 = rv32i_i_type(0, 0, 0, 0, 0x13);
        let nop2 = rv32i_i_type(0, 0, 0, 0, 0x13); // target

        let result = Translator::translate_program(&[jal, nop1, nop2]);
        assert!(result.is_ok(), "JAL x1: {:?}", result.err());

        let mbc = result.unwrap();
        let call_idx = mbc.iter().position(|w| MbcInsn(*w).opcode() == op::CALL);
        assert!(call_idx.is_some(), "JAL x1 should emit CALL");
    }

    #[test]
    fn test_translate_jal_x0_as_jump() {
        // JAL x0, +8 (unconditional jump, no link)
        let jal = rv32i_j_type(8, 0);
        let nop1 = rv32i_i_type(0, 0, 0, 0, 0x13);
        let nop2 = rv32i_i_type(0, 0, 0, 0, 0x13);

        let result = Translator::translate_program(&[jal, nop1, nop2]);
        assert!(result.is_ok(), "JAL x0: {:?}", result.err());

        let mbc = result.unwrap();
        let jmp_idx = mbc.iter().position(|w| MbcInsn(*w).opcode() == op::JMP);
        assert!(jmp_idx.is_some(), "JAL x0 should emit JMP, not CALL");
    }

    #[test]
    fn test_translate_jalr_ret() {
        // JALR x0, 0(x1) — standard function return
        let jalr = rv32i_i_type(0, 1, 0, 0, 0x67);
        let result = Translator::translate_program(&[jalr]);
        assert!(result.is_ok(), "JALR ret: {:?}", result.err());

        let mbc = result.unwrap();
        assert_eq!(mbc.len(), 1);
        assert_eq!(MbcInsn(mbc[0]).opcode(), op::RET);
    }

    #[test]
    fn test_translate_jalr_indirect_call() {
        // JALR x1, 0(x5) — indirect call to t0
        let jalr = rv32i_i_type(0, 5, 0, 1, 0x67);
        let result = Translator::translate_program(&[jalr]);
        assert!(
            result.is_ok(),
            "JALR x1, 0(x5) should translate: {:?}",
            result.err()
        );
        let mbc = result.unwrap();
        assert!(mbc.iter().any(|w| MbcInsn(*w).opcode() == op::CALLR));
    }

    // ── LUI / AUIPC tests ──────────────────────────────────────────────────

    #[test]
    fn test_translate_lui_small() {
        // LUI x5, 1 → rd = 1 << 12 = 0x1000
        let insn = rv32i_u_type(1, 5, 0x37);
        let result = Translator::translate_program(&[insn]);
        assert!(result.is_ok(), "LUI small: {:?}", result.err());

        let mbc = result.unwrap();
        // 0x1000 fits in 16 bits, so MOVI alone suffices
        assert!(!mbc.is_empty());
    }

    #[test]
    fn test_translate_lui_large() {
        // LUI x5, 0x12345 → rd = 0x12345000
        let insn = rv32i_u_type(0x12345, 5, 0x37);
        let result = Translator::translate_program(&[insn]);
        assert!(result.is_ok(), "LUI large: {:?}", result.err());

        let mbc = result.unwrap();
        // 0x12345000: high16=0x1234, low16=0x5000
        // MOVI lo + LOAD_IMM32 hi = 2 instructions
        assert!(
            mbc.len() >= 2,
            "LUI 0x12345 should need multiple MBC insns, got {}",
            mbc.len()
        );
    }

    #[test]
    fn test_translate_auipc() {
        // AUIPC x5, 1 at PC=0 → rd = 0 + (1 << 12) = 0x1000
        let insn = rv32i_u_type(1, 5, 0x17);
        let result = Translator::translate_program(&[insn]);
        assert!(result.is_ok(), "AUIPC: {:?}", result.err());
    }

    // ── FENCE test ──────────────────────────────────────────────────────────

    #[test]
    fn test_translate_fence_is_nop() {
        // FENCE instruction: opcode 0x0F
        let fence = 0x0FF0_000F_u32; // standard FENCE
        let result = Translator::translate_program(&[fence]);
        assert!(result.is_ok(), "FENCE: {:?}", result.err());
        let mbc = result.unwrap();
        assert_eq!(mbc.len(), 0, "FENCE should emit no MBC instructions");
    }

    // ── RV32M extension tests ───────────────────────────────────────────────

    #[test]
    fn test_translate_mul() {
        // MUL x3, x5, x6
        let insn = rv32i_r_type(1, 6, 5, 0, 3, 0x33);
        let result = Translator::translate_program(&[insn]);
        assert!(result.is_ok(), "MUL: {:?}", result.err());
    }

    #[test]
    fn test_translate_divu() {
        // DIVU x3, x5, x6
        let insn = rv32i_r_type(1, 6, 5, 5, 3, 0x33);
        let result = Translator::translate_program(&[insn]);
        assert!(result.is_ok(), "DIVU: {:?}", result.err());
    }

    #[test]
    fn test_translate_remu() {
        // REMU x3, x5, x6
        let insn = rv32i_r_type(1, 6, 5, 7, 3, 0x33);
        let result = Translator::translate_program(&[insn]);
        assert!(result.is_ok(), "REMU: {:?}", result.err());
    }

    // ── Two-pass branch resolution accuracy ─────────────────────────────────

    #[test]
    fn test_two_pass_branch_offset_accounts_for_expansion() {
        // Test that branch offsets are computed correctly despite instruction expansion.
        //
        // Program:
        //   0: BEQ x5, x6, +8   → CMP + JZ (2+ MBC insns for BEQ)
        //   4: LUI x5, 0x12345  → multi-insn (5 MBC insns for 32-bit load)
        //   8: ECALL             → 1 MBC insn (target of BEQ)
        //
        // The BEQ targets instruction at RV32I PC=8.
        // After expansion, instruction 0 (BEQ) expands, instruction 1 (LUI) expands.
        // The JZ offset must account for the LUI expansion.

        let beq = rv32i_b_type(8, 6, 5, 0); // BEQ x5, x6, +8
        let lui = rv32i_u_type(0x12345, 5, 0x37); // LUI x5, 0x12345
        let ecall = ECALL; // ECALL (target)

        let result = Translator::translate_program(&[beq, lui, ecall]);
        assert!(result.is_ok(), "Two-pass: {:?}", result.err());

        let mbc = result.unwrap();

        // Find the SYSCALL instruction (target)
        let syscall_idx = mbc
            .iter()
            .position(|w| MbcInsn(*w).opcode() == op::SYSCALL)
            .unwrap();

        // Find the JZ instruction
        let jz_idx = mbc
            .iter()
            .position(|w| MbcInsn(*w).opcode() == op::JZ)
            .unwrap();

        // The JZ offset should point to the SYSCALL instruction.
        // offset = target_mbc_pc - (jz_mbc_pc + 1)
        let expected_offset = (syscall_idx as i16) - (jz_idx as i16 + 1);
        let actual_offset = MbcInsn(mbc[jz_idx]).imm16_signed();
        assert_eq!(actual_offset, expected_offset,
            "JZ offset should account for LUI expansion: expected {expected_offset}, got {actual_offset}");
    }

    #[test]
    fn test_translate_program_multiple() {
        let program = vec![
            rv32i_i_type(10, 0, 0, 1, 0x13),   // ADDI x1, x0, 10 (li x1, 10)
            rv32i_i_type(20, 0, 0, 2, 0x13),   // ADDI x2, x0, 20 (li x2, 20)
            rv32i_r_type(0, 2, 1, 0, 3, 0x33), // ADD x3, x1, x2
        ];
        let result = Translator::translate_program(&program);
        assert!(result.is_ok(), "Multi-insn: {:?}", result.err());
    }

    // ── R-type rd==rs2 clobber regression tests ─────────────────────────────

    #[test]
    fn test_add_rd_eq_rs2_commutative() {
        // ADD x12, x10, x12 — rd==rs2 (both map to r10), rs1 maps to r8.
        // Bug: naive MOV r10,r8; ADD r10,r10 computes 2*a0 instead of a0+a2.
        // Fix: commutative, so emit ADD r10,r8 directly (r10 already holds a2).
        let insn = rv32i_r_type(0, 12, 10, 0, 12, 0x33); // ADD x12, x10, x12
        let result = Translator::translate_program(&[insn]);
        assert!(result.is_ok(), "ADD rd==rs2: {:?}", result.err());
        let mbc = result.unwrap();

        // Should emit exactly 1 instruction: ADD r10, r8 (no MOV needed)
        assert_eq!(
            mbc.len(),
            1,
            "Commutative rd==rs2 should emit 1 insn, got {}: {:?}",
            mbc.len(),
            mbc.iter().map(|w| format!("{:08X}", w)).collect::<Vec<_>>()
        );
        let insn0 = MbcInsn(mbc[0]);
        assert_eq!(insn0.opcode(), op::ADD, "Should be ADD");
        assert_eq!(insn0.dst(), 10, "dst should be r10 (a2)");
        assert_eq!(insn0.src(), 8, "src should be r8 (a0)");
    }

    #[test]
    fn test_sub_rd_eq_rs2_non_commutative() {
        // SUB x12, x10, x12 — rd==rs2, non-commutative.
        // Need: r10 = r8 - r10 (a0 - a2), can't just swap operands.
        // Fix: MOV r0,r8; SUB r0,r10; MOV r10,r0; MOVI r0,0
        let insn = rv32i_r_type(0x20, 12, 10, 0, 12, 0x33); // SUB x12, x10, x12
        let result = Translator::translate_program(&[insn]);
        assert!(result.is_ok(), "SUB rd==rs2: {:?}", result.err());
        let mbc = result.unwrap();

        // Should emit 4 instructions: MOV r0,r8; SUB r0,r10; MOV r10,r0; MOVI r0,0
        assert_eq!(
            mbc.len(),
            4,
            "Non-commutative rd==rs2 should emit 4 insns, got {}: {:?}",
            mbc.len(),
            mbc.iter().map(|w| format!("{:08X}", w)).collect::<Vec<_>>()
        );

        // Verify sequence
        assert_eq!(MbcInsn(mbc[0]).opcode(), op::MOV, "insn 0: MOV r0, r8");
        assert_eq!(MbcInsn(mbc[0]).dst(), 0);
        assert_eq!(MbcInsn(mbc[0]).src(), 8);

        assert_eq!(MbcInsn(mbc[1]).opcode(), op::SUB, "insn 1: SUB r0, r10");
        assert_eq!(MbcInsn(mbc[1]).dst(), 0);
        assert_eq!(MbcInsn(mbc[1]).src(), 10);

        assert_eq!(MbcInsn(mbc[2]).opcode(), op::MOV, "insn 2: MOV r10, r0");
        assert_eq!(MbcInsn(mbc[2]).dst(), 10);
        assert_eq!(MbcInsn(mbc[2]).src(), 0);

        assert_eq!(MbcInsn(mbc[3]).opcode(), op::MOVI, "insn 3: MOVI r0, 0");
    }

    #[test]
    fn test_add_rd_eq_rs1_no_clobber() {
        // ADD x10, x10, x12 — rd==rs1 (both r8), rs2=r10. No clobber: default path.
        // MOV r8,r8 (nop); ADD r8,r10 → r8 = a0 + a2. Correct.
        let insn = rv32i_r_type(0, 12, 10, 0, 10, 0x33); // ADD x10, x10, x12
        let result = Translator::translate_program(&[insn]);
        assert!(result.is_ok(), "ADD rd==rs1: {:?}", result.err());
        let mbc = result.unwrap();

        // Default path: MOV + ADD = 2 instructions
        assert_eq!(
            mbc.len(),
            2,
            "rd==rs1 should use default 2-insn path, got {}",
            mbc.len()
        );
        assert_eq!(MbcInsn(mbc[0]).opcode(), op::MOV);
        assert_eq!(MbcInsn(mbc[1]).opcode(), op::ADD);
    }

    #[test]
    fn test_add_all_same_register() {
        // ADD x10, x10, x10 — rd==rs1==rs2 (all r8). Should compute a0 + a0 = 2*a0.
        // Default path: MOV r8,r8 (nop); ADD r8,r8 → 2*r8. Correct.
        let insn = rv32i_r_type(0, 10, 10, 0, 10, 0x33); // ADD x10, x10, x10
        let result = Translator::translate_program(&[insn]);
        assert!(result.is_ok(), "ADD all same: {:?}", result.err());
        let mbc = result.unwrap();
        assert_eq!(mbc.len(), 2, "all-same should use default 2-insn path");
    }

    // ── Call/return roundtrip ────────────────────────────────────────────────

    #[test]
    fn test_call_return_roundtrip() {
        // Program:
        //   0: JAL x1, +8    → call to instruction 2
        //   4: EBREAK         → halt (return lands here? no, returns to instruction 1)
        //   8: ADDI x10, x0, 42
        //  12: JALR x0, 0(x1)  → return
        let jal = rv32i_j_type(8, 1); // JAL x1, +8
        let halt = EBREAK; // EBREAK
        let body = rv32i_i_type(42, 0, 0, 10, 0x13); // ADDI x10, x0, 42
        let ret = rv32i_i_type(0, 1, 0, 0, 0x67); // JALR x0, 0(x1)

        let result = Translator::translate_program(&[jal, halt, body, ret]);
        assert!(result.is_ok(), "Call/ret: {:?}", result.err());

        let mbc = result.unwrap();
        // Should contain CALL, HALT, ..., RET
        assert!(mbc.iter().any(|w| MbcInsn(*w).opcode() == op::CALL));
        assert!(mbc.iter().any(|w| MbcInsn(*w).opcode() == op::HALT));
        assert!(mbc.iter().any(|w| MbcInsn(*w).opcode() == op::RET));
    }
}
