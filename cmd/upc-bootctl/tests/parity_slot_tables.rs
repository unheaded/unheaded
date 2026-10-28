// SPDX-License-Identifier: GPL-3.0-or-later
//! ADR-109: the slot-table constants the BPF interpreter uses
//! (ebpf/monad-cpu-ebpf/src/phase12.rs) must equal the kernel's
//! (crates/xv6-mbc/adapters/upc_slots.h), and the BootParams word that carries
//! the table address must be the first word of BootParams v3's `reserved`.

use std::path::PathBuf;

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

fn rust_const(src: &str, name: &str) -> u32 {
    let l = src
        .lines()
        .find(|l| {
            l.trim_start()
                .starts_with(&format!("pub const {name}: u32 = "))
        })
        .unwrap_or_else(|| panic!("{name} not in phase12.rs"));
    parse(l.split('=').nth(1).unwrap().split("//").next().unwrap())
}

fn c_define(src: &str, name: &str) -> u32 {
    let l = src
        .lines()
        .find(|l| l.split_whitespace().take(2).eq(["#define", name]))
        .unwrap_or_else(|| panic!("{name} not in upc_slots.h"));
    parse(l.split_whitespace().nth(2).unwrap())
}

const NAMES: [&str; 7] = [
    "UPC_SLOT_TABLES_BP_BYTE",
    "UPC_SLOTS",
    "UPC_ST_TF_OFF",
    "UPC_ST_CUR_SLOT_OFF",
    "UPC_ST_GEN_OFF",
    "UPC_ST_TRAP_A7_OFF",
    "UPC_ST_TRAP_A6_OFF",
];

#[test]
fn bpf_and_kernel_agree_on_the_slot_tables() {
    let rs = read("ebpf/monad-cpu-ebpf/src/phase12.rs");
    let h = read("crates/xv6-mbc/adapters/upc_slots.h");
    for n in NAMES {
        assert_eq!(rust_const(&rs, n), c_define(&h, n), "{n} differs");
    }
}

#[test]
fn layout_is_the_adr_layout() {
    let rs = read("ebpf/monad-cpu-ebpf/src/phase12.rs");
    let slots = rust_const(&rs, "UPC_SLOTS");
    assert_eq!(rust_const(&rs, "UPC_ST_CUR_SLOT_OFF"), 4 * slots);
    assert_eq!(rust_const(&rs, "UPC_ST_GEN_OFF"), 4 * slots + 4);
    assert_eq!(rust_const(&rs, "UPC_ST_TRAP_A7_OFF"), 8 * slots + 4);
    assert_eq!(rust_const(&rs, "UPC_ST_TRAP_A6_OFF"), 8 * slots + 8);
    // BootParams v3 sits at 0x100; `reserved` starts at byte offset 96.
    assert_eq!(rust_const(&rs, "UPC_SLOT_TABLES_BP_BYTE"), 0x100 + 96);
}
