// SPDX-License-Identifier: GPL-3.0-or-later
//! The loader's copy of the xv6 slice geometry (src/geometry.rs) must equal
//! the constants the code that maps the slices actually uses (S-ASCEND-2 Step
//! 106). The eBPF crate cannot be imported on the host, so this reads the
//! sources. At boot the gate also compares against the object's
//! UPC_MMU_GEOMETRY; this test catches the drift before a boot does.

#![allow(dead_code)]

#[path = "../src/geometry.rs"]
mod geometry;

use std::path::PathBuf;

fn repo() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../..")
}

fn read(rel: &str) -> String {
    std::fs::read_to_string(repo().join(rel)).unwrap_or_else(|e| panic!("read {rel}: {e}"))
}

/// The value of `const NAME: u32 = <hex>;` (pub or not) in Rust source.
fn rust_const(src: &str, name: &str) -> u32 {
    let line = src
        .lines()
        .find(|l| {
            let t = l.trim_start();
            let t = t.strip_prefix("pub ").unwrap_or(t);
            t.starts_with(&format!("const {name}: "))
        })
        .unwrap_or_else(|| panic!("const {name} not found"));
    let rhs = line.split('=').nth(1).unwrap().split(';').next().unwrap();
    parse_num(rhs)
}

fn parse_num(s: &str) -> u32 {
    let t: String = s.trim().chars().filter(|c| *c != '_').collect();
    let t = t.trim_end_matches("UL").trim_end_matches('u');
    match t.strip_prefix("0x") {
        Some(h) => u32::from_str_radix(h, 16).unwrap_or_else(|_| panic!("hex {s}")),
        None => t.parse().unwrap_or_else(|_| panic!("number {s}")),
    }
}

#[test]
fn slice_geometry_matches_phase12() {
    let p12 = read("ebpf/monad-cpu-ebpf/src/phase12.rs");
    assert_eq!(
        rust_const(&p12, "USER_CHILD_SLICE"),
        geometry::XV6_CHILD_SLICE_BASE
    );
    assert_eq!(rust_const(&p12, "SLICE_STRIDE"), geometry::XV6_SLICE_STRIDE);
    assert_eq!(
        rust_const(&p12, "RAM_ISOLATION_CEILING"),
        geometry::XV6_ISOLATION_CEILING
    );
    assert_eq!(rust_const(&p12, "PER_PID_PGD_BASE"), geometry::XV6_PGD_BASE);
    assert_eq!(rust_const(&p12, "PGD_SIZE_BYTES"), geometry::XV6_PGD_SIZE);
    assert_eq!(rust_const(&p12, "MAX_PROCESSES"), geometry::XV6_MAX_PIDS);
}

#[test]
fn slice_len_matches_user_va_ceiling() {
    let main = read("ebpf/monad-cpu-ebpf/src/main.rs");
    assert_eq!(
        rust_const(&main, "USER_VA_CEILING"),
        geometry::XV6_SLICE_LEN
    );
    // PROC_TABLE has one row per pid.
    assert!(
        main.contains(&format!(
            "static PROC_TABLE: Array<[u32; 22]> = Array::with_max_entries({}, 0);",
            geometry::XV6_MAX_PIDS
        )),
        "PROC_TABLE max_entries is not {}",
        geometry::XV6_MAX_PIDS
    );
}

#[test]
fn object_geometry_static_names_the_same_constants() {
    // UPC_MMU_GEOMETRY must be built from these five, in this order.
    let facts = read("ebpf/monad-cpu-ebpf/src/upc_facts.rs");
    let body = facts
        .split("pub static UPC_MMU_GEOMETRY: [u32; 5] = [")
        .nth(1)
        .expect("UPC_MMU_GEOMETRY static")
        .split("];")
        .next()
        .unwrap();
    let items: Vec<&str> = body
        .split(',')
        .map(str::trim)
        .filter(|s| !s.is_empty())
        .collect();
    assert_eq!(
        items,
        [
            "phase12::USER_CHILD_SLICE",
            "phase12::SLICE_STRIDE",
            "crate::USER_VA_CEILING",
            "phase12::RAM_ISOLATION_CEILING",
            "intr::MAX_PROCESSES as u32",
        ]
    );
}

#[test]
fn kernel_and_adapter_layout_matches_reserved_regions() {
    // The owned kernel caps physical memory at PHYSTOP...
    let mk = read("crates/xv6-mbc/adapters/Makefile.mbc");
    let phystop = mk
        .split("-DPHYSTOP=")
        .nth(1)
        .expect("-DPHYSTOP in Makefile.mbc")
        .split_whitespace()
        .next()
        .unwrap();
    assert_eq!(parse_num(phystop), geometry::XV6_PHYSTOP);
    // ...and places pid page directories where the loader reserves them.
    let uvm = read("crates/xv6-mbc/adapters/uvm.c");
    let pgd = uvm
        .lines()
        .find(|l| l.starts_with("#define MBC_PER_PID_PGD_BASE"))
        .expect("MBC_PER_PID_PGD_BASE in uvm.c")
        .split_whitespace()
        .nth(2)
        .unwrap();
    assert_eq!(parse_num(pgd), geometry::XV6_PGD_BASE);
}

#[test]
fn boot_stack_top_matches_initial_cpu_state() {
    let runner = read("cmd/upc-bootctl/src/runner.rs");
    let body = runner
        .split("pub fn xv6_initial_cpu_state() -> MbcCpuState {")
        .nth(1)
        .expect("xv6_initial_cpu_state in runner.rs");
    let sp = body
        .lines()
        .find(|l| l.trim_start().starts_with("state.regs[15] = "))
        .expect("SP assignment")
        .split('=')
        .nth(1)
        .unwrap()
        .split(';')
        .next()
        .unwrap();
    assert_eq!(parse_num(sp), geometry::XV6_BOOT_SP);
}

#[test]
fn gate_nway_falsification_stride_is_below_the_slice() {
    // gate_nway.c probes at HIGH_BASE, "the falsification stride": a stride
    // that small must be one the geometry gate refuses.
    let c = read("crates/xv6-mbc/upstream/user/gate_nway.c");
    let high = c
        .lines()
        .find(|l| l.starts_with("#define HIGH_BASE"))
        .expect("HIGH_BASE in gate_nway.c")
        .split_whitespace()
        .nth(2)
        .unwrap();
    assert!(parse_num(high) < geometry::XV6_SLICE_LEN);
}
