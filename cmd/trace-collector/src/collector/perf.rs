//! Perf Event Array Collector - per-CPU event reading.
//!
//! THE WHISPERING VOID's secondary ears: perf event arrays provide
//! per-CPU event buffers for high-throughput scenarios.
//!
//! Perf event arrays are older than ring buffers but offer advantages
//! for certain workloads:
//! - Per-CPU buffers eliminate contention
//! - Native integration with perf infrastructure
//! - Good for high-frequency per-CPU events
//!
//! Memory layout:
//! - Metadata page: Contains head/tail pointers
//! - Data pages: Ring buffer of perf_event_header records

use std::path::PathBuf;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;
use std::time::Duration;

use anyhow::{Context, Result};
use crossbeam::channel::Sender;
use memmap2::{MmapMut, MmapOptions};
use tracing::{debug, error, trace, warn};

use super::events::EventFilter;
use super::CollectorStats;
use crate::bpf::{bpf_map_info, bpf_obj_get, BpfError, BpfMapType};
use crate::events::Event;
use crate::metrics;

/// Page size
const PAGE_SIZE: usize = 4096;

/// Perf event types (from perf_event.h)
const PERF_RECORD_MMAP: u32 = 1;
const PERF_RECORD_LOST: u32 = 2;
const PERF_RECORD_SAMPLE: u32 = 9;

/// Perf event header
#[repr(C)]
#[derive(Debug, Clone, Copy)]
pub struct PerfEventHeader {
    pub event_type: u32,
    pub misc: u16,
    pub size: u16,
}

impl PerfEventHeader {
    const SIZE: usize = std::mem::size_of::<Self>();
}

/// Perf event mmap page (metadata at start of mmap region)
#[repr(C)]
pub struct PerfEventMmapPage {
    pub version: u32,
    pub compat_version: u32,
    pub lock: u32,
    pub index: u32,
    pub offset: i64,
    pub time_enabled: u64,
    pub time_running: u64,
    pub capabilities: u64,
    pub pmc_width: u16,
    pub time_shift: u16,
    pub time_mult: u32,
    pub time_offset: u64,
    pub time_zero: u64,
    pub size: u32,
    pub __reserved_1: u32,
    pub time_cycles: u64,
    pub time_mask: u64,
    pub __reserved: [u8; 928],
    /// Head pointer (written by kernel)
    pub data_head: u64,
    /// Tail pointer (written by userspace)
    pub data_tail: u64,
    pub data_offset: u64,
    pub data_size: u64,
    pub aux_head: u64,
    pub aux_tail: u64,
    pub aux_offset: u64,
    pub aux_size: u64,
}

/// Lost event record
#[repr(C)]
#[derive(Debug, Clone, Copy)]
pub struct PerfEventLost {
    pub header: PerfEventHeader,
    pub id: u64,
    pub lost: u64,
}

/// Sample event record (for BPF_PERF_OUTPUT)
#[repr(C)]
#[derive(Debug, Clone, Copy)]
pub struct PerfEventSample {
    pub header: PerfEventHeader,
    pub size: u32,
    // Variable-length data follows
}

/// Perf event collector for a single CPU
pub struct PerfCollector {
    /// Path to the BPF map
    path: PathBuf,
    /// CPU this collector reads from
    cpu: usize,
    /// Memory-mapped buffer
    mmap: MmapMut,
    /// Data buffer size (excluding metadata page)
    buffer_size: usize,
    /// Perf event file descriptor
    perf_fd: i32,
    /// BPF map file descriptor
    map_fd: i32,
    /// Shared statistics
    stats: Arc<CollectorStats>,
}

impl PerfCollector {
    /// Create a new perf event collector for a specific CPU
    pub fn new(
        path: PathBuf,
        cpu: usize,
        num_pages: usize,
        stats: Arc<CollectorStats>,
    ) -> Result<Self> {
        // Open the pinned BPF map
        let map_fd =
            bpf_obj_get(&path).map_err(|e| anyhow::anyhow!("Failed to open BPF map: {}", e))?;

        // Verify it's a perf event array
        let info =
            bpf_map_info(map_fd).map_err(|e| anyhow::anyhow!("Failed to get map info: {}", e))?;

        if info.map_type != BpfMapType::PerfEventArray as u32 {
            unsafe { libc::close(map_fd) };
            return Err(anyhow::anyhow!(
                "Invalid map type: expected PerfEventArray ({}), got {}",
                BpfMapType::PerfEventArray as u32,
                info.map_type
            ));
        }

        debug!(
            path = %path.display(),
            name = info.name_str(),
            cpu = cpu,
            pages = num_pages,
            "Opening perf event buffer"
        );

        // Create perf event for this CPU
        let perf_fd = Self::create_perf_event(cpu)?;

        // Update the BPF map with this perf event fd
        Self::update_perf_map(map_fd, cpu as u32, perf_fd)?;

        // mmap the perf buffer (1 metadata page + num_pages data pages)
        let mmap_size = (1 + num_pages) * PAGE_SIZE;

        use std::os::fd::FromRawFd;
        let file = unsafe { std::fs::File::from_raw_fd(perf_fd) };

        let mmap = unsafe {
            MmapOptions::new()
                .len(mmap_size)
                .map_mut(&file)
                .context("Failed to mmap perf buffer")?
        };

        // Prevent file from being closed
        std::mem::forget(file);

        Ok(Self {
            path,
            cpu,
            mmap,
            buffer_size: num_pages * PAGE_SIZE,
            perf_fd,
            map_fd,
            stats,
        })
    }

    /// Create a perf event for the given CPU
    fn create_perf_event(cpu: usize) -> Result<i32> {
        #[repr(C)]
        struct PerfEventAttr {
            type_: u32,
            size: u32,
            config: u64,
            sample_period_or_freq: u64,
            sample_type: u64,
            read_format: u64,
            flags: u64,
            wakeup_events_or_watermark: u32,
            bp_type: u32,
            bp_addr_or_config1: u64,
            bp_len_or_config2: u64,
            branch_sample_type: u64,
            sample_regs_user: u64,
            sample_stack_user: u32,
            clockid: i32,
            sample_regs_intr: u64,
            aux_watermark: u32,
            sample_max_stack: u16,
            __reserved_2: u16,
            aux_sample_size: u32,
            __reserved_3: u32,
            sig_data: u64,
        }

        let mut attr = unsafe { std::mem::zeroed::<PerfEventAttr>() };
        attr.type_ = 1; // PERF_TYPE_SOFTWARE
        attr.size = std::mem::size_of::<PerfEventAttr>() as u32;
        attr.config = 10; // PERF_COUNT_SW_BPF_OUTPUT
        attr.sample_type = 1; // PERF_SAMPLE_RAW
        attr.wakeup_events_or_watermark = 1;

        let fd = unsafe {
            libc::syscall(
                libc::SYS_perf_event_open,
                &attr as *const _,
                -1i32,      // pid (-1 = all processes)
                cpu as i32, // cpu
                -1i32,      // group_fd
                0u64,       // flags
            )
        };

        if fd < 0 {
            return Err(anyhow::anyhow!(
                "perf_event_open failed for CPU {}: {}",
                cpu,
                std::io::Error::last_os_error()
            ));
        }

        // Enable the perf event
        // PERF_EVENT_IOC_ENABLE = 0x2400
        let ret = unsafe { libc::ioctl(fd as i32, 0x2400, 0) };
        if ret < 0 {
            unsafe { libc::close(fd as i32) };
            return Err(anyhow::anyhow!(
                "Failed to enable perf event: {}",
                std::io::Error::last_os_error()
            ));
        }

        Ok(fd as i32)
    }

    /// Update the BPF map with the perf event fd
    fn update_perf_map(map_fd: i32, key: u32, value: i32) -> Result<()> {
        #[repr(C)]
        struct BpfMapUpdateAttr {
            map_fd: u32,
            _pad: u32,
            key: u64,
            value_or_next_key: u64,
            flags: u64,
        }

        let attr = BpfMapUpdateAttr {
            map_fd: map_fd as u32,
            _pad: 0,
            key: &key as *const _ as u64,
            value_or_next_key: &value as *const _ as u64,
            flags: 0, // BPF_ANY
        };

        let ret = unsafe {
            libc::syscall(
                libc::SYS_bpf,
                2u32, // BPF_MAP_UPDATE_ELEM
                &attr as *const _,
                std::mem::size_of::<BpfMapUpdateAttr>(),
            )
        };

        if ret < 0 {
            return Err(anyhow::anyhow!(
                "Failed to update perf map: {}",
                std::io::Error::last_os_error()
            ));
        }

        Ok(())
    }

    /// Get reference to mmap page (metadata)
    #[inline]
    fn mmap_page(&self) -> &PerfEventMmapPage {
        unsafe { &*(self.mmap.as_ptr() as *const PerfEventMmapPage) }
    }

    /// Get mutable reference to mmap page
    #[inline]
    fn mmap_page_mut(&mut self) -> &mut PerfEventMmapPage {
        unsafe { &mut *(self.mmap.as_mut_ptr() as *mut PerfEventMmapPage) }
    }

    /// Get pointer to data buffer (after metadata page)
    #[inline]
    fn data_ptr(&self) -> *const u8 {
        unsafe { self.mmap.as_ptr().add(PAGE_SIZE) }
    }

    /// Read data at offset with wrap-around
    fn read_data(&self, offset: usize, len: usize) -> Vec<u8> {
        let mut result = Vec::with_capacity(len);
        let data = self.data_ptr();

        for i in 0..len {
            let pos = (offset + i) % self.buffer_size;
            result.push(unsafe { *data.add(pos) });
        }

        result
    }

    /// Poll for available data
    fn poll_wait(&self, timeout_ms: i32) -> Result<bool> {
        let mut pollfd = libc::pollfd {
            fd: self.perf_fd,
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

        // Memory barrier before reading head
        std::sync::atomic::fence(Ordering::Acquire);

        let page = self.mmap_page();
        let data_head = page.data_head;
        let data_tail = page.data_tail;

        if data_head == data_tail {
            return Ok(0);
        }

        let mut tail = data_tail as usize;
        let head = data_head as usize;

        while tail < head {
            // Read event header
            let header_data = self.read_data(tail % self.buffer_size, PerfEventHeader::SIZE);
            let header = unsafe { *(header_data.as_ptr() as *const PerfEventHeader) };

            let event_size = header.size as usize;
            if event_size == 0 || event_size > self.buffer_size {
                warn!(cpu = self.cpu, size = event_size, "Invalid perf event size");
                break;
            }

            match header.event_type {
                PERF_RECORD_SAMPLE => {
                    // Read sample size (4 bytes after header)
                    let sample_size_data =
                        self.read_data((tail + PerfEventHeader::SIZE) % self.buffer_size, 4);
                    let sample_size =
                        unsafe { *(sample_size_data.as_ptr() as *const u32) } as usize;

                    // Read the actual sample data
                    let sample_data = self.read_data(
                        (tail + PerfEventHeader::SIZE + 4) % self.buffer_size,
                        sample_size,
                    );

                    bytes_read += sample_size;

                    match Event::from_bytes(&sample_data) {
                        Ok(event) => {
                            // Apply filter if configured
                            let should_send = filter
                                .as_ref()
                                .map(|f| f.matches(&event))
                                .unwrap_or(true);

                            if should_send {
                                match sender.try_send(event) {
                                    Ok(()) => {
                                        events_read += 1;
                                        metrics::record_event_received(
                                            "perf",
                                            &format!("cpu{}", self.cpu),
                                        );
                                    }
                                    Err(crossbeam::channel::TrySendError::Full(_)) => {
                                        metrics::record_event_dropped();
                                        self.stats.record_drop(1);
                                    }
                                    Err(crossbeam::channel::TrySendError::Disconnected(_)) => {
                                        return Err(anyhow::anyhow!("Channel disconnected"));
                                    }
                                }
                            }
                        }
                        Err(e) => {
                            trace!(cpu = self.cpu, error = %e, "Failed to parse perf event");
                            self.stats.record_error();
                        }
                    }
                }
                PERF_RECORD_LOST => {
                    // Lost events record
                    let lost_data =
                        self.read_data(tail % self.buffer_size, std::mem::size_of::<PerfEventLost>());
                    let lost = unsafe { *(lost_data.as_ptr() as *const PerfEventLost) };

                    warn!(
                        cpu = self.cpu,
                        lost = lost.lost,
                        "Perf events lost due to buffer overflow"
                    );

                    self.stats.record_drop(lost.lost);
                    for _ in 0..lost.lost {
                        metrics::record_event_dropped();
                    }
                }
                _ => {
                    trace!(
                        cpu = self.cpu,
                        event_type = header.event_type,
                        "Unknown perf event type"
                    );
                }
            }

            tail += event_size;
        }

        // Update tail pointer (signals kernel we've consumed data)
        std::sync::atomic::fence(Ordering::Release);
        self.mmap_page_mut().data_tail = tail as u64;

        // Record statistics
        if events_read > 0 {
            self.stats
                .record_read(events_read as u64, bytes_read as u64, "perf");
            metrics::set_perf_events_cpu(self.cpu, events_read as u64);
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
        debug!(
            cpu = self.cpu,
            path = %self.path.display(),
            "Perf event collector starting"
        );

        let poll_timeout_ms = 100;

        while !shutdown.load(Ordering::Relaxed) {
            match self.poll_wait(poll_timeout_ms) {
                Ok(true) => {
                    match self.read_events(&sender, &filter) {
                        Ok(count) => {
                            if count > 0 {
                                trace!(cpu = self.cpu, events = count, "Read perf events");
                            }
                        }
                        Err(e) => {
                            if e.to_string().contains("disconnected") {
                                debug!(cpu = self.cpu, "Channel disconnected, stopping");
                                break;
                            }
                            warn!(cpu = self.cpu, error = %e, "Error reading perf events");
                            self.stats.record_error();
                        }
                    }
                }
                Ok(false) => {
                    // Timeout, continue
                }
                Err(e) => {
                    warn!(cpu = self.cpu, error = %e, "Perf poll error");
                    self.stats.record_error();
                    tokio::time::sleep(Duration::from_millis(10)).await;
                }
            }

            tokio::task::yield_now().await;
        }

        debug!(cpu = self.cpu, "Perf event collector stopped");
        Ok(())
    }
}

impl Drop for PerfCollector {
    fn drop(&mut self) {
        // Disable the perf event
        // PERF_EVENT_IOC_DISABLE = 0x2401
        unsafe {
            libc::ioctl(self.perf_fd, 0x2401, 0);
            libc::close(self.perf_fd);
            libc::close(self.map_fd);
        }
    }
}

// SAFETY: The collector manages its own memory and fds, safe to send between threads
unsafe impl Send for PerfCollector {}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_perf_event_header_size() {
        assert_eq!(PerfEventHeader::SIZE, 8);
    }

    #[test]
    fn test_mmap_page_size() {
        // Mmap page should fit within one page
        assert!(std::mem::size_of::<PerfEventMmapPage>() <= PAGE_SIZE);
    }

    #[test]
    fn test_perf_event_lost_size() {
        assert_eq!(std::mem::size_of::<PerfEventLost>(), 24);
    }
}
