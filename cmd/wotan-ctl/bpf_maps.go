package main

import (
	"encoding/binary"
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

// BPFMap represents a handle to a BPF map (either pinned or freshly opened).
type BPFMap struct {
	fd   int
	name string
}

// bpfObjGet opens a pinned BPF object (map/program) by its path using
// the raw BPF_OBJ_GET syscall (command 7).
func bpfObjGet(pinPath string) (int, error) {
	pathBytes, err := unix.BytePtrFromString(pinPath)
	if err != nil {
		return -1, fmt.Errorf("invalid path: %w", err)
	}

	// BPF_OBJ_GET attr: { pathname (u64), bpf_fd (u32), file_flags (u32) }
	attr := struct {
		pathname  uint64
		bpfFd     uint32
		fileFlags uint32
	}{
		pathname: uint64(uintptr(unsafe.Pointer(pathBytes))),
	}

	const BPF_OBJ_GET = 7
	fd, _, errno := unix.Syscall(unix.SYS_BPF, BPF_OBJ_GET, uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr))
	if errno != 0 {
		return -1, errno
	}
	return int(fd), nil
}

// openBPFMap opens a BPF map by trying multiple methods:
// 1. First try to open as a pinned map at the given path
// 2. If not pinned, return an error indicating the map couldn't be accessed
//
// On non-Linux systems, this gracefully returns an error.
func openBPFMap(pinPath string) (*BPFMap, error) {
	// Try to open the pinned map file
	fd, err := bpfObjGet(pinPath)
	if err != nil {
		// Map not pinned or not accessible
		return nil, fmt.Errorf("failed to get pinned BPF map at %s: %v", pinPath, err)
	}

	return &BPFMap{
		fd:   fd,
		name: pinPath,
	}, nil
}

// Update inserts or updates a key-value pair in the BPF map.
// Both key and value are converted to binary using little-endian encoding.
func (m *BPFMap) Update(key interface{}, value interface{}) error {
	if m == nil || m.fd < 0 {
		return fmt.Errorf("BPF map not initialized")
	}

	// Encode key to bytes
	keyBytes, err := encodeValue(key)
	if err != nil {
		return fmt.Errorf("encode key: %w", err)
	}

	// Encode value to bytes
	valueBytes, err := encodeValue(value)
	if err != nil {
		return fmt.Errorf("encode value: %w", err)
	}

	// Call BPF_MAP_UPDATE_ELEM
	return m.updateElem(keyBytes, valueBytes)
}

// updateElem performs the BPF_MAP_UPDATE_ELEM syscall.
func (m *BPFMap) updateElem(key, value []byte) error {
	attr := struct {
		mapFd uint32
		key   uint64
		value uint64
		flags uint64
	}{
		mapFd: uint32(m.fd),
		key:   uint64(uintptr(unsafe.Pointer(&key[0]))),
		value: uint64(uintptr(unsafe.Pointer(&value[0]))),
		flags: 0, // BPF_ANY (overwrite existing or create new)
	}

	// BPF syscall (number 321 on x86_64)
	// First arg: command (BPF_MAP_UPDATE_ELEM = 2)
	// Second arg: pointer to attr struct
	// Third arg: size of attr struct
	_, _, errno := unix.Syscall(unix.SYS_BPF, 2, uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr))
	if errno != 0 {
		return fmt.Errorf("BPF_MAP_UPDATE_ELEM: %w", errno)
	}

	return nil
}

// Lookup retrieves a value from the BPF map by key.
// Returns (value bytes, found, error).
func (m *BPFMap) Lookup(key interface{}) ([]byte, bool, error) {
	if m == nil || m.fd < 0 {
		return nil, false, fmt.Errorf("BPF map not initialized")
	}

	// Encode key to bytes
	keyBytes, err := encodeValue(key)
	if err != nil {
		return nil, false, fmt.Errorf("encode key: %w", err)
	}

	// Allocate space for value (assuming 8 bytes max for u32 values)
	// In production, you'd want to know the map's value size ahead of time
	value := make([]byte, 8)

	attr := struct {
		mapFd uint32
		key   uint64
		value uint64
		pad1  uint32
		pad2  uint32
	}{
		mapFd: uint32(m.fd),
		key:   uint64(uintptr(unsafe.Pointer(&keyBytes[0]))),
		value: uint64(uintptr(unsafe.Pointer(&value[0]))),
	}

	// BPF_MAP_LOOKUP_ELEM = 1
	_, _, errno := unix.Syscall(unix.SYS_BPF, 1, uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr))

	if errno == unix.ENOENT {
		return nil, false, nil
	} else if errno != 0 {
		return nil, false, fmt.Errorf("BPF_MAP_LOOKUP_ELEM: %w", errno)
	}

	return value, true, nil
}

// encodeValue converts a Go value to bytes using little-endian encoding.
func encodeValue(v interface{}) ([]byte, error) {
	var buf [8]byte

	switch val := v.(type) {
	case uint32:
		binary.LittleEndian.PutUint32(buf[:4], val)
		return buf[:4], nil
	case uint64:
		binary.LittleEndian.PutUint64(buf[:], val)
		return buf[:], nil
	case int32:
		binary.LittleEndian.PutUint32(buf[:4], uint32(val))
		return buf[:4], nil
	case int64:
		binary.LittleEndian.PutUint64(buf[:], uint64(val))
		return buf[:], nil
	case []byte:
		return val, nil
	case CpuState:
		// Serialize CpuState struct
		buf := make([]byte, unsafe.Sizeof(CpuState{}))
		ptr := unsafe.Pointer(&val)
		copy(buf, (*[unsafe.Sizeof(CpuState{})]byte)(ptr)[:])
		return buf, nil
	default:
		return nil, fmt.Errorf("unsupported type: %T", v)
	}
}

// Close closes the BPF map file descriptor.
func (m *BPFMap) Close() error {
	if m == nil || m.fd < 0 {
		return nil
	}
	return unix.Close(m.fd)
}
