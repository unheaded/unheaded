// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package ebpf

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func makeTestEvent(evType EventType, hopID uint8, flowLabel uint16, ts uint64) [AnamnesisEventSize]byte {
	ev := AnamnesisEvent{
		TimestampNs: ts,
		EventType:   evType,
		HopID:       hopID,
		FlowLabelLo: flowLabel,
		Monad: MonadState{
			Version:      MonadVersion,
			SrcServiceID: 0x0A,
			DstServiceID: 0x0B,
			HopCount:     hopID,
		},
	}
	return ev.Encode()
}

func TestAnamnesisReader_DecodesEvents(t *testing.T) {
	rawCh := make(chan []byte, 10)
	reader := NewAnamnesisReader(rawCh, 100)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := reader.Poll(ctx)

	// Send 5 events
	for i := 0; i < 5; i++ {
		buf := makeTestEvent(EventBirth, uint8(i), uint16(0x1000+i), uint64(i)*1000)
		rawCh <- buf[:]
	}

	// Read them back
	for i := 0; i < 5; i++ {
		select {
		case ev := <-events:
			if ev.EventType != EventBirth {
				t.Errorf("event %d: type = %v, want Birth", i, ev.EventType)
			}
			if ev.HopID != uint8(i) {
				t.Errorf("event %d: hop_id = %d, want %d", i, ev.HopID, i)
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for event %d", i)
		}
	}

	stats := reader.Stats()
	if stats.EventsRead != 5 {
		t.Errorf("EventsRead = %d, want 5", stats.EventsRead)
	}
	if stats.BytesRead != 5*AnamnesisEventSize {
		t.Errorf("BytesRead = %d, want %d", stats.BytesRead, 5*AnamnesisEventSize)
	}
}

func TestAnamnesisReader_SkipsInvalidEvents(t *testing.T) {
	rawCh := make(chan []byte, 10)
	reader := NewAnamnesisReader(rawCh, 100)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := reader.Poll(ctx)

	// Send invalid event (too short)
	rawCh <- make([]byte, 10)

	// Send invalid event (unknown type)
	var bad [32]byte
	bad[8] = 0xFF // invalid event type
	rawCh <- bad[:]

	// Send valid event
	good := makeTestEvent(EventHop, 1, 0x2000, 999)
	rawCh <- good[:]

	// Should only get the valid one
	select {
	case ev := <-events:
		if ev.EventType != EventHop {
			t.Errorf("type = %v, want Hop", ev.EventType)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for valid event")
	}

	stats := reader.Stats()
	if stats.DecodeErrors != 2 {
		t.Errorf("DecodeErrors = %d, want 2", stats.DecodeErrors)
	}
	if stats.EventsRead != 1 {
		t.Errorf("EventsRead = %d, want 1", stats.EventsRead)
	}
}

func TestAnamnesisReader_Backpressure(t *testing.T) {
	rawCh := make(chan []byte, 100)
	// Tiny buffer to trigger drops
	reader := NewAnamnesisReader(rawCh, 2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Don't consume the events channel — let it fill up
	_ = reader.Poll(ctx)

	// Send many events
	for i := 0; i < 20; i++ {
		buf := makeTestEvent(EventBirth, uint8(i%256), 0x1000, uint64(i))
		rawCh <- buf[:]
	}

	// Give the reader goroutine time to process
	time.Sleep(50 * time.Millisecond)

	stats := reader.Stats()
	total := stats.EventsRead + stats.EventsDropped
	if total != 20 {
		t.Errorf("EventsRead(%d) + EventsDropped(%d) = %d, want 20",
			stats.EventsRead, stats.EventsDropped, total)
	}
	if stats.EventsDropped == 0 {
		t.Error("expected some dropped events with buffer size 2")
	}
}

func TestAnamnesisReader_ContextCancellation(t *testing.T) {
	rawCh := make(chan []byte, 10)
	reader := NewAnamnesisReader(rawCh, 100)

	ctx, cancel := context.WithCancel(context.Background())
	events := reader.Poll(ctx)

	// Cancel context
	cancel()

	// Channel should eventually close
	select {
	case _, ok := <-events:
		if ok {
			// Got an event before close, that's fine — wait for close
			select {
			case _, ok := <-events:
				if ok {
					t.Error("expected channel to close after context cancellation")
				}
			case <-time.After(time.Second):
				t.Fatal("timeout waiting for channel close")
			}
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for channel close")
	}
}

func TestAnamnesisReader_RawChannelClose(t *testing.T) {
	rawCh := make(chan []byte, 10)
	reader := NewAnamnesisReader(rawCh, 100)

	ctx := context.Background()
	events := reader.Poll(ctx)

	// Close the raw channel
	close(rawCh)

	// Events channel should close
	select {
	case _, ok := <-events:
		if ok {
			select {
			case _, ok := <-events:
				if ok {
					t.Error("expected channel to close when raw channel closes")
				}
			case <-time.After(time.Second):
				t.Fatal("timeout waiting for channel close")
			}
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for channel close")
	}
}

func TestAnamnesisReader_AllEventTypes(t *testing.T) {
	rawCh := make(chan []byte, 10)
	reader := NewAnamnesisReader(rawCh, 100)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := reader.Poll(ctx)

	types := []EventType{EventBirth, EventHop, EventDeath, EventAnomaly, EventChaos}
	for _, et := range types {
		buf := makeTestEvent(et, 1, 0x1234, 1000)
		rawCh <- buf[:]
	}

	for _, et := range types {
		select {
		case ev := <-events:
			if ev.EventType != et {
				t.Errorf("type = %v, want %v", ev.EventType, et)
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for event type %v", et)
		}
	}
}

func TestAnamnesisReader_StatsAreAtomic(t *testing.T) {
	rawCh := make(chan []byte, 1000)
	reader := NewAnamnesisReader(rawCh, 1000)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := reader.Poll(ctx)

	// Write events from multiple goroutines
	done := make(chan struct{})
	go func() {
		for i := 0; i < 500; i++ {
			buf := makeTestEvent(EventBirth, uint8(i%256), 0x1000, uint64(i))
			rawCh <- buf[:]
		}
		close(done)
	}()

	// Read stats concurrently
	var lastRead uint64
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				s := reader.Stats()
				cur := atomic.LoadUint64(&s.EventsRead)
				if cur < lastRead {
					t.Errorf("stats went backwards: %d < %d", cur, lastRead)
				}
				lastRead = cur
			}
		}
	}()

	// Drain events
	count := 0
	for count < 500 {
		select {
		case <-events:
			count++
		case <-time.After(5 * time.Second):
			t.Fatalf("timeout after reading %d events", count)
		}
	}

	<-done
}

func TestNewAnamnesisReader_DefaultBufSize(t *testing.T) {
	rawCh := make(chan []byte)
	reader := NewAnamnesisReader(rawCh, 0)
	if reader.bufSize != 4096 {
		t.Errorf("default bufSize = %d, want 4096", reader.bufSize)
	}
}
