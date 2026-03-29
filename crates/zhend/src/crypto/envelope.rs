//! # Sealed Fragment Envelope
//!
//! Encrypts a Pu fragment payload using hybrid ML-KEM + X25519 key
//! encapsulation with AES-256-GCM symmetric encryption.
//!
//! Wire format:
//! ```text
//! ┌─────────────────────────────────────────┐
//! │ version: u8 (0x01)                      │
//! ├─────────────────────────────────────────┤
//! │ kem_ciphertext: [u8; 1120]              │ ← ML-KEM-768 ct + X25519 eph pk
//! ├─────────────────────────────────────────┤
//! │ nonce: [u8; 12]                         │ ← AES-256-GCM nonce
//! ├─────────────────────────────────────────┤
//! │ ciphertext_len: u32 (LE)                │
//! ├─────────────────────────────────────────┤
//! │ ciphertext: [u8; ciphertext_len]        │ ← AES-256-GCM(payload)
//! ├─────────────────────────────────────────┤
//! │ tag: [u8; 16]                           │ ← AES-256-GCM auth tag
//! └─────────────────────────────────────────┘
//! ```
//!
//! The BLAKE3 fragment ID is computed over the PLAINTEXT payload,
//! not the sealed envelope. This means a sealed fragment and its
//! unsealed counterpart share the same FragmentId. Content identity
//! transcends encryption.

use aes_gcm::{
    aead::{Aead, KeyInit},
    Aes256Gcm, Nonce,
};
use zeroize::Zeroize;

use super::kem;
use crate::{ZhenError, ZhenResult};

/// Current envelope version.
const ENVELOPE_VERSION: u8 = 0x01;

/// A sealed (encrypted) fragment payload.
#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub struct SealedFragment {
    /// Envelope version for forward compatibility.
    pub version: u8,
    /// Hybrid KEM ciphertext (ML-KEM-768 ct + X25519 ephemeral public).
    pub kem_ciphertext: Vec<u8>,
    /// AES-256-GCM nonce (12 bytes, random per seal operation).
    pub nonce: Vec<u8>,
    /// AES-256-GCM encrypted payload.
    pub ciphertext: Vec<u8>,
}

/// Seal a fragment payload for a specific recipient.
///
/// Generates a fresh ephemeral keypair, performs hybrid KEM encapsulation,
/// derives AES-256-GCM key via HKDF, encrypts payload.
///
/// The recipient's hybrid public key must be [ML-KEM-768 pk || X25519 pk].
pub fn seal(
    plaintext: &[u8],
    recipient_public: &[u8],
) -> ZhenResult<SealedFragment> {
    // Hybrid KEM: derive shared secret.
    let encap = kem::encapsulate(recipient_public)?;

    // AES-256-GCM encrypt.
    let key = aes_gcm::Key::<Aes256Gcm>::from_slice(encap.shared_secret.as_bytes());
    let cipher = Aes256Gcm::new(key);

    // Random 96-bit nonce.
    let mut nonce_bytes = [0u8; 12];
    use rand::RngCore;
    rand::thread_rng().fill_bytes(&mut nonce_bytes);
    let nonce = Nonce::from_slice(&nonce_bytes);

    let ciphertext = cipher
        .encrypt(nonce, plaintext)
        .map_err(|e| ZhenError::Transport(format!("AES-GCM seal failed: {e}")))?;

    Ok(SealedFragment {
        version: ENVELOPE_VERSION,
        kem_ciphertext: encap.ciphertext,
        nonce: nonce_bytes.to_vec(),
        ciphertext,
    })
}

/// Unseal a fragment payload using our hybrid keypair.
///
/// Performs hybrid KEM decapsulation, derives AES-256-GCM key,
/// decrypts and authenticates payload.
///
/// Returns the plaintext payload on success. ALL INPUTS HOSTILE.
pub fn unseal(
    sealed: &SealedFragment,
    keypair: &kem::HybridKemKeypair,
) -> ZhenResult<Vec<u8>> {
    // Version check.
    if sealed.version != ENVELOPE_VERSION {
        return Err(ZhenError::Transport(format!(
            "unsupported envelope version: {} (expected {})",
            sealed.version, ENVELOPE_VERSION
        )));
    }

    // Nonce length check.
    if sealed.nonce.len() != 12 {
        return Err(ZhenError::Transport(format!(
            "invalid nonce length: {} (expected 12)",
            sealed.nonce.len()
        )));
    }

    // Hybrid KEM: recover shared secret.
    let shared = kem::decapsulate(keypair, &sealed.kem_ciphertext)?;

    // AES-256-GCM decrypt.
    let key = aes_gcm::Key::<Aes256Gcm>::from_slice(shared.as_bytes());
    let cipher = Aes256Gcm::new(key);
    let nonce = Nonce::from_slice(&sealed.nonce);

    let plaintext = cipher
        .decrypt(nonce, sealed.ciphertext.as_ref())
        .map_err(|_| ZhenError::IntegrityFailure {
            hash: "sealed-fragment".into(),
        })?;

    Ok(plaintext)
}

/// Estimated sealed envelope overhead in bytes (excluding payload).
/// KEM ciphertext + nonce + GCM tag + version + length prefix.
pub const fn envelope_overhead() -> usize {
    1       // version
    + 1120  // ML-KEM-768 ct (1088) + X25519 eph pk (32)
    + 12    // nonce
    + 16    // AES-GCM tag (included in ciphertext by aes-gcm crate)
    + 4     // length prefix
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::crypto::kem::HybridKemKeypair;

    #[test]
    fn test_seal_unseal_roundtrip() {
        let recipient = HybridKemKeypair::generate();
        let plaintext = b"the dao that can be named is not the eternal dao";

        let sealed = seal(plaintext, &recipient.public_bytes())
            .expect("seal should succeed");

        let recovered = unseal(&sealed, &recipient)
            .expect("unseal should succeed");

        assert_eq!(&recovered, plaintext);
    }

    #[test]
    fn test_wrong_key_fails_unseal() {
        let alice = HybridKemKeypair::generate();
        let bob = HybridKemKeypair::generate();

        let sealed = seal(b"for alice only", &alice.public_bytes()).unwrap();

        // Bob tries to unseal Alice's fragment.
        let result = unseal(&sealed, &bob);
        assert!(result.is_err(), "wrong key must fail to unseal");
    }

    #[test]
    fn test_tampered_ciphertext_fails() {
        let kp = HybridKemKeypair::generate();
        let mut sealed = seal(b"pristine", &kp.public_bytes()).unwrap();

        // Flip a byte in the ciphertext.
        if let Some(byte) = sealed.ciphertext.last_mut() {
            *byte ^= 0xFF;
        }

        let result = unseal(&sealed, &kp);
        assert!(result.is_err(), "tampered ciphertext must fail authentication");
    }

    #[test]
    fn test_tampered_nonce_fails() {
        let kp = HybridKemKeypair::generate();
        let mut sealed = seal(b"pristine", &kp.public_bytes()).unwrap();

        sealed.nonce[0] ^= 0xFF;

        let result = unseal(&sealed, &kp);
        assert!(result.is_err(), "tampered nonce must fail");
    }

    #[test]
    fn test_empty_payload() {
        let kp = HybridKemKeypair::generate();
        let sealed = seal(b"", &kp.public_bytes()).unwrap();
        let recovered = unseal(&sealed, &kp).unwrap();
        assert!(recovered.is_empty());
    }

    #[test]
    fn test_large_payload() {
        let kp = HybridKemKeypair::generate();
        let large = vec![0xAB_u8; 1024 * 1024]; // 1MB
        let sealed = seal(&large, &kp.public_bytes()).unwrap();
        let recovered = unseal(&sealed, &kp).unwrap();
        assert_eq!(recovered, large);
    }

    #[test]
    fn test_version_check() {
        let kp = HybridKemKeypair::generate();
        let mut sealed = seal(b"test", &kp.public_bytes()).unwrap();
        sealed.version = 0xFF;
        let result = unseal(&sealed, &kp);
        assert!(result.is_err());
    }

    #[test]
    fn test_envelope_overhead_constant() {
        // Sanity check the overhead constant.
        assert!(envelope_overhead() > 1100, "overhead must include KEM ciphertext");
        assert!(envelope_overhead() < 1200, "overhead should not be unreasonably large");
    }
}
