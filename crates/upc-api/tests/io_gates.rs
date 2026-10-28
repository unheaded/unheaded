// SPDX-License-Identifier: GPL-3.0-or-later
//! The loader's two I/O-time gates (ADR-080 §"I/O-time gates", S-ASCEND-2
//! Steps 95-106): Gate A compares the Workload's declared syscall surface with
//! the `UPC_SURFACE_ID` the eBPF object carries; Gate B checks the per-pid MMU
//! slice geometry. Both run inside `load()` before the first map write.
//!
//! The geometry used here is synthetic but shaped like the shipped xv6 one
//! (cmd/upc-bootctl/src/geometry.rs carries the real constants and its own
//! test); the gate itself takes every value from its arguments.

use core::convert::Infallible;
use upc_api::surface_hash::{sha256, surface_id};
use upc_api::{
    load, validate_against_object, BootProtocol, FeatureGate, GuestImage, ImageSink, LoadError,
    MapCapacities, MbcPc, MemRegion, MemoryModel, MmuLayout, ObjectFacts, ObjectFactsError,
    SliceGeometry, SyscallDesc, SyscallSurface, ValidationError, Workload,
};

const MIB: u32 = 0x0010_0000;

/// RAM_MAP of 64 MiB (16 Mi words), like `monad_common::mbc_maps::RAM_MAP_WORDS`.
const CAPS: MapCapacities = MapCapacities {
    rom_words: 0x10000,
    rv2mbc_entries: 0x10000,
    ram_words: 16 * MIB,
};

static KERNELISH: [SyscallDesc; 3] = [
    SyscallDesc {
        number: 1,
        name: "fork",
    },
    SyscallDesc {
        number: 16,
        name: "write",
    },
    SyscallDesc {
        number: 21,
        name: "close",
    },
];

static KERNELISH_PLUS_ONE: [SyscallDesc; 4] = [
    SyscallDesc {
        number: 1,
        name: "fork",
    },
    SyscallDesc {
        number: 16,
        name: "write",
    },
    SyscallDesc {
        number: 21,
        name: "close",
    },
    SyscallDesc {
        number: 4,
        name: "pipe",
    },
];

static DOOMISH: [SyscallDesc; 2] = [
    SyscallDesc {
        number: 1,
        name: "DRAW_FRAME",
    },
    SyscallDesc {
        number: 2,
        name: "GET_KEY",
    },
];

fn numbers(s: &[SyscallDesc]) -> Vec<u32> {
    s.iter().map(|d| d.number).collect()
}

/// 16 MiB base, 8 MiB stride and mapped length, 56 MiB ceiling, 8 pids.
fn geometry() -> SliceGeometry {
    SliceGeometry {
        base: 16 * MIB,
        stride: 8 * MIB,
        slice_len: 8 * MIB,
        ceiling: 56 * MIB,
        max_pids: 8,
    }
}

fn reserved() -> Vec<MemRegion> {
    vec![
        MemRegion {
            name: "kernel",
            start: 0,
            len: 8 * MIB,
        },
        MemRegion {
            name: "ramdisk",
            start: 8 * MIB,
            len: 4 * MIB,
        },
        MemRegion {
            name: "pgd",
            start: 0x00F0_0000,
            len: 8 * 0x1000,
        },
    ]
}

fn kernel_workload(syscalls: &'static [SyscallDesc]) -> Workload {
    Workload {
        name: "xv6".to_string(),
        image: GuestImage {
            rom: vec![0x2700_0000, 0xDEAD_BEEF, 0x0000_0001, 0xCAFE_F00D],
            rom_start_slot: 0x4000,
            rv2mbc: vec![0x4000, 0x4001],
            rv2mbc_word_base: 0,
            expected_rv2mbc_sha256: None,
            entry: MbcPc(0x4000),
        },
        memory: MemoryModel::PerPidMmu,
        surface: SyscallSurface {
            name: "xv6",
            feature_gate: FeatureGate::AscendLinux,
            syscalls,
        },
        boot: BootProtocol::Direct,
        mmu: Some(MmuLayout {
            slices: geometry(),
            reserved: reserved(),
        }),
    }
}

fn doom_workload() -> Workload {
    Workload {
        name: "doom".to_string(),
        image: GuestImage {
            rom: vec![0xDEAD_BEEF; 8],
            rom_start_slot: 0,
            rv2mbc: vec![0, 4],
            rv2mbc_word_base: 0,
            expected_rv2mbc_sha256: None,
            entry: MbcPc(0),
        },
        memory: MemoryModel::Flat,
        surface: SyscallSurface {
            name: "doom",
            feature_gate: FeatureGate::Default,
            syscalls: &DOOMISH,
        },
        boot: BootProtocol::Direct,
        mmu: None,
    }
}

/// The facts an object built for `syscalls` with `geometry()` would carry.
fn facts_for(syscalls: &[SyscallDesc]) -> ObjectFacts {
    ObjectFacts {
        surface_id: surface_id(&numbers(syscalls)),
        mmu_geometry: Some(geometry()),
    }
}

#[derive(Default)]
struct RecordingSink {
    rom: Vec<(u32, u32)>,
    rv2mbc: Vec<(u32, u32)>,
    frozen: bool,
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
    fn finalize(&mut self) -> Result<(), Infallible> {
        self.frozen = true;
        Ok(())
    }
}

fn oracle(base: u32, values: &[u32]) -> Vec<(u32, u32)> {
    values
        .iter()
        .enumerate()
        .map(|(i, &v)| (base + i as u32, v))
        .collect()
}

// ── (1) Gate A: matching surface passes ──────────────────────────────────────

#[test]
fn surface_id_match_passes() {
    let w = kernel_workload(&KERNELISH);
    assert_eq!(
        validate_against_object(&w, &CAPS, &facts_for(&KERNELISH)),
        Ok(())
    );
}

// ── (2) Gate A: one number added to the declared surface ─────────────────────

#[test]
fn surface_with_one_extra_number_is_refused() {
    let w = kernel_workload(&KERNELISH_PLUS_ONE);
    let facts = facts_for(&KERNELISH);
    match validate_against_object(&w, &CAPS, &facts) {
        Err(ValidationError::SurfaceObjectMismatch { declared, object }) => {
            assert_eq!(declared, surface_id(&numbers(&KERNELISH_PLUS_ONE)));
            assert_eq!(object, facts.surface_id);
        }
        other => panic!("expected SurfaceObjectMismatch, got {other:?}"),
    }
}

#[test]
fn doom_surface_against_ascend_object_is_refused() {
    // The Step 109 mistake in miniature: the xv6 kernel descriptor checked
    // against an object built for Doom's surface.
    let w = kernel_workload(&KERNELISH);
    let facts = facts_for(&DOOMISH);
    assert!(matches!(
        validate_against_object(&w, &CAPS, &facts),
        Err(ValidationError::SurfaceObjectMismatch { .. })
    ));
}

#[test]
fn surface_mismatch_display_names_both_ids() {
    let e = ValidationError::SurfaceObjectMismatch {
        declared: [0xAB; 32],
        object: [0xCD; 32],
    };
    let s = e.to_string();
    assert!(s.contains("abababab"), "{s}");
    assert!(s.contains("cdcdcdcd"), "{s}");
}

// ── (3) Gate B: stride below the mapped slice length ─────────────────────────

#[test]
fn stride_below_slice_len_is_overlap() {
    let mut w = kernel_workload(&KERNELISH);
    // 6 MiB: gate_nway.c's falsification stride (HIGH_BASE).
    w.mmu.as_mut().unwrap().slices.stride = 6 * MIB;
    assert!(matches!(
        validate_against_object(&w, &CAPS, &facts_for(&KERNELISH)),
        Err(ValidationError::SliceOverlap { .. })
    ));
}

// ── (4) Gate B: a slice past the end of RAM_MAP ──────────────────────────────

#[test]
fn slice_past_ram_map_is_out_of_range() {
    let mut w = kernel_workload(&KERNELISH);
    let g = &mut w.mmu.as_mut().unwrap().slices;
    g.base = 60 * MIB; // pid 1 maps [60, 68) MiB of a 64 MiB RAM_MAP
    g.ceiling = 72 * MIB;
    match validate_against_object(&w, &CAPS, &facts_for(&KERNELISH)) {
        Err(ValidationError::SliceOutOfRange {
            pid,
            end,
            ram_bytes,
            ..
        }) => {
            assert_eq!(pid, 1);
            assert_eq!(end, 68 * MIB as u64);
            assert_eq!(ram_bytes, 64 * MIB as u64);
        }
        other => panic!("expected SliceOutOfRange, got {other:?}"),
    }
}

// ── (5) Gate B: a slice overlapping the ramdisk ──────────────────────────────

#[test]
fn slice_overlapping_ramdisk_is_overlap() {
    let mut w = kernel_workload(&KERNELISH);
    w.mmu.as_mut().unwrap().slices.base = 10 * MIB; // pid 1 maps [10, 18) MiB
    match validate_against_object(&w, &CAPS, &facts_for(&KERNELISH)) {
        Err(ValidationError::SliceOverlap { pid, other, .. }) => {
            assert_eq!(pid, 1);
            assert_eq!(other, "ramdisk");
        }
        other => panic!("expected SliceOverlap with the ramdisk, got {other:?}"),
    }
}

#[test]
fn slice_overlapping_pgd_region_is_overlap() {
    let mut w = kernel_workload(&KERNELISH);
    w.mmu.as_mut().unwrap().slices.base = 12 * MIB; // [12, 20) MiB covers 0x00F00000
    match validate_against_object(&w, &CAPS, &facts_for(&KERNELISH)) {
        Err(ValidationError::SliceOverlap { other, .. }) => assert_eq!(other, "pgd"),
        other => panic!("expected SliceOverlap with the pgd region, got {other:?}"),
    }
}

#[test]
fn only_slices_under_the_ceiling_are_checked() {
    // pids whose slice top would pass the ceiling are refused by fork(), so a
    // reserved region above the ceiling is never mapped and must not trip the gate.
    let mut w = kernel_workload(&KERNELISH);
    w.mmu.as_mut().unwrap().reserved.push(MemRegion {
        name: "boot stack",
        start: 56 * MIB,
        len: 7 * MIB,
    });
    assert_eq!(
        validate_against_object(&w, &CAPS, &facts_for(&KERNELISH)),
        Ok(())
    );
}

#[test]
fn zero_length_slice_is_refused() {
    let mut w = kernel_workload(&KERNELISH);
    w.mmu.as_mut().unwrap().slices.slice_len = 0;
    assert!(matches!(
        validate_against_object(&w, &CAPS, &facts_for(&KERNELISH)),
        Err(ValidationError::EmptySlice)
    ));
}

#[test]
fn hostile_geometry_does_not_wrap() {
    // u32 arithmetic would wrap base + k*stride back into low RAM.
    let mut w = kernel_workload(&KERNELISH);
    let g = &mut w.mmu.as_mut().unwrap().slices;
    g.base = u32::MAX - MIB;
    g.stride = 8 * MIB;
    g.slice_len = 8 * MIB;
    g.ceiling = u32::MAX;
    assert!(matches!(
        validate_against_object(&w, &CAPS, &facts_for(&KERNELISH)),
        Err(ValidationError::SliceOutOfRange { .. })
    ));
}

#[test]
fn max_pids_outside_u8_pid_range_is_refused() {
    for max_pids in [0u32, 257, u32::MAX] {
        let mut w = kernel_workload(&KERNELISH);
        w.mmu.as_mut().unwrap().slices.max_pids = max_pids;
        assert_eq!(
            validate_against_object(&w, &CAPS, &facts_for(&KERNELISH)),
            Err(ValidationError::MaxPidsOutOfRange { max_pids })
        );
    }
}

// ── Gate B against the object ────────────────────────────────────────────────

#[test]
fn object_geometry_mismatch_is_refused() {
    let w = kernel_workload(&KERNELISH);
    let mut facts = facts_for(&KERNELISH);
    facts.mmu_geometry = Some(SliceGeometry {
        stride: 16 * MIB,
        slice_len: 16 * MIB,
        ..geometry()
    });
    assert!(matches!(
        validate_against_object(&w, &CAPS, &facts),
        Err(ValidationError::SliceGeometryMismatch { .. })
    ));
}

#[test]
fn object_without_geometry_is_refused_for_per_pid_mmu() {
    let w = kernel_workload(&KERNELISH);
    let mut facts = facts_for(&KERNELISH);
    facts.mmu_geometry = None;
    assert_eq!(
        validate_against_object(&w, &CAPS, &facts),
        Err(ValidationError::ObjectGeometryMissing)
    );
}

#[test]
fn per_pid_mmu_without_layout_is_refused() {
    let mut w = kernel_workload(&KERNELISH);
    w.mmu = None;
    assert!(matches!(
        validate_against_object(&w, &CAPS, &facts_for(&KERNELISH)),
        Err(ValidationError::MmuLayoutMismatch { .. })
    ));
}

#[test]
fn flat_memory_with_layout_is_refused() {
    let mut w = doom_workload();
    w.mmu = Some(MmuLayout {
        slices: geometry(),
        reserved: reserved(),
    });
    assert!(matches!(
        validate_against_object(&w, &CAPS, &facts_for(&DOOMISH)),
        Err(ValidationError::MmuLayoutMismatch { .. })
    ));
}

// ── (6) Doom (mmu: None) is unaffected by Gate B ─────────────────────────────

#[test]
fn doom_flat_workload_passes_with_or_without_object_geometry() {
    let w = doom_workload();
    let mut facts = facts_for(&DOOMISH);
    assert_eq!(validate_against_object(&w, &CAPS, &facts), Ok(()));
    facts.mmu_geometry = None;
    assert_eq!(validate_against_object(&w, &CAPS, &facts), Ok(()));
}

// ── (7) load(): the gates run before the first write; writes byte-identical ──

#[test]
fn load_with_matching_facts_writes_the_inline_sequence() {
    let w = kernel_workload(&KERNELISH);
    let mut sink = RecordingSink::default();
    load(&w, &CAPS, &facts_for(&KERNELISH), &mut sink).unwrap();
    assert_eq!(sink.rom, oracle(0x4000, &w.image.rom));
    assert_eq!(sink.rv2mbc, oracle(0, &w.image.rv2mbc));

    let d = doom_workload();
    let mut sink = RecordingSink::default();
    load(&d, &CAPS, &facts_for(&DOOMISH), &mut sink).unwrap();
    assert_eq!(sink.rom, oracle(0, &d.image.rom));
    assert_eq!(sink.rv2mbc, oracle(0, &d.image.rv2mbc));
    assert!(sink.frozen);
}

#[test]
fn load_refused_by_either_gate_writes_nothing() {
    let w = kernel_workload(&KERNELISH);
    let mut sink = RecordingSink::default();
    let err = load(&w, &CAPS, &facts_for(&DOOMISH), &mut sink).unwrap_err();
    assert!(matches!(
        err,
        LoadError::Invalid(ValidationError::SurfaceObjectMismatch { .. })
    ));
    assert!(sink.rom.is_empty() && sink.rv2mbc.is_empty() && !sink.frozen);

    let mut w = kernel_workload(&KERNELISH);
    w.mmu.as_mut().unwrap().slices.stride = 4 * MIB;
    let mut sink = RecordingSink::default();
    let err = load(&w, &CAPS, &facts_for(&KERNELISH), &mut sink).unwrap_err();
    assert!(matches!(
        err,
        LoadError::Invalid(ValidationError::SliceOverlap { .. })
    ));
    assert!(sink.rom.is_empty() && sink.rv2mbc.is_empty() && !sink.frozen);
}

// ── The surface id itself ────────────────────────────────────────────────────

#[test]
fn sha256_matches_fips_180_vectors() {
    fn hex(b: [u8; 32]) -> String {
        b.iter().map(|x| format!("{x:02x}")).collect()
    }
    assert_eq!(
        hex(sha256(b"")),
        "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
    );
    assert_eq!(
        hex(sha256(b"abc")),
        "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
    );
    assert_eq!(
        hex(sha256(
            b"abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq"
        )),
        "248d6a61d20638b8e5c026930c3e6039a33ce45964ff2167f6ecedd419db06c1"
    );
    assert_eq!(
        hex(sha256(&vec![b'a'; 1_000_000])),
        "cdc76e5c9914fb9281a1c7e284d73e67f1809a48a497200e046d39ccc7112cd0"
    );
}

#[test]
fn surface_id_ignores_order_and_repetition_but_not_content() {
    assert_eq!(surface_id(&[16, 1, 21]), surface_id(&[1, 16, 21, 16]));
    assert_ne!(surface_id(&[1, 16, 21]), surface_id(&[1, 16, 21, 4]));
    assert_ne!(surface_id(&[1, 16]), surface_id(&[1, 17]));
    assert_ne!(surface_id(&[]), sha256(b""));
}

// ── Reading the facts out of an ELF object ───────────────────────────────────

/// A minimal ELF64 little-endian relocatable with sections
/// [null, .rodata.upc, .symtab, .strtab, .shstrtab] and two symbols in
/// `.rodata.upc`: UPC_SURFACE_ID (32 bytes at offset 0) and, optionally,
/// UPC_MMU_GEOMETRY (5 x u32 at offset 32).
fn synthetic_elf(id: [u8; 32], geometry: Option<[u32; 5]>) -> Vec<u8> {
    let mut section = id.to_vec();
    if let Some(g) = geometry {
        for v in g {
            section.extend_from_slice(&v.to_le_bytes());
        }
    }
    let mut strtab = vec![0u8];
    let id_name = strtab.len() as u32;
    strtab.extend_from_slice(b"UPC_SURFACE_ID\0");
    let geo_name = strtab.len() as u32;
    strtab.extend_from_slice(b"UPC_MMU_GEOMETRY\0");
    let shstrtab: &[u8] = b"\0.rodata.upc\0.symtab\0.strtab\0.shstrtab\0";
    let (n_rodata, n_symtab, n_strtab, n_shstrtab) = (1u32, 13u32, 21u32, 29u32);

    let sym = |name: u32, shndx: u16, value: u64, size: u64| -> Vec<u8> {
        let mut s = Vec::new();
        s.extend_from_slice(&name.to_le_bytes());
        s.push(0x11); // STB_GLOBAL, STT_OBJECT
        s.push(0);
        s.extend_from_slice(&shndx.to_le_bytes());
        s.extend_from_slice(&value.to_le_bytes());
        s.extend_from_slice(&size.to_le_bytes());
        s
    };
    let mut symtab = vec![0u8; 24];
    symtab.extend(sym(id_name, 1, 0, 32));
    if geometry.is_some() {
        symtab.extend(sym(geo_name, 1, 32, 20));
    }

    let mut out = vec![0u8; 64];
    let off_rodata = out.len() as u64;
    out.extend_from_slice(&section);
    let off_symtab = out.len() as u64;
    out.extend_from_slice(&symtab);
    let off_strtab = out.len() as u64;
    out.extend_from_slice(&strtab);
    let off_shstrtab = out.len() as u64;
    out.extend_from_slice(shstrtab);
    while !out.len().is_multiple_of(8) {
        out.push(0);
    }
    let shoff = out.len() as u64;

    let sh = |name: u32, ty: u32, off: u64, size: u64, link: u32, entsize: u64| -> Vec<u8> {
        let mut s = Vec::new();
        s.extend_from_slice(&name.to_le_bytes());
        s.extend_from_slice(&ty.to_le_bytes());
        s.extend_from_slice(&0u64.to_le_bytes()); // flags
        s.extend_from_slice(&0u64.to_le_bytes()); // addr
        s.extend_from_slice(&off.to_le_bytes());
        s.extend_from_slice(&size.to_le_bytes());
        s.extend_from_slice(&link.to_le_bytes());
        s.extend_from_slice(&0u32.to_le_bytes()); // info
        s.extend_from_slice(&1u64.to_le_bytes()); // addralign
        s.extend_from_slice(&entsize.to_le_bytes());
        s
    };
    out.extend(vec![0u8; 64]); // SHN_UNDEF
    out.extend(sh(n_rodata, 1, off_rodata, section.len() as u64, 0, 0));
    out.extend(sh(n_symtab, 2, off_symtab, symtab.len() as u64, 3, 24));
    out.extend(sh(n_strtab, 3, off_strtab, strtab.len() as u64, 0, 0));
    out.extend(sh(n_shstrtab, 3, off_shstrtab, shstrtab.len() as u64, 0, 0));

    out[0..4].copy_from_slice(b"\x7fELF");
    out[4] = 2; // ELFCLASS64
    out[5] = 1; // ELFDATA2LSB
    out[6] = 1; // EV_CURRENT
    out[16..18].copy_from_slice(&1u16.to_le_bytes()); // ET_REL
    out[18..20].copy_from_slice(&247u16.to_le_bytes()); // EM_BPF
    out[20..24].copy_from_slice(&1u32.to_le_bytes());
    out[40..48].copy_from_slice(&shoff.to_le_bytes());
    out[52..54].copy_from_slice(&64u16.to_le_bytes()); // e_ehsize
    out[58..60].copy_from_slice(&64u16.to_le_bytes()); // e_shentsize
    out[60..62].copy_from_slice(&5u16.to_le_bytes()); // e_shnum
    out[62..64].copy_from_slice(&4u16.to_le_bytes()); // e_shstrndx
    out
}

#[test]
fn object_facts_read_from_elf() {
    let id = surface_id(&[1, 16, 21]);
    let g = [16 * MIB, 8 * MIB, 8 * MIB, 56 * MIB, 8];
    let facts = ObjectFacts::from_elf(&synthetic_elf(id, Some(g))).unwrap();
    assert_eq!(facts.surface_id, id);
    assert_eq!(facts.mmu_geometry, Some(geometry()));

    let facts = ObjectFacts::from_elf(&synthetic_elf(id, None)).unwrap();
    assert_eq!(facts.surface_id, id);
    assert_eq!(facts.mmu_geometry, None);
}

#[test]
fn object_facts_refuse_non_elf_and_truncation() {
    assert_eq!(
        ObjectFacts::from_elf(b"not an elf at all, just bytes"),
        Err(ObjectFactsError::NotElf64Le)
    );
    let elf = synthetic_elf(surface_id(&[1]), None);
    for cut in [0usize, 10, 63, 64, 100, elf.len() - 1] {
        assert!(
            ObjectFacts::from_elf(&elf[..cut]).is_err(),
            "truncated at {cut} parsed"
        );
    }
}

#[test]
fn object_without_surface_id_is_refused() {
    // Rename the symbol so the section exists but UPC_SURFACE_ID does not:
    // an object built before the gate existed.
    let mut elf = synthetic_elf(surface_id(&[1]), None);
    let pos = elf
        .windows(14)
        .position(|w| w == b"UPC_SURFACE_ID")
        .unwrap();
    elf[pos] = b'X';
    assert_eq!(
        ObjectFacts::from_elf(&elf),
        Err(ObjectFactsError::MissingSymbol("UPC_SURFACE_ID"))
    );
}

#[test]
fn object_with_hostile_symbol_bounds_is_refused() {
    // A symbol whose value+size runs past its section must not be read.
    let mut elf = synthetic_elf(surface_id(&[1]), None);
    let sym_off = 64 + 32 + 24; // header, section data, null symbol
    elf[sym_off + 8..sym_off + 16].copy_from_slice(&u64::MAX.to_le_bytes());
    assert!(ObjectFacts::from_elf(&elf).is_err());
}
