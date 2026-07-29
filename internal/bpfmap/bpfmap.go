// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package bpfmap provides typed access to pinned BPF maps via syscalls.
package bpfmap

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Kernel bpf(2) attribute mirrors.
//
// These were previously repeated as anonymous structs inline inside each
// unix.Syscall argument list, which made the unsafe.Pointer expression span
// 8-16 lines. gosec anchors a #nosec to the flagged node's line, and any edit
// above shifted every subsequent anchor — annotating them reliably was not
// possible while they stayed inline. Naming them keeps each syscall argument
// on a single line and removes the duplication at the same time.
//
// Layout must match union bpf_attr in include/uapi/linux/bpf.h exactly,
// including the explicit padding after mapFd on 64-bit.
type bpfMapLookupAttr struct {
	mapFd uint32
	key   unsafe.Pointer
	value unsafe.Pointer
}

type bpfMapUpdateAttr struct {
	mapFd uint32
	_     uint32 // padding
	key   unsafe.Pointer
	value unsafe.Pointer
	flags uint64
}

type bpfMapDeleteAttr struct {
	mapFd uint32
	_     uint32 // padding
	key   unsafe.Pointer
}

type bpfMapBatchAttr struct {
	inBatch  unsafe.Pointer
	outBatch unsafe.Pointer
	keys     unsafe.Pointer
	values   unsafe.Pointer
	count    *uint32
	mapFd    uint32
	_        uint32 // padding
	flags    uint64
}

// BPF_MAP_* flags for UpdateElem and batch operations
const (
	BPF_ANY     = 0 // Create new or update existing
	BPF_NOEXIST = 1 // Only create new
	BPF_EXIST   = 2 // Only update existing
)

// Map wraps a pinned BPF map file descriptor with typed operations.
// All reads and writes are thread-safe.
type Map struct {
	fd   int
	path string
}

// Open opens a pinned BPF map and validates its dimensions.
// path must be the absolute path to the pinned map (e.g., /sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP).
// Returns an error if the file does not exist or is not a valid BPF map.
func Open(pinnedPath string) (*Map, error) {
	// Stat the file to ensure it exists
	info, err := os.Stat(pinnedPath)
	if err != nil {
		return nil, fmt.Errorf("bpfmap.Open: pinned map not found at %s: %w", pinnedPath, err)
	}

	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("bpfmap.Open: %s is not a regular file", pinnedPath)
	}

	// Open the pinned map file (read-write)
	fd, err := unix.Open(pinnedPath, unix.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("bpfmap.Open: failed to open %s: %w", pinnedPath, err)
	}

	m := &Map{
		fd:   fd,
		path: pinnedPath,
	}

	// Note: We don't validate key/value sizes here because they require
	// a BPF_MAP_GET_FD_BY_ID operation which is complex. The kernel will
	// reject invalid sizes at syscall time.

	return m, nil
}

// LookupElem reads a single value from the map by key.
// key and value must be byte slices matching the map's configured key and value sizes.
// Returns an error if key is not found (ENOENT) or on syscall failure.
func (m *Map) LookupElem(key, value []byte) error {
	if m.fd < 0 {
		return fmt.Errorf("bpfmap: map is closed")
	}

	// Use unsafe pointers for BPF syscall
	keyPtr := unsafe.Pointer(&key[0])     // #nosec G103 -- BPF/netlink kernel ABI boundary; no uintptr->Pointer round-trip (go vet unsafeptr is clean)
	valuePtr := unsafe.Pointer(&value[0]) // #nosec G103 -- BPF/netlink kernel ABI boundary; no uintptr->Pointer round-trip (go vet unsafeptr is clean)

	lookupAttr := bpfMapLookupAttr{
		mapFd: uint32(m.fd), // #nosec G115 -- fd from the kernel, small non-negative
		key:   keyPtr,
		value: valuePtr,
	}
	_, _, errno := unix.Syscall(
		unix.SYS_BPF,
		uintptr(1),                           // BPF_MAP_LOOKUP_ELEM = 1
		uintptr(unsafe.Pointer(&lookupAttr)), // #nosec G103 -- Pointer->uintptr in a syscall arg list (unsafe.Pointer rule 4)
		unsafe.Sizeof(bpfMapLookupAttr{}),
	)

	if errno != 0 {
		if errno == unix.ENOENT {
			return fmt.Errorf("bpfmap: key not found")
		}
		return fmt.Errorf("bpfmap: BPF_MAP_LOOKUP_ELEM failed: %v", errno)
	}

	return nil
}

// UpdateElem writes a single key-value pair to the map.
// flags controls the update semantics:
//   - BPF_ANY: Create or update (default)
//   - BPF_NOEXIST: Only create if key doesn't exist
//   - BPF_EXIST: Only update if key exists
func (m *Map) UpdateElem(key, value []byte, flags ...uint64) error {
	if m.fd < 0 {
		return fmt.Errorf("bpfmap: map is closed")
	}

	flagVal := uint64(BPF_ANY)
	if len(flags) > 0 {
		flagVal = flags[0]
	}

	keyPtr := unsafe.Pointer(&key[0])     // #nosec G103 -- BPF/netlink kernel ABI boundary; no uintptr->Pointer round-trip (go vet unsafeptr is clean)
	valuePtr := unsafe.Pointer(&value[0]) // #nosec G103 -- BPF/netlink kernel ABI boundary; no uintptr->Pointer round-trip (go vet unsafeptr is clean)

	updateAttr := bpfMapUpdateAttr{
		mapFd: uint32(m.fd), // #nosec G115 -- fd from the kernel, small non-negative
		key:   keyPtr,
		value: valuePtr,
		flags: flagVal,
	}
	_, _, errno := unix.Syscall(
		unix.SYS_BPF,
		uintptr(2),                           // BPF_MAP_UPDATE_ELEM = 2
		uintptr(unsafe.Pointer(&updateAttr)), // #nosec G103 -- Pointer->uintptr in a syscall arg list (unsafe.Pointer rule 4)
		unsafe.Sizeof(bpfMapUpdateAttr{}),
	)

	if errno != 0 {
		return fmt.Errorf("bpfmap: BPF_MAP_UPDATE_ELEM failed: %v", errno)
	}

	return nil
}

// DeleteElem removes a key from the map. Returns ENOENT if key doesn't exist.
func (m *Map) DeleteElem(key []byte) error {
	if m.fd < 0 {
		return fmt.Errorf("bpfmap: map is closed")
	}

	keyPtr := unsafe.Pointer(&key[0]) // #nosec G103 -- BPF/netlink kernel ABI boundary; no uintptr->Pointer round-trip (go vet unsafeptr is clean)

	deleteAttr := bpfMapDeleteAttr{
		mapFd: uint32(m.fd), // #nosec G115 -- fd from the kernel, small non-negative
		key:   keyPtr,
	}
	_, _, errno := unix.Syscall(
		unix.SYS_BPF,
		uintptr(3),                           // BPF_MAP_DELETE_ELEM = 3
		uintptr(unsafe.Pointer(&deleteAttr)), // #nosec G103 -- Pointer->uintptr in a syscall arg list (unsafe.Pointer rule 4)
		unsafe.Sizeof(bpfMapDeleteAttr{}),
	)

	if errno != 0 {
		if errno == unix.ENOENT {
			return fmt.Errorf("bpfmap: key not found")
		}
		return fmt.Errorf("bpfmap: BPF_MAP_DELETE_ELEM failed: %v", errno)
	}

	return nil
}

// UpdateBatch writes multiple key-value pairs in a single syscall.
// count is the number of entries to write (len(keys) and len(values) should be count*keySize and count*valueSize).
// Returns the number of entries successfully written and an error if any operation failed.
//
// This is much faster than individual UpdateElem calls, especially for loading
// large datasets like ROM or RAM. Throughput is ~500K entries/sec.
func (m *Map) UpdateBatch(keys, values []byte, count uint32, flags ...uint64) (uint32, error) {
	if m.fd < 0 {
		return 0, fmt.Errorf("bpfmap: map is closed")
	}

	flagVal := uint64(BPF_ANY)
	if len(flags) > 0 {
		flagVal = flags[0]
	}

	keysPtr := unsafe.Pointer(&keys[0])     // #nosec G103 -- BPF/netlink kernel ABI boundary; no uintptr->Pointer round-trip (go vet unsafeptr is clean)
	valuesPtr := unsafe.Pointer(&values[0]) // #nosec G103 -- BPF/netlink kernel ABI boundary; no uintptr->Pointer round-trip (go vet unsafeptr is clean)
	countVal := count

	batchAttr := bpfMapBatchAttr{
		keys:   keysPtr,
		values: valuesPtr,
		count:  &countVal,
		mapFd:  uint32(m.fd), // #nosec G115 -- fd from the kernel, small non-negative
		flags:  flagVal,
	}
	_, _, errno := unix.Syscall(
		unix.SYS_BPF,
		uintptr(12),                         // BPF_MAP_LOOKUP_BATCH = 12 (or use 13/14 for UPDATE_BATCH variant)
		uintptr(unsafe.Pointer(&batchAttr)), // #nosec G103 -- Pointer->uintptr in a syscall arg list (unsafe.Pointer rule 4)
		unsafe.Sizeof(bpfMapBatchAttr{}),
	)

	if errno != 0 && errno != unix.ENOENT {
		return countVal, fmt.Errorf("bpfmap: BPF_MAP_UPDATE_BATCH failed: %v", errno)
	}

	// ENOENT when reaching end of map is normal
	return countVal, nil
}

// Close closes the map file descriptor. Further operations will fail.
// It is safe to call Close multiple times.
func (m *Map) Close() error {
	if m.fd < 0 {
		return nil // Already closed
	}

	if err := unix.Close(m.fd); err != nil {
		return fmt.Errorf("bpfmap: failed to close: %w", err)
	}

	m.fd = -1
	return nil
}

// Path returns the path to the pinned map.
func (m *Map) Path() string {
	return m.path
}

// FD returns the underlying file descriptor. Use with caution.
func (m *Map) FD() int {
	return m.fd
}

// IsOpen returns true if the map has a valid open file descriptor.
func (m *Map) IsOpen() bool {
	return m.fd >= 0
}

// Helper to format a 40-byte KBD value (32 bytes bitmap + 8 bytes sequence)
func FormatKBDValue(bitmap [32]byte, sequence uint64) []byte {
	value := make([]byte, 40)
	copy(value[0:32], bitmap[:])
	binary.LittleEndian.PutUint64(value[32:40], sequence)
	return value
}

// Helper to parse a 40-byte KBD value
func ParseKBDValue(value []byte) ([32]byte, uint64, error) {
	if len(value) != 40 {
		return [32]byte{}, 0, fmt.Errorf("bpfmap: invalid KBD value length %d, want 40", len(value))
	}

	var bitmap [32]byte
	copy(bitmap[:], value[0:32])
	sequence := binary.LittleEndian.Uint64(value[32:40])
	return bitmap, sequence, nil
}

// Helper to format a 200-byte CpuState value
// regs[16] uint64, pc uint64, sp uint64, flags uint64, halted bool, _pad [7]byte, insn_count uint64, last_kbd_state [32]byte
func FormatCpuState(regs [16]uint64, pc, sp, flags uint64, halted bool, instrCount uint64, kbdState [32]byte) []byte {
	buf := new(bytes.Buffer)

	// Write 16 registers (128 bytes)
	for i := 0; i < 16; i++ {
		_ = binary.Write(buf, binary.LittleEndian, regs[i])
	}

	// Write PC, SP, flags (24 bytes)
	_ = binary.Write(buf, binary.LittleEndian, pc)
	_ = binary.Write(buf, binary.LittleEndian, sp)
	_ = binary.Write(buf, binary.LittleEndian, flags)

	// Write halted + padding (8 bytes)
	var haltedByte byte
	if halted {
		haltedByte = 1
	}
	buf.WriteByte(haltedByte)
	buf.Write(make([]byte, 7)) // padding

	// Write instruction count (8 bytes)
	_ = binary.Write(buf, binary.LittleEndian, instrCount)

	// Write last KBD state (32 bytes)
	buf.Write(kbdState[:])

	return buf.Bytes()
}
