// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build !linux

package ebpf

// PinnedMap is unavailable off Linux.
type PinnedMap struct{}

// OpenPinnedMap returns ErrNotSupported off Linux.
func OpenPinnedMap(string) (*PinnedMap, error) { return nil, ErrNotSupported }

// KeySize returns 0 off Linux.
func (*PinnedMap) KeySize() uint32 { return 0 }

// ValueSize returns 0 off Linux.
func (*PinnedMap) ValueSize() uint32 { return 0 }

// Lookup returns ErrNotSupported off Linux.
func (*PinnedMap) Lookup([]byte) ([]byte, error) { return nil, ErrNotSupported }

// Update returns ErrNotSupported off Linux.
func (*PinnedMap) Update(_, _ []byte) error { return ErrNotSupported }

// Delete returns ErrNotSupported off Linux.
func (*PinnedMap) Delete([]byte) error { return ErrNotSupported }

// Iterate returns ErrNotSupported off Linux.
func (*PinnedMap) Iterate(func(key, value []byte) error) error { return ErrNotSupported }

// Close returns nil off Linux.
func (*PinnedMap) Close() error { return nil }
