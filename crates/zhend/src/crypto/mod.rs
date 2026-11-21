// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.
//! # Crypto — Post-Quantum Signatures for Zhen
//!
//! **Sacred Law: "That which remembers forever must be armored against forever."**
//!
//! This module holds peer identity signatures: ML-DSA-65 (FIPS 204), in
//! [`sign`]. zhend has no transport encryption: gRPC is plaintext on loopback
//! and gossip is UDP. The QUIC server and its TLS 1.3 (rustls) were never
//! spawned and are archived in `unheaded-zhend-subsystems` (ADR-152).
//!
//! The hybrid ML-KEM-768 + X25519 key encapsulation (`kem`) and the
//! AES-256-GCM sealed fragment envelope (`envelope`) that used to live here
//! had no callers and were deleted on 2026-11-18. Nothing in zhend encrypts a
//! fragment payload.

#[cfg(feature = "pq")]
pub mod sign;

// Re-export key types when PQ is enabled.
#[cfg(feature = "pq")]
pub use sign::PqSigningKeypair;
