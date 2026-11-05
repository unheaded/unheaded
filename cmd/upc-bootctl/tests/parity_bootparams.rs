// SPDX-License-Identifier: GPL-3.0-or-later
//! The frozen-ABI check for Boot Protocol v2/v3 (ADR-067 Decision 7, DP-414).
//! `BootParamsV2` (src/bootparams.rs) and its two C mirrors, `start_mbc.c` and
//! the bootstub's `start.c`, must name the same fields in the same order, with
//! the SHA and `reserved` at their frozen offsets; and the RV text base the
//! loader sends must be the one kernel-mbc.ld links.

use std::path::PathBuf;

const RUST_SRC: &str = "cmd/upc-bootctl/src/bootparams.rs";
const C_MIRRORS: [&str; 2] = [
    "crates/xv6-mbc/adapters/start_mbc.c",
    "crates/upc-bootstub/src/start.c",
];

fn read(rel: &str) -> String {
    let p = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../..")
        .join(rel);
    std::fs::read_to_string(&p).unwrap_or_else(|e| panic!("read {}: {e}", p.display()))
}

fn parse(v: &str) -> u32 {
    let v = v.trim().trim_end_matches(';').replace('_', "");
    match v.strip_prefix("0x") {
        Some(h) => u32::from_str_radix(h, 16).unwrap(),
        None => v.parse().unwrap(),
    }
}

fn rust_const(src: &str, name: &str, ty: &str) -> u32 {
    let l = src
        .lines()
        .find(|l| {
            l.trim_start()
                .starts_with(&format!("pub const {name}: {ty} = "))
        })
        .unwrap_or_else(|| panic!("{name} not in bootparams.rs"));
    parse(l.split('=').nth(1).unwrap().split("//").next().unwrap())
}

/// (field name, size in bytes), in declaration order.
fn rust_fields(src: &str) -> Vec<(String, u32)> {
    let body = src
        .split("pub struct BootParamsV2 {")
        .nth(1)
        .and_then(|b| b.split("\n}").next())
        .expect("struct BootParamsV2 in bootparams.rs");
    body.lines()
        .filter_map(|l| l.trim_start().strip_prefix("pub "))
        .map(|l| {
            let (name, rest) = l.split_once(':').expect("a pub field");
            let size = match rest.split(',').next().unwrap().trim() {
                "u32" => 4,
                "[u8; 32]" => 32,
                "[u32; 40]" => 160,
                t => panic!("{name}: type {t} is new to BootParams"),
            };
            (name.to_string(), size)
        })
        .collect()
}

/// (field name, size in bytes), in declaration order.
fn c_fields(src: &str) -> Vec<(String, u32)> {
    let body = src
        .split("struct boot_params_v2 {")
        .nth(1)
        .and_then(|b| b.split("};").next())
        .expect("struct boot_params_v2 in a C mirror");
    body.lines()
        .filter_map(|l| l.split_once(';'))
        .map(|(decl, _)| {
            let mut it = decl.split_whitespace();
            let (ty, field) = (it.next().unwrap(), it.next().unwrap());
            let (name, count) = match field.split_once('[') {
                Some((n, c)) => (n, parse(c.trim_end_matches(']'))),
                None => (field, 1),
            };
            let size = match ty {
                "uint32" | "uint32_t" => 4,
                "uint8" | "uint8_t" => 1,
                t => panic!("{name}: type {t} is new to BootParams"),
            };
            (name.to_string(), size * count)
        })
        .collect()
}

fn offset_of(fields: &[(String, u32)], name: &str) -> u32 {
    let i = fields
        .iter()
        .position(|(n, _)| n == name)
        .unwrap_or_else(|| panic!("{name} missing"));
    fields[..i].iter().map(|(_, s)| s).sum()
}

#[test]
fn field_order_agrees_in_all_three_definitions() {
    let names = |f: &[(String, u32)]| f.iter().map(|(n, _)| n.clone()).collect::<Vec<_>>();
    let rust = names(&rust_fields(&read(RUST_SRC)));
    assert_eq!(rust.len(), 18, "BootParams gained or lost a field");
    for c in C_MIRRORS {
        assert_eq!(names(&c_fields(&read(c))), rust, "{c} differs");
    }
}

#[test]
fn frozen_offsets_hold_in_all_three_definitions() {
    let mut all = vec![(RUST_SRC, rust_fields(&read(RUST_SRC)))];
    for c in C_MIRRORS {
        all.push((c, c_fields(&read(c))));
    }
    for (src, f) in all {
        assert_eq!(offset_of(&f, "rv2mbc_sha256"), 64, "{src}");
        assert_eq!(offset_of(&f, "reserved"), 96, "{src}");
        assert_eq!(f.iter().map(|(_, s)| s).sum::<u32>(), 256, "{src}");
    }
}

#[test]
fn rv_text_base_word_sits_at_byte_0x164() {
    let idx = rust_const(&read(RUST_SRC), "RV_TEXT_BASE_RESERVED_IDX", "usize");
    assert_eq!(idx, 1, "reserved[0] is the kernel's slot-table word");
    assert_eq!(0x100 + 96 + 4 * idx, 0x164);
}

#[test]
fn rv_text_base_is_the_linked_text_base() {
    let base = rust_const(&read(RUST_SRC), "XV6_TEXT_RV_WORD_BASE", "u32");
    assert_eq!(base, 0x8000);
    let ld = read("crates/xv6-mbc/adapters/kernel-mbc.ld");
    let text = ld
        .lines()
        .find_map(|l| l.trim_start().strip_prefix(".text "))
        .expect(".text in kernel-mbc.ld");
    let addr = parse(text.split_whitespace().next().unwrap());
    assert_eq!(addr, 0x0002_0000, "kernel-mbc.ld moved .text");
    assert_eq!(addr >> 2, base);
}
