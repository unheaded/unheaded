// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux
// +build linux

package ebpf

import (
	"golang.org/x/sys/unix"
)

// munmapKernelRegion releases a kernel-owned mmap region.
//
// `addr` is the value returned by an earlier mmap(2) syscall on a BPF map FD.
// It is NOT a Go-managed pointer — the GC has no awareness of the underlying
// memory.
//
// This previously built a []byte via
//
//	unsafe.Slice((*byte)(unsafe.Pointer(addr)), size)
//
// purely to satisfy unix.Munmap's signature, and carried a
// `//nolint:govet,gosec` comment to silence the resulting unsafeptr warning.
// That suppression never worked: `//nolint` is golangci-lint syntax and
// `go vet` does not parse it, so the warning had been failing `go vet` — and
// the pre-commit hook — the entire time it was there.
//
// Rather than hunt for a directive that does suppress it, the conversion is
// gone. munmap(2) takes an address and a length, so the syscall can be
// invoked with the raw uintptr directly: no unsafe.Pointer, nothing for
// unsafeptr to flag, and no reliance on a lint escape hatch.
func munmapKernelRegion(addr uintptr, size int) error {
	if addr == 0 || size == 0 {
		return nil
	}
	if _, _, errno := unix.Syscall(unix.SYS_MUNMAP, addr, uintptr(size), 0); errno != 0 {
		return errno
	}
	return nil
}
