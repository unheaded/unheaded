// SPDX-License-Identifier: GPL-3.0-or-later
#![no_main]

use libfuzzer_sys::fuzz_target;

// Layout of fuzzer input: 64 bytes of initial registers (16 x u32 LE), then
// ROM instruction words (u32 LE). The CPU must never panic regardless of
// input. The body lives in the crate (`execute::run_fuzz_input`) so the
// seeded `cargo test` in `tests/seeded_execute.rs` runs the same code.
fuzz_target!(|data: &[u8]| {
    monad_mbc::execute::run_fuzz_input(data);
});
