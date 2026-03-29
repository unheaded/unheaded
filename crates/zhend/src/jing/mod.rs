//! # Jing (經) — The Classics / Deep Archive
//!
//! L3 cold storage. Append-only log of ancient fragments.
//! Access requires pilgrimage — slow, intentional, traversal-based.
//!
//! Jing is the geological bedrock. The oldest strata. Knowledge that
//! hasn't been accessed in weeks or months but NEVER dies.
//!
//! Design: append-only log files, mmap'd for read, compacted periodically.
//! Think WAL but write-only — no overwrite, no delete, no truncate.
//! The Library does not burn pages.

pub mod archive;
pub mod pilgrimage;

pub use archive::Archive;
