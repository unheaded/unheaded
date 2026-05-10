// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux
// +build linux

package ebpf

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

// munmapKernelRegion releases a kernel-owned mmap region.
//
// `addr` is the value returned by an earlier mmap(2) syscall on a BPF map FD.
// It is NOT a Go-managed pointer — the GC has no awareness of the underlying
// memory, so converting via unsafe.Pointer is safe (the standard go-vet
// `unsafeptr` warning is a known false-positive for kernel-owned regions).
//
// We isolate this conversion in a small dedicated function so the lint
// suppression has a single, documented home rather than being scattered
// across two call sites in loader.go.
//
//nolint:govet,gosec // kernel mmap address; GC reachability does not apply.
func munmapKernelRegion(addr uintptr, size int) error {
	if addr == 0 || size == 0 {
		return nil
	}
	mem := unsafe.Slice((*byte)(unsafe.Pointer(addr)), size)
	return unix.Munmap(mem)
}
