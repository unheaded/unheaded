// SPDX-License-Identifier: GPL-3.0-or-later
#![no_main]
//! S5 Phase 17 Step 420 (Lich hook): the fuzz input is the entropy for a
//! random validated XDP-stage cartridge and its run input (every FIELD value
//! and the clock). The cartridge runs in lockstep on the shared step
//! (`monad_common::cart`, the code `cart_xdp` runs) and on the host MBC
//! emulator (`monad-mbc` execute.rs); registers, flags, PC and the flow
//! window are compared after every instruction. Any divergence is a finding:
//! the panic message names the step, the opcode and what differs, and the
//! crashing input reproduces it.

use libfuzzer_sys::fuzz_target;
use upc_cartridge::diff::{gen_input, gen_xdp_cartridge, lockstep, Bytes};
use upc_cartridge::{validate, Image};

fuzz_target!(|data: &[u8]| {
    let mut e = Bytes::new(data);
    let img = gen_xdp_cartridge(&mut e);
    let input = gen_input(&mut e);
    let raw = img.encode();
    assert_eq!(
        Image::decode(&raw).as_ref(),
        Ok(&img),
        "encode/decode round trip"
    );
    validate(&img).expect("generator produced an image the validator rejects");
    if let Err(d) = lockstep(&img, &input) {
        panic!("{d}\nimage {raw:02x?}\ninput {input:?}");
    }
});
