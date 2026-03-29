//! # Pu (朴) — The Uncarved Block
//!
//! Fragment format for Zhen. Each Pu is content-addressed via BLAKE3,
//! self-describing, and uncommitted to meaning until encountered in context.
//!
//! A Pu becomes meaningful only through De (relevance) — like jade that
//! reveals its Li (pattern) only when cut.

pub mod fragment;
pub mod codec;
pub mod store;

pub use fragment::{Fragment, FragmentId, Tier};
pub use store::TieredStore;
