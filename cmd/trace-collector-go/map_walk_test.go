// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	ciliumebpf "github.com/cilium/ebpf"
	"golang.org/x/sys/unix"
)

// flowEntries builds n distinct flows in FLOWS's layout (key 16, value 72).
func flowEntries(n int) (keys, values [][]byte) {
	keys, values = make([][]byte, n), make([][]byte, n)
	now := ktimeNow()
	for i := range n {
		fk := makeFlowKey([4]byte{10, 10, byte(i >> 8), byte(i)}, [4]byte{10, 20, 0, 1}, uint16(1024+i%60000), 443, 6)
		fs := makeFlowState(ConnStateEstablished, 1000, now, 1, 2, 100, 200)
		k, v := fk.Encode(), fs.Encode()
		keys[i], values[i] = k[:], v[:]
	}
	return keys, values
}

// batchMockMap is a flow map the reader can read in batches: the MockMapReader
// it embeds is the per-entry walk, the fake is the batched view.
type batchMockMap struct {
	*MockMapReader
	fake    *fakeBatchedMap
	viewErr error
}

func (b *batchMockMap) batchView() (batchedMap, error) {
	if b.viewErr != nil {
		return nil, b.viewErr
	}
	return b.fake, nil
}

// batchMockLoader hands the flow reader a batch-capable flow map.
type batchMockLoader struct {
	*MockBPFLoader
	flows *batchMockMap
}

func (l *batchMockLoader) GetFlowStateMap() MapReader { return l.flows }

func newBatchMockLoader(keys, values [][]byte, batchSize int) *batchMockLoader {
	mock := NewMockMapReader()
	for i := range keys {
		mock.Insert(keys[i], values[i])
	}
	return &batchMockLoader{
		MockBPFLoader: NewMockBPFLoader(),
		flows:         &batchMockMap{MockMapReader: mock, fake: &fakeBatchedMap{keys: keys, values: values, batchSize: batchSize}},
	}
}

func knownFlowKeys(fr *FlowReader) map[string]bool {
	fr.mu.Lock()
	defer fr.mu.Unlock()
	out := make(map[string]bool, len(fr.knownFlows))
	for k := range fr.knownFlows {
		out[k] = true
	}
	return out
}

// The walker is the one loop both readers share.
func TestMapWalkerBatchesThenStops(t *testing.T) {
	keys, values := flowEntries(10_000)
	m := &fakeBatchedMap{keys: keys, values: values, batchSize: mapSweepBatch}
	w := newMapWalker("flows", m)
	n := 0
	if err := w.read(context.Background(), func(_, _ []byte) { n++ }); err != nil {
		t.Fatal(err)
	}
	if n != 10_000 || m.batchCalls != 3 || m.walkCalls != 0 {
		t.Fatalf("visited %d in %d batch calls and %d walks; want 10000, 3, 0", n, m.batchCalls, m.walkCalls)
	}
}

// Reading 10,000 entries must not allocate per batch: the buffers are the
// map's, and the walker keeps nothing.
func TestMapWalkerAllocsDoNotScaleWithBatches(t *testing.T) {
	keys, values := flowEntries(10_000)
	ctx := context.Background()
	visit := func(_, _ []byte) {}
	allocs := func(batchSize int) float64 {
		w := newMapWalker("flows", &fakeBatchedMap{keys: keys, values: values, batchSize: batchSize})
		return testing.AllocsPerRun(20, func() {
			if err := w.read(ctx, visit); err != nil {
				t.Fatal(err)
			}
		})
	}
	few, many := allocs(mapSweepBatch), allocs(16) // 3 batches against 625
	if few != 0 || many != 0 {
		t.Fatalf("allocations per sweep: %v with 3 batches, %v with 625; want 0 and 0", few, many)
	}
}

// The batched read gives the flow reader the same flows as the per-entry
// walk, and the same events.
func TestFlowReader_BatchedReadMatchesWalk(t *testing.T) {
	keys, values := flowEntries(10_000)

	batched := newBatchMockLoader(keys, values, mapSweepBatch)
	walked := NewMockBPFLoader()
	for i := range keys {
		walked.flowMap.Insert(keys[i], values[i])
	}

	rb := NewFlowReader(batched, nil, DefaultFlowReaderConfig())
	rw := NewFlowReader(walked, nil, DefaultFlowReaderConfig())
	rb.pollOnce(context.Background())
	rw.pollOnce(context.Background())

	if batched.flows.fake.batchCalls != 3 || batched.flows.fake.walkCalls != 0 {
		t.Fatalf("batch calls = %d, walks = %d; want 3 and 0", batched.flows.fake.batchCalls, batched.flows.fake.walkCalls)
	}
	kb, kw := knownFlowKeys(rb), knownFlowKeys(rw)
	if len(kb) != 10_000 || len(kw) != 10_000 {
		t.Fatalf("known flows: batched %d, walked %d; want 10000 each", len(kb), len(kw))
	}
	for k := range kw {
		if !kb[k] {
			t.Fatalf("flow %s seen by the walk and not by the batched read", k)
		}
	}
	sb, sw := rb.Stats(), rw.Stats()
	if sb.FlowsRead != sw.FlowsRead || sb.EventsSent != sw.EventsSent || sb.DecodeErrors != sw.DecodeErrors || sb.FlowsExpired != sw.FlowsExpired {
		t.Fatalf("stats differ: batched %+v, walked %+v", sb, sw)
	}
	if sb.FlowsRead != 10_000 || sb.EventsSent != 10_000 {
		t.Fatalf("batched stats %+v: want 10000 read, 10000 new-flow events", sb)
	}
	if got := flowMapEntries.Value(); got != 10_000 {
		t.Fatalf("trace_collector_flow_map_entries = %v, want 10000", got)
	}

	// A second poll of an unchanged map: nothing new, nothing expired.
	rb.pollOnce(context.Background())
	if s := rb.Stats(); s.EventsSent != 10_000 || s.FlowsExpired != 0 {
		t.Fatalf("after an unchanged poll: %+v", s)
	}
}

// A kernel that refuses the batch call: the reader walks the map as it always
// did, says so once, and every such sweep is counted.
func TestFlowReader_BatchUnsupportedFallsBackAndCounts(t *testing.T) {
	keys, values := flowEntries(50)
	loader := newBatchMockLoader(keys, values, mapSweepBatch)
	loader.flows.fake.batchErr = fmt.Errorf("map batch lookup: %w", ciliumebpf.ErrNotSupported)
	before := mapUnbatchedSweeps.WithLabelValues("flows").Value()

	fr := NewFlowReader(loader, nil, DefaultFlowReaderConfig())
	for range 3 {
		fr.pollOnce(context.Background())
	}
	if got := loader.flows.fake.batchCalls; got != 1 {
		t.Fatalf("batch calls = %d, want 1: a refused batch read is not retried every sweep", got)
	}
	if got := loader.flows.fake.walkCalls; got != 3 {
		t.Fatalf("walks = %d, want 3", got)
	}
	if s := fr.Stats(); s.FlowsRead != 150 || s.EventsSent != 50 {
		t.Fatalf("stats %+v: want 150 read over three polls, 50 new-flow events", s)
	}
	if got := mapUnbatchedSweeps.WithLabelValues("flows").Value() - before; got != 3 {
		t.Fatalf("trace_collector_map_unbatched_sweeps_total{map=\"flows\"} rose by %v, want 3", got)
	}
}

// A map with no batched view at all (the mock, or a view that could not be
// opened) is walked, and counted the same way.
func TestFlowReader_NoBatchViewIsWalked(t *testing.T) {
	keys, values := flowEntries(5)
	loader := newBatchMockLoader(keys, values, mapSweepBatch)
	loader.flows.viewErr = errors.New("FLOWS has key 20 bytes, want 16")
	before := mapUnbatchedSweeps.WithLabelValues("flows").Value()

	fr := NewFlowReader(loader, nil, DefaultFlowReaderConfig())
	fr.pollOnce(context.Background())
	if s := fr.Stats(); s.FlowsRead != 5 {
		t.Fatalf("FlowsRead = %d, want 5 (through the mock's Iterate)", s.FlowsRead)
	}
	if loader.flows.fake.batchCalls != 0 {
		t.Fatalf("the refused view was read %d times", loader.flows.fake.batchCalls)
	}
	if got := mapUnbatchedSweeps.WithLabelValues("flows").Value() - before; got != 1 {
		t.Fatalf("unbatched sweeps rose by %v, want 1", got)
	}
}

// A read that failed part way saw only some of the map. The flows it did not
// reach are still there: none may be expired on the strength of it.
func TestFlowReader_PartialBatchReadExpiresNothing(t *testing.T) {
	keys, values := flowEntries(10_000)
	loader := newBatchMockLoader(keys, values, mapSweepBatch)
	fr := NewFlowReader(loader, nil, DefaultFlowReaderConfig())
	fr.pollOnce(context.Background())
	errsBefore := flowReaderErrors.WithLabelValues("iterate").Value()

	loader.flows.fake.batchCalls, loader.flows.fake.failAtCall = 0, 2
	fr.pollOnce(context.Background())
	if s := fr.Stats(); s.FlowsExpired != 0 {
		t.Fatalf("%d flows expired after a partial read", s.FlowsExpired)
	}
	if got := fr.ActiveFlows(); got != 10_000 {
		t.Fatalf("active flows = %d after a partial read, want 10000", got)
	}
	if got := flowReaderErrors.WithLabelValues("iterate").Value() - errsBefore; got != 1 {
		t.Fatalf("flow_reader_errors_total{operation=\"iterate\"} rose by %v, want 1", got)
	}
}

// The real map, in the kernel: an LRU hash of FLOWS's shape, read through the
// view the flow reader opens on a duplicated descriptor. Needs root.
//
//	go test -c -o /tmp/tc.test ./cmd/trace-collector-go && sudo /tmp/tc.test -test.run TestFlowsBatchRealLRUMap -test.v
func TestFlowsBatchRealLRUMap(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root (CAP_BPF)")
	}
	const n = 10_000
	km, err := ciliumebpf.NewMap(&ciliumebpf.MapSpec{
		Type: ciliumebpf.LRUHash, KeySize: FlowKeySize, ValueSize: FlowStateSize, MaxEntries: 65536,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer km.Close()
	keys, values := flowEntries(n)
	for i := range keys {
		if err := km.Put(keys[i], values[i]); err != nil {
			t.Fatal(err)
		}
	}

	// The reader gets its own descriptor, as it does from the loader.
	fd, err := unix.FcntlInt(uintptr(km.FD()), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	view, err := newFlowsBatchMap(fd, NewMockMapReader())
	if err != nil {
		t.Fatal(err)
	}
	w := newMapWalker("flows", view)
	w.onFallback = func(err error) { t.Fatalf("this kernel refused the batch read: %v", err) }

	for sweep := range 2 {
		seen := make(map[string]bool, n)
		if err := w.read(context.Background(), func(k, v []byte) {
			if len(k) != FlowKeySize || len(v) != FlowStateSize {
				t.Fatalf("entry of %d/%d bytes", len(k), len(v))
			}
			seen[string(k)] = true
		}); err != nil {
			t.Fatal(err)
		}
		if len(seen) != n {
			t.Fatalf("sweep %d read %d distinct flows from the kernel map, want %d", sweep, len(seen), n)
		}
	}

	// A map of another shape is refused before it is read.
	other, err := ciliumebpf.NewMap(&ciliumebpf.MapSpec{Type: ciliumebpf.LRUHash, KeySize: 20, ValueSize: FlowStateSize, MaxEntries: 16})
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	fd, err = unix.FcntlInt(uintptr(other.FD()), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newFlowsBatchMap(fd, NewMockMapReader()); err == nil {
		t.Fatal("a map with a 20-byte key was accepted as FLOWS")
	}
}
