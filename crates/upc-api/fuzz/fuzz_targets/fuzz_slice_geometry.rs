// SPDX-License-Identifier: GPL-3.0-or-later
//! S-ASCEND-2 Step 294: the per-pid slice geometry gate. Any declared geometry,
//! RAM capacity and reserved region must give Ok or a ValidationError, never an
//! arithmetic panic, and an Ok must mean the slices are in RAM and disjoint.
#![no_main]
use libfuzzer_sys::fuzz_target;
use upc_api::{
    check_mmu, MapCapacities, MemRegion, MemoryModel, MmuLayout, ObjectFacts, SliceGeometry,
};

fn word(d: &[u8], i: usize) -> u32 {
    let mut b = [0u8; 4];
    for (k, x) in b.iter_mut().enumerate() {
        *x = *d.get(i * 4 + k).unwrap_or(&0);
    }
    u32::from_le_bytes(b)
}

fuzz_target!(|data: &[u8]| {
    let g = SliceGeometry::from_words([
        word(data, 0),
        word(data, 1),
        word(data, 2),
        word(data, 3),
        word(data, 4),
    ]);
    let caps = MapCapacities {
        rom_words: word(data, 5),
        rv2mbc_entries: word(data, 6),
        ram_words: word(data, 7),
    };
    let layout = MmuLayout {
        slices: g,
        reserved: vec![MemRegion {
            name: "fuzz",
            start: word(data, 8),
            len: word(data, 9),
        }],
    };
    let facts = ObjectFacts {
        surface_id: [0u8; 32],
        mmu_geometry: Some(g),
    };
    if check_mmu(MemoryModel::PerPidMmu, Some(&layout), &caps, &facts).is_ok() {
        let ram = caps.ram_words as u64 * 4;
        let s = g.admitted_slices();
        for (i, &(_, _start, end)) in s.iter().enumerate() {
            assert!(end <= ram, "accepted slice past RAM_MAP");
            if let Some(&(_, next, _)) = s.get(i + 1) {
                assert!(end <= next, "accepted overlapping slices");
            }
        }
    }
});
