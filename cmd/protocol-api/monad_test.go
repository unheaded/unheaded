// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package main

import (
	"context"
	"encoding/hex"
	"testing"

	pb "unheaded/proto/unheaded/v1"
)

// TestComputeCRC16 tests CRC-16/CCITT-FALSE computation with known test vectors
func TestComputeCRC16(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected uint16
	}{
		{
			name:     "all zeros",
			data:     make([]byte, 18),
			expected: 0x45AB, // CRC-16/CCITT-FALSE of 18 zero bytes
		},
		{
			name: "all ones",
			data: func() []byte {
				b := make([]byte, 18)
				for i := range b {
					b[i] = 0xFF
				}
				return b
			}(),
			expected: 0x0041, // CRC-16/CCITT-FALSE of 18 0xFF bytes
		},
		{
			name:     "known test vector 1",
			data:     []byte{0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			expected: 0x99AE, // CRC-16/CCITT-FALSE of "123456789" + 9 zero bytes
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Pad to 18 bytes if needed
			data := make([]byte, 18)
			copy(data, tt.data)

			result := computeCRC16(data)
			if result != tt.expected {
				t.Errorf("computeCRC16() = %04x, want %04x", result, tt.expected)
			}
		})
	}
}

// TestVerifyCRC16 tests CRC verification with complete 20-byte Monad registers
func TestVerifyCRC16(t *testing.T) {
	// Create a valid Monad register
	data := make([]byte, 20)
	// Fill with test data
	for i := 0; i < 18; i++ {
		data[i] = byte(i)
	}

	// Compute and store CRC
	crc := computeCRC16(data)
	data[18] = byte((crc >> 8) & 0xFF)
	data[19] = byte(crc & 0xFF)

	// Verify should pass
	if !verifyCRC16(data) {
		t.Errorf("verifyCRC16() = false, want true for valid CRC")
	}

	// Corrupt the CRC
	data[18] ^= 0xFF
	if verifyCRC16(data) {
		t.Errorf("verifyCRC16() = true, want false for corrupted CRC")
	}

	// Verify with wrong length
	shortData := make([]byte, 19)
	if verifyCRC16(shortData) {
		t.Errorf("verifyCRC16() = true, want false for short data")
	}
}

// TestEncodeDecodeRoundtrip tests encoding and then decoding a Monad register
func TestEncodeDecodeRoundtrip(t *testing.T) {
	server := &monadServer{}

	encodeReq := &pb.EncodeRequest{
		FlowLabel:     0x12345,
		KingdomMode:   pb.KingdomMode_KINGDOM_MODE_ACTIVE,
		Timestamp:     1234567890,
		SophiaDictId:  42,
		WotanSequence: 999,
		Reserved1:     0,
		Reserved2:     0,
	}

	// Encode
	ctx := context.Background()
	encodeResp, err := server.Encode(ctx, encodeReq)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	if !encodeResp.Success {
		t.Fatalf("Encode() success = false")
	}

	if len(encodeResp.Data) != 20 {
		t.Fatalf("Encode() data length = %d, want 20", len(encodeResp.Data))
	}

	// Decode
	decodeReq := &pb.DecodeRequest{
		Data: encodeResp.Data,
	}

	decodeResp, err := server.Decode(ctx, decodeReq)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if !decodeResp.Valid {
		t.Fatalf("Decode() valid = false")
	}

	if !decodeResp.CrcValid {
		t.Fatalf("Decode() crc_valid = false")
	}

	// Verify all fields match (with masking for field-specific sizes)
	if decodeResp.FlowLabel != (encodeReq.FlowLabel & 0xFFFFF) {
		t.Errorf("flow_label mismatch: %d != %d", decodeResp.FlowLabel, encodeReq.FlowLabel&0xFFFFF)
	}

	if decodeResp.KingdomMode != encodeReq.KingdomMode {
		t.Errorf("kingdom_mode mismatch: %d != %d", decodeResp.KingdomMode, encodeReq.KingdomMode)
	}

	if decodeResp.Timestamp != (encodeReq.Timestamp&0xFFFFFFFFFF) {
		t.Errorf("timestamp mismatch: %d != %d", decodeResp.Timestamp, encodeReq.Timestamp&0xFFFFFFFFFF)
	}

	if decodeResp.SophiaDictId != (encodeReq.SophiaDictId & 0xFFFF) {
		t.Errorf("sophia_dict_id mismatch: %d != %d", decodeResp.SophiaDictId, encodeReq.SophiaDictId&0xFFFF)
	}
}

// TestKingdomModeEncoding tests all four Kingdom Mode states
func TestKingdomModeEncoding(t *testing.T) {
	tests := []struct {
		name string
		mode pb.KingdomMode
		k1   uint8
		k0   uint8
	}{
		{
			name: "IDLE",
			mode: pb.KingdomMode_KINGDOM_MODE_IDLE,
			k1:   0,
			k0:   0,
		},
		{
			name: "ACTIVE",
			mode: pb.KingdomMode_KINGDOM_MODE_ACTIVE,
			k1:   0,
			k0:   1,
		},
		{
			name: "CLOSING",
			mode: pb.KingdomMode_KINGDOM_MODE_CLOSING,
			k1:   1,
			k0:   0,
		},
		{
			name: "CLOSED",
			mode: pb.KingdomMode_KINGDOM_MODE_CLOSED,
			k1:   1,
			k0:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test encoding
			k1, k0 := encodeKingdomMode(tt.mode)
			if k1 != tt.k1 || k0 != tt.k0 {
				t.Errorf("encodeKingdomMode() = (%d, %d), want (%d, %d)", k1, k0, tt.k1, tt.k0)
			}

			// Test decoding
			decoded := decodeKingdomMode(k1, k0)
			if decoded != tt.mode {
				t.Errorf("decodeKingdomMode() = %d, want %d", decoded, tt.mode)
			}
		})
	}
}

// TestExponentEncoding tests exponent encoding and decoding
func TestExponentEncoding(t *testing.T) {
	tests := []struct {
		name    string
		value   uint32
		wantMin uint32
		wantMax uint32
	}{
		{
			name:    "zero",
			value:   0,
			wantMin: 0,
			wantMax: 0,
		},
		{
			name:    "small value",
			value:   42,
			wantMin: 42,
			wantMax: 42,
		},
		{
			name:    "large value",
			value:   65535,
			wantMin: 256, // After encoding and decoding, may lose precision
			wantMax: 65535,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mantissa, exponent := encodeExponent(tt.value)
			decoded := decodeExponent(mantissa, exponent)

			if decoded < tt.wantMin || decoded > tt.wantMax {
				t.Errorf("encodeExponent/decodeExponent roundtrip: %d -> (%d, %d) -> %d, want range [%d, %d]",
					tt.value, mantissa, exponent, decoded, tt.wantMin, tt.wantMax)
			}
		})
	}
}

// TestMonadDecodeInvalidCRC tests that invalid CRC is detected
func TestMonadDecodeInvalidCRC(t *testing.T) {
	server := &monadServer{}

	// Create 20 bytes with intentionally wrong CRC
	data := make([]byte, 20)
	for i := 0; i < 18; i++ {
		data[i] = byte(i)
	}
	data[18] = 0xFF // Wrong CRC
	data[19] = 0xFF

	decodeReq := &pb.DecodeRequest{
		Data: data,
	}

	decodeResp, err := server.Decode(context.Background(), decodeReq)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if decodeResp.CrcValid {
		t.Errorf("Decode() crc_valid = true, want false for invalid CRC")
	}

	if decodeResp.Valid {
		t.Errorf("Decode() valid = true, want false for invalid CRC")
	}
}

// TestMonadDecodeShortInput tests that short input is rejected
func TestMonadDecodeShortInput(t *testing.T) {
	server := &monadServer{}

	tests := []struct {
		name string
		len  int
	}{
		{"empty", 0},
		{"partial", 10},
		{"nineteen bytes", 19},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, tt.len)

			decodeReq := &pb.DecodeRequest{
				Data: data,
			}

			decodeResp, err := server.Decode(context.Background(), decodeReq)
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}

			if decodeResp.Valid {
				t.Errorf("Decode() valid = true for %d bytes, want false", tt.len)
			}

			if decodeResp.ErrorMessage == "" {
				t.Errorf("Decode() error_message empty, want error message")
			}
		})
	}
}

// TestMonadDecodeLongInput tests that oversized input is rejected
func TestMonadDecodeLongInput(t *testing.T) {
	server := &monadServer{}

	data := make([]byte, 100)

	decodeReq := &pb.DecodeRequest{
		Data: data,
	}

	decodeResp, err := server.Decode(context.Background(), decodeReq)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if decodeResp.Valid {
		t.Errorf("Decode() valid = true for 100 bytes, want false")
	}
}

// TestMonadEncodeOverflow tests that field values are properly masked
func TestMonadEncodeOverflow(t *testing.T) {
	server := &monadServer{}

	encodeReq := &pb.EncodeRequest{
		FlowLabel:     0xFFFFFFFF, // Should be masked to 20 bits
		KingdomMode:   pb.KingdomMode_KINGDOM_MODE_CLOSED,
		Timestamp:     0x9999999999, // Should be masked to 40 bits
		SophiaDictId:  0xFFFF,
		WotanSequence: 0xFFFFFFFF, // Should be masked
	}

	encodeResp, err := server.Encode(context.Background(), encodeReq)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	if !encodeResp.Success {
		t.Fatalf("Encode() success = false")
	}

	// Decode and verify
	decodeResp, err := server.Decode(context.Background(), &pb.DecodeRequest{Data: encodeResp.Data})
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	// Flow label should be masked to 20 bits (0xFFFFF)
	if decodeResp.FlowLabel > 0xFFFFF {
		t.Errorf("flow_label = %x, must be <= 0xFFFFF", decodeResp.FlowLabel)
	}

	// Timestamp should be masked to 40 bits
	if decodeResp.Timestamp > 0xFFFFFFFFFF {
		t.Errorf("timestamp = %x, must be <= 0xFFFFFFFFFF", decodeResp.Timestamp)
	}
}

// BenchmarkComputeCRC16 benchmarks CRC computation
func BenchmarkComputeCRC16(b *testing.B) {
	data := make([]byte, 18)
	for i := 0; i < 18; i++ {
		data[i] = byte(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		computeCRC16(data)
	}
}

// BenchmarkEncode benchmarks Monad encoding
func BenchmarkEncode(b *testing.B) {
	server := &monadServer{}
	req := &pb.EncodeRequest{
		FlowLabel:     0x12345,
		KingdomMode:   pb.KingdomMode_KINGDOM_MODE_ACTIVE,
		Timestamp:     1234567890,
		SophiaDictId:  42,
		WotanSequence: 999,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		server.Encode(context.Background(), req)
	}
}

// BenchmarkDecode benchmarks Monad decoding
func BenchmarkDecode(b *testing.B) {
	server := &monadServer{}
	encReq := &pb.EncodeRequest{
		FlowLabel:     0x12345,
		KingdomMode:   pb.KingdomMode_KINGDOM_MODE_ACTIVE,
		Timestamp:     1234567890,
		SophiaDictId:  42,
		WotanSequence: 999,
	}

	encResp, _ := server.Encode(context.Background(), encReq)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		server.Decode(context.Background(), &pb.DecodeRequest{Data: encResp.Data})
	}
}

// FuzzMonadDecode is a fuzz test to ensure Decode never panics
func FuzzMonadDecode(f *testing.F) {
	server := &monadServer{}

	// Add seed corpus
	f.Add([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF})

	f.Fuzz(func(t *testing.T, data []byte) {
		// Decode should never panic, regardless of input
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Decode() panicked: %v", r)
			}
		}()

		req := &pb.DecodeRequest{Data: data}
		_, err := server.Decode(context.Background(), req)

		// We don't care about the error, just that it doesn't panic
		_ = err
	})
}

// TestHexEncodingExamples provides examples of hex-encoded Monad registers
func TestHexEncodingExamples(t *testing.T) {
	examples := []struct {
		name string
		hex  string
	}{
		{
			name: "Example 1: simple idle flow",
			hex:  "0000000000000000000000000000000000001234",
		},
		{
			name: "Example 2: active flow with CRC",
			hex:  "12345600112233445566778899aabbccddeeff00",
		},
	}

	server := &monadServer{}

	for _, ex := range examples {
		t.Run(ex.name, func(t *testing.T) {
			data, err := hex.DecodeString(ex.hex)
			if err != nil {
				t.Fatalf("Invalid hex string: %v", err)
			}

			if len(data) != 20 {
				t.Logf("Hex example not 20 bytes: %d", len(data))
				return
			}

			decodeReq := &pb.DecodeRequest{Data: data}
			decodeResp, err := server.Decode(context.Background(), decodeReq)
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}

			_ = decodeResp // Use the response
		})
	}
}
