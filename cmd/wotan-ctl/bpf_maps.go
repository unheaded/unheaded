// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"encoding/binary"
	"fmt"
	"unsafe"

	"unheaded/pkg/ebpf"
)

// elemMap is the part of a pinned map this tool writes through. *ebpf.PinnedMap
// holds every call to the map's own key and value sizes, because the kernel reads
// those sizes through the buffers whatever the slices hold, and it refuses a map
// whose element does not fit one value's buffer.
type elemMap interface {
	ValueSize() uint32
	Update(key, value []byte) error
	Close() error
}

var _ elemMap = (*ebpf.PinnedMap)(nil)

// BPFMap represents a handle to a pinned BPF map.
type BPFMap struct {
	m    elemMap
	name string
}

// openBPFMap opens the BPF map pinned at pinPath.
//
// Off Linux, or when no map is pinned there, this returns an error, which callers
// take to mean a dry run.
func openBPFMap(pinPath string) (*BPFMap, error) {
	m, err := ebpf.OpenPinnedMap(pinPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get pinned BPF map at %s: %w", pinPath, err)
	}
	return &BPFMap{m: m, name: pinPath}, nil
}

// Update inserts or updates a key-value pair in the BPF map.
// Both key and value are converted to binary using little-endian encoding.
func (m *BPFMap) Update(key interface{}, value interface{}) error {
	if m == nil || m.m == nil {
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

	// A kernel object built without the ascend-linux feature lacks the trailing
	// words of CpuState and is otherwise the same layout.
	if _, ok := value.(CpuState); ok && m.m.ValueSize() == cpuStateSizeNoAscend {
		valueBytes = valueBytes[:cpuStateSizeNoAscend]
	}

	// BPF_ANY: overwrite an existing entry or create a new one.
	return m.m.Update(keyBytes, valueBytes)
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
		binary.LittleEndian.PutUint32(buf[:4], uint32(val)) // #nosec G115 -- a signed value's two's-complement bits, as the map stores them
		return buf[:4], nil
	case int64:
		binary.LittleEndian.PutUint64(buf[:], uint64(val)) // #nosec G115 -- a signed value's two's-complement bits, as the map stores them
		return buf[:], nil
	case []byte:
		return val, nil
	case CpuState:
		// Serialize CpuState struct
		buf := make([]byte, unsafe.Sizeof(CpuState{}))
		ptr := unsafe.Pointer(&val) // #nosec G103 -- reads exactly sizeof(CpuState) bytes of this local copy
		copy(buf, (*[unsafe.Sizeof(CpuState{})]byte)(ptr)[:])
		return buf, nil
	default:
		return nil, fmt.Errorf("unsupported type: %T", v)
	}
}

// Close closes the BPF map file descriptor.
func (m *BPFMap) Close() error {
	if m == nil || m.m == nil {
		return nil
	}
	return m.m.Close()
}
