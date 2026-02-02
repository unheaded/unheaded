//! Trace Correlation Engine
//!
//! THE WHISPERING VOID weaves together the threads of distributed traces.
//!
//! This module correlates events from multiple eBPF sources into coherent
//! distributed traces. It maintains:
//!
//! - Active trace state (spans, services, timing)
//! - Cross-service correlation via trace IDs
//! - Flow-to-trace associations
//! - Latency aggregation per trace
//!
//! ## Architecture
//!
//! ```text
//! Events from eBPF → Correlation Engine → Correlated Traces → Publishers
//!                           ↓
//!                    Trace Store (query)
//! ```

mod engine;
mod span;
mod store;

pub use engine::{
    CorrelationConfig, CorrelationEngine, CorrelationStats, TraceContext, TraceSummary,
};
pub use span::{Span, SpanKind, SpanStatus};
pub use store::{TraceQuery, TraceQueryResult, TraceStore, TraceStoreConfig};

use std::fmt;

/// 128-bit Trace ID following W3C Trace Context
#[derive(Clone, Copy, PartialEq, Eq, Hash, Default)]
pub struct TraceId {
    pub high: u64,
    pub low: u64,
}

impl TraceId {
    /// Create a new trace ID
    pub const fn new(high: u64, low: u64) -> Self {
        Self { high, low }
    }

    /// Create a zero trace ID (invalid)
    pub const fn zero() -> Self {
        Self { high: 0, low: 0 }
    }

    /// Check if this is a zero/invalid trace ID
    pub const fn is_zero(&self) -> bool {
        self.high == 0 && self.low == 0
    }

    /// Generate a new random trace ID
    pub fn generate() -> Self {
        use std::time::SystemTime;

        // Use current time + random bits for uniqueness
        let now = SystemTime::now()
            .duration_since(SystemTime::UNIX_EPOCH)
            .unwrap_or_default();

        let high = (now.as_nanos() as u64) ^ (std::process::id() as u64);
        let low = now.as_nanos() as u64 ^ fastrand::u64(..);

        Self { high, low }
    }

    /// Create from 16 bytes (big-endian)
    pub fn from_bytes(bytes: [u8; 16]) -> Self {
        let high = u64::from_be_bytes([
            bytes[0], bytes[1], bytes[2], bytes[3], bytes[4], bytes[5], bytes[6], bytes[7],
        ]);
        let low = u64::from_be_bytes([
            bytes[8], bytes[9], bytes[10], bytes[11], bytes[12], bytes[13], bytes[14], bytes[15],
        ]);
        Self { high, low }
    }

    /// Convert to 16 bytes (big-endian)
    pub fn to_bytes(&self) -> [u8; 16] {
        let h = self.high.to_be_bytes();
        let l = self.low.to_be_bytes();
        [
            h[0], h[1], h[2], h[3], h[4], h[5], h[6], h[7], l[0], l[1], l[2], l[3], l[4], l[5],
            l[6], l[7],
        ]
    }

    /// Parse from hex string (32 characters)
    pub fn from_hex(s: &str) -> Option<Self> {
        if s.len() != 32 {
            return None;
        }

        let high = u64::from_str_radix(&s[0..16], 16).ok()?;
        let low = u64::from_str_radix(&s[16..32], 16).ok()?;
        Some(Self { high, low })
    }

    /// Convert to hex string (32 characters)
    pub fn to_hex(&self) -> String {
        format!("{:016x}{:016x}", self.high, self.low)
    }
}

impl fmt::Debug for TraceId {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "TraceId({})", self.to_hex())
    }
}

impl fmt::Display for TraceId {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.to_hex())
    }
}

impl serde::Serialize for TraceId {
    fn serialize<S>(&self, serializer: S) -> Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        serializer.serialize_str(&self.to_hex())
    }
}

impl<'de> serde::Deserialize<'de> for TraceId {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        let s = String::deserialize(deserializer)?;
        TraceId::from_hex(&s).ok_or_else(|| serde::de::Error::custom("invalid trace ID"))
    }
}

/// 64-bit Span ID
#[derive(Clone, Copy, PartialEq, Eq, Hash, Default)]
pub struct SpanId(pub u64);

impl SpanId {
    pub const fn new(id: u64) -> Self {
        Self(id)
    }

    pub const fn zero() -> Self {
        Self(0)
    }

    pub const fn is_zero(&self) -> bool {
        self.0 == 0
    }

    pub fn generate() -> Self {
        Self(fastrand::u64(..))
    }

    pub fn from_hex(s: &str) -> Option<Self> {
        u64::from_str_radix(s, 16).ok().map(Self)
    }

    pub fn to_hex(&self) -> String {
        format!("{:016x}", self.0)
    }
}

impl fmt::Debug for SpanId {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "SpanId({})", self.to_hex())
    }
}

impl fmt::Display for SpanId {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.to_hex())
    }
}

impl serde::Serialize for SpanId {
    fn serialize<S>(&self, serializer: S) -> Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        serializer.serialize_str(&self.to_hex())
    }
}

impl<'de> serde::Deserialize<'de> for SpanId {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        let s = String::deserialize(deserializer)?;
        SpanId::from_hex(&s).ok_or_else(|| serde::de::Error::custom("invalid span ID"))
    }
}

/// 5-tuple flow key for correlation
#[derive(Clone, Copy, PartialEq, Eq, Hash, Debug)]
pub struct FlowKey {
    pub src_addr: u32,
    pub dst_addr: u32,
    pub src_port: u16,
    pub dst_port: u16,
    pub protocol: u8,
}

impl FlowKey {
    pub fn new(src_addr: u32, dst_addr: u32, src_port: u16, dst_port: u16, protocol: u8) -> Self {
        Self {
            src_addr,
            dst_addr,
            src_port,
            dst_port,
            protocol,
        }
    }

    /// Create normalized key (smaller IP first) for bidirectional matching
    pub fn normalized(&self) -> Self {
        if self.src_addr < self.dst_addr
            || (self.src_addr == self.dst_addr && self.src_port < self.dst_port)
        {
            *self
        } else {
            Self {
                src_addr: self.dst_addr,
                dst_addr: self.src_addr,
                src_port: self.dst_port,
                dst_port: self.src_port,
                protocol: self.protocol,
            }
        }
    }

    /// Compute hash for the flow key
    pub fn hash(&self) -> u32 {
        let mut h: u32 = 2166136261;
        h = h.wrapping_mul(16777619) ^ self.src_addr;
        h = h.wrapping_mul(16777619) ^ self.dst_addr;
        h = h.wrapping_mul(16777619) ^ (self.src_port as u32);
        h = h.wrapping_mul(16777619) ^ (self.dst_port as u32);
        h = h.wrapping_mul(16777619) ^ (self.protocol as u32);
        h
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_trace_id() {
        let id = TraceId::new(0x0123456789abcdef, 0xfedcba9876543210);
        assert_eq!(id.to_hex(), "0123456789abcdeffedcba9876543210");

        let parsed = TraceId::from_hex("0123456789abcdeffedcba9876543210").unwrap();
        assert_eq!(id, parsed);

        let bytes = id.to_bytes();
        let from_bytes = TraceId::from_bytes(bytes);
        assert_eq!(id, from_bytes);
    }

    #[test]
    fn test_trace_id_zero() {
        let zero = TraceId::zero();
        assert!(zero.is_zero());

        let non_zero = TraceId::new(1, 0);
        assert!(!non_zero.is_zero());
    }

    #[test]
    fn test_span_id() {
        let id = SpanId::new(0x123456789abcdef0);
        assert_eq!(id.to_hex(), "123456789abcdef0");

        let parsed = SpanId::from_hex("123456789abcdef0").unwrap();
        assert_eq!(id, parsed);
    }

    #[test]
    fn test_flow_key_normalized() {
        let key1 = FlowKey::new(100, 200, 1000, 2000, 6);
        let key2 = FlowKey::new(200, 100, 2000, 1000, 6);

        assert_eq!(key1.normalized(), key2.normalized());
    }

    #[test]
    fn test_trace_id_generate() {
        let id1 = TraceId::generate();
        let id2 = TraceId::generate();

        assert!(!id1.is_zero());
        assert!(!id2.is_zero());
        assert_ne!(id1, id2);
    }
}
