// SPDX-License-Identifier: GPL-3.0-or-later
//
//! # upc-api — the UPC programmatic-API surface (bones)
//!
//! Scaffold-only skeleton for the **UPC guest contract** defined in
//! `docs/adr/ADR-080-upc-programmatic-api.md`. The UPC (Unheaded Protocol
//! Computer) is a general substrate that runs *arbitrary MBC programs* inside
//! an eBPF/XDP interpreter core. **Doom** and **xv6 / ASCEND-LINUX** are its
//! first two guests (ADR-080 §5); this crate names the contract they both
//! satisfy so future guests (uClinux/Linux, experiments) plug in the same way.
//!
//! ## Where this crate runs (read this first)
//!
//! `upc-api` is a **host-side, userspace orchestration** crate. It is intended
//! to be consumed by the loaders — `cmd/upc-bootctl` (xv6/Linux) and
//! `crates/doom-runner` (Doom) — which populate the BPF maps, set the entry PC,
//! write `BootParams`, and drive trigger packets.
//!
//! It does **not** run inside the eBPF interpreter (`ebpf/monad-cpu-ebpf`). The
//! BPF verifier forbids the dynamic dispatch / trait objects / function-pointer
//! tables a runtime "registered dispatch" would need, so the interpreter's
//! syscall dispatch **must** stay a hardcoded, `cfg`-partitioned `if/else`
//! chain (ADR-080 §3). Accordingly the types here *describe* a guest's surface
//! for the host; they never dispatch a syscall at runtime.
//!
//! ## The five-part guest contract (ADR-080 §2)
//!
//! | Part | Item | Module |
//! |------|------|--------|
//! | 1+2  | MBC image (`ROM_MAP`) + branch-translation map (`RV2MBC_MAP`) + entry PC | [`image`] |
//! | 3    | Memory model (flat RAM vs per-pid Sv32 MMU) | [`memory`] |
//! | 4    | Syscall surface — host-side *descriptor* of the guest's API into the host | [`syscall`] |
//! | 5    | Boot protocol (none, or Boot Protocol v2 bootstub) | [`boot`] |
//! | —    | The four bundled into one loadable guest | [`workload`] |
//! | —    | Host-side registry of surfaces (ADR-080 §4) | [`registry`] |
//!
//! ## The intended payoff
//!
//! Today `cmd/upc-bootctl` and `crates/doom-runner` each hardcode their guest's
//! five parts independently. The goal of this contract is that both would
//! instead construct a [`Workload`] and hand it to **one shared loader** — so
//! adding a new guest is "declare a [`Workload`]," not "write a third bespoke
//! loader." The [`registry`] is the host-side catalog that shared loader
//! consults to select among guests (ADR-080 §4 registered-dispatch model,
//! realized on the host rather than in the verifier-bound interpreter).
//!
//! ## What this crate IS / IS NOT
//!
//! - **IS** the type-level contract: traits, enums, and opaque placeholder
//!   types with doc comments. Every function body is `todo!()`.
//! - **IS NOT** an interpreter, a loader, or a syscall implementation. Those
//!   live in `ebpf/monad-cpu-ebpf` (core), `cmd/upc-bootctl` +
//!   `crates/doom-runner` (loaders), and the per-workload adapters
//!   (`crates/xv6-mbc`, `crates/doom-runner`).
//!
//! ## Open design issues to settle in Epic 1.2.3 adoption (found on review)
//!
//! 1. **The opaque types are not constructible, so the traits can't be
//!    implemented.** [`image::MbcImage`] / [`image::Rv2MbcMap`] have a private
//!    `()` field and no constructor, and [`image::GuestImage`] returns
//!    `&MbcImage` — a loader outside this crate cannot build one to return. The
//!    fix is part of the trait-vs-struct decision below.
//! 2. **Trait objects vs descriptor structs.** This scaffold uses object-safe
//!    traits (`&dyn GuestImage` / `&dyn SyscallSurface`); the 2026-07-03 panel
//!    (Developer) preferred **plain descriptor structs + free functions** — the
//!    `Workload` is *data*, not behaviour, and `&dyn` adds the same indirection
//!    the eBPF verifier forbids downstream. Moving to descriptor structs (fields
//!    the loader fills: a `Vec<u32>` image, an rv2mbc map, an entry PC) also
//!    dissolves issue 1 (no opaque types to construct). Recommended direction.

#![deny(missing_docs)]

pub mod boot;
pub mod image;
pub mod memory;
pub mod registry;
pub mod syscall;
pub mod workload;

// Headline re-exports for ergonomics — the contract's public face.
pub use boot::BootProtocol;
pub use image::GuestImage;
pub use memory::MemoryModel;
pub use registry::SurfaceRegistry;
pub use syscall::{FeatureGate, SyscallDesc, SyscallSurface};
pub use workload::Workload;

#[cfg(test)]
mod tests {
    use super::*;

    // Compile-time proof the contract traits are object-safe: the host-side
    // registry (ADR-080 §4) holds surfaces behind `&dyn`, and a shared loader
    // holds images behind `&dyn`. Type-level only — no `todo!()` is reached.
    #[allow(dead_code)]
    fn _surface_is_object_safe(_s: &dyn syscall::SyscallSurface) {}
    #[allow(dead_code)]
    fn _image_is_object_safe(_i: &dyn image::GuestImage) {}
}
