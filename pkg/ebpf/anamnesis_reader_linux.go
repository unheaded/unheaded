// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux
// +build linux

package ebpf

import (
	"context"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// DefaultAnamnesisRingPath is the default pin path for the Anamnesis ring buffer.
const DefaultAnamnesisRingPath = "/sys/fs/bpf/unheaded/anamnesis_events"

// OpenPinnedAnamnesisReader opens a pinned BPF ring buffer map and returns an
// AnamnesisReader that decodes events from it.
//
// The pin path should point to a BPF_MAP_TYPE_RINGBUF map populated by
// Shield/Hop/Yaldabaoth BPF programs.
//
// The reader runs until ctx is cancelled.  The caller is responsible for
// cancelling the context to release resources.
func OpenPinnedAnamnesisReader(ctx context.Context, pinPath string, bufSize int) (*AnamnesisReader, error) {
	if pinPath == "" {
		pinPath = DefaultAnamnesisRingPath
	}

	// Open the pinned ring buffer map
	mapFD, err := bpfObjGet(pinPath)
	if err != nil {
		return nil, fmt.Errorf("open pinned ring buffer %s: %w", pinPath, err)
	}

	// Get map info to determine ring buffer size
	ringSize, err := getRingbufSize(mapFD)
	if err != nil {
		_ = unix.Close(mapFD)
		return nil, fmt.Errorf("get ring buffer size: %w", err)
	}

	// mmap the ring buffer: the consumer page read-write, the producer page
	// and double-mapped data area read-only (the kernel refuses a writable
	// mapping of those; one read-write mmap of the whole ring failed with
	// EPERM every time). See consumeRingbuf for the record format.
	pageSize := os.Getpagesize()
	consPage, err := unix.Mmap(mapFD, 0, pageSize, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		_ = unix.Close(mapFD)
		return nil, fmt.Errorf("mmap ring buffer consumer page: %w", err)
	}
	prodArea, err := unix.Mmap(mapFD, int64(pageSize), pageSize+2*ringSize, unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		_ = unix.Munmap(consPage)
		_ = unix.Close(mapFD)
		return nil, fmt.Errorf("mmap ring buffer data: %w", err)
	}

	// Create raw byte channel
	rawCh := make(chan []byte, 1024)

	// Start the ring buffer polling goroutine
	go pinnedRingbufPoller(ctx, mapFD, consPage, prodArea, rawCh, ringSize, pageSize)

	return NewAnamnesisReader(rawCh, bufSize), nil
}

// getRingbufSize returns the ring buffer size from the map info.
func getRingbufSize(mapFD int) (int, error) {
	info := bpfMapInfo{}
	attr := bpfObjGetInfoAttr{
		BPFFD:   uint32(mapFD), // #nosec G115 -- BPF syscall ABI: Go file descriptors are int, the kernel takes __u32; FDs are small non-negative
		InfoLen: uint32(unsafe.Sizeof(info)),
		Info:    uint64(uintptr(unsafe.Pointer(&info))), // #nosec G103 -- compile-time size of a fixed kernel struct; no pointer is dereferenced
	}

	_, err := bpfSyscall(BPF_OBJ_GET_INFO_BY_FD, unsafe.Pointer(&attr), unsafe.Sizeof(attr)) // #nosec G103 -- compile-time size of a fixed kernel struct; no pointer is dereferenced
	if err != nil {
		return 0, err
	}

	size := int(info.MaxEntries)
	if size == 0 {
		size = 256 * 1024 // Default 256KB
	}
	return size, nil
}

// pinnedRingbufPoller reads raw records from the mapped ring buffer and
// sends them to the output channel (blocking: none are dropped). It runs
// until ctx is cancelled.
func pinnedRingbufPoller(ctx context.Context, mapFD int, consPage, prodArea []byte,
	out chan<- []byte, ringSize, pageSize int) {

	defer close(out)
	defer func() { _ = unix.Munmap(prodArea) }()
	defer func() { _ = unix.Munmap(consPage) }()
	defer func() { _ = unix.Close(mapFD) }()

	consumerPos := (*uint64)(unsafe.Pointer(&consPage[0])) // #nosec G103 -- typed overlay on the kernel's consumer-position word
	producerPos := (*uint64)(unsafe.Pointer(&prodArea[0])) // #nosec G103 -- typed overlay on the kernel's producer-position word
	data := prodArea[pageSize:]

	pollFDs := []unix.PollFd{{
		Fd:     int32(mapFD), // #nosec G115 -- poll(2) FD field is int32 by ABI
		Events: unix.POLLIN,
	}}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Poll with 100ms timeout
		n, err := unix.Poll(pollFDs, 100)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			return
		}
		if n == 0 {
			continue
		}

		consumeRingbuf(consumerPos, producerPos, data, ringSize, func(rec []byte) bool {
			select {
			case out <- rec:
				return true
			case <-ctx.Done():
				return false
			}
		})
		if ctx.Err() != nil {
			return
		}
	}
}
