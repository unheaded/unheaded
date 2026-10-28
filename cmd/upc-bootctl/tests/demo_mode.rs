// SPDX-License-Identifier: GPL-3.0-or-later
//! S-ASCEND-2 Step 117: a live `boot` or `console` needs `UPC_DEMO_MODE=1`.
//! The check runs before anything touches BPF or a netns; `--dry-run` never
//! needs it. These tests never reach BPF: the refused runs stop at the gate,
//! the dry runs stop before loading, and the allowed run uses a kernel path
//! that does not exist, so it fails on the file read after passing the gate.

use std::path::PathBuf;
use std::process::{Command, Output};

const GATE_MSG: &str = "UPC_DEMO_MODE=1 required for a live boot; add --dry-run";

fn bootctl(args: &[&str], demo: Option<&str>) -> Output {
    let mut cmd = Command::new(env!("CARGO_BIN_EXE_upc-bootctl"));
    cmd.args(args).env_remove("UPC_DEMO_MODE");
    // No object at this path, so a dry run skips the object preflight.
    cmd.env("MONAD_CPU_EBPF_OBJ", "/nonexistent/monad-cpu-ebpf");
    if let Some(v) = demo {
        cmd.env("UPC_DEMO_MODE", v);
    }
    cmd.output().expect("run upc-bootctl")
}

/// A structurally valid kernel image: any whole number of MBC words.
fn tiny_kernel(tag: &str) -> PathBuf {
    let p = std::env::temp_dir().join(format!("upc-demo-mode-{}-{}.mbc", tag, std::process::id()));
    std::fs::write(&p, [0u8; 64]).expect("write tiny kernel");
    p
}

fn stderr(o: &Output) -> String {
    String::from_utf8_lossy(&o.stderr).into_owned()
}

#[test]
fn live_boot_without_demo_mode_is_refused() {
    let k = tiny_kernel("refused");
    let o = bootctl(
        &["boot", "--kernel", k.to_str().unwrap(), "--instance", "111"],
        None,
    );
    std::fs::remove_file(&k).ok();
    assert!(
        !o.status.success(),
        "a live boot without UPC_DEMO_MODE must fail"
    );
    assert!(stderr(&o).contains(GATE_MSG), "stderr: {}", stderr(&o));
}

#[test]
fn demo_mode_other_than_1_is_refused() {
    let k = tiny_kernel("zero");
    let o = bootctl(&["boot", "--kernel", k.to_str().unwrap()], Some("0"));
    std::fs::remove_file(&k).ok();
    assert!(!o.status.success());
    assert!(stderr(&o).contains(GATE_MSG), "stderr: {}", stderr(&o));
}

#[test]
fn live_console_without_demo_mode_is_refused() {
    let k = tiny_kernel("console");
    let o = bootctl(&["console", "--kernel", k.to_str().unwrap()], None);
    std::fs::remove_file(&k).ok();
    assert!(!o.status.success());
    assert!(stderr(&o).contains(GATE_MSG), "stderr: {}", stderr(&o));
}

#[test]
fn dry_run_needs_no_demo_mode() {
    let k = tiny_kernel("dry");
    let o = bootctl(
        &[
            "boot",
            "--kernel",
            k.to_str().unwrap(),
            "--instance",
            "111",
            "--dry-run",
        ],
        None,
    );
    std::fs::remove_file(&k).ok();
    assert!(o.status.success(), "dry run must pass: {}", stderr(&o));
    assert!(!stderr(&o).contains(GATE_MSG));
}

#[test]
fn demo_mode_1_passes_the_gate() {
    let o = bootctl(
        &[
            "boot",
            "--kernel",
            "/nonexistent/kernel.mbc",
            "--instance",
            "111",
        ],
        Some("1"),
    );
    assert!(!o.status.success(), "the kernel path does not exist");
    let e = stderr(&o);
    assert!(!e.contains(GATE_MSG), "the gate must not refuse: {e}");
    assert!(e.contains("read kernel"), "expected the file error: {e}");
}
