// SPDX-License-Identifier: GPL-2.0-only
// Unheaded Kingdom — XDP Redirect Program
//
// Steers inbound packets to AF_XDP sockets via XSKMAP for zero-copy
// userspace packet I/O.  Parses Ethernet + IPv4 headers, checks per-queue
// CONFIG map, and redirects to the XSK socket bound to the RX queue.
// Packets that don't match or have no bound socket fall through to XDP_PASS.
//
// Maps:
//   XSKS   — BPF_MAP_TYPE_XSKMAP, key=queue_id, value=socket_fd
//   CONFIG — BPF_MAP_TYPE_HASH,    key=queue_id, value=XdpRedirectConfig
//   STATS  — BPF_MAP_TYPE_HASH,    key=queue_id, value=XdpRedirectStats

#![no_std]
#![no_main]

use af_xdp_common::{XdpRedirectConfig, XdpRedirectStats};
use aya_ebpf::{
    bindings::xdp_action,
    macros::{map, xdp},
    maps::{HashMap, XskMap},
    programs::XdpContext,
};

// ── Maps ─────────────────────────────────────────────────────────────────────

/// XSKMAP — maps queue IDs to AF_XDP socket file descriptors.
/// Userspace writes: XSKS[queue_id] = socket_fd
/// Kernel reads:     bpf_redirect_map(&XSKS, queue_id, flags)
#[map]
static XSKS: XskMap = XskMap::with_max_entries(64, 0);

/// CONFIG — per-queue redirect configuration.
/// Userspace writes to enable/disable redirect and set protocol filters.
/// Key: queue_id (u32), Value: XdpRedirectConfig
#[map]
static CONFIG: HashMap<u32, XdpRedirectConfig> = HashMap::with_max_entries(256, 0);

/// STATS — per-queue packet counters.
/// Updated by eBPF, read by userspace for monitoring.
/// Key: queue_id (u32), Value: XdpRedirectStats
#[map]
static STATS: HashMap<u32, XdpRedirectStats> = HashMap::with_max_entries(256, 0);

// ── Constants ────────────────────────────────────────────────────────────────

const ETH_HLEN: usize = 14;
const ETH_P_IPV4: u16 = 0x0800_u16.to_be();
const IP_HDR_MIN: usize = 20;

// ── Header types ─────────────────────────────────────────────────────────────

#[repr(C, packed)]
struct EthHdr {
    dst_mac: [u8; 6],
    src_mac: [u8; 6],
    proto: u16, // network byte order
}

#[repr(C, packed)]
struct IpHdr {
    ver_ihl: u8,
    tos: u8,
    tot_len: u16,
    id: u16,
    frag_off: u16,
    ttl: u8,
    protocol: u8,
    checksum: u16,
    src_ip: u32,
    dst_ip: u32,
}

// ── XDP entry point ──────────────────────────────────────────────────────────

#[xdp]
pub fn xdp_redirect(ctx: XdpContext) -> u32 {
    match try_redirect(&ctx) {
        Ok(action) => action,
        Err(_) => xdp_action::XDP_PASS,
    }
}

#[inline(always)]
fn try_redirect(ctx: &XdpContext) -> Result<u32, u64> {
    let data = ctx.data();
    let data_end = ctx.data_end();

    // ── Parse Ethernet ───────────────────────────────────────────────────
    if data + ETH_HLEN > data_end {
        return Ok(xdp_action::XDP_PASS);
    }
    let eth = unsafe { &*(data as *const EthHdr) };

    // Only redirect IPv4 (extension to IPv6 deferred to shield integration)
    if eth.proto != ETH_P_IPV4 {
        return Ok(xdp_action::XDP_PASS);
    }

    // ── Parse IPv4 ───────────────────────────────────────────────────────
    let ip_start = data + ETH_HLEN;
    if ip_start + IP_HDR_MIN > data_end {
        return Ok(xdp_action::XDP_PASS);
    }
    let _ip = unsafe { &*(ip_start as *const IpHdr) };

    // ── Per-queue config check ───────────────────────────────────────────
    let queue_id = unsafe { (*ctx.ctx).rx_queue_index };

    // Check if redirect is enabled for this queue
    let enabled = if let Some(cfg) = unsafe { CONFIG.get(&queue_id) } {
        cfg.enabled != 0
    } else {
        false // Default: disabled until userspace enables
    };

    if !enabled {
        // Update passed counter
        if let Some(st) = STATS.get_ptr_mut(&queue_id) {
            unsafe { (*st).passed += 1 };
        }
        return Ok(xdp_action::XDP_PASS);
    }

    // ── Redirect to AF_XDP socket ────────────────────────────────────────
    match XSKS.redirect(queue_id, 0) {
        Ok(action) => {
            // Increment redirected counter
            if let Some(st) = STATS.get_ptr_mut(&queue_id) {
                unsafe { (*st).redirected += 1 };
            }
            Ok(action)
        }
        Err(_) => {
            // No XSK socket bound to this queue — pass to kernel stack
            if let Some(st) = STATS.get_ptr_mut(&queue_id) {
                unsafe { (*st).passed += 1 };
            }
            Ok(xdp_action::XDP_PASS)
        }
    }
}

// ── Panic handler (required for no_std eBPF) ─────────────────────────────────

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    unsafe { core::hint::unreachable_unchecked() }
}
