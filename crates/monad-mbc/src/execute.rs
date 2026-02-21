//! Userspace MBC fetch-decode-execute loop.
//!
//! This mirrors the BPF implementation (monad-cpu-ebpf/src/main.rs) but runs
//! in userspace for testing and validation.

use monad_common::{
    MbcCpuState, MbcInsn, mbc_opcodes as op, mbc_flags as mf, mbc_syscalls as sys,
    mbc_mmap as mmap, REG_SP,
};

/// Execution errors.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum ExecError {
    Halted,
    RomFault,
    InvalidSyscall(u32),
    DivisionByZero,
    CycleBudgetExhausted,
}

pub const MAX_CYCLES_PER_TICK: u32 = 1000;

/// Userspace MBC CPU emulator.
pub struct Cpu {
    pub state: MbcCpuState,
    pub ram: Vec<u32>,   // word-addressed
    pub rom: Vec<u32>,
    pub screen: Vec<u8>, // 320x200 = 64000 bytes
    pub kbd: u32,
    pub ticks_ms: u32,
}

impl Cpu {
    /// Create a new CPU with SP initialized to 0xFFFF_0000.
    pub fn new() -> Self {
        let mut state = MbcCpuState::default();
        state.regs[REG_SP as usize] = 0xFFFF_0000;

        Cpu {
            state,
            ram: vec![0; 0x10000],      // 64K words
            rom: Vec::new(),
            screen: vec![0; 64000],     // 320x200
            kbd: 0,
            ticks_ms: 0,
        }
    }

    /// Load ROM from a slice of u32 words.
    pub fn load_rom(&mut self, rom: &[u32]) {
        self.rom.clear();
        self.rom.extend_from_slice(rom);
    }

    /// Execute a single instruction.
    pub fn step(&mut self) -> Result<(), ExecError> {
        if self.state.halted != 0 {
            return Err(ExecError::Halted);
        }

        // Fetch instruction
        let pc = self.state.pc as usize;
        if pc >= self.rom.len() {
            return Err(ExecError::RomFault);
        }

        let insn_word = self.rom[pc];
        let insn = MbcInsn(insn_word);

        // Advance PC
        self.state.pc = self.state.pc.wrapping_add(1);

        // Decode and execute
        self.execute_insn(insn)?;

        Ok(())
    }

    /// Execute up to max_cycles instructions, returning cycles executed.
    pub fn run(&mut self, max_cycles: u32) -> Result<u32, ExecError> {
        for cycles in 0..max_cycles {
            match self.step() {
                Ok(()) => {}
                Err(ExecError::Halted) => return Ok(cycles + 1),
                Err(e) => return Err(e),
            }
        }
        Err(ExecError::CycleBudgetExhausted)
    }

    /// Execute a single instruction (fetch-decode-execute).
    fn execute_insn(&mut self, insn: MbcInsn) -> Result<(), ExecError> {
        let opcode = insn.opcode();
        let dst = insn.dst() as usize;
        let src = insn.src() as usize;
        let imm = insn.imm16() as u32;

        // For branch instructions: extract 24-bit offset from raw word (bits 23:0)
        // This combines dst(4) + src(4) + imm16(16) = 24 bits
        let branch_raw = insn.0 & 0x00FF_FFFF;
        let branch_offset = if branch_raw & 0x0080_0000 != 0 {
            (branch_raw | 0xFF00_0000) as i32
        } else {
            branch_raw as i32
        };

        match opcode {
            // === ALU operations ===
            op::ADD => {
                let (result, carry) = self.state.regs[dst].overflowing_add(self.state.regs[src]);
                set_flags(self, result, carry);
                self.state.regs[dst] = result;
            }
            op::SUB => {
                let (result, carry) = self.state.regs[dst].overflowing_sub(self.state.regs[src]);
                set_flags(self, result, carry);
                self.state.regs[dst] = result;
            }
            op::MUL => {
                let (result, carry) = self.state.regs[dst].overflowing_mul(self.state.regs[src]);
                set_flags(self, result, carry);
                self.state.regs[dst] = result;
            }
            op::DIV => {
                if self.state.regs[src] == 0 {
                    // Division by zero: saturate to 0xFFFFFFFF
                    self.state.regs[dst] = 0xFFFFFFFF;
                    self.state.flags = mf::C; // Set carry flag
                } else {
                    let result = self.state.regs[dst] / self.state.regs[src];
                    set_flags(self, result, false);
                    self.state.regs[dst] = result;
                }
            }
            op::MOD => {
                if self.state.regs[src] == 0 {
                    self.state.regs[dst] = 0;
                } else {
                    let result = self.state.regs[dst] % self.state.regs[src];
                    set_flags(self, result, false);
                    self.state.regs[dst] = result;
                }
            }
            op::NEG => {
                let (result, carry) = (0u32).overflowing_sub(self.state.regs[dst]);
                set_flags(self, result, carry);
                self.state.regs[dst] = result;
            }
            op::AND => {
                let result = self.state.regs[dst] & self.state.regs[src];
                set_flags(self, result, false);
                self.state.regs[dst] = result;
            }
            op::OR => {
                let result = self.state.regs[dst] | self.state.regs[src];
                set_flags(self, result, false);
                self.state.regs[dst] = result;
            }
            op::XOR => {
                let result = self.state.regs[dst] ^ self.state.regs[src];
                set_flags(self, result, false);
                self.state.regs[dst] = result;
            }
            op::NOT => {
                let result = !self.state.regs[dst];
                set_flags(self, result, false);
                self.state.regs[dst] = result;
            }

            // === Shift operations (immediate) ===
            op::SHL => {
                let shamt = (imm & 0xFF) as u32;
                let result = self.state.regs[dst].wrapping_shl(shamt);
                let carry = if shamt > 0 && shamt <= 32 {
                    (self.state.regs[dst] >> (32 - shamt)) & 1 != 0
                } else {
                    false
                };
                set_flags(self, result, carry);
                self.state.regs[dst] = result;
            }
            op::SHR => {
                let shamt = (imm & 0xFF) as u32;
                let result = self.state.regs[dst].wrapping_shr(shamt);
                let carry = if shamt > 0 && shamt <= 32 {
                    (self.state.regs[dst] >> (shamt - 1)) & 1 != 0
                } else {
                    false
                };
                set_flags(self, result, carry);
                self.state.regs[dst] = result;
            }
            op::SAR => {
                let shamt = (imm & 0xFF) as u32;
                let result = (self.state.regs[dst] as i32).wrapping_shr(shamt) as u32;
                let carry = if shamt > 0 && shamt <= 32 {
                    ((self.state.regs[dst] as i32) >> (shamt - 1)) & 1 != 0
                } else {
                    false
                };
                set_flags(self, result, carry);
                self.state.regs[dst] = result;
            }

            // === Shift operations (register) ===
            op::SHLR => {
                let shamt = (self.state.regs[src] & 0xFF) as u32;
                let result = self.state.regs[dst].wrapping_shl(shamt);
                let carry = if shamt > 0 && shamt <= 32 {
                    (self.state.regs[dst] >> (32 - shamt)) & 1 != 0
                } else {
                    false
                };
                set_flags(self, result, carry);
                self.state.regs[dst] = result;
            }
            op::SHRR => {
                let shamt = (self.state.regs[src] & 0xFF) as u32;
                let result = self.state.regs[dst].wrapping_shr(shamt);
                let carry = if shamt > 0 && shamt <= 32 {
                    (self.state.regs[dst] >> (shamt - 1)) & 1 != 0
                } else {
                    false
                };
                set_flags(self, result, carry);
                self.state.regs[dst] = result;
            }
            op::SARR => {
                let shamt = (self.state.regs[src] & 0xFF) as u32;
                let result = (self.state.regs[dst] as i32).wrapping_shr(shamt) as u32;
                let carry = if shamt > 0 && shamt <= 32 {
                    ((self.state.regs[dst] as i32) >> (shamt - 1)) & 1 != 0
                } else {
                    false
                };
                set_flags(self, result, carry);
                self.state.regs[dst] = result;
            }

            op::MULH => {
                let a = self.state.regs[dst] as u64;
                let b = self.state.regs[src] as u64;
                let result = ((a * b) >> 32) as u32;
                set_flags(self, result, false);
                self.state.regs[dst] = result;
            }

            // === Register operations ===
            op::MOV => {
                self.state.regs[dst] = self.state.regs[src];
            }
            op::MOVI => {
                self.state.regs[dst] = imm;
            }

            // === Compare ===
            op::CMP => {
                let (result, carry) = self.state.regs[dst].overflowing_sub(self.state.regs[src]);
                set_flags(self, result, carry);
            }

            // === Branches ===
            op::JMP => {
                self.state.pc = self.state.pc.wrapping_add(branch_offset as u32);
            }
            op::JZ => {
                if (self.state.flags & mf::Z) != 0 {
                    self.state.pc = self.state.pc.wrapping_add(branch_offset as u32);
                }
            }
            op::JNZ => {
                if (self.state.flags & mf::Z) == 0 {
                    self.state.pc = self.state.pc.wrapping_add(branch_offset as u32);
                }
            }
            op::JN => {
                if (self.state.flags & mf::N) != 0 {
                    self.state.pc = self.state.pc.wrapping_add(branch_offset as u32);
                }
            }
            op::JP => {
                if (self.state.flags & mf::N) == 0 && (self.state.flags & mf::Z) == 0 {
                    self.state.pc = self.state.pc.wrapping_add(branch_offset as u32);
                }
            }
            op::JC => {
                if (self.state.flags & mf::C) != 0 {
                    self.state.pc = self.state.pc.wrapping_add(branch_offset as u32);
                }
            }
            op::JNC => {
                if (self.state.flags & mf::C) == 0 {
                    self.state.pc = self.state.pc.wrapping_add(branch_offset as u32);
                }
            }

            // === Call/Return ===
            op::CALL => {
                let target = branch_raw;
                // Push return address (PC is already advanced)
                let sp = self.state.regs[REG_SP as usize] as usize;
                if sp < self.ram.len() {
                    self.ram[sp] = self.state.pc;
                    self.state.regs[REG_SP as usize] = self.state.regs[REG_SP as usize].wrapping_sub(1);
                }
                self.state.pc = target;
            }
            op::RET => {
                let sp = self.state.regs[REG_SP as usize].wrapping_add(1) as usize;
                if sp < self.ram.len() {
                    self.state.pc = self.ram[sp];
                    self.state.regs[REG_SP as usize] = sp as u32;
                }
            }
            op::JMPR => {
                self.state.pc = self.state.regs[src];
            }
            op::CALLR => {
                let sp = self.state.regs[REG_SP as usize] as usize;
                if sp < self.ram.len() {
                    self.ram[sp] = self.state.pc;
                    self.state.regs[REG_SP as usize] = self.state.regs[REG_SP as usize].wrapping_sub(1);
                }
                self.state.pc = self.state.regs[src];
            }

            // === Memory operations ===
            op::LD => {
                let addr = self.state.regs[src].wrapping_add(imm as i16 as u32) as usize;
                let word_addr = addr >> 2;
                if word_addr < self.ram.len() {
                    self.state.regs[dst] = self.ram[word_addr];
                }
            }
            op::ST => {
                let addr = self.state.regs[dst].wrapping_add(imm as i16 as u32) as usize;
                // Check if this is a screen write
                if addr >= mmap::SCREEN_BASE as usize && addr < (mmap::SCREEN_BASE + mmap::SCREEN_SIZE) as usize {
                    let screen_offset = (addr - mmap::SCREEN_BASE as usize) as usize;
                    if screen_offset < self.screen.len() {
                        self.screen[screen_offset] = self.state.regs[src] as u8;
                    }
                } else {
                    let word_addr = addr >> 2;
                    if word_addr < self.ram.len() {
                        self.ram[word_addr] = self.state.regs[src];
                    }
                }
            }
            op::LDB => {
                let addr = self.state.regs[src].wrapping_add(imm as i16 as u32) as usize;
                let byte_offset = addr & 3;
                let word_addr = addr >> 2;
                if word_addr < self.ram.len() {
                    let word = self.ram[word_addr];
                    let byte = ((word >> (byte_offset * 8)) & 0xFF) as u32;
                    self.state.regs[dst] = byte;
                }
            }
            op::STB => {
                let addr = self.state.regs[dst].wrapping_add(imm as i16 as u32) as usize;
                // Check if this is a screen write
                if addr >= mmap::SCREEN_BASE as usize && addr < (mmap::SCREEN_BASE + mmap::SCREEN_SIZE) as usize {
                    let screen_offset = (addr - mmap::SCREEN_BASE as usize) as usize;
                    if screen_offset < self.screen.len() {
                        self.screen[screen_offset] = self.state.regs[src] as u8;
                    }
                } else {
                    let byte_offset = addr & 3;
                    let word_addr = addr >> 2;
                    if word_addr < self.ram.len() {
                        let mask = 0xFF << (byte_offset * 8);
                        self.ram[word_addr] = (self.ram[word_addr] & !mask)
                            | ((self.state.regs[src] & 0xFF) << (byte_offset * 8));
                    }
                }
            }
            op::LDH => {
                let addr = self.state.regs[src].wrapping_add(imm as i16 as u32) as usize;
                let halfword_offset = (addr >> 1) & 1;
                let word_addr = addr >> 2;
                if word_addr < self.ram.len() {
                    let word = self.ram[word_addr];
                    let halfword = ((word >> (halfword_offset * 16)) & 0xFFFF) as u32;
                    self.state.regs[dst] = halfword;
                }
            }
            op::STH => {
                let addr = self.state.regs[dst].wrapping_add(imm as i16 as u32) as usize;
                // Check if this is a screen write
                if addr >= mmap::SCREEN_BASE as usize && addr < (mmap::SCREEN_BASE + mmap::SCREEN_SIZE) as usize {
                    let screen_offset = (addr - mmap::SCREEN_BASE as usize) as usize;
                    if screen_offset + 1 < self.screen.len() {
                        self.screen[screen_offset] = (self.state.regs[src] & 0xFF) as u8;
                        self.screen[screen_offset + 1] = ((self.state.regs[src] >> 8) & 0xFF) as u8;
                    }
                } else {
                    let halfword_offset = (addr >> 1) & 1;
                    let word_addr = addr >> 2;
                    if word_addr < self.ram.len() {
                        let mask = 0xFFFF << (halfword_offset * 16);
                        self.ram[word_addr] = (self.ram[word_addr] & !mask)
                            | ((self.state.regs[src] & 0xFFFF) << (halfword_offset * 16));
                    }
                }
            }

            // === System calls ===
            op::SYSCALL => {
                let syscall_id = self.state.regs[0];
                match syscall_id {
                    sys::SYS_DRAW_FRAME => {
                        // In userspace, we just acknowledge it
                    }
                    sys::SYS_GET_KEY => {
                        self.state.regs[0] = self.kbd;
                    }
                    sys::SYS_GET_TICKS => {
                        self.state.regs[0] = self.ticks_ms;
                    }
                    sys::SYS_SLEEP => {
                        let ms = self.state.regs[1];
                        self.ticks_ms = self.ticks_ms.wrapping_add(ms);
                    }
                    _ => return Err(ExecError::InvalidSyscall(syscall_id)),
                }
            }

            // === Halt ===
            op::HALT => {
                self.state.halted = 1;
                return Err(ExecError::Halted);
            }

            _ => {
                // Unknown opcode; treat as NOP or error
            }
        }

        Ok(())
    }
}

/// Set Z, N, C flags based on result and carry.
fn set_flags(cpu: &mut Cpu, result: u32, carry: bool) {
    cpu.state.flags = 0;
    if result == 0 {
        cpu.state.flags |= mf::Z;
    }
    if (result as i32) < 0 {
        cpu.state.flags |= mf::N;
    }
    if carry {
        cpu.state.flags |= mf::C;
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // Helper: encode ALU instruction (opcode, dst, src, imm=0)
    fn alu(opcode: u8, dst: u8, src: u8) -> MbcInsn {
        MbcInsn::encode(opcode, dst, src, 0)
    }

    // Helper: encode shift instruction (opcode, dst, imm as shift amount)
    fn shift(opcode: u8, dst: u8, shamt: u16) -> MbcInsn {
        MbcInsn::encode(opcode, dst, 0, shamt)
    }

    // Helper: encode branch instruction with 24-bit offset in lower bits.
    // The 24-bit offset is spread across dst(4) + src(4) + imm16(16).
    fn branch(opcode: u8, offset: u32) -> MbcInsn {
        let off24 = offset & 0x00FF_FFFF;
        let dst = ((off24 >> 20) & 0xF) as u8;
        let src = ((off24 >> 16) & 0xF) as u8;
        let imm = (off24 & 0xFFFF) as u16;
        MbcInsn::encode(opcode, dst, src, imm)
    }

    // Helper: encode immediate instruction
    fn imm(opcode: u8, dst: u8, value: u16) -> MbcInsn {
        MbcInsn::encode(opcode, dst, 0, value)
    }

    // Helper: encode memory instruction (opcode, dst, src, offset)
    fn mem(opcode: u8, dst: u8, src: u8, offset: u16) -> MbcInsn {
        MbcInsn::encode(opcode, dst, src, offset)
    }

    #[test]
    fn test_add() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 10;
        cpu.state.regs[1] = 20;
        cpu.execute_insn(alu(op::ADD, 0, 1)).unwrap();
        assert_eq!(cpu.state.regs[0], 30);
    }

    #[test]
    fn test_add_overflow() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 0xFFFFFFFF;
        cpu.state.regs[1] = 1;
        cpu.execute_insn(alu(op::ADD, 0, 1)).unwrap();
        assert_eq!(cpu.state.regs[0], 0);
        assert!(cpu.state.flags & mf::C != 0);
    }

    #[test]
    fn test_sub() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 30;
        cpu.state.regs[1] = 10;
        cpu.execute_insn(alu(op::SUB, 0, 1)).unwrap();
        assert_eq!(cpu.state.regs[0], 20);
    }

    #[test]
    fn test_mul() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 6;
        cpu.state.regs[1] = 7;
        cpu.execute_insn(alu(op::MUL, 0, 1)).unwrap();
        assert_eq!(cpu.state.regs[0], 42);
    }

    #[test]
    fn test_div() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 20;
        cpu.state.regs[1] = 4;
        cpu.execute_insn(alu(op::DIV, 0, 1)).unwrap();
        assert_eq!(cpu.state.regs[0], 5);
    }

    #[test]
    fn test_div_by_zero() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 10;
        cpu.state.regs[1] = 0;
        cpu.execute_insn(alu(op::DIV, 0, 1)).unwrap();
        assert_eq!(cpu.state.regs[0], 0xFFFFFFFF);
        assert!(cpu.state.flags & mf::C != 0);
    }

    #[test]
    fn test_mod() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 17;
        cpu.state.regs[1] = 5;
        cpu.execute_insn(alu(op::MOD, 0, 1)).unwrap();
        assert_eq!(cpu.state.regs[0], 2);
    }

    #[test]
    fn test_neg() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 5;
        cpu.execute_insn(alu(op::NEG, 0, 0)).unwrap();
        assert_eq!(cpu.state.regs[0], u32::wrapping_neg(5));
    }

    #[test]
    fn test_and() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 0b1010;
        cpu.state.regs[1] = 0b1100;
        cpu.execute_insn(alu(op::AND, 0, 1)).unwrap();
        assert_eq!(cpu.state.regs[0], 0b1000);
    }

    #[test]
    fn test_or() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 0b1010;
        cpu.state.regs[1] = 0b1100;
        cpu.execute_insn(alu(op::OR, 0, 1)).unwrap();
        assert_eq!(cpu.state.regs[0], 0b1110);
    }

    #[test]
    fn test_xor() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 0b1010;
        cpu.state.regs[1] = 0b1100;
        cpu.execute_insn(alu(op::XOR, 0, 1)).unwrap();
        assert_eq!(cpu.state.regs[0], 0b0110);
    }

    #[test]
    fn test_not() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 0;
        cpu.execute_insn(alu(op::NOT, 0, 0)).unwrap();
        assert_eq!(cpu.state.regs[0], 0xFFFFFFFF);
    }

    #[test]
    fn test_shl_by_0() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 0x12345678;
        cpu.execute_insn(shift(op::SHL, 0, 0)).unwrap();
        assert_eq!(cpu.state.regs[0], 0x12345678);
    }

    #[test]
    fn test_shl_by_1() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 0x80000000;
        cpu.execute_insn(shift(op::SHL, 0, 1)).unwrap();
        assert_eq!(cpu.state.regs[0], 0);
        assert!(cpu.state.flags & mf::C != 0);
    }

    #[test]
    fn test_shr() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 0x80000000;
        cpu.execute_insn(shift(op::SHR, 0, 1)).unwrap();
        assert_eq!(cpu.state.regs[0], 0x40000000);
    }

    #[test]
    fn test_sar_sign_propagation() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 0x80000000u32;
        cpu.execute_insn(shift(op::SAR, 0, 1)).unwrap();
        assert_eq!(cpu.state.regs[0], 0xC0000000);
    }

    #[test]
    fn test_shlr() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 1;
        cpu.state.regs[1] = 3;
        cpu.execute_insn(alu(op::SHLR, 0, 1)).unwrap();
        assert_eq!(cpu.state.regs[0], 8);
    }

    #[test]
    fn test_shrr() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 0x80000000;
        cpu.state.regs[1] = 1;
        cpu.execute_insn(alu(op::SHRR, 0, 1)).unwrap();
        assert_eq!(cpu.state.regs[0], 0x40000000);
    }

    #[test]
    fn test_sarr() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 0x80000000u32;
        cpu.state.regs[1] = 1;
        cpu.execute_insn(alu(op::SARR, 0, 1)).unwrap();
        assert_eq!(cpu.state.regs[0], 0xC0000000);
    }

    #[test]
    fn test_mulh() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 0x80000000;
        cpu.state.regs[1] = 2;
        cpu.execute_insn(alu(op::MULH, 0, 1)).unwrap();
        // 0x80000000 * 2 = 0x1_00000000, high word = 1
        assert_eq!(cpu.state.regs[0], 1);
    }

    #[test]
    fn test_mov() {
        let mut cpu = Cpu::new();
        cpu.state.regs[1] = 42;
        cpu.execute_insn(alu(op::MOV, 0, 1)).unwrap();
        assert_eq!(cpu.state.regs[0], 42);
    }

    #[test]
    fn test_movi() {
        let mut cpu = Cpu::new();
        cpu.execute_insn(imm(op::MOVI, 0, 12345)).unwrap();
        assert_eq!(cpu.state.regs[0], 12345);
    }

    #[test]
    fn test_cmp_equal() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 10;
        cpu.state.regs[1] = 10;
        cpu.execute_insn(alu(op::CMP, 0, 1)).unwrap();
        assert!(cpu.state.flags & mf::Z != 0);
    }

    #[test]
    fn test_cmp_not_equal() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 10;
        cpu.state.regs[1] = 5;
        cpu.execute_insn(alu(op::CMP, 0, 1)).unwrap();
        assert_eq!(cpu.state.flags & mf::Z, 0);
    }

    #[test]
    fn test_jmp() {
        let mut cpu = Cpu::new();
        cpu.state.pc = 0;
        cpu.rom.extend_from_slice(&[0; 100]);
        cpu.execute_insn(branch(op::JMP, 10)).unwrap();
        assert_eq!(cpu.state.pc, 11); // PC already incremented, then jumped +10
    }

    #[test]
    fn test_jmp_backward() {
        let mut cpu = Cpu::new();
        cpu.state.pc = 50;
        cpu.rom.extend_from_slice(&[0; 100]);
        // -16 as 24-bit two's complement: 0xFFFFF0
        cpu.execute_insn(branch(op::JMP, 0xFFFFF0)).unwrap();
        assert_eq!(cpu.state.pc as i32, 51i32 - 16);
    }

    #[test]
    fn test_jz_taken() {
        let mut cpu = Cpu::new();
        cpu.state.pc = 0;
        cpu.state.flags = mf::Z;
        cpu.rom.extend_from_slice(&[0; 100]);
        cpu.execute_insn(branch(op::JZ, 5)).unwrap();
        assert_eq!(cpu.state.pc, 6); // 1 (post-increment) + 5 (jump offset)
    }

    #[test]
    fn test_jz_not_taken() {
        let mut cpu = Cpu::new();
        cpu.state.pc = 0;
        cpu.state.flags = 0;
        cpu.rom.extend_from_slice(&[0; 100]);
        cpu.execute_insn(branch(op::JZ, 5)).unwrap();
        assert_eq!(cpu.state.pc, 1); // PC just incremented
    }

    #[test]
    fn test_jnz_taken() {
        let mut cpu = Cpu::new();
        cpu.state.pc = 0;
        cpu.state.flags = 0;
        cpu.rom.extend_from_slice(&[0; 100]);
        cpu.execute_insn(branch(op::JNZ, 5)).unwrap();
        assert_eq!(cpu.state.pc, 6);
    }

    #[test]
    fn test_jn_taken() {
        let mut cpu = Cpu::new();
        cpu.state.pc = 0;
        cpu.state.flags = mf::N;
        cpu.rom.extend_from_slice(&[0; 100]);
        cpu.execute_insn(branch(op::JN, 5)).unwrap();
        assert_eq!(cpu.state.pc, 6);
    }

    #[test]
    fn test_jp_taken() {
        let mut cpu = Cpu::new();
        cpu.state.pc = 0;
        cpu.state.flags = 0; // Neither N nor Z
        cpu.rom.extend_from_slice(&[0; 100]);
        cpu.execute_insn(branch(op::JP, 5)).unwrap();
        assert_eq!(cpu.state.pc, 6);
    }

    #[test]
    fn test_jc_taken() {
        let mut cpu = Cpu::new();
        cpu.state.pc = 0;
        cpu.state.flags = mf::C;
        cpu.rom.extend_from_slice(&[0; 100]);
        cpu.execute_insn(branch(op::JC, 5)).unwrap();
        assert_eq!(cpu.state.pc, 6);
    }

    #[test]
    fn test_jnc_taken() {
        let mut cpu = Cpu::new();
        cpu.state.pc = 0;
        cpu.state.flags = 0;
        cpu.rom.extend_from_slice(&[0; 100]);
        cpu.execute_insn(branch(op::JNC, 5)).unwrap();
        assert_eq!(cpu.state.pc, 6);
    }

    #[test]
    fn test_call_ret() {
        let mut cpu = Cpu::new();
        cpu.rom.extend_from_slice(&[0; 100]);
        cpu.state.pc = 10;
        cpu.state.regs[REG_SP as usize] = 0x1000;

        // CALL to address 50
        cpu.execute_insn(branch(op::CALL, 50)).unwrap();
        assert_eq!(cpu.state.pc, 50);
        assert_eq!(cpu.state.regs[REG_SP as usize], 0xFFF);
        assert_eq!(cpu.ram[0x1000], 11); // Return address pushed

        // RET
        cpu.execute_insn(alu(op::RET, 0, 0)).unwrap();
        assert_eq!(cpu.state.pc, 11);
        assert_eq!(cpu.state.regs[REG_SP as usize], 0x1000);
    }

    #[test]
    fn test_jmpr() {
        let mut cpu = Cpu::new();
        cpu.rom.extend_from_slice(&[0; 100]);
        cpu.state.pc = 0;
        cpu.state.regs[1] = 42;

        cpu.execute_insn(alu(op::JMPR, 0, 1)).unwrap();
        assert_eq!(cpu.state.pc, 42);
    }

    #[test]
    fn test_callr() {
        let mut cpu = Cpu::new();
        cpu.rom.extend_from_slice(&[0; 100]);
        cpu.state.pc = 10;
        cpu.state.regs[REG_SP as usize] = 0x1000;
        cpu.state.regs[1] = 50;

        cpu.execute_insn(alu(op::CALLR, 0, 1)).unwrap();
        assert_eq!(cpu.state.pc, 50);
        assert_eq!(cpu.ram[0x1000], 11);
    }

    #[test]
    fn test_ld_st() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 100;
        cpu.state.regs[1] = 0x100; // byte address

        // ST: ram[0x100>>2] = 100
        cpu.execute_insn(mem(op::ST, 1, 0, 0)).unwrap();
        assert_eq!(cpu.ram[0x100 >> 2], 100);

        // LD: r0 = ram[0x100>>2]
        cpu.state.regs[0] = 0;
        cpu.execute_insn(mem(op::LD, 0, 1, 0)).unwrap();
        assert_eq!(cpu.state.regs[0], 100);
    }

    #[test]
    fn test_ldb_stb() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 0xAB;
        cpu.state.regs[1] = 0x100;

        // STB: store byte
        cpu.execute_insn(mem(op::STB, 1, 0, 0)).unwrap();

        // LDB: load byte
        cpu.state.regs[0] = 0;
        cpu.execute_insn(mem(op::LDB, 0, 1, 0)).unwrap();
        assert_eq!(cpu.state.regs[0], 0xAB);
    }

    #[test]
    fn test_ldh_sth() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 0xABCD;
        cpu.state.regs[1] = 0x100;

        // STH: store halfword
        cpu.execute_insn(mem(op::STH, 1, 0, 0)).unwrap();

        // LDH: load halfword
        cpu.state.regs[0] = 0;
        cpu.execute_insn(mem(op::LDH, 0, 1, 0)).unwrap();
        assert_eq!(cpu.state.regs[0], 0xABCD);
    }

    #[test]
    fn test_halt() {
        let mut cpu = Cpu::new();
        cpu.rom.extend_from_slice(&[0; 10]);
        let result = cpu.execute_insn(alu(op::HALT, 0, 0));
        assert_eq!(result, Err(ExecError::Halted));
        assert_eq!(cpu.state.halted, 1);
    }

    #[test]
    fn test_syscall_get_ticks() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = sys::SYS_GET_TICKS;
        cpu.ticks_ms = 100;

        cpu.execute_insn(alu(op::SYSCALL, 0, 0)).unwrap();
        assert_eq!(cpu.state.regs[0], 100);
    }

    #[test]
    fn test_syscall_sleep() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = sys::SYS_SLEEP;
        cpu.state.regs[1] = 50;
        cpu.ticks_ms = 100;

        cpu.execute_insn(alu(op::SYSCALL, 0, 0)).unwrap();
        assert_eq!(cpu.ticks_ms, 150);
    }

    #[test]
    fn test_cycle_budget_exhaustion() {
        let mut cpu = Cpu::new();
        // Fill ROM with NOPs (unknown opcodes treated as NOP)
        cpu.rom.extend_from_slice(&[0; 10]);
        let result = cpu.run(5);
        assert_eq!(result, Err(ExecError::CycleBudgetExhausted));
    }

    #[test]
    fn test_sum_1_to_10_program() {
        // Program: sum 1 + 2 + ... + 10
        // r0 = sum, r1 = counter, r2 = max
        let mut cpu = Cpu::new();

        let program = vec![
            MbcInsn::encode(op::MOVI, 0, 0, 0).0,       // MOVI r0, 0
            MbcInsn::encode(op::MOVI, 1, 0, 1).0,       // MOVI r1, 1
            MbcInsn::encode(op::MOVI, 2, 0, 10).0,      // MOVI r2, 10
            MbcInsn::encode(op::ADD, 0, 1, 0).0,         // ADD r0, r1
            MbcInsn::encode(op::MOVI, 3, 0, 1).0,       // MOVI r3, 1
            MbcInsn::encode(op::ADD, 1, 3, 0).0,         // ADD r1, r3
            MbcInsn::encode(op::CMP, 1, 2, 0).0,         // CMP r1, r2
            // JNZ -4: 24-bit two's complement = 0xFFFFFC, packed across dst+src+imm16
            branch(op::JNZ, 0xFFFFFC).0,                 // JNZ -4
            MbcInsn::encode(op::HALT, 0, 0, 0).0,        // HALT
        ];

        cpu.load_rom(&program);
        let result = cpu.run(1000);

        assert!(result.is_ok() || result == Err(ExecError::Halted));
        // Sum of 1..10 = 55
        assert_eq!(cpu.state.regs[0], 55);
    }
}
