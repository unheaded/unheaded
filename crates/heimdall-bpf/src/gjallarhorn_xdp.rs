// SPDX-License-Identifier: GPL-3.0-or-later
//! Heimdall Gjallarhorn XDP observer (ADR-043, S5 Phase 5).
//!
//! Sees Gjallarhorn trigger packets on the wire and reports each one on a
//! ringbuf; heimdall-daemon reads the ringbuf and decides what to do.
//!
//! Observe-only: every packet gets XDP_PASS (S5 Step 138). The userspace UDP
//! path keeps working, a parser bug here can never black-hole the link, and
//! ADR-043 hard condition #1 (alerts only) leaves XDP nothing to act on.
//!
//! Parse path (v1): Ethernet -> IPv6 (fixed header only; no extension-header
//! walk, so a horn behind a Hop-by-Hop or other extension header is not seen)
//! -> UDP dport 16901 -> payload of exactly 20 bytes -> magic "GJLR" ->
//! kind 1 or 2 -> reserved bytes 17..20 zero.
//!
//! Wire layout of the 20-byte payload (pkg/gjallarhorn, big-endian):
//!   [0:4] "GJLR" | [4] kind | [5:9] cluster | [9:17] manifest_ptr | [17:20] 0
//!
//! Verifier rules followed: every packet read is bounds-checked against
//! data_end first; ringbuf records are written field by field (no zero-init
//! of a padded struct, which `--btf` turns into a rejected memset call); no
//! loops.
#![no_std]
#![no_main]

use aya_ebpf::{
    bindings::xdp_action,
    helpers::bpf_ktime_get_ns,
    macros::{map, xdp},
    maps::{PerCpuArray, RingBuf},
    programs::XdpContext,
};

const ETH_HLEN: usize = 14;
const ETH_P_IPV6: u16 = 0x86DD;
const IPV6_HLEN: usize = 40;
const IPPROTO_UDP: u8 = 17;
const UDP_HLEN: usize = 8;
const GJALLARHORN_PORT: u16 = 16901;
const HORN_LEN: usize = 20;

/// Stats slots in `HORN_STATS` (per-CPU u64 counters).
const STAT_SEEN: u32 = 0; // UDP to port 16901
const STAT_VALID: u32 = 1; // passed every check, record submitted
const STAT_BAD_MAGIC: u32 = 2;
const STAT_BAD_KIND: u32 = 3;
const STAT_BAD_LEN: u32 = 4; // UDP payload not exactly 20 bytes
const STAT_BAD_RESERVED: u32 = 5;
const STAT_RB_FULL: u32 = 6; // valid horn, ringbuf had no room
const STAT_SLOTS: u32 = 8;

/// One ringbuf record per valid horn. 32 bytes, no implicit padding:
/// every byte is a named field so the record is fully written.
/// Integers are host order (little-endian on bpfel), decoded from the
/// big-endian wire.
#[repr(C)]
pub struct HornEvent {
    pub kind: u8,
    pub pad0: u8,
    pub pad1: u16,
    pub cluster: u32,
    pub manifest_ptr: u64,
    /// XOR of the four 32-bit words of the IPv6 source address.
    pub src_hash: u32,
    pub pad2: u32,
    pub ts_ns: u64,
}

#[map]
static HORN_EVENTS: RingBuf = RingBuf::with_byte_size(64 * 1024, 0);

#[map]
static HORN_STATS: PerCpuArray<u64> = PerCpuArray::with_max_entries(STAT_SLOTS, 0);

#[xdp]
pub fn heimdall_gjallarhorn(ctx: XdpContext) -> u32 {
    observe(&ctx);
    xdp_action::XDP_PASS
}

#[inline(always)]
fn bump(slot: u32) {
    if let Some(v) = HORN_STATS.get_ptr_mut(slot) {
        unsafe { *v += 1 };
    }
}

#[inline(always)]
fn observe(ctx: &XdpContext) {
    let data = ctx.data();
    let end = ctx.data_end();

    // Ethernet + IPv6 fixed header + UDP header must all be present.
    if data + ETH_HLEN + IPV6_HLEN + UDP_HLEN > end {
        return;
    }
    let eth_proto = unsafe { u16::from_be(*((data + 12) as *const u16)) };
    if eth_proto != ETH_P_IPV6 {
        return;
    }
    let ip = data + ETH_HLEN;
    let next_header = unsafe { *((ip + 6) as *const u8) };
    if next_header != IPPROTO_UDP {
        return;
    }
    let udp = ip + IPV6_HLEN;
    let dport = unsafe { u16::from_be(*((udp + 2) as *const u16)) };
    if dport != GJALLARHORN_PORT {
        return;
    }
    bump(STAT_SEEN);

    let udp_len = unsafe { u16::from_be(*((udp + 4) as *const u16)) } as usize;
    let payload = udp + UDP_HLEN;
    if udp_len != UDP_HLEN + HORN_LEN || payload + HORN_LEN > end {
        bump(STAT_BAD_LEN);
        return;
    }
    // Every read below is inside [payload, payload + 20), checked above.
    let b = |o: usize| unsafe { *((payload + o) as *const u8) };

    // Magic "GJLR".
    if b(0) != b'G' || b(1) != b'J' || b(2) != b'L' || b(3) != b'R' {
        bump(STAT_BAD_MAGIC);
        return;
    }
    let kind = b(4);
    if kind != 1 && kind != 2 {
        bump(STAT_BAD_KIND);
        return;
    }
    let reserved = b(17) | b(18) | b(19);
    if reserved != 0 {
        bump(STAT_BAD_RESERVED);
        return;
    }

    // Big-endian fields, assembled from byte loads (no loops, no alignment
    // assumption).
    let cluster: u32 = (b(5) as u32) << 24 | (b(6) as u32) << 16 | (b(7) as u32) << 8 | b(8) as u32;
    let manifest_ptr: u64 = (b(9) as u64) << 56
        | (b(10) as u64) << 48
        | (b(11) as u64) << 40
        | (b(12) as u64) << 32
        | (b(13) as u64) << 24
        | (b(14) as u64) << 16
        | (b(15) as u64) << 8
        | b(16) as u64;
    // IPv6 source address is bytes 8..24 of the IPv6 header.
    let src = ip + 8;
    // Byte loads only: read_unaligned lowers to a memcpy call, which the
    // verifier rejects as a global function.
    let w = |o: usize| unsafe {
        (*((src + o) as *const u8) as u32)
            | (*((src + o + 1) as *const u8) as u32) << 8
            | (*((src + o + 2) as *const u8) as u32) << 16
            | (*((src + o + 3) as *const u8) as u32) << 24
    };
    let src_hash = w(0) ^ w(4) ^ w(8) ^ w(12);

    match HORN_EVENTS.reserve::<HornEvent>(0) {
        Some(mut entry) => {
            let ev = entry.as_mut_ptr();
            unsafe {
                (*ev).kind = kind;
                (*ev).pad0 = 0;
                (*ev).pad1 = 0;
                (*ev).cluster = cluster;
                (*ev).manifest_ptr = manifest_ptr;
                (*ev).src_hash = src_hash;
                (*ev).pad2 = 0;
                (*ev).ts_ns = bpf_ktime_get_ns();
            }
            entry.submit(0);
            bump(STAT_VALID);
        }
        None => bump(STAT_RB_FULL),
    }
}

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    loop {}
}
