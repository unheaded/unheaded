// SPDX-License-Identifier: GPL-3.0-or-later
//! L17: `boot` and `console` hold the eBPF object to crates/objtrust before they
//! read it, dry run included, and --allow-untrusted-ebpf-obj-dir is the only way
//! past. The object here is junk, so a run let through stops at the object gates
//! with no map, program or netns made.
//!
//! The unit keeps images in /var/lib/unheaded/upc, which its own account owns,
//! so the object it loads must live elsewhere: the last test reads the unit.

use sha2::{Digest, Sha256};
use std::path::{Path, PathBuf};
use std::process::{Command, Output};

const RV: &[u8] = &[0x00, 0x40, 0x00, 0x00, 0x04, 0x40, 0x00, 0x00];
const UNTRUSTED: &str = "writable by a non-root user";
const FLAG: &str = "--allow-untrusted-ebpf-obj-dir";

/// A user-owned temp dir with a kernel, its checked `.rv2mbc`, and an object.
struct Fixture {
    dir: PathBuf,
}

impl Fixture {
    fn new(tag: &str) -> Self {
        let dir = std::env::temp_dir().join(format!("upc-objtrust-{tag}-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&dir);
        std::fs::create_dir_all(&dir).unwrap();
        std::fs::write(dir.join("xv6-mbc.mbc"), [0u8; 64]).unwrap();
        std::fs::write(dir.join("xv6-mbc.rv2mbc"), RV).unwrap();
        let sum: String = Sha256::digest(RV)
            .iter()
            .map(|b| format!("{b:02x}"))
            .collect();
        std::fs::write(
            dir.join("xv6-mbc.rv2mbc.sha256"),
            format!("{sum}  xv6-mbc.rv2mbc\n"),
        )
        .unwrap();
        std::fs::write(dir.join("monad-cpu-ebpf"), b"not an elf").unwrap();
        Fixture { dir }
    }
    fn kernel(&self) -> String {
        self.dir.join("xv6-mbc.mbc").display().to_string()
    }
    fn object(&self) -> PathBuf {
        self.dir.join("monad-cpu-ebpf")
    }
}

impl Drop for Fixture {
    fn drop(&mut self) {
        std::fs::remove_dir_all(&self.dir).ok();
    }
}

fn bootctl(args: &[&str], object: &Path) -> Output {
    Command::new(env!("CARGO_BIN_EXE_upc-bootctl"))
        .args(args)
        .env("UPC_DEMO_MODE", "1")
        .env_remove("UPC_RV2MBC_SHA")
        .env("MONAD_CPU_EBPF_OBJ", object)
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
fn a_dry_run_refuses_a_user_owned_object() {
    let f = Fixture::new("dry");
    let o = bootctl(&["boot", "--kernel", &f.kernel(), "--dry-run"], &f.object());
    assert!(!o.status.success(), "{}", out(&o));
    assert!(out(&o).contains(UNTRUSTED), "{}", out(&o));
}

#[test]
fn a_live_boot_refuses_a_user_owned_object() {
    let f = Fixture::new("live");
    let o = bootctl(&["boot", "--kernel", &f.kernel()], &f.object());
    assert!(!o.status.success(), "{}", out(&o));
    assert!(out(&o).contains(UNTRUSTED), "{}", out(&o));
}

#[test]
fn a_console_refuses_a_user_owned_object() {
    let f = Fixture::new("console");
    let o = bootctl(&["console", "--kernel", &f.kernel()], &f.object());
    assert!(!o.status.success(), "{}", out(&o));
    assert!(out(&o).contains(UNTRUSTED), "{}", out(&o));
}

#[test]
fn the_flag_reaches_the_object_gates() {
    let f = Fixture::new("flag");
    for args in [
        vec!["boot", "--kernel", &f.kernel(), "--dry-run", FLAG],
        vec!["boot", "--kernel", &f.kernel(), FLAG],
        vec!["console", "--kernel", &f.kernel(), FLAG],
    ] {
        let o = bootctl(&args, &f.object());
        assert!(!o.status.success(), "junk is no object: {}", out(&o));
        assert!(!out(&o).contains(UNTRUSTED), "{}", out(&o));
        assert!(out(&o).contains("refusing to load"), "{}", out(&o));
    }
}

#[test]
fn the_unit_loads_an_object_its_own_account_cannot_replace() {
    let unit = Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("../../deploy/systemd/unheaded-upc-bootctl@.service");
    let text = std::fs::read_to_string(unit).unwrap();
    let object = text
        .lines()
        .find_map(|l| l.strip_prefix("Environment=MONAD_CPU_EBPF_OBJ="))
        .expect("the unit names its object");
    assert_eq!(object, "/usr/lib/unheaded/ebpf/monad-cpu-ebpf");
    assert!(!text.contains(FLAG), "the unit must not opt out");
    assert!(
        !text
            .lines()
            .any(|l| l.starts_with("ReadWritePaths=") && l.contains("/usr/lib/unheaded")),
        "the object's directory is not the unit's to write"
    );
}
