// SPDX-License-Identifier: GPL-3.0-or-later
//! The shared image loader (Epic 1.2.3): one [`load`] over a [`Workload`] that
//! both loaders route their `ROM_MAP` / `RV2MBC_MAP` population through.
//!
//! Today `cmd/upc-bootctl` and `crates/doom-runner` each open the same two maps
//! and run the same `set(base + i, value)` loop by hand (ADR-080 §"intended
//! payoff"). This module names that population once: a loader builds a
//! [`Workload`] (relocating its CALL immediates / shifting its `.rv2mbc` entries
//! as it fills the [`GuestImage`]), then calls
//! `load(&workload, &caps, &mut sink)`. Adding a guest becomes "declare a
//! `Workload`," not "write a third bespoke map-population loop."
//!
//! ## What load DOES and does NOT touch
//!
//! `load` writes only the two maps parts 1+2 of the contract describe — the MBC
//! image into `ROM_MAP` and the branch-translation table into `RV2MBC_MAP`. It
//! deliberately does NOT touch:
//!
//! - **RAM staging** — Doom's WAD + ELF `.data`/`.rodata`; xv6's ramdisk,
//!   `BootParams`, CSR/IVT region, identity pgd, staged program `.data`. These are
//!   guest-specific and outside parts 1+2.
//! - **The initial CPU state** — SP, argument registers, `priv_level`,
//!   `mmu_enabled`, `page_dir_base`, `rv2mbc_base`. The [`entry`](GuestImage::entry)
//!   PC is *validated* here (it must land inside the loaded ROM) but *set* by the
//!   loader when it builds that state.
//! - **The `.rv2mbc` SHA-256 integrity gate** — the [`GuestImage`] only carries
//!   the expectation ([`expected_rv2mbc_sha256`](GuestImage::expected_rv2mbc_sha256));
//!   this crate stays zero-dep, so the loader computes and enforces the hash over
//!   the raw `.rv2mbc` bytes before it builds the descriptor.
//!
//! Keeping the seam at "raw ROM/RV2MBC writes" is what makes the refactor
//! byte-identical: `load` drives exactly the `set(base + i, value)` sequence each
//! loader emits today, proven by the characterization tests below.

use crate::image::GuestImage;
use crate::workload::{validate, MapCapacities, ValidationError, Workload};

/// The BPF-map write surface [`load`] drives. Each loader implements it over its
/// own map handles — `upc-bootctl`'s `BootRunner` (`populate_rom_at` and the
/// `RV2MBC_MAP` write loop) and `doom-runner`'s `aya::Array` handles — so both
/// populate `ROM_MAP` and `RV2MBC_MAP` through one code path.
///
/// The two methods are **batch-level** (a whole region per call), not per-word,
/// on purpose: an aya `Array` handle borrows the eBPF object, so opening it once
/// per region and looping inside — exactly what both loaders do today — is both
/// the efficient shape and the byte-identical one. The trait carries no aya types
/// and no map lifetimes, so `upc-api` stays zero-dep and never pulls in the
/// interpreter crate.
pub trait ImageSink {
    /// The loader's own write-error type (e.g. `anyhow::Error`, an aya map error).
    type Error;

    /// Write `words` into `ROM_MAP` at absolute slots `start_slot + i`, in order.
    fn write_rom(&mut self, start_slot: u32, words: &[u32]) -> Result<(), Self::Error>;

    /// Write `entries` (MBC PCs) into `RV2MBC_MAP` at absolute indices
    /// `base + i`, in order.
    fn write_rv2mbc(&mut self, base: u32, entries: &[u32]) -> Result<(), Self::Error>;
}

/// Why [`load`] refused or failed.
#[derive(Debug, PartialEq, Eq)]
pub enum LoadError<E> {
    /// The workload failed its pre-load [`validate`] check. Validation runs
    /// before the first write, so on this error every map is untouched
    /// (fail-closed).
    Invalid(ValidationError),
    /// An [`ImageSink`] write failed. Wraps the loader's own error type. `ROM_MAP`
    /// may already have been written when the `RV2MBC_MAP` write fails — the
    /// caller aborts the boot rather than run a partially-loaded image.
    Sink(E),
}

impl<E: core::fmt::Display> core::fmt::Display for LoadError<E> {
    fn fmt(&self, f: &mut core::fmt::Formatter<'_>) -> core::fmt::Result {
        match self {
            LoadError::Invalid(e) => write!(f, "workload rejected before load: {e}"),
            LoadError::Sink(e) => write!(f, "map write failed during load: {e}"),
        }
    }
}

impl<E: std::error::Error + 'static> std::error::Error for LoadError<E> {
    fn source(&self) -> Option<&(dyn std::error::Error + 'static)> {
        match self {
            LoadError::Invalid(e) => Some(e),
            LoadError::Sink(e) => Some(e),
        }
    }
}

/// Validate `workload` against `caps`, then write its image (contract parts 1+2)
/// through `sink`.
///
/// Validation ([`validate`]) runs first, so a rejected workload leaves every map
/// untouched (BlackMage: fail-closed before the first write — the descriptor is
/// the single validation choke point). On success this drives, in order:
///
/// - the ROM region at `ROM_MAP[rom_start_slot ..]`, then
/// - the branch table at `RV2MBC_MAP[rv2mbc_word_base ..]`,
///
/// which is byte-for-byte the sequence `cmd/upc-bootctl` and `crates/doom-runner`
/// emit inline today (doom loads at base 0; bootctl at its `start_slot` /
/// `text_rv_word_base`). The `base + i` additions cannot overflow `u32`:
/// `validate` already proved `rom_start_slot + rom.len()` and
/// `rv2mbc_word_base + rv2mbc.len()` both fit within the (`u32`) map capacities.
pub fn load<S: ImageSink>(
    workload: &Workload,
    caps: &MapCapacities,
    sink: &mut S,
) -> Result<(), LoadError<S::Error>> {
    validate(workload, caps).map_err(LoadError::Invalid)?;
    let image: &GuestImage = &workload.image;
    sink.write_rom(image.rom_start_slot, &image.rom)
        .map_err(LoadError::Sink)?;
    sink.write_rv2mbc(image.rv2mbc_word_base, &image.rv2mbc)
        .map_err(LoadError::Sink)?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::boot::BootProtocol;
    use crate::image::MbcPc;
    use crate::memory::MemoryModel;
    use crate::syscall::{FeatureGate, SyscallDesc, SyscallSurface};
    use core::convert::Infallible;

    const CAPS: MapCapacities = MapCapacities {
        rom_words: 0x10000,
        rv2mbc_entries: 0x10000,
        ram_words: 0x10000,
    };

    /// Test double: expands each batch write into the `(index, value)` pairs a
    /// loader's inline `set(base + i, value)` loop emits, in order. This IS the
    /// byte-identical oracle — the tests assert its contents equal that exact
    /// sequence.
    #[derive(Default)]
    struct RecordingSink {
        rom: Vec<(u32, u32)>,
        rv2mbc: Vec<(u32, u32)>,
    }

    impl ImageSink for RecordingSink {
        type Error = Infallible;
        fn write_rom(&mut self, start_slot: u32, words: &[u32]) -> Result<(), Infallible> {
            for (i, &w) in words.iter().enumerate() {
                self.rom.push((start_slot + i as u32, w));
            }
            Ok(())
        }
        fn write_rv2mbc(&mut self, base: u32, entries: &[u32]) -> Result<(), Infallible> {
            for (i, &e) in entries.iter().enumerate() {
                self.rv2mbc.push((base + i as u32, e));
            }
            Ok(())
        }
    }

    static DOOM_SURFACE: [SyscallDesc; 2] = [
        SyscallDesc {
            number: 0,
            name: "DRAW_FRAME",
        },
        SyscallDesc {
            number: 1,
            name: "GET_KEY",
        },
    ];

    /// The reference sequence a loader's inline loop emits: `set(base + i, v)`.
    fn oracle(base: u32, values: &[u32]) -> Vec<(u32, u32)> {
        values
            .iter()
            .enumerate()
            .map(|(i, &v)| (base + i as u32, v))
            .collect()
    }

    fn workload(rom_start_slot: u32, rv2mbc_word_base: u32) -> Workload {
        Workload {
            name: "test".to_string(),
            image: GuestImage {
                rom: vec![0x2700_0000, 0xDEAD_BEEF, 0x0000_0001, 0xCAFE_F00D],
                rom_start_slot,
                rv2mbc: vec![0, 4, 8, 12, 16],
                rv2mbc_word_base,
                expected_rv2mbc_sha256: None,
                entry: MbcPc(rom_start_slot),
            },
            memory: MemoryModel::Flat,
            surface: SyscallSurface {
                name: "test",
                feature_gate: FeatureGate::Default,
                syscalls: &DOOM_SURFACE,
            },
            boot: BootProtocol::Direct,
        }
    }

    #[test]
    fn doom_base_zero_writes_match_inline_loop() {
        // doom-runner: rom_map.set(i, rom[i]) and rv2mbc_map.set(i, rv2mbc[i]).
        let w = workload(0, 0);
        let mut sink = RecordingSink::default();
        load(&w, &CAPS, &mut sink).unwrap();
        assert_eq!(sink.rom, oracle(0, &w.image.rom));
        assert_eq!(sink.rv2mbc, oracle(0, &w.image.rv2mbc));
    }

    #[test]
    fn nonzero_bases_shift_by_base_like_bootctl() {
        // upc-bootctl userland path: populate_rom_at(0x4000, ..); the kernel path
        // loads rv2mbc at base 0x8000. Exercise independent, non-zero bases.
        let w = workload(0x4000, 0x8000);
        let mut sink = RecordingSink::default();
        load(&w, &CAPS, &mut sink).unwrap();
        assert_eq!(sink.rom, oracle(0x4000, &w.image.rom));
        assert_eq!(sink.rv2mbc, oracle(0x8000, &w.image.rv2mbc));
    }

    #[test]
    fn rom_written_before_rv2mbc() {
        // Order matters for a faithful oracle: both loaders write ROM first.
        let w = workload(0, 0);
        let mut sink = RecordingSink::default();
        load(&w, &CAPS, &mut sink).unwrap();
        assert_eq!(sink.rom.len(), 4);
        assert_eq!(sink.rv2mbc.len(), 5);
    }

    #[test]
    fn invalid_workload_writes_nothing() {
        // Fail-closed: a validation error must touch neither map.
        let mut w = workload(0, 0);
        w.image.rom = vec![]; // -> EmptyRom
        let mut sink = RecordingSink::default();
        let err = load(&w, &CAPS, &mut sink).unwrap_err();
        assert_eq!(err, LoadError::Invalid(ValidationError::EmptyRom));
        assert!(sink.rom.is_empty());
        assert!(sink.rv2mbc.is_empty());
    }

    #[test]
    fn out_of_range_entry_rejected_before_write() {
        let mut w = workload(0, 0);
        w.image.entry = MbcPc(9999); // past rom end -> EntryOutOfRange
        let mut sink = RecordingSink::default();
        let err = load(&w, &CAPS, &mut sink).unwrap_err();
        assert!(matches!(
            err,
            LoadError::Invalid(ValidationError::EntryOutOfRange { .. })
        ));
        assert!(sink.rom.is_empty());
    }

    #[test]
    fn sink_error_propagates() {
        // A failing sink surfaces as LoadError::Sink, not a panic.
        struct FailingSink;
        impl ImageSink for FailingSink {
            type Error = &'static str;
            fn write_rom(&mut self, _: u32, _: &[u32]) -> Result<(), &'static str> {
                Err("rom write blew up")
            }
            fn write_rv2mbc(&mut self, _: u32, _: &[u32]) -> Result<(), &'static str> {
                Ok(())
            }
        }
        let w = workload(0, 0);
        let mut sink = FailingSink;
        let err = load(&w, &CAPS, &mut sink).unwrap_err();
        assert_eq!(err, LoadError::Sink("rom write blew up"));
    }
}
