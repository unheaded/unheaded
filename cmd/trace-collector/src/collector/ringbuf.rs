//! Ring Buffer Collector - mmap-based zero-copy event reading.
//!
//! THE WHISPERING VOID's primary ear: the BPF ring buffer provides
//! ordered, efficient, and lock-free event delivery from kernel space.
//!
//! The ring buffer is a SPSC (single-producer, single-consumer) data structure
//! where eBPF programs are producers and we are the consumer.
//!
//! Memory layout (from BPF perspective):
//! - Producer page: Contains producer position (kernel writes, we read)
//! - Consumer page: Contains consumer position (we write, kernel reads)
//! - Data pages: Ring buffer data with wrap-around support
//!
//! Each record has an 8-byte header:
//! - Bits 0-27: Record length
//! - Bit 28: Reserved
//! - Bit 29: Reserved
//! - Bit 30: Discard bit (set when BPF discards the record)
//! - Bit 31: Busy bit (set while record is being written)

use std::path::PathBuf;
use std::ptr;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::Arc;
use std::time::Duration;

use anyhow::{Context, Result};
use crossbeam::channel::Sender;
use memmap2::{MmapMut, MmapOptions};
use tracing::{debug, trace, warn};

use super::events::EventFilter;
use super::CollectorStats;
use crate::bpf::{bpf_map_info, bpf_obj_get, BpfMapType};
use crate::events::Event;
use crate::metrics;

/// Ring buffer record header bits
const BPF_RINGBUF_BUSY_BIT: u32 = 1 << 31;
const BPF_RINGBUF_DISCARD_BIT: u32 = 1 << 30;
const BPF_RINGBUF_HDR_SZ: usize = 8;

/// Extract length from header (lower 28 bits)
#[inline]
fn ringbuf_rec_len(header: u32) -> u32 {
    header & ((1 << 28) - 1)
}

/// Check if record is busy (still being written)
#[inline]
fn ringbuf_rec_is_busy(header: u32) -> bool {
    (header & BPF_RINGBUF_BUSY_BIT) != 0
}

/// Check if record is discarded
#[inline]
fn ringbuf_rec_is_discarded(header: u32) -> bool {
    (header & BPF_RINGBUF_DISCARD_BIT) != 0
}

/// Round up to 8-byte alignment
#[inline]
fn round_up_8(len: u32) -> u32 {
    (len + 7) & !7
}

/// Page size constant
const PAGE_SIZE: usize = 4096;

/// Ring buffer collector with zero-copy reading
pub struct RingBufCollector {
    /// Path to the BPF map
    path: PathBuf,
    /// Memory-mapped producer page
    producer_mmap: MmapMut,
    /// Memory-mapped consumer page
    consumer_mmap: MmapMut,
    /// Memory-mapped data pages
    data_mmap: MmapMut,
    /// Ring buffer data size
    #[allow(dead_code)]
    data_size: usize,
    /// Mask for wrap-around (size - 1)
    mask: usize,
    /// BPF map file descriptor
    fd: i32,
    /// Shared statistics
    stats: Arc<CollectorStats>,
}

impl RingBufCollector {
    /// Create a new ring buffer collector
    pub fn new(path: PathBuf, _expected_size: usize, stats: Arc<CollectorStats>) -> Result<Self> {
        // Open the pinned BPF map
        let fd =
            bpf_obj_get(&path).map_err(|e| anyhow::anyhow!("Failed to open BPF map: {}", e))?;

        // Get map info to verify type and size
        let info =
            bpf_map_info(fd).map_err(|e| anyhow::anyhow!("Failed to get map info: {}", e))?;

        if info.map_type != BpfMapType::RingBuf as u32 {
            unsafe { libc::close(fd) };
            return Err(anyhow::anyhow!(
                "Invalid map type: expected RingBuf ({}), got {}",
                BpfMapType::RingBuf as u32,
                info.map_type
            ));
        }

        let data_size = info.max_entries as usize;
        debug!(
            path = %path.display(),
            name = info.name_str(),
            size = data_size,
            "Opening ring buffer"
        );

        // Validate size is power of 2
        if data_size == 0 || (data_size & (data_size - 1)) != 0 {
            unsafe { libc::close(fd) };
            return Err(anyhow::anyhow!(
                "Ring buffer size must be power of 2, got {}",
                data_size
            ));
        }

        // Create file wrapper for mmap (don't close on drop)
        use std::os::fd::FromRawFd;
        let file = unsafe { std::fs::File::from_raw_fd(fd) };

        // mmap producer page (read-only, offset 0)
        let producer_mmap = unsafe {
            MmapOptions::new()
                .len(PAGE_SIZE)
                .offset(0)
                .map_mut(&file)
                .context("Failed to mmap producer page")?
        };

        // mmap consumer page (read-write, offset PAGE_SIZE)
        let consumer_mmap = unsafe {
            MmapOptions::new()
                .len(PAGE_SIZE)
                .offset(PAGE_SIZE as u64)
                .map_mut(&file)
                .context("Failed to mmap consumer page")?
        };

        // mmap data pages (offset 2*PAGE_SIZE, size = 2 * data_size for wrap-around)
        let data_mmap = unsafe {
            MmapOptions::new()
                .len(data_size * 2)
                .offset((2 * PAGE_SIZE) as u64)
                .map_mut(&file)
                .context("Failed to mmap data pages")?
        };

        // Prevent file from being closed (we manage the fd ourselves)
        std::mem::forget(file);

        Ok(Self {
            path,
            producer_mmap,
            consumer_mmap,
            data_mmap,
            data_size,
            mask: data_size - 1,
            fd,
            stats,
        })
    }

    /// Get producer position (written by kernel, volatile read)
    #[inline]
    fn producer_pos(&self) -> u64 {
        let ptr = self.producer_mmap.as_ptr() as *const AtomicU64;
        unsafe { (*ptr).load(Ordering::Acquire) }
    }

    /// Get consumer position
    #[inline]
    fn consumer_pos(&self) -> u64 {
        let ptr = self.consumer_mmap.as_ptr() as *const AtomicU64;
        unsafe { (*ptr).load(Ordering::Acquire) }
    }

    /// Update consumer position (signals kernel we've consumed data)
    #[inline]
    fn set_consumer_pos(&mut self, pos: u64) {
        let ptr = self.consumer_mmap.as_mut_ptr() as *mut AtomicU64;
        unsafe { (*ptr).store(pos, Ordering::Release) };
    }

    /// Read header at given position (with wrap-around)
    #[inline]
    fn read_header(&self, pos: usize) -> u32 {
        let offset = pos & self.mask;
        let ptr = unsafe { self.data_mmap.as_ptr().add(offset) as *const u32 };
        // Volatile read to ensure we see kernel updates
        unsafe { ptr::read_volatile(ptr) }
    }

    /// Get slice of record data (zero-copy)
    #[inline]
    fn record_data(&self, pos: usize, len: usize) -> &[u8] {
        let offset = (pos + BPF_RINGBUF_HDR_SZ) & self.mask;
        // Data is double-mapped, so no wrap-around handling needed
        unsafe { std::slice::from_raw_parts(self.data_mmap.as_ptr().add(offset), len) }
    }

    /// Poll for available data
    fn poll_wait(&self, timeout_ms: i32) -> Result<bool> {
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
            return Err(anyhow::anyhow!("poll failed: {}", err));
        }

        Ok(ret > 0)
    }

    /// Read all available events
    fn read_events(
        &mut self,
        sender: &Sender<Event>,
        filter: &Option<EventFilter>,
    ) -> Result<usize> {
        let mut events_read = 0;
        let mut bytes_read = 0;
        let mut cons_pos = self.consumer_pos();
        let prod_pos = self.producer_pos();

        // Update metrics with current positions
        metrics::set_ringbuf_positions(cons_pos, prod_pos);

        while cons_pos < prod_pos {
            let header = self.read_header(cons_pos as usize);

            // Check if record is still being written
            if ringbuf_rec_is_busy(header) {
                // Producer still writing, yield and retry
                break;
            }

            let len = ringbuf_rec_len(header);
            let record_size = BPF_RINGBUF_HDR_SZ + round_up_8(len) as usize;

            // Check if record was discarded
            if !ringbuf_rec_is_discarded(header) {
                // Valid record, parse it
                let data = self.record_data(cons_pos as usize, len as usize);
                bytes_read += data.len();

                match Event::from_bytes(data) {
                    Ok(event) => {
                        // Apply filter if configured
                        let should_send =
                            filter.as_ref().map(|f| f.matches(&event)).unwrap_or(true);

                        if should_send {
                            match sender.try_send(event) {
                                Ok(()) => {
                                    events_read += 1;
                                    metrics::record_event_received("ringbuf", "bpf");
                                }
                                Err(crossbeam::channel::TrySendError::Full(_)) => {
                                    metrics::record_event_dropped();
                                    self.stats.record_drop(1);
                                    trace!("Event queue full, dropping event");
                                }
                                Err(crossbeam::channel::TrySendError::Disconnected(_)) => {
                                    return Err(anyhow::anyhow!("Channel disconnected"));
                                }
                            }
                        }
                    }
                    Err(e) => {
                        trace!(error = %e, len = len, "Failed to parse event");
                        self.stats.record_error();
                    }
                }
            }

            cons_pos += record_size as u64;
        }

        // Update consumer position to signal kernel
        if cons_pos != self.consumer_pos() {
            self.set_consumer_pos(cons_pos);
        }

        // Record statistics
        if events_read > 0 {
            self.stats
                .record_read(events_read as u64, bytes_read as u64, "ringbuf");
        }

        Ok(events_read)
    }

    /// Run the collector loop
    pub async fn run(
        mut self,
        sender: Sender<Event>,
        shutdown: Arc<AtomicBool>,
        filter: Option<EventFilter>,
    ) -> Result<()> {
        debug!(path = %self.path.display(), "Ring buffer collector starting");

        let poll_timeout_ms = 100;

        while !shutdown.load(Ordering::Relaxed) {
            // Poll for available data
            match self.poll_wait(poll_timeout_ms) {
                Ok(true) => {
                    // Data available
                    match self.read_events(&sender, &filter) {
                        Ok(count) => {
                            if count > 0 {
                                trace!(events = count, "Read events from ring buffer");
                            }
                        }
                        Err(e) => {
                            if e.to_string().contains("disconnected") {
                                debug!("Channel disconnected, stopping");
                                break;
                            }
                            warn!(error = %e, "Error reading ring buffer");
                            self.stats.record_error();
                        }
                    }
                }
                Ok(false) => {
                    // Timeout, just continue
                }
                Err(e) => {
                    warn!(error = %e, "Poll error");
                    self.stats.record_error();
                    tokio::time::sleep(Duration::from_millis(10)).await;
                }
            }

            // Yield to allow other tasks to run
            tokio::task::yield_now().await;
        }

        debug!(
            events = self.stats.events_read.load(Ordering::Relaxed),
            "Ring buffer collector stopped"
        );
        Ok(())
    }
}

impl Drop for RingBufCollector {
    fn drop(&mut self) {
        unsafe {
            libc::close(self.fd);
        }
    }
}

// SAFETY: The collector manages its own memory and fd, safe to send between threads
unsafe impl Send for RingBufCollector {}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_ringbuf_rec_len() {
        assert_eq!(ringbuf_rec_len(100), 100);
        assert_eq!(ringbuf_rec_len(0x0FFFFFFF), 0x0FFFFFFF);
        assert_eq!(ringbuf_rec_len(0xFFFFFFFF), 0x0FFFFFFF);
    }

    #[test]
    fn test_ringbuf_rec_is_busy() {
        assert!(!ringbuf_rec_is_busy(0));
        assert!(!ringbuf_rec_is_busy(100));
        assert!(ringbuf_rec_is_busy(BPF_RINGBUF_BUSY_BIT));
        assert!(ringbuf_rec_is_busy(BPF_RINGBUF_BUSY_BIT | 100));
    }

    #[test]
    fn test_ringbuf_rec_is_discarded() {
        assert!(!ringbuf_rec_is_discarded(0));
        assert!(!ringbuf_rec_is_discarded(100));
        assert!(ringbuf_rec_is_discarded(BPF_RINGBUF_DISCARD_BIT));
        assert!(ringbuf_rec_is_discarded(BPF_RINGBUF_DISCARD_BIT | 100));
    }

    #[test]
    fn test_round_up_8() {
        assert_eq!(round_up_8(0), 0);
        assert_eq!(round_up_8(1), 8);
        assert_eq!(round_up_8(7), 8);
        assert_eq!(round_up_8(8), 8);
        assert_eq!(round_up_8(9), 16);
        assert_eq!(round_up_8(15), 16);
        assert_eq!(round_up_8(16), 16);
    }
}
