// SPDX-License-Identifier: GPL-2.0-only

//! # pqc-common
//!
//! Shared BPF-side types for Post-Quantum Cryptographic authentication.
//! Implements signature-by-reference: full PQC signatures live in Sophia
//! BPF maps, while the frozen Monad wire format carries 12-byte references.

#![cfg_attr(not(test), no_std)]
#![deny(missing_docs)]

/// PQC Algorithm identifiers (1 byte)
#[repr(u8)]
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum AlgoId {
    /// FIPS 205 - SLH-DSA (Hash-based signature)
    SlhDsa = 0x01,
    /// FIPS 204 - ML-DSA (Lattice signature)
    MlDsa = 0x02,
    /// FIPS 206 - FN-DSA (Lattice signature, userspace-only)
    FnDsa = 0x03,
    /// FIPS 203 - ML-KEM (Lattice KEM)
    MlKem = 0x04,
    /// FIPS 207 - HQC (Code-based KEM)
    Hqc = 0x05,
}

/// 12-byte PQC value carried in the Monad scratch bytes.
///
/// Layout (network byte order):
/// ```text
/// Byte  0-3:  sig_ref   (u32) — Sophia PQC_SIG_MAP index
/// Byte  4-5:  key_ref   (u16) — Sophia PQC_KEY_MAP index
/// Byte  6-9:  hash_pfx  ([u8; 4]) — SHA-256(signature)[0:4]
/// Byte 10-11: seq_num   (u16) — per-flow replay protection
/// ```
#[repr(C, packed)]
#[derive(Clone, Copy)]
pub struct PqcValue {
    /// Index into Sophia PQC_SIG_MAP BPF map
    pub sig_ref: u32,
    /// Index into Sophia PQC_KEY_MAP BPF map
    pub key_ref: u16,
    /// SHA-256(signature)[0:4] prefix
    pub hash_pfx: [u8; 4],
    /// Per-flow sequence number
    pub seq_num: u16,
}

/// Size of PqcValue in bytes
pub const PQC_VALUE_SIZE: usize = 12;

// Compile-time layout assertion — no runtime cost.
const _: () = {
    let _ = [(); PQC_VALUE_SIZE - core::mem::size_of::<PqcValue>()];
    let _ = [(); core::mem::size_of::<PqcValue>() - PQC_VALUE_SIZE];
};

/// PQC signature verification status stored in BPF map
#[repr(u8)]
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum PqcSigStatus {
    /// Not yet verified
    Pending = 0x00,
    /// Verification passed
    Valid = 0x01,
    /// Verification failed
    Invalid = 0x02,
    /// Key expired or revoked
    Expired = 0x03,
    /// Algorithm not supported
    Unsupported = 0x04,
}

/// Compliance tier (2-bit Kingdom Mode K1|K0)
#[repr(u8)]
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum ComplianceTier {
    /// No PQC required
    None = 0x00,
    /// Standard: at least one PQC signature
    Standard = 0x01,
    /// Enhanced: PQC + specific algorithm requirements
    Enhanced = 0x02,
    /// Sovereign: 2-of-3 cross-algorithm multi-sig
    Sovereign = 0x03,
}

/// PQC flags for the S+CUSTOM dual-flag
pub const FLAG_SAMPLED: u8 = 0x08;
/// Custom flag
pub const FLAG_CUSTOM: u8 = 0x02;
/// PQC active = S+CUSTOM both set
pub const FLAG_PQC_ACTIVE: u8 = FLAG_SAMPLED | FLAG_CUSTOM;

/// Check if PQC authentication is active in flags byte
#[inline]
pub fn is_pqc_active(flags: u8) -> bool {
    flags & FLAG_PQC_ACTIVE == FLAG_PQC_ACTIVE
}

impl PqcValue {
    /// Check if the PQC value is all zeros (no authentication)
    #[inline]
    pub fn is_zero(&self) -> bool {
        self.sig_ref == 0 && self.key_ref == 0 && self.hash_pfx == [0; 4] && self.seq_num == 0
    }

    /// Read sig_ref in network byte order
    #[inline]
    pub fn sig_ref_be(&self) -> u32 {
        u32::from_be(self.sig_ref)
    }

    /// Read key_ref in network byte order
    #[inline]
    pub fn key_ref_be(&self) -> u16 {
        u16::from_be(self.key_ref)
    }

    /// Read seq_num in network byte order
    #[inline]
    pub fn seq_num_be(&self) -> u16 {
        u16::from_be(self.seq_num)
    }
}

/// BPF map entry for cached signature verification status
#[repr(C, packed)]
#[derive(Clone, Copy)]
pub struct PqcSigStatusEntry {
    /// Verification status
    pub status: PqcSigStatus,
    /// Algorithm used
    pub algo_id: u8,
    /// Compliance tier
    pub tier: ComplianceTier,
    /// Padding for alignment
    pub _pad: u8,
    /// Timestamp of last verification (unix seconds, truncated to u32)
    pub verified_at: u32,
}

/// Size of PqcSigStatusEntry in bytes
pub const PQC_SIG_STATUS_ENTRY_SIZE: usize = 8;

// Compile-time layout assertion for PqcSigStatusEntry.
const _: () = {
    let _ = [(); PQC_SIG_STATUS_ENTRY_SIZE - core::mem::size_of::<PqcSigStatusEntry>()];
    let _ = [(); core::mem::size_of::<PqcSigStatusEntry>() - PQC_SIG_STATUS_ENTRY_SIZE];
};

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn pqc_value_size() {
        assert_eq!(core::mem::size_of::<PqcValue>(), PQC_VALUE_SIZE);
        assert_eq!(PQC_VALUE_SIZE, 12);
    }

    #[test]
    fn pqc_sig_status_entry_size() {
        assert_eq!(
            core::mem::size_of::<PqcSigStatusEntry>(),
            PQC_SIG_STATUS_ENTRY_SIZE
        );
    }

    #[test]
    fn is_zero_on_default() {
        let v = PqcValue {
            sig_ref: 0,
            key_ref: 0,
            hash_pfx: [0; 4],
            seq_num: 0,
        };
        assert!(v.is_zero());
    }

    #[test]
    fn is_zero_nonzero_sig_ref() {
        let v = PqcValue {
            sig_ref: 1,
            key_ref: 0,
            hash_pfx: [0; 4],
            seq_num: 0,
        };
        assert!(!v.is_zero());
    }

    #[test]
    fn flag_pqc_active_value() {
        assert_eq!(FLAG_PQC_ACTIVE, 0x0A);
        assert_eq!(FLAG_SAMPLED, 0x08);
        assert_eq!(FLAG_CUSTOM, 0x02);
    }

    #[test]
    fn is_pqc_active_checks() {
        assert!(!is_pqc_active(0x00));
        assert!(!is_pqc_active(FLAG_SAMPLED));
        assert!(!is_pqc_active(FLAG_CUSTOM));
        assert!(is_pqc_active(FLAG_PQC_ACTIVE));
        assert!(is_pqc_active(0xFF));
    }

    #[test]
    fn algo_id_values() {
        assert_eq!(AlgoId::SlhDsa as u8, 0x01);
        assert_eq!(AlgoId::MlDsa as u8, 0x02);
        assert_eq!(AlgoId::FnDsa as u8, 0x03);
        assert_eq!(AlgoId::MlKem as u8, 0x04);
        assert_eq!(AlgoId::Hqc as u8, 0x05);
    }

    #[test]
    fn compliance_tier_values() {
        assert_eq!(ComplianceTier::None as u8, 0x00);
        assert_eq!(ComplianceTier::Standard as u8, 0x01);
        assert_eq!(ComplianceTier::Enhanced as u8, 0x02);
        assert_eq!(ComplianceTier::Sovereign as u8, 0x03);
    }

    #[test]
    fn sig_status_values() {
        assert_eq!(PqcSigStatus::Pending as u8, 0x00);
        assert_eq!(PqcSigStatus::Valid as u8, 0x01);
        assert_eq!(PqcSigStatus::Invalid as u8, 0x02);
        assert_eq!(PqcSigStatus::Expired as u8, 0x03);
        assert_eq!(PqcSigStatus::Unsupported as u8, 0x04);
    }

    #[test]
    fn flag_constants_match_monad_common() {
        // Verify our constants match the monad-common flags module
        assert_eq!(FLAG_SAMPLED, monad_common::flags::SAMPLED);
        assert_eq!(FLAG_CUSTOM, monad_common::flags::CUSTOM);
    }
}
