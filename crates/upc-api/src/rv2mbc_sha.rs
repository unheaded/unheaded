// SPDX-License-Identifier: GPL-3.0-or-later
//! The `.rv2mbc` SHA-256 integrity gate (ADR-075 §Security #1; default-on
//! since S-ASCEND-2 Item 5). One resolver for both loaders.
//!
//! A `.rv2mbc` file maps RV word addresses to MBC PCs for every indirect branch
//! and privileged return, so a tampered one redirects control flow anywhere in
//! the image. A loader therefore verifies every `.rv2mbc` it puts into
//! `RV2MBC_MAP`, before the first map write. The expected hash is, in order:
//!
//! 1. an explicit value the operator passed (`--rv2mbc-sha`, or the
//!    `UPC_RV2MBC_SHA` environment variable upc-bootctl has read since Phase
//!    1.3): 64 hex digits, case-insensitive, surrounding whitespace ignored;
//! 2. the sibling sidecar `<file>.rv2mbc.sha256` the Makefiles write
//!    (`sha256sum` format: the first whitespace-separated token, 64 lowercase
//!    hex digits);
//! 3. otherwise the load is refused ([`Rv2mbcShaError::Required`]).
//!
//! The hash is SHA-256 over the file's on-disk bytes, computed with
//! [`crate::surface_hash::sha256`] (this crate stays dependency-free).
//! Loaders verify the same bytes they then write: [`read_verified`] reads the
//! file once and returns those bytes.

use std::path::{Path, PathBuf};

use crate::object::hex32;
use crate::surface_hash::sha256;

/// Where the expected hash came from.
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum ShaSource {
    /// The operator passed it (flag or environment).
    Explicit,
    /// Read from this sidecar file.
    Sidecar(PathBuf),
}

/// A `.rv2mbc` file whose bytes matched the expected SHA-256.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Verified {
    /// The file's bytes, as read and hashed.
    pub bytes: Vec<u8>,
    /// Their SHA-256 (equal to the expected hash).
    pub sha256: [u8; 32],
    /// Where the expected hash came from.
    pub source: ShaSource,
}

/// Why the gate refused a `.rv2mbc` file.
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum Rv2mbcShaError {
    /// No explicit hash and no sidecar.
    Required {
        /// The `.rv2mbc` file.
        path: PathBuf,
        /// The sidecar that was looked for.
        sidecar: PathBuf,
    },
    /// The file's SHA-256 is not the expected one.
    Mismatch {
        /// The `.rv2mbc` file.
        path: PathBuf,
        /// Expected hash, lowercase hex.
        expected: String,
        /// Computed hash, lowercase hex.
        actual: String,
    },
    /// The explicit value or the sidecar is not a SHA-256 in hex.
    Malformed {
        /// "explicit value" or the sidecar path.
        origin: String,
        /// What was found (truncated to 80 chars).
        value: String,
    },
    /// The `.rv2mbc` file or its sidecar could not be read.
    Io {
        /// The file that failed.
        path: PathBuf,
        /// The OS error.
        error: String,
    },
}

impl core::fmt::Display for Rv2mbcShaError {
    fn fmt(&self, f: &mut core::fmt::Formatter<'_>) -> core::fmt::Result {
        match self {
            Rv2mbcShaError::Required { path, sidecar } => write!(
                f,
                "Rv2mbcShaRequired: {} has no expected SHA-256; pass --rv2mbc-sha <hex> \
                 (kernel) or create {} (`sha256sum {} | cut -d' ' -f1 > {}`)",
                path.display(),
                sidecar.display(),
                path.display(),
                sidecar.display()
            ),
            Rv2mbcShaError::Mismatch {
                path,
                expected,
                actual,
            } => write!(
                f,
                "Rv2mbcShaMismatch: {}: expected sha256 {expected}, computed {actual}",
                path.display()
            ),
            Rv2mbcShaError::Malformed { origin, value } => write!(
                f,
                "Rv2mbcShaMalformed: {origin} is not a 64-digit hex SHA-256: {value:?}"
            ),
            Rv2mbcShaError::Io { path, error } => {
                write!(f, "Rv2mbcShaIo: read {}: {error}", path.display())
            }
        }
    }
}

impl std::error::Error for Rv2mbcShaError {}

/// The sidecar for `rv2mbc`: the same path with `.sha256` appended
/// (`xv6-mbc.rv2mbc` → `xv6-mbc.rv2mbc.sha256`).
pub fn sidecar_path(rv2mbc: &Path) -> PathBuf {
    let mut s = rv2mbc.as_os_str().to_owned();
    s.push(".sha256");
    PathBuf::from(s)
}

/// Parse exactly 64 hex digits (`[0-9a-f]`, plus `[A-F]` when `allow_upper`).
fn parse_hex64(s: &str, allow_upper: bool) -> Option<[u8; 32]> {
    let b = s.as_bytes();
    if b.len() != 64 {
        return None;
    }
    let nib = |c: u8| match c {
        b'0'..=b'9' => Some(c - b'0'),
        b'a'..=b'f' => Some(c - b'a' + 10),
        b'A'..=b'F' if allow_upper => Some(c - b'A' + 10),
        _ => None,
    };
    let mut out = [0u8; 32];
    for (i, pair) in b.as_chunks::<2>().0.iter().enumerate() {
        out[i] = (nib(pair[0])? << 4) | nib(pair[1])?;
    }
    Some(out)
}

fn malformed(origin: String, value: &str) -> Rv2mbcShaError {
    Rv2mbcShaError::Malformed {
        origin,
        value: value.chars().take(80).collect(),
    }
}

/// Resolve the expected SHA-256 for `rv2mbc`: `explicit`, else its sidecar,
/// else [`Rv2mbcShaError::Required`]. Does not read `rv2mbc` itself.
pub fn expected_sha256(
    rv2mbc: &Path,
    explicit: Option<&str>,
) -> Result<([u8; 32], ShaSource), Rv2mbcShaError> {
    if let Some(v) = explicit {
        let t = v.trim();
        return parse_hex64(t, true)
            .map(|h| (h, ShaSource::Explicit))
            .ok_or_else(|| malformed("explicit value".to_string(), t));
    }
    let sidecar = sidecar_path(rv2mbc);
    let text = match std::fs::read_to_string(&sidecar) {
        Ok(t) => t,
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => {
            return Err(Rv2mbcShaError::Required {
                path: rv2mbc.to_path_buf(),
                sidecar,
            })
        }
        Err(e) => {
            return Err(Rv2mbcShaError::Io {
                path: sidecar,
                error: e.to_string(),
            })
        }
    };
    let token = text.split_whitespace().next().unwrap_or("");
    match parse_hex64(token, false) {
        Some(h) => Ok((h, ShaSource::Sidecar(sidecar))),
        None => Err(malformed(sidecar.display().to_string(), token)),
    }
}

/// Check `bytes` (the contents of `rv2mbc`) against `expected`.
pub fn verify(rv2mbc: &Path, bytes: &[u8], expected: &[u8; 32]) -> Result<(), Rv2mbcShaError> {
    let actual = sha256(bytes);
    if &actual == expected {
        Ok(())
    } else {
        Err(Rv2mbcShaError::Mismatch {
            path: rv2mbc.to_path_buf(),
            expected: hex32(expected),
            actual: hex32(&actual),
        })
    }
}

/// Resolve the expected hash ([`expected_sha256`]), then read `rv2mbc` once and
/// [`verify`] it. The returned bytes are the ones that were hashed; a loader
/// writes those and does not read the file again.
pub fn read_verified(rv2mbc: &Path, explicit: Option<&str>) -> Result<Verified, Rv2mbcShaError> {
    let (expected, source) = expected_sha256(rv2mbc, explicit)?;
    let bytes = std::fs::read(rv2mbc).map_err(|e| Rv2mbcShaError::Io {
        path: rv2mbc.to_path_buf(),
        error: e.to_string(),
    })?;
    verify(rv2mbc, &bytes, &expected)?;
    Ok(Verified {
        bytes,
        sha256: expected,
        source,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn hex64_round_trips_hex32() {
        let h: [u8; 32] = core::array::from_fn(|i| (i * 7) as u8);
        assert_eq!(parse_hex64(&hex32(&h), false), Some(h));
        assert_eq!(parse_hex64(&hex32(&h).to_ascii_uppercase(), true), Some(h));
        assert_eq!(parse_hex64(&hex32(&h).to_ascii_uppercase(), false), None);
    }

    #[test]
    fn hex64_rejects_wrong_lengths_and_non_hex() {
        assert_eq!(parse_hex64("", true), None);
        assert_eq!(parse_hex64(&"a".repeat(63), true), None);
        assert_eq!(parse_hex64(&"a".repeat(65), true), None);
        assert_eq!(parse_hex64(&"g".repeat(64), true), None);
        // a multi-byte char must not be split or accepted
        assert_eq!(parse_hex64(&format!("{}é", "a".repeat(62)), true), None);
    }
}
