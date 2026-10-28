// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package errors is the conformance reference for the Unheaded Protocol Error
// Codes registry: draft-bellis-unheaded-protocol-foundation-06 §"Error Code
// Registry" and docs/protocol/error-registry.md (the draft wins on conflict).
//
// Codes are 8-bit. 0x00-0x0C are the normative codes, 0x0D is reserved,
// 0x0E-0x1E unallocated, 0x1F and 0xFF greasing, 0x20-0xFE private use.
// TestRegistryParity parses both documents at test time, so a number, name or
// level that drifts from the text fails the build's tests.
//
// The Rust twin is monad_common::ErrorCode (ebpf/monad-common), checked
// against the same file by its own host test. Shield's egress program counts
// CRC_VALIDATION_FAILED and VERSION_NOT_SUPPORTED in its ERROR_COUNTERS map
// keyed by these values (S-TRUE-WIRE Step 136).
package errors
