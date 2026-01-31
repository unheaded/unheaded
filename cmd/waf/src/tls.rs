//! TLS Termination for THE SHIELD
//!
//! Provides TLS termination using rustls for pure-Rust implementation.
//! Supports TLS 1.2 and 1.3 with modern cipher suites.

use crate::config::TlsConfig;
use rustls::pki_types::{CertificateDer, PrivateKeyDer};
use rustls::server::ServerConfig;
use rustls_pemfile::{certs, private_key};
use std::fs::File;
use std::io::BufReader;
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
    pub async fn accept<S>(&self, stream: S) -> Result<tokio_rustls::server::TlsStream<S>, std::io::Error>
    where
        S: AsyncRead + AsyncWrite + Unpin,
    {
        self.acceptor.accept(stream).await
    }
}

/// Load certificates from a PEM file
fn load_certs(path: &str) -> Result<Vec<CertificateDer<'static>>, TlsError> {
    let file = File::open(path)
        .map_err(|e| TlsError::CertificateError(format!("Failed to open {}: {}", path, e)))?;
    let mut reader = BufReader::new(file);

    let certs: Vec<_> = certs(&mut reader)
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
    let file = File::open(path)
        .map_err(|e| TlsError::KeyError(format!("Failed to open {}: {}", path, e)))?;
    let mut reader = BufReader::new(file);

    let key = private_key(&mut reader)
        .map_err(|e| TlsError::KeyError(format!("Failed to parse private key: {}", e)))?
        .ok_or_else(|| TlsError::KeyError("No private key found in file".to_string()))?;

    Ok(key)
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

    // Build configuration with safe defaults
    let config = ServerConfig::builder_with_protocol_versions(&versions)
        .with_no_client_auth()
        .with_single_cert(certs, key)
        .map_err(|e| TlsError::ConfigError(e.to_string()))?;

    Ok(config)
}

/// Generate a self-signed certificate for testing
#[cfg(feature = "test-cert")]
pub fn generate_self_signed() -> Result<(Vec<u8>, Vec<u8>), TlsError> {
    use rcgen::{generate_simple_self_signed, CertifiedKey};

    let subject_alt_names = vec![
        "localhost".to_string(),
        "127.0.0.1".to_string(),
    ];

    let CertifiedKey { cert, key_pair } = generate_simple_self_signed(subject_alt_names)
        .map_err(|e| TlsError::ConfigError(format!("Failed to generate certificate: {}", e)))?;

    Ok((cert.pem().into_bytes(), key_pair.serialize_pem().into_bytes()))
}

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
    use std::path::PathBuf;

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
}
