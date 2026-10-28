// SPDX-License-Identifier: GPL-3.0-or-later
//! The freeze step of the image load (S-ASCEND-2 Steps 157-161, ADR-080 §7).
//!
//! `ROM_MAP` and `RV2MBC_MAP` are frozen (`BPF_MAP_FREEZE`) once, after the
//! boot's last write to either and after the gates, before XDP attach. The aya
//! sinks do the freeze in `ImageSink::finalize`; these tests pin the order on a
//! recording sink: writes, then exactly one `Freeze`, and a failed freeze stops
//! the boot before attach.
//!
//! `load()` is the single-image path (doom-runner): gates, writes, finalize.
//! `write_image()` is the same minus finalize, for a loader that writes several
//! images (upc-bootctl: kernel, init, resident programs); it calls `finalize()`
//! once after the last of them.

use upc_api::surface_hash::surface_id;
use upc_api::{
    finalize, load, write_image, BootProtocol, FeatureGate, GuestImage, ImageSink, LoadError,
    MapCapacities, MbcPc, MemoryModel, ObjectFacts, SyscallDesc, SyscallSurface, ValidationError,
    Workload,
};

const CAPS: MapCapacities = MapCapacities {
    rom_words: 0x10000,
    rv2mbc_entries: 0x10000,
    ram_words: 0x10000,
};

static SURFACE: [SyscallDesc; 2] = [
    SyscallDesc {
        number: 0,
        name: "DRAW_FRAME",
    },
    SyscallDesc {
        number: 1,
        name: "GET_KEY",
    },
];

fn facts() -> ObjectFacts {
    ObjectFacts {
        surface_id: surface_id(&[0, 1]),
        mmu_geometry: None,
    }
}

fn workload(rom_start_slot: u32, rv2mbc_word_base: u32) -> Workload {
    Workload {
        name: "freeze-test".to_string(),
        image: GuestImage {
            rom: vec![0x2700_0000, 0xDEAD_BEEF, 0x0000_0001],
            rom_start_slot,
            rv2mbc: vec![rom_start_slot, rom_start_slot + 1],
            rv2mbc_word_base,
            expected_rv2mbc_sha256: None,
            entry: MbcPc(rom_start_slot),
        },
        memory: MemoryModel::Flat,
        surface: SyscallSurface {
            name: "freeze-test",
            feature_gate: FeatureGate::Default,
            syscalls: &SURFACE,
        },
        boot: BootProtocol::Direct,
        mmu: None,
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
enum Event {
    Rom { start: u32, words: usize },
    Rv2mbc { base: u32, entries: usize },
    Freeze,
    Attach,
}

/// Records every call in order. `fail_finalize` makes the freeze fail the way a
/// kernel without BPF_MAP_FREEZE or a missing CAP_BPF would.
#[derive(Default)]
struct EventSink {
    events: Vec<Event>,
    fail_finalize: bool,
    fail_rom: bool,
}

impl ImageSink for EventSink {
    type Error = &'static str;

    fn write_rom(&mut self, start_slot: u32, words: &[u32]) -> Result<(), Self::Error> {
        if self.fail_rom {
            return Err("ROM_MAP write failed");
        }
        self.events.push(Event::Rom {
            start: start_slot,
            words: words.len(),
        });
        Ok(())
    }

    fn write_rv2mbc(&mut self, base: u32, entries: &[u32]) -> Result<(), Self::Error> {
        self.events.push(Event::Rv2mbc {
            base,
            entries: entries.len(),
        });
        Ok(())
    }

    fn finalize(&mut self) -> Result<(), Self::Error> {
        if self.fail_finalize {
            return Err("BPF_MAP_FREEZE: EPERM");
        }
        self.events.push(Event::Freeze);
        Ok(())
    }
}

impl EventSink {
    fn freezes(&self) -> usize {
        self.events.iter().filter(|e| **e == Event::Freeze).count()
    }

    /// The loader's attach step: only reached when the load succeeded.
    fn attach(&mut self) {
        self.events.push(Event::Attach);
    }
}

#[test]
fn load_freezes_exactly_once_after_the_last_write() {
    let w = workload(0, 0);
    let mut sink = EventSink::default();
    load(&w, &CAPS, &facts(), &mut sink).unwrap();
    assert_eq!(
        sink.events,
        vec![
            Event::Rom { start: 0, words: 3 },
            Event::Rv2mbc {
                base: 0,
                entries: 2
            },
            Event::Freeze,
        ]
    );
}

#[test]
fn write_image_does_not_freeze() {
    let w = workload(0x4000, 0);
    let mut sink = EventSink::default();
    write_image(&w, &CAPS, &facts(), &mut sink).unwrap();
    assert_eq!(sink.freezes(), 0, "events: {:?}", sink.events);
    assert_eq!(sink.events.len(), 2);
}

#[test]
fn several_images_then_finalize_freeze_once_after_all_writes() {
    // upc-bootctl's shape: init, then two resident programs, then one freeze.
    let mut sink = EventSink::default();
    for (rom, rv) in [(0x4000, 0), (0x6000, 0x1000), (0x9000, 0x2000)] {
        write_image(&workload(rom, rv), &CAPS, &facts(), &mut sink).unwrap();
    }
    finalize(&mut sink).unwrap();
    assert_eq!(sink.freezes(), 1);
    assert_eq!(sink.events.last(), Some(&Event::Freeze));
    assert_eq!(sink.events.len(), 7);
}

#[test]
fn failed_freeze_fails_load_and_nothing_attaches() {
    let w = workload(0, 0);
    let mut sink = EventSink {
        fail_finalize: true,
        ..EventSink::default()
    };
    let res = load(&w, &CAPS, &facts(), &mut sink);
    assert_eq!(res, Err(LoadError::Finalize("BPF_MAP_FREEZE: EPERM")));
    if res.is_ok() {
        sink.attach();
    }
    assert!(!sink.events.contains(&Event::Attach));
    assert_eq!(sink.freezes(), 0);
}

#[test]
fn failed_finalize_after_several_images_is_a_finalize_error() {
    let mut sink = EventSink {
        fail_finalize: true,
        ..EventSink::default()
    };
    write_image(&workload(0x4000, 0), &CAPS, &facts(), &mut sink).unwrap();
    assert_eq!(
        finalize(&mut sink),
        Err(LoadError::Finalize("BPF_MAP_FREEZE: EPERM"))
    );
}

#[test]
fn freeze_error_message_names_the_freeze() {
    let e: LoadError<&str> = LoadError::Finalize("EPERM");
    let s = e.to_string();
    assert!(s.contains("freeze"), "{s}");
    assert!(s.contains("EPERM"), "{s}");
}

#[test]
fn refused_workload_is_never_frozen() {
    // The gates run before the writes and the freeze: a refusal leaves the maps
    // empty and writable (the loader drops the object).
    let mut w = workload(0, 0);
    w.image.rom.clear();
    let mut sink = EventSink::default();
    let err = load(&w, &CAPS, &facts(), &mut sink).unwrap_err();
    assert_eq!(err, LoadError::Invalid(ValidationError::EmptyRom));
    assert!(sink.events.is_empty(), "events: {:?}", sink.events);

    let w = workload(0, 0);
    let wrong = ObjectFacts {
        surface_id: surface_id(&[0, 1, 2]),
        mmu_geometry: None,
    };
    let mut sink = EventSink::default();
    let err = load(&w, &CAPS, &wrong, &mut sink).unwrap_err();
    assert!(matches!(
        err,
        LoadError::Invalid(ValidationError::SurfaceObjectMismatch { .. })
    ));
    assert!(sink.events.is_empty(), "events: {:?}", sink.events);
}

#[test]
fn failed_write_is_never_frozen() {
    let w = workload(0, 0);
    let mut sink = EventSink {
        fail_rom: true,
        ..EventSink::default()
    };
    let err = load(&w, &CAPS, &facts(), &mut sink).unwrap_err();
    assert_eq!(err, LoadError::Sink("ROM_MAP write failed"));
    assert_eq!(sink.freezes(), 0);
}

#[test]
fn successful_load_then_attach_orders_freeze_before_attach() {
    let w = workload(0, 0);
    let mut sink = EventSink::default();
    if load(&w, &CAPS, &facts(), &mut sink).is_ok() {
        sink.attach();
    }
    let n = sink.events.len();
    assert_eq!(&sink.events[n - 2..], &[Event::Freeze, Event::Attach]);
}
