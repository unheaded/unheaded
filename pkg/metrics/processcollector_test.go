// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package metrics

import (
	"bytes"
	"math"
	"os"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"syscall"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/common/expfmt"
)

func gatherOurProcess(t *testing.T, namespace string) map[string]*family {
	t.Helper()
	reg := NewRegistry()
	if err := NewProcessCollector(namespace).Register(reg); err != nil {
		t.Fatalf("register: %v", err)
	}
	var buf bytes.Buffer
	if err := reg.Gather(&buf); err != nil {
		t.Fatalf("gather: %v", err)
	}
	return parseExposition(t, buf.String())
}

func gatherTheirProcess(t *testing.T) map[string]*family {
	t.Helper()
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
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

// Pinned without client_golang so it outlives the parity test (ADR-094 step 6).
func TestProcessCollector_PublishesTheClientGolangNames(t *testing.T) {
	want := map[string]string{
		"process_cpu_seconds_total":        "counter",
		"process_max_fds":                  "gauge",
		"process_open_fds":                 "gauge",
		"process_resident_memory_bytes":    "gauge",
		"process_start_time_seconds":       "gauge",
		"process_virtual_memory_bytes":     "gauge",
		"process_virtual_memory_max_bytes": "gauge",
	}
	got := gatherOurProcess(t, "")
	for name, typ := range want {
		f := got[name]
		if f == nil {
			t.Errorf("missing %s", name)
			continue
		}
		if f.typ != typ {
			t.Errorf("%s: type %s, want %s", name, f.typ, typ)
		}
		if len(f.samples) != 1 {
			t.Errorf("%s: %d samples, want 1", name, len(f.samples))
		}
	}
	if len(got) != len(want) {
		t.Errorf("%d families, want %d", len(got), len(want))
	}
}

// Same bracket as the go_* parity test: ours, client_golang, ours again.
func TestProcessCollector_MatchesClientGolang(t *testing.T) {
	runtime.GC()
	defer debug.SetGCPercent(debug.SetGCPercent(-1))

	before := gatherOurProcess(t, "")
	theirs := gatherTheirProcess(t)
	after := gatherOurProcess(t, "")

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
		if bf.typ != tf.typ || bf.help != tf.help {
			t.Errorf("%s: ours %s %q, client_golang %s %q", name, bf.typ, bf.help, tf.typ, tf.help)
		}
		for series, tv := range tf.samples {
			bv, ok := bf.samples[series]
			if !ok {
				t.Errorf("series %s missing", series)
				continue
			}
			av := af.samples[series]
			lo, hi := math.Min(bv, av)-procSlack(name), math.Max(bv, av)+procSlack(name)
			if tv < lo || tv > hi {
				t.Errorf("%s: client_golang %g outside ours [%g, %g]", series, tv, bv, av)
			}
		}
	}
}

// procSlack: what may escape the bracket. See the ADR-094 step 1b note.
func procSlack(name string) float64 {
	switch name {
	case "process_start_time_seconds":
		// Both compute btime + starttime/100; float rounding only.
		return 1e-6
	}
	return 0
}

// comm is attacker-influenced (prctl PR_SET_NAME, or just the binary name)
// and may contain spaces and ')'. Counting from the first ')' or splitting
// the line on spaces reads every later field from the wrong column.
func TestParseProcStat_CommWithParensAndSpaces(t *testing.T) {
	line := "4242 (evil) (x y) S 1 4242 4242 0 -1 4194560 100 0 0 0 " +
		"250 50 0 0 20 0 9 0 777 123456789 2048 18446744073709551615 1 1 0 0 0 0 0 0 0 0 0 0 17 3 0 0 0 0 0\n"
	st, err := parseProcStatBytes([]byte(line))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if st.utime != 250 || st.stime != 50 || st.starttime != 777 || st.vsize != 123456789 || st.rss != 2048 {
		t.Errorf("got %+v, want utime=250 stime=50 starttime=777 vsize=123456789 rss=2048", st)
	}
}

func TestParseProcStat_RejectsTruncated(t *testing.T) {
	for _, in := range []string{"", "1 (x", "1 (x) S 1 2 3", "1 (x) S 1 1 1 0 -1 0 0 0 0 0 NaN 0 0 0 20 0 1 0 5 6 7"} {
		if _, err := parseProcStatBytes([]byte(in)); err == nil {
			t.Errorf("%q: want error", in)
		}
	}
}

// A failed read omits the sample; it must not publish 0, which on a counter
// reads as a reset.
func TestOptionalMetric_OmitsSampleOnFailure(t *testing.T) {
	reg := NewRegistry()
	reg.MustRegister(&optionalMetric{
		desc: NewDesc("x_total", "x", TypeCounter, nil, nil),
		read: func() (float64, bool) { return 0, false },
	})
	var buf bytes.Buffer
	if err := reg.Gather(&buf); err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.HasPrefix(line, "x_total") {
			t.Errorf("sample published on failed read: %q", line)
		}
	}
}

func TestProcessCollector_Namespace(t *testing.T) {
	got := gatherOurProcess(t, "svc")
	if got["svc_process_open_fds"] == nil {
		t.Errorf("want svc_process_open_fds, got families %v", keys(got))
	}
}

// Opening a descriptor must move process_open_fds by exactly one — the fast
// path's st_size and the fallback's count both have to mean "descriptors".
func TestProcessCollector_OpenFDsTracksDescriptors(t *testing.T) {
	n0, err := countOpenFDs()
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open("/proc/self/stat")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	n1, err := countOpenFDs()
	if err != nil {
		t.Fatal(err)
	}
	if n1 != n0+1 {
		t.Errorf("open_fds %g -> %g after one open, want +1", n0, n1)
	}
}

// The fallback runs only on kernels older than 6.2, so it is never reached
// here by accident. It must agree with the fast path.
func TestCountOpenFDs_FallbackAgreesWithFastPath(t *testing.T) {
	fi, err := os.Stat("/proc/self/fd")
	if err != nil || fi.Size() == 0 {
		t.Skip("kernel has no st_size fast path to compare against")
	}
	fast := float64(fi.Size())
	slow, err := countOpenFDsByReadDir()
	if err != nil {
		t.Fatal(err)
	}
	if slow != fast {
		t.Errorf("readdir path %g, st_size path %g", slow, fast)
	}
}

// Distinct values per input, so each unit conversion and field choice is
// visible. Live readings cannot do this; see deriveProcess.
func TestDeriveProcess_Units(t *testing.T) {
	st := procStat{utime: 250, stime: 50, vsize: 123456789, rss: 3}
	s := deriveProcess(st, 4096,
		syscall.Rlimit{Cur: 1024, Max: 524288},
		syscall.Rlimit{Cur: 1 << 33, Max: math.MaxUint64})
	cases := []struct {
		name      string
		got, want float64
	}{
		{"cpu_seconds (utime+stime)/100", s.cpuSeconds, 3},
		{"virtual_memory_bytes", s.virtualMem, 123456789},
		{"resident_memory_bytes rss*pagesize", s.residentMem, 3 * 4096},
		{"max_fds soft limit", s.maxFDs, 1024},
		{"virtual_memory_max_bytes soft limit", s.virtualMax, 1 << 33},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %g, want %g", c.name, c.got, c.want)
		}
	}
	if got := deriveProcess(st, 4096, syscall.Rlimit{}, syscall.Rlimit{Cur: math.MaxUint64}).virtualMax; got != 1.8446744073709552e+19 {
		t.Errorf("unlimited address space = %g, want 1.8446744073709552e+19 as client_golang publishes", got)
	}
}

func TestStartTime_BootTimePlusTicks(t *testing.T) {
	btime, err := parseBtime([]byte("cpu  1 2 3\nintr 5\nbtime 1790000000\nprocesses 9\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := startTime(btime, 12345); got != 1790000123.45 {
		t.Errorf("start time %f, want 1790000123.45", got)
	}
	if _, err := parseBtime([]byte("cpu 1\n")); err == nil {
		t.Error("missing btime: want error")
	}
}

func keys(m map[string]*family) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
