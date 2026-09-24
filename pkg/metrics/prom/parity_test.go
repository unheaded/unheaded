// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package prom_test

import (
	"bufio"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

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
		prometheus.ExponentialBuckets(64, 4, 5), []string{"kind"})
	for _, v := range []float64{10, 64, 65, 300, 100000} {
		hv("kernel", v)
	}
	hv("user", 1024)

	a.unusedVec("never_touched_total", "Declared, never incremented.")
}

func theirs() (api, *prometheus.Registry) {
	reg := prometheus.NewRegistry()
	f := promauto.With(reg)
	return api{
		counter: func(ns, sub, name, help string, cl map[string]string) func(float64) {
			c := f.NewCounter(prometheus.CounterOpts{Namespace: ns, Subsystem: sub, Name: name, Help: help, ConstLabels: cl})
			return c.Add
		},
		counterVec: func(ns, name, help string, labels []string) (func(map[string]string) func(), func(...string) func(float64)) {
			v := f.NewCounterVec(prometheus.CounterOpts{Namespace: ns, Name: name, Help: help}, labels)
			return func(l map[string]string) func() { return v.With(l).Inc },
				func(vals ...string) func(float64) { return v.WithLabelValues(vals...).Add }
		},
		gauge: func(name, help string) func(float64) {
			return f.NewGauge(prometheus.GaugeOpts{Name: name, Help: help}).Set
		},
		gaugeVec: func(name, help string, labels []string) func(string, float64) {
			v := f.NewGaugeVec(prometheus.GaugeOpts{Name: name, Help: help}, labels)
			return func(l string, x float64) { v.WithLabelValues(l).Set(x) }
		},
		gaugeFunc: func(name, help string, read func() float64) {
			reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: name, Help: help}, read))
		},
		histogram: func(name, help string, b []float64) func(float64) {
			return f.NewHistogram(prometheus.HistogramOpts{Name: name, Help: help, Buckets: b}).Observe
		},
		histogramVec: func(ns, sub, name, help string, b []float64, labels []string) func(string, float64) {
			v := f.NewHistogramVec(prometheus.HistogramOpts{Namespace: ns, Subsystem: sub, Name: name, Help: help, Buckets: b}, labels)
			return func(l string, x float64) { v.WithLabelValues(l).Observe(x) }
		},
		unusedVec: func(name, help string) {
			f.NewCounterVec(prometheus.CounterOpts{Name: name, Help: help}, []string{"x"})
		},
	}, reg
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

// Every declaration and operation the survey found in use, through both
// libraries, served through each one's handler: a scraper must see the
// same families, metadata and values, with nothing extra on either side.
func TestShim_MatchesClientGolangExactly(t *testing.T) {
	tapi, treg := theirs()
	exercise(tapi)
	oapi, oreg := ours()
	exercise(oapi)

	te := parse(t, serve(promhttp.HandlerFor(treg, promhttp.HandlerOpts{})))
	oe := parse(t, serve(oreg.Handler()))

	if len(te.samples) < 30 {
		t.Fatalf("client_golang produced only %d samples; the scenario is not exercising much", len(te.samples))
	}
	for name, typ := range te.typ {
		if oe.typ[name] != typ {
			t.Errorf("%s: type %q, client_golang %q", name, oe.typ[name], typ)
		}
		if oe.help[name] != te.help[name] {
			t.Errorf("%s: help %q, client_golang %q", name, oe.help[name], te.help[name])
		}
	}
	for name := range oe.typ {
		if _, ok := te.typ[name]; !ok {
			t.Errorf("extra family %s (client_golang omits it)", name)
		}
	}
	keys := make([]string, 0, len(te.samples))
	for k := range te.samples {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		ov, ok := oe.samples[k]
		if !ok {
			t.Errorf("missing series %s", k)
			continue
		}
		if ov != te.samples[k] {
			t.Errorf("%s: %g, client_golang %g", k, ov, te.samples[k])
		}
	}
	for k := range oe.samples {
		if _, ok := te.samples[k]; !ok {
			t.Errorf("extra series %s", k)
		}
	}
}

func TestShim_DefBucketsAndFQName(t *testing.T) {
	if fmt.Sprint(prom.DefBuckets) != fmt.Sprint(prometheus.DefBuckets) {
		t.Errorf("DefBuckets %v, client_golang %v", prom.DefBuckets, prometheus.DefBuckets)
	}
	for _, c := range [][3]string{
		{"", "", ""}, {"ns", "sub", ""}, {"", "", "n"}, {"ns", "", "n"},
		{"", "sub", "n"}, {"ns", "sub", "n"},
	} {
		if got, want := prom.BuildFQName(c[0], c[1], c[2]), prometheus.BuildFQName(c[0], c[1], c[2]); got != want {
			t.Errorf("BuildFQName%q = %q, client_golang %q", c, got, want)
		}
	}
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

// Bounds must be bit-identical to client_golang's, or an le series changes
// name under every dashboard. Covers every argument set used in the tree
// (git grep, 2026-09-24) plus widths that do not round-trip in binary.
func TestShim_BucketGeneratorsBitIdentical(t *testing.T) {
	for _, a := range [][3]float64{
		{0.0001, 2, 12}, {0.001, 2, 12}, {0.001, 2, 14}, {100, 10, 6}, {100, 10, 8},
		{1024, 4, 10}, {64, 4, 5}, {0.1, 3, 20}, {0.005, 1.5, 30},
	} {
		want := prometheus.ExponentialBuckets(a[0], a[1], int(a[2]))
		got := prom.ExponentialBuckets(a[0], a[1], int(a[2]))
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("ExponentialBuckets%v[%d] = %v, client_golang %v", a, i, got[i], want[i])
			}
		}
	}
	for _, a := range [][3]float64{{0, 0.1, 30}, {0.05, 0.05, 40}, {1, 2.5, 10}, {-1, 0.3, 25}} {
		want := prometheus.LinearBuckets(a[0], a[1], int(a[2]))
		got := prom.LinearBuckets(a[0], a[1], int(a[2]))
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("LinearBuckets%v[%d] = %v, client_golang %v", a, i, got[i], want[i])
			}
		}
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

// Same name, different const label values: one family, both series. Every
// other combination must be refused exactly where client_golang refuses it.
func TestShim_SameNameRegistrationMatchesClientGolang(t *testing.T) {
	treg, oreg := prometheus.NewRegistry(), prom.NewRegistry()
	for _, svc := range []string{"a", "b"} {
		tc := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "http_total", Help: "h", ConstLabels: prometheus.Labels{"service": svc}}, []string{"code"})
		oc := prom.NewCounterVec(prom.CounterOpts{Name: "http_total", Help: "h", ConstLabels: prom.Labels{"service": svc}}, []string{"code"})
		treg.MustRegister(tc)
		oreg.MustRegister(oc)
		tc.WithLabelValues("200").Inc()
		oc.WithLabelValues("200").Inc()
	}
	te := parse(t, serve(promhttp.HandlerFor(treg, promhttp.HandlerOpts{})))
	oe := parse(t, serve(oreg.Handler()))
	if len(oe.samples) != 2 || len(te.samples) != 2 {
		t.Fatalf("want 2 series each; ours %v, client_golang %v", oe.samples, te.samples)
	}
	for k, v := range te.samples {
		if oe.samples[k] != v {
			t.Errorf("%s: ours %g, client_golang %g", k, oe.samples[k], v)
		}
	}

	type decl struct {
		why   string
		help  string
		gauge bool
		vars  []string
		cl    map[string]string
	}
	for _, d := range []decl{
		{why: "identical const labels", help: "h", vars: []string{"code"}, cl: map[string]string{"service": "a"}},
		{why: "different help", help: "other", vars: []string{"code"}, cl: map[string]string{"service": "c"}},
		{why: "different type", help: "h", gauge: true, vars: []string{"code"}, cl: map[string]string{"service": "t"}},
		{why: "different variable labels", help: "h", vars: []string{"method"}, cl: map[string]string{"service": "c"}},
		{why: "different const label names", help: "h", vars: []string{"code"}, cl: map[string]string{"svc": "c"}},
		{why: "accepted: new const value", help: "h", vars: []string{"code"}, cl: map[string]string{"service": "d"}},
	} {
		var terr, oerr error
		var tg *prometheus.GaugeVec
		if d.gauge {
			tg = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "http_total", Help: d.help, ConstLabels: d.cl}, d.vars)
			terr = treg.Register(tg)
			tg.WithLabelValues("200").Set(1) // a series, so Gather has something to clash
			oerr = oreg.Register(prom.NewGaugeVec(prom.GaugeOpts{Name: "http_total", Help: d.help, ConstLabels: d.cl}, d.vars))
		} else {
			terr = treg.Register(prometheus.NewCounterVec(prometheus.CounterOpts{Name: "http_total", Help: d.help, ConstLabels: d.cl}, d.vars))
			oerr = oreg.Register(prom.NewCounterVec(prom.CounterOpts{Name: "http_total", Help: d.help, ConstLabels: d.cl}, d.vars))
		}
		if d.gauge {
			// client_golang accepts a type clash at Register and refuses it
			// at Gather, failing the whole scrape. Ours refuses at Register,
			// where the caller can see which line did it. Both refuse.
			if oerr == nil {
				t.Errorf("%s: ours accepted it", d.why)
			}
			if _, gerr := treg.Gather(); gerr == nil {
				t.Errorf("%s: expected client_golang to refuse at Gather", d.why)
			}
			if terr == nil {
				treg.Unregister(tg)
			}
			continue
		}
		if (terr == nil) != (oerr == nil) {
			t.Errorf("%s: client_golang err=%v, ours err=%v", d.why, terr, oerr)
		}
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
