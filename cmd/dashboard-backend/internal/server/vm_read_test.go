// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeVM answers /api/v1/query from a table of PromQL -> result JSON; a query
// not in the table gets an empty vector. down makes every answer a 500.
type fakeVM struct {
	answers map[string]string
	down    atomic.Bool
	queries atomic.Int64
}

func (f *fakeVM) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.queries.Add(1)
	if f.down.Load() {
		http.Error(w, "down", http.StatusInternalServerError)
		return
	}
	res := f.answers[r.URL.Query().Get("query")]
	if res == "" {
		res = "[]"
	}
	_, _ = fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":%s}}`, res)
}

func one(v string) string { return `[{"metric":{},"value":[1700000000,"` + v + `"]}]` }

func bySvc(label string, kv ...string) string {
	var parts []string
	for i := 0; i+1 < len(kv); i += 2 {
		parts = append(parts, fmt.Sprintf(`{"metric":{%q:%q},"value":[1700000000,%q]}`, label, kv[i], kv[i+1]))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func newVMServer(t *testing.T, vm *fakeVM) *Server {
	t.Helper()
	hs := httptest.NewServer(vm)
	t.Cleanup(hs.Close)
	cfg := DefaultConfig()
	cfg.WotanAddr = "localhost:19999"
	cfg.VMUrl = hs.URL
	srv, err := NewServer(cfg, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	return srv
}

func getJSON(t *testing.T, h http.HandlerFunc, path string) (int, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest(http.MethodGet, path, nil))
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func fullVM() *fakeVM {
	return &fakeVM{answers: map[string]string{
		qRequestRate:                     one("12.5"),
		qErrorPercent:                    one("0.4"),
		quantile(qLatencyQuantile, 0.5):  one("0.002"),
		quantile(qLatencyQuantile, 0.95): one("0.010"),
		quantile(qLatencyQuantile, 0.99): one("0.025"),
		qServicesReporting:               one("3"),
		qTargetsScraped:                  one("7"),
		qUptime:                          one("3600"),
		qSvcRequestRate:                  bySvc("service", "wotan", "10", "timeguru", "2.5"),
		qSvcErrorRate:                    bySvc("service", "wotan", "0.05"),
		quantile(qSvcQuantile, 0.5):      bySvc("service", "wotan", "0.001"),
		quantile(qSvcQuantile, 0.9):      bySvc("service", "wotan", "0.004"),
		quantile(qSvcQuantile, 0.99):     bySvc("service", "wotan", "0.02"),
		qSvcMeanSeconds:                  bySvc("service", "wotan", "0.002"),
		qSvcSamples:                      bySvc("service", "wotan", "3000"),
		qJobMemory:                       bySvc("job", "wotan", "52428800"),
		qJobCPU:                          bySvc("job", "wotan", "0.02"),
	}}
}

// ADR-147 decision 9: every graph reads VictoriaMetrics. Each field the page
// reads (dash-fields.txt) comes from the fake's answers.
func TestMetricsEndpointsReadVictoria(t *testing.T) {
	srv := newVMServer(t, fullVM())

	code, sum := getJSON(t, srv.handleMetricsSummary, "/api/v1/metrics/summary")
	if code != http.StatusOK {
		t.Fatalf("summary status %d: %v", code, sum)
	}
	lat, _ := sum["latency_ms"].(map[string]any)
	for k, want := range map[string]any{
		"request_rate": 12.5, "error_rate": 0.4, "services_reporting": 3.0,
		"services_scraped": 7.0, "window_seconds": 60.0, "uptime_seconds": 3600.0,
	} {
		if sum[k] != want {
			t.Errorf("summary %s = %v, want %v", k, sum[k], want)
		}
	}
	if lat["p50"] != 2.0 || lat["p95"] != 10.0 || lat["p99"] != 25.0 {
		t.Errorf("latency_ms = %v, want p50 2, p95 10, p99 25", lat)
	}

	srv.ebpfIngestor = nil
	code, l := getJSON(t, srv.handleLatency, "/api/v1/latency")
	if code != http.StatusOK || l["source"] != "victoriametrics" {
		t.Fatalf("latency %d %v", code, l)
	}
	pct, _ := l["percentiles"].(map[string]any)
	ws, _ := pct["http_wotan"].([]any)
	if len(pct) != 1 || len(ws) != 1 {
		t.Fatalf("percentiles = %v, want only http_wotan with one window", pct)
	}
	w0 := ws[0].(map[string]any)
	for k, want := range map[string]float64{"p50_ns": 1e6, "p90_ns": 4e6, "p99_ns": 2e7, "mean_ns": 2e6, "sample_count": 3000} {
		if w0[k] != want {
			t.Errorf("http_wotan %s = %v, want %v", k, w0[k], want)
		}
	}

	code, m := getJSON(t, srv.handleAPIMetrics, "/api/v1/metrics")
	if code != http.StatusOK {
		t.Fatalf("metrics status %d", code)
	}
	svcs, _ := m["services"].(map[string]any)
	wotan, _ := svcs["wotan"].(map[string]any)
	if wotan["request_rate"] != 10.0 || wotan["error_rate"] != 0.05 || wotan["p99_ms"] != 20.0 ||
		wotan["memory_bytes"] != 52428800.0 || wotan["cpu_cores"] != 0.02 {
		t.Errorf("wotan row = %v", wotan)
	}
	if tg, _ := svcs["timeguru"].(map[string]any); tg["request_rate"] != 2.5 || tg["p99_ms"] != nil {
		t.Errorf("timeguru row = %v, want rate 2.5 and no p99 (null)", tg)
	}
}

// VictoriaMetrics down is a 503 the page can see, never a 200 with zeros.
func TestVictoriaDownIsVisible(t *testing.T) {
	vm := fullVM()
	vm.down.Store(true)
	srv := newVMServer(t, vm)
	srv.ebpfIngestor = nil
	for path, h := range map[string]http.HandlerFunc{
		"/api/v1/metrics/summary": srv.handleMetricsSummary,
		"/api/v1/metrics":         srv.handleAPIMetrics,
		"/api/v1/latency":         srv.handleLatency,
		"/api/v1/aggregated":      srv.handleAggregated,
	} {
		code, body := getJSON(t, h, path)
		if code != http.StatusServiceUnavailable || body["error"] != "victoriametrics unavailable" {
			t.Errorf("%s: %d %v, want 503 victoriametrics unavailable", path, code, body)
		}
	}
	// No URL configured is the same answer.
	cfg := DefaultConfig()
	cfg.WotanAddr = "localhost:19999"
	bare, err := NewServer(cfg, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	if code, body := getJSON(t, bare.handleMetricsSummary, "/api/v1/metrics/summary"); code != http.StatusServiceUnavailable ||
		body["error"] != "victoriametrics unavailable" {
		t.Errorf("no --vm-url: %d %v", code, body)
	}
}

// The dashboard scrapes no target and polls no /health every 2 s: its
// health monitor (15 s) is the one poller left.
func TestNoTargetScrapes(t *testing.T) {
	var metricsGets, healthGets atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/metrics":
			metricsGets.Add(1)
		case "/health":
			healthGets.Add(1)
		}
		_, _ = w.Write([]byte(`{"status":"healthy"}`))
	}))
	defer target.Close()
	addr := strings.TrimPrefix(target.URL, "http://")

	cfg := DefaultConfig()
	cfg.WotanAddr = "localhost:19999"
	cfg.ServiceEndpoints = map[string]string{"wotan": addr, "timeguru": addr}
	srv, err := NewServer(cfg, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * time.Second)
	cancel()
	sctx, scancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer scancel()
	srv.Shutdown(sctx)

	if n := metricsGets.Load(); n != 0 {
		t.Errorf("GET /metrics %d times in 3 s, want 0 (no private scrape)", n)
	}
	// The health monitor's first round reads each of the two endpoints'
	// health and ready URLs (both /health here): 4. The 2 s poller that
	// went added two more at 2 s.
	if n := healthGets.Load(); n > 4 {
		t.Errorf("GET /health %d times in 3 s, want at most 4 (one 15 s monitor round)", n)
	}
}

// /api/v1/metrics/query builds its PromQL from a checked name and quoted
// label values: nothing else reaches VictoriaMetrics.
func TestMetricsQuerySelector(t *testing.T) {
	got, err := selector("unheaded_http_requests_total", map[string]string{"service": `wo"tan`, "job": "x"})
	if err != nil || got != `unheaded_http_requests_total{job="x",service="wo\"tan"}` {
		t.Errorf("selector = %q, %v", got, err)
	}
	for _, bad := range []struct {
		name   string
		labels map[string]string
	}{
		{"up} or vector(1", nil},
		{"", nil},
		{"up", map[string]string{"a=~\".*\"": "x"}},
	} {
		if _, err := selector(bad.name, bad.labels); err == nil {
			t.Errorf("selector(%q, %v) accepted", bad.name, bad.labels)
		}
	}
	srv := newVMServer(t, &fakeVM{})
	w := httptest.NewRecorder()
	srv.handleMetricsQuery(w, httptest.NewRequest(http.MethodPost, "/api/v1/metrics/query",
		strings.NewReader(`{"name":"up) or vector(1"}`)))
	if w.Code != http.StatusBadRequest {
		t.Errorf("injected name: status %d, want 400", w.Code)
	}
}
