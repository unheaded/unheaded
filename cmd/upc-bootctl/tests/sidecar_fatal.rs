// SPDX-License-Identifier: GPL-3.0-or-later
//! S-STAGE-2C Phase 5 (Stevie's answer A-61): a bad or missing `.data` sidecar
//! refuses the boot. The `--userland` image's sidecar used to be loaded with a
//! warning on failure, and a resident program with no sidecar was given an
//! empty data region in silence: either way the program ran with whatever was
//! in memory where its strings and globals should be.
//!
//! The check runs before the eBPF object is opened, so a dry run shows it.
//! `MONAD_CPU_EBPF_OBJ` points at a path that does not exist.

use std::path::{Path, PathBuf};
use std::process::{Command, Output};

const RV: &[u8] = &[0x00, 0x40, 0x00, 0x00, 0x04, 0x40, 0x00, 0x00];
/// A TLV with no record: count 0.
const EMPTY_TLV: &[u8] = &[0, 0, 0, 0];
/// One record at 0x1000, four bytes.
const ONE_RECORD: &[u8] = &[
    1, 0, 0, 0, 0x00, 0x10, 0, 0, 4, 0, 0, 0, 0xAA, 0xBB, 0xCC, 0xDD,
];

struct Fixture {
    dir: PathBuf,
}

impl Fixture {
    fn new(tag: &str) -> Self {
        let dir = std::env::temp_dir().join(format!("upc-sidecar-{tag}-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&dir);
        std::fs::create_dir_all(&dir).unwrap();
        let f = Fixture { dir };
        f.image("xv6-mbc");
        f
    }
    fn path(&self, name: &str) -> PathBuf {
        self.dir.join(format!("{name}.mbc"))
    }
    /// `<name>.mbc` (16 zero words) and its `.rv2mbc`; no `.data`.
    fn image(&self, name: &str) {
        std::fs::write(self.path(name), [0u8; 64]).unwrap();
        std::fs::write(self.dir.join(format!("{name}.rv2mbc")), RV).unwrap();
    }
    fn data(&self, name: &str, bytes: &[u8]) {
        std::fs::write(self.dir.join(format!("{name}.data")), bytes).unwrap();
    }
}

impl Drop for Fixture {
    fn drop(&mut self) {
        std::fs::remove_dir_all(&self.dir).ok();
    }
}

fn dry_run(kernel: &Path, userland: &Path) -> Output {
    Command::new(env!("CARGO_BIN_EXE_upc-bootctl"))
        .args([
            "boot",
            "--kernel",
            kernel.to_str().unwrap(),
            "--userland",
            userland.to_str().unwrap(),
            "--dry-run",
            "--allow-unverified-rv2mbc",
        ])
        .env_remove("UPC_DEMO_MODE")
        .env_remove("UPC_RV2MBC_SHA")
        .env("MONAD_CPU_EBPF_OBJ", "/nonexistent/monad-cpu-ebpf")
        .output()
        .expect("run upc-bootctl")
}

fn out(o: &Output) -> String {
    format!(
        "{}{}",
        String::from_utf8_lossy(&o.stdout),
        String::from_utf8_lossy(&o.stderr)
    )
}

#[test]
fn good_sidecars_pass_and_an_empty_tlv_is_one() {
    let f = Fixture::new("good");
    f.image("init");
    f.data("init", ONE_RECORD);
    f.image("sh");
    f.data("sh", EMPTY_TLV);
    let o = dry_run(&f.path("xv6-mbc"), &f.path("init"));
    let e = out(&o);
    assert!(o.status.success(), "{e}");
    assert!(!e.contains("UserSidecar"), "{e}");
}

#[test]
fn a_truncated_userland_sidecar_refuses_the_boot() {
    let f = Fixture::new("truncated");
    f.image("init");
    f.data("init", &ONE_RECORD[..10]); // the record header is cut short
    let o = dry_run(&f.path("xv6-mbc"), &f.path("init"));
    let e = out(&o);
    assert!(!o.status.success(), "must refuse: {e}");
    assert!(e.contains("UserSidecarBad"), "{e}");
    assert!(e.contains("init.data"), "the file must be named: {e}");
}

#[test]
fn a_missing_userland_sidecar_refuses_the_boot() {
    let f = Fixture::new("missing-init");
    f.image("init");
    let o = dry_run(&f.path("xv6-mbc"), &f.path("init"));
    let e = out(&o);
    assert!(!o.status.success(), "must refuse: {e}");
    assert!(e.contains("UserSidecarMissing"), "{e}");
    assert!(e.contains("init.data"), "{e}");
}

#[test]
fn a_resident_program_with_no_sidecar_refuses_the_boot() {
    let f = Fixture::new("missing-resident");
    f.image("init");
    f.data("init", EMPTY_TLV);
    f.image("sh"); // sh.mbc is there, sh.data is not
    let o = dry_run(&f.path("xv6-mbc"), &f.path("init"));
    let e = out(&o);
    assert!(!o.status.success(), "must refuse: {e}");
    assert!(e.contains("UserSidecarMissing"), "{e}");
    assert!(e.contains("sh.data"), "{e}");
}

#[test]
fn a_resident_program_with_a_bad_sidecar_refuses_the_boot() {
    let f = Fixture::new("bad-resident");
    f.image("init");
    f.data("init", EMPTY_TLV);
    f.image("sh");
    f.data("sh", &[2, 0]); // shorter than the record count
    let o = dry_run(&f.path("xv6-mbc"), &f.path("init"));
    let e = out(&o);
    assert!(!o.status.success(), "must refuse: {e}");
    assert!(e.contains("UserSidecarBad") && e.contains("sh.data"), "{e}");
}
