// SPDX-License-Identifier: GPL-2.0-only
// UMEM (User Memory) allocator for AF_XDP zero-copy packet pool
//
// Sacred Law: NO external deps. Raw syscalls only via crate::syscall.

use af_xdp_common::{
    CompletionDesc, FillDesc, XskConfig, XskRingOffsets, XskUmemReg,
    SOL_XDP, XDP_UMEM_COMPLETION_RING, XDP_UMEM_FILL_RING, XDP_UMEM_REG,
    XDP_PGOFF_FILL_RING, XDP_PGOFF_COMPLETION_RING,
    XDP_MMAP_OFFSETS, XskMmapOffsets,
};
use crate::syscall;
use std::ptr::NonNull;

// =============================================================================
// UMEM — shared memory region for zero-copy AF_XDP packet buffers
// =============================================================================

/// UMEM — shared memory region for zero-copy AF_XDP packet buffers.
/// Manages allocation, deallocation, and ring buffer access for the
/// fill and completion rings that feed the kernel's AF_XDP path.
pub struct Umem {
    /// Mmap'd region base address
    addr: NonNull<u8>,
    /// Total allocated size in bytes
    size: u64,
    /// Configuration (frame size, count, headroom, flags)
    config: XskConfig,
    /// Free frame stack (LIFO allocator — offsets relative to UMEM base)
    free_frames: Vec<u64>,
}

impl Umem {
    /// Create new UMEM with given configuration.
    /// Allocates a contiguous memory region via mmap and initializes
    /// the free frame pool.
    pub fn new(config: XskConfig) -> Result<Self, &'static str> {
        validate_config(&config)?;

        let total_size = (config.frame_size as u64) * (config.frame_count as u64);

        // Allocate via anonymous mmap (MAP_SHARED for kernel access)
        let ptr = unsafe {
            syscall::mmap(
                std::ptr::null_mut(),
                total_size,
                syscall::PROT_READ | syscall::PROT_WRITE,
                syscall::MAP_SHARED | syscall::MAP_ANONYMOUS | syscall::MAP_POPULATE,
                -1,
                0,
            )
            .map_err(|_| "mmap allocation failed")?
        };

        let addr = NonNull::new(ptr).ok_or("mmap returned NULL")?;

        // Initialize free frame stack (all frames initially free)
        let mut free_frames = Vec::with_capacity(config.frame_count as usize);
        for i in 0..config.frame_count {
            free_frames.push((i as u64) * (config.frame_size as u64));
        }

        Ok(Umem {
            addr,
            size: total_size,
            config,
            free_frames,
        })
    }

    /// Allocate a frame from UMEM. Returns the frame offset relative
    /// to the UMEM base address.
    pub fn alloc_frame(&mut self) -> Option<u64> {
        self.free_frames.pop()
    }

    /// Free a frame back to pool. `offset` is relative to UMEM base.
    pub fn free_frame(&mut self, offset: u64) {
        if offset < self.size {
            self.free_frames.push(offset);
        }
    }

    /// Get base address of UMEM region.
    pub fn base_addr(&self) -> *mut u8 {
        self.addr.as_ptr()
    }

    /// Get total size of UMEM region in bytes.
    pub fn total_size(&self) -> u64 {
        self.size
    }

    /// Get configuration.
    pub fn config(&self) -> &XskConfig {
        &self.config
    }

    /// Get number of free frames available.
    pub fn free_frame_count(&self) -> usize {
        self.free_frames.len()
    }

    /// Get a raw pointer to a frame at the given offset.
    pub fn frame_ptr(&self, offset: u64) -> *mut u8 {
        unsafe { self.addr.as_ptr().add(offset as usize) }
    }

    /// Register this UMEM with an AF_XDP socket via setsockopt(XDP_UMEM_REG).
    pub fn register(&self, sock_fd: i32) -> Result<(), &'static str> {
        let reg = XskUmemReg {
            addr: self.addr.as_ptr() as u64,
            len: self.size,
            chunk_size: self.config.frame_size,
            headroom: self.config.headroom,
            flags: self.config.flags,
        };

        unsafe {
            syscall::setsockopt(
                sock_fd,
                SOL_XDP,
                XDP_UMEM_REG,
                &reg as *const XskUmemReg as *const u8,
                std::mem::size_of::<XskUmemReg>() as u32,
            )
            .map_err(|_| "setsockopt XDP_UMEM_REG failed")
        }
    }

    /// Set the fill ring size via setsockopt(XDP_UMEM_FILL_RING).
    pub fn set_fill_ring_size(&self, sock_fd: i32, size: u32) -> Result<(), &'static str> {
        unsafe {
            syscall::setsockopt(
                sock_fd,
                SOL_XDP,
                XDP_UMEM_FILL_RING,
                &size as *const u32 as *const u8,
                std::mem::size_of::<u32>() as u32,
            )
            .map_err(|_| "setsockopt XDP_UMEM_FILL_RING failed")
        }
    }

    /// Set the completion ring size via setsockopt(XDP_UMEM_COMPLETION_RING).
    pub fn set_completion_ring_size(&self, sock_fd: i32, size: u32) -> Result<(), &'static str> {
        unsafe {
            syscall::setsockopt(
                sock_fd,
                SOL_XDP,
                XDP_UMEM_COMPLETION_RING,
                &size as *const u32 as *const u8,
                std::mem::size_of::<u32>() as u32,
            )
            .map_err(|_| "setsockopt XDP_UMEM_COMPLETION_RING failed")
        }
    }

    /// Query mmap offsets for all rings via getsockopt(XDP_MMAP_OFFSETS).
    pub fn query_mmap_offsets(sock_fd: i32) -> Result<XskMmapOffsets, &'static str> {
        let mut offsets: XskMmapOffsets = unsafe { std::mem::zeroed() };
        let mut optlen = std::mem::size_of::<XskMmapOffsets>() as u32;

        unsafe {
            syscall::getsockopt(
                sock_fd,
                SOL_XDP,
                XDP_MMAP_OFFSETS,
                &mut offsets as *mut XskMmapOffsets as *mut u8,
                &mut optlen,
            )
            .map_err(|_| "getsockopt XDP_MMAP_OFFSETS failed")?;
        }

        Ok(offsets)
    }
}

impl Drop for Umem {
    fn drop(&mut self) {
        unsafe {
            let _ = syscall::munmap(self.addr.as_ptr(), self.size);
        }
    }
}

// =============================================================================
// Fill Ring — userspace produces frame addresses for kernel to fill with packets
// =============================================================================

/// Fill ring — userspace producer, kernel consumer.
/// Userspace pushes empty frame addresses; kernel fills them with received packets.
pub struct FillRing {
    /// Producer index (userspace increments after pushing)
    producer: *mut u32,
    /// Consumer index (kernel increments after consuming)
    consumer: *const u32,
    /// Descriptor array of frame addresses
    descs: *mut u64,
    /// Ring size (number of descriptors, must be power of 2)
    size: u32,
    /// Mask for fast modulo (size - 1)
    mask: u32,
    /// Mmap'd memory base (for cleanup)
    map_addr: *mut u8,
    /// Mmap'd memory size
    map_size: u64,
}

impl FillRing {
    /// Create fill ring by mmap'ing the kernel-allocated region.
    pub fn new(
        sock_fd: i32,
        offsets: &XskRingOffsets,
        ring_size: u32,
    ) -> Result<Self, &'static str> {
        // Calculate mmap size: need to cover the largest offset + ring data
        let map_size = offsets.desc + (ring_size as u64) * std::mem::size_of::<u64>() as u64;

        let map_addr = unsafe {
            syscall::mmap(
                std::ptr::null_mut(),
                map_size,
                syscall::PROT_READ | syscall::PROT_WRITE,
                syscall::MAP_SHARED | syscall::MAP_POPULATE,
                sock_fd,
                XDP_PGOFF_FILL_RING as i64,
            )
            .map_err(|_| "mmap fill ring failed")?
        };

        unsafe {
            Ok(FillRing {
                producer: map_addr.add(offsets.producer as usize) as *mut u32,
                consumer: map_addr.add(offsets.consumer as usize) as *const u32,
                descs: map_addr.add(offsets.desc as usize) as *mut u64,
                size: ring_size,
                mask: ring_size - 1,
                map_addr,
                map_size,
            })
        }
    }

    /// Push a batch of frame addresses into the fill ring.
    /// Returns the number of addresses actually pushed (may be less if ring is full).
    pub fn produce(&mut self, addrs: &[u64]) -> u32 {
        unsafe {
            syscall::smp_rmb();
            let producer = *self.producer;
            let consumer = *self.consumer;
            let free_slots = self.size - (producer - consumer);
            let count = (addrs.len() as u32).min(free_slots);

            for i in 0..count {
                let idx = ((producer + i) & self.mask) as usize;
                *self.descs.add(idx) = addrs[i as usize];
            }

            syscall::smp_wmb();
            *self.producer = producer + count;
            count
        }
    }

    /// Get number of free slots in the ring.
    pub fn free_slots(&self) -> u32 {
        unsafe {
            let producer = *self.producer;
            let consumer = *self.consumer;
            self.size - (producer - consumer)
        }
    }
}

impl Drop for FillRing {
    fn drop(&mut self) {
        if !self.map_addr.is_null() {
            unsafe {
                let _ = syscall::munmap(self.map_addr, self.map_size);
            }
        }
    }
}

// =============================================================================
// Completion Ring — kernel produces completed TX frame addresses
// =============================================================================

/// Completion ring — kernel producer, userspace consumer.
/// Kernel pushes addresses of frames whose TX has completed; userspace
/// recycles them back into the free pool.
pub struct CompletionRing {
    /// Consumer index (userspace increments after consuming)
    consumer: *mut u32,
    /// Producer index (kernel increments after producing)
    producer: *const u32,
    /// Descriptor array of completed frame addresses
    descs: *const u64,
    /// Ring size
    size: u32,
    /// Mask for fast modulo
    mask: u32,
    /// Mmap'd memory base
    map_addr: *mut u8,
    /// Mmap'd memory size
    map_size: u64,
}

impl CompletionRing {
    /// Create completion ring by mmap'ing the kernel-allocated region.
    pub fn new(
        sock_fd: i32,
        offsets: &XskRingOffsets,
        ring_size: u32,
    ) -> Result<Self, &'static str> {
        let map_size = offsets.desc + (ring_size as u64) * std::mem::size_of::<u64>() as u64;

        let map_addr = unsafe {
            syscall::mmap(
                std::ptr::null_mut(),
                map_size,
                syscall::PROT_READ | syscall::PROT_WRITE,
                syscall::MAP_SHARED | syscall::MAP_POPULATE,
                sock_fd,
                XDP_PGOFF_COMPLETION_RING as i64,
            )
            .map_err(|_| "mmap completion ring failed")?
        };

        unsafe {
            Ok(CompletionRing {
                consumer: map_addr.add(offsets.consumer as usize) as *mut u32,
                producer: map_addr.add(offsets.producer as usize) as *const u32,
                descs: map_addr.add(offsets.desc as usize) as *const u64,
                size: ring_size,
                mask: ring_size - 1,
                map_addr,
                map_size,
            })
        }
    }

    /// Drain completed frame addresses from the ring.
    /// Returns a Vec of frame offsets that can be recycled back to the UMEM free pool.
    pub fn consume(&mut self) -> Vec<u64> {
        unsafe {
            syscall::smp_rmb();
            let producer = *self.producer;
            let consumer = *self.consumer;
            let available = producer - consumer;

            let mut addrs = Vec::with_capacity(available as usize);
            for i in 0..available {
                let idx = ((consumer + i) & self.mask) as usize;
                addrs.push(*self.descs.add(idx));
            }

            syscall::smp_wmb();
            *self.consumer = producer;
            addrs
        }
    }

    /// Drain completed frames directly back into the UMEM free pool.
    pub fn recycle_to(&mut self, umem: &mut Umem) -> u32 {
        let addrs = self.consume();
        let count = addrs.len() as u32;
        for addr in addrs {
            umem.free_frame(addr);
        }
        count
    }
}

impl Drop for CompletionRing {
    fn drop(&mut self) {
        if !self.map_addr.is_null() {
            unsafe {
                let _ = syscall::munmap(self.map_addr, self.map_size);
            }
        }
    }
}

// =============================================================================
// Configuration validation
// =============================================================================

/// Validate AF_XDP UMEM configuration parameters.
pub fn validate_config(config: &XskConfig) -> Result<(), &'static str> {
    if config.frame_size == 0 || !config.frame_size.is_power_of_two() {
        return Err("frame_size must be a non-zero power of 2");
    }
    if config.frame_size < af_xdp_common::MIN_FRAME_SIZE {
        return Err("frame_size must be >= 2048");
    }
    if config.frame_count == 0 {
        return Err("frame_count must be > 0");
    }
    if config.frame_count > 1_000_000 {
        return Err("frame_count too large (max 1M)");
    }
    if config.headroom >= config.frame_size {
        return Err("headroom must be less than frame_size");
    }
    Ok(())
}

// =============================================================================
// Tests
// =============================================================================

#[cfg(test)]
mod tests {
    use super::*;

    fn test_config(frame_count: u32) -> XskConfig {
        XskConfig {
            frame_size: 4096,
            frame_count,
            headroom: 0,
            flags: 0,
        }
    }

    #[test]
    fn test_umem_creation() {
        let umem = Umem::new(test_config(16));
        assert!(umem.is_ok(), "UMEM creation should succeed");
        let umem = umem.unwrap();
        assert_eq!(umem.total_size(), 4096 * 16);
        assert_eq!(umem.free_frame_count(), 16);
    }

    #[test]
    fn test_umem_frame_allocation() {
        let mut umem = Umem::new(test_config(10)).unwrap();
        assert_eq!(umem.free_frame_count(), 10);

        // Allocate all frames
        for i in 0..10 {
            let frame = umem.alloc_frame();
            assert!(frame.is_some(), "Frame {} allocation should succeed", i);
        }
        assert_eq!(umem.free_frame_count(), 0);

        // OOM on next allocation
        assert!(umem.alloc_frame().is_none(), "Should be OOM");
    }

    #[test]
    fn test_umem_frame_free_reuse() {
        let mut umem = Umem::new(test_config(2)).unwrap();

        let f1 = umem.alloc_frame().unwrap();
        let _f2 = umem.alloc_frame().unwrap();
        assert!(umem.alloc_frame().is_none());

        // Free f1 and reuse
        umem.free_frame(f1);
        assert_eq!(umem.free_frame_count(), 1);
        let f3 = umem.alloc_frame().unwrap();
        assert_eq!(f3, f1, "Freed frame should be reused (LIFO)");
    }

    #[test]
    fn test_umem_frame_ptr() {
        let umem = Umem::new(test_config(4)).unwrap();
        let base = umem.base_addr() as u64;

        // Frame pointers should be within UMEM region
        let ptr0 = umem.frame_ptr(0) as u64;
        assert_eq!(ptr0, base);

        let ptr1 = umem.frame_ptr(4096) as u64;
        assert_eq!(ptr1, base + 4096);
    }

    #[test]
    fn test_umem_write_read() {
        let mut umem = Umem::new(test_config(2)).unwrap();
        let offset = umem.alloc_frame().unwrap();
        let ptr = umem.frame_ptr(offset);

        // Write a test pattern
        unsafe {
            let slice = std::slice::from_raw_parts_mut(ptr, 64);
            for (i, b) in slice.iter_mut().enumerate() {
                *b = i as u8;
            }
            // Read it back
            for i in 0..64 {
                assert_eq!(*ptr.add(i), i as u8);
            }
        }
    }

    #[test]
    fn test_invalid_config_zero_frame_size() {
        let config = XskConfig { frame_size: 0, frame_count: 10, headroom: 0, flags: 0 };
        assert!(Umem::new(config).is_err());
    }

    #[test]
    fn test_invalid_config_non_power_of_2() {
        let config = XskConfig { frame_size: 3000, frame_count: 10, headroom: 0, flags: 0 };
        assert!(Umem::new(config).is_err());
    }

    #[test]
    fn test_invalid_config_too_small() {
        let config = XskConfig { frame_size: 1024, frame_count: 10, headroom: 0, flags: 0 };
        assert!(Umem::new(config).is_err());
    }

    #[test]
    fn test_invalid_config_zero_count() {
        let config = XskConfig { frame_size: 4096, frame_count: 0, headroom: 0, flags: 0 };
        assert!(Umem::new(config).is_err());
    }

    #[test]
    fn test_invalid_config_headroom_too_large() {
        let config = XskConfig { frame_size: 4096, frame_count: 10, headroom: 4096, flags: 0 };
        assert!(Umem::new(config).is_err());
    }

    #[test]
    fn test_validate_config_valid() {
        let config = test_config(512);
        assert!(validate_config(&config).is_ok());
    }

    #[test]
    fn test_free_out_of_range_ignored() {
        let mut umem = Umem::new(test_config(2)).unwrap();
        let initial_free = umem.free_frame_count();
        umem.free_frame(u64::MAX); // out of range, should be ignored
        assert_eq!(umem.free_frame_count(), initial_free);
    }

    #[test]
    fn test_umem_drop_cleanup() {
        // Create and drop — should not leak or crash
        let umem = Umem::new(test_config(4)).unwrap();
        let _base = umem.base_addr();
        drop(umem);
    }
}
