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
