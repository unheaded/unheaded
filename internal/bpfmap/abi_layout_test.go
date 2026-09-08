// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package bpfmap

import (
	"testing"
	"unsafe"
)

// TestBPFAttrLayout pins the size and field offsets of the bpf(2) attribute
// mirrors.
//
// These began as anonymous structs written out twice per call site — once for
// the &struct{...} passed to the kernel and once for the unsafe.Sizeof
// argument. Two copies of a kernel ABI layout that must agree is a latent bug:
// edit one and the kernel receives a pointer to a struct whose declared size is
// wrong, which it reads or writes past.
//
// They are now named types used in both places, so the two cannot disagree.
// This test additionally pins the layout itself, because the padding after
// mapFd is not cosmetic — union bpf_attr places the 64-bit pointer fields on an
// 8-byte boundary, and dropping the explicit uint32 pad silently shifts every
// field after it.
func TestBPFAttrLayout(t *testing.T) {
	t.Run("lookup", func(t *testing.T) {
		var a bpfMapLookupAttr
		if got, want := unsafe.Offsetof(a.key), uintptr(8); got != want {
			t.Errorf("key offset = %d, want %d (pointer must be 8-byte aligned)", got, want)
		}
		if got, want := unsafe.Offsetof(a.value), uintptr(16); got != want {
			t.Errorf("value offset = %d, want %d", got, want)
		}
		if got, want := unsafe.Sizeof(a), uintptr(24); got != want {
			t.Errorf("size = %d, want %d", got, want)
		}
	})

	t.Run("update", func(t *testing.T) {
		var a bpfMapUpdateAttr
		if got, want := unsafe.Offsetof(a.key), uintptr(8); got != want {
			t.Errorf("key offset = %d, want %d (explicit uint32 pad after mapFd)", got, want)
		}
		if got, want := unsafe.Offsetof(a.value), uintptr(16); got != want {
			t.Errorf("value offset = %d, want %d", got, want)
		}
		if got, want := unsafe.Offsetof(a.flags), uintptr(24); got != want {
			t.Errorf("flags offset = %d, want %d", got, want)
		}
		if got, want := unsafe.Sizeof(a), uintptr(32); got != want {
			t.Errorf("size = %d, want %d", got, want)
		}
	})

	t.Run("batch", func(t *testing.T) {
		var a bpfMapBatchAttr
		if got, want := unsafe.Offsetof(a.count), uintptr(32); got != want {
			t.Errorf("count offset = %d, want %d", got, want)
		}
		if got, want := unsafe.Offsetof(a.flags), uintptr(48); got != want {
			t.Errorf("flags offset = %d, want %d (explicit pad after mapFd)", got, want)
		}
		if got, want := unsafe.Sizeof(a), uintptr(56); got != want {
			t.Errorf("size = %d, want %d", got, want)
		}
	})

	t.Run("delete", func(t *testing.T) {
		var a bpfMapDeleteAttr
		if got, want := unsafe.Offsetof(a.key), uintptr(8); got != want {
			t.Errorf("key offset = %d, want %d", got, want)
		}
		if got, want := unsafe.Sizeof(a), uintptr(16); got != want {
			t.Errorf("size = %d, want %d", got, want)
		}
	})
}
