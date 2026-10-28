// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package tlv

import (
	"bytes"
	stderrors "errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	perrors "unheaded/pkg/protocol/errors"
)

var draftTLVEntry = regexp.MustCompile(`^\s+0x([0-9A-Fa-f]{2}) \(([^,]+), (optional|required), (\d+) bytes`)
var draftTLVRange = regexp.MustCompile(`^Reserved Range: 0x([0-9A-Fa-f]{2})-0x([0-9A-Fa-f]{2}) \(([^)]+)\)`)

// TestRegistryParity parses foundation-06 §"TLV Type Registry (Extended)" and
// checks the initial entries and the reserved ranges against the constants.
func TestRegistryParity(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "protocol",
		"draft-bellis-unheaded-protocol-foundation-06.md"))
	if err != nil {
		t.Fatalf("read draft: %v", err)
	}
	in := false
	entries, ranges := 0, 0
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "## TLV Type Registry") {
			in = true
			continue
		}
		if in && strings.HasPrefix(line, "## ") {
			break
		}
		if !in {
			continue
		}
		if m := draftTLVEntry.FindStringSubmatch(line); m != nil {
			v, _ := strconv.ParseUint(m[1], 16, 8)
			size, _ := strconv.Atoi(m[4])
			e, ok := Lookup(TLVType(v))
			if !ok {
				t.Errorf("registry entry 0x%02x (%s) has no constant", v, m[2])
				continue
			}
			if e.Name != m[2] || e.Length != size || e.Required != (m[3] == "required") {
				t.Errorf("0x%02x: Lookup = %+v, registry %s/%s/%d", v, e, m[2], m[3], size)
			}
			entries++
		}
		if m := draftTLVRange.FindStringSubmatch(line); m != nil {
			lo, _ := strconv.ParseUint(m[1], 16, 8)
			hi, _ := strconv.ParseUint(m[2], 16, 8)
			for v := lo; v <= hi; v++ {
				typ := TLVType(v)
				if got := typ.Range().String(); got != m[3] {
					t.Errorf("0x%02x Range() = %q, registry %q", v, got, m[3])
				}
				if typ.IsCritical() {
					t.Errorf("0x%02x reported critical", v)
				}
				if crit := typ | CriticalBit; !crit.IsCritical() || crit.Range() != typ.Range() {
					t.Errorf("0x%02x critical twin misclassified", uint8(crit))
				}
			}
			ranges++
		}
	}
	if entries != len(registered) {
		t.Errorf("parsed %d registry entries, package registers %d", entries, len(registered))
	}
	if ranges != 4 {
		t.Errorf("parsed %d reserved ranges, want 4", ranges)
	}
}

func TestRegisteredConstants(t *testing.T) {
	if RingPathCounter != 0x01 || UnheadedMetricV1 != 0x2A || CriticalBit != 0x80 {
		t.Fatalf("constants drifted: RPC=0x%02x METRIC=0x%02x C=0x%02x",
			uint8(RingPathCounter), uint8(UnheadedMetricV1), uint8(CriticalBit))
	}
}

func TestValidateRegistered(t *testing.T) {
	cases := []struct {
		name    string
		tlv     TLV
		wantErr bool
		code    perrors.Code
	}{
		{"ring path counter ok", TLV{RingPathCounter, 4, make([]byte, 4)}, false, 0},
		{"ring path counter short", TLV{RingPathCounter, 2, make([]byte, 2)}, true, perrors.UnknownCriticalTLV},
		{"metric ok", TLV{UnheadedMetricV1, 52, make([]byte, 52)}, false, 0},
		{"unknown non-critical skipped", TLV{0x33, 1, []byte{1}}, false, 0},
		{"unknown critical refused", TLV{0x85, 1, []byte{1}}, true, perrors.UnknownCriticalTLV},
		{"length mismatch", TLV{RingPathCounter, 4, make([]byte, 3)}, true, perrors.UnknownCriticalTLV},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRegistered(tc.tlv)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil {
				return
			}
			var pe *perrors.ProtocolError
			if tc.tlv.Type.IsUnknownCritical() {
				if !stderrors.As(err, &pe) || pe.Code() != tc.code {
					t.Errorf("err = %v, want code %s", err, tc.code)
				}
			}
		})
	}
}

func TestParseValidTLV(t *testing.T) {
	t.Run("SingleTLV", func(t *testing.T) {
		data := []byte{0x01, 0x03, 'f', 'o', 'o'}
		block, err := Parse(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(block) != 1 {
			t.Fatalf("expected 1 TLV, got %d", len(block))
		}
		if block[0].Type != 0x01 {
			t.Errorf("expected type 0x01, got 0x%02x", block[0].Type)
		}
		if block[0].Length != 3 {
			t.Errorf("expected length 3, got %d", block[0].Length)
		}
		if !bytes.Equal(block[0].Value, []byte("foo")) {
			t.Errorf("expected value 'foo', got %v", block[0].Value)
		}
	})

	t.Run("MultipleTLVs", func(t *testing.T) {
		data := []byte{
			0x01, 0x02, 'a', 'b',
			0x02, 0x03, 'x', 'y', 'z',
		}
		block, err := Parse(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(block) != 2 {
			t.Fatalf("expected 2 TLVs, got %d", len(block))
		}
	})

	t.Run("EmptyData", func(t *testing.T) {
		data := []byte{}
		block, err := Parse(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(block) != 0 {
			t.Fatalf("expected empty block, got %d TLVs", len(block))
		}
	})

	t.Run("ZeroLengthValue", func(t *testing.T) {
		data := []byte{0x01, 0x00}
		block, err := Parse(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(block) != 1 {
			t.Fatalf("expected 1 TLV, got %d", len(block))
		}
		if len(block[0].Value) != 0 {
			t.Errorf("expected zero-length value")
		}
	})
}

func TestParseTruncated(t *testing.T) {
	testCases := []struct {
		name string
		data []byte
	}{
		{
			name: "TruncatedHeader",
			data: []byte{0x01},
		},
		{
			name: "TruncatedValue",
			data: []byte{0x01, 0x05, 'a', 'b'},
		},
		{
			name: "TruncatedMidValue",
			data: []byte{0x01, 0x03, 'a'},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.data)
			if err == nil {
				t.Fatal("expected error for truncated data")
			}
		})
	}
}

func TestMaxTLVsPerMonad(t *testing.T) {
	t.Run("ExactMaxTLVs", func(t *testing.T) {
		// Build data with exactly MaxTLVsPerMonad TLVs
		data := []byte{}
		for i := 0; i < MaxTLVsPerMonad; i++ {
			data = append(data, byte(i), 0x01, 'a')
		}

		block, err := Parse(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(block) != MaxTLVsPerMonad {
			t.Errorf("expected %d TLVs, got %d", MaxTLVsPerMonad, len(block))
		}
	})

	t.Run("ExceedsMaxTLVs", func(t *testing.T) {
		// Build data with MaxTLVsPerMonad + 1 TLVs
		data := []byte{}
		for i := 0; i <= MaxTLVsPerMonad; i++ {
			data = append(data, byte(i), 0x01, 'a')
		}

		_, err := Parse(data)
		if err == nil {
			t.Fatal("expected error when exceeding max TLVs")
		}
	})
}

func TestSerializeValid(t *testing.T) {
	t.Run("SingleTLV", func(t *testing.T) {
		tlv := TLV{Type: 0x01, Length: 3, Value: []byte("foo")}
		block := TLVBlock{tlv}

		data, err := Serialize(block)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := []byte{0x01, 0x03, 'f', 'o', 'o'}
		if !bytes.Equal(data, expected) {
			t.Errorf("expected %v, got %v", expected, data)
		}
	})

	t.Run("MultipleTLVs", func(t *testing.T) {
		block := TLVBlock{
			{Type: 0x01, Length: 2, Value: []byte("ab")},
			{Type: 0x02, Length: 3, Value: []byte("xyz")},
		}

		data, err := Serialize(block)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := []byte{0x01, 0x02, 'a', 'b', 0x02, 0x03, 'x', 'y', 'z'}
		if !bytes.Equal(data, expected) {
			t.Errorf("expected %v, got %v", expected, data)
		}
	})

	t.Run("EmptyBlock", func(t *testing.T) {
		block := TLVBlock{}
		data, err := Serialize(block)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(data) != 0 {
			t.Errorf("expected empty data, got %v", data)
		}
	})
}

func TestSerializeInvalid(t *testing.T) {
	t.Run("TooManyTLVs", func(t *testing.T) {
		block := make(TLVBlock, MaxTLVsPerMonad+1)
		_, err := Serialize(block)
		if err == nil {
			t.Fatal("expected error for too many TLVs")
		}
	})

	t.Run("LengthMismatch", func(t *testing.T) {
		block := TLVBlock{
			{Type: 0x01, Length: 5, Value: []byte("ab")}, // Length says 5, value is 2
		}
		_, err := Serialize(block)
		if err == nil {
			t.Fatal("expected error for length mismatch")
		}
	})

	t.Run("ValueTooLong", func(t *testing.T) {
		longValue := make([]byte, 256)
		block := TLVBlock{
			{Type: 0x01, Length: 255, Value: longValue},
		}
		_, err := Serialize(block)
		if err == nil {
			t.Fatal("expected error for value too long")
		}
	})
}

func TestRoundTrip(t *testing.T) {
	originalBlock := TLVBlock{
		{Type: RingPathCounter, Length: 4, Value: []byte{0x00, 0x00, 0x00, 0x05}},
		{Type: 0x02, Length: 8, Value: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}},
		{Type: 0x03, Length: 2, Value: []byte{0x00, 0x01}},
	}

	// Serialize
	data, err := Serialize(originalBlock)
	if err != nil {
		t.Fatalf("serialize failed: %v", err)
	}

	// Parse
	parsedBlock, err := Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	// Compare
	if len(parsedBlock) != len(originalBlock) {
		t.Fatalf("block size mismatch: expected %d, got %d", len(originalBlock), len(parsedBlock))
	}

	for i, original := range originalBlock {
		parsed := parsedBlock[i]
		if parsed.Type != original.Type {
			t.Errorf("TLV %d type mismatch: expected 0x%02x, got 0x%02x", i, original.Type, parsed.Type)
		}
		if parsed.Length != original.Length {
			t.Errorf("TLV %d length mismatch: expected %d, got %d", i, original.Length, parsed.Length)
		}
		if !bytes.Equal(parsed.Value, original.Value) {
			t.Errorf("TLV %d value mismatch: expected %v, got %v", i, original.Value, parsed.Value)
		}
	}
}

func TestNewTLV(t *testing.T) {
	t.Run("ValidTLV", func(t *testing.T) {
		tlv, err := NewTLV(0x01, []byte("hello"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tlv == nil {
			t.Fatal("got nil TLV")
		}
		if tlv.Type != 0x01 {
			t.Errorf("expected type 0x01, got 0x%02x", tlv.Type)
		}
		if tlv.Length != 5 {
			t.Errorf("expected length 5, got %d", tlv.Length)
		}
		if !bytes.Equal(tlv.Value, []byte("hello")) {
			t.Errorf("value mismatch")
		}
	})

	t.Run("ValueTooLong", func(t *testing.T) {
		longValue := make([]byte, 256)
		_, err := NewTLV(0x01, longValue)
		if err == nil {
			t.Fatal("expected error for value too long")
		}
	})
}

func TestNewFoundationTLV(t *testing.T) {
	if _, err := NewFoundationTLV(RingPathCounter, []byte{0, 0, 0, 5}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := NewFoundationTLV(RingPathCounter|CriticalBit, []byte{0, 0, 0, 5}); err != nil {
		t.Fatalf("critical foundation type refused: %v", err)
	}
	if _, err := NewFoundationTLV(0x20, []byte{0x05}); err == nil {
		t.Fatal("expected error for a Sophia-range type")
	}
}

func TestFindTLV(t *testing.T) {
	block := TLVBlock{
		{Type: RingPathCounter, Length: 4, Value: []byte{0x00, 0x00, 0x00, 0x05}},
		{Type: 0x02, Length: 8, Value: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}},
		{Type: 0x03, Length: 2, Value: []byte{0x00, 0x01}},
	}

	t.Run("FindExisting", func(t *testing.T) {
		tlv := block.Find(0x02)
		if tlv == nil {
			t.Fatal("failed to find existing TLV")
		}
		if tlv.Type != 0x02 {
			t.Errorf("wrong TLV returned")
		}
	})

	t.Run("FindNonExisting", func(t *testing.T) {
		tlv := block.Find(0xff)
		if tlv != nil {
			t.Fatal("expected nil for non-existing TLV")
		}
	})
}

func TestFindAllTLV(t *testing.T) {
	block := TLVBlock{
		{Type: 0x10, Length: 1, Value: []byte{0x01}},
		{Type: 0x10, Length: 1, Value: []byte{0x02}},
		{Type: 0x20, Length: 1, Value: []byte{0x03}},
		{Type: 0x10, Length: 1, Value: []byte{0x04}},
	}

	results := block.FindAll(0x10)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	values := []byte{0x01, 0x02, 0x04}
	for i, result := range results {
		if result.Value[0] != values[i] {
			t.Errorf("result %d: expected value %d, got %d", i, values[i], result.Value[0])
		}
	}
}

func TestValidateBlock(t *testing.T) {
	t.Run("ValidBlock", func(t *testing.T) {
		block := TLVBlock{
			{Type: 0x01, Length: 3, Value: []byte("foo")},
			{Type: 0x02, Length: 2, Value: []byte("ab")},
		}
		err := ValidateBlock(block)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("TooManyTLVs", func(t *testing.T) {
		block := make(TLVBlock, MaxTLVsPerMonad+1)
		err := ValidateBlock(block)
		if err == nil {
			t.Fatal("expected error for too many TLVs")
		}
	})

	t.Run("LengthMismatch", func(t *testing.T) {
		block := TLVBlock{
			{Type: 0x01, Length: 5, Value: []byte("ab")},
		}
		err := ValidateBlock(block)
		if err == nil {
			t.Fatal("expected error for length mismatch")
		}
	})
}
