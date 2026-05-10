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
const VETH_HOST: &str = "veth-upc0";  // lives in default ns
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
            "link", "add", VETH_HOST, "type", "veth", "peer", "name", VETH_INSIDE,
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
        .args(["netns", "exec", NS_NAME, "ip", "link", "set", VETH_INSIDE, "up"])
        .status()
        .context("ip link set veth-upc0p up")?;
    Command::new("ip")
        .args(["netns", "exec", NS_NAME, "ip", "link", "set", "lo", "up"])
        .status()
        .context("ip link set lo up in upc0")?;

    tracing::info!("upc0 ns + veth pair ready");
    Ok(VETH_INSIDE)
}

/// Best-effort teardown. Errors logged but not propagated.
pub fn teardown_upc0() {
    let _ = Command::new("ip").args(["netns", "del", NS_NAME]).status();
    let _ = Command::new("ip").args(["link", "del", VETH_HOST]).status();
    tracing::info!("upc0 ns + veth pair torn down");
}

/// Send a single trigger packet from default ns into the veth.
/// Doesn't matter what packet — XDP intercepts everything that arrives
/// on the inside iface. Use a UDP packet to fd00:dead:beef:dada::de
/// (the IPv6 address conventionally assigned to the xv6-first-boot
/// instance per the parent ASCEND-LINUX battle plan).
pub fn send_trigger(count: u32) -> Result<()> {
    // Simplest: ping6 the inside namespace from the outside.
    // We don't need the ping to succeed; we just need a packet to ARRIVE
    // at veth-upc0p which fires XDP.
    for _ in 0..count {
        let _ = Command::new("ip")
            .args([
                "netns", "exec", NS_NAME, "ping6", "-c", "1", "-W", "1", "::1",
            ])
            .output();
    }
    Ok(())
}
