// SPDX-License-Identifier: GPL-3.0-only
//! doom-runner — Aya-based Doom-over-IPv6 runtime.
//!
//! Replaces the fragile shell script + Go loader chain with a single Rust binary
//! that handles:
//!
//! 1. **Memory layout** — single source of truth for the Doom address space
//! 2. **ELF loading** — parse doom.elf, load sections into BPF maps
//! 3. **Tail call chain** — N programs x 16 insns = 16N insns/tick
//! 4. **Network ring** — namespace + veth setup (replaces doom-ring.sh)
//! 5. **Screen bridge** — WebSocket server for browser rendering
//!
//! # Usage
//!
//! ```bash
//! # Full pipeline: setup ring, load Doom, start bridge
//! sudo doom-runner run --elf doom/doom.elf --mbc doom/doom.mbc --wad doom1.wad
//!
//! # Just set up the network ring
//! sudo doom-runner ring setup
//!
//! # Just load the Doom binary into existing BPF maps
//! sudo doom-runner load --elf doom/doom.elf --mbc doom/doom.mbc --wad doom1.wad
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

use anyhow::{Context, Result};
use clap::{Parser, Subcommand};
use tracing::{error, info};
use tracing_subscriber::EnvFilter;

#[derive(Parser, Debug)]
#[command(
    name = "doom-runner",
    about = "Doom-over-IPv6 runtime — Aya-based BPF program loader and memory manager",
    version,
)]
struct Cli {
    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand, Debug)]
enum Commands {
    /// Full pipeline: setup ring, load Doom, attach BPF, start bridge.
    Run {
        /// Path to doom.elf (linked RISC-V binary).
        #[arg(long)]
        elf: PathBuf,

        /// Path to doom.mbc (pre-translated MBC binary).
        #[arg(long)]
        mbc: PathBuf,

        /// Path to doom.rv2mbc (RV32I-to-MBC address map).
        #[arg(long)]
        rv2mbc: Option<PathBuf>,

        /// Path to doom1.wad.
        #[arg(long)]
        wad: PathBuf,

        /// Path to monad-cpu-ebpf ELF object.
        #[arg(long, default_value = "ebpf/target/bpfel-unknown-none/release/monad-cpu-ebpf")]
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

    /// Load Doom binary into existing BPF maps (ring must be set up).
    Load {
        /// Path to doom.elf.
        #[arg(long)]
        elf: PathBuf,

        /// Path to doom.mbc.
        #[arg(long)]
        mbc: PathBuf,

        /// Path to doom.rv2mbc.
        #[arg(long)]
        rv2mbc: Option<PathBuf>,

        /// Path to doom1.wad.
        #[arg(long)]
        wad: PathBuf,

        /// BPF map pin directory.
        #[arg(long, default_value = "/sys/fs/bpf/unheaded/doom-ring/maps")]
        map_pin_dir: PathBuf,
    },

    /// Network ring management.
    Ring {
        #[command(subcommand)]
        action: RingAction,
    },

    /// Screen bridge server.
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
    // Initialize tracing
    tracing_subscriber::fmt()
        .with_env_filter(
            EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| EnvFilter::new("doom_runner=info")),
        )
        .init();

    let cli = Cli::parse();

    match cli.command {
        Commands::Run {
            elf,
            mbc,
            rv2mbc,
            wad,
            ebpf_obj,
            hops,
            chain_depth,
            bridge_addr,
            skip_ring,
            headless,
        } => {
            cmd_run(elf, mbc, rv2mbc, wad, ebpf_obj, hops, chain_depth, bridge_addr, skip_ring, headless).await
        }
        Commands::Load {
            elf,
            mbc,
            rv2mbc,
            wad,
            map_pin_dir,
        } => cmd_load(elf, mbc, rv2mbc, wad, map_pin_dir).await,
        Commands::Ring { action } => cmd_ring(action),
        Commands::Bridge { addr } => cmd_bridge(addr).await,
        Commands::Layout => cmd_layout(),
    }
}

/// Full pipeline: ring setup -> load -> attach BPF -> bridge.
#[allow(clippy::too_many_arguments)]
async fn cmd_run(
    elf_path: PathBuf,
    mbc_path: PathBuf,
    rv2mbc_path: Option<PathBuf>,
    wad_path: PathBuf,
    ebpf_obj: PathBuf,
    hops: u32,
    chain_depth: usize,
    bridge_addr: String,
    skip_ring: bool,
    headless: bool,
) -> Result<()> {
    info!("doom-runner: full pipeline");

    // Step 0: Validate memory layout
    memory::validate_layout().context("memory layout validation failed")?;
    info!("memory layout validated");

    // Step 1: Set up network ring
    if !skip_ring {
        let ring_config = ring::RingConfig {
            num_hops: hops,
            ebpf_obj_path: ebpf_obj.display().to_string(),
            ..Default::default()
        };
        ring::setup(&ring_config).context("ring setup failed")?;
    } else {
        info!("skipping ring setup (--skip-ring)");
    }

    // Step 2: Load Doom binary
    let map_pin_dir = PathBuf::from("/sys/fs/bpf/unheaded/doom-ring/maps");
    load_doom(&elf_path, &mbc_path, rv2mbc_path.as_deref(), &wad_path, &map_pin_dir)
        .await
        .context("doom loading failed")?;

    // Step 3: Set up tail call chain
    let tc_config = tail_calls::TailCallConfig {
        chain_depth,
        ebpf_obj_path: ebpf_obj.display().to_string(),
        ..tail_calls::TailCallConfig::new(ebpf_obj.display().to_string())
    };
    let plan = tail_calls::plan(tc_config)?;
    tail_calls::load_chain(&plan).await?;

    // Step 4: Start bridge (blocks)
    if !headless {
        let bridge_config = bridge::BridgeConfig {
            listen_addr: bridge_addr,
            ..Default::default()
        };
        bridge::serve(&bridge_config).await?;
    } else {
        info!("headless mode — no bridge server");
        info!("doom-runner ready. Send tick packets to start execution.");
        // In headless mode, just wait forever (ctrl-c to exit)
        tokio::signal::ctrl_c().await?;
    }

    Ok(())
}

/// Load Doom binary into BPF maps.
async fn cmd_load(
    elf_path: PathBuf,
    mbc_path: PathBuf,
    rv2mbc_path: Option<PathBuf>,
    wad_path: PathBuf,
    map_pin_dir: PathBuf,
) -> Result<()> {
    memory::validate_layout().context("memory layout validation failed")?;
    load_doom(&elf_path, &mbc_path, rv2mbc_path.as_deref(), &wad_path, &map_pin_dir).await
}

/// Core loading logic shared by `run` and `load` commands.
async fn load_doom(
    elf_path: &PathBuf,
    mbc_path: &PathBuf,
    rv2mbc_path: Option<&std::path::Path>,
    wad_path: &PathBuf,
    _map_pin_dir: &PathBuf,
) -> Result<()> {
    info!("loading doom...");

    // Parse ELF for data sections
    let sections = loader::parse_elf(elf_path)
        .context("failed to parse doom.elf")?;
    info!(
        "ELF: {} data sections, bss={:#x}+{:#x}, entry=rv:{:#010x}",
        sections.data_sections.len(),
        sections.bss_start,
        sections.bss_len,
        sections.entry_rv_addr,
    );

    // Load MBC ROM
    let rom = loader::load_mbc_rom(mbc_path)
        .context("failed to load doom.mbc")?;
    info!("ROM: {} MBC instructions", rom.len());

    // Load rv2mbc table (optional)
    if let Some(path) = rv2mbc_path {
        let rv2mbc = loader::load_rv2mbc(path)
            .context("failed to load rv2mbc")?;
        info!("RV2MBC: {} entries", rv2mbc.len());
    }

    // Load WAD
    let wad_data = loader::load_wad(wad_path)
        .context("failed to load WAD")?;

    // Stage RAM image
    let ram_updates = loader::stage_ram_image(
        &sections.data_sections,
        Some(&wad_data),
    )?;
    info!("RAM: {} non-zero words to write", ram_updates.len());

    // Compute initial CPU state
    // TODO: translate entry_rv_addr through rv2mbc to get MBC PC
    let cpu_state = loader::InitialCpuState::new(0);
    info!(
        "CPU: pc={}, sp={:#010x}, heap_start={:#010x}, heap_size={:#x}",
        cpu_state.pc,
        cpu_state.regs[15],
        cpu_state.regs[10],
        cpu_state.regs[11],
    );

    // TODO: Actually write to BPF maps via Aya.
    //
    // This requires the ring to be set up and maps to be pinned.
    // The write logic will be:
    //
    // use aya::maps::Array;
    //
    // let mut rom_map = Array::<_, u32>::from_pin(map_pin_dir.join("ROM_MAP"))?;
    // for (i, &insn) in rom.iter().enumerate() {
    //     rom_map.set(i as u32, insn, 0)?;
    // }
    //
    // let mut ram_map = Array::<_, u32>::from_pin(map_pin_dir.join("RAM_MAP"))?;
    // for (idx, val) in &ram_updates {
    //     ram_map.set(*idx, *val, 0)?;
    // }
    //
    // let mut cpu_map = HashMap::<_, u32, MbcCpuState>::from_pin(map_pin_dir.join("CPU_MAP"))?;
    // cpu_map.insert(0, cpu_state_to_bytes(&cpu_state), 0)?;

    info!("doom loaded (map writes not yet implemented — scaffold only)");
    Ok(())
}

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
async fn cmd_bridge(addr: String) -> Result<()> {
    let config = bridge::BridgeConfig {
        listen_addr: addr,
        ..Default::default()
    };
    bridge::serve(&config).await
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
    println!(
        "  Debug         {:#010x}",
        memory::DEBUG_BASE,
    );
    println!(
        "  Stack top     {:#010x}   (grows down)",
        memory::STACK_TOP,
    );
    println!();
    println!("BPF Map Sizing:");
    println!("  ROM_MAP:    {:>10} entries ({} MiB)", memory::ROM_MAP_ENTRIES, memory::ROM_MAP_ENTRIES * 4 / (1024 * 1024));
    println!("  RAM_MAP:    {:>10} entries ({} MiB)", memory::RAM_MAP_ENTRIES, memory::RAM_MAP_ENTRIES * 4 / (1024 * 1024));
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
            error!("Layout validation: FAIL — {e}");
            Err(e.into())
        }
    }
}
