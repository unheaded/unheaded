// SPDX-License-Identifier: GPL-3.0-or-later
//! The [`Workload`] — the four contract parts bundled into one loadable guest.
//!
//! This is the type a loader (`cmd/upc-bootctl`, `crates/doom-runner`) would
//! construct and hand to one shared loader instead of hardcoding each guest's
//! parts by hand (ADR-080 §"intended payoff"). "Add a guest" then becomes
//! "declare a `Workload`," not "write a third bespoke loader."
//!
//! It is a plain DESCRIPTOR: it names what to load, not how the maps are
//! populated (that is the loader's job). The image + surface are held behind
//! `&dyn` so heterogeneous guests share one loader path; the memory model and
//! boot protocol are small copyable enums; `rv2mbc` and boot are treated as
//! strategy-specific and may be trivial/absent for a given guest (Scientist,
//! 2026-07-03 panel).

use crate::boot::BootProtocol;
use crate::image::GuestImage;
use crate::memory::MemoryModel;
use crate::syscall::SyscallSurface;

/// A loadable UPC guest: the five-part contract (ADR-080 §2) as one descriptor.
/// The shared loader reads these to populate `ROM_MAP` / `RV2MBC_MAP` / `RAM_MAP`,
/// set the entry PC and memory model, run the boot protocol, and validate the
/// declared syscall surface against the loaded eBPF object (allowlist).
pub struct Workload<'a> {
    /// A stable name for logging / selection (e.g. "doom", "xv6").
    pub name: &'a str,
    /// Parts 1+2: the MBC image, its branch map, and entry PC.
    pub image: &'a dyn GuestImage,
    /// Part 3: the memory model the loader configures before dispatch.
    pub memory: MemoryModel,
    /// Part 4: the host-side descriptor of the guest's syscall surface (an
    /// allowlist the loader checks against the loaded object's feature set).
    pub surface: &'a dyn SyscallSurface,
    /// Part 5: how control is handed to the guest. `Direct` for the common case.
    pub boot: BootProtocol,
}

impl<'a> Workload<'a> {
    /// Validate the descriptor's internal consistency before load: the declared
    /// [`surface`](Self::surface) allowlist is non-empty and matches the loaded
    /// object's feature gate, the image entry is in range, etc. (BlackMage,
    /// 2026-07-03 panel — the Workload is the single validation choke point).
    /// The actual map-population + rv2mbc-SHA + slice-isolation checks are the
    /// shared loader's; this names the descriptor-level precondition.
    pub fn validate(&self) -> Result<(), &'static str> {
        todo!("check surface allowlist non-empty + feature_gate consistent + entry in range")
    }
}
