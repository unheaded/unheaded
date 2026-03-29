//! # De (德) — Virtue / Relevance Gravity
//!
//! Fragments surface through De — not query. Context attracts relevant
//! knowledge the way gravity attracts mass. No search bar. No SQL.
//!
//! De = cosine_similarity(context_embedding, fragment_embedding)
//!
//! High De = fragment bubbles up. Low De = fragment stays in strata.

pub mod embedder;
pub mod similarity;
pub mod context;

pub use context::ContextVector;
