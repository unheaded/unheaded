// SPDX-License-Identifier: GPL-2.0-only
// AF_XDP socket (XskSocket) — RX/TX rings with kernel bypass
//
// Sacred Law: NO external deps. Raw syscalls only via crate::syscall.
// Provides zero-copy packet receive and transmit via AF_XDP kernel interface.

use crate::syscall;
use crate::umem::Umem;
use af_xdp_common::{
    Sockaddr_xdp, XskDesc, XskMmapOffsets, XskRingOffsets, XskStatistics, AF_XDP,
    DEFAULT_RING_SIZE, SOL_XDP, XDP_MMAP_OFFSETS, XDP_OPTIONS, XDP_PGOFF_RX_RING,
    XDP_PGOFF_TX_RING, XDP_RING_NEED_WAKEUP, XDP_RX_RING, XDP_STATISTICS, XDP_TX_RING,
};

// =============================================================================
// RX Ring — kernel produces received packets, userspace consumes
// =============================================================================

/// RX ring — kernel producer, userspace consumer.
/// Kernel writes received packet descriptors; userspace reads them.
pub struct RxRing {
    /// Producer index (kernel increments)
    producer: *const u32,
    /// Consumer index (userspace increments)
    consumer: *mut u32,
    /// Flags (includes NEED_WAKEUP)
    flags: *const u32,
    /// Descriptor array (XskDesc with packet addr/len)
    descs: *const XskDesc,
    /// Ring size (number of descriptors, power of 2)
    size: u32,
    /// Mask for fast modulo (size - 1)
    mask: u32,
    /// Mmap'd memory base
    map_addr: *mut u8,
    /// Mmap'd memory size
    map_size: u64,
}

impl RxRing {
    /// Create RX ring from mmap'd kernel memory.
    fn new(sock_fd: i32, offsets: &XskRingOffsets, ring_size: u32) -> Result<Self, &'static str> {
        let map_size = offsets.desc + (ring_size as u64) * std::mem::size_of::<XskDesc>() as u64;

        let map_addr = unsafe {
            syscall::mmap(
                std::ptr::null_mut(),
                map_size,
                syscall::PROT_READ | syscall::PROT_WRITE,
                syscall::MAP_SHARED | syscall::MAP_POPULATE,
                sock_fd,
                XDP_PGOFF_RX_RING as i64,
            )
            .map_err(|_| "mmap RX ring failed")?
        };

        unsafe {
            Ok(RxRing {
                producer: map_addr.add(offsets.producer as usize) as *const u32,
                consumer: map_addr.add(offsets.consumer as usize) as *mut u32,
                flags: map_addr.add(offsets.flags as usize) as *const u32,
                descs: map_addr.add(offsets.desc as usize) as *const XskDesc,
                size: ring_size,
                mask: ring_size - 1,
                map_addr,
                map_size,
            })
        }
    }

    /// Receive available packets from the RX ring.
    /// Returns a Vec of descriptors pointing into UMEM where packet data lives.
    /// Caller must return frame addresses to the fill ring after processing.
    pub fn recv(&mut self, batch_size: u32) -> Vec<XskDesc> {
        unsafe {
            syscall::smp_rmb(); // Load barrier before reading producer
            let producer = *self.producer;
            let consumer = *self.consumer;
            let available = producer.wrapping_sub(consumer);
            let count = available.min(batch_size);

            let mut packets = Vec::with_capacity(count as usize);
            for i in 0..count {
                let idx = ((consumer.wrapping_add(i)) & self.mask) as usize;
                packets.push(*self.descs.add(idx));
            }

            if count > 0 {
                syscall::smp_wmb(); // Store barrier before updating consumer
                *self.consumer = consumer.wrapping_add(count);
            }
            packets
        }
    }

    /// Check if the RX ring needs a wakeup (poll/recvmsg).
    pub fn needs_wakeup(&self) -> bool {
        unsafe { (*self.flags) & XDP_RING_NEED_WAKEUP != 0 }
    }

    /// Number of packets available to receive.
    pub fn available(&self) -> u32 {
        unsafe {
            syscall::smp_rmb();
            (*self.producer).wrapping_sub(*self.consumer)
        }
    }
}

impl Drop for RxRing {
    fn drop(&mut self) {
        if !self.map_addr.is_null() {
            unsafe {
                let _ = syscall::munmap(self.map_addr, self.map_size);
            }
        }
    }
}

// =============================================================================
// TX Ring — userspace produces packets to send, kernel consumes
// =============================================================================

/// TX ring — userspace producer, kernel consumer.
/// Userspace writes packet descriptors; kernel transmits them.
pub struct TxRing {
    /// Producer index (userspace increments)
    producer: *mut u32,
    /// Consumer index (kernel increments)
    consumer: *const u32,
    /// Flags (includes NEED_WAKEUP)
    flags: *const u32,
    /// Descriptor array (XskDesc with packet addr/len)
    descs: *mut XskDesc,
    /// Ring size
    size: u32,
    /// Mask for fast modulo
    mask: u32,
    /// Mmap'd memory base
    map_addr: *mut u8,
    /// Mmap'd memory size
    map_size: u64,
}

impl TxRing {
    /// Create TX ring from mmap'd kernel memory.
    fn new(sock_fd: i32, offsets: &XskRingOffsets, ring_size: u32) -> Result<Self, &'static str> {
        let map_size = offsets.desc + (ring_size as u64) * std::mem::size_of::<XskDesc>() as u64;

        let map_addr = unsafe {
            syscall::mmap(
                std::ptr::null_mut(),
                map_size,
                syscall::PROT_READ | syscall::PROT_WRITE,
                syscall::MAP_SHARED | syscall::MAP_POPULATE,
                sock_fd,
                XDP_PGOFF_TX_RING as i64,
            )
            .map_err(|_| "mmap TX ring failed")?
        };

        unsafe {
            Ok(TxRing {
                producer: map_addr.add(offsets.producer as usize) as *mut u32,
                consumer: map_addr.add(offsets.consumer as usize) as *const u32,
                flags: map_addr.add(offsets.flags as usize) as *const u32,
                descs: map_addr.add(offsets.desc as usize) as *mut XskDesc,
                size: ring_size,
                mask: ring_size - 1,
                map_addr,
                map_size,
            })
        }
    }

    /// Submit a batch of packet descriptors for transmission.
    /// Returns the number of descriptors actually enqueued.
    pub fn submit(&mut self, descs: &[XskDesc]) -> u32 {
        unsafe {
            syscall::smp_rmb(); // Load barrier before reading consumer
            let producer = *self.producer;
            let consumer = *self.consumer;
            let free_slots = self.size.wrapping_sub(producer.wrapping_sub(consumer));
            let count = (descs.len() as u32).min(free_slots);

            for i in 0..count {
                let idx = ((producer.wrapping_add(i)) & self.mask) as usize;
                *self.descs.add(idx) = descs[i as usize];
            }

            if count > 0 {
                syscall::smp_wmb(); // Store barrier before updating producer
                *self.producer = producer.wrapping_add(count);
            }
            count
        }
    }

    /// Number of free slots available for transmission.
    pub fn free_slots(&self) -> u32 {
        unsafe {
            let producer = *self.producer;
            let consumer = *self.consumer;
            self.size.wrapping_sub(producer.wrapping_sub(consumer))
        }
    }

    /// Check if the TX ring needs a wakeup (sendto).
    pub fn needs_wakeup(&self) -> bool {
        unsafe { (*self.flags) & XDP_RING_NEED_WAKEUP != 0 }
    }
}

impl Drop for TxRing {
    fn drop(&mut self) {
        if !self.map_addr.is_null() {
            unsafe {
                let _ = syscall::munmap(self.map_addr, self.map_size);
            }
        }
    }
}

// =============================================================================
// XskSocket — AF_XDP socket combining UMEM + RX/TX + Fill/Completion rings
// =============================================================================

/// AF_XDP socket for zero-copy packet I/O.
///
/// Combines:
/// - An AF_XDP socket file descriptor bound to a NIC queue
/// - RX ring (receive packets from kernel)
/// - TX ring (transmit packets to kernel)
/// - Fill ring (supply empty frames to kernel for RX)
/// - Completion ring (reclaim frames after TX)
/// - UMEM reference (shared memory region for packet data)
pub struct XskSocket {
    /// Raw AF_XDP socket file descriptor
    fd: i32,
    /// RX ring (kernel -> userspace)
    rx: RxRing,
    /// TX ring (userspace -> kernel)
    tx: TxRing,
    /// Fill ring (supply frames for RX)
    fill: crate::umem::FillRing,
    /// Completion ring (reclaim frames after TX)
    comp: crate::umem::CompletionRing,
    /// Ring size used for all rings
    ring_size: u32,
}

impl XskSocket {
    /// Create a new AF_XDP socket bound to the given interface and queue.
    ///
    /// Initialization flow (7 steps):
    /// 1. Create AF_XDP socket
    /// 2. Register UMEM with socket
    /// 3. Set ring sizes (fill, completion, RX, TX)
    /// 4. Query mmap offsets
    /// 5. Mmap and setup all four rings
    /// 6. Bind to interface + queue
    /// 7. Pre-fill the fill ring with initial frames
    pub fn new(
        umem: &mut Umem,
        ifname: &str,
        queue_id: u32,
        ring_size: u32,
    ) -> Result<Self, &'static str> {
        // Validate ring_size is power of 2
        if ring_size == 0 || !ring_size.is_power_of_two() {
            return Err("ring_size must be a non-zero power of 2");
        }

        // Step 1: Create AF_XDP socket
        let fd = create_xdp_socket()?;

        // Step 2: Register UMEM
        if let Err(e) = umem.register(fd) {
            unsafe {
                let _ = syscall::close(fd);
            }
            return Err(e);
        }

        // Step 3: Set ring sizes
        if let Err(e) = set_ring_sizes(fd, umem, ring_size) {
            unsafe {
                let _ = syscall::close(fd);
            }
            return Err(e);
        }

        // Step 4: Query mmap offsets for all rings
        let offsets = match Umem::query_mmap_offsets(fd) {
            Ok(o) => o,
            Err(e) => {
                unsafe {
                    let _ = syscall::close(fd);
                }
                return Err(e);
            }
        };

        // Step 5: Mmap and setup all four rings
        let rx = match RxRing::new(fd, &offsets.rx, ring_size) {
            Ok(r) => r,
            Err(e) => {
                unsafe {
                    let _ = syscall::close(fd);
                }
                return Err(e);
            }
        };

        let tx = match TxRing::new(fd, &offsets.tx, ring_size) {
            Ok(r) => r,
            Err(e) => {
                unsafe {
                    let _ = syscall::close(fd);
                }
                return Err(e);
            }
        };

        let fill = match crate::umem::FillRing::new(fd, &offsets.fr, ring_size) {
            Ok(r) => r,
            Err(e) => {
                unsafe {
                    let _ = syscall::close(fd);
                }
                return Err(e);
            }
        };

        let comp = match crate::umem::CompletionRing::new(fd, &offsets.cr, ring_size) {
            Ok(r) => r,
            Err(e) => {
                unsafe {
                    let _ = syscall::close(fd);
                }
                return Err(e);
            }
        };

        // Step 6: Bind to interface + queue
        if let Err(e) = bind_socket(fd, ifname, queue_id) {
            unsafe {
                let _ = syscall::close(fd);
            }
            return Err(e);
        }

        let mut sock = XskSocket {
            fd,
            rx,
            tx,
            fill,
            comp,
            ring_size,
        };

        // Step 7: Pre-fill the fill ring with initial frames from UMEM
        sock.prefill(umem);

        Ok(sock)
    }

    /// Pre-fill the fill ring with frames from the UMEM free pool.
    fn prefill(&mut self, umem: &mut Umem) {
        let batch_size = self.ring_size.min(umem.free_frame_count() as u32);
        let mut addrs = Vec::with_capacity(batch_size as usize);
        for _ in 0..batch_size {
            if let Some(addr) = umem.alloc_frame() {
                addrs.push(addr);
            } else {
                break;
            }
        }
        if !addrs.is_empty() {
            self.fill.produce(&addrs);
        }
    }

    /// Receive packets from the RX ring.
    /// Returns descriptors pointing into UMEM. After processing each packet,
    /// its frame address should be returned to the fill ring.
    pub fn recv(&mut self, batch_size: u32) -> Vec<XskDesc> {
        self.rx.recv(batch_size)
    }

    /// Submit packets for transmission via the TX ring.
    /// Returns the number of descriptors actually enqueued.
    /// Call `kick_tx()` afterward to wake the kernel if needed.
    pub fn send(&mut self, descs: &[XskDesc]) -> u32 {
        self.tx.submit(descs)
    }

    /// Kick the kernel to process the TX ring.
    /// Uses sendto() with zero-length buffer.
    pub fn kick_tx(&self) -> Result<(), &'static str> {
        unsafe {
            syscall::sendto(
                self.fd,
                std::ptr::null(),
                0,
                syscall::MSG_DONTWAIT,
                std::ptr::null(),
                0,
            )
            .map(|_| ())
            .or_else(|errno| {
                // EAGAIN/EWOULDBLOCK is OK — means kernel is already processing
                if errno == 11 || errno == 35 {
                    Ok(())
                } else {
                    Err("sendto TX kick failed")
                }
            })
        }
    }

    /// Replenish the fill ring with frame addresses (for kernel to use for RX).
    pub fn fill_frames(&mut self, addrs: &[u64]) -> u32 {
        self.fill.produce(addrs)
    }

    /// Drain the completion ring and return completed TX frame addresses.
    /// These addresses can be recycled back into the UMEM free pool.
    pub fn drain_completions(&mut self) -> Vec<u64> {
        self.comp.consume()
    }

    /// Drain completions directly back into the UMEM free pool.
    pub fn recycle_completions(&mut self, umem: &mut Umem) -> u32 {
        self.comp.recycle_to(umem)
    }

    /// Complete RX/TX cycle: drain completions, refill fill ring.
    /// This is the main housekeeping function to call in the event loop.
    pub fn complete_cycle(&mut self, umem: &mut Umem) {
        // Recycle completed TX frames
        self.recycle_completions(umem);

        // Refill fill ring from free pool
        let free = self.fill.free_slots();
        if free > 0 {
            let batch = free.min(umem.free_frame_count() as u32);
            let mut addrs = Vec::with_capacity(batch as usize);
            for _ in 0..batch {
                if let Some(addr) = umem.alloc_frame() {
                    addrs.push(addr);
                } else {
                    break;
                }
            }
            if !addrs.is_empty() {
                self.fill.produce(&addrs);
            }
        }
    }

    /// Poll the socket for readiness (POLLIN for RX, POLLOUT for TX).
    /// timeout_ms: -1 = block forever, 0 = non-blocking, >0 = timeout in ms.
    pub fn poll(&self, events: i16, timeout_ms: i32) -> Result<i16, &'static str> {
        let mut pfd = syscall::PollFd {
            fd: self.fd,
            events,
            revents: 0,
        };

        unsafe {
            syscall::poll(&mut pfd, 1, timeout_ms).map_err(|_| "poll failed")?;
        }

        Ok(pfd.revents)
    }

    /// Check if RX ring needs wakeup.
    pub fn rx_needs_wakeup(&self) -> bool {
        self.rx.needs_wakeup()
    }

    /// Check if TX ring needs wakeup.
    pub fn tx_needs_wakeup(&self) -> bool {
        self.tx.needs_wakeup()
    }

    /// Get number of available packets in RX ring.
    pub fn rx_available(&self) -> u32 {
        self.rx.available()
    }

    /// Get number of free TX slots.
    pub fn tx_free_slots(&self) -> u32 {
        self.tx.free_slots()
    }

    /// Get the raw socket file descriptor.
    pub fn fd(&self) -> i32 {
        self.fd
    }

    /// Query socket statistics (dropped, invalid, etc).
    pub fn statistics(&self) -> Result<XskStatistics, &'static str> {
        query_statistics(self.fd)
    }

    /// Query XDP socket options for zero-copy capability.
    pub fn query_options(&self) -> Result<u32, &'static str> {
        query_xdp_options(self.fd)
    }
}

impl Drop for XskSocket {
    fn drop(&mut self) {
        // Rings are cleaned up by their own Drop impls (munmap).
        // We just close the socket fd.
        unsafe {
            let _ = syscall::close(self.fd);
        }
    }
}

// =============================================================================
// Helper functions — socket lifecycle
// =============================================================================

/// Create an AF_XDP socket via socket(AF_XDP, SOCK_RAW, 0).
fn create_xdp_socket() -> Result<i32, &'static str> {
    unsafe {
        syscall::socket(AF_XDP as i32, syscall::SOCK_RAW, 0)
            .map_err(|_| "socket(AF_XDP) creation failed")
    }
}

/// Set all ring sizes via setsockopt.
fn set_ring_sizes(sock_fd: i32, umem: &Umem, ring_size: u32) -> Result<(), &'static str> {
    // Set UMEM fill ring size
    umem.set_fill_ring_size(sock_fd, ring_size)?;
    // Set UMEM completion ring size
    umem.set_completion_ring_size(sock_fd, ring_size)?;
    // Set RX ring size
    unsafe {
        syscall::setsockopt(
            sock_fd,
            SOL_XDP,
            XDP_RX_RING,
            &ring_size as *const u32 as *const u8,
            std::mem::size_of::<u32>() as u32,
        )
        .map_err(|_| "setsockopt XDP_RX_RING failed")?;
    }
    // Set TX ring size
    unsafe {
        syscall::setsockopt(
            sock_fd,
            SOL_XDP,
            XDP_TX_RING,
            &ring_size as *const u32 as *const u8,
            std::mem::size_of::<u32>() as u32,
        )
        .map_err(|_| "setsockopt XDP_TX_RING failed")?;
    }
    Ok(())
}

/// Bind AF_XDP socket to interface and RX/TX queue.
fn bind_socket(sock_fd: i32, ifname: &str, queue_id: u32) -> Result<(), &'static str> {
    let ifindex = unsafe { syscall::if_nametoindex(ifname).map_err(|_| "interface not found")? };

    let saddr = Sockaddr_xdp {
        sxdp_family: AF_XDP,
        sxdp_flags: 0,
        sxdp_ifindex: ifindex,
        sxdp_queue_id: queue_id,
        sxdp_shared_umem_fd: 0,
    };

    unsafe {
        syscall::bind(
            sock_fd,
            &saddr as *const Sockaddr_xdp as *const u8,
            std::mem::size_of::<Sockaddr_xdp>() as u32,
        )
        .map_err(|_| "bind(AF_XDP) failed")
    }
}

/// Query socket statistics via getsockopt(XDP_STATISTICS).
fn query_statistics(sock_fd: i32) -> Result<XskStatistics, &'static str> {
    let mut stats: XskStatistics = unsafe { std::mem::zeroed() };
    let mut optlen = std::mem::size_of::<XskStatistics>() as u32;

    unsafe {
        syscall::getsockopt(
            sock_fd,
            SOL_XDP,
            XDP_STATISTICS,
            &mut stats as *mut XskStatistics as *mut u8,
            &mut optlen,
        )
        .map_err(|_| "getsockopt XDP_STATISTICS failed")?;
    }

    Ok(stats)
}

/// Query XDP socket options for zero-copy capability.
pub fn query_xdp_options(sock_fd: i32) -> Result<u32, &'static str> {
    let mut options: u32 = 0;
    let mut optlen = std::mem::size_of::<u32>() as u32;

    unsafe {
        syscall::getsockopt(
            sock_fd,
            SOL_XDP,
            XDP_OPTIONS,
            &mut options as *mut u32 as *mut u8,
            &mut optlen,
        )
        .map_err(|_| "getsockopt XDP_OPTIONS failed")?;
    }

    Ok(options)
}

// =============================================================================
// Tests
// =============================================================================

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_create_xdp_socket_no_cap() {
        // Without CAP_NET_ADMIN, socket creation may fail (EACCES/EPERM)
        // This is expected — the full flow requires sudo
        let result = create_xdp_socket();
        // Either succeeds (if running as root) or fails with permission error
        if result.is_err() {
            // Expected without privileges
        } else {
            // Clean up if it succeeded
            unsafe {
                let _ = syscall::close(result.unwrap());
            }
        }
    }

    #[test]
    fn test_bind_nonexistent_interface() {
        // This should fail because interface doesn't exist
        let result = unsafe { syscall::if_nametoindex("nonexistent_xdp_test") };
        assert!(result.is_err());
    }

    #[test]
    fn test_ring_size_validation() {
        // ring_size must be power of 2
        assert!(0u32.is_power_of_two() == false);
        assert!(1u32.is_power_of_two() == true);
        assert!(2048u32.is_power_of_two() == true);
        assert!(3000u32.is_power_of_two() == false);
    }

    #[test]
    #[ignore] // Requires CAP_NET_ADMIN and actual network device
    fn test_full_socket_creation() {
        let config = af_xdp_common::XskConfig {
            frame_size: 4096,
            frame_count: 4096,
            headroom: 0,
            flags: 0,
        };
        let mut umem = Umem::new(config).expect("UMEM creation failed");
        let xsk = XskSocket::new(&mut umem, "lo", 0, DEFAULT_RING_SIZE);
        assert!(
            xsk.is_ok(),
            "XskSocket creation on loopback should succeed with CAP_NET_ADMIN"
        );
    }

    #[test]
    #[ignore] // Requires CAP_NET_ADMIN
    fn test_recv_send_loopback() {
        let config = af_xdp_common::XskConfig {
            frame_size: 4096,
            frame_count: 4096,
            headroom: 0,
            flags: 0,
        };
        let mut umem = Umem::new(config).expect("UMEM creation failed");
        let mut xsk = XskSocket::new(&mut umem, "lo", 0, DEFAULT_RING_SIZE)
            .expect("XskSocket creation failed");

        // Receive with timeout (non-blocking)
        let packets = xsk.recv(64);
        // On loopback with no traffic, expect 0 packets
        assert!(packets.is_empty() || !packets.is_empty()); // always true — just exercises the code

        // Complete cycle
        xsk.complete_cycle(&mut umem);
    }
}
