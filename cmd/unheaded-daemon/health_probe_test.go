// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeProber records every probe and answers from a function.
type fakeProber struct {
	mu    sync.Mutex
	calls []string // "ip:port path"
	err   func(ip string) error
}

func (p *fakeProber) Probe(_ context.Context, ip string, port int, path string) error {
	p.mu.Lock()
	p.calls = append(p.calls, net.JoinHostPort(ip, strconv.Itoa(port))+" "+path)
	f := p.err
	p.mu.Unlock()
	if f == nil {
		return nil
	}
	return f(ip)
}

func (p *fakeProber) callList() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.calls...)
}

func TestHealthSpec_Validation(t *testing.T) {
	long := "/" + strings.Repeat("a", 256)
	for _, tc := range []struct {
		name string
		h    HealthSpec
		ok   bool
	}{
		{"valid", HealthSpec{Port: 8080, Path: "/health"}, true},
		{"valid root", HealthSpec{Port: 1, Path: "/"}, true},
		{"port 0", HealthSpec{Port: 0, Path: "/health"}, false},
		{"port 65536", HealthSpec{Port: 65536, Path: "/health"}, false},
		{"negative port", HealthSpec{Port: -1, Path: "/health"}, false},
		{"no leading slash", HealthSpec{Port: 8080, Path: "health"}, false},
		{"empty path", HealthSpec{Port: 8080, Path: ""}, false},
		{"dot dot", HealthSpec{Port: 8080, Path: "/a/../admin"}, false},
		{"absolute URL", HealthSpec{Port: 8080, Path: "http://169.254.169.254/latest"}, false},
		{"scheme relative", HealthSpec{Port: 8080, Path: "//evil.example/x"}, false},
		{"query", HealthSpec{Port: 8080, Path: "/health?x=1"}, false},
		{"fragment", HealthSpec{Port: 8080, Path: "/health#x"}, false},
		{"control char", HealthSpec{Port: 8080, Path: "/health\r\nHost: x"}, false},
		{"too long", HealthSpec{Port: 8080, Path: long}, false},
	} {
		err := tc.h.validate()
		if (err == nil) != tc.ok {
			t.Errorf("%s: validate() = %v, want ok=%v", tc.name, err, tc.ok)
		}
	}
}

// A PUT with a bad health spec is refused before anything is stored.
func TestContainerPut_RefusesBadHealthSpec(t *testing.T) {
	d := newTestDaemon(t)
	d.authEnabled = true
	mux := http.NewServeMux()
	d.registerHandlers(mux)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/v1/containers/web",
		strings.NewReader(`{"image":"steel-echo-v1","health":{"port":8080,"path":"http://10.0.0.1/"}}`)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("PUT with an absolute-URL health path: %d, want 400", w.Code)
	}
	if _, ok := d.stateManager.desired["web"]; ok {
		t.Error("refused spec was stored")
	}
}

// countingLXD counts restarts on top of another client.
type countingLXD struct {
	LXDClient
	restarts atomic.Int32
}

func (c *countingLXD) RestartContainer(ctx context.Context, name string) error {
	c.restarts.Add(1)
	return c.LXDClient.RestartContainer(ctx, name)
}

// healthDaemon: enforce mode, one running managed instance "web" at
// 10.10.20.7 desired with a health spec, and a fake prober.
func healthDaemon(t *testing.T, enforce bool) (*Daemon, *MockLXDClient, *fakeProber) {
	t.Helper()
	d, m := safeDaemon(t, enforce, false)
	p := &fakeProber{}
	d.prober = p
	m.containers["web"] = &ContainerState{ID: "web", Status: "running", Health: "unknown", Managed: true, IP: "10.10.20.7"}
	d.stateManager.desired["web"] = &ContainerSpec{ID: "web", Image: "steel-echo-v1", Health: &HealthSpec{Port: 8080, Path: "/health"}}
	return d, m, p
}

// The probe goes to the instance IP LXD reported, at the spec's port and
// path: no field of the desired spec names a host.
func TestReconcile_ProbesTheInstanceIP(t *testing.T) {
	d, _, p := healthDaemon(t, false)
	d.reconcile()
	calls := p.callList()
	if len(calls) != 1 || calls[0] != "10.10.20.7:8080 /health" {
		t.Fatalf("probes %v, want one to 10.10.20.7:8080 /health", calls)
	}
	if h := d.stateManager.actual["web"].Health; h != "healthy" {
		t.Errorf("health %q after a passing probe, want healthy", h)
	}
}

func TestReconcile_ThreeFailedProbesMarkUnhealthyAndPlanRestart(t *testing.T) {
	d, m, p := healthDaemon(t, true)
	p.err = func(string) error { return errors.New("connection refused") }
	c := &countingLXD{LXDClient: m}
	d.lxdClient = c
	restarts := &c.restarts

	for i := 1; i <= 2; i++ {
		d.reconcile()
		if h := d.stateManager.actual["web"].Health; h == "unhealthy" {
			t.Fatalf("unhealthy after %d failed probe(s), want 3", i)
		}
	}
	if restarts.Load() != 0 {
		t.Fatal("restarted before the third failure")
	}
	d.stateManager.mu.Lock()
	_ = d.syncActualStateFromLXD(t.Context())
	d.probeHealth(t.Context())
	h := d.stateManager.actual["web"].Health
	actions := d.planReconcile()
	d.stateManager.mu.Unlock()
	if h != "unhealthy" {
		t.Fatalf("health %q after 3 failed probes, want unhealthy", h)
	}
	if len(actions) != 1 || actions[0].Action != "restart" || actions[0].ContainerID != "web" {
		t.Fatalf("plan %+v, want one restart of web", actions)
	}

	// One success makes it healthy again.
	p.mu.Lock()
	p.err = nil
	p.mu.Unlock()
	d.reconcile()
	if h := d.stateManager.actual["web"].Health; h != "healthy" {
		t.Errorf("health %q after a passing probe, want healthy", h)
	}
}

// Observe mode probes and reports, but the restart stays planned.
func TestReconcile_UnhealthyInObserveModeIsOnlyPlanned(t *testing.T) {
	d, m, p := healthDaemon(t, false)
	p.err = func(string) error { return errors.New("500") }
	c := &countingLXD{LXDClient: m}
	d.lxdClient = c
	restarts := &c.restarts
	for range 4 {
		d.reconcile()
	}
	if restarts.Load() != 0 {
		t.Error("observe mode restarted an unhealthy instance")
	}
}

// Without a health spec Cuirass does not probe and does not invent health.
func TestReconcile_NoHealthSpecStaysUnknown(t *testing.T) {
	f := newFakeLXD()
	f.instances["web"] = instance("web", "Running", map[string]string{ManagedLabel: "true"})
	d := newTestDaemon(t)
	d.lxdClient = unixLXD(t, f, "")
	p := &fakeProber{}
	d.prober = p
	d.stateManager.desired["web"] = &ContainerSpec{ID: "web", Image: "x"}
	d.reconcile()
	if len(p.callList()) != 0 {
		t.Errorf("probed %v with no health spec", p.callList())
	}
	if h := d.stateManager.actual["web"].Health; h != "unknown" {
		t.Errorf("health %q, want unknown", h)
	}
}

// An instance whose reported address is loopback, link-local, unspecified
// or multicast is not probed (a container picks its own addresses; this
// keeps the probe off the host's own services): the check fails instead.
func TestReconcile_RefusesToProbeNonInstanceAddresses(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "::1", "169.254.169.254", "fe80::1", "0.0.0.0", "224.0.0.1", "not-an-ip"} {
		d, m, p := healthDaemon(t, false)
		m.containers["web"].IP = ip
		for range 3 {
			d.reconcile()
		}
		if calls := p.callList(); len(calls) != 0 {
			t.Errorf("ip %s: probed %v", ip, calls)
		}
		if h := d.stateManager.actual["web"].Health; h != "unhealthy" {
			t.Errorf("ip %s: health %q after 3 refusals, want unhealthy", ip, h)
		}
	}
}

// The real prober: 2xx is healthy; a redirect is not followed; anything
// else, or no answer within the timeout, fails; HTTP_PROXY is ignored.
func TestHTTPProber(t *testing.T) {
	var otherHit atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { otherHit.Store(true) }))
	t.Cleanup(other.Close)
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("/big", func(w http.ResponseWriter, r *http.Request) {
		chunk := make([]byte, 64<<10)
		for i := 0; i < 160; i++ { // 10 MiB
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	})
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL+"/x", http.StatusFound) })
	mux.HandleFunc("/500", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) })
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(3 * time.Second):
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	host, portStr, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	port, _ := strconv.Atoi(portStr)

	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("http_proxy", "http://127.0.0.1:1")
	p := newHTTPProber(300 * time.Millisecond)
	ctx := context.Background()

	if err := p.Probe(ctx, host, port, "/ok"); err != nil {
		t.Errorf("/ok: %v (HTTP_PROXY must not be used)", err)
	}
	if err := p.Probe(ctx, host, port, "/big"); err != nil {
		t.Errorf("/big: %v (the body is discarded after 4 KiB, not read to the end)", err)
	}
	if err := p.Probe(ctx, host, port, "/redirect"); err == nil || otherHit.Load() {
		t.Errorf("/redirect: err=%v, other server hit=%v; want failure and no follow", err, otherHit.Load())
	}
	if err := p.Probe(ctx, host, port, "/500"); err == nil {
		t.Error("/500 counted as healthy")
	}
	start := time.Now()
	if err := p.Probe(ctx, host, port, "/slow"); err == nil {
		t.Error("/slow counted as healthy")
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("/slow took %v, want the timeout", took)
	}
}
