//! # Tiered Store — Geological Memory
//!
//! L1 (Hot)  — In-memory HashMap, mmap-backed. Sub-millisecond.
//! L2 (Warm) — sled embedded KV. Single-digit milliseconds.
//! L3 (Cold) — Append-only archive files. Seconds to minutes (Jing).
//!
//! Fragments sediment downward over time (Hot → Warm → Cold).
//! Any access resets to Hot. Fragments NEVER die.

use std::collections::HashMap;
use std::sync::RwLock;

use crate::{ZhenError, ZhenResult};
use super::codec;
use super::fragment::{Fragment, FragmentId, Tier};

/// The three-tier storage engine for Zhen fragments.
pub struct TieredStore {
    /// L1 — hot fragments in memory. Fast. Volatile between restarts
    /// unless backed by mmap (future optimization).
    l1: RwLock<HashMap<FragmentId, Fragment>>,

    /// L2 — warm fragments in sled. Persistent. Crash-safe.
    l2: sled::Db,

    // L3 — cold archive. Delegated to jing module.
    // TODO: integrate jing::Archive for L3
}

impl TieredStore {
    /// Open or create a tiered store at the given data directory.
    pub fn open(data_dir: &std::path::Path) -> ZhenResult<Self> {
        let l2_path = data_dir.join("l2_warm");
        let l2 = sled::open(&l2_path)?;

        Ok(Self {
            l1: RwLock::new(HashMap::new()),
            l2,
        })
    }

    /// Ingest a new fragment. Verifies integrity, deduplicates, stores in L1.
    ///
    /// Returns `true` if this was a novel fragment, `false` if duplicate.
    pub fn ingest(&self, fragment: Fragment) -> ZhenResult<bool> {
        // ALL INPUTS HOSTILE — verify before storing.
        if !fragment.verify() {
            return Err(ZhenError::IntegrityFailure {
                hash: fragment.id.to_hex(),
            });
        }

        // Dedup: check all tiers.
        if self.contains(&fragment.id)? {
            return Ok(false);
        }

        // Store in L1 (hot).
        let mut l1 = self.l1.write().map_err(|e| {
            ZhenError::Storage(sled::Error::Unsupported(e.to_string().into()))
        })?;
        l1.insert(fragment.id.clone(), fragment);

        Ok(true)
    }

    /// Retrieve a fragment by ID from any tier. Touches on access (resets to Hot).
    pub fn get(&self, id: &FragmentId) -> ZhenResult<Option<Fragment>> {
        // Check L1 first.
        {
            let mut l1 = self.l1.write().map_err(|e| {
                ZhenError::Storage(sled::Error::Unsupported(e.to_string().into()))
            })?;
            if let Some(frag) = l1.get_mut(id) {
                frag.touch();
                return Ok(Some(frag.clone()));
            }
        }

        // Check L2.
        if let Some(bytes) = self.l2.get(&id.0)? {
            let mut frag = codec::decode(&bytes)?;
            frag.touch();

            // Promote back to L1.
            let mut l1 = self.l1.write().map_err(|e| {
                ZhenError::Storage(sled::Error::Unsupported(e.to_string().into()))
            })?;
            l1.insert(id.clone(), frag.clone());

            // Remove from L2 (it's hot now).
            self.l2.remove(&id.0)?;

            return Ok(Some(frag));
        }

        // TODO: Check L3 (jing archive).

        Ok(None)
    }

    /// Check if a fragment exists in any tier (without touching).
    pub fn contains(&self, id: &FragmentId) -> ZhenResult<bool> {
        // L1
        {
            let l1 = self.l1.read().map_err(|e| {
                ZhenError::Storage(sled::Error::Unsupported(e.to_string().into()))
            })?;
            if l1.contains_key(id) {
                return Ok(true);
            }
        }

        // L2
        if self.l2.contains_key(&id.0)? {
            return Ok(true);
        }

        // TODO: L3

        Ok(false)
    }

    /// Migrate cold fragments from L1 → L2 based on last_accessed threshold.
    pub fn sediment(&self, l1_to_l2_secs: u64) -> ZhenResult<u64> {
        let now = chrono::Utc::now();
        let threshold = chrono::Duration::seconds(l1_to_l2_secs as i64);
        let mut migrated = 0u64;

        let mut to_migrate = Vec::new();

        // Identify candidates.
        {
            let l1 = self.l1.read().map_err(|e| {
                ZhenError::Storage(sled::Error::Unsupported(e.to_string().into()))
            })?;
            for (id, frag) in l1.iter() {
                if let Some(age) = now.signed_duration_since(frag.last_accessed).to_std().ok() {
                    if age > threshold.to_std().unwrap_or_default() {
                        to_migrate.push(id.clone());
                    }
                }
            }
        }

        // Migrate.
        let mut l1 = self.l1.write().map_err(|e| {
            ZhenError::Storage(sled::Error::Unsupported(e.to_string().into()))
        })?;
        for id in to_migrate {
            if let Some(mut frag) = l1.remove(&id) {
                frag.tier = Tier::Warm;
                let encoded = codec::encode(&frag)?;
                self.l2.insert(&id.0, encoded)?;
                migrated += 1;
            }
        }

        if migrated > 0 {
            self.l2.flush()?;
            tracing::info!(migrated, "sedimentation cycle complete: L1 → L2");
        }

        Ok(migrated)
    }

    /// Total fragment count across all tiers.
    pub fn count(&self) -> ZhenResult<usize> {
        let l1_count = self.l1.read().map_err(|e| {
            ZhenError::Storage(sled::Error::Unsupported(e.to_string().into()))
        })?.len();
        let l2_count = self.l2.len();

        Ok(l1_count + l2_count)
    }

    /// L1 fragment count (hot).
    pub fn l1_count(&self) -> ZhenResult<usize> {
        Ok(self.l1.read().map_err(|e| {
            ZhenError::Storage(sled::Error::Unsupported(e.to_string().into()))
        })?.len())
    }

    /// All fragment IDs currently in L1 (for gossip selection).
    pub fn l1_ids(&self) -> ZhenResult<Vec<FragmentId>> {
        let l1 = self.l1.read().map_err(|e| {
            ZhenError::Storage(sled::Error::Unsupported(e.to_string().into()))
        })?;
        Ok(l1.keys().cloned().collect())
    }

    /// All fragments currently in L1 (for embedding/similarity search).
    pub fn l1_fragments(&self) -> ZhenResult<Vec<Fragment>> {
        let l1 = self.l1.read().map_err(|e| {
            ZhenError::Storage(sled::Error::Unsupported(e.to_string().into()))
        })?;
        Ok(l1.values().cloned().collect())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    fn test_store() -> (TieredStore, TempDir) {
        let tmp = TempDir::new().unwrap();
        let store = TieredStore::open(tmp.path()).unwrap();
        (store, tmp)
    }

    #[test]
    fn test_ingest_and_retrieve() {
        let (store, _tmp) = test_store();
        let frag = Fragment::new(b"hello zhen".to_vec(), vec![], None);
        let id = frag.id.clone();

        assert!(store.ingest(frag).unwrap(), "first ingest should be novel");
        assert!(!store.ingest(Fragment::new(b"hello zhen".to_vec(), vec![], None)).unwrap(),
                "duplicate should return false");

        let retrieved = store.get(&id).unwrap().expect("fragment should exist");
        assert_eq!(retrieved.payload, b"hello zhen");
        assert_eq!(retrieved.access_count, 1); // touched on get
    }

    #[test]
    fn test_sedimentation() {
        let (store, _tmp) = test_store();

        // Ingest a fragment.
        let frag = Fragment::new(b"old knowledge".to_vec(), vec![], None);
        let id = frag.id.clone();
        store.ingest(frag).unwrap();

        // With threshold 0 seconds, everything migrates immediately.
        let migrated = store.sediment(0).unwrap();
        assert_eq!(migrated, 1);

        // L1 should be empty.
        assert_eq!(store.l1_count().unwrap(), 0);

        // But fragment is still retrievable (from L2, promoted back to L1).
        let retrieved = store.get(&id).unwrap().expect("fragment survives sedimentation");
        assert_eq!(retrieved.payload, b"old knowledge");
        assert_eq!(retrieved.tier, Tier::Hot); // promoted back
    }

    #[test]
    fn test_tampered_fragment_rejected() {
        let (store, _tmp) = test_store();
        let mut frag = Fragment::new(b"legit".to_vec(), vec![], None);
        frag.payload = b"tampered".to_vec(); // id won't match

        let result = store.ingest(frag);
        assert!(result.is_err());
    }

    #[test]
    fn test_count() {
        let (store, _tmp) = test_store();
        assert_eq!(store.count().unwrap(), 0);

        store.ingest(Fragment::new(b"one".to_vec(), vec![], None)).unwrap();
        store.ingest(Fragment::new(b"two".to_vec(), vec![], None)).unwrap();
        assert_eq!(store.count().unwrap(), 2);
    }
}
