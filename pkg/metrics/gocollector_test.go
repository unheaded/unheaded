// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package metrics

import (
	"bufio"
	"bytes"
	"math"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/common/expfmt"
)

// family is one metric family parsed out of text exposition.
type family struct {
	help, typ string
	samples   map[string]float64 // series (name + labels) -> value
}

// parseExposition is deliberately strict: a sample with no preceding # TYPE
// for its family is an error, because that is the malformed-exposition class
// pkg/metrics exists to rule out.
func parseExposition(t *testing.T, text string) map[string]*family {
	t.Helper()
	fams := make(map[string]*family)
	get := func(name string) *family {
		if fams[name] == nil {
			fams[name] = &family{samples: make(map[string]float64)}
		}
		return fams[name]
	}
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
		case strings.HasPrefix(line, "# HELP "):
			parts := strings.SplitN(strings.TrimPrefix(line, "# HELP "), " ", 2)
			get(parts[0]).help = parts[1]
		case strings.HasPrefix(line, "# TYPE "):
			parts := strings.SplitN(strings.TrimPrefix(line, "# TYPE "), " ", 2)
			f := get(parts[0])
			if f.typ != "" {
				t.Errorf("duplicate # TYPE for %s", parts[0])
			}
			f.typ = parts[1]
		default:
			sp := strings.LastIndexByte(line, ' ')
			series, val := line[:sp], line[sp+1:]
			name := series
			if i := strings.IndexByte(name, '{'); i >= 0 {
				name = name[:i]
			}
			base := name
			for _, suf := range []string{"_sum", "_count"} {
				if b := strings.TrimSuffix(name, suf); b != name && fams[b] != nil && fams[b].typ == "summary" {
					base = b
				}
			}
			f := fams[base]
			if f == nil || f.typ == "" {
				t.Errorf("sample %q has no # TYPE", series)
				continue
			}
			v, err := strconv.ParseFloat(val, 64)
			if err != nil {
				t.Errorf("sample %q: bad value %q", series, val)
				continue
			}
			f.samples[series] = v
		}
	}
	return fams
}

func gatherOurs(t *testing.T) map[string]*family {
	t.Helper()
	reg := NewRegistry()
	if err := NewGoCollector().Register(reg); err != nil {
		t.Fatalf("register: %v", err)
	}
	var buf bytes.Buffer
	if err := reg.Gather(&buf); err != nil {
		t.Fatalf("gather: %v", err)
	}
	return parseExposition(t, buf.String())
}

func gatherTheirs(t *testing.T) map[string]*family {
	t.Helper()
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("client_golang gather: %v", err)
	}
	var buf bytes.Buffer
	for _, mf := range mfs {
		if _, err := expfmt.MetricFamilyToText(&buf, mf); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}
	return parseExposition(t, buf.String())
}

// The names are the contract. This list is what client_golang v1.18
// publishes by default; it is pinned here without importing client_golang so
// it survives ADR-094 step 6, when the parity test below goes with the
// dependency.
func TestGoCollector_PublishesTheClientGolangNames(t *testing.T) {
	want := map[string]string{
		"go_gc_duration_seconds":           "summary",
		"go_goroutines":                    "gauge",
		"go_info":                          "gauge",
		"go_memstats_alloc_bytes":          "gauge",
		"go_memstats_alloc_bytes_total":    "counter",
		"go_memstats_buck_hash_sys_bytes":  "gauge",
		"go_memstats_frees_total":          "counter",
		"go_memstats_gc_sys_bytes":         "gauge",
		"go_memstats_heap_alloc_bytes":     "gauge",
		"go_memstats_heap_idle_bytes":      "gauge",
		"go_memstats_heap_inuse_bytes":     "gauge",
		"go_memstats_heap_objects":         "gauge",
		"go_memstats_heap_released_bytes":  "gauge",
		"go_memstats_heap_sys_bytes":       "gauge",
		"go_memstats_last_gc_time_seconds": "gauge",
		"go_memstats_lookups_total":        "counter",
		"go_memstats_mallocs_total":        "counter",
		"go_memstats_mcache_inuse_bytes":   "gauge",
		"go_memstats_mcache_sys_bytes":     "gauge",
		"go_memstats_mspan_inuse_bytes":    "gauge",
		"go_memstats_mspan_sys_bytes":      "gauge",
		"go_memstats_next_gc_bytes":        "gauge",
		"go_memstats_other_sys_bytes":      "gauge",
		"go_memstats_stack_inuse_bytes":    "gauge",
		"go_memstats_stack_sys_bytes":      "gauge",
		"go_memstats_sys_bytes":            "gauge",
		"go_threads":                       "gauge",
	}
	got := gatherOurs(t)
	for name, typ := range want {
		f := got[name]
		if f == nil {
			t.Errorf("missing %s", name)
			continue
		}
		if f.typ != typ {
			t.Errorf("%s: type %s, want %s", name, f.typ, typ)
		}
		if len(f.samples) == 0 {
			t.Errorf("%s: declared but no sample", name)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("unexpected family %s", name)
		}
	}
}

// Side by side in one process: every name, type and help string identical,
// and every client_golang value bracketed by two readings of ours.
//
// Two independent reads of a live runtime never agree exactly, and a
// tolerance wide enough to absorb that also absorbs real errors. So: pause
// the GC, read ours, read client_golang, read ours again. With no collection
// between them the heap only grows, and client_golang's reading must fall
// between our two. For almost every series that needs no tolerance at all.
func TestGoCollector_MatchesClientGolang(t *testing.T) {
	runtime.GC()
	defer debug.SetGCPercent(debug.SetGCPercent(-1))

	before := gatherOurs(t)
	theirs := gatherTheirs(t)
	after := gatherOurs(t)

	names := make([]string, 0, len(theirs))
	for n := range theirs {
		names = append(names, n)
	}
	sort.Strings(names)
	if len(before) != len(theirs) {
		t.Errorf("family count: ours %d, client_golang %d", len(before), len(theirs))
	}

	for _, name := range names {
		tf, bf, af := theirs[name], before[name], after[name]
		if bf == nil {
			t.Errorf("%s: client_golang publishes it, we do not", name)
			continue
		}
		if bf.typ != tf.typ {
			t.Errorf("%s: type %s, client_golang %s", name, bf.typ, tf.typ)
		}
		if bf.help != tf.help {
			t.Errorf("%s: help %q, client_golang %q", name, bf.help, tf.help)
		}
		for series, tv := range tf.samples {
			bv, ok := bf.samples[series]
			if !ok {
				t.Errorf("%s: series %s missing", name, series)
				continue
			}
			av := af.samples[series]
			lo, hi := math.Min(bv, av)-slack(name), math.Max(bv, av)+slack(name)
			if tv < lo || tv > hi {
				t.Errorf("%s: client_golang %g outside ours [%g, %g]", series, tv, bv, av)
			}
		}
		for series := range bf.samples {
			if _, ok := tf.samples[series]; !ok {
				t.Errorf("%s: extra series %s", name, series)
			}
		}
	}
}

// slack is what may escape the bracket, measured over 300 bracketed runs on
// go1.25. Everything not listed escaped zero times and gets zero.
func slack(name string) float64 {
	switch name {
	case "go_goroutines":
		// client_golang's Gather collects on its own goroutines, so its
		// reading sees them and ours do not: +2 on every run.
		return 4
	case "go_threads":
		return 2
	case "go_memstats_gc_sys_bytes", "go_memstats_other_sys_bytes":
		// The runtime moves metadata between these two classes; their sum
		// is stable. Largest move seen: 13312.
		return 32 << 10
	case "go_memstats_next_gc_bytes":
		// The heap goal counts stack space (Go 1.21+), which changes with
		// the GC paused. Largest move seen: 6144.
		return 32 << 10
	}
	return 0
}

// The derived fields are sums of runtime classes. If the sums are wrong the
// numbers still look plausible, so check the identities MemStats documents.
func TestGoCollector_MemstatsIdentities(t *testing.T) {
	gc := NewGoCollector()
	s := gc.snapshot()
	if s.heapSys != s.heapInuse+s.heapIdle {
		t.Errorf("heap_sys %g != heap_inuse %g + heap_idle %g", s.heapSys, s.heapInuse, s.heapIdle)
	}
	if s.alloc != s.heapAlloc {
		t.Errorf("alloc %g != heap_alloc %g", s.alloc, s.heapAlloc)
	}
	if s.mallocs < s.frees {
		t.Errorf("mallocs %g < frees %g", s.mallocs, s.frees)
	}
	if s.sys < s.heapSys+s.stackSys {
		t.Errorf("sys %g < heap_sys %g + stack_sys %g", s.sys, s.heapSys, s.stackSys)
	}
	if s.heapAlloc == 0 || s.sys == 0 || s.nextGC == 0 {
		t.Errorf("zero where the runtime cannot be zero: %+v", s)
	}
}

// Every family in one Gather must come from one runtime reading, or
// heap_sys != heap_inuse + heap_idle can appear within a single scrape.
func TestGoCollector_OneReadingPerScrape(t *testing.T) {
	gc := NewGoCollector()
	reg := NewRegistry()
	if err := gc.Register(reg); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := reg.Gather(&buf); err != nil {
		t.Fatal(err)
	}
	fams := parseExposition(t, buf.String())
	v := func(n string) float64 { return fams[n].samples[n] }
	if v("go_memstats_heap_sys_bytes") != v("go_memstats_heap_inuse_bytes")+v("go_memstats_heap_idle_bytes") {
		t.Errorf("scrape mixed readings: sys %g inuse %g idle %g",
			v("go_memstats_heap_sys_bytes"), v("go_memstats_heap_inuse_bytes"), v("go_memstats_heap_idle_bytes"))
	}
}

func TestGoCollector_RegisterConflictIsReported(t *testing.T) {
	reg := NewRegistry()
	reg.MustRegister(NewGauge("go_goroutines", "already here", nil))
	if err := NewGoCollector().Register(reg); err == nil {
		t.Fatal("want conflict error, got nil")
	}
}

// Each runtime/metrics class gets its own bit, so every derived field must
// equal exactly the OR of the classes it sums. A dropped class is a missing
// bit and a wrong class is a stray one — including classes that read 0 on
// this host and so cannot be caught by comparing live values.
func TestDeriveMemstats_SumsTheRightClasses(t *testing.T) {
	bit := make(map[string]uint64, len(goRMKeys))
	for i, k := range goRMKeys {
		bit[k] = 1 << i
	}
	var s goSnapshot
	deriveMemstats(&s, func(k string) uint64 { return bit[k] })

	sum := func(keys ...string) float64 {
		var v uint64
		for _, k := range keys {
			v |= bit[k]
		}
		return float64(v)
	}
	cases := []struct {
		field string
		got   float64
		want  float64
	}{
		{"mallocs", s.mallocs, sum(rmAllocsObjects, rmTinyAllocs)},
		{"frees", s.frees, sum(rmFreesObjects, rmTinyAllocs)},
		{"totalAlloc", s.totalAlloc, sum(rmAllocsBytes)},
		{"sys", s.sys, sum(rmTotal)},
		{"lookups", s.lookups, 0},
		{"alloc", s.alloc, sum(rmHeapObjBytes)},
		{"heapAlloc", s.heapAlloc, sum(rmHeapObjBytes)},
		{"heapInuse", s.heapInuse, sum(rmHeapObjBytes, rmHeapUnused)},
		{"heapReleased", s.heapReleased, sum(rmHeapReleased)},
		{"heapIdle", s.heapIdle, sum(rmHeapReleased, rmHeapFree)},
		{"heapSys", s.heapSys, sum(rmHeapObjBytes, rmHeapUnused, rmHeapReleased, rmHeapFree)},
		{"heapObjects", s.heapObjects, sum(rmHeapObjects)},
		{"stackInuse", s.stackInuse, sum(rmHeapStacks)},
		{"stackSys", s.stackSys, sum(rmHeapStacks, rmOSStacks)},
		{"mspanInuse", s.mspanInuse, sum(rmMSpanInuse)},
		{"mspanSys", s.mspanSys, sum(rmMSpanInuse, rmMSpanFree)},
		{"mcacheInuse", s.mcacheInuse, sum(rmMCacheInuse)},
		{"mcacheSys", s.mcacheSys, sum(rmMCacheInuse, rmMCacheFree)},
		{"buckHashSys", s.buckHashSys, sum(rmProfBuckets)},
		{"gcSys", s.gcSys, sum(rmMetadataOther)},
		{"otherSys", s.otherSys, sum(rmOther)},
		{"nextGC", s.nextGC, sum(rmHeapGoal)},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %#x, want %#x", c.field, uint64(c.got), uint64(c.want))
		}
	}
}

// A runtime/metrics key the running Go does not know reads as 0 without
// error. If a Go upgrade renames one, a go_memstats_* series flatlines while
// every other test here stays green — so fail the upgrade instead.
func TestGoCollector_EveryRuntimeKeyExists(t *testing.T) {
	gc := NewGoCollector()
	for _, k := range goRMKeys {
		if _, ok := gc.index[k]; !ok {
			t.Errorf("runtime/metrics has no %q on %s", k, runtime.Version())
		}
	}
}
