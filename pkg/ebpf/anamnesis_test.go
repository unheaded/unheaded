// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package ebpf

import (
	"encoding/binary"
	"encoding/json"
	"math/rand"
	"testing"
)

// ── MonadState tests ────────────────────────────────────────────────────────

func TestDecodeMonadState_Roundtrip(t *testing.T) {
	m := MonadState{
		Version:      0x01,
		SrcServiceID: 0x0A,
		DstServiceID: 0x0B,
		HopCount:     3,
		QosClass:     0x01,
		FlowAction:   0x01,
		CircuitState: 0x01,
		Flags:        FlagTraced | FlagSampled,
		LatencyHint:  250,
		DeployRing:   0x02,
		MeshFlags:    0x00,
		SrcPrefixLo:  0x10,
		DstPrefixLo:  0x20,
		Scratch:      [4]uint8{0xDE, 0xAD, 0xBE, 0xEF},
		Checksum:     0xABCD,
	}

	encoded := m.Encode()
	if len(encoded) != MonadSize {
		t.Fatalf("encoded size = %d, want %d", len(encoded), MonadSize)
	}

	decoded, err := DecodeMonadState(encoded[:])
	if err != nil {
		t.Fatalf("DecodeMonadState: %v", err)
	}

	if decoded != m {
		t.Errorf("roundtrip mismatch:\n  got:  %+v\n  want: %+v", decoded, m)
	}
}

func TestDecodeMonadState_TooShort(t *testing.T) {
	_, err := DecodeMonadState(make([]byte, 19))
	if err == nil {
		t.Fatal("expected error for short input")
	}
}

func TestMonadState_FieldOffsets(t *testing.T) {
	// Verify encoding matches the spec byte-for-byte
	m := MonadState{
		Version:      0x01,
		SrcServiceID: 0x0A,
		DstServiceID: 0x0B,
		HopCount:     0x03,
		QosClass:     0x01,
		FlowAction:   0x01,
		CircuitState: 0x01,
		Flags:        0x20,
		LatencyHint:  100, // 0x0064 in BE
		DeployRing:   0x01,
		MeshFlags:    0x00,
		SrcPrefixLo:  0x10,
		DstPrefixLo:  0x20,
		Scratch:      [4]uint8{0x11, 0x22, 0x33, 0x44},
		Checksum:     0xABCD,
	}
	b := m.Encode()

	tests := []struct {
		name   string
		offset int
		want   byte
	}{
		{"version", 0x00, 0x01},
		{"src_service_id", 0x01, 0x0A},
		{"dst_service_id", 0x02, 0x0B},
		{"hop_count", 0x03, 0x03},
		{"qos_class", 0x04, 0x01},
		{"flow_action", 0x05, 0x01},
		{"circuit_state", 0x06, 0x01},
		{"flags", 0x07, 0x20},
		{"latency_hint_hi", 0x08, 0x00},
		{"latency_hint_lo", 0x09, 0x64},
		{"deploy_ring", 0x0A, 0x01},
		{"mesh_flags", 0x0B, 0x00},
		{"src_prefix_lo", 0x0C, 0x10},
		{"dst_prefix_lo", 0x0D, 0x20},
		{"scratch[0]", 0x0E, 0x11},
		{"scratch[1]", 0x0F, 0x22},
		{"scratch[2]", 0x10, 0x33},
		{"scratch[3]", 0x11, 0x44},
		{"checksum_hi", 0x12, 0xAB},
		{"checksum_lo", 0x13, 0xCD},
	}

	for _, tt := range tests {
		if b[tt.offset] != tt.want {
			t.Errorf("%s: b[0x%02x] = 0x%02x, want 0x%02x", tt.name, tt.offset, b[tt.offset], tt.want)
		}
	}
}

func TestMonadState_ScratchR0R1(t *testing.T) {
	m := MonadState{
		Scratch: [4]uint8{0xBE, 0xEF, 0xDE, 0xAD},
	}
	if got := m.ScratchR0(); got != 0xBEEF {
		t.Errorf("ScratchR0() = 0x%04x, want 0xBEEF", got)
	}
	if got := m.ScratchR1(); got != 0xDEAD {
		t.Errorf("ScratchR1() = 0x%04x, want 0xDEAD", got)
	}
}

func TestMonadState_Flags(t *testing.T) {
	m := MonadState{Flags: FlagTraced | FlagChaos}
	if !m.HasFlag(FlagTraced) {
		t.Error("expected TRACED flag set")
	}
	if !m.HasFlag(FlagChaos) {
		t.Error("expected CHAOS flag set")
	}
	if m.HasFlag(FlagCanary) {
		t.Error("expected CANARY flag unset")
	}

	names := m.FlagNames()
	if len(names) != 2 {
		t.Fatalf("FlagNames() len = %d, want 2", len(names))
	}
	if names[0] != "CHAOS" || names[1] != "TRACED" {
		t.Errorf("FlagNames() = %v, want [CHAOS TRACED]", names)
	}
}

// ── CRC-16/CCITT tests ─────────────────────────────────────────────────────

func TestCRC16CCITT_StandardCheckValue(t *testing.T) {
	// Canonical test vector for CRC-16/IBM-3740 (CCITT-FALSE)
	crc := CRC16CCITT([]byte("123456789"))
	if crc != 0x29B1 {
		t.Errorf("CRC16CCITT(\"123456789\") = 0x%04x, want 0x29B1", crc)
	}
}

func TestCRC16CCITT_EmptyInput(t *testing.T) {
	crc := CRC16CCITT([]byte{})
	if crc != 0xFFFF {
		t.Errorf("CRC16CCITT(\"\") = 0x%04x, want 0xFFFF", crc)
	}
}

func TestCRC16CCITT_SingleZeroByte(t *testing.T) {
	crc := CRC16CCITT([]byte{0x00})
	if crc != 0xE1F0 {
		t.Errorf("CRC16CCITT([0x00]) = 0x%04x, want 0xE1F0", crc)
	}
}

func TestCRC16CCITT_DetectsSingleBitFlip(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06,
		0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C,
		0x0D, 0x0E, 0x0F, 0x10, 0x11, 0x12}
	original := CRC16CCITT(data)

	corrupted := make([]byte, len(data))
	copy(corrupted, data)
	corrupted[7] ^= 0x01

	if CRC16CCITT(corrupted) == original {
		t.Error("CRC should detect single-bit flip")
	}
}

func TestMonadState_VerifyChecksum(t *testing.T) {
	m := MonadState{
		Version:      MonadVersion,
		SrcServiceID: 0x42,
		DstServiceID: 0x07,
		HopCount:     3,
		QosClass:     0x01,
		FlowAction:   0x01,
		CircuitState: 0x01,
		LatencyHint:  512,
		Scratch:      [4]uint8{0xDE, 0xAD, 0x00, 0x00},
	}
	m.RecomputeChecksum()

	if !m.VerifyChecksum() {
		t.Error("freshly computed checksum must verify")
	}
	if m.Checksum == 0 {
		t.Error("checksum must not be zero")
	}

	// Corrupt and verify failure
	m.DstServiceID ^= 0x01
	if m.VerifyChecksum() {
		t.Error("checksum must fail after corruption")
	}
}

// ── EventType tests ─────────────────────────────────────────────────────────

func TestEventType_Valid(t *testing.T) {
	for _, et := range []EventType{EventBirth, EventHop, EventDeath, EventAnomaly, EventChaos} {
		if !et.Valid() {
			t.Errorf("EventType(%d).Valid() = false, want true", et)
		}
	}
	if EventType(0).Valid() {
		t.Error("EventType(0) should be invalid")
	}
	if EventType(6).Valid() {
		t.Error("EventType(6) should be invalid")
	}
}

func TestEventType_String(t *testing.T) {
	tests := []struct {
		et   EventType
		want string
	}{
		{EventBirth, "birth"},
		{EventHop, "hop"},
		{EventDeath, "death"},
		{EventAnomaly, "anomaly"},
		{EventChaos, "chaos"},
		{EventType(99), "unknown(99)"},
	}
	for _, tt := range tests {
		if got := tt.et.String(); got != tt.want {
			t.Errorf("EventType(%d).String() = %q, want %q", tt.et, got, tt.want)
		}
	}
}

func TestEventType_MarshalJSON(t *testing.T) {
	b, err := json.Marshal(EventBirth)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if string(b) != `"birth"` {
		t.Errorf("MarshalJSON = %s, want \"birth\"", b)
	}
}

// ── AnamnesisEvent tests ────────────────────────────────────────────────────

func TestDecodeAnamnesisEvent_Roundtrip(t *testing.T) {
	ev := &AnamnesisEvent{
		TimestampNs: 1_000_000_000,
		EventType:   EventBirth,
		HopID:       42,
		FlowLabelLo: 0x3F7E,
		Monad: MonadState{
			Version:      MonadVersion,
			SrcServiceID: 0x0A,
			DstServiceID: 0x0B,
			HopCount:     3,
			QosClass:     0x01,
			FlowAction:   0x01,
			CircuitState: 0x01,
			Flags:        FlagTraced,
			LatencyHint:  100,
			DeployRing:   0x01,
			Scratch:      [4]uint8{0xDE, 0xAD, 0xBE, 0xEF},
			Checksum:     0x1234,
		},
	}

	encoded := ev.Encode()
	if len(encoded) != AnamnesisEventSize {
		t.Fatalf("encoded size = %d, want %d", len(encoded), AnamnesisEventSize)
	}

	decoded, err := DecodeAnamnesisEvent(encoded[:])
	if err != nil {
		t.Fatalf("DecodeAnamnesisEvent: %v", err)
	}

	if decoded.TimestampNs != ev.TimestampNs {
		t.Errorf("TimestampNs = %d, want %d", decoded.TimestampNs, ev.TimestampNs)
	}
	if decoded.EventType != ev.EventType {
		t.Errorf("EventType = %d, want %d", decoded.EventType, ev.EventType)
	}
	if decoded.HopID != ev.HopID {
		t.Errorf("HopID = %d, want %d", decoded.HopID, ev.HopID)
	}
	if decoded.FlowLabelLo != ev.FlowLabelLo {
		t.Errorf("FlowLabelLo = 0x%04x, want 0x%04x", decoded.FlowLabelLo, ev.FlowLabelLo)
	}
	if decoded.Monad != ev.Monad {
		t.Errorf("Monad mismatch:\n  got:  %+v\n  want: %+v", decoded.Monad, ev.Monad)
	}
}

func TestDecodeAnamnesisEvent_WireLayout(t *testing.T) {
	// Construct a known 32-byte buffer and verify each field decodes correctly
	var buf [32]byte
	// timestamp_ns = 0x0102030405060708 (LE)
	binary.LittleEndian.PutUint64(buf[0:8], 0x0102030405060708)
	buf[8] = 0x01  // event_type = Birth
	buf[9] = 0x2A  // hop_id = 42
	buf[10] = 0x3F // flow_label_lo[0] (BE)
	buf[11] = 0x7E // flow_label_lo[1] (BE)
	// monad at [12..32]
	buf[12] = MonadVersion // version
	buf[13] = 0x0A         // src_service_id
	buf[14] = 0x0B         // dst_service_id
	buf[15] = 0x03         // hop_count

	ev, err := DecodeAnamnesisEvent(buf[:])
	if err != nil {
		t.Fatalf("DecodeAnamnesisEvent: %v", err)
	}

	if ev.TimestampNs != 0x0102030405060708 {
		t.Errorf("TimestampNs = 0x%x, want 0x0102030405060708", ev.TimestampNs)
	}
	if ev.EventType != EventBirth {
		t.Errorf("EventType = %d, want Birth", ev.EventType)
	}
	if ev.HopID != 42 {
		t.Errorf("HopID = %d, want 42", ev.HopID)
	}
	if ev.FlowLabelLo != 0x3F7E {
		t.Errorf("FlowLabelLo = 0x%04x, want 0x3F7E", ev.FlowLabelLo)
	}
	if ev.Monad.Version != MonadVersion {
		t.Errorf("Monad.Version = 0x%02x, want 0x%02x", ev.Monad.Version, MonadVersion)
	}
	if ev.Monad.SrcServiceID != 0x0A {
		t.Errorf("Monad.SrcServiceID = 0x%02x, want 0x0A", ev.Monad.SrcServiceID)
	}
	if ev.Monad.HopCount != 3 {
		t.Errorf("Monad.HopCount = %d, want 3", ev.Monad.HopCount)
	}
}

func TestDecodeAnamnesisEvent_TooShort(t *testing.T) {
	_, err := DecodeAnamnesisEvent(make([]byte, 31))
	if err == nil {
		t.Fatal("expected error for short input")
	}
}

func TestDecodeAnamnesisEvent_UnknownType(t *testing.T) {
	var buf [32]byte
	buf[8] = 0x00 // invalid event type
	_, err := DecodeAnamnesisEvent(buf[:])
	if err == nil {
		t.Fatal("expected error for unknown event type")
	}

	buf[8] = 0xFF // another invalid
	_, err = DecodeAnamnesisEvent(buf[:])
	if err == nil {
		t.Fatal("expected error for unknown event type 0xFF")
	}
}

func TestDecodeAnamnesisEvent_AllEventTypes(t *testing.T) {
	for _, et := range []EventType{EventBirth, EventHop, EventDeath, EventAnomaly, EventChaos} {
		var buf [32]byte
		buf[8] = uint8(et)
		ev, err := DecodeAnamnesisEvent(buf[:])
		if err != nil {
			t.Errorf("EventType %d: unexpected error: %v", et, err)
			continue
		}
		if ev.EventType != et {
			t.Errorf("EventType = %d, want %d", ev.EventType, et)
		}
	}
}

// ── Fuzz test ───────────────────────────────────────────────────────────────

func TestDecodeAnamnesisEvent_FuzzRandom(t *testing.T) {
	// Decode 10000 random 32-byte buffers; must not panic.
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 10_000; i++ {
		buf := make([]byte, 32)
		rng.Read(buf)
		// Ignore errors — we're testing it doesn't panic
		DecodeAnamnesisEvent(buf)
	}
}

func TestDecodeAnamnesisEvent_FuzzShort(t *testing.T) {
	// Decode random short buffers; must not panic.
	rng := rand.New(rand.NewSource(99))
	for i := 0; i < 1000; i++ {
		length := rng.Intn(32)
		buf := make([]byte, length)
		rng.Read(buf)
		_, err := DecodeAnamnesisEvent(buf)
		if err == nil && length < AnamnesisEventSize {
			t.Errorf("expected error for buffer length %d", length)
		}
	}
}

// ── JSON tests ──────────────────────────────────────────────────────────────

func TestAnamnesisEvent_ToJSON(t *testing.T) {
	ev := &AnamnesisEvent{
		TimestampNs: 1_000_000_000,
		EventType:   EventBirth,
		HopID:       1,
		FlowLabelLo: 0x1234,
		Monad: MonadState{
			Version:      MonadVersion,
			SrcServiceID: 0x0A,
			HopCount:     2,
			Flags:        FlagTraced,
		},
	}

	data, err := ev.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON: %v", err)
	}

	// Verify it's valid JSON
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Check event_type is a string
	if et, ok := raw["event_type"].(string); !ok || et != "birth" {
		t.Errorf("event_type = %v, want \"birth\"", raw["event_type"])
	}
}

func TestAnamnesisEvent_ToWebSocketJSON(t *testing.T) {
	ev := &AnamnesisEvent{
		TimestampNs: 500,
		EventType:   EventChaos,
		HopID:       3,
		FlowLabelLo: 0xABCD,
	}

	data, err := ev.ToWebSocketJSON()
	if err != nil {
		t.Fatalf("ToWebSocketJSON: %v", err)
	}

	var msg WebSocketMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if msg.Type != "chaos" {
		t.Errorf("Type = %q, want \"chaos\"", msg.Type)
	}
	if msg.Event == nil {
		t.Fatal("Event is nil")
	}
	if msg.Event.HopID != 3 {
		t.Errorf("Event.HopID = %d, want 3", msg.Event.HopID)
	}
}

// ── Cross-validation with Rust ──────────────────────────────────────────────

func TestMonadState_MatchesRustLayout(t *testing.T) {
	// This test verifies that a Monad encoded by Go produces byte-identical
	// output to the Rust Monad::to_bytes() for the same field values.
	//
	// Reference: monad-common Rust test `monad_bytes_layout_exact`
	m := MonadState{
		Version:      0x01,
		SrcServiceID: 0x0A,
		DstServiceID: 0x0B,
		HopCount:     0x03,
		QosClass:     0x01,
		FlowAction:   0x01,
		CircuitState: 0x01,
		Flags:        0x20,
		LatencyHint:  100, // 0x0064 BE
		DeployRing:   0x01,
		MeshFlags:    0x00,
		SrcPrefixLo:  0x10,
		DstPrefixLo:  0x20,
		Scratch:      [4]uint8{0x11, 0x22, 0x33, 0x44},
		Checksum:     0xABCD,
	}

	b := m.Encode()

	// Expected from Rust test: monad_bytes_layout_exact
	expected := [20]byte{
		0x01,       // version
		0x0A,       // src_service_id
		0x0B,       // dst_service_id
		0x03,       // hop_count
		0x01,       // qos_class
		0x01,       // flow_action
		0x01,       // circuit_state
		0x20,       // flags
		0x00, 0x64, // latency_hint (BE)
		0x01,       // deploy_ring
		0x00,       // mesh_flags
		0x10,       // src_prefix_lo
		0x20,       // dst_prefix_lo
		0x11,       // scratch[0]
		0x22,       // scratch[1]
		0x33,       // scratch[2]
		0x44,       // scratch[3]
		0xAB, 0xCD, // checksum (BE)
	}

	if b != expected {
		t.Errorf("Go encoding differs from Rust:\n  got:  %x\n  want: %x", b, expected)
	}
}

func TestCRC16CCITT_MatchesRustCrcCcitt(t *testing.T) {
	// Cross-validate with Rust test: crc_ccitt_single_zero_byte
	if crc := CRC16CCITT([]byte{0x00}); crc != 0xE1F0 {
		t.Errorf("single zero byte CRC = 0x%04x, want 0xE1F0", crc)
	}

	// Cross-validate with Rust test: crc_ccitt_standard_check_value
	if crc := CRC16CCITT([]byte("123456789")); crc != 0x29B1 {
		t.Errorf("standard check value CRC = 0x%04x, want 0x29B1", crc)
	}
}
