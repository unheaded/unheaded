//! Gossip protocol messages exchanged between zhend peers.
//!
//! All messages are bincode-serialized for compact wire representation.
//! ALL INPUTS HOSTILE — every deserialized message is validated before use.

use super::transport::MAX_MSG_SIZE;
use crate::{ZhenError, ZhenResult};
use serde::{Deserialize, Serialize};

#[cfg(feature = "pq")]
use crate::crypto::sign::PeerIdentity;

/// Messages exchanged between zhend peers over the gossip transport.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum GossipMessage {
    /// "I have these fragments" — send BLAKE3 digests for pull-on-demand.
    DigestBatch {
        /// BLAKE3 hashes of fragments this node has.
        digests: Vec<[u8; 32]>,
    },

    /// "I want these fragments" — request by hash.
    WantBatch {
        /// BLAKE3 hashes of fragments the requester wants.
        wanted: Vec<[u8; 32]>,
    },

    /// "Here are the fragments you wanted" — encoded fragment payloads.
    FragmentBatch {
        /// Bincode-encoded fragments (full Fragment structs).
        fragments: Vec<Vec<u8>>,
    },

    /// Peer membership ping (SWIM protocol).
    Ping {
        /// Sender's incarnation number for state disambiguation.
        incarnation: u64,
    },

    /// Peer membership ping response.
    PingAck {
        /// Responder's incarnation number.
        incarnation: u64,
        /// How many fragments this peer currently holds.
        fragment_count: u64,
    },

    /// PQ identity offer — "here is my ML-DSA-65 identity, verify me."
    #[cfg(feature = "pq")]
    IdentityOffer {
        /// The sender's PQ peer identity with self-attestation.
        identity: PeerIdentity,
    },

    /// PQ identity acknowledgement — "I verified you, here is mine."
    #[cfg(feature = "pq")]
    IdentityAck {
        /// The responder's PQ peer identity with self-attestation.
        identity: PeerIdentity,
    },
}

impl GossipMessage {
    /// Serialize to wire format (bincode).
    pub fn encode(&self) -> ZhenResult<Vec<u8>> {
        let bytes =
            bincode::serialize(self).map_err(|e| ZhenError::Serialization(e.to_string()))?;

        if bytes.len() > MAX_MSG_SIZE {
            return Err(ZhenError::Transport(format!(
                "encoded message too large: {} bytes > {} max",
                bytes.len(),
                MAX_MSG_SIZE
            )));
        }

        Ok(bytes)
    }

    /// Deserialize from wire format (bincode).
    ///
    /// ALL INPUTS HOSTILE — validates size before deserializing.
    pub fn decode(bytes: &[u8]) -> ZhenResult<Self> {
        if bytes.is_empty() {
            return Err(ZhenError::Gossip("empty message".into()));
        }

        if bytes.len() > MAX_MSG_SIZE {
            return Err(ZhenError::Gossip(format!(
                "message too large: {} bytes > {} max",
                bytes.len(),
                MAX_MSG_SIZE
            )));
        }

        bincode::deserialize(bytes).map_err(|e| ZhenError::Serialization(e.to_string()))
    }
}

/// Maximum number of digests per DigestBatch message.
/// Each digest is 32 bytes. At 1500 digests we're ~48KB, leaving room for framing.
pub const MAX_DIGESTS_PER_BATCH: usize = 1500;

/// Maximum total payload bytes per FragmentBatch message.
/// Fragment batches are chunked to stay under MAX_MSG_SIZE.
pub const MAX_FRAGMENT_BATCH_BYTES: usize = 60_000;

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_digest_batch_roundtrip() {
        let msg = GossipMessage::DigestBatch {
            digests: vec![[42u8; 32], [99u8; 32]],
        };
        let encoded = msg.encode().unwrap();
        let decoded = GossipMessage::decode(&encoded).unwrap();

        match decoded {
            GossipMessage::DigestBatch { digests } => {
                assert_eq!(digests.len(), 2);
                assert_eq!(digests[0], [42u8; 32]);
                assert_eq!(digests[1], [99u8; 32]);
            }
            _ => panic!("wrong variant"),
        }
    }

    #[test]
    fn test_want_batch_roundtrip() {
        let msg = GossipMessage::WantBatch {
            wanted: vec![[1u8; 32]],
        };
        let encoded = msg.encode().unwrap();
        let decoded = GossipMessage::decode(&encoded).unwrap();

        match decoded {
            GossipMessage::WantBatch { wanted } => {
                assert_eq!(wanted.len(), 1);
                assert_eq!(wanted[0], [1u8; 32]);
            }
            _ => panic!("wrong variant"),
        }
    }

    #[test]
    fn test_fragment_batch_roundtrip() {
        let msg = GossipMessage::FragmentBatch {
            fragments: vec![vec![1, 2, 3], vec![4, 5, 6]],
        };
        let encoded = msg.encode().unwrap();
        let decoded = GossipMessage::decode(&encoded).unwrap();

        match decoded {
            GossipMessage::FragmentBatch { fragments } => {
                assert_eq!(fragments.len(), 2);
                assert_eq!(fragments[0], vec![1, 2, 3]);
            }
            _ => panic!("wrong variant"),
        }
    }

    #[test]
    fn test_ping_roundtrip() {
        let msg = GossipMessage::Ping { incarnation: 42 };
        let encoded = msg.encode().unwrap();
        let decoded = GossipMessage::decode(&encoded).unwrap();

        match decoded {
            GossipMessage::Ping { incarnation } => assert_eq!(incarnation, 42),
            _ => panic!("wrong variant"),
        }
    }

    #[test]
    fn test_ping_ack_roundtrip() {
        let msg = GossipMessage::PingAck {
            incarnation: 7,
            fragment_count: 1024,
        };
        let encoded = msg.encode().unwrap();
        let decoded = GossipMessage::decode(&encoded).unwrap();

        match decoded {
            GossipMessage::PingAck {
                incarnation,
                fragment_count,
            } => {
                assert_eq!(incarnation, 7);
                assert_eq!(fragment_count, 1024);
            }
            _ => panic!("wrong variant"),
        }
    }

    #[test]
    fn test_empty_input_rejected() {
        let result = GossipMessage::decode(&[]);
        assert!(result.is_err());
    }

    #[test]
    fn test_garbage_input_rejected() {
        let result = GossipMessage::decode(&[0xFF, 0xFE, 0xFD]);
        assert!(result.is_err());
    }

    #[cfg(feature = "pq")]
    mod pq_message_tests {
        use super::*;
        use crate::crypto::sign::{PeerIdentity, PqSigningKeypair};

        #[test]
        fn test_identity_offer_roundtrip() {
            let kp = PqSigningKeypair::generate();
            let identity = PeerIdentity::new(&kp, Some("offer-node".into())).unwrap();

            let msg = GossipMessage::IdentityOffer { identity };
            let encoded = msg.encode().unwrap();
            let decoded = GossipMessage::decode(&encoded).unwrap();

            match decoded {
                GossipMessage::IdentityOffer { identity } => {
                    assert_eq!(identity.name, Some("offer-node".into()));
                    assert!(
                        identity.verify_self().unwrap(),
                        "decoded identity must verify"
                    );
                }
                _ => panic!("wrong variant"),
            }
        }

        #[test]
        fn test_identity_ack_roundtrip() {
            let kp = PqSigningKeypair::generate();
            let identity = PeerIdentity::new(&kp, Some("ack-node".into())).unwrap();

            let msg = GossipMessage::IdentityAck { identity };
            let encoded = msg.encode().unwrap();
            let decoded = GossipMessage::decode(&encoded).unwrap();

            match decoded {
                GossipMessage::IdentityAck { identity } => {
                    assert_eq!(identity.name, Some("ack-node".into()));
                    assert!(
                        identity.verify_self().unwrap(),
                        "decoded identity must verify"
                    );
                }
                _ => panic!("wrong variant"),
            }
        }
    }
}
