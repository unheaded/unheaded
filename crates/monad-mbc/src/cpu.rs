//! Userspace MBC CPU state — wraps monad_common::MbcCpuState with emulation helpers.
//!
//! This module provides a CpuState wrapper for the Doom-over-IPv6 PoC MBC emulator.
//! The underlying MbcCpuState is BPF-compatible (80 bytes), and this userspace wrapper
//! adds emulation helpers such as RAM/ROM access, flag manipulation, and lifecycle methods.

use monad_common::MbcCpuState;

// Re-export common constants for convenience
pub use monad_common::{mbc_flags, mbc_mmap, REG_SP, REG_SP_DEFAULT};

/// RAM size for the userspace emulator (64KB address space).
pub const RAM_SIZE: u32 = 0x0001_0000;
pub const RAM_WORDS: usize = (RAM_SIZE / 4) as usize;

/// Maximum CPU cycles allowed per IPv6 packet processing burst.
pub const MAX_CYCLES_PER_PACKET: u32 = 1000;

/// Userspace MBC CPU emulator state.
///
/// Wraps the BPF-compatible MbcCpuState with additional storage (RAM, ROM, screen, keyboard)
/// and helper methods for instruction execution and memory access.
pub struct Cpu {
    /// BPF-compatible CPU state (mirrors monad_common::MbcCpuState).
    pub state: MbcCpuState,
    /// Userspace RAM (64KB, indexed by word address).
    pub ram: Vec<u32>,
    /// Instruction ROM (loaded externally).
    pub rom: Vec<u32>,
    /// Screen buffer (64000 bytes for 320x200 framebuffer).
    pub screen: Vec<u8>,
    /// Keyboard state (single u32 register).
    pub kbd: u32,
    /// Cycle budget remaining for current execution burst.
    pub cycles_remaining: u32,
}

impl Cpu {
    /// Creates a new CPU with initialized default state.
    ///
    /// - SP (register 15) set to REG_SP_DEFAULT (0xFFFF_0000)
    /// - PC, flags, and other registers zeroed
    /// - RAM and ROM empty (ROM must be loaded separately)
    pub fn new() -> Self {
        let mut state = MbcCpuState {
            regs: [0u32; 16],
            pc: 0,
            flags: 0,
            halted: 0,
            stalled: 0,
            _pad: 0,
            sleep_until_ns: 0,
            insn_count: 0,
            cache_hits: 0,
            cache_misses: 0,
            interrupt_pending: 0,
            interrupt_vector: 0,
            interrupts_enabled: 0,
            _pad2: 0,
            tick_counter: 0,
            program_break: monad_common::DEFAULT_PROGRAM_BREAK,
            exit_code: 0,
            current_pid: 0,
            num_processes: 1,
            mmu_enabled: 0,
            _pad3: 0,
            page_dir_base: 0,
        };

        // Initialize stack pointer to default value
        state.regs[REG_SP as usize] = REG_SP_DEFAULT;

        Self {
            state,
            ram: vec![0u32; RAM_WORDS],
            rom: Vec::new(),
            screen: vec![0u8; mbc_mmap::SCREEN_SIZE as usize],
            kbd: 0,
            cycles_remaining: MAX_CYCLES_PER_PACKET,
        }
    }

    /// Loads a ROM into the CPU's instruction memory.
    ///
    /// # Arguments
    /// * `rom` - Slice of u32 words representing the ROM
    pub fn load_rom(&mut self, rom: &[u32]) {
        self.rom = rom.to_vec();
    }

    /// Resets CPU state while preserving ROM and cycle budget.
    ///
    /// Clears:
    /// - All registers (except SP, which is reset to default)
    /// - PC, flags, halted, stalled states
    /// - Sleep timer and instruction counters
    /// - Cache hit/miss counters
    pub fn reset(&mut self) {
        self.state.regs = [0u32; 16];
        self.state.regs[REG_SP as usize] = REG_SP_DEFAULT;
        self.state.pc = 0;
        self.state.flags = 0;
        self.state.halted = 0;
        self.state.stalled = 0;
        self.state._pad = 0;
        self.state.sleep_until_ns = 0;
        self.state.insn_count = 0;
        self.state.cache_hits = 0;
        self.state.cache_misses = 0;
        self.cycles_remaining = MAX_CYCLES_PER_PACKET;
    }

    /// Checks if the CPU is halted.
    #[inline]
    pub fn is_halted(&self) -> bool {
        self.state.halted != 0
    }

    /// Checks if the CPU is sleeping (sleep_until_ns in future).
    #[inline]
    pub fn is_sleeping(&self) -> bool {
        self.state.sleep_until_ns > 0
    }

    /// Reads the Zero flag (bit 0).
    #[inline]
    pub fn flag_z(&self) -> bool {
        (self.state.flags & mbc_flags::Z) != 0
    }

    /// Reads the Negative flag (bit 1).
    #[inline]
    pub fn flag_n(&self) -> bool {
        (self.state.flags & mbc_flags::N) != 0
    }

    /// Reads the Carry flag (bit 2).
    #[inline]
    pub fn flag_c(&self) -> bool {
        (self.state.flags & mbc_flags::C) != 0
    }

    /// Sets flags based on a result and optional carry.
    ///
    /// Updates Z flag if result is zero, N flag if result's sign bit is set.
    /// C flag is set if carry is true.
    ///
    /// # Arguments
    /// * `result` - The result value to derive Z and N flags from
    /// * `carry` - Whether to set the carry flag
    pub fn set_flags(&mut self, result: u32, carry: bool) {
        self.state.flags = 0;

        if result == 0 {
            self.state.flags |= mbc_flags::Z;
        }

        if (result as i32) < 0 {
            self.state.flags |= mbc_flags::N;
        }

        if carry {
            self.state.flags |= mbc_flags::C;
        }
    }

    /// Reads a 32-bit word from RAM at the given byte address.
    ///
    /// # Arguments
    /// * `addr` - Byte address in RAM
    ///
    /// # Returns
    /// The 32-bit word at that address, or 0 if out of bounds.
    pub fn ram_read_word(&self, addr: u32) -> u32 {
        if addr >= RAM_SIZE || addr % 4 != 0 {
            return 0;
        }
        let word_idx = (addr / 4) as usize;
        self.ram.get(word_idx).copied().unwrap_or(0)
    }

    /// Writes a 32-bit word to RAM at the given byte address.
    ///
    /// # Arguments
    /// * `addr` - Byte address in RAM
    /// * `value` - The 32-bit word to write
    ///
    /// # Returns
    /// true if write succeeded, false if out of bounds or misaligned.
    pub fn ram_write_word(&mut self, addr: u32, value: u32) -> bool {
        if addr >= RAM_SIZE || addr % 4 != 0 {
            return false;
        }
        let word_idx = (addr / 4) as usize;
        if let Some(slot) = self.ram.get_mut(word_idx) {
            *slot = value;
            true
        } else {
            false
        }
    }

    /// Reads a byte from RAM at the given byte address.
    ///
    /// # Arguments
    /// * `addr` - Byte address in RAM
    ///
    /// # Returns
    /// The byte at that address, or 0 if out of bounds.
    pub fn ram_read_byte(&self, addr: u32) -> u8 {
        if addr >= RAM_SIZE {
            return 0;
        }
        let word_idx = (addr / 4) as usize;
        let byte_offset = (addr % 4) as usize;

        if let Some(&word) = self.ram.get(word_idx) {
            ((word >> (byte_offset * 8)) & 0xFF) as u8
        } else {
            0
        }
    }

    /// Writes a byte to RAM at the given byte address.
    ///
    /// # Arguments
    /// * `addr` - Byte address in RAM
    /// * `value` - The byte to write
    ///
    /// # Returns
    /// true if write succeeded, false if out of bounds.
    pub fn ram_write_byte(&mut self, addr: u32, value: u8) -> bool {
        if addr >= RAM_SIZE {
            return false;
        }
        let word_idx = (addr / 4) as usize;
        let byte_offset = (addr % 4) as usize;

        if let Some(word) = self.ram.get_mut(word_idx) {
            let mask = 0xFF_u32 << (byte_offset * 8);
            *word = (*word & !mask) | ((value as u32) << (byte_offset * 8));
            true
        } else {
            false
        }
    }

    /// Reads a 32-bit word from ROM at the given word address.
    ///
    /// # Arguments
    /// * `word_addr` - Word address (not byte address)
    ///
    /// # Returns
    /// The instruction word, or 0 if out of ROM bounds.
    pub fn rom_read(&self, word_addr: u32) -> u32 {
        self.rom.get(word_addr as usize).copied().unwrap_or(0)
    }

    /// Reads a 32-bit word from the keyboard I/O register.
    ///
    /// # Returns
    /// The current keyboard state.
    pub fn kbd_read(&self) -> u32 {
        self.kbd
    }

    /// Writes to the keyboard I/O register.
    ///
    /// # Arguments
    /// * `value` - The value to write to the keyboard register
    pub fn kbd_write(&mut self, value: u32) {
        self.kbd = value;
    }
}

impl Default for Cpu {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // Compile-time assertion: MbcCpuState = 16×u32 regs + u32 pc + u8×4 (flags/halted/stalled/pad)
    //   + u64 sleep_until_ns + u64 insn_count + u64 cache_hits + u64 cache_misses
    //   + u8×4 (interrupt_pending/vector/enabled/_pad2) + u32 tick_counter
    //   + u32 program_break + u32 exit_code + u8 current_pid + u8 num_processes
    //   + [u8; 6] _pad3 = 128 bytes
    const _: [u8; 128] = [0u8; std::mem::size_of::<MbcCpuState>()];

    #[test]
    fn test_mbccpustate_size() {
        assert_eq!(std::mem::size_of::<MbcCpuState>(), 128);
    }

    #[test]
    fn test_cpu_new() {
        let cpu = Cpu::new();

        // Check SP is at default
        assert_eq!(cpu.state.regs[REG_SP as usize], REG_SP_DEFAULT);

        // Check PC is zero
        assert_eq!(cpu.state.pc, 0);

        // Check flags are clear
        assert_eq!(cpu.state.flags, 0);

        // Check halted and stalled are clear
        assert_eq!(cpu.state.halted, 0);
        assert_eq!(cpu.state.stalled, 0);

        // Check other registers are zero
        for i in 0..15 {
            assert_eq!(cpu.state.regs[i], 0);
        }

        // Check counters are zero
        assert_eq!(cpu.state.insn_count, 0);
        assert_eq!(cpu.state.cache_hits, 0);
        assert_eq!(cpu.state.cache_misses, 0);

        // Check RAM is allocated and zeroed
        assert_eq!(cpu.ram.len(), RAM_WORDS);
        assert!(cpu.ram.iter().all(|&x| x == 0));

        // Check cycle budget
        assert_eq!(cpu.cycles_remaining, MAX_CYCLES_PER_PACKET);
    }

    #[test]
    fn test_cpu_reset() {
        let mut cpu = Cpu::new();

        // Modify state
        cpu.state.regs[0] = 42;
        cpu.state.pc = 100;
        cpu.state.flags = 0xFF;
        cpu.state.halted = 1;
        cpu.state.insn_count = 500;
        cpu.cycles_remaining = 100;

        // Reset
        cpu.reset();

        // Verify state is reset
        assert_eq!(cpu.state.regs[0], 0);
        assert_eq!(cpu.state.regs[REG_SP as usize], REG_SP_DEFAULT);
        assert_eq!(cpu.state.pc, 0);
        assert_eq!(cpu.state.flags, 0);
        assert_eq!(cpu.state.halted, 0);
        assert_eq!(cpu.state.insn_count, 0);
        assert_eq!(cpu.cycles_remaining, MAX_CYCLES_PER_PACKET);
    }

    #[test]
    fn test_flag_z_isolation() {
        let mut cpu = Cpu::new();

        // Set only Z flag
        cpu.state.flags = mbc_flags::Z;
        assert!(cpu.flag_z());
        assert!(!cpu.flag_n());
        assert!(!cpu.flag_c());

        // Set only N flag
        cpu.state.flags = mbc_flags::N;
        assert!(!cpu.flag_z());
        assert!(cpu.flag_n());
        assert!(!cpu.flag_c());

        // Set only C flag
        cpu.state.flags = mbc_flags::C;
        assert!(!cpu.flag_z());
        assert!(!cpu.flag_n());
        assert!(cpu.flag_c());
    }

    #[test]
    fn test_flag_n_isolation() {
        let mut cpu = Cpu::new();

        // Start with N set
        cpu.state.flags = mbc_flags::N;
        assert!(cpu.flag_n());

        // Set Z without affecting N
        cpu.state.flags |= mbc_flags::Z;
        assert!(cpu.flag_z());
        assert!(cpu.flag_n());
        assert!(!cpu.flag_c());
    }

    #[test]
    fn test_flag_c_isolation() {
        let mut cpu = Cpu::new();

        // Start with C set
        cpu.state.flags = mbc_flags::C;
        assert!(cpu.flag_c());

        // Clear C, set Z
        cpu.state.flags = mbc_flags::Z;
        assert!(cpu.flag_z());
        assert!(!cpu.flag_n());
        assert!(!cpu.flag_c());
    }

    #[test]
    fn test_set_flags_zero() {
        let mut cpu = Cpu::new();

        cpu.set_flags(0, false);
        assert!(cpu.flag_z());
        assert!(!cpu.flag_n());
        assert!(!cpu.flag_c());
    }

    #[test]
    fn test_set_flags_negative() {
        let mut cpu = Cpu::new();

        // 0x8000_0000 is negative when interpreted as i32
        cpu.set_flags(0x8000_0000, false);
        assert!(!cpu.flag_z());
        assert!(cpu.flag_n());
        assert!(!cpu.flag_c());
    }

    #[test]
    fn test_set_flags_carry() {
        let mut cpu = Cpu::new();

        cpu.set_flags(100, true);
        assert!(!cpu.flag_z());
        assert!(!cpu.flag_n());
        assert!(cpu.flag_c());
    }

    #[test]
    fn test_set_flags_combined() {
        let mut cpu = Cpu::new();

        // 0xFFFF_FFFF is -1 in two's complement
        cpu.set_flags(0xFFFF_FFFF, true);
        assert!(!cpu.flag_z());
        assert!(cpu.flag_n());
        assert!(cpu.flag_c());
    }

    #[test]
    fn test_ram_read_write_word_roundtrip() {
        let mut cpu = Cpu::new();

        let test_value = 0xDEAD_BEEF;
        let addr = 0x1000;

        // Write and read back
        assert!(cpu.ram_write_word(addr, test_value));
        assert_eq!(cpu.ram_read_word(addr), test_value);
    }

    #[test]
    fn test_ram_read_write_multiple_words() {
        let mut cpu = Cpu::new();

        let values = [0x1111_1111, 0x2222_2222, 0x3333_3333];

        for (i, &value) in values.iter().enumerate() {
            let addr = (i as u32) * 4;
            assert!(cpu.ram_write_word(addr, value));
        }

        for (i, &expected) in values.iter().enumerate() {
            let addr = (i as u32) * 4;
            assert_eq!(cpu.ram_read_word(addr), expected);
        }
    }

    #[test]
    fn test_ram_oob_read_returns_zero() {
        let cpu = Cpu::new();

        // Read beyond RAM size
        assert_eq!(cpu.ram_read_word(0xFFFF_FFFF), 0);
        assert_eq!(cpu.ram_read_word(RAM_SIZE), 0);
        assert_eq!(cpu.ram_read_word(RAM_SIZE + 4), 0);
    }

    #[test]
    fn test_ram_oob_write_returns_false() {
        let mut cpu = Cpu::new();

        // Write beyond RAM size
        assert!(!cpu.ram_write_word(0xFFFF_FFFF, 42));
        assert!(!cpu.ram_write_word(RAM_SIZE, 42));
        assert!(!cpu.ram_write_word(RAM_SIZE + 4, 42));
    }

    #[test]
    fn test_ram_misaligned_access() {
        let mut cpu = Cpu::new();

        // Misaligned word access should fail
        assert!(!cpu.ram_write_word(0x0001, 42));
        assert!(!cpu.ram_write_word(0x0002, 42));
        assert!(!cpu.ram_write_word(0x0003, 42));

        assert_eq!(cpu.ram_read_word(0x0001), 0);
        assert_eq!(cpu.ram_read_word(0x0002), 0);
        assert_eq!(cpu.ram_read_word(0x0003), 0);
    }

    #[test]
    fn test_ram_read_write_byte_roundtrip() {
        let mut cpu = Cpu::new();

        let bytes = [0x12, 0x34, 0x56, 0x78];
        let addr = 0x2000;

        for (i, &byte) in bytes.iter().enumerate() {
            assert!(cpu.ram_write_byte(addr + i as u32, byte));
        }

        for (i, &expected) in bytes.iter().enumerate() {
            assert_eq!(cpu.ram_read_byte(addr + i as u32), expected);
        }
    }

    #[test]
    fn test_ram_byte_access_within_word() {
        let mut cpu = Cpu::new();

        // Write a full word
        assert!(cpu.ram_write_word(0x0000, 0x78563412));

        // Read individual bytes (little-endian)
        assert_eq!(cpu.ram_read_byte(0x0000), 0x12);
        assert_eq!(cpu.ram_read_byte(0x0001), 0x34);
        assert_eq!(cpu.ram_read_byte(0x0002), 0x56);
        assert_eq!(cpu.ram_read_byte(0x0003), 0x78);
    }

    #[test]
    fn test_ram_oob_byte_read_returns_zero() {
        let cpu = Cpu::new();

        assert_eq!(cpu.ram_read_byte(0xFFFF_FFFF), 0);
        assert_eq!(cpu.ram_read_byte(RAM_SIZE), 0);
        assert_eq!(cpu.ram_read_byte(RAM_SIZE + 1), 0);
    }

    #[test]
    fn test_ram_oob_byte_write_returns_false() {
        let mut cpu = Cpu::new();

        assert!(!cpu.ram_write_byte(0xFFFF_FFFF, 42));
        assert!(!cpu.ram_write_byte(RAM_SIZE, 42));
        assert!(!cpu.ram_write_byte(RAM_SIZE + 1, 42));
    }

    #[test]
    fn test_load_rom() {
        let mut cpu = Cpu::new();
        let rom = [0x0000_0001, 0x0000_0002, 0x0000_0003];

        cpu.load_rom(&rom);

        assert_eq!(cpu.rom.len(), 3);
        assert_eq!(cpu.rom[0], 0x0000_0001);
        assert_eq!(cpu.rom[1], 0x0000_0002);
        assert_eq!(cpu.rom[2], 0x0000_0003);
    }

    #[test]
    fn test_rom_read() {
        let mut cpu = Cpu::new();
        let rom = [0xAAAA_AAAA, 0xBBBB_BBBB, 0xCCCC_CCCC];

        cpu.load_rom(&rom);

        assert_eq!(cpu.rom_read(0), 0xAAAA_AAAA);
        assert_eq!(cpu.rom_read(1), 0xBBBB_BBBB);
        assert_eq!(cpu.rom_read(2), 0xCCCC_CCCC);
        assert_eq!(cpu.rom_read(3), 0); // OOB returns 0
    }

    #[test]
    fn test_rom_oob_returns_zero() {
        let cpu = Cpu::new();

        // No ROM loaded, all reads should return 0
        assert_eq!(cpu.rom_read(0), 0);
        assert_eq!(cpu.rom_read(100), 0);
        assert_eq!(cpu.rom_read(0xFFFF_FFFF), 0);
    }

    #[test]
    fn test_kbd_read_write() {
        let mut cpu = Cpu::new();

        assert_eq!(cpu.kbd_read(), 0);

        cpu.kbd_write(0x1234_5678);
        assert_eq!(cpu.kbd_read(), 0x1234_5678);

        cpu.kbd_write(0xDEAD_BEEF);
        assert_eq!(cpu.kbd_read(), 0xDEAD_BEEF);
    }

    #[test]
    fn test_is_halted() {
        let mut cpu = Cpu::new();

        assert!(!cpu.is_halted());

        cpu.state.halted = 1;
        assert!(cpu.is_halted());

        cpu.state.halted = 0;
        assert!(!cpu.is_halted());
    }

    #[test]
    fn test_is_sleeping() {
        let mut cpu = Cpu::new();

        assert!(!cpu.is_sleeping());

        cpu.state.sleep_until_ns = 1;
        assert!(cpu.is_sleeping());

        cpu.state.sleep_until_ns = 0;
        assert!(!cpu.is_sleeping());
    }

    #[test]
    fn test_cpu_default() {
        let cpu = Cpu::default();

        // Should be equivalent to Cpu::new()
        assert_eq!(cpu.state.regs[REG_SP as usize], REG_SP_DEFAULT);
        assert_eq!(cpu.state.pc, 0);
        assert_eq!(cpu.state.flags, 0);
    }

    #[test]
    fn test_screen_buffer_allocation() {
        let cpu = Cpu::new();

        assert_eq!(cpu.screen.len(), mbc_mmap::SCREEN_SIZE as usize);
        assert!(cpu.screen.iter().all(|&x| x == 0));
    }

    #[test]
    fn test_ram_word_size() {
        let cpu = Cpu::new();

        // Verify RAM is large enough for the address space
        assert!(cpu.ram.len() >= RAM_WORDS);
    }
}
