// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.
//! # Pu (朴) — The Uncarved Block
//!
//! Fragment format for Zhen. Each Pu is content-addressed via BLAKE3,
//! self-describing, and uncommitted to meaning until encountered in context.
//!
//! A Pu becomes meaningful only through De (relevance) — like jade that
//! reveals its Li (pattern) only when cut.

pub mod codec;
pub mod fragment;
pub mod store;

pub use fragment::{Fragment, FragmentId, Tier};
pub use store::TieredStore;
