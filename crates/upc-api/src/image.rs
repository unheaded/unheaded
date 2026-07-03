// SPDX-License-Identifier: GPL-3.0-or-later
//! Contract parts 1 + 2 (ADR-080 §2): the MBC image and its branch map.
//!
//! A guest supplies an MBC bytecode image (`ROM_MAP`, one word per slot) plus a
//! branch-translation map (`RV2MBC_MAP`) that turns RV word addresses into MBC
//! program counters for indirect branches (JMPR/CALLR) and privileged returns
//! (MRET/SRET). Return addresses are tagged (ADR-079) so the ABI is
//! image-size-independent.
//!
//! This module names only the host-side *loading* accessors: what the loader
//! (`cmd/upc-bootctl`, `crates/doom-runner`) reads in order to populate the
//! `ROM_MAP` / `RV2MBC_MAP` BPF maps and set the initial CPU PC. No concrete
//! image data or addresses live here.

/// A program-counter value in MBC space: a word index into the image's ROM
/// region. Concrete entry values are the loader's convention (ADR-080 §5:
/// Doom PC=0, xv6 PC=0x4000) and are NEVER hardcoded in this crate.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct MbcPc(pub u32);

/// A word address in the RV32I source program (pre-translation), used as the
/// key into [`Rv2MbcMap`].
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct RvWordAddr(pub u32);

/// Opaque handle to the MBC bytecode image the loader will write into `ROM_MAP`.
/// The backing store (a host `Vec`, an mmap of the `.mbc` artifact) is chosen by
/// the loader; this crate names the contract, not the storage. Sizing limits
/// (`ROM_MAP` = 262 K words today) are an open Phase-2 substrate decision
/// (ADR-080 §4) — not encoded here.
pub struct MbcImage {
    // Backing store is loader-defined; no fields in the scaffold.
    _private: (),
}

/// Opaque handle to the branch-translation map (`RV2MBC_MAP`). Occupies a
/// per-program disjoint region via `rv2mbc_base` (ADR-077) so multiple images
/// can share the core without collisions.
pub struct Rv2MbcMap {
    // Backing store is loader-defined; no fields in the scaffold.
    _private: (),
}

impl Rv2MbcMap {
    /// Translate an RV word address to its MBC PC, if this image maps it.
    /// Consulted by the loader when it must seed indirect-branch / privileged
    /// return targets (ADR-080 §2.2; ADR-077/079).
    pub fn translate(&self, _rv: RvWordAddr) -> Option<MbcPc> {
        todo!("look up `rv` in RV2MBC_MAP; source region is per-program via rv2mbc_base (ADR-077)")
    }
}

/// Parts 1 + 2 of the guest contract: an MBC image, its branch map, and the
/// entry PC. Describes what the host loader reads to populate the BPF maps and
/// set the initial CPU state. Today `cmd/upc-bootctl` and `crates/doom-runner`
/// perform this population by hand; this trait abstracts it.
pub trait GuestImage {
    /// The MBC bytecode image (`ROM_MAP` contents). Produced offline by
    /// `rv32i_to_mbc` or emitted directly (ADR-080 §2.1).
    fn rom(&self) -> &MbcImage;

    /// The RV→MBC branch-translation map (`RV2MBC_MAP` contents) for this image.
    fn rv2mbc(&self) -> &Rv2MbcMap;

    /// The entry MBC PC the loader sets on the CPU. Value is the loader's
    /// convention (ADR-080 §5); the trait only names the accessor.
    fn entry(&self) -> MbcPc;
}
