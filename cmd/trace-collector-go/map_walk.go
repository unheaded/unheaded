// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// map_walk.go — one pass over a BPF hash map, read in batches.
//
// Walking a map with get-next-key and lookup is two bpf() calls per entry.
// The latency sweep did that every 200 ms and the flow reader every second;
// each cost grew with its map until it was most of a core (FND-044, ADR-144).
// A batch lookup reads thousands of entries per call. Both readers share the
// loop here; each keeps what it does with an entry.
package main

import (
	"context"
	"errors"
	"fmt"

	ciliumebpf "github.com/cilium/ebpf"
	"github.com/rs/zerolog/log"

	"unheaded/pkg/metrics/auto"
	"unheaded/pkg/metrics/prom"
)

// mapSweepBatch is how many entries one batch lookup asks the kernel for. A
// full 65,536 entry map is 16 batches and a final short one.
const mapSweepBatch = 4096

// mapUnbatchedSweeps counts the passes that fell back to the per-entry walk.
// The label is the reader's fixed name ("latency", "flows"), nothing else.
var mapUnbatchedSweeps = auto.NewCounterVec(prom.CounterOpts{
	Name: "trace_collector_map_unbatched_sweeps_total",
	Help: "Sweeps that read a BPF map one entry at a time (two bpf() calls per entry) because it has no batch lookup here; not zero means the collector's CPU grows with that map",
}, []string{"map"})

// batchedMap is what a pass needs from a BPF map. The kernel map is behind
// ebpfBatchMap; tests use a fake.
type batchedMap interface {
	// batch reads the next batch of entries and calls visit for each one.
	// first starts a new pass over the map. done reports that the pass
	// reached the end. The slices passed to visit are reused: copy to keep.
	batch(first bool, visit func(key, value []byte)) (done bool, err error)
	// walk visits every entry one at a time, for a map that has no batch
	// lookup (an old kernel, or a reader with no batched view).
	walk(visit func(key, value []byte)) error
}

// mapWalker makes passes over one map. It is used from one goroutine.
type mapWalker struct {
	name string // the reader's name, and the metric label
	m    batchedMap

	// noBatch is set the first time the batch lookup is refused; the passes
	// after that walk the map.
	noBatch    bool
	onFallback func(error)
}

func newMapWalker(name string, m batchedMap) *mapWalker {
	return &mapWalker{
		name: name,
		m:    m,
		onFallback: func(err error) {
			log.Warn().Err(err).Str("map", name).Msg("no batch lookup for this map; reading it one entry at a time (two bpf() calls per entry)")
		},
	}
}

// read visits every entry, in batches where the map allows it. On an error
// the pass was partial: the caller must not conclude that an entry it did
// not see is gone.
func (w *mapWalker) read(ctx context.Context, visit func(key, value []byte)) error {
	if w.noBatch {
		return w.walk(visit)
	}
	first := true
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		done, err := w.m.batch(first, visit)
		if first && errors.Is(err, ciliumebpf.ErrNotSupported) {
			w.noBatch = true
			w.onFallback(err)
			return w.walk(visit)
		}
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		first = false
	}
}

func (w *mapWalker) walk(visit func(key, value []byte)) error {
	mapUnbatchedSweeps.WithLabelValues(w.name).Inc()
	return w.m.walk(visit)
}

// ebpfBatchMap reads a kernel hash map whose key is a K and whose value is a
// V (byte arrays of the map's sizes). Its buffers are allocated once and
// reused by every pass.
type ebpfBatchMap[K, V any] struct {
	m          *ciliumebpf.Map
	cursor     ciliumebpf.MapBatchCursor
	keys       []K
	values     []V
	keyBytes   func(*K) []byte
	valueBytes func(*V) []byte
	// walkFn replaces the per-entry walk; nil walks with cilium's iterator.
	walkFn func(visit func(key, value []byte)) error
}

func newEBPFBatchMap[K, V any](m *ciliumebpf.Map, keyBytes func(*K) []byte, valueBytes func(*V) []byte) *ebpfBatchMap[K, V] {
	return &ebpfBatchMap[K, V]{
		m:          m,
		keys:       make([]K, mapSweepBatch),
		values:     make([]V, mapSweepBatch),
		keyBytes:   keyBytes,
		valueBytes: valueBytes,
	}
}

func (e *ebpfBatchMap[K, V]) batch(first bool, visit func(key, value []byte)) (bool, error) {
	if first {
		e.cursor = ciliumebpf.MapBatchCursor{}
	}
	n, err := e.m.BatchLookup(&e.cursor, e.keys, e.values, nil)
	// The kernel reports the end of the map as "key does not exist", together
	// with the last entries.
	done := errors.Is(err, ciliumebpf.ErrKeyNotExist)
	if err != nil && !done {
		return false, err
	}
	for i := range n {
		visit(e.keyBytes(&e.keys[i]), e.valueBytes(&e.values[i]))
	}
	return done, nil
}

func (e *ebpfBatchMap[K, V]) walk(visit func(key, value []byte)) error {
	if e.walkFn != nil {
		return e.walkFn(visit)
	}
	iter := e.m.Iterate()
	var key, value []byte
	for iter.Next(&key, &value) {
		visit(key, value)
	}
	return iter.Err()
}

// iterateOnly is a MapReader with no batched view: every pass is a walk.
type iterateOnly struct {
	mr MapReader
}

func (i iterateOnly) batch(bool, func(key, value []byte)) (bool, error) {
	return false, fmt.Errorf("this map reader has no batched view: %w", ciliumebpf.ErrNotSupported)
}

func (i iterateOnly) walk(visit func(key, value []byte)) error {
	return walkMapReader(i.mr, visit)
}

func walkMapReader(mr MapReader, visit func(key, value []byte)) error {
	return mr.Iterate(func(key, value []byte) error {
		visit(key, value)
		return nil
	})
}

// batchSource is a MapReader that can also be read in batches.
type batchSource interface {
	batchView() (batchedMap, error)
}

// newFlowsBatchMap opens a batched view of flow_tracker's FLOWS map on fd, a
// descriptor the caller hands over (it is closed here on an error, and by the
// view otherwise). The per-entry walk stays the MapReader's own Iterate. A map
// that is not FLOWS's shape is refused, so it is walked and its entries are
// counted as decode errors, as before.
func newFlowsBatchMap(fd int, mr MapReader) (batchedMap, error) {
	m, err := ciliumebpf.NewMapFromFD(fd) // owns fd from here, closes it on an error
	if err != nil {
		return nil, fmt.Errorf("open FLOWS for batch lookups: %w", err)
	}
	if m.KeySize() != FlowKeySize || m.ValueSize() != FlowStateSize {
		_ = m.Close()
		return nil, fmt.Errorf("FLOWS has key %d and value %d bytes, want %d and %d", m.KeySize(), m.ValueSize(), FlowKeySize, FlowStateSize)
	}
	view := newEBPFBatchMap(m,
		func(k *[FlowKeySize]byte) []byte { return k[:] },
		func(v *[FlowStateSize]byte) []byte { return v[:] })
	view.walkFn = func(visit func(key, value []byte)) error { return walkMapReader(mr, visit) }
	return view, nil
}
