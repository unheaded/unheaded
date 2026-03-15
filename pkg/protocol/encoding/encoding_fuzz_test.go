// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package encoding

import (
	"testing"
)

// FuzzDecodeVarint feeds random bytes to DecodeVarint to verify it never panics.
// This is security-critical: untrusted wire data hits DecodeVarint first.
func FuzzDecodeVarint(f *testing.F) {
	// Seed corpus: valid encodings for each length class.
	f.Add([]byte{0x00})                                                 // 1-byte: 0
	f.Add([]byte{0x25})                                                 // 1-byte: 37 (RFC 9000 example)
	f.Add([]byte{0x3F})                                                 // 1-byte: max (63)
	f.Add([]byte{0x40, 0x40})                                           // 2-byte: 64
	f.Add([]byte{0x7B, 0xBD})                                           // 2-byte: 15293 (RFC 9000 example)
	f.Add([]byte{0x7F, 0xFF})                                           // 2-byte: max (16383)
	f.Add([]byte{0x80, 0x00, 0x40, 0x00})                               // 4-byte: 16384
	f.Add([]byte{0x9D, 0x7F, 0x3E, 0x7D})                               // 4-byte: 494878333 (RFC 9000 example)
	f.Add([]byte{0xBF, 0xFF, 0xFF, 0xFF})                               // 4-byte: max (1073741823)
	f.Add([]byte{0xC0, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x00})       // 8-byte: 1073741824
	f.Add([]byte{0xC2, 0x19, 0x7C, 0x5E, 0xFF, 0x14, 0xE8, 0x8C})       // 8-byte: 151288809941952652
	f.Add([]byte{})                                                      // empty
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF})       // max 8-byte encoding
	f.Add([]byte{0x40})                                                  // truncated 2-byte
	f.Add([]byte{0x80, 0x00})                                            // truncated 4-byte
	f.Add([]byte{0xC0, 0x00, 0x00, 0x00})                               // truncated 8-byte

	f.Fuzz(func(t *testing.T, data []byte) {
		val, n, err := DecodeVarint(data)
		if err != nil {
			return // errors are expected for malformed input
		}

		// Invariant: consumed bytes must be 1, 2, 4, or 8.
		if n != 1 && n != 2 && n != 4 && n != 8 {
			t.Errorf("DecodeVarint consumed %d bytes, want 1/2/4/8", n)
		}

		// Invariant: decoded value must not exceed MaxVarint.
		if val > MaxVarint {
			t.Errorf("DecodeVarint returned %d, exceeds MaxVarint (%d)", val, MaxVarint)
		}

		// Invariant: re-encoding should produce a valid encoding that decodes
		// to the same value.
		reEncoded, err := EncodeVarint(val)
		if err != nil {
			t.Fatalf("EncodeVarint(%d) error after successful decode: %v", val, err)
		}
		reDecoded, _, err := DecodeVarint(reEncoded)
		if err != nil {
			t.Fatalf("DecodeVarint(re-encoded) error: %v", err)
		}
		if reDecoded != val {
			t.Errorf("roundtrip mismatch: decode=%d, re-encode-decode=%d", val, reDecoded)
		}
	})
}

// FuzzEncodeVarint feeds random uint64 values to EncodeVarint and verifies roundtrip.
func FuzzEncodeVarint(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(1))
	f.Add(uint64(63))
	f.Add(uint64(64))
	f.Add(uint64(16383))
	f.Add(uint64(16384))
	f.Add(uint64(1073741823))
	f.Add(uint64(1073741824))
	f.Add(MaxVarint)
	f.Add(MaxVarint + 1)
	f.Add(^uint64(0)) // max uint64

	f.Fuzz(func(t *testing.T, val uint64) {
		encoded, err := EncodeVarint(val)
		if val > MaxVarint {
			if err == nil {
				t.Errorf("EncodeVarint(%d) should error for values > MaxVarint", val)
			}
			return
		}
		if err != nil {
			t.Fatalf("EncodeVarint(%d) unexpected error: %v", val, err)
		}

		decoded, n, err := DecodeVarint(encoded)
		if err != nil {
			t.Fatalf("DecodeVarint(EncodeVarint(%d)) error: %v", val, err)
		}
		if decoded != val {
			t.Errorf("roundtrip(%d) = %d", val, decoded)
		}
		if n != len(encoded) {
			t.Errorf("consumed %d bytes, encoded %d bytes", n, len(encoded))
		}
	})
}

// FuzzDecodeExponent feeds random uint16 values to DecodeExponent.
func FuzzDecodeExponent(f *testing.F) {
	f.Add(uint16(0))
	f.Add(uint16(0x0110)) // small value: exp=1, mantissa=1
	f.Add(uint16(0x0420)) // exp=4, mantissa=2
	f.Add(uint16(0x2080)) // exp=32, mantissa=8
	f.Add(uint16(0xFFF0)) // overflow case
	f.Add(uint16(0xFF00)) // large exp, zero mantissa
	f.Add(uint16(0x0010)) // exp=0, mantissa=1 (still zero because exp=0)
	f.Add(uint16(0xFFFF)) // all bits set

	f.Fuzz(func(t *testing.T, encoded uint16) {
		val, err := DecodeExponent(encoded)
		if err != nil {
			return // overflow errors are expected
		}
		// The decoded value should be representable as uint64 (no panic).
		_ = val
	})
}

// FuzzDecodeTLV feeds random bytes to DecodeTLV.
func FuzzDecodeTLV(f *testing.F) {
	f.Add([]byte{0x01, 0x03, 0xAA, 0xBB, 0xCC})            // valid TLV
	f.Add([]byte{0x42, 0x00})                                 // zero-length value
	f.Add([]byte{0x01, 0x04, 0xDE, 0xAD, 0xBE, 0xEF})        // 4-byte value
	f.Add([]byte{})                                            // empty
	f.Add([]byte{0x01})                                        // truncated header
	f.Add([]byte{0x01, 0xFF})                                  // length exceeds buffer
	f.Add([]byte{0x01, 0x02, 0xAA})                            // truncated value

	f.Fuzz(func(t *testing.T, data []byte) {
		tlv, n, err := DecodeTLV(data)
		if err != nil {
			return
		}

		// Invariant: consumed bytes = 2 + length.
		expected := 2 + int(tlv.Length)
		if n != expected {
			t.Errorf("DecodeTLV consumed %d bytes, want %d (length=%d)", n, expected, tlv.Length)
		}

		// Invariant: value length matches declared length.
		if len(tlv.Value) != int(tlv.Length) {
			t.Errorf("value len=%d, declared length=%d", len(tlv.Value), tlv.Length)
		}
	})
}

// FuzzDecodeTLVSequence feeds random bytes to DecodeTLVSequence.
func FuzzDecodeTLVSequence(f *testing.F) {
	// Two TLVs back to back.
	f.Add([]byte{0x01, 0x02, 0xAA, 0xBB, 0x02, 0x01, 0xCC})
	f.Add([]byte{0x01, 0x00, 0x02, 0x00, 0x03, 0x00, 0x04, 0x00}) // four zero-length TLVs
	f.Add([]byte{})
	f.Add([]byte{0xFF, 0xFF})

	f.Fuzz(func(t *testing.T, data []byte) {
		tlvs, n, err := DecodeTLVSequence(data)
		if err != nil {
			// Partial decode is valid -- just verify no panic.
			_ = tlvs
			_ = n
			return
		}

		// Invariant: total consumed bytes should equal sum of individual TLV sizes.
		totalExpected := 0
		for _, tlv := range tlvs {
			totalExpected += 2 + int(tlv.Length)
		}
		if n != totalExpected {
			t.Errorf("DecodeTLVSequence consumed %d bytes, expected %d", n, totalExpected)
		}
	})
}

// FuzzCRC16CCITT verifies CRC-16 is deterministic on any input.
func FuzzCRC16CCITT(f *testing.F) {
	f.Add([]byte("123456789"))
	f.Add([]byte{})
	f.Add([]byte{0x00})
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF})

	f.Fuzz(func(t *testing.T, data []byte) {
		a := CRC16CCITT(data)
		b := CRC16CCITT(data)
		if a != b {
			t.Errorf("CRC16CCITT not deterministic: 0x%04X != 0x%04X", a, b)
		}
	})
}

// FuzzCRC32MPEG2 verifies CRC-32 is deterministic on any input.
func FuzzCRC32MPEG2(f *testing.F) {
	f.Add([]byte("123456789"))
	f.Add([]byte{})
	f.Add([]byte{0x00})
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF})

	f.Fuzz(func(t *testing.T, data []byte) {
		a := CRC32MPEG2(data)
		b := CRC32MPEG2(data)
		if a != b {
			t.Errorf("CRC32MPEG2 not deterministic: 0x%08X != 0x%08X", a, b)
		}
	})
}
