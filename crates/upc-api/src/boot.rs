// SPDX-License-Identifier: GPL-3.0-or-later
//! Contract part 5 (ADR-080 §2.5): the guest's boot protocol.
//!
//! Most guests are entered directly at their entry PC (Doom PC=0, xv6 PC=0x4000)
//! with no boot handshake. Kernel-class guests may instead use **UPC Boot
//! Protocol v2**: the loader writes a 256-byte `BootParams` at a fixed address
//! and prepends a stage-1 `upc-bootstub` that verifies the params, sets
//! `MEPC`/`MSTATUS.MPP`, and `MRET`s into the kernel in supervisor mode
//! (ADR-067; `crates/upc-bootstub`; `cmd/upc-bootctl/src/bootparams.rs`).
//!
//! This is an OPTIONAL / strategy-specific part of the contract (Scientist,
//! 2026-07-03 panel): a guest that boots directly does not carry one. Concrete
//! `BootParams` fields and addresses live in the loader, not here.

/// How the host loader hands control to a guest (ADR-080 §2.5). Optional per
/// guest — [`Direct`](BootProtocol::Direct) is the common case.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum BootProtocol {
    /// Enter the guest directly at its entry PC; no boot handshake. Doom and the
    /// current xv6 path both use this.
    Direct,
    /// UPC Boot Protocol v2: write `BootParams`, prepend the `upc-bootstub`
    /// stage-1 stub, `MRET` into the kernel at supervisor privilege (ADR-067).
    /// The params layout is loader-defined (`bootparams.rs`), not fixed here.
    BootstubV2,
}
