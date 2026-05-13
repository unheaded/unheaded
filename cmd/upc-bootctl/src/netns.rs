// SPDX-License-Identifier: GPL-3.0-or-later
//! Minimal network namespace + veth setup for upc-bootctl.
//!
//! Phase 1.1 SHIP needs ONE veth pair so XDP can attach + we can poke
//! a trigger packet. Mirrors the simpler half of `crates/doom-runner/src/ring.rs`
//! (single-hop instead of multi-hop ring). Idempotent — safe to call
//! repeatedly during dev.

#![allow(dead_code)]

use anyhow::{Context, Result};
use std::process::Command;

const NS_NAME: &str = "upc0";
const VETH_HOST: &str = "veth-upc0"; // lives in default ns
const VETH_INSIDE: &str = "veth-upc0p"; // moved into upc0 ns

/// Tear down any prior state, then create namespace + veth pair.
/// Returns the name of the iface inside the namespace (XDP attach target).
pub fn setup_upc0() -> Result<&'static str> {
    // Best-effort teardown so re-runs work cleanly. Any failure is fine —
    // probably means the resource didn't exist.
    let _ = Command::new("ip").args(["netns", "del", NS_NAME]).status();
    let _ = Command::new("ip").args(["link", "del", VETH_HOST]).status();

    Command::new("ip")
        .args(["netns", "add", NS_NAME])
        .status()
        .context("ip netns add upc0")?
        .success()
        .then_some(())
        .context("ip netns add upc0 returned non-zero")?;

    Command::new("ip")
        .args([
            "link",
            "add",
            VETH_HOST,
            "type",
            "veth",
            "peer",
            "name",
            VETH_INSIDE,
        ])
        .status()
        .context("ip link add veth pair")?
        .success()
        .then_some(())
        .context("ip link add veth pair returned non-zero")?;

    // Move the inside-end into upc0 namespace
    Command::new("ip")
        .args(["link", "set", VETH_INSIDE, "netns", NS_NAME])
        .status()
        .context("ip link set veth into ns")?
        .success()
        .then_some(())
        .context("ip link set returned non-zero")?;

    // Bring both ends up
    Command::new("ip")
        .args(["link", "set", VETH_HOST, "up"])
        .status()
        .context("ip link set veth-upc0 up")?;
    Command::new("ip")
        .args([
            "netns",
            "exec",
            NS_NAME,
            "ip",
            "link",
            "set",
            VETH_INSIDE,
            "up",
        ])
        .status()
        .context("ip link set veth-upc0p up")?;
    Command::new("ip")
        .args(["netns", "exec", NS_NAME, "ip", "link", "set", "lo", "up"])
        .status()
        .context("ip link set lo up in upc0")?;

    // Assign IPv6 link-local addresses so ping6 can route across the pair.
    // Host-side: fd00:dead:beef:dada::1, namespace-side: ::de
    Command::new("ip")
        .args(["addr", "add", "fd00:dead:beef:dada::1/64", "dev", VETH_HOST])
        .status()
        .context("ip addr add host-side")?;
    Command::new("ip")
        .args([
            "netns",
            "exec",
            NS_NAME,
            "ip",
            "addr",
            "add",
            "fd00:dead:beef:dada::de/64",
            "dev",
            VETH_INSIDE,
        ])
        .status()
        .context("ip addr add ns-side")?;
    // Brief settle for SLAAC + DAD
    std::thread::sleep(std::time::Duration::from_millis(100));

    tracing::info!("upc0 ns + veth pair ready (host=::1, ns=::de)");
    Ok(VETH_INSIDE)
}

/// Best-effort teardown. Errors logged but not propagated.
pub fn teardown_upc0() {
    let _ = Command::new("ip").args(["netns", "del", NS_NAME]).status();
    let _ = Command::new("ip").args(["link", "del", VETH_HOST]).status();
    tracing::info!("upc0 ns + veth pair torn down");
}

/// Send `count` Monad-format trigger packets via scripts/doom-tick.py.
/// This is the canonical UPC tick format: IPv6 + Hop-by-Hop + Monad
/// register, with flow_label = instance ID. The eBPF program dispatches
/// on `flow_label & 0xFF` so flow_label=0xDE selects CPU instance 0xDE.
///
/// Plain ping6 would have flow_label=0 → no match in CPU_MAP → XDP_DROP.
pub fn send_trigger(count: u32, instance: u8) -> Result<()> {
    let flow_label = format!("0x{:X}", instance);
    let count_str = count.to_string();
    // Resolve absolute path so cwd (cargo subdir) doesn't matter.
    let script = std::env::var("UPC_DOOM_TICK").unwrap_or_else(|_| {
        // Walk up to repo root by looking for the .git dir
        let mut cur = std::env::current_dir().expect("cwd");
        for _ in 0..10 {
            if cur.join(".git").exists() {
                break;
            }
            if !cur.pop() {
                break;
            }
        }
        cur.join("scripts/doom-tick.py")
            .to_string_lossy()
            .into_owned()
    });
    // doom-tick.py reads --interface and uses AF_PACKET. We send into
    // upc0 namespace (the inside of the veth, where XDP is attached
    // host-side at the OTHER end). This makes packets traverse
    // veth-upc0p → veth-upc0 = XDP ingress.
    let out = Command::new("ip")
        .args([
            "netns",
            "exec",
            NS_NAME,
            "python3",
            &script,
            "--flow-label",
            &flow_label,
            "--count",
            &count_str,
            "--burst",
            "--interface",
            VETH_INSIDE,
        ])
        .output()
        .context("invoke scripts/doom-tick.py")?;
    let stdout = String::from_utf8_lossy(&out.stdout);
    let stderr = String::from_utf8_lossy(&out.stderr);
    if !out.status.success() {
        eprintln!(
            "doom-tick.py FAILED ({}):\nstdout: {}\nstderr: {}",
            out.status, stdout, stderr
        );
    } else if !stderr.is_empty() {
        eprintln!("doom-tick.py stderr: {}", stderr);
    } else {
        // Show last line of stdout for visibility
        let last = stdout.lines().rev().next().unwrap_or("");
        eprintln!("doom-tick.py: {}", last);
    }
    Ok(())
}
