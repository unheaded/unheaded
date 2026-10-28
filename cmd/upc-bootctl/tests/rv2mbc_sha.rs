// SPDX-License-Identifier: GPL-3.0-or-later
//! S-ASCEND-2 Item 5 (Steps 122-127): every `.rv2mbc` upc-bootctl puts into
//! RV2MBC_MAP is checked against a SHA-256 before any map write. The expected
//! hash comes from `--rv2mbc-sha` / `UPC_RV2MBC_SHA` (kernel only), else the
//! sibling `<file>.rv2mbc.sha256`, else the boot is refused.
//! `--allow-unverified-rv2mbc` is a dry-run-only hatch.
//!
//! These tests never reach BPF: `MONAD_CPU_EBPF_OBJ` points at a path that does
//! not exist, so a live run that got past the gate would stop at the object
//! check before any map, program or netns exists.

use sha2::{Digest, Sha256};
use std::path::{Path, PathBuf};
use std::process::{Command, Output};

const RV: &[u8] = &[0x00, 0x40, 0x00, 0x00, 0x04, 0x40, 0x00, 0x00];
const WRONG: &str = "0000000000000000000000000000000000000000000000000000000000000000";
const NO_OBJECT: &str = "monad-cpu-ebpf object not found";

fn hex(bytes: &[u8]) -> String {
    Sha256::digest(bytes)
        .iter()
        .map(|b| format!("{b:02x}"))
        .collect()
}

/// A temp dir holding `xv6-mbc.mbc` (16 zero words) and `xv6-mbc.rv2mbc`.
struct Fixture {
    dir: PathBuf,
}

impl Fixture {
    fn new(tag: &str) -> Self {
        let dir = std::env::temp_dir().join(format!("upc-rv2mbc-{tag}-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&dir);
        std::fs::create_dir_all(&dir).unwrap();
        let f = Fixture { dir };
        f.program("xv6-mbc");
        f
    }
    fn kernel(&self) -> PathBuf {
        self.dir.join("xv6-mbc.mbc")
    }
    fn rv2mbc(&self, name: &str) -> PathBuf {
        self.dir.join(format!("{name}.rv2mbc"))
    }
    /// `<name>.mbc` + `<name>.rv2mbc`, no sidecar.
    fn program(&self, name: &str) {
        std::fs::write(self.dir.join(format!("{name}.mbc")), [0u8; 64]).unwrap();
        std::fs::write(self.rv2mbc(name), RV).unwrap();
    }
    fn sidecar(&self, name: &str, text: &str) {
        let mut p = self.rv2mbc(name).into_os_string();
        p.push(".sha256");
        std::fs::write(p, text).unwrap();
    }
    /// A correct `sha256sum`-format sidecar.
    fn good_sidecar(&self, name: &str) {
        self.sidecar(name, &format!("{}  {name}.rv2mbc\n", hex(RV)));
    }
}

impl Drop for Fixture {
    fn drop(&mut self) {
        std::fs::remove_dir_all(&self.dir).ok();
    }
}

fn bootctl(args: &[&str], envs: &[(&str, &str)]) -> Output {
    let mut cmd = Command::new(env!("CARGO_BIN_EXE_upc-bootctl"));
    cmd.args(args)
        .env_remove("UPC_DEMO_MODE")
        .env_remove("UPC_RV2MBC_SHA")
        .env("MONAD_CPU_EBPF_OBJ", "/nonexistent/monad-cpu-ebpf");
    for (k, v) in envs {
        cmd.env(k, v);
    }
    cmd.output().expect("run upc-bootctl")
}

fn dry_run(k: &Path, extra: &[&str], envs: &[(&str, &str)]) -> Output {
    let mut args = vec!["boot", "--kernel", k.to_str().unwrap(), "--dry-run"];
    args.extend_from_slice(extra);
    bootctl(&args, envs)
}

fn out(o: &Output) -> String {
    format!(
        "{}{}",
        String::from_utf8_lossy(&o.stdout),
        String::from_utf8_lossy(&o.stderr)
    )
}

// (1) no SHA, no flag → refused with Rv2mbcShaRequired.
#[test]
fn no_sha_and_no_flag_is_refused() {
    let f = Fixture::new("required");
    let o = dry_run(&f.kernel(), &[], &[]);
    let e = out(&o);
    assert!(!o.status.success(), "must refuse: {e}");
    assert!(e.contains("Rv2mbcShaRequired"), "{e}");
    assert!(
        e.contains(&f.rv2mbc("xv6-mbc").display().to_string()),
        "{e}"
    );
}

// (2) wrong SHA → Rv2mbcShaMismatch with both hashes, from each input.
#[test]
fn wrong_flag_sha_is_a_mismatch() {
    let f = Fixture::new("mismatch-flag");
    let o = dry_run(&f.kernel(), &["--rv2mbc-sha", WRONG], &[]);
    let e = out(&o);
    assert!(!o.status.success(), "{e}");
    assert!(e.contains("Rv2mbcShaMismatch"), "{e}");
    assert!(e.contains(WRONG) && e.contains(&hex(RV)), "{e}");
}

#[test]
fn wrong_env_sha_is_a_mismatch() {
    let f = Fixture::new("mismatch-env");
    let o = dry_run(&f.kernel(), &[], &[("UPC_RV2MBC_SHA", WRONG)]);
    let e = out(&o);
    assert!(!o.status.success(), "{e}");
    assert!(e.contains("Rv2mbcShaMismatch"), "{e}");
}

#[test]
fn wrong_sidecar_is_a_mismatch() {
    let f = Fixture::new("mismatch-sidecar");
    f.sidecar("xv6-mbc", WRONG);
    let o = dry_run(&f.kernel(), &[], &[]);
    let e = out(&o);
    assert!(!o.status.success(), "{e}");
    assert!(e.contains("Rv2mbcShaMismatch"), "{e}");
}

// (3) sidecar present and correct → proceeds; BootParams v3 carries the hash.
#[test]
fn correct_sidecar_proceeds() {
    let f = Fixture::new("sidecar-ok");
    f.good_sidecar("xv6-mbc");
    let o = dry_run(&f.kernel(), &[], &[]);
    let e = out(&o);
    assert!(o.status.success(), "{e}");
    assert!(e.contains(&hex(RV)), "the verified hash is reported: {e}");
    assert!(e.contains("version = 3"), "{e}");
}

#[test]
fn correct_flag_sha_proceeds_without_a_sidecar() {
    let f = Fixture::new("flag-ok");
    let o = dry_run(&f.kernel(), &["--rv2mbc-sha", &hex(RV)], &[]);
    assert!(o.status.success(), "{}", out(&o));
}

// (4) --allow-unverified-rv2mbc + dry-run → warning + proceed.
#[test]
fn hatch_with_dry_run_warns_and_proceeds() {
    let f = Fixture::new("hatch");
    let o = dry_run(&f.kernel(), &["--allow-unverified-rv2mbc"], &[]);
    let e = out(&o);
    assert!(o.status.success(), "{e}");
    assert!(
        String::from_utf8_lossy(&o.stderr).contains("warning: --allow-unverified-rv2mbc"),
        "{e}"
    );
}

#[test]
fn hatch_does_not_excuse_a_mismatch() {
    let f = Fixture::new("hatch-mismatch");
    f.sidecar("xv6-mbc", WRONG);
    let o = dry_run(&f.kernel(), &["--allow-unverified-rv2mbc"], &[]);
    assert!(!o.status.success());
    assert!(out(&o).contains("Rv2mbcShaMismatch"), "{}", out(&o));
}

#[test]
fn hatch_on_a_live_boot_is_refused() {
    let f = Fixture::new("hatch-live");
    let o = bootctl(
        &[
            "boot",
            "--kernel",
            f.kernel().to_str().unwrap(),
            "--allow-unverified-rv2mbc",
        ],
        &[("UPC_DEMO_MODE", "1")],
    );
    let e = out(&o);
    assert!(!o.status.success(), "{e}");
    assert!(
        e.contains("--allow-unverified-rv2mbc is accepted only with --dry-run"),
        "{e}"
    );
    assert!(!e.contains(NO_OBJECT), "refused before boot_prepare: {e}");
}

// The gate runs before boot_prepare on a live boot and on console.
#[test]
fn live_boot_without_sha_is_refused_before_loading() {
    let f = Fixture::new("live");
    let o = bootctl(
        &["boot", "--kernel", f.kernel().to_str().unwrap()],
        &[("UPC_DEMO_MODE", "1")],
    );
    let e = out(&o);
    assert!(!o.status.success(), "{e}");
    assert!(e.contains("Rv2mbcShaRequired"), "{e}");
    assert!(!e.contains(NO_OBJECT), "{e}");
}

#[test]
fn live_boot_with_sidecar_passes_the_gate() {
    let f = Fixture::new("live-ok");
    f.good_sidecar("xv6-mbc");
    let o = bootctl(
        &["boot", "--kernel", f.kernel().to_str().unwrap()],
        &[("UPC_DEMO_MODE", "1")],
    );
    let e = out(&o);
    assert!(!o.status.success(), "no object, so it stops there: {e}");
    assert!(e.contains(NO_OBJECT), "got past the gate: {e}");
}

#[test]
fn live_boot_without_a_kernel_rv2mbc_is_refused() {
    let f = Fixture::new("live-missing");
    std::fs::remove_file(f.rv2mbc("xv6-mbc")).unwrap();
    let o = bootctl(
        &["boot", "--kernel", f.kernel().to_str().unwrap()],
        &[("UPC_DEMO_MODE", "1")],
    );
    let e = out(&o);
    assert!(!o.status.success(), "{e}");
    assert!(e.contains("Rv2mbcMissing"), "{e}");
    assert!(!e.contains(NO_OBJECT), "{e}");
}

#[test]
fn console_without_sha_is_refused_before_loading() {
    let f = Fixture::new("console");
    let o = bootctl(
        &["console", "--kernel", f.kernel().to_str().unwrap()],
        &[("UPC_DEMO_MODE", "1")],
    );
    let e = out(&o);
    assert!(!o.status.success(), "{e}");
    assert!(e.contains("Rv2mbcShaRequired"), "{e}");
    assert!(!e.contains(NO_OBJECT), "{e}");
}

// The userland program and every resident program are gated too.
#[test]
fn userland_without_sidecar_is_refused() {
    let f = Fixture::new("userland");
    f.good_sidecar("xv6-mbc");
    f.program("init");
    let init = f.dir.join("init.mbc");
    let o = dry_run(&f.kernel(), &["--userland", init.to_str().unwrap()], &[]);
    let e = out(&o);
    assert!(!o.status.success(), "{e}");
    assert!(e.contains("Rv2mbcShaRequired"), "{e}");
    assert!(e.contains(&f.rv2mbc("init").display().to_string()), "{e}");
}

#[test]
fn kernel_flag_does_not_cover_the_userland() {
    let f = Fixture::new("flag-kernel-only");
    f.program("init");
    let init = f.dir.join("init.mbc");
    let o = dry_run(
        &f.kernel(),
        &[
            "--rv2mbc-sha",
            &hex(RV),
            "--userland",
            init.to_str().unwrap(),
        ],
        &[],
    );
    let e = out(&o);
    assert!(!o.status.success(), "{e}");
    assert!(e.contains(&f.rv2mbc("init").display().to_string()), "{e}");
}

#[test]
fn resident_program_without_sidecar_is_refused() {
    let f = Fixture::new("resident");
    f.good_sidecar("xv6-mbc");
    f.program("init");
    f.good_sidecar("init");
    f.program("sh");
    let init = f.dir.join("init.mbc");
    let o = dry_run(&f.kernel(), &["--userland", init.to_str().unwrap()], &[]);
    let e = out(&o);
    assert!(!o.status.success(), "{e}");
    assert!(e.contains("Rv2mbcShaRequired"), "{e}");
    assert!(e.contains(&f.rv2mbc("sh").display().to_string()), "{e}");
}

#[test]
fn resident_program_mismatch_is_refused() {
    let f = Fixture::new("resident-mismatch");
    f.good_sidecar("xv6-mbc");
    f.program("init");
    f.good_sidecar("init");
    f.program("ls");
    f.sidecar("ls", WRONG);
    let init = f.dir.join("init.mbc");
    let o = dry_run(&f.kernel(), &["--userland", init.to_str().unwrap()], &[]);
    let e = out(&o);
    assert!(!o.status.success(), "{e}");
    assert!(e.contains("Rv2mbcShaMismatch"), "{e}");
}

#[test]
fn all_sidecars_present_proceeds() {
    let f = Fixture::new("all-ok");
    f.good_sidecar("xv6-mbc");
    for p in ["init", "sh", "ls"] {
        f.program(p);
        f.good_sidecar(p);
    }
    let init = f.dir.join("init.mbc");
    let o = dry_run(&f.kernel(), &["--userland", init.to_str().unwrap()], &[]);
    assert!(o.status.success(), "{}", out(&o));
}
