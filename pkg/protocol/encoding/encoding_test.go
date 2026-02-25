// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package encoding

import (
	"bytes"
	"math"
	"testing"
)

// === VARINT TESTS (RFC 9000 §16) ===

func TestEncodeDecodeVarint_SingleByte(t *testing.T) {
	// RFC 9000 §16: Values 0-63 use 1 byte (prefix 00).
	tests := []struct {
		value    uint64
		wantLen  int
		wantByte byte
	}{
		{0, 1, 0x00},
		{1, 1, 0x01},
		{37, 1, 0x25},     // RFC 9000 §A.1 example
		{63, 1, 0x3F},     // max 1-byte value
	}
	for _, tt := range tests {
		encoded, err := EncodeVarint(tt.value)
		if err != nil {
			t.Fatalf("EncodeVarint(%d) error: %v", tt.value, err)
		}
		if len(encoded) != tt.wantLen {
			t.Errorf("EncodeVarint(%d) len = %d, want %d", tt.value, len(encoded), tt.wantLen)
		}
		if encoded[0] != tt.wantByte {
			t.Errorf("EncodeVarint(%d) = 0x%02x, want 0x%02x", tt.value, encoded[0], tt.wantByte)
		}

		decoded, n, err := DecodeVarint(encoded)
		if err != nil {
			t.Fatalf("DecodeVarint error: %v", err)
		}
		if decoded != tt.value {
			t.Errorf("DecodeVarint = %d, want %d", decoded, tt.value)
		}
		if n != tt.wantLen {
			t.Errorf("DecodeVarint consumed %d bytes, want %d", n, tt.wantLen)
		}
	}
}

func TestEncodeDecodeVarint_TwoBytes(t *testing.T) {
	// RFC 9000 §16: Values 64-16383 use 2 bytes (prefix 01).
	tests := []uint64{64, 15293, 16383}
	for _, v := range tests {
		encoded, err := EncodeVarint(v)
		if err != nil {
			t.Fatalf("EncodeVarint(%d) error: %v", v, err)
		}
		if len(encoded) != 2 {
			t.Errorf("EncodeVarint(%d) len = %d, want 2", v, len(encoded))
		}
		// Verify prefix bits are 01.
		if encoded[0]>>6 != 1 {
			t.Errorf("EncodeVarint(%d) prefix = %d, want 1", v, encoded[0]>>6)
		}

		decoded, n, err := DecodeVarint(encoded)
		if err != nil {
			t.Fatalf("DecodeVarint error: %v", err)
		}
		if decoded != v || n != 2 {
			t.Errorf("DecodeVarint = (%d, %d), want (%d, 2)", decoded, n, v)
		}
	}
}

func TestEncodeDecodeVarint_FourBytes(t *testing.T) {
	tests := []uint64{16384, 494878333, 1073741823}
	for _, v := range tests {
		encoded, err := EncodeVarint(v)
		if err != nil {
			t.Fatalf("EncodeVarint(%d) error: %v", v, err)
		}
		if len(encoded) != 4 {
			t.Errorf("EncodeVarint(%d) len = %d, want 4", v, len(encoded))
		}
		if encoded[0]>>6 != 2 {
			t.Errorf("EncodeVarint(%d) prefix = %d, want 2", v, encoded[0]>>6)
		}

		decoded, n, err := DecodeVarint(encoded)
		if err != nil {
			t.Fatalf("DecodeVarint error: %v", err)
		}
		if decoded != v || n != 4 {
			t.Errorf("DecodeVarint = (%d, %d), want (%d, 4)", decoded, n, v)
		}
	}
}

func TestEncodeDecodeVarint_EightBytes(t *testing.T) {
	tests := []uint64{1073741824, 151288809941952652, MaxVarint}
	for _, v := range tests {
		encoded, err := EncodeVarint(v)
		if err != nil {
			t.Fatalf("EncodeVarint(%d) error: %v", v, err)
		}
		if len(encoded) != 8 {
			t.Errorf("EncodeVarint(%d) len = %d, want 8", v, len(encoded))
		}
		if encoded[0]>>6 != 3 {
			t.Errorf("EncodeVarint(%d) prefix = %d, want 3", v, encoded[0]>>6)
		}

		decoded, n, err := DecodeVarint(encoded)
		if err != nil {
			t.Fatalf("DecodeVarint error: %v", err)
		}
		if decoded != v || n != 8 {
			t.Errorf("DecodeVarint = (%d, %d), want (%d, 8)", decoded, n, v)
		}
	}
}

func TestEncodeVarint_TooLarge(t *testing.T) {
	_, err := EncodeVarint(MaxVarint + 1)
	if err != ErrVarintTooLarge {
		t.Errorf("EncodeVarint(MaxVarint+1) error = %v, want ErrVarintTooLarge", err)
	}
}

func TestDecodeVarint_Truncated(t *testing.T) {
	// Empty buffer.
	_, _, err := DecodeVarint(nil)
	if err != ErrVarintTruncated {
		t.Errorf("DecodeVarint(nil) error = %v, want ErrVarintTruncated", err)
	}

	// 2-byte prefix but only 1 byte available.
	_, _, err = DecodeVarint([]byte{0x40})
	if err != ErrVarintTruncated {
		t.Errorf("DecodeVarint(truncated 2-byte) error = %v, want ErrVarintTruncated", err)
	}
}

// === EXPONENT ENCODING TESTS (Monad Wire Format) ===

func TestEncodeDecodeExponent_Zero(t *testing.T) {
	encoded := EncodeExponent(0)
	if encoded != 0 {
		t.Errorf("EncodeExponent(0) = 0x%04x, want 0x0000", encoded)
	}

	decoded, err := DecodeExponent(0)
	if err != nil {
		t.Fatalf("DecodeExponent(0) error: %v", err)
	}
	if decoded != 0 {
		t.Errorf("DecodeExponent(0) = %d, want 0", decoded)
	}
}

func TestEncodeDecodeExponent_SmallValues(t *testing.T) {
	// Test that small values round-trip approximately.
	values := []uint32{1, 2, 4, 8, 16, 100, 255, 1000}
	for _, v := range values {
		encoded := EncodeExponent(v)
		decoded, err := DecodeExponent(encoded)
		if err != nil {
			t.Fatalf("DecodeExponent(EncodeExponent(%d)) error: %v", v, err)
		}
		// Exponent encoding is lossy — verify within 2× range.
		if decoded == 0 && v != 0 {
			t.Errorf("DecodeExponent(EncodeExponent(%d)) = 0, expected non-zero", v)
		}
		if decoded > uint64(v)*2 {
			t.Errorf("DecodeExponent(EncodeExponent(%d)) = %d, too far from original", v, decoded)
		}
	}
}

func TestDecodeExponent_Overflow(t *testing.T) {
	// Craft a value that would overflow: exp=255 (shift=254) with any mantissa.
	// exp byte = 0xFF, mantissa nibble = 0xF0 → encoded = 0xFF F0.
	crafted := uint16(0xFFF0)
	_, err := DecodeExponent(crafted)
	if err != ErrExponentOverflow {
		t.Errorf("DecodeExponent(0xFFF0) error = %v, want ErrExponentOverflow", err)
	}
}

// === CRC TESTS ===

func TestCRC16CCITT_KnownVectors(t *testing.T) {
	// Standard test vector: "123456789" → CRC-16/CCITT-FALSE = 0x29B1.
	data := []byte("123456789")
	got := CRC16CCITT(data)
	want := uint16(0x29B1)
	if got != want {
		t.Errorf("CRC16CCITT(%q) = 0x%04X, want 0x%04X", data, got, want)
	}
}

func TestCRC16CCITT_Empty(t *testing.T) {
	got := CRC16CCITT(nil)
	want := uint16(0xFFFF) // Initial value with no data.
	if got != want {
		t.Errorf("CRC16CCITT(nil) = 0x%04X, want 0x%04X", got, want)
	}
}

func TestCRC32MPEG2_KnownVectors(t *testing.T) {
	// Standard test vector: "123456789" → CRC-32/MPEG-2 = 0x0376E6E7.
	data := []byte("123456789")
	got := CRC32MPEG2(data)
	want := uint32(0x0376E6E7)
	if got != want {
		t.Errorf("CRC32MPEG2(%q) = 0x%08X, want 0x%08X", data, got, want)
	}
}

func TestCRC32MPEG2_Empty(t *testing.T) {
	got := CRC32MPEG2(nil)
	want := uint32(0xFFFFFFFF)
	if got != want {
		t.Errorf("CRC32MPEG2(nil) = 0x%08X, want 0x%08X", got, want)
	}
}

// === TLV TESTS (RFC 9114 §7 / RFC 8200 §4) ===

func TestEncodeTLV_Simple(t *testing.T) {
	value := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	encoded, err := EncodeTLV(0x42, value)
	if err != nil {
		t.Fatalf("EncodeTLV error: %v", err)
	}
	if len(encoded) != 6 {
		t.Fatalf("EncodeTLV len = %d, want 6", len(encoded))
	}
	if encoded[0] != 0x42 {
		t.Errorf("TLV type = 0x%02x, want 0x42", encoded[0])
	}
	if encoded[1] != 4 {
		t.Errorf("TLV length = %d, want 4", encoded[1])
	}
	if !bytes.Equal(encoded[2:], value) {
		t.Errorf("TLV value = %x, want %x", encoded[2:], value)
	}
}

func TestEncodeTLV_Empty(t *testing.T) {
	encoded, err := EncodeTLV(0x01, nil)
	if err != nil {
		t.Fatalf("EncodeTLV error: %v", err)
	}
	if len(encoded) != 2 {
		t.Fatalf("EncodeTLV(empty) len = %d, want 2", len(encoded))
	}
	if encoded[1] != 0 {
		t.Errorf("TLV length = %d, want 0", encoded[1])
	}
}

func TestEncodeTLV_TooLarge(t *testing.T) {
	bigValue := make([]byte, MaxTLVValueLen+1)
	_, err := EncodeTLV(0x01, bigValue)
	if err == nil {
		t.Error("EncodeTLV(oversized) should return error")
	}
}

func TestDecodeTLV_Simple(t *testing.T) {
	wire := []byte{0x42, 0x04, 0xDE, 0xAD, 0xBE, 0xEF}
	tlv, n, err := DecodeTLV(wire)
	if err != nil {
		t.Fatalf("DecodeTLV error: %v", err)
	}
	if n != 6 {
		t.Errorf("DecodeTLV consumed %d bytes, want 6", n)
	}
	if tlv.Type != 0x42 || tlv.Length != 4 {
		t.Errorf("TLV type/length = (%d, %d), want (0x42, 4)", tlv.Type, tlv.Length)
	}
	if !bytes.Equal(tlv.Value, []byte{0xDE, 0xAD, 0xBE, 0xEF}) {
		t.Errorf("TLV value = %x, want DEADBEEF", tlv.Value)
	}
}

func TestDecodeTLV_Truncated(t *testing.T) {
	// Too short for header.
	_, _, err := DecodeTLV([]byte{0x01})
	if err != ErrTLVTruncated {
		t.Errorf("DecodeTLV(1 byte) error = %v, want ErrTLVTruncated", err)
	}

	// Header says 4 bytes but only 2 available.
	_, _, err = DecodeTLV([]byte{0x01, 0x04, 0xAA, 0xBB})
	if err != ErrTLVTruncated {
		t.Errorf("DecodeTLV(truncated value) error = %v, want ErrTLVTruncated", err)
	}
}

func TestDecodeTLVSequence(t *testing.T) {
	// Two TLVs back to back.
	wire := []byte{
		0x01, 0x02, 0xAA, 0xBB,
		0x02, 0x03, 0xCC, 0xDD, 0xEE,
	}
	tlvs, n, err := DecodeTLVSequence(wire)
	if err != nil {
		t.Fatalf("DecodeTLVSequence error: %v", err)
	}
	if n != 9 {
		t.Errorf("consumed %d bytes, want 9", n)
	}
	if len(tlvs) != 2 {
		t.Fatalf("got %d TLVs, want 2", len(tlvs))
	}
	if tlvs[0].Type != 0x01 || tlvs[1].Type != 0x02 {
		t.Errorf("TLV types = (%d, %d), want (1, 2)", tlvs[0].Type, tlvs[1].Type)
	}
}

// === ROUNDTRIP / FUZZ SEEDS ===

func TestVarint_RoundtripBoundaries(t *testing.T) {
	// Test every boundary value between encoding lengths.
	boundaries := []uint64{0, 63, 64, 16383, 16384, 1073741823, 1073741824, MaxVarint}
	for _, v := range boundaries {
		encoded, err := EncodeVarint(v)
		if err != nil {
			t.Fatalf("EncodeVarint(%d) error: %v", v, err)
		}
		decoded, _, err := DecodeVarint(encoded)
		if err != nil {
			t.Fatalf("DecodeVarint(EncodeVarint(%d)) error: %v", v, err)
		}
		if decoded != v {
			t.Errorf("roundtrip(%d) = %d", v, decoded)
		}
	}
}

func TestCRC16_Deterministic(t *testing.T) {
	data := []byte("Unheaded Protocol Monad Register")
	a := CRC16CCITT(data)
	b := CRC16CCITT(data)
	if a != b {
		t.Errorf("CRC16CCITT not deterministic: 0x%04X != 0x%04X", a, b)
	}
}

// Suppress unused import warning for math.
var _ = math.MaxFloat64
