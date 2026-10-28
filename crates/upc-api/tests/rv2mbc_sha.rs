// SPDX-License-Identifier: GPL-3.0-or-later
//! The `.rv2mbc` SHA-256 gate (ADR-075 §Security #1, S-ASCEND-2 Item 5): one
//! resolver both loaders use. The expected hash comes from an explicit value,
//! else the sibling `<file>.rv2mbc.sha256` sidecar, else the load is refused.

use std::path::{Path, PathBuf};
use upc_api::object::hex32;
use upc_api::rv2mbc_sha::{
    expected_sha256, read_verified, sidecar_path, verify, Rv2mbcShaError, ShaSource,
};
use upc_api::surface_hash::sha256;

const BYTES: &[u8] = &[0x00, 0x40, 0x00, 0x00, 0x04, 0x40, 0x00, 0x00];

/// A fresh directory per test, holding `prog.rv2mbc` with [`BYTES`].
fn fixture(tag: &str) -> (PathBuf, PathBuf) {
    let dir = std::env::temp_dir().join(format!("upc-api-rv2mbc-{tag}-{}", std::process::id()));
    let _ = std::fs::remove_dir_all(&dir);
    std::fs::create_dir_all(&dir).unwrap();
    let rv = dir.join("prog.rv2mbc");
    std::fs::write(&rv, BYTES).unwrap();
    (dir, rv)
}

fn good_hex() -> String {
    hex32(&sha256(BYTES))
}

const WRONG: &str = "0000000000000000000000000000000000000000000000000000000000000000";

fn write_sidecar(rv: &Path, text: &str) {
    std::fs::write(sidecar_path(rv), text).unwrap();
}

#[test]
fn sidecar_is_the_rv2mbc_path_plus_sha256() {
    assert_eq!(
        sidecar_path(Path::new("t/xv6-mbc.rv2mbc")),
        PathBuf::from("t/xv6-mbc.rv2mbc.sha256")
    );
}

#[test]
fn no_explicit_value_and_no_sidecar_is_required() {
    let (dir, rv) = fixture("required");
    let err = expected_sha256(&rv, None).unwrap_err();
    assert!(matches!(err, Rv2mbcShaError::Required { .. }), "{err:?}");
    let msg = err.to_string();
    assert!(msg.contains("Rv2mbcShaRequired"), "{msg}");
    assert!(msg.contains(&rv.display().to_string()), "{msg}");
    assert!(
        msg.contains(&sidecar_path(&rv).display().to_string()),
        "{msg}"
    );
    // read_verified refuses the same way (before reading the file).
    assert!(matches!(
        read_verified(&rv, None),
        Err(Rv2mbcShaError::Required { .. })
    ));
    std::fs::remove_dir_all(dir).ok();
}

#[test]
fn sha256sum_format_sidecar_verifies() {
    let (dir, rv) = fixture("sha256sum");
    write_sidecar(&rv, &format!("{}  prog.rv2mbc\n", good_hex()));
    let v = read_verified(&rv, None).unwrap();
    assert_eq!(v.bytes, BYTES);
    assert_eq!(v.sha256, sha256(BYTES));
    assert_eq!(v.source, ShaSource::Sidecar(sidecar_path(&rv)));
    std::fs::remove_dir_all(dir).ok();
}

#[test]
fn bare_hash_sidecar_verifies() {
    let (dir, rv) = fixture("bare");
    write_sidecar(&rv, &format!("{}\n", good_hex()));
    assert_eq!(read_verified(&rv, None).unwrap().sha256, sha256(BYTES));
    std::fs::remove_dir_all(dir).ok();
}

#[test]
fn wrong_sidecar_is_a_mismatch_naming_both_hashes() {
    let (dir, rv) = fixture("mismatch");
    write_sidecar(&rv, WRONG);
    let err = read_verified(&rv, None).unwrap_err();
    assert!(matches!(err, Rv2mbcShaError::Mismatch { .. }), "{err:?}");
    let msg = err.to_string();
    assert!(msg.contains("Rv2mbcShaMismatch"), "{msg}");
    assert!(msg.contains(WRONG), "{msg}");
    assert!(msg.contains(&good_hex()), "{msg}");
    assert!(msg.contains(&rv.display().to_string()), "{msg}");
    std::fs::remove_dir_all(dir).ok();
}

#[test]
fn explicit_value_wins_over_the_sidecar() {
    let (dir, rv) = fixture("explicit");
    write_sidecar(&rv, WRONG);
    let v = read_verified(&rv, Some(&good_hex())).unwrap();
    assert_eq!(v.source, ShaSource::Explicit);

    // and a wrong explicit value is a mismatch even with a correct sidecar.
    write_sidecar(&rv, &good_hex());
    assert!(matches!(
        read_verified(&rv, Some(WRONG)),
        Err(Rv2mbcShaError::Mismatch { .. })
    ));
    std::fs::remove_dir_all(dir).ok();
}

#[test]
fn explicit_value_is_case_insensitive_and_trimmed() {
    let (dir, rv) = fixture("upper");
    let upper = format!(" {}\n", good_hex().to_ascii_uppercase());
    assert!(read_verified(&rv, Some(&upper)).is_ok());
    std::fs::remove_dir_all(dir).ok();
}

#[test]
fn malformed_values_are_refused() {
    let (dir, rv) = fixture("malformed");
    let short = &good_hex()[..63];
    for bad in ["", "   \n", short, "zz", &good_hex().to_ascii_uppercase()] {
        write_sidecar(&rv, bad);
        let err = expected_sha256(&rv, None).unwrap_err();
        assert!(
            matches!(err, Rv2mbcShaError::Malformed { .. }),
            "sidecar {bad:?}: {err:?}"
        );
        assert!(err.to_string().contains("Rv2mbcShaMalformed"));
    }
    for bad in ["", short, "not-hex"] {
        assert!(
            matches!(
                expected_sha256(&rv, Some(bad)),
                Err(Rv2mbcShaError::Malformed { .. })
            ),
            "explicit {bad:?}"
        );
    }
    std::fs::remove_dir_all(dir).ok();
}

#[test]
fn verify_checks_the_given_bytes() {
    let rv = Path::new("x.rv2mbc");
    assert_eq!(verify(rv, BYTES, &sha256(BYTES)), Ok(()));
    assert!(matches!(
        verify(rv, b"tampered", &sha256(BYTES)),
        Err(Rv2mbcShaError::Mismatch { .. })
    ));
}

#[test]
fn missing_rv2mbc_with_a_sidecar_is_an_io_error() {
    let (dir, rv) = fixture("io");
    write_sidecar(&rv, &good_hex());
    std::fs::remove_file(&rv).unwrap();
    assert!(matches!(
        read_verified(&rv, None),
        Err(Rv2mbcShaError::Io { .. })
    ));
    std::fs::remove_dir_all(dir).ok();
}
