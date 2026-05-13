// SPDX-License-Identifier: GPL-3.0-or-later
//! Codec for Pu fragments — encode/decode for storage and gossip transport.
//!
//! Uses bincode for compact binary serialization. All decode operations
//! validate integrity (BLAKE3 check) before returning — ALL INPUTS HOSTILE.

use super::fragment::Fragment;
use crate::{ZhenError, ZhenResult};

/// Encode a fragment to compact binary representation.
pub fn encode(fragment: &Fragment) -> ZhenResult<Vec<u8>> {
    bincode::serde::encode_to_vec(fragment, bincode::config::standard())
        .map_err(|e| ZhenError::Serialization(e.to_string()))
}

/// Decode a fragment from binary, verifying integrity.
///
/// Returns error if:
/// - Deserialization fails (malformed input)
/// - BLAKE3 verification fails (tampered payload)
pub fn decode(bytes: &[u8]) -> ZhenResult<Fragment> {
    let (fragment, _consumed): (Fragment, usize) =
        bincode::serde::decode_from_slice(bytes, bincode::config::standard())
            .map_err(|e| ZhenError::Serialization(e.to_string()))?;

    if !fragment.verify() {
        return Err(ZhenError::IntegrityFailure {
            hash: fragment.id.to_hex(),
        });
    }

    Ok(fragment)
}

/// Encode a fragment for gossip transport (compact, no redundant fields).
/// Same as encode() for now — future optimization: strip fields not needed
/// for gossip (e.g., local access_count, tier).
pub fn encode_for_gossip(fragment: &Fragment) -> ZhenResult<Vec<u8>> {
    // TODO: strip local-only fields for leaner gossip payloads
    encode(fragment)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::pu::fragment::Fragment;

    #[test]
    fn test_roundtrip() {
        let frag = Fragment::new(b"roundtrip test".to_vec(), vec![1, 2, 3], None);
        let encoded = encode(&frag).expect("encode failed");
        let decoded = decode(&encoded).expect("decode failed");

        assert_eq!(frag.id, decoded.id);
        assert_eq!(frag.payload, decoded.payload);
        assert_eq!(frag.embedding, decoded.embedding);
    }

    #[test]
    fn test_tampered_payload_rejected() {
        let frag = Fragment::new(b"original".to_vec(), vec![], None);
        let mut encoded = encode(&frag).expect("encode failed");

        // Corrupt some bytes in the middle (payload region).
        if encoded.len() > 50 {
            encoded[50] ^= 0xFF;
        }

        // Should fail either deserialization or integrity check.
        let result = decode(&encoded);
        assert!(result.is_err());
    }

    #[test]
    fn test_empty_bytes_rejected() {
        let result = decode(&[]);
        assert!(result.is_err());
    }
}
