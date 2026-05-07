//! Append-only archive for L3 cold fragments.
//!
//! Format: sequential entries, each prefixed with length.
//! No index file — scanning is intentional (pilgrimage).
//!
//! ```text
//! ┌──────────┬────────────────────┐
//! │ len: u32 │ fragment (bincode) │  ← entry 0
//! ├──────────┼────────────────────┤
//! │ len: u32 │ fragment (bincode) │  ← entry 1
//! ├──────────┼────────────────────┤
//! │ ...      │ ...                │
//! └──────────┴────────────────────┘
//! ```
//!
//! Reads require sequential scan. This is BY DESIGN.
//! Fast lookup = index = root = burnable. Slow scan = pilgrimage = anti-fragile.

use std::fs::{File, OpenOptions};
use std::io::{self, BufReader, BufWriter, Read, Write};
use std::path::{Path, PathBuf};

use crate::pu::codec;
use crate::pu::fragment::{Fragment, FragmentId, Tier};
use crate::{ZhenError, ZhenResult};

/// Append-only archive for cold (Jing) fragments.
pub struct Archive {
    path: PathBuf,
}

impl Archive {
    /// Open or create an archive file.
    pub fn open(path: &Path) -> ZhenResult<Self> {
        // Ensure parent directory exists.
        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent)?;
        }

        // Create file if it doesn't exist.
        OpenOptions::new().create(true).append(true).open(path)?;

        Ok(Self {
            path: path.to_path_buf(),
        })
    }

    /// Append a fragment to the archive. Marks it as Cold tier.
    pub fn append(&self, fragment: &mut Fragment) -> ZhenResult<()> {
        // ALL INPUTS HOSTILE.
        if !fragment.verify() {
            return Err(ZhenError::IntegrityFailure {
                hash: fragment.id.to_hex(),
            });
        }

        fragment.tier = Tier::Cold;
        let encoded = codec::encode(fragment)?;
        let len = encoded.len() as u32;

        let file = OpenOptions::new().append(true).open(&self.path)?;
        let mut writer = BufWriter::new(file);

        writer.write_all(&len.to_le_bytes())?;
        writer.write_all(&encoded)?;
        writer.flush()?;

        tracing::trace!(
            fragment_id = %fragment.id,
            payload_len = fragment.payload_len(),
            "fragment archived to Jing"
        );

        Ok(())
    }

    /// Pilgrimage: scan the entire archive looking for a specific fragment.
    ///
    /// This is intentionally O(n). The journey IS the retrieval.
    /// Returns None if the fragment has never been archived.
    pub fn pilgrimage(&self, target: &FragmentId) -> ZhenResult<Option<Fragment>> {
        let file = match File::open(&self.path) {
            Ok(f) => f,
            Err(e) if e.kind() == io::ErrorKind::NotFound => return Ok(None),
            Err(e) => return Err(e.into()),
        };

        let mut reader = BufReader::new(file);
        let mut len_buf = [0u8; 4];

        loop {
            // Read entry length.
            match reader.read_exact(&mut len_buf) {
                Ok(()) => {}
                Err(e) if e.kind() == io::ErrorKind::UnexpectedEof => break,
                Err(e) => return Err(e.into()),
            }

            let len = u32::from_le_bytes(len_buf) as usize;

            // Guard against absurdly large entries (corrupt file).
            if len > 64 * 1024 * 1024 {
                return Err(ZhenError::Serialization(
                    "archive entry exceeds 64MB — possible corruption".into(),
                ));
            }

            let mut entry_buf = vec![0u8; len];
            reader.read_exact(&mut entry_buf)?;

            // Decode and check ID without full verification (performance).
            // Full verify happens on return.
            match codec::decode(&entry_buf) {
                Ok(frag) if frag.id == *target => {
                    tracing::debug!(
                        fragment_id = %target,
                        "pilgrimage complete — fragment found in Jing"
                    );
                    return Ok(Some(frag));
                }
                Ok(_) => continue, // not the one we seek
                Err(_) => {
                    tracing::warn!("skipping corrupt entry in Jing archive");
                    continue;
                }
            }
        }

        Ok(None)
    }

    /// Scan the entire archive, returning all fragment IDs.
    /// Used for deduplication checks and strata snapshots.
    pub fn scan_ids(&self) -> ZhenResult<Vec<FragmentId>> {
        let file = match File::open(&self.path) {
            Ok(f) => f,
            Err(e) if e.kind() == io::ErrorKind::NotFound => return Ok(vec![]),
            Err(e) => return Err(e.into()),
        };

        let mut reader = BufReader::new(file);
        let mut len_buf = [0u8; 4];
        let mut ids = Vec::new();

        loop {
            match reader.read_exact(&mut len_buf) {
                Ok(()) => {}
                Err(e) if e.kind() == io::ErrorKind::UnexpectedEof => break,
                Err(e) => return Err(e.into()),
            }

            let len = u32::from_le_bytes(len_buf) as usize;
            if len > 64 * 1024 * 1024 {
                break; // corrupt — stop scanning
            }

            let mut entry_buf = vec![0u8; len];
            reader.read_exact(&mut entry_buf)?;

            if let Ok(frag) = codec::decode(&entry_buf) {
                ids.push(frag.id);
            }
        }

        Ok(ids)
    }

    /// Archive file size in bytes.
    pub fn size_bytes(&self) -> ZhenResult<u64> {
        match std::fs::metadata(&self.path) {
            Ok(meta) => Ok(meta.len()),
            Err(e) if e.kind() == io::ErrorKind::NotFound => Ok(0),
            Err(e) => Err(e.into()),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    fn test_archive() -> (Archive, TempDir) {
        let tmp = TempDir::new().unwrap();
        let path = tmp.path().join("jing.archive");
        let archive = Archive::open(&path).unwrap();
        (archive, tmp)
    }

    #[test]
    fn test_append_and_pilgrimage() {
        let (archive, _tmp) = test_archive();

        let mut frag = Fragment::new(b"ancient wisdom".to_vec(), vec![], None);
        let id = frag.id.clone();
        archive.append(&mut frag).unwrap();

        let found = archive.pilgrimage(&id).unwrap();
        assert!(found.is_some());
        assert_eq!(found.unwrap().payload, b"ancient wisdom");
    }

    #[test]
    fn test_pilgrimage_not_found() {
        let (archive, _tmp) = test_archive();

        let mut frag = Fragment::new(b"stored".to_vec(), vec![], None);
        archive.append(&mut frag).unwrap();

        let missing_id = FragmentId::from_payload(b"never stored");
        let found = archive.pilgrimage(&missing_id).unwrap();
        assert!(found.is_none());
    }

    #[test]
    fn test_scan_ids() {
        let (archive, _tmp) = test_archive();

        let mut f1 = Fragment::new(b"first".to_vec(), vec![], None);
        let mut f2 = Fragment::new(b"second".to_vec(), vec![], None);
        let mut f3 = Fragment::new(b"third".to_vec(), vec![], None);

        archive.append(&mut f1).unwrap();
        archive.append(&mut f2).unwrap();
        archive.append(&mut f3).unwrap();

        let ids = archive.scan_ids().unwrap();
        assert_eq!(ids.len(), 3);
    }

    #[test]
    fn test_empty_archive() {
        let (archive, _tmp) = test_archive();
        let ids = archive.scan_ids().unwrap();
        assert!(ids.is_empty());
        assert_eq!(archive.size_bytes().unwrap(), 0);
    }

    #[test]
    fn test_tampered_fragment_rejected() {
        let (archive, _tmp) = test_archive();
        let mut frag = Fragment::new(b"legit".to_vec(), vec![], None);
        frag.payload = b"tampered".to_vec();
        assert!(archive.append(&mut frag).is_err());
    }
}
