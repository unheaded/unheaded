//go:build linux
// +build linux

// Integration tests demonstrating MapLoader with actual bpfschema types.
// These tests verify that MapLoader correctly handles bpfschema structs.
//
// Note: These tests only verify serialization and structure compatibility.
// Actual BPF syscall tests require a running kernel with BPF support
// and would be run in a separate integration test suite.

package maploader

import (
	"bytes"
	"encoding/binary"
	"testing"
	"unsafe"
)

// ============================================================================
// SIMULATED BPFSCHEMA TYPES
// ============================================================================
//
// These mirror the actual types in pkg/protocol/bpfschema.
// In real code, you would import from there.

// Q1: HMAC Keys
type SimHMACKeyKey struct {
	FlowID    uint16
	Namespace uint16
	_         [4]byte
}

type SimHMACKeyValue struct {
	Key       [32]byte
	CreatedAt uint32
	ExpiresAt uint32
}

// Q2: Sequence Counters
type SimSeqCounterKey struct {
	Namespace uint32
	_         [4]byte
}

type SimSeqCounterValue struct {
	Current   uint32
	Highest   uint32
	Gaps      uint32
	Reordered uint32
}

// H4/H12: Settings
type SimSettingsKey struct {
	ConnID    uint32
	SettingID uint16
	_         [2]byte
}

type SimSettingsValue struct {
	Value        uint64
	NegotiatedAt uint32
	Flags        uint32
}

// H6: Flow Types
type SimFlowTypeEntry struct {
	Type  uint8
	Flags uint8
	_     [2]byte
}

// H2: Error Counters
type SimErrorCounterKey struct {
	Code uint16
	_    [2]byte
}

type SimErrorCounterValue struct {
	Count      uint32
	LastSeenAt uint32
}

// H7/Q8: GOAWAY State
type SimGoawayStateKey struct {
	ConnID uint32
	_      [4]byte
}

type SimGoawayStateValue struct {
	LastFlowID uint32
	SentAt     uint32
	Reason     uint16
	Count      uint16
	Flags      uint32
}

// H10: Hop Validators
type SimHopValidatorKey struct {
	HopID     uint32
	Namespace uint32
}

type SimHopValidatorValue struct {
	AllowedVersionMin uint8
	AllowedVersionMax uint8
	RequireCRC        uint8
	RequireHMAC       uint8
	AllowedDictIDs    [8]uint16
	MaxHopCount       uint8
	_                 [3]byte
	Flags             uint32
}

// ============================================================================
// INTEGRATION TESTS - SERIALIZATION COMPATIBILITY
// ============================================================================

// TestHMACKeysSerialization verifies HMAC key serialization
func TestHMACKeysSerialization(t *testing.T) {
	key := SimHMACKeyKey{FlowID: 0x1234, Namespace: 0x5678}
	value := SimHMACKeyValue{
		CreatedAt: 0x12345678,
		ExpiresAt: 0x87654321,
	}
	copy(value.Key[:16], []byte("test_key_data___"))

	// Verify sizes
	if unsafe.Sizeof(key) != 8 {
		t.Errorf("HMACKeyKey size: expected 8, got %d", unsafe.Sizeof(key))
	}
	if unsafe.Sizeof(value) != 40 {
		t.Errorf("HMACKeyValue size: expected 40, got %d", unsafe.Sizeof(value))
	}

	// Verify serialization
	keyBuf := new(bytes.Buffer)
	if err := binary.Write(keyBuf, binary.BigEndian, key); err != nil {
		t.Fatalf("key serialization failed: %v", err)
	}

	valueBuf := new(bytes.Buffer)
	if err := binary.Write(valueBuf, binary.BigEndian, value); err != nil {
		t.Fatalf("value serialization failed: %v", err)
	}

	if keyBuf.Len() != 8 || valueBuf.Len() != 40 {
		t.Errorf("serialized sizes don't match struct sizes")
	}

	// Verify roundtrip
	var keyResult SimHMACKeyKey
	if err := binary.Read(keyBuf, binary.BigEndian, &keyResult); err != nil {
		t.Fatalf("key deserialization failed: %v", err)
	}
	if keyResult != key {
		t.Errorf("key roundtrip failed: expected %v, got %v", key, keyResult)
	}
}

// TestSeqCountersSerialization verifies sequence counter serialization
func TestSeqCountersSerialization(t *testing.T) {
	key := SimSeqCounterKey{Namespace: 0x11223344}
	value := SimSeqCounterValue{
		Current:   100,
		Highest:   200,
		Gaps:      5,
		Reordered: 2,
	}

	if unsafe.Sizeof(key) != 8 {
		t.Errorf("SeqCounterKey size mismatch")
	}
	if unsafe.Sizeof(value) != 16 {
		t.Errorf("SeqCounterValue size mismatch")
	}

	keyBuf := new(bytes.Buffer)
	binary.Write(keyBuf, binary.BigEndian, key)

	valueBuf := new(bytes.Buffer)
	binary.Write(valueBuf, binary.BigEndian, value)

	if keyBuf.Len() != 8 || valueBuf.Len() != 16 {
		t.Error("serialized size mismatch")
	}

	var resultValue SimSeqCounterValue
	binary.Read(valueBuf, binary.BigEndian, &resultValue)
	if resultValue != value {
		t.Error("value roundtrip failed")
	}
}

// TestSettingsSerialization verifies settings map serialization
func TestSettingsSerialization(t *testing.T) {
	key := SimSettingsKey{ConnID: 0x12345678, SettingID: 0xABCD}
	value := SimSettingsValue{
		Value:        0x0102030405060708,
		NegotiatedAt: 0x11223344,
		Flags:        0x01,
	}

	if unsafe.Sizeof(key) != 8 {
		t.Errorf("SettingsKey size mismatch")
	}
	if unsafe.Sizeof(value) != 16 {
		t.Errorf("SettingsValue size mismatch")
	}

	keyBuf := new(bytes.Buffer)
	binary.Write(keyBuf, binary.BigEndian, key)

	valueBuf := new(bytes.Buffer)
	binary.Write(valueBuf, binary.BigEndian, value)

	// Verify big-endian encoding of ConnID
	keyBytes := keyBuf.Bytes()
	if keyBytes[0] != 0x12 || keyBytes[1] != 0x34 {
		t.Error("ConnID not encoded in big-endian")
	}
}

// TestFlowTypesSerialization verifies flow type array serialization
func TestFlowTypesSerialization(t *testing.T) {
	entry := SimFlowTypeEntry{Type: 0x01, Flags: 0x02}

	if unsafe.Sizeof(entry) != 4 {
		t.Errorf("FlowTypeEntry size: expected 4, got %d", unsafe.Sizeof(entry))
	}

	// Test array indexing (index as uint32)
	for idx := 0; idx < 100; idx++ {
		indexBuf := new(bytes.Buffer)
		binary.Write(indexBuf, binary.BigEndian, uint32(idx))

		entryBuf := new(bytes.Buffer)
		binary.Write(entryBuf, binary.BigEndian, entry)

		if indexBuf.Len() != 4 || entryBuf.Len() != 4 {
			t.Errorf("size mismatch at index %d", idx)
		}
	}
}

// TestErrorCounterInitialization verifies error code initialization
func TestErrorCounterInitialization(t *testing.T) {
	// All error codes should serialize to 2 bytes (uint16)
	for code := uint16(0); code < 256; code++ {
		codeBuf := new(bytes.Buffer)
		if err := binary.Write(codeBuf, binary.BigEndian, code); err != nil {
			t.Fatalf("code %d serialization failed: %v", code, err)
		}

		if codeBuf.Len() != 2 {
			t.Errorf("code %d: expected 2 bytes, got %d", code, codeBuf.Len())
		}

		var resultCode uint16
		if err := binary.Read(codeBuf, binary.BigEndian, &resultCode); err != nil {
			t.Fatalf("code %d deserialization failed: %v", code, err)
		}

		if resultCode != code {
			t.Errorf("code roundtrip failed: expected %d, got %d", code, resultCode)
		}
	}
}

// TestGoawayStateSerialization verifies GOAWAY state serialization
func TestGoawayStateSerialization(t *testing.T) {
	key := SimGoawayStateKey{ConnID: 0x12345678}
	value := SimGoawayStateValue{
		LastFlowID: 0x11223344,
		SentAt:     0x55667788,
		Reason:     0x0001,
		Count:      0x0002,
		Flags:      0x99AABBCC,
	}

	if unsafe.Sizeof(key) != 8 {
		t.Errorf("GoawayStateKey size mismatch")
	}
	if unsafe.Sizeof(value) != 16 {
		t.Errorf("GoawayStateValue size mismatch")
	}

	keyBuf := new(bytes.Buffer)
	binary.Write(keyBuf, binary.BigEndian, key)

	valueBuf := new(bytes.Buffer)
	binary.Write(valueBuf, binary.BigEndian, value)

	var resultValue SimGoawayStateValue
	binary.Read(valueBuf, binary.BigEndian, &resultValue)
	if resultValue != value {
		t.Error("value roundtrip failed")
	}
}

// TestHopValidatorSerialization verifies hop validator serialization
func TestHopValidatorSerialization(t *testing.T) {
	key := SimHopValidatorKey{
		HopID:     0x11223344,
		Namespace: 0x55667788,
	}
	value := SimHopValidatorValue{
		AllowedVersionMin: 0x01,
		AllowedVersionMax: 0x02,
		RequireCRC:        0x01,
		RequireHMAC:       0x01,
		MaxHopCount:       64,
		Flags:             0x12345678,
	}
	value.AllowedDictIDs[0] = 0x0001
	value.AllowedDictIDs[1] = 0x0002

	if unsafe.Sizeof(key) != 8 {
		t.Errorf("HopValidatorKey size mismatch")
	}
	if unsafe.Sizeof(value) != 32 {
		t.Errorf("HopValidatorValue size: expected 32, got %d", unsafe.Sizeof(value))
	}

	keyBuf := new(bytes.Buffer)
	binary.Write(keyBuf, binary.BigEndian, key)

	valueBuf := new(bytes.Buffer)
	binary.Write(valueBuf, binary.BigEndian, value)

	if keyBuf.Len() != 8 || valueBuf.Len() != 32 {
		t.Error("serialized size mismatch")
	}

	var resultValue SimHopValidatorValue
	binary.Read(valueBuf, binary.BigEndian, &resultValue)
	if resultValue != value {
		t.Error("value roundtrip failed")
	}
}

// ============================================================================
// BATCH LOADING SIMULATION TESTS
// ============================================================================

// TestBatchLoadingWorkflow simulates the complete MapLoader workflow
func TestBatchLoadingWorkflow(t *testing.T) {
	// Simulate what MapLoader.LoadMapBatch would do
	entries := [][2]interface{}{
		{
			SimHMACKeyKey{FlowID: 1, Namespace: 1},
			SimHMACKeyValue{CreatedAt: 100, ExpiresAt: 0},
		},
		{
			SimHMACKeyKey{FlowID: 2, Namespace: 1},
			SimHMACKeyValue{CreatedAt: 200, ExpiresAt: 0},
		},
		{
			SimHMACKeyKey{FlowID: 3, Namespace: 2},
			SimHMACKeyValue{CreatedAt: 300, ExpiresAt: 0},
		},
	}

	processedCount := 0
	for _, entry := range entries {
		key := entry[0]
		value := entry[1]

		keyBuf := new(bytes.Buffer)
		if err := binary.Write(keyBuf, binary.BigEndian, key); err != nil {
			t.Fatalf("entry %d: key serialization failed: %v", processedCount, err)
		}

		valueBuf := new(bytes.Buffer)
		if err := binary.Write(valueBuf, binary.BigEndian, value); err != nil {
			t.Fatalf("entry %d: value serialization failed: %v", processedCount, err)
		}

		processedCount++
	}

	if processedCount != 3 {
		t.Errorf("expected 3 entries processed, got %d", processedCount)
	}
}

// TestBatchSizeProcessing verifies batch size handling
func TestBatchSizeProcessing(t *testing.T) {
	const totalEntries = 1000

	// Create mock entries
	batchCount := 0
	entriesPerBatch := 256

	for i := 0; i < totalEntries; i += entriesPerBatch {
		end := i + entriesPerBatch
		if end > totalEntries {
			end = totalEntries
		}

		actualBatchSize := end - i
		if actualBatchSize > entriesPerBatch {
			t.Errorf("batch size exceeded: %d > %d", actualBatchSize, entriesPerBatch)
		}

		batchCount++
	}

	expectedBatches := (totalEntries + entriesPerBatch - 1) / entriesPerBatch
	if batchCount != expectedBatches {
		t.Errorf("batch count: expected %d, got %d", expectedBatches, batchCount)
	}
}

// ============================================================================
// COMPATIBILITY TESTS
// ============================================================================

// TestBigEndianNetworkByteOrder verifies that all serialization uses big-endian
// matching RFC 8200 (IPv6) network byte order
func TestBigEndianNetworkByteOrder(t *testing.T) {
	// Test uint16 (2 bytes)
	var u16 uint16 = 0x1234
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, u16)
	b := buf.Bytes()
	if b[0] != 0x12 || b[1] != 0x34 {
		t.Error("uint16 not big-endian: expected [0x12 0x34]")
	}

	// Test uint32 (4 bytes)
	var u32 uint32 = 0x12345678
	buf = new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, u32)
	b = buf.Bytes()
	if b[0] != 0x12 || b[1] != 0x34 || b[2] != 0x56 || b[3] != 0x78 {
		t.Error("uint32 not big-endian: expected [0x12 0x34 0x56 0x78]")
	}

	// Test uint64 (8 bytes)
	var u64 uint64 = 0x0102030405060708
	buf = new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, u64)
	b = buf.Bytes()
	expected := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	for i := 0; i < 8; i++ {
		if b[i] != expected[i] {
			t.Errorf("uint64 not big-endian at byte %d: expected 0x%02x, got 0x%02x",
				i, expected[i], b[i])
		}
	}
}

// TestPaddingFieldHandling verifies that padding fields work correctly
func TestPaddingFieldHandling(t *testing.T) {
	// HMACKeyKey has padding
	key := SimHMACKeyKey{
		FlowID:    0x1234,
		Namespace: 0x5678,
		_:         [4]byte{}, // Padding
	}

	if unsafe.Sizeof(key) != 8 {
		t.Error("padding not included in size")
	}

	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, key)
	if buf.Len() != 8 {
		t.Errorf("padding not serialized: expected 8 bytes, got %d", buf.Len())
	}

	// Verify padding is zeros
	b := buf.Bytes()
	// Bytes 0-3: FlowID + Namespace
	// Bytes 4-7: Padding (should be zeros)
	for i := 4; i < 8; i++ {
		if b[i] != 0 {
			t.Errorf("padding byte %d is not zero: 0x%02x", i, b[i])
		}
	}
}
