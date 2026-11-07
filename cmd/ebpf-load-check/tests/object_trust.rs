// SPDX-License-Identifier: GPL-2.0-only
// Copyright (c) 2026 Stevie Bellis.

//! L17: the tool reads no object a non-root user could have written, unless told
//! to. The object here is junk on purpose: past the trust check it fails to parse.

use std::path::PathBuf;
use std::process::{Command, Output};

fn user_owned_object(tag: &str) -> PathBuf {
    let dir = std::env::temp_dir().join(format!("load-check-{tag}-{}", std::process::id()));
    std::fs::create_dir_all(&dir).expect("scratch dir");
    let p = dir.join("prog.o");
    std::fs::write(&p, b"not an elf").expect("write object");
    p
}

fn load_check(args: &[&str]) -> Output {
    Command::new(env!("CARGO_BIN_EXE_ebpf-load-check"))
        .args(args)
        .output()
        .expect("run ebpf-load-check")
}

fn stderr(o: &Output) -> String {
    String::from_utf8_lossy(&o.stderr).into_owned()
}

#[test]
fn a_user_owned_object_is_refused_before_it_is_read() {
    let p = user_owned_object("refused");
    let o = load_check(&[p.to_str().unwrap()]);
    assert_eq!(o.status.code(), Some(2), "{}", stderr(&o));
    assert!(
        stderr(&o).contains("writable by a non-root user"),
        "{}",
        stderr(&o)
    );
    assert!(!stderr(&o).contains("cannot parse"), "{}", stderr(&o));
}

#[test]
fn the_flag_lets_a_build_tree_through() {
    let p = user_owned_object("allowed");
    let o = load_check(&["--allow-untrusted-ebpf-obj-dir", p.to_str().unwrap()]);
    assert!(stderr(&o).contains("cannot parse"), "{}", stderr(&o));
}

#[test]
fn anything_else_on_the_command_line_is_a_usage_error() {
    let o = load_check(&["--allow-everything", "x"]);
    assert_eq!(o.status.code(), Some(2));
    assert!(stderr(&o).contains("usage:"), "{}", stderr(&o));
}
