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
use crate::events::Event;
use crate::metrics;

/// Ring buffer record header bits
const BPF_RINGBUF_BUSY_BIT: u32 = 1 << 31;
const BPF_RINGBUF_DISCARD_BIT: u32 = 1 << 30;
const BPF_RINGBUF_LEN_MASK: u32 = (1 << 28) - 1;

/// Page size (typically 4096)
const PAGE_SIZE: usize = 4096;

/// Ring buffer reader for zero-copy event reading
pub struct RingBufReader {
    /// Memory-mapped producer page
    producer_mmap: MmapMut,
    /// Memory-mapped consumer page
    consumer_mmap: MmapMut,
    /// Memory-mapped data pages
    data_mmap: MmapMut,
    /// Ring buffer size (data portion only)
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

        // Producer page (read-only, offset 0)
        let producer_mmap = unsafe {
            MmapOptions::new()
                .len(PAGE_SIZE)
                .offset(0)
                .map_mut(&std::fs::File::from_raw_fd(fd))
                .map_err(|e| BpfError::Mmap(e.to_string()))?
        };

        // Consumer page (read-write, offset PAGE_SIZE)
        let consumer_mmap = unsafe {
            MmapOptions::new()
                .len(PAGE_SIZE)
                .offset(PAGE_SIZE as u64)
                .map_mut(&std::fs::File::from_raw_fd(fd))
                .map_err(|e| BpfError::Mmap(e.to_string()))?
        };

        // Data pages (offset 2*PAGE_SIZE, size = 2 * data_size for wrap-around)
        let data_mmap = unsafe {
            MmapOptions::new()
                .len(data_size * 2)
                .offset((2 * PAGE_SIZE) as u64)
                .map_mut(&std::fs::File::from_raw_fd(fd))
                .map_err(|e| BpfError::Mmap(e.to_string()))?
        };

        Ok(Self {
            producer_mmap,
            consumer_mmap,
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

    /// Poll for available data
    fn poll_wait(&self, timeout_ms: i32) -> Result<bool, BpfError> {
        use std::os::unix::io::AsRawFd;

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

                match Event::from_bytes(data) {
                    Ok(event) => {
                        // Record processing latency
                        let now = std::time::Instant::now();

                        if sender.try_send(event).is_err() {
                            // Channel full, record drop
                            metrics::record_event_dropped();
                            warn!("Event queue full, dropping event");
                        } else {
                            events_read += 1;
                            metrics::record_event_received("ringbuf", "bpf");
                        }

                        // This is just measuring our processing time, not the full latency
                        metrics::record_processing_latency_ns(now.elapsed().as_nanos() as u64);
                    }
                    Err(e) => {
                        trace!(error = %e, "Failed to parse event");
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
    pub async fn run(
        mut self,
        sender: Sender<Event>,
        shutdown: Arc<AtomicBool>,
    ) -> Result<(), BpfError> {
        debug!("Ring buffer reader starting");

        while !shutdown.load(Ordering::Relaxed) {
            // Poll for data with timeout
            match self.poll_wait(100) {
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
