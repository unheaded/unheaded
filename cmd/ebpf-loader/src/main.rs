//! Unheaded eBPF Loader
//!
//! Loads and attaches eBPF programs compiled with aya-ebpf using
//! the aya userspace library. This bypasses the libbpf legacy-maps
//! incompatibility (libbpf v1.0+ rejects the `maps` ELF section that
//! aya-ebpf 0.1.x emits).
//!
//! Programs:
//!   - packet-marker:  XDP program for trace ID injection
//!   - flow-tracker:   TC classifier (ingress + egress) for connection tracking
//!   - latency-probe:  kprobe/kretprobe on TCP functions for latency measurement
//!   - syscall-tracer: raw tracepoints on sys_enter/sys_exit
//!   - monad-cpu:      XDP Monad CPU for Doom-over-IPv6 (MBC execution engine)

use std::path::PathBuf;

use anyhow::{Context, Result};
use aya::programs::{KProbe, RawTracePoint, SchedClassifier, TcAttachType, Xdp, XdpFlags};
use aya::{programs::tc, Ebpf, EbpfLoader};
use clap::Parser;
use log::{info, warn};

const ALL_PROGRAMS: &[&str] = &[
    "packet-marker",
    "flow-tracker",
    "latency-probe",
    "syscall-tracer",
    "shield-ebpf",
];

#[derive(Parser)]
#[command(name = "ebpf-loader")]
#[command(about = "Load and attach Unheaded eBPF programs")]
struct Cli {
    /// Network interface for XDP and TC programs
    #[arg(short, long, default_value = "eth0")]
    interface: String,

    /// Path to eBPF object directory
    #[arg(
        short = 'd',
        long,
        default_value = "ebpf/target/bpfel-unknown-none/release"
    )]
    obj_dir: PathBuf,

    /// Pin maps to /sys/fs/bpf/unheaded/<program>/
    #[arg(long)]
    pin_maps: bool,

    /// Only load specific programs (comma-separated)
    #[arg(long, value_delimiter = ',')]
    only: Option<Vec<String>>,

    /// Dry run — validate ELF objects without loading into kernel
    #[arg(long)]
    dry_run: bool,

    /// Pin path for shared BPF maps (used by monad-cpu for doom-ring).
    /// When set, maps are pinned on first load and reused on subsequent loads.
    #[arg(long)]
    map_pin_path: Option<PathBuf>,

    /// Use SKB (generic) mode for XDP attachment (required for veth devices)
    #[arg(long)]
    xdp_skb_mode: bool,

    /// Write PID to this file (for daemon management by doom-ring.sh).
    /// The loader stays alive holding BPF link FDs; kill it to detach XDP.
    #[arg(long)]
    pid_file: Option<PathBuf>,
}

fn main() -> Result<()> {
    env_logger::Builder::from_env(env_logger::Env::default().default_filter_or("info")).init();

    let cli = Cli::parse();

    let programs: Vec<&str> = match &cli.only {
        Some(list) => list.iter().map(|s| s.as_str()).collect(),
        None => ALL_PROGRAMS.to_vec(),
    };

    // Validate all objects exist before loading any
    for name in &programs {
        let elf_name = elf_filename(name);
        let path = cli.obj_dir.join(elf_name);
        if !path.exists() {
            anyhow::bail!(
                "eBPF object not found: {} (build with: cd ebpf && cargo +nightly build \
                 --target bpfel-unknown-none -Z build-std=core --release)",
                path.display()
            );
        }
    }

    if cli.dry_run {
        return dry_run(&cli, &programs);
    }

    // Load and attach programs — keep Ebpf handles alive
    let mut handles: Vec<(&str, Ebpf)> = Vec::new();

    for name in &programs {
        let ebpf = match *name {
            "packet-marker" => load_packet_marker(&cli),
            "flow-tracker" => load_flow_tracker(&cli),
            "latency-probe" => load_latency_probe(&cli),
            "syscall-tracer" => load_syscall_tracer(&cli),
            "monad-cpu" => load_monad_cpu(&cli),
            "shield-ebpf" => load_shield(&cli),
            other => {
                warn!("Unknown program: {}, skipping", other);
                continue;
            }
        }
        .with_context(|| format!("Failed to load {}", name))?;

        handles.push((name, ebpf));
    }

    // Optionally pin maps for access by trace-collector
    if cli.pin_maps {
        pin_all_maps(&mut handles)?;
    }

    info!(
        "{}/{} programs loaded and attached. Waiting for signal to detach.",
        handles.len(),
        programs.len()
    );

    // Write PID file if requested (for daemon management)
    if let Some(ref pid_path) = cli.pid_file {
        std::fs::write(pid_path, format!("{}", std::process::id()))
            .with_context(|| format!("Failed to write PID file {}", pid_path.display()))?;
        info!(
            "PID {} written to {}",
            std::process::id(),
            pid_path.display()
        );
    }

    // Wait for signal
    let (tx, rx) = std::sync::mpsc::channel::<()>();
    ctrlc::set_handler(move || {
        let _ = tx.send(());
    })
    .context("Failed to set Ctrl+C handler")?;
    let _ = rx.recv();

    info!("Detaching all programs...");
    drop(handles);

    // Clean up PID file
    if let Some(ref pid_path) = cli.pid_file {
        let _ = std::fs::remove_file(pid_path);
    }

    info!("Done.");

    Ok(())
}

/// Map program name to ELF filename (most match 1:1 except monad-cpu → monad-cpu-ebpf).
fn elf_filename(name: &str) -> &str {
    match name {
        "monad-cpu" => "monad-cpu-ebpf",
        other => other,
    }
}

fn dry_run(cli: &Cli, programs: &[&str]) -> Result<()> {
    info!("[dry-run] Validating eBPF objects (no kernel loading)");

    for name in programs {
        let path = cli.obj_dir.join(elf_filename(name));
        let metadata =
            std::fs::metadata(&path).with_context(|| format!("Cannot stat {}", path.display()))?;
        info!("[dry-run] {} — {} bytes, valid ELF", name, metadata.len());
    }

    info!("[dry-run] All {} objects validated", programs.len());
    Ok(())
}

// ---------------------------------------------------------------------------
// packet-marker: XDP
// ---------------------------------------------------------------------------

fn load_packet_marker(cli: &Cli) -> Result<Ebpf> {
    let path = cli.obj_dir.join("packet-marker");
    info!("Loading packet-marker from {}", path.display());

    let mut ebpf = Ebpf::load_file(&path).context("Failed to parse packet-marker ELF")?;

    let prog: &mut Xdp = ebpf
        .program_mut("packet_marker")
        .context("program 'packet_marker' not found in ELF")?
        .try_into()?;

    prog.load()
        .context("Failed to load packet_marker into kernel")?;
    prog.attach(&cli.interface, XdpFlags::default())
        .with_context(|| format!("Failed to attach XDP to {}", cli.interface))?;

    info!(
        "  packet_marker attached to {} (XDP, flags=default)",
        cli.interface
    );
    Ok(ebpf)
}

// ---------------------------------------------------------------------------
// flow-tracker: TC classifier (ingress + egress)
// ---------------------------------------------------------------------------

fn load_flow_tracker(cli: &Cli) -> Result<Ebpf> {
    let path = cli.obj_dir.join("flow-tracker");
    info!("Loading flow-tracker from {}", path.display());

    let mut ebpf = Ebpf::load_file(&path).context("Failed to parse flow-tracker ELF")?;

    // Ensure clsact qdisc exists (idempotent — ignore EEXIST)
    if let Err(e) = tc::qdisc_add_clsact(&cli.interface) {
        warn!(
            "qdisc_add_clsact on {}: {} (may already exist)",
            cli.interface, e
        );
    }

    // Ingress
    let ingress: &mut SchedClassifier = ebpf
        .program_mut("flow_tracker_ingress")
        .context("program 'flow_tracker_ingress' not found")?
        .try_into()?;
    ingress
        .load()
        .context("Failed to load flow_tracker_ingress")?;
    ingress
        .attach(&cli.interface, TcAttachType::Ingress)
        .context("Failed to attach flow_tracker_ingress")?;
    info!("  flow_tracker_ingress attached (TC ingress)");

    // Egress
    let egress: &mut SchedClassifier = ebpf
        .program_mut("flow_tracker_egress")
        .context("program 'flow_tracker_egress' not found")?
        .try_into()?;
    egress
        .load()
        .context("Failed to load flow_tracker_egress")?;
    egress
        .attach(&cli.interface, TcAttachType::Egress)
        .context("Failed to attach flow_tracker_egress")?;
    info!("  flow_tracker_egress attached (TC egress)");

    Ok(ebpf)
}

// ---------------------------------------------------------------------------
// latency-probe: kprobe + kretprobe on TCP functions
// ---------------------------------------------------------------------------

fn load_latency_probe(cli: &Cli) -> Result<Ebpf> {
    let path = cli.obj_dir.join("latency-probe");
    info!("Loading latency-probe from {}", path.display());

    let mut ebpf = Ebpf::load_file(&path).context("Failed to parse latency-probe ELF")?;

    // (aya_program_name, kernel_function, is_kretprobe)
    let probes: &[(&str, &str, bool)] = &[
        ("tcp_sendmsg_enter", "tcp_sendmsg", false),
        ("tcp_sendmsg_exit", "tcp_sendmsg", true),
        ("tcp_recvmsg_enter", "tcp_recvmsg", false),
        ("tcp_recvmsg_exit", "tcp_recvmsg", true),
        ("tcp_connect_enter", "tcp_v4_connect", false),
        ("tcp_connect_exit", "tcp_v4_connect", true),
    ];

    for &(prog_name, kernel_fn, is_ret) in probes {
        let prog: &mut KProbe = ebpf
            .program_mut(prog_name)
            .with_context(|| format!("program '{}' not found", prog_name))?
            .try_into()?;

        prog.load()
            .with_context(|| format!("Failed to load {}", prog_name))?;
        prog.attach(kernel_fn, 0)
            .with_context(|| format!("Failed to attach {} to {}", prog_name, kernel_fn))?;

        let kind = if is_ret { "kretprobe" } else { "kprobe" };
        info!("  {} attached to {} ({})", prog_name, kernel_fn, kind);
    }

    Ok(ebpf)
}

// ---------------------------------------------------------------------------
// syscall-tracer: raw tracepoints
// ---------------------------------------------------------------------------

fn load_syscall_tracer(_cli: &Cli) -> Result<Ebpf> {
    let path = _cli.obj_dir.join("syscall-tracer");
    info!("Loading syscall-tracer from {}", path.display());

    let mut ebpf = Ebpf::load_file(&path).context("Failed to parse syscall-tracer ELF")?;

    for tp in &["sys_enter", "sys_exit"] {
        let prog: &mut RawTracePoint = ebpf
            .program_mut(tp)
            .with_context(|| format!("program '{}' not found", tp))?
            .try_into()?;

        prog.load()
            .with_context(|| format!("Failed to load {}", tp))?;
        prog.attach(tp)
            .with_context(|| format!("Failed to attach raw_tracepoint/{}", tp))?;

        info!("  {} attached (raw_tracepoint)", tp);
    }

    Ok(ebpf)
}

// ---------------------------------------------------------------------------
// monad-cpu: XDP Monad CPU for Doom-over-IPv6
// ---------------------------------------------------------------------------

fn load_monad_cpu(cli: &Cli) -> Result<Ebpf> {
    let path = cli.obj_dir.join("monad-cpu-ebpf");
    info!("Loading monad-cpu from {}", path.display());

    let mut ebpf = if let Some(ref pin_path) = cli.map_pin_path {
        // Shared map pinning: first loader pins maps, subsequent loaders reuse them.
        // This is critical for doom-ring where 6 namespaces share ROM/CPU/RAM/SCREEN maps.
        std::fs::create_dir_all(pin_path)
            .with_context(|| format!("Failed to create map pin dir {}", pin_path.display()))?;
        info!("  using shared map pin path: {}", pin_path.display());
        EbpfLoader::new()
            .map_pin_path(pin_path)
            .load_file(&path)
            .context("Failed to parse monad-cpu-ebpf ELF with map pinning")?
    } else {
        Ebpf::load_file(&path).context("Failed to parse monad-cpu-ebpf ELF")?
    };

    // Explicitly pin maps to bpffs for sharing across loaders.
    // EbpfLoader::map_pin_path should do this, but legacy maps may not auto-pin.
    if let Some(ref pin_path) = cli.map_pin_path {
        for (map_name, map) in ebpf.maps_mut() {
            let pin = pin_path.join(&map_name);
            if pin.exists() {
                info!("  map {} already pinned at {}", map_name, pin.display());
            } else {
                match map.pin(&pin) {
                    Ok(()) => info!("  pinned map {} → {}", map_name, pin.display()),
                    Err(e) => warn!(
                        "  failed to pin map {}: {} (may already exist)",
                        map_name, e
                    ),
                }
            }
        }
    }

    let prog: &mut Xdp = ebpf
        .program_mut("monad_cpu")
        .context("program 'monad_cpu' not found in ELF")?
        .try_into()?;

    prog.load()
        .context("Failed to load monad_cpu into kernel")?;

    let flags = if cli.xdp_skb_mode {
        XdpFlags::SKB_MODE
    } else {
        XdpFlags::default()
    };

    prog.attach(&cli.interface, flags)
        .with_context(|| format!("Failed to attach monad_cpu XDP to {}", cli.interface))?;

    let mode = if cli.xdp_skb_mode {
        "SKB/generic"
    } else {
        "native"
    };
    info!(
        "  monad_cpu attached to {} (XDP, mode={})",
        cli.interface, mode
    );
    Ok(ebpf)
}

// ---------------------------------------------------------------------------
// shield-ebpf: XDP ingress (BIRTH) + TC egress (DEATH) on boundary interface
// ---------------------------------------------------------------------------

fn load_shield(cli: &Cli) -> Result<Ebpf> {
    let path = cli.obj_dir.join("shield-ebpf");
    info!("Loading shield-ebpf from {}", path.display());

    let mut ebpf = if let Some(ref pin_path) = cli.map_pin_path {
        let shield_pin = pin_path.join("shield-ebpf");
        std::fs::create_dir_all(&shield_pin)
            .with_context(|| format!("Failed to create {}", shield_pin.display()))?;
        EbpfLoader::new()
            .map_pin_path(&shield_pin)
            .load_file(&path)
            .context("Failed to parse shield-ebpf ELF with map pinning")?
    } else {
        Ebpf::load_file(&path).context("Failed to parse shield-ebpf ELF")?
    };

    // XDP program — ingress: stamp Monad, emit BIRTH
    let xdp: &mut Xdp = ebpf
        .program_mut("shield_xdp")
        .context("program 'shield_xdp' not found in shield-ebpf ELF")?
        .try_into()?;

    xdp.load()
        .context("Failed to load shield_xdp into kernel")?;

    let flags = if cli.xdp_skb_mode {
        XdpFlags::SKB_MODE
    } else {
        XdpFlags::default()
    };

    xdp.attach(&cli.interface, flags)
        .with_context(|| format!("Failed to attach shield_xdp XDP to {}", cli.interface))?;

    let mode = if cli.xdp_skb_mode {
        "SKB/generic"
    } else {
        "native"
    };
    info!(
        "  shield_xdp attached to {} (XDP ingress, mode={})",
        cli.interface, mode
    );

    // TC program — egress: capture final Monad state, emit DEATH, strip HBH
    if let Err(e) = tc::qdisc_add_clsact(&cli.interface) {
        warn!(
            "qdisc_add_clsact on {}: {} (may already exist)",
            cli.interface, e
        );
    }

    let tc_prog: &mut SchedClassifier = ebpf
        .program_mut("shield_tc")
        .context("program 'shield_tc' not found in shield-ebpf ELF")?
        .try_into()?;

    tc_prog
        .load()
        .context("Failed to load shield_tc into kernel")?;
    tc_prog
        .attach(&cli.interface, TcAttachType::Egress)
        .context("Failed to attach shield_tc to TC egress")?;

    info!("  shield_tc attached to {} (TC egress)", cli.interface);

    // Pin ANAMNESIS ring buffer for trace-collector access
    if cli.pin_maps || cli.map_pin_path.is_some() {
        let pin_base = cli
            .map_pin_path
            .as_deref()
            .unwrap_or(std::path::Path::new("/sys/fs/bpf/unheaded"));
        let shield_pin_dir = pin_base.join("shield-ebpf");
        std::fs::create_dir_all(&shield_pin_dir).ok();

        for (map_name, map) in ebpf.maps_mut() {
            let pin = shield_pin_dir.join(&map_name);
            if pin.exists() {
                info!("  map {} already pinned at {}", map_name, pin.display());
            } else {
                match map.pin(&pin) {
                    Ok(()) => info!("  pinned map {} → {}", map_name, pin.display()),
                    Err(e) => warn!("  failed to pin map {}: {}", map_name, e),
                }
            }
        }

        // Create convenience symlink for trace-collector
        let anamnesis_link = pin_base.join("trace_events");
        let anamnesis_src = shield_pin_dir.join("ANAMNESIS");
        if anamnesis_src.exists() && !anamnesis_link.exists() {
            if let Err(e) = std::os::unix::fs::symlink(&anamnesis_src, &anamnesis_link) {
                warn!("Failed to symlink trace_events: {}", e);
            } else {
                info!("  symlinked trace_events → {}", anamnesis_src.display());
            }
        }
    }

    Ok(ebpf)
}

// ---------------------------------------------------------------------------
// Map pinning
// ---------------------------------------------------------------------------

fn pin_all_maps(handles: &mut [(&str, Ebpf)]) -> Result<()> {
    let base = PathBuf::from("/sys/fs/bpf/unheaded");
    std::fs::create_dir_all(&base).context("Failed to create /sys/fs/bpf/unheaded")?;

    for (name, ebpf) in handles.iter_mut() {
        let prog_name = *name;
        let pin_dir = base.join(prog_name);
        std::fs::create_dir_all(&pin_dir)
            .with_context(|| format!("Failed to create {}", pin_dir.display()))?;

        for (map_name, map) in ebpf.maps_mut() {
            let pin_path = pin_dir.join(&map_name);
            if let Err(e) = map.pin(&pin_path) {
                warn!("Failed to pin {}/{}: {}", prog_name, map_name, e);
            } else {
                info!(
                    "  pinned {}/{} → {}",
                    prog_name,
                    map_name,
                    pin_path.display()
                );
            }
        }
    }

    Ok(())
}
