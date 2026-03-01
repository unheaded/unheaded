// SPDX-License-Identifier: GPL-2.0-only
// Unheaded Kingdom — High-performance packet RX/TX engine
//
// Wraps XskSocket + Umem into a unified packet processing engine with
// burst-oriented API, statistics tracking, event loop, and signal handling.
//
// Integration test scenario (loopback):
// 1. Create XdpEngine on veth0 (virtual interface)
// 2. Attach xdp_redirect eBPF program to kernel
// 3. Register XSK socket in XSKMAP
// 4. Craft a test packet (ETH+IP+ICMP)
// 5. Transmit via tx_burst()
// 6. Expect packet to loopback via kernel (requires kernel support)
// 7. Receive via rx_burst()
// 8. Verify packet integrity
//
// Status: Marked [STUCK] without real AF_XDP-capable NIC
//
// Busy-poll mode (SO_BUSY_POLL):
// Set socket option with setsockopt(socket_fd, SOL_SOCKET, SO_BUSY_POLL, &timeout)
// Enables busy-waiting in kernel for lowest latency.
// Trade-off: higher CPU usage vs. reduced latency (typically <10us).

use crate::umem::Umem;
use crate::xsk::XskSocket;
use af_xdp_common::{XskConfig, XskDesc, DEFAULT_FRAME_COUNT, DEFAULT_FRAME_SIZE};
use std::os::unix::io::RawFd;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;

// ── PacketBuf ────────────────────────────────────────────────────────────────

/// A received or to-be-transmitted packet buffer.
/// References a frame in UMEM by address + length.
pub struct PacketBuf {
    /// Frame address in UMEM (byte offset from base)
    pub addr: u64,
    /// Packet data length
    pub len: u32,
}

impl PacketBuf {
    /// Get read-only slice of packet data from UMEM.
    ///
    /// # Safety
    /// Caller must ensure addr + len lies within UMEM bounds.
    pub fn as_slice<'a>(&self, umem: &'a Umem) -> &'a [u8] {
        if self.len == 0 {
            return &[];
        }
        let ptr = umem.frame_ptr(self.addr);
        unsafe { std::slice::from_raw_parts(ptr, self.len as usize) }
    }

    /// Get mutable slice of packet data from UMEM.
    ///
    /// # Safety
    /// Caller must ensure addr + len lies within UMEM bounds and no
    /// aliasing references exist.
    pub fn as_mut_slice<'a>(&self, umem: &'a mut Umem) -> &'a mut [u8] {
        if self.len == 0 {
            return &mut [];
        }
        let ptr = umem.frame_ptr(self.addr);
        unsafe { std::slice::from_raw_parts_mut(ptr, self.len as usize) }
    }
}

// ── EngineStats ──────────────────────────────────────────────────────────────

/// Statistics snapshot from the XDP engine.
pub struct EngineStats {
    pub rx_packets: u64,
    pub tx_packets: u64,
    pub rx_bytes: u64,
    pub tx_bytes: u64,
    pub rx_drops: u64,
    pub fill_empty: u64,
    pub comp_full: u64,
}

// ── XdpEngine ────────────────────────────────────────────────────────────────

/// High-performance packet RX/TX engine.
///
/// Manages AF_XDP socket, UMEM, and the four ring buffers (fill, completion,
/// RX, TX). Provides burst-oriented API for efficient packet processing.
///
/// # Safety
/// - Assumes single-threaded access (no internal synchronization)
/// - UMEM frames must not be double-freed
/// - Packet buffers are valid only during same call context
pub struct XdpEngine {
    socket: XskSocket,
    umem: Umem,

    // Statistics
    rx_packets: u64,
    tx_packets: u64,
    rx_bytes: u64,
    tx_bytes: u64,
    rx_drops: u64,
    fill_empty: u64,
    comp_full: u64,
}

impl XdpEngine {
    /// Create a new XDP engine bound to interface and queue.
    pub fn new(iface: &str, queue: u32, frame_count: u32) -> Result<Self, &'static str> {
        let config = XskConfig {
            frame_size: DEFAULT_FRAME_SIZE,
            frame_count: if frame_count > 0 {
                frame_count
            } else {
                DEFAULT_FRAME_COUNT
            },
            headroom: 0,
            flags: 0,
        };

        let mut umem = Umem::new(config)?;
        let ring_size = config.frame_count; // one descriptor per frame
        let socket = XskSocket::new(&mut umem, iface, queue, ring_size)?;

        Ok(XdpEngine {
            socket,
            umem,
            rx_packets: 0,
            tx_packets: 0,
            rx_bytes: 0,
            tx_bytes: 0,
            rx_drops: 0,
            fill_empty: 0,
            comp_full: 0,
        })
    }

    /// Receive a burst of packets from the RX ring.
    /// Returns packet buffers referencing UMEM frames.
    pub fn rx_burst(&mut self, batch_size: u32) -> Vec<PacketBuf> {
        // Recycle completed TX frames back to UMEM before receiving
        self.socket.complete_cycle(&mut self.umem);

        let descs = self.socket.recv(batch_size);
        if descs.is_empty() {
            self.rx_drops += 1;
            return Vec::new();
        }

        let mut packets = Vec::with_capacity(descs.len());
        for desc in &descs {
            self.rx_bytes += desc.len as u64;
            packets.push(PacketBuf {
                addr: desc.addr,
                len: desc.len,
            });
        }

        self.rx_packets += packets.len() as u64;

        // Refill fill ring with free frames
        self.refill_fill_ring();

        packets
    }

    /// Transmit a burst of packets via the TX ring.
    /// Returns number of packets successfully submitted.
    pub fn tx_burst(&mut self, packets: &[PacketBuf]) -> usize {
        // Drain completion ring first to free up TX space
        self.socket.complete_cycle(&mut self.umem);

        // Build descriptors
        let descs: Vec<XskDesc> = packets
            .iter()
            .map(|pkt| XskDesc {
                addr: pkt.addr,
                len: pkt.len,
                options: 0,
            })
            .collect();

        let sent = self.socket.send(&descs) as usize;

        // Kick TX if needed
        if sent > 0 && self.socket.tx_needs_wakeup() {
            let _ = self.socket.kick_tx();
        }

        for pkt in &packets[..sent] {
            self.tx_bytes += pkt.len as u64;
        }
        self.tx_packets += sent as u64;

        if sent < packets.len() {
            self.comp_full += 1;
        }

        sent
    }

    /// Allocate a UMEM frame for constructing a packet to transmit.
    pub fn alloc_frame(&mut self) -> Option<u64> {
        self.umem.alloc_frame()
    }

    /// Return a frame to the free pool (after processing).
    pub fn free_frame(&mut self, addr: u64) {
        self.umem.free_frame(addr);
    }

    /// Get a mutable pointer to the frame data for writing.
    pub fn frame_ptr(&self, addr: u64) -> *mut u8 {
        self.umem.frame_ptr(addr)
    }

    /// Refill the fill ring with free UMEM frames.
    fn refill_fill_ring(&mut self) {
        let mut addrs = Vec::new();
        while let Some(addr) = self.umem.alloc_frame() {
            addrs.push(addr);
            if addrs.len() >= 64 {
                break; // Batch limit
            }
        }

        if addrs.is_empty() {
            self.fill_empty += 1;
            return;
        }

        let filled = self.socket.fill_frames(&addrs);
        // Return unused frames back to pool
        for addr in &addrs[filled as usize..] {
            self.umem.free_frame(*addr);
        }
    }

    /// Get statistics snapshot.
    pub fn stats(&self) -> EngineStats {
        EngineStats {
            rx_packets: self.rx_packets,
            tx_packets: self.tx_packets,
            rx_bytes: self.rx_bytes,
            tx_bytes: self.tx_bytes,
            rx_drops: self.rx_drops,
            fill_empty: self.fill_empty,
            comp_full: self.comp_full,
        }
    }

    /// Poll for events with timeout.
    pub fn poll(&self, timeout_ms: i32) -> Result<bool, &'static str> {
        // POLLIN = 0x0001
        let revents = self.socket.poll(0x0001, timeout_ms)?;
        Ok(revents != 0)
    }

    /// Get the underlying socket file descriptor.
    pub fn fd(&self) -> i32 {
        self.socket.fd()
    }

    /// Shutdown gracefully: drain rings and release resources.
    pub fn shutdown(&mut self) -> Result<(), &'static str> {
        // Drain remaining RX packets
        loop {
            let pkts = self.rx_burst(256);
            if pkts.is_empty() {
                break;
            }
            // Free received frames back to UMEM
            for pkt in &pkts {
                self.umem.free_frame(pkt.addr);
            }
        }

        // Drain completion ring
        self.socket.complete_cycle(&mut self.umem);

        Ok(())
    }
}

impl Drop for XdpEngine {
    fn drop(&mut self) {
        let _ = self.shutdown();
    }
}

// ── EventLoop ────────────────────────────────────────────────────────────────

/// Epoll-based event loop skeleton for AF_XDP socket.
///
/// Uses epoll to efficiently wait for RX/TX events instead of busy-polling.
/// For lowest latency, use SO_BUSY_POLL instead.
pub struct EventLoop {
    epoll_fd: RawFd,
    socket_fd: RawFd,
}

impl EventLoop {
    /// Create event loop for the given socket fd.
    pub fn new(socket_fd: RawFd) -> Result<Self, &'static str> {
        // epoll_create1(EPOLL_CLOEXEC=0x80000)
        let epoll_fd = unsafe { libc_epoll_create1(0x80000) };
        if epoll_fd < 0 {
            return Err("epoll_create1 failed");
        }

        // epoll_ctl(ADD, socket_fd, EPOLLIN)
        let mut event = EpollEvent {
            events: 0x001, // EPOLLIN
            data: socket_fd as u64,
        };
        let ret = unsafe {
            libc_epoll_ctl(epoll_fd, 1, socket_fd, &mut event) // EPOLL_CTL_ADD=1
        };
        if ret < 0 {
            unsafe { crate::syscall::close(epoll_fd) };
            return Err("epoll_ctl ADD failed");
        }

        Ok(EventLoop {
            epoll_fd,
            socket_fd,
        })
    }

    /// Wait for events with timeout (milliseconds).
    /// Returns number of ready events (0 = timeout).
    pub fn wait(&self, timeout_ms: i32) -> Result<usize, &'static str> {
        let mut events = [EpollEvent { events: 0, data: 0 }; 16];
        let n = unsafe {
            libc_epoll_wait(
                self.epoll_fd,
                events.as_mut_ptr(),
                events.len() as i32,
                timeout_ms,
            )
        };
        if n < 0 {
            return Err("epoll_wait failed");
        }
        Ok(n as usize)
    }
}

impl Drop for EventLoop {
    fn drop(&mut self) {
        unsafe { crate::syscall::close(self.epoll_fd) };
    }
}

/// Minimal epoll event structure (matches kernel layout).
#[repr(C, packed)]
#[derive(Clone, Copy)]
struct EpollEvent {
    events: u32,
    data: u64,
}

// Raw epoll syscalls (no libc dep)
unsafe fn libc_epoll_create1(flags: i32) -> i32 {
    let ret: i64;
    std::arch::asm!(
        "syscall",
        in("rax") 291_u64, // SYS_epoll_create1
        in("rdi") flags as u64,
        lateout("rax") ret,
        lateout("rcx") _,
        lateout("r11") _,
    );
    ret as i32
}

unsafe fn libc_epoll_ctl(epfd: i32, op: i32, fd: i32, event: *mut EpollEvent) -> i32 {
    let ret: i64;
    std::arch::asm!(
        "syscall",
        in("rax") 233_u64, // SYS_epoll_ctl
        in("rdi") epfd as u64,
        in("rsi") op as u64,
        in("rdx") fd as u64,
        in("r10") event as u64,
        lateout("rax") ret,
        lateout("rcx") _,
        lateout("r11") _,
    );
    ret as i32
}

unsafe fn libc_epoll_wait(epfd: i32, events: *mut EpollEvent, max: i32, timeout: i32) -> i32 {
    let ret: i64;
    std::arch::asm!(
        "syscall",
        in("rax") 232_u64, // SYS_epoll_wait
        in("rdi") epfd as u64,
        in("rsi") events as u64,
        in("rdx") max as u64,
        in("r10") timeout as u64,
        lateout("rax") ret,
        lateout("rcx") _,
        lateout("r11") _,
    );
    ret as i32
}

// ── SignalHandler ────────────────────────────────────────────────────────────

/// Cooperative signal handler for graceful shutdown.
/// Share the handler across threads; call `trigger()` from signal context.
pub struct SignalHandler {
    should_exit: Arc<AtomicBool>,
}

impl SignalHandler {
    pub fn new() -> Self {
        SignalHandler {
            should_exit: Arc::new(AtomicBool::new(false)),
        }
    }

    /// Check if shutdown was requested.
    pub fn should_exit(&self) -> bool {
        self.should_exit.load(Ordering::Relaxed)
    }

    /// Trigger shutdown.
    pub fn trigger(&self) {
        self.should_exit.store(true, Ordering::Release);
    }

    /// Get a clone of the inner flag for sharing across threads.
    pub fn flag(&self) -> Arc<AtomicBool> {
        Arc::clone(&self.should_exit)
    }
}

impl Default for SignalHandler {
    fn default() -> Self {
        Self::new()
    }
}
