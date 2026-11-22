// SPDX-License-Identifier: GPL-2.0-only
// Copyright (c) 2026 Stevie Bellis.

//! M-198.7: a pin left under the Shield pin directory by an earlier load names
//! a map this load does not use, and a reader of the pin would go on reading
//! it. load_shield refuses it, as load_hop does, before anything is parsed,
//! loaded or attached. No kernel is needed: the refusal comes first.

use std::path::{Path, PathBuf};
use std::process::{Command, Output};

fn scratch(tag: &str, program: &str) -> PathBuf {
    let dir = std::env::temp_dir().join(format!("ebpf-loader-{tag}-{}", std::process::id()));
    let _ = std::fs::remove_dir_all(&dir);
    std::fs::create_dir_all(&dir).expect("scratch dir");
    // Not an ELF: a loader that gets as far as parsing it fails there instead.
    std::fs::write(dir.join(program), b"object").expect("write object");
    dir
}

fn loader(dir: &Path, program: &str, pins: &Path) -> Output {
    Command::new(env!("CARGO_BIN_EXE_ebpf-loader"))
        .args(["--only", program, "--obj-dir"])
        .arg(dir)
        .arg("--allow-untrusted-ebpf-obj-dir")
        .arg("--map-pin-path")
        .arg(pins)
        .output()
        .expect("run ebpf-loader")
}

fn stderr(o: &Output) -> String {
    String::from_utf8_lossy(&o.stderr).into_owned()
}

fn refuses_a_stale_pin(program: &str, map: &str) {
    let dir = scratch(&format!("stale-{program}"), program);
    let pins = dir.join("pins");
    let pin_dir = pins.join(program);
    std::fs::create_dir_all(&pin_dir).expect("pin dir");
    let stale = pin_dir.join(map);
    std::fs::write(&stale, b"").expect("stale pin");

    let o = loader(&dir, program, &pins);
    let err = stderr(&o);
    assert!(!o.status.success(), "a stale pin was accepted: {err}");
    assert!(
        err.contains(&stale.display().to_string()),
        "the refusal does not name the pin: {err}"
    );
    assert!(
        err.contains("left by an earlier load"),
        "the refusal does not give the reason: {err}"
    );
    assert!(
        !err.contains("Failed to parse"),
        "the object was read before the refusal: {err}"
    );
    assert!(stale.exists(), "the loader removed the pin itself");
    let _ = std::fs::remove_dir_all(&dir);
}

#[test]
fn load_shield_refuses_a_stale_pin() {
    refuses_a_stale_pin("shield-ebpf", "ANAMNESIS");
}

#[test]
fn load_hop_refuses_a_stale_pin_before_loading() {
    refuses_a_stale_pin("hop-ebpf", "HOP_VALIDATORS");
}

/// An empty pin directory, which the loader itself creates, is not stale: the
/// run gets past the check and fails on the object, which is not an ELF.
#[test]
fn an_empty_pin_directory_is_not_stale() {
    let dir = scratch("empty-pins", "shield-ebpf");
    let pins = dir.join("pins");
    std::fs::create_dir_all(pins.join("shield-ebpf")).expect("pin dir");
    let o = loader(&dir, "shield-ebpf", &pins);
    let err = stderr(&o);
    assert!(!o.status.success());
    assert!(
        err.contains("Failed to parse") && !err.contains("left by an earlier load"),
        "{err}"
    );
    let _ = std::fs::remove_dir_all(&dir);
}

/// A trace_events link that points at some other map is the same hazard for
/// the collector: refused, not skipped.
#[test]
fn load_shield_refuses_a_trace_events_link_to_another_map() {
    let dir = scratch("stale-link", "shield-ebpf");
    let pins = dir.join("pins");
    std::fs::create_dir_all(&pins).expect("pins");
    let elsewhere = dir.join("elsewhere");
    std::fs::write(&elsewhere, b"").expect("other map");
    let link = pins.join("trace_events");
    std::os::unix::fs::symlink(&elsewhere, &link).expect("link");

    let o = loader(&dir, "shield-ebpf", &pins);
    let err = stderr(&o);
    assert!(!o.status.success(), "a stale link was accepted: {err}");
    assert!(
        err.contains(&link.display().to_string()) && err.contains("trace_events"),
        "the refusal does not name the link: {err}"
    );
    assert!(!err.contains("Failed to parse"), "{err}");
    let _ = std::fs::remove_dir_all(&dir);
}
