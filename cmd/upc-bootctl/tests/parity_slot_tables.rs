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

const NAMES: [&str; 10] = [
    "UPC_SLOT_TABLES_BP_BYTE",
    "UPC_SLOTS",
    "UPC_TF_STRIDE",
    "UPC_ST_CUR_SLOT_OFF",
    "UPC_ST_TRAP_A7_OFF",
    "UPC_ST_FRESH_MASK_OFF",
    "UPC_ST_SIZE",
    "UPC_TAG_SLOT",
    "UPC_TAG_A7",
    "UPC_TAG_MASK",
];

#[test]
fn bpf_and_kernel_agree_on_the_slot_tables() {
    let rs = read("ebpf/monad-cpu-ebpf/src/phase12.rs");
    let h = read("crates/xv6-mbc/adapters/upc_slots.h");
    for n in NAMES {
        assert_eq!(rust_const(&rs, n), c_define(&h, n), "{n} differs");
    }
}

/// xv6's trapframe under ilp32e: 36 fields of 4 bytes (`uint64` is 4 bytes).
fn trapframe_bytes() -> u32 {
    let h = read("crates/xv6-mbc/upstream/kernel/proc.h");
    let body = h
        .split("struct trapframe {")
        .nth(1)
        .and_then(|b| b.split("};").next())
        .expect("struct trapframe in proc.h");
    let fields = body
        .lines()
        .filter(|l| l.contains("uint32 ") || l.contains("uint64 "))
        .count() as u32;
    4 * fields
}

#[test]
fn layout_is_the_adr_layout() {
    let rs = read("ebpf/monad-cpu-ebpf/src/phase12.rs");
    let slots = rust_const(&rs, "UPC_SLOTS");
    let stride = rust_const(&rs, "UPC_TF_STRIDE");
    assert_eq!(trapframe_bytes(), 144, "struct trapframe changed size");
    assert!(trapframe_bytes() <= stride);
    assert!(
        stride.is_power_of_two(),
        "BPF computes base + (s << log2 stride)"
    );
    assert_eq!(rust_const(&rs, "UPC_ST_CUR_SLOT_OFF"), slots * stride);
    assert_eq!(rust_const(&rs, "UPC_ST_TRAP_A7_OFF"), slots * stride + 4);
    assert_eq!(rust_const(&rs, "UPC_ST_FRESH_MASK_OFF"), slots * stride + 8);
    assert_eq!(rust_const(&rs, "UPC_ST_SIZE"), slots * stride + 16);
    let mask = rust_const(&rs, "UPC_TAG_MASK");
    assert_eq!(rust_const(&rs, "UPC_TAG_SLOT") & !mask, 0);
    assert_eq!(rust_const(&rs, "UPC_TAG_A7") & !mask, 0);
    assert_ne!(
        rust_const(&rs, "UPC_TAG_SLOT"),
        rust_const(&rs, "UPC_TAG_A7")
    );
    // BootParams v3 sits at 0x100; `reserved` starts at byte offset 96.
    assert_eq!(rust_const(&rs, "UPC_SLOT_TABLES_BP_BYTE"), 0x100 + 96);
}
