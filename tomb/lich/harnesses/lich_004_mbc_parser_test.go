// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// LICH-004: MBC (Monad ByteCode) Parser Fuzzer
//
// Target: MBC bytecode parsing and validation.
// Goal: Find crashes in the MBC instruction decoder, invalid opcode handling,
//       and operand parsing edge cases.
//
// MBC is a custom bytecode format for Monad state machine programs.
// The instruction format is:
//   [Opcode:1][Operand1:4][Operand2:4] = 9 bytes per instruction
//
// Attack vectors:
//   - Invalid/reserved opcodes
//   - Truncated instructions (< 9 bytes)
//   - Programs exceeding max instruction count
//   - Operand values at boundary conditions
//   - Infinite loop detection in programs
//   - Stack overflow from nested calls

package harnesses

import (
	"encoding/binary"
	"fmt"
	"testing"
)

// MBC opcodes -- mirrors the Rust crate definitions
const (
	MBC_NOP      uint8 = 0x00
	MBC_LOAD     uint8 = 0x01
	MBC_STORE    uint8 = 0x02
	MBC_ADD      uint8 = 0x03
	MBC_SUB      uint8 = 0x04
	MBC_MUL      uint8 = 0x05
	MBC_DIV      uint8 = 0x06
	MBC_MOD      uint8 = 0x07
	MBC_AND      uint8 = 0x08
	MBC_OR       uint8 = 0x09
	MBC_XOR      uint8 = 0x0A
	MBC_SHL      uint8 = 0x0B
	MBC_SHR      uint8 = 0x0C
	MBC_JMP      uint8 = 0x10
	MBC_JEQ      uint8 = 0x11
	MBC_JNE      uint8 = 0x12
	MBC_JGT      uint8 = 0x13
	MBC_JLT      uint8 = 0x14
	MBC_CALL     uint8 = 0x20
	MBC_RET      uint8 = 0x21
	MBC_EMIT     uint8 = 0x30
	MBC_HALT     uint8 = 0xFF
)

// MBCInstSize is the fixed size of an MBC instruction.
const MBCInstSize = 9

// MaxMBCInstructions is the maximum number of instructions in a program.
const MaxMBCInstructions = 4096

// MaxMBCRegisters is the number of registers available.
const MaxMBCRegisters = 16

// MaxMBCStackDepth is the maximum call stack depth.
const MaxMBCStackDepth = 64

// MBCInstruction represents a parsed MBC instruction.
type MBCInstruction struct {
	Opcode   uint8
	Operand1 uint32
	Operand2 uint32
}

// MBCProgram represents a parsed MBC program.
type MBCProgram struct {
	Instructions []MBCInstruction
}

// ParseMBCProgram parses a byte slice into an MBC program.
func ParseMBCProgram(data []byte) (*MBCProgram, error) {
	if len(data) == 0 {
		return &MBCProgram{Instructions: nil}, nil
	}

	if len(data)%MBCInstSize != 0 {
		return nil, fmt.Errorf("mbc: program size %d not aligned to %d-byte instructions", len(data), MBCInstSize)
	}

	numInst := len(data) / MBCInstSize
	if numInst > MaxMBCInstructions {
		return nil, fmt.Errorf("mbc: program has %d instructions, max %d", numInst, MaxMBCInstructions)
	}

	prog := &MBCProgram{
		Instructions: make([]MBCInstruction, numInst),
	}

	for i := 0; i < numInst; i++ {
		offset := i * MBCInstSize
		inst := MBCInstruction{
			Opcode:   data[offset],
			Operand1: binary.BigEndian.Uint32(data[offset+1 : offset+5]),
			Operand2: binary.BigEndian.Uint32(data[offset+5 : offset+9]),
		}
		prog.Instructions[i] = inst
	}

	return prog, nil
}

// ValidateMBCProgram performs static analysis on a parsed program.
func ValidateMBCProgram(prog *MBCProgram) error {
	if prog == nil {
		return fmt.Errorf("mbc: nil program")
	}

	if len(prog.Instructions) == 0 {
		return nil
	}

	for i, inst := range prog.Instructions {
		// Validate opcode
		if !isValidMBCOpcode(inst.Opcode) {
			return fmt.Errorf("mbc: invalid opcode 0x%02X at instruction %d", inst.Opcode, i)
		}

		// Validate register operands for register-using instructions
		switch inst.Opcode {
		case MBC_LOAD, MBC_STORE:
			if inst.Operand1 >= MaxMBCRegisters {
				return fmt.Errorf("mbc: register %d out of range at instruction %d", inst.Operand1, i)
			}
		case MBC_ADD, MBC_SUB, MBC_MUL, MBC_DIV, MBC_MOD,
			MBC_AND, MBC_OR, MBC_XOR, MBC_SHL, MBC_SHR:
			if inst.Operand1 >= MaxMBCRegisters || inst.Operand2 >= MaxMBCRegisters {
				return fmt.Errorf("mbc: register out of range at instruction %d", i)
			}
		case MBC_JMP, MBC_JEQ, MBC_JNE, MBC_JGT, MBC_JLT:
			target := int(inst.Operand1)
			if target >= len(prog.Instructions) {
				return fmt.Errorf("mbc: jump target %d out of bounds at instruction %d (max %d)",
					target, i, len(prog.Instructions)-1)
			}
		case MBC_CALL:
			target := int(inst.Operand1)
			if target >= len(prog.Instructions) {
				return fmt.Errorf("mbc: call target %d out of bounds at instruction %d", target, i)
			}
		}
	}

	// Check for HALT instruction
	lastInst := prog.Instructions[len(prog.Instructions)-1]
	if lastInst.Opcode != MBC_HALT && lastInst.Opcode != MBC_RET {
		return fmt.Errorf("mbc: program does not end with HALT or RET")
	}

	return nil
}

func isValidMBCOpcode(op uint8) bool {
	switch op {
	case MBC_NOP, MBC_LOAD, MBC_STORE,
		MBC_ADD, MBC_SUB, MBC_MUL, MBC_DIV, MBC_MOD,
		MBC_AND, MBC_OR, MBC_XOR, MBC_SHL, MBC_SHR,
		MBC_JMP, MBC_JEQ, MBC_JNE, MBC_JGT, MBC_JLT,
		MBC_CALL, MBC_RET, MBC_EMIT, MBC_HALT:
		return true
	default:
		return false
	}
}

// EncodeMBCInstruction encodes a single instruction.
func EncodeMBCInstruction(inst MBCInstruction) []byte {
	buf := make([]byte, MBCInstSize)
	buf[0] = inst.Opcode
	binary.BigEndian.PutUint32(buf[1:5], inst.Operand1)
	binary.BigEndian.PutUint32(buf[5:9], inst.Operand2)
	return buf
}

// FuzzMBCParsing feeds random bytes to the MBC parser.
func FuzzMBCParsing(f *testing.F) {
	// Valid minimal program: NOP + HALT
	validProg := make([]byte, MBCInstSize*2)
	validProg[0] = MBC_NOP
	validProg[MBCInstSize] = MBC_HALT
	f.Add(validProg)

	// Empty program
	f.Add([]byte{})

	// Truncated instruction
	f.Add([]byte{MBC_NOP, 0x00, 0x00})

	// Single HALT
	halt := make([]byte, MBCInstSize)
	halt[0] = MBC_HALT
	f.Add(halt)

	// All zeros (9 bytes = one NOP with zero operands)
	f.Add(make([]byte, MBCInstSize))

	// All 0xFF (one instruction with opcode 0xFF = HALT)
	allFF := make([]byte, MBCInstSize)
	for i := range allFF {
		allFF[i] = 0xFF
	}
	f.Add(allFF)

	// Invalid opcode
	invalidOp := make([]byte, MBCInstSize)
	invalidOp[0] = 0xFE // not a valid opcode
	f.Add(invalidOp)

	// Jump to self (potential infinite loop)
	jumpSelf := make([]byte, MBCInstSize*2)
	jumpSelf[0] = MBC_JMP
	binary.BigEndian.PutUint32(jumpSelf[1:5], 0) // jump to instruction 0
	jumpSelf[MBCInstSize] = MBC_HALT
	f.Add(jumpSelf)

	// Out-of-bounds jump
	oobJump := make([]byte, MBCInstSize*2)
	oobJump[0] = MBC_JMP
	binary.BigEndian.PutUint32(oobJump[1:5], 999) // way out of bounds
	oobJump[MBCInstSize] = MBC_HALT
	f.Add(oobJump)

	// Division by zero setup
	divZero := make([]byte, MBCInstSize*3)
	divZero[0] = MBC_LOAD                              // LOAD r0, 42
	binary.BigEndian.PutUint32(divZero[1:5], 0)         // register 0
	binary.BigEndian.PutUint32(divZero[5:9], 42)        // value 42
	divZero[MBCInstSize] = MBC_DIV                      // DIV r0, r1 (r1 = 0)
	binary.BigEndian.PutUint32(divZero[MBCInstSize+1:MBCInstSize+5], 0)
	binary.BigEndian.PutUint32(divZero[MBCInstSize+5:MBCInstSize+9], 1)
	divZero[MBCInstSize*2] = MBC_HALT
	f.Add(divZero)

	f.Fuzz(func(t *testing.T, data []byte) {
		prog, parseErr := ParseMBCProgram(data)
		if parseErr != nil {
			return // parse errors are expected
		}

		// Validate the parsed program
		valErr := ValidateMBCProgram(prog)
		_ = valErr // validation errors are expected

		// Roundtrip: re-encode and verify
		if prog != nil && len(prog.Instructions) > 0 {
			reEncoded := make([]byte, 0, len(prog.Instructions)*MBCInstSize)
			for _, inst := range prog.Instructions {
				reEncoded = append(reEncoded, EncodeMBCInstruction(inst)...)
			}

			reParsed, reErr := ParseMBCProgram(reEncoded)
			if reErr != nil {
				t.Fatalf("roundtrip parse failed: %v", reErr)
			}

			if len(reParsed.Instructions) != len(prog.Instructions) {
				t.Fatalf("roundtrip length mismatch: %d vs %d",
					len(reParsed.Instructions), len(prog.Instructions))
			}

			for i := range prog.Instructions {
				if prog.Instructions[i] != reParsed.Instructions[i] {
					t.Errorf("instruction %d mismatch after roundtrip", i)
				}
			}
		}
	})
}

// FuzzMBCValidation generates structured programs and validates them.
func FuzzMBCValidation(f *testing.F) {
	f.Add(uint8(MBC_NOP), uint32(0), uint32(0), uint8(MBC_HALT), uint32(0), uint32(0))
	f.Add(uint8(MBC_LOAD), uint32(0), uint32(42), uint8(MBC_HALT), uint32(0), uint32(0))
	f.Add(uint8(MBC_JMP), uint32(1), uint32(0), uint8(MBC_HALT), uint32(0), uint32(0))
	f.Add(uint8(MBC_ADD), uint32(0), uint32(1), uint8(MBC_RET), uint32(0), uint32(0))
	f.Add(uint8(MBC_DIV), uint32(0), uint32(0), uint8(MBC_HALT), uint32(0), uint32(0))
	f.Add(uint8(0xFE), uint32(0), uint32(0), uint8(MBC_HALT), uint32(0), uint32(0)) // invalid opcode

	f.Fuzz(func(t *testing.T, op1 uint8, a1 uint32, b1 uint32, op2 uint8, a2 uint32, b2 uint32) {
		// Build a 2-instruction program
		data := make([]byte, MBCInstSize*2)
		data[0] = op1
		binary.BigEndian.PutUint32(data[1:5], a1)
		binary.BigEndian.PutUint32(data[5:9], b1)
		data[MBCInstSize] = op2
		binary.BigEndian.PutUint32(data[MBCInstSize+1:MBCInstSize+5], a2)
		binary.BigEndian.PutUint32(data[MBCInstSize+5:MBCInstSize+9], b2)

		prog, err := ParseMBCProgram(data)
		if err != nil {
			return
		}

		valErr := ValidateMBCProgram(prog)
		_ = valErr // validation errors are expected for fuzzed programs
	})
}
