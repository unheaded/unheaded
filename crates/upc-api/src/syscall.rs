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
//!
//! ## Descriptor, not trait object (resolved 2026-07-03, Epic 1.2.3)
//!
//! The surface was previously a `SyscallSurface` trait held behind `&dyn`. It is
//! now a plain struct: a name, a [`FeatureGate`], and a slice of [`SyscallDesc`].
//! The `&dyn` indirection bought nothing (the surface is data, not behaviour) and
//! echoed the very dynamic dispatch the verifier forbids downstream.

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
/// surface. It DESCRIBES; it does not dispatch.
///
/// The shared loader treats [`syscalls`](Self::syscalls) as an ALLOWLIST and
/// [`feature_gate`](Self::feature_gate) as the object it must have been built
/// with. The syscall table is a `&'static` slice because each guest's surface is
/// a compile-time constant the loader owns (e.g. `static DOOM_SURFACE`); this
/// crate never declares one.
#[derive(Clone, Copy, Debug)]
pub struct SyscallSurface {
    /// A stable identifier for this surface (e.g. "doom", "xv6").
    pub name: &'static str,
    /// The cargo feature that must be present in the loaded eBPF object for this
    /// surface's handlers to exist (ADR-080 §3).
    pub feature_gate: FeatureGate,
    /// Every syscall this guest is permitted to issue. The shared loader rejects,
    /// before it runs, any guest whose image invokes a number outside this set —
    /// or whose set exceeds the loaded object's feature (BlackMage: the surface
    /// is a trust boundary).
    pub syscalls: &'static [SyscallDesc],
}
