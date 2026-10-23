// SPDX-License-Identifier: GPL-3.0-or-later
#![no_main]
//! S5 Phase 17 Step 421 (Lich hook): arbitrary bytes through the loader's
//! checks, `Image::decode` then `validate`. Either may refuse; neither may
//! panic. An image both accept is run once (XDP: one packet with zero
//! inputs; tick: one job, at most 8 calls) on the reference runner, which
//! must not panic either: a bad cartridge is a `Fault` value.
//!
//! Seeds: `tomb/lich/seeds/cartridge/` (the shipped cartridges plus
//! mutations).

use libfuzzer_sys::fuzz_target;
use monad_common::cart::{CartState, STAGE_XDP};
use sha2::{Digest, Sha256};
use upc_cartridge::reference::{run_packet, run_tick_job, PacketInput};
use upc_cartridge::{state_hash, validate, Image, HEADER_LEN};

fuzz_target!(|data: &[u8]| {
    check(data);
    // The embedded sha256 rejects almost every mutation at decode. Second
    // pass: the same bytes with the sha recomputed, so the header, size,
    // validator and runner paths see mutated content too.
    if data.len() >= HEADER_LEN {
        let mut b = data.to_vec();
        let mut h = Sha256::new();
        h.update(&b[..20]);
        h.update(&b[HEADER_LEN..]);
        let sha: [u8; 32] = h.finalize().into();
        b[20..HEADER_LEN].copy_from_slice(&sha);
        check(&b);
    }
});

fn check(data: &[u8]) {
    let Ok(img) = Image::decode(data) else {
        return;
    };
    // Decoding is canonical: re-encoding gives back the same bytes.
    assert_eq!(img.encode(), data, "decode accepted a non-canonical image");
    if validate(&img).is_err() {
        return;
    }
    if img.stage == STAGE_XDP {
        let mut st = CartState::zeroed();
        let _ = run_packet(&img, &mut st, &PacketInput::default());
        let _ = state_hash(&st);
    } else {
        let (st, calls) = run_tick_job(&img, &[0; 8], 0, 8);
        assert!(!calls.is_empty() && calls.len() <= 8);
        let _ = state_hash(&st);
    }
}
