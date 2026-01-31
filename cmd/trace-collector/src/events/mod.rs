//! Event types and parsing for kernel events.
//!
//! Supports:
//! - Packet traces (network events)
//! - Syscall events
//! - Latency probes

pub mod latency;
pub mod packet;
pub mod syscall;

pub use latency::LatencyEvent;
pub use packet::PacketEvent;
pub use syscall::SyscallEvent;

use std::time::{Duration, SystemTime, UNIX_EPOCH};

use bytes::Bytes;
use serde::{Deserialize, Serialize};
use thiserror::Error;

/// Event parsing errors
#[derive(Error, Debug)]
pub enum EventError {
    #[error("Invalid event header: {0}")]
    InvalidHeader(String),

    #[error("Unknown event type: {0}")]
    UnknownType(u32),

    #[error("Insufficient data: expected {expected}, got {actual}")]
    InsufficientData { expected: usize, actual: usize },

    #[error("Invalid UTF-8 in event data")]
    InvalidUtf8,

    #[error("Checksum mismatch")]
    ChecksumMismatch,
}

/// Event type discriminator (matches BPF-side definitions)
#[repr(u32)]
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub enum EventType {
    /// Network packet event
    Packet = 1,
    /// System call event
    Syscall = 2,
    /// Latency probe event
    Latency = 3,
    /// Process lifecycle event
    Process = 4,
    /// File system event
    FileSystem = 5,
    /// Memory event
    Memory = 6,
    /// Custom/user-defined event
    Custom = 255,
}

impl TryFrom<u32> for EventType {
    type Error = EventError;

    fn try_from(value: u32) -> Result<Self, Self::Error> {
        match value {
            1 => Ok(EventType::Packet),
            2 => Ok(EventType::Syscall),
            3 => Ok(EventType::Latency),
            4 => Ok(EventType::Process),
            5 => Ok(EventType::FileSystem),
            6 => Ok(EventType::Memory),
            255 => Ok(EventType::Custom),
            _ => Err(EventError::UnknownType(value)),
        }
    }
}

/// Common event header (16 bytes, matches BPF struct)
#[repr(C, packed)]
#[derive(Debug, Clone, Copy)]
pub struct EventHeader {
    /// Event type
    pub event_type: u32,
    /// Event flags
    pub flags: u16,
    /// CPU where event was generated
    pub cpu: u16,
    /// Timestamp in nanoseconds since boot
    pub timestamp_ns: u64,
}

impl EventHeader {
    pub const SIZE: usize = std::mem::size_of::<Self>();

    /// Parse header from raw bytes
    pub fn from_bytes(data: &[u8]) -> Result<Self, EventError> {
        if data.len() < Self::SIZE {
            return Err(EventError::InsufficientData {
                expected: Self::SIZE,
                actual: data.len(),
            });
        }

        // SAFETY: We've verified the length and the struct is repr(C, packed)
        let header = unsafe { std::ptr::read_unaligned(data.as_ptr() as *const Self) };
        Ok(header)
    }

    /// Convert boot timestamp to wall clock time
    pub fn to_wall_time(&self, boot_time: SystemTime) -> SystemTime {
        boot_time + Duration::from_nanos(self.timestamp_ns)
    }
}

/// Unified event structure
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Event {
    /// Event type
    pub event_type: EventType,
    /// CPU where event originated
    pub cpu: u16,
    /// Timestamp (nanoseconds since Unix epoch)
    pub timestamp_ns: u64,
    /// Process ID
    pub pid: u32,
    /// Thread ID
    pub tid: u32,
    /// Process/command name
    pub comm: String,
    /// Event-specific data
    pub data: EventData,
    /// Raw event bytes (for forwarding)
    #[serde(skip)]
    pub raw: Option<Bytes>,
}

impl Event {
    /// Create a new event from parsed data
    pub fn new(
        event_type: EventType,
        cpu: u16,
        timestamp_ns: u64,
        pid: u32,
        tid: u32,
        comm: String,
        data: EventData,
    ) -> Self {
        Self {
            event_type,
            cpu,
            timestamp_ns,
            pid,
            tid,
            comm,
            data,
            raw: None,
        }
    }

    /// Parse event from raw BPF data
    pub fn from_bytes(data: &[u8]) -> Result<Self, EventError> {
        let header = EventHeader::from_bytes(data)?;
        let event_type = EventType::try_from(header.event_type)?;

        let payload = &data[EventHeader::SIZE..];

        let (pid, tid, comm, event_data) = match event_type {
            EventType::Packet => {
                let packet = PacketEvent::from_bytes(payload)?;
                (packet.pid, packet.tid, packet.comm.clone(), EventData::Packet(packet))
            }
            EventType::Syscall => {
                let syscall = SyscallEvent::from_bytes(payload)?;
                (syscall.pid, syscall.tid, syscall.comm.clone(), EventData::Syscall(syscall))
            }
            EventType::Latency => {
                let latency = LatencyEvent::from_bytes(payload)?;
                (latency.pid, latency.tid, latency.comm.clone(), EventData::Latency(latency))
            }
            _ => {
                // For unknown types, store raw data
                (0, 0, String::new(), EventData::Raw(payload.to_vec()))
            }
        };

        Ok(Self {
            event_type,
            cpu: header.cpu,
            timestamp_ns: header.timestamp_ns,
            pid,
            tid,
            comm,
            data: event_data,
            raw: Some(Bytes::copy_from_slice(data)),
        })
    }

    /// Get wall clock timestamp
    pub fn wall_time(&self) -> SystemTime {
        UNIX_EPOCH + Duration::from_nanos(self.timestamp_ns)
    }

    /// Estimated size in bytes
    pub fn size_bytes(&self) -> usize {
        self.raw.as_ref().map(|b| b.len()).unwrap_or(128)
    }
}

/// Event-specific data variants
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type")]
pub enum EventData {
    Packet(PacketEvent),
    Syscall(SyscallEvent),
    Latency(LatencyEvent),
    Raw(Vec<u8>),
}

/// Event batch for efficient publishing
#[derive(Debug, Clone)]
pub struct EventBatch {
    pub events: Vec<Event>,
    pub first_timestamp: u64,
    pub last_timestamp: u64,
    pub total_bytes: usize,
}

impl EventBatch {
    pub fn new() -> Self {
        Self {
            events: Vec::with_capacity(1024),
            first_timestamp: 0,
            last_timestamp: 0,
            total_bytes: 0,
        }
    }

    pub fn with_capacity(capacity: usize) -> Self {
        Self {
            events: Vec::with_capacity(capacity),
            first_timestamp: 0,
            last_timestamp: 0,
            total_bytes: 0,
        }
    }

    pub fn push(&mut self, event: Event) {
        if self.events.is_empty() {
            self.first_timestamp = event.timestamp_ns;
        }
        self.last_timestamp = event.timestamp_ns;
        self.total_bytes += event.size_bytes();
        self.events.push(event);
    }

    pub fn len(&self) -> usize {
        self.events.len()
    }

    pub fn is_empty(&self) -> bool {
        self.events.is_empty()
    }

    pub fn clear(&mut self) {
        self.events.clear();
        self.first_timestamp = 0;
        self.last_timestamp = 0;
        self.total_bytes = 0;
    }

    pub fn duration_ns(&self) -> u64 {
        self.last_timestamp.saturating_sub(self.first_timestamp)
    }
}

impl Default for EventBatch {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_event_header_size() {
        assert_eq!(EventHeader::SIZE, 16);
    }

    #[test]
    fn test_event_type_conversion() {
        assert_eq!(EventType::try_from(1).unwrap(), EventType::Packet);
        assert_eq!(EventType::try_from(2).unwrap(), EventType::Syscall);
        assert!(EventType::try_from(999).is_err());
    }

    #[test]
    fn test_event_batch() {
        let mut batch = EventBatch::new();
        assert!(batch.is_empty());

        let event = Event::new(
            EventType::Packet,
            0,
            1000,
            1234,
            1234,
            "test".to_string(),
            EventData::Raw(vec![]),
        );
        batch.push(event);

        assert_eq!(batch.len(), 1);
        assert_eq!(batch.first_timestamp, 1000);
    }
}
