// SPDX-License-Identifier: GPL-3.0-or-later
#![no_main]

use libfuzzer_sys::fuzz_target;
use monad_mbc::instruction::decode_checked;

fuzz_target!(|data: &[u8]| {
    // Feed every 4-byte chunk as a u32 instruction word to decode_checked.
    // It must never panic -- only return Ok or Err.
    if data.len() < 4 {
        return;
    }
    for chunk in data.as_chunks::<4>().0.iter() {
        let word = u32::from_le_bytes(*chunk);
        let _ = decode_checked(word);
    }
});
