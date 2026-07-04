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
//! loaders know how to load. A shared loader consults it to select and validate a
//! guest's surface before populating the maps. It is a lookup table over surface
//! descriptors, never a runtime dispatcher.

use crate::syscall::SyscallSurface;

/// A host-side catalog of known guest syscall surfaces (ADR-080 §4). The shared
/// loader looks a surface up by name to confirm a [`Workload`]'s declared surface
/// is one the host actually supports before load. Holds surface descriptors —
/// they only DESCRIBE, never dispatch.
///
/// [`Workload`]: crate::workload::Workload
#[derive(Clone, Debug, Default)]
pub struct SurfaceRegistry {
    surfaces: Vec<SyscallSurface>,
}

impl SurfaceRegistry {
    /// An empty registry. The host registers the surfaces its loaders support
    /// (today: "doom", "xv6").
    pub fn new() -> Self {
        Self {
            surfaces: Vec::new(),
        }
    }

    /// Register a guest's syscall surface. A later registration under a name that
    /// already exists replaces the earlier one (last write wins), so a loader can
    /// override a default without duplicate entries.
    pub fn register(&mut self, surface: SyscallSurface) {
        if let Some(existing) = self.surfaces.iter_mut().find(|s| s.name == surface.name) {
            *existing = surface;
        } else {
            self.surfaces.push(surface);
        }
    }

    /// Look up a registered surface by name, for the shared loader's pre-load
    /// validation. `None` if no surface was registered under `name`.
    pub fn get(&self, name: &str) -> Option<&SyscallSurface> {
        self.surfaces.iter().find(|s| s.name == name)
    }

    /// The number of registered surfaces.
    pub fn len(&self) -> usize {
        self.surfaces.len()
    }

    /// Whether no surfaces are registered.
    pub fn is_empty(&self) -> bool {
        self.surfaces.is_empty()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::syscall::{FeatureGate, SyscallDesc};

    static DOOM_SYS: [SyscallDesc; 1] = [SyscallDesc {
        number: 0,
        name: "DRAW_FRAME",
    }];
    static XV6_SYS: [SyscallDesc; 1] = [SyscallDesc {
        number: 16,
        name: "write",
    }];

    fn doom() -> SyscallSurface {
        SyscallSurface {
            name: "doom",
            feature_gate: FeatureGate::Default,
            syscalls: &DOOM_SYS,
        }
    }

    fn xv6() -> SyscallSurface {
        SyscallSurface {
            name: "xv6",
            feature_gate: FeatureGate::AscendLinux,
            syscalls: &XV6_SYS,
        }
    }

    #[test]
    fn new_registry_is_empty() {
        let r = SurfaceRegistry::new();
        assert!(r.is_empty());
        assert_eq!(r.len(), 0);
        assert!(r.get("doom").is_none());
    }

    #[test]
    fn register_and_get() {
        let mut r = SurfaceRegistry::new();
        r.register(doom());
        r.register(xv6());
        assert_eq!(r.len(), 2);
        assert_eq!(r.get("doom").unwrap().feature_gate, FeatureGate::Default);
        assert_eq!(r.get("xv6").unwrap().feature_gate, FeatureGate::AscendLinux);
        assert!(r.get("linux").is_none());
    }

    #[test]
    fn re_register_replaces_not_duplicates() {
        let mut r = SurfaceRegistry::new();
        r.register(doom());
        // Same name, different feature gate — last write wins, no duplicate row.
        r.register(SyscallSurface {
            name: "doom",
            feature_gate: FeatureGate::AscendLinux,
            syscalls: &DOOM_SYS,
        });
        assert_eq!(r.len(), 1);
        assert_eq!(
            r.get("doom").unwrap().feature_gate,
            FeatureGate::AscendLinux
        );
    }
}
