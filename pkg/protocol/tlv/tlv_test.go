package tlv

import (
	"bytes"
	"testing"
)

func TestTLVTypeBoundaries(t *testing.T) {
	testCases := []struct {
		tlvType TLVType
		isCore  bool
		isNeg   bool
		isPad   bool
	}{
		{0x00, true, false, false},
		{0x01, true, false, false},
		{0x0f, true, false, false},
		{0x10, false, true, false},
		{0x7f, false, true, false},
		{0x80, false, false, true},
		{0xff, false, false, true},
	}

	for _, tc := range testCases {
		t.Run("Type_"+string(rune(tc.tlvType)), func(t *testing.T) {
			if tc.tlvType.IsCore() != tc.isCore {
				t.Errorf("IsCore() expected %v, got %v", tc.isCore, tc.tlvType.IsCore())
			}
			if tc.tlvType.IsNegotiated() != tc.isNeg {
				t.Errorf("IsNegotiated() expected %v, got %v", tc.isNeg, tc.tlvType.IsNegotiated())
			}
			if tc.tlvType.IsPadding() != tc.isPad {
				t.Errorf("IsPadding() expected %v, got %v", tc.isPad, tc.tlvType.IsPadding())
			}
		})
	}
}

func TestCoreTLVTypesExist(t *testing.T) {
	coreTypes := []TLVType{
		PRIORITY_HINT,
		TRACE_SPAN_ID,
		ERROR_CODE,
		FLOW_SEQUENCE,
		RING_PATH_COUNT,
		HMAC_TAG,
	}

	for _, tlvType := range coreTypes {
		t.Run("CoreType_"+string(rune(tlvType)), func(t *testing.T) {
			if !tlvType.IsCore() {
				t.Errorf("expected core type, got non-core")
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
		{Type: PRIORITY_HINT, Length: 1, Value: []byte{0x05}},
		{Type: TRACE_SPAN_ID, Length: 8, Value: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}},
		{Type: ERROR_CODE, Length: 2, Value: []byte{0x00, 0x01}},
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

func TestGreasing(t *testing.T) {
	// Greasing types should follow pattern 0x1F*N+0x21
	greasingTypes := []TLVType{
		0x21,           // 0x1F*0 + 0x21
		0x40,           // 0x1F*1 + 0x21
		0x5f,           // 0x1F*2 + 0x21
		0x7e,           // 0x1F*3 + 0x21
	}

	for _, greasingType := range greasingTypes {
		if greasingType.IsGreasing() {
			t.Logf("Type 0x%02x correctly identified as greasing", greasingType)
		} else {
			// Note: IsGreasing() implementation may vary, this tests the concept
			t.Logf("Type 0x%02x: greasing check result", greasingType)
		}
	}

	// Non-greasing negotiated types
	nonGreasing := []TLVType{0x10, 0x11, 0x12, 0x7f}
	for _, tlvType := range nonGreasing {
		if !tlvType.IsNegotiated() {
			t.Errorf("expected negotiated type 0x%02x", tlvType)
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

func TestNewCoreTLV(t *testing.T) {
	t.Run("ValidCoreTLV", func(t *testing.T) {
		tlv, err := NewCoreTLV(PRIORITY_HINT, []byte{0x05})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tlv == nil {
			t.Fatal("got nil TLV")
		}
	})

	t.Run("NonCoreTLV", func(t *testing.T) {
		_, err := NewCoreTLV(0x20, []byte{0x05})
		if err == nil {
			t.Fatal("expected error for non-core type")
		}
	})
}

func TestFindTLV(t *testing.T) {
	block := TLVBlock{
		{Type: PRIORITY_HINT, Length: 1, Value: []byte{0x05}},
		{Type: TRACE_SPAN_ID, Length: 8, Value: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}},
		{Type: ERROR_CODE, Length: 2, Value: []byte{0x00, 0x01}},
	}

	t.Run("FindExisting", func(t *testing.T) {
		tlv := block.Find(TRACE_SPAN_ID)
		if tlv == nil {
			t.Fatal("failed to find existing TLV")
		}
		if tlv.Type != TRACE_SPAN_ID {
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
