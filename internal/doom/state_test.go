package doom

import (
	"encoding/binary"
	"strings"
	"testing"
)

// TestDefaultCpuState tests the default CPU state initialization.
func TestDefaultCpuState(t *testing.T) {
	cpu := DefaultCpuState()

	if cpu.PC != RegPCDefault {
		t.Errorf("PC = 0x%X, want 0x%X", cpu.PC, RegPCDefault)
	}

	if cpu.Regs[RegSP] != RegSPDefault {
		t.Errorf("Regs[SP] = 0x%08X, want 0x%08X", cpu.Regs[RegSP], RegSPDefault)
	}

	if cpu.Flags != 0 {
		t.Errorf("Flags = 0x%X, want 0", cpu.Flags)
	}

	if cpu.Halted != 0 {
		t.Errorf("Halted = %d, want 0", cpu.Halted)
	}

	if cpu.Stalled != 0 {
		t.Errorf("Stalled = %d, want 0", cpu.Stalled)
	}

	if cpu.InsnCount != 0 {
		t.Errorf("InsnCount = %d, want 0", cpu.InsnCount)
	}

	if cpu.CacheHits != 0 {
		t.Errorf("CacheHits = %d, want 0", cpu.CacheHits)
	}

	if cpu.CacheMisses != 0 {
		t.Errorf("CacheMisses = %d, want 0", cpu.CacheMisses)
	}

	if cpu.SleepUntil != 0 {
		t.Errorf("SleepUntil = %d, want 0", cpu.SleepUntil)
	}

	// All non-SP registers should be zero
	for i := 0; i < RegCount; i++ {
		if i == RegSP {
			continue
		}
		if cpu.Regs[i] != 0 {
			t.Errorf("Regs[%d] = 0x%X, want 0", i, cpu.Regs[i])
		}
	}
}

// TestReadWriteCpuState tests reading and writing CPU state through a mock map.
func TestReadWriteCpuState(t *testing.T) {
	m := newMockMap()

	// Write a CPU state
	cpu := DefaultCpuState()
	cpu.PC = 0x42
	cpu.Regs[0] = 0xDEAD
	cpu.InsnCount = 9999

	if err := WriteCpuState(m, cpu); err != nil {
		t.Fatalf("WriteCpuState: %v", err)
	}

	// Read it back
	got, err := ReadCpuState(m)
	if err != nil {
		t.Fatalf("ReadCpuState: %v", err)
	}

	if got.PC != cpu.PC {
		t.Errorf("PC = 0x%X, want 0x%X", got.PC, cpu.PC)
	}
	if got.Regs[0] != cpu.Regs[0] {
		t.Errorf("Regs[0] = 0x%X, want 0x%X", got.Regs[0], cpu.Regs[0])
	}
	if got.Regs[RegSP] != cpu.Regs[RegSP] {
		t.Errorf("Regs[SP] = 0x%X, want 0x%X", got.Regs[RegSP], cpu.Regs[RegSP])
	}
	if got.InsnCount != cpu.InsnCount {
		t.Errorf("InsnCount = %d, want %d", got.InsnCount, cpu.InsnCount)
	}
}

// TestReadCpuStateNotFound tests reading CPU state when the map is empty.
func TestReadCpuStateNotFound(t *testing.T) {
	m := newMockMap()
	_, err := ReadCpuState(m)
	if err == nil {
		t.Fatal("ReadCpuState should fail on empty map")
	}
}

// TestReadCpuStateError tests reading CPU state with error map.
func TestReadCpuStateError(t *testing.T) {
	m := newErrorMap("bpf error")
	_, err := ReadCpuState(m)
	if err == nil {
		t.Fatal("ReadCpuState should fail with error map")
	}
}

// TestWriteCpuStateError tests writing CPU state with error map.
func TestWriteCpuStateError(t *testing.T) {
	m := newErrorMap("bpf error")
	cpu := DefaultCpuState()
	err := WriteCpuState(m, cpu)
	if err == nil {
		t.Fatal("WriteCpuState should fail with error map")
	}
}

// TestResetCpuState tests resetting CPU state to defaults.
func TestResetCpuState(t *testing.T) {
	m := newMockMap()

	// Write a modified state first
	cpu := &CpuState{
		PC:        0x5000,
		Flags:     FlagZero | FlagCarry,
		Halted:    1,
		InsnCount: 1000000,
	}
	cpu.Regs[0] = 0xAAAA
	cpu.Regs[RegSP] = 0xBBBB

	if err := WriteCpuState(m, cpu); err != nil {
		t.Fatalf("WriteCpuState: %v", err)
	}

	// Reset
	if err := ResetCpuState(m); err != nil {
		t.Fatalf("ResetCpuState: %v", err)
	}

	// Read back and verify it's default
	got, err := ReadCpuState(m)
	if err != nil {
		t.Fatalf("ReadCpuState: %v", err)
	}

	if got.PC != RegPCDefault {
		t.Errorf("PC = 0x%X, want 0x%X", got.PC, RegPCDefault)
	}
	if got.Regs[RegSP] != RegSPDefault {
		t.Errorf("Regs[SP] = 0x%08X, want 0x%08X", got.Regs[RegSP], RegSPDefault)
	}
	if got.Flags != 0 {
		t.Errorf("Flags = 0x%X, want 0", got.Flags)
	}
	if got.Halted != 0 {
		t.Errorf("Halted = %d, want 0", got.Halted)
	}
	if got.InsnCount != 0 {
		t.Errorf("InsnCount = %d, want 0", got.InsnCount)
	}
	if got.Regs[0] != 0 {
		t.Errorf("Regs[0] = 0x%X, want 0", got.Regs[0])
	}
}

// TestResetCpuStateError tests resetting CPU state with error map.
func TestResetCpuStateError(t *testing.T) {
	m := newErrorMap("bpf error")
	err := ResetCpuState(m)
	if err == nil {
		t.Fatal("ResetCpuState should fail with error map")
	}
}

// TestCpuStateIsHalted tests the IsHalted helper.
func TestCpuStateIsHalted(t *testing.T) {
	cpu := DefaultCpuState()
	if cpu.IsHalted() {
		t.Error("default state should not be halted")
	}

	cpu.Halted = 1
	if !cpu.IsHalted() {
		t.Error("state with Halted=1 should be halted")
	}
}

// TestCpuStateIsStalled tests the IsStalled helper.
func TestCpuStateIsStalled(t *testing.T) {
	cpu := DefaultCpuState()
	if cpu.IsStalled() {
		t.Error("default state should not be stalled")
	}

	cpu.Stalled = 1
	if !cpu.IsStalled() {
		t.Error("state with Stalled=1 should be stalled")
	}
}

// TestCpuStateHasFlag tests the HasFlag helper.
func TestCpuStateHasFlag(t *testing.T) {
	cpu := DefaultCpuState()
	if cpu.HasFlag(FlagZero) {
		t.Error("default state should not have zero flag")
	}

	cpu.Flags = FlagZero | FlagCarry
	if !cpu.HasFlag(FlagZero) {
		t.Error("should have zero flag")
	}
	if !cpu.HasFlag(FlagCarry) {
		t.Error("should have carry flag")
	}
	if cpu.HasFlag(FlagNegative) {
		t.Error("should not have negative flag")
	}
}

// TestCpuStateFormatTable tests the table formatter.
func TestCpuStateFormatTable(t *testing.T) {
	cpu := DefaultCpuState()
	cpu.PC = 0x1234
	cpu.Flags = FlagZero | FlagNegative
	cpu.InsnCount = 42000
	cpu.CacheHits = 300
	cpu.CacheMisses = 50

	table := cpu.FormatTable()

	// Verify key elements are present
	checks := []string{
		"PC:", "0x00001234",
		"ZN-",
		"running",
		"42000",
		"300 hits",
		"50 misses",
		"SP",
		"0xFFFF0000",
	}
	for _, check := range checks {
		if !strings.Contains(table, check) {
			t.Errorf("FormatTable should contain %q, got:\n%s", check, table)
		}
	}
}

// TestCpuStateFormatTableHalted tests the table formatter for halted state.
func TestCpuStateFormatTableHalted(t *testing.T) {
	cpu := DefaultCpuState()
	cpu.Halted = 1
	table := cpu.FormatTable()
	if !strings.Contains(table, "HALTED") {
		t.Errorf("FormatTable should show HALTED, got:\n%s", table)
	}
}

// TestCpuStateFormatTableStalled tests the table formatter for stalled state.
func TestCpuStateFormatTableStalled(t *testing.T) {
	cpu := DefaultCpuState()
	cpu.Stalled = 1
	table := cpu.FormatTable()
	if !strings.Contains(table, "STALLED") {
		t.Errorf("FormatTable should show STALLED, got:\n%s", table)
	}
}

// TestCpuStateFormatTableSleeping tests the table formatter with sleep.
func TestCpuStateFormatTableSleeping(t *testing.T) {
	cpu := DefaultCpuState()
	cpu.SleepUntil = 999999
	table := cpu.FormatTable()
	if !strings.Contains(table, "Sleep") {
		t.Errorf("FormatTable should show Sleep info, got:\n%s", table)
	}
}

// TestCpuStateFormatTableCacheRate tests the hit rate formatting.
func TestCpuStateFormatTableCacheRate(t *testing.T) {
	cpu := DefaultCpuState()
	cpu.CacheHits = 75
	cpu.CacheMisses = 25
	table := cpu.FormatTable()
	if !strings.Contains(table, "75.0%") {
		t.Errorf("FormatTable should show 75.0%% hit rate, got:\n%s", table)
	}
}

// TestCpuStateFormatOneLiner tests the one-liner formatter.
func TestCpuStateFormatOneLiner(t *testing.T) {
	cpu := DefaultCpuState()
	cpu.PC = 0x42
	cpu.Flags = FlagZero
	cpu.InsnCount = 100

	line := cpu.FormatOneLiner()
	checks := []string{"PC=0x00000042", "SP=0xFFFF0000", "Z", "run", "insns=100"}
	for _, check := range checks {
		if !strings.Contains(line, check) {
			t.Errorf("FormatOneLiner should contain %q, got: %s", check, line)
		}
	}
}

// TestCpuStateFormatOneLinerHalted tests the one-liner for halted state.
func TestCpuStateFormatOneLinerHalted(t *testing.T) {
	cpu := DefaultCpuState()
	cpu.Halted = 1
	line := cpu.FormatOneLiner()
	if !strings.Contains(line, "HALT") {
		t.Errorf("FormatOneLiner should contain HALT, got: %s", line)
	}
}

// TestCpuStateFormatOneLinerNoFlags tests the one-liner with no flags.
func TestCpuStateFormatOneLinerNoFlags(t *testing.T) {
	cpu := DefaultCpuState()
	line := cpu.FormatOneLiner()
	if !strings.Contains(line, "flags=-") {
		t.Errorf("FormatOneLiner should show flags=-, got: %s", line)
	}
}

// TestWriteReadCpuStateAllFields verifies all fields survive write/read cycle.
func TestWriteReadCpuStateAllFields(t *testing.T) {
	m := newMockMap()

	cpu := &CpuState{
		PC:          0xABCD,
		Flags:       FlagZero | FlagNegative | FlagCarry,
		Halted:      1,
		Stalled:     1,
		Pad:         0xFF,
		SleepUntil:  123456789,
		InsnCount:   987654321,
		CacheHits:   111,
		CacheMisses: 222,
	}
	for i := 0; i < RegCount; i++ {
		cpu.Regs[i] = uint32(i * 0x11111111)
	}

	if err := WriteCpuState(m, cpu); err != nil {
		t.Fatalf("WriteCpuState: %v", err)
	}

	got, err := ReadCpuState(m)
	if err != nil {
		t.Fatalf("ReadCpuState: %v", err)
	}

	// Verify cpu_map key is 0
	key := make([]byte, 4)
	binary.LittleEndian.PutUint32(key, 0)

	// Verify all fields
	if got.PC != cpu.PC {
		t.Errorf("PC mismatch")
	}
	if got.Flags != cpu.Flags {
		t.Errorf("Flags mismatch")
	}
	if got.Halted != cpu.Halted {
		t.Errorf("Halted mismatch")
	}
	if got.Stalled != cpu.Stalled {
		t.Errorf("Stalled mismatch")
	}
	if got.SleepUntil != cpu.SleepUntil {
		t.Errorf("SleepUntil mismatch")
	}
	if got.InsnCount != cpu.InsnCount {
		t.Errorf("InsnCount mismatch")
	}
	if got.CacheHits != cpu.CacheHits {
		t.Errorf("CacheHits mismatch")
	}
	if got.CacheMisses != cpu.CacheMisses {
		t.Errorf("CacheMisses mismatch")
	}
	for i := 0; i < RegCount; i++ {
		if got.Regs[i] != cpu.Regs[i] {
			t.Errorf("Regs[%d] = 0x%X, want 0x%X", i, got.Regs[i], cpu.Regs[i])
		}
	}
}
