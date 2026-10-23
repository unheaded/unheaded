// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package cuirassdriver

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/deploy/rollback"
)

// mapLookup is an in-memory deployment history, newest last.
type mapLookup struct{ deps []*rollback.DeploymentInfo }

func (m *mapLookup) Get(_ context.Context, id string) (*rollback.DeploymentInfo, error) {
	for _, d := range m.deps {
		if d.ID == id {
			return d, nil
		}
	}
	return nil, rollback.ErrDeploymentNotFound
}

func (m *mapLookup) GetPrevious(_ context.Context, svc, before string) (*rollback.DeploymentInfo, error) {
	seen := false
	for i := len(m.deps) - 1; i >= 0; i-- {
		d := m.deps[i]
		if d.ID == before {
			seen = true
			continue
		}
		if seen && d.ServiceName == svc && d.State == "succeeded" {
			return d, nil
		}
	}
	return nil, rollback.ErrNoRollbackTarget
}

func (m *mapLookup) GetByService(_ context.Context, svc string, limit int) ([]*rollback.DeploymentInfo, error) {
	var out []*rollback.DeploymentInfo
	for _, d := range m.deps {
		if d.ServiceName == svc {
			out = append(out, d)
		}
	}
	return out, nil
}

func seedV2(f *fakeCuirass) {
	for i, id := range []string{"steel-echo-v2-0", "steel-echo-v2-1"} {
		spec := containerSpec{ID: id, Image: "steel-echo-v2", Labels: map[string]string{"svc": "steel-echo", "ver": "v2"},
			Health: &HealthSpec{Port: 8080, Path: "/health"}}
		if i == 0 {
			spec.Proxy = testProxy
		}
		f.seed(spec)
	}
}

func history() *mapLookup {
	return &mapLookup{deps: []*rollback.DeploymentInfo{
		{ID: "dep-1", ServiceName: "steel-echo", Version: "v1", State: "succeeded", Replicas: 2},
		{ID: "dep-2", ServiceName: "steel-echo", Version: "v2", State: "succeeded", Replicas: 2},
	}}
}

func newManager(t *testing.T, srv string, lookup rollback.DeploymentLookup) *rollback.Manager {
	t.Helper()
	d := newTestDriver(t, srv)
	specs := func(svc string) (ServiceSpec, error) {
		if svc != "steel-echo" {
			return ServiceSpec{}, errors.New("unknown service")
		}
		return testSpec(), nil
	}
	return rollback.NewManager(lookup, &Updater{Driver: d, Specs: specs}, &Shifter{Driver: d, Specs: specs})
}

func onlyVersion(t *testing.T, f *fakeCuirass, ver string, n int) {
	t.Helper()
	ids := f.desiredIDs()
	if len(ids) != n {
		t.Fatalf("desired = %v, want %d instances of %s", ids, n, ver)
	}
	for _, id := range ids {
		if !strings.HasPrefix(id, "steel-echo-"+ver+"-") {
			t.Fatalf("desired = %v, want only %s", ids, ver)
		}
	}
	holders := f.proxyHolders()
	if len(holders) != 1 || !strings.HasPrefix(holders[0], "steel-echo-"+ver+"-") {
		t.Fatalf("proxy holders = %v, want one %s instance", holders, ver)
	}
}

func TestRollback_RestoresPreviousVersionThroughDriver(t *testing.T) {
	f, srv := newFakeCuirass(t)
	seedV2(f)
	m := newManager(t, srv.URL, history())

	rb, err := m.Rollback(context.Background(), &rollback.RollbackRequest{DeploymentID: "dep-2", Reason: rollback.RollbackReasonManual})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done, err := m.Wait(ctx, rb.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.State != rollback.RollbackStateCompleted || done.ToVersion != "v1" {
		t.Fatalf("rollback = %+v", done)
	}
	onlyVersion(t, f, "v1", 2)
}

func TestRollback_NoPreviousVersionIsAnError(t *testing.T) {
	f, srv := newFakeCuirass(t)
	seedV2(f)
	m := newManager(t, srv.URL, &mapLookup{deps: []*rollback.DeploymentInfo{
		{ID: "dep-2", ServiceName: "steel-echo", Version: "v2", State: "succeeded", Replicas: 2},
	}})
	if _, err := m.Rollback(context.Background(), &rollback.RollbackRequest{DeploymentID: "dep-2", Reason: rollback.RollbackReasonManual}); err == nil {
		t.Fatal("rollback with no previous deployment succeeded")
	}
	if ids := f.desiredIDs(); len(ids) != 2 {
		t.Fatalf("a refused rollback changed desired: %v", ids)
	}
}

func TestRollback_ManagerWithoutUpdaterFails(t *testing.T) {
	m := rollback.NewManager(history(), nil, nil)
	rb, err := m.Rollback(context.Background(), &rollback.RollbackRequest{DeploymentID: "dep-2"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done, err := m.Wait(ctx, rb.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.State != rollback.RollbackStateFailed {
		t.Fatalf("rollback with no updater ended %s, want failed", done.State)
	}
}

func TestAutoRollbackMonitor_TriggersOnFailedHealth(t *testing.T) {
	f, srv := newFakeCuirass(t)
	seedV2(f)
	m := newManager(t, srv.URL, history())
	mon := rollback.NewAutoRollbackMonitor(m, &rollback.AutoRollbackConfig{Enabled: true, HealthCheckFailures: 2})
	mon.Watch("dep-2", "steel-echo")

	mon.RecordHealthCheckFailure(context.Background(), "dep-2")
	if m.IsRollbackInProgress("steel-echo") {
		t.Fatal("rolled back after one failure, threshold is two")
	}
	mon.RecordHealthCheckFailure(context.Background(), "dep-2")

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		rbs, _ := m.GetByService(context.Background(), "steel-echo")
		if len(rbs) == 1 {
			if rbs[0].State != rollback.RollbackStateCompleted || rbs[0].Reason != rollback.RollbackReasonHealthCheck {
				t.Fatalf("auto rollback = %+v", rbs[0])
			}
			onlyVersion(t, f, "v1", 2)
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no automatic rollback after two failed health checks")
}
