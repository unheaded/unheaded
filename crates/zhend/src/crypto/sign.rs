//! Post-Quantum Digital Signatures: ML-DSA-65 (Dilithium)
//!
//! Used for:
//! - Gossip peer identity authentication (Sybil resistance)
//! - Fragment origin attestation (optional, non-repudiation)
//! - Admission control (peer must prove Kingdom membership)
//!
//! ML-DSA-65 provides NIST Security Level 3 (~128-bit post-quantum).
//! Signature size: 3293 bytes. Public key: 1952 bytes.
//! These are larger than Ed25519 (64B sig, 32B pk) but quantum-safe.
//!
//! We accept the size cost. The alternative is forgeable peer identity
//! under quantum attack. Unacceptable for a system that relies on
//! authenticated gossip to prevent Sybil attacks.

use zeroize::Zeroize;

#[cfg(feature = "pq")]
use pqcrypto_dilithium::dilithium3;
#[cfg(feature = "pq")]
use pqcrypto_traits::sign::{DetachedSignature, PublicKey as SignPk, SecretKey as SignSk};

/// ML-DSA-65 signing keypair for peer authentication.
pub struct PqSigningKeypair {
    /// Public key (1952 bytes).
    pub public: Vec<u8>,
    /// Secret key. ZEROIZE ON DROP.
    secret: Vec<u8>,
}

impl PqSigningKeypair {
    /// Generate a fresh ML-DSA-65 keypair.
    pub fn generate() -> Self {
        let (pk, sk) = dilithium3::keypair();
        Self {
            public: pk.as_bytes().to_vec(),
            secret: sk.as_bytes().to_vec(),
        }
    }

    /// Sign a message. Returns detached signature bytes (3293 bytes).
    pub fn sign(&self, message: &[u8]) -> Result<Vec<u8>, crate::ZhenError> {
        let sk = dilithium3::SecretKey::from_bytes(&self.secret)
            .map_err(|_| crate::ZhenError::Transport("invalid ML-DSA secret key".into()))?;
        let sig = dilithium3::detached_sign(message, &sk);
        Ok(sig.as_bytes().to_vec())
    }

    /// Public key bytes for transmission.
    pub fn public_bytes(&self) -> &[u8] {
        &self.public
    }
}

impl Drop for PqSigningKeypair {
    fn drop(&mut self) {
        self.secret.zeroize();
    }
}

/// Verify a ML-DSA-65 detached signature against a public key.
///
/// ALL INPUTS HOSTILE. Public key and signature from untrusted peer.
pub fn verify(
    message: &[u8],
    signature: &[u8],
    public_key: &[u8],
) -> Result<bool, crate::ZhenError> {
    let pk = dilithium3::PublicKey::from_bytes(public_key)
        .map_err(|_| crate::ZhenError::Transport("invalid ML-DSA public key".into()))?;
    let sig = dilithium3::DetachedSignature::from_bytes(signature)
        .map_err(|_| crate::ZhenError::Transport("invalid ML-DSA signature".into()))?;

    match dilithium3::verify_detached_signature(&sig, message, &pk) {
        Ok(()) => Ok(true),
        Err(_) => Ok(false),
    }
}

/// Peer identity: public signing key + metadata for gossip admission control.
#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub struct PeerIdentity {
    /// ML-DSA-65 public key (1952 bytes).
    pub signing_key: Vec<u8>,
    /// Human-readable peer name (optional).
    pub name: Option<String>,
    /// Self-signed attestation: sign(signing_key || name).
    /// Proves the peer controls the private key.
    pub self_attestation: Vec<u8>,
}

impl PeerIdentity {
    /// Create a new peer identity with self-attestation.
    pub fn new(keypair: &PqSigningKeypair, name: Option<String>) -> Result<Self, crate::ZhenError> {
        // Build attestation message.
        let mut msg = Vec::new();
        msg.extend_from_slice(b"zhen-peer-identity-v1:");
        msg.extend_from_slice(&keypair.public);
        if let Some(ref n) = name {
            msg.extend_from_slice(b":");
            msg.extend_from_slice(n.as_bytes());
        }

        let attestation = keypair.sign(&msg)?;

        Ok(Self {
            signing_key: keypair.public.clone(),
            name,
            self_attestation: attestation,
        })
    }

    /// Verify this identity's self-attestation.
    /// Returns true if the peer provably controls the signing key.
    pub fn verify_self(&self) -> Result<bool, crate::ZhenError> {
        let mut msg = Vec::new();
        msg.extend_from_slice(b"zhen-peer-identity-v1:");
        msg.extend_from_slice(&self.signing_key);
        if let Some(ref n) = self.name {
            msg.extend_from_slice(b":");
            msg.extend_from_slice(n.as_bytes());
        }

        verify(&msg, &self.self_attestation, &self.signing_key)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_sign_and_verify() {
        let kp = PqSigningKeypair::generate();
        let message = b"the library that cannot burn";
        let sig = kp.sign(message).expect("signing should succeed");

        assert!(
            verify(message, &sig, kp.public_bytes()).expect("verify should not error"),
            "valid signature must verify"
        );
    }

    #[test]
    fn test_wrong_message_fails() {
        let kp = PqSigningKeypair::generate();
        let sig = kp.sign(b"original").expect("signing should succeed");

        assert!(
            !verify(b"tampered", &sig, kp.public_bytes()).expect("verify should not error"),
            "wrong message must not verify"
        );
    }

    #[test]
    fn test_wrong_key_fails() {
        let kp1 = PqSigningKeypair::generate();
        let kp2 = PqSigningKeypair::generate();
        let message = b"test message";
        let sig = kp1.sign(message).expect("signing should succeed");

        assert!(
            !verify(message, &sig, kp2.public_bytes()).expect("verify should not error"),
            "wrong key must not verify"
        );
    }

    #[test]
    fn test_peer_identity_self_attestation() {
        let kp = PqSigningKeypair::generate();
        let identity = PeerIdentity::new(&kp, Some("zhend-node-alpha".into()))
            .expect("identity creation should succeed");

        assert!(
            identity
                .verify_self()
                .expect("self-verification should not error"),
            "self-attestation must verify"
        );
    }

    #[test]
    fn test_peer_identity_tampered_attestation() {
        let kp = PqSigningKeypair::generate();
        let mut identity = PeerIdentity::new(&kp, Some("alpha".into())).unwrap();

        // Tamper with the name after attestation.
        identity.name = Some("evil-node".into());

        assert!(
            !identity.verify_self().expect("verify should not error"),
            "tampered identity must fail verification"
        );
    }

    #[test]
    fn test_truncated_signature_rejected() {
        let kp = PqSigningKeypair::generate();
        let result = verify(b"msg", &[0u8; 16], kp.public_bytes());
        // Truncated signature must either error or fail verification — never verify as valid.
        match result {
            Err(_) => {} // rejected at parse level — good
            Ok(valid) => assert!(!valid, "truncated signature must not verify as valid"),
        }
    }

    #[test]
    fn test_truncated_public_key_rejected() {
        let kp = PqSigningKeypair::generate();
        let sig = kp.sign(b"msg").unwrap();
        let result = verify(b"msg", &sig, &[0u8; 16]);
        assert!(result.is_err());
    }
}
