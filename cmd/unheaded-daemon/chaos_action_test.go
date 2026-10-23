// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// chaosRecorder records ChaosStop calls on top of another client.
type chaosRecorder struct {
	LXDClient
	mu    sync.Mutex
	stops []string // "name force=<bool>"
}

func (c *chaosRecorder) ChaosStop(ctx context.Context, name string, force bool) error {
	c.mu.Lock()
	if force {
		c.stops = append(c.stops, name+" force=true")
	} else {
		c.stops = append(c.stops, name+" force=false")
	}
	c.mu.Unlock()
	return c.LXDClient.ChaosStop(ctx, name, force)
}

type chaosFixture struct {
	d      *Daemon
	m      *MockLXDClient
	rec    *chaosRecorder
	mux    *http.ServeMux
	mu     sync.Mutex
	events []map[string]any
}

// newChaosFixture: auth and chaos on, enforce mode; "steel-echo-v1-0" is
// managed, running and labelled chaos=allowed; "plain-0" is managed and
// running without the label; pihole is running and not ours.
func newChaosFixture(t *testing.T) *chaosFixture {
	t.Helper()
	d, m := safeDaemon(t, true, false)
	d.authEnabled = true
	d.config.Chaos = true
	m.containers["steel-echo-v1-0"] = &ContainerState{ID: "steel-echo-v1-0", Status: "running", Managed: true,
		Labels: map[string]string{"chaos": "allowed", "svc": "steel-echo"}}
	m.containers["plain-0"] = &ContainerState{ID: "plain-0", Status: "running", Managed: true}
	d.stateManager.desired["steel-echo-v1-0"] = &ContainerSpec{ID: "steel-echo-v1-0", Image: "steel-echo-v1", Labels: map[string]string{"chaos": "allowed"}}
	d.stateManager.desired["plain-0"] = &ContainerSpec{ID: "plain-0", Image: "x"}
	f := &chaosFixture{d: d, m: m, rec: &chaosRecorder{LXDClient: m}}
	d.lxdClient = f.rec
	d.chaosPublish = func(_ context.Context, payload []byte) error {
		var ev map[string]any
		if err := json.Unmarshal(payload, &ev); err != nil {
			t.Errorf("chaos event is not JSON: %v", err)
		}
		f.mu.Lock()
		f.events = append(f.events, ev)
		f.mu.Unlock()
		return nil
	}
	d.syncActualStateFromLXD(t.Context())
	f.mux = http.NewServeMux()
	d.registerHandlers(f.mux)
	return f
}

func (f *chaosFixture) post(id, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/containers/"+id+"/chaos", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, r)
	return w
}

func (f *chaosFixture) eventCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.events)
}

func (f *chaosFixture) stopCalls() []string {
	f.rec.mu.Lock()
	defer f.rec.mu.Unlock()
	return append([]string(nil), f.rec.stops...)
}

const killBody = `{"action":"kill","reason":"gameday scenario A"}`

func TestChaos_NeedsAuth(t *testing.T) {
	f := newChaosFixture(t)
	f.d.authEnabled = false
	if w := f.post("steel-echo-v1-0", killBody); w.Code != http.StatusForbidden {
		t.Errorf("without AUTH_ENABLED: %d, want 403", w.Code)
	}
	if calls := f.stopCalls(); len(calls) != 0 {
		t.Errorf("stopped %v without auth", calls)
	}
}

func TestChaos_OffUnlessEnabled(t *testing.T) {
	f := newChaosFixture(t)
	f.d.config.Chaos = false
	if w := f.post("steel-echo-v1-0", killBody); w.Code != http.StatusForbidden {
		t.Errorf("without CUIRASS_CHAOS: %d, want 403", w.Code)
	}
	if calls := f.stopCalls(); len(calls) != 0 {
		t.Errorf("stopped %v with chaos off", calls)
	}
}

// Observe mode has no auto-revert (nothing starts the instance again), so
// chaos is refused there even when switched on.
func TestChaos_NeedsEnforce(t *testing.T) {
	f := newChaosFixture(t)
	f.d.config.Enforce = false
	if w := f.post("steel-echo-v1-0", killBody); w.Code != http.StatusForbidden {
		t.Errorf("observe mode: %d, want 403", w.Code)
	}
	if calls := f.stopCalls(); len(calls) != 0 {
		t.Errorf("stopped %v in observe mode", calls)
	}
}

func TestChaos_RefusesUnmanagedAndUnlabelled(t *testing.T) {
	f := newChaosFixture(t)
	if w := f.post("pihole", killBody); w.Code != http.StatusConflict {
		t.Errorf("pihole (not ours): %d, want 409", w.Code)
	}
	if w := f.post("plain-0", killBody); w.Code != http.StatusConflict {
		t.Errorf("managed without chaos=allowed: %d, want 409", w.Code)
	}
	if w := f.post("nope-0", killBody); w.Code != http.StatusNotFound {
		t.Errorf("unknown instance: %d, want 404", w.Code)
	}
	if calls := f.stopCalls(); len(calls) != 0 {
		t.Errorf("stopped %v", calls)
	}
	if f.m.containers["pihole"].Status != "running" || f.m.containers["plain-0"].Status != "running" {
		t.Error("a refused target changed state")
	}
}

// permissiveChaos stops anything it is asked to: a client that has lost its
// own checks.
type permissiveChaos struct {
	LXDClient
	stops int
}

func (p *permissiveChaos) ChaosStop(context.Context, string, bool) error { p.stops++; return nil }

// The handler refuses on its own, whatever the client would do.
func TestChaos_HandlerChecksOnItsOwn(t *testing.T) {
	f := newChaosFixture(t)
	p := &permissiveChaos{LXDClient: f.m}
	f.d.lxdClient = p
	for _, id := range []string{"pihole", "plain-0"} {
		if w := f.post(id, killBody); w.Code != http.StatusConflict {
			t.Errorf("%s: %d, want 409 from the handler's own check", id, w.Code)
		}
	}
	if p.stops != 0 {
		t.Errorf("the client was asked %d times", p.stops)
	}
}

// The client checks managed and the label itself, against LXD, whatever the
// handler's cached view said.
func TestChaos_ClientChecksIndependently(t *testing.T) {
	fl := newFakeLXD()
	fl.instances["plain"] = instance("plain", "Running", map[string]string{ManagedLabel: "true"})
	fl.instances["ok"] = instance("ok", "Running", map[string]string{ManagedLabel: "true", lxdLabelPrefix + "chaos": "allowed"})
	c := unixLXD(t, fl, "")
	ctx := t.Context()
	if err := c.ChaosStop(ctx, "pihole", true); !errors.Is(err, errLXDNotManaged) {
		t.Errorf("pihole: %v, want errLXDNotManaged", err)
	}
	if err := c.ChaosStop(ctx, "plain", true); !errors.Is(err, errChaosNotAllowed) {
		t.Errorf("plain: %v, want errChaosNotAllowed", err)
	}
	for _, line := range strings.Split(fl.callLog(), "\n") {
		if !strings.HasPrefix(line, "GET ") {
			t.Errorf("refused chaos sent a write: %s", line)
		}
	}
	if err := c.ChaosStop(ctx, "ok", false); err != nil {
		t.Fatalf("ok: %v", err)
	}
	if fl.instances["ok"]["status"] != "Stopped" || fl.instances["pihole"]["status"] != "Running" {
		t.Error("wrong instance stopped")
	}
}

func TestChaos_StopAndKill(t *testing.T) {
	f := newChaosFixture(t)
	if w := f.post("steel-echo-v1-0", `{"action":"stop","reason":"scenario B"}`); w.Code != http.StatusOK {
		t.Fatalf("stop: %d %s", w.Code, w.Body.String())
	}
	f.m.containers["steel-echo-v1-0"].Status = "running"
	if w := f.post("steel-echo-v1-0", killBody); w.Code != http.StatusOK {
		t.Fatalf("kill: %d %s", w.Code, w.Body.String())
	}
	calls := f.stopCalls()
	if len(calls) != 2 || calls[0] != "steel-echo-v1-0 force=false" || calls[1] != "steel-echo-v1-0 force=true" {
		t.Errorf("calls %v, want stop (force=false) then kill (force=true)", calls)
	}
}

func TestChaos_BadRequests(t *testing.T) {
	f := newChaosFixture(t)
	for _, tc := range []struct {
		name, body string
		want       int
	}{
		{"unknown action", `{"action":"delete","reason":"x"}`, http.StatusBadRequest},
		{"no reason", `{"action":"kill"}`, http.StatusBadRequest},
		{"reason too long", `{"action":"kill","reason":"` + strings.Repeat("a", 257) + `"}`, http.StatusBadRequest},
		{"control chars in reason", `{"action":"kill","reason":"a\nb"}`, http.StatusBadRequest},
		{"body too large", `{"action":"kill","reason":"` + strings.Repeat("a", 5000) + `"}`, http.StatusRequestEntityTooLarge},
		{"not JSON", `kill`, http.StatusBadRequest},
	} {
		if w := f.post("steel-echo-v1-0", tc.body); w.Code != tc.want {
			t.Errorf("%s: %d, want %d", tc.name, w.Code, tc.want)
		}
	}
	r := httptest.NewRequest(http.MethodPost, "/api/v1/containers/steel-echo-v1-0/chaos", strings.NewReader(killBody))
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, r) // no Content-Type
	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("no content type: %d, want 415", w.Code)
	}
	get := httptest.NewRecorder()
	f.mux.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/v1/containers/steel-echo-v1-0/chaos", nil))
	if get.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET chaos: %d, want 405", get.Code)
	}
	if calls := f.stopCalls(); len(calls) != 0 {
		t.Errorf("bad requests stopped %v", calls)
	}
}

// Every call, allowed or refused, is published on cuirass.chaos with what was
// asked and what came of it.
func TestChaos_EveryCallIsPublished(t *testing.T) {
	f := newChaosFixture(t)
	f.post("steel-echo-v1-0", killBody) // done
	f.post("pihole", killBody)          // refused: not ours
	f.d.config.Chaos = false
	f.post("steel-echo-v1-0", killBody) // refused: chaos off
	f.d.authEnabled = false
	f.post("steel-echo-v1-0", killBody) // refused: no auth
	if n := f.eventCount(); n != 4 {
		t.Fatalf("%d events, want 4", n)
	}
	want := []string{"done", "refused", "refused", "refused"}
	for i, ev := range f.events {
		if ev["result"] != want[i] || ev["action"] != "kill" || ev["reason"] != "gameday scenario A" || ev["id"] == "" {
			t.Errorf("event %d = %v, want result %s", i, ev, want[i])
		}
	}
}

// The reconciler is not told: in enforce mode the next sync starts a killed
// instance again. That is the auto-revert.
func TestChaos_ReconcileRestartsAfterKill(t *testing.T) {
	f := newChaosFixture(t)
	if w := f.post("steel-echo-v1-0", killBody); w.Code != http.StatusOK {
		t.Fatalf("kill: %d", w.Code)
	}
	if s := f.m.containers["steel-echo-v1-0"].Status; s != "stopped" {
		t.Fatalf("status after kill %q, want stopped", s)
	}
	f.d.reconcile()
	if s := f.m.containers["steel-echo-v1-0"].Status; s != "running" {
		t.Errorf("status after reconcile %q, want running again", s)
	}
}
