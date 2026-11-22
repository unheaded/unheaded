// SPDX-License-Identifier: GPL-2.0-only
// Copyright (c) 2024-2026 Stevie Bellis.
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
//!   - hop-ebpf:       XDP per-hop Monad processing; only with --only hop-ebpf

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

    /// Load objects a non-root user could have written, as a build tree's are.
    /// Development only; the default --obj-dir is a build tree.
    #[arg(long)]
    allow_untrusted_ebpf_obj_dir: bool,
}

fn main() -> Result<()> {
    env_logger::Builder::from_env(env_logger::Env::default().default_filter_or("info")).init();

    let cli = Cli::parse();

    let programs: Vec<&str> = match &cli.only {
        Some(list) => list.iter().map(|s| s.as_str()).collect(),
        None => ALL_PROGRAMS.to_vec(),
    };

    // Validate all objects exist, and that only root could have written them,
    // before loading any
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
        objtrust::check_unless_allowed(&path, cli.allow_untrusted_ebpf_obj_dir)?;
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
            "hop-ebpf" => load_hop(&cli),
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

    // Wait for SIGINT, SIGTERM or SIGHUP (ctrlc's `termination` feature adds
    // the last two): systemd stops a unit with SIGTERM, and the cleanup below
    // has to run for it too.
    let (tx, rx) = std::sync::mpsc::channel::<()>();
    ctrlc::set_handler(move || {
        let _ = tx.send(());
    })
    .context("Failed to set the signal handler")?;
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
            let pin = pin_path.join(map_name);
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
// Pins left by an earlier load
// ---------------------------------------------------------------------------

/// Where maps are pinned when neither --map-pin-path nor another base is given.
const DEFAULT_PIN_BASE: &str = "/sys/fs/bpf/unheaded";

/// The base directory this run pins under, if it pins at all.
fn pin_base(cli: &Cli) -> Option<PathBuf> {
    if !(cli.pin_maps || cli.map_pin_path.is_some()) {
        return None;
    }
    Some(
        cli.map_pin_path
            .clone()
            .unwrap_or_else(|| PathBuf::from(DEFAULT_PIN_BASE)),
    )
}

/// Refuse a pin directory that already holds something. Pins outlive the
/// loader, and none of these objects' maps is pinned by name, so every load
/// makes new maps: a pin found here names a map of an earlier load. Left in
/// place it would be read (the collector, on Shield's ANAMNESIS) or written
/// (`unheaded hop rules apply`) in place of the map the new programs use.
/// Called before anything is parsed or attached. The loader does not remove
/// the pins itself: another loader may be running on them.
fn refuse_stale_pins(pin_dir: &std::path::Path) -> Result<()> {
    let entries = match std::fs::read_dir(pin_dir) {
        Ok(entries) => entries,
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => return Ok(()),
        Err(e) => {
            return Err(e).with_context(|| format!("Failed to read {}", pin_dir.display()));
        }
    };
    let mut found: Vec<PathBuf> = Vec::new();
    for entry in entries {
        let entry = entry.with_context(|| format!("Failed to read {}", pin_dir.display()))?;
        found.push(entry.path());
    }
    found.sort();
    let Some(first) = found.first() else {
        return Ok(());
    };
    anyhow::bail!(
        "{} was left by an earlier load ({} in {}): it names a map this load would not use, \
         so whatever reads or writes the pin would work on the old map. Make sure no other \
         loader is running, remove {}, and start again",
        first.display(),
        if found.len() == 1 {
            "the only entry".to_string()
        } else {
            format!("one of {} entries", found.len())
        },
        pin_dir.display(),
        pin_dir.display()
    );
}

/// Refuse a trace_events link that points anywhere but `target`. A link to
/// `target` itself is kept: the path is the same, and it names the new pin
/// once this load has made it.
fn refuse_stale_trace_link(link: &std::path::Path, target: &std::path::Path) -> Result<()> {
    match std::fs::read_link(link) {
        Ok(current) if current == target => Ok(()),
        Ok(current) => anyhow::bail!(
            "the trace_events link {} points to {}, not to {}: it was left by an earlier load \
             and the collector would read that map. Remove the link and start again",
            link.display(),
            current.display(),
            target.display()
        ),
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(()),
        Err(e) => Err(e).with_context(|| {
            format!(
                "{} is in the way of the trace_events link and is not a symlink",
                link.display()
            )
        }),
    }
}

// ---------------------------------------------------------------------------
// shield-ebpf: XDP ingress (BIRTH) + TC egress (DEATH) on boundary interface
// ---------------------------------------------------------------------------

/// Pinning is refused, not warned about (M-198.7): a pin already there, a pin
/// that cannot be made and a trace_events link to another map are errors, as
/// in load_hop. The collector reads Shield's ANAMNESIS through these paths, and
/// a stale one feeds it a map no program writes.
fn load_shield(cli: &Cli) -> Result<Ebpf> {
    let path = cli.obj_dir.join("shield-ebpf");
    info!("Loading shield-ebpf from {}", path.display());

    if let Some(base) = pin_base(cli) {
        let pin_dir = base.join("shield-ebpf");
        refuse_stale_pins(&pin_dir)?;
        refuse_stale_trace_link(&base.join("trace_events"), &pin_dir.join("ANAMNESIS"))?;
    }

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

    // Pin every map, ANAMNESIS among them, for trace-collector access
    if let Some(pin_base) = pin_base(cli) {
        let shield_pin_dir = pin_base.join("shield-ebpf");
        std::fs::create_dir_all(&shield_pin_dir)
            .with_context(|| format!("Failed to create {}", shield_pin_dir.display()))?;

        for (map_name, map) in ebpf.maps_mut() {
            // .rodata and the like are the object's constants, not state, and
            // bpffs refuses a dot in a name (EPERM).
            if map_name.starts_with('.') {
                continue;
            }
            let pin = shield_pin_dir.join(map_name);
            map.pin(&pin)
                .with_context(|| format!("Failed to pin map {} at {}", map_name, pin.display()))?;
            info!("  pinned map {} → {}", map_name, pin.display());
        }

        // Convenience symlink for trace-collector. One that already names this
        // path was checked before the load and now leads to the new pin.
        let anamnesis_link = pin_base.join("trace_events");
        let anamnesis_src = shield_pin_dir.join("ANAMNESIS");
        if std::fs::read_link(&anamnesis_link).is_err() {
            std::os::unix::fs::symlink(&anamnesis_src, &anamnesis_link).with_context(|| {
                format!(
                    "Failed to link {} to {}",
                    anamnesis_link.display(),
                    anamnesis_src.display()
                )
            })?;
            info!("  symlinked trace_events → {}", anamnesis_src.display());
        }
    }

    Ok(ebpf)
}

// ---------------------------------------------------------------------------
// hop-ebpf: XDP
// ---------------------------------------------------------------------------

/// Outside the default program set: `--only hop-ebpf` is the one way in. Pin
/// handling follows load_shield's, with two differences. There is no
/// trace_events link, which stays Shield's. And a pin that cannot be made is
/// an error: `unheaded hop rules apply` writes through these pins, so a stale
/// one left by an earlier load would take rules this object never reads.
fn load_hop(cli: &Cli) -> Result<Ebpf> {
    let path = cli.obj_dir.join("hop-ebpf");
    info!("Loading hop-ebpf from {}", path.display());

    // The same refusal the pin loop below would give, before anything is
    // loaded or attached.
    if let Some(base) = pin_base(cli) {
        refuse_stale_pins(&base.join("hop-ebpf"))?;
    }

    let mut ebpf = if let Some(ref pin_path) = cli.map_pin_path {
        let hop_pin = pin_path.join("hop-ebpf");
        std::fs::create_dir_all(&hop_pin)
            .with_context(|| format!("Failed to create {}", hop_pin.display()))?;
        EbpfLoader::new()
            .map_pin_path(&hop_pin)
            .load_file(&path)
            .context("Failed to parse hop-ebpf ELF with map pinning")?
    } else {
        Ebpf::load_file(&path).context("Failed to parse hop-ebpf ELF")?
    };

    let xdp: &mut Xdp = ebpf
        .program_mut("hop_xdp")
        .context("program 'hop_xdp' not found in hop-ebpf ELF")?
        .try_into()?;

    xdp.load().context("Failed to load hop_xdp into kernel")?;

    let flags = if cli.xdp_skb_mode {
        XdpFlags::SKB_MODE
    } else {
        XdpFlags::default()
    };

    xdp.attach(&cli.interface, flags)
        .with_context(|| format!("Failed to attach hop_xdp XDP to {}", cli.interface))?;

    let mode = if cli.xdp_skb_mode {
        "SKB/generic"
    } else {
        "native"
    };
    info!(
        "  hop_xdp attached to {} (XDP, mode={})",
        cli.interface, mode
    );

    if let Some(pin_base) = pin_base(cli) {
        let hop_pin_dir = pin_base.join("hop-ebpf");
        std::fs::create_dir_all(&hop_pin_dir)
            .with_context(|| format!("Failed to create {}", hop_pin_dir.display()))?;

        for (map_name, map) in ebpf.maps_mut() {
            // .rodata.cst8 and the like are the object's constants, not state,
            // and bpffs refuses a dot in a name (EPERM).
            if map_name.starts_with('.') {
                continue;
            }
            let pin = hop_pin_dir.join(map_name);
            map.pin(&pin)
                .with_context(|| format!("Failed to pin map {} at {}", map_name, pin.display()))?;
            info!("  pinned map {} → {}", map_name, pin.display());
        }
    }

    Ok(ebpf)
}

// ---------------------------------------------------------------------------
// Map pinning
// ---------------------------------------------------------------------------

fn pin_all_maps(handles: &mut [(&str, Ebpf)]) -> Result<()> {
    let base = PathBuf::from(DEFAULT_PIN_BASE);
    std::fs::create_dir_all(&base).context("Failed to create /sys/fs/bpf/unheaded")?;

    for (name, ebpf) in handles.iter_mut() {
        let prog_name = *name;
        let pin_dir = base.join(prog_name);
        std::fs::create_dir_all(&pin_dir)
            .with_context(|| format!("Failed to create {}", pin_dir.display()))?;

        for (map_name, map) in ebpf.maps_mut() {
            let pin_path = pin_dir.join(map_name);
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
