// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.
//! EdDSA service tokens for Rust binaries (ADR-103 §2, §6).
//!
//! The same tokens Go's `pkg/auth` mints with `ServiceTokenMinter.Mint`,
//! byte for byte (Ed25519 signatures are deterministic):
//!
//! - header `{"alg":"EdDSA","typ":"JWT","kid":"<kid>"}`
//! - claims `{"sub":<name>,"iss":"unheaded","aud":<target>,"exp":<iat+300>,
//!   "iat":<now>,"roles":["service"]}`
//! - compact JWS, base64url without padding
//! - kid = the first 16 hex characters of SHA-256 over the 32-byte public key
//! - key file = one `UNHEADED ED25519 SEED` PEM block (the 32-byte RFC 8032
//!   seed) as `unheaded-keygen` writes it; refused when group or others can
//!   read or write it, like `auth.LoadPrivateKey`.
//!
//! [`load_identity`] is the Rust form of `wotanClient.UseServiceIdentity`:
//! `AUTH_KEY_FILE`, else `/etc/unheaded/keys/<name>.key`; a missing key is
//! [`Identity::Missing`] (the caller logs one warning and runs without a
//! token), any other key problem is an error.

use std::collections::HashMap;
use std::fmt;
use std::fs::File;
use std::io::Read;
use std::os::unix::fs::PermissionsExt;
use std::path::{Path, PathBuf};
use std::sync::Mutex;
use std::time::{SystemTime, UNIX_EPOCH};

use base64::engine::general_purpose::{STANDARD, URL_SAFE_NO_PAD};
use base64::Engine as _;
use ring::signature::{Ed25519KeyPair, KeyPair as _};

/// The "iss" of every service token (`auth.EdDSAServiceIssuer`).
pub const ISSUER: &str = "unheaded";
/// The "aud" of a token for Wotan (`wotanClient.WotanAudience`).
pub const WOTAN_AUDIENCE: &str = "wotan";
/// Lifetime of a minted token in seconds (`auth.EdDSAServiceTokenTTL`).
pub const TOKEN_TTL_SECS: u64 = 300;
/// A cached token is replaced this many seconds before it expires.
pub const REFRESH_SECS: u64 = 30;
/// Where a provisioned host keeps service keys (`auth.DefaultKeyDir`).
pub const DEFAULT_KEY_DIR: &str = "/etc/unheaded/keys";
/// The environment variable naming the key file (`AUTH_KEY_FILE`).
pub const KEY_FILE_ENV: &str = "AUTH_KEY_FILE";

const SEED_PEM_TYPE: &str = "UNHEADED ED25519 SEED";
const MAX_KEY_FILE_BYTES: u64 = 4096;
const MAX_KEY_NAME_LEN: usize = 64;
const SEED_LEN: usize = 32;

/// Why a key could not be loaded or a token minted.
#[derive(Debug)]
pub enum Error {
    /// Opening or reading the key file failed.
    Io(PathBuf, std::io::Error),
    /// The key file is readable or writable by group or others.
    Permissions(PathBuf, u32),
    /// The key file is not a regular file.
    NotRegular(PathBuf),
    /// The key file is not exactly one `UNHEADED ED25519 SEED` PEM block.
    Format(PathBuf, &'static str),
    /// A service or audience name that is not a valid key name.
    BadName(String),
    /// ring refused the seed (cannot happen for 32 bytes).
    Key,
}

impl fmt::Display for Error {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Error::Io(p, e) => write!(f, "service key {}: {}", p.display(), e),
            Error::Permissions(p, m) => write!(
                f,
                "service key {} is {:04o}: group or others can access it (want 0400 or 0600)",
                p.display(),
                m
            ),
            Error::NotRegular(p) => write!(f, "service key {} is not a regular file", p.display()),
            Error::Format(p, why) => write!(
                f,
                "service key {} is not an {} PEM file: {}",
                p.display(),
                SEED_PEM_TYPE,
                why
            ),
            Error::BadName(n) => write!(
                f,
                "bad key name {n:?} (want [a-z0-9][a-z0-9_-]*, at most {MAX_KEY_NAME_LEN} bytes)"
            ),
            Error::Key => write!(f, "Ed25519 seed refused"),
        }
    }
}

impl std::error::Error for Error {}

/// `auth.ValidKeyName`: lower-case letters, digits, '-' and '_', starting
/// with a letter or digit, at most 64 bytes.
pub fn valid_key_name(name: &str) -> bool {
    if name.is_empty() || name.len() > MAX_KEY_NAME_LEN {
        return false;
    }
    name.bytes().enumerate().all(|(i, c)| {
        c.is_ascii_lowercase() || c.is_ascii_digit() || ((c == b'-' || c == b'_') && i > 0)
    })
}

/// `auth.KeyID`: the first 16 hex characters of SHA-256 over the raw key.
pub fn key_id(public_key: &[u8]) -> String {
    let sum = ring::digest::digest(&ring::digest::SHA256, public_key);
    let mut s = String::with_capacity(16);
    for b in &sum.as_ref()[..8] {
        s.push_str(&format!("{b:02x}"));
    }
    s
}

/// Parses a key file's bytes: exactly one `UNHEADED ED25519 SEED` block, no
/// PEM headers, a 32-byte body, nothing but whitespace around it.
pub fn parse_seed_pem(path: &Path, raw: &[u8]) -> Result<[u8; SEED_LEN], Error> {
    let fmt_err = |why| Error::Format(path.to_path_buf(), why);
    let text = std::str::from_utf8(raw).map_err(|_| fmt_err("not text"))?;
    let begin = format!("-----BEGIN {SEED_PEM_TYPE}-----");
    let end = format!("-----END {SEED_PEM_TYPE}-----");
    let lines: Vec<&str> = text.trim().lines().map(str::trim_end).collect();
    if lines.len() < 3 || lines[0] != begin || lines[lines.len() - 1] != end {
        return Err(fmt_err("missing or wrong BEGIN/END lines"));
    }
    let body = &lines[1..lines.len() - 1];
    if body.iter().any(|l| l.contains(':') || l.contains("-----")) {
        return Err(fmt_err("PEM headers or a second block"));
    }
    let seed = STANDARD
        .decode(body.concat())
        .map_err(|_| fmt_err("body is not base64"))?;
    seed.try_into().map_err(|_| fmt_err("seed is not 32 bytes"))
}

/// Reads a private key file like `auth.LoadPrivateKey`: a regular file
/// that group and others cannot access, at most 4096 bytes, one seed block.
pub fn read_key_file(path: &Path) -> Result<[u8; SEED_LEN], Error> {
    let f = File::open(path).map_err(|e| Error::Io(path.to_path_buf(), e))?;
    // Stat the open file, not the path, so the check is about what is read.
    let meta = f.metadata().map_err(|e| Error::Io(path.to_path_buf(), e))?;
    if !meta.file_type().is_file() {
        return Err(Error::NotRegular(path.to_path_buf()));
    }
    let mode = meta.permissions().mode() & 0o7777;
    if mode & 0o077 != 0 {
        return Err(Error::Permissions(path.to_path_buf(), mode));
    }
    let mut raw = Vec::new();
    f.take(MAX_KEY_FILE_BYTES + 1)
        .read_to_end(&mut raw)
        .map_err(|e| Error::Io(path.to_path_buf(), e))?;
    if raw.len() as u64 > MAX_KEY_FILE_BYTES {
        return Err(Error::Format(path.to_path_buf(), "larger than 4096 bytes"));
    }
    parse_seed_pem(path, &raw)
}

fn b64(data: &[u8]) -> String {
    URL_SAFE_NO_PAD.encode(data)
}

fn unix_now() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_secs())
        .unwrap_or(0)
}

/// This service's token minter: `auth.ServiceTokenMinter` in Rust. Tokens
/// are cached per audience and replaced 30 s before they expire. Safe to
/// share between threads.
pub struct Minter {
    name: String,
    key: Ed25519KeyPair,
    kid: String,
    cache: Mutex<HashMap<String, (String, u64)>>,
}

impl fmt::Debug for Minter {
    // Never prints the key.
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("Minter")
            .field("name", &self.name)
            .field("kid", &self.kid)
            .finish()
    }
}

impl Minter {
    /// A minter for service `name` from its 32-byte Ed25519 seed.
    pub fn from_seed(name: &str, seed: &[u8; SEED_LEN]) -> Result<Self, Error> {
        if !valid_key_name(name) {
            return Err(Error::BadName(name.to_string()));
        }
        let key = Ed25519KeyPair::from_seed_unchecked(seed).map_err(|_| Error::Key)?;
        let kid = key_id(key.public_key().as_ref());
        Ok(Self {
            name: name.to_string(),
            key,
            kid,
            cache: Mutex::new(HashMap::new()),
        })
    }

    /// `auth.LoadServiceTokenMinter`: reads the key file (see
    /// [`read_key_file`]) and returns the minter for `name`.
    pub fn load(name: &str, path: &Path) -> Result<Self, Error> {
        Self::from_seed(name, &read_key_file(path)?)
    }

    /// The service name tokens are minted for (their "sub").
    pub fn name(&self) -> &str {
        &self.name
    }

    /// The key id in every token's header.
    pub fn kid(&self) -> &str {
        &self.kid
    }

    /// The raw 32-byte public key.
    pub fn public_key(&self) -> &[u8] {
        self.key.public_key().as_ref()
    }

    /// Mints a fresh token for `aud` issued at `iat` (Unix seconds), not
    /// cached. `aud` is a service identity, so it is held to the key-name
    /// rules: nothing in the claims needs JSON escaping.
    pub fn mint_at(&self, aud: &str, iat: u64) -> Result<String, Error> {
        if !valid_key_name(aud) {
            return Err(Error::BadName(aud.to_string()));
        }
        // Field order and spelling are Go's json.Marshal of JWTHeader and
        // JWTClaims (nbf, extra and act are omitempty and unset).
        let header = format!(r#"{{"alg":"EdDSA","typ":"JWT","kid":"{}"}}"#, self.kid);
        let claims = format!(
            r#"{{"sub":"{}","iss":"{}","aud":"{}","exp":{},"iat":{},"roles":["service"]}}"#,
            self.name,
            ISSUER,
            aud,
            iat + TOKEN_TTL_SECS,
            iat
        );
        let input = format!("{}.{}", b64(header.as_bytes()), b64(claims.as_bytes()));
        let sig = self.key.sign(input.as_bytes());
        Ok(format!("{}.{}", input, b64(sig.as_ref())))
    }

    /// A token for `aud`, from the cache while it has more than 30 s left.
    pub fn mint(&self, aud: &str) -> Result<String, Error> {
        self.mint_cached(aud, unix_now())
    }

    fn mint_cached(&self, aud: &str, now: u64) -> Result<String, Error> {
        let mut cache = self.cache.lock().unwrap_or_else(|p| p.into_inner());
        if let Some((tok, exp)) = cache.get(aud) {
            if now + REFRESH_SECS < *exp {
                return Ok(tok.clone());
            }
        }
        let tok = self.mint_at(aud, now)?;
        if cache.len() >= 256 {
            cache.clear();
        }
        cache.insert(aud.to_string(), (tok.clone(), now + TOKEN_TTL_SECS));
        Ok(tok)
    }

    /// `Bearer <token>` for `aud`, the value of an Authorization header or
    /// of gRPC "authorization" metadata.
    pub fn authorization(&self, aud: &str) -> Result<String, Error> {
        Ok(format!("Bearer {}", self.mint(aud)?))
    }
}

/// What [`load_identity`] found.
#[derive(Debug)]
pub enum Identity {
    /// The key loaded: present its tokens.
    Loaded(Minter),
    /// No key file at this path: run without a token (a Wotan with
    /// AUTH_ENABLED=true refuses the calls; run `unheaded-keygen <name>`).
    Missing(PathBuf),
}

/// The key file for `name`: `AUTH_KEY_FILE`, else
/// `/etc/unheaded/keys/<name>.key`.
pub fn key_file_for(name: &str) -> PathBuf {
    match std::env::var_os(KEY_FILE_ENV) {
        Some(p) if !p.is_empty() => PathBuf::from(p),
        _ => Path::new(DEFAULT_KEY_DIR).join(format!("{name}.key")),
    }
}

/// `wotanClient.UseServiceIdentity` in Rust: loads `name`'s key from
/// [`key_file_for`]. A missing file is `Ok(Identity::Missing)`; a key that
/// is there but unusable (mode, format) is an error, a misconfiguration
/// rather than a host before unheaded-keygen.
pub fn load_identity(name: &str) -> Result<Identity, Error> {
    load_identity_from(name, &key_file_for(name))
}

/// [`load_identity`] with an explicit key path.
pub fn load_identity_from(name: &str, path: &Path) -> Result<Identity, Error> {
    match Minter::load(name, path) {
        Ok(m) => Ok(Identity::Loaded(m)),
        Err(Error::Io(p, e)) if e.kind() == std::io::ErrorKind::NotFound => {
            Ok(Identity::Missing(p))
        }
        Err(e) => Err(e),
    }
}

/// The one-line warning for a missing key, worded like the Go clients'.
pub fn missing_key_warning(name: &str, path: &Path) -> String {
    format!(
        "no service key at {}; Wotan calls carry no token as {:?} (a Wotan with AUTH_ENABLED=true refuses them; run unheaded-keygen {}, ADR-103)",
        path.display(),
        name,
        name
    )
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Write;
    use std::os::unix::fs::OpenOptionsExt;

    fn seed() -> [u8; 32] {
        // A test seed made at run time, never a literal key.
        let mut s = [0u8; 32];
        for (i, b) in s.iter_mut().enumerate() {
            *b = (i as u8).wrapping_mul(37).wrapping_add(11);
        }
        s
    }

    fn pem(seed: &[u8]) -> String {
        format!(
            "-----BEGIN {SEED_PEM_TYPE}-----\n{}\n-----END {SEED_PEM_TYPE}-----\n",
            STANDARD.encode(seed)
        )
    }

    fn tmpdir(tag: &str) -> PathBuf {
        let d = std::env::temp_dir().join(format!(
            "svc-token-{}-{}-{}",
            tag,
            std::process::id(),
            unix_now()
        ));
        std::fs::create_dir_all(&d).unwrap();
        d
    }

    fn write_key(dir: &Path, name: &str, body: &str, mode: u32) -> PathBuf {
        let p = dir.join(name);
        let mut f = std::fs::OpenOptions::new()
            .write(true)
            .create_new(true)
            .mode(mode)
            .open(&p)
            .unwrap();
        f.write_all(body.as_bytes()).unwrap();
        // umask aside
        std::fs::set_permissions(&p, std::fs::Permissions::from_mode(mode)).unwrap();
        p
    }

    fn split(tok: &str) -> (String, String, Vec<u8>) {
        let parts: Vec<&str> = tok.split('.').collect();
        assert_eq!(parts.len(), 3, "{tok}");
        let h = String::from_utf8(URL_SAFE_NO_PAD.decode(parts[0]).unwrap()).unwrap();
        let c = String::from_utf8(URL_SAFE_NO_PAD.decode(parts[1]).unwrap()).unwrap();
        (h, c, URL_SAFE_NO_PAD.decode(parts[2]).unwrap())
    }

    #[test]
    fn key_names_follow_go() {
        for ok in ["wotan", "trace-collector", "ebpf-collector", "a", "x_1"] {
            assert!(valid_key_name(ok), "{ok}");
        }
        let long = "a".repeat(65);
        for bad in ["", "-a", "_a", "A", "a.b", "a b", "a/b", long.as_str()] {
            assert!(!valid_key_name(bad), "{bad}");
        }
    }

    #[test]
    fn token_shape_matches_pkg_auth() {
        let m = Minter::from_seed("trace-collector", &seed()).unwrap();
        assert_eq!(m.kid().len(), 16);
        assert_eq!(m.kid(), key_id(m.public_key()));
        let tok = m.mint_at("wotan", 1_700_000_000).unwrap();
        let (h, c, sig) = split(&tok);
        assert_eq!(
            h,
            format!(r#"{{"alg":"EdDSA","typ":"JWT","kid":"{}"}}"#, m.kid())
        );
        assert_eq!(
            c,
            r#"{"sub":"trace-collector","iss":"unheaded","aud":"wotan","exp":1700000300,"iat":1700000000,"roles":["service"]}"#
        );
        assert_eq!(sig.len(), 64);
        let input = tok.rsplit_once('.').unwrap().0;
        ring::signature::UnparsedPublicKey::new(&ring::signature::ED25519, m.public_key())
            .verify(input.as_bytes(), &sig)
            .expect("signature verifies");
        // Deterministic: the same inputs give the same token.
        assert_eq!(tok, m.mint_at("wotan", 1_700_000_000).unwrap());
    }

    #[test]
    fn bad_names_refused() {
        assert!(matches!(
            Minter::from_seed("Trace", &seed()),
            Err(Error::BadName(_))
        ));
        let m = Minter::from_seed("ebpf-collector", &seed()).unwrap();
        assert!(matches!(m.mint_at("", 1), Err(Error::BadName(_))));
        assert!(matches!(m.mint_at("wo\"tan", 1), Err(Error::BadName(_))));
    }

    #[test]
    fn cache_reuses_until_refresh_window() {
        let m = Minter::from_seed("ebpf-collector", &seed()).unwrap();
        let t0 = m.mint_cached("wotan", 1000).unwrap();
        assert_eq!(t0, m.mint_cached("wotan", 1000 + 269).unwrap());
        let t1 = m.mint_cached("wotan", 1000 + 270).unwrap();
        assert_ne!(t0, t1, "replaced 30 s before exp");
        assert_ne!(t1, m.mint_cached("other", 1270).unwrap(), "per audience");
        assert!(m.authorization("wotan").unwrap().starts_with("Bearer ey"));
    }

    #[test]
    fn key_file_round_trip_and_refusals() {
        let d = tmpdir("keys");
        let good = write_key(&d, "good.key", &pem(&seed()), 0o400);
        let m = Minter::load("trace-collector", &good).unwrap();
        assert_eq!(
            m.kid(),
            Minter::from_seed("trace-collector", &seed()).unwrap().kid()
        );

        let open = write_key(&d, "open.key", &pem(&seed()), 0o644);
        assert!(matches!(
            Minter::load("x", &open),
            Err(Error::Permissions(_, 0o644))
        ));

        let short = write_key(&d, "short.key", &pem(&[1u8; 31]), 0o600);
        assert!(matches!(Minter::load("x", &short), Err(Error::Format(..))));
        let other = pem(&seed()).replace(SEED_PEM_TYPE, "PRIVATE KEY");
        let wrong = write_key(&d, "wrong.key", &other, 0o600);
        assert!(matches!(Minter::load("x", &wrong), Err(Error::Format(..))));
        let two = format!("{}{}", pem(&seed()), pem(&seed()));
        let two = write_key(&d, "two.key", &two, 0o600);
        assert!(matches!(Minter::load("x", &two), Err(Error::Format(..))));
        let hdr = pem(&seed()).replacen("\n", "\nProc-Type: 4,ENCRYPTED\n", 1);
        let hdr = write_key(&d, "hdr.key", &hdr, 0o600);
        assert!(matches!(Minter::load("x", &hdr), Err(Error::Format(..))));

        assert!(matches!(
            Minter::load("x", &d),
            Err(Error::NotRegular(_)) | Err(Error::Permissions(..))
        ));

        match load_identity_from("trace-collector", &d.join("absent.key")).unwrap() {
            Identity::Missing(p) => assert!(p.ends_with("absent.key")),
            Identity::Loaded(_) => panic!("absent key loaded"),
        }
        assert!(load_identity_from("trace-collector", &open).is_err());
        assert!(matches!(
            load_identity_from("trace-collector", &good).unwrap(),
            Identity::Loaded(_)
        ));
        std::fs::remove_dir_all(&d).unwrap();
    }

    #[test]
    fn missing_warning_names_the_fix() {
        let w = missing_key_warning("ebpf-collector", Path::new("/k/e.key"));
        assert!(w.contains("/k/e.key") && w.contains("unheaded-keygen ebpf-collector"));
    }

    #[test]
    fn debug_hides_the_key() {
        let m = Minter::from_seed("wotan", &seed()).unwrap();
        let s = format!("{m:?}");
        assert!(s.contains("wotan") && s.contains(m.kid()) && !s.contains("key:"));
    }
}
