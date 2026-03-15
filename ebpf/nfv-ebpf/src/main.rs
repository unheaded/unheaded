// SPDX-License-Identifier: GPL-2.0-only
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//! Unheaded Protocol — NFV eBPF Program (App 10: Network Function Chaining)
//!
//! This XDP program implements a tail-call-based composable network function
//! pipeline.  Each packet carries a chain_id in its Monad `circuit_state` byte
//! (bits 5-0) and an auth level (bits 7-6).  The main entry point sets up an
//! NfvContext, then tail-calls into the first function of the chain.
//!
//! Sample network functions (each a separate `#[xdp]` entry point):
//!   - `nfv_firewall`:     deny-list lookup, drop if found
//!   - `nfv_nat`:          IPv6 address rewrite from NAT_MAP
//!   - `nfv_rate_limiter`: token bucket rate limiting
//!   - `nfv_lb_route`:     select backend, rewrite L2 for redirect
//!
//! Each function performs its work, updates the NfvContext verdict, and
//! tail-calls to the next function via NFV_PROG_ARRAY.  The final function
//! applies the aggregate verdict (XDP_PASS / XDP_DROP / XDP_TX).
//!
//! # BPF Verifier Notes
//!
//! All packet reads use `read_volatile` with explicit bounds checks.  Every loop
//! is bounded by a compile-time constant.  `core::hint::black_box` prevents LLVM
//! from eliminating the per-iteration bounds checks the verifier requires.

#![no_std]
#![no_main]

use aya_ebpf::{
    bindings::xdp_action,
    helpers::bpf_ktime_get_ns,
    macros::{map, xdp},
    maps::{HashMap, PerCpuArray, ProgramArray, RingBuf},
    programs::XdpContext,
};
use monad_common::{
    flags, HBH_TOTAL_LEN, IPV6_FIXED_HDR_LEN, IPV6_NEXTHDR_HBH, MONAD_OPT_DATA_LEN,
    MONAD_OPT_TYPE, MONAD_SIZE,
};

// ── BPF Maps ─────────────────────────────────────────────────────────────────

/// Chain definitions: chain_id -> ChainDef.
/// Each chain defines up to 10 functions to execute in sequence.
#[map]
static CHAIN_DEFS: HashMap<u32, [u8; 44]> = HashMap::with_max_entries(64, 0);
// ChainDef layout (44 bytes):
//   [0..4]:   function_count (u32)
//   [4..44]:  functions[10] (10 x u32, indices into NFV_PROG_ARRAY)

/// Program array for tail-call dispatch into individual network functions.
#[map]
static NFV_PROG_ARRAY: ProgramArray = ProgramArray::with_max_entries(32, 0);

/// Per-CPU NFV context shared across tail-called functions.
/// Index 0 holds the current NfvContext for this packet.
#[map]
static NFV_CONTEXT: PerCpuArray<[u8; 24]> = PerCpuArray::with_max_entries(1, 0);
// NfvContext layout (24 bytes):
//   [0..4]:   chain_id (u32)
//   [4..8]:   current_func (u32, index into chain's function list)
//   [8..12]:  packet_verdict (u32, accumulated XDP verdict)
//   [12..20]: latency_ns (u64, start timestamp for latency measurement)
//   [20..24]: packets_processed (u32)

/// IPv6 address deny list for nfv_firewall function.
#[map]
static DENY_LIST: HashMap<[u8; 16], u8> = HashMap::with_max_entries(10_000, 0);

/// NAT mapping table: nat_key (src_addr hash) -> nat_value (rewritten addr).
#[map]
static NAT_MAP: HashMap<[u8; 16], [u8; 16]> = HashMap::with_max_entries(10_000, 0);

/// Rate limiter state: flow_key -> TokenBucket.
#[map]
static RATE_STATE: HashMap<u32, [u8; 24]> = HashMap::with_max_entries(10_000, 0);
// TokenBucket layout (24 bytes):
//   [0..8]:   tokens (u64, current token count)
//   [8..16]:  last_refill_ns (u64, timestamp of last refill)
//   [16..20]: rate (u32, tokens per second)
//   [20..24]: burst (u32, max bucket size)

/// Backend list for load balancing: svc_id -> BackendList.
#[map]
static BACKENDS: HashMap<u32, [u8; 64]> = HashMap::with_max_entries(256, 0);
// BackendList layout (64 bytes):
//   [0..4]:   count (u32, number of active backends)
//   [4..64]:  backends[10] (10 x 6 bytes = 60: MAC address of each backend)

/// Runtime configuration — tunable from userspace without reloading.
///
/// | Key | Meaning                          | Default |
/// |-----|----------------------------------|---------|
/// | 0   | node_id (this node's identifier) | 0       |
/// | 1   | default_chain_id                 | 0       |
/// | 2   | rate_limit_tokens_per_sec        | 1000    |
/// | 3   | rate_limit_burst                 | 100     |
#[map]
static CONFIG: HashMap<u32, u64> = HashMap::with_max_entries(16, 0);

/// Statistics counters — observable from userspace.
#[map]
static STATS: HashMap<u32, u64> = HashMap::with_max_entries(32, 0);

/// Anamnesis ring buffer for NFV events (24-byte events).
#[map]
static ANAMNESIS: RingBuf = RingBuf::with_byte_size(4 * 1024 * 1024, 0);

// ── Stats keys ────────────────────────────────────────────────────────────────
const STAT_PACKETS_TOTAL: u32 = 0;
const STAT_CHAINS_EXECUTED: u32 = 1;
const STAT_FUNCTIONS_EXECUTED: u32 = 2;
const STAT_DROPS: u32 = 3;
const STAT_REDIRECTS: u32 = 4;
const STAT_TAIL_CALL_ERRORS: u32 = 5;
const STAT_EVENTS_SENT: u32 = 6;
const STAT_EVENTS_DROPPED: u32 = 7;
const STAT_FIREWALL_DENIED: u32 = 8;
const STAT_NAT_REWRITES: u32 = 9;
const STAT_RATE_LIMITED: u32 = 10;
const STAT_LB_ROUTED: u32 = 11;

// ── Config keys ───────────────────────────────────────────────────────────────
const CFG_NODE_ID: u32 = 0;
const CFG_DEFAULT_CHAIN_ID: u32 = 1;
const CFG_RATE_TOKENS_PER_SEC: u32 = 2;
const CFG_RATE_BURST: u32 = 3;

// ── NFV event type constants ──────────────────────────────────────────────────
const NFV_EVENT_CHAIN_START: u8 = 0x01;
const NFV_EVENT_CHAIN_END: u8 = 0x02;
const NFV_EVENT_FUNC_EXEC: u8 = 0x03;
const NFV_EVENT_DROP: u8 = 0x04;
const NFV_EVENT_REDIRECT: u8 = 0x05;

// ── Wire-format helpers ─────────────────────────────────────────────────────

/// Ethernet header (14 bytes, packed).
#[repr(C, packed)]
struct EthHdr {
    dst: [u8; 6],
    src: [u8; 6],
    proto: u16,
}

/// IPv6 fixed header (40 bytes, packed).
#[repr(C, packed)]
struct Ipv6Hdr {
    vtf: u32,
    payload_len: u16,
    next_header: u8,
    hop_limit: u8,
    src: [u8; 16],
    dst: [u8; 16],
}

const ETH_HLEN: usize = 14;
const ETH_P_IPV6: u16 = 0x86DD;

// ── Main XDP entry point ─────────────────────────────────────────────────────

#[xdp]
pub fn nfv_xdp(ctx: XdpContext) -> u32 {
    match try_nfv_xdp(&ctx) {
        Ok(action) => action,
        Err(_) => xdp_action::XDP_PASS, // fail-open: never silently drop
    }
}

#[inline(always)]
fn try_nfv_xdp(ctx: &XdpContext) -> Result<u32, ()> {
    increment_stat(STAT_PACKETS_TOTAL);

    let data = ctx.data();
    let data_end = ctx.data_end();

    // ── Ethernet ──────────────────────────────────────────────────────────────
    if data + ETH_HLEN > data_end {
        return Ok(xdp_action::XDP_PASS);
    }
    let eth = unsafe { &*(data as *const EthHdr) };
    if u16::from_be(eth.proto) != ETH_P_IPV6 {
        return Ok(xdp_action::XDP_PASS);
    }

    // ── IPv6 Fixed Header ─────────────────────────────────────────────────────
    let ip_start = data + ETH_HLEN;
    if ip_start + IPV6_FIXED_HDR_LEN > data_end {
        return Ok(xdp_action::XDP_PASS);
    }
    let ip = unsafe { &*(ip_start as *const Ipv6Hdr) };

    if ip.next_header != IPV6_NEXTHDR_HBH {
        return Ok(xdp_action::XDP_PASS);
    }

    // ── Hop-by-Hop Header ─────────────────────────────────────────────────────
    let hbh_start = ip_start + IPV6_FIXED_HDR_LEN;

    if hbh_start + HBH_TOTAL_LEN > data_end {
        return Ok(xdp_action::XDP_PASS);
    }

    let hbh_ext_len = unsafe { core::ptr::read_volatile((hbh_start + 1) as *const u8) };
    let hbh_total = (hbh_ext_len as usize + 1) * 8;

    if hbh_start + hbh_total > data_end {
        return Ok(xdp_action::XDP_PASS);
    }

    let opts_start = hbh_start + 2;
    let opts_end = hbh_start + hbh_total;

    // ── Find Monad option ─────────────────────────────────────────────────────
    let monad_opt_off = find_monad_option(opts_start, opts_end, data_end)?;

    let monad_data_off = monad_opt_off + 2;
    if monad_data_off + MONAD_SIZE > data_end {
        return Ok(xdp_action::XDP_PASS);
    }

    // ── Read Monad ────────────────────────────────────────────────────────────
    let monad = read_monad_from_pkt(monad_data_off, data_end)?;

    // ── Extract chain_id and auth level from circuit_state ────────────────────
    // bits 5-0: chain_id (0-63)
    // bits 7-6: auth level (0-3, higher = more privileged)
    let chain_id = (monad.circuit_state & 0x3F) as u32;
    let _auth_level = (monad.circuit_state >> 6) & 0x03;

    // ── Look up chain definition ──────────────────────────────────────────────
    let chain_key = if chain_id == 0 {
        cfg(CFG_DEFAULT_CHAIN_ID) as u32
    } else {
        chain_id
    };

    let chain_def = match unsafe { CHAIN_DEFS.get(&chain_key) } {
        Some(cd) => cd,
        None => return Ok(xdp_action::XDP_PASS), // no chain defined, pass through
    };

    // Read function_count from ChainDef
    let function_count =
        unsafe { core::ptr::read_volatile(chain_def.as_ptr() as *const u32) } as u32;

    if function_count == 0 || function_count > 10 {
        return Ok(xdp_action::XDP_PASS);
    }

    // ── Set up NfvContext ─────────────────────────────────────────────────────
    let now = unsafe { bpf_ktime_get_ns() };
    let ctx_idx: u32 = 0;

    if let Some(nfv_ctx) = NFV_CONTEXT.get_ptr_mut(ctx_idx) {
        let ptr = nfv_ctx as *mut u8;
        unsafe {
            // chain_id
            core::ptr::write_volatile(ptr as *mut u32, chain_key);
            // current_func = 0
            core::ptr::write_volatile(ptr.add(4) as *mut u32, 0u32);
            // packet_verdict = XDP_PASS
            core::ptr::write_volatile(ptr.add(8) as *mut u32, xdp_action::XDP_PASS);
            // latency_ns = now
            core::ptr::write_volatile(ptr.add(12) as *mut u64, now);
            // packets_processed = 0
            core::ptr::write_volatile(ptr.add(20) as *mut u32, 0u32);
        }
    } else {
        return Ok(xdp_action::XDP_PASS);
    }

    increment_stat(STAT_CHAINS_EXECUTED);

    // Emit chain start event
    emit_nfv_event(NFV_EVENT_CHAIN_START, chain_key, 0, xdp_action::XDP_PASS, 0, 0);

    // ── Read first function index from chain and tail-call ────────────────────
    let first_func_idx = unsafe {
        core::ptr::read_volatile(chain_def.as_ptr().add(4) as *const u32)
    };

    // Tail-call into the first function
    unsafe {
        NFV_PROG_ARRAY.tail_call(ctx, first_func_idx).ok();
    }

    // If tail-call fails, we reach here
    increment_stat(STAT_TAIL_CALL_ERRORS);
    Ok(xdp_action::XDP_PASS)
}

// ── Sample NFV Functions ──────────────────────────────────────────────────────

/// NFV Firewall: deny-list lookup, drop if source IPv6 address is found.
#[xdp]
pub fn nfv_firewall(ctx: XdpContext) -> u32 {
    match try_nfv_firewall(&ctx) {
        Ok(action) => action,
        Err(_) => xdp_action::XDP_PASS,
    }
}

#[inline(always)]
fn try_nfv_firewall(ctx: &XdpContext) -> Result<u32, ()> {
    increment_stat(STAT_FUNCTIONS_EXECUTED);

    let data = ctx.data();
    let data_end = ctx.data_end();

    // ── Parse to IPv6 source address ──────────────────────────────────────────
    if data + ETH_HLEN + IPV6_FIXED_HDR_LEN > data_end {
        return advance_chain(ctx);
    }

    let ip = unsafe { &*((data + ETH_HLEN) as *const Ipv6Hdr) };

    // Read source IPv6 address
    let mut src_addr = [0u8; 16];
    for i in 0..16usize {
        src_addr[i] = unsafe {
            core::ptr::read_volatile(
                core::hint::black_box((data + ETH_HLEN + 8 + i) as *const u8),
            )
        };
    }

    // ── Deny-list lookup ──────────────────────────────────────────────────────
    if unsafe { DENY_LIST.get(&src_addr) }.is_some() {
        increment_stat(STAT_FIREWALL_DENIED);
        increment_stat(STAT_DROPS);

        // Update verdict in NfvContext
        set_verdict(xdp_action::XDP_DROP);
        emit_nfv_event(NFV_EVENT_DROP, 0, 0, xdp_action::XDP_DROP, 0, 0);

        // Skip remaining chain — apply verdict immediately
        return apply_final_verdict();
    }

    let _ = ip;
    advance_chain(ctx)
}

/// NFV NAT: IPv6 address rewrite from NAT_MAP.
#[xdp]
pub fn nfv_nat(ctx: XdpContext) -> u32 {
    match try_nfv_nat(&ctx) {
        Ok(action) => action,
        Err(_) => xdp_action::XDP_PASS,
    }
}

#[inline(always)]
fn try_nfv_nat(ctx: &XdpContext) -> Result<u32, ()> {
    increment_stat(STAT_FUNCTIONS_EXECUTED);

    let data = ctx.data();
    let data_end = ctx.data_end();

    if data + ETH_HLEN + IPV6_FIXED_HDR_LEN > data_end {
        return advance_chain(ctx);
    }

    // Read source IPv6 address for NAT lookup
    let mut src_addr = [0u8; 16];
    for i in 0..16usize {
        src_addr[i] = unsafe {
            core::ptr::read_volatile(
                core::hint::black_box((data + ETH_HLEN + 8 + i) as *const u8),
            )
        };
    }

    // ── NAT lookup and rewrite ────────────────────────────────────────────────
    if let Some(new_addr) = unsafe { NAT_MAP.get(&src_addr) } {
        // Rewrite source IPv6 address in-place
        let addr_offset = data + ETH_HLEN + 8; // offset of src addr in IPv6 header
        if addr_offset + 16 <= data_end {
            for i in 0..16usize {
                unsafe {
                    core::ptr::write_volatile(
                        core::hint::black_box((addr_offset + i) as *mut u8),
                        new_addr[i],
                    );
                }
            }
            increment_stat(STAT_NAT_REWRITES);
        }
    }

    advance_chain(ctx)
}

/// NFV Rate Limiter: token bucket rate limiting per flow.
#[xdp]
pub fn nfv_rate_limiter(ctx: XdpContext) -> u32 {
    match try_nfv_rate_limiter(&ctx) {
        Ok(action) => action,
        Err(_) => xdp_action::XDP_PASS,
    }
}

#[inline(always)]
fn try_nfv_rate_limiter(ctx: &XdpContext) -> Result<u32, ()> {
    increment_stat(STAT_FUNCTIONS_EXECUTED);

    let data = ctx.data();
    let data_end = ctx.data_end();

    if data + ETH_HLEN + IPV6_FIXED_HDR_LEN > data_end {
        return advance_chain(ctx);
    }

    let ip = unsafe { &*((data + ETH_HLEN) as *const Ipv6Hdr) };

    // Use flow label as flow key for rate limiting
    let vtf = u32::from_be(unsafe { core::ptr::read_unaligned(core::ptr::addr_of!(ip.vtf)) });
    let flow_key = vtf & 0x000F_FFFF;

    let now = unsafe { bpf_ktime_get_ns() };

    // ── Token bucket check ────────────────────────────────────────────────────
    if let Some(bucket) = RATE_STATE.get_ptr_mut(&flow_key) {
        let ptr = bucket as *mut u8;
        let tokens = unsafe { core::ptr::read_volatile(ptr as *const u64) };
        let last_refill = unsafe { core::ptr::read_volatile(ptr.add(8) as *const u64) };
        let rate = unsafe { core::ptr::read_volatile(ptr.add(16) as *const u32) } as u64;
        let burst = unsafe { core::ptr::read_volatile(ptr.add(20) as *const u32) } as u64;

        // Calculate tokens to add since last refill
        let elapsed_ns = now.saturating_sub(last_refill);
        let new_tokens_raw = (elapsed_ns.saturating_mul(rate)) / 1_000_000_000;
        let new_tokens = tokens.saturating_add(new_tokens_raw);
        let capped_tokens = if new_tokens > burst { burst } else { new_tokens };

        if capped_tokens == 0 {
            // Rate limited — drop
            increment_stat(STAT_RATE_LIMITED);
            increment_stat(STAT_DROPS);
            set_verdict(xdp_action::XDP_DROP);
            return apply_final_verdict();
        }

        // Consume one token
        unsafe {
            core::ptr::write_volatile(ptr as *mut u64, capped_tokens - 1);
            core::ptr::write_volatile(ptr.add(8) as *mut u64, now);
        }
    } else {
        // First packet for this flow — initialize bucket
        let rate = cfg(CFG_RATE_TOKENS_PER_SEC) as u32;
        let burst = cfg(CFG_RATE_BURST) as u32;
        let rate = if rate == 0 { 1000 } else { rate };
        let burst = if burst == 0 { 100 } else { burst };

        let mut bucket = [0u8; 24];
        // Initial tokens = burst - 1 (consumed one)
        let init_tokens = (burst - 1) as u64;
        unsafe {
            core::ptr::write_volatile(bucket.as_mut_ptr() as *mut u64, init_tokens);
            core::ptr::write_volatile(bucket.as_mut_ptr().add(8) as *mut u64, now);
            core::ptr::write_volatile(bucket.as_mut_ptr().add(16) as *mut u32, rate);
            core::ptr::write_volatile(bucket.as_mut_ptr().add(20) as *mut u32, burst);
        }
        let _ = RATE_STATE.insert(&flow_key, &bucket, 0);
    }

    advance_chain(ctx)
}

/// NFV Load Balancer / Route: select backend and rewrite L2 for redirect.
#[xdp]
pub fn nfv_lb_route(ctx: XdpContext) -> u32 {
    match try_nfv_lb_route(&ctx) {
        Ok(action) => action,
        Err(_) => xdp_action::XDP_PASS,
    }
}

#[inline(always)]
fn try_nfv_lb_route(ctx: &XdpContext) -> Result<u32, ()> {
    increment_stat(STAT_FUNCTIONS_EXECUTED);

    let data = ctx.data();
    let data_end = ctx.data_end();

    if data + ETH_HLEN + IPV6_FIXED_HDR_LEN > data_end {
        return advance_chain(ctx);
    }

    // Find Monad to get service ID for backend selection
    let hbh_start = data + ETH_HLEN + IPV6_FIXED_HDR_LEN;
    if hbh_start + HBH_TOTAL_LEN > data_end {
        return advance_chain(ctx);
    }

    let hbh_ext_len = unsafe { core::ptr::read_volatile((hbh_start + 1) as *const u8) };
    let hbh_total = (hbh_ext_len as usize + 1) * 8;

    if hbh_start + hbh_total > data_end {
        return advance_chain(ctx);
    }

    let opts_start = hbh_start + 2;
    let opts_end = hbh_start + hbh_total;

    let monad_opt_off = match find_monad_option(opts_start, opts_end, data_end) {
        Ok(off) => off,
        Err(_) => return advance_chain(ctx),
    };

    let monad_data_off = monad_opt_off + 2;
    if monad_data_off + MONAD_SIZE > data_end {
        return advance_chain(ctx);
    }

    let monad = read_monad_from_pkt(monad_data_off, data_end)?;

    // Use dst_service_id to look up backends
    let svc_id = monad.dst_service_id as u32;

    if let Some(backend_list) = unsafe { BACKENDS.get(&svc_id) } {
        let count = unsafe { core::ptr::read_volatile(backend_list.as_ptr() as *const u32) };
        if count > 0 && count <= 10 {
            // Simple round-robin: use flow label to select backend
            let ip = unsafe { &*((data + ETH_HLEN) as *const Ipv6Hdr) };
            let vtf = u32::from_be(
                unsafe { core::ptr::read_unaligned(core::ptr::addr_of!(ip.vtf)) },
            );
            let flow_label = vtf & 0x000F_FFFF;
            let backend_idx = (flow_label % count) as usize;

            // Rewrite destination MAC address to selected backend
            let mac_offset = 4 + backend_idx * 6; // offset into BackendList
            if mac_offset + 6 <= 64 && data + 6 <= data_end {
                for i in 0..6usize {
                    let mac_byte = unsafe {
                        core::ptr::read_volatile(
                            core::hint::black_box(backend_list.as_ptr().add(mac_offset + i)),
                        )
                    };
                    unsafe {
                        core::ptr::write_volatile(
                            core::hint::black_box((data + i) as *mut u8),
                            mac_byte,
                        );
                    }
                }

                increment_stat(STAT_LB_ROUTED);
                increment_stat(STAT_REDIRECTS);
                set_verdict(xdp_action::XDP_TX);
                emit_nfv_event(NFV_EVENT_REDIRECT, svc_id, backend_idx as u32, xdp_action::XDP_TX, 0, 0);
            }
        }
    }

    advance_chain(ctx)
}

// ── Chain advancement helper ──────────────────────────────────────────────────

/// Read current NfvContext, advance to next function, tail-call or apply verdict.
#[inline(always)]
fn advance_chain(ctx: &XdpContext) -> Result<u32, ()> {
    let ctx_idx: u32 = 0;

    let nfv_ctx = match NFV_CONTEXT.get_ptr_mut(ctx_idx) {
        Some(ptr) => ptr,
        None => return Ok(xdp_action::XDP_PASS),
    };

    let ptr = nfv_ctx as *mut u8;
    let chain_id = unsafe { core::ptr::read_volatile(ptr as *const u32) };
    let current_func = unsafe { core::ptr::read_volatile(ptr.add(4) as *const u32) };

    // Advance to next function
    let next_func = current_func + 1;
    unsafe {
        core::ptr::write_volatile(ptr.add(4) as *mut u32, next_func);
    }

    // Look up chain to find next function index
    let chain_def = match unsafe { CHAIN_DEFS.get(&chain_id) } {
        Some(cd) => cd,
        None => return apply_final_verdict(),
    };

    let function_count =
        unsafe { core::ptr::read_volatile(chain_def.as_ptr() as *const u32) };

    if next_func >= function_count {
        // Chain complete — apply final verdict
        let start_ns = unsafe { core::ptr::read_volatile(ptr.add(12) as *const u64) };
        let now = unsafe { bpf_ktime_get_ns() };
        let latency = now.saturating_sub(start_ns);

        emit_nfv_event(NFV_EVENT_CHAIN_END, chain_id, next_func, 0, latency, 0);

        return apply_final_verdict();
    }

    // Read next function's prog_array index from chain
    let func_offset = 4 + (next_func as usize) * 4;
    if func_offset + 4 > 44 {
        return apply_final_verdict();
    }
    let next_prog_idx = unsafe {
        core::ptr::read_volatile(chain_def.as_ptr().add(func_offset) as *const u32)
    };

    emit_nfv_event(NFV_EVENT_FUNC_EXEC, chain_id, next_func, 0, 0, 0);

    // Tail-call to next function
    unsafe {
        NFV_PROG_ARRAY.tail_call(ctx, next_prog_idx).ok();
    }

    // Tail-call failed
    increment_stat(STAT_TAIL_CALL_ERRORS);
    apply_final_verdict()
}

/// Set the verdict in NfvContext.
#[inline(always)]
fn set_verdict(verdict: u32) {
    let ctx_idx: u32 = 0;
    if let Some(nfv_ctx) = NFV_CONTEXT.get_ptr_mut(ctx_idx) {
        let ptr = nfv_ctx as *mut u8;
        unsafe {
            core::ptr::write_volatile(ptr.add(8) as *mut u32, verdict);
        }
    }
}

/// Read and return the final verdict from NfvContext.
#[inline(always)]
fn apply_final_verdict() -> Result<u32, ()> {
    let ctx_idx: u32 = 0;
    if let Some(nfv_ctx) = NFV_CONTEXT.get_ptr_mut(ctx_idx) {
        let ptr = nfv_ctx as *mut u8;
        let verdict = unsafe { core::ptr::read_volatile(ptr.add(8) as *const u32) };

        // Increment packets_processed
        let processed = unsafe { core::ptr::read_volatile(ptr.add(20) as *const u32) };
        unsafe {
            core::ptr::write_volatile(ptr.add(20) as *mut u32, processed.saturating_add(1));
        }

        match verdict {
            xdp_action::XDP_DROP => {
                increment_stat(STAT_DROPS);
            }
            xdp_action::XDP_TX => {
                increment_stat(STAT_REDIRECTS);
            }
            _ => {}
        }

        Ok(verdict)
    } else {
        Ok(xdp_action::XDP_PASS)
    }
}

// ── Packet memory helpers ─────────────────────────────────────────────────────

/// Find the byte offset of the MONAD_OPT_TYPE option within an HBH option area.
///
/// Bounded to 16 iterations — verifier-safe.  `black_box` prevents LLVM from
/// eliminating the per-iteration `data_end` checks.
#[inline(always)]
fn find_monad_option(opts_start: usize, opts_end: usize, data_end: usize) -> Result<usize, ()> {
    let opts_end = core::hint::black_box(opts_end);
    let mut offset = opts_start;

    for _ in 0..16usize {
        if offset >= opts_end {
            break;
        }
        if offset + 1 > data_end {
            break;
        }

        let opt_type = unsafe { core::ptr::read_volatile(offset as *const u8) };

        if opt_type == 0 {
            break;
        }

        if opt_type == 1 {
            offset += 1;
            continue;
        }

        if offset + 2 > data_end {
            break;
        }
        let opt_data_len = unsafe { core::ptr::read_volatile((offset + 1) as *const u8) };
        let opt_total = 2 + opt_data_len as usize;

        if opt_total < 2 || offset + opt_total > data_end {
            break;
        }

        if opt_type == MONAD_OPT_TYPE
            && opt_data_len == MONAD_OPT_DATA_LEN
            && offset + 2 + MONAD_SIZE <= data_end
        {
            return Ok(offset);
        }

        offset += opt_total;
    }

    Err(())
}

/// Read 20 Monad bytes from packet memory into a [`monad_common::Monad`] value.
#[inline(always)]
fn read_monad_from_pkt(start: usize, data_end: usize) -> Result<monad_common::Monad, ()> {
    if start + MONAD_SIZE > data_end {
        return Err(());
    }
    let mut bytes = [0u8; 20];
    #[allow(clippy::needless_range_loop)]
    for i in 0..20usize {
        bytes[i] = unsafe { core::ptr::read_volatile((start + i) as *const u8) };
    }
    Ok(monad_common::Monad::from_bytes(bytes))
}

// ── NFV event emission ────────────────────────────────────────────────────────

/// NFV Anamnesis event (24 bytes).
#[repr(C)]
struct NfvEvent {
    event_type: u8,
    chain_id: u8,
    function_index: u8,
    decision: u8,
    latency_ns: u32,
    packets_processed: u32,
    timestamp: u64,
    _pad: [u8; 4],
}

/// Emit a 24-byte NFV event to the ANAMNESIS ring buffer.
#[inline(always)]
fn emit_nfv_event(
    event_type: u8,
    chain_id: u32,
    function_index: u32,
    decision: u32,
    latency_ns: u64,
    packets_processed: u32,
) {
    let now = unsafe { bpf_ktime_get_ns() };
    let event = NfvEvent {
        event_type,
        chain_id: chain_id as u8,
        function_index: function_index as u8,
        decision: decision as u8,
        latency_ns: (latency_ns & 0xFFFF_FFFF) as u32,
        packets_processed,
        timestamp: now,
        _pad: [0u8; 4],
    };

    if let Some(mut buf) = ANAMNESIS.reserve::<NfvEvent>(0) {
        unsafe {
            core::ptr::write(buf.as_mut_ptr(), event);
            buf.submit(0);
        }
        increment_stat(STAT_EVENTS_SENT);
    } else {
        increment_stat(STAT_EVENTS_DROPPED);
    }
}

// ── Config / stats helpers ────────────────────────────────────────────────────

/// Read a CONFIG entry, returning 0 if not present.
#[inline(always)]
fn cfg(key: u32) -> u64 {
    match unsafe { CONFIG.get(&key) } {
        Some(v) => *v,
        None => 0,
    }
}

/// Saturating increment of a STATS counter.
#[inline(always)]
fn increment_stat(key: u32) {
    if let Some(v) = STATS.get_ptr_mut(&key) {
        unsafe {
            *v = (*v).saturating_add(1);
        }
    } else {
        let _ = STATS.insert(&key, &1u64, 0);
    }
}

// ── Panic handler (required for #![no_std]) ───────────────────────────────────

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    loop {}
}
