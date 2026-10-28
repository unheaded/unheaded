// SPDX-License-Identifier: GPL-3.0-or-later
//! S-ASCEND-2 Steps 157-161: upc-bootctl freezes ROM_MAP and RV2MBC_MAP once,
//! after the boot's last write to either and before XDP attach.
//!
//! bootctl writes the kernel image and its `.rv2mbc` outside `upc_api` and the
//! init and resident images through `upc_api::write_image`, so no single
//! `upc_api::load` call is last. The freeze is `upc_api::finalize(&mut runner)`
//! in `boot_prepare`. This test pins its place in the source: after every
//! ROM/RV2MBC write, before the attach. A write moved after the freeze would
//! fail live with EPERM; this catches it without root.

use std::path::PathBuf;

fn main_rs() -> String {
    let p = PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("src/main.rs");
    std::fs::read_to_string(&p).unwrap_or_else(|e| panic!("read {}: {e}", p.display()))
}

/// The text of `fn <name>(` up to the next top-level item.
fn fn_body<'a>(src: &'a str, name: &str) -> &'a str {
    let start = src
        .find(&format!("\nfn {name}("))
        .unwrap_or_else(|| panic!("fn {name} not in main.rs"));
    let rest = &src[start + 1..];
    let end = rest[1..]
        .find("\n}\n")
        .map(|i| i + 1 + 3)
        .unwrap_or(rest.len());
    &rest[..end]
}

/// Code lines only: `//` comments dropped, so prose naming a call does not count.
fn code(body: &str) -> String {
    body.lines()
        .map(|l| match l.find("//") {
            Some(i) => &l[..i],
            None => l,
        })
        .collect::<Vec<_>>()
        .join("\n")
}

const WRITES: [&str; 5] = [
    "populate_rom(",
    "populate_rom_at(",
    "populate_rv2mbc(",
    "upc_api::write_image(",
    "load_resident_program(",
];
const FREEZE: &str = "upc_api::finalize(&mut runner)";

#[test]
fn boot_prepare_freezes_after_the_last_image_write_and_before_attach() {
    let src = main_rs();
    let body = code(fn_body(&src, "boot_prepare"));
    let last_write = WRITES
        .iter()
        .filter_map(|w| body.rfind(w).map(|i| (i, *w)))
        .max()
        .expect("boot_prepare writes no image");
    assert_eq!(
        body.matches(FREEZE).count(),
        1,
        "boot_prepare must freeze exactly once ({FREEZE})"
    );
    let freeze = body.find(FREEZE).unwrap();
    let attach = body
        .find("attach_xdp(")
        .expect("no attach_xdp in boot_prepare");
    assert!(
        last_write.0 < freeze,
        "{} comes after the freeze",
        last_write.1
    );
    assert!(freeze < attach, "the freeze must come before attach_xdp");
}

#[test]
fn no_image_load_in_bootctl_freezes_early() {
    // upc_api::load finalizes; in bootctl a later image write would then fail
    // EPERM. Every per-image load goes through write_image.
    let src = code(&main_rs());
    assert!(
        !src.contains("upc_api::load("),
        "use upc_api::write_image in bootctl: upc_api::load freezes the maps"
    );
    let resident = code(fn_body(&main_rs(), "load_resident_program"));
    assert!(resident.contains("upc_api::write_image("));
}
