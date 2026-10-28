// SPDX-License-Identifier: GPL-3.0-only
//! doom-runner — Aya-based Doom-over-IPv6 runtime.
//!
//! Replaces the fragile shell script + Go loader chain with a single Rust binary
//! that owns the ENTIRE pipeline: eBPF program loading creates maps, and
//! doom-runner writes data directly into THOSE maps. No pins. No mismatches.
//!
//! This solves the systemic map alignment bug where doom-ring.sh pins BPF maps
//! but the XDP program creates its OWN maps with different IDs.
//!
//! # Usage
//!
//! ```bash
//! # Full pipeline: setup ring, load XDP via Aya, load Doom data, start bridge
//! sudo doom-runner run --doom-mbc doom/doom.mbc --doom-elf doom/doom.elf \
//!     --rv2mbc doom/doom.rv2mbc --wad doom1.wad --hops 2
//!
//! # `doom/doom.rv2mbc` is checked against `doom/doom.rv2mbc.sha256` (or
//! # --rv2mbc-sha) before the ring or any map exists; --dry-run stops there.
//!
//! # Just set up the network ring
//! sudo doom-runner ring setup
//!
//! # Tear down everything
//! sudo doom-runner ring teardown
//! ```

pub mod bridge;
pub mod loader;
pub mod memory;
pub mod ring;
pub mod tail_calls;

use std::path::PathBuf;
use std::sync::{Arc, Mutex};
use std::time::Instant;

use anyhow::{bail, Context, Result};
use aya::maps::{Array, HashMap as AyaHashMap, ProgramArray};
use aya::programs::Xdp;
use aya::Ebpf;
use clap::{Parser, Subcommand};
use tracing::{error, info, warn};
use tracing_subscriber::EnvFilter;

// ── MbcCpuState replica ─────────────────────────────────────────────────────
//
// Must match monad-common's MbcCpuState exactly: 136 bytes (ABI v2),
// #[repr(C)]. We define it here to avoid pulling monad-common (which
// targets both bpfel-unknown-none and host). Verified by size_of assertion.
//
// ABI v2 (ASCEND-LINUX, ADR-067) added priv_level + reservation_address;
// Doom doesn't use these but the BPF map shape must match the eBPF program.

/// CPU state for the MBC virtual machine — must match monad-common exactly.
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
} // total: 136 (was 128 in ABI v1)

// Safety: MbcCpuState is #[repr(C)], Copy, contains only primitives.
unsafe impl aya::Pod for MbcCpuState {}

const _: () = assert!(std::mem::size_of::<MbcCpuState>() == 136);

/// Doom's syscall surface (contract part 4, ADR-080): exactly the ecall numbers
/// the default (non-ascend) interpreter build has an arm for
/// (`ebpf/monad-cpu-ebpf/surface.rs::SURFACE_DEFAULT`): DRAW_FRAME..SLEEP are
/// `monad_common::mbc_syscalls` 1..4, write 16 and READ_BLOCK/WRITE_BLOCK
/// 200/201 are compiled into both builds. (This list used 0..3 until
/// S-ASCEND-2, copied from `demos/doom/doomgeneric_unheaded.c`, which still
/// defines them that way.) Gate A refuses an object whose `UPC_SURFACE_ID` is
/// not the id of this list. Host-side descriptor only.
static DOOM_SYSCALLS: [upc_api::SyscallDesc; 7] = [
    upc_api::SyscallDesc {
        number: 1,
        name: "DRAW_FRAME",
    },
    upc_api::SyscallDesc {
        number: 2,
        name: "GET_KEY",
    },
    upc_api::SyscallDesc {
        number: 3,
        name: "GET_TICKS",
    },
    upc_api::SyscallDesc {
        number: 4,
        name: "SLEEP",
    },
    upc_api::SyscallDesc {
        number: 16,
        name: "write",
    },
    upc_api::SyscallDesc {
        number: 200,
        name: "read_block",
    },
    upc_api::SyscallDesc {
        number: 201,
        name: "write_block",
    },
];

/// Doom's surface descriptor: the default-build syscall allowlist.
static DOOM_SURFACE: upc_api::SyscallSurface = upc_api::SyscallSurface {
    name: "doom",
    feature_gate: upc_api::FeatureGate::Default,
    syscalls: &DOOM_SYSCALLS,
};

/// The BPF-map capacities `upc_api::load` validates Doom's Workload against. Doom
/// always runs on the non-ascend (default-image) build, so these are the DEFAULT
/// map sizes, taken from `crate::memory` (which mirrors `monad_common::mbc_maps`'
/// DEFAULT values). Unlike the multi-object `upc-bootctl` loader — which cannot
/// know at compile time whether it loaded a `large-image` object and so guards
/// against the `*_LARGE` ceilings — doom-runner has exactly one known build, so
/// the tighter DEFAULT bounds are both correct and stricter.
fn doom_map_capacities() -> upc_api::MapCapacities {
    upc_api::MapCapacities {
        rom_words: memory::ROM_MAP_ENTRIES,
        rv2mbc_entries: memory::RV2MBC_MAP_ENTRIES,
        ram_words: memory::RAM_MAP_ENTRIES,
    }
}

/// Build Doom's `Workload`: the MBC image at ROM slot 0, its `.rv2mbc` table at
/// index 0, entry PC 0 (Doom enters at the first ROM slot), flat memory, direct
/// boot. RAM (WAD + ELF data) and the initial CPU state are populated separately
/// — they are outside contract parts 1+2. `rv2mbc_sha256` is the hash
/// [`checked_doom_rv2mbc`] verified.
fn doom_workload(
    rom: Vec<u32>,
    rv2mbc: Vec<u32>,
    rv2mbc_sha256: Option<String>,
) -> upc_api::Workload {
    upc_api::Workload {
        name: "doom".to_string(),
        image: upc_api::GuestImage {
            rom,
            rom_start_slot: 0,
            rv2mbc,
            rv2mbc_word_base: 0,
            expected_rv2mbc_sha256: rv2mbc_sha256,
            entry: upc_api::MbcPc(0),
        },
        memory: upc_api::MemoryModel::Flat,
        surface: DOOM_SURFACE,
        boot: upc_api::BootProtocol::Direct,
        mmu: None,
    }
}

/// Doom's `.rv2mbc`, read once and SHA-checked (S-ASCEND-2 Item 5).
#[derive(Debug)]
struct DoomRv2mbc {
    entries: Vec<u32>,
    /// The verified SHA-256 (hex). `None` only under the dry-run hatch.
    sha256: Option<String>,
}

/// `--allow-unverified-rv2mbc` is a lab-only hatch: accepted with `--dry-run`,
/// refused on a live run.
fn hatch_gate(allow_unverified: bool, dry_run: bool) -> Result<()> {
    if allow_unverified && !dry_run {
        bail!(
            "--allow-unverified-rv2mbc is accepted only with --dry-run; a live run \
             verifies doom.rv2mbc (commit or regenerate doom/doom.rv2mbc.sha256)"
        );
    }
    Ok(())
}

/// Read `path` once and check it with the shared resolver (`explicit`, else
/// `<path>.sha256`, else refuse) before anything writes RV2MBC_MAP. Under the
/// hatch a missing hash is a warning; a mismatch is refused either way. No
/// `--rv2mbc` loads nothing (indirect jumps then do not work, as before).
fn checked_doom_rv2mbc(
    path: Option<&std::path::Path>,
    explicit: Option<&str>,
    allow_unverified: bool,
) -> Result<Option<DoomRv2mbc>> {
    let Some(path) = path else {
        return Ok(None);
    };
    let (bytes, sha256) = match upc_api::rv2mbc_sha::read_verified(path, explicit) {
        Ok(v) => (v.bytes, Some(upc_api::object::hex32(&v.sha256))),
        Err(e @ upc_api::rv2mbc_sha::Rv2mbcShaError::Required { .. }) if allow_unverified => {
            warn!("--allow-unverified-rv2mbc: {e}; continuing unverified (dry run)");
            let bytes = std::fs::read(path)
                .with_context(|| format!("failed to read rv2mbc: {}", path.display()))?;
            (bytes, None)
        }
        Err(e) => bail!("refusing to load: {e}"),
    };
    let entries = loader::parse_rv2mbc(&bytes, path)?;
    Ok(Some(DoomRv2mbc { entries, sha256 }))
}

/// Gate A (and Gate B, which a flat guest passes) on the object file before
/// the ring is built or aya creates a map: the ascend object under Doom is
/// refused here. `upc_api::load` runs both again before its first write.
fn preflight_object(ebpf_obj: &std::path::Path) -> Result<upc_api::ObjectFacts> {
    let facts = upc_api::ObjectFacts::from_path(ebpf_obj)
        .map_err(|e| anyhow::anyhow!("refusing to load {}: {e}", ebpf_obj.display()))?;
    upc_api::check_surface(&DOOM_SURFACE, &facts)
        .and_then(|()| {
            upc_api::check_mmu(
                upc_api::MemoryModel::Flat,
                None,
                &doom_map_capacities(),
                &facts,
            )
        })
        .map_err(|e| anyhow::anyhow!("refusing to load {}: {e}", ebpf_obj.display()))?;
    Ok(facts)
}

/// An [`upc_api::ImageSink`] over the live Aya eBPF object. `write_rom` /
/// `write_rv2mbc` open their map once and run the same `set(base + i, value)`
/// loop doom-runner used inline, so routing through `upc_api::load` writes
/// byte-identical `ROM_MAP` / `RV2MBC_MAP` contents.
struct EbpfImageSink<'a> {
    ebpf: &'a mut Ebpf,
}

impl upc_api::ImageSink for EbpfImageSink<'_> {
    type Error = anyhow::Error;

    fn write_rom(&mut self, start_slot: u32, words: &[u32]) -> Result<()> {
        let mut rom_map: Array<_, u32> = Array::try_from(
            self.ebpf
                .map_mut("ROM_MAP")
                .context("ROM_MAP not found in eBPF program")?,
        )?;
        for (i, &insn) in words.iter().enumerate() {
            let idx = start_slot + i as u32;
            rom_map
                .set(idx, insn, 0)
                .with_context(|| format!("ROM_MAP write failed at index {idx}"))?;
        }
        Ok(())
    }

    fn write_rv2mbc(&mut self, base: u32, entries: &[u32]) -> Result<()> {
        let mut rv2mbc_map: Array<_, u32> = Array::try_from(
            self.ebpf
                .map_mut("RV2MBC_MAP")
                .context("RV2MBC_MAP not found in eBPF program")?,
        )?;
        for (i, &val) in entries.iter().enumerate() {
            let idx = base + i as u32;
            rv2mbc_map
                .set(idx, val, 0)
                .with_context(|| format!("RV2MBC_MAP write failed at index {idx}"))?;
        }
        Ok(())
    }
}

/// CPU instance ID for Doom (0xDE — matches Go loader convention).
const CPU_INSTANCE_ID: u32 = 0xDE;

/// Default SP value — must match Go loader's defaultSP (0x3F00000).
/// This is a word address used directly by CALL/RET in the MBC VM.
const DEFAULT_SP: u32 = 0x03F0_0000;

// ── CLI ─────────────────────────────────────────────────────────────────────

#[derive(Parser, Debug)]
#[command(
    name = "doom-runner",
    about = "Doom-over-IPv6 runtime — Aya-based BPF program loader and memory manager",
    version
)]
struct Cli {
    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand, Debug)]
enum Commands {
    /// Full pipeline: setup ring, load XDP+Doom via Aya, attach, start bridge.
    Run {
        /// Path to doom.mbc (pre-translated MBC binary).
        #[arg(long)]
        doom_mbc: PathBuf,

        /// Path to doom.elf (linked RISC-V binary).
        #[arg(long)]
        doom_elf: PathBuf,

        /// Path to doom.rv2mbc (RV32I-to-MBC address map).
        #[arg(long)]
        rv2mbc: Option<PathBuf>,

        /// Path to doom1.wad.
        #[arg(long)]
        wad: PathBuf,

        /// Path to monad-cpu-ebpf ELF object.
        #[arg(
            long,
            default_value = "ebpf/target/bpfel-unknown-none/release/monad-cpu-ebpf"
        )]
        ebpf_obj: PathBuf,

        /// Number of hops in the packet ring (2-6).
        #[arg(long, default_value = "2")]
        hops: u32,

        /// Tail call chain depth (1-33).
        #[arg(long, default_value = "16")]
        chain_depth: usize,

        /// Bridge listen address.
        #[arg(long, default_value = "0.0.0.0:16666")]
        bridge_addr: String,

        /// Skip network ring setup (assume already configured).
        #[arg(long)]
        skip_ring: bool,

        /// Skip bridge server (headless mode).
        #[arg(long)]
        headless: bool,

        /// Expected SHA-256 (64 hex digits) of the `--rv2mbc` file. Else its
        /// `<file>.sha256` sidecar; with neither the run is refused.
        #[arg(long, value_name = "HEX")]
        rv2mbc_sha: Option<String>,

        /// Lab-only: continue when the `.rv2mbc` has no expected SHA-256
        /// (warning). Accepted only with --dry-run; never excuses a mismatch.
        #[arg(long)]
        allow_unverified_rv2mbc: bool,

        /// Check the layout, the object gates and the `.rv2mbc` SHA, then
        /// exit: no ring, no eBPF load, no map write.
        #[arg(long)]
        dry_run: bool,
    },

    /// Network ring management.
    Ring {
        #[command(subcommand)]
        action: RingAction,
    },

    /// Screen bridge server (standalone, reads from pinned maps).
    Bridge {
        /// Listen address.
        #[arg(long, default_value = "0.0.0.0:16666")]
        addr: String,
    },

    /// Validate memory layout and print address map.
    Layout,
}

#[derive(Subcommand, Debug)]
enum RingAction {
    /// Create namespaces, veth pairs, routes.
    Setup {
        /// Number of hops (2-6).
        #[arg(long, default_value = "2")]
        hops: u32,
    },
    /// Destroy ring and remove BPF pins.
    Teardown {
        /// Number of hops (must match setup).
        #[arg(long, default_value = "2")]
        hops: u32,
    },
    /// Show ring status.
    Status {
        #[arg(long, default_value = "2")]
        hops: u32,
    },
}

#[tokio::main]
async fn main() -> Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(
            EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| EnvFilter::new("doom_runner=info")),
        )
        .init();

    let cli = Cli::parse();

    match cli.command {
        Commands::Run {
            doom_mbc,
            doom_elf,
            rv2mbc,
            wad,
            ebpf_obj,
            hops,
            chain_depth,
            bridge_addr,
            skip_ring,
            headless,
            rv2mbc_sha,
            allow_unverified_rv2mbc,
            dry_run,
        } => {
            cmd_run(
                doom_mbc,
                doom_elf,
                rv2mbc,
                wad,
                ebpf_obj,
                hops,
                chain_depth,
                bridge_addr,
                skip_ring,
                headless,
                rv2mbc_sha,
                allow_unverified_rv2mbc,
                dry_run,
            )
            .await
        }
        Commands::Ring { action } => cmd_ring(action),
        Commands::Bridge { addr } => cmd_bridge(addr).await,
        Commands::Layout => cmd_layout(),
    }
}

// ── Full Pipeline ───────────────────────────────────────────────────────────

/// Full pipeline: ring setup -> load XDP via Aya -> load Doom data -> verify -> bridge.
#[allow(clippy::too_many_arguments)]
async fn cmd_run(
    mbc_path: PathBuf,
    elf_path: PathBuf,
    rv2mbc_path: Option<PathBuf>,
    wad_path: PathBuf,
    ebpf_obj: PathBuf,
    hops: u32,
    chain_depth: usize,
    bridge_addr: String,
    skip_ring: bool,
    headless: bool,
    rv2mbc_sha: Option<String>,
    allow_unverified_rv2mbc: bool,
    dry_run: bool,
) -> Result<()> {
    info!("doom-runner: Aya-based full pipeline (no pins, no mismatches)");
    let t0 = Instant::now();
    hatch_gate(allow_unverified_rv2mbc, dry_run)?;

    // Step 0: Validate memory layout
    memory::validate_layout().context("memory layout validation failed")?;
    info!("memory layout validated");

    // Step 0b: the object's surface must be Doom's (S-ASCEND-2 Steps 95-106).
    // A dry run without an object skips it (the gates read the file only).
    let facts = if dry_run && !ebpf_obj.exists() {
        info!(
            "dry run: object gates not run: no object at {}",
            ebpf_obj.display()
        );
        None
    } else {
        let facts = preflight_object(&ebpf_obj)?;
        info!(
            "object gates passed: {} carries Doom's surface",
            ebpf_obj.display()
        );
        Some(facts)
    };

    // Step 0c: the .rv2mbc SHA-256 gate (S-ASCEND-2 Item 5), before the ring or
    // any map exists. These bytes are the ones written to RV2MBC_MAP below.
    let rv2mbc_checked = checked_doom_rv2mbc(
        rv2mbc_path.as_deref(),
        rv2mbc_sha.as_deref(),
        allow_unverified_rv2mbc,
    )?;
    match &rv2mbc_checked {
        Some(DoomRv2mbc {
            entries,
            sha256: Some(h),
        }) => info!("RV2MBC: {} entries, sha256 {h} verified", entries.len()),
        Some(DoomRv2mbc { entries, .. }) => {
            warn!("RV2MBC: {} entries UNVERIFIED (dry run)", entries.len())
        }
        None => warn!("no rv2mbc file provided — indirect jumps will not work"),
    }
    if dry_run {
        println!("dry-run complete: no ring, no eBPF load, no map write");
        return Ok(());
    }
    let facts = facts.context("object facts missing on a live run")?;

    // Step 1: Set up network ring (namespaces + veth pairs)
    if !skip_ring {
        let ring_config = ring::RingConfig {
            num_hops: hops,
            ebpf_obj_path: ebpf_obj.display().to_string(),
            ..Default::default()
        };
        ring::setup(&ring_config).context("ring setup failed")?;
        info!("network ring ready ({hops} hops)");
    } else {
        info!("skipping ring setup (--skip-ring)");
    }

    // Step 2: Load eBPF program via Aya — THIS creates the maps
    info!("loading eBPF program from {}", ebpf_obj.display());
    let mut ebpf = Ebpf::load_file(&ebpf_obj)
        .with_context(|| format!("failed to load eBPF object: {}", ebpf_obj.display()))?;
    info!("eBPF object loaded");

    // List all maps for diagnostics
    let map_names: Vec<String> = ebpf.maps().map(|(name, _)| name.to_string()).collect();
    info!("maps created by eBPF program: {:?}", map_names);

    // Step 3: Load and attach XDP program to veth interfaces
    {
        let program: &mut Xdp = ebpf
            .program_mut("monad_cpu")
            .context("monad_cpu XDP program not found in eBPF object")?
            .try_into()
            .context("monad_cpu is not an XDP program")?;
        program
            .load()
            .context("failed to load monad_cpu XDP program")?;
        info!("monad_cpu XDP program loaded into kernel");

        // UPC_VERIFIER_STATS: report the kernel's post-load verified-instruction
        // count for the non-ascend (Doom) object. This is the only reliable read
        // for an Aya object (its legacy maps section can't be loaded by
        // libbpf/veristat/bpftool). Mirrors cmd/upc-bootctl; the regression
        // harness (scripts/upc-regression.sh) captures it. Printed only when set.
        if std::env::var_os("UPC_VERIFIER_STATS").is_some() {
            match program.info() {
                Ok(i) => match i.verified_instruction_count() {
                    Some(n) => println!(
                        "  VERIFIER: monad_cpu verified {n} insns ({:.1}% of 1M ceiling)",
                        n as f64 / 1_000_000.0 * 100.0
                    ),
                    None => println!("  VERIFIER: verified_insns unavailable (kernel < 5.16?)"),
                },
                Err(e) => println!("  VERIFIER: prog info unavailable: {e}"),
            }
        }

        // Attach to veth interfaces in the ring
        // For a 2-hop ring: veth01p (in monad1) and veth10p (in monad0)
        // We attach in the default namespace since the veths were moved there
        // Actually, for namespace-local attachment we need to use netns fd.
        // For now, attach to interfaces visible in our namespace.
        // The ring setup moves veths into namespaces, so we need nsenter.
        // Aya's attach takes an interface name — it works with interfaces in current ns.
        //
        // For production: we'll use ip netns exec to attach. For now, log and continue.
        // The XDP program will be attached after we move to the namespace.
        info!("XDP program loaded — attachment to veth interfaces requires namespace context");
        info!("(the program and maps are now active in the kernel)");
    }

    // Step 3b: Set up tail call chain for higher throughput
    // Populate TAIL_CALL_PROGS[0] with monad_cpu's own FD so it can self-tail-call.
    {
        let tc_config = tail_calls::TailCallConfig::new(ebpf_obj.display().to_string());
        let tc_plan = tail_calls::plan(tail_calls::TailCallConfig {
            chain_depth,
            ..tc_config
        })?;

        // Clone the program FD first (immutable borrow), then drop it before
        // taking the mutable borrow for the prog array map.
        let prog_fd = {
            let prog: &Xdp = ebpf
                .program("monad_cpu")
                .context("monad_cpu program not found for tail call setup")?
                .try_into()
                .context("monad_cpu is not an XDP program")?;
            prog.fd()
                .context("monad_cpu has no FD — was it loaded?")?
                .try_clone()
                .context("failed to clone monad_cpu FD")?
        }; // immutable borrow of `ebpf` dropped here

        let mut prog_array = ProgramArray::try_from(
            ebpf.map_mut("TAIL_CALL_PROGS")
                .context("TAIL_CALL_PROGS map not found in eBPF program")?,
        )
        .context("TAIL_CALL_PROGS is not a ProgramArray")?;

        prog_array
            .set(0, &prog_fd, 0)
            .context("failed to set monad_cpu in TAIL_CALL_PROGS[0]")?;

        info!(
            "tail call chain: TAIL_CALL_PROGS[0] = monad_cpu (self), \
             {} insns/tick ({} rounds x {} insns)",
            tc_plan.total_insns_per_tick,
            tc_plan.config.chain_depth,
            tail_calls::INSNS_PER_PROGRAM,
        );
    }

    // Step 4: Parse all data files
    info!("parsing doom data files...");

    let sections = loader::parse_elf(&elf_path).context("failed to parse doom.elf")?;
    info!(
        "ELF: {} data sections, bss={:#x}+{:#x}, entry=rv:{:#010x}",
        sections.data_sections.len(),
        sections.bss_start,
        sections.bss_len,
        sections.entry_rv_addr,
    );

    let rom = loader::load_mbc_rom(&mbc_path).context("failed to load doom.mbc")?;
    info!(
        "ROM: {} MBC instructions ({:.1} KiB)",
        rom.len(),
        rom.len() as f64 * 4.0 / 1024.0
    );

    let (rv2mbc_data, rv2mbc_sha256) = match rv2mbc_checked {
        Some(r) => (Some(r.entries), r.sha256),
        None => (None, None),
    };

    let wad_data = loader::load_wad(&wad_path).context("failed to load WAD")?;

    let ram_updates = loader::stage_ram_image(&sections.data_sections, Some(&wad_data))?;

    // heap_ptr is now a normal BSS variable in the program — no magic address.
    // The linker script defines __heap_start/__heap_end. The program initializes
    // heap_ptr from __heap_start on first malloc (JVM/LXD-style self-contained heap).
    // doom-runner does NOT need to write anything for heap initialization.

    info!("RAM: {} non-zero words to write", ram_updates.len());

    // Step 5: Write data into the REAL maps (the ones the XDP program uses)
    info!("writing data into eBPF maps (Aya-owned, no pins)...");

    // 5a + 5c: ROM_MAP + RV2MBC_MAP via the shared image loader (Epic 1.2.3,
    // ADR-080). Build Doom's Workload and route both map writes through
    // upc_api::load: validate() (image bounds, entry-in-ROM, every rv2mbc target
    // in ROM) then the same set(i, value) loops doom-runner ran inline. A missing
    // .rv2mbc yields an empty table (already warned above) — no writes, matching
    // the prior `if let Some` skip. RAM_MAP (5b) and CPU_MAP (5d) stay Doom-
    // specific and are populated below.
    let workload = doom_workload(rom, rv2mbc_data.unwrap_or_default(), rv2mbc_sha256);
    {
        let t = Instant::now();
        let n_rom = workload.image.rom.len();
        let n_rv = workload.image.rv2mbc.len();
        let mut sink = EbpfImageSink { ebpf: &mut ebpf };
        upc_api::load(&workload, &doom_map_capacities(), &facts, &mut sink)
            .map_err(|e| anyhow::anyhow!("doom image load: {e}"))?;
        info!(
            "ROM_MAP + RV2MBC_MAP: {n_rom} + {n_rv} entries written in {:.1}s (upc_api::load)",
            t.elapsed().as_secs_f64()
        );
    }

    // 5b: RAM_MAP — ELF data sections + WAD
    {
        let t = Instant::now();
        let mut ram_map: Array<_, u32> = Array::try_from(
            ebpf.map_mut("RAM_MAP")
                .context("RAM_MAP not found in eBPF program")?,
        )?;
        info!("RAM_MAP: writing {} non-zero words...", ram_updates.len());
        let total = ram_updates.len();
        for (n, (idx, val)) in ram_updates.iter().enumerate() {
            ram_map
                .set(*idx, val, 0)
                .with_context(|| format!("RAM_MAP write failed at word index {idx:#x}"))?;
            if (n + 1) % 100_000 == 0 {
                let pct = (n + 1) as f64 / total as f64 * 100.0;
                info!("RAM_MAP: [{pct:.1}%] {}/{total} words written", n + 1);
            }
        }
        info!(
            "RAM_MAP: {} entries written in {:.1}s",
            total,
            t.elapsed().as_secs_f64()
        );
    }

    // (ROM_MAP + RV2MBC_MAP were written together above via upc_api::load.)

    // 5d: CPU_MAP — initial CPU state
    {
        let mut cpu_map: AyaHashMap<_, u32, MbcCpuState> = AyaHashMap::try_from(
            ebpf.map_mut("CPU_MAP")
                .context("CPU_MAP not found in eBPF program")?,
        )?;

        let mut cpu = MbcCpuState {
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
            program_break: memory::HEAP_START,
            exit_code: 0,
            current_pid: 0,
            num_processes: 1,
            mmu_enabled: 0,
            _pad3: 0,
            page_dir_base: 0,
            // ABI v2 (ASCEND-LINUX, ADR-067) — Doom runs in single-ring mode
            // with no LR/SC active.
            priv_level: 1, // S-mode (Doom doesn't distinguish; 1 is harmless)
            _pad4: 0,
            _pad5: 0,
            _pad6: 0,
            reservation_address: 0xFFFF_FFFF,
        };
        // SP = stack top (used directly as word address by CALL/RET)
        cpu.regs[15] = DEFAULT_SP;
        // a0 = heap start, a1 = heap size (Doom's Z_Init arguments)
        cpu.regs[10] = memory::HEAP_START;
        cpu.regs[11] = memory::HEAP_SIZE;

        cpu_map
            .insert(CPU_INSTANCE_ID, cpu, 0)
            .context("CPU_MAP insert failed")?;
        info!(
            "CPU_MAP: instance {CPU_INSTANCE_ID:#x} initialized (pc=0, sp={:#x}, heap={:#x}+{:#x})",
            cpu.regs[15], cpu.regs[10], cpu.regs[11],
        );
    }

    // Step 6: Verify loaded data
    info!("verifying loaded data...");
    verify_maps(&mut ebpf, &workload.image.rom, &ram_updates, &wad_data)?;

    let elapsed = t0.elapsed();
    info!(
        "doom-runner: pipeline complete in {:.1}s — Doom is loaded and ready",
        elapsed.as_secs_f64()
    );
    info!("send tick packets to veth interfaces to start MBC execution");

    // Step 7: Wrap Ebpf in Arc<Mutex<>> and start bridge or hold alive.
    // CRITICAL: if this process exits, kernel GCs the program + maps → all data lost.
    let ebpf = Arc::new(Mutex::new(ebpf));

    if headless {
        info!("doom-runner: headless mode — holding BPF program + maps alive");
        info!("attach XDP and inject packets to start Doom execution");
        tokio::signal::ctrl_c().await?;
        info!("shutting down — BPF program + maps will be released");
    } else {
        info!("doom-runner: starting integrated WebSocket bridge");
        let bridge_config = bridge::BridgeConfig {
            listen_addr: bridge_addr,
            ..Default::default()
        };
        bridge::serve(ebpf, &bridge_config).await?;
        info!("shutting down — BPF program + maps will be released");
    }

    Ok(())
}

/// Verify that data was correctly written to the maps.
fn verify_maps(
    ebpf: &mut Ebpf,
    rom: &[u32],
    _ram_updates: &[(u32, u32)],
    wad_data: &[u8],
) -> Result<()> {
    // Verify ROM_MAP[0]
    {
        let rom_map: Array<_, u32> =
            Array::try_from(ebpf.map_mut("ROM_MAP").context("ROM_MAP not found")?)?;
        let first_insn = rom_map
            .get(&0, 0)
            .context("ROM_MAP read failed at index 0")?;
        if first_insn != rom[0] {
            bail!(
                "ROM_MAP verification FAILED: expected {:#010x} at index 0, got {:#010x}",
                rom[0],
                first_insn,
            );
        }
        // Also check last instruction
        let last_idx = (rom.len() - 1) as u32;
        let last_insn = rom_map
            .get(&last_idx, 0)
            .with_context(|| format!("ROM_MAP read failed at index {last_idx}"))?;
        if last_insn != rom[rom.len() - 1] {
            bail!(
                "ROM_MAP verification FAILED at last index {last_idx}: expected {:#010x}, got {:#010x}",
                rom[rom.len() - 1], last_insn,
            );
        }
        info!(
            "ROM_MAP: PASS (first={:#010x}, last={:#010x}, {} entries)",
            first_insn,
            last_insn,
            rom.len()
        );
    }

    // Verify CPU_MAP[0xDE]
    {
        let cpu_map: AyaHashMap<_, u32, MbcCpuState> =
            AyaHashMap::try_from(ebpf.map_mut("CPU_MAP").context("CPU_MAP not found")?)?;
        let cpu = cpu_map
            .get(&CPU_INSTANCE_ID, 0)
            .context("CPU_MAP read failed for instance 0xDE")?;
        if cpu.pc != 0 {
            bail!("CPU_MAP verification FAILED: pc={}, expected 0", cpu.pc);
        }
        if cpu.regs[15] != DEFAULT_SP {
            bail!(
                "CPU_MAP verification FAILED: sp={:#x}, expected {DEFAULT_SP:#x}",
                cpu.regs[15],
            );
        }
        if cpu.halted != 0 {
            bail!(
                "CPU_MAP verification FAILED: halted={}, expected 0",
                cpu.halted
            );
        }
        info!(
            "CPU_MAP: PASS (instance={CPU_INSTANCE_ID:#x}, pc={}, sp={:#x}, halted={})",
            cpu.pc, cpu.regs[15], cpu.halted,
        );
    }

    // Verify WAD magic at WAD_BASE
    if wad_data.len() >= 4 {
        let mut ram_map: Array<_, u32> =
            Array::try_from(ebpf.map_mut("RAM_MAP").context("RAM_MAP not found")?)?;
        let wad_base_word = memory::WAD_BASE / 4;
        let wad_first_word = ram_map
            .get(&wad_base_word, 0)
            .with_context(|| format!("RAM_MAP read failed at WAD_BASE word {wad_base_word:#x}"))?;
        let expected = u32::from_le_bytes([wad_data[0], wad_data[1], wad_data[2], wad_data[3]]);
        if wad_first_word != expected {
            bail!(
                "RAM_MAP verification FAILED at WAD_BASE: expected {:#010x} ('{}'), got {:#010x}",
                expected,
                std::str::from_utf8(&wad_data[..4]).unwrap_or("????"),
                wad_first_word,
            );
        }
        let magic_str = std::str::from_utf8(&wad_data[..4]).unwrap_or("????");
        info!("RAM_MAP: PASS (WAD magic at {wad_base_word:#x} = '{magic_str}')");

        // Write actual WAD file size to WAD_SIZE_ADDR so libc_stubs.c open() uses
        // the correct size instead of a hardcoded constant. Fixes texture corruption
        // when WAD size doesn't match the hardcoded value.
        let wad_size_word = memory::WAD_SIZE_ADDR / 4;
        ram_map.set(wad_size_word, wad_data.len() as u32, 0)?;
        info!(
            "RAM_MAP: WAD size {} bytes written to {wad_size_word:#x}",
            wad_data.len()
        );

        // Publish the canonical IWAD probe name so libc_stubs.c access() can report
        // the right filename to IdentifyVersion(). Without this the gamemode is
        // pinned at build time and only one IWAD flavour ever works.
        let flavour = loader::detect_iwad_flavour(wad_data)?;
        let probe = flavour.probe_name();
        let mut name_buf = [0u8; memory::WAD_IWAD_NAME_SIZE as usize];
        // probe_name() values are all well under the buffer; NUL terminator is
        // guaranteed because the buffer starts zeroed and is strictly longer.
        name_buf[..probe.len()].copy_from_slice(probe.as_bytes());
        let name_base_word = memory::WAD_IWAD_NAME_ADDR / 4;
        for (i, word) in loader::bytes_to_words(&name_buf).into_iter().enumerate() {
            ram_map.set(name_base_word + i as u32, word, 0)?;
        }
        info!("RAM_MAP: IWAD probe name '{probe}' written to {name_base_word:#x} ({flavour:?})");
    }

    // Verify STATS map is empty/zeroed
    {
        let stats: AyaHashMap<_, u32, u64> =
            AyaHashMap::try_from(ebpf.map_mut("STATS").context("STATS not found")?)?;
        // STATS is a HashMap — empty means no entries yet (no packets processed)
        let count = stats.iter().count();
        if count == 0 {
            info!("STATS: PASS (empty — no packets processed yet)");
        } else {
            info!("STATS: {count} entries present (prior state?)");
        }
    }

    Ok(())
}

// ── Subcommands ─────────────────────────────────────────────────────────────

/// Ring management subcommand.
fn cmd_ring(action: RingAction) -> Result<()> {
    match action {
        RingAction::Setup { hops } => {
            let config = ring::RingConfig {
                num_hops: hops,
                ..Default::default()
            };
            ring::setup(&config)
        }
        RingAction::Teardown { hops } => {
            let config = ring::RingConfig {
                num_hops: hops,
                ..Default::default()
            };
            ring::teardown(&config)
        }
        RingAction::Status { hops } => {
            info!("ring status (TODO: implement status display for {hops} hops)");
            Ok(())
        }
    }
}

/// Bridge server subcommand.
///
/// Standalone bridge requires a running eBPF program with pinned maps.
/// For integrated use, prefer `doom-runner run` which starts the bridge
/// with direct map access via the Aya Ebpf object.
async fn cmd_bridge(_addr: String) -> Result<()> {
    anyhow::bail!(
        "standalone bridge is no longer supported — use `doom-runner run` instead.\n\
         The integrated bridge reads maps directly from the Aya Ebpf object."
    );
}

/// Print and validate memory layout.
fn cmd_layout() -> Result<()> {
    println!("Doom-over-IPv6 Memory Layout");
    println!("============================");
    println!();
    println!("Region          Start        End          Size");
    println!("------          -----        ---          ----");
    println!(
        "ROM             {:#010x}   {:#010x}   {} KiB",
        memory::ROM_BASE,
        memory::ROM_BASE + memory::ROM_SIZE,
        memory::ROM_SIZE / 1024,
    );
    println!(
        "RAM base        {:#010x}   {:#010x}   {} KiB",
        memory::RAM_BASE,
        memory::RAM_BASE + memory::RAM_SIZE,
        memory::RAM_SIZE / 1024,
    );
    println!(
        "  KBD           {:#010x}   {:#010x}   4 bytes",
        memory::KBD_ADDR,
        memory::KBD_ADDR + 4,
    );
    println!(
        "  Screen        {:#010x}   {:#010x}   {} bytes ({}x{})",
        memory::SCREEN_BASE,
        memory::SCREEN_BASE + memory::SCREEN_SIZE,
        memory::SCREEN_SIZE,
        memory::SCREEN_WIDTH,
        memory::SCREEN_HEIGHT,
    );
    println!(
        "  Heap          {:#010x}   {:#010x}   {} MiB",
        memory::HEAP_START,
        memory::HEAP_END,
        memory::HEAP_SIZE / (1024 * 1024),
    );
    println!(
        "  WAD           {:#010x}   {:#010x}   {} MiB (max)",
        memory::WAD_BASE,
        memory::WAD_BASE + memory::WAD_MAX_SIZE,
        memory::WAD_MAX_SIZE / (1024 * 1024),
    );
    println!("  Debug         {:#010x}", memory::DEBUG_BASE,);
    println!("  Stack top     {:#010x}   (grows down)", memory::STACK_TOP,);
    println!();
    println!("BPF Map Sizing:");
    println!(
        "  ROM_MAP:    {:>10} entries ({} MiB)",
        memory::ROM_MAP_ENTRIES,
        memory::ROM_MAP_ENTRIES * 4 / (1024 * 1024)
    );
    println!(
        "  RAM_MAP:    {:>10} entries ({} MiB)",
        memory::RAM_MAP_ENTRIES,
        memory::RAM_MAP_ENTRIES * 4 / (1024 * 1024)
    );
    println!("  SCREEN_MAP: {:>10} entries", memory::SCREEN_MAP_ENTRIES);
    println!("  CPU_MAP:    {:>10} entries", memory::CPU_MAP_ENTRIES);
    println!("  KBD_MAP:    {:>10} entries", memory::KBD_MAP_ENTRIES);
    println!("  RV2MBC_MAP: {:>10} entries", memory::RV2MBC_MAP_ENTRIES);
    println!();

    match memory::validate_layout() {
        Ok(()) => {
            println!("Layout validation: PASS");
            Ok(())
        }
        Err(e) => {
            error!("Layout validation: FAIL -- {e}");
            Err(e.into())
        }
    }
}

#[cfg(test)]
mod upc_gate_tests {
    use super::*;

    mod object_surface {
        include!("../../../ebpf/monad-cpu-ebpf/surface.rs");
    }

    fn facts(numbers: &[u32]) -> upc_api::ObjectFacts {
        upc_api::ObjectFacts {
            surface_id: upc_api::surface_hash::surface_id(numbers),
            mmu_geometry: None,
        }
    }

    #[test]
    fn doom_surface_is_the_default_object_surface() {
        let mut declared: Vec<u32> = DOOM_SYSCALLS.iter().map(|d| d.number).collect();
        declared.sort_unstable();
        assert_eq!(declared, object_surface::SURFACE_DEFAULT);
        assert_eq!(
            upc_api::check_surface(&DOOM_SURFACE, &facts(object_surface::SURFACE_DEFAULT)),
            Ok(())
        );
    }

    #[test]
    fn doom_refused_by_the_ascend_object() {
        assert!(matches!(
            upc_api::check_surface(&DOOM_SURFACE, &facts(object_surface::SURFACE_ASCEND)),
            Err(upc_api::ValidationError::SurfaceObjectMismatch { .. })
        ));
    }

    #[test]
    fn doom_workload_is_flat_and_passes_gate_b() {
        let w = doom_workload(vec![0x2700_0000], vec![0], None);
        assert_eq!(w.mmu, None);
        assert_eq!(
            upc_api::validate_against_object(
                &w,
                &doom_map_capacities(),
                &facts(object_surface::SURFACE_DEFAULT)
            ),
            Ok(())
        );
    }
}

#[cfg(test)]
mod rv2mbc_gate_tests {
    //! S-ASCEND-2 Item 5: the Doom `.rv2mbc` is checked against its SHA-256
    //! (explicit `--rv2mbc-sha`, else `doom.rv2mbc.sha256`) before any map write.
    use super::*;
    use std::path::{Path, PathBuf};

    const RV: &[u8] = &[0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00];
    const WRONG: &str = "0000000000000000000000000000000000000000000000000000000000000000";

    fn good_hex() -> String {
        upc_api::object::hex32(&upc_api::surface_hash::sha256(RV))
    }

    fn fixture(tag: &str) -> PathBuf {
        let dir = std::env::temp_dir().join(format!("doom-rv2mbc-{tag}-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&dir);
        std::fs::create_dir_all(&dir).unwrap();
        let rv = dir.join("doom.rv2mbc");
        std::fs::write(&rv, RV).unwrap();
        rv
    }

    fn sidecar(rv: &Path, text: &str) {
        std::fs::write(upc_api::rv2mbc_sha::sidecar_path(rv), text).unwrap();
    }

    fn cleanup(rv: &Path) {
        std::fs::remove_dir_all(rv.parent().unwrap()).ok();
    }

    #[test]
    fn no_sidecar_is_refused() {
        let rv = fixture("required");
        let err = checked_doom_rv2mbc(Some(&rv), None, false).unwrap_err();
        assert!(err.to_string().contains("Rv2mbcShaRequired"), "{err:#}");
        cleanup(&rv);
    }

    #[test]
    fn correct_sidecar_loads_the_entries() {
        let rv = fixture("ok");
        sidecar(&rv, &format!("{}  doom.rv2mbc\n", good_hex()));
        let got = checked_doom_rv2mbc(Some(&rv), None, false)
            .unwrap()
            .unwrap();
        assert_eq!(got.entries, vec![0, 1]);
        assert_eq!(got.sha256.as_deref(), Some(good_hex().as_str()));
        cleanup(&rv);
    }

    #[test]
    fn explicit_sha_wins() {
        let rv = fixture("explicit");
        sidecar(&rv, WRONG);
        assert!(checked_doom_rv2mbc(Some(&rv), Some(&good_hex()), false).is_ok());
        cleanup(&rv);
    }

    #[test]
    fn mismatch_is_refused_even_with_the_hatch() {
        let rv = fixture("mismatch");
        sidecar(&rv, WRONG);
        let err = checked_doom_rv2mbc(Some(&rv), None, true).unwrap_err();
        assert!(err.to_string().contains("Rv2mbcShaMismatch"), "{err:#}");
        cleanup(&rv);
    }

    #[test]
    fn hatch_loads_unverified_without_a_hash() {
        let rv = fixture("hatch");
        let got = checked_doom_rv2mbc(Some(&rv), None, true).unwrap().unwrap();
        assert_eq!(got.entries, vec![0, 1]);
        assert_eq!(got.sha256, None);
        cleanup(&rv);
    }

    #[test]
    fn no_rv2mbc_flag_loads_nothing() {
        assert!(checked_doom_rv2mbc(None, None, false).unwrap().is_none());
    }

    #[test]
    fn hatch_is_dry_run_only() {
        assert!(hatch_gate(false, false).is_ok());
        assert!(hatch_gate(true, true).is_ok());
        let err = hatch_gate(true, false).unwrap_err().to_string();
        assert!(
            err.contains("--allow-unverified-rv2mbc is accepted only with --dry-run"),
            "{err}"
        );
    }

    #[test]
    fn verified_hash_reaches_the_workload() {
        let w = doom_workload(vec![0x2700_0000], vec![0], Some(good_hex()));
        assert_eq!(w.image.expected_rv2mbc_sha256, Some(good_hex()));
    }

    /// The committed `doom/doom.rv2mbc.sha256` must match `doom/doom.rv2mbc`.
    #[test]
    fn committed_doom_sidecar_matches() {
        let rv = Path::new(env!("CARGO_MANIFEST_DIR")).join("../../doom/doom.rv2mbc");
        let got = checked_doom_rv2mbc(Some(&rv), None, false)
            .expect("doom/doom.rv2mbc verifies against its committed sidecar")
            .unwrap();
        assert!(!got.entries.is_empty());
    }
}
