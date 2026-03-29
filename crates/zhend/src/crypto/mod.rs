//! # Crypto — Post-Quantum Hardened Cryptography for Zhen
//!
//! **Sacred Law: "That which remembers forever must be armored against forever."**
//!
//! All asymmetric cryptography in Zhen uses hybrid constructions:
//! - Key encapsulation: ML-KEM-768/1024 (Kyber) + X25519
//! - Digital signatures: ML-DSA-65 (Dilithium)
//! - Symmetric encryption: AES-256-GCM (quantum-safe at 128-bit post-Grover)
//! - Key derivation: HKDF-SHA256 over concatenated shared secrets
//!
//! Classical-only asymmetric crypto is FORBIDDEN for any operation protecting
//! data with lifetime > 1 year. This is a hard architectural constraint.
//!
//! ## Why Hybrid?
//!
//! If ML-KEM is broken by cryptanalysis, X25519 still holds.
//! If X25519 is broken by quantum computer, ML-KEM still holds.
//! Both must be broken simultaneously to compromise the system.
//! The cost is ~1.5KB extra per handshake. The alternative is a Library
//! whose encryption has an expiration date. Unacceptable.

#[cfg(feature = "pq")]
pub mod kem;
#[cfg(feature = "pq")]
pub mod sign;
#[cfg(feature = "pq")]
pub mod envelope;

// Re-export key types when PQ is enabled.
#[cfg(feature = "pq")]
pub use kem::HybridKemKeypair;
#[cfg(feature = "pq")]
pub use sign::PqSigningKeypair;
#[cfg(feature = "pq")]
pub use envelope::SealedFragment;
