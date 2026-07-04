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
use std::fs;
use std::path::Path;

/// CPU state for the MBC virtual machine — must match monad-common's
/// `MbcCpuState` exactly. ASCEND ABI, 144 bytes: ADR-067's 136-byte v2
/// (priv_level + reservation_address) plus ADR-077's per-process
/// rv2mbc_base (+pad). upc-bootctl always loads the `--features
/// ascend-linux` object, so this mirror is unconditionally 144 (no cfg).
///
/// Lifted verbatim from `crates/doom-runner/src/main.rs` so the
/// `unsafe impl aya::Pod` and the const-assert on the size live in
/// one well-tested form. Future refactor: extract to a shared
/// `monad-common-host` crate so doom-runner and upc-bootctl share
/// the type instead of duplicating it.
#[repr(C)]
#[derive(Clone, Copy, Debug)]
pub struct MbcCpuState {
    pub regs: [u32; 16],        // 64 bytes
    pub pc: u32,                // 4
    pub flags: u8,              // 1
    pub halted: u8,             // 1
    pub stalled: u8,            // 1
    pub _pad: u8,               // 1
    pub sleep_until_ns: u64,    // 8
    pub insn_count: u64,        // 8
    pub cache_hits: u64,        // 8
    pub cache_misses: u64,      // 8
    pub interrupt_pending: u8,  // 1
    pub interrupt_vector: u8,   // 1
    pub interrupts_enabled: u8, // 1
    pub _pad2: u8,              // 1
    pub tick_counter: u32,      // 4
    pub program_break: u32,     // 4
    pub exit_code: u32,         // 4
    pub current_pid: u8,        // 1
    pub num_processes: u8,      // 1
    pub mmu_enabled: u8,        // 1
    pub _pad3: u8,              // 1
    pub page_dir_base: u32,     // 4 (offset 124)
    // ── ASCEND-LINUX ABI v2 (ADR-067) ───────────────────────────────────
    pub priv_level: u8,           // 1 (offset 128) M=0/S=1/U=3
    pub _pad4: u8,                // 1
    pub _pad5: u8,                // 1
    pub _pad6: u8,                // 1
    pub reservation_address: u32, // 4 (offset 132) LR.W reservation tracker
    // ── Phase 1.7 Gate B (ADR-077): per-process RV2MBC base ──────────────
    pub rv2mbc_base: u32, // 4 (offset 136) per-process RV2MBC_MAP offset
    pub _pad7: [u8; 4],   // 4 keep size_of % 8 == 0 (136 → 144)
} // total: 144

// Safety: MbcCpuState is #[repr(C)], Copy, contains only primitives.
unsafe impl aya::Pod for MbcCpuState {}

const _: () = assert!(std::mem::size_of::<MbcCpuState>() == 144);

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
        // ADR-074 Option A (2026-05-30): MMU on kingdom-wide, but the interpreter
        // only translates user-mode (priv 3) accesses — the kernel stays flat.
        // page_dir_base points at pid 0's fixed per-pid pgd region.
        mmu_enabled: 1,
        _pad3: 0,
        page_dir_base: 0x00F0_0000,
        priv_level: 0,
        _pad4: 0,
        _pad5: 0,
        _pad6: 0,
        reservation_address: 0xFFFF_FFFF,
        // pid 0 (init) lives at RV2MBC base 0 → identity lookups (ADR-077).
        rv2mbc_base: 0,
        _pad7: [0; 4],
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
        let ebpf = Ebpf::load_file(ebpf_obj_path)
            .with_context(|| format!("load eBPF object: {}", ebpf_obj_path.display()))?;
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
        self.populate_rom_at(0, mbc_words)
    }

    /// Populate `RV2MBC_MAP` from the `.rv2mbc` sibling file emitted by
    /// the rv32i-to-mbc translator. Format: a flat little-endian u32
    /// array where index = RV word offset from `.text` start, value =
    /// corresponding MBC PC (word index in ROM_MAP).
    ///
    /// `text_rv_word_base` shifts the load offset so that the BPF
    /// interpreter's absolute-RV-word lookups (JMPR, CALLR, MRET, SRET)
    /// land on the right entry. For xv6-mbc.mbc the `.text` section
    /// starts at RV byte 0x20000 (RV word 0x8000) per kernel-mbc.ld.
    ///
    /// Without this, JALR / CALLR / MRET / SRET land on unset entries
    /// (0 = MBC slot 0) and execution falls through to garbage. Phase
    /// 1.3 AP-2 fix.
    ///
    /// Phase 1.3 Step 6 (ADR-075 §Security #1): compute SHA-256 over the
    /// .rv2mbc bytes and surface it as a hex string. If `expected_sha`
    /// is Some, fail-fast when the computed hash mismatches —
    /// attacker-supplied `.rv2mbc` files would redirect every indirect
    /// branch in the loaded image. The caller (typically the
    /// `UPC_RV2MBC_SHA` env-var path in main.rs) decides whether to
    /// enforce.
    pub fn populate_rv2mbc(
        &mut self,
        bytes: &[u8],
        text_rv_word_base: u32,
        expected_sha: Option<&str>,
    ) -> Result<()> {
        if bytes.len() % 4 != 0 {
            bail!("rv2mbc file length {} not 4-byte aligned", bytes.len());
        }
        // Compute SHA-256 BEFORE touching BPF maps so a mismatch leaves
        // RV2MBC_MAP in its prior state (all-zero on a fresh boot).
        let computed_sha = {
            use sha2::{Digest, Sha256};
            let mut h = Sha256::new();
            h.update(bytes);
            let out = h.finalize();
            let mut s = String::with_capacity(64);
            for b in out.iter() {
                use std::fmt::Write;
                let _ = write!(&mut s, "{:02x}", b);
            }
            s
        };
        if let Some(want) = expected_sha {
            let want_norm = want.trim().to_ascii_lowercase();
            if want_norm != computed_sha {
                bail!(
                    "rv2mbc integrity gate FAILED: computed sha256={computed_sha}, expected={want_norm}"
                );
            }
            tracing::info!(sha256 = %computed_sha, "RV2MBC integrity gate PASSED");
        } else {
            tracing::info!(sha256 = %computed_sha, "RV2MBC SHA-256 logged (no gate enforced)");
        }
        let entries = bytes.len() / 4;
        let mut map: Array<_, u32> = Array::try_from(
            self.ebpf
                .map_mut("RV2MBC_MAP")
                .context("RV2MBC_MAP not found")?,
        )?;
        for i in 0..entries {
            let off = i * 4;
            let mbc_pc =
                u32::from_le_bytes([bytes[off], bytes[off + 1], bytes[off + 2], bytes[off + 3]]);
            let absolute_idx = text_rv_word_base + i as u32;
            map.set(absolute_idx, mbc_pc, 0)
                .with_context(|| format!("RV2MBC_MAP[{}] write", absolute_idx))?;
        }
        tracing::info!(
            entries = entries,
            base = text_rv_word_base,
            "RV2MBC_MAP populated"
        );
        Ok(())
    }

    /// Write `mbc_words` into ROM_MAP starting at slot `start_slot` (a
    /// word index). Used by Phase 2 to place the upc-bootstub.mbc at
    /// slot 0x4000 (byte 0x10000) and a separately-built uClinux/Linux
    /// kernel at slot 0x8000 (byte 0x20000). For Phase 1.1 xv6 the
    /// caller still uses populate_rom() (= start_slot=0) because xv6's
    /// .mbc image self-locates per its own kernel-mbc.ld layout.
    pub fn populate_rom_at(&mut self, start_slot: u32, mbc_words: &[u32]) -> Result<()> {
        let mut rom: Array<_, u32> =
            Array::try_from(self.ebpf.map_mut("ROM_MAP").context("ROM_MAP not found")?)?;
        for (i, &word) in mbc_words.iter().enumerate() {
            let slot = start_slot + i as u32;
            rom.set(slot, word, 0)
                .with_context(|| format!("ROM_MAP[{}] write", slot))?;
        }
        tracing::info!(
            start_slot = start_slot,
            words = mbc_words.len(),
            "ROM_MAP populated"
        );
        Ok(())
    }

    /// Build pid 0's identity page directory at its fixed per-pid pgd region
    /// (ADR-074 Allocator A1, `pgd_base`). Maps the low `n_superpages` × 4 MiB
    /// of the virtual address space identity-onto physical RAM using superpage
    /// leaf PDEs (`PTE_LEAF`). This is the Gate-1 "MMU live, identity slice"
    /// table: translation is a no-op transform, so the boot must still reach
    /// `init: starting sh` — proving the live walk + leaf + priv-gating path.
    ///
    /// PDE flag bits MUST match `monad_common::mmu` (PRESENT=1<<11, LEAF=1<<6,
    /// WRITE=1<<10, USER=1<<9); mirrored here because bootctl does not import
    /// the no_std common crate.
    pub fn populate_identity_pgd(&mut self, pgd_base: u32, n_superpages: u32) -> Result<()> {
        const PTE_PRESENT: u32 = 1 << 11;
        const PTE_LEAF: u32 = 1 << 6;
        const PTE_WRITE: u32 = 1 << 10;
        const PTE_USER: u32 = 1 << 9;
        let flags = PTE_PRESENT | PTE_LEAF | PTE_WRITE | PTE_USER;
        let mut bytes = Vec::with_capacity((n_superpages as usize) * 4);
        for k in 0..n_superpages {
            let pde = (k << 22) | flags; // identity: VA 4MiB-page k → PA 4MiB-page k
            bytes.extend_from_slice(&pde.to_le_bytes());
        }
        self.populate_ram(&[(pgd_base, &bytes)])?;
        tracing::info!(
            pgd_base = format!("0x{:08X}", pgd_base),
            superpages = n_superpages,
            "identity pgd written (pid 0, ADR-074 Gate 1)"
        );
        Ok(())
    }

    /// Write multiple byte regions into RAM_MAP. Each `(byte_addr, data)`
    /// is packed into u32 words (little-endian) and written at
    /// `byte_addr / 4`. Sub-word remainders are zero-padded.
    pub fn populate_ram(&mut self, regions: &[(u32, &[u8])]) -> Result<()> {
        let mut ram: Array<_, u32> =
            Array::try_from(self.ebpf.map_mut("RAM_MAP").context("RAM_MAP not found")?)?;
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

    /// Load a `.data` TLV image (emitted by rv32i-to-mbc alongside the
    /// `.mbc` artifact) into RAM_MAP at the byte-VMA addresses the C
    /// compiler's loaded immediates expect. Format: `[count u32 LE]
    /// [(byte_addr u32, len u32, bytes... padded to 4-byte alignment)]+`.
    ///
    /// Without this step, xv6's `lui a3, 0x27; addi a3, 1584` loads a
    /// byte address (0x27630) into a register, and the subsequent
    /// `lbu a4, 0(a3)` reads zero from RAM_MAP — the .rodata string
    /// "xv6 booting..." never reaches the TTY. Task #61 closure.
    pub fn load_data_image(&mut self, path: &Path) -> Result<()> {
        let blob =
            fs::read(path).with_context(|| format!("read data image: {}", path.display()))?;
        if blob.len() < 4 {
            anyhow::bail!(
                "data image too short ({} bytes); expected at least the count header",
                blob.len()
            );
        }
        let count = u32::from_le_bytes([blob[0], blob[1], blob[2], blob[3]]) as usize;
        let mut off = 4usize;
        let mut regions: Vec<(u32, Vec<u8>)> = Vec::with_capacity(count);
        for i in 0..count {
            if off + 8 > blob.len() {
                anyhow::bail!(
                    "data image truncated at record {} (off=0x{:x} blob_len=0x{:x})",
                    i,
                    off,
                    blob.len()
                );
            }
            let byte_addr =
                u32::from_le_bytes([blob[off], blob[off + 1], blob[off + 2], blob[off + 3]]);
            let len =
                u32::from_le_bytes([blob[off + 4], blob[off + 5], blob[off + 6], blob[off + 7]])
                    as usize;
            off += 8;
            if off + len > blob.len() {
                anyhow::bail!(
                    "data image record {} payload truncated (off=0x{:x} len=0x{:x} blob_len=0x{:x})",
                    i,
                    off,
                    len,
                    blob.len()
                );
            }
            let bytes = blob[off..off + len].to_vec();
            off += len;
            // Skip the 4-byte alignment padding the writer added.
            while off % 4 != 0 && off < blob.len() {
                off += 1;
            }
            regions.push((byte_addr, bytes));
        }
        tracing::info!(
            count = regions.len(),
            total_bytes = regions.iter().map(|(_, b)| b.len()).sum::<usize>(),
            "data image parsed"
        );
        // Reuse populate_ram so the byte-address → word-address layout is
        // handled identically to the existing IVT/BootParams/CSR writes.
        let region_refs: Vec<(u32, &[u8])> =
            regions.iter().map(|(a, b)| (*a, b.as_slice())).collect();
        self.populate_ram(&region_refs)
    }

    /// Parse a `.data` TLV blob into `(byte_addr, bytes)` regions — the same
    /// format `load_data_image` consumes, but returned to the caller instead
    /// of written at the linked VAs. Used by `stage_program_data` to build a
    /// flat exec-staging image for a non-pid-0 program (ADR-077 Gate B).
    fn parse_data_regions(blob: &[u8], what: &str) -> Result<Vec<(u32, Vec<u8>)>> {
        if blob.len() < 4 {
            bail!("{what}: data image too short ({} bytes)", blob.len());
        }
        let count = u32::from_le_bytes([blob[0], blob[1], blob[2], blob[3]]) as usize;
        let mut off = 4usize;
        let mut regions = Vec::with_capacity(count);
        for i in 0..count {
            if off + 8 > blob.len() {
                bail!("{what}: truncated at record {i}");
            }
            let byte_addr =
                u32::from_le_bytes([blob[off], blob[off + 1], blob[off + 2], blob[off + 3]]);
            let len =
                u32::from_le_bytes([blob[off + 4], blob[off + 5], blob[off + 6], blob[off + 7]])
                    as usize;
            off += 8;
            if off + len > blob.len() {
                bail!("{what}: record {i} payload truncated");
            }
            regions.push((byte_addr, blob[off..off + len].to_vec()));
            off += len;
            while off % 4 != 0 && off < blob.len() {
                off += 1;
            }
        }
        Ok(regions)
    }

    /// Stage a program's `.data`/`.rodata` as a single flat blob in a pristine
    /// RAM region so `exec(7)` can re-copy it into the running pid's physical
    /// slice (ADR-077 Gate B). Unlike `load_data_image` (which writes at the
    /// linked VAs for pid 0), this packs every region into a contiguous buffer
    /// `[min_va, max_va)` and writes it at `stage_byte_va` — well above the
    /// ramdisk and pid slices so fork-copy never clobbers it.
    ///
    /// Returns `(data_dst_va, data_len_words)`: the user VA where exec lands
    /// the blob (the linked min VA) and its word count. `(0, 0)` if the
    /// program has no initialized data.
    pub fn stage_program_data(
        &mut self,
        data_path: &Path,
        stage_byte_va: u32,
    ) -> Result<(u32, u32)> {
        let blob = fs::read(data_path)
            .with_context(|| format!("read program data: {}", data_path.display()))?;
        let regions = Self::parse_data_regions(&blob, &data_path.display().to_string())?;
        if regions.is_empty() {
            return Ok((0, 0));
        }
        let min_va = regions.iter().map(|(a, _)| *a).min().unwrap() & !3;
        let max_va = regions
            .iter()
            .map(|(a, b)| a + b.len() as u32)
            .max()
            .unwrap();
        let span = ((max_va - min_va) + 3) & !3; // round up to whole words
        let mut flat = vec![0u8; span as usize];
        for (va, bytes) in &regions {
            let off = (va - min_va) as usize;
            flat[off..off + bytes.len()].copy_from_slice(bytes);
        }
        // Stage the flat blob at the pristine high RAM address.
        self.populate_ram(&[(stage_byte_va, &flat)])?;
        Ok((min_va, span / 4))
    }

    /// Write one PROGRAM_TABLE row. `row` is the 8-word contract defined in
    /// `monad-cpu-ebpf`'s `pt` module (ADR-077 Gate B).
    pub fn populate_program_table(&mut self, slot: u32, row: [u32; 8]) -> Result<()> {
        let mut pt: Array<_, [u32; 8]> = Array::try_from(
            self.ebpf
                .map_mut("PROGRAM_TABLE")
                .context("PROGRAM_TABLE not found")?,
        )?;
        pt.set(slot, row, 0)
            .with_context(|| format!("PROGRAM_TABLE[{slot}] write"))?;
        Ok(())
    }

    /// Pre-fill the KBD ring with up to 64 scripted console-input bytes (Gate C
    /// `--input`; Gate D Phase 6 grew the ring 8→64 so a full command line like
    /// `cat README\n` fits). Sets KBD_MAP[i] = (byte<<1)|1 for i in 0..n,
    /// KBD_HEAD = n, KBD_TAIL = 0; the guest's read(5) drain pops one byte per
    /// call. Capped at the 64-slot KBD_MAP — errors (never silently truncates)
    /// on overflow.
    pub fn write_kbd(&mut self, bytes: &[u8]) -> Result<()> {
        let n = bytes.len();
        if n > 64 {
            bail!("--input exceeds KBD ring capacity (64 bytes), got {n}");
        }
        {
            let mut kbd: Array<_, u32> =
                Array::try_from(self.ebpf.map_mut("KBD_MAP").context("KBD_MAP not found")?)?;
            for (i, &b) in bytes.iter().enumerate() {
                kbd.set(i as u32, ((b as u32) << 1) | 1, 0)
                    .with_context(|| format!("KBD_MAP[{i}] write"))?;
            }
        }
        {
            let mut head: Array<_, u32> = Array::try_from(
                self.ebpf
                    .map_mut("KBD_HEAD")
                    .context("KBD_HEAD not found")?,
            )?;
            head.set(0, n as u32, 0).context("KBD_HEAD write")?;
        }
        {
            let mut tail: Array<_, u32> = Array::try_from(
                self.ebpf
                    .map_mut("KBD_TAIL")
                    .context("KBD_TAIL not found")?,
            )?;
            tail.set(0, 0u32, 0).context("KBD_TAIL write")?;
        }
        tracing::info!(bytes = n, "KBD ring pre-filled (--input)");
        Ok(())
    }

    /// Insert the initial CPU state for `instance_id` into CPU_MAP.
    pub fn populate_cpu(&mut self, initial_state: MbcCpuState) -> Result<()> {
        let mut cpu: AyaHashMap<_, u32, MbcCpuState> =
            AyaHashMap::try_from(self.ebpf.map_mut("CPU_MAP").context("CPU_MAP not found")?)?;
        cpu.insert(self.instance_id, initial_state, 0)
            .with_context(|| format!("CPU_MAP[0x{:X}] insert", self.instance_id))?;
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
        // Verifier-budget baseline (Phase 2.0 preflight): the kernel reports
        // the post-dead-code-elimination verified-instruction count once the
        // program loads. This is the number that races the 1M complexity
        // ceiling — the ONLY reliable way to read it for an Aya object, since
        // its legacy `maps` section can't be loaded by libbpf/veristat/bpftool.
        // Printed only when UPC_VERIFIER_STATS is set (keeps normal boots quiet).
        if std::env::var_os("UPC_VERIFIER_STATS").is_some() {
            match prog.info() {
                Ok(info) => match info.verified_instruction_count() {
                    Some(n) => println!(
                        "  VERIFIER: monad_cpu verified {n} insns ({:.1}% of 1M ceiling)",
                        n as f64 / 1_000_000.0 * 100.0
                    ),
                    None => println!("  VERIFIER: verified_insns unavailable (kernel < 5.16?)"),
                },
                Err(e) => println!("  VERIFIER: prog info unavailable: {e}"),
            }
        }
        prog.attach(iface, aya::programs::XdpFlags::default())
            .with_context(|| format!("XDP attach to {}", iface))?;
        tracing::info!(iface, "XDP attached");
        Ok(())
    }

    /// Read the current CPU state for `instance_id` from CPU_MAP.
    pub fn cpu_state(&self) -> Result<MbcCpuState> {
        let cpu: AyaHashMap<_, u32, MbcCpuState> =
            AyaHashMap::try_from(self.ebpf.map("CPU_MAP").context("CPU_MAP")?)?;
        cpu.get(&self.instance_id, 0)
            .with_context(|| format!("CPU_MAP[0x{:X}] get", self.instance_id))
    }

    /// Drain new bytes from TTY_MAP since `head_cursor` was last read.
    /// TTY_MAP is a 4096-byte circular buffer; TTY_HEAD is the next-write
    /// position maintained by the eBPF interpreter on MMIO 0xC001 writes.
    pub fn read_tty(&self, head_cursor: &mut u32) -> Result<Vec<u8>> {
        let tty: Array<_, u8> = Array::try_from(self.ebpf.map("TTY_MAP").context("TTY_MAP")?)?;
        let head_map: Array<_, u32> =
            Array::try_from(self.ebpf.map("TTY_HEAD").context("TTY_HEAD")?)?;
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

    /// Read selected STATS counters (non-wrapping, unlike the TTY ring) for
    /// post-mortem diagnosis. Returns one value per requested key (0 if unset).
    pub fn read_stats(&self, keys: &[u32]) -> Result<Vec<u64>> {
        let stats: AyaHashMap<_, u32, u64> =
            AyaHashMap::try_from(self.ebpf.map("STATS").context("STATS")?)?;
        Ok(keys.iter().map(|k| stats.get(k, 0).unwrap_or(0)).collect())
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

/// Route `BootRunner` through the shared image loader (Epic 1.2.3, ADR-080).
/// `write_rom` delegates to the existing `populate_rom_at`; `write_rv2mbc` runs
/// the same absolute-index `set(base + i, entry)` loop as `populate_rv2mbc`'s
/// inner body, minus the SHA gate — the `.rv2mbc` integrity gate stays on the
/// kernel path (which does not go through `upc_api::load`) since the userland
/// programs this sink serves carry no `expected_rv2mbc_sha256`.
impl upc_api::ImageSink for BootRunner {
    type Error = anyhow::Error;

    fn write_rom(&mut self, start_slot: u32, words: &[u32]) -> Result<()> {
        self.populate_rom_at(start_slot, words)
    }

    fn write_rv2mbc(&mut self, base: u32, entries: &[u32]) -> Result<()> {
        let mut map: Array<_, u32> = Array::try_from(
            self.ebpf
                .map_mut("RV2MBC_MAP")
                .context("RV2MBC_MAP not found")?,
        )?;
        for (i, &entry) in entries.iter().enumerate() {
            let idx = base + i as u32;
            map.set(idx, entry, 0)
                .with_context(|| format!("RV2MBC_MAP[{idx}] write"))?;
        }
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
    fn mbc_cpu_state_size_is_144() {
        // ASCEND ABI (ADR-077): 136-byte v2 + rv2mbc_base (+pad) = 144.
        assert_eq!(std::mem::size_of::<MbcCpuState>(), 144);
    }

    #[test]
    fn xv6_initial_cpu_state_rv2mbc_base_is_zero() {
        // pid 0 (init) → identity RV2MBC lookups.
        assert_eq!(xv6_initial_cpu_state().rv2mbc_base, 0);
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
