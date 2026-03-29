//! IPv6 Hop-by-Hop extension header parsing for Zhen option type.
//!
//! The Zhen HbH option carries fragment metadata piggybacked on
//! existing Monad packets. Format:
//!
//! ```text
//!  0                   1                   2                   3
//!  0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
//! +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
//! | Option Type   | Opt Data Len  |  Fragment Hash (BLAKE3)       |
//! +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+                               +
//! |                     (32 bytes total)                          |
//! +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
//! ```
//!
//! Option Type: TBD (IANA allocation via RFC Editor)
//! Change action: 00 (skip if unrecognized)
//! Mutable: YES (intermediate nodes MAY update metadata)

/// Zhen HbH option type (placeholder — pending IANA allocation).
pub const ZHEN_OPTION_TYPE: u8 = 0x3E; // experimental range

/// BLAKE3 hash length in bytes.
pub const BLAKE3_HASH_LEN: usize = 32;

/// Minimum Zhen HbH option length (type + len + hash).
pub const ZHEN_OPTION_MIN_LEN: usize = 2 + BLAKE3_HASH_LEN;

/// Parsed Zhen HbH option from a packet.
#[derive(Debug, Clone)]
pub struct ZhenHbhOption {
    /// BLAKE3 hash of the fragment being advertised.
    pub fragment_hash: [u8; 32],
    /// Embedding dimension (from option, if present).
    pub embedding_dim: Option<u8>,
    /// Payload length hint (from option, if present).
    pub payload_len: Option<u16>,
}

/// Parse a Zhen option from raw HbH option bytes.
///
/// Returns None if:
/// - Buffer too short
/// - Option type doesn't match
/// - Data length inconsistent
///
/// ALL INPUTS HOSTILE.
pub fn parse_zhen_option(data: &[u8]) -> Option<ZhenHbhOption> {
    if data.len() < ZHEN_OPTION_MIN_LEN {
        return None;
    }

    if data[0] != ZHEN_OPTION_TYPE {
        return None;
    }

    let opt_data_len = data[1] as usize;
    if data.len() < 2 + opt_data_len {
        return None;
    }

    if opt_data_len < BLAKE3_HASH_LEN {
        return None;
    }

    let mut hash = [0u8; 32];
    hash.copy_from_slice(&data[2..2 + 32]);

    let embedding_dim = if opt_data_len > BLAKE3_HASH_LEN {
        Some(data[2 + 32])
    } else {
        None
    };

    let payload_len = if opt_data_len > BLAKE3_HASH_LEN + 1 {
        Some(u16::from_be_bytes([data[2 + 33], data[2 + 34]]))
    } else {
        None
    };

    Some(ZhenHbhOption {
        fragment_hash: hash,
        embedding_dim,
        payload_len,
    })
}

/// Serialize a Zhen HbH option for insertion into outgoing packets.
pub fn encode_zhen_option(hash: &[u8; 32], embedding_dim: Option<u8>, payload_len: Option<u16>) -> Vec<u8> {
    let mut buf = Vec::with_capacity(ZHEN_OPTION_MIN_LEN + 3);

    buf.push(ZHEN_OPTION_TYPE);

    let data_len = BLAKE3_HASH_LEN
        + embedding_dim.map_or(0, |_| 1)
        + payload_len.map_or(0, |_| 2);
    buf.push(data_len as u8);

    buf.extend_from_slice(hash);

    if let Some(dim) = embedding_dim {
        buf.push(dim);
    }
    if let Some(plen) = payload_len {
        buf.extend_from_slice(&plen.to_be_bytes());
    }

    buf
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_roundtrip_minimal() {
        let hash = blake3::hash(b"test fragment").as_bytes().clone();
        let encoded = encode_zhen_option(&hash, None, None);
        let parsed = parse_zhen_option(&encoded).expect("should parse");
        assert_eq!(parsed.fragment_hash, hash);
        assert!(parsed.embedding_dim.is_none());
        assert!(parsed.payload_len.is_none());
    }

    #[test]
    fn test_roundtrip_full() {
        let hash = blake3::hash(b"full metadata").as_bytes().clone();
        let encoded = encode_zhen_option(&hash, Some(128), Some(4096));
        let parsed = parse_zhen_option(&encoded).expect("should parse");
        assert_eq!(parsed.fragment_hash, hash);
        assert_eq!(parsed.embedding_dim, Some(128));
        assert_eq!(parsed.payload_len, Some(4096));
    }

    #[test]
    fn test_reject_short_buffer() {
        assert!(parse_zhen_option(&[]).is_none());
        assert!(parse_zhen_option(&[ZHEN_OPTION_TYPE]).is_none());
        assert!(parse_zhen_option(&[ZHEN_OPTION_TYPE, 0xFF]).is_none());
    }

    #[test]
    fn test_reject_wrong_type() {
        let mut encoded = encode_zhen_option(&[0u8; 32], None, None);
        encoded[0] = 0x00; // wrong type
        assert!(parse_zhen_option(&encoded).is_none());
    }
}
