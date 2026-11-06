// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package ebpf

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

const mapTypeRingbuf = BPF_MAP_TYPE_RINGBUF

// ErrKeySize is returned when a key's length is not the map's key size.
var ErrKeySize = errors.New("ebpf: key size mismatch")

// ErrValueSize is returned when a value's length is not the map's value size.
var ErrValueSize = errors.New("ebpf: value size mismatch")

// PinnedMap is an open handle on a map pinned in bpffs. It is the in-house
// counterpart of the loader's ReadMap for maps this process did not load
// (DP-211). Not safe for concurrent Close.
type PinnedMap struct {
	fd        int
	keySize   uint32
	valueSize uint32
}

// OpenPinnedMap opens the map pinned at path. The caller must Close it.
func OpenPinnedMap(path string) (*PinnedMap, error) {
	fd, err := bpfObjGet(path)
	if err != nil {
		return nil, err
	}
	info, err := bpfGetMapInfo(fd)
	if err != nil {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("map info %s: %w", path, err)
	}
	if !mapTypeToMapType(info.Type).elemFitsOneBuffer() {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("%w: %s has type %d", ErrUnsupportedMapType, path, info.Type)
	}
	return &PinnedMap{fd: fd, keySize: info.KeySize, valueSize: info.ValueSize}, nil
}

// KeySize returns the map's key size in bytes.
func (m *PinnedMap) KeySize() uint32 { return m.keySize }

// ValueSize returns the map's value size in bytes.
func (m *PinnedMap) ValueSize() uint32 { return m.valueSize }

func (m *PinnedMap) checkKey(key []byte) error {
	if uint32(len(key)) != m.keySize || len(key) == 0 { // #nosec G115 -- a slice length fits uint32 for any key a map accepts
		return fmt.Errorf("%w: got %d, want %d", ErrKeySize, len(key), m.keySize)
	}
	return nil
}

// Lookup returns the value for key, or (nil, nil) when the key is absent.
func (m *PinnedMap) Lookup(key []byte) ([]byte, error) {
	if err := m.checkKey(key); err != nil {
		return nil, err
	}
	value := make([]byte, m.valueSize)
	err := bpfMapLookupElem(m.fd, unsafe.Pointer(&key[0]), unsafe.Pointer(&value[0])) // #nosec G103 -- BPF kernel ABI boundary; no uintptr->Pointer round-trip
	if errors.Is(err, syscall.ENOENT) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return value, nil
}

// Update writes key=value (create or replace).
func (m *PinnedMap) Update(key, value []byte) error {
	if err := m.checkKey(key); err != nil {
		return err
	}
	if uint32(len(value)) != m.valueSize || len(value) == 0 { // #nosec G115 -- a slice length fits uint32 for any value a map accepts
		return fmt.Errorf("%w: got %d, want %d", ErrValueSize, len(value), m.valueSize)
	}
	return bpfMapUpdateElem(m.fd, unsafe.Pointer(&key[0]), unsafe.Pointer(&value[0]), BPF_ANY) // #nosec G103 -- BPF kernel ABI boundary; no uintptr->Pointer round-trip
}

// Delete removes key; an absent key is not an error.
func (m *PinnedMap) Delete(key []byte) error {
	if err := m.checkKey(key); err != nil {
		return err
	}
	err := bpfMapDeleteElem(m.fd, unsafe.Pointer(&key[0])) // #nosec G103 -- BPF kernel ABI boundary; no uintptr->Pointer round-trip
	if errors.Is(err, syscall.ENOENT) {
		return nil
	}
	return err
}

// Iterate calls fn for every entry with copies of the key and value. A non-nil
// error from fn stops the walk and is returned. An entry deleted between the
// key walk and its lookup is skipped.
func (m *PinnedMap) Iterate(fn func(key, value []byte) error) error {
	key := make([]byte, m.keySize)
	next := make([]byte, m.keySize)
	var cur unsafe.Pointer // nil: first key
	for {
		err := bpfMapGetNextKey(m.fd, cur, unsafe.Pointer(&next[0])) // #nosec G103 -- BPF kernel ABI boundary; no uintptr->Pointer round-trip
		if errors.Is(err, syscall.ENOENT) {
			return nil
		}
		if err != nil {
			return err
		}
		copy(key, next)
		cur = unsafe.Pointer(&key[0]) // #nosec G103 -- BPF kernel ABI boundary; no uintptr->Pointer round-trip
		value, err := m.Lookup(key)
		if err != nil {
			return err
		}
		if value == nil {
			continue
		}
		k := make([]byte, len(key))
		copy(k, key)
		if err := fn(k, value); err != nil {
			return err
		}
	}
}

// Close releases the handle. Safe to call twice.
func (m *PinnedMap) Close() error {
	if m.fd < 0 {
		return nil
	}
	err := syscall.Close(m.fd)
	m.fd = -1
	return err
}
