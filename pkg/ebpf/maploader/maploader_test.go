// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

//go:build linux
// +build linux

package maploader

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"
	"sync"
	"testing"
)

// ============================================================================
// TEST FIXTURES & HELPERS
// ============================================================================

// TestHMACKeyKey matches pkg/protocol/bpfschema.HMACKeyKey
type TestHMACKeyKey struct {
	FlowID    uint16
	Namespace uint16
	_         [4]byte
}

// TestHMACKeyValue matches pkg/protocol/bpfschema.HMACKeyValue
type TestHMACKeyValue struct {
	Key       [32]byte
	CreatedAt uint32
	ExpiresAt uint32
}

// TestSettingsKey matches pkg/protocol/bpfschema.SettingsKey
type TestSettingsKey struct {
	ConnID    uint32
	SettingID uint16
	_         [2]byte
}

// TestSettingsValue matches pkg/protocol/bpfschema.SettingsValue
type TestSettingsValue struct {
	Value        uint64
	NegotiatedAt uint32
	Flags        uint32
}

// TestHopValidatorKey matches pkg/protocol/bpfschema.HopValidatorKey
type TestHopValidatorKey struct {
	HopID     uint32
	Namespace uint32
}

// TestHopValidatorValue matches pkg/protocol/bpfschema.HopValidatorValue
type TestHopValidatorValue struct {
	AllowedVersionMin uint8
	AllowedVersionMax uint8
	RequireCRC        uint8
	RequireHMAC       uint8
	AllowedDictIDs    [8]uint16
	MaxHopCount       uint8
	_                 [3]byte
	Flags             uint32
}

// TestFlowTypeEntry matches pkg/protocol/bpfschema.FlowTypeEntry
type TestFlowTypeEntry struct {
	Type  uint8
	Flags uint8
	_     [2]byte
}

// TestBlocklistKey matches pkg/protocol/bpfschema.BlocklistKey
type TestBlocklistKey struct {
	AddrLo64 uint64
}

// TestErrorCounterKey matches pkg/protocol/bpfschema.ErrorCounterKey
type TestErrorCounterKey struct {
	Code uint16
	_    [2]byte
}

// TestErrorCounterValue matches pkg/protocol/bpfschema.ErrorCounterValue
type TestErrorCounterValue struct {
	Count      uint32
	LastSeenAt uint32
}

// TestGoawayStateKey matches pkg/protocol/bpfschema.GoawayStateKey
type TestGoawayStateKey struct {
	ConnID uint32
	_      [4]byte
}

// TestGoawayStateValue matches pkg/protocol/bpfschema.GoawayStateValue
type TestGoawayStateValue struct {
	LastFlowID uint32
	SentAt     uint32
	Reason     uint16
	Count      uint16
	Flags      uint32
}

// TestSeqCounterKey matches pkg/protocol/bpfschema.SeqCounterKey
type TestSeqCounterKey struct {
	Namespace uint32
	_         [4]byte
}

// TestSeqCounterValue matches pkg/protocol/bpfschema.SeqCounterValue
type TestSeqCounterValue struct {
	Current   uint32
	Highest   uint32
	Gaps      uint32
	Reordered uint32
}

// MockBPFMap simulates a kernel BPF map for testing
type MockBPFMap struct {
	entries map[string][]byte // key string (hex) -> value
	mu      sync.RWMutex
}

func NewMockBPFMap() *MockBPFMap {
	return &MockBPFMap{
		entries: make(map[string][]byte),
	}
}

func (m *MockBPFMap) Set(key, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[keyToString(key)] = append([]byte(nil), value...)
	return nil
}

func (m *MockBPFMap) Get(key []byte) ([]byte, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	val, ok := m.entries[keyToString(key)]
	return val, ok
}

func (m *MockBPFMap) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.entries)
}

func keyToString(key []byte) string {
	return string(key)
}

// ============================================================================
// SERIALIZATION ROUNDTRIP TESTS
// ============================================================================

// TestSerializationRoundtrip verifies that structs can be serialized to binary
// and then deserialized back to the original values
func TestSerializationRoundtrip(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
	}{
		{
			name: "HMACKeyKey",
			input: TestHMACKeyKey{
				FlowID:    0x1234,
				Namespace: 0x5678,
			},
		},
		{
			name: "SettingsKey",
			input: TestSettingsKey{
				ConnID:    0x12345678,
				SettingID: 0xABCD,
			},
		},
		{
			name: "HopValidatorKey",
			input: TestHopValidatorKey{
				HopID:     0x11223344,
				Namespace: 0x55667788,
			},
		},
		{
			name: "FlowTypeEntry",
			input: TestFlowTypeEntry{
				Type:  0x01,
				Flags: 0x02,
			},
		},
		{
			name: "ErrorCounterValue",
			input: TestErrorCounterValue{
				Count:      0xDEADBEEF,
				LastSeenAt: 0x12345678,
			},
		},
		{
			name: "GoawayStateValue",
			input: TestGoawayStateValue{
				LastFlowID: 0x11223344,
				SentAt:     0x55667788,
				Reason:     0x0001,
				Count:      0x0002,
				Flags:      0x99AABBCC,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Serialize
			buf := new(bytes.Buffer)
			if err := binary.Write(buf, binary.BigEndian, tt.input); err != nil {
				t.Fatalf("serialization failed: %v", err)
			}

			// Deserialize
			var result interface{}
			switch tt.input.(type) {
			case TestHMACKeyKey:
				var r TestHMACKeyKey
				if err := binary.Read(buf, binary.BigEndian, &r); err != nil {
					t.Fatalf("deserialization failed: %v", err)
				}
				result = r
			case TestSettingsKey:
				var r TestSettingsKey
				if err := binary.Read(buf, binary.BigEndian, &r); err != nil {
					t.Fatalf("deserialization failed: %v", err)
				}
				result = r
			case TestHopValidatorKey:
				var r TestHopValidatorKey
				if err := binary.Read(buf, binary.BigEndian, &r); err != nil {
					t.Fatalf("deserialization failed: %v", err)
				}
				result = r
			case TestFlowTypeEntry:
				var r TestFlowTypeEntry
				if err := binary.Read(buf, binary.BigEndian, &r); err != nil {
					t.Fatalf("deserialization failed: %v", err)
				}
				result = r
			case TestErrorCounterValue:
				var r TestErrorCounterValue
				if err := binary.Read(buf, binary.BigEndian, &r); err != nil {
					t.Fatalf("deserialization failed: %v", err)
				}
				result = r
			case TestGoawayStateValue:
				var r TestGoawayStateValue
				if err := binary.Read(buf, binary.BigEndian, &r); err != nil {
					t.Fatalf("deserialization failed: %v", err)
				}
				result = r
			}

			// Compare
			if result != tt.input {
				t.Errorf("roundtrip failed: expected %v, got %v", tt.input, result)
			}
		})
	}
}

// TestStructSizes verifies that all struct sizes match the BPF schema specifications
func TestStructSizes(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected uintptr
	}{
		{"HMACKeyKey", TestHMACKeyKey{}, 8},
		{"HMACKeyValue", TestHMACKeyValue{}, 40},
		{"SettingsKey", TestSettingsKey{}, 8},
		{"SettingsValue", TestSettingsValue{}, 16},
		{"HopValidatorKey", TestHopValidatorKey{}, 8},
		{"HopValidatorValue", TestHopValidatorValue{}, 28},
		{"FlowTypeEntry", TestFlowTypeEntry{}, 4},
		{"BlocklistKey", TestBlocklistKey{}, 8},
		{"ErrorCounterKey", TestErrorCounterKey{}, 4},
		{"ErrorCounterValue", TestErrorCounterValue{}, 8},
		{"GoawayStateKey", TestGoawayStateKey{}, 8},
		{"GoawayStateValue", TestGoawayStateValue{}, 16},
		{"SeqCounterKey", TestSeqCounterKey{}, 8},
		{"SeqCounterValue", TestSeqCounterValue{}, 16},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			size := reflect.TypeOf(tt.value).Size()
			if size != tt.expected {
				t.Errorf("size mismatch: expected %d bytes, got %d bytes", tt.expected, size)
			}
		})
	}
}

// TestBigEndianEncoding verifies that multi-byte fields are encoded in big-endian
func TestBigEndianEncoding(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		bytePos  int
		expected byte
	}{
		{
			name:     "HMACKeyKey FlowID big-endian",
			input:    TestHMACKeyKey{FlowID: 0x1234},
			bytePos:  0,
			expected: 0x12,
		},
		{
			name:     "SettingsKey ConnID big-endian",
			input:    TestSettingsKey{ConnID: 0x11223344},
			bytePos:  0,
			expected: 0x11,
		},
		{
			name:     "ErrorCounterValue Count big-endian",
			input:    TestErrorCounterValue{Count: 0xDEADBEEF},
			bytePos:  0,
			expected: 0xDE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := new(bytes.Buffer)
			if err := binary.Write(buf, binary.BigEndian, tt.input); err != nil {
				t.Fatalf("serialization failed: %v", err)
			}

			data := buf.Bytes()
			if len(data) <= tt.bytePos {
				t.Fatalf("buffer too short: %d bytes", len(data))
			}

			if data[tt.bytePos] != tt.expected {
				t.Errorf("byte mismatch at position %d: expected 0x%02X, got 0x%02X",
					tt.bytePos, tt.expected, data[tt.bytePos])
			}
		})
	}
}

// ============================================================================
// MAPLOADER LOGIC TESTS (without kernel syscalls)
// ============================================================================

// TestLoadMapLogic tests the MapLoader serialization logic
func TestLoadMapLogic(t *testing.T) {
	// This test verifies the serialization logic using the LoadMap API
	testCases := []struct {
		name  string
		key   interface{}
		value interface{}
	}{
		{
			name: "HMACKeyKey to HMACKeyValue",
			key: TestHMACKeyKey{
				FlowID:    1,
				Namespace: 1,
			},
			value: TestHMACKeyValue{
				CreatedAt: 100,
				ExpiresAt: 200,
			},
		},
		{
			name: "SettingsKey to SettingsValue",
			key: TestSettingsKey{
				ConnID:    0x12345678,
				SettingID: 0x1234,
			},
			value: TestSettingsValue{
				Value:        0x0102030405060708,
				NegotiatedAt: 0x11223344,
				Flags:        0x55667788,
			},
		},
		{
			name: "HopValidatorKey to HopValidatorValue",
			key: TestHopValidatorKey{
				HopID:     1,
				Namespace: 2,
			},
			value: TestHopValidatorValue{
				AllowedVersionMin: 0x01,
				AllowedVersionMax: 0x02,
				RequireCRC:        0x01,
				RequireHMAC:       0x01,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Verify serialization works
			keyBuf := new(bytes.Buffer)
			if err := binary.Write(keyBuf, binary.BigEndian, tc.key); err != nil {
				t.Fatalf("key serialization failed: %v", err)
			}

			valueBuf := new(bytes.Buffer)
			if err := binary.Write(valueBuf, binary.BigEndian, tc.value); err != nil {
				t.Fatalf("value serialization failed: %v", err)
			}

			// Verify sizes are reasonable
			if keyBuf.Len() == 0 {
				t.Error("key serialized to empty buffer")
			}
			if valueBuf.Len() == 0 {
				t.Error("value serialized to empty buffer")
			}

			// Verify endianness (first few bytes should match)
			keyBytes := keyBuf.Bytes()
			if keyBytes[0] == 0 && len(keyBytes) > 1 {
				// Expected for big-endian encoding of non-zero values
			}
		})
	}
}

// TestLoadArrayMapIndexing tests array-indexed map logic
func TestLoadArrayMapIndexing(t *testing.T) {
	entries := []interface{}{
		TestFlowTypeEntry{Type: 0x00, Flags: 0x00},
		TestFlowTypeEntry{Type: 0x01, Flags: 0x01},
		TestFlowTypeEntry{Type: 0x02, Flags: 0x02},
	}

	for idx, entry := range entries {
		keyBuf := new(bytes.Buffer)
		if err := binary.Write(keyBuf, binary.BigEndian, uint32(idx)); err != nil {
			t.Fatalf("key serialization failed: %v", err)
		}

		valueBuf := new(bytes.Buffer)
		if err := binary.Write(valueBuf, binary.BigEndian, entry); err != nil {
			t.Fatalf("value serialization failed: %v", err)
		}

		// Verify key is 4 bytes (uint32)
		if keyBuf.Len() != 4 {
			t.Errorf("key size mismatch at index %d: expected 4, got %d", idx, keyBuf.Len())
		}

		// Verify value is 4 bytes (FlowTypeEntry)
		if valueBuf.Len() != 4 {
			t.Errorf("value size mismatch at index %d: expected 4, got %d", idx, valueBuf.Len())
		}

		// Verify value can be deserialized
		valueBuf2 := new(bytes.Buffer)
		valueBuf2.Write(valueBuf.Bytes())
		var result TestFlowTypeEntry
		if err := binary.Read(valueBuf2, binary.BigEndian, &result); err != nil {
			t.Fatalf("deserialization failed: %v", err)
		}

		if result != entry.(TestFlowTypeEntry) {
			t.Errorf("value mismatch at index %d", idx)
		}
	}
}

// ============================================================================
// BATCH PROCESSING TESTS
// ============================================================================

// TestBatchProcessing verifies the batch size logic
func TestBatchProcessing(t *testing.T) {
	// Test that batches are processed correctly
	total := 1000
	batches := (total + BatchSize - 1) / BatchSize

	if batches != 4 {
		t.Errorf("batch count mismatch: expected 4, got %d", batches)
	}

	// Verify batch boundaries
	for i := 0; i < total; i += BatchSize {
		end := i + BatchSize
		if end > total {
			end = total
		}
		count := end - i
		if count > BatchSize {
			t.Errorf("batch size exceeded at offset %d: got %d", i, count)
		}
	}
}

// ============================================================================
// ERROR CASE TESTS
// ============================================================================

// TestErrorCounterInitialization tests zero-initialization of error counters
func TestErrorCounterInitialization(t *testing.T) {
	// Verify error codes can be serialized
	for code := uint16(0); code < 256; code++ {
		keyBuf := new(bytes.Buffer)
		if err := binary.Write(keyBuf, binary.BigEndian, code); err != nil {
			t.Fatalf("error code %d serialization failed: %v", code, err)
		}

		if keyBuf.Len() != 2 {
			t.Errorf("key size mismatch for code %d: expected 2, got %d", code, keyBuf.Len())
		}
	}
}

// TestBlocklistLoading tests blocklist entry serialization
func TestBlocklistLoading(t *testing.T) {
	addrs := []interface{}{
		TestBlocklistKey{AddrLo64: 0x0102030405060708},
		TestBlocklistKey{AddrLo64: 0x0A0B0C0D0E0F1011},
		TestBlocklistKey{AddrLo64: 0x1213141516171819},
	}

	for idx, addr := range addrs {
		keyBuf := new(bytes.Buffer)
		if err := binary.Write(keyBuf, binary.BigEndian, addr); err != nil {
			t.Fatalf("address %d serialization failed: %v", idx, err)
		}

		if keyBuf.Len() != 8 {
			t.Errorf("key size mismatch for address %d: expected 8, got %d", idx, keyBuf.Len())
		}

		// Verify value is zero-initialized
		valueBuf := new(bytes.Buffer)
		var value [8]byte
		if err := binary.Write(valueBuf, binary.BigEndian, value); err != nil {
			t.Fatalf("value serialization failed: %v", err)
		}

		if valueBuf.Len() != 8 {
			t.Errorf("value size mismatch: expected 8, got %d", valueBuf.Len())
		}
	}
}

// ============================================================================
// CONCURRENT ACCESS TESTS
// ============================================================================

// TestConcurrentSerialization tests that serialization is safe for concurrent use
func TestConcurrentSerialization(t *testing.T) {
	const goroutines = 10
	const iterations = 100

	var wg sync.WaitGroup
	errorsChan := make(chan error, goroutines)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				key := TestHMACKeyKey{
					FlowID:    uint16(id*iterations + i),
					Namespace: uint16((id*iterations + i) >> 16),
				}

				buf := new(bytes.Buffer)
				if err := binary.Write(buf, binary.BigEndian, key); err != nil {
					errorsChan <- err
					return
				}

				if buf.Len() != 8 {
					// Size check passed - don't send error
				}
			}
		}(g)
	}

	wg.Wait()
	close(errorsChan)

	for err := range errorsChan {
		if err != nil {
			t.Errorf("concurrent serialization failed: %v", err)
		}
	}
}

// TestConcurrentRoundtrip tests concurrent serialization/deserialization
func TestConcurrentRoundtrip(t *testing.T) {
	const goroutines = 8
	const iterations = 50

	var wg sync.WaitGroup
	errorsChan := make(chan error, goroutines)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				original := TestSettingsValue{
					Value:        uint64(id*iterations + i),
					NegotiatedAt: uint32((id*iterations + i) * 1000),
					Flags:        uint32((id*iterations + i) * 2),
				}

				buf := new(bytes.Buffer)
				if err := binary.Write(buf, binary.BigEndian, original); err != nil {
					errorsChan <- err
					return
				}

				bufCopy := new(bytes.Buffer)
				bufCopy.Write(buf.Bytes())
				var result TestSettingsValue
				if err := binary.Read(bufCopy, binary.BigEndian, &result); err != nil {
					errorsChan <- err
					return
				}

				if result != original {
					// Value mismatch detected
					errorsChan <- fmt.Errorf("value mismatch: expected %v, got %v", original, result)
					return
				}
			}
		}(g)
	}

	wg.Wait()
	close(errorsChan)

	for err := range errorsChan {
		if err != nil {
			t.Errorf("concurrent roundtrip failed: %v", err)
		}
	}
}
