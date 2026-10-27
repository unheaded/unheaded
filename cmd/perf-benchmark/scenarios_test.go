// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"unheaded/pkg/deploy/cuirassdriver/cuirasstest"
)

func TestParseScenarios(t *testing.T) {
	cases := []struct {
		in      string
		want    []string
		viaAll  bool
		wantErr bool
	}{
		{in: "e2e-trace", want: []string{"e2e-trace"}},
		{in: "e2e-trace, http3 ,e2e-trace", want: []string{"e2e-trace", "http3"}},
		{in: "all", want: []string{"e2e-trace", "container-start", "http-load", "http3"}, viaAll: true},
		{in: "http-load,all", want: []string{"http-load", "e2e-trace", "container-start", "http3"}, viaAll: true},
		{in: "bogus", wantErr: true},
		{in: "e2e-trace,bogus", wantErr: true},
		{in: "", wantErr: true},
		{in: " , ", wantErr: true},
	}
	for _, c := range cases {
		got, viaAll, err := parseScenarios(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseScenarios(%q): want error, got %v", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseScenarios(%q): %v", c.in, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) || viaAll != c.viaAll {
			t.Errorf("parseScenarios(%q) = %v, all=%v; want %v, all=%v", c.in, got, viaAll, c.want, c.viaAll)
		}
	}
}

func TestPercentileNearestRank(t *testing.T) {
	var d []time.Duration
	for i := 1; i <= 100; i++ {
		d = append(d, time.Duration(i)*time.Millisecond)
	}
	r := summarize("x", d, 0)
	if r.N != 100 || r.P50 != 50*time.Millisecond || r.P95 != 95*time.Millisecond || r.P99 != 99*time.Millisecond {
		t.Fatalf("summarize = %+v", r)
	}
	if r.ErrorRatio != 0 {
		t.Fatalf("error ratio %v", r.ErrorRatio)
	}
	r = summarize("x", d[:3], 1)
	if r.Attempts != 4 || r.Errors != 1 || r.ErrorRatio != 0.25 {
		t.Fatalf("summarize with errors = %+v", r)
	}
	r = summarize("x", nil, 0)
	if r.N != 0 || r.P99 != 0 || r.ErrorRatio != 0 {
		t.Fatalf("empty summarize = %+v", r)
	}
}

func TestReportSchema(t *testing.T) {
	rep := newReport("west", "abc123", time.Date(2026, 10, 27, 1, 2, 3, 0, time.UTC),
		[]Result{{Name: "http3", Skipped: "curl lacks HTTP3"}, {Name: "http-load", N: 10, P99: time.Millisecond}})
	b, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]any{"schema": "perf/v1", "host": "west", "commit": "abc123", "at": "2026-10-27T01:02:03Z", "duration_unit": "ns"} {
		if m[k] != want {
			t.Errorf("%s = %v, want %v", k, m[k], want)
		}
	}
	res := m["results"].([]any)
	if len(res) != 2 {
		t.Fatalf("results: %v", res)
	}
	r0 := res[0].(map[string]any)
	if r0["name"] != "http3" || r0["skipped"] != "curl lacks HTTP3" {
		t.Errorf("result 0: %v", r0)
	}
	r1 := res[1].(map[string]any)
	for _, k := range []string{"name", "n", "p50", "p95", "p99", "error_ratio"} {
		if _, ok := r1[k]; !ok {
			t.Errorf("result 1 lacks %q: %v", k, r1)
		}
	}
	if r1["p99"] != float64(time.Millisecond) {
		t.Errorf("p99 = %v, want ns", r1["p99"])
	}
}

func TestContainerStartNeedsAllowLXD(t *testing.T) {
	cfg := Config{Iterations: 1, Rate: 1, Duration: time.Second}
	if err := validate([]string{"container-start"}, false, cfg); err == nil || !strings.Contains(err.Error(), "-allow-lxd") {
		t.Fatalf("explicit container-start without -allow-lxd: %v", err)
	}
	if err := validate([]string{"e2e-trace", "container-start", "http-load", "http3"}, true, cfg); err != nil {
		t.Fatalf("all without -allow-lxd should validate (container-start is skipped): %v", err)
	}
	r := runScenario(context.Background(), "container-start", cfg)
	if !strings.Contains(r.Skipped, "-allow-lxd") {
		t.Fatalf("container-start without -allow-lxd under all: %+v", r)
	}
	cfg.AllowLXD = true
	if err := validate([]string{"container-start"}, false, cfg); err == nil || !strings.Contains(err.Error(), "CUIRASS_API_KEY") {
		t.Fatalf("container-start without a key: %v", err)
	}
	cfg.CuirassKey = "k"
	if err := validate([]string{"container-start"}, false, cfg); err != nil {
		t.Fatalf("container-start with -allow-lxd and a key: %v", err)
	}
	if err := validate([]string{"http-load"}, false, Config{Iterations: 1, Rate: 0, Duration: time.Second}); err == nil {
		t.Fatal("http-load with rate 0 should not validate")
	}
}

func TestRunScenarioUnknown(t *testing.T) {
	r := runScenario(context.Background(), "nope", Config{})
	if r.FirstError == "" || r.ErrorRatio != 1 {
		t.Fatalf("unknown scenario: %+v", r)
	}
}

// --- http-load ---

func TestHTTPLoadMeasuresRateAndErrors(t *testing.T) {
	var n atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1)%10 == 0 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()

	cfg := Config{HealthURL: srv.URL, Rate: 200, Duration: time.Second, Concurrency: 16}
	r := runHTTPLoad(context.Background(), cfg)
	if r.Skipped != "" {
		t.Fatalf("skipped: %+v", r)
	}
	if r.Attempts < 150 || r.Attempts > 230 {
		t.Fatalf("attempts %d at 200/s for 1s: %+v", r.Attempts, r)
	}
	if r.ErrorRatio < 0.05 || r.ErrorRatio > 0.15 {
		t.Fatalf("error ratio %v, want ~0.1: %+v", r.ErrorRatio, r)
	}
	if r.P99 <= 0 || r.P50 > r.P99 {
		t.Fatalf("latencies: %+v", r)
	}
	if r.RateAchieved < 120 {
		t.Fatalf("achieved rate %v: %+v", r.RateAchieved, r)
	}
	if r.Target != srv.URL {
		t.Fatalf("target %q", r.Target)
	}
}

func TestHTTPLoadHonoursCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	r := runHTTPLoad(ctx, Config{HealthURL: srv.URL, Rate: 1000, Duration: time.Minute, Concurrency: 4})
	if time.Since(start) > 2*time.Second {
		t.Fatalf("canceled http-load took %v", time.Since(start))
	}
	if r.Note == "" {
		t.Fatalf("canceled run should say so: %+v", r)
	}
}

// --- http3 ---

func fakeCurl(t *testing.T, features, answer string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "curl")
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then\n  echo 'curl 8.14.1 (x86_64-pc-linux-gnu)'\n  echo 'Features: " + features + "'\n  exit 0\nfi\nprintf '%s' '" + answer + "'\n"
	if err := os.WriteFile(p, []byte(script), 0o700); err != nil { // #nosec G306 -- test fixture must be executable
		t.Fatal(err)
	}
	return p
}

func TestHTTP3SkippedWithoutCurlHTTP3(t *testing.T) {
	cfg := Config{Iterations: 3, Curl: fakeCurl(t, "alt-svc HTTP2 HTTPS-proxy", "200 0.010")}
	r := runHTTP3(context.Background(), cfg)
	if !strings.Contains(r.Skipped, "HTTP3") {
		t.Fatalf("want skipped for lack of HTTP3: %+v", r)
	}
	r = runHTTP3(context.Background(), Config{Iterations: 1, Curl: filepath.Join(t.TempDir(), "no-such-curl")})
	if r.Skipped == "" {
		t.Fatalf("missing curl should skip: %+v", r)
	}
}

func TestHTTP3Measured(t *testing.T) {
	cfg := Config{Iterations: 4, Curl: fakeCurl(t, "alt-svc HTTP2 HTTP3 HTTPS-proxy", "200 0.012"), HTTP3URL: "https://localhost:1/health"}
	r := runHTTP3(context.Background(), cfg)
	if r.Skipped != "" || r.N != 4 || r.Errors != 0 {
		t.Fatalf("http3: %+v", r)
	}
	if r.P50 != 12*time.Millisecond {
		t.Fatalf("p50 %v want 12ms", r.P50)
	}
	bad := Config{Iterations: 2, Curl: fakeCurl(t, "HTTP3", "503 0.002"), HTTP3URL: "https://localhost:1/health"}
	r = runHTTP3(context.Background(), bad)
	if r.Errors != 2 || r.ErrorRatio != 1 {
		t.Fatalf("http3 503: %+v", r)
	}
}

// --- e2e-trace ---

// fakeStack is a Wotan topic API plus the dashboard's /api/v1/ebpf/events
// ring: a publish on the ingested topic appears as an event after delay.
type fakeStack struct {
	mu       sync.Mutex
	events   []map[string]any
	seq      uint64
	approved bool
	ingested string
	delay    time.Duration
	drop     bool
	wotan    *httptest.Server
	dash     *httptest.Server
	pubs     atomic.Int64
}

func newFakeStack(t *testing.T, ingested string) *fakeStack {
	f := &fakeStack{approved: true, ingested: ingested}
	f.wotan = httptest.NewServer(http.HandlerFunc(f.serveWotan))
	f.dash = httptest.NewServer(http.HandlerFunc(f.serveDash))
	t.Cleanup(f.wotan.Close)
	t.Cleanup(f.dash.Close)
	// background noise: other packet events with trace id zero
	f.add("ebpf.packet.events", map[string]any{"trace_id": map[string]any{"high": 0, "low": 0}})
	return f
}

func (f *fakeStack) add(topic string, data any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	f.events = append(f.events, map[string]any{"seq": f.seq, "topic": topic, "type": "packet", "data": data})
}

func (f *fakeStack) serveWotan(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/subscribe"):
		var req struct {
			DisplayName string `json:"display_name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		status := "pending"
		if f.approved && req.DisplayName == "perf-benchmark" {
			status = "approved"
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"subscriber": map[string]any{"subscriber_id": "6f1c2b8e-0000-4000-8000-000000000001", "status": status}})
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/publish"):
		topic := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/topics/"), "/publish")
		var req struct {
			SubscriberID string `json:"subscriber_id"`
			Payload      string `json:"payload"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SubscriberID == "" || req.Payload == "" {
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		// Raw, as the dashboard reads it: a float64 round trip would
		// lose the low bits of a 64-bit trace id.
		data := json.RawMessage(req.Payload)
		if !json.Valid(data) {
			http.Error(w, "payload not json", http.StatusBadRequest)
			return
		}
		f.pubs.Add(1)
		if topic == f.ingested && !f.drop {
			go func() {
				time.Sleep(f.delay)
				f.add("ebpf.packet.events", map[string]any{"trace_id": map[string]any{"high": 1, "low": 2}})
				f.add(topic, data)
			}()
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"status":"published"}`)
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeStack) serveDash(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/v1/ebpf/events" {
		http.NotFound(w, r)
		return
	}
	after, _ := strconv.ParseUint(r.URL.Query().Get("after_seq"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 100
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	// Like EventRing.Since: newest first, at most limit newer than
	// after_seq (0 when absent), last_seq the ring's newest.
	var out []map[string]any
	for i := len(f.events) - 1; i >= 0 && len(out) < limit; i-- {
		if f.events[i]["seq"].(uint64) <= after {
			break
		}
		out = append(out, f.events[i])
	}
	last := f.seq
	_ = json.NewEncoder(w).Encode(map[string]any{"events": out, "count": len(out), "last_seq": last})
}

func e2eConfig(f *fakeStack) Config {
	return Config{
		Iterations:   5,
		WotanURL:     f.wotan.URL,
		DashboardURL: f.dash.URL,
		E2ETopic:     "bench.perf.packet",
		WotanName:    "perf-benchmark",
		Poll:         time.Millisecond,
		E2ETimeout:   time.Second,
	}
}

func TestE2ETraceMeasuresPublishToDashboard(t *testing.T) {
	f := newFakeStack(t, "bench.perf.packet")
	f.delay = 5 * time.Millisecond
	r := runE2ETrace(context.Background(), e2eConfig(f))
	if r.N != 5 || r.Errors != 0 || r.ErrorRatio != 0 {
		t.Fatalf("e2e-trace: %+v", r)
	}
	if r.P50 < 5*time.Millisecond || r.P99 > time.Second {
		t.Fatalf("latencies: %+v", r)
	}
	if got := f.pubs.Load(); got != 5 {
		t.Fatalf("published %d, want 5", got)
	}
	if !strings.Contains(r.Target, "bench.perf.packet") {
		t.Fatalf("target %q", r.Target)
	}
}

func TestE2ETraceTimesOutWhenDashboardNeverSeesIt(t *testing.T) {
	f := newFakeStack(t, "bench.perf.packet")
	f.drop = true
	cfg := e2eConfig(f)
	cfg.Iterations = 2
	cfg.E2ETimeout = 50 * time.Millisecond
	r := runE2ETrace(context.Background(), cfg)
	if r.N != 0 || r.Errors != 2 || r.ErrorRatio != 1 || !strings.Contains(r.FirstError, "not seen") {
		t.Fatalf("e2e-trace with dropped events: %+v", r)
	}
}

func TestE2ETraceNeedsApprovedName(t *testing.T) {
	f := newFakeStack(t, "bench.perf.packet")
	f.approved = false
	r := runE2ETrace(context.Background(), e2eConfig(f))
	if r.ErrorRatio != 1 || !strings.Contains(r.FirstError, "auto_approve") {
		t.Fatalf("pending subscriber: %+v", r)
	}
	if f.pubs.Load() != 0 {
		t.Fatal("published without an approved subscriber")
	}
}

func TestE2ETraceIgnoresOtherTopicsAndTraceIDs(t *testing.T) {
	// The ingested topic differs from the published one: the dashboard
	// never sees the bench event, only noise, so nothing may match.
	f := newFakeStack(t, "something.else")
	cfg := e2eConfig(f)
	cfg.Iterations = 1
	cfg.E2ETimeout = 30 * time.Millisecond
	r := runE2ETrace(context.Background(), cfg)
	if r.N != 0 || r.Errors != 1 {
		t.Fatalf("matched an unrelated event: %+v", r)
	}
}

func TestBenchPacketPayload(t *testing.T) {
	p := benchPacket(traceID{High: 7, Low: 9}, time.Unix(0, 42))
	var m map[string]any
	if err := json.Unmarshal(p, &m); err != nil {
		t.Fatal(err)
	}
	tid := m["trace_id"].(map[string]any)
	if tid["high"] != float64(7) || tid["low"] != float64(9) || m["timestamp_ns"] != float64(42) {
		t.Fatalf("payload %s", p)
	}
	fk := m["flow_key"].(map[string]any)
	// RFC 2544 benchmarking range, never a real host
	if !strings.HasPrefix(fk["src_addr"].(string), "198.18.") || !strings.HasPrefix(fk["dst_addr"].(string), "198.18.") {
		t.Fatalf("flow key %v", fk)
	}
}

// --- container-start ---

func csConfig(f *cuirasstest.Fake) Config {
	return Config{
		Iterations: 3,
		AllowLXD:   true,
		CuirassURL: f.URL,
		CuirassKey: cuirasstest.Key,
		CSImage:    "steel-echo-v1",
		CSTimeout:  5 * time.Second,
		Poll:       time.Millisecond,
	}
}

func TestContainerStartCreatesMeasuresAndDeletes(t *testing.T) {
	f := cuirasstest.New(t)
	f.HealthyAfter = 2
	r := runContainerStart(context.Background(), csConfig(f))
	if r.N != 3 || r.Errors != 0 {
		t.Fatalf("container-start: %+v", r)
	}
	f.Mu.Lock()
	defer f.Mu.Unlock()
	if len(f.Desired) != 0 || len(f.Actual) != 0 {
		t.Fatalf("left behind: desired %v actual %v", f.Desired, f.Actual)
	}
}

func TestContainerStartSpecIsLabelledAndUnexposed(t *testing.T) {
	f := cuirasstest.New(t)
	f.HealthyAfter = 1000 // never healthy within the timeout
	cfg := csConfig(f)
	cfg.Iterations = 1
	cfg.CSTimeout = 100 * time.Millisecond
	var (
		mu   sync.Mutex
		seen *cuirasstest.Spec
	)
	wrap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			body, _ := io.ReadAll(r.Body)
			var s cuirasstest.Spec
			_ = json.Unmarshal(body, &s)
			mu.Lock()
			seen = &s
			mu.Unlock()
			r.Body = io.NopCloser(strings.NewReader(string(body)))
		}
		f.ServeHTTP(w, r)
	}))
	defer wrap.Close()
	cfg.CuirassURL = wrap.URL
	r := runContainerStart(context.Background(), cfg)
	if r.Errors != 1 || r.N != 0 {
		t.Fatalf("never-healthy instance: %+v", r)
	}
	mu.Lock()
	defer mu.Unlock()
	if seen == nil || seen.ID != "steel-echo-bench-0" || seen.Image != "steel-echo-v1" ||
		seen.Labels["svc"] != "steel-echo-bench" || seen.Proxy != nil || seen.Labels["chaos"] != "" ||
		seen.Health == nil || seen.Health.Port != 8080 || seen.Health.Path != "/health" {
		t.Fatalf("spec %+v", seen)
	}
	f.Mu.Lock()
	defer f.Mu.Unlock()
	if len(f.Desired) != 0 {
		t.Fatalf("a failed start must still be deleted: %v", f.Desired)
	}
}

func TestContainerStartRefusesLeftovers(t *testing.T) {
	f := cuirasstest.New(t)
	f.Seed(cuirasstest.Spec{ID: "steel-echo-bench-0", Name: "steel-echo-bench-0", Image: "steel-echo-v1"})
	r := runContainerStart(context.Background(), csConfig(f))
	if r.N != 0 || !strings.Contains(r.FirstError, "steel-echo-bench-0") {
		t.Fatalf("leftover bench instance: %+v", r)
	}
	f.Mu.Lock()
	defer f.Mu.Unlock()
	if _, ok := f.Desired["steel-echo-bench-0"]; !ok {
		t.Fatal("the harness deleted an instance it did not create")
	}
}

func TestContainerStartWrongKey(t *testing.T) {
	f := cuirasstest.New(t)
	cfg := csConfig(f)
	cfg.CuirassKey = "wrong"
	cfg.Iterations = 1
	r := runContainerStart(context.Background(), cfg)
	if r.N != 0 || r.Errors != 1 || !strings.Contains(r.FirstError, "401") {
		t.Fatalf("wrong key: %+v", r)
	}
}

func TestRunAllWritesReport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	out := filepath.Join(t.TempDir(), "perf.json")
	cfg := Config{Iterations: 1, HealthURL: srv.URL, Rate: 50, Duration: 200 * time.Millisecond, Concurrency: 2,
		Curl: fakeCurl(t, "HTTP2", "")}
	if err := run(context.Background(), []string{"http-load", "http3"}, cfg, out, "host1", "c0ffee"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out) // #nosec G304 -- test temp file
	if err != nil {
		t.Fatal(err)
	}
	var rep Report
	if err := json.Unmarshal(b, &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Schema != "perf/v1" || len(rep.Results) != 2 || rep.Results[0].Name != "http-load" || rep.Results[1].Skipped == "" {
		t.Fatalf("report %s", fmt.Sprint(rep))
	}
}
