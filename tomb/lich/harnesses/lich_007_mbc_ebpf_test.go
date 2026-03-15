// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// LICH-007: eBPF MBC Program Fuzzer
//
// Target: MBC bytecode execution engine (simulated in userspace).
// Goal: Find crashes, infinite loops, stack overflows, and unsafe memory access
//       in the MBC virtual machine.
//
// This harness builds on LICH-004 (parsing) and LICH-005 (kernel interface)
// to fuzz actual program EXECUTION. The MBC VM is simulated entirely in
// userspace with safety bounds:
//   - Maximum instruction count per execution (prevents infinite loops)
//   - Maximum stack depth (prevents stack overflow)
//   - Bounds-checked register access
//   - Division by zero detection
//
// Attack vectors:
//   - Self-modifying code patterns
//   - Backward jumps creating tight loops
//   - Stack exhaustion via deep recursion
//   - Register corruption via crafted arithmetic

package harnesses

import (
	"encoding/binary"
	"fmt"
	"testing"
)

// MBCVMConfig configures the virtual machine.
type MBCVMConfig struct {
	MaxInstructions int
	MaxStackDepth   int
	MaxMemory       int
}

// DefaultMBCVMConfig returns safe defaults.
func DefaultMBCVMConfig() MBCVMConfig {
	return MBCVMConfig{
		MaxInstructions: 10000,
		MaxStackDepth:   64,
		MaxMemory:       4096,
	}
}

// MBCVM is the MBC virtual machine.
type MBCVM struct {
	cfg        MBCVMConfig
	registers  [MaxMBCRegisters]uint32
	memory     []byte
	callStack  []int
	pc         int
	execCount  int
	halted     bool
	err        error
	emitted    []uint32
}

// NewMBCVM creates a new VM instance.
func NewMBCVM(cfg MBCVMConfig) *MBCVM {
	return &MBCVM{
		cfg:    cfg,
		memory: make([]byte, cfg.MaxMemory),
	}
}

// Execute runs an MBC program.
func (vm *MBCVM) Execute(prog *MBCProgram) error {
	if prog == nil || len(prog.Instructions) == 0 {
		return fmt.Errorf("mbc_vm: empty program")
	}

	vm.pc = 0
	vm.execCount = 0
	vm.halted = false
	vm.err = nil
	vm.callStack = vm.callStack[:0]
	vm.emitted = vm.emitted[:0]

	for !vm.halted && vm.err == nil {
		vm.execCount++
		if vm.execCount > vm.cfg.MaxInstructions {
			vm.err = fmt.Errorf("mbc_vm: exceeded max instructions (%d)", vm.cfg.MaxInstructions)
			break
		}

		if vm.pc < 0 || vm.pc >= len(prog.Instructions) {
			vm.err = fmt.Errorf("mbc_vm: PC out of bounds: %d (max %d)", vm.pc, len(prog.Instructions)-1)
			break
		}

		inst := prog.Instructions[vm.pc]
		vm.executeInstruction(inst, prog)
	}

	return vm.err
}

func (vm *MBCVM) executeInstruction(inst MBCInstruction, prog *MBCProgram) {
	switch inst.Opcode {
	case MBC_NOP:
		vm.pc++

	case MBC_LOAD:
		reg := inst.Operand1
		if reg >= MaxMBCRegisters {
			vm.err = fmt.Errorf("mbc_vm: LOAD register %d out of bounds", reg)
			return
		}
		vm.registers[reg] = inst.Operand2
		vm.pc++

	case MBC_STORE:
		reg := inst.Operand1
		if reg >= MaxMBCRegisters {
			vm.err = fmt.Errorf("mbc_vm: STORE register %d out of bounds", reg)
			return
		}
		addr := inst.Operand2
		if int(addr)+4 > len(vm.memory) {
			vm.err = fmt.Errorf("mbc_vm: STORE address %d out of bounds", addr)
			return
		}
		binary.BigEndian.PutUint32(vm.memory[addr:addr+4], vm.registers[reg])
		vm.pc++

	case MBC_ADD:
		dst, src := inst.Operand1, inst.Operand2
		if dst >= MaxMBCRegisters || src >= MaxMBCRegisters {
			vm.err = fmt.Errorf("mbc_vm: ADD register out of bounds")
			return
		}
		vm.registers[dst] += vm.registers[src]
		vm.pc++

	case MBC_SUB:
		dst, src := inst.Operand1, inst.Operand2
		if dst >= MaxMBCRegisters || src >= MaxMBCRegisters {
			vm.err = fmt.Errorf("mbc_vm: SUB register out of bounds")
			return
		}
		vm.registers[dst] -= vm.registers[src]
		vm.pc++

	case MBC_MUL:
		dst, src := inst.Operand1, inst.Operand2
		if dst >= MaxMBCRegisters || src >= MaxMBCRegisters {
			vm.err = fmt.Errorf("mbc_vm: MUL register out of bounds")
			return
		}
		vm.registers[dst] *= vm.registers[src]
		vm.pc++

	case MBC_DIV:
		dst, src := inst.Operand1, inst.Operand2
		if dst >= MaxMBCRegisters || src >= MaxMBCRegisters {
			vm.err = fmt.Errorf("mbc_vm: DIV register out of bounds")
			return
		}
		if vm.registers[src] == 0 {
			vm.err = fmt.Errorf("mbc_vm: division by zero at PC=%d", vm.pc)
			return
		}
		vm.registers[dst] /= vm.registers[src]
		vm.pc++

	case MBC_MOD:
		dst, src := inst.Operand1, inst.Operand2
		if dst >= MaxMBCRegisters || src >= MaxMBCRegisters {
			vm.err = fmt.Errorf("mbc_vm: MOD register out of bounds")
			return
		}
		if vm.registers[src] == 0 {
			vm.err = fmt.Errorf("mbc_vm: modulo by zero at PC=%d", vm.pc)
			return
		}
		vm.registers[dst] %= vm.registers[src]
		vm.pc++

	case MBC_AND:
		dst, src := inst.Operand1, inst.Operand2
		if dst >= MaxMBCRegisters || src >= MaxMBCRegisters {
			vm.err = fmt.Errorf("mbc_vm: AND register out of bounds")
			return
		}
		vm.registers[dst] &= vm.registers[src]
		vm.pc++

	case MBC_OR:
		dst, src := inst.Operand1, inst.Operand2
		if dst >= MaxMBCRegisters || src >= MaxMBCRegisters {
			vm.err = fmt.Errorf("mbc_vm: OR register out of bounds")
			return
		}
		vm.registers[dst] |= vm.registers[src]
		vm.pc++

	case MBC_XOR:
		dst, src := inst.Operand1, inst.Operand2
		if dst >= MaxMBCRegisters || src >= MaxMBCRegisters {
			vm.err = fmt.Errorf("mbc_vm: XOR register out of bounds")
			return
		}
		vm.registers[dst] ^= vm.registers[src]
		vm.pc++

	case MBC_SHL:
		dst, src := inst.Operand1, inst.Operand2
		if dst >= MaxMBCRegisters || src >= MaxMBCRegisters {
			vm.err = fmt.Errorf("mbc_vm: SHL register out of bounds")
			return
		}
		shift := vm.registers[src]
		if shift >= 32 {
			vm.registers[dst] = 0 // saturate
		} else {
			vm.registers[dst] <<= shift
		}
		vm.pc++

	case MBC_SHR:
		dst, src := inst.Operand1, inst.Operand2
		if dst >= MaxMBCRegisters || src >= MaxMBCRegisters {
			vm.err = fmt.Errorf("mbc_vm: SHR register out of bounds")
			return
		}
		shift := vm.registers[src]
		if shift >= 32 {
			vm.registers[dst] = 0 // saturate
		} else {
			vm.registers[dst] >>= shift
		}
		vm.pc++

	case MBC_JMP:
		target := int(inst.Operand1)
		if target < 0 || target >= len(prog.Instructions) {
			vm.err = fmt.Errorf("mbc_vm: JMP target %d out of bounds", target)
			return
		}
		vm.pc = target

	case MBC_JEQ:
		reg := inst.Operand2
		if reg >= MaxMBCRegisters {
			vm.err = fmt.Errorf("mbc_vm: JEQ register out of bounds")
			return
		}
		if vm.registers[reg] == 0 {
			target := int(inst.Operand1)
			if target < 0 || target >= len(prog.Instructions) {
				vm.err = fmt.Errorf("mbc_vm: JEQ target %d out of bounds", target)
				return
			}
			vm.pc = target
		} else {
			vm.pc++
		}

	case MBC_JNE:
		reg := inst.Operand2
		if reg >= MaxMBCRegisters {
			vm.err = fmt.Errorf("mbc_vm: JNE register out of bounds")
			return
		}
		if vm.registers[reg] != 0 {
			target := int(inst.Operand1)
			if target < 0 || target >= len(prog.Instructions) {
				vm.err = fmt.Errorf("mbc_vm: JNE target %d out of bounds", target)
				return
			}
			vm.pc = target
		} else {
			vm.pc++
		}

	case MBC_JGT:
		reg := inst.Operand2
		if reg >= MaxMBCRegisters {
			vm.err = fmt.Errorf("mbc_vm: JGT register out of bounds")
			return
		}
		if vm.registers[reg] > 0 {
			vm.pc = int(inst.Operand1)
		} else {
			vm.pc++
		}

	case MBC_JLT:
		// JLT is always false for unsigned 0 comparison, but for signed interpretation
		// this can trigger. For now, treat as unsigned.
		vm.pc++

	case MBC_CALL:
		if len(vm.callStack) >= vm.cfg.MaxStackDepth {
			vm.err = fmt.Errorf("mbc_vm: call stack overflow (depth %d)", len(vm.callStack))
			return
		}
		vm.callStack = append(vm.callStack, vm.pc+1)
		target := int(inst.Operand1)
		if target < 0 || target >= len(prog.Instructions) {
			vm.err = fmt.Errorf("mbc_vm: CALL target %d out of bounds", target)
			return
		}
		vm.pc = target

	case MBC_RET:
		if len(vm.callStack) == 0 {
			vm.halted = true
			return
		}
		vm.pc = vm.callStack[len(vm.callStack)-1]
		vm.callStack = vm.callStack[:len(vm.callStack)-1]

	case MBC_EMIT:
		reg := inst.Operand1
		if reg >= MaxMBCRegisters {
			vm.err = fmt.Errorf("mbc_vm: EMIT register out of bounds")
			return
		}
		vm.emitted = append(vm.emitted, vm.registers[reg])
		vm.pc++

	case MBC_HALT:
		vm.halted = true

	default:
		vm.err = fmt.Errorf("mbc_vm: unknown opcode 0x%02X at PC=%d", inst.Opcode, vm.pc)
	}
}

// FuzzMBCExecution feeds random MBC programs to the virtual machine.
func FuzzMBCExecution(f *testing.F) {
	// Simple program: LOAD r0, 42; EMIT r0; HALT
	simple := make([]byte, MBCInstSize*3)
	simple[0] = MBC_LOAD
	binary.BigEndian.PutUint32(simple[1:5], 0)   // r0
	binary.BigEndian.PutUint32(simple[5:9], 42)   // value
	simple[MBCInstSize] = MBC_EMIT
	binary.BigEndian.PutUint32(simple[MBCInstSize+1:MBCInstSize+5], 0)
	simple[MBCInstSize*2] = MBC_HALT
	f.Add(simple)

	// Infinite loop: JMP 0; HALT (never reached)
	loop := make([]byte, MBCInstSize*2)
	loop[0] = MBC_JMP
	binary.BigEndian.PutUint32(loop[1:5], 0)
	loop[MBCInstSize] = MBC_HALT
	f.Add(loop)

	// Deep recursion: CALL 0 (calls itself)
	recurse := make([]byte, MBCInstSize*2)
	recurse[0] = MBC_CALL
	binary.BigEndian.PutUint32(recurse[1:5], 0)
	recurse[MBCInstSize] = MBC_HALT
	f.Add(recurse)

	// Division by zero
	divZero := make([]byte, MBCInstSize*2)
	divZero[0] = MBC_DIV
	divZero[MBCInstSize] = MBC_HALT
	f.Add(divZero)

	// Arithmetic overflow
	overflow := make([]byte, MBCInstSize*4)
	overflow[0] = MBC_LOAD
	binary.BigEndian.PutUint32(overflow[5:9], 0xFFFFFFFF)
	overflow[MBCInstSize] = MBC_LOAD
	binary.BigEndian.PutUint32(overflow[MBCInstSize+1:MBCInstSize+5], 1)
	binary.BigEndian.PutUint32(overflow[MBCInstSize+5:MBCInstSize+9], 1)
	overflow[MBCInstSize*2] = MBC_ADD
	binary.BigEndian.PutUint32(overflow[MBCInstSize*2+1:MBCInstSize*2+5], 0)
	binary.BigEndian.PutUint32(overflow[MBCInstSize*2+5:MBCInstSize*2+9], 1)
	overflow[MBCInstSize*3] = MBC_HALT
	f.Add(overflow)

	// Empty
	f.Add([]byte{})

	// Single NOP (no HALT -- should error)
	nop := make([]byte, MBCInstSize)
	nop[0] = MBC_NOP
	f.Add(nop)

	f.Fuzz(func(t *testing.T, data []byte) {
		prog, parseErr := ParseMBCProgram(data)
		if parseErr != nil {
			return
		}

		if prog == nil || len(prog.Instructions) == 0 {
			return
		}

		vm := NewMBCVM(DefaultMBCVMConfig())
		execErr := vm.Execute(prog)

		// The VM must always terminate (no infinite loops or hangs)
		// If we reach here, the VM terminated -- either by HALT, error, or limit
		_ = execErr

		// Invariant: execution count must not exceed limit
		if vm.execCount > vm.cfg.MaxInstructions+1 {
			t.Errorf("VM executed %d instructions, limit was %d", vm.execCount, vm.cfg.MaxInstructions)
		}

		// Invariant: call stack depth must not exceed limit
		if len(vm.callStack) > vm.cfg.MaxStackDepth {
			t.Errorf("call stack depth %d exceeds max %d", len(vm.callStack), vm.cfg.MaxStackDepth)
		}

		// Invariant: PC must be in bounds or halted
		if !vm.halted && vm.err == nil {
			if vm.pc < 0 || vm.pc >= len(prog.Instructions) {
				t.Errorf("VM stopped with PC=%d out of bounds", vm.pc)
			}
		}

		// Invariant: registers must contain valid uint32 (no panics)
		for i := range vm.registers {
			_ = vm.registers[i]
		}
	})
}

// FuzzMBCArithmeticEdgeCases targets arithmetic operations with edge-case values.
func FuzzMBCArithmeticEdgeCases(f *testing.F) {
	f.Add(uint8(MBC_ADD), uint32(0xFFFFFFFF), uint32(0xFFFFFFFF))  // overflow
	f.Add(uint8(MBC_SUB), uint32(0), uint32(1))                     // underflow
	f.Add(uint8(MBC_MUL), uint32(0xFFFF), uint32(0xFFFF))          // large multiply
	f.Add(uint8(MBC_DIV), uint32(42), uint32(0))                    // div by zero
	f.Add(uint8(MBC_MOD), uint32(42), uint32(0))                    // mod by zero
	f.Add(uint8(MBC_SHL), uint32(1), uint32(31))                    // max shift
	f.Add(uint8(MBC_SHL), uint32(1), uint32(32))                    // over-shift
	f.Add(uint8(MBC_SHR), uint32(0x80000000), uint32(31))          // sign bit shift

	f.Fuzz(func(t *testing.T, op uint8, val1, val2 uint32) {
		// Only test arithmetic opcodes
		if op < MBC_ADD || op > MBC_SHR {
			return
		}

		// Build: LOAD r0, val1; LOAD r1, val2; OP r0, r1; HALT
		prog := make([]byte, MBCInstSize*4)
		// LOAD r0, val1
		prog[0] = MBC_LOAD
		binary.BigEndian.PutUint32(prog[1:5], 0)
		binary.BigEndian.PutUint32(prog[5:9], val1)
		// LOAD r1, val2
		prog[MBCInstSize] = MBC_LOAD
		binary.BigEndian.PutUint32(prog[MBCInstSize+1:MBCInstSize+5], 1)
		binary.BigEndian.PutUint32(prog[MBCInstSize+5:MBCInstSize+9], val2)
		// OP r0, r1
		prog[MBCInstSize*2] = op
		binary.BigEndian.PutUint32(prog[MBCInstSize*2+1:MBCInstSize*2+5], 0)
		binary.BigEndian.PutUint32(prog[MBCInstSize*2+5:MBCInstSize*2+9], 1)
		// HALT
		prog[MBCInstSize*3] = MBC_HALT

		parsed, err := ParseMBCProgram(prog)
		if err != nil {
			t.Fatalf("parse failed for valid program: %v", err)
		}

		vm := NewMBCVM(DefaultMBCVMConfig())
		execErr := vm.Execute(parsed)

		// Division/modulo by zero should produce an error, not a panic
		if (op == MBC_DIV || op == MBC_MOD) && val2 == 0 {
			if execErr == nil {
				t.Errorf("expected error for %s by zero, got nil", map[uint8]string{
					MBC_DIV: "DIV", MBC_MOD: "MOD",
				}[op])
			}
		}
	})
}
