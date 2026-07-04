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
//! - **IS** the type-level contract: descriptor structs, enums, and one
//!   [`validate`] free function. A [`Workload`] is plain data
//!   a loader constructs; the checks are functions over that data.
//! - **IS NOT** an interpreter, a loader, or a syscall implementation. Those
//!   live in `ebpf/monad-cpu-ebpf` (core), `cmd/upc-bootctl` +
//!   `crates/doom-runner` (loaders), and the per-workload adapters
//!   (`crates/xv6-mbc`, `crates/doom-runner`). This crate names *what* to load
//!   and refuses an inconsistent descriptor; the loaders own *how* maps are
//!   populated.
//!
//! ## Design decisions settled 2026-07-03 (Epic 1.2.3)
//!
//! The review-flagged scaffold issues are resolved in favour of the 2026-07-03
//! panel's recommendation — **descriptor structs + free functions**:
//!
//! 1. **Constructibility.** The opaque `MbcImage` / `Rv2MbcMap` handles are gone;
//!    [`image::GuestImage`] is now a struct with public `Vec<u32>` ROM / branch
//!    fields, load offsets, and an entry PC a loader fills directly.
//! 2. **Trait objects → descriptor structs.** [`syscall::SyscallSurface`] and
//!    [`Workload`] are plain structs, not `&dyn` traits — the `Workload` is data,
//!    not behaviour, and the removed indirection echoed the very dynamic dispatch
//!    the eBPF verifier forbids downstream. Validation lives in the
//!    [`validate`] free function.

#![deny(missing_docs)]

pub mod boot;
pub mod image;
pub mod load;
pub mod memory;
pub mod registry;
pub mod syscall;
pub mod workload;

// Headline re-exports for ergonomics — the contract's public face.
pub use boot::BootProtocol;
pub use image::{GuestImage, MbcPc, RvWordAddr};
pub use load::{load, ImageSink, LoadError};
pub use memory::MemoryModel;
pub use registry::SurfaceRegistry;
pub use syscall::{FeatureGate, SyscallDesc, SyscallSurface};
pub use workload::{validate, MapCapacities, ValidationError, Workload};
