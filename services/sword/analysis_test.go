// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package sword

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/deploy/strategy"
	"unheaded/pkg/sloguard"
)

// scrape renders the house HTTP metrics for one instance.
func scrape(ok, errs int, buckets map[string]int) string {
	var b strings.Builder
	b.WriteString("# HELP unheaded_http_requests_total Total HTTP requests\n# TYPE unheaded_http_requests_total counter\n")
	fmt.Fprintf(&b, "unheaded_http_requests_total{method=\"GET\",path=\"/\",service=\"steel-echo\",status=\"200\"} %d\n", ok)
	fmt.Fprintf(&b, "unheaded_http_requests_total{method=\"GET\",path=\"/\",service=\"steel-echo\",status=\"500\"} %d\n", errs)
	// Probes and scrapes never count.
	b.WriteString("unheaded_http_requests_total{method=\"GET\",path=\"/health\",service=\"steel-echo\",status=\"500\"} 9999\n")
	b.WriteString("unheaded_http_requests_total{method=\"GET\",path=\"/metrics\",service=\"steel-echo\",status=\"200\"} 777\n")
	for _, le := range []string{"0.005", "0.01", "0.025", "0.05", "0.1", "0.25", "0.5", "1", "+Inf"} {
		fmt.Fprintf(&b, "unheaded_http_request_duration_seconds_bucket{le=%q,method=\"GET\",path=\"/\",service=\"steel-echo\"} %d\n", le, buckets[le])
	}
	b.WriteString("unheaded_http_request_duration_seconds_bucket{le=\"+Inf\",method=\"GET\",path=\"/health\",service=\"steel-echo\"} 5000\n")
	return b.String()
}

// instanceServer serves successive scrapes, repeating the last.
type instanceServer struct {
	mu     sync.Mutex
	bodies []string
	n      int
	srv    *httptest.Server
}

func newInstance(t *testing.T, bodies ...string) *instanceServer {
	t.Helper()
	is := &instanceServer{bodies: bodies}
	is.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			http.NotFound(w, r)
			return
		}
		is.mu.Lock()
		b := is.bodies[min(is.n, len(is.bodies)-1)]
		is.n++
		is.mu.Unlock()
		_, _ = w.Write([]byte(b))
	}))
	t.Cleanup(is.srv.Close)
	return is
}

type stubGuard struct {
	v   sloguard.Verdict
	err error
	n   int
	mu  sync.Mutex
}

func (g *stubGuard) Check(context.Context) (sloguard.Verdict, error) {
	g.mu.Lock()
	g.n++
	g.mu.Unlock()
	return g.v, g.err
}

func okGuard() *stubGuard { return &stubGuard{v: sloguard.Verdict{OK: true}} }

func analyzerFor(g SLOChecker, servers map[string]*instanceServer) *Analyzer {
	a := NewAnalyzer(g)
	a.metricsURL = func(inst strategy.Instance, _ int) (string, error) {
		s, ok := servers[inst.ID]
		if !ok {
			return "", errors.New("no server")
		}
		return s.srv.URL + "/metrics", nil
	}
	return a
}

func request(min int) *strategy.AnalysisRequest {
	return &strategy.AnalysisRequest{
		ServiceName: "steel-echo", StableVersion: "v3", CanaryVersion: "v4", Step: 1, Weight: 33,
		Canary: []strategy.Instance{{ID: "c0", Address: "10.10.10.40"}},
		Stable: []strategy.Instance{{ID: "s0", Address: "10.10.10.41"}},
		Config: &strategy.AnalysisConfig{MinSampleSize: min, Interval: time.Millisecond},
	}
}

func TestAnalysis_ErrorRatioFromCounterDeltas(t *testing.T) {
	// Lifetime totals are large; only the interval's delta counts:
	// +210 ok, +90 errors = 300 requests, error ratio 0.30.
	canary := newInstance(t,
		scrape(1000, 5, map[string]int{"+Inf": 1005, "0.05": 1005}),
		scrape(1210, 95, map[string]int{"+Inf": 1305, "0.05": 1305}))
	stable := newInstance(t,
		scrape(5000, 0, map[string]int{"+Inf": 5000, "0.05": 5000}),
		scrape(5600, 0, map[string]int{"+Inf": 5600, "0.05": 5600}))
	a := analyzerFor(okGuard(), map[string]*instanceServer{"c0": canary, "s0": stable})
	res, err := a.Analyze(context.Background(), request(200), 8080)
	if err != nil {
		t.Fatal(err)
	}
	m := res.Metrics
	if m == nil || m.RequestCount != 300 || m.ErrorCount != 90 || m.ErrorRate < 0.2999 || m.ErrorRate > 0.3001 {
		t.Fatalf("metrics %+v, want 300 requests, 90 errors, 0.30", m)
	}
	if m.SuccessRate < 0.6999 || m.SuccessRate > 0.7001 {
		t.Fatalf("success rate %v", m.SuccessRate)
	}
	if !strings.Contains(res.Message, "stable") {
		t.Fatalf("message does not compare with stable: %q", res.Message)
	}
	// The strategy applies the threshold to these numbers; at 1% this fails.
	cfg := &strategy.AnalysisConfig{ErrorRateThreshold: 0.01}
	if m.ErrorRate <= cfg.ErrorRateThreshold {
		t.Fatal("a 30% canary is under a 1% threshold?")
	}
}

func TestAnalysis_P99FromHistogramDeltas(t *testing.T) {
	// Before: everything fast. During the interval: 100 new requests, 98
	// under 50 ms, 2 between 250 ms and 500 ms -> p99 bucket is 0.5 s.
	before := map[string]int{"0.005": 0, "0.01": 0, "0.025": 0, "0.05": 10000, "0.1": 10000, "0.25": 10000, "0.5": 10000, "1": 10000, "+Inf": 10000}
	after := map[string]int{"0.005": 0, "0.01": 0, "0.025": 0, "0.05": 10098, "0.1": 10098, "0.25": 10098, "0.5": 10100, "1": 10100, "+Inf": 10100}
	canary := newInstance(t, scrape(10000, 0, before), scrape(10100, 0, after))
	a := analyzerFor(okGuard(), map[string]*instanceServer{"c0": canary})
	req := request(50)
	req.Stable = nil
	res, err := a.Analyze(context.Background(), req, 8080)
	if err != nil {
		t.Fatal(err)
	}
	if res.Metrics.LatencyP99 != 500*time.Millisecond {
		t.Fatalf("p99 = %v, want 500ms (the lifetime histogram would say 50ms)", res.Metrics.LatencyP99)
	}
}

func TestAnalysis_TooFewSamplesIsInconclusive(t *testing.T) {
	canary := newInstance(t,
		scrape(100, 0, map[string]int{"+Inf": 100}),
		scrape(150, 0, map[string]int{"+Inf": 150}), // +50 < 200
		scrape(150, 0, map[string]int{"+Inf": 150}),
		scrape(400, 0, map[string]int{"+Inf": 400, "0.05": 400})) // +250 >= 200
	a := analyzerFor(okGuard(), map[string]*instanceServer{"c0": canary})
	req := request(200)
	req.Stable = nil
	res, err := a.Analyze(context.Background(), req, 8080)
	if err != nil {
		t.Fatal(err)
	}
	// One inconclusive interval holds; the next conclusive one decides.
	if !res.Pass || res.Metrics.RequestCount != 250 {
		t.Fatalf("result %+v metrics %+v, want a pass on the second interval's 250 requests", res, res.Metrics)
	}
}

func TestAnalysis_ThreeInconclusiveRollsBack(t *testing.T) {
	canary := newInstance(t, scrape(100, 0, map[string]int{"+Inf": 100})) // never moves
	a := analyzerFor(okGuard(), map[string]*instanceServer{"c0": canary})
	req := request(200)
	req.Stable = nil
	res, err := a.Analyze(context.Background(), req, 8080)
	if err != nil {
		t.Fatal(err)
	}
	if res.Pass || !res.ShouldRollback || !strings.Contains(res.Message, "inconclusive") {
		t.Fatalf("result %+v, want a rollback after three inconclusive intervals", res)
	}
	canary.mu.Lock()
	n := canary.n
	canary.mu.Unlock()
	if n != 6 {
		t.Fatalf("scraped %d times, want 3 intervals x 2", n)
	}
}

func TestAnalysis_UnreadableCanaryIsInconclusive(t *testing.T) {
	a := analyzerFor(okGuard(), map[string]*instanceServer{})
	req := request(1)
	req.Stable = nil
	res, err := a.Analyze(context.Background(), req, 8080)
	if err != nil {
		t.Fatal(err)
	}
	if res.Pass || !res.ShouldRollback {
		t.Fatalf("a canary whose /metrics cannot be read passed: %+v", res)
	}
}

func TestAnalysis_KingdomFastBurnFails(t *testing.T) {
	canary := newInstance(t, scrape(0, 0, nil), scrape(1000, 0, map[string]int{"+Inf": 1000, "0.05": 1000}))
	g := &stubGuard{v: sloguard.Verdict{OK: false, Burning: []string{"wotan"}, Reason: "wotan burning"}}
	a := analyzerFor(g, map[string]*instanceServer{"c0": canary})
	res, err := a.Analyze(context.Background(), request(10), 8080)
	if err != nil {
		t.Fatal(err)
	}
	if res.Pass || !res.ShouldRollback || !strings.Contains(res.Message, "wotan") {
		t.Fatalf("a burning Kingdom SLO passed: %+v", res)
	}
}

func TestAnalysis_MissingSLOSeriesFails(t *testing.T) {
	canary := newInstance(t, scrape(0, 0, nil), scrape(1000, 0, map[string]int{"+Inf": 1000}))
	for name, g := range map[string]*stubGuard{
		"missing series": {v: sloguard.Verdict{OK: false, Missing: []string{"dashboard"}, Reason: "dashboard has no slo:error_ratio:5m series"}},
		"victoria down":  {err: errors.New("connection refused"), v: sloguard.Verdict{Reason: "cannot read Victoria"}},
	} {
		a := analyzerFor(g, map[string]*instanceServer{"c0": canary})
		res, err := a.Analyze(context.Background(), request(10), 8080)
		if err != nil {
			t.Fatal(err)
		}
		if res.Pass || !res.ShouldRollback {
			t.Fatalf("%s: passed %+v", name, res)
		}
	}
	// No guard at all fails closed too.
	a := analyzerFor(nil, map[string]*instanceServer{"c0": canary})
	if res, _ := a.Analyze(context.Background(), request(10), 8080); res == nil || res.Pass {
		t.Fatalf("no guard: %+v", res)
	}
}

// The default target is the instance's own address from Cuirass, checked
// like Cuirass's probe; anything else is refused before any request.
func TestAnalysis_DefaultMetricsURLRefusesOddAddresses(t *testing.T) {
	a := NewAnalyzer(okGuard())
	if u, err := a.metricsURL(strategy.Instance{ID: "c", Address: "10.10.10.40"}, 8080); err != nil || u != "http://10.10.10.40:8080/metrics" {
		t.Fatalf("url %q err %v", u, err)
	}
	for _, addr := range []string{"127.0.0.1", "::1", "0.0.0.0", "169.254.169.254", "224.0.0.1", "", "example.com", "10.0.0.1:80"} {
		if _, err := a.metricsURL(strategy.Instance{ID: "c", Address: addr}, 8080); err == nil {
			t.Errorf("address %q accepted", addr)
		}
	}
	if _, err := a.metricsURL(strategy.Instance{ID: "c", Address: "10.10.10.40"}, 0); err == nil {
		t.Error("port 0 accepted")
	}
}

func TestParseHTTPMetrics_Hostile(t *testing.T) {
	for name, body := range map[string]string{
		"garbage value":   `unheaded_http_requests_total{path="/",status="200"} lots` + "\n",
		"unclosed labels": `unheaded_http_requests_total{path="/",status="200" 5` + "\n",
		"negative":        `unheaded_http_requests_total{path="/",status="200"} -5` + "\n",
	} {
		if _, err := parseHTTPMetrics(strings.NewReader(body)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// Other families and comments are ignored; escaped quotes parse.
	s, err := parseHTTPMetrics(strings.NewReader("# c\nother_total 5\n" +
		`unheaded_http_requests_total{path="/a\"b",status="503"} 7` + "\n"))
	if err != nil || s.requests != 7 || s.errors != 7 {
		t.Fatalf("sample %+v err %v", s, err)
	}
}

func FuzzParseHTTPMetrics(f *testing.F) {
	f.Add(scrape(10, 2, map[string]int{"+Inf": 12}))
	f.Add(`unheaded_http_requests_total{path="/",status="200"} 1e308` + "\n")
	f.Fuzz(func(t *testing.T, body string) {
		s, err := parseHTTPMetrics(strings.NewReader(body))
		if err != nil {
			return
		}
		if s.requests < 0 || s.errors < 0 || s.errors > s.requests {
			t.Fatalf("impossible sample %+v", s)
		}
	})
}
