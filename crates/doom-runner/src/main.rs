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
// Must match monad-common's MbcCpuState exactly: 128 bytes, #[repr(C)].
// We define it here to avoid pulling monad-common (which targets both
// bpfel-unknown-none and host). Verified by size_of assertion below.

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
    pub page_dir_base: u32,     // 4
} // total: 128

// Safety: MbcCpuState is #[repr(C)], Copy, contains only primitives.
unsafe impl aya::Pod for MbcCpuState {}

const _: () = assert!(std::mem::size_of::<MbcCpuState>() == 128);

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
) -> Result<()> {
    info!("doom-runner: Aya-based full pipeline (no pins, no mismatches)");
    let t0 = Instant::now();

    // Step 0: Validate memory layout
    memory::validate_layout().context("memory layout validation failed")?;
    info!("memory layout validated");

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

    let rv2mbc_data = if let Some(ref path) = rv2mbc_path {
        let data = loader::load_rv2mbc(path).context("failed to load rv2mbc")?;
        info!("RV2MBC: {} entries", data.len());
        Some(data)
    } else {
        warn!("no rv2mbc file provided — indirect jumps will not work");
        None
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

    // 5a: ROM_MAP — MBC instructions
    {
        let t = Instant::now();
        let mut rom_map: Array<_, u32> = Array::try_from(
            ebpf.map_mut("ROM_MAP")
                .context("ROM_MAP not found in eBPF program")?,
        )?;
        info!("ROM_MAP: writing {} instructions...", rom.len());
        for (i, insn) in rom.iter().enumerate() {
            rom_map
                .set(i as u32, insn, 0)
                .with_context(|| format!("ROM_MAP write failed at index {i}"))?;
        }
        info!(
            "ROM_MAP: {} entries written in {:.1}s",
            rom.len(),
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

    // 5c: RV2MBC_MAP — address translation table
    if let Some(ref rv2mbc) = rv2mbc_data {
        let t = Instant::now();
        let mut rv2mbc_map: Array<_, u32> = Array::try_from(
            ebpf.map_mut("RV2MBC_MAP")
                .context("RV2MBC_MAP not found in eBPF program")?,
        )?;
        info!("RV2MBC_MAP: writing {} entries...", rv2mbc.len());
        for (i, val) in rv2mbc.iter().enumerate() {
            rv2mbc_map
                .set(i as u32, val, 0)
                .with_context(|| format!("RV2MBC_MAP write failed at index {i}"))?;
        }
        info!(
            "RV2MBC_MAP: {} entries written in {:.1}s",
            rv2mbc.len(),
            t.elapsed().as_secs_f64()
        );
    }

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
    verify_maps(&mut ebpf, &rom, &ram_updates, &wad_data)?;

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
