// SPDX-License-Identifier: GPL-3.0-or-later
//! Contract part 3 (ADR-080 §2.3): the guest memory model.
//!
//! Every guest runs against a flat physical `RAM_MAP`. Kernel-class guests may
//! additionally request per-pid virtual-address slices backed by an Sv32-style
//! MMU (ADR-074). This module names only the host loader's *choice* of model —
//! the choice that governs how it lays out `RAM_MAP` and whether it seeds
//! per-pid page tables. The total size (e.g. 64 MiB) and page-table layout are
//! loader/core parameters and are NOT fixed here.

/// The memory model a workload runs under (ADR-080 §2.3). Selects how the host
/// loader configures guest memory before dispatch.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum MemoryModel {
    /// A single flat physical `RAM_MAP` shared by the whole guest. Doom uses
    /// this — the framebuffer is written straight to physical RAM. Total size
    /// is loader-configured, not fixed here.
    Flat,
    /// Per-pid virtual-address slices with an Sv32-style MMU (ADR-074): each
    /// process gets an isolated page table rooted at a per-pid pgd. xv6 /
    /// ASCEND-LINUX use this.
    PerPidMmu,
}

/// The per-pid physical slice geometry of a [`MemoryModel::PerPidMmu`] guest
/// (ADR-074; ADR-080 §"I/O-time gates", Gate B).
///
/// pid 0 keeps an identity mapping and is not a slice. Each child pid `p >= 1`
/// maps its user VA window `[0, slice_len)` onto the physical range
/// `[base + (p-1)*stride, base + (p-1)*stride + slice_len)`. The core admits a
/// child only while `p < max_pids` and `base + (p-1)*stride + stride <= ceiling`
/// (fork() returns -1 otherwise), so those are exactly the slices the gate
/// checks. The values are the loader's and the object's, never this crate's.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct SliceGeometry {
    /// Physical base of pid 1's slice (bytes).
    pub base: u32,
    /// Distance between consecutive pids' slices (bytes).
    pub stride: u32,
    /// Bytes of physical RAM each slice maps (the user VA window).
    pub slice_len: u32,
    /// Highest physical address a slice may reach under fork()'s capacity guard.
    pub ceiling: u32,
    /// Process-table size: pids `0..max_pids` exist, so children are `1..max_pids`.
    pub max_pids: u32,
}

impl SliceGeometry {
    /// The geometry from the five little-endian words the eBPF object carries
    /// in `UPC_MMU_GEOMETRY`: `[base, stride, slice_len, ceiling, max_pids]`.
    pub fn from_words(w: [u32; 5]) -> Self {
        SliceGeometry {
            base: w[0],
            stride: w[1],
            slice_len: w[2],
            ceiling: w[3],
            max_pids: w[4],
        }
    }

    /// The physical byte ranges `(pid, start, end)` of every child slice the
    /// core can map, in pid order. 64-bit arithmetic: no input wraps.
    pub fn admitted_slices(&self) -> Vec<(u32, u64, u64)> {
        let mut out = Vec::new();
        for pid in 1..self.max_pids {
            let start = self.base as u64 + (pid as u64 - 1) * self.stride as u64;
            if start + self.stride as u64 > self.ceiling as u64 {
                break;
            }
            out.push((pid, start, start + self.slice_len as u64));
        }
        out
    }

    /// The first child pid whose slice arithmetic overflows `u32`, as
    /// `(pid, start, end)`. The core computes `base + (pid-1)*stride (+ stride)`
    /// in wrapping `u32`, so such a pid would pass fork()'s ceiling check with a
    /// wrapped (low) address. `None` when every pid's arithmetic is exact.
    pub fn first_wrapping_slice(&self) -> Option<(u32, u64, u64)> {
        const U32_SPAN: u64 = 1 << 32;
        for pid in 1..self.max_pids {
            let start = self.base as u64 + (pid as u64 - 1) * self.stride as u64;
            let end = start + self.slice_len as u64;
            if start + self.stride as u64 > U32_SPAN || end > U32_SPAN {
                return Some((pid, start, end));
            }
        }
        None
    }
}

/// A physical RAM range no child slice may overlap: the kernel image, the
/// ramdisk, staged program data, the page-directory region, MMIO windows.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct MemRegion {
    /// What lives there, for the refusal message.
    pub name: &'static str,
    /// First byte.
    pub start: u32,
    /// Length in bytes.
    pub len: u32,
}

/// The MMU half of a per-pid-MMU [`Workload`](crate::Workload): the slice
/// geometry the loader expects the object to ship, and the regions it has
/// placed in physical RAM that the slices must stay clear of.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct MmuLayout {
    /// Child slice geometry; must equal the object's `UPC_MMU_GEOMETRY`.
    pub slices: SliceGeometry,
    /// Regions the loader places in RAM that no slice may overlap.
    pub reserved: Vec<MemRegion>,
}
