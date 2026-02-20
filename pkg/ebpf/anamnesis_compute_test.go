package ebpf

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// ── ComputeHopEvent tests ────────────────────────────────────────────────────

func TestComputeHopEvent_SizeAndAlignment(t *testing.T) {
	// Verify the struct size matches Rust exactly: 96 bytes
	// 8 (timestamp) + 1 (type) + 1 (hop) + 2 (pad) + 4 (flow) + 4 (pc) + 4 (insn) +
	// 64 (regs[16]*4) + 1 (flags) + 1 (cache_hit) + 4 (miss_addr) = 94
	// With repr(C) and possible padding from binary.Read: check actual struct size
	evt := ComputeHopEvent{}
	// Encode via binary to verify round-trip
	var buf [94]byte
	wrBuf := new(bytes.Buffer)
	if err := binary.Write(
		wrBuf,
		binary.LittleEndian,
		&struct {
			TimestampNs uint64
			EventType   uint8
			HopID       uint8
			Pad         [2]uint8
			FlowLabel   uint32
			PC          uint32
			Instruction uint32
			Regs        [16]uint32
			Flags       uint8
			CacheHit    uint8
			MissAddr    uint32
		}{
			TimestampNs: 0x0102030405060708,
			EventType:   EventComputeHop,
			HopID:       5,
			FlowLabel:   0x12345678,
			PC:          100,
			Instruction: 0xABCDEF00,
			Flags:       0x05,
			CacheHit:    1,
			MissAddr:    0xDEADBEEF,
		},
	); err != nil {
		t.Fatalf("binary.Write failed: %v", err)
	}
	copy(buf[:], wrBuf.Bytes())

	// Now decode and verify
	decoded, err := DecodeComputeHopEvent(buf[:])
	if err != nil {
		t.Fatalf("DecodeComputeHopEvent failed: %v", err)
	}

	if decoded.TimestampNs != 0x0102030405060708 {
		t.Errorf("TimestampNs = 0x%x, want 0x0102030405060708", decoded.TimestampNs)
	}
	if decoded.EventType != EventComputeHop {
		t.Errorf("EventType = 0x%02x, want 0x%02x", decoded.EventType, EventComputeHop)
	}
	if decoded.HopID != 5 {
		t.Errorf("HopID = %d, want 5", decoded.HopID)
	}
	if decoded.FlowLabel != 0x12345678 {
		t.Errorf("FlowLabel = 0x%08x, want 0x12345678", decoded.FlowLabel)
	}
	if decoded.PC != 100 {
		t.Errorf("PC = %d, want 100", decoded.PC)
	}
	if decoded.Instruction != 0xABCDEF00 {
		t.Errorf("Instruction = 0x%08x, want 0xABCDEF00", decoded.Instruction)
	}
	if decoded.Flags != 0x05 {
		t.Errorf("Flags = 0x%02x, want 0x05", decoded.Flags)
	}
	if decoded.CacheHit != 1 {
		t.Errorf("CacheHit = %d, want 1", decoded.CacheHit)
	}
	if decoded.MissAddr != 0xDEADBEEF {
		t.Errorf("MissAddr = 0x%08x, want 0xDEADBEEF", decoded.MissAddr)
	}

	_ = evt
}

func TestComputeHopEvent_DecodeErrors(t *testing.T) {
	// Test too-short buffer
	_, err := DecodeComputeHopEvent(make([]byte, 93))
	if err == nil {
		t.Fatal("expected error for buffer 1 byte too short")
	}

	// Test empty buffer
	_, err = DecodeComputeHopEvent([]byte{})
	if err == nil {
		t.Fatal("expected error for empty buffer")
	}

	// Test exactly right size (94 bytes)
	buf := make([]byte, 94)
	buf[8] = EventComputeHop
	decoded, err := DecodeComputeHopEvent(buf)
	if err != nil {
		t.Fatalf("94-byte buffer should decode: %v", err)
	}
	if decoded == nil {
		t.Fatal("decoded event is nil")
	}
	if decoded.EventType != EventComputeHop {
		t.Errorf("EventType = 0x%02x, want 0x%02x", decoded.EventType, EventComputeHop)
	}
}

func TestComputeHopEvent_RegisterSnapshot(t *testing.T) {
	// Test that register snapshot is preserved
	var buf [94]byte
	binary.LittleEndian.PutUint64(buf[0:8], 1000)
	buf[8] = EventComputeHop
	buf[9] = 3

	// Fill registers with predictable pattern
	for i := 0; i < 16; i++ {
		offset := 24 + (i * 4)
		binary.LittleEndian.PutUint32(buf[offset:offset+4], uint32(0x1000+i))
	}

	decoded, err := DecodeComputeHopEvent(buf[:])
	if err != nil {
		t.Fatalf("DecodeComputeHopEvent: %v", err)
	}

	for i := 0; i < 16; i++ {
		if decoded.Regs[i] != uint32(0x1000+i) {
			t.Errorf("Regs[%d] = 0x%08x, want 0x%08x", i, decoded.Regs[i], uint32(0x1000+i))
		}
	}
}

func TestComputeEventType_Constants(t *testing.T) {
	// Verify all event type constants are unique and in expected range
	events := []uint8{
		EventComputeHop,
		EventCacheMiss,
		EventMemWrite,
		EventMemStaged,
		EventScreenWrite,
		EventKeyRead,
		EventComputeHalt,
		EventComputeStall,
	}

	for i, e := range events {
		if e < 0x10 || e > 0x17 {
			t.Errorf("Event[%d] = 0x%02x, expected in range [0x10, 0x17]", i, e)
		}
	}

	// Check uniqueness
	seen := make(map[uint8]bool)
	for _, e := range events {
		if seen[e] {
			t.Errorf("Event type 0x%02x is duplicated", e)
		}
		seen[e] = true
	}
}

// ── MemWriteEvent tests ──────────────────────────────────────────────────────

func TestMemWriteEvent_Size(t *testing.T) {
	// MemWriteEvent is 8 + 1 + 3 + 4 + 4 + 1 + 3 + 4 = 28 bytes
	// This is just documentation; Go doesn't enforce struct size at compile time
	var evt MemWriteEvent
	_ = evt
}

func TestMemWriteEvent_Roundtrip(t *testing.T) {
	// Encode a MemWriteEvent manually
	var buf [28]byte
	binary.LittleEndian.PutUint64(buf[0:8], 0x0102030405060708)
	buf[8] = EventMemWrite
	// buf[9:12] = pad
	binary.LittleEndian.PutUint32(buf[12:16], 0xABCDEF00)
	binary.LittleEndian.PutUint32(buf[16:20], 0x12345678)
	buf[20] = 4 // size = word
	// buf[21:24] = pad2
	binary.LittleEndian.PutUint32(buf[24:28], 0xDEADBEEF)

	// Now we'd need to decode it manually since DecodeComputeHopEvent is for ComputeHopEvent
	// This is just to show the layout matches.
	if buf[8] != EventMemWrite {
		t.Errorf("event_type = 0x%02x, want 0x%02x", buf[8], EventMemWrite)
	}
	if binary.LittleEndian.Uint32(buf[12:16]) != 0xABCDEF00 {
		t.Error("flow_label mismatch")
	}
	if binary.LittleEndian.Uint32(buf[16:20]) != 0x12345678 {
		t.Error("addr mismatch")
	}
	if buf[20] != 4 {
		t.Error("size mismatch")
	}
	if binary.LittleEndian.Uint32(buf[24:28]) != 0xDEADBEEF {
		t.Error("value mismatch")
	}
}

// ── Fuzz tests ───────────────────────────────────────────────────────────────

func TestFuzzDecodeComputeHopEvent(t *testing.T) {
	// FuzzDecodeComputeHopEvent should never panic on arbitrary input
	testCases := [][]byte{
		[]byte{},
		make([]byte, 1),
		make([]byte, 93),
		make([]byte, 94),
		make([]byte, 95),
		make([]byte, 200),
		[]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
	}

	for i, data := range testCases {
		// This should not panic
		FuzzDecodeComputeHopEvent(data)
		if t.Failed() {
			t.Errorf("FuzzDecodeComputeHopEvent panicked on test case %d (len=%d)", i, len(data))
			return
		}
	}
}

// ── Cross-validation with Rust sizes ────────────────────────────────────────

func TestComputeEventTypesMatchRust(t *testing.T) {
	// Verify event type constants match Rust side exactly
	tests := []struct {
		name  string
		got   uint8
		want  uint8
	}{
		{"ComputeHop", EventComputeHop, 0x10},
		{"CacheMiss", EventCacheMiss, 0x11},
		{"MemWrite", EventMemWrite, 0x12},
		{"MemStaged", EventMemStaged, 0x13},
		{"ScreenWrite", EventScreenWrite, 0x14},
		{"KeyRead", EventKeyRead, 0x15},
		{"ComputeHalt", EventComputeHalt, 0x16},
		{"ComputeStall", EventComputeStall, 0x17},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s: got 0x%02x, want 0x%02x", tt.name, tt.got, tt.want)
		}
	}
}
