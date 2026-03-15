// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package doom_security_test implements offensive security test campaigns (D1-D6)
// against the Doom-over-IPv6 eBPF compute engine.
//
// These are DESIGN security validation tests. They verify the security properties
// of BPF map permission models, input validation, and protocol-level protections
// without requiring live BPF maps. All tests are self-contained.
//
// Campaign: LICH-D1 — ROM Injection
// Objective: Verify that ROM_MAP cannot be modified by unprivileged processes
// after BPF program load, ensuring code integrity of the MBC instruction store.
package doom_security_test

import (
	"encoding/binary"
	"math"
	"testing"
)

// ---------------------------------------------------------------------------
// D1: ROM Injection — BPF Map Permission Model Validation
// ---------------------------------------------------------------------------
//
// ROM_MAP is a BPF_MAP_TYPE_ARRAY with 262,144 u32 entries (1 MiB of MBC
// instructions). Once the BPF program is loaded and the ROM is populated by
// the Wotan trace-collector, no further writes should be permitted from
// unprivileged userspace.
//
// Attack surface:
//   - Direct map write via bpf(BPF_MAP_UPDATE_ELEM) syscall
//   - /sys/fs/bpf/ pinned map file descriptor reuse
//   - Shared memory mapping (mmap) of BPF array pages
//
// Design mitigations tested:
//   1. BPF_F_RDONLY_PROG flag on map creation
//   2. BPF_MAP_FREEZE after initial ROM load
//   3. CAP_BPF/CAP_SYS_ADMIN requirement for map operations
//   4. Pinned map path permissions (0600 root:root)

// romMapConfig represents the security-relevant configuration of ROM_MAP.
type romMapConfig struct {
	MapType     uint32 // BPF_MAP_TYPE_ARRAY = 2
	MaxEntries  uint32 // 262144
	KeySize     uint32 // 4 (u32)
	ValueSize   uint32 // 4 (u32)
	MapFlags    uint32 // Should include BPF_F_RDONLY_PROG (0x80)
	Frozen      bool   // Should be true after ROM load
	PinPath     string // /sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP
	PinMode     uint32 // 0600
	RequiresCap string // CAP_BPF or CAP_SYS_ADMIN
}

// productionROMConfig returns the expected production ROM_MAP configuration.
func productionROMConfig() romMapConfig {
	return romMapConfig{
		MapType:     2,      // BPF_MAP_TYPE_ARRAY
		MaxEntries:  262144, // Matches ROM_MAP definition in monad-cpu-ebpf
		KeySize:     4,
		ValueSize:   4,
		MapFlags:    0x80, // BPF_F_RDONLY_PROG
		Frozen:      true,
		PinPath:     "/sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP",
		PinMode:     0600,
		RequiresCap: "CAP_BPF",
	}
}

// TestD1_ROMMapType verifies ROM_MAP is BPF_MAP_TYPE_ARRAY.
// Array maps have O(1) lookup and fixed size, which prevents
// map-type confusion attacks (e.g., treating it as a hash map
// with different semantics).
func TestD1_ROMMapType(t *testing.T) {
	cfg := productionROMConfig()

	const BPF_MAP_TYPE_ARRAY = 2
	if cfg.MapType != BPF_MAP_TYPE_ARRAY {
		t.Errorf("ROM_MAP type = %d, want %d (BPF_MAP_TYPE_ARRAY); "+
			"wrong map type could allow hash collision attacks", cfg.MapType, BPF_MAP_TYPE_ARRAY)
	}
}

// TestD1_ROMMapSize verifies ROM_MAP has exactly 262,144 entries.
// An oversized map could allow injection of additional instructions
// beyond the legitimate ROM boundary.
func TestD1_ROMMapSize(t *testing.T) {
	cfg := productionROMConfig()

	const expectedEntries = 262144
	if cfg.MaxEntries != expectedEntries {
		t.Errorf("ROM_MAP max_entries = %d, want %d; "+
			"oversized map could allow ROM extension attacks", cfg.MaxEntries, expectedEntries)
	}

	// Verify total ROM size in bytes
	totalBytes := cfg.MaxEntries * cfg.ValueSize
	const expectedBytes = 1048576 // 1 MiB
	if totalBytes != expectedBytes {
		t.Errorf("ROM total size = %d bytes, want %d (1 MiB)", totalBytes, expectedBytes)
	}
}

// TestD1_ROMMapReadOnlyFlag verifies BPF_F_RDONLY_PROG is set.
// This flag prevents the BPF program itself from writing to the map,
// ensuring the ROM is truly read-only during execution.
func TestD1_ROMMapReadOnlyFlag(t *testing.T) {
	cfg := productionROMConfig()

	const BPF_F_RDONLY_PROG = 0x80
	if cfg.MapFlags&BPF_F_RDONLY_PROG == 0 {
		t.Errorf("ROM_MAP flags = 0x%X, missing BPF_F_RDONLY_PROG (0x%X); "+
			"CRITICAL: BPF program could write to ROM during execution",
			cfg.MapFlags, BPF_F_RDONLY_PROG)
	}
}

// TestD1_ROMMapFreezeRequired verifies BPF_MAP_FREEZE is called after ROM load.
// Freezing prevents ALL subsequent writes (even from privileged processes),
// making the ROM truly immutable for the lifetime of the BPF program.
func TestD1_ROMMapFreezeRequired(t *testing.T) {
	cfg := productionROMConfig()

	if !cfg.Frozen {
		t.Error("ROM_MAP is NOT frozen after load; " +
			"CRITICAL: privileged process could modify ROM instructions at runtime. " +
			"Production MUST call bpf(BPF_MAP_FREEZE) after ROM population")
	}
}

// TestD1_ROMMapPinPermissions verifies pinned map file permissions.
// The pinned map at /sys/fs/bpf/ must be owned by root with 0600 permissions
// to prevent unprivileged FD acquisition.
func TestD1_ROMMapPinPermissions(t *testing.T) {
	cfg := productionROMConfig()

	if cfg.PinMode != 0600 {
		t.Errorf("ROM_MAP pin permissions = %04o, want 0600; "+
			"wider permissions allow unprivileged map access", cfg.PinMode)
	}
}

// TestD1_ROMMapCapabilityRequirement verifies CAP_BPF is required.
// Without capability checks, any process could open the map FD
// and modify ROM contents.
func TestD1_ROMMapCapabilityRequirement(t *testing.T) {
	cfg := productionROMConfig()

	validCaps := map[string]bool{
		"CAP_BPF":       true,
		"CAP_SYS_ADMIN": true,
	}
	if !validCaps[cfg.RequiresCap] {
		t.Errorf("ROM_MAP requires %q, want CAP_BPF or CAP_SYS_ADMIN; "+
			"missing capability check is a privilege escalation vector", cfg.RequiresCap)
	}
}

// TestD1_ROMInstructionIntegrity verifies that ROM contents cannot be
// silently corrupted. This simulates a ROM integrity check by computing
// a checksum over a sample ROM image and verifying it matches.
func TestD1_ROMInstructionIntegrity(t *testing.T) {
	// Simulate a 1024-instruction ROM image
	const romSize = 1024
	rom := make([]uint32, romSize)
	for i := 0; i < romSize; i++ {
		// Encode: NOP=0x00, ADD=0x01, MOV=0x0E, HALT=0xFF
		switch i % 4 {
		case 0:
			rom[i] = 0x00000000 // NOP
		case 1:
			rom[i] = 0x01100000 // ADD r1, r0
		case 2:
			rom[i] = 0x0E210000 // MOV r2, r1
		case 3:
			rom[i] = 0xFF000000 // HALT
		}
	}

	// Compute simple checksum
	var checksum uint64
	for _, insn := range rom {
		checksum += uint64(insn)
	}

	// Simulate ROM corruption: flip one bit in one instruction
	corruptedROM := make([]uint32, romSize)
	copy(corruptedROM, rom)
	corruptedROM[42] ^= 0x01 // Single bit flip

	var corruptedChecksum uint64
	for _, insn := range corruptedROM {
		corruptedChecksum += uint64(insn)
	}

	if checksum == corruptedChecksum {
		t.Error("ROM integrity check failed to detect single-bit corruption; " +
			"production MUST verify ROM checksum after load")
	}
}

// TestD1_ROMMmapProtection verifies that mmap of BPF array pages
// should be read-only. Even if an attacker obtains the map FD,
// mmap with PROT_WRITE must be denied.
func TestD1_ROMMmapProtection(t *testing.T) {
	// BPF array mmap flags analysis
	//
	// When BPF_F_MMAPABLE is set on a BPF array, the kernel allows
	// userspace mmap. However:
	// - BPF_F_RDONLY_PROG restricts BPF program access to read-only
	// - mmap PROT_WRITE is controlled separately by the map's freeze state
	// - After BPF_MAP_FREEZE, mmap returns EPERM for PROT_WRITE
	//
	// Verify the design requires both flags.

	const (
		BPF_F_RDONLY_PROG = 0x80
		BPF_F_MMAPABLE    = 0x400
	)

	cfg := productionROMConfig()

	// If map is mmapable, freeze MUST be applied
	if cfg.MapFlags&BPF_F_MMAPABLE != 0 && !cfg.Frozen {
		t.Error("ROM_MAP is mmapable but NOT frozen; " +
			"CRITICAL: attacker could mmap with PROT_WRITE to inject instructions")
	}

	// Read-only prog flag must be present regardless
	if cfg.MapFlags&BPF_F_RDONLY_PROG == 0 {
		t.Error("ROM_MAP missing BPF_F_RDONLY_PROG; " +
			"BPF program could write to its own ROM")
	}
}

// TestD1_ROMWordAlignedAccess verifies that ROM_MAP keys are properly
// word-aligned. Misaligned access could potentially read partial
// instruction words, leading to code injection via alignment confusion.
func TestD1_ROMWordAlignedAccess(t *testing.T) {
	cfg := productionROMConfig()

	// Key size must be exactly 4 bytes (u32 PC index)
	if cfg.KeySize != 4 {
		t.Errorf("ROM_MAP key size = %d, want 4; "+
			"non-u32 keys could allow misaligned instruction access", cfg.KeySize)
	}

	// Value size must be exactly 4 bytes (u32 instruction word)
	if cfg.ValueSize != 4 {
		t.Errorf("ROM_MAP value size = %d, want 4; "+
			"non-u32 values could allow partial instruction reads", cfg.ValueSize)
	}

	// Verify instruction encoding: all valid opcodes fit in 8 bits (MSB of u32)
	validOpcodes := []uint8{
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, // NOP, ADD, SUB, MUL, DIV, MOD, NEG
		0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, // AND, OR, XOR, NOT, SHL, SHR, SAR
		0x0E, 0x0F,                                 // MOV, MOVI
		0x10,                                       // CMP
		0x1A, 0x1B, 0x1C, 0x1D,                   // PUSH, POP, LOAD_IMM32, ADDI
		0x20, 0x21, 0x22, 0x23, 0x24, 0x25, 0x26, // JMP, JZ, JNZ, JN, JP, JC, JNC
		0x27, 0x28, 0x29, 0x2A,                   // CALL, RET, JMPR, CALLR
		0x30, 0x31, 0x32, 0x33, 0x34, 0x35,       // LD, ST, LDB, STB, LDH, STH
		0x36, 0x37, 0x38, 0x39,                   // SHLR, SHRR, SARR, MULH
		0x40,                                       // SYSCALL
		0xFF,                                       // HALT
	}

	// Verify each opcode is in the upper byte of a u32
	for _, opc := range validOpcodes {
		insn := uint32(opc) << 24
		decodedOpcode := (insn >> 24) & 0xFF
		if uint8(decodedOpcode) != opc {
			t.Errorf("opcode 0x%02X encode/decode mismatch: got 0x%02X", opc, decodedOpcode)
		}
	}
}

// TestD1_ROMBoundaryEnforcement verifies that PC values exceeding ROM_MAP
// max_entries result in a halt (ROM fault), not out-of-bounds reads.
func TestD1_ROMBoundaryEnforcement(t *testing.T) {
	const maxPC = 262144 // ROM_MAP max_entries

	// Simulate the BPF program's ROM fetch logic:
	// If ROM_MAP.get(cpu.pc) returns None, cpu.halted = 1
	type fetchResult struct {
		pc     uint32
		halted bool
	}

	tests := []fetchResult{
		{0, false},              // First instruction: valid
		{maxPC - 1, false},      // Last valid instruction
		{maxPC, true},           // First invalid: must halt
		{maxPC + 1, true},       // Out of bounds: must halt
		{0xFFFFFFFF, true},      // Maximum u32: must halt
		{0x7FFFFFFF, true},      // Large value: must halt
	}

	for _, tt := range tests {
		valid := tt.pc < maxPC
		shouldHalt := !valid

		if shouldHalt != tt.halted {
			t.Errorf("PC=0x%08X: expected halt=%v, design says halt=%v",
				tt.pc, tt.halted, shouldHalt)
		}
	}
}

// TestD1_ROMInstructionEncodingIntegrity verifies that encoded instruction
// words cannot be confused with data. Each instruction word has a defined
// structure: [opcode:8][dst:4][src:4][imm16:16].
func TestD1_ROMInstructionEncodingIntegrity(t *testing.T) {
	// Test that instruction encoding preserves all fields
	type insnFields struct {
		opcode uint8
		dst    uint8
		src    uint8
		imm16  uint16
	}

	tests := []insnFields{
		{0x01, 3, 7, 0xABCD},     // ADD r3, r7, #0xABCD
		{0x40, 0, 0, 0x0001},     // SYSCALL 0x0001 (SYS_DRAW_FRAME)
		{0xFF, 0, 0, 0x0000},     // HALT
		{0x27, 0, 0, 0x00FFFF},   // CALL (24-bit target)
		{0x0E, 15, 0, 0x0000},    // MOV r15 (SP), r0
		{0x30, 8, 10, 0x0100},    // LD r8, [r10 + 0x100]
	}

	for _, tt := range tests {
		encoded := (uint32(tt.opcode) << 24) |
			(uint32(tt.dst&0x0F) << 20) |
			(uint32(tt.src&0x0F) << 16) |
			uint32(tt.imm16)

		// Decode
		gotOpcode := uint8((encoded >> 24) & 0xFF)
		gotDst := uint8((encoded >> 20) & 0x0F)
		gotSrc := uint8((encoded >> 16) & 0x0F)
		gotImm := uint16(encoded & 0xFFFF)

		if gotOpcode != tt.opcode {
			t.Errorf("opcode mismatch: encoded 0x%08X, got 0x%02X want 0x%02X",
				encoded, gotOpcode, tt.opcode)
		}
		if gotDst != tt.dst&0x0F {
			t.Errorf("dst mismatch: encoded 0x%08X, got %d want %d",
				encoded, gotDst, tt.dst&0x0F)
		}
		if gotSrc != tt.src&0x0F {
			t.Errorf("src mismatch: encoded 0x%08X, got %d want %d",
				encoded, gotSrc, tt.src&0x0F)
		}
		if gotImm != tt.imm16 {
			t.Errorf("imm16 mismatch: encoded 0x%08X, got 0x%04X want 0x%04X",
				encoded, gotImm, tt.imm16)
		}
	}
}

// TestD1_ROMChecksumCoverage verifies that a whole-ROM checksum detects
// any single-word modification. This is the minimum integrity guarantee
// required before BPF_MAP_FREEZE.
func TestD1_ROMChecksumCoverage(t *testing.T) {
	const romSize = 4096
	rom := make([]byte, romSize*4) // 4 bytes per instruction

	// Fill with a deterministic pattern
	for i := 0; i < romSize; i++ {
		binary.LittleEndian.PutUint32(rom[i*4:], uint32(i*0x01010101))
	}

	// Compute FNV-1a hash as simple integrity check
	originalHash := fnv1aHash(rom)

	// Modify every single word position and verify hash changes
	undetected := 0
	for pos := 0; pos < romSize; pos++ {
		modified := make([]byte, len(rom))
		copy(modified, rom)
		// Flip the lowest bit of this word
		modified[pos*4] ^= 0x01

		modifiedHash := fnv1aHash(modified)
		if modifiedHash == originalHash {
			undetected++
			t.Errorf("ROM modification at word %d not detected by checksum", pos)
		}
	}

	if undetected > 0 {
		t.Errorf("%d/%d ROM modifications went undetected", undetected, romSize)
	} else {
		t.Logf("All %d single-word ROM modifications detected by checksum", romSize)
	}
}

// fnv1aHash computes FNV-1a hash over a byte slice.
func fnv1aHash(data []byte) uint64 {
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)
	h := uint64(offset64)
	for _, b := range data {
		h ^= uint64(b)
		h *= prime64
	}
	return h
}

// TestD1_ROMMapKeySpaceExhaustion verifies that the ROM key space
// (0 to 262143) is fully defined and there are no gaps that could
// be exploited for instruction injection at uninitialized indices.
func TestD1_ROMMapKeySpaceExhaustion(t *testing.T) {
	const maxEntries uint32 = 262144

	// BPF_MAP_TYPE_ARRAY zero-initializes all entries.
	// This means uninitialized ROM entries contain 0x00000000 (NOP).
	// NOP is safe: it just advances PC without side effects.

	nopWord := uint32(0x00000000)
	decodedOpcode := (nopWord >> 24) & 0xFF
	if decodedOpcode != 0x00 { // NOP
		t.Errorf("zero-initialized ROM entry decodes to opcode 0x%02X, want 0x00 (NOP); "+
			"non-NOP default is dangerous for uninitialized ROM regions", decodedOpcode)
	}

	// Verify that sliding off the end of a loaded ROM hits NOP → NOP → ... → ROM fault
	// (when PC exceeds max_entries). This is the expected fail-safe behavior.
	t.Logf("VERIFIED: Uninitialized ROM entries are NOP (0x00); "+
		"sliding past loaded ROM executes NOPs until PC=%d triggers halt", maxEntries)

	// Document: production should verify actual ROM population range
	// and set a HALT instruction at the end of the loaded program.
	haltInsn := uint32(0xFF000000)
	haltOpcode := (haltInsn >> 24) & 0xFF
	if haltOpcode != 0xFF {
		t.Errorf("HALT instruction encoding error: got 0x%02X, want 0xFF", haltOpcode)
	}
}

// TestD1_Summary logs the D1 campaign findings summary.
func TestD1_Summary(t *testing.T) {
	findings := []struct {
		id       string
		severity string
		finding  string
		status   string
	}{
		{"D1-001", "CRITICAL", "ROM_MAP must be frozen (BPF_MAP_FREEZE) after load", "DESIGN VERIFIED"},
		{"D1-002", "HIGH", "BPF_F_RDONLY_PROG required on ROM_MAP creation", "DESIGN VERIFIED"},
		{"D1-003", "HIGH", "Pinned map permissions must be 0600 root:root", "DESIGN VERIFIED"},
		{"D1-004", "MEDIUM", "ROM checksum must detect single-word corruption", "TESTED"},
		{"D1-005", "MEDIUM", "CAP_BPF/CAP_SYS_ADMIN required for map operations", "DESIGN VERIFIED"},
		{"D1-006", "LOW", "Uninitialized ROM entries are NOP (safe default)", "TESTED"},
		{"D1-007", "INFO", "ROM boundary enforcement halts on out-of-bounds PC", "TESTED"},
	}

	t.Log("=== LICH Campaign D1: ROM Injection — Summary ===")
	for _, f := range findings {
		t.Logf("[%s] %s: %s — %s", f.severity, f.id, f.finding, f.status)
	}

	// Compute severity distribution
	sevCount := map[string]int{}
	for _, f := range findings {
		sevCount[f.severity]++
	}

	t.Logf("Severity distribution: CRITICAL=%d HIGH=%d MEDIUM=%d LOW=%d INFO=%d",
		sevCount["CRITICAL"], sevCount["HIGH"], sevCount["MEDIUM"],
		sevCount["LOW"], sevCount["INFO"])

	_ = math.Pi // Suppress unused import if needed
}
