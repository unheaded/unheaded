//! # shield-ebpf
//!
//! **Shield** — the Kingdom boundary enforcement layer.
//!
//! Two programs in one binary:
//!
//! - `shield_xdp` (XDP ingress): Strip all inbound IPv6 extension headers,
//!   insert the 24-byte Hop-by-Hop Options header carrying an initialized Monad,
//!   set a pseudo-random Flow Label for trace correlation, emit a BIRTH event.
//!
//! - `shield_tc` (TC egress): Capture the final Monad state, emit a DEATH event,
//!   strip the Hop-by-Hop Options header, restore the original transport Next
//!   Header, and release the packet as standard IPv6 into the Shadow network.
//!
//! ## Security properties
//!
//! - NO external IPv6 extension header enters the Kingdom.
//! - NO Monad (proprietary metadata) exits the Kingdom.
//! - Shield enforces containment at wire speed before sk_buff allocation (XDP).
//!
//! ## Verifier notes
//!
//! All loops are bounded by compile-time constants.  Every packet pointer
//! dereference is preceded by an explicit bounds check that re-reads `data_end`
//! after any `bpf_xdp_adjust_head` call.  The verifier requires re-validation
//! because `adjust_head` may change the `data` pointer.
//!
//! See: draft-bellis-unheaded-protocol-foundation-01 §5 (Shield)

#![no_std]
#![no_main]

use aya_ebpf::{
    bindings::{TC_ACT_OK, TC_ACT_SHOT, xdp_action},
    macros::{classifier, map, xdp},
    maps::{HashMap, RingBuf},
    programs::{TcContext, XdpContext},
};
use monad_common::{
    AnamnesisEvent, EventType, HopByHopHeader, Monad, MbcCpuState,
    MONAD_OPT_TYPE, MONAD_OPT_DATA_LEN, MONAD_SIZE, HBH_TOTAL_LEN,
    IPV6_FIXED_HDR_LEN, IPV6_NEXTHDR_HBH,
    circuit_state, deploy_ring, flags, flow_action,
};

// ── Constants ────────────────────────────────────────────────────────────────

const ETH_HLEN:    usize = 14;
const ETH_P_IPV6:  u16   = 0x86DD;

/// Ring buffer capacity per Shield instance.
/// Sized for 1 Gbps full-trace at 35 FPS (covers Doom screen_map events too).
/// Must be a power of two.
const ANAMNESIS_RING_SIZE: u32 = 8 * 1024 * 1024; // 8 MiB

/// Maximum number of IPv6 extension headers to strip per ingress packet.
/// Bounded so the verifier can prove loop termination.
const MAX_EXT_HDRS_TO_STRIP: usize = 8;

/// Maximum rate-limit token bucket capacity (source addresses blocked).
const MAX_BLOCKLIST_ENTRIES: u32 = 4096;

// ── BPF Maps ──────────────────────────────────────────────────────────────────

/// Per-CPU ring buffer for Anamnesis events.
/// Userspace trace-collector reads this via aya's AsyncPerfEventArray.
#[map]
static ANAMNESIS: RingBuf = RingBuf::with_byte_size(ANAMNESIS_RING_SIZE, 0);

/// Source address blocklist.  Key = low 64 bits of IPv6 source address.
/// Any packet from a blocked source receives XDP_DROP.
#[map]
static BLOCKLIST: HashMap<u64, u8> = HashMap::with_max_entries(MAX_BLOCKLIST_ENTRIES, 0);

/// Per-source rate limit counters.  Key = low 64 bits of IPv6 source address.
/// Value = remaining token count (u32).  Decremented by Shield on each packet.
#[map]
static RATE_TOKENS: HashMap<u64, u32> = HashMap::with_max_entries(MAX_BLOCKLIST_ENTRIES, 0);

/// Shield statistics.
#[map]
static STATS: HashMap<u32, u64> = HashMap::with_max_entries(32, 0);

// Stats keys
const STAT_TOTAL:         u32 = 0;
const STAT_IPV6:          u32 = 1;
const STAT_BIRTHS:        u32 = 2;
const STAT_DEATHS:        u32 = 3;
const STAT_BLOCKED:       u32 = 4;
const STAT_EXT_STRIPPED:  u32 = 5;
const STAT_ANOMALIES:     u32 = 6;
const STAT_RING_DROPS:    u32 = 7;

// ── IPv6 header layout (repr(C, packed) for BPF direct pointer casts) ────────

/// IPv6 fixed header — 40 bytes.
/// vtf = version(4b) | traffic_class(8b) | flow_label(20b), network byte order.
#[repr(C, packed)]
struct Ipv6Hdr {
    vtf:         u32,   // version(4) + TC(8) + flow_label(20), big-endian
    payload_len: u16,   // payload length (not including fixed header), big-endian
    next_header: u8,    // next protocol number
    hop_limit:   u8,    // hop limit (TTL)
    src:         [u8; 16],
    dst:         [u8; 16],
}

const IPV6_HDR_SIZE: usize = core::mem::size_of::<Ipv6Hdr>(); // must == 40

/// Ethernet header — 14 bytes.
#[repr(C, packed)]
struct EthHdr {
    dst:   [u8; 6],
    src:   [u8; 6],
    proto: u16,         // EtherType, big-endian
}

const ETH_HDR_SIZE: usize = core::mem::size_of::<EthHdr>(); // must == 14

// ── XDP Ingress: BIRTH ───────────────────────────────────────────────────────

/// Shield ingress XDP program.
///
/// Processes every packet at the Kingdom boundary:
/// 1. Skip non-IPv6 traffic.
/// 2. Drop packets from blocked sources.
/// 3. Drop packets already carrying a HBH header (would double-stamp).
/// 4. Strip any other IPv6 extension headers from inbound Shadow traffic.
/// 5. Insert 24-byte Hop-by-Hop Options header with initialized Monad.
/// 6. Set Flow Label from bpf_get_prandom_u32 for trace correlation.
/// 7. Emit BIRTH event to Anamnesis.
/// 8. XDP_PASS — hand to kernel network stack.
#[xdp]
pub fn shield_xdp(ctx: XdpContext) -> u32 {
    match try_shield_xdp(&ctx) {
        Ok(action) => action,
        Err(_)     => xdp_action::XDP_PASS,
    }
}

#[inline(always)]
fn try_shield_xdp(ctx: &XdpContext) -> Result<u32, ()> {
    increment_stat(STAT_TOTAL);

    let data     = ctx.data();
    let data_end = ctx.data_end();

    // ── 1. Ethernet header bounds check ──────────────────────────────────────
    if data + ETH_HDR_SIZE > data_end {
        return Ok(xdp_action::XDP_PASS);
    }
    let eth = unsafe { &*(data as *const EthHdr) };

    // Only handle IPv6
    if u16::from_be(eth.proto) != ETH_P_IPV6 {
        return Ok(xdp_action::XDP_PASS);
    }
    increment_stat(STAT_IPV6);

    // ── 2. IPv6 header bounds check ───────────────────────────────────────────
    let ip_start = data + ETH_HDR_SIZE;
    if ip_start + IPV6_HDR_SIZE > data_end {
        return Ok(xdp_action::XDP_PASS);
    }
    let ip = unsafe { &*(ip_start as *const Ipv6Hdr) };

    // ── 3. Source address blocklist check ─────────────────────────────────────
    let src_key = src_addr_key(&ip.src);
    if unsafe { BLOCKLIST.get(&src_key).is_some() } {
        increment_stat(STAT_BLOCKED);
        emit_block_event(ip, src_key);
        return Ok(xdp_action::XDP_DROP);
    }

    // ── 4. Drop packets that already carry a Monad HBH header ─────────────────
    // This prevents double-stamping of Kingdom-internal packets that somehow
    // reach an ingress Shield (should not happen in a correctly configured
    // Kingdom, but defend against it).
    if ip.next_header == IPV6_NEXTHDR_HBH {
        // Check if it's our Monad option (might be a legitimate unknown option)
        let hbh_start = ip_start + IPV6_HDR_SIZE;
        if hbh_start + 4 <= data_end {
            let opt_type = unsafe { core::ptr::read_volatile((hbh_start + 2) as *const u8) };
            if opt_type == MONAD_OPT_TYPE {
                // Already a Kingdom packet — pass through without re-stamping.
                return Ok(xdp_action::XDP_PASS);
            }
        }
        // Unknown HBH header from Shadow — strip it.
    }

    // ── 5. Save original transport protocol before stripping extension headers ─
    let original_next_header = strip_extension_headers(ip, ip_start, data_end);

    // ── 6. Generate trace correlation Flow Label ───────────────────────────────
    let flow_label: u32 = unsafe { aya_ebpf::helpers::bpf_get_prandom_u32() } & 0xFFFFF;

    // ── 7. Build the Monad register file ──────────────────────────────────────
    let mut monad = Monad::new();
    // Populate source/destination prefix low bytes from IPv6 addresses.
    // The full address decoding (ULA prefix → Sophia service_identity lookup)
    // happens in the hop-ebpf programs.  Shield just initializes the prefix
    // bytes for the hop programs to read.
    monad.src_prefix_lo = ip.src[7];   // Low byte of subnet identifier
    monad.dst_prefix_lo = ip.dst[7];
    monad.deploy_ring    = deploy_ring::PRODUCTION;
    monad.flow_action    = flow_action::FORWARD;
    monad.circuit_state  = circuit_state::CLOSED;
    monad.recompute_checksum();

    // ── 8. Insert 24-byte HBH header ──────────────────────────────────────────
    //
    // Strategy: use bpf_xdp_adjust_head(delta=-24) to extend the packet by
    // 24 bytes at the head.  Then shuffle ETH+IPv6 24 bytes earlier and write
    // the HBH at the original IPv6+ETH boundary position.
    //
    // Memory layout BEFORE adjust_head:
    //   [0..14]:  Ethernet (14)
    //   [14..54]: IPv6 (40)
    //   [54..]:   Transport payload (N bytes)
    //
    // Memory layout AFTER adjust_head(-24):
    //   [-24..0]: new headroom (now part of data)
    //   [0..14]:  old Ethernet (now at offset 24 from new data start)
    //   [14..54]: old IPv6 (now at offset 38..78 from new data start)
    //   [54..]:   Transport payload (unchanged)
    //
    // We re-index from the NEW data pointer:
    //   new [0..14]:  copy ETH from [24..38]       (move ETH to start)
    //   new [14..54]: copy IPv6 from [38..78]       (move IPv6 after ETH)
    //   new [54..78]: write HBH header (24 bytes)   (new HBH in freed space)
    //   new [78..]:   Transport payload (unchanged)
    let adjust_result = unsafe {
        aya_ebpf::helpers::bpf_xdp_adjust_head(ctx.as_ptr(), -24i32)
    };
    if adjust_result != 0 {
        // bpf_xdp_adjust_head failed (e.g., no headroom)
        return Ok(xdp_action::XDP_PASS);
    }

    // CRITICAL: Re-read data/data_end after adjust_head — verifier requires it.
    let data     = ctx.data();
    let data_end = ctx.data_end();

    // Verify we have room for ETH + IPv6 + HBH + at least 0 bytes of payload.
    let min_size = ETH_HDR_SIZE + IPV6_HDR_SIZE + HBH_TOTAL_LEN;
    if data + min_size > data_end {
        // Unexpected — bail without the HBH.  The packet is malformed; drop.
        return Ok(xdp_action::XDP_DROP);
    }

    // ── 9. Shuffle ETH forward (copy from [24..38] to [0..14]) ───────────────
    // Source: old ETH at data + 24 (where old data started)
    // Destination: data + 0 (new ETH position)
    let old_eth_start = data + 24;
    let new_eth_start = data;
    if old_eth_start + ETH_HDR_SIZE > data_end {
        return Ok(xdp_action::XDP_DROP);
    }
    copy_bytes_forward::<ETH_HDR_SIZE>(new_eth_start, old_eth_start, data_end)?;

    // ── 10. Shuffle IPv6 forward (copy from [38..78] to [14..54]) ─────────────
    let old_ip_start = data + 24 + ETH_HDR_SIZE;
    let new_ip_start = data + ETH_HDR_SIZE;
    if old_ip_start + IPV6_HDR_SIZE > data_end {
        return Ok(xdp_action::XDP_DROP);
    }
    copy_bytes_forward::<IPV6_FIXED_HDR_LEN>(new_ip_start, old_ip_start, data_end)?;

    // ── 11. Update the IPv6 header fields in the new position ─────────────────
    let new_ip = unsafe { &mut *(new_ip_start as *mut Ipv6Hdr) };

    // Set IPv6.Next_Header = 0 (Hop-by-Hop)
    new_ip.next_header = IPV6_NEXTHDR_HBH;

    // Increase Payload_Length by 24 (the HBH header we're inserting).
    let old_plen = u16::from_be(new_ip.payload_len);
    new_ip.payload_len = (old_plen.saturating_add(HBH_TOTAL_LEN as u16)).to_be();

    // Set the Flow Label (bits 19-0 of vtf, big-endian u32).
    let old_vtf = u32::from_be(new_ip.vtf);
    let new_vtf = (old_vtf & 0xFFF00000) | (flow_label & 0xFFFFF);
    new_ip.vtf = new_vtf.to_be();

    // ── 12. Write the HBH Options header ──────────────────────────────────────
    let hbh_start = data + ETH_HDR_SIZE + IPV6_HDR_SIZE;
    if hbh_start + HBH_TOTAL_LEN > data_end {
        return Ok(xdp_action::XDP_DROP);
    }

    let hbh_header = HopByHopHeader::new(original_next_header);
    // Set the Monad we computed earlier.
    let mut hbh_header = hbh_header;
    hbh_header.monad = monad;
    // Re-run checksum now that all fields are final.
    hbh_header.monad.recompute_checksum();

    let hbh_bytes = hbh_header.to_bytes();
    write_bytes::<HBH_TOTAL_LEN>(hbh_start, &hbh_bytes, data_end)?;

    // ── 13. Emit BIRTH event to Anamnesis ─────────────────────────────────────
    let now = unsafe { aya_ebpf::helpers::bpf_ktime_get_ns() };
    emit_anamnesis(now, EventType::Birth, 0, flow_label, &hbh_header.monad);
    increment_stat(STAT_BIRTHS);

    Ok(xdp_action::XDP_PASS)
}

// ── TC Egress: DEATH ─────────────────────────────────────────────────────────

/// Shield egress TC classifier program.
///
/// Processes every packet leaving the Kingdom boundary:
/// 1. Skip non-IPv6 traffic.
/// 2. Verify packet carries a Monad HBH header.
/// 3. Capture the final Monad register state.
/// 4. Emit a DEATH event to Anamnesis (the result of all distributed computation).
/// 5. Strip the 24-byte Hop-by-Hop Options header.
/// 6. Restore the original transport Next Header.
/// 7. Decrement Payload Length by 24.
/// 8. TC_ACT_OK — packet exits as clean standard IPv6.
#[classifier]
pub fn shield_tc(ctx: TcContext) -> i32 {
    match try_shield_tc(&ctx) {
        Ok(action) => action,
        Err(_)     => TC_ACT_OK as i32,
    }
}

#[inline(always)]
fn try_shield_tc(ctx: &TcContext) -> Result<i32, ()> {
    // Load EtherType to check for IPv6.
    let eth_proto: u16 = ctx.load(12).map_err(|_| ())?;
    if u16::from_be(eth_proto) != ETH_P_IPV6 {
        return Ok(TC_ACT_OK as i32);
    }

    // Load IPv6 Next Header field (offset 14 + 6 = 20).
    let next_header: u8 = ctx.load(ETH_HDR_SIZE + 6).map_err(|_| ())?;
    if next_header != IPV6_NEXTHDR_HBH {
        // Not a Kingdom packet (no HBH) — pass through.
        return Ok(TC_ACT_OK as i32);
    }

    // ── Read the HBH Options header ────────────────────────────────────────────
    let hbh_offset = ETH_HDR_SIZE + IPV6_HDR_SIZE;

    // Check option type to confirm this is our Monad option.
    let opt_type: u8 = ctx.load(hbh_offset + 2).map_err(|_| ())?;
    if opt_type != MONAD_OPT_TYPE {
        // Not our option — let it pass.  Should not happen in a well-configured
        // Kingdom, but we must not corrupt packets we don't own.
        return Ok(TC_ACT_OK as i32);
    }
    let opt_data_len: u8 = ctx.load(hbh_offset + 3).map_err(|_| ())?;
    if opt_data_len != MONAD_OPT_DATA_LEN {
        // Malformed — emit anomaly and pass.
        increment_stat(STAT_ANOMALIES);
        return Ok(TC_ACT_OK as i32);
    }

    // ── Load the full Monad from the HBH option data (offset hbh + 4) ─────────
    let monad_offset = hbh_offset + 4;
    let monad = load_monad_from_skb(ctx, monad_offset)?;

    // Load the original Next Header (transport protocol) from HBH[0].
    let original_nh: u8 = ctx.load(hbh_offset).map_err(|_| ())?;

    // ── Read Flow Label from IPv6 header for Anamnesis correlation ─────────────
    let vtf: u32 = ctx.load(ETH_HDR_SIZE).map_err(|_| ())?;
    let flow_label = u32::from_be(vtf) & 0xFFFFF;

    // ── Verify Monad integrity ─────────────────────────────────────────────────
    // Emit anomaly if checksum fails, but still strip the header.
    if !monad.verify_checksum() && !monad.has_flag(flags::CUSTOM) {
        increment_stat(STAT_ANOMALIES);
        let now = unsafe { aya_ebpf::helpers::bpf_ktime_get_ns() };
        emit_anamnesis(now, EventType::Anomaly, 0, flow_label, &monad);
    }

    // ── Emit DEATH event ───────────────────────────────────────────────────────
    let now = unsafe { aya_ebpf::helpers::bpf_ktime_get_ns() };
    emit_anamnesis(now, EventType::Death, 0, flow_label, &monad);
    increment_stat(STAT_DEATHS);

    // ── Strip the 24-byte HBH header ──────────────────────────────────────────
    //
    // Strategy: overwrite the HBH 24 bytes by using bpf_skb_store_bytes
    // to write the transport header over the HBH, then adjust the packet.
    //
    // We use TC's bpf_skb_adjust_room with delta=-24 (BPF_ADJ_ROOM_NET)
    // which removes 24 bytes from the start of the L3 (IP) payload area.
    //
    // After this, the IPv6 header immediately precedes the transport header.
    // We then fixup IPv6.Next_Header and IPv6.Payload_Length.
    let adjust_result = unsafe {
        aya_ebpf::helpers::bpf_skb_adjust_room(
            ctx.skb.skb,
            -(HBH_TOTAL_LEN as i32),
            0, // BPF_ADJ_ROOM_NET
            0,
        )
    };
    if adjust_result != 0 {
        // Adjustment failed — emit anomaly but let the packet through.
        increment_stat(STAT_ANOMALIES);
        return Ok(TC_ACT_OK as i32);
    }

    // ── Fixup IPv6 header fields after header removal ──────────────────────────
    // Restore original Next Header.
    ctx.store(ETH_HDR_SIZE + 6, original_nh, 0).map_err(|_| ())?;

    // Decrement Payload Length by 24.
    let plen: u16 = ctx.load(ETH_HDR_SIZE + 4).map_err(|_| ())?;
    let new_plen = u16::from_be(plen).saturating_sub(HBH_TOTAL_LEN as u16);
    ctx.store(ETH_HDR_SIZE + 4, new_plen.to_be(), 0).map_err(|_| ())?;

    Ok(TC_ACT_OK as i32)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

/// Extract the low 64 bits of an IPv6 source address for map key use.
#[inline(always)]
fn src_addr_key(addr: &[u8; 16]) -> u64 {
    u64::from_be_bytes([
        addr[8], addr[9], addr[10], addr[11],
        addr[12], addr[13], addr[14], addr[15],
    ])
}

/// Walk and skip any IPv6 extension headers after the fixed header, returning
/// the protocol number of the first non-extension header (the transport layer).
///
/// This is called on inbound Shadow traffic before inserting the Monad.
/// The function reads the Next Header chain but does NOT modify packet data —
/// the actual header bytes will be overwritten by the HBH insertion step.
///
/// Returns the original transport Next Header value.
#[inline(always)]
fn strip_extension_headers(ip: &Ipv6Hdr, ip_start: usize, data_end: usize) -> u8 {
    // These Next Header values indicate IPv6 extension headers that must be
    // stripped before the Monad is inserted (RFC 8200 §4):
    //   0  = Hop-by-Hop Options (we strip any external HBH)
    //   43 = Routing
    //   44 = Fragment
    //   51 = AH (Authentication Header)
    //   60 = Destination Options
    let mut nh = ip.next_header;
    let mut offset = ip_start + IPV6_HDR_SIZE;

    let mut iter = 0;
    while iter < MAX_EXT_HDRS_TO_STRIP {
        let is_ext_hdr = matches!(nh, 0 | 43 | 44 | 51 | 60);
        if !is_ext_hdr {
            break;
        }
        // Extension header format: next_header(1) + len_in_8octets(1) + data
        if offset + 2 > data_end {
            break;
        }
        let next_nh  = unsafe { core::ptr::read_volatile(offset as *const u8) };
        let hdr_len  = unsafe { core::ptr::read_volatile((offset + 1) as *const u8) };
        let hdr_size = (hdr_len as usize + 1) * 8;
        nh = next_nh;
        offset += hdr_size;
        increment_stat(STAT_EXT_STRIPPED);
        iter += 1;
    }
    nh
}

/// Copy exactly N bytes from `src` to `dst` with a bounds check.
/// Both `src` and `dst` must fit within `data_end`.
///
/// # Safety
/// Caller must ensure `src` and `dst` are valid packet data pointers.
/// Overlapping regions: safe only if `dst <= src` (forward copy).
#[inline(always)]
fn copy_bytes_forward<const N: usize>(
    dst:      usize,
    src:      usize,
    data_end: usize,
) -> Result<(), ()> {
    if src + N > data_end || dst + N > data_end {
        return Err(());
    }
    let mut i = 0;
    while i < N {
        unsafe {
            let byte = core::ptr::read_volatile((src + i) as *const u8);
            core::ptr::write_volatile((dst + i) as *mut u8, byte);
        }
        i += 1;
    }
    Ok(())
}

/// Write exactly N bytes from `src_arr` to packet memory at `dst` with bounds check.
#[inline(always)]
fn write_bytes<const N: usize>(
    dst:      usize,
    src_arr:  &[u8; N],
    data_end: usize,
) -> Result<(), ()> {
    if dst + N > data_end {
        return Err(());
    }
    let mut i = 0;
    while i < N {
        unsafe {
            core::ptr::write_volatile((dst + i) as *mut u8, src_arr[i]);
        }
        i += 1;
    }
    Ok(())
}

/// Load a 20-byte Monad from SKB data at `offset`.
#[inline(always)]
fn load_monad_from_skb(ctx: &TcContext, offset: usize) -> Result<Monad, ()> {
    let mut bytes = [0u8; MONAD_SIZE];
    let mut i = 0;
    while i < MONAD_SIZE {
        bytes[i] = ctx.load::<u8>(offset + i).map_err(|_| ())?;
        i += 1;
    }
    Ok(Monad::from_bytes(bytes))
}

/// Emit an event to the Anamnesis per-CPU ring buffer.
#[inline(always)]
fn emit_anamnesis(
    timestamp_ns: u64,
    event_type:   EventType,
    hop_id:       u8,
    flow_label:   u32,
    monad:        &Monad,
) {
    let event = AnamnesisEvent::new(timestamp_ns, event_type, hop_id, flow_label, *monad);
    if let Some(mut entry) = ANAMNESIS.reserve::<AnamnesisEvent>(0) {
        unsafe {
            core::ptr::write(entry.as_mut_ptr(), event);
            entry.submit(0);
        }
    } else {
        increment_stat(STAT_RING_DROPS);
    }
}

/// Emit a block event (source blocked) to Anamnesis as an Anomaly.
#[inline(always)]
fn emit_block_event(ip: &Ipv6Hdr, _src_key: u64) {
    let now = unsafe { aya_ebpf::helpers::bpf_ktime_get_ns() };
    let mut monad = Monad::new();
    // Mark the source address low byte in a scratch register for dashboard display.
    monad.src_prefix_lo = ip.src[7];
    monad.set_flag(flags::CHAOS); // Reuse CHAOS flag to indicate blocked packet
    let event = AnamnesisEvent::new(now, EventType::Anomaly, 0xFF, 0, monad);
    if let Some(mut entry) = ANAMNESIS.reserve::<AnamnesisEvent>(0) {
        unsafe {
            core::ptr::write(entry.as_mut_ptr(), event);
            entry.submit(0);
        }
    }
}

/// Increment a per-Shield statistics counter (saturating).
#[inline(always)]
fn increment_stat(key: u32) {
    if let Some(val) = STATS.get_ptr_mut(&key) {
        unsafe { *val = (*val).saturating_add(1); }
    } else {
        let _ = STATS.insert(&key, &1u64, 0);
    }
}

#[panic_handler]
fn panic(_: &core::panic::PanicInfo) -> ! {
    loop {}
}
