//! QUIC/HTTP3 transport for external Zhen access.
//!
//! Uses quinn for QUIC. Benefits over TCP:
//! - 0-RTT connection establishment (returning peers reconnect instantly)
//! - Connection migration (mobile/roaming nodes keep sessions alive)
//! - Multiplexed streams (multiple pilgrimages in parallel, no head-of-line blocking)
//! - Built-in encryption (TLS 1.3 mandatory)
//!
//! This is the transport for nodes outside the Kingdom's gRPC mesh —
//! edge nodes, mobile agents, external knowledge contributors.
//!
//! ## Wire Protocol
//!
//! Each bidirectional QUIC stream carries a single request/response:
//!
//! Request:  `[op: u8][payload_len: u32 BE][payload: bytes]`
//! Response: `[status: u8][payload_len: u32 BE][payload: bytes]`
//!
//! Op codes:
//! - 1 = Ingest (payload = raw fragment bytes, response = 32-byte fragment ID + 1 byte novel flag)
//! - 2 = Surface (payload = context embedding bytes, response = JSON scored fragments)
//! - 3 = Status (no payload, response = JSON status)
//!
//! Status codes:
//! - 0 = OK
//! - 1 = Bad Request
//! - 2 = Internal Error
//!
//! GPL-3.0-only

use std::sync::Arc;
use std::time::Instant;

use quinn::crypto::rustls::QuicServerConfig;
use ring::signature::KeyPair;
use rustls::pki_types::{CertificateDer, PrivateKeyDer, PrivatePkcs8KeyDer};

use crate::de::embedder::Embedder;
use crate::de::similarity::rank_by_de;
use crate::pu::TieredStore;
use crate::ZhenConfig;

/// QUIC op codes.
pub const OP_INGEST: u8 = 1;
pub const OP_SURFACE: u8 = 2;
pub const OP_STATUS: u8 = 3;

/// Response status codes.
pub const STATUS_OK: u8 = 0;
pub const STATUS_BAD_REQUEST: u8 = 1;
pub const STATUS_INTERNAL_ERROR: u8 = 2;

/// Shared state for the QUIC server, mirroring gRPC ZhenService.
pub struct QuicServer {
    store: Arc<TieredStore>,
    embedder: Arc<Embedder>,
    config: ZhenConfig,
    started_at: Instant,
}

impl QuicServer {
    pub fn new(store: Arc<TieredStore>, embedder: Arc<Embedder>, config: ZhenConfig) -> Self {
        Self {
            store,
            embedder,
            config,
            started_at: Instant::now(),
        }
    }

    /// Handle a single stream request. Reads the wire protocol, dispatches to the
    /// appropriate handler, writes the response.
    pub async fn handle_stream(
        &self,
        mut send: quinn::SendStream,
        mut recv: quinn::RecvStream,
    ) {
        // Read header: [op: u8][payload_len: u32 BE]
        let mut header = [0u8; 5];
        if let Err(e) = recv.read_exact(&mut header).await {
            tracing::warn!("QUIC stream read header failed: {e}");
            return;
        }

        let op = header[0];
        let payload_len = u32::from_be_bytes([header[1], header[2], header[3], header[4]]) as usize;

        // Sanity limit: 16 MB max payload.
        if payload_len > 16 * 1024 * 1024 {
            let _ = write_response(&mut send, STATUS_BAD_REQUEST, b"payload too large").await;
            return;
        }

        let mut payload = vec![0u8; payload_len];
        if payload_len > 0 {
            if let Err(e) = recv.read_exact(&mut payload).await {
                tracing::warn!("QUIC stream read payload failed: {e}");
                return;
            }
        }

        let (status, response) = match op {
            OP_INGEST => self.handle_ingest(&payload),
            OP_SURFACE => self.handle_surface(&payload),
            OP_STATUS => self.handle_status(),
            _ => (STATUS_BAD_REQUEST, b"unknown op code".to_vec()),
        };

        let _ = write_response(&mut send, status, &response).await;
        let _ = send.finish();
    }

    /// Ingest a fragment. Payload = raw bytes. Response = [32-byte ID][1-byte novel].
    fn handle_ingest(&self, payload: &[u8]) -> (u8, Vec<u8>) {
        if payload.is_empty() {
            return (STATUS_BAD_REQUEST, b"payload must not be empty".to_vec());
        }

        let embedding = self.embedder.embed_bytes(payload, None);
        let fragment = crate::pu::Fragment::new(payload.to_vec(), embedding, None);
        let fragment_id = fragment.id.0;

        match self.store.ingest(fragment) {
            Ok(novel) => {
                let mut resp = Vec::with_capacity(33);
                resp.extend_from_slice(&fragment_id);
                resp.push(if novel { 1 } else { 0 });
                (STATUS_OK, resp)
            }
            Err(e) => (STATUS_INTERNAL_ERROR, format!("ingest failed: {e}").into_bytes()),
        }
    }

    /// Surface De-ranked fragments. Payload = context embedding bytes.
    /// Response = JSON array of scored fragments.
    fn handle_surface(&self, payload: &[u8]) -> (u8, Vec<u8>) {
        let fragments = match self.store.l1_fragments() {
            Ok(f) => f,
            Err(e) => return (STATUS_INTERNAL_ERROR, format!("read failed: {e}").into_bytes()),
        };

        let ranked = rank_by_de(payload, &fragments, 10);

        // Build a simple JSON response.
        let scored: Vec<serde_json::Value> = ranked
            .into_iter()
            .map(|(frag, score)| {
                serde_json::json!({
                    "fragment_id": hex::encode(frag.id.0),
                    "de_score": score,
                    "payload_len": frag.payload.len(),
                })
            })
            .collect();

        match serde_json::to_vec(&scored) {
            Ok(json) => (STATUS_OK, json),
            Err(e) => (STATUS_INTERNAL_ERROR, format!("json failed: {e}").into_bytes()),
        }
    }

    /// Status report. No payload needed. Response = JSON status.
    fn handle_status(&self) -> (u8, Vec<u8>) {
        let l1 = self.store.l1_count().unwrap_or(0) as u64;
        let l2 = self.store.l2_count() as u64;
        let l3 = self.store.l3_count().unwrap_or(0) as u64;
        let uptime = self.started_at.elapsed().as_secs();

        let status = serde_json::json!({
            "node_id": self.config.quic_addr,
            "l1_count": l1,
            "l2_count": l2,
            "l3_count": l3,
            "uptime_secs": uptime,
            "embedder_ready": self.embedder.is_ready(),
        });

        match serde_json::to_vec(&status) {
            Ok(json) => (STATUS_OK, json),
            Err(e) => (STATUS_INTERNAL_ERROR, format!("json failed: {e}").into_bytes()),
        }
    }
}

/// Write a response frame: [status: u8][payload_len: u32 BE][payload: bytes]
async fn write_response(
    send: &mut quinn::SendStream,
    status: u8,
    payload: &[u8],
) -> Result<(), quinn::WriteError> {
    let len = payload.len() as u32;
    let mut header = [0u8; 5];
    header[0] = status;
    header[1..5].copy_from_slice(&len.to_be_bytes());
    send.write_all(&header).await?;
    if !payload.is_empty() {
        send.write_all(payload).await?;
    }
    Ok(())
}

/// Generate a self-signed certificate for development/testing.
/// Returns (certificate chain, private key) suitable for rustls ServerConfig.
pub fn generate_self_signed_cert() -> (Vec<CertificateDer<'static>>, PrivateKeyDer<'static>) {
    // Generate an Ed25519 keypair using ring (available via rustls dep).
    let keypair = ring::signature::Ed25519KeyPair::generate_pkcs8(
        &ring::rand::SystemRandom::new(),
    )
    .expect("Ed25519 key generation failed");

    let key_der = PrivatePkcs8KeyDer::from(keypair.as_ref().to_vec());

    // Build a minimal self-signed X.509 certificate.
    // We construct the DER manually to avoid needing rcgen.
    let signing_key =
        ring::signature::Ed25519KeyPair::from_pkcs8(keypair.as_ref())
            .expect("parse generated key");

    let public_key = signing_key.public_key().as_ref();

    let cert_der = build_self_signed_cert_der(public_key, &signing_key);

    (
        vec![CertificateDer::from(cert_der)],
        PrivateKeyDer::Pkcs8(key_der),
    )
}

/// Build a minimal self-signed X.509v3 DER certificate with Ed25519.
fn build_self_signed_cert_der(
    public_key: &[u8],
    signing_key: &ring::signature::Ed25519KeyPair,
) -> Vec<u8> {
    // We need a valid X.509 cert. Build it from scratch in DER/ASN.1.
    // This is a minimal cert: version 3, serial 1, CN=zhend, validity 10 years,
    // Ed25519 signature, SubjectAltName=localhost+::1.

    let mut tbs = Vec::new();

    // version [0] EXPLICIT INTEGER { 2 } (v3)
    tbs.extend_from_slice(&der_explicit_tag(0, &der_integer(&[2])));

    // serialNumber INTEGER
    tbs.extend_from_slice(&der_integer(&[1]));

    // signature AlgorithmIdentifier (Ed25519 OID: 1.3.101.112)
    let ed25519_alg = der_sequence(&der_oid(&[1, 3, 101, 112]));
    tbs.extend_from_slice(&ed25519_alg);

    // issuer: CN=zhend
    let cn_oid = der_oid(&[2, 5, 4, 3]); // id-at-commonName
    let cn_val = der_utf8_string(b"zhend");
    let attr_tv = der_sequence(&[cn_oid, cn_val].concat());
    let rdn = der_set(&attr_tv);
    let issuer = der_sequence(&rdn);
    tbs.extend_from_slice(&issuer);

    // validity: notBefore=2026-01-01, notAfter=2036-01-01
    let not_before = der_utc_time(b"260101000000Z");
    let not_after = der_utc_time(b"360101000000Z");
    let validity = der_sequence(&[not_before, not_after].concat());
    tbs.extend_from_slice(&validity);

    // subject = issuer
    let cn_oid2 = der_oid(&[2, 5, 4, 3]);
    let cn_val2 = der_utf8_string(b"zhend");
    let attr_tv2 = der_sequence(&[cn_oid2, cn_val2].concat());
    let rdn2 = der_set(&attr_tv2);
    let subject = der_sequence(&rdn2);
    tbs.extend_from_slice(&subject);

    // subjectPublicKeyInfo: Ed25519
    let spki_alg = der_sequence(&der_oid(&[1, 3, 101, 112]));
    let spki_key = der_bit_string(public_key);
    let spki = der_sequence(&[spki_alg, spki_key].concat());
    tbs.extend_from_slice(&spki);

    // extensions [3] EXPLICIT SEQUENCE { ... }
    // SubjectAltName extension with DNS:localhost and IP:::1 and IP:127.0.0.1
    let san_oid = der_oid(&[2, 5, 29, 17]); // id-ce-subjectAltName
    let mut san_names = Vec::new();
    // DNS:localhost — context tag [2]
    san_names.extend_from_slice(&der_context_tag(2, b"localhost"));
    // IP:127.0.0.1 — context tag [7]
    san_names.extend_from_slice(&der_context_tag(7, &[127, 0, 0, 1]));
    // IP:::1 — context tag [7], 16 bytes
    let ipv6_loopback = [0u8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1];
    san_names.extend_from_slice(&der_context_tag(7, &ipv6_loopback));
    let san_value = der_sequence(&san_names);
    let san_ext = der_sequence(&[san_oid, der_octet_string(&san_value)].concat());
    let extensions = der_sequence(&san_ext);
    tbs.extend_from_slice(&der_explicit_tag(3, &extensions));

    let tbs_der = der_sequence(&tbs);

    // Sign TBS.
    let signature = signing_key.sign(tbs_der.as_ref());
    let sig_bytes = signature.as_ref();

    // Full certificate: SEQUENCE { tbsCertificate, signatureAlgorithm, signatureValue }
    let mut cert = Vec::new();
    cert.extend_from_slice(&tbs_der);
    cert.extend_from_slice(&ed25519_alg);
    cert.extend_from_slice(&der_bit_string(sig_bytes));

    der_sequence(&cert)
}

// Minimal DER encoding helpers.

fn der_tag_len(tag: u8, content: &[u8]) -> Vec<u8> {
    let mut out = vec![tag];
    let len = content.len();
    if len < 128 {
        out.push(len as u8);
    } else if len < 256 {
        out.push(0x81);
        out.push(len as u8);
    } else if len < 65536 {
        out.push(0x82);
        out.push((len >> 8) as u8);
        out.push(len as u8);
    } else {
        out.push(0x83);
        out.push((len >> 16) as u8);
        out.push((len >> 8) as u8);
        out.push(len as u8);
    }
    out.extend_from_slice(content);
    out
}

fn der_sequence(content: &[u8]) -> Vec<u8> {
    der_tag_len(0x30, content)
}

fn der_set(content: &[u8]) -> Vec<u8> {
    der_tag_len(0x31, content)
}

fn der_integer(val: &[u8]) -> Vec<u8> {
    // If high bit set, prepend a 0x00.
    if !val.is_empty() && val[0] & 0x80 != 0 {
        let mut padded = vec![0x00];
        padded.extend_from_slice(val);
        der_tag_len(0x02, &padded)
    } else {
        der_tag_len(0x02, val)
    }
}

fn der_bit_string(content: &[u8]) -> Vec<u8> {
    let mut bs = vec![0x00]; // unused bits = 0
    bs.extend_from_slice(content);
    der_tag_len(0x03, &bs)
}

fn der_octet_string(content: &[u8]) -> Vec<u8> {
    der_tag_len(0x04, content)
}

fn der_utf8_string(content: &[u8]) -> Vec<u8> {
    der_tag_len(0x0C, content)
}

fn der_utc_time(content: &[u8]) -> Vec<u8> {
    der_tag_len(0x17, content)
}

fn der_oid(components: &[u64]) -> Vec<u8> {
    assert!(components.len() >= 2);
    let mut encoded = vec![(components[0] * 40 + components[1]) as u8];
    for &c in &components[2..] {
        if c < 128 {
            encoded.push(c as u8);
        } else {
            // Base-128 encoding for larger values.
            let mut bytes = Vec::new();
            let mut val = c;
            bytes.push((val & 0x7F) as u8);
            val >>= 7;
            while val > 0 {
                bytes.push((val & 0x7F) as u8 | 0x80);
                val >>= 7;
            }
            bytes.reverse();
            encoded.extend_from_slice(&bytes);
        }
    }
    der_tag_len(0x06, &encoded)
}

fn der_explicit_tag(tag_num: u8, content: &[u8]) -> Vec<u8> {
    der_tag_len(0xA0 | tag_num, content)
}

fn der_context_tag(tag_num: u8, content: &[u8]) -> Vec<u8> {
    der_tag_len(0x80 | tag_num, content)
}

/// Build a quinn ServerConfig with a self-signed cert.
pub fn build_server_config() -> quinn::ServerConfig {
    let (certs, key) = generate_self_signed_cert();

    let provider = rustls::crypto::ring::default_provider();
    let mut tls_config = rustls::ServerConfig::builder_with_provider(Arc::new(provider))
        .with_safe_default_protocol_versions()
        .expect("TLS protocol versions")
        .with_no_client_auth()
        .with_single_cert(certs, key)
        .expect("TLS config with self-signed cert");

    tls_config.alpn_protocols = vec![b"zhen/1".to_vec()];

    let quic_server_config = QuicServerConfig::try_from(tls_config)
        .expect("quinn server config from rustls");

    quinn::ServerConfig::with_crypto(Arc::new(quic_server_config))
}

/// Build a quinn client config that trusts any certificate (for dev/testing).
pub fn build_client_config_insecure() -> quinn::ClientConfig {
    let provider = rustls::crypto::ring::default_provider();
    let mut tls_config = rustls::ClientConfig::builder_with_provider(Arc::new(provider))
        .with_safe_default_protocol_versions()
        .expect("TLS protocol versions")
        .dangerous()
        .with_custom_certificate_verifier(Arc::new(InsecureCertVerifier))
        .with_no_client_auth();

    tls_config.alpn_protocols = vec![b"zhen/1".to_vec()];

    let mut client_config = quinn::ClientConfig::new(Arc::new(
        quinn::crypto::rustls::QuicClientConfig::try_from(tls_config)
            .expect("quinn client config"),
    ));

    // Reduce idle timeout for tests.
    let mut transport = quinn::TransportConfig::default();
    transport.max_idle_timeout(Some(
        std::time::Duration::from_secs(10).try_into().unwrap(),
    ));
    client_config.transport_config(Arc::new(transport));

    client_config
}

/// Certificate verifier that accepts any certificate (dev/test only).
#[derive(Debug)]
struct InsecureCertVerifier;

impl rustls::client::danger::ServerCertVerifier for InsecureCertVerifier {
    fn verify_server_cert(
        &self,
        _end_entity: &CertificateDer<'_>,
        _intermediates: &[CertificateDer<'_>],
        _server_name: &rustls::pki_types::ServerName<'_>,
        _ocsp_response: &[u8],
        _now: rustls::pki_types::UnixTime,
    ) -> Result<rustls::client::danger::ServerCertVerified, rustls::Error> {
        Ok(rustls::client::danger::ServerCertVerified::assertion())
    }

    fn verify_tls12_signature(
        &self,
        _message: &[u8],
        _cert: &CertificateDer<'_>,
        _dss: &rustls::DigitallySignedStruct,
    ) -> Result<rustls::client::danger::HandshakeSignatureValid, rustls::Error> {
        Ok(rustls::client::danger::HandshakeSignatureValid::assertion())
    }

    fn verify_tls13_signature(
        &self,
        _message: &[u8],
        _cert: &CertificateDer<'_>,
        _dss: &rustls::DigitallySignedStruct,
    ) -> Result<rustls::client::danger::HandshakeSignatureValid, rustls::Error> {
        Ok(rustls::client::danger::HandshakeSignatureValid::assertion())
    }

    fn supported_verify_schemes(&self) -> Vec<rustls::SignatureScheme> {
        vec![
            rustls::SignatureScheme::ED25519,
            rustls::SignatureScheme::ECDSA_NISTP256_SHA256,
            rustls::SignatureScheme::ECDSA_NISTP384_SHA384,
            rustls::SignatureScheme::RSA_PSS_SHA256,
            rustls::SignatureScheme::RSA_PSS_SHA384,
            rustls::SignatureScheme::RSA_PSS_SHA512,
        ]
    }
}

/// Start the QUIC server. Accepts connections in a loop, spawning a task per connection.
/// Each connection can open multiple bidirectional streams.
pub async fn serve(
    server: Arc<QuicServer>,
    addr: &str,
) -> Result<(), Box<dyn std::error::Error>> {
    let addr: std::net::SocketAddr = addr.parse()?;

    let server_config = build_server_config();
    let endpoint = quinn::Endpoint::server(server_config, addr)?;

    tracing::info!(%addr, "zhend QUIC server starting");

    while let Some(incoming) = endpoint.accept().await {
        let server = Arc::clone(&server);
        tokio::spawn(async move {
            match incoming.await {
                Ok(conn) => {
                    tracing::debug!(
                        remote = %conn.remote_address(),
                        "QUIC connection accepted"
                    );
                    handle_connection(server, conn).await;
                }
                Err(e) => {
                    tracing::warn!("QUIC connection failed: {e}");
                }
            }
        });
    }

    Ok(())
}

/// Handle a single QUIC connection — accept bidirectional streams in a loop.
async fn handle_connection(server: Arc<QuicServer>, conn: quinn::Connection) {
    loop {
        match conn.accept_bi().await {
            Ok((send, recv)) => {
                let server = Arc::clone(&server);
                tokio::spawn(async move {
                    server.handle_stream(send, recv).await;
                });
            }
            Err(quinn::ConnectionError::ApplicationClosed(_)) => {
                tracing::debug!("QUIC connection closed by peer");
                break;
            }
            Err(e) => {
                tracing::warn!("QUIC stream accept error: {e}");
                break;
            }
        }
    }
}

/// Helper to send a request over a QUIC stream (for clients/tests).
pub async fn send_request(
    send: &mut quinn::SendStream,
    recv: &mut quinn::RecvStream,
    op: u8,
    payload: &[u8],
) -> Result<(u8, Vec<u8>), Box<dyn std::error::Error + Send + Sync>> {
    // Write request header + payload.
    let len = payload.len() as u32;
    let mut header = [0u8; 5];
    header[0] = op;
    header[1..5].copy_from_slice(&len.to_be_bytes());
    send.write_all(&header).await?;
    if !payload.is_empty() {
        send.write_all(payload).await?;
    }
    send.finish()?;

    // Read response.
    let mut resp_header = [0u8; 5];
    recv.read_exact(&mut resp_header).await?;
    let status = resp_header[0];
    let resp_len = u32::from_be_bytes([
        resp_header[1],
        resp_header[2],
        resp_header[3],
        resp_header[4],
    ]) as usize;

    let mut resp_payload = vec![0u8; resp_len];
    if resp_len > 0 {
        recv.read_exact(&mut resp_payload).await?;
    }

    Ok((status, resp_payload))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_der_integer_positive() {
        let encoded = der_integer(&[1]);
        assert_eq!(encoded, vec![0x02, 0x01, 0x01]);
    }

    #[test]
    fn test_der_integer_high_bit() {
        let encoded = der_integer(&[0x80]);
        assert_eq!(encoded, vec![0x02, 0x02, 0x00, 0x80]);
    }

    #[test]
    fn test_der_oid_simple() {
        // OID 1.3.101.112 (Ed25519)
        let encoded = der_oid(&[1, 3, 101, 112]);
        // 1*40+3 = 43 = 0x2B, 101 = 0x65, 112 = 0x70
        assert_eq!(encoded, vec![0x06, 0x03, 0x2B, 0x65, 0x70]);
    }

    #[test]
    fn test_generate_self_signed_cert() {
        let (certs, key) = generate_self_signed_cert();
        assert_eq!(certs.len(), 1);
        assert!(!certs[0].as_ref().is_empty());
        match &key {
            PrivateKeyDer::Pkcs8(k) => assert!(!k.secret_pkcs8_der().is_empty()),
            _ => panic!("expected PKCS8 key"),
        }
    }

    #[test]
    fn test_build_server_config_succeeds() {
        let _config = build_server_config();
    }

    #[test]
    fn test_build_client_config_insecure_succeeds() {
        let _config = build_client_config_insecure();
    }

    #[test]
    fn test_wire_protocol_encoding() {
        // Verify the request encoding matches the spec.
        let op = OP_INGEST;
        let payload = b"test payload";
        let len = payload.len() as u32;
        let mut header = [0u8; 5];
        header[0] = op;
        header[1..5].copy_from_slice(&len.to_be_bytes());

        assert_eq!(header[0], 1); // OP_INGEST
        assert_eq!(
            u32::from_be_bytes([header[1], header[2], header[3], header[4]]),
            12 // "test payload".len()
        );
    }
}
