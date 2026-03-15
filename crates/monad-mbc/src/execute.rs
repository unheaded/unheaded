//! Userspace MBC fetch-decode-execute loop.
//!
//! This mirrors the BPF implementation (monad-cpu-ebpf/src/main.rs) but runs
//! in userspace for testing and validation.

use monad_common::{
    MbcCpuState, MbcInsn, mbc_block as blk, mbc_opcodes as op, mbc_flags as mf,
    mbc_syscalls as sys, mbc_linux_syscalls as lsys, mbc_interrupts as intr,
    mbc_mmap as mmap, mbc_mmu as mmu, REG_SP,
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
    /// TTY output buffer — collects SYS_WRITE output to fd 1/2 (Level 4f).
    pub tty_output: Vec<u8>,
    /// Instance ID for SYS_GETPID (defaults to 0).
    pub instance_id: u32,
    /// Process table for Level 4c scheduler: 4 slots of 20 u32s each.
    /// Layout per slot: [r0..r15, PC, flags, SP_copy, program_break]
    pub proc_table: [[u32; 20]; 4],
    /// Halted process bitmask (bit i set = process i has exited).
    pub halted_mask: u32,
    /// Software TLB: 64 entries, direct-mapped. Each entry: [vpn, pfn, flags].
    pub tlb: [[u32; 3]; 64],
}

impl Cpu {
    /// Create a new CPU with SP initialized to 0xFFFF_0000.
    pub fn new() -> Self {
        let mut state = MbcCpuState::default();
        state.regs[REG_SP as usize] = 0xFFFF_0000;

        Cpu {
            state,
            ram: vec![0; 0x300000],     // 3M words — covers ramdisk at 0x200000 + 1M words
            rom: Vec::new(),
            screen: vec![0; 64000],     // 320x200
            kbd: 0,
            ticks_ms: 0,
            tty_output: Vec::new(),
            instance_id: 0,
            proc_table: [[0u32; 20]; 4],
            halted_mask: 0,
            tlb: [[0u32; 3]; 64],
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

        // Interrupt dispatch: if an interrupt is pending and enabled, service it
        // before fetching the next instruction.
        if self.state.interrupt_pending != 0 && self.state.interrupts_enabled != 0 {
            // Push flags to stack
            self.state.regs[REG_SP as usize] = self.state.regs[REG_SP as usize].wrapping_sub(4);
            let sp_flags = (self.state.regs[REG_SP as usize] >> 2) as usize;
            if sp_flags < self.ram.len() {
                self.ram[sp_flags] = self.state.flags as u32;
            }
            // Push PC to stack
            self.state.regs[REG_SP as usize] = self.state.regs[REG_SP as usize].wrapping_sub(4);
            let sp_pc = (self.state.regs[REG_SP as usize] >> 2) as usize;
            if sp_pc < self.ram.len() {
                self.ram[sp_pc] = self.state.pc;
            }
            // Disable interrupts
            self.state.interrupts_enabled = 0;
            // Jump to IVT handler
            let ivt_addr = ((self.state.interrupt_vector as u32) * 4) >> 2;
            self.state.pc = self.mem_read_word(ivt_addr as usize);
            // Clear pending
            self.state.interrupt_pending = 0;
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

        // Increment instruction counter (matches BPF insn_count tracking)
        self.state.insn_count = self.state.insn_count.wrapping_add(1);

        Ok(())
    }

    // ── MMU / address translation (Level 4d) ────────────────────────────────

    /// Translate a virtual address to a physical address using the software TLB
    /// and two-level page table walk. Returns the address unchanged when MMU is
    /// disabled (flat addressing mode / Doom compatibility).
    fn translate_address(&mut self, vaddr: u32) -> u32 {
        if self.state.mmu_enabled == 0 {
            return vaddr;
        }

        let vpn = vaddr >> 12;
        let offset = vaddr & 0xFFF;
        let tlb_idx = (vpn & 63) as usize;

        // Check TLB
        let entry = self.tlb[tlb_idx];
        if entry[0] == vpn && (entry[2] & mmu::PTE_PRESENT) != 0 {
            // TLB hit
            return (entry[1] << 12) | offset;
        }

        // TLB miss — walk page tables
        let pde_idx = (vaddr >> 22) as usize;
        let pte_idx = ((vaddr >> 12) & 0x3FF) as usize;

        // Read page directory entry
        let pd_word_addr = (self.state.page_dir_base >> 2) as usize + pde_idx;
        let pde = self.mem_read_word(pd_word_addr);
        if (pde & mmu::PTE_PRESENT) == 0 {
            return vaddr; // Page fault — return unmapped
        }

        // Read page table entry
        let pt_base = pde & mmu::PTE_PFN_MASK;
        let pt_word_addr = (pt_base >> 2) as usize + pte_idx;
        let pte = self.mem_read_word(pt_word_addr);
        if (pte & mmu::PTE_PRESENT) == 0 {
            return vaddr; // Page fault
        }

        let pfn = (pte & mmu::PTE_PFN_MASK) >> 12;

        // Update TLB
        self.tlb[tlb_idx] = [vpn, pfn, pte];

        (pfn << 12) | offset
    }

    /// Flush all TLB entries (called by SYS_FLUSH_TLB).
    fn flush_tlb(&mut self) {
        self.tlb = [[0u32; 3]; 64];
    }

    // ── Memory helpers (mirror BPF mem_read_word / mem_write_word) ───────────

    /// Read a 32-bit word from MBC address space.
    /// Handles KBD register and screen region projection, matching BPF behavior.
    fn mem_read_word(&self, word_addr: usize) -> u32 {
        // KBD register: word address 0xFFFF >> 2 = 0x3FFF
        let kbd_word = (mmap::KBD_ADDR >> 2) as usize;
        if word_addr == kbd_word {
            return self.kbd;
        }
        // Screen region projection
        let screen_word_start = (mmap::SCREEN_BASE >> 2) as usize;
        let screen_word_end = ((mmap::SCREEN_BASE + mmap::SCREEN_SIZE + 3) >> 2) as usize;
        if word_addr >= screen_word_start && word_addr < screen_word_end {
            let pixel_base = (word_addr - screen_word_start) * 4;
            let mut val = 0u32;
            for i in 0..4 {
                if pixel_base + i < self.screen.len() {
                    val |= (self.screen[pixel_base + i] as u32) << (i * 8);
                }
            }
            return val;
        }
        if word_addr < self.ram.len() {
            self.ram[word_addr]
        } else {
            0
        }
    }

    /// Read a single byte from MBC address space.
    fn mem_read_byte(&self, byte_addr: usize) -> u8 {
        // Screen region: direct byte access
        if byte_addr >= mmap::SCREEN_BASE as usize
            && byte_addr < (mmap::SCREEN_BASE + mmap::SCREEN_SIZE) as usize
        {
            let px = byte_addr - mmap::SCREEN_BASE as usize;
            return if px < self.screen.len() { self.screen[px] } else { 0 };
        }
        let word_addr = byte_addr >> 2;
        let byte_shift = (byte_addr & 3) * 8;
        let word = self.mem_read_word(word_addr);
        ((word >> byte_shift) & 0xFF) as u8
    }

    /// Read a 16-bit halfword from MBC address space (little-endian).
    fn mem_read_half(&self, byte_addr: usize) -> u16 {
        let word_addr = byte_addr >> 2;
        let half_shift = (byte_addr & 2) * 8;
        let word = self.mem_read_word(word_addr);
        ((word >> half_shift) & 0xFFFF) as u16
    }

    /// Write a 32-bit word to MBC address space. Handles screen projection.
    fn mem_write_word(&mut self, word_addr: usize, value: u32) {
        let screen_word_start = (mmap::SCREEN_BASE >> 2) as usize;
        let screen_word_end = ((mmap::SCREEN_BASE + mmap::SCREEN_SIZE + 3) >> 2) as usize;
        if word_addr >= screen_word_start && word_addr < screen_word_end {
            let pixel_base = (word_addr - screen_word_start) * 4;
            let bytes = value.to_le_bytes();
            for (i, &b) in bytes.iter().enumerate() {
                if pixel_base + i < self.screen.len() {
                    self.screen[pixel_base + i] = b;
                }
            }
            return;
        }
        if word_addr < self.ram.len() {
            self.ram[word_addr] = value;
        }
    }

    /// Write a single byte to MBC address space.
    fn mem_write_byte(&mut self, byte_addr: usize, value: u8) {
        if byte_addr >= mmap::SCREEN_BASE as usize
            && byte_addr < (mmap::SCREEN_BASE + mmap::SCREEN_SIZE) as usize
        {
            let px = byte_addr - mmap::SCREEN_BASE as usize;
            if px < self.screen.len() {
                self.screen[px] = value;
            }
            return;
        }
        let word_addr = byte_addr >> 2;
        let byte_shift = (byte_addr & 3) * 8;
        let old = self.mem_read_word(word_addr);
        let new = (old & !(0xFF << byte_shift)) | ((value as u32) << byte_shift);
        if word_addr < self.ram.len() {
            self.ram[word_addr] = new;
        }
    }

    /// Write a 16-bit halfword to MBC address space (little-endian).
    fn mem_write_half(&mut self, byte_addr: usize, value: u16) {
        let word_addr = byte_addr >> 2;
        let half_shift = (byte_addr & 2) * 8;
        let old = self.mem_read_word(word_addr);
        let new = (old & !(0xFFFF << half_shift)) | ((value as u32) << half_shift);
        self.mem_write_word(word_addr, new);
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
            // === No-op ===
            op::NOP => {
                // No operation — just advance PC (already done by step())
            }

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

            // === Stack operations ===
            op::PUSH => {
                let sp = self.state.regs[REG_SP as usize] as usize;
                if sp < self.ram.len() {
                    self.ram[sp] = self.state.regs[dst];
                    self.state.regs[REG_SP as usize] = self.state.regs[REG_SP as usize].wrapping_sub(1);
                }
            }
            op::POP => {
                let sp = self.state.regs[REG_SP as usize].wrapping_add(1) as usize;
                if sp < self.ram.len() {
                    self.state.regs[dst] = self.ram[sp];
                    self.state.regs[REG_SP as usize] = sp as u32;
                }
            }

            // === Extended immediate ===
            op::LOAD_IMM32 => {
                // Load upper 16 bits of dst, preserving lower 16 bits.
                // regs[dst] = (imm16 << 16) | (regs[dst] & 0xFFFF)
                self.state.regs[dst] = (imm << 16) | (self.state.regs[dst] & 0xFFFF);
            }
            op::ADDI => {
                // dst = dst + sign_extend(imm16)
                let sext = imm as i16 as i32 as u32;
                let (result, carry) = self.state.regs[dst].overflowing_add(sext);
                set_flags(self, result, carry);
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

            // === Memory operations (using mem helpers for screen/KBD projection) ===
            // All addresses go through translate_address() for MMU support (Level 4d).
            // When mmu_enabled == 0, translate_address returns address unchanged.
            op::LD => {
                let raw_addr = self.state.regs[src].wrapping_add(imm as i16 as u32);
                let addr = self.translate_address(raw_addr) as usize;
                self.state.regs[dst] = self.mem_read_word(addr >> 2);
            }
            op::ST => {
                let raw_addr = self.state.regs[dst].wrapping_add(imm as i16 as u32);
                let addr = self.translate_address(raw_addr) as usize;
                self.mem_write_word(addr >> 2, self.state.regs[src]);
            }
            op::LDB => {
                let raw_addr = self.state.regs[src].wrapping_add(imm as i16 as u32);
                let addr = self.translate_address(raw_addr) as usize;
                self.state.regs[dst] = self.mem_read_byte(addr) as u32;
            }
            op::STB => {
                let raw_addr = self.state.regs[dst].wrapping_add(imm as i16 as u32);
                let addr = self.translate_address(raw_addr) as usize;
                self.mem_write_byte(addr, self.state.regs[src] as u8);
            }
            op::LDH => {
                let raw_addr = self.state.regs[src].wrapping_add(imm as i16 as u32);
                let addr = self.translate_address(raw_addr) as usize;
                self.state.regs[dst] = self.mem_read_half(addr) as u32;
            }
            op::STH => {
                let raw_addr = self.state.regs[dst].wrapping_add(imm as i16 as u32);
                let addr = self.translate_address(raw_addr) as usize;
                self.mem_write_half(addr, self.state.regs[src] as u16);
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

            // === Interrupts ===
            op::INT => {
                let vector = (imm & 0xFF) as u8;
                if vector == intr::VECTOR_SYSCALL {
                    // ── INT 0x80: Linux-compatible syscall dispatch (Level 4b) ──
                    // Convention: r0 = syscall number, r1-r3 = args.
                    let syscall_nr = self.state.regs[0];
                    if syscall_nr == lsys::SYS_EXIT {
                        self.state.exit_code = self.state.regs[1];
                        self.state.halted = 1;
                        return Err(ExecError::Halted);
                    } else if syscall_nr == lsys::SYS_WRITE {
                        let fd = self.state.regs[1];
                        let buf_addr = self.state.regs[2];
                        let len = self.state.regs[3];
                        if fd == 1 || fd == 2 {
                            let max_write = if len < 256 { len } else { 256 };
                            let mut written: u32 = 0;
                            let mut b: u32 = 0;
                            while b < max_write {
                                let byte_val = self.mem_read_byte(buf_addr.wrapping_add(b) as usize);
                                self.tty_output.push(byte_val);
                                written += 1;
                                b += 1;
                            }
                            self.state.regs[0] = written;
                        } else {
                            self.state.regs[0] = (-9i32) as u32; // EBADF
                        }
                    } else if syscall_nr == lsys::SYS_BRK {
                        let new_brk = self.state.regs[1];
                        if new_brk == 0 {
                            self.state.regs[0] = self.state.program_break;
                        } else {
                            self.state.program_break = new_brk;
                            self.state.regs[0] = new_brk;
                        }
                    } else if syscall_nr == lsys::SYS_GETPID {
                        self.state.regs[0] = self.instance_id;
                    } else if syscall_nr == lsys::SYS_CLOCK_GETTIME {
                        let tp_addr = self.state.regs[1] as usize;
                        // In userspace, use ticks_ms as time source
                        let secs = self.ticks_ms / 1000;
                        let nsecs = (self.ticks_ms % 1000) * 1_000_000;
                        self.mem_write_word(tp_addr >> 2, secs);
                        self.mem_write_word((tp_addr + 4) >> 2, nsecs);
                        self.state.regs[0] = 0;
                    } else if syscall_nr == lsys::SYS_NANOSLEEP {
                        let req_addr = self.state.regs[1] as usize;
                        let secs = self.mem_read_word(req_addr >> 2);
                        let nsecs = self.mem_read_word((req_addr + 4) >> 2);
                        let sleep_ms = secs * 1000 + nsecs / 1_000_000;
                        self.ticks_ms = self.ticks_ms.wrapping_add(sleep_ms);
                        self.state.regs[0] = 0;
                    } else if syscall_nr == lsys::SYS_READ_BLOCK {
                        // SYS_READ_BLOCK(200): r1=block_num, r2=buf_addr (dest in RAM).
                        // Copy 128 words (512 bytes) from ramdisk to destination buffer.
                        let block_num = self.state.regs[1];
                        let buf_addr = self.state.regs[2];
                        if block_num < blk::TOTAL_BLOCKS {
                            let src_base = (blk::RAMDISK_BASE_WORD + block_num * blk::WORDS_PER_BLOCK) as usize;
                            let dst_base = (buf_addr >> 2) as usize;
                            for w in 0..128usize {
                                let val = if src_base + w < self.ram.len() {
                                    self.ram[src_base + w]
                                } else {
                                    0
                                };
                                if dst_base + w < self.ram.len() {
                                    self.ram[dst_base + w] = val;
                                }
                            }
                            self.state.regs[0] = blk::BLOCK_SIZE; // 512 bytes read
                        } else {
                            self.state.regs[0] = (-(lsys::EIO as i32)) as u32;
                        }
                    } else if syscall_nr == lsys::SYS_WRITE_BLOCK {
                        // SYS_WRITE_BLOCK(201): r1=block_num, r2=buf_addr (src in RAM).
                        // Copy 128 words (512 bytes) from source buffer to ramdisk.
                        let block_num = self.state.regs[1];
                        let buf_addr = self.state.regs[2];
                        if block_num < blk::TOTAL_BLOCKS {
                            let dst_base = (blk::RAMDISK_BASE_WORD + block_num * blk::WORDS_PER_BLOCK) as usize;
                            let src_base = (buf_addr >> 2) as usize;
                            for w in 0..128usize {
                                let val = if src_base + w < self.ram.len() {
                                    self.ram[src_base + w]
                                } else {
                                    0
                                };
                                if dst_base + w < self.ram.len() {
                                    self.ram[dst_base + w] = val;
                                }
                            }
                            self.state.regs[0] = blk::BLOCK_SIZE; // 512 bytes written
                        } else {
                            self.state.regs[0] = (-(lsys::EIO as i32)) as u32;
                        }
                    } else if syscall_nr == lsys::SYS_FORK {
                        // ── SYS_FORK(2): Create child process (Level 4c scheduler) ──
                        if self.state.num_processes >= intr::MAX_PROCESSES {
                            self.state.regs[0] = (-11i32) as u32; // EAGAIN
                        } else {
                            let child_pid = self.state.num_processes as usize;
                            // Save child state: copy parent's regs + PC + flags + SP + brk
                            let mut child_state = [0u32; 20];
                            for r in 0..16 {
                                child_state[r] = self.state.regs[r];
                            }
                            child_state[16] = self.state.pc;
                            child_state[17] = self.state.flags as u32;
                            child_state[18] = self.state.regs[REG_SP as usize];
                            child_state[19] = self.state.program_break;
                            // Child gets 0 in r0
                            child_state[0] = 0;
                            self.proc_table[child_pid] = child_state;
                            self.state.num_processes += 1;
                            // Parent gets child_pid in r0
                            self.state.regs[0] = child_pid as u32;
                        }
                    } else if syscall_nr == lsys::SYS_SCHED_YIELD {
                        // ── SYS_SCHED_YIELD(158): Voluntary context switch (Level 4c) ──
                        if self.state.num_processes > 1 {
                            self.scheduler_context_switch();
                        }
                        self.state.regs[0] = 0;
                    // ── Level 5b FUZIX-critical syscalls ─────────────────
                    } else if syscall_nr == lsys::SYS_READ {
                        // SYS_READ(3): r1=fd, r2=buf_addr, r3=len.
                        // If fd==0 (stdin), read from kbd input.
                        let fd = self.state.regs[1];
                        let buf_addr = self.state.regs[2] as usize;
                        let len = self.state.regs[3];
                        if fd == 0 {
                            // In userspace emulator, read from kbd field.
                            // Treat kbd as a single pending character (low 8 bits of key code).
                            if self.kbd != 0 {
                                let pressed = self.kbd & 1;
                                if pressed == 1 {
                                    let ch = ((self.kbd >> 1) & 0xFF) as u8;
                                    let dst_word = buf_addr >> 2;
                                    let dst_byte_off = buf_addr & 3;
                                    if dst_word < self.ram.len() {
                                        let existing = self.ram[dst_word];
                                        let mask = !(0xFFu32 << (dst_byte_off * 8));
                                        let new_val = (existing & mask)
                                            | ((ch as u32) << (dst_byte_off * 8));
                                        self.ram[dst_word] = new_val;
                                    }
                                    self.kbd = 0; // consume
                                    self.state.regs[0] = 1; // 1 byte read
                                } else {
                                    self.kbd = 0;
                                    self.state.regs[0] = 0;
                                }
                            } else {
                                // No input available — return 0 bytes.
                                self.state.regs[0] = 0;
                            }
                        } else {
                            self.state.regs[0] = (-9i32) as u32; // EBADF
                        }
                    } else if syscall_nr == lsys::SYS_OPEN {
                        // SYS_OPEN(5): stub — return fd=3.
                        self.state.regs[0] = 3;
                    } else if syscall_nr == lsys::SYS_CLOSE {
                        // SYS_CLOSE(6): no-op, return 0.
                        self.state.regs[0] = 0;
                    } else if syscall_nr == lsys::SYS_WAITPID {
                        // SYS_WAITPID(7): r1=pid. Check if target process halted.
                        let target_pid = self.state.regs[1];
                        if target_pid < self.state.num_processes as u32 {
                            if (self.halted_mask >> target_pid) & 1 != 0 {
                                // Child exited.
                                self.state.regs[0] = target_pid;
                            } else {
                                // Not exited — yield and retry.
                                if self.state.num_processes > 1 {
                                    self.scheduler_context_switch();
                                }
                                // Back up PC to re-execute on next tick.
                                self.state.pc = self.state.pc.wrapping_sub(1);
                                return Ok(());
                            }
                        } else {
                            self.state.regs[0] = (-10i32) as u32; // ECHILD
                        }
                    // ── MMU control syscalls (Level 4d) ──────────────────
                    } else if syscall_nr == lsys::SYS_SET_PAGE_DIR {
                        // SYS_SET_PAGE_DIR(250): r1 = physical address of page directory.
                        self.state.page_dir_base = self.state.regs[1];
                        self.state.regs[0] = 0;
                    } else if syscall_nr == lsys::SYS_ENABLE_MMU {
                        // SYS_ENABLE_MMU(251): enable paging.
                        self.state.mmu_enabled = 1;
                        self.state.regs[0] = 0;
                    } else if syscall_nr == lsys::SYS_FLUSH_TLB {
                        // SYS_FLUSH_TLB(252): invalidate all TLB entries.
                        self.flush_tlb();
                        self.state.regs[0] = 0;
                    } else {
                        // Unknown syscall: return -ENOSYS
                        self.state.regs[0] = (-(lsys::ENOSYS as i32)) as u32;
                    }
                } else {
                    // Non-0x80 software interrupt: standard IVT dispatch.
                    // Push flags (SP is byte address, 4 bytes per entry)
                    self.state.regs[REG_SP as usize] = self.state.regs[REG_SP as usize].wrapping_sub(4);
                    let sp_flags = (self.state.regs[REG_SP as usize] >> 2) as usize;
                    if sp_flags < self.ram.len() {
                        self.ram[sp_flags] = self.state.flags as u32;
                    }
                    // Push PC (already advanced = return address)
                    self.state.regs[REG_SP as usize] = self.state.regs[REG_SP as usize].wrapping_sub(4);
                    let sp_pc = (self.state.regs[REG_SP as usize] >> 2) as usize;
                    if sp_pc < self.ram.len() {
                        self.ram[sp_pc] = self.state.pc;
                    }
                    // Disable interrupts
                    self.state.interrupts_enabled = 0;
                    // Jump to IVT handler
                    let ivt_addr = ((vector as u32) * 4) >> 2;
                    self.state.pc = self.mem_read_word(ivt_addr as usize);
                }
            }
            op::IRET => {
                // Return from interrupt: pop PC + flags, re-enable interrupts.
                let sp_pc = (self.state.regs[REG_SP as usize] >> 2) as usize;
                if sp_pc < self.ram.len() {
                    self.state.pc = self.ram[sp_pc];
                }
                self.state.regs[REG_SP as usize] = self.state.regs[REG_SP as usize].wrapping_add(4);
                let sp_flags = (self.state.regs[REG_SP as usize] >> 2) as usize;
                if sp_flags < self.ram.len() {
                    self.state.flags = self.ram[sp_flags] as u8;
                }
                self.state.regs[REG_SP as usize] = self.state.regs[REG_SP as usize].wrapping_add(4);
                // Re-enable interrupts
                self.state.interrupts_enabled = 1;
            }

            // === Halt ===
            op::HALT => {
                self.state.halted = 1;
                return Err(ExecError::Halted);
            }

            _ => {
                // Unknown opcode: treat as NOP (fail-open for forward compat)
            }
        }

        Ok(())
    }

    /// Perform a round-robin context switch (Level 4c scheduler).
    ///
    /// Saves current process state to proc_table, advances to next runnable
    /// process, and loads its state.
    pub fn scheduler_context_switch(&mut self) {
        let old_pid = self.state.current_pid as usize;
        let num = self.state.num_processes as usize;
        if num <= 1 {
            return;
        }

        // 1. Save current process state
        let mut save = [0u32; 20];
        for r in 0..16 {
            save[r] = self.state.regs[r];
        }
        save[16] = self.state.pc;
        save[17] = self.state.flags as u32;
        save[18] = self.state.regs[REG_SP as usize];
        save[19] = self.state.program_break;
        self.proc_table[old_pid] = save;

        // 2. Find next runnable process (round-robin, skip halted)
        let mut next_pid = (old_pid + 1) % num;
        for _ in 0..4 {
            if next_pid < num && (self.halted_mask & (1 << next_pid)) == 0 {
                break;
            }
            next_pid = (next_pid + 1) % num;
        }

        if next_pid == old_pid {
            return; // no other runnable process
        }

        // 3. Load next process state
        let load = self.proc_table[next_pid];
        for r in 0..16 {
            self.state.regs[r] = load[r];
        }
        self.state.pc = load[16];
        self.state.flags = load[17] as u8;
        self.state.program_break = load[19];

        // 4. Update current_pid
        self.state.current_pid = next_pid as u8;
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
        // execute_insn adds offset to current PC (step() pre-increments, but we call directly)
        assert_eq!(cpu.state.pc, 10);
    }

    #[test]
    fn test_jmp_backward() {
        let mut cpu = Cpu::new();
        cpu.state.pc = 50;
        cpu.rom.extend_from_slice(&[0; 100]);
        // -16 as 24-bit two's complement: 0xFFFFF0
        cpu.execute_insn(branch(op::JMP, 0xFFFFF0)).unwrap();
        assert_eq!(cpu.state.pc as i32, 50i32 - 16);
    }

    #[test]
    fn test_jz_taken() {
        let mut cpu = Cpu::new();
        cpu.state.pc = 0;
        cpu.state.flags = mf::Z;
        cpu.rom.extend_from_slice(&[0; 100]);
        cpu.execute_insn(branch(op::JZ, 5)).unwrap();
        assert_eq!(cpu.state.pc, 5);
    }

    #[test]
    fn test_jz_not_taken() {
        let mut cpu = Cpu::new();
        cpu.state.pc = 0;
        cpu.state.flags = 0;
        cpu.rom.extend_from_slice(&[0; 100]);
        cpu.execute_insn(branch(op::JZ, 5)).unwrap();
        assert_eq!(cpu.state.pc, 0); // Not taken, PC unchanged by execute_insn
    }

    #[test]
    fn test_jnz_taken() {
        let mut cpu = Cpu::new();
        cpu.state.pc = 0;
        cpu.state.flags = 0;
        cpu.rom.extend_from_slice(&[0; 100]);
        cpu.execute_insn(branch(op::JNZ, 5)).unwrap();
        assert_eq!(cpu.state.pc, 5);
    }

    #[test]
    fn test_jn_taken() {
        let mut cpu = Cpu::new();
        cpu.state.pc = 0;
        cpu.state.flags = mf::N;
        cpu.rom.extend_from_slice(&[0; 100]);
        cpu.execute_insn(branch(op::JN, 5)).unwrap();
        assert_eq!(cpu.state.pc, 5);
    }

    #[test]
    fn test_jp_taken() {
        let mut cpu = Cpu::new();
        cpu.state.pc = 0;
        cpu.state.flags = 0; // Neither N nor Z
        cpu.rom.extend_from_slice(&[0; 100]);
        cpu.execute_insn(branch(op::JP, 5)).unwrap();
        assert_eq!(cpu.state.pc, 5);
    }

    #[test]
    fn test_jc_taken() {
        let mut cpu = Cpu::new();
        cpu.state.pc = 0;
        cpu.state.flags = mf::C;
        cpu.rom.extend_from_slice(&[0; 100]);
        cpu.execute_insn(branch(op::JC, 5)).unwrap();
        assert_eq!(cpu.state.pc, 5);
    }

    #[test]
    fn test_jnc_taken() {
        let mut cpu = Cpu::new();
        cpu.state.pc = 0;
        cpu.state.flags = 0;
        cpu.rom.extend_from_slice(&[0; 100]);
        cpu.execute_insn(branch(op::JNC, 5)).unwrap();
        assert_eq!(cpu.state.pc, 5);
    }

    #[test]
    fn test_call_ret() {
        let mut cpu = Cpu::new();
        cpu.rom.extend_from_slice(&[0; 100]);
        cpu.state.pc = 10;
        cpu.state.regs[REG_SP as usize] = 0x1000;

        // CALL to address 50 — pushes current PC (10) as return addr
        cpu.execute_insn(branch(op::CALL, 50)).unwrap();
        assert_eq!(cpu.state.pc, 50);
        assert_eq!(cpu.state.regs[REG_SP as usize], 0xFFF);
        assert_eq!(cpu.ram[0x1000], 10); // Return address = PC at time of CALL

        // RET
        cpu.execute_insn(alu(op::RET, 0, 0)).unwrap();
        assert_eq!(cpu.state.pc, 10);
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
        assert_eq!(cpu.ram[0x1000], 10);
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
    fn test_nop() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 42;
        cpu.state.regs[1] = 99;
        let saved_flags = cpu.state.flags;
        cpu.execute_insn(MbcInsn::encode(op::NOP, 0, 0, 0)).unwrap();
        // NOP should not modify any register or flags
        assert_eq!(cpu.state.regs[0], 42);
        assert_eq!(cpu.state.regs[1], 99);
        assert_eq!(cpu.state.flags, saved_flags);
    }

    #[test]
    fn test_push_pop() {
        let mut cpu = Cpu::new();
        cpu.state.regs[REG_SP as usize] = 0x1000;
        cpu.state.regs[3] = 0xDEADBEEF;

        // PUSH r3
        cpu.execute_insn(MbcInsn::encode(op::PUSH, 3, 0, 0)).unwrap();
        assert_eq!(cpu.state.regs[REG_SP as usize], 0xFFF);
        assert_eq!(cpu.ram[0x1000], 0xDEADBEEF);

        // Clear r3, then POP to verify roundtrip
        cpu.state.regs[3] = 0;

        // POP r3
        cpu.execute_insn(MbcInsn::encode(op::POP, 3, 0, 0)).unwrap();
        assert_eq!(cpu.state.regs[3], 0xDEADBEEF);
        assert_eq!(cpu.state.regs[REG_SP as usize], 0x1000);
    }

    #[test]
    fn test_push_pop_multiple() {
        let mut cpu = Cpu::new();
        cpu.state.regs[REG_SP as usize] = 0x1000;
        cpu.state.regs[0] = 111;
        cpu.state.regs[1] = 222;

        // Push r0, r1
        cpu.execute_insn(MbcInsn::encode(op::PUSH, 0, 0, 0)).unwrap();
        cpu.execute_insn(MbcInsn::encode(op::PUSH, 1, 0, 0)).unwrap();
        assert_eq!(cpu.state.regs[REG_SP as usize], 0xFFE);

        // Pop into reversed registers (LIFO order)
        cpu.execute_insn(MbcInsn::encode(op::POP, 2, 0, 0)).unwrap();
        assert_eq!(cpu.state.regs[2], 222); // Last pushed = first popped
        cpu.execute_insn(MbcInsn::encode(op::POP, 3, 0, 0)).unwrap();
        assert_eq!(cpu.state.regs[3], 111);
        assert_eq!(cpu.state.regs[REG_SP as usize], 0x1000);
    }

    #[test]
    fn test_load_imm32() {
        let mut cpu = Cpu::new();
        // Load lower 16 bits first
        cpu.execute_insn(imm(op::MOVI, 0, 0x5678)).unwrap();
        assert_eq!(cpu.state.regs[0], 0x5678);
        // Load upper 16 bits
        cpu.execute_insn(imm(op::LOAD_IMM32, 0, 0x1234)).unwrap();
        assert_eq!(cpu.state.regs[0], 0x12345678);
    }

    #[test]
    fn test_load_imm32_preserves_lower() {
        let mut cpu = Cpu::new();
        cpu.state.regs[1] = 0x0000ABCD;
        cpu.execute_insn(imm(op::LOAD_IMM32, 1, 0xEF01)).unwrap();
        assert_eq!(cpu.state.regs[1], 0xEF01ABCD);
    }

    #[test]
    fn test_addi_positive() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 100;
        cpu.execute_insn(imm(op::ADDI, 0, 42)).unwrap();
        assert_eq!(cpu.state.regs[0], 142);
    }

    #[test]
    fn test_addi_negative() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 100;
        // -10 as u16 = 0xFFF6
        cpu.execute_insn(imm(op::ADDI, 0, (-10_i16) as u16)).unwrap();
        assert_eq!(cpu.state.regs[0], 90);
    }

    #[test]
    fn test_addi_overflow_sets_carry() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 0xFFFFFFFF;
        cpu.execute_insn(imm(op::ADDI, 0, 1)).unwrap();
        assert_eq!(cpu.state.regs[0], 0);
        assert!(cpu.state.flags & mf::Z != 0);
        assert!(cpu.state.flags & mf::C != 0);
    }

    #[test]
    fn test_addi_zero() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 42;
        cpu.execute_insn(imm(op::ADDI, 0, 0)).unwrap();
        assert_eq!(cpu.state.regs[0], 42);
    }

    #[test]
    fn test_cycle_budget_exhaustion() {
        let mut cpu = Cpu::new();
        // Fill ROM with NOP instructions
        let nop_word = MbcInsn::encode(op::NOP, 0, 0, 0).0;
        cpu.rom.extend_from_slice(&[nop_word; 10]);
        let result = cpu.run(5);
        assert_eq!(result, Err(ExecError::CycleBudgetExhausted));
    }

    #[test]
    fn test_sum_1_to_10_program() {
        // Program: sum 1 + 2 + ... + 10
        // r0 = sum, r1 = counter, r2 = max
        let mut cpu = Cpu::new();

        // step() increments PC before execute_insn, so branch offsets are relative
        // to (instruction_position + 1). To jump from index 7 back to index 3,
        // we need offset = 3 - (7+1) = -5 → 24-bit two's complement = 0xFFFFFB.
        let program = vec![
            MbcInsn::encode(op::MOVI, 0, 0, 0).0,       // 0: MOVI r0, 0
            MbcInsn::encode(op::MOVI, 1, 0, 1).0,       // 1: MOVI r1, 1
            MbcInsn::encode(op::MOVI, 2, 0, 11).0,      // 2: MOVI r2, 11 (loop until counter==11)
            MbcInsn::encode(op::ADD, 0, 1, 0).0,         // 3: ADD r0, r1
            MbcInsn::encode(op::MOVI, 3, 0, 1).0,       // 4: MOVI r3, 1
            MbcInsn::encode(op::ADD, 1, 3, 0).0,         // 5: ADD r1, r3
            MbcInsn::encode(op::CMP, 1, 2, 0).0,         // 6: CMP r1, r2
            branch(op::JNZ, 0xFFFFFB).0,                 // 7: JNZ -5 (→ PC 8-5=3)
            MbcInsn::encode(op::HALT, 0, 0, 0).0,        // 8: HALT
        ];

        cpu.load_rom(&program);
        let result = cpu.run(1000);

        assert!(result.is_ok() || result == Err(ExecError::Halted));
        // Sum of 1..10 = 55
        assert_eq!(cpu.state.regs[0], 55);
    }

    // ── Phase 2: Hardening tests ────────────────────────────────────────────

    #[test]
    fn test_step_increments_insn_count() {
        let mut cpu = Cpu::new();
        let nop = MbcInsn::encode(op::NOP, 0, 0, 0).0;
        cpu.load_rom(&[nop, nop, nop, MbcInsn::encode(op::HALT, 0, 0, 0).0]);
        let _ = cpu.run(100);
        // 3 NOPs + 1 HALT that returns Err before increment = 3
        assert_eq!(cpu.state.insn_count, 3);
    }

    #[test]
    fn test_screen_write_via_stb() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 0xAB;
        cpu.state.regs[1] = mmap::SCREEN_BASE;
        cpu.execute_insn(mem(op::STB, 1, 0, 0)).unwrap();
        assert_eq!(cpu.screen[0], 0xAB);
    }

    #[test]
    fn test_screen_read_via_ldb() {
        let mut cpu = Cpu::new();
        cpu.screen[42] = 0xCD;
        cpu.state.regs[1] = mmap::SCREEN_BASE + 42;
        cpu.execute_insn(mem(op::LDB, 0, 1, 0)).unwrap();
        assert_eq!(cpu.state.regs[0], 0xCD);
    }

    #[test]
    fn test_screen_write_read_word() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 0x04030201;
        cpu.state.regs[1] = mmap::SCREEN_BASE;
        // ST writes a word to screen base
        cpu.execute_insn(mem(op::ST, 1, 0, 0)).unwrap();
        assert_eq!(cpu.screen[0], 0x01);
        assert_eq!(cpu.screen[1], 0x02);
        assert_eq!(cpu.screen[2], 0x03);
        assert_eq!(cpu.screen[3], 0x04);
        // LD reads it back
        cpu.state.regs[0] = 0;
        cpu.execute_insn(mem(op::LD, 0, 1, 0)).unwrap();
        assert_eq!(cpu.state.regs[0], 0x04030201);
    }

    #[test]
    fn test_kbd_read_via_ld() {
        let mut cpu = Cpu::new();
        cpu.kbd = 0xDEAD;
        // KBD_ADDR = 0xFFFF, word addr = 0xFFFF >> 2 = 0x3FFF
        cpu.state.regs[1] = mmap::KBD_ADDR & !3; // word-aligned address
        cpu.execute_insn(mem(op::LD, 0, 1, 0)).unwrap();
        assert_eq!(cpu.state.regs[0], 0xDEAD);
    }

    #[test]
    fn test_div_by_zero_carry_flag() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 42;
        cpu.state.regs[1] = 0;
        cpu.execute_insn(alu(op::DIV, 0, 1)).unwrap();
        assert_eq!(cpu.state.regs[0], 0xFFFFFFFF);
        assert_ne!(cpu.state.flags & mf::C, 0); // Carry set on div-by-zero
    }

    #[test]
    fn test_mod_by_zero_returns_zero() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 42;
        cpu.state.regs[1] = 0;
        cpu.execute_insn(alu(op::MOD, 0, 1)).unwrap();
        assert_eq!(cpu.state.regs[0], 0);
    }

    #[test]
    fn test_shl_by_0_1_31_32() {
        let mut cpu = Cpu::new();

        // SHL by 0 = no change
        cpu.state.regs[0] = 0xDEAD;
        cpu.execute_insn(imm(op::SHL, 0, 0)).unwrap();
        assert_eq!(cpu.state.regs[0], 0xDEAD);

        // SHL by 1
        cpu.state.regs[0] = 0x01;
        cpu.execute_insn(imm(op::SHL, 0, 1)).unwrap();
        assert_eq!(cpu.state.regs[0], 0x02);

        // SHL by 31
        cpu.state.regs[0] = 0x01;
        cpu.execute_insn(imm(op::SHL, 0, 31)).unwrap();
        assert_eq!(cpu.state.regs[0], 0x80000000);

        // SHL by 32: wrapping_shl(32) for u32 shifts by 32%32=0 (no change)
        // BPF masks with 0x1F so shift by 32 → shift by 0
        cpu.state.regs[0] = 0x01;
        cpu.execute_insn(imm(op::SHL, 0, 32)).unwrap();
        assert_eq!(cpu.state.regs[0], 0x01);
    }

    #[test]
    fn test_mulh_large_product() {
        let mut cpu = Cpu::new();
        // 0x80000000 * 2 = 0x1_00000000 → high word = 1
        cpu.state.regs[0] = 0x80000000;
        cpu.state.regs[1] = 2;
        cpu.execute_insn(alu(op::MULH, 0, 1)).unwrap();
        assert_eq!(cpu.state.regs[0], 1);
    }

    #[test]
    fn test_cmp_flags_exhaustive() {
        let mut cpu = Cpu::new();

        // CMP(10, 10) → Z set, N clear
        cpu.state.regs[0] = 10;
        cpu.state.regs[1] = 10;
        cpu.execute_insn(alu(op::CMP, 0, 1)).unwrap();
        assert_ne!(cpu.state.flags & mf::Z, 0);
        assert_eq!(cpu.state.flags & mf::N, 0);

        // CMP(10, 5) → result positive, no Z/N
        cpu.state.regs[0] = 10;
        cpu.state.regs[1] = 5;
        cpu.execute_insn(alu(op::CMP, 0, 1)).unwrap();
        assert_eq!(cpu.state.flags & mf::Z, 0);
        assert_eq!(cpu.state.flags & mf::N, 0);

        // CMP(5, 10) → result negative, N set
        cpu.state.regs[0] = 5;
        cpu.state.regs[1] = 10;
        cpu.execute_insn(alu(op::CMP, 0, 1)).unwrap();
        assert_eq!(cpu.state.flags & mf::Z, 0);
        assert_ne!(cpu.state.flags & mf::N, 0);

        // CMP(0xFFFFFFFF, 0) → subtraction wraps, carry set
        cpu.state.regs[0] = 0xFFFFFFFF;
        cpu.state.regs[1] = 0;
        cpu.execute_insn(alu(op::CMP, 0, 1)).unwrap();
        assert_eq!(cpu.state.flags & mf::Z, 0);
        assert_ne!(cpu.state.flags & mf::N, 0);
    }

    #[test]
    fn test_nested_call_ret() {
        // step() increments PC before execute_insn, so CALL at index 0:
        //   PC goes 0→1 (step), then CALL pushes 1, jumps to target.
        // RET pops return addr, resumes from there.
        //
        // 0: CALL 4     → push 1, PC=4
        // 1: HALT        ← final return
        // 2: NOP
        // 3: NOP
        // 4: CALL 7     → push 5, PC=7
        // 5: RET         → pop 1, PC=1 → HALT
        // 6: NOP
        // 7: MOVI r0, 99
        // 8: RET         → pop 5, PC=5 → RET again

        let program = vec![
            branch(op::CALL, 4).0,                        // 0: CALL 4
            MbcInsn::encode(op::HALT, 0, 0, 0).0,        // 1: HALT (final)
            MbcInsn::encode(op::NOP, 0, 0, 0).0,         // 2: NOP
            MbcInsn::encode(op::NOP, 0, 0, 0).0,         // 3: NOP
            branch(op::CALL, 7).0,                        // 4: CALL 7
            MbcInsn::encode(op::RET, 0, 0, 0).0,         // 5: RET
            MbcInsn::encode(op::NOP, 0, 0, 0).0,         // 6: NOP
            MbcInsn::encode(op::MOVI, 0, 0, 99).0,       // 7: MOVI r0, 99
            MbcInsn::encode(op::RET, 0, 0, 0).0,         // 8: RET
        ];

        let mut cpu = Cpu::new();
        // Set SP within userspace RAM bounds (not 0xFFFF_0000 which is BPF HashMap-only)
        cpu.state.regs[REG_SP as usize] = 0x1000;
        cpu.load_rom(&program);
        let result = cpu.run(100);
        // run() returns Ok(cycles) when HALT is hit
        assert!(result.is_ok());
        assert_eq!(cpu.state.regs[0], 99);
        assert_eq!(cpu.state.halted, 1);
    }

    #[test]
    fn test_rom_fault_on_pc_out_of_bounds() {
        let mut cpu = Cpu::new();
        // Forward jump beyond ROM
        cpu.rom.extend_from_slice(&[branch(op::JMP, 100).0]);
        let step1 = cpu.step(); // executes JMP, PC → 100+1 = 101
        assert!(step1.is_ok());
        let step2 = cpu.step(); // PC=101 > rom.len()=1, should RomFault
        assert_eq!(step2, Err(ExecError::RomFault));
    }

    #[test]
    fn test_syscall_get_key() {
        let mut cpu = Cpu::new();
        cpu.kbd = 0x42;
        cpu.state.regs[0] = sys::SYS_GET_KEY;
        cpu.execute_insn(alu(op::SYSCALL, 0, 0)).unwrap();
        assert_eq!(cpu.state.regs[0], 0x42);
    }

    #[test]
    fn test_syscall_invalid() {
        let mut cpu = Cpu::new();
        cpu.state.regs[0] = 0xFF;
        let result = cpu.execute_insn(alu(op::SYSCALL, 0, 0));
        assert_eq!(result, Err(ExecError::InvalidSyscall(0xFF)));
    }

    #[test]
    fn test_instruction_roundtrip_encode_decode() {
        // For all valid opcodes, verify encode → decode fields match
        for opc_val in 0..=255u8 {
            for dst in 0..16u8 {
                for src in 0..16u8 {
                    let imm16 = (dst as u16 * 17) ^ (src as u16 * 31);
                    let insn = MbcInsn::encode(opc_val, dst, src, imm16);
                    assert_eq!(insn.opcode(), opc_val);
                    assert_eq!(insn.dst(), dst & 0xF);
                    assert_eq!(insn.src(), src & 0xF);
                    assert_eq!(insn.imm16(), imm16);
                }
            }
        }
    }

    #[test]
    fn test_adversarial_random_instructions() {
        // Load ROM with pseudo-random u32 values — must not panic
        let mut cpu = Cpu::new();
        let mut rom = Vec::with_capacity(1000);
        let mut seed = 0xDEADBEEFu32;
        for _ in 0..1000 {
            seed ^= seed << 13;
            seed ^= seed >> 17;
            seed ^= seed << 5;
            rom.push(seed);
        }
        cpu.load_rom(&rom);
        let _ = cpu.run(1000);
        // Success = no panic
    }

    #[test]
    fn test_fibonacci() {
        // fib(10) = 55: r0=fib(n-1), r1=fib(n-2), r2=counter, r3=max, r4=temp
        let program = vec![
            MbcInsn::encode(op::MOVI, 0, 0, 1).0,       // 0: r0 = 1 (fib(1))
            MbcInsn::encode(op::MOVI, 1, 0, 0).0,       // 1: r1 = 0 (fib(0))
            MbcInsn::encode(op::MOVI, 2, 0, 2).0,       // 2: r2 = 2 (counter)
            MbcInsn::encode(op::MOVI, 3, 0, 11).0,      // 3: r3 = 11 (loop until counter==11, computing fib(10))
            // loop:
            MbcInsn::encode(op::MOV, 4, 0, 0).0,        // 4: r4 = r0
            MbcInsn::encode(op::ADD, 0, 1, 0).0,         // 5: r0 = r0 + r1
            MbcInsn::encode(op::MOV, 1, 4, 0).0,         // 6: r1 = r4 (old r0)
            MbcInsn::encode(op::ADDI, 2, 0, 1).0,        // 7: r2 += 1
            MbcInsn::encode(op::CMP, 2, 3, 0).0,         // 8: cmp r2, r3
            branch(op::JNZ, 0xFFFFFA).0,                 // 9: JNZ -6 → PC 10-6=4 (back to MOV r4,r0)
            MbcInsn::encode(op::HALT, 0, 0, 0).0,        // 10: HALT
        ];

        let mut cpu = Cpu::new();
        cpu.load_rom(&program);
        let _ = cpu.run(1000);
        assert_eq!(cpu.state.regs[0], 55); // fib(10) = 55
    }

    #[test]
    fn test_memory_copy_program() {
        // Store values to RAM, then copy them elsewhere
        let mut cpu = Cpu::new();
        // Manually set up: store 4 words at addr 0x100, copy to 0x200
        cpu.ram[0x100 >> 2] = 0xAAAA;
        cpu.ram[(0x100 + 4) >> 2] = 0xBBBB;
        cpu.ram[(0x100 + 8) >> 2] = 0xCCCC;
        cpu.ram[(0x100 + 12) >> 2] = 0xDDDD;

        let program = vec![
            MbcInsn::encode(op::MOVI, 1, 0, 0x0100 as u16).0,  // 0: r1 = src addr
            MbcInsn::encode(op::MOVI, 2, 0, 0x0200 as u16).0,  // 1: r2 = dst addr
            MbcInsn::encode(op::MOVI, 3, 0, 4).0,               // 2: r3 = count
            // loop:
            MbcInsn::encode(op::LD, 0, 1, 0).0,                 // 3: r0 = RAM[r1]
            MbcInsn::encode(op::ST, 2, 0, 0).0,                 // 4: RAM[r2] = r0
            MbcInsn::encode(op::ADDI, 1, 0, 4).0,               // 5: r1 += 4
            MbcInsn::encode(op::ADDI, 2, 0, 4).0,               // 6: r2 += 4
            MbcInsn::encode(op::ADDI, 3, 0, 0xFFFF).0,          // 7: r3 += -1 (sign extend)
            MbcInsn::encode(op::CMP, 3, 4, 0).0,                // 8: cmp r3, r4 (r4=0)
            branch(op::JNZ, 0xFFFFF9).0,                        // 9: JNZ -7 → 10-7=3 (back to LD)
            MbcInsn::encode(op::HALT, 0, 0, 0).0,               // 10: HALT
        ];

        cpu.load_rom(&program);
        let _ = cpu.run(1000);

        assert_eq!(cpu.ram[0x200 >> 2], 0xAAAA);
        assert_eq!(cpu.ram[(0x200 + 4) >> 2], 0xBBBB);
        assert_eq!(cpu.ram[(0x200 + 8) >> 2], 0xCCCC);
        assert_eq!(cpu.ram[(0x200 + 12) >> 2], 0xDDDD);
    }
}
