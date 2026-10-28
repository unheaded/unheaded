// SPDX-License-Identifier: GPL-3.0-or-later
//! S-ASCEND-2 Step 159 (live): after a boot's image load, a userspace update of
//! ROM_MAP or RV2MBC_MAP through the loader's own handle is refused with EPERM.
//!
//! Ignored by default and skipped unless `UPC_LIVE=1`. It needs root (or the
//! four capabilities), the `upc0` namespace (`scripts/upc-netns.sh up`), the
//! ascend object (`--features ascend-linux`) and the xv6 images. Run the test
//! binary inside the namespace:
//!
//! ```text
//! T=$(cargo test --release --test freeze_live --no-run 2>&1 \
//!     | grep -oE 'target/release/deps/freeze_live-[0-9a-f]+' | head -1)
//! sudo ip netns exec upc0 env UPC_LIVE=1 "$T" --ignored --nocapture
//! ```
//!
//! `UPC_K` overrides the image directory, `MONAD_CPU_EBPF_OBJ` the object.

use std::path::PathBuf;
use std::process::Command;

fn repo() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../..")
}

#[test]
#[ignore = "live: needs root, upc0 and the xv6 images; set UPC_LIVE=1"]
fn rom_and_rv2mbc_refuse_userspace_updates_after_load() {
    if std::env::var("UPC_LIVE").as_deref() != Ok("1") {
        eprintln!("skipped: UPC_LIVE is not 1");
        return;
    }
    let k = std::env::var_os("UPC_K")
        .map(PathBuf::from)
        .unwrap_or_else(|| repo().join("crates/xv6-mbc/upstream/target"));
    let obj = std::env::var_os("MONAD_CPU_EBPF_OBJ")
        .map(PathBuf::from)
        .unwrap_or_else(|| repo().join("ebpf/target/bpfel-unknown-none/release/monad-cpu-ebpf"));
    let out = Command::new(env!("CARGO_BIN_EXE_upc-bootctl"))
        .env("UPC_DEMO_MODE", "1")
        .env("MONAD_CPU_EBPF_OBJ", &obj)
        .args(["boot", "--kernel"])
        .arg(k.join("xv6-mbc.mbc"))
        .arg("--ramdisk")
        .arg(k.join("fs.img"))
        .arg("--userland")
        .arg(k.join("init.mbc"))
        .args([
            "--triggers",
            "3000",
            "--instance",
            "111",
            "--input",
            "echo\n",
            "--freeze-check",
        ])
        .output()
        .expect("run upc-bootctl");
    let stdout = String::from_utf8_lossy(&out.stdout);
    let stderr = String::from_utf8_lossy(&out.stderr);
    assert!(out.status.success(), "boot failed\n{stdout}\n{stderr}");
    for map in ["ROM_MAP", "RV2MBC_MAP"] {
        let line = format!("freeze check: {map} update refused (EPERM)");
        assert!(stdout.contains(&line), "missing `{line}`\n{stdout}");
    }
    assert!(
        stdout.contains("FIRST HEARTBEAT"),
        "the guest must still run on frozen maps\n{stdout}"
    );
}
