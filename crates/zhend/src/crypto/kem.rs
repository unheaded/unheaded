//! Hybrid Key Encapsulation Mechanism: ML-KEM-768 + X25519
//!
//! Belt-and-suspenders approach to key exchange. The shared secret is
//! derived from BOTH classical and post-quantum components via HKDF.
//! An adversary must break both to recover the shared secret.
//!
//! Used for:
//! - Gossip peer session key establishment
//! - Per-fragment payload encryption key encapsulation
//!
//! Wire format of encapsulated key:
//! ```text
//! [ml_kem_ciphertext (1088 bytes)] [x25519_public (32 bytes)]
//! ```
//! Total: 1120 bytes for ML-KEM-768 hybrid, 1600 bytes for ML-KEM-1024 hybrid.

use zeroize::Zeroize;

#[cfg(feature = "pq")]
// 2026-05-08 (Wave B): pqcrypto-kyber → pqcrypto-mlkem (FIPS 203 standardized name).
use pqcrypto_mlkem::mlkem768 as kyber768;
#[cfg(feature = "pq")]
use pqcrypto_traits::kem::{
    Ciphertext as KemCt, PublicKey as KemPk, SecretKey as KemSk, SharedSecret as KemSs,
};

/// Hybrid KEM keypair: ML-KEM-768 + X25519.
pub struct HybridKemKeypair {
    /// ML-KEM-768 public key (1184 bytes).
    pub pq_public: Vec<u8>,
    /// ML-KEM-768 secret key (2400 bytes). ZEROIZE ON DROP.
    pq_secret: Vec<u8>,
    /// X25519 static secret.
    classical_secret: x25519_dalek::StaticSecret,
    /// X25519 public key (32 bytes).
    pub classical_public: x25519_dalek::PublicKey,
}

impl HybridKemKeypair {
    /// Generate a fresh hybrid keypair.
    pub fn generate() -> Self {
        // ML-KEM-768 keypair.
        let (pq_pk, pq_sk) = kyber768::keypair();
        let pq_public = pq_pk.as_bytes().to_vec();
        let pq_secret = pq_sk.as_bytes().to_vec();

        // X25519 keypair.
        let classical_secret = x25519_dalek::StaticSecret::random_from_rng(rand::thread_rng());
        let classical_public = x25519_dalek::PublicKey::from(&classical_secret);

        Self {
            pq_public,
            pq_secret,
            classical_secret,
            classical_public,
        }
    }

    /// Combined public key bytes for transmission to peer.
    /// Format: [ml_kem_public (1184 bytes)][x25519_public (32 bytes)]
    pub fn public_bytes(&self) -> Vec<u8> {
        let mut out = Vec::with_capacity(self.pq_public.len() + 32);
        out.extend_from_slice(&self.pq_public);
        out.extend_from_slice(self.classical_public.as_bytes());
        out
    }
}

impl Drop for HybridKemKeypair {
    fn drop(&mut self) {
        self.pq_secret.zeroize();
    }
}

/// Result of hybrid encapsulation (sender side).
pub struct EncapsulatedKey {
    /// Combined ciphertext to send to recipient.
    /// Format: [ml_kem_ciphertext][x25519_ephemeral_public]
    pub ciphertext: Vec<u8>,
    /// Derived shared secret (32 bytes). ZEROIZE ON DROP.
    pub shared_secret: SharedSecret,
}

/// Zeroizing wrapper around a 32-byte shared secret.
pub struct SharedSecret(Vec<u8>);

impl SharedSecret {
    pub fn as_bytes(&self) -> &[u8] {
        &self.0
    }
}

impl Drop for SharedSecret {
    fn drop(&mut self) {
        self.0.zeroize();
    }
}

/// Encapsulate: generate a shared secret for a recipient's hybrid public key.
///
/// Returns the ciphertext (to send) and the shared secret (to use for AES-GCM).
pub fn encapsulate(recipient_public: &[u8]) -> Result<EncapsulatedKey, crate::ZhenError> {
    // Parse recipient's combined public key.
    let pq_pk_len = kyber768::public_key_bytes();
    if recipient_public.len() < pq_pk_len + 32 {
        return Err(crate::ZhenError::Transport(
            "recipient public key too short for hybrid KEM".into(),
        ));
    }

    let pq_pk_bytes = &recipient_public[..pq_pk_len];
    let x25519_pk_bytes = &recipient_public[pq_pk_len..pq_pk_len + 32];

    // ML-KEM-768 encapsulation.
    let pq_pk = kyber768::PublicKey::from_bytes(pq_pk_bytes)
        .map_err(|_| crate::ZhenError::Transport("invalid ML-KEM public key".into()))?;
    let (pq_ss, pq_ct) = kyber768::encapsulate(&pq_pk);

    // X25519 ephemeral key exchange.
    let eph_secret = x25519_dalek::StaticSecret::random_from_rng(rand::thread_rng());
    let eph_public = x25519_dalek::PublicKey::from(&eph_secret);

    let mut x25519_pk = [0u8; 32];
    x25519_pk.copy_from_slice(x25519_pk_bytes);
    let recipient_x25519 = x25519_dalek::PublicKey::from(x25519_pk);
    let classical_ss = eph_secret.diffie_hellman(&recipient_x25519);

    // Combine shared secrets via HKDF-SHA256.
    let combined = derive_shared_secret(pq_ss.as_bytes(), classical_ss.as_bytes());

    // Build combined ciphertext.
    let mut ciphertext = Vec::with_capacity(pq_ct.as_bytes().len() + 32);
    ciphertext.extend_from_slice(pq_ct.as_bytes());
    ciphertext.extend_from_slice(eph_public.as_bytes());

    Ok(EncapsulatedKey {
        ciphertext,
        shared_secret: SharedSecret(combined),
    })
}

/// Decapsulate: recover the shared secret from ciphertext using our keypair.
pub fn decapsulate(
    keypair: &HybridKemKeypair,
    ciphertext: &[u8],
) -> Result<SharedSecret, crate::ZhenError> {
    let pq_ct_len = kyber768::ciphertext_bytes();
    if ciphertext.len() < pq_ct_len + 32 {
        return Err(crate::ZhenError::Transport(
            "ciphertext too short for hybrid KEM".into(),
        ));
    }

    let pq_ct_bytes = &ciphertext[..pq_ct_len];
    let eph_pk_bytes = &ciphertext[pq_ct_len..pq_ct_len + 32];

    // ML-KEM-768 decapsulation.
    let pq_sk = kyber768::SecretKey::from_bytes(&keypair.pq_secret)
        .map_err(|_| crate::ZhenError::Transport("invalid ML-KEM secret key".into()))?;
    let pq_ct = kyber768::Ciphertext::from_bytes(pq_ct_bytes)
        .map_err(|_| crate::ZhenError::Transport("invalid ML-KEM ciphertext".into()))?;
    let pq_ss = kyber768::decapsulate(&pq_ct, &pq_sk);

    // X25519 decapsulation.
    let mut eph_pk = [0u8; 32];
    eph_pk.copy_from_slice(eph_pk_bytes);
    let eph_public = x25519_dalek::PublicKey::from(eph_pk);
    let classical_ss = keypair.classical_secret.diffie_hellman(&eph_public);

    // Combine via HKDF-SHA256.
    let combined = derive_shared_secret(pq_ss.as_bytes(), classical_ss.as_bytes());

    Ok(SharedSecret(combined))
}

/// Derive a 32-byte shared secret from PQ + classical shared secrets via HKDF-SHA256.
fn derive_shared_secret(pq_ss: &[u8], classical_ss: &[u8]) -> Vec<u8> {
    use hkdf::Hkdf;
    use sha2::Sha256;

    // IKM = pq_shared_secret || classical_shared_secret
    let mut ikm = Vec::with_capacity(pq_ss.len() + classical_ss.len());
    ikm.extend_from_slice(pq_ss);
    ikm.extend_from_slice(classical_ss);

    // Salt = domain separator
    let salt = b"zhen-hybrid-kem-v1";

    // Info = context binding
    let info = b"fragment-encryption";

    let hk = Hkdf::<Sha256>::new(Some(salt), &ikm);
    let mut okm = vec![0u8; 32];
    hk.expand(info, &mut okm)
        .expect("HKDF-SHA256 expand should not fail for 32-byte output");

    // Zeroize intermediate.
    let mut ikm_z = ikm;
    ikm_z.zeroize();

    okm
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_hybrid_kem_roundtrip() {
        let recipient = HybridKemKeypair::generate();
        let pub_bytes = recipient.public_bytes();

        let encap = encapsulate(&pub_bytes).expect("encapsulation should succeed");
        let decap =
            decapsulate(&recipient, &encap.ciphertext).expect("decapsulation should succeed");

        assert_eq!(
            encap.shared_secret.as_bytes(),
            decap.as_bytes(),
            "sender and recipient must derive the same shared secret"
        );
    }

    #[test]
    fn test_different_keypairs_different_secrets() {
        let alice = HybridKemKeypair::generate();
        let bob = HybridKemKeypair::generate();

        let encap_alice = encapsulate(&alice.public_bytes()).unwrap();
        let encap_bob = encapsulate(&bob.public_bytes()).unwrap();

        // Different recipients → different shared secrets.
        assert_ne!(
            encap_alice.shared_secret.as_bytes(),
            encap_bob.shared_secret.as_bytes(),
        );
    }

    #[test]
    fn test_wrong_keypair_decapsulation() {
        let alice = HybridKemKeypair::generate();
        let bob = HybridKemKeypair::generate();

        let encap = encapsulate(&alice.public_bytes()).unwrap();

        // Bob tries to decapsulate Alice's ciphertext — should produce different secret.
        // ML-KEM has implicit rejection: wrong key → random-looking output, not error.
        let wrong_ss = decapsulate(&bob, &encap.ciphertext).unwrap();
        assert_ne!(
            encap.shared_secret.as_bytes(),
            wrong_ss.as_bytes(),
            "wrong key must not produce correct shared secret"
        );
    }

    #[test]
    fn test_truncated_public_key_rejected() {
        let result = encapsulate(&[0u8; 16]);
        assert!(result.is_err());
    }

    #[test]
    fn test_truncated_ciphertext_rejected() {
        let kp = HybridKemKeypair::generate();
        let result = decapsulate(&kp, &[0u8; 16]);
        assert!(result.is_err());
    }
}
