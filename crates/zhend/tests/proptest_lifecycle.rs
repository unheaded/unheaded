//! Property-based tests for the full fragment lifecycle.
//!
//! Verifies that random fragment sequences survive: ingest -> sediment L1->L2
//! -> sediment L2->L3 -> retrieve from any tier. Uses proptest for exhaustive
//! random exploration.
//!
//! GPL-3.0-only

use proptest::prelude::*;
use tempfile::TempDir;
use zhend::pu::fragment::{Fragment, FragmentId, Tier};
use zhend::pu::store::TieredStore;

/// Generate a random fragment with arbitrary payload and optional embedding.
fn arb_fragment() -> impl Strategy<Value = Fragment> {
    (
        prop::collection::vec(any::<u8>(), 1..512), // payload (non-empty)
        prop::collection::vec(any::<u8>(), 0..64),  // embedding
    )
        .prop_map(|(payload, embedding)| Fragment::new(payload, embedding, None))
}

proptest! {
    #![proptest_config(ProptestConfig::with_cases(64))]

    /// Ingest N random fragments -> sediment all to L2 -> verify all retrievable.
    #[test]
    fn ingest_sediment_l2_retrieve(
        fragments in prop::collection::vec(arb_fragment(), 1..32)
    ) {
        let tmp = TempDir::new().unwrap();
        let store = TieredStore::open(tmp.path()).unwrap();

        let mut ids: Vec<FragmentId> = Vec::new();
        let mut expected_payloads: Vec<Vec<u8>> = Vec::new();

        for frag in fragments {
            let id = frag.id.clone();
            let payload = frag.payload.clone();
            if store.ingest(frag).unwrap() {
                ids.push(id);
                expected_payloads.push(payload);
            }
        }

        let novel_count = ids.len();

        // Sediment L1 -> L2.
        store.sediment(0).unwrap();
        prop_assert_eq!(store.l1_count().unwrap(), 0);

        // Retrieve all from L2.
        for (id, expected_payload) in ids.iter().zip(expected_payloads.iter()) {
            let frag = store.get(id).unwrap();
            prop_assert!(frag.is_some(), "fragment {} missing after L2 sedimentation", id);
            let frag = frag.unwrap();
            prop_assert_eq!(&frag.payload, expected_payload);
            prop_assert!(frag.verify());
        }

        // Total count should be stable.
        prop_assert_eq!(store.count().unwrap(), novel_count);
    }

    /// Ingest N -> sediment to L2 -> sediment to L3 -> pilgrimage retrieves all.
    #[test]
    fn full_sedimentation_to_l3(
        fragments in prop::collection::vec(arb_fragment(), 1..32)
    ) {
        let tmp = TempDir::new().unwrap();
        let store = TieredStore::open(tmp.path()).unwrap();

        let mut ids: Vec<FragmentId> = Vec::new();
        let mut expected_payloads: Vec<Vec<u8>> = Vec::new();

        for frag in fragments {
            let id = frag.id.clone();
            let payload = frag.payload.clone();
            if store.ingest(frag).unwrap() {
                ids.push(id);
                expected_payloads.push(payload);
            }
        }

        // L1 -> L2 -> L3.
        store.sediment(0).unwrap();
        store.deep_sediment(0).unwrap();

        prop_assert_eq!(store.l1_count().unwrap(), 0);
        prop_assert_eq!(store.l3_count().unwrap(), ids.len());

        // Pilgrimage retrieve all.
        for (id, expected_payload) in ids.iter().zip(expected_payloads.iter()) {
            let frag = store.get(id).unwrap();
            prop_assert!(frag.is_some(), "fragment {} missing after L3 deep sedimentation", id);
            let frag = frag.unwrap();
            prop_assert_eq!(&frag.payload, expected_payload);
            prop_assert!(frag.verify());
            prop_assert_eq!(frag.tier, Tier::Hot); // promoted back
        }
    }

    /// Ingest, sediment, retrieve interleaved randomly.
    #[test]
    fn interleaved_operations(
        fragments in prop::collection::vec(arb_fragment(), 2..20),
        sediment_after in 1usize..10,
    ) {
        let tmp = TempDir::new().unwrap();
        let store = TieredStore::open(tmp.path()).unwrap();

        let mut all_ids: Vec<FragmentId> = Vec::new();

        for (i, frag) in fragments.into_iter().enumerate() {
            let id = frag.id.clone();
            if store.ingest(frag).unwrap() {
                all_ids.push(id);
            }

            // Periodically sediment.
            if (i + 1) % sediment_after == 0 {
                store.sediment(0).unwrap();
            }
        }

        // Deep sediment whatever is in L2.
        store.deep_sediment(0).unwrap();

        // All fragments must still be retrievable from some tier.
        for id in &all_ids {
            let frag = store.get(id).unwrap();
            prop_assert!(frag.is_some(), "fragment {} lost during interleaved operations", id);
            prop_assert!(frag.unwrap().verify());
        }
    }

    /// Snapshot + restore preserves all L1 fragments across restarts.
    #[test]
    fn snapshot_restore_preserves_fragments(
        fragments in prop::collection::vec(arb_fragment(), 1..32)
    ) {
        let tmp = TempDir::new().unwrap();

        let mut ids: Vec<FragmentId> = Vec::new();
        let mut expected_payloads: Vec<Vec<u8>> = Vec::new();

        // Phase 1: ingest + snapshot.
        {
            let store = TieredStore::open(tmp.path()).unwrap();
            for frag in fragments {
                let id = frag.id.clone();
                let payload = frag.payload.clone();
                if store.ingest(frag).unwrap() {
                    ids.push(id);
                    expected_payloads.push(payload);
                }
            }
            let snapped = store.snapshot().unwrap();
            prop_assert_eq!(snapped, ids.len());
        }

        // Phase 2: reopen -> auto-restore.
        {
            let store = TieredStore::open(tmp.path()).unwrap();
            prop_assert_eq!(store.l1_count().unwrap(), ids.len());

            for (id, expected_payload) in ids.iter().zip(expected_payloads.iter()) {
                let frag = store.get(id).unwrap();
                prop_assert!(frag.is_some(), "fragment {} lost after snapshot/restore", id);
                prop_assert_eq!(&frag.unwrap().payload, expected_payload);
            }
        }
    }
}
