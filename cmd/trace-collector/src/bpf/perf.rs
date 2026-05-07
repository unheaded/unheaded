//! Perf Event Array reading.
//!
//! Perf event arrays are older than ring buffers but still widely used.
//! Each CPU has its own perf buffer for lock-free per-CPU event submission.
//!
//! Memory layout per CPU:
//! - Data pages with perf event headers
//! - Each event has a perf_event_header followed by sample data

use std::os::unix::io::FromRawFd;
use std::path::Path;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;
use std::time::Duration;

use anyhow::Result;
use crossbeam::channel::Sender;
use memmap2::{MmapMut, MmapOptions};
use tracing::{debug, trace, warn};

use super::{bpf_map_info, bpf_obj_get, BpfError, BpfMapType};
use crate::events::Event;
use crate::metrics;

/// Page size
const PAGE_SIZE: usize = 4096;

/// Perf event header types
#[repr(u32)]
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum PerfEventType {
    MMap = 1,
    Lost = 2,
    Comm = 3,
    Exit = 4,
    Throttle = 5,
    Unthrottle = 6,
    Fork = 7,
    Read = 8,
    Sample = 9,
    MMap2 = 10,
}

/// Perf event header (from perf_event.h)
#[repr(C)]
#[derive(Debug, Clone, Copy)]
pub struct PerfEventHeader {
    pub event_type: u32,
    pub misc: u16,
    pub size: u16,
}

/// Perf event mmap page (from perf_event.h)
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
    pub data_head: u64, // Head pointer, written by kernel
    pub data_tail: u64, // Tail pointer, written by userspace
    pub data_offset: u64,
    pub data_size: u64,
    pub aux_head: u64,
    pub aux_tail: u64,
    pub aux_offset: u64,
    pub aux_size: u64,
}

/// Lost event structure
#[repr(C)]
#[derive(Debug, Clone, Copy)]
pub struct PerfEventLost {
    pub header: PerfEventHeader,
    pub id: u64,
    pub lost: u64,
}

/// Sample event structure (custom, after BPF_PERF_OUTPUT)
#[repr(C)]
#[derive(Debug, Clone, Copy)]
pub struct PerfEventSample {
    pub header: PerfEventHeader,
    pub size: u32,
    // data follows
}

/// Perf event reader for a single CPU
pub struct PerfEventReader {
    /// Memory-mapped buffer
    mmap: MmapMut,
    /// Buffer size (data portion)
    buffer_size: usize,
    /// CPU this reader is for
    cpu: usize,
    /// File descriptor
    fd: i32,
    /// Map file descriptor
    map_fd: i32,
}

impl PerfEventReader {
    /// Create a new perf event reader for a specific CPU
    pub fn new(path: &Path, cpu: usize, num_pages: usize) -> Result<Self, BpfError> {
        // Open the pinned map
        let map_fd = bpf_obj_get(path)?;

        // Verify it's a perf event array
        let info = bpf_map_info(map_fd)?;
        if info.map_type != BpfMapType::PerfEventArray as u32 {
            return Err(BpfError::InvalidMapType {
                expected: BpfMapType::PerfEventArray as u32,
                actual: info.map_type,
            });
        }

        debug!(
            name = info.name_str(),
            cpu = cpu,
            pages = num_pages,
            "Opening perf event buffer"
        );

        // Create perf event for this CPU
        let fd = Self::create_perf_event(cpu)?;

        // Update the BPF map with this fd
        Self::update_perf_map(map_fd, cpu as u32, fd)?;

        // mmap the perf buffer
        // Size = 1 page for metadata + num_pages for data
        let mmap_size = (1 + num_pages) * PAGE_SIZE;

        let mmap = unsafe {
            MmapOptions::new()
                .len(mmap_size)
                .map_mut(&std::fs::File::from_raw_fd(fd))
                .map_err(|e| BpfError::Mmap(e.to_string()))?
        };

        Ok(Self {
            mmap,
            buffer_size: num_pages * PAGE_SIZE,
            cpu,
            fd,
            map_fd,
        })
    }

    /// Create a perf event for the given CPU
    fn create_perf_event(cpu: usize) -> Result<i32, BpfError> {
        use std::mem::MaybeUninit;

        // perf_event_attr structure
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

        let mut attr: PerfEventAttr = unsafe { MaybeUninit::zeroed().assume_init() };

        // PERF_TYPE_SOFTWARE
        attr.type_ = 1;
        attr.size = std::mem::size_of::<PerfEventAttr>() as u32;
        // PERF_COUNT_SW_BPF_OUTPUT
        attr.config = 10;
        attr.sample_type = 1; // PERF_SAMPLE_RAW
        attr.wakeup_events_or_watermark = 1;

        // Flags: disabled initially
        attr.flags = 0;

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
            return Err(BpfError::PerfEvent(format!(
                "perf_event_open failed for CPU {}: {}",
                cpu,
                std::io::Error::last_os_error()
            )));
        }

        // Enable the event
        let ret = unsafe { libc::ioctl(fd as i32, 0x2400, 0) }; // PERF_EVENT_IOC_ENABLE
        if ret < 0 {
            unsafe { libc::close(fd as i32) };
            return Err(BpfError::PerfEvent(format!(
                "Failed to enable perf event: {}",
                std::io::Error::last_os_error()
            )));
        }

        Ok(fd as i32)
    }

    /// Update the BPF map with the perf event fd
    fn update_perf_map(map_fd: i32, key: u32, value: i32) -> Result<(), BpfError> {
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
            return Err(BpfError::Syscall(format!(
                "Failed to update perf map: {}",
                std::io::Error::last_os_error()
            )));
        }

        Ok(())
    }

    /// Get the mmap page
    fn mmap_page(&self) -> &PerfEventMmapPage {
        unsafe { &*(self.mmap.as_ptr() as *const PerfEventMmapPage) }
    }

    /// Get mutable mmap page (for updating tail)
    fn mmap_page_mut(&mut self) -> &mut PerfEventMmapPage {
        unsafe { &mut *(self.mmap.as_mut_ptr() as *mut PerfEventMmapPage) }
    }

    /// Get data buffer pointer
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

    /// Read available events
    fn read_events(&mut self, sender: &Sender<Event>) -> Result<usize, BpfError> {
        let mut events_read = 0;

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
            let header_data = self.read_data(tail % self.buffer_size, 8);
            let header = unsafe { *(header_data.as_ptr() as *const PerfEventHeader) };

            let event_size = header.size as usize;
            if event_size == 0 {
                warn!("Invalid perf event size");
                break;
            }

            match header.event_type {
                9 => {
                    // PERF_RECORD_SAMPLE
                    // Skip header, read sample size
                    let sample_header_data = self.read_data((tail + 8) % self.buffer_size, 4);
                    let sample_size =
                        unsafe { *(sample_header_data.as_ptr() as *const u32) } as usize;

                    // Read the actual sample data
                    let sample_data = self.read_data((tail + 12) % self.buffer_size, sample_size);

                    match Event::from_bytes(&sample_data) {
                        Ok(event) => {
                            if sender.try_send(event).is_err() {
                                metrics::record_event_dropped();
                            } else {
                                events_read += 1;
                                metrics::record_event_received("perf", &format!("cpu{}", self.cpu));
                            }
                        }
                        Err(e) => {
                            trace!(error = %e, "Failed to parse perf event");
                        }
                    }
                }
                2 => {
                    // PERF_RECORD_LOST
                    let lost_data = self.read_data(tail % self.buffer_size, 24);
                    let lost = unsafe { *(lost_data.as_ptr() as *const PerfEventLost) };
                    warn!(
                        cpu = self.cpu,
                        lost = lost.lost,
                        "Perf events lost due to buffer overflow"
                    );

                    for _ in 0..lost.lost {
                        metrics::record_event_dropped();
                    }
                }
                _ => {
                    trace!(event_type = header.event_type, "Unknown perf event type");
                }
            }

            tail += event_size;
        }

        // Update tail pointer
        // Memory barrier before writing tail
        std::sync::atomic::fence(Ordering::Release);
        self.mmap_page_mut().data_tail = tail as u64;

        // Update metrics
        metrics::set_perf_events_cpu(self.cpu, events_read as u64);

        Ok(events_read)
    }

    /// Run the reader loop
    pub async fn run(
        mut self,
        sender: Sender<Event>,
        shutdown: Arc<AtomicBool>,
    ) -> Result<(), BpfError> {
        debug!(cpu = self.cpu, "Perf event reader starting");

        while !shutdown.load(Ordering::Relaxed) {
            match self.poll_wait(100) {
                Ok(true) => match self.read_events(&sender) {
                    Ok(count) => {
                        if count > 0 {
                            trace!(cpu = self.cpu, events = count, "Read perf events");
                        }
                    }
                    Err(e) => {
                        warn!(cpu = self.cpu, error = %e, "Error reading perf events");
                    }
                },
                Ok(false) => continue,
                Err(e) => {
                    warn!(cpu = self.cpu, error = %e, "Perf poll error");
                    tokio::time::sleep(Duration::from_millis(10)).await;
                }
            }

            tokio::task::yield_now().await;
        }

        debug!(cpu = self.cpu, "Perf event reader stopped");
        Ok(())
    }
}

impl Drop for PerfEventReader {
    fn drop(&mut self) {
        // Disable the event
        unsafe {
            libc::ioctl(self.fd, 0x2401, 0); // PERF_EVENT_IOC_DISABLE
            libc::close(self.fd);
            libc::close(self.map_fd);
        }
    }
}

unsafe impl Send for PerfEventReader {}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_perf_event_header_size() {
        assert_eq!(std::mem::size_of::<PerfEventHeader>(), 8);
    }

    #[test]
    fn test_mmap_page_size() {
        // Mmap page should be exactly one page
        assert!(std::mem::size_of::<PerfEventMmapPage>() <= PAGE_SIZE);
    }
}
