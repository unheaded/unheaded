//! Phase 2 Exit Gate: fragments survive restart across all tiers.
//!
//! This test simulates the full lifecycle:
//! 1. Ingest fragments into L1
//! 2. Sediment a portion to L2
//! 3. Deep-sediment a portion to L3
//! 4. Snapshot remaining L1 to disk
//! 5. "Kill" (drop the store)
//! 6. Restart (reopen from same data_dir)
//! 7. Verify ALL fragments are retrievable from some tier
//!
//! Uses 10K fragments (L3 pilgrimage is O(n) per lookup — 100K would be
//! quadratic). The property tested is tier-agnostic survival, not scale.
//! The sustained ingest benchmark separately validates >10K frags/sec.
//!
//! GPL-3.0-only

use tempfile::TempDir;
use zhend::pu::fragment::{Fragment, FragmentId};
use zhend::pu::store::TieredStore;

#[test]
fn exit_gate_fragments_survive_restart() {
    let tmp = TempDir::new().unwrap();
    let total = 10_000;

    // Pre-compute all fragment IDs and payloads.
    let payloads: Vec<Vec<u8>> = (0..total)
        .map(|i| format!("fragment_{:08}", i).into_bytes())
        .collect();
    let ids: Vec<FragmentId> = payloads
        .iter()
        .map(|p| FragmentId::from_payload(p))
        .collect();

    // Phase 1: Ingest, distribute across tiers, snapshot, "kill".
    {
        let store = TieredStore::open(tmp.path()).unwrap();

        // Ingest all.
        for payload in &payloads {
            let frag = Fragment::new(payload.clone(), vec![], None);
            assert!(store.ingest(frag).unwrap());
        }
        assert_eq!(store.l1_count().unwrap(), total);

        // Move all to L2.
        store.sediment(0).unwrap();
        assert_eq!(store.l1_count().unwrap(), 0);

        // Move all to L3.
        store.deep_sediment(0).unwrap();
        assert_eq!(store.l3_count().unwrap(), total);

        // Promote first 4000 back to L1 (via pilgrimage).
        for id in ids.iter().take(4_000) {
            store.get(id).unwrap().expect("should be in L3");
        }
        assert_eq!(store.l1_count().unwrap(), 4_000);

        // Sediment 2000 of L1 to L2.
        store.sediment(0).unwrap();
        assert_eq!(store.l1_count().unwrap(), 0);

        // Re-access first 2000 to put them back in L1.
        for id in ids.iter().take(2_000) {
            store.get(id).unwrap().expect("should be in L2");
        }
        assert_eq!(store.l1_count().unwrap(), 2_000);

        // Final distribution:
        // L1: 2K (will be snapshotted)
        // L2: 2K (in sled, persistent)
        // L3: 6K (in archive, persistent — note: promoted fragments also
        //         remain in archive since it's append-only, but dedup
        //         means they won't be double-counted)
        let count = store.count().unwrap();
        assert_eq!(count, total);

        // Snapshot L1.
        let snapped = store.snapshot().unwrap();
        assert_eq!(snapped, 2_000);
    }

    // Phase 2: "Restart" — reopen from same data_dir.
    {
        let store = TieredStore::open(tmp.path()).unwrap();

        // L1 restored from snapshot.
        assert_eq!(store.l1_count().unwrap(), 2_000);

        // Verify ALL fragments are retrievable.
        let mut found = 0;
        for (i, id) in ids.iter().enumerate() {
            let frag = store.get(id).unwrap();
            assert!(
                frag.is_some(),
                "fragment {} (index {}) lost after restart",
                id,
                i
            );
            let frag = frag.unwrap();
            assert!(
                frag.verify(),
                "fragment {} failed integrity after restart",
                id
            );
            assert_eq!(&frag.payload, &payloads[i]);
            found += 1;
        }

        assert_eq!(found, total, "not all fragments survived restart");
        eprintln!(
            "EXIT GATE PASSED: {}/{} fragments survived kill+restart across L1/L2/L3",
            found, total
        );
    }
}

/// Bonus: verify fragments ingested directly into each tier survive.
#[test]
fn exit_gate_tier_isolation() {
    let tmp = TempDir::new().unwrap();

    let p_l1 = b"stays_in_l1".to_vec();
    let p_l2 = b"sediments_to_l2".to_vec();
    let p_l3 = b"archives_to_l3".to_vec();

    let id_l1 = FragmentId::from_payload(&p_l1);
    let id_l2 = FragmentId::from_payload(&p_l2);
    let id_l3 = FragmentId::from_payload(&p_l3);

    {
        let store = TieredStore::open(tmp.path()).unwrap();

        // Ingest all 3.
        store
            .ingest(Fragment::new(p_l1.clone(), vec![], None))
            .unwrap();
        store
            .ingest(Fragment::new(p_l2.clone(), vec![], None))
            .unwrap();
        store
            .ingest(Fragment::new(p_l3.clone(), vec![], None))
            .unwrap();

        // Sediment all to L2.
        store.sediment(0).unwrap();

        // Deep-sediment all to L3.
        store.deep_sediment(0).unwrap();

        // Promote l1 and l2 fragments back.
        store.get(&id_l1).unwrap();
        store.get(&id_l2).unwrap();

        // Sediment l2 back to L2.
        store.sediment(0).unwrap();

        // Re-access l1 to keep it hot.
        store.get(&id_l1).unwrap();

        // Now: l1 in L1, l2 in L2, l3 in L3.
        assert_eq!(store.l1_count().unwrap(), 1);

        store.snapshot().unwrap();
    }

    // Restart.
    {
        let store = TieredStore::open(tmp.path()).unwrap();

        let f1 = store.get(&id_l1).unwrap().expect("L1 fragment lost");
        assert_eq!(f1.payload, p_l1);

        let f2 = store.get(&id_l2).unwrap().expect("L2 fragment lost");
        assert_eq!(f2.payload, p_l2);

        let f3 = store.get(&id_l3).unwrap().expect("L3 fragment lost");
        assert_eq!(f3.payload, p_l3);
    }
}
