// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// AnamnesisReader reads AnamnesisEvent structs from a BPF ring buffer.
//
// It wraps a raw byte channel (from NativeLoader.ReadRingbuf or any other
// source) and decodes 32-byte records into typed AnamnesisEvent structs.
//
// This file is platform-independent — it does not use any Linux-specific
// syscalls.  The Linux-specific ring buffer opening is in
// anamnesis_reader_linux.go.
package ebpf

import (
	"context"
	"sync"
	"sync/atomic"
)

// ReaderStats tracks AnamnesisReader performance counters.
type ReaderStats struct {
	EventsRead    uint64 `json:"events_read"`
	EventsDropped uint64 `json:"events_dropped"`
	BytesRead     uint64 `json:"bytes_read"`
	DecodeErrors  uint64 `json:"decode_errors"`
	// LegacyDecodes counts records decoded with the pre-S-TRUE-WIRE event
	// numbering (WithLegacyEventTypes); nonzero means an old object is
	// still loaded on this source.
	LegacyDecodes uint64 `json:"legacy_decodes"`
}

// AnamnesisReader decodes AnamnesisEvent records from a raw byte channel.
//
// Usage:
//
//	reader := NewAnamnesisReader(rawCh, 4096)
//	events := reader.Poll(ctx)
//	for ev := range events {
//	    // process ev
//	}
type AnamnesisReader struct {
	raw       <-chan []byte
	events    chan *AnamnesisEvent
	bufSize   int
	stats     ReaderStats
	closeOnce sync.Once
	legacy    bool
}

// NewAnamnesisReader creates a reader that decodes events from a raw byte channel.
// bufSize controls the output channel capacity (backpressure threshold).
func NewAnamnesisReader(raw <-chan []byte, bufSize int) *AnamnesisReader {
	if bufSize <= 0 {
		bufSize = 4096
	}
	return &AnamnesisReader{
		raw:     raw,
		events:  make(chan *AnamnesisEvent, bufSize),
		bufSize: bufSize,
	}
}

// WithLegacyEventTypes makes the reader decode event types in the
// pre-S-TRUE-WIRE numbering, for a ring buffer fed by an object built before
// the renumber. The loader knows which object it loaded; the byte cannot say.
// Call before Poll.
func (r *AnamnesisReader) WithLegacyEventTypes() *AnamnesisReader {
	r.legacy = true
	return r
}

// Poll starts reading events and returns the decoded event channel.
// The returned channel is closed when ctx is cancelled or the raw channel closes.
// Poll may only be called once.
func (r *AnamnesisReader) Poll(ctx context.Context) <-chan *AnamnesisEvent {
	go r.readLoop(ctx)
	return r.events
}

// Stats returns a snapshot of the reader's performance counters.
func (r *AnamnesisReader) Stats() ReaderStats {
	return ReaderStats{
		EventsRead:    atomic.LoadUint64(&r.stats.EventsRead),
		EventsDropped: atomic.LoadUint64(&r.stats.EventsDropped),
		BytesRead:     atomic.LoadUint64(&r.stats.BytesRead),
		DecodeErrors:  atomic.LoadUint64(&r.stats.DecodeErrors),
		LegacyDecodes: atomic.LoadUint64(&r.stats.LegacyDecodes),
	}
}

func (r *AnamnesisReader) readLoop(ctx context.Context) {
	defer r.closeOnce.Do(func() { close(r.events) })

	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-r.raw:
			if !ok {
				return // raw channel closed
			}
			atomic.AddUint64(&r.stats.BytesRead, uint64(len(data)))

			decode := DecodeAnamnesisEvent
			if r.legacy {
				decode = DecodeAnamnesisEventLegacy
			}
			ev, err := decode(data)
			if err != nil {
				atomic.AddUint64(&r.stats.DecodeErrors, 1)
				continue
			}
			if r.legacy {
				atomic.AddUint64(&r.stats.LegacyDecodes, 1)
			}

			// Non-blocking send with backpressure
			select {
			case r.events <- ev:
				atomic.AddUint64(&r.stats.EventsRead, 1)
			default:
				// Channel full — drop event, don't block
				atomic.AddUint64(&r.stats.EventsDropped, 1)
			}
		}
	}
}
