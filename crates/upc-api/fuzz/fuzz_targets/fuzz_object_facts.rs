// SPDX-License-Identifier: GPL-3.0-or-later
//! S-ASCEND-2 Step 294: the loader reads UPC_SURFACE_ID and UPC_MMU_GEOMETRY
//! out of an eBPF object before trusting it. Arbitrary bytes must give Ok or
//! Err, never a panic, and an Ok always carries a whole 32-byte surface id.
#![no_main]
use libfuzzer_sys::fuzz_target;

fuzz_target!(|data: &[u8]| {
    if let Ok(facts) = upc_api::ObjectFacts::from_elf(data) {
        assert_eq!(facts.surface_id.len(), 32);
    }
});
