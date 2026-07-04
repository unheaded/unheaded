// SPDX-License-Identifier: GPL-3.0-or-later
//! Contract parts 1 + 2 (ADR-080 §2): the MBC image and its branch map.
//!
//! A guest supplies an MBC bytecode image (`ROM_MAP`, one word per slot) plus a
//! branch-translation map (`RV2MBC_MAP`) that turns RV word addresses into MBC
//! program counters for indirect branches (JMPR/CALLR) and privileged returns
//! (MRET/SRET). Return addresses are tagged (ADR-079) so the ABI is
//! image-size-independent.
//!
//! This module carries the host-side *data* a loader (`cmd/upc-bootctl`,
//! `crates/doom-runner`) reads to populate the `ROM_MAP` / `RV2MBC_MAP` BPF maps
//! and set the initial CPU PC. It is a plain descriptor — the loader owns the
//! files on disk and fills these fields; nothing here dispatches or executes.
//!
//! ## Descriptor, not opaque handle (resolved 2026-07-03, Epic 1.2.3)
//!
//! An earlier scaffold used opaque `MbcImage` / `Rv2MbcMap` handles behind a
//! `GuestImage` trait. Those types had no constructors, so a loader outside this
//! crate could not build one — the contract was unimplementable. Per the panel's
//! recommendation the image is now a **descriptor struct**: the ROM words, the
//! branch table, the load offsets, and the entry PC are all public data the
//! loader fills directly.

/// A program-counter value in MBC space: a word index into the image's ROM
/// region. Concrete entry values are the loader's convention (ADR-080 §5:
/// Doom PC=0, xv6 PC=0x4000) and are NEVER hardcoded in this crate.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct MbcPc(pub u32);

/// A word address in the RV32I source program (pre-translation), used as the
/// key into a guest's branch-translation table.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct RvWordAddr(pub u32);

/// Parts 1 + 2 of the guest contract: an MBC image, its branch-translation
/// table, and the entry PC — as host-side data.
///
/// The loader constructs one of these (from the `.mbc` / `.rv2mbc` artifacts a
/// guest ships) and hands it to the shared loader, which writes [`rom`] into
/// `ROM_MAP` at [`rom_start_slot`] and [`rv2mbc`] into `RV2MBC_MAP` at
/// [`rv2mbc_word_base`], then sets the CPU PC to [`entry`]. Today
/// `cmd/upc-bootctl` and `crates/doom-runner` build these regions inline; this
/// struct is the shared shape they converge on.
///
/// [`rom`]: GuestImage::rom
/// [`rom_start_slot`]: GuestImage::rom_start_slot
/// [`rv2mbc`]: GuestImage::rv2mbc
/// [`rv2mbc_word_base`]: GuestImage::rv2mbc_word_base
/// [`entry`]: GuestImage::entry
#[derive(Clone, Debug, Default)]
pub struct GuestImage {
    /// The MBC bytecode image — `ROM_MAP` contents, one `u32` per slot. Produced
    /// offline by `rv32i_to_mbc` or emitted directly (ADR-080 §2.1).
    pub rom: Vec<u32>,

    /// The `ROM_MAP` word index at which [`rom`](Self::rom) is written. Doom and
    /// pid-0 xv6 both use 0 (the `.mbc` self-locates from offset 0); Boot
    /// Protocol v2 stages a bootstub at one slot and the kernel at another
    /// (upc-bootctl uses 0x4000 / 0x8000).
    pub rom_start_slot: u32,

    /// The RV→MBC branch-translation table — `RV2MBC_MAP` contents. A flat array
    /// where `rv2mbc[i]` is the MBC PC for RV word address
    /// `rv2mbc_word_base + i`. Consulted by the interpreter for JMPR / CALLR /
    /// MRET / SRET (ADR-080 §2.2; ADR-077/079).
    pub rv2mbc: Vec<u32>,

    /// The absolute `RV2MBC_MAP` index at which [`rv2mbc`](Self::rv2mbc) is
    /// written — the per-program disjoint base (`rv2mbc_base`, ADR-077) that lets
    /// multiple images share the core without colliding. For xv6-mbc the `.text`
    /// section starts at RV word 0x8000, so this is the shift the loader applies.
    pub rv2mbc_word_base: u32,

    /// Optional integrity gate: the expected lowercase-hex SHA-256 of the
    /// `.rv2mbc` artifact's on-disk bytes (a flat little-endian `u32` array, so
    /// this equals the SHA-256 of `rv2mbc` re-serialized little-endian).
    ///
    /// This is a **load-time** BlackMage hook (ADR-075 §Security #1): a tampered
    /// `.rv2mbc` file redirects every indirect branch in the image, so when this
    /// is `Some`, the shared loader passes it to the runner's `populate_rv2mbc`
    /// as `expected_sha` and refuses to boot on mismatch. `upc-api` itself takes
    /// no dependency on a hashing crate; it only carries the expectation.
    pub expected_rv2mbc_sha256: Option<String>,

    /// The entry MBC PC the loader sets on the CPU. Value is the loader's
    /// convention (ADR-080 §5); the descriptor only carries it.
    pub entry: MbcPc,
}
