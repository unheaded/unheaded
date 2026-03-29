// SPDX-License-Identifier: GPL-3.0-only
//! Network namespace and veth ring setup for Doom-over-IPv6.
//!
//! Replaces `doom-ring.sh` with structured Rust code. Uses `std::process::Command`
//! to call `ip` (iproute2) for namespace/veth management, and Aya for XDP attachment.
//!
//! # Topology
//!
//! ```text
//!   ns0 --veth01--> ns1
//!    ^                |
//!    |                v
//!   ns1 <--veth10-- ns0    (for 2-hop ring)
//! ```
//!
//! Each hop runs `monad_cpu` at XDP. All hops share the same BPF maps via pinning.
//! The wire is the processor. Wotan is the RAM.

use std::process::Command;

use anyhow::{bail, Context, Result};
use tracing::{debug, info, warn};

/// Kingdom ULA prefix (per draft-bellis-02 section 7).
const IPV6_PREFIX: &str = "fd00:3f:75";

/// Default namespace prefix.
const NS_PREFIX: &str = "monad";

/// Default MTU for veth pairs.
const MTU: u32 = 1500;

/// BPF filesystem root for Doom ring pins.
const BPFFS_ROOT: &str = "/sys/fs/bpf/unheaded/doom-ring";

/// Configuration for the packet circulation ring.
#[derive(Debug, Clone)]
pub struct RingConfig {
    /// Number of hops in the ring (2-6).
    pub num_hops: u32,
    /// Namespace prefix (default: "monad").
    pub ns_prefix: String,
    /// IPv6 ULA prefix.
    pub ipv6_prefix: String,
    /// MTU for veth pairs.
    pub mtu: u32,
    /// Path to the compiled monad-cpu-ebpf object.
    pub ebpf_obj_path: String,
    /// BPF map pin directory.
    pub map_pin_dir: String,
}

impl Default for RingConfig {
    fn default() -> Self {
        Self {
            num_hops: 2,
            ns_prefix: NS_PREFIX.to_string(),
            ipv6_prefix: IPV6_PREFIX.to_string(),
            mtu: MTU,
            ebpf_obj_path: String::new(),
            map_pin_dir: format!("{BPFFS_ROOT}/maps"),
        }
    }
}

/// Run an `ip` command, returning its stdout. Fails on non-zero exit.
fn ip_cmd(args: &[&str]) -> Result<String> {
    let output = Command::new("ip")
        .args(args)
        .output()
        .context("failed to execute `ip` command")?;

    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr);
        bail!("ip {} failed: {}", args.join(" "), stderr.trim());
    }

    Ok(String::from_utf8_lossy(&output.stdout).to_string())
}

/// Run a command inside a network namespace.
fn ip_netns_exec(ns: &str, args: &[&str]) -> Result<String> {
    let mut full_args = vec!["netns", "exec", ns, "ip"];
    full_args.extend_from_slice(args);
    ip_cmd(&full_args)
}

/// Run a sysctl inside a network namespace.
fn sysctl_in_ns(ns: &str, key: &str, value: &str) -> Result<()> {
    let output = Command::new("ip")
        .args(["netns", "exec", ns, "sysctl", "-qw", &format!("{key}={value}")])
        .output()
        .context("failed to execute sysctl")?;

    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr);
        warn!("sysctl {key}={value} in {ns}: {}", stderr.trim());
    }
    Ok(())
}

/// Check if a network namespace exists.
fn ns_exists(name: &str) -> bool {
    std::path::Path::new(&format!("/var/run/netns/{name}")).exists()
}

/// Set up the packet circulation ring.
///
/// Creates namespaces, veth pairs, IPv6 addresses, routes, and neighbor entries.
/// Does NOT load BPF programs — that is handled by the loader/tail_calls modules.
pub fn setup(config: &RingConfig) -> Result<()> {
    info!("setting up doom ring ({} hops)...", config.num_hops);

    // Ensure bpffs is mounted
    ensure_bpffs()?;
    std::fs::create_dir_all(&config.map_pin_dir)
        .with_context(|| format!("failed to create map pin dir: {}", config.map_pin_dir))?;

    // Create namespaces
    for i in 0..config.num_hops {
        let ns = format!("{}{i}", config.ns_prefix);
        if ns_exists(&ns) {
            warn!("namespace {ns} already exists, skipping creation");
        } else {
            ip_cmd(&["netns", "add", &ns])
                .with_context(|| format!("failed to create namespace {ns}"))?;
            info!("created namespace: {ns}");
        }
        // Bring up loopback
        let _ = ip_netns_exec(&ns, &["link", "set", "lo", "up"]);
    }

    // Create veth pairs (directed ring)
    for i in 0..config.num_hops {
        let j = (i + 1) % config.num_hops;
        let ns_src = format!("{}{i}", config.ns_prefix);
        let ns_dst = format!("{}{j}", config.ns_prefix);
        let veth_src = format!("veth{i}{j}");
        let veth_dst = format!("veth{i}{j}p");

        // Check if already exists
        if ip_netns_exec(&ns_src, &["link", "show", &veth_src]).is_ok() {
            warn!("veth pair {veth_src} already exists, skipping");
            continue;
        }

        // Create veth pair in default namespace, then move to target namespaces
        ip_cmd(&["link", "add", &veth_src, "type", "veth", "peer", "name", &veth_dst])?;
        ip_cmd(&["link", "set", &veth_src, "netns", &ns_src])?;
        ip_cmd(&["link", "set", &veth_dst, "netns", &ns_dst])?;

        // IPv6 addresses
        let addr_src = format!("{}:{i}::1/64", config.ipv6_prefix);
        let addr_dst = format!("{}:{i}::2/64", config.ipv6_prefix);
        ip_netns_exec(&ns_src, &["-6", "addr", "add", &addr_src, "dev", &veth_src])?;
        ip_netns_exec(&ns_dst, &["-6", "addr", "add", &addr_dst, "dev", &veth_dst])?;

        // MTU + up
        let mtu_str = config.mtu.to_string();
        ip_netns_exec(&ns_src, &["link", "set", &veth_src, "mtu", &mtu_str, "up"])?;
        ip_netns_exec(&ns_dst, &["link", "set", &veth_dst, "mtu", &mtu_str, "up"])?;

        // Disable DAD
        let dad_key_src = format!("net.ipv6.conf.{veth_src}.accept_dad");
        let dad_key_dst = format!("net.ipv6.conf.{veth_dst}.accept_dad");
        let _ = sysctl_in_ns(&ns_src, &dad_key_src, "0");
        let _ = sysctl_in_ns(&ns_dst, &dad_key_dst, "0");

        info!("veth: {ns_src}/{veth_src} <-> {ns_dst}/{veth_dst}");
    }

    // Static routes for circulation
    for i in 0..config.num_hops {
        let j = (i + 1) % config.num_hops;
        let ns = format!("{}{i}", config.ns_prefix);
        let veth_out = format!("veth{i}{j}");
        let via = format!("{}:{i}::2", config.ipv6_prefix);

        // Try replace first, then add
        if ip_netns_exec(&ns, &["-6", "route", "replace", "default", "via", &via, "dev", &veth_out]).is_err() {
            let _ = ip_netns_exec(&ns, &["-6", "route", "add", "default", "via", &via, "dev", &veth_out]);
        }

        debug!("route: {ns} -> {}{j} via {veth_out}", config.ns_prefix);
    }

    // Enable IPv6 forwarding
    for i in 0..config.num_hops {
        let ns = format!("{}{i}", config.ns_prefix);
        let _ = sysctl_in_ns(&ns, "net.ipv6.conf.all.forwarding", "1");
    }

    // Static neighbor entries
    for i in 0..config.num_hops {
        let j = (i + 1) % config.num_hops;
        let ns_src = format!("{}{i}", config.ns_prefix);
        let ns_dst = format!("{}{j}", config.ns_prefix);
        let veth_src = format!("veth{i}{j}");
        let veth_dst = format!("veth{i}{j}p");

        // Get MAC of peer
        if let Ok(output) = ip_netns_exec(&ns_dst, &["link", "show", &veth_dst]) {
            if let Some(mac) = parse_mac_from_ip_output(&output) {
                let neigh_addr = format!("{}:{i}::2", config.ipv6_prefix);
                let _ = ip_netns_exec(
                    &ns_src,
                    &["-6", "neigh", "replace", &neigh_addr, "lladdr", &mac, "dev", &veth_src, "nud", "permanent"],
                );
            }
        }
    }

    info!("doom ring setup complete ({} hops)", config.num_hops);
    Ok(())
}

/// Tear down the packet circulation ring.
pub fn teardown(config: &RingConfig) -> Result<()> {
    info!("tearing down doom ring...");

    // Detach XDP from all hops
    for i in 0..config.num_hops {
        let prev = if i == 0 { config.num_hops - 1 } else { i - 1 };
        let ns = format!("{}{i}", config.ns_prefix);
        let veth_in = format!("veth{prev}{i}p");

        if ns_exists(&ns) {
            let _ = ip_netns_exec(&ns, &["link", "set", &veth_in, "xdp", "off"]);
        }
    }

    // Remove BPF pins
    let bpffs = std::path::Path::new(BPFFS_ROOT);
    if bpffs.exists() {
        std::fs::remove_dir_all(bpffs).ok();
        info!("removed BPF pins: {BPFFS_ROOT}");
    }

    // Delete namespaces (also removes veth pairs)
    for i in 0..config.num_hops {
        let ns = format!("{}{i}", config.ns_prefix);
        if ns_exists(&ns) {
            ip_cmd(&["netns", "del", &ns])?;
            info!("deleted namespace: {ns}");
        }
    }

    info!("teardown complete");
    Ok(())
}

/// Ensure bpffs is mounted.
fn ensure_bpffs() -> Result<()> {
    let mounts = std::fs::read_to_string("/proc/mounts")
        .context("failed to read /proc/mounts")?;

    if !mounts.contains("type bpf") && !mounts.contains("bpf /sys/fs/bpf") {
        info!("mounting bpffs at /sys/fs/bpf");
        let status = Command::new("mount")
            .args(["-t", "bpf", "bpf", "/sys/fs/bpf"])
            .status()
            .context("failed to mount bpffs")?;
        if !status.success() {
            bail!("failed to mount bpffs (exit code: {status})");
        }
    }
    Ok(())
}

/// Parse a MAC address from `ip link show` output.
/// Looks for a line containing "ether" followed by a MAC address.
fn parse_mac_from_ip_output(output: &str) -> Option<String> {
    for line in output.lines() {
        let trimmed = line.trim();
        if let Some(rest) = trimmed.strip_prefix("link/ether ") {
            if let Some(mac) = rest.split_whitespace().next() {
                return Some(mac.to_string());
            }
        }
    }
    None
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_mac_works() {
        let output = "2: veth01p@if3: <BROADCAST,MULTICAST,UP> mtu 1500\n    \
                       link/ether aa:bb:cc:dd:ee:ff brd ff:ff:ff:ff:ff:ff";
        assert_eq!(
            parse_mac_from_ip_output(output),
            Some("aa:bb:cc:dd:ee:ff".to_string())
        );
    }

    #[test]
    fn parse_mac_no_match() {
        assert_eq!(parse_mac_from_ip_output("no ethernet here"), None);
    }

    #[test]
    fn default_config() {
        let cfg = RingConfig::default();
        assert_eq!(cfg.num_hops, 2);
        assert_eq!(cfg.ns_prefix, "monad");
        assert_eq!(cfg.mtu, 1500);
    }
}
