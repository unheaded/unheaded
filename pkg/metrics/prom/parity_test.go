// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package prom_test

import (
	"bufio"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"unheaded/pkg/metrics/auto"
	"unheaded/pkg/metrics/prom"
)

// exposition is text format reduced to what a scraper sees: family metadata
// and sample values, keyed by name plus labels in sorted order. client_golang
// writes le last and pkg/metrics sorts it in; both are valid and mean the
// same, so order is canonicalised. le values are canonicalised through
// float parsing ("1" and "1.0" are one bucket).
type exposition struct {
	help, typ map[string]string
	samples   map[string]float64
}

func parse(t *testing.T, text string) exposition {
	t.Helper()
	e := exposition{help: map[string]string{}, typ: map[string]string{}, samples: map[string]float64{}}
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
		case strings.HasPrefix(line, "# HELP "):
			p := strings.SplitN(line[7:], " ", 2)
			e.help[p[0]] = p[1]
		case strings.HasPrefix(line, "# TYPE "):
			p := strings.SplitN(line[7:], " ", 2)
			if _, dup := e.typ[p[0]]; dup {
				t.Errorf("duplicate TYPE for %s", p[0])
			}
			e.typ[p[0]] = p[1]
		default:
			sp := strings.LastIndexByte(line, ' ')
			series, raw := line[:sp], line[sp+1:]
			v, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				t.Fatalf("bad value in %q", line)
			}
			key := canonical(t, series)
			if _, dup := e.samples[key]; dup {
				t.Errorf("duplicate series %s", key)
			}
			e.samples[key] = v
		}
	}
	return e
}

func canonical(t *testing.T, series string) string {
	i := strings.IndexByte(series, '{')
	if i < 0 {
		return series
	}
	name, body := series[:i], strings.TrimSuffix(series[i+1:], "}")
	var pairs []string
	for _, kv := range strings.Split(body, ",") {
		eq := strings.IndexByte(kv, '=')
		k, v := kv[:eq], strings.Trim(kv[eq+1:], `"`)
		if k == "le" && v != "+Inf" {
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				t.Fatalf("bad le in %q", series)
			}
			v = strconv.FormatFloat(f, 'g', -1, 64)
		}
		pairs = append(pairs, k+"="+v)
	}
	sort.Strings(pairs)
	return name + "{" + strings.Join(pairs, ",") + "}"
}

// scenario declares and exercises the same metrics through either library.
// Every value is deterministic, so the comparison is exact.
type api struct {
	counter      func(ns, sub, name, help string, cl map[string]string) func(float64)
	counterVec   func(ns, name, help string, labels []string) (with func(map[string]string) func(), withValues func(...string) func(float64))
	gauge        func(name, help string) func(float64)
	gaugeVec     func(name, help string, labels []string) func(string, float64)
	gaugeFunc    func(name, help string, read func() float64)
	histogram    func(name, help string, buckets []float64) func(float64)
	histogramVec func(ns, sub, name, help string, buckets []float64, labels []string) func(string, float64)
	unusedVec    func(name, help string)
}

func exercise(a api) {
	inc := a.counter("unheaded", "wotan", "messages_total", "Messages published.", map[string]string{"node": "west"})
	inc(1)
	inc(41)
	a.counter("", "", "bare_total", "No namespace.", nil)(3)

	with, withValues := a.counterVec("svc", "requests_total", "Requests by status.", []string{"status", "method"})
	with(map[string]string{"method": "GET", "status": "200"})()
	with(map[string]string{"status": "200", "method": "GET"})()
	withValues("500", "POST")(2)

	a.gauge("queue_depth", "Queue depth.")(-7.5)
	set := a.gaugeVec("disk_used_bytes", "Disk used.", []string{"host"})
	set("east", 1.5e9)
	set("west", 0)
	a.gaugeFunc("uptime_seconds", "Uptime.", func() float64 { return 12345 })

	obs := a.histogram("latency_seconds", "Default buckets.", nil)
	for _, v := range []float64{0.001, 0.005, 0.07, 0.3, 2, 99} {
		obs(v)
	}
	hv := a.histogramVec("unheaded", "trace", "span_bytes", "Exponential buckets.",
		[]float64{64, 256, 1024, 4096, 16384}, []string{"kind"}) // ExponentialBuckets(64, 4, 5), written out
	for _, v := range []float64{10, 64, 65, 300, 100000} {
		hv("kernel", v)
	}
	hv("user", 1024)

	a.unusedVec("never_touched_total", "Declared, never incremented.")
}

func ours() (api, *prom.Registry) {
	reg := prom.NewRegistry()
	f := auto.With(reg)
	return api{
		counter: func(ns, sub, name, help string, cl map[string]string) func(float64) {
			return f.NewCounter(prom.CounterOpts{Namespace: ns, Subsystem: sub, Name: name, Help: help, ConstLabels: cl}).Add
		},
		counterVec: func(ns, name, help string, labels []string) (func(map[string]string) func(), func(...string) func(float64)) {
			v := f.NewCounterVec(prom.CounterOpts{Namespace: ns, Name: name, Help: help}, labels)
			return func(l map[string]string) func() { return v.With(l).Inc },
				func(vals ...string) func(float64) { return v.WithLabelValues(vals...).Add }
		},
		gauge: func(name, help string) func(float64) {
			return f.NewGauge(prom.GaugeOpts{Name: name, Help: help}).Set
		},
		gaugeVec: func(name, help string, labels []string) func(string, float64) {
			v := f.NewGaugeVec(prom.GaugeOpts{Name: name, Help: help}, labels)
			return func(l string, x float64) { v.WithLabelValues(l).Set(x) }
		},
		gaugeFunc: func(name, help string, read func() float64) {
			reg.MustRegister(prom.NewGaugeFunc(prom.GaugeOpts{Name: name, Help: help}, read))
		},
		histogram: func(name, help string, b []float64) func(float64) {
			return f.NewHistogram(prom.HistogramOpts{Name: name, Help: help, Buckets: b}).Observe
		},
		histogramVec: func(ns, sub, name, help string, b []float64, labels []string) func(string, float64) {
			v := f.NewHistogramVec(prom.HistogramOpts{Namespace: ns, Subsystem: sub, Name: name, Help: help, Buckets: b}, labels)
			return func(l string, x float64) { v.WithLabelValues(l).Observe(x) }
		},
		unusedVec: func(name, help string) {
			f.NewCounterVec(prom.CounterOpts{Name: name, Help: help}, []string{"x"})
		},
	}, reg
}

func serve(h http.Handler) string {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	return rec.Body.String()
}

// The default registry must carry go_* and process_*, or a service moving
// off promhttp.Handler() silently loses go_goroutines.
func TestShim_DefaultRegistryCarriesRuntimeSeries(t *testing.T) {
	out := serve(prom.Handler())
	for _, want := range []string{"# TYPE go_goroutines gauge", "# TYPE process_resident_memory_bytes gauge"} {
		if !strings.Contains(out, want) {
			t.Errorf("default registry lacks %q", want)
		}
	}
	if out := serve(prom.NewRegistry().Handler()); out != "" {
		t.Errorf("a new registry must start empty, as client_golang's does; got:\n%s", out)
	}
}

// auto.With(nil) registers nowhere, as promauto.With(nil) does.
func TestAuto_WithNilRegistersNowhere(t *testing.T) {
	c := auto.With(nil).NewCounter(prom.CounterOpts{Name: "orphan_total", Help: "x"})
	c.Inc()
	if strings.Contains(serve(prom.Handler()), "orphan_total") {
		t.Error("auto.With(nil) registered with the default registry")
	}
}

// Unregister removes one family member, identified by its const labels.
func TestShim_UnregisterRemovesOneMember(t *testing.T) {
	reg := prom.NewRegistry()
	a := prom.NewCounter(prom.CounterOpts{Name: "x_total", Help: "h", ConstLabels: prom.Labels{"s": "a"}})
	b := prom.NewCounter(prom.CounterOpts{Name: "x_total", Help: "h", ConstLabels: prom.Labels{"s": "b"}})
	reg.MustRegister(a, b)
	a.Inc()
	b.Inc()
	if !reg.Unregister(a) {
		t.Fatal("unregister a failed")
	}
	out := serve(reg.Handler())
	if strings.Contains(out, `s="a"`) || !strings.Contains(out, `s="b"`) {
		t.Errorf("want only s=b left:\n%s", out)
	}
}

// readGolden returns the data lines of a testdata file recorded from
// client_golang v1.18.0 before the dependency was removed (ADR-094 step 6).
func readGolden(t *testing.T, name string) [][]string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var rows [][]string
	for _, l := range strings.Split(string(b), "\n") {
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		rows = append(rows, strings.Split(l, "\t"))
	}
	return rows
}

func bits(t *testing.T, s string) float64 {
	t.Helper()
	u, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		t.Fatalf("bad float bits %q", s)
	}
	return math.Float64frombits(u)
}

// Every declaration and operation the survey found in use, served by the
// shim, must match what client_golang served for the same scenario:
// families, help, and every series value bit for bit, with nothing extra.
func TestShim_MatchesClientGolangExactly(t *testing.T) {
	want := exposition{help: map[string]string{}, typ: map[string]string{}, samples: map[string]float64{}}
	for _, r := range readGolden(t, "client_golang_v1.18.0_scenario.golden") {
		switch r[0] {
		case "TYPE":
			want.typ[r[1]] = r[2]
		case "HELP":
			want.help[r[1]] = r[2]
		case "SAMPLE":
			want.samples[r[1]] = bits(t, r[2])
		}
	}
	if len(want.samples) < 30 {
		t.Fatalf("golden has only %d samples", len(want.samples))
	}

	oapi, oreg := ours()
	exercise(oapi)
	got := parse(t, serve(oreg.Handler()))

	for name, typ := range want.typ {
		if got.typ[name] != typ {
			t.Errorf("%s: type %q, client_golang %q", name, got.typ[name], typ)
		}
		if got.help[name] != want.help[name] {
			t.Errorf("%s: help %q, client_golang %q", name, got.help[name], want.help[name])
		}
	}
	for name := range got.typ {
		if _, ok := want.typ[name]; !ok {
			t.Errorf("extra family %s (client_golang omits it)", name)
		}
	}
	for k, v := range want.samples {
		g, ok := got.samples[k]
		switch {
		case !ok:
			t.Errorf("missing series %s", k)
		case math.Float64bits(g) != math.Float64bits(v):
			t.Errorf("%s: %v, client_golang %v", k, g, v)
		}
	}
	for k := range got.samples {
		if _, ok := want.samples[k]; !ok {
			t.Errorf("extra series %s", k)
		}
	}
}

// DefBuckets, BuildFQName and the bucket generators must reproduce
// client_golang's output bit for bit: a bound that differs in the last bit
// is a different le series to every dashboard that queries it.
func TestShim_BucketsAndNamesMatchClientGolang(t *testing.T) {
	n := 0
	for _, r := range readGolden(t, "client_golang_v1.18.0_buckets.golden") {
		switch r[0] {
		case "exp", "lin":
			start, _ := strconv.ParseFloat(r[1], 64)
			step, _ := strconv.ParseFloat(r[2], 64)
			count, _ := strconv.Atoi(r[3])
			i, _ := strconv.Atoi(r[4])
			var got []float64
			if r[0] == "exp" {
				got = prom.ExponentialBuckets(start, step, count)
			} else {
				got = prom.LinearBuckets(start, step, count)
			}
			if w := bits(t, r[5]); math.Float64bits(got[i]) != math.Float64bits(w) {
				t.Errorf("%s(%v, %v, %d)[%d] = %v, client_golang %v", r[0], start, step, count, i, got[i], w)
			}
		case "def":
			i, _ := strconv.Atoi(r[1])
			if w := bits(t, r[2]); math.Float64bits(prom.DefBuckets[i]) != math.Float64bits(w) {
				t.Errorf("DefBuckets[%d] = %v, client_golang %v", i, prom.DefBuckets[i], w)
			}
		case "fq":
			want := ""
			if len(r) > 4 {
				want = r[4]
			}
			if got := prom.BuildFQName(r[1], r[2], r[3]); got != want {
				t.Errorf("BuildFQName(%q, %q, %q) = %q, client_golang %q", r[1], r[2], r[3], got, want)
			}
		}
		n++
	}
	if n < 230 {
		t.Fatalf("golden has only %d rows", n)
	}
	for _, bad := range []func(){
		func() { prom.ExponentialBuckets(1, 2, 0) },
		func() { prom.ExponentialBuckets(0, 2, 3) },
		func() { prom.ExponentialBuckets(1, 1, 3) },
		func() { prom.LinearBuckets(0, 1, 0) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("invalid bucket arguments did not panic")
				}
			}()
			bad()
		}()
	}
}

// Same name, different const label values: one family, both series. The
// accept/reject outcomes are client_golang v1.18.0's, observed side by side
// before the dependency was removed, with one deliberate difference: a type
// clash is refused here at Register; client_golang accepted it and failed
// the whole scrape at Gather.
func TestShim_SameNameRegistrationMatchesClientGolang(t *testing.T) {
	reg := prom.NewRegistry()
	for _, svc := range []string{"a", "b"} {
		c := prom.NewCounterVec(prom.CounterOpts{Name: "http_total", Help: "h", ConstLabels: prom.Labels{"service": svc}}, []string{"code"})
		reg.MustRegister(c)
		c.WithLabelValues("200").Inc()
	}
	got := parse(t, serve(reg.Handler()))
	for _, k := range []string{"http_total{code=200,service=a}", "http_total{code=200,service=b}"} {
		if got.samples[k] != 1 {
			t.Errorf("want %s = 1 in one family; got %v", k, got.samples)
		}
	}

	for _, c := range []struct {
		why    string
		accept bool
		reg    func() error
	}{
		{"identical const labels", false, func() error {
			return reg.Register(prom.NewCounterVec(prom.CounterOpts{Name: "http_total", Help: "h", ConstLabels: prom.Labels{"service": "a"}}, []string{"code"}))
		}},
		{"different help", false, func() error {
			return reg.Register(prom.NewCounterVec(prom.CounterOpts{Name: "http_total", Help: "other", ConstLabels: prom.Labels{"service": "c"}}, []string{"code"}))
		}},
		{"different type (client_golang: refused at Gather)", false, func() error {
			return reg.Register(prom.NewGaugeVec(prom.GaugeOpts{Name: "http_total", Help: "h", ConstLabels: prom.Labels{"service": "t"}}, []string{"code"}))
		}},
		{"different variable labels", false, func() error {
			return reg.Register(prom.NewCounterVec(prom.CounterOpts{Name: "http_total", Help: "h", ConstLabels: prom.Labels{"service": "c"}}, []string{"method"}))
		}},
		{"different const label names", false, func() error {
			return reg.Register(prom.NewCounterVec(prom.CounterOpts{Name: "http_total", Help: "h", ConstLabels: prom.Labels{"svc": "c"}}, []string{"code"}))
		}},
		{"new const value", true, func() error {
			return reg.Register(prom.NewCounterVec(prom.CounterOpts{Name: "http_total", Help: "h", ConstLabels: prom.Labels{"service": "d"}}, []string{"code"}))
		}},
	} {
		if err := c.reg(); (err == nil) != c.accept {
			t.Errorf("%s: accepted=%v, client_golang accepted=%v (err %v)", c.why, err == nil, c.accept, err)
		}
	}
}
