// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func reconcileNow(d *Daemon) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	d.registerHandlers(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/reconcile", nil))
	return rec
}

func TestReconcileNow_NeedsAuth(t *testing.T) {
	d, m := safeDaemon(t, true, false)
	d.stateManager.desired["web"] = &ContainerSpec{ID: "web", Image: "x"}
	rec := reconcileNow(d)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST /api/v1/reconcile without AUTH_ENABLED = %d, want 403", rec.Code)
	}
	if _, ok := m.containers["web"]; ok {
		t.Fatal("a refused reconcile still acted")
	}
}

func TestReconcileNow_RunsOneCycleAndReturnsActions(t *testing.T) {
	d, m := safeDaemon(t, true, false)
	d.authEnabled = true
	d.stateManager.desired["web"] = &ContainerSpec{ID: "web", Image: "x"}
	rec := reconcileNow(d)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Actions []ReconcileAction `json:"actions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Actions) != 1 || body.Actions[0].Action != "create" || !body.Actions[0].Success {
		t.Fatalf("actions = %+v, want one successful create", body.Actions)
	}
	if c, ok := m.containers["web"]; !ok || c.Status != "running" {
		t.Fatalf("web not created and started: %+v", c)
	}
	if _, ok := m.containers["pihole"]; !ok {
		t.Fatal("pihole touched")
	}
}

func TestReconcileNow_ObserveModeOnlyPlans(t *testing.T) {
	d, m := safeDaemon(t, false, false)
	d.authEnabled = true
	d.stateManager.desired["web"] = &ContainerSpec{ID: "web", Image: "x"}
	rec := reconcileNow(d)
	var body struct {
		Actions []ReconcileAction `json:"actions"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusOK || len(body.Actions) != 1 || !body.Actions[0].Planned {
		t.Fatalf("observe mode: %d %+v", rec.Code, body.Actions)
	}
	if _, ok := m.containers["web"]; ok {
		t.Fatal("observe mode created an instance")
	}
}

func TestReconcileNow_RateLimited(t *testing.T) {
	d, _ := safeDaemon(t, true, false)
	d.authEnabled = true
	if rec := reconcileNow(d); rec.Code != http.StatusOK {
		t.Fatalf("first call = %d", rec.Code)
	}
	rec := reconcileNow(d)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("second call within a second = %d (Retry-After %q), want 429", rec.Code, rec.Header().Get("Retry-After"))
	}
	d.reconcileNowMu.Lock()
	d.lastReconcileNow = time.Now().Add(-2 * time.Second)
	d.reconcileNowMu.Unlock()
	if rec := reconcileNow(d); rec.Code != http.StatusOK {
		t.Fatalf("after a second = %d", rec.Code)
	}
}

func TestReconcileNow_OnlyPost(t *testing.T) {
	d, _ := safeDaemon(t, true, false)
	d.authEnabled = true
	mux := http.NewServeMux()
	d.registerHandlers(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/reconcile", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET = %d, want 405", rec.Code)
	}
}

func TestReconcileNow_LXDDownIs502(t *testing.T) {
	d, _ := safeDaemon(t, true, false)
	d.authEnabled = true
	d.lxdClient = &failingList{LXDClient: d.lxdClient}
	if rec := reconcileNow(d); rec.Code != http.StatusBadGateway {
		t.Fatalf("LXD down = %d, want 502", rec.Code)
	}
}

type failingList struct{ LXDClient }

func (f *failingList) ListContainers(context.Context) (map[string]*ContainerState, error) {
	return nil, errLXDNotManaged
}

// proxyOrder records UpdateProxy calls in order.
type proxyOrder struct {
	LXDClient
	mu    sync.Mutex
	calls []string
}

func (p *proxyOrder) UpdateProxy(ctx context.Context, name string, spec *ProxySpec) error {
	p.mu.Lock()
	if spec == nil {
		p.calls = append(p.calls, "remove "+name)
	} else {
		p.calls = append(p.calls, "add "+name)
	}
	p.mu.Unlock()
	return p.LXDClient.UpdateProxy(ctx, name, spec)
}

// Moving the stable endpoint from v3 back to v2 (a rollback): v2 sorts
// first, but LXD cannot bind the listen address twice, so the removal from
// v3 must be applied before the add on v2 in the same reconcile.
func TestReconcile_ProxyRemovalsApplyFirst(t *testing.T) {
	d, m := safeDaemon(t, true, false)
	px := &ProxySpec{Listen: "tcp:127.0.0.1:26601", Connect: "tcp:127.0.0.1:8080"}
	m.containers["svc-v2-0"] = &ContainerState{ID: "svc-v2-0", Status: "running", Managed: true}
	m.containers["svc-v3-0"] = &ContainerState{ID: "svc-v3-0", Status: "running", Managed: true, Proxy: copyProxy(px)}
	d.stateManager.desired["svc-v2-0"] = &ContainerSpec{ID: "svc-v2-0", Image: "svc-v2", Proxy: px}
	d.stateManager.desired["svc-v3-0"] = &ContainerSpec{ID: "svc-v3-0", Image: "svc-v3"}
	rec := &proxyOrder{LXDClient: m}
	d.lxdClient = rec
	d.reconcile()
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.calls) != 2 || rec.calls[0] != "remove svc-v3-0" || rec.calls[1] != "add svc-v2-0" {
		t.Fatalf("UpdateProxy order = %v, want removal first", rec.calls)
	}
}
