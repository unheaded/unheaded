// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Safe mode (Stevie, 2026-10-22): as first written, the first reconcile
// against west's real LXD would have force-deleted pihole, the only
// instance there, as "orphaned". Cuirass now touches only instances that
// carry ManagedLabel, deletes orphans only when pruning, and only observes
// unless enforcing.

func safeDaemon(t *testing.T, enforce, prune bool) (*Daemon, *MockLXDClient) {
	t.Helper()
	d := newTestDaemon(t)
	d.config.Enforce, d.config.Prune = enforce, prune
	m := d.lxdClient.(*MockLXDClient)
	// pihole: running, not ours.
	m.containers["pihole"] = &ContainerState{ID: "pihole", Status: "running", Health: "healthy"}
	return d, m
}

func TestReconcile_ObserveModeChangesNothing(t *testing.T) {
	d, m := safeDaemon(t, false, true)
	d.stateManager.desired["web"] = &ContainerSpec{ID: "web", Name: "web", Image: "nixos"}
	m.containers["old"] = &ContainerState{ID: "old", Status: "stopped", Managed: true}

	d.syncActualStateFromLXD()
	actions := d.planReconcile()
	d.reconcile()

	if _, ok := m.containers["web"]; ok {
		t.Error("observe mode created a container")
	}
	if _, ok := m.containers["old"]; !ok {
		t.Error("observe mode deleted a container")
	}
	if len(actions) != 2 {
		t.Fatalf("planned %d actions, want create web + delete old: %+v", len(actions), actions)
	}
}

func TestReconcile_ObserveModeMarksActionsPlanned(t *testing.T) {
	d, _ := safeDaemon(t, false, false)
	d.stateManager.desired["web"] = &ContainerSpec{ID: "web"}
	d.syncActualStateFromLXD()
	actions := d.planReconcile()
	for i := range actions {
		d.applyAction(t.Context(), &actions[i])
	}
	if len(actions) != 1 || !actions[0].Planned || actions[0].Success {
		t.Fatalf("want one planned, untaken create: %+v", actions)
	}
}

func TestReconcile_EnforceCreatesManaged(t *testing.T) {
	d, m := safeDaemon(t, true, false)
	d.stateManager.desired["web"] = &ContainerSpec{ID: "web", Name: "web", Image: "nixos"}
	d.reconcile()
	c, ok := m.containers["web"]
	if !ok || c.Status != "running" || !c.Managed {
		t.Fatalf("want web created, running and managed, got %+v", c)
	}
}

func TestReconcile_NeverDeletesUnmanaged(t *testing.T) {
	d, m := safeDaemon(t, true, true) // the most permissive mode
	for range 3 {
		d.reconcile()
	}
	if _, ok := m.containers["pihole"]; !ok {
		t.Fatal("reconcile deleted pihole, an instance Unheaded does not manage")
	}
	d.detectDrift()
	for _, dr := range d.stateManager.drifts {
		if dr.ContainerID == "pihole" {
			t.Errorf("unmanaged pihole reported as drift %q", dr.DriftType)
		}
	}
}

func TestReconcile_RefusesUnmanagedWithDesiredName(t *testing.T) {
	d, m := safeDaemon(t, true, true)
	m.containers["pihole"].Status = "stopped"
	d.stateManager.desired["pihole"] = &ContainerSpec{ID: "pihole"}
	d.syncActualStateFromLXD()
	actions := d.planReconcile()
	if len(actions) != 1 || actions[0].Action != "refuse" {
		t.Fatalf("want one refusal, got %+v", actions)
	}
	d.reconcile()
	if m.containers["pihole"].Status != "stopped" {
		t.Error("reconcile started an unmanaged instance")
	}
	d.detectDrift()
	if len(d.stateManager.drifts) != 1 || d.stateManager.drifts[0].DriftType != "unmanaged" {
		t.Errorf("want one unmanaged drift, got %+v", d.stateManager.drifts)
	}
}

func TestReconcile_PruneGatesManagedOrphans(t *testing.T) {
	for _, prune := range []bool{false, true} {
		d, m := safeDaemon(t, true, prune)
		m.containers["old"] = &ContainerState{ID: "old", Status: "running", Health: "healthy", Managed: true}
		d.reconcile()
		_, kept := m.containers["old"]
		if kept == prune {
			t.Errorf("prune=%v: managed orphan kept=%v", prune, kept)
		}
	}
}

func TestReconcile_ActualFollowsDeletesOutsideCuirass(t *testing.T) {
	d, m := safeDaemon(t, false, false)
	d.syncActualStateFromLXD()
	delete(m.containers, "pihole")
	d.syncActualStateFromLXD()
	if _, ok := d.stateManager.actual["pihole"]; ok {
		t.Error("actual state still lists an instance LXD no longer has")
	}
}

func TestContainerWrites_NeedAuth(t *testing.T) {
	for _, authOn := range []bool{false, true} {
		d := newTestDaemon(t)
		d.authEnabled = authOn
		mux := http.NewServeMux()
		d.registerHandlers(mux)

		put := httptest.NewRecorder()
		mux.ServeHTTP(put, httptest.NewRequest(http.MethodPut, "/api/v1/containers/web", strings.NewReader(`{"name":"web"}`)))
		del := httptest.NewRecorder()
		mux.ServeHTTP(del, httptest.NewRequest(http.MethodDelete, "/api/v1/containers/web", nil))
		get := httptest.NewRecorder()
		mux.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/v1/containers/web", nil))

		want := http.StatusForbidden
		if authOn {
			want = http.StatusOK
		}
		if put.Code != want || del.Code != want {
			t.Errorf("auth=%v: PUT %d, DELETE %d, want %d", authOn, put.Code, del.Code, want)
		}
		if get.Code == http.StatusForbidden {
			t.Errorf("auth=%v: GET refused; reads stay open", authOn)
		}
		_, desired := d.stateManager.desired["web"]
		if desired != false {
			t.Errorf("auth=%v: web still desired after PUT+DELETE", authOn)
		}
	}
}
