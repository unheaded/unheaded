// SPDX-License-Identifier: GPL-3.0-or-later
//! The [`Workload`] — the five contract parts bundled into one loadable guest,
//! plus the [`validate`] precondition the shared loader checks before it touches
//! a single BPF map.
//!
//! This is the type a loader (`cmd/upc-bootctl`, `crates/doom-runner`) constructs
//! and hands to one shared loader instead of hardcoding each guest's parts by
//! hand (ADR-080 §"intended payoff"). "Add a guest" then becomes "declare a
//! `Workload`," not "write a third bespoke loader."
//!
//! A `Workload` is a plain DESCRIPTOR: it names *what* to load, not *how* the maps
//! are populated (that is the loader's job). Per the 2026-07-03 panel it is data,
//! not behaviour — so the checks live in the free function [`validate`], not in a
//! method, and nothing here dispatches or executes.
//!
//! Two checks need the eBPF object the loader is about to load, so they live in
//! [`validate_against_object`] (ADR-080 §"I/O-time gates"): Gate A compares the
//! declared syscall surface with the object's `UPC_SURFACE_ID`
//! ([`check_surface`]); Gate B checks the per-pid MMU slice geometry and that it
//! is the one the object ships ([`check_mmu`]). [`crate::load`] runs both
//! before its first map write.

use crate::boot::BootProtocol;
use crate::image::GuestImage;
use crate::memory::{MemoryModel, MmuLayout, SliceGeometry};
use crate::object::{hex32, ObjectFacts};
use crate::surface_hash::surface_id;
use crate::syscall::SyscallSurface;

/// A loadable UPC guest: the five-part contract (ADR-080 §2) as one descriptor.
/// The shared loader reads these to populate `ROM_MAP` / `RV2MBC_MAP` / `RAM_MAP`,
/// set the entry PC and memory model, run the boot protocol, and validate the
/// declared syscall surface against the loaded eBPF object (an allowlist).
#[derive(Clone, Debug)]
pub struct Workload {
    /// A stable name for logging / selection (e.g. "doom", "xv6").
    pub name: String,
    /// Parts 1 + 2: the MBC image, its branch map, and entry PC.
    pub image: GuestImage,
    /// Part 3: the memory model the loader configures before dispatch.
    pub memory: MemoryModel,
    /// Part 4: the host-side descriptor of the guest's syscall surface (an
    /// allowlist the loader checks against the loaded object's feature set).
    pub surface: SyscallSurface,
    /// Part 5: how control is handed to the guest. `Direct` for the common case.
    pub boot: BootProtocol,
    /// Part 3, per-pid detail: the slice geometry and reserved RAM regions of a
    /// [`MemoryModel::PerPidMmu`] guest (Gate B). `None` for a flat guest (Doom).
    pub mmu: Option<MmuLayout>,
}

/// The BPF-map capacities the shared loader validates a [`Workload`] against.
///
/// `upc-api` takes no dependency on the interpreter crate, so the loader fills
/// these from `monad_common::mbc_maps` — the single source of truth for map
/// sizes — and passes them in. Use the `*_LARGE` maxima: the host can't know the
/// interpreter's `large-image` cargo feature at compile time, so it guards
/// against the larger ceiling and relies on `aya::Array::set` for the actual
/// per-build bound (see `mbc_maps` doc comment).
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct MapCapacities {
    /// `ROM_MAP` capacity in MBC words (`ROM_MAP_WORDS_LARGE`).
    pub rom_words: u32,
    /// `RV2MBC_MAP` capacity in entries (`RV2MBC_ENTRIES_LARGE`).
    pub rv2mbc_entries: u32,
    /// `RAM_MAP` capacity in words (`RAM_MAP_WORDS`).
    pub ram_words: u32,
}

/// Why a [`Workload`] failed its pre-load [`validate`] check. Every variant is a
/// refusal to boot a guest whose descriptor would write outside a map, hand the
/// CPU an entry PC that isn't in its own image, present a branch target outside
/// ROM, or declare an inconsistent surface (BlackMage: the descriptor is the
/// single validation choke point before any map write).
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum ValidationError {
    /// The workload or its surface has an empty name.
    EmptyName,
    /// The image ROM is empty — nothing to execute.
    EmptyRom,
    /// `rom_start_slot + rom.len()` exceeds `ROM_MAP` capacity.
    RomOverflow {
        /// First `ROM_MAP` slot the image would occupy.
        start_slot: u32,
        /// Number of words in the image.
        words: usize,
        /// `ROM_MAP` capacity (words).
        capacity: u32,
    },
    /// `rv2mbc_word_base + rv2mbc.len()` exceeds `RV2MBC_MAP` capacity.
    Rv2MbcOverflow {
        /// First `RV2MBC_MAP` index the table would occupy.
        base: u32,
        /// Number of entries in the table.
        entries: usize,
        /// `RV2MBC_MAP` capacity (entries).
        capacity: u32,
    },
    /// The entry PC is not inside the loaded ROM region
    /// `[rom_start_slot, rom_start_slot + rom.len())`.
    EntryOutOfRange {
        /// The declared entry PC.
        entry: u32,
        /// First slot of the loaded ROM region (inclusive).
        rom_start: u32,
        /// One past the last slot of the loaded ROM region (exclusive).
        rom_end: u32,
    },
    /// A branch-translation entry points at an MBC PC outside `ROM_MAP` — a
    /// corrupt or hostile `.rv2mbc` would redirect an indirect branch off the end
    /// of the image.
    Rv2MbcTargetOutOfRange {
        /// Index into the `rv2mbc` table.
        index: usize,
        /// The out-of-range MBC PC.
        target: u32,
        /// `ROM_MAP` capacity (words).
        rom_capacity: u32,
    },
    /// The syscall surface declares no syscalls — an empty allowlist can never be
    /// satisfied, so the guest could not call the host at all.
    EmptySurface,
    /// Two syscalls in the surface share a number; the allowlist is ambiguous.
    DuplicateSyscall {
        /// The duplicated syscall number.
        number: u32,
    },
    /// A `BootstubV2` guest is kernel-class, so it must declare the `AscendLinux`
    /// feature gate (the bootstub / supervisor-mode path is compiled under it).
    BootstubRequiresAscend,
    /// Gate A: the declared surface's id differs from the object's
    /// `UPC_SURFACE_ID` — the object was built for another guest (e.g. the Doom
    /// object under the xv6 kernel) or the descriptor drifted from the dispatch.
    SurfaceObjectMismatch {
        /// `surface_id` of the Workload's declared syscall numbers.
        declared: [u8; 32],
        /// `UPC_SURFACE_ID` read from the object.
        object: [u8; 32],
    },
    /// Gate B: the memory model and the presence of an [`MmuLayout`] disagree
    /// (per-pid MMU without a layout, or a flat guest with one).
    MmuLayoutMismatch {
        /// The declared memory model.
        memory: MemoryModel,
        /// Whether the Workload carries an [`MmuLayout`].
        has_layout: bool,
    },
    /// Gate B: a slice maps zero bytes.
    EmptySlice,
    /// Gate B: `max_pids` outside `1..=256` (the core's pid is a `u8`).
    MaxPidsOutOfRange {
        /// The declared process-table size.
        max_pids: u32,
    },
    /// Gate B: a child slice overlaps the next pid's slice (`stride <
    /// slice_len`) or a reserved region.
    SliceOverlap {
        /// The child pid whose slice overlaps.
        pid: u32,
        /// First byte of that slice.
        start: u64,
        /// One past its last byte.
        end: u64,
        /// What it overlaps: `"slice of pid N"` or the reserved region's name.
        other: String,
        /// First byte of the other range.
        other_start: u64,
        /// One past its last byte.
        other_end: u64,
    },
    /// Gate B: a child slice ends past `RAM_MAP` or its address arithmetic
    /// overflows `u32` (the core would wrap it into low RAM).
    SliceOutOfRange {
        /// The child pid.
        pid: u32,
        /// First byte of its slice.
        start: u64,
        /// One past its last byte.
        end: u64,
        /// `RAM_MAP` size in bytes.
        ram_bytes: u64,
    },
    /// Gate B: the per-pid MMU guest's object carries no `UPC_MMU_GEOMETRY`.
    ObjectGeometryMissing,
    /// Gate B: the declared geometry is not the one the object ships.
    SliceGeometryMismatch {
        /// The Workload's geometry.
        declared: SliceGeometry,
        /// `UPC_MMU_GEOMETRY` read from the object.
        object: SliceGeometry,
    },
}

impl core::fmt::Display for ValidationError {
    fn fmt(&self, f: &mut core::fmt::Formatter<'_>) -> core::fmt::Result {
        match self {
            ValidationError::EmptyName => write!(f, "workload or surface name is empty"),
            ValidationError::EmptyRom => write!(f, "image ROM is empty"),
            ValidationError::RomOverflow {
                start_slot,
                words,
                capacity,
            } => write!(
                f,
                "ROM overflow: start_slot {start_slot} + {words} words exceeds ROM_MAP capacity {capacity}"
            ),
            ValidationError::Rv2MbcOverflow {
                base,
                entries,
                capacity,
            } => write!(
                f,
                "RV2MBC overflow: base {base} + {entries} entries exceeds RV2MBC_MAP capacity {capacity}"
            ),
            ValidationError::EntryOutOfRange {
                entry,
                rom_start,
                rom_end,
            } => write!(
                f,
                "entry PC {entry} outside loaded ROM region [{rom_start}, {rom_end})"
            ),
            ValidationError::Rv2MbcTargetOutOfRange {
                index,
                target,
                rom_capacity,
            } => write!(
                f,
                "rv2mbc[{index}] target {target} outside ROM_MAP capacity {rom_capacity}"
            ),
            ValidationError::EmptySurface => write!(f, "syscall surface allowlist is empty"),
            ValidationError::DuplicateSyscall { number } => {
                write!(f, "duplicate syscall number {number} in surface")
            }
            ValidationError::BootstubRequiresAscend => {
                write!(f, "BootstubV2 guest must declare the AscendLinux feature gate")
            }
            ValidationError::SurfaceObjectMismatch { declared, object } => write!(
                f,
                "SurfaceObjectMismatch: declared syscall surface id {} != object UPC_SURFACE_ID {} \
                 (wrong eBPF object for this guest, e.g. built without/with --features ascend-linux)",
                hex32(declared),
                hex32(object)
            ),
            ValidationError::MmuLayoutMismatch { memory, has_layout } => write!(
                f,
                "memory model {memory:?} {} an MMU slice layout",
                if *has_layout { "must not carry" } else { "requires" }
            ),
            ValidationError::EmptySlice => write!(f, "per-pid slice length is zero"),
            ValidationError::MaxPidsOutOfRange { max_pids } => {
                write!(f, "max_pids {max_pids} outside 1..=256")
            }
            ValidationError::SliceOverlap {
                pid,
                start,
                end,
                other,
                other_start,
                other_end,
            } => write!(
                f,
                "SliceOverlap: pid {pid} slice [0x{start:X}, 0x{end:X}) overlaps {other} \
                 [0x{other_start:X}, 0x{other_end:X})"
            ),
            ValidationError::SliceOutOfRange {
                pid,
                start,
                end,
                ram_bytes,
            } => write!(
                f,
                "SliceOutOfRange: pid {pid} slice [0x{start:X}, 0x{end:X}) outside RAM_MAP \
                 [0, 0x{ram_bytes:X}) or past the u32 address space"
            ),
            ValidationError::ObjectGeometryMissing => write!(
                f,
                "object carries no UPC_MMU_GEOMETRY; a per-pid MMU guest needs it"
            ),
            ValidationError::SliceGeometryMismatch { declared, object } => write!(
                f,
                "SliceGeometryMismatch: loader geometry {declared:?} != object UPC_MMU_GEOMETRY \
                 {object:?} (update the loader's constants to the object's phase12.rs)"
            ),
        }
    }
}

impl std::error::Error for ValidationError {}

/// Validate a [`Workload`]'s internal consistency against the map `caps` before
/// the shared loader writes a single BPF map (ADR-080 §2; BlackMage, 2026-07-03
/// panel — the `Workload` is the single validation choke point).
///
/// This checks everything derivable from the descriptor alone: the image fits in
/// `ROM_MAP`, the branch table fits in `RV2MBC_MAP` and points only into ROM, the
/// entry PC lands inside the loaded image, the syscall allowlist is non-empty and
/// unambiguous, names are present, and a `BootstubV2` guest declares the ascend
/// feature. The checks that need the loaded object run in
/// [`validate_against_object`]: Gate A (surface vs `UPC_SURFACE_ID`) and Gate B
/// (per-pid slice geometry vs RAM_MAP, reserved regions and
/// `UPC_MMU_GEOMETRY`). [`crate::load`] runs this function and that one before
/// the first map write; the loaders also run Gates A and B on the object file
/// before they open it. The `.rv2mbc` SHA-256 integrity gate
/// ([`GuestImage::expected_rv2mbc_sha256`]) stays with the loader, which hashes
/// the raw file.
///
/// [`GuestImage::expected_rv2mbc_sha256`]: crate::image::GuestImage::expected_rv2mbc_sha256
pub fn validate(workload: &Workload, caps: &MapCapacities) -> Result<(), ValidationError> {
    if workload.name.is_empty() || workload.surface.name.is_empty() {
        return Err(ValidationError::EmptyName);
    }

    let img = &workload.image;
    if img.rom.is_empty() {
        return Err(ValidationError::EmptyRom);
    }

    // Image bounds: the ROM region must fit inside ROM_MAP. u64 math so a hostile
    // start_slot near u32::MAX can't wrap the sum.
    let rom_end = img.rom_start_slot as u64 + img.rom.len() as u64;
    if rom_end > caps.rom_words as u64 {
        return Err(ValidationError::RomOverflow {
            start_slot: img.rom_start_slot,
            words: img.rom.len(),
            capacity: caps.rom_words,
        });
    }

    // The branch table must fit inside RV2MBC_MAP.
    let rv_end = img.rv2mbc_word_base as u64 + img.rv2mbc.len() as u64;
    if rv_end > caps.rv2mbc_entries as u64 {
        return Err(ValidationError::Rv2MbcOverflow {
            base: img.rv2mbc_word_base,
            entries: img.rv2mbc.len(),
            capacity: caps.rv2mbc_entries,
        });
    }

    // The entry PC must land inside the loaded image, not merely inside the map.
    let entry = img.entry.0;
    if (entry as u64) < img.rom_start_slot as u64 || (entry as u64) >= rom_end {
        return Err(ValidationError::EntryOutOfRange {
            entry,
            rom_start: img.rom_start_slot,
            rom_end: rom_end as u32,
        });
    }

    // Every branch target must stay inside ROM_MAP (0 = slot 0 is legal). A target
    // past the map end means a JMPR/CALLR/MRET would fetch off the end of ROM.
    for (i, &target) in img.rv2mbc.iter().enumerate() {
        if target >= caps.rom_words {
            return Err(ValidationError::Rv2MbcTargetOutOfRange {
                index: i,
                target,
                rom_capacity: caps.rom_words,
            });
        }
    }

    // Surface allowlist must be non-empty and unambiguous.
    let syscalls = workload.surface.syscalls;
    if syscalls.is_empty() {
        return Err(ValidationError::EmptySurface);
    }
    for (i, a) in syscalls.iter().enumerate() {
        for b in &syscalls[i + 1..] {
            if a.number == b.number {
                return Err(ValidationError::DuplicateSyscall { number: a.number });
            }
        }
    }

    // A bootstub (kernel-class) guest must carry the ascend surface.
    if matches!(workload.boot, BootProtocol::BootstubV2)
        && workload.surface.feature_gate != crate::syscall::FeatureGate::AscendLinux
    {
        return Err(ValidationError::BootstubRequiresAscend);
    }

    Ok(())
}

/// Gate A (ADR-080 §"I/O-time gates"): the declared syscall surface must be the
/// one the object compiles in. Compares
/// [`surface_id`](crate::surface_hash::surface_id) of the declared numbers with
/// the object's `UPC_SURFACE_ID`; any difference refuses, fail-closed.
pub fn check_surface(surface: &SyscallSurface, facts: &ObjectFacts) -> Result<(), ValidationError> {
    let numbers: Vec<u32> = surface.syscalls.iter().map(|d| d.number).collect();
    let declared = surface_id(&numbers);
    if declared != facts.surface_id {
        return Err(ValidationError::SurfaceObjectMismatch {
            declared,
            object: facts.surface_id,
        });
    }
    Ok(())
}

/// Gate B (ADR-080 §"I/O-time gates"): per-pid MMU slice geometry.
///
/// A [`MemoryModel::Flat`] guest must carry no layout and passes. A
/// [`MemoryModel::PerPidMmu`] guest must carry one, and every child slice the
/// core can map ([`SliceGeometry::admitted_slices`]) must: be non-empty; not
/// overlap the next pid's (`stride >= slice_len`); end inside `RAM_MAP`
/// (`caps.ram_words * 4` bytes) with no `u32` wrap; and overlap no reserved
/// region. The declared geometry is checked first, then it must equal the
/// object's `UPC_MMU_GEOMETRY`, so the geometry that passes is the one the
/// interpreter ships.
pub fn check_mmu(
    memory: MemoryModel,
    mmu: Option<&MmuLayout>,
    caps: &MapCapacities,
    facts: &ObjectFacts,
) -> Result<(), ValidationError> {
    let layout = match (memory, mmu) {
        (MemoryModel::Flat, None) => return Ok(()),
        (MemoryModel::PerPidMmu, Some(l)) => l,
        (memory, mmu) => {
            return Err(ValidationError::MmuLayoutMismatch {
                memory,
                has_layout: mmu.is_some(),
            })
        }
    };
    let g = &layout.slices;
    if g.slice_len == 0 {
        return Err(ValidationError::EmptySlice);
    }
    if g.max_pids == 0 || g.max_pids > 256 {
        return Err(ValidationError::MaxPidsOutOfRange {
            max_pids: g.max_pids,
        });
    }
    let ram_bytes = caps.ram_words as u64 * 4;
    if let Some((pid, start, end)) = g.first_wrapping_slice() {
        return Err(ValidationError::SliceOutOfRange {
            pid,
            start,
            end,
            ram_bytes,
        });
    }

    let slices = g.admitted_slices();
    if g.stride < g.slice_len {
        // Uniform stride: pid p's slice runs into pid p+1's. Report the first
        // pair, or pid 1 against the slot pid 2 would occupy.
        let (pid, start, end) = slices.first().copied().unwrap_or((
            1,
            g.base as u64,
            g.base as u64 + g.slice_len as u64,
        ));
        let other_start = start + g.stride as u64;
        return Err(ValidationError::SliceOverlap {
            pid,
            start,
            end,
            other: format!("slice of pid {}", pid + 1),
            other_start,
            other_end: other_start + g.slice_len as u64,
        });
    }
    for &(pid, start, end) in &slices {
        if end > ram_bytes {
            return Err(ValidationError::SliceOutOfRange {
                pid,
                start,
                end,
                ram_bytes,
            });
        }
        for r in &layout.reserved {
            let (rs, re) = (r.start as u64, r.start as u64 + r.len as u64);
            if r.len != 0 && start < re && rs < end {
                return Err(ValidationError::SliceOverlap {
                    pid,
                    start,
                    end,
                    other: r.name.to_string(),
                    other_start: rs,
                    other_end: re,
                });
            }
        }
    }

    match facts.mmu_geometry {
        None => Err(ValidationError::ObjectGeometryMissing),
        Some(object) if object != *g => Err(ValidationError::SliceGeometryMismatch {
            declared: *g,
            object,
        }),
        Some(_) => Ok(()),
    }
}

/// Both I/O-time gates for one [`Workload`] against the object it will run on:
/// [`check_surface`] then [`check_mmu`]. Called by [`crate::load`] after
/// [`validate`] and before any map write.
pub fn validate_against_object(
    workload: &Workload,
    caps: &MapCapacities,
    facts: &ObjectFacts,
) -> Result<(), ValidationError> {
    check_surface(&workload.surface, facts)?;
    check_mmu(workload.memory, workload.mmu.as_ref(), caps, facts)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::image::MbcPc;
    use crate::syscall::{FeatureGate, SyscallDesc};

    // A small, permissive set of capacities so overflow is easy to exercise.
    const CAPS: MapCapacities = MapCapacities {
        rom_words: 1024,
        rv2mbc_entries: 512,
        ram_words: 4096,
    };

    static DOOMISH_SURFACE: [SyscallDesc; 2] = [
        SyscallDesc {
            number: 0,
            name: "DRAW_FRAME",
        },
        SyscallDesc {
            number: 1,
            name: "GET_KEY",
        },
    ];

    fn sample() -> Workload {
        Workload {
            name: "doom".to_string(),
            image: GuestImage {
                rom: vec![0xDEAD_BEEF; 100],
                rom_start_slot: 0,
                rv2mbc: vec![0, 4, 8, 12],
                rv2mbc_word_base: 0,
                expected_rv2mbc_sha256: None,
                entry: MbcPc(0),
            },
            memory: MemoryModel::Flat,
            surface: SyscallSurface {
                name: "doom",
                feature_gate: FeatureGate::Default,
                syscalls: &DOOMISH_SURFACE,
            },
            boot: BootProtocol::Direct,
            mmu: None,
        }
    }

    #[test]
    fn valid_workload_passes() {
        assert_eq!(validate(&sample(), &CAPS), Ok(()));
    }

    #[test]
    fn empty_workload_name_rejected() {
        let mut w = sample();
        w.name = String::new();
        assert_eq!(validate(&w, &CAPS), Err(ValidationError::EmptyName));
    }

    #[test]
    fn empty_rom_rejected() {
        let mut w = sample();
        w.image.rom = vec![];
        assert_eq!(validate(&w, &CAPS), Err(ValidationError::EmptyRom));
    }

    #[test]
    fn rom_overflow_rejected() {
        let mut w = sample();
        w.image.rom_start_slot = 1000;
        w.image.entry = MbcPc(1000);
        // 1000 + 100 = 1100 > 1024.
        assert_eq!(
            validate(&w, &CAPS),
            Err(ValidationError::RomOverflow {
                start_slot: 1000,
                words: 100,
                capacity: 1024,
            })
        );
    }

    #[test]
    fn rom_overflow_does_not_wrap_on_huge_start_slot() {
        // A hostile start_slot near u32::MAX must report overflow, not wrap the
        // u32 addition to a small in-range sum.
        let mut w = sample();
        w.image.rom_start_slot = u32::MAX - 10;
        w.image.entry = MbcPc(u32::MAX - 10);
        assert!(matches!(
            validate(&w, &CAPS),
            Err(ValidationError::RomOverflow { .. })
        ));
    }

    #[test]
    fn rv2mbc_overflow_rejected() {
        let mut w = sample();
        w.image.rv2mbc_word_base = 510;
        // 510 + 4 = 514 > 512.
        assert!(matches!(
            validate(&w, &CAPS),
            Err(ValidationError::Rv2MbcOverflow { .. })
        ));
    }

    #[test]
    fn entry_past_rom_end_rejected() {
        let mut w = sample();
        w.image.entry = MbcPc(100); // rom is [0,100), so 100 is one past the end.
        assert_eq!(
            validate(&w, &CAPS),
            Err(ValidationError::EntryOutOfRange {
                entry: 100,
                rom_start: 0,
                rom_end: 100,
            })
        );
    }

    #[test]
    fn entry_below_rom_start_rejected() {
        let mut w = sample();
        w.image.rom_start_slot = 50;
        w.image.entry = MbcPc(10); // below the loaded region start.
                                   // rom now spans [50, 150).
        assert!(matches!(
            validate(&w, &CAPS),
            Err(ValidationError::EntryOutOfRange { .. })
        ));
    }

    #[test]
    fn nonzero_rom_start_with_entry_inside_passes() {
        let mut w = sample();
        w.image.rom_start_slot = 0x100;
        w.image.entry = MbcPc(0x104); // inside [0x100, 0x100+100).
        assert_eq!(validate(&w, &CAPS), Ok(()));
    }

    #[test]
    fn rv2mbc_target_outside_rom_rejected() {
        let mut w = sample();
        w.image.rv2mbc = vec![0, 4, 5000]; // 5000 >= rom_words 1024.
        assert_eq!(
            validate(&w, &CAPS),
            Err(ValidationError::Rv2MbcTargetOutOfRange {
                index: 2,
                target: 5000,
                rom_capacity: 1024,
            })
        );
    }

    #[test]
    fn zero_rv2mbc_target_is_legal() {
        // 0 = ROM slot 0 is a valid branch target (dense tables have gaps).
        let mut w = sample();
        w.image.rv2mbc = vec![0, 0, 0, 0];
        assert_eq!(validate(&w, &CAPS), Ok(()));
    }

    #[test]
    fn empty_surface_rejected() {
        static EMPTY: [SyscallDesc; 0] = [];
        let mut w = sample();
        w.surface.syscalls = &EMPTY;
        assert_eq!(validate(&w, &CAPS), Err(ValidationError::EmptySurface));
    }

    #[test]
    fn duplicate_syscall_number_rejected() {
        static DUP: [SyscallDesc; 2] = [
            SyscallDesc {
                number: 7,
                name: "a",
            },
            SyscallDesc {
                number: 7,
                name: "b",
            },
        ];
        let mut w = sample();
        w.surface.syscalls = &DUP;
        assert_eq!(
            validate(&w, &CAPS),
            Err(ValidationError::DuplicateSyscall { number: 7 })
        );
    }

    #[test]
    fn bootstub_without_ascend_rejected() {
        let mut w = sample();
        w.boot = BootProtocol::BootstubV2; // surface is still FeatureGate::Default.
        assert_eq!(
            validate(&w, &CAPS),
            Err(ValidationError::BootstubRequiresAscend)
        );
    }

    #[test]
    fn bootstub_with_ascend_passes() {
        static XV6_SURFACE: [SyscallDesc; 1] = [SyscallDesc {
            number: 16,
            name: "write",
        }];
        let mut w = sample();
        w.boot = BootProtocol::BootstubV2;
        w.memory = MemoryModel::PerPidMmu;
        w.surface = SyscallSurface {
            name: "xv6",
            feature_gate: FeatureGate::AscendLinux,
            syscalls: &XV6_SURFACE,
        };
        assert_eq!(validate(&w, &CAPS), Ok(()));
    }

    #[test]
    fn error_display_is_human_readable() {
        let e = ValidationError::RomOverflow {
            start_slot: 1000,
            words: 100,
            capacity: 1024,
        };
        let s = format!("{e}");
        assert!(s.contains("ROM overflow"));
        assert!(s.contains("1024"));
    }
}
