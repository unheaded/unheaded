// SPDX-License-Identifier: GPL-2.0-only
// Unheaded Kingdom — AF_XDP zero-copy packet I/O common types
// Shared between kernel eBPF programs and userspace AF_XDP socket loader
//
// Kernel >= 5.15 required. These types define the C ABI boundary
// between eBPF programs (kernel) and userspace AF_XDP socket handler.

#![cfg_attr(not(test), no_std)]
#![allow(non_camel_case_types)]

// =============================================================================
// Compile-time assertion helper
// =============================================================================

/// Compile-time assertion: panics at compile time if `a != b`.
macro_rules! const_assert_size {
    ($t:ty, $sz:expr) => {
        const _: () = {
            if core::mem::size_of::<$t>() != $sz {
                panic!(concat!(
                    "size_of::<",
                    stringify!($t),
                    ">() != ",
                    stringify!($sz)
                ));
            }
        };
    };
}

macro_rules! const_assert_align {
    ($t:ty, $align:expr) => {
        const _: () = {
            if core::mem::align_of::<$t>() != $align {
                panic!(concat!(
                    "align_of::<",
                    stringify!($t),
                    ">() != ",
                    stringify!($align)
                ));
            }
        };
    };
}

// =============================================================================
// UMEM Descriptor — points to a frame in the shared memory region
// =============================================================================

/// UMEM descriptor — points to a frame in the shared memory region.
/// Used in fill ring (kernel consumer, userspace producer)
/// and completion ring (kernel producer, userspace consumer).
#[repr(C)]
#[derive(Copy, Clone, Debug, Default, PartialEq, Eq)]
pub struct XskDesc {
    /// Frame address in UMEM (byte offset from UMEM base)
    pub addr: u64,
    /// Frame data length in bytes
    pub len: u32,
    /// Frame options/flags
    pub options: u32,
}

const_assert_size!(XskDesc, 16);
const_assert_align!(XskDesc, 8);

// =============================================================================
// Ring Offsets — mmap region layout (from XDP_MMAP_OFFSETS getsockopt)
// =============================================================================

/// Ring offsets for mmap'd regions (returned by XDP_MMAP_OFFSETS getsockopt).
/// Describes the layout of producer, consumer, descriptor, and flags
/// within the mmap'd ring buffer memory.
#[repr(C)]
#[derive(Copy, Clone, Debug, Default, PartialEq, Eq)]
pub struct XskRingOffsets {
    /// Byte offset to producer index within mmap'd region
    pub producer: u64,
    /// Byte offset to consumer index within mmap'd region
    pub consumer: u64,
    /// Byte offset to descriptor array within mmap'd region
    pub desc: u64,
    /// Byte offset to flags word within mmap'd region
    pub flags: u64,
}

const_assert_size!(XskRingOffsets, 32);
const_assert_align!(XskRingOffsets, 8);

// =============================================================================
// XSK Configuration — UMEM parameters for setsockopt(XDP_UMEM_REG)
// =============================================================================

/// AF_XDP UMEM configuration passed to setsockopt(XDP_UMEM_REG).
#[repr(C)]
#[derive(Copy, Clone, Debug, Default, PartialEq, Eq)]
pub struct XskUmemReg {
    /// Base address of the UMEM region (mmap'd memory)
    pub addr: u64,
    /// Total size of the UMEM region in bytes
    pub len: u64,
    /// Size of each frame (power of 2, >= 2048)
    pub chunk_size: u32,
    /// Headroom in each frame (packet data offset)
    pub headroom: u32,
    /// Configuration flags (XDP_UMEM_UNALIGNED_CHUNK_FLAG, etc)
    pub flags: u32,
}

const_assert_size!(XskUmemReg, 32);

/// User-facing configuration for UMEM creation (before registration).
#[repr(C)]
#[derive(Copy, Clone, Debug, Default, PartialEq, Eq)]
pub struct XskConfig {
    /// Size of each frame (power of 2, >= 2048)
    pub frame_size: u32,
    /// Total number of frames
    pub frame_count: u32,
    /// Headroom in each frame (packet offset)
    pub headroom: u32,
    /// Configuration flags (XDP_SHARED_UMEM, etc)
    pub flags: u32,
}

const_assert_size!(XskConfig, 16);
const_assert_align!(XskConfig, 4);

// =============================================================================
// RX/TX Ring Descriptors
// =============================================================================

/// Completion descriptor (kernel -> userspace, returned frames after TX).
#[repr(C)]
#[derive(Copy, Clone, Debug, Default, PartialEq, Eq)]
pub struct CompletionDesc {
    /// Frame address that was transmitted (UMEM offset)
    pub addr: u64,
}

const_assert_size!(CompletionDesc, 8);

/// Fill descriptor (userspace -> kernel, frames available for RX).
/// Just a u64 address pointing into UMEM.
#[repr(C)]
#[derive(Copy, Clone, Debug, Default, PartialEq, Eq)]
pub struct FillDesc {
    /// Frame address available for kernel to fill (UMEM offset)
    pub addr: u64,
}

const_assert_size!(FillDesc, 8);

// =============================================================================
// Socket Address — for bind()
// =============================================================================

/// Socket address for AF_XDP bind().
/// Passed to bind(fd, &sxdp, sizeof(sxdp)).
#[repr(C)]
#[derive(Copy, Clone, Debug, Default, PartialEq, Eq)]
pub struct Sockaddr_xdp {
    /// Address family: AF_XDP = 44
    pub sxdp_family: u16,
    /// XDP bind flags (XDP_SHARED_UMEM, XDP_COPY, XDP_ZEROCOPY)
    pub sxdp_flags: u16,
    /// Interface index (from if_nametoindex)
    pub sxdp_ifindex: u32,
    /// RX/TX queue ID on the NIC
    pub sxdp_queue_id: u32,
    /// Shared UMEM file descriptor (if XDP_SHARED_UMEM flag set)
    pub sxdp_shared_umem_fd: u32,
}

const_assert_size!(Sockaddr_xdp, 16);

// =============================================================================
// Socket Statistics — from getsockopt(XDP_STATISTICS)
// =============================================================================

/// AF_XDP socket statistics (from getsockopt XDP_STATISTICS).
#[repr(C)]
#[derive(Copy, Clone, Debug, Default, PartialEq, Eq)]
pub struct XskStatistics {
    /// Frames dropped (RX ring full when kernel tried to deliver)
    pub rx_dropped: u64,
    /// Invalid RX descriptors encountered
    pub rx_invalid_descs: u64,
    /// Invalid TX descriptors encountered
    pub tx_invalid_descs: u64,
    /// RX ring was full (overflow count)
    pub rx_ring_full: u64,
    /// Fill ring had no descriptors when kernel needed one
    pub rx_fill_ring_empty_descs: u64,
    /// TX ring had no descriptors when sendto was called
    pub tx_ring_empty_descs: u64,
}

const_assert_size!(XskStatistics, 48);
const_assert_align!(XskStatistics, 8);

// =============================================================================
// Mmap Offsets — full structure returned by getsockopt(XDP_MMAP_OFFSETS)
// =============================================================================

/// Complete mmap offsets structure returned by getsockopt(XDP_MMAP_OFFSETS).
/// Contains offsets for all four rings: RX, TX, Fill, and Completion.
#[repr(C)]
#[derive(Copy, Clone, Debug, Default, PartialEq, Eq)]
pub struct XskMmapOffsets {
    /// RX ring offsets
    pub rx: XskRingOffsets,
    /// TX ring offsets
    pub tx: XskRingOffsets,
    /// Fill ring offsets
    pub fr: XskRingOffsets,
    /// Completion ring offsets
    pub cr: XskRingOffsets,
}

const_assert_size!(XskMmapOffsets, 128);

// =============================================================================
// AF_XDP Address Family
// =============================================================================

/// AF_XDP address family number (Linux kernel constant)
pub const AF_XDP: u16 = 44;

/// SOL_XDP socket option level
pub const SOL_XDP: i32 = 283;

// =============================================================================
// AF_XDP Socket Option Constants — for setsockopt(fd, SOL_XDP, option, ...)
// These values match the kernel's linux/if_xdp.h definitions (kernel >= 5.15)
// =============================================================================

/// Get mmap offsets for all rings
pub const XDP_MMAP_OFFSETS: i32 = 1;
/// Set RX ring size
pub const XDP_RX_RING: i32 = 2;
/// Set TX ring size
pub const XDP_TX_RING: i32 = 3;
/// Register UMEM region
pub const XDP_UMEM_REG: i32 = 4;
/// Set UMEM fill ring size
pub const XDP_UMEM_FILL_RING: i32 = 5;
/// Set UMEM completion ring size
pub const XDP_UMEM_COMPLETION_RING: i32 = 6;
/// Get socket statistics
pub const XDP_STATISTICS: i32 = 7;
/// Get socket options/capabilities
pub const XDP_OPTIONS: i32 = 8;

// =============================================================================
// AF_XDP Bind Flags
// =============================================================================

/// Share UMEM between multiple sockets
pub const XDP_SHARED_UMEM: u16 = 1 << 0;
/// Force copy mode (no zero-copy)
pub const XDP_COPY: u16 = 1 << 1;
/// Request zero-copy mode
pub const XDP_ZEROCOPY: u16 = 1 << 2;
/// Enable need-wakeup flag for power efficiency
pub const XDP_USE_NEED_WAKEUP: u16 = 1 << 3;

// =============================================================================
// AF_XDP UMEM Flags
// =============================================================================

/// Allow unaligned chunk placement in UMEM
pub const XDP_UMEM_UNALIGNED_CHUNK_FLAG: u32 = 1 << 0;

// =============================================================================
// XDP Actions (for BPF program return values)
// =============================================================================

/// Pass packet up the network stack
pub const XDP_PASS: u32 = 2;
/// Drop packet
pub const XDP_DROP: u32 = 1;
/// Bounce packet back out the same interface
pub const XDP_TX: u32 = 3;
/// Redirect packet (to another interface, CPU, or AF_XDP socket)
pub const XDP_REDIRECT: u32 = 4;
/// Abort (error, increment error counter and drop)
pub const XDP_ABORTED: u32 = 0;

// =============================================================================
// Ring Need-Wakeup Flags
// =============================================================================

/// Ring needs wakeup via poll/sendto
pub const XDP_RING_NEED_WAKEUP: u32 = 1 << 0;

// =============================================================================
// Mmap Offsets for pgoff parameter
// =============================================================================

/// mmap pgoff for RX ring
pub const XDP_PGOFF_RX_RING: u64 = 0;
/// mmap pgoff for TX ring
pub const XDP_PGOFF_TX_RING: u64 = 0x80000000;
/// mmap pgoff for fill ring
pub const XDP_PGOFF_FILL_RING: u64 = 0x100000000;
/// mmap pgoff for completion ring
pub const XDP_PGOFF_COMPLETION_RING: u64 = 0x180000000;

// =============================================================================
// BPF Map Type for XSKMAP
// =============================================================================

/// BPF_MAP_TYPE_XSKMAP value
pub const BPF_MAP_TYPE_XSKMAP: u32 = 17;

// =============================================================================
// Default ring and frame sizes
// =============================================================================

/// Default number of descriptors per ring
pub const DEFAULT_RING_SIZE: u32 = 2048;
/// Default frame size (4KB)
pub const DEFAULT_FRAME_SIZE: u32 = 4096;
/// Default number of frames
pub const DEFAULT_FRAME_COUNT: u32 = 4096;
/// Default headroom (0 = no headroom)
pub const DEFAULT_HEADROOM: u32 = 0;
/// Minimum frame size (2KB)
pub const MIN_FRAME_SIZE: u32 = 2048;

// =============================================================================
// Tests
// =============================================================================

#[cfg(test)]
mod tests {
    use super::*;

    // ── Struct layout tests ─────────────────────────────────

    #[test]
    fn test_xsk_desc_layout() {
        assert_eq!(core::mem::size_of::<XskDesc>(), 16);
        assert_eq!(core::mem::align_of::<XskDesc>(), 8);
    }

    #[test]
    fn test_xsk_ring_offsets_layout() {
        assert_eq!(core::mem::size_of::<XskRingOffsets>(), 32);
        assert_eq!(core::mem::align_of::<XskRingOffsets>(), 8);
    }

    #[test]
    fn test_xsk_config_layout() {
        assert_eq!(core::mem::size_of::<XskConfig>(), 16);
        assert_eq!(core::mem::align_of::<XskConfig>(), 4);
    }

    #[test]
    fn test_xsk_umem_reg_layout() {
        assert_eq!(core::mem::size_of::<XskUmemReg>(), 32);
    }

    #[test]
    fn test_completion_desc_layout() {
        assert_eq!(core::mem::size_of::<CompletionDesc>(), 8);
    }

    #[test]
    fn test_fill_desc_layout() {
        assert_eq!(core::mem::size_of::<FillDesc>(), 8);
    }

    #[test]
    fn test_sockaddr_xdp_layout() {
        assert_eq!(core::mem::size_of::<Sockaddr_xdp>(), 16);
    }

    #[test]
    fn test_xsk_statistics_layout() {
        assert_eq!(core::mem::size_of::<XskStatistics>(), 48);
        assert_eq!(core::mem::align_of::<XskStatistics>(), 8);
    }

    #[test]
    fn test_xsk_mmap_offsets_layout() {
        assert_eq!(core::mem::size_of::<XskMmapOffsets>(), 128);
        assert_eq!(core::mem::align_of::<XskMmapOffsets>(), 8);
    }

    // ── XskDesc ─────────────────────────────────────────────

    #[test]
    fn test_xsk_desc_default() {
        let d = XskDesc::default();
        assert_eq!(d.addr, 0);
        assert_eq!(d.len, 0);
        assert_eq!(d.options, 0);
    }

    #[test]
    fn test_xsk_desc_copy() {
        let d = XskDesc { addr: 0x1000, len: 4096, options: 0 };
        let d2 = d;
        assert_eq!(d, d2);
    }

    // ── XskConfig ───────────────────────────────────────────

    #[test]
    fn test_xsk_config_default() {
        let c = XskConfig::default();
        assert_eq!(c.frame_size, 0);
        assert_eq!(c.frame_count, 0);
        assert_eq!(c.headroom, 0);
        assert_eq!(c.flags, 0);
    }

    // ── Sockaddr_xdp ────────────────────────────────────────

    #[test]
    fn test_sockaddr_xdp_af_xdp() {
        let sa = Sockaddr_xdp {
            sxdp_family: AF_XDP,
            sxdp_flags: 0,
            sxdp_ifindex: 1,
            sxdp_queue_id: 0,
            sxdp_shared_umem_fd: 0,
        };
        assert_eq!(sa.sxdp_family, 44);
        assert_eq!(sa.sxdp_ifindex, 1);
    }

    // ── Constants ───────────────────────────────────────────

    #[test]
    fn test_af_xdp_constant() {
        assert_eq!(AF_XDP, 44);
    }

    #[test]
    fn test_sol_xdp_constant() {
        assert_eq!(SOL_XDP, 283);
    }

    #[test]
    fn test_socket_option_constants() {
        assert_eq!(XDP_MMAP_OFFSETS, 1);
        assert_eq!(XDP_RX_RING, 2);
        assert_eq!(XDP_TX_RING, 3);
        assert_eq!(XDP_UMEM_REG, 4);
        assert_eq!(XDP_UMEM_FILL_RING, 5);
        assert_eq!(XDP_UMEM_COMPLETION_RING, 6);
        assert_eq!(XDP_STATISTICS, 7);
        assert_eq!(XDP_OPTIONS, 8);
    }

    #[test]
    fn test_bind_flags() {
        assert_eq!(XDP_SHARED_UMEM, 1);
        assert_eq!(XDP_COPY, 2);
        assert_eq!(XDP_ZEROCOPY, 4);
        assert_eq!(XDP_USE_NEED_WAKEUP, 8);
        // Flags should not overlap
        let all = XDP_SHARED_UMEM | XDP_COPY | XDP_ZEROCOPY | XDP_USE_NEED_WAKEUP;
        assert_eq!(all.count_ones(), 4);
    }

    #[test]
    fn test_xdp_actions() {
        assert_eq!(XDP_ABORTED, 0);
        assert_eq!(XDP_DROP, 1);
        assert_eq!(XDP_PASS, 2);
        assert_eq!(XDP_TX, 3);
        assert_eq!(XDP_REDIRECT, 4);
    }

    #[test]
    fn test_pgoff_constants() {
        assert_eq!(XDP_PGOFF_RX_RING, 0);
        assert_eq!(XDP_PGOFF_TX_RING, 0x80000000);
        assert_eq!(XDP_PGOFF_FILL_RING, 0x100000000);
        assert_eq!(XDP_PGOFF_COMPLETION_RING, 0x180000000);
    }

    #[test]
    fn test_bpf_map_type_xskmap() {
        assert_eq!(BPF_MAP_TYPE_XSKMAP, 17);
    }

    #[test]
    fn test_default_sizes() {
        assert_eq!(DEFAULT_RING_SIZE, 2048);
        assert_eq!(DEFAULT_FRAME_SIZE, 4096);
        assert!(DEFAULT_FRAME_SIZE.is_power_of_two());
        assert!(DEFAULT_RING_SIZE.is_power_of_two());
        assert!(DEFAULT_FRAME_SIZE >= MIN_FRAME_SIZE);
    }

    // ── XskStatistics ───────────────────────────────────────

    #[test]
    fn test_xsk_statistics_default() {
        let s = XskStatistics::default();
        assert_eq!(s.rx_dropped, 0);
        assert_eq!(s.rx_invalid_descs, 0);
        assert_eq!(s.tx_invalid_descs, 0);
        assert_eq!(s.rx_ring_full, 0);
        assert_eq!(s.rx_fill_ring_empty_descs, 0);
        assert_eq!(s.tx_ring_empty_descs, 0);
    }
}
