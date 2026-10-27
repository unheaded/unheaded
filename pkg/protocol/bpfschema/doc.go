// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package bpfschema defines the Go mirrors of the BPF map key/value
// structures and map names shared between Go userspace (map population,
// reading) and the Rust eBPF programs under ebpf/ (map lookup, update).
//
// Rust is the source of truth. The structs the verifier loads live in
// ebpf/monad-common/src/lib.rs, ebpf/common/src/lib.rs and the program
// crates (ebpf/*/src); rust_source_test.go parses those #[repr(C)] and
// #[repr(C, packed)] definitions and #[map] declarations at test time and
// fails when a Go mirror's size, field offsets, map name or map key/value
// size differs. Add a Go mirror only after the Rust struct or map exists;
// change the Rust first, then follow it here. Never loosen the parser to
// make a mismatch pass.
//
// Map-name constants hold the names Aya gives the maps (the #[map] static
// identifier unless #[map(name = "...")] overrides it). A few key/value
// types (RingPath*, SophiaSync*, TLVHandlerEntry, PrefetchHint*) are design
// placeholders with no Rust map yet and carry no map-name constant.
//
// Multi-byte fields of the Monad and Anamnesis event are big-endian
// (network byte order) per RFC 8200; values a BPF program writes from a
// native integer (timestamps, counters) are in host byte order.
//
// Map type selection follows RFC 9669 §5.4 BPF map semantics:
//   - HASH: Exact-match lookup, dynamic sizing
//   - PERCPU_HASH: Per-CPU counters, no lock contention
//   - PERCPU_ARRAY: Fixed-index per-CPU values
//   - LRU_HASH: Bounded memory with automatic eviction
//   - ARRAY: Fixed-size, indexed access
package bpfschema
