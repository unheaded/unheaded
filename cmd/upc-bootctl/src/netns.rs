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

// ── S-ASCEND-2 Items 1-3: the namespace made by scripts/upc-netns.sh ──────
//
// `scripts/upc-netns.sh up` (or unheaded-upc-netns.service) creates `upc0`
// with BOTH veth ends inside it, so nothing the guest emits reaches the root
// namespace, and installs a forward-drop rule there as defence in depth.
// bootctl runs inside `upc0` (`ip netns exec upc0 …`, or the unit's
// NetworkNamespacePath=) and needs no `ip`, no netns creation and no execve:
// it attaches XDP to `veth-upc0` and sends triggers out of `veth-upc0p` over
// an AF_PACKET socket (CAP_BPF + CAP_NET_ADMIN + CAP_NET_RAW).

static CREATE_NETNS: std::sync::atomic::AtomicBool = std::sync::atomic::AtomicBool::new(false);

/// `--create-netns`: the old root-namespace setup (bootctl runs `ip` and
/// `ip netns exec upc0 python3 doom-tick.py`; needs root). Default off.
pub fn set_create_netns(on: bool) {
    CREATE_NETNS.store(on, std::sync::atomic::Ordering::SeqCst);
}

fn create_mode() -> bool {
    CREATE_NETNS.load(std::sync::atomic::Ordering::SeqCst)
}

/// Before XDP attach: create the old setup (`--create-netns`) or verify the
/// namespace upc-netns.sh made. Returns the XDP attach interface.
pub fn prepare() -> Result<&'static str> {
    if create_mode() {
        setup_upc0()?;
        Ok(VETH_HOST)
    } else {
        ensure_present()
    }
}

/// Send `count` triggers the way the current mode needs.
pub fn trigger(count: u32, instance: u8, verbose: bool) -> Result<()> {
    if create_mode() {
        send_trigger_opts(count, instance, verbose)
    } else {
        send_trigger_native(count, instance, verbose)
    }
}

/// After the run: tear down what `--create-netns` made. The shared namespace
/// belongs to upc-netns.sh and stays.
pub fn finish() {
    if create_mode() {
        teardown_upc0();
    }
}

/// The interface index of `name` in the calling process's namespace, or 0.
fn ifindex(name: &str) -> u32 {
    let Ok(c) = std::ffi::CString::new(name) else {
        return 0;
    };
    // SAFETY: c is a valid NUL-terminated string for the duration of the call.
    unsafe { libc::if_nametoindex(c.as_ptr()) }
}

/// Verify the caller is inside the namespace upc-netns.sh made: both veth
/// ends must be visible here. Returns the XDP attach interface.
pub fn ensure_present() -> Result<&'static str> {
    let host = ifindex(VETH_HOST);
    let inside = ifindex(VETH_INSIDE);
    if host == 0 || inside == 0 {
        anyhow::bail!(
            "{VETH_HOST}/{VETH_INSIDE} not found in this network namespace: run              `scripts/upc-netns.sh up` and start bootctl inside it              (`ip netns exec {NS_NAME} …` or unheaded-upc-bootctl@.service),              or pass --create-netns for the old root-namespace setup"
        );
    }
    tracing::info!(host, inside, "upc0 namespace present");
    Ok(VETH_HOST)
}

/// CRC-16/CCITT-FALSE (poly 0x1021, init 0xFFFF, no reflection): the Monad
/// register checksum, as in scripts/doom-tick.py.
fn crc16_ccitt_false(data: &[u8]) -> u16 {
    let mut crc: u16 = 0xFFFF;
    for &b in data {
        crc ^= (b as u16) << 8;
        for _ in 0..8 {
            crc = if crc & 0x8000 != 0 {
                (crc << 1) ^ 0x1021
            } else {
                crc << 1
            };
        }
    }
    crc
}

/// One trigger frame, byte for byte what `scripts/doom-tick.py` builds with
/// its defaults: Ethernet (dst 02:42:ac:11:00:03, src 02:42:ac:11:00:02) +
/// IPv6 (flow label, next header HBH, hop limit 255, fd00:3f:75::1 →
/// fd00:dead::1) + a 28-byte HBH block carrying the 20-byte Monad register
/// (version 1, services 0xFF, hop 1, trace id, flags CUSTOM, CRC-16).
fn build_trigger_frame(flow_label: u32, trace_id: u32) -> Vec<u8> {
    let mut monad = [0u8; 20];
    monad[0] = 0x01;
    monad[1] = 0xFF;
    monad[2] = 0xFF;
    monad[3] = 1;
    monad[4..8].copy_from_slice(&trace_id.to_be_bytes());
    monad[0x0B] = 0x02;
    let crc = crc16_ccitt_false(&monad[..0x12]);
    monad[0x12..].copy_from_slice(&crc.to_be_bytes());

    let mut hbh = [0u8; 28];
    hbh[0] = 59; // next header: no next header
    hbh[1] = 2;
    hbh[2] = 0x3E;
    hbh[3] = 20;
    hbh[4..24].copy_from_slice(&monad);

    let mut f = Vec::with_capacity(14 + 40 + hbh.len());
    f.extend_from_slice(&[0x02, 0x42, 0xac, 0x11, 0x00, 0x03]);
    f.extend_from_slice(&[0x02, 0x42, 0xac, 0x11, 0x00, 0x02]);
    f.extend_from_slice(&0x86DDu16.to_be_bytes());
    f.extend_from_slice(&((6u32 << 28) | (flow_label & 0xFFFFF)).to_be_bytes());
    f.extend_from_slice(&(hbh.len() as u16).to_be_bytes());
    f.push(0); // next header: hop-by-hop
    f.push(255);
    f.extend_from_slice(&[
        0xfd, 0x00, 0x00, 0x3f, 0x00, 0x75, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1,
    ]);
    f.extend_from_slice(&[0xfd, 0x00, 0xde, 0xad, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1]);
    f.extend_from_slice(&hbh);
    f
}

/// Send `count` trigger frames out of `veth-upc0p` from inside `upc0`, over an
/// AF_PACKET socket (CAP_NET_RAW). They arrive at `veth-upc0` ingress, where
/// XDP is attached. Trace ids start at a per-run value and increment, as the
/// script's did.
pub fn send_trigger_native(count: u32, instance: u8, verbose: bool) -> Result<()> {
    let idx = ifindex(VETH_INSIDE);
    if idx == 0 {
        anyhow::bail!("{VETH_INSIDE} not found: is bootctl inside the {NS_NAME} namespace?");
    }
    // SAFETY: plain socket(2); the fd is closed below on every path.
    let fd = unsafe {
        libc::socket(
            libc::AF_PACKET,
            libc::SOCK_RAW,
            (libc::ETH_P_IPV6 as u16).to_be() as i32,
        )
    };
    if fd < 0 {
        let e = std::io::Error::last_os_error();
        anyhow::bail!(
            "AF_PACKET socket: {e} (needs CAP_NET_RAW: run via unheaded-upc-bootctl@ or sudo)"
        );
    }
    // SAFETY: sockaddr_ll is plain data; zeroed is a valid initial value.
    let mut sll: libc::sockaddr_ll = unsafe { std::mem::zeroed() };
    sll.sll_family = libc::AF_PACKET as u16;
    sll.sll_protocol = (libc::ETH_P_IPV6 as u16).to_be();
    sll.sll_ifindex = idx as i32;
    sll.sll_halen = 6;
    sll.sll_addr[..6].copy_from_slice(&[0x02, 0x42, 0xac, 0x11, 0x00, 0x03]);
    let seed = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.subsec_nanos() ^ (d.as_secs() as u32))
        .unwrap_or(1)
        ^ std::process::id();
    let flow_label = instance as u32;
    let mut sent = 0u32;
    let mut err: Option<std::io::Error> = None;
    let mut stalls = 0u32;
    let mut i = 0u32;
    'frame: while i < count {
        let frame = build_trigger_frame(flow_label, seed.wrapping_add(i).max(1));
        // SAFETY: frame and sll outlive the call; lengths are their real sizes.
        let n = unsafe {
            libc::sendto(
                fd,
                frame.as_ptr() as *const libc::c_void,
                frame.len(),
                0,
                &sll as *const libc::sockaddr_ll as *const libc::sockaddr,
                std::mem::size_of::<libc::sockaddr_ll>() as libc::socklen_t,
            )
        };
        if n < 0 {
            let e = std::io::Error::last_os_error();
            // The receiving veth's backlog is full: the XDP side is behind.
            // Back off and resend this frame; give up after ~2 s without
            // progress (a stalled consumer, not a slow one).
            if e.raw_os_error() == Some(libc::ENOBUFS) && stalls < 20_000 {
                stalls += 1;
                std::thread::sleep(std::time::Duration::from_micros(100));
                continue 'frame;
            }
            err = Some(e);
            break;
        }
        stalls = 0;
        sent += 1;
        i += 1;
    }
    // SAFETY: fd is the socket opened above.
    unsafe { libc::close(fd) };
    if let Some(e) = err {
        anyhow::bail!("trigger send failed after {sent} of {count} frames: {e}");
    }
    if verbose {
        eprintln!("trigger: sent {sent} frames on {VETH_INSIDE} (flow label 0x{flow_label:X})");
    }
    Ok(())
}

/// Send `count` Monad-format trigger packets via scripts/doom-tick.py.
/// This is the canonical UPC tick format: IPv6 + Hop-by-Hop + Monad
/// register, with flow_label = instance ID. The eBPF program dispatches
/// on `flow_label & 0xFF` so flow_label=0xDE selects CPU instance 0xDE.
///
/// Plain ping6 would have flow_label=0 → no match in CPU_MAP → XDP_DROP.
pub fn send_trigger(count: u32, instance: u8) -> Result<()> {
    send_trigger_opts(count, instance, true)
}

/// `send_trigger` for the interactive console loop: prints only on failure,
/// so a batch every second does not flood stderr.
pub fn send_trigger_quiet(count: u32, instance: u8) -> Result<()> {
    send_trigger_opts(count, instance, false)
}

fn send_trigger_opts(count: u32, instance: u8, verbose: bool) -> Result<()> {
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
    } else if !verbose {
        // quiet: success is silent
    } else if !stderr.is_empty() {
        eprintln!("doom-tick.py stderr: {}", stderr);
    } else {
        // Show last line of stdout for visibility
        let last = stdout.lines().next_back().unwrap_or("");
        eprintln!("doom-tick.py: {}", last);
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn hex(b: &[u8]) -> String {
        b.iter().map(|x| format!("{x:02x}")).collect()
    }

    /// `scripts/doom-tick.py --dry-run --flow-label 0x6F --trace-id 0x12345678`
    /// (and 0xDE / 0xFFFFFFFF): the native sender must put the same bytes on
    /// the wire as the script it replaces.
    #[test]
    fn trigger_frame_matches_doom_tick_py() {
        assert_eq!(
            hex(&build_trigger_frame(0x6F, 0x1234_5678)),
            "0242ac1100030242ac11000286dd6000006f001c00fffd00003f007500000000000000000001\
             fd00dead0000000000000000000000013b023e1401ffff0112345678000000020000000000004a1f00000000"
        );
        assert_eq!(
            hex(&build_trigger_frame(0xDE, 0xFFFF_FFFF)),
            "0242ac1100030242ac11000286dd600000de001c00fffd00003f007500000000000000000001\
             fd00dead0000000000000000000000013b023e1401ffff01ffffffff0000000200000000000091fa00000000"
        );
    }

    #[test]
    fn trigger_frame_masks_flow_label_to_20_bits() {
        let f = build_trigger_frame(0xFFF_FFFFF, 1);
        assert_eq!(&f[14..18], &[0x60, 0x0F, 0xFF, 0xFF]);
    }

    #[test]
    fn crc16_ccitt_false_check_value() {
        // The standard check value for CRC-16/CCITT-FALSE over "123456789".
        assert_eq!(crc16_ccitt_false(b"123456789"), 0x29B1);
    }
}
