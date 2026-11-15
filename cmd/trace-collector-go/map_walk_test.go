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

// ── Entries that did not change since the last pass ─────────────────────────

// setKtime pins the BPF clock for a test and returns a function that moves it.
func setKtime(t testing.TB, start uint64) (set func(uint64)) {
	t.Helper()
	old := ktimeNow
	now := start
	ktimeNow = func() uint64 { return now }
	t.Cleanup(func() { ktimeNow = old })
	return func(v uint64) { now = v }
}

const testSecond = uint64(1_000_000_000)

func putFlow(m *fakeBatchedMap, i int, fs FlowStateEntry) {
	v := fs.Encode()
	m.values[i] = v[:]
}

// drainFlowEvents empties the reader's event channel and counts by type.
func drainFlowEvents(fr *FlowReader) map[uint8]int {
	got := map[uint8]int{}
	for {
		select {
		case ev := <-fr.Events():
			got[ev.EventType]++
		default:
			return got
		}
	}
}

// West, 2026-11-15: FLOWS held 7,337 entries, 54 of them live, and the reader
// announced every dead one as new and then expired on every poll: 7,281 of
// each a second, 48 million in under four hours. Nothing deletes a flow from
// the map, so a dead entry stays until the LRU needs the room. A flow that
// was already dead when the reader first saw it is not news.
func TestFlowReader_DeadFlowIsNotAnnouncedEveryPoll(t *testing.T) {
	setKtime(t, 1000*testSecond)
	keys, values := flowEntries(2) // both stamped with the clock: 1000 s
	loader := newBatchMockLoader(keys, values, mapSweepBatch)
	putFlow(loader.flows.fake, 1, makeFlowState(ConnStateEstablished, 100*testSecond, 400*testSecond, 5, 5, 500, 500)) // quiet for 600 s

	fr := NewFlowReader(loader, nil, DefaultFlowReaderConfig()) // stale after 60 s
	for range 3 {
		fr.pollOnce(context.Background())
	}
	s := fr.Stats()
	if s.EventsSent != 1 || s.FlowsExpired != 0 {
		t.Fatalf("after three polls: %d events, %d expired; want 1 (the live flow, once) and 0", s.EventsSent, s.FlowsExpired)
	}
	if got := drainFlowEvents(fr); got[FlowEventNew] != 1 || len(got) != 1 {
		t.Fatalf("events by type %v, want one new", got)
	}
	if fr.ActiveFlows() != 1 {
		t.Fatalf("active flows = %d, want 1", fr.ActiveFlows())
	}
	if s.FlowsRead != 6 {
		t.Fatalf("FlowsRead = %d, want 6: a dead entry is still read, only not announced", s.FlowsRead)
	}
}

// A flow the reader knows goes quiet: it expires once, with its last
// counters, and is not announced again while its entry sits in the map.
func TestFlowReader_KnownFlowGoingStaleExpiresOnce(t *testing.T) {
	set := setKtime(t, 1000*testSecond)
	keys, values := flowEntries(1)
	loader := newBatchMockLoader(keys, values, mapSweepBatch)
	fr := NewFlowReader(loader, nil, DefaultFlowReaderConfig())

	fr.pollOnce(context.Background())
	if got := drainFlowEvents(fr); got[FlowEventNew] != 1 {
		t.Fatalf("first poll: %v, want one new", got)
	}
	for i := range 5 { // 61 s later and on: the entry is still in the map
		set((1061 + uint64(i)) * testSecond)
		fr.pollOnce(context.Background())
	}
	got := drainFlowEvents(fr)
	if got[FlowEventExpired] != 1 || got[FlowEventNew] != 0 {
		t.Fatalf("after the flow went quiet: %v, want exactly one expired and nothing else", got)
	}
	if s := fr.Stats(); s.FlowsExpired != 1 || s.EventsSent != 2 {
		t.Fatalf("stats %+v: want 1 expired, 2 events in all", s)
	}

	// It comes back: the kernel stamps the same entry and keeps counting. That
	// is a new flow to the reader, with the entry's running totals, as before.
	set(1200 * testSecond)
	putFlow(loader.flows.fake, 0, makeFlowState(ConnStateEstablished, 1000, 1200*testSecond, 7, 9, 700, 900))
	fr.pollOnce(context.Background())
	select {
	case ev := <-fr.Events():
		if ev.EventType != FlowEventNew || ev.State.PacketsIn != 7 || ev.State.PacketsOut != 9 {
			t.Fatalf("on return: type %d packets %d/%d, want new with 7/9", ev.EventType, ev.State.PacketsIn, ev.State.PacketsOut)
		}
	default:
		t.Fatal("a flow that came back was not announced")
	}
}

// The reader stalls for longer than the stale TTL while a flow it knows
// changes state and then goes quiet. The entry is stale by the time it is
// read, but it changed since the last complete poll, so it is read in full:
// the state change is reported and the expiry carries the final counters.
func TestFlowReader_StaleEntryChangedSinceLastPollIsReadInFull(t *testing.T) {
	set := setKtime(t, 1000*testSecond)
	keys, values := flowEntries(1)
	loader := newBatchMockLoader(keys, values, mapSweepBatch)
	fr := NewFlowReader(loader, nil, DefaultFlowReaderConfig())
	fr.pollOnce(context.Background())
	drainFlowEvents(fr)

	putFlow(loader.flows.fake, 0, makeFlowState(ConnStateClosed, 1000, 1010*testSecond, 40, 50, 4000, 5000))
	set(1100 * testSecond) // no poll between 1000 s and 1100 s
	fr.pollOnce(context.Background())

	var sawChange, sawExpired bool
	for {
		select {
		case ev := <-fr.Events():
			switch ev.EventType {
			case FlowEventStateChange:
				sawChange = ev.State.State == ConnStateClosed
			case FlowEventExpired:
				sawExpired = ev.State.PacketsIn == 40 && ev.State.BytesOut == 5000
			}
			continue
		default:
		}
		break
	}
	if !sawChange || !sawExpired {
		t.Fatalf("state change reported: %v; expiry with the final counters: %v; want both", sawChange, sawExpired)
	}
}

// A poll of a map full of dead flows allocates the same few objects whether
// there are a thousand of them or ten thousand.
func TestFlowReader_StaleEntriesCostNoAllocation(t *testing.T) {
	setKtime(t, 1000*testSecond)
	allocs := func(n int) float64 {
		keys, values := flowEntries(n)
		loader := newBatchMockLoader(keys, values, mapSweepBatch)
		for i := range n {
			putFlow(loader.flows.fake, i, makeFlowState(ConnStateEstablished, 1, 300*testSecond, 1, 1, 1, 1))
		}
		fr := NewFlowReader(loader, nil, DefaultFlowReaderConfig())
		fr.pollOnce(context.Background())
		return testing.AllocsPerRun(10, func() { fr.pollOnce(context.Background()) })
	}
	small, large := allocs(1_000), allocs(10_000)
	if small != large || large > 8 {
		t.Fatalf("allocations per poll: %v with 1,000 dead flows, %v with 10,000; want the same and at most 8", small, large)
	}
}

func putLatency(m *fakeBatchedMap, i int, e LatencyEntry) {
	v := e.Encode()
	m.values[i] = v[:]
}

// An entry with no sample since the last sweep is passed over on its stamp:
// no decode, no tracker entry, no allocation of its own.
func TestSweepUnchangedEntriesCostNoAllocation(t *testing.T) {
	setKtime(t, 1000*testSecond)
	keys, values := latencyEntries(10_000, LatencyTraceKeySize, 1) // stamped long before 1000 s
	m := &fakeBatchedMap{keys: keys, values: values, batchSize: mapSweepBatch}
	s, published := newTestSweeper(m)
	ctx := context.Background()
	if err := s.sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if len(*published) != 10_000 {
		t.Fatalf("first sweep published %d, want 10000", len(*published))
	}
	if err := s.sweep(ctx); err != nil { // the tracker lets go of them
		t.Fatal(err)
	}
	if got := len(s.rtts.seen); got != 0 {
		t.Fatalf("the tracker holds %d entries that have not changed for two sweeps, want 0", got)
	}
	*published = nil
	allocs := testing.AllocsPerRun(10, func() {
		if err := s.sweep(ctx); err != nil {
			t.Fatal(err)
		}
	})
	// The few that remain are the sweep's own closure and counters, not the entries.
	if allocs > 4 || len(*published) != 0 {
		t.Fatalf("a sweep of 10,000 unchanged entries: %v allocations, %d published; want at most 4 and 0", allocs, len(*published))
	}
}

// The stamp only rules entries out. Every new sample is still published
// exactly once, wherever it lands relative to a sweep.
func TestSweepStampFilterPublishesEachSampleOnce(t *testing.T) {
	set := setKtime(t, 200*testSecond)
	keys, values := latencyEntries(1, LatencyTraceKeySize, 1)
	m := &fakeBatchedMap{keys: keys, values: values, batchSize: mapSweepBatch}
	s, published := newTestSweeper(m)
	ctx := context.Background()
	sweepAt := func(at uint64) int {
		t.Helper()
		set(at * testSecond)
		*published = nil
		if err := s.sweep(ctx); err != nil {
			t.Fatal(err)
		}
		return len(*published)
	}
	sample := func(samples, at uint64) {
		putLatency(m, 0, LatencyEntry{RTTNS: 1500, MinRTTNS: 1000, MaxRTTNS: 2000, Samples: samples, LastUpdateNS: at * testSecond})
	}

	sample(1, 100)
	if n := sweepAt(200); n != 1 {
		t.Fatalf("first sweep published %d, want 1", n)
	}
	if n := sweepAt(300); n != 0 {
		t.Fatalf("no new sample: published %d", n)
	}
	sample(2, 350) // between two sweeps
	if n := sweepAt(400); n != 1 {
		t.Fatalf("a sample between sweeps: published %d, want 1", n)
	}
	sample(3, 450) // lands after the sweep at 400 began and before it read the entry
	*published = nil
	if err := s.sweep(ctx); err != nil { // the clock still reads 400
		t.Fatal(err)
	}
	if len(*published) != 1 {
		t.Fatalf("a sample during the sweep: published %d, want 1", len(*published))
	}
	if n := sweepAt(500); n != 0 {
		t.Fatalf("the sample seen during the last sweep was published again (%d)", n)
	}
	if n := sweepAt(600); n != 0 {
		t.Fatalf("published %d with no new sample", n)
	}
	sample(4, 650)
	if n := sweepAt(700); n != 1 {
		t.Fatalf("a sample after the tracker let go: published %d, want 1", n)
	}
}

// A sweep that failed part way does not move the mark: a sample older than
// the failed sweep and newer than the last complete one is still published.
func TestSweepPartialSweepDoesNotMoveTheMark(t *testing.T) {
	set := setKtime(t, 200*testSecond)
	keys, values := latencyEntries(2, LatencyTraceKeySize, 1)
	m := &fakeBatchedMap{keys: keys, values: values, batchSize: 1}
	s, published := newTestSweeper(m)
	ctx := context.Background()
	if err := s.sweep(ctx); err != nil {
		t.Fatal(err)
	}
	putLatency(m, 1, LatencyEntry{RTTNS: 1, MinRTTNS: 1, MaxRTTNS: 1, Samples: 2, LastUpdateNS: 250 * testSecond})

	set(300 * testSecond)
	m.batchCalls, m.failAtCall = 0, 1 // fails after the first entry, before the second
	if err := s.sweep(ctx); !errors.Is(err, errMidSweep) {
		t.Fatalf("err = %v, want the mid-sweep error", err)
	}
	set(400 * testSecond)
	m.failAtCall = 0
	*published = nil
	if err := s.sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if len(*published) != 1 {
		t.Fatalf("published %d after a failed sweep, want the one sample it missed", len(*published))
	}
}
