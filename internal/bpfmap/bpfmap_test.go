//go:build !integration
// +build !integration

package bpfmap

import (
	"testing"
)

// TestUpdateElem_InvalidFD verifies that UpdateElem fails gracefully on a closed map.
func TestUpdateElem_InvalidFD(t *testing.T) {
	m := &Map{fd: -1}

	key := []byte{0}
	value := make([]byte, 10)

	err := m.UpdateElem(key, value)
	if err == nil {
		t.Error("expected error on closed map, got nil")
	}
}

// TestClose_Idempotent verifies that Close can be called multiple times safely.
func TestClose_Idempotent(t *testing.T) {
	m := &Map{fd: -1} // Already closed

	err1 := m.Close()
	err2 := m.Close()

	if err1 != nil {
		t.Errorf("first Close() returned error: %v", err1)
	}
	if err2 != nil {
		t.Errorf("second Close() returned error: %v", err2)
	}
}

// TestPath verifies that Path() returns the pinned path.
func TestPath(t *testing.T) {
	expectedPath := "/sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP"
	m := &Map{path: expectedPath}

	if m.Path() != expectedPath {
		t.Errorf("Path() returned %q, want %q", m.Path(), expectedPath)
	}
}

// TestIsOpen verifies the IsOpen() status check.
func TestIsOpen(t *testing.T) {
	tests := []struct {
		name     string
		fd       int
		wantOpen bool
	}{
		{"Valid FD", 3, true},
		{"Closed (minus one)", -1, false},
		{"Closed (zero)", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Map{fd: tt.fd}
			if m.IsOpen() != tt.wantOpen {
				t.Errorf("IsOpen() = %v, want %v", m.IsOpen(), tt.wantOpen)
			}
		})
	}
}

// TestFormatCpuState verifies CPU state serialization.
func TestFormatCpuState(t *testing.T) {
	var regs [16]uint64
	for i := 0; i < 16; i++ {
		regs[i] = uint64(i) + 1
	}
	pc := uint64(0x1000)
	sp := uint64(0x3F00000)
	flags := uint64(0)
	halted := false
	instrCount := uint64(12345)
	var kbdState [32]byte

	state := FormatCpuState(regs, pc, sp, flags, halted, instrCount, kbdState)

	if len(state) != 136 {
		t.Errorf("FormatCpuState returned %d bytes, want 136", len(state))
	}

	// Spot-check: first register should be 0x01 (little-endian)
	if state[0] != 0x01 {
		t.Errorf("first byte = 0x%02x, want 0x01", state[0])
	}
}

// TestKBDValueFormat verifies KBD map value encoding.
func TestKBDValueFormat(t *testing.T) {
	var bitmap [32]byte
	bitmap[0] = 0xFF // Set lower 8 bits
	sequence := uint64(42)

	value := FormatKBDValue(bitmap, sequence)

	if len(value) != 40 {
		t.Errorf("FormatKBDValue returned %d bytes, want 40", len(value))
	}

	// Parse it back
	parsedBitmap, parsedSeq, err := ParseKBDValue(value)
	if err != nil {
		t.Fatalf("ParseKBDValue failed: %v", err)
	}

	if parsedBitmap[0] != 0xFF {
		t.Errorf("parsed bitmap[0] = 0x%02x, want 0xFF", parsedBitmap[0])
	}
	if parsedSeq != 42 {
		t.Errorf("parsed sequence = %d, want 42", parsedSeq)
	}
}

// TestParseKBDValue_InvalidLength verifies error on malformed KBD value.
func TestParseKBDValue_InvalidLength(t *testing.T) {
	tests := []struct {
		name      string
		valueLen  int
		wantError bool
	}{
		{"Correct length", 40, false},
		{"Too short", 39, true},
		{"Too long", 41, true},
		{"Empty", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := make([]byte, tt.valueLen)
			_, _, err := ParseKBDValue(value)
			if (err != nil) != tt.wantError {
				t.Errorf("ParseKBDValue error = %v, wantError = %v", err, tt.wantError)
			}
		})
	}
}

// BenchmarkFormatCpuState benchmarks CPU state serialization.
func BenchmarkFormatCpuState(b *testing.B) {
	var regs [16]uint64
	for i := 0; i < 16; i++ {
		regs[i] = uint64(i)
	}
	pc := uint64(0x1000)
	sp := uint64(0x3F00000)
	var kbdState [32]byte

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FormatCpuState(regs, pc, sp, 0, false, uint64(i), kbdState)
	}
}

// Integration test stubs (disabled by build tag)

// TestOpen_Integration opens a real BPF map from /sys/fs/bpf
// Run with: go test -tags=integration ./...
func TestOpen_Integration(t *testing.T) {
	// Example: test opening a real pinned map
	// m, err := Open("/sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP")
	// if err != nil {
	//     t.Fatalf("Open failed: %v", err)
	// }
	// defer m.Close()
	//
	// if !m.IsOpen() {
	//     t.Error("expected map to be open")
	// }
}
