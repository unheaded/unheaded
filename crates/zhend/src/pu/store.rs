//! # Tiered Store — Geological Memory
//!
//! L1 (Hot)  — In-memory HashMap, mmap-backed. Sub-millisecond.
//! L2 (Warm) — sled embedded KV. Single-digit milliseconds.
//! L3 (Cold) — Append-only archive files. Seconds to minutes (Jing).
//!
//! Fragments sediment downward over time (Hot → Warm → Cold).
//! Any access resets to Hot. Fragments NEVER die.

use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::RwLock;

use crate::jing::Archive;
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

    /// L3 — cold archive. Append-only Jing strata.
    l3: Archive,

    /// Path for L1 snapshot persistence (graceful shutdown → restart).
    snapshot_path: PathBuf,
}

impl TieredStore {
    /// Open or create a tiered store at the given data directory.
    ///
    /// On open, restores any L1 snapshot from a prior graceful shutdown.
    pub fn open(data_dir: &std::path::Path) -> ZhenResult<Self> {
        let l2_path = data_dir.join("l2_warm");
        let l2 = sled::open(&l2_path)?;

        let l3_path = data_dir.join("l3_cold.archive");
        let l3 = Archive::open(&l3_path)?;

        let snapshot_path = data_dir.join("l1_snapshot.bin");

        let mut store = Self {
            l1: RwLock::new(HashMap::new()),
            l2,
            l3,
            snapshot_path,
        };

        // Restore L1 from snapshot if it exists (graceful restart path).
        match store.restore() {
            Ok(0) => {} // no snapshot or empty
            Ok(n) => {
                tracing::info!(restored = n, "L1 snapshot restored from disk");
            }
            Err(e) => {
                tracing::warn!(error = %e, "L1 snapshot restore failed — starting with empty L1");
            }
        }

        Ok(store)
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
            ZhenError::Storage(sled::Error::Unsupported(e.to_string()))
        })?;
        l1.insert(fragment.id.clone(), fragment);

        Ok(true)
    }

    /// Retrieve a fragment by ID from any tier. Touches on access (resets to Hot).
    pub fn get(&self, id: &FragmentId) -> ZhenResult<Option<Fragment>> {
        // Check L1 first.
        {
            let mut l1 = self.l1.write().map_err(|e| {
                ZhenError::Storage(sled::Error::Unsupported(e.to_string()))
            })?;
            if let Some(frag) = l1.get_mut(id) {
                frag.touch();
                return Ok(Some(frag.clone()));
            }
        }

        // Check L2.
        if let Some(bytes) = self.l2.get(id.0)? {
            let mut frag = codec::decode(&bytes)?;
            frag.touch();

            // Promote back to L1.
            let mut l1 = self.l1.write().map_err(|e| {
                ZhenError::Storage(sled::Error::Unsupported(e.to_string()))
            })?;
            l1.insert(id.clone(), frag.clone());

            // Remove from L2 (it's hot now).
            self.l2.remove(id.0)?;

            return Ok(Some(frag));
        }

        // Check L3 (Jing cold archive — pilgrimage).
        if let Some(mut frag) = self.l3.pilgrimage(id)? {
            frag.touch();

            // Promote to L1.
            let mut l1 = self.l1.write().map_err(|e| {
                ZhenError::Storage(sled::Error::Unsupported(e.to_string()))
            })?;
            l1.insert(id.clone(), frag.clone());

            tracing::debug!(fragment_id = %id, "fragment promoted from L3 → L1");
            return Ok(Some(frag));
        }

        Ok(None)
    }

    /// Check if a fragment exists in any tier (without touching).
    pub fn contains(&self, id: &FragmentId) -> ZhenResult<bool> {
        // L1
        {
            let l1 = self.l1.read().map_err(|e| {
                ZhenError::Storage(sled::Error::Unsupported(e.to_string()))
            })?;
            if l1.contains_key(id) {
                return Ok(true);
            }
        }

        // L2
        if self.l2.contains_key(id.0)? {
            return Ok(true);
        }

        // L3 — pilgrimage scan (slow, but correct).
        if self.l3.pilgrimage(id)?.is_some() {
            return Ok(true);
        }

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
                ZhenError::Storage(sled::Error::Unsupported(e.to_string()))
            })?;
            for (id, frag) in l1.iter() {
                if let Ok(age) = now.signed_duration_since(frag.last_accessed).to_std() {
                    if age > threshold.to_std().unwrap_or_default() {
                        to_migrate.push(id.clone());
                    }
                }
            }
        }

        // Migrate.
        let mut l1 = self.l1.write().map_err(|e| {
            ZhenError::Storage(sled::Error::Unsupported(e.to_string()))
        })?;
        for id in to_migrate {
            if let Some(mut frag) = l1.remove(&id) {
                frag.tier = Tier::Warm;
                let encoded = codec::encode(&frag)?;
                self.l2.insert(id.0, encoded)?;
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
            ZhenError::Storage(sled::Error::Unsupported(e.to_string()))
        })?.len();
        let l2_count = self.l2.len();
        let l3_count = self.l3.scan_ids()?.len();

        Ok(l1_count + l2_count + l3_count)
    }

    /// L1 fragment count (hot).
    pub fn l1_count(&self) -> ZhenResult<usize> {
        Ok(self.l1.read().map_err(|e| {
            ZhenError::Storage(sled::Error::Unsupported(e.to_string()))
        })?.len())
    }

    /// All fragment IDs currently in L1 (for gossip selection).
    pub fn l1_ids(&self) -> ZhenResult<Vec<FragmentId>> {
        let l1 = self.l1.read().map_err(|e| {
            ZhenError::Storage(sled::Error::Unsupported(e.to_string()))
        })?;
        Ok(l1.keys().cloned().collect())
    }

    /// All fragments currently in L1 (for embedding/similarity search).
    pub fn l1_fragments(&self) -> ZhenResult<Vec<Fragment>> {
        let l1 = self.l1.read().map_err(|e| {
            ZhenError::Storage(sled::Error::Unsupported(e.to_string()))
        })?;
        Ok(l1.values().cloned().collect())
    }

    /// Migrate cold fragments from L2 → L3 based on last_accessed threshold.
    ///
    /// L3 is append-only (Jing archive). Once archived, fragments are removed
    /// from L2 to avoid duplication across tiers.
    pub fn deep_sediment(&self, l2_to_l3_secs: u64) -> ZhenResult<u64> {
        let now = chrono::Utc::now();
        let threshold = chrono::Duration::seconds(l2_to_l3_secs as i64);
        let mut migrated = 0u64;

        // Scan L2 for candidates.
        let mut to_archive = Vec::new();
        for entry in self.l2.iter() {
            let (key, value) = entry?;
            match codec::decode(&value) {
                Ok(frag) => {
                    if let Ok(age) = now.signed_duration_since(frag.last_accessed).to_std() {
                        if age > threshold.to_std().unwrap_or_default() {
                            to_archive.push((key, frag));
                        }
                    }
                }
                Err(e) => {
                    tracing::warn!(error = %e, "skipping corrupt L2 entry during deep sedimentation");
                }
            }
        }

        // Archive to L3 and remove from L2.
        for (key, mut frag) in to_archive {
            self.l3.append(&mut frag)?;
            self.l2.remove(key)?;
            migrated += 1;
        }

        if migrated > 0 {
            self.l2.flush()?;
            tracing::info!(migrated, "deep sedimentation cycle complete: L2 → L3");
        }

        Ok(migrated)
    }

    /// Snapshot L1 to disk. Call on graceful shutdown to persist hot fragments.
    ///
    /// Returns the number of fragments written.
    pub fn snapshot(&self) -> ZhenResult<usize> {
        let l1 = self.l1.read().map_err(|e| {
            ZhenError::Storage(sled::Error::Unsupported(e.to_string()))
        })?;

        let fragments: Vec<&Fragment> = l1.values().collect();
        let count = fragments.len();

        let encoded = bincode::serialize(&fragments)
            .map_err(|e| ZhenError::Serialization(e.to_string()))?;

        // Write atomically: write to tmp, then rename.
        let tmp_path = self.snapshot_path.with_extension("bin.tmp");
        std::fs::write(&tmp_path, &encoded)?;
        std::fs::rename(&tmp_path, &self.snapshot_path)?;

        tracing::info!(fragments = count, "L1 snapshot written to disk");
        Ok(count)
    }

    /// Restore L1 from a prior snapshot. Call on startup.
    ///
    /// Returns the number of fragments restored. If no snapshot exists,
    /// returns 0 (not an error — clean start).
    pub fn restore(&mut self) -> ZhenResult<usize> {
        let bytes = match std::fs::read(&self.snapshot_path) {
            Ok(b) => b,
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => return Ok(0),
            Err(e) => return Err(e.into()),
        };

        let fragments: Vec<Fragment> = bincode::deserialize(&bytes)
            .map_err(|e| ZhenError::Serialization(e.to_string()))?;

        let mut l1 = self.l1.write().map_err(|e| {
            ZhenError::Storage(sled::Error::Unsupported(e.to_string()))
        })?;

        let mut restored = 0;
        for frag in fragments {
            // ALL INPUTS HOSTILE — verify even our own snapshot.
            if frag.verify() {
                l1.insert(frag.id.clone(), frag);
                restored += 1;
            } else {
                tracing::warn!("skipping corrupt fragment in L1 snapshot");
            }
        }

        // Remove snapshot after successful restore (avoid stale data on next restart).
        let _ = std::fs::remove_file(&self.snapshot_path);

        Ok(restored)
    }

    /// L3 fragment count (cold/Jing).
    pub fn l3_count(&self) -> ZhenResult<usize> {
        Ok(self.l3.scan_ids()?.len())
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

    #[test]
    fn test_deep_sedimentation_l2_to_l3() {
        let (store, _tmp) = test_store();

        let frag = Fragment::new(b"deep knowledge".to_vec(), vec![], None);
        let id = frag.id.clone();
        store.ingest(frag).unwrap();

        // L1 → L2 (threshold 0 = immediate).
        let migrated = store.sediment(0).unwrap();
        assert_eq!(migrated, 1);
        assert_eq!(store.l1_count().unwrap(), 0);

        // L2 → L3 (threshold 0 = immediate).
        let deep_migrated = store.deep_sediment(0).unwrap();
        assert_eq!(deep_migrated, 1);

        // L2 should be empty now.
        assert_eq!(store.l2.len(), 0);

        // Fragment still retrievable (from L3, promoted to L1).
        let retrieved = store.get(&id).unwrap().expect("fragment survives deep sedimentation");
        assert_eq!(retrieved.payload, b"deep knowledge");
        assert_eq!(retrieved.tier, Tier::Hot); // promoted back
    }

    #[test]
    fn test_l3_count() {
        let (store, _tmp) = test_store();

        store.ingest(Fragment::new(b"a".to_vec(), vec![], None)).unwrap();
        store.ingest(Fragment::new(b"b".to_vec(), vec![], None)).unwrap();

        store.sediment(0).unwrap();
        store.deep_sediment(0).unwrap();

        assert_eq!(store.l3_count().unwrap(), 2);
        assert_eq!(store.count().unwrap(), 2); // total across all tiers
    }

    #[test]
    fn test_contains_checks_all_tiers() {
        let (store, _tmp) = test_store();

        let frag = Fragment::new(b"findable".to_vec(), vec![], None);
        let id = frag.id.clone();
        store.ingest(frag).unwrap();

        // In L1.
        assert!(store.contains(&id).unwrap());

        // Move to L2.
        store.sediment(0).unwrap();
        assert!(store.contains(&id).unwrap());

        // Move to L3.
        store.deep_sediment(0).unwrap();
        assert!(store.contains(&id).unwrap());
    }

    #[test]
    fn test_snapshot_and_restore() {
        let tmp = TempDir::new().unwrap();

        // Phase 1: ingest fragments and snapshot.
        {
            let store = TieredStore::open(tmp.path()).unwrap();
            store.ingest(Fragment::new(b"survive restart".to_vec(), vec![], None)).unwrap();
            store.ingest(Fragment::new(b"persist forever".to_vec(), vec![], None)).unwrap();
            let snapped = store.snapshot().unwrap();
            assert_eq!(snapped, 2);
        }

        // Phase 2: open fresh store — should auto-restore from snapshot.
        {
            let store = TieredStore::open(tmp.path()).unwrap();
            assert_eq!(store.l1_count().unwrap(), 2);

            let id = FragmentId::from_payload(b"survive restart");
            let frag = store.get(&id).unwrap().expect("fragment survives restart");
            assert_eq!(frag.payload, b"survive restart");
        }
    }

    #[test]
    fn test_snapshot_empty_l1() {
        let (store, _tmp) = test_store();
        let snapped = store.snapshot().unwrap();
        assert_eq!(snapped, 0);
    }

    #[test]
    fn test_full_lifecycle_l1_l2_l3_retrieve() {
        let (store, _tmp) = test_store();

        // Ingest 5 fragments.
        let mut ids = Vec::new();
        for i in 0..5 {
            let payload = format!("fragment_{}", i).into_bytes();
            let frag = Fragment::new(payload, vec![], None);
            ids.push(frag.id.clone());
            store.ingest(frag).unwrap();
        }

        assert_eq!(store.l1_count().unwrap(), 5);
        assert_eq!(store.count().unwrap(), 5);

        // Sediment L1 → L2.
        store.sediment(0).unwrap();
        assert_eq!(store.l1_count().unwrap(), 0);

        // Sediment L2 → L3.
        store.deep_sediment(0).unwrap();
        assert_eq!(store.l3_count().unwrap(), 5);

        // Retrieve ALL from L3 — each pilgrimages then promotes to L1.
        for id in &ids {
            let frag = store.get(id).unwrap().expect("fragment must survive full lifecycle");
            assert!(frag.verify());
            assert_eq!(frag.tier, Tier::Hot);
        }

        assert_eq!(store.l1_count().unwrap(), 5);
    }

    #[test]
    fn test_dedup_across_tiers() {
        let (store, _tmp) = test_store();

        let frag1 = Fragment::new(b"unique".to_vec(), vec![], None);
        assert!(store.ingest(frag1).unwrap());

        // Same payload in L1 → dedup.
        let frag2 = Fragment::new(b"unique".to_vec(), vec![], None);
        assert!(!store.ingest(frag2).unwrap());

        // Move to L2, try ingesting again.
        store.sediment(0).unwrap();
        let frag3 = Fragment::new(b"unique".to_vec(), vec![], None);
        assert!(!store.ingest(frag3).unwrap());

        // Move to L3, try ingesting again.
        store.deep_sediment(0).unwrap();
        let frag4 = Fragment::new(b"unique".to_vec(), vec![], None);
        assert!(!store.ingest(frag4).unwrap());
    }
}
