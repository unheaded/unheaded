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
	"testing"
)

func scrape(t *testing.T, d *Daemon) string {
	t.Helper()
	mux := http.NewServeMux()
	d.registerHandlers(mux)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	return w.Body.String()
}

// failingStart makes every start fail.
type failingStart struct{ LXDClient }

func (failingStart) StartContainer(context.Context, string) error { return errors.New("lxd says no") }

func TestMetrics_ReconcileActionsByResult(t *testing.T) {
	// observe: planned
	d, _ := safeDaemon(t, false, false)
	d.stateManager.desired["web"] = &ContainerSpec{ID: "web"}
	d.stateManager.desired["pihole"] = &ContainerSpec{ID: "pihole"} // refused: not ours
	d.reconcile()
	body := scrape(t, d)
	for _, want := range []string{
		`cuirass_reconcile_actions_total{action="create",result="planned"} 1`,
		`cuirass_reconcile_actions_total{action="refuse",result="refused"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("observe: /metrics lacks %s", want)
		}
	}

	// enforce: done, and failed
	d2, m := safeDaemon(t, true, false)
	d2.stateManager.desired["web"] = &ContainerSpec{ID: "web"}
	d2.reconcile()
	m.containers["web"].Status = "stopped"
	d2.lxdClient = failingStart{m}
	d2.reconcile()
	body = scrape(t, d2)
	for _, want := range []string{
		`cuirass_reconcile_actions_total{action="create",result="done"} 1`,
		`cuirass_reconcile_actions_total{action="start",result="failed"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("enforce: /metrics lacks %s\n%s", want, body)
		}
	}
}

// /api/v1/info says which backend and modes are live, so mock and real
// can be told apart without reading the environment.
func TestInfo_ReportsBackendAndModes(t *testing.T) {
	d := newTestDaemon(t)
	d.config.LXDBackend, d.config.LXDProject = "https", "unheaded"
	d.config.Enforce, d.config.Prune, d.config.Chaos = true, false, true
	mux := http.NewServeMux()
	d.registerHandlers(mux)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/info", nil))
	var info struct {
		LXDBackend string `json:"lxd_backend"`
		LXDProject string `json:"lxd_project"`
		Enforce    *bool  `json:"enforce"`
		Prune      *bool  `json:"prune"`
		Chaos      *bool  `json:"chaos"`
	}
	if err := json.NewDecoder(w.Body).Decode(&info); err != nil {
		t.Fatal(err)
	}
	if info.LXDBackend != "https" || info.LXDProject != "unheaded" || info.Enforce == nil || !*info.Enforce ||
		info.Prune == nil || *info.Prune || info.Chaos == nil || !*info.Chaos {
		t.Errorf("info %+v", info)
	}
	d.config.LXDBackend = ""
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/info", nil))
	if !strings.Contains(w.Body.String(), `"lxd_backend":"mock"`) {
		t.Errorf("empty backend should read mock: %s", w.Body.String())
	}
}
