// SPDX-License-Identifier: GPL-3.0-or-later
//! `ebpf/monad-cpu-ebpf/surface.rs` (what build.rs hashes into UPC_SURFACE_ID)
//! must list exactly the ecall numbers the `op::SYSCALL` dispatch in
//! `src/main.rs` has a top-level arm for, per build (S-ASCEND-2 Step 106).
//! Adding or removing an arm without updating surface.rs fails here, before a
//! loader refuses the object at boot.

use std::collections::BTreeSet;
use std::path::PathBuf;

mod object_surface {
    include!("../../../ebpf/monad-cpu-ebpf/surface.rs");
}

fn read(rel: &str) -> String {
    let p = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../..")
        .join(rel);
    std::fs::read_to_string(&p).unwrap_or_else(|e| panic!("read {}: {e}", p.display()))
}

/// `pub const NAME: u32 = N;` inside `pub mod <module> {` of monad-common.
fn monad_common_const(src: &str, module: &str, name: &str) -> u32 {
    let body = src
        .split(&format!("pub mod {module} {{"))
        .nth(1)
        .unwrap_or_else(|| panic!("mod {module} not in monad-common"));
    let line = body
        .lines()
        .find(|l| {
            l.trim_start()
                .starts_with(&format!("pub const {name}: u32 = "))
        })
        .unwrap_or_else(|| panic!("{module}::{name} not found"));
    let rhs = line
        .split('=')
        .nth(1)
        .unwrap()
        .split(';')
        .next()
        .unwrap()
        .trim();
    match rhs.strip_prefix("0x") {
        Some(h) => u32::from_str_radix(&h.replace('_', ""), 16).unwrap(),
        None => rhs.replace('_', "").parse().unwrap(),
    }
}

#[derive(Debug, PartialEq, Eq, Clone, Copy)]
enum Gate {
    Both,
    AscendOnly,
    DefaultOnly,
}

/// Every top-level arm of the `op::SYSCALL` chain as (gate, number).
fn dispatch_arms() -> Vec<(Gate, u32)> {
    let main = read("ebpf/monad-cpu-ebpf/src/main.rs");
    let common = read("ebpf/monad-common/src/lib.rs");
    let start = main
        .find("} else if opc == op::SYSCALL {")
        .expect("op::SYSCALL arm in main.rs");
    let rest = &main[start..];
    let end = rest
        .find("// Unknown syscall: silently ignore")
        .expect("end of the op::SYSCALL chain");
    let chain = &rest[..end];

    // The chain's arms share the indentation of its first `if`.
    let first = chain
        .lines()
        .find(|l| l.trim_start().starts_with("if ") && l.contains("syscall_nr =="))
        .expect("first arm");
    let indent = first.len() - first.trim_start().len();

    let mut arms = Vec::new();
    for line in chain.lines() {
        if line.len() - line.trim_start().len() != indent {
            continue;
        }
        let t = line.trim_start();
        let cond = match t
            .strip_prefix("} else if ")
            .or_else(|| t.strip_prefix("if "))
        {
            Some(c) => c,
            None => continue,
        };
        let gate = if cond.contains("!cfg!(feature = \"ascend-linux\")") {
            Gate::DefaultOnly
        } else if cond.contains("cfg!(feature = \"ascend-linux\")") {
            Gate::AscendOnly
        } else {
            Gate::Both
        };
        let parts: Vec<&str> = cond.split("syscall_nr == ").skip(1).collect();
        assert_eq!(
            parts.len(),
            1,
            "arm with {} comparisons: {line}",
            parts.len()
        );
        let tok = parts[0]
            .split(|c: char| c.is_whitespace() || c == '{' || c == '|' || c == '&')
            .next()
            .unwrap();
        let number = if let Some(n) = tok.strip_prefix("sys::") {
            monad_common_const(&common, "mbc_syscalls", n)
        } else if let Some(n) = tok.strip_prefix("lsys::") {
            monad_common_const(&common, "mbc_linux_syscalls", n)
        } else {
            tok.parse()
                .unwrap_or_else(|_| panic!("unrecognised syscall token {tok:?} in: {line}"))
        };
        arms.push((gate, number));
    }
    arms
}

fn build_surface(ascend: bool) -> BTreeSet<u32> {
    dispatch_arms()
        .into_iter()
        .filter(|(g, _)| match g {
            Gate::Both => true,
            Gate::AscendOnly => ascend,
            Gate::DefaultOnly => !ascend,
        })
        .map(|(_, n)| n)
        .collect()
}

#[test]
fn dispatch_chain_parses() {
    // 19 arms at the time of writing; a parse that finds few means the
    // chain moved and this test needs updating, not that the surface shrank.
    assert!(dispatch_arms().len() >= 15, "{:?}", dispatch_arms());
}

#[test]
fn default_surface_matches_dispatch() {
    let listed: BTreeSet<u32> = object_surface::SURFACE_DEFAULT.iter().copied().collect();
    assert_eq!(
        listed.len(),
        object_surface::SURFACE_DEFAULT.len(),
        "duplicate"
    );
    assert_eq!(build_surface(false), listed);
}

#[test]
fn ascend_surface_matches_dispatch() {
    let listed: BTreeSet<u32> = object_surface::SURFACE_ASCEND.iter().copied().collect();
    assert_eq!(
        listed.len(),
        object_surface::SURFACE_ASCEND.len(),
        "duplicate"
    );
    assert_eq!(build_surface(true), listed);
}

#[test]
fn the_two_builds_have_different_surface_ids() {
    // Gate A can only tell the objects apart if their ids differ.
    assert_ne!(
        upc_api::surface_hash::surface_id(object_surface::SURFACE_DEFAULT),
        upc_api::surface_hash::surface_id(object_surface::SURFACE_ASCEND)
    );
}
