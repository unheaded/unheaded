// SPDX-License-Identifier: GPL-2.0-only
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//! Unheaded Protocol — Anomaly eBPF Program (App 12: ML Anomaly Detection at XDP)
//!
//! This XDP program implements quantized decision tree inference for real-time
//! anomaly detection at wire speed.  Features are extracted from each packet
//! and quantized to i16.  A pre-trained decision tree model (loaded by userspace)
//! is traversed to produce an anomaly score (0-100).
//!
//! Feature extraction (all quantized to i16):
//!   - Packet size (current vs baseline mean/stddev)
//!   - Inter-arrival time delta
//!   - Flow entropy estimate (byte frequency in 16-byte window)
//!   - SYN/ACK ratio (connection pattern)
//!   - Protocol deviation flags
//!
//! Decision tree inference:
//!   - Traverse from root node, comparing feature[node.feature_idx] with threshold
//!   - Max depth 12 (bounded loop, BPF verifier safe)
//!   - Return leaf node score (0-100)
//!
//! Score update uses EMA: new_score = (old_score + inferred_score) / 2
//!
//! If score > adaptive threshold: set SAMPLED flag (0x08) in Monad flags as
//! suspicious indicator.  Events sampled 1:100 to ring buffer.
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
    maps::{Array, HashMap, LruHashMap, RingBuf},
    programs::XdpContext,
};
use monad_common::{
    flags, Monad, HBH_TOTAL_LEN, IPV6_FIXED_HDR_LEN, IPV6_NEXTHDR_HBH, MONAD_OPT_DATA_LEN,
    MONAD_OPT_TYPE, MONAD_SIZE,
};

// ── BPF Maps ─────────────────────────────────────────────────────────────────

/// Decision tree model weights: node_index -> TreeNode.
/// Loaded by userspace from a trained model.
#[map]
static MODEL_WEIGHTS: Array<[u8; 12]> = Array::with_max_entries(10_000, 0);
// TreeNode layout (12 bytes):
//   [0..2]:   threshold_i16 (i16, quantized threshold for comparison)
//   [2]:      feature_idx (u8, which extracted feature to compare)
//   [3]:      flags (u8, bit 0 = is_leaf)
//   [4..6]:   left_child (u16, index of left child node)
//   [6..8]:   right_child (u16, index of right child node)
//   [8..10]:  leaf_score (i16, anomaly score if is_leaf, range 0-100)
//   [10..12]: _pad (2 bytes)

/// Per-flow state for anomaly tracking (LRU, 1M entries).
#[map]
static FLOW_STATE: LruHashMap<u32, [u8; 28]> = LruHashMap::with_max_entries(1_000_000, 0);
// FlowState layout (28 bytes):
//   [0..2]:   anomaly_score_i16 (i16, EMA-filtered anomaly score)
//   [2..6]:   packet_count (u32, total packets seen for this flow)
//   [6..14]:  last_seen_ns (u64, timestamp of last packet)
//   [14..22]: byte_total (u64, cumulative bytes for this flow)
//   [22..26]: pkt_size_sum (u32, sum of packet sizes for mean calculation)
//   [26..28]: inter_arrival_sum (u16, truncated sum of inter-arrival times)

/// Adaptive thresholds per service: service_id -> Threshold.
#[map]
static ADAPTIVE_THRESHOLDS: HashMap<u32, [u8; 8]> = HashMap::with_max_entries(256, 0);
// Threshold layout (8 bytes):
//   [0..2]:   mean_i16 (i16, running mean of anomaly scores)
//   [2..4]:   std_dev_i16 (i16, running stddev approximation)
//   [4..6]:   threshold_i16 (i16, current detection threshold)
//   [6..8]:   _pad (2 bytes)

/// Baseline statistics per service for feature normalization.
#[map]
static BASELINE_STATS: HashMap<u32, [u8; 16]> = HashMap::with_max_entries(256, 0);
// BaselineStats layout (16 bytes):
//   [0..2]:   pkt_size_mean (i16, baseline packet size mean)
//   [2..4]:   pkt_size_stddev (i16, baseline packet size stddev)
//   [4..6]:   entropy_mean (i16, baseline entropy mean)
//   [6..8]:   conn_rate (i16, baseline connection rate)
//   [8..16]:  _pad (8 bytes)

/// Anomaly events emitted to ring buffer (sampled 1:100).
#[map]
static ANOMALY_EVENTS: RingBuf = RingBuf::with_byte_size(256 * 1024, 0);

/// Runtime configuration — tunable from userspace without reloading.
///
/// | Key | Meaning                            | Default |
/// |-----|------------------------------------|---------|
/// | 0   | node_id (this node's identifier)   | 0       |
/// | 1   | default_threshold (score 0-100)    | 50      |
/// | 2   | sample_rate (1:N event sampling)   | 100     |
/// | 3   | max_tree_depth (bounded traversal) | 12      |
/// | 4   | enable_flagging (set SAMPLED flag) | 1       |
#[map]
static CONFIG: HashMap<u32, u64> = HashMap::with_max_entries(16, 0);

/// Statistics counters — observable from userspace.
#[map]
static STATS: HashMap<u32, u64> = HashMap::with_max_entries(32, 0);

// ── Stats keys ────────────────────────────────────────────────────────────────
const STAT_PACKETS_TOTAL: u32 = 0;
const STAT_ANOMALIES_DETECTED: u32 = 1;
const STAT_SCORES_ABOVE_THRESHOLD: u32 = 2;
const STAT_MODEL_EVALUATIONS: u32 = 3;
const STAT_THRESHOLD_UPDATES: u32 = 4;
const STAT_EVENTS_SENT: u32 = 5;
const STAT_EVENTS_DROPPED: u32 = 6;
const STAT_FLOWS_TRACKED: u32 = 7;
const STAT_FLOWS_NEW: u32 = 8;

// ── Config keys ───────────────────────────────────────────────────────────────
const CFG_NODE_ID: u32 = 0;
const CFG_DEFAULT_THRESHOLD: u32 = 1;
const CFG_SAMPLE_RATE: u32 = 2;
const CFG_MAX_TREE_DEPTH: u32 = 3;
const CFG_ENABLE_FLAGGING: u32 = 4;

// ── Feature indices for decision tree ─────────────────────────────────────────
const FEAT_PKT_SIZE: u8 = 0;
const FEAT_INTER_ARRIVAL: u8 = 1;
const FEAT_ENTROPY: u8 = 2;
const FEAT_CONN_PATTERN: u8 = 3;
const FEAT_PROTO_DEVIATION: u8 = 4;
const NUM_FEATURES: usize = 5;

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

// ── Anomaly event structure ───────────────────────────────────────────────────

/// Anomaly event emitted to ring buffer (24 bytes).
#[repr(C)]
struct AnomalyEvent {
    flow_key: u32,
    score: i16,
    service_id: u8,
    _pad: u8,
    timestamp: u64,
    features: [i16; 4], // truncated feature snapshot
}

// ── XDP entry point ──────────────────────────────────────────────────────────

#[xdp]
pub fn anomaly_xdp(ctx: XdpContext) -> u32 {
    match try_anomaly_xdp(&ctx) {
        Ok(action) => action,
        Err(_) => xdp_action::XDP_PASS, // fail-open: never silently drop
    }
}

#[inline(always)]
fn try_anomaly_xdp(ctx: &XdpContext) -> Result<u32, ()> {
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

    // Extract flow label as flow key
    let vtf = u32::from_be(unsafe { core::ptr::read_unaligned(core::ptr::addr_of!(ip.vtf)) });
    let flow_key = vtf & 0x000F_FFFF;

    // Read payload length for packet size feature
    let payload_len =
        u16::from_be(unsafe { core::ptr::read_unaligned(core::ptr::addr_of!(ip.payload_len)) });

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

    let now = unsafe { bpf_ktime_get_ns() };
    let service_id = monad.dst_service_id as u32;

    // ── Feature Extraction ────────────────────────────────────────────────────
    let mut features = [0i16; NUM_FEATURES];

    // Feature 0: Packet size deviation from baseline
    features[FEAT_PKT_SIZE as usize] = extract_pkt_size_feature(payload_len, service_id);

    // Feature 1: Inter-arrival time delta
    features[FEAT_INTER_ARRIVAL as usize] = extract_inter_arrival_feature(flow_key, now);

    // Feature 2: Flow entropy estimate (byte frequency in 16-byte window)
    features[FEAT_ENTROPY as usize] =
        extract_entropy_feature(data, data_end, hbh_start + hbh_total);

    // Feature 3: Connection pattern (SYN/ACK ratio approximation from Monad)
    features[FEAT_CONN_PATTERN as usize] = extract_conn_pattern_feature(&monad);

    // Feature 4: Protocol deviation flags
    features[FEAT_PROTO_DEVIATION as usize] = extract_proto_deviation_feature(&monad);

    // ── Decision Tree Inference ───────────────────────────────────────────────
    let inferred_score = run_decision_tree(&features);
    increment_stat(STAT_MODEL_EVALUATIONS);

    // ── Flow State Update (EMA filter) ────────────────────────────────────────
    let current_score = update_flow_state(flow_key, inferred_score, now, payload_len);

    // ── Adaptive Threshold Check ──────────────────────────────────────────────
    let threshold = get_threshold(service_id);

    if current_score > threshold {
        increment_stat(STAT_SCORES_ABOVE_THRESHOLD);
        increment_stat(STAT_ANOMALIES_DETECTED);

        // Set SAMPLED flag (0x08) in Monad as suspicious indicator
        let enable_flagging = cfg(CFG_ENABLE_FLAGGING);
        if enable_flagging != 0 {
            let mut m = monad;
            m.set_flag(flags::SAMPLED);

            // Write modified Monad back
            write_monad_to_pkt(monad_data_off, data_end, &m)?;
        }

        // Sample events 1:100 to ring buffer
        let sample_rate = cfg(CFG_SAMPLE_RATE) as u32;
        let rate = if sample_rate == 0 { 100 } else { sample_rate };

        // Use packet_count from flow state for sampling
        let pkt_count = get_flow_packet_count(flow_key);
        if pkt_count % rate == 0 {
            emit_anomaly_event(flow_key, current_score, service_id as u8, &features);
        }
    }

    Ok(xdp_action::XDP_PASS)
}

// ── Feature Extraction Functions ──────────────────────────────────────────────

/// Extract packet size feature: deviation from baseline mean, quantized to i16.
#[inline(always)]
fn extract_pkt_size_feature(payload_len: u16, service_id: u32) -> i16 {
    let (mean, stddev) = match unsafe { BASELINE_STATS.get(&service_id) } {
        Some(bs) => {
            let mean = unsafe { core::ptr::read_volatile(bs.as_ptr() as *const i16) };
            let stddev = unsafe { core::ptr::read_volatile(bs.as_ptr().add(2) as *const i16) };
            (mean, if stddev == 0 { 1 } else { stddev })
        }
        None => (512i16, 128i16), // reasonable defaults
    };

    // Z-score: (value - mean) / stddev, scaled to i16 range
    let diff = (payload_len as i16).saturating_sub(mean);
    // Multiply by 100 for precision before dividing
    // BPF doesn't support signed division — use unsigned with sign tracking
    let is_negative = diff < 0;
    let abs_diff = if diff < 0 {
        (-diff) as u32
    } else {
        diff as u32
    };
    let abs_stddev = if stddev == 0 { 1u32 } else { stddev as u32 };
    let z_abs = (abs_diff * 100) / abs_stddev;
    let z_score_scaled = if is_negative {
        -(z_abs as i32)
    } else {
        z_abs as i32
    };
    z_score_scaled.clamp(-32768, 32767) as i16
}

/// Extract inter-arrival time delta from flow state.
#[inline(always)]
fn extract_inter_arrival_feature(flow_key: u32, now: u64) -> i16 {
    if let Some(flow) = unsafe { FLOW_STATE.get(&flow_key) } {
        let last_seen = unsafe { core::ptr::read_volatile(flow.as_ptr().add(6) as *const u64) };
        if last_seen > 0 {
            let delta_ns = now.saturating_sub(last_seen);
            // Convert to microseconds and clamp to i16
            let delta_us = (delta_ns / 1000) as i32;
            return delta_us.clamp(0, 32767) as i16;
        }
    }
    0i16 // first packet in flow
}

/// Extract entropy estimate from a 16-byte window after the HBH header.
/// Uses a simplified byte frequency approximation.
#[inline(always)]
fn extract_entropy_feature(data: usize, data_end: usize, payload_start: usize) -> i16 {
    let window_size: usize = 16;

    if payload_start + window_size > data_end {
        return 0;
    }

    // Count unique byte values in the 16-byte window (simplified entropy)
    let mut seen = [0u8; 16]; // track up to 16 unique values
    let mut unique_count: u8 = 0;

    for i in 0..16usize {
        let byte_val = unsafe {
            core::ptr::read_volatile(core::hint::black_box((payload_start + i) as *const u8))
        };

        // Check if we have seen this value before (bounded scan)
        let mut found = false;
        for j in 0..16usize {
            if j >= unique_count as usize {
                break;
            }
            if core::hint::black_box(seen[j]) == byte_val {
                found = true;
                break;
            }
        }

        if !found && (unique_count as usize) < 16 {
            seen[unique_count as usize] = byte_val;
            unique_count = unique_count.saturating_add(1);
        }
    }

    // Normalize: 0 unique = 0 entropy, 16 unique = max entropy
    // Scale to 0-1000 range (i16)
    ((unique_count as i16) * 62) // 16 * 62 ~= 1000
}

/// Extract connection pattern feature from Monad fields.
/// Uses hop_count, flow_action, and circuit_state to approximate SYN/ACK ratio.
#[inline(always)]
fn extract_conn_pattern_feature(monad: &Monad) -> i16 {
    // Heuristic: unusual flow_action or circuit_state patterns indicate anomalies
    let mut score: i16 = 0;

    // High hop count relative to normal is suspicious
    if monad.hop_count > 20 {
        score = score.saturating_add(200);
    }

    // Non-standard flow actions
    if monad.flow_action > 5 {
        score = score.saturating_add(300);
    }

    // Open circuit state is abnormal for most traffic
    if monad.circuit_state == monad_common::circuit_state::OPEN {
        score = score.saturating_add(250);
    }

    // CHAOS flag set is inherently suspicious
    if monad.has_flag(flags::CHAOS) {
        score = score.saturating_add(500);
    }

    score
}

/// Extract protocol deviation flags from Monad.
#[inline(always)]
fn extract_proto_deviation_feature(monad: &Monad) -> i16 {
    let mut deviation: i16 = 0;

    // Version mismatch
    if monad.version != monad_common::MONAD_VERSION {
        deviation = deviation.saturating_add(500);
    }

    // Reserved flag set (violation)
    if monad.has_flag(flags::RSVD) {
        deviation = deviation.saturating_add(400);
    }

    // CUSTOM flag with unexpected scratch values
    if monad.has_flag(flags::CUSTOM) {
        deviation = deviation.saturating_add(100);
    }

    // Invalid QoS class (0 is unusual)
    if monad.qos_class == 0 {
        deviation = deviation.saturating_add(50);
    }

    deviation
}

// ── Decision Tree Inference ───────────────────────────────────────────────────

/// Traverse the decision tree model and return a leaf score (0-100).
/// Max depth 12 — bounded loop for BPF verifier safety.
#[inline(always)]
fn run_decision_tree(features: &[i16; NUM_FEATURES]) -> i16 {
    let max_depth = cfg(CFG_MAX_TREE_DEPTH) as u32;
    let max_depth = if max_depth == 0 || max_depth > 12 {
        12
    } else {
        max_depth
    };

    let mut node_idx: u32 = 0; // start at root

    // Bounded traversal — max 12 iterations
    for _ in 0..12u32 {
        let node = match MODEL_WEIGHTS.get(node_idx) {
            Some(n) => n,
            None => return 50, // model not loaded, return neutral score
        };

        // Parse TreeNode fields
        let threshold = unsafe { core::ptr::read_volatile(node.as_ptr() as *const i16) };
        let feature_idx = unsafe { core::ptr::read_volatile(node.as_ptr().add(2)) };
        let node_flags = unsafe { core::ptr::read_volatile(node.as_ptr().add(3)) };
        let left_child = unsafe { core::ptr::read_volatile(node.as_ptr().add(4) as *const u16) };
        let right_child = unsafe { core::ptr::read_volatile(node.as_ptr().add(6) as *const u16) };
        let leaf_score = unsafe { core::ptr::read_volatile(node.as_ptr().add(8) as *const i16) };

        // Check if leaf node (bit 0 of flags)
        let is_leaf = (node_flags & 0x01) != 0;
        if is_leaf {
            // Clamp score to 0-100
            return if leaf_score < 0 {
                0
            } else if leaf_score > 100 {
                100
            } else {
                leaf_score
            };
        }

        // Get the feature value for comparison
        let feat_val = if (feature_idx as usize) < NUM_FEATURES {
            features[feature_idx as usize]
        } else {
            0 // out of bounds, use 0
        };

        // Traverse: left if less than threshold, right if greater/equal
        if feat_val < threshold {
            node_idx = left_child as u32;
        } else {
            node_idx = right_child as u32;
        }

        // Bounds check to prevent infinite loops in corrupted models
        if node_idx >= 10_000 {
            return 50; // corrupted model, return neutral
        }
    }

    // Reached max depth without finding leaf — return neutral score
    50
}

// ── Flow State Management ─────────────────────────────────────────────────────

/// Update flow state with new score using EMA filter.
/// Returns the updated (filtered) anomaly score.
#[inline(always)]
fn update_flow_state(flow_key: u32, inferred_score: i16, now: u64, payload_len: u16) -> i16 {
    if let Some(flow) = FLOW_STATE.get_ptr_mut(&flow_key) {
        let ptr = flow as *mut u8;

        // Read current state
        let old_score = unsafe { core::ptr::read_volatile(ptr as *const i16) };
        let pkt_count = unsafe { core::ptr::read_volatile(ptr.add(2) as *const u32) };
        let byte_total = unsafe { core::ptr::read_volatile(ptr.add(14) as *const u64) };
        let pkt_size_sum = unsafe { core::ptr::read_volatile(ptr.add(22) as *const u32) };

        // EMA: new_score = (old_score + inferred_score) / 2
        let new_score = ((old_score as i32 + inferred_score as i32) / 2) as i16;

        // Update state
        unsafe {
            core::ptr::write_volatile(ptr as *mut i16, new_score);
            core::ptr::write_volatile(ptr.add(2) as *mut u32, pkt_count.saturating_add(1));
            core::ptr::write_volatile(ptr.add(6) as *mut u64, now);
            core::ptr::write_volatile(
                ptr.add(14) as *mut u64,
                byte_total.saturating_add(payload_len as u64),
            );
            core::ptr::write_volatile(
                ptr.add(22) as *mut u32,
                pkt_size_sum.saturating_add(payload_len as u32),
            );
        }

        increment_stat(STAT_FLOWS_TRACKED);
        new_score
    } else {
        // New flow — initialize state
        let mut state = [0u8; 28];
        unsafe {
            // anomaly_score_i16
            core::ptr::write_volatile(state.as_mut_ptr() as *mut i16, inferred_score);
            // packet_count = 1
            core::ptr::write_volatile(state.as_mut_ptr().add(2) as *mut u32, 1u32);
            // last_seen_ns
            core::ptr::write_volatile(state.as_mut_ptr().add(6) as *mut u64, now);
            // byte_total
            core::ptr::write_volatile(state.as_mut_ptr().add(14) as *mut u64, payload_len as u64);
            // pkt_size_sum
            core::ptr::write_volatile(state.as_mut_ptr().add(22) as *mut u32, payload_len as u32);
        }
        let _ = FLOW_STATE.insert(&flow_key, &state, 0);
        increment_stat(STAT_FLOWS_NEW);
        inferred_score
    }
}

/// Get packet count from flow state for sampling decisions.
#[inline(always)]
fn get_flow_packet_count(flow_key: u32) -> u32 {
    if let Some(flow) = unsafe { FLOW_STATE.get(&flow_key) } {
        unsafe { core::ptr::read_volatile(flow.as_ptr().add(2) as *const u32) }
    } else {
        1
    }
}

/// Get the adaptive threshold for a service, falling back to config default.
#[inline(always)]
fn get_threshold(service_id: u32) -> i16 {
    if let Some(thresh) = unsafe { ADAPTIVE_THRESHOLDS.get(&service_id) } {
        unsafe { core::ptr::read_volatile(thresh.as_ptr().add(4) as *const i16) }
    } else {
        let default = cfg(CFG_DEFAULT_THRESHOLD) as i16;
        if default == 0 {
            50
        } else {
            default
        }
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

/// Write a mutated [`Monad`] back into packet memory at the same offset.
#[inline(always)]
fn write_monad_to_pkt(start: usize, data_end: usize, m: &Monad) -> Result<(), ()> {
    if start + MONAD_SIZE > data_end {
        return Err(());
    }
    let bytes = m.to_bytes();
    #[allow(clippy::needless_range_loop)]
    for i in 0..20usize {
        unsafe { core::ptr::write_volatile((start + i) as *mut u8, bytes[i]) };
    }
    Ok(())
}

// ── Anomaly event emission ────────────────────────────────────────────────────

/// Emit an anomaly event to the ANOMALY_EVENTS ring buffer.
#[inline(always)]
fn emit_anomaly_event(flow_key: u32, score: i16, service_id: u8, features: &[i16; NUM_FEATURES]) {
    let now = unsafe { bpf_ktime_get_ns() };
    let event = AnomalyEvent {
        flow_key,
        score,
        service_id,
        _pad: 0,
        timestamp: now,
        features: [features[0], features[1], features[2], features[3]],
    };

    if let Some(mut buf) = ANOMALY_EVENTS.reserve::<AnomalyEvent>(0) {
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
