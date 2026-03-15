// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package bpf provides shared BPF map access primitives for Unheaded services.
//
// It implements raw BPF syscall wrappers for reading and writing pinned BPF maps
// without depending on external libraries. Used by doom-bridge, doom-loader, and
// other tools that interact with the Doom-over-IPv6 compute ring.
package bpf

import (
	"encoding/binary"
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

// BPF syscall commands.
const (
	bpfMapLookupElem  = 1
	bpfMapUpdateElem  = 2
	bpfObjGetCmd      = 7
	bpfMapLookupBatch = 24
)

// CpuState represents the CPU state stored in BPF cpu_map.
// Field order and padding must match MbcCpuState in ebpf/monad-common/src/lib.rs.
// Total size: 120 bytes.
type CpuState struct {
	Regs              [16]uint32 // offset 0:   64 bytes (16 x u32)
	PC                uint32     // offset 64:  4 bytes
	Flags             uint8      // offset 68:  1 byte
	Halted            uint8      // offset 69:  1 byte
	Stalled           uint8      // offset 70:  1 byte
	Pad               uint8      // offset 71:  1 byte
	SleepUntil        uint64     // offset 72:  8 bytes
	InsnCount         uint64     // offset 80:  8 bytes
	CacheHits         uint64     // offset 88:  8 bytes
	CacheMisses       uint64     // offset 96:  8 bytes
	InterruptPending  uint8      // offset 104: 1 byte
	InterruptVector   uint8      // offset 105: 1 byte
	InterruptsEnabled uint8      // offset 106: 1 byte
	Pad2              uint8      // offset 107: 1 byte
	TickCounter       uint32     // offset 108: 4 bytes
	ProgramBreak      uint32     // offset 112: 4 bytes
	ExitCode          uint32     // offset 116: 4 bytes
}

// CpuStateSize is the expected binary size of CpuState.
const CpuStateSize = 120

// Map represents a handle to a pinned BPF map.
type Map struct {
	fd   int
	name string
}

// BpfObjGet opens a pinned BPF object by filesystem path using the raw
// BPF_OBJ_GET syscall (command 7).
func BpfObjGet(pinPath string) (int, error) {
	pathBytes, err := unix.BytePtrFromString(pinPath)
	if err != nil {
		return -1, fmt.Errorf("invalid path: %w", err)
	}

	attr := struct {
		pathname  uint64
		bpfFd     uint32
		fileFlags uint32
	}{
		pathname: uint64(uintptr(unsafe.Pointer(pathBytes))),
	}

	fd, _, errno := unix.Syscall(unix.SYS_BPF, bpfObjGetCmd, uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr))
	if errno != 0 {
		return -1, fmt.Errorf("BPF_OBJ_GET(%s): %w", pinPath, errno)
	}
	return int(fd), nil
}

// OpenMap opens a pinned BPF map at the given path.
func OpenMap(pinPath string) (*Map, error) {
	fd, err := BpfObjGet(pinPath)
	if err != nil {
		return nil, err
	}
	return &Map{fd: fd, name: pinPath}, nil
}

// Fd returns the underlying file descriptor.
func (m *Map) Fd() int {
	if m == nil {
		return -1
	}
	return m.fd
}

// Close closes the BPF map file descriptor.
func (m *Map) Close() error {
	if m == nil || m.fd < 0 {
		return nil
	}
	return unix.Close(m.fd)
}

// LookupElem reads a single element from the BPF map.
// keyBytes must be pre-encoded. valueSize determines the output buffer size.
func (m *Map) LookupElem(keyBytes []byte, valueSize int) ([]byte, error) {
	if m == nil || m.fd < 0 {
		return nil, fmt.Errorf("BPF map not initialized")
	}

	value := make([]byte, valueSize)

	attr := struct {
		mapFd uint32
		pad0  uint32
		key   uint64
		value uint64
	}{
		mapFd: uint32(m.fd),
		key:   uint64(uintptr(unsafe.Pointer(&keyBytes[0]))),
		value: uint64(uintptr(unsafe.Pointer(&value[0]))),
	}

	_, _, errno := unix.Syscall(unix.SYS_BPF, bpfMapLookupElem, uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr))
	if errno != 0 {
		return nil, fmt.Errorf("BPF_MAP_LOOKUP_ELEM: %w", errno)
	}
	return value, nil
}

// UpdateElem writes a single element to the BPF map.
func (m *Map) UpdateElem(keyBytes, valueBytes []byte) error {
	if m == nil || m.fd < 0 {
		return fmt.Errorf("BPF map not initialized")
	}

	attr := struct {
		mapFd uint32
		pad0  uint32
		key   uint64
		value uint64
		flags uint64
	}{
		mapFd: uint32(m.fd),
		key:   uint64(uintptr(unsafe.Pointer(&keyBytes[0]))),
		value: uint64(uintptr(unsafe.Pointer(&valueBytes[0]))),
		flags: 0, // BPF_ANY
	}

	_, _, errno := unix.Syscall(unix.SYS_BPF, bpfMapUpdateElem, uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr))
	if errno != 0 {
		return fmt.Errorf("BPF_MAP_UPDATE_ELEM: %w", errno)
	}
	return nil
}

// LookupBatch reads multiple elements from the BPF map using iterated batch
// syscalls. The kernel may return fewer entries than requested per call, so
// this function loops until all entries are read or the map is exhausted.
// Returns (keys, values, count, error). On partial success returns what was read.
func (m *Map) LookupBatch(count uint32, keySize, valueSize int) ([]byte, []byte, uint32, error) {
	if m == nil || m.fd < 0 {
		return nil, nil, 0, fmt.Errorf("BPF map not initialized")
	}

	keys := make([]byte, int(count)*keySize)
	values := make([]byte, int(count)*valueSize)
	var inBatch, outBatch uint64
	totalRead := uint32(0)
	firstCall := true

	type batchAttr struct {
		inBatch   uint64
		outBatch  uint64
		keys      uint64
		values    uint64
		count     uint32
		mapFd     uint32
		elemFlags uint64
		flags     uint64
	}

	for totalRead < count {
		remaining := count - totalRead
		kOff := int(totalRead) * keySize
		vOff := int(totalRead) * valueSize

		var inPtr uint64
		if firstCall {
			inPtr = uint64(uintptr(unsafe.Pointer(&inBatch)))
		} else {
			inPtr = uint64(uintptr(unsafe.Pointer(&outBatch)))
		}

		attr := batchAttr{
			inBatch:  inPtr,
			outBatch: uint64(uintptr(unsafe.Pointer(&outBatch))),
			keys:     uint64(uintptr(unsafe.Pointer(&keys[kOff]))),
			values:   uint64(uintptr(unsafe.Pointer(&values[vOff]))),
			count:    remaining,
			mapFd:    uint32(m.fd),
		}

		_, _, errno := unix.Syscall(unix.SYS_BPF, bpfMapLookupBatch, uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr))
		if errno == unix.ENOENT {
			// End of map — add what was read and return.
			totalRead += attr.count
			break
		}
		if errno != 0 {
			if totalRead > 0 {
				// Partial success — return what we have.
				break
			}
			return nil, nil, 0, fmt.Errorf("BPF_MAP_LOOKUP_BATCH: %w", errno)
		}
		if attr.count == 0 {
			break // No progress — avoid infinite loop.
		}
		totalRead += attr.count
		firstCall = false
	}

	return keys, values, totalRead, nil
}

// DecodeCpuState decodes raw bytes into a CpuState struct.
func DecodeCpuState(data []byte) *CpuState {
	if len(data) < CpuStateSize {
		padded := make([]byte, CpuStateSize)
		copy(padded, data)
		data = padded
	}

	cpu := &CpuState{}
	for i := 0; i < 16; i++ {
		cpu.Regs[i] = binary.LittleEndian.Uint32(data[i*4 : i*4+4])
	}
	cpu.PC = binary.LittleEndian.Uint32(data[64:68])
	cpu.Flags = data[68]
	cpu.Halted = data[69]
	cpu.Stalled = data[70]
	cpu.Pad = data[71]
	cpu.SleepUntil = binary.LittleEndian.Uint64(data[72:80])
	cpu.InsnCount = binary.LittleEndian.Uint64(data[80:88])
	cpu.CacheHits = binary.LittleEndian.Uint64(data[88:96])
	cpu.CacheMisses = binary.LittleEndian.Uint64(data[96:104])
	cpu.InterruptPending = data[104]
	cpu.InterruptVector = data[105]
	cpu.InterruptsEnabled = data[106]
	cpu.Pad2 = data[107]
	cpu.TickCounter = binary.LittleEndian.Uint32(data[108:112])
	cpu.ProgramBreak = binary.LittleEndian.Uint32(data[112:116])
	cpu.ExitCode = binary.LittleEndian.Uint32(data[116:120])
	return cpu
}

// EncodeCpuState encodes a CpuState struct to raw bytes.
func EncodeCpuState(cpu *CpuState) []byte {
	data := make([]byte, CpuStateSize)
	for i := 0; i < 16; i++ {
		binary.LittleEndian.PutUint32(data[i*4:i*4+4], cpu.Regs[i])
	}
	binary.LittleEndian.PutUint32(data[64:68], cpu.PC)
	data[68] = cpu.Flags
	data[69] = cpu.Halted
	data[70] = cpu.Stalled
	data[71] = cpu.Pad
	binary.LittleEndian.PutUint64(data[72:80], cpu.SleepUntil)
	binary.LittleEndian.PutUint64(data[80:88], cpu.InsnCount)
	binary.LittleEndian.PutUint64(data[88:96], cpu.CacheHits)
	binary.LittleEndian.PutUint64(data[96:104], cpu.CacheMisses)
	data[104] = cpu.InterruptPending
	data[105] = cpu.InterruptVector
	data[106] = cpu.InterruptsEnabled
	data[107] = cpu.Pad2
	binary.LittleEndian.PutUint32(data[108:112], cpu.TickCounter)
	binary.LittleEndian.PutUint32(data[112:116], cpu.ProgramBreak)
	binary.LittleEndian.PutUint32(data[116:120], cpu.ExitCode)
	return data
}
