//! # monad-common
//!
//! Shared types for the **Unheaded Protocol Foundation**
//! (draft-bellis-unheaded-protocol-foundation-01).
//!
//! ## Wire Format
//!
//! The Monad is a 20-octet register file carried as an IPv6 Hop-by-Hop option.
//! Every Kingdom packet carries one.  eBPF programs at each hop read, compute
//! on, and write back the Monad at wire speed.
//!
//! ```text
//! IPv6 Fixed Header (40 octets)
//!   Next Header = 0 (Hop-by-Hop)
//!   Flow Label  = trace correlation (bpf_get_prandom_u32, set by Shield)
//!
//! Hop-by-Hop Options Header (24 octets)
//!   Next Header  = original transport protocol
//!   Hdr Ext Len  = 2 (2 × 8 bytes beyond the first 8 = 24 total)
//!   Opt Type     = MONAD_OPT_TYPE (action 00, change-en-route 1)
//!   Opt Data Len = 20
//!   [Monad: 20 bytes]
//!
//! Payload (TCP/UDP/etc)
//! ```
//!
//! ## Crate Compatibility
//!
//! This crate compiles for:
//! - `bpfel-unknown-none` — kernel-space eBPF programs (no_std, no alloc)
//! - any host target       — userspace tests, trace-collector, wotan bridge
//!
//! The `#![cfg_attr(not(test), no_std)]` gate keeps std out of BPF builds
//! while allowing normal `cargo test` on the host.

#![cfg_attr(not(test), no_std)]
#![deny(missing_docs)]

// ── Protocol version ─────────────────────────────────────────────────────────

/// Protocol version carried in `Monad.version` (offset 0x00).
/// Value 0x01 corresponds to draft-bellis-unheaded-protocol-foundation-01.
pub const MONAD_VERSION: u8 = 0x01;

// ── Hop-by-Hop option type ───────────────────────────────────────────────────

/// IPv6 Hop-by-Hop Option Type for the Monad.
///
/// Encoding per RFC 8200 §4.2 and RFC 9673 §6:
///   bits 7-6 (action):          00 — skip if unrecognized
///   bit  5   (change-en-route): 1  — option data MAY change at each hop
///   bits 4-0 (number):          0x1E (30) — locally assigned in limited domain
///
/// Binary: 0b0011_1110 = 0x3E
pub const MONAD_OPT_TYPE: u8 = 0x3E;

/// Fixed length of the Monad Option Data (octets 0x00–0x13 of the option body).
pub const MONAD_OPT_DATA_LEN: u8 = 20;

/// Hdr Ext Len value for the Hop-by-Hop Options header carrying the Monad.
/// Value 2 → 2 additional 8-octet units beyond the first 8 → total 24 octets.
pub const HBH_HDR_EXT_LEN: u8 = 2;

/// Total size of the Hop-by-Hop Options header including the Monad (octets).
pub const HBH_TOTAL_LEN: usize = 24;

/// Size of the Monad register file (octets).
pub const MONAD_SIZE: usize = 20;

/// IPv6 Next Header value for Hop-by-Hop Options (RFC 8200 §4.3).
pub const IPV6_NEXTHDR_HBH: u8 = 0;

/// IPv6 fixed header size (octets).
pub const IPV6_FIXED_HDR_LEN: usize = 40;

// ── Monad field offsets ───────────────────────────────────────────────────────
//
// These constants give BPF programs fast, fixed-offset access to each Monad
// register without decoding the struct.  They mirror the [Monad] field layout
// byte-for-byte and are verified by the test suite.

/// Offset of the Version field (raw, not exponent-encoded).
pub const OFF_VERSION: usize = 0x00;
/// Offset of the Src Service ID field (exponent-encoded).
pub const OFF_SRC_SERVICE_ID: usize = 0x01;
/// Offset of the Dst Service ID field (exponent-encoded).
pub const OFF_DST_SERVICE_ID: usize = 0x02;
/// Offset of the Hop Count field (raw u8, incremented by each hop).
pub const OFF_HOP_COUNT: usize = 0x03;
/// Offset of the QoS Class field (exponent-encoded).
pub const OFF_QOS_CLASS: usize = 0x04;
/// Offset of the Flow Action field (exponent-encoded).
pub const OFF_FLOW_ACTION: usize = 0x05;
/// Offset of the Circuit State field (exponent-encoded).
pub const OFF_CIRCUIT_STATE: usize = 0x06;
/// Offset of the Flags bitfield.
pub const OFF_FLAGS: usize = 0x07;
/// Offset of the Latency Hint field (u16, network byte order, raw).
pub const OFF_LATENCY_HINT: usize = 0x08;
/// Offset of the Deployment Ring field (exponent-encoded).
pub const OFF_DEPLOY_RING: usize = 0x0A;
/// Offset of the Mesh Flags field (exponent-encoded).
pub const OFF_MESH_FLAGS: usize = 0x0B;
/// Offset of the Src Prefix Lo field (low octet of Kingdom ULA source subnet).
pub const OFF_SRC_PREFIX_LO: usize = 0x0C;
/// Offset of the Dst Prefix Lo field (low octet of Kingdom ULA destination subnet).
pub const OFF_DST_PREFIX_LO: usize = 0x0D;
/// Offset of Scratch[0] (general-purpose exponent register 0).
pub const OFF_SCRATCH_0: usize = 0x0E;
/// Offset of Scratch[1] (general-purpose exponent register 1).
pub const OFF_SCRATCH_1: usize = 0x0F;
/// Offset of Scratch[2] (general-purpose exponent register 2).
pub const OFF_SCRATCH_2: usize = 0x10;
/// Offset of Scratch[3] (general-purpose exponent register 3).
pub const OFF_SCRATCH_3: usize = 0x11;
/// Offset of the Checksum field (CRC-16/CCITT over octets 0x00–0x11).
pub const OFF_CHECKSUM: usize = 0x12;

/// Number of bytes protected by the Monad checksum (octets 0x00 through 0x11).
pub const MONAD_CRC_DATA_LEN: usize = 18;

// ── Monad Flags bitfield ──────────────────────────────────────────────────────

/// Flags bitfield constants for [`Monad::flags`] (offset [`OFF_FLAGS`]).
pub mod flags {
    /// CHAOS (bit 7): set by Yaldabaoth chaos injection.  This packet was
    /// deliberately modified.  All downstream hops observe this flag.
    pub const CHAOS: u8 = 0x80;

    /// CANARY (bit 6): packet belongs to a canary deployment path.
    pub const CANARY: u8 = 0x40;

    /// TRACED (bit 5): every hop MUST emit a full Anamnesis event for this packet.
    pub const TRACED: u8 = 0x20;

    /// ENCRYPT (bit 4): intra-Kingdom TLS payload.
    pub const ENCRYPT: u8 = 0x10;

    /// SAMPLED (bit 3): packet selected for statistical sampling.
    pub const SAMPLED: u8 = 0x08;

    /// MIRROR (bit 2): this is a mirror copy; the original does not carry this flag.
    pub const MIRROR: u8 = 0x04;

    /// CUSTOM (bit 1): Scratch and Checksum fields carry exponent-encoded values
    /// rather than their default semantics (§BPF-Edge-Codec).
    pub const CUSTOM: u8 = 0x02;

    /// RSVD (bit 0): reserved, MUST be zero.
    pub const RSVD: u8 = 0x01;

    /// Mask of all defined flag bits.
    pub const ALL_DEFINED: u8 = CHAOS | CANARY | TRACED | ENCRYPT | SAMPLED | MIRROR | CUSTOM;
}

// ── Anamnesis event types ─────────────────────────────────────────────────────

/// Event type values emitted to per-CPU Anamnesis ring buffers.
#[repr(u8)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum EventType {
    /// Shield ingress inserted the Monad (packet birth).
    Birth   = 0x01,
    /// Intermediate eBPF program processed the packet at a Kingdom hop.
    #[default]
    Hop     = 0x02,
    /// Shield egress stripped the Monad (packet death).
    Death   = 0x03,
    /// Checksum failure, unknown Sophia key, or decode error.
    Anomaly = 0x04,
    /// Chaos injection subsystem (Yaldabaoth) modified the packet.
    Chaos   = 0x05,
}

impl EventType {
    /// Parse from raw u8.  Returns `None` for unknown values.
    #[inline]
    pub const fn from_u8(v: u8) -> Option<Self> {
        match v {
            0x01 => Some(Self::Birth),
            0x02 => Some(Self::Hop),
            0x03 => Some(Self::Death),
            0x04 => Some(Self::Anomaly),
            0x05 => Some(Self::Chaos),
            _    => None,
        }
    }
}

// ── Yaldabaoth chaos modes ────────────────────────────────────────────────────

/// Chaos injection modes for the Yaldabaoth subsystem (TC egress hook).
#[repr(u8)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum ChaosMode {
    /// XOR a random byte in Monad offsets 0x01–0x11 with a random mask.
    /// Checksum is NOT recomputed — the corruption is intentional.
    #[default]
    BitFlip      = 0x01,
    /// Use TC scheduling to add a configurable microsecond delay.
    Delay        = 0x02,
    /// Clone the packet via bpf_clone_redirect to the same interface.
    Duplicate    = 0x03,
    /// Zero out Monad octets 0x08 through 0x13 (Latency Hint through Checksum).
    Truncate     = 0x04,
    /// Set the CHAOS flag without any other modification.
    ChaosMarker  = 0x05,
}

// ── Well-known Sophia key values ──────────────────────────────────────────────

/// Common values for the exponent-encoded Circuit State field.
pub mod circuit_state {
    /// Circuit is closed — normal traffic flow.
    pub const CLOSED:    u8 = 0x01;
    /// Circuit is open — traffic is blocked (too many failures).
    pub const OPEN:      u8 = 0x02;
    /// Circuit is half-open — probe traffic allowed for recovery testing.
    pub const HALF_OPEN: u8 = 0x03;
}

/// Common values for the exponent-encoded Flow Action field.
pub mod flow_action {
    /// Forward the packet normally.
    pub const FORWARD: u8 = 0x01;
    /// Collect a full distributed trace for this packet.
    pub const TRACE:   u8 = 0x02;
    /// Select this packet for statistical sampling.
    pub const SAMPLE:  u8 = 0x03;
    /// Mirror the packet to an analysis interface.
    pub const MIRROR:  u8 = 0x04;
    /// Drop the packet at the next hop.
    pub const DROP:    u8 = 0x05;
}

/// Common values for the exponent-encoded Deployment Ring field.
pub mod deploy_ring {
    /// Production traffic — full stability guarantees.
    pub const PRODUCTION: u8 = 0x01;
    /// Canary ring — small percentage of production traffic.
    pub const CANARY:     u8 = 0x02;
    /// Staging ring — pre-production validation.
    pub const STAGING:    u8 = 0x03;
    /// Development ring — unrestricted experimentation.
    pub const DEVELOPMENT: u8 = 0x04;
}

// ── CRC-16/CCITT ─────────────────────────────────────────────────────────────

/// CRC-16/CCITT (polynomial 0x1021, initial value 0xFFFF).
///
/// Covers Monad octets 0x00 through 0x11 (18 bytes, the first 18 of the 20-byte
/// register file).  No final XOR, no input/output reflection — standard
/// CRC-16/CCITT (also known as CRC-16/IBM-3740, CRC-CCITT-FALSE).
///
/// Computation cost: ~50 ns for 18 bytes on modern x86 hardware.
pub struct CrcCcitt;

impl CrcCcitt {
    const POLY: u16 = 0x1021;
    const INIT: u16 = 0xFFFF;

    /// Compute CRC-16/CCITT over `data`.
    ///
    /// # Examples
    ///
    /// ```
    /// # use monad_common::CrcCcitt;
    /// let crc = CrcCcitt::compute(b"123456789");
    /// assert_eq!(crc, 0x29B1); // Standard CCITT check value
    /// ```
    #[inline]
    pub fn compute(data: &[u8]) -> u16 {
        let mut crc = Self::INIT;
        let mut i = 0;
        // Manual loop for no_std — cannot use Iterator in BPF context
        while i < data.len() {
            crc ^= (data[i] as u16) << 8;
            let mut j = 0;
            while j < 8 {
                if crc & 0x8000 != 0 {
                    crc = (crc << 1) ^ Self::POLY;
                } else {
                    crc <<= 1;
                }
                j += 1;
            }
            i += 1;
        }
        crc
    }

    /// Compute CRC-16/CCITT over a fixed-size 18-byte slice (the protected
    /// portion of the Monad register file).
    #[inline]
    pub fn compute_monad(data: &[u8; MONAD_CRC_DATA_LEN]) -> u16 {
        Self::compute(data)
    }
}

// ── Monad: 20-byte register file ──────────────────────────────────────────────

/// The Monad — the 20-octet register file carried in every Kingdom packet.
///
/// Layout (from draft-bellis-unheaded-protocol-foundation-01 §3.3):
///
/// ```text
///  0                   1                   2                   3
///  0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
/// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
/// |    Version    | Src Service ID| Dst Service ID|   Hop Count   |
/// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
/// |   QoS Class   |  Flow Action  | Circuit State |     Flags     |
/// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
/// |        Latency Hint           |  Deploy Ring  |  Mesh Flags   |
/// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
/// | Src Prefix Lo | Dst Prefix Lo |  Scratch [0]  |  Scratch [1]  |
/// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
/// |  Scratch [2]  |  Scratch [3]  |           Checksum            |
/// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
/// ```
///
/// All fields are stored in network byte order (big-endian).
/// `latency_hint` and `checksum` are big-endian u16.
///
/// Use [`Monad::new`] to construct with default values and correct version.
/// Use [`Monad::compute_checksum`] / [`Monad::verify_checksum`] for integrity.
/// Use [`Monad::increment_hop`] for per-hop processing.
#[repr(C)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Monad {
    /// Version (raw, not exponent-encoded).  MUST be [`MONAD_VERSION`] (0x01).
    pub version: u8,

    /// Src Service ID (exponent-encoded, Sophia `service_identity` dict).
    pub src_service_id: u8,

    /// Dst Service ID (exponent-encoded, Sophia `service_identity` dict).
    pub dst_service_id: u8,

    /// Hop Count (raw u8).  Zero at Shield ingress, incremented each hop.
    pub hop_count: u8,

    /// QoS Class (exponent-encoded).
    pub qos_class: u8,

    /// Flow Action (exponent-encoded, see [`flow_action`] mod).
    pub flow_action: u8,

    /// Circuit State (exponent-encoded, see [`circuit_state`] mod).
    pub circuit_state: u8,

    /// Flags bitfield (raw).  See [`flags`] mod for bit definitions.
    pub flags: u8,

    /// Latency Hint (raw u16, network byte order).  Upstream latency in µs.
    pub latency_hint: [u8; 2],

    /// Deployment Ring (exponent-encoded, see [`deploy_ring`] mod).
    pub deploy_ring: u8,

    /// Mesh Flags (exponent-encoded).
    pub mesh_flags: u8,

    /// Src Prefix Lo — low octet of the Kingdom ULA source subnet identifier.
    pub src_prefix_lo: u8,

    /// Dst Prefix Lo — low octet of the Kingdom ULA destination subnet identifier.
    pub dst_prefix_lo: u8,

    /// Scratch registers [0..3] — general-purpose exponent-encoded GP-regs.
    /// These are the hot registers for the Doom-over-IPv6 computational PoC:
    /// `scratch[0:1]` = accumulator pair, `scratch[2:3]` = address register pair.
    pub scratch: [u8; 4],

    /// Checksum (CRC-16/CCITT over `Monad` octets 0x00–0x11, network byte order).
    /// When [`flags::CUSTOM`] is set, these two bytes carry exponent-encoded keys.
    pub checksum: [u8; 2],
}

// Compile-time layout assertions — no runtime cost.
// These will fail to compile if the struct layout is wrong.
const _: () = {
    let m = Monad {
        version: 0, src_service_id: 0, dst_service_id: 0, hop_count: 0,
        qos_class: 0, flow_action: 0, circuit_state: 0, flags: 0,
        latency_hint: [0; 2], deploy_ring: 0, mesh_flags: 0,
        src_prefix_lo: 0, dst_prefix_lo: 0, scratch: [0; 4], checksum: [0; 2],
    };
    // Size check — must be exactly 20 bytes
    let _ = [(); MONAD_SIZE - core::mem::size_of::<Monad>()];
    let _ = [(); core::mem::size_of::<Monad>() - MONAD_SIZE];
    let _ = m;
};

impl Monad {
    /// Construct a Monad initialized for Shield ingress.
    ///
    /// Fields not provided by the caller are zeroed.  Checksum is NOT computed
    /// by this constructor — call [`Monad::recompute_checksum`] after filling in
    /// the service IDs and other fields.
    #[inline]
    pub const fn new() -> Self {
        Self {
            version:        MONAD_VERSION,
            src_service_id: 0,
            dst_service_id: 0,
            hop_count:      0,
            qos_class:      0,
            flow_action:    flow_action::FORWARD,
            circuit_state:  circuit_state::CLOSED,
            flags:          0,
            latency_hint:   [0; 2],
            deploy_ring:    deploy_ring::PRODUCTION,
            mesh_flags:     0,
            src_prefix_lo:  0,
            dst_prefix_lo:  0,
            scratch:        [0; 4],
            checksum:       [0; 2],
        }
    }

    /// Serialize the Monad to a 20-byte array.
    #[inline]
    pub const fn to_bytes(&self) -> [u8; MONAD_SIZE] {
        [
            self.version,
            self.src_service_id,
            self.dst_service_id,
            self.hop_count,
            self.qos_class,
            self.flow_action,
            self.circuit_state,
            self.flags,
            self.latency_hint[0],
            self.latency_hint[1],
            self.deploy_ring,
            self.mesh_flags,
            self.src_prefix_lo,
            self.dst_prefix_lo,
            self.scratch[0],
            self.scratch[1],
            self.scratch[2],
            self.scratch[3],
            self.checksum[0],
            self.checksum[1],
        ]
    }

    /// Deserialize a Monad from a 20-byte array.
    #[inline]
    pub const fn from_bytes(b: [u8; MONAD_SIZE]) -> Self {
        Self {
            version:        b[0],
            src_service_id: b[1],
            dst_service_id: b[2],
            hop_count:      b[3],
            qos_class:      b[4],
            flow_action:    b[5],
            circuit_state:  b[6],
            flags:          b[7],
            latency_hint:   [b[8], b[9]],
            deploy_ring:    b[10],
            mesh_flags:     b[11],
            src_prefix_lo:  b[12],
            dst_prefix_lo:  b[13],
            scratch:        [b[14], b[15], b[16], b[17]],
            checksum:       [b[18], b[19]],
        }
    }

    /// Read the Latency Hint as a native u16 (converts from network byte order).
    #[inline]
    pub const fn latency_hint_u16(&self) -> u16 {
        u16::from_be_bytes(self.latency_hint)
    }

    /// Set the Latency Hint from a native u16 (converts to network byte order).
    #[inline]
    pub fn set_latency_hint(&mut self, us: u16) {
        self.latency_hint = us.to_be_bytes();
    }

    /// Read the Checksum as a native u16 (converts from network byte order).
    #[inline]
    pub const fn checksum_u16(&self) -> u16 {
        u16::from_be_bytes(self.checksum)
    }

    /// Return the 18-byte slice protected by the CRC (offsets 0x00–0x11).
    #[inline]
    pub fn crc_region(&self) -> [u8; MONAD_CRC_DATA_LEN] {
        let b = self.to_bytes();
        let mut out = [0u8; MONAD_CRC_DATA_LEN];
        let mut i = 0;
        while i < MONAD_CRC_DATA_LEN {
            out[i] = b[i];
            i += 1;
        }
        out
    }

    /// Compute and store the CRC-16/CCITT checksum in `self.checksum`.
    ///
    /// Called after any Monad field modification.  In eBPF, called at the end
    /// of per-hop processing before returning XDP_PASS.
    #[inline]
    pub fn recompute_checksum(&mut self) {
        let region = self.crc_region();
        let crc = CrcCcitt::compute_monad(&region);
        self.checksum = crc.to_be_bytes();
    }

    /// Verify the CRC-16/CCITT checksum.  Returns `true` if valid.
    ///
    /// In eBPF, called at the start of per-hop processing before reading any
    /// Monad field.  A `false` return triggers an [`EventType::Anomaly`] event.
    ///
    /// Note: if [`flags::CUSTOM`] is set, the checksum field carries exponent
    /// keys instead; the caller is responsible for skipping this check.
    #[inline]
    pub fn verify_checksum(&self) -> bool {
        let region = self.crc_region();
        let expected = CrcCcitt::compute_monad(&region);
        expected == self.checksum_u16()
    }

    /// Increment the Hop Count, saturating at 255.
    ///
    /// Called at each Kingdom hop after checksum verification and before
    /// component-specific logic.  Recompute the checksum after.
    #[inline]
    pub fn increment_hop(&mut self) {
        self.hop_count = self.hop_count.saturating_add(1);
    }

    /// Test whether a flag bit is set.  Use constants from the [`flags`] module.
    #[inline]
    pub const fn has_flag(&self, bit: u8) -> bool {
        self.flags & bit != 0
    }

    /// Set a flag bit.
    #[inline]
    pub fn set_flag(&mut self, bit: u8) {
        self.flags |= bit;
    }

    /// Clear a flag bit.
    #[inline]
    pub fn clear_flag(&mut self, bit: u8) {
        self.flags &= !bit;
    }

    /// Read Scratch[0:1] as a big-endian u16 accumulator pair.
    /// Used by the Doom-over-IPv6 PoC CPU program.
    #[inline]
    pub const fn scratch_r0(&self) -> u16 {
        u16::from_be_bytes([self.scratch[0], self.scratch[1]])
    }

    /// Write Scratch[0:1] as a big-endian u16.
    #[inline]
    pub fn set_scratch_r0(&mut self, v: u16) {
        let b = v.to_be_bytes();
        self.scratch[0] = b[0];
        self.scratch[1] = b[1];
    }

    /// Read Scratch[2:3] as a big-endian u16 address register pair.
    #[inline]
    pub const fn scratch_r1(&self) -> u16 {
        u16::from_be_bytes([self.scratch[2], self.scratch[3]])
    }

    /// Write Scratch[2:3] as a big-endian u16.
    #[inline]
    pub fn set_scratch_r1(&mut self, v: u16) {
        let b = v.to_be_bytes();
        self.scratch[2] = b[0];
        self.scratch[3] = b[1];
    }

    /// Returns `true` if this Monad has a valid version and default flags.
    #[inline]
    pub const fn is_valid(&self) -> bool {
        self.version == MONAD_VERSION
        && (self.flags & flags::RSVD) == 0
    }
}

// ── Hop-by-Hop Options Header carrying the Monad ────────────────────────────

/// IPv6 Hop-by-Hop Options Header containing the Monad option (24 octets total).
///
/// Structure:
/// ```text
/// Octet 0:   Next Header (original transport protocol, e.g., 6=TCP, 17=UDP)
/// Octet 1:   Hdr Ext Len = 2  (2 additional 8-octet units beyond first 8)
/// Octet 2:   Opt Type = MONAD_OPT_TYPE (0x3E)
/// Octet 3:   Opt Data Len = 20
/// Octets 4–23: Monad register file (20 bytes)
/// ```
///
/// This header is inserted by Shield XDP ingress and stripped by Shield TC egress.
#[repr(C)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct HopByHopHeader {
    /// Next Header — the original transport protocol number (saved from IPv6 fixed header).
    pub next_header: u8,
    /// Hdr Ext Len — MUST be [`HBH_HDR_EXT_LEN`] (2).
    pub hdr_ext_len: u8,
    /// Option Type — MUST be [`MONAD_OPT_TYPE`] (0x3E).
    pub opt_type: u8,
    /// Option Data Length — MUST be [`MONAD_OPT_DATA_LEN`] (20).
    pub opt_data_len: u8,
    /// The Monad register file (20 bytes of option data).
    pub monad: Monad,
}

const _: () = {
    let _ = [(); HBH_TOTAL_LEN - core::mem::size_of::<HopByHopHeader>()];
    let _ = [(); core::mem::size_of::<HopByHopHeader>() - HBH_TOTAL_LEN];
};

impl HopByHopHeader {
    /// Construct a default HBH header for Shield ingress insertion.
    #[inline]
    pub const fn new(next_header: u8) -> Self {
        Self {
            next_header,
            hdr_ext_len:  HBH_HDR_EXT_LEN,
            opt_type:     MONAD_OPT_TYPE,
            opt_data_len: MONAD_OPT_DATA_LEN,
            monad:        Monad::new(),
        }
    }

    /// Serialize to 24 bytes.
    #[inline]
    pub fn to_bytes(&self) -> [u8; HBH_TOTAL_LEN] {
        let mut out = [0u8; HBH_TOTAL_LEN];
        out[0] = self.next_header;
        out[1] = self.hdr_ext_len;
        out[2] = self.opt_type;
        out[3] = self.opt_data_len;
        let m = self.monad.to_bytes();
        let mut i = 0;
        while i < MONAD_SIZE {
            out[4 + i] = m[i];
            i += 1;
        }
        out
    }
}

// ── Anamnesis Event ──────────────────────────────────────────────────────────

/// Anamnesis event emitted to per-CPU ring buffers by every eBPF interaction.
///
/// Exactly 32 bytes — fits within a single x86 cache line.
///
/// Structure (from draft-bellis §6.1):
/// ```text
/// [0..8]:   timestamp_ns  — bpf_ktime_get_ns()
/// [8]:      event_type    — EventType enum value
/// [9]:      hop_id        — which hop emitted this event (Sophia-assigned)
/// [10..12]: flow_label_lo — low 16 bits of IPv6 Flow Label (trace correlation)
/// [12..32]: monad         — full 20-byte Monad snapshot
/// ```
///
/// The Flow Label from the IPv6 fixed header is the primary correlation key:
/// all Anamnesis events sharing a Flow Label belong to the same traced flow.
/// Head-based sampling uses `flow_label % divisor`.
#[repr(C)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct AnamnesisEvent {
    /// Kernel timestamp at time of event (bpf_ktime_get_ns, nanoseconds).
    pub timestamp_ns: u64,

    /// Event type (Birth, Hop, Death, Anomaly, Chaos).
    pub event_type: u8,

    /// Hop ID — the Sophia-assigned identifier of the hop that emitted this event.
    pub hop_id: u8,

    /// Low 16 bits of the IPv6 Flow Label (trace correlation key).
    /// Full 20-bit flow label: `(flow_label_hi_nibble << 16) | u16::from_be_bytes(flow_label_lo)`.
    pub flow_label_lo: [u8; 2],

    /// Complete 20-byte Monad snapshot at the time of the event.
    pub monad: Monad,
}

const _: () = {
    let _ = [(); 32 - core::mem::size_of::<AnamnesisEvent>()];
    let _ = [(); core::mem::size_of::<AnamnesisEvent>() - 32];
};

impl AnamnesisEvent {
    /// Construct an Anamnesis event.
    #[inline]
    pub const fn new(
        timestamp_ns: u64,
        event_type:   EventType,
        hop_id:       u8,
        flow_label:   u32,
        monad:        Monad,
    ) -> Self {
        let flow_lo = (flow_label & 0xFFFF) as u16;
        Self {
            timestamp_ns,
            event_type: event_type as u8,
            hop_id,
            flow_label_lo: flow_lo.to_be_bytes(),
            monad,
        }
    }

    /// Read the flow label low 16 bits as a native u16.
    #[inline]
    pub const fn flow_label_u16(&self) -> u16 {
        u16::from_be_bytes(self.flow_label_lo)
    }

    /// Head-based sampling decision.  Returns `true` if this flow should be
    /// sampled, given a `divisor` (e.g., 1 = 100%, 10 = ~10%).
    #[inline]
    pub const fn should_sample(&self, divisor: u16) -> bool {
        if divisor == 0 {
            return false;
        }
        self.flow_label_u16() % divisor == 0
    }
}

// ── Sophia BPF map entry ─────────────────────────────────────────────────────

/// Value type for Sophia lookup dictionaries (BPF_MAP_TYPE_HASH, key u8).
///
/// Each exponent-encoded field in the Monad is a u8 key into a Sophia map.
/// The map returns a [`SophiaEntry`] describing the key's semantics.
///
/// BPF map definition (kernel side):
/// ```c
/// struct {
///     __uint(type, BPF_MAP_TYPE_HASH);
///     __uint(max_entries, 256);
///     __type(key, u8);
///     __type(value, struct sophia_entry);
/// } sophia_service_identity SEC(".maps");
/// ```
#[repr(C)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct SophiaEntry {
    /// Sub-dictionary identifier for compositional lookup chains (0 = leaf).
    pub dict_id: u8,
    /// Reserved for alignment and future use.
    pub _pad: [u8; 1],
    /// Numeric metadata (e.g., port number, weight, priority).
    pub meta: u16,
    /// Opaque extended value (endpoint address, policy ID, etc.).
    pub value: u32,
    /// Human-readable name for dashboard display (null-terminated, ASCII).
    pub name: [u8; 24],
}

const _: () = {
    let _ = [(); 32 - core::mem::size_of::<SophiaEntry>()];
    let _ = [(); core::mem::size_of::<SophiaEntry>() - 32];
};

// ── CPU State & Compute Events (Doom-over-IPv6 PoC) ───────────────────────

/// CPU state for a single MBC compute flow, keyed by IPv6 flow label.
/// Stored in BPF cpu_map (BPF_MAP_TYPE_HASH, max=256 flows).
/// Persists across packet hops — this IS the CPU register file.
#[repr(C)]
#[derive(Clone, Copy, Default)]
pub struct CpuState {
    /// General purpose registers r0–r15 (32-bit each).  r15 = SP.
    pub regs: [u32; 16],
    /// Program counter (index into rom_map).
    pub pc: u32,
    /// Status flags: bit 0=Z, bit 1=N, bit 2=C.
    pub flags: u8,
    /// 1 if waiting for cache miss to be serviced.
    pub stalled: u8,
    /// 1 if HALT syscall executed.
    pub halted: u8,
    /// Padding byte for alignment.
    pub _pad: u8,
    /// bpf_ktime_get_ns() value, sleep if < now.
    pub sleep_until: u64,
    /// Total instructions executed (for IPS stats).
    pub insn_count: u64,
    /// L1 cache hits.
    pub cache_hits: u64,
    /// L1 cache misses.
    pub cache_misses: u64,
}

/// Keyboard state written by Wotan, read by BPF SYSCALL 0x02.
/// Stored in kbd_map (BPF_MAP_TYPE_ARRAY, index 0).
#[repr(C)]
#[derive(Clone, Copy, Default)]
pub struct KeyboardState {
    /// Keycode.
    pub key: u32,
    /// 1=pressed, 0=released.
    pub pressed: u32,
    /// Monotonically increasing, BPF checks if changed.
    pub sequence: u64,
}

/// CPU state flag bit positions.
pub mod cpu_flags {
    /// Zero flag (Z): set when the result of the last ALU op was zero.
    pub const ZERO: u8 = 0b001;
    /// Negative flag (N): set when the MSB of the last result is 1.
    pub const NEGATIVE: u8 = 0b010;
    /// Carry flag (C): set on unsigned overflow/borrow.
    pub const CARRY: u8 = 0b100;
}

/// Register aliases for CpuState.
pub const REG_SP: usize = 15;
pub const REG_PC_DEFAULT: u32 = 0;
pub const REG_SP_DEFAULT: u32 = 0xFFFF_0000;

// ── Compute engine Anamnesis event types ────────────────────────────────────

/// Compute engine event type: one MBC instruction executed.
pub const EVENT_COMPUTE_HOP: u8 = 0x10;
/// Compute engine event type: L1 cache miss, addr emitted to Wotan.
pub const EVENT_CACHE_MISS: u8 = 0x11;
/// Compute engine event type: dirty page written, Wotan flushes to L2.
pub const EVENT_MEM_WRITE: u8 = 0x12;
/// Compute engine event type: Wotan staged a page into L1.
pub const EVENT_MEM_STAGED: u8 = 0x13;
/// Compute engine event type: SYSCALL DG_DrawFrame emitted.
pub const EVENT_SCREEN_WRITE: u8 = 0x14;
/// Compute engine event type: SYSCALL DG_GetKey executed.
pub const EVENT_KEY_READ: u8 = 0x15;
/// Compute engine event type: HALT syscall.
pub const EVENT_COMPUTE_HALT: u8 = 0x16;
/// Compute engine event type: stall (cache miss, sleep).
pub const EVENT_COMPUTE_STALL: u8 = 0x17;

/// Emitted by monad-cpu-ebpf via BPF ring buffer on every instruction executed.
/// Also used for CACHE_MISS events (with instruction=0).
/// Total size: 8+1+1+2+4+4+4+(16*4)+1+1+4 = 96 bytes
#[repr(C)]
#[derive(Clone, Copy, Default)]
pub struct ComputeHopEvent {
    /// Kernel timestamp at time of event (bpf_ktime_get_ns, nanoseconds).
    pub timestamp_ns: u64,
    /// Event type (EVENT_COMPUTE_HOP, EVENT_CACHE_MISS, etc.).
    pub event_type: u8,
    /// Which namespace/hop (0-5 for 6-hop ring).
    pub hop_id: u8,
    /// Padding for alignment.
    pub _pad: [u8; 2],
    /// IPv6 flow label.
    pub flow_label: u32,
    /// Program counter.
    pub pc: u32,
    /// Raw MBC instruction word (0 for CACHE_MISS).
    pub instruction: u32,
    /// Full register snapshot at time of event.
    pub regs: [u32; 16],
    /// CPU flags.
    pub flags: u8,
    /// 1=L1 hit, 0=miss (for stats).
    pub cache_hit: u8,
    /// For CACHE_MISS events: the address that missed.
    pub miss_addr: u32,
}

/// Emitted by monad-cpu-ebpf for MEM_WRITE events (dirty writeback to Wotan L2).
#[repr(C)]
#[derive(Clone, Copy)]
pub struct MemWriteEvent {
    /// Kernel timestamp at time of event (bpf_ktime_get_ns, nanoseconds).
    pub timestamp_ns: u64,
    /// Event type (EVENT_MEM_WRITE).
    pub event_type: u8,
    /// Padding for alignment.
    pub _pad: [u8; 3],
    /// IPv6 flow label.
    pub flow_label: u32,
    /// Memory address being written.
    pub addr: u32,
    /// 1=byte, 2=halfword, 4=word.
    pub size: u8,
    /// Padding for alignment.
    pub _pad2: [u8; 3],
    /// Value being written.
    pub value: u32,
}

// ── MBC CPU state (Doom-over-IPv6 PoC) ───────────────────────────────────────

/// General-purpose registers for the MBC ISA VM.
///
/// 16 registers, 32-bit.  r15 is the stack pointer (SP alias).
/// Carried in a Sophia BPF hash map keyed by IPv6 Flow Label,
/// providing per-flow isolated execution contexts.
pub const MBC_REG_COUNT: usize = 16;

/// CPU state for the Monad Bytecode (MBC) virtual machine.
///
/// Stored in `cpu_map: BPF_MAP_TYPE_HASH<u32 (flow_label), MbcCpuState>`.
/// Each Kingdom hop reads this state, executes one instruction, and writes it back.
///
/// Includes L1 cache statistics for memory hierarchy visibility.
/// See doom-over-ipv6-plan.md Phase D3/D4 for the full execution model.
#[repr(C)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct MbcCpuState {
    /// General purpose registers r0–r15 (32-bit each).  r15 = SP.
    pub regs: [u32; MBC_REG_COUNT],
    /// Program counter — index into `rom_map`.
    pub pc: u32,
    /// Status flags: bit 0=Zero (Z), bit 1=Negative (N), bit 2=Carry (C).
    pub flags: u8,
    /// `1` when the CPU has executed a HALT instruction.
    pub halted: u8,
    /// `1` when waiting for cache miss to be serviced by Wotan (D-003).
    pub stalled: u8,
    /// Padding byte for alignment.
    pub _pad: u8,
    /// "Sleep until" timestamp (bpf_ktime_get_ns, nanoseconds).
    /// When non-zero, the CPU skips execution until this time is reached.
    pub sleep_until_ns: u64,
    /// Total instructions executed (for IPS stats).
    pub insn_count: u64,
    /// L1 cache hits.
    pub cache_hits: u64,
    /// L1 cache misses.
    pub cache_misses: u64,
}

/// MBC CPU flags — stored in `MbcCpuState.flags`.
pub mod mbc_flags {
    /// Zero flag (Z): set when the result of the last ALU op was zero.
    pub const Z: u8 = 0x01;
    /// Negative flag (N): set when the MSB of the last result is 1.
    pub const N: u8 = 0x02;
    /// Carry flag (C): set on unsigned overflow/borrow.
    pub const C: u8 = 0x04;
}

/// MBC instruction word (32-bit fixed-width encoding).
///
/// Encoding format:
/// ```text
/// [31..24]: opcode (u8)
/// [23..20]: dst register (4 bits)
/// [19..16]: src register (4 bits)
/// [15..0]:  immediate / address / branch offset (u16)
/// ```
///
/// For branch instructions, the immediate is a signed 16-bit PC-relative offset.
/// For load/store, it is a 16-bit base address.
#[repr(transparent)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct MbcInsn(pub u32);

impl MbcInsn {
    /// Decode the opcode field (bits 31–24).
    #[inline]
    pub const fn opcode(&self) -> u8 {
        ((self.0 >> 24) & 0xFF) as u8
    }

    /// Decode the destination register (bits 23–20).
    #[inline]
    pub const fn dst(&self) -> u8 {
        ((self.0 >> 20) & 0x0F) as u8
    }

    /// Decode the source register (bits 19–16).
    #[inline]
    pub const fn src(&self) -> u8 {
        ((self.0 >> 16) & 0x0F) as u8
    }

    /// Decode the immediate / address / offset (bits 15–0).
    #[inline]
    pub const fn imm16(&self) -> u16 {
        (self.0 & 0xFFFF) as u16
    }

    /// Decode the immediate as a signed 16-bit branch offset.
    #[inline]
    pub const fn imm16_signed(&self) -> i16 {
        self.imm16() as i16
    }

    /// Encode a new instruction word.
    #[inline]
    pub const fn encode(opcode: u8, dst: u8, src: u8, imm: u16) -> Self {
        Self(
            ((opcode as u32) << 24)
            | (((dst & 0x0F) as u32) << 20)
            | (((src & 0x0F) as u32) << 16)
            | (imm as u32),
        )
    }
}

/// MBC opcode values.
pub mod mbc_opcodes {
    // ── Arithmetic ────────────────────────────────────────────
    /// `dst = dst + src`
    pub const ADD:  u8 = 0x01;
    /// `dst = dst - src`
    pub const SUB:  u8 = 0x02;
    /// `dst = dst * src`
    pub const MUL:  u8 = 0x03;
    /// `dst = dst / src`  (unsigned)
    pub const DIV:  u8 = 0x04;
    /// `dst = dst % src`  (unsigned)
    pub const MOD:  u8 = 0x05;
    /// `dst = -dst`
    pub const NEG:  u8 = 0x06;

    // ── Logical / bitwise ─────────────────────────────────────
    /// `dst = dst & src`
    pub const AND:  u8 = 0x07;
    /// `dst = dst | src`
    pub const OR:   u8 = 0x08;
    /// `dst = dst ^ src`
    pub const XOR:  u8 = 0x09;
    /// `dst = ~dst`
    pub const NOT:  u8 = 0x0A;
    /// `dst = dst << imm16`
    pub const SHL:  u8 = 0x0B;
    /// `dst = dst >> imm16`  (logical)
    pub const SHR:  u8 = 0x0C;
    /// `dst = dst >> imm16`  (arithmetic, sign-extending)
    pub const SAR:  u8 = 0x0D;

    // ── Register operations ────────────────────────────────────
    /// `dst = src`
    pub const MOV:  u8 = 0x0E;
    /// `dst = zero_extend(imm16)`
    pub const MOVI: u8 = 0x0F;

    // ── Comparison ─────────────────────────────────────────────
    /// `flags = cmp(dst, src)` (sets Z, N, C without storing result)
    pub const CMP:  u8 = 0x10;

    // ── Branch ─────────────────────────────────────────────────
    /// `pc += imm16_signed` (unconditional)
    pub const JMP:  u8 = 0x20;
    /// `if Z: pc += imm16_signed`
    pub const JZ:   u8 = 0x21;
    /// `if !Z: pc += imm16_signed`
    pub const JNZ:  u8 = 0x22;
    /// `if N: pc += imm16_signed`
    pub const JN:   u8 = 0x23;
    /// `if !N: pc += imm16_signed`
    pub const JP:   u8 = 0x24;
    /// `if C: pc += imm16_signed`
    pub const JC:   u8 = 0x25;
    /// `if !C: pc += imm16_signed`
    pub const JNC:  u8 = 0x26;
    /// Push PC+1 to stack, then `pc = imm16`
    pub const CALL: u8 = 0x27;
    /// Pop PC from stack
    pub const RET:  u8 = 0x28;

    // ── Memory ─────────────────────────────────────────────────
    /// `dst = ram_map[src + imm16]`  (32-bit load)
    pub const LD:   u8 = 0x30;
    /// `ram_map[dst + imm16] = src`  (32-bit store)
    pub const ST:   u8 = 0x31;
    /// `dst = zero_extend(ram_map[src + imm16])`  (byte load)
    pub const LDB:  u8 = 0x32;
    /// `ram_map[dst + imm16] = src & 0xFF`  (byte store)
    pub const STB:  u8 = 0x33;
    /// `dst = zero_extend(ram_map[src + imm16])` (16-bit load)
    pub const LDH:  u8 = 0x34;
    /// `ram_map[dst + imm16] = src & 0xFFFF`  (16-bit store)
    pub const STH:  u8 = 0x35;

    // ── System ─────────────────────────────────────────────────
    /// Invoke I/O callback.  `imm16` = syscall number (see `mbc_syscalls`).
    pub const SYSCALL: u8 = 0x40;
    /// Halt execution.  Sets `MbcCpuState.halted = 1`.
    pub const HALT:    u8 = 0xFF;
}

/// MBC syscall numbers (value in `r0` at time of SYSCALL instruction).
pub mod mbc_syscalls {
    /// `DG_DrawFrame()` — copy pixel buffer from ram_map to screen_map.
    /// `r1` = pointer to pixel buffer in ram_map address space.
    pub const SYS_DRAW_FRAME: u32 = 0x01;
    /// `DG_GetKey()` — read from kbd_map.
    /// Returns: `r0` = key scancode, `r1` = 1 if pressed / 0 if released.
    pub const SYS_GET_KEY:    u32 = 0x02;
    /// `DG_GetTicksMs()` — `r0 = bpf_ktime_get_ns() / 1_000_000`.
    pub const SYS_GET_TICKS:  u32 = 0x03;
    /// `DG_SleepMs(ms)` — set `sleep_until = now + r0 * 1_000_000`.
    pub const SYS_SLEEP:      u32 = 0x04;
}

/// Screen / I/O memory map constants for the Doom-over-IPv6 PoC.
pub mod mbc_mmap {
    /// Base address of general RAM in `ram_map`.
    pub const RAM_BASE:    u32 = 0x0000_0000;
    /// Size of general RAM (0xC000 = 49152 bytes = 48 KiB).
    pub const RAM_SIZE:    u32 = 0x0000_C000;
    /// Base address of the screen I/O region in `ram_map`.
    /// Maps to `screen_map[0..64000]` (320x200 pixels, 8-bit palette indices).
    pub const SCREEN_BASE: u32 = 0x0000_C000;
    /// Size of screen I/O region (320×200 = 64000 bytes).
    pub const SCREEN_SIZE: u32 = 64_000;
    /// Address of the keyboard I/O word (1 u32: `scancode << 1 | pressed`).
    pub const KBD_ADDR:    u32 = 0x0000_FFFF;
    /// Base address of the WAD data in `ram_map` (loaded by Wotan).
    pub const WAD_BASE:    u32 = 0x0001_0000;
    /// Maximum WAD size (4 MB — sufficient for doom1.wad).
    pub const WAD_MAX_SIZE: u32 = 4 * 1024 * 1024;
}

// ── Ring buffer sizing helpers ────────────────────────────────────────────────

/// Required Anamnesis ring buffer size (bytes) for given traffic parameters.
///
/// Formula from draft-bellis §6.3:
/// `M_ring = R × S × E × T_hot`
///
/// where:
/// - `packets_per_sec` = packet rate (pps)
/// - `sampling_num / sampling_den` = sampling fraction (e.g., 1/1 = 100%)
/// - `hot_retention_sec` = hot retention time in ring buffer (seconds)
///
/// Returns the size in bytes, rounded up to the nearest power of two
/// (required by the BPF ring buffer allocator).
pub const fn anamnesis_ring_size(
    packets_per_sec:  u64,
    sampling_num:     u64,
    sampling_den:     u64,
    hot_retention_sec: u64,
) -> u64 {
    if sampling_den == 0 {
        return 0;
    }
    let raw = packets_per_sec
        .saturating_mul(sampling_num)
        .saturating_div(sampling_den)
        .saturating_mul(core::mem::size_of::<AnamnesisEvent>() as u64)
        .saturating_mul(hot_retention_sec);
    // Round up to next power of two
    let mut p = 1u64;
    while p < raw {
        p = match p.checked_mul(2) {
            Some(v) => v,
            None    => return u64::MAX,
        };
    }
    p
}

// ═══════════════════════════════════════════════════════════════════════════
// Tests — compiled for host targets only (`cargo test -p monad-common`)
// ═══════════════════════════════════════════════════════════════════════════

#[cfg(test)]
mod tests {
    use super::*;

    // ── Size and layout assertions ───────────────────────────────────────────

    #[test]
    fn monad_is_exactly_20_bytes() {
        assert_eq!(core::mem::size_of::<Monad>(), MONAD_SIZE);
    }

    #[test]
    fn hop_by_hop_header_is_exactly_24_bytes() {
        assert_eq!(core::mem::size_of::<HopByHopHeader>(), HBH_TOTAL_LEN);
    }

    #[test]
    fn anamnesis_event_is_exactly_32_bytes() {
        assert_eq!(core::mem::size_of::<AnamnesisEvent>(), 32);
    }

    #[test]
    fn sophia_entry_is_exactly_32_bytes() {
        assert_eq!(core::mem::size_of::<SophiaEntry>(), 32);
    }

    #[test]
    fn mbc_cpu_state_is_aligned() {
        // Not a fixed-size contract, but should be 8-byte aligned
        assert_eq!(core::mem::size_of::<MbcCpuState>() % 8, 0);
    }

    // ── Monad field offsets ──────────────────────────────────────────────────

    #[test]
    fn monad_field_offsets_match_spec() {
        let m = Monad::new();
        let base = &m as *const Monad as usize;

        macro_rules! off {
            ($field:ident) => {
                ((&m.$field as *const _ as usize) - base)
            };
        }

        assert_eq!(off!(version),        OFF_VERSION,        "version");
        assert_eq!(off!(src_service_id), OFF_SRC_SERVICE_ID, "src_service_id");
        assert_eq!(off!(dst_service_id), OFF_DST_SERVICE_ID, "dst_service_id");
        assert_eq!(off!(hop_count),      OFF_HOP_COUNT,      "hop_count");
        assert_eq!(off!(qos_class),      OFF_QOS_CLASS,      "qos_class");
        assert_eq!(off!(flow_action),    OFF_FLOW_ACTION,    "flow_action");
        assert_eq!(off!(circuit_state),  OFF_CIRCUIT_STATE,  "circuit_state");
        assert_eq!(off!(flags),          OFF_FLAGS,          "flags");
        assert_eq!(off!(latency_hint),   OFF_LATENCY_HINT,   "latency_hint");
        assert_eq!(off!(deploy_ring),    OFF_DEPLOY_RING,    "deploy_ring");
        assert_eq!(off!(mesh_flags),     OFF_MESH_FLAGS,     "mesh_flags");
        assert_eq!(off!(src_prefix_lo),  OFF_SRC_PREFIX_LO,  "src_prefix_lo");
        assert_eq!(off!(dst_prefix_lo),  OFF_DST_PREFIX_LO,  "dst_prefix_lo");
        assert_eq!(off!(scratch),        OFF_SCRATCH_0,      "scratch[0]");
        assert_eq!(off!(checksum),       OFF_CHECKSUM,       "checksum");
    }

    #[test]
    fn monad_bytes_roundtrip() {
        let m = Monad {
            version:        0x01,
            src_service_id: 0x02,
            dst_service_id: 0x03,
            hop_count:      0x05,
            qos_class:      0x06,
            flow_action:    flow_action::TRACE,
            circuit_state:  circuit_state::CLOSED,
            flags:          flags::TRACED | flags::SAMPLED,
            latency_hint:   250u16.to_be_bytes(),
            deploy_ring:    deploy_ring::CANARY,
            mesh_flags:     0x00,
            src_prefix_lo:  0x0A,
            dst_prefix_lo:  0x0B,
            scratch:        [0xDE, 0xAD, 0xBE, 0xEF],
            checksum:       [0; 2],
        };
        let bytes = m.to_bytes();
        let restored = Monad::from_bytes(bytes);
        assert_eq!(m, restored);
    }

    #[test]
    fn monad_bytes_layout_exact() {
        let m = Monad {
            version:        0x01,
            src_service_id: 0x0A,
            dst_service_id: 0x0B,
            hop_count:      0x03,
            qos_class:      0x01,
            flow_action:    0x01,
            circuit_state:  0x01,
            flags:          0x20,
            latency_hint:   [0x00, 0x64], // 100 µs
            deploy_ring:    0x01,
            mesh_flags:     0x00,
            src_prefix_lo:  0x10,
            dst_prefix_lo:  0x20,
            scratch:        [0x11, 0x22, 0x33, 0x44],
            checksum:       [0xAB, 0xCD],
        };
        let b = m.to_bytes();
        assert_eq!(b[OFF_VERSION],        0x01, "version");
        assert_eq!(b[OFF_SRC_SERVICE_ID], 0x0A, "src_service_id");
        assert_eq!(b[OFF_DST_SERVICE_ID], 0x0B, "dst_service_id");
        assert_eq!(b[OFF_HOP_COUNT],      0x03, "hop_count");
        assert_eq!(b[OFF_QOS_CLASS],      0x01, "qos_class");
        assert_eq!(b[OFF_FLOW_ACTION],    0x01, "flow_action");
        assert_eq!(b[OFF_CIRCUIT_STATE],  0x01, "circuit_state");
        assert_eq!(b[OFF_FLAGS],          0x20, "flags");
        assert_eq!(b[OFF_LATENCY_HINT],   0x00, "latency_hint[0]");
        assert_eq!(b[OFF_LATENCY_HINT+1], 0x64, "latency_hint[1]");
        assert_eq!(b[OFF_DEPLOY_RING],    0x01, "deploy_ring");
        assert_eq!(b[OFF_MESH_FLAGS],     0x00, "mesh_flags");
        assert_eq!(b[OFF_SRC_PREFIX_LO],  0x10, "src_prefix_lo");
        assert_eq!(b[OFF_DST_PREFIX_LO],  0x20, "dst_prefix_lo");
        assert_eq!(b[OFF_SCRATCH_0],      0x11, "scratch[0]");
        assert_eq!(b[OFF_SCRATCH_0+1],    0x22, "scratch[1]");
        assert_eq!(b[OFF_SCRATCH_0+2],    0x33, "scratch[2]");
        assert_eq!(b[OFF_SCRATCH_0+3],    0x44, "scratch[3]");
        assert_eq!(b[OFF_CHECKSUM],       0xAB, "checksum[0]");
        assert_eq!(b[OFF_CHECKSUM+1],     0xCD, "checksum[1]");
    }

    // ── CRC-16/CCITT ─────────────────────────────────────────────────────────

    #[test]
    fn crc_ccitt_standard_check_value() {
        // The CRC-CCITT check value for "123456789" is 0x29B1.
        // This is the canonical test vector for CRC-16/IBM-3740.
        let crc = CrcCcitt::compute(b"123456789");
        assert_eq!(crc, 0x29B1, "CRC-CCITT check value mismatch");
    }

    #[test]
    fn crc_ccitt_empty_input_is_init_value() {
        // CRC of empty input is the initial value (0xFFFF).
        let crc = CrcCcitt::compute(b"");
        assert_eq!(crc, 0xFFFF);
    }

    #[test]
    fn crc_ccitt_single_zero_byte() {
        let crc = CrcCcitt::compute(&[0x00]);
        assert_eq!(crc, 0xE1F0);
    }

    #[test]
    fn crc_ccitt_all_zeros_18_bytes() {
        let data = [0u8; 18];
        let crc = CrcCcitt::compute(&data);
        // Deterministic — just verify it's not 0 or 0xFFFF
        assert_ne!(crc, 0x0000);
        assert_ne!(crc, 0xFFFF);
    }

    #[test]
    fn crc_ccitt_single_bit_flip_detected() {
        let data = [0x01u8, 0x02, 0x03, 0x04, 0x05, 0x06,
                    0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C,
                    0x0D, 0x0E, 0x0F, 0x10, 0x11, 0x12];
        let original = CrcCcitt::compute(&data);
        // Flip a single bit in byte 7
        let mut corrupted = data;
        corrupted[7] ^= 0x01;
        let flipped = CrcCcitt::compute(&corrupted);
        assert_ne!(original, flipped, "Single-bit flip must be detected");
    }

    #[test]
    fn crc_ccitt_any_double_bit_flip_detected() {
        let data: [u8; 18] = [
            0xAB, 0xCD, 0xEF, 0x01, 0x23, 0x45,
            0x67, 0x89, 0x00, 0xFF, 0x10, 0x20,
            0x30, 0x40, 0x50, 0x60, 0x70, 0x80,
        ];
        let original = CrcCcitt::compute(&data);
        // Flip two bits in the same byte
        let mut corrupted = data;
        corrupted[5] ^= 0x03; // 2 bits
        let flipped = CrcCcitt::compute(&corrupted);
        assert_ne!(original, flipped, "Double-bit flip must be detected");
    }

    // ── Monad checksum integration ─────────────────────────────────────────

    #[test]
    fn monad_recompute_and_verify_checksum() {
        let mut m = Monad::new();
        m.src_service_id = 0x42;
        m.dst_service_id = 0x07;
        m.hop_count = 3;
        m.qos_class = 0x01;
        m.set_latency_hint(512);
        m.scratch[0] = 0xDE;
        m.scratch[1] = 0xAD;

        m.recompute_checksum();
        assert!(m.verify_checksum(), "Freshly computed checksum must verify");
        assert_ne!(m.checksum, [0; 2], "Checksum must not be all-zeros");
    }

    #[test]
    fn monad_verify_detects_field_corruption() {
        let mut m = Monad::new();
        m.src_service_id = 0x10;
        m.recompute_checksum();
        assert!(m.verify_checksum());

        // Corrupt one field
        m.dst_service_id ^= 0x01;
        assert!(!m.verify_checksum(), "Checksum must fail after field corruption");
    }

    #[test]
    fn monad_increment_hop_saturates_at_255() {
        let mut m = Monad::new();
        m.hop_count = 254;
        m.increment_hop();
        assert_eq!(m.hop_count, 255);
        m.increment_hop();
        assert_eq!(m.hop_count, 255, "Must saturate at 255");
    }

    #[test]
    fn monad_increment_hop_does_not_alter_checksum_field() {
        let mut m = Monad::new();
        m.recompute_checksum();
        let ck_before = m.checksum;
        m.increment_hop();
        // Checksum field is NOT auto-updated by increment_hop — caller must
        // call recompute_checksum() explicitly.
        assert_eq!(m.checksum, ck_before);
    }

    #[test]
    fn monad_flags_set_clear() {
        let mut m = Monad::new();
        assert!(!m.has_flag(flags::TRACED));

        m.set_flag(flags::TRACED);
        assert!(m.has_flag(flags::TRACED));
        assert!(!m.has_flag(flags::CHAOS));

        m.set_flag(flags::CHAOS);
        assert!(m.has_flag(flags::CHAOS));
        assert!(m.has_flag(flags::TRACED)); // TRACED still set

        m.clear_flag(flags::TRACED);
        assert!(!m.has_flag(flags::TRACED));
        assert!(m.has_flag(flags::CHAOS));
    }

    #[test]
    fn monad_flags_all_defined_bits_do_not_overlap() {
        // Each flag is an independent bit
        let bits = [
            flags::CHAOS, flags::CANARY, flags::TRACED, flags::ENCRYPT,
            flags::SAMPLED, flags::MIRROR, flags::CUSTOM,
        ];
        let mut seen: u8 = 0;
        for &b in &bits {
            assert_eq!(b.count_ones(), 1, "Each flag must be a single bit");
            assert_eq!(seen & b, 0, "Flag bits must not overlap: {:#04x}", b);
            seen |= b;
        }
    }

    #[test]
    fn monad_is_valid_default() {
        let m = Monad::new();
        assert!(m.is_valid());
    }

    #[test]
    fn monad_is_invalid_with_rsvd_set() {
        let mut m = Monad::new();
        m.flags |= flags::RSVD;
        assert!(!m.is_valid());
    }

    #[test]
    fn monad_latency_hint_roundtrip() {
        let mut m = Monad::new();
        m.set_latency_hint(1337);
        assert_eq!(m.latency_hint_u16(), 1337);
    }

    #[test]
    fn monad_latency_hint_max_value() {
        let mut m = Monad::new();
        m.set_latency_hint(u16::MAX);
        assert_eq!(m.latency_hint_u16(), u16::MAX);
    }

    #[test]
    fn monad_scratch_r0_r1_roundtrip() {
        let mut m = Monad::new();
        m.set_scratch_r0(0xBEEF);
        m.set_scratch_r1(0xDEAD);
        assert_eq!(m.scratch_r0(), 0xBEEF);
        assert_eq!(m.scratch_r1(), 0xDEAD);
    }

    #[test]
    fn monad_scratch_registers_are_independent() {
        let mut m = Monad::new();
        m.set_scratch_r0(0xAAAA);
        m.set_scratch_r1(0x5555);
        assert_eq!(m.scratch_r0(), 0xAAAA);
        assert_eq!(m.scratch_r1(), 0x5555);
    }

    // ── HopByHopHeader ────────────────────────────────────────────────────

    #[test]
    fn hbh_header_new_sets_correct_constants() {
        let hbh = HopByHopHeader::new(6); // TCP
        assert_eq!(hbh.next_header,  6);
        assert_eq!(hbh.hdr_ext_len,  HBH_HDR_EXT_LEN);
        assert_eq!(hbh.opt_type,     MONAD_OPT_TYPE);
        assert_eq!(hbh.opt_data_len, MONAD_OPT_DATA_LEN);
        assert_eq!(hbh.monad.version, MONAD_VERSION);
    }

    #[test]
    fn hbh_header_serializes_to_24_bytes() {
        let hbh = HopByHopHeader::new(17); // UDP
        let b = hbh.to_bytes();
        assert_eq!(b.len(), HBH_TOTAL_LEN);
        assert_eq!(b[0], 17);              // next_header
        assert_eq!(b[1], HBH_HDR_EXT_LEN);
        assert_eq!(b[2], MONAD_OPT_TYPE);
        assert_eq!(b[3], MONAD_OPT_DATA_LEN);
        assert_eq!(b[4], MONAD_VERSION);   // monad.version at byte 4
    }

    #[test]
    fn opt_type_encoding_matches_rfc() {
        // Bits 7-6 = action 00 (skip if unrecognized)
        assert_eq!((MONAD_OPT_TYPE >> 6) & 0x03, 0b00, "action bits must be 00");
        // Bit 5 = change-en-route 1
        assert_eq!((MONAD_OPT_TYPE >> 5) & 0x01, 0b1, "change-en-route bit must be 1");
    }

    // ── AnamnesisEvent ────────────────────────────────────────────────────

    #[test]
    fn anamnesis_event_new_fields() {
        let m = Monad::new();
        let ev = AnamnesisEvent::new(
            1_000_000_000, // 1 second
            EventType::Birth,
            42,
            0x0A3F7E, // flow label (20 bits)
            m,
        );
        assert_eq!(ev.timestamp_ns, 1_000_000_000);
        assert_eq!(ev.event_type, EventType::Birth as u8);
        assert_eq!(ev.hop_id, 42);
        // flow_label_lo should be low 16 bits of 0x0A3F7E = 0x3F7E
        assert_eq!(ev.flow_label_u16(), 0x3F7E);
    }

    #[test]
    fn anamnesis_head_based_sampling() {
        let m = Monad::new();
        // flow_label_lo = 100 → 100 % 10 == 0 → sampled
        let ev = AnamnesisEvent {
            timestamp_ns:  0,
            event_type:    EventType::Hop as u8,
            hop_id:        0,
            flow_label_lo: 100u16.to_be_bytes(),
            monad:         m,
        };
        assert!(ev.should_sample(10));
        assert!(!ev.should_sample(7));  // 100 % 7 = 2 ≠ 0
    }

    #[test]
    fn anamnesis_sampling_zero_divisor_never_samples() {
        let m = Monad::new();
        let ev = AnamnesisEvent {
            timestamp_ns: 0, event_type: 0, hop_id: 0,
            flow_label_lo: 0u16.to_be_bytes(), monad: m,
        };
        assert!(!ev.should_sample(0));
    }

    #[test]
    fn anamnesis_event_contains_full_monad_snapshot() {
        let mut m = Monad::new();
        m.src_service_id = 0x42;
        m.hop_count = 7;
        m.recompute_checksum();

        let ev = AnamnesisEvent::new(999, EventType::Hop, 0, 0x1234, m);
        assert_eq!(ev.monad.src_service_id, 0x42);
        assert_eq!(ev.monad.hop_count, 7);
        assert!(ev.monad.verify_checksum());
    }

    // ── EventType ─────────────────────────────────────────────────────────

    #[test]
    fn event_type_values_match_spec() {
        assert_eq!(EventType::Birth   as u8, 0x01);
        assert_eq!(EventType::Hop     as u8, 0x02);
        assert_eq!(EventType::Death   as u8, 0x03);
        assert_eq!(EventType::Anomaly as u8, 0x04);
        assert_eq!(EventType::Chaos   as u8, 0x05);
    }

    #[test]
    fn event_type_from_u8_roundtrip() {
        for v in 1u8..=5 {
            assert!(EventType::from_u8(v).is_some(), "Valid type {v} must parse");
        }
        assert!(EventType::from_u8(0).is_none());
        assert!(EventType::from_u8(6).is_none());
        assert!(EventType::from_u8(0xFF).is_none());
    }

    // ── ChaosMode ─────────────────────────────────────────────────────────

    #[test]
    fn chaos_mode_values_match_spec() {
        assert_eq!(ChaosMode::BitFlip     as u8, 0x01);
        assert_eq!(ChaosMode::Delay       as u8, 0x02);
        assert_eq!(ChaosMode::Duplicate   as u8, 0x03);
        assert_eq!(ChaosMode::Truncate    as u8, 0x04);
        assert_eq!(ChaosMode::ChaosMarker as u8, 0x05);
    }

    // ── Circuit state & flow action ────────────────────────────────────────

    #[test]
    fn circuit_state_constants_are_distinct() {
        let states = [
            circuit_state::CLOSED,
            circuit_state::OPEN,
            circuit_state::HALF_OPEN,
        ];
        for i in 0..states.len() {
            for j in (i+1)..states.len() {
                assert_ne!(states[i], states[j], "Circuit states must be distinct");
            }
        }
    }

    #[test]
    fn flow_action_constants_are_distinct() {
        let actions = [
            flow_action::FORWARD,
            flow_action::TRACE,
            flow_action::SAMPLE,
            flow_action::MIRROR,
            flow_action::DROP,
        ];
        for i in 0..actions.len() {
            for j in (i+1)..actions.len() {
                assert_ne!(actions[i], actions[j], "Flow actions must be distinct");
            }
        }
    }

    // ── MBC ISA ────────────────────────────────────────────────────────────

    #[test]
    fn mbc_insn_encode_decode_roundtrip() {
        let insn = MbcInsn::encode(mbc_opcodes::ADD, 3, 7, 0xABCD);
        assert_eq!(insn.opcode(), mbc_opcodes::ADD);
        assert_eq!(insn.dst(), 3);
        assert_eq!(insn.src(), 7);
        assert_eq!(insn.imm16(), 0xABCD);
    }

    #[test]
    fn mbc_insn_register_fields_are_4_bits() {
        // dst and src are 4-bit fields — encoding high bits must be masked off
        let insn = MbcInsn::encode(0x01, 0xFF, 0xFF, 0x0000);
        assert_eq!(insn.dst(), 0x0F, "dst must be masked to 4 bits");
        assert_eq!(insn.src(), 0x0F, "src must be masked to 4 bits");
    }

    #[test]
    fn mbc_insn_signed_branch_offset() {
        let insn = MbcInsn::encode(mbc_opcodes::JMP, 0, 0, 0xFFFE); // -2
        assert_eq!(insn.imm16_signed(), -2i16);
    }

    #[test]
    fn mbc_opcodes_halt_is_0xff() {
        assert_eq!(mbc_opcodes::HALT, 0xFF);
    }

    #[test]
    fn mbc_mmap_screen_fits_doom_320x200() {
        assert_eq!(mbc_mmap::SCREEN_SIZE, 320 * 200);
    }

    #[test]
    fn mbc_mmap_screen_base_plus_size_is_kbd_addr() {
        // Screen region ends before keyboard address
        let screen_end = mbc_mmap::SCREEN_BASE + mbc_mmap::SCREEN_SIZE;
        assert!(screen_end <= mbc_mmap::KBD_ADDR);
    }

    // ── Ring buffer sizing ─────────────────────────────────────────────────

    #[test]
    fn anamnesis_ring_size_example_from_spec() {
        // From draft-bellis §6.3:
        // 10 Gbps, 1500-byte avg → R = 833,333 pps, S = 1.0, T = 2s
        // Expected raw = 833,333 × 1 × 32 × 2 = 53,333,312 bytes
        // Next power of 2 = 67,108,864 (64 MiB)
        let size = anamnesis_ring_size(833_333, 1, 1, 2);
        assert_eq!(size, 67_108_864, "Should round up to 64 MiB");
    }

    #[test]
    fn anamnesis_ring_size_zero_divisor_returns_zero() {
        let size = anamnesis_ring_size(100_000, 1, 0, 5);
        assert_eq!(size, 0);
    }

    #[test]
    fn anamnesis_ring_size_is_always_power_of_two() {
        let rates = [1000u64, 100_000, 500_000, 1_000_000];
        for r in rates {
            let sz = anamnesis_ring_size(r, 1, 1, 2);
            assert!(sz.is_power_of_two(), "size {sz} must be power of 2 for rate {r}");
        }
    }

    // ── MONAD_VERSION constant ─────────────────────────────────────────────

    #[test]
    fn protocol_version_is_one() {
        assert_eq!(MONAD_VERSION, 0x01);
    }

    #[test]
    fn monad_new_version_matches_protocol_version() {
        assert_eq!(Monad::new().version, MONAD_VERSION);
    }

    // ── CPU State and Compute Events ───────────────────────────────────────

    #[test]
    fn cpu_state_size() {
        // CpuState is 16*4 + 4 + 1 + 1 + 1 + 1 + 8 + 8 + 8 + 8 = 80 bytes
        assert_eq!(core::mem::size_of::<CpuState>(), 80);
    }

    #[test]
    fn cpu_state_default() {
        let cpu = CpuState::default();
        assert_eq!(cpu.pc, 0);
        assert_eq!(cpu.flags, 0);
        assert_eq!(cpu.halted, 0);
        assert_eq!(cpu.stalled, 0);
    }

    #[test]
    fn keyboard_state_size() {
        // KeyboardState is 4 + 4 + 8 = 16 bytes
        assert_eq!(core::mem::size_of::<KeyboardState>(), 16);
    }

    #[test]
    fn compute_hop_event_size() {
        // 8+1+1+2+4+4+4+64+1+1+4 = 94... recalc: 8+1+1+2+4+4+4+64+1+1+4 = 94
        // With repr(C): 8+1+1+[2 pad]+4+4+4+[64]+1+1+[2 pad]+4 = 96
        assert_eq!(core::mem::size_of::<ComputeHopEvent>(), 96);
    }

    #[test]
    fn compute_hop_event_default() {
        let ev = ComputeHopEvent::default();
        assert_eq!(ev.timestamp_ns, 0);
        assert_eq!(ev.event_type, 0);
        assert_eq!(ev.instruction, 0);
    }

    #[test]
    fn mem_write_event_size() {
        // 8+1+[3 pad]+4+4+1+[3 pad]+4 = 28 bytes
        assert_eq!(core::mem::size_of::<MemWriteEvent>(), 28);
    }

    #[test]
    fn event_type_constants_are_distinct() {
        let events = [
            EVENT_COMPUTE_HOP,
            EVENT_CACHE_MISS,
            EVENT_MEM_WRITE,
            EVENT_MEM_STAGED,
            EVENT_SCREEN_WRITE,
            EVENT_KEY_READ,
            EVENT_COMPUTE_HALT,
            EVENT_COMPUTE_STALL,
        ];
        for i in 0..events.len() {
            for j in (i + 1)..events.len() {
                assert_ne!(events[i], events[j], "Event types must be distinct");
            }
        }
    }

    #[test]
    fn cpu_flag_bits_do_not_overlap() {
        assert_eq!(cpu_flags::ZERO, 0b001);
        assert_eq!(cpu_flags::NEGATIVE, 0b010);
        assert_eq!(cpu_flags::CARRY, 0b100);
        // Verify no overlap
        assert_eq!(cpu_flags::ZERO & cpu_flags::NEGATIVE, 0);
        assert_eq!(cpu_flags::ZERO & cpu_flags::CARRY, 0);
        assert_eq!(cpu_flags::NEGATIVE & cpu_flags::CARRY, 0);
    }

    #[test]
    fn reg_sp_alias_is_15() {
        assert_eq!(REG_SP, 15);
    }
}
