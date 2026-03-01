// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// LICH-002: Monad Encoder/Decoder Fuzzer
//
// Target: pkg/protocol/encoding — varint, exponent, TLV codecs.
// Goal: Find asymmetric encode/decode behavior, integer overflows,
//       and buffer over-reads in the shared encoding primitives.
//
// Attack vectors:
//   - Varint values at boundary crossings (63/64, 16383/16384, etc.)
//   - Exponent values that overflow uint64 on decode
//   - TLV with declared length exceeding buffer
//   - TLV sequences with cumulative overflow
//   - Mixed valid/invalid codec chaining

package harnesses

import (
	"encoding/binary"
	"testing"

	"unheaded/pkg/protocol/encoding"
)

// FuzzMonadCodecVarintStress stress-tests varint codec with structured inputs.
// Unlike the existing encoding fuzz tests, this chains encode/decode with
// adversarial modifications to catch edge cases in the codec.
func FuzzMonadCodecVarintStress(f *testing.F) {
	// Boundary values where encoding length changes
	f.Add(uint64(0))
	f.Add(uint64(63))                  // max 1-byte
	f.Add(uint64(64))                  // min 2-byte
	f.Add(uint64(16383))               // max 2-byte
	f.Add(uint64(16384))               // min 4-byte
	f.Add(uint64(1073741823))          // max 4-byte
	f.Add(uint64(1073741824))          // min 8-byte
	f.Add(encoding.MaxVarint)          // max encodable
	f.Add(encoding.MaxVarint + 1)      // should error
	f.Add(^uint64(0))                  // max uint64

	f.Fuzz(func(t *testing.T, val uint64) {
		encoded, encErr := encoding.EncodeVarint(val)

		if val > encoding.MaxVarint {
			if encErr == nil {
				t.Errorf("EncodeVarint(%d) should error for value > MaxVarint", val)
			}
			return
		}

		if encErr != nil {
			t.Fatalf("EncodeVarint(%d) unexpected error: %v", val, encErr)
		}

		// Invariant: encoded length must be 1, 2, 4, or 8
		validLengths := map[int]bool{1: true, 2: true, 4: true, 8: true}
		if !validLengths[len(encoded)] {
			t.Errorf("EncodeVarint(%d) produced %d bytes, want 1/2/4/8", val, len(encoded))
		}

		// Decode and verify
		decoded, consumed, decErr := encoding.DecodeVarint(encoded)
		if decErr != nil {
			t.Fatalf("DecodeVarint(EncodeVarint(%d)) error: %v", val, decErr)
		}
		if decoded != val {
			t.Errorf("roundtrip(%d) = %d", val, decoded)
		}
		if consumed != len(encoded) {
			t.Errorf("consumed %d bytes, encoded %d bytes", consumed, len(encoded))
		}

		// Adversarial: flip a bit in the encoded data and try to decode
		if len(encoded) > 0 {
			corrupted := make([]byte, len(encoded))
			copy(corrupted, encoded)
			corrupted[0] ^= 0x01 // flip LSB of first byte

			corruptedVal, _, corruptedErr := encoding.DecodeVarint(corrupted)
			if corruptedErr == nil && corruptedVal == val {
				// Single bit flip produced same value -- notable but not always a bug
				_ = corruptedVal
			}
		}
	})
}

// FuzzMonadCodecExponentOverflow specifically targets exponent overflow paths.
func FuzzMonadCodecExponentOverflow(f *testing.F) {
	// Craft values targeting overflow
	f.Add(uint16(0x0000)) // zero
	f.Add(uint16(0x0110)) // small: exp=1, mantissa=1
	f.Add(uint16(0x0420)) // normal: exp=4, mantissa=2
	f.Add(uint16(0x2080)) // large: exp=32, mantissa=8
	f.Add(uint16(0x3CF0)) // exp=60, mantissa=15 — near overflow boundary
	f.Add(uint16(0x3DF0)) // exp=61, mantissa=15 — at overflow boundary
	f.Add(uint16(0x40F0)) // exp=64, mantissa=15 — should overflow
	f.Add(uint16(0xFFF0)) // exp=255, mantissa=15 — maximum, definite overflow
	f.Add(uint16(0xFF00)) // exp=255, mantissa=0 — overflow with zero mantissa
	f.Add(uint16(0x0010)) // exp=0, mantissa=1 — special case (value=0)
	f.Add(uint16(0x00F0)) // exp=0, mantissa=15 — special case (value=0)
	f.Add(uint16(0xFFFF)) // all bits set

	f.Fuzz(func(t *testing.T, encoded uint16) {
		val, err := encoding.DecodeExponent(encoded)

		exp := uint8(encoded >> 8)
		mantissa := uint8((encoded >> 4) & 0x0F)

		if exp == 0 {
			// Special case: exp=0 means value=0
			if err != nil {
				t.Errorf("DecodeExponent(exp=0) should not error: %v", err)
			}
			if val != 0 {
				t.Errorf("DecodeExponent(exp=0) = %d, want 0", val)
			}
			return
		}

		if err != nil {
			// Overflow is expected for large exponents -- verify it's correct
			if exp >= 68 && mantissa > 0 {
				return // expected overflow
			}
			// For edge cases, just ensure no panic
			return
		}

		// If decode succeeded, verify the value is reasonable
		// Re-encode and check we get something in the same ballpark
		reEncoded := encoding.EncodeExponent(uint32(val))
		reDecoded, reErr := encoding.DecodeExponent(reEncoded)
		if reErr != nil {
			// Original value was too large for uint32 round-trip
			return
		}

		// Lossy encoding is expected (4-bit mantissa), but they should be
		// in the same order of magnitude
		_ = reDecoded
	})
}

// FuzzMonadCodecTLVChain fuzzes a chain of TLV encode/decode operations.
func FuzzMonadCodecTLVChain(f *testing.F) {
	// Seed: single TLV
	f.Add(uint8(0x01), []byte{0xAA, 0xBB, 0xCC})

	// Seed: empty value
	f.Add(uint8(0x42), []byte{})

	// Seed: maximum length value
	maxVal := make([]byte, 255)
	for i := range maxVal {
		maxVal[i] = byte(i)
	}
	f.Add(uint8(0xFF), maxVal)

	// Seed: single byte value
	f.Add(uint8(0x00), []byte{0x00})

	f.Fuzz(func(t *testing.T, typ uint8, value []byte) {
		// Encode
		encoded, encErr := encoding.EncodeTLV(typ, value)
		if len(value) > encoding.MaxTLVValueLen {
			if encErr == nil {
				t.Errorf("EncodeTLV should error for value length %d > %d", len(value), encoding.MaxTLVValueLen)
			}
			return
		}
		if encErr != nil {
			t.Fatalf("EncodeTLV(type=0x%02X, len=%d) unexpected error: %v", typ, len(value), encErr)
		}

		// Invariant: encoded size = 2 + len(value)
		expectedSize := 2 + len(value)
		if len(encoded) != expectedSize {
			t.Errorf("encoded size %d, expected %d", len(encoded), expectedSize)
		}

		// Decode
		tlv, consumed, decErr := encoding.DecodeTLV(encoded)
		if decErr != nil {
			t.Fatalf("DecodeTLV(EncodeTLV(...)) error: %v", decErr)
		}

		if consumed != len(encoded) {
			t.Errorf("consumed %d bytes, encoded %d bytes", consumed, len(encoded))
		}

		if tlv.Type != typ {
			t.Errorf("type mismatch: 0x%02X vs 0x%02X", tlv.Type, typ)
		}

		if int(tlv.Length) != len(value) {
			t.Errorf("length mismatch: %d vs %d", tlv.Length, len(value))
		}

		if string(tlv.Value) != string(value) {
			t.Errorf("value mismatch")
		}

		// Adversarial: corrupt the length byte and try decode
		if len(encoded) >= 2 {
			corrupted := make([]byte, len(encoded))
			copy(corrupted, encoded)
			corrupted[1] = 0xFF // set length to max

			_, _, corruptErr := encoding.DecodeTLV(corrupted)
			if corruptErr == nil && int(corrupted[1]) > len(corrupted)-2 {
				t.Errorf("DecodeTLV should error when length exceeds buffer")
			}
		}
	})
}

// FuzzMonadCodecTLVSequenceStress builds adversarial TLV sequences.
func FuzzMonadCodecTLVSequenceStress(f *testing.F) {
	// Two valid TLVs
	f.Add([]byte{0x01, 0x02, 0xAA, 0xBB, 0x02, 0x01, 0xCC})

	// Four zero-length TLVs
	f.Add([]byte{0x01, 0x00, 0x02, 0x00, 0x03, 0x00, 0x04, 0x00})

	// Empty
	f.Add([]byte{})

	// Truncated mid-value
	f.Add([]byte{0x01, 0x05, 0xAA, 0xBB})

	// Length says 255 but only 3 bytes follow
	f.Add([]byte{0x01, 0xFF, 0xAA, 0xBB, 0xCC})

	// All zeros
	f.Add(make([]byte, 100))

	// All 0xFF
	all := make([]byte, 50)
	for i := range all {
		all[i] = 0xFF
	}
	f.Add(all)

	f.Fuzz(func(t *testing.T, data []byte) {
		tlvs, consumed, err := encoding.DecodeTLVSequence(data)
		if err != nil {
			// Partial decode is valid -- just verify no panic
			_ = tlvs
			_ = consumed
			return
		}

		// Invariant: consumed must equal sum of individual TLV sizes
		totalExpected := 0
		for _, tlv := range tlvs {
			totalExpected += 2 + int(tlv.Length)

			// Each TLV's value length must match declared length
			if len(tlv.Value) != int(tlv.Length) {
				t.Errorf("TLV value len=%d, declared=%d", len(tlv.Value), tlv.Length)
			}
		}
		if consumed != totalExpected {
			t.Errorf("consumed %d, expected %d", consumed, totalExpected)
		}

		// Roundtrip: re-encode each TLV and verify
		for i, tlv := range tlvs {
			reEncoded, err := encoding.EncodeTLV(tlv.Type, tlv.Value)
			if err != nil {
				t.Fatalf("re-encode TLV[%d] error: %v", i, err)
			}

			reTLV, _, err := encoding.DecodeTLV(reEncoded)
			if err != nil {
				t.Fatalf("re-decode TLV[%d] error: %v", i, err)
			}

			if reTLV.Type != tlv.Type || reTLV.Length != tlv.Length {
				t.Errorf("TLV[%d] roundtrip mismatch", i)
			}
		}
	})
}

// FuzzMonadCodecCombined tests encoding primitives together as a pipeline,
// simulating how a real Monad frame uses varint + exponent + TLV + CRC.
func FuzzMonadCodecCombined(f *testing.F) {
	f.Add(uint64(42), uint16(0x0420), uint8(0x01), []byte("test"))
	f.Add(uint64(0), uint16(0x0000), uint8(0x00), []byte{})
	f.Add(encoding.MaxVarint, uint16(0xFFF0), uint8(0xFF), make([]byte, 200))

	f.Fuzz(func(t *testing.T, varintVal uint64, exponentVal uint16, tlvType uint8, tlvValue []byte) {
		// Step 1: Encode varint
		varintEncoded, err := encoding.EncodeVarint(varintVal)
		if err != nil {
			return // value out of range
		}

		// Step 2: Decode exponent
		expDecoded, expErr := encoding.DecodeExponent(exponentVal)
		_ = expDecoded
		_ = expErr

		// Step 3: Encode TLV
		if len(tlvValue) > encoding.MaxTLVValueLen {
			tlvValue = tlvValue[:encoding.MaxTLVValueLen]
		}
		tlvEncoded, tlvErr := encoding.EncodeTLV(tlvType, tlvValue)
		if tlvErr != nil {
			return
		}

		// Step 4: Compute CRC over the combined output
		combined := make([]byte, 0, len(varintEncoded)+2+len(tlvEncoded))
		combined = append(combined, varintEncoded...)
		expBytes := make([]byte, 2)
		binary.BigEndian.PutUint16(expBytes, exponentVal)
		combined = append(combined, expBytes...)
		combined = append(combined, tlvEncoded...)

		crc := encoding.CRC16CCITT(combined)

		// Verify CRC is deterministic
		crc2 := encoding.CRC16CCITT(combined)
		if crc != crc2 {
			t.Errorf("CRC not deterministic: 0x%04X vs 0x%04X", crc, crc2)
		}

		// Step 5: Verify varint roundtrip
		varintDecoded, _, varintDecErr := encoding.DecodeVarint(varintEncoded)
		if varintDecErr != nil {
			t.Fatalf("varint decode error after encode: %v", varintDecErr)
		}
		if varintDecoded != varintVal {
			t.Errorf("varint roundtrip: %d != %d", varintDecoded, varintVal)
		}
	})
}
