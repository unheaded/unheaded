// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package metrics

import (
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
	rtmetrics "runtime/metrics"
	"sync"
	"time"
)

// GoCollector publishes the go_* runtime series that prometheus/client_golang
// registers by default — the same 27 names, types, help text and units.
//
// Names and units are the contract, not an implementation detail: dashboards
// and alerts already query go_goroutines and go_memstats_heap_alloc_bytes,
// and those two series are how the B8 goroutine leak and the scraper OOM were
// diagnosed. A renamed series does not error; it silently flatlines a graph.
//
// Memory figures come from runtime/metrics, not runtime.ReadMemStats, using
// client_golang v1.18's derivation (memStatsFromRM). ReadMemStats stops the
// world; runtime/metrics does not. Deriving the same way is also what makes
// the two comparable at all — the fields are sums of different runtime
// classes, and two plausible-looking derivations disagree.
type GoCollector struct {
	mu      sync.Mutex
	samples []rtmetrics.Sample
	index   map[string]int
	ttl     time.Duration
	taken   time.Time
	snap    goSnapshot
}

// goSnapshot is one reading of the runtime, shared by every go_* family so
// that a scrape's series are mutually consistent: heap_sys must equal
// heap_inuse + heap_idle, which only holds if both came from one read.
type goSnapshot struct {
	goroutines, threads float64

	gcCount     uint64
	gcPauseSum  float64
	gcQuantiles [5]float64 // min, 25%, 50%, 75%, max — debug.ReadGCStats' order
	lastGC      float64

	alloc, totalAlloc, sys, lookups, mallocs, frees              float64
	heapAlloc, heapSys, heapIdle, heapInuse, heapReleased        float64
	heapObjects, stackInuse, stackSys, mspanInuse, mspanSys      float64
	mcacheInuse, mcacheSys, buckHashSys, gcSys, otherSys, nextGC float64
}

// runtime/metrics keys read by the memstats derivation.
const (
	rmTinyAllocs    = "/gc/heap/tiny/allocs:objects"
	rmAllocsObjects = "/gc/heap/allocs:objects"
	rmFreesObjects  = "/gc/heap/frees:objects"
	rmAllocsBytes   = "/gc/heap/allocs:bytes"
	rmHeapObjects   = "/gc/heap/objects:objects"
	rmHeapGoal      = "/gc/heap/goal:bytes"
	rmTotal         = "/memory/classes/total:bytes"
	rmHeapObjBytes  = "/memory/classes/heap/objects:bytes"
	rmHeapUnused    = "/memory/classes/heap/unused:bytes"
	rmHeapReleased  = "/memory/classes/heap/released:bytes"
	rmHeapFree      = "/memory/classes/heap/free:bytes"
	rmHeapStacks    = "/memory/classes/heap/stacks:bytes"
	rmOSStacks      = "/memory/classes/os-stacks:bytes"
	rmMSpanInuse    = "/memory/classes/metadata/mspan/inuse:bytes"
	rmMSpanFree     = "/memory/classes/metadata/mspan/free:bytes"
	rmMCacheInuse   = "/memory/classes/metadata/mcache/inuse:bytes"
	rmMCacheFree    = "/memory/classes/metadata/mcache/free:bytes"
	rmProfBuckets   = "/memory/classes/profiling/buckets:bytes"
	rmMetadataOther = "/memory/classes/metadata/other:bytes"
	rmOther         = "/memory/classes/other:bytes"
)

var goRMKeys = []string{
	rmTinyAllocs, rmAllocsObjects, rmFreesObjects, rmAllocsBytes, rmHeapObjects,
	rmHeapGoal, rmTotal, rmHeapObjBytes, rmHeapUnused, rmHeapReleased, rmHeapFree,
	rmHeapStacks, rmOSStacks, rmMSpanInuse, rmMSpanFree, rmMCacheInuse,
	rmMCacheFree, rmProfBuckets, rmMetadataOther, rmOther,
}

// goSnapshotTTL bounds how long one runtime reading is reused. It must be
// longer than a Gather takes, so every family in a scrape shares one reading,
// and far shorter than any scrape interval, so each scrape gets a fresh one.
const goSnapshotTTL = 500 * time.Millisecond

// NewGoCollector returns a collector for the go_* runtime series. Register it
// with Register(reg); it contributes one Collector per metric family.
func NewGoCollector() *GoCollector {
	gc := &GoCollector{index: make(map[string]int), ttl: goSnapshotTTL}

	// Keep only the keys this runtime supports. A key the runtime does not
	// know reads as KindBad, and a missing class then reads as 0 — the same
	// best-effort behaviour as client_golang, rather than a panic on upgrade.
	supported := make(map[string]bool)
	for _, d := range rtmetrics.All() {
		supported[d.Name] = true
	}
	for _, k := range goRMKeys {
		if supported[k] {
			gc.index[k] = len(gc.samples)
			gc.samples = append(gc.samples, rtmetrics.Sample{Name: k})
		}
	}
	return gc
}

// snapshot returns a reading no older than the TTL.
func (gc *GoCollector) snapshot() goSnapshot {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	if !gc.taken.IsZero() && time.Since(gc.taken) < gc.ttl {
		return gc.snap
	}
	gc.snap = gc.read()
	gc.taken = time.Now()
	return gc.snap
}

// read takes a fresh reading. Caller holds gc.mu (samples is reused).
func (gc *GoCollector) read() goSnapshot {
	var s goSnapshot

	s.goroutines = float64(runtime.NumGoroutine())
	n, _ := runtime.ThreadCreateProfile(nil)
	s.threads = float64(n)

	var st debug.GCStats
	st.PauseQuantiles = make([]time.Duration, 5)
	debug.ReadGCStats(&st)
	s.gcCount = uint64(st.NumGC)
	s.gcPauseSum = st.PauseTotal.Seconds()
	for i, q := range st.PauseQuantiles {
		s.gcQuantiles[i] = q.Seconds()
	}
	s.lastGC = float64(st.LastGC.UnixNano()) / 1e9

	rtmetrics.Read(gc.samples)
	deriveMemstats(&s, func(k string) uint64 {
		i, ok := gc.index[k]
		if !ok || gc.samples[i].Value.Kind() != rtmetrics.KindUint64 {
			return 0
		}
		return gc.samples[i].Value.Uint64()
	})
	return s
}

// deriveMemstats fills the go_memstats_* fields from runtime/metrics classes.
// It is a pure function of get so the sums can be tested class by class: on a
// cgo-free Linux binary several classes (os-stacks among them) read 0, and a
// sum that drops one is indistinguishable from a correct one at runtime.
func deriveMemstats(s *goSnapshot, get func(string) uint64) {
	// Mirrors client_golang v1.18 memStatsFromRM. MemStats counts tiny
	// allocations in both Mallocs and Frees, so Mallocs-Frees stays a live
	// object count; the runtime/metrics derivation preserves that.
	tiny := get(rmTinyAllocs)
	heapAlloc := get(rmHeapObjBytes)
	heapInuse := heapAlloc + get(rmHeapUnused)
	heapReleased := get(rmHeapReleased)
	heapIdle := heapReleased + get(rmHeapFree)
	stackInuse := get(rmHeapStacks)
	mspanInuse := get(rmMSpanInuse)
	mcacheInuse := get(rmMCacheInuse)

	s.mallocs = float64(get(rmAllocsObjects) + tiny)
	s.frees = float64(get(rmFreesObjects) + tiny)
	s.totalAlloc = float64(get(rmAllocsBytes))
	s.sys = float64(get(rmTotal))
	s.lookups = 0 // the runtime has always reported zero
	s.heapAlloc = float64(heapAlloc)
	s.alloc = s.heapAlloc
	s.heapInuse = float64(heapInuse)
	s.heapReleased = float64(heapReleased)
	s.heapIdle = float64(heapIdle)
	s.heapSys = float64(heapInuse + heapIdle)
	s.heapObjects = float64(get(rmHeapObjects))
	s.stackInuse = float64(stackInuse)
	s.stackSys = float64(stackInuse + get(rmOSStacks))
	s.mspanInuse = float64(mspanInuse)
	s.mspanSys = float64(mspanInuse + get(rmMSpanFree))
	s.mcacheInuse = float64(mcacheInuse)
	s.mcacheSys = float64(mcacheInuse + get(rmMCacheFree))
	s.buckHashSys = float64(get(rmProfBuckets))
	s.gcSys = float64(get(rmMetadataOther))
	s.otherSys = float64(get(rmOther))
	s.nextGC = float64(get(rmHeapGoal))
}

// Collectors returns one Collector per go_* family.
func (gc *GoCollector) Collectors() []Collector {
	g := func(name, help string, f func(goSnapshot) float64) Collector {
		return NewFuncGauge(name, help, nil, func() float64 { return f(gc.snapshot()) })
	}
	c := func(name, help string, f func(goSnapshot) float64) Collector {
		return NewFuncCounter(name, help, nil, func() float64 { return f(gc.snapshot()) })
	}
	m := func(suffix string) string { return "go_memstats_" + suffix }

	return []Collector{
		&goGCSummary{gc: gc},
		g("go_goroutines", "Number of goroutines that currently exist.", func(s goSnapshot) float64 { return s.goroutines }),
		NewFuncGauge("go_info", "Information about the Go environment.", Labels{"version": runtime.Version()}, func() float64 { return 1 }),
		g(m("alloc_bytes"), "Number of bytes allocated and still in use.", func(s goSnapshot) float64 { return s.alloc }),
		c(m("alloc_bytes_total"), "Total number of bytes allocated, even if freed.", func(s goSnapshot) float64 { return s.totalAlloc }),
		g(m("buck_hash_sys_bytes"), "Number of bytes used by the profiling bucket hash table.", func(s goSnapshot) float64 { return s.buckHashSys }),
		c(m("frees_total"), "Total number of frees.", func(s goSnapshot) float64 { return s.frees }),
		g(m("gc_sys_bytes"), "Number of bytes used for garbage collection system metadata.", func(s goSnapshot) float64 { return s.gcSys }),
		g(m("heap_alloc_bytes"), "Number of heap bytes allocated and still in use.", func(s goSnapshot) float64 { return s.heapAlloc }),
		g(m("heap_idle_bytes"), "Number of heap bytes waiting to be used.", func(s goSnapshot) float64 { return s.heapIdle }),
		g(m("heap_inuse_bytes"), "Number of heap bytes that are in use.", func(s goSnapshot) float64 { return s.heapInuse }),
		g(m("heap_objects"), "Number of allocated objects.", func(s goSnapshot) float64 { return s.heapObjects }),
		g(m("heap_released_bytes"), "Number of heap bytes released to OS.", func(s goSnapshot) float64 { return s.heapReleased }),
		g(m("heap_sys_bytes"), "Number of heap bytes obtained from system.", func(s goSnapshot) float64 { return s.heapSys }),
		g(m("last_gc_time_seconds"), "Number of seconds since 1970 of last garbage collection.", func(s goSnapshot) float64 { return s.lastGC }),
		c(m("lookups_total"), "Total number of pointer lookups.", func(s goSnapshot) float64 { return s.lookups }),
		c(m("mallocs_total"), "Total number of mallocs.", func(s goSnapshot) float64 { return s.mallocs }),
		g(m("mcache_inuse_bytes"), "Number of bytes in use by mcache structures.", func(s goSnapshot) float64 { return s.mcacheInuse }),
		g(m("mcache_sys_bytes"), "Number of bytes used for mcache structures obtained from system.", func(s goSnapshot) float64 { return s.mcacheSys }),
		g(m("mspan_inuse_bytes"), "Number of bytes in use by mspan structures.", func(s goSnapshot) float64 { return s.mspanInuse }),
		g(m("mspan_sys_bytes"), "Number of bytes used for mspan structures obtained from system.", func(s goSnapshot) float64 { return s.mspanSys }),
		g(m("next_gc_bytes"), "Number of heap bytes when next garbage collection will take place.", func(s goSnapshot) float64 { return s.nextGC }),
		g(m("other_sys_bytes"), "Number of bytes used for other system allocations.", func(s goSnapshot) float64 { return s.otherSys }),
		g(m("stack_inuse_bytes"), "Number of bytes in use by the stack allocator.", func(s goSnapshot) float64 { return s.stackInuse }),
		g(m("stack_sys_bytes"), "Number of bytes obtained from system for stack allocator.", func(s goSnapshot) float64 { return s.stackSys }),
		g(m("sys_bytes"), "Number of bytes obtained from system.", func(s goSnapshot) float64 { return s.sys }),
		g("go_threads", "Number of OS threads created.", func(s goSnapshot) float64 { return s.threads }),
	}
}

// Register adds every go_* family to reg. It stops at the first conflict, so
// a registry that already carries one of these names is reported rather than
// left half-populated without explanation.
func (gc *GoCollector) Register(reg *Registry) error {
	for _, c := range gc.Collectors() {
		if err := reg.Register(c); err != nil {
			return fmt.Errorf("go collector: %w", err)
		}
	}
	return nil
}

// goGCSummary is go_gc_duration_seconds: a summary built from the runtime's
// own pause quantiles rather than from observations, so it cannot be a
// metrics.Summary.
type goGCSummary struct {
	gc *GoCollector
}

var goGCSummaryDesc = NewDesc("go_gc_duration_seconds",
	"A summary of the pause duration of garbage collection cycles.", TypeSummary, nil, nil)

func (g *goGCSummary) Describe() *Desc { return goGCSummaryDesc }

func (g *goGCSummary) Write(w io.Writer) error {
	s := g.gc.snapshot()
	for i, q := range []string{"0", "0.25", "0.5", "0.75", "1"} {
		if _, err := fmt.Fprintf(w, "go_gc_duration_seconds{quantile=%q} %s\n", q, formatFloat(s.gcQuantiles[i])); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "go_gc_duration_seconds_sum %s\ngo_gc_duration_seconds_count %d\n",
		formatFloat(s.gcPauseSum), s.gcCount); err != nil {
		return err
	}
	return nil
}
