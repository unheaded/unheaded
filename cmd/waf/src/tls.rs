// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

//! TLS Termination for THE SHIELD
//!
//! Provides TLS termination using rustls for pure-Rust implementation.
//! Supports TLS 1.2 and 1.3 with modern cipher suites.

use crate::config::TlsConfig;
use rustls::pki_types::pem::{self, PemObject};
use rustls::pki_types::{CertificateDer, PrivateKeyDer};
use rustls::server::ServerConfig;
use std::path::Path;
use std::sync::Arc;
use tokio::io::{AsyncRead, AsyncWrite};
use tokio_rustls::TlsAcceptor;

/// TLS configuration errors
#[derive(Debug)]
pub enum TlsError {
    /// Failed to read certificate file
    CertificateError(String),
    /// Failed to read private key file
    KeyError(String),
    /// Failed to build TLS configuration
    ConfigError(String),
    /// Invalid TLS version specified
    InvalidVersion(String),
}

impl std::fmt::Display for TlsError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            TlsError::CertificateError(msg) => write!(f, "Certificate error: {}", msg),
            TlsError::KeyError(msg) => write!(f, "Key error: {}", msg),
            TlsError::ConfigError(msg) => write!(f, "TLS config error: {}", msg),
            TlsError::InvalidVersion(msg) => write!(f, "Invalid TLS version: {}", msg),
        }
    }
}

impl std::error::Error for TlsError {}

/// TLS manager for the WAF
pub struct TlsManager {
    /// TLS acceptor for incoming connections
    acceptor: TlsAcceptor,
    /// Server configuration
    #[allow(dead_code)]
    config: Arc<ServerConfig>,
}

impl TlsManager {
    /// Create a new TLS manager from configuration
    pub fn from_config(config: &TlsConfig) -> Result<Self, TlsError> {
        // Load certificates
        let certs = load_certs(&config.cert_path)?;

        // Load private key
        let key = load_private_key(&config.key_path)?;

        // Build server config
        let server_config = build_server_config(certs, key, &config.min_version)?;
        let config = Arc::new(server_config);
        let acceptor = TlsAcceptor::from(config.clone());

        Ok(Self { acceptor, config })
    }

    /// Get the TLS acceptor
    pub fn acceptor(&self) -> &TlsAcceptor {
        &self.acceptor
    }

    /// Accept a TLS connection
    pub async fn accept<S>(
        &self,
        stream: S,
    ) -> Result<tokio_rustls::server::TlsStream<S>, std::io::Error>
    where
        S: AsyncRead + AsyncWrite + Unpin,
    {
        self.acceptor.accept(stream).await
    }
}

/// Load certificates from a PEM file.
///
/// PEM is parsed by rustls-pki-types (`PemObject`), already part of rustls;
/// rustls-pemfile was unmaintained (RUSTSEC-2025-0134, FND-022).
fn load_certs(path: &str) -> Result<Vec<CertificateDer<'static>>, TlsError> {
    let certs: Vec<_> = CertificateDer::pem_file_iter(path)
        .map_err(|e| TlsError::CertificateError(format!("Failed to open {}: {}", path, e)))?
        .collect::<Result<Vec<_>, _>>()
        .map_err(|e| TlsError::CertificateError(format!("Failed to parse certificates: {}", e)))?;

    if certs.is_empty() {
        return Err(TlsError::CertificateError(
            "No certificates found in file".to_string(),
        ));
    }

    Ok(certs)
}

/// Load private key from a PEM file
fn load_private_key(path: &str) -> Result<PrivateKeyDer<'static>, TlsError> {
    // The first PKCS#8, PKCS#1 or SEC1 key section; other sections (such as
    // a certificate in the same file) are skipped.
    PrivateKeyDer::from_pem_file(path).map_err(|e| match e {
        pem::Error::NoItemsFound => TlsError::KeyError("No private key found in file".to_string()),
        pem::Error::Io(e) => TlsError::KeyError(format!("Failed to open {}: {}", path, e)),
        e => TlsError::KeyError(format!("Failed to parse private key: {}", e)),
    })
}

/// The crypto provider for every TLS config the WAF builds: aws-lc-rs, whose
/// default key-exchange groups put the hybrid post-quantum X25519MLKEM768
/// first (rustls `prefer-post-quantum`), then X25519, P-256 and P-384 for
/// clients that offer no hybrid. The WAF is an edge, so a classical-only
/// client is still served (ADR-105 "Preferred" profile).
fn crypto_provider() -> Arc<rustls::crypto::CryptoProvider> {
    Arc::new(rustls::crypto::aws_lc_rs::default_provider())
}

/// Build the rustls server configuration
fn build_server_config(
    certs: Vec<CertificateDer<'static>>,
    key: PrivateKeyDer<'static>,
    min_version: &str,
) -> Result<ServerConfig, TlsError> {
    // Select protocol versions
    let versions = match min_version {
        "1.2" => vec![&rustls::version::TLS12, &rustls::version::TLS13],
        "1.3" => vec![&rustls::version::TLS13],
        other => {
            return Err(TlsError::InvalidVersion(format!(
                "Unsupported TLS version: {}. Use 1.2 or 1.3",
                other
            )))
        }
    };

    // Build configuration with safe defaults on an explicit provider. Never
    // rely on the process-level default: it is ambiguous (and panics) when
    // more than one provider is compiled in.
    let config = ServerConfig::builder_with_provider(crypto_provider())
        .with_protocol_versions(&versions)
        .map_err(|e| TlsError::ConfigError(e.to_string()))?
        .with_no_client_auth()
        .with_single_cert(certs, key)
        .map_err(|e| TlsError::ConfigError(e.to_string()))?;

    Ok(config)
}

// A `generate_self_signed()` helper lived here, gated on `#[cfg(feature =
// "test-cert")]`. It was removed 2026-08-03: the crate had no `[features]`
// section, so the gate could never be enabled, and the body called `rcgen`,
// which was never a dependency. It had therefore never compiled once.
// `crates/zhend/src/api/quic.rs` already carries a working
// `generate_self_signed_cert()` built on rustls types — use that if a test
// certificate is ever needed here.

/// TLS connection info for logging and metrics
#[derive(Debug, Clone)]
pub struct TlsInfo {
    /// TLS protocol version
    pub version: &'static str,
    /// Cipher suite used
    pub cipher_suite: &'static str,
    /// SNI hostname (if provided)
    pub sni_hostname: Option<String>,
}

impl TlsInfo {
    /// Extract TLS info from a connection
    pub fn from_connection<S>(stream: &tokio_rustls::server::TlsStream<S>) -> Self {
        let (_, session) = stream.get_ref();

        let version = match session.protocol_version() {
            Some(rustls::ProtocolVersion::TLSv1_2) => "TLSv1.2",
            Some(rustls::ProtocolVersion::TLSv1_3) => "TLSv1.3",
            _ => "Unknown",
        };

        let cipher_suite = session
            .negotiated_cipher_suite()
            .map(|cs| cs.suite().as_str().unwrap_or("Unknown"))
            .unwrap_or("Unknown");

        let sni_hostname = session.server_name().map(|s| s.to_string());

        Self {
            version,
            cipher_suite,
            sni_hostname,
        }
    }
}

/// Check if a path looks like a TLS certificate
pub fn is_cert_file(path: &Path) -> bool {
    path.extension()
        .and_then(|ext| ext.to_str())
        .map(|ext| matches!(ext.to_lowercase().as_str(), "pem" | "crt" | "cer"))
        .unwrap_or(false)
}

/// Check if a path looks like a TLS private key
pub fn is_key_file(path: &Path) -> bool {
    path.extension()
        .and_then(|ext| ext.to_str())
        .map(|ext| matches!(ext.to_lowercase().as_str(), "pem" | "key"))
        .unwrap_or(false)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_is_cert_file() {
        assert!(is_cert_file(Path::new("cert.pem")));
        assert!(is_cert_file(Path::new("cert.crt")));
        assert!(is_cert_file(Path::new("cert.cer")));
        assert!(!is_cert_file(Path::new("cert.txt")));
    }

    #[test]
    fn test_is_key_file() {
        assert!(is_key_file(Path::new("key.pem")));
        assert!(is_key_file(Path::new("private.key")));
        assert!(!is_key_file(Path::new("key.txt")));
    }

    #[test]
    fn test_invalid_version() {
        let result = build_server_config(vec![], PrivateKeyDer::Pkcs8(vec![].into()), "1.1");
        assert!(result.is_err());
        if let Err(TlsError::InvalidVersion(_)) = result {
            // Expected
        } else {
            panic!("Expected InvalidVersion error");
        }
    }

    // ------------------------------------------------------------------
    // Post-quantum key exchange (S-QUANTUM-ARMOR Phase 7, ADR-105).
    //
    // The test certificate is generated at test time, never checked in: a
    // committed private key would trip gitleaks and the shrink-only
    // .gitleaksignore. The DER builder is the minimal Ed25519 one from
    // crates/zhend/src/api/quic.rs, on aws-lc-rs instead of ring.
    // ------------------------------------------------------------------

    use aws_lc_rs::signature::{Ed25519KeyPair, KeyPair};
    use rustls::pki_types::{PrivatePkcs8KeyDer, ServerName};
    use rustls::{ClientConfig, ClientConnection, NamedGroup, RootCertStore, ServerConnection};

    fn der(tag: u8, content: &[u8]) -> Vec<u8> {
        let mut out = vec![tag];
        let len = content.len();
        if len < 0x80 {
            out.push(len as u8);
        } else if len < 0x100 {
            out.extend_from_slice(&[0x81, len as u8]);
        } else {
            out.extend_from_slice(&[0x82, (len >> 8) as u8, len as u8]);
        }
        out.extend_from_slice(content);
        out
    }

    fn der_seq(parts: &[&[u8]]) -> Vec<u8> {
        der(0x30, &parts.concat())
    }

    fn der_bits(content: &[u8]) -> Vec<u8> {
        der(0x03, &[&[0u8][..], content].concat())
    }

    /// Self-signed Ed25519 X.509v3 certificate, CN=shield-test,
    /// SAN DNS:localhost, valid 2026-01-01 .. 2036-01-01.
    fn test_cert() -> (Vec<CertificateDer<'static>>, PrivateKeyDer<'static>) {
        let rng = aws_lc_rs::rand::SystemRandom::new();
        let pkcs8 = Ed25519KeyPair::generate_pkcs8(&rng).expect("Ed25519 key generation");
        let key = Ed25519KeyPair::from_pkcs8(pkcs8.as_ref()).expect("parse generated key");

        let ed25519 = der_seq(&[&der(0x06, &[0x2B, 0x65, 0x70])]); // 1.3.101.112
        let cn = der(0x0C, b"shield-test");
        let name = der_seq(&[&der(
            0x31,
            &der_seq(&[&der(0x06, &[0x55, 0x04, 0x03]), &cn]),
        )]);
        let validity = der_seq(&[&der(0x17, b"260101000000Z"), &der(0x17, b"360101000000Z")]);
        let spki = der_seq(&[&ed25519, &der_bits(key.public_key().as_ref())]);
        let san = der_seq(&[&der(0x82, b"localhost")]);
        let san_ext = der_seq(&[&der(0x06, &[0x55, 0x1D, 0x11]), &der(0x04, &san)]);
        let extensions = der(0xA3, &der_seq(&[&san_ext]));
        let tbs = der_seq(&[
            &der(0xA0, &der(0x02, &[2])), // v3
            &der(0x02, &[1]),             // serial
            &ed25519,
            &name,
            &validity,
            &name,
            &spki,
            &extensions,
        ]);
        let sig = key.sign(&tbs);
        let cert = der_seq(&[&tbs, &ed25519, &der_bits(sig.as_ref())]);

        (
            vec![CertificateDer::from(cert)],
            PrivateKeyDer::Pkcs8(PrivatePkcs8KeyDer::from(pkcs8.as_ref().to_vec())),
        )
    }

    fn client_config(
        provider: rustls::crypto::CryptoProvider,
        trusted: &CertificateDer<'static>,
    ) -> ClientConfig {
        let mut roots = RootCertStore::empty();
        roots.add(trusted.clone()).expect("trust the test cert");
        ClientConfig::builder_with_provider(Arc::new(provider))
            .with_protocol_versions(&[&rustls::version::TLS13])
            .expect("client protocol versions")
            .with_root_certificates(roots)
            .with_no_client_auth()
    }

    /// In-memory TLS handshake: pump records between the two ends until
    /// neither is handshaking. Returns the server's negotiated group.
    fn handshake(server: ServerConfig, client: ClientConfig) -> Result<NamedGroup, rustls::Error> {
        let name = ServerName::try_from("localhost").expect("server name");
        let mut c = ClientConnection::new(Arc::new(client), name)?;
        let mut s = ServerConnection::new(Arc::new(server))?;
        for _ in 0..16 {
            if !c.is_handshaking() && !s.is_handshaking() {
                break;
            }
            let mut buf = Vec::new();
            c.write_tls(&mut buf).expect("client write_tls");
            s.read_tls(&mut buf.as_slice()).expect("server read_tls");
            s.process_new_packets()?;
            let mut buf = Vec::new();
            s.write_tls(&mut buf).expect("server write_tls");
            c.read_tls(&mut buf.as_slice()).expect("client read_tls");
            c.process_new_packets()?;
        }
        assert!(
            !c.is_handshaking() && !s.is_handshaking(),
            "handshake did not complete"
        );
        let group = s
            .negotiated_key_exchange_group()
            .map(|g| g.name())
            .expect("server reports a key exchange group");
        assert_eq!(
            c.negotiated_key_exchange_group().map(|g| g.name()),
            Some(group),
            "client and server disagree on the group"
        );
        Ok(group)
    }

    /// H6 regression: rustls was built with both `ring` and `aws_lc_rs`
    /// and no process-level CryptoProvider was installed, so building the
    /// server config panicked. The provider is now passed explicitly.
    #[test]
    fn build_server_config_does_not_panic() {
        for v in ["1.2", "1.3"] {
            let (certs, key) = test_cert();
            assert!(
                build_server_config(certs, key, v).is_ok(),
                "min_version {v} must build"
            );
        }
    }

    #[test]
    fn handshake_negotiates_x25519mlkem768() {
        let (certs, key) = test_cert();
        let trusted = certs[0].clone();
        let server = build_server_config(certs, key, "1.3").expect("server config");
        let client = client_config(rustls::crypto::aws_lc_rs::default_provider(), &trusted);
        let group = handshake(server, client).expect("handshake");
        assert_eq!(
            group,
            NamedGroup::X25519MLKEM768,
            "WAF negotiated a classical key exchange group"
        );
    }

    #[test]
    fn handshake_with_tls12_floor_still_negotiates_x25519mlkem768() {
        let (certs, key) = test_cert();
        let trusted = certs[0].clone();
        let server = build_server_config(certs, key, "1.2").expect("server config");
        let client = client_config(rustls::crypto::aws_lc_rs::default_provider(), &trusted);
        let group = handshake(server, client).expect("handshake");
        assert_eq!(group, NamedGroup::X25519MLKEM768);
    }

    /// The WAF is an edge (Preferred profile, ADR-105): a client that only
    /// offers classical X25519 is still served, with X25519. This documents
    /// the accepted fallback; it is not the hybrid guarantee.
    #[test]
    fn classical_only_client_falls_back_to_x25519() {
        let (certs, key) = test_cert();
        let trusted = certs[0].clone();
        let server = build_server_config(certs, key, "1.3").expect("server config");
        let classical = rustls::crypto::CryptoProvider {
            kx_groups: vec![rustls::crypto::aws_lc_rs::kx_group::X25519],
            ..rustls::crypto::aws_lc_rs::default_provider()
        };
        let group = handshake(server, client_config(classical, &trusted)).expect("handshake");
        assert_eq!(group, NamedGroup::X25519);
    }

    // ------------------------------------------------------------------
    // PEM loading (load_certs / load_private_key). The PEM is built at
    // test time from the generated certificate, never checked in.
    // ------------------------------------------------------------------

    fn base64(data: &[u8]) -> String {
        const T: &[u8; 64] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
        let mut out = String::new();
        for c in data.chunks(3) {
            let n = (u32::from(c[0]) << 16)
                | (u32::from(*c.get(1).unwrap_or(&0)) << 8)
                | u32::from(*c.get(2).unwrap_or(&0));
            for i in 0..4 {
                if i <= c.len() {
                    out.push(T[((n >> (18 - 6 * i)) & 63) as usize] as char);
                } else {
                    out.push('=');
                }
            }
        }
        out
    }

    fn pem(label: &str, der: &[u8]) -> String {
        let b = base64(der);
        let mut out = format!("-----BEGIN {label}-----\n");
        for line in b.as_bytes().chunks(64) {
            out.push_str(std::str::from_utf8(line).unwrap());
            out.push('\n');
        }
        out.push_str(&format!("-----END {label}-----\n"));
        out
    }

    /// A file under a fresh per-test directory, removed on drop.
    struct TempPem(std::path::PathBuf);
    impl TempPem {
        fn new(name: &str, contents: &str) -> Self {
            let dir =
                std::env::temp_dir().join(format!("waf-tls-test-{}-{}", std::process::id(), name));
            std::fs::create_dir_all(&dir).expect("temp dir");
            let path = dir.join("f.pem");
            std::fs::write(&path, contents).expect("write pem");
            TempPem(path)
        }
        fn path(&self) -> &str {
            self.0.to_str().unwrap()
        }
    }
    impl Drop for TempPem {
        fn drop(&mut self) {
            if let Some(dir) = self.0.parent() {
                let _ = std::fs::remove_dir_all(dir);
            }
        }
    }

    #[test]
    fn load_certs_and_key_round_trip_pem() {
        let (certs, key) = test_cert();
        let chain = format!(
            "junk before the first section\n{}{}",
            pem("CERTIFICATE", certs[0].as_ref()),
            pem("CERTIFICATE", certs[0].as_ref())
        );
        let cert_file = TempPem::new("chain", &chain);
        let loaded = load_certs(cert_file.path()).expect("load certs");
        assert_eq!(loaded.len(), 2, "every CERTIFICATE section is loaded");
        assert_eq!(loaded[0].as_ref(), certs[0].as_ref());

        // The key file may hold the certificate too; the key is still found.
        let both = format!(
            "{}{}",
            pem("CERTIFICATE", certs[0].as_ref()),
            pem("PRIVATE KEY", key.secret_der())
        );
        let key_file = TempPem::new("key", &both);
        let loaded_key = load_private_key(key_file.path()).expect("load key");
        assert!(matches!(loaded_key, PrivateKeyDer::Pkcs8(_)));
        assert_eq!(loaded_key.secret_der(), key.secret_der());

        // What was loaded is usable: it builds a server config.
        build_server_config(loaded, loaded_key, "1.3").expect("server config from PEM files");
    }

    #[test]
    fn load_certs_rejects_missing_empty_and_corrupt() {
        assert!(matches!(
            load_certs("/nonexistent/waf-tls-test.pem"),
            Err(TlsError::CertificateError(_))
        ));
        let empty = TempPem::new("empty", "no pem here\n");
        assert!(matches!(
            load_certs(empty.path()),
            Err(TlsError::CertificateError(_))
        ));
        let corrupt = TempPem::new(
            "corrupt",
            "-----BEGIN CERTIFICATE-----\n!!!not base64!!!\n-----END CERTIFICATE-----\n",
        );
        assert!(matches!(
            load_certs(corrupt.path()),
            Err(TlsError::CertificateError(_))
        ));
    }

    #[test]
    fn load_private_key_rejects_missing_and_keyless() {
        assert!(matches!(
            load_private_key("/nonexistent/waf-tls-test.pem"),
            Err(TlsError::KeyError(_))
        ));
        let (certs, _) = test_cert();
        let cert_only = TempPem::new("certonly", &pem("CERTIFICATE", certs[0].as_ref()));
        assert!(matches!(
            load_private_key(cert_only.path()),
            Err(TlsError::KeyError(_))
        ));
    }
}
