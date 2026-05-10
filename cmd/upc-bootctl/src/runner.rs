// SPDX-License-Identifier: GPL-3.0-or-later
//! Live BPF boot path for upc-bootctl. Lifts the aya pattern from
//! crates/doom-runner/src/main.rs and adapts it for xv6-mbc kernel
//! images per UPC Boot Protocol v2.
//!
//! ASCEND-LINUX Phase 1.1 SHIP per
//! references/battle-plan-phase11-ship-2026-05-10.md.

#![allow(dead_code)] // Phase 2 lays the foundation; Phase 4-6 wire callers.

use anyhow::{anyhow, bail, Context, Result};
use aya::maps::{Array, HashMap as AyaHashMap};
use aya::programs::Xdp;
use aya::Ebpf;
use std::path::Path;

/// CPU state for the MBC virtual machine — must match monad-common's
/// `MbcCpuState` exactly. ABI v2, 136 bytes (added priv_level +
/// reservation_address per ADR-067).
///
/// Lifted verbatim from `crates/doom-runner/src/main.rs` so the
/// `unsafe impl aya::Pod` and the const-assert on the size live in
/// one well-tested form. Future refactor: extract to a shared
/// `monad-common-host` crate so doom-runner and upc-bootctl share
/// the type instead of duplicating it.
#[repr(C)]
#[derive(Clone, Copy, Debug)]
pub struct MbcCpuState {
    pub regs: [u32; 16],          // 64 bytes
    pub pc: u32,                  // 4
    pub flags: u8,                // 1
    pub halted: u8,               // 1
    pub stalled: u8,              // 1
    pub _pad: u8,                 // 1
    pub sleep_until_ns: u64,      // 8
    pub insn_count: u64,          // 8
    pub cache_hits: u64,          // 8
    pub cache_misses: u64,        // 8
    pub interrupt_pending: u8,    // 1
    pub interrupt_vector: u8,     // 1
    pub interrupts_enabled: u8,   // 1
    pub _pad2: u8,                // 1
    pub tick_counter: u32,        // 4
    pub program_break: u32,       // 4
    pub exit_code: u32,           // 4
    pub current_pid: u8,          // 1
    pub num_processes: u8,        // 1
    pub mmu_enabled: u8,          // 1
    pub _pad3: u8,                // 1
    pub page_dir_base: u32,       // 4 (offset 124)
    // ── ASCEND-LINUX ABI v2 (ADR-067) ───────────────────────────────────
    pub priv_level: u8,           // 1 (offset 128) M=0/S=1/U=3
    pub _pad4: u8,                // 1
    pub _pad5: u8,                // 1
    pub _pad6: u8,                // 1
    pub reservation_address: u32, // 4 (offset 132) LR.W reservation tracker
} // total: 136

// Safety: MbcCpuState is #[repr(C)], Copy, contains only primitives.
unsafe impl aya::Pod for MbcCpuState {}

const _: () = assert!(std::mem::size_of::<MbcCpuState>() == 136);

/// Initial CPU state for an xv6 boot. PC=0 (slot 0 of ROM_MAP = first
/// MBC instruction of xv6-mbc.mbc = start_mbc.c::start entry — the .mbc
/// file packs from offset 0, NOT from kernel.ld's byte-address layout);
/// SP=0x03F0_0000 (byte address; MBC SP is byte-addressed for CALL/RET).
/// priv_level=0 (M-mode); reservation_address=0xFFFFFFFF (no LR pending).
pub fn xv6_initial_cpu_state() -> MbcCpuState {
    let mut state = MbcCpuState {
        regs: [0u32; 16],
        pc: 0, // slot 0 of ROM_MAP — first instruction of start_mbc.c
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
        program_break: 0,
        exit_code: 0,
        current_pid: 0,
        num_processes: 1,
        mmu_enabled: 0,
        _pad3: 0,
        page_dir_base: 0,
        priv_level: 0,
        _pad4: 0,
        _pad5: 0,
        _pad6: 0,
        reservation_address: 0xFFFF_FFFF,
    };
    state.regs[15] = 0x03F0_0000; // SP = stack top byte address
    state
}

/// BootRunner owns the eBPF object handle and the instance ID; methods
/// populate maps + attach XDP + drain TTY + cleanup.
pub struct BootRunner {
    ebpf: Ebpf,
    instance_id: u32,
}

impl BootRunner {
    pub fn open(ebpf_obj_path: &Path, instance_id: u32) -> Result<Self> {
        let ebpf = Ebpf::load_file(ebpf_obj_path).with_context(|| {
            format!("load eBPF object: {}", ebpf_obj_path.display())
        })?;
        Ok(Self { ebpf, instance_id })
    }

    /// Write `mbc_words` into ROM_MAP starting at slot 0. xv6-mbc.mbc is
    /// already laid out as [stage1 @ 0x10000, kernel @ 0x20000, ...] so a
    /// linear slot-0 write places stage1 at MBC PC index 0x4000 (which is
    /// byte address 0x10000) and the kernel at 0x8000.
    ///
    /// Note: this assumes the .mbc was assembled with the matching base
    /// addresses. If the kernel image has a non-zero base offset, the
    /// caller must shift `mbc_words` accordingly.
    pub fn populate_rom(&mut self, mbc_words: &[u32]) -> Result<()> {
        let mut rom: Array<_, u32> = Array::try_from(
            self.ebpf.map_mut("ROM_MAP").context("ROM_MAP not found")?,
        )?;
        for (i, &word) in mbc_words.iter().enumerate() {
            rom.set(i as u32, word, 0)
                .with_context(|| format!("ROM_MAP[{}] write", i))?;
        }
        tracing::info!(words = mbc_words.len(), "ROM_MAP populated");
        Ok(())
    }

    /// Write multiple byte regions into RAM_MAP. Each `(byte_addr, data)`
    /// is packed into u32 words (little-endian) and written at
    /// `byte_addr / 4`. Sub-word remainders are zero-padded.
    pub fn populate_ram(&mut self, regions: &[(u32, &[u8])]) -> Result<()> {
        let mut ram: Array<_, u32> = Array::try_from(
            self.ebpf.map_mut("RAM_MAP").context("RAM_MAP not found")?,
        )?;
        for &(byte_addr, data) in regions {
            if byte_addr % 4 != 0 {
                bail!(
                    "populate_ram: byte_addr 0x{:08X} not 4-byte aligned",
                    byte_addr
                );
            }
            let word_addr_base = byte_addr / 4;
            let mut chunks = data.chunks_exact(4);
            let mut written = 0u32;
            for (i, chunk) in chunks.by_ref().enumerate() {
                let w = u32::from_le_bytes([chunk[0], chunk[1], chunk[2], chunk[3]]);
                ram.set(word_addr_base + i as u32, w, 0).with_context(|| {
                    format!("RAM_MAP[0x{:08X}] write", byte_addr + (i * 4) as u32)
                })?;
                written = i as u32 + 1;
            }
            let rem = chunks.remainder();
            if !rem.is_empty() {
                let mut padded = [0u8; 4];
                padded[..rem.len()].copy_from_slice(rem);
                let w = u32::from_le_bytes(padded);
                ram.set(word_addr_base + written, w, 0)?;
            }
            tracing::info!(
                byte_addr = format!("0x{:08X}", byte_addr),
                bytes = data.len(),
                "RAM region written"
            );
        }
        Ok(())
    }

    /// Insert the initial CPU state for `instance_id` into CPU_MAP.
    pub fn populate_cpu(&mut self, initial_state: MbcCpuState) -> Result<()> {
        let mut cpu: AyaHashMap<_, u32, MbcCpuState> = AyaHashMap::try_from(
            self.ebpf.map_mut("CPU_MAP").context("CPU_MAP not found")?,
        )?;
        cpu.insert(self.instance_id, initial_state, 0).with_context(|| {
            format!("CPU_MAP[0x{:X}] insert", self.instance_id)
        })?;
        tracing::info!(instance = self.instance_id, "CPU_MAP populated");
        Ok(())
    }

    /// Load + attach the `monad_cpu` XDP program to `iface`.
    pub fn attach_xdp(&mut self, iface: &str) -> Result<()> {
        let prog: &mut Xdp = self
            .ebpf
            .program_mut("monad_cpu")
            .context("monad_cpu program not found in eBPF object")?
            .try_into()?;
        prog.load().context("XDP program load")?;
        prog.attach(iface, aya::programs::XdpFlags::default())
            .with_context(|| format!("XDP attach to {}", iface))?;
        tracing::info!(iface, "XDP attached");
        Ok(())
    }

    /// Read the current CPU state for `instance_id` from CPU_MAP.
    pub fn cpu_state(&self) -> Result<MbcCpuState> {
        let cpu: AyaHashMap<_, u32, MbcCpuState> = AyaHashMap::try_from(
            self.ebpf.map("CPU_MAP").context("CPU_MAP")?,
        )?;
        cpu.get(&self.instance_id, 0)
            .with_context(|| format!("CPU_MAP[0x{:X}] get", self.instance_id))
    }

    /// Drain new bytes from TTY_MAP since `head_cursor` was last read.
    /// TTY_MAP is a 4096-byte circular buffer; TTY_HEAD is the next-write
    /// position maintained by the eBPF interpreter on MMIO 0xC001 writes.
    pub fn read_tty(&self, head_cursor: &mut u32) -> Result<Vec<u8>> {
        let tty: Array<_, u8> = Array::try_from(
            self.ebpf.map("TTY_MAP").context("TTY_MAP")?,
        )?;
        let head_map: Array<_, u32> = Array::try_from(
            self.ebpf.map("TTY_HEAD").context("TTY_HEAD")?,
        )?;
        let new_head = head_map.get(&0u32, 0)?;
        if new_head == *head_cursor {
            return Ok(vec![]);
        }
        let mut bytes = Vec::new();
        let cap = 4096u32;
        let mut idx = *head_cursor;
        while idx != new_head {
            bytes.push(tty.get(&idx, 0)?);
            idx = (idx + 1) % cap;
        }
        *head_cursor = new_head;
        Ok(bytes)
    }

    /// Best-effort cleanup: remove this instance's CPU_MAP entry.
    /// SCREEN_MAP and KBD_MAP are arrays — caller can zero them via
    /// populate_ram() if desired (Phase 6 does this).
    pub fn cleanup(&mut self) -> Result<()> {
        let mut cpu: AyaHashMap<_, u32, MbcCpuState> = AyaHashMap::try_from(
            self.ebpf
                .map_mut("CPU_MAP")
                .context("CPU_MAP not found at cleanup")?,
        )?;
        let _ = cpu.remove(&self.instance_id);
        tracing::info!(instance = self.instance_id, "CPU_MAP entry removed");
        Ok(())
    }
}

// Module-level error helper — keeps anyhow::anyhow imports tidy if extended later.
#[allow(dead_code)]
fn _unused_anyhow_keepalive() -> anyhow::Error {
    anyhow!("placeholder")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn mbc_cpu_state_size_is_136() {
        assert_eq!(std::mem::size_of::<MbcCpuState>(), 136);
    }

    #[test]
    fn xv6_initial_cpu_state_has_correct_pc_and_sp() {
        let state = xv6_initial_cpu_state();
        // PC=0 is slot 0 of ROM_MAP = first MBC instruction of xv6-mbc.mbc
        // (.mbc files pack from offset 0; the .stage1 byte address from
        // kernel-mbc.ld is for the linker, not the ROM_MAP loader).
        assert_eq!(state.pc, 0);
        assert_eq!(state.regs[15], 0x03F0_0000); // SP = stack top
        assert_eq!(state.priv_level, 0); // M-mode
        assert_eq!(state.reservation_address, 0xFFFF_FFFF);
        assert_eq!(state.halted, 0);
        assert_eq!(state.num_processes, 1);
    }
}
