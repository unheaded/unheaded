// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package tlv

import (
	"testing"
)

// FuzzParse feeds random bytes to tlv.Parse to verify it never panics
// and that successful parses produce valid TLVBlocks.
func FuzzParse(f *testing.F) {
	// Valid single TLV.
	f.Add([]byte{0x01, 0x03, 'f', 'o', 'o'})
	// Two TLVs.
	f.Add([]byte{0x01, 0x02, 'a', 'b', 0x02, 0x03, 'x', 'y', 'z'})
	// Empty input.
	f.Add([]byte{})
	// Zero-length value TLV.
	f.Add([]byte{0x01, 0x00})
	// Truncated header.
	f.Add([]byte{0x01})
	// Truncated value.
	f.Add([]byte{0x01, 0x05, 'a', 'b'})
	// Four TLVs at the max limit.
	f.Add([]byte{0x00, 0x01, 'a', 0x01, 0x01, 'b', 0x02, 0x01, 'c', 0x03, 0x01, 'd'})
	// Five TLVs exceeding MaxTLVsPerMonad.
	f.Add([]byte{0x00, 0x00, 0x01, 0x00, 0x02, 0x00, 0x03, 0x00, 0x04, 0x00})
	// All 0xFF bytes.
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF})

	f.Fuzz(func(t *testing.T, data []byte) {
		block, err := Parse(data)
		if err != nil {
			return // errors are expected for malformed input
		}

		// Invariant: block must not exceed max TLVs per Monad.
		if len(block) > MaxTLVsPerMonad {
			t.Errorf("Parse returned %d TLVs, max is %d", len(block), MaxTLVsPerMonad)
		}

		// Invariant: each TLV's value length must match its declared length.
		for i, tlv := range block {
			if len(tlv.Value) != int(tlv.Length) {
				t.Errorf("TLV[%d]: value len=%d, declared length=%d", i, len(tlv.Value), tlv.Length)
			}
		}
	})
}

// FuzzParseSerializeRoundtrip verifies that Parse -> Serialize -> Parse
// produces the same TLVBlock for any valid input.
func FuzzParseSerializeRoundtrip(f *testing.F) {
	f.Add([]byte{0x01, 0x03, 'f', 'o', 'o'})
	f.Add([]byte{0x01, 0x02, 'a', 'b', 0x02, 0x03, 'x', 'y', 'z'})
	f.Add([]byte{0x01, 0x00})
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		block1, err := Parse(data)
		if err != nil {
			return
		}

		// Serialize the parsed block.
		serialized, err := Serialize(block1)
		if err != nil {
			// Serialize can fail if the block was somehow invalid.
			return
		}

		// Re-parse.
		block2, err := Parse(serialized)
		if err != nil {
			t.Fatalf("Parse(Serialize(block)) error: %v", err)
		}

		// Invariant: both blocks should have the same length.
		if len(block1) != len(block2) {
			t.Fatalf("roundtrip block length mismatch: %d vs %d", len(block1), len(block2))
		}

		for i := range block1 {
			if block1[i].Type != block2[i].Type {
				t.Errorf("TLV[%d] type mismatch: 0x%02x vs 0x%02x", i, block1[i].Type, block2[i].Type)
			}
			if block1[i].Length != block2[i].Length {
				t.Errorf("TLV[%d] length mismatch: %d vs %d", i, block1[i].Length, block2[i].Length)
			}
			if string(block1[i].Value) != string(block2[i].Value) {
				t.Errorf("TLV[%d] value mismatch", i)
			}
		}
	})
}

// FuzzValidateBlock feeds random TLV blocks to ValidateBlock.
func FuzzValidateBlock(f *testing.F) {
	f.Add([]byte{0x01, 0x03, 'f', 'o', 'o'})
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		block, err := Parse(data)
		if err != nil {
			return
		}

		// ValidateBlock should not panic on any parsed block.
		_ = ValidateBlock(block)
	})
}
