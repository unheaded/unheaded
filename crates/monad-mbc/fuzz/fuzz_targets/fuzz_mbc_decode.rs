#![no_main]

use libfuzzer_sys::fuzz_target;
use monad_mbc::instruction::decode_checked;

fuzz_target!(|data: &[u8]| {
    // Feed every 4-byte chunk as a u32 instruction word to decode_checked.
    // It must never panic -- only return Ok or Err.
    if data.len() < 4 {
        return;
    }
    for chunk in data.chunks_exact(4) {
        let word = u32::from_le_bytes([chunk[0], chunk[1], chunk[2], chunk[3]]);
        let _ = decode_checked(word);
    }
});
