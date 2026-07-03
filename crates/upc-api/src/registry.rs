// SPDX-License-Identifier: GPL-3.0-or-later
//! Host-side surface registry (ADR-080 §4: the registered-dispatch model,
//! realized on the HOST rather than in the verifier-bound interpreter).
//!
//! ADR-080 §4 describes a future "registered dispatch" where a workload declares
//! its syscall surface as a table the system consults, so new guests slot in
//! without editing a central `if/else`. That table CANNOT live in the eBPF
//! interpreter — the verifier forbids the dynamic dispatch it would need, and a
//! runtime-registered surface would forfeit the `cfg`-partitioning that keeps
//! each build under the 1M budget (ADR-080 §3; Scientist, 2026-07-03 panel).
//!
//! So the registry lives HERE, host-side: a catalog of the syscall surfaces the
//! loaders know how to load. A shared loader consults it to select and validate
//! a guest's surface before populating the maps. It is a lookup table over
//! surface descriptors, never a runtime dispatcher.

use crate::syscall::SyscallSurface;

/// A host-side catalog of known guest syscall surfaces (ADR-080 §4). The shared
/// loader looks a surface up by name to validate a [`super::workload::Workload`]
/// before load. Holds surfaces behind `&dyn` — they only DESCRIBE, never
/// dispatch.
pub struct SurfaceRegistry<'a> {
    // Backing store (a Vec / map of registered surfaces) is filled by the host;
    // no fields in the scaffold.
    _surfaces: core::marker::PhantomData<&'a dyn SyscallSurface>,
}

impl<'a> SurfaceRegistry<'a> {
    /// An empty registry. The host registers the surfaces its loaders support
    /// (today: "doom", "xv6").
    pub fn new() -> Self {
        todo!("construct an empty registry; host registers known surfaces")
    }

    /// Register a guest's syscall surface under its name.
    pub fn register(&mut self, _surface: &'a dyn SyscallSurface) {
        todo!("insert `surface` keyed by surface.name()")
    }

    /// Look up a registered surface by name, for the shared loader's
    /// pre-load validation.
    pub fn get(&self, _name: &str) -> Option<&'a dyn SyscallSurface> {
        todo!("return the surface registered under `name`, if any")
    }
}

impl<'a> Default for SurfaceRegistry<'a> {
    fn default() -> Self {
        Self::new()
    }
}
