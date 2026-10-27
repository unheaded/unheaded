// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// parity_test.go: Go/Rust struct parity is derived from the Rust source in
// rust_source_test.go (S-TRUE-WIRE Step 121): sizes and field offsets of
// MonadRegister, AnamnesisEvent, MbcCpuState, FlowKey, FlowState,
// FlowMigrationTokenValue and FlowCancelValue are computed from the
// #[repr(C)] / #[repr(C, packed)] definitions under ebpf/*/src and compared
// with the Go structs. The hardcoded per-struct numbers that used to live
// here had drifted (MbcCpuState 128 vs Rust 136; the two packed flow-tracker
// values 48/24 vs Rust 44/20) and are gone.
//
// One hardcoded sentinel stays: the Monad is 20 bytes on the wire
// (foundation-06), so a parser bug that reads the Rust wrongly cannot make
// both sides agree on a different number.

package bpfschema

import (
	"testing"
	"unsafe"
)

// TestMonadSizeSentinel pins the frozen 20-byte Monad independently of the
// Rust-source parser.
func TestMonadSizeSentinel(t *testing.T) {
	const monadWireSize = 20 // foundation-06 §Packet Format; wire format v0x01 is frozen.
	if MonadRegisterSize != monadWireSize {
		t.Fatalf("MonadRegisterSize = %d, want %d", MonadRegisterSize, monadWireSize)
	}
	if got := unsafe.Sizeof(MonadRegister{}); got != monadWireSize {
		t.Fatalf("unsafe.Sizeof(MonadRegister{}) = %d, want %d", got, monadWireSize)
	}
	if got := unsafe.Sizeof(AnamnesisEvent{}); got != 12+monadWireSize {
		t.Fatalf("unsafe.Sizeof(AnamnesisEvent{}) = %d, want 12-byte header + Monad = %d", got, 12+monadWireSize)
	}
}
