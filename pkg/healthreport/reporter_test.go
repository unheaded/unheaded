// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package healthreport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/health"
)

// fakePub records every publish.
type fakePub struct {
	mu   sync.Mutex
	msgs []published
	got  chan struct{}
	err  error
}

type published struct {
	topic   string
	payload []byte
}

func newFakePub() *fakePub { return &fakePub{got: make(chan struct{}, 64)} }

func (f *fakePub) Publish(_ context.Context, topic string, payload []byte) error {
	f.mu.Lock()
	f.msgs = append(f.msgs, published{topic, append([]byte(nil), payload...)})
	f.mu.Unlock()
	select {
	case f.got <- struct{}{}:
	default:
	}
	return f.err
}

func (f *fakePub) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.msgs)
}

func (f *fakePub) last(t *testing.T) (string, health.HealthReport) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	m := f.msgs[len(f.msgs)-1]
	// Contract: the payload decodes into Akira's own report type, the one
	// cmd/akira's consumeReports unmarshals from system.health.reports, and
	// carries no field that type does not know.
	var r health.HealthReport
	dec := json.NewDecoder(strings.NewReader(string(m.payload)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		t.Fatalf("payload %s does not decode into health.HealthReport: %v", m.payload, err)
	}
	return m.topic, r
}

func wait(t *testing.T, f *fakePub) {
	t.Helper()
	select {
	case <-f.got:
	case <-time.After(5 * time.Second):
		t.Fatal("no report published")
	}
}

func TestStartPublishesAkiraReports(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pub := newFakePub()
	Start(ctx, pub, Config{
		Service:  "timeguru",
		Host:     "west",
		Interval: 20 * time.Millisecond,
		Probe:    func(context.Context) error { return nil },
	})
	wait(t, pub) // first report at start, not after one interval
	wait(t, pub) // then once per interval
	topic, r := pub.last(t)
	if topic != Topic || Topic != "system.health.reports" {
		t.Fatalf("topic = %q", topic)
	}
	if r.Service != "timeguru" || r.Host != "west" || r.Reporter != "timeguru@west" {
		t.Fatalf("report identity = %+v", r)
	}
	if !r.Healthy || r.Error != "" {
		t.Fatalf("healthy probe reported %+v", r)
	}
	if r.Timestamp.IsZero() {
		t.Fatal("report has no timestamp")
	}

	// Akira's ballot accepts it as one vote.
	b := health.NewBallot()
	b.Record(r)
	v := b.Tally(time.Now())
	if len(v) != 1 || v[0].Reporters != 1 || v[0].Failing != 0 {
		t.Fatalf("ballot tally = %+v", v)
	}
}

func TestStartStopsOnContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	pub := newFakePub()
	done := Start(ctx, pub, Config{
		Service: "captain", Host: "east", Interval: 10 * time.Millisecond,
		Probe: func(context.Context) error { return nil },
	})
	wait(t, pub)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("reporter did not stop on ctx")
	}
	n := pub.count()
	time.Sleep(50 * time.Millisecond)
	if pub.count() != n {
		t.Fatal("reporter published after ctx ended")
	}
}

func TestProbeErrorReportsFailingTruncated(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pub := newFakePub()
	long := strings.Repeat("x", 500)
	Start(ctx, pub, Config{
		Service: "sophia", Host: "west", Interval: time.Hour,
		Probe: func(context.Context) error { return errors.New(long) },
	})
	wait(t, pub)
	_, r := pub.last(t)
	if r.Healthy {
		t.Fatal("probe error reported healthy")
	}
	if len(r.Error) != MaxErrorBytes || MaxErrorBytes != 200 {
		t.Fatalf("error is %d bytes, want 200", len(r.Error))
	}
}

func TestTruncateKeepsUTF8Valid(t *testing.T) {
	s := strings.Repeat("a", 199) + "é" // é is two bytes: straddles byte 200
	if got := truncate(s, 200); got != strings.Repeat("a", 199) {
		t.Fatalf("truncate split a rune: %q", got[190:])
	}
	if got := truncate("short", 200); got != "short" {
		t.Fatalf("truncate changed a short string: %q", got)
	}
}

func TestPublishErrorDoesNotStopReporter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pub := newFakePub()
	pub.err = errors.New("wotan down")
	Start(ctx, pub, Config{
		Service: "monad", Host: "west", Interval: 10 * time.Millisecond,
		Probe: func(context.Context) error { return nil },
	})
	wait(t, pub)
	wait(t, pub)
}

func TestStartRejectsProgrammingErrors(t *testing.T) {
	ok := Config{Service: "s", Host: "h", Probe: func(context.Context) error { return nil }}
	cases := map[string]func(){
		"nil publisher": func() { Start(context.Background(), nil, ok) },
		"nil probe": func() {
			c := ok
			c.Probe = nil
			Start(context.Background(), newFakePub(), c)
		},
		"no service": func() {
			c := ok
			c.Service = ""
			Start(context.Background(), newFakePub(), c)
		},
		"no host": func() {
			c := ok
			c.Host = ""
			Start(context.Background(), newFakePub(), c)
		},
	}
	for name, fn := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("Start did not panic")
				}
			}()
			fn()
		})
	}
}

func TestDefaultInterval(t *testing.T) {
	// One report per Akira sweep, so a reporter's vote never goes stale
	// between two of its reports.
	if DefaultInterval != health.HealthCheckInterval {
		t.Fatalf("DefaultInterval = %v, Akira sweeps every %v", DefaultInterval, health.HealthCheckInterval)
	}
}

func TestHTTPProbe(t *testing.T) {
	var mu sync.Mutex
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("probe hit %s", r.URL.Path)
		}
		mu.Lock()
		defer mu.Unlock()
		w.WriteHeader(status)
	}))
	defer srv.Close()
	probe := HTTPProbe(srv.URL + "/health")
	if err := probe(context.Background()); err != nil {
		t.Fatalf("200: %v", err)
	}
	mu.Lock()
	status = http.StatusServiceUnavailable
	mu.Unlock()
	if err := probe(context.Background()); err == nil || !strings.Contains(err.Error(), "HTTP 503") {
		t.Fatalf("503: %v", err)
	}
	srv.Close()
	if err := probe(context.Background()); err == nil {
		t.Fatal("closed server probed healthy")
	}
}

func TestHealthURL(t *testing.T) {
	cases := []struct {
		listen string
		port   int
		want   string
	}{
		{":19000", 19000, "http://127.0.0.1:19000/health"},
		{"0.0.0.0:19001", 19001, "http://127.0.0.1:19001/health"},
		{"[::]:19002", 19002, "http://127.0.0.1:19002/health"},
		{"", 20000, "http://127.0.0.1:20000/health"},
		{"10.10.10.20:19003", 19003, "http://10.10.10.20:19003/health"},
		{"[::1]:19004", 19004, "http://[::1]:19004/health"},
		{"localhost:19005", 19005, "http://localhost:19005/health"},
		{"garbage", 19006, "http://127.0.0.1:19006/health"},
	}
	for _, c := range cases {
		if got := HealthURL(c.listen, c.port); got != c.want {
			t.Errorf("HealthURL(%q, %d) = %q, want %q", c.listen, c.port, got, c.want)
		}
	}
}

func TestBallotName(t *testing.T) {
	t.Setenv("HEALTH_REPORT_SERVICE", "")
	cases := map[string]string{
		"kanban-app":        "kanban",
		"dashboard-backend": "dashboard",
		"unheaded-daemon":   "daemon",
		"timeguru":          "timeguru",
		"sword":             "sword",
	}
	for in, want := range cases {
		if got := BallotName(in); got != want {
			t.Errorf("BallotName(%q) = %q, want %q", in, got, want)
		}
	}
	t.Setenv("HEALTH_REPORT_SERVICE", "cuirass")
	if got := BallotName("unheaded-daemon"); got != "cuirass" {
		t.Errorf("override ignored: %q", got)
	}
}

func TestHostName(t *testing.T) {
	t.Setenv("KINGDOM_NODE", "east")
	if got := HostName(); got != "east" {
		t.Fatalf("HostName = %q", got)
	}
	t.Setenv("KINGDOM_NODE", "")
	if got := HostName(); got == "" {
		t.Fatal("HostName empty without KINGDOM_NODE")
	}
}
