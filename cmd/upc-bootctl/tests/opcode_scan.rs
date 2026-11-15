// SPDX-License-Identifier: GPL-3.0-or-later
//! S-STAGE-2C Phase 4, ADR-135 D17: upc-bootctl reads the `--userland` image
//! and every resident program word by word before the eBPF object opens, and
//! refuses the boot when one holds an atomic (XCHG, CAS, LR_W, SC_W) or a
//! privileged return (MRET, SRET). The kernel image is not scanned.
//!
//! `UPC_ALLOW_USER_PRIVRET=1` lets MRET and SRET through for the one probe
//! that runs them at U-mode on purpose (`hostile_reply`); the interpreter
//! refuses both at priv 3. Nothing lets an atomic through.
//!
//! These tests never reach BPF: every run is `--dry-run` with
//! `MONAD_CPU_EBPF_OBJ` pointing at a path that does not exist.

use monad_common::mbc_opcodes as op;
use std::path::{Path, PathBuf};
use std::process::{Command, Output};

const RV: &[u8] = &[0x00, 0x40, 0x00, 0x00, 0x04, 0x40, 0x00, 0x00];

fn insn(opcode: u8, dst: u32, src: u32, imm: u32) -> u32 {
    ((opcode as u32) << 24) | (dst << 20) | (src << 16) | imm
}

/// A temp dir with a kernel that holds MRET and SRET, as the real one does.
struct Fixture {
    dir: PathBuf,
}

impl Fixture {
    fn new(tag: &str) -> Self {
        let dir = std::env::temp_dir().join(format!("upc-opscan-{tag}-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&dir);
        std::fs::create_dir_all(&dir).unwrap();
        let f = Fixture { dir };
        let mut kernel = vec![0u32; 16];
        kernel[3] = insn(op::MRET, 0, 0, 0);
        kernel[9] = insn(op::SRET, 0, 0, 0);
        f.image("xv6-mbc", &kernel);
        f
    }
    fn path(&self, name: &str) -> PathBuf {
        self.dir.join(format!("{name}.mbc"))
    }
    /// `<name>.mbc` with these words, plus a `.rv2mbc` (unverified: dry run).
    fn image(&self, name: &str, words: &[u32]) {
        let bytes: Vec<u8> = words.iter().flat_map(|w| w.to_le_bytes()).collect();
        std::fs::write(self.path(name), bytes).unwrap();
        std::fs::write(self.dir.join(format!("{name}.rv2mbc")), RV).unwrap();
    }
    /// A clean image with one word replaced.
    fn image_with(&self, name: &str, at: usize, word: u32) {
        let mut words = vec![insn(op::LD, 1, 2, 4); 8];
        words[at] = word;
        self.image(name, &words);
    }
}

impl Drop for Fixture {
    fn drop(&mut self) {
        std::fs::remove_dir_all(&self.dir).ok();
    }
}

fn dry_run(kernel: &Path, userland: &Path, envs: &[(&str, &str)]) -> Output {
    let mut cmd = Command::new(env!("CARGO_BIN_EXE_upc-bootctl"));
    cmd.args([
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
    .env_remove("UPC_ALLOW_USER_PRIVRET")
    .env("MONAD_CPU_EBPF_OBJ", "/nonexistent/monad-cpu-ebpf");
    for (k, v) in envs {
        cmd.env(k, v);
    }
    cmd.output().expect("run upc-bootctl")
}

fn out(o: &Output) -> String {
    format!(
        "{}{}",
        String::from_utf8_lossy(&o.stdout),
        String::from_utf8_lossy(&o.stderr)
    )
}

#[test]
fn a_clean_user_image_passes_beside_a_kernel_with_mret_and_sret() {
    let f = Fixture::new("clean");
    f.image_with("init", 0, insn(op::LD, 3, 4, 8));
    let o = dry_run(&f.path("xv6-mbc"), &f.path("init"), &[]);
    let e = out(&o);
    assert!(o.status.success(), "{e}");
    assert!(!e.contains("UserImageOpcode"), "{e}");
}

#[test]
fn each_forbidden_opcode_in_the_user_image_is_refused_and_named() {
    for (opcode, name) in [
        (op::XCHG, "XCHG"),
        (op::CAS, "CAS"),
        (op::MRET, "MRET"),
        (op::SRET, "SRET"),
        (op::LR_W, "LR_W"),
        (op::SC_W, "SC_W"),
    ] {
        let f = Fixture::new(name);
        f.image_with("init", 5, insn(opcode, 1, 2, 0));
        let o = dry_run(&f.path("xv6-mbc"), &f.path("init"), &[]);
        let e = out(&o);
        assert!(!o.status.success(), "{name} must refuse: {e}");
        assert!(e.contains("UserImageOpcode"), "{name}: {e}");
        assert!(e.contains(name), "{name} not named: {e}");
        assert!(e.contains("word 5"), "{name}: word not named: {e}");
        assert!(e.contains("init"), "{name}: image not named: {e}");
    }
}

#[test]
fn a_resident_program_with_xchg_is_refused() {
    let f = Fixture::new("resident");
    f.image_with("init", 0, insn(op::LD, 3, 4, 8));
    f.image_with("sh", 2, insn(op::XCHG, 13, 13, 0));
    let o = dry_run(&f.path("xv6-mbc"), &f.path("init"), &[]);
    let e = out(&o);
    assert!(!o.status.success(), "{e}");
    assert!(
        e.contains("UserImageOpcode") && e.contains("sh") && e.contains("XCHG"),
        "{e}"
    );
}

#[test]
fn the_probe_hatch_admits_privileged_returns_and_never_an_atomic() {
    let hatch = [("UPC_ALLOW_USER_PRIVRET", "1")];

    let f = Fixture::new("hatch-ret");
    f.image_with("init", 1, insn(op::MRET, 0, 0, 0));
    f.image_with("sh", 1, insn(op::SRET, 0, 0, 0));
    let o = dry_run(&f.path("xv6-mbc"), &f.path("init"), &hatch);
    let e = out(&o);
    assert!(o.status.success(), "{e}");
    assert!(
        e.contains("UPC_ALLOW_USER_PRIVRET"),
        "the hatch must be said out loud: {e}"
    );

    for (opcode, name) in [
        (op::XCHG, "XCHG"),
        (op::CAS, "CAS"),
        (op::LR_W, "LR_W"),
        (op::SC_W, "SC_W"),
    ] {
        let f = Fixture::new(&format!("hatch-{name}"));
        f.image_with("init", 1, insn(opcode, 0, 0, 0));
        let o = dry_run(&f.path("xv6-mbc"), &f.path("init"), &hatch);
        let e = out(&o);
        assert!(
            !o.status.success(),
            "{name} with the hatch must still refuse: {e}"
        );
        assert!(e.contains("UserImageOpcode") && e.contains(name), "{e}");
    }

    // Any other value of the variable is not the hatch.
    let f = Fixture::new("hatch-other");
    f.image_with("init", 1, insn(op::SRET, 0, 0, 0));
    let o = dry_run(
        &f.path("xv6-mbc"),
        &f.path("init"),
        &[("UPC_ALLOW_USER_PRIVRET", "yes")],
    );
    assert!(!o.status.success(), "{}", out(&o));
}
