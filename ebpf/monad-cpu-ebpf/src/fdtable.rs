// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//! Phase 1.7 per-process file-descriptor table (ASCEND-LINUX).
//!
//! The pure index math, descriptor-kind constants, free-slot policy, and the
//! off-target inode-state model live in [`monad_common::fdtable`] so they are
//! unit-tested off-target (the ebpf bin crate's own `cargo test` is poisoned by
//! `ebpf/.cargo/config.toml`'s `build-std`; the working harness is
//! `cargo test -p monad-common` — see the Gate D battle plan's Phase 1
//! test-home correction). This module re-exports them so the BPF side
//! (`main.rs`) keeps its `fdtable::*` call sites unchanged; the live BPF maps
//! (`FD_TABLE`, `FD_INODE_MAP`) and their `get`/`get_ptr_mut` accessors live in
//! `main.rs`.

pub use monad_common::fdtable::*;
