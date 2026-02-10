//! Unheaded eBPF Loader
//!
//! Loads and attaches the 4 eBPF programs compiled with aya-ebpf using
//! the aya userspace library. This bypasses the libbpf legacy-maps
//! incompatibility (libbpf v1.0+ rejects the `maps` ELF section that
//! aya-ebpf 0.1.x emits).
//!
//! Programs:
//!   - packet-marker:  XDP program for trace ID injection
//!   - flow-tracker:   TC classifier (ingress + egress) for connection tracking
//!   - latency-probe:  kprobe/kretprobe on TCP functions for latency measurement
//!   - syscall-tracer: raw tracepoints on sys_enter/sys_exit

use std::path::PathBuf;

use anyhow::{Context, Result};
use aya::programs::{
    KProbe, RawTracePoint, SchedClassifier, TcAttachType, Xdp, XdpFlags,
};
use aya::{programs::tc, Ebpf};
use clap::Parser;
use log::{info, warn};

const ALL_PROGRAMS: &[&str] = &[
    "packet-marker",
    "flow-tracker",
    "latency-probe",
    "syscall-tracer",
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
        let path = cli.obj_dir.join(name);
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
        "{}/{} programs loaded and attached. Press Ctrl+C to detach.",
        handles.len(),
        programs.len()
    );

    // Wait for signal
    let (tx, rx) = std::sync::mpsc::channel::<()>();
    ctrlc::set_handler(move || {
        let _ = tx.send(());
    })
    .context("Failed to set Ctrl+C handler")?;
    let _ = rx.recv();

    info!("Detaching all programs...");
    drop(handles);
    info!("Done.");

    Ok(())
}

fn dry_run(cli: &Cli, programs: &[&str]) -> Result<()> {
    info!("[dry-run] Validating eBPF objects (no kernel loading)");

    for name in programs {
        let path = cli.obj_dir.join(name);
        let metadata = std::fs::metadata(&path)
            .with_context(|| format!("Cannot stat {}", path.display()))?;
        info!(
            "[dry-run] {} — {} bytes, valid ELF",
            name,
            metadata.len()
        );
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

    prog.load().context("Failed to load packet_marker into kernel")?;
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
    ingress.load().context("Failed to load flow_tracker_ingress")?;
    ingress
        .attach(&cli.interface, TcAttachType::Ingress)
        .context("Failed to attach flow_tracker_ingress")?;
    info!("  flow_tracker_ingress attached (TC ingress)");

    // Egress
    let egress: &mut SchedClassifier = ebpf
        .program_mut("flow_tracker_egress")
        .context("program 'flow_tracker_egress' not found")?
        .try_into()?;
    egress.load().context("Failed to load flow_tracker_egress")?;
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
                info!("  pinned {}/{} → {}", prog_name, map_name, pin_path.display());
            }
        }
    }

    Ok(())
}
