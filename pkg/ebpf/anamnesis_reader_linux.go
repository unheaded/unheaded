// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

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
		unix.Close(mapFD)
		return nil, fmt.Errorf("get ring buffer size: %w", err)
	}

	// mmap the ring buffer
	pageSize := os.Getpagesize()
	mmapSize := pageSize + 2*ringSize

	mmapData, err := unix.Mmap(mapFD, 0, mmapSize,
		unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		unix.Close(mapFD)
		return nil, fmt.Errorf("mmap ring buffer: %w", err)
	}

	// Create raw byte channel
	rawCh := make(chan []byte, 1024)

	// Start the ring buffer polling goroutine
	go pinnedRingbufPoller(ctx, mapFD, mmapData, rawCh, ringSize, pageSize)

	return NewAnamnesisReader(rawCh, bufSize), nil
}

// getRingbufSize returns the ring buffer size from the map info.
func getRingbufSize(mapFD int) (int, error) {
	info := bpfMapInfo{}
	attr := bpfObjGetInfoAttr{
		BPFFD:   uint32(mapFD),
		InfoLen: uint32(unsafe.Sizeof(info)),
		Info:    uint64(uintptr(unsafe.Pointer(&info))),
	}

	_, err := bpfSyscall(BPF_OBJ_GET_INFO_BY_FD, unsafe.Pointer(&attr), unsafe.Sizeof(attr))
	if err != nil {
		return 0, err
	}

	size := int(info.MaxEntries)
	if size == 0 {
		size = 256 * 1024 // Default 256KB
	}
	return size, nil
}

// pinnedRingbufPoller reads raw records from a mmap'd ring buffer and sends
// them to the output channel.  It runs until ctx is cancelled.
func pinnedRingbufPoller(ctx context.Context, mapFD int, mmapData []byte,
	out chan<- []byte, ringSize, pageSize int) {

	defer close(out)
	defer unix.Munmap(mmapData)
	defer unix.Close(mapFD)

	consumerPos := (*uint64)(unsafe.Pointer(&mmapData[0]))
	producerPos := (*uint64)(unsafe.Pointer(&mmapData[pageSize-8]))
	dataStart := pageSize

	pollFDs := []unix.PollFd{{
		Fd:     int32(mapFD),
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

		// Drain available records
		for {
			cons := *consumerPos
			prod := *producerPos
			if cons >= prod {
				break
			}

			offset := int(cons % uint64(ringSize))
			hdr := (*ringbufHeader)(unsafe.Pointer(&mmapData[dataStart+offset]))
			recordLen := hdr.Len

			if recordLen&BPF_RINGBUF_BUSY_BIT != 0 {
				break
			}

			dataLen := recordLen & ^uint32(BPF_RINGBUF_BUSY_BIT|BPF_RINGBUF_DISCARD_BIT)

			if recordLen&BPF_RINGBUF_DISCARD_BIT != 0 {
				*consumerPos = cons + uint64(roundUp(int(BPF_RINGBUF_HDR_SZ+dataLen), 8))
				continue
			}

			dataOffset := dataStart + offset + BPF_RINGBUF_HDR_SZ
			data := make([]byte, dataLen)
			copy(data, mmapData[dataOffset:dataOffset+int(dataLen)])

			*consumerPos = cons + uint64(roundUp(int(BPF_RINGBUF_HDR_SZ+dataLen), 8))

			select {
			case out <- data:
			case <-ctx.Done():
				return
			}
		}
	}
}
