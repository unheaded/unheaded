// Package main provides BPF map access for the doom-bridge.
// This file implements raw BPF syscall wrappers for reading screen framebuffer,
// CPU state, stats, and writing keyboard input to pinned BPF maps.
package main

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
// Total size: 104 bytes.
type CpuState struct {
	Regs        [16]uint32 // offset 0:  64 bytes (16 x u32)
	PC          uint32     // offset 64: 4 bytes
	Flags       uint8      // offset 68: 1 byte
	Halted      uint8      // offset 69: 1 byte
	Stalled     uint8      // offset 70: 1 byte
	Pad         uint8      // offset 71: 1 byte
	SleepUntil  uint64     // offset 72: 8 bytes
	InsnCount   uint64     // offset 80: 8 bytes
	CacheHits   uint64     // offset 88: 8 bytes
	CacheMisses uint64     // offset 96: 8 bytes
}

// cpuStateSize is the expected binary size of CpuState.
const cpuStateSize = 104

// BPFMap represents a handle to a pinned BPF map.
type BPFMap struct {
	fd   int
	name string
}

// bpfObjGet opens a pinned BPF object by filesystem path using the raw
// BPF_OBJ_GET syscall (command 7).
func bpfObjGet(pinPath string) (int, error) {
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

// openBPFMap opens a pinned BPF map at the given path.
func openBPFMap(pinPath string) (*BPFMap, error) {
	fd, err := bpfObjGet(pinPath)
	if err != nil {
		return nil, err
	}
	return &BPFMap{fd: fd, name: pinPath}, nil
}

// Close closes the BPF map file descriptor.
func (m *BPFMap) Close() error {
	if m == nil || m.fd < 0 {
		return nil
	}
	return unix.Close(m.fd)
}

// LookupElem reads a single element from the BPF map.
// keyBytes must be pre-encoded. valueSize determines the output buffer size.
func (m *BPFMap) LookupElem(keyBytes []byte, valueSize int) ([]byte, error) {
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
func (m *BPFMap) UpdateElem(keyBytes, valueBytes []byte) error {
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

// LookupBatch reads multiple elements from the BPF map in a single syscall.
// Returns (keys, values, count, error). On partial success returns what was read.
func (m *BPFMap) LookupBatch(count uint32, keySize, valueSize int) ([]byte, []byte, uint32, error) {
	if m == nil || m.fd < 0 {
		return nil, nil, 0, fmt.Errorf("BPF map not initialized")
	}

	keys := make([]byte, int(count)*keySize)
	values := make([]byte, int(count)*valueSize)
	var inBatch, outBatch uint64
	readCount := count

	// BPF_MAP_LOOKUP_BATCH attr layout:
	// struct {
	//     __aligned_u64 in_batch;    // offset 0
	//     __aligned_u64 out_batch;   // offset 8
	//     __aligned_u64 keys;        // offset 16
	//     __aligned_u64 values;      // offset 24
	//     __u32         count;       // offset 32
	//     __u32         map_fd;      // offset 36
	//     __u64         elem_flags;  // offset 40
	//     __u64         flags;       // offset 48
	// }
	attr := struct {
		inBatch   uint64
		outBatch  uint64
		keys      uint64
		values    uint64
		count     uint32
		mapFd     uint32
		elemFlags uint64
		flags     uint64
	}{
		inBatch:  uint64(uintptr(unsafe.Pointer(&inBatch))),
		outBatch: uint64(uintptr(unsafe.Pointer(&outBatch))),
		keys:     uint64(uintptr(unsafe.Pointer(&keys[0]))),
		values:   uint64(uintptr(unsafe.Pointer(&values[0]))),
		count:    readCount,
		mapFd:    uint32(m.fd),
	}

	_, _, errno := unix.Syscall(unix.SYS_BPF, bpfMapLookupBatch, uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr))
	// ENOENT means we read everything (end of map). That is success.
	if errno != 0 && errno != unix.ENOENT {
		return nil, nil, 0, fmt.Errorf("BPF_MAP_LOOKUP_BATCH: %w", errno)
	}
	return keys, values, attr.count, nil
}

// readScreenBatch reads the entire SCREEN_MAP using batch lookup.
// Returns 64000 bytes of pixel data (320x200, 8-bit palette indices).
func readScreenBatch(screenMap *BPFMap) ([]byte, error) {
	const screenSize = 64000
	_, values, count, err := screenMap.LookupBatch(screenSize, 4, 1)
	if err != nil {
		return nil, err
	}
	if count < screenSize {
		// Pad remaining with zeros
		result := make([]byte, screenSize)
		copy(result, values[:count])
		return result, nil
	}
	return values[:screenSize], nil
}

// readScreenIndividual reads the SCREEN_MAP one element at a time.
// Fallback for kernels without batch support.
func readScreenIndividual(screenMap *BPFMap) ([]byte, error) {
	const screenSize = 64000
	pixels := make([]byte, screenSize)
	keyBuf := make([]byte, 4)

	for i := uint32(0); i < screenSize; i++ {
		binary.LittleEndian.PutUint32(keyBuf, i)
		val, err := screenMap.LookupElem(keyBuf, 1)
		if err != nil {
			// On error, leave as zero (black pixel)
			continue
		}
		pixels[i] = val[0]
	}
	return pixels, nil
}

// readStatsMap reads the 4 stats counters from the STATS map.
// Keys: 0=packets_total, 1=cpu_ticks, 2=insns_executed, 3=halted
func readStatsMap(statsMap *BPFMap) (packets, ticks, insns, halted uint64, err error) {
	keyBuf := make([]byte, 4)

	for i, ptr := range []*uint64{&packets, &ticks, &insns, &halted} {
		binary.LittleEndian.PutUint32(keyBuf, uint32(i))
		val, lookupErr := statsMap.LookupElem(keyBuf, 8)
		if lookupErr != nil {
			continue // leave as zero
		}
		*ptr = binary.LittleEndian.Uint64(val)
	}
	return
}

// readCpuMap reads the CPU state from CPU_MAP at key 0.
func readCpuMap(cpuMap *BPFMap) (*CpuState, error) {
	keyBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(keyBuf, 0)

	val, err := cpuMap.LookupElem(keyBuf, cpuStateSize)
	if err != nil {
		return nil, err
	}

	return decodeCpuState(val), nil
}

// decodeCpuState decodes raw bytes into a CpuState struct.
func decodeCpuState(data []byte) *CpuState {
	if len(data) < cpuStateSize {
		padded := make([]byte, cpuStateSize)
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
	return cpu
}

// writeKbdMap writes a key event to KBD_MAP[0].
// Encoding: (scancode << 1) | pressed_flag
func writeKbdMap(kbdMap *BPFMap, scancode uint16, pressed bool) error {
	keyBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(keyBuf, 0) // KBD_MAP index 0

	var pressedBit uint32
	if pressed {
		pressedBit = 1
	}
	value := (uint32(scancode) << 1) | pressedBit

	valBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(valBuf, value)

	return kbdMap.UpdateElem(keyBuf, valBuf)
}
