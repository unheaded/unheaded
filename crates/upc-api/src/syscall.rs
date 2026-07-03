// SPDX-License-Identifier: GPL-3.0-or-later
//! Contract part 4 (ADR-080 §2.4, §3): the guest's syscall surface — as a
//! host-side **descriptor**, not a runtime dispatcher.
//!
//! A guest calls into the host through a set of syscall numbers. In the UPC that
//! surface is compiled into the eBPF interpreter as a hardcoded, `cfg`-partitioned
//! `if/else` chain and selected at BUILD time (ADR-080 §3): Doom's build carries
//! only DRAW_FRAME/GET_KEY/GET_TICKS/SLEEP; the ascend build carries the xv6
//! surface. The partitioning is what keeps each build under the eBPF verifier's
//! 1M-instruction budget AND keeps each guest's reachable attack surface minimal.
//!
//! Therefore this module never dispatches a syscall — it only *describes* a
//! guest's surface for the host: which numbers it uses, human-readable names, and
//! the [`FeatureGate`] (cargo feature) that compiles that surface into the object.
//! A shared loader uses the descriptor to check that the loaded eBPF object's
//! feature set actually matches the guest's declared surface (an allowlist).

/// The cargo feature that compiles a given surface into the `monad-cpu-ebpf`
/// object. Selection is a build-time decision (ADR-080 §3); this names it so the
/// host can verify the loaded object matches the guest's declared surface. The
/// concrete feature strings are the interpreter crate's, not fixed here.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum FeatureGate {
    /// The default (non-ascend) build — Doom's surface only.
    Default,
    /// The `ascend-linux` build — the xv6 / kernel surface.
    AscendLinux,
}

/// One syscall a guest declares: its number (as seen in the ecall convention)
/// and a human-readable name. No handler lives here — the handler is compiled
/// into the interpreter (ADR-080 §3). Concrete numbers belong to the guest's
/// ABI and are NOT hardcoded in this crate.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct SyscallDesc {
    /// The syscall number in the guest's calling convention.
    pub number: u32,
    /// A short human-readable name (e.g. "DRAW_FRAME", "open"), for logging and
    /// the allowlist check.
    pub name: &'static str,
}

/// Part 4 of the guest contract: a host-side description of the guest's syscall
/// surface. Object-safe so a shared loader / [`super::registry::SurfaceRegistry`]
/// can hold surfaces behind `&dyn`. It DESCRIBES; it does not dispatch.
pub trait SyscallSurface {
    /// A stable identifier for this surface (e.g. "doom", "xv6").
    fn name(&self) -> &str;

    /// The cargo feature that must be present in the loaded eBPF object for this
    /// surface's handlers to exist (ADR-080 §3).
    fn feature_gate(&self) -> FeatureGate;

    /// Every syscall this guest is permitted to issue. The shared loader treats
    /// this as an ALLOWLIST: a guest whose image invokes a number outside this
    /// set — or whose set exceeds the loaded object's feature — is rejected
    /// before it runs (BlackMage: surface is a trust boundary).
    fn syscalls(&self) -> &[SyscallDesc];
}
