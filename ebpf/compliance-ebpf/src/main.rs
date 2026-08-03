// SPDX-License-Identifier: GPL-2.0-only
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//! Unheaded Protocol — Zero-Trust Compliance Enforcement (App 09)
//!
//! This XDP program implements wire-level compliance checking:
//!
//! 1. Parse IPv6 + HBH headers to locate the Monad option
//! 2. Extract src/dst service IDs, classification (K1|K0 bits), flags
//! 3. Look up service policy: allow or block
//! 4. If SENSITIVE (10) or SOVEREIGN_PII (11): verify encryption flag (E bit) is set
//! 5. Check geographic zone: if PII, verify packet doesn't exit zone
//! 6. Write 64-byte audit record to AUDIT_LOG ring buffer
//! 7. Block non-compliant traffic at wire speed
//!
//! # Data Classification
//!
//! Classification is encoded in Monad scratch registers (K1|K0 bits):
//! - 00 = PUBLIC: no restrictions
//! - 01 = INTERNAL: organizational access only
//! - 10 = SENSITIVE: requires encryption (E flag must be set)
//! - 11 = SOVEREIGN_PII: requires encryption + geo-zone containment
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
    maps::{HashMap, RingBuf},
    programs::XdpContext,
};
use monad_common::{
    flags, Monad, HBH_TOTAL_LEN, IPV6_FIXED_HDR_LEN, IPV6_NEXTHDR_HBH, MONAD_OPT_DATA_LEN,
    MONAD_OPT_TYPE, MONAD_SIZE,
};

// ── Classification levels (K1|K0 bits) ──────────────────────────────────────

#[allow(dead_code)] // wire-protocol enum value — defines the ABI, not all variants are emitted yet
const CLASS_PUBLIC: u8 = 0b00;
#[allow(dead_code)] // wire-protocol enum value — defines the ABI, not all variants are emitted yet
const CLASS_INTERNAL: u8 = 0b01;
const CLASS_SENSITIVE: u8 = 0b10;
const CLASS_SOVEREIGN_PII: u8 = 0b11;

// ── Policy action values ─────────────────────────────────────────────────────

const ACTION_ALLOW: u8 = 0x01;
const ACTION_BLOCK: u8 = 0x02;

// ── Violation types for audit records ────────────────────────────────────────

const VIOLATION_NONE: u8 = 0x00;
const VIOLATION_POLICY_DENIED: u8 = 0x01;
const VIOLATION_ENCRYPTION_REQUIRED: u8 = 0x02;
const VIOLATION_PII_EGRESS: u8 = 0x03;
const VIOLATION_ZONE_BREACH: u8 = 0x04;

// ── BPF Maps ─────────────────────────────────────────────────────────────────

/// Service-to-service policy map.
/// Key: (src_svc: u16, dst_svc: u16) packed as u32
/// Value: PolicyAction { action(u8), audit_level(u8), min_qos(u8),
///        max_latency_ms(u8), requires_encryption(u8), _pad(u8,u16) } = 8 bytes
#[map]
static SERVICE_POLICIES: HashMap<u32, [u8; 8]> = HashMap::with_max_entries(4096, 0);

/// Classification tag metadata.
/// Key: tag (u32, only low 4 bits used for K1|K0 combos)
/// Value: ClassInfo { name_hash(u32), encryption_required(u8),
///        geo_constraints(u8), retention_days(u16) } = 8 bytes
#[map]
static CLASSIFICATION_TAGS: HashMap<u32, [u8; 8]> = HashMap::with_max_entries(16, 0);

/// Geographic zone definitions.
/// Key: zone_id (u32)
/// Value: ZoneInfo { allowed_exit_zones_mask(u64), requires_vpn(u8), pii_vault(u8),
///        _pad(u16), _reserved(u32) } = 16 bytes
#[map]
static GEO_ZONES: HashMap<u32, [u8; 16]> = HashMap::with_max_entries(64, 0);

/// Audit log ring buffer — 2 MiB for compliance audit records.
/// Each record is 64 bytes.
#[map]
static AUDIT_LOG: RingBuf = RingBuf::with_byte_size(2 * 1024 * 1024, 0);

/// Runtime configuration — tunable from userspace without reloading.
#[map]
static CONFIG: HashMap<u32, u64> = HashMap::with_max_entries(16, 0);

/// Statistics counters — observable from userspace.
#[map]
static STATS: HashMap<u32, u64> = HashMap::with_max_entries(32, 0);

// ── Stats keys ────────────────────────────────────────────────────────────────
const STAT_PACKETS_TOTAL: u32 = 0;
const STAT_ALLOWED: u32 = 1;
const STAT_BLOCKED: u32 = 2;
const STAT_POLICY_VIOLATIONS: u32 = 3;
const STAT_PII_EGRESS_BLOCKED: u32 = 4;
const STAT_ENCRYPTION_VIOLATIONS: u32 = 5;
const STAT_AUDIT_RECORDS: u32 = 6;
#[allow(dead_code)] // wire-protocol enum value — defines the ABI, not all variants are emitted yet
const STAT_EVENTS_SENT: u32 = 7;
#[allow(dead_code)] // wire-protocol enum value — defines the ABI, not all variants are emitted yet
const STAT_EVENTS_DROPPED: u32 = 8;

// ── Config keys ──────────────────────────────────────────────────────────────
#[allow(dead_code)] // wire-protocol enum value — defines the ABI, not all variants are emitted yet
const CFG_HOP_ID: u32 = 0;
const CFG_LOCAL_ZONE_ID: u32 = 1;

// ── Audit record (64 bytes) ─────────────────────────────────────────────────

/// Wire-level audit record emitted to AUDIT_LOG ring buffer.
///
/// Layout (64 bytes, one cache line on x86):
/// ```text
/// [0..8]:    timestamp_ns     — bpf_ktime_get_ns()
/// [8..10]:   src_service_id   — source service (u16)
/// [10..12]:  dst_service_id   — destination service (u16)
/// [12]:      decision         — ALLOW(1) or BLOCK(2)
/// [13]:      violation_type   — see VIOLATION_* constants
/// [14]:      classification   — K1|K0 bits
/// [15]:      qos_class        — from Monad
/// [16..20]:  trace_id         — flow_label (u32)
/// [20]:      zone_id          — geographic zone
/// [21]:      flags            — Monad flags snapshot
/// [22..24]:  latency_hint     — from Monad
/// [24..44]:  monad_snapshot   — full 20-byte Monad
/// [44..64]:  _reserved        — reserved for future use
/// ```
#[repr(C)]
#[derive(Clone, Copy)]
struct AuditRecord {
    timestamp_ns: u64,
    src_service_id: u16,
    dst_service_id: u16,
    decision: u8,
    violation_type: u8,
    classification: u8,
    qos_class: u8,
    trace_id: u32,
    zone_id: u8,
    flags: u8,
    latency_hint: [u8; 2],
    monad_snapshot: [u8; 20],
    _reserved: [u8; 20],
}

const _: () = {
    let _ = [(); 64 - core::mem::size_of::<AuditRecord>()];
    let _ = [(); core::mem::size_of::<AuditRecord>() - 64];
};

// ── Wire-format helpers ──────────────────────────────────────────────────────

#[repr(C, packed)]
struct EthHdr {
    dst: [u8; 6],
    src: [u8; 6],
    proto: u16,
}

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

// ── XDP entry point ──────────────────────────────────────────────────────────

#[xdp]
pub fn compliance_xdp(ctx: XdpContext) -> u32 {
    match try_compliance_xdp(&ctx) {
        Ok(action) => action,
        Err(_) => xdp_action::XDP_PASS, // fail-open: never silently drop
    }
}

#[inline(always)]
fn try_compliance_xdp(ctx: &XdpContext) -> Result<u32, ()> {
    increment_stat(STAT_PACKETS_TOTAL);

    let data = ctx.data();
    let data_end = ctx.data_end();

    // ── Ethernet ─────────────────────────────────────────────────────────────
    if data + ETH_HLEN > data_end {
        return Ok(xdp_action::XDP_PASS);
    }
    let eth = unsafe { &*(data as *const EthHdr) };
    if u16::from_be(eth.proto) != ETH_P_IPV6 {
        return Ok(xdp_action::XDP_PASS);
    }

    // ── IPv6 Fixed Header ────────────────────────────────────────────────────
    let ip_start = data + ETH_HLEN;
    if ip_start + IPV6_FIXED_HDR_LEN > data_end {
        return Ok(xdp_action::XDP_PASS);
    }
    let ip = unsafe { &*(ip_start as *const Ipv6Hdr) };

    if ip.next_header != IPV6_NEXTHDR_HBH {
        return Ok(xdp_action::XDP_PASS);
    }

    // Extract Flow Label from the packed vtf field (low 20 bits).
    let vtf = u32::from_be(unsafe { core::ptr::read_unaligned(core::ptr::addr_of!(ip.vtf)) });
    let flow_label = vtf & 0x000F_FFFF;

    // ── Hop-by-Hop Header ────────────────────────────────────────────────────
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

    // ── Find Monad option ────────────────────────────────────────────────────
    let monad_opt_off = find_monad_option(opts_start, opts_end, data_end)?;

    let monad_data_off = monad_opt_off + 2;
    if monad_data_off + MONAD_SIZE > data_end {
        return Ok(xdp_action::XDP_PASS);
    }

    // ── Read Monad ───────────────────────────────────────────────────────────
    let monad = read_monad_from_pkt(monad_data_off, data_end)?;

    // ── Extract classification from scratch registers (K1|K0 bits) ───────────
    // Classification is stored in the low 2 bits of scratch[0]
    let classification = monad.scratch[0] & 0x03;

    // ── Extract zone_id from dst_prefix_lo ───────────────────────────────────
    let src_zone = monad.src_prefix_lo;
    let dst_zone = monad.dst_prefix_lo;

    // ── Service policy lookup ────────────────────────────────────────────────
    let policy_key = (monad.src_service_id as u32) | ((monad.dst_service_id as u32) << 16);
    let mut decision = ACTION_ALLOW;
    let mut violation = VIOLATION_NONE;

    if let Some(policy) = unsafe { SERVICE_POLICIES.get(&policy_key) } {
        let action = unsafe { core::ptr::read_volatile(&policy[0]) };
        let _audit_level = unsafe { core::ptr::read_volatile(&policy[1]) };
        let requires_enc = unsafe { core::ptr::read_volatile(&policy[4]) };

        if action == ACTION_BLOCK {
            decision = ACTION_BLOCK;
            violation = VIOLATION_POLICY_DENIED;
            increment_stat(STAT_POLICY_VIOLATIONS);
        }

        // Check encryption requirement from policy
        if requires_enc != 0 && !monad.has_flag(flags::ENCRYPT) {
            decision = ACTION_BLOCK;
            violation = VIOLATION_ENCRYPTION_REQUIRED;
            increment_stat(STAT_ENCRYPTION_VIOLATIONS);
        }
    }

    // ── Classification-based enforcement ─────────────────────────────────────
    if decision == ACTION_ALLOW {
        match classification {
            CLASS_SENSITIVE | CLASS_SOVEREIGN_PII
                // SENSITIVE and SOVEREIGN_PII require encryption (E bit)
                if !monad.has_flag(flags::ENCRYPT) => {
                    decision = ACTION_BLOCK;
                    violation = VIOLATION_ENCRYPTION_REQUIRED;
                    increment_stat(STAT_ENCRYPTION_VIOLATIONS);
                }
            _ => {
                // PUBLIC and INTERNAL: no encryption requirement
            }
        }
    }

    // ── Geographic zone enforcement (PII) ────────────────────────────────────
    if decision == ACTION_ALLOW && classification == CLASS_SOVEREIGN_PII {
        let local_zone = cfg(CFG_LOCAL_ZONE_ID) as u32;

        // Check if the destination zone allows exit from the source zone
        if let Some(zone_info) = unsafe { GEO_ZONES.get(&(dst_zone as u32)) } {
            // allowed_exit_zones_mask at bytes 0..8
            let allowed_mask = u64::from_ne_bytes([
                zone_info[0],
                zone_info[1],
                zone_info[2],
                zone_info[3],
                zone_info[4],
                zone_info[5],
                zone_info[6],
                zone_info[7],
            ]);
            let pii_vault = unsafe { core::ptr::read_volatile(&zone_info[9]) };

            // Check if PII is allowed to exit to this zone
            if pii_vault != 0 {
                // This is a PII vault zone — check if dst zone is in allowed mask
                let zone_bit = 1u64 << (dst_zone as u64 & 0x3F);
                if allowed_mask & zone_bit == 0 {
                    decision = ACTION_BLOCK;
                    violation = VIOLATION_PII_EGRESS;
                    increment_stat(STAT_PII_EGRESS_BLOCKED);
                }
            }

            // Also check local zone constraint
            if local_zone > 0 && src_zone != dst_zone {
                let src_bit = 1u64 << (src_zone as u64 & 0x3F);
                if allowed_mask & src_bit == 0 {
                    decision = ACTION_BLOCK;
                    violation = VIOLATION_ZONE_BREACH;
                    increment_stat(STAT_PII_EGRESS_BLOCKED);
                }
            }
        }
    }

    // ── Write audit record ───────────────────────────────────────────────────
    let now = unsafe { bpf_ktime_get_ns() };
    let record = AuditRecord {
        timestamp_ns: now,
        src_service_id: monad.src_service_id as u16,
        dst_service_id: monad.dst_service_id as u16,
        decision,
        violation_type: violation,
        classification,
        qos_class: monad.qos_class,
        trace_id: flow_label,
        zone_id: dst_zone,
        flags: monad.flags,
        latency_hint: monad.latency_hint,
        monad_snapshot: monad.to_bytes(),
        _reserved: [0u8; 20],
    };

    if let Some(mut buf) = AUDIT_LOG.reserve::<AuditRecord>(0) {
        unsafe {
            core::ptr::write(buf.as_mut_ptr(), record);
            buf.submit(0);
        }
        increment_stat(STAT_AUDIT_RECORDS);
    }

    // ── Enforce decision ─────────────────────────────────────────────────────
    if decision == ACTION_BLOCK {
        increment_stat(STAT_BLOCKED);
        return Ok(xdp_action::XDP_DROP);
    }

    increment_stat(STAT_ALLOWED);
    Ok(xdp_action::XDP_PASS)
}

// ── Packet memory helpers ─────────────────────────────────────────────────────

/// Find the byte offset of the MONAD_OPT_TYPE option within an HBH option area.
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

/// Read 20 Monad bytes from packet memory into a [`Monad`] value.
#[inline(always)]
fn read_monad_from_pkt(start: usize, data_end: usize) -> Result<Monad, ()> {
    if start + MONAD_SIZE > data_end {
        return Err(());
    }
    let mut bytes = [0u8; 20];
    #[allow(clippy::needless_range_loop)]
    for i in 0..20usize {
        bytes[i] = unsafe { core::ptr::read_volatile((start + i) as *const u8) };
    }
    Ok(Monad::from_bytes(bytes))
}

// A `write_monad_to_pkt` helper lived here, carried over from the program
// template. Removed 2026-08-03: this program binds the Monad immutably and
// never mutates it — it is a read-only classifier, so there was nothing to
// write back. Restore it from git if this program ever needs to rewrite the
// register in place.

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

// ── Panic handler (required for #![no_std]) ─────────────────────────────────

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    loop {}
}
