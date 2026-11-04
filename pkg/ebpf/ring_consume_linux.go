// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package ebpf

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// ConsumePinnedRing hands every record of the ring buffer pinned at path to
// emit, in order, until ctx ends. It returns nil then, having unmapped the
// ring and closed the map; a pin it cannot open or map, a pin that is not a
// ring buffer and a failed poll are errors, and end it at once.
//
// emit runs on the caller's goroutine, one record at a time, and owns the
// slice it gets. A slow emit backs up into the ring, and the kernel then fails
// the producer's reservation: nothing here drops a record it has read.
//
// A ring has one consumer position. A second reader of the same pin does not
// see the records twice; each takes some of them from the other.
func ConsumePinnedRing(ctx context.Context, path string, emit func([]byte)) error {
	fd, err := bpfObjGet(path)
	if err != nil {
		return fmt.Errorf("open pinned ring %s: %w", path, err)
	}
	defer func() { _ = unix.Close(fd) }()

	info, err := bpfGetMapInfo(fd)
	if err != nil {
		return fmt.Errorf("map info %s: %w", path, err)
	}
	if info.Type != mapTypeRingbuf {
		return fmt.Errorf("%s is not a ring buffer: map type %d", path, info.Type)
	}
	ringSize, err := getRingbufSize(fd)
	if err != nil {
		return fmt.Errorf("ring size %s: %w", path, err)
	}

	// The consumer page is read-write; the kernel refuses a writable mapping
	// of the producer page and the double-mapped data area after it.
	pageSize := os.Getpagesize()
	consPage, err := unix.Mmap(fd, 0, pageSize, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		return fmt.Errorf("mmap consumer page %s: %w", path, err)
	}
	defer func() { _ = unix.Munmap(consPage) }()
	prodArea, err := unix.Mmap(fd, int64(pageSize), pageSize+2*ringSize, unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		return fmt.Errorf("mmap ring data %s: %w", path, err)
	}
	defer func() { _ = unix.Munmap(prodArea) }()

	consumerPos := (*uint64)(unsafe.Pointer(&consPage[0])) // #nosec G103 -- typed overlay on the kernel's consumer-position word
	producerPos := (*uint64)(unsafe.Pointer(&prodArea[0])) // #nosec G103 -- typed overlay on the kernel's producer-position word
	data := prodArea[pageSize:]
	pollFDs := []unix.PollFd{{
		Fd:     int32(fd), // #nosec G115 -- poll(2) FD field is int32 by ABI
		Events: unix.POLLIN,
	}}

	for ctx.Err() == nil {
		// Bounded, so a quiet ring still notices ctx.
		n, err := unix.Poll(pollFDs, 100)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return fmt.Errorf("poll ring %s: %w", path, err)
		}
		if n == 0 {
			continue
		}
		emitted := consumeRingbuf(consumerPos, producerPos, data, ringSize, func(rec []byte) bool {
			emit(rec)
			return ctx.Err() == nil
		})
		if emitted == 0 {
			// Poll is level-triggered: a record reserved and not yet committed
			// keeps it ready while there is nothing to take. Do not spin on it.
			time.Sleep(time.Millisecond)
		}
	}
	return nil
}
