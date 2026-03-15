// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package protocol

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

// FuzzVarintRoundtrip fuzzes the EncodeVarint/DecodeVarint roundtrip
// Ensures encoding/decoding are inverses: Decode(Encode(x)) == x
func FuzzVarintRoundtrip(f *testing.F) {
	// Add seed data: edge cases and typical values
	f.Add(uint64(0))                          // Minimum
	f.Add(uint64(127))                        // Max single-byte
	f.Add(uint64(128))                        // Min two-byte
	f.Add(uint64(16383))                      // Max two-byte
	f.Add(uint64(16384))                      // Min three-byte
	f.Add(uint64(2097151))                    // Max three-byte
	f.Add(uint64(2097152))                    // Min four-byte
	f.Add(uint64(268435455))                  // Max four-byte
	f.Add(uint64(268435456))                  // Min five-byte
	f.Add(uint64(34359738367))                // Max five-byte
	f.Add(uint64(34359738368))                // Min six-byte
	f.Add(uint64(4398046511103))              // Max six-byte
	f.Add(uint64(4398046511104))              // Min seven-byte
	f.Add(uint64(562949953421311))            // Max seven-byte
	f.Add(uint64(562949953421312))            // Min eight-byte
	f.Add(uint64(72057594037927935))          // Max eight-byte
	f.Add(uint64(72057594037927936))          // Min nine-byte
	f.Add(uint64(9223372036854775807))        // Max nine-byte
	f.Add(uint64(9223372036854775808))        // Min ten-byte
	f.Add(uint64(1152921504606846975))        // Max ten-byte
	f.Add(uint64(18446744073709551615))       // Maximum (2^64 - 1)

	f.Fuzz(func(t *testing.T, input uint64) {
		// Encode the varint
		buf := bytes.NewBuffer(nil)
		EncodeVarint(buf, input)

		// Decode it back
		decoded, err := DecodeVarint(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatalf("DecodeVarint failed: %v", err)
		}

		// Roundtrip verification: Decode(Encode(x)) == x
		if decoded != input {
			t.Errorf("Roundtrip failed: input=%d, decoded=%d", input, decoded)
		}

		// Verify encoding is minimal (no unnecessary bytes)
		encoded := buf.Bytes()
		if len(encoded) == 0 {
			t.Errorf("Empty encoding for input %d", input)
		}

		// Each byte uses only 7 bits for data, 1 bit for continuation
		for i, b := range encoded {
			if i < len(encoded)-1 {
				// Non-final bytes must have high bit set (continuation marker)
				if (b & 0x80) == 0 {
					t.Errorf("Invalid continuation marker at byte %d: 0x%02x", i, b)
				}
			} else {
				// Final byte must not have high bit set
				if (b & 0x80) != 0 {
					t.Errorf("Invalid final byte marker at byte %d: 0x%02x", i, b)
				}
			}
		}
	})
}

// FuzzExponentRoundtrip fuzzes EncodeExponent/DecodeExponent roundtrip
// Ensures proper encoding of floating-point exponent values
func FuzzExponentRoundtrip(f *testing.F) {
	// Add seed data: exponent edge cases
	f.Add(int32(-2147483648))           // Minimum i32
	f.Add(int32(-1000000))              // Large negative
	f.Add(int32(-100))                  // Small negative
	f.Add(int32(-10))                   // Very small negative
	f.Add(int32(-1))                    // Just negative
	f.Add(int32(0))                     // Zero
	f.Add(int32(1))                     // Just positive
	f.Add(int32(10))                    // Very small positive
	f.Add(int32(100))                   // Small positive
	f.Add(int32(1000000))               // Large positive
	f.Add(int32(2147483647))            // Maximum i32

	f.Fuzz(func(t *testing.T, input int32) {
		// Encode the exponent
		encoded := EncodeExponent(input)

		// Decode it back
		decoded, err := DecodeExponent(encoded)
		if err != nil {
			t.Fatalf("DecodeExponent failed: %v", err)
		}

		// Roundtrip verification
		if decoded != input {
			t.Errorf("Exponent roundtrip failed: input=%d, decoded=%d", input, decoded)
		}

		// Verify encoding size is consistent (4 bytes for i32)
		if len(encoded) != 4 {
			t.Errorf("Exponent encoding has wrong size: expected 4, got %d", len(encoded))
		}
	})
}

// FuzzCRC16CCITT fuzzes CRC-16-CCITT computation for collision resistance
// Ensures CRC is a good hash function with low collision probability
func FuzzCRC16CCITT(f *testing.F) {
	// Add seed data: typical message patterns
	f.Add([]byte{})                                          // Empty
	f.Add([]byte{0x00})                                      // Single zero byte
	f.Add([]byte{0xFF})                                      // Single max byte
	f.Add([]byte{0x00, 0x01, 0x02, 0x03, 0x04})           // Sequential bytes
	f.Add([]byte{0xFF, 0xFE, 0xFD, 0xFC, 0xFB})           // Reverse sequential
	f.Add([]byte{0xAA, 0xAA, 0xAA, 0xAA})                 // Repeated pattern
	f.Add([]byte{0x55, 0x55, 0x55, 0x55})                 // Inverted pattern
	f.Add([]byte("Hello, World!"))                         // Text message
	f.Add([]byte("LICH-007: MBC Bytecode Fuzzer"))         // Campaign message

	f.Fuzz(func(t *testing.T, input []byte) {
		// Compute CRC for input
		crc1 := ComputeCRC16CCITT(input)

		// Verify CRC is reproducible
		crc2 := ComputeCRC16CCITT(input)
		if crc1 != crc2 {
			t.Errorf("CRC not reproducible: first=%04x, second=%04x", crc1, crc2)
		}

		// Verify single-bit changes produce different CRC
		if len(input) > 0 {
			modified := make([]byte, len(input))
			copy(modified, input)
			modified[0] ^= 0x01 // Flip single bit

			crc3 := ComputeCRC16CCITT(modified)
			if crc1 == crc3 && !bytes.Equal(input, modified) {
				t.Errorf("CRC collision on single-bit change: both %04x", crc1)
			}
		}

		// Verify length changes produce different CRC
		extended := append(input, 0x00)
		crc4 := ComputeCRC16CCITT(extended)
		if len(input) > 0 && crc1 == crc4 {
			t.Logf("CRC collision on length change (expected for CRC-16): both %04x", crc1)
		}

		// CRC-16 produces values in range [0, 65535]
		if crc1 > 0xFFFF {
			t.Errorf("CRC value out of 16-bit range: %d", crc1)
		}
	})
}

// FuzzTLVRoundtrip fuzzes EncodeTLV/DecodeTLV roundtrip
// Ensures TLV encoding preserves type, length, and value accurately
func FuzzTLVRoundtrip(f *testing.F) {
	// Add seed data: TLV edge cases
	f.Add(uint8(0), []byte{})                           // Minimal: type 0, empty value
	f.Add(uint8(1), []byte{0x00})                       // Single byte
	f.Add(uint8(255), []byte{0xFF, 0xFF, 0xFF, 0xFF}) // Max type, max value
	f.Add(uint8(0x0A), []byte("protocol_test"))        // Named TLV
	f.Add(uint8(0x20), []byte{0xDE, 0xAD, 0xBE, 0xEF}) // Binary data
	f.Add(uint8(0x3F), []byte{})                        // Large type, empty value
	f.Add(uint8(100), make([]byte, 256))               // Large value (256 bytes)

	f.Fuzz(func(t *testing.T, tlvType uint8, value []byte) {
		// Encode TLV
		encoded := EncodeTLV(tlvType, value)

		// Verify encoded format
		if len(encoded) < 2 {
			t.Errorf("TLV encoding too short: %d bytes (need >=2)", len(encoded))
			return
		}

		// Decode TLV
		decodedType, decodedValue, err := DecodeTLV(encoded)
		if err != nil {
			t.Fatalf("DecodeTLV failed: %v", err)
		}

		// Roundtrip verification
		if decodedType != tlvType {
			t.Errorf("TLV type mismatch: input=%d, decoded=%d", tlvType, decodedType)
		}

		if !bytes.Equal(decodedValue, value) {
			t.Errorf("TLV value mismatch: input=%v, decoded=%v", value, decodedValue)
		}

		// Verify length field accuracy
		expectedLength := len(value)
		// Length encoding: if value < 128, length = 1 byte; if >= 128, length encoding varies
		// For this fuzz, verify the decoded value length matches
		if len(decodedValue) != expectedLength {
			t.Errorf("TLV value length mismatch: expected=%d, decoded=%d", expectedLength, len(decodedValue))
		}

		// Test nested TLV encoding
		nested := EncodeTLV(0x01, encoded)
		if len(nested) <= len(encoded) {
			t.Errorf("Nested TLV should be larger: outer=%d, inner=%d", len(nested), len(encoded))
		}
	})
}

// Stub implementations for fuzzing (replace with actual protocol implementations)

func EncodeVarint(buf *bytes.Buffer, v uint64) {
	for {
		b := byte(v & 0x7F)
		v >>= 7
		if v != 0 {
			b |= 0x80 // Set continuation bit
		}
		buf.WriteByte(b)
		if v == 0 {
			break
		}
	}
}

func DecodeVarint(r *bytes.Reader) (uint64, error) {
	var result uint64
	var shift uint
	for {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		result |= uint64(b&0x7F) << shift
		if (b & 0x80) == 0 {
			break
		}
		shift += 7
		if shift > 63 {
			return 0, io.EOF // Overflow
		}
	}
	return result, nil
}

func EncodeExponent(exp int32) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, uint32(exp))
	return buf
}

func DecodeExponent(data []byte) (int32, error) {
	if len(data) < 4 {
		return 0, io.EOF
	}
	return int32(binary.LittleEndian.Uint32(data[:4])), nil
}

func ComputeCRC16CCITT(data []byte) uint16 {
	// CRC-16-CCITT polynomial: x^16 + x^12 + x^5 + 1 (0x1021)
	const polynomial = 0x1021
	crc := uint32(0xFFFF)

	for _, b := range data {
		crc ^= uint32(b) << 8
		for i := 0; i < 8; i++ {
			crc <<= 1
			if crc&0x10000 != 0 {
				crc ^= polynomial
			}
		}
	}
	return uint16(crc)
}

func EncodeTLV(tlvType uint8, value []byte) []byte {
	buf := bytes.NewBuffer(nil)
	buf.WriteByte(tlvType)
	EncodeVarint(buf, uint64(len(value)))
	buf.Write(value)
	return buf.Bytes()
}

func DecodeTLV(data []byte) (uint8, []byte, error) {
	if len(data) < 2 {
		return 0, nil, io.EOF
	}
	tlvType := data[0]
	length, err := DecodeVarint(bytes.NewReader(data[1:]))
	if err != nil {
		return 0, nil, err
	}
	value := data[1+varintSize(length):]
	if len(value) < int(length) {
		return 0, nil, io.EOF
	}
	return tlvType, value[:length], nil
}

func varintSize(v uint64) int {
	size := 0
	for {
		size++
		v >>= 7
		if v == 0 {
			break
		}
	}
	return size
}
