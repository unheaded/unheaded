// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build linux

package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	ciliumebpf "github.com/cilium/ebpf"
)

// fakeBatchedMap is LATENCY_MAP without a kernel: entries handed out in
// batches of batchSize, or one at a time through walk.
type fakeBatchedMap struct {
	keys, values [][]byte
	batchSize    int

	batchErr   error // every batch call fails with it (ErrNotSupported: an old kernel)
	failAtCall int   // this batch call (1-based) delivers its entries and then fails
	walkErr    error

	pos        int
	batchCalls int
	walkCalls  int
}

var errMidSweep = errors.New("map changed under the read")

func (f *fakeBatchedMap) batch(first bool, visit func(key, value []byte)) (bool, error) {
	f.batchCalls++
	if f.batchErr != nil {
		return false, f.batchErr
	}
	if first {
		f.pos = 0
	}
	end := min(f.pos+f.batchSize, len(f.keys))
	for i := f.pos; i < end; i++ {
		visit(f.keys[i], f.values[i])
	}
	f.pos = end
	if f.batchCalls == f.failAtCall {
		return false, errMidSweep
	}
	return f.pos == len(f.keys), nil
}

func (f *fakeBatchedMap) walk(visit func(key, value []byte)) error {
	f.walkCalls++
	for i := range f.keys {
		visit(f.keys[i], f.values[i])
	}
	return f.walkErr
}

// latencyEntries builds n entries, one thread each, with the given sample
// count. keySize is LatencyTraceKeySize for a good map.
func latencyEntries(n, keySize int, samples uint64) (keys, values [][]byte) {
	keys, values = make([][]byte, n), make([][]byte, n)
	for i := range n {
		k := make([]byte, keySize)
		binary.LittleEndian.PutUint32(k[16:20], 1000)        // pid
		binary.LittleEndian.PutUint32(k[20:24], uint32(i+1)) // tid
		e := LatencyEntry{RTTNS: 1500, MinRTTNS: 1000, MaxRTTNS: 2000, Samples: samples, LastUpdateNS: uint64(i)}
		v := e.Encode()
		keys[i], values[i] = k, v[:]
	}
	return keys, values
}

// newTestSweeper returns a sweeper over m that collects what it publishes.
func newTestSweeper(m batchedMap) (*latencySweeper, *[][]byte) {
	var published [][]byte
	s := newLatencySweeper(m, func(p []byte) { published = append(published, p) })
	s.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	return s, &published
}

// One sweep of a 10,000 entry map is three batch reads, not 20,000 system
// calls: the old sweep made two bpf() calls per entry, every 200 ms (FND-044).
func TestSweepBatches(t *testing.T) {
	keys, values := latencyEntries(10_000, LatencyTraceKeySize, 1)
	m := &fakeBatchedMap{keys: keys, values: values, batchSize: latencySweepBatch}
	s, published := newTestSweeper(m)

	if err := s.sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.batchCalls != 3 || m.walkCalls != 0 {
		t.Fatalf("batch calls = %d, walks = %d; want 3 and 0", m.batchCalls, m.walkCalls)
	}
	if len(*published) != 10_000 {
		t.Fatalf("published %d entries, want 10000", len(*published))
	}
}

// A kernel without batch lookups is read one entry at a time, and says so
// once, not on every sweep.
func TestSweepFallsBackWhenBatchUnsupported(t *testing.T) {
	keys, values := latencyEntries(50, LatencyTraceKeySize, 1)
	m := &fakeBatchedMap{keys: keys, values: values, batchSize: latencySweepBatch,
		batchErr: fmt.Errorf("map batch lookup: %w", ciliumebpf.ErrNotSupported)}
	s, published := newTestSweeper(m)
	warned := 0
	s.w.onFallback = func(error) { warned++ }

	for range 3 {
		if err := s.sweep(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if m.walkCalls != 3 {
		t.Fatalf("walks = %d, want one per sweep (3)", m.walkCalls)
	}
	if m.batchCalls != 1 {
		t.Fatalf("batch calls = %d, want 1: an unsupported batch read is not retried every sweep", m.batchCalls)
	}
	if warned != 1 {
		t.Fatalf("fallback logged %d times, want once", warned)
	}
	if len(*published) != 50 {
		t.Fatalf("published %d entries, want 50 (the first sweep; later ones saw nothing new)", len(*published))
	}
}

// West's state before the fix: an object with a 40-byte key under a binary
// that reads 24. Every entry was skipped by a bare continue and nothing
// counted it.
func TestSweepCountsDecodeErrors(t *testing.T) {
	keys, values := latencyEntries(7, 40, 1)
	m := &fakeBatchedMap{keys: keys, values: values, batchSize: latencySweepBatch}
	s, published := newTestSweeper(m)
	before := latencyDecodeErrors.Value()

	if err := s.sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := latencyDecodeErrors.Value() - before; got != 7 {
		t.Fatalf("trace_collector_latency_decode_errors_total rose by %v, want 7", got)
	}
	if len(*published) != 0 {
		t.Fatalf("published %d entries that did not decode", len(*published))
	}

	// A short value is a decode error too.
	keys, values = latencyEntries(2, LatencyTraceKeySize, 1)
	values[1] = values[1][:32]
	m = &fakeBatchedMap{keys: keys, values: values, batchSize: latencySweepBatch}
	s, published = newTestSweeper(m)
	before = latencyDecodeErrors.Value()
	if err := s.sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := latencyDecodeErrors.Value() - before; got != 1 || len(*published) != 1 {
		t.Fatalf("short value: decode errors +%v, published %d; want +1 and 1", got, len(*published))
	}
}

// A sweep that failed part way did not see every entry, so it must not
// forget the ones it missed: they would all look new on the next sweep.
func TestSweepPartialErrorKeepsTracker(t *testing.T) {
	keys, values := latencyEntries(10_000, LatencyTraceKeySize, 1)
	m := &fakeBatchedMap{keys: keys, values: values, batchSize: latencySweepBatch}
	s, published := newTestSweeper(m)
	if err := s.sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	*published = nil
	gaugeBefore := latencyMapEntries.Value()

	m.batchCalls, m.failAtCall = 0, 2 // the second batch of the next sweep fails
	err := s.sweep(context.Background())
	if !errors.Is(err, errMidSweep) {
		t.Fatalf("err = %v, want the mid-sweep error", err)
	}
	if got := len(s.rtts.seen); got != 10_000 {
		t.Fatalf("tracker holds %d entries after a partial sweep, want all 10000", got)
	}
	if latencyMapEntries.Value() != gaugeBefore {
		t.Fatalf("entries gauge moved to %v on a partial sweep", latencyMapEntries.Value())
	}

	m.failAtCall = 0
	if err := s.sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(*published) != 0 {
		t.Fatalf("published %d entries after a partial sweep with no new samples, want 0", len(*published))
	}
}

// A cancelled sweep is a partial sweep.
func TestSweepStopsOnCancel(t *testing.T) {
	keys, values := latencyEntries(10_000, LatencyTraceKeySize, 1)
	m := &fakeBatchedMap{keys: keys, values: values, batchSize: latencySweepBatch}
	s, _ := newTestSweeper(m)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.sweep(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if m.batchCalls != 0 {
		t.Fatalf("batch calls = %d after cancel, want 0", m.batchCalls)
	}
}

// Only entries with a new sample since the last sweep go out: re-sending
// every thread's latest RTT each sweep weighted the dashboard's percentiles
// by threads x sweeps.
func TestSweepPublishesOnlyFresh(t *testing.T) {
	keys, values := latencyEntries(4, LatencyTraceKeySize, 1)
	m := &fakeBatchedMap{keys: keys, values: values, batchSize: latencySweepBatch}
	s, published := newTestSweeper(m)

	if err := s.sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(*published) != 4 {
		t.Fatalf("first sweep published %d, want 4", len(*published))
	}
	*published = nil
	if err := s.sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(*published) != 0 {
		t.Fatalf("unchanged sweep published %d, want 0", len(*published))
	}

	// One thread takes a new sample.
	e := LatencyEntry{RTTNS: 9000, MinRTTNS: 1000, MaxRTTNS: 9000, Samples: 2}
	v := e.Encode()
	m.values[2] = v[:]
	if err := s.sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(*published) != 1 {
		t.Fatalf("published %d after one new sample, want 1", len(*published))
	}
	want := `{"latency_ns":9000,"max_rtt_ns":9000,"min_rtt_ns":1000,"operation":"tcp_send","pid":1000,"samples":2,"tid":3,"timestamp_ns":1700000000000000000,"trace_id":{"high":0,"low":0}}`
	if got := string((*published)[0]); got != want {
		t.Fatalf("payload\n got %s\nwant %s", got, want)
	}
}

func TestSweepSetsEntriesGauge(t *testing.T) {
	keys, values := latencyEntries(321, LatencyTraceKeySize, 0) // no samples: nothing is published
	m := &fakeBatchedMap{keys: keys, values: values, batchSize: 100}
	s, published := newTestSweeper(m)
	if err := s.sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := latencyMapEntries.Value(); got != 321 {
		t.Fatalf("trace_collector_latency_map_entries = %v, want 321", got)
	}
	if len(*published) != 0 {
		t.Fatalf("published %d entries with no samples", len(*published))
	}
}

// The real map, in the kernel: an LRU hash of the probe's layout, filled from
// here, read back through the adapter the collector uses. Needs root; the
// tests above cover the logic without it.
//
//	go test -c -o /tmp/tc.test ./cmd/trace-collector-go && sudo /tmp/tc.test -test.run TestSweepRealLRUMap -test.v
func TestSweepRealLRUMap(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root (CAP_BPF)")
	}
	const n = 10_000
	km, err := ciliumebpf.NewMap(&ciliumebpf.MapSpec{
		Type: ciliumebpf.LRUHash, KeySize: LatencyTraceKeySize, ValueSize: LatencyEntrySize, MaxEntries: 65536,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer km.Close()
	keys, values := latencyEntries(n, LatencyTraceKeySize, 1)
	for i := range keys {
		if err := km.Put(keys[i], values[i]); err != nil {
			t.Fatal(err)
		}
	}

	seen := map[string]bool{}
	s := newLatencySweeper(newEBPFLatencyMap(km), func([]byte) {})
	s.w.onFallback = func(err error) { t.Fatalf("this kernel refused the batch read: %v", err) }
	tids := 0
	s.publish = func(p []byte) { seen[string(p)] = true; tids++ }
	if err := s.sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if tids != n || len(seen) != n {
		t.Fatalf("read %d entries (%d distinct) from the kernel map, want %d", tids, len(seen), n)
	}
	if got := latencyMapEntries.Value(); got != n {
		t.Fatalf("entries gauge = %v, want %d", got, n)
	}
	// A second sweep reads the same entries and publishes none.
	tids = 0
	if err := s.sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if tids != 0 {
		t.Fatalf("second sweep published %d entries, want 0", tids)
	}
}
