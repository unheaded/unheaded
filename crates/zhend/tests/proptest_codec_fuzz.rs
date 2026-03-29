//! Fuzz the codec: malformed bytes must never panic, only return errors.
//!
//! Uses proptest to feed random byte vectors to codec::decode().
//! ALL INPUTS HOSTILE — the codec must handle anything gracefully.
//!
//! GPL-3.0-only

use proptest::prelude::*;
use zhend::pu::codec;
use zhend::pu::fragment::Fragment;

proptest! {
    #![proptest_config(ProptestConfig::with_cases(1024))]

    /// Random bytes fed to decode() must never panic.
    #[test]
    fn decode_never_panics(bytes in prop::collection::vec(any::<u8>(), 0..1024)) {
        // This is the fuzz test. We don't care about the result,
        // only that it doesn't panic.
        let _ = codec::decode(&bytes);
    }

    /// Random bytes with valid-looking length prefix must never panic.
    #[test]
    fn decode_with_length_prefix_never_panics(
        prefix in prop::collection::vec(any::<u8>(), 0..8),
        body in prop::collection::vec(any::<u8>(), 0..512),
    ) {
        let mut input = prefix;
        input.extend_from_slice(&body);
        let _ = codec::decode(&input);
    }

    /// Encode then corrupt: corrupted encoded fragments must either fail
    /// decode or produce a fragment with different identity/payload.
    ///
    /// Note: single-byte corruption of non-payload fields (timestamps,
    /// counters) may still pass the BLAKE3 integrity check since the id
    /// is computed from payload only. This is BY DESIGN — the integrity
    /// check protects payload authenticity, not metadata.
    #[test]
    fn corrupted_encoded_fragment_handled(
        payload in prop::collection::vec(any::<u8>(), 1..256),
        corruption_index in any::<usize>(),
        corruption_byte in any::<u8>(),
    ) {
        let frag = Fragment::new(payload, vec![], None);
        let mut encoded = codec::encode(&frag).unwrap();

        if !encoded.is_empty() {
            let idx = corruption_index % encoded.len();
            let original = encoded[idx];
            encoded[idx] = corruption_byte;

            // The important property: decode NEVER panics.
            let result = codec::decode(&encoded);

            if corruption_byte != original {
                match result {
                    Err(_) => {} // expected: corruption detected
                    Ok(decoded) => {
                        // If decode succeeded, either:
                        // 1. The corruption hit a non-payload field and
                        //    the fragment still verifies (valid behavior).
                        // 2. The corruption produced a valid but different
                        //    fragment (extremely unlikely but possible).
                        // Both are safe — no panic, no UB.
                        prop_assert!(decoded.verify(),
                            "decoded corrupt fragment must still pass verify if decode succeeded");
                    }
                }
            }
        }
    }

    /// Empty and near-empty inputs must not panic.
    #[test]
    fn tiny_inputs_never_panic(size in 0usize..8) {
        let bytes: Vec<u8> = (0..size).map(|i| i as u8).collect();
        let _ = codec::decode(&bytes);
    }

    /// Encode/decode roundtrip preserves fragment identity for any payload.
    #[test]
    fn roundtrip_preserves_identity(
        payload in prop::collection::vec(any::<u8>(), 0..1024),
        embedding in prop::collection::vec(any::<u8>(), 0..128),
    ) {
        let frag = Fragment::new(payload.clone(), embedding.clone(), None);
        let encoded = codec::encode(&frag).unwrap();
        let decoded = codec::decode(&encoded).unwrap();

        prop_assert_eq!(&frag.id, &decoded.id);
        prop_assert_eq!(&frag.payload, &decoded.payload);
        prop_assert_eq!(&frag.embedding, &decoded.embedding);
        prop_assert!(decoded.verify());
    }
}
