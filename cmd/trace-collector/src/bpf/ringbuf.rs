// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

//! BPF Ring Buffer reading with zero-copy semantics.
//!
//! The ring buffer is a SPSC (single-producer, single-consumer) data structure
//! where the BPF program is the producer and we are the consumer.
//!
//! Memory layout:
//! - Producer page (read-only for userspace)
//! - Consumer page (read-write for userspace)
//! - Data pages (read-only for userspace)
//!
//! Each record has an 8-byte header:
//! - Lower 28 bits: length
//! - Bit 28: BPF_RINGBUF_BUSY_BIT
//! - Bit 29: BPF_RINGBUF_DISCARD_BIT

use std::os::unix::io::FromRawFd;
use std::path::Path;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::Arc;
use std::time::Duration;

use anyhow::Result;
use crossbeam::channel::Sender;
use memmap2::{MmapMut, MmapOptions};
use tracing::{debug, trace, warn};

use super::{bpf_map_info, bpf_obj_get, BpfError, BpfMapType};
use crate::events::{Event, EventData, EventType};
use crate::metrics;

/// Ring buffer record header bits
const BPF_RINGBUF_BUSY_BIT: u32 = 1 << 31;
const BPF_RINGBUF_DISCARD_BIT: u32 = 1 << 30;
const BPF_RINGBUF_LEN_MASK: u32 = (1 << 28) - 1;

/// Page size (typically 4096)
const PAGE_SIZE: usize = 4096;

/// Ring buffer reader for zero-copy event reading.
///
/// BPF ring buffer mmap layout (from kernel/bpf/ringbuf.c):
///   Page 0 (offset 0):            consumer_pos — WRITABLE by userspace
///   Page 1 (offset PAGE_SIZE):    producer_pos — read-only for userspace
///   Pages 2..2N+1 (offset 2*PS):  data pages   — read-only, mapped twice for wrap
pub struct RingBufReader {
    /// Memory-mapped consumer page at offset 0 (read-write — we update consumer position)
    consumer_mmap: MmapMut,
    /// Memory-mapped producer page at offset PAGE_SIZE (read-only — kernel writes producer position)
    producer_mmap: memmap2::Mmap,
    /// Memory-mapped data pages at offset 2*PAGE_SIZE (read-only — kernel writes event data)
    data_mmap: memmap2::Mmap,
    /// Ring buffer size (data portion only)
    #[allow(dead_code)]
    data_size: usize,
    /// Mask for wrapping (size - 1, since size is power of 2)
    mask: usize,
    /// File descriptor
    fd: i32,
}

impl RingBufReader {
    /// Create a new ring buffer reader from a pinned BPF map path
    pub fn new(path: &Path, expected_size: usize) -> Result<Self, BpfError> {
        // Open the pinned map
        let fd = bpf_obj_get(path)?;

        // Verify it's a ring buffer
        let info = bpf_map_info(fd)?;
        if info.map_type != BpfMapType::RingBuf as u32 {
            return Err(BpfError::InvalidMapType {
                expected: BpfMapType::RingBuf as u32,
                actual: info.map_type,
            });
        }

        let data_size = info.max_entries as usize;
        debug!(
            name = info.name_str(),
            size = data_size,
            "Opening ring buffer"
        );

        // Validate size is power of 2
        if data_size == 0 || (data_size & (data_size - 1)) != 0 {
            return Err(BpfError::Mmap(format!(
                "Ring buffer size must be power of 2, got {}",
                data_size
            )));
        }

        let _ = expected_size; // We use the actual size from map info

        // mmap the ring buffer
        // Layout: producer page | consumer page | data pages (x2 for wrap-around)
        //
        // IMPORTANT: We must keep the File alive across all mmaps. File::from_raw_fd
        // takes ownership, so we create it once and borrow for each mmap call.
        // We use ManuallyDrop to prevent the File from closing our fd on drop
        // (we close it ourselves in RingBufReader::drop).
        let file = std::mem::ManuallyDrop::new(unsafe { std::fs::File::from_raw_fd(fd) });

        // Consumer page at offset 0 (read-write — userspace writes consumer_pos)
        // Kernel only allows PROT_WRITE on pgoff==0 with size==PAGE_SIZE.
        let consumer_mmap = unsafe {
            MmapOptions::new()
                .len(PAGE_SIZE)
                .offset(0)
                .map_mut(&*file)
                .map_err(|e| BpfError::Mmap(format!("consumer mmap (offset 0): {}", e)))?
        };

        // Producer page at offset PAGE_SIZE (read-only — kernel writes producer_pos)
        let producer_mmap = unsafe {
            MmapOptions::new()
                .len(PAGE_SIZE)
                .offset(PAGE_SIZE as u64)
                .map(&*file)
                .map_err(|e| {
                    BpfError::Mmap(format!("producer mmap (offset {}): {}", PAGE_SIZE, e))
                })?
        };

        // Data pages at offset 2*PAGE_SIZE (read-only, mapped twice for wrap-around)
        let data_mmap = unsafe {
            MmapOptions::new()
                .len(data_size * 2)
                .offset((2 * PAGE_SIZE) as u64)
                .map(&*file)
                .map_err(|e| {
                    BpfError::Mmap(format!("data mmap (offset {}): {}", 2 * PAGE_SIZE, e))
                })?
        };

        Ok(Self {
            consumer_mmap,
            producer_mmap,
            data_mmap,
            data_size,
            mask: data_size - 1,
            fd,
        })
    }

    /// Get the producer position (written by kernel)
    #[inline]
    fn producer_pos(&self) -> u64 {
        let ptr = self.producer_mmap.as_ptr() as *const AtomicU64;
        unsafe { (*ptr).load(Ordering::Acquire) }
    }

    /// Get the consumer position (written by us)
    #[inline]
    fn consumer_pos(&self) -> u64 {
        let ptr = self.consumer_mmap.as_ptr() as *const AtomicU64;
        unsafe { (*ptr).load(Ordering::Acquire) }
    }

    /// Set the consumer position
    #[inline]
    fn set_consumer_pos(&mut self, pos: u64) {
        let ptr = self.consumer_mmap.as_mut_ptr() as *mut AtomicU64;
        unsafe { (*ptr).store(pos, Ordering::Release) };
    }

    /// Read a record header at the given position
    #[inline]
    fn read_header(&self, pos: usize) -> u32 {
        let offset = pos & self.mask;
        let ptr = unsafe { self.data_mmap.as_ptr().add(offset) as *const u32 };
        unsafe { std::ptr::read_volatile(ptr) }
    }

    /// Get a pointer to record data
    #[inline]
    fn record_data(&self, pos: usize, len: usize) -> &[u8] {
        let offset = (pos + 8) & self.mask; // Skip 8-byte header
        unsafe { std::slice::from_raw_parts(self.data_mmap.as_ptr().add(offset), len) }
    }

    /// Round up length to 8-byte alignment
    #[inline]
    fn round_up_len(len: u32) -> u32 {
        (len + 7) & !7
    }

    /// Parse a 32-byte AnamnesisEvent from Shield eBPF.
    ///
    /// Layout (monad-common/src/lib.rs):
    ///   [0..8]   timestamp_ns: u64 (LE, bpf_ktime_get_ns)
    ///   [8]      event_type: u8 (0=Birth, 1=Hop, 2=Death, 3=Anomaly, 4=Chaos)
    ///   [9]      hop_id: u8
    ///   [10..12] flow_label_lo: [u8; 2] (BE)
    ///   [12..32] monad: Monad (20 bytes)
    fn parse_anamnesis_event(data: &[u8]) -> Result<Event, BpfError> {
        if data.len() < 32 {
            return Err(BpfError::Syscall(format!(
                "AnamnesisEvent too short: {} < 32",
                data.len()
            )));
        }

        let timestamp_ns = u64::from_le_bytes(data[0..8].try_into().unwrap());
        let event_type_raw = data[8];
        let _hop_id = data[9];
        let _flow_label = u16::from_be_bytes(data[10..12].try_into().unwrap());

        // EventType (monad-common): Birth=1, Hop=2, Death=3, Anomaly=4, Chaos=5
        let event_type_name = match event_type_raw {
            1 => "Birth",
            2 => "Hop",
            3 => "Death",
            4 => "Anomaly",
            5 => "Chaos",
            _ => "Unknown",
        };

        // Map Shield event types to trace-collector EventType
        let event_type = match event_type_raw {
            1..=3 => EventType::Packet, // Birth/Hop/Death → Packet
            4 | 5 => EventType::Custom, // Anomaly/Chaos → Custom
            _ => EventType::Custom,
        };

        Ok(Event {
            event_type,
            cpu: 0,
            timestamp_ns,
            pid: 0,
            tid: 0,
            comm: event_type_name.to_string(),
            data: EventData::Raw(data.to_vec()),
            raw: Some(bytes::Bytes::copy_from_slice(data)),
        })
    }

    /// Poll for available data
    fn poll_wait(&self, timeout_ms: i32) -> Result<bool, BpfError> {
        let mut pollfd = libc::pollfd {
            fd: self.fd,
            events: libc::POLLIN,
            revents: 0,
        };

        let ret = unsafe { libc::poll(&mut pollfd, 1, timeout_ms) };

        if ret < 0 {
            let err = std::io::Error::last_os_error();
            if err.kind() == std::io::ErrorKind::Interrupted {
                return Ok(false);
            }
            return Err(BpfError::Syscall(format!("poll failed: {}", err)));
        }

        Ok(ret > 0)
    }

    /// Read available events from the ring buffer
    fn read_events(&mut self, sender: &Sender<Event>) -> Result<usize, BpfError> {
        let mut events_read = 0;
        let mut cons_pos = self.consumer_pos();
        let prod_pos = self.producer_pos();

        // Update metrics
        metrics::set_ringbuf_positions(cons_pos, prod_pos);

        while cons_pos < prod_pos {
            let header = self.read_header(cons_pos as usize);

            // Check if record is ready
            if (header & BPF_RINGBUF_BUSY_BIT) != 0 {
                // Record still being written, wait
                break;
            }

            let len = header & BPF_RINGBUF_LEN_MASK;
            let record_size = 8 + Self::round_up_len(len) as usize;

            // Check for discard
            if (header & BPF_RINGBUF_DISCARD_BIT) == 0 {
                // Valid record, parse it
                let data = self.record_data(cons_pos as usize, len as usize);

                // Try generic Event format first, fall back to AnamnesisEvent (32 bytes)
                let event = Event::from_bytes(data).or_else(|_| Self::parse_anamnesis_event(data));

                match event {
                    Ok(ev) => {
                        let now = std::time::Instant::now();

                        if sender.try_send(ev).is_err() {
                            metrics::record_event_dropped();
                            warn!("Event queue full, dropping event");
                        } else {
                            events_read += 1;
                            metrics::record_event_received("ringbuf", "bpf");
                        }

                        metrics::record_processing_latency_ns(now.elapsed().as_nanos() as u64);
                    }
                    Err(e) => {
                        trace!(error = %e, len = len, "Failed to parse event");
                    }
                }
            }

            cons_pos += record_size as u64;
        }

        // Update consumer position
        if cons_pos != self.consumer_pos() {
            self.set_consumer_pos(cons_pos);
        }

        Ok(events_read)
    }

    /// Run the reader loop
    /// Run the reader loop, polling with `poll_timeout_ms` between shutdown
    /// checks.
    ///
    /// The timeout used to be hardcoded to 100 ms here while
    /// `MultiSourceConfig::poll_timeout_ms` — settable through two separate
    /// builder methods — was accepted and discarded. Now it reaches the poll.
    pub async fn run(
        mut self,
        sender: Sender<Event>,
        shutdown: Arc<AtomicBool>,
        poll_timeout_ms: u64,
    ) -> Result<(), BpfError> {
        debug!("Ring buffer reader starting");

        // Drain any existing data before entering the poll loop.
        // Without this, a full ring buffer deadlocks: poll() waits for NEW data
        // but the producer can't write because the buffer is full.
        match self.read_events(&sender) {
            Ok(count) if count > 0 => {
                debug!(
                    events = count,
                    "Drained existing ring buffer data on startup"
                );
            }
            Err(e) => {
                warn!(error = %e, "Error draining existing ring buffer data");
            }
            _ => {}
        }

        while !shutdown.load(Ordering::Relaxed) {
            // Poll for data with timeout
            match self.poll_wait(poll_timeout_ms as i32) {
                Ok(true) => {
                    // Data available, read it
                    match self.read_events(&sender) {
                        Ok(count) => {
                            if count > 0 {
                                trace!(events = count, "Read events from ring buffer");
                            }
                        }
                        Err(e) => {
                            warn!(error = %e, "Error reading from ring buffer");
                        }
                    }
                }
                Ok(false) => {
                    // Timeout, check shutdown and continue
                    continue;
                }
                Err(e) => {
                    warn!(error = %e, "Poll error");
                    // Brief sleep before retry
                    tokio::time::sleep(Duration::from_millis(10)).await;
                }
            }

            // Yield to allow other tasks to run
            tokio::task::yield_now().await;
        }

        debug!("Ring buffer reader stopped");
        Ok(())
    }
}

impl Drop for RingBufReader {
    fn drop(&mut self) {
        // Close the file descriptor
        unsafe {
            libc::close(self.fd);
        }
    }
}

// Ring buffer is not Send/Sync by default due to raw pointers
// but our usage is safe since we control access
unsafe impl Send for RingBufReader {}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_round_up_len() {
        assert_eq!(RingBufReader::round_up_len(0), 0);
        assert_eq!(RingBufReader::round_up_len(1), 8);
        assert_eq!(RingBufReader::round_up_len(7), 8);
        assert_eq!(RingBufReader::round_up_len(8), 8);
        assert_eq!(RingBufReader::round_up_len(9), 16);
    }

    #[test]
    fn test_header_bits() {
        let header = 100 | BPF_RINGBUF_BUSY_BIT;
        assert!((header & BPF_RINGBUF_BUSY_BIT) != 0);
        assert_eq!(header & BPF_RINGBUF_LEN_MASK, 100);

        let header2 = 50 | BPF_RINGBUF_DISCARD_BIT;
        assert!((header2 & BPF_RINGBUF_DISCARD_BIT) != 0);
        assert!((header2 & BPF_RINGBUF_BUSY_BIT) == 0);
    }
}
