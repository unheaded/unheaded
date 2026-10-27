// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestParseKnobs_Bounds(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		rate    float64
		latency time.Duration
		wantErr bool
	}{
		{"unset is a healthy build", nil, 0, 0, false},
		{"bad build", map[string]string{"STEEL_ERROR_RATE": "0.3"}, 0.3, 0, false},
		{"all errors", map[string]string{"STEEL_ERROR_RATE": "1"}, 1, 0, false},
		{"latency", map[string]string{"STEEL_LATENCY_MS": "250"}, 0, 250 * time.Millisecond, false},
		{"latency at the cap", map[string]string{"STEEL_LATENCY_MS": "10000"}, 0, 10 * time.Second, false},
		{"negative rate", map[string]string{"STEEL_ERROR_RATE": "-0.1"}, 0, 0, true},
		{"rate over 1", map[string]string{"STEEL_ERROR_RATE": "1.01"}, 0, 0, true},
		{"rate NaN", map[string]string{"STEEL_ERROR_RATE": "NaN"}, 0, 0, true},
		{"rate Inf", map[string]string{"STEEL_ERROR_RATE": "+Inf"}, 0, 0, true},
		{"rate garbage", map[string]string{"STEEL_ERROR_RATE": "lots"}, 0, 0, true},
		{"negative latency", map[string]string{"STEEL_LATENCY_MS": "-1"}, 0, 0, true},
		{"latency over the cap", map[string]string{"STEEL_LATENCY_MS": "10001"}, 0, 0, true},
		{"latency garbage", map[string]string{"STEEL_LATENCY_MS": "1s"}, 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k, err := parseKnobs(envMap(tt.env))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if k.errorRate != tt.rate || k.latency != tt.latency {
				t.Fatalf("knobs = %+v, want rate %v latency %v", k, tt.rate, tt.latency)
			}
		})
	}
}

func newTestServer(t *testing.T, k knobs) *server {
	t.Helper()
	return newServer("v9", "steel-echo-v9-0", k, 42)
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestServe_HealthVersionRoot(t *testing.T) {
	h := newTestServer(t, knobs{}).handler()

	if rec := get(t, h, "/health"); rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != "ok" {
		t.Fatalf("/health = %d %q", rec.Code, rec.Body.String())
	}
	if rec := get(t, h, "/version"); rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != "v9" {
		t.Fatalf("/version = %d %q", rec.Code, rec.Body.String())
	}
	rec := get(t, h, "/")
	if rec.Code != 200 {
		t.Fatalf("/ = %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("/ body not JSON: %v", err)
	}
	if body["version"] != "v9" || body["hostname"] != "steel-echo-v9-0" || len(body) != 2 {
		t.Fatalf("/ body = %v, want exactly hostname and version", body)
	}
}

// Zero user data access: whatever a client sends, the answer carries only
// the hostname and version, never the request's path, query, headers or body.
func TestServe_NeverEchoesRequestData(t *testing.T) {
	h := newTestServer(t, knobs{}).handler()
	const marker = "SECRETMARKER7f3a"
	req := httptest.NewRequest(http.MethodGet, "/?q="+marker, strings.NewReader(marker))
	req.Header.Set("X-Probe", marker)
	req.Header.Set("Cookie", "session="+marker)
	req.Header.Set("User-Agent", marker)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if strings.Contains(rec.Body.String(), marker) {
		t.Fatalf("response echoes request data: %q", rec.Body.String())
	}
	for k, vs := range rec.Header() {
		for _, v := range vs {
			if strings.Contains(v, marker) {
				t.Fatalf("response header %s echoes request data", k)
			}
		}
	}
	// An unknown path is a plain 404, not an echo of the path.
	rec = get(t, h, "/"+marker)
	if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), marker) {
		t.Fatalf("unknown path = %d %q", rec.Code, rec.Body.String())
	}
}

func TestServe_RootRefusesWrites(t *testing.T) {
	h := newTestServer(t, knobs{}).handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("x")))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST / = %d, want 405", rec.Code)
	}
}

func TestServe_ErrorRateWithinFivePercent(t *testing.T) {
	const n = 10000
	h := newTestServer(t, knobs{errorRate: 0.3}).handler()
	errs := 0
	for i := 0; i < n; i++ {
		rec := get(t, h, "/")
		switch rec.Code {
		case 200:
		case 500:
			errs++
		default:
			t.Fatalf("unexpected status %d", rec.Code)
		}
	}
	got := float64(errs) / n
	if math.Abs(got-0.3) > 0.3*0.05 {
		t.Fatalf("error fraction %.4f, want 0.30 +/- 5%%", got)
	}
	// The knob applies to / only: health stays green on a bad build (so
	// Cuirass keeps it running and only canary analysis can catch it).
	for i := 0; i < 200; i++ {
		if rec := get(t, h, "/health"); rec.Code != 200 {
			t.Fatalf("/health = %d on a bad build", rec.Code)
		}
	}
}

func TestServe_ZeroRateNeverFails(t *testing.T) {
	h := newTestServer(t, knobs{}).handler()
	for i := 0; i < 2000; i++ {
		if rec := get(t, h, "/"); rec.Code != 200 {
			t.Fatalf("/ = %d with STEEL_ERROR_RATE unset", rec.Code)
		}
	}
}

func TestServe_LatencyKnob(t *testing.T) {
	s := newTestServer(t, knobs{latency: 80 * time.Millisecond})
	h := s.handler()
	start := time.Now()
	if rec := get(t, h, "/"); rec.Code != 200 {
		t.Fatalf("/ = %d", rec.Code)
	}
	if d := time.Since(start); d < 80*time.Millisecond {
		t.Fatalf("/ answered in %v, want >= 80ms", d)
	}
	start = time.Now()
	get(t, h, "/health")
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Fatalf("/health took %v: latency knob must not slow health", d)
	}
}

func TestServe_MetricsHouseNames(t *testing.T) {
	h := newTestServer(t, knobs{}).handler()
	for i := 0; i < 3; i++ {
		get(t, h, "/")
	}
	rec := get(t, h, "/metrics")
	found := map[string]bool{}
	for _, line := range strings.Split(rec.Body.String(), "\n") {
		if !strings.Contains(line, `service="steel-echo"`) || !strings.Contains(line, `path="/"`) {
			continue
		}
		for _, name := range []string{"unheaded_http_requests_total{", "unheaded_http_request_duration_seconds_bucket{"} {
			if strings.HasPrefix(line, name) {
				found[name] = true
			}
		}
	}
	if len(found) != 2 {
		t.Fatalf("/metrics lacks the house series for path /: found %v", found)
	}
}

func TestServe_ShutsDownOnSignalContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { errc <- serve(ctx, "127.0.0.1:0", newTestServer(t, knobs{})) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("serve returned %v after cancel, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not stop after its context ended")
	}
}

func TestShutdownSignals_IncludeSIGPWR(t *testing.T) {
	names := map[string]bool{}
	for _, s := range shutdownSignals {
		names[s.String()] = true
	}
	for _, want := range []string{"terminated", "interrupt", "power failure"} {
		if !names[want] {
			t.Fatalf("shutdown signals %v lack %q", shutdownSignals, want)
		}
	}
}

// --- load ---

func TestParseLoadFlags(t *testing.T) {
	good := [][]string{
		{"--target", "http://127.0.0.1:26601/", "--rps", "10", "--duration", "30s"},
		{"--discover", "http://127.0.0.1:19006", "--svc", "steel-echo"},
	}
	for _, args := range good {
		if _, err := parseLoadFlags(args); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	bad := [][]string{
		{}, // neither target nor discover
		{"--target", "http://a/", "--discover", "http://b", "--svc", "x"}, // both
		{"--discover", "http://127.0.0.1:19006"},                          // no svc
		{"--target", "ftp://127.0.0.1/"},
		{"--target", "http://127.0.0.1/", "--rps", "0"},
		{"--target", "http://127.0.0.1/", "--rps", "5001"},
		{"--target", "http://127.0.0.1/", "--duration", "0s"},
		{"--target", "http://127.0.0.1/", "--duration", "25h"},
		{"--discover", "http://127.0.0.1:19006", "--svc", "Bad Name"},
		{"--discover", "http://127.0.0.1:19006", "--svc", "x", "--port", "0"},
		{"--target", "http://user:pw@127.0.0.1/"},
	}
	for _, args := range bad {
		if _, err := parseLoadFlags(args); err == nil {
			t.Fatalf("%v: accepted, want an error", args)
		}
	}
}

// fakeCuirass serves /api/v1/state/actual from a mutable map.
type fakeCuirass struct {
	mu    sync.Mutex
	state map[string]any
	calls atomic.Int64
	paths sync.Map
}

func (f *fakeCuirass) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.calls.Add(1)
	f.paths.Store(r.Method+" "+r.URL.Path, true)
	if r.Method != http.MethodGet || r.URL.Path != "/api/v1/state/actual" {
		http.NotFound(w, r)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	_ = json.NewEncoder(w).Encode(f.state)
}

func inst(ip, svc, status, health string, managed bool) map[string]any {
	return map[string]any{"ip": ip, "status": status, "health": health, "managed": managed,
		"labels": map[string]string{"svc": svc}}
}

func TestDiscover_PicksHealthyManagedInstancesOfTheService(t *testing.T) {
	fc := &fakeCuirass{state: map[string]any{
		"steel-echo-v1-0": inst("10.10.10.21", "steel-echo", "running", "healthy", true),
		"steel-echo-v1-1": inst("10.10.10.22", "steel-echo", "running", "unhealthy", true),
		"steel-echo-v1-2": inst("10.10.10.23", "steel-echo", "stopped", "healthy", true),
		"other-v1-0":      inst("10.10.10.24", "other", "running", "healthy", true),
		"pihole":          inst("10.10.10.25", "steel-echo", "running", "healthy", false),
		"steel-echo-v1-3": inst("not-an-ip", "steel-echo", "running", "healthy", true),
		"steel-echo-v1-4": inst("127.0.0.1", "steel-echo", "running", "healthy", true),
	}}
	srv := httptest.NewServer(fc)
	defer srv.Close()

	d := newDiscoverer(srv.URL, "steel-echo", 8080, srv.Client())
	if err := d.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := d.snapshot()
	if len(got) != 1 || got[0] != "http://10.10.10.21:8080/" {
		t.Fatalf("targets = %v, want only steel-echo-v1-0", got)
	}
	// Discover reads only the actual state, with GET.
	n := 0
	fc.paths.Range(func(k, _ any) bool {
		n++
		if k.(string) != "GET /api/v1/state/actual" {
			t.Fatalf("discover called %s", k)
		}
		return true
	})
	if n != 1 {
		t.Fatalf("discover made %d distinct calls", n)
	}
}

func TestDiscover_RemovedInstanceDroppedWithinOnePeriod(t *testing.T) {
	fc := &fakeCuirass{state: map[string]any{
		"steel-echo-v1-0": inst("10.10.10.21", "steel-echo", "running", "healthy", true),
		"steel-echo-v1-1": inst("10.10.10.22", "steel-echo", "running", "healthy", true),
	}}
	srv := httptest.NewServer(fc)
	defer srv.Close()

	const period = 100 * time.Millisecond
	d := newDiscoverer(srv.URL, "steel-echo", 8080, srv.Client())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := d.refresh(ctx); err != nil {
		t.Fatal(err)
	}
	go d.run(ctx, period)
	if got := d.snapshot(); len(got) != 2 {
		t.Fatalf("targets = %v, want 2", got)
	}

	fc.mu.Lock()
	delete(fc.state, "steel-echo-v1-1")
	fc.mu.Unlock()
	removed := time.Now()

	deadline := removed.Add(period + period/2) // one period plus scheduling slack
	for time.Now().Before(deadline) {
		if got := d.snapshot(); len(got) == 1 {
			if got[0] != "http://10.10.10.21:8080/" {
				t.Fatalf("kept the wrong instance: %v", got)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("removed instance still targeted %v after one period: %v", time.Since(removed), d.snapshot())
}

func TestDiscover_FailedRefreshKeepsLastTargets(t *testing.T) {
	fc := &fakeCuirass{state: map[string]any{
		"steel-echo-v1-0": inst("10.10.10.21", "steel-echo", "running", "healthy", true),
	}}
	srv := httptest.NewServer(fc)
	d := newDiscoverer(srv.URL, "steel-echo", 8080, srv.Client())
	if err := d.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	srv.Close()
	if err := d.refresh(context.Background()); err == nil {
		t.Fatal("refresh against a closed Cuirass succeeded")
	}
	if got := d.snapshot(); len(got) != 1 {
		t.Fatalf("a failed refresh changed the targets: %v", got)
	}
}

func TestMeter_CSVColumnsAndPercentiles(t *testing.T) {
	m := newMeter()
	for i := 1; i <= 100; i++ {
		m.record(1000, result{status: 200, latency: time.Duration(i) * time.Millisecond, version: "v1"})
	}
	m.record(1000, result{status: 500, latency: 5 * time.Millisecond, version: "v2"})
	m.record(1000, result{connErr: true})
	m.record(1001, result{status: 200, latency: time.Millisecond, version: "v2"})
	m.record(1001, result{status: 200, latency: time.Millisecond, version: "bad,version\n=cmd"})

	var buf bytes.Buffer
	if err := m.writeHeader(&buf); err != nil {
		t.Fatal(err)
	}
	if err := m.flushBefore(&buf, 2000); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatalf("CSV does not parse: %v", err)
	}
	want := []string{"ts", "sent", "ok", "err5xx", "errconn", "p50_ms", "p99_ms", "versions_seen"}
	if strings.Join(rows[0], ",") != strings.Join(want, ",") {
		t.Fatalf("header = %v, want %v", rows[0], want)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want header + 2 seconds", len(rows))
	}
	r := rows[1]
	if r[0] != "1000" || r[1] != "102" || r[2] != "100" || r[3] != "1" || r[4] != "1" {
		t.Fatalf("second 1000 = %v", r)
	}
	p50, _ := strconv.ParseFloat(r[5], 64)
	p99, _ := strconv.ParseFloat(r[6], 64)
	if p50 < 49 || p50 > 52 || p99 < 98 || p99 > 101 {
		t.Fatalf("p50 %v p99 %v", p50, p99)
	}
	if r[7] != "v1;v2" {
		t.Fatalf("versions_seen = %q, want v1;v2", r[7])
	}
	// A version string that is not a plain token is never written as-is.
	if rows[2][7] != "?;v2" {
		t.Fatalf("second 1001 versions = %q, want ?;v2", rows[2][7])
	}
}

func TestLoad_AgainstSteelEcho(t *testing.T) {
	srv := httptest.NewServer(newTestServer(t, knobs{errorRate: 0.5}).handler())
	defer srv.Close()

	cfg := loadConfig{target: srv.URL + "/", rps: 50, duration: 1200 * time.Millisecond, timeout: time.Second}
	var out bytes.Buffer
	sum, err := runLoad(context.Background(), cfg, &out, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(&out).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 2 {
		t.Fatalf("CSV has %d rows", len(rows))
	}
	var sent, ok, e5 int
	for _, r := range rows[1:] {
		s, _ := strconv.Atoi(r[1])
		o, _ := strconv.Atoi(r[2])
		e, _ := strconv.Atoi(r[3])
		sent, ok, e5 = sent+s, ok+o, e5+e
	}
	if sent != sum.sent || ok != sum.ok || sent < 40 {
		t.Fatalf("CSV totals sent %d ok %d vs summary %+v", sent, ok, sum)
	}
	if e5 == 0 || ok == 0 || ok+e5 != sent {
		t.Fatalf("with a 50%% error build: sent %d ok %d 5xx %d", sent, ok, e5)
	}
}

func TestLoad_ConnectionErrorsCounted(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL + "/"
	srv.Close()
	cfg := loadConfig{target: url, rps: 20, duration: 500 * time.Millisecond, timeout: 200 * time.Millisecond}
	sum, err := runLoad(context.Background(), cfg, io.Discard, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	if sum.sent == 0 || sum.errconn != sum.sent || sum.ok != 0 {
		t.Fatalf("summary %+v, want every request a connection error", sum)
	}
}

// FND-023: Cuirass keeps a killed replica "healthy" until its next sync, so a
// discover refresh alone kept sending it a share of the traffic. The meter
// now drops a target on its first connection error and re-admits it only
// when Cuirass still lists it and a direct probe answers.
func TestDiscover_DeadTargetDroppedUntilProbeAnswers(t *testing.T) {
	fc := &fakeCuirass{state: map[string]any{
		"steel-echo-v1-0": inst("10.10.10.21", "steel-echo", "running", "healthy", true),
		"steel-echo-v1-1": inst("10.10.10.22", "steel-echo", "running", "healthy", true),
	}}
	srv := httptest.NewServer(fc)
	defer srv.Close()

	const a, b = "http://10.10.10.21:8080/", "http://10.10.10.22:8080/"
	d := newDiscoverer(srv.URL, "steel-echo", 8080, srv.Client())
	var probeOK atomic.Bool
	var probed sync.Map
	d.probe = func(_ context.Context, target string) bool { probed.Store(target, true); return probeOK.Load() }
	ctx := context.Background()
	if err := d.refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if got := d.snapshot(); len(got) != 2 {
		t.Fatalf("targets = %v, want 2", got)
	}

	// An answered request, even a 5xx, is a live replica: it stays.
	d.observe(b, result{status: http.StatusInternalServerError})
	d.observe(b, result{status: http.StatusOK})
	if got := d.snapshot(); len(got) != 2 {
		t.Fatalf("an answering replica was dropped: %v", got)
	}
	// The first connection error drops it at once, before any refresh.
	d.observe(b, result{connErr: true})
	if got := d.snapshot(); len(got) != 1 || got[0] != a {
		t.Fatalf("after a connection error targets = %v, want only %s", got, a)
	}
	// Cuirass still lists it (its sync lags) and the probe fails: stays out.
	if err := d.refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if got := d.snapshot(); len(got) != 1 || got[0] != a {
		t.Fatalf("a listed but dead replica came back: %v", got)
	}
	if _, ok := probed.Load(b); !ok {
		t.Fatal("refresh did not probe the dead target")
	}
	if _, ok := probed.Load(a); ok {
		t.Fatal("refresh probed a live target")
	}
	// The probe answers: re-admitted.
	probeOK.Store(true)
	if err := d.refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if got := d.snapshot(); len(got) != 2 {
		t.Fatalf("a replica whose probe answers was not re-admitted: %v", got)
	}
	// Dead again, then Cuirass drops it: the dead mark goes with it, so a
	// later replica on the same address is not born dead.
	d.observe(b, result{connErr: true})
	fc.mu.Lock()
	delete(fc.state, "steel-echo-v1-1")
	fc.mu.Unlock()
	if err := d.refresh(ctx); err != nil {
		t.Fatal(err)
	}
	fc.mu.Lock()
	fc.state["steel-echo-v1-2"] = inst("10.10.10.22", "steel-echo", "running", "healthy", true)
	fc.mu.Unlock()
	probeOK.Store(false)
	if err := d.refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if got := d.snapshot(); len(got) != 2 {
		t.Fatalf("a new replica on a once-dead address was kept out: %v", got)
	}
}

func TestDiscover_ProbeNeedsA2xx(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer ok.Close()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer bad.Close()
	gone := httptest.NewServer(http.NotFoundHandler())
	goneURL := gone.URL + "/"
	gone.Close()
	d := newDiscoverer("http://127.0.0.1:1", "steel-echo", 8080, http.DefaultClient)
	ctx := context.Background()
	if !d.probe(ctx, ok.URL+"/") {
		t.Fatal("a 200 probe was refused")
	}
	if d.probe(ctx, bad.URL+"/") {
		t.Fatal("a 503 probe re-admitted the target")
	}
	if d.probe(ctx, goneURL) {
		t.Fatal("a closed port re-admitted the target")
	}
}
