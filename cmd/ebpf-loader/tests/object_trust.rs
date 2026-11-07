// SPDX-License-Identifier: GPL-2.0-only
// Copyright (c) 2026 Stevie Bellis.

//! L17: every object is held to crates/objtrust before any of them is read. A
//! dry run stops at the same gate, so it shows the refusal without a kernel.

use std::path::PathBuf;
use std::process::{Command, Output};

fn user_owned_dir(tag: &str) -> PathBuf {
    let dir = std::env::temp_dir().join(format!("ebpf-loader-{tag}-{}", std::process::id()));
    std::fs::create_dir_all(&dir).expect("scratch dir");
    std::fs::write(dir.join("packet-marker"), b"object").expect("write object");
    dir
}

fn loader(dir: &PathBuf, extra: &[&str]) -> Output {
    Command::new(env!("CARGO_BIN_EXE_ebpf-loader"))
        .args(["--dry-run", "--only", "packet-marker", "--obj-dir"])
        .arg(dir)
        .args(extra)
        .output()
        .expect("run ebpf-loader")
}

fn stderr(o: &Output) -> String {
    String::from_utf8_lossy(&o.stderr).into_owned()
}

#[test]
fn a_user_owned_object_is_refused() {
    let o = loader(&user_owned_dir("refused"), &[]);
    assert!(!o.status.success(), "a user-owned object passed");
    assert!(
        stderr(&o).contains("writable by a non-root user"),
        "{}",
        stderr(&o)
    );
}

#[test]
fn the_flag_lets_a_build_tree_through() {
    let o = loader(
        &user_owned_dir("allowed"),
        &["--allow-untrusted-ebpf-obj-dir"],
    );
    assert!(o.status.success(), "{}", stderr(&o));
}
